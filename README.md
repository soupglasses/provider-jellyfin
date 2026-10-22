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

Generated APIs, controllers, CRDs and the Terraform schema are committed. To regenerate
with the tools pinned in the Dockerfile:

```sh
docker buildx build --target generated --output type=local,dest=. .
```

With those tools installed locally, `bash generate.sh` updates the same files directly.
Upjet generates both scopes; the script discards cluster APIs and controllers because
this provider serves only namespaced resources. CI checks generation reproducibility
and runs controller and connector behavior tests.

## Development

Keep application behavior in the embedded Terraform provider and packaging concerns
here. Commit generated files alongside source changes so Go tooling can resolve imports.
Generated files use [`.gitattributes`](.gitattributes) to
[collapse their diffs on GitHub](https://docs.github.com/en/repositories/working-with-files/managing-files/customizing-how-changed-files-appear-on-github).

CI publishes commit-qualified main packages after unit and native-package tests.
A semantic `v*` tag on main publishes a multiarchitecture package and release checksums.
Never move release tags; publish a new version instead.

Renovate groups Go dependency updates to preserve Kubernetes client compatibility.
It also updates Actions, build tools, container digests and native test dependencies.
Dependency upgrades pass the same behavior checks as other changes.

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
