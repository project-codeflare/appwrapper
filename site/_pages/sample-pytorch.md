---
permalink: /samples/pytorch/
title: "AppWrapper Containing PyTorch Job"
classes: wide
---


```yaml
apiVersion: workload.codeflare.dev/v1beta2
kind: AppWrapper
metadata:
  name: sample-pytorch-job
  annotations:
    kueue.x-k8s.io/queue-name: user-queue
spec:
  components:
  - podSets:
    - replicas: 1
      path: template.spec.pytorchReplicaSpecs.Master.template
    - replicas: 1
      path: template.spec.pytorchReplicaSpecs.Worker.template
    template:
      apiVersion: "kubeflow.org/v1"
      kind: PyTorchJob
      metadata:
        name: pytorch-simple
      spec:
        pytorchReplicaSpecs:
          Master:
            replicas: 1
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
                      cpu: 1
          Worker:
            replicas: 1
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
                      cpu: 1
```