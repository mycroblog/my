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
    'voice_room_started', 'voice_room_invited', 'admin_error_group', 'admin_hook_failing', 'admin_domain_list_failing', 'status', 'announcement',
    'admin_hook_appeal', 'hook_appeal_decided', 'auto_delete_limit', 'scheduled_status_failed',
    'reply_request', 'boost_request', 'quote_request', 'interaction_accepted', 'interaction_rejected'
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
  -- announcement: the announcements row.
  announcement_id TEXT,
  -- admin_hook_appeal, hook_appeal_decided: the hook_decision_appeals row.
  hook_appeal_id TEXT,
  -- auto_delete_limit: the status_auto_delete_limit_notices row.
  auto_delete_limit_notice_id TEXT,
  -- scheduled_status_failed: the draft_schedules row of the schedule that could not be published.
  draft_schedule_id TEXT,
  dedupe_key TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  -- nobody is notified of their own actions.
  CHECK (source_account_id IS NULL OR source_account_id <> recipient_account_id)
) STRICT;
CREATE UNIQUE INDEX notifications_recipient_account_id_dedupe_key ON notifications (recipient_account_id, dedupe_key);
CREATE INDEX notifications_recipient_account_id_id ON notifications (recipient_account_id, id);
CREATE INDEX notifications_recipient_account_id_type_id ON notifications (recipient_account_id, type, id);
-- The retention purge keeps a remote status that a notification is about (ADR-0006).
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
    'voice_room_started', 'voice_room_invited', 'admin_error_group', 'admin_hook_failing', 'admin_domain_list_failing', 'status', 'announcement',
    'admin_hook_appeal', 'hook_appeal_decided', 'auto_delete_limit', 'scheduled_status_failed',
    'reply_request', 'boost_request', 'quote_request', 'interaction_accepted', 'interaction_rejected'
  )),
  PRIMARY KEY (subscription_id, notification_type)
) STRICT;

-- Current value: a local account's choice of a channel of a group of notification types (FR-210,
-- ADR-0086 §9, §13). A row exists once the user chose; until then the group's default applies
-- (notification.Groups). Only the channels the user can choose have rows: the instance's notices
-- are always in the app, and the service closure and supporter emails always sent. The group of
-- an email is also the category of its unsubscribe link, which turns its email row off.
CREATE TABLE notification_settings (
  account_id TEXT NOT NULL,
  notification_group TEXT NOT NULL CHECK (notification_group IN (
    'mention', 'follow', 'follow_request', 'follow_accepted', 'boost', 'favourite', 'reaction', 'quote', 'poll_closed',
    'account_moved', 'voice',
    'admin_report', 'admin_sign_up', 'admin_appeal', 'admin_webhook_disabled', 'admin_custom_emoji_request',
    'admin_account_moved', 'admin_service_closure_due', 'admin_error_group', 'admin_hook_failing', 'admin_hook_appeal', 'admin_domain_list_failing',
    'moderation', 'status', 'announcement', 'auto_delete_limit', 'interaction_request',
    'scheduled_status_failed'
  )),
  channel TEXT NOT NULL CHECK (channel IN ('in_app', 'email')),
  enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, notification_group, channel),
  CHECK (notification_group <> 'moderation' OR channel = 'email'),
  CHECK (notification_group NOT IN ('status', 'announcement') OR channel = 'in_app')
) STRICT;

-- Current value: a local account's subscription to the new posts of another account (ADR-0086
-- §1): each new post of the target that the subscriber may see notifies the subscriber
-- (status). A local target needs no follow; a remote one only while the subscriber's follow of it
-- is active. The target is never told. At most 1,000 per subscriber.
CREATE TABLE post_subscriptions (
  account_id TEXT NOT NULL,
  target_account_id TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, target_account_id),
  CHECK (account_id <> target_account_id)
) STRICT;
-- The list of a subscriber's subscriptions, newest first.
CREATE INDEX post_subscriptions_account_created_at_target ON post_subscriptions (account_id, created_at, target_account_id);
-- The fan-out of a new post to the subscribers of its author, in order of the subscriber.
CREATE INDEX post_subscriptions_target_account ON post_subscriptions (target_account_id, account_id);

-- Current value: a subscription asked for with a follow of a remote account that awaits its
-- server's Accept (ADR-0086 §14). The acceptance turns it into a post_subscriptions row; the end
-- or the rejection of the follow deletes it.
CREATE TABLE post_subscription_requests (
  account_id TEXT NOT NULL,
  target_account_id TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, target_account_id)
) STRICT;
CREATE INDEX post_subscription_requests_target_account ON post_subscription_requests (target_account_id, account_id);

