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

package tikv

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/zhangjinpeng87/tikv-operator/pkg/apis/core/v1alpha1"
)

// TiKVReconciler reconciles a TiKV instance by managing its Pod, ConfigMap, and PVCs
type TiKVReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// Setup sets up the controller with the Manager.
func Setup(mgr manager.Manager) error {
	r := &TiKVReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    mgr.GetLogger().WithName("tikv"),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.TiKV{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		WithOptions(controller.Options{}).
		Complete(r)
}

// Reconcile manages Pod, ConfigMap, and PVCs for a TiKV instance
func (r *TiKVReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("tikv", req.NamespacedName)

	tikv := &v1alpha1.TiKV{}
	if err := r.Get(ctx, req.NamespacedName, tikv); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if TiKV is being deleted
	if !tikv.DeletionTimestamp.IsZero() {
		// Handle finalizers if needed
		return ctrl.Result{}, nil
	}

	// Ensure Service exists (headless service for TiKV cluster)
	if err := r.reconcileService(ctx, tikv); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure ConfigMap exists
	if err := r.reconcileConfigMap(ctx, tikv); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure PVCs exist
	if err := r.reconcilePVCs(ctx, tikv); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure Pod exists
	if err := r.reconcilePod(ctx, tikv); err != nil {
		return ctrl.Result{}, err
	}

	// Update status from Pod
	if err := r.updateStatus(ctx, tikv); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *TiKVReconciler) reconcileService(ctx context.Context, tikv *v1alpha1.TiKV) error {
	svcName := fmt.Sprintf("%s-tikv", tikv.Spec.Cluster.Name)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: tikv.Namespace,
		},
	}

	op, err := ctrl.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = map[string]string{
			v1alpha1.LabelKeyManagedBy: v1alpha1.LabelValManagedByOperator,
			v1alpha1.LabelKeyCluster:   tikv.Spec.Cluster.Name,
			v1alpha1.LabelKeyComponent: v1alpha1.LabelValComponentTiKV,
		}
		svc.Spec = corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone, // Headless service
			Selector: map[string]string{
				v1alpha1.LabelKeyCluster:   tikv.Spec.Cluster.Name,
				v1alpha1.LabelKeyComponent: v1alpha1.LabelValComponentTiKV,
			},
			Ports: []corev1.ServicePort{
				{Name: v1alpha1.TiKVPortNameClient, Port: v1alpha1.DefaultTiKVPortClient},
				{Name: v1alpha1.TiKVPortNameStatus, Port: v1alpha1.DefaultTiKVPortStatus},
			},
		}
		// Don't set owner reference for service (shared by all TiKV instances)
		return nil
	})

	if err != nil {
		return err
	}
	if op != ctrl.OperationResultNone {
		r.Log.Info("Service reconciled", "operation", op, "name", svcName)
	}
	return nil
}

func (r *TiKVReconciler) reconcileConfigMap(ctx context.Context, tikv *v1alpha1.TiKV) error {
	cmName := fmt.Sprintf("%s-config", tikv.Name)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: tikv.Namespace,
		},
	}

	op, err := ctrl.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Labels = map[string]string{
			v1alpha1.LabelKeyManagedBy: v1alpha1.LabelValManagedByOperator,
			v1alpha1.LabelKeyCluster:   tikv.Spec.Cluster.Name,
			v1alpha1.LabelKeyComponent: v1alpha1.LabelValComponentTiKV,
			v1alpha1.LabelKeyInstance:  tikv.Name,
		}
		cm.Data = map[string]string{
			"config-file": tikv.Spec.Config,
		}
		return controllerutil.SetControllerReference(tikv, cm, r.Scheme)
	})

	if err != nil {
		return err
	}
	if op != ctrl.OperationResultNone {
		r.Log.Info("ConfigMap reconciled", "operation", op, "name", cmName)
	}
	return nil
}

func (r *TiKVReconciler) reconcilePVCs(ctx context.Context, tikv *v1alpha1.TiKV) error {
	for _, vol := range tikv.Spec.Volumes {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", tikv.Name, vol.Name),
				Namespace: tikv.Namespace,
			},
		}

		op, err := ctrl.CreateOrUpdate(ctx, r.Client, pvc, func() error {
			pvc.Labels = map[string]string{
				v1alpha1.LabelKeyManagedBy:  v1alpha1.LabelValManagedByOperator,
				v1alpha1.LabelKeyCluster:    tikv.Spec.Cluster.Name,
				v1alpha1.LabelKeyComponent:  v1alpha1.LabelValComponentTiKV,
				v1alpha1.LabelKeyInstance:   tikv.Name,
				v1alpha1.LabelKeyVolumeName: vol.Name,
			}
			pvc.Spec = corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: vol.Storage,
					},
				},
			}
			if vol.StorageClassName != nil {
				pvc.Spec.StorageClassName = vol.StorageClassName
			}
			return controllerutil.SetControllerReference(tikv, pvc, r.Scheme)
		})

		if err != nil {
			return err
		}
		if op != ctrl.OperationResultNone {
			r.Log.Info("PVC reconciled", "operation", op, "name", pvc.Name)
		}
	}
	return nil
}

