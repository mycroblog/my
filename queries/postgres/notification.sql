-- SPDX-License-Identifier: AGPL-3.0-only

-- Notifications (event) and their dismissals (event).

-- InsertNotification inserts nothing when the recipient already has a notification with the
-- dedupe key.
-- name: InsertNotification :execrows
INSERT INTO notifications (id, recipient_account_id, type, source_account_id, source_domain, status_id, moderation_action_id, report_id, subject_user_id, severance_id, appeal_id, custom_emoji_request_id, supporter_subscription_id, reaction_emoji, reaction_custom_emoji_id, account_move_id, account_move_outcome, service_closure_event_id, service_closure_change, voice_room_id, error_group_change, error_group_id, error_group_hour, announcement_id, dedupe_key, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26)
ON CONFLICT (recipient_account_id, dedupe_key) DO NOTHING;

-- name: GetNotification :one
SELECT id, recipient_account_id, type, source_account_id, source_domain, status_id, moderation_action_id, report_id, subject_user_id, severance_id, appeal_id, custom_emoji_request_id, supporter_subscription_id, reaction_emoji, reaction_custom_emoji_id, account_move_id, account_move_outcome, service_closure_event_id, service_closure_change, voice_room_id, error_group_change, error_group_id, error_group_hour, announcement_id, dedupe_key, created_at
FROM notifications WHERE id = $1;

-- InsertNotificationDismissal inserts nothing when the notification is already dismissed.
-- name: InsertNotificationDismissal :execrows
INSERT INTO notification_dismissals (notification_id, dismissed_at) VALUES ($1, $2)
ON CONFLICT (notification_id) DO NOTHING;

-- ListNotifications pages the notifications of a recipient that are not dismissed, whose
-- source no exclusion in force at now hides, whose status is not deleted, and whose announcement,
-- if any, is among shown_announcements (the announcements the users see now), newest first; an empty before starts at the
-- newest. With all_types false only the listed types count; hidden_types are left out.
-- name: ListNotifications :many
SELECT n.id, n.recipient_account_id, n.type, n.source_account_id, n.source_domain, n.status_id, n.moderation_action_id, n.report_id,
  n.subject_user_id, n.severance_id, n.appeal_id, n.custom_emoji_request_id, n.supporter_subscription_id, n.reaction_emoji, n.reaction_custom_emoji_id, n.account_move_id, n.account_move_outcome, n.service_closure_event_id, n.service_closure_change, n.voice_room_id, n.error_group_change, n.error_group_id, n.error_group_hour, n.announcement_id, n.dedupe_key, n.created_at
FROM notifications n
WHERE n.recipient_account_id = sqlc.arg(recipient_account_id)
  AND (sqlc.narg(before)::uuid IS NULL OR n.id < sqlc.narg(before)::uuid)
  AND (sqlc.arg(all_types)::boolean OR n.type = ANY(sqlc.arg(types)::text[]))
  AND NOT (n.type = ANY(sqlc.arg(hidden_types)::text[]))
  AND NOT EXISTS (SELECT 1 FROM notification_dismissals d WHERE d.notification_id = n.id)
  AND NOT EXISTS (SELECT 1 FROM notification_exclusions x
    WHERE x.viewer_account_id = n.recipient_account_id AND x.target_account_id = n.source_account_id
      AND (x.expires_at IS NULL OR x.expires_at > sqlc.arg(now)::timestamptz))
  AND NOT EXISTS (SELECT 1 FROM notification_domain_exclusions dx
    WHERE dx.viewer_account_id = n.recipient_account_id
      AND (dx.domain = n.source_domain OR right(n.source_domain, length(dx.domain) + 1) = '.' || dx.domain))
  AND NOT EXISTS (SELECT 1 FROM notification_deleted_statuses s WHERE s.status_id = n.status_id)
  AND (n.announcement_id IS NULL OR n.announcement_id = ANY(sqlc.arg(shown_announcements)::uuid[]))
ORDER BY n.id DESC
LIMIT sqlc.arg(page_limit);

