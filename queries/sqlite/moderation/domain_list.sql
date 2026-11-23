-- SPDX-License-Identifier: AGPL-3.0-only

-- The subscriptions to domain block lists (data-model 4.3, ADR-0159 section 5): the subscriptions,
-- their settings and deletion, their fetches, the entries of the last list read, the actions
-- they issued, and the rejected drafts.

-- name: InsertDomainListSubscription :exec
INSERT INTO domain_list_subscriptions (id, url, format, created_by_user_id, created_at) VALUES (?, ?, ?, ?, ?);

-- name: InsertDomainListSubscriptionRevision :exec
INSERT INTO domain_list_subscription_revisions (
  id, subscription_id, title, mode, suspend_as, silence_as, apply_reject_media, apply_reject_reports,
  public_comment_from_list, revised_by_user_id, revised_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- InsertDomainListSubscriptionDeletion inserts nothing when the subscription is already deleted.
-- name: InsertDomainListSubscriptionDeletion :execrows
INSERT INTO domain_list_subscription_deletions (subscription_id, lift_actions, deleted_by_user_id, deleted_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (subscription_id) DO NOTHING;

-- name: UpsertDomainListSubscriptionHead :exec
INSERT INTO domain_list_subscription_heads (
  subscription_id, etag, last_modified, digest
) VALUES (?, ?, ?, ?)
ON CONFLICT (subscription_id) DO UPDATE SET
  etag = EXCLUDED.etag, last_modified = EXCLUDED.last_modified, digest = EXCLUDED.digest;

-- GetDomainListSubscription returns a subscription with its settings in effect (the newest
-- revision), its head, its last fetch, and its deletion. The last fetch is the newest, and
-- consecutive_failures counts the failed fetches after the last one that did not fail.
-- name: GetDomainListSubscription :one
SELECT s.id, s.url, s.format, s.created_by_user_id, s.created_at,
  r.id AS revision_id, r.title, r.mode, r.suspend_as, r.silence_as, r.apply_reject_media, r.apply_reject_reports,
  r.public_comment_from_list, r.revised_by_user_id, r.revised_at,
  h.etag, h.last_modified, h.digest, f.id AS last_fetch_id,
  (SELECT COUNT(*) FROM domain_list_fetches g WHERE g.subscription_id = s.id AND g.outcome = 'failed'
    AND NOT EXISTS (SELECT 1 FROM domain_list_fetches k WHERE k.subscription_id = s.id AND k.outcome <> 'failed' AND k.id > g.id)
  ) AS consecutive_failures,
  f.outcome AS last_outcome, f.http_status AS last_http_status, f.error_code AS last_error_code,
  f.entry_count AS last_entry_count, f.fetched_at AS last_fetched_at,
  d.subscription_id AS deletion_subscription_id, d.lift_actions, d.deleted_by_user_id, d.deleted_at
FROM domain_list_subscriptions s
JOIN domain_list_subscription_revisions r ON r.subscription_id = s.id
  AND NOT EXISTS (SELECT 1 FROM domain_list_subscription_revisions rr WHERE rr.subscription_id = s.id AND rr.id > r.id)
JOIN domain_list_subscription_heads h ON h.subscription_id = s.id
LEFT JOIN domain_list_fetches f ON f.id = (
  SELECT g.id FROM domain_list_fetches g WHERE g.subscription_id = s.id ORDER BY g.id DESC LIMIT 1
)
LEFT JOIN domain_list_subscription_deletions d ON d.subscription_id = s.id
WHERE s.id = ?;

-- ListDomainListSubscriptions returns the subscriptions that are not deleted, newest first, as
-- GetDomainListSubscription does, those before the subscription before (empty for the first
-- page).
-- name: ListDomainListSubscriptions :many
SELECT s.id, s.url, s.format, s.created_by_user_id, s.created_at,
  r.id AS revision_id, r.title, r.mode, r.suspend_as, r.silence_as, r.apply_reject_media, r.apply_reject_reports,
  r.public_comment_from_list, r.revised_by_user_id, r.revised_at,
  h.etag, h.last_modified, h.digest, f.id AS last_fetch_id,
  (SELECT COUNT(*) FROM domain_list_fetches g WHERE g.subscription_id = s.id AND g.outcome = 'failed'
    AND NOT EXISTS (SELECT 1 FROM domain_list_fetches k WHERE k.subscription_id = s.id AND k.outcome <> 'failed' AND k.id > g.id)
  ) AS consecutive_failures,
  f.outcome AS last_outcome, f.http_status AS last_http_status, f.error_code AS last_error_code,
  f.entry_count AS last_entry_count, f.fetched_at AS last_fetched_at,
  d.subscription_id AS deletion_subscription_id, d.lift_actions, d.deleted_by_user_id, d.deleted_at
FROM domain_list_subscriptions s
JOIN domain_list_subscription_revisions r ON r.subscription_id = s.id
  AND NOT EXISTS (SELECT 1 FROM domain_list_subscription_revisions rr WHERE rr.subscription_id = s.id AND rr.id > r.id)
JOIN domain_list_subscription_heads h ON h.subscription_id = s.id
LEFT JOIN domain_list_fetches f ON f.id = (
  SELECT g.id FROM domain_list_fetches g WHERE g.subscription_id = s.id ORDER BY g.id DESC LIMIT 1
)
LEFT JOIN domain_list_subscription_deletions d ON d.subscription_id = s.id
WHERE d.subscription_id IS NULL
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR s.id < sqlc.arg(before))
ORDER BY s.id DESC
LIMIT sqlc.arg(lim);

-- name: InsertDomainListFetch :exec
INSERT INTO domain_list_fetches (id, subscription_id, outcome, http_status, error_code, entry_count, fetched_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- DeleteDomainListFetchesBefore deletes at most @batch_limit fetches below @id_before made before
-- @fetched_before, the oldest first, except the last one of their subscription that did not fail
-- and the failed ones after it, from which the head's last fetch and count of failures derive.
-- name: DeleteDomainListFetchesBefore :execrows
DELETE FROM domain_list_fetches WHERE id IN (
  SELECT f.id FROM domain_list_fetches f
  WHERE f.id < CAST(sqlc.arg(id_before) AS TEXT) AND f.fetched_at < CAST(sqlc.arg(fetched_before) AS INTEGER)
    AND EXISTS (
      SELECT 1 FROM domain_list_fetches k
      WHERE k.subscription_id = f.subscription_id AND k.outcome <> 'failed' AND k.id > f.id
    )
  ORDER BY f.id
  LIMIT sqlc.arg(batch_limit)
);

-- ListDomainListFetches returns the fetches of a subscription, newest first, those before the
-- fetch before (empty for the first page).
-- name: ListDomainListFetches :many
SELECT id, subscription_id, outcome, http_status, error_code, entry_count, fetched_at
FROM domain_list_fetches
WHERE subscription_id = sqlc.arg(subscription_id)
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR id < sqlc.arg(before))
ORDER BY id DESC
LIMIT sqlc.arg(lim);

-- name: ListDomainListEntries :many
SELECT subscription_id, domain, severity, reject_media, reject_reports, public_comment, obfuscate, since_fetch_id
FROM domain_list_entries WHERE subscription_id = ?
ORDER BY domain;

-- name: GetDomainListEntry :one
SELECT subscription_id, domain, severity, reject_media, reject_reports, public_comment, obfuscate, since_fetch_id
FROM domain_list_entries WHERE subscription_id = ? AND domain = ?;

-- name: UpsertDomainListEntry :exec
INSERT INTO domain_list_entries (
  subscription_id, domain, severity, reject_media, reject_reports, public_comment, obfuscate, since_fetch_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (subscription_id, domain) DO UPDATE SET
  severity = EXCLUDED.severity, reject_media = EXCLUDED.reject_media, reject_reports = EXCLUDED.reject_reports,
  public_comment = EXCLUDED.public_comment, obfuscate = EXCLUDED.obfuscate, since_fetch_id = EXCLUDED.since_fetch_id;

-- name: DeleteDomainListEntry :exec
DELETE FROM domain_list_entries WHERE subscription_id = ? AND domain = ?;

-- name: DeleteDomainListEntriesOf :exec
DELETE FROM domain_list_entries WHERE subscription_id = ?;

-- name: InsertSubscriptionDomainAction :exec
INSERT INTO subscription_domain_actions (action_id, subscription_id, fetch_id) VALUES (?, ?, ?);

-- name: GetSubscriptionDomainAction :one
SELECT action_id, subscription_id, fetch_id FROM subscription_domain_actions WHERE action_id = ?;

-- ListSubscriptionDomainActionsInEffect returns the actions in effect at now that a subscription
-- issued, by ID: those on a domain, or on every domain when it is empty.
-- name: ListSubscriptionDomainActionsInEffect :many
SELECT a.id, a.domain, a.kind, a.public_comment, a.private_note, a.obfuscate, a.issued_by_user_id,
  a.expires_at, a.issued_at
FROM subscription_domain_actions sa
JOIN domain_moderation_actions a ON a.id = sa.action_id
WHERE sa.subscription_id = sqlc.arg(subscription_id)
  AND (CAST(sqlc.arg(domain) AS TEXT) = '' OR a.domain = sqlc.arg(domain))
  AND NOT EXISTS (SELECT 1 FROM domain_moderation_action_lifts l WHERE l.action_id = a.id)
  AND (a.expires_at IS NULL OR a.expires_at > CAST(sqlc.arg(now) AS INTEGER))
ORDER BY a.id;

-- name: InsertDomainListDraftRejection :exec
INSERT INTO domain_list_draft_rejections (
  id, subscription_id, change_kind, domain, kind, entry_since_fetch_id, action_id, rejected_by_user_id, rejected_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListDomainListDraftRejections :many
SELECT id, subscription_id, change_kind, domain, kind, entry_since_fetch_id, action_id, rejected_by_user_id, rejected_at
FROM domain_list_draft_rejections WHERE subscription_id = ?
ORDER BY id;
