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

package pd

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

// PDReconciler reconciles a PD instance by managing its Pod, ConfigMap, and PVCs
type PDReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// Setup sets up the controller with the Manager.
func Setup(mgr manager.Manager) error {
	r := &PDReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    mgr.GetLogger().WithName("pd"),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PD{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		WithOptions(controller.Options{}).
		Complete(r)
}

// Reconcile manages Pod, ConfigMap, and PVCs for a PD instance
func (r *PDReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("pd", req.NamespacedName)

	pd := &v1alpha1.PD{}
	if err := r.Get(ctx, req.NamespacedName, pd); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if PD is being deleted
	if !pd.DeletionTimestamp.IsZero() {
		// Handle finalizers if needed
		return ctrl.Result{}, nil
	}

	// Ensure Service exists (headless service for PD cluster)
	if err := r.reconcileService(ctx, pd); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure ConfigMap exists
	if err := r.reconcileConfigMap(ctx, pd); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure PVCs exist
	if err := r.reconcilePVCs(ctx, pd); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure Pod exists
	if err := r.reconcilePod(ctx, pd); err != nil {
		return ctrl.Result{}, err
	}

	// Update status from Pod
	if err := r.updateStatus(ctx, pd); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *PDReconciler) reconcileService(ctx context.Context, pd *v1alpha1.PD) error {
	// Service name is based on cluster name, not subdomain
	svcName := fmt.Sprintf("%s-pd", pd.Spec.Cluster.Name)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: pd.Namespace,
		},
	}

	op, err := ctrl.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = map[string]string{
			v1alpha1.LabelKeyManagedBy: v1alpha1.LabelValManagedByOperator,
			v1alpha1.LabelKeyCluster:   pd.Spec.Cluster.Name,
			v1alpha1.LabelKeyComponent: v1alpha1.LabelValComponentPD,
		}
		svc.Spec = corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone, // Headless service
			Selector: map[string]string{
				v1alpha1.LabelKeyCluster:   pd.Spec.Cluster.Name,
				v1alpha1.LabelKeyComponent: v1alpha1.LabelValComponentPD,
			},
			Ports: []corev1.ServicePort{
				{Name: v1alpha1.PDPortNameClient, Port: v1alpha1.DefaultPDPortClient},
				{Name: v1alpha1.PDPortNamePeer, Port: v1alpha1.DefaultPDPortPeer},
			},
		}
		// Don't set owner reference for service (shared by all PD instances)
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

func (r *PDReconciler) reconcileConfigMap(ctx context.Context, pd *v1alpha1.PD) error {
	cmName := fmt.Sprintf("%s-config", pd.Name)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: pd.Namespace,
		},
	}

	op, err := ctrl.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Labels = map[string]string{
			v1alpha1.LabelKeyManagedBy: v1alpha1.LabelValManagedByOperator,
			v1alpha1.LabelKeyCluster:   pd.Spec.Cluster.Name,
			v1alpha1.LabelKeyComponent: v1alpha1.LabelValComponentPD,
			v1alpha1.LabelKeyInstance:  pd.Name,
		}
		cm.Data = map[string]string{
			"config-file": pd.Spec.Config,
		}
		return controllerutil.SetControllerReference(pd, cm, r.Scheme)
	})

	if err != nil {
		return err
	}
	if op != ctrl.OperationResultNone {
		r.Log.Info("ConfigMap reconciled", "operation", op, "name", cmName)
	}
	return nil
}

