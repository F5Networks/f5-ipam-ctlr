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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Store tests", func() {
	It("creates a store", func() {
		st := NewStore()
		Expect(st).ToNot(BeNil())
	})

	Context("class methods", func() {
		var st *Store
		BeforeEach(func() {
			st = NewStore()
			Expect(st).ToNot(BeNil())
		})

		It("adds records", func() {
			st.addRecord("1.2.3.4", []string{"bar.com", "foo.com"})
			record := st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"bar.com", "foo.com"}))

			st.addRecord("1.2.3.4", []string{"baz.com"})
			record = st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"bar.com", "baz.com", "foo.com"}))

			st.addRecord("5.6.7.8", []string{"qux.com"})
			record = st.getHosts("5.6.7.8")
			Expect(record).To(Equal([]string{"qux.com"}))
			Expect(len(st.Records)).To(Equal(2))

			// Try adding a record again; shouldn't be added twice
			st.addRecord("5.6.7.8", []string{"qux.com"})
			record = st.getHosts("5.6.7.8")
			Expect(record).To(Equal([]string{"qux.com"}))
		})

		It("overwrites records", func() {
			st.addRecord("1.2.3.4", []string{"foo.com"})
			record := st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"foo.com"}))

			st.overwriteRecord("1.2.3.4", []string{"bar.com"})
			record = st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"bar.com"}))
		})

		It("deletes records", func() {
			st.addRecord("1.2.3.4", []string{"foo.com"})
			record := st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"foo.com"}))

			st.deleteRecord("1.2.3.4")
			record = st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{}))
			Expect(len(st.Records)).To(Equal(0))
			Expect(st.Records["1.2.3.4"]).To(BeNil())
		})

		It("deletes hosts from records", func() {
			st.addRecord("1.2.3.4", []string{"bar.com", "baz.com", "foo.com", "qux.com"})
			record := st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"bar.com", "baz.com", "foo.com", "qux.com"}))

			st.deleteHost("bar.com")
			record = st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"baz.com", "foo.com", "qux.com"}))

			st.deleteHosts([]string{"baz.com", "foo.com"})
			record = st.getHosts("1.2.3.4")
			Expect(record).To(Equal([]string{"qux.com"}))
		})

		It("gets an IP address for a host", func() {
			st.addRecord("1.2.3.4", []string{"foo.com"})
			st.addRecord("5.6.7.8", []string{"bar.com"})
			ip := st.getIP("foo.com")
			Expect(ip).To(Equal("1.2.3.4"))
			ip = st.getIP("bar.com")
			Expect(ip).To(Equal("5.6.7.8"))
			ip = st.getIP("fake.com")
			Expect(ip).To(Equal(""))
		})
	})
})
