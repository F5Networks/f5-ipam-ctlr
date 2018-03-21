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
	"bytes"
	"encoding/gob"
	"fmt"
	"sync"
	"time"

	log "github.com/F5Networks/f5-ipam-ctlr/pkg/vlogger"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	watch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const ipamWatchAnnotation = "ipam.f5.com/ip-allocation"
const groupAnnotation = "ipam.f5.com/group"
const netviewAnnotation = "ipam.f5.com/infoblox-netview"
const cidrAnnotation = "ipam.f5.com/network-cidr"
const hostnameAnnotation = "virtual-server.f5.com/hostname"
const ipAnnotation = "virtual-server.f5.com/ip"

// The IPAM system being used
var IPAM string

// Client that the controller uses to talk to Kubernetes
type K8sClient struct {
	// Kubernetes client
	kubeClient kubernetes.Interface
	// Rest Clients
	restClientv1      rest.Interface
	restClientv1beta1 rest.Interface

	// Tells whether the client is fully synced on startup
	initialState bool

	// Mutex for all informers
	informersMutex sync.Mutex

	// Queue and informers for namespaces and resources
	nsQueue     workqueue.RateLimitingInterface
	rsQueue     workqueue.RateLimitingInterface
	nsInformer  cache.SharedIndexInformer
	rsInformers map[string]*rsInformer

	// map of resources, grouped by specified groupname
	ipGroup IPGroup
	// tells whether IPGroups were updated and need to be written out
	updated bool

	// Channel for sending data to controller
	channel chan<- bytes.Buffer
}

// Struct to allow NewKubernetesClient to receive all or some parameters
type KubeParams struct {
	KubeClient     kubernetes.Interface
	restClient     rest.Interface
	Namespaces     []string
	NamespaceLabel string
	Channel        chan<- bytes.Buffer
}

// Sets up an interface with Kubernetes
func NewKubernetesClient(
	params *KubeParams,
) (*K8sClient, error) {
	var k8sClient *K8sClient
	var err error
	resyncPeriod := 30 * time.Second
	namespaces := params.Namespaces
	namespaceLabel := params.NamespaceLabel

	rsQueue := workqueue.NewNamedRateLimitingQueue(
		workqueue.DefaultControllerRateLimiter(), "resource-controller")
	nsQueue := workqueue.NewNamedRateLimitingQueue(
		workqueue.DefaultControllerRateLimiter(), "namespace-controller")

	k8sClient = &K8sClient{
		kubeClient:        params.KubeClient,
		restClientv1:      params.restClient,
		restClientv1beta1: params.restClient,
		channel:           params.Channel,
		rsQueue:           rsQueue,
		nsQueue:           nsQueue,
		rsInformers:       make(map[string]*rsInformer),
		ipGroup:           IPGroup{Groups: make(IPGroups), GroupMutex: new(sync.Mutex)},
	}

	if nil != k8sClient.kubeClient && nil == k8sClient.restClientv1 {
		// This is the normal production case, but need the checks for unit tests.
		k8sClient.restClientv1 = k8sClient.kubeClient.Core().RESTClient()
	}
	if nil != k8sClient.kubeClient && nil == k8sClient.restClientv1beta1 {
		// This is the normal production case, but need the checks for unit tests.
		k8sClient.restClientv1beta1 = k8sClient.kubeClient.Extensions().RESTClient()
	}

	// Add namespace/resource informers
	if len(namespaceLabel) == 0 {
		if len(namespaces) == 0 {
			// Watching all namespaces
			_, err = k8sClient.addNamespace("", resyncPeriod)
			if nil != err {
				log.Warningf("Failed to add informers for all namespaces: %v", err)
			}
		} else {
			// Watching some namespaces
			for _, namespace := range namespaces {
				_, err = k8sClient.addNamespace(namespace, resyncPeriod)
				if nil != err {
					log.Warningf("Failed to add informers for namespace %v: %v", namespace, err)
				}
			}
		}

	} else {
		ls, err := createLabel(namespaceLabel)
		if nil != err {
			log.Warningf("Failed to create label selector: %v", err)
		}
		err = k8sClient.addNamespaceLabelInformer(ls, resyncPeriod)
		if nil != err {
			log.Warningf("Failed to add label watch for all namespaces: %v", err)
		}
	}
	return k8sClient, nil
}

