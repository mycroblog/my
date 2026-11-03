// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
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
	// ListOwnedOAuthClientIDs returns, in tx, the clients that the user registered.
	ListOwnedOAuthClientIDs(ctx context.Context, tx uow.Tx, userID string) ([]string, error)
	// BlankOwnedOAuthClientDetails removes the names and websites of the clients that the user
	// registered, with those of their revisions.
	BlankOwnedOAuthClientDetails(ctx context.Context, tx uow.Tx, userID string) error
	// InsertOAuthClientDisablement records d unless the client is disabled already.
	InsertOAuthClientDisablement(ctx context.Context, tx uow.Tx, d identity.OAuthClientDisablement) (bool, error)
	// RevokeTokensOfClient revokes the tokens of a client that still work.
	RevokeTokensOfClient(ctx context.Context, tx uow.Tx, clientID string, reason identity.TokenRevocationReason, at time.Time) error
}

// Eraser erases the identity data of a deleted local account's user (data-model §8). The rows
// of one user do not grow with other users, so one transaction erases them all. The clients that
// the user registered lose their names and websites and are disabled, which stops the tokens other
// users gave them; like any client, they keep their row.
type Eraser struct {
	uow     uow.UnitOfWork
	erasure ErasureRepository
	clock   kernel.Clock
	ids     kernel.IDGenerator
}

// NewEraser returns the identity module's eraser.
func NewEraser(u uow.UnitOfWork, r ErasureRepository, clock kernel.Clock, ids kernel.IDGenerator) *Eraser {
	return &Eraser{uow: u, erasure: r, clock: clock, ids: ids}
}

// EraseAccountData implements account.DataEraser.
func (e *Eraser) EraseAccountData(ctx context.Context, t account.ErasureTarget, _ string, _ int) (account.ErasureStep, error) {
	if t.UserID == "" {
		return account.ErasureStep{Done: true}, nil
	}
	now := e.clock.Now()
	err := e.uow.Tx(ctx, func(tx uow.Tx) error {
		if err := e.erasure.EraseUserData(ctx, tx, t.UserID); err != nil {
			return err
		}
		clients, err := e.erasure.ListOwnedOAuthClientIDs(ctx, tx, t.UserID)
		if err != nil {
			return err
		}
		for _, id := range clients {
			d := identity.OAuthClientDisablement{ID: e.ids.NewID(), ClientID: id, DisabledAt: now}
			if _, err := e.erasure.InsertOAuthClientDisablement(ctx, tx, d); err != nil {
				return err
			}
			if err := e.erasure.RevokeTokensOfClient(ctx, tx, id, identity.TokenRevokedClientDisabled, now); err != nil {
				return err
			}
		}
		return e.erasure.BlankOwnedOAuthClientDetails(ctx, tx, t.UserID)
	})
	return account.ErasureStep{Done: true}, err
}
