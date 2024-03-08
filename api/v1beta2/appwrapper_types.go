/*
Copyright 2024 IBM Corporation.

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

package v1beta2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// AppWrapperSpec defines the desired state of the appwrapper
type AppWrapperSpec struct {
	// Components lists the components in the job
	Components []AppWrapperComponent `json:"components"`

	// Suspend suspends the job when set to true
	Suspend bool `json:"suspend,omitempty"`
}

// AppWrapperComponent describes a wrapped resource
type AppWrapperComponent struct {
	// PodSets contained in the component
	PodSets []AppWrapperPodSet `json:"podSets"`

	// PodSetInfos assigned to the Component by Kueue
	PodSetInfos []AppWrapperPodSetInfo `json:"podSetInfos,omitempty"`

	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:EmbeddedResource
	// Template for the component
	Template runtime.RawExtension `json:"template"`
}

// AppWrapperPodSet describes an homogeneous set of pods
type AppWrapperPodSet struct {
	// Replicas is the number of pods in the set
	Replicas *int32 `json:"replicas,omitempty"`

	// Path to the PodTemplateSpec
	Path string `json:"path"`
}

type AppWrapperPodSetInfo struct {
	Annotations  map[string]string   `json:"annotations,omitempty"`
	Labels       map[string]string   `json:"labels,omitempty"`
	NodeSelector map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations  []corev1.Toleration `json:"tolerations,omitempty"`
}

// AppWrapperStatus defines the observed state of the appwrapper
type AppWrapperStatus struct {
	// Phase of the AppWrapper object
	Phase AppWrapperPhase `json:"phase,omitempty"`

	// Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// AppWrapperPhase is the phase of the appwrapper
type AppWrapperPhase string

const (
	AppWrapperEmpty       AppWrapperPhase = ""
	AppWrapperSuspended   AppWrapperPhase = "Suspended"
	AppWrapperResuming    AppWrapperPhase = "Resuming"
	AppWrapperRunning     AppWrapperPhase = "Running"
	AppWrapperSuspending  AppWrapperPhase = "Suspending"
	AppWrapperSucceeded   AppWrapperPhase = "Succeeded"
	AppWrapperFailed      AppWrapperPhase = "Failed"
	AppWrapperTerminating AppWrapperPhase = "Terminating"
)

type AppWrapperCondition string

const (
	QuotaReserved     AppWrapperCondition = "QuotaReserved"
	ResourcesDeployed AppWrapperCondition = "ResourcesDeployed"
	PodsReady         AppWrapperCondition = "PodsReady"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=`.status.phase`

// AppWrapper is the Schema for the appwrappers API
type AppWrapper struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppWrapperSpec   `json:"spec,omitempty"`
	Status AppWrapperStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AppWrapperList contains a list of appwrappers
type AppWrapperList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppWrapper `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AppWrapper{}, &AppWrapperList{})
}
