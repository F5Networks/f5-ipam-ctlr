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

package test

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest/fake"
)

func NewNamespace(name string, labels map[string]string) *v1.Namespace {
	return &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
}

func NewConfigMap(
	name,
	namespace string,
	annotations map[string]string,
	data map[string]string,
) *v1.ConfigMap {
	return &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Data: data,
	}
}

func CopyConfigMap(old v1.ConfigMap) *v1.ConfigMap {
	a := make(map[string]string)
	for key, value := range old.Annotations {
		a[key] = value
	}
	return NewConfigMap(
		old.ObjectMeta.Name,
		old.ObjectMeta.Namespace,
		a,
		old.Data,
	)
}

func NewIngress(
	name,
	namespace string,
	annotations map[string]string,
	spec v1beta1.IngressSpec,
) *v1beta1.Ingress {
	return &v1beta1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "extensions/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: spec,
	}
}

func CopyIngress(old v1beta1.Ingress) *v1beta1.Ingress {
	a := make(map[string]string)
	for key, value := range old.Annotations {
		a[key] = value
	}
	return NewIngress(
		old.ObjectMeta.Name,
		old.ObjectMeta.Namespace,
		a,
		old.Spec,
	)
}

// CreateFakeHTTPClient returns a fake RESTClient which also satisfies rest.Interface
func CreateFakeHTTPClient() *fake.RESTClient {
	fakeClient := &fake.RESTClient{
		NegotiatedSerializer: &fakeNegotiatedSerializer{},
		Resp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte(""))),
		},
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			header := http.Header{}
			header.Set("Content-Type", runtime.ContentTypeJSON)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       ioutil.NopCloser(bytes.NewReader([]byte(""))),
			}, nil
		}),
	}
	return fakeClient
}

// // Below here is all used to mock the client calls
type fakeNegotiatedSerializer struct{}

func (fns *fakeNegotiatedSerializer) SupportedMediaTypes() []runtime.SerializerInfo {
	info := runtime.SerializerInfo{
		MediaType:        runtime.ContentTypeJSON,
		EncodesAsText:    true,
		Serializer:       nil,
		PrettySerializer: nil,
		StreamSerializer: &runtime.StreamSerializerInfo{
			EncodesAsText: true,
			Serializer:    &fakeDecoder{IsWatching: true},
			Framer:        &fakeFrame{},
		},
	}
	return []runtime.SerializerInfo{info}
}

func (fns *fakeNegotiatedSerializer) EncoderForVersion(
	serializer runtime.Encoder,
	gv runtime.GroupVersioner,
) runtime.Encoder {
	return nil
}

func (fns *fakeNegotiatedSerializer) DecoderToVersion(
	serializer runtime.Decoder,
	gv runtime.GroupVersioner,
) runtime.Decoder {
	return &fakeDecoder{}
}

type fakeDecoder struct {
	IsWatching bool
}

func (fd *fakeDecoder) Decode(
	data []byte,
	defaults *schema.GroupVersionKind,
	into runtime.Object,
) (runtime.Object, *schema.GroupVersionKind, error) {
	if fd.IsWatching {
		return nil, nil, io.EOF
	}
	return &v1.ConfigMapList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMapList",
			APIVersion: "v1",
		},
		ListMeta: metav1.ListMeta{
			SelfLink:        "/api/v1/namespaces/potato/configmaps",
			ResourceVersion: "1403005",
		},
		Items: []v1.ConfigMap{},
	}, nil, nil
}

func (fd *fakeDecoder) Encode(obj runtime.Object, w io.Writer) error {
	return nil
}

type fakeFrame struct{}

func (ff *fakeFrame) NewFrameReader(r io.ReadCloser) io.ReadCloser {
	return r
}
func (ff *fakeFrame) NewFrameWriter(w io.Writer) io.Writer {
	return w
}
