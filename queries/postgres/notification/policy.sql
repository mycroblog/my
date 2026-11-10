-- SPDX-License-Identifier: AGPL-3.0-only

-- The notification policy (current value), the filtered notifications (event), the accepted
-- sources (resource), the dismissals of the requests (event), and the requests (projection)
-- (ADR-0158, data-model 4.7). A list of pairs is two arrays of the same length, the recipients
-- and the sources.

-- name: ListNotificationPolicies :many
SELECT account_id, policy_condition, policy_action, updated_at FROM notification_policies
WHERE account_id = $1 ORDER BY policy_condition;

-- name: ListNotificationPoliciesOf :many
SELECT account_id, policy_condition, policy_action, updated_at FROM notification_policies
WHERE account_id = ANY(sqlc.arg(account_ids)::uuid[]) ORDER BY account_id, policy_condition;

-- name: UpsertNotificationPolicy :exec
INSERT INTO notification_policies (account_id, policy_condition, policy_action, updated_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (account_id, policy_condition) DO UPDATE SET policy_action = excluded.policy_action, updated_at = excluded.updated_at;

-- InsertNotificationFilterings records the filtering of each notification (the unnests of a
-- select list advance together).
-- name: InsertNotificationFilterings :exec
INSERT INTO notification_filterings (notification_id, recipient_account_id, source_account_id)
SELECT unnest(sqlc.arg(notification_ids)::uuid[]), unnest(sqlc.arg(recipient_account_ids)::uuid[]), unnest(sqlc.arg(source_account_ids)::uuid[])
ON CONFLICT (notification_id) DO NOTHING;

-- name: NotificationSourceAccepted :one
SELECT EXISTS (SELECT 1 FROM notification_accepted_sources WHERE recipient_account_id = $1 AND source_account_id = $2);

-- InsertNotificationAcceptedSource inserts nothing when the recipient already accepted the source.
-- name: InsertNotificationAcceptedSource :exec
INSERT INTO notification_accepted_sources (recipient_account_id, source_account_id, created_at) VALUES ($1, $2, $3)
ON CONFLICT (recipient_account_id, source_account_id) DO NOTHING;

-- name: InsertNotificationRequestDismissal :exec
INSERT INTO notification_request_dismissals (id, recipient_account_id, source_account_id, last_notification_id, dismissed_at)
VALUES ($1, $2, $3, $4, $5);

-- DeleteNotificationRequestsOf and InsertNotificationRequestsOf derive the requests of the
-- pairs again: the filtered notifications of each that its recipient has neither accepted nor
-- dismissed.
-- name: DeleteNotificationRequestsOf :exec
DELETE FROM notification_requests WHERE (recipient_account_id, source_account_id) IN (
  SELECT unnest(sqlc.arg(recipient_account_ids)::uuid[]), unnest(sqlc.arg(source_account_ids)::uuid[]));

-- name: InsertNotificationRequestsOf :exec
INSERT INTO notification_requests (recipient_account_id, source_account_id, notifications_count, last_notification_id)
SELECT f.recipient_account_id, f.source_account_id, COUNT(*), MAX(f.notification_id::text)::uuid FROM notification_filterings f
WHERE (f.recipient_account_id, f.source_account_id) IN (
    SELECT unnest(sqlc.arg(recipient_account_ids)::uuid[]), unnest(sqlc.arg(source_account_ids)::uuid[]))
  AND NOT EXISTS (SELECT 1 FROM notification_accepted_sources a
    WHERE a.recipient_account_id = f.recipient_account_id AND a.source_account_id = f.source_account_id)
  AND NOT EXISTS (SELECT 1 FROM notification_request_dismissals rd
    WHERE rd.recipient_account_id = f.recipient_account_id AND rd.source_account_id = f.source_account_id AND rd.last_notification_id >= f.notification_id)
GROUP BY f.recipient_account_id, f.source_account_id;

-- name: DeleteAllNotificationRequests :exec
DELETE FROM notification_requests;

-- RebuildNotificationRequests is InsertNotificationRequestsOf of every pair.
-- name: RebuildNotificationRequests :exec
INSERT INTO notification_requests (recipient_account_id, source_account_id, notifications_count, last_notification_id)
SELECT f.recipient_account_id, f.source_account_id, COUNT(*), MAX(f.notification_id::text)::uuid FROM notification_filterings f
WHERE NOT EXISTS (SELECT 1 FROM notification_accepted_sources a
    WHERE a.recipient_account_id = f.recipient_account_id AND a.source_account_id = f.source_account_id)
  AND NOT EXISTS (SELECT 1 FROM notification_request_dismissals rd
    WHERE rd.recipient_account_id = f.recipient_account_id AND rd.source_account_id = f.source_account_id AND rd.last_notification_id >= f.notification_id)
GROUP BY f.recipient_account_id, f.source_account_id;

-- ListNotificationRequests pages the requests of a recipient whose source no exclusion in force
-- at now hides, newest notification first; a null before starts at the newest. The domain
-- exclusions apply to the source domain of the newest notification, which each row carries.
-- name: ListNotificationRequests :many
SELECT r.recipient_account_id, r.source_account_id, r.notifications_count, r.last_notification_id, sqlc.embed(n) FROM notification_requests r
JOIN notifications n ON n.id = r.last_notification_id
WHERE r.recipient_account_id = sqlc.arg(recipient_account_id)
  AND (sqlc.narg(before)::uuid IS NULL OR r.last_notification_id < sqlc.narg(before)::uuid)
  AND NOT EXISTS (SELECT 1 FROM notification_exclusions x
    WHERE x.viewer_account_id = r.recipient_account_id AND x.target_account_id = r.source_account_id
      AND (x.expires_at IS NULL OR x.expires_at > sqlc.arg(now)::timestamptz))
  AND NOT EXISTS (SELECT 1 FROM notifications ln JOIN notification_domain_exclusions dx ON dx.viewer_account_id = ln.recipient_account_id
    WHERE ln.id = r.last_notification_id
      AND (dx.domain = ln.source_domain OR right(ln.source_domain, length(dx.domain) + 1) = '.' || dx.domain))
ORDER BY r.last_notification_id DESC
LIMIT sqlc.arg(page_limit);

-- GetNotificationRequest returns the request of a source as ListNotificationRequests lists it.
-- name: GetNotificationRequest :one
SELECT r.recipient_account_id, r.source_account_id, r.notifications_count, r.last_notification_id, sqlc.embed(n) FROM notification_requests r
JOIN notifications n ON n.id = r.last_notification_id
WHERE r.recipient_account_id = sqlc.arg(recipient_account_id) AND r.source_account_id = sqlc.arg(source_account_id)
  AND NOT EXISTS (SELECT 1 FROM notification_exclusions x
    WHERE x.viewer_account_id = r.recipient_account_id AND x.target_account_id = r.source_account_id
      AND (x.expires_at IS NULL OR x.expires_at > sqlc.arg(now)::timestamptz))
  AND NOT EXISTS (SELECT 1 FROM notifications ln JOIN notification_domain_exclusions dx ON dx.viewer_account_id = ln.recipient_account_id
    WHERE ln.id = r.last_notification_id
      AND (dx.domain = ln.source_domain OR right(ln.source_domain, length(dx.domain) + 1) = '.' || dx.domain));

-- GetNotificationRequestTx returns the request of a source, whatever hides it.
-- name: GetNotificationRequestTx :one
SELECT recipient_account_id, source_account_id, notifications_count, last_notification_id FROM notification_requests
WHERE recipient_account_id = $1 AND source_account_id = $2;

-- SummarizeNotificationRequests counts the requests that ListNotificationRequests lists and their
-- notifications.
-- name: SummarizeNotificationRequests :one
SELECT COUNT(*)::bigint AS accounts, COALESCE(SUM(r.notifications_count), 0)::bigint AS notifications FROM notification_requests r
WHERE r.recipient_account_id = sqlc.arg(recipient_account_id)
  AND NOT EXISTS (SELECT 1 FROM notification_exclusions x
    WHERE x.viewer_account_id = r.recipient_account_id AND x.target_account_id = r.source_account_id
      AND (x.expires_at IS NULL OR x.expires_at > sqlc.arg(now)::timestamptz))
  AND NOT EXISTS (SELECT 1 FROM notifications ln JOIN notification_domain_exclusions dx ON dx.viewer_account_id = ln.recipient_account_id
    WHERE ln.id = r.last_notification_id
      AND (dx.domain = ln.source_domain OR right(ln.source_domain, length(dx.domain) + 1) = '.' || dx.domain));

-- ListNotificationRequestNotifications pages the filtered notifications of a source that wait in
-- its request, newest first: neither accepted nor covered by a dismissal of the request, not
-- dismissed one by one, and not about a deleted status. A null before starts at the newest.
-- name: ListNotificationRequestNotifications :many
SELECT n.id, n.recipient_account_id, n.type, n.source_account_id, n.source_domain, n.status_id, n.moderation_action_id, n.report_id,
  n.subject_user_id, n.severance_id, n.appeal_id, n.custom_emoji_request_id, n.supporter_subscription_id, n.reaction_emoji, n.reaction_custom_emoji_id, n.account_move_id, n.account_move_outcome, n.service_closure_event_id, n.service_closure_change, n.voice_room_id, n.error_group_change, n.error_group_id, n.error_group_hour, n.announcement_id, n.hook_appeal_id, n.auto_delete_limit_notice_id, n.draft_schedule_id, n.dedupe_key, n.created_at
FROM notification_filterings f JOIN notifications n ON n.id = f.notification_id
WHERE f.recipient_account_id = sqlc.arg(recipient_account_id) AND f.source_account_id = sqlc.arg(source_account_id)
  AND (sqlc.narg(before)::uuid IS NULL OR f.notification_id < sqlc.narg(before)::uuid)
  AND NOT EXISTS (SELECT 1 FROM notification_accepted_sources a
    WHERE a.recipient_account_id = f.recipient_account_id AND a.source_account_id = f.source_account_id)
  AND NOT EXISTS (SELECT 1 FROM notification_request_dismissals rd
    WHERE rd.recipient_account_id = f.recipient_account_id AND rd.source_account_id = f.source_account_id AND rd.last_notification_id >= f.notification_id)
  AND NOT EXISTS (SELECT 1 FROM notification_dismissals d WHERE d.notification_id = n.id)
  AND NOT EXISTS (SELECT 1 FROM notification_deleted_statuses s WHERE s.status_id = n.status_id)
ORDER BY f.notification_id DESC
LIMIT sqlc.arg(page_limit);

-- FilteredPairsOfStatuses returns the pairs of the filtered notifications about the statuses,
-- whose requests the erasure of those notifications derives again.
-- name: FilteredPairsOfStatuses :many
SELECT DISTINCT f.recipient_account_id, f.source_account_id FROM notification_filterings f JOIN notifications n ON n.id = f.notification_id
WHERE n.status_id = ANY(sqlc.arg(ids)::uuid[]);

-- The erasure of deleted accounts' data (data-model sections 4.7 and 8): the rows of the policy
-- whose recipient or source is the account, a bounded batch (@lim) of those that grow with the
-- number of accounts.

-- name: EraseNotificationFilterings :exec
DELETE FROM notification_filterings WHERE notification_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseNotificationFilteringsOfStatuses :exec
DELETE FROM notification_filterings WHERE notification_id IN (
  SELECT n.id FROM notifications n WHERE n.status_id = ANY(sqlc.arg(ids)::uuid[]));

-- name: EraseNotificationPoliciesOf :exec
DELETE FROM notification_policies WHERE account_id = sqlc.arg(account_id);

-- name: EraseNotificationAcceptedSourcesOf :execrows
DELETE FROM notification_accepted_sources WHERE (recipient_account_id, source_account_id) IN (
  SELECT x.recipient_account_id, x.source_account_id FROM notification_accepted_sources x
  WHERE x.recipient_account_id = sqlc.arg(account_id) OR x.source_account_id = sqlc.arg(account_id)
  ORDER BY x.recipient_account_id, x.source_account_id LIMIT sqlc.arg(lim));

-- name: EraseNotificationRequestDismissalsOf :execrows
DELETE FROM notification_request_dismissals WHERE id IN (
  SELECT x.id FROM notification_request_dismissals x
  WHERE x.recipient_account_id = sqlc.arg(account_id) OR x.source_account_id = sqlc.arg(account_id)
  ORDER BY x.id LIMIT sqlc.arg(lim));

-- name: EraseNotificationRequestsOf :execrows
DELETE FROM notification_requests WHERE (recipient_account_id, source_account_id) IN (
  SELECT x.recipient_account_id, x.source_account_id FROM notification_requests x
  WHERE x.recipient_account_id = sqlc.arg(account_id) OR x.source_account_id = sqlc.arg(account_id)
  ORDER BY x.recipient_account_id, x.source_account_id LIMIT sqlc.arg(lim));
