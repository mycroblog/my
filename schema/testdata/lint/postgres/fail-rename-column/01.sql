-- SPDX-License-Identifier: AGPL-3.0-only
-- Renames kind to category: a drop and an add (DS103).
-- want: lint DS103
CREATE TABLE notes (id uuid PRIMARY KEY, body text NOT NULL, rating integer, category text);
CREATE INDEX notes_kind ON notes (category);
CREATE TABLE tags (id uuid PRIMARY KEY, name text NOT NULL, scope text NOT NULL CHECK (scope IN ('local', 'remote')));
