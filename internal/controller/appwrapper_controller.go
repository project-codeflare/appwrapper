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

	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	"sigs.k8s.io/kueue/pkg/controller/constants"
	"sigs.k8s.io/kueue/pkg/controller/jobframework"
	"sigs.k8s.io/kueue/pkg/podset"
	utilmaps "sigs.k8s.io/kueue/pkg/util/maps"
	"sigs.k8s.io/kueue/pkg/workload"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
)

const (
	AppWrapperLabel     = "workload.codeflare.dev/appwrapper"
	appWrapperFinalizer = "workload.codeflare.dev/finalizer"
	childJobQueueName   = "workload.codeflare.dev.admitted"
)

// AppWrapperReconciler reconciles an appwrapper
type AppWrapperReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *AppWrapperConfig
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

// permission to manipulate workloads controlling appwrapper components to enable admitting them to our pseudo-clusterqueue
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads,verbs=get
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads/status,verbs=get;update;patch

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
		if controllerutil.ContainsFinalizer(aw, appWrapperFinalizer) {
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

			if controllerutil.RemoveFinalizer(aw, appWrapperFinalizer) {
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
		if controllerutil.AddFinalizer(aw, appWrapperFinalizer) {
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
			Message: "AppWrapper was unsuspended by Kueue",
		})
		meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
			Type:    string(workloadv1beta2.ResourcesDeployed),
			Status:  metav1.ConditionTrue,
			Reason:  string(workloadv1beta2.AppWrapperResuming),
			Message: "AppWrapper was unsuspended by Kueue",
		})
		return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperResuming)

	case workloadv1beta2.AppWrapperResuming: // deploying components
		if aw.Spec.Suspend {
			return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperSuspending) // abort deployment
		}
		err, fatal := r.createComponents(ctx, aw)
		if err != nil {
			if fatal {
				return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperFailed) // abort on fatal error
			}
			return ctrl.Result{}, err // retry creation on transient error
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
		if podStatus.succeeded >= podStatus.expected && (podStatus.pending+podStatus.running+podStatus.failed == 0) {
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.QuotaReserved),
				Status:  metav1.ConditionFalse,
				Reason:  string(workloadv1beta2.AppWrapperSucceeded),
				Message: fmt.Sprintf("%v pods succeeded and no running, pending, or failed pods", podStatus.succeeded),
			})
			return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperSucceeded)
		}
		if podStatus.failed > 0 {
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:   string(workloadv1beta2.PodsReady),
				Status: metav1.ConditionFalse,
				Reason: "PodsFailed",
				Message: fmt.Sprintf("%v pods failed (%v pods pending; %v pods running; %v pods succeeded)",
					podStatus.failed, podStatus.pending, podStatus.running, podStatus.succeeded),
			})
			return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperFailed)
		}
		if podStatus.running+podStatus.succeeded >= podStatus.expected {
			meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
				Type:    string(workloadv1beta2.PodsReady),
				Status:  metav1.ConditionTrue,
				Reason:  "SufficientPodsReady",
				Message: fmt.Sprintf("%v pods running; %v pods succeeded", podStatus.running, podStatus.succeeded),
			})
			return ctrl.Result{RequeueAfter: time.Minute}, r.Status().Update(ctx, aw)
		}
		r.propagateAdmission(ctx, aw)
		meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
			Type:   string(workloadv1beta2.PodsReady),
			Status: metav1.ConditionFalse,
			Reason: "InsufficientPodsReady",
			Message: fmt.Sprintf("%v pods pending; %v pods running; %v pods succeeded",
				podStatus.pending, podStatus.running, podStatus.succeeded),
		})
		return ctrl.Result{RequeueAfter: 5 * time.Second}, r.Status().Update(ctx, aw)

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
				Message: "AppWrapper was suspended by Kueue",
			})
		}
		meta.SetStatusCondition(&aw.Status.Conditions, metav1.Condition{
			Type:    string(workloadv1beta2.QuotaReserved),
			Status:  metav1.ConditionFalse,
			Reason:  string(workloadv1beta2.AppWrapperSuspended),
			Message: "AppWrapper was suspended by Kueue",
		})
		return r.updateStatus(ctx, aw, workloadv1beta2.AppWrapperSuspended)

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
	obj.SetLabels(utilmaps.MergeKeepFirst(obj.GetLabels(), map[string]string{AppWrapperLabel: aw.Name, constants.QueueLabel: childJobQueueName}))

	awLabels := map[string]string{AppWrapperLabel: aw.Name}
	for podSetsIdx, podSet := range component.PodSets {
		toInject := component.PodSetInfos[podSetsIdx]

		p, err := getRawTemplate(obj.UnstructuredContent(), podSet.Path)
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

func (r *AppWrapperReconciler) propagateAdmission(ctx context.Context, aw *workloadv1beta2.AppWrapper) {
	for componentIdx, component := range aw.Spec.Components {
		if len(component.PodSets) > 0 {
			obj, err := parseComponent(aw, component.Template.Raw)
			if err != nil {
				return
			}
			wlName := jobframework.GetWorkloadNameForOwnerWithGVK(obj.GetName(), obj.GroupVersionKind())
			wl := &kueue.Workload{}
			if err := r.Client.Get(ctx, client.ObjectKey{Namespace: aw.Namespace, Name: wlName}, wl); err == nil {
				if !workload.IsAdmitted(wl) {
					admission := kueue.Admission{
						ClusterQueue:      childJobQueueName,
						PodSetAssignments: make([]kueue.PodSetAssignment, len(aw.Spec.Components[componentIdx].PodSets)),
					}
					for i := range admission.PodSetAssignments {
						admission.PodSetAssignments[i].Name = wl.Spec.PodSets[i].Name
					}
					newWorkload := wl.DeepCopy()
					workload.SetQuotaReservation(newWorkload, &admission)
					_ = workload.SyncAdmittedCondition(newWorkload)
					if err = workload.ApplyAdmissionStatus(ctx, r.Client, newWorkload, false); err != nil {
						log.FromContext(ctx).Error(err, "syncing admission", "appwrapper", aw, "componentIdx", componentIdx, "workload", wl, "newworkload", newWorkload)
					}
				}
			}
		}
	}
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

func (r *AppWrapperReconciler) workloadStatus(ctx context.Context, aw *workloadv1beta2.AppWrapper) (*podStatusSummary, error) {
	pods := &v1.PodList{}
	if err := r.List(ctx, pods,
		client.InNamespace(aw.Namespace),
		client.MatchingLabels{AppWrapperLabel: aw.Name}); err != nil {
		return nil, err
	}
	summary := &podStatusSummary{expected: ExpectedPodCount(aw)}

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

func replicas(ps workloadv1beta2.AppWrapperPodSet) int32 {
	if ps.Replicas == nil {
		return 1
	} else {
		return *ps.Replicas
	}
}

func ExpectedPodCount(aw *workloadv1beta2.AppWrapper) int32 {
	var expected int32
	for _, c := range aw.Spec.Components {
		for _, s := range c.PodSets {
			expected += replicas(s)
		}
	}
	return expected
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppWrapperReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadv1beta2.AppWrapper{}).
		Watches(&v1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.podMapFunc)).
		Complete(r)
}
