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

import "sort"

// Map of all IP addresses and their corresponding hostnames
type Store struct {
	Records map[string]hostSet
}

// Map of hosts (bool unused)
type hostSet map[string]bool

func NewStore() *Store {
	var st Store
	st.Records = make(map[string]hostSet)
	return &st
}

// Adds/Updates a record for an IP and hosts
func (st *Store) addRecord(ip string, hosts []string) {
	if _, found := st.Records[ip]; found {
		for _, host := range hosts {
			if _, ok := st.Records[ip][host]; !ok {
				st.Records[ip][host] = true
			}
		}
	} else {
		st.overwriteRecord(ip, hosts)
	}
}

// Overwrites the contents of a record
func (st *Store) overwriteRecord(ip string, hosts []string) {
	st.Records[ip] = make(hostSet)
	for _, host := range hosts {
		st.Records[ip][host] = true
	}
}

// Deletes a record for an IP address
func (st *Store) deleteRecord(ip string) {
	delete(st.Records, ip)
}

// Deletes a host from a record
func (st *Store) deleteHost(delHost string) {
	for _, hosts := range st.Records {
		if _, ok := hosts[delHost]; ok {
			delete(hosts, delHost)
			return
		}
	}
}

// Deletes hosts from records
func (st *Store) deleteHosts(delHosts []string) {
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
func (st *Store) getIP(host string) string {
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
