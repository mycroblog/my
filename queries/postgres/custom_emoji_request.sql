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
DELETE FROM custom_emoji_pending_requests WHERE request_id = sqlc.arg(request_id)::uuid;

-- name: InsertCustomEmojiRequestOutcome :exec
INSERT INTO custom_emoji_request_outcomes (id, request_id, outcome, decided_by_user_id, reason, emoji_id, decided_at)
VALUES (@id, @request_id, @outcome, @decided_by_user_id, @reason, @emoji_id, @decided_at);

-- LocalCustomEmojiShortcodeTaken reports whether a local emoji has @shortcode.
-- name: LocalCustomEmojiShortcodeTaken :one
SELECT EXISTS (SELECT 1 FROM custom_emojis WHERE shortcode = @shortcode AND domain = '') AS taken;

-- CustomEmojiShortcodeReserved reports whether a pending request to create reserves @shortcode.
-- name: CustomEmojiShortcodeReserved :one
SELECT EXISTS (SELECT 1 FROM custom_emoji_pending_requests WHERE reserved_shortcode = @shortcode::text) AS reserved;

-- name: GetPendingImageRequestOfEmoji :one
SELECT request_id FROM custom_emoji_pending_requests WHERE emoji_id = sqlc.arg(emoji_id)::uuid;

-- CountCustomEmojiUsage counts what the monthly limit of a creator counts (ADR-0124 section 1): the
-- requests to create an emoji that the user submitted in [@since, @until), approved and pending
-- apart. Rejected, withdrawn, and lapsed requests do not count; deleting an approved emoji does
-- not change the count.
-- name: CountCustomEmojiUsage :one
SELECT COUNT(*) FILTER (WHERE o.outcome = 'approved')::bigint AS approved,
  COUNT(*) FILTER (WHERE o.request_id IS NULL)::bigint AS pending
FROM custom_emoji_requests r
LEFT JOIN custom_emoji_request_outcomes o ON o.request_id = r.id
WHERE r.requester_user_id = sqlc.arg(user_id)::uuid AND r.kind = 'create'
  AND r.created_at >= sqlc.arg(since)::timestamptz AND r.created_at < sqlc.arg(until)::timestamptz
  AND (o.request_id IS NULL OR o.outcome = 'approved');

-- IsMediaOfPendingCustomEmojiRequest reports whether a pending request uses @media_id as its
-- image, which keeps the upload from the cleanup of the unattached uploads.
-- name: IsMediaOfPendingCustomEmojiRequest :one
SELECT EXISTS (SELECT 1 FROM custom_emoji_requests r
  JOIN custom_emoji_pending_requests p ON p.request_id = r.id
  WHERE r.media_id = sqlc.arg(media_id)::uuid) AS pending;

-- name: GetCustomEmojiRequest :one
SELECT r.id, r.kind, r.requester_user_id, r.emoji_id, r.shortcode, r.media_id, r.created_at,
  COALESCE(o.id::text, '')::text AS outcome_id, COALESCE(o.outcome, '')::text AS outcome,
  COALESCE(o.decided_by_user_id::text, '')::text AS decided_by_user_id, COALESCE(o.reason, '')::text AS reason,
  COALESCE(o.emoji_id::text, '')::text AS outcome_emoji_id, COALESCE(o.decided_at, r.created_at)::timestamptz AS decided_at
FROM custom_emoji_requests r
LEFT JOIN custom_emoji_request_outcomes o ON o.request_id = r.id
WHERE r.id = sqlc.arg(id)::uuid;

-- ListCustomEmojiRequests returns a page of the requests, newest first: an empty @requester_user_id
-- selects every requester, and an empty @state every state, 'pending' those without an outcome,
-- and an outcome those that ended so.
-- name: ListCustomEmojiRequests :many
SELECT r.id, r.kind, r.requester_user_id, r.emoji_id, r.shortcode, r.media_id, r.created_at,
  COALESCE(o.id::text, '')::text AS outcome_id, COALESCE(o.outcome, '')::text AS outcome,
  COALESCE(o.decided_by_user_id::text, '')::text AS decided_by_user_id, COALESCE(o.reason, '')::text AS reason,
  COALESCE(o.emoji_id::text, '')::text AS outcome_emoji_id, COALESCE(o.decided_at, r.created_at)::timestamptz AS decided_at
FROM custom_emoji_requests r
LEFT JOIN custom_emoji_request_outcomes o ON o.request_id = r.id
WHERE (sqlc.narg(requester_user_id)::uuid IS NULL OR r.requester_user_id = sqlc.narg(requester_user_id)::uuid)
  AND (@state::text = '' OR (@state::text = 'pending' AND o.id IS NULL) OR o.outcome = @state::text)
  AND (sqlc.narg(before_id)::uuid IS NULL OR r.id < sqlc.narg(before_id)::uuid)
ORDER BY r.id DESC
LIMIT @row_limit;

-- ListCustomEmojiRequestsByID returns the requests among @ids, in ID order.
-- name: ListCustomEmojiRequestsByID :many
SELECT r.id, r.kind, r.requester_user_id, r.emoji_id, r.shortcode, r.media_id, r.created_at,
  COALESCE(o.id::text, '')::text AS outcome_id, COALESCE(o.outcome, '')::text AS outcome,
  COALESCE(o.decided_by_user_id::text, '')::text AS decided_by_user_id, COALESCE(o.reason, '')::text AS reason,
  COALESCE(o.emoji_id::text, '')::text AS outcome_emoji_id, COALESCE(o.decided_at, r.created_at)::timestamptz AS decided_at
FROM custom_emoji_requests r
LEFT JOIN custom_emoji_request_outcomes o ON o.request_id = r.id
WHERE r.id = ANY(sqlc.arg(ids)::uuid[])
ORDER BY r.id;

-- ListPendingCustomEmojiRequestsOfUser returns the pending requests of a user, for its erasure.
-- name: ListPendingCustomEmojiRequestsOfUser :many
SELECT request_id FROM custom_emoji_pending_requests
WHERE requester_user_id = sqlc.arg(user_id)::uuid ORDER BY request_id;

-- BlankCustomEmojiRequester takes an erased user out of its requests, which stay for the audit log.
-- name: BlankCustomEmojiRequester :exec
UPDATE custom_emoji_requests SET requester_user_id = NULL WHERE requester_user_id = sqlc.arg(user_id)::uuid;

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
