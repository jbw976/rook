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

// Package mds to manage a rook file system.
package file

import (
	"fmt"
	"reflect"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	cephv1alpha1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1alpha1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	customResourceName       = "filesystem"
	customResourceNamePlural = "filesystems"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-file")

// FilesystemResource represents the file system custom resource
var FilesystemResource = opkit.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   cephv1alpha1.CustomResourceGroup,
	Version: cephv1alpha1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(cephv1alpha1.Filesystem{}).Name(),
}

var FilesystemResourceLegacy = opkit.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   rookalpha.CustomResourceGroup,
	Version: rookalpha.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(rookalpha.Filesystem{}).Name(),
}

// FilesystemController represents a controller for file system custom resources
type FilesystemController struct {
	context     *clusterd.Context
	rookImage   string
	hostNetwork bool
	ownerRef    metav1.OwnerReference
}

// NewFilesystemController create controller for watching file system custom resources created
func NewFilesystemController(context *clusterd.Context, rookImage string, hostNetwork bool, ownerRef metav1.OwnerReference) *FilesystemController {
	return &FilesystemController{
		context:     context,
		rookImage:   rookImage,
		hostNetwork: hostNetwork,
		ownerRef:    ownerRef,
	}
}

// StartWatch watches for instances of Filesystem custom resources and acts on them
func (c *FilesystemController) StartWatch(namespace string, stopCh chan struct{}, watchLegacyTypes bool) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching filesystem resource in namespace %s", namespace)
	watcher := opkit.NewWatcher(FilesystemResource, namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1alpha1().RESTClient())
	go watcher.Watch(&cephv1alpha1.Filesystem{}, stopCh)

	if watchLegacyTypes {
		logger.Infof("start watching legacy filesystems in all namespaces")
		watcherLegacy := opkit.NewWatcher(FilesystemResourceLegacy, namespace, resourceHandlerFuncs, c.context.RookClientset.RookV1alpha1().RESTClient())
		go watcherLegacy.Watch(&cephv1alpha1.Filesystem{}, stopCh)
	}

	return nil
}

func (c *FilesystemController) onAdd(obj interface{}) {
	filesystem, migrationNeeded, err := c.getFilesystemObject(obj)
	if err != nil {
		logger.Errorf("failed to get filesystem object: %+v", err)
		return
	}

	if migrationNeeded {
		if err = c.migrateFilesystemObject(filesystem); err != nil {
			logger.Errorf("failed to migrate filesystem %s in namespace %s: %+v", filesystem.Name, filesystem.Namespace, err)
		}
		return
	}

	err = CreateFilesystem(c.context, *filesystem, c.rookImage, c.hostNetwork, c.filesystemOwners(filesystem))
	if err != nil {
		logger.Errorf("failed to create file system %s. %+v", filesystem.Name, err)
	}
}

func (c *FilesystemController) onUpdate(oldObj, newObj interface{}) {
	oldFS, _, err := c.getFilesystemObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old filesystem object: %+v", err)
		return
	}
	newFS, migrationNeeded, err := c.getFilesystemObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new filesystem object: %+v", err)
		return
	}

	if migrationNeeded {
		if err = c.migrateFilesystemObject(newFS); err != nil {
			logger.Errorf("failed to migrate filesystem %s in namespace %s: %+v", newFS.Name, newFS.Namespace, err)
		}
		return
	}

	if !filesystemChanged(oldFS.Spec, newFS.Spec) {
		logger.Debugf("filesystem %s not updated", newFS.Name)
		return
	}

	// if the file system is modified, allow the file system to be created if it wasn't already
	logger.Infof("updating filesystem %s", newFS)
	err = CreateFilesystem(c.context, *newFS, c.rookImage, c.hostNetwork, c.filesystemOwners(newFS))
	if err != nil {
		logger.Errorf("failed to create (modify) file system %s. %+v", newFS.Name, err)
	}
}

func (c *FilesystemController) onDelete(obj interface{}) {
	filesystem, migrationNeeded, err := c.getFilesystemObject(obj)
	if err != nil {
		logger.Errorf("failed to get filesystem object: %+v", err)
		return
	}

	if migrationNeeded {
		logger.Infof("ignoring deletion of legacy filesystem %s in namespace %s", filesystem.Name, filesystem.Namespace)
		return
	}

	err = DeleteFilesystem(c.context, *filesystem)
	if err != nil {
		logger.Errorf("failed to delete file system %s. %+v", filesystem.Name, err)
	}
}

