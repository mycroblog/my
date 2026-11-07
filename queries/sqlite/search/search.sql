-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Queries of the search module (ADR-0149 section 10, data-model section 4.18), owned by
-- internal/app/search.

-- StartSearchRebuild writes the row of a new rebuild, unless a rebuild is unfinished.
-- name: StartSearchRebuild :execrows
INSERT INTO search_rebuild (
  id, rebuild_id, mode, phase, after_status_id, rate, processed, failed, last_error, started_at, updated_at
) VALUES (
  1, @rebuild_id, @mode, @phase, '', @rate, 0, 0, '', @started_at, @started_at
)
ON CONFLICT (id) DO UPDATE SET
  rebuild_id = excluded.rebuild_id, mode = excluded.mode, phase = excluded.phase,
  after_status_id = excluded.after_status_id, rate = excluded.rate, processed = excluded.processed,
  failed = excluded.failed, last_error = excluded.last_error, started_at = excluded.started_at,
  updated_at = excluded.updated_at
WHERE search_rebuild.phase = 'finished';

-- name: DeleteOtherSearchRebuildFailures :exec
DELETE FROM search_rebuild_failures WHERE rebuild_id <> @rebuild_id;

-- name: GetSearchRebuild :one
SELECT rebuild_id, mode, phase, after_status_id, rate, processed, failed, last_error, started_at, updated_at
FROM search_rebuild WHERE id = 1;

-- StepSearchRebuild moves the rebuild on if it is still where the caller read it, which leaves a
-- job run twice with nothing to do the second time.
-- name: StepSearchRebuild :execrows
UPDATE search_rebuild SET
  phase = @phase, after_status_id = @after_status_id, processed = processed + @processed,
  failed = failed + @failed, last_error = @last_error, updated_at = @updated_at
WHERE id = 1 AND rebuild_id = @rebuild_id AND phase = @from_phase AND after_status_id = @from_after;

-- name: SetSearchRebuildRate :exec
UPDATE search_rebuild SET rate = @rate WHERE id = 1 AND rebuild_id = @rebuild_id;

-- name: SetSearchRebuildError :exec
UPDATE search_rebuild SET last_error = @last_error, updated_at = @updated_at
WHERE id = 1 AND rebuild_id = @rebuild_id;

-- name: AddSearchRebuildFailure :exec
INSERT INTO search_rebuild_failures (rebuild_id, status_id, message, failed_at)
VALUES (@rebuild_id, @status_id, @message, @failed_at)
ON CONFLICT (rebuild_id, status_id) DO UPDATE SET message = excluded.message, failed_at = excluded.failed_at;

-- name: ListSearchRebuildFailures :many
SELECT status_id, message, failed_at FROM search_rebuild_failures
WHERE rebuild_id = @rebuild_id ORDER BY status_id LIMIT @page_limit;

-- name: DeleteSearchRebuild :exec
DELETE FROM search_rebuild;

-- name: DeleteSearchRebuildFailures :exec
DELETE FROM search_rebuild_failures;
