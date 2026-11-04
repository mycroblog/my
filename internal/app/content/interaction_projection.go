// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

var errBoostEdgeExists = errors.New("content: the account already boosts the status")

// RegisterInteractionProjectors registers the synchronous projectors of the interactions with
// statuses (data-model §9). Register them after RegisterProjectors, which creates the counters
// of a status.
//   - boost_edges holds each boost that is not retracted, of a status that is not deleted;
//   - status_counters.boosts_count counts those boosts and favourites_count the favourites of
//     the status. Both change only when an event says a row was actually written;
//   - local_post_counters counts, besides the statuses that RegisterProjectors counts, each
//     local account's boosts in boost_edges.
//   - status_reaction_counts counts the reactions to each status that is not deleted, per emoji
//     (ADR-0051).
//
// The rebuild queries state the same rules in SQL (InteractionRepository.RebuildBoostEdges,
// InteractionRepository.RebuildReactionCounts, and Repository.RebuildCounters); the property test in testkit.InteractionRepositoryContract
// checks that both agree.
func RegisterInteractionProjectors(r *uow.Projectors, repo InteractionRepository) {
	r.Register(EventBoosted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		b := e.(Boosted).Boost
		inserted, err := repo.InsertBoostEdge(ctx, tx, content.EdgeOf(b))
		if err != nil {
			return err
		}
		if !inserted {
			// A concurrent boost of the same pair won; a new attempt finds its edge.
			return uow.MarkRetryable(errBoostEdgeExists)
		}
		if err := repo.AddBoosts(ctx, tx, b.StatusID, 1); err != nil {
			return err
		}
		return addLocalBoost(ctx, tx, repo, b, 1)
	})
	r.Register(EventBoostRetracted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		b := e.(BoostRetracted).Boost
		deleted, err := repo.DeleteBoostEdge(ctx, tx, content.EdgeOf(b))
		if err != nil || !deleted {
			return err
		}
		if err := repo.AddBoosts(ctx, tx, b.StatusID, -1); err != nil {
			return err
		}
		return addLocalBoost(ctx, tx, repo, b, -1)
	})
	r.Register(EventFavourited, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.AddFavourites(ctx, tx, e.(Favourited).Favourite.StatusID, 1)
	})
	r.Register(EventUnfavourited, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.AddFavourites(ctx, tx, e.(Unfavourited).Favourite.StatusID, -1)
	})
	r.Register(EventReacted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		re := e.(Reacted).Reaction
		return repo.AddReactions(ctx, tx, re.StatusID, re.Emoji, 1)
	})
	r.Register(EventUnreacted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		re := e.(Unreacted).Reaction
		return repo.AddReactions(ctx, tx, re.StatusID, re.Emoji, -1)
	})
	// A deleted status leaves every status projection; its counters go with RegisterProjectors.
	// Its local boosts no longer count as their boosters' posts.
	r.Register(EventStatusDeleted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		statusID := e.(StatusDeleted).Status.ID
		if err := repo.DeleteStatusReactionCounts(ctx, tx, statusID); err != nil {
			return err
		}
		boosters, err := repo.ListLocalStatusBoosters(ctx, tx, statusID)
		if err != nil {
			return err
		}
		for _, id := range boosters {
			if err := repo.AddLocalPosts(ctx, tx, id, -1); err != nil {
				return err
			}
		}
		return repo.DeleteStatusBoostEdges(ctx, tx, statusID)
	})
}

// addLocalBoost adds delta to the local posts of the booster of b when b CountsAsLocalPost.
func addLocalBoost(ctx context.Context, tx uow.Tx, repo InteractionRepository, b content.Boost, delta int64) error {
	if !b.CountsAsLocalPost() {
		return nil
	}
	return repo.AddLocalPosts(ctx, tx, b.BoosterAccountID, delta)
}

// RebuildInteractionProjections empties boost_edges and status_reaction_counts and derives them
// again from the boosts and their retractions and from the reactions (data-model §9). The
// boosts and favourites counts are rebuilt with the other status counters by
// RebuildProjections.
func RebuildInteractionProjections(ctx context.Context, u uow.UnitOfWork, repo InteractionRepository) error {
	if err := u.Tx(ctx, func(tx uow.Tx) error { return repo.RebuildBoostEdges(ctx, tx) }); err != nil {
		return fmt.Errorf("rebuild boost_edges: %w", err)
	}
	if err := u.Tx(ctx, func(tx uow.Tx) error { return repo.RebuildReactionCounts(ctx, tx) }); err != nil {
		return fmt.Errorf("rebuild status_reaction_counts: %w", err)
	}
	return nil
}
