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
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/kueue/pkg/controller/jobframework"
	utilmaps "sigs.k8s.io/kueue/pkg/util/maps"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	wlc "github.com/project-codeflare/appwrapper/internal/controller/workload"
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
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
	Config   *config.AppWrapperConfig
}

type podStatusSummary struct {
	expected        int32
	pending         int32
	running         int32
	succeeded       int32
	failed          int32
	terminalFailure bool
	unhealthyNodes  sets.Set[string]
}

type componentStatusSummary struct {
	expected int32
	deployed int32
	failed   int32
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
//+kubebuilder:rbac:groups=ray.io,resources=rayclusters;rayjobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile reconciles an appwrapper
// Please see [aw-states] for documentation of this method.
//
// [aw-states]: https://project-codeflare.github.io/appwrapper/arch-controller/#framework-controller
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
			orig := copyForStatusPatch(aw)
			if meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)) {
				if !r.deleteComponents(ctx, aw) {
					// one or more components are still terminating
					if aw.Status.Phase != workloadv1beta2.AppWrapperTerminating {
						// Set Phase for better UX, but ignore errors. We still want to requeue after 5 seconds (not immediately)
						aw.Status.Phase = workloadv1beta2.AppWrapperTerminating
						_ = r.Status().Patch(ctx, aw, client.MergeFrom(orig))
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
				if err := r.Status().Patch(ctx, aw, client.MergeFrom(orig)); err != nil {
					return ctrl.Result{}, err
				}
			}

			if controllerutil.RemoveFinalizer(aw, AppWrapperFinalizer) {
				if err := r.Update(ctx, aw); err != nil {
					return ctrl.Result{}, err
				}
				log.FromContext(ctx).Info("Finalizer Deleted")
			}
		}
		return ctrl.Result{}, nil
	}

	switch aw.Status.Phase {

	case workloadv1beta2.AppWrapperEmpty: // initial state
		if !controllerutil.ContainsFinalizer(aw, AppWrapperFinalizer) {
			// The AppWrapperFinalizer is added by our webhook, so if we get here it means that we are
			// running in dev mode (`make run`) which disables the webhook. To make dev mode as
			// useful as possible, replicate as much of AppWrapperWebhook.Default() as we can without having the admission.Request.
			if r.Config.EnableKueueIntegrations {
				if r.Config.DefaultQueueName != "" {
					aw.Labels = utilmaps.MergeKeepFirst(aw.Labels, map[string]string{"kueue.x-k8s.io/queue-name": r.Config.DefaultQueueName})
				}
				jobframework.ApplyDefaultForSuspend((*wlc.AppWrapper)(aw), r.Config.KueueJobReconciller.ManageJobsWithoutQueueName)
			}
			controllerutil.AddFinalizer(aw, AppWrapperFinalizer)
			if err := r.Update(ctx, aw); err != nil {
				return ctrl.Result{}, err
			}
			log.FromContext(ctx).Info("No webhook: applied default initializations")
		}

		orig := copyForStatusPatch(aw)
		if err := utils.EnsureComponentStatusInitialized(aw); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, r.transitionToPhase(ctx, orig, aw, workloadv1beta2.AppWrapperSuspended)

	case workloadv1beta2.AppWrapperSuspended: // no components deployed
		if aw.Spec.Suspend {
			return ctrl.Result{}, nil // remain suspended
		}

		// begin deployment
		orig := copyForStatusPatch(aw)
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
		return ctrl.Result{}, r.transitionToPhase(ctx, orig, aw, workloadv1beta2.AppWrapperResuming)

	case workloadv1beta2.AppWrapperResuming: // deploying components
		if aw.Spec.Suspend {
			return ctrl.Result{}, r.transitionToPhase(ctx, copyForStatusPatch(aw), aw, workloadv1beta2.AppWrapperSuspending) // abort deployment
		}
		err, fatal := r.createComponents(ctx, aw) // NOTE: createComponents applies patches to aw.Status incrementally as resources are created
		orig := copyForStatusPatch(aw)
		if err != nil {
			if !fatal {
				startTime := meta.FindStatusCondition(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)).LastTransitionTime
				graceDuration := r.admissionGraceDuration(ctx, aw)
				if time.Now().Before(startTime.Add(graceDuration)) {
					// be patient; non-fatal error; requeue and keep trying
					return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
				}
			}
			detailMsg := fmt.Sprintf("error creating components: %v", err)
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.Unhealthy),
				Status:  metav1.ConditionTrue,
				Reason:  "CreateFailed",
				Message: detailMsg,
			})
			r.Recorder.Event(aw, v1.EventTypeNormal, string(workloadv1beta2.Unhealthy), "CreateFailed: "+detailMsg)
			if fatal {
				return ctrl.Result{}, r.transitionToPhase(ctx, orig, aw, workloadv1beta2.AppWrapperFailed) // always move to failed on fatal error
			} else {
				return ctrl.Result{}, r.resetOrFail(ctx, orig, aw, false, 1)
			}
		}
		return ctrl.Result{}, r.transitionToPhase(ctx, orig, aw, workloadv1beta2.AppWrapperRunning)

	case workloadv1beta2.AppWrapperRunning: // components deployed
		orig := copyForStatusPatch(aw)
		if aw.Spec.Suspend {
			return ctrl.Result{}, r.transitionToPhase(ctx, orig, aw, workloadv1beta2.AppWrapperSuspending) // begin undeployment
		}

		// Gather status information at the Component and Pod level.
		compStatus, err := r.getComponentStatus(ctx, aw)
		if err != nil {
			return ctrl.Result{}, err
		}
		podStatus, err := r.getPodStatus(ctx, aw)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Detect externally deleted components and transition to Failed with no GracePeriod or retry
		detailMsg := fmt.Sprintf("Only found %v deployed components, but was expecting %v", compStatus.deployed, compStatus.expected)
		if compStatus.deployed != compStatus.expected {
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.Unhealthy),
				Status:  metav1.ConditionTrue,
				Reason:  "MissingComponent",
				Message: detailMsg,
			})
			r.Recorder.Event(aw, v1.EventTypeNormal, string(workloadv1beta2.Unhealthy), "MissingComponent: "+detailMsg)
			return ctrl.Result{}, r.transitionToPhase(ctx, orig, aw, workloadv1beta2.AppWrapperFailed)
		}

		// If a component's controller has put it into a failed state, we do not need
		// to allow a grace period.  The situation will not self-correct.
		detailMsg = fmt.Sprintf("Found %v failed components", compStatus.failed)
		if compStatus.failed > 0 {
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.Unhealthy),
				Status:  metav1.ConditionTrue,
				Reason:  "FailedComponent",
				Message: detailMsg,
			})
			r.Recorder.Event(aw, v1.EventTypeNormal, string(workloadv1beta2.Unhealthy), "FailedComponent: "+detailMsg)
			return ctrl.Result{}, r.resetOrFail(ctx, orig, aw, podStatus.terminalFailure, 1)
		}

		// Handle Success
		if podStatus.succeeded >= podStatus.expected && (podStatus.pending+podStatus.running+podStatus.failed == 0) {
			msg := fmt.Sprintf("%v pods succeeded and no running, pending, or failed pods", podStatus.succeeded)
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.QuotaReserved),
				Status:  metav1.ConditionFalse,
				Reason:  string(workloadv1beta2.AppWrapperSucceeded),
				Message: msg,
			})
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.ResourcesDeployed),
				Status:  metav1.ConditionTrue,
				Reason:  string(workloadv1beta2.AppWrapperSucceeded),
				Message: msg,
			})
			return ctrl.Result{}, r.transitionToPhase(ctx, orig, aw, workloadv1beta2.AppWrapperSucceeded)
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
				return requeueAfter(deadline.Sub(now), r.Status().Patch(ctx, aw, client.MergeFrom(orig)))
			} else {
				r.Recorder.Eventf(aw, v1.EventTypeNormal, string(workloadv1beta2.Unhealthy), "FoundFailedPods: %v failed pods", podStatus.failed)
				return ctrl.Result{}, r.resetOrFail(ctx, orig, aw, podStatus.terminalFailure, 1)
			}
		}

		// Initiate migration of workloads that are using resources that Autopilot has flagged as unhealthy
		detailMsg = fmt.Sprintf("Workload contains pods using unhealthy resources on Nodes: %v", podStatus.unhealthyNodes)
		if len(podStatus.unhealthyNodes) > 0 {
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.Unhealthy),
				Status:  metav1.ConditionTrue,
				Reason:  "AutopilotUnhealthy",
				Message: detailMsg,
			})
			r.Recorder.Event(aw, v1.EventTypeNormal, string(workloadv1beta2.Unhealthy), detailMsg)
			return ctrl.Result{}, r.resetOrFail(ctx, orig, aw, false, 0) // Autopilot triggered evacuation does not increment retry count
		}

		clearCondition(aw, workloadv1beta2.Unhealthy, "FoundNoFailedPods", "")

		if podStatus.running+podStatus.succeeded >= podStatus.expected {
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.PodsReady),
				Status:  metav1.ConditionTrue,
				Reason:  "SufficientPodsReady",
				Message: fmt.Sprintf("%v pods running; %v pods succeeded", podStatus.running, podStatus.succeeded),
			})
			return requeueAfter(time.Minute, r.Status().Patch(ctx, aw, client.MergeFrom(orig)))
		}

		// Not ready yet; either continue to wait or giveup if the warmup period has expired
		podDetailsMessage := fmt.Sprintf("%v pods pending; %v pods running; %v pods succeeded", podStatus.pending, podStatus.running, podStatus.succeeded)
		clearCondition(aw, workloadv1beta2.PodsReady, "InsufficientPodsReady", podDetailsMessage)
		whenDeployed := meta.FindStatusCondition(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)).LastTransitionTime
		var graceDuration time.Duration
		if podStatus.pending+podStatus.running+podStatus.succeeded >= podStatus.expected {
			graceDuration = r.warmupGraceDuration(ctx, aw)
		} else {
			graceDuration = r.admissionGraceDuration(ctx, aw)
		}
		if time.Now().Before(whenDeployed.Add(graceDuration)) {
			return requeueAfter(5*time.Second, r.Status().Patch(ctx, aw, client.MergeFrom(orig)))
		} else {
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.Unhealthy),
				Status:  metav1.ConditionTrue,
				Reason:  "InsufficientPodsReady",
				Message: podDetailsMessage,
			})
			r.Recorder.Event(aw, v1.EventTypeNormal, string(workloadv1beta2.Unhealthy), "InsufficientPodsReady: "+podDetailsMessage)
			return ctrl.Result{}, r.resetOrFail(ctx, orig, aw, podStatus.terminalFailure, 1)
		}

	case workloadv1beta2.AppWrapperSuspending: // undeploying components
		orig := copyForStatusPatch(aw)
		// finish undeploying components irrespective of desired state (suspend bit)
		if meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)) {
			if !r.deleteComponents(ctx, aw) {
				return requeueAfter(5*time.Second, r.Status().Patch(ctx, aw, client.MergeFrom(orig)))
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
		return ctrl.Result{}, r.transitionToPhase(ctx, orig, aw, workloadv1beta2.AppWrapperSuspended)

	case workloadv1beta2.AppWrapperResetting:
		orig := copyForStatusPatch(aw)
		if aw.Spec.Suspend {
			return ctrl.Result{}, r.transitionToPhase(ctx, orig, aw, workloadv1beta2.AppWrapperSuspending) // Suspending trumps Resetting
		}

		clearCondition(aw, workloadv1beta2.PodsReady, string(workloadv1beta2.AppWrapperResetting), "")
		if meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)) {
			if !r.deleteComponents(ctx, aw) {
				return requeueAfter(5*time.Second, r.Status().Patch(ctx, aw, client.MergeFrom(orig)))
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
		pauseDuration := r.retryPauseDuration(ctx, aw)
		now := time.Now()
		deadline := whenReset.Add(pauseDuration)
		if now.Before(deadline) {
			return requeueAfter(deadline.Sub(now), r.Status().Patch(ctx, aw, client.MergeFrom(orig)))
		}

		meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
			Type:    string(workloadv1beta2.ResourcesDeployed),
			Status:  metav1.ConditionTrue,
			Reason:  string(workloadv1beta2.AppWrapperResuming),
			Message: "Reset complete; resuming",
		})
		return ctrl.Result{}, r.transitionToPhase(ctx, orig, aw, workloadv1beta2.AppWrapperResuming)

	case workloadv1beta2.AppWrapperFailed:
		// Support for debugging failed jobs.
		// When an appwrapper is annotated with a non-zero debugging delay,
		// we hold quota for the delay period and do not delete the resources of
		// a failed appwrapper unless Kueue preempts it by setting Suspend to true.
		deletionDelay := r.deletionOnFailureGraceDuration(ctx, aw)

		orig := copyForStatusPatch(aw)
		if deletionDelay > 0 && !aw.Spec.Suspend {
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.DeletingResources),
				Status:  metav1.ConditionFalse,
				Reason:  "DeletionPaused",
				Message: fmt.Sprintf("%v has value %v", workloadv1beta2.DeletionOnFailureGracePeriodAnnotation, deletionDelay),
			})
			whenDelayed := meta.FindStatusCondition(aw.Status.Conditions, string(workloadv1beta2.DeletingResources)).LastTransitionTime

			now := time.Now()
			deadline := whenDelayed.Add(deletionDelay)
			if now.Before(deadline) {
				return requeueAfter(deadline.Sub(now), r.Status().Patch(ctx, aw, client.MergeFrom(orig)))
			}
		}

		if meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)) {
			if !r.deleteComponents(ctx, aw) {
				return requeueAfter(5*time.Second, r.Status().Patch(ctx, aw, client.MergeFrom(orig)))
			}
			msg := "Resources deleted for failed AppWrapper"
			if deletionDelay > 0 && aw.Spec.Suspend {
				msg = "Kueue forced resource deletion by suspending AppWrapper"
			}
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.ResourcesDeployed),
				Status:  metav1.ConditionFalse,
				Reason:  string(workloadv1beta2.AppWrapperFailed),
				Message: msg,
			})
		}
		meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
			Type:    string(workloadv1beta2.QuotaReserved),
			Status:  metav1.ConditionFalse,
			Reason:  string(workloadv1beta2.AppWrapperFailed),
			Message: "No resources deployed",
		})
		return ctrl.Result{}, r.Status().Patch(ctx, aw, client.MergeFrom(orig))

	case workloadv1beta2.AppWrapperSucceeded:
		if meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)) {
			deletionDelay := r.timeToLiveAfterSucceededDuration(ctx, aw)
			whenSucceeded := meta.FindStatusCondition(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed)).LastTransitionTime
			now := time.Now()
			deadline := whenSucceeded.Add(deletionDelay)
			if now.Before(deadline) {
				return requeueAfter(deadline.Sub(now), nil)
			}

			orig := copyForStatusPatch(aw)
			if !r.deleteComponents(ctx, aw) {
				return requeueAfter(5*time.Second, r.Status().Patch(ctx, aw, client.MergeFrom(orig)))
			}
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.ResourcesDeployed),
				Status:  metav1.ConditionFalse,
				Reason:  string(workloadv1beta2.AppWrapperSucceeded),
				Message: fmt.Sprintf("Time to live after success of %v expired", deletionDelay),
			})
			return ctrl.Result{}, r.Status().Patch(ctx, aw, client.MergeFrom(orig))
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *AppWrapperReconciler) transitionToPhase(ctx context.Context, orig *workloadv1beta2.AppWrapper, modified *workloadv1beta2.AppWrapper, phase workloadv1beta2.AppWrapperPhase) error {
	modified.Status.Phase = phase
	if err := r.Status().Patch(ctx, modified, client.MergeFrom(orig)); err != nil {
		return err
	}
	log.FromContext(ctx).Info(string(phase), "phase", phase)
	return nil
}

