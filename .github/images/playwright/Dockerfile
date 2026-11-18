# SPDX-License-Identifier: AGPL-3.0-only
#
# The image the browser tests of CI run in (the jobs web-stories-test and web-e2e of ci.yml, web-stories-scan and
# web-e2e of slow-suite.yml): Playwright's own image, which holds the browsers, their system packages and fonts, with
# what the jobs add to it, Bun and jq. Without it each job installs the browser and its packages with apt on the
# runner, which took from 15 seconds to over six minutes with the state of the package mirrors.
#
# scripts/browser-image.sh names the image after the Playwright of web/e2e/package.json, the Bun of .tool-versions and
# this file, and builds it when the registry has none of that name: a change of one of the three makes a new image.
ARG PLAYWRIGHT_VERSION
ARG BUN_VERSION

FROM oven/bun:${BUN_VERSION}-slim AS bun

FROM mcr.microsoft.com/playwright:v${PLAYWRIGHT_VERSION}-noble
# Every tool of the workspace runs on Bun (ADR-0025 §10). The setup action cannot install it here: it unpacks an
# archive with unzip, which the image lacks.
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
# jq: the slow suite lists the spec files of a shard with it. The image already has openssl, for the development
# certificates of the E2E tests, and zstd, with which the cache of bin/task is packed.
RUN apt-get update \
    && apt-get install -y --no-install-recommends jq \
    && rm -rf /var/lib/apt/lists/*
