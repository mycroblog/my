# SPDX-License-Identifier: AGPL-3.0-only
#
# The image the browser tests of CI run in (the jobs web-stories-test and web-e2e of ci.yml, web-stories-scan and
# web-e2e of slow-suite.yml): Ubuntu with Chromium alone, its system packages and fonts as Playwright installs them
# (`playwright install --with-deps chromium`: the fonts of Playwright's own image, which the layout tests measure text
# with), and what the jobs add: Bun, jq, git for the checkout, openssl for the development certificates of the E2E
# tests, and zstd, with which the cache of bin/task is packed. Playwright's own image also holds Firefox and WebKit,
# which no test opens: it is nearly twice the size, and a runner pulls it in anything from 20 seconds to four minutes.
#
# scripts/browser-image.sh names the image after the Playwright of web/e2e/package.json, the Bun of .tool-versions and
# this file, and builds it when the registry has none of that name: a change of one of the three makes a new image.
ARG PLAYWRIGHT_VERSION
ARG BUN_VERSION

FROM oven/bun:${BUN_VERSION}-slim AS bun

FROM ubuntu:noble
ARG PLAYWRIGHT_VERSION
ARG DEBIAN_FRONTEND=noninteractive
ENV LANG=C.UTF-8 LC_ALL=C.UTF-8 PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
# Every tool of the workspace runs on Bun (ADR-0025 §10).
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
# The browser's system packages and fonts, then the browser where the jobs' user (the runner's, 1001) reads it: two
# layers of about the same size, which a pull fetches side by side. Nothing of the installer is left behind.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git jq openssl zstd \
    && bun x "playwright-core@${PLAYWRIGHT_VERSION}" install-deps chromium \
    && rm -rf /root/.bun /root/.cache /tmp/* /var/lib/apt/lists/*
RUN bun x "playwright-core@${PLAYWRIGHT_VERSION}" install chromium \
    && chmod -R a+rX /ms-playwright \
    && rm -rf /root/.bun /root/.cache /tmp/*
