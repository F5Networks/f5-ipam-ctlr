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
	"encoding/json"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	ibclient "github.com/infobloxopen/infoblox-go-client"
)

const FQDN = ".com"

type mockIBConnector struct {
	aRecords     []aRecord
	cnameRecords []cnameRecord
	network      ibclient.Network
	zone         ibclient.ZoneAuth
	refCount     int
}

type aRecord struct {
	obj ibclient.RecordA
	ref string
}

type cnameRecord struct {
	obj ibclient.RecordCNAME
	ref string
}

// Mock: creates an Infoblox object
func (client *mockIBConnector) CreateObject(
	obj ibclient.IBObject,
) (ref string, err error) {
	var a *ibclient.RecordA
	var c *ibclient.RecordCNAME
	var n *ibclient.Network
	var z *ibclient.ZoneAuth

	refC := strconv.Itoa(client.refCount)
	switch obj.(type) {
	case *ibclient.RecordA:
		a = obj.(*ibclient.RecordA)
		a.Ref = refC
		a.Zone = FQDN
	case *ibclient.RecordCNAME:
		c = obj.(*ibclient.RecordCNAME)
		c.Ref = refC
		c.Zone = FQDN
	case *ibclient.Network:
		n = obj.(*ibclient.Network)
		n.Ref = refC
	case *ibclient.ZoneAuth:
		z = obj.(*ibclient.ZoneAuth)
		z.Ref = refC
	}

	if a != nil {
		newObj := aRecord{
			obj: *a,
			ref: refC,
		}
		client.aRecords = append(client.aRecords, newObj)
	} else if c != nil {
		newObj := cnameRecord{
			obj: *c,
			ref: refC,
		}
		client.cnameRecords = append(client.cnameRecords, newObj)
	} else if n != nil {
		client.network = *n
	} else {
		client.zone = *z
	}

	client.refCount++
	return refC, nil
}

// Mock: gets an Infoblox object
func (client *mockIBConnector) GetObject(
	obj ibclient.IBObject,
	ref string,
	res interface{},
) (err error) {
	var aList []ibclient.RecordA
	var cList []ibclient.RecordCNAME
	var nList []ibclient.Network
	var zList []ibclient.ZoneAuth
	var objName, fqdn string
	var aRec, cRec bool

	switch o := obj.(type) {
	case *ibclient.RecordA:
		objName = o.Name
		fqdn = o.Zone
		aRec = true
	case *ibclient.RecordCNAME:
		objName = o.Name
		fqdn = o.Zone
		cRec = true
	case *ibclient.Network:
		objName = o.NetviewName
	case *ibclient.ZoneAuth:
		objName = "zone"
	}

	for _, a := range client.aRecords {
		// If names are equal, or name is empty but zones are equal, return this object
		if (objName == a.obj.Name || (objName == "" && fqdn == a.obj.Zone)) && aRec {
			aList = append(aList, a.obj)
			listJSON, _ := json.Marshal(aList)
			json.Unmarshal(listJSON, res)
		}
	}
	for _, c := range client.cnameRecords {
		// If names are equal, or name is empty but zones are equal, return this object
		if (objName == c.obj.Name || (objName == "" && fqdn == c.obj.Zone)) && cRec {
			cList = append(cList, c.obj)
			listJSON, _ := json.Marshal(cList)
			json.Unmarshal(listJSON, res)
		}
	}
	if objName == client.network.NetviewName {
		nList = append(nList, client.network)
		listJSON, _ := json.Marshal(nList)
		json.Unmarshal(listJSON, res)
	}
	if objName == "zone" {
		zList = append(zList, client.zone)
		listJSON, _ := json.Marshal(zList)
		json.Unmarshal(listJSON, res)
	}
	return nil
}

// Mock: deletes an Infoblox object
func (client *mockIBConnector) DeleteObject(
	ref string,
) (refRes string, err error) {
	for i, obj := range client.aRecords {
		if obj.ref == ref {
			copy(client.aRecords[i:], client.aRecords[i+1:])
			client.aRecords[len(client.aRecords)-1] = aRecord{}
			client.aRecords = client.aRecords[:len(client.aRecords)-1]
			return "", nil
		}
	}
	for i, obj := range client.cnameRecords {
		if obj.ref == ref {
			copy(client.cnameRecords[i:], client.cnameRecords[i+1:])
			client.cnameRecords[len(client.cnameRecords)-1] = cnameRecord{}
			client.cnameRecords = client.cnameRecords[:len(client.cnameRecords)-1]
			return "", nil
		}
	}
	return "", nil
}

