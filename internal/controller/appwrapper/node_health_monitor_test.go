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

package appwrapper

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/project-codeflare/appwrapper/pkg/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NodeMonitor Controller", func() {
	var node1Name = types.NamespacedName{Name: "fake-node-1"}
	var node2Name = types.NamespacedName{Name: "fake-node-2"}
	var nodeMonitor *NodeHealthMonitor
	nodeGPUs := v1.ResourceList{v1.ResourceName("nvidia.com/gpu"): resource.MustParse("4")}

	createNode := func(nodeName string) {
		node := &v1.Node{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
			ObjectMeta: metav1.ObjectMeta{Name: nodeName, Labels: map[string]string{"key1": "value1"}},
		}
		Expect(k8sClient.Create(ctx, node)).To(Succeed())
		node = getNode(nodeName)
		node.Status.Capacity = nodeGPUs
		Expect(k8sClient.Status().Update(ctx, node)).To(Succeed())
	}

	deleteNode := func(nodeName string) {
		Expect(k8sClient.Delete(ctx, &v1.Node{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
			ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		})).To(Succeed())
	}

	BeforeEach(func() {
		// Create reconcillers
		awConfig := config.NewAppWrapperConfig()
		nodeMonitor = &NodeHealthMonitor{
			Client: k8sClient,
			Config: awConfig,
		}
	})

	AfterEach(func() {
		nodeMonitor = nil
	})

	It("Autopilot Monitoring", func() {
		createNode(node1Name.Name)
		createNode(node2Name.Name)

		_, err := nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node2Name})
		Expect(err).NotTo(HaveOccurred())

		By("Healthy cluster has no unhealthy nodes")
		Expect(noExecuteNodes).Should(BeEmpty())

		By("A node labeled EVICT is detected as unhealthy")
		node := getNode(node1Name.Name)
		node.Labels["autopilot.ibm.com/gpuhealth"] = "EVICT"
		Expect(k8sClient.Update(ctx, node)).Should(Succeed())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node2Name})
		Expect(err).NotTo(HaveOccurred())
		Expect(noExecuteNodes).Should(HaveLen(1))
		Expect(noExecuteNodes).Should(HaveKey(node1Name.Name))
		Expect(noExecuteNodes[node1Name.Name]).Should(HaveKey("nvidia.com/gpu"))

		By("Repeated reconcile does not change map")
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node2Name})
		Expect(err).NotTo(HaveOccurred())
		Expect(noExecuteNodes).Should(HaveLen(1))
		Expect(noExecuteNodes).Should(HaveKey(node1Name.Name))
		Expect(noExecuteNodes[node1Name.Name]).Should(HaveKey("nvidia.com/gpu"))

		By("Removing the EVICT label updates unhealthyNodes")
		node.Labels["autopilot.ibm.com/gpuhealth"] = "WARN"
		Expect(k8sClient.Update(ctx, node)).Should(Succeed())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		Expect(noExecuteNodes).Should(BeEmpty())

		deleteNode(node1Name.Name)
		deleteNode(node2Name.Name)
	})
})
