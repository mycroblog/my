-- SPDX-License-Identifier: AGPL-3.0-only
-- The base schema every other case of this dialect is compared with.
CREATE TABLE notes (id uuid PRIMARY KEY, body text NOT NULL, rating integer, kind text);
CREATE INDEX notes_kind ON notes (kind);
CREATE TABLE tags (id uuid PRIMARY KEY, name text NOT NULL, scope text NOT NULL CHECK (scope IN ('local', 'remote')));
