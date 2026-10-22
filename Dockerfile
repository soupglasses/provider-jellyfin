# SPDX-FileCopyrightText: 2026 Sofie Finnes <sofie+git@finnes.dev>
# SPDX-License-Identifier: MPL-2.0
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine3.23@sha256:754d5200fc3edae76d3b6598a9a3fac7e74981ee6a5413ab955025f185753ef2 AS generate
WORKDIR /src
RUN apk add --no-cache bash && \
    go install golang.org/x/tools/cmd/goimports@v0.52.0 && \
    go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.20.1 && \
    go install github.com/crossplane/crossplane-tools/cmd/angryjet@v0.0.0-20260719180100-659f1dc036c5 && \
    go install github.com/google/yamlfmt/cmd/yamlfmt@v0.21.0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build bash generate.sh && \
    cp "$(go list -m -f '{{.Dir}}' github.com/ThePhaseless/terraform-provider-jellyfin)/LICENSE" /tmp/terraform-provider-LICENSE

FROM generate AS unit-tests
RUN --mount=type=cache,target=/root/.cache/go-build go test -buildvcs=false -p=2 -coverprofile=/tmp/coverage.out . ./cmd/provider

FROM unit-tests AS build
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -buildvcs=false -p=2 -o /out/provider ./cmd/provider

FROM scratch AS generated
COPY --from=generate /src/generated/crds /generated/crds
COPY --from=generate /src/config/schema.json /generated/schema.json
COPY --from=generate /src/apis /apis
COPY --from=generate /src/internal/controller /internal/controller

FROM scratch AS coverage
COPY --from=unit-tests /tmp/coverage.out /coverage.out

FROM --platform=$BUILDPLATFORM alpine:3.24@sha256:71e9b7bae160ba167fa17414401b94cbcdaf3c32feeab9298bd794a2a0614c35 AS certificates
RUN apk add --no-cache ca-certificates

FROM scratch AS runtime
LABEL org.opencontainers.image.source="https://github.com/soupglasses/provider-jellyfin"
COPY --from=certificates /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/provider /provider
COPY LICENSE /licenses/provider-jellyfin-MPL
COPY --from=generate /tmp/terraform-provider-LICENSE /licenses/terraform-provider-jellyfin-MPL
USER 65532:65532
ENTRYPOINT ["/provider"]
