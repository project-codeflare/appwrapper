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
	"encoding/json"
	"fmt"
	"time"

	awv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	utilmaps "github.com/project-codeflare/appwrapper/internal/util"
	"github.com/project-codeflare/appwrapper/pkg/utils"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	kresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/kueue/pkg/podset"
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

func hasResourceRequest(spec map[string]interface{}, resource string) bool {
	usesResource := func(container map[string]interface{}) bool {
		_, ok := container["resources"]
		if !ok {
			return false
		}
		resources, ok := container["resources"].(map[string]interface{})
		if !ok {
			return false
		}
		for _, key := range []string{"limits", "requests"} {
			if _, ok := resources[key]; ok {
				if list, ok := resources[key].(map[string]interface{}); ok {
					if _, ok := list[resource]; ok {
						switch quantity := list[resource].(type) {
						case int:
							if quantity > 0 {
								return true
							}
						case int32:
							if quantity > 0 {
								return true
							}
						case int64:
							if quantity > 0 {
								return true
							}
						case string:
							kq, err := kresource.ParseQuantity(quantity)
							if err == nil && !kq.IsZero() {
								return true
							}
						}
					}
				}
			}
		}
		return false
	}

	for _, key := range []string{"containers", "initContainers"} {
		if containers, ok := spec[key]; ok {
			if carray, ok := containers.([]interface{}); ok {
				for _, containerI := range carray {
					container, ok := containerI.(map[string]interface{})
					if ok && usesResource(container) {
						return true
					}
				}
			}
		}
	}

	return false
}

func addNodeSelectorsToAffinity(spec map[string]interface{}, exprsToAdd []v1.NodeSelectorRequirement, required bool, weight int32) error {
	if _, ok := spec["affinity"]; !ok {
		spec["affinity"] = map[string]interface{}{}
	}
	affinity, ok := spec["affinity"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("spec.affinity is not a map")
	}
	if _, ok := affinity["nodeAffinity"]; !ok {
		affinity["nodeAffinity"] = map[string]interface{}{}
	}
	nodeAffinity, ok := affinity["nodeAffinity"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("spec.affinity.nodeAffinity is not a map")
	}
	if required {
		if _, ok := nodeAffinity["requiredDuringSchedulingIgnoredDuringExecution"]; !ok {
			nodeAffinity["requiredDuringSchedulingIgnoredDuringExecution"] = map[string]interface{}{}
		}
		nodeSelector, ok := nodeAffinity["requiredDuringSchedulingIgnoredDuringExecution"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution is not a map")
		}
		if _, ok := nodeSelector["nodeSelectorTerms"]; !ok {
			nodeSelector["nodeSelectorTerms"] = []interface{}{map[string]interface{}{}}
		}
		existingTerms, ok := nodeSelector["nodeSelectorTerms"].([]interface{})
		if !ok {
			return fmt.Errorf("spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms is not an array")
		}
		for idx, term := range existingTerms {
			selTerm, ok := term.(map[string]interface{})
			if !ok {
				return fmt.Errorf("spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[%v] is not an map", idx)
			}
			if _, ok := selTerm["matchExpressions"]; !ok {
				selTerm["matchExpressions"] = []interface{}{}
			}
			matchExpressions, ok := selTerm["matchExpressions"].([]interface{})
			if !ok {
				return fmt.Errorf("spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[%v].matchExpressions is not an map", idx)
			}
			for _, expr := range exprsToAdd {
				bytes, err := json.Marshal(expr)
				if err != nil {
					return fmt.Errorf("marshalling selectorTerm %v: %w", expr, err)
				}
				var obj interface{}
				if err = json.Unmarshal(bytes, &obj); err != nil {
					return fmt.Errorf("unmarshalling selectorTerm %v: %w", expr, err)
				}
				matchExpressions = append(matchExpressions, obj)
			}
			selTerm["matchExpressions"] = matchExpressions
		}
	} else {
		if _, ok := nodeAffinity["preferredDuringSchedulingIgnoredDuringExecution"]; !ok {
			nodeAffinity["preferredDuringSchedulingIgnoredDuringExecution"] = []interface{}{}
		}
		terms, ok := nodeAffinity["preferredDuringSchedulingIgnoredDuringExecution"].([]interface{})
		if !ok {
			return fmt.Errorf("spec.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution is not an array")
		}
		bytes, err := json.Marshal(v1.PreferredSchedulingTerm{Weight: weight, Preference: v1.NodeSelectorTerm{MatchExpressions: exprsToAdd}})
		if err != nil {
			return fmt.Errorf("marshalling selectorTerms %v: %w", exprsToAdd, err)
		}
		var obj interface{}
		if err = json.Unmarshal(bytes, &obj); err != nil {
			return fmt.Errorf("unmarshalling selectorTerms %v: %w", exprsToAdd, err)
		}
		terms = append(terms, obj)
		nodeAffinity["preferredDuringSchedulingIgnoredDuringExecution"] = terms
	}

	return nil
}

