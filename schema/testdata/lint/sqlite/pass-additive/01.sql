-- SPDX-License-Identifier: AGPL-3.0-only
-- A table, a nullable column, a column with a DEFAULT and a CHECK on itself, and an index.
-- want: the change is additive
CREATE TABLE notes (id TEXT PRIMARY KEY, body TEXT NOT NULL, rating INTEGER, kind TEXT, summary TEXT, state TEXT NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'closed'))) STRICT;
CREATE INDEX notes_kind ON notes (kind);
CREATE INDEX notes_rating ON notes (rating);
CREATE TABLE tags (id TEXT PRIMARY KEY, name TEXT NOT NULL, scope TEXT NOT NULL CHECK (scope IN ('local', 'remote'))) STRICT;
CREATE TABLE links (id TEXT PRIMARY KEY, note_id TEXT NOT NULL REFERENCES notes (id), url TEXT NOT NULL) STRICT;
CREATE UNIQUE INDEX links_note_url ON links (note_id, url);