// Adds informers for resources in the specified namespaces
func (client *K8sClient) addNamespace(
	namespace string,
	resyncPeriod time.Duration,
) (*rsInformer, error) {
	client.informersMutex.Lock()
	defer client.informersMutex.Unlock()

	if client.watchingAllNamespaces() {
		return nil, fmt.Errorf(
			"Cannot add additional namespaces when already watching all.")
	} else if len(client.rsInformers) > 0 && "" == namespace {
		return nil, fmt.Errorf(
			"Cannot watch all namespaces when already watching specific ones.")
	}
	var rsInf *rsInformer
	var found bool
	if rsInf, found = client.rsInformers[namespace]; found {
		return rsInf, nil
	}
	rsInf = client.newRsInformer(namespace, resyncPeriod)
	client.rsInformers[namespace] = rsInf
	return rsInf, nil
}

// Adds a namespace informer if a label is specified
func (client *K8sClient) addNamespaceLabelInformer(
	labelSelector labels.Selector,
	resyncPeriod time.Duration,
) error {
	client.informersMutex.Lock()
	defer client.informersMutex.Unlock()

	if nil != client.nsInformer {
		return fmt.Errorf("Already have a namespace label informer added.")
	}
	if 0 != len(client.rsInformers) {
		return fmt.Errorf("Cannot set a namespace label informer when informers " +
			"have been setup for one or more namespaces.")
	}
	client.nsInformer = cache.NewSharedIndexInformer(
		newListWatch(
			client.restClientv1,
			"namespaces",
			"",
			labelSelector,
		),
		&v1.Namespace{},
		resyncPeriod,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	client.nsInformer.AddEventHandlerWithResyncPeriod(
		&cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { client.enqueueNamespace(obj) },
			UpdateFunc: func(old, cur interface{}) { client.enqueueNamespace(cur) },
			DeleteFunc: func(obj interface{}) { client.enqueueNamespace(obj) },
		},
		resyncPeriod,
	)
	return nil
}

// Resource Informer for a namespace
type rsInformer struct {
	namespace      string
	cfgMapInformer cache.SharedIndexInformer
	ingInformer    cache.SharedIndexInformer
	stopCh         chan struct{}
}

// Creates a Resource Informer for a namespace (watches Ingress/ConfigMaps)
func (client *K8sClient) newRsInformer(
	namespace string,
	resyncPeriod time.Duration,
) *rsInformer {
	rsInf := rsInformer{
		namespace: namespace,
		stopCh:    make(chan struct{}),
		cfgMapInformer: cache.NewSharedIndexInformer(
			newListWatch(
				client.restClientv1,
				"configmaps",
				namespace,
				labels.Everything(),
			),
			&v1.ConfigMap{},
			resyncPeriod,
			cache.Indexers{"name": MetaNameIndexFunc},
		),
		ingInformer: cache.NewSharedIndexInformer(
			newListWatch(
				client.restClientv1beta1,
				"ingresses",
				namespace,
				labels.Everything(),
			),
			&v1beta1.Ingress{},
			resyncPeriod,
			cache.Indexers{"name": MetaNameIndexFunc},
		),
	}

	rsInf.cfgMapInformer.AddEventHandlerWithResyncPeriod(
		&cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { client.addConfigMap(obj) },
			UpdateFunc: func(old, cur interface{}) { client.updateConfigMap(old, cur) },
			DeleteFunc: func(obj interface{}) { client.deleteConfigMap(obj) },
		},
		resyncPeriod,
	)

	rsInf.ingInformer.AddEventHandlerWithResyncPeriod(
		&cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { client.addIngress(obj) },
			UpdateFunc: func(old, cur interface{}) { client.updateIngress(old, cur) },
			DeleteFunc: func(obj interface{}) { client.deleteIngress(obj) },
		},
		resyncPeriod,
	)

	return &rsInf
}

