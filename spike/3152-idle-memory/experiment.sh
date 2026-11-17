#!/bin/bash
# experiment.sh <label> <idle seconds> NAME=value...: start `my serve` (BIN, default ./my, a build of a commit that
# has pprof_listen) with the settings, measure it when it has started and again after it idled, then stop it.
# The database of MY_DATABASE_URL has the schema applied (`my schema apply`, `my db upgrade`) with the settings of
# env.sh, which the first run writes.
set -eu
L=$1; IDLE=$2; shift 2
A=127.0.0.1:16060
[ -f env.sh ] || echo "BASE=(MY_HOST=localhost:18091 MY_LISTEN=127.0.0.1:18091 MY_KEK=$(openssl rand -base64 32) MY_MAIL=none MY_BLOB=fs)" > env.sh
. ./env.sh
# exec, so that $! is the server and not a shell around it
(exec env -i PATH="$PATH" HOME="$HOME" "${BASE[@]}" MY_PPROF_LISTEN=$A "$@" ${BIN:-./my} serve > out-$L.log 2>&1) &
PID=$!
trap 'kill $PID 2>/dev/null; wait $PID 2>/dev/null || true' EXIT
curl -fsS -o /dev/null --retry 60 --retry-connrefused --retry-delay 1 "http://$A/debug/pprof/"
test "$(readlink /proc/$PID/exe)" = "$(readlink -f ${BIN:-./my})"
./measure.sh $L-start $PID $A
sleep "$IDLE"
./measure.sh $L-idle $PID $A
