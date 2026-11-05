-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Queries of the inbox, delivery, and remote instances (data-model section 4.8), owned by
-- internal/app/federation.

-- InsertReceivedActivity records an accepted activity once; zero rows means a duplicate.
-- name: InsertReceivedActivity :execrows
INSERT INTO received_activities (activity_uri, actor_uri, type, signature_scheme, trust, received_at)
VALUES (@activity_uri, @actor_uri, @type, @signature_scheme, @trust, @received_at)
ON CONFLICT (activity_uri) DO NOTHING;

-- name: GetReceivedActivity :one
SELECT activity_uri, actor_uri, type, signature_scheme, trust, received_at FROM received_activities
WHERE activity_uri = @activity_uri;

-- GetReceivedActivityWithDocument returns a received activity while its document is kept.
-- name: GetReceivedActivityWithDocument :one
SELECT a.activity_uri, a.actor_uri, a.type, a.signature_scheme, a.trust, a.received_at
FROM received_activities a
JOIN received_activity_documents d ON d.activity_uri = a.activity_uri
WHERE a.activity_uri = @activity_uri;

-- name: InsertReceivedActivityDocument :exec
INSERT INTO received_activity_documents (activity_uri, document) VALUES (@activity_uri, @document)
ON CONFLICT (activity_uri) DO NOTHING;

-- name: GetReceivedActivityDocument :one
SELECT document FROM received_activity_documents WHERE activity_uri = @activity_uri;

-- DeleteUnconfirmedReceivedActivity removes an activity received with trust refetch, which its
-- origin did not confirm; zero rows means there is none.
-- name: DeleteUnconfirmedReceivedActivity :execrows
DELETE FROM received_activities WHERE activity_uri = @activity_uri AND trust = 'refetch';

-- name: DeleteReceivedActivityDocument :exec
DELETE FROM received_activity_documents WHERE activity_uri = @activity_uri;

-- DeleteReceivedActivityDocumentsBefore removes at most @batch_limit documents of activities
-- received before @received_before.
-- name: DeleteReceivedActivityDocumentsBefore :execrows
DELETE FROM received_activity_documents
WHERE activity_uri IN (
  SELECT a.activity_uri FROM received_activities a
  JOIN received_activity_documents d ON d.activity_uri = a.activity_uri
  WHERE a.received_at < @received_before ORDER BY a.received_at LIMIT @batch_limit
);

-- DeleteReceivedActivitiesBefore removes at most @batch_limit activities received before
-- @received_before whose documents are gone, so that no document outlives its activity.
-- name: DeleteReceivedActivitiesBefore :execrows
DELETE FROM received_activities
WHERE activity_uri IN (
  SELECT a.activity_uri FROM received_activities a
  WHERE a.received_at < @received_before
    AND NOT EXISTS (SELECT 1 FROM received_activity_documents d WHERE d.activity_uri = a.activity_uri)
  ORDER BY a.received_at LIMIT @batch_limit
);

-- name: InsertRemoteObjectTombstone :execrows
INSERT INTO remote_object_tombstones (uri, actor_uri, activity_uri, received_at)
VALUES (@uri, @actor_uri, @activity_uri, @received_at)
ON CONFLICT (uri, actor_uri) DO NOTHING;

-- name: HasRemoteObjectTombstone :one
SELECT EXISTS (SELECT 1 FROM remote_object_tombstones WHERE uri = @uri AND actor_uri = @actor_uri);

-- name: DeleteRemoteObjectTombstonesBefore :execrows
DELETE FROM remote_object_tombstones
WHERE (uri, actor_uri) IN (
  SELECT t.uri, t.actor_uri FROM remote_object_tombstones t
  WHERE t.received_at < @received_before ORDER BY t.received_at LIMIT @batch_limit
);

-- name: InsertOutboundActivity :exec
INSERT INTO outbound_activities (id, activity_uri, type, actor_account_id, object_type, object_id, created_at)
VALUES (@id, @activity_uri, @type, @actor_account_id, @object_type, @object_id, @created_at);

-- name: GetOutboundActivity :one
SELECT id, activity_uri, type, actor_account_id, object_type, object_id, created_at
FROM outbound_activities WHERE id = @id;