// Returns the resource informer for a namespace
func (client *K8sClient) getResourceInformer(
	namespace string,
) (*rsInformer, bool) {
	client.informersMutex.Lock()
	defer client.informersMutex.Unlock()

	toFind := namespace
	if client.watchingAllNamespaces() {
		toFind = ""
	}
	rsInf, found := client.rsInformers[toFind]
	return rsInf, found
}

// Locks the informers and removes the rsInformers for a namespace
func (client *K8sClient) removeResourceInformers(namespace string) error {
	client.informersMutex.Lock()
	defer client.informersMutex.Unlock()

	if _, found := client.rsInformers[namespace]; !found {
		return fmt.Errorf("No informers exist for namespace %v", namespace)
	}
	delete(client.rsInformers, namespace)
	return nil
}

// Adds a Namespace to the workqueue
func (client *K8sClient) enqueueNamespace(obj interface{}) {
	ns := obj.(*v1.Namespace)
	client.nsQueue.Add(ns.ObjectMeta.Name)
}

// Adds a ConfigMap to the workqueue
func (client *K8sClient) addConfigMap(obj interface{}) {
	if ok, key := client.checkValidConfigMap(obj); ok {
		client.rsQueue.Add(*key)
	}
}

// Checks if an updated ConfigMap is still watched; process normally
func (client *K8sClient) updateConfigMap(old interface{}, cur interface{}) {
	cm := cur.(*v1.ConfigMap)

	if ok, key := client.checkValidConfigMap(old); ok {
		specFound, spec, _ := client.ipGroup.getSpec(*key)
		if ok, _ = client.checkValidConfigMap(cur); ok {
			// Updated ConfigMap is still valid, check if hosts have changed
			if specFound {
				for _, host := range spec.Hosts {
					if !cmHostExists(cm, host) {
						client.ipGroup.removeHost(host, *key)
					}
				}
			}
			client.rsQueue.Add(*key)
		} else {
			// New ConfigMap is no longer valid, remove its data from IPGroup
			if specFound {
				client.ipGroup.removeFromIPGroup(spec)
				client.writeIPGroups()
			}
		}
	} else if ok, key = client.checkValidConfigMap(cur); ok {
		// Old ConfigMap wasn't valid, but new one is
		client.rsQueue.Add(*key)
	}
}

// If a ConfigMap is deleted, remove its Spec
func (client *K8sClient) deleteConfigMap(obj interface{}) {
	if ok, key := client.checkValidConfigMap(obj); ok {
		found, spec, _ := client.ipGroup.getSpec(*key)
		if found {
			client.ipGroup.removeFromIPGroup(spec)
			client.writeIPGroups()
		}
	}
}

// Adds an Ingress to the workqueue
func (client *K8sClient) addIngress(obj interface{}) {
	if ok, key := client.checkValidIngress(obj); ok {
		client.rsQueue.Add(*key)
	}
}

// Checks if an updated Ingress is still watched; process normally
func (client *K8sClient) updateIngress(old interface{}, cur interface{}) {
	ing := cur.(*v1beta1.Ingress)

	if ok, key := client.checkValidIngress(old); ok {
		specFound, spec, grp := client.ipGroup.getSpec(*key)
		if ok, _ = client.checkValidIngress(cur); ok {
			if specFound {
				// Updated Ingress is still valid, check if hosts have changed
				for _, host := range spec.Hosts {
					if !ingHostExists(ing, host) {
						client.ipGroup.removeHost(host, *key)
						client.updated = true
					}
				}
				// Check if group has changed
				if val, _ := ing.ObjectMeta.Annotations[groupAnnotation]; val != grp {
					client.ipGroup.removeFromIPGroup(spec)
				}
			}
			client.rsQueue.Add(*key)
		} else {
			// New Ingress is no longer valid, remove its data from IPGroup
			if specFound {
				client.ipGroup.removeFromIPGroup(spec)
				client.writeIPGroups()
			}
		}
	} else if ok, key = client.checkValidIngress(cur); ok {
		// Old Ingress wasn't valid, but new one is
		client.rsQueue.Add(*key)
	}
}

