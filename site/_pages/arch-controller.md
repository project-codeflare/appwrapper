---
permalink: /arch-controller/
title: "AppWrapper Controllers"
classes: wide
---

Kueue has a well-developed pattern for Kueue-enabling a Custom
Resource Definition and its associated operator. Following this pattern
allows the resulting operator to smoothly run alongside the core Kueue
operator. The pattern consists of three main elements: an Admission
Controller, a Workload Controller, and a Framework Controller.

#### Admission Controller

Kueue requires the definition of an Admission Controller that ensures
that the `.spec.suspend` field of newly created AppWrapper instances is
set to true. We also leverage the Admission Controller to ensure that
the user creating the AppWrapper is also entitled to create the contained resources
and to validate AppWrapper-specific invariants.

See [appwrapper_webhook.go]({{ site.gh_main_url }}/internal/webhook/appwrapper_webhook.go)
for the implementation.

#### Workload Controller

An instantiation of Kueue’s GenericReconciller along with an
implementation of Kueue’s GenericJob interface for the AppWrapper
CRD. As is standard practice in Kueue, this controller will watch
AppWrapper instances and their owned Workload instances to reconcile
the two. This controller will make it possible for Kueue to suspend,
resume, and constrain the placement of the AppWrapper. It will report
the status of the AppWrapper to Kueue.

See [workload_controller.go]({{ site.gh_main_url }}/internal/controller/workload/workload_controller.go)
for the implementation.

A small additional piece of logic is currently needed to generalize
Kueue's ability to recognize parent/children relationships and enforce
that admission by Kueue of the parent AppWrapper will be propagated to
its immediate children.

See [child_admission_controller.go]({{ site.gh_main_url }}/internal/controller/workload/child_admission_controller.go)
for the implementation.

#### Framework Controller

A standard reconciliation loop that watches AppWrapper instances and
is responsible for all AppWrapper-specific operations including
creating, monitoring, and deleting the wrapped resources in response
to the modifications of the AppWrapper instance’s specification and
status made by the Workload Controller described above.

```mermaid!
---
title: AppWrapper Phase Transitions
---
stateDiagram-v2
    e : Empty

    sd : Suspended
    rs : Resuming
    rn : Running
    rt : Resetting
    sg : Suspending
    s  : Succeeded
    f  : Failed

    %% Happy Path
    e --> sd
    sd --> rs : Suspend == false
    rs --> rn
    rn --> s

    %% Requeuing
    rs --> sg : Suspend == true
    rn --> sg : Suspend == true
    rt --> sg : Suspend == true
    sg --> sd

    %% Failures
    rs --> f
    rn --> f
    rn --> rt : Workload Unhealthy
    rt --> rs

    classDef quota fill:lightblue
    class rs quota
    class rn quota
    class rt quota
    class sg quota

    classDef failed fill:pink
    class f failed

    classDef succeeded fill:lightgreen
    class s succeeded
```

The state diagram above depicts the transitions between the Phases of an AppWrapper. These states are augmented by two orthogonal conditions:
   + **QuotaReserved** indicates whether the AppWrapper is considered Active by Kueue.
   + **ResourcesDeployed** indicates whether wrapped resources may exist on the cluster.

QuotaReserved and ResourcesDeployed are both true in states colored blue below.

QuotaReserved and ResourcesDeployed will initially be true in the Failed state (pink),
but will become false when the Framework Controller succeeds at deleting all resources created
in the Resuming phase.

ResourcesDeployed will be true in the Succeeded state (green), but QuotaReserved will be false.

Any phase may transition to the Terminating phase (not shown) when the AppWrapper is deleted.
During the Terminating phase, QuotaReserved and ResourcesDeployed may initially be true
but will become false once the Framework Controller succeeds at deleting all associated resources.

See [appwrapper_controller.go]({{ site.gh_main_url }}/internal/controller/appwrapper/appwrapper_controller.go)
for the implementation.
