// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/graphgen"
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
	var p graphgen.InsertFollowsParams
	for _, f := range fs {
		p.Ids, p.Uris = append(p.Ids, f.ID), append(p.Uris, f.URI)
		p.FollowerAccountIds, p.FolloweeAccountIds = append(p.FollowerAccountIds, f.FollowerAccountID), append(p.FolloweeAccountIds, f.FolloweeAccountID)
		p.RequestedAts = append(p.RequestedAts, f.RequestedAt)
	}
	return postgres.Wrap(q.InsertFollows(ctx, p), "insert follows")
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
	var p graphgen.UpsertManyFollowPreferencesParams
	for _, pr := range ps {
		p.FollowIds, p.ShowBoosts = append(p.FollowIds, pr.FollowID), append(p.ShowBoosts, pr.ShowBoosts)
	}
	return postgres.Wrap(q.UpsertManyFollowPreferences(ctx, p), "upsert follow preferences")
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
	err = q.InsertAutoFollowAcceptances(ctx, graphgen.InsertAutoFollowAcceptancesParams{FollowIds: followIDs, AcceptedBy: string(graph.AcceptedByAuto), AcceptedAt: at})
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
	var p graphgen.InsertFollowEdgesParams
	for _, e := range es {
		p.FollowerAccountIds, p.FolloweeAccountIds = append(p.FollowerAccountIds, e.FollowerAccountID), append(p.FolloweeAccountIds, e.FolloweeAccountID)
		p.FollowIds, p.States, p.Sinces = append(p.FollowIds, e.FollowID), append(p.States, string(e.State)), append(p.Sinces, e.Since)
	}
	n, err := q.InsertFollowEdges(ctx, p)
	return int(n), postgres.Wrap(err, "insert follow edges")
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
	rows, err := q.ListFollowPreferences(ctx, followIDs)
	if err != nil {
		return nil, postgres.Wrap(err, "list follow preferences")
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
	ids, err := r.db.reader().DomainMutersAmong(ctx, graphgen.DomainMutersAmongParams{AccountIds: accountIDs, Domains: domains})
	if err != nil {
		return nil, postgres.Wrap(err, "list domain muters")
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
	targets := others
	from, err := q.ListBlockEdgesFrom(ctx, graphgen.ListBlockEdgesFromParams{AccountID: accountID, Targets: targets})
	if err != nil {
		return nil, nil, postgres.Wrap(err, "list block edges from "+accountID)
	}
	for _, row := range from {
		blocking[row.BlockedAccountID] = true
	}
	to, err := q.ListBlockEdgesTo(ctx, graphgen.ListBlockEdgesToParams{AccountID: accountID, Targets: targets})
	if err != nil {
		return nil, nil, postgres.Wrap(err, "list block edges to "+accountID)
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
	blocking, err := q.ListBlockEdgesFrom(ctx, graphgen.ListBlockEdgesFromParams{AccountID: recipientID, Targets: sourceIDs})
	if err != nil {
		return out, postgres.Wrap(err, "list block edges from "+recipientID)
	}
	for _, row := range blocking {
		out.Blocked[row.BlockedAccountID] = true
	}
	blockedBy, err := q.ListBlockEdgesTo(ctx, graphgen.ListBlockEdgesToParams{AccountID: recipientID, Targets: sourceIDs})
	if err != nil {
		return out, postgres.Wrap(err, "list block edges to "+recipientID)
	}
	for _, row := range blockedBy {
		out.Blocked[row.BlockerAccountID] = true
	}
	mutes, err := q.ListActiveMutesOf(ctx, graphgen.ListActiveMutesOfParams{AccountID: recipientID, Targets: sourceIDs, Now: now})
	if err != nil {
		return out, postgres.Wrap(err, "list mutes of "+recipientID)
	}
	for _, row := range mutes {
		m := muteFrom(row)
		out.Mutes[m.TargetAccountID] = m
	}
	following, err := q.ListFollowEdgesFrom(ctx, graphgen.ListFollowEdgesFromParams{AccountID: recipientID, Targets: sourceIDs})
	if err != nil {
		return out, postgres.Wrap(err, "list follow edges from "+recipientID)
	}
	for _, row := range following {
		e := edgeFrom(row)
		out.Follows[e.FolloweeAccountID] = e
	}
	return out, nil
}