// If an Ingress is deleted, remove its Spec
func (client *K8sClient) deleteIngress(obj interface{}) {
	if ok, key := client.checkValidIngress(obj); ok {
		found, spec, _ := client.ipGroup.getSpec(*key)
		if found {
			client.ipGroup.removeFromIPGroup(spec)
			client.writeIPGroups()
		}
	}
}

func (client *K8sClient) checkValidConfigMap(obj interface{}) (bool, *resourceKey) {
	cm := obj.(*v1.ConfigMap)
	// Check that the ConfigMap is in a namespace we're watching
	if _, ok := client.getResourceInformer(cm.ObjectMeta.Namespace); ok {
		// Verify that the ConfigMap is tagged with our annotation
		if val, ok := cm.ObjectMeta.Annotations[ipamWatchAnnotation]; ok && val == "dynamic" {
			rKey := &resourceKey{
				Kind:      "ConfigMap",
				Name:      cm.ObjectMeta.Name,
				Namespace: cm.ObjectMeta.Namespace,
			}
			return true, rKey
		}
	}
	return false, nil
}

func (client *K8sClient) checkValidIngress(obj interface{}) (bool, *resourceKey) {
	ing := obj.(*v1beta1.Ingress)
	// Check that the Ingress is in a namespace we're watching
	if _, ok := client.getResourceInformer(ing.ObjectMeta.Namespace); ok {
		// Verify that the Ingress is tagged with our annotation
		if val, ok := ing.ObjectMeta.Annotations[ipamWatchAnnotation]; ok && val == "dynamic" {
			rKey := &resourceKey{
				Kind:      "Ingress",
				Name:      ing.ObjectMeta.Name,
				Namespace: ing.ObjectMeta.Namespace,
			}
			return true, rKey
		}
	}
	return false, nil
}

// Runs the client
func (client *K8sClient) Run(stopCh <-chan struct{}) {
	log.Infof("Kubernetes client started: (%p)", client)
	go client.runImpl(stopCh)
}

func (client *K8sClient) runImpl(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer client.rsQueue.ShutDown()
	defer client.nsQueue.ShutDown()

	if nil != client.nsInformer {
		client.startNamespaceInformer(stopCh)
		go wait.Until(client.namespaceWorker, time.Second, stopCh)
	}
	client.startResourceInformers()
	go wait.Until(client.resourceWorker, time.Second, stopCh)

	<-stopCh
	client.informersMutex.Lock()
	defer client.informersMutex.Unlock()
	for _, rsInf := range client.rsInformers {
		rsInf.stop()
	}
}

// Starts and syncs the namespace informer
func (client *K8sClient) startNamespaceInformer(stopCh <-chan struct{}) {
	client.informersMutex.Lock()
	defer client.informersMutex.Unlock()
	go client.nsInformer.Run(stopCh)
	cache.WaitForCacheSync(stopCh, client.nsInformer.HasSynced)
}

// Starts and syncs the resource informers
func (client *K8sClient) startResourceInformers() {
	client.informersMutex.Lock()
	defer client.informersMutex.Unlock()
	for _, rsInf := range client.rsInformers {
		rsInf.start()
	}
	for _, rsInf := range client.rsInformers {
		rsInf.waitForCacheSync()
	}
}

// Runs a resource informer
func (rsInf *rsInformer) start() {
	go rsInf.cfgMapInformer.Run(rsInf.stopCh)
	go rsInf.ingInformer.Run(rsInf.stopCh)
}

// Stops a resource informer
func (rsInf *rsInformer) stop() {
	close(rsInf.stopCh)
}

// Waits for a resource informer's cache to be synced
func (rsInf *rsInformer) waitForCacheSync() {
	cache.WaitForCacheSync(
		rsInf.stopCh,
		rsInf.cfgMapInformer.HasSynced,
		rsInf.ingInformer.HasSynced,
	)
}

