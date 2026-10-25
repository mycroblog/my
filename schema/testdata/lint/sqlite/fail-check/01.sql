-- SPDX-License-Identifier: AGPL-3.0-only
-- Adds a CHECK on the existing column rating (not reported by the lint; found by the inspection).
-- want: inspect: table "notes" gets a CHECK
CREATE TABLE notes (id TEXT PRIMARY KEY, body TEXT NOT NULL, rating INTEGER, kind TEXT, CHECK (rating BETWEEN 1 AND 5)) STRICT;
CREATE INDEX notes_kind ON notes (kind);
CREATE TABLE tags (id TEXT PRIMARY KEY, name TEXT NOT NULL, scope TEXT NOT NULL CHECK (scope IN ('local', 'remote'))) STRICT;
