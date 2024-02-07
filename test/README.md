# AppWrapper End-to-End  Tests

This directory contains both go and kuttl tests suites that are
designed to be run against an AppWrapper operator deployed on a
Kubernetes cluster with Kueue installed.

The [../hack/](../hack) directory contains scripts that can be used to
create an appropriately configured test cluster using `kind` and to run
the tests.  The tests can be run in two primary modes:
  1. ***Fully automated***  The script [../hack/run-e2e-kind.sh](../hack/run-e2e-kind.sh)
    fully automates creating a properly configured `kind` cluster, deploying
    Kueue and the AppWrapper CRDs and controller on the cluster, running all
    tests on the cluster, and then deleting the cluster when the tests are completed.
    For easy of use, run this script by doing a `make run-e2e` to ensure that the tests are run
    against an image that contains your locally modified code.
  2. ***Development mode*** The script [../hack/create-test-cluster.sh](../hack/create-test-cluster.sh)
     can be used to create a correctly configured test cluster.
     The script [../hack/deploy-kueue.sh](../hack/deploy-kueue.sh) can be used to
     deploy Kueue on the test cluster.
     You can then use either `make install; make run` or `make install; make kind-push; make deploy`
     to deploy the AppWrapper CRDs and run the controller.
     After everything is running, you can then either run test cases
     individually or use the script
     [../hack/run-tests-on-cluster.sh](../hack/run-tests-on-cluster.sh) to
     run the entire AppWrapper test suite against the system you just deployed.