//gocyclo:ignore
func (r *AppWrapperReconciler) createComponent(ctx context.Context, aw *awv1beta2.AppWrapper, componentIdx int) (error, bool) {
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
		return err, true
	}
	awLabels := map[string]string{awv1beta2.AppWrapperLabel: aw.Name}
	obj.SetLabels(utilmaps.MergeKeepFirst(obj.GetLabels(), awLabels))

	for podSetsIdx, podSet := range componentStatus.PodSets {
		toInject := &awv1beta2.AppWrapperPodSetInfo{}
		if podSetsIdx < len(component.PodSetInfos) {
			toInject = &component.PodSetInfos[podSetsIdx]
		}

		p, err := utils.GetRawTemplate(obj.UnstructuredContent(), podSet.Path)
		if err != nil {
			return err, true // Should not happen, path validity is enforced by validateAppWrapperInvariants
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
				return podset.BadPodSetsUpdateError("annotations", err), true
			}
			metadata["annotations"] = utilmaps.MergeKeepFirst(existing, toInject.Annotations)
		}

		// Labels
		mergedLabels := utilmaps.MergeKeepFirst(toInject.Labels, awLabels)
		existing := toMap(metadata["labels"])
		if err := utilmaps.HaveConflict(existing, mergedLabels); err != nil {
			return podset.BadPodSetsUpdateError("labels", err), true
		}
		metadata["labels"] = utilmaps.MergeKeepFirst(existing, mergedLabels)

		// NodeSelectors
		if len(toInject.NodeSelector) > 0 {
			existing := toMap(spec["nodeSelector"])
			if err := utilmaps.HaveConflict(existing, toInject.NodeSelector); err != nil {
				return podset.BadPodSetsUpdateError("nodeSelector", err), true
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

		// SchedulingGates
		if len(toInject.SchedulingGates) > 0 {
			if _, ok := spec["schedulingGates"]; !ok {
				spec["schedulingGates"] = []interface{}{}
			}
			schedulingGates := spec["schedulingGates"].([]interface{})
			for _, addition := range toInject.SchedulingGates {
				duplicate := false
				for _, existing := range schedulingGates {
					if imap, ok := existing.(map[string]interface{}); ok {
						if iName, ok := imap["name"]; ok {
							if sName, ok := iName.(string); ok && sName == addition.Name {
								duplicate = true
								break
							}
						}
					}
				}
				if !duplicate {
					schedulingGates = append(schedulingGates, map[string]interface{}{"name": addition.Name})
				}
			}
			spec["schedulingGates"] = schedulingGates
		}

		// Scheduler Name
		if r.Config.SchedulerName != "" {
			if existing, _ := spec["schedulerName"].(string); existing == "" {
				spec["schedulerName"] = r.Config.SchedulerName
			}
		}

		if r.Config.Autopilot != nil && r.Config.Autopilot.InjectAntiAffinities {
			toAddRequired := map[string][]string{}
			toAddPreferred := map[string][]string{}
			for resource, taints := range r.Config.Autopilot.ResourceTaints {
				if hasResourceRequest(spec, resource) {
					for _, taint := range taints {
						if taint.Effect == v1.TaintEffectNoExecute || taint.Effect == v1.TaintEffectNoSchedule {
							toAddRequired[taint.Key] = append(toAddRequired[taint.Key], taint.Value)
						} else if taint.Effect == v1.TaintEffectPreferNoSchedule {
							toAddPreferred[taint.Key] = append(toAddPreferred[taint.Key], taint.Value)
						}
					}
				}
			}
			if len(toAddRequired) > 0 {
				matchExpressions := []v1.NodeSelectorRequirement{}
				for k, v := range toAddRequired {
					matchExpressions = append(matchExpressions, v1.NodeSelectorRequirement{Operator: v1.NodeSelectorOpNotIn, Key: k, Values: v})
				}
				if err := addNodeSelectorsToAffinity(spec, matchExpressions, true, 0); err != nil {
					log.FromContext(ctx).Error(err, "failed to inject Autopilot affinities")
				}
			}
			if len(toAddPreferred) > 0 {
				matchExpressions := []v1.NodeSelectorRequirement{}
				for k, v := range toAddPreferred {
					matchExpressions = append(matchExpressions, v1.NodeSelectorRequirement{Operator: v1.NodeSelectorOpNotIn, Key: k, Values: v})
				}
				weight := ptr.Deref(r.Config.Autopilot.PreferNoScheduleWeight, 1)
				if err := addNodeSelectorsToAffinity(spec, matchExpressions, false, weight); err != nil {
					log.FromContext(ctx).Error(err, "failed to inject Autopilot affinities")
				}
			}
		}
	}

	if err := controllerutil.SetControllerReference(aw, obj, r.Scheme); err != nil {
		return err, true
	}

	log.FromContext(ctx).Info("After injection", "obj", obj)

	orig := copyForStatusPatch(aw)
	if meta.FindStatusCondition(aw.Status.ComponentStatus[componentIdx].Conditions, string(awv1beta2.ResourcesDeployed)) == nil {
		aw.Status.ComponentStatus[componentIdx].Name = obj.GetName()
		aw.Status.ComponentStatus[componentIdx].Kind = obj.GetKind()
		aw.Status.ComponentStatus[componentIdx].APIVersion = obj.GetAPIVersion()
		meta.SetStatusCondition(&aw.Status.ComponentStatus[componentIdx].Conditions, metav1.Condition{
			Type:   string(awv1beta2.ResourcesDeployed),
			Status: metav1.ConditionUnknown,
			Reason: "ComponentCreationInitiated",
		})
		if err := r.Status().Patch(ctx, aw, client.MergeFrom(orig)); err != nil {
			return err, false
		}
	}

	if err := r.Create(ctx, obj); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// obj is not updated if Create returns an error; Get required for accurate information
			if err := r.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
				return err, false
			}
			ctrlRef := metav1.GetControllerOf(obj)
			if ctrlRef == nil || ctrlRef.Name != aw.Name {
				return fmt.Errorf("resource %v exists, but is not controlled by appwrapper", obj.GetName()), true
			}
			// fall through.  This is not actually an error. The object already exists and the correct appwrapper owns it.
		} else {
			// resource not actually created; patch status to reflect that
			orig := copyForStatusPatch(aw)
			meta.SetStatusCondition(&aw.Status.ComponentStatus[componentIdx].Conditions, metav1.Condition{
				Type:   string(awv1beta2.ResourcesDeployed),
				Status: metav1.ConditionFalse,
				Reason: "ComponentCreationErrored",
			})
			if patchErr := r.Status().Patch(ctx, aw, client.MergeFrom(orig)); patchErr != nil {
				// ugh.  Patch failed, so retry the create so we can get to a consistient state
				return patchErr, false
			}
			// return actual error
			return err, meta.IsNoMatchError(err) || apierrors.IsInvalid(err) // fatal
		}
	}

	orig = copyForStatusPatch(aw)
	aw.Status.ComponentStatus[componentIdx].Name = obj.GetName() // Update name to support usage of GenerateName
	meta.SetStatusCondition(&aw.Status.ComponentStatus[componentIdx].Conditions, metav1.Condition{
		Type:   string(awv1beta2.ResourcesDeployed),
		Status: metav1.ConditionTrue,
		Reason: "ComponentCreatedSuccessfully",
	})
	if err := r.Status().Patch(ctx, aw, client.MergeFrom(orig)); err != nil {
		return err, false
	}

	return nil, false
}