-- ListOutboundActivitiesOfObject returns the activities sent about an object, oldest first.
-- name: ListOutboundActivitiesOfObject :many
SELECT id, activity_uri, type, actor_account_id, object_type, object_id, created_at
FROM outbound_activities WHERE object_type = @object_type AND object_id = @object_id ORDER BY id;

-- InsertOutboundActivityDocument stores the body once; a repeated fan-out keeps the first.
-- name: InsertOutboundActivityDocument :execrows
INSERT INTO outbound_activity_documents (outbound_activity_id, document, built_at)
VALUES (@outbound_activity_id, @document, @built_at)
ON CONFLICT (outbound_activity_id) DO NOTHING;

-- name: GetOutboundActivityDocument :one
SELECT document FROM outbound_activity_documents WHERE outbound_activity_id = @outbound_activity_id;

-- DeleteOutboundActivityDocumentsBefore removes at most @batch_limit documents built before
-- @built_before. The activity's ID is a UUIDv7 no later than its document, so @id_before (the
-- UUIDv7 floor of @built_before) bounds the scan of the primary key.
-- name: DeleteOutboundActivityDocumentsBefore :execrows
DELETE FROM outbound_activity_documents
WHERE outbound_activity_id IN (
  SELECT d.outbound_activity_id FROM outbound_activity_documents d
  WHERE d.outbound_activity_id < @id_before AND d.built_at < @built_before
  ORDER BY d.outbound_activity_id LIMIT @batch_limit
);

-- InsertDelivery adds an inbox to an activity once; zero rows means it was already there.
-- name: InsertDelivery :execrows
INSERT INTO deliveries (id, outbound_activity_id, inbox_url, domain, created_at)
VALUES (@id, @outbound_activity_id, @inbox_url, @domain, @created_at)
ON CONFLICT (outbound_activity_id, inbox_url) DO NOTHING;

-- name: GetDelivery :one
SELECT id, outbound_activity_id, inbox_url, domain, created_at FROM deliveries WHERE id = @id;

-- ListDeliveriesOfOutboundActivity returns the inboxes of an activity in id order after
-- @after_id, for the Update and Delete of what it created.
-- name: ListDeliveriesOfOutboundActivity :many
SELECT id, outbound_activity_id, inbox_url, domain, created_at FROM deliveries
WHERE outbound_activity_id = @outbound_activity_id AND id > @after_id ORDER BY id LIMIT @batch_limit;

-- name: InsertDeliveryAttempt :exec
INSERT INTO delivery_attempts (id, delivery_id, outcome, signature_scheme, status_code, error, attempted_at)
VALUES (@id, @delivery_id, @outcome, @signature_scheme, @status_code, @error, @attempted_at);

-- name: ListDeliveryAttempts :many
SELECT id, delivery_id, outcome, signature_scheme, status_code, error, attempted_at FROM delivery_attempts
WHERE delivery_id = @delivery_id ORDER BY id;

-- InsertOutboundFanOutCompletion records once that the fan-out of an activity enqueued its last
-- batch.
-- name: InsertOutboundFanOutCompletion :exec
INSERT INTO outbound_fanout_completions (outbound_activity_id, completed_at)
VALUES (@outbound_activity_id, @completed_at)
ON CONFLICT (outbound_activity_id) DO NOTHING;

-- InsertClosureDelete records the closure's Delete of an account once; zero rows means it was
-- already there.
-- name: InsertClosureDelete :execrows
INSERT INTO closure_deletes (account_id, outbound_activity_id, recorded_at)
VALUES (@account_id, @outbound_activity_id, @recorded_at)
ON CONFLICT (account_id) DO NOTHING;

-- name: ListClosureDeleteAccounts :many
SELECT account_id FROM closure_deletes WHERE account_id IN (sqlc.slice(account_ids)) ORDER BY account_id;

-- name: CountClosureDeletes :one
SELECT COUNT(*) FROM closure_deletes;

-- CountUnfinishedClosureFanOuts counts the closure's Deletes whose fan-out has not ended.
-- name: CountUnfinishedClosureFanOuts :one
SELECT COUNT(*) FROM closure_deletes c
WHERE NOT EXISTS (SELECT 1 FROM outbound_fanout_completions f WHERE f.outbound_activity_id = c.outbound_activity_id);