func (c *FilesystemController) filesystemOwners(fs *cephv1alpha1.Filesystem) []metav1.OwnerReference {

	// Only set the cluster crd as the owner of the filesystem resources.
	// If the filesystem crd is deleted, the operator will explicitly remove the filesystem resources.
	// If the filesystem crd still exists when the cluster crd is deleted, this will make sure the filesystem
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func filesystemChanged(oldFS, newFS cephv1alpha1.FilesystemSpec) bool {
	if len(oldFS.DataPools) != len(newFS.DataPools) {
		logger.Infof("number of data pools changed from %d to %d", len(oldFS.DataPools), len(newFS.DataPools))
		return true
	}
	if oldFS.MetadataServer.ActiveCount != newFS.MetadataServer.ActiveCount {
		logger.Infof("number of mds active changed from %d to %d", oldFS.MetadataServer.ActiveCount, newFS.MetadataServer.ActiveCount)
		return true
	}
	if oldFS.MetadataServer.ActiveStandby != newFS.MetadataServer.ActiveStandby {
		logger.Infof("mds active standby changed from %t to %t", oldFS.MetadataServer.ActiveStandby, newFS.MetadataServer.ActiveStandby)
		return true
	}
	return false
}

func (c *FilesystemController) getFilesystemObject(obj interface{}) (filesystem *cephv1alpha1.Filesystem, migrationNeeded bool, err error) {
	var ok bool
	filesystem, ok = obj.(*cephv1alpha1.Filesystem)
	if ok {
		// the filesystem object is of the latest type, simply return it
		return filesystem.DeepCopy(), false, nil
	}

	// type assertion to current filesystem type failed, try instead asserting to the legacy filesystem type
	// then convert it to the current filesystem type
	filesystemLegacy := obj.(*rookalpha.Filesystem).DeepCopy()
	filesystem, err = convertLegacyFilesystem(filesystemLegacy)
	if err != nil {
		return nil, true, fmt.Errorf("failed to convert legacy filesystem object. err: %+v. legacy object: %+v", err, filesystemLegacy)
	}

	return filesystem, true, nil
}

func (c *FilesystemController) migrateFilesystemObject(filesystemToMigrate *cephv1alpha1.Filesystem) error {
	logger.Infof("migrating legacy filesystem %s in namespace %s", filesystemToMigrate.Name, filesystemToMigrate.Namespace)

	_, err := c.context.RookClientset.CephV1alpha1().Filesystems(filesystemToMigrate.Namespace).Get(filesystemToMigrate.Name, metav1.GetOptions{})
	if err == nil {
		// filesystem of current type with same name/namespace already exists, don't overwrite it
		logger.Warningf("filesystem object %s in namespace %s already exists, will not overwrite with migrated legacy filesystem.",
			filesystemToMigrate.Name, filesystemToMigrate.Namespace)
	} else {
		if !errors.IsAlreadyExists(err) {
			return err
		}

		// filesystem of current type does not already exist, create it now to complete the migration
		_, err = c.context.RookClientset.CephV1alpha1().Filesystems(filesystemToMigrate.Namespace).Create(filesystemToMigrate)
		if err != nil {
			return err
		}

		logger.Infof("completed migration of legacy filesystem %s in namespace %s", filesystemToMigrate.Name, filesystemToMigrate.Namespace)
	}

	// delete the legacy filesystem instance, it should not be used anymore now that a migrated instance of the current type exists
	logger.Infof("deleting legacy filesystem %s in namespace %s", filesystemToMigrate.Name, filesystemToMigrate.Namespace)
	err = c.context.RookClientset.RookV1alpha1().Filesystems(filesystemToMigrate.Namespace).Delete(filesystemToMigrate.Name, nil)
	return err
}

func convertLegacyFilesystem(legacyFilesystem *rookalpha.Filesystem) (*cephv1alpha1.Filesystem, error) {
	if legacyFilesystem == nil {
		return nil, nil
	}

	return nil, fmt.Errorf("converting legacy filesystem not yet implemented")
}
