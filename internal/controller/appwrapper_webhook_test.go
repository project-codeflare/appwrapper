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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("AppWrapper Webhook Tests", func() {

	Context("Defaulting Webhook", func() {
		It("Suspended is set to true", func() {
			aw := wrapSpec("aw", "default", workloadv1beta2.AppWrapperSpec{
				Suspend:    false,
				Components: []workloadv1beta2.AppWrapperComponent{pod(100)},
			})

			Expect(k8sClient.Create(ctx, aw)).To(Succeed())
			Expect(aw.Spec.Suspend).Should(BeTrue(), "aw.Spec.Suspend should have been changed to true")
			Expect(k8sClient.Delete(ctx, aw)).To(Succeed())
		})
	})

	Context("Validating Webhook", func() {
		Context("Structural Invariants", func() {
			It("There must be at least one podspec (a)", func() {
				aw := wrapSpec("aw", "default", workloadv1beta2.AppWrapperSpec{
					Components: []workloadv1beta2.AppWrapperComponent{},
				})
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
			})

			It("There must be at least one podspec (b)", func() {
				aw := wrapSpec("aw", "default", workloadv1beta2.AppWrapperSpec{
					Components: []workloadv1beta2.AppWrapperComponent{service()},
				})
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
			})

			It("There must be no more than 8 podspecs", func() {
				aw := wrapSpec("aw", "default", workloadv1beta2.AppWrapperSpec{
					Components: []workloadv1beta2.AppWrapperComponent{pod(100), pod(100), pod(100), pod(100),
						pod(100), pod(100), pod(100), pod(100), pod(100)},
				})
				Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
			})

		})

		It("Nested AppWrappers are rejected", func() {
			child := wrapSpec("child", "default", workloadv1beta2.AppWrapperSpec{
				Components: []workloadv1beta2.AppWrapperComponent{pod(100)},
			})
			childBytes, err := json.Marshal(child)
			Expect(err).ShouldNot(HaveOccurred())

			aw := wrapSpec("aw", "default", workloadv1beta2.AppWrapperSpec{
				Components: []workloadv1beta2.AppWrapperComponent{pod(100), {
					PodSets:  []workloadv1beta2.AppWrapperPodSet{},
					Template: runtime.RawExtension{Raw: childBytes}},
				},
			})

			Expect(k8sClient.Create(ctx, aw)).ShouldNot(Succeed())
		})

		It("RBAC is enforced", func() {

			// TODO(user): Add your logic here

		})

		It("Well-formed AppWrappers are accepted", func() {
			aw := wrapSpec("aw", "default", workloadv1beta2.AppWrapperSpec{
				Suspend:    false,
				Components: []workloadv1beta2.AppWrapperComponent{pod(100), deployment(4, 100)},
			})

			Expect(k8sClient.Create(ctx, aw)).To(Succeed())
			Expect(aw.Spec.Suspend).Should(BeTrue(), "Legal AppWrappers should be accepted")
			Expect(k8sClient.Delete(ctx, aw)).To(Succeed())

		})
	})

})
