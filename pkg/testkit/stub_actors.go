// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"slices"
	"sync"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
)

// stubActors stands in for the account directory of the content and moderation use cases: it
// serves the actors set on it.
type stubActors struct {
	mu     sync.Mutex
	actors map[string]appaccount.Actor
}

func (s *stubActors) set(a appaccount.Actor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actors[a.Account.ID] = a
}

func (s *stubActors) local(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.actors[id].Account.IsLocal()
}

func (s *stubActors) FindByHandle(context.Context, string, string) (appaccount.Actor, error) {
	return appaccount.Actor{}, appaccount.ErrNotFound
}

func (s *stubActors) FindByID(_ context.Context, id string) (appaccount.Actor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.actors[id]
	if !ok {
		return appaccount.Actor{}, appaccount.ErrNotFound
	}
	return a, nil
}

func (s *stubActors) ExistingIDs(_ context.Context, ids []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, id := range ids {
		if _, ok := s.actors[id]; ok {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out, nil
}

// MoveState implements the graph module's Accounts: no stub account has moved.
func (s *stubActors) MoveState(context.Context, string) (appaccount.MoveState, error) {
	return appaccount.MoveState{}, appaccount.ErrNotFound
}

func (s *stubActors) FindByIDTx(ctx context.Context, _ uow.Tx, id string) (appaccount.Actor, error) {
	return s.FindByID(ctx, id)
}

// AccountDomain implements appmoderation.AccountDomains.
func (s *stubActors) AccountDomain(ctx context.Context, id string) (string, error) {
	a, err := s.FindByID(ctx, id)
	return a.Account.Domain, err
}

func (s *stubActors) AccountDomainTx(ctx context.Context, _ uow.Tx, id string) (string, error) {
	return s.AccountDomain(ctx, id)
}
