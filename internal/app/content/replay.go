// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// IsDeleted reports inside tx whether a status has a deletion, for rebuilding other modules'
// projections of deletions (notification_deleted_statuses, data-model §9).
func (s *Service) IsDeleted(ctx context.Context, tx uow.Tx, statusID string) (bool, error) {
	return s.d.Repository.IsDeleted(ctx, tx, statusID)
}

// ReplayEvents passes to emit, inside tx, the events that derive other modules' projections of
// statuses and boosts from their current state (data-model §9), so that those modules rebuild
// their projections without reading this module's tables: a StatusCreated with the current
// revision for each status that is not deleted, ordered by status ID (a direct one with its
// current conversation), then a Boosted for each live boost, ordered by boost ID. Deleted statuses and retracted boosts emit nothing, so a
// projection that applies these events to empty tables ends where incremental updating did.
// StatusCreated.HeldReply is not replayed: the projections rebuilt from these events (the
// timelines) do not read it.
func ReplayEvents(ctx context.Context, tx uow.Tx, statuses Repository, interactions InteractionRepository, emit func(uow.Event) error) error {
	posts, err := statuses.ListCurrentPosts(ctx, tx)
	if err != nil {
		return err
	}
	// The conversations of the direct posts, which the list of direct messages is keyed by
	// (ADR-0154 §6), read together. The current conversation is the one every merge ended in, so
	// no merge is replayed.
	var direct []string
	for _, p := range posts {
		if p.Status.Visibility == VisibilityDirect {
			direct = append(direct, p.Status.ID)
		}
	}
	var convs map[string]content.Conversation
	if len(direct) > 0 {
		if convs, err = statuses.StatusConversationsTx(ctx, tx, direct); err != nil {
			return err
		}
	}
	live := make(map[string]content.Status, len(posts))
	for _, p := range posts {
		live[p.Status.ID] = p.Status
		if err := emit(StatusCreated{Status: p.Status, Revision: p.Revision, ConversationID: convs[p.Status.ID].ID}); err != nil {
			return err
		}
	}
	boosts, err := interactions.ListLiveBoosts(ctx, tx)
	if err != nil {
		return err
	}
	for _, b := range boosts {
		s, ok := live[b.StatusID]
		if !ok {
			return fmt.Errorf("content: live boost %s of status %s, which is deleted or missing", b.ID, b.StatusID)
		}
		if err := emit(Boosted{Boost: b, Status: RefOf(s)}); err != nil {
			return err
		}
	}
	return nil
}
