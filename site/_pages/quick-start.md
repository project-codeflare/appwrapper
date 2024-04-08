---
permalink: /quick-start/
title: "Quick Start Guide"
classes: wide
---

## Installing from a Release

If you have a Kubernetes cluster with Kueue installed, you can install
a release of the AppWrapper CRD and operator simply by doing:
```sh
kubectl apply --server-side -f https://github.com/project-codeflare/appwrapper/releases/download/RELEASE_VERSION/install.yaml
```
Replace `RELEASE_VERSION` in the above URL with an actual version, for example `v0.3.2`.
