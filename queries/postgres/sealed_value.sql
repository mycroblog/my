-- SPDX-License-Identifier: AGPL-3.0-only

-- ListSealedAccountPrivateKeys returns a page of local_account_private_keys.private_key_enc ordered by account_key_id.
-- name: ListSealedAccountPrivateKeys :many
SELECT account_key_id AS row_id, private_key_enc AS sealed FROM local_account_private_keys WHERE account_key_id > COALESCE(sqlc.narg(after)::uuid, '00000000-0000-0000-0000-000000000000') ORDER BY account_key_id LIMIT sqlc.arg(page_limit);

-- ReplaceSealedAccountPrivateKeys rewrites local_account_private_keys.private_key_enc of a row that still holds old_sealed.
-- name: ReplaceSealedAccountPrivateKeys :execrows
UPDATE local_account_private_keys SET private_key_enc = sqlc.arg(sealed) WHERE account_key_id = sqlc.arg(row_id) AND private_key_enc = sqlc.arg(old_sealed);

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

-- ListSealedWebhookSecrets returns a page of webhook_secrets.sealed_secret ordered by id.
-- name: ListSealedWebhookSecrets :many
SELECT id AS row_id, sealed_secret AS sealed FROM webhook_secrets WHERE id > COALESCE(sqlc.narg(after)::uuid, '00000000-0000-0000-0000-000000000000') ORDER BY id LIMIT sqlc.arg(page_limit);

-- ReplaceSealedWebhookSecrets rewrites webhook_secrets.sealed_secret of a row that still holds old_sealed.
-- name: ReplaceSealedWebhookSecrets :execrows
UPDATE webhook_secrets SET sealed_secret = sqlc.arg(sealed) WHERE id = sqlc.arg(row_id) AND sealed_secret = sqlc.arg(old_sealed);

-- ListSealedInstanceSecrets returns a page of instance_secrets.sealed_secret ordered by id.
-- name: ListSealedInstanceSecrets :many
SELECT id AS row_id, sealed_secret AS sealed FROM instance_secrets WHERE id > COALESCE(sqlc.narg(after)::uuid, '00000000-0000-0000-0000-000000000000') ORDER BY id LIMIT sqlc.arg(page_limit);

-- ReplaceSealedInstanceSecrets rewrites instance_secrets.sealed_secret of a row that still holds old_sealed.
-- name: ReplaceSealedInstanceSecrets :execrows
UPDATE instance_secrets SET sealed_secret = sqlc.arg(sealed) WHERE id = sqlc.arg(row_id) AND sealed_secret = sqlc.arg(old_sealed);
