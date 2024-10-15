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
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	"github.com/project-codeflare/appwrapper/pkg/config"
)

// NodeHealthMonitor watches Nodes and maintains mappings of Nodes that have either
// been marked as Unschedulable or that have been labeled to indicate that
// they have resources that Autopilot has tainted as NoSchedule or NoExeucte.
// This information is used to automate the maintenance of the lendingLimit of
// a designated slack ClusterQueue and to migrate running workloads away from NoExecute resources.
type NodeHealthMonitor struct {
	client.Client
	Config *config.AppWrapperConfig
}

var (
	// noExecuteNodes is a mapping from Node names to resources with an Autopilot NoExeucte taint
	noExecuteNodes      = make(map[string]sets.Set[string])
	noExecuteNodesMutex sync.RWMutex

	// noScheduleNodes is a mapping from Node names to resource quantities that are unschedulable.
	// A resource may be unscheduable either because:
	//  (a) the Node is cordoned (node.Spec.Unschedulable is true) or
	//  (b) Autopilot has labeled the with either a NoExecute or NoSchedule taint.
	noScheduleNodes = make(map[string]map[string]*resource.Quantity)
)

// permission to watch nodes
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=kueue.x-k8s.io,resources=clusterqueues,verbs=get;list;watch;update;patch

func (r *NodeHealthMonitor) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	node := &v1.Node{}
	if err := r.Get(ctx, req.NamespacedName, node); err != nil {
		return ctrl.Result{}, nil
	}

	r.updateNoExecuteNodes(ctx, node)

	// If there is a slack ClusterQueue, update its lending limits

	if r.Config.SlackQueueName == "" {
		return ctrl.Result{}, nil
	}

	cq := &kueue.ClusterQueue{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.Config.SlackQueueName}, cq); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil // give up if slack quota is not defined
		}
		return ctrl.Result{}, err
	}

	r.updateNoScheduleNodes(ctx, cq, node)

	return r.updateLendingLimits(ctx, cq)
}

func (r *NodeHealthMonitor) updateNoExecuteNodes(ctx context.Context, node *v1.Node) {
	noExecuteResources := make(sets.Set[string])
	for key, value := range node.GetLabels() {
		for resourceName, taints := range r.Config.Autopilot.ResourceTaints {
			for _, taint := range taints {
				if key == taint.Key && value == taint.Value && taint.Effect == v1.TaintEffectNoExecute {
					noExecuteResources.Insert(resourceName)
				}
			}
		}
	}

	noExecuteNodesChanged := false
	noExecuteNodesMutex.Lock() // BEGIN CRITICAL SECTION
	if priorEntry, ok := noExecuteNodes[node.GetName()]; ok {
		if len(noExecuteResources) == 0 {
			delete(noExecuteNodes, node.GetName())
			noExecuteNodesChanged = true
		} else if !priorEntry.Equal(noExecuteResources) {
			noExecuteNodes[node.GetName()] = noExecuteResources
			noExecuteNodesChanged = true
		}
	} else if len(noExecuteResources) > 0 {
		noExecuteNodes[node.GetName()] = noExecuteResources
		noExecuteNodesChanged = true
	}
	noExecuteNodesMutex.Unlock() // END CRITICAL SECTION

	// Safe to log outside the mutex because because this method is the only writer of noExecuteNodes
	// and the controller runtime is configured to not allow concurrent execution of this controller.
	if noExecuteNodesChanged {
		log.FromContext(ctx).Info("Updated node NoExecute information", "Number NoExecute Nodes", len(noExecuteNodes), "NoExecute Resource Details", noExecuteNodes)
	}
}

func (r *NodeHealthMonitor) updateNoScheduleNodes(_ context.Context, cq *kueue.ClusterQueue, node *v1.Node) {
	// update unschedulable resource quantities for this node
	noScheduleQuantities := make(map[string]*resource.Quantity)
	if node.Spec.Unschedulable {
		// add all non-pod resources covered by cq if the node is cordoned
		for _, resourceName := range cq.Spec.ResourceGroups[0].Flavors[0].Resources {
			if string(resourceName.Name) != "pods" {
				noScheduleQuantities[string(resourceName.Name)] = node.Status.Capacity.Name(resourceName.Name, resource.DecimalSI)
			}
		}
	} else {
		for key, value := range node.GetLabels() {
			for resourceName, taints := range r.Config.Autopilot.ResourceTaints {
				for _, taint := range taints {
					if key == taint.Key && value == taint.Value {
						noScheduleQuantities[resourceName] = node.Status.Capacity.Name(v1.ResourceName(resourceName), resource.DecimalSI)
					}
				}
			}
		}
	}

	if len(noScheduleQuantities) > 0 {
		noScheduleNodes[node.GetName()] = noScheduleQuantities
	} else {
		delete(noScheduleNodes, node.GetName())
	}
}

func (r *NodeHealthMonitor) updateLendingLimits(ctx context.Context, cq *kueue.ClusterQueue) (ctrl.Result, error) {

	// compute unschedulable resource totals
	unschedulableQuantities := map[string]*resource.Quantity{}
	for _, quantities := range noScheduleNodes {
		for resourceName, quantity := range quantities {
			if !quantity.IsZero() {
				if unschedulableQuantities[resourceName] == nil {
					unschedulableQuantities[resourceName] = ptr.To(*quantity)
				} else {
					unschedulableQuantities[resourceName].Add(*quantity)
				}
			}
		}
	}

	// enforce lending limits on 1st flavor of 1st resource group
	resources := cq.Spec.ResourceGroups[0].Flavors[0].Resources
	limitsChanged := false
	for i, quota := range resources {
		var lendingLimit *resource.Quantity
		if unschedulableQuantity := unschedulableQuantities[quota.Name.String()]; unschedulableQuantity != nil {
			if quota.NominalQuota.Cmp(*unschedulableQuantity) > 0 {
				lendingLimit = ptr.To(quota.NominalQuota)
				lendingLimit.Sub(*unschedulableQuantity)
			} else {
				lendingLimit = resource.NewQuantity(0, resource.DecimalSI)
			}
		}
		if quota.LendingLimit == nil && lendingLimit != nil ||
			quota.LendingLimit != nil && lendingLimit == nil ||
			quota.LendingLimit != nil && lendingLimit != nil && quota.LendingLimit.Cmp(*lendingLimit) != 0 {
			limitsChanged = true
			resources[i].LendingLimit = lendingLimit
		}
	}

	// update lending limits
	if limitsChanged {
		err := r.Update(ctx, cq)
		if err == nil {
			log.FromContext(ctx).Info("Updated lending limits", "Resources", resources)
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
func (r *NodeHealthMonitor) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Watches(&v1.Node{}, &handler.EnqueueRequestForObject{}).
		Named("NodeMonitor").
		Complete(r)
}
