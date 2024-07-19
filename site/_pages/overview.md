---
layout: single
permalink: /
excerpt: "Project Overview"
redirect_from:
  - /about/
  - /about.html
classes: wide
---

An AppWrapper contains a collection of Kubernetes resources that a
user desires to manage as a single logical workload. AppWrappers are
designed to smoothly interoperate with
[Kueue](https://kueue.sigs.k8s.io).  They provide a flexible and
workload-agnostic mechanism for enabling Kueue to manage a group of
Kubernetes resources as a single logical unit without requiring any
Kueue-specific support by the controllers of those resources.

AppWrappers are designed to harden workloads by providing an
additional level of automatic fault detection and recovery. The AppWrapper
controller monitors the health of the workload and if corrective actions
are not taken by the primary resource controllers within specified deadlines,
the AppWrapper controller will orchestrate workload-level retries and
resource deletion to ensure that either the workload returns to a
healthy state or is cleanly removed from the cluster and its quota
freed for use by other workloads. If [Autopilot](https://github.com/ibm/autopilot)
is also being used on the cluster, the AppWrapper controller can be configured
to automatically inject Node anti-affinities into Pods and to trigger
retries when Pods in already running workloads are using resources
that Autopilot has tagged as unhealthy. For details on customizing and
configuring these fault tolerance capabilities, please see the
[Fault Tolerance](https://project-codeflare.github.io/appwrapper/arch-controller/)
section of our website.

AppWrappers are designed to be used as part of fully open source software stack
to run production batch workloads on Kubernetes and OpenShift. The [MLBatch](https://github.com/project-codeflare/mlbatch)
project leverages [Kueue](https://kueue.sigs.k8s.io), the [Kubeflow Training
Operator](https://www.kubeflow.org/docs/components/training/),
[KubeRay](https://docs.ray.io/en/latest/cluster/kubernetes/index.html), and the
[Codeflare Operator](https://github.com/project-codeflare/codeflare-operator)
from [Red Hat OpenShift
AI](https://www.redhat.com/en/technologies/cloud-computing/openshift/openshift-ai).
MLBatch enables [AppWrappers](https://project-codeflare.github.io/appwrapper/)
and adds
[Coscheduler](https://github.com/kubernetes-sigs/scheduler-plugins/blob/master/pkg/coscheduling/README.md).
MLBatch includes a number of configuration steps to help these components work
in harmony and support large workloads on large clusters.
