-- SPDX-License-Identifier: AGPL-3.0-only

-- The supporter program, its plans, and their subscriptions (ADR-0072 section 15).

-- name: InsertSupporterProgramChange :exec
INSERT INTO supporter_program_changes (id, state, provider, changed_by_user_id, changed_at) VALUES ($1, $2, $3, $4, $5);

-- name: GetLastSupporterProgramChange :one
SELECT id, state, provider, changed_by_user_id, changed_at FROM supporter_program_changes ORDER BY id DESC LIMIT 1;

-- name: InsertSupporterPlan :exec
INSERT INTO supporter_plans (id, role_id, currency, created_by_user_id, created_at) VALUES ($1, $2, $3, $4, $5);

-- name: InsertSupporterPlanRevision :exec
INSERT INTO supporter_plan_revisions (
  id, plan_id, name, description, rank, monthly_amount_minor, annual_discount_percent, emoji_limit, visibility,
  revised_by_user_id, revised_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: UpsertSupporterPlanHead :exec
INSERT INTO supporter_plan_heads (plan_id, revision_id, rank, visibility) VALUES ($1, $2, $3, $4)
ON CONFLICT (plan_id) DO UPDATE SET revision_id = excluded.revision_id, rank = excluded.rank, visibility = excluded.visibility;

-- name: InsertSupporterPlanRetirement :exec
INSERT INTO supporter_plan_retirements (plan_id, retired_by_user_id, retired_at) VALUES ($1, $2, $3);

-- name: DeleteSupporterPlanHead :exec
DELETE FROM supporter_plan_heads WHERE plan_id = $1;

-- ListSupporterPlans returns every plan, or the one of plan_id, with its newest revision and
-- its retirement (retired_at NULL for none), by rank.
-- name: ListSupporterPlans :many
SELECT p.id, p.role_id, p.currency, p.created_by_user_id, p.created_at,
  r.id AS revision_id, r.name, r.description, r.rank, r.monthly_amount_minor, r.annual_discount_percent,
  r.emoji_limit, r.visibility, r.revised_by_user_id, r.revised_at,
  EXISTS (SELECT 1 FROM supporter_plan_retirements t WHERE t.plan_id = p.id) AS retired,
  rt.retired_at
FROM supporter_plans p
JOIN supporter_plan_revisions r ON r.id = (SELECT x.id FROM supporter_plan_revisions x WHERE x.plan_id = p.id ORDER BY x.id DESC LIMIT 1)
LEFT JOIN supporter_plan_retirements rt ON rt.plan_id = p.id
WHERE sqlc.narg(plan_id)::uuid IS NULL OR p.id = sqlc.narg(plan_id)::uuid
ORDER BY r.rank, p.id;

-- ListSupporterPriceHeads returns the current prices of every plan, or of the one of plan_id.
-- name: ListSupporterPriceHeads :many
SELECT h.plan_id, h.interval, pr.id, pr.amount_minor, pr.currency, pr.provider, pr.provider_price_id, pr.created_at,
  EXISTS (SELECT 1 FROM supporter_plan_price_archivals a WHERE a.price_id = pr.id) AS archived
FROM supporter_plan_price_heads h
JOIN supporter_plan_prices pr ON pr.id = h.price_id
WHERE sqlc.narg(plan_id)::uuid IS NULL OR h.plan_id = sqlc.narg(plan_id)::uuid
ORDER BY h.plan_id, h.interval;

-- name: InsertSupporterPrice :exec
INSERT INTO supporter_plan_prices (id, plan_id, interval, amount_minor, currency, provider, provider_price_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT DO NOTHING;

-- name: UpsertSupporterPriceHead :exec
INSERT INTO supporter_plan_price_heads (plan_id, interval, price_id) VALUES ($1, $2, $3)
ON CONFLICT (plan_id, interval) DO UPDATE SET price_id = excluded.price_id;

-- name: InsertSupporterPriceArchival :exec
INSERT INTO supporter_plan_price_archivals (price_id, archived_at) VALUES ($1, $2)
ON CONFLICT (price_id) DO NOTHING;

-- name: GetSupporterPriceByRef :one
SELECT pr.id, pr.plan_id, pr.interval, pr.amount_minor, pr.currency, pr.provider, pr.provider_price_id, pr.created_at,
  EXISTS (SELECT 1 FROM supporter_plan_price_archivals a WHERE a.price_id = pr.id) AS archived
FROM supporter_plan_prices pr WHERE pr.provider = $1 AND pr.provider_price_id = $2;

-- name: GetSupporterPrice :one
SELECT pr.id, pr.plan_id, pr.interval, pr.amount_minor, pr.currency, pr.provider, pr.provider_price_id, pr.created_at,
  EXISTS (SELECT 1 FROM supporter_plan_price_archivals a WHERE a.price_id = pr.id) AS archived
FROM supporter_plan_prices pr WHERE pr.id = $1;

-- name: InsertSupporterCustomer :exec
INSERT INTO supporter_customers (id, user_id, provider, provider_customer_id, created_at) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT DO NOTHING;

-- name: GetSupporterCustomer :one
SELECT id, user_id, provider, provider_customer_id, created_at FROM supporter_customers WHERE provider = $1 AND user_id = $2;

-- name: InsertSupporterSubscription :exec
INSERT INTO supporter_subscriptions (id, user_id, plan_id, price_id, provider, created_at) VALUES ($1, $2, $3, $4, $5, $6);

-- name: InsertSupporterTransition :exec
INSERT INTO supporter_subscription_transitions (id, subscription_id, state, plan_id, price_id, period_ends_at, reason, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: InsertSupporterGracePeriod :exec
INSERT INTO supporter_grace_periods (transition_id, failed_at, ends_at) VALUES ($1, $2, $3);

-- name: InsertSupporterScheduledChange :exec
INSERT INTO supporter_scheduled_changes (transition_id, price_id, effective_at) VALUES ($1, $2, $3);

-- name: UpsertSupporterSubscriptionHead :exec
INSERT INTO supporter_subscription_heads (subscription_id, transition_id, user_id, plan_id, state) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (subscription_id) DO UPDATE SET transition_id = excluded.transition_id, user_id = excluded.user_id,
  plan_id = excluded.plan_id, state = excluded.state;

-- name: InsertSupporterLiveSubscription :exec
INSERT INTO supporter_live_subscriptions (user_id, subscription_id) VALUES ($1, $2);

-- name: EnsureSupporterLiveSubscription :exec
INSERT INTO supporter_live_subscriptions (user_id, subscription_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: DeleteSupporterLiveSubscription :exec
DELETE FROM supporter_live_subscriptions WHERE subscription_id = $1;

-- name: UpsertSupporterDeadline :exec
INSERT INTO supporter_subscription_deadlines (subscription_id, kind, due_at) VALUES ($1, $2, $3)
ON CONFLICT (subscription_id) DO UPDATE SET kind = excluded.kind, due_at = excluded.due_at;

-- name: DeleteSupporterDeadline :exec
DELETE FROM supporter_subscription_deadlines WHERE subscription_id = $1;

-- name: InsertSupporterCheckout :exec
INSERT INTO supporter_checkouts (id, subscription_id, provider, provider_checkout_id, expires_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetLatestSupporterCheckout :one
SELECT id, subscription_id, provider, provider_checkout_id, expires_at, created_at
FROM supporter_checkouts WHERE subscription_id = $1 ORDER BY id DESC LIMIT 1;

-- GetSupporterSubscription returns a subscription with its current transition.
-- name: GetSupporterSubscription :one
SELECT s.id, s.user_id, s.plan_id, s.price_id, s.provider, s.created_at,
  t.id AS transition_id, t.state, t.plan_id AS transition_plan_id, t.price_id AS transition_price_id,
  t.period_ends_at, t.reason, t.occurred_at
FROM supporter_subscriptions s
JOIN supporter_subscription_heads h ON h.subscription_id = s.id
JOIN supporter_subscription_transitions t ON t.id = h.transition_id
WHERE s.id = $1;

-- GetSupporterSubscriptionForUpdate reads and locks a subscription inside the transaction that
-- changes it.
-- name: GetSupporterSubscriptionForUpdate :one
SELECT s.id, s.user_id, s.plan_id, s.price_id, s.provider, s.created_at,
  t.id AS transition_id, t.state, t.plan_id AS transition_plan_id, t.price_id AS transition_price_id,
  t.period_ends_at, t.reason, t.occurred_at
FROM supporter_subscriptions s
JOIN supporter_subscription_heads h ON h.subscription_id = s.id
JOIN supporter_subscription_transitions t ON t.id = h.transition_id
WHERE s.id = $1
FOR UPDATE OF s;

-- name: GetSupporterGracePeriod :one
SELECT failed_at, ends_at FROM supporter_grace_periods WHERE transition_id = $1;

-- name: GetSupporterScheduledChange :one
SELECT price_id, effective_at FROM supporter_scheduled_changes WHERE transition_id = $1;

-- name: GetSupporterLink :one
SELECT provider_subscription_id FROM supporter_subscription_links WHERE subscription_id = $1;

-- name: InsertSupporterLink :exec
INSERT INTO supporter_subscription_links (subscription_id, provider, provider_subscription_id, linked_at) VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: GetSupporterSubscriptionIDByRef :one
SELECT subscription_id FROM supporter_subscription_links WHERE provider = $1 AND provider_subscription_id = $2;

-- name: GetSupporterSubscriptionIDByCheckout :one
SELECT subscription_id FROM supporter_checkouts WHERE provider = $1 AND provider_checkout_id = $2;

-- name: GetSupporterLiveSubscriptionID :one
SELECT subscription_id FROM supporter_live_subscriptions WHERE user_id = $1;

-- name: InsertSupporterReversal :execrows
INSERT INTO supporter_subscription_reversals (id, subscription_id, kind, provider, provider_reversal_id, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (provider, provider_reversal_id) DO NOTHING;

-- name: HasRevokingSupporterReversal :one
SELECT EXISTS (
  SELECT 1 FROM supporter_subscription_reversals WHERE subscription_id = $1 AND kind IN ('full_refund', 'chargeback')
) AS found;

-- ListKnownSupporterReversals returns the reversals of provider among ids, a JSON array.
-- name: ListKnownSupporterReversals :many
SELECT provider_reversal_id FROM supporter_subscription_reversals
WHERE provider = sqlc.arg(provider) AND provider_reversal_id = ANY(sqlc.arg(ids)::text[])
ORDER BY provider_reversal_id;

-- ListOpenSupporterCheckouts returns the latest checkouts of the pending subscriptions that
-- have not expired.
-- name: ListOpenSupporterCheckouts :many
SELECT c.provider_checkout_id FROM supporter_checkouts c
JOIN supporter_subscription_heads h ON h.subscription_id = c.subscription_id
WHERE c.provider = sqlc.arg(provider) AND h.state = 'pending' AND c.expires_at > sqlc.arg(now)
  AND c.id = (SELECT x.id FROM supporter_checkouts x WHERE x.subscription_id = c.subscription_id ORDER BY x.id DESC LIMIT 1)
ORDER BY c.id;

-- name: ListDueSupporterDeadlines :many
SELECT subscription_id, kind, due_at FROM supporter_subscription_deadlines
WHERE due_at <= sqlc.arg(now) ORDER BY due_at, subscription_id LIMIT sqlc.arg(lim);

-- ListSupporterSubscriptionIDs returns a page of subscriptions, newest first, of plan_id and
-- state when they are not ''.
-- name: ListSupporterSubscriptionIDs :many
SELECT h.subscription_id FROM supporter_subscription_heads h
WHERE (sqlc.narg(plan_id)::uuid IS NULL OR h.plan_id = sqlc.narg(plan_id)::uuid)
  AND (sqlc.arg(state)::text = '' OR h.state = sqlc.arg(state)::text)
  AND (sqlc.narg(before)::uuid IS NULL OR h.subscription_id < sqlc.narg(before)::uuid)
ORDER BY h.subscription_id DESC LIMIT sqlc.arg(lim);

-- name: ListLiveSupporterSubscriptionIDsOfPlan :many
SELECT l.subscription_id FROM supporter_live_subscriptions l
JOIN supporter_subscription_heads h ON h.subscription_id = l.subscription_id
WHERE h.plan_id = $1 ORDER BY l.subscription_id;

-- name: ListLiveSupporterSubscriptionIDs :many
SELECT subscription_id FROM supporter_live_subscriptions ORDER BY subscription_id;

-- name: CountSupporterLiveSubscriptions :one
SELECT COUNT(*) FROM supporter_live_subscriptions;

-- name: CountSupporterMembers :many
SELECT plan_id, COUNT(*) AS members FROM supporter_subscription_heads
WHERE state IN ('active', 'canceling', 'past_due') GROUP BY plan_id ORDER BY plan_id;

-- ListSupporterSubscriptionUsers returns the subscribers of the subscriptions among ids, a JSON array.
-- name: ListSupporterSubscriptionUsers :many
SELECT id, user_id, plan_id FROM supporter_subscriptions
WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY id;

-- name: InsertSupporterListing :exec
INSERT INTO supporter_listings (user_id, created_at) VALUES ($1, $2) ON CONFLICT (user_id) DO NOTHING;

-- name: HasSupporterListing :one
SELECT EXISTS (SELECT 1 FROM supporter_listings WHERE user_id = $1) AS found;

-- ListSupporterListings returns a page of the listed members of the plans that are not
-- retired, by the plan's rank and newest first within a plan, after the member of after_user_id
-- (its plan's rank and when it was listed) unless that is ''; of plan_id unless it is ''.
-- name: ListSupporterListings :many
SELECT l.user_id, h.plan_id, ph.rank, l.created_at FROM supporter_listings l
JOIN supporter_live_subscriptions s ON s.user_id = l.user_id
JOIN supporter_subscription_heads h ON h.subscription_id = s.subscription_id
JOIN supporter_plan_heads ph ON ph.plan_id = h.plan_id
WHERE h.state IN ('active', 'canceling', 'past_due')
  AND (sqlc.narg(plan_id)::uuid IS NULL OR h.plan_id = sqlc.narg(plan_id)::uuid)
  AND (sqlc.narg(after_user_id)::uuid IS NULL OR ph.rank > sqlc.arg(after_rank)::integer
    OR (ph.rank = sqlc.arg(after_rank)::integer AND (l.created_at < sqlc.arg(after_at)::timestamptz
      OR (l.created_at = sqlc.arg(after_at)::timestamptz AND l.user_id < sqlc.narg(after_user_id)::uuid))))
ORDER BY ph.rank, l.created_at DESC, l.user_id DESC LIMIT sqlc.arg(lim);

-- name: DeleteSupporterListing :exec
DELETE FROM supporter_listings WHERE user_id = $1;

-- name: DeleteSupporterCustomersOfUser :exec
DELETE FROM supporter_customers WHERE user_id = $1;

-- Rebuilding the projections (data-model section 9).

-- name: DeleteAllSupporterPlanHeads :exec
DELETE FROM supporter_plan_heads;

-- name: RebuildSupporterPlanHeads :exec
INSERT INTO supporter_plan_heads (plan_id, revision_id, rank, visibility)
SELECT r.plan_id, r.id, r.rank, r.visibility FROM supporter_plan_revisions r
WHERE r.id = (SELECT x.id FROM supporter_plan_revisions x WHERE x.plan_id = r.plan_id ORDER BY x.id DESC LIMIT 1)
  AND NOT EXISTS (SELECT 1 FROM supporter_plan_retirements t WHERE t.plan_id = r.plan_id);

-- name: DeleteAllSupporterPriceHeads :exec
DELETE FROM supporter_plan_price_heads;

-- name: RebuildSupporterPriceHeads :exec
INSERT INTO supporter_plan_price_heads (plan_id, interval, price_id)
SELECT pr.plan_id, pr.interval, pr.id FROM supporter_plan_prices pr
WHERE pr.id = (
  SELECT x.id FROM supporter_plan_prices x
  WHERE x.plan_id = pr.plan_id AND x.interval = pr.interval
    AND NOT EXISTS (SELECT 1 FROM supporter_plan_price_archivals a WHERE a.price_id = x.id)
  ORDER BY x.id DESC LIMIT 1
);

-- name: DeleteAllSupporterSubscriptionHeads :exec
DELETE FROM supporter_subscription_heads;

-- name: RebuildSupporterSubscriptionHeads :exec
INSERT INTO supporter_subscription_heads (subscription_id, transition_id, user_id, plan_id, state)
SELECT t.subscription_id, t.id, s.user_id, t.plan_id, t.state
FROM supporter_subscription_transitions t
JOIN supporter_subscriptions s ON s.id = t.subscription_id
WHERE t.id = (SELECT x.id FROM supporter_subscription_transitions x WHERE x.subscription_id = t.subscription_id ORDER BY x.id DESC LIMIT 1);

-- name: DeleteAllSupporterLiveSubscriptions :exec
DELETE FROM supporter_live_subscriptions;

-- name: RebuildSupporterLiveSubscriptions :exec
INSERT INTO supporter_live_subscriptions (user_id, subscription_id)
SELECT h.user_id, h.subscription_id FROM supporter_subscription_heads h
WHERE h.state NOT IN ('expired', 'revoked', 'abandoned');

-- name: DeleteAllSupporterDeadlines :exec
DELETE FROM supporter_subscription_deadlines;

-- name: RebuildSupporterDeadlines :exec
INSERT INTO supporter_subscription_deadlines (subscription_id, kind, due_at)
SELECT h.subscription_id, 'checkout', s.created_at + interval '24 hours'
FROM supporter_subscription_heads h JOIN supporter_subscriptions s ON s.id = h.subscription_id
WHERE h.state = 'pending'
UNION ALL
SELECT h.subscription_id, 'period_end', t.period_ends_at
FROM supporter_subscription_heads h JOIN supporter_subscription_transitions t ON t.id = h.transition_id
WHERE h.state IN ('active', 'canceling') AND t.period_ends_at IS NOT NULL
UNION ALL
SELECT h.subscription_id, 'grace', g.ends_at
FROM supporter_subscription_heads h JOIN supporter_grace_periods g ON g.transition_id = h.transition_id
WHERE h.state = 'past_due';
