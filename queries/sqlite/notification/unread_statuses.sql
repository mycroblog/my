-- SPDX-License-Identifier: AGPL-3.0-only

-- The statuses of the unread notifications (data-model 4.7, ADR-0157 section 7): the unread count
-- leaves out the notifications whose status a keyword filter of the recipient hides, so it judges
-- their statuses.

-- ListUnreadNotificationStatusIDs returns the status of each of the newest lim notifications of a
-- recipient after the ID after that CountNotificationsAfter counts and that have a status, once
-- per notification, newest first.
-- name: ListUnreadNotificationStatusIDs :many
SELECT CAST(n.status_id AS TEXT) AS status_id FROM notifications n
WHERE n.recipient_account_id = sqlc.arg(recipient_account_id)
  AND n.id > CAST(sqlc.arg(after) AS TEXT)
  AND n.status_id IS NOT NULL
  AND instr(CAST(sqlc.arg(hidden_types) AS TEXT), ',' || n.type || ',') = 0
  AND NOT EXISTS (SELECT 1 FROM notification_dismissals d WHERE d.notification_id = n.id)
  AND NOT EXISTS (SELECT 1 FROM notification_exclusions x
    WHERE x.viewer_account_id = n.recipient_account_id AND x.target_account_id = n.source_account_id
      AND (x.expires_at IS NULL OR x.expires_at > CAST(sqlc.arg(now) AS INTEGER)))
  AND NOT EXISTS (SELECT 1 FROM notification_domain_exclusions dx
    WHERE dx.viewer_account_id = n.recipient_account_id
      AND (dx.domain = n.source_domain OR substr(n.source_domain, -(length(dx.domain) + 1)) = '.' || dx.domain))
  AND NOT EXISTS (SELECT 1 FROM notification_deleted_statuses s WHERE s.status_id = n.status_id)
  AND (n.announcement_id IS NULL OR instr(CAST(sqlc.arg(shown_announcements) AS TEXT), ',' || n.announcement_id || ',') > 0)
ORDER BY n.id DESC
LIMIT sqlc.arg(lim);
