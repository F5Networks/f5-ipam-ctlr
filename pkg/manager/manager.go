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

import store "github.com/F5Networks/f5-ipam-ctlr/pkg/store"

// Client defines the interface that the IPAM system should implement
type Client interface {
	// Creates an A record
	CreateARecord(name, ipAddr, netview string) bool
	// Deletes an A record and releases the IP address
	DeleteARecord(name, ipAddr, netview, cidr string)
	// Creates a CNAME record
	CreateCNAMERecord(name, canonical, netview string)
	// Deletes a CNAME record
	DeleteCNAMERecord(name, ipAddr, netview, cidr string)
	// Converts a CNAME to an A record, and updates all following CNAME references
	CNAMEToA(availHosts []string, delHost, ipAddr, netview, cidr string) (string, error)
	// Gets and reserves the next available IP address
	GetNextAddr(netview, cidr string) string
	// Releases an IP address
	ReleaseAddr(netview, cidr, ipAddr string)
	// Returns the current IPAM system records
	GetRecords() *store.Store
}
