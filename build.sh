#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Sofie Finnes <sofie+git@finnes.dev>
# SPDX-License-Identifier: MPL-2.0
set -euo pipefail
package_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
output=${1:?Pass the output .xpkg path}
platform=${2:-linux/amd64}
image="provider-jellyfin:${platform##*/}-development"
docker buildx build --load --platform "$platform" --target runtime --tag "$image" "$package_dir"
crossplane xpkg build --package-root "$package_dir/package" \
    --embed-runtime-image "$image" --package-file "$output"
