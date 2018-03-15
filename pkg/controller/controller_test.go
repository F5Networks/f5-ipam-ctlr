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
	"sort"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/F5Networks/f5-ipam-ctlr/pkg/manager"
	orch "github.com/F5Networks/f5-ipam-ctlr/pkg/orchestration"
	"github.com/F5Networks/f5-ipam-ctlr/pkg/store"
)

var net1 = "1.2.3.0/24"
var net2 = "5.6.7.0/24"

// Returns some sample IPGroups
func mockIPGroups() orch.IPGroup {
	return orch.IPGroup{
		GroupMutex: new(sync.Mutex),
		Groups: orch.IPGroups{
			orch.GroupKey{
				Name:    "A",
				Netview: "default",
				Cidr:    net1,
			}: []orch.Spec{
				{
					Kind:      "Ingress",
					Name:      "ing1",
					Namespace: "default",
					Hosts:     []string{"host1", "host2"},
				},
				{
					Kind:      "Ingress",
					Name:      "ing2",
					Namespace: "default",
					Hosts:     []string{"host3"},
				},
			},
			orch.GroupKey{
				Name:    "",
				Netview: "default",
				Cidr:    net2,
			}: []orch.Spec{
				{
					Kind:      "ConfigMap",
					Name:      "cfg1",
					Namespace: "default",
					Hosts:     []string{"host4"},
					Netview:   "default",
					Cidr:      net2,
				},
			},
		},
	}
}

// Mocks the orchestration client
type mockOClient struct {
	orch.Client
	oChan chan<- *orch.IPGroup
}

// Mock: calls writeIPGroups when Run
func (client *mockOClient) Run(stopCh <-chan struct{}) {
	go client.writeIPGroups()
}

// Mock: writes the sample IPGroups to the shared channel
func (client *mockOClient) writeIPGroups() {
	ipGroups := mockIPGroups()
	select {
	case client.oChan <- &ipGroups:
	case <-time.After(3 * time.Second):
	}
}

// Mock: not used
func (client *mockOClient) AnnotateResources(ip string, hosts []string) {}

// Mocks the manager client
type mockIClient struct {
	manager.Client

	net1addrs    []string
	net2addrs    []string
	nextAddr1Idx int
	nextAddr2Idx int
}

// Mock dummy functions to adhere to interface
func (client *mockIClient) CreateARecord(name, ipAddr, netview string) bool {
	return true
}
func (client *mockIClient) DeleteARecord(name, ipAddr, netview, cidr string)     {}
func (client *mockIClient) CreateCNAMERecord(name, canonical, netview string)    {}
func (client *mockIClient) DeleteCNAMERecord(name, ipAddr, netview, cidr string) {}

// Mock: returns the first available host to be the new A record
func (client *mockIClient) CNAMEToA(
	availHosts []string,
	delHost,
	ipAddr,
	netview,
	cidr string,
) (string, error) {
	return availHosts[0], nil
}

// Mock: returns the next available addr in the network
func (client *mockIClient) GetNextAddr(netview, cidr string) string {
	var addr string
	if cidr == net1 {
		addr = client.net1addrs[client.nextAddr1Idx]
		client.nextAddr1Idx++
	} else {
		addr = client.net2addrs[client.nextAddr2Idx]
		client.nextAddr2Idx++
	}
	return addr
}

// Mock: decrements the addr list index to "release" an IP
func (client *mockIClient) ReleaseAddr(netview, cidr, ipAddr string) {
	if cidr == net1 {
		client.nextAddr1Idx--
	} else {
		client.nextAddr2Idx--
	}
}

// Mock: returns the expected store records based on the above IPGroups
func (client *mockIClient) GetRecords() *store.Store {
	return &store.Store{
		Records: map[string]store.HostSet{
			"1.2.3.1": store.HostSet{
				"host1": A,
				"host2": CNAME,
				"host3": CNAME,
			},
			"5.6.7.8": store.HostSet{
				"host4": A,
			},
		},
		Netviews: map[string]string{
			"1.2.3.1": "default",
			"5.6.7.8": "default",
		},
		Cidrs: map[string]string{
			"1.2.3.1": net1,
			"5.6.7.8": net2,
		},
	}
}

var _ = Describe("Controller tests", func() {
	var oClient orch.Client
	var iClient manager.Client
	var oChan chan *orch.IPGroup

	BeforeEach(func() {
		oChan = make(chan *orch.IPGroup)
		oClient = &mockOClient{
			oChan: oChan,
		}
		iClient = &mockIClient{
			net1addrs:    []string{"1.2.3.1", "1.2.3.2"},
			nextAddr1Idx: 0,
			net2addrs:    []string{"5.6.7.8", "5.6.7.9"},
			nextAddr2Idx: 0,
		}
	})

	Context("class methods", func() {
		var ctlr *Controller
		var ipGroup orch.IPGroup
		BeforeEach(func() {
			ctlr = NewController(oClient, iClient, oChan, 30)
			Expect(ctlr).ToNot(BeNil())
			Expect(ctlr.store).ToNot(BeNil())
			ipGroup = mockIPGroups()
		})

		It("refreshes its store", func() {
			ctlr.refreshStore()
			Expect(ctlr.store).To(Equal(ctlr.iClient.GetRecords()))
		})

		It("processes IPGroups", func() {
			records := ctlr.iClient.GetRecords()
			// Add entries
			ctlr.processIPGroups(&ipGroup)
			Expect(ctlr.store).To(Equal(records))

			// Delete an entry
			key := orch.GroupKey{
				Name:    "",
				Netview: "default",
				Cidr:    net2,
			}
			delete(ipGroup.Groups, key)
			delete(records.Records, "5.6.7.8")
			delete(records.Netviews, "5.6.7.8")
			delete(records.Cidrs, "5.6.7.8")

			ctlr.processIPGroups(&ipGroup)
			Expect(ctlr.store).To(Equal(records))

			// Delete Ingress w/ A record (should be replaced)
			key = orch.GroupKey{
				Name:    "A",
				Netview: "default",
				Cidr:    net1,
			}
			specs := ipGroup.Groups[key]
			copy(specs[0:], specs[1:])
			specs[len(specs)-1] = orch.Spec{}
			ipGroup.Groups[key] = specs[:len(specs)-1]
			records.Records = map[string]store.HostSet{
				"1.2.3.1": store.HostSet{
					"host3": A,
				},
			}

			ctlr.processIPGroups(&ipGroup)
			Expect(ctlr.store).To(Equal(records))
		})

		It("listens to the orchestration and processes IPGroups", func() {
			stopCh := make(<-chan struct{})
			ctlr.Run(stopCh)
			Expect(ctlr.store).To(Equal(ctlr.iClient.GetRecords()))
		})
	})

	Context("util functions", func() {
		It("returns whether a slice contains a value", func() {
			s := []string{"one", "two", "three"}
			one := "one"
			five := "five"
			Expect(contains(s, one)).To(BeTrue())
			Expect(contains(s, five)).To(BeFalse())
		})

		It("returns a list of map keys", func() {
			m := map[string]string{
				"1": "one",
				"2": "two",
				"3": "three",
			}
			expList := []string{"1", "2", "3"}
			res := mapKeys(m)
			sort.Strings(res)
			Expect(res).To(Equal(expList))
		})

		It("removes an element from a slice", func() {
			s := []string{"one", "two", "three"}
			s = removeElement(s, "two")
			Expect(s).To(Equal([]string{"one", "three"}))
		})
	})
})
