# Sample AppWrappers

This directory contains a number of example yamls showing how to wrap
different Pod-creating Kubernetes resources in an AppWrapper.
AppWrappers can be used to wrap any Kubernetes Kind that uses `PodSpecTemplate`
to express its Pods.

An AppWrapper contains a `component` array of `AppWrapperComponents`.
Each component has two main pieces: a `template` that defines the wrapped resource
and a `podSets` array that gives the `replicas` and `path` within the template
for each `PodSpecTemplate`.   For correct operation of the AppWrapper, it is
required that the provided `path` and `replicas` information correctly represent
the Pod creating behavior of the wrapped resource.

To simplify the user experience, for a selection of commonly-used Kubernetes
resource Kinds, the AppWrapper controller can automatically infer the `podSets`
array if it is not provided. For these same kinds, the AppWrapper controller
will validate that any explicitly provided `podSet` entries match the definitions in
`template`. The current set of automatically inferred Kinds is:
   + v1 Pod
   + apps/v1 Deployment
   + apps/v1 StatefulSet
   + batch/v1 Job
   + kubeflow.org/v1 PyTorchJob
   + ray.io/v1 RayCluster
   + ray.io/v1 RayJob

In all the examples, if the Kind supports automatic inference the `podSets`
are elided.
