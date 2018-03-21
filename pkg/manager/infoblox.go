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
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	log "github.com/F5Networks/f5-ipam-ctlr/pkg/vlogger"

	store "github.com/F5Networks/f5-ipam-ctlr/pkg/store"

	ibclient "github.com/infobloxopen/infoblox-go-client"
)

const EAKey = "F5-IPAM"
const EAVal = "managed"

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
	ea        ibclient.EA
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
	requestBuilder := &requestBuilder{}
	requestor := &ibclient.WapiHttpRequestor{}
	conn, err := ibclient.NewConnector(hostConfig, transportConfig, requestBuilder, requestor)
	if err != nil {
		return nil, err
	}
	objMgr := ibclient.NewObjectManager(conn, cmpType, "0")

	// Create an Extensible Attribute for resource tracking
	if eaDef, _ := objMgr.GetEADefinition(EAKey); eaDef == nil {
		eaDef := ibclient.EADefinition{
			Name:    EAKey,
			Type:    "STRING",
			Comment: "Managed by the F5 IPAM Controller",
		}
		_, err = objMgr.CreateEADefinition(eaDef)
		if err != nil {
			return nil, err
		}
	}

	iClient := &IBloxClient{
		conn:      conn,
		objMgr:    objMgr,
		recordRef: make(recordRef),
		ea:        ibclient.EA{EAKey: EAVal},
	}

	return iClient, nil
}

// Creates an A record
func (iClient *IBloxClient) CreateARecord(name, ipAddr, netview string) bool {
	obj := ibclient.NewRecordA(
		ibclient.RecordA{
			Name:     name,
			Ipv4Addr: ipAddr,
			View:     netview,
			Ea:       iClient.ea,
		},
	)
	res, err := iClient.getARecord(obj)
	// If error or record already exists, return
	if err != nil || (res != nil && len(res) > 0) {
		return false
	}

	log.Infof("Creating A record for '%s' as '%s'.", name, ipAddr)
	ref, _ := iClient.conn.CreateObject(obj)
	iClient.recordRef[name] = ref
	return true
}

