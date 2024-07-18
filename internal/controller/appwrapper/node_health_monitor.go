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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"

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

// unhealthyNodes is a mapping from Node names to a set of resources that Autopilot has labeled as unhealthy on that Node
var unhealthyNodes = make(map[string]sets.Set[string])

// permission to watch nodes
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *NodeHealthMonitor) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	node := &metav1.PartialObjectMetadata{}
	node.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Node",
	})
	if err := r.Get(ctx, req.NamespacedName, node); err != nil {
		return ctrl.Result{}, nil
	}

	log.FromContext(ctx).V(2).Info("Reconcilling", "node", req.NamespacedName)

	flaggedResources := make(sets.Set[string])
	for key, value := range node.GetLabels() {
		for resource, apLabels := range r.Config.Autopilot.ResourceUnhealthyConfig {
			if apValue, ok := apLabels[key]; ok && apValue == value {
				flaggedResources.Insert(resource)
			}
		}
	}

	hadEntries := len(flaggedResources) > 0

	if len(flaggedResources) == 0 {
		delete(unhealthyNodes, node.GetName())
	} else {
		unhealthyNodes[node.GetName()] = flaggedResources
	}

	if len(unhealthyNodes) == 0 {
		if hadEntries {
			log.FromContext(ctx).Info("All nodes now healthy")
		} else {
			log.FromContext(ctx).V(2).Info("All nodes now healthy")
		}
	} else {
		log.FromContext(ctx).Info("Some nodes unhealthy", "number", len(unhealthyNodes), "details", unhealthyNodes)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeHealthMonitor) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WatchesMetadata(&v1.Node{}, &handler.EnqueueRequestForObject{}).
		Named("NodeMonitor").
		Complete(r)
}
