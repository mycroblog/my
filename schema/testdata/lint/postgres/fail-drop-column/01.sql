-- SPDX-License-Identifier: AGPL-3.0-only
-- Drops the column kind (DS103).
-- want: lint DS103
CREATE TABLE notes (id uuid PRIMARY KEY, body text NOT NULL, rating integer);
CREATE TABLE tags (id uuid PRIMARY KEY, name text NOT NULL, scope text NOT NULL CHECK (scope IN ('local', 'remote')));
