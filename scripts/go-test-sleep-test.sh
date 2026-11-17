#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/go-test-sleep.sh on a repository it builds: new uses of time.Sleep in a Go test fail, the uses of the
# baseline pass, and the baseline only shrinks.
set -euo pipefail

check="$(cd "$(dirname "$0")" && pwd)/go-test-sleep.sh"
repo="$(mktemp -d)"
trap 'rm -rf "$repo"' EXIT
fail=0

g() { git -C "$repo" -c user.name=t -c user.email=t@example.com "$@"; }
put() { mkdir -p "$(dirname "$repo/$1")" && printf '%b' "$2" > "$repo/$1"; }

# expect WANT NAME: the check's exit status on the working tree of the repository, against the branch base.
expect() {
  local want=$1 name=$2 got=0
  "$check" --root "$repo" --base base > "$repo.out" 2>&1 || got=$?
  if [ "$got" != "$want" ]; then
    echo "FAIL: $name: exit $got, want $want" >&2
    sed 's/^/  /' "$repo.out" >&2
    fail=1
  fi
}

sleep2='package a\n\nfunc TestA(t *testing.T) {\n\ttime.Sleep(time.Millisecond)\n\ttime.Sleep(time.Second)\n}\n'
sleep1='package a\n\nfunc TestA(t *testing.T) {\n\ttime.Sleep(time.Millisecond)\n}\n'
g init -q
put scripts/go-test-sleep.baseline '# head\n\na/a_test.go 2\n'
put a/a_test.go "$sleep2"
put a/a.go 'package a\n\nfunc wait() { time.Sleep(time.Second) }\n'
put b/b_test.go 'package b\n\n// mytime.Sleep and time.Sleeper are other names\nvar _ = mytime.Sleeper\n'
g add -A
g commit -q -m base
g branch base

expect 0 "the uses of the baseline, and a Sleep outside the tests"
put b/b_test.go "$sleep1"
expect 1 "a new file that sleeps"
g checkout -q -- b/b_test.go
put a/a_test.go "$sleep2"'func TestB(t *testing.T) { time.Sleep(1) }\n'
expect 1 "a file of the baseline that sleeps more"
put a/a_test.go "$sleep1"
expect 1 "a use gone with its line kept"
put scripts/go-test-sleep.baseline '# head\n\na/a_test.go 1\n'
expect 0 "a use gone with its count lowered"
put a/a_test.go 'package a\n'
put scripts/go-test-sleep.baseline '# head\n'
expect 0 "the last use gone with its line"
put a/a_test.go "$sleep2"'func TestB(t *testing.T) { time.Sleep(1) }\n'
put scripts/go-test-sleep.baseline '# head\n\na/a_test.go 3\n'
expect 1 "a count raised above its base"
put a/a_test.go "$sleep2"
put b/b_test.go "$sleep1"
put scripts/go-test-sleep.baseline '# head\n\na/a_test.go 2\nb/b_test.go 1\n'
expect 1 "a file added to the baseline"

rm -f "$repo.out"
exit "$fail"
