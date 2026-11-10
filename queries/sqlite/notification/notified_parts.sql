-- SPDX-License-Identifier: AGPL-3.0-only

-- The error group notices of many hours at once, for the pages of notifications. Each reads what
-- GetErrorGroupNotices reads of one hour; an hour without notices has no row. The lists are JSON arrays.

-- name: ListErrorGroupNoticesOf :many
SELECT hour_start, notices FROM error_group_notice_hours WHERE hour_start IN (SELECT value FROM json_each(CAST(sqlc.arg(hours) AS TEXT))) ORDER BY hour_start;
