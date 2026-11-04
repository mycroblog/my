// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// ErasableStatus is a status of a deleted account, with whether it is deleted already.
type ErasableStatus struct {
	Status  content.Status
	Deleted bool
}

// ErasureRepository is the part of the content module's persistence that the erasure of deleted
// accounts uses (data-model §8). The database adapters' status repositories implement it and
// pass testkit.AccountErasureContract.
type ErasureRepository interface {
	// ListAuthorStatusesToErase returns up to limit statuses of the author in ID order after the
	// status ID after, deleted or not.
	ListAuthorStatusesToErase(ctx context.Context, tx uow.Tx, authorAccountID, after string, limit int) ([]ErasableStatus, error)
	// EraseStatuses deletes the rows of deleted statuses: what PurgeStatuses deletes, their local
	// rows and counters, every boost of them, and the favourites, reactions (with their counts),
	// bookmarks, and pins of them. The deletions stay.
	EraseStatuses(ctx context.Context, tx uow.Tx, statusIDs []string) error
	// EraseInteractionsBy deletes up to limit of each of the account's favourites, reactions,
	// bookmarks, pins, boosts (with their retractions), and remote media files that nothing uses,
	// and its automatic deletion policy, counting the favourites, the reactions, and the boosts in force out of the statuses'
	// counters, and returns the most rows that one kind lost.
	EraseInteractionsBy(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error)
}

// StatusNotificationEraser deletes inside tx the notifications about statuses, with their
// dismissals and their rows of deleted statuses; *notification.Service implements it.
type StatusNotificationEraser interface {
	EraseStatusNotifications(ctx context.Context, tx uow.Tx, statusIDs []string) error
}

// ErasureDeps are the ports of the content module's part of the erasure.
type ErasureDeps struct {
	UnitOfWork    uow.UnitOfWork
	Repository    Repository
	Erasure       ErasureRepository
	Notifications StatusNotificationEraser
	// Evidence keeps the statuses that the moderation records need; nil keeps none.
	Evidence RetainedStatuses
	Clock    kernel.Clock
}

// StatusEraser erases the statuses of deleted accounts, local and remote (data-model §8): each status that
// is not deleted yet gets a deletion with origin account_deletion, which takes it out of every
// projection as the author's deletion does but sends no Delete (the Delete of the actor stands
// for them, federation.md §7.1); then its rows, every boost, favourite, reaction, bookmark, and pin of it,
// and the notifications about it go. A status that a report or an action points at keeps its
// rows (ADR-0058) until a later run finds the record gone.
type StatusEraser struct{ d ErasureDeps }

// NewStatusEraser returns the content module's eraser of statuses on d.
func NewStatusEraser(d ErasureDeps) *StatusEraser { return &StatusEraser{d: d} }

// EraseAccountData implements account.DataEraser: one batch of the author's statuses after the
// cursor (a status ID).
func (e *StatusEraser) EraseAccountData(ctx context.Context, t appaccount.ErasureTarget, cursor string, limit int) (appaccount.ErasureStep, error) {
	now := kernel.Normalize(e.d.Clock.Now())
	var out appaccount.ErasureStep
	err := e.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		out = appaccount.ErasureStep{Next: cursor}
		statuses, err := e.d.Erasure.ListAuthorStatusesToErase(ctx, tx, t.AccountID, cursor, limit)
		if err != nil {
			return err
		}
		var ids []string
		for _, s := range statuses {
			st := s.Status
			out.Next = st.ID
			if !s.Deleted {
				d := content.Deletion{
					StatusID: st.ID, URI: st.URI, AuthorAccountID: st.AuthorAccountID,
					Origin: content.DeletedByAccountDeletion, DeletedAt: now,
				}
				if err := e.d.Repository.InsertDeletion(ctx, tx, d); err != nil {
					return fmt.Errorf("erase %s: %w", st.URI, err)
				}
				if err := tx.Emit(ctx, StatusDeleted{Deletion: d, Status: st}); err != nil {
					return err
				}
			}
			if e.d.Evidence != nil {
				retained, err := e.d.Evidence.StatusRetained(ctx, tx, st.ID, st.AuthorAccountID)
				if err != nil {
					return err
				}
				if retained {
					out.Kept = true
					continue
				}
			}
			ids = append(ids, st.ID)
		}
		out.Done = len(statuses) < limit
		if len(ids) == 0 {
			return nil
		}
		if e.d.Notifications != nil {
			if err := e.d.Notifications.EraseStatusNotifications(ctx, tx, ids); err != nil {
				return err
			}
		}
		return e.d.Erasure.EraseStatuses(ctx, tx, ids)
	})
	return out, err
}

// InteractionEraser erases a deleted account's favourites, reactions, bookmarks, pins, and boosts of
// other statuses (data-model §8), and a remote account's media files, counting them out of the statuses' counters. The timeline
// entries of its boosts and the notifications of its interactions go with the timeline and
// notification modules' erasers.
type InteractionEraser struct {
	uow     uow.UnitOfWork
	erasure ErasureRepository
}

// NewInteractionEraser returns the content module's eraser of interactions.
func NewInteractionEraser(u uow.UnitOfWork, r ErasureRepository) *InteractionEraser {
	return &InteractionEraser{uow: u, erasure: r}
}

// EraseAccountData implements account.DataEraser.
func (e *InteractionEraser) EraseAccountData(ctx context.Context, t appaccount.ErasureTarget, _ string, limit int) (appaccount.ErasureStep, error) {
	var n int64
	err := e.uow.Tx(ctx, func(tx uow.Tx) error {
		var err error
		n, err = e.erasure.EraseInteractionsBy(ctx, tx, t.AccountID, limit)
		return err
	})
	return appaccount.ErasureStep{Done: n < int64(limit)}, err
}