-- The erasure of deleted local accounts' data (account, #397, data-model §4.2, §8) looks up
-- the rows of an account.
CREATE INDEX notifications_source_account_id ON notifications (source_account_id);
CREATE INDEX notifications_subject_user_id ON notifications (subject_user_id);

CREATE INDEX notification_exclusions_target_account_id ON notification_exclusions (target_account_id);

-- Thread mutes (ADR-0158 §8, §9).
-- Current value: a local account's mute of a conversation (content's conversations, ADR-0154),
-- which keeps the mention, boost, favourite, reaction, quote, and status notifications about the
-- statuses of the conversation from being created; timelines still show them. The merge of a
-- conversation (content.conversation_merged) moves its rows to the conversation it was merged
-- into, keeping one row when the account muted both.
CREATE TABLE conversation_mutes (
  account_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, conversation_id)
) STRICT;
-- The accounts that mute a conversation: the check of the recipients of a notification, and the
-- merge.
CREATE INDEX conversation_mutes_conversation_account ON conversation_mutes (conversation_id, account_id);

-- ---------------------------------------------------------------------------------------------
-- The notification policy and the requests of the filtered notifications (ADR-0158 §1-§7,
-- §9, data-model §4.7).
-- ---------------------------------------------------------------------------------------------

-- Current value: a local account's action for the notifications whose source meets a condition.
-- A condition without a row accepts.
CREATE TABLE notification_policies (
  account_id TEXT NOT NULL,
  policy_condition TEXT NOT NULL CHECK (policy_condition IN (
    'not_following', 'not_follower', 'new_account', 'unsolicited_private_mention', 'limited_account'
  )),
  policy_action TEXT NOT NULL CHECK (policy_action IN ('accept', 'filter', 'drop')),
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, policy_condition)
) STRICT;

-- Event (subtype of notifications): the policy filtered the notification when it was created.
-- It is listed only once its recipient accepts its source and no dismissal of the source's
-- request covers it; until then it waits in the request. Never changed after it is created.
CREATE TABLE notification_filterings (
  notification_id TEXT NOT NULL PRIMARY KEY,
  recipient_account_id TEXT NOT NULL,
  source_account_id TEXT NOT NULL
) STRICT;
CREATE INDEX notification_filterings_recipient_source ON notification_filterings (recipient_account_id, source_account_id, notification_id);
CREATE INDEX notification_filterings_source_account_id ON notification_filterings (source_account_id);

-- Resource: a source whose request the recipient accepted. Its filtered notifications are
-- listed, and the policy accepts its later ones.
CREATE TABLE notification_accepted_sources (
  recipient_account_id TEXT NOT NULL,
  source_account_id TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (recipient_account_id, source_account_id),
  CHECK (recipient_account_id <> source_account_id)
) STRICT;
CREATE INDEX notification_accepted_sources_source_account_id ON notification_accepted_sources (source_account_id);

-- Event: the recipient dismissed the request of a source. The filtered notifications up to and
-- including last_notification_id leave the request and are never listed.
CREATE TABLE notification_request_dismissals (
  id TEXT NOT NULL PRIMARY KEY,
  recipient_account_id TEXT NOT NULL,
  source_account_id TEXT NOT NULL,
  last_notification_id TEXT NOT NULL,
  dismissed_at INTEGER NOT NULL
) STRICT;
CREATE INDEX notification_request_dismissals_recipient_source ON notification_request_dismissals (recipient_account_id, source_account_id, last_notification_id);
CREATE INDEX notification_request_dismissals_source_account_id ON notification_request_dismissals (source_account_id);

-- Projection of notification_filterings, notification_accepted_sources, and
-- notification_request_dismissals: the filtered notifications of each source that wait for the
-- recipient, neither accepted nor dismissed.
CREATE TABLE notification_requests (
  recipient_account_id TEXT NOT NULL,
  source_account_id TEXT NOT NULL,
  notifications_count INTEGER NOT NULL CHECK (notifications_count > 0),
  last_notification_id TEXT NOT NULL,
  PRIMARY KEY (recipient_account_id, source_account_id)
) STRICT;
CREATE INDEX notification_requests_recipient_last ON notification_requests (recipient_account_id, last_notification_id);
CREATE INDEX notification_requests_source_account_id ON notification_requests (source_account_id);
