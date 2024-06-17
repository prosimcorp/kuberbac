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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MatchRegexT struct {
	Negative   bool   `json:"negative,omitempty"`
	Expression string `json:"expression"`
}

// TODO
type NameSelectorT struct {
	MatchList  []string    `json:"matchList,omitempty"`
	MatchRegex MatchRegexT `json:"matchRegex,omitempty"`
}

// TODO
type NamespaceSelectorT struct {
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
	MatchList   []string          `json:"matchList,omitempty"`
	MatchRegex  MatchRegexT       `json:"matchRegex,omitempty"`
}

// TODO
type DynamicRoleBindingSourceSubject struct {
	ApiGroup string `json:"apiGroup"`
	Kind     string `json:"kind"`

	NameSelector      NameSelectorT      `json:"nameSelector"`
	NamespaceSelector NamespaceSelectorT `json:"namespaceSelector,omitempty"`
}

// TODO
type DynamicRoleBindingSource struct {
	ClusterRole string `json:"clusterRole"`

	Subject DynamicRoleBindingSourceSubject `json:"subject"`
}

// TODO
type DynamicRoleBindingTargets struct {
	NamespaceSelector NamespaceSelectorT `json:"namespaceSelector"`
}

// DynamicRoleBindingSpec defines the desired state of DynamicRoleBinding
type DynamicRoleBindingSpec struct {

	// SynchronizationSpec defines the behavior of synchronization
	Synchronization SynchronizationT `json:"synchronization"`

	//
	Source  DynamicRoleBindingSource  `json:"source"`
	Targets DynamicRoleBindingTargets `json:"targets"`
}

// DynamicRoleBindingStatus defines the observed state of DynamicRoleBinding
type DynamicRoleBindingStatus struct {

	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DynamicRoleBinding is the Schema for the dynamicrolebindings API
type DynamicRoleBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DynamicRoleBindingSpec   `json:"spec,omitempty"`
	Status DynamicRoleBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DynamicRoleBindingList contains a list of DynamicRoleBinding
type DynamicRoleBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DynamicRoleBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DynamicRoleBinding{}, &DynamicRoleBindingList{})
}
