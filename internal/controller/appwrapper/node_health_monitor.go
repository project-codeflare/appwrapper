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

// NodeHealthMonitor maintains the set of nodes that Autopilot has labelled as unhealthy
type NodeHealthMonitor struct {
	client.Client
	Config *config.AppWrapperConfig
}

var (
	// unhealthyNodes is a mapping from Node names to a set of resources that Autopilot has labeled as unhealthy on that Node
	unhealthyNodes      = make(map[string]sets.Set[string])
	unhealthyNodesMutex sync.RWMutex

	// unschedulableNodes is a mapping from Node names to resource quantities than Autopilot has labeled as unschedulable on that Node
	unschedulableNodes = make(map[string]map[string]*resource.Quantity)
)

// permission to watch nodes
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=kueue.x-k8s.io,resources=clusterqueues,verbs=get;list;watch;update;patch

//gocyclo:ignore
func (r *NodeHealthMonitor) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	node := &v1.Node{}
	if err := r.Get(ctx, req.NamespacedName, node); err != nil {
		return ctrl.Result{}, nil
	}

	flaggedResources := make(sets.Set[string])
	for key, value := range node.GetLabels() {
		for resourceName, taints := range r.Config.Autopilot.ResourceTaints {
			for _, taint := range taints {
				if key == taint.Key && value == taint.Value && taint.Effect == v1.TaintEffectNoExecute {
					flaggedResources.Insert(resourceName)
				}
			}
		}
	}

	nodeChanged := false
	unhealthyNodesMutex.Lock() // BEGIN CRITICAL SECTION
	if priorEntry, ok := unhealthyNodes[node.GetName()]; ok {
		if len(flaggedResources) == 0 {
			delete(unhealthyNodes, node.GetName())
			nodeChanged = true
		} else if !priorEntry.Equal(flaggedResources) {
			unhealthyNodes[node.GetName()] = flaggedResources
			nodeChanged = true
		}
	} else if len(flaggedResources) > 0 {
		unhealthyNodes[node.GetName()] = flaggedResources
		nodeChanged = true
	}
	unhealthyNodesMutex.Unlock() // END CRITICAL SECTION

	// Unsynchronized reads of unhealthyNodes below are safe because this method
	// is the only writer to the map and the controller runtime is configured to
	// not allow concurrent execution of this method.

	if nodeChanged {
		log.FromContext(ctx).Info("Updated node health information", "Number Unhealthy Nodes", len(unhealthyNodes), "Unhealthy Resource Details", unhealthyNodes)
	}

	// update lending limits on slack quota if configured

	if r.Config.SlackQueueName == "" {
		return ctrl.Result{}, nil
	}

	// get slack quota
	cq := &kueue.ClusterQueue{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.Config.SlackQueueName}, cq); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil // give up if slack quota is not defined
		}
		return ctrl.Result{}, err
	}

	// update unschedulable resource quantities for this node
	flaggedQuantities := make(map[string]*resource.Quantity)
	if node.Spec.Unschedulable {
		// flag all configured resources if the node is cordoned
		for resourceName := range r.Config.Autopilot.ResourceTaints {
			flaggedQuantities[resourceName] = node.Status.Capacity.Name(v1.ResourceName(resourceName), resource.DecimalSI)
		}
	} else {
		for key, value := range node.GetLabels() {
			for resourceName, taints := range r.Config.Autopilot.ResourceTaints {
				for _, taint := range taints {
					if key == taint.Key && value == taint.Value {
						flaggedQuantities[resourceName] = node.Status.Capacity.Name(v1.ResourceName(resourceName), resource.DecimalSI)
					}
				}
			}
		}
	}

	if len(flaggedQuantities) > 0 {
		unschedulableNodes[node.GetName()] = flaggedQuantities
	} else {
		delete(unschedulableNodes, node.GetName())
	}

	// compute unschedulable resource totals
	unschedulableQuantities := map[string]*resource.Quantity{}
	for _, quantities := range unschedulableNodes {
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
	var err error
	if limitsChanged {
		err = r.Update(ctx, cq)
		if err == nil {
			log.FromContext(ctx).Info("Updated lending limits", "Resources", resources)
		}
	}

	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeHealthMonitor) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Watches(&v1.Node{}, &handler.EnqueueRequestForObject{}).
		Named("NodeMonitor").
		Complete(r)
}