// Continually processes namespaces in the queue
func (client *K8sClient) namespaceWorker() {
	for client.processNextNamespace() {
	}
}

func (client *K8sClient) processNextNamespace() bool {
	key, quit := client.nsQueue.Get()
	if quit {
		return false
	}
	defer client.nsQueue.Done(key)

	err := client.syncNamespace(key.(string))
	if err == nil {
		client.nsQueue.Forget(key)
		return true
	}
	utilruntime.HandleError(fmt.Errorf("Sync %v failed with %v", key, err))
	client.nsQueue.AddRateLimited(key)
	return true
}

func (client *K8sClient) syncNamespace(namespace string) error {
	_, exists, err := client.nsInformer.GetIndexer().GetByKey(namespace)
	if nil != err {
		log.Warningf("Error looking up namespace '%v': %v", namespace, err)
		return err
	}

	rsInf, found := client.getResourceInformer(namespace)
	if exists && found {
		return nil
	}
	if exists {
		// Namespace exists but there's no rsInformer for it; add it
		rsInf, err = client.addNamespace(namespace, 0)
		if err != nil {
			return fmt.Errorf(
				"Failed to add informers for namespace %v: %v", namespace, err)
		}
		rsInf.start()
		rsInf.waitForCacheSync()
	} else {
		// Namespace doesn't exist but does have an rsInformer; delete it
		rsInf.stop()
		client.removeResourceInformers(namespace)
	}

	return nil
}

// Continually processes resources in the queue
func (client *K8sClient) resourceWorker() {
	for client.processNextResource() {
	}
}

func (client *K8sClient) processNextResource() bool {
	// Startup case: If there are no items in k8s when we start up (queues are empty and
	// initialState is false), we still need to send the empty groups to the Controller
	// so that it knows to clear the IPAM system of any existing records.
	if !client.initialState {
		go func() {
			// We sleep to give k8s a chance to add items to the queue (if there are any).
			// We don't want to enter this block too fast before the queue has been populated.
			time.Sleep(2 * time.Second)
			if client.rsQueue.Len() == 0 && client.nsQueue.Len() == 0 {
				client.writeIPGroups()
			}
		}()
	}

	key, quit := client.rsQueue.Get()
	if quit {
		return false
	}
	defer client.rsQueue.Done(key)

	err := client.syncResource(key.(resourceKey))
	if err == nil {
		client.rsQueue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("Sync %v failed with %v", key, err))
	client.rsQueue.AddRateLimited(key)
	return true
}

