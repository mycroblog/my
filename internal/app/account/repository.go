// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

var (
	// ErrNotFound reports that no row matches a lookup.
	ErrNotFound = errors.New("account: not found")
	// ErrUsernameTaken reports a local username that another local account already has,
	// compared case-insensitively.
	ErrUsernameTaken = errors.New("account: username is taken")
	// ErrInstanceActorExists reports a second instance actor: there is only one local
	// Application actor.
	ErrInstanceActorExists = errors.New("account: the instance actor already exists")
	// ErrNotRemote reports a remote actor's document whose id is a local account's.
	ErrNotRemote = errors.New("account: not a remote account")
)

// Repository is the persistence port of the account module (data-model §4.2). The database
// adapters implement it and pass testkit.AccountRepositoryContract.
//
// Methods that take a uow.Tx run inside that transaction; the others read committed data outside
// any transaction. Lookups that find nothing return an error wrapping ErrNotFound.
type Repository interface {
	TxReads

	// Resources and events, written by commands.

	// InsertAccount inserts an account. A second local Application actor fails with
	// ErrInstanceActorExists.
	InsertAccount(ctx context.Context, tx uow.Tx, a account.Account) error
	// InsertLocalAccount inserts the local subtype of an account. A username whose normalized
	// form is taken fails with ErrUsernameTaken.
	InsertLocalAccount(ctx context.Context, tx uow.Tx, la account.LocalAccount) error
	// InsertProfileRevision inserts a profile revision and its fields.
	InsertProfileRevision(ctx context.Context, tx uow.Tx, r account.ProfileRevision) error
	// InsertKey inserts a public key and, when sealedPrivateKey is not nil, the sealed private
	// key of a local actor.
	InsertKey(ctx context.Context, tx uow.Tx, k account.Key, sealedPrivateKey []byte) error
	// InsertLifecycleEvent inserts a lifecycle event.
	InsertLifecycleEvent(ctx context.Context, tx uow.Tx, e account.LifecycleEvent) error

	// Projections, written only by this module's projectors and rebuild.

	UpsertHandle(ctx context.Context, tx uow.Tx, h account.Handle) error
	DeleteAllHandles(ctx context.Context, tx uow.Tx) error
	// ProfileHead returns the head of an account with the Seq of the revision it points at.
	ProfileHead(ctx context.Context, tx uow.Tx, accountID string) (account.ProfileHead, error)
	UpsertProfileHead(ctx context.Context, tx uow.Tx, h account.ProfileHead) error
	DeleteAllProfileHeads(ctx context.Context, tx uow.Tx) error
	// LifecycleState returns the lifecycle state of an account inside tx, locking its row where
	// the database can, so that two transactions that change the lifecycle (the deletion job and
	// a cancellation) do not both act on the state they read.
	LifecycleState(ctx context.Context, tx uow.Tx, accountID string) (account.LifecycleState, error)
	UpsertLifecycleState(ctx context.Context, tx uow.Tx, s account.LifecycleState) error
	DeleteAllLifecycleStates(ctx context.Context, tx uow.Tx) error
	// UpsertRemoteHead points a remote actor's head at its newest snapshot.
	UpsertRemoteHead(ctx context.Context, tx uow.Tx, h account.RemoteHead) error
	DeleteAllRemoteHeads(ctx context.Context, tx uow.Tx) error
	// ListRemoteHeads returns every remote actor's head, ordered by account ID, for keeping
	// their fetch state across a rebuild.
	ListRemoteHeads(ctx context.Context, tx uow.Tx) ([]account.RemoteHead, error)
	// ListLatestRemoteSnapshotHeads returns, for every remote actor with a snapshot, the head
	// that its newest snapshot makes, last fetched when the snapshot was and with no failures,
	// ordered by account ID.
	ListLatestRemoteSnapshotHeads(ctx context.Context, tx uow.Tx) ([]account.RemoteHead, error)

	// Projection sources, in a stable order, for rebuilding the projections.

	// ListLocalAccounts returns every local account, ordered by account ID.
	ListLocalAccounts(ctx context.Context, tx uow.Tx) ([]account.LocalAccount, error)
	// ListProfileRevisionSeqs returns every profile revision as a head candidate
	// (account ID, revision ID, Seq), ordered by account ID and Seq.
	ListProfileRevisionSeqs(ctx context.Context, tx uow.Tx) ([]account.ProfileHead, error)
	// ListLifecycleEvents returns every lifecycle event, ordered by account ID and event ID.
	ListLifecycleEvents(ctx context.Context, tx uow.Tx) ([]account.LifecycleEvent, error)

	// Projection contents, ordered by account ID, for verifying the projections.

	ListHandles(ctx context.Context) ([]account.Handle, error)
	ListProfileHeads(ctx context.Context) ([]account.ProfileHead, error)
	ListLifecycleStates(ctx context.Context) ([]account.LifecycleState, error)

	// Lookups.

	FindAccount(ctx context.Context, id string) (account.Account, error)
	// Account is FindAccount inside tx.
	Account(ctx context.Context, tx uow.Tx, id string) (account.Account, error)
	// FindAccountByURI returns the account with an ActivityPub id.
	FindAccountByURI(ctx context.Context, uri string) (account.Account, error)
	// AnyAccountOnDomain reports whether one of the accounts ids is on domain.
	AnyAccountOnDomain(ctx context.Context, domain string, ids []string) (bool, error)
	// ExistingAccountIDs returns, in ID order, the accounts among ids that exist.
	ExistingAccountIDs(ctx context.Context, ids []string) ([]string, error)
	// AccountsOnDomain returns inside tx, in ID order, the accounts among ids that are on domain
	// or one of its subdomains.
	AccountsOnDomain(ctx context.Context, tx uow.Tx, domain string, ids []string) ([]string, error)
	// ListLocalAccountIDs returns up to limit local account IDs in order, after the ID after (""
	// for the first page).
	ListLocalAccountIDs(ctx context.Context, after string, limit int) ([]string, error)
	// FindInstanceActor returns the local Application actor.
	FindInstanceActor(ctx context.Context) (account.Account, error)
	FindLocalAccount(ctx context.Context, accountID string) (account.LocalAccount, error)
	// FindLocalAccountByUser returns the local account a user logs in as.
	FindLocalAccountByUser(ctx context.Context, userID string) (account.LocalAccount, error)
	// FindHandle resolves a normalized username and a domain (account.LocalDomain for local
	// accounts) through the account_handles projection.
	FindHandle(ctx context.Context, usernameNormalized, domain string) (account.Handle, error)
	FindHandleOf(ctx context.Context, accountID string) (account.Handle, error)
	// FindCurrentProfile returns the revision that the account's profile head points at, with its fields.
	FindCurrentProfile(ctx context.Context, accountID string) (account.ProfileRevision, error)
	// FindLifecycleState returns the projected state; ErrNotFound means the account is active.
	FindLifecycleState(ctx context.Context, accountID string) (account.LifecycleState, error)
	// ListDueDeletions returns up to limit lifecycle states in deletion_scheduled whose
	// deletion is due at or before now, earliest due first.
	ListDueDeletions(ctx context.Context, now time.Time, limit int) ([]account.LifecycleState, error)
	// FindRemoteHead returns the head of a remote actor; ErrNotFound means the account is local
	// or has never been fetched.
	FindRemoteHead(ctx context.Context, accountID string) (account.RemoteHead, error)
	// ListKeys returns an account's public keys, oldest first.
	ListKeys(ctx context.Context, accountID string) ([]account.Key, error)
	// FindSealedPrivateKey returns the sealed private key of a local actor's key.
	FindSealedPrivateKey(ctx context.Context, accountKeyID string) ([]byte, error)
	// IsMediaReferenced reports whether any profile revision uses a media file as its avatar
	// or header.
	IsMediaReferenced(ctx context.Context, mediaID string) (bool, error)
	// IsEmojiReferenced reports whether any profile revision, current or not, uses a custom
	// emoji.
	IsEmojiReferenced(ctx context.Context, emojiID string) (bool, error)
	// CountLocalAccounts counts the local accounts of users (not the instance actor) that are
	// not deleted.
	CountLocalAccounts(ctx context.Context) (int, error)

	RemoteRepository
	MoveRepository
	AliasRepository
	SearchRepository
	SearchTermRepository
}

// TxReads are the reads of Repository that run in a transaction tx, with what tx has written, for the
// use cases that present what an event's transaction wrote (the data of the webhook messages,
// ADR-0060). Each one reads as the method of the same name without Tx does.
type TxReads interface {
	FindAccountTx(ctx context.Context, tx uow.Tx, id string) (account.Account, error)
	FindLocalAccountTx(ctx context.Context, tx uow.Tx, accountID string) (account.LocalAccount, error)
	FindLocalAccountByUserTx(ctx context.Context, tx uow.Tx, userID string) (account.LocalAccount, error)
	FindHandleOfTx(ctx context.Context, tx uow.Tx, accountID string) (account.Handle, error)
	FindCurrentProfileTx(ctx context.Context, tx uow.Tx, accountID string) (account.ProfileRevision, error)
	FindLifecycleStateTx(ctx context.Context, tx uow.Tx, accountID string) (account.LifecycleState, error)
	ListKeysTx(ctx context.Context, tx uow.Tx, accountID string) ([]account.Key, error)
	ListRemoteProfileURLsTx(ctx context.Context, tx uow.Tx, accountIDs []string) (map[string]string, error)
}
