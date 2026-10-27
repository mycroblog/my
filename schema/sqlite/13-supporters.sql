-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for SQLite (ADR-0023): the tables of the supporters module
-- (docs/architecture/data-model/04-tables/13-supporters.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- supporters: the supporter program, its plans, and their subscriptions (data-model §4.13,
-- FR-910 to FR-912, ADR-0072 §15). Owned by internal/app/supporters.
-- ---------------------------------------------------------------------------------------------

-- Event: the supporter program enabled or disabled (ADR-0072 §2); the newest row is the current
-- state, and no row is disabled. provider is the payment provider that an enabled program uses,
-- '' for disabled. changed_by_user_id is NULL for the CLI.
CREATE TABLE supporter_program_changes (
  id TEXT NOT NULL PRIMARY KEY,
  state TEXT NOT NULL CHECK (state IN ('enabled', 'disabled')),
  provider TEXT NOT NULL CHECK (provider IN ('', 'stripe', 'payjp')),
  changed_by_user_id TEXT,
  changed_at INTEGER NOT NULL,
  CHECK ((state = 'enabled') = (provider <> ''))
) STRICT;

-- Resource: a plan (ADR-0072 §3). The role and the currency never change; one role is the role of
-- one plan. Plans are never deleted: retiring one keeps the rows that refer to it.
CREATE TABLE supporter_plans (
  id TEXT NOT NULL PRIMARY KEY,
  role_id TEXT NOT NULL,
  currency TEXT NOT NULL CHECK (length(currency) = 3 AND upper(currency) = currency),
  created_by_user_id TEXT,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX supporter_plans_role_id ON supporter_plans (role_id);

-- Event (revision): the terms of a plan; the newest row is current. The yearly price is
-- floor(monthly_amount_minor * 12 * (100 - annual_discount_percent) / 100). revised_by_user_id is
-- NULL for the CLI.
CREATE TABLE supporter_plan_revisions (
  id TEXT NOT NULL PRIMARY KEY,
  plan_id TEXT NOT NULL,
  name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 50),
  description TEXT NOT NULL CHECK (length(description) <= 1000),
  rank INTEGER NOT NULL CHECK (rank BETWEEN 0 AND 1000000),
  monthly_amount_minor INTEGER NOT NULL CHECK (monthly_amount_minor BETWEEN 1 AND 1000000000000),
  annual_discount_percent INTEGER NOT NULL CHECK (annual_discount_percent BETWEEN 0 AND 90),
  emoji_limit INTEGER NOT NULL CHECK (emoji_limit BETWEEN 0 AND 10000),
  visibility TEXT NOT NULL CHECK (visibility IN ('public', 'hidden')),
  revised_by_user_id TEXT,
  revised_at INTEGER NOT NULL
) STRICT;
CREATE INDEX supporter_plan_revisions_plan_id_id ON supporter_plan_revisions (plan_id, id DESC);

-- Event: a plan retired (ADR-0072 §9); it cannot be undone. retired_by_user_id is NULL for the CLI.
CREATE TABLE supporter_plan_retirements (
  plan_id TEXT NOT NULL PRIMARY KEY,
  retired_by_user_id TEXT,
  retired_at INTEGER NOT NULL
) STRICT;

-- Projection: the current revision of each plan that is not retired, with its rank, unique among
-- them, and its visibility.
CREATE TABLE supporter_plan_heads (
  plan_id TEXT NOT NULL PRIMARY KEY,
  revision_id TEXT NOT NULL,
  rank INTEGER NOT NULL,
  visibility TEXT NOT NULL
) STRICT;
CREATE UNIQUE INDEX supporter_plan_heads_rank ON supporter_plan_heads (rank);

-- Resource: a recurring price that the supporters.ensure_prices job made at the provider for a
-- plan, an interval, and an amount (ADR-0072 §3). A price's amount never changes: a new amount is
-- a new price. A plan may have any number of prices of the same interval and amount (an
-- amount the plan had before comes back as a new price); only the provider's ID is unique.
CREATE TABLE supporter_plan_prices (
  id TEXT NOT NULL PRIMARY KEY,
  plan_id TEXT NOT NULL,
  interval TEXT NOT NULL CHECK (interval IN ('month', 'year')),
  amount_minor INTEGER NOT NULL CHECK (amount_minor >= 1),
  currency TEXT NOT NULL CHECK (length(currency) = 3),
  provider TEXT NOT NULL CHECK (provider IN ('stripe', 'payjp')),
  provider_price_id TEXT NOT NULL CHECK (length(provider_price_id) BETWEEN 1 AND 255),
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX supporter_plan_prices_provider_price ON supporter_plan_prices (provider, provider_price_id);

-- Projection: the price of each plan and interval that new subscriptions and changes of plan use:
-- the newest price of the plan and the interval that is not archived.
CREATE TABLE supporter_plan_price_heads (
  plan_id TEXT NOT NULL,
  interval TEXT NOT NULL,
  price_id TEXT NOT NULL,
  PRIMARY KEY (plan_id, interval)
) STRICT;

-- Event: a price archived at the provider; the subscriptions on it keep renewing.
CREATE TABLE supporter_plan_price_archivals (
  price_id TEXT NOT NULL PRIMARY KEY,
  archived_at INTEGER NOT NULL
) STRICT;

-- Resource: the provider's customer of a user. The erasure of the account deletes it.
CREATE TABLE supporter_customers (
  id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  provider TEXT NOT NULL CHECK (provider IN ('stripe', 'payjp')),
  provider_customer_id TEXT NOT NULL CHECK (length(provider_customer_id) BETWEEN 1 AND 255),
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX supporter_customers_provider_user ON supporter_customers (provider, user_id);
CREATE UNIQUE INDEX supporter_customers_provider_customer ON supporter_customers (provider, provider_customer_id);

-- Resource: a subscription. id is the TEXT that the provider keeps on its checkout and its
-- subscription; plan_id and price_id are the ones it started with.
CREATE TABLE supporter_subscriptions (
  id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  plan_id TEXT NOT NULL,
  price_id TEXT NOT NULL,
  provider TEXT NOT NULL CHECK (provider IN ('stripe', 'payjp')),
  created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX supporter_subscriptions_user_id_id ON supporter_subscriptions (user_id, id DESC);

-- Event: a hosted checkout started for a pending subscription, open until expires_at; starting
-- again while the subscription is pending makes a new one. The reconciliation re-reads the open
-- ones.
CREATE TABLE supporter_checkouts (
  id TEXT NOT NULL PRIMARY KEY,
  subscription_id TEXT NOT NULL,
  provider TEXT NOT NULL CHECK (provider IN ('stripe', 'payjp')),
  provider_checkout_id TEXT NOT NULL CHECK (length(provider_checkout_id) BETWEEN 1 AND 255),
  expires_at INTEGER NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX supporter_checkouts_provider_checkout ON supporter_checkouts (provider, provider_checkout_id);
CREATE INDEX supporter_checkouts_subscription_id_id ON supporter_checkouts (subscription_id, id DESC);
CREATE INDEX supporter_checkouts_provider_expires_at ON supporter_checkouts (provider, expires_at);

-- Event: the provider's subscription of ours, recorded once, from the first snapshot or the
-- completed checkout that names it.
CREATE TABLE supporter_subscription_links (
  subscription_id TEXT NOT NULL PRIMARY KEY,
  provider TEXT NOT NULL CHECK (provider IN ('stripe', 'payjp')),
  provider_subscription_id TEXT NOT NULL CHECK (length(provider_subscription_id) BETWEEN 1 AND 255),
  linked_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX supporter_subscription_links_provider_subscription ON supporter_subscription_links (provider, provider_subscription_id);

-- Event (revision): the state of a subscription after a change (ADR-0072 §7), recorded only when
-- the state, the plan, the price, the end of the period, the grace, or the scheduled change
-- differs from the previous row; the newest row is current. period_ends_at is NULL exactly for
-- pending and abandoned.
CREATE TABLE supporter_subscription_transitions (
  id TEXT NOT NULL PRIMARY KEY,
  subscription_id TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('pending', 'active', 'canceling', 'past_due', 'lapsed', 'expired', 'revoked', 'abandoned')),
  plan_id TEXT NOT NULL,
  price_id TEXT NOT NULL,
  period_ends_at INTEGER,
  reason TEXT NOT NULL CHECK (reason IN (
    'started', 'synced', 'grace_expired', 'reversed', 'plan_retired', 'account_closed', 'account_suspended', 'staff', 'service_closed'
  )),
  occurred_at INTEGER NOT NULL,
  CHECK ((state IN ('pending', 'abandoned')) = (period_ends_at IS NULL))
) STRICT;
CREATE INDEX supporter_subscription_transitions_subscription_id_id ON supporter_subscription_transitions (subscription_id, id DESC);

-- Event: the grace period of a past_due transition: 7 days from the first failed payment.
CREATE TABLE supporter_grace_periods (
  transition_id TEXT NOT NULL PRIMARY KEY,
  failed_at INTEGER NOT NULL,
  ends_at INTEGER NOT NULL,
  CHECK (ends_at > failed_at)
) STRICT;

-- Event: the change to a lower plan or to the monthly price that waits for the end of the period
-- at the time of a transition. No row is no scheduled change.
CREATE TABLE supporter_scheduled_changes (
  transition_id TEXT NOT NULL PRIMARY KEY,
  price_id TEXT NOT NULL,
  effective_at INTEGER NOT NULL
) STRICT;

-- Event: a refund or a chargeback of a payment of a subscription, from the provider's final
-- money movements, once per movement. A full refund and a chargeback revoke the subscription.
CREATE TABLE supporter_subscription_reversals (
  id TEXT NOT NULL PRIMARY KEY,
  subscription_id TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('full_refund', 'partial_refund', 'chargeback')),
  provider TEXT NOT NULL CHECK (provider IN ('stripe', 'payjp')),
  provider_reversal_id TEXT NOT NULL CHECK (length(provider_reversal_id) BETWEEN 1 AND 255),
  occurred_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX supporter_subscription_reversals_provider_reversal ON supporter_subscription_reversals (provider, provider_reversal_id);
CREATE INDEX supporter_subscription_reversals_subscription_id ON supporter_subscription_reversals (subscription_id);

-- Projection: the current transition of each subscription.
CREATE TABLE supporter_subscription_heads (
  subscription_id TEXT NOT NULL PRIMARY KEY,
  transition_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  plan_id TEXT NOT NULL,
  state TEXT NOT NULL
) STRICT;
CREATE INDEX supporter_subscription_heads_plan_id_state ON supporter_subscription_heads (plan_id, state);
CREATE INDEX supporter_subscription_heads_user_id ON supporter_subscription_heads (user_id);

-- Projection: the subscription of each user that is not in a final state; a user has at most one.
CREATE TABLE supporter_live_subscriptions (
  user_id TEXT NOT NULL PRIMARY KEY,
  subscription_id TEXT NOT NULL
) STRICT;
CREATE UNIQUE INDEX supporter_live_subscriptions_subscription_id ON supporter_live_subscriptions (subscription_id);

-- Projection: when the Scheduler re-reads a subscription: a day after a pending one started
-- (checkout), the end of the period of an active or canceling one (period_end), and the end of
-- the grace of a past_due one (grace).
CREATE TABLE supporter_subscription_deadlines (
  subscription_id TEXT NOT NULL PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('checkout', 'period_end', 'grace')),
  due_at INTEGER NOT NULL
) STRICT;
CREATE INDEX supporter_subscription_deadlines_due_at ON supporter_subscription_deadlines (due_at);

-- Current value: when the newest snapshot of the provider applied to a subscription was read
-- (ADR-0074 §3). A snapshot read before it is not applied, so that an older state read at the
-- same time does not replace a newer one.
CREATE TABLE supporter_subscription_reads (
  subscription_id TEXT NOT NULL PRIMARY KEY,
  read_at INTEGER NOT NULL
) STRICT;

-- Current value: the members who chose to be listed on the supporter list (ADR-0071 §6),
-- deleted in the transaction that takes their plan's role away and by the erasure of the account.
CREATE TABLE supporter_listings (
  user_id TEXT NOT NULL PRIMARY KEY,
  created_at INTEGER NOT NULL
) STRICT;

-- Event: a webhook delivery of the payment provider (ADR-0074 §3, §4), recorded once per
-- (provider, event_id) in the transaction that queues its supporters.payment_event job. Only the
-- IDs and the type are kept, never the body (§5). The daily purge deletes the rows older than
-- 45 days; a delivery that comes again after it is recorded again, and its job changes nothing.
CREATE TABLE payment_webhook_receipts (
  id TEXT NOT NULL PRIMARY KEY,
  provider TEXT NOT NULL CHECK (provider IN ('stripe', 'payjp')),
  event_id TEXT NOT NULL CHECK (length(event_id) BETWEEN 1 AND 255),
  event_type TEXT NOT NULL CHECK (length(event_type) BETWEEN 1 AND 100),
  object_id TEXT NOT NULL CHECK (length(object_id) BETWEEN 1 AND 255),
  received_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX payment_webhook_receipts_event ON payment_webhook_receipts (provider, event_id);
CREATE INDEX payment_webhook_receipts_provider_received_at ON payment_webhook_receipts (provider, received_at);
CREATE INDEX payment_webhook_receipts_received_at ON payment_webhook_receipts (received_at);

-- Event: one attempt of the reconciliation of the payment provider (ADR-0074 §4), one per job
-- (run_key): succeeded, partial (it used payments_reconcile_max_requests; error_kind request_cap
-- and a continuation job), or failed with error_kind. The cursors are where the attempt stopped
-- ('' for the start of a list); the next attempt starts from those of the newest succeeded or
-- partial row, and from changes_until minus 5 minutes for the provider's changes.
CREATE TABLE payment_reconcile_runs (
  id TEXT NOT NULL PRIMARY KEY,
  provider TEXT NOT NULL CHECK (provider IN ('stripe', 'payjp')),
  run_key TEXT NOT NULL CHECK (length(run_key) BETWEEN 1 AND 255),
  outcome TEXT NOT NULL CHECK (outcome IN ('succeeded', 'partial', 'failed')),
  error_kind TEXT NOT NULL CHECK (error_kind IN (
    'none', 'request_cap', 'permission', 'not_found', 'invalid_request', 'throttled', 'unavailable',
    'invalid_signature', 'stale_signature', 'mode_mismatch', 'invalid_response'
  )),
  changes_until INTEGER NOT NULL,
  changes_cursor TEXT NOT NULL CHECK (length(changes_cursor) <= 255),
  money_cursor TEXT NOT NULL CHECK (length(money_cursor) <= 255),
  sweep_cursor TEXT NOT NULL CHECK (length(sweep_cursor) <= 255),
  requests INTEGER NOT NULL CHECK (requests >= 0),
  checked_subscriptions INTEGER NOT NULL CHECK (checked_subscriptions >= 0),
  corrected_subscriptions INTEGER NOT NULL CHECK (corrected_subscriptions BETWEEN 0 AND checked_subscriptions),
  occurred_at INTEGER NOT NULL,
  CHECK ((outcome = 'succeeded') = (error_kind = 'none')),
  CHECK ((outcome = 'partial') = (error_kind = 'request_cap'))
) STRICT;
CREATE UNIQUE INDEX payment_reconcile_runs_run_key ON payment_reconcile_runs (provider, run_key);
CREATE INDEX payment_reconcile_runs_provider_occurred_at ON payment_reconcile_runs (provider, occurred_at);