// Extract the hostname data from a resource and save the information
func (client *K8sClient) syncResource(rKey resourceKey) error {
	var ret bool
	rsInf, _ := client.getResourceInformer(rKey.Namespace)
	if rKey.Kind == "ConfigMap" {
		cfgMaps, err := rsInf.cfgMapInformer.GetIndexer().ByIndex("name", rKey.Name)
		if err != nil {
			log.Warningf("Unable to list ConfigMaps for namespace '%v': %v", rKey.Namespace, err)
			return err
		}

		for _, obj := range cfgMaps {
			cm := obj.(*v1.ConfigMap)
			// Skip any ConfigMaps that don't match the one we care about
			if cm.ObjectMeta.Namespace != rKey.Namespace || cm.ObjectMeta.Name != rKey.Name {
				continue
			}

			var host, netview, cidr string
			var ok bool
			if cidr, ok = cm.ObjectMeta.Annotations[cidrAnnotation]; !ok {
				log.Errorf(
					"No network cidr annotation provided for ConfigMap '%v'.", rKey.Name)
				return nil
			}
			if netview, ok = cm.ObjectMeta.Annotations[netviewAnnotation]; !ok && IPAM == "infoblox" {
				log.Errorf(
					"ConfigMap '%v' does not have required Infoblox netview annotation.", rKey.Name)
				return nil
			}
			if host, ok = cm.ObjectMeta.Annotations[hostnameAnnotation]; !ok {
				log.Errorf("No hostname annotation provided for ConfigMap '%v'.", rKey.Name)
				return nil
			}

			// Get the hostname from the annotation
			log.Debugf("Found host '%v' for ConfigMap '%s'", host, rKey.Name)

			ret = client.ipGroup.addToIPGroup(
				GroupKey{
					Name:    "",
					Netview: netview,
					Cidr:    cidr,
				},
				"ConfigMap",
				rKey.Name,
				rKey.Namespace,
				[]string{host},
			)
			client.updated = client.updated || ret
		}
	} else if rKey.Kind == "Ingress" {
		ingresses, err := rsInf.ingInformer.GetIndexer().ByIndex("name", rKey.Name)
		if err != nil {
			log.Warningf("Unable to list Ingresses for namespace '%v': %v", rKey.Namespace, err)
			return err
		}

		for _, obj := range ingresses {
			ing := obj.(*v1beta1.Ingress)
			// Skip any Ingresses that don't match the one we care about
			if ing.ObjectMeta.Namespace != rKey.Namespace || ing.ObjectMeta.Name != rKey.Name {
				continue
			}
			var val, groupName, netview, cidr string
			var ok bool
			if val, ok = ing.ObjectMeta.Annotations[groupAnnotation]; ok {
				groupName = val
			}

			if cidr, ok = ing.ObjectMeta.Annotations[cidrAnnotation]; !ok {
				log.Errorf(
					"No network cidr annotation provided for Ingress '%v'.", rKey.Name)
				return nil
			}
			if netview, ok = ing.ObjectMeta.Annotations[netviewAnnotation]; !ok && IPAM == "infoblox" {
				log.Errorf(
					"Ingress '%v' does not have required Infoblox netview annotation.", rKey.Name)
				return nil
			}

			if ing.Spec.Rules == nil { // single-service
				// Get the hostname from the annotation
				if host, ok := ing.ObjectMeta.Annotations[hostnameAnnotation]; ok {
					log.Debugf("Found host '%v' for Ingress '%s'", host, rKey.Name)
					ret = client.ipGroup.addToIPGroup(
						GroupKey{
							Name:    groupName,
							Netview: netview,
							Cidr:    cidr,
						},
						"Ingress",
						rKey.Name,
						rKey.Namespace,
						[]string{host},
					)
					client.updated = client.updated || ret
				} else {
					log.Warningf("No hostname annotation provided for Ingress '%v'.", rKey.Name)
				}
			} else { // multi-service
				var hosts []string
				for _, rule := range ing.Spec.Rules {
					hosts = append(hosts, rule.Host)
				}
				log.Debugf("Found hosts '%v' for Ingress '%s'", hosts, rKey.Name)
				ret = client.ipGroup.addToIPGroup(
					GroupKey{
						Name:    groupName,
						Netview: netview,
						Cidr:    cidr,
					},
					"Ingress",
					rKey.Name,
					rKey.Namespace,
					hosts,
				)
				client.updated = client.updated || ret
			}
		}
	}

	if client.updated {
		client.writeIPGroups()
	}
	return nil
}

// Writes the IPGroups to the Controller
func (client *K8sClient) writeIPGroups() {
	client.ipGroup.GroupMutex.Lock()
	defer client.ipGroup.GroupMutex.Unlock()

	if client.rsQueue.Len() == 0 && client.nsQueue.Len() == 0 || client.initialState {
		if client.channel != nil {
			log.Infof("Kubernetes client wrote %v hosts to Controller.",
				client.ipGroup.NumHosts())

			var ipBytes bytes.Buffer
			err := gob.NewEncoder(&ipBytes).Encode(client.ipGroup.Groups)
			if err != nil {
				log.Errorf("Couldn't encode IPGroups: %v", err)
				return
			}

			select {
			case client.channel <- ipBytes:
				log.Debug("Kubernetes client received ACK from Controller.")
			case <-time.After(3 * time.Second):
			}
		}
		client.initialState = true
		// Reset updated back to false
		client.updated = false
	}
}