-- ListNotificationsAfter is ListNotifications in the other direction: the notifications whose
-- ID is greater than after, oldest first.
-- name: ListNotificationsAfter :many
SELECT n.id, n.recipient_account_id, n.type, n.source_account_id, n.source_domain, n.status_id, n.moderation_action_id, n.report_id,
  n.subject_user_id, n.severance_id, n.appeal_id, n.custom_emoji_request_id, n.supporter_subscription_id, n.reaction_emoji, n.reaction_custom_emoji_id, n.account_move_id, n.account_move_outcome, n.service_closure_event_id, n.service_closure_change, n.voice_room_id, n.error_group_change, n.error_group_id, n.error_group_hour, n.announcement_id, n.dedupe_key, n.created_at
FROM notifications n
WHERE n.recipient_account_id = sqlc.arg(recipient_account_id)
  AND n.id > sqlc.arg(after)::uuid
  AND (sqlc.arg(all_types)::boolean OR n.type = ANY(sqlc.arg(types)::text[]))
  AND NOT (n.type = ANY(sqlc.arg(hidden_types)::text[]))
  AND NOT EXISTS (SELECT 1 FROM notification_dismissals d WHERE d.notification_id = n.id)
  AND NOT EXISTS (SELECT 1 FROM notification_exclusions x
    WHERE x.viewer_account_id = n.recipient_account_id AND x.target_account_id = n.source_account_id
      AND (x.expires_at IS NULL OR x.expires_at > sqlc.arg(now)::timestamptz))
  AND NOT EXISTS (SELECT 1 FROM notification_domain_exclusions dx
    WHERE dx.viewer_account_id = n.recipient_account_id
      AND (dx.domain = n.source_domain OR right(n.source_domain, length(dx.domain) + 1) = '.' || dx.domain))
  AND NOT EXISTS (SELECT 1 FROM notification_deleted_statuses s WHERE s.status_id = n.status_id)
  AND (n.announcement_id IS NULL OR n.announcement_id = ANY(sqlc.arg(shown_announcements)::uuid[]))
ORDER BY n.id ASC
LIMIT sqlc.arg(page_limit);

-- NotificationListed reports whether ListNotifications would list the notification id of the
-- recipient.
-- name: NotificationListed :one
SELECT EXISTS (SELECT 1 FROM notifications n
WHERE n.id = sqlc.arg(id)::uuid AND n.recipient_account_id = sqlc.arg(recipient_account_id)
  AND NOT (n.type = ANY(sqlc.arg(hidden_types)::text[]))
  AND NOT EXISTS (SELECT 1 FROM notification_dismissals d WHERE d.notification_id = n.id)
  AND NOT EXISTS (SELECT 1 FROM notification_exclusions x
    WHERE x.viewer_account_id = n.recipient_account_id AND x.target_account_id = n.source_account_id
      AND (x.expires_at IS NULL OR x.expires_at > sqlc.arg(now)::timestamptz))
  AND NOT EXISTS (SELECT 1 FROM notification_domain_exclusions dx
    WHERE dx.viewer_account_id = n.recipient_account_id
      AND (dx.domain = n.source_domain OR right(n.source_domain, length(dx.domain) + 1) = '.' || dx.domain))
  AND NOT EXISTS (SELECT 1 FROM notification_deleted_statuses s WHERE s.status_id = n.status_id)
  AND (n.announcement_id IS NULL OR n.announcement_id = ANY(sqlc.arg(shown_announcements)::uuid[])));

-- CountNotificationsAfter counts the notifications of a recipient after the ID after that
-- ListNotifications would return; an empty after counts them all. hidden_types is that of
-- ListNotifications.
-- name: CountNotificationsAfter :one
SELECT COUNT(*) FROM notifications n
WHERE n.recipient_account_id = sqlc.arg(recipient_account_id)
  AND n.id > COALESCE(sqlc.narg(after)::uuid, '00000000-0000-0000-0000-000000000000')
  AND NOT (n.type = ANY(sqlc.arg(hidden_types)::text[]))
  AND NOT EXISTS (SELECT 1 FROM notification_dismissals d WHERE d.notification_id = n.id)
  AND NOT EXISTS (SELECT 1 FROM notification_exclusions x
    WHERE x.viewer_account_id = n.recipient_account_id AND x.target_account_id = n.source_account_id
      AND (x.expires_at IS NULL OR x.expires_at > sqlc.arg(now)::timestamptz))
  AND NOT EXISTS (SELECT 1 FROM notification_domain_exclusions dx
    WHERE dx.viewer_account_id = n.recipient_account_id
      AND (dx.domain = n.source_domain OR right(n.source_domain, length(dx.domain) + 1) = '.' || dx.domain))
  AND NOT EXISTS (SELECT 1 FROM notification_deleted_statuses s WHERE s.status_id = n.status_id)
  AND (n.announcement_id IS NULL OR n.announcement_id = ANY(sqlc.arg(shown_announcements)::uuid[]));

