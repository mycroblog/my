-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertOAuthClientIfAbsent :exec
INSERT INTO oauth_clients (id, client_id, client_secret_hash, name, website, scope, registration, owner_user_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (client_id) DO NOTHING;

-- name: GetOAuthClientByClientID :one
SELECT id, client_id, client_secret_hash, name, website, scope, registration, owner_user_id, created_at
FROM oauth_clients
WHERE client_id = ?;

-- name: GetOAuthClientByID :one
SELECT id, client_id, client_secret_hash, name, website, scope, registration, owner_user_id, created_at
FROM oauth_clients
WHERE id = ?;

-- GetOAuthClientState returns whether a client (by its id) is disabled, that is has a
-- disablement without a lift, and whether it is deleted.
-- name: GetOAuthClientState :one
SELECT
  EXISTS (
    SELECT 1 FROM oauth_client_disablements d
    WHERE d.client_id = c.id AND NOT EXISTS (SELECT 1 FROM oauth_client_disablement_lifts l WHERE l.disablement_id = d.id)
  ) AS disabled,
  EXISTS (SELECT 1 FROM oauth_client_deletions x WHERE x.client_id = c.id) AS deleted
FROM oauth_clients c
WHERE c.id = sqlc.arg(id);

-- InsertOAuthClientDisablementIfEnabled records a disablement unless the client is disabled
-- already, so that repeating it adds nothing.
-- name: InsertOAuthClientDisablementIfEnabled :execrows
INSERT INTO oauth_client_disablements (id, client_id, disabled_by_user_id, disabled_at)
SELECT sqlc.arg(id), sqlc.arg(client_id), sqlc.narg(disabled_by_user_id), sqlc.arg(disabled_at)
WHERE NOT EXISTS (
    SELECT 1 FROM oauth_client_disablements d
    WHERE d.client_id = sqlc.arg(client_id) AND NOT EXISTS (SELECT 1 FROM oauth_client_disablement_lifts l WHERE l.disablement_id = d.id)
  );

-- LiftOAuthClientDisablements lifts every disablement of a client that has no lift yet.
-- name: LiftOAuthClientDisablements :execrows
INSERT INTO oauth_client_disablement_lifts (disablement_id, lifted_by_user_id, lifted_at)
SELECT d.id, sqlc.narg(lifted_by_user_id), sqlc.arg(lifted_at) FROM oauth_client_disablements d
WHERE d.client_id = sqlc.arg(client_id)
ON CONFLICT (disablement_id) DO NOTHING;

-- name: InsertOAuthClientDeletionIfAbsent :execrows
INSERT INTO oauth_client_deletions (client_id, reason, deleted_by_user_id, deleted_at) VALUES (?, ?, ?, ?)
ON CONFLICT (client_id) DO NOTHING;

-- name: ListOAuthClientRedirectURIs :many
SELECT redirect_uri FROM oauth_client_redirect_uris WHERE client_id = ? ORDER BY redirect_uri;

-- name: InsertOAuthClientRedirectURIIfAbsent :exec
INSERT INTO oauth_client_redirect_uris (client_id, redirect_uri) VALUES (?, ?)
ON CONFLICT (client_id, redirect_uri) DO NOTHING;

-- name: DeleteOAuthClientRedirectURI :exec
DELETE FROM oauth_client_redirect_uris WHERE client_id = ? AND redirect_uri = ?;

-- name: InsertAuthorizationCode :exec
INSERT INTO oauth_authorization_codes (code_hash, client_id, user_id, redirect_uri, scope, code_challenge, session_id, issued_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetAuthorizationCode :one
SELECT code_hash, client_id, user_id, redirect_uri, scope, code_challenge, session_id, issued_at, expires_at
FROM oauth_authorization_codes
WHERE code_hash = ?;

-- name: InsertAuthorizationCodeRedemptionIfAbsent :execrows
INSERT INTO oauth_authorization_code_redemptions (code_hash, token_id, redeemed_at) VALUES (?, ?, ?)
ON CONFLICT (code_hash) DO NOTHING;

-- name: GetAuthorizationCodeRedemption :one
SELECT code_hash, token_id, redeemed_at FROM oauth_authorization_code_redemptions WHERE code_hash = ?;

-- name: InsertAccessToken :exec
INSERT INTO oauth_access_tokens (
  id, family_id, client_id, user_id, access_token_hash, refresh_token_hash, scope, rotated_from_token_id,
  issued_at, expires_at, refresh_expires_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- AccessTokenByAccessHash returns a token, whether it has a revocation, and whether its client is disabled
-- or deleted (ADR-0090 section 1).
-- name: GetAccessTokenByAccessHash :one
SELECT t.id, t.family_id, t.client_id, t.user_id, t.access_token_hash, t.refresh_token_hash, t.scope,
  t.rotated_from_token_id, t.issued_at, t.expires_at, t.refresh_expires_at,
  EXISTS (SELECT 1 FROM oauth_token_revocations r WHERE r.token_id = t.id) AS revoked,
  EXISTS (
    SELECT 1 FROM oauth_client_disablements d
    WHERE d.client_id = t.client_id AND NOT EXISTS (SELECT 1 FROM oauth_client_disablement_lifts l WHERE l.disablement_id = d.id)
  ) AS client_disabled,
  EXISTS (SELECT 1 FROM oauth_client_deletions x WHERE x.client_id = t.client_id) AS client_deleted
FROM oauth_access_tokens t
WHERE t.access_token_hash = ?;

-- AccessTokenByRefreshHash returns a token, whether it has a revocation, and whether its client is disabled
-- or deleted (ADR-0090 section 1).
-- name: GetAccessTokenByRefreshHash :one
SELECT t.id, t.family_id, t.client_id, t.user_id, t.access_token_hash, t.refresh_token_hash, t.scope,
  t.rotated_from_token_id, t.issued_at, t.expires_at, t.refresh_expires_at,
  EXISTS (SELECT 1 FROM oauth_token_revocations r WHERE r.token_id = t.id) AS revoked,
  EXISTS (
    SELECT 1 FROM oauth_client_disablements d
    WHERE d.client_id = t.client_id AND NOT EXISTS (SELECT 1 FROM oauth_client_disablement_lifts l WHERE l.disablement_id = d.id)
  ) AS client_disabled,
  EXISTS (SELECT 1 FROM oauth_client_deletions x WHERE x.client_id = t.client_id) AS client_deleted
FROM oauth_access_tokens t
WHERE t.refresh_token_hash = ?;

-- name: InsertTokenRevocationIfAbsent :execrows
INSERT INTO oauth_token_revocations (token_id, reason, revoked_at) VALUES (?, ?, ?)
ON CONFLICT (token_id) DO NOTHING;

-- name: RevokeTokenFamily :exec
INSERT INTO oauth_token_revocations (token_id, reason, revoked_at)
SELECT t.id, sqlc.arg(reason), sqlc.arg(revoked_at) FROM oauth_access_tokens t WHERE t.family_id = sqlc.arg(family_id)
ON CONFLICT (token_id) DO NOTHING;

-- name: RevokeSessionOfTokenFamily :exec
INSERT INTO session_revocations (session_id, reason, revoked_at)
SELECT c.session_id, sqlc.arg(reason), sqlc.arg(revoked_at)
FROM oauth_authorization_code_redemptions r
JOIN oauth_authorization_codes c ON c.code_hash = r.code_hash
WHERE r.token_id = sqlc.arg(family_id) AND c.session_id IS NOT NULL
ON CONFLICT (session_id) DO NOTHING;

-- RevokeTokensOfUser revokes every token of a user whose access or refresh token has not expired
-- at revoked_at and that has no revocation yet.
-- name: RevokeTokensOfUser :exec
INSERT INTO oauth_token_revocations (token_id, reason, revoked_at)
SELECT t.id, sqlc.arg(reason), sqlc.arg(revoked_at) FROM oauth_access_tokens t
WHERE t.user_id = sqlc.arg(user_id)
  AND (t.expires_at > sqlc.arg(revoked_at) OR t.refresh_expires_at > sqlc.arg(revoked_at))
ON CONFLICT (token_id) DO NOTHING;

-- RevokeOtherTokensOfUser revokes every token of a user outside the family keep_family_id whose
-- access or refresh token has not expired at revoked_at and that has no revocation yet.
-- name: RevokeOtherTokensOfUser :exec
INSERT INTO oauth_token_revocations (token_id, reason, revoked_at)
SELECT t.id, sqlc.arg(reason), sqlc.arg(revoked_at) FROM oauth_access_tokens t
WHERE t.user_id = sqlc.arg(user_id) AND t.family_id <> sqlc.arg(keep_family_id)
  AND (t.expires_at > sqlc.arg(revoked_at) OR t.refresh_expires_at > sqlc.arg(revoked_at))
ON CONFLICT (token_id) DO NOTHING;

-- TokenFamilyActive reports whether a token of the family has no revocation and an access or
-- refresh token that has not expired at now: whether the grant can still be used or refreshed.
-- name: TokenFamilyActive :one
SELECT EXISTS (
  SELECT 1 FROM oauth_access_tokens t
  WHERE t.family_id = sqlc.arg(family_id)
    AND (t.expires_at > sqlc.arg(now) OR t.refresh_expires_at > sqlc.arg(now))
    AND NOT EXISTS (SELECT 1 FROM oauth_token_revocations r WHERE r.token_id = t.id)
) AS active;

-- RevokeTokensOfClient revokes every token of a client whose access or refresh token has not
-- expired at revoked_at and that has no revocation yet.
-- name: RevokeTokensOfClient :exec
INSERT INTO oauth_token_revocations (token_id, reason, revoked_at)
SELECT t.id, sqlc.arg(reason), sqlc.arg(revoked_at) FROM oauth_access_tokens t
WHERE t.client_id = sqlc.arg(client_id)
  AND (t.expires_at > sqlc.arg(revoked_at) OR t.refresh_expires_at > sqlc.arg(revoked_at))
ON CONFLICT (token_id) DO NOTHING;

-- ListUnusedDynamicOAuthClients returns the IDs of the dynamically registered clients that have no
-- deletion and are unused (ADR-0090 section 1), oldest first: no authorization code was exchanged
-- and the client was registered before unredeemed_before, or it has no token that was issued after
-- idle_before, whose family was used after idle_before (oauth_token_family_last_used), or that is
-- still usable at now.
-- name: ListUnusedDynamicOAuthClients :many
SELECT c.id FROM oauth_clients c
WHERE c.registration = 'dynamic'
  AND NOT EXISTS (SELECT 1 FROM oauth_client_deletions x WHERE x.client_id = c.id)
  AND (
    (c.created_at < sqlc.arg(unredeemed_before) AND NOT EXISTS (
      SELECT 1 FROM oauth_authorization_codes a
      JOIN oauth_authorization_code_redemptions r ON r.code_hash = a.code_hash
      WHERE a.client_id = c.id))
    OR
    (c.created_at < sqlc.arg(idle_before) AND NOT EXISTS (
      SELECT 1 FROM oauth_access_tokens t
      WHERE t.client_id = c.id
        AND (t.issued_at >= sqlc.arg(idle_before)
          OR EXISTS (SELECT 1 FROM oauth_token_family_last_used u WHERE u.family_id = t.family_id AND u.last_used_at >= sqlc.arg(idle_before))
          OR ((t.expires_at > sqlc.arg(now) OR t.refresh_expires_at > sqlc.arg(now))
            AND NOT EXISTS (SELECT 1 FROM oauth_token_revocations v WHERE v.token_id = t.id)))))
  )
ORDER BY c.created_at, c.id
LIMIT sqlc.arg(batch_limit);

-- UpsertOAuthClientMetadataDocument stores the last fetch of a client's metadata document.
-- name: UpsertOAuthClientMetadataDocument :exec
INSERT INTO oauth_client_metadata_documents (client_id, client_name, client_uri, logo_uri, scope, fetched_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (client_id) DO UPDATE SET
  client_name = excluded.client_name, client_uri = excluded.client_uri, logo_uri = excluded.logo_uri,
  scope = excluded.scope, fetched_at = excluded.fetched_at, expires_at = excluded.expires_at;

-- name: GetOAuthClientMetadataDocument :one
SELECT client_id, client_name, client_uri, logo_uri, scope, fetched_at, expires_at
FROM oauth_client_metadata_documents
WHERE client_id = ?;

-- InsertOAuthClientHostDenialIfAllowed records a denial unless the host is denied already, so
-- that repeating it adds nothing.
-- name: InsertOAuthClientHostDenialIfAllowed :execrows
INSERT INTO oauth_client_host_denials (id, host, denied_by_user_id, denied_at)
SELECT sqlc.arg(id), sqlc.arg(host), sqlc.narg(denied_by_user_id), sqlc.arg(denied_at)
WHERE NOT EXISTS (
    SELECT 1 FROM oauth_client_host_denials d
    WHERE d.host = sqlc.arg(host) AND NOT EXISTS (SELECT 1 FROM oauth_client_host_denial_lifts l WHERE l.denial_id = d.id)
  );

-- LiftOAuthClientHostDenials lifts every denial of a host that has no lift yet.
-- name: LiftOAuthClientHostDenials :execrows
INSERT INTO oauth_client_host_denial_lifts (denial_id, lifted_by_user_id, lifted_at)
SELECT d.id, sqlc.narg(lifted_by_user_id), sqlc.arg(lifted_at) FROM oauth_client_host_denials d
WHERE d.host = sqlc.arg(host)
ON CONFLICT (denial_id) DO NOTHING;

-- OAuthClientHostDenied reports whether a host has a denial without a lift.
-- name: OAuthClientHostDenied :one
SELECT EXISTS (
  SELECT 1 FROM oauth_client_host_denials d
  WHERE d.host = sqlc.arg(host) AND NOT EXISTS (SELECT 1 FROM oauth_client_host_denial_lifts l WHERE l.denial_id = d.id)
) AS denied;

-- ListOAuthClients lists the clients that are not deleted, newest first, optionally of one
-- registration route, with whether each is disabled.
-- name: ListOAuthClients :many
SELECT c.id, c.client_id, c.client_secret_hash, c.name, c.website, c.scope, c.registration, c.owner_user_id, c.created_at,
  EXISTS (
    SELECT 1 FROM oauth_client_disablements d
    WHERE d.client_id = c.id AND NOT EXISTS (SELECT 1 FROM oauth_client_disablement_lifts l WHERE l.disablement_id = d.id)
  ) AS disabled
FROM oauth_clients c
WHERE NOT EXISTS (SELECT 1 FROM oauth_client_deletions x WHERE x.client_id = c.id)
  AND (CAST(sqlc.arg(registration) AS TEXT) = '' OR c.registration = sqlc.arg(registration))
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR c.id < sqlc.arg(before))
ORDER BY c.id DESC
LIMIT sqlc.arg(page_limit);

-- ListMetadataDocumentClientsOfHost lists the metadata document clients whose client_id may be
-- on a host; the caller keeps those whose host is exactly it.
-- name: ListMetadataDocumentClientsOfHost :many
SELECT id, client_id FROM oauth_clients
WHERE registration = 'metadata_document'
  AND (client_id LIKE 'https://' || sqlc.arg(host) || '/%' OR client_id LIKE 'https://' || sqlc.arg(host) || ':%');

-- ListOAuthClientHostDenials lists the denials of hosts that have no lift, newest first.
-- name: ListOAuthClientHostDenials :many
SELECT d.id, d.host, d.denied_by_user_id, d.denied_at
FROM oauth_client_host_denials d
WHERE NOT EXISTS (SELECT 1 FROM oauth_client_host_denial_lifts l WHERE l.denial_id = d.id)
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR d.id < sqlc.arg(before))
ORDER BY d.id DESC
LIMIT sqlc.arg(page_limit);

-- UpsertTokenFamilyLastUsed records that a token family authenticated a request at used_at.
-- name: UpsertTokenFamilyLastUsed :exec
INSERT INTO oauth_token_family_last_used (family_id, last_used_at)
VALUES (?, ?)
ON CONFLICT (family_id) DO UPDATE SET last_used_at = excluded.last_used_at;

-- ListAuthorizedAppTokens returns the tokens of a user that still work (no revocation, an access
-- or refresh token that has not expired at now) with the client, when the token's family was
-- first issued, and when it was last used (NULL when it never was recorded).
-- name: ListAuthorizedAppTokens :many
SELECT t.family_id, t.scope AS token_scope,
  CAST((SELECT min(f.issued_at) FROM oauth_access_tokens f WHERE f.family_id = t.family_id) AS INTEGER) AS authorized_at,
  u.last_used_at,
  c.id, c.client_id, c.client_secret_hash, c.name, c.website, c.scope, c.registration, c.owner_user_id, c.created_at
FROM oauth_access_tokens t
JOIN oauth_clients c ON c.id = t.client_id
LEFT JOIN oauth_token_family_last_used u ON u.family_id = t.family_id
WHERE t.user_id = sqlc.arg(user_id)
  AND (t.expires_at > sqlc.arg(now) OR t.refresh_expires_at > sqlc.arg(now))
  AND NOT EXISTS (SELECT 1 FROM oauth_token_revocations r WHERE r.token_id = t.id)
ORDER BY t.family_id;

-- RevokeTokensOfUserAndClient revokes every token of the client that the user holds and that has
-- no revocation yet.
-- name: RevokeTokensOfUserAndClient :execrows
INSERT INTO oauth_token_revocations (token_id, reason, revoked_at)
SELECT t.id, sqlc.arg(reason), sqlc.arg(revoked_at) FROM oauth_access_tokens t
WHERE t.user_id = sqlc.arg(user_id) AND t.client_id = sqlc.arg(client_id)
ON CONFLICT (token_id) DO NOTHING;
