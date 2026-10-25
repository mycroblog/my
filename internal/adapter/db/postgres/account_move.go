// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// InsertMove implements account.Repository.
func (r *AccountRepository) InsertMove(ctx context.Context, tx uow.Tx, m account.Move) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertAccountMove(ctx, postgresgen.InsertAccountMoveParams{
		ID: m.ID, AccountID: m.AccountID, TargetAccountID: m.TargetAccountID, InitiatedBy: string(m.InitiatedBy),
		ActivityUri: toNullString(m.ActivityURI), OccurredAt: m.OccurredAt,
	})
	return n > 0, wrap(err, "insert account move")
}

func moveFrom(row postgresgen.AccountMove) account.Move {
	return account.Move{
		ID: row.ID, AccountID: row.AccountID, TargetAccountID: row.TargetAccountID,
		InitiatedBy: account.MoveInitiator(row.InitiatedBy), ActivityURI: fromNullString(row.ActivityUri), OccurredAt: utc(row.OccurredAt),
	}
}

// LatestMove implements account.Repository.
func (r *AccountRepository) LatestMove(ctx context.Context, tx uow.Tx, accountID string) (account.Move, error) {
	q, err := queries(tx)
	if err != nil {
		return account.Move{}, err
	}
	row, err := q.GetLatestAccountMove(ctx, accountID)
	if err != nil {
		return account.Move{}, notFound(err, appaccount.ErrNotFound, "latest move of "+accountID)
	}
	return moveFrom(row), nil
}

// FindMove implements account.Repository.
func (r *AccountRepository) FindMove(ctx context.Context, id string) (account.Move, error) {
	row, err := r.db.reader().GetAccountMove(ctx, id)
	if err != nil {
		return account.Move{}, notFound(err, appaccount.ErrNotFound, "account move "+id)
	}
	return moveFrom(row), nil
}

// UpsertMoveState implements account.Repository.
func (r *AccountRepository) UpsertMoveState(ctx context.Context, tx uow.Tx, s account.MoveState) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertAccountMoveState(ctx, postgresgen.UpsertAccountMoveStateParams{
		AccountID: s.AccountID, MoveID: s.MoveID, TargetAccountID: s.TargetAccountID, Since: s.Since,
	})
	return wrap(err, "upsert account move state")
}

// DeleteAllMoveStates implements account.Repository.
func (r *AccountRepository) DeleteAllMoveStates(ctx context.Context, tx uow.Tx) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAllAccountMoveStates(ctx), "delete account move states")
}

func moveStateFrom(row postgresgen.AccountMoveState) account.MoveState {
	return account.MoveState{AccountID: row.AccountID, MoveID: row.MoveID, TargetAccountID: row.TargetAccountID, Since: utc(row.Since)}
}

// FindMoveState implements account.Repository.
func (r *AccountRepository) FindMoveState(ctx context.Context, accountID string) (account.MoveState, error) {
	row, err := r.db.reader().GetAccountMoveState(ctx, accountID)
	if err != nil {
		return account.MoveState{}, notFound(err, appaccount.ErrNotFound, "move state of "+accountID)
	}
	return moveStateFrom(row), nil
}

// ListMoveRecords implements account.Repository.
func (r *AccountRepository) ListMoveRecords(ctx context.Context, tx uow.Tx) ([]account.MoveRecord, error) {
	q, err := queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAccountMoveRecords(ctx)
	if err != nil {
		return nil, wrap(err, "list account moves")
	}
	out := make([]account.MoveRecord, len(rows))
	for i, row := range rows {
		out[i] = account.MoveRecord{
			Move: moveFrom(postgresgen.AccountMove{
				ID: row.ID, AccountID: row.AccountID, TargetAccountID: row.TargetAccountID, InitiatedBy: row.InitiatedBy,
				ActivityUri: row.ActivityUri, OccurredAt: row.OccurredAt,
			}),
			Cancelled: row.Cancelled,
		}
	}
	return out, nil
}

// ListMoveStates implements account.Repository.
func (r *AccountRepository) ListMoveStates(ctx context.Context) ([]account.MoveState, error) {
	rows, err := r.db.reader().ListAccountMoveStates(ctx)
	if err != nil {
		return nil, wrap(err, "list account move states")
	}
	out := make([]account.MoveState, len(rows))
	for i, row := range rows {
		out[i] = moveStateFrom(row)
	}
	return out, nil
}
