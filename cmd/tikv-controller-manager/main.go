// Copyright 2024 TiKV Project Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You can obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"flag"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/zhangjinpeng87/tikv-operator/pkg/apis/core/v1alpha1"
	"github.com/zhangjinpeng87/tikv-operator/pkg/controllers/cluster"
	"github.com/zhangjinpeng87/tikv-operator/pkg/controllers/pd"
	"github.com/zhangjinpeng87/tikv-operator/pkg/controllers/pdgroup"
	"github.com/zhangjinpeng87/tikv-operator/pkg/controllers/tikv"
	"github.com/zhangjinpeng87/tikv-operator/pkg/controllers/tikvgroup"
	"github.com/zhangjinpeng87/tikv-operator/pkg/scheme"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme.Scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "tikv-controller-manager",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup indexers
	if err = addIndexer(context.Background(), mgr); err != nil {
		setupLog.Error(err, "unable to add indexer")
		os.Exit(1)
	}

	// Setup controllers
	if err = cluster.Setup(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Cluster")
		os.Exit(1)
	}

	if err = pdgroup.Setup(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PDGroup")
		os.Exit(1)
	}

	if err = pd.Setup(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PD")
		os.Exit(1)
	}

	if err = tikvgroup.Setup(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TiKVGroup")
		os.Exit(1)
	}

	if err = tikv.Setup(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TiKV")
		os.Exit(1)
	}

	// Setup health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func addIndexer(ctx context.Context, mgr ctrl.Manager) error {
	// Index PDGroup by cluster name
	if err := mgr.GetFieldIndexer().IndexField(ctx, &v1alpha1.PDGroup{}, "spec.cluster.name",
		func(obj client.Object) []string {
			pdg := obj.(*v1alpha1.PDGroup)
			return []string{pdg.Spec.Cluster.Name}
		}); err != nil {
		return err
	}

	// Index TiKVGroup by cluster name
	if err := mgr.GetFieldIndexer().IndexField(ctx, &v1alpha1.TiKVGroup{}, "spec.cluster.name",
		func(obj client.Object) []string {
			kvg := obj.(*v1alpha1.TiKVGroup)
			return []string{kvg.Spec.Cluster.Name}
		}); err != nil {
		return err
	}

	return nil
}
