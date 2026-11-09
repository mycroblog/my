-- SPDX-License-Identifier: AGPL-3.0-only

-- The error group notices of many hours at once, for the pages of notifications. Each reads what
-- GetErrorGroupNotices reads of one hour; an hour without notices has no row.

-- name: ListErrorGroupNoticesOf :many
SELECT hour_start, notices FROM error_group_notice_hours WHERE hour_start = ANY(sqlc.arg(hours)::timestamptz[]) ORDER BY hour_start;
