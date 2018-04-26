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

Portions of this file came from https://github.com/cockroachdb/cockroach, which uses the same license.
*/

// Package cluster to manage a cockroachdb cluster.
package cluster

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	cockroachdbv1alpha1 "github.com/rook/rook/pkg/apis/cockroachdb.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1beta1 "k8s.io/api/apps/v1beta1"
	"k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

const (
	CustomResourceName       = "cluster"
	CustomResourceNamePlural = "clusters"
	appName                  = "cockroachdb"
	initCreateInterval       = 6 * time.Second
	initCreateTimeout        = 1 * time.Minute
	updateClusterInterval    = 30 * time.Second
	updateClusterTimeout     = 1 * time.Hour
	httpPort                 = 8080
	httpPortName             = "http"
	grpcPort                 = 26257
	grpcPortName             = "grpc"
	volumeNameDataDir        = "datadir"
	envVarChannel            = "COCKROACH_CHANNEL"
	envVarValChannelSecure   = "kubernetes-secure"
	envVarValChannelInsecure = "kubernetes-insecure"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-cluster")
)

var ClusterResource = opkit.CustomResource{
	Name:    CustomResourceName,
	Plural:  CustomResourceNamePlural,
	Group:   cockroachdbv1alpha1.CustomResourceGroup,
	Version: cockroachdbv1alpha1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(cockroachdbv1alpha1.Cluster{}).Name(),
}

type ClusterController struct {
	context        *clusterd.Context
	containerImage string
}

func NewClusterController(context *clusterd.Context, containerImage string) *ClusterController {
	return &ClusterController{
		context:        context,
		containerImage: containerImage,
	}
}

type cluster struct {
	context   *clusterd.Context
	namespace string
	spec      cockroachdbv1alpha1.ClusterSpec
	ownerRef  metav1.OwnerReference
}

func newCluster(c *cockroachdbv1alpha1.Cluster, context *clusterd.Context) *cluster {
	return &cluster{
		context:   context,
		namespace: c.Namespace,
		spec:      c.Spec,
		ownerRef:  clusterOwnerRef(c.Namespace, string(c.UID)),
	}
}

func clusterOwnerRef(namespace, clusterID string) metav1.OwnerReference {
	blockOwner := true
	return metav1.OwnerReference{
		APIVersion:         ClusterResource.Version,
		Kind:               ClusterResource.Kind,
		Name:               namespace,
		UID:                types.UID(clusterID),
		BlockOwnerDeletion: &blockOwner,
	}
}

func (c *ClusterController) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching cockroachdb clusters in all namespaces")
	watcher := opkit.NewWatcher(ClusterResource, namespace, resourceHandlerFuncs, c.context.RookClientset.CockroachdbV1alpha1().RESTClient())
	go watcher.Watch(&cockroachdbv1alpha1.Cluster{}, stopCh)

	return nil
}

func (c *ClusterController) onAdd(obj interface{}) {
	clusterObj := obj.(*cockroachdbv1alpha1.Cluster)
	logger.Infof("new cluster %s added to namespace %s", clusterObj.Name, clusterObj.Namespace)

	cluster := newCluster(clusterObj, c.context)

	if cluster.spec.Replicas < 1 {
		logger.Errorf("invalid replicas count: %d. Must be at least 1.", cluster.spec.Replicas)
		return
	}

	if err := c.createClientService(cluster); err != nil {
		logger.Errorf("failed to create client service: %+v", err)
		return
	}

	if err := c.createReplicaService(cluster); err != nil {
		logger.Errorf("failed to create replica service: %+v", err)
		return
	}

	if err := c.createPodDisruptionBudget(cluster); err != nil {
		logger.Errorf("failed to create pod disruption budget: %+v", err)
		return
	}

	if err := c.createStatefulSet(cluster); err != nil {
		logger.Errorf("failed to create stateful set: %+v", err)
		return
	}

	// retry to init the cluster until it succeeds or times out
	err := wait.Poll(initCreateInterval, initCreateTimeout, func() (bool, error) {
		if err := c.initCluster(cluster); err != nil {
			return false, nil
		}

		return true, nil
	})
	if err != nil {
		logger.Errorf("failed to initialize cluster in namespace %s: %+v", cluster.namespace, err)
		return
	}

	logger.Infof("succeeded creating and initializing cluster in namespace %s", cluster.namespace)
}

func (c *ClusterController) onUpdate(oldObj, newObj interface{}) {
	_ = oldObj.(*cockroachdbv1alpha1.Cluster)
	newCluster := newObj.(*cockroachdbv1alpha1.Cluster)
	logger.Infof("cluster %s updated in namespace %s", newCluster.Name, newCluster.Namespace)
}

