-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The search module: the progress of a rebuild of the full-text search index (ADR-0149 section 10,
-- data-model section 4.18). With search = none nothing writes to these tables.

-- Current value: the latest rebuild, one row once a rebuild has started. It is progress, with no
-- business meaning as history: the next rebuild replaces it, and my search clear deletes it.
-- after_status_id is the cursor of the walk, the last status handled, empty before the first.
CREATE TABLE search_rebuild (
  id INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
  rebuild_id TEXT NOT NULL,
  mode TEXT NOT NULL CHECK (mode IN ('clear', 'keep')),
  phase TEXT NOT NULL CHECK (phase IN ('clearing', 'filling', 'finished')),
  after_status_id TEXT NOT NULL,
  rate INTEGER NOT NULL CHECK (rate > 0),
  processed INTEGER NOT NULL CHECK (processed >= 0),
  failed INTEGER NOT NULL CHECK (failed >= 0),
  last_error TEXT NOT NULL,
  started_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
) STRICT;

-- Current value (child): the statuses the rebuild could not write, up to a limit. It holds IDs and
-- the error, never the text of a status.
CREATE TABLE search_rebuild_failures (
  rebuild_id TEXT NOT NULL,
  status_id TEXT NOT NULL,
  message TEXT NOT NULL,
  failed_at INTEGER NOT NULL,
  PRIMARY KEY (rebuild_id, status_id)
) STRICT;
