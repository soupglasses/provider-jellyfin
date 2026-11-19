# shellcheck shell=bash
# SPDX-FileCopyrightText: 2026 Sofie Finnes <sofie+git@finnes.dev>
# SPDX-License-Identifier: MPL-2.0
# Helpers for the Chainsaw scripts. Jellyfin is reached from inside its own Pod, so no
# port-forward has to survive the restarts the plugin tests cause.
set -euo pipefail
fixture=jellyfin-fixture
admin='{"Username":"fixture-admin","Pw":"disposable-kind-test-password"}'
client='MediaBrowser Client="KindTest", Device="Fixture", DeviceId="kind-test", Version="1"'

# api METHOD PATH [BODY]: call the fixture API, authenticated once the administrator exists.
api() {
    local auth=$client token
    if [[ ${JELLYFIN_ANONYMOUS:-} != true ]]; then
        token=$(JELLYFIN_ANONYMOUS=true api POST /Users/AuthenticateByName "$admin" | jq -r .AccessToken)
        auth+=", Token=\"$token\""
    fi
    local args=(--silent --show-error --fail --request "$1" --header 'Content-Type: application/json'
        --header "Authorization: $auth")
    if (($# > 2)); then
        args+=(--data "$3")
    fi
    kubectl exec --namespace media "$fixture" -- curl "${args[@]}" "http://127.0.0.1:8096$2"
}

# eventually DESCRIPTION COMMAND...: retry COMMAND for up to 150 seconds.
eventually() {
    local description=$1 deadline=$((SECONDS + 150))
    shift
    until "$@" 2>/dev/null; do
        if ((SECONDS >= deadline)); then
            echo "timed out: $description" >&2
            return 1
        fi
        sleep 2
    done
}

# check PATH FILTER [jq arguments...]: the GET response satisfies the jq FILTER.
check() {
    local path=$1 filter=$2
    shift 2
    api GET "$path" | jq --exit-status "$@" "$filter" >/dev/null
}

# expect DESCRIPTION PATH FILTER [jq arguments...]: check eventually succeeds.
expect() {
    local description=$1
    shift
    eventually "$description" check "$@"
}

# remember KEY VALUE and recall KEY keep baselines between steps.
remember() {
    kubectl create configmap e2e-state --namespace media --dry-run=client --output yaml |
        kubectl apply --filename - >/dev/null
    kubectl patch configmap e2e-state --namespace media --type merge \
        --patch "$(jq --null-input --arg key "$1" --arg value "$2" '{data: {($key): $value}}')" >/dev/null
}
recall() {
    kubectl get configmap e2e-state --namespace media --output json | jq --raw-output --arg key "$1" '.data[$key]'
}

# reconciled KIND/NAME: Ready, and Synced for the current generation.
reconciled() {
    kubectl get --namespace media "$1" --output json | jq --exit-status '
        .metadata.generation as $generation
        | [.status.conditions[]? | select(.status == "True")]
        | any(.type == "Ready") and any(.type == "Synced" and .observedGeneration == $generation)' >/dev/null
}