func (r *TiKVReconciler) reconcilePod(ctx context.Context, tikv *v1alpha1.TiKV) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tikv.Name,
			Namespace: tikv.Namespace,
		},
	}

	op, err := ctrl.CreateOrUpdate(ctx, r.Client, pod, func() error {
		// Labels
		pod.Labels = map[string]string{
			v1alpha1.LabelKeyManagedBy: v1alpha1.LabelValManagedByOperator,
			v1alpha1.LabelKeyCluster:   tikv.Spec.Cluster.Name,
			v1alpha1.LabelKeyComponent: v1alpha1.LabelValComponentTiKV,
			v1alpha1.LabelKeyInstance:  tikv.Name,
		}

		// Image
		image := "pingcap/tikv:v8.5.4"
		if tikv.Spec.Image != nil {
			image = *tikv.Spec.Image
		} else if tikv.Spec.Version != "" {
			image = fmt.Sprintf("pingcap/tikv:%s", tikv.Spec.Version)
		}

		// Container
		container := corev1.Container{
			Name:  "tikv",
			Image: image,
			Ports: []corev1.ContainerPort{
				{Name: v1alpha1.TiKVPortNameClient, ContainerPort: v1alpha1.DefaultTiKVPortClient},
				{Name: v1alpha1.TiKVPortNameStatus, ContainerPort: v1alpha1.DefaultTiKVPortStatus},
			},
			Command: []string{
				"/tikv-server",
				"--addr=0.0.0.0:20160",
				"--advertise-addr=$(POD_NAME).$(HEADLESS_SERVICE):20160",
				"--status-addr=0.0.0.0:20180",
				"--pd=$(PD_SERVICE):2379",
				"--data-dir=/var/lib/tikv",
				"--config=/etc/tikv/tikv.toml",
			},
			Env: []corev1.EnvVar{
				{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
				}},
				{Name: "HEADLESS_SERVICE", Value: fmt.Sprintf("%s-tikv", tikv.Spec.Cluster.Name)},
				{Name: "PD_SERVICE", Value: tikv.Spec.Cluster.Name + "-pd"},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "config", MountPath: "/etc/tikv"},
			},
			Resources: corev1.ResourceRequirements{},
		}

		// Resources
		if tikv.Spec.Resources.CPU != nil {
			container.Resources.Requests[corev1.ResourceCPU] = *tikv.Spec.Resources.CPU
			container.Resources.Limits[corev1.ResourceCPU] = *tikv.Spec.Resources.CPU
		}
		if tikv.Spec.Resources.Memory != nil {
			container.Resources.Requests[corev1.ResourceMemory] = *tikv.Spec.Resources.Memory
			container.Resources.Limits[corev1.ResourceMemory] = *tikv.Spec.Resources.Memory
		}

		// Volume mounts for data volumes
		for _, vol := range tikv.Spec.Volumes {
			for _, mount := range vol.Mounts {
				mountPath := mount.MountPath
				if mountPath == "" {
					if mount.Type == v1alpha1.VolumeMountTypeTiKVData {
						mountPath = v1alpha1.VolumeMountTiKVDataDefaultPath
					}
				}
				container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
					Name:      vol.Name,
					MountPath: mountPath,
					SubPath:   mount.SubPath,
				})
			}
		}

		pod.Spec = corev1.PodSpec{
			Containers: []corev1.Container{container},
			Volumes: []corev1.Volume{
				{
					Name: "config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: fmt.Sprintf("%s-config", tikv.Name),
							},
						},
					},
				},
			},
		}

		// Add PVC volumes
		for _, vol := range tikv.Spec.Volumes {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: vol.Name,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: fmt.Sprintf("%s-%s", tikv.Name, vol.Name),
					},
				},
			})
		}

		// Restart policy
		pod.Spec.RestartPolicy = corev1.RestartPolicyAlways

		return controllerutil.SetControllerReference(tikv, pod, r.Scheme)
	})

	if err != nil {
		return err
	}
	if op != ctrl.OperationResultNone {
		r.Log.Info("Pod reconciled", "operation", op, "name", pod.Name)
	}
	return nil
}

func (r *TiKVReconciler) updateStatus(ctx context.Context, tikv *v1alpha1.TiKV) error {
	pod := &corev1.Pod{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: tikv.Namespace,
		Name:      tikv.Name,
	}, pod); err != nil {
		// Pod not found, update status accordingly
		tikv.Status.CommonStatus.ObservedGeneration = tikv.Generation
		return r.Status().Update(ctx, tikv)
	}

	// Update status based on Pod status
	tikv.Status.CommonStatus.ObservedGeneration = tikv.Generation

	// Check if Pod is ready
	if pod.Status.Phase == corev1.PodRunning {
		ready := false
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if ready {
			// TODO: Query PD API to get store ID and state
			// For now, set a placeholder
			if tikv.Status.ID == "" {
				tikv.Status.ID = "pending"
				tikv.Status.State = v1alpha1.StoreStatePreparing
			}
		}
	} else {
		// Pod is not running, clear status
		tikv.Status.ID = ""
		tikv.Status.State = ""
	}

	return r.Status().Update(ctx, tikv)
}
