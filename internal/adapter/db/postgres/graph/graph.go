// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/graphgen"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// Repository implements graph.Repository on PostgreSQL.
type Repository struct {
	db *moduleDB
}

var _ appgraph.Repository = (*Repository)(nil)

// NewRepository returns the graph module's repository on d.
func NewRepository(d *postgres.DB) *Repository { return &Repository{db: &moduleDB{d}} }

// InsertFollow implements graph.Repository.
func (r *Repository) InsertFollow(ctx context.Context, tx uow.Tx, f graph.Follow) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertFollow(ctx, graphgen.InsertFollowParams{
		ID: f.ID, Uri: f.URI, FollowerAccountID: f.FollowerAccountID, FolloweeAccountID: f.FolloweeAccountID,
		RequestedAt: f.RequestedAt,
	}), "insert follow")
}

// alreadyRecorded maps a primary key violation of table (keyed by follow_id) to graph.ErrAlreadyRecorded.
func alreadyRecorded(err error, table, followID string) error {
	if postgres.IsUniqueViolation(err, table+"_pkey") {
		return fmt.Errorf("%s of follow %s: %w", table, followID, appgraph.ErrAlreadyRecorded)
	}
	return postgres.Wrap(err, "insert "+table)
}

// InsertAcceptance implements graph.Repository.
func (r *Repository) InsertAcceptance(ctx context.Context, tx uow.Tx, a graph.Acceptance) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertFollowAcceptance(ctx, graphgen.InsertFollowAcceptanceParams{
		FollowID: a.FollowID, AcceptedBy: string(a.AcceptedBy), ActivityUri: postgres.ToNullString(a.ActivityURI),
		AcceptedAt: a.AcceptedAt,
	})
	return alreadyRecorded(err, "follow_acceptances", a.FollowID)
}

// InsertRejection implements graph.Repository.
func (r *Repository) InsertRejection(ctx context.Context, tx uow.Tx, rej graph.Rejection) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertFollowRejection(ctx, graphgen.InsertFollowRejectionParams{
		FollowID: rej.FollowID, RejectedBy: string(rej.RejectedBy), ActivityUri: postgres.ToNullString(rej.ActivityURI),
		RejectedAt: rej.RejectedAt,
	})
	return alreadyRecorded(err, "follow_rejections", rej.FollowID)
}

// InsertTermination implements graph.Repository.
func (r *Repository) InsertTermination(ctx context.Context, tx uow.Tx, t graph.Termination) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertFollowTermination(ctx, graphgen.InsertFollowTerminationParams{
		FollowID: t.FollowID, Reason: string(t.Reason), TerminatedAt: t.TerminatedAt,
	})
	return alreadyRecorded(err, "follow_terminations", t.FollowID)
}

// UpsertPreferences implements graph.Repository.
func (r *Repository) UpsertPreferences(ctx context.Context, tx uow.Tx, p graph.Preferences) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.UpsertFollowPreferences(ctx, graphgen.UpsertFollowPreferencesParams{
		FollowID: p.FollowID, ShowBoosts: p.ShowBoosts,
	}), "upsert follow preferences")
}

func followFrom(row graphgen.Follow) graph.Follow {
	return graph.Follow{
		ID: row.ID, URI: row.Uri, FollowerAccountID: row.FollowerAccountID, FolloweeAccountID: row.FolloweeAccountID,
		RequestedAt: postgres.UTC(row.RequestedAt),
	}
}

// Follow implements graph.Repository.
func (r *Repository) Follow(ctx context.Context, tx uow.Tx, id string) (graph.Follow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return graph.Follow{}, err
	}
	row, err := q.GetFollow(ctx, id)
	if err != nil {
		return graph.Follow{}, postgres.NotFound(err, appgraph.ErrNotFound, "follow "+id)
	}
	return followFrom(row), nil
}

// FollowByURI implements graph.Repository.
func (r *Repository) FollowByURI(ctx context.Context, tx uow.Tx, uri string) (graph.Follow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return graph.Follow{}, err
	}
	row, err := q.GetFollowByURI(ctx, uri)
	if err != nil {
		return graph.Follow{}, postgres.NotFound(err, appgraph.ErrNotFound, "follow "+uri)
	}
	return followFrom(row), nil
}

// FindFollow implements graph.Repository.
func (r *Repository) FindFollow(ctx context.Context, id string) (graph.Follow, error) {
	row, err := r.db.reader().GetFollow(ctx, id)
	if err != nil {
		return graph.Follow{}, postgres.NotFound(err, appgraph.ErrNotFound, "follow "+id)
	}
	return followFrom(row), nil
}

func preferencesFrom(row graphgen.FollowPreference) graph.Preferences {
	return graph.Preferences{FollowID: row.FollowID, ShowBoosts: row.ShowBoosts}
}

