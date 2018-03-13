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

package manager

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	log "github.com/F5Networks/f5-ipam-ctlr/pkg/vlogger"

	store "github.com/F5Networks/f5-ipam-ctlr/pkg/store"

	ibclient "github.com/infobloxopen/infoblox-go-client"
)

// Map of host records to their Infoblox "ref" strings
type recordRef map[string]string

// Convert ibclient.ObjectManager to interface for unit testing
type objectManager interface {
	AllocateIP(netview, cidr, ipAddr, macAddress, vmID string) (*ibclient.FixedAddress, error)
	ReleaseIP(netview, cidr, ipAddr, macAddr string) (string, error)
}

// Client that the controller uses to talk to Infoblox
type IBloxClient struct {
	conn      ibclient.IBConnector
	objMgr    objectManager
	recordRef recordRef
}

// Struct to allow NewInfobloxClient to receive all or some parameters
type InfobloxParams struct {
	Host     string
	Version  string
	Port     int
	Username string
	Password string
}

// Sets up an interface with Infoblox
func NewInfobloxClient(
	params *InfobloxParams,
	cmpType string,
) (*IBloxClient, error) {
	hostConfig := ibclient.HostConfig{
		Host:     params.Host,
		Version:  params.Version,
		Port:     strconv.Itoa(params.Port),
		Username: params.Username,
		Password: params.Password,
	}
	// TransportConfig params: sslVerify, httpRequestsTimeout, httpPoolConnections
	// These are the common values
	transportConfig := ibclient.NewTransportConfig("false", 20, 10)
	requestBuilder := &ibclient.WapiRequestBuilder{}
	requestor := &ibclient.WapiHttpRequestor{}
	conn, err := ibclient.NewConnector(hostConfig, transportConfig, requestBuilder, requestor)
	if err != nil {
		return nil, err
	}
	objMgr := ibclient.NewObjectManager(conn, cmpType, "0")

	iClient := &IBloxClient{
		conn:      conn,
		objMgr:    objMgr,
		recordRef: make(recordRef),
	}

	return iClient, nil
}

// Creates an A record
func (iClient *IBloxClient) CreateARecord(name, ipAddr, netview string) {
	var res []ibclient.RecordA
	obj := ibclient.NewRecordA(
		ibclient.RecordA{
			Name:     name,
			Ipv4Addr: ipAddr,
			View:     netview,
		},
	)
	err := iClient.conn.GetObject(obj, "", &res)
	// If error or record already exists, return
	if err != nil || (res != nil && len(res) > 0) {
		return
	}

	log.Infof("Creating A record for '%s' as '%s'.", name, ipAddr)
	ref, _ := iClient.conn.CreateObject(obj)
	iClient.recordRef[name] = ref
}

// Deletes an A record and releases the IP address
func (iClient *IBloxClient) DeleteARecord(name, ipAddr, netview, cidr string) {
	var res []ibclient.RecordA
	obj := ibclient.NewRecordA(
		ibclient.RecordA{
			Name:     name,
			Ipv4Addr: ipAddr,
			View:     netview,
		},
	)
	err := iClient.conn.GetObject(obj, "", &res)
	if err != nil {
		return
	}
	// If record doesn't exist, release IP and return
	if res == nil || len(res) == 0 {
		log.Infof("Releasing IP address '%s'.", ipAddr)
		iClient.ReleaseAddr(netview, cidr, ipAddr)
		return
	}

	for _, record := range res {
		netCidr := iClient.getNetworkCIDR(netview, record.Ipv4Addr)
		if netCidr == cidr {
			log.Infof("Deleting A record for '%s'.", name)
			iClient.conn.DeleteObject(record.Ref)
			delete(iClient.recordRef, name)
		}
	}

	log.Infof("Releasing IP address '%s'.", ipAddr)
	iClient.ReleaseAddr(netview, cidr, ipAddr)
}

// Creates a CNAME record
func (iClient *IBloxClient) CreateCNAMERecord(name, canonical, netview string) {
	var res []ibclient.RecordCNAME
	obj := ibclient.NewRecordCNAME(
		ibclient.RecordCNAME{
			Name:      name,
			Canonical: canonical,
			View:      netview,
		},
	)
	err := iClient.conn.GetObject(obj, "", &res)
	// If error or record already exists, return
	if err != nil || (res != nil && len(res) > 0) {
		return
	}

	log.Infof("Creating CNAME record for '%s' as '%s'.", name, canonical)
	ref, _ := iClient.conn.CreateObject(obj)
	iClient.recordRef[name] = ref
}

// Deletes a CNAME record
func (iClient *IBloxClient) DeleteCNAMERecord(name, ipAddr, netview, cidr string) {
	var res []ibclient.RecordCNAME
	obj := ibclient.NewRecordCNAME(
		ibclient.RecordCNAME{
			Name: name,
			View: netview,
		},
	)
	err := iClient.conn.GetObject(obj, "", &res)
	// If error or record doesn't exist, return
	if err != nil || res == nil || len(res) == 0 {
		return
	}

	for _, record := range res {
		netCidr := iClient.getNetworkCIDR(netview, ipAddr)
		if netCidr == cidr {
			log.Infof("Deleting CNAME record for '%s'.", name)
			iClient.conn.DeleteObject(record.Ref)
			delete(iClient.recordRef, name)
		}
	}
}

