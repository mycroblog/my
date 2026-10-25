// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// The aliases of local accounts (ADR-0089, FR-009): the accounts that a local user moves from,
// which the user's actor names in alsoKnownAs so that they may move to it. The caller resolves a
// handle or URL to the alias's actor (WebFinger and the fetch of the actor, outside any
// transaction) before it adds the alias. Every change emits AliasesChanged, which sends the
// actor's Update to its followers as a profile change does.

// AliasRepository is the part of Repository about aliases (data-model §4.2).
type AliasRepository interface {
	// InsertAlias inserts an alias and reports false, writing nothing, when the account has it
	// already.
	InsertAlias(ctx context.Context, tx uow.Tx, a account.Alias) (bool, error)
	// DeleteAlias deletes an alias and reports whether there was one.
	DeleteAlias(ctx context.Context, tx uow.Tx, accountID, uri string) (bool, error)
	// CountAliases counts the aliases of an account inside tx.
	CountAliases(ctx context.Context, tx uow.Tx, accountID string) (int, error)
	// ListAliases returns the aliases of an account, oldest first.
	ListAliases(ctx context.Context, accountID string) ([]account.Alias, error)
	// HasAlias reports whether an account has the alias uri.
	HasAlias(ctx context.Context, accountID, uri string) (bool, error)
}

// EventAliasesChanged is emitted when an alias is added or removed.
const EventAliasesChanged = "account.aliases_changed"

// AliasesChanged is emitted when a local account adds or removes an alias, in the transaction
// that changes it.
type AliasesChanged struct {
	AccountID string
	ChangedAt time.Time
}

// EventType implements uow.Event.
func (AliasesChanged) EventType() string { return EventAliasesChanged }

// AddAlias adds alias, a resolved actor, to the aliases of the local account accountID and emits
// AliasesChanged. An alias the account has already changes nothing and reports false. The
// account itself fails with account.ErrSelfAlias, the instance actor with ErrNotFound, and an
// alias beyond account.MaxAliases with account.ErrTooManyAliases.
func (s *Service) AddAlias(ctx context.Context, accountID string, alias Actor) (account.Alias, bool, error) {
	if alias.IsInstanceActor() || alias.Deleted() {
		return account.Alias{}, false, fmt.Errorf("alias %s: %w", alias.Account.URI, ErrNotFound)
	}
	a := account.Alias{AccountID: accountID, URI: alias.Account.URI, AddedAt: kernel.Normalize(s.d.Clock.Now())}
	repo := s.d.Repository
	var added bool
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		added = false
		own, err := s.localAccount(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if err := a.Validate(own.URI); err != nil {
			return err
		}
		n, err := repo.CountAliases(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if n >= account.MaxAliases {
			if ok, err := repo.HasAlias(ctx, accountID, a.URI); err != nil || ok {
				return err
			}
			return account.ErrTooManyAliases
		}
		if added, err = repo.InsertAlias(ctx, tx, a); err != nil || !added {
			return err
		}
		return tx.Emit(ctx, AliasesChanged{AccountID: accountID, ChangedAt: a.AddedAt})
	})
	if err != nil {
		return account.Alias{}, false, err
	}
	return a, added, nil
}

// RemoveAlias removes the alias uri of the local account accountID and emits AliasesChanged. An
// alias the account does not have changes nothing and reports false.
func (s *Service) RemoveAlias(ctx context.Context, accountID, uri string) (bool, error) {
	var removed bool
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		removed = false
		if _, err := s.localAccount(ctx, tx, accountID); err != nil {
			return err
		}
		var err error
		if removed, err = s.d.Repository.DeleteAlias(ctx, tx, accountID, uri); err != nil || !removed {
			return err
		}
		return tx.Emit(ctx, AliasesChanged{AccountID: accountID, ChangedAt: kernel.Normalize(s.d.Clock.Now())})
	})
	return removed, err
}

// Aliases returns the aliases of an account, oldest first; a remote account has none.
func (s *Service) Aliases(ctx context.Context, accountID string) ([]account.Alias, error) {
	return s.d.Repository.ListAliases(ctx, accountID)
}

// HasAlias reports whether the account accountID names the actor aliasURI among its aliases,
// which lets that actor move to it (ADR-0089).
func (s *Service) HasAlias(ctx context.Context, accountID, aliasURI string) (bool, error) {
	return s.d.Repository.HasAlias(ctx, accountID, aliasURI)
}

// localAccount returns the local account accountID inside tx; a remote account or the instance
// actor fails with ErrNotLocal.
func (s *Service) localAccount(ctx context.Context, tx uow.Tx, accountID string) (account.Account, error) {
	a, err := s.d.Repository.Account(ctx, tx, accountID)
	switch {
	case err != nil:
		return account.Account{}, err
	case !a.IsLocal() || a.ActorType == account.ActorApplication:
		return account.Account{}, fmt.Errorf("account %s: %w", accountID, ErrNotLocal)
	}
	return a, nil
}