// Mock: updates an Infoblox object
func (client *mockIBConnector) UpdateObject(
	obj ibclient.IBObject,
	ref string,
) (refRes string, err error) {
	for i, a := range client.aRecords {
		if a.ref == ref {
			netview := client.aRecords[i].obj.View
			client.aRecords[i].obj = *obj.(*ibclient.RecordA)
			client.aRecords[i].obj.View = netview
			return ref, nil
		}
	}
	for i, c := range client.cnameRecords {
		if c.ref == ref {
			client.cnameRecords[i].obj = *obj.(*ibclient.RecordCNAME)
			return ref, nil
		}
	}
	return ref, nil
}

type mockObjectManager struct{}

// Mock: not used
func (objMgr *mockObjectManager) AllocateIP(
	netview,
	cidr,
	ipAddr,
	macAddress,
	vmID string,
) (*ibclient.FixedAddress, error) {
	return nil, nil
}

// Mock: not used
func (objMgr *mockObjectManager) ReleaseIP(
	netview,
	cidr,
	ipAddr,
	macAddr string,
) (string, error) {
	return "", nil
}

func newIBloxClient() *IBloxClient {
	return &IBloxClient{
		conn:      &mockIBConnector{},
		objMgr:    &mockObjectManager{},
		recordRef: make(recordRef),
	}
}

