-- SPDX-License-Identifier: AGPL-3.0-only

-- ListDataUpgradeRuns returns the recorded data upgrade steps, ordered by key.
-- name: ListDataUpgradeRuns :many
SELECT step_key, completed_at FROM data_upgrade_runs ORDER BY step_key;

-- InsertDataUpgradeRun records that a data upgrade step completed; a step recorded before keeps
-- its first record.
-- name: InsertDataUpgradeRun :exec
INSERT INTO data_upgrade_runs (step_key, completed_at) VALUES (@step_key, @completed_at)
ON CONFLICT (step_key) DO NOTHING;
