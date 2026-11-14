# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
# SPDX-License-Identifier: AGPL-3.0-only
#
# Container images for cmd/my (ADR-0010): multi-arch (linux/amd64, linux/arm64), static, non-root, distroless.
#
#   docker build -t my .    # the server (default target "app"), which also applies the schema (my schema apply, ADR-0023)
#
# Base images are pinned by tag and multi-arch index digest, written in the FROM lines themselves so that Dependabot can
# update them. The Go and Bun tags must match .tool-versions (docker:dockerfile-check), so they are raised by
# hand; Dependabot raises the digests.

# The bun binary of the development image.
FROM --platform=$BUILDPLATFORM oven/bun:1.4.4-slim@sha256:1846adf22496bd711036aae8be147e57bdebbaea2e5264e3e0b5af1db187d9a4 AS bun

# The Go toolchain, of the build stage and the development image.
FROM --platform=$BUILDPLATFORM golang:1.27.3-trixie@sha256:af1bfc2614ebb1dbd31af514b78740fd678850e76498762b425ecee4c5c6ea80 AS go

# Cross-compiles on the build host's platform; Go needs no emulation for the target architecture.
FROM go AS build
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
# internal/interface/web embeds its dist/ (ADR-0028): the files of web/brand alone, without the client's Vite build that
# `scripts/task.sh web:build` adds, so the image's shell loads no client script yet.
RUN --mount=type=bind,target=. \
    --mount=type=bind,source=web/brand,target=internal/interface/web/dist \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags "${GO_TAGS}" -ldflags "-s -w -X main.version=${VERSION}" -o /out/my ./cmd/my \
    && scripts/check-release-build.sh /out/my \
    && mkdir -p /out/data

# The development image of the root compose.yaml (CONTRIBUTING.md): the Go toolchain with Bun, which runs `my` from the
# checkout that compose.yaml mounts at /src. It starts as root and scripts/dev-compose.sh runs the work as the owner of
# the checkout, so that what it writes there stays the developer's. The caches in HOME are named volumes.
FROM go AS dev
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
ENV GOTOOLCHAIN=local GOWORK=off CGO_ENABLED=0 HOME=/home/dev \
    GOCACHE=/home/dev/.cache/go-build GOMODCACHE=/home/dev/go/pkg/mod BUN_INSTALL_CACHE_DIR=/home/dev/.bun/install/cache \
    PATH=/home/dev/bin:$PATH
RUN mkdir -p /home/dev/.cache/go-build /home/dev/go/pkg/mod /home/dev/.bun/install/cache /home/dev/bin
WORKDIR /src

FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329 AS app
COPY --from=build /out/my /usr/local/bin/my
COPY --from=build --chown=nonroot:nonroot /out/data /data
WORKDIR /data
ENV MY_LISTEN=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/my"]
CMD ["serve"]
