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

// Package to manage a Minio object store.
package minio

import (
	"fmt"
	"reflect"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	miniov1alpha1 "github.com/rook/rook/pkg/apis/minio.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"k8s.io/api/apps/v1beta2"
	"k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"
)

// TODO: A lot of these constants are specific to the KubeCon demo. Let's
// revisit these and determine what should be specified in the resource spec.
const (
	customResourceName       = "objectstore"
	customResourceNamePlural = "objectstores"
	minioNamespace           = "rook-minio-system"
	minioPVCName             = "minio-pvc"
	minioPVCAccessMode       = "ReadWriteOnce"
	minioLabel               = "minio"
	minioServerSuffix        = minioNamespace + ".svc.cluster.local"
	minioVolumeName          = "data"
	minioMountPath           = "/data"
	minioStorageGBString     = "10G"
	minioCtrName             = "minio"
	minioCtrImage            = "minio/minio:RELEASE.2018-04-19T22-54-58Z"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "minio-op-object")

// ObjectStoreResource represents the object store custom resource
var ObjectStoreResource = opkit.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   miniov1alpha1.CustomResourceGroup,
	Version: miniov1alpha1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(miniov1alpha1.ObjectStore{}).Name(),
}

// MinioController represents a controller object for object store custom resources
type MinioController struct {
	context   *clusterd.Context
	rookImage string
}

// NewMinioController create controller for watching object store custom resources created
func NewMinioController(context *clusterd.Context, rookImage string) *MinioController {
	return &MinioController{
		context:   context,
		rookImage: rookImage,
	}
}

// StartWatch watches for instances of ObjectStore custom resources and acts on them
func (c *MinioController) StartWatch(namespace string, stopCh chan struct{}) error {
	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching object store resources in namespace %s", namespace)
	watcher := opkit.NewWatcher(ObjectStoreResource, namespace, resourceHandlerFuncs, c.context.RookClientset.MinioV1alpha1().RESTClient())
	go watcher.Watch(&miniov1alpha1.ObjectStore{}, stopCh)

	return nil
}

func (c *MinioController) makeMinioHeadlessService(name string, spec miniov1alpha1.ObjectStoreSpec) (*v1.Service, error) {
	coreV1Client := c.context.Clientset.CoreV1()

	svc, err := coreV1Client.Services(minioNamespace).Create(&v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"app": minioLabel},
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": minioLabel},
			Ports: []v1.ServicePort{
				{
					Port: spec.Networking.Port,
				},
			},
			ClusterIP: v1.ClusterIPNone,
		},
	})

	return svc, err
}

func (c *MinioController) buildMinioCtrArgs(statefulSetPrefix string, headlessServiceName string, serverCount int32) []string {
	args := []string{"server"}
	for i := int32(0); i < serverCount; i++ {
		serverAddress := fmt.Sprintf("http://%s-%d.%s.%s%s", statefulSetPrefix, i, headlessServiceName, minioServerSuffix, minioMountPath)
		args = append(args, serverAddress)
	}
	return args
}

func (c *MinioController) makeMinioPodSpec(name string, ctrName string, ctrImage string, port int32, envVars map[string]string, numServers int32) v1.PodTemplateSpec {
	var env []v1.EnvVar
	for k, v := range envVars {
		env = append(env, v1.EnvVar{Name: k, Value: v})
	}

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"app": minioLabel},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  ctrName,
					Image: ctrImage,
					Env:   env,
					Ports: []v1.ContainerPort{
						{
							ContainerPort: port,
						},
					},
					// TODO: Let's go about building these args in a more elegant way.
					Args: c.buildMinioCtrArgs(name, name, numServers),
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      minioVolumeName,
							MountPath: minioMountPath,
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: minioVolumeName,
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: minioPVCName,
						},
					},
				},
			},
		},
	}

	return podSpec
}

func (c *MinioController) makeMinioStatefulSet(name string, spec miniov1alpha1.ObjectStoreSpec) (*v1beta2.StatefulSet, error) {
	appsClient := c.context.Clientset.AppsV1beta2()

	envVars := map[string]string{
		"MINIO_ACCESS_KEY": spec.Credentials.AccessKey,
		"MINIO_SECRET_KEY": spec.Credentials.SecretKey}

	podSpec := c.makeMinioPodSpec(name, minioCtrName, minioCtrImage, spec.Networking.Port, envVars, spec.NumServers)

	ss := v1beta2.StatefulSet{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"app": minioLabel},
		},
		Spec: v1beta2.StatefulSetSpec{
			Replicas: &spec.NumServers,
			Selector: &meta_v1.LabelSelector{
				MatchLabels: map[string]string{"app": minioLabel},
			},
			Template: podSpec,
			VolumeClaimTemplates: []v1.PersistentVolumeClaim{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      minioVolumeName,
						Namespace: minioNamespace,
					},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceStorage: resource.MustParse(minioStorageGBString),
							},
						},
					},
				},
			},
			ServiceName: name,
			// TODO: liveness probe
		},
	}

	return appsClient.StatefulSets(minioNamespace).Create(&ss)
}

