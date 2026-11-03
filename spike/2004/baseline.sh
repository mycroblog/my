#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# baseline.sh <spike binary> <my binary>: the memory of a real `my serve` with role = worker, idle.
set -u
spike=$1; my=$2
docker exec spike2004-pg psql -U my -d my -qc 'CREATE DATABASE myreal' 2>/dev/null
url='postgres://my:my@127.0.0.1:55732/myreal?sslmode=disable'
"$spike" -mode schema -dsn "$url" 2>&1 | tail -1 | cut -c1-200
MY_HOST=spike.example MY_DATABASE_URL="$url" MY_DB=postgres MY_ROLE=worker MY_KEK="$(openssl rand -base64 32)" MY_LISTEN=127.0.0.1:18998 \
  "$my" serve >/dev/null 2>"$spike.real.log" &
pid=$!
sleep 15
if kill -0 "$pid" 2>/dev/null; then
  grep -E 'VmRSS|VmHWM|Threads' "/proc/$pid/status" | tr -s '\t ' ' '
else
  echo "the server exited"; tail -3 "$spike.real.log" | cut -c1-300
fi
kill "$pid" 2>/dev/null
wait "$pid" 2>/dev/null
