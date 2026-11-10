-- SPDX-License-Identifier: AGPL-3.0-only

-- The date polls of the events (ADR-0161 section 7): the candidates (resources) and their
-- withdrawals (events), the votes (current values), and the candidate a confirmation fixed.

-- name: InsertGatheringCandidate :exec
INSERT INTO gathering_candidates (id, gathering_id, starts_at, ends_at, added_by_account_id, added_at) VALUES (?, ?, ?, ?, ?, ?);

-- name: InsertGatheringCandidateWithdrawal :execrows
INSERT INTO gathering_candidate_withdrawals (candidate_id, withdrawn_by_account_id, withdrawn_at) VALUES (?, ?, ?)
ON CONFLICT (candidate_id) DO NOTHING;

-- GetGatheringCandidate returns a candidate of an event, whether it was withdrawn, and whether a
-- confirmation fixed it.
-- name: GetGatheringCandidate :one
SELECT k.id, k.gathering_id, k.starts_at, k.ends_at, k.added_by_account_id, k.added_at,
  EXISTS (SELECT 1 FROM gathering_candidate_withdrawals w WHERE w.candidate_id = k.id) AS withdrawn,
  EXISTS (SELECT 1 FROM gathering_confirmation_candidates f WHERE f.candidate_id = k.id) AS confirmed
FROM gathering_candidates k
WHERE k.gathering_id = ? AND k.id = ?;

-- ListGatheringCandidates returns the candidates of an event that were not withdrawn, by the
-- earliest start, with how many accounts voted each answer, the vote of an account ('' for none),
-- and whether a confirmation fixed the candidate.
-- name: ListGatheringCandidates :many
SELECT k.id, k.gathering_id, k.starts_at, k.ends_at, k.added_by_account_id, k.added_at,
  CAST((SELECT COUNT(*) FROM gathering_votes o WHERE o.candidate_id = k.id AND o.vote = 'yes') AS INTEGER) AS yes_count,
  CAST((SELECT COUNT(*) FROM gathering_votes o WHERE o.candidate_id = k.id AND o.vote = 'maybe') AS INTEGER) AS maybe_count,
  CAST((SELECT COUNT(*) FROM gathering_votes o WHERE o.candidate_id = k.id AND o.vote = 'no') AS INTEGER) AS no_count,
  CAST(COALESCE((SELECT o.vote FROM gathering_votes o WHERE o.candidate_id = k.id AND o.account_id = sqlc.arg(account_id)), '') AS TEXT) AS viewer_vote,
  EXISTS (SELECT 1 FROM gathering_confirmation_candidates f WHERE f.candidate_id = k.id) AS confirmed
FROM gathering_candidates k
WHERE k.gathering_id = sqlc.arg(gathering_id)
  AND NOT EXISTS (SELECT 1 FROM gathering_candidate_withdrawals w WHERE w.candidate_id = k.id)
ORDER BY k.starts_at, k.id;

-- name: UpsertGatheringVote :exec
INSERT INTO gathering_votes (candidate_id, account_id, gathering_id, vote, voted_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (candidate_id, account_id) DO UPDATE SET vote = excluded.vote, voted_at = excluded.voted_at;

-- ListGatheringVotes returns the votes of a candidate after an account ID (or from the first with
-- ''), of one answer unless vote_filter is '', in the order of the account IDs.
-- name: ListGatheringVotes :many
SELECT account_id, vote FROM gathering_votes
WHERE candidate_id = sqlc.arg(candidate_id)
  AND (CAST(sqlc.arg(vote_filter) AS TEXT) = '' OR vote = CAST(sqlc.arg(vote_filter) AS TEXT))
  AND account_id > CAST(sqlc.arg(after_id) AS TEXT)
ORDER BY account_id
LIMIT sqlc.arg(row_limit);

-- name: InsertGatheringConfirmationCandidate :exec
INSERT INTO gathering_confirmation_candidates (confirmation_id, candidate_id) VALUES (?, ?);

-- ListGatheringPollAudience returns the accounts after an ID (or from the first with '') that
-- voted in the date poll of an event or are invited to it by an invitation not revoked, each
-- once, in the order of their IDs.
-- name: ListGatheringPollAudience :many
SELECT CAST(x.account_id AS TEXT) AS account_id FROM (
  SELECT o.account_id AS account_id FROM gathering_votes o WHERE o.gathering_id = sqlc.arg(gathering_id)
  UNION
  SELECT i.invitee_account_id AS account_id FROM gathering_invitations i
  WHERE i.gathering_id = sqlc.arg(gathering_id)
    AND NOT EXISTS (SELECT 1 FROM gathering_invitation_revocations r WHERE r.invitation_id = i.id)
) x
WHERE x.account_id > CAST(sqlc.arg(after_id) AS TEXT)
ORDER BY x.account_id
LIMIT sqlc.arg(row_limit);
