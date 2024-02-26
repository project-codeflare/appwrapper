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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/kueue/pkg/podset"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
)

var _ = Describe("AppWrapper Controller", func() {
	var awReconciler *AppWrapperReconciler
	var awName types.NamespacedName
	markerPodSet := podset.PodSetInfo{
		Labels:      map[string]string{"testkey1": "value1"},
		Annotations: map[string]string{"test2": "test2"},
	}

	BeforeEach(func() {
		By("Create an AppWrapper containing two Pods")
		aw := toAppWrapper(pod(100), pod(100))
		aw.Spec.Suspend = true
		Expect(k8sClient.Create(ctx, aw)).To(Succeed())
		awName = types.NamespacedName{
			Name:      aw.Name,
			Namespace: aw.Namespace,
		}
		awReconciler = &AppWrapperReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	AfterEach(func() {
		By("Cleanup the specific resource instance AppWrapper")
		aw := &workloadv1beta2.AppWrapper{}
		Expect(k8sClient.Get(ctx, awName, aw)).To(Succeed())
		Expect(k8sClient.Delete(ctx, aw)).To(Succeed())
	})

	It("Happy Path Lifecycle", func() {
		By("Reconciling: Empty -> Suspended")
		_, err := awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw := getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(workloadv1beta2.AppWrapperSuspended))
		Expect(controllerutil.ContainsFinalizer(aw, appWrapperFinalizer)).Should(BeTrue())

		By("Updating aw.Spec by invoking RunWithPodSetsInfo")
		Expect((*AppWrapper)(aw).RunWithPodSetsInfo([]podset.PodSetInfo{markerPodSet, markerPodSet})).To(Succeed())
		Expect(aw.Spec.Suspend).To(BeFalse())
		Expect(k8sClient.Update(ctx, aw)).To(Succeed())

		By("Reconciling: Suspended -> Resuming")
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(workloadv1beta2.AppWrapperResuming))
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.QuotaReserved))).Should(BeTrue())

		By("Reconciling: Resuming -> Running")
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(workloadv1beta2.AppWrapperRunning))
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.QuotaReserved))).Should(BeTrue())
		podStatus, err := awReconciler.workloadStatus(ctx, aw)
		Expect(err).NotTo(HaveOccurred())
		Expect(podStatus.pending).Should(Equal(expectedPodCount(aw)))

		By("Simulating all Pods Running")
		Expect(setPodStatus(aw, v1.PodRunning, expectedPodCount(aw))).To(Succeed())
		By("Reconciling: Running -> Running")
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(workloadv1beta2.AppWrapperRunning))
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.QuotaReserved))).Should(BeTrue())
		podStatus, err = awReconciler.workloadStatus(ctx, aw)
		Expect(err).NotTo(HaveOccurred())
		Expect(podStatus.running).Should(Equal(expectedPodCount(aw)))

		By("Simulating one Pod Completing")
		Expect(setPodStatus(aw, v1.PodSucceeded, 1)).To(Succeed())
		By("Reconciling: Running -> Running")
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(workloadv1beta2.AppWrapperRunning))
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.QuotaReserved))).Should(BeTrue())
		podStatus, err = awReconciler.workloadStatus(ctx, aw)
		Expect(err).NotTo(HaveOccurred())
		Expect(podStatus.running).Should(Equal(expectedPodCount(aw) - 1))
		Expect(podStatus.succeeded).Should(Equal(int32(1)))

		By("Simulating all Pods Completing")
		Expect(setPodStatus(aw, v1.PodSucceeded, expectedPodCount(aw))).To(Succeed())
		By("Reconciling: Running -> Succeeded")
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(workloadv1beta2.AppWrapperSucceeded))
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.QuotaReserved))).Should(BeFalse())
	})

})
