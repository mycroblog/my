-- SPDX-License-Identifier: AGPL-3.0-only
-- Makes the existing column rating NOT NULL (a warning of the lint).
-- want: lint LT101
CREATE TABLE notes (id TEXT PRIMARY KEY, body TEXT NOT NULL, rating INTEGER NOT NULL, kind TEXT) STRICT;
CREATE INDEX notes_kind ON notes (kind);
CREATE TABLE tags (id TEXT PRIMARY KEY, name TEXT NOT NULL, scope TEXT NOT NULL CHECK (scope IN ('local', 'remote'))) STRICT;
