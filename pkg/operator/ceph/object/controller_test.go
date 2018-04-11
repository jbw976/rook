/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package rgw to manage a rook object store.
package object

import (
	"testing"

	cephv1alpha1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestObjectStoreChanged(t *testing.T) {
	old := cephv1alpha1.ObjectStoreSpec{Gateway: cephv1alpha1.GatewaySpec{Port: 80, SecurePort: 443, Instances: 1, AllNodes: false, SSLCertificateRef: ""}}
	new := cephv1alpha1.ObjectStoreSpec{Gateway: cephv1alpha1.GatewaySpec{Port: 80, SecurePort: 443, Instances: 1, AllNodes: false, SSLCertificateRef: ""}}
	// nothing changed
	assert.False(t, storeChanged(old, new))

	// there was a change
	new = cephv1alpha1.ObjectStoreSpec{Gateway: cephv1alpha1.GatewaySpec{Port: 81, SecurePort: 443, Instances: 1, AllNodes: false, SSLCertificateRef: ""}}
	assert.True(t, storeChanged(old, new))

	new = cephv1alpha1.ObjectStoreSpec{Gateway: cephv1alpha1.GatewaySpec{Port: 80, SecurePort: 444, Instances: 1, AllNodes: false, SSLCertificateRef: ""}}
	assert.True(t, storeChanged(old, new))

	new = cephv1alpha1.ObjectStoreSpec{Gateway: cephv1alpha1.GatewaySpec{Port: 80, SecurePort: 443, Instances: 2, AllNodes: false, SSLCertificateRef: ""}}
	assert.True(t, storeChanged(old, new))

	new = cephv1alpha1.ObjectStoreSpec{Gateway: cephv1alpha1.GatewaySpec{Port: 80, SecurePort: 443, Instances: 1, AllNodes: true, SSLCertificateRef: ""}}
	assert.True(t, storeChanged(old, new))

	new = cephv1alpha1.ObjectStoreSpec{Gateway: cephv1alpha1.GatewaySpec{Port: 80, SecurePort: 443, Instances: 1, AllNodes: false, SSLCertificateRef: "mysecret"}}
	assert.True(t, storeChanged(old, new))
}
