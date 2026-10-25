// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// RemoteRepository is the part of Repository about remote actors: their subtype rows,
// snapshots, handle checks, and the rotation of their keys (data-model §4.2). Lookups that find
// nothing return an error wrapping ErrNotFound.
type RemoteRepository interface {
	// AccountByURI returns the account with an ActivityPub id inside tx.
	AccountByURI(ctx context.Context, tx uow.Tx, uri string) (account.Account, error)
	// InsertRemoteAccount inserts the remote subtype of an account unless it has one, and
	// reports whether it did.
	InsertRemoteAccount(ctx context.Context, tx uow.Tx, ra account.RemoteAccount) (bool, error)
	// LatestRemoteSnapshot returns the newest snapshot of a remote actor.
	LatestRemoteSnapshot(ctx context.Context, tx uow.Tx, accountID string) (account.RemoteSnapshot, error)
	// FindLatestRemoteSnapshot is LatestRemoteSnapshot outside any transaction.
	FindLatestRemoteSnapshot(ctx context.Context, accountID string) (account.RemoteSnapshot, error)
	InsertRemoteSnapshot(ctx context.Context, tx uow.Tx, s account.RemoteSnapshot) error
	// ListRemoteProfileURLs returns the profile page of the current snapshot (the head's) of each
	// remote actor in accountIDs, keyed by account ID; actors without one are absent.
	ListRemoteProfileURLs(ctx context.Context, accountIDs []string) (map[string]string, error)
	// RecordRemoteFetchFailure counts a failed fetch in the actor's head and sets its
	// LastFetchedAt to at; it reports whether the actor has a head.
	RecordRemoteFetchFailure(ctx context.Context, tx uow.Tx, accountID string, at time.Time) (bool, error)
	// ListStaleRemoteHeads returns at most limit heads last fetched before before, ordered by
	// LastFetchedAt and AccountID, starting after the head after (the zero value for the first).
	ListStaleRemoteHeads(ctx context.Context, before time.Time, after account.RemoteHead, limit int) ([]account.RemoteHead, error)
	// ListKnownSharedInboxes returns at most limit distinct shared inboxes of the heads of
	// remote actors that are not deleted, ordered, after the shared inbox after.
	ListKnownSharedInboxes(ctx context.Context, after string, limit int) ([]string, error)
	// CountKnownSharedInboxes counts the shared inboxes of ListKnownSharedInboxes.
	CountKnownSharedInboxes(ctx context.Context) (int, error)

	// LatestHandleVerification returns the newest handle check of a remote actor.
	LatestHandleVerification(ctx context.Context, tx uow.Tx, accountID string) (account.HandleVerification, error)
	InsertHandleVerification(ctx context.Context, tx uow.Tx, v account.HandleVerification) error
	// ListLatestHandleVerifications returns the newest handle check of every remote actor,
	// ordered by check ID, for rebuilding account_handles.
	ListLatestHandleVerifications(ctx context.Context, tx uow.Tx) ([]account.HandleVerification, error)
	// DeleteHandle removes the handle of an account from the account_handles projection.
	DeleteHandle(ctx context.Context, tx uow.Tx, accountID string) error
	// DeleteOtherHandle removes the handle with h's name and domain unless h.AccountID holds it.
	DeleteOtherHandle(ctx context.Context, tx uow.Tx, h account.Handle) error

	// CurrentProfile is FindCurrentProfile inside tx.
	CurrentProfile(ctx context.Context, tx uow.Tx, accountID string) (account.ProfileRevision, error)

	// KeyByFingerprint returns the key with an IRI and a fingerprint, revoked or not.
	KeyByFingerprint(ctx context.Context, tx uow.Tx, keyID string, fingerprint []byte) (account.Key, error)
	// CurrentKey returns the newest key with an IRI that has no revocation.
	CurrentKey(ctx context.Context, tx uow.Tx, keyID string) (account.Key, error)
	// FindCurrentKey is CurrentKey outside any transaction.
	FindCurrentKey(ctx context.Context, keyID string) (account.Key, error)
	// ListActiveKeys returns the keys of an account that have no revocation, oldest first.
	ListActiveKeys(ctx context.Context, tx uow.Tx, accountID string) ([]account.Key, error)
	// InsertKeyRevocation revokes a key unless it is revoked, and reports whether it did.
	InsertKeyRevocation(ctx context.Context, tx uow.Tx, r account.KeyRevocation) (bool, error)
}
