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
	"maps"
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/project-codeflare/appwrapper/pkg/config"
)

// NodeHealthMonitor watches Nodes and maintains mappings of Nodes that have either
// been marked as Unschedulable or that have been labeled to indicate that
// they have resources that Autopilot has tainted as NoSchedule or NoExecute.
// This information is used to automate the maintenance of the lendingLimit of
// a designated slack ClusterQueue and to migrate running workloads away from NoExecute resources.
type NodeHealthMonitor struct {
	client.Client
	Config *config.AppWrapperConfig
}

var (
	// noExecuteNodes is a mapping from Node names to resources with an Autopilot NoExecute taint
	noExecuteNodes = make(map[string]sets.Set[string])
	// noExecuteNodesMutex synchronizes access to noExecuteNodes
	noExecuteNodesMutex sync.RWMutex

	// noScheduleNodes is a mapping from Node names to ResourceLists of unschedulable resources.
	// A resource may be unschedulable either because:
	//  (a) the Node is cordoned (node.Spec.Unschedulable is true) or
	//  (b) Autopilot has labeled the Node with a NoExecute or NoSchedule taint for the resource.
	noScheduleNodes = make(map[string]v1.ResourceList)
	// noScheduleNodesMutex synchronizes access to noScheduleNodes
	noScheduleNodesMutex sync.RWMutex
)

// permission to watch nodes
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *NodeHealthMonitor) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	node := &v1.Node{}
	if err := r.Get(ctx, req.NamespacedName, node); err != nil {
		if errors.IsNotFound(err) {
			r.updateForNodeDeletion(ctx, req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if node.DeletionTimestamp.IsZero() {
		r.updateNoExecuteNodes(ctx, node)
		r.updateNoScheduleNodes(ctx, node)
	} else {
		r.updateForNodeDeletion(ctx, req.Name)
	}

	return ctrl.Result{}, nil
}

// update noExecuteNodes and noScheduleNodes for the deletion of nodeName
func (r *NodeHealthMonitor) updateForNodeDeletion(ctx context.Context, nodeName string) {
	if _, ok := noExecuteNodes[nodeName]; ok {
		noExecuteNodesMutex.Lock() // BEGIN CRITICAL SECTION
		delete(noExecuteNodes, nodeName)
		noExecuteNodesMutex.Unlock() // END CRITICAL SECTION
		log.FromContext(ctx).Info("Updated NoExecute information due to Node deletion",
			"Number NoExecute Nodes", len(noExecuteNodes), "NoExecute Resource Details", noExecuteNodes)
	}
	if _, ok := noScheduleNodes[nodeName]; ok {
		noScheduleNodesMutex.Lock() // BEGIN CRITICAL SECTION
		delete(noScheduleNodes, nodeName)
		noScheduleNodesMutex.Unlock() // END CRITICAL SECTION
		log.FromContext(ctx).Info("Updated NoSchedule information due to Node deletion",
			"Number NoSchedule Nodes", len(noScheduleNodes), "NoSchedule Resource Details", noScheduleNodes)
	}
}

// update noExecuteNodes entry for node
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

	if noExecuteNodesChanged {
		log.FromContext(ctx).Info("Updated NoExecute information", "Number NoExecute Nodes", len(noExecuteNodes), "NoExecute Resource Details", noExecuteNodes)
	}
}

// update noScheduleNodes entry for node
func (r *NodeHealthMonitor) updateNoScheduleNodes(ctx context.Context, node *v1.Node) {
	var noScheduleResources v1.ResourceList
	if node.Spec.Unschedulable {
		noScheduleResources = node.Status.Capacity.DeepCopy()
		delete(noScheduleResources, v1.ResourcePods)
	} else {
		noScheduleResources = make(v1.ResourceList)
		for key, value := range node.GetLabels() {
			for resourceName, taints := range r.Config.Autopilot.ResourceTaints {
				for _, taint := range taints {
					if taint.Effect == v1.TaintEffectNoExecute || taint.Effect == v1.TaintEffectNoSchedule {
						if key == taint.Key && value == taint.Value {
							quantity := node.Status.Capacity.Name(v1.ResourceName(resourceName), resource.DecimalSI)
							if !quantity.IsZero() {
								noScheduleResources[v1.ResourceName(resourceName)] = *quantity
							}
						}
					}
				}
			}
		}
	}

	noScheduleNodesChanged := false
	noScheduleNodesMutex.Lock() // BEGIN CRITICAL SECTION
	if priorEntry, ok := noScheduleNodes[node.GetName()]; ok {
		if len(noScheduleResources) == 0 {
			delete(noScheduleNodes, node.GetName())
			noScheduleNodesChanged = true
		} else if !maps.Equal(priorEntry, noScheduleResources) {
			noScheduleNodes[node.GetName()] = noScheduleResources
			noScheduleNodesChanged = true
		}
	} else if len(noScheduleResources) > 0 {
		noScheduleNodes[node.GetName()] = noScheduleResources
		noScheduleNodesChanged = true
	}
	noScheduleNodesMutex.Unlock() // END CRITICAL SECTION

	if noScheduleNodesChanged {
		log.FromContext(ctx).Info("Updated NoSchedule information", "Number NoSchedule Nodes", len(noScheduleNodes), "NoSchedule Resource Details", noScheduleNodes)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeHealthMonitor) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Watches(&v1.Node{}, &handler.EnqueueRequestForObject{}).
		Named("NodeMonitor").
		Complete(r)
}
