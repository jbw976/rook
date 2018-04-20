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
package cluster

import (
	"testing"

	cephv1alpha1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1alpha1"
	rookv1alpha1 "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConvertLegacyCluster(t *testing.T) {
	f := false

	legacyCluster := rookv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-cluster-5283",
			Namespace: "rook-9837",
		},
		Spec: rookv1alpha1.ClusterSpec{
			Backend:         "ceph",
			DataDirHostPath: "/var/lib/rook302",
			HostNetwork:     true,
			MonCount:        5,
			Storage: rookv1alpha1.StorageSpec{
				UseAllNodes: false,
				Selection: rookv1alpha1.Selection{
					UseAllDevices:  &f,
					DeviceFilter:   "dev1*",
					MetadataDevice: "nvme033",
					Directories: []rookv1alpha1.Directory{
						{Path: "/rook/dir1"},
					},
				},
				Config: rookv1alpha1.Config{
					Location: "datacenter=dc083",
					StoreConfig: rookv1alpha1.StoreConfig{
						StoreType:      "filestore",
						JournalSizeMB:  100,
						WalSizeMB:      200,
						DatabaseSizeMB: 300,
					},
				},
				Nodes: []rookv1alpha1.Node{
					{ // node with no node specific config
						Name: "node1",
					},
					{ // node with a lot of node specific config
						Name: "node2",
						Devices: []rookv1alpha1.Device{
							{Name: "vdx1"},
						},
						Selection: rookv1alpha1.Selection{
							UseAllDevices:  &f,
							DeviceFilter:   "dev2*",
							MetadataDevice: "nvme982",
							Directories: []rookv1alpha1.Directory{
								{Path: "/rook/dir2"},
							},
						},
						Config: rookv1alpha1.Config{
							Location: "datacenter=dc083,rack=rackA",
							StoreConfig: rookv1alpha1.StoreConfig{
								StoreType:      "bluestore",
								JournalSizeMB:  1000,
								WalSizeMB:      2000,
								DatabaseSizeMB: 3000,
							},
						},
					},
				},
			},
		},
	}

	expectedCluster := cephv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-cluster-5283",
			Namespace: "rook-9837",
		},
		Spec: cephv1alpha1.ClusterSpec{
			DataDirHostPath: "/var/lib/rook302",
			MonCount:        5,
			MetadataDevice:  "nvme033",
			StoreConfig: cephv1alpha1.StoreConfig{
				StoreType:      "filestore",
				JournalSizeMB:  100,
				WalSizeMB:      200,
				DatabaseSizeMB: 300,
			},
			Network: rookv1alpha2.NetworkSpec{HostNetwork: true},
			Storage: rookv1alpha2.StorageScopeSpec{
				UseAllNodes: false,
				Location:    "datacenter=dc083",
				Config: map[string]string{
					"storeType":      "filestore",
					"journalSizeMB":  "100",
					"walSizeMB":      "200",
					"databaseSizeMB": "300",
				},
				Selection: rookv1alpha2.Selection{
					UseAllDevices: &f,
					DeviceFilter:  "dev1*",
					Directories: []rookv1alpha2.Directory{
						{Path: "/rook/dir1"},
					},
				},
				Nodes: []rookv1alpha2.Node{
					{
						Name: "node1",
					},
					{
						Name:     "node2",
						Location: "datacenter=dc083,rack=rackA",
						Devices: []rookv1alpha2.Device{
							{Name: "vdx1"},
						},
						Selection: rookv1alpha2.Selection{
							UseAllDevices: &f,
							DeviceFilter:  "dev2*",
							Directories: []rookv1alpha2.Directory{
								{Path: "/rook/dir2"},
							},
						},
						Config: map[string]string{
							"metadataDevice": "nvme982",
							"storeType":      "bluestore",
							"journalSizeMB":  "1000",
							"walSizeMB":      "2000",
							"databaseSizeMB": "3000",
						},
					},
				},
			},
		},
	}

	// convert the legacy cluster and compare it to the expected cluster result
	assert.Equal(t, expectedCluster, convertLegacyCluster(&legacyCluster))
}
