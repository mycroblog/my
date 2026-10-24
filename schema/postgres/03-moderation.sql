-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for PostgreSQL (ADR-0023): the tables of the moderation module
-- (docs/architecture/data-model/04-tables/03-moderation.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- moderation: actions on accounts and domains, the restrictions they impose, and reports
-- (data-model §4.3, §5.2, docs/architecture/federation.md §11). Owned by internal/app/moderation.
-- ---------------------------------------------------------------------------------------------

-- Event: a moderation action on an account. A ban never expires.
CREATE TABLE moderation_actions (
  id uuid NOT NULL PRIMARY KEY,
  target_account_id uuid NOT NULL,
  kind text NOT NULL CHECK (kind IN (
    'warn', 'mark_statuses_sensitive', 'delete_statuses', 'force_sensitive', 'limit',
    'restrict_posting', 'restrict_interaction', 'disable_login', 'suspend', 'ban'
  )),
  reason_category text NOT NULL CHECK (reason_category IN ('spam', 'legal', 'violation', 'other')),
  public_reason text,
  private_note text,
  report_id uuid,
  issuer text NOT NULL CHECK (issuer IN ('moderator', 'system', 'remote')),
  issued_by_user_id uuid,
  notify_target boolean NOT NULL,
  expires_at timestamptz,
  issued_at timestamptz NOT NULL,
  CHECK (kind <> 'ban' OR expires_at IS NULL),
  CHECK (expires_at IS NULL OR expires_at > issued_at)
);
CREATE INDEX moderation_actions_target_account_id_id ON moderation_actions (target_account_id, id DESC);
CREATE INDEX moderation_actions_report_id ON moderation_actions (report_id);

-- Event: a moderation action on an account was lifted. Expiry is not a lift.
-- lifted_by_user_id is NULL for the system.
CREATE TABLE moderation_action_lifts (
  action_id uuid NOT NULL PRIMARY KEY,
  lifted_by_user_id uuid,
  reason text,
  lifted_at timestamptz NOT NULL
);

-- Projection: the restrictions that the actions on an account impose, one row per restriction
-- and action, until effective_until (NULL: until lifted).
CREATE TABLE account_restrictions (
  account_id uuid NOT NULL,
  restriction text NOT NULL CHECK (restriction IN (
    'login', 'post', 'interact', 'visibility_ceiling_private', 'public_discovery',
    'unsolicited_reach', 'media_sensitive', 'profile_view', 'federation', 'media_fetch',
    'report_intake'
  )),
  source_action_id uuid NOT NULL,
  effective_until timestamptz,
  PRIMARY KEY (account_id, restriction, source_action_id)
);
CREATE INDEX account_restrictions_source_action_id ON account_restrictions (source_action_id);
-- The removal of expired rows, which tells the end of expired suspensions (federation.md §11.1).
CREATE INDEX account_restrictions_effective_until ON account_restrictions (effective_until);

-- Event: a moderation action on a domain and its subdomains. issued_by_user_id is NULL for the
-- CLI.
CREATE TABLE domain_moderation_actions (
  id uuid NOT NULL PRIMARY KEY,
  domain text NOT NULL,
  kind text NOT NULL CHECK (kind IN ('limit', 'suspend', 'reject_media', 'reject_reports', 'force_sensitive')),
  public_comment text,
  private_note text,
  obfuscate boolean NOT NULL,
  issued_by_user_id uuid,
  expires_at timestamptz,
  issued_at timestamptz NOT NULL,
  CHECK (expires_at IS NULL OR expires_at > issued_at)
);
CREATE INDEX domain_moderation_actions_domain_id ON domain_moderation_actions (domain, id DESC);

-- Event: a domain action was lifted. lifted_by_user_id is NULL for the CLI.
CREATE TABLE domain_moderation_action_lifts (
  action_id uuid NOT NULL PRIMARY KEY,
  lifted_by_user_id uuid,
  lifted_at timestamptz NOT NULL
);

-- Projection: the restrictions that the actions on a domain impose on it and its subdomains,
-- until effective_until (NULL: until lifted).
CREATE TABLE domain_restrictions (
  domain text NOT NULL,
  restriction text NOT NULL CHECK (restriction IN (
    'public_discovery', 'unsolicited_reach', 'profile_view', 'federation', 'media_fetch',
    'report_intake', 'media_sensitive'
  )),
  source_action_id uuid NOT NULL,
  effective_until timestamptz,
  PRIMARY KEY (domain, restriction, source_action_id)
);
CREATE INDEX domain_restrictions_source_action_id ON domain_restrictions (source_action_id);

-- Resource: a report of an account. uri is the id of a received Flag (or the URI synthesized
-- for a Flag without one), NULL for a local report.
CREATE TABLE reports (
  id uuid NOT NULL PRIMARY KEY,
  uri text,
  reporter_account_id uuid NOT NULL,
  target_account_id uuid NOT NULL,
  category text NOT NULL CHECK (category IN ('spam', 'legal', 'violation', 'other')),
  comment text NOT NULL,
  forwarded boolean NOT NULL,
  filed_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX reports_uri ON reports (uri);
CREATE INDEX reports_target_account_id_id ON reports (target_account_id, id);
-- A reporter lists their own reports.
CREATE INDEX reports_reporter_account_id_id ON reports (reporter_account_id, id);

-- Resource (child of a report): the statuses a report points at.
CREATE TABLE report_statuses (
  report_id uuid NOT NULL,
  status_id uuid NOT NULL,
  PRIMARY KEY (report_id, status_id)
);

-- Resource (child of a report): the instance rules a local report of the violation category
-- cites. A rule keeps its ID across revisions (instance_rules).
CREATE TABLE report_rule_citations (
  report_id uuid NOT NULL,
  rule_id uuid NOT NULL,
  PRIMARY KEY (report_id, rule_id)
);

-- Projection: whether a report is open, how it was resolved, and who handles it. outcome is
-- that of the latest resolution, set exactly when the report is resolved.
CREATE TABLE report_states (
  report_id uuid NOT NULL PRIMARY KEY,
  state text NOT NULL CHECK (state IN ('open', 'resolved')),
  outcome text CHECK (outcome IN ('actioned', 'dismissed')),
  assignee_user_id uuid,
  since timestamptz NOT NULL,
  CHECK ((state = 'resolved') = (outcome IS NOT NULL))
);
CREATE INDEX report_states_state_report_id ON report_states (state, report_id);

-- Event: a moderator took a report, or gave it up (assignee_user_id NULL).
-- assigned_by_user_id is NULL for the CLI.
CREATE TABLE report_assignments (
  id uuid NOT NULL PRIMARY KEY,
  report_id uuid NOT NULL,
  assignee_user_id uuid,
  assigned_by_user_id uuid,
  assigned_at timestamptz NOT NULL
);
CREATE INDEX report_assignments_report_id_id ON report_assignments (report_id, id DESC);

-- Event: a moderator resolved a report. resolved_by_user_id is NULL for the CLI.
CREATE TABLE report_resolutions (
  id uuid NOT NULL PRIMARY KEY,
  report_id uuid NOT NULL,
  outcome text NOT NULL CHECK (outcome IN ('actioned', 'dismissed')),
  resolved_by_user_id uuid,
  resolved_at timestamptz NOT NULL
);
CREATE INDEX report_resolutions_report_id_id ON report_resolutions (report_id, id DESC);

-- moderation and content: actions on statuses (#364, data-model §4.3, §4.4) -------------------

-- Event child: a status that an action on statuses (delete_statuses, mark_statuses_sensitive)
-- acts on. The status is the target account's.
CREATE TABLE moderation_action_statuses (
  action_id uuid NOT NULL,
  status_id uuid NOT NULL,
  PRIMARY KEY (action_id, status_id)
);

-- moderation: notes on reports and their reopenings (#333, data-model §4.3) --------------------

-- Event: a moderator reopened a resolved report. reopened_by_user_id is NULL for the CLI.
CREATE TABLE report_reopenings (
  id uuid NOT NULL PRIMARY KEY,
  report_id uuid NOT NULL,
  reopened_by_user_id uuid,
  reopened_at timestamptz NOT NULL
);
CREATE INDEX report_reopenings_report_id_id ON report_reopenings (report_id, id DESC);

-- Event: a moderator's internal note on a report, shown to the moderators only.
-- author_user_id is NULL for the CLI.
CREATE TABLE report_notes (
  id uuid NOT NULL PRIMARY KEY,
  report_id uuid NOT NULL,
  author_user_id uuid,
  body text NOT NULL,
  written_at timestamptz NOT NULL
);
CREATE INDEX report_notes_report_id_id ON report_notes (report_id, id);

-- moderation: the audit log (data-model §4.3, ADR-0037). Owned by internal/app/moderation.
-- ---------------------------------------------------------------------------------------------

-- Event: one administrative operation, appended in the transaction of the operation by the
-- moderation module's projector. Never updated; only the retention cleanup deletes rows
-- (ADR-0038). target_label copies the target's display name, so that the entry stays readable
-- after the target is erased. source_event_id is the row of the source event, which holds the
-- reasons and notes. actor_user_id is set exactly for a user actor (the CLI acts as no user).
CREATE TABLE audit_log_entries (
  id uuid NOT NULL PRIMARY KEY,
  actor_kind text NOT NULL CHECK (actor_kind IN ('user', 'cli', 'system', 'remote')),
  actor_user_id uuid,
  action text NOT NULL CHECK (action IN (
    'account_action_issued', 'account_action_lifted', 'domain_action_issued',
    'domain_action_lifted', 'report_assigned', 'report_unassigned', 'report_resolved',
    'report_reopened', 'report_note_added',
    'registration_approved', 'registration_rejected', 'setting_changed', 'text_changed',
    'rule_added', 'rule_revised', 'rule_moved', 'rule_retired', 'role_granted', 'role_revoked',
    'role_created', 'role_updated', 'role_deleted', 'appeal_upheld',
    'appeal_overturned', 'account_deletion_cancelled',
    'revealed_private_data', 'signup_block_added', 'signup_block_lifted', 'two_factor_disabled',
    'password_reset_link_issued',
    'custom_emoji_created', 'custom_emoji_image_replaced', 'custom_emoji_copied', 'custom_emoji_updated',
    'custom_emoji_enabled', 'custom_emoji_disabled', 'custom_emoji_deleted',
    'webhook_created', 'webhook_updated', 'webhook_deleted', 'webhook_enabled', 'webhook_disabled',
    'webhook_secret_rotated', 'announcement_created', 'announcement_updated',
    'announcement_published', 'announcement_withdrawn', 'invite_created', 'invite_revoked',
    'relay_added', 'relay_followed', 'relay_unfollowed', 'relay_removed', 'relay_accepted', 'relay_rejected',
    'status_refetch_queued', 'account_statuses_refetch_queued', 'domain_statuses_refetch_queued',
    'billing_import_queued',
    'cost_item_created', 'cost_item_relabeled', 'cost_item_price_revised', 'cost_item_end_changed', 'cost_item_canceled',
    'supporter_program_enabled', 'supporter_program_disabled', 'supporter_plan_created', 'supporter_plan_revised',
    'supporter_plan_retired', 'supporter_subscription_canceled',
    'custom_emoji_request_submitted', 'custom_emoji_request_withdrawn', 'custom_emoji_request_approved',
    'custom_emoji_request_rejected', 'payment_reconcile_queued',
    'oauth_client_disabled', 'oauth_client_enabled', 'oauth_client_host_denied', 'oauth_client_host_allowed'
  )),
  target_kind text NOT NULL CHECK (target_kind IN (
    'account', 'status', 'domain', 'report', 'appeal', 'user', 'role', 'setting', 'rule',
    'signup_block', 'custom_emoji', 'webhook', 'announcement', 'invite', 'relay', 'billing_source',
    'cost_item', 'supporter_program', 'supporter_plan', 'supporter_subscription', 'custom_emoji_request',
    'payment_provider', 'oauth_client', 'oauth_client_host'
  )),
  target_id text COLLATE "C" NOT NULL,
  target_label text NOT NULL,
  source_event_id uuid,
  occurred_at timestamptz NOT NULL,
  CHECK ((actor_kind = 'user') = (actor_user_id IS NOT NULL))
);
CREATE INDEX audit_log_entries_target_kind_target_id_id ON audit_log_entries (target_kind, target_id, id DESC);
CREATE INDEX audit_log_entries_actor_user_id_id ON audit_log_entries (actor_user_id, id DESC);

-- ---------------------------------------------------------------------------------------------
-- moderation: the follows that suspensions cut (data-model §4.3, FR-717, ADR-0036). Owned by
-- internal/app/moderation.
-- ---------------------------------------------------------------------------------------------

-- Event: the active follows that one local account lost when a domain (domain_action_id) or a
-- remote account (action_id) was suspended; exactly one of the two is set. One row per account
-- and action, so that processing the same action again records and notifies nothing more.
CREATE TABLE relationship_severances (
  id uuid NOT NULL PRIMARY KEY,
  domain_action_id uuid,
  action_id uuid,
  account_id uuid NOT NULL,
  followers_lost integer NOT NULL CHECK (followers_lost >= 0),
  following_lost integer NOT NULL CHECK (following_lost >= 0),
  occurred_at timestamptz NOT NULL,
  CONSTRAINT relationship_severances_action_check CHECK ((domain_action_id IS NULL) <> (action_id IS NULL))
);
CREATE INDEX relationship_severances_account_id_id ON relationship_severances (account_id, id DESC);
CREATE UNIQUE INDEX relationship_severances_domain_action_id_account_id ON relationship_severances (domain_action_id, account_id);
CREATE UNIQUE INDEX relationship_severances_action_id_account_id ON relationship_severances (action_id, account_id);

-- ---------------------------------------------------------------------------------------------
-- moderation: appeals of actions on local accounts (#337, data-model §4.3, FR-707, ADR-0038).
-- Owned by internal/app/moderation.
-- ---------------------------------------------------------------------------------------------

-- Resource: a local account's appeal of an action on it, at most one per action, within the
-- appeal window (instance setting appeal_window) from the action.
CREATE TABLE moderation_appeals (
  id uuid NOT NULL PRIMARY KEY,
  action_id uuid NOT NULL,
  account_id uuid NOT NULL,
  body text NOT NULL,
  submitted_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX moderation_appeals_action_id ON moderation_appeals (action_id);

-- Event: a moderator's decision on an appeal, at most one. An overturned appeal records the
-- lift of its action in the same transaction. decided_by_user_id is NULL for the CLI.
CREATE TABLE moderation_appeal_decisions (
  appeal_id uuid NOT NULL PRIMARY KEY,
  decision text NOT NULL CHECK (decision IN ('upheld', 'overturned')),
  public_reason text,
  decided_by_user_id uuid,
  decided_at timestamptz NOT NULL
);

-- Projection: where each appeal stands, for the queue of pending appeals.
CREATE TABLE appeal_states (
  appeal_id uuid NOT NULL PRIMARY KEY,
  state text NOT NULL CHECK (state IN ('pending', 'upheld', 'overturned')),
  since timestamptz NOT NULL
);
CREATE INDEX appeal_states_state_appeal_id ON appeal_states (state, appeal_id);

-- moderation: the sign-up block list (data-model §4.3, FR-719). Owned by internal/app/moderation.
-- ---------------------------------------------------------------------------------------------

-- Event: a block on sign-ups from the registration page. value is an IDNA A-label domain
-- (email_domain, which covers its subdomains), the hex SHA-256 of a canonical email address
-- (email_hash), or a masked CIDR prefix (ip_range). moderation_action_id is the ban the block
-- was made from; created_by_user_id is NULL for the CLI. A block is in effect until it expires
-- or is lifted.
CREATE TABLE signup_blocks (
  id uuid NOT NULL PRIMARY KEY,
  kind text NOT NULL CHECK (kind IN ('email_domain', 'email_hash', 'ip_range')),
  value text COLLATE "C" NOT NULL,
  severity text NOT NULL CHECK (severity IN ('reject', 'require_approval')),
  comment text,
  moderation_action_id uuid,
  created_by_user_id uuid,
  expires_at timestamptz,
  created_at timestamptz NOT NULL,
  CHECK (expires_at IS NULL OR expires_at > created_at)
);
CREATE INDEX signup_blocks_kind_value ON signup_blocks (kind, value);

-- Event: a sign-up block was lifted. Expiry is not a lift. lifted_by_user_id is NULL for the
-- CLI.
CREATE TABLE signup_block_lifts (
  block_id uuid NOT NULL PRIMARY KEY,
  lifted_by_user_id uuid,
  lifted_at timestamptz NOT NULL
);

-- ---------------------------------------------------------------------------------------------
-- moderation: retention of the moderation records and the holds that stop it (#349,
-- data-model §4.3, §8, ADR-0038). Owned by internal/app/moderation.
-- ---------------------------------------------------------------------------------------------

-- Event: an operator's hold on the erasure of an account's records (a court order to preserve
-- them, a disclosure request in progress). created_by_user_id is NULL for the CLI.
CREATE TABLE retention_holds (
  id uuid NOT NULL PRIMARY KEY,
  account_id uuid NOT NULL,
  reason text NOT NULL,
  created_by_user_id uuid,
  created_at timestamptz NOT NULL
);
CREATE INDEX retention_holds_account_id ON retention_holds (account_id);

-- Event: a hold was released, at most once. released_by_user_id is NULL for the CLI.
CREATE TABLE retention_hold_releases (
  hold_id uuid NOT NULL PRIMARY KEY,
  released_by_user_id uuid,
  released_at timestamptz NOT NULL
);

-- The purge of remote statuses keeps the statuses that a report or an action points at.
CREATE INDEX report_statuses_status_id ON report_statuses (status_id);
CREATE INDEX moderation_action_statuses_status_id ON moderation_action_statuses (status_id);
