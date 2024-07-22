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
	"reflect"
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/project-codeflare/appwrapper/pkg/config"
)

// NodeHealthMonitor maintains the set of nodes that Autopilot has labelled as unhealthy
type NodeHealthMonitor struct {
	client.Client
	Config *config.AppWrapperConfig
}

var (
	// unhealthyNodes is a mapping from Node names to a set of resource quantities that Autopilot has labeled as unhealthy on that Node
	unhealthyNodes      = make(map[string]map[string]*resource.Quantity)
	unhealthyNodesMutex sync.RWMutex
)

// permission to watch nodes
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *NodeHealthMonitor) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	node := &v1.Node{}
	if err := r.Get(ctx, req.NamespacedName, node); err != nil {
		return ctrl.Result{}, nil
	}

	flaggedResources := make(map[string]*resource.Quantity)
	for key, value := range node.GetLabels() {
		for r, apLabels := range r.Config.Autopilot.ResourceUnhealthyConfig {
			if apValue, ok := apLabels[key]; ok && apValue == value {
				flaggedResources[r] = node.Status.Capacity.Name(v1.ResourceName(r), resource.DecimalSI)
			}
		}
	}

	nodeChanged := false
	unhealthyNodesMutex.Lock() // BEGIN CRITICAL SECTION
	if priorEntry, ok := unhealthyNodes[node.GetName()]; ok {
		if len(flaggedResources) == 0 {
			delete(unhealthyNodes, node.GetName())
			nodeChanged = true
		} else if !reflect.DeepEqual(priorEntry, flaggedResources) {
			unhealthyNodes[node.GetName()] = flaggedResources
			nodeChanged = true
		}
	} else if len(flaggedResources) > 0 {
		unhealthyNodes[node.GetName()] = flaggedResources
		nodeChanged = true
	}
	numUnhealthy := len(unhealthyNodes)
	unhealthyNodesMutex.Unlock() // END CRITICAL SECTION

	if nodeChanged {
		// This unsynchronized read of unhealthyNodes for logging purposes is safe because this method
		// is the only writer to the map and the controller runtime is configured to not allow concurrent execution of this method.
		log.FromContext(ctx).Info("Updated node health information", "Number Unhealthy Nodes", numUnhealthy, "Unhealthy Resource Details", unhealthyNodes)
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
