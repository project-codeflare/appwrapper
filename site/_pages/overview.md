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
resource deletions to ensure that either the workload returns to a
healthy state or it is cleanly removed from the cluster and its quota
freed for use by other workloads.
