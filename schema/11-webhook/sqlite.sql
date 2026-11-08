-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for SQLite (ADR-0023): the tables of the webhook module
-- (docs/architecture/data-model/04-tables/11-webhook.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- webhook: the instance's webhooks (data-model §4.11, FR-722, ADR-0060). Owned by
-- internal/app/webhook.
-- ---------------------------------------------------------------------------------------------

-- Resource: an endpoint that an administrator set up. created_by_user_id is NULL for the CLI.
CREATE TABLE webhook_endpoints (
  id TEXT NOT NULL PRIMARY KEY,
  created_by_user_id TEXT,
  created_at INTEGER NOT NULL
) STRICT;

-- Event (revision): the URL and the description of an endpoint; the newest row is current
-- (webhook_endpoint_heads). description is NULL for none.
CREATE TABLE webhook_endpoint_revisions (
  id TEXT NOT NULL PRIMARY KEY,
  endpoint_id TEXT NOT NULL,
  url TEXT NOT NULL CHECK (length(url) BETWEEN 1 AND 2048),
  description TEXT CHECK (description IS NULL OR length(description) BETWEEN 1 AND 200),
  revised_by_user_id TEXT,
  revised_at INTEGER NOT NULL
) STRICT;
CREATE INDEX webhook_endpoint_revisions_endpoint_id_id ON webhook_endpoint_revisions (endpoint_id, id DESC);

-- Event (child of a revision): an event that the revision subscribes to.
CREATE TABLE webhook_endpoint_revision_events (
  revision_id TEXT NOT NULL,
  event_type TEXT NOT NULL CHECK (event_type IN (
    'report.created', 'report.resolved', 'report.reopened', 'registration.pending',
    'account.created', 'account.updated', 'moderation_action.created', 'moderation_action.lifted',
    'appeal.created', 'domain_moderation_action.created', 'domain_moderation_action.lifted',
    'status.created', 'custom_emoji.created', 'custom_emoji.updated', 'custom_emoji.deleted',
    'error_group.created', 'error_group.regressed'
  )),
  PRIMARY KEY (revision_id, event_type)
) STRICT;
CREATE INDEX webhook_endpoint_revision_events_event_type ON webhook_endpoint_revision_events (event_type);

-- Event: a secret of an endpoint, sealed with kernel.Sealer (ADR-0019 §6). A rotation adds a row;
-- the row before it still signs for 24 hours after the new row's created_at.
CREATE TABLE webhook_secrets (
  id TEXT NOT NULL PRIMARY KEY,
  endpoint_id TEXT NOT NULL,
  sealed_secret BLOB NOT NULL,
  created_by_user_id TEXT,
  created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX webhook_secrets_endpoint_id_id ON webhook_secrets (endpoint_id, id DESC);

-- Event: an endpoint was enabled, disabled, or deleted: by an administrator (admin, with
-- changed_by_user_id unless the CLI did it), after a 410 Gone (gone), or after failing for long
-- (failing).
CREATE TABLE webhook_endpoint_state_changes (
  id TEXT NOT NULL PRIMARY KEY,
  endpoint_id TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('enabled', 'disabled', 'deleted')),
  reason TEXT NOT NULL CHECK (reason IN ('admin', 'gone', 'failing')),
  changed_by_user_id TEXT,
  changed_at INTEGER NOT NULL,
  CHECK (changed_by_user_id IS NULL OR reason = 'admin')
) STRICT;
CREATE INDEX webhook_endpoint_state_changes_endpoint_id_id ON webhook_endpoint_state_changes (endpoint_id, id DESC);

-- Resource: a message, the body that every endpoint subscribing to its event receives as is (an
-- uninterpreted document, data-model rule 4). id is its webhook-id. Removed with its deliveries
-- 14 days after created_at (FR-803).
CREATE TABLE webhook_messages (
  id TEXT NOT NULL PRIMARY KEY,
  event_type TEXT NOT NULL CHECK (event_type IN (
    'report.created', 'report.resolved', 'report.reopened', 'registration.pending',
    'account.created', 'account.updated', 'moderation_action.created', 'moderation_action.lifted',
    'appeal.created', 'domain_moderation_action.created', 'domain_moderation_action.lifted',
    'status.created', 'custom_emoji.created', 'custom_emoji.updated', 'custom_emoji.deleted',
    'error_group.created', 'error_group.regressed',
    'webhook.test'
  )),
  body BLOB NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX webhook_messages_created_at ON webhook_messages (created_at);

-- Resource: a message to one endpoint, which a webhook.deliver job sends.
CREATE TABLE webhook_deliveries (
  id TEXT NOT NULL PRIMARY KEY,
  message_id TEXT NOT NULL,
  endpoint_id TEXT NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX webhook_deliveries_message_id_endpoint_id ON webhook_deliveries (message_id, endpoint_id);
CREATE INDEX webhook_deliveries_endpoint_id_id ON webhook_deliveries (endpoint_id, id DESC);

-- Event: one attempt to send a delivery. status_code is the response's, for succeeded and
-- http_error only.
CREATE TABLE webhook_delivery_attempts (
  id TEXT NOT NULL PRIMARY KEY,
  delivery_id TEXT NOT NULL,
  outcome TEXT NOT NULL CHECK (outcome IN (
    'succeeded', 'http_error', 'timeout', 'connection_error', 'forbidden_destination'
  )),
  status_code INTEGER CHECK (status_code IS NULL OR status_code BETWEEN 100 AND 599),
  duration_ms INTEGER NOT NULL CHECK (duration_ms >= 0),
  attempted_at INTEGER NOT NULL,
  CHECK ((outcome IN ('succeeded', 'http_error')) = (status_code IS NOT NULL))
) STRICT;
CREATE INDEX webhook_delivery_attempts_delivery_id_id ON webhook_delivery_attempts (delivery_id, id);

-- Projection of the revisions, secrets, and state changes: the current revision, state, and
-- secret of each endpoint.
CREATE TABLE webhook_endpoint_heads (
  endpoint_id TEXT NOT NULL PRIMARY KEY,
  revision_id TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('enabled', 'disabled', 'deleted')),
  secret_id TEXT NOT NULL
) STRICT;
CREATE INDEX webhook_endpoint_heads_state ON webhook_endpoint_heads (state);

-- Projection of the attempts: an endpoint's run of failed attempts since its last success. A row
-- means failures without a success since first_failed_at; a succeeded attempt removes it.
CREATE TABLE webhook_endpoint_failures (
  endpoint_id TEXT NOT NULL PRIMARY KEY,
  first_failure_attempt_id TEXT NOT NULL,
  first_failed_at INTEGER NOT NULL,
  consecutive_failures INTEGER NOT NULL CHECK (consecutive_failures > 0)
) STRICT;
