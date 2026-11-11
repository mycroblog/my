#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/ci-apt-install.sh with a fake apt-get on package sources in a temporary directory.
#
#   scripts/ci-apt-install-test.sh
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/ci-apt-install.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# The fake reaches the hosts of FAKE_REACHABLE (a space-separated list). Like apt-get, `update` exits 0 with warnings
# when it fetches no Ubuntu index, and `install` then finds no package.
cat >"$tmp/apt-get" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "$*" >>"$FAKE_STATE/calls"
case "$1" in
update)
  rm -f "$FAKE_STATE/indexes"
  urls="$(cat "$CI_APT_ROOT/etc/apt/sources.list" "$CI_APT_ROOT"/etc/apt/sources.list.d/* 2>/dev/null |
    grep -E '^(URIs:|deb )' | grep -Eo 'https?://[^ ]+')"
  missed=0
  for url in $urls; do
    host="${url#*://}"
    host="${host%%/*}"
    if [[ " $FAKE_REACHABLE " == *" $host "* ]]; then
      echo "Get:1 $url noble InRelease"
    else
      echo "W: Failed to fetch $url  Could not resolve '$host'"
      [[ "$url" == *nodesource* ]] || missed=1
    fi
  done
  if [[ "$missed" -eq 0 ]]; then
    touch "$FAKE_STATE/indexes"
  else
    echo "W: Some index files failed to download. They have been ignored, or old ones used instead."
  fi
  ;;
install)
  if [[ ! -e "$FAKE_STATE/indexes" ]]; then
    echo "E: Unable to locate package ${*: -1}"
    exit 100
  fi
  echo "Setting up ${*: -1}"
  ;;
esac
EOF
chmod +x "$tmp/apt-get"

nodesource="deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_26.x nodistro main"

# setup FORMAT makes the sources of an image in $tmp/root: deb822 (noble) or classic.
setup() {
  rm -rf "$tmp/root" "$tmp/state"
  mkdir -p "$tmp/root/etc/apt/sources.list.d" "$tmp/state"
  echo "$nodesource" >"$tmp/root/etc/apt/sources.list.d/nodesource.list"
  if [[ "$1" == deb822 ]]; then
    echo "# Ubuntu sources have moved to /etc/apt/sources.list.d/ubuntu.sources" >"$tmp/root/etc/apt/sources.list"
    cat >"$tmp/root/etc/apt/sources.list.d/ubuntu.sources" <<'EOF'
## See http://help.ubuntu.com/community/UpgradeNotes and http://archive.ubuntu.com/ubuntu/ for the suites.
Types: deb
URIs: http://azure.archive.ubuntu.com/ubuntu/
Suites: noble noble-updates noble-backports
Components: main universe restricted multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

Types: deb
URIs: http://security.ubuntu.com/ubuntu/
Suites: noble-security
Components: main universe restricted multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
EOF
  else
    cat >"$tmp/root/etc/apt/sources.list" <<'EOF'
# deb http://archive.ubuntu.com/ubuntu/ jammy-proposed main
deb http://archive.ubuntu.com/ubuntu/ jammy main universe
deb [arch=amd64] http://azure.archive.ubuntu.com/ubuntu jammy-updates main universe
deb-src http://security.ubuntu.com/ubuntu/ jammy-security main
EOF
  fi
  cp -R "$tmp/root" "$tmp/image"
}

failed=0
fail() {
  echo "FAIL: $*" >&2
  failed=1
}

# run NAME WANT_STATUS REACHABLE PACKAGE...: the output is in $tmp/out.
run() {
  local name="$1" want="$2" reachable="$3" status=0
  shift 3
  CI_APT_GET="$tmp/apt-get" CI_APT_ROOT="$tmp/root" CI_APT_ARCH=amd64 \
    CI_APT_MIRRORS="https://mirror.example.org/ubuntu https://other.example.net/pub/ubuntu" \
    FAKE_STATE="$tmp/state" FAKE_REACHABLE="$reachable" "$script" "$@" >"$tmp/out" 2>&1 || status=$?
  [[ "$status" -eq "$want" ]] || fail "$name: want exit $want, got $status: $(cat "$tmp/out")"
}

# says NAME TEXT: the last line of the output contains TEXT.
says() {
  [[ "$(tail -n 1 "$tmp/out")" == *"$2"* ]] || fail "$1: want the last line to contain '$2', got '$(tail -n 1 "$tmp/out")'"
}

# same NAME FILE: the file of the sources is the image's own.
same() {
  cmp -s "$tmp/root/etc/apt/$2" "$tmp/image/etc/apt/$2" || fail "$1: $2 changed: $(cat "$tmp/root/etc/apt/$2")"
}