// Deletes an A record and releases the IP address
func (iClient *IBloxClient) DeleteARecord(name, ipAddr, netview, cidr string) {
	obj := ibclient.NewRecordA(
		ibclient.RecordA{
			Name:     name,
			Ipv4Addr: ipAddr,
			View:     netview,
			Ea:       iClient.ea,
		},
	)
	res, err := iClient.getARecord(obj)
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
	obj := ibclient.NewRecordCNAME(
		ibclient.RecordCNAME{
			Name:      name,
			Canonical: canonical,
			View:      netview,
			Ea:        iClient.ea,
		},
	)
	res, err := iClient.getCNAMERecord(obj)
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
	obj := ibclient.NewRecordCNAME(
		ibclient.RecordCNAME{
			Name: name,
			View: netview,
			Ea:   iClient.ea,
		},
	)
	res, err := iClient.getCNAMERecord(obj)
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
					Ea:       iClient.ea,
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
					Ea:        iClient.ea,
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
	var err error

	obj := ibclient.NewZoneAuth(ibclient.ZoneAuth{})
	err = iClient.conn.GetObject(obj, "", &zones)
	if err != nil {
		return st
	}

	for _, zone := range zones {
		// Get A records
		var resA []ibclient.RecordA
		objA := ibclient.NewRecordA(
			ibclient.RecordA{
				Zone: zone.Fqdn,
			},
		)
		resA, err = iClient.getARecord(objA)
		if err != nil {
			return st
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
		resC, err = iClient.getCNAMERecord(objC)
		if err != nil {
			return st
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

// FIXME (berman):
// The following methods call GetObject on a specific Record object. However, since we
// had to override infoblox-go-client's API request URL, we get back all objects with
// our configured Extensible Attribute, rather than just the single object. The functions
// will loop through all returned objects until we find the matching object.

// Returns an A record from Infoblox
func (iClient *IBloxClient) getARecord(
	obj *ibclient.RecordA,
) ([]ibclient.RecordA, error) {
	var res, ret []ibclient.RecordA
	err := iClient.conn.GetObject(obj, "", &res)
	if err != nil {
		return nil, err
	}
	for _, o := range res {
		if o.Name == obj.Name && o.Ipv4Addr == obj.Ipv4Addr && o.View == obj.View {
			ret = append(ret, o)
		} else if obj.Name == "" && o.Zone == obj.Zone {
			ret = append(ret, o)
		}
	}
	return ret, nil
}

// Returns a CNAME record from Infoblox
func (iClient *IBloxClient) getCNAMERecord(
	obj *ibclient.RecordCNAME,
) ([]ibclient.RecordCNAME, error) {
	var res, ret []ibclient.RecordCNAME
	err := iClient.conn.GetObject(obj, "", &res)
	if err != nil {
		return nil, err
	}

	for _, o := range res {
		if o.Name == obj.Name && o.View == obj.View {
			ret = append(ret, o)
		} else if obj.Name == "" && o.Zone == obj.Zone {
			ret = append(ret, o)
		}
	}
	return ret, nil
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

// FIXME (berman):
// infoblox-go-client does not properly create the URL string when we need
// to request a resource that has an extattr. This code is here to format the URL
// how we need it to be formatted. We also needed to override BuildRequest from infoblox-go-client
// in order to ensure that OUR BuildUrl function is called, rather than the
// WapiRequestBuilder.BuildUrl that would be called if we were to use the go-client's method

// Wrapper around WapiRequestBuilder
type requestBuilder struct {
	*ibclient.WapiRequestBuilder
	HostConfig ibclient.HostConfig
}

func (rb *requestBuilder) Init(cfg ibclient.HostConfig) {
	rb.HostConfig = cfg
}

// Override function to pass in Extensible Attribute to request
func (rb *requestBuilder) BuildUrl(
	r ibclient.RequestType,
	objType,
	ref string,
	returnFields []string,
) string {
	aRec := "record:a"
	cnameRec := "record:cname"

	path := []string{"wapi", "v" + rb.HostConfig.Version}
	if len(ref) > 0 {
		path = append(path, ref)
	} else {
		path = append(path, objType)
	}

	qry := ""
	vals := url.Values{}
	if r == ibclient.GET {
		if objType == aRec || objType == cnameRec {
			vals.Set("*"+EAKey+"~", EAVal)
		}
		if len(returnFields) > 0 {
			vals.Set("_return_fields", strings.Join(returnFields, ","))
		}
		qry = vals.Encode()
	}

	u := url.URL{
		Scheme:   "https",
		Host:     rb.HostConfig.Host + ":" + rb.HostConfig.Port,
		Path:     strings.Join(path, "/"),
		RawQuery: qry,
	}
	return u.String()
}

// Override function to call our version of BuildUrl
func (rb *requestBuilder) BuildRequest(
	r ibclient.RequestType,
	obj ibclient.IBObject,
	ref string,
) (*http.Request, error) {
	var objType string
	var returnFields []string
	if obj != nil {
		objType = obj.ObjectType()
		returnFields = obj.ReturnFields()
	}
	url := rb.BuildUrl(r, objType, ref, returnFields)

	var body []byte
	if obj != nil {
		// Uses ibclient.WapiRequestBuilder method
		body = rb.BuildBody(r, obj)
	}

	req, err := http.NewRequest(toString(r), url, bytes.NewBuffer(body))
	if err != nil {
		log.Errorf("Error building HTTP request: %v", err)
		return req, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(rb.HostConfig.Username, rb.HostConfig.Password)

	return req, nil
}

// Converts an ibclient.RequestType to its string representation
func toString(r ibclient.RequestType) string {
	switch r {
	case ibclient.CREATE:
		return "POST"
	case ibclient.GET:
		return "GET"
	case ibclient.DELETE:
		return "DELETE"
	case ibclient.UPDATE:
		return "PUT"
	}

	return ""
}
