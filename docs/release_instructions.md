## Release Instructions

The entire AppWrapper release process is automated via GitHub actions.

To make a release, simply create a new release tag (vX.Y.Z) and push the
tag to the main branch.  This will trigger the `release` workflow which
will:
   + build, tag, and push images to [quay.io/ibm/appwrapper](https://quay.io/repository/ibm/appwrapper)
   + generate the install.yaml for the release
   + create a [GitHub release](https://github.com/project-codeflare/appwrapper/releases) that contains the install.yaml


## Installing from a Release

If you have a Kubernetes cluster with Kueue installed, you can install
a release of the AppWrapper CRD and operator simply by doing:
```sh
kubectl apply --server-side -f https://github.com/project-codeflare/appwrapper/releases/download/RELEASE_VERSION/install.yaml
```
Replace `RELEASE_VERSION` in the above URL with an actual version, for example `v0.3.2`.
