// Copyright 2024 TiKV Project Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// CondSuspended is a condition to display whether the group or instance is suspended
	CondSuspended     = "Suspended"
	ReasonSuspended   = "Suspended"
	ReasonSuspending  = "Suspending"
	ReasonUnsuspended = "Unsuspended"
)

const (
	ReasonUnknown = "Unknown"
	// Ready means all managed resources are ready.
	CondRunning = "Running"
	CondReady   = "Ready"
	// reason for both
	ReasonReady   = CondReady
	ReasonRunning = CondRunning
	// reason for group
	ReasonNotAllInstancesReady = "NotAllInstancesReady"
	// reason for instance
	ReasonPodNotCreated      = "PodNotCreated"
	ReasonPodNotReady        = "PodNotReady"
	ReasonPodNotRunning      = "PodNotRunning"
	ReasonPodTerminating     = "PodTerminating"
	ReasonInstanceNotHealthy = "InstanceNotHealthy"

	// Synced means all specs of managed resources are up to date
	CondSynced                    = "Synced"
	ReasonSynced                  = CondSynced
	ReasonNotAllInstancesUpToDate = "NotAllInstancesUpToDate"
	ReasonPodNotUpToDate          = "PodNotUpToDate"
	ReasonPodNotDeleted           = "PodNotDeleted"
)

const (
	// KeyPrefix defines key prefix of well known labels and annotations
	KeyPrefix = "tikv.org/"

	// LabelKeyManagedBy means resources are managed by tikv operator
	LabelKeyManagedBy         = KeyPrefix + "managed-by"
	LabelValManagedByOperator = "tikv-operator"

	// LabelKeyCluster means which tikv cluster the resource belongs to
	LabelKeyCluster = KeyPrefix + "cluster"
	// LabelKeyComponent means the component of the resource
	LabelKeyComponent = KeyPrefix + "component"
	// LabelKeyGroup means the component group of the resource
	LabelKeyGroup = KeyPrefix + "group"
	// LabelKeyInstance means the instance of the resource
	LabelKeyInstance = KeyPrefix + "instance"

	// LabelKeyPodSpecHash is the hash of the pod spec
	LabelKeyPodSpecHash = KeyPrefix + "pod-spec-hash"

	// LabelKeyInstanceRevisionHash is the revision hash of the instance
	LabelKeyInstanceRevisionHash = KeyPrefix + "instance-revision-hash"

	// LabelKeyConfigHash is the hash of the user-specified config
	LabelKeyConfigHash = KeyPrefix + "config-hash"

	// LabelKeyVolumeName is used to distinguish different volumes
	LabelKeyVolumeName = KeyPrefix + "volume-name"
)

const (
	// Label value for meta.LabelKeyComponent
	LabelValComponentPD   = "pd"
	LabelValComponentTiKV = "tikv"
)

// ConfigUpdateStrategy represents the strategy to update configuration
type ConfigUpdateStrategy string

const (
	// ConfigUpdateStrategyHotReload updates config without restarting
	ConfigUpdateStrategyHotReload ConfigUpdateStrategy = "HotReload"

	// ConfigUpdateStrategyRestart performs a restart to apply changed configs
	ConfigUpdateStrategyRestart ConfigUpdateStrategy = "Restart"
)

// ObjectMeta is defined for replacing the embedded metav1.ObjectMeta
// Now only labels and annotations are allowed
type ObjectMeta struct {
	Name        string            `json:"name,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ClusterReference is a reference to cluster
type ClusterReference struct {
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="cluster name is immutable"
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$
	Name string `json:"name"`
}

// Topology means the topo for scheduling
// e.g. topology.kubernetes.io/zone: us-west-1a
// It will be translated to pod.spec.nodeSelector
// IMPORTANT: Topology is immutable for an instance
// +kubebuilder:validation:MinProperties=1
type Topology map[string]string

// Overlay defines some templates of k8s native resources.
// Users can specify this field to overlay the spec of managed resources(pod, pvcs, ...).
type Overlay struct {
	Pod *PodOverlay `json:"pod,omitempty"`
	// +listType=map
	// +listMapKey=name
	PersistentVolumeClaims []NamedPersistentVolumeClaimOverlay `json:"volumeClaims,omitempty"`
}

type PodOverlay struct {
	ObjectMeta `json:"metadata,omitempty"`
	Spec       *corev1.PodSpec `json:"spec,omitempty"`
}

type NamedPersistentVolumeClaimOverlay struct {
	Name                  string                       `json:"name"`
	PersistentVolumeClaim PersistentVolumeClaimOverlay `json:"volumeClaim"`
}

type PersistentVolumeClaimOverlay struct {
	ObjectMeta `json:"metadata,omitempty"`
	Spec       *corev1.PersistentVolumeClaimSpec `json:"spec,omitempty"`
}

// Volume defines a persistent volume, it will be mounted at a specified root path
type Volume struct {
	Name             string            `json:"name"`
	Mounts           []VolumeMount     `json:"mounts"`
	Storage          resource.Quantity `json:"storage"`
	StorageClassName *string           `json:"storageClassName,omitempty"`
}

type VolumeMount struct {
	Type      VolumeMountType `json:"type"`
	MountPath string          `json:"mountPath,omitempty"`
	SubPath   string          `json:"subPath,omitempty"`
}

type VolumeMountType string

const (
	// VolumeMountTypePDData means data dir of PD
	VolumeMountTypePDData VolumeMountType = "data"
	// VolumeMountTypeTiKVData means data dir of TiKV
	VolumeMountTypeTiKVData VolumeMountType = "data"

	VolumeMountPDDataDefaultPath   = "/var/lib/pd"
	VolumeMountTiKVDataDefaultPath = "/var/lib/tikv"
)

// Port defines a listen port
type Port struct {
	Port int32 `json:"port"`
}

// SchedulePolicy defines how instances of the group schedules its pod
type SchedulePolicy struct {
	Type         SchedulePolicyType          `json:"type"`
	EvenlySpread *SchedulePolicyEvenlySpread `json:"evenlySpread,omitempty"`
}

type SchedulePolicyType string

const (
	SchedulePolicyTypeEvenlySpread = "EvenlySpread"
)

type SchedulePolicyEvenlySpread struct {
	Topologies []ScheduleTopology `json:"topologies"`
}

type ScheduleTopology struct {
	Topology Topology `json:"topology"`
	Weight   *int32   `json:"weight,omitempty"`
}

// ResourceRequirements describes the compute resource requirements
type ResourceRequirements struct {
	CPU    *resource.Quantity `json:"cpu,omitempty"`
	Memory *resource.Quantity `json:"memory,omitempty"`
}

// CommonStatus defines common status fields for instances and groups
type CommonStatus struct {
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	CurrentRevision    string             `json:"currentRevision,omitempty"`
	UpdateRevision     string             `json:"updateRevision,omitempty"`
	CollisionCount     *int32             `json:"collisionCount,omitempty"`
}

// GroupStatus defines the common status fields for all component groups
type GroupStatus struct {
	Version         string `json:"version,omitempty"`
	Selector        string `json:"selector"`
	Replicas        int32  `json:"replicas"`
	ReadyReplicas   int32  `json:"readyReplicas"`
	CurrentReplicas int32  `json:"currentReplicas"`
	UpdatedReplicas int32  `json:"updatedReplicas"`
}

// UpdateStrategy defines the update strategy
type UpdateStrategy struct {
	Config ConfigUpdateStrategy `json:"config,omitempty"`
}

// TLS defines a common tls config for all components
type TLS struct {
	Enabled bool `json:"enabled,omitempty"`
}
