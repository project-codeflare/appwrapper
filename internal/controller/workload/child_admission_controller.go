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

package workload

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	"sigs.k8s.io/kueue/pkg/controller/jobframework"
	"sigs.k8s.io/kueue/pkg/workload"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
)

const (
	childJobQueueName = "workload.codeflare.dev.admitted"
)

// ChildWorkloadReconciler reconciles the admission status of an AppWrapper's child workloads
type ChildWorkloadReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// permission to get and read appwrappers
//+kubebuilder:rbac:groups=workload.codeflare.dev,resources=appwrappers,verbs=get;list;watch
//+kubebuilder:rbac:groups=workload.codeflare.dev,resources=appwrappers/status,verbs=get

// permission to manipulate workloads controlling appwrapper components to enable admitting them to our pseudo-clusterqueue
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads,verbs=get
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads/status,verbs=get;update;patch

// Reconcile propagates the Admission of an AppWrapper to its children's Workload objects
func (r *ChildWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	aw := &workloadv1beta2.AppWrapper{}
	if err := r.Get(ctx, req.NamespacedName, aw); err != nil {
		return ctrl.Result{}, nil
	}

	if !aw.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// One reason for being Running but not PodsReady is that a child was suspended on creation by Kueue. Rectify that.
	if aw.Status.Phase == workloadv1beta2.AppWrapperRunning &&
		!meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.PodsReady)) &&
		!meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.Unhealthy)) {
		admittedChildren := 0
		childrenWithPods := 0
		for componentIdx, component := range aw.Spec.Components {
			if len(component.PodSets) > 0 {
				childrenWithPods += 1
				unstruct := &unstructured.Unstructured{}
				if _, gvk, err := unstructured.UnstructuredJSONScheme.Decode(component.Template.Raw, nil, unstruct); err == nil {
					wlName := jobframework.GetWorkloadNameForOwnerWithGVK(unstruct.GetName(), *gvk)
					wl := &kueue.Workload{}
					if err := r.Client.Get(ctx, client.ObjectKey{Namespace: aw.Namespace, Name: wlName}, wl); err == nil {
						if workload.IsAdmitted(wl) {
							admittedChildren += 1
						} else {
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
		if admittedChildren == childrenWithPods {
			return ctrl.Result{}, nil
		} else {
			return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChildWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadv1beta2.AppWrapper{}).
		Complete(r)
}