// createComponents incrementally patches aw.Status -- MUST NOT CARRY STATUS PATCHES ACROSS INVOCATIONS
func (r *AppWrapperReconciler) createComponents(ctx context.Context, aw *awv1beta2.AppWrapper) (error, bool) {
	for componentIdx := range aw.Spec.Components {
		if !meta.IsStatusConditionTrue(aw.Status.ComponentStatus[componentIdx].Conditions, string(awv1beta2.ResourcesDeployed)) {
			if err, fatal := r.createComponent(ctx, aw, componentIdx); err != nil {
				return err, fatal
			}
		}
	}
	return nil, false
}

func (r *AppWrapperReconciler) deleteComponents(ctx context.Context, aw *awv1beta2.AppWrapper) bool {
	deleteIfPresent := func(idx int, opts ...client.DeleteOption) bool {
		cs := &aw.Status.ComponentStatus[idx]
		rd := meta.FindStatusCondition(cs.Conditions, string(awv1beta2.ResourcesDeployed))
		if rd == nil || rd.Status == metav1.ConditionFalse || (rd.Status == metav1.ConditionUnknown && cs.Name == "") {
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
					Type:   string(awv1beta2.ResourcesDeployed),
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
		Type:   string(awv1beta2.DeletingResources),
		Status: metav1.ConditionTrue,
		Reason: "DeletionInitiated",
	})

	componentsRemaining := false
	for componentIdx := range aw.Spec.Components {
		componentsRemaining = deleteIfPresent(componentIdx, client.PropagationPolicy(metav1.DeletePropagationBackground)) || componentsRemaining
	}

	deletionGracePeriod := r.forcefulDeletionGraceDuration(ctx, aw)
	whenInitiated := meta.FindStatusCondition(aw.Status.Conditions, string(awv1beta2.DeletingResources)).LastTransitionTime
	gracePeriodExpired := time.Now().After(whenInitiated.Time.Add(deletionGracePeriod))

	if componentsRemaining && !gracePeriodExpired {
		// Resources left and deadline hasn't expired, just requeue the deletion
		return false
	}

	pods := &v1.PodList{Items: []v1.Pod{}}
	if err := r.List(ctx, pods,
		client.UnsafeDisableDeepCopy,
		client.InNamespace(aw.Namespace),
		client.MatchingLabels{awv1beta2.AppWrapperLabel: aw.Name}); err != nil {
		log.FromContext(ctx).Error(err, "Pod list error")
	}

	if !componentsRemaining && len(pods.Items) == 0 {
		// no resources or pods left; deletion is complete
		clearCondition(aw, awv1beta2.DeletingResources, "DeletionComplete", "")
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
