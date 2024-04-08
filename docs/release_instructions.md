## Release Instructions

The AppWrapper project release process is driven via GitHub actions.

To make a release, create a new release tag (vX.Y.Z) and push the
tag to the main branch.  This will trigger the `release` workflow which
will:
   + build, tag, and push images to [quay.io/ibm/appwrapper](https://quay.io/repository/ibm/appwrapper)
   + generate the install.yaml for the release
   + create a [GitHub release](https://github.com/project-codeflare/appwrapper/releases) that contains the install.yaml

After the automated release process completes, do a followup PR containing the
following updates to the main README and project website:
   + Update the AppWrapper version number in the installation section of [README.md](../README.md#Installation).
   + Update the `appwrapper_version` and `kueue_version` variables in [_config.yaml](../site/_config.yaml).
