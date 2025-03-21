# Sample AppWrappers

This directory contains a number of example yamls showing how to wrap
different Pod-creating Kubernetes resources in an AppWrapper.
An AppWrapper can be used to wrap one or more instances of
any Kubernetes Kind that uses `PodSpecTemplate` to define its Pods.
An AppWrapper must contain at least one such Pod-creating resource in addition
to zero or more non-Pod-creating resources.

An AppWrapper contains a`components` array containing the wrapped resources.
Each component has two main pieces: a `template` that defines the wrapped resource
and a `podSets` array that gives the `replicas` and `path` within the template
for each `PodSpecTemplate`.   For correct operation of the AppWrapper, it is
required that the provided `path` and `replicas` information correctly represent
the Pod creating behavior of the wrapped resource.  For resources that do not
created Pods (eg `Services` or `Secrets`) `podSets` should be empty and thus omitted.

To simplify the user experience, for a selection of commonly-used Kubernetes
resource Kinds, the AppWrapper controller can automatically infer the `podSets`
array if it is not provided. For these same Kinds, the AppWrapper controller
will validate that any explicitly provided `podSet` entries do in fact match the
definitions in `template`.
The current set of automatically inferred Kinds is:
   + v1 Pod
   + apps/v1 Deployment
   + apps/v1 StatefulSet
   + batch/v1 Job
   + kubeflow.org/v1 PyTorchJob
   + ray.io/v1 RayCluster
   + ray.io/v1 RayJob
   + jobset.x-k8s.io/v1alpha2 JobSet

In all of the examples, if `podSets` inference is supported for the wrapped Kind,
then `podSets` is omitted from the sample yaml.