func (r *AppWrapperReconciler) resetOrFail(ctx context.Context, orig *workloadv1beta2.AppWrapper, aw *workloadv1beta2.AppWrapper, terminalFailure bool, retryIncrement int32) error {
	maxRetries := r.retryLimit(ctx, aw)
	if !terminalFailure && aw.Status.Retries < maxRetries {
		aw.Status.Retries += retryIncrement
		return r.transitionToPhase(ctx, orig, aw, workloadv1beta2.AppWrapperResetting)
	} else {
		return r.transitionToPhase(ctx, orig, aw, workloadv1beta2.AppWrapperFailed)
	}
}

//gocyclo:ignore
func (r *AppWrapperReconciler) getPodStatus(ctx context.Context, aw *workloadv1beta2.AppWrapper) (*podStatusSummary, error) {
	pods := &v1.PodList{}
	if err := r.List(ctx, pods,
		client.InNamespace(aw.Namespace),
		client.MatchingLabels{AppWrapperLabel: aw.Name}); err != nil {
		return nil, err
	}
	pc, err := utils.ExpectedPodCount(aw)
	if err != nil {
		return nil, err
	}
	summary := &podStatusSummary{expected: pc}
	checkUnhealthyNodes := r.Config.Autopilot != nil && r.Config.Autopilot.MonitorNodes

	for _, pod := range pods.Items {
		switch pod.Status.Phase {
		case v1.PodPending:
			summary.pending += 1
		case v1.PodRunning:
			if pod.DeletionTimestamp.IsZero() {
				summary.running += 1
				if checkUnhealthyNodes {
					unhealthyNodesMutex.RLock() // BEGIN CRITICAL SECTION
					if len(unhealthyNodes) > 0 {
						if resources, ok := unhealthyNodes[pod.Spec.NodeName]; ok {
							for badResource := range resources {
								for _, container := range pod.Spec.Containers {
									if limit, ok := container.Resources.Limits[v1.ResourceName(badResource)]; ok {
										if !limit.IsZero() {
											if summary.unhealthyNodes == nil {
												summary.unhealthyNodes = make(sets.Set[string])
											}
											summary.unhealthyNodes.Insert(pod.Spec.NodeName)
										}
									}
									if request, ok := container.Resources.Requests[v1.ResourceName(badResource)]; ok {
										if !request.IsZero() {
											if summary.unhealthyNodes == nil {
												summary.unhealthyNodes = make(sets.Set[string])
											}
											summary.unhealthyNodes.Insert(pod.Spec.NodeName)
										}
									}
								}
							}
						}
					}
					unhealthyNodesMutex.RUnlock() // END CRITICAL SECTION
				}
			}
		case v1.PodSucceeded:
			summary.succeeded += 1
		case v1.PodFailed:
			summary.failed += 1
			if terminalCodes := r.terminalExitCodes(ctx, aw); len(terminalCodes) > 0 {
				for _, containerStatus := range pod.Status.ContainerStatuses {
					if containerStatus.State.Terminated != nil {
						exitCode := containerStatus.State.Terminated.ExitCode
						if exitCode != 0 {
							for _, ec := range terminalCodes {
								if ec == int(exitCode) {
									summary.terminalFailure = true
									break
								}
							}
						}
					}
				}
			}
			if retryableCodes := r.retryableExitCodes(ctx, aw); len(retryableCodes) > 0 {
				for _, containerStatus := range pod.Status.ContainerStatuses {
					if containerStatus.State.Terminated != nil {
						exitCode := containerStatus.State.Terminated.ExitCode
						if exitCode != 0 {
							terminal := true
							for _, ec := range retryableCodes {
								if ec == int(exitCode) {
									terminal = false
									break
								}
							}
							if terminal {
								summary.terminalFailure = terminal
							}
						}
					}
				}
			}
		}
	}

	return summary, nil
}

