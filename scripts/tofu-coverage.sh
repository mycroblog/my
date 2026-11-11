#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Fails unless each public module of deploy/opentofu (mycroblog-*) says how it handles every configuration key. Its
# coverage.json puts each key of modules/internal/settings/settings.json, alone or by a prefix ending in *, in one class:
#
#   "section":  { "<variable>": [keys] }  the section, a variable of the module, that sets it;
#   "fixed":    [keys]                    a value the module sets itself;
#   "generic":  [keys]                    through the settings and secrets inputs;
#   "excluded": { "<key>": "<reason>" }   not handled, for the reason given.
#
# It fails on a key with no class, a key that two entries match, an entry that matches no key, a section that is no variable
# of the module, and a section or fixed key whose MY_ name tests/coverage.tftest.hcl does not name: that file holds the
# wiring tests, which plan each kind on mocked providers and assert that the service's environment has those names.
#
# Usage: scripts/tofu-coverage.sh
#
# TOFU_COVERAGE_ROOT overrides the repository root, for the tests of this script.
set -euo pipefail

root="${TOFU_COVERAGE_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
settings="$root/deploy/opentofu/modules/internal/settings/settings.json"

status=0
found=0
for module in "$root"/deploy/opentofu/mycroblog-*/; do
  module="${module%/}"
  [ -d "$module" ] || continue
  found=1
  name="${module#"$root"/}"
  coverage="$module/coverage.json"
  tests="$module/tests/coverage.tftest.hcl"
  if [ ! -f "$coverage" ]; then
    echo "tofu-coverage: $name has no coverage.json, which classifies every configuration key" >&2
    status=1
    continue
  fi
  wired=""
  [ -f "$tests" ] && wired="$(grep -oE '"MY_[A-Z0-9_]+"' "$tests" | tr -d '"' | LC_ALL=C sort -u)"
  variables="$(cat "$module"/*.tf | sed -nE 's/^variable "([a-z0-9_]+)".*/\1/p' | LC_ALL=C sort -u)"

  if ! errors="$(jq -r -n \
    --slurpfile settings "$settings" \
    --slurpfile coverage "$coverage" \
    --arg wired "$wired" \
    --arg variables "$variables" '
    $settings[0] as $all | $coverage[0] as $c |
    ($wired | split("\n")) as $wired |
    ($variables | split("\n")) as $variables |
    def matches($entry; $key):
      if ($entry | endswith("*")) then ($key | startswith($entry | rtrimstr("*"))) else $key == $entry end;
    # Every entry with its class, as {entry, class}.
    [
      ($c.section // {} | to_entries[] | .key as $s | .value[] | {entry: ., class: "section \($s)"}),
      ($c.fixed // [] | .[] | {entry: ., class: "fixed"}),
      ($c.generic // [] | .[] | {entry: ., class: "generic"}),
      ($c.excluded // {} | keys[] | {entry: ., class: "excluded"})
    ] as $entries |
    (
      ($c | keys - ["section", "fixed", "generic", "excluded"] | .[] | "\(.) is no class: the classes are section, fixed, generic, and excluded"),
      ($c.section // {} | keys[] | select(. as $s | $variables | index($s) | not) | "the section \(.) is no variable of the module"),
      ($c.excluded // {} | to_entries[] | select((.value | type) != "string" or .value == "") | "the excluded \(.key) gives no reason"),
      ($entries[] | . as $e | select([$all | keys[] | select(matches($e.entry; .))] | length == 0) | "\($e.entry) (\($e.class)) matches no configuration key"),
      ($all | keys[] | . as $key | [$entries[] | select(matches(.entry; $key))] |
        if length == 0 then "\($key) has no class"
        elif length > 1 then "\($key) is in two entries: \(map("\(.entry) (\(.class))") | join(", "))"
        elif (.[0].class | test("^(section|fixed)")) and ($wired | index($all[$key].env) | not) then
          "\($key) (\(.[0].class)) has no wiring test: tests/coverage.tftest.hcl does not name \($all[$key].env)"
        else empty end)
    )
  ')"; then
    echo "tofu-coverage: $name/coverage.json could not be read" >&2
    status=1
    continue
  fi
  if [ -n "$errors" ]; then
    while IFS= read -r error; do
      echo "tofu-coverage: $name/coverage.json: $error" >&2
    done <<< "$errors"
    status=1
  fi
done

if [ "$found" -eq 0 ]; then
  echo "tofu-coverage: no public module under deploy/opentofu" >&2
  exit 1
fi
exit "$status"