// Preferences implements graph.Repository.
func (r *Repository) Preferences(ctx context.Context, tx uow.Tx, followID string) (graph.Preferences, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return graph.Preferences{}, err
	}
	return findPreferences(ctx, q, followID)
}

// FindPreferences implements graph.Repository.
func (r *Repository) FindPreferences(ctx context.Context, followID string) (graph.Preferences, error) {
	return findPreferences(ctx, r.db.reader(), followID)
}

func findPreferences(ctx context.Context, q *graphgen.Queries, followID string) (graph.Preferences, error) {
	row, err := q.GetFollowPreferences(ctx, followID)
	if err != nil {
		return graph.Preferences{}, postgres.NotFound(err, appgraph.ErrNotFound, "follow preferences "+followID)
	}
	return preferencesFrom(row), nil
}

func edgeFrom(row graphgen.FollowEdge) graph.Edge {
	return graph.Edge{
		FollowerAccountID: row.FollowerAccountID, FolloweeAccountID: row.FolloweeAccountID, FollowID: row.FollowID,
		State: graph.State(row.State), Since: postgres.UTC(row.Since),
	}
}

func edgesFrom(rows []graphgen.FollowEdge) []graph.Edge {
	out := make([]graph.Edge, len(rows))
	for i, row := range rows {
		out[i] = edgeFrom(row)
	}
	return out
}

// Edge implements graph.Repository.
func (r *Repository) Edge(ctx context.Context, tx uow.Tx, followerAccountID, followeeAccountID string) (graph.Edge, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return graph.Edge{}, err
	}
	return findEdge(ctx, q, followerAccountID, followeeAccountID)
}

// ListEdgesOf implements graph.Repository.
func (r *Repository) ListEdgesOf(ctx context.Context, tx uow.Tx, accountID string) ([]graph.Edge, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListFollowEdgesOf(ctx, accountID)
	if err != nil {
		return nil, postgres.Wrap(err, "list follow edges of "+accountID)
	}
	return edgesFrom(rows), nil
}

// FindEdge implements graph.Repository.
func (r *Repository) FindEdge(ctx context.Context, followerAccountID, followeeAccountID string) (graph.Edge, error) {
	return findEdge(ctx, r.db.reader(), followerAccountID, followeeAccountID)
}

func findEdge(ctx context.Context, q *graphgen.Queries, followerAccountID, followeeAccountID string) (graph.Edge, error) {
	row, err := q.GetFollowEdge(ctx, graphgen.GetFollowEdgeParams{
		FollowerAccountID: followerAccountID, FolloweeAccountID: followeeAccountID,
	})
	if err != nil {
		return graph.Edge{}, postgres.NotFound(err, appgraph.ErrNotFound, "follow edge "+followerAccountID+" -> "+followeeAccountID)
	}
	return edgeFrom(row), nil
}

// InsertEdge implements graph.Repository.
func (r *Repository) InsertEdge(ctx context.Context, tx uow.Tx, e graph.Edge) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertFollowEdge(ctx, graphgen.InsertFollowEdgeParams{
		FollowerAccountID: e.FollowerAccountID, FolloweeAccountID: e.FolloweeAccountID, FollowID: e.FollowID,
		State: string(e.State), Since: e.Since,
	})
	if err != nil {
		return false, postgres.Wrap(err, "insert follow edge")
	}
	return n == 1, nil
}

// UpdateEdge implements graph.Repository.
func (r *Repository) UpdateEdge(ctx context.Context, tx uow.Tx, e graph.Edge, from graph.State) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.UpdateFollowEdge(ctx, graphgen.UpdateFollowEdgeParams{
		FollowerAccountID: e.FollowerAccountID, FolloweeAccountID: e.FolloweeAccountID, FollowID: e.FollowID,
		State: string(e.State), Since: e.Since, FromState: string(from),
	})
	if err != nil {
		return false, postgres.Wrap(err, "update follow edge")
	}
	return n == 1, nil
}

// DeleteEdge implements graph.Repository.
func (r *Repository) DeleteEdge(ctx context.Context, tx uow.Tx, followerAccountID, followeeAccountID, followID string, state graph.State) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteFollowEdge(ctx, graphgen.DeleteFollowEdgeParams{
		FollowerAccountID: followerAccountID, FolloweeAccountID: followeeAccountID, FollowID: followID, State: string(state),
	})
	if err != nil {
		return false, postgres.Wrap(err, "delete follow edge")
	}
	return n == 1, nil
}

// DeleteAllEdges implements graph.Repository.
func (r *Repository) DeleteAllEdges(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteAllFollowEdges(ctx), "delete follow edges")
}

