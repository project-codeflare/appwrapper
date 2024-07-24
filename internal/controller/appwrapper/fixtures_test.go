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
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	"sigs.k8s.io/yaml"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
)

const charset = "abcdefghijklmnopqrstuvwxyz0123456789"

func randName(baseName string) string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, 6)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return fmt.Sprintf("%s-%s", baseName, string(b))
}

func toAppWrapper(components ...workloadv1beta2.AppWrapperComponent) *workloadv1beta2.AppWrapper {
	return &workloadv1beta2.AppWrapper{
		TypeMeta:   metav1.TypeMeta{APIVersion: workloadv1beta2.GroupVersion.String(), Kind: "AppWrapper"},
		ObjectMeta: metav1.ObjectMeta{Name: randName("aw"), Namespace: "default"},
		Spec:       workloadv1beta2.AppWrapperSpec{Components: components},
	}
}

func getAppWrapper(typeNamespacedName types.NamespacedName) *workloadv1beta2.AppWrapper {
	aw := &workloadv1beta2.AppWrapper{}
	err := k8sClient.Get(ctx, typeNamespacedName, aw)
	Expect(err).NotTo(HaveOccurred())
	return aw
}

func getNode(name string) *v1.Node {
	node := &v1.Node{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, node)
	Expect(err).NotTo(HaveOccurred())
	return node
}

// envTest doesn't have a Pod controller; so simulate it
func setPodStatus(aw *workloadv1beta2.AppWrapper, phase v1.PodPhase, numToChange int32) error {
	podList := &v1.PodList{}
	err := k8sClient.List(ctx, podList, &client.ListOptions{Namespace: aw.Namespace})
	if err != nil {
		return err
	}
	for _, pod := range podList.Items {
		if numToChange <= 0 {
			return nil
		}
		if awn, found := pod.Labels[AppWrapperLabel]; found && awn == aw.Name {
			pod.Status.Phase = phase
			err = k8sClient.Status().Update(ctx, &pod)
			if err != nil {
				return err
			}
			numToChange -= 1
		}
	}
	return nil
}

const podYAML = `
apiVersion: v1
kind: Pod
metadata:
  name: %v
spec:
  restartPolicy: Never
  containers:
  - name: busybox
    image: quay.io/project-codeflare/busybox:1.36
    command: ["sh", "-c", "sleep 10"]
    resources:
      requests:
        cpu: %v`

func pod(milliCPU int64) workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(podYAML,
		randName("pod"),
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return workloadv1beta2.AppWrapperComponent{
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{{Replicas: ptr.To(int32(1)), Path: "template"}},
		Template:        runtime.RawExtension{Raw: jsonBytes},
	}
}

const malformedPodYAML = `
apiVersion: v1
kind: Pod
metadata:
  name: %v
spec:
  restartPolicy: Never
  containers:
  - name: busybox
    command: ["sh", "-c", "sleep 10"]
    resources:
      requests:
        cpu: %v`

func malformedPod(milliCPU int64) workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(malformedPodYAML,
		randName("pod"),
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return workloadv1beta2.AppWrapperComponent{
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{{Replicas: ptr.To(int32(1)), Path: "template"}},
		Template:        runtime.RawExtension{Raw: jsonBytes},
	}
}

func slackQueue(queueName string, nominalQuota resource.Quantity) *kueue.ClusterQueue {
	return &kueue.ClusterQueue{
		TypeMeta:   metav1.TypeMeta{APIVersion: kueue.GroupVersion.String(), Kind: "ClusterQueue"},
		ObjectMeta: metav1.ObjectMeta{Name: queueName},
		Spec: kueue.ClusterQueueSpec{
			ResourceGroups: []kueue.ResourceGroup{{
				CoveredResources: []v1.ResourceName{v1.ResourceName("nvidia.com/gpu")},
				Flavors: []kueue.FlavorQuotas{{
					Name:      "default-flavor",
					Resources: []kueue.ResourceQuota{{Name: v1.ResourceName("nvidia.com/gpu"), NominalQuota: nominalQuota}}}}}}},
	}
}