func (r *PDReconciler) reconcilePVCs(ctx context.Context, pd *v1alpha1.PD) error {
	for _, vol := range pd.Spec.Volumes {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", pd.Name, vol.Name),
				Namespace: pd.Namespace,
			},
		}

		op, err := ctrl.CreateOrUpdate(ctx, r.Client, pvc, func() error {
			pvc.Labels = map[string]string{
				v1alpha1.LabelKeyManagedBy:  v1alpha1.LabelValManagedByOperator,
				v1alpha1.LabelKeyCluster:    pd.Spec.Cluster.Name,
				v1alpha1.LabelKeyComponent:  v1alpha1.LabelValComponentPD,
				v1alpha1.LabelKeyInstance:   pd.Name,
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
			return controllerutil.SetControllerReference(pd, pvc, r.Scheme)
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

func (r *PDReconciler) reconcilePod(ctx context.Context, pd *v1alpha1.PD) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pd.Name,
			Namespace: pd.Namespace,
		},
	}

	op, err := ctrl.CreateOrUpdate(ctx, r.Client, pod, func() error {
		// Labels
		pod.Labels = map[string]string{
			v1alpha1.LabelKeyManagedBy: v1alpha1.LabelValManagedByOperator,
			v1alpha1.LabelKeyCluster:   pd.Spec.Cluster.Name,
			v1alpha1.LabelKeyComponent: v1alpha1.LabelValComponentPD,
			v1alpha1.LabelKeyInstance:  pd.Name,
		}

		// Image
		image := "pingcap/pd:latest"
		if pd.Spec.Image != nil {
			image = *pd.Spec.Image
		} else if pd.Spec.Version != "" {
			image = fmt.Sprintf("pingcap/pd:%s", pd.Spec.Version)
		}

		// Container
		container := corev1.Container{
			Name:  "pd",
			Image: image,
			Ports: []corev1.ContainerPort{
				{Name: v1alpha1.PDPortNameClient, ContainerPort: v1alpha1.DefaultPDPortClient},
				{Name: v1alpha1.PDPortNamePeer, ContainerPort: v1alpha1.DefaultPDPortPeer},
			},
			Command: []string{
				"/pd-server",
				"--name=$(POD_NAME)",
				"--client-urls=http://0.0.0.0:2379",
				"--peer-urls=http://0.0.0.0:2380",
				"--advertise-client-urls=http://$(POD_NAME).$(PD_SERVICE):2379",
				"--advertise-peer-urls=http://$(POD_NAME).$(PD_SERVICE):2380",
				"--data-dir=/var/lib/pd",
				"--config=/etc/pd/pd.toml",
			},
			Env: []corev1.EnvVar{
				{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
				}},
				{Name: "PD_SERVICE", Value: svcName},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "config", MountPath: "/etc/pd"},
			},
			Resources: corev1.ResourceRequirements{},
		}

		// Resources
		if pd.Spec.Resources.CPU != nil {
			container.Resources.Requests[corev1.ResourceCPU] = *pd.Spec.Resources.CPU
			container.Resources.Limits[corev1.ResourceCPU] = *pd.Spec.Resources.CPU
		}
		if pd.Spec.Resources.Memory != nil {
			container.Resources.Requests[corev1.ResourceMemory] = *pd.Spec.Resources.Memory
			container.Resources.Limits[corev1.ResourceMemory] = *pd.Spec.Resources.Memory
		}

		// Volume mounts for data volumes
		for _, vol := range pd.Spec.Volumes {
			for _, mount := range vol.Mounts {
				mountPath := mount.MountPath
				if mountPath == "" {
					if mount.Type == v1alpha1.VolumeMountTypePDData {
						mountPath = v1alpha1.VolumeMountPDDataDefaultPath
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
								Name: fmt.Sprintf("%s-config", pd.Name),
							},
						},
					},
				},
			},
		}

		// Add PVC volumes
		for _, vol := range pd.Spec.Volumes {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: vol.Name,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: fmt.Sprintf("%s-%s", pd.Name, vol.Name),
					},
				},
			})
		}

		// Restart policy
		pod.Spec.RestartPolicy = corev1.RestartPolicyAlways

		return controllerutil.SetControllerReference(pd, pod, r.Scheme)
	})

	if err != nil {
		return err
	}
	if op != ctrl.OperationResultNone {
		r.Log.Info("Pod reconciled", "operation", op, "name", pod.Name)
	}
	return nil
}

func (r *PDReconciler) updateStatus(ctx context.Context, pd *v1alpha1.PD) error {
	pod := &corev1.Pod{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: pd.Namespace,
		Name:      pd.Name,
	}, pod); err != nil {
		// Pod not found, update status accordingly
		pd.Status.ObservedGeneration = pd.Generation
		return r.Status().Update(ctx, pd)
	}

	// Update status based on Pod status
	pd.Status.CommonStatus.ObservedGeneration = pd.Generation

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
			// TODO: Query PD API to get member ID and leader status
			// For now, set a placeholder
			if pd.Status.ID == "" {
				pd.Status.ID = "pending"
			}
		}
	} else {
		// Pod is not running, clear ID
		pd.Status.ID = ""
		pd.Status.IsLeader = false
	}

	return r.Status().Update(ctx, pd)
}
