// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/graphgen"
	appgraph "github.com/mycroblog/my/internal/app/graph"
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
	q := r.db.reader()
	following, err := q.ListFollowEdgesFrom(ctx, graphgen.ListFollowEdgesFromParams{AccountID: accountID, Targets: targetIDs})
	if err != nil {
		return out, postgres.Wrap(err, "list follow edges from "+accountID)
	}
	var followIDs []string
	for _, row := range following {
		e := edgeFrom(row)
		out.Following[e.FolloweeAccountID] = e
		followIDs = append(followIDs, e.FollowID)
	}
	followedBy, err := q.ListFollowEdgesTo(ctx, graphgen.ListFollowEdgesToParams{AccountID: accountID, Targets: targetIDs})
	if err != nil {
		return out, postgres.Wrap(err, "list follow edges to "+accountID)
	}
	for _, row := range followedBy {
		e := edgeFrom(row)
		out.FollowedBy[e.FollowerAccountID] = e
	}
	if len(followIDs) > 0 {
		ids := followIDs
		prefs, err := q.ListFollowPreferences(ctx, ids)
		if err != nil {
			return out, postgres.Wrap(err, "list follow preferences")
		}
		for _, row := range prefs {
			p := preferencesFrom(row)
			out.Preferences[p.FollowID] = p
		}
	}
	blocking, err := q.ListBlockEdgesFrom(ctx, graphgen.ListBlockEdgesFromParams{AccountID: accountID, Targets: targetIDs})
	if err != nil {
		return out, postgres.Wrap(err, "list block edges from "+accountID)
	}
	for _, row := range blocking {
		out.Blocking[row.BlockedAccountID] = graph.BlockEdge(row)
	}
	blockedBy, err := q.ListBlockEdgesTo(ctx, graphgen.ListBlockEdgesToParams{AccountID: accountID, Targets: targetIDs})
	if err != nil {
		return out, postgres.Wrap(err, "list block edges to "+accountID)
	}
	for _, row := range blockedBy {
		out.BlockedBy[row.BlockerAccountID] = graph.BlockEdge(row)
	}
	mutes, err := q.ListActiveMutesOf(ctx, graphgen.ListActiveMutesOfParams{AccountID: accountID, Targets: targetIDs, Now: now})
	if err != nil {
		return out, postgres.Wrap(err, "list mutes of "+accountID)
	}
	for _, row := range mutes {
		m := muteFrom(row)
		out.Mutes[m.TargetAccountID] = m
	}
	return out, nil
}
