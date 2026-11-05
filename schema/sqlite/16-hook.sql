-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for SQLite (ADR-0023): the tables of the hook module
-- (docs/architecture/data-model/04-tables/16-hook.md). schema/README.md describes the
-- layout and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- hook: the decisions of the decision hooks (data-model §4.16, ADR-0147 §6). Owned by
-- internal/app/hook. No column holds the text of a post, an email address, or an IP address, and
-- hook.purge_decisions removes the rows after retention_hook_decisions.
-- ---------------------------------------------------------------------------------------------

-- Event: what a hook answered about one subject, one row per hook and subject, so that running a
-- job again reads the row and does not call the plugin. outcome is a verdict, or the kind of a
-- failure (timeout, error, invalid, breaker_open). subject_key is what identifies the subject for
-- the hook, such as the URI of a received activity. account_id is the account the subject belongs
-- to, which holds and erasures go by, NULL when there is none. reason_code and note are '' for none.
CREATE TABLE hook_decisions (
  id TEXT NOT NULL PRIMARY KEY,
  hook TEXT NOT NULL CHECK (hook IN ('inbound.status', 'inbound.follow', 'signup.check', 'media.check', 'local.status')),
  subject_key TEXT NOT NULL,
  account_id TEXT,
  mode TEXT NOT NULL CHECK (mode IN ('observe', 'enforce')),
  outcome TEXT NOT NULL CHECK (outcome IN ('allow', 'drop', 'flag', 'mark_sensitive', 'require_approval', 'reject', 'timeout', 'error', 'invalid', 'breaker_open')),
  reason_code TEXT NOT NULL CHECK (reason_code IN ('', 'spam', 'harassment', 'sexual', 'violence', 'illegal', 'malware', 'other')),
  note TEXT NOT NULL,
  latency_ms INTEGER NOT NULL CHECK (latency_ms >= 0),
  decided_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX hook_decisions_hook_subject_key ON hook_decisions (hook, subject_key);
CREATE INDEX hook_decisions_decided_at_id ON hook_decisions (decided_at, id);
CREATE INDEX hook_decisions_account_id ON hook_decisions (account_id);

-- Event child: a status that a decision had an effect on, the source of the projection
-- status_sensitivity_overrides (ADR-0147 §8), written in the transaction that knows the status.
CREATE TABLE hook_decision_statuses (
  decision_id TEXT NOT NULL,
  status_id TEXT NOT NULL,
  PRIMARY KEY (decision_id, status_id)
) STRICT;

-- Event: a moderator cancelled a decision (ADR-0147 §9). A decision is cancelled once.
CREATE TABLE hook_decision_overrides (
  decision_id TEXT NOT NULL PRIMARY KEY,
  overridden_by_user_id TEXT,
  overridden_at INTEGER NOT NULL
) STRICT;
