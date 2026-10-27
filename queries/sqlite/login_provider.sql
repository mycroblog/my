-- SPDX-License-Identifier: AGPL-3.0-only

-- External login providers and the external identities linked to users (data-model 4.1,
-- ADR-0106). Providers, revisions, secrets, state changes, external identities, links, and
-- unlinks are written once; login_provider_heads and user_external_identities are the
-- projections.

-- name: InsertLoginProvider :exec
INSERT INTO login_providers (id, created_by_user_id, created_at) VALUES (?, ?, ?);

-- name: InsertLoginProviderRevision :exec
INSERT INTO login_provider_revisions (id, provider_id, kind, display_name, issuer, client_id, scopes,
  allow_signup, allow_link, revised_by_user_id, revised_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertLoginProviderRevisionClaim :exec
INSERT INTO login_provider_revision_claims (revision_id, claim, value) VALUES (?, ?, ?);

-- name: InsertLoginProviderRevisionOrganization :exec
INSERT INTO login_provider_revision_organizations (revision_id, organization) VALUES (?, ?);

-- name: InsertLoginProviderSecret :exec
INSERT INTO login_provider_secrets (id, provider_id, sealed_secret, created_by_user_id, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: InsertLoginProviderStateChange :exec
INSERT INTO login_provider_state_changes (id, provider_id, state, changed_by_user_id, changed_at)
VALUES (?, ?, ?, ?, ?);

-- name: UpsertLoginProviderHead :exec
INSERT INTO login_provider_heads (provider_id, revision_id, state, secret_id) VALUES (?, ?, ?, ?)
ON CONFLICT (provider_id) DO UPDATE SET
  revision_id = excluded.revision_id, state = excluded.state, secret_id = excluded.secret_id;

-- name: GetLoginProviderHeadForUpdate :one
SELECT provider_id, revision_id, state, secret_id FROM login_provider_heads WHERE provider_id = ?;

-- ListLoginProviders returns the providers that are not deleted, newest first, with their
-- current revision and last state change; only the provider id unless it is empty.
-- name: ListLoginProviders :many
SELECT p.id, p.created_by_user_id, p.created_at, h.revision_id, h.state, h.secret_id,
  r.kind, r.display_name, r.issuer, r.client_id, r.scopes, r.allow_signup, r.allow_link, r.revised_at,
  c.changed_at AS state_changed_at
FROM login_provider_heads h
JOIN login_providers p ON p.id = h.provider_id
JOIN login_provider_revisions r ON r.id = h.revision_id
JOIN login_provider_state_changes c ON c.id = (
  SELECT c2.id FROM login_provider_state_changes c2 WHERE c2.provider_id = h.provider_id
  ORDER BY c2.id DESC LIMIT 1
)
WHERE h.state <> 'deleted' AND (CAST(sqlc.arg(id) AS TEXT) = '' OR h.provider_id = sqlc.arg(id))
ORDER BY p.id DESC;

-- ListLoginProviderRevisionClaims and ListLoginProviderRevisionOrganizations take the revisions
-- ids as a JSON array of strings.
-- name: ListLoginProviderRevisionClaims :many
SELECT revision_id, claim, value FROM login_provider_revision_claims
WHERE revision_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))
ORDER BY revision_id, claim, value;

-- name: ListLoginProviderRevisionOrganizations :many
SELECT revision_id, organization FROM login_provider_revision_organizations
WHERE revision_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))
ORDER BY revision_id, organization;

-- GetCurrentLoginProviderSecret returns the current secret of a provider that is not deleted.
-- name: GetCurrentLoginProviderSecret :one
SELECT s.id, s.provider_id, s.sealed_secret, s.created_by_user_id, s.created_at
FROM login_provider_heads h JOIN login_provider_secrets s ON s.id = h.secret_id
WHERE h.provider_id = ? AND h.state <> 'deleted';

-- InsertExternalIdentity inserts an external identity unless one with the same provider and
-- subject exists.
-- name: InsertExternalIdentity :exec
INSERT INTO external_identities (id, provider_id, subject, created_at) VALUES (?, ?, ?, ?)
ON CONFLICT (provider_id, subject) DO NOTHING;

-- name: GetExternalIdentity :one
SELECT id, provider_id, subject, created_at FROM external_identities
WHERE provider_id = ? AND subject = ?;

-- name: InsertExternalIdentityLink :exec
INSERT INTO external_identity_links (id, external_identity_id, user_id, linked_at) VALUES (?, ?, ?, ?);

-- InsertUserExternalIdentity adds a link to the current links unless its external identity is
-- linked or its user has a link to the provider; the caller checks that it inserted a row.
-- name: InsertUserExternalIdentity :execrows
INSERT INTO user_external_identities (external_identity_id, link_id, user_id, provider_id)
VALUES (?, ?, ?, ?)
ON CONFLICT DO NOTHING;

-- name: GetUserExternalIdentityByIdentity :one
SELECT link_id, user_id FROM user_external_identities WHERE external_identity_id = ?;

-- name: InsertExternalIdentityUnlink :execrows
INSERT INTO external_identity_unlinks (link_id, unlinked_at) VALUES (?, ?)
ON CONFLICT (link_id) DO NOTHING;

-- name: DeleteUserExternalIdentityByLink :exec
DELETE FROM user_external_identities WHERE link_id = ?;

-- FindLinkedExternalIdentity returns the current link of the external identity with a provider
-- and a subject.
-- name: FindLinkedExternalIdentity :one
SELECT u.link_id, u.external_identity_id, u.provider_id, x.subject, u.user_id, l.linked_at
FROM external_identities x
JOIN user_external_identities u ON u.external_identity_id = x.id
JOIN external_identity_links l ON l.id = u.link_id
WHERE x.provider_id = ? AND x.subject = ?;

-- name: ListLinkedExternalIdentities :many
SELECT u.link_id, u.external_identity_id, u.provider_id, x.subject, u.user_id, l.linked_at
FROM user_external_identities u
JOIN external_identities x ON x.id = u.external_identity_id
JOIN external_identity_links l ON l.id = u.link_id
WHERE u.user_id = ?
ORDER BY u.link_id;

-- CountLoginProviderLinks counts the users linked to each provider that has one.
-- name: CountLoginProviderLinks :many
SELECT provider_id, COUNT(*) AS users FROM user_external_identities GROUP BY provider_id;