//gocyclo:ignore
func (r *AppWrapperReconciler) getComponentStatus(ctx context.Context, aw *workloadv1beta2.AppWrapper) (*componentStatusSummary, error) {
	summary := &componentStatusSummary{expected: int32(len(aw.Status.ComponentStatus))}

	for componentIdx := range aw.Status.ComponentStatus {
		cs := &aw.Status.ComponentStatus[componentIdx]
		switch cs.APIVersion + ":" + cs.Kind {

		case "batch/v1:Job":
			obj := &batchv1.Job{}
			if err := r.Get(ctx, types.NamespacedName{Name: cs.Name, Namespace: aw.Namespace}, obj); err == nil {
				if obj.GetDeletionTimestamp().IsZero() {
					summary.deployed += 1

					// batch/v1 Jobs are failed when status.Conditions contains an entry with type "Failed" and status "True"
					for _, jc := range obj.Status.Conditions {
						if jc.Type == batchv1.JobFailed && jc.Status == v1.ConditionTrue {
							summary.failed += 1
						}
					}
				}

			} else if !apierrors.IsNotFound(err) {
				return nil, err
			}

		case "kubeflow.org/v1:PyTorchJob":
			obj := &unstructured.Unstructured{}
			obj.SetAPIVersion(cs.APIVersion)
			obj.SetKind(cs.Kind)
			if err := r.Get(ctx, types.NamespacedName{Name: cs.Name, Namespace: aw.Namespace}, obj); err == nil {
				if obj.GetDeletionTimestamp().IsZero() {
					summary.deployed += 1

					// PyTorchJob is failed if status.Conditions contains an entry with type "Failed" and status "True"
					status, ok := obj.UnstructuredContent()["status"]
					if !ok {
						continue
					}
					cond, ok := status.(map[string]interface{})["conditions"]
					if !ok {
						continue
					}
					condArray, ok := cond.([]interface{})
					if !ok {
						continue
					}
					for _, aCond := range condArray {
						if condMap, ok := aCond.(map[string]interface{}); ok {
							if condType, ok := condMap["type"]; ok && condType.(string) == "Failed" {
								if status, ok := condMap["status"]; ok && status.(string) == "True" {
									summary.failed += 1
								}
							}
						}
					}
				}
			} else if !apierrors.IsNotFound(err) {
				return nil, err
			}

		case "ray.io/v1:RayCluster":
			obj := &unstructured.Unstructured{}
			obj.SetAPIVersion(cs.APIVersion)
			obj.SetKind(cs.Kind)
			if err := r.Get(ctx, types.NamespacedName{Name: cs.Name, Namespace: aw.Namespace}, obj); err == nil {
				if obj.GetDeletionTimestamp().IsZero() {
					summary.deployed += 1

					/* Disabled because failed is not a terminal state.
					 *  We've observed RC transiently entering "failed" before becoming "ready" due to ingress not being ready
					 * TODO: Explore fixing in upstream projects.

					// RayCluster is failed if status.State is "failed"
					status, ok := obj.UnstructuredContent()["status"]
					if !ok {
						continue
					}
					state, ok := status.(map[string]interface{})["state"]
					if !ok {
						continue
					}
					if state.(string) == "failed" {
						summary.failed += 1
					}
					*/
				}
			} else if !apierrors.IsNotFound(err) {
				return nil, err
			}

		case "ray.io/v1:RayJob":
			obj := &unstructured.Unstructured{}
			obj.SetAPIVersion(cs.APIVersion)
			obj.SetKind(cs.Kind)
			if err := r.Get(ctx, types.NamespacedName{Name: cs.Name, Namespace: aw.Namespace}, obj); err == nil {
				if obj.GetDeletionTimestamp().IsZero() {
					summary.deployed += 1

					/* Disabled because we are not sure if failed is  a terminal state.
					 * TODO: Determine whether or not RayJob has the same issue as RayCluster

					// RayJob is failed if status.jobsStatus is "FAILED"
					status, ok := obj.UnstructuredContent()["status"]
					if !ok {
						continue
					}
					jobStatus, ok := status.(map[string]interface{})["jobStatus"]
					if !ok {
						continue
					}
					if jobStatus.(string) == "FAILED" {
						summary.failed += 1
					}
					*/
				}
			} else if !apierrors.IsNotFound(err) {
				return nil, err
			}

		default:
			obj := &metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{Kind: cs.Kind, APIVersion: cs.APIVersion}}
			if err := r.Get(ctx, types.NamespacedName{Name: cs.Name, Namespace: aw.Namespace}, obj); err == nil {
				if obj.GetDeletionTimestamp().IsZero() {
					summary.deployed += 1
				}
			} else if !apierrors.IsNotFound(err) {
				return nil, err
			}
		}
	}

	return summary, nil
}

