#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# The shards of the server module: its packages in groups by layer, which CI gives a runner each (ADR-0079 §11).
# `go test` and golangci-lint use every core of a runner, and cmd/my and internal/platform, which depend on every
# other package, take as long alone as the rest together, so side by side on one runner they wait for each other.
# `scripts/task.sh go:test SHARD=<name>` and `go:lint SHARD=<name>` run one shard; without SHARD they run `./...`.
#
#   scripts/go-shard.sh NAME     the package patterns of the shard, on one line
#   scripts/go-shard.sh --list   the names of the shards, one per line
#   scripts/go-shard.sh --check  fail when a package of `./...` is in no shard, or in two
#
# A new directory at the top of the module, or of internal/, is in no shard until it is added below: --check, which
# the merge queue runs (go:shard-check), fails on it, so that no package leaves the tests without a word.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

# NAME=the directories of the shard, each with its subdirectories.
SHARDS=(
  # The commands of `my`: one package, the slowest, which starts the server as a process for most of its tests.
  "cmd=cmd"
  # The assembly of the server: the tests that run the whole server in the test process.
  "platform=internal/platform"
  # The adapters, more than half of the packages.
  "adapter=internal/adapter"
  # The HTTP interfaces.
  "interface=internal/interface"
  # The use cases and the domain, with the flow tests across them and the server's messages.
  "app=internal/app internal/domain internal/flowtest internal/i18n"
  # What is outside cmd/ and internal/: pkg/ and the packages of one file that embed the spec, the catalogs and the
  # schema, and the Go programs of scripts/ and test/.
  "pkg=pkg api locales schema scripts test"
)

dirs_of() {
  local entry
  for entry in "${SHARDS[@]}"; do
    if [ "${entry%%=*}" = "$1" ]; then
      echo "${entry#*=}"
      return 0
    fi
  done
  echo "go-shard.sh: no shard named $1: $(printf '%s ' "${SHARDS[@]%%=*}")" >&2
  return 1
}

case "${1:-}" in
  --list)
    printf '%s\n' "${SHARDS[@]%%=*}"
    ;;
  --check)
    cd "$root"
    module="$(GOWORK=off go list -m)"
    status=0
    while IFS= read -r pkg; do
      rel="${pkg#"$module"}"
      rel="${rel#/}"
      found=()
      for entry in "${SHARDS[@]}"; do
        for dir in ${entry#*=}; do
          case "$rel/" in "$dir"/*) found+=("${entry%%=*}") ;; esac
        done
      done
      if [ "${#found[@]}" -eq 0 ]; then
        echo "go-shard.sh: ${rel:-.} is in no shard, so no job of CI tests or lints it: add its directory to a shard of SHARDS in scripts/go-shard.sh" >&2
        status=1
      elif [ "${#found[@]}" -gt 1 ]; then
        echo "go-shard.sh: ${rel:-.} is in the shards ${found[*]}: keep it in one" >&2
        status=1
      fi
    done < <(GOWORK=off go list ./...)
    if [ "$status" -eq 0 ]; then echo "every package of the server module is in one shard"; fi
    exit "$status"
    ;;
  "" | -*)
    sed -n '9,11p' "$0" | sed 's/^# \{0,1\}//' >&2
    exit 2
    ;;
  *)
    dirs="$(dirs_of "$1")"
    out=""
    for dir in $dirs; do out+="./$dir/... "; done
    echo "${out% }"
    ;;
esac
