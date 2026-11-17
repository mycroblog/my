#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/changed-web-packages.sh on made-up lists of changed paths: which web packages' tests each list selects,
# in the workspace as its manifests declare it. It reads no git history and changes no file of the repository.
#
#   scripts/changed-web-packages-test.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/changed-web-packages.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failed=0

# expect NAME WANT PATH... passes when the paths select exactly the packages WANT (space-separated, in any order).
expect() {
  local name="$1" want got
  want="$(printf '%s' "$2" | tr ' ' '\n' | sort | tr '\n' ' ')"
  want="${want% }"
  shift 2
  printf '%s\n' "$@" > "$tmp/files"
  got="$("$script" --files "$tmp/files" 2> "$tmp/stderr" | tr '\n' ' ')"
  got="${got% }"
  if [ "$got" != "$want" ]; then
    echo "FAIL: $name: want '$want', got '$got'" >&2
    failed=1
  fi
}

all="app app-components app-data app-features design-system design-tokens devtools e2e platform"
# The packages that test on the fake API, and the fake API's own, @my/e2e, which depends on the client library and the
# tokens alone: a change to it, or to what it depends on, reaches them all; a change to a layer reaches that layer and
# the layers above it.
fake="app app-components app-data app-features e2e"
# The design system and the layers above it.
parts="app app-components app-features design-system"

got="$("$script" --all | tr '\n' ' ')"
if [ "${got% }" != "$all" ]; then
  echo "FAIL: --all: want '$all', got '${got% }'" >&2
  failed=1
fi

# A package and the packages above it in the layers (ADR-0025 §3).
expect "the app, which nothing depends on" "app" web/app/src/main.tsx
expect "the tokens" "$fake design-system design-tokens" web/design-tokens/src/tokens.ts
expect "the design system" "$parts" web/design-system/src/button.tsx
expect "the shared components" "app app-components app-features" web/app-components/src/index.ts
expect "the platform" "app app-data app-features platform" web/platform/src/index.ts
expect "the server data" "app app-data app-features" web/app-data/src/index.ts
expect "the features" "app app-features" web/app-features/src/index.ts
expect "two packages add up" "app app-components app-data app-features design-system platform" web/design-system/src/button.tsx web/platform/src/index.ts

# What a test depends on outside the layers, as its package's manifest declares it.
expect "the fake API" "$fake" web/e2e/src/fake/api.ts
expect "the scenario the fake API reads" "$fake" web/e2e/src/scenario.ts
expect "the specs of the E2E tests, which are the fake API's package too" "$fake" web/e2e/specs/sign-in.spec.ts
expect "a DOM preload: the packages with tests on a DOM, what depends on them, and its own" "$parts devtools" web/devtools/src/happy-dom.ts
expect "the client library the server data is built on" "$fake" pkg/mykit/ts/src/client.ts
expect "the spec its types are generated from" "$fake" api/openapi.yaml
expect "a catalog of a package" "$parts" web/design-system/locales/en/disclosure.json
expect "the compiler of the catalogs: the packages that have some, and what depends on them" "$parts" web/i18n/src/compile.ts
expect "the tools the compiler is built on" "$parts" tools/i18n/src/check.ts

# What every package shares, and what scripts/changed-contexts.sh gives the web context outside the packages.
expect "the lockfile" "$all" bun.lock
expect "the workspace manifest" "$all" package.json
expect "the workspace's bunfig" "$all" bunfig.toml
expect "the base tsconfig" "$all" tsconfig.base.json
expect "the tasks of the workspace" "$all" tasks/web.yml
expect "the selection itself" "$all" scripts/changed-web-packages.sh
expect "the root Taskfile" "$all" Taskfile.yml
expect "the Go list a test of the tokens reads" "$all" internal/domain/instance/theme_tokens.gen.go
expect "the vectors a test of the features shares with the server" "$all" pkg/testkit/fixtures/text/tokens.json
expect "the brand assets, in no package" "$all" web/brand/icon.svg
if ! grep -q 'web/brand/icon.svg belongs to no package' "$tmp/stderr"; then
  echo "FAIL: the brand assets: no line on stderr says why every package is selected" >&2
  failed=1
fi

# What reaches no package's tests.
expect "the documents alone" "" docs/adr/0079-ci-single-gate-merge-queue.md README.md
expect "the server's code" "" internal/app/content/text.go
expect "the Storybook host, which nothing depends on" "" web/storybook/shot.ts
expect "the documentation site, which the site context tests" "" web/docs/src/foo.ts
expect "a package nothing under web/ depends on" "" pkg/mybot/ts/src/index.ts
expect "the map files" "" web/context-map.yaml web/app/context-map.yaml
expect "this test" "" scripts/changed-web-packages-test.sh

# Every directory under web/ that holds a test file is a package of --all, or one whose tests another context runs: a
# package cannot be added with tests that web:test does not run.
selected=" $("$script" --all | tr '\n' ' ')docs i18n "
for dir in "$root"/web/*/; do
  name="$(basename "$dir")"
  case "$selected" in *" $name "*) continue ;; esac
  if [ -n "$(find "$dir" -name node_modules -prune -o -name '*.test.*' -print -quit)" ]; then
    echo "FAIL: web/$name has tests that --all does not select" >&2
    failed=1
  fi
done

# A package added to the workspace is selected with no edit of the selection: a copy of the workspace's manifests with
# one more package, which depends on the design system.
mkdir -p "$tmp/ws/scripts"
cp "$script" "$root/scripts/changed-contexts.sh" "$tmp/ws/scripts/"
cp "$root/package.json" "$tmp/ws/"
mapfile -t patterns < <(jq -r '.workspaces[]' "$root/package.json")
for pattern in "${patterns[@]}"; do
  for f in "$root"/$pattern/package.json; do
    [ -f "$f" ] || continue
    dir="$(dirname "${f#"$root"/}")"
    mkdir -p "$tmp/ws/$dir"
    cp "$f" "$tmp/ws/$dir/"
    [ -d "$root/$dir/locales" ] && mkdir "$tmp/ws/$dir/locales"
  done
done
mkdir "$tmp/ws/web/added"
cat > "$tmp/ws/web/added/package.json" <<'EOF'
{ "name": "@my/added", "scripts": { "test": "bun test" }, "dependencies": { "@my/design-system": "workspace:*" } }
EOF
script="$tmp/ws/scripts/changed-web-packages.sh"
expect "an added package" "added" web/added/src/index.ts
expect "what an added package depends on" "added $parts" web/design-system/src/button.tsx
expect "what every package shares, with an added package" "added $all" bun.lock

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "changed-web-packages.sh: all cases passed"
