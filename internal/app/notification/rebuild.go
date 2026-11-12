// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"fmt"

	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
)

// Source replays events of another module inside tx through emit, for RebuildExclusions.
type Source func(ctx context.Context, tx uow.Tx, emit func(uow.Event) error) error

// GraphSource replays the blocks and mutes of the graph module (appgraph.ReplayEvents).
func GraphSource(repo appgraph.Repository) Source {
	return func(ctx context.Context, tx uow.Tx, emit func(uow.Event) error) error {
		return appgraph.ReplayEvents(ctx, tx, repo, emit)
	}
}

// DeletedStatuses is the part of the content module's public API that rebuilding
// notification_deleted_statuses reads: whether a status is deleted. *appcontent.Service
// implements it.
type DeletedStatuses interface {
	IsDeleted(ctx context.Context, tx uow.Tx, statusID string) (bool, error)
}

// RebuildExclusions empties notification_exclusions and derives it again, in one transaction,
// by applying the exclusion projectors to the events that graph replays: the blocks in force
// and the mutes. It creates no notifications.
func RebuildExclusions(ctx context.Context, u uow.UnitOfWork, repo Repository, graph Source) error {
	projectors := &uow.Projectors{}
	registerExclusionProjectors(projectors, repo)
	err := u.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.DeleteAllExclusions(ctx, tx); err != nil {
			return err
		}
		return graph(ctx, tx, func(e uow.Event) error { return projectors.Project(ctx, tx, e) })
	})
	if err != nil {
		return fmt.Errorf("rebuild notification_exclusions: %w", err)
	}
	return nil
}

// RebuildDeletedStatuses empties notification_deleted_statuses and records again, in one
// transaction, every status that a notification is about and that the content module has
// deleted: the rows that the projector of content.status_deleted keeps.
func RebuildDeletedStatuses(ctx context.Context, u uow.UnitOfWork, repo Repository, statuses DeletedStatuses) error {
	err := u.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.DeleteAllDeletedStatuses(ctx, tx); err != nil {
			return err
		}
		ids, err := repo.ListNotifiedStatusIDs(ctx, tx)
		if err != nil {
			return err
		}
		for _, id := range ids {
			deleted, err := statuses.IsDeleted(ctx, tx, id)
			if err != nil {
				return err
			}
			if !deleted {
				continue
			}
			if err := repo.InsertDeletedStatus(ctx, tx, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("rebuild notification_deleted_statuses: %w", err)
	}
	return nil
}
