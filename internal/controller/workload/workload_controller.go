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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	"sigs.k8s.io/kueue/pkg/controller/jobframework"
	"sigs.k8s.io/kueue/pkg/podset"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	"github.com/project-codeflare/appwrapper/pkg/utils"
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
	WorkloadReconciler = jobframework.NewGenericReconcilerFactory(
		func() jobframework.GenericJob { return &AppWrapper{} },
		func(b *builder.Builder, c client.Client) *builder.Builder {
			return b.Named("AppWrapperWorkload")
		},
	)
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
	if err := utils.EnsureComponentStatusInitialized((*workloadv1beta2.AppWrapper)(aw)); err != nil {
		// Kueue will raise an error on zero length PodSet.  Unfortunately, the Kueue API prevents propagating the actual error
		return podSets
	}
	for idx := range aw.Status.ComponentStatus {
		if len(aw.Status.ComponentStatus[idx].PodSets) > 0 {
			obj := &unstructured.Unstructured{}
			if _, _, err := unstructured.UnstructuredJSONScheme.Decode(aw.Spec.Components[idx].Template.Raw, nil, obj); err != nil {
				// Should be unreachable; Template.Raw validated by AppWrapper AdmissionController
				return []kueue.PodSet{} // Kueue will raise an error on zero length PodSet.
			}
			for psIdx, podSet := range aw.Status.ComponentStatus[idx].PodSets {
				replicas := utils.Replicas(podSet)
				if template, err := utils.GetPodTemplateSpec(obj, podSet.Path); err == nil {
					podSets = append(podSets, kueue.PodSet{
						Name:     fmt.Sprintf("%s-%v-%v", aw.Name, idx, psIdx),
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
	if err := utils.EnsureComponentStatusInitialized((*workloadv1beta2.AppWrapper)(aw)); err != nil {
		return err
	}
	podSetsInfoIndex := 0
	for idx := range aw.Spec.Components {
		if len(aw.Spec.Components[idx].PodSetInfos) != len(aw.Status.ComponentStatus[idx].PodSets) {
			aw.Spec.Components[idx].PodSetInfos = make([]workloadv1beta2.AppWrapperPodSetInfo, len(aw.Status.ComponentStatus[idx].PodSets))
		}
		for podSetIdx := range aw.Status.ComponentStatus[idx].PodSets {
			podSetsInfoIndex += 1
			if podSetsInfoIndex > len(podSetsInfo) {
				continue // we will return an error below...continuing to get an accurate count for the error message
			}
			aw.Spec.Components[idx].PodSetInfos[podSetIdx] = workloadv1beta2.AppWrapperPodSetInfo{
				Annotations:  podSetsInfo[podSetsInfoIndex-1].Annotations,
				Labels:       podSetsInfo[podSetsInfoIndex-1].Labels,
				NodeSelector: podSetsInfo[podSetsInfoIndex-1].NodeSelector,
				Tolerations:  podSetsInfo[podSetsInfoIndex-1].Tolerations,
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

func (aw *AppWrapper) Finished() (message string, success, finished bool) {
	switch aw.Status.Phase {
	case workloadv1beta2.AppWrapperSucceeded:
		return "AppWrapper finished successfully", true, true

	case workloadv1beta2.AppWrapperFailed:
		if meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)) {
			return "Still deleting resources for failed AppWrapper", false, false
		} else {
			return "AppWrapper failed", false, true
		}
	}
	return "", false, false
}

func (aw *AppWrapper) PodsReady() bool {
	return meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.PodsReady))
}
