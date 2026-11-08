#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Copies the desired-state schema of one dialect into a flat directory for the Atlas CLI (ADR-0023, ADR-0081 §8).
#
#   scripts/schema-files.sh sqlite|postgres DEST [TREE]
#
# The schema is one directory per module, schema/<NN>-<module>/<dialect>.sql. Atlas Community Edition reads the SQL
# files of one directory in name order and takes one SQL source only (several file:// URLs are refused), so this
# copies each module's file of the dialect to DEST/<NN>-<module>.sql, which keeps the order. TREE is the root of the
# tree to read (default: this repository); a tree whose files are directly in schema/<dialect>/ is copied as it is,
# so that db:lint can compare with such a base commit. DEST must exist; the files are added to it.
set -euo pipefail

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
  sed -n '6p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
fi
dialect="$1" dest="$2" tree="${3:-$(cd "$(dirname "$0")/.." && pwd)}"
case "$dialect" in sqlite | postgres) ;; *) echo "schema-files: unknown dialect $dialect" >&2 && exit 2 ;; esac

shopt -s nullglob
if [ -d "$tree/schema/$dialect" ]; then
  for f in "$tree/schema/$dialect"/*.sql; do cp "$f" "$dest/"; done
  exit 0
fi
for f in "$tree"/schema/[0-9][0-9]-*/"$dialect.sql"; do
  cp "$f" "$dest/$(basename "$(dirname "$f")").sql"
done
