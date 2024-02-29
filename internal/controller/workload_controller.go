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

package controller

import (
	"fmt"
	"maps"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	"sigs.k8s.io/kueue/pkg/controller/jobframework"
	"sigs.k8s.io/kueue/pkg/podset"
	utilmaps "sigs.k8s.io/kueue/pkg/util/maps"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
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
	i := 0
	for _, component := range aw.Spec.Components {
		obj := &unstructured.Unstructured{}
		if _, _, err := unstructured.UnstructuredJSONScheme.Decode(component.Template.Raw, nil, obj); err != nil {
			continue
		}

		for _, podSet := range component.PodSets {
			replicas := int32(1)
			if podSet.Replicas != nil {
				replicas = *podSet.Replicas
			}
			template, err := getPodTemplateSpec(obj, podSet.Path)
			if err == nil {
				podSets = append(podSets, kueue.PodSet{
					Name:     aw.Name + "-" + fmt.Sprint(i),
					Template: *template,
					Count:    replicas,
				})
				i++
			}
		}
	}
	return podSets
}

// RunWithPodSetsInfo injects awLabels and Kueue's PodSetInfo into each nested PodTemplateSpec and sets aw.spec.Suspend to false
func (aw *AppWrapper) RunWithPodSetsInfo(podSetsInfo []podset.PodSetInfo) error {
	toMap := func(x interface{}) map[string]string {
		if x == nil {
			return nil
		} else {
			if sm, ok := x.(map[string]string); ok {
				return sm
			} else if im, ok := x.(map[string]interface{}); ok {
				sm := make(map[string]string, len(im))
				for k, v := range im {
					str, ok := v.(string)
					if ok {
						sm[k] = str
					} else {
						sm[k] = fmt.Sprint(v)
					}
				}
				return sm
			} else {
				return nil
			}
		}
	}
	awLabels := map[string]string{AppWrapperLabel: aw.Name}

	podSetsInfoIndex := 0
	for componentIndex := range aw.Spec.Components {
		component := &aw.Spec.Components[componentIndex]
		if len(component.PodSets) == 0 {
			continue // no PodSets; nothing to do for this component
		}

		obj := &unstructured.Unstructured{}
		if _, _, err := unstructured.UnstructuredJSONScheme.Decode(component.Template.Raw, nil, obj); err != nil {
			return err
		}

		for _, podSet := range component.PodSets {
			podSetsInfoIndex += 1
			if podSetsInfoIndex > len(podSetsInfo) {
				continue // we're going to return an error below...just continuing to get an accurate count for the error message
			}
			toInject := podSetsInfo[podSetsInfoIndex-1]

			p, err := getRawTemplate(obj.UnstructuredContent(), podSet.Path)
			if err != nil {
				return err // Should not happen, path validity is enforced by validateAppWrapperInvariants
			}
			if md, ok := p["metadata"]; !ok || md == nil {
				p["metadata"] = make(map[string]interface{})
			}
			metadata := p["metadata"].(map[string]interface{})
			spec := p["spec"].(map[string]interface{}) // Must exist, enforced by validateAppWrapperInvariants

			// Annotations
			if len(toInject.Annotations) > 0 {
				existing := toMap(metadata["annotations"])
				if err := utilmaps.HaveConflict(existing, toInject.Annotations); err != nil {
					return podset.BadPodSetsUpdateError("annotations", err)
				}
				metadata["annotations"] = utilmaps.MergeKeepFirst(existing, toInject.Annotations)
			}

			// Labels
			mergedLabels := utilmaps.MergeKeepFirst(toInject.Labels, awLabels)
			existing := toMap(metadata["labels"])
			if err := utilmaps.HaveConflict(existing, mergedLabels); err != nil {
				return podset.BadPodSetsUpdateError("labels", err)
			}
			metadata["labels"] = utilmaps.MergeKeepFirst(existing, mergedLabels)

			// NodeSelectors
			if len(toInject.NodeSelector) > 0 {
				existing := toMap(metadata["nodeSelector"])
				if err := utilmaps.HaveConflict(existing, toInject.NodeSelector); err != nil {
					return podset.BadPodSetsUpdateError("nodeSelector", err)
				}
				metadata["nodeSelector"] = utilmaps.MergeKeepFirst(existing, toInject.NodeSelector)
			}

			// Tolerations
			if len(toInject.Tolerations) > 0 {
				if _, ok := spec["tolerations"]; !ok {
					spec["tolerations"] = []interface{}{}
				}
				tolerations := spec["tolerations"].([]interface{})
				for _, addition := range toInject.Tolerations {
					tolerations = append(tolerations, addition)
				}
				spec["tolerations"] = tolerations
			}
		}

		// Update the AppWrapper's spec with the modified component
		bytes, err := obj.MarshalJSON()
		if err != nil {
			return err
		}
		component.Template.Raw = bytes
	}

	if podSetsInfoIndex != len(podSetsInfo) {
		return podset.BadPodSetsInfoLenError(podSetsInfoIndex, len(podSetsInfo))
	}

	aw.Spec.Suspend = false

	return nil
}

// RestorePodSetsInfo updates aw.Spec.Components to restore the labels, annotations, nodeSelectors, and tolerations from podSetsInfo
func (aw *AppWrapper) RestorePodSetsInfo(podSetsInfo []podset.PodSetInfo) bool {
	podSetsInfoIndex := 0
	for componentIndex := range aw.Spec.Components {
		component := &aw.Spec.Components[componentIndex]
		if len(component.PodSets) == 0 {
			continue // no PodSets; nothing to do for this component
		}
		obj := &unstructured.Unstructured{}
		if _, _, err := unstructured.UnstructuredJSONScheme.Decode(component.Template.Raw, nil, obj); err != nil {
			continue // Kueue provides no way to indicate that RestorePodSetsInfo hit an error
		}

		for _, podSet := range component.PodSets {
			podSetsInfoIndex += 1
			if podSetsInfoIndex > len(podSetsInfo) {
				continue // Should be impossible; Kueue should only have called RestorePodSetsInfo if RunWithPodSetsInfo returned without an error
			}
			toRestore := podSetsInfo[podSetsInfoIndex-1]
			p, err := getRawTemplate(obj.UnstructuredContent(), podSet.Path)
			if err != nil {
				continue // Kueue provides no way to indicate that RestorePodSetsInfo hit an error
			}

			metadata := p["metadata"].(map[string]interface{}) // Must be non-nil, because we injected a label
			spec := p["spec"].(map[string]interface{})         // Must exist, enforced by validateAppWrapperInvariants

			if len(toRestore.Annotations) > 0 {
				metadata["annotations"] = maps.Clone(toRestore.Annotations)
			} else {
				delete(metadata, "annotations")
			}

			if len(toRestore.Labels) > 0 {
				metadata["labels"] = maps.Clone(toRestore.Labels)
			} else {
				delete(metadata, "labels")
			}

			if len(toRestore.NodeSelector) > 0 {
				spec["nodeSelector"] = maps.Clone(toRestore.NodeSelector)
			} else {
				delete(spec, "nodeSelector")
			}

			if len(toRestore.Tolerations) > 0 {
				tolerations := make([]interface{}, len(toRestore.Tolerations))
				for idx, tol := range toRestore.Tolerations {
					tolerations[idx] = tol
				}
				spec["tolerations"] = tolerations
			} else {
				delete(spec, "tolerations")
			}
		}

		// Update the AppWrapper's spec with the restored component
		bytes, err := obj.MarshalJSON()
		if err != nil {
			continue
		}
		component.Template.Raw = bytes
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
