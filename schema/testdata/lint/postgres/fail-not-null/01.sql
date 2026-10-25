-- SPDX-License-Identifier: AGPL-3.0-only
-- Makes the existing column rating NOT NULL (a warning of the lint).
-- want: lint MF104
CREATE TABLE notes (id uuid PRIMARY KEY, body text NOT NULL, rating integer NOT NULL, kind text);
CREATE INDEX notes_kind ON notes (kind);
CREATE TABLE tags (id uuid PRIMARY KEY, name text NOT NULL, scope text NOT NULL CHECK (scope IN ('local', 'remote')));
