## Release Instructions

1. Submit a housekeeping PR that does the following:
   + Update the AppWrapper version number in the installation section of [README.md](../README.md#Installation).
   + Update the `appwrapper_version` variable in [_config.yaml](../site/_config.yaml).

2. After merging the PR, create a new release tag (vX.Y.Z) and push the
tag to the main branch.  This will trigger the `release` workflow which
will:
   + build, tag, and push images to [quay.io/ibm/appwrapper](https://quay.io/repository/ibm/appwrapper)
   + generate the install.yaml for the release
   + create a [GitHub release](https://github.com/project-codeflare/appwrapper/releases) that contains the install.yaml

3. To workaround back level go versions in ODH, we also maintain a
   codeflare-releases branch.  After making a release, rebase the
   codeflare-releases branch and then tag it using a `cf` prefix
   instead of a `v`. (eg v0.21.2 ==> cf0.21.2).  After making this
   tag, you can update the codeflare-operator, using the vX.Y.Z tag in
   the Makefile and the cfX.Y.Z in the replace clause in codeflare's
   go.mod.
