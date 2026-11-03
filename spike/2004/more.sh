#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# more.sh <spike binary> <work dir> stall|types: the second measurement of #2004.
set -u
bin=$1; work=$2; what=$3
pg='postgres://my:my@127.0.0.1:55732/my?sslmode=disable'
psql() { docker exec spike2004-pg psql -U my -d my -tAc "$1"; }

# one <dsn> <procs> <slots> <delay> <window> <label> <flags...>: runs workers and prints jobs/s.
one() {
  local dsn=$1 procs=$2 slots=$3 delay=$4 window=$5 label=$6; shift 6
  local err pids=() c0 c1 t0 t1
  err=$(mktemp)
  for _ in $(seq 1 "$procs"); do
    "$bin" -mode worker -dsn "$dsn" -slots "$slots" "$@" 2>>"$err" &
    pids+=($!)
  done
  sleep 5
  c0=$(psql 'SELECT count(*) FROM jobs'); t0=$(date +%s.%N)
  sleep "$window"
  c1=$(psql 'SELECT count(*) FROM jobs'); t1=$(date +%s.%N)
  local rss=0 r
  for p in "${pids[@]}"; do r=$(awk '/VmRSS/{print $2}' "/proc/$p/status" 2>/dev/null); rss=$((rss + ${r:-0})); done
  local conns; conns=$(psql "SELECT count(*) FROM pg_stat_activity WHERE datname='my' AND pid<>pg_backend_pid()")
  local waiting; waiting=$(psql "SELECT count(*) FROM pg_stat_activity WHERE datname='my' AND state='idle in transaction'")
  kill "${pids[@]}" 2>/dev/null
  wait "${pids[@]}" 2>/dev/null
  local claim; claim=$(awk -F'[= ]' '/^claims=/{n+=$2; s+=$2*$4} END{if(n) printf "claims=%d claim_avg_ms=%.2f", n, s/n}' "$err")
  local dead; dead=$(grep -c 'context deadline exceeded' "$err")
  awk -v l="$label" -v p="$procs" -v s="$slots" -v c0="$c0" -v c1="$c1" -v t0="$t0" -v t1="$t1" -v rss="$rss" -v cl="$claim" -v co="$conns" -v w="$waiting" -v d="$dead" \
    'BEGIN{printf "%s procs=%d slots=%d jobs/s=%.1f rss=%.0fMiB conns=%d idle_in_tx=%d deadline_errors=%d %s\n", l,p,s,(c0-c1)/(t1-t0),rss/1024,co,w,d,cl}'
  rm -f "$err"
}

case $what in
stall)
  small="$pg&pool_max_conns=4"
  psql 'TRUNCATE jobs' >/dev/null
  "$bin" -mode load -dsn "$pg" -jobs 200000 -delay 10ms
  for hold in 0s 5ms; do
    for s in 1 2 3 4 5 8 32 80; do
      one "$small" 1 "$s" 10ms 25 "pool=4 hold=$hold" -tx-second -hold "$hold" -lease 15s
      psql "UPDATE jobs SET locked_by=NULL, locked_until=NULL WHERE locked_by IS NOT NULL" >/dev/null
    done
  done
  echo "# without the second connection"
  for s in 4 8 80; do
    one "$small" 1 "$s" 10ms 25 "pool=4 plain" -lease 15s
    psql "UPDATE jobs SET locked_by=NULL, locked_until=NULL WHERE locked_by IS NOT NULL" >/dev/null
  done
  ;;
types)
  for rep in 1 2; do
    echo "# repeat $rep"
    for cfg in "1 32" "1 128" "4 32"; do
      set -- $cfg
      for t in 0 1 3 82; do
        psql 'TRUNCATE jobs' >/dev/null
        "$bin" -mode backlog -dsn "$pg" -delay 200ms
        one "$pg" "$1" "$2" 200ms 20 "types=$t delay=200ms" -types "$t"
      done
    done
    for t in 0 82; do
      psql 'TRUNCATE jobs' >/dev/null
      "$bin" -mode backlog -dsn "$pg" -delay 10ms
      one "$pg" 1 32 10ms 20 "types=$t delay=10ms" -types "$t"
    done
  done
  ;;
esac
