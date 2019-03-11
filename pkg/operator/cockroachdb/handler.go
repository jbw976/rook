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

// Package cockroachdb to manage cockroachdb instances.
package cockroachdb

import (
	"fmt"

	corev1alpha1 "github.com/crossplaneio/crossplane/pkg/apis/core/v1alpha1"
	cockroachdbv1alpha1 "github.com/rook/rook/pkg/apis/cockroachdb.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CockroachDBHandler struct {
	rookContext *clusterd.Context
}

func (h *CockroachDBHandler) Find(name types.NamespacedName, c client.Client) (corev1alpha1.Resource, error) {
	return h.rookContext.RookClientset.CockroachdbV1alpha1().Clusters(name.Namespace).Get(name.Name, metav1.GetOptions{})
}

// Provision create new Cluster
func (h *CockroachDBHandler) Provision(class *corev1alpha1.ResourceClass, claim corev1alpha1.ResourceClaim, c client.Client) (corev1alpha1.Resource, error) {
	// construct Cluster Spec from class definition
	clusterSpec := cockroachdbv1alpha1.NewClusterSpec(class.Parameters)

	clusterName := fmt.Sprintf("cockroachdb-%s", claim.GetUID())

	// assign reclaim policy from the resource class
	clusterSpec.ReclaimPolicy = class.ReclaimPolicy

	// set class and claim references
	clusterSpec.ClassRef = class.ObjectReference()
	clusterSpec.ClaimRef = claim.ObjectReference()

	// create and save Cluster
	cluster := &cockroachdbv1alpha1.Cluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: cockroachdbv1alpha1.APIVersion,
			Kind:       cockroachdbv1alpha1.ClusterKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       class.Namespace,
			Name:            clusterName,
			OwnerReferences: []metav1.OwnerReference{claim.OwnerReference()},
		},
		Spec: *clusterSpec,
	}

	return h.rookContext.RookClientset.CockroachdbV1alpha1().Clusters(class.Namespace).Create(cluster)
}

// SetBindStatus updates resource state binding phase
// TODO: this SetBindStatus function could be refactored to 1 common implementation for all providers
func (h CockroachDBHandler) SetBindStatus(name types.NamespacedName, c client.Client, bound bool) error {
	cluster, err := h.rookContext.RookClientset.CockroachdbV1alpha1().Clusters(name.Namespace).Get(name.Name, metav1.GetOptions{})
	if err != nil {
		// TODO: the CRD is not found and the binding state is supposed to be unbound. is this OK?
		if errors.IsNotFound(err) && !bound {
			return nil
		}
		return err
	}

	cluster.Status.SetBound(bound)

	_, err = h.rookContext.RookClientset.CockroachdbV1alpha1().Clusters(cluster.Namespace).Update(cluster)
	return err
}
