# SPDX-FileCopyrightText: 2026 Sofie Finnes <sofie+git@finnes.dev>
# SPDX-License-Identifier: MPL-2.0
"""Exercise the installed controller against a disposable Jellyfin in kind."""

import json
import os
from pathlib import Path
import socket
import subprocess
import time
import urllib.error
import urllib.request


def kube(*args, value=None):
    return subprocess.check_output(
        ["kubectl", "-n", "media", *args],
        input=None if value is None else json.dumps(value).encode(),
    )


def apply(value):
    kube("apply", "-f", "-", value=value)


def eventually(check, description):
    deadline = time.monotonic() + 150
    while time.monotonic() < deadline:
        try:
            if check():
                return
        except (urllib.error.URLError, ConnectionError):
            pass
        time.sleep(2)
    raise AssertionError(description)


apply(
    {
        "apiVersion": "v1",
        "kind": "Pod",
        "metadata": {"name": "jellyfin-fixture", "labels": {"app": "jellyfin-fixture"}},
        "spec": {
            "securityContext": {
                "runAsNonRoot": True,
                "runAsUser": 1000,
                "runAsGroup": 1000,
                "fsGroup": 1000,
                "seccompProfile": {"type": "RuntimeDefault"},
            },
            "containers": [
                {
                    "name": "jellyfin",
                    "image": os.environ.get("JELLYFIN_TEST_IMAGE") or (
                        Path(__file__).with_name("jellyfin-image.txt").read_text().strip()
                    ),
                    "readinessProbe": {
                        "httpGet": {"path": "/health", "port": 8096},
                        "periodSeconds": 2,
                        "timeoutSeconds": 2,
                    },
                    "securityContext": {
                        "allowPrivilegeEscalation": False,
                        "capabilities": {"drop": ["ALL"]},
                    },
                    "volumeMounts": [
                        {"name": name, "mountPath": "/" + name}
                        for name in ("config", "cache", "media")
                    ],
                }
            ],
            "volumes": [{"name": name, "emptyDir": {}} for name in ("config", "cache", "media")],
        },
    }
)
apply(
    {
        "apiVersion": "v1",
        "kind": "Service",
        "metadata": {"name": "jellyfin-fixture"},
        "spec": {
            "selector": {"app": "jellyfin-fixture"},
            "ports": [{"port": 8096, "targetPort": 8096}],
        },
    }
)
kube("wait", "pod/jellyfin-fixture", "--for=condition=Ready", "--timeout=180s")
with socket.socket() as listener:
    listener.bind(("127.0.0.1", 0))
    port = listener.getsockname()[1]


def start_forward():
    return subprocess.Popen(
        [
            "kubectl",
            "-n",
            "media",
            "port-forward",
            "pod/jellyfin-fixture",
            f"{port}:8096",
        ],
        stdout=subprocess.DEVNULL,
    )


forward = start_forward()
token = None


def api(path, value=None):
    headers = {
        "Content-Type": "application/json",
        "Authorization": 'MediaBrowser Client="KindTest", Device="Fixture", DeviceId="kind-test", Version="1"',
    }
    if token:
        headers["Authorization"] += ", Token=" + json.dumps(token)
    request = urllib.request.Request(
        f"http://127.0.0.1:{port}{path}",
        data=None if value is None else json.dumps(value).encode(),
        headers=headers,
    )
    with urllib.request.urlopen(request, timeout=10) as response:
        raw = response.read()
        return (
            json.loads(raw)
            if raw and response.headers.get_content_type() == "application/json"
            else raw
        )


def fixture_ready():
    global forward
    # Jellyfin can restart its listener during startup, which terminates port-forward.
    if forward.poll() is not None:
        forward = start_forward()
    return api("/health") == b"Healthy"


