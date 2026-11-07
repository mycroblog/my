// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/accountgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// UpsertRemoteHead implements account.Repository.
func (r *Repository) UpsertRemoteHead(ctx context.Context, tx uow.Tx, h account.RemoteHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertRemoteAccountHead(ctx, accountgen.UpsertRemoteAccountHeadParams{
		AccountID: h.AccountID, SnapshotID: h.SnapshotID, InboxUrl: h.InboxURL,
		SharedInboxUrl: sqlite.ToNullString(h.SharedInboxURL), LastFetchedAt: sqlite.ToMillis(h.LastFetchedAt),
		ConsecutiveFetchFailures: int64(h.ConsecutiveFetchFailures),
	})
	return sqlite.Wrap(err, "upsert remote account head")
}

// FindRemoteHead implements account.Repository.
func (r *Repository) FindRemoteHead(ctx context.Context, accountID string) (account.RemoteHead, error) {
	row, err := r.db.reader().GetRemoteAccountHead(ctx, accountID)
	if err != nil {
		return account.RemoteHead{}, sqlite.NotFound(err, appaccount.ErrNotFound, "remote head of "+accountID)
	}
	return remoteHeadFrom(row), nil
}

func keyFrom(row accountgen.AccountKey) account.Key {
	return account.Key{
		ID: row.ID, AccountID: row.AccountID, KeyID: row.KeyID, Algorithm: account.KeyAlgorithm(row.Algorithm),
		PublicKeyPEM: row.PublicKeyPem, Fingerprint: row.Fingerprint, CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}
}

func remoteHeadFrom(row accountgen.RemoteAccountHead) account.RemoteHead {
	return account.RemoteHead{
		AccountID: row.AccountID, SnapshotID: row.SnapshotID, InboxURL: row.InboxUrl,
		SharedInboxURL: row.SharedInboxUrl.String, LastFetchedAt: sqlite.FromMillis(row.LastFetchedAt),
		ConsecutiveFetchFailures: int(row.ConsecutiveFetchFailures),
	}
}

func handleVerificationFrom(row accountgen.RemoteAccountHandleVerification) account.HandleVerification {
	return account.HandleVerification{
		ID: row.ID, AccountID: row.AccountID, Username: row.Username, Domain: row.Domain,
		Outcome: account.HandleOutcome(row.Outcome), CheckedAt: sqlite.FromMillis(row.CheckedAt),
	}
}

// AccountByURI implements account.Repository.
func (r *Repository) AccountByURI(ctx context.Context, tx uow.Tx, uri string) (account.Account, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.Account{}, err
	}
	row, err := q.GetAccountByURI(ctx, uri)
	if err != nil {
		return account.Account{}, sqlite.NotFound(err, appaccount.ErrNotFound, "account "+uri)
	}
	return accountFrom(row), nil
}

// InsertRemoteAccount implements account.Repository.
func (r *Repository) InsertRemoteAccount(ctx context.Context, tx uow.Tx, ra account.RemoteAccount) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertRemoteAccount(ctx, accountgen.InsertRemoteAccountParams{AccountID: ra.AccountID, FirstSeenAt: sqlite.ToMillis(ra.FirstSeenAt)})
	return n > 0, sqlite.Wrap(err, "insert remote account")
}

// LatestRemoteSnapshot implements account.Repository.
func (r *Repository) LatestRemoteSnapshot(ctx context.Context, tx uow.Tx, accountID string) (account.RemoteSnapshot, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.RemoteSnapshot{}, err
	}
	row, err := q.GetLatestRemoteAccountSnapshot(ctx, accountID)
	if err != nil {
		return account.RemoteSnapshot{}, sqlite.NotFound(err, appaccount.ErrNotFound, "snapshot of "+accountID)
	}
	return remoteSnapshotFrom(row), nil
}

// RemoteProfileDigest implements account.Repository.
func (r *Repository) RemoteProfileDigest(ctx context.Context, tx uow.Tx, accountID string) ([]byte, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	return remoteProfileDigest(ctx, q, accountID)
}

// FindRemoteProfileDigest implements account.Repository.
func (r *Repository) FindRemoteProfileDigest(ctx context.Context, accountID string) ([]byte, error) {
	return remoteProfileDigest(ctx, r.db.reader(), accountID)
}

func remoteProfileDigest(ctx context.Context, q *accountgen.Queries, accountID string) ([]byte, error) {
	digest, err := q.GetCurrentRemoteProfileDigest(ctx, accountID)
	if err != nil {
		return nil, sqlite.NotFound(err, appaccount.ErrNotFound, "remote profile revision of "+accountID)
	}
	return digest, nil
}

