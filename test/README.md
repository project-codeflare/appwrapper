# AppWrapper End-to-End  Tests

This directory contains both go and kuttl tests suites that are
designed to be run against an AppWrapper operator deployed on a
Kubernetes cluster with Kueue and the Kubeflow operator installed.

The [../hack/](../hack) directory contains scripts that can be used to
create an appropriately configured test cluster using `kind` and to run
the tests.
