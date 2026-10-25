#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# First-start initialization of the upgrade test's database (compose.yaml): the role and the databases of the
# server (`my`, and `my_dev` for Atlas), each prepared with the previous release's schema/postgres.bootstrap.sql.
set -eu

psql -v ON_ERROR_STOP=1 -q -U postgres -d postgres <<'SQL'
CREATE ROLE my LOGIN PASSWORD 'my';
CREATE DATABASE my OWNER my;
CREATE DATABASE my_dev OWNER my;
SQL

for db in my my_dev; do
	PGOPTIONS=--client-min-messages=warning psql -v ON_ERROR_STOP=1 -q -U postgres -d "$db" -f /bootstrap/postgres.bootstrap.sql
done
