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

package appwrapper

import (
	"context"
	"fmt"
	"time"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	"github.com/project-codeflare/appwrapper/pkg/utils"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/kueue/pkg/controller/constants"
	"sigs.k8s.io/kueue/pkg/podset"
	utilmaps "sigs.k8s.io/kueue/pkg/util/maps"
)

func parseComponent(raw []byte, expectedNamespace string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	if _, _, err := unstructured.UnstructuredJSONScheme.Decode(raw, nil, obj); err != nil {
		return nil, err
	}
	namespace := obj.GetNamespace()
	if namespace == "" {
		obj.SetNamespace(expectedNamespace)
	} else if namespace != expectedNamespace {
		// Should not happen, namespace equality checked by validateAppWrapperInvariants
		return nil, fmt.Errorf("component namespace \"%s\" is different from appwrapper namespace \"%s\"", namespace, expectedNamespace)
	}
	return obj, nil
}

//gocyclo:ignore
func (r *AppWrapperReconciler) createComponent(ctx context.Context, aw *workloadv1beta2.AppWrapper, componentIdx int) (*unstructured.Unstructured, error, bool) {
	component := aw.Spec.Components[componentIdx]
	componentStatus := aw.Status.ComponentStatus[componentIdx]
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

	obj, err := parseComponent(component.Template.Raw, aw.Namespace)
	if err != nil {
		return nil, err, true
	}
	if r.Config.EnableKueueIntegrations && !r.Config.DisableChildAdmissionCtrl {
		obj.SetLabels(utilmaps.MergeKeepFirst(obj.GetLabels(), map[string]string{AppWrapperLabel: aw.Name, constants.QueueLabel: childJobQueueName}))
	} else {
		obj.SetLabels(utilmaps.MergeKeepFirst(obj.GetLabels(), map[string]string{AppWrapperLabel: aw.Name}))
	}

	awLabels := map[string]string{AppWrapperLabel: aw.Name}
	for podSetsIdx, podSet := range componentStatus.PodSets {
		toInject := &workloadv1beta2.AppWrapperPodSetInfo{}
		if r.Config.EnableKueueIntegrations {
			if podSetsIdx < len(component.PodSetInfos) {
				toInject = &component.PodSetInfos[podSetsIdx]
			} else {
				return nil, fmt.Errorf("missing podSetInfo %v for component %v", podSetsIdx, componentIdx), true
			}
		}

		p, err := utils.GetRawTemplate(obj.UnstructuredContent(), podSet.Path)
		if err != nil {
			return nil, err, true // Should not happen, path validity is enforced by validateAppWrapperInvariants
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
				return nil, podset.BadPodSetsUpdateError("annotations", err), true
			}
			metadata["annotations"] = utilmaps.MergeKeepFirst(existing, toInject.Annotations)
		}

		// Labels
		mergedLabels := utilmaps.MergeKeepFirst(toInject.Labels, awLabels)
		existing := toMap(metadata["labels"])
		if err := utilmaps.HaveConflict(existing, mergedLabels); err != nil {
			return nil, podset.BadPodSetsUpdateError("labels", err), true
		}
		metadata["labels"] = utilmaps.MergeKeepFirst(existing, mergedLabels)

		// NodeSelectors
		if len(toInject.NodeSelector) > 0 {
			existing := toMap(spec["nodeSelector"])
			if err := utilmaps.HaveConflict(existing, toInject.NodeSelector); err != nil {
				return nil, podset.BadPodSetsUpdateError("nodeSelector", err), true
			}
			spec["nodeSelector"] = utilmaps.MergeKeepFirst(existing, toInject.NodeSelector)
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

		// Scheduler Name
		if r.Config.SchedulerName != "" {
			if existing, _ := spec["schedulerName"].(string); existing == "" {
				spec["schedulerName"] = r.Config.SchedulerName
			}
		}
	}

	if err := controllerutil.SetControllerReference(aw, obj, r.Scheme); err != nil {
		return nil, err, true
	}

	if err := r.Create(ctx, obj); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// obj is not updated if Create returns an error; Get required for accurate information
			if err := r.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
				return nil, err, false
			}
			ctrlRef := metav1.GetControllerOf(obj)
			if ctrlRef == nil || ctrlRef.Name != aw.Name {
				return nil, fmt.Errorf("resource %v exists, but is not controlled by appwrapper", obj.GetName()), true
			}
			return obj, nil, false // ok -- already exists and the correct appwrapper owns it.
		} else {
			return nil, err, meta.IsNoMatchError(err) || apierrors.IsInvalid(err) // fatal
		}
	}

	return obj, nil, false
}

