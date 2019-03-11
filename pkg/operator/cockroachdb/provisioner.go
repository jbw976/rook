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
	"context"

	storagev1alpha1 "github.com/crossplaneio/crossplane/pkg/apis/storage/v1alpha1"
	corecontroller "github.com/crossplaneio/crossplane/pkg/controller/core"
	cockroachdbv1alpha1 "github.com/rook/rook/pkg/apis/cockroachdb.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	provisionerControllerName = "cockroachdb-postgresql"
	provisionerFinalizerName  = "finalizer.postgresql.cockroachdb.rook.io"
)

var (
	ctx = context.Background()
)

type Provisioner struct {
	*corecontroller.Reconciler
}

func AddProvisionerToManager(mgr manager.Manager, rookContext *clusterd.Context) error {
	return addProvisioner(mgr, newProvisioner(mgr, rookContext))
}

func newProvisioner(mgr manager.Manager, rookContext *clusterd.Context) reconcile.Reconciler {
	handlers := map[string]corecontroller.ResourceHandler{
		cockroachdbv1alpha1.ClusterKindAPIVersion: &CockroachDBHandler{rookContext: rookContext},
	}

	r := &Provisioner{
		Reconciler: corecontroller.NewReconciler(mgr, provisionerControllerName, provisionerFinalizerName, handlers),
	}
	return r
}

func addProvisioner(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(provisionerControllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Instance
	err = c.Watch(&source.Kind{Type: &storagev1alpha1.PostgreSQLInstance{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a PostgreSQLInstance object and makes changes based on the state read
// and what is in the Instance.Spec
func (r *Provisioner) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// fetch the CRD instance
	instance := &storagev1alpha1.PostgreSQLInstance{}
	err := r.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return corecontroller.Result, nil
		}
		return corecontroller.Result, err
	}

	return r.DoReconcile(instance)
}
