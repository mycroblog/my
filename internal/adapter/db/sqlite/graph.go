// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// GraphRepository implements graph.Repository on SQLite.
type GraphRepository struct {
	db *DB
}

var _ appgraph.Repository = (*GraphRepository)(nil)

// GraphRepository returns the graph module's repository on d.
func (d *DB) GraphRepository() *GraphRepository { return &GraphRepository{db: d} }

// InsertFollow implements graph.Repository.
func (r *GraphRepository) InsertFollow(ctx context.Context, tx uow.Tx, f graph.Follow) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertFollow(ctx, sqlitegen.InsertFollowParams{
		ID: f.ID, Uri: f.URI, FollowerAccountID: f.FollowerAccountID, FolloweeAccountID: f.FolloweeAccountID,
		RequestedAt: toMillis(f.RequestedAt),
	}), "insert follow")
}

// alreadyRecorded maps a primary key violation of table (keyed by follow_id) to graph.ErrAlreadyRecorded.
func alreadyRecorded(err error, table, followID string) error {
	if isUniqueViolation(err, table+".follow_id") {
		return fmt.Errorf("%s of follow %s: %w", table, followID, appgraph.ErrAlreadyRecorded)
	}
	return wrap(err, "insert "+table)
}

// InsertAcceptance implements graph.Repository.
func (r *GraphRepository) InsertAcceptance(ctx context.Context, tx uow.Tx, a graph.Acceptance) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertFollowAcceptance(ctx, sqlitegen.InsertFollowAcceptanceParams{
		FollowID: a.FollowID, AcceptedBy: string(a.AcceptedBy), ActivityUri: toNullString(a.ActivityURI),
		AcceptedAt: toMillis(a.AcceptedAt),
	})
	return alreadyRecorded(err, "follow_acceptances", a.FollowID)
}

// InsertRejection implements graph.Repository.
func (r *GraphRepository) InsertRejection(ctx context.Context, tx uow.Tx, rej graph.Rejection) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertFollowRejection(ctx, sqlitegen.InsertFollowRejectionParams{
		FollowID: rej.FollowID, RejectedBy: string(rej.RejectedBy), ActivityUri: toNullString(rej.ActivityURI),
		RejectedAt: toMillis(rej.RejectedAt),
	})
	return alreadyRecorded(err, "follow_rejections", rej.FollowID)
}

// InsertTermination implements graph.Repository.
func (r *GraphRepository) InsertTermination(ctx context.Context, tx uow.Tx, t graph.Termination) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertFollowTermination(ctx, sqlitegen.InsertFollowTerminationParams{
		FollowID: t.FollowID, Reason: string(t.Reason), TerminatedAt: toMillis(t.TerminatedAt),
	})
	return alreadyRecorded(err, "follow_terminations", t.FollowID)
}

// UpsertPreferences implements graph.Repository.
func (r *GraphRepository) UpsertPreferences(ctx context.Context, tx uow.Tx, p graph.Preferences) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertFollowPreferences(ctx, sqlitegen.UpsertFollowPreferencesParams{
		FollowID: p.FollowID, ShowBoosts: boolToInt(p.ShowBoosts), Notify: boolToInt(p.Notify),
	}), "upsert follow preferences")
}

func followFrom(row sqlitegen.Follow) graph.Follow {
	return graph.Follow{
		ID: row.ID, URI: row.Uri, FollowerAccountID: row.FollowerAccountID, FolloweeAccountID: row.FolloweeAccountID,
		RequestedAt: fromMillis(row.RequestedAt),
	}
}

// Follow implements graph.Repository.
func (r *GraphRepository) Follow(ctx context.Context, tx uow.Tx, id string) (graph.Follow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return graph.Follow{}, err
	}
	row, err := q.GetFollow(ctx, id)
	if err != nil {
		return graph.Follow{}, notFound(err, appgraph.ErrNotFound, "follow "+id)
	}
	return followFrom(row), nil
}

// FollowByURI implements graph.Repository.
func (r *GraphRepository) FollowByURI(ctx context.Context, tx uow.Tx, uri string) (graph.Follow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return graph.Follow{}, err
	}
	row, err := q.GetFollowByURI(ctx, uri)
	if err != nil {
		return graph.Follow{}, notFound(err, appgraph.ErrNotFound, "follow "+uri)
	}
	return followFrom(row), nil
}

// FindFollow implements graph.Repository.
func (r *GraphRepository) FindFollow(ctx context.Context, id string) (graph.Follow, error) {
	row, err := r.db.reader().GetFollow(ctx, id)
	if err != nil {
		return graph.Follow{}, notFound(err, appgraph.ErrNotFound, "follow "+id)
	}
	return followFrom(row), nil
}

func preferencesFrom(row sqlitegen.FollowPreference) graph.Preferences {
	return graph.Preferences{FollowID: row.FollowID, ShowBoosts: row.ShowBoosts != 0, Notify: row.Notify != 0}
}

