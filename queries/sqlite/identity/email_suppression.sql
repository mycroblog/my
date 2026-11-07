-- SPDX-License-Identifier: AGPL-3.0-only

-- The suppression of email addresses that do not receive email (ADR-0086 section 5).

-- name: InsertEmailDeliveryFailure :execrows
INSERT INTO email_delivery_failures (id, email_address_id, kind, source, feedback_key, enhanced_status, occurred_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (feedback_key) DO NOTHING;

-- name: GetPrimaryEmailAddressIDByNormalized :one
SELECT email_address_id FROM user_primary_emails WHERE address_normalized = ?;

-- name: InsertEmailSuppressionLift :exec
INSERT INTO email_suppression_lifts (id, email_address_id, reason, lifted_at) VALUES (?, ?, ?, ?);

-- name: DeleteEmailSuppression :exec
DELETE FROM email_suppressions WHERE email_address_id = ?;

-- The rule of email_suppressions (data-model 4.1): of the failures of an address after its last
-- lift, the earliest hard bounce, or the earliest complaint when there is no hard bounce.
-- name: DeriveEmailSuppression :exec
INSERT INTO email_suppressions (email_address_id, kind, failure_id)
SELECT f.email_address_id, f.kind, f.id FROM email_delivery_failures f
WHERE f.email_address_id = sqlc.arg(email_address_id)
  AND NOT EXISTS (
    SELECT 1 FROM email_suppression_lifts l1 WHERE l1.email_address_id = f.email_address_id AND l1.lifted_at >= f.occurred_at)
  AND (f.kind = 'hard_bounce' OR NOT EXISTS (
    SELECT 1 FROM email_delivery_failures h WHERE h.email_address_id = f.email_address_id AND h.kind = 'hard_bounce'
      AND NOT EXISTS (
        SELECT 1 FROM email_suppression_lifts l2 WHERE l2.email_address_id = h.email_address_id AND l2.lifted_at >= h.occurred_at)))
  AND NOT EXISTS (
    SELECT 1 FROM email_delivery_failures o WHERE o.email_address_id = f.email_address_id AND o.kind = f.kind
      AND (o.occurred_at < f.occurred_at OR (o.occurred_at = f.occurred_at AND o.id < f.id))
      AND NOT EXISTS (
        SELECT 1 FROM email_suppression_lifts l3 WHERE l3.email_address_id = o.email_address_id AND l3.lifted_at >= o.occurred_at));

-- name: GetEmailSuppression :one
SELECT s.email_address_id, s.kind, s.failure_id, f.occurred_at
FROM email_suppressions s
JOIN email_delivery_failures f ON f.id = s.failure_id
WHERE s.email_address_id = ?;
