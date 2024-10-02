/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TargetT defines the spec of the target section of a DynamicClusterRole
type TargetT struct {
	Name string `json:"name"`

	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`

	SeparateScopes bool `json:"separateScopes,omitempty"`
}

// DynamicClusterRoleSpec defines the desired state of DynamicClusterRole
type DynamicClusterRoleSpec struct {

	// SynchronizationSpec defines the behavior of synchronization
	Synchronization SynchronizationT `json:"synchronization"`

	//
	Target TargetT             `json:"target"`
	Allow  []rbacv1.PolicyRule `json:"allow"`
	Deny   []rbacv1.PolicyRule `json:"deny"`
}

// DynamicClusterRoleStatus defines the observed state of DynamicClusterRole
type DynamicClusterRoleStatus struct {

	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"ResourceSynced\")].status",description=""
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"ResourceSynced\")].reason",description=""
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description=""

// DynamicClusterRole is the Schema for the dynamicclusterroles API
type DynamicClusterRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DynamicClusterRoleSpec   `json:"spec,omitempty"`
	Status DynamicClusterRoleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DynamicClusterRoleList contains a list of DynamicClusterRole
type DynamicClusterRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DynamicClusterRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DynamicClusterRole{}, &DynamicClusterRoleList{})
}
