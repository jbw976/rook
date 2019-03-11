/*
Copyright 2018 The Rook Authors. All rights reserved.

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
package v1alpha1

import (
	"strconv"

	crossplanecorev1alpha1 "github.com/crossplaneio/crossplane/pkg/apis/core/v1alpha1"
	"github.com/crossplaneio/crossplane/pkg/util"
	rook "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ***************************************************************************
// IMPORTANT FOR CODE GENERATION
// If the types in this file are updated, you will need to run
// `make codegen` to generate the new types under the client/clientset folder.
// ***************************************************************************

type ClusterState string

const (
	ClusterStateAvailable ClusterState = "available"
	ClusterStateCreating  ClusterState = "creating"
	ClusterStateDeleting  ClusterState = "deleting"
	ClusterStateFailed    ClusterState = "failed"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ClusterSpec   `json:"spec"`
	Status            ClusterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Cluster `json:"items"`
}

type ClusterSpec struct {
	// The annotations-related configuration to add/set on each Pod related object.
	Annotations         rook.Annotations      `json:"annotations,omitempty"`
	Storage             rook.StorageScopeSpec `json:"scope,omitempty"`
	Network             rook.NetworkSpec      `json:"network,omitempty"`
	Secure              bool                  `json:"secure,omitempty"`
	CachePercent        int                   `json:"cachePercent,omitempty"`
	MaxSQLMemoryPercent int                   `json:"maxSQLMemoryPercent,omitempty"`

	// Kubernetes object references
	ClaimRef            *corev1.ObjectReference      `json:"claimRef,omitempty"`
	ClassRef            *corev1.ObjectReference      `json:"classRef,omitempty"`
	ConnectionSecretRef *corev1.LocalObjectReference `json:"connectionSecretRef,omitempty"`

	// ReclaimPolicy identifies how to handle the cloud resource after the deletion of this type
	ReclaimPolicy crossplanecorev1alpha1.ReclaimPolicy `json:"reclaimPolicy,omitempty"`
}

type ClusterStatus struct {
	crossplanecorev1alpha1.ConditionedStatus
	crossplanecorev1alpha1.BindingStatusPhase
	State    ClusterState `json:"state,omitempty"`
	Message  string       `json:"message,omitempty"`
	Endpoint string       `json:"endpoint,omitempty"`
}

func NewClusterSpec(properties map[string]string) *ClusterSpec {
	spec := &ClusterSpec{
		ReclaimPolicy: crossplanecorev1alpha1.ReclaimRetain,
	}

	val, ok := properties["nodeCount"]
	if ok {
		if nodeCount, err := strconv.Atoi(val); err == nil {
			spec.Storage.NodeCount = nodeCount
		}
	}

	val, ok = properties["storageSize"]
	if ok {
		if storageSize, err := resource.ParseQuantity(val); err == nil {
			spec.Storage.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rook-cockroachdb-data",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: storageSize,
							},
						},
					},
				},
			}
		}
	}

	val, ok = properties["httpPort"]
	if ok {
		if httpPort, err := strconv.Atoi(val); err == nil {
			spec.Network.Ports = append(spec.Network.Ports, rook.PortSpec{Name: "http", Port: int32(httpPort)})
		}
	}

	val, ok = properties["grpcPort"]
	if ok {
		if grpcPort, err := strconv.Atoi(val); err == nil {
			spec.Network.Ports = append(spec.Network.Ports, rook.PortSpec{Name: "grpc", Port: int32(grpcPort)})
		}
	}

	val, ok = properties["secure"]
	if ok {
		if secure, err := strconv.ParseBool(val); err == nil {
			spec.Secure = secure
		}
	}

	val, ok = properties["cachePercent"]
	if ok {
		if cachePercent, err := strconv.Atoi(val); err == nil {
			spec.CachePercent = cachePercent
		}
	}

	val, ok = properties["maxSQLMemoryPercent"]
	if ok {
		if maxSQLMemoryPercent, err := strconv.Atoi(val); err == nil {
			spec.MaxSQLMemoryPercent = maxSQLMemoryPercent
		}
	}

	return spec
}

// ConnectionSecretName returns a secret name from the reference
func (c *Cluster) ConnectionSecretName() string {
	if c.Spec.ConnectionSecretRef == nil {
		c.Spec.ConnectionSecretRef = &corev1.LocalObjectReference{
			Name: c.Name,
		}
	} else if c.Spec.ConnectionSecretRef.Name == "" {
		c.Spec.ConnectionSecretRef.Name = c.Name
	}

	return c.Spec.ConnectionSecretRef.Name
}

// ObjectReference to this Cluster
func (c *Cluster) ObjectReference() *corev1.ObjectReference {
	return util.ObjectReference(c.ObjectMeta, util.IfEmptyString(c.APIVersion, APIVersion),
		util.IfEmptyString(c.Kind, ClusterKind))
}

// State returns cluster state value saved in the status (could be empty)
func (c *Cluster) State() string {
	return string(c.Status.State)
}

// IsAvailable for usage/binding
func (c *Cluster) IsAvailable() bool {
	return c.State() == string(ClusterStateAvailable)
}

// IsBound determines if the resource is in a bound binding state
func (c *Cluster) IsBound() bool {
	return c.Status.IsBound()
}

// SetBound sets the binding state of this resource
func (c *Cluster) SetBound(bound bool) {
	c.Status.SetBound(bound)
}
