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

	"github.com/F5Networks/f5-ipam-ctlr/test"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

type mockK8sClient struct {
	client  *K8sClient
	mutex   sync.Mutex
	rsMutex map[resourceKey]*sync.Mutex
}

func newMockK8sClient(params *KubeParams) *mockK8sClient {
	client, err := NewKubernetesClient(params)
	Expect(err).To(BeNil())
	return &mockK8sClient{
		client:  client,
		mutex:   sync.Mutex{},
		rsMutex: make(map[resourceKey]*sync.Mutex),
	}
}

func (c *mockK8sClient) getRsMutex(rKey resourceKey) *sync.Mutex {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	mtx, ok := c.rsMutex[rKey]
	if !ok {
		mtx = &sync.Mutex{}
		c.rsMutex[rKey] = mtx
	}
	return mtx
}

func (c *mockK8sClient) addNamespace(ns *v1.Namespace, label string) bool {
	if "" == label {
		return false
	}
	_, found := ns.ObjectMeta.Labels[label]
	if found {
		c.client.nsInformer.GetStore().Add(ns)
		c.client.syncNamespace(ns.ObjectMeta.Name)
	}
	return found
}

func (c *mockK8sClient) addConfigMap(cm *v1.ConfigMap) bool {
	ok, rKey := c.client.checkValidConfigMap(cm)
	if ok {
		rsInf, _ := c.client.getResourceInformer(cm.ObjectMeta.Namespace)
		rsInf.cfgMapInformer.GetStore().Add(cm)
		mtx := c.getRsMutex(*rKey)
		mtx.Lock()
		defer mtx.Unlock()
		c.client.syncResource(*rKey)

	}
	return ok
}

func (c *mockK8sClient) updateConfigMap(old, cur *v1.ConfigMap) bool {
	c.client.updateConfigMap(old, cur)
	ok, rKey := c.client.checkValidConfigMap(cur)
	rsInf, _ := c.client.getResourceInformer(cur.ObjectMeta.Namespace)
	rsInf.cfgMapInformer.GetStore().Update(cur)
	if ok {
		mtx := c.getRsMutex(*rKey)
		mtx.Lock()
		defer mtx.Unlock()
		c.client.syncResource(*rKey)

	}
	return ok
}

func (c *mockK8sClient) deleteConfigMap(cm *v1.ConfigMap) bool {
	c.client.deleteConfigMap(cm)
	ok, _ := c.client.checkValidConfigMap(cm)
	return ok
}

func (c *mockK8sClient) addIngress(ing *v1beta1.Ingress) bool {
	ok, rKey := c.client.checkValidIngress(ing)
	if ok {
		rsInf, _ := c.client.getResourceInformer(ing.ObjectMeta.Namespace)
		rsInf.ingInformer.GetStore().Add(ing)
		mtx := c.getRsMutex(*rKey)
		mtx.Lock()
		defer mtx.Unlock()
		c.client.syncResource(*rKey)
	}
	return ok
}

func (c *mockK8sClient) updateIngress(old, cur *v1beta1.Ingress) bool {
	c.client.updateIngress(old, cur)
	ok, rKey := c.client.checkValidIngress(cur)
	rsInf, _ := c.client.getResourceInformer(cur.ObjectMeta.Namespace)
	rsInf.ingInformer.GetStore().Update(cur)
	if ok {
		mtx := c.getRsMutex(*rKey)
		mtx.Lock()
		defer mtx.Unlock()
		c.client.syncResource(*rKey)
	}

	return ok
}

func (c *mockK8sClient) deleteIngress(ing *v1beta1.Ingress) bool {
	c.client.deleteIngress(ing)
	ok, _ := c.client.checkValidIngress(ing)
	return ok
}

