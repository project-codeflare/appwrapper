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

	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	awv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	"github.com/project-codeflare/appwrapper/pkg/utils"

	// . "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	testNamespace = "e2e-test"
	testQueueName = "e2e-test-queue"
)

type myKey struct {
	key string
}

func getClient(ctx context.Context) client.Client {
	kubeClient := ctx.Value(myKey{key: "kubeclient"})
	return kubeClient.(client.Client)
}

func getLimitedClient(ctx context.Context) client.Client {
	kubeClient := ctx.Value(myKey{key: "kubelimitedclient"})
	return kubeClient.(client.Client)
}

func extendContextWithClient(ctx context.Context) context.Context {
	baseConfig := ctrl.GetConfigOrDie()
	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(awv1beta2.AddToScheme(scheme)).To(Succeed())

	// Create a client with full permissions
	k8sClient, err := client.New(baseConfig, client.Options{Scheme: scheme})
	Expect(err).To(Succeed())
	return context.WithValue(ctx, myKey{key: "kubeclient"}, k8sClient)
}

func ensureNamespaceExists(ctx context.Context) {
	err := getClient(ctx).Create(ctx, &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	})
	Expect(client.IgnoreAlreadyExists(err)).To(Succeed())
}

func extendContextWithLimitedClient(ctx context.Context) context.Context {
	limitedUser := "e2e-limited-user"

	// Ensure limited RBACs exist
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e-test:limited"},
		Rules: []rbacv1.PolicyRule{
			{Verbs: []string{"*"}, APIGroups: []string{"workload.codeflare.dev"}, Resources: []string{"appwrappers"}},
			{Verbs: []string{"*"}, APIGroups: []string{""}, Resources: []string{"pods"}},
			{Verbs: []string{"get"}, APIGroups: []string{"apps"}, Resources: []string{"deployments"}},
		},
	}
	Expect(client.IgnoreAlreadyExists(getClient(ctx).Create(ctx, clusterRole))).To(Succeed())
	roleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e-test:limited"},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, APIGroup: v1.SchemeGroupVersion.Group, Name: limitedUser}},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.SchemeGroupVersion.Group, Kind: "ClusterRole", Name: clusterRole.Name},
	}
	Expect(client.IgnoreAlreadyExists(getClient(ctx).Create(ctx, roleBinding))).To(Succeed())

	// Create a client that impersonates as limitedUser
	baseConfig := ctrl.GetConfigOrDie()
	limitedCfg := *baseConfig
	limitedCfg.Impersonate = rest.ImpersonationConfig{UserName: limitedUser}
	limitedClient, err := client.New(&limitedCfg, client.Options{Scheme: getClient(ctx).Scheme()})
	Expect(err).NotTo(HaveOccurred())
	return context.WithValue(ctx, myKey{key: "kubelimitedclient"}, limitedClient)
}

const flavorYAML = `
apiVersion: kueue.x-k8s.io/v1beta1
kind: ResourceFlavor
metadata:
  name: e2e-test-flavor
`
const clusterQueueYAML = `
apiVersion: kueue.x-k8s.io/v1beta1
kind: ClusterQueue
metadata:
  name: ` + testQueueName + `
spec:
  namespaceSelector: {}
  resourceGroups:
  - coveredResources: [cpu, nvidia.com/gpu]
    flavors:
    - name: e2e-test-flavor
      resources:
      - name: cpu
        nominalQuota: 2000m
      - name: nvidia.com/gpu
        nominalQuota: 2
`
const localQueueYAML = `
apiVersion: kueue.x-k8s.io/v1beta1
kind: LocalQueue
metadata:
  namespace: ` + testNamespace + `
  name: ` + testQueueName + `
spec:
  clusterQueue: ` + testQueueName

func createFromYaml(ctx context.Context, yamlString string) {
	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	obj := &unstructured.Unstructured{}
	_, _, err = unstructured.UnstructuredJSONScheme.Decode(jsonBytes, nil, obj)
	Expect(err).NotTo(HaveOccurred())
	err = getClient(ctx).Create(ctx, obj)
	Expect(client.IgnoreAlreadyExists(err)).To(Succeed())
}

func ensureTestQueuesExist(ctx context.Context) {
	createFromYaml(ctx, flavorYAML)
	createFromYaml(ctx, clusterQueueYAML)
	createFromYaml(ctx, localQueueYAML)
}

func cleanupTestObjects(ctx context.Context, appwrappers []*awv1beta2.AppWrapper) {
	if appwrappers == nil {
		return
	}
	for _, aw := range appwrappers {
		awNamespace := aw.Namespace
		awName := aw.Name

		err := deleteAppWrapper(ctx, aw.Name, aw.Namespace)
		Expect(err).To(Succeed())
		err = waitAWPodsDeleted(ctx, awNamespace, awName)
		Expect(err).To(Succeed())
	}
}

func deleteAppWrapper(ctx context.Context, name string, namespace string) error {
	foreground := metav1.DeletePropagationForeground
	aw := &awv1beta2.AppWrapper{ObjectMeta: metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}}
	return getClient(ctx).Delete(ctx, aw, &client.DeleteOptions{PropagationPolicy: &foreground})
}

func createAppWrapper(ctx context.Context, components ...awv1beta2.AppWrapperComponent) *awv1beta2.AppWrapper {
	aw := toAppWrapper(components...)
	Expect(getClient(ctx).Create(ctx, aw)).To(Succeed())
	return aw
}

