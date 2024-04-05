---
permalink: /arch-overview/
title: "AppWrapper Design"
classes: wide
---
## Custom Resource Definition

TODO: text + code snippets


## Controller Architecture

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

See [appwrapper_webhook.go](./internal/webhook/appwrapper_webhook.go)
for the implementation.

#### Workload Controller

An instantiation of Kueue’s GenericReconciller along with an
implementation of Kueue’s GenericJob interface for the AppWrapper
CRD. As is standard practice in Kueue, this controller will watch
AppWrapper instances and their owned Workload instances to reconcile
the two. This controller will make it possible for Kueue to suspend,
resume, and constrain the placement of the AppWrapper. It will report
the status of the AppWrapper to Kueue.

See [workload_controller.go](./internal/controller/workload/workload_controller.go)
for the implementation.

A small additional piece of logic is currently needed to generalize
Kueue's ability to recognize parent/children relationships and enforce
that admission by Kueue of the parent AppWrapper will be propagated to
its immediate children.

See [child_admission_controller.go](./internal/controller/workload/child_admission_controller.go)
for the implementation.

#### Framework Controller

A standard reconciliation loop that watches AppWrapper instances and
is responsible for all AppWrapper-specific operations including
creating, monitoring, and deleting the wrapped resources in response
to the modifications of the AppWrapper instance’s specification and
status made by the Workload Controller described above.

This [state transition diagram](docs/state-diagram.md) depicts the
lifecycle of an AppWrapper; the implementation is found in
[appwrapper_controller.go](./internal/controller/appwrapper/appwrapper_controller.go).

TODO: inline the diagram here.
