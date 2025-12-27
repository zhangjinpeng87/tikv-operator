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

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="PD",type="integer",JSONPath=".status.components[?(@.kind==\"PD\")].replicas"
// +kubebuilder:printcolumn:name="TiKV",type="integer",JSONPath=".status.components[?(@.kind==\"TiKV\")].replicas"
// +kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.status.conditions[?(@.type=="Available")].status`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Cluster defines a TiKV cluster
// +kubebuilder:validation:XValidation:rule="size(self.metadata.name) <= 37",message="name must not exceed 37 characters"
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// ClusterList defines a list of TiKV clusters
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Cluster `json:"items"`
}

// ClusterSpec defines the desired state of Cluster
type ClusterSpec struct {
	// Paused specifies whether to pause the reconciliation loop for all components
	Paused bool `json:"paused,omitempty"`

	// RevisionHistoryLimit is the maximum number of revisions that will
	// be maintained in each Group's revision history
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// ClusterStatus defines the observed state of Cluster
type ClusterStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Components is the status of each component in the cluster
	Components []ComponentStatus `json:"components,omitempty"`

	// Conditions contains the current status of the cluster
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ID is the cluster id
	ID string `json:"id"`

	// PD means url of the pd service, e.g. http://pd:2379
	PD string `json:"pd,omitempty"`
}

// ComponentKind represents the kind of component
type ComponentKind string

const (
	ComponentKindPD   ComponentKind = "PD"
	ComponentKindTiKV ComponentKind = "TiKV"
)

// ComponentStatus is the status of a component in the cluster
type ComponentStatus struct {
	Kind     ComponentKind `json:"kind"`
	Replicas int32         `json:"replicas"`
}

const (
	// ClusterCondAvailable means the cluster is available
	ClusterCondAvailable = "Available"
	// ClusterCondProgressing means the cluster is progressing
	ClusterCondProgressing = "Progressing"
)
