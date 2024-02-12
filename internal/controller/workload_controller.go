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
	"encoding/json"
	"fmt"
	"maps"
	"strings"

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
	WorkloadReconciler = jobframework.NewGenericReconciler(func() jobframework.GenericJob { return &AppWrapper{} }, nil)
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
		for _, podSet := range component.PodSets {
			replicas := int32(1)
			if podSet.Replicas != nil {
				replicas = *podSet.Replicas
			}
			template, err := getPodTemplateSpec(component.Template.Raw, podSet.Path)
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

func (aw *AppWrapper) RunWithPodSetsInfo(podSetsInfo []podset.PodSetInfo) error {
	aw.Spec.Suspend = false

	// Update aw.Spec.Components to inject our labels and Kueue's PodSetInfo into every nested PodTemplateSpec
	podSetsInfoIndex := 0
	for componentIndex := range aw.Spec.Components {
		component := &aw.Spec.Components[componentIndex]
		objChanged := false
		obj := &unstructured.Unstructured{}
		if _, _, err := unstructured.UnstructuredJSONScheme.Decode(component.Template.Raw, nil, obj); err != nil {
			return err
		}

		for _, podSet := range component.PodSets {
			podSetsInfoIndex += 1
			if podSetsInfoIndex <= len(podSetsInfo) {
				toInject := podSetsInfo[podSetsInfoIndex-1]
				parts := strings.Split(podSet.Path, ".")
				p := obj.UnstructuredContent()
				var ok bool
				for i := 1; i < len(parts); i++ {
					p, ok = p[parts[i]].(map[string]interface{})
					if !ok {
						return fmt.Errorf("path element %v not found (segment %v of %v)", parts[i], i, len(parts))
					}
				}
				objChanged = true // Even if currentInfo is empty, we will still add appWrapperLabel to p.metadata.labels

				if _, ok := p["metadata"]; !ok {
					p["metadata"] = make(map[string]interface{})
				}
				metadata := p["metadata"].(map[string]interface{})

				// Annotations
				if len(toInject.Annotations) > 0 {
					if _, ok := metadata["annotations"]; !ok {
						metadata["annotations"] = make(map[string]string)
					}
					annotations := metadata["annotations"].(map[string]string)
					if err := utilmaps.HaveConflict(annotations, toInject.Annotations); err != nil {
						return podset.BadPodSetsUpdateError("annotations", err)
					}
					metadata["annotations"] = utilmaps.MergeKeepFirst(annotations, toInject.Annotations)
				}

				// Labels
				if _, ok := metadata["labels"]; !ok {
					metadata["labels"] = make(map[string]string)
				}
				labels := metadata["labels"].(map[string]string)
				labels[appWrapperLabel] = aw.Name
				if len(toInject.Labels) > 0 {
					if err := utilmaps.HaveConflict(labels, toInject.Labels); err != nil {
						return podset.BadPodSetsUpdateError("labels", err)
					}
					labels = utilmaps.MergeKeepFirst(labels, toInject.Labels)
				}
				metadata["labels"] = labels

				spec := p["spec"].(map[string]interface{}) // AppWrapper ValidatingAC ensures this succeeds

				// NodeSelectors
				if len(toInject.NodeSelector) > 0 {
					if _, ok := spec["nodeSelector"]; !ok {
						spec["nodeSelector"] = make(map[string]string)
					}
					selector := spec["nodeSelector"].(map[string]string)
					if err := utilmaps.HaveConflict(selector, toInject.NodeSelector); err != nil {
						return podset.BadPodSetsUpdateError("nodeSelector", err)
					}
					spec["nodeSelector"] = utilmaps.MergeKeepFirst(selector, toInject.NodeSelector)
				}

				// Tolerations
				if len(toInject.Tolerations) > 0 {
					if _, ok := spec["tolerations"]; !ok {
						spec["tolerations"] = []interface{}{}
					}
					tolerations := spec["tolerations"].([]interface{})
					for _, addition := range toInject.Tolerations {
						bytes, err := json.Marshal(addition)
						if err != nil {
							return err
						}
						tmp := &unstructured.Unstructured{}
						if _, _, err := unstructured.UnstructuredJSONScheme.Decode(bytes, nil, tmp); err != nil {
							return err
						}
						tolerations = append(tolerations, tmp.UnstructuredContent())
					}
				}
			}
		}

		if objChanged {
			bytes, err := obj.MarshalJSON()
			if err != nil {
				return err
			}
			component.Template.Raw = bytes
		}
	}

	if podSetsInfoIndex != len(podSetsInfo) {
		return podset.BadPodSetsInfoLenError(podSetsInfoIndex, len(podSetsInfo))
	}

	return nil
}

func (aw *AppWrapper) RestorePodSetsInfo(podSetsInfo []podset.PodSetInfo) bool {
	// Update aw.Spec.Components to restore all the saved labels, annotations, nodeSelectors, and tolerations.
	podSetsInfoIndex := 0
	for componentIndex := range aw.Spec.Components {
		component := &aw.Spec.Components[componentIndex]
		objChanged := false
		obj := &unstructured.Unstructured{}
		if _, _, err := unstructured.UnstructuredJSONScheme.Decode(component.Template.Raw, nil, obj); err != nil {
			continue // Kueue provides no way to indicate that RestorePodSetsInfo hit an error
		}

	PODSETLOOP:
		for _, podSet := range component.PodSets {
			podSetsInfoIndex += 1
			if podSetsInfoIndex <= len(podSetsInfo) {
				toRestore := podSetsInfo[podSetsInfoIndex-1]
				parts := strings.Split(podSet.Path, ".")
				p := obj.UnstructuredContent()
				var ok bool
				for i := 1; i < len(parts); i++ {
					p, ok = p[parts[i]].(map[string]interface{})
					if !ok {
						continue PODSETLOOP // Kueue provides no way to indicate that RestorePodSetsInfo hit an error
					}
				}
				objChanged = true // We injected a label into every PodTemplateSpec, so we always have something to remove

				metadata := p["metadata"].(map[string]interface{}) // Must be non-nil, because we injected a label
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

				spec := p["spec"].(map[string]interface{})
				if len(toRestore.Labels) > 0 {
					spec["nodeSelector"] = maps.Clone(toRestore.NodeSelector)
				} else {
					delete(spec, "nodeSelector")
				}

				if len(toRestore.Tolerations) > 0 {
					tolerations := make([]interface{}, len(toRestore.Tolerations))
					for idx, tol := range toRestore.Tolerations {
						bytes, err := json.Marshal(tol)
						if err != nil {
							continue // should be impossible
						}
						tmp := &unstructured.Unstructured{}
						if _, _, err := unstructured.UnstructuredJSONScheme.Decode(bytes, nil, tmp); err != nil {
							continue // should be impossible
						}
						tolerations[idx] = tmp.UnstructuredContent()
					}
					spec["tolerations"] = tolerations
				} else {
					delete(spec, "tolerations")
				}
			}
		}

		if objChanged {
			bytes, err := obj.MarshalJSON()
			if err != nil {
				continue
			}
			component.Template.Raw = bytes
		}
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
	return true // TODO
}