func (r *AppWrapperReconciler) limitDuration(desired time.Duration) time.Duration {
	if desired < 0 {
		return 0 * time.Second
	} else if desired > r.Config.FaultTolerance.GracePeriodMaximum {
		return r.Config.FaultTolerance.GracePeriodMaximum
	} else {
		return desired
	}
}

func (r *AppWrapperReconciler) admissionGraceDuration(ctx context.Context, aw *workloadv1beta2.AppWrapper) time.Duration {
	if userPeriod, ok := aw.Annotations[workloadv1beta2.AdmissionGracePeriodDurationAnnotation]; ok {
		if duration, err := time.ParseDuration(userPeriod); err == nil {
			return r.limitDuration(duration)
		} else {
			log.FromContext(ctx).Error(err, "Malformed admission grace period annotation; using default", "annotation", userPeriod)
		}
	}
	return r.limitDuration(r.Config.FaultTolerance.AdmissionGracePeriod)
}

func (r *AppWrapperReconciler) warmupGraceDuration(ctx context.Context, aw *workloadv1beta2.AppWrapper) time.Duration {
	if userPeriod, ok := aw.Annotations[workloadv1beta2.WarmupGracePeriodDurationAnnotation]; ok {
		if duration, err := time.ParseDuration(userPeriod); err == nil {
			return r.limitDuration(duration)
		} else {
			log.FromContext(ctx).Error(err, "Malformed warmup grace period annotation; using default", "annotation", userPeriod)
		}
	}
	return r.limitDuration(r.Config.FaultTolerance.WarmupGracePeriod)
}

