-- SPDX-License-Identifier: AGPL-3.0-only
-- Adds a unique index on the existing column name (a warning of the lint).
-- want: lint MF101
CREATE TABLE notes (id uuid PRIMARY KEY, body text NOT NULL, rating integer, kind text);
CREATE INDEX notes_kind ON notes (kind);
CREATE TABLE tags (id uuid PRIMARY KEY, name text NOT NULL, scope text NOT NULL CHECK (scope IN ('local', 'remote')));
CREATE UNIQUE INDEX tags_name ON tags (name);
