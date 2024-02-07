# AppWrapper

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0)
[![Continuous Integration](https://github.com/project-codeflare/appwrapper/actions/workflows/CI.yaml/badge.svg)](https://github.com/project-codeflare/appwrapper/actions/workflows/CI.yaml)

An AppWrapper is a collection of Kubernetes resources than can be jointly queued
and dispatched using [Kueue](https://kueue.sigs.k8s.io).

## Description
// TODO(user): An in-depth paragraph about your project and overview of use

## Getting Started

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

Deploy Kueue on the cluster with:
```sh
./hack/deploy-kueue.sh
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
kubectl apply -f samples/appwrapper.yaml
```

You should shortly see a Pod called `sample` running.
After running for 5 seconds, the Pod will complete and the
AppWrapper's status will be Completed.
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
sample   Completed
```

You can now delete the sample AppWrapper.
```sh
kubectl delete -f samples/appwrapper.yaml
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
samples/appwrapper.yaml` and observe its execution as described
above.

After deleting all AppWrapper CR instances, you can uninstall the CRDs
with:
```sh
make uninstall
```

## Contributing

### Pre-commit hooks

This repository includes pre-configured pre-commit hooks. Make sure to install
the hooks immediately after cloning the repository:
```sh
pre-commit install
```
See [https://pre-commit.com](https://pre-commit.com) for prerequisites.

### Running tests locally

TODO: Document this once we finish porting the scripts from MCADv2.

## License

Copyright 2024 IBM Corporation.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
