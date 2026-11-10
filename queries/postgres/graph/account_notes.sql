-- SPDX-License-Identifier: AGPL-3.0-only

-- Private notes on accounts (current value, FR-310).

-- UpsertAccountNote writes the note of an account on a target, replacing the one it had.
-- name: UpsertAccountNote :exec
INSERT INTO account_notes (account_id, target_account_id, note, updated_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (account_id, target_account_id) DO UPDATE SET
  note = excluded.note,
  updated_at = excluded.updated_at;

-- DeleteAccountNote deletes the note and reports, by the rows it deleted, whether there was one.
-- name: DeleteAccountNote :execrows
DELETE FROM account_notes WHERE account_id = $1 AND target_account_id = $2;

-- name: FindAccountNote :one
SELECT account_id, target_account_id, note, updated_at FROM account_notes
WHERE account_id = $1 AND target_account_id = $2;

-- ListAccountNotesOn returns the notes of an account on those of targets.
-- name: ListAccountNotesOn :many
SELECT account_id, target_account_id, note, updated_at FROM account_notes
WHERE account_id = sqlc.arg(account_id) AND target_account_id = ANY(sqlc.arg(targets)::uuid[])
ORDER BY target_account_id;

-- ListAccountNotesBy returns every note of an account, oldest first, for its data export.
-- name: ListAccountNotesBy :many
SELECT account_id, target_account_id, note, updated_at FROM account_notes
WHERE account_id = $1
ORDER BY updated_at, target_account_id;
