-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for SQLite (ADR-0023): the tables of the hook module
-- (docs/architecture/data-model/04-tables/16-hook.md). schema/README.md describes the
-- layout and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- hook: the decisions of the decision hooks (data-model §4.16, ADR-0147 §6). Owned by
-- internal/app/hook. No decision holds the text of a post, an email address, or an IP address; an
-- appeal holds the text its author sends. hook.purge_decisions removes the decisions after
-- retention_hook_decisions, with their appeals.
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

-- Resource: an author's appeal of a reject of local.status (ADR-0147 §9), at most one per
-- decision, within appeal_window_days of the decision. It holds the text the author wanted to post,
-- which the decision does not, for the person who reviews it. comment is '' for none. The rows go
-- with their decision, and with their account when it is erased.
CREATE TABLE hook_decision_appeals (
  id TEXT NOT NULL PRIMARY KEY,
  decision_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  status_text TEXT NOT NULL,
  comment TEXT NOT NULL,
  submitted_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX hook_decision_appeals_decision_id ON hook_decision_appeals (decision_id);
CREATE INDEX hook_decision_appeals_account_id ON hook_decision_appeals (account_id);

-- Event: a moderator's decision on an appeal, at most one. An overturned appeal records the
-- cancellation of its decision (hook_decision_overrides) and the exemption of the account in the same
-- transaction. decided_by_user_id is NULL for the CLI.
CREATE TABLE hook_decision_appeal_decisions (
  appeal_id TEXT NOT NULL PRIMARY KEY,
  decision TEXT NOT NULL CHECK (decision IN ('upheld', 'overturned')),
  decided_by_user_id TEXT,
  decided_at INTEGER NOT NULL
) STRICT;

-- Current value: the hook no longer reviews the account, because a moderator overturned its appeal.
-- The row is the value (data-model §1, rule 5): it does not follow the decision or the appeal, which
-- retention removes. Erasing the account removes it.
CREATE TABLE hook_account_exemptions (
  hook TEXT NOT NULL CHECK (hook IN ('local.status')),
  account_id TEXT NOT NULL,
  exempted_at INTEGER NOT NULL,
  PRIMARY KEY (hook, account_id)
) STRICT;
CREATE INDEX hook_account_exemptions_account_id ON hook_account_exemptions (account_id);
