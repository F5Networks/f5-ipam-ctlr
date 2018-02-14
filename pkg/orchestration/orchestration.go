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

import "reflect"

// Defines the interface the orchestration should implement
type Client interface {
	// Runs the client, watching for resources
	Run(stopCh <-chan struct{})
}

// Adds or updates a list of hosts that share an IP address (same groupName)
// Resources in the "" group will not share IP addresses
func (ipGrp *IPGroup) addToIPGroup(
	groupName,
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
	}
	if grp, found := ipGrp.Groups[groupName]; found {
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
			ipGrp.Groups[groupName][i].Hosts = spec.Hosts
		}
		if !haveSpec {
			ipGrp.Groups[groupName] = append(ipGrp.Groups[groupName], newSpec)
			updated = true
		}
	} else {
		ipGrp.Groups[groupName] = []Spec{newSpec}
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
	var deleted bool
	for name, grp := range ipGrp.Groups {
		for i, spec := range grp {
			if spec.equalsKey(key) {
				for j, host := range spec.Hosts {
					if host == rmHost {
						copy(spec.Hosts[j:], spec.Hosts[j+1:])
						spec.Hosts[len(spec.Hosts)-1] = ""
						ipGrp.Groups[name][i].Hosts = spec.Hosts[:len(spec.Hosts)-1]
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

// Returns a Spec for a resource
func (ipGrp *IPGroup) getSpec(key resourceKey) (bool, Spec, string) {
	for name, grp := range ipGrp.Groups {
		for _, spec := range grp {
			if spec.equalsKey(key) {
				return true, spec, name
			}
		}
	}
	return false, Spec{}, ""
}

// Returns the number of total hosts stored
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
