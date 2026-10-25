-- SPDX-License-Identifier: AGPL-3.0-only
-- Adds a unique index on the existing column name (a warning of the lint).
-- want: lint MF101
CREATE TABLE notes (id TEXT PRIMARY KEY, body TEXT NOT NULL, rating INTEGER, kind TEXT) STRICT;
CREATE INDEX notes_kind ON notes (kind);
CREATE TABLE tags (id TEXT PRIMARY KEY, name TEXT NOT NULL, scope TEXT NOT NULL CHECK (scope IN ('local', 'remote'))) STRICT;
CREATE UNIQUE INDEX tags_name ON tags (name);
