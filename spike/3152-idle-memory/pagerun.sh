#!/bin/bash
# pagerun.sh <binary built without -s -w> NAME=value...: needs <binary>.nm, the output of `go tool nm -size <binary>`; start the binary, and print which of its pages are resident once it has started
set -eu
BINARY=$1; shift
A=127.0.0.1:16060
. ./env.sh   # written by experiment.sh
(exec env -i PATH="$PATH" HOME="$HOME" "${BASE[@]}" MY_PPROF_LISTEN=$A "$@" $BINARY serve > out-pages.log 2>&1) &
PID=$!
trap 'kill $PID 2>/dev/null; wait $PID 2>/dev/null || true' EXIT
curl -fsS -o /dev/null --retry 60 --retry-connrefused --retry-delay 1 "http://$A/debug/pprof/"
grep -E '^(VmRSS|RssAnon|RssFile)' /proc/$PID/status | tr '\n' ' '; echo
python3 pages.py $PID $BINARY $BINARY.nm 24
