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

package cluster

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/zhangjinpeng87/tikv-operator/pkg/apis/core/v1alpha1"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// Setup sets up the controller with the Manager.
func Setup(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Cluster{}).
		Watches(&v1alpha1.PDGroup{}, handler.EnqueueRequestsFromMapFunc(enqueueForPDGroup)).
		Watches(&v1alpha1.TiKVGroup{}, handler.EnqueueRequestsFromMapFunc(enqueueForTiKVGroup)).
		WithOptions(controller.Options{}).
		Complete(&ClusterReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
			Log:    mgr.GetLogger().WithName("cluster"),
		})
}

func enqueueForPDGroup(ctx context.Context, obj client.Object) []reconcile.Request {
	pdg := obj.(*v1alpha1.PDGroup)
	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Namespace: obj.GetNamespace(),
				Name:      pdg.Spec.Cluster.Name,
			},
		},
	}
}

func enqueueForTiKVGroup(ctx context.Context, obj client.Object) []reconcile.Request {
	kvg := obj.(*v1alpha1.TiKVGroup)
	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Namespace: obj.GetNamespace(),
				Name:      kvg.Spec.Cluster.Name,
			},
		},
	}
}

// Reconcile is part of the main kubernetes reconciliation loop
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("cluster", req.NamespacedName)

	cluster := &v1alpha1.Cluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Update cluster status by aggregating status from all groups
	if err := r.updateClusterStatus(ctx, cluster); err != nil {
		log.Error(err, "failed to update cluster status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) updateClusterStatus(ctx context.Context, cluster *v1alpha1.Cluster) error {
	// List all PDGroups for this cluster
	var pdGroupList v1alpha1.PDGroupList
	if err := r.List(ctx, &pdGroupList, client.InNamespace(cluster.Namespace),
		client.MatchingFields{"spec.cluster.name": cluster.Name}); err != nil {
		return err
	}

	// List all TiKVGroups for this cluster
	var tikvGroupList v1alpha1.TiKVGroupList
	if err := r.List(ctx, &tikvGroupList, client.InNamespace(cluster.Namespace),
		client.MatchingFields{"spec.cluster.name": cluster.Name}); err != nil {
		return err
	}

	// Aggregate component status
	components := []v1alpha1.ComponentStatus{}

	var pdReplicas int32
	for _, pdg := range pdGroupList.Items {
		pdReplicas += pdg.Status.GroupStatus.Replicas
	}
	if pdReplicas > 0 {
		components = append(components, v1alpha1.ComponentStatus{
			Kind:     v1alpha1.ComponentKindPD,
			Replicas: pdReplicas,
		})
	}

	var tikvReplicas int32
	for _, kvg := range tikvGroupList.Items {
		tikvReplicas += kvg.Status.GroupStatus.Replicas
	}
	if tikvReplicas > 0 {
		components = append(components, v1alpha1.ComponentStatus{
			Kind:     v1alpha1.ComponentKindTiKV,
			Replicas: tikvReplicas,
		})
	}

	// Update cluster status
	cluster.Status.Components = components
	cluster.Status.ObservedGeneration = cluster.Generation

	// Set PD URL if available
	if len(pdGroupList.Items) > 0 {
		cluster.Status.PD = "http://pd:2379"
	}

	return r.Status().Update(ctx, cluster)
}
