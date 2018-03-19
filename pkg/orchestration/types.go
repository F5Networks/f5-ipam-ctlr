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

import "sync"

type (
	// resourceKey defines a resource for processing
	resourceKey struct {
		Kind      string
		Name      string
		Namespace string
	}

	// IPGroup is a struct of IPGroups and a protective mutex
	IPGroup struct {
		Groups     IPGroups
		GroupMutex *sync.Mutex
	}

	// IPGroups are groups of Specs
	// Specs in the same group will share an IP address
	// Specs in the "" (empty string) group will get their own IP addresses
	IPGroups map[GroupKey][]Spec

	// GroupKey indexes into a group of Specs
	GroupKey struct {
		Name    string
		Netview string
		Cidr    string
	}

	// Spec represents a single resource and its hosts
	Spec struct {
		Kind      string
		Name      string
		Namespace string
		Hosts     []string
		Netview   string // used for non-shared IP group
		Cidr      string // used for non-shared IP group
	}
)