func (r *AppWrapperReconciler) failureGraceDuration(ctx context.Context, aw *workloadv1beta2.AppWrapper) time.Duration {
	if userPeriod, ok := aw.Annotations[workloadv1beta2.FailureGracePeriodDurationAnnotation]; ok {
		if duration, err := time.ParseDuration(userPeriod); err == nil {
			return r.limitDuration(duration)
		} else {
			log.FromContext(ctx).Error(err, "Malformed failure grace period annotation; using default", "annotation", userPeriod)
		}
	}
	return r.limitDuration(r.Config.FaultTolerance.FailureGracePeriod)
}

func (r *AppWrapperReconciler) retryLimit(ctx context.Context, aw *workloadv1beta2.AppWrapper) int32 {
	if userLimit, ok := aw.Annotations[workloadv1beta2.RetryLimitAnnotation]; ok {
		if limit, err := strconv.Atoi(userLimit); err == nil {
			return int32(limit)
		} else {
			log.FromContext(ctx).Error(err, "Malformed retry limit annotation; using default", "annotation", userLimit)
		}
	}
	return r.Config.FaultTolerance.RetryLimit
}

func (r *AppWrapperReconciler) retryPauseDuration(ctx context.Context, aw *workloadv1beta2.AppWrapper) time.Duration {
	if userPeriod, ok := aw.Annotations[workloadv1beta2.RetryPausePeriodDurationAnnotation]; ok {
		if duration, err := time.ParseDuration(userPeriod); err == nil {
			return r.limitDuration(duration)
		} else {
			log.FromContext(ctx).Error(err, "Malformed retry pause annotation; using default", "annotation", userPeriod)
		}
	}
	return r.limitDuration(r.Config.FaultTolerance.RetryPausePeriod)
}

