-- SPDX-License-Identifier: AGPL-3.0-only

-- Private notes on accounts (current value, FR-310). A list argument is a JSON array of strings,
-- read with json_each.

-- UpsertAccountNote writes the note of an account on a target, replacing the one it had.
-- name: UpsertAccountNote :exec
INSERT INTO account_notes (account_id, target_account_id, note, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (account_id, target_account_id) DO UPDATE SET
  note = excluded.note,
  updated_at = excluded.updated_at;

-- DeleteAccountNote deletes the note and reports, by the rows it deleted, whether there was one.
-- name: DeleteAccountNote :execrows
DELETE FROM account_notes WHERE account_id = ? AND target_account_id = ?;

-- name: FindAccountNote :one
SELECT account_id, target_account_id, note, updated_at FROM account_notes
WHERE account_id = ? AND target_account_id = ?;

-- ListAccountNotesOn returns the notes of an account on those of targets.
-- name: ListAccountNotesOn :many
SELECT account_id, target_account_id, note, updated_at FROM account_notes
WHERE account_id = sqlc.arg(account_id) AND target_account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(targets) AS TEXT)))
ORDER BY target_account_id;

-- ListAccountNotesBy returns every note of an account, oldest first, for its data export.
-- name: ListAccountNotesBy :many
SELECT account_id, target_account_id, note, updated_at FROM account_notes
WHERE account_id = ?
ORDER BY updated_at, target_account_id;
