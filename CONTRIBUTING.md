<!--
SPDX-FileCopyrightText: 2026 Sofie Finnes <sofie+git@finnes.dev>
SPDX-License-Identifier: MPL-2.0
-->

# Development

Keep application behavior in the embedded Terraform provider. Changes there should
be suitable for contribution to its upstream project; packaging concerns belong here.
The nested Go module path permits the existing internal constructor without requiring
a new public API in the Terraform provider.

Build with `bash build.sh /tmp/provider-jellyfin.xpkg linux/amd64` and run
`PROVIDER_PACKAGE=/tmp/provider-jellyfin.xpkg bash kind-test.sh` through Mise.
`docker buildx build --target generated --output type=local,dest=/tmp/generated .`
exports APIs, controllers, CRDs and the Terraform schema. Commit generated Go files
and CRDs alongside source changes so dependency tooling can resolve local imports.

CI publishes commit-qualified main packages after unit and native-package tests.
A semantic `v*` tag on main publishes a multiarchitecture package and release checksums. Never move release tags; publish a new version instead.

Renovate updates Go dependencies together to preserve Kubernetes client compatibility.
It also updates Actions, build tools, container digests and native test dependencies.
Dependency upgrades pass the same behavior checks as other changes.
