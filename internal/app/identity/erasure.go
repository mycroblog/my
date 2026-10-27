// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
)

// ErasureRepository is the part of the identity module's persistence that the erasure of
// deleted accounts uses (data-model §8). The database adapters' identity repositories implement
// it and pass testkit.AccountErasureContract.
type ErasureRepository interface {
	// EraseUserData deletes the user's email addresses with their verifications and the primary
	// address, password credentials, one-time tokens with their redemptions, sessions with their
	// revocations and activity, authorization codes and access tokens with their redemptions and
	// revocations, preferences, the choices to show role badges, timeline languages,
	// registration application with its decisions and state, and the links to external
	// identities with their unlinks and the external identities that no other user linked. The user, the origins of its sessions and registration (erased after
	// their retention, ADR-0038), its invites and role grants stay.
	EraseUserData(ctx context.Context, tx uow.Tx, userID string) error
}

// Eraser erases the identity data of a deleted local account's user (data-model §8). The rows
// of one user do not grow with other users, so one transaction erases them all.
type Eraser struct {
	uow     uow.UnitOfWork
	erasure ErasureRepository
}

// NewEraser returns the identity module's eraser.
func NewEraser(u uow.UnitOfWork, r ErasureRepository) *Eraser {
	return &Eraser{uow: u, erasure: r}
}

// EraseAccountData implements account.DataEraser.
func (e *Eraser) EraseAccountData(ctx context.Context, t account.ErasureTarget, _ string, _ int) (account.ErasureStep, error) {
	if t.UserID == "" {
		return account.ErasureStep{Done: true}, nil
	}
	err := e.uow.Tx(ctx, func(tx uow.Tx) error { return e.erasure.EraseUserData(ctx, tx, t.UserID) })
	return account.ErasureStep{Done: true}, err
}
