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

package webhook

import (
	"bytes"
	"context"
	"fmt"

	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	discovery "k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	authClientv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	utilmaps "sigs.k8s.io/kueue/pkg/util/maps"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"sigs.k8s.io/kueue/pkg/controller/jobframework"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	awc "github.com/project-codeflare/appwrapper/internal/controller/appwrapper"
	wlc "github.com/project-codeflare/appwrapper/internal/controller/workload"
	"github.com/project-codeflare/appwrapper/pkg/config"
	"github.com/project-codeflare/appwrapper/pkg/utils"
)

const (
	AppWrapperUsernameLabel = "workload.codeflare.dev/user"
	AppWrapperUserIDLabel   = "workload.codeflare.dev/userid"
	QueueNameLabel          = "kueue.x-k8s.io/queue-name"
)

type AppWrapperWebhook struct {
	Config                *config.AppWrapperConfig
	SubjectAccessReviewer authClientv1.SubjectAccessReviewInterface
	client                client.Client
	DiscoveryClient       *discovery.DiscoveryClient
	kindToResourceCache   map[string]string
}

//+kubebuilder:webhook:path=/mutate-workload-codeflare-dev-v1beta2-appwrapper,mutating=true,failurePolicy=fail,sideEffects=None,groups=workload.codeflare.dev,resources=appwrappers,verbs=create,versions=v1beta2,name=mappwrapper.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &AppWrapperWebhook{}

// Default fills in default values when an AppWrapper is created:
//  1. Inject default queue name
//  2. Ensure Suspend is set appropriately
//  3. Add labels with the user name and id
func (w *AppWrapperWebhook) Default(ctx context.Context, obj runtime.Object) error {
	aw := obj.(*workloadv1beta2.AppWrapper)
	log.FromContext(ctx).V(2).Info("Applying defaults", "job", aw)

	// Queue name and Suspend
	if w.Config.EnableKueueIntegrations {
		if w.Config.DefaultQueueName != "" {
			aw.Labels = utilmaps.MergeKeepFirst(aw.Labels, map[string]string{QueueNameLabel: w.Config.DefaultQueueName})
		}
		nsSelector, err := metav1.LabelSelectorAsSelector(w.Config.KueueJobReconciller.ManageJobsNamespaceSelector)
		if err != nil {
			return err
		}
		err = jobframework.ApplyDefaultForSuspend(ctx, (*wlc.AppWrapper)(aw), w.client, w.Config.KueueJobReconciller.ManageJobsWithoutQueueName, nsSelector)
		if err != nil {
			return err
		}
	}

	// inject labels with user name and id
	request, err := admission.RequestFromContext(ctx)
	if err != nil {
		return err
	}
	userInfo := request.UserInfo
	username := utils.SanitizeLabel(userInfo.Username)
	aw.Labels = utilmaps.MergeKeepFirst(map[string]string{AppWrapperUsernameLabel: username, AppWrapperUserIDLabel: userInfo.UID}, aw.Labels)

	// do not inject finalizer if managed by another controller
	if aw.Spec.ManagedBy != nil && *aw.Spec.ManagedBy != workloadv1beta2.AppWrapperControllerName {
		return nil
	}

	// inject finalizer now (avoid reconcilier errors between the AppWrapper and WorkloadControllers when it is admitted by a ClusterQueue almost immediately)
	controllerutil.AddFinalizer(aw, awc.AppWrapperFinalizer)

	return nil
}

//+kubebuilder:webhook:path=/validate-workload-codeflare-dev-v1beta2-appwrapper,mutating=false,failurePolicy=fail,sideEffects=None,groups=workload.codeflare.dev,resources=appwrappers,verbs=create;update,versions=v1beta2,name=vappwrapper.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &AppWrapperWebhook{}

// ValidateCreate validates invariants when an AppWrapper is created
func (w *AppWrapperWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	aw := obj.(*workloadv1beta2.AppWrapper)
	log.FromContext(ctx).V(2).Info("Validating create", "job", aw)
	allErrors := w.validateAppWrapperCreate(ctx, aw)
	if w.Config.EnableKueueIntegrations {
		allErrors = append(allErrors, jobframework.ValidateJobOnCreate((*wlc.AppWrapper)(aw))...)
	}
	return nil, allErrors.ToAggregate()
}

