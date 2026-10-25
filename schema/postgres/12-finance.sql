-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for PostgreSQL (ADR-0023): the tables of the finance module
-- (docs/architecture/data-model/04-tables/12-finance.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- finance: the instance's costs (data-model §4.12, FR-901, FR-902, ADR-0067). Owned by
-- internal/app/finance. Dates are calendar dates (the logical type date).
-- ---------------------------------------------------------------------------------------------

-- Resource: a cost item, paid once on starts_on or every interval weeks, months, or years from
-- it. The schedule and the start never change. created_by_user_id is NULL for the CLI.
CREATE TABLE cost_items (
  id uuid NOT NULL PRIMARY KEY,
  schedule text NOT NULL CHECK (schedule IN ('once', 'weeks', 'months', 'years')),
  interval integer NOT NULL,
  starts_on date NOT NULL,
  created_by_user_id uuid,
  created_at timestamptz NOT NULL,
  CHECK ((schedule = 'once' AND interval = 1) OR (schedule <> 'once' AND interval BETWEEN 1 AND 99))
);

-- Event (revision): the name, provider, category, and memo of a cost item; the newest row is
-- current (cost_item_heads). provider_normalized groups the breakdown (NFKC, lower case). memo is
-- '' for none.
CREATE TABLE cost_item_label_revisions (
  id uuid NOT NULL PRIMARY KEY,
  cost_item_id uuid NOT NULL,
  name text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
  provider text NOT NULL CHECK (length(provider) BETWEEN 1 AND 100),
  provider_normalized text NOT NULL,
  category text NOT NULL CHECK (category IN (
    'compute', 'database', 'storage', 'network', 'domain', 'email', 'software', 'other'
  )),
  memo text NOT NULL,
  revised_by_user_id uuid,
  revised_at timestamptz NOT NULL
);
CREATE INDEX cost_item_label_revisions_cost_item_id_id ON cost_item_label_revisions (cost_item_id, id DESC);

-- Event (revision): the price of the payments of a cost item on or after effective_on, in the
-- minor unit of an ISO 4217 currency. Of the rows of the same effective_on, the last recorded
-- (the largest id) applies. The first row starts on the item's starts_on.
CREATE TABLE cost_item_price_revisions (
  id uuid NOT NULL PRIMARY KEY,
  cost_item_id uuid NOT NULL,
  effective_on date NOT NULL,
  amount_minor bigint NOT NULL CHECK (amount_minor >= 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  revised_by_user_id uuid,
  revised_at timestamptz NOT NULL
);
CREATE INDEX cost_item_price_revisions_cost_item_id_id ON cost_item_price_revisions (cost_item_id, id);

-- Event: the end of a cost item changed: no payment after ends_on (included), or no end for
-- NULL. The newest row is current (cost_item_heads).
CREATE TABLE cost_item_end_changes (
  id uuid NOT NULL PRIMARY KEY,
  cost_item_id uuid NOT NULL,
  ends_on date,
  changed_by_user_id uuid,
  changed_at timestamptz NOT NULL
);
CREATE INDEX cost_item_end_changes_cost_item_id_id ON cost_item_end_changes (cost_item_id, id DESC);

-- Event: a cost item was canceled, which removes it from every period, past ones included.
-- There is no undo.
CREATE TABLE cost_item_cancellations (
  id uuid NOT NULL PRIMARY KEY,
  cost_item_id uuid NOT NULL,
  canceled_by_user_id uuid,
  canceled_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX cost_item_cancellations_cost_item_id ON cost_item_cancellations (cost_item_id);

-- Projection of the label revisions, end changes, and cancellations: the current label and end
-- of each cost item. A row means the item is not canceled; ends_on is NULL for no end.
CREATE TABLE cost_item_heads (
  cost_item_id uuid NOT NULL PRIMARY KEY,
  label_revision_id uuid NOT NULL,
  ends_on date
);

-- Event: one attempt of a billing import (ADR-0068 §5), succeeded or failed with error_kind
-- ('none' for a success). run_key is the idempotency key of its job, which every retry shares.
-- requests counts the billed API requests the attempt made: the caps of §4 sum them by UTC day
-- and UTC month of occurred_at.
CREATE TABLE billing_import_runs (
  id uuid NOT NULL PRIMARY KEY,
  source text NOT NULL CHECK (source IN ('aws', 'gcp')),
  run_key text COLLATE "C" NOT NULL,
  outcome text NOT NULL CHECK (outcome IN ('succeeded', 'failed')),
  error_kind text NOT NULL CHECK (error_kind IN (
    'none', 'permission', 'not_enabled', 'not_found', 'throttled', 'request_cap', 'bytes_cap',
    'invalid_response', 'unavailable'
  )),
  requests integer NOT NULL CHECK (requests >= 0),
  bytes_billed bigint NOT NULL CHECK (bytes_billed >= 0),
  occurred_at timestamptz NOT NULL,
  CHECK ((outcome = 'succeeded') = (error_kind = 'none'))
);
CREATE INDEX billing_import_runs_source_occurred_at ON billing_import_runs (source, occurred_at);
CREATE INDEX billing_import_runs_run_key ON billing_import_runs (run_key);

-- Event (revision): the amount of a provider's charges of one billing month (YYYY-MM in the
-- provider's time zone), service, and charge type, spread over [starts_on, ends_on], recorded
-- when the value or its kind changed. The newest row of a key is current (imported_cost_heads).
-- The attempts of one job write a key once (the unique run_key).
CREATE TABLE imported_cost_revisions (
  id uuid NOT NULL PRIMARY KEY,
  source text NOT NULL CHECK (source IN ('aws', 'gcp')),
  billing_month text COLLATE "C" NOT NULL CHECK (billing_month ~ '^[0-9]{4}-[0-1][0-9]$'),
  service text COLLATE "C" NOT NULL CHECK (length(service) BETWEEN 1 AND 200),
  charge_type text NOT NULL CHECK (charge_type IN (
    'usage', 'credit', 'refund', 'tax', 'support', 'fee', 'adjustment', 'other'
  )),
  category text NOT NULL CHECK (category IN (
    'compute', 'database', 'storage', 'network', 'domain', 'email', 'software', 'other'
  )),
  starts_on date NOT NULL,
  ends_on date NOT NULL,
  amount_minor bigint NOT NULL,
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  kind text NOT NULL CHECK (kind IN ('estimate', 'final', 'correction')),
  run_id uuid NOT NULL,
  run_key text COLLATE "C" NOT NULL,
  recorded_at timestamptz NOT NULL,
  CHECK (starts_on <= ends_on)
);
CREATE UNIQUE INDEX imported_cost_revisions_key_run_key ON imported_cost_revisions (source, billing_month, service, charge_type, run_key);

-- Projection of imported_cost_revisions: the newest revision of each key, updated in the
-- transaction that adds the revision.
CREATE TABLE imported_cost_heads (
  source text NOT NULL,
  billing_month text COLLATE "C" NOT NULL,
  service text COLLATE "C" NOT NULL,
  charge_type text NOT NULL,
  revision_id uuid NOT NULL,
  PRIMARY KEY (source, billing_month, service, charge_type)
);

-- Event: a money movement on the payment provider's account (ADR-0073 §2, §8): a payment, a
-- refund, a chargeback, its reversal, or a fee that belongs to no payment, recorded once per
-- (provider, provider_id) when its amounts are final, with its gross, fee, and net in the minor
-- unit of its currency. occurred_on is occurred_at as a date in the provider's reporting time
-- zone; covers_* is the billing period of a payment. parent_movement_id is the payment of a
-- refund, a chargeback, or a reversal. subscription_id refers to the subscription without a
-- foreign key (NULL for a fee); no row holds a user.
CREATE TABLE income_movements (
  id uuid NOT NULL PRIMARY KEY,
  provider text NOT NULL CHECK (provider IN ('stripe', 'payjp')),
  provider_id text COLLATE "C" NOT NULL CHECK (length(provider_id) BETWEEN 1 AND 255),
  kind text NOT NULL CHECK (kind IN ('payment', 'refund', 'chargeback', 'chargeback_reversal', 'fee')),
  parent_movement_id uuid,
  subscription_id uuid,
  occurred_at timestamptz NOT NULL,
  occurred_on date NOT NULL,
  covers_starts_on date,
  covers_ends_on date,
  gross_minor bigint NOT NULL,
  fee_minor bigint NOT NULL,
  net_minor bigint NOT NULL,
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  recorded_at timestamptz NOT NULL,
  CHECK (net_minor = gross_minor - fee_minor),
  CHECK ((covers_starts_on IS NULL) = (covers_ends_on IS NULL)),
  CHECK (covers_starts_on IS NULL OR (kind = 'payment' AND covers_starts_on <= covers_ends_on)),
  CHECK (parent_movement_id IS NULL OR kind IN ('refund', 'chargeback', 'chargeback_reversal'))
);
CREATE UNIQUE INDEX income_movements_provider_id ON income_movements (provider, provider_id);
CREATE INDEX income_movements_occurred_on ON income_movements (occurred_on);
CREATE INDEX income_movements_subscription_occurred_at ON income_movements (subscription_id, occurred_at);
