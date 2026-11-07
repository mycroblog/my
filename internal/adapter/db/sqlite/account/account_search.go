// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/accountgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// The account_search_terms projection of the account module and its prefix search (ADR-0092 §1).

var _ appaccount.SearchTermRepository = (*Repository)(nil)

// ReplaceSearchTerms implements account.SearchTermRepository.
func (r *Repository) ReplaceSearchTerms(ctx context.Context, tx uow.Tx, accountID string, terms []account.SearchTerm) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteAccountSearchTerms(ctx, accountID); err != nil {
		return sqlite.Wrap(err, "delete account search terms")
	}
	for _, t := range terms {
		if err := q.InsertAccountSearchTerm(ctx, accountgen.InsertAccountSearchTermParams{
			AccountID: accountID, Kind: string(t.Kind), Term: t.Term, IsLocal: sqlite.BoolToInt(t.IsLocal),
		}); err != nil {
			return sqlite.Wrap(err, "insert account search term")
		}
	}
	return nil
}

// DeleteAllSearchTerms implements account.SearchTermRepository.
func (r *Repository) DeleteAllSearchTerms(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAllAccountSearchTerms(ctx), "delete account search terms")
}

// ListAccountIDsWithUsernameTerm implements account.SearchTermRepository.
func (r *Repository) ListAccountIDsWithUsernameTerm(ctx context.Context, tx uow.Tx, term string) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListAccountIDsWithUsernameTerm(ctx, term)
	return ids, sqlite.Wrap(err, "list accounts with a username term")
}

// ListHandleAccountIDs implements account.SearchTermRepository.
func (r *Repository) ListHandleAccountIDs(ctx context.Context, tx uow.Tx) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListHandleAccountIDs(ctx)
	return ids, sqlite.Wrap(err, "list accounts with a handle")
}

// ListSearchTerms implements account.SearchTermRepository.
func (r *Repository) ListSearchTerms(ctx context.Context) ([]account.SearchTerm, error) {
	rows, err := r.db.reader().ListAccountSearchTerms(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list account search terms")
	}
	out := make([]account.SearchTerm, len(rows))
	for i, row := range rows {
		out[i] = account.SearchTerm{AccountID: row.AccountID, Kind: account.SearchTermKind(row.Kind), Term: row.Term, IsLocal: row.IsLocal != 0}
	}
	return out, nil
}

// SearchTerms implements account.SearchTermRepository.
func (r *Repository) SearchTerms(ctx context.Context, low, high string, limit int) ([]account.SearchTerm, error) {
	rows, err := r.db.reader().SearchAccountTerms(ctx, accountgen.SearchAccountTermsParams{Low: low, High: high, Lim: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "search account terms")
	}
	out := make([]account.SearchTerm, len(rows))
	for i, row := range rows {
		out[i] = account.SearchTerm{
			AccountID: row.AccountID, Kind: account.SearchTermKind(row.Kind), Term: row.Term, IsLocal: row.IsLocal != 0, Domain: row.Domain,
		}
	}
	return out, nil
}
