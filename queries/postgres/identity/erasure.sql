-- SPDX-License-Identifier: AGPL-3.0-only

-- The erasure of deleted accounts' data (data-model sections 4.2 and 8, #397). Each module
-- erases its own tables; the statements whose rows grow with the number of users or followers
-- take a bounded batch (@lim). A list argument is an array of strings.

-- identity: the user's rows, which do not grow with other users ---------------------------------

-- name: EraseEmailSuppressions :exec
DELETE FROM email_suppressions WHERE email_address_id IN (
  SELECT a.id FROM user_email_addresses a WHERE a.user_id = sqlc.arg(user_id));

-- name: EraseEmailSuppressionLifts :exec
DELETE FROM email_suppression_lifts WHERE email_address_id IN (
  SELECT a.id FROM user_email_addresses a WHERE a.user_id = sqlc.arg(user_id));

-- name: EraseEmailDeliveryFailures :exec
DELETE FROM email_delivery_failures WHERE email_address_id IN (
  SELECT a.id FROM user_email_addresses a WHERE a.user_id = sqlc.arg(user_id));

-- name: EraseEmailVerifications :exec
DELETE FROM user_email_verifications WHERE email_address_id IN (
  SELECT a.id FROM user_email_addresses a WHERE a.user_id = sqlc.arg(user_id));

-- name: EraseUserPrimaryEmail :exec
DELETE FROM user_primary_emails WHERE user_id = sqlc.arg(user_id);

-- name: EraseEmailAddresses :exec
DELETE FROM user_email_addresses WHERE user_id = sqlc.arg(user_id);

-- name: ErasePasswordCredentialRemovals :exec
DELETE FROM password_credential_removals WHERE password_credential_id IN (
  SELECT c.id FROM password_credentials c WHERE c.user_id = sqlc.arg(user_id));

-- name: ErasePasswordCredentials :exec
DELETE FROM password_credentials WHERE user_id = sqlc.arg(user_id);

-- name: EraseTOTPRecoveryCodeUses :exec
DELETE FROM totp_recovery_code_uses WHERE enrollment_id IN (
  SELECT e.id FROM totp_enrollments e WHERE e.user_id = sqlc.arg(user_id));

-- name: EraseTOTPRecoveryCodes :exec
DELETE FROM totp_recovery_codes WHERE enrollment_id IN (
  SELECT e.id FROM totp_enrollments e WHERE e.user_id = sqlc.arg(user_id));

-- name: EraseTOTPEnrollmentRevocations :exec
DELETE FROM totp_enrollment_revocations WHERE enrollment_id IN (
  SELECT e.id FROM totp_enrollments e WHERE e.user_id = sqlc.arg(user_id));

-- name: EraseTOTPEnrollments :exec
DELETE FROM totp_enrollments WHERE user_id = sqlc.arg(user_id);

-- name: ErasePasskeyCredentialRevocations :exec
DELETE FROM passkey_credential_revocations WHERE passkey_id IN (
  SELECT c.id FROM passkey_credentials c WHERE c.user_id = sqlc.arg(user_id));

-- name: ErasePasskeyCredentials :exec
DELETE FROM passkey_credentials WHERE user_id = sqlc.arg(user_id);

-- name: EraseUserExternalIdentities :exec
DELETE FROM user_external_identities WHERE user_id = sqlc.arg(user_id);

-- name: EraseExternalIdentityUnlinks :exec
DELETE FROM external_identity_unlinks WHERE link_id IN (
  SELECT l.id FROM external_identity_links l WHERE l.user_id = sqlc.arg(user_id));

-- EraseExternalIdentities deletes the external identities that only the user linked.
-- name: EraseExternalIdentities :exec
DELETE FROM external_identities WHERE id IN (
  SELECT l.external_identity_id FROM external_identity_links l WHERE l.user_id = sqlc.arg(user_id))
  AND NOT EXISTS (
    SELECT 1 FROM external_identity_links o
    WHERE o.external_identity_id = external_identities.id AND o.user_id <> sqlc.arg(user_id));

