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

// AppWrapperSpec defines the desired state of the AppWrapper
type AppWrapperSpec struct {
	// Components lists the components contained in the AppWrapper
	Components []AppWrapperComponent `json:"components"`

	// Suspend suspends the AppWrapper when set to true
	//+optional
	Suspend bool `json:"suspend,omitempty"`
}

// AppWrapperComponent describes a single wrapped Kubernetes resource
type AppWrapperComponent struct {
	// Annotations is an unstructured key value map that may be used to store and retrieve
	// arbitrary metadata about the Component to customize its treatment by the AppWrapper controller.
	//+optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// PodSets contained in the Component
	//+optional
	PodSets []AppWrapperPodSet `json:"podSets,omitempty"`

	// PodSetInfos assigned to the Component's PodSets by Kueue
	//+optional
	PodSetInfos []AppWrapperPodSetInfo `json:"podSetInfos,omitempty"`

	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:EmbeddedResource
	// Template defines the Kubernetes resource for the Component
	Template runtime.RawExtension `json:"template"`
}

// AppWrapperPodSet describes an homogeneous set of pods
type AppWrapperPodSet struct {
	// Replicas is the number of pods in this PodSet
	//+optional
	Replicas *int32 `json:"replicas,omitempty"`

	// PodPath is the path Component.Template to the PodTemplateSpec for this PodSet
	PodPath string `json:"podPath"`
}

// AppWrapperPodSetInfo contains the data that Kueue wants to inject into an admitted PodSpecTemplate
type AppWrapperPodSetInfo struct {
	// Annotations to be added to the PodSpecTemplate
	//+optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// Labels to be added to the PodSepcTemplate
	//+optional
	Labels map[string]string `json:"labels,omitempty"`
	// NodeSelectors to be added to the PodSpecTemplate
	//+optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations to be added to the PodSpecTemplate
	//+optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// AppWrapperStatus defines the observed state of the appwrapper
type AppWrapperStatus struct {
	// Phase of the AppWrapper object
	//+optional
	Phase AppWrapperPhase `json:"phase,omitempty"`

	// Retries counts the number of times the AppWrapper has entered the Resetting Phase
	//+optional
	Retries int32 `json:"resettingCount,omitempty"`

	// Conditions hold the latest available observations of the AppWrapper current state.
	//
	// The type of the condition could be:
	//
	// - QuotaReserved: The AppWrapper was admitted by Kueue and has quota allocated to it
	// - ResourcesDeployed: The contained resources are deployed (or being deployed) on the cluster
	// - PodsReady: All pods of the contained resources are in the Ready or Succeeded state
	// - Unhealthy: One or more of the contained resources is unhealthy
	// - DeletingResources: The contained resources are in the process of being deleted from the cluster
	//
	//+optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// AppWrapperPhase is the phase of the appwrapper
type AppWrapperPhase string

const (
	AppWrapperEmpty       AppWrapperPhase = ""
	AppWrapperSuspended   AppWrapperPhase = "Suspended"
	AppWrapperResuming    AppWrapperPhase = "Resuming"
	AppWrapperRunning     AppWrapperPhase = "Running"
	AppWrapperResetting   AppWrapperPhase = "Resetting"
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
	Unhealthy         AppWrapperCondition = "Unhealthy"
	DeletingResources AppWrapperCondition = "DeletingResources"
)

const (
	WarmupGracePeriodDurationAnnotation             = "workload.codeflare.dev.appwrapper/warmupGracePeriodDuration"
	FailureGracePeriodDurationAnnotation            = "workload.codeflare.dev.appwrapper/failureGracePeriodDuration"
	ResetPauseDurationAnnotation                    = "workload.codeflare.dev.appwrapper/resetPauseDuration"
	RetryLimitAnnotation                            = "workload.codeflare.dev.appwrapper/retryLimit"
	DeletionGracePeriodAnnotation                   = "workload.codeflare.dev.appwrapper/deletionGracePeriodDuration"
	DebuggingFailureDeletionDelayDurationAnnotation = "workload.codeflare.dev.appwrapper/debuggingFailureDeletionDelayDuration"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Quota Reserved",type="string",JSONPath=".status.conditions[?(@.type==\"QuotaReserved\")].status"
//+kubebuilder:printcolumn:name="Resources Deployed",type="string",JSONPath=".status.conditions[?(@.type==\"ResourcesDeployed\")].status"
//+kubebuilder:printcolumn:name="Unhealthy",type="string",JSONPath=".status.conditions[?(@.type==\"Unhealthy\")].status"

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