// Preferences implements graph.Repository.
func (r *GraphRepository) Preferences(ctx context.Context, tx uow.Tx, followID string) (graph.Preferences, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return graph.Preferences{}, err
	}
	return findPreferences(ctx, q, followID)
}

// FindPreferences implements graph.Repository.
func (r *GraphRepository) FindPreferences(ctx context.Context, followID string) (graph.Preferences, error) {
	return findPreferences(ctx, r.db.reader(), followID)
}

func findPreferences(ctx context.Context, q *sqlitegen.Queries, followID string) (graph.Preferences, error) {
	row, err := q.GetFollowPreferences(ctx, followID)
	if err != nil {
		return graph.Preferences{}, notFound(err, appgraph.ErrNotFound, "follow preferences "+followID)
	}
	return preferencesFrom(row), nil
}

func edgeFrom(row sqlitegen.FollowEdge) graph.Edge {
	return graph.Edge{
		FollowerAccountID: row.FollowerAccountID, FolloweeAccountID: row.FolloweeAccountID, FollowID: row.FollowID,
		State: graph.State(row.State), Since: fromMillis(row.Since),
	}
}

func edgesFrom(rows []sqlitegen.FollowEdge) []graph.Edge {
	out := make([]graph.Edge, len(rows))
	for i, row := range rows {
		out[i] = edgeFrom(row)
	}
	return out
}

// Edge implements graph.Repository.
func (r *GraphRepository) Edge(ctx context.Context, tx uow.Tx, followerAccountID, followeeAccountID string) (graph.Edge, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return graph.Edge{}, err
	}
	return findEdge(ctx, q, followerAccountID, followeeAccountID)
}

// ListEdgesOf implements graph.Repository.
func (r *GraphRepository) ListEdgesOf(ctx context.Context, tx uow.Tx, accountID string) ([]graph.Edge, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListFollowEdgesOf(ctx, accountID)
	if err != nil {
		return nil, wrap(err, "list follow edges of "+accountID)
	}
	return edgesFrom(rows), nil
}

// FindEdge implements graph.Repository.
func (r *GraphRepository) FindEdge(ctx context.Context, followerAccountID, followeeAccountID string) (graph.Edge, error) {
	return findEdge(ctx, r.db.reader(), followerAccountID, followeeAccountID)
}

func findEdge(ctx context.Context, q *sqlitegen.Queries, followerAccountID, followeeAccountID string) (graph.Edge, error) {
	row, err := q.GetFollowEdge(ctx, sqlitegen.GetFollowEdgeParams{
		FollowerAccountID: followerAccountID, FolloweeAccountID: followeeAccountID,
	})
	if err != nil {
		return graph.Edge{}, notFound(err, appgraph.ErrNotFound, "follow edge "+followerAccountID+" -> "+followeeAccountID)
	}
	return edgeFrom(row), nil
}

// InsertEdge implements graph.Repository.
func (r *GraphRepository) InsertEdge(ctx context.Context, tx uow.Tx, e graph.Edge) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertFollowEdge(ctx, sqlitegen.InsertFollowEdgeParams{
		FollowerAccountID: e.FollowerAccountID, FolloweeAccountID: e.FolloweeAccountID, FollowID: e.FollowID,
		State: string(e.State), Since: toMillis(e.Since),
	})
	if err != nil {
		return false, wrap(err, "insert follow edge")
	}
	return n == 1, nil
}

// UpdateEdge implements graph.Repository.
func (r *GraphRepository) UpdateEdge(ctx context.Context, tx uow.Tx, e graph.Edge, from graph.State) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.UpdateFollowEdge(ctx, sqlitegen.UpdateFollowEdgeParams{
		FollowerAccountID: e.FollowerAccountID, FolloweeAccountID: e.FolloweeAccountID, FollowID: e.FollowID,
		State: string(e.State), Since: toMillis(e.Since), FromState: string(from),
	})
	if err != nil {
		return false, wrap(err, "update follow edge")
	}
	return n == 1, nil
}

// DeleteEdge implements graph.Repository.
func (r *GraphRepository) DeleteEdge(ctx context.Context, tx uow.Tx, followerAccountID, followeeAccountID, followID string, state graph.State) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteFollowEdge(ctx, sqlitegen.DeleteFollowEdgeParams{
		FollowerAccountID: followerAccountID, FolloweeAccountID: followeeAccountID, FollowID: followID, State: string(state),
	})
	if err != nil {
		return false, wrap(err, "delete follow edge")
	}
	return n == 1, nil
}

// DeleteAllEdges implements graph.Repository.
func (r *GraphRepository) DeleteAllEdges(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAllFollowEdges(ctx), "delete follow edges")
}

