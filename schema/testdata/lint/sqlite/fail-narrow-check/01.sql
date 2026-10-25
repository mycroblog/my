-- SPDX-License-Identifier: AGPL-3.0-only
-- Removes a value from the values the CHECK of scope allows.
-- want: inspect: table "tags" gets a CHECK
CREATE TABLE notes (id TEXT PRIMARY KEY, body TEXT NOT NULL, rating INTEGER, kind TEXT) STRICT;
CREATE INDEX notes_kind ON notes (kind);
CREATE TABLE tags (id TEXT PRIMARY KEY, name TEXT NOT NULL, scope TEXT NOT NULL CHECK (scope IN ('local'))) STRICT;
