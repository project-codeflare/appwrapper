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

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	"github.com/project-codeflare/appwrapper/pkg/utils"
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

func parseComponent(aw *workloadv1beta2.AppWrapper, raw []byte) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	if _, _, err := unstructured.UnstructuredJSONScheme.Decode(raw, nil, obj); err != nil {
		return nil, err
	}
	namespace := obj.GetNamespace()
	if namespace == "" {
		obj.SetNamespace(aw.Namespace)
	} else if namespace != aw.Namespace {
		// Should not happen, namespace equality checked by validateAppWrapperInvariants
		return nil, fmt.Errorf("component namespace \"%s\" is different from appwrapper namespace \"%s\"", namespace, aw.Namespace)
	}
	return obj, nil
}

func (r *AppWrapperReconciler) createComponent(ctx context.Context, aw *workloadv1beta2.AppWrapper, componentIdx int) (*unstructured.Unstructured, error, bool) {
	component := aw.Spec.Components[componentIdx]
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

	obj, err := parseComponent(aw, component.Template.Raw)
	if err != nil {
		return nil, err, true
	}
	if r.Config.StandaloneMode {
		obj.SetLabels(utilmaps.MergeKeepFirst(obj.GetLabels(), map[string]string{AppWrapperLabel: aw.Name}))
	} else {
		obj.SetLabels(utilmaps.MergeKeepFirst(obj.GetLabels(), map[string]string{AppWrapperLabel: aw.Name, constants.QueueLabel: childJobQueueName}))
	}

	awLabels := map[string]string{AppWrapperLabel: aw.Name}
	for podSetsIdx, podSet := range component.PodSets {
		toInject := &workloadv1beta2.AppWrapperPodSetInfo{}
		if !r.Config.StandaloneMode {
			toInject = &component.PodSetInfos[podSetsIdx]
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
			existing := toMap(metadata["nodeSelector"])
			if err := utilmaps.HaveConflict(existing, toInject.NodeSelector); err != nil {
				return nil, podset.BadPodSetsUpdateError("nodeSelector", err), true
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

	if err := controllerutil.SetControllerReference(aw, obj, r.Scheme); err != nil {
		return nil, err, true
	}

	if err := r.Create(ctx, obj); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, err, meta.IsNoMatchError(err) || apierrors.IsInvalid(err) // fatal
		}
	}

	return obj, nil, false
}

func (r *AppWrapperReconciler) createComponents(ctx context.Context, aw *workloadv1beta2.AppWrapper) (error, bool) {
	for componentIdx := range aw.Spec.Components {
		_, err, fatal := r.createComponent(ctx, aw, componentIdx)
		if err != nil {
			return err, fatal
		}
	}
	return nil, false
}

func (r *AppWrapperReconciler) deleteComponents(ctx context.Context, aw *workloadv1beta2.AppWrapper) bool {
	// TODO forceful deletion: See https://github.com/project-codeflare/appwrapper/issues/36
	log := log.FromContext(ctx)
	remaining := 0
	for _, component := range aw.Spec.Components {
		obj, err := parseComponent(aw, component.Template.Raw)
		if err != nil {
			log.Error(err, "Parsing error")
			continue
		}
		if err := r.Delete(ctx, obj, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			if !apierrors.IsNotFound(err) {
				log.Error(err, "Deletion error")
			}
			continue
		}
		remaining++ // no error deleting resource, resource therefore still exists
	}
	return remaining == 0
}
