---
permalink: /dev-setup/
title: "Development Setup"
classes: wide
---

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
