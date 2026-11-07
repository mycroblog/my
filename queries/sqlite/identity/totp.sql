-- SPDX-License-Identifier: AGPL-3.0-only

-- TOTP two-factor authentication (data-model 4.1, FR-005).

-- name: InsertTOTPEnrollment :exec
INSERT INTO totp_enrollments (id, user_id, secret_enc, enrolled_at) VALUES (?, ?, ?, ?);

-- name: GetNewestTOTPEnrollment :one
-- The newest enrollment of a user and whether it was revoked.
SELECT e.id, e.user_id, e.secret_enc, e.enrolled_at,
  EXISTS (SELECT 1 FROM totp_enrollment_revocations r WHERE r.enrollment_id = e.id) AS revoked
FROM totp_enrollments e WHERE e.user_id = ? ORDER BY e.id DESC LIMIT 1;

-- name: InsertTOTPEnrollmentRevocation :exec
INSERT INTO totp_enrollment_revocations (enrollment_id, revoked_at) VALUES (?, ?)
ON CONFLICT (enrollment_id) DO NOTHING;

-- name: InsertTOTPRecoveryCode :exec
INSERT INTO totp_recovery_codes (enrollment_id, code_hash) VALUES (?, ?);

-- name: UseTOTPRecoveryCode :execrows
-- Records the use of a recovery code that exists and was not used; it inserts nothing otherwise.
INSERT INTO totp_recovery_code_uses (enrollment_id, code_hash, used_at)
SELECT c.enrollment_id, c.code_hash, sqlc.arg(used_at) FROM totp_recovery_codes c
WHERE c.enrollment_id = sqlc.arg(enrollment_id) AND c.code_hash = sqlc.arg(code_hash)
ON CONFLICT (enrollment_id, code_hash) DO NOTHING;

-- name: CountUnusedTOTPRecoveryCodes :one
SELECT count(*) FROM totp_recovery_codes c
WHERE c.enrollment_id = ? AND NOT EXISTS (
  SELECT 1 FROM totp_recovery_code_uses u WHERE u.enrollment_id = c.enrollment_id AND u.code_hash = c.code_hash);
