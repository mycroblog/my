# syntax=docker/dockerfile:1
# SPDX-License-Identifier: AGPL-3.0-only
#
# Container images for cmd/my (ADR-0010): multi-arch (linux/amd64, linux/arm64), static, non-root, distroless.
#
#   docker build -t my .                       # the server (default target "app")
#   docker build -t my-schema --target schema . # Atlas Community Edition + schema/ (ADR-0023)
#
# Base images are pinned by tag and multi-arch index digest. The Go, Node.js, and Bun tags must match .tool-versions.

ARG GO_IMAGE=golang:1.27.3-trixie@sha256:604106102d33b1084e37ca08adf98ac0813624ae8595bd0d7f3965ecebded09c
ARG NODE_IMAGE=node:26.12.1-trixie-slim@sha256:1653c74b30c27b26f68c2d21279be12139d2ba723e3a682c425ec4bfb6070422
ARG BUN_IMAGE=oven/bun:1.4.4-slim@sha256:1846adf22496bd711036aae8be147e57bdebbaea2e5264e3e0b5af1db187d9a4
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

# The bun binary for the web stage, which runs on the build host's platform.
FROM --platform=$BUILDPLATFORM ${BUN_IMAGE} AS bun

# The web client (app/web), built as `scripts/task.sh web:build` does: Bun installs the packages and runs the
# scripts, Vite runs on Node.js (ADR-0025). The output is the same for every platform, so it is built
# once on the build host's platform. The build reads generated files that are not committed (ADR-0081):
# the client pseudo-locales from tools/i18n and the theme token list of @my/ui are generated first, and
# the Vite plugin generates the route tree.
FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS web
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
WORKDIR /src
COPY app/package.json app/bun.lock app/bunfig.toml app/
COPY app/ui/package.json app/ui/
COPY app/web/package.json app/web/
# @mycroblog/mykit, a workspace package outside app/ that app/web depends on (ADR-0093 §1).
COPY pkg/mykit/ts/package.json pkg/mykit/ts/
COPY tools/i18n/package.json tools/i18n/bun.lock tools/i18n/bunfig.toml tools/i18n/
RUN --mount=type=cache,target=/root/.bun/install/cache \
    cd app && bun install --frozen-lockfile \
    && cd ../tools/i18n && bun install --frozen-lockfile
COPY locales/ locales/
COPY tools/i18n/ tools/i18n/
COPY app/ app/
COPY pkg/mykit/ts/ pkg/mykit/ts/
# The same version as the server's, which the client sends with its error reports.
ARG VERSION=dev
RUN cd tools/i18n && bun run src/gen.ts --ts \
    && cd ../../app/ui && bun run scripts/theme-tokens.ts --ts \
    && cd .. && VITE_MY_RELEASE="$VERSION" bun run build

# Cross-compiles on the build host's platform; Go needs no emulation for the target architecture.
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build
ARG TARGETOS TARGETARCH
# Version stamped into `my version` and NodeInfo.
ARG VERSION=dev
# Comma-separated build tags that leave adapters out of the server (docs/reference/build-tags.md); empty builds every adapter.
ARG GO_TAGS=""
ENV GOTOOLCHAIN=local CGO_ENABLED=0
WORKDIR /src
# An image is never a test build: the test-only tag e2e_fakes adds a payment provider that records payments as paid
# without any money moving, so the build stops here when GO_TAGS has it, and the built binary is checked again below.
RUN --mount=type=bind,source=scripts/check-release-build.sh,target=/usr/local/bin/check-release-build.sh \
    check-release-build.sh tags "${GO_TAGS}"
RUN --mount=type=bind,source=go.mod,target=go.mod \
    --mount=type=bind,source=go.sum,target=go.sum \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download
# internal/interface/web embeds the web build from its dist/ (ADR-0028).
RUN --mount=type=bind,target=. \
    --mount=type=bind,from=web,source=/src/app/web/dist,target=internal/interface/web/dist \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags "${GO_TAGS}" -ldflags "-s -w -X main.version=${VERSION}" -o /out/my ./cmd/my \
    && scripts/check-release-build.sh /out/my \
    && mkdir -p /out/data

# Atlas Community Edition for the target platform, at the version and checksum pinned in scripts/install-atlas.sh.
FROM ${GO_IMAGE} AS atlas
RUN --mount=type=bind,source=scripts/install-atlas.sh,target=/install-atlas.sh \
    /install-atlas.sh /out/atlas

# One-shot schema apply: `atlas schema apply` with the directory schema/<dialect> under /schema (ADR-0023).
FROM ${RUNTIME_IMAGE} AS schema
COPY --from=atlas /out/atlas /usr/local/bin/atlas
COPY schema/*.sql /schema/
COPY schema/sqlite /schema/sqlite/
COPY schema/postgres /schema/postgres/
# Owned by the non-root user so that a named volume mounted here starts out writable.
COPY --from=build --chown=nonroot:nonroot /out/data /data
WORKDIR /data
ENTRYPOINT ["/usr/local/bin/atlas"]
CMD ["version"]

FROM ${RUNTIME_IMAGE} AS app
COPY --from=build /out/my /usr/local/bin/my
COPY --from=build --chown=nonroot:nonroot /out/data /data
WORKDIR /data
ENV MY_LISTEN=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/my"]
CMD ["serve"]
