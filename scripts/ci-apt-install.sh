#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Installs Ubuntu packages in a CI job's container, and tries other mirrors when the image's own package sources
# cannot be reached: runners have gone hours without resolving azure.archive.ubuntu.com and security.ubuntu.com, where
# `apt-get update` exits 0 with warnings and `apt-get install` then finds no package. apt verifies the Release
# signatures with Ubuntu's keys whatever the mirror, so the integrity of a package does not depend on the mirror.
#
# The Ubuntu sources (DEB822 `URIs:` lines of *.sources, `deb` lines of sources.list and *.list) are pointed at each
# mirror in turn, the archive and the security hosts alike; other sources are left as they are. The sources stay on
# the mirror that worked, and return to the image's own when none did.
#
#   scripts/ci-apt-install.sh <package>...
#
# The tests set CI_APT_GET (the apt-get command), CI_APT_ROOT (the directory that holds etc/apt), CI_APT_ARCH and
# CI_APT_MIRRORS (a space-separated list).
set -euo pipefail

if [[ $# -eq 0 ]]; then
  echo "usage: scripts/ci-apt-install.sh <package>..." >&2
  exit 2
fi

apt_get="${CI_APT_GET:-apt-get}"
apt_dir="${CI_APT_ROOT:-}/etc/apt"
arch="${CI_APT_ARCH:-$(dpkg --print-architecture)}"

# Mirrors on other domains than ubuntu.com first, then Ubuntu's own under another name than the image's. Each one
# serves noble, noble-updates and noble-security for the architecture. amd64 and i386 are in /ubuntu, every other
# architecture in /ubuntu-ports, which fewer mirrors carry.
case "$arch" in
amd64 | i386)
  mirrors=(https://mirrors.kernel.org/ubuntu https://mirrors.mit.edu/ubuntu http://archive.ubuntu.com/ubuntu)
  ;;
*)
  mirrors=(https://mirrors.ocf.berkeley.edu/ubuntu-ports https://mirrors.mit.edu/ubuntu-ports http://ports.ubuntu.com/ubuntu-ports)
  ;;
esac
if [[ -n "${CI_APT_MIRRORS:-}" ]]; then
  read -r -a mirrors <<<"$CI_APT_MIRRORS"
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

sources=()
for file in "$apt_dir/sources.list" "$apt_dir"/sources.list.d/*.sources "$apt_dir"/sources.list.d/*.list; do
  [[ -f "$file" ]] && sources+=("$file")
done
for i in "${!sources[@]}"; do
  cp "${sources[$i]}" "$work/$i"
done

# use_mirror MIRROR writes the sources again from the image's own, with MIRROR for every Ubuntu host; without an
# argument it restores the image's own.
use_mirror() {
  local i
  for i in "${!sources[@]}"; do
    if [[ $# -eq 0 ]]; then
      cat "$work/$i" >"${sources[$i]}"
    else
      sed -E "/^(URIs:|deb(-src)?[[:space:]])/ s#https?://([A-Za-z0-9-]+\.)*(archive|security|ports)\.ubuntu\.com/ubuntu(-ports)?(/|[[:space:]]|\$)#$1\4#g" \
        "$work/$i" >"${sources[$i]}"
    fi
  done
}

last_error=""
# attempt PACKAGE... updates the indexes and installs. The install decides: `update` exits 0 without the indexes, and
# warns about a third-party source that is down while the Ubuntu ones are fine.
attempt() {
  local log="$work/log" status=0
  "$apt_get" update 2>&1 | tee "$log" || status=$?
  if [[ "$status" -ne 0 ]] || grep -Eq 'Failed to fetch|Could not resolve' "$log"; then
    last_error="$(grep -E 'Failed to fetch|Could not resolve|^E:' "$log" | tail -n 1 || true)"
  fi
  status=0
  "$apt_get" install -y --no-install-recommends "$@" 2>&1 | tee "$log" || status=$?
  if [[ "$status" -eq 0 ]]; then
    return 0
  fi
  last_error="$(grep -E '^E:' "$log" | tail -n 1 || true)${last_error:+ (after: ${last_error# })}"
  return 1
}

export DEBIAN_FRONTEND=noninteractive

if attempt "$@"; then
  echo "ci-apt-install: installed $* from the image's sources"
  exit 0
fi
tried=("the image's sources")
for mirror in "${mirrors[@]}"; do
  echo "ci-apt-install: could not install $* from ${tried[-1]}: trying $mirror" >&2
  tried+=("$mirror")
  use_mirror "$mirror"
  last_error=""
  if attempt "$@"; then
    echo "ci-apt-install: installed $* from $mirror"
    exit 0
  fi
done
use_mirror

list="$(printf '%s, ' "${tried[@]}")"
echo "ci-apt-install: could not install $*; tried ${list%, }; last error: ${last_error:-none printed}" >&2
exit 1
