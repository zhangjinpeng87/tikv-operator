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

package tikvgroup

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/zhangjinpeng87/tikv-operator/pkg/apis/core/v1alpha1"
)

// TiKVGroupReconciler reconciles a TiKVGroup object
type TiKVGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// Setup sets up the controller with the Manager.
func Setup(mgr manager.Manager) error {
	r := &TiKVGroupReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    mgr.GetLogger().WithName("tikvgroup"),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.TiKVGroup{}).
		Owns(&v1alpha1.TiKV{}).
		Watches(&v1alpha1.Cluster{}, handler.EnqueueRequestsFromMapFunc(r.enqueueForCluster)).
		WithOptions(controller.Options{}).
		Complete(r)
}

func (r *TiKVGroupReconciler) enqueueForCluster(ctx context.Context, obj client.Object) []reconcile.Request {
	cluster := obj.(*v1alpha1.Cluster)
	var tikvGroupList v1alpha1.TiKVGroupList
	if err := r.List(ctx, &tikvGroupList, client.InNamespace(cluster.Namespace),
		client.MatchingFields{"spec.cluster.name": cluster.Name}); err != nil {
		return []reconcile.Request{}
	}

	requests := []reconcile.Request{}
	for _, kvg := range tikvGroupList.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      kvg.Name,
				Namespace: kvg.Namespace,
			},
		})
	}
	return requests
}

// Reconcile reconciles TiKVGroup by managing TiKV instances
func (r *TiKVGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("tikvgroup", req.NamespacedName)

	tikvGroup := &v1alpha1.TiKVGroup{}
	if err := r.Get(ctx, req.NamespacedName, tikvGroup); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if cluster exists
	cluster := &v1alpha1.Cluster{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: req.Namespace,
		Name:      tikvGroup.Spec.Cluster.Name,
	}, cluster); err != nil {
		log.Error(err, "cluster not found")
		return ctrl.Result{}, err
	}

	// List existing TiKV instances
	var tikvList v1alpha1.TiKVList
	if err := r.List(ctx, &tikvList, client.InNamespace(req.Namespace),
		client.MatchingLabels{
			v1alpha1.LabelKeyCluster:   tikvGroup.Spec.Cluster.Name,
			v1alpha1.LabelKeyGroup:     tikvGroup.Name,
			v1alpha1.LabelKeyComponent: v1alpha1.LabelValComponentTiKV,
		}); err != nil {
		return ctrl.Result{}, err
	}

	desiredReplicas := int32(0)
	if tikvGroup.Spec.Replicas != nil {
		desiredReplicas = *tikvGroup.Spec.Replicas
	}
	currentReplicas := int32(len(tikvList.Items))

	// Scale out: create new TiKV instances
	if desiredReplicas > currentReplicas {
		for i := currentReplicas; i < desiredReplicas; i++ {
			tikvName := fmt.Sprintf("%s-tikv-%d", tikvGroup.Name, i)
			tikv := r.buildTiKV(tikvGroup, tikvName)
			if err := controllerutil.SetControllerReference(tikvGroup, tikv, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.Create(ctx, tikv); err != nil {
				if !errors.IsAlreadyExists(err) {
					log.Error(err, "failed to create TiKV instance", "name", tikvName)
					return ctrl.Result{}, err
				}
			} else {
				log.Info("created TiKV instance", "name", tikvName)
			}
		}
	}

	// Scale in: delete TiKV instances (in reverse order)
	if desiredReplicas < currentReplicas {
		toDelete := currentReplicas - desiredReplicas
		for i := currentReplicas - 1; i >= desiredReplicas; i-- {
			if toDelete <= 0 {
				break
			}
			tikvName := fmt.Sprintf("%s-tikv-%d", tikvGroup.Name, i)
			tikv := &v1alpha1.TiKV{}
			if err := r.Get(ctx, types.NamespacedName{
				Namespace: req.Namespace,
				Name:      tikvName,
			}, tikv); err != nil {
				if !errors.IsNotFound(err) {
					log.Error(err, "failed to get TiKV instance", "name", tikvName)
					continue
				}
			} else {
				// Mark as offline first for graceful deletion
				if !tikv.Spec.Offline {
					tikv.Spec.Offline = true
					if err := r.Update(ctx, tikv); err != nil {
						log.Error(err, "failed to mark TiKV offline", "name", tikvName)
						continue
					}
				}
				// TODO: Wait for offline completion before deletion
				// For now, just delete directly
				if err := r.Delete(ctx, tikv); err != nil {
					log.Error(err, "failed to delete TiKV instance", "name", tikvName)
					return ctrl.Result{}, err
				}
				log.Info("deleted TiKV instance", "name", tikvName)
				toDelete--
			}
		}
	}

	// Update status
	if err := r.updateStatus(ctx, tikvGroup, tikvList.Items); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *TiKVGroupReconciler) buildTiKV(tikvGroup *v1alpha1.TiKVGroup, name string) *v1alpha1.TiKV {
	tikv := &v1alpha1.TiKV{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: tikvGroup.Namespace,
			Labels: map[string]string{
				v1alpha1.LabelKeyManagedBy: v1alpha1.LabelValManagedByOperator,
				v1alpha1.LabelKeyCluster:   tikvGroup.Spec.Cluster.Name,
				v1alpha1.LabelKeyComponent: v1alpha1.LabelValComponentTiKV,
				v1alpha1.LabelKeyGroup:     tikvGroup.Name,
				v1alpha1.LabelKeyInstance:  name,
			},
		},
		Spec: v1alpha1.TiKVSpec{
			Cluster:          tikvGroup.Spec.Cluster,
			TiKVTemplateSpec: tikvGroup.Spec.Template.Spec,
		},
	}
	return tikv
}

func (r *TiKVGroupReconciler) updateStatus(ctx context.Context, tikvGroup *v1alpha1.TiKVGroup, tikvs []v1alpha1.TiKV) error {
	readyReplicas := int32(0)
	for _, tikv := range tikvs {
		// Check if TiKV is ready (simplified check)
		if tikv.Status.ID != "" && tikv.Status.State == v1alpha1.StoreStateServing {
			readyReplicas++
		}
	}

	replicas := int32(len(tikvs))
	tikvGroup.Status.CommonStatus.ObservedGeneration = tikvGroup.Generation
	tikvGroup.Status.GroupStatus.Replicas = replicas
	tikvGroup.Status.GroupStatus.ReadyReplicas = readyReplicas
	tikvGroup.Status.GroupStatus.CurrentReplicas = replicas
	tikvGroup.Status.GroupStatus.UpdatedReplicas = replicas
	tikvGroup.Status.GroupStatus.Selector = fmt.Sprintf("%s=%s,%s=%s",
		v1alpha1.LabelKeyCluster, tikvGroup.Spec.Cluster.Name,
		v1alpha1.LabelKeyGroup, tikvGroup.Name)

	return r.Status().Update(ctx, tikvGroup)
}
