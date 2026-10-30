-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Subscriptions to the new posts of an account (notification, ADR-0123, data-model section 4.7).

-- InsertPostSubscription inserts nothing when the account already subscribes to the target.
-- name: InsertPostSubscription :execrows
INSERT INTO post_subscriptions (account_id, target_account_id, created_at) VALUES (?, ?, ?)
ON CONFLICT (account_id, target_account_id) DO NOTHING;

-- name: DeletePostSubscription :execrows
DELETE FROM post_subscriptions WHERE account_id = ? AND target_account_id = ?;

-- name: PostSubscriptionExists :one
SELECT EXISTS (SELECT 1 FROM post_subscriptions WHERE account_id = ? AND target_account_id = ?);

-- name: CountPostSubscriptions :one
SELECT COUNT(*) FROM post_subscriptions WHERE account_id = ?;

-- name: HasPostSubscribers :one
SELECT EXISTS (SELECT 1 FROM post_subscriptions WHERE target_account_id = ?);

-- ListPostSubscriptionsPage pages the subscriptions of an account, newest first by
-- (created_at, target_account_id); an empty before_target_account_id starts at the newest.
-- name: ListPostSubscriptionsPage :many
SELECT account_id, target_account_id, created_at
FROM post_subscriptions
WHERE account_id = sqlc.arg(account_id)
  AND (CAST(sqlc.arg(before_target_account_id) AS TEXT) = ''
    OR created_at < sqlc.arg(before_created_at)
    OR (created_at = sqlc.arg(before_created_at) AND target_account_id < CAST(sqlc.arg(before_target_account_id) AS TEXT)))
ORDER BY created_at DESC, target_account_id DESC
LIMIT sqlc.arg(page_limit);

-- ListPostSubscribersPage pages the subscribers of a target by the subscriber; an empty after
-- starts at the first.
-- name: ListPostSubscribersPage :many
SELECT account_id
FROM post_subscriptions
WHERE target_account_id = sqlc.arg(target_account_id) AND account_id > CAST(sqlc.arg(after) AS TEXT)
ORDER BY account_id
LIMIT sqlc.arg(page_limit);

-- name: UpsertPostSubscriptionRequest :exec
INSERT INTO post_subscription_requests (account_id, target_account_id, created_at) VALUES (?, ?, ?)
ON CONFLICT (account_id, target_account_id) DO NOTHING;

-- DeletePostSubscriptionRequest deletes the request and reports, by the rows it deleted,
-- whether there was one.
-- name: DeletePostSubscriptionRequest :execrows
DELETE FROM post_subscription_requests WHERE account_id = ? AND target_account_id = ?;

-- name: PostSubscriptionRequestExists :one
SELECT EXISTS (SELECT 1 FROM post_subscription_requests WHERE account_id = ? AND target_account_id = ?);