// AddCounters implements graph.Repository.
func (r *Repository) AddCounters(ctx context.Context, tx uow.Tx, accountID string, followersDelta, followingDelta int64) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.AddToFollowCounters(ctx, graphgen.AddToFollowCountersParams{
		AccountID: accountID, FollowersCount: followersDelta, FollowingCount: followingDelta,
	})
	if err != nil {
		return postgres.Wrap(err, "add to follow counters")
	}
	if n == 0 {
		err = q.AddFollowCounters(ctx, graphgen.AddFollowCountersParams{
			AccountID: accountID, FollowersCount: followersDelta, FollowingCount: followingDelta,
		})
		if err != nil {
			return postgres.Wrap(err, "add follow counters")
		}
	}
	return postgres.Wrap(q.DeleteZeroFollowCounters(ctx, accountID), "delete zero follow counters")
}

// UpsertCounters implements graph.Repository.
func (r *Repository) UpsertCounters(ctx context.Context, tx uow.Tx, c graph.Counters) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.UpsertFollowCounters(ctx, graphgen.UpsertFollowCountersParams(c)), "upsert follow counters")
}

// DeleteAllCounters implements graph.Repository.
func (r *Repository) DeleteAllCounters(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteAllFollowCounters(ctx), "delete follow counters")
}

// ListHistories implements graph.Repository.
func (r *Repository) ListHistories(ctx context.Context, tx uow.Tx) ([]graph.History, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListFollowHistories(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "list follow histories")
	}
	out := make([]graph.History, len(rows))
	for i, row := range rows {
		h := graph.History{Follow: graph.Follow{
			ID: row.ID, URI: row.Uri, FollowerAccountID: row.FollowerAccountID, FolloweeAccountID: row.FolloweeAccountID,
			RequestedAt: postgres.UTC(row.RequestedAt),
		}}
		if row.AcceptedAt != nil {
			h.Acceptance = &graph.Acceptance{
				FollowID: row.ID, AcceptedBy: graph.Acceptor(postgres.FromNullString(row.AcceptedBy)),
				ActivityURI: postgres.FromNullString(row.AcceptanceActivityUri), AcceptedAt: postgres.UTC(*row.AcceptedAt),
			}
		}
		if row.RejectedAt != nil {
			h.Rejection = &graph.Rejection{
				FollowID: row.ID, RejectedBy: graph.Rejector(postgres.FromNullString(row.RejectedBy)),
				ActivityURI: postgres.FromNullString(row.RejectionActivityUri), RejectedAt: postgres.UTC(*row.RejectedAt),
			}
		}
		if row.TerminatedAt != nil {
			h.Termination = &graph.Termination{
				FollowID: row.ID, Reason: graph.TerminationReason(postgres.FromNullString(row.TerminationReason)),
				TerminatedAt: postgres.UTC(*row.TerminatedAt),
			}
		}
		out[i] = h
	}
	return out, nil
}

// ListEdges implements graph.Repository.
func (r *Repository) ListEdges(ctx context.Context) ([]graph.Edge, error) {
	rows, err := r.db.reader().ListFollowEdges(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "list follow edges")
	}
	return edgesFrom(rows), nil
}

// ListCounters implements graph.Repository.
func (r *Repository) ListCounters(ctx context.Context) ([]graph.Counters, error) {
	rows, err := r.db.reader().ListFollowCounters(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "list follow counters")
	}
	out := make([]graph.Counters, len(rows))
	for i, row := range rows {
		out[i] = graph.Counters(row)
	}
	return out, nil
}

// FindCounters implements graph.Repository.
func (r *Repository) FindCounters(ctx context.Context, accountID string) (graph.Counters, error) {
	row, err := r.db.reader().GetFollowCounters(ctx, accountID)
	if err != nil {
		return graph.Counters{}, postgres.NotFound(err, appgraph.ErrNotFound, "follow counters "+accountID)
	}
	return graph.Counters(row), nil
}

// CountFollowers implements graph.Repository.
func (r *Repository) CountFollowers(ctx context.Context, followeeAccountID string, state graph.State) (int64, error) {
	n, err := r.db.reader().CountFollowersInState(ctx, graphgen.CountFollowersInStateParams{AccountID: followeeAccountID, State: string(state)})
	if err != nil {
		return 0, postgres.Wrap(err, "count followers")
	}
	return n, nil
}

// ListFollowers implements graph.Repository.
func (r *Repository) ListFollowers(ctx context.Context, followeeAccountID string, state graph.State, before string, limit int) ([]graph.Edge, error) {
	n, err := postgres.ToInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListFollowersPage(ctx, graphgen.ListFollowersPageParams{
		AccountID: followeeAccountID, State: string(state), Before: postgres.ToNullString(before), PageLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list followers")
	}
	return edgesFrom(rows), nil
}

// ListFollowing implements graph.Repository.
func (r *Repository) ListFollowing(ctx context.Context, followerAccountID string, state graph.State, before string, limit int) ([]graph.Edge, error) {
	n, err := postgres.ToInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListFollowingPage(ctx, graphgen.ListFollowingPageParams{
		AccountID: followerAccountID, State: string(state), Before: postgres.ToNullString(before), PageLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list following")
	}
	return edgesFrom(rows), nil
}
