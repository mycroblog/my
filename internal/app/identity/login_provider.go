// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

var (
	// ErrExternalIdentityLinked reports that an external identity is linked to a user already.
	ErrExternalIdentityLinked = errors.New("identity: the external identity is linked to a user")
	// ErrLoginProviderLinked reports that a user has a link to the provider already: a user
	// links at most one account per provider.
	ErrLoginProviderLinked = errors.New("identity: the user has a link to the login provider")
)

// LoginProviderRepository is the part of Repository that keeps the external login providers and
// the external identities that users link (data-model §4.1, ADR-0106 §2). The database adapters
// pass testkit.LoginProviderRepositoryContract.
//
// A provider is inserted with its first revision, secret, and state change, and its head, in one
// transaction.
type LoginProviderRepository interface {
	InsertLoginProvider(ctx context.Context, tx uow.Tx, id, createdByUserID string, createdAt time.Time) error
	// InsertLoginProviderRevision records a revision with its claims and Organizations.
	InsertLoginProviderRevision(ctx context.Context, tx uow.Tx, r identity.LoginProviderRevision) error
	InsertLoginProviderSecret(ctx context.Context, tx uow.Tx, s identity.LoginProviderSecret) error
	InsertLoginProviderStateChange(ctx context.Context, tx uow.Tx, c identity.LoginProviderStateChange) error
	// UpsertLoginProviderHead sets the current revision, state, and secret of a provider.
	UpsertLoginProviderHead(ctx context.Context, tx uow.Tx, h identity.LoginProviderHead) error
	// LoginProviderHeadForUpdate returns the head of a provider in tx, locking it on the
	// databases that lock rows, so that the changes of one provider serialize; ErrNotFound for
	// an unknown provider.
	LoginProviderHeadForUpdate(ctx context.Context, tx uow.Tx, providerID string) (identity.LoginProviderHead, error)
	// FindLoginProvider returns a provider that is not deleted, with its current settings and
	// state; ErrNotFound otherwise.
	FindLoginProvider(ctx context.Context, id string) (identity.LoginProvider, error)
	// ListLoginProviders returns the providers that are not deleted, newest first.
	ListLoginProviders(ctx context.Context) ([]identity.LoginProvider, error)
	// FindLoginProviderSecret returns the current secret of a provider that is not deleted;
	// ErrNotFound otherwise.
	FindLoginProviderSecret(ctx context.Context, providerID string) (identity.LoginProviderSecret, error)

	// EnsureExternalIdentity inserts x unless an external identity with the same provider and
	// subject exists, and returns the stored one: x, or the existing one.
	EnsureExternalIdentity(ctx context.Context, tx uow.Tx, x identity.ExternalIdentity) (identity.ExternalIdentity, error)
	// InsertExternalIdentityLink records l, a link of the external identity x, and adds it to the
	// current links of its user. It fails with ErrExternalIdentityLinked when x has a current
	// link, and with ErrLoginProviderLinked when the user has a current link to x's provider.
	InsertExternalIdentityLink(ctx context.Context, tx uow.Tx, x identity.ExternalIdentity, l identity.ExternalIdentityLink) error
	// InsertExternalIdentityUnlink ends a link and removes it from the current links; it does
	// nothing for a link that ended.
	InsertExternalIdentityUnlink(ctx context.Context, tx uow.Tx, u identity.ExternalIdentityUnlink) error
	// FindLinkedExternalIdentity returns the current link of the account subject at a provider;
	// ErrNotFound when it has none.
	FindLinkedExternalIdentity(ctx context.Context, providerID, subject string) (identity.LinkedExternalIdentity, error)
	// ListLinkedExternalIdentities returns the current links of a user, oldest first.
	ListLinkedExternalIdentities(ctx context.Context, userID string) ([]identity.LinkedExternalIdentity, error)
}
