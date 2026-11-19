#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Sofie Finnes <sofie+git@finnes.dev>
# SPDX-License-Identifier: MPL-2.0
# Starts an empty Jellyfin named $1 in the media namespace and waits until it is healthy.
set -euo pipefail
name=${1:?Pass the fixture name}
image=${JELLYFIN_TEST_IMAGE:-$(cat "$(dirname -- "${BASH_SOURCE[0]}")/jellyfin-image.txt")}
kubectl apply --namespace media --filename - <<MANIFEST
apiVersion: v1
kind: Pod
metadata:
  name: $name
  labels:
    app: $name
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    runAsGroup: 1000
    fsGroup: 1000
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: jellyfin
      image: $image
      readinessProbe:
        httpGet:
          path: /health
          port: 8096
        periodSeconds: 2
        timeoutSeconds: 2
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop: [ALL]
      volumeMounts:
        - {name: config, mountPath: /config}
        - {name: cache, mountPath: /cache}
        - {name: media, mountPath: /media}
  volumes:
    - {name: config, emptyDir: {}}
    - {name: cache, emptyDir: {}}
    - {name: media, emptyDir: {}}
---
apiVersion: v1
kind: Service
metadata:
  name: $name
spec:
  selector:
    app: $name
  ports:
    - port: 8096
      targetPort: 8096
MANIFEST
kubectl wait --namespace media "pod/$name" --for=condition=Ready --timeout=180s
# Jellyfin can restart its listener during startup, so wait for a healthy response too.
for ((attempt = 0; attempt < 75; attempt++)); do
    if [[ $(kubectl exec --namespace media "$name" -- curl --silent http://127.0.0.1:8096/health) == Healthy ]]; then
        exit 0
    fi
    sleep 2
done
echo "Jellyfin fixture $name did not become healthy" >&2
exit 1
