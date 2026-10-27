-- SPDX-License-Identifier: AGPL-3.0-only

-- LockModerationTarget takes a transaction-scoped advisory lock on the restrictions of one
-- account or domain, so that the actions and lifts on it serialize under READ COMMITTED.
-- name: LockModerationTarget :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key)::text, 0));

-- Account actions ---------------------------------------------------------------------------

-- name: InsertModerationAction :exec
INSERT INTO moderation_actions (
  id, target_account_id, kind, reason_category, public_reason, private_note, report_id, issuer,
  issued_by_user_id, notify_target, expires_at, issued_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: GetModerationAction :one
SELECT id, target_account_id, kind, reason_category, public_reason, private_note, report_id, issuer,
  issued_by_user_id, notify_target, expires_at, issued_at
FROM moderation_actions WHERE id = $1;

-- InsertModerationActionLift inserts nothing when the action is already lifted.
-- name: InsertModerationActionLift :execrows
INSERT INTO moderation_action_lifts (action_id, lifted_by_user_id, reason, lifted_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (action_id) DO NOTHING;

-- name: InsertAccountRestriction :exec
INSERT INTO account_restrictions (account_id, restriction, source_action_id, effective_until) VALUES ($1, $2, $3, $4);

-- name: DeleteAccountRestrictionsOfAction :exec
DELETE FROM account_restrictions WHERE source_action_id = $1;

-- name: ListAccountRestrictionRows :many
SELECT account_id, restriction, source_action_id, effective_until
FROM account_restrictions WHERE account_id = $1
ORDER BY restriction, source_action_id;

-- DeleteExpiredAccountRestrictions deletes at most batch_limit rows of any account that have
-- expired at now, the earliest expired first, and returns them. A row that another transaction
-- deletes meanwhile is returned by that one only.
-- name: DeleteExpiredAccountRestrictions :many
DELETE FROM account_restrictions
WHERE effective_until <= sqlc.arg(now)::timestamptz
  AND (account_id, restriction, source_action_id) IN (
    SELECT r.account_id, r.restriction, r.source_action_id FROM account_restrictions AS r
    WHERE r.effective_until <= sqlc.arg(now)::timestamptz
    ORDER BY r.effective_until
    LIMIT sqlc.arg(batch_limit)::integer
  )
RETURNING account_id, restriction, source_action_id, effective_until;

-- ListAccountRestrictionsInEffect returns the distinct restrictions of an account in effect at now.
-- name: ListAccountRestrictionsInEffect :many
SELECT DISTINCT restriction FROM account_restrictions
WHERE account_id = sqlc.arg(account_id)
  AND (effective_until IS NULL OR effective_until > sqlc.arg(now)::timestamptz)
ORDER BY restriction;

-- Domain actions ----------------------------------------------------------------------------

-- name: InsertDomainModerationAction :exec
INSERT INTO domain_moderation_actions (
  id, domain, kind, public_comment, private_note, obfuscate, issued_by_user_id, expires_at, issued_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetDomainModerationAction :one
SELECT id, domain, kind, public_comment, private_note, obfuscate, issued_by_user_id, expires_at, issued_at
FROM domain_moderation_actions WHERE id = $1;

-- InsertDomainModerationActionLift inserts nothing when the action is already lifted.
-- name: InsertDomainModerationActionLift :execrows
INSERT INTO domain_moderation_action_lifts (action_id, lifted_by_user_id, lifted_at) VALUES ($1, $2, $3)
ON CONFLICT (action_id) DO NOTHING;

-- name: InsertDomainRestriction :exec
INSERT INTO domain_restrictions (domain, restriction, source_action_id, effective_until) VALUES ($1, $2, $3, $4);

-- name: DeleteDomainRestrictionsOfAction :exec
DELETE FROM domain_restrictions WHERE source_action_id = $1;

-- name: ListDomainRestrictionRows :many
SELECT domain, restriction, source_action_id, effective_until
FROM domain_restrictions WHERE domain = $1
ORDER BY restriction, source_action_id;

-- ListDomainRestrictionsInEffect returns the distinct restrictions in effect at now of any of the
-- domains.
-- name: ListDomainRestrictionsInEffect :many
SELECT DISTINCT restriction FROM domain_restrictions
WHERE (effective_until IS NULL OR effective_until > sqlc.arg(now)::timestamptz)
  AND domain = ANY(sqlc.arg(domains)::text[])
ORDER BY restriction;

-- Reports -----------------------------------------------------------------------------------

-- InsertReport inserts nothing when a report with the URI exists.
-- name: InsertReport :execrows
INSERT INTO reports (id, uri, reporter_account_id, target_account_id, category, comment, forwarded, filed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (uri) DO NOTHING;

-- name: InsertReportStatus :exec
INSERT INTO report_statuses (report_id, status_id) VALUES ($1, $2)
ON CONFLICT (report_id, status_id) DO NOTHING;

-- name: InsertReportRuleCitation :exec
INSERT INTO report_rule_citations (report_id, rule_id) VALUES ($1, $2)
ON CONFLICT (report_id, rule_id) DO NOTHING;

-- name: UpsertReportState :exec
INSERT INTO report_states (report_id, state, outcome, assignee_user_id, since) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (report_id) DO UPDATE SET
  state = excluded.state, outcome = excluded.outcome, assignee_user_id = excluded.assignee_user_id,
  since = excluded.since;

-- name: GetReport :one
SELECT id, uri, reporter_account_id, target_account_id, category, comment, forwarded, filed_at
FROM reports WHERE id = $1;

-- name: GetReportByURI :one
SELECT id, uri, reporter_account_id, target_account_id, category, comment, forwarded, filed_at
FROM reports WHERE uri = $1;

-- name: ListReportStatusIDs :many
SELECT status_id FROM report_statuses WHERE report_id = $1 ORDER BY status_id;

-- name: InsertReportVoiceRoom :exec
INSERT INTO report_voice_rooms (report_id, room_id, revision_id) VALUES ($1, $2, $3)
ON CONFLICT (report_id) DO NOTHING;

-- name: GetReportVoiceRoom :many
SELECT room_id, revision_id FROM report_voice_rooms WHERE report_id = $1;

-- name: ListReportedVoiceRooms :many
SELECT DISTINCT room_id FROM report_voice_rooms WHERE room_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY room_id;

-- name: ListReportRuleIDs :many
SELECT rule_id FROM report_rule_citations WHERE report_id = $1 ORDER BY rule_id;

-- name: GetReportState :one
SELECT report_id, state, outcome, assignee_user_id, since FROM report_states WHERE report_id = $1;

-- ListReportsOfTarget returns the reports of a target account, oldest first.
-- name: ListReportsOfTarget :many
SELECT id, uri, reporter_account_id, target_account_id, category, comment, forwarded, filed_at
FROM reports WHERE target_account_id = $1 ORDER BY id;

-- ListReportsByReporter returns the reports that an account filed with their state, newest
-- first, those before the report before (empty for the first page).
-- name: ListReportsByReporter :many
SELECT r.id, r.uri, r.reporter_account_id, r.target_account_id, r.category, r.comment, r.forwarded,
  r.filed_at, s.state, s.outcome, s.assignee_user_id, s.since
FROM reports r JOIN report_states s ON s.report_id = r.id
WHERE r.reporter_account_id = sqlc.arg(reporter_account_id)::uuid
  AND (sqlc.narg(before)::uuid IS NULL OR r.id < sqlc.narg(before)::uuid)
ORDER BY r.id DESC
LIMIT sqlc.arg(lim);

-- Moderators' handling of reports and domain actions -----------------------------------------

-- name: InsertReportAssignment :exec
INSERT INTO report_assignments (id, report_id, assignee_user_id, assigned_by_user_id, assigned_at)
VALUES ($1, $2, $3, $4, $5);

-- name: InsertReportResolution :exec
INSERT INTO report_resolutions (id, report_id, outcome, resolved_by_user_id, resolved_at)
VALUES ($1, $2, $3, $4, $5);

-- GetReportStateForUpdate reads and locks the state of a report inside the transaction that
-- changes it.
-- name: GetReportStateForUpdate :one
SELECT report_id, state, outcome, assignee_user_id, since FROM report_states WHERE report_id = $1
FOR UPDATE;

-- ListReports returns the reports in a state (empty for any) with their state, newest first,
-- those before the report before (empty for the first page).
-- name: ListReports :many
SELECT r.id, r.uri, r.reporter_account_id, r.target_account_id, r.category, r.comment, r.forwarded,
  r.filed_at, s.state, s.outcome, s.assignee_user_id, s.since
FROM reports r JOIN report_states s ON s.report_id = r.id
WHERE (sqlc.arg(state)::text = '' OR s.state = sqlc.arg(state)::text)
  AND (sqlc.narg(before)::uuid IS NULL OR r.id < sqlc.narg(before)::uuid)
ORDER BY r.id DESC
LIMIT sqlc.arg(lim);

-- ListDomainModerationActions returns the domain actions with their lifts, newest first: those
-- of a domain (empty for every domain), of the kinds (empty for every kind), only those in effect
-- at in_effect_at unless it is NULL, and those before the action before (empty for the first
-- page).
-- name: ListDomainModerationActions :many
SELECT a.id, a.domain, a.kind, a.public_comment, a.private_note, a.obfuscate, a.issued_by_user_id,
  a.expires_at, a.issued_at, l.action_id AS lift_action_id, l.lifted_by_user_id, l.lifted_at
FROM domain_moderation_actions a
LEFT JOIN domain_moderation_action_lifts l ON l.action_id = a.id
WHERE (sqlc.arg(domain)::text = '' OR a.domain = sqlc.arg(domain)::text)
  AND (cardinality(sqlc.arg(kinds)::text[]) = 0 OR a.kind = ANY(sqlc.arg(kinds)::text[]))
  AND (sqlc.narg(in_effect_at)::timestamptz IS NULL
    OR (l.action_id IS NULL AND (a.expires_at IS NULL OR a.expires_at > sqlc.narg(in_effect_at)::timestamptz)))
  AND (sqlc.narg(before)::uuid IS NULL OR a.id < sqlc.narg(before)::uuid)
ORDER BY a.id DESC
LIMIT sqlc.arg(lim);

-- name: GetDomainModerationActionRecord :one
SELECT a.id, a.domain, a.kind, a.public_comment, a.private_note, a.obfuscate, a.issued_by_user_id,
  a.expires_at, a.issued_at, l.action_id AS lift_action_id, l.lifted_by_user_id, l.lifted_at
FROM domain_moderation_actions a
LEFT JOIN domain_moderation_action_lifts l ON l.action_id = a.id
WHERE a.id = $1;

-- ListModerationActions returns the actions on accounts with their lifts, newest first: those
-- on an account (empty for every account), only those in effect at in_effect_at unless it is
-- NULL, and those before the action before (empty for the first page).
-- name: ListModerationActions :many
SELECT a.id, a.target_account_id, a.kind, a.reason_category, a.public_reason, a.private_note,
  a.report_id, a.issuer, a.issued_by_user_id, a.notify_target, a.expires_at, a.issued_at,
  l.action_id AS lift_action_id, l.lifted_by_user_id, l.reason AS lift_reason, l.lifted_at
FROM moderation_actions a
LEFT JOIN moderation_action_lifts l ON l.action_id = a.id
WHERE (sqlc.narg(account_id)::uuid IS NULL OR a.target_account_id = sqlc.narg(account_id)::uuid)
  AND (sqlc.narg(in_effect_at)::timestamptz IS NULL
    OR (l.action_id IS NULL AND (a.expires_at IS NULL OR a.expires_at > sqlc.narg(in_effect_at)::timestamptz)))
  AND (sqlc.narg(before)::uuid IS NULL OR a.id < sqlc.narg(before)::uuid)
ORDER BY a.id DESC
LIMIT sqlc.arg(lim);

-- name: GetModerationActionRecord :one
SELECT a.id, a.target_account_id, a.kind, a.reason_category, a.public_reason, a.private_note,
  a.report_id, a.issuer, a.issued_by_user_id, a.notify_target, a.expires_at, a.issued_at,
  l.action_id AS lift_action_id, l.lifted_by_user_id, l.reason AS lift_reason, l.lifted_at
FROM moderation_actions a
LEFT JOIN moderation_action_lifts l ON l.action_id = a.id
WHERE a.id = $1;

-- Moderator notifications and counts (#338) ---------------------------------------------------

-- CountReportsInState counts the reports in a state.
-- name: CountReportsInState :one
SELECT COUNT(*) FROM report_states WHERE state = $1;

-- HasOtherOpenReportOf reports whether a report of the target other than the one given is open.
-- name: HasOtherOpenReportOf :one
SELECT EXISTS (
  SELECT 1 FROM reports r JOIN report_states s ON s.report_id = r.id
  WHERE r.target_account_id = sqlc.arg(target_account_id)::uuid AND r.id <> sqlc.arg(report_id)::uuid AND s.state = 'open'
);

-- Notes on reports and their reopenings (#333) -------------------------------------------------

-- name: InsertReportReopening :exec
INSERT INTO report_reopenings (id, report_id, reopened_by_user_id, reopened_at) VALUES ($1, $2, $3, $4);

-- name: InsertReportNote :exec
INSERT INTO report_notes (id, report_id, author_user_id, body, written_at) VALUES ($1, $2, $3, $4, $5);

-- ListReportNotes returns the notes on a report, oldest first.
-- name: ListReportNotes :many
SELECT id, report_id, author_user_id, body, written_at FROM report_notes WHERE report_id = $1 ORDER BY id;

-- CountOtherReportsOfTarget counts the reports of a target account other than the one given.
-- name: CountOtherReportsOfTarget :one
SELECT COUNT(*) FROM reports
WHERE target_account_id = sqlc.arg(target_account_id)::uuid AND id <> sqlc.arg(report_id)::uuid;

-- CountModerationActionsOfTarget counts the actions on an account, ended ones included.
-- name: CountModerationActionsOfTarget :one
SELECT COUNT(*) FROM moderation_actions WHERE target_account_id = $1;