// FindLatestRemoteSnapshot implements account.Repository.
func (r *Repository) FindLatestRemoteSnapshot(ctx context.Context, accountID string) (account.RemoteSnapshot, error) {
	row, err := r.db.reader().GetLatestRemoteAccountSnapshot(ctx, accountID)
	if err != nil {
		return account.RemoteSnapshot{}, sqlite.NotFound(err, appaccount.ErrNotFound, "snapshot of "+accountID)
	}
	return remoteSnapshotFrom(row), nil
}

func remoteSnapshotFrom(row accountgen.RemoteAccountSnapshot) account.RemoteSnapshot {
	return account.RemoteSnapshot{
		ID: row.ID, AccountID: row.AccountID, PreferredUsername: row.PreferredUsername,
		ProfileURL: row.ProfileUrl.String, InboxURL: row.InboxUrl, SharedInboxURL: row.SharedInboxUrl.String,
		OutboxURL: row.OutboxUrl.String, FollowersURL: row.FollowersUrl.String, FollowingURL: row.FollowingUrl.String,
		FeaturedURL: row.FeaturedUrl.String, FollowersTotal: sqlite.FromNullInt64(row.FollowersTotal),
		FollowingTotal: sqlite.FromNullInt64(row.FollowingTotal), StatusesTotal: sqlite.FromNullInt64(row.StatusesTotal),
		DocumentDigest: row.DocumentDigest, FetchedAt: sqlite.FromMillis(row.FetchedAt),
	}
}

// ListRemoteProfileURLs implements account.Repository.
func (r *Repository) ListRemoteProfileURLs(ctx context.Context, accountIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(accountIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListRemoteProfileURLs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list remote profile urls")
	}
	for _, row := range rows {
		if u := row.ProfileUrl.String; u != "" {
			out[row.AccountID] = u
		}
	}
	return out, nil
}

// InsertRemoteSnapshot implements account.Repository.
func (r *Repository) InsertRemoteSnapshot(ctx context.Context, tx uow.Tx, s account.RemoteSnapshot) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRemoteAccountSnapshot(ctx, accountgen.InsertRemoteAccountSnapshotParams{
		ID: s.ID, AccountID: s.AccountID, PreferredUsername: s.PreferredUsername,
		ProfileUrl: sqlite.ToNullString(s.ProfileURL), InboxUrl: s.InboxURL, SharedInboxUrl: sqlite.ToNullString(s.SharedInboxURL),
		OutboxUrl: sqlite.ToNullString(s.OutboxURL), FollowersUrl: sqlite.ToNullString(s.FollowersURL),
		FollowingUrl: sqlite.ToNullString(s.FollowingURL), FeaturedUrl: sqlite.ToNullString(s.FeaturedURL),
		FollowersTotal: sqlite.ToNullInt64(s.FollowersTotal), FollowingTotal: sqlite.ToNullInt64(s.FollowingTotal),
		StatusesTotal: sqlite.ToNullInt64(s.StatusesTotal), DocumentDigest: s.DocumentDigest, FetchedAt: sqlite.ToMillis(s.FetchedAt),
	})
	return sqlite.Wrap(err, "insert remote account snapshot")
}

// RecordRemoteFetchFailure implements account.Repository.
func (r *Repository) RecordRemoteFetchFailure(ctx context.Context, tx uow.Tx, accountID string, at time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.RecordRemoteAccountFetchFailure(ctx, accountgen.RecordRemoteAccountFetchFailureParams{AccountID: accountID, LastFetchedAt: sqlite.ToMillis(at)})
	return n > 0, sqlite.Wrap(err, "record remote fetch failure")
}

// ListStaleRemoteHeads implements account.Repository.
func (r *Repository) ListStaleRemoteHeads(ctx context.Context, before time.Time, after account.RemoteHead, limit int) ([]account.RemoteHead, error) {
	rows, err := r.db.reader().ListStaleRemoteAccountHeads(ctx, accountgen.ListStaleRemoteAccountHeadsParams{
		FetchedBefore: sqlite.ToMillis(before), AfterFetchedAt: sqlite.ToMillis(after.LastFetchedAt), AfterAccountID: after.AccountID, BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list stale remote account heads")
	}
	out := make([]account.RemoteHead, len(rows))
	for i, row := range rows {
		out[i] = remoteHeadFrom(row)
	}
	return out, nil
}

// ListKnownSharedInboxes implements account.Repository.
func (r *Repository) ListKnownSharedInboxes(ctx context.Context, after string, limit int) ([]string, error) {
	rows, err := r.db.reader().ListKnownSharedInboxes(ctx, accountgen.ListKnownSharedInboxesParams{After: after, BatchLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list known shared inboxes")
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.String)
	}
	return out, nil
}

// CountKnownSharedInboxes implements account.Repository.
func (r *Repository) CountKnownSharedInboxes(ctx context.Context) (int, error) {
	n, err := r.db.reader().CountKnownSharedInboxes(ctx)
	return int(n), sqlite.Wrap(err, "count known shared inboxes")
}

// LatestHandleVerification implements account.Repository.
func (r *Repository) LatestHandleVerification(ctx context.Context, tx uow.Tx, accountID string) (account.HandleVerification, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.HandleVerification{}, err
	}
	row, err := q.GetLatestRemoteAccountHandleVerification(ctx, accountID)
	if err != nil {
		return account.HandleVerification{}, sqlite.NotFound(err, appaccount.ErrNotFound, "handle check of "+accountID)
	}
	return handleVerificationFrom(row), nil
}

