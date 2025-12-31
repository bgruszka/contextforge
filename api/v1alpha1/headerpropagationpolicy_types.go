/*
Copyright 2025.

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

// HeaderConfig defines a single header to propagate
type HeaderConfig struct {
	// Name is the HTTP header name to propagate
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9-]+$`
	Name string `json:"name"`

	// Generate indicates whether to auto-generate this header if missing
	// +optional
	Generate bool `json:"generate,omitempty"`

	// GeneratorType specifies how to generate the header value (uuid, ulid, timestamp)
	// +kubebuilder:validation:Enum=uuid;ulid;timestamp
	// +optional
	GeneratorType string `json:"generatorType,omitempty"`

	// Propagate indicates whether to propagate this header to outbound requests
	// +kubebuilder:default=true
	// +optional
	Propagate *bool `json:"propagate,omitempty"`
}

// PropagationRule defines a set of headers and conditions for propagation
type PropagationRule struct {
	// Headers is the list of headers to propagate with this rule
	// +kubebuilder:validation:MinItems=1
	Headers []HeaderConfig `json:"headers"`

	// PathRegex is an optional regex pattern to match request paths
	// +optional
	PathRegex string `json:"pathRegex,omitempty"`

	// Methods is an optional list of HTTP methods this rule applies to
	// +optional
	Methods []string `json:"methods,omitempty"`
}

// HeaderPropagationPolicySpec defines the desired state of HeaderPropagationPolicy
type HeaderPropagationPolicySpec struct {
	// PodSelector selects pods to apply this policy to
	// +optional
	PodSelector *metav1.LabelSelector `json:"podSelector,omitempty"`

	// PropagationRules defines the header propagation rules
	// +kubebuilder:validation:MinItems=1
	PropagationRules []PropagationRule `json:"propagationRules"`
}

// HeaderPropagationPolicyStatus defines the observed state of HeaderPropagationPolicy
type HeaderPropagationPolicyStatus struct {
	// Conditions represent the current state of the HeaderPropagationPolicy resource
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// AppliedToPods is the count of pods this policy is applied to
	// +optional
	AppliedToPods int32 `json:"appliedToPods,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Applied To",type="integer",JSONPath=".status.appliedToPods"

// HeaderPropagationPolicy is the Schema for the headerpropagationpolicies API
type HeaderPropagationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HeaderPropagationPolicySpec   `json:"spec,omitempty"`
	Status HeaderPropagationPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HeaderPropagationPolicyList contains a list of HeaderPropagationPolicy
type HeaderPropagationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HeaderPropagationPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HeaderPropagationPolicy{}, &HeaderPropagationPolicyList{})
}
