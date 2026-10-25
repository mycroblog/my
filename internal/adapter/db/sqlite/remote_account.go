// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// UpsertRemoteHead implements account.Repository.
func (r *AccountRepository) UpsertRemoteHead(ctx context.Context, tx uow.Tx, h account.RemoteHead) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertRemoteAccountHead(ctx, sqlitegen.UpsertRemoteAccountHeadParams{
		AccountID: h.AccountID, SnapshotID: h.SnapshotID, InboxUrl: h.InboxURL,
		SharedInboxUrl: toNullString(h.SharedInboxURL), LastFetchedAt: toMillis(h.LastFetchedAt),
		ConsecutiveFetchFailures: int64(h.ConsecutiveFetchFailures),
	})
	return wrap(err, "upsert remote account head")
}

// FindRemoteHead implements account.Repository.
func (r *AccountRepository) FindRemoteHead(ctx context.Context, accountID string) (account.RemoteHead, error) {
	row, err := r.db.reader().GetRemoteAccountHead(ctx, accountID)
	if err != nil {
		return account.RemoteHead{}, notFound(err, appaccount.ErrNotFound, "remote head of "+accountID)
	}
	return remoteHeadFrom(row), nil
}

func keyFrom(row sqlitegen.AccountKey) account.Key {
	return account.Key{
		ID: row.ID, AccountID: row.AccountID, KeyID: row.KeyID, Algorithm: account.KeyAlgorithm(row.Algorithm),
		PublicKeyPEM: row.PublicKeyPem, Fingerprint: row.Fingerprint, CreatedAt: fromMillis(row.CreatedAt),
	}
}

func remoteHeadFrom(row sqlitegen.RemoteAccountHead) account.RemoteHead {
	return account.RemoteHead{
		AccountID: row.AccountID, SnapshotID: row.SnapshotID, InboxURL: row.InboxUrl,
		SharedInboxURL: row.SharedInboxUrl.String, LastFetchedAt: fromMillis(row.LastFetchedAt),
		ConsecutiveFetchFailures: int(row.ConsecutiveFetchFailures),
	}
}

func handleVerificationFrom(row sqlitegen.RemoteAccountHandleVerification) account.HandleVerification {
	return account.HandleVerification{
		ID: row.ID, AccountID: row.AccountID, Username: row.Username, Domain: row.Domain,
		Outcome: account.HandleOutcome(row.Outcome), CheckedAt: fromMillis(row.CheckedAt),
	}
}

func toNullInt64(n *int64) sql.NullInt64 {
	if n == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *n, Valid: true}
}

func fromNullInt64(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	return &n.Int64
}

// AccountByURI implements account.Repository.
func (r *AccountRepository) AccountByURI(ctx context.Context, tx uow.Tx, uri string) (account.Account, error) {
	q, err := queries(tx)
	if err != nil {
		return account.Account{}, err
	}
	row, err := q.GetAccountByURI(ctx, uri)
	if err != nil {
		return account.Account{}, notFound(err, appaccount.ErrNotFound, "account "+uri)
	}
	return accountFrom(row), nil
}

// InsertRemoteAccount implements account.Repository.
func (r *AccountRepository) InsertRemoteAccount(ctx context.Context, tx uow.Tx, ra account.RemoteAccount) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertRemoteAccount(ctx, sqlitegen.InsertRemoteAccountParams{AccountID: ra.AccountID, FirstSeenAt: toMillis(ra.FirstSeenAt)})
	return n > 0, wrap(err, "insert remote account")
}

// LatestRemoteSnapshot implements account.Repository.
func (r *AccountRepository) LatestRemoteSnapshot(ctx context.Context, tx uow.Tx, accountID string) (account.RemoteSnapshot, error) {
	q, err := queries(tx)
	if err != nil {
		return account.RemoteSnapshot{}, err
	}
	row, err := q.GetLatestRemoteAccountSnapshot(ctx, accountID)
	if err != nil {
		return account.RemoteSnapshot{}, notFound(err, appaccount.ErrNotFound, "snapshot of "+accountID)
	}
	return remoteSnapshotFrom(row), nil
}

// FindLatestRemoteSnapshot implements account.Repository.
func (r *AccountRepository) FindLatestRemoteSnapshot(ctx context.Context, accountID string) (account.RemoteSnapshot, error) {
	row, err := r.db.reader().GetLatestRemoteAccountSnapshot(ctx, accountID)
	if err != nil {
		return account.RemoteSnapshot{}, notFound(err, appaccount.ErrNotFound, "snapshot of "+accountID)
	}
	return remoteSnapshotFrom(row), nil
}

