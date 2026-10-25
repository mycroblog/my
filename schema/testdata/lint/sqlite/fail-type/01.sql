-- SPDX-License-Identifier: AGPL-3.0-only
-- Changes the type of rating (not reported by the lint; found by the inspection).
-- want: inspect: the type of column "notes"."rating" changes
CREATE TABLE notes (id TEXT PRIMARY KEY, body TEXT NOT NULL, rating TEXT, kind TEXT) STRICT;
CREATE INDEX notes_kind ON notes (kind);
CREATE TABLE tags (id TEXT PRIMARY KEY, name TEXT NOT NULL, scope TEXT NOT NULL CHECK (scope IN ('local', 'remote'))) STRICT;