// InsertHandleVerification implements account.Repository.
func (r *Repository) InsertHandleVerification(ctx context.Context, tx uow.Tx, v account.HandleVerification) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRemoteAccountHandleVerification(ctx, accountgen.InsertRemoteAccountHandleVerificationParams{
		ID: v.ID, AccountID: v.AccountID, Username: v.Username, Domain: v.Domain,
		Outcome: string(v.Outcome), CheckedAt: sqlite.ToMillis(v.CheckedAt),
	})
	return sqlite.Wrap(err, "insert handle verification")
}

// ListLatestHandleVerifications implements account.Repository.
func (r *Repository) ListLatestHandleVerifications(ctx context.Context, tx uow.Tx) ([]account.HandleVerification, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLatestRemoteAccountHandleVerifications(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list handle verifications")
	}
	out := make([]account.HandleVerification, len(rows))
	for i, row := range rows {
		out[i] = handleVerificationFrom(row)
	}
	return out, nil
}

// DeleteHandle implements account.Repository.
func (r *Repository) DeleteHandle(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAccountHandle(ctx, accountID), "delete account handle")
}

// DeleteOtherHandle implements account.Repository.
func (r *Repository) DeleteOtherHandle(ctx context.Context, tx uow.Tx, h account.Handle) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.DeleteOtherAccountHandle(ctx, accountgen.DeleteOtherAccountHandleParams{
		UsernameNormalized: h.UsernameNormalized, Domain: h.Domain, AccountID: h.AccountID,
	})
	return sqlite.Wrap(err, "delete other account handle")
}

// KeyByFingerprint implements account.Repository.
func (r *Repository) KeyByFingerprint(ctx context.Context, tx uow.Tx, keyID string, fingerprint []byte) (account.Key, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.Key{}, err
	}
	row, err := q.GetAccountKeyByKeyIDAndFingerprint(ctx, accountgen.GetAccountKeyByKeyIDAndFingerprintParams{KeyID: keyID, Fingerprint: fingerprint})
	if err != nil {
		return account.Key{}, sqlite.NotFound(err, appaccount.ErrNotFound, "key "+keyID)
	}
	return keyFrom(row), nil
}

// CurrentKey implements account.Repository.
func (r *Repository) CurrentKey(ctx context.Context, tx uow.Tx, keyID string) (account.Key, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.Key{}, err
	}
	row, err := q.GetCurrentAccountKey(ctx, keyID)
	if err != nil {
		return account.Key{}, sqlite.NotFound(err, appaccount.ErrNotFound, "key "+keyID)
	}
	return keyFrom(row), nil
}

// FindCurrentKey implements account.Repository.
func (r *Repository) FindCurrentKey(ctx context.Context, keyID string) (account.Key, error) {
	row, err := r.db.reader().GetCurrentAccountKey(ctx, keyID)
	if err != nil {
		return account.Key{}, sqlite.NotFound(err, appaccount.ErrNotFound, "key "+keyID)
	}
	return keyFrom(row), nil
}

// ListActiveKeys implements account.Repository.
func (r *Repository) ListActiveKeys(ctx context.Context, tx uow.Tx, accountID string) ([]account.Key, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListActiveAccountKeys(ctx, accountID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list active account keys")
	}
	out := make([]account.Key, len(rows))
	for i, row := range rows {
		out[i] = keyFrom(row)
	}
	return out, nil
}

// InsertKeyRevocation implements account.Repository.
func (r *Repository) InsertKeyRevocation(ctx context.Context, tx uow.Tx, rev account.KeyRevocation) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertAccountKeyRevocation(ctx, accountgen.InsertAccountKeyRevocationParams{AccountKeyID: rev.AccountKeyID, RevokedAt: sqlite.ToMillis(rev.RevokedAt)})
	return n > 0, sqlite.Wrap(err, "insert key revocation")
}
