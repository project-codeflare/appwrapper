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
	"time"

	// . "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	kc "sigs.k8s.io/kueue/pkg/controller/constants"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	"github.com/project-codeflare/appwrapper/internal/controller"
)

const (
	testNamespace  = "e2e-test"
	testFlavorName = "e2e-test-flavor"
	testQueueName  = "e2e-test-queue"
)

type myKey struct {
	key string
}

func getClient(ctx context.Context) client.Client {
	kubeClient := ctx.Value(myKey{key: "kubeclient"})
	return kubeClient.(client.Client)
}

func extendContextWithClient(ctx context.Context) context.Context {
	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(workloadv1beta2.AddToScheme(scheme)).To(Succeed())
	Expect(kueue.AddToScheme(scheme)).To(Succeed())
	kubeclient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	Expect(err).To(Succeed())
	return context.WithValue(ctx, myKey{key: "kubeclient"}, kubeclient)
}

func ensureNamespaceExists(ctx context.Context) {
	err := getClient(ctx).Create(ctx, &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	})
	Expect(client.IgnoreAlreadyExists(err)).To(Succeed())
}

func ensureTestQueuesExist(ctx context.Context) {
	rf := &kueue.ResourceFlavor{ObjectMeta: metav1.ObjectMeta{Name: testFlavorName}}
	err := getClient(ctx).Create(ctx, rf)
	Expect(client.IgnoreAlreadyExists(err)).To(Succeed())
	cq := &kueue.ClusterQueue{
		ObjectMeta: metav1.ObjectMeta{Name: testQueueName},
		Spec: kueue.ClusterQueueSpec{
			NamespaceSelector: &metav1.LabelSelector{},
			ResourceGroups: []kueue.ResourceGroup{{
				CoveredResources: []v1.ResourceName{v1.ResourceCPU},
				Flavors: []kueue.FlavorQuotas{{
					Name:      testFlavorName,
					Resources: []kueue.ResourceQuota{{Name: v1.ResourceCPU, NominalQuota: *resource.NewMilliQuantity(2000, resource.DecimalSI)}},
				}},
			},
			},
		},
	}
	err = getClient(ctx).Create(ctx, cq)
	Expect(client.IgnoreAlreadyExists(err)).To(Succeed())

	lq := &kueue.LocalQueue{
		ObjectMeta: metav1.ObjectMeta{Name: testQueueName, Namespace: testNamespace},
		Spec:       kueue.LocalQueueSpec{ClusterQueue: kueue.ClusterQueueReference(testQueueName)},
	}
	err = getClient(ctx).Create(ctx, lq)
	Expect(client.IgnoreAlreadyExists(err)).To(Succeed())
}

func cleanupTestObjects(ctx context.Context, appwrappers []*workloadv1beta2.AppWrapper) {
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
	aw := &workloadv1beta2.AppWrapper{ObjectMeta: metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}}
	return getClient(ctx).Delete(ctx, aw, &client.DeleteOptions{PropagationPolicy: &foreground})
}

func createAppWrapper(ctx context.Context, components ...workloadv1beta2.AppWrapperComponent) *workloadv1beta2.AppWrapper {
	aw := toAppWrapper(components...)
	Expect(getClient(ctx).Create(ctx, aw)).To(Succeed())
	return aw
}

func toAppWrapper(components ...workloadv1beta2.AppWrapperComponent) *workloadv1beta2.AppWrapper {
	return &workloadv1beta2.AppWrapper{
		ObjectMeta: metav1.ObjectMeta{
			Name:        randName("aw"),
			Namespace:   testNamespace,
			Annotations: map[string]string{kc.QueueLabel: testQueueName},
		},
		Spec: workloadv1beta2.AppWrapperSpec{Components: components},
	}
}

func getAppWrapper(ctx context.Context, typeNamespacedName types.NamespacedName) *workloadv1beta2.AppWrapper {
	aw := &workloadv1beta2.AppWrapper{}
	err := getClient(ctx).Get(ctx, typeNamespacedName, aw)
	Expect(err).NotTo(HaveOccurred())
	return aw
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
			if awn, found := pod.Labels[controller.AppWrapperLabel]; found && awn == awName {
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
			if awn, found := podFromPodList.Labels[controller.AppWrapperLabel]; found && awn == awName {
				return false, nil
			}
		}
		return true, nil
	}
}

func waitAWPodsDeleted(ctx context.Context, awNamespace string, awName string) error {
	return wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 90*time.Second, true, noPodsExist(awNamespace, awName))
}

func waitAWPodsReady(ctx context.Context, aw *workloadv1beta2.AppWrapper) error {
	numExpected := controller.ExpectedPodCount(aw)
	phases := []v1.PodPhase{v1.PodRunning, v1.PodSucceeded}
	return wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 90*time.Second, true, podsInPhase(aw.Namespace, aw.Name, phases, numExpected))
}
