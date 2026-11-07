-- SPDX-License-Identifier: AGPL-3.0-only

-- The sources and the emptying of the projections of moderation, identity, content (the
-- sensitivity overrides and the poll tallies), and timeline (the restrictions) that
-- `my projection rebuild` derives again (data-model section 9). The Rebuild queries state the
-- rules of the commands and projectors in SQL.

-- name: DeleteAllStatusSensitivityOverrides :exec
DELETE FROM status_sensitivity_overrides;

-- DeleteStatusSensitivityOverridesWithoutStatus drops what the rebuild recorded for a status with
-- no row: the purge of a status removes its row here, and its decisions and actions can outlast it.
-- name: DeleteStatusSensitivityOverridesWithoutStatus :exec
DELETE FROM status_sensitivity_overrides
WHERE NOT EXISTS (SELECT 1 FROM statuses s WHERE s.id = status_sensitivity_overrides.status_id);

-- name: DeleteAllPollTallies :exec
DELETE FROM poll_tallies;

-- RebuildPollTallies states the projector's rule: the counts of the newest observation of each
-- poll.
-- name: RebuildPollTallies :exec
INSERT INTO poll_tallies (poll_id, position, votes, voters_count)
SELECT o.poll_id, c.position, c.votes, o.voters_count
FROM poll_tally_observations o JOIN poll_tally_observation_counts c ON c.observation_id = o.id
WHERE NOT EXISTS (SELECT 1 FROM poll_tally_observations n WHERE n.poll_id = o.poll_id AND n.id > o.id);
