// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/accountgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// InsertAlias implements account.Repository.
func (r *Repository) InsertAlias(ctx context.Context, tx uow.Tx, a account.Alias) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertAccountAlias(ctx, accountgen.InsertAccountAliasParams{AccountID: a.AccountID, AliasUri: a.URI, AddedAt: sqlite.ToMillis(a.AddedAt)})
	return n > 0, sqlite.Wrap(err, "insert account alias")
}

// DeleteAlias implements account.Repository.
func (r *Repository) DeleteAlias(ctx context.Context, tx uow.Tx, accountID, uri string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteAccountAlias(ctx, accountgen.DeleteAccountAliasParams{AccountID: accountID, AliasUri: uri})
	return n > 0, sqlite.Wrap(err, "delete account alias")
}

// CountAliases implements account.Repository.
func (r *Repository) CountAliases(ctx context.Context, tx uow.Tx, accountID string) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountAccountAliases(ctx, accountID)
	return int(n), sqlite.Wrap(err, "count account aliases")
}

// ListAliases implements account.Repository.
func (r *Repository) ListAliases(ctx context.Context, accountID string) ([]account.Alias, error) {
	rows, err := r.db.reader().ListAccountAliases(ctx, accountID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list account aliases")
	}
	out := make([]account.Alias, len(rows))
	for i, row := range rows {
		out[i] = account.Alias{AccountID: row.AccountID, URI: row.AliasUri, AddedAt: sqlite.FromMillis(row.AddedAt)}
	}
	return out, nil
}

// HasAlias implements account.Repository.
func (r *Repository) HasAlias(ctx context.Context, accountID, uri string) (bool, error) {
	ok, err := r.db.reader().HasAccountAlias(ctx, accountgen.HasAccountAliasParams{AccountID: accountID, AliasUri: uri})
	return ok, sqlite.Wrap(err, "find account alias")
}