func (c *MinioController) makeMinioService(name string, spec miniov1alpha1.ObjectStoreSpec) (*v1.Service, error) {
	coreV1Client := c.context.Clientset.CoreV1()

	// Parse the specified protocol. If we don't recognize it, just log an error and go with TCP.
	protocol := v1.ProtocolTCP
	if spec.Networking.Protocol == "UDP" {
		protocol = v1.ProtocolUDP
	} else if spec.Networking.Protocol != "TCP" {
		logger.Errorf("unrecognized protocol %s, setting to TCP", spec.Networking.Protocol)
		protocol = v1.ProtocolTCP
	}

	svc, err := coreV1Client.Services(minioNamespace).Create(&v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"app": minioLabel},
		},
		Spec: v1.ServiceSpec{
			Type:     v1.ServiceTypeNodePort,
			Selector: map[string]string{"app": minioLabel},
			Ports: []v1.ServicePort{
				{
					Port:       spec.Networking.Port,
					TargetPort: intstr.FromInt(int(spec.Networking.TargetPort)),
					Protocol:   protocol,
				},
			},
		},
	})

	return svc, err
}

func (c *MinioController) onAdd(obj interface{}) {
	objectstore := obj.(*miniov1alpha1.ObjectStore).DeepCopy()

	// TODO: Error handling. Do we need to delete all the previously successful creations?

	// Create the headless service.
	logger.Infof("Creating Minio headless service %s.", objectstore.Name)
	_, err := c.makeMinioHeadlessService(objectstore.Name, objectstore.Spec)
	if err != nil {
		logger.Errorf("failed to create minio service: %v", err)
		return
	}
	logger.Infof("Finished creating Minio headless service %s.", objectstore.Name)

	// Create the stateful set.
	logger.Infof("Creating Minio stateful set %s.", objectstore.Name)
	_, err = c.makeMinioStatefulSet(objectstore.Name, objectstore.Spec)
	if err != nil {
		logger.Errorf("failed to create minio stateful set: %v", err)
		return
	}
	logger.Infof("Finished creating Minio stateful set %s.", objectstore.Name)

	// Create the nodeport service.
	svcName := objectstore.Name + "-service"
	logger.Infof("Creating Minio service %s.", svcName)
	_, err = c.makeMinioService(svcName, objectstore.Spec)
	if err != nil {
		logger.Errorf("failed to create minio service: %v", err)
		return
	}
	logger.Infof("Finished creating Minio service %s.", svcName)
}

func (c *MinioController) onUpdate(oldObj, newObj interface{}) {
	oldStore := oldObj.(*miniov1alpha1.ObjectStore).DeepCopy()
	newStore := newObj.(*miniov1alpha1.ObjectStore).DeepCopy()

	// TODO: Do stuff.
	_ = oldStore
	_ = newStore
	logger.Infof("Called onUpdate.")
}

func (c *MinioController) onDelete(obj interface{}) {
	objectstore := obj.(*miniov1alpha1.ObjectStore).DeepCopy()
	var delOpts meta_v1.DeleteOptions
	propPolicy := meta_v1.DeletePropagationForeground
	delOpts.PropagationPolicy = &propPolicy
	//listOpts := meta_v1.ListOptions{LabelSelector: "app=" + minioLabel}
	coreV1Client := c.context.Clientset.CoreV1()

	// Delete nodeport service.
	svcName := objectstore.Name + "-service"
	logger.Infof("Deleting Minio service %s.", svcName)
	// TODO: Perhaps we shouldn't use DeleteCollection, but this works for a demo.
	err := coreV1Client.Services(minioNamespace).Delete(svcName, &delOpts)
	if err != nil {
		logger.Errorf("failed to delete service: %v", err)
	}
	logger.Infof("Finished deleting Minio service %s.", svcName)

	// Delete stateful set.
	logger.Infof("Deleting Minio stateful set %s.", objectstore.Name)
	appsClient := c.context.Clientset.AppsV1beta2()
	err = appsClient.StatefulSets(minioNamespace).Delete(objectstore.Name, &delOpts)
	if err != nil {
		logger.Errorf("failed to delete stateful se t: %v", err)
	}
	logger.Infof("Finished deleting Minio stateful set %s.", objectstore.Name)

	// Delete headless service.
	logger.Infof("Deleting Minio headless service %s.", objectstore.Name)
	err = coreV1Client.Services(minioNamespace).Delete(objectstore.Name, &delOpts)
	if err != nil {
		logger.Errorf("failed to delete service: %v", err)
	}
	logger.Infof("Finished deleting Minio headless service %s.", objectstore.Name)
}
