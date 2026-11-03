#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Fails when the web build of the Dockerfile differs between two versions of the server. The version is the only build
# argument that changes per commit: when it reaches the bundle, every hashed asset and sw.js change with it, and every
# browser offers an update for an image with no change to the client.
#
# The first build may reuse the layers of an earlier build, such as the smoke test's. The second one rebuilds the web
# stage, so the check also fails on anything else that varies per build, such as a date in a generated file.
# Usage: scripts/docker-web-reproducible.sh   (needs docker with BuildKit)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
out="$(mktemp -d)"
trap 'rm -rf "$out"' EXIT

docker build --quiet --target web-dist --build-arg VERSION=reproducible-a --output "type=local,dest=$out/a" "$root"
docker build --quiet --target web-dist --build-arg VERSION=reproducible-b --no-cache-filter web --output "type=local,dest=$out/b" "$root"

# A build that wrote nothing would compare equal.
if [[ ! -s "$out/a/sw.js" ]]; then
	echo "docker-web-reproducible: the build has no sw.js" >&2
	exit 1
fi
if ! diff -rq "$out/a" "$out/b" >"$out/diff"; then
	echo "docker-web-reproducible: the web build differs between two builds with different versions ($(wc -l <"$out/diff") differences):" >&2
	sed -e "s|$out/||g" "$out/diff" | head -20 >&2
	exit 1
fi
echo "docker-web-reproducible: $(find "$out/a" -type f | wc -l) files are identical"