func (r *AppWrapperReconciler) forcefulDeletionGraceDuration(ctx context.Context, aw *workloadv1beta2.AppWrapper) time.Duration {
	if userPeriod, ok := aw.Annotations[workloadv1beta2.ForcefulDeletionGracePeriodAnnotation]; ok {
		if duration, err := time.ParseDuration(userPeriod); err == nil {
			return r.limitDuration(duration)
		} else {
			log.FromContext(ctx).Error(err, "Malformed forceful deletion period annotation; using default", "annotation", userPeriod)
		}
	}
	return r.limitDuration(r.Config.FaultTolerance.ForcefulDeletionGracePeriod)
}

func (r *AppWrapperReconciler) deletionOnFailureGraceDuration(ctx context.Context, aw *workloadv1beta2.AppWrapper) time.Duration {
	if userPeriod, ok := aw.Annotations[workloadv1beta2.DeletionOnFailureGracePeriodAnnotation]; ok {
		if duration, err := time.ParseDuration(userPeriod); err == nil {
			return r.limitDuration(duration)
		} else {
			log.FromContext(ctx).Error(err, "Malformed deletion on failure grace period annotation; using default of 0", "annotation", userPeriod)
		}
	}
	return 0 * time.Second
}

func (r *AppWrapperReconciler) timeToLiveAfterSucceededDuration(ctx context.Context, aw *workloadv1beta2.AppWrapper) time.Duration {
	if userPeriod, ok := aw.Annotations[workloadv1beta2.SuccessTTLAnnotation]; ok {
		if duration, err := time.ParseDuration(userPeriod); err == nil {
			if duration > 0 && duration < r.Config.FaultTolerance.SuccessTTL {
				return duration
			}
		} else {
			log.FromContext(ctx).Error(err, "Malformed successTTL annotation; using default", "annotation", userPeriod)
		}
	}
	return r.Config.FaultTolerance.SuccessTTL
}

