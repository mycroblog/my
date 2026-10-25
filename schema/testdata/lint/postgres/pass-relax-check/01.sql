-- SPDX-License-Identifier: AGPL-3.0-only
-- Adds a value to the values the CHECK of scope allows.
-- want: the change is additive
CREATE TABLE notes (id uuid PRIMARY KEY, body text NOT NULL, rating integer, kind text);
CREATE INDEX notes_kind ON notes (kind);
CREATE TABLE tags (id uuid PRIMARY KEY, name text NOT NULL, scope text NOT NULL CHECK (scope IN ('local', 'remote', 'both')));