func remoteSnapshotFrom(row sqlitegen.RemoteAccountSnapshot) account.RemoteSnapshot {
	return account.RemoteSnapshot{
		ID: row.ID, AccountID: row.AccountID, PreferredUsername: row.PreferredUsername,
		ProfileURL: row.ProfileUrl.String, InboxURL: row.InboxUrl, SharedInboxURL: row.SharedInboxUrl.String,
		OutboxURL: row.OutboxUrl.String, FollowersURL: row.FollowersUrl.String, FollowingURL: row.FollowingUrl.String,
		FeaturedURL: row.FeaturedUrl.String, FollowersTotal: fromNullInt64(row.FollowersTotal),
		FollowingTotal: fromNullInt64(row.FollowingTotal), StatusesTotal: fromNullInt64(row.StatusesTotal),
		DocumentDigest: row.DocumentDigest, FetchedAt: fromMillis(row.FetchedAt),
	}
}

// ListRemoteProfileURLs implements account.Repository.
func (r *AccountRepository) ListRemoteProfileURLs(ctx context.Context, accountIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	list, err := jsonList(accountIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListRemoteProfileURLs(ctx, list)
	if err != nil {
		return nil, wrap(err, "list remote profile urls")
	}
	for _, row := range rows {
		if u := row.ProfileUrl.String; u != "" {
			out[row.AccountID] = u
		}
	}
	return out, nil
}

// InsertRemoteSnapshot implements account.Repository.
func (r *AccountRepository) InsertRemoteSnapshot(ctx context.Context, tx uow.Tx, s account.RemoteSnapshot) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRemoteAccountSnapshot(ctx, sqlitegen.InsertRemoteAccountSnapshotParams{
		ID: s.ID, AccountID: s.AccountID, PreferredUsername: s.PreferredUsername,
		ProfileUrl: toNullString(s.ProfileURL), InboxUrl: s.InboxURL, SharedInboxUrl: toNullString(s.SharedInboxURL),
		OutboxUrl: toNullString(s.OutboxURL), FollowersUrl: toNullString(s.FollowersURL),
		FollowingUrl: toNullString(s.FollowingURL), FeaturedUrl: toNullString(s.FeaturedURL),
		FollowersTotal: toNullInt64(s.FollowersTotal), FollowingTotal: toNullInt64(s.FollowingTotal),
		StatusesTotal: toNullInt64(s.StatusesTotal), DocumentDigest: s.DocumentDigest, FetchedAt: toMillis(s.FetchedAt),
	})
	return wrap(err, "insert remote account snapshot")
}

// RecordRemoteFetchFailure implements account.Repository.
func (r *AccountRepository) RecordRemoteFetchFailure(ctx context.Context, tx uow.Tx, accountID string, at time.Time) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.RecordRemoteAccountFetchFailure(ctx, sqlitegen.RecordRemoteAccountFetchFailureParams{AccountID: accountID, LastFetchedAt: toMillis(at)})
	return n > 0, wrap(err, "record remote fetch failure")
}

// ListStaleRemoteHeads implements account.Repository.
func (r *AccountRepository) ListStaleRemoteHeads(ctx context.Context, before time.Time, after account.RemoteHead, limit int) ([]account.RemoteHead, error) {
	rows, err := r.db.reader().ListStaleRemoteAccountHeads(ctx, sqlitegen.ListStaleRemoteAccountHeadsParams{
		FetchedBefore: toMillis(before), AfterFetchedAt: toMillis(after.LastFetchedAt), AfterAccountID: after.AccountID, BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list stale remote account heads")
	}
	out := make([]account.RemoteHead, len(rows))
	for i, row := range rows {
		out[i] = remoteHeadFrom(row)
	}
	return out, nil
}

// ListKnownSharedInboxes implements account.Repository.
func (r *AccountRepository) ListKnownSharedInboxes(ctx context.Context, after string, limit int) ([]string, error) {
	rows, err := r.db.reader().ListKnownSharedInboxes(ctx, sqlitegen.ListKnownSharedInboxesParams{After: after, BatchLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list known shared inboxes")
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.String)
	}
	return out, nil
}

// CountKnownSharedInboxes implements account.Repository.
func (r *AccountRepository) CountKnownSharedInboxes(ctx context.Context) (int, error) {
	n, err := r.db.reader().CountKnownSharedInboxes(ctx)
	return int(n), wrap(err, "count known shared inboxes")
}

// LatestHandleVerification implements account.Repository.
func (r *AccountRepository) LatestHandleVerification(ctx context.Context, tx uow.Tx, accountID string) (account.HandleVerification, error) {
	q, err := queries(tx)
	if err != nil {
		return account.HandleVerification{}, err
	}
	row, err := q.GetLatestRemoteAccountHandleVerification(ctx, accountID)
	if err != nil {
		return account.HandleVerification{}, notFound(err, appaccount.ErrNotFound, "handle check of "+accountID)
	}
	return handleVerificationFrom(row), nil
}

// InsertHandleVerification implements account.Repository.
func (r *AccountRepository) InsertHandleVerification(ctx context.Context, tx uow.Tx, v account.HandleVerification) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRemoteAccountHandleVerification(ctx, sqlitegen.InsertRemoteAccountHandleVerificationParams{
		ID: v.ID, AccountID: v.AccountID, Username: v.Username, Domain: v.Domain,
		Outcome: string(v.Outcome), CheckedAt: toMillis(v.CheckedAt),
	})
	return wrap(err, "insert handle verification")
}

