-- SPDX-License-Identifier: AGPL-3.0-only

-- The instance's costs (data-model 4.12, ADR-0067). Cost items, label and price revisions, end
-- changes, and cancellations are written once; cost_item_heads is the projection. Dates are
-- TEXT as YYYY-MM-DD.

-- name: InsertCostItem :exec
INSERT INTO cost_items (id, schedule, interval, starts_on, created_by_user_id, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: InsertCostItemLabelRevision :exec
INSERT INTO cost_item_label_revisions (
  id, cost_item_id, name, provider, provider_normalized, category, memo, revised_by_user_id, revised_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertCostItemPriceRevision :exec
INSERT INTO cost_item_price_revisions (id, cost_item_id, effective_on, amount_minor, currency, revised_by_user_id, revised_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: InsertCostItemEndChange :exec
INSERT INTO cost_item_end_changes (id, cost_item_id, ends_on, changed_by_user_id, changed_at) VALUES (?, ?, ?, ?, ?);

-- name: InsertCostItemCancellation :exec
INSERT INTO cost_item_cancellations (id, cost_item_id, canceled_by_user_id, canceled_at) VALUES (?, ?, ?, ?);

-- name: UpsertCostItemHead :exec
INSERT INTO cost_item_heads (cost_item_id, label_revision_id, ends_on) VALUES (?, ?, ?)
ON CONFLICT (cost_item_id) DO UPDATE SET label_revision_id = excluded.label_revision_id, ends_on = excluded.ends_on;

-- name: DeleteCostItemHead :exec
DELETE FROM cost_item_heads WHERE cost_item_id = ?;

-- name: GetCostItemCurrent :one
SELECT h.cost_item_id, h.label_revision_id, h.ends_on, i.starts_on, l.name
FROM cost_item_heads h JOIN cost_items i ON i.id = h.cost_item_id
  JOIN cost_item_label_revisions l ON l.id = h.label_revision_id
WHERE h.cost_item_id = ?;

-- ListCostItems returns up to lim cost items that are not canceled, newest first, with their
-- current label and end: only the item id unless it is empty, and only those before before
-- unless it is empty.
-- name: ListCostItems :many
SELECT i.id, i.schedule, i.interval, i.starts_on, i.created_by_user_id, i.created_at, h.ends_on,
  l.name, l.provider, l.provider_normalized, l.category, l.memo, h.label_revision_id, l.revised_at AS label_revised_at
FROM cost_item_heads h
JOIN cost_items i ON i.id = h.cost_item_id
JOIN cost_item_label_revisions l ON l.id = h.label_revision_id
WHERE (CAST(sqlc.arg(id) AS TEXT) = '' OR h.cost_item_id = sqlc.arg(id))
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR h.cost_item_id < sqlc.arg(before))
ORDER BY h.cost_item_id DESC
LIMIT sqlc.arg(lim);

-- ListCostItemPriceRevisions returns the price revisions of the cost items ids (a JSON array of
-- strings) in the order they were recorded.
-- name: ListCostItemPriceRevisions :many
SELECT id, cost_item_id, effective_on, amount_minor, currency, revised_at FROM cost_item_price_revisions
WHERE cost_item_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))
ORDER BY cost_item_id, id;

-- The billing imports (ADR-0068 section 5). billing_import_runs and imported_cost_revisions are
-- written once; imported_cost_heads is the projection of the revisions.