func (r *AppWrapperReconciler) terminalExitCodes(_ context.Context, aw *workloadv1beta2.AppWrapper) []int {
	ans := []int{}
	if exitCodeAnn, ok := aw.Annotations[workloadv1beta2.TerminalExitCodesAnnotation]; ok {
		exitCodes := strings.Split(exitCodeAnn, ",")
		for _, str := range exitCodes {
			exitCode, err := strconv.Atoi(str)
			if err == nil {
				ans = append(ans, exitCode)
			}
		}
	}
	return ans
}

func (r *AppWrapperReconciler) retryableExitCodes(_ context.Context, aw *workloadv1beta2.AppWrapper) []int {
	ans := []int{}
	if exitCodeAnn, ok := aw.Annotations[workloadv1beta2.RetryableExitCodesAnnotation]; ok {
		exitCodes := strings.Split(exitCodeAnn, ",")
		for _, str := range exitCodes {
			exitCode, err := strconv.Atoi(str)
			if err == nil {
				ans = append(ans, exitCode)
			}
		}
	}
	return ans
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

// podMapFunc maps pods to appwrappers and generates reconcile.Requests for those whose Status.Phase is PodSucceeded
func (r *AppWrapperReconciler) podMapFunc(ctx context.Context, obj client.Object) []reconcile.Request {
	pod := obj.(*v1.Pod)
	if name, ok := pod.Labels[AppWrapperLabel]; ok {
		if pod.Status.Phase == v1.PodSucceeded {
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: pod.Namespace, Name: name}}}
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppWrapperReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadv1beta2.AppWrapper{}).
		Watches(&v1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.podMapFunc)).
		Named("AppWrapper").
		Complete(r)
}

// copyForStatusPatch returns an AppWrapper with an empty Spec and a DeepCopy of orig's Status for use in a subsequent Status().Patch(...) call
func copyForStatusPatch(orig *workloadv1beta2.AppWrapper) *workloadv1beta2.AppWrapper {
	copy := workloadv1beta2.AppWrapper{
		TypeMeta:   orig.TypeMeta,
		ObjectMeta: orig.ObjectMeta,
		Status:     *orig.Status.DeepCopy(),
	}
	return &copy
}

// requeueAfter requeues the request after the specified duration
func requeueAfter(duration time.Duration, err error) (ctrl.Result, error) {
	if err != nil {
		// eliminate "Warning: Reconciler returned both a non-zero result and a non-nil error."
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: duration}, nil
}
