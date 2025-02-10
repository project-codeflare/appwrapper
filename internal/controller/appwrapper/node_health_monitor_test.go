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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("NodeMonitor Controller", func() {
	var slackQueueName = "fake-queue"
	var dispatch = types.NamespacedName{Name: slackQueueName}
	var node1Name = types.NamespacedName{Name: "fake-node-1"}
	var node2Name = types.NamespacedName{Name: "fake-node-2"}
	var nodeMonitor *NodeHealthMonitor
	var cqMonitor *SlackClusterQueueMonitor
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
		awConfig.SlackQueueName = slackQueueName
		conduit := make(chan event.GenericEvent, 1)
		nodeMonitor = &NodeHealthMonitor{
			Client: k8sClient,
			Config: awConfig,
			Events: conduit,
		}
		cqMonitor = &SlackClusterQueueMonitor{
			Client: k8sClient,
			Config: awConfig,
			Events: conduit,
		}
	})

	AfterEach(func() {
		nodeMonitor = nil
		cqMonitor = nil
	})

	It("Autopilot Monitoring", func() {
		createNode(node1Name.Name)
		createNode(node2Name.Name)

		_, err := nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node2Name})
		Expect(err).NotTo(HaveOccurred())

		By("Healthy cluster has no unhealthy nodes")
		Expect(len(noExecuteNodes)).Should(Equal(0))

		By("A node labeled EVICT is detected as unhealthy")
		node := getNode(node1Name.Name)
		node.Labels["autopilot.ibm.com/gpuhealth"] = "EVICT"
		Expect(k8sClient.Update(ctx, node)).Should(Succeed())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node2Name})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(noExecuteNodes)).Should(Equal(1))
		Expect(noExecuteNodes).Should(HaveKey(node1Name.Name))
		Expect(noExecuteNodes[node1Name.Name]).Should(HaveKey("nvidia.com/gpu"))

		By("Repeated reconcile does not change map")
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node2Name})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(noExecuteNodes)).Should(Equal(1))
		Expect(noExecuteNodes).Should(HaveKey(node1Name.Name))
		Expect(noExecuteNodes[node1Name.Name]).Should(HaveKey("nvidia.com/gpu"))

		By("Removing the EVICT label updates unhealthyNodes")
		node.Labels["autopilot.ibm.com/gpuhealth"] = "WARN"
		Expect(k8sClient.Update(ctx, node)).Should(Succeed())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(noExecuteNodes)).Should(Equal(0))

		deleteNode(node1Name.Name)
		deleteNode(node2Name.Name)
	})

	It("ClusterQueue Lending Adjustment", func() {
		createNode(node1Name.Name)
		createNode(node2Name.Name)

		_, err := nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node2Name})
		Expect(err).NotTo(HaveOccurred())

		// start with 6 gpus
		queue := slackQueue(slackQueueName, resource.MustParse("6"))
		Expect(k8sClient.Create(ctx, queue)).To(Succeed())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: slackQueueName}, queue)).Should(Succeed())
		Expect(queue.Spec.ResourceGroups[0].Flavors[0].Resources[0].LendingLimit).Should(BeNil())

		// remove 4 gpus, lending limit should be 2
		node1 := getNode(node1Name.Name)
		node1.Labels["autopilot.ibm.com/gpuhealth"] = "EVICT"
		Expect(k8sClient.Update(ctx, node1)).Should(Succeed())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = cqMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: dispatch})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: slackQueueName}, queue)).Should(Succeed())
		Expect(queue.Spec.ResourceGroups[0].Flavors[0].Resources[0].LendingLimit.Value()).Should(Equal(int64(2)))

		// remove another 4 gpus, lending limit should be 0 = max(0, 6-4-4)
		node2 := getNode(node2Name.Name)
		node2.Labels["autopilot.ibm.com/gpuhealth"] = "TESTING"
		Expect(k8sClient.Update(ctx, node2)).Should(Succeed())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node2Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = cqMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: dispatch})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: slackQueueName}, queue)).Should(Succeed())
		Expect(queue.Spec.ResourceGroups[0].Flavors[0].Resources[0].LendingLimit).ShouldNot(BeNil())
		Expect(queue.Spec.ResourceGroups[0].Flavors[0].Resources[0].LendingLimit.Value()).Should(Equal(int64(0)))

		// restore 4 gpus, lending limit should be 2
		node1.Labels["autopilot.ibm.com/gpuhealth"] = "OK"
		Expect(k8sClient.Update(ctx, node1)).Should(Succeed())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = cqMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: dispatch})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: slackQueueName}, queue)).Should(Succeed())
		Expect(queue.Spec.ResourceGroups[0].Flavors[0].Resources[0].LendingLimit).ShouldNot(BeNil())
		Expect(queue.Spec.ResourceGroups[0].Flavors[0].Resources[0].LendingLimit.Value()).Should(Equal(int64(2)))

		// restore last 4 gpus, lending limit should be nil
		node2.Labels["autopilot.ibm.com/gpuhealth"] = "OK"
		Expect(k8sClient.Update(ctx, node2)).Should(Succeed())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node2Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = cqMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: dispatch})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: slackQueueName}, queue)).Should(Succeed())
		Expect(queue.Spec.ResourceGroups[0].Flavors[0].Resources[0].LendingLimit).Should(BeNil())

		// cordon node1, lending limit should be 2
		node1 = getNode(node1Name.Name)
		node1.Spec.Unschedulable = true
		Expect(k8sClient.Update(ctx, node1)).Should(Succeed())
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = cqMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: dispatch})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: slackQueueName}, queue)).Should(Succeed())
		Expect(queue.Spec.ResourceGroups[0].Flavors[0].Resources[0].LendingLimit.Value()).Should(Equal(int64(2)))

		// Increase the slack cluster queue's quota by 2 and expect LendngLimit to increase by 2 to become 4
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: slackQueueName}, queue)).Should(Succeed())
		queue.Spec.ResourceGroups[0].Flavors[0].Resources[0].NominalQuota = resource.MustParse("8")
		Expect(k8sClient.Update(ctx, queue)).Should(Succeed())
		_, err = cqMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: slackQueueName}})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: slackQueueName}, queue)).Should(Succeed())
		Expect(queue.Spec.ResourceGroups[0].Flavors[0].Resources[0].LendingLimit.Value()).Should(Equal(int64(4)))

		// Deleting a noncordoned node should not change the lending limit
		deleteNode(node2Name.Name)
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node2Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = cqMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: dispatch})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: slackQueueName}, queue)).Should(Succeed())
		Expect(queue.Spec.ResourceGroups[0].Flavors[0].Resources[0].LendingLimit.Value()).Should(Equal(int64(4)))

		// Delete the cordoned node; lending limit should now by nil
		deleteNode(node1Name.Name)
		_, err = nodeMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: node1Name})
		Expect(err).NotTo(HaveOccurred())
		_, err = cqMonitor.Reconcile(ctx, reconcile.Request{NamespacedName: dispatch})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: slackQueueName}, queue)).Should(Succeed())
		Expect(queue.Spec.ResourceGroups[0].Flavors[0].Resources[0].LendingLimit).Should(BeNil())

		Expect(k8sClient.Delete(ctx, queue)).To(Succeed())
	})
})
