-- SPDX-License-Identifier: AGPL-3.0-only
-- Renames kind to category: a drop and an add (DS103).
-- want: lint DS103
CREATE TABLE notes (id TEXT PRIMARY KEY, body TEXT NOT NULL, rating INTEGER, category TEXT) STRICT;
CREATE INDEX notes_kind ON notes (category);
CREATE TABLE tags (id TEXT PRIMARY KEY, name TEXT NOT NULL, scope TEXT NOT NULL CHECK (scope IN ('local', 'remote'))) STRICT;
