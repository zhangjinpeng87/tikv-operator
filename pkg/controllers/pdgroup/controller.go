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

package pdgroup

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

// PDGroupReconciler reconciles a PDGroup object
type PDGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// Setup sets up the controller with the Manager.
func Setup(mgr manager.Manager) error {
	r := &PDGroupReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    mgr.GetLogger().WithName("pdgroup"),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PDGroup{}).
		Owns(&v1alpha1.PD{}).
		Watches(&v1alpha1.Cluster{}, handler.EnqueueRequestsFromMapFunc(r.enqueueForCluster)).
		WithOptions(controller.Options{}).
		Complete(r)
}

func (r *PDGroupReconciler) enqueueForCluster(ctx context.Context, obj client.Object) []reconcile.Request {
	cluster := obj.(*v1alpha1.Cluster)
	var pdGroupList v1alpha1.PDGroupList
	if err := r.List(ctx, &pdGroupList, client.InNamespace(cluster.Namespace),
		client.MatchingFields{"spec.cluster.name": cluster.Name}); err != nil {
		return []reconcile.Request{}
	}

	requests := []reconcile.Request{}
	for _, pdg := range pdGroupList.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      pdg.Name,
				Namespace: pdg.Namespace,
			},
		})
	}
	return requests
}

// Reconcile reconciles PDGroup by managing PD instances
func (r *PDGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("pdgroup", req.NamespacedName)

	pdGroup := &v1alpha1.PDGroup{}
	if err := r.Get(ctx, req.NamespacedName, pdGroup); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if cluster exists
	cluster := &v1alpha1.Cluster{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: req.Namespace,
		Name:      pdGroup.Spec.Cluster.Name,
	}, cluster); err != nil {
		log.Error(err, "cluster not found")
		return ctrl.Result{}, err
	}

	// List existing PD instances
	var pdList v1alpha1.PDList
	if err := r.List(ctx, &pdList, client.InNamespace(req.Namespace),
		client.MatchingLabels{
			v1alpha1.LabelKeyCluster:   pdGroup.Spec.Cluster.Name,
			v1alpha1.LabelKeyGroup:     pdGroup.Name,
			v1alpha1.LabelKeyComponent: v1alpha1.LabelValComponentPD,
		}); err != nil {
		return ctrl.Result{}, err
	}

	desiredReplicas := int32(0)
	if pdGroup.Spec.Replicas != nil {
		desiredReplicas = *pdGroup.Spec.Replicas
	}
	currentReplicas := int32(len(pdList.Items))

	// Scale out: create new PD instances
	if desiredReplicas > currentReplicas {
		for i := currentReplicas; i < desiredReplicas; i++ {
			pdName := fmt.Sprintf("%s-pd-%d", pdGroup.Name, i)
			pd := r.buildPD(pdGroup, pdName)
			if err := controllerutil.SetControllerReference(pdGroup, pd, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.Create(ctx, pd); err != nil {
				if !errors.IsAlreadyExists(err) {
					log.Error(err, "failed to create PD instance", "name", pdName)
					return ctrl.Result{}, err
				}
			} else {
				log.Info("created PD instance", "name", pdName)
			}
		}
	}

	// Scale in: delete PD instances (in reverse order)
	if desiredReplicas < currentReplicas {
		toDelete := currentReplicas - desiredReplicas
		for i := currentReplicas - 1; i >= desiredReplicas; i-- {
			if toDelete <= 0 {
				break
			}
			pdName := fmt.Sprintf("%s-pd-%d", pdGroup.Name, i)
			pd := &v1alpha1.PD{}
			if err := r.Get(ctx, types.NamespacedName{
				Namespace: req.Namespace,
				Name:      pdName,
			}, pd); err != nil {
				if !errors.IsNotFound(err) {
					log.Error(err, "failed to get PD instance", "name", pdName)
					continue
				}
			} else {
				if err := r.Delete(ctx, pd); err != nil {
					log.Error(err, "failed to delete PD instance", "name", pdName)
					return ctrl.Result{}, err
				}
				log.Info("deleted PD instance", "name", pdName)
				toDelete--
			}
		}
	}

	// Update status
	if err := r.updateStatus(ctx, pdGroup, pdList.Items); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *PDGroupReconciler) buildPD(pdGroup *v1alpha1.PDGroup, name string) *v1alpha1.PD {
	pd := &v1alpha1.PD{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      name,
			Namespace: pdGroup.Namespace,
			Labels: map[string]string{
				v1alpha1.LabelKeyManagedBy: v1alpha1.LabelValManagedByOperator,
				v1alpha1.LabelKeyCluster:   pdGroup.Spec.Cluster.Name,
				v1alpha1.LabelKeyComponent: v1alpha1.LabelValComponentPD,
				v1alpha1.LabelKeyGroup:     pdGroup.Name,
				v1alpha1.LabelKeyInstance:  name,
			},
			// Owner reference will be set when creating
		},
		Spec: v1alpha1.PDSpec{
			Cluster:        pdGroup.Spec.Cluster,
			Subdomain:      fmt.Sprintf("%s-pd", pdGroup.Spec.Cluster.Name),
			PDTemplateSpec: pdGroup.Spec.Template.Spec,
		},
	}
	return pd
}

func (r *PDGroupReconciler) updateStatus(ctx context.Context, pdGroup *v1alpha1.PDGroup, pds []v1alpha1.PD) error {
	readyReplicas := int32(0)
	for _, pd := range pds {
		// Check if PD is ready (simplified check)
		if pd.Status.ID != "" {
			readyReplicas++
		}
	}

	replicas := int32(len(pds))
	pdGroup.Status.CommonStatus.ObservedGeneration = pdGroup.Generation
	pdGroup.Status.GroupStatus.Replicas = replicas
	pdGroup.Status.GroupStatus.ReadyReplicas = readyReplicas
	pdGroup.Status.GroupStatus.CurrentReplicas = replicas
	pdGroup.Status.GroupStatus.UpdatedReplicas = replicas
	pdGroup.Status.GroupStatus.Selector = fmt.Sprintf("%s=%s,%s=%s",
		v1alpha1.LabelKeyCluster, pdGroup.Spec.Cluster.Name,
		v1alpha1.LabelKeyGroup, pdGroup.Name)

	return r.Status().Update(ctx, pdGroup)
}
