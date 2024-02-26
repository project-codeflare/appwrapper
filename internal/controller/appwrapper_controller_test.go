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
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
)

var _ = Describe("AppWrapper Controller", func() {
	Context("Happy Path Lifecyle", func() {
		var controllerReconciler *AppWrapperReconciler
		var typeNamespacedName types.NamespacedName

		It("Create AppWrapper", func() {
			aw := toAppWrapper(pod(100), pod(100))
			aw.Spec.Suspend = true
			Expect(k8sClient.Create(ctx, aw)).To(Succeed())

			controllerReconciler = &AppWrapperReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			typeNamespacedName = types.NamespacedName{
				Name:      aw.Name,
				Namespace: aw.Namespace,
			}
		})

		It("Empty -> Suspended", func() {
			By("Reconciling the created resource")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			aw := getAppWrapper(typeNamespacedName)
			Expect(aw.Status.Phase).Should(Equal(workloadv1beta2.AppWrapperSuspended))
			Expect(controllerutil.ContainsFinalizer(aw, appWrapperFinalizer)).Should(BeTrue())
		})

		It("Suspended -> Deploying", func() {
			By("Setting suspend to false")
			aw := getAppWrapper(typeNamespacedName)
			aw.Spec.Suspend = false
			Expect(k8sClient.Update(ctx, aw)).To(Succeed())

			By("Reconciling the created resource")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			aw = getAppWrapper(typeNamespacedName)
			Expect(aw.Status.Phase).Should(Equal(workloadv1beta2.AppWrapperResuming))
			Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.ResourcesDeployed))).Should(BeTrue())
			Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(workloadv1beta2.QuotaReserved))).Should(BeTrue())
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