// AddCounters implements graph.Repository.
func (r *GraphRepository) AddCounters(ctx context.Context, tx uow.Tx, accountID string, followersDelta, followingDelta int64) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.AddToFollowCounters(ctx, sqlitegen.AddToFollowCountersParams{
		AccountID: accountID, FollowersCount: followersDelta, FollowingCount: followingDelta,
	})
	if err != nil {
		return wrap(err, "add to follow counters")
	}
	if n == 0 {
		err = q.AddFollowCounters(ctx, sqlitegen.AddFollowCountersParams{
			AccountID: accountID, FollowersCount: followersDelta, FollowingCount: followingDelta,
		})
		if err != nil {
			return wrap(err, "add follow counters")
		}
	}
	return wrap(q.DeleteZeroFollowCounters(ctx, accountID), "delete zero follow counters")
}

// UpsertCounters implements graph.Repository.
func (r *GraphRepository) UpsertCounters(ctx context.Context, tx uow.Tx, c graph.Counters) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertFollowCounters(ctx, sqlitegen.UpsertFollowCountersParams(c)), "upsert follow counters")
}

// DeleteAllCounters implements graph.Repository.
func (r *GraphRepository) DeleteAllCounters(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAllFollowCounters(ctx), "delete follow counters")
}

// ListHistories implements graph.Repository.
func (r *GraphRepository) ListHistories(ctx context.Context, tx uow.Tx) ([]graph.History, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListFollowHistories(ctx)
	if err != nil {
		return nil, wrap(err, "list follow histories")
	}
	out := make([]graph.History, len(rows))
	for i, row := range rows {
		h := graph.History{Follow: graph.Follow{
			ID: row.ID, URI: row.Uri, FollowerAccountID: row.FollowerAccountID, FolloweeAccountID: row.FolloweeAccountID,
			RequestedAt: fromMillis(row.RequestedAt),
		}}
		if row.AcceptedAt.Valid {
			h.Acceptance = &graph.Acceptance{
				FollowID: row.ID, AcceptedBy: graph.Acceptor(row.AcceptedBy.String),
				ActivityURI: row.AcceptanceActivityUri.String, AcceptedAt: fromMillis(row.AcceptedAt.Int64),
			}
		}
		if row.RejectedAt.Valid {
			h.Rejection = &graph.Rejection{
				FollowID: row.ID, RejectedBy: graph.Rejector(row.RejectedBy.String),
				ActivityURI: row.RejectionActivityUri.String, RejectedAt: fromMillis(row.RejectedAt.Int64),
			}
		}
		if row.TerminatedAt.Valid {
			h.Termination = &graph.Termination{
				FollowID: row.ID, Reason: graph.TerminationReason(row.TerminationReason.String),
				TerminatedAt: fromMillis(row.TerminatedAt.Int64),
			}
		}
		out[i] = h
	}
	return out, nil
}

// ListEdges implements graph.Repository.
func (r *GraphRepository) ListEdges(ctx context.Context) ([]graph.Edge, error) {
	rows, err := r.db.reader().ListFollowEdges(ctx)
	if err != nil {
		return nil, wrap(err, "list follow edges")
	}
	return edgesFrom(rows), nil
}

// ListCounters implements graph.Repository.
func (r *GraphRepository) ListCounters(ctx context.Context) ([]graph.Counters, error) {
	rows, err := r.db.reader().ListFollowCounters(ctx)
	if err != nil {
		return nil, wrap(err, "list follow counters")
	}
	out := make([]graph.Counters, len(rows))
	for i, row := range rows {
		out[i] = graph.Counters(row)
	}
	return out, nil
}

// FindCounters implements graph.Repository.
func (r *GraphRepository) FindCounters(ctx context.Context, accountID string) (graph.Counters, error) {
	row, err := r.db.reader().GetFollowCounters(ctx, accountID)
	if err != nil {
		return graph.Counters{}, notFound(err, appgraph.ErrNotFound, "follow counters "+accountID)
	}
	return graph.Counters(row), nil
}

// ListFollowers implements graph.Repository.
func (r *GraphRepository) ListFollowers(ctx context.Context, followeeAccountID string, state graph.State, before string, limit int) ([]graph.Edge, error) {
	rows, err := r.db.reader().ListFollowersPage(ctx, sqlitegen.ListFollowersPageParams{
		AccountID: followeeAccountID, State: string(state), Before: before, PageLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list followers")
	}
	return edgesFrom(rows), nil
}

// ListFollowing implements graph.Repository.
func (r *GraphRepository) ListFollowing(ctx context.Context, followerAccountID string, state graph.State, before string, limit int) ([]graph.Edge, error) {
	rows, err := r.db.reader().ListFollowingPage(ctx, sqlitegen.ListFollowingPageParams{
		AccountID: followerAccountID, State: string(state), Before: before, PageLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list following")
	}
	return edgesFrom(rows), nil
}
