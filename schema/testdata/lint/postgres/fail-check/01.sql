-- SPDX-License-Identifier: AGPL-3.0-only
-- Adds a CHECK on the existing column rating (not reported by the lint; found by the inspection).
-- want: inspect: table "notes" gets a CHECK
CREATE TABLE notes (id uuid PRIMARY KEY, body text NOT NULL, rating integer, kind text, CHECK (rating BETWEEN 1 AND 5));
CREATE INDEX notes_kind ON notes (kind);
CREATE TABLE tags (id uuid PRIMARY KEY, name text NOT NULL, scope text NOT NULL CHECK (scope IN ('local', 'remote')));
