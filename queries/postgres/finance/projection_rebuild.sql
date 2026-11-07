-- SPDX-License-Identifier: AGPL-3.0-only

-- The sources and the emptying of the projections of moderation, identity, content (the
-- sensitivity overrides and the poll tallies), and timeline (the restrictions) that
-- `my projection rebuild` derives again (data-model section 9). The Rebuild queries state the
-- rules of the commands and projectors in SQL.

-- name: DeleteAllCostItemHeads :exec
DELETE FROM cost_item_heads;

-- RebuildCostItemHeads states the rule of the cost item commands: every item that is not
-- canceled, with its newest label revision and end change (no end without one).
-- name: RebuildCostItemHeads :exec
INSERT INTO cost_item_heads (cost_item_id, label_revision_id, ends_on)
SELECT i.id,
  (SELECT l.id FROM cost_item_label_revisions l WHERE l.cost_item_id = i.id ORDER BY l.id DESC LIMIT 1),
  (SELECT e.ends_on FROM cost_item_end_changes e WHERE e.cost_item_id = i.id ORDER BY e.id DESC LIMIT 1)
FROM cost_items i
WHERE NOT EXISTS (SELECT 1 FROM cost_item_cancellations c WHERE c.cost_item_id = i.id);

-- name: DeleteAllImportedCostHeads :exec
DELETE FROM imported_cost_heads;

-- RebuildImportedCostHeads states the rule of the billing imports: the newest revision of each
-- key (the largest id, a UUIDv7 that follows the recording order).
-- name: RebuildImportedCostHeads :exec
INSERT INTO imported_cost_heads (source, billing_month, service, charge_type, revision_id)
SELECT DISTINCT ON (r.source, r.billing_month, r.service, r.charge_type)
  r.source, r.billing_month, r.service, r.charge_type, r.id
FROM imported_cost_revisions r
ORDER BY r.source, r.billing_month, r.service, r.charge_type, r.id DESC;
