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

package webhook

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	awv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	utilmaps "github.com/project-codeflare/appwrapper/internal/util"
)

var _ = Describe("AppWrapper Webhook Tests", func() {

	Context("Defaulting Webhook", func() {
		It("Suspended is set to true", func() {
			aw := toAppWrapper(pod(100))

			Expect(k8sClient.Create(ctx, aw)).To(Succeed())
			Expect(aw.Spec.Suspend).Should(BeTrue(), "aw.Spec.Suspend should have been changed to true")
			Expect(k8sClient.Delete(ctx, aw)).To(Succeed())
		})

		It("Default queue name is set", func() {
			aw := toAppWrapper(pod(100))

			Expect(k8sClient.Create(ctx, aw)).To(Succeed())
			Expect(aw.Labels[QueueNameLabel]).Should(BeIdenticalTo(defaultQueueName), "aw should be labeled with the default queue name")
			Expect(k8sClient.Delete(ctx, aw)).To(Succeed())
		})

		It("Provided queue name is not overridden by default queue name", func() {
			aw := toAppWrapper(pod(100))
			aw.Labels = utilmaps.MergeKeepFirst(map[string]string{QueueNameLabel: userProvidedQueueName}, aw.Labels)

			Expect(k8sClient.Create(ctx, aw)).To(Succeed())
			Expect(aw.Labels[QueueNameLabel]).Should(BeIdenticalTo(userProvidedQueueName), "queue name should not be overridden")
			Expect(k8sClient.Delete(ctx, aw)).To(Succeed())
		})

		It("User name and ID are set", func() {
			aw := toAppWrapper(pod(100))
			aw.Labels = utilmaps.MergeKeepFirst(map[string]string{AppWrapperUsernameLabel: "bad", AppWrapperUserIDLabel: "bad"}, aw.Labels)

			Expect(k8sLimitedClient.Create(ctx, aw)).To(Succeed())
			Expect(aw.Labels[AppWrapperUsernameLabel]).Should(BeIdenticalTo(limitedUserName))
			Expect(aw.Labels[AppWrapperUserIDLabel]).Should(BeIdenticalTo(limitedUserID))
			Expect(k8sLimitedClient.Delete(ctx, aw)).To(Succeed())
		})
	})

	Context("Validating Webhook", func() {
		Context("Structural Invariants", func() {
			It("There must be at least one podspec (a)", func() {
				aw := toAppWrapper()
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
			})

			It("There must be at least one podspec (b)", func() {
				aw := toAppWrapper(service())
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
			})

			It("There must be no more than 8 podspecs", func() {
				aw := toAppWrapper(pod(100), pod(100), pod(100), pod(100), pod(100), pod(100), pod(100), pod(100), pod(100))
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
			})

			It("Non-existent PodSpec paths are rejected", func() {
				comp := deployment(4, 100)
				comp.DeclaredPodSets[0].Path = "template.spec.missing"
				aw := toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = ""
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
			})

			It("PodSpec paths must refer to a PodSpecTemplate", func() {
				comp := deployment(4, 100)
				comp.DeclaredPodSets[0].Path = "template.spec.template.metadata"
				aw := toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
			})

			It("Validation of Array and Map path elements", func() {
				comp := jobSet(2, 100)
				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs.template.spec.template"
				aw := toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs"
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs[0].template[0].spec.template"
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs[10].template.spec.template"
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs[-1].template.spec.template"
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs[a10].template.spec.template"
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs[1"
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs[1]].template.spec.template"
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
			})
		})

		It("Components in other namespaces are rejected", func() {
			aw := toAppWrapper(namespacedPod("test", 100))
			Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
		})

		It("Nested AppWrappers are rejected", func() {
			child := toAppWrapper(pod(100))
			childBytes, err := json.Marshal(child)
			Expect(err).ShouldNot(HaveOccurred())
			aw := toAppWrapper(pod(100), awv1beta2.AppWrapperComponent{
				DeclaredPodSets: []awv1beta2.AppWrapperPodSet{},
				Template:        runtime.RawExtension{Raw: childBytes},
			})
			Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
		})

		It("User name and ID are immutable", func() {
			aw := toAppWrapper(pod(100))
			awName := types.NamespacedName{Name: aw.Name, Namespace: aw.Namespace}
			Expect(k8sClient.Create(ctx, aw)).Should(Succeed())

			aw = getAppWrapper(awName)
			aw.Labels[AppWrapperUsernameLabel] = "bad"
			Expect(k8sClient.Update(ctx, aw)).ShouldNot(Succeed())

			aw = getAppWrapper(awName)
			aw.Labels[AppWrapperUserIDLabel] = "bad"
			Expect(k8sClient.Update(ctx, aw)).ShouldNot(Succeed())

			Expect(k8sClient.Delete(ctx, aw)).To(Succeed())
		})

		It("User name and ID should be preserved on updates", func() {
			aw := toAppWrapper(pod(100))
			awName := types.NamespacedName{Name: aw.Name, Namespace: aw.Namespace}
			Expect(k8sLimitedClient.Create(ctx, aw)).Should(Succeed())

			aw = getAppWrapper(awName)
			Expect(k8sClient.Update(ctx, aw)).Should(Succeed())

			aw = getAppWrapper(awName)
			Expect(aw.Labels[AppWrapperUsernameLabel]).Should(BeIdenticalTo(limitedUserName))
			Expect(aw.Labels[AppWrapperUserIDLabel]).Should(BeIdenticalTo(limitedUserID))
			Expect(k8sLimitedClient.Delete(ctx, aw)).To(Succeed())
		})

		Context("aw.Spec.Components is immutable", func() {
			It("Updates to non-sensitive fields are allowed", func() {
				aw := toAppWrapper(pod(100), deployment(4, 100))
				awName := types.NamespacedName{Name: aw.Name, Namespace: aw.Namespace}
				Expect(k8sClient.Create(ctx, aw)).Should(Succeed())

				aw = getAppWrapper(awName)
				aw.Spec.Suspend = true
				Expect(k8sClient.Update(ctx, aw)).Should(Succeed())

				aw = getAppWrapper(awName)
				aw.Spec.Components[1].PodSetInfos = make([]awv1beta2.AppWrapperPodSetInfo, 1)
				Expect(k8sClient.Update(ctx, aw)).Should(Succeed())
			})

			It("Updates to sensitive fields are rejected", func() {
				aw := toAppWrapper(pod(100), deployment(4, 100))
				awName := types.NamespacedName{Name: aw.Name, Namespace: aw.Namespace}
				Expect(k8sClient.Create(ctx, aw)).Should(Succeed())

				aw = getAppWrapper(awName)
				aw.Spec.Components[0].Template = aw.Spec.Components[1].Template
				Expect(k8sClient.Update(ctx, aw)).ShouldNot(Succeed())

				aw = getAppWrapper(awName)
				aw.Spec.Components = append(aw.Spec.Components, aw.Spec.Components[0])
				Expect(k8sClient.Update(ctx, aw)).ShouldNot(Succeed())

				aw = getAppWrapper(awName)
				aw.Spec.Components[0].DeclaredPodSets = append(aw.Spec.Components[0].DeclaredPodSets, aw.Spec.Components[0].DeclaredPodSets...)
				Expect(k8sClient.Update(ctx, aw)).ShouldNot(Succeed())

				aw = getAppWrapper(awName)
				aw.Spec.Components[0].DeclaredPodSets[0].Path = "bad"
				Expect(k8sClient.Update(ctx, aw)).ShouldNot(Succeed())

				aw = getAppWrapper(awName)
				aw.Spec.Components[0].DeclaredPodSets[0].Replicas = ptr.To(int32(12))
				Expect(k8sClient.Update(ctx, aw)).ShouldNot(Succeed())
			})
		})

		Context("RBAC is enforced for wrapped resouces", func() {
			It("AppWrapper containing permitted resources can be created", func() {
				aw := toAppWrapper(pod(100))
				Expect(k8sLimitedClient.Create(ctx, aw)).To(Succeed(), "Limited user should be allowed to create AppWrapper containing Pods")
				Expect(k8sLimitedClient.Delete(ctx, aw)).To(Succeed())
			})

			It("AppWrapper containing unpermitted resources cannot be created", func() {
				aw := toAppWrapper(deployment(4, 100))
				Expect(k8sLimitedClient.Create(ctx, aw)).NotTo(Succeed(), "Limited user should not be allowed to create AppWrapper containing Deployments")
			})
		})

		It("Well-formed AppWrappers are accepted", func() {
			aw := toAppWrapper(pod(100), deployment(1, 100), namespacedPod("default", 100), rayCluster(1, 100), jobSet(1, 100))

			Expect(k8sClient.Create(ctx, aw)).To(Succeed(), "Legal AppWrappers should be accepted")
			Expect(aw.Spec.Suspend).Should(BeTrue())
			Expect(k8sClient.Delete(ctx, aw)).To(Succeed())
		})

		Context("PodSets are inferred for known GVKs", func() {
			It("PodSets are inferred for common kinds", func() {
				aw := toAppWrapper(pod(100), deploymentForInference(1, 100), podForInference(100),
					jobForInference(2, 4, 100), jobForInference(8, 4, 100))

				Expect(k8sClient.Create(ctx, aw)).To(Succeed(), "PodSets should be inferred")
				Expect(aw.Spec.Suspend).Should(BeTrue())
				Expect(k8sClient.Delete(ctx, aw)).To(Succeed())
			})

			It("PodSets are inferred for PyTorchJobs, RayClusters, and RayJobs", func() {
				aw := toAppWrapper(pytorchJobForInference(100, 4, 100), rayClusterForInference(7, 100), rayJobForInference(7, 100))

				Expect(k8sClient.Create(ctx, aw)).To(Succeed(), "PodSets should be inferred")
				Expect(aw.Spec.Suspend).Should(BeTrue())
				Expect(k8sClient.Delete(ctx, aw)).To(Succeed())
			})
		})
	})

})
