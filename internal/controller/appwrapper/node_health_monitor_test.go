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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/project-codeflare/appwrapper/pkg/config"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("NodeMonitor Controller", func() {
	var node1Name = types.NamespacedName{Name: "fake-node-1"}
	var node2Name = types.NamespacedName{Name: "fake-node-2"}
	var nodeMonitor *NodeHealthMonitor
	nodeGPUs := v1.ResourceList{v1.ResourceName("nvidia.com/gpu"): resource.MustParse("4")}

	BeforeEach(func() {
		for _, nodeName := range []string{node1Name.Name, node2Name.Name} {
			node := &v1.Node{
				TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
				ObjectMeta: metav1.ObjectMeta{Name: nodeName, Labels: map[string]string{"key1": "value1"}},
			}
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			node = getNode(nodeName)
			node.Status.Capacity = nodeGPUs
			Expect(k8sClient.Status().Update(ctx, node)).To(Succeed())
		}

		// Create reconciller
		awConfig := config.NewAppWrapperConfig()
		nodeMonitor = &NodeHealthMonitor{
			Client: k8sClient,
			Config: awConfig,
		}
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &v1.Node{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
			ObjectMeta: metav1.ObjectMeta{Name: node1Name.Name},
		})).To(Succeed())
		Expect(k8sClient.Delete(ctx, &v1.Node{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
			ObjectMeta: metav1.ObjectMeta{Name: node2Name.Name},
		})).To(Succeed())
		nodeMonitor = nil
	})

	It("Autopilot Monitoring", func() {
		_, err := nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node2Name})
		Expect(err).NotTo(HaveOccurred())

		By("Healthy cluster has no unhealthy nodes")
		Expect(len(unhealthyNodes)).Should(Equal(0))

		By("A newly labeled node is detected as unhealthy")
		node := getNode(node1Name.Name)
		node.Labels["autopilot.ibm.com/gpuhealth"] = "ERR"
		Expect(k8sClient.Update(ctx, node)).Should(Succeed())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node2Name})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(unhealthyNodes)).Should(Equal(1))
		Expect(unhealthyNodes).Should(HaveKey(node1Name.Name))
		Expect(unhealthyNodes[node1Name.Name]).Should(HaveKey("nvidia.com/gpu"))
		Expect(unhealthyNodes[node1Name.Name]["nvidia.com/gpu"].String()).Should(Equal("4"))

		By("Repeated reconcile does not change map")
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node2Name})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(unhealthyNodes)).Should(Equal(1))
		Expect(unhealthyNodes).Should(HaveKey(node1Name.Name))
		Expect(unhealthyNodes[node1Name.Name]).Should(HaveKey("nvidia.com/gpu"))
		Expect(unhealthyNodes[node1Name.Name]["nvidia.com/gpu"].String()).Should(Equal("4"))

		By("Removing the autopilot label updates unhealthyNodes")
		node.Labels["autopilot.ibm.com/gpuhealth"] = "OK"
		Expect(k8sClient.Update(ctx, node)).Should(Succeed())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(unhealthyNodes)).Should(Equal(0))
	})

	It("ClusterQueue Lending Adjustment", func() {
		// TODO: tardieu
	})
})
