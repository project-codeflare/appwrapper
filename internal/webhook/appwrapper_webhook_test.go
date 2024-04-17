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

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

var _ = Describe("AppWrapper Webhook Tests", func() {

	Context("Defaulting Webhook", func() {
		It("Suspended is set to true", func() {
			aw := toAppWrapper(pod(100))

			Expect(k8sClient.Create(ctx, aw)).To(Succeed())
			Expect(aw.Spec.Suspend).Should(BeTrue(), "aw.Spec.Suspend should have been changed to true")
			Expect(k8sClient.Delete(ctx, aw)).To(Succeed())
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
				comp.PodSets[0].PodPath = "template.spec.missing"
				aw := toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.PodSets[0].PodPath = ""
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
			})

			It("PodSpec paths must refer to a PodSpecTemplate", func() {
				comp := deployment(4, 100)
				comp.PodSets[0].PodPath = "template.spec.template.metadata"
				aw := toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
			})

			It("Validation of Array and Map path elements", func() {
				comp := jobSet(2, 100)
				comp.PodSets[0].PodPath = "template.spec.replicatedJobs.template.spec.template"
				aw := toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.PodSets[0].PodPath = "template.spec.replicatedJobs"
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.PodSets[0].PodPath = "template.spec.replicatedJobs[0].template[0].spec.template"
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.PodSets[0].PodPath = "template.spec.replicatedJobs[10].template.spec.template"
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.PodSets[0].PodPath = "template.spec.replicatedJobs[-1].template.spec.template"
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.PodSets[0].PodPath = "template.spec.replicatedJobs[a10].template.spec.template"
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.PodSets[0].PodPath = "template.spec.replicatedJobs[1"
				aw = toAppWrapper(comp)
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())

				comp.PodSets[0].PodPath = "template.spec.replicatedJobs[1]].template.spec.template"
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
			aw := toAppWrapper(pod(100), workloadv1beta2.AppWrapperComponent{
				PodSets:  []workloadv1beta2.AppWrapperPodSet{},
				Template: runtime.RawExtension{Raw: childBytes},
			})
			Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
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
				aw.Spec.Components[1].PodSetInfos = make([]workloadv1beta2.AppWrapperPodSetInfo, 1)
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
				aw.Spec.Components[0].PodSets = append(aw.Spec.Components[0].PodSets, aw.Spec.Components[0].PodSets...)
				Expect(k8sClient.Update(ctx, aw)).ShouldNot(Succeed())

				aw = getAppWrapper(awName)
				aw.Spec.Components[0].PodSets[0].PodPath = "bad"
				Expect(k8sClient.Update(ctx, aw)).ShouldNot(Succeed())

				aw = getAppWrapper(awName)
				aw.Spec.Components[0].PodSets[0].Replicas = ptr.To(int32(12))
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
	})

})
