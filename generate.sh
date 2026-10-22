#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Sofie Finnes <sofie+git@finnes.dev>
# SPDX-License-Identifier: MPL-2.0
set -euo pipefail
cd "$(dirname "$0")"
go run -buildvcs=false ./cmd/generate
# Upjet requires both scopes during generation; only namespaced APIs are shipped.
rm -rf apis/cluster internal/controller/cluster
controller-gen object:headerFile=hack/boilerplate.go.txt paths=./apis/...
angryjet generate-methodsets --header-file=hack/boilerplate.go.txt ./apis/...
# Retired resources must not leave installable CRDs behind.
rm -f package/crds/*.yaml package/crds/*.yaml.license
controller-gen crd:allowDangerousTypes=true paths=./apis/namespaced/... output:crd:dir=package/crds
for manifest in package/crds/*.yaml; do
    yamlfmt "$manifest"
    cat > "$manifest.license" <<'LICENSE'
SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
SPDX-License-Identifier: Apache-2.0
LICENSE
done
