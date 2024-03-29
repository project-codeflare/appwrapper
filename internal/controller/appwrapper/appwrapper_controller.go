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
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"sigs.k8s.io/kueue/pkg/controller/constants"
	"sigs.k8s.io/kueue/pkg/podset"
	utilmaps "sigs.k8s.io/kueue/pkg/util/maps"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	"github.com/project-codeflare/appwrapper/pkg/config"
	"github.com/project-codeflare/appwrapper/pkg/utils"
)

const (
	AppWrapperLabel     = "workload.codeflare.dev/appwrapper"
	AppWrapperFinalizer = "workload.codeflare.dev/finalizer"
	childJobQueueName   = "workload.codeflare.dev.admitted"
)

// AppWrapperReconciler reconciles an appwrapper
type AppWrapperReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *config.AppWrapperConfig
}

type podStatusSummary struct {
	expected  int32
	pending   int32
	running   int32
	succeeded int32
	failed    int32
}

// permission to fully control appwrappers
//+kubebuilder:rbac:groups=workload.codeflare.dev,resources=appwrappers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workload.codeflare.dev,resources=appwrappers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workload.codeflare.dev,resources=appwrappers/finalizers,verbs=update

// permission to edit wrapped resources: pods, services, jobs, podgroups, pytorchjobs, rayclusters

//+kubebuilder:rbac:groups="",resources=pods;services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=scheduling.sigs.k8s.io,resources=podgroups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=scheduling.x-k8s.io,resources=podgroups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubeflow.org,resources=pytorchjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.ray.io,resources=rayclusters,verbs=get;list;watch;create;update;patch;delete

