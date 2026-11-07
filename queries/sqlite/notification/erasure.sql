-- SPDX-License-Identifier: AGPL-3.0-only

-- The erasure of deleted accounts' data (data-model sections 4.2 and 8, #397). Each module
-- erases its own tables; the statements whose rows grow with the number of users or followers
-- take a bounded batch (@lim). A list argument is a JSON array of strings, read with json_each.

-- name: EraseNotificationDismissalsOfStatuses :exec
DELETE FROM notification_dismissals WHERE notification_id IN (
  SELECT n.id FROM notifications n WHERE n.status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))));

-- name: EraseNotificationsOfStatuses :exec
DELETE FROM notifications WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseDeletedStatusesOfStatuses :exec
DELETE FROM notification_deleted_statuses WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- ListNotificationIDsToErase returns up to @lim notifications to, from, or about the account and
-- its user.
-- name: ListNotificationIDsToErase :many
SELECT id FROM notifications
WHERE recipient_account_id = sqlc.arg(account_id) OR source_account_id = sqlc.arg(account_id)
  OR subject_user_id = sqlc.arg(user_id)
ORDER BY id
LIMIT sqlc.arg(lim);

-- name: EraseNotificationDismissals :exec
DELETE FROM notification_dismissals WHERE notification_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseNotifications :exec
DELETE FROM notifications WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseNotificationReadMarker :exec
DELETE FROM notification_read_markers WHERE account_id = sqlc.arg(account_id);

-- name: EraseNotificationSettingsOf :exec
DELETE FROM notification_settings WHERE account_id = sqlc.arg(account_id);

-- name: EraseNotificationExclusionsAsViewer :execrows
DELETE FROM notification_exclusions WHERE notification_exclusions.viewer_account_id = sqlc.arg(account_id) AND notification_exclusions.target_account_id IN (
  SELECT x.target_account_id FROM notification_exclusions x WHERE x.viewer_account_id = sqlc.arg(account_id)
  ORDER BY x.target_account_id LIMIT sqlc.arg(lim));

-- name: EraseNotificationExclusionsAsTarget :execrows
DELETE FROM notification_exclusions WHERE notification_exclusions.target_account_id = sqlc.arg(account_id) AND notification_exclusions.viewer_account_id IN (
  SELECT x.viewer_account_id FROM notification_exclusions x WHERE x.target_account_id = sqlc.arg(account_id)
  ORDER BY x.viewer_account_id LIMIT sqlc.arg(lim));

-- name: EraseNotificationDomainExclusionsOf :execrows
DELETE FROM notification_domain_exclusions WHERE notification_domain_exclusions.viewer_account_id = sqlc.arg(account_id) AND notification_domain_exclusions.domain IN (
  SELECT x.domain FROM notification_domain_exclusions x WHERE x.viewer_account_id = sqlc.arg(account_id)
  ORDER BY x.domain LIMIT sqlc.arg(lim));

-- name: ErasePostSubscriptionsOf :exec
DELETE FROM post_subscriptions WHERE account_id = sqlc.arg(account_id);

-- name: ErasePostSubscriptionRequestsOf :exec
DELETE FROM post_subscription_requests WHERE account_id = sqlc.arg(account_id);

-- name: ErasePostSubscriptionsAsTarget :execrows
DELETE FROM post_subscriptions WHERE post_subscriptions.target_account_id = sqlc.arg(account_id) AND post_subscriptions.account_id IN (
  SELECT x.account_id FROM post_subscriptions x WHERE x.target_account_id = sqlc.arg(account_id)
  ORDER BY x.account_id LIMIT sqlc.arg(lim));

-- name: ErasePostSubscriptionRequestsAsTarget :execrows
DELETE FROM post_subscription_requests WHERE post_subscription_requests.target_account_id = sqlc.arg(account_id) AND post_subscription_requests.account_id IN (
  SELECT x.account_id FROM post_subscription_requests x WHERE x.target_account_id = sqlc.arg(account_id)
  ORDER BY x.account_id LIMIT sqlc.arg(lim));
