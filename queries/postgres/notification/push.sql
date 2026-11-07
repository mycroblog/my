-- SPDX-License-Identifier: AGPL-3.0-only

-- Web Push subscriptions (current values) and the notification types pushed to them (FR-206).

-- DeletePushSubscriptionAlertsReplacedBy deletes the alerts of the subscriptions that a new
-- subscription of the token family or the endpoint replaces.
-- name: DeletePushSubscriptionAlertsReplacedBy :exec
DELETE FROM push_subscription_alerts WHERE subscription_id IN (
  SELECT s.id FROM push_subscriptions s
  WHERE s.oauth_token_family_id = sqlc.arg(oauth_token_family_id) OR s.endpoint = sqlc.arg(endpoint));

-- name: DeletePushSubscriptionsReplacedBy :exec
DELETE FROM push_subscriptions
WHERE oauth_token_family_id = sqlc.arg(oauth_token_family_id) OR endpoint = sqlc.arg(endpoint);

-- name: InsertPushSubscription :exec
INSERT INTO push_subscriptions (id, account_id, oauth_token_family_id, endpoint, key_p256dh, key_auth, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: InsertPushSubscriptionAlert :exec
INSERT INTO push_subscription_alerts (subscription_id, notification_type) VALUES ($1, $2)
ON CONFLICT (subscription_id, notification_type) DO NOTHING;

-- name: GetPushSubscription :one
SELECT id, account_id, oauth_token_family_id, endpoint, key_p256dh, key_auth, created_at, updated_at
FROM push_subscriptions WHERE id = $1;

-- name: GetPushSubscriptionOfTokenFamily :one
SELECT id, account_id, oauth_token_family_id, endpoint, key_p256dh, key_auth, created_at, updated_at
FROM push_subscriptions WHERE oauth_token_family_id = $1;

-- name: ListPushSubscriptionAlerts :many
SELECT notification_type FROM push_subscription_alerts WHERE subscription_id = $1 ORDER BY notification_type;

-- ListPushSubscriptionIDsForAlert returns the subscriptions of an account that take a type of
-- notification.
-- name: ListPushSubscriptionIDsForAlert :many
SELECT s.id FROM push_subscriptions s
JOIN push_subscription_alerts a ON a.subscription_id = s.id
WHERE s.account_id = sqlc.arg(account_id) AND a.notification_type = sqlc.arg(notification_type)
ORDER BY s.id;

-- name: DeletePushSubscriptionAlerts :exec
DELETE FROM push_subscription_alerts WHERE subscription_id = $1;

-- name: DeletePushSubscription :execrows
DELETE FROM push_subscriptions WHERE id = $1;

-- name: ErasePushSubscriptionAlertsOf :exec
DELETE FROM push_subscription_alerts WHERE subscription_id IN (
  SELECT s.id FROM push_subscriptions s WHERE s.account_id = $1);

-- name: ErasePushSubscriptionsOf :exec
DELETE FROM push_subscriptions WHERE account_id = $1;