// Reconcile reconciles an appwrapper
// Please see [aw-states] for documentation of this method.
//
// [aw-states]: https://github.com/project-codeflare/appwrapper/blob/main/docs/state-diagram.md
//
//gocyclo:ignore
func (r *AppWrapperReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	aw := &workloadv1beta2.AppWrapper{}
	if err := r.Get(ctx, req.NamespacedName, aw); err != nil {
		return ctrl.Result{}, nil
	}

	// handle deletion first
	if !aw.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(aw, AppWrapperFinalizer) {
			statusUpdated := false
			if meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)) {
				if !r.deleteComponents(ctx, aw) {
					// one or more components are still terminating
					if aw.Status.Phase != workloadv1beta2.AppWrapperTerminating {
						// Set Phase for better UX, but ignore errors. We still want to requeue after 5 seconds (not immediately)
						aw.Status.Phase = workloadv1beta2.AppWrapperTerminating
						_ = r.Status().Update(ctx, aw)
					}
					return ctrl.Result{RequeueAfter: 5 * time.Second}, nil // check after a short while
				}
				meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
					Type:    string(workloadv1beta2.ResourcesDeployed),
					Status:  metav1.ConditionFalse,
					Reason:  string(workloadv1beta2.AppWrapperTerminating),
					Message: "Resources successfully deleted",
				})
				statusUpdated = true
			}

			if meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.QuotaReserved)) {
				meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
					Type:    string(workloadv1beta2.QuotaReserved),
					Status:  metav1.ConditionFalse,
					Reason:  string(workloadv1beta2.AppWrapperTerminating),
					Message: "No resources deployed",
				})
				statusUpdated = true
			}
			if statusUpdated {
				if err := r.Status().Update(ctx, aw); err != nil {
					return ctrl.Result{}, err
				}
			}

			if controllerutil.RemoveFinalizer(aw, AppWrapperFinalizer) {
				if err := r.Update(ctx, aw); err != nil {
					return ctrl.Result{}, err
				}
				log.FromContext(ctx).Info("Deleted")
			}
		}
		return ctrl.Result{}, nil
	}

	switch aw.Status.Phase {

	case workloadv1beta2.AppWrapperEmpty: // initial state, inject finalizer
		if controllerutil.AddFinalizer(aw, AppWrapperFinalizer) {
			if err := r.Update(ctx, aw); err != nil {
				return ctrl.Result{}, err
			}
		}
		return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperSuspended)

	case workloadv1beta2.AppWrapperSuspended: // no components deployed
		if aw.Spec.Suspend {
			return ctrl.Result{}, nil // remain suspended
		}
		// begin deployment
		meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
			Type:    string(workloadv1beta2.QuotaReserved),
			Status:  metav1.ConditionTrue,
			Reason:  string(workloadv1beta2.AppWrapperResuming),
			Message: "Suspend is false",
		})
		meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
			Type:    string(workloadv1beta2.ResourcesDeployed),
			Status:  metav1.ConditionTrue,
			Reason:  string(workloadv1beta2.AppWrapperResuming),
			Message: "Suspend is false",
		})
		meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
			Type:    string(workloadv1beta2.PodsReady),
			Status:  metav1.ConditionFalse,
			Reason:  string(workloadv1beta2.AppWrapperResuming),
			Message: "Suspend is false",
		})
		meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
			Type:    string(workloadv1beta2.Unhealthy),
			Status:  metav1.ConditionFalse,
			Reason:  string(workloadv1beta2.AppWrapperResuming),
			Message: "Suspend is false",
		})
		return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperResuming)

	case workloadv1beta2.AppWrapperResuming: // deploying components
		if aw.Spec.Suspend {
			return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperSuspending) // abort deployment
		}
		err, fatal := r.createComponents(ctx, aw)
		if err != nil {
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.Unhealthy),
				Status:  metav1.ConditionTrue,
				Reason:  "CreateFailed",
				Message: fmt.Sprintf("error creating components: %v", err),
			})
			if fatal {
				return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperFailed) // abort on fatal error
			} else {
				return r.resetOrFail(ctx, aw)
			}
		}
		return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperRunning)

	case workloadv1beta2.AppWrapperRunning: // components deployed
		if aw.Spec.Suspend {
			return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperSuspending) // begin undeployment
		}
		podStatus, err := r.workloadStatus(ctx, aw)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Handle Success
		if podStatus.succeeded >= podStatus.expected && (podStatus.pending+podStatus.running+podStatus.failed == 0) {
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.QuotaReserved),
				Status:  metav1.ConditionFalse,
				Reason:  string(workloadv1beta2.AppWrapperSucceeded),
				Message: fmt.Sprintf("%v pods succeeded and no running, pending, or failed pods", podStatus.succeeded),
			})
			return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperSucceeded)
		}

		// Handle Failed Pods
		if podStatus.failed > 0 {
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:   string(workloadv1beta2.Unhealthy),
				Status: metav1.ConditionTrue,
				Reason: "FoundFailedPods",
				// Intentionally no detailed message with failed pod count, since changing the message resets the transition time
			})

			// Grace period to give the resource controller a chance to correct the failure
			whenDetected := meta.FindStatusCondition(aw.Status.Conditions, string(workloadv1beta2.Unhealthy)).LastTransitionTime
			gracePeriod := r.failureGraceDuration(ctx, aw)
			now := time.Now()
			deadline := whenDetected.Add(gracePeriod)
			if now.Before(deadline) {
				return ctrl.Result{RequeueAfter: deadline.Sub(now)}, r.Status().Update(ctx, aw)
			} else {
				return r.resetOrFail(ctx, aw)
			}
		}

		clearCondition(aw, workloadv1beta2.Unhealthy, "FoundNoFailedPods", "")

		if podStatus.running+podStatus.succeeded >= podStatus.expected {
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.PodsReady),
				Status:  metav1.ConditionTrue,
				Reason:  "SufficientPodsReady",
				Message: fmt.Sprintf("%v pods running; %v pods succeeded", podStatus.running, podStatus.succeeded),
			})
			return ctrl.Result{RequeueAfter: time.Minute}, r.Status().Update(ctx, aw)
		}

		// Not ready yet; either continue to wait or giveup if the warmup period has expired
		podDetailsMessage := fmt.Sprintf("%v pods pending; %v pods running; %v pods succeeded", podStatus.pending, podStatus.running, podStatus.succeeded)
		clearCondition(aw, workloadv1beta2.PodsReady, "InsufficientPodsReady", podDetailsMessage)
		whenDeployed := meta.FindStatusCondition(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)).LastTransitionTime
		warmupDuration := r.warmupGraceDuration(ctx, aw)
		if time.Now().Before(whenDeployed.Add(warmupDuration)) {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, r.Status().Update(ctx, aw)
		} else {
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.Unhealthy),
				Status:  metav1.ConditionTrue,
				Reason:  "InsufficientPodsReady",
				Message: podDetailsMessage,
			})
			return r.resetOrFail(ctx, aw)
		}

	case workloadv1beta2.AppWrapperSuspending: // undeploying components
		// finish undeploying components irrespective of desired state (suspend bit)
		if meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)) {
			if !r.deleteComponents(ctx, aw) {
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.ResourcesDeployed),
				Status:  metav1.ConditionFalse,
				Reason:  string(workloadv1beta2.AppWrapperSuspended),
				Message: "Suspend is true",
			})
		}
		meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
			Type:    string(workloadv1beta2.QuotaReserved),
			Status:  metav1.ConditionFalse,
			Reason:  string(workloadv1beta2.AppWrapperSuspended),
			Message: "Suspend is true",
		})
		clearCondition(aw, workloadv1beta2.PodsReady, string(workloadv1beta2.AppWrapperSuspended), "")
		clearCondition(aw, workloadv1beta2.Unhealthy, string(workloadv1beta2.AppWrapperSuspended), "")
		return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperSuspended)

	case workloadv1beta2.AppWrapperResetting:
		if aw.Spec.Suspend {
			return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperSuspending) // Suspending trumps Resetting
		}

		clearCondition(aw, workloadv1beta2.PodsReady, string(workloadv1beta2.AppWrapperResetting), "")
		if meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)) {
			if !r.deleteComponents(ctx, aw) {
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.ResourcesDeployed),
				Status:  metav1.ConditionFalse,
				Reason:  string(workloadv1beta2.AppWrapperResetting),
				Message: "Resources deleted for resetting AppWrapper",
			})
		}

		// Pause before transitioning to Resuming to heuristically allow transient system problems to subside
		whenReset := meta.FindStatusCondition(aw.Status.Conditions, string(workloadv1beta2.Unhealthy)).LastTransitionTime
		pauseDuration := r.resettingPauseDuration(ctx, aw)
		now := time.Now()
		deadline := whenReset.Add(pauseDuration)
		if now.Before(deadline) {
			return ctrl.Result{RequeueAfter: deadline.Sub(now)}, r.Status().Update(ctx, aw)
		}

		meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
			Type:    string(workloadv1beta2.ResourcesDeployed),
			Status:  metav1.ConditionTrue,
			Reason:  string(workloadv1beta2.AppWrapperResuming),
			Message: "Reset complete; resuming",
		})
		return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperResuming)

	case workloadv1beta2.AppWrapperFailed:
		if meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)) {
			if !r.deleteComponents(ctx, aw) {
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.ResourcesDeployed),
				Status:  metav1.ConditionFalse,
				Reason:  string(workloadv1beta2.AppWrapperFailed),
				Message: "Resources deleted for failed AppWrapper",
			})
		}
		meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
			Type:    string(workloadv1beta2.QuotaReserved),
			Status:  metav1.ConditionFalse,
			Reason:  string(workloadv1beta2.AppWrapperFailed),
			Message: "No resources deployed",
		})
		return ctrl.Result{}, r.Status().Update(ctx, aw)
	}

	return ctrl.Result{}, nil
}

