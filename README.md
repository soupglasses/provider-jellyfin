<!--
SPDX-FileCopyrightText: 2026 Sofie Finnes <sofie+git@finnes.dev>
SPDX-License-Identifier: MPL-2.0
-->

# Jellyfin Upjet provider

This controller embeds the Jellyfin Terraform Plugin Framework provider through
Upjet. It runs no Terraform executable and needs no Terraform state volume.
The upstream fork and revision are pinned in [go.mod](go.mod).

The Go module is nested under the upstream module path because the upstream
provider constructor is internal. The `replace` directive downloads the pinned
fork; builds do not depend on another local checkout. A public upstream constructor
would let this package use the ordinary repository module path.

`config/provider.go` selects resources and stable external identifiers. Upjet
generates the APIs and reconcilers; application behavior stays in the upstream
Terraform provider. Plugin names and repository URLs remain external identifiers
even though Jellyfin returns different IDs. Only namespaced APIs are installed.
The controller watches one configured namespace and resolves credentials locally.

## Build and validation

```sh
mise install
mise exec -- bash build.sh /tmp/provider-jellyfin.xpkg linux/amd64
mise exec -- env PROVIDER_PACKAGE=/tmp/provider-jellyfin.xpkg bash kind-test.sh
```

The native package test installs Crossplane and an empty Jellyfin instance in a
restricted kind cluster. It verifies initial setup, all resource kinds, observation without writes,
Secret rotation, settings updates, drift correction, controller restarts and
orphan/adopt/delete policies while preserving unrelated server state.
Set `JELLYFIN_TEST_IMAGE` to test a local Jellyfin image.

Generated APIs and CRDs are committed. Run `bash generate.sh` with the tools pinned
in the Dockerfile, and copy `generated/crds/` into `package/crds/`. CI checks that
these files reproduce and runs controller and connector behavior tests.

## Configuration and adoption

The example in [tests/provider.yaml](tests/provider.yaml) watches the `media`
namespace. Configure a namespaced ProviderConfig with the server endpoint and
local credential references. On an empty server, the embedded Terraform provider
creates the initial administrator and completes setup. Existing servers authenticate
without recreating accounts or resetting passwords.

Start adoption with `[Observe]`, confirm the intended objects, then enable
`[Observe, Create, Update]` after stopping other settings writers. Excluding Delete
preserves application objects when Kubernetes resources are pruned. Library path
changes require replacement and should be planned as a migration.

Native Crossplane provider controllers receive cluster-wide Secret permissions.
Namespace-scoped reconciliation does not create a Kubernetes authorization boundary;
install this package only as a trusted controller.

## Licensing

This repository is licensed under MPL-2.0, matching the embedded Terraform provider.
Generated Crossplane artifacts retain their Apache-2.0 notices. Runtime images
include both provider licenses.
