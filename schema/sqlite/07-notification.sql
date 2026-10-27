-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for SQLite (ADR-0023): the tables of the notification module
-- (docs/architecture/data-model/04-tables/07-notification.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- notification: notifications, their dismissals, read markers, the exclusions and deleted
-- statuses that hide them, and the Web Push subscriptions (data-model §4.7, FR-205, FR-206).
-- Owned by internal/app/notification.
-- ---------------------------------------------------------------------------------------------

-- Event: something happened to a local account. dedupe_key makes generating it again from the
-- same domain event, or from an action the source repeats, insert nothing.
CREATE TABLE notifications (
  id TEXT NOT NULL PRIMARY KEY,
  recipient_account_id TEXT NOT NULL,
  type TEXT NOT NULL CHECK (type IN (
    'follow', 'follow_request', 'follow_accepted', 'mention', 'boost', 'favourite', 'reaction', 'quote', 'poll_closed',
    'moderation_notice',
    'report_resolved', 'admin_report', 'admin_sign_up', 'relationships_severed', 'appeal_decided',
    'admin_appeal', 'admin_webhook_disabled', 'admin_custom_emoji_request', 'custom_emoji_request_decided',
    'supporter_payment_failed', 'supporter_subscription_ended', 'supporter_plan_retired',
    'account_moved', 'admin_account_moved',
    'service_closure', 'admin_service_closure_due', 'supporter_service_closure',
    'voice_room_started', 'voice_room_invited', 'admin_error_group'
  )),
  source_account_id TEXT,
  -- the domain of a remote source account; NULL for a local source or none.
  source_domain TEXT,
  status_id TEXT,
  moderation_action_id TEXT,
  -- admin_report, report_resolved: the report.
  report_id TEXT,
  -- admin_sign_up: the user whose registration awaits approval.
  subject_user_id TEXT,
  -- relationships_severed: the relationship_severances row.
  severance_id TEXT,
  -- appeal_decided, admin_appeal: the appeal.
  appeal_id TEXT,
  -- admin_custom_emoji_request, custom_emoji_request_decided: the custom_emoji_requests row.
  custom_emoji_request_id TEXT,
  -- supporter_payment_failed, supporter_subscription_ended, supporter_plan_retired: the
  -- supporter_subscriptions row.
  supporter_subscription_id TEXT,
  -- reaction: the emoji, a Unicode emoji or the custom emoji (custom_emojis.id).
  reaction_emoji TEXT,
  reaction_custom_emoji_id TEXT,
  -- account_moved, admin_account_moved: the account_moves row of the move.
  account_move_id TEXT,
  -- account_moved: what became of the recipient's follow of the moved account.
  account_move_outcome TEXT CHECK (account_move_outcome IN ('followed', 'requested', 'blocking', 'blocked_by', 'domain_muted')),
  -- service_closure, admin_service_closure_due: the service_closure_events row that it tells of
  -- (the announcement whose closing date has passed, for admin_service_closure_due);
  -- supporter_service_closure: the announcement that cancels the subscription.
  service_closure_event_id TEXT,
  -- service_closure: what the event did to the closure.
  service_closure_change TEXT CHECK (service_closure_change IN ('announced', 'rescheduled', 'withdrawn')),
  -- voice_room_started, voice_room_invited: the voice_rooms row.
  voice_room_id TEXT,
  -- admin_error_group: what it tells of; the error_groups row of created and regressed; the
  -- hour (its start) that others folds the groups of beyond the hourly cap.
  error_group_change TEXT CHECK (error_group_change IN ('created', 'regressed', 'others')),
  error_group_id TEXT,
  error_group_hour INTEGER,
  dedupe_key TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  -- nobody is notified of their own actions.
  CHECK (source_account_id IS NULL OR source_account_id <> recipient_account_id)
) STRICT;
CREATE UNIQUE INDEX notifications_recipient_account_id_dedupe_key ON notifications (recipient_account_id, dedupe_key);
CREATE INDEX notifications_recipient_account_id_id ON notifications (recipient_account_id, id);
CREATE INDEX notifications_recipient_account_id_type_id ON notifications (recipient_account_id, type, id);
-- The retention purge keeps a remote status that a notification is about (ADR-0034).
CREATE INDEX notifications_status_id ON notifications (status_id);

-- Current value: how many admin_error_group notices the instance made in the hour that starts
-- at hour_start (ADR-0103 §8). Every new or regressed group that notifies adds one in the
-- transaction of its event; the first 10 of the hour notify of their groups, the 11th makes the
-- notification that folds the rest, whose count is notices - 10.
CREATE TABLE error_group_notice_hours (
  hour_start INTEGER NOT NULL PRIMARY KEY,
  notices INTEGER NOT NULL CHECK (notices > 0)
) STRICT;

-- Event: the recipient dismissed a notification. A notification is dismissed at most once.
CREATE TABLE notification_dismissals (
  notification_id TEXT NOT NULL PRIMARY KEY,
  dismissed_at INTEGER NOT NULL
) STRICT;

-- Current value: the newest notification an account has read; greater IDs are unread.
CREATE TABLE notification_read_markers (
  account_id TEXT NOT NULL PRIMARY KEY,
  last_read_id TEXT NOT NULL
) STRICT;

-- Projection of the graph events: while in force (expires_at NULL or later than now), the
-- notifications from target_account_id are hidden from viewer_account_id at read time.
CREATE TABLE notification_exclusions (
  viewer_account_id TEXT NOT NULL,
  target_account_id TEXT NOT NULL,
  reason TEXT NOT NULL CHECK (reason IN ('block', 'blocked_by', 'mute')),
  expires_at INTEGER,
  PRIMARY KEY (viewer_account_id, target_account_id, reason)
) STRICT;

-- Projection of the graph events: the notifications whose source_domain is the domain or one
-- of its subdomains are hidden from viewer_account_id at read time (FR-304).
CREATE TABLE notification_domain_exclusions (
  viewer_account_id TEXT NOT NULL,
  domain TEXT NOT NULL,
  PRIMARY KEY (viewer_account_id, domain)
) STRICT;

-- Projection of content.status_deleted: the notifications about these statuses are hidden at
-- read time; the erasure job deletes them and the row.
CREATE TABLE notification_deleted_statuses (
  status_id TEXT NOT NULL PRIMARY KEY
) STRICT;

-- Current value: where the Web Push notifications of a local account go (FR-206). One per OAuth
-- grant (the token family that registered it) and per push service endpoint; registering
-- either again replaces the row. key_p256dh and key_auth are the browser's keys in unpadded
-- base64url (RFC 8291 §3).
CREATE TABLE push_subscriptions (
  id TEXT NOT NULL PRIMARY KEY,
  account_id TEXT NOT NULL,
  oauth_token_family_id TEXT NOT NULL,
  endpoint TEXT NOT NULL,
  key_p256dh TEXT NOT NULL,
  key_auth TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX push_subscriptions_endpoint ON push_subscriptions (endpoint);
CREATE UNIQUE INDEX push_subscriptions_oauth_token_family_id ON push_subscriptions (oauth_token_family_id);
CREATE INDEX push_subscriptions_account_id ON push_subscriptions (account_id);

-- Current value: the notification types pushed to a subscription.
CREATE TABLE push_subscription_alerts (
  subscription_id TEXT NOT NULL,
  notification_type TEXT NOT NULL CHECK (notification_type IN (
    'follow', 'follow_request', 'follow_accepted', 'mention', 'boost', 'favourite', 'reaction', 'quote', 'poll_closed',
    'moderation_notice',
    'report_resolved', 'admin_report', 'admin_sign_up', 'relationships_severed', 'appeal_decided',
    'admin_appeal', 'admin_webhook_disabled', 'admin_custom_emoji_request', 'custom_emoji_request_decided',
    'supporter_payment_failed', 'supporter_subscription_ended', 'supporter_plan_retired',
    'account_moved', 'admin_account_moved',
    'service_closure', 'admin_service_closure_due', 'supporter_service_closure',
    'voice_room_started', 'voice_room_invited', 'admin_error_group'
  )),
  PRIMARY KEY (subscription_id, notification_type)
) STRICT;

-- The erasure of deleted local accounts' data (account, #397, data-model §4.2, §8) looks up
-- the rows of an account.
CREATE INDEX notifications_source_account_id ON notifications (source_account_id);
CREATE INDEX notifications_subject_user_id ON notifications (subject_user_id);

CREATE INDEX notification_exclusions_target_account_id ON notification_exclusions (target_account_id);
