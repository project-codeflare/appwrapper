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
	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes"
	authClientv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/kueue/pkg/controller/jobframework"
)

type AppWrapperWebhook struct {
	Config                *AppWrapperConfig
	SubjectAccessReviewer authClientv1.SubjectAccessReviewInterface
}

//+kubebuilder:webhook:path=/mutate-workload-codeflare-dev-v1beta2-appwrapper,mutating=true,failurePolicy=fail,sideEffects=None,groups=workload.codeflare.dev,resources=appwrappers,verbs=create,versions=v1beta2,name=mappwrapper.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &AppWrapperWebhook{}

// Default ensures that Suspend is set appropriately when an AppWrapper is created
func (w *AppWrapperWebhook) Default(ctx context.Context, obj runtime.Object) error {
	aw := obj.(*workloadv1beta2.AppWrapper)
	log.FromContext(ctx).Info("Applying defaults", "job", aw)
	jobframework.ApplyDefaultForSuspend((*AppWrapper)(aw), w.Config.ManageJobsWithoutQueueName)
	return nil
}

//+kubebuilder:webhook:path=/validate-workload-codeflare-dev-v1beta2-appwrapper,mutating=false,failurePolicy=fail,sideEffects=None,groups=workload.codeflare.dev,resources=appwrappers,verbs=create;update,versions=v1beta2,name=vappwrapper.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &AppWrapperWebhook{}

// ValidateCreate validates invariants when an AppWrapper is created
func (w *AppWrapperWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	aw := obj.(*workloadv1beta2.AppWrapper)

	allErrors := w.validateAppWrapperInvariants(ctx, aw)
	if w.Config.ManageJobsWithoutQueueName || jobframework.QueueName((*AppWrapper)(aw)) != "" {
		allErrors = append(allErrors, jobframework.ValidateCreateForQueueName((*AppWrapper)(aw))...)
	}

	if len(allErrors) > 0 {
		log.FromContext(ctx).Info("Validating create failed", "job", aw, "errors", allErrors)
	} else {
		log.FromContext(ctx).Info("Validating create succeeded", "job", aw)
	}

	return nil, allErrors.ToAggregate()
}

// ValidateUpdate validates invariants when an AppWrapper is updated
func (w *AppWrapperWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldAW := oldObj.(*workloadv1beta2.AppWrapper)
	newAW := newObj.(*workloadv1beta2.AppWrapper)

	allErrors := w.validateAppWrapperInvariants(ctx, newAW)
	if w.Config.ManageJobsWithoutQueueName || jobframework.QueueName((*AppWrapper)(newAW)) != "" {
		allErrors = append(allErrors, jobframework.ValidateUpdateForQueueName((*AppWrapper)(oldAW), (*AppWrapper)(newAW))...)
		allErrors = append(allErrors, jobframework.ValidateUpdateForWorkloadPriorityClassName((*AppWrapper)(oldAW), (*AppWrapper)(newAW))...)
	}

	if len(allErrors) > 0 {
		log.FromContext(ctx).Info("Validating update failed", "job", newAW, "errors", allErrors)
	} else {
		log.FromContext(ctx).Info("Validating create succeeded", "job", newAW)
	}

	return nil, allErrors.ToAggregate()
}

