-- SPDX-License-Identifier: AGPL-3.0-only
-- Adds a value to the values the CHECK of scope allows, written over lines.
-- want: the change is additive
CREATE TABLE notes (id TEXT PRIMARY KEY, body TEXT NOT NULL, rating INTEGER, kind TEXT) STRICT;
CREATE INDEX notes_kind ON notes (kind);
CREATE TABLE tags (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  scope TEXT NOT NULL CHECK (scope IN (
    'local', 'remote',
    'both'
  ))
) STRICT;
