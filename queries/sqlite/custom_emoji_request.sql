-- SPDX-License-Identifier: AGPL-3.0-only
-- The requests of the holders of create_custom_emojis and their outcomes (ADR-0070 sections 2 and 7). Owned
-- by internal/app/content.

-- name: InsertCustomEmojiRequest :exec
INSERT INTO custom_emoji_requests (id, kind, requester_user_id, emoji_id, shortcode, media_id, created_at)
VALUES (@id, @kind, @requester_user_id, @emoji_id, @shortcode, @media_id, @created_at);

-- name: InsertCustomEmojiPendingRequest :exec
INSERT INTO custom_emoji_pending_requests (request_id, requester_user_id, reserved_shortcode, emoji_id)
VALUES (@request_id, @requester_user_id, @reserved_shortcode, @emoji_id);

-- name: DeleteCustomEmojiPendingRequest :execrows
DELETE FROM custom_emoji_pending_requests WHERE request_id = @request_id;

-- name: InsertCustomEmojiRequestOutcome :exec
INSERT INTO custom_emoji_request_outcomes (id, request_id, outcome, decided_by_user_id, reason, emoji_id, decided_at)
VALUES (@id, @request_id, @outcome, @decided_by_user_id, @reason, @emoji_id, @decided_at);

-- LocalCustomEmojiShortcodeTaken reports whether a local emoji has @shortcode.
-- name: LocalCustomEmojiShortcodeTaken :one
SELECT CAST(EXISTS (SELECT 1 FROM custom_emojis WHERE shortcode = @shortcode AND domain = '') AS INTEGER) AS taken;

-- CustomEmojiShortcodeReserved reports whether a pending request to create reserves @shortcode.
-- name: CustomEmojiShortcodeReserved :one
SELECT CAST(EXISTS (SELECT 1 FROM custom_emoji_pending_requests WHERE reserved_shortcode = CAST(@shortcode AS TEXT)) AS INTEGER) AS reserved;

-- name: GetPendingImageRequestOfEmoji :one
SELECT request_id FROM custom_emoji_pending_requests WHERE emoji_id = CAST(@emoji_id AS TEXT);

-- CountCustomEmojiUsage counts what the monthly limit of a creator counts (ADR-0124 section 1): the
-- requests to create an emoji that the user submitted in [@since, @until), approved and pending
-- apart. Rejected, withdrawn, and lapsed requests do not count; deleting an approved emoji does
-- not change the count.
-- name: CountCustomEmojiUsage :one
SELECT CAST(COALESCE(SUM(CASE WHEN o.outcome = 'approved' THEN 1 ELSE 0 END), 0) AS INTEGER) AS approved,
  CAST(COALESCE(SUM(CASE WHEN o.request_id IS NULL THEN 1 ELSE 0 END), 0) AS INTEGER) AS pending
FROM custom_emoji_requests r
LEFT JOIN custom_emoji_request_outcomes o ON o.request_id = r.id
WHERE r.requester_user_id = @user_id AND r.kind = 'create'
  AND r.created_at >= @since AND r.created_at < @until
  AND (o.request_id IS NULL OR o.outcome = 'approved');

-- IsMediaOfPendingCustomEmojiRequest reports whether a pending request uses @media_id as its
-- image, which keeps the upload from the cleanup of the unattached uploads.
-- name: IsMediaOfPendingCustomEmojiRequest :one
SELECT CAST(EXISTS (SELECT 1 FROM custom_emoji_requests r
  JOIN custom_emoji_pending_requests p ON p.request_id = r.id
  WHERE r.media_id = @media_id) AS INTEGER) AS pending;

-- name: GetCustomEmojiRequest :one
SELECT r.id, r.kind, r.requester_user_id, r.emoji_id, r.shortcode, r.media_id, r.created_at,
  CAST(COALESCE(o.id, '') AS TEXT) AS outcome_id, CAST(COALESCE(o.outcome, '') AS TEXT) AS outcome,
  CAST(COALESCE(o.decided_by_user_id, '') AS TEXT) AS decided_by_user_id, CAST(COALESCE(o.reason, '') AS TEXT) AS reason,
  CAST(COALESCE(o.emoji_id, '') AS TEXT) AS outcome_emoji_id, CAST(COALESCE(o.decided_at, 0) AS INTEGER) AS decided_at