// Changes an available CNAME to an A record, updates all ensuing CNAMEs
// in the list to point to the new record
func (iClient *IBloxClient) CNAMEToA(
	availHosts []string,
	delHost,
	ipAddr,
	netview,
	cidr string,
) (string, error) {
	var aRecord string
	for i, host := range availHosts {
		if i == 0 {
			// Make the first available host the new A record
			obj := ibclient.NewRecordA(
				ibclient.RecordA{
					Name:     host,
					Ipv4Addr: ipAddr,
				},
			)
			oldRef, found := iClient.recordRef[delHost]
			if !found {
				iClient.DeleteARecord(delHost, ipAddr, netview, cidr)
				return "", fmt.Errorf(
					"Couldn't find reference to host '%s' "+
						"while trying to update CNAME to A record.", delHost)
			}
			aRecord = host
			// Delete the CNAME record first
			iClient.DeleteCNAMERecord(host, ipAddr, netview, cidr)
			// Update the A record
			log.Infof("Updating A record for '%s' to be '%s'.", delHost, host)
			newRef, _ := iClient.conn.UpdateObject(obj, oldRef)
			iClient.recordRef[host] = newRef
		} else {
			// Update all subsequent CNAMEs to point to new A record
			obj := ibclient.NewRecordCNAME(
				ibclient.RecordCNAME{
					Name:      host,
					Canonical: aRecord,
					View:      netview,
				},
			)
			oldRef, found := iClient.recordRef[host]
			if !found {
				return "", fmt.Errorf(
					"Couldn't find reference to host '%s' "+
						"while trying to update CNAME.", host)
			}
			log.Infof("Updating CNAME record for '%s' to point at '%s'.", host, aRecord)
			newRef, _ := iClient.conn.UpdateObject(obj, oldRef)
			iClient.recordRef[host] = newRef
		}
	}
	return aRecord, nil
}

// Reserves and returns the next available address in the network
func (iClient *IBloxClient) GetNextAddr(netview, cidr string) string {
	nextAddr, err := iClient.objMgr.AllocateIP(netview, cidr, "", "", "")
	if err != nil {
		return ""
	}
	return ipFromRef(nextAddr.IPAddress)
}

// Releases an IP address back to Infoblox
func (iClient *IBloxClient) ReleaseAddr(netview, cidr, ipAddr string) {
	iClient.objMgr.ReleaseIP(netview, cidr, ipAddr, "")
}

// Returns the current Infoblox records
func (iClient *IBloxClient) GetRecords() *store.Store {
	st := store.NewStore()
	var zones []ibclient.ZoneAuth

	obj := ibclient.NewZoneAuth(ibclient.ZoneAuth{})
	err := iClient.conn.GetObject(obj, "", &zones)
	if err != nil {
		return nil
	}

	for _, zone := range zones {
		// Get A records
		var resA []ibclient.RecordA
		objA := ibclient.NewRecordA(
			ibclient.RecordA{
				Zone: zone.Fqdn,
			},
		)
		err = iClient.conn.GetObject(objA, "", &resA)
		if err != nil {
			return nil
		}
		for _, res := range resA {
			cidr := iClient.getNetworkCIDR(res.View, res.Ipv4Addr)
			st.AddRecord(res.Ipv4Addr, res.Name, "A", res.View, cidr)
		}

		// Get CNAME records
		var resC []ibclient.RecordCNAME
		objC := ibclient.NewRecordCNAME(
			ibclient.RecordCNAME{
				Zone: zone.Fqdn,
			},
		)
		err = iClient.conn.GetObject(objC, "", &resC)
		if err != nil {
			return nil
		}
		for _, res := range resC {
			var ipAddr string
			// Find the A record for this CNAME; get its IP address
			for _, aRec := range resA {
				if aRec.Name == res.Canonical {
					ipAddr = aRec.Ipv4Addr
				}
			}
			cidr := iClient.getNetworkCIDR(res.View, ipAddr)
			st.AddRecord(ipAddr, res.Name, "CNAME", res.View, cidr)
		}
	}
	return st
}

// Returns the network CIDR for an IP in a netview
func (iClient *IBloxClient) getNetworkCIDR(netview, ip string) string {
	var res []ibclient.Network
	var cidr string
	network := ibclient.NewNetwork(ibclient.Network{NetviewName: netview})
	iClient.conn.GetObject(network, "", &res)

	for _, n := range res {
		_, netCidr, _ := net.ParseCIDR(n.Cidr)
		addr := net.ParseIP(ip)
		if netCidr.Contains(addr) {
			cidr = n.Cidr
			break
		}
	}
	return cidr
}

// Extracts the IP address from a Fixed Address reference
func ipFromRef(ref string) string {
	split := strings.Split(ref, ":")
	ipAndNetview := split[len(split)-1]
	ip := strings.Split(ipAndNetview, "/")[0]
	return ip
}
