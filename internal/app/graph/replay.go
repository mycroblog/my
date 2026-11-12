// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// ReplayEvents passes to emit, inside tx, the events that derive other modules' projections of
// follows, blocks, and mutes from their current state, so that those modules
// rebuild their projections without reading this module's tables: for each live follow ordered
// by follow ID, a Followed when it is active or a FollowRequested when it is pending, with its
// current preferences; then a Blocked for each block in force, ordered by block ID; then a Muted
// for each mute, expired ones included, ordered by account and target; then a DomainMuted for
// each domain mute, ordered by account and domain. Rejected and terminated
// follows and retracted blocks emit nothing, so a projection that applies these events to empty
// tables ends where incremental updating did.
func ReplayEvents(ctx context.Context, tx uow.Tx, repo Repository, emit func(uow.Event) error) error {
	histories, err := repo.ListHistories(ctx, tx)
	if err != nil {
		return err
	}
	for _, h := range histories {
		edge, ok := h.Edge()
		if !ok {
			continue
		}
		prefs, err := repo.Preferences(ctx, tx, h.Follow.ID)
		if err != nil {
			return err
		}
		var e uow.Event = FollowRequested{Follow: h.Follow, Preferences: prefs}
		if edge.State == graph.StateActive {
			e = Followed{Follow: h.Follow, Acceptance: *h.Acceptance, Preferences: prefs}
		}
		if err := emit(e); err != nil {
			return err
		}
	}
	blocks, err := repo.ListBlockHistories(ctx, tx)
	if err != nil {
		return err
	}
	for _, h := range blocks {
		if _, ok := h.Edge(); !ok {
			continue
		}
		if err := emit(Blocked{Block: h.Block}); err != nil {
			return err
		}
	}
	mutes, err := repo.ListAllMutes(ctx, tx)
	if err != nil {
		return err
	}
	for _, m := range mutes {
		if err := emit(Muted{Mute: m}); err != nil {
			return err
		}
	}
	domains, err := repo.ListAllDomainMutes(ctx, tx)
	if err != nil {
		return err
	}
	for _, m := range domains {
		if err := emit(DomainMuted{Mute: m}); err != nil {
			return err
		}
	}
	return nil
}