// ListLatestHandleVerifications implements account.Repository.
func (r *AccountRepository) ListLatestHandleVerifications(ctx context.Context, tx uow.Tx) ([]account.HandleVerification, error) {
	q, err := queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLatestRemoteAccountHandleVerifications(ctx)
	if err != nil {
		return nil, wrap(err, "list handle verifications")
	}
	out := make([]account.HandleVerification, len(rows))
	for i, row := range rows {
		out[i] = handleVerificationFrom(row)
	}
	return out, nil
}

// DeleteHandle implements account.Repository.
func (r *AccountRepository) DeleteHandle(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAccountHandle(ctx, accountID), "delete account handle")
}

// DeleteOtherHandle implements account.Repository.
func (r *AccountRepository) DeleteOtherHandle(ctx context.Context, tx uow.Tx, h account.Handle) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.DeleteOtherAccountHandle(ctx, sqlitegen.DeleteOtherAccountHandleParams{
		UsernameNormalized: h.UsernameNormalized, Domain: h.Domain, AccountID: h.AccountID,
	})
	return wrap(err, "delete other account handle")
}

// KeyByFingerprint implements account.Repository.
func (r *AccountRepository) KeyByFingerprint(ctx context.Context, tx uow.Tx, keyID string, fingerprint []byte) (account.Key, error) {
	q, err := queries(tx)
	if err != nil {
		return account.Key{}, err
	}
	row, err := q.GetAccountKeyByKeyIDAndFingerprint(ctx, sqlitegen.GetAccountKeyByKeyIDAndFingerprintParams{KeyID: keyID, Fingerprint: fingerprint})
	if err != nil {
		return account.Key{}, notFound(err, appaccount.ErrNotFound, "key "+keyID)
	}
	return keyFrom(row), nil
}

// CurrentKey implements account.Repository.
func (r *AccountRepository) CurrentKey(ctx context.Context, tx uow.Tx, keyID string) (account.Key, error) {
	q, err := queries(tx)
	if err != nil {
		return account.Key{}, err
	}
	row, err := q.GetCurrentAccountKey(ctx, keyID)
	if err != nil {
		return account.Key{}, notFound(err, appaccount.ErrNotFound, "key "+keyID)
	}
	return keyFrom(row), nil
}

// FindCurrentKey implements account.Repository.
func (r *AccountRepository) FindCurrentKey(ctx context.Context, keyID string) (account.Key, error) {
	row, err := r.db.reader().GetCurrentAccountKey(ctx, keyID)
	if err != nil {
		return account.Key{}, notFound(err, appaccount.ErrNotFound, "key "+keyID)
	}
	return keyFrom(row), nil
}

// ListActiveKeys implements account.Repository.
func (r *AccountRepository) ListActiveKeys(ctx context.Context, tx uow.Tx, accountID string) ([]account.Key, error) {
	q, err := queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListActiveAccountKeys(ctx, accountID)
	if err != nil {
		return nil, wrap(err, "list active account keys")
	}
	out := make([]account.Key, len(rows))
	for i, row := range rows {
		out[i] = keyFrom(row)
	}
	return out, nil
}

// InsertKeyRevocation implements account.Repository.
func (r *AccountRepository) InsertKeyRevocation(ctx context.Context, tx uow.Tx, rev account.KeyRevocation) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertAccountKeyRevocation(ctx, sqlitegen.InsertAccountKeyRevocationParams{AccountKeyID: rev.AccountKeyID, RevokedAt: toMillis(rev.RevokedAt)})
	return n > 0, wrap(err, "insert key revocation")
}