try:
    eventually(fixture_ready, "Jellyfin did not start")
    password = "disposable-kind-test-password"
    initial = api("/System/Info/Public")
    assert initial["StartupWizardCompleted"] is False, "fixture was already initialized"
    apply(
        {
            "apiVersion": "v1",
            "kind": "Secret",
            "metadata": {"name": "kind-credentials"},
            "stringData": {"password": password},
        }
    )
    apply(
        {
            "apiVersion": "jellyfin.finnes.dev/v1beta1",
            "kind": "ProviderConfig",
            "metadata": {"name": "kind-fixture"},
            "spec": {
                "endpoint": "http://jellyfin-fixture.media.svc.cluster.local:8096",
                "username": "fixture-admin",
                "passwordSecretRef": {"name": "kind-credentials", "key": "password"},
            },
        }
    )
    resource = {
        "apiVersion": "jellyfin.finnes.dev/v1alpha1",
        "kind": "SystemConfiguration",
        "metadata": {
            "name": "kind-fixture",
            "annotations": {"crossplane.io/external-name": "system"},
        },
        "spec": {
            "managementPolicies": ["Observe", "Create", "Update"],
            "providerConfigRef": {"name": "kind-fixture", "kind": "ProviderConfig"},
            "forProvider": {"serverName": "Fresh Crossplane instance"},
        },
    }
    apply(resource)
    kube(
        "wait",
        "systemconfiguration/kind-fixture",
        "--for=condition=Synced",
        "--timeout=120s",
    )
    assert api("/System/Info/Public")["StartupWizardCompleted"] is True
    token = api(
        "/Users/AuthenticateByName", {"Username": "fixture-admin", "Pw": password}
    )["AccessToken"]
    users = api("/Users")
    assert len(users) == 1 and users[0]["Policy"]["IsAdministrator"], (
        "provider did not create exactly one first administrator"
    )
    assert api("/System/Info/Public")["Id"] == initial["Id"], "server was replaced"
    baseline = api("/System/Configuration")
    assert baseline["ServerName"] == "Fresh Crossplane instance"
    resource["spec"]["managementPolicies"] = ["Observe"]
    resource["spec"]["forProvider"]["serverName"] = "Managed by Crossplane"
    apply(resource)
    # Wait for a new reconciliation rather than accepting the previous Synced condition.
    generation = json.loads(
        kube("get", "systemconfiguration/kind-fixture", "-o", "json")
    )["metadata"]["generation"]
    eventually(
        lambda: any(
            condition["type"] == "Synced"
            and condition["status"] == "True"
            and condition.get("observedGeneration") == generation
            for condition in json.loads(
                kube("get", "systemconfiguration/kind-fixture", "-o", "json")
            )["status"]["conditions"]
        ),
        "Observe did not reconcile the new generation",
    )
    assert api("/System/Configuration")["ServerName"] == baseline["ServerName"], (
        "Observe wrote settings"
    )
    resource["spec"]["managementPolicies"] = ["Observe", "Create", "Update"]
    apply(resource)
    eventually(
        lambda: api("/System/Configuration")["ServerName"] == "Managed by Crossplane",
        "controller did not apply desired settings",
    )
    drift = api("/System/Configuration")
    drift["ServerName"] = "External drift"
    api("/System/Configuration", drift)
    eventually(
        lambda: api("/System/Configuration")["ServerName"] == "Managed by Crossplane",
        "controller did not correct API drift",
    )
    kube(
        "wait",
        "systemconfiguration/kind-fixture",
        "--for=condition=Synced",
        "--timeout=120s",
    )
    for field in ("UICulture", "MetadataCountryCode", "PreferredMetadataLanguage"):
        assert api("/System/Configuration")[field] == baseline[field], field
    plugin = next(item for item in api("/Plugins") if item["Name"] == "TMDb")
    plugin_path = f"/Plugins/{plugin['Id']}/Configuration"
    plugin_baseline = api(plugin_path)
    apply(
        {
            "apiVersion": "v1",
            "kind": "Secret",
            "metadata": {"name": "kind-plugin-settings"},
            "stringData": {"configuration.json": "{}"},
        }
    )
    apply(
        {
            "apiVersion": "jellyfin.finnes.dev/v1alpha1",
            "kind": "PluginConfiguration",
            "metadata": {
                "name": "kind-plugin",
                "annotations": {"crossplane.io/external-name": plugin["Id"]},
            },
            "spec": {
                "managementPolicies": ["Observe"],
                "providerConfigRef": {"name": "kind-fixture", "kind": "ProviderConfig"},
                "forProvider": {
                    "configurationJsonSecretRef": {
                        "name": "kind-plugin-settings",
                        "key": "configuration.json",
                    }
                },
            },
        }
    )
    for condition in ("Ready", "Synced"):
        kube(
            "wait",
            "pluginconfiguration/kind-plugin",
            f"--for=condition={condition}",
            "--timeout=120s",
        )
    assert api(plugin_path) == plugin_baseline, "Observe changed plugin configuration"
    def managed(kind, name, external, desired, policies=None):
        value = {
            "apiVersion": "jellyfin.finnes.dev/v1alpha1",
            "kind": kind,
            "metadata": {
                "name": name,
                "annotations": {"crossplane.io/external-name": external},
            },
            "spec": {
                "managementPolicies": policies or ["Observe", "Create", "Update"],
                "providerConfigRef": {"name": "kind-fixture", "kind": "ProviderConfig"},
                "forProvider": desired,
            },
        }
        apply(value)
        return value

    def reconciled(kind, name):
        current = json.loads(kube("get", kind + "/" + name, "-o", "json"))
        return all(any(
            condition["type"] == expected
            and condition["status"] == "True"
            and (expected != "Synced" or condition.get("observedGeneration") == current["metadata"]["generation"])
            for condition in current.get("status", {}).get("conditions", [])
        ) for expected in ("Ready", "Synced"))

    branding_path = "/System/Configuration/branding"
    network_path = "/System/Configuration/network"
    branding_baseline = api(branding_path)
    network_baseline = api(network_path)
    repos_baseline = api("/Repositories")
    repo_url = "http://jellyfin-fixture.media.svc.cluster.local:8096/disposable-catalog.json"
    task = next(task for task in api("/ScheduledTasks") if not task["IsHidden"])
    task_path = "/ScheduledTasks/" + task["Id"]
    resources = [
        managed("BrandingConfiguration", "branding", "branding", {"loginDisclaimer": "Managed disclaimer"}),
        managed("NetworkingConfiguration", "network", "networking", {
            "enablePublishedServerUriByRequest": not network_baseline["EnablePublishedServerUriByRequest"],
        }),
        managed("PluginRepository", "repository", repo_url, {"name": "Disposable repository", "enabled": False}, ["*"]),
        managed("Library", "library", "Disposable library", {
            "collectionType": "movies", "paths": ["/media"],
            "libraryOptions": {"enableRealtimeMonitor": False},
        }),
        managed("ScheduledTask", "task", task["Key"], {"triggers": [{
            "type": "IntervalTrigger", "intervalTicks": 864000000000,
        }]}),
        managed("Plugin", "builtin-plugin", plugin["Name"], {
            "repositoryUrl": repos_baseline[0]["Url"],
        }, ["Observe"]),
    ]
    for item in resources:
        eventually(lambda item=item: reconciled(item["kind"], item["metadata"]["name"]),
                   "resource did not reconcile: " + item["kind"])
        print("Verified " + item["kind"], flush=True)
    assert api(branding_path)["LoginDisclaimer"] == "Managed disclaimer"
    assert api(network_path)["EnablePublishedServerUriByRequest"] != network_baseline["EnablePublishedServerUriByRequest"]
    assert any(repo["Url"] == repo_url and not repo["Enabled"] for repo in api("/Repositories"))
    library = next(item for item in api("/Library/VirtualFolders") if item["Name"] == "Disposable library")
    assert library["Locations"] == ["/media"]
    assert library["LibraryOptions"]["EnableRealtimeMonitor"] is False
    assert api(task_path)["Triggers"][0]["IntervalTicks"] == 864000000000
    for field, value in branding_baseline.items():
        if field != "LoginDisclaimer":
            assert api(branding_path).get(field) == value, "unmanaged branding changed: " + field

    # Exercise Secret-backed writes using a built-in plugin, without an application-specific plugin.
    setting = next(key for key, value in plugin_baseline.items() if isinstance(value, bool))
    plugin_resource = json.loads(kube("get", "pluginconfiguration/kind-plugin", "-o", "json"))
    plugin_resource["spec"]["managementPolicies"] = ["Observe", "Create", "Update"]
    apply(plugin_resource)
    for desired in (not plugin_baseline[setting], plugin_baseline[setting]):
        apply({"apiVersion": "v1", "kind": "Secret", "metadata": {"name": "kind-plugin-settings"},
               "stringData": {"configuration.json": json.dumps({setting: desired})}})
        eventually(lambda: api(plugin_path)[setting] == desired, "Secret rotation did not update plugin settings")
    for key, value in plugin_baseline.items():
        if key != setting:
            assert api(plugin_path)[key] == value, "unmanaged plugin configuration changed: " + key

    changed_branding = api(branding_path)
    changed_branding["LoginDisclaimer"] = "External drift"
    api(branding_path, changed_branding)
    eventually(lambda: api(branding_path)["LoginDisclaimer"] == "Managed disclaimer",
               "branding drift was not corrected")
    resources[0]["spec"]["forProvider"]["loginDisclaimer"] = "Updated disclaimer"
    resources[2]["spec"]["forProvider"]["name"] = "Renamed repository"
    for item in (resources[0], resources[2]):
        apply(item)
    eventually(lambda: api(branding_path)["LoginDisclaimer"] == "Updated disclaimer", "branding update failed")
    eventually(lambda: any(repo["Url"] == repo_url and repo["Name"] == "Renamed repository" for repo in api("/Repositories")),
               "repository update failed")

    revision = subprocess.check_output([
        "kubectl", "get", "provider", "jellyfin", "-o", "jsonpath={.status.currentRevision}",
    ], text=True).strip()
    assert revision, "provider has no active revision"
    for operation in ("restart", "status"):
        subprocess.run(["kubectl", "-n", "crossplane-system", "rollout", operation,
                        "deployment/" + revision, *(["--timeout=90s"] if operation == "status" else [])], check=True)
    for item in resources:
        eventually(lambda item=item: reconciled(item["kind"], item["metadata"]["name"]), "resource failed after controller restart")
    assert next(item for item in api("/Library/VirtualFolders") if item["Name"] == "Disposable library")["ItemId"] == library["ItemId"]

    # Removing a CR without Delete must preserve the external library; adoption keeps its identity.
    kube("delete", "library/library", "--wait=true", "--timeout=90s")
    assert next(item for item in api("/Library/VirtualFolders") if item["Name"] == "Disposable library")["ItemId"] == library["ItemId"]
    resources[3]["spec"]["managementPolicies"] = ["*"]
    apply(resources[3])
    eventually(lambda: reconciled("Library", "library"), "library adoption failed")
    assert next(item for item in api("/Library/VirtualFolders") if item["Name"] == "Disposable library")["ItemId"] == library["ItemId"]
    kube("delete", "library/library", "pluginrepository/repository", "--wait=true", "--timeout=90s")
    eventually(lambda: not any(item["Name"] == "Disposable library" for item in api("/Library/VirtualFolders")), "Delete did not remove library")
    assert api("/Repositories") == repos_baseline, "repository deletion changed unrelated repositories"
    assert [(u["Id"], u["Name"]) for u in api("/Users")] == [(u["Id"], u["Name"]) for u in users]
    print("Native package reconciled all resource kinds, rotated Secret-backed settings, corrected drift, survived controller restart, and enforced orphan/adopt/delete policies.")

finally:
    forward.terminate()
    forward.wait(timeout=10)
