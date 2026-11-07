// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

var (
	// ErrBoostExists reports a boost whose URI another boost already has.
	ErrBoostExists = errors.New("content: a boost with this URI exists")
	// ErrBoostRetracted reports a second retraction of a boost.
	ErrBoostRetracted = errors.New("content: the boost is already retracted")
	// ErrFavouriteURIUsed reports a favourite whose URI another favourite already has.
	ErrFavouriteURIUsed = errors.New("content: a favourite with this URI exists")
	// ErrReactionURIUsed reports a reaction whose URI another reaction already has.
	ErrReactionURIUsed = errors.New("content: a reaction with this URI exists")
)

// InteractionRepository is the persistence port of the interactions of accounts with statuses
// (data-model §4.4): boosts with their retractions and the boost_edges projection, and the
// current values favourites, bookmarks, and status_pins. The database adapters implement it and
// pass testkit.InteractionRepositoryContract; testkit.InteractionFlows (internal/flowtest/content) drives the
// Interactions use cases on them.
//
// Methods that take a uow.Tx run inside that transaction; the others read committed data outside
// any transaction. Lookups that find nothing return an error wrapping ErrNotFound.
type InteractionRepository interface {
	// Boosts: a resource and its retraction event, written by commands.

	// InsertBoost inserts a boost. A URI that another boost has fails with ErrBoostExists.
	InsertBoost(ctx context.Context, tx uow.Tx, b content.Boost) error
	// InsertBoostRetraction records the retraction of a boost. A boost that is already
	// retracted fails with ErrBoostRetracted.
	InsertBoostRetraction(ctx context.Context, tx uow.Tx, r content.BoostRetraction) error
	// Boost returns a boost.
	Boost(ctx context.Context, tx uow.Tx, id string) (content.Boost, error)

	// boost_edges, written only by this module's projectors and rebuild.

	// BoostEdge returns the live boost of a status by an account.
	BoostEdge(ctx context.Context, tx uow.Tx, boosterAccountID, statusID string) (content.BoostEdge, error)
	// InsertBoostEdge inserts e and reports whether it did; false means the pair already has a
	// live boost.
	InsertBoostEdge(ctx context.Context, tx uow.Tx, e content.BoostEdge) (bool, error)
	// DeleteBoostEdge removes the edge of boost e.BoostID and reports whether there was one.
	DeleteBoostEdge(ctx context.Context, tx uow.Tx, e content.BoostEdge) (bool, error)
	// DeleteStatusBoostEdges removes the edges of every boost of a status.
	DeleteStatusBoostEdges(ctx context.Context, tx uow.Tx, statusID string) error
	// RebuildBoostEdges empties boost_edges and derives it again from the boosts that are not
	// retracted, of statuses that are not deleted (data-model §9).
	RebuildBoostEdges(ctx context.Context, tx uow.Tx) error
	// ListBoostEdges returns boost_edges ordered by booster and status, for verifying it.
	ListBoostEdges(ctx context.Context) ([]content.BoostEdge, error)
	// ListLiveBoosts returns the boosts that boost_edges holds, ordered by ID, as a source of
	// other modules' projections (ReplayEvents).
	ListLiveBoosts(ctx context.Context, tx uow.Tx) ([]content.Boost, error)
	// ListLocalStatusBoosters returns the local accounts that boost a status in boost_edges,
	// ordered by ID.
	ListLocalStatusBoosters(ctx context.Context, tx uow.Tx, statusID string) ([]string, error)

	// status_counters, shared with the status projectors (Repository.RebuildCounters counts
	// the boosts and favourites by the same rules).

	// AddBoosts adds delta to the boosts count of a status that has counters; a status without
	// counters (a deleted one) is left alone.
	AddBoosts(ctx context.Context, tx uow.Tx, statusID string, delta int64) error
	// AddFavourites adds delta to the favourites count of a status that has counters.
	AddFavourites(ctx context.Context, tx uow.Tx, statusID string, delta int64) error
	// AddLocalPosts adds delta to the local posts of an account (local_post_counters, shared
	// with the status projectors); the counter goes away when it reaches zero.
	AddLocalPosts(ctx context.Context, tx uow.Tx, accountID string, delta int64) error

	// Mentioned reports whether the current revision of a status mentions an account
	// (status_mention_index), for the visibility check.
	Mentioned(ctx context.Context, statusID, accountID string) (bool, error)

	// Current values. Each insert does nothing when the account already has the row for the
	// status and reports whether it inserted; each delete reports whether there was a row.
	// Only a row actually inserted or deleted changes a counter (data-model §2.3).

	// InsertFavourite inserts f. A URI that another favourite has fails with
	// ErrFavouriteURIUsed.
	InsertFavourite(ctx context.Context, tx uow.Tx, f content.Favourite) (bool, error)
	// DeleteFavourite removes the favourite of a status by an account and returns it.
	DeleteFavourite(ctx context.Context, tx uow.Tx, accountID, statusID string) (content.Favourite, bool, error)
	InsertBookmark(ctx context.Context, tx uow.Tx, b content.Bookmark) (bool, error)
	DeleteBookmark(ctx context.Context, tx uow.Tx, accountID, statusID string) (bool, error)
	// InsertPin stores p in the lowest of the account's slots 1 to content.MaxPins that is free;
	// the unique slot per account keeps concurrent pins within the limit. It stores nothing and
	// returns false when the account already pins the status, when every slot is taken, or when
	// a concurrent transaction takes the free slot first.
	InsertPin(ctx context.Context, tx uow.Tx, p content.Pin) (bool, error)
	DeletePin(ctx context.Context, tx uow.Tx, accountID, statusID string) (bool, error)
	// DeletePinsOf removes every pin of an account and returns how many it removed.
	DeletePinsOf(ctx context.Context, tx uow.Tx, accountID string) (int64, error)
	// HasPin reports whether an account pins a status.
	HasPin(ctx context.Context, tx uow.Tx, accountID, statusID string) (bool, error)
	// CountPins returns the number of statuses an account pins.
	CountPins(ctx context.Context, tx uow.Tx, accountID string) (int64, error)

	// Lookups.

	// Emoji reactions (ADR-0051): a resource deleted when it is taken back, and the counts per
	// emoji of each status that is not deleted (status_reaction_counts), written only by this
	// module's projectors and rebuild.

	// InsertReaction inserts r and reports whether it did; false means the account already
	// reacts to the status with the emoji. A URI that another reaction has fails with
	// ErrReactionURIUsed.
	InsertReaction(ctx context.Context, tx uow.Tx, r content.Reaction) (bool, error)
	// DeleteReaction removes the reaction id and returns it, reporting whether there was one.
	DeleteReaction(ctx context.Context, tx uow.Tx, id string) (content.Reaction, bool, error)
	// DeleteAccountStatusReactions removes the reactions of an account to a status, except the
	// one with the emoji whose key is exceptKey ("" removes them all), and returns them.
	DeleteAccountStatusReactions(ctx context.Context, tx uow.Tx, accountID, statusID, exceptKey string) ([]content.Reaction, error)
	// AddReactions adds delta (1 or -1) to the count of the reactions to a status with an emoji.
	// A status that is deleted gains no count; a count that reaches zero goes away.
	AddReactions(ctx context.Context, tx uow.Tx, statusID string, emoji content.ReactionEmoji, delta int64) error
	// DeleteStatusReactionCounts removes the counts of a status.
	DeleteStatusReactionCounts(ctx context.Context, tx uow.Tx, statusID string) error
	// RebuildReactionCounts empties status_reaction_counts and derives it again from the
	// reactions to statuses that are not deleted (data-model §9).
	RebuildReactionCounts(ctx context.Context, tx uow.Tx) error
	// FindReaction returns the reaction of an account to a status with the emoji whose key
	// (content.ReactionEmoji.Key) is emojiKey.
	FindReaction(ctx context.Context, accountID, statusID, emojiKey string) (content.Reaction, error)
	// FindReactionByURI returns the reaction whose Like or EmojiReact is uri.
	FindReactionByURI(ctx context.Context, uri string) (content.Reaction, error)
	// ListAccountStatusReactions returns the reactions of an account to a status, oldest first.
	ListAccountStatusReactions(ctx context.Context, accountID, statusID string) ([]content.Reaction, error)
	// ListStatusReactions returns the reactions to a status newest first, only those with the
	// emoji whose key is emojiKey unless it is "": those with an ID below before ("" starts at
	// the newest), at most limit of them.
	ListStatusReactions(ctx context.Context, statusID, emojiKey, before string, limit int) ([]content.Reaction, error)
	// ListReactionCounts returns the counts of the reactions to a status, the largest first
	// (ties by key), at most limit of them.
	ListReactionCounts(ctx context.Context, statusID string, limit int) ([]content.ReactionCount, error)
	// ListAllReactionCounts returns status_reaction_counts ordered by status and key, for
	// verifying it.
	ListAllReactionCounts(ctx context.Context) ([]content.ReactionCount, error)

	FindBoost(ctx context.Context, id string) (content.Boost, error)
	FindBoostByURI(ctx context.Context, uri string) (content.Boost, error)
	FindBoostRetraction(ctx context.Context, boostID string) (content.BoostRetraction, error)
	// FindBoostRetractionTx is FindBoostRetraction inside tx.
	FindBoostRetractionTx(ctx context.Context, tx uow.Tx, boostID string) (content.BoostRetraction, error)
	FindBoostEdge(ctx context.Context, boosterAccountID, statusID string) (content.BoostEdge, error)
	FindFavourite(ctx context.Context, accountID, statusID string) (content.Favourite, error)
	// FindFavouriteByURI returns the favourite whose Like is uri.
	FindFavouriteByURI(ctx context.Context, uri string) (content.Favourite, error)
	FindBookmark(ctx context.Context, accountID, statusID string) (content.Bookmark, error)
	FindPin(ctx context.Context, accountID, statusID string) (content.Pin, error)
	// ListFavourites, ListBookmarks, and ListPins return an account's rows newest first:
	// those with an ID below before ("" starts at the newest), at most limit of them.
	ListFavourites(ctx context.Context, accountID, before string, limit int) ([]content.Favourite, error)
	ListBookmarks(ctx context.Context, accountID, before string, limit int) ([]content.Bookmark, error)
	ListPins(ctx context.Context, accountID string) ([]content.Pin, error)
}
