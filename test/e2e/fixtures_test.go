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
	"math/rand"
	"time"

	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
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

const podYAML = `
apiVersion: v1
kind: Pod
metadata:
  name: %v
spec:
  restartPolicy: Never
  terminationGracePeriodSeconds: 0
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
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{{Path: "template"}},
		Template:        runtime.RawExtension{Raw: jsonBytes},
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
    command: ["sh", "-c", "sleep 10"]
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
	return workloadv1beta2.AppWrapperComponent{
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{{Path: "template"}},
		Template:        runtime.RawExtension{Raw: jsonBytes},
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
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{},
		Template:        runtime.RawExtension{Raw: jsonBytes},
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
	return workloadv1beta2.AppWrapperComponent{
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{{Replicas: ptr.To(int32(replicaCount)), Path: "template.spec.template"}},
		Template:        runtime.RawExtension{Raw: jsonBytes},
	}
}

const statefulesetYAML = `
apiVersion: apps/v1
kind: StatefulSet
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

func statefulset(replicaCount int, milliCPU int64) workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(statefulesetYAML,
		randName("statefulset"),
		replicaCount,
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return workloadv1beta2.AppWrapperComponent{
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{{Replicas: ptr.To(int32(replicaCount)), Path: "template.spec.template"}},
		Template:        runtime.RawExtension{Raw: jsonBytes},
	}
}

const batchJobYAML = `
apiVersion: batch/v1
kind: Job
metadata:
  generateName: %v
spec:
  template:
    spec:
      restartPolicy: Never
      terminationGracePeriodSeconds: 0
      containers:
      - name: busybox
        image: quay.io/project-codeflare/busybox:1.36
        command: ["sh", "-c", "sleep 600"]
        resources:
          requests:
            cpu: %v
`

func batchjob(milliCPU int64) workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(batchJobYAML,
		"batchjob-",
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return workloadv1beta2.AppWrapperComponent{
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{{Path: "template.spec.template"}},
		Template:        runtime.RawExtension{Raw: jsonBytes},
	}
}

const failingBatchJobYAML = `
apiVersion: batch/v1
kind: Job
metadata:
  name: %v
spec:
  template:
    spec:
      restartPolicy: Never
      terminationGracePeriodSeconds: 0
      containers:
      - name: busybox
        image: quay.io/project-codeflare/busybox:1.36
        command: ["sh", "-c", "sleep 10; exit 1"]
        resources:
          requests:
            cpu: %v
`

func failingBatchjob(milliCPU int64) workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(failingBatchJobYAML,
		randName("batchjob"),
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return workloadv1beta2.AppWrapperComponent{
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{{Path: "template.spec.template"}},
		Template:        runtime.RawExtension{Raw: jsonBytes},
	}
}

const succeedingBatchJobYAML = `
apiVersion: batch/v1
kind: Job
metadata:
  generateName: %v
spec:
  template:
    spec:
      restartPolicy: Never
      terminationGracePeriodSeconds: 0
      containers:
      - name: busybox
        image: quay.io/project-codeflare/busybox:1.36
        command: ["sh", "-c", "sleep 10; exit 0"]
        resources:
          requests:
            cpu: %v
`

func succeedingBatchjob(milliCPU int64) workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(succeedingBatchJobYAML,
		"batchjob-",
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return workloadv1beta2.AppWrapperComponent{
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{{Path: "template.spec.template"}},
		Template:        runtime.RawExtension{Raw: jsonBytes},
	}
}

// This is not a useful PyTorchJob:
// 1. Using a dummy busybox image to avoid pulling a large & rate-limited image from dockerhub
// 2. We avoid needing the injected sidecar (alpine:3.10 from dockerhub) by not specifying a Master
const pytorchYAML = `
apiVersion: "kubeflow.org/v1"
kind: PyTorchJob
metadata:
  name: %v
spec:
  pytorchReplicaSpecs:
    Worker:
      replicas: %v
      restartPolicy: OnFailure
      template:
        spec:
          terminationGracePeriodSeconds: 0
          containers:
          - name: pytorch
            image: quay.io/project-codeflare/busybox:1.36
            command: ["sh", "-c", "sleep 10"]
            resources:
              requests:
                cpu: %v
`

func pytorchjob(replicasWorker int, milliCPUWorker int64) workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(pytorchYAML,
		randName("pytorchjob"),
		replicasWorker,
		resource.NewMilliQuantity(milliCPUWorker, resource.DecimalSI),
	)
	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return workloadv1beta2.AppWrapperComponent{
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{
			{Replicas: ptr.To(int32(replicasWorker)), Path: "template.spec.pytorchReplicaSpecs.Worker.template"},
		},
		Template: runtime.RawExtension{Raw: jsonBytes},
	}
}

// This is not a functional RayCluster:
//  1. Using a dummy busybox image to avoid pulling a large & rate-limited image from dockerhub,
//     which means the command injected by the kuberay operator will never work.
//
// It is only useful to check that we validate the PodSpecTemplates and can reach the Resuming state.
const rayclusterYAML = `
apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: %v
spec:
  rayVersion: '2.9.0'
  headGroupSpec:
    rayStartParams: {}
    template:
      spec:
        containers:
          - name: ray-head
            image: quay.io/project-codeflare/busybox:1.36
            command: ["sh", "-c", "sleep 10"]
            resources:
              requests:
                cpu: %v

  workerGroupSpecs:
  - replicas: %v
    minReplicas: %v
    maxReplicas: %v
    groupName: small-group
    rayStartParams: {}
    # Pod template
    template:
      spec:
        containers:
          - name: ray-worker
            image: quay.io/project-codeflare/busybox:1.36
            command: ["sh", "-c", "sleep 10"]
            resources:
              requests:
                cpu: %v
`

