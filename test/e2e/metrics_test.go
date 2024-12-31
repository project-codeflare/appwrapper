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
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	managerNamespace             = "appwrapper-system"
	serviceAccountName           = "appwrapper-controller-manager"
	metricsReaderClusterRoleName = "appwrapper-metrics-reader"
	metricsServiceName           = "appwrapper-controller-manager-metrics-service"
)

var _ = Describe("Metrics", Label("Metrics"), func() {
	It("should ensure the metrics endpoint is serving metrics", Label("Metrics"), func() {
		By("Creating a ClusterRoleBinding for the service account to allow access to metrics")
		metricsReaderClusterRoleBinding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "e2e-test-aw-metrics-reader-crb"},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      serviceAccountName,
					Namespace: managerNamespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     metricsReaderClusterRoleName,
			},
		}
		Expect(getClient(ctx).Create(ctx, metricsReaderClusterRoleBinding)).Should(Succeed())
		DeferCleanup(func() {
			By("Deleting the ClusterRoleBinding", func() {
				Expect(getClient(ctx).Delete(ctx, metricsReaderClusterRoleBinding)).To(Succeed())
			})
		})

		By("Creating the curl-metrics pod using a service account that can access the metrics endpoint")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "curl-metrics", Namespace: managerNamespace},
			Spec: corev1.PodSpec{
				ServiceAccountName: serviceAccountName,
				Containers: []corev1.Container{{
					Name:    "curl",
					Image:   "quay.io/curl/curl:8.11.1",
					Command: []string{"sleep", "3600"},
				}},
			},
		}
		Expect(getClient(ctx).Create(ctx, pod)).Should(Succeed())
		DeferCleanup(func() {
			By("Deleting the pod", func() {
				Expect(getClient(ctx).Delete(ctx, pod)).Should(Succeed())
			})
		})

		By("Waiting for the curl-metrics pod to be running.", func() {
			Eventually(func(g Gomega) {
				createdPod := &corev1.Pod{}
				g.Expect(getClient(ctx).Get(ctx, client.ObjectKeyFromObject(pod), createdPod)).To(Succeed())
				g.Expect(createdPod.Status.Phase).To(Equal(corev1.PodRunning))
			}, 60*time.Second).Should(Succeed())
		})

		metrics := []string{
			"controller_runtime_reconcile_total",
			"appwrapper_phase_total",
		}

		By("Getting the metrics by checking curl-metrics logs", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "exec", "-n", managerNamespace, "curl-metrics", "--", "/bin/sh", "-c",
					fmt.Sprintf(
						"curl -v -k -H \"Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)\" https://%s.%s.svc.cluster.local:8443/metrics ",
						metricsServiceName, managerNamespace,
					),
				)
				metricsOutput, err := cmd.CombinedOutput()
				_, _ = fmt.Fprintf(GinkgoWriter, "err: %v\n", err)
				g.Expect(err).NotTo(HaveOccurred())
				_, _ = fmt.Fprintf(GinkgoWriter, "output is: %v\n", string(metricsOutput))
				for _, metric := range metrics {
					g.Expect(string(metricsOutput)).To(ContainSubstring(metric))
				}
			}, 3600*time.Second).Should(Succeed())
		})
	})
})
