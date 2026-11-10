// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/graphgen"
	"github.com/mycroblog/my/internal/domain/graph"
)

// FindPreferencesOf implements graph.Repository.
func (r *Repository) FindPreferencesOf(ctx context.Context, followIDs []string) (map[string]graph.Preferences, error) {
	out := make(map[string]graph.Preferences, len(followIDs))
	if len(followIDs) == 0 {
		return out, nil
	}
	list := followIDs
	rows, err := r.db.reader().ListFollowPreferences(ctx, list)
	if err != nil {
		return nil, postgres.Wrap(err, "list follow preferences")
	}
	for _, row := range rows {
		p := preferencesFrom(row)
		out[p.FollowID] = p
	}
	return out, nil
}

// FindMutesOn implements graph.Repository.
func (r *Repository) FindMutesOn(ctx context.Context, targetID string, accountIDs []string, now time.Time) (map[string]graph.Mute, error) {
	out := make(map[string]graph.Mute, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	targets := accountIDs
	rows, err := r.db.reader().ListActiveMutesTo(ctx, graphgen.ListActiveMutesToParams{AccountID: targetID, Targets: targets, Now: now})
	if err != nil {
		return nil, postgres.Wrap(err, "list mutes of "+targetID)
	}
	for _, row := range rows {
		m := muteFrom(row)
		out[m.AccountID] = m
	}
	return out, nil
}
