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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/kueue/pkg/podset"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
)

var _ = Describe("AppWrapper Controller", func() {
	Context("Happy Path Lifecyle", func() {
		var awReconciler *AppWrapperReconciler
		var awName types.NamespacedName
		markerPodSet := podset.PodSetInfo{
			Labels:      map[string]string{"testkey1": "value1"},
			Annotations: map[string]string{"test2": "test2"},
		}

		It("Create AppWrapper", func() {
			aw := toAppWrapper(pod(100), pod(100))
			aw.Spec.Suspend = true
			Expect(k8sClient.Create(ctx, aw)).To(Succeed())

			awReconciler = &AppWrapperReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			awName = types.NamespacedName{
				Name:      aw.Name,
				Namespace: aw.Namespace,
			}
		})

		It("Empty -> Suspended", func() {
			By("Reconciling")
			_, err := awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
			Expect(err).NotTo(HaveOccurred())

			aw := getAppWrapper(awName)
			Expect(aw.Status.Phase).Should(Equal(workloadv1beta2.AppWrapperSuspended))
			Expect(controllerutil.ContainsFinalizer(aw, appWrapperFinalizer)).Should(BeTrue())
		})

		It("Suspended -> Resuming", func() {
			By("Updating aw.Spec by invoking RunWithPodSetsInfo")
			aw := getAppWrapper(awName)
			err := (*AppWrapper)(aw).RunWithPodSetsInfo([]podset.PodSetInfo{markerPodSet, markerPodSet})
			Expect(err).NotTo(HaveOccurred())
			log.FromContext(ctx).Info("S-R", "aw after set", aw)
			Expect(aw.Spec.Suspend).To(BeFalse())
			Expect(k8sClient.Update(ctx, aw)).To(Succeed())

			By("Reconciling")
			_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
			Expect(err).NotTo(HaveOccurred())

			aw = getAppWrapper(awName)
			Expect(aw.Status.Phase).Should(Equal(workloadv1beta2.AppWrapperResuming))
			Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed))).Should(BeTrue())
			Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.QuotaReserved))).Should(BeTrue())
		})

		It("Resuming -> Running", func() {
			By("Reconciling")
			_, err := awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for Resources to be Created")
			aw := getAppWrapper(awName)
			Expect(waitAWPodsPending(aw, 30*time.Second)).Should(Succeed())

			By("Reconciling")
			_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
			Expect(err).NotTo(HaveOccurred())

			aw = getAppWrapper(awName)
			Expect(aw.Status.Phase).Should(Equal(workloadv1beta2.AppWrapperRunning))
			Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed))).Should(BeTrue())
			Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.QuotaReserved))).Should(BeTrue())
		})

		It("Running -> Succeeded", func() {
			By("Simulating pod completion")
			aw := getAppWrapper(awName)
			Expect(simulatePodCompletion(aw)).To(Succeed())

			By("Reconciling")
			_, err := awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
			Expect(err).NotTo(HaveOccurred())

			aw = getAppWrapper(awName)
			Expect(aw.Status.Phase).Should(Equal(workloadv1beta2.AppWrapperSucceeded))
			Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed))).Should(BeTrue())
			Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.QuotaReserved))).Should(BeFalse())
		})

		/*
			AfterEach(func() {
				// TODO(user): Cleanup logic after each test, like removing the resource instance.
				resource := &workloadv1beta2.AppWrapper{}
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				Expect(err).NotTo(HaveOccurred())

				By("Cleanup the specific resource instance AppWrapper")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			})
		*/

	})
})