-- CountClosureDeliveries counts the deliveries of the closure's Deletes by their latest outcome
-- ('' before the first attempt) and whether they used up @max_attempts.
-- name: CountClosureDeliveries :many
SELECT
  CAST(COALESCE((SELECT a.outcome FROM delivery_attempts a WHERE a.delivery_id = d.id ORDER BY a.id DESC LIMIT 1), '') AS TEXT) AS outcome,
  CAST((SELECT COUNT(*) FROM delivery_attempts a WHERE a.delivery_id = d.id) >= CAST(@max_attempts AS INTEGER) AS INTEGER) AS exhausted,
  COUNT(*) AS deliveries
FROM closure_deletes c JOIN deliveries d ON d.outbound_activity_id = c.outbound_activity_id
GROUP BY 1, 2;

-- CountPendingMoveSends counts the Moves recorded since @since whose fan-out has not ended or
-- that have a delivery still to be attempted: none yet, or a retryable failure with attempts
-- left of @max_attempts.
-- name: CountPendingMoveSends :one
SELECT COUNT(*) FROM outbound_activities o
WHERE o.object_type = 'move' AND o.created_at >= @since AND (
  NOT EXISTS (SELECT 1 FROM outbound_fanout_completions f WHERE f.outbound_activity_id = o.id)
  OR EXISTS (
    SELECT 1 FROM deliveries d
    WHERE d.outbound_activity_id = o.id
      AND COALESCE((SELECT a.outcome FROM delivery_attempts a WHERE a.delivery_id = d.id ORDER BY a.id DESC LIMIT 1), 'retryable_failure') = 'retryable_failure'
      AND (SELECT COUNT(*) FROM delivery_attempts a WHERE a.delivery_id = d.id) < CAST(@max_attempts AS INTEGER)
  )
);

-- DeleteDeliveryAttemptsBefore removes at most @batch_limit attempts made before
-- @attempted_before; @id_before (the UUIDv7 floor of @attempted_before) bounds the scan of the
-- primary key.
-- name: DeleteDeliveryAttemptsBefore :execrows
DELETE FROM delivery_attempts
WHERE id IN (
  SELECT a.id FROM delivery_attempts a WHERE a.id < @id_before AND a.attempted_at < @attempted_before
  ORDER BY a.id LIMIT @batch_limit
);

-- name: InsertRemoteInstance :execrows
INSERT INTO remote_instances (domain, first_seen_at) VALUES (@domain, @first_seen_at)
ON CONFLICT (domain) DO NOTHING;

-- name: InsertInstanceNodeinfoObservation :exec
INSERT INTO instance_nodeinfo_observations (id, domain, software_name, software_version, observed_at)
VALUES (@id, @domain, @software_name, @software_version, @observed_at);

-- name: GetLatestInstanceNodeinfoObservation :one
SELECT id, domain, software_name, software_version, observed_at FROM instance_nodeinfo_observations
WHERE domain = @domain ORDER BY id DESC LIMIT 1;

-- name: InsertInstanceAvailabilityEvent :exec
INSERT INTO instance_availability_events (id, domain, kind, reason, occurred_at)
VALUES (@id, @domain, @kind, @reason, @occurred_at);

-- ListInstanceAvailabilityEvents returns the availability events of a server, newest first,
-- before the event @before ('' for the newest).
-- name: ListInstanceAvailabilityEvents :many
SELECT id, domain, kind, reason, occurred_at FROM instance_availability_events
WHERE domain = sqlc.arg(domain) AND (CAST(sqlc.arg(before) AS TEXT) = '' OR id < CAST(sqlc.arg(before) AS TEXT))
ORDER BY id DESC LIMIT sqlc.arg(batch_limit);

-- name: UpsertInstanceHealth :exec
INSERT INTO instance_health (domain, state, since, failure_days, last_failure_day)
VALUES (@domain, @state, @since, @failure_days, @last_failure_day)
ON CONFLICT (domain) DO UPDATE SET
  state = excluded.state,
  since = excluded.since,
  failure_days = excluded.failure_days,
  last_failure_day = excluded.last_failure_day;

