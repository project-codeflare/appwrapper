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

package webhook

import (
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/gomega"

	awv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
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

func toAppWrapper(components ...awv1beta2.AppWrapperComponent) *awv1beta2.AppWrapper {
	return &awv1beta2.AppWrapper{
		TypeMeta:   metav1.TypeMeta{APIVersion: awv1beta2.GroupVersion.String(), Kind: "AppWrapper"},
		ObjectMeta: metav1.ObjectMeta{Name: randName("aw"), Namespace: "default"},
		Spec:       awv1beta2.AppWrapperSpec{Components: components},
	}
}

func getAppWrapper(typeNamespacedName types.NamespacedName) *awv1beta2.AppWrapper {
	aw := &awv1beta2.AppWrapper{}
	err := k8sClient.Get(ctx, typeNamespacedName, aw)
	Expect(err).NotTo(HaveOccurred())
	return aw
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

func pod(milliCPU int64) awv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(podYAML,
		randName("pod"),
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return awv1beta2.AppWrapperComponent{
		DeclaredPodSets: []awv1beta2.AppWrapperPodSet{{Path: "template"}},
		Template:        runtime.RawExtension{Raw: jsonBytes},
	}
}

func podForInference(milliCPU int64) awv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(podYAML,
		randName("pod"),
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return awv1beta2.AppWrapperComponent{
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
    command: ["sh", "-c", "sleep 10"]
    resources:
      requests:
        cpu: %v`

func namespacedPod(namespace string, milliCPU int64) awv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(namespacedPodYAML,
		randName("pod"),
		namespace,
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return awv1beta2.AppWrapperComponent{
		DeclaredPodSets: []awv1beta2.AppWrapperPodSet{{Path: "template"}},
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

func service() awv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(serviceYAML, randName("service"))
	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return awv1beta2.AppWrapperComponent{
		DeclaredPodSets: []awv1beta2.AppWrapperPodSet{},
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

func deployment(replicaCount int, milliCPU int64) awv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(deploymentYAML,
		randName("deployment"),
		replicaCount,
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return awv1beta2.AppWrapperComponent{
		DeclaredPodSets: []awv1beta2.AppWrapperPodSet{{Replicas: ptr.To(int32(replicaCount)), Path: "template.spec.template"}},
		Template:        runtime.RawExtension{Raw: jsonBytes},
	}
}

func deploymentForInference(replicaCount int, milliCPU int64) awv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(deploymentYAML,
		randName("deployment"),
		replicaCount,
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return awv1beta2.AppWrapperComponent{
		Template: runtime.RawExtension{Raw: jsonBytes},
	}
}

const rayClusterYAML = `
apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: %v
spec:
  autoscalerOptions:
    idleTimeoutSeconds: 60
    imagePullPolicy: Always
    resources:
      limits:
        cpu: 500m
        memory: 512Mi
      requests:
        cpu: 500m
        memory: 512Mi
    upscalingMode: Default
  enableInTreeAutoscaling: false
  headGroupSpec:
    rayStartParams:
      block: 'true'
      dashboard-host: 0.0.0.0
      num-gpus: '0'
    serviceType: ClusterIP
    template:
      spec:
        containers:
        - env:
          - name: MY_POD_IP
            valueFrom:
              fieldRef:
                fieldPath: status.podIP
          - name: RAY_USE_TLS
            value: '0'
          - name: RAY_TLS_SERVER_CERT
            value: /home/ray/workspace/tls/server.crt
          - name: RAY_TLS_SERVER_KEY
            value: /home/ray/workspace/tls/server.key
          - name: RAY_TLS_CA_CERT
            value: /home/ray/workspace/tls/ca.crt
          image: quay.io/project-codeflare/ray:latest-py39-cu118
          imagePullPolicy: Always
          lifecycle:
            preStop:
              exec:
                command:
                - /bin/sh
                - -c
                - ray stop
          name: ray-head
          ports:
          - containerPort: 6379
            name: gcs
          - containerPort: 8265
            name: dashboard
          - containerPort: 10001
            name: client
          resources:
            limits:
              cpu: 2
              memory: 8G
              nvidia.com/gpu: 0
            requests:
              cpu: 2
              memory: 8G
              nvidia.com/gpu: 0
          volumeMounts:
          - mountPath: /etc/pki/tls/certs/odh-trusted-ca-bundle.crt
            name: odh-trusted-ca-cert
            subPath: odh-trusted-ca-bundle.crt
          - mountPath: /etc/ssl/certs/odh-trusted-ca-bundle.crt
            name: odh-trusted-ca-cert
            subPath: odh-trusted-ca-bundle.crt
          - mountPath: /etc/pki/tls/certs/odh-ca-bundle.crt
            name: odh-ca-cert
            subPath: odh-ca-bundle.crt
          - mountPath: /etc/ssl/certs/odh-ca-bundle.crt
            name: odh-ca-cert
            subPath: odh-ca-bundle.crt
        imagePullSecrets:
        - name: unit-test-pull-secret
        volumes:
        - configMap:
            items:
            - key: ca-bundle.crt
              path: odh-trusted-ca-bundle.crt
            name: odh-trusted-ca-bundle
            optional: true
          name: odh-trusted-ca-cert
        - configMap:
            items:
            - key: odh-ca-bundle.crt
              path: odh-ca-bundle.crt
            name: odh-trusted-ca-bundle
            optional: true
          name: odh-ca-cert
  rayVersion: 2.7.0
  workerGroupSpecs:
  - groupName: small-group-unit-test-cluster-ray
    maxReplicas: %v
    minReplicas: %v
    rayStartParams:
      block: 'true'
      num-gpus: '7'
    replicas: %v
    template:
      metadata:
        annotations:
          key: value
        labels:
          key: value
      spec:
        containers:
        - env:
          - name: MY_POD_IP
            valueFrom:
              fieldRef:
                fieldPath: status.podIP
          - name: RAY_USE_TLS
            value: '0'
          - name: RAY_TLS_SERVER_CERT
            value: /home/ray/workspace/tls/server.crt
          - name: RAY_TLS_SERVER_KEY
            value: /home/ray/workspace/tls/server.key
          - name: RAY_TLS_CA_CERT
            value: /home/ray/workspace/tls/ca.crt
          image: quay.io/project-codeflare/ray:latest-py39-cu118
          lifecycle:
            preStop:
              exec:
                command:
                - /bin/sh
                - -c
                - ray stop
          name: machine-learning
          resources:
            requests:
              cpu: %v
              memory: 5G
              nvidia.com/gpu: 7
          volumeMounts:
          - mountPath: /etc/pki/tls/certs/odh-trusted-ca-bundle.crt
            name: odh-trusted-ca-cert
            subPath: odh-trusted-ca-bundle.crt
          - mountPath: /etc/ssl/certs/odh-trusted-ca-bundle.crt
            name: odh-trusted-ca-cert
            subPath: odh-trusted-ca-bundle.crt
          - mountPath: /etc/pki/tls/certs/odh-ca-bundle.crt
            name: odh-ca-cert
            subPath: odh-ca-bundle.crt
          - mountPath: /etc/ssl/certs/odh-ca-bundle.crt
            name: odh-ca-cert
            subPath: odh-ca-bundle.crt
        imagePullSecrets:
        - name: unit-test-pull-secret
        volumes:
        - configMap:
            items:
            - key: ca-bundle.crt
              path: odh-trusted-ca-bundle.crt
            name: odh-trusted-ca-bundle
            optional: true
          name: odh-trusted-ca-cert
        - configMap:
            items:
            - key: odh-ca-bundle.crt
              path: odh-ca-bundle.crt
            name: odh-trusted-ca-bundle
            optional: true
          name: odh-ca-cert
`

func rayCluster(workerCount int, milliCPU int64) awv1beta2.AppWrapperComponent {
	workerCPU := resource.NewMilliQuantity(milliCPU, resource.DecimalSI)
	yamlString := fmt.Sprintf(rayClusterYAML,
		randName("raycluster"),
		workerCount, workerCount, workerCount,
		workerCPU)

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return awv1beta2.AppWrapperComponent{
		DeclaredPodSets: []awv1beta2.AppWrapperPodSet{
			{Replicas: ptr.To(int32(1)), Path: "template.spec.headGroupSpec.template"},
			{Replicas: ptr.To(int32(workerCount)), Path: "template.spec.workerGroupSpecs[0].template"},
		},
		Template: runtime.RawExtension{Raw: jsonBytes},
	}
}

func rayClusterForInference(workerCount int, milliCPU int64) awv1beta2.AppWrapperComponent {
	workerCPU := resource.NewMilliQuantity(milliCPU, resource.DecimalSI)
	yamlString := fmt.Sprintf(rayClusterYAML,
		randName("raycluster"),
		workerCount, workerCount, workerCount,
		workerCPU)

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return awv1beta2.AppWrapperComponent{
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

func jobSet(replicasWorker int, milliCPUWorker int64) awv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(jobSetYAML,
		randName("jobset"),
		replicasWorker, replicasWorker,
		resource.NewMilliQuantity(milliCPUWorker, resource.DecimalSI),
	)
	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return awv1beta2.AppWrapperComponent{
		DeclaredPodSets: []awv1beta2.AppWrapperPodSet{
			{Path: "template.spec.replicatedJobs[0].template.spec.template"},
			{Replicas: ptr.To(int32(replicasWorker)), Path: "template.spec.replicatedJobs[1].template.spec.template"},
		},
		Template: runtime.RawExtension{Raw: jsonBytes},
	}
}

const jobYAML = `
apiVersion: batch/v1
kind: Job
metadata:
  name: %v
spec:
  parallelism: %v
  completions: %v
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: busybox
        image: quay.io/project-codeflare/busybox:1.36
        command: ["sh", "-c", "sleep 30"]
        resources:
          requests:
            cpu: %v`

func jobForInference(parallelism int, completions int, milliCPU int64) awv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(jobYAML,
		randName("job"),
		parallelism,
		completions,
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return awv1beta2.AppWrapperComponent{
		Template: runtime.RawExtension{Raw: jsonBytes},
	}
}

const pytorchJobYAML = `
apiVersion: "kubeflow.org/v1"
kind: PyTorchJob
metadata:
  name: %v
spec:
  pytorchReplicaSpecs:
    Master:
      restartPolicy: OnFailure
      template:
        spec:
          containers:
          - name: pytorch
            image: docker.io/kubeflowkatib/pytorch-mnist-cpu:v1beta1-fc858d1
            command:
            - "python3"
            - "/opt/pytorch-mnist/mnist.py"
            - "--epochs=1"
            resources:
              requests:
                cpu: %v
    Worker:
      replicas: %v
      restartPolicy: OnFailure
      template:
        spec:
          containers:
          - name: pytorch
            image: docker.io/kubeflowkatib/pytorch-mnist-cpu:v1beta1-fc858d1
            command:
            - "python3"
            - "/opt/pytorch-mnist/mnist.py"
            - "--epochs=1"
            resources:
              requests:
                cpu: %v`

func pytorchJobForInference(masterMilliCPU int64, workerReplicas int, workerMilliCPU int64) awv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(pytorchJobYAML,
		randName("pytorch-job"),
		resource.NewMilliQuantity(masterMilliCPU, resource.DecimalSI),
		workerReplicas,
		resource.NewMilliQuantity(workerMilliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return awv1beta2.AppWrapperComponent{
		Template: runtime.RawExtension{Raw: jsonBytes},
	}
}

const rayJobYAML = `
apiVersion: ray.io/v1
kind: RayJob
metadata:
  name: %v
spec:
  rayClusterSpec:
    headGroupSpec:
      template:
        spec:
          containers:
            - name: ray-head
              image: rayproject/ray:2.9.0
              resources:
                requests:
                  cpu: 1
    workerGroupSpecs:
      - replicas: %v
        template:
          spec:
            containers:
              - name: ray-worker
                image: rayproject/ray:2.9.0
                resources:
                  requests:
                    cpu: %v
`

func rayJobForInference(workerCount int, milliCPU int64) awv1beta2.AppWrapperComponent {
	yamlString := fmt.Sprintf(rayJobYAML,
		randName("rayjob"),
		workerCount,
		resource.NewMilliQuantity(milliCPU, resource.DecimalSI))

	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlString))
	Expect(err).NotTo(HaveOccurred())
	return awv1beta2.AppWrapperComponent{
		Template: runtime.RawExtension{Raw: jsonBytes},
	}
}
