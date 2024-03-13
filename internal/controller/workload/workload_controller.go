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

package workload

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/controller-runtime/pkg/client"

	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	"sigs.k8s.io/kueue/pkg/controller/jobframework"
	"sigs.k8s.io/kueue/pkg/podset"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	"github.com/project-codeflare/appwrapper/internal/utils"
)

// +kubebuilder:rbac:groups=scheduling.k8s.io,resources=priorityclasses,verbs=list;get;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;watch;update;patch
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads/finalizers,verbs=update
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=resourceflavors,verbs=get;list;watch
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloadpriorityclasses,verbs=get;list;watch

type AppWrapper workloadv1beta2.AppWrapper

var (
	GVK                = workloadv1beta2.GroupVersion.WithKind("AppWrapper")
	WorkloadReconciler = jobframework.NewGenericReconcilerFactory(func() jobframework.GenericJob { return &AppWrapper{} })
)

func (aw *AppWrapper) Object() client.Object {
	return (*workloadv1beta2.AppWrapper)(aw)
}

func (aw *AppWrapper) IsSuspended() bool {
	return aw.Spec.Suspend
}

func (aw *AppWrapper) IsActive() bool {
	return meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.QuotaReserved))
}

func (aw *AppWrapper) Suspend() {
	aw.Spec.Suspend = true
}

func (aw *AppWrapper) GVK() schema.GroupVersionKind {
	return GVK
}

func (aw *AppWrapper) PodSets() []kueue.PodSet {
	podSets := []kueue.PodSet{}
	for componentIdx, component := range aw.Spec.Components {
		if len(component.PodSets) > 0 {
			obj := &unstructured.Unstructured{}
			if _, _, err := unstructured.UnstructuredJSONScheme.Decode(component.Template.Raw, nil, obj); err != nil {
				continue // Should be unreachable; Template.Raw validated by our AdmissionController
			}
			for psIdx, podSet := range component.PodSets {
				replicas := int32(1)
				if podSet.Replicas != nil {
					replicas = *podSet.Replicas
				}
				if template, err := utils.GetPodTemplateSpec(obj, podSet.Path); err == nil {
					podSets = append(podSets, kueue.PodSet{
						Name:     fmt.Sprintf("%s-%v-%v", aw.Name, componentIdx, psIdx),
						Template: *template,
						Count:    replicas,
					})
				}
			}
		}
	}
	return podSets
}

// RunWithPodSetsInfo records the assigned PodSetInfos for each component and sets aw.spec.Suspend to false
func (aw *AppWrapper) RunWithPodSetsInfo(podSetsInfo []podset.PodSetInfo) error {
	podSetsInfoIndex := 0
	for componentIdx := range aw.Spec.Components {
		component := &aw.Spec.Components[componentIdx]
		if len(component.PodSetInfos) != len(component.PodSets) {
			component.PodSetInfos = make([]workloadv1beta2.AppWrapperPodSetInfo, len(component.PodSets))
		}
		for podSetIdx := range component.PodSets {
			podSetsInfoIndex += 1
			if podSetsInfoIndex > len(podSetsInfo) {
				continue // we will return an error below...continuing to get an accurate count for the error message
			}
			component.PodSetInfos[podSetIdx] = workloadv1beta2.AppWrapperPodSetInfo{
				Annotations:  podSetsInfo[podSetIdx].Annotations,
				Labels:       podSetsInfo[podSetIdx].Labels,
				NodeSelector: podSetsInfo[podSetIdx].NodeSelector,
				Tolerations:  podSetsInfo[podSetIdx].Tolerations,
			}
		}
	}

	if podSetsInfoIndex != len(podSetsInfo) {
		return podset.BadPodSetsInfoLenError(podSetsInfoIndex, len(podSetsInfo))
	}

	aw.Spec.Suspend = false

	return nil
}

// RestorePodSetsInfo clears the PodSetInfos saved by RunWithPodSetsInfo
func (aw *AppWrapper) RestorePodSetsInfo(podSetsInfo []podset.PodSetInfo) bool {
	for idx := range aw.Spec.Components {
		aw.Spec.Components[idx].PodSetInfos = nil
	}
	return true
}

func (aw *AppWrapper) Finished() (metav1.Condition, bool) {
	condition := metav1.Condition{
		Type:   kueue.WorkloadFinished,
		Status: metav1.ConditionFalse,
		Reason: string(aw.Status.Phase),
	}

	switch aw.Status.Phase {
	case workloadv1beta2.AppWrapperSucceeded:
		condition.Status = metav1.ConditionTrue
		condition.Message = "AppWrapper finished successfully"
		return condition, true

	case workloadv1beta2.AppWrapperFailed:
		if meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)) {
			condition.Message = "Still deleting resources for failed AppWrapper"
			return condition, false
		} else {
			condition.Status = metav1.ConditionTrue
			condition.Message = "AppWrapper failed"
			return condition, true
		}
	}

	return condition, false
}

func (aw *AppWrapper) PodsReady() bool {
	return meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.PodsReady))
}
