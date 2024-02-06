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

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type
func (w *AppWrapperWebhook) Default(ctx context.Context, obj runtime.Object) error {
	job := obj.(*workloadv1beta2.AppWrapper)
	log.FromContext(ctx).Info("Applying defaults", "job", job)
	jobframework.ApplyDefaultForSuspend((*AppWrapper)(job), w.ManageJobsWithoutQueueName)
	return nil
}

//+kubebuilder:webhook:path=/validate-workload-codeflare-dev-v1beta2-appwrapper,mutating=false,failurePolicy=fail,sideEffects=None,groups=workload.codeflare.dev,resources=appwrappers,verbs=create;update,versions=v1beta2,name=vappwrapper.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &AppWrapperWebhook{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (w *AppWrapperWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	job := obj.(*workloadv1beta2.AppWrapper)
	log.FromContext(ctx).Info("Validating create", "job", job)
	return nil, w.validateCreate(job).ToAggregate()
}

func (w *AppWrapperWebhook) validateCreate(job *workloadv1beta2.AppWrapper) field.ErrorList {
	var allErrors field.ErrorList

	if w.ManageJobsWithoutQueueName || jobframework.QueueName((*AppWrapper)(job)) != "" {
		components := job.Spec.Components
		componentsPath := field.NewPath("spec").Child("components")
		podSpecCount := 0
		for idx, component := range components {
			podSetsPath := componentsPath.Index(idx).Child("podSets")
			for psIdx, ps := range component.PodSets {
				podSetPath := podSetsPath.Index(psIdx)
				if ps.Path == "" {
					allErrors = append(allErrors, field.Required(podSetPath.Child("path"), "podspec must specify path"))
				}

				// TODO: Validatate the ps.Path resolves to a PodSpec

				// TODO: RBAC check to make sure that the user has the ability to create the wrapped resources

				podSpecCount += 1
			}
		}
		if podSpecCount == 0 {
			allErrors = append(allErrors, field.Invalid(componentsPath, components, "components contains no podspecs"))
		}
		if podSpecCount > 8 {
			allErrors = append(allErrors, field.Invalid(componentsPath, components, fmt.Sprintf("components contains %v podspecs; at most 8 are allowed", podSpecCount)))
		}
	}

	allErrors = append(allErrors, jobframework.ValidateCreateForQueueName((*AppWrapper)(job))...)
	return allErrors
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (w *AppWrapperWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldJob := oldObj.(*workloadv1beta2.AppWrapper)
	newJob := newObj.(*workloadv1beta2.AppWrapper)
	if w.ManageJobsWithoutQueueName || jobframework.QueueName((*AppWrapper)(newJob)) != "" {
		log.FromContext(ctx).Info("Validating update", "job", newJob)
		allErrors := jobframework.ValidateUpdateForQueueName((*AppWrapper)(oldJob), (*AppWrapper)(newJob))
		allErrors = append(allErrors, w.validateCreate(newJob)...)
		allErrors = append(allErrors, jobframework.ValidateUpdateForWorkloadPriorityClassName((*AppWrapper)(oldJob), (*AppWrapper)(newJob))...)
		return nil, allErrors.ToAggregate()
	}
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (w *AppWrapperWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
