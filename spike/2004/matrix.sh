#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# matrix.sh <spike binary> <work dir> setup|smoke|all
set -u
here=$(cd "$(dirname "$0")" && pwd)
bin=$1; work=$2; what=$3
pg='postgres://my:my@127.0.0.1:55732/my?sslmode=disable'
small="$pg&pool_max_conns=4"
sq="file:$work/s.db"
run() { "$here/run.sh" "$bin" "$@"; }
case $what in
setup)
  "$bin" -mode schema -dsn "$pg" 2>&1 | tail -2 | cut -c1-300
  docker exec spike2004-pg psql -U my -d my -tAc "select count(*) from information_schema.tables where table_name in ('jobs','dead_jobs')"
  rm -f "$work"/s.db*
  "$bin" -mode schema -dialect sqlite -dsn "$sq" 2>&1 | tail -2 | cut -c1-300
  sqlite3 "$work/s.db" "select count(*) from sqlite_master where name='jobs'"
  nohup "$bin" -mode remote >/dev/null 2>&1 &
  sleep 1
  curl -s 'http://127.0.0.1:18999/?d=10ms'; echo
  ;;
smoke)
  run postgres "$pg" 1 8 200ms 0 6
  ;;
all)
  for rep in 1 2; do
    echo "# repeat $rep"
    for d in 200ms 2s; do
      for cfg in "1 1" "8 1" "1 8" "32 1" "1 32" "1 128"; do
        set -- $cfg
        run postgres "$pg" "$1" "$2" "$d" 0 20
      done
    done
    echo "# small pool: pool_max_conns=4 in each process"
    run postgres "$small" 1 32 200ms 0 20
    run postgres "$small" 1 128 200ms 0 20
    run postgres "$small" 32 1 200ms 0 20
    echo "# the HTTP client's 8 connections per host, one host"
    run postgres "$pg" 1 32 200ms 8 20
    echo "# sqlite, a write transaction in each job"
    for s in 1 8 32 128; do run sqlite "$sq" 1 "$s" 200ms 0 20; done
  done
  ;;
dispatch)
  for rep in 1 2; do
    echo "# repeat $rep: runner per slot, then one claim loop"
    for cfg in "$pg 128" "$small 128" "$small 32" "$pg 32"; do
      set -- $cfg
      SPIKE_FLAGS= run postgres "$1" 1 "$2" 200ms 0 20
      SPIKE_FLAGS=-dispatch run postgres "$1" 1 "$2" 200ms 0 20
    done
    SPIKE_FLAGS=-dispatch run sqlite "$sq" 1 32 200ms 0 20
    SPIKE_FLAGS=-dispatch run sqlite "$sq" 1 128 200ms 0 20
  done
  ;;
idle)
  docker exec spike2004-pg psql -U my -d my -qc 'TRUNCATE jobs'
  for flags in "" "-dispatch"; do
    "$bin" -mode worker -dsn "$pg" -slots 128 $flags 2>/dev/null &
    pid=$!
    sleep 3
    a=$(docker exec spike2004-pg psql -U my -d my -tAc "select xact_commit+xact_rollback from pg_stat_database where datname='my'")
    sleep 20
    b=$(docker exec spike2004-pg psql -U my -d my -tAc "select xact_commit+xact_rollback from pg_stat_database where datname='my'")
    kill "$pid"; wait "$pid" 2>/dev/null
    echo "idle slots=128 flags='$flags' transactions/s=$(( (b-a)/20 ))"
  done
  ;;
shared)
  for rep in 1 2; do
    echo "# repeat $rep: a runner per slot with one polling slot"
    for cfg in "$pg 128" "$small 128" "$pg 32" "$small 32"; do
      set -- $cfg
      SPIKE_FLAGS=-shared-poll run postgres "$1" 1 "$2" 200ms 0 20
    done
    SPIKE_FLAGS=-shared-poll run sqlite "$sq" 1 32 200ms 0 20
  done
  docker exec spike2004-pg psql -U my -d my -qc 'TRUNCATE jobs'
  "$bin" -mode worker -dsn "$pg" -slots 128 -shared-poll 2>/dev/null &
  pid=$!
  sleep 3
  a=$(docker exec spike2004-pg psql -U my -d my -tAc "select xact_commit+xact_rollback from pg_stat_database where datname='my'")
  sleep 20
  b=$(docker exec spike2004-pg psql -U my -d my -tAc "select xact_commit+xact_rollback from pg_stat_database where datname='my'")
  kill "$pid"; wait "$pid" 2>/dev/null
  echo "idle slots=128 shared-poll transactions/s=$(( (b-a)/20 ))"
  ;;
race)
  run postgres "$pg" 1 32 200ms 0 10
  ;;
esac
