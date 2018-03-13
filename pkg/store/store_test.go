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

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const A = "A"
const CNAME = "CNAME"

var _ = Describe("Store tests", func() {
	Context("class methods", func() {
		var st *Store
		netview := "default"
		cidr := "1.2.3.0/24"
		BeforeEach(func() {
			st = NewStore()
			Expect(st).ToNot(BeNil())
		})

		It("adds records", func() {
			st.AddRecord("1.2.3.4", "bar.com", A, netview, cidr)
			record := st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"bar.com"}))

			st.AddRecord("1.2.3.4", "baz.com", CNAME, netview, cidr)
			record = st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"bar.com", "baz.com"}))

			st.AddRecord("5.6.7.8", "qux.com", A, netview, cidr)
			record = st.getHosts("5.6.7.8")
			Expect(record).To(Equal([]string{"qux.com"}))
			Expect(len(st.Records)).To(Equal(2))

			// Try adding a record again; shouldn't be added twice
			st.AddRecord("5.6.7.8", "qux.com", A, netview, cidr)
			record = st.getHosts("5.6.7.8")
			Expect(record).To(Equal([]string{"qux.com"}))
		})

		It("overwrites records", func() {
			st.AddRecord("1.2.3.4", "foo.com", A, netview, cidr)
			record := st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"foo.com"}))

			st.overwriteRecord("1.2.3.4", "bar.com", A, netview, cidr)
			record = st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"bar.com"}))
		})

		It("deletes records", func() {
			st.AddRecord("1.2.3.4", "foo.com", CNAME, netview, cidr)
			record := st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"foo.com"}))

			st.deleteRecord("1.2.3.4")
			record = st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{}))
			Expect(len(st.Records)).To(Equal(0))
			Expect(st.Records["1.2.3.4"]).To(BeNil())
		})

		It("deletes hosts from records", func() {
			st.AddRecord("1.2.3.4", "bar.com", A, netview, cidr)
			st.AddRecord("1.2.3.4", "baz.com", CNAME, netview, cidr)
			st.AddRecord("1.2.3.4", "foo.com", CNAME, netview, cidr)
			st.AddRecord("1.2.3.4", "qux.com", CNAME, netview, cidr)
			record := st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"bar.com", "baz.com", "foo.com", "qux.com"}))

			st.deleteHost("bar.com")
			record = st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"baz.com", "foo.com", "qux.com"}))

			st.DeleteHosts([]string{"baz.com", "foo.com"})
			record = st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"qux.com"}))

			st.DeleteHosts([]string{"qux.com"})
			record = st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{}))
		})

		It("gets an IP address for a host", func() {
			st.AddRecord("1.2.3.4", "foo.com", A, netview, cidr)
			st.AddRecord("5.6.7.8", "bar.com", CNAME, netview, cidr)
			ip := st.GetIP("foo.com")
			Expect(ip).To(Equal("1.2.3.4"))
			ip = st.GetIP("bar.com")
			Expect(ip).To(Equal("5.6.7.8"))
			ip = st.GetIP("fake.com")
			Expect(ip).To(Equal(""))
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
	})
})