var _ = Describe("Infoblox tests", func() {
	var iClient *IBloxClient
	netview := "default"
	cidr := "1.2.3.0/24"
	BeforeEach(func() {
		iClient = newIBloxClient()
		network := ibclient.NewNetwork(
			ibclient.Network{NetviewName: netview, Cidr: cidr})
		ref, _ := iClient.conn.CreateObject(network)
		Expect(ref).To(Equal("0"))
		zone := ibclient.NewZoneAuth(
			ibclient.ZoneAuth{Fqdn: FQDN})
		ref, _ = iClient.conn.CreateObject(zone)
		Expect(ref).To(Equal("1"))
	})

	It("creates and deletes A records", func() {
		host := "foo.com"
		ip := "1.2.3.4"
		expRef := "2"
		// Add a record
		iClient.CreateARecord(host, ip, netview)
		Expect(len(iClient.conn.(*mockIBConnector).aRecords)).To(Equal(1))
		Expect(iClient.recordRef[host]).To(Equal(expRef))

		// Try to create again, should not change ref
		iClient.CreateARecord(host, ip, netview)
		Expect(len(iClient.conn.(*mockIBConnector).aRecords)).To(Equal(1))
		Expect(iClient.recordRef[host]).To(Equal(expRef))

		// Delete the record
		iClient.DeleteARecord(host, ip, netview, cidr)
		Expect(len(iClient.conn.(*mockIBConnector).aRecords)).To(Equal(0))
		Expect(iClient.recordRef[host]).To(Equal(""))
	})

	It("creates and deletes CNAME records", func() {
		host := "bar.com"
		canonical := "foo.com"
		ip := "1.2.3.4"
		expRef := "2"
		// Add a record
		iClient.CreateCNAMERecord(host, canonical, netview)
		Expect(len(iClient.conn.(*mockIBConnector).cnameRecords)).To(Equal(1))
		Expect(iClient.recordRef[host]).To(Equal(expRef))

		// Try to create again, should not change ref
		iClient.CreateCNAMERecord(host, canonical, netview)
		Expect(len(iClient.conn.(*mockIBConnector).cnameRecords)).To(Equal(1))
		Expect(iClient.recordRef[host]).To(Equal(expRef))

		// Delete the record
		iClient.DeleteCNAMERecord(host, ip, netview, cidr)
		Expect(len(iClient.conn.(*mockIBConnector).cnameRecords)).To(Equal(0))
		Expect(iClient.recordRef[host]).To(Equal(""))

		// Try to delete again, nothing should change
		iClient.DeleteCNAMERecord(host, ip, netview, cidr)
		Expect(len(iClient.conn.(*mockIBConnector).cnameRecords)).To(Equal(0))
		Expect(iClient.recordRef[host]).To(Equal(""))
	})

	It("converts a CNAME record to an A record", func() {
		availHosts := []string{"foo.com", "bar.com", "baz.com"}
		delHost := "foobar.com"
		ip := "1.2.3.4"
		// Create our records first
		iClient.CreateARecord(delHost, ip, netview)
		iClient.CreateCNAMERecord(availHosts[0], delHost, netview)
		iClient.CreateCNAMERecord(availHosts[1], delHost, netview)
		iClient.CreateCNAMERecord(availHosts[2], delHost, netview)

		Expect(len(iClient.conn.(*mockIBConnector).cnameRecords)).To(Equal(3))
		aRecord, err := iClient.CNAMEToA(availHosts, delHost, ip, netview, cidr)
		Expect(err).To(BeNil())
		Expect(aRecord).To(Equal(availHosts[0]))
		Expect(len(iClient.conn.(*mockIBConnector).cnameRecords)).To(Equal(2))

		// Verify A record
		var resA []ibclient.RecordA
		a := ibclient.NewRecordA(
			ibclient.RecordA{
				Name: delHost,
			},
		)
		_ = iClient.conn.GetObject(a, "", &resA)
		Expect(resA).To(BeNil())

		a = ibclient.NewRecordA(
			ibclient.RecordA{
				Name: aRecord,
			},
		)
		_ = iClient.conn.GetObject(a, "", &resA)
		Expect(resA).ToNot(BeNil())
		Expect(resA[0].Ipv4Addr).To(Equal(ip))

		// Verify CNAME records
		var resCNAME []ibclient.RecordCNAME
		cname := ibclient.NewRecordCNAME(
			ibclient.RecordCNAME{
				Name: aRecord,
			},
		)
		_ = iClient.conn.GetObject(cname, "", &resCNAME)
		Expect(resCNAME).To(BeNil())

		cname = ibclient.NewRecordCNAME(
			ibclient.RecordCNAME{
				Name: availHosts[1],
			},
		)
		_ = iClient.conn.GetObject(cname, "", &resCNAME)
		Expect(resCNAME).ToNot(BeNil())
		Expect(resCNAME[0].Canonical).To(Equal(aRecord))

		cname = ibclient.NewRecordCNAME(
			ibclient.RecordCNAME{
				Name: availHosts[2],
			},
		)
		_ = iClient.conn.GetObject(cname, "", &resCNAME)
		Expect(resCNAME).ToNot(BeNil())
		Expect(resCNAME[0].Canonical).To(Equal(aRecord))
	})

	It("gets the current Infoblox records", func() {
		a := "foobar.com"
		ip := "1.2.3.4"
		// Create records
		iClient.CreateARecord(a, ip, netview)
		iClient.CreateCNAMERecord("foo.com", a, netview)
		iClient.CreateCNAMERecord("bar.com", a, netview)
		Expect(len(iClient.conn.(*mockIBConnector).aRecords)).To(Equal(1))
		Expect(len(iClient.conn.(*mockIBConnector).cnameRecords)).To(Equal(2))

		// Get records
		st := iClient.GetRecords()
		Expect(len(st.Records)).To(Equal(1))
		Expect(len(st.Netviews)).To(Equal(1))
		Expect(len(st.Cidrs)).To(Equal(1))

		hosts, ok := st.Records[ip]
		Expect(ok).To(BeTrue())
		aRec, ok := hosts[a]
		Expect(ok).To(BeTrue())
		Expect(aRec).To(Equal("A"))

		cname, ok := hosts["foo.com"]
		Expect(ok).To(BeTrue())
		Expect(cname).To(Equal("CNAME"))
		cname, ok = hosts["bar.com"]
		Expect(ok).To(BeTrue())
		Expect(cname).To(Equal("CNAME"))
	})

	It("returns the network cidr for an IP address", func() {
		ip := "1.2.3.4"
		testCidr := iClient.getNetworkCIDR(netview, ip)
		Expect(testCidr).To(Equal(cidr))
		ip = "5.6.7.8"
		testCidr = iClient.getNetworkCIDR(netview, ip)
		Expect(testCidr).To(Equal(""))
	})

	Context("util functions", func() {
		It("gets an IP address from an Infoblox ref string", func() {
			ref := "ZG5zLmhvc3QkLl9kZWZhdWx0Lm9yZy5naC5mb28:1.2.3.4/default"
			ip := ipFromRef(ref)
			Expect(ip).To(Equal("1.2.3.4"))
		})
	})
})