-- name: GetInstanceHealth :one
SELECT domain, state, since, failure_days, last_failure_day FROM instance_health WHERE domain = @domain;

-- ListUnavailableInstances returns the servers that are not delivery targets, in domain order
-- after @after_domain.
-- name: ListUnavailableInstances :many
SELECT domain, state, since, failure_days, last_failure_day FROM instance_health
WHERE state = 'unavailable' AND domain > @after_domain ORDER BY domain LIMIT @batch_limit;

-- ListInstanceHealth returns the servers with a health row (those with failures or unavailable),
-- in domain order after @after_domain, of the state @state ('' for every state).
-- name: ListInstanceHealth :many
SELECT domain, state, since, failure_days, last_failure_day FROM instance_health
WHERE (CAST(sqlc.arg(state) AS TEXT) = '' OR state = CAST(sqlc.arg(state) AS TEXT)) AND domain > sqlc.arg(after_domain)
ORDER BY domain LIMIT sqlc.arg(batch_limit);

-- CountInstanceHealth counts by state the servers that failed on a day since they last answered
-- or that are unavailable, for the metrics snapshot.
-- name: CountInstanceHealth :many
SELECT state, count(*) AS servers FROM instance_health
WHERE state = 'unavailable' OR failure_days > 0 GROUP BY state;

-- LockInstanceHealth reads the health of a server in the transaction that updates it. SQLite
-- serializes writers, so it needs no row lock.
-- name: LockInstanceHealth :one
SELECT domain, state, since, failure_days, last_failure_day FROM instance_health WHERE domain = @domain;

-- InsertReceivedActivityAnswer inserts nothing when the activity has been answered.
-- name: InsertReceivedActivityAnswer :execrows
INSERT INTO received_activity_answers (activity_uri, answered_at)
VALUES (@activity_uri, @answered_at)
ON CONFLICT (activity_uri) DO NOTHING;

-- DeleteReceivedActivityAnswersBefore removes at most @batch_limit records of answers made before
-- @answered_before.
-- name: DeleteReceivedActivityAnswersBefore :execrows
DELETE FROM received_activity_answers
WHERE activity_uri IN (
  SELECT a.activity_uri FROM received_activity_answers a
  WHERE a.answered_at < @answered_before ORDER BY a.answered_at LIMIT @batch_limit
);

-- InsertRemoteFollowTombstone inserts nothing when the Undo is already recorded.
-- name: InsertRemoteFollowTombstone :execrows
INSERT INTO remote_follow_tombstones (activity_uri, actor_uri, object_uri, received_at)
VALUES (@activity_uri, @actor_uri, @object_uri, @received_at)
ON CONFLICT (activity_uri) DO NOTHING;

-- GetLiveRemoteFollowTombstone returns the oldest tombstone of the pair that has not dropped a
-- Follow yet: no remote_object_tombstones row was recorded with its activity_uri.
-- name: GetLiveRemoteFollowTombstone :one
SELECT t.activity_uri, t.actor_uri, t.object_uri, t.received_at
FROM remote_follow_tombstones t
WHERE t.actor_uri = @actor_uri AND t.object_uri = @object_uri
  AND NOT EXISTS (SELECT 1 FROM remote_object_tombstones o WHERE o.activity_uri = t.activity_uri)
ORDER BY t.received_at, t.activity_uri
LIMIT 1;

-- name: DeleteRemoteFollowTombstonesBefore :execrows
DELETE FROM remote_follow_tombstones
WHERE activity_uri IN (
  SELECT t.activity_uri FROM remote_follow_tombstones t
  WHERE t.received_at < @received_before ORDER BY t.received_at LIMIT @batch_limit
);

-- ListSupersededStatusCreates returns at most @batch_limit Creates of statuses whose Delete, or
-- the Delete of whose actor, was recorded before @created_before: the fan-out of that Delete read
-- the inboxes of the Create, and nothing reads them again. The closure's Delete of an actor stays
-- out of this, because it erases nothing.
-- name: ListSupersededStatusCreates :many
SELECT c.id FROM outbound_activities d
JOIN outbound_activities c ON c.object_type = 'status' AND c.type = 'Create' AND c.object_id = d.object_id
WHERE d.type = 'Delete' AND d.object_type = 'status' AND d.created_at < @created_before
UNION ALL
SELECT c.id FROM outbound_activities d
JOIN outbound_activities c ON c.object_type = 'status' AND c.type = 'Create' AND c.actor_account_id = d.object_id
WHERE d.type = 'Delete' AND d.object_type = 'account' AND d.created_at < @created_before
  AND NOT EXISTS (SELECT 1 FROM closure_deletes x WHERE x.outbound_activity_id = d.id)
