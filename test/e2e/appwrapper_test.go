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
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
)

var _ = Describe("AppWrapper E2E Test", func() {
	var appwrappers []*workloadv1beta2.AppWrapper

	BeforeEach(func() {
		appwrappers = []*workloadv1beta2.AppWrapper{}
	})

	AfterEach(func() {
		By("Cleaning up test objects")
		cleanupTestObjects(ctx, appwrappers)
	})

	Describe("Creation of Fundamental GVKs", Label("Kueue", "Standalone"), func() {
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
		It("Batch Jobs", func() {
			aw := createAppWrapper(ctx, batchjob(250))
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
		})

		It("Mixed Basic Resources", func() {
			aw := createAppWrapper(ctx, pod(100), deployment(2, 100), statefulset(2, 100), service(), batchjob(100))
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
		})
	})

	Describe("Creation of Kubeflow Training Operator GVKs", Label("Kueue", "Standalone"), func() {
		It("PyTorch Jobs", func() {
			aw := createAppWrapper(ctx, pytorchjob(2, 250))
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
		})

		// TODO: Additional Kubeflow Training Operator GVKs of interest
	})

	// TODO: Ray Clusters

	Describe("Error Handling for Invalid Resources", func() {
		// TODO: Replicate scenarios from the webhook unit tests

	})

	Describe("Queueing and Preemption", Label("Kueue"), func() {
		It("Basic Queuing", Label("slow"), func() {
			By("Jobs should be admitted when there is available quota")
			aw := createAppWrapper(ctx, deployment(2, 500))
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
			aw2 := createAppWrapper(ctx, deployment(2, 500))
			appwrappers = append(appwrappers, aw2)
			Expect(waitAWPodsReady(ctx, aw2)).Should(Succeed())

			By("Jobs should be queued when quota is exhausted")
			aw3 := createAppWrapper(ctx, deployment(2, 250))
			appwrappers = append(appwrappers, aw3)
			Eventually(AppWrapperPhase(ctx, aw3), 10*time.Second).Should(Equal(workloadv1beta2.AppWrapperSuspended))
			Consistently(AppWrapperPhase(ctx, aw3), 20*time.Second).Should(Equal(workloadv1beta2.AppWrapperSuspended))

			By("Queued job is admitted when quota becomes available")
			Expect(deleteAppWrapper(ctx, aw.Name, aw.Namespace)).Should(Succeed())
			appwrappers = []*workloadv1beta2.AppWrapper{aw2, aw3}
			Expect(waitAWPodsReady(ctx, aw3)).Should(Succeed())
		})
	})

	// AppWrapper consumes the entire quota itself; tests verify that we don't double count children
	Describe("Recognition of Child Jobs", Label("Kueue"), func() {
		It("Batch Job", func() {
			aw := createAppWrapper(ctx, batchjob(2000))
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
		})

		It("PyTorch Job", func() {
			aw := createAppWrapper(ctx, pytorchjob(2, 1000))
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
		})

		It("Compound Workloads", func() {
			aw := createAppWrapper(ctx, batchjob(500), pytorchjob(2, 500), deployment(2, 250))
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
		})
	})

	Describe("Detection of Completion Status", Label("slow"), Label("Kueue", "Standalone"), func() {
		It("A successful Batch Job yields a successful AppWrapper", func() {
			aw := createAppWrapper(ctx, succeedingBatchjob(500))
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
			Eventually(AppWrapperPhase(ctx, aw), 60*time.Second).Should(Equal(workloadv1beta2.AppWrapperSucceeded))
		})

		It("A failed Batch Job yields a failed AppWrapper", func() {
			aw := createAppWrapper(ctx, failingBatchjob(500))
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
			Eventually(AppWrapperPhase(ctx, aw), 90*time.Second).Should(Equal(workloadv1beta2.AppWrapperFailed))
		})
	})

	Describe("Load Testing", Label("slow"), Label("Kueue", "Standalone"), func() {
		It("Create 50 AppWrappers", func() {
			const (
				awCount   = 50
				cpuDemand = 5
			)

			By("Creating 50 AppWrappers")
			replicas := 2
			for i := 0; i < awCount; i++ {
				aw := createAppWrapper(ctx, deployment(replicas, cpuDemand))
				appwrappers = append(appwrappers, aw)
			}
			nonRunningAWs := appwrappers

			By("Polling for all AppWrappers to be Running")
			err := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 1*time.Minute, false, func(ctx context.Context) (done bool, err error) {
				t := time.Now()
				toCheckAWS := make([]*workloadv1beta2.AppWrapper, 0, len(appwrappers))
				for _, aw := range nonRunningAWs {
					if !checkAppWrapperRunning(ctx, aw) {
						toCheckAWS = append(toCheckAWS, aw)
					}
				}
				nonRunningAWs = toCheckAWS
				if len(toCheckAWS) == 0 {
					fmt.Fprintf(GinkgoWriter, "\tAll AppWrappers Running at time %s\n", t.Format(time.RFC3339))
					return true, nil
				}
				fmt.Fprintf(GinkgoWriter, "\tThere are %d non-Running AppWrappers at time %s\n", len(toCheckAWS), t.Format(time.RFC3339))
				return false, nil
			})
			if err != nil {
				fmt.Fprintf(GinkgoWriter, "Load Testing - Create 50 AppWrappers - There are %d non-Running AppWrappers, err = %v\n", len(nonRunningAWs), err)
				for _, uaw := range nonRunningAWs {
					fmt.Fprintf(GinkgoWriter, "Load Testing - Create 50 AppWrappers - Non-Running AW '%s/%s'\n", uaw.Namespace, uaw.Name)
				}
			}
			Expect(err).Should(Succeed(), "All AppWrappers should have ready Pods")

			By("Polling for all pods to become ready")
			nonReadyAWs := appwrappers
			err = wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 3*time.Minute, false, func(ctx context.Context) (done bool, err error) {
				t := time.Now()
				toCheckAWS := make([]*workloadv1beta2.AppWrapper, 0, len(appwrappers))
				for _, aw := range nonReadyAWs {
					if !checkAllAWPodsReady(ctx, aw) {
						toCheckAWS = append(toCheckAWS, aw)
					}
				}
				nonReadyAWs = toCheckAWS
				if len(toCheckAWS) == 0 {
					fmt.Fprintf(GinkgoWriter, "\tAll pods ready at time %s\n", t.Format(time.RFC3339))
					return true, nil
				}
				fmt.Fprintf(GinkgoWriter, "\tThere are %d app wrappers without ready pods at time %s\n", len(toCheckAWS), t.Format(time.RFC3339))
				return false, nil
			})
			if err != nil {
				fmt.Fprintf(GinkgoWriter, "Load Testing - Create 50 AppWrappers - There are %d app wrappers without ready pods, err = %v\n", len(nonReadyAWs), err)
				for _, uaw := range nonReadyAWs {
					fmt.Fprintf(GinkgoWriter, "Load Testing - Create 50 AppWrappers - Non-Ready AW '%s/%s'\n", uaw.Namespace, uaw.Name)
				}
			}
			Expect(err).Should(Succeed(), "All AppWrappers should have ready Pods")
		})
	})

})
