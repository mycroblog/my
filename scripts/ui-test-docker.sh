#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Runs the @my/ui story tests (`bun run test` in app/ui) inside the pinned Playwright image that CI uses,
# so screenshots match the committed baselines, which are generated only there (ADR-0110).
# The workspace's node_modules are container-only volumes, so the host's install is left alone, and the
# screenshots the run writes are handed back to the calling user.
# Usage: PLAYWRIGHT_IMAGE=<image@digest> scripts/ui-test-docker.sh [vitest args, e.g. --update]   (needs docker)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
image="${PLAYWRIGHT_IMAGE:?set PLAYWRIGHT_IMAGE (scripts/task.sh ui:test-docker does)}"
bun_version="$(awk '$1 == "bun" { print $2 }' "$root/.tool-versions")"

# shellcheck disable=SC2016 # expanded by the container's shell
script='
npm install --global --silent "bun@$BUN_VERSION"
cd app
bun install --frozen-lockfile
cd ui
status=0
bun run test "$@" || status=$?
if [[ -d src/__screenshots__ ]]; then chown -R "$HOST_UID:$HOST_GID" src/__screenshots__; fi
exit "$status"
'

# Docker creates missing mount points as root, and an empty root-owned node_modules makes the host's
# `bun install` fail, so they are created here as the calling user.
mkdir -p "$root/app/node_modules" "$root/app/ui/node_modules" "$root/app/web/node_modules"

exec docker run --rm --ipc=host \
	-v "$root:/work" -w /work \
	-v /work/app/node_modules -v /work/app/ui/node_modules -v /work/app/web/node_modules \
	-v my-ui-test-bun-cache:/root/.bun/install/cache \
	-e BUN_VERSION="$bun_version" -e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" -e CI \
	"$image" bash -euo pipefail -c "$script" ui-test-docker "$@"
