// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
)

// Domain events of the finance module.
const (
	// EventCostItemChanged follows the creation of a cost item and every change of its label or
	// end, with its new head.
	EventCostItemChanged = "finance.cost_item_changed"
	// EventCostItemCanceled follows the cancellation of a cost item.
	EventCostItemCanceled = "finance.cost_item_canceled"
)

// CostItemChanged carries the new head of a cost item.
type CostItemChanged struct{ Head Head }

// EventType implements uow.Event.
func (CostItemChanged) EventType() string { return EventCostItemChanged }

// CostItemCanceled carries the cancellation of a cost item.
type CostItemCanceled struct {
	CostItemID     string
	CancellationID string
	At             time.Time
}

// EventType implements uow.Event.
func (CostItemCanceled) EventType() string { return EventCostItemCanceled }

// RegisterProjectors registers the projector of the module's own projection: the cost item heads
// from CostItemChanged and CostItemCanceled.
func RegisterProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventCostItemChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.UpsertHead(ctx, tx, e.(CostItemChanged).Head)
	})
	r.Register(EventCostItemCanceled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.DeleteHead(ctx, tx, e.(CostItemCanceled).CostItemID)
	})
}

// RebuildProjections derives cost_item_heads again from the cost items, their label revisions,
// end changes, and cancellations, and imported_cost_heads from the imported cost revisions, in
// one transaction.
func RebuildProjections(ctx context.Context, u uow.UnitOfWork, repo Repository) error {
	err := u.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.RebuildHeads(ctx, tx); err != nil {
			return err
		}
		return repo.RebuildImportedHeads(ctx, tx)
	})
	if err != nil {
		return fmt.Errorf("rebuild the finance projections: %w", err)
	}
	return nil
}
