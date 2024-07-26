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
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"

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
	})

	Describe("Creation of Kuberay GVKs", Label("Kueue", "Standalone"), func() {
		It("RayClusters", func() {
			aw := createAppWrapper(ctx, raycluster(500, 2, 250))
			appwrappers = append(appwrappers, aw)
			// Non-functonal RayCluster; will never reach Running Phase
			Eventually(AppWrapperPhase(ctx, aw), 15*time.Second).Should(Equal(workloadv1beta2.AppWrapperResuming))
		})

		It("RayJobs", func() {
			aw := createAppWrapper(ctx, rayjob(500, 2, 250))
			appwrappers = append(appwrappers, aw)
			// Non-functonal RayJob; will never reach Running Phase
			Eventually(AppWrapperPhase(ctx, aw), 15*time.Second).Should(Equal(workloadv1beta2.AppWrapperResuming))
		})
	})

	// TODO: JobSets (would have to deploy JobSet controller on e2e test cluster)

	Describe("Webhook Enforces AppWrapper Invariants", Label("Webhook"), func() {
		Context("Structural Invariants", func() {
			It("There must be at least one podspec (a)", func() {
				aw := toAppWrapper()
				Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())
			})

			It("There must be at least one podspec (b)", func() {
				aw := toAppWrapper(service())
				Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())
			})

			It("There must be no more than 8 podspecs", func() {
				aw := toAppWrapper(pod(100), pod(100), pod(100), pod(100), pod(100), pod(100), pod(100), pod(100), pod(100))
				Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())
			})

			It("Non-existent PodSpec paths are rejected", func() {
				comp := deployment(4, 100)
				comp.DeclaredPodSets[0].Path = "template.spec.missing"
				aw := toAppWrapper(comp)
				Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = ""
				aw = toAppWrapper(comp)
				Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())
			})

			It("PodSpec paths must refer to a PodSpecTemplate", func() {
				comp := deployment(4, 100)
				comp.DeclaredPodSets[0].Path = "template.spec.template.metadata"
				aw := toAppWrapper(comp)
				Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())
			})

			It("Validation of Array and Map path elements", func() {
				comp := jobSet(2, 100)
				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs.template.spec.template"
				aw := toAppWrapper(comp)
				Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs"
				aw = toAppWrapper(comp)
				Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs[0].template[0].spec.template"
				aw = toAppWrapper(comp)
				Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs[10].template.spec.template"
				aw = toAppWrapper(comp)
				Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs[-1].template.spec.template"
				aw = toAppWrapper(comp)
				Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs[a10].template.spec.template"
				aw = toAppWrapper(comp)
				Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs[1"
				aw = toAppWrapper(comp)
				Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())

				comp.DeclaredPodSets[0].Path = "template.spec.replicatedJobs[1]].template.spec.template"
				aw = toAppWrapper(comp)
				Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())
			})
		})

		It("Components in other namespaces are rejected", func() {
			aw := toAppWrapper(namespacedPod("test", 100))
			Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())
		})

		It("Nested AppWrappers are rejected", func() {
			child := toAppWrapper(pod(100))
			childBytes, err := json.Marshal(child)
			Expect(err).ShouldNot(HaveOccurred())
			aw := toAppWrapper(pod(100), workloadv1beta2.AppWrapperComponent{
				DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{},
				Template:        runtime.RawExtension{Raw: childBytes},
			})
			Expect(getClient(ctx).Create(ctx, aw)).ShouldNot(Succeed())
		})

		It("Sensitive fields of aw.Spec.Components are immutable", func() {
			aw := createAppWrapper(ctx, pod(1000), deployment(4, 1000))
			appwrappers = append(appwrappers, aw)
			awName := types.NamespacedName{Name: aw.Name, Namespace: aw.Namespace}

			Expect(updateAppWrapper(ctx, awName, func(aw *workloadv1beta2.AppWrapper) {
				aw.Spec.Components[0].Template = aw.Spec.Components[1].Template
			})).ShouldNot(Succeed())

			Expect(updateAppWrapper(ctx, awName, func(aw *workloadv1beta2.AppWrapper) {
				aw.Spec.Components = append(aw.Spec.Components, aw.Spec.Components[0])
			})).ShouldNot(Succeed())

			Expect(updateAppWrapper(ctx, awName, func(aw *workloadv1beta2.AppWrapper) {
				aw.Spec.Components[0].DeclaredPodSets = append(aw.Spec.Components[0].DeclaredPodSets, aw.Spec.Components[0].DeclaredPodSets...)
			})).ShouldNot(Succeed())

			Expect(updateAppWrapper(ctx, awName, func(aw *workloadv1beta2.AppWrapper) {
				aw.Spec.Components[0].DeclaredPodSets[0].Path = "bad"
			})).ShouldNot(Succeed())

			Expect(updateAppWrapper(ctx, awName, func(aw *workloadv1beta2.AppWrapper) {
				aw.Spec.Components[0].DeclaredPodSets[0].Replicas = ptr.To(int32(12))
			})).ShouldNot(Succeed())
		})
	})

	Describe("Webhook Enforces RBAC", Label("Webhook"), func() {
		It("AppWrapper containing permitted resources can be created", func() {
			aw := toAppWrapper(pod(100))
			Expect(getLimitedClient(ctx).Create(ctx, aw)).To(Succeed(), "Limited user should be allowed to create AppWrapper containing Pods")
			Expect(getClient(ctx).Delete(ctx, aw)).To(Succeed())
		})

		It("AppWrapper containing unpermitted resources cannot be created", func() {
			aw := toAppWrapper(deployment(4, 100))
			Expect(getLimitedClient(ctx).Create(ctx, aw)).NotTo(Succeed(), "Limited user should not be allowed to create AppWrapper containing Deployments")
		})
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
			Eventually(AppWrapperPhase(ctx, aw3), 30*time.Second).Should(Equal(workloadv1beta2.AppWrapperSuspended))
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
			aw := toAppWrapper(failingBatchjob(500))
			if aw.Annotations == nil {
				aw.Annotations = make(map[string]string)
			}
			aw.Annotations[workloadv1beta2.FailureGracePeriodDurationAnnotation] = "0s"
			aw.Annotations[workloadv1beta2.RetryLimitAnnotation] = "0"
			Expect(getClient(ctx).Create(ctx, aw)).To(Succeed())
			appwrappers = append(appwrappers, aw)
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
			Eventually(AppWrapperPhase(ctx, aw), 90*time.Second).Should(Equal(workloadv1beta2.AppWrapperFailed))
		})

		It("Deleting a Running Component yields a failed AppWrapper", func() {
			aw := createAppWrapper(ctx, pytorchjob(2, 500))
			appwrappers = append(appwrappers, aw)
			Eventually(AppWrapperPhase(ctx, aw), 60*time.Second).Should(Equal(workloadv1beta2.AppWrapperRunning))
			aw = getAppWrapper(ctx, types.NamespacedName{Name: aw.Name, Namespace: aw.Namespace})
			toDelete := &metav1.PartialObjectMetadata{
				TypeMeta:   metav1.TypeMeta{Kind: aw.Status.ComponentStatus[0].Kind, APIVersion: aw.Status.ComponentStatus[0].APIVersion},
				ObjectMeta: metav1.ObjectMeta{Name: aw.Status.ComponentStatus[0].Name, Namespace: aw.Namespace},
			}
			Expect(getClient(ctx).Delete(ctx, toDelete)).Should(Succeed())
			Eventually(AppWrapperPhase(ctx, aw), 60*time.Second).Should(Equal(workloadv1beta2.AppWrapperFailed))
		})
	})

	Describe("Autopilot Job Migration", Label("slow"), Label("Kueue", "Standalone"), func() {
		It("A running job is migrated away from an unhealthy node", func() {
			aw := createAppWrapper(ctx, autopilotjob(200, 1))
			appwrappers = append(appwrappers, aw)
			awName := types.NamespacedName{Name: aw.Name, Namespace: aw.Namespace}
			By("workload is running")
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
			By("node is labeled by autopilot")
			nodeName, err := getNodeForAppwrapper(ctx, awName)
			Expect(err).ShouldNot(HaveOccurred())
			DeferCleanup(func() {
				err := updateNode(ctx, nodeName, func(n *v1.Node) { delete(n.Labels, "autopilot.ibm.com/gpuhealth") })
				Expect(err).ShouldNot(HaveOccurred())
			})
			err = updateNode(ctx, nodeName, func(n *v1.Node) { n.Labels["autopilot.ibm.com/gpuhealth"] = "EVICT" })
			Expect(err).ShouldNot(HaveOccurred())
			By("workload is reset")
			Eventually(AppWrapperPhase(ctx, aw), 120*time.Second).Should(Equal(workloadv1beta2.AppWrapperResetting))
			By("workload is running again")
			Eventually(AppWrapperPhase(ctx, aw), 120*time.Second).Should(Equal(workloadv1beta2.AppWrapperRunning))
			Expect(waitAWPodsReady(ctx, aw)).Should(Succeed())
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
