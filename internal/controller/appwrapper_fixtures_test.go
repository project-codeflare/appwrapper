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
	"context"
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/gomega"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
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

func podsInPhase(awNamespace string, awName string, phase []v1.PodPhase, minimumPodCount int32) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		podList := &v1.PodList{}
		err := k8sClient.List(ctx, podList, &client.ListOptions{Namespace: awNamespace})
		if err != nil {
			return false, err
		}
		matchingPodCount := int32(0)
		for _, pod := range podList.Items {
			if awn, found := pod.Labels[appWrapperLabel]; found && awn == awName {
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

// envTest doesn't have a Pod controller; so simulate it
func simulatePodCompletion(aw *workloadv1beta2.AppWrapper) error {
	podList := &v1.PodList{}
	err := k8sClient.List(ctx, podList, &client.ListOptions{Namespace: aw.Namespace})
	if err != nil {
		return err
	}
	for _, pod := range podList.Items {
		if awn, found := pod.Labels[appWrapperLabel]; found && awn == aw.Name {
			pod.Status.Phase = v1.PodSucceeded
			err = k8sClient.Status().Update(ctx, &pod)
			if err != nil {
				return err
			}
			log.FromContext(ctx).Info("spc", "pod", pod)
		}
	}
	return nil
}

func waitAWPodsReady(aw *workloadv1beta2.AppWrapper, timeout time.Duration) error {
	phases := []v1.PodPhase{v1.PodRunning, v1.PodSucceeded}
	numExpected := expectedPodCount(aw)
	return wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, timeout, true, podsInPhase(aw.Namespace, aw.Name, phases, numExpected))
}

func waitAWPodsPending(aw *workloadv1beta2.AppWrapper, timeout time.Duration) error {
	phases := []v1.PodPhase{v1.PodPending}
	numExpected := expectedPodCount(aw)
	return wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, timeout, true, podsInPhase(aw.Namespace, aw.Name, phases, numExpected))
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
    command: ["sh", "-c", "sleep 1"]
    resources:
      requests:
        cpu: %v`

func pod(milliCPU int64) workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(podYAML,
		randName("pod"),
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	replicas := int32(1)
	return workloadv1beta2.AppWrapperComponent{
		PodSets:  []workloadv1beta2.AppWrapperPodSet{{Replicas: &replicas, Path: "template"}},
		Template: runtime.RawExtension{Raw: jsonBytes},
	}
}

const namespacedPodYAML = `
apiVersion: v1
kind: Pod
metadata:
  name: %v
  namespace: %v
spec:
  restartPolicy: Never
  containers:
  - name: busybox
    image: quay.io/project-codeflare/busybox:1.36
    command: ["sh", "-c", "sleep 1000"]
    resources:
      requests:
        cpu: %v`

func namespacedPod(namespace string, milliCPU int64) workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(namespacedPodYAML,
		randName("pod"),
		namespace,
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	replicas := int32(1)
	return workloadv1beta2.AppWrapperComponent{
		PodSets:  []workloadv1beta2.AppWrapperPodSet{{Replicas: &replicas, Path: "template"}},
		Template: runtime.RawExtension{Raw: jsonBytes},
	}
}

const serviceYAML = `
apiVersion: v1
kind: Service
metadata:
  name: %v
spec:
  selector:
    app: test
  ports:
  - protocol: TCP
    port: 80
    targetPort: 8080`

func service() workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(serviceYAML, randName("service"))
	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return workloadv1beta2.AppWrapperComponent{
		PodSets:  []workloadv1beta2.AppWrapperPodSet{},
		Template: runtime.RawExtension{Raw: jsonBytes},
	}
}

const deploymentYAML = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %v
  labels:
    app: test
spec:
  replicas: %v
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      terminationGracePeriodSeconds: 0
      containers:
      - name: busybox
        image: quay.io/project-codeflare/busybox:1.36
        command: ["sh", "-c", "sleep 10000"]
        resources:
          requests:
            cpu: %v`

func deployment(replicaCount int, milliCPU int64) workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(deploymentYAML,
		randName("deployment"),
		replicaCount,
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	replicas := int32(replicaCount)
	return workloadv1beta2.AppWrapperComponent{
		PodSets:  []workloadv1beta2.AppWrapperPodSet{{Replicas: &replicas, Path: "template.spec.template"}},
		Template: runtime.RawExtension{Raw: jsonBytes},
	}
}