func (c *ClusterController) onDelete(obj interface{}) {
	cluster := obj.(*cockroachdbv1alpha1.Cluster)
	logger.Infof("cluster %s deleted from namespace %s", cluster.Name, cluster.Namespace)
}

func (c *ClusterController) createClientService(cluster *cluster) error {
	// This service is meant to be used by clients of the database. It exposes a ClusterIP that will
	// automatically load balance connections to the different database pods.
	clientService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "cockroachdb-public",
			Namespace:       cluster.namespace,
			OwnerReferences: []metav1.OwnerReference{cluster.ownerRef},
			Labels:          createAppLabels(),
		},
		Spec: v1.ServiceSpec{
			Selector: createAppLabels(),
			Type:     v1.ServiceTypeClusterIP,
			Ports:    createServicePorts(),
		},
	}

	if _, err := c.context.Clientset.CoreV1().Services(cluster.namespace).Create(clientService); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("client service %s already exists in namespace %s", clientService.Name, clientService.Namespace)
	} else {
		logger.Infof("client service %s started in namespace %s", clientService.Name, clientService.Namespace)
	}

	return nil
}

func (c *ClusterController) createReplicaService(cluster *cluster) error {
	// This service only exists to create DNS entries for each pod in the stateful
	// set such that they can resolve each other's IP addresses. It does not
	// create a load-balanced ClusterIP and should not be used directly by clients
	// in most circumstances.
	replicaService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            appName,
			Namespace:       cluster.namespace,
			OwnerReferences: []metav1.OwnerReference{cluster.ownerRef},
			Labels:          createAppLabels(),
			Annotations: map[string]string{
				// Use this annotation in addition to the actual publishNotReadyAddresses
				// field below because the annotation will stop being respected soon but the
				// field is broken in some versions of Kubernetes:
				// https://github.com/kubernetes/kubernetes/issues/58662
				"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
				"prometheus.io/scrape":                                   "true",
				"prometheus.io/path":                                     "_status/vars",
				"prometheus.io/port":                                     "8080",
			},
		},
		Spec: v1.ServiceSpec{
			Selector: createAppLabels(),
			// We want all pods in the StatefulSet to have their addresses published for
			// the sake of the other CockroachDB pods even before they're ready, since they
			// have to be able to talk to each other in order to become ready.
			PublishNotReadyAddresses: true,
			ClusterIP:                "None",
			Ports:                    createServicePorts(),
		},
	}

	if _, err := c.context.Clientset.CoreV1().Services(cluster.namespace).Create(replicaService); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("replica service %s already exists in namespace %s", replicaService.Name, replicaService.Namespace)
	} else {
		logger.Infof("replica service %s started in namespace %s", replicaService.Name, replicaService.Namespace)
	}

	return nil
}

func (c *ClusterController) createPodDisruptionBudget(cluster *cluster) error {
	maxUnavailable := intstr.FromInt(int(1))

	pdb := &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "cockroachdb-budget",
			Namespace:       cluster.namespace,
			OwnerReferences: []metav1.OwnerReference{cluster.ownerRef},
			Labels:          createAppLabels(),
		},
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: createAppLabels(),
			},
			MaxUnavailable: &maxUnavailable,
		},
	}

	if _, err := c.context.Clientset.PolicyV1beta1().PodDisruptionBudgets(cluster.namespace).Create(pdb); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("pod disruption budget %s already exists in namespace %s", pdb.Name, pdb.Namespace)
	} else {
		logger.Infof("pod disruption budget %s created in namespace %s", pdb.Name, pdb.Namespace)
	}

	return nil
}

