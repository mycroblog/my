#!/bin/bash
# SPDX-License-Identifier: AGPL-3.0-only
# Seeds the databases `burst` and `mixed` in the throwaway container.
set -e
cd "$(dirname "$0")"
docker cp seed.sql spike-1996-pg:/tmp/seed.sql
P="docker exec -i spike-1996-pg psql -X -q -v ON_ERROR_STOP=1 -U postgres"
$P -d spike -Atc 'select version()'
for m in burst mixed; do
  $P -d spike -c "create database $m"
  $P -d $m -v mode=$m -f /tmp/seed.sql
done
$P -d burst -Atc "select type,count(*) from jobs group by 1; select pg_size_pretty(pg_total_relation_size('jobs'))"
