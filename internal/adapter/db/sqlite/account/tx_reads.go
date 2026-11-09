// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// The reads in a transaction (the TxReads of the repositories): each runs the read of the same
// name on a repository over a copy of the database whose reads run in the transaction.

// FindAccountTx implements the reads in a transaction.
func (r *Repository) FindAccountTx(ctx context.Context, tx uow.Tx, id string) (account.Account, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.Account{}, err
	}
	return (&Repository{db: db}).FindAccount(ctx, id)
}

// FindLocalAccountTx implements the reads in a transaction.
func (r *Repository) FindLocalAccountTx(ctx context.Context, tx uow.Tx, accountID string) (account.LocalAccount, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.LocalAccount{}, err
	}
	return (&Repository{db: db}).FindLocalAccount(ctx, accountID)
}

// FindLocalAccountByUserTx implements the reads in a transaction.
func (r *Repository) FindLocalAccountByUserTx(ctx context.Context, tx uow.Tx, userID string) (account.LocalAccount, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.LocalAccount{}, err
	}
	return (&Repository{db: db}).FindLocalAccountByUser(ctx, userID)
}

// FindHandleOfTx implements the reads in a transaction.
func (r *Repository) FindHandleOfTx(ctx context.Context, tx uow.Tx, accountID string) (account.Handle, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.Handle{}, err
	}
	return (&Repository{db: db}).FindHandleOf(ctx, accountID)
}

// FindCurrentProfileTx implements the reads in a transaction.
func (r *Repository) FindCurrentProfileTx(ctx context.Context, tx uow.Tx, accountID string) (account.ProfileRevision, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.ProfileRevision{}, err
	}
	return (&Repository{db: db}).FindCurrentProfile(ctx, accountID)
}

// FindLifecycleStateTx implements the reads in a transaction.
func (r *Repository) FindLifecycleStateTx(ctx context.Context, tx uow.Tx, accountID string) (account.LifecycleState, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.LifecycleState{}, err
	}
	return (&Repository{db: db}).FindLifecycleState(ctx, accountID)
}

// ListKeysTx implements the reads in a transaction.
func (r *Repository) ListKeysTx(ctx context.Context, tx uow.Tx, accountID string) ([]account.Key, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).ListKeys(ctx, accountID)
}

// ListRemoteProfileURLsTx implements the reads in a transaction.
func (r *Repository) ListRemoteProfileURLsTx(ctx context.Context, tx uow.Tx, accountIDs []string) (map[string]string, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).ListRemoteProfileURLs(ctx, accountIDs)
}

// FindInstanceActorTx implements the reads in a transaction.
func (r *Repository) FindInstanceActorTx(ctx context.Context, tx uow.Tx) (account.Account, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.Account{}, err
	}
	return (&Repository{db: db}).FindInstanceActor(ctx)
}

// FindRemoteHeadTx implements the reads in a transaction.
func (r *Repository) FindRemoteHeadTx(ctx context.Context, tx uow.Tx, accountID string) (account.RemoteHead, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.RemoteHead{}, err
	}
	return (&Repository{db: db}).FindRemoteHead(ctx, accountID)
}

// HasAliasTx implements the reads in a transaction.
func (r *Repository) HasAliasTx(ctx context.Context, tx uow.Tx, accountID, uri string) (bool, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return false, err
	}
	return (&Repository{db: db}).HasAlias(ctx, accountID, uri)
}

// FindMoveTx implements the reads in a transaction.
func (r *Repository) FindMoveTx(ctx context.Context, tx uow.Tx, id string) (account.Move, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.Move{}, err
	}
	return (&Repository{db: db}).FindMove(ctx, id)
}

// CountLocalAccountsTx implements the reads in a transaction.
func (r *Repository) CountLocalAccountsTx(ctx context.Context, tx uow.Tx) (int, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return 0, err
	}
	return (&Repository{db: db}).CountLocalAccounts(ctx)
}

// FindAccountsTx implements the reads in a transaction.
func (r *Repository) FindAccountsTx(ctx context.Context, tx uow.Tx, ids []string) (map[string]account.Account, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).FindAccounts(ctx, ids)
}

// FindLifecycleStatesTx implements the reads in a transaction.
func (r *Repository) FindLifecycleStatesTx(ctx context.Context, tx uow.Tx, ids []string) (map[string]account.LifecycleState, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).FindLifecycleStates(ctx, ids)
}
