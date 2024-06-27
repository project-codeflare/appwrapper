# AppWrapper

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0)
[![Continuous Integration](https://github.com/project-codeflare/appwrapper/actions/workflows/CI.yaml/badge.svg)](https://github.com/project-codeflare/appwrapper/actions/workflows/CI.yaml)

An AppWrapper contains a collection of Kubernetes resources that a
user desires to manage as a single logical workload. AppWrappers are
designed to smoothly interoperate with
[Kueue](https://kueue.sigs.k8s.io).  They provide a flexible and
workload-agnostic mechanism for enabling Kueue to manage a group of
Kubernetes resources as a single logical unit without requiring any
Kueue-specific support by the controllers of those resources. If the
operators for some of the contained resources are Kueue-aware, the
AppWrapper operator ensures that when Kueue admits an AppWrapper for
execution, all of the necessary information will be propagated
to cause the child's Kueue-enabled operator to admit it as well.
For a more detailed description of the overall design, see the
[Architecture](https://project-codeflare.github.io/appwrapper/arch-controller/)
section of our website.

AppWrappers are designed to harden workloads by providing an
additional level of automatic fault detection and recovery. The AppWrapper
controller monitors the health of the workload and if corrective actions
are not taken by the primary resource controllers within specified deadlines,
the AppWrapper controller will orchestrate workload-level retries and
resource deletion to ensure that either the workload returns to a
healthy state or is cleanly removed from the cluster and its quota
freed for use by other workloads.  For details on customizing and
configuring these fault tolerance capabilities, please see the
[Fault Tolerance](https://project-codeflare.github.io/appwrapper/arch-controller/)
section of our website.

## Installation

To install the latest release of AppWrapper in a Kubernetes cluster with Kueue already installed
and configured, simply run the command:

```sh
kubectl apply --server-side -f https://github.com/project-codeflare/appwrapper/releases/download/v0.20.1/install.yaml
```

The controller runs in the `appwrapper-system` namespace.

Read the [Quick Start Guide](https://project-codeflare.github.io/appwrapper/quick-start/) to learn more.

## Usage

For example of AppWrapper usage, browse our [Samples](./samples) directory or
see the [Samples](https://project-codeflare.github.io/appwrapper/samples/) section
of the project website.

## Development

To contribute to the AppWrapper project and for detailed instructions on how to
build and deploy the project from source, see the
[Development Setup](https://project-codeflare.github.io/appwrapper/dev-setup/) section
of the project website.

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
