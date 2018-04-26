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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ***************************************************************************
// IMPORTANT FOR CODE GENERATION
// If the types in this file are updated, you will need to run
// `make codegen` to generate the new types under the client/clientset folder.
// ***************************************************************************

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ObjectStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ObjectStoreSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ObjectStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []ObjectStore `json:"items"`
}

// ObjectStoreSpec represent the spec of a pool
type ObjectStoreSpec struct {
	// The number of Minio servers we spin up.
	NumServers int32 `json:"numServers"`

	// Minio cluster credential configuration.
	Credentials CredentialConfig `json:"credentials"`

	// Networking configuration.
	Networking NetworkConfig `json:"network"`
}

// Username/password to access Minio via SDK, CLI, and browser interface.
// TODO: User k8s secret instead of plaintext.
type CredentialConfig struct {
	// Username.
	AccessKey string `json:"accessKey"`

	// Password.
	SecretKey string `json:"secretKey"`
}

type NetworkConfig struct {
	// TODO: figure out difference between port/targetPort.
	Port int32 `json:"port"`

	// Currently not used.
	TargetPort int32 `json:"targetPort"`

	// TODO: perhaps a raw string isn't the best thing to use here.
	Protocol string `json:"protocol"`
}
