// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"context"
	"slices"
	"sync"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
)

// StubActors stands in for the account directory of the content and moderation use cases: it
// serves the actors set on it.
type StubActors struct {
	Mu     sync.Mutex
	Actors map[string]appaccount.Actor
}

// Set serves a.
func (s *StubActors) Set(a appaccount.Actor) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Actors[a.Account.ID] = a
}

// Local reports whether the actor of id is local.
func (s *StubActors) Local(id string) bool {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.Actors[id].Account.IsLocal()
}

func (s *StubActors) FindByHandle(context.Context, string, string) (appaccount.Actor, error) {
	return appaccount.Actor{}, appaccount.ErrNotFound
}

func (s *StubActors) FindByID(_ context.Context, id string) (appaccount.Actor, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	a, ok := s.Actors[id]
	if !ok {
		return appaccount.Actor{}, appaccount.ErrNotFound
	}
	return a, nil
}

func (s *StubActors) FindByIDs(_ context.Context, ids []string) (map[string]appaccount.Actor, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	out := make(map[string]appaccount.Actor, len(ids))
	for _, id := range ids {
		if a, ok := s.Actors[id]; ok {
			out[id] = a
		}
	}
	return out, nil
}

func (s *StubActors) ExistingIDs(_ context.Context, ids []string) ([]string, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	var out []string
	for _, id := range ids {
		if _, ok := s.Actors[id]; ok {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out, nil
}

// MoveState implements the graph module's Accounts: no stub account has moved.
func (s *StubActors) MoveState(context.Context, string) (appaccount.MoveState, error) {
	return appaccount.MoveState{}, appaccount.ErrNotFound
}

func (s *StubActors) FindByIDTx(ctx context.Context, _ uow.Tx, id string) (appaccount.Actor, error) {
	return s.FindByID(ctx, id)
}

// AccountDomain implements appmoderation.AccountDomains.
func (s *StubActors) AccountDomain(ctx context.Context, id string) (string, error) {
	a, err := s.FindByID(ctx, id)
	return a.Account.Domain, err
}

func (s *StubActors) AccountDomainTx(ctx context.Context, _ uow.Tx, id string) (string, error) {
	return s.AccountDomain(ctx, id)
}
