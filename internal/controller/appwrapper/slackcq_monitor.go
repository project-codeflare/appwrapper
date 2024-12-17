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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	"github.com/project-codeflare/appwrapper/pkg/config"
)

// SlackClusterQueueMonitor uses the information gathered by the NodeHealthMonitor to
// adjust the lending limitLimits of a designated slack ClusterQueue
type SlackClusterQueueMonitor struct {
	client.Client
	Config *config.AppWrapperConfig
	Events chan event.GenericEvent // event channel for NodeHealthMonitor to trigger SlackClusterQueueMonitor
}

// permission to watch, get and update clusterqueues
//+kubebuilder:rbac:groups=kueue.x-k8s.io,resources=clusterqueues,verbs=get;list;watch;update;patch

func (r *SlackClusterQueueMonitor) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if req.Name != r.Config.SlackQueueName {
		return ctrl.Result{}, nil
	}

	cq := &kueue.ClusterQueue{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.Config.SlackQueueName}, cq); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil // give up if slack cluster queue is not defined
		}
		return ctrl.Result{}, err
	}

	// Compute the total quantities of unschedulable resources
	unschedulableQuantities := map[v1.ResourceName]*resource.Quantity{}
	noScheduleNodesMutex.RLock() // BEGIN CRITICAL SECTION
	for _, quantities := range noScheduleNodes {
		for resourceName, quantity := range quantities {
			if !quantity.IsZero() {
				if unschedulableQuantities[resourceName] == nil {
					unschedulableQuantities[resourceName] = ptr.To(quantity)
				} else {
					unschedulableQuantities[resourceName].Add(quantity)
				}
			}
		}
	}
	noScheduleNodesMutex.RUnlock() // END CRITICAL SECTION

	// enforce lending limits on 1st flavor of 1st resource group
	resources := cq.Spec.ResourceGroups[0].Flavors[0].Resources
	delta := map[v1.ResourceName]*resource.Quantity{}
	for i, quota := range resources {
		var lendingLimit *resource.Quantity
		if unschedulableQuantity := unschedulableQuantities[quota.Name]; unschedulableQuantity != nil {
			if quota.NominalQuota.Cmp(*unschedulableQuantity) > 0 {
				lendingLimit = ptr.To(quota.NominalQuota)
				lendingLimit.Sub(*unschedulableQuantity)
			} else {
				lendingLimit = resource.NewQuantity(0, resource.DecimalSI)
			}
		}
		if quota.LendingLimit == nil && lendingLimit != nil {
			delta[quota.Name] = ptr.To(quota.NominalQuota)
			delta[quota.Name].Sub(*lendingLimit)
			delta[quota.Name].Neg()
			resources[i].LendingLimit = lendingLimit
		} else if quota.LendingLimit != nil && lendingLimit == nil {
			delta[quota.Name] = ptr.To(quota.NominalQuota)
			delta[quota.Name].Sub(*quota.LendingLimit)
			resources[i].LendingLimit = lendingLimit
		} else if quota.LendingLimit != nil && lendingLimit != nil && quota.LendingLimit.Cmp(*lendingLimit) != 0 {
			delta[quota.Name] = ptr.To(*quota.LendingLimit)
			delta[quota.Name].Sub(*lendingLimit)
			delta[quota.Name].Neg()
			resources[i].LendingLimit = lendingLimit
		}
	}

	// update lending limits
	if len(delta) > 0 {
		err := r.Update(ctx, cq)
		if err == nil {
			log.FromContext(ctx).Info("Updated lending limits", "Changed by", delta, "Updated Resources", resources)
			return ctrl.Result{}, nil
		} else if errors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		} else {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SlackClusterQueueMonitor) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Watches(&kueue.ClusterQueue{}, &handler.EnqueueRequestForObject{}).
		WatchesRawSource(source.Channel(r.Events, &handler.EnqueueRequestForObject{})).
		Named("SlackClusterQueueMonitor").
		Complete(r)
}