LIMIT @batch_limit;

-- ListExpiredOutboundActivities returns at most @batch_limit activities recorded before
-- @created_before, the oldest first, except a status's Create; the closure's Deletes, which the
-- closure counts; and a Delete that a kept Create still depends on (ListSupersededStatusCreates).
-- name: ListExpiredOutboundActivities :many
SELECT o.id FROM outbound_activities o
WHERE NOT (o.object_type = 'status' AND o.type = 'Create')
  AND o.created_at < @created_before
  AND NOT EXISTS (SELECT 1 FROM closure_deletes x WHERE x.outbound_activity_id = o.id)
  AND NOT (o.type = 'Delete' AND o.object_type = 'status' AND EXISTS (
    SELECT 1 FROM outbound_activities c WHERE c.object_type = 'status' AND c.type = 'Create' AND c.object_id = o.object_id))
  AND NOT (o.type = 'Delete' AND o.object_type = 'account' AND EXISTS (
    SELECT 1 FROM outbound_activities c WHERE c.object_type = 'status' AND c.type = 'Create' AND c.actor_account_id = o.object_id))
ORDER BY o.created_at, o.id
LIMIT @batch_limit;

-- name: DeleteAttemptsOfOutboundActivities :exec
DELETE FROM delivery_attempts WHERE delivery_id IN (
  SELECT d.id FROM deliveries d WHERE d.outbound_activity_id IN (SELECT value FROM json_each(CAST(sqlc.arg(activity_ids) AS TEXT))));

-- name: DeleteDeliveriesOfOutboundActivities :exec
DELETE FROM deliveries WHERE outbound_activity_id IN (SELECT value FROM json_each(CAST(sqlc.arg(activity_ids) AS TEXT)));

-- name: DeleteDocumentsOfOutboundActivities :exec
DELETE FROM outbound_activity_documents WHERE outbound_activity_id IN (SELECT value FROM json_each(CAST(sqlc.arg(activity_ids) AS TEXT)));

-- name: DeleteCompletionsOfOutboundActivities :exec
DELETE FROM outbound_fanout_completions WHERE outbound_activity_id IN (SELECT value FROM json_each(CAST(sqlc.arg(activity_ids) AS TEXT)));

-- name: DeleteOutboundActivities :exec
DELETE FROM outbound_activities WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(activity_ids) AS TEXT)));

-- DeleteNodeInfoObservationsBefore removes at most @batch_limit observations made before
-- @observed_before, except the newest of each domain. @id_before (the UUIDv7 floor of
-- @observed_before) bounds the scan of the primary key.
-- name: DeleteNodeInfoObservationsBefore :execrows
DELETE FROM instance_nodeinfo_observations
WHERE id IN (
  SELECT o.id FROM instance_nodeinfo_observations o
  WHERE o.id < @id_before AND o.observed_at < @observed_before
    AND EXISTS (SELECT 1 FROM instance_nodeinfo_observations n WHERE n.domain = o.domain AND n.id > o.id)
  ORDER BY o.id LIMIT @batch_limit
);

-- DeleteAvailabilityEventsBefore removes at most @batch_limit events that occurred before
-- @occurred_before, except the newest of each domain. @id_before (the UUIDv7 floor of
-- @occurred_before) bounds the scan of the primary key.
-- name: DeleteAvailabilityEventsBefore :execrows
DELETE FROM instance_availability_events
WHERE id IN (
  SELECT e.id FROM instance_availability_events e
  WHERE e.id < @id_before AND e.occurred_at < @occurred_before
    AND EXISTS (SELECT 1 FROM instance_availability_events n WHERE n.domain = e.domain AND n.id > e.id)
  ORDER BY e.id LIMIT @batch_limit
);
