-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for SQLite (ADR-0023): the tables of the federation module
-- (docs/architecture/data-model/04-tables/08-federation.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- federation: inbox, delivery, and remote instances (data-model §4.8,
-- docs/architecture/federation.md). Owned by internal/app/federation.
-- ---------------------------------------------------------------------------------------------

-- Event: an activity accepted at an inbox, once per activity_uri (synthesized from the body's
-- digest when the activity has no id). trust is refetch when the signer is not the actor.
CREATE TABLE received_activities (
  activity_uri TEXT NOT NULL PRIMARY KEY,
  actor_uri TEXT NOT NULL,
  type TEXT NOT NULL,
  signature_scheme TEXT NOT NULL CHECK (signature_scheme IN ('rfc9421', 'cavage')),
  trust TEXT NOT NULL CHECK (trust IN ('signed', 'refetch')),
  received_at INTEGER NOT NULL
) STRICT;
CREATE INDEX received_activities_received_at ON received_activities (received_at);

-- Event: the received document with the bytes it arrived with, read by the asynchronous
-- processing and kept briefly for investigation and fixtures.
CREATE TABLE received_activity_documents (
  activity_uri TEXT NOT NULL PRIMARY KEY,
  document BLOB NOT NULL
) STRICT;

-- Event: a Delete of an unknown object or an Undo of an unknown target. A Create, Update, or
-- Follow of the uri that arrives later is dropped.
CREATE TABLE remote_object_tombstones (
  uri TEXT NOT NULL PRIMARY KEY,
  actor_uri TEXT NOT NULL,
  activity_uri TEXT NOT NULL,
  received_at INTEGER NOT NULL
) STRICT;
CREATE INDEX remote_object_tombstones_received_at ON remote_object_tombstones (received_at);

-- Event: an activity a local actor sends about one of its objects.
CREATE TABLE outbound_activities (
  id TEXT NOT NULL PRIMARY KEY,
  activity_uri TEXT NOT NULL,
  type TEXT NOT NULL,
  actor_account_id TEXT NOT NULL,
  object_type TEXT NOT NULL CHECK (object_type IN ('status', 'boost', 'favourite', 'follow', 'block', 'account', 'report', 'poll_vote', 'relay', 'quote', 'refusal', 'move')),
  object_id TEXT NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX outbound_activities_activity_uri ON outbound_activities (activity_uri);
CREATE INDEX outbound_activities_object_type_object_id_id ON outbound_activities (object_type, object_id, id);

-- Event: the body of an outbound activity, built once by the fan-out so that every delivery
-- and retry sends the same bytes.
CREATE TABLE outbound_activity_documents (
  outbound_activity_id TEXT NOT NULL PRIMARY KEY,
  document BLOB NOT NULL,
  built_at INTEGER NOT NULL
) STRICT;

-- Event: an inbox (shared inboxes merged) an outbound activity is delivered to. It is also the
-- record of where a status went, for its later Update and Delete.
CREATE TABLE deliveries (
  id TEXT NOT NULL PRIMARY KEY,
  outbound_activity_id TEXT NOT NULL,
  inbox_url TEXT NOT NULL,
  domain TEXT NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX deliveries_outbound_activity_id_inbox_url ON deliveries (outbound_activity_id, inbox_url);

-- Event: one attempt of a delivery. signature_scheme is NULL when no response came back.
CREATE TABLE delivery_attempts (
  id TEXT NOT NULL PRIMARY KEY,
  delivery_id TEXT NOT NULL,
  outcome TEXT NOT NULL CHECK (outcome IN ('succeeded', 'retryable_failure', 'permanent_failure', 'skipped')),
  signature_scheme TEXT CHECK (signature_scheme IN ('rfc9421', 'cavage')),
  http_status INTEGER,
  error TEXT,
  attempted_at INTEGER NOT NULL
) STRICT;
CREATE INDEX delivery_attempts_delivery_id_id ON delivery_attempts (delivery_id, id);

-- Resource: a server this instance has seen.
CREATE TABLE remote_instances (
  domain TEXT NOT NULL PRIMARY KEY,
  first_seen_at INTEGER NOT NULL
) STRICT;

-- Event: the software a server reported in its NodeInfo.
CREATE TABLE instance_nodeinfo_observations (
  id TEXT NOT NULL PRIMARY KEY,
  domain TEXT NOT NULL,
  software_name TEXT NOT NULL,
  software_version TEXT NOT NULL,
  observed_at INTEGER NOT NULL
) STRICT;
CREATE INDEX instance_nodeinfo_observations_domain_id ON instance_nodeinfo_observations (domain, id DESC);

-- Event: a server was marked unavailable or available as a delivery target, with the reason.
CREATE TABLE instance_availability_events (
  id TEXT NOT NULL PRIMARY KEY,
  domain TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('marked_unavailable', 'marked_available')),
  reason TEXT NOT NULL CHECK (reason IN (
    'failure_days', 'delivery_succeeded', 'inbound_request', 'probe_succeeded', 'operator'
  )),
  occurred_at INTEGER NOT NULL
) STRICT;
CREATE INDEX instance_availability_events_domain_id ON instance_availability_events (domain, id DESC);

-- Current value: whether a server is a delivery target, from delivery_attempts and
-- instance_availability_events. failure_days counts the distinct UTC days of the latest run of
-- failures; last_failure_day is YYYYMMDD, or '' without a failure.
CREATE TABLE instance_health (
  domain TEXT NOT NULL PRIMARY KEY,
  state TEXT NOT NULL CHECK (state IN ('available', 'unavailable')),
  since INTEGER NOT NULL,
  failure_days INTEGER NOT NULL CHECK (failure_days >= 0),
  last_failure_day TEXT NOT NULL CHECK (length(last_failure_day) IN (0, 8))
) STRICT;
CREATE INDEX instance_health_state ON instance_health (state);

-- Resource: a relay the operator registered (ADR-0061). url is what was entered (the inbox of a
-- mastodon relay, the actor of a litepub one); inbox_url is where posts are delivered.
-- created_by_user_id is NULL for the CLI. Deleted with its events and state only when it is
-- left or rejected.
CREATE TABLE relays (
  id TEXT NOT NULL PRIMARY KEY,
  protocol TEXT NOT NULL CHECK (protocol IN ('mastodon', 'litepub')),
  url TEXT NOT NULL CHECK (length(url) BETWEEN 1 AND 2048),
  inbox_url TEXT NOT NULL CHECK (length(inbox_url) BETWEEN 1 AND 2048),
  receive INTEGER NOT NULL CHECK (receive IN (0, 1)),
  created_by_user_id TEXT,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX relays_url ON relays (url);

-- Event: a change of a relay's membership. follow_activity_uri is the Follow that follow_sent
-- sent and that the others are about; relay_actor_uri is the actor of an Accept, NULL otherwise;
-- actor_user_id is NULL for the CLI and for what the relay sent. One event of each kind per
-- Follow, so that an answer received again records nothing.
CREATE TABLE relay_events (
  id TEXT NOT NULL PRIMARY KEY,
  relay_id TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('follow_sent', 'accepted', 'rejected', 'left')),
  follow_activity_uri TEXT NOT NULL,
  relay_actor_uri TEXT,
  actor_user_id TEXT,
  occurred_at INTEGER NOT NULL,
  CHECK ((kind = 'accepted') = (relay_actor_uri IS NOT NULL))
) STRICT;
CREATE INDEX relay_events_relay_id_id ON relay_events (relay_id, id DESC);
CREATE UNIQUE INDEX relay_events_relay_id_follow_activity_uri_kind ON relay_events (relay_id, follow_activity_uri, kind);

-- Projection of relay_events: where a relay's membership stands, from its newest event.
-- relay_actor_uri is the actor that accepted it, NULL unless accepted.
CREATE TABLE relay_states (
  relay_id TEXT NOT NULL PRIMARY KEY,
  state TEXT NOT NULL CHECK (state IN ('pending', 'accepted', 'rejected', 'left')),
  follow_activity_uri TEXT NOT NULL,
  relay_actor_uri TEXT,
  since INTEGER NOT NULL,
  CHECK ((state = 'accepted') = (relay_actor_uri IS NOT NULL))
) STRICT;
CREATE INDEX relay_states_follow_activity_uri ON relay_states (follow_activity_uri);
CREATE INDEX relay_states_relay_actor_uri ON relay_states (relay_actor_uri);

-- graph and federation: re-sent Follows and early Undo Follows (data-model §4.5, §4.8) ----------

-- Event: an Undo of an unknown Follow, by the pair of its actor and the object that the embedded
-- Follow names. The first Follow of the pair that arrives later is dropped, and its URI recorded
-- in remote_object_tombstones with this activity_uri.
CREATE TABLE remote_follow_tombstones (
  activity_uri TEXT NOT NULL PRIMARY KEY,
  actor_uri TEXT NOT NULL,
  object_uri TEXT NOT NULL,
  received_at INTEGER NOT NULL
) STRICT;
CREATE INDEX remote_follow_tombstones_actor_uri_object_uri ON remote_follow_tombstones (actor_uri, object_uri, received_at);
CREATE INDEX remote_follow_tombstones_received_at ON remote_follow_tombstones (received_at);
CREATE INDEX remote_object_tombstones_activity_uri ON remote_object_tombstones (activity_uri);
