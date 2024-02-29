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

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	awv1b2 "github.com/project-codeflare/appwrapper/api/v1beta2"
)

var _ = Describe("AppWrapper E2E Test", func() {
	var appwrappers []*awv1b2.AppWrapper

	BeforeEach(func() {
		appwrappers = []*awv1b2.AppWrapper{}
	})

	AfterEach(func() {
		By("Cleaning up test objects")
		cleanupTestObjects(ctx, appwrappers)
	})

	Describe("Creation of Fundamental GVKs", func() {
		It("Pods", func() {
			aw := createAppWrapper(ctx, pod(250), pod(250))
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
		})
		It("Deployments", func() {
			aw := createAppWrapper(ctx, deployment(2, 200))
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
		})
		It("StatefulSets", func() {
			aw := createAppWrapper(ctx, statefulset(2, 200))
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
		})
		// TODO: Batch v1.Jobs
		It("Mixed Basic Resources", func() {
			aw := createAppWrapper(ctx, pod(100), deployment(2, 100), statefulset(2, 100), service())
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
		})
	})

	Describe("Error Handling for Invalid Resources", func() {
		// TODO: Replicate scenarios from the AdmissionController unit tests

	})

	Describe("Queueing and Preemption", func() {
		It("Basic Queuing", Label("slow"), func() {
			By("Jobs should be admitted when there is available quota")
			aw := createAppWrapper(ctx, deployment(2, 250))
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
			aw2 := createAppWrapper(ctx, deployment(2, 250))
			appwrappers = append(appwrappers, aw2)
			Expect(waitAWPodsReady(ctx, aw2)).Should(Succeed())

			By("Jobs should be queued when quota remains")
			aw3 := createAppWrapper(ctx, deployment(2, 250))
			appwrappers = append(appwrappers, aw3)
			Consistently(AppWrapperIsQueued(ctx, aw3), "20s").Should(BeTrue())

		})

	})

	Describe("Detection of Completion Status", func() {

	})

	Describe("Load Testing", Label("slow"), func() {

	})

})
