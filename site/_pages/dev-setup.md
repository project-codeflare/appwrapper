---
permalink: /dev-setup/
title: "Development Setup"
classes: wide
---

### Prerequisites

You'll need `go` v1.21.0+ installed on your development machine.

You'll need a container runtime and cli (eg `docker` or `rancher-desktop`).

You’ll need a Kubernetes cluster to run against.

You can use [kind](https://sigs.k8s.io/kind) to get a local cluster
for testing, or run against a remote cluster. All commands shown in
this readme will automatically use the current context in your
kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

For the purposes of simplifying the getting started documentation, we
proceed assuming you will create a local `kind` cluster.

### Create your cluster and deploy Kueue

Create the cluster with:
```sh
./hack/create-test-cluster.sh
```

Deploy Kueue on the cluster and configure it to have queues in your default namespace
with a nominal quota of 4 CPUs with:
```sh
./hack/deploy-kueue.sh
```

You can verify Kueue is configured as expected with:
```sh
% kubectl get localqueues,clusterqueues -o wide
NAME                                   CLUSTERQUEUE    PENDING WORKLOADS   ADMITTED WORKLOADS
localqueue.kueue.x-k8s.io/user-queue   cluster-queue   0                   0

NAME                                        COHORT   STRATEGY         PENDING WORKLOADS   ADMITTED WORKLOADS
clusterqueue.kueue.x-k8s.io/cluster-queue            BestEffortFIFO   0                   0
```

### Deploy on the cluster

Build your image and push it to the cluster with:
```sh
make docker-build kind-push
```

Deploy the CRDs and controller to the cluster:
```sh
make deploy
```

Within a few seconds, the controller pod in the `appwrapper-system`
namespace should be Ready.  Verify this with:
```sh
kubectl get pods -n appwrapper-system
```

You can now try deploying a sample `AppWrapper`:
```sh
kubectl apply -f samples/wrapped-pod.yaml
```

You should shortly see a Pod called `sample` running.
After running for 5 seconds, the Pod will complete and the
AppWrapper's status will be Succeeded.
```sh
% kubectl get appwrappers
NAME     STATUS
sample   Running
% kubectl get pods
NAME     READY   STATUS    RESTARTS   AGE
sample   1/1     Running   0          2s
% kubectl get pods
NAME     READY   STATUS      RESTARTS   AGE
sample   0/1     Completed   0          9s
% kubectl get appwrappers
NAME     STATUS
sample   Succeeded
```

You can now delete the sample AppWrapper.
```sh
kubectl delete -f samples/wrapped-pod.yaml
```

To undeploy the CRDs and controller from the cluster:
```sh
make undeploy
```

### Run the controller as a local process against the cluster

For faster development and debugging, you can run the controller
directly on your development machine as local process that will
automatically be connected to the cluster.  Note that in this
configuration, the webhooks that implement the Admission Controllers
are not operational.  Therefore your CRDs will not be validated and
you must explictly set the `suspended` field to `true` in your
AppWrapper YAML files.

Install the CRDs into the cluster:

```sh
make install
```

Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):
```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

You can now deploy a sample with `kubectl apply -f
samples/wrapped-pod.yaml` and observe its execution as described
above.

After deleting all AppWrapper CR instances, you can uninstall the CRDs
with:
```sh
make uninstall
```













### Pre-commit hooks

This repository includes pre-configured pre-commit hooks. Make sure to install
the hooks immediately after cloning the repository:
```sh
pre-commit install
```
See [https://pre-commit.com](https://pre-commit.com) for prerequisites.

### Running unit tests

Unit tests can be run at any time by doing `make test`.
No additional setup is required.

### Running end-to-end tests

A suite of end-to-end tests are run as part of the project's
[continuous intergration workflow](./.github/workflows/CI.yaml).
These tests can also be run locally aginst a deployed version of Kueue
and the AppWrapper controller.

To create and initialize your cluster, perform the following steps:
```shell
./hack/create-test-cluster.sh
./hack/deploy-kueue.sh
```

Next build and deploy the AppWrapper operator
```shell
make docker-build kind-push
make deploy
```

Finally, run the test suite
```shell
./hack/run-tests-on-cluster.sh
```
