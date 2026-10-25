// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// InsertAlias implements account.Repository.
func (r *AccountRepository) InsertAlias(ctx context.Context, tx uow.Tx, a account.Alias) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertAccountAlias(ctx, postgresgen.InsertAccountAliasParams{AccountID: a.AccountID, AliasUri: a.URI, AddedAt: a.AddedAt})
	return n > 0, wrap(err, "insert account alias")
}

// DeleteAlias implements account.Repository.
func (r *AccountRepository) DeleteAlias(ctx context.Context, tx uow.Tx, accountID, uri string) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteAccountAlias(ctx, postgresgen.DeleteAccountAliasParams{AccountID: accountID, AliasUri: uri})
	return n > 0, wrap(err, "delete account alias")
}

// CountAliases implements account.Repository.
func (r *AccountRepository) CountAliases(ctx context.Context, tx uow.Tx, accountID string) (int, error) {
	q, err := queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountAccountAliases(ctx, accountID)
	return int(n), wrap(err, "count account aliases")
}

// ListAliases implements account.Repository.
func (r *AccountRepository) ListAliases(ctx context.Context, accountID string) ([]account.Alias, error) {
	rows, err := r.db.reader().ListAccountAliases(ctx, accountID)
	if err != nil {
		return nil, wrap(err, "list account aliases")
	}
	out := make([]account.Alias, len(rows))
	for i, row := range rows {
		out[i] = account.Alias{AccountID: row.AccountID, URI: row.AliasUri, AddedAt: utc(row.AddedAt)}
	}
	return out, nil
}

// HasAlias implements account.Repository.
func (r *AccountRepository) HasAlias(ctx context.Context, accountID, uri string) (bool, error) {
	ok, err := r.db.reader().HasAccountAlias(ctx, postgresgen.HasAccountAliasParams{AccountID: accountID, AliasUri: uri})
	return ok, wrap(err, "find account alias")
}