-- name: EraseExternalIdentityLinks :exec
DELETE FROM external_identity_links WHERE user_id = sqlc.arg(user_id);

-- name: EraseUserTokenRedemptions :exec
DELETE FROM user_token_redemptions WHERE token_id IN (
  SELECT t.id FROM user_tokens t WHERE t.user_id = sqlc.arg(user_id));

-- name: EraseUserTokens :exec
DELETE FROM user_tokens WHERE user_id = sqlc.arg(user_id);

-- name: EraseSessionRevocations :exec
DELETE FROM session_revocations WHERE session_id IN (
  SELECT s.id FROM sessions s WHERE s.user_id = sqlc.arg(user_id));

-- name: EraseSessionActivity :exec
DELETE FROM session_activity WHERE session_id IN (
  SELECT s.id FROM sessions s WHERE s.user_id = sqlc.arg(user_id));

-- name: EraseSessions :exec
DELETE FROM sessions WHERE user_id = sqlc.arg(user_id);

-- name: EraseAuthorizationCodeRedemptions :exec
DELETE FROM oauth_authorization_code_redemptions WHERE code_hash IN (
  SELECT c.code_hash FROM oauth_authorization_codes c WHERE c.user_id = sqlc.arg(user_id));

-- name: EraseAuthorizationCodes :exec
DELETE FROM oauth_authorization_codes WHERE user_id = sqlc.arg(user_id);

-- name: EraseAccessTokenRevocations :exec
DELETE FROM oauth_token_revocations WHERE token_id IN (
  SELECT t.id FROM oauth_access_tokens t WHERE t.user_id = sqlc.arg(user_id));

-- name: EraseTokenFamilyLastUsed :exec
DELETE FROM oauth_token_family_last_used WHERE family_id IN (
  SELECT t.family_id FROM oauth_access_tokens t WHERE t.user_id = sqlc.arg(user_id));

-- name: EraseAccessTokens :exec
DELETE FROM oauth_access_tokens WHERE user_id = sqlc.arg(user_id);

-- name: EraseUserPreferences :exec
DELETE FROM user_preferences WHERE user_id = sqlc.arg(user_id);

-- name: EraseUserRoleBadgeDisplays :exec
DELETE FROM user_role_badge_displays WHERE user_id = sqlc.arg(user_id);

-- name: EraseUserTimelineLanguages :exec
DELETE FROM user_timeline_languages WHERE user_id = sqlc.arg(user_id);

-- name: EraseUserTimeZone :exec
DELETE FROM user_time_zones WHERE user_id = sqlc.arg(user_id);

-- name: EraseUserInteractionPolicies :exec
DELETE FROM user_interaction_policies WHERE user_id = sqlc.arg(user_id);

-- name: EraseRegistrationDecisions :exec
DELETE FROM registration_decisions WHERE application_id IN (
  SELECT a.id FROM registration_applications a WHERE a.user_id = sqlc.arg(user_id));

-- name: EraseRegistrationApplications :exec
DELETE FROM registration_applications WHERE user_id = sqlc.arg(user_id);

-- name: EraseRegistrationState :exec
DELETE FROM user_registration_states WHERE user_id = sqlc.arg(user_id);

-- name: EraseServiceClosureEmails :exec
DELETE FROM service_closure_emails WHERE user_id = sqlc.arg(user_id);

-- name: ListOAuthClientIDsOwnedBy :many
SELECT id FROM oauth_clients WHERE owner_user_id = sqlc.arg(user_id)::uuid ORDER BY id;

-- BlankOAuthClientDetailsOf removes the name and website of the clients of an erased owner.
-- name: BlankOAuthClientDetailsOf :exec
UPDATE oauth_clients SET name = '', website = NULL WHERE owner_user_id = sqlc.arg(user_id)::uuid;

-- BlankOAuthClientRevisionDetailsOf removes the same from the revisions of those clients.
-- name: BlankOAuthClientRevisionDetailsOf :exec
UPDATE oauth_client_revisions SET name = '', website = NULL WHERE oauth_client_id IN (
  SELECT c.id FROM oauth_clients c WHERE c.owner_user_id = sqlc.arg(user_id)::uuid);
