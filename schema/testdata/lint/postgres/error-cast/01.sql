-- SPDX-License-Identifier: AGPL-3.0-only
-- Changes kind to uuid, which PostgreSQL cannot cast: the change does not apply, approved or not.
-- want: does not apply
CREATE TABLE notes (id uuid PRIMARY KEY, body text NOT NULL, rating integer, kind uuid);
CREATE INDEX notes_kind ON notes (kind);
CREATE TABLE tags (id uuid PRIMARY KEY, name text NOT NULL, scope text NOT NULL CHECK (scope IN ('local', 'remote')));