// podMapFunc maps pods to appwrappers
func (r *AppWrapperReconciler) podMapFunc(ctx context.Context, obj client.Object) []reconcile.Request {
	pod := obj.(*v1.Pod)
	if name, ok := pod.Labels[AppWrapperLabel]; ok {
		if pod.Status.Phase == v1.PodSucceeded {
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: pod.Namespace, Name: name}}}
		}
	}
	return nil
}

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

func (r *AppWrapperReconciler) updateStatus(ctx context.Context, aw *workloadv1beta2.AppWrapper, phase workloadv1beta2.AppWrapperPhase) (ctrl.Result, error) {
	aw.Status.Phase = phase
	if err := r.Status().Update(ctx, aw); err != nil {
		return ctrl.Result{}, err
	}
	log.FromContext(ctx).Info(string(phase), "phase", phase)
	return ctrl.Result{}, nil
}

func (r *AppWrapperReconciler) resetOrFail(ctx context.Context, aw *workloadv1beta2.AppWrapper) (ctrl.Result, error) {
	maxRetries := r.retryLimit(ctx, aw)
	if aw.Status.Retries < maxRetries {
		aw.Status.Retries += 1
		return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperResetting)
	} else {
		return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperFailed)
	}
}

func (r *AppWrapperReconciler) workloadStatus(ctx context.Context, aw *workloadv1beta2.AppWrapper) (*podStatusSummary, error) {
	pods := &v1.PodList{}
	if err := r.List(ctx, pods,
		client.InNamespace(aw.Namespace),
		client.MatchingLabels{AppWrapperLabel: aw.Name}); err != nil {
		return nil, err
	}
	summary := &podStatusSummary{expected: utils.ExpectedPodCount(aw)}

	for _, pod := range pods.Items {
		switch pod.Status.Phase {
		case v1.PodPending:
			summary.pending += 1
		case v1.PodRunning:
			summary.running += 1
		case v1.PodSucceeded:
			summary.succeeded += 1
		case v1.PodFailed:
			summary.failed += 1
		}
	}

	return summary, nil
}

