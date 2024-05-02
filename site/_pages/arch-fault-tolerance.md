---
permalink: /arch-fault-tolerance/
title: "Fault Tolerance"
classes: wide
---

### Overall Design

The `podSets` contained in the AppWrapper specification enable the AppWrapper
controller to inject labels into every Pod that is created by
the workload during its execution. Throughout the execution of the
workload, the AppWrapper controller monitors the number and health of
all labeled Pods and uses this information to determine if a
workload is unhealthy.  A workload can be deemed *unhealthy* if any of
the following conditions are true:
   + There are a non-zero number of `Failed` Pods.
   + It takes longer than `AdmissionGracePeriod` for the expected
     number of Pods to at least reach the `Pending` state.
   + It takes longer than the `WarmupGracePeriod` for the expected
     number of Pods to at least reach the `Running` state.

If a workload is determined to be unhealthy, the AppWrapper controller
first waits for a `FailureGracePeriod` to allow the primary resource
controller an opportunity to react and return the workload to a
healthy state.  If the `FailureGracePeriod` expires, the AppWrapper
controller will *reset* the workload by deleting its resources, waiting
for a `ResetPause`, and then creating new instances of the resources.
During this reset period, the AppWrapper **does not** release the workload's
quota; this ensures that when the resources are recreated they will still
have sufficient quota to execute.  The number of times an AppWrapper is reset
is tracked as part of its status; if the number of resets exceeds the `RetryLimit`,
then the AppWrapper moves into a `Failed` state and its resources are deleted
(thus finally releasing its quota). If at any time during this retry loop,
an AppWrapper is suspended (ie, Kueue decides to preempt the AppWrapper),
the AppWrapper controller will respect this request by proceeding to delete
the resources

When the AppWrapper controller decides to delete the resources for a workload,
it proceeds through several phases.  First it does a normal delete of the
resources, allowing the primary resource controllers time to cascade the deletion
through all child resources. During a `DeletionGracePeriod`, the AppWrapper controller
monitors to see if the primary controllers have managed to successfully delete
all of the workload's Pods and resources.  If they fail to accomplish this within
the `DeletionGracePeriod`, the AppWrapper controller then initiates a *forceful*
deletion of all remaining Pods and resources by deleting them with a `GracePeriod` of `0`.
An AppWrapper will continue to have its `ResourcesDeployed` condition to be
`True` until all resources and Pods are successfully deleted.

This process ensures that when `ResourcesDeployed` becomes `False`, which
indicates to Kueue that the quota has been released, all resources created by
a failed workload will have been totally removed from the cluster.

### Configuration Details

The parameters of the retry loop described about are configured at the operator level
and can be customized on a per-AppWrapper basis by adding annotations.
The table below lists the parameters, gives their default, and the annotation that
can be used to customize them.

| Parameter              | Default Value | Annotation                                                       |
|------------------------|---------------|------------------------------------------------------------------|
| AdmissionGracePeriod   |      1 Minute | workload.codeflare.dev.appwrapper/admissionGracePeriodDuration   |
| WarmupGracePeriod      |     5 Minutes | workload.codeflare.dev.appwrapper/warmupGracePeriodDuration      |
| FailureGracePeriod     |      1 Minute | workload.codeflare.dev.appwrapper/failureGracePeriodDuration     |
| ResetPause             |    90 Seconds | workload.codeflare.dev.appwrapper/resetPauseDuration             |
| RetryLimit             |             3 | workload.codeflare.dev.appwrapper/retryLimit                     |
| DeletionGracePeriod    |    10 Minutes | workload.codeflare.dev.appwrapper/deletionGracePeriodDuration    |
| GracePeriodCeiling     |      24 Hours | Not Applicable                                                   |
| SuccessTTLCeiling      |        7 Days | workload.codeflare.dev.appwrapper/successTTLDuration             |


The `GracePeriodCeiling` imposes an upper limit on the other grace periods to
reduce the impact of user-added annotations on overall system utilization.

To support debugging `Failed` workloads, an additional annotation
`workload.codeflare.dev.appwrapper/debuggingFailureDeletionDelayDuration` can
be added to an AppWrapper when it is created to add a delay between the time the
AppWrapper enters the `Failed` state and when the process of deleting its resources
begins. Since the AppWrapper continues to consume quota during this delayed deletion period,
this annotation should be used sparingly and only when interactive debugging of
the failed workload is being actively pursued.

All child resources for an AppWrapper that successfully completed will be automatically
deleted `SuccessTTLCeiling` time after the AppWrapper entered the `Succeeded` state.
This duration can be shortened on a per-AppWrapper basis using the
`workload.codeflare.dev.appwrapper/successTTLDuration` annotation.
