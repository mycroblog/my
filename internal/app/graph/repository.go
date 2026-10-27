// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

var (
	// ErrNotFound reports that no row matches a lookup, or that a command finds no follow or
	// follow request to act on.
	ErrNotFound = errors.New("graph: not found")
	// ErrAlreadyRecorded reports a second acceptance, rejection, or termination of one follow, or
	// a second retraction of one block.
	ErrAlreadyRecorded = errors.New("graph: the follow or block already has this event")
)

// Repository is the persistence port of the graph module: follows, blocks, mutes, and domain
// mutes
// (data-model §4.5). The database adapters implement it and pass testkit.FollowRepositoryContract
// and testkit.BlockRepositoryContract.
//
// Methods that take a uow.Tx run inside that transaction; the others read committed data outside
// any transaction. Lookups that find nothing return an error wrapping ErrNotFound.
type Repository interface {
	// Resources, events, and current values, written by commands.

	InsertFollow(ctx context.Context, tx uow.Tx, f graph.Follow) error
	// InsertAcceptance, InsertRejection, and InsertTermination fail with ErrAlreadyRecorded when
	// the follow already has an event of the same kind.
	InsertAcceptance(ctx context.Context, tx uow.Tx, a graph.Acceptance) error
	InsertRejection(ctx context.Context, tx uow.Tx, r graph.Rejection) error
	InsertTermination(ctx context.Context, tx uow.Tx, t graph.Termination) error
	UpsertPreferences(ctx context.Context, tx uow.Tx, p graph.Preferences) error
	// Follow returns a follow by ID.
	Follow(ctx context.Context, tx uow.Tx, id string) (graph.Follow, error)
	// FollowByURI returns the follow whose Follow activity has the IRI uri.
	FollowByURI(ctx context.Context, tx uow.Tx, uri string) (graph.Follow, error)
	// InsertResend records a Follow received again for an active follow, and reports false,
	// writing nothing, when its URI is already recorded.
	InsertResend(ctx context.Context, tx uow.Tx, r graph.Resend) (bool, error)
	// FollowByResendURI returns the follow that a Follow received again with the IRI uri names.
	FollowByResendURI(ctx context.Context, tx uow.Tx, uri string) (graph.Follow, error)
	// Preferences returns the preferences of a follow.
	Preferences(ctx context.Context, tx uow.Tx, followID string) (graph.Preferences, error)
	// LockPair serializes the transactions that follow or block between accountA and accountB
	// (in either order) until tx ends, so that a follow and a block of one pair never both
	// commit having missed each other. Adapters whose write transactions already serialize may
	// do nothing.
	LockPair(ctx context.Context, tx uow.Tx, accountA, accountB string) error
	InsertBlock(ctx context.Context, tx uow.Tx, b graph.Block) error
	// InsertBlockRetraction fails with ErrAlreadyRecorded when the block is already retracted.
	InsertBlockRetraction(ctx context.Context, tx uow.Tx, r graph.BlockRetraction) error
	// Block returns a block by ID.
	Block(ctx context.Context, tx uow.Tx, id string) (graph.Block, error)
	// BlockByURI returns the block with an activity URI and its retraction, if any.
	BlockByURI(ctx context.Context, tx uow.Tx, uri string) (graph.BlockHistory, error)
	UpsertMute(ctx context.Context, tx uow.Tx, m graph.Mute) error
	// DeleteMute deletes accountID's mute of targetAccountID and reports whether there was one.
	DeleteMute(ctx context.Context, tx uow.Tx, accountID, targetAccountID string) (bool, error)
	// DeleteExpiredMutes deletes at most limit mutes that have expired at now and returns them.
	DeleteExpiredMutes(ctx context.Context, tx uow.Tx, now time.Time, limit int) ([]graph.Mute, error)
	// Mute is FindMute inside tx.
	Mute(ctx context.Context, tx uow.Tx, accountID, targetAccountID string, now time.Time) (graph.Mute, error)
	// InsertDomainMute inserts a domain mute and reports false, writing nothing, when the
	// account already mutes the domain.
	InsertDomainMute(ctx context.Context, tx uow.Tx, m graph.DomainMute) (bool, error)
	// DeleteDomainMute deletes accountID's mute of domain and reports whether there was one.
	DeleteDomainMute(ctx context.Context, tx uow.Tx, accountID, domain string) (bool, error)
	// CountDomainMutes returns how many domains accountID mutes.
	CountDomainMutes(ctx context.Context, tx uow.Tx, accountID string) (int, error)

	// Projections, written only by this module's projectors and rebuild.

	// Edge returns the edge from follower to followee.
	Edge(ctx context.Context, tx uow.Tx, followerAccountID, followeeAccountID string) (graph.Edge, error)
	// ListEdgesOf returns the edges into and out of an account, ordered by follower and followee.
	ListEdgesOf(ctx context.Context, tx uow.Tx, accountID string) ([]graph.Edge, error)
	// InsertEdge inserts an edge and reports false, writing nothing, when the pair already has one.
	InsertEdge(ctx context.Context, tx uow.Tx, e graph.Edge) (bool, error)
	// UpdateEdge replaces the state and since of the pair's edge when it belongs to e.FollowID
	// and is in state from, and reports whether it did.
	UpdateEdge(ctx context.Context, tx uow.Tx, e graph.Edge, from graph.State) (bool, error)
	// DeleteEdge deletes the pair's edge when it belongs to followID and is in state, and
	// reports whether it did.
	DeleteEdge(ctx context.Context, tx uow.Tx, followerAccountID, followeeAccountID, followID string, state graph.State) (bool, error)
	DeleteAllEdges(ctx context.Context, tx uow.Tx) error
	// AddCounters adds the deltas to an account's counters, creating the row from zero, and
	// deletes the row when both counters reach zero: no row means zero.
	AddCounters(ctx context.Context, tx uow.Tx, accountID string, followersDelta, followingDelta int64) error
	UpsertCounters(ctx context.Context, tx uow.Tx, c graph.Counters) error
	DeleteAllCounters(ctx context.Context, tx uow.Tx) error
	// BlockEdge returns the edge from blocker to blocked.
	BlockEdge(ctx context.Context, tx uow.Tx, blockerAccountID, blockedAccountID string) (graph.BlockEdge, error)
	// InsertBlockEdge inserts an edge and reports false, writing nothing, when the pair already has one.
	InsertBlockEdge(ctx context.Context, tx uow.Tx, e graph.BlockEdge) (bool, error)
	// DeleteBlockEdge deletes the pair's edge when it belongs to blockID.
	DeleteBlockEdge(ctx context.Context, tx uow.Tx, blockerAccountID, blockedAccountID, blockID string) error
	DeleteAllBlockEdges(ctx context.Context, tx uow.Tx) error

	// Projection sources, in a stable order, for rebuilding the projections.

	// ListHistories returns every follow with its acceptance, rejection, and termination,
	// ordered by follow ID.
	ListHistories(ctx context.Context, tx uow.Tx) ([]graph.History, error)
	// ListBlockHistories returns every block with its retraction, ordered by block ID.
	ListBlockHistories(ctx context.Context, tx uow.Tx) ([]graph.BlockHistory, error)
	// ListAllMutes returns every mute, expired ones included, ordered by account and target, as
	// a source of other modules' projections (ReplayEvents).
	ListAllMutes(ctx context.Context, tx uow.Tx) ([]graph.Mute, error)
	// ListAllDomainMutes returns every domain mute, ordered by account and domain, as a source
	// of other modules' projections (ReplayEvents).
	ListAllDomainMutes(ctx context.Context, tx uow.Tx) ([]graph.DomainMute, error)

	// Projection contents, for verifying the projections.

	// ListEdges returns every edge, ordered by follower and followee.
	ListEdges(ctx context.Context) ([]graph.Edge, error)
	// ListCounters returns every counters row, ordered by account ID.
	ListCounters(ctx context.Context) ([]graph.Counters, error)
	// ListBlockEdges returns every block edge, ordered by blocker and blocked account.
	ListBlockEdges(ctx context.Context) ([]graph.BlockEdge, error)

	// Lookups.

	// FindFollow returns a follow by ID.
	FindFollow(ctx context.Context, id string) (graph.Follow, error)
	FindEdge(ctx context.Context, followerAccountID, followeeAccountID string) (graph.Edge, error)
	FindPreferences(ctx context.Context, followID string) (graph.Preferences, error)
	// FindCounters returns an account's counters; ErrNotFound means both are zero.
	FindCounters(ctx context.Context, accountID string) (graph.Counters, error)
	// ListFollowers returns the edges into followeeAccountID in state, newest follow first,
	// starting after the follow ID before ("" for the newest) and returning at most limit edges.
	ListFollowers(ctx context.Context, followeeAccountID string, state graph.State, before string, limit int) ([]graph.Edge, error)
	// ListFollowing is ListFollowers for the edges out of followerAccountID.
	ListFollowing(ctx context.Context, followerAccountID string, state graph.State, before string, limit int) ([]graph.Edge, error)
	FindBlockEdge(ctx context.Context, blockerAccountID, blockedAccountID string) (graph.BlockEdge, error)
	// FindBlock returns a block by ID, retracted or not.
	FindBlock(ctx context.Context, id string) (graph.Block, error)
	// ListBlocking returns the block edges out of blockerAccountID, newest block first, starting
	// after the block ID before ("" for the newest) and returning at most limit edges.
	ListBlocking(ctx context.Context, blockerAccountID, before string, limit int) ([]graph.BlockEdge, error)
	// ListBlockers returns the block edges into blockedAccountID in block ID order, starting
	// after the block ID after ("" for the first) and returning at most limit edges.
	ListBlockers(ctx context.Context, blockedAccountID, after string, limit int) ([]graph.BlockEdge, error)
	// ListMuters returns the mutes of targetAccountID that have not expired at now, in the order
	// of the muting accounts, starting after the account ID after ("" for the first) and
	// returning at most limit mutes.
	ListMuters(ctx context.Context, targetAccountID string, now time.Time, after string, limit int) ([]graph.Mute, error)
	// FindMute returns accountID's mute of targetAccountID; ErrNotFound means none, or one that
	// has expired at now.
	FindMute(ctx context.Context, accountID, targetAccountID string, now time.Time) (graph.Mute, error)
	// ListMutes returns accountID's mutes that have not expired at now, newest first by creation
	// and then target account ID, starting after before (the zero cursor for the newest) and
	// returning at most limit mutes.
	ListMutes(ctx context.Context, accountID string, now time.Time, before MuteCursor, limit int) ([]graph.Mute, error)
	// ListDomainMutes returns accountID's domain mutes in the order of their domains, starting
	// after the domain after ("" for the first) and returning at most limit mutes.
	ListDomainMutes(ctx context.Context, accountID, after string, limit int) ([]graph.DomainMute, error)
	// MutesAnyDomain reports whether accountID mutes one of domains.
	MutesAnyDomain(ctx context.Context, accountID string, domains []string) (bool, error)
	// RelationsAmong returns the relations of accountID with the accounts among ids, with the
	// mutes that have not expired at now.
	RelationsAmong(ctx context.Context, accountID string, ids []string, now time.Time) (RelationsAmong, error)
}

// RelationsAmong are the relations of one account with some others, each list in ID order: the
// accounts it blocks, those that block it, those it mutes, those it actively follows, and the
// counters of those that have them.
type RelationsAmong struct {
	Blocked, BlockedBy, Muted, Following []string
	Counters                             []graph.Counters
}

// MuteCursor is the position of a mute in a list of mutes: its creation and target account.
// The zero cursor is the start of the list.
type MuteCursor struct {
	CreatedAt       time.Time
	TargetAccountID string
}

// MuteCursorOf returns the position of m, to continue a list after it.
func MuteCursorOf(m graph.Mute) MuteCursor {
	return MuteCursor{CreatedAt: m.CreatedAt, TargetAccountID: m.TargetAccountID}
}
