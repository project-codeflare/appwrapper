# AppWrapper State Diagram

The state diagram below describes the transitions between the Phases of an AppWrapper. These states are augmented by two orthogonal conditions:
   + QuotaReserved indicates whether the AppWrapper is considered Active by Kueue.
   + ResourcesDeployed indicates whether wrapped resources may exist on the cluster.

QuotaReserved and ResourcesDeployed are both true in states colored blue below.

QuotaReserved and ResourcesDeployed will initially be true in the Failed state (pink),
but will become false when the controller succeeds at deleting the resources created
in the Resuming phase.

ResourcesDeployed will be true in the Succeeded state (green), but QuotaReserved will be false.

Any phase may transition to the Terminating phase (not shown) when the AppWrapper is deleted.
During the Terminating phase, QuotaReserved and ResourcesDeployed may initially be true
but will become false once the controller succeeds at deleting any associated resources.

```mermaid
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
    sg --> sd

    %% Failures
    rs --> f
    rn --> f
    rn --> rt : Pod Failures
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
