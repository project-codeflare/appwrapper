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
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	"sigs.k8s.io/kueue/pkg/controller/jobframework"
	"sigs.k8s.io/kueue/pkg/podset"

	awv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	"github.com/project-codeflare/appwrapper/pkg/utils"
)

// +kubebuilder:rbac:groups=scheduling.k8s.io,resources=priorityclasses,verbs=list;get;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;watch;update;patch
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads/finalizers,verbs=update
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=resourceflavors,verbs=get;list;watch
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloadpriorityclasses,verbs=get;list;watch

type AppWrapper awv1beta2.AppWrapper

var (
	GVK                = awv1beta2.GroupVersion.WithKind("AppWrapper")
	WorkloadReconciler = jobframework.NewGenericReconcilerFactory(
		func() jobframework.GenericJob { return &AppWrapper{} },
		func(b *builder.Builder, c client.Client) *builder.Builder {
			return b.Named("AppWrapperWorkload")
		},
	)
)

func (aw *AppWrapper) Object() client.Object {
	return (*awv1beta2.AppWrapper)(aw)
}

func (aw *AppWrapper) IsSuspended() bool {
	return aw.Spec.Suspend
}

func (aw *AppWrapper) IsActive() bool {
	return meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.QuotaReserved))
}

func (aw *AppWrapper) Suspend() {
	aw.Spec.Suspend = true
}

func (aw *AppWrapper) GVK() schema.GroupVersionKind {
	return GVK
}

func (aw *AppWrapper) PodSets() []kueue.PodSet {
	podSpecTemplates, awPodSets, err := utils.GetComponentPodSpecs((*awv1beta2.AppWrapper)(aw))
	if err != nil {
		// Kueue will raise an error on zero length PodSet; the Kueue GenericJob API prevents propagating the actual error.
		return []kueue.PodSet{}
	}
	podSets := []kueue.PodSet{}
	for psIndex := range podSpecTemplates {
		podSets = append(podSets, kueue.PodSet{
			Name:            fmt.Sprintf("%s-%v", aw.Name, psIndex),
			Template:        *podSpecTemplates[psIndex],
			Count:           utils.Replicas(awPodSets[psIndex]),
			TopologyRequest: jobframework.PodSetTopologyRequest(&(podSpecTemplates[psIndex].ObjectMeta), nil, nil, nil),
		})
	}
	return podSets
}

func (aw *AppWrapper) RunWithPodSetsInfo(podSetsInfo []podset.PodSetInfo) error {
	awPodSetsInfo := make([]awv1beta2.AppWrapperPodSetInfo, len(podSetsInfo))
	for idx := range podSetsInfo {
		awPodSetsInfo[idx].Annotations = podSetsInfo[idx].Annotations
		awPodSetsInfo[idx].Labels = podSetsInfo[idx].Labels
		awPodSetsInfo[idx].NodeSelector = podSetsInfo[idx].NodeSelector
		awPodSetsInfo[idx].Tolerations = podSetsInfo[idx].Tolerations
		awPodSetsInfo[idx].SchedulingGates = podSetsInfo[idx].SchedulingGates
	}

	if err := utils.SetPodSetInfos((*awv1beta2.AppWrapper)(aw), awPodSetsInfo); err != nil {
		return fmt.Errorf("%w: %v", podset.ErrInvalidPodsetInfo, err)
	}
	aw.Spec.Suspend = false
	return nil
}

func (aw *AppWrapper) RestorePodSetsInfo(podSetsInfo []podset.PodSetInfo) bool {
	return utils.ClearPodSetInfos((*awv1beta2.AppWrapper)(aw))
}

func (aw *AppWrapper) Finished() (message string, success, finished bool) {
	switch aw.Status.Phase {
	case awv1beta2.AppWrapperSucceeded:
		return "AppWrapper finished successfully", true, true

	case awv1beta2.AppWrapperFailed:
		if meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.ResourcesDeployed)) {
			return "Still deleting resources for failed AppWrapper", false, false
		} else {
			return "AppWrapper failed", false, true
		}
	}
	return "", false, false
}

func (aw *AppWrapper) PodsReady() bool {
	return meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.PodsReady))
}
