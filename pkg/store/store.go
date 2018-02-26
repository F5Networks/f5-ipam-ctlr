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

package store

import "sort"

// Map of all IP addresses and their corresponding hostnames, netviews, and CIDRs
type Store struct {
	Records  map[string]hostSet
	Netviews map[string]string
	Cidrs    map[string]string
}

// Map of hosts (value is record type)
type hostSet map[string]string

func NewStore() *Store {
	var st Store
	st.Records = make(map[string]hostSet)
	st.Netviews = make(map[string]string)
	st.Cidrs = make(map[string]string)
	return &st
}

// Adds/Updates a record for an IP and hosts
func (st *Store) AddRecord(ip, host, recordType, netview, cidr string) {
	if _, found := st.Records[ip]; found {
		if _, ok := st.Records[ip][host]; !ok {
			st.Records[ip][host] = recordType
			st.Netviews[ip] = netview
			st.Cidrs[ip] = cidr
		}
	} else {
		st.overwriteRecord(ip, host, recordType, netview, cidr)
	}
}

// Overwrites the contents of a record
func (st *Store) overwriteRecord(ip, host, recordType, netview, cidr string) {
	st.Records[ip] = make(hostSet)
	st.Records[ip][host] = recordType
	st.Netviews[ip] = netview
	st.Cidrs[ip] = cidr
}

// Deletes a record for an IP address
func (st *Store) deleteRecord(ip string) {
	delete(st.Records, ip)
	delete(st.Netviews, ip)
	delete(st.Cidrs, ip)
}

// Deletes a host from a record
func (st *Store) deleteHost(delHost string) {
	for ip, hosts := range st.Records {
		if _, ok := hosts[delHost]; ok {
			delete(hosts, delHost)
			if len(hosts) == 0 {
				st.deleteRecord(ip)
			}
			return
		}
	}
}

// Deletes hosts from records
func (st *Store) DeleteHosts(delHosts []string) {
	for _, delHost := range delHosts {
		st.deleteHost(delHost)
	}
}

// Returns the hosts for a given IP address
func (st *Store) getHosts(ip string) []string {
	var hosts []string
	if _, found := st.Records[ip]; found {
		for host, _ := range st.Records[ip] {
			hosts = append(hosts, host)
		}
		sort.Strings(hosts)
		return hosts
	} else {
		return []string{}
	}
}

// Returns the IP address for a given host
func (st *Store) GetIP(host string) string {
	for ip, hosts := range st.Records {
		if _, ok := hosts[host]; ok {
			return ip
		}
	}
	return ""
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
