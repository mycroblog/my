// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"

	"github.com/mycroblog/my/internal/domain/account"
)

// The account search of the moderators (FR-714): the accounts newest first, in keyset pages by
// account ID. The identity module's AccountAdmin combines it with what the other modules know.

// Origins of the accounts that a search keeps.
const (
	OriginLocal  = "local"
	OriginRemote = "remote"
)

// SearchQuery selects the accounts of a search. Empty fields select every account; the
// instance actor is never selected.
type SearchQuery struct {
	// UsernamePrefix keeps the accounts whose handle's username starts with it, compared
	// case-insensitively. A remote account whose handle is not verified has no username to match.
	UsernamePrefix string
	// Domain keeps the remote accounts of one domain.
	Domain string
	// Origin keeps the local (OriginLocal) or the remote (OriginRemote) accounts.
	Origin string
	// Before is the ID of the last account of the previous page; "" starts from the newest.
	Before string
}

// SearchRepository is the part of the account module's persistence that the account search
// reads. The database adapters implement it and pass identitytest.AdminAccountContract.
type SearchRepository interface {
	// SearchAccountIDs returns up to limit accounts that q selects, newest first, by ID.
	SearchAccountIDs(ctx context.Context, q SearchQuery, limit int) ([]string, error)
	// ListAccountIDsInLifecycleState returns up to limit accounts in a lifecycle state newest
	// first, by ID, whose ID is below before ("" for the first page). Accounts without lifecycle
	// events are active and have no row, so state is never account.StateActive.
	ListAccountIDsInLifecycleState(ctx context.Context, state account.LifecycleStateName, before string, limit int) ([]string, error)
}

// Search returns up to limit accounts that q selects, newest first, by ID.
func (s *Service) Search(ctx context.Context, q SearchQuery, limit int) ([]string, error) {
	q.UsernamePrefix = account.NormalizeUsername(q.UsernamePrefix)
	q.Domain = account.NormalizeDomain(q.Domain)
	return s.d.Repository.SearchAccountIDs(ctx, q, limit)
}

// DeletedAccounts returns up to limit deleted accounts, locally or on their servers, newest
// first, by ID, whose ID is below before ("" for the first page).
func (s *Service) DeletedAccounts(ctx context.Context, before string, limit int) ([]string, error) {
	return s.d.Repository.ListAccountIDsInLifecycleState(ctx, account.StateDeleted, before, limit)
}

// DeactivatedAccounts returns up to limit accounts that their owners deactivated, newest first, by
// ID, whose ID is below before ("" for the first page).
func (s *Service) DeactivatedAccounts(ctx context.Context, before string, limit int) ([]string, error) {
	return s.d.Repository.ListAccountIDsInLifecycleState(ctx, account.StateDeactivated, before, limit)
}

// NormalizeUsername returns the form in which usernames compare: lowercase.
func NormalizeUsername(username string) string { return account.NormalizeUsername(username) }

// NormalizeDomain returns the stored form of a domain: lowercase.
func NormalizeDomain(domain string) string { return account.NormalizeDomain(domain) }