FROM custom_emoji_requests r
LEFT JOIN custom_emoji_request_outcomes o ON o.request_id = r.id
WHERE r.id = @id;

-- ListCustomEmojiRequests returns a page of the requests, newest first: an empty @requester_user_id
-- selects every requester, and an empty @state every state, 'pending' those without an outcome,
-- and an outcome those that ended so.
-- name: ListCustomEmojiRequests :many
SELECT r.id, r.kind, r.requester_user_id, r.emoji_id, r.shortcode, r.media_id, r.created_at,
  CAST(COALESCE(o.id, '') AS TEXT) AS outcome_id, CAST(COALESCE(o.outcome, '') AS TEXT) AS outcome,
  CAST(COALESCE(o.decided_by_user_id, '') AS TEXT) AS decided_by_user_id, CAST(COALESCE(o.reason, '') AS TEXT) AS reason,
  CAST(COALESCE(o.emoji_id, '') AS TEXT) AS outcome_emoji_id, CAST(COALESCE(o.decided_at, 0) AS INTEGER) AS decided_at
FROM custom_emoji_requests r
LEFT JOIN custom_emoji_request_outcomes o ON o.request_id = r.id
WHERE (CAST(@requester_user_id AS TEXT) = '' OR r.requester_user_id = CAST(@requester_user_id AS TEXT))
  AND (CAST(@state AS TEXT) = '' OR (CAST(@state AS TEXT) = 'pending' AND o.id IS NULL) OR o.outcome = CAST(@state AS TEXT))
  AND (CAST(@before_id AS TEXT) = '' OR r.id < CAST(@before_id AS TEXT))
ORDER BY r.id DESC
LIMIT @row_limit;

-- ListCustomEmojiRequestsByID returns the requests among @ids (a JSON array), in ID order.
-- name: ListCustomEmojiRequestsByID :many
SELECT r.id, r.kind, r.requester_user_id, r.emoji_id, r.shortcode, r.media_id, r.created_at,
  CAST(COALESCE(o.id, '') AS TEXT) AS outcome_id, CAST(COALESCE(o.outcome, '') AS TEXT) AS outcome,
  CAST(COALESCE(o.decided_by_user_id, '') AS TEXT) AS decided_by_user_id, CAST(COALESCE(o.reason, '') AS TEXT) AS reason,
  CAST(COALESCE(o.emoji_id, '') AS TEXT) AS outcome_emoji_id, CAST(COALESCE(o.decided_at, 0) AS INTEGER) AS decided_at
FROM custom_emoji_requests r
LEFT JOIN custom_emoji_request_outcomes o ON o.request_id = r.id
WHERE r.id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))
ORDER BY r.id;

-- ListPendingCustomEmojiRequestsOfUser returns the pending requests of a user, for its erasure.
-- name: ListPendingCustomEmojiRequestsOfUser :many
SELECT request_id FROM custom_emoji_pending_requests
WHERE requester_user_id = sqlc.arg(user_id) ORDER BY request_id;

-- BlankCustomEmojiRequester takes an erased user out of its requests, which stay for the audit log.
-- name: BlankCustomEmojiRequester :exec
UPDATE custom_emoji_requests SET requester_user_id = NULL WHERE requester_user_id = sqlc.arg(user_id);

-- name: DeleteAllCustomEmojiPendingRequests :exec
DELETE FROM custom_emoji_pending_requests;

-- RebuildCustomEmojiPendingRequests states the rule of the writes: a request without an outcome is
-- pending, a request to create reserves its shortcode, and a request for an image names its emoji.
-- A request whose requester was erased has an outcome by then.
-- name: RebuildCustomEmojiPendingRequests :exec
INSERT INTO custom_emoji_pending_requests (request_id, requester_user_id, reserved_shortcode, emoji_id)
SELECT r.id, r.requester_user_id, CASE WHEN r.kind = 'create' THEN r.shortcode END, r.emoji_id
FROM custom_emoji_requests r
WHERE r.requester_user_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM custom_emoji_request_outcomes o WHERE o.request_id = r.id);
