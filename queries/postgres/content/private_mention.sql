-- SPDX-License-Identifier: AGPL-3.0-only

-- PrivateMention reports whether a status is direct, and whether its conversation holds a direct
-- status of the recipient that mentions the source (the notification policy's
-- unsolicited_private_mention, ADR-0158 section 6). No row means an unknown status.
-- name: PrivateMention :one
SELECT (s.visibility = 'direct')::boolean AS direct, EXISTS (
  SELECT 1 FROM status_conversations sc
  JOIN status_conversations rc ON rc.conversation_id = sc.conversation_id
  JOIN statuses r ON r.id = rc.status_id
  JOIN status_mention_index m ON m.status_id = r.id
  WHERE sc.status_id = s.id AND r.author_account_id = sqlc.arg(recipient_id) AND r.visibility = 'direct'
    AND m.account_id = sqlc.arg(source_id)
)::boolean AS solicited
FROM statuses s WHERE s.id = sqlc.arg(status_id);
