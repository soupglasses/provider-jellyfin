#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Sofie Finnes <sofie+git@finnes.dev>
# SPDX-License-Identifier: MPL-2.0
set -euo pipefail
package_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
output=${1:?Pass the output .xpkg path}
platform=${2:-linux/amd64}
image="provider-jellyfin:${platform##*/}-development"
work_dir=$(mktemp -d)
trap 'rm -rf -- "$work_dir"' EXIT
docker buildx build --load --platform "$platform" --target runtime --tag "$image" "$package_dir"
docker buildx build --platform "$platform" --target generated --output "type=local,dest=$work_dir" "$package_dir"
cp -- "$package_dir/package/crossplane.yaml" "$work_dir/package/crossplane.yaml"
crossplane xpkg build --package-root "$work_dir/package" \
    --embed-runtime-image "$image" --package-file "$output"