func (r *AppWrapperReconciler) createComponents(ctx context.Context, aw *workloadv1beta2.AppWrapper) (error, bool) {
	for componentIdx := range aw.Spec.Components {
		if !meta.IsStatusConditionTrue(aw.Status.ComponentStatus[componentIdx].Conditions, string(workloadv1beta2.ResourcesDeployed)) {
			obj, err, fatal := r.createComponent(ctx, aw, componentIdx)
			if err != nil {
				return err, fatal
			}
			aw.Status.ComponentStatus[componentIdx].Name = obj.GetName()
			aw.Status.ComponentStatus[componentIdx].Kind = obj.GetKind()
			aw.Status.ComponentStatus[componentIdx].APIVersion = obj.GetAPIVersion()
			meta.SetStatusCondition(&aw.Status.ComponentStatus[componentIdx].Conditions, metav1.Condition{
				Type:   string(workloadv1beta2.ResourcesDeployed),
				Status: metav1.ConditionTrue,
				Reason: "CompononetCreated",
			})
		}
	}
	return nil, false
}

func (r *AppWrapperReconciler) deleteComponents(ctx context.Context, aw *workloadv1beta2.AppWrapper) bool {
	deleteIfPresent := func(idx int, opts ...client.DeleteOption) bool {
		cs := &aw.Status.ComponentStatus[idx]
		if !meta.IsStatusConditionTrue(cs.Conditions, string(workloadv1beta2.ResourcesDeployed)) {
			return false // not present
		}
		obj := &metav1.PartialObjectMetadata{
			TypeMeta:   metav1.TypeMeta{Kind: cs.Kind, APIVersion: cs.APIVersion},
			ObjectMeta: metav1.ObjectMeta{Name: cs.Name, Namespace: aw.Namespace},
		}
		if err := r.Delete(ctx, obj, opts...); err != nil {
			if apierrors.IsNotFound(err) {
				// Has already been undeployed; update componentStatus and return not present
				meta.SetStatusCondition(&cs.Conditions, metav1.Condition{
					Type:   string(workloadv1beta2.ResourcesDeployed),
					Status: metav1.ConditionFalse,
					Reason: "CompononetDeleted",
				})
				return false
			} else {
				log.FromContext(ctx).Error(err, "Deletion error")
				return true // unexpected error ==> still present
			}
		}
		return true // still present
	}

	meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
		Type:   string(workloadv1beta2.DeletingResources),
		Status: metav1.ConditionTrue,
		Reason: "DeletionInitiated",
	})

	componentsRemaining := false
	for componentIdx := range aw.Spec.Components {
		componentsRemaining = deleteIfPresent(componentIdx, client.PropagationPolicy(metav1.DeletePropagationBackground)) || componentsRemaining
	}

	deletionGracePeriod := r.forcefulDeletionGraceDuration(ctx, aw)
	whenInitiated := meta.FindStatusCondition(aw.Status.Conditions, string(workloadv1beta2.DeletingResources)).LastTransitionTime
	gracePeriodExpired := time.Now().After(whenInitiated.Time.Add(deletionGracePeriod))

	if componentsRemaining && !gracePeriodExpired {
		// Resources left and deadline hasn't expired, just requeue the deletion
		return false
	}

	pods := &v1.PodList{Items: []v1.Pod{}}
	if err := r.List(ctx, pods,
		client.UnsafeDisableDeepCopy,
		client.InNamespace(aw.Namespace),
		client.MatchingLabels{AppWrapperLabel: aw.Name}); err != nil {
		log.FromContext(ctx).Error(err, "Pod list error")
	}

	if !componentsRemaining && len(pods.Items) == 0 {
		// no resources or pods left; deletion is complete
		clearCondition(aw, workloadv1beta2.DeletingResources, "DeletionComplete", "")
		return true
	}

	if gracePeriodExpired {
		if len(pods.Items) > 0 {
			// force deletion of pods first
			for _, pod := range pods.Items {
				if err := r.Delete(ctx, &pod, client.GracePeriodSeconds(0)); err != nil {
					log.FromContext(ctx).Error(err, "Forceful pod deletion error")
				}
			}
		} else {
			// force deletion of wrapped resources once pods are gone
			for componentIdx := range aw.Spec.Components {
				_ = deleteIfPresent(componentIdx, client.GracePeriodSeconds(0))
			}
		}
	}

	// requeue deletion
	return false
}