-- Read markers (current value).

-- name: GetNotificationReadMarker :one
SELECT account_id, last_read_id FROM notification_read_markers WHERE account_id = $1;

-- AdvanceNotificationReadMarker moves the marker forward only: it changes nothing when the
-- marker is already at or past last_read_id.
-- name: AdvanceNotificationReadMarker :execrows
INSERT INTO notification_read_markers (account_id, last_read_id) VALUES ($1, $2)
ON CONFLICT (account_id) DO UPDATE SET last_read_id = excluded.last_read_id
WHERE excluded.last_read_id > notification_read_markers.last_read_id;

-- Exclusions (projection of the graph events).

-- UpsertNotificationExclusion replaces the expiry of an existing exclusion.
-- name: UpsertNotificationExclusion :exec
INSERT INTO notification_exclusions (viewer_account_id, target_account_id, reason, expires_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (viewer_account_id, target_account_id, reason) DO UPDATE SET expires_at = excluded.expires_at;

-- name: DeleteNotificationExclusion :exec
DELETE FROM notification_exclusions WHERE viewer_account_id = $1 AND target_account_id = $2 AND reason = $3;

-- Domain exclusions (projection of the graph's domain mutes).

-- name: InsertNotificationDomainExclusion :exec
INSERT INTO notification_domain_exclusions (viewer_account_id, domain) VALUES ($1, $2)
ON CONFLICT (viewer_account_id, domain) DO NOTHING;

-- name: DeleteNotificationDomainExclusion :exec
DELETE FROM notification_domain_exclusions WHERE viewer_account_id = $1 AND domain = $2;

-- DomainExcluded reports whether a domain exclusion of the viewer covers domain: its own or
-- that of one of its parent domains.
-- name: DomainExcluded :one
SELECT EXISTS (SELECT 1 FROM notification_domain_exclusions dx
  WHERE dx.viewer_account_id = sqlc.arg(viewer_account_id)
    AND (dx.domain = sqlc.arg(domain)::text OR right(sqlc.arg(domain)::text, length(dx.domain) + 1) = '.' || dx.domain));

-- Deleted statuses (projection of content.status_deleted).

-- InsertNotificationDeletedStatus inserts nothing when the status is already recorded.
-- name: InsertNotificationDeletedStatus :exec
INSERT INTO notification_deleted_statuses (status_id) VALUES ($1)
ON CONFLICT (status_id) DO NOTHING;

-- Rebuilding the projections (data-model section 9).

-- name: DeleteAllNotificationExclusions :exec
DELETE FROM notification_exclusions;

-- name: DeleteAllNotificationDomainExclusions :exec
DELETE FROM notification_domain_exclusions;

-- name: DeleteAllNotificationDeletedStatuses :exec
DELETE FROM notification_deleted_statuses;

-- ListNotifiedStatusIDs returns every status that a notification, dismissed or not, is about,
-- in order.
-- name: ListNotifiedStatusIDs :many
SELECT DISTINCT status_id FROM notifications WHERE status_id IS NOT NULL ORDER BY status_id;

-- AddErrorGroupNotice adds one notice to the hour and returns the hour's notices with it.
-- name: AddErrorGroupNotice :one
INSERT INTO error_group_notice_hours (hour_start, notices) VALUES ($1, 1)
ON CONFLICT (hour_start) DO UPDATE SET notices = error_group_notice_hours.notices + 1
RETURNING notices;

-- name: GetErrorGroupNotices :one
SELECT COALESCE((SELECT notices FROM error_group_notice_hours WHERE hour_start = $1), 0)::bigint;

-- name: DeleteErrorGroupNoticesBefore :execrows
DELETE FROM error_group_notice_hours WHERE hour_start < $1;

-- ListNotificationSettings returns the channels a local account chose (FR-210).
-- name: ListNotificationSettings :many
SELECT account_id, notification_group, channel, enabled, updated_at FROM notification_settings
WHERE account_id = $1 ORDER BY notification_group, channel;

-- name: UpsertNotificationSetting :exec
INSERT INTO notification_settings (account_id, notification_group, channel, enabled, updated_at) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (account_id, notification_group, channel) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at;