# sources NAME FILE WANT: the lines of FILE that are neither comments nor empty equal WANT.
sources() {
  local got
  got="$(grep -Ev '^(#|$)' "$tmp/root/etc/apt/$2" || true)"
  [[ "$got" == "$3" ]] || fail "$1: $2: want
$3
got
$got"
}

setup deb822
run "the image's sources work" 0 "azure.archive.ubuntu.com security.ubuntu.com deb.nodesource.com" unzip jq
says "the image's sources work" "installed unzip jq from the image's sources"
same "the image's sources work" sources.list.d/ubuntu.sources
[[ "$(cat "$tmp/state/calls")" == $'update\ninstall -y --no-install-recommends unzip jq' ]] ||
  fail "the image's sources work: unexpected apt-get calls: $(cat "$tmp/state/calls")"

setup deb822
run "a third-party source is down" 0 "azure.archive.ubuntu.com security.ubuntu.com" unzip
says "a third-party source is down" "installed unzip from the image's sources"

setup deb822
run "update exits 0 but install fails, the first mirror works" 0 "mirror.example.org deb.nodesource.com" unzip
says "the first mirror works" "installed unzip from https://mirror.example.org/ubuntu"
sources "deb822" sources.list.d/ubuntu.sources "Types: deb
URIs: https://mirror.example.org/ubuntu/
Suites: noble noble-updates noble-backports
Components: main universe restricted multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
Types: deb
URIs: https://mirror.example.org/ubuntu/
Suites: noble-security
Components: main universe restricted multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg"
grep -Fq "## See http://help.ubuntu.com/community/UpgradeNotes and http://archive.ubuntu.com/ubuntu/ for" \
  "$tmp/root/etc/apt/sources.list.d/ubuntu.sources" || fail "deb822: a comment changed"
same "deb822" sources.list
same "deb822" sources.list.d/nodesource.list

setup deb822
run "the first mirror fails, the second works" 0 "other.example.net" unzip
says "the second mirror works" "installed unzip from https://other.example.net/pub/ubuntu"
grep -Fq "trying https://mirror.example.org/ubuntu" "$tmp/out" || fail "the second mirror works: the first is not reported"
[[ "$(grep -c '^URIs: https://other.example.net/pub/ubuntu/$' "$tmp/root/etc/apt/sources.list.d/ubuntu.sources")" -eq 2 ]] ||
  fail "the second mirror works: $(cat "$tmp/root/etc/apt/sources.list.d/ubuntu.sources")"
same "the second mirror works" sources.list.d/nodesource.list

setup classic
run "classic sources" 0 "mirror.example.org" unzip
says "classic sources" "installed unzip from https://mirror.example.org/ubuntu"
sources "classic" sources.list "deb https://mirror.example.org/ubuntu/ jammy main universe
deb [arch=amd64] https://mirror.example.org/ubuntu jammy-updates main universe
deb-src https://mirror.example.org/ubuntu/ jammy-security main"
grep -Fxq "# deb http://archive.ubuntu.com/ubuntu/ jammy-proposed main" "$tmp/root/etc/apt/sources.list" ||
  fail "classic: a comment changed"
same "classic" sources.list.d/nodesource.list

setup deb822
run "every mirror fails" 1 "deb.nodesource.com" unzip
says "every mirror fails" "could not install unzip; tried the image's sources, https://mirror.example.org/ubuntu, https://other.example.net/pub/ubuntu; last error: E: Unable to locate package unzip"
same "every mirror fails" sources.list.d/ubuntu.sources
same "every mirror fails" sources.list.d/nodesource.list
if grep -Eq 'allow-unauthenticated|AllowUnauthenticated|AllowInsecure|trusted=yes' "$script" "$tmp/state/calls"; then
  fail "every mirror fails: signature checks are disabled"
fi

run "no package" 2 ""

# The mirrors the script lists by default, per architecture.
setup deb822
CI_APT_GET="$tmp/apt-get" CI_APT_ROOT="$tmp/root" CI_APT_ARCH=arm64 FAKE_STATE="$tmp/state" FAKE_REACHABLE="ports.ubuntu.com" \
  "$script" unzip >"$tmp/out" 2>&1 || fail "arm64: $(cat "$tmp/out")"
says "arm64" "installed unzip from http://ports.ubuntu.com/ubuntu-ports"

if [[ "$failed" -ne 0 ]]; then
  exit 1
fi
echo "ci-apt-install-test: ok"
