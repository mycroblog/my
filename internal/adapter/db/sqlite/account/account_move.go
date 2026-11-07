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

// InsertMove implements account.Repository.
func (r *Repository) InsertMove(ctx context.Context, tx uow.Tx, m account.Move) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertAccountMove(ctx, accountgen.InsertAccountMoveParams{
		ID: m.ID, AccountID: m.AccountID, TargetAccountID: m.TargetAccountID, InitiatedBy: string(m.InitiatedBy),
		ActivityUri: sqlite.ToNullString(m.ActivityURI), OccurredAt: sqlite.ToMillis(m.OccurredAt),
	})
	return n > 0, sqlite.Wrap(err, "insert account move")
}

func moveFrom(row accountgen.AccountMove) account.Move {
	return account.Move{
		ID: row.ID, AccountID: row.AccountID, TargetAccountID: row.TargetAccountID,
		InitiatedBy: account.MoveInitiator(row.InitiatedBy), ActivityURI: row.ActivityUri.String, OccurredAt: sqlite.FromMillis(row.OccurredAt),
	}
}

// LatestMove implements account.Repository.
func (r *Repository) LatestMove(ctx context.Context, tx uow.Tx, accountID string) (account.Move, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.Move{}, err
	}
	row, err := q.GetLatestAccountMove(ctx, accountID)
	if err != nil {
		return account.Move{}, sqlite.NotFound(err, appaccount.ErrNotFound, "latest move of "+accountID)
	}
	return moveFrom(row), nil
}

// FindMove implements account.Repository.
func (r *Repository) FindMove(ctx context.Context, id string) (account.Move, error) {
	row, err := r.db.reader().GetAccountMove(ctx, id)
	if err != nil {
		return account.Move{}, sqlite.NotFound(err, appaccount.ErrNotFound, "account move "+id)
	}
	return moveFrom(row), nil
}

// UpsertMoveState implements account.Repository.
func (r *Repository) UpsertMoveState(ctx context.Context, tx uow.Tx, s account.MoveState) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertAccountMoveState(ctx, accountgen.UpsertAccountMoveStateParams{
		AccountID: s.AccountID, MoveID: s.MoveID, TargetAccountID: s.TargetAccountID, Since: sqlite.ToMillis(s.Since),
	})
	return sqlite.Wrap(err, "upsert account move state")
}

// DeleteAllMoveStates implements account.Repository.
func (r *Repository) DeleteAllMoveStates(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAllAccountMoveStates(ctx), "delete account move states")
}

func moveStateFrom(row accountgen.AccountMoveState) account.MoveState {
	return account.MoveState{AccountID: row.AccountID, MoveID: row.MoveID, TargetAccountID: row.TargetAccountID, Since: sqlite.FromMillis(row.Since)}
}

// FindMoveState implements account.Repository.
func (r *Repository) FindMoveState(ctx context.Context, accountID string) (account.MoveState, error) {
	row, err := r.db.reader().GetAccountMoveState(ctx, accountID)
	if err != nil {
		return account.MoveState{}, sqlite.NotFound(err, appaccount.ErrNotFound, "move state of "+accountID)
	}
	return moveStateFrom(row), nil
}

// ListMoveRecords implements account.Repository.
func (r *Repository) ListMoveRecords(ctx context.Context, tx uow.Tx) ([]account.MoveRecord, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAccountMoveRecords(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list account moves")
	}
	out := make([]account.MoveRecord, len(rows))
	for i, row := range rows {
		out[i] = account.MoveRecord{
			Move: moveFrom(accountgen.AccountMove{
				ID: row.ID, AccountID: row.AccountID, TargetAccountID: row.TargetAccountID, InitiatedBy: row.InitiatedBy,
				ActivityUri: row.ActivityUri, OccurredAt: row.OccurredAt,
			}),
			Canceled: row.Canceled,
		}
	}
	return out, nil
}

// ListMoveStates implements account.Repository.
func (r *Repository) ListMoveStates(ctx context.Context) ([]account.MoveState, error) {
	rows, err := r.db.reader().ListAccountMoveStates(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list account move states")
	}
	out := make([]account.MoveState, len(rows))
	for i, row := range rows {
		out[i] = moveStateFrom(row)
	}
	return out, nil
}

// FindMoveStateTx implements account.Repository.
func (r *Repository) FindMoveStateTx(ctx context.Context, tx uow.Tx, accountID string) (account.MoveState, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.MoveState{}, err
	}
	row, err := q.GetAccountMoveState(ctx, accountID)
	if err != nil {
		return account.MoveState{}, sqlite.NotFound(err, appaccount.ErrNotFound, "move state of "+accountID)
	}
	return moveStateFrom(row), nil
}

// InsertMoveCancellation implements account.Repository.
func (r *Repository) InsertMoveCancellation(ctx context.Context, tx uow.Tx, c account.MoveCancellation) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertAccountMoveCancellation(ctx, accountgen.InsertAccountMoveCancellationParams{MoveID: c.MoveID, CanceledAt: sqlite.ToMillis(c.CanceledAt)})
	return n > 0, sqlite.Wrap(err, "insert account move cancellation")
}

// DeleteMoveState implements account.Repository.
func (r *Repository) DeleteMoveState(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAccountMoveState(ctx, accountID), "delete account move state")
}
