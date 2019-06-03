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

package cockroachdb

import (
	"fmt"
	"time"

	"github.com/crossplaneio/crossplane/pkg/apis"
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/clusterd"
	operator "github.com/rook/rook/pkg/operator/cockroachdb"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

const containerName = "rook-cockroachdb-operator"

var operatorCmd = &cobra.Command{
	Use:   "operator",
	Short: "Runs the cockroachdb operator to deploy and manage cockroachdb in kubernetes clusters",
	Long: `Runs the cockroachdb operator to deploy and manage cockroachdb in kubernetes clusters.
https://github.com/rook/rook`,
}

func init() {
	flags.SetFlagsFromEnv(operatorCmd.Flags(), rook.RookEnvVarPrefix)
	flags.SetLoggingFlags(operatorCmd.Flags())

	operatorCmd.RunE = startOperator
}

func startOperator(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(operatorCmd.Flags())

	clientset, apiExtClientset, rookClientset, err := rook.GetClientset()
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to get k8s clients. %+v", err))
	}

	context := createContext()
	context.NetworkInfo = clusterd.NetworkInfo{}
	context.ConfigDir = k8sutil.DataDir
	context.Clientset = clientset
	context.APIExtensionClientset = apiExtClientset
	context.RookClientset = rookClientset

	// Using the current image version to deploy other rook pods
	pod, err := k8sutil.GetRunningPod(clientset)
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to get pod. %+v", err))
	}

	rookImage, err := k8sutil.GetContainerImage(pod, containerName)
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to get container image. %+v", err))
	}

	logger.Info("starting cockroachdb controllers")
	cfg, err := rest.InClusterConfig()
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to get k8s config. %+v", err))
	}

	syncPeriod := 1 * time.Minute
	mgr, err := manager.New(cfg, manager.Options{SyncPeriod: &syncPeriod})
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to create manager. %+v", err))
	}

	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to add schemes. %+v", err))
	}

	if err := operator.AddToManager(mgr, context); err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to add controllers to manager. %+v", err))
	}

	go func() {
		if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
			rook.TerminateFatal(fmt.Errorf("failed to start manager. %+v", err))
		}
	}()

	logger.Infof("starting cockroachdb operator")
	op := operator.New(context, rookImage)
	err = op.Run()
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to run operator. %+v", err))
	}

	return nil
}
