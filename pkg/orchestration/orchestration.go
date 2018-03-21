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

package orchestration

import (
	"reflect"
	"sort"
)

// Client defines the interface the orchestration should implement
type Client interface {
	// Runs the client, watching for resources
	Run(stopCh <-chan struct{})
	// Writes the IPGroups to the Controller
	writeIPGroups()
	// Annotates resources that contain given hosts with the given IP address
	AnnotateResources(ip string, hosts []string)
}

// Adds or updates a list of hosts that share an IP address (same groupName)
// Resources in the "" group will not share IP addresses
func (ipGrp *IPGroup) addToIPGroup(
	gKey GroupKey,
	kind,
	rsName,
	namespace string,
	hosts []string,
) bool {
	ipGrp.GroupMutex.Lock()
	defer ipGrp.GroupMutex.Unlock()

	var updated bool
	newSpec := Spec{
		Kind:      kind,
		Name:      rsName,
		Namespace: namespace,
		Hosts:     hosts,
		Netview:   gKey.Netview,
		Cidr:      gKey.Cidr,
	}
	if grp, found := ipGrp.Groups[gKey]; found {
		var haveSpec bool
		for i, spec := range grp {
			if spec.equals(newSpec) {
				haveSpec = true
				for _, host := range hosts {
					if !contains(spec.Hosts, host) {
						spec.Hosts = append(spec.Hosts, host)
						updated = true
					}
				}
			}
			ipGrp.Groups[gKey][i].Hosts = spec.Hosts
		}
		if !haveSpec {
			ipGrp.Groups[gKey] = append(ipGrp.Groups[gKey], newSpec)
			updated = true
		}
	} else {
		ipGrp.Groups[gKey] = []Spec{newSpec}
		updated = true
	}
	return updated
}

// Removes a resource from its IPGroup
func (ipGrp *IPGroup) removeFromIPGroup(rmSpec Spec) {
	ipGrp.GroupMutex.Lock()
	defer ipGrp.GroupMutex.Unlock()

	var deleted bool
	for grpName, grp := range ipGrp.Groups {
		for i, spec := range grp {
			if spec.equals(rmSpec) {
				copy(grp[i:], grp[i+1:])
				grp[len(grp)-1] = Spec{}
				ipGrp.Groups[grpName] = grp[:len(grp)-1]
				deleted = true
				break
			}
		}
		// If last entry is nil, delete the group
		if len(grp) == 1 && reflect.DeepEqual(grp[0], Spec{}) {
			delete(ipGrp.Groups, grpName)
		}
		if deleted {
			break
		}
	}
}

// Removes a host from its Spec
func (ipGrp *IPGroup) removeHost(rmHost string, key resourceKey) {
	ipGrp.GroupMutex.Lock()
	defer ipGrp.GroupMutex.Unlock()

	var deleted bool
	for gKey, grp := range ipGrp.Groups {
		for i, spec := range grp {
			if spec.equalsKey(key) {
				for j, host := range spec.Hosts {
					if host == rmHost {
						spec.Hosts = append(spec.Hosts[:j], spec.Hosts[j+1:]...)
						ipGrp.Groups[gKey][i].Hosts = spec.Hosts
						deleted = true
						break
					}
				}
			}
			if deleted {
				break
			}
		}
	}
}

// Returns all of the Specs that contain hosts matching those passed in
func (ipGrp *IPGroup) getSpecsWithHosts(hosts []string) []Spec {
	ipGrp.GroupMutex.Lock()
	defer ipGrp.GroupMutex.Unlock()

	var specs []Spec
	for _, grp := range ipGrp.Groups {
		for _, spec := range grp {
			// If the first host in the spec is in hosts, then
			// we can be certain that all spec.Hosts are in hosts
			if contains(hosts, spec.Hosts[0]) {
				specs = append(specs, spec)
			}
		}
	}
	return specs
}

// GetAllHosts returns all hosts across IPGroups
func (ipGrp *IPGroup) GetAllHosts() []string {
	var hosts []string
	for _, grp := range ipGrp.Groups {
		for _, spec := range grp {
			hosts = append(hosts, spec.Hosts...)
		}
	}
	sort.Strings(hosts)
	return hosts
}

// Returns a Spec for a resource
func (ipGrp *IPGroup) getSpec(key resourceKey) (bool, Spec, string) {
	for gKey, grp := range ipGrp.Groups {
		for _, spec := range grp {
			if spec.equalsKey(key) {
				return true, spec, gKey.Name
			}
		}
	}
	return false, Spec{}, ""
}

// NumHosts returns the number of total hosts stored
func (ipGrp *IPGroup) NumHosts() int {
	sum := 0
	for _, grp := range ipGrp.Groups {
		for _, spec := range grp {
			sum += len(spec.Hosts)
		}
	}
	return sum
}

// Returns whether a Spec corresponds to a resourceKey
func (spec *Spec) equalsKey(key resourceKey) bool {
	if spec.Kind == key.Kind && spec.Name == key.Name && spec.Namespace == key.Namespace {
		return true
	}
	return false
}

// Returns whether a Spec equals another (minus hosts)
func (spec *Spec) equals(newSpec Spec) bool {
	if spec.Kind == newSpec.Kind &&
		spec.Name == newSpec.Name &&
		spec.Namespace == newSpec.Namespace {
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
