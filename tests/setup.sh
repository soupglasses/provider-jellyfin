#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Sofie Finnes <sofie+git@finnes.dev>
# SPDX-License-Identifier: MPL-2.0
set -euo pipefail
repo_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
if [[ -n "${JELLYFIN_TEST_IMAGE:-}" ]]; then
    kind load docker-image --name "${KIND_CLUSTER_NAME:?}" "$JELLYFIN_TEST_IMAGE"
fi
kubectl create namespace media
kubectl label namespace media pod-security.kubernetes.io/enforce=restricted

# Install the package without contacting the production registry.
yq '(select(.kind == "Provider") | .spec.package) = strenv(PROVIDER_PACKAGE_REF)' "$repo_dir/tests/provider.yaml" |
    kubectl apply --filename -
kubectl wait provider/jellyfin --for=condition=Healthy --timeout=180s
kubectl rollout status --namespace crossplane-system \
    deployment/"$(kubectl get provider jellyfin --output jsonpath='{.status.currentRevision}')" --timeout=90s
if [[ -n "${TEST_ARTIFACTS_DIR:-}" ]]; then
    mkdir -p -- "$TEST_ARTIFACTS_DIR"
    kubectl get clusterrole,clusterrolebinding --output yaml >"$TEST_ARTIFACTS_DIR/rbac.yaml"
fi

# The examples configure their own empty server through the ProviderConfig they document.
"$repo_dir/tests/fixture.sh" jellyfin
kubectl exec --namespace media jellyfin -- mkdir -p /media/movies
kubectl apply --filename "$repo_dir/examples/providerconfig.yaml"
