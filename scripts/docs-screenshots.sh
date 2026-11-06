#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Takes the screenshots of the documentation site and the README (app/web/scripts/screenshots.ts) inside the pinned
# Playwright image that CI uses, so that every run draws with the same Chromium and fonts and only a change of a
# screen changes the images. It reads the static Storybook of app/web/storybook-static, which
# `scripts/task.sh docs:screenshots` builds first, and writes app/docs/src/assets/screenshots/*.webp as the calling user.
# The workspace's node_modules are container-only volumes, so the host's install is left alone.
# Usage: PLAYWRIGHT_IMAGE=<image@digest> scripts/docs-screenshots.sh [<name> ...]   (needs docker)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
image="${PLAYWRIGHT_IMAGE:?set PLAYWRIGHT_IMAGE (scripts/task.sh docs:screenshots does)}"
bun_version="$(awk '$1 == "bun" { print $2 }' "$root/.tool-versions")"

# shellcheck disable=SC2016 # expanded by the container's shell
script='
npm install --global --silent "bun@$BUN_VERSION"
cd app
bun install --frozen-lockfile
cd web
status=0
bun scripts/screenshots.ts "$@" || status=$?
chown -R "$HOST_UID:$HOST_GID" ../docs/src/assets/screenshots
exit "$status"
'

# Every workspace's node_modules is a container-only volume. Docker creates missing mount points as root, and an
# empty root-owned node_modules makes the host's `bun install` fail, so they are created here as the calling user.
workspaces=(app app/ui app/web app/mail app/docs pkg/mykit/ts pkg/mybot/ts)
volumes=()
for workspace in "${workspaces[@]}"; do
	mkdir -p "$root/$workspace/node_modules"
	volumes+=(-v "/work/$workspace/node_modules")
done
mkdir -p "$root/app/docs/src/assets/screenshots"

exec docker run --rm --ipc=host \
	-v "$root:/work" -w /work \
	"${volumes[@]}" \
	-v my-ui-test-bun-cache:/root/.bun/install/cache \
	-e BUN_VERSION="$bun_version" -e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" \
	"$image" bash -euo pipefail -c "$script" docs-screenshots "$@"