-- name: InsertBillingImportRun :exec
INSERT INTO billing_import_runs (id, source, run_key, outcome, error_kind, requests, bytes_billed, occurred_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- SumBillingImportRuns sums the requests and the bytes billed of the attempts of a source that
-- occurred in [from, until), counting the reservation of an attempt that is not recorded yet in
-- place of its requests.
-- name: SumBillingImportRuns :one
SELECT CAST(COALESCE(SUM(requests), 0) AS INTEGER) AS requests, CAST(COALESCE(SUM(bytes_billed), 0) AS INTEGER) AS bytes_billed
FROM (
  SELECT u.requests, u.bytes_billed FROM billing_import_runs u
  WHERE u.source = sqlc.arg(source) AND u.occurred_at >= sqlc.arg(from_at) AND u.occurred_at < sqlc.arg(until_at)
  UNION ALL
  SELECT r.requests, 0 FROM billing_import_reservations r
  WHERE r.source = sqlc.arg(source) AND r.reserved_at >= sqlc.arg(from_at) AND r.reserved_at < sqlc.arg(until_at)
    AND NOT EXISTS (SELECT 1 FROM billing_import_runs u WHERE u.id = r.id)
);

-- InsertBillingImportReservation records the requests that an attempt reserved before it calls
-- the provider.
-- name: InsertBillingImportReservation :exec
INSERT INTO billing_import_reservations (id, source, run_key, requests, reserved_at) VALUES (?, ?, ?, ?, ?);

-- PendingBillingImportReservation reports whether an attempt of a source reserved requests at or
-- after since and is not recorded yet: it is still running.
-- name: PendingBillingImportReservation :one
SELECT CAST(EXISTS (
  SELECT 1 FROM billing_import_reservations r
  WHERE r.source = sqlc.arg(source) AND r.reserved_at >= sqlc.arg(since)
    AND NOT EXISTS (SELECT 1 FROM billing_import_runs u WHERE u.id = r.id)
) AS INTEGER) AS pending;

-- BillingImportRunKeySucceeded reports whether an attempt of a source with the run key succeeded.
-- name: BillingImportRunKeySucceeded :one
SELECT CAST(EXISTS (
  SELECT 1 FROM billing_import_runs WHERE source = ? AND run_key = ? AND outcome = 'succeeded'
) AS INTEGER) AS succeeded;

-- name: CountBillingImportRunsByKey :one
SELECT COUNT(*) FROM billing_import_runs WHERE source = ? AND run_key = ?;

-- LastBillingImportRun returns the newest attempt of a source, only among the successes unless
-- include_failed is true.
-- name: LastBillingImportRun :one
SELECT id, source, run_key, outcome, error_kind, requests, bytes_billed, occurred_at FROM billing_import_runs
WHERE source = sqlc.arg(source) AND (CAST(sqlc.arg(include_failed) AS INTEGER) = 1 OR outcome = 'succeeded')
ORDER BY occurred_at DESC, id DESC
LIMIT 1;

-- InsertImportedCostRevision writes nothing when an attempt of the same job wrote the key.
-- name: InsertImportedCostRevision :execrows
INSERT INTO imported_cost_revisions (
  id, source, billing_month, service, charge_type, category, starts_on, ends_on, amount_minor, currency, kind,
  run_id, run_key, recorded_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (source, billing_month, service, charge_type, run_key) DO NOTHING;

-- name: UpsertImportedCostHead :exec
INSERT INTO imported_cost_heads (source, billing_month, service, charge_type, revision_id) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (source, billing_month, service, charge_type) DO UPDATE SET revision_id = excluded.revision_id;

-- ListImportedCostHeads returns the newest revision of every key, of one source unless source is
-- empty and of the billing months months (a JSON array of YYYY-MM) unless it is '[]'.
-- name: ListImportedCostHeads :many
SELECT r.id, r.source, r.billing_month, r.service, r.charge_type, r.category, r.starts_on, r.ends_on, r.amount_minor,
  r.currency, r.kind, r.run_id, r.run_key, r.recorded_at
FROM imported_cost_heads h JOIN imported_cost_revisions r ON r.id = h.revision_id
WHERE (CAST(sqlc.arg(source) AS TEXT) = '' OR h.source = sqlc.arg(source))
  AND (CAST(sqlc.arg(months) AS TEXT) = '[]' OR h.billing_month IN (SELECT value FROM json_each(CAST(sqlc.arg(months) AS TEXT))))
ORDER BY h.source, h.billing_month, h.service, h.charge_type;

-- The income records (ADR-0073 section 8). income_movements is written once per (provider,
-- provider_id); the totals read it on every request.

-- InsertIncomeMovement writes nothing when the movement is already recorded.
-- name: InsertIncomeMovement :execrows
INSERT INTO income_movements (
  id, provider, provider_id, kind, parent_movement_id, subscription_id, occurred_at, occurred_on, covers_starts_on,
  covers_ends_on, gross_minor, fee_minor, net_minor, currency, recorded_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (provider, provider_id) DO NOTHING;

-- name: FindIncomeMovementID :one
SELECT id FROM income_movements WHERE provider = ? AND provider_id = ?;

-- KnownIncomeMovements returns the provider IDs among ids (a JSON array) that are recorded.
-- name: KnownIncomeMovements :many
SELECT provider_id FROM income_movements
WHERE provider = sqlc.arg(provider) AND provider_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- ListIncomeMovementsInWindow returns the movements whose place in the totals can meet the
-- dates from starts_on to ends_on: those on the dates, the payments whose period meets them,
-- and the movements on or before ends_on of a payment whose period ends on or after starts_on,
-- with the period of their payment.
-- name: ListIncomeMovementsInWindow :many
SELECT m.kind, m.occurred_on, m.covers_starts_on, m.covers_ends_on, m.gross_minor, m.fee_minor, m.net_minor, m.currency,
  p.covers_starts_on AS parent_covers_starts_on, p.covers_ends_on AS parent_covers_ends_on
FROM income_movements m LEFT JOIN income_movements p ON p.id = m.parent_movement_id
WHERE (m.occurred_on >= sqlc.arg(starts_on) AND m.occurred_on <= sqlc.arg(ends_on))
  OR (m.covers_starts_on <= sqlc.arg(ends_on) AND m.covers_ends_on >= sqlc.arg(starts_on))
  OR (m.occurred_on <= sqlc.arg(ends_on) AND p.covers_ends_on >= sqlc.arg(starts_on))
ORDER BY m.occurred_on, m.id;

-- ListIncomeMovements returns up to lim movements on the dates from starts_on to ends_on, of
-- kind unless it is empty, newest first, before the movement before unless it is empty.
-- name: ListIncomeMovements :many
SELECT m.id, m.provider, m.provider_id, m.kind, m.subscription_id, m.occurred_at, m.occurred_on, m.covers_starts_on,
  m.covers_ends_on, m.gross_minor, m.fee_minor, m.net_minor, m.currency, m.recorded_at, p.provider_id AS parent_provider_id
FROM income_movements m LEFT JOIN income_movements p ON p.id = m.parent_movement_id
WHERE m.occurred_on >= sqlc.arg(starts_on) AND m.occurred_on <= sqlc.arg(ends_on)
  AND (CAST(sqlc.arg(kind) AS TEXT) = '' OR m.kind = sqlc.arg(kind))
  AND (CAST(sqlc.arg(before) AS TEXT) = ''
    OR m.occurred_at < (SELECT b.occurred_at FROM income_movements b WHERE b.id = sqlc.arg(before))
    OR (m.occurred_at = (SELECT b.occurred_at FROM income_movements b WHERE b.id = sqlc.arg(before)) AND m.id < sqlc.arg(before)))
ORDER BY m.occurred_at DESC, m.id DESC
LIMIT sqlc.arg(lim);