var _ = Describe("Kubernetes Client tests", func() {
	var realClient *K8sClient
	var mockClient *mockK8sClient
	var fakeClient kubernetes.Interface
	var err error

	BeforeEach(func() {
		fakeClient = fake.NewSimpleClientset()
		Expect(fakeClient).ToNot(BeNil())
	})

	It("creates a kubernetes client", func() {
		realClient, err = NewKubernetesClient(&KubeParams{
			KubeClient: fakeClient,
			restClient: test.CreateFakeHTTPClient(),
			Namespaces: []string{"ns1", "ns2"},
		})
		Expect(err).To(BeNil())
		Expect(realClient).ToNot(BeNil())
		Expect(len(realClient.rsInformers)).To(Equal(2))
	})

	Context("Using real client (testing class methods)", func() {
		BeforeEach(func() {
			realClient = &K8sClient{
				kubeClient:  fakeClient,
				rsInformers: make(map[string]*rsInformer),
			}
		})
		AfterEach(func() {
			for _, rsInf := range realClient.rsInformers {
				rsInf.stop()
			}
		})

		It("adds and removes informers for namespaces", func() {
			ns1 := "ns1"
			ns2 := "ns2"
			// Watch all namespaces
			Expect(realClient.watchingAllNamespaces()).To(BeFalse())
			_, err = realClient.addNamespace("", 0)
			Expect(err).To(BeNil())
			Expect(len(realClient.rsInformers)).To(Equal(1))
			_, ok := realClient.rsInformers[""]
			Expect(ok).To(BeTrue())
			Expect(realClient.watchingAllNamespaces()).To(BeTrue())
			rsInf, ok := realClient.getResourceInformer("")
			Expect(ok).To(BeTrue())
			Expect(rsInf).ToNot(BeNil())

			// Add new namespaces (should fail since watching all)
			_, err = realClient.addNamespace("ns1", 0)
			Expect(err).ToNot(BeNil())

			// Remove the rsInformer for all namespaces
			err = realClient.removeResourceInformers("")
			Expect(err).To(BeNil())
			Expect(len(realClient.rsInformers)).To(Equal(0))

			// Add new namespaces
			_, err = realClient.addNamespace("ns1", 0)
			Expect(err).To(BeNil())
			_, err = realClient.addNamespace("ns2", 0)
			Expect(err).To(BeNil())
			Expect(len(realClient.rsInformers)).To(Equal(2))
			_, ok = realClient.rsInformers[ns1]
			Expect(ok).To(BeTrue())
			_, ok = realClient.rsInformers[ns2]
			Expect(ok).To(BeTrue())
			Expect(realClient.watchingAllNamespaces()).To(BeFalse())

			// Try adding a namespace label informer (should fail)
			label, err := labels.Parse("watching")
			err = realClient.addNamespaceLabelInformer(label, 0)
			Expect(err).ToNot(BeNil())

			// Remove all informers and add label informer
			err = realClient.removeResourceInformers(ns1)
			Expect(err).To(BeNil())
			err = realClient.removeResourceInformers(ns2)
			Expect(err).To(BeNil())
			Expect(len(realClient.rsInformers)).To(Equal(0))
			err = realClient.addNamespaceLabelInformer(label, 0)
			Expect(err).To(BeNil())
			Expect(realClient.nsInformer).ToNot(BeNil())
		})
	})

	Context("Using mock client (testing functionality)", func() {
		AfterEach(func() {
			for _, rsInf := range mockClient.client.rsInformers {
				rsInf.stop()
			}
		})

		Context("with namespace label", func() {
			BeforeEach(func() {
				mockClient = newMockK8sClient(&KubeParams{
					KubeClient:     fakeClient,
					restClient:     test.CreateFakeHTTPClient(),
					NamespaceLabel: "watching",
				})
			})

			It("adds an nsInformer via label", func() {
				annotations := map[string]string{ipamWatchAnnotation: "dynamic"}
				data := map[string]string{"data": "testData"}
				spec := v1beta1.IngressSpec{
					Backend: &v1beta1.IngressBackend{
						ServiceName: "foo",
						ServicePort: intstr.IntOrString{IntVal: 80},
					},
				}

				ns1 := test.NewNamespace("ns1", map[string]string{})
				ns2 := test.NewNamespace("ns2", map[string]string{"watching": "yes"})

				// Label for ns2, so we will only watch ns2
				r := mockClient.addNamespace(ns1, "")
				Expect(r).To(BeFalse())
				r = mockClient.addNamespace(ns2, "watching")
				Expect(r).To(BeTrue())

				// Verify ConfigMaps and Ingresses in ns1 are not processed; ns2 are processed
				cfgNs1 := test.NewConfigMap("one", ns1.ObjectMeta.Name, annotations, data)
				cfgNs2 := test.NewConfigMap("two", ns2.ObjectMeta.Name, annotations, data)
				r = mockClient.addConfigMap(cfgNs1)
				Expect(r).To(BeFalse())
				r = mockClient.addConfigMap(cfgNs2)
				Expect(r).To(BeTrue())

				ingNs1 := test.NewIngress("one", ns1.ObjectMeta.Name, annotations, spec)
				ingNs2 := test.NewIngress("two", ns2.ObjectMeta.Name, annotations, spec)
				r = mockClient.addIngress(ingNs1)
				Expect(r).To(BeFalse())
				r = mockClient.addIngress(ingNs2)
				Expect(r).To(BeTrue())
			})
		})

		Context("without namespace label", func() {
			ns1 := "ns1"
			ns2 := "ns2"
			netview := "default"
			cidr := "1.2.3.0/24"
			emptyGrp := GroupKey{
				Name:    "",
				Netview: netview,
				Cidr:    cidr,
			}
			aGrp := GroupKey{
				Name:    "A",
				Netview: netview,
				Cidr:    cidr,
			}
			bGrp := GroupKey{
				Name:    "B",
				Netview: netview,
				Cidr:    cidr,
			}
			BeforeEach(func() {
				mockClient = newMockK8sClient(&KubeParams{
					KubeClient: fakeClient,
					restClient: test.CreateFakeHTTPClient(),
				})
			})

			It("configures IPGroups for watched resources", func() {
				data := map[string]string{"data": "testData"}
				spec1 := v1beta1.IngressSpec{
					Backend: &v1beta1.IngressBackend{
						ServiceName: "foo",
						ServicePort: intstr.IntOrString{IntVal: 80},
					},
				}
				spec2 := v1beta1.IngressSpec{
					Rules: []v1beta1.IngressRule{
						{Host: "bar.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{
										{Path: "/foo",
											Backend: v1beta1.IngressBackend{
												ServiceName: "foo",
												ServicePort: intstr.IntOrString{IntVal: 80},
											},
										},
									},
								},
							},
						},
						{Host: "baz.com",
							IngressRuleValue: v1beta1.IngressRuleValue{
								HTTP: &v1beta1.HTTPIngressRuleValue{
									Paths: []v1beta1.HTTPIngressPath{
										{Path: "/foo",
											Backend: v1beta1.IngressBackend{
												ServiceName: "foo",
												ServicePort: intstr.IntOrString{IntVal: 80},
											},
										},
									},
								},
							},
						},
					},
				}

				// No hostname to start; shouldn't create entry for cfgMap
				cfg1 := test.NewConfigMap("cfg1", ns1,
					map[string]string{
						ipamWatchAnnotation: "dynamic",
					}, data)
				r := mockClient.addConfigMap(cfg1)
				Expect(r).To(BeTrue())

				// Single-service Ingress
				ing1 := test.NewIngress("ing1", ns2,
					map[string]string{
						ipamWatchAnnotation: "dynamic",
						groupAnnotation:     "A",
						hostnameAnnotation:  "foo.com",
						netviewAnnotation:   netview,
						cidrAnnotation:      cidr,
					}, spec1)
				r = mockClient.addIngress(ing1)
				Expect(r).To(BeTrue())
				ing1Spec := Spec{
					Kind:      "Ingress",
					Name:      "ing1",
					Namespace: ns2,
					Hosts:     []string{"foo.com"},
					Netview:   netview,
					Cidr:      cidr,
				}

				Expect(len(mockClient.client.ipGroup.Groups)).To(Equal(1))
				groupA := mockClient.client.ipGroup.Groups[aGrp]
				Expect(groupA).ToNot(BeNil())
				Expect(groupA).To(Equal([]Spec{ing1Spec}))
				Expect(mockClient.client.ipGroup.Groups[emptyGrp]).To(BeNil())

				// Multi-service Ingress
				ing2 := test.NewIngress("ing2", ns1,
					map[string]string{
						ipamWatchAnnotation: "dynamic",
						groupAnnotation:     "A",
						netviewAnnotation:   netview,
						cidrAnnotation:      cidr,
					}, spec2)
				r = mockClient.addIngress(ing2)
				Expect(r).To(BeTrue())
				ing2Spec := Spec{
					Kind:      "Ingress",
					Name:      "ing2",
					Namespace: ns1,
					Hosts:     []string{"bar.com", "baz.com"},
					Netview:   netview,
					Cidr:      cidr,
				}

				Expect(len(mockClient.client.ipGroup.Groups)).To(Equal(1))
				groupA = mockClient.client.ipGroup.Groups[aGrp]
				Expect(groupA).ToNot(BeNil())
				Expect(groupA).To(Equal([]Spec{ing1Spec, ing2Spec}))

				// Update cfgMap to have hostname
				cfg1.ObjectMeta.Annotations = map[string]string{
					ipamWatchAnnotation: "dynamic",
					hostnameAnnotation:  "qux.com",
					netviewAnnotation:   netview,
					cidrAnnotation:      cidr,
				}
				oldCfg := test.CopyConfigMap(*cfg1)
				mockClient.updateConfigMap(oldCfg, cfg1)
				cfg1Spec := Spec{
					Kind:      "ConfigMap",
					Name:      "cfg1",
					Namespace: ns1,
					Hosts:     []string{"qux.com"},
					Netview:   netview,
					Cidr:      cidr,
				}

				Expect(len(mockClient.client.ipGroup.Groups)).To(Equal(2))
				groupAll := mockClient.client.ipGroup.Groups[emptyGrp]
				Expect(groupAll).ToNot(BeNil())
				Expect(groupAll).To(Equal([]Spec{cfg1Spec}))

				// Try adding resource without ipam annotation; should fail
				ing3 := test.NewIngress("ing3", ns2,
					map[string]string{
						groupAnnotation:    "A",
						hostnameAnnotation: "foo.com",
					}, spec1)
				r = mockClient.addIngress(ing3)
				Expect(r).To(BeFalse())

				// Delete ConfigMap; IPGroup should be removed
				mockClient.deleteConfigMap(cfg1)
				Expect(len(mockClient.client.ipGroup.Groups)).To(Equal(1))
				groupAll = mockClient.client.ipGroup.Groups[emptyGrp]
				Expect(groupAll).To(BeNil())

				// Remove IPAM annotation; IPGroup should be removed
				oldIng := test.CopyIngress(*ing1)
				delete(ing1.ObjectMeta.Annotations, ipamWatchAnnotation)
				mockClient.updateIngress(oldIng, ing1)
				groupA = mockClient.client.ipGroup.Groups[aGrp]
				Expect(groupA).ToNot(BeNil())
				Expect(groupA).To(Equal([]Spec{ing2Spec}))

				// Change group
				oldIng = test.CopyIngress(*ing2)
				ing2.ObjectMeta.Annotations[groupAnnotation] = "B"
				mockClient.updateIngress(oldIng, ing2)
				groupA = mockClient.client.ipGroup.Groups[aGrp]
				Expect(groupA).To(BeNil())
				groupB := mockClient.client.ipGroup.Groups[bGrp]
				Expect(groupB).ToNot(BeNil())
				Expect(groupB).To(Equal([]Spec{ing2Spec}))

				// Change host
				oldIng = test.CopyIngress(*ing2)
				ing2.Spec.Rules[0].Host = "newhost.com"
				mockClient.updateIngress(oldIng, ing2)
				groupB = mockClient.client.ipGroup.Groups[bGrp]
				ing2Spec.Hosts = []string{"baz.com", "newhost.com"}
				Expect(groupB).To(Equal([]Spec{ing2Spec}))

				// Remove last Ingress
				mockClient.deleteIngress(ing2)
				Expect(len(mockClient.client.ipGroup.Groups)).To(Equal(0))
				groupA = mockClient.client.ipGroup.Groups[aGrp]
				Expect(groupA).To(BeNil())
			})
		})
	})
})
