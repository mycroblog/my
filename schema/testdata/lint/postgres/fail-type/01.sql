-- SPDX-License-Identifier: AGPL-3.0-only
-- Widens rating to bigint, which PostgreSQL casts implicitly (not reported by the lint; found by the inspection).
-- want: inspect: the type of column "notes"."rating" changes
CREATE TABLE notes (id uuid PRIMARY KEY, body text NOT NULL, rating bigint, kind text);
CREATE INDEX notes_kind ON notes (kind);
CREATE TABLE tags (id uuid PRIMARY KEY, name text NOT NULL, scope text NOT NULL CHECK (scope IN ('local', 'remote')));
