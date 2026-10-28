#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# First-start initialization of the live interoperability database (compose.yaml): the role and databases of
# this server, each prepared with schema/postgres.bootstrap.sql as `scripts/task.sh db:up` does. Mastodon creates its own
# database as the superuser; Misskey and Akkoma get a role and an empty database, which they migrate when they start
# (Akkoma's extensions need the superuser, so they are created here, as its sample_psql.eex does).
set -eu

psql -v ON_ERROR_STOP=1 -q -U postgres -d postgres <<'SQL'
CREATE ROLE my LOGIN PASSWORD 'my';
CREATE DATABASE my OWNER my;
CREATE DATABASE my_dev OWNER my;
CREATE ROLE misskey LOGIN PASSWORD 'misskey';
CREATE DATABASE misskey OWNER misskey;
CREATE ROLE akkoma LOGIN PASSWORD 'akkoma';
CREATE DATABASE akkoma OWNER akkoma;
\c akkoma
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
SQL

for db in my my_dev; do
	PGOPTIONS=--client-min-messages=warning psql -v ON_ERROR_STOP=1 -q -U postgres -d "$db" -f /bootstrap/postgres.bootstrap.sql
done
