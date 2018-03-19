/*-
 * Copyright (c) 2018, F5 Networks, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package controller

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"sync"
	"time"

	log "github.com/F5Networks/f5-ipam-ctlr/pkg/vlogger"

	"github.com/F5Networks/f5-ipam-ctlr/pkg/manager"
	"github.com/F5Networks/f5-ipam-ctlr/pkg/orchestration"
	ipStore "github.com/F5Networks/f5-ipam-ctlr/pkg/store"
)

const A = "A"
const CNAME = "CNAME"

// Controller manages communication between the orchestration and ipam system
type Controller struct {
	oClient orchestration.Client
	iClient manager.Client
	// Channel for receiving data from the orchestration
	oChan <-chan bytes.Buffer
	// Interval at which to verify the IPAM system configuration
	vInterval int
	// Store for tracking IP to hosts mappings
	store *ipStore.Store
}

// NewController creates a Controller object
func NewController(
	oClient orchestration.Client,
	iClient manager.Client,
	oChan <-chan bytes.Buffer,
	verifyInterval int,
) *Controller {
	return &Controller{
		oClient:   oClient,
		iClient:   iClient,
		oChan:     oChan,
		vInterval: verifyInterval,
		store:     ipStore.NewStore(),
	}
}

// Run starts up the orchestration client and processes IPGroups as they are received
func (ctlr *Controller) Run(stopCh <-chan struct{}) {
	log.Infof("Controller started: (%p)", ctlr)

	ctlr.refreshStore()
	ctlr.oClient.Run(stopCh)
	handleIPGroups := func(ipGroup *orchestration.IPGroup, ipBytes bytes.Buffer) {
		ipGroup.Groups = make(orchestration.IPGroups)
		err := gob.NewDecoder(&ipBytes).Decode(&ipGroup.Groups)
		if err != nil {
			log.Errorf("Could not decode IPGroups: %v", err)
			return
		}

		if ipGroup != nil {
			log.Debugf("Controller received %v hosts from Orchestration client.",
				ipGroup.NumHosts())
			ctlr.processIPGroups(ipGroup)
			ctlr.writeIPHosts()
		} else {
			log.Error("Controller could not get data from Orchestration client.")
		}
	}
	go func() {
		log.Debug("Controller waiting for updates from Orchestration client.")
		ipGroup := &orchestration.IPGroup{
			GroupMutex: new(sync.Mutex),
		}
		for {
			if ctlr.vInterval != 0 {
				select {
				case ipBytes := <-ctlr.oChan:
					handleIPGroups(ipGroup, ipBytes)
				case <-time.After(time.Duration(ctlr.vInterval) * time.Second):
					// Periodically check the IPAM system to see if any changes have occurred
					if ipGroup != nil && ctlr.refreshStore() {
						ctlr.processIPGroups(ipGroup)
					}
				}
			} else {
				select {
				case ipBytes := <-ctlr.oChan:
					handleIPGroups(ipGroup, ipBytes)
				}
			}
		}
	}()
}

// Read through IPGroups, allocate/release IP addresses for hosts, and keep store updated
func (ctlr *Controller) processIPGroups(ipGroup *orchestration.IPGroup) {
	ipGroup.GroupMutex.Lock()
	defer ipGroup.GroupMutex.Unlock()

	var uniqueIP, addrUsed bool
	var nextAddr, curAddr, netview, cidr string
	// Loop through IP Groups and create any new records
	for group, specs := range ipGroup.Groups {
		var firstHost string
		if group.Name == "" {
			uniqueIP = true
		} else {
			uniqueIP = false
		}
		// If group is not "", then the whole group shares an IP address
		if !uniqueIP {
			addrUsed = false
			netview = group.Netview
			cidr = group.Cidr
			nextAddr = ctlr.iClient.GetNextAddr(netview, cidr)
			curAddr = nextAddr
		}
		for _, spec := range specs {
			// If group is "", then each Spec gets its own IP address
			if uniqueIP {
				addrUsed = false
				netview = spec.Netview
				cidr = spec.Cidr
				nextAddr = ctlr.iClient.GetNextAddr(netview, cidr)
				curAddr = nextAddr
			}
			for i, host := range spec.Hosts {
				// Save the first host added (for use as CNAME for remaining hosts)
				if i == 0 {
					if !uniqueIP && firstHost == "" {
						firstHost = host
					} else if uniqueIP {
						firstHost = host
					}
				}

				if ctlr.store.GetIP(host) != "" {
					// This host already has an IP address
					curAddr = ctlr.store.GetIP(host)
					continue
				} else {
					var recordType string
					if host == firstHost {
						if ctlr.iClient.CreateARecord(host, nextAddr, netview) {
							addrUsed = true
							recordType = A
						} else {
							continue
						}
					} else {
						ctlr.iClient.CreateCNAMERecord(host, firstHost, netview)
						recordType = CNAME
					}
					// add this record to our internal store
					ctlr.store.AddRecord(curAddr, host, recordType, netview, cidr)
				}
			}
			// If nextAddr was reserved but not used, release it
			if uniqueIP && !addrUsed {
				ctlr.iClient.ReleaseAddr(netview, cidr, nextAddr)
			}
		}
		// If nextAddr was reserved but not used, release it
		if !uniqueIP && !addrUsed {
			ctlr.iClient.ReleaseAddr(netview, cidr, nextAddr)
		}
	}

	// Loop through our internal store and find which records can be deleted
	ipGroupHosts := ipGroup.GetAllHosts()
	var toRemove []string
	for ip, hosts := range ctlr.store.Records {
		netview := ctlr.store.Netviews[ip]
		cidr := ctlr.store.Cidrs[ip]
		availHosts := mapKeys(hosts)
		for host, recordType := range hosts {
			if !contains(ipGroupHosts, host) {
				availHosts = removeElement(availHosts, host)
				// Delete the record from the IPAM system
				if recordType == A {
					if len(availHosts) > 0 {
						// We need to pick a new A record since there are still CNAMEs
						newA, err := ctlr.iClient.CNAMEToA(
							availHosts, host, ip, netview, cidr)
						if err != nil {
							log.Warningf("%v", err)
						}
						ctlr.store.Records[ip][newA] = A
					} else {
						ctlr.iClient.DeleteARecord(host, ip, netview, cidr)
					}
				} else if recordType == CNAME {
					ctlr.iClient.DeleteCNAMERecord(host, ip, netview, cidr)
				}
				// Delete the host from our internal store
				toRemove = append(toRemove, host)
			}
		}
	}
	ctlr.store.DeleteHosts(toRemove)
}

// Writes the IP addresses and their hosts back to the orchestration
func (ctlr *Controller) writeIPHosts() {
	for ip, rec := range ctlr.store.Records {
		hosts := mapKeys(rec)
		ctlr.oClient.AnnotateResources(ip, hosts)
	}
}

// On startup, refreshes the internal store to match the IPAM system's records
func (ctlr *Controller) refreshStore() bool {
	ipamStore := ctlr.iClient.GetRecords()
	if !reflect.DeepEqual(ctlr.store, ipamStore) {
		ctlr.store = ipamStore
		return true
	}
	return false
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// Returns a list of map keys
func mapKeys(m map[string]string) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func removeElement(s []string, val string) []string {
	for i, ele := range s {
		if ele == val {
			s = append(s[:i], s[i+1:]...)
			break
		}
	}
	return s
}
