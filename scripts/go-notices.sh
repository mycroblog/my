#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Writes the license notices of the Go code linked into the server: the standard library and every module that
# ./cmd/my imports, each with its license's identifier and full text, as Markdown in the form of the npm notices that
# Vite writes (web/app/vite.config.ts). The image carries the file next to LICENSE (ADR-0010 §10). It fails when the
# license of a module cannot be identified or its text found, so that nothing is released without its notice.
#
#   scripts/go-notices.sh OUT   run from the repository root; GOOS, GOARCH and GOFLAGS (build tags) select the build
#                               whose modules are listed, as for `go build`
#
# go-licenses (tools/go.mod) is built for the host whatever GOOS and GOARCH say; GO_LICENSES names another binary
# (scripts/go-notices-test.sh). scripts/go-notices-test.sh tests it.
set -euo pipefail

if (($# != 1)); then
  echo "usage: $0 OUT" >&2
  exit 2
fi
out="$1"

# The server's own module: its license is LICENSE, which goes next to the notices.
self=github.com/mycroblog/my

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

golicenses="${GO_LICENSES:-}"
if [[ -z "$golicenses" ]]; then
  golicenses="$work/go-licenses"
  env -u GOOS -u GOARCH -u GOFLAGS GOWORK=off \
    go build -modfile=tools/go.mod -o "$golicenses" github.com/google/go-licenses/v2
fi

# One line per module and license: name, version, license identifier, path of the license's file.
printf '%s' '{{range .}}{{.Name}}{{"\t"}}{{.Version}}{{"\t"}}{{.LicenseName}}{{"\t"}}{{.LicensePath}}{{"\n"}}{{end}}' \
  > "$work/report.tmpl"
GOWORK=off "$golicenses" report ./cmd/my --template "$work/report.tmpl" --logtostderr=false --log_dir="$work" --stderrthreshold=ERROR > "$work/report.tsv"

goroot="$(go env GOROOT)"
{
  printf '# Licenses\n\nThe server links the following Go code, under the following licenses:\n'
  printf '\n## Go standard library - %s (BSD-3-Clause)\n\n' "$(go env GOVERSION)"
  cat "$goroot/LICENSE"
} > "$work/notices.md"

problems=0
prev=""
names=""
path=""
version=""
flush() {
  if [[ -z "$prev" ]]; then return; fi
  printf '\n## %s - %s (%s)\n\n' "$prev" "$version" "$names" >> "$work/notices.md"
  cat "$path" >> "$work/notices.md"
}
while IFS=$'\t' read -r name ver license file; do
  if [[ "$name" == "$self" ]]; then continue; fi
  if [[ "$license" == Unknown || -z "$license" ]]; then
    echo "go-notices: the license of $name cannot be identified: give the module a known license or replace it" >&2
    problems=1
    continue
  fi
  if [[ "$file" == Unknown || ! -f "$file" ]]; then
    echo "go-notices: the license text of $name is missing: give the module its license file or replace it" >&2
    problems=1
    continue
  fi
  if [[ "$name" == "$prev" ]]; then
    names="$names, $license"
    continue
  fi
  flush
  prev="$name" version="$ver" names="$license" path="$file"
done < "$work/report.tsv"
flush

if ((problems)); then exit 1; fi
if [[ "$prev" == "" ]]; then
  echo "go-notices: go-licenses listed no module of ./cmd/my" >&2
  exit 1
fi
mkdir -p "$(dirname "$out")"
cp "$work/notices.md" "$out"
