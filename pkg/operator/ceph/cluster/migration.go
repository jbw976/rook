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

// Package cluster to manage a Ceph cluster.
package cluster

import (
	cephv1alpha1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1alpha1"
	rookv1alpha1 "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *ClusterController) getClusterObject(obj interface{}) (cluster *cephv1alpha1.Cluster, migrationNeeded bool, err error) {
	var ok bool
	cluster, ok = obj.(*cephv1alpha1.Cluster)
	if ok {
		// the cluster object is of the latest type, simply return it
		return cluster.DeepCopy(), false, nil
	}

	// type assertion to current cluster type failed, try instead asserting to the legacy cluster type
	// then convert it to the current cluster type
	clusterLegacy := obj.(*rookv1alpha1.Cluster).DeepCopy()
	cluster = convertLegacyCluster(clusterLegacy)

	return cluster, true, nil
}

func (c *ClusterController) migrateClusterObject(clusterToMigrate *cephv1alpha1.Cluster) error {
	logger.Infof("migrating legacy cluster %s in namespace %s", clusterToMigrate.Name, clusterToMigrate.Namespace)

	_, err := c.context.RookClientset.CephV1alpha1().Clusters(clusterToMigrate.Namespace).Get(clusterToMigrate.Name, metav1.GetOptions{})
	if err == nil {
		// cluster of current type with same name/namespace already exists, don't overwrite it
		logger.Warningf("cluster object %s in namespace %s already exists, will not overwrite with migrated legacy cluster.",
			clusterToMigrate.Name, clusterToMigrate.Namespace)
	} else {
		if !errors.IsAlreadyExists(err) {
			return err
		}

		// cluster of current type does not already exist, create it now to complete the migration
		_, err = c.context.RookClientset.CephV1alpha1().Clusters(clusterToMigrate.Namespace).Create(clusterToMigrate)
		if err != nil {
			return err
		}

		logger.Infof("completed migration of legacy cluster %s in namespace %s", clusterToMigrate.Name, clusterToMigrate.Namespace)
	}

	// delete the legacy cluster instance, it should not be used anymore now that a migrated instance of the current type exists
	logger.Infof("deleting legacy cluster %s in namespace %s", clusterToMigrate.Name, clusterToMigrate.Namespace)
	err = c.context.RookClientset.RookV1alpha1().Clusters(clusterToMigrate.Namespace).Delete(clusterToMigrate.Name, nil)
	return err
}

func convertLegacyCluster(legacyCluster *rookv1alpha1.Cluster) *cephv1alpha1.Cluster {
	if legacyCluster == nil {
		return nil
	}

	legacySpec := legacyCluster.Spec

	cluster := &cephv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      legacyCluster.Name,
			Namespace: legacyCluster.Namespace,
		},
		Spec: cephv1alpha1.ClusterSpec{
			Storage: convertLegacyStorageScope(legacySpec.Storage),
		},
	}

	return cluster
}

func convertLegacyStorageScope(legacyStorageSpec rookv1alpha1.StorageSpec) rookv1alpha2.StorageScopeSpec {
	s := rookv1alpha2.StorageScopeSpec{
		UseAllNodes: legacyStorageSpec.UseAllNodes,
		Location:    legacyStorageSpec.Location,
	}

	return s
}