func raycluster(milliCPUHead int64, replicasWorker int, milliCPUWorker int64) workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(rayclusterYAML,
		randName("raycluster"),
		resource.NewMilliQuantity(milliCPUHead, resource.DecimalSI),
		replicasWorker, replicasWorker, replicasWorker,
		resource.NewMilliQuantity(milliCPUWorker, resource.DecimalSI),
	)
	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return workloadv1beta2.AppWrapperComponent{
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{
			{Replicas: ptr.To(int32(1)), Path: "template.spec.headGroupSpec.template"},
			{Replicas: ptr.To(int32(replicasWorker)), Path: "template.spec.workerGroupSpecs[0].template"},
		},
		Template: runtime.RawExtension{Raw: jsonBytes},
	}
}

// This is not a functional RayJob:
//  1. Using a dummy busybox image to avoid pulling a large & rate-limited image from dockerhub,
//     which means the command injected by the kuberay operator will never work.
//
// It is only useful to check that we validate the PodSpecTemplates and can reach the Resuming state.
const rayjobYAML = `
apiVersion: ray.io/v1
kind: RayJob
metadata:
  name: %v
spec:
  shutdownAfterJobFinishes: true
  rayClusterSpec:
      rayVersion: '2.9.0'
      headGroupSpec:
        rayStartParams: {}
        template:
          spec:
            containers:
              - name: ray-head
                image: quay.io/project-codeflare/busybox:1.36
                command: ["sh", "-c", "sleep 10"]
                resources:
                  requests:
                    cpu: %v

      workerGroupSpecs:
      - replicas: %v
        minReplicas: %v
        maxReplicas: %v
        groupName: small-group
        rayStartParams: {}
        # Pod template
        template:
          spec:
            containers:
              - name: ray-worker
                image: quay.io/project-codeflare/busybox:1.36
                command: ["sh", "-c", "sleep 10"]
                resources:
                  requests:
                    cpu: %v
`

func rayjob(milliCPUHead int64, replicasWorker int, milliCPUWorker int64) workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(rayjobYAML,
		randName("raycluster"),
		resource.NewMilliQuantity(milliCPUHead, resource.DecimalSI),
		replicasWorker, replicasWorker, replicasWorker,
		resource.NewMilliQuantity(milliCPUWorker, resource.DecimalSI),
	)
	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return workloadv1beta2.AppWrapperComponent{
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{
			{Replicas: ptr.To(int32(1)), Path: "template.spec.rayClusterSpec.headGroupSpec.template"},
			{Replicas: ptr.To(int32(replicasWorker)), Path: "template.spec.rayClusterSpec.workerGroupSpecs[0].template"},
		},
		Template: runtime.RawExtension{Raw: jsonBytes},
	}
}

const jobSetYAML = `
apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: %v
spec:
  replicatedJobs:
  - name: driver
    template:
      spec:
        parallelism: 1
        completions: 1
        backoffLimit: 0
        template:
          spec:
            containers:
            - name: sleep
              image: quay.io/project-codeflare/busybox:1.36
              command: ["sh", "-c", "sleep 10"]
              resources:
                requests:
                  cpu: 100m
  - name: workers
    template:
      spec:
        parallelism: %v
        completions: %v
        backoffLimit: 0
        template:
          spec:
            containers:
            - name: sleep
              image: quay.io/project-codeflare/busybox:1.36
              command: ["sh", "-c", "sleep 10"]
              resources:
                requests:
                  cpu: %v
`

func jobSet(replicasWorker int, milliCPUWorker int64) workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(jobSetYAML,
		randName("jobset"),
		replicasWorker, replicasWorker,
		resource.NewMilliQuantity(milliCPUWorker, resource.DecimalSI),
	)
	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return workloadv1beta2.AppWrapperComponent{
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{
			{Path: "template.spec.replicatedJobs[0].template.spec.template"},
			{Replicas: ptr.To(int32(replicasWorker)), Path: "template.spec.replicatedJobs[1].template.spec.template"},
		},
		Template: runtime.RawExtension{Raw: jsonBytes},
	}
}

const autopilotJobYAML = `
apiVersion: batch/v1
kind: Job
metadata:
  generateName: %v
spec:
  template:
    spec:
      restartPolicy: Never
      terminationGracePeriodSeconds: 0
      containers:
      - name: busybox
        image: quay.io/project-codeflare/busybox:1.36
        command: ["sh", "-c", "sleep 600"]
        resources:
          requests:
            cpu: %v
            nvidia.com/gpu: %v
          limits:
            nvidia.com/gpu: %v
`

func autopilotjob(milliCPU int64, gpus int64) workloadv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(autopilotJobYAML,
		"apjob-",
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI),
		resource.NewQuantity(gpus, resource.DecimalSI),
		resource.NewQuantity(gpus, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return workloadv1beta2.AppWrapperComponent{
		Annotations: map[string]string{
			workloadv1beta2.RetryPausePeriodDurationAnnotation:   "5s",
			workloadv1beta2.FailureGracePeriodDurationAnnotation: "5s",
		},
		DeclaredPodSets: []workloadv1beta2.AppWrapperPodSet{{Path: "template.spec.template"}},
		Template:        runtime.RawExtension{Raw: jsonBytes},
	}
}