// Annotates resources that contain given hosts with the given IP address
func (client *K8sClient) AnnotateResources(ip string, hosts []string) {
	var name, namespace string
	specsForIP := client.ipGroup.getSpecsWithHosts(hosts)

	for _, spec := range specsForIP {
		name = spec.Name
		namespace = spec.Namespace

		if spec.Kind == "ConfigMap" {
			cm, err := client.kubeClient.Core().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
			if err != nil {
				log.Errorf(
					"Could not retrieve ConfigMap '%s' to annotate with IP address: %v",
					name, err)
				continue
			}
			if cm.ObjectMeta.Annotations[ipAnnotation] != ip {
				cm.ObjectMeta.Annotations[ipAnnotation] = ip
				_, err := client.kubeClient.CoreV1().ConfigMaps(namespace).Update(cm)
				if nil != err {
					log.Errorf("Error when updating IP annotation: %v", err)
				} else {
					log.Debugf("Annotating ConfigMap '%s' with IP address '%s'.", name, ip)
				}
			}
		} else if spec.Kind == "Ingress" {
			ing, err := client.kubeClient.Extensions().Ingresses(namespace).
				Get(name, metav1.GetOptions{})
			if err != nil {
				log.Errorf(
					"Could not retrieve Ingress '%s' to annotate with IP address: %v",
					name, err)
				continue
			}
			if ing.ObjectMeta.Annotations[ipAnnotation] != ip {
				ing.ObjectMeta.Annotations[ipAnnotation] = ip
				_, err := client.kubeClient.Extensions().Ingresses(namespace).Update(ing)
				if nil != err {
					log.Errorf("Error when updating IP annotation: %v", err)
				} else {
					log.Debugf("Annotating Ingress '%s' with IP address '%s'.", name, ip)
				}
			}
		}
	}
}

// Check if a host still exists in a ConfigMap
func cmHostExists(cm *v1.ConfigMap, host string) bool {
	if val, ok := cm.ObjectMeta.Annotations[hostnameAnnotation]; ok && val == host {
		return true
	}
	return false
}

// Check if a host still exists in an Ingress
func ingHostExists(ing *v1beta1.Ingress, host string) bool {
	if ing.Spec.Rules == nil { // single-service
		if val, ok := ing.ObjectMeta.Annotations[hostnameAnnotation]; ok && val == host {
			return true
		}
		return false
	} else { // multi-service
		var hosts []string
		for _, rule := range ing.Spec.Rules {
			hosts = append(hosts, rule.Host)
		}
		if contains(hosts, host) {
			return true
		}
		return false
	}
}

func newListWatch(
	c cache.Getter,
	resource string,
	namespace string,
	labelSelector labels.Selector,
) cache.ListerWatcher {
	listFunc := func(options metav1.ListOptions) (runtime.Object, error) {
		return c.Get().
			Namespace(namespace).
			Resource(resource).
			VersionedParams(&metav1.ListOptions{
				LabelSelector: labelSelector.String(),
			}, metav1.ParameterCodec).
			Do().
			Get()
	}
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		return c.Get().
			Prefix("watch").
			Namespace(namespace).
			Resource(resource).
			VersionedParams(&metav1.ListOptions{
				LabelSelector: labelSelector.String(),
			}, metav1.ParameterCodec).
			Watch()
	}
	return &cache.ListWatch{ListFunc: listFunc, WatchFunc: watchFunc}
}

// Index function that indexes based on an object's name
func MetaNameIndexFunc(obj interface{}) ([]string, error) {
	meta, err := meta.Accessor(obj)
	if err != nil {
		return []string{""}, fmt.Errorf("object has no meta: %v", err)
	}
	return []string{meta.GetName()}, nil
}

func createLabel(label string) (labels.Selector, error) {
	var l labels.Selector
	var err error
	if label == "" {
		l = labels.Everything()
	} else {
		l, err = labels.Parse(label)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse Label Selector string: %v", err)
		}
	}
	return l, nil
}

func (client *K8sClient) watchingAllNamespaces() bool {
	if len(client.rsInformers) == 0 {
		return false
	}
	_, watchingAll := client.rsInformers[""]
	return watchingAll
}