func (c *ClusterController) createStatefulSet(cluster *cluster) error {
	replicas := int32(cluster.spec.Replicas)

	var envVarChannelVal string
	if cluster.spec.Secure {
		envVarChannelVal = envVarValChannelSecure
	} else {
		envVarChannelVal = envVarValChannelInsecure
	}

	terminationGracePeriodSeconds := int64(60)

	statefulSet := &appsv1beta1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            appName,
			Namespace:       cluster.namespace,
			OwnerReferences: []metav1.OwnerReference{cluster.ownerRef},
		},
		Spec: appsv1beta1.StatefulSetSpec{
			ServiceName: appName,
			Replicas:    &replicas,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.namespace,
					Labels:    createAppLabels(),
				},
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						PodAntiAffinity: &v1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
								{
									Weight: int32(100),
									PodAffinityTerm: v1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:      k8sutil.AppAttr,
													Operator: metav1.LabelSelectorOpIn,
													Values:   []string{appName},
												},
											},
										},
										TopologyKey: apis.LabelHostname,
									},
								},
							},
						},
					},
					Containers: []v1.Container{
						{
							Name:            appName,
							Image:           c.containerImage,
							ImagePullPolicy: v1.PullIfNotPresent,
							Ports: []v1.ContainerPort{
								{
									Name:          grpcPortName,
									ContainerPort: int32(grpcPort),
								},
								{
									Name:          httpPortName,
									ContainerPort: int32(httpPort),
								},
							},
							LivenessProbe: &v1.Probe{
								Handler: v1.Handler{
									HTTPGet: &v1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromString(httpPortName),
									},
								},
								InitialDelaySeconds: int32(30),
								PeriodSeconds:       int32(5),
							},
							ReadinessProbe: &v1.Probe{
								Handler: v1.Handler{
									HTTPGet: &v1.HTTPGetAction{
										Path: "/health?ready=1",
										Port: intstr.FromString(httpPortName),
									},
								},
								InitialDelaySeconds: int32(10),
								PeriodSeconds:       int32(5),
								FailureThreshold:    int32(2),
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      volumeNameDataDir,
									MountPath: "/cockroach/cockroach-data",
								},
							},
							Env: []v1.EnvVar{
								{
									Name:  envVarChannel,
									Value: envVarChannelVal,
								},
							},
							Command: []string{
								"/bin/bash",
								"-ecx",
								createCommand(cluster),
							},
						},
					},
					//  No pre-stop hook is required, a SIGTERM plus some time is all that's needed for graceful shutdown of a node.
					TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
					Volumes: []v1.Volume{
						{
							Name: volumeNameDataDir,
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: volumeNameDataDir,
								},
							},
						},
					},
				},
			},
			PodManagementPolicy: appsv1beta1.ParallelPodManagement,
			UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateStatefulSetStrategyType,
			},
			VolumeClaimTemplates: []v1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      volumeNameDataDir,
						Namespace: cluster.namespace,
					},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceStorage: cluster.spec.VolumeSize,
							},
						},
					},
				},
			},
		},
	}

	if _, err := c.context.Clientset.AppsV1beta1().StatefulSets(cluster.namespace).Create(statefulSet); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("stateful set %s already exists in namespace %s", statefulSet.Name, statefulSet.Namespace)
	} else {
		logger.Infof("stateful set %s created in namespace %s", statefulSet.Name, statefulSet.Namespace)
	}

	return nil
}

func (c *ClusterController) initCluster(cluster *cluster) error {
	hostFlag := fmt.Sprintf("--host=%s", createQualifiedReplicaServiceName(0, cluster.namespace))
	out, err := c.context.Executor.ExecuteCommandWithCombinedOutput(false, "cockroachdb init",
		"/cockroach/cockroach", "init", "--insecure", hostFlag)

	var outcome string
	if err != nil {
		outcome = "failed"
	} else {
		outcome = "succeeded"
	}

	logger.Infof("cluster init %s for namespace %s: %s", outcome, cluster.namespace, out)
	return err
}

func createAppLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr: appName,
	}
}

func createServicePorts() []v1.ServicePort {
	return []v1.ServicePort{
		{
			// The main port, served by gRPC, serves Postgres-flavor SQL, internode traffic and the cli.
			Name:       grpcPortName,
			Port:       int32(grpcPort),
			TargetPort: intstr.FromInt(int(grpcPort)),
		},
		{
			// The secondary port serves the UI as well as health and debug endpoints.
			Name:       httpPortName,
			Port:       int32(httpPort),
			TargetPort: intstr.FromInt(int(httpPort)),
		},
	}
}

// creates a qualified name of the replica service for a given replica and namespace, e.g., cockroachdb-0.cockroachdb.rook-cockroachdb
func createQualifiedReplicaServiceName(replicaNum int, namespace string) string {
	return fmt.Sprintf("%s-%d.%s.%s", appName, replicaNum, appName, namespace)
}

func createCommand(cluster *cluster) string {
	var insecureFlag string
	if !cluster.spec.Secure {
		insecureFlag = "--insecure"
	}

	var joinFlag string
	if cluster.spec.Replicas > 0 {
		// generate a list of DNS names of instances to join with that takes into account the service name of each stateful set
		// instance and the namespace they are in. e.g., cockroachdb-0.cockroachdb.rook-cockroachdb
		joinList := make([]string, cluster.spec.Replicas)
		for i := 0; i < cluster.spec.Replicas; i++ {
			joinList[i] = createQualifiedReplicaServiceName(i, cluster.namespace)
		}

		joinFlag = strings.Join(joinList, ",")
	}

	// The use of qualified `hostname -f` is crucial: Other nodes aren't able to look up the unqualified hostname.
	return fmt.Sprintf("exec /cockroach/cockroach start --logtostderr %s --advertise-host $(hostname -f) --http-host 0.0.0.0 --join %s --cache %s%% --max-sql-memory %s%%",
		insecureFlag, joinFlag, strconv.Itoa(cluster.spec.CachePercent), strconv.Itoa(cluster.spec.MaxSQLMemoryPercent))
}
