-- SPDX-License-Identifier: AGPL-3.0-only

-- The service closure emails (ADR-0088 section 2, ADR-0086 section 1).

-- name: ListVerifiedPrimaryEmailUsers :many
SELECT p.user_id FROM user_primary_emails p
WHERE p.user_id > sqlc.arg(after)
  AND EXISTS (SELECT 1 FROM user_email_verifications v WHERE v.email_address_id = p.email_address_id)
ORDER BY p.user_id
LIMIT sqlc.arg(row_limit);

-- name: InsertServiceClosureEmail :execrows
INSERT INTO service_closure_emails (id, closure_event_id, email, user_id, queued_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (closure_event_id, email, user_id) DO NOTHING;

-- name: ServiceClosureEmailQueued :one
SELECT EXISTS (
  SELECT 1 FROM service_closure_emails WHERE closure_event_id = sqlc.arg(closure_event_id) AND email = sqlc.arg(email)
) AS queued;
