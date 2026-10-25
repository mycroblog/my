-- SPDX-License-Identifier: AGPL-3.0-only
-- A table, a nullable column, a column with a DEFAULT and a CHECK on itself, and an index.
-- want: the change is additive
CREATE TABLE notes (id uuid PRIMARY KEY, body text NOT NULL, rating integer, kind text, summary text, state text NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'closed')));
CREATE INDEX notes_kind ON notes (kind);
CREATE INDEX notes_rating ON notes (rating);
CREATE TABLE tags (id uuid PRIMARY KEY, name text NOT NULL, scope text NOT NULL CHECK (scope IN ('local', 'remote')));
CREATE TABLE links (id uuid PRIMARY KEY, note_id uuid NOT NULL REFERENCES notes (id), url text NOT NULL);
CREATE UNIQUE INDEX links_note_url ON links (note_id, url);
