// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/graphgen"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// InsertFollows implements graph.Repository.
func (r *Repository) InsertFollows(ctx context.Context, tx uow.Tx, fs []graph.Follow) error {
	if len(fs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	type row struct {
		ID                string `json:"id"`
		URI               string `json:"uri"`
		FollowerAccountID string `json:"follower_account_id"`
		FolloweeAccountID string `json:"followee_account_id"`
		RequestedAt       int64  `json:"requested_at"`
	}
	rows := make([]row, len(fs))
	for i, f := range fs {
		rows[i] = row{ID: f.ID, URI: f.URI, FollowerAccountID: f.FollowerAccountID, FolloweeAccountID: f.FolloweeAccountID, RequestedAt: sqlite.ToMillis(f.RequestedAt)}
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertFollows(ctx, string(list)), "insert follows")
}

// UpsertManyPreferences implements graph.Repository.
func (r *Repository) UpsertManyPreferences(ctx context.Context, tx uow.Tx, ps []graph.Preferences) error {
	if len(ps) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	type row struct {
		FollowID   string `json:"follow_id"`
		ShowBoosts bool   `json:"show_boosts"`
	}
	rows := make([]row, len(ps))
	for i, pr := range ps {
		rows[i] = row{FollowID: pr.FollowID, ShowBoosts: pr.ShowBoosts}
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertManyFollowPreferences(ctx, string(list)), "upsert follow preferences")
}

// InsertAutoAcceptances implements graph.Repository.
func (r *Repository) InsertAutoAcceptances(ctx context.Context, tx uow.Tx, followIDs []string, at time.Time) error {
	if len(followIDs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	list, err := sqlite.JSONList(followIDs)
	if err != nil {
		return err
	}
	err = q.InsertAutoFollowAcceptances(ctx, graphgen.InsertAutoFollowAcceptancesParams{FollowIds: list, AcceptedBy: string(graph.AcceptedByAuto), AcceptedAt: sqlite.ToMillis(at)})
	return alreadyRecorded(err, "follow_acceptances", followIDs[0])
}

// InsertEdges implements graph.Repository.
func (r *Repository) InsertEdges(ctx context.Context, tx uow.Tx, es []graph.Edge) (int, error) {
	if len(es) == 0 {
		return 0, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	type row struct {
		FollowerAccountID string `json:"follower_account_id"`
		FolloweeAccountID string `json:"followee_account_id"`
		FollowID          string `json:"follow_id"`
		State             string `json:"state"`
		Since             int64  `json:"since"`
	}
	rows := make([]row, len(es))
	for i, e := range es {
		rows[i] = row{FollowerAccountID: e.FollowerAccountID, FolloweeAccountID: e.FolloweeAccountID, FollowID: e.FollowID, State: string(e.State), Since: sqlite.ToMillis(e.Since)}
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return 0, err
	}
	n, err := q.InsertFollowEdges(ctx, string(list))
	return int(n), sqlite.Wrap(err, "insert follow edges")
}

// PreferencesOf implements graph.Repository.
func (r *Repository) PreferencesOf(ctx context.Context, tx uow.Tx, followIDs []string) (map[string]graph.Preferences, error) {
	out := make(map[string]graph.Preferences, len(followIDs))
	if len(followIDs) == 0 {
		return out, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	list, err := sqlite.JSONList(followIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListFollowPreferences(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list follow preferences")
	}
	for _, row := range rows {
		p := preferencesFrom(row)
		out[p.FollowID] = p
	}
	return out, nil
}

// DomainMutersAmong implements graph.Repository.
func (r *Repository) DomainMutersAmong(ctx context.Context, accountIDs, domains []string) (map[string]bool, error) {
	out := make(map[string]bool, len(accountIDs))
	if len(accountIDs) == 0 || len(domains) == 0 {
		return out, nil
	}
	accounts, err := sqlite.JSONList(accountIDs)
	if err != nil {
		return nil, err
	}
	ds, err := sqlite.JSONList(domains)
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().DomainMutersAmong(ctx, graphgen.DomainMutersAmongParams{AccountIds: accounts, Domains: ds})
	if err != nil {
		return nil, sqlite.Wrap(err, "list domain muters")
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// BlockEdgesWithTx implements graph.Repository.
func (r *Repository) BlockEdgesWithTx(ctx context.Context, tx uow.Tx, accountID string, others []string) (blocking, blockedBy map[string]bool, err error) {
	blocking, blockedBy = map[string]bool{}, map[string]bool{}
	if len(others) == 0 {
		return blocking, blockedBy, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, nil, err
	}
	targets, err := sqlite.JSONList(others)
	if err != nil {
		return nil, nil, err
	}
	from, err := q.ListBlockEdgesFrom(ctx, graphgen.ListBlockEdgesFromParams{AccountID: accountID, Targets: targets})
	if err != nil {
		return nil, nil, sqlite.Wrap(err, "list block edges from "+accountID)
	}
	for _, row := range from {
		blocking[row.BlockedAccountID] = true
	}
	to, err := q.ListBlockEdgesTo(ctx, graphgen.ListBlockEdgesToParams{AccountID: accountID, Targets: targets})
	if err != nil {
		return nil, nil, sqlite.Wrap(err, "list block edges to "+accountID)
	}
	for _, row := range to {
		blockedBy[row.BlockerAccountID] = true
	}
	return blocking, blockedBy, nil
}

// NotificationRelationsFromTx implements graph.Repository.
func (r *Repository) NotificationRelationsFromTx(ctx context.Context, tx uow.Tx, recipientID string, sourceIDs []string, now time.Time) (appgraph.NotificationRelations, error) {
	out := appgraph.NotificationRelations{Blocked: map[string]bool{}, Mutes: map[string]graph.Mute{}, Follows: map[string]graph.Edge{}}
	if len(sourceIDs) == 0 {
		return out, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return out, err
	}
	targets, err := sqlite.JSONList(sourceIDs)
	if err != nil {
		return out, err
	}
	blocking, err := q.ListBlockEdgesFrom(ctx, graphgen.ListBlockEdgesFromParams{AccountID: recipientID, Targets: targets})
	if err != nil {
		return out, sqlite.Wrap(err, "list block edges from "+recipientID)
	}
	for _, row := range blocking {
		out.Blocked[row.BlockedAccountID] = true
	}
	blockedBy, err := q.ListBlockEdgesTo(ctx, graphgen.ListBlockEdgesToParams{AccountID: recipientID, Targets: targets})
	if err != nil {
		return out, sqlite.Wrap(err, "list block edges to "+recipientID)
	}
	for _, row := range blockedBy {
		out.Blocked[row.BlockerAccountID] = true
	}
	mutes, err := q.ListActiveMutesOf(ctx, graphgen.ListActiveMutesOfParams{AccountID: recipientID, Targets: targets, Now: sqlite.ToMillis(now)})
	if err != nil {
		return out, sqlite.Wrap(err, "list mutes of "+recipientID)
	}
	for _, row := range mutes {
		m := muteFrom(row)
		out.Mutes[m.TargetAccountID] = m
	}
	following, err := q.ListFollowEdgesFrom(ctx, graphgen.ListFollowEdgesFromParams{AccountID: recipientID, Targets: targets})
	if err != nil {
		return out, sqlite.Wrap(err, "list follow edges from "+recipientID)
	}
	for _, row := range following {
		e := edgeFrom(row)
		out.Follows[e.FolloweeAccountID] = e
	}
	return out, nil
}
