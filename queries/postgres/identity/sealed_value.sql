-- SPDX-License-Identifier: AGPL-3.0-only

-- ListSealedLoginProviderSecrets returns a page of login_provider_secrets.sealed_secret ordered by id.
-- name: ListSealedLoginProviderSecrets :many
SELECT id AS row_id, sealed_secret AS sealed FROM login_provider_secrets WHERE id > COALESCE(sqlc.narg(after)::uuid, '00000000-0000-0000-0000-000000000000') ORDER BY id LIMIT sqlc.arg(page_limit);

-- ReplaceSealedLoginProviderSecrets rewrites login_provider_secrets.sealed_secret of a row that still holds old_sealed.
-- name: ReplaceSealedLoginProviderSecrets :execrows
UPDATE login_provider_secrets SET sealed_secret = sqlc.arg(sealed) WHERE id = sqlc.arg(row_id) AND sealed_secret = sqlc.arg(old_sealed);

-- ListSealedTOTPEnrollmentSecrets returns a page of totp_enrollments.secret_enc ordered by id.
-- name: ListSealedTOTPEnrollmentSecrets :many
SELECT id AS row_id, secret_enc AS sealed FROM totp_enrollments WHERE id > COALESCE(sqlc.narg(after)::uuid, '00000000-0000-0000-0000-000000000000') ORDER BY id LIMIT sqlc.arg(page_limit);

-- ReplaceSealedTOTPEnrollmentSecrets rewrites totp_enrollments.secret_enc of a row that still holds old_sealed.
-- name: ReplaceSealedTOTPEnrollmentSecrets :execrows
UPDATE totp_enrollments SET secret_enc = sqlc.arg(sealed) WHERE id = sqlc.arg(row_id) AND secret_enc = sqlc.arg(old_sealed);
