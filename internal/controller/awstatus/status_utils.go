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

package awstatus

import (
	"context"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	"github.com/project-codeflare/appwrapper/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	cachedClient client.Client
)

func CacheClient(k8sclient client.Client) {
	cachedClient = k8sclient
}

const controllerName = "workload.codeflare.dev-appwrapper"

func BaseForStatusPatch(aw *workloadv1beta2.AppWrapper) *workloadv1beta2.AppWrapper {
	patch := &workloadv1beta2.AppWrapper{
		ObjectMeta: metav1.ObjectMeta{
			UID:       aw.UID,
			Name:      aw.Name,
			Namespace: aw.Namespace,
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: workloadv1beta2.GroupVersion.String(),
			Kind:       "AppWrapper",
		},
	}
	return patch
}

func EnsureComponentStatusInitialized(ctx context.Context, aw *workloadv1beta2.AppWrapper) error {
	if len(aw.Status.ComponentStatus) == len(aw.Spec.Components) {
		return nil
	}

	// Construct definitive PodSets from the Spec + InferPodSets and cache in the Status (to avoid clashing with user updates to the Spec via apply)
	compStatus := make([]workloadv1beta2.AppWrapperComponentStatus, len(aw.Spec.Components))
	for idx := range aw.Spec.Components {
		if len(aw.Spec.Components[idx].DeclaredPodSets) > 0 {
			compStatus[idx].PodSets = aw.Spec.Components[idx].DeclaredPodSets
		} else {
			obj := &unstructured.Unstructured{}
			if _, _, err := unstructured.UnstructuredJSONScheme.Decode(aw.Spec.Components[idx].Template.Raw, nil, obj); err != nil {
				// Transient error; Template.Raw was validated by our AdmissionController
				return err
			}
			podSets, err := utils.InferPodSets(obj)
			if err != nil {
				// Transient error; InferPodSets was validated by our AdmissionController
				return err
			}
			compStatus[idx].PodSets = podSets
		}
	}
	aw.Status.ComponentStatus = compStatus

	patch := BaseForStatusPatch(aw)
	patch.Status.ComponentStatus = compStatus
	return cachedClient.Status().Patch(ctx, patch, client.Apply, client.FieldOwner(controllerName), client.ForceOwnership)
}
