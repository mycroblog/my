-- SPDX-License-Identifier: AGPL-3.0-only
-- Drops the table tags (DS102).
-- want: lint DS102
CREATE TABLE notes (id TEXT PRIMARY KEY, body TEXT NOT NULL, rating INTEGER, kind TEXT) STRICT;
CREATE INDEX notes_kind ON notes (kind);