// ValidateUpdate validates invariants when an AppWrapper is updated
func (w *AppWrapperWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldAW := oldObj.(*workloadv1beta2.AppWrapper)
	newAW := newObj.(*workloadv1beta2.AppWrapper)
	log.FromContext(ctx).V(2).Info("Validating update", "job", newAW)
	allErrors := w.validateAppWrapperUpdate(oldAW, newAW)
	if w.Config.EnableKueueIntegrations {
		allErrors = append(allErrors, jobframework.ValidateJobOnUpdate((*wlc.AppWrapper)(oldAW), (*wlc.AppWrapper)(newAW))...)
	}
	return nil, allErrors.ToAggregate()
}

// ValidateDelete is a noop for us, but is required to implement the CustomValidator interface
func (w *AppWrapperWebhook) ValidateDelete(context.Context, runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// rbacs required to enable SubjectAccessReview
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=list

// validateAppWrapperCreate checks these invariants:
//  1. AppWrappers must not contain other AppWrappers
//  2. AppWrappers must only contain resources intended for their own namespace
//  3. AppWrappers must not contain any resources that the user could not create directly
//  4. Every PodSet must be well-formed: the Path must exist and must be parseable as a PodSpecTemplate
//  5. AppWrappers must contain between 1 and 8 PodSets (Kueue invariant)
func (w *AppWrapperWebhook) validateAppWrapperCreate(ctx context.Context, aw *workloadv1beta2.AppWrapper) field.ErrorList {
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
		if *gvk == wlc.GVK {
			allErrors = append(allErrors, field.Forbidden(compPath.Child("template"), "Nested AppWrappers are forbidden"))
		}

		// 2. Forbid creation of resources in other namespaces
		if unstruct.GetNamespace() != "" && unstruct.GetNamespace() != aw.Namespace {
			allErrors = append(allErrors, field.Forbidden(compPath.Child("template").Child("metadata").Child("namespace"),
				"AppWrappers cannot create objects in other namespaces"))
		}

		// 3. RBAC check: Perform SubjectAccessReview to verify user is entitled to create component
		if w.Config.UserRBACAdmissionCheck {
			ra := authv1.ResourceAttributes{
				Namespace: aw.Namespace,
				Verb:      "create",
				Group:     gvk.Group,
				Version:   gvk.Version,
				Resource:  w.lookupResource(gvk),
			}
			sar := &authv1.SubjectAccessReview{
				Spec: authv1.SubjectAccessReviewSpec{
					ResourceAttributes: &ra,
					User:               userInfo.Username,
					UID:                userInfo.UID,
					Groups:             userInfo.Groups,
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
					reason := fmt.Sprintf("User %v is not authorized to create %v in %v", userInfo.Username, ra.Resource, ra.Namespace)
					allErrors = append(allErrors, field.Forbidden(compPath.Child("template"), reason))
				}
			}
		}

		// 4. Every DeclaredPodSet must specify a path within Template to a v1.PodSpecTemplate
		podSetsPath := compPath.Child("podSets")
		for psIdx, ps := range component.DeclaredPodSets {
			podSetPath := podSetsPath.Index(psIdx)
			if ps.Path == "" {
				allErrors = append(allErrors, field.Required(podSetPath.Child("path"), "podspec must specify path"))
			}
			if _, err := utils.GetPodTemplateSpec(unstruct, ps.Path); err != nil {
				allErrors = append(allErrors, field.Invalid(podSetPath.Child("path"), ps.Path, fmt.Sprintf("path does not refer to a v1.PodSpecTemplate: %v", err)))
			}
		}

		// 5. Validate PodSets for known GVKs
		if inferred, err := utils.InferPodSets(unstruct); err != nil {
			allErrors = append(allErrors, field.Invalid(compPath.Child("template"), component.Template, fmt.Sprintf("error inferring PodSets: %v", err)))
		} else {
			if len(component.DeclaredPodSets) > len(inferred) {
				podSpecCount += len(component.DeclaredPodSets)
			} else {
				podSpecCount += len(inferred)
			}
			if err := utils.ValidatePodSets(component.DeclaredPodSets, inferred); err != nil {
				allErrors = append(allErrors, field.Invalid(podSetsPath, component.DeclaredPodSets, err.Error()))
			}
		}
	}

	// 6. Enforce Kueue limitation that 0 < podSpecCount <= 8
	if podSpecCount == 0 {
		allErrors = append(allErrors, field.Invalid(componentsPath, components, "components contains no podspecs"))
	}
	if podSpecCount > 8 {
		allErrors = append(allErrors, field.Invalid(componentsPath, components, fmt.Sprintf("components contains %v podspecs; at most 8 are allowed", podSpecCount)))
	}

	return allErrors
}

