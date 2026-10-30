-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Subscriptions to the new posts of an account (notification, ADR-0123, data-model section 4.7).

-- InsertPostSubscription inserts nothing when the account already subscribes to the target.
-- name: InsertPostSubscription :execrows
INSERT INTO post_subscriptions (account_id, target_account_id, created_at) VALUES ($1, $2, $3)
ON CONFLICT (account_id, target_account_id) DO NOTHING;

-- name: DeletePostSubscription :execrows
DELETE FROM post_subscriptions WHERE account_id = $1 AND target_account_id = $2;

-- name: PostSubscriptionExists :one
SELECT EXISTS (SELECT 1 FROM post_subscriptions WHERE account_id = $1 AND target_account_id = $2);

-- name: CountPostSubscriptions :one
SELECT COUNT(*) FROM post_subscriptions WHERE account_id = $1;

-- name: HasPostSubscribers :one
SELECT EXISTS (SELECT 1 FROM post_subscriptions WHERE target_account_id = $1);

-- ListPostSubscriptionsPage pages the subscriptions of an account, newest first by
-- (created_at, target_account_id); a NULL before_target_account_id starts at the newest.
-- name: ListPostSubscriptionsPage :many
SELECT account_id, target_account_id, created_at
FROM post_subscriptions
WHERE account_id = sqlc.arg(account_id)
  AND (sqlc.narg(before_target_account_id)::uuid IS NULL
    OR created_at < sqlc.arg(before_created_at)::timestamptz
    OR (created_at = sqlc.arg(before_created_at)::timestamptz AND target_account_id < sqlc.narg(before_target_account_id)::uuid))
ORDER BY created_at DESC, target_account_id DESC
LIMIT sqlc.arg(page_limit);

-- ListPostSubscribersPage pages the subscribers of a target by the subscriber; a NULL after
-- starts at the first.
-- name: ListPostSubscribersPage :many
SELECT account_id
FROM post_subscriptions
WHERE target_account_id = sqlc.arg(target_account_id)
  AND (sqlc.narg(after)::uuid IS NULL OR account_id > sqlc.narg(after)::uuid)
ORDER BY account_id
LIMIT sqlc.arg(page_limit);

-- name: UpsertPostSubscriptionRequest :exec
INSERT INTO post_subscription_requests (account_id, target_account_id, created_at) VALUES ($1, $2, $3)
ON CONFLICT (account_id, target_account_id) DO NOTHING;

-- DeletePostSubscriptionRequest deletes the request and reports, by the rows it deleted,
-- whether there was one.
-- name: DeletePostSubscriptionRequest :execrows
DELETE FROM post_subscription_requests WHERE account_id = $1 AND target_account_id = $2;

-- name: PostSubscriptionRequestExists :one
SELECT EXISTS (SELECT 1 FROM post_subscription_requests WHERE account_id = $1 AND target_account_id = $2);
