-- SPDX-License-Identifier: AGPL-3.0-only

-- The records of the payment provider's facts (ADR-0074 section 4): the webhook receipts and the
-- attempts of the reconciliation, written once.

-- InsertPaymentWebhookReceipt writes nothing when the event of the provider is recorded.
-- name: InsertPaymentWebhookReceipt :execrows
INSERT INTO payment_webhook_receipts (id, provider, event_id, event_type, object_id, received_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (provider, event_id) DO NOTHING;

-- name: LastPaymentWebhookReceipt :one
SELECT id, provider, event_id, event_type, object_id, received_at FROM payment_webhook_receipts
WHERE provider = ?
ORDER BY received_at DESC, id DESC
LIMIT 1;

-- PurgePaymentWebhookReceipts deletes up to lim receipts received before before_at.
-- name: PurgePaymentWebhookReceipts :execrows
DELETE FROM payment_webhook_receipts WHERE id IN (
  SELECT r.id FROM payment_webhook_receipts r WHERE r.received_at < sqlc.arg(before_at) ORDER BY r.received_at LIMIT sqlc.arg(lim)
);

-- InsertPaymentReconcileRun writes nothing when an attempt of the job is recorded.
-- name: InsertPaymentReconcileRun :execrows
INSERT INTO payment_reconcile_runs (
  id, provider, run_key, outcome, error_kind, changes_until, changes_cursor, money_cursor, sweep_cursor,
  requests, checked_subscriptions, corrected_subscriptions, occurred_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (provider, run_key) DO NOTHING;

-- name: CountPaymentReconcileRunsByKey :one
SELECT COUNT(*) FROM payment_reconcile_runs WHERE provider = ? AND run_key = ?;

-- LastPaymentReconcileRun returns the newest attempt of a provider, only among the succeeded and
-- partial ones unless include_failed is true.
-- name: LastPaymentReconcileRun :one
SELECT id, provider, run_key, outcome, error_kind, changes_until, changes_cursor, money_cursor, sweep_cursor,
  requests, checked_subscriptions, corrected_subscriptions, occurred_at
FROM payment_reconcile_runs
WHERE provider = sqlc.arg(provider) AND (CAST(sqlc.arg(include_failed) AS INTEGER) = 1 OR outcome <> 'failed')
ORDER BY occurred_at DESC, id DESC
LIMIT 1;

-- ClaimPaymentCheckout takes the claim of a checkout when no one holds it or its claim has
-- expired, and reports through its row count whether it did.
-- name: ClaimPaymentCheckout :execrows
INSERT INTO payment_checkout_claims (provider, checkout_id, holder, claimed_at, expires_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (provider, checkout_id) DO UPDATE SET
  holder = excluded.holder,
  claimed_at = excluded.claimed_at,
  expires_at = excluded.expires_at
WHERE payment_checkout_claims.expires_at <= excluded.claimed_at;

-- ReleasePaymentCheckout deletes the claim of a checkout when holder still holds it.
-- name: ReleasePaymentCheckout :exec
DELETE FROM payment_checkout_claims WHERE provider = ? AND checkout_id = ? AND holder = ?;

-- PurgePaymentCheckoutClaims deletes the claims that expired before before_at.
-- name: PurgePaymentCheckoutClaims :execrows
DELETE FROM payment_checkout_claims WHERE expires_at < sqlc.arg(before_at);
