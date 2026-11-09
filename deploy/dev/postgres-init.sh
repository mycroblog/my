#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Runs once, when the PostgreSQL of the root compose.yaml starts on an empty volume (/docker-entrypoint-initdb.d): creates
# the dev databases of Atlas, my_dev and my_lint, and runs the bootstrap SQL of the schema on them and on my, as the
# task db:up did on its container (`my schema apply` runs it on my too; it is idempotent).
set -eu

for db in my_dev my_lint; do
	createdb -U "$POSTGRES_USER" "$db"
done
for db in my my_dev my_lint; do
	PGOPTIONS=--client-min-messages=warning psql -v ON_ERROR_STOP=1 -q -U "$POSTGRES_USER" -d "$db" -f /my/bootstrap.sql
done