// validateAppWrapperUpdate enforces deep immutablity of all fields that were validated by validateAppWrapperCreate
func (w *AppWrapperWebhook) validateAppWrapperUpdate(old *workloadv1beta2.AppWrapper, new *workloadv1beta2.AppWrapper) field.ErrorList {
	allErrors := field.ErrorList{}
	msg := "attempt to change immutable field"
	componentsPath := field.NewPath("spec").Child("components")
	if len(old.Spec.Components) != len(new.Spec.Components) {
		return field.ErrorList{field.Forbidden(componentsPath, msg)}
	}
	for idx := range new.Spec.Components {
		compPath := componentsPath.Index(idx)
		oldComponent := old.Spec.Components[idx]
		newComponent := new.Spec.Components[idx]
		if !bytes.Equal(oldComponent.Template.Raw, newComponent.Template.Raw) {
			allErrors = append(allErrors, field.Forbidden(compPath.Child("template").Child("raw"), msg))
		}
		if len(oldComponent.DeclaredPodSets) != len(newComponent.DeclaredPodSets) {
			allErrors = append(allErrors, field.Forbidden(compPath.Child("podsets"), msg))
		} else {
			for psIdx := range newComponent.DeclaredPodSets {
				if utils.Replicas(oldComponent.DeclaredPodSets[psIdx]) != utils.Replicas(newComponent.DeclaredPodSets[psIdx]) {
					allErrors = append(allErrors, field.Forbidden(compPath.Child("podsets").Index(psIdx).Child("replicas"), msg))
				}
				if oldComponent.DeclaredPodSets[psIdx].Path != newComponent.DeclaredPodSets[psIdx].Path {
					allErrors = append(allErrors, field.Forbidden(compPath.Child("podsets").Index(psIdx).Child("path"), msg))
				}
			}
		}
	}

	// ensure user name and id are not mutated
	if old.Labels[AppWrapperUsernameLabel] != new.Labels[AppWrapperUsernameLabel] {
		allErrors = append(allErrors, field.Forbidden(field.NewPath("metadata").Child("labels").Key(AppWrapperUsernameLabel), msg))
	}
	if old.Labels[AppWrapperUserIDLabel] != new.Labels[AppWrapperUserIDLabel] {
		allErrors = append(allErrors, field.Forbidden(field.NewPath("metadata").Child("labels").Key(AppWrapperUserIDLabel), msg))
	}

	// ensure managedBy field is immutable
	if old.Spec.ManagedBy != new.Spec.ManagedBy {
		allErrors = append(allErrors, field.Forbidden(field.NewPath("spec").Child("managedBy"), msg))
	}

	return allErrors
}

func (w *AppWrapperWebhook) lookupResource(gvk *schema.GroupVersionKind) string {
	if known, ok := w.kindToResourceCache[gvk.String()]; ok {
		return known
	}
	resources, err := w.DiscoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		return "*"
	}
	for _, r := range resources.APIResources {
		if r.Kind == gvk.Kind {
			w.kindToResourceCache[gvk.String()] = r.Name
			return r.Name
		}
	}
	return "*"
}

func SetupAppWrapperWebhook(mgr ctrl.Manager, awConfig *config.AppWrapperConfig) error {
	kubeClient, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	wh := &AppWrapperWebhook{
		Config:                awConfig,
		client:                mgr.GetClient(),
		DiscoveryClient:       kubeClient.DiscoveryClient,
		SubjectAccessReviewer: kubeClient.AuthorizationV1().SubjectAccessReviews(),
		kindToResourceCache:   make(map[string]string),
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&workloadv1beta2.AppWrapper{}).
		WithDefaulter(wh).
		WithValidator(wh).
		Complete()
}