func (r *AppWrapperReconciler) warmupGraceDuration(ctx context.Context, aw *workloadv1beta2.AppWrapper) time.Duration {
	if userPeriod, ok := aw.Annotations[workloadv1beta2.WarmupGracePeriodDurationAnnotation]; ok {
		if duration, err := time.ParseDuration(userPeriod); err == nil {
			return duration
		} else {
			log.FromContext(ctx).Info("Malformed warmup period annotation", "annotation", userPeriod, "error", err)
		}
	}
	return r.Config.FaultTolerance.WarmupGracePeriod
}

func (r *AppWrapperReconciler) failureGraceDuration(ctx context.Context, aw *workloadv1beta2.AppWrapper) time.Duration {
	if userPeriod, ok := aw.Annotations[workloadv1beta2.FailureGracePeriodDurationAnnotation]; ok {
		if duration, err := time.ParseDuration(userPeriod); err == nil {
			return duration
		} else {
			log.FromContext(ctx).Info("Malformed grace period annotation", "annotation", userPeriod, "error", err)
		}
	}
	return r.Config.FaultTolerance.FailureGracePeriod
}

func (r *AppWrapperReconciler) retryLimit(ctx context.Context, aw *workloadv1beta2.AppWrapper) int32 {
	if userLimit, ok := aw.Annotations[workloadv1beta2.RetryLimitAnnotation]; ok {
		if limit, err := strconv.Atoi(userLimit); err == nil {
			return int32(limit)
		} else {
			log.FromContext(ctx).Info("Malformed retry limit annotation", "annotation", userLimit, "error", err)
		}
	}
	return r.Config.FaultTolerance.RetryLimit
}

func (r *AppWrapperReconciler) resettingPauseDuration(ctx context.Context, aw *workloadv1beta2.AppWrapper) time.Duration {
	if userPeriod, ok := aw.Annotations[workloadv1beta2.ResetPauseDurationAnnotation]; ok {
		if duration, err := time.ParseDuration(userPeriod); err == nil {
			return duration
		} else {
			log.FromContext(ctx).Info("Malformed reset pause annotation", "annotation", userPeriod, "error", err)
		}
	}
	return r.Config.FaultTolerance.ResetPause
}

func clearCondition(aw *workloadv1beta2.AppWrapper, condition workloadv1beta2.AppWrapperCondition, reason string, message string) {
	if meta.IsStatusConditionTrue(aw.Status.Conditions, string(condition)) {
		meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
			Type:    string(condition),
			Status:  metav1.ConditionFalse,
			Reason:  reason,
			Message: message,
		})
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppWrapperReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadv1beta2.AppWrapper{}).
		Watches(&v1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.podMapFunc)).
		Complete(r)
}