// ValidateDelete is a noop for us, but is required to implement the CustomValidator interface
func (w *AppWrapperWebhook) ValidateDelete(context.Context, runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// validateAppWrapperInvariants checks these invariants:
//  1. AppWrappers must not contain other AppWrappers
//  2. AppWrappers must only contain resources intended for their own namespace
//  3. AppWrappers must not contain any resources that the user could not create directly
//  4. Every PodSet must be well-formed: the Path must exist and must be parseable as a PodSpecTemplate
//  5. AppWrappers must contain between 1 and 8 PodSets (Kueue invariant)
func (w *AppWrapperWebhook) validateAppWrapperInvariants(ctx context.Context, aw *workloadv1beta2.AppWrapper) field.ErrorList {
	allErrors := field.ErrorList{}
	components := aw.Spec.Components
	componentsPath := field.NewPath("spec").Child("components")
	podSpecCount := 0
	request, err := admission.RequestFromContext(ctx)
	if err != nil {
		allErrors = append(allErrors, field.InternalError(componentsPath, err))
	}
	userInfo := request.UserInfo

	for idx, component := range components {
		compPath := componentsPath.Index(idx)
		unstruct := &unstructured.Unstructured{}
		_, gvk, err := unstructured.UnstructuredJSONScheme.Decode(component.Template.Raw, nil, unstruct)
		if err != nil {
			allErrors = append(allErrors, field.Invalid(compPath.Child("template"), component.Template, "failed to decode as JSON"))
		}

		// 1. Deny nested AppWrappers
		if *gvk == GVK {
			allErrors = append(allErrors, field.Forbidden(compPath.Child("template"), "Nested AppWrappers are forbidden"))
		}

		// 2. Forbid multi-namespace creations
		if unstruct.GetNamespace() != "" && unstruct.GetNamespace() != aw.Namespace {
			allErrors = append(allErrors, field.Forbidden(compPath.Child("template").Child("metadata").Child("namespace"),
				"AppWrappers cannot create objects in other namespaces"))
		}

		// 3. RBAC check: Perform SubjectAccessReview to verify user can directly create each wrapped resource
		sar := &authv1.SubjectAccessReview{
			Spec: authv1.SubjectAccessReviewSpec{
				ResourceAttributes: &authv1.ResourceAttributes{
					Namespace: aw.Namespace,
					Verb:      "create",
					Group:     gvk.Group,
					Version:   gvk.Version,
					Resource:  gvk.Kind,
				},
				User:   userInfo.Username,
				UID:    userInfo.UID,
				Groups: userInfo.Groups,
			}}
		if len(userInfo.Extra) > 0 {
			sar.Spec.Extra = make(map[string]authv1.ExtraValue, len(userInfo.Extra))
			for k, v := range userInfo.Extra {
				sar.Spec.Extra[k] = authv1.ExtraValue(v)
			}
		}
		sar, err = w.SubjectAccessReviewer.Create(ctx, sar, metav1.CreateOptions{})
		if err != nil {
			allErrors = append(allErrors, field.InternalError(compPath.Child("template"), err))
		} else {
			if !sar.Status.Allowed {
				reason := sar.Status.Reason
				if reason == "" {
					reason = "Permission denied"
				}
				allErrors = append(allErrors, field.Forbidden(compPath.Child("template"), reason))
			}
		}

		// 4. Each PodSet.Path must specify a path within Template to a v1.PodSpecTemplate
		podSetsPath := compPath.Child("podSets")
		for psIdx, ps := range component.PodSets {
			podSetPath := podSetsPath.Index(psIdx)
			if ps.Path == "" {
				allErrors = append(allErrors, field.Required(podSetPath.Child("path"), "podspec must specify path"))
			}
			if _, err := getPodTemplateSpec(unstruct, ps.Path); err != nil {
				allErrors = append(allErrors, field.Invalid(podSetPath.Child("path"), ps.Path,
					fmt.Sprintf("path does not refer to a v1.PodSpecTemplate: %v", err)))
			}
			podSpecCount += 1
		}
	}

	// 5. Enforce Kueue limitation that 0 < podSpecCount <= 8
	if podSpecCount == 0 {
		allErrors = append(allErrors, field.Invalid(componentsPath, components, "components contains no podspecs"))
	}
	if podSpecCount > 8 {
		allErrors = append(allErrors, field.Invalid(componentsPath, components, fmt.Sprintf("components contains %v podspecs; at most 8 are allowed", podSpecCount)))
	}

	return allErrors
}

func (wh *AppWrapperWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	kubeClient, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	wh.SubjectAccessReviewer = kubeClient.AuthorizationV1().SubjectAccessReviews()
	return ctrl.NewWebhookManagedBy(mgr).
		For(&workloadv1beta2.AppWrapper{}).
		WithDefaulter(wh).
		WithValidator(wh).
		Complete()
}
