-- SPDX-License-Identifier: AGPL-3.0-only
-- Drops the table tags (DS102).
-- want: lint DS102
CREATE TABLE notes (id uuid PRIMARY KEY, body text NOT NULL, rating integer, kind text);
CREATE INDEX notes_kind ON notes (kind);
