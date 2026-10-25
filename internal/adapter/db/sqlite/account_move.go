// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
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
	n, err := q.InsertAccountMove(ctx, sqlitegen.InsertAccountMoveParams{
		ID: m.ID, AccountID: m.AccountID, TargetAccountID: m.TargetAccountID, InitiatedBy: string(m.InitiatedBy),
		ActivityUri: toNullString(m.ActivityURI), OccurredAt: toMillis(m.OccurredAt),
	})
	return n > 0, wrap(err, "insert account move")
}

func moveFrom(row sqlitegen.AccountMove) account.Move {
	return account.Move{
		ID: row.ID, AccountID: row.AccountID, TargetAccountID: row.TargetAccountID,
		InitiatedBy: account.MoveInitiator(row.InitiatedBy), ActivityURI: row.ActivityUri.String, OccurredAt: fromMillis(row.OccurredAt),
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
	err = q.UpsertAccountMoveState(ctx, sqlitegen.UpsertAccountMoveStateParams{
		AccountID: s.AccountID, MoveID: s.MoveID, TargetAccountID: s.TargetAccountID, Since: toMillis(s.Since),
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

func moveStateFrom(row sqlitegen.AccountMoveState) account.MoveState {
	return account.MoveState{AccountID: row.AccountID, MoveID: row.MoveID, TargetAccountID: row.TargetAccountID, Since: fromMillis(row.Since)}
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
			Move: moveFrom(sqlitegen.AccountMove{
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

// FindMoveStateTx implements account.Repository.
func (r *AccountRepository) FindMoveStateTx(ctx context.Context, tx uow.Tx, accountID string) (account.MoveState, error) {
	q, err := queries(tx)
	if err != nil {
		return account.MoveState{}, err
	}
	row, err := q.GetAccountMoveState(ctx, accountID)
	if err != nil {
		return account.MoveState{}, notFound(err, appaccount.ErrNotFound, "move state of "+accountID)
	}
	return moveStateFrom(row), nil
}

// InsertMoveCancellation implements account.Repository.
func (r *AccountRepository) InsertMoveCancellation(ctx context.Context, tx uow.Tx, c account.MoveCancellation) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertAccountMoveCancellation(ctx, sqlitegen.InsertAccountMoveCancellationParams{MoveID: c.MoveID, CancelledAt: toMillis(c.CancelledAt)})
	return n > 0, wrap(err, "insert account move cancellation")
}

// DeleteMoveState implements account.Repository.
func (r *AccountRepository) DeleteMoveState(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAccountMoveState(ctx, accountID), "delete account move state")
}
