-- SPDX-License-Identifier: AGPL-3.0-only

-- The tips (ADR-0162 section 12).

-- name: InsertSupporterTipChange :exec
INSERT INTO supporter_tip_changes (id, state, currency, changed_by_user_id, changed_at) VALUES ($1, $2, $3, $4, $5);

-- name: InsertSupporterTipAmount :exec
INSERT INTO supporter_tip_amounts (change_id, amount_minor) VALUES ($1, $2);

-- name: GetLastSupporterTipChange :one
SELECT id, state, currency, changed_by_user_id, changed_at FROM supporter_tip_changes ORDER BY id DESC LIMIT 1;

-- name: ListSupporterTipAmounts :many
SELECT amount_minor FROM supporter_tip_amounts WHERE change_id = $1 ORDER BY amount_minor;

-- name: InsertSupporterTip :exec
INSERT INTO supporter_tips (id, user_id, amount_minor, currency, provider, created_at) VALUES ($1, $2, $3, $4, $5, $6);

-- name: InsertSupporterOpenTip :exec
INSERT INTO supporter_open_tips (user_id, tip_id, expires_at) VALUES ($1, $2, $3);

-- name: InsertSupporterTipCheckout :execrows
INSERT INTO supporter_tip_checkouts (tip_id, provider, provider_checkout_id, expires_at, created_at) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT DO NOTHING;

-- name: UpdateSupporterOpenTipExpiry :exec
UPDATE supporter_open_tips SET expires_at = $1 WHERE tip_id = $2;

-- name: InsertSupporterTipPayment :execrows
INSERT INTO supporter_tip_payments (tip_id, provider, provider_payment_id, amount_minor, currency, paid_at) VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT DO NOTHING;

-- name: InsertSupporterTipOutcome :execrows
INSERT INTO supporter_tip_outcomes (id, tip_id, kind, occurred_at) VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: DeleteSupporterOpenTip :exec
DELETE FROM supporter_open_tips WHERE tip_id = $1;

-- name: GetSupporterTip :one
SELECT id, user_id, amount_minor, currency, provider, created_at FROM supporter_tips WHERE id = $1;

-- name: GetSupporterTipCheckout :one
SELECT provider, provider_checkout_id, expires_at, created_at FROM supporter_tip_checkouts WHERE tip_id = $1;

-- name: GetSupporterTipPayment :one
SELECT provider, provider_payment_id, amount_minor, currency, paid_at FROM supporter_tip_payments WHERE tip_id = $1;

-- name: ListSupporterTipOutcomes :many
SELECT kind FROM supporter_tip_outcomes WHERE tip_id = $1 ORDER BY kind;

-- name: FindSupporterTipByCheckout :one
SELECT tip_id FROM supporter_tip_checkouts WHERE provider = $1 AND provider_checkout_id = $2;

-- name: FindSupporterTipByPayment :one
SELECT tip_id FROM supporter_tip_payments WHERE provider = $1 AND provider_payment_id = $2;

-- name: FindSupporterOpenTip :one
SELECT tip_id FROM supporter_open_tips WHERE user_id = $1;

-- name: ListDueSupporterOpenTips :many
SELECT tip_id FROM supporter_open_tips WHERE expires_at <= $1 ORDER BY expires_at, tip_id LIMIT $2;

-- name: ListSupporterOpenTipCheckouts :many
SELECT c.provider_checkout_id FROM supporter_open_tips o
JOIN supporter_tip_checkouts c ON c.tip_id = o.tip_id
WHERE c.provider = $1 AND o.expires_at > $2
ORDER BY o.expires_at, o.tip_id;

-- The erasure of a user's tips (ADR-0162 section 18).

-- name: DeleteSupporterTipOutcomesOfUser :exec
DELETE FROM supporter_tip_outcomes WHERE tip_id IN (SELECT id FROM supporter_tips WHERE user_id = $1);

-- name: DeleteSupporterTipPaymentsOfUser :exec
DELETE FROM supporter_tip_payments WHERE tip_id IN (SELECT id FROM supporter_tips WHERE user_id = $1);

-- name: DeleteSupporterTipCheckoutsOfUser :exec
DELETE FROM supporter_tip_checkouts WHERE tip_id IN (SELECT id FROM supporter_tips WHERE user_id = $1);

-- name: DeleteSupporterOpenTipOfUser :exec
DELETE FROM supporter_open_tips WHERE user_id = $1;

-- name: DeleteSupporterTipsOfUser :exec
DELETE FROM supporter_tips WHERE user_id = $1;

-- Rebuilding the open tips (data-model section 9): the newest tip of each user that has neither
-- a payment nor the outcome expired, until the end of its checkout, or an hour after its start
-- without one.

-- name: DeleteAllSupporterOpenTips :exec
DELETE FROM supporter_open_tips;

-- ListSupporterTipUsers returns the users of the tips among ids.
-- name: ListSupporterTipUsers :many
SELECT id, user_id FROM supporter_tips WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: RebuildSupporterOpenTips :exec
INSERT INTO supporter_open_tips (user_id, tip_id, expires_at)
SELECT DISTINCT ON (t.user_id) t.user_id, t.id,
  COALESCE((SELECT c.expires_at FROM supporter_tip_checkouts c WHERE c.tip_id = t.id), t.created_at + interval '1 hour')
FROM supporter_tips t
WHERE NOT EXISTS (SELECT 1 FROM supporter_tip_payments p WHERE p.tip_id = t.id)
  AND NOT EXISTS (SELECT 1 FROM supporter_tip_outcomes o WHERE o.tip_id = t.id AND o.kind = 'expired')
ORDER BY t.user_id, t.id DESC;
