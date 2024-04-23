---
permalink: /samples/batch-job/
title: "AppWrapper Containing a Batch Job"
classes: wide
---


```yaml
apiVersion: workload.codeflare.dev/v1beta2
kind: AppWrapper
metadata:
  name: sample-job
  annotations:
    kueue.x-k8s.io/queue-name: user-queue
spec:
  components:
  - podSets:
    - replicas: 1
      path: template.spec.template
    template:
      apiVersion: batch/v1
      kind: Job
      metadata:
        name: sample-job
      spec:
        template:
          spec:
            restartPolicy: Never
            containers:
            - name: busybox
              image: quay.io/project-codeflare/busybox:1.36
              command: ["sh", "-c", "sleep 30"]
              resources:
                requests:
                  cpu: 1

```
