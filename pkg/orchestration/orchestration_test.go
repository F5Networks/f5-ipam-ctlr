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
	"sync"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Orchestration tests", func() {
	It("correctly configures IPGroups", func() {
		ipGroup := IPGroup{Groups: make(IPGroups), GroupMutex: new(sync.Mutex)}
		ns := "default"
		netview := "default"
		cidr := "1.2.3.0/24"
		groupA := GroupKey{
			Name:    "A",
			Netview: netview,
			Cidr:    cidr,
		}
		groupB := GroupKey{
			Name:    "B",
			Netview: netview,
			Cidr:    cidr,
		}
		// Add an entry to group A
		ipGroup.addToIPGroup(groupA, "Ingress", "ing1", ns, []string{"foo.com", "bar.com"})
		spec1 := Spec{
			Kind:      "Ingress",
			Name:      "ing1",
			Namespace: ns,
			Hosts:     []string{"foo.com", "bar.com"},
			Netview:   netview,
			Cidr:      cidr,
		}
		specs, found := ipGroup.Groups[groupA]
		Expect(found).To(BeTrue())
		Expect(specs).To(Equal([]Spec{spec1}))
		Expect(ipGroup.NumHosts()).To(Equal(2))

		// Add a second entry to group A
		ipGroup.addToIPGroup(groupA, "Ingress", "ing2", ns, []string{"baz.com"})
		spec2 := Spec{
			Kind:      "Ingress",
			Name:      "ing2",
			Namespace: ns,
			Hosts:     []string{"baz.com"},
			Netview:   netview,
			Cidr:      cidr,
		}
		specs, found = ipGroup.Groups[groupA]
		Expect(found).To(BeTrue())
		Expect(specs).To(Equal([]Spec{spec1, spec2}))
		Expect(ipGroup.NumHosts()).To(Equal(3))

		// Add an entry to group B
		ipGroup.addToIPGroup(groupB, "Ingress", "ing3", ns, []string{"qux.com"})
		spec3 := Spec{
			Kind:      "Ingress",
			Name:      "ing3",
			Namespace: ns,
			Hosts:     []string{"qux.com"},
			Netview:   netview,
			Cidr:      cidr,
		}
		specs, found = ipGroup.Groups[groupB]
		Expect(found).To(BeTrue())
		Expect(specs).To(Equal([]Spec{spec3}))
		Expect(ipGroup.NumHosts()).To(Equal(4))

		// Re-add with new host
		ipGroup.addToIPGroup(groupB, "Ingress", "ing3", ns, []string{"qux.com", "foobar.com"})
		spec3 = Spec{
			Kind:      "Ingress",
			Name:      "ing3",
			Namespace: ns,
			Hosts:     []string{"qux.com", "foobar.com"},
			Netview:   netview,
			Cidr:      cidr,
		}
		specs, found = ipGroup.Groups[groupB]
		Expect(found).To(BeTrue())
		Expect(specs).To(Equal([]Spec{spec3}))
		Expect(ipGroup.NumHosts()).To(Equal(5))
		Expect(ipGroup.GetAllHosts()).To(Equal(
			[]string{"bar.com", "baz.com", "foo.com", "foobar.com", "qux.com"}))

		// Remove host
		ipGroup.removeHost("qux.com", resourceKey{
			Kind:      "Ingress",
			Name:      "ing3",
			Namespace: ns,
		})
		specs, _ = ipGroup.Groups[groupB]
		spec3.Hosts = []string{"foobar.com"}
		Expect(specs).To(Equal([]Spec{spec3}))

		// Remove a resource from group B
		ipGroup.removeFromIPGroup(spec3)
		_, found = ipGroup.Groups[groupB]
		Expect(found).To(BeFalse())

		// Remove a resource from group A
		ipGroup.removeFromIPGroup(spec2)
		specs, found = ipGroup.Groups[groupA]
		Expect(found).To(BeTrue())
		Expect(specs).To(Equal([]Spec{spec1}))
	})
})
