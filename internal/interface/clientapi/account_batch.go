// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"fmt"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/domain/account"
)

// profileRestrictionBatch is a ProfileRestrictions that reads many accounts at once, as
// *moderation.AccountPolicy does.
type profileRestrictionBatch interface {
	ProfileHiddenOf(ctx context.Context, accountIDs []string) (map[string]bool, error)
}

// relationshipBatch is a Graph that reads the relationships of an account with many at once, as
// *appgraph.Service does.
type relationshipBatch interface {
	Relationships(ctx context.Context, accountID string, targetIDs []string) (map[string]appgraph.Relationship, error)
}

// presentableAmong returns, by ID, the accounts of ids that presentableByID would present, reading
// the accounts, their profile restrictions, and the viewer's blocks with a number of queries that
// does not grow with ids.
func (s *Server) presentableAmong(ctx context.Context, viewer *Viewer, ids []string, blockCheck bool) (map[string]appaccount.Actor, error) {
	var unique []string
	for _, id := range ids {
		if !slices.Contains(unique, id) {
			unique = append(unique, id)
		}
	}
	out := map[string]appaccount.Actor{}
	if len(unique) == 0 {
		return out, nil
	}
	found, err := s.o.Accounts.FindByIDs(ctx, unique)
	if err != nil {
		return nil, err
	}
	return s.presentableOf(ctx, viewer, found, unique, blockCheck)
}

// presentableOf returns, by ID, the accounts of found, among ids, that presentable would present,
// reading their profile restrictions and the viewer's blocks together.
func (s *Server) presentableOf(ctx context.Context, viewer *Viewer, found map[string]appaccount.Actor, ids []string, blockCheck bool) (map[string]appaccount.Actor, error) {
	out := map[string]appaccount.Actor{}
	// others are the accounts that the checks of moderation and blocks apply to: every one but
	// the viewer's own.
	var others []string
	for _, id := range ids {
		a, ok := found[id]
		if !ok || a.Account.ActorType == account.ActorApplication || !a.Lifecycle.Active() {
			continue
		}
		out[id] = a
		if viewer == nil || viewer.AccountID != id {
			others = append(others, id)
		}
	}
	if len(others) == 0 {
		return out, nil
	}
	hidden, err := s.profilesHidden(ctx, others)
	if err != nil {
		return nil, err
	}
	blockers := map[string]bool{}
	if blockCheck && viewer != nil {
		if blockers, err = s.blockersOf(ctx, viewer.AccountID, others); err != nil {
			return nil, err
		}
	}
	for _, id := range others {
		if hidden[id] || blockers[id] {
			delete(out, id)
		}
	}
	return out, nil
}

// presentableActors returns the accounts of ids that presentableByID would present, in the order
// of ids, with the reads of presentableAmong.
func (s *Server) presentableActors(ctx context.Context, viewer *Viewer, ids []string, blockCheck bool) ([]appaccount.Actor, error) {
	shown, err := s.presentableAmong(ctx, viewer, ids, blockCheck)
	if err != nil {
		return nil, err
	}
	var out []appaccount.Actor
	for _, id := range ids {
		if a, ok := shown[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

// profilesHidden reports, by account, whether moderation hides the profiles of ids: together when
// ProfileRestrictions can, one by one otherwise.
func (s *Server) profilesHidden(ctx context.Context, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if s.o.ProfileRestrictions == nil {
		return out, nil
	}
	if b, ok := s.o.ProfileRestrictions.(profileRestrictionBatch); ok {
		return b.ProfileHiddenOf(ctx, ids)
	}
	for _, id := range ids {
		hidden, err := s.o.ProfileRestrictions.ProfileHidden(ctx, id)
		if err != nil {
			return nil, err
		}
		out[id] = hidden
	}
	return out, nil
}

// blockersOf reports, by account, whether each of ids blocks accountID: together when Graph can,
// one by one otherwise.
func (s *Server) blockersOf(ctx context.Context, accountID string, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if b, ok := s.o.Graph.(relationshipBatch); ok {
		rs, err := b.Relationships(ctx, accountID, ids)
		if err != nil {
			return nil, fmt.Errorf("clientapi: the blocks of %s: %w", accountID, err)
		}
		for _, id := range ids {
			out[id] = rs[id].BlockedBy
		}
		return out, nil
	}
	for _, id := range ids {
		blocked, err := s.o.Graph.IsBlocking(ctx, id, accountID)
		if err != nil {
			return nil, err
		}
		out[id] = blocked
	}
	return out, nil
}

// relationships returns the relationships of accountID with ids, by target: together when Graph
// can, one by one otherwise.
func (s *Server) relationships(ctx context.Context, accountID string, ids []string) (map[string]appgraph.Relationship, error) {
	if b, ok := s.o.Graph.(relationshipBatch); ok {
		return b.Relationships(ctx, accountID, ids)
	}
	out := make(map[string]appgraph.Relationship, len(ids))
	for _, id := range ids {
		r, err := s.o.Graph.Relationship(ctx, accountID, id)
		if err != nil {
			return nil, err
		}
		out[id] = r
	}
	return out, nil
}

// subscriptionBatch is a Subscriptions that reads many targets at once, as
// *appnotification.Subscriptions does.
type subscriptionBatch interface {
	SubscribedAmong(ctx context.Context, accountID string, targetIDs []string) (map[string]bool, error)
}

// subscribedAmong returns the targets among ids that accountID subscribes to: together when
// Subscriptions can, one by one otherwise.
func (s *Server) subscribedAmong(ctx context.Context, accountID string, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if s.o.Subscriptions == nil || len(ids) == 0 {
		return out, nil
	}
	if b, ok := s.o.Subscriptions.(subscriptionBatch); ok {
		return b.SubscribedAmong(ctx, accountID, ids)
	}
	for _, id := range ids {
		ok, err := s.o.Subscriptions.Subscribed(ctx, accountID, id)
		if err != nil {
			return nil, err
		}
		out[id] = ok
	}
	return out, nil
}
