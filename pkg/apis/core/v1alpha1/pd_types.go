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
	PDPortNameClient         = "client"
	PDPortNamePeer           = "peer"
	DefaultPDPortClient      = 2379
	DefaultPDPortPeer        = 2380
	DefaultPDMinReadySeconds = 5
)

const (
	PDCondInitialized = "Initialized"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:resource:categories=group,shortName=pdg
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

// PDGroup defines a group of similar PD instances
// +kubebuilder:validation:XValidation:rule="size(self.metadata.name) <= 40",message="name must not exceed 40 characters"
type PDGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PDGroupSpec   `json:"spec,omitempty"`
	Status PDGroupStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// PDGroupList defines a list of PD groups
type PDGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []PDGroup `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=instance
// +kubebuilder:selectablefield:JSONPath=`.spec.cluster.name`
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.cluster.name`
// +kubebuilder:printcolumn:name="Leader",type=string,JSONPath=`.status.isLeader`
// +kubebuilder:printcolumn:name="Synced",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PD defines a PD instance
// +kubebuilder:validation:XValidation:rule="size(self.metadata.name) <= 47",message="name must not exceed 47 characters"
type PD struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PDSpec   `json:"spec,omitempty"`
	Status PDStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// PDList defines a list of PD instances
type PDList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []PD `json:"items"`
}

// PDGroupSpec describes the common attributes of a PDGroup
type PDGroupSpec struct {
	Cluster ClusterReference `json:"cluster"`

	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas"`

	// Bootstrapped means that pd cluster has been bootstrapped
	Bootstrapped bool `json:"bootstrapped,omitempty"`

	// +listType=map
	// +listMapKey=type
	SchedulePolicies []SchedulePolicy `json:"schedulePolicies,omitempty"`

	Template PDTemplate `json:"template"`
}

type PDTemplate struct {
	ObjectMeta `json:"metadata,omitempty"`
	Spec       PDTemplateSpec `json:"spec"`
}

// PDTemplateSpec can only be specified in PDGroup
type PDTemplateSpec struct {
	// Version must be a semantic version
	Version string `json:"version"`

	// Image is pd's image, default is pingcap/pd
	Image *string `json:"image,omitempty"`

	Resources      ResourceRequirements `json:"resources,omitempty"`
	UpdateStrategy UpdateStrategy       `json:"updateStrategy,omitempty"`

	// Config defines config file of PD (TOML format)
	Config string `json:"config,omitempty"`

	// Volumes defines persistent volumes of PD
	Volumes []Volume `json:"volumes"`

	// Overlay defines a k8s native resource template patch
	Overlay *Overlay `json:"overlay,omitempty"`
}

type PDGroupStatus struct {
	CommonStatus `json:",inline"`
	GroupStatus  `json:",inline"`
}

// PDSpec describes the common attributes of a PD instance
type PDSpec struct {
	Cluster ClusterReference `json:"cluster"`

	// Topology defines the topology domain of this pd instance
	Topology Topology `json:"topology,omitempty"`

	// Subdomain means the subdomain of the exported pd dns
	Subdomain string `json:"subdomain"`

	// PDTemplateSpec embedded fields managed by PDGroup
	PDTemplateSpec `json:",inline"`
}

type PDStatus struct {
	CommonStatus `json:",inline"`

	// ID is the member id of this pd instance
	ID string `json:"id"`

	// IsLeader indicates whether this pd is the leader
	IsLeader bool `json:"isLeader"`
}