func toAppWrapper(components ...awv1beta2.AppWrapperComponent) *awv1beta2.AppWrapper {
	return &awv1beta2.AppWrapper{
		TypeMeta: metav1.TypeMeta{APIVersion: awv1beta2.GroupVersion.String(), Kind: awv1beta2.AppWrapperKind},
		ObjectMeta: metav1.ObjectMeta{
			Name:      randName("aw"),
			Namespace: testNamespace,
			Labels:    map[string]string{"kueue.x-k8s.io/queue-name": testQueueName},
		},
		Spec: awv1beta2.AppWrapperSpec{Components: components},
	}
}

func updateAppWrapper(ctx context.Context, awName types.NamespacedName, update func(*awv1beta2.AppWrapper)) error {
	for {
		aw := getAppWrapper(ctx, awName)
		update(aw)
		err := getClient(ctx).Update(ctx, aw)
		if err == nil {
			return nil
		}
		if !apierrors.IsConflict(err) {
			return err
		}
	}
}

func getAppWrapper(ctx context.Context, awName types.NamespacedName) *awv1beta2.AppWrapper {
	aw := &awv1beta2.AppWrapper{}
	err := getClient(ctx).Get(ctx, awName, aw)
	Expect(err).NotTo(HaveOccurred())
	return aw
}

func getNodeForAppwrapper(ctx context.Context, awName types.NamespacedName) (string, error) {
	podList := &v1.PodList{}
	err := getClient(ctx).List(ctx, podList, &client.ListOptions{Namespace: awName.Namespace})
	if err != nil {
		return "", err
	}
	for _, pod := range podList.Items {
		if awn, found := pod.Labels[awv1beta2.AppWrapperLabel]; found && awn == awName.Name {
			return pod.Spec.NodeName, nil
		}
	}
	return "", fmt.Errorf("No pods found for %v", awName)
}

func updateNode(ctx context.Context, nodeName string, update func(*v1.Node)) error {
	for {
		node := &v1.Node{}
		err := getClient(ctx).Get(ctx, types.NamespacedName{Name: nodeName}, node)
		Expect(err).NotTo(HaveOccurred())
		update(node)
		err = getClient(ctx).Update(ctx, node)
		if err == nil {
			return nil
		}
		if !apierrors.IsConflict(err) {
			return err
		}
	}
}

func podsInPhase(awNamespace string, awName string, phase []v1.PodPhase, minimumPodCount int32) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		podList := &v1.PodList{}
		err := getClient(ctx).List(ctx, podList, &client.ListOptions{Namespace: awNamespace})
		if err != nil {
			return false, err
		}

		matchingPodCount := int32(0)
		for _, pod := range podList.Items {
			if awn, found := pod.Labels[awv1beta2.AppWrapperLabel]; found && awn == awName {
				for _, p := range phase {
					if pod.Status.Phase == p {
						matchingPodCount++
						break
					}
				}
			}
		}

		return minimumPodCount <= matchingPodCount, nil
	}
}

func noPodsExist(awNamespace string, awName string) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		podList := &v1.PodList{}
		err := getClient(ctx).List(context.Background(), podList, &client.ListOptions{Namespace: awNamespace})
		if err != nil {
			return false, err
		}

		for _, podFromPodList := range podList.Items {
			if awn, found := podFromPodList.Labels[awv1beta2.AppWrapperLabel]; found && awn == awName {
				return false, nil
			}
		}
		return true, nil
	}
}

func waitAWPodsDeleted(ctx context.Context, awNamespace string, awName string) error {
	return wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 90*time.Second, true, noPodsExist(awNamespace, awName))
}

func waitAWPodsReady(ctx context.Context, aw *awv1beta2.AppWrapper) error {
	numExpected, err := utils.ExpectedPodCount(aw)
	if err != nil {
		return err
	}
	phases := []v1.PodPhase{v1.PodRunning, v1.PodSucceeded}
	return wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 120*time.Second, true, podsInPhase(aw.Namespace, aw.Name, phases, numExpected))
}

func checkAllAWPodsReady(ctx context.Context, aw *awv1beta2.AppWrapper) bool {
	numExpected, err := utils.ExpectedPodCount(aw)
	if err != nil {
		return false
	}
	phases := []v1.PodPhase{v1.PodRunning, v1.PodSucceeded}
	err = wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 100*time.Millisecond, true, podsInPhase(aw.Namespace, aw.Name, phases, numExpected))
	return err == nil
}

func checkAppWrapperRunning(ctx context.Context, aw *awv1beta2.AppWrapper) bool {
	aw2 := &awv1beta2.AppWrapper{}
	err := getClient(ctx).Get(ctx, client.ObjectKey{Namespace: aw.Namespace, Name: aw.Name}, aw2)
	Expect(err).NotTo(HaveOccurred())
	return aw2.Status.Phase == awv1beta2.AppWrapperRunning
}

func AppWrapperPhase(ctx context.Context, aw *awv1beta2.AppWrapper) func(g Gomega) awv1beta2.AppWrapperPhase {
	name := aw.Name
	namespace := aw.Namespace
	return func(g Gomega) awv1beta2.AppWrapperPhase {
		aw := &awv1beta2.AppWrapper{}
		err := getClient(ctx).Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, aw)
		g.Expect(err).NotTo(HaveOccurred())
		return aw.Status.Phase
	}
}
