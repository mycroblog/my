-- SPDX-License-Identifier: AGPL-3.0-only

-- The sensitivity overrides and the polls of many statuses at once, one query each, for the pages
-- that present many statuses. Each reads what the query of one of content.sql and poll.sql reads.

-- name: ListStatusesSensitivityOverridden :many
SELECT DISTINCT status_id FROM status_sensitivity_overrides WHERE status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY status_id;

-- name: ListPollTalliesOf :many
SELECT poll_id, position, votes, voters_count FROM poll_tallies WHERE poll_id = ANY(sqlc.arg(poll_ids)::uuid[]) ORDER BY poll_id, position;

-- name: ListPollVotesOfAccountIn :many
SELECT id, poll_id, account_id, position, uri, voted_at FROM poll_votes
WHERE account_id = sqlc.arg(account_id)::uuid AND poll_id = ANY(sqlc.arg(poll_ids)::uuid[]) ORDER BY poll_id, position;

-- ListPollVotesOfAccountsIn is ListPollVotesOfAccountIn of many accounts.
-- name: ListPollVotesOfAccountsIn :many
SELECT id, poll_id, account_id, position, uri, voted_at FROM poll_votes
WHERE account_id = ANY(sqlc.arg(account_ids)::uuid[]) AND poll_id = ANY(sqlc.arg(poll_ids)::uuid[]) ORDER BY account_id, poll_id, position;
