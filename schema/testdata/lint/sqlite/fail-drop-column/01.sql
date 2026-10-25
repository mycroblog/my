-- SPDX-License-Identifier: AGPL-3.0-only
-- Drops the column kind (DS103).
-- want: lint DS103
CREATE TABLE notes (id TEXT PRIMARY KEY, body TEXT NOT NULL, rating INTEGER) STRICT;
CREATE TABLE tags (id TEXT PRIMARY KEY, name TEXT NOT NULL, scope TEXT NOT NULL CHECK (scope IN ('local', 'remote'))) STRICT;
