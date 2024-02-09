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
	"context"
	"fmt"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/kueue/pkg/controller/jobframework"
)

type AppWrapperWebhook struct {
	ManageJobsWithoutQueueName bool
}

//+kubebuilder:webhook:path=/mutate-workload-codeflare-dev-v1beta2-appwrapper,mutating=true,failurePolicy=fail,sideEffects=None,groups=workload.codeflare.dev,resources=appwrappers,verbs=create,versions=v1beta2,name=mappwrapper.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &AppWrapperWebhook{}

// Default ensures that Suspend is set appropriately when an AppWrapper is created
func (w *AppWrapperWebhook) Default(ctx context.Context, obj runtime.Object) error {
	aw := obj.(*workloadv1beta2.AppWrapper)
	log.FromContext(ctx).Info("Applying defaults", "job", aw)
	jobframework.ApplyDefaultForSuspend((*AppWrapper)(aw), w.ManageJobsWithoutQueueName)
	return nil
}

//+kubebuilder:webhook:path=/validate-workload-codeflare-dev-v1beta2-appwrapper,mutating=false,failurePolicy=fail,sideEffects=None,groups=workload.codeflare.dev,resources=appwrappers,verbs=create;update,versions=v1beta2,name=vappwrapper.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &AppWrapperWebhook{}

// ValidateCreate validates invariants when an AppWrapper is created
func (w *AppWrapperWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	aw := obj.(*workloadv1beta2.AppWrapper)
	log.FromContext(ctx).Info("Validating create", "job", aw)

	allErrors := w.validateAppWrapperInvariants(ctx, aw)

	if w.ManageJobsWithoutQueueName || jobframework.QueueName((*AppWrapper)(aw)) != "" {
		allErrors = append(allErrors, jobframework.ValidateCreateForQueueName((*AppWrapper)(aw))...)
	}

	return nil, allErrors.ToAggregate()
}

// ValidateUpdate validates invariants when an AppWrapper is updated
func (w *AppWrapperWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldAW := oldObj.(*workloadv1beta2.AppWrapper)
	newAW := newObj.(*workloadv1beta2.AppWrapper)
	log.FromContext(ctx).Info("Validating update", "job", newAW)

	allErrors := w.validateAppWrapperInvariants(ctx, newAW)

	if w.ManageJobsWithoutQueueName || jobframework.QueueName((*AppWrapper)(newAW)) != "" {
		allErrors = append(allErrors, jobframework.ValidateUpdateForQueueName((*AppWrapper)(oldAW), (*AppWrapper)(newAW))...)
		allErrors = append(allErrors, jobframework.ValidateUpdateForWorkloadPriorityClassName((*AppWrapper)(oldAW), (*AppWrapper)(newAW))...)
	}

	return nil, allErrors.ToAggregate()
}

// ValidateDelete is a noop for us, but is required to implement the CustomValidator interface
func (w *AppWrapperWebhook) ValidateDelete(context.Context, runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// validateAppWrapperInvariants checks AppWrapper-specific invariants
func (w *AppWrapperWebhook) validateAppWrapperInvariants(_ context.Context, aw *workloadv1beta2.AppWrapper) field.ErrorList {
	allErrors := field.ErrorList{}
	components := aw.Spec.Components
	componentsPath := field.NewPath("spec").Child("components")
	podSpecCount := 0

	for idx, component := range components {

		// Each PodSet.Path must specify a path within Template to a v1.PodSpecTemplate
		podSetsPath := componentsPath.Index(idx).Child("podSets")
		for psIdx, ps := range component.PodSets {
			podSetPath := podSetsPath.Index(psIdx)
			if ps.Path == "" {
				allErrors = append(allErrors, field.Required(podSetPath.Child("path"), "podspec must specify path"))
			}
			if _, err := getPodTemplateSpec(component.Template.Raw, ps.Path); err != nil {
				allErrors = append(allErrors, field.Invalid(podSetPath.Child("path"), ps.Path,
					fmt.Sprintf("path does not refer to a v1.PodSpecTemplate: %v", err)))
			}
			podSpecCount += 1
		}

		// TODO: RBAC check to make sure that the user has permissions to create the component

		// TODO: We could attempt to validate the object is namespaced and the namespace is the same as the AppWrapper's namespace
		//       This is currently enforced when the resources are created.

	}

	// Enforce Kueue limitation that 0 < podSpecCount <= 8
	if podSpecCount == 0 {
		allErrors = append(allErrors, field.Invalid(componentsPath, components, "components contains no podspecs"))
	}
	if podSpecCount > 8 {
		allErrors = append(allErrors, field.Invalid(componentsPath, components, fmt.Sprintf("components contains %v podspecs; at most 8 are allowed", podSpecCount)))
	}

	return allErrors
}
