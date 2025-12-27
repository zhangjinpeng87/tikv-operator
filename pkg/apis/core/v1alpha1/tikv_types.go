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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TiKVPortNameClient         = "client"
	TiKVPortNameStatus         = "status"
	DefaultTiKVPortClient      = 20160
	DefaultTiKVPortStatus      = 20180
	DefaultTiKVMinReadySeconds = 5
)

const (
	// LeadersEvicted means all leaders are evicted
	TiKVCondLeadersEvicted = "LeadersEvicted"
	ReasonNotEvicted       = "NotEvicted"
	ReasonEvicting         = "Evicting"
	ReasonEvicted          = "Evicted"
	ReasonStoreIsRemoved   = "StoreIsRemoved"
)

const (
	// Store state
	StoreStatePreparing = "Preparing"
	StoreStateServing   = "Serving"
	StoreStateRemoving  = "Removing"
	StoreStateRemoved   = "Removed"
)

const (
	// StoreOfflinedConditionType represents if the store complete the offline
	StoreOfflinedConditionType = "Offlined"
	ReasonOfflineProcessing    = "Processing"
	ReasonOfflineCanceling     = "Canceling"
	ReasonOfflineCompleted     = "Completed"
	ReasonOfflineFailed        = "Failed"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:resource:categories=group,shortName=kvg
// +kubebuilder:selectablefield:JSONPath=`.spec.cluster.name`
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.cluster.name`
// +kubebuilder:printcolumn:name="Desired",type=string,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Updated",type=string,JSONPath=`.status.updatedReplicas`
// +kubebuilder:printcolumn:name="UpdateRevision",type=string,JSONPath=`.status.updateRevision`
// +kubebuilder:printcolumn:name="CurrentRevision",type=string,JSONPath=`.status.currentRevision`
// +kubebuilder:printcolumn:name="Synced",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// TiKVGroup defines a group of similar TiKV instances
// +kubebuilder:validation:XValidation:rule="size(self.metadata.name) <= 40",message="name must not exceed 40 characters"
type TiKVGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TiKVGroupSpec   `json:"spec,omitempty"`
	Status TiKVGroupStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// TiKVGroupList defines a list of TiKV groups
type TiKVGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []TiKVGroup `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=instance
// +kubebuilder:selectablefield:JSONPath=`.spec.cluster.name`
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.cluster.name`
// +kubebuilder:printcolumn:name="StoreID",type=string,JSONPath=`.status.id`
// +kubebuilder:printcolumn:name="StoreState",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Offline",type=boolean,JSONPath=`.spec.offline`
// +kubebuilder:printcolumn:name="Synced",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// TiKV defines a TiKV instance
// +kubebuilder:validation:XValidation:rule="size(self.metadata.name) <= 47",message="name must not exceed 47 characters"
type TiKV struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TiKVSpec   `json:"spec,omitempty"`
	Status TiKVStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// TiKVList defines a list of TiKV instances
type TiKVList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []TiKV `json:"items"`
}

// TiKVGroupSpec describes the common attributes of a TiKVGroup
type TiKVGroupSpec struct {
	Cluster ClusterReference `json:"cluster"`

	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas"`

	// +listType=map
	// +listMapKey=type
	SchedulePolicies []SchedulePolicy `json:"schedulePolicies,omitempty"`

	Template TiKVTemplate `json:"template"`
}

type TiKVTemplate struct {
	ObjectMeta `json:"metadata,omitempty"`
	Spec       TiKVTemplateSpec `json:"spec"`
}

// TiKVTemplateSpec can only be specified in TiKVGroup
type TiKVTemplateSpec struct {
	// Version must be a semantic version
	Version string `json:"version"`

	// Image is tikv's image, default is pingcap/tikv:v8.5.4
	Image *string `json:"image,omitempty"`

	Resources      ResourceRequirements `json:"resources,omitempty"`
	UpdateStrategy UpdateStrategy       `json:"updateStrategy,omitempty"`

	// Config defines config file of TiKV (TOML format)
	Config string `json:"config,omitempty"`

	// Volumes defines persistent volumes of TiKV
	Volumes []Volume `json:"volumes"`

	// Overlay defines a k8s native resource template patch
	Overlay *Overlay `json:"overlay,omitempty"`
}

type TiKVGroupStatus struct {
	CommonStatus `json:",inline"`
	GroupStatus  `json:",inline"`
}

// TiKVSpec describes the common attributes of a TiKV instance
type TiKVSpec struct {
	Cluster ClusterReference `json:"cluster"`

	// Topology defines the topology domain of this tikv instance
	Topology Topology `json:"topology,omitempty"`

	// Offline marks the store as offline in PD to begin data migration
	Offline bool `json:"offline,omitempty"`

	// TiKVTemplateSpec embedded fields managed by TiKVGroup
	TiKVTemplateSpec `json:",inline"`
}

type TiKVStatus struct {
	CommonStatus `json:",inline"`

	// StoreStatus embedded store status
	StoreStatus `json:",inline"`
}

// StoreStatus defines the common status fields for all stores
type StoreStatus struct {
	// ID is the store id
	ID string `json:"id,omitempty"`

	// State is the store state
	State string `json:"state,omitempty"`
}
