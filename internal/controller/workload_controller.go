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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	"sigs.k8s.io/kueue/pkg/controller/jobframework"
	"sigs.k8s.io/kueue/pkg/podset"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
)

// +kubebuilder:rbac:groups=scheduling.k8s.io,resources=priorityclasses,verbs=list;get;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;watch;update;patch
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads/finalizers,verbs=update
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=resourceflavors,verbs=get;list;watch
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloadpriorityclasses,verbs=get;list;watch

type AppWrapper workloadv1beta2.AppWrapper

var (
	GVK                = workloadv1beta2.GroupVersion.WithKind("AppWrapper")
	WorkloadReconciler = jobframework.NewGenericReconciler(func() jobframework.GenericJob { return &AppWrapper{} }, nil)
)

func (aw *AppWrapper) Object() client.Object {
	return (*workloadv1beta2.AppWrapper)(aw)
}

func (aw *AppWrapper) IsSuspended() bool {
	return aw.Spec.Suspend
}

func (aw *AppWrapper) IsActive() bool {
	return aw.Status.Phase == workloadv1beta2.AppWrapperDeploying ||
		aw.Status.Phase == workloadv1beta2.AppWrapperRunning ||
		aw.Status.Phase == workloadv1beta2.AppWrapperSuspending ||
		aw.Status.Phase == workloadv1beta2.AppWrapperDeleting
}

func (aw *AppWrapper) Suspend() {
	aw.Spec.Suspend = true
}

func (aw *AppWrapper) GVK() schema.GroupVersionKind {
	return GVK
}

func (aw *AppWrapper) PodSets() []kueue.PodSet {
	podSets := []kueue.PodSet{}
	i := 0
	for _, component := range aw.Spec.Components {
		for _, podSet := range component.PodSets {
			replicas := int32(1)
			if podSet.Replicas != nil {
				replicas = *podSet.Replicas
			}
			template, err := getPodTemplateSpec(component.Template.Raw, podSet.Path)
			if err == nil {
				podSets = append(podSets, kueue.PodSet{
					Name:     aw.Name + "-" + fmt.Sprint(i),
					Template: *template,
					Count:    replicas,
				})
				i++
			}
		}
	}
	return podSets
}

func (aw *AppWrapper) RunWithPodSetsInfo(podSetsInfo []podset.PodSetInfo) error {
	aw.Spec.Suspend = false
	return nil // TODO
}

func (aw *AppWrapper) RestorePodSetsInfo(podSetsInfo []podset.PodSetInfo) bool {
	return false // TODO
}

func (aw *AppWrapper) Finished() (metav1.Condition, bool) {
	condition := metav1.Condition{
		Type:   kueue.WorkloadFinished,
		Status: metav1.ConditionTrue,
		Reason: "AppWrapperFinished",
	}
	var finished bool
	switch aw.Status.Phase {
	case workloadv1beta2.AppWrapperCompleted:
		finished = true
		condition.Message = "AppWrapper finished successfully"
	case workloadv1beta2.AppWrapperFailed:
		finished = true
		condition.Message = "AppWrapper failed"
	}
	return condition, finished
}

func (aw *AppWrapper) PodsReady() bool {
	return true // TODO
}
