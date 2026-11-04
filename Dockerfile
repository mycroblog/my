# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
# SPDX-License-Identifier: AGPL-3.0-only
#
# Container images for cmd/my (ADR-0010): multi-arch (linux/amd64, linux/arm64), static, non-root, distroless.
#
#   docker build -t my .    # the server (default target "app"), which also applies the schema (my schema apply, ADR-0023)
#
# Base images are pinned by tag and multi-arch index digest, written in the FROM lines themselves so that Dependabot can
# update them. The Go, Node.js, and Bun tags must match .tool-versions (docker:dockerfile-check), so they are raised by
# hand; Dependabot raises the digests.

# The bun binary for the web stage, which runs on the build host's platform.
FROM --platform=$BUILDPLATFORM oven/bun:1.4.4-slim@sha256:1846adf22496bd711036aae8be147e57bdebbaea2e5264e3e0b5af1db187d9a4 AS bun

# The web client (app/web), built as `scripts/task.sh web:build` does: Bun installs the packages and runs the
# scripts, Vite runs on Node.js (ADR-0025). The output is the same for every platform, so it is built
# once on the build host's platform. The build reads generated files that are not committed (ADR-0081):
# the client pseudo-locales from tools/i18n and the theme token list of @my/ui are generated first, and
# the Vite plugin generates the route tree.
FROM --platform=$BUILDPLATFORM node:26.12.1-trixie-slim@sha256:94816674c62774da0d099a947bf4613b9e1972a2733abd0d0cd80e795f8411d0 AS web
# The SBOM of a released image covers this stage too: its npm packages reach the image only through the Go embed.
ARG BUILDKIT_SBOM_SCAN_STAGE=true
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
# No VERSION here: the server gives the client its version at run time (runtimeConfig in internal/interface/web), so
# that images of different commits with the same client have the same web assets and sw.js, and browsers see no update
# (scripts/task.sh docker:web-reproducible).
RUN cd tools/i18n && bun run src/gen.ts --ts \
    && cd ../../app/ui && bun run scripts/theme-tokens.ts --ts \
    && cd .. && bun run build

# The files of the web build alone, for docker:web-reproducible to compare; the image embeds them in the stage below.
FROM scratch AS web-dist
COPY --from=web /src/app/web/dist /

# Cross-compiles on the build host's platform; Go needs no emulation for the target architecture.
FROM --platform=$BUILDPLATFORM golang:1.27.3-trixie@sha256:c65ecfe613b539ac74c62a4c3b2c27e8f8a6076384c8f27e8a3c0f3be17cc385 AS build
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

FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329 AS app
COPY --from=build /out/my /usr/local/bin/my
COPY --from=build --chown=nonroot:nonroot /out/data /data
WORKDIR /data
ENV MY_LISTEN=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/my"]
CMD ["serve"]
