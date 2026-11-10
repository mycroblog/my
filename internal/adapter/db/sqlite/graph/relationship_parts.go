// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/graphgen"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// FindRelationshipParts implements graph.Repository: one query per part, mapped as the reads of
// one pair map their rows (edgeFrom, preferencesFrom, muteFrom).
func (r *Repository) FindRelationshipParts(ctx context.Context, accountID string, targetIDs []string, now time.Time) (appgraph.RelationshipParts, error) {
	out := appgraph.RelationshipParts{
		Following: map[string]graph.Edge{}, FollowedBy: map[string]graph.Edge{}, Preferences: map[string]graph.Preferences{},
		Blocking: map[string]graph.BlockEdge{}, BlockedBy: map[string]graph.BlockEdge{}, Mutes: map[string]graph.Mute{},
	}
	if len(targetIDs) == 0 {
		return out, nil
	}
	targets, err := sqlite.JSONList(targetIDs)
	if err != nil {
		return out, err
	}
	q := r.db.reader()
	following, err := q.ListFollowEdgesFrom(ctx, graphgen.ListFollowEdgesFromParams{AccountID: accountID, Targets: targets})
	if err != nil {
		return out, sqlite.Wrap(err, "list follow edges from "+accountID)
	}
	var followIDs []string
	for _, row := range following {
		e := edgeFrom(row)
		out.Following[e.FolloweeAccountID] = e
		followIDs = append(followIDs, e.FollowID)
	}
	followedBy, err := q.ListFollowEdgesTo(ctx, graphgen.ListFollowEdgesToParams{AccountID: accountID, Targets: targets})
	if err != nil {
		return out, sqlite.Wrap(err, "list follow edges to "+accountID)
	}
	for _, row := range followedBy {
		e := edgeFrom(row)
		out.FollowedBy[e.FollowerAccountID] = e
	}
	if len(followIDs) > 0 {
		ids, err := sqlite.JSONList(followIDs)
		if err != nil {
			return out, err
		}
		prefs, err := q.ListFollowPreferences(ctx, ids)
		if err != nil {
			return out, sqlite.Wrap(err, "list follow preferences")
		}
		for _, row := range prefs {
			p := preferencesFrom(row)
			out.Preferences[p.FollowID] = p
		}
	}
	blocking, err := q.ListBlockEdgesFrom(ctx, graphgen.ListBlockEdgesFromParams{AccountID: accountID, Targets: targets})
	if err != nil {
		return out, sqlite.Wrap(err, "list block edges from "+accountID)
	}
	for _, row := range blocking {
		out.Blocking[row.BlockedAccountID] = graph.BlockEdge(row)
	}
	blockedBy, err := q.ListBlockEdgesTo(ctx, graphgen.ListBlockEdgesToParams{AccountID: accountID, Targets: targets})
	if err != nil {
		return out, sqlite.Wrap(err, "list block edges to "+accountID)
	}
	for _, row := range blockedBy {
		out.BlockedBy[row.BlockerAccountID] = graph.BlockEdge(row)
	}
	mutes, err := q.ListActiveMutesOf(ctx, graphgen.ListActiveMutesOfParams{AccountID: accountID, Targets: targets, Now: sqlite.ToMillis(now)})
	if err != nil {
		return out, sqlite.Wrap(err, "list mutes of "+accountID)
	}
	for _, row := range mutes {
		m := muteFrom(row)
		out.Mutes[m.TargetAccountID] = m
	}
	return out, nil
}

// NotificationRelationsTx implements graph.Repository: one query per part, inside tx.
func (r *Repository) NotificationRelationsTx(ctx context.Context, tx uow.Tx, sourceID string, recipientIDs []string, now time.Time) (appgraph.NotificationRelations, error) {
	out := appgraph.NotificationRelations{Blocked: map[string]bool{}, Mutes: map[string]graph.Mute{}, Follows: map[string]graph.Edge{}}
	if len(recipientIDs) == 0 {
		return out, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return out, err
	}
	targets, err := sqlite.JSONList(recipientIDs)
	if err != nil {
		return out, err
	}
	blocking, err := q.ListBlockEdgesFrom(ctx, graphgen.ListBlockEdgesFromParams{AccountID: sourceID, Targets: targets})
	if err != nil {
		return out, sqlite.Wrap(err, "list block edges from "+sourceID)
	}
	for _, row := range blocking {
		out.Blocked[row.BlockedAccountID] = true
	}
	blockedBy, err := q.ListBlockEdgesTo(ctx, graphgen.ListBlockEdgesToParams{AccountID: sourceID, Targets: targets})
	if err != nil {
		return out, sqlite.Wrap(err, "list block edges to "+sourceID)
	}
	for _, row := range blockedBy {
		out.Blocked[row.BlockerAccountID] = true
	}
	mutes, err := q.ListActiveMutesTo(ctx, graphgen.ListActiveMutesToParams{AccountID: sourceID, Targets: targets, Now: sqlite.ToMillis(now)})
	if err != nil {
		return out, sqlite.Wrap(err, "list mutes of "+sourceID)
	}
	for _, row := range mutes {
		m := muteFrom(row)
		out.Mutes[m.AccountID] = m
	}
	followers, err := q.ListFollowEdgesTo(ctx, graphgen.ListFollowEdgesToParams{AccountID: sourceID, Targets: targets})
	if err != nil {
		return out, sqlite.Wrap(err, "list follow edges to "+sourceID)
	}
	for _, row := range followers {
		e := edgeFrom(row)
		out.Follows[e.FollowerAccountID] = e
	}
	return out, nil
}
