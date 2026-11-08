// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/accountgen"
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
	failures, err := postgres.ToInt32(h.ConsecutiveFetchFailures, "consecutive fetch failures")
	if err != nil {
		return err
	}
	err = q.UpsertRemoteAccountHead(ctx, accountgen.UpsertRemoteAccountHeadParams{
		AccountID: h.AccountID, SnapshotID: h.SnapshotID, InboxUrl: h.InboxURL,
		SharedInboxUrl: postgres.ToNullString(h.SharedInboxURL), LastFetchedAt: h.LastFetchedAt,
		ConsecutiveFetchFailures: failures,
	})
	return postgres.Wrap(err, "upsert remote account head")
}

// FindRemoteHead implements account.Repository.
func (r *Repository) FindRemoteHead(ctx context.Context, accountID string) (account.RemoteHead, error) {
	row, err := r.db.reader().GetRemoteAccountHead(ctx, accountID)
	if err != nil {
		return account.RemoteHead{}, postgres.NotFound(err, appaccount.ErrNotFound, "remote head of "+accountID)
	}
	return remoteHeadFrom(row), nil
}

func keyFrom(row accountgen.AccountKey) account.Key {
	return account.Key{
		ID: row.ID, AccountID: row.AccountID, KeyID: row.KeyID, Algorithm: account.KeyAlgorithm(row.Algorithm),
		PublicKeyPEM: row.PublicKeyPem, Fingerprint: row.Fingerprint, CreatedAt: postgres.UTC(row.CreatedAt),
	}
}

func remoteHeadFrom(row accountgen.RemoteAccountHead) account.RemoteHead {
	return account.RemoteHead{
		AccountID: row.AccountID, SnapshotID: row.SnapshotID, InboxURL: row.InboxUrl,
		SharedInboxURL: postgres.FromNullString(row.SharedInboxUrl), LastFetchedAt: postgres.UTC(row.LastFetchedAt),
		ConsecutiveFetchFailures: int(row.ConsecutiveFetchFailures),
	}
}

func handleVerificationFrom(row accountgen.RemoteAccountHandleVerification) account.HandleVerification {
	return account.HandleVerification{
		ID: row.ID, AccountID: row.AccountID, Username: row.Username, Domain: row.Domain,
		Outcome: account.HandleOutcome(row.Outcome), CheckedAt: postgres.UTC(row.CheckedAt),
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
		return account.Account{}, postgres.NotFound(err, appaccount.ErrNotFound, "account "+uri)
	}
	return accountFrom(row), nil
}

// InsertRemoteAccount implements account.Repository.
func (r *Repository) InsertRemoteAccount(ctx context.Context, tx uow.Tx, ra account.RemoteAccount) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertRemoteAccount(ctx, accountgen.InsertRemoteAccountParams{AccountID: ra.AccountID, FirstSeenAt: ra.FirstSeenAt})
	return n > 0, postgres.Wrap(err, "insert remote account")
}

// LatestRemoteSnapshot implements account.Repository.
func (r *Repository) LatestRemoteSnapshot(ctx context.Context, tx uow.Tx, accountID string) (account.RemoteSnapshot, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.RemoteSnapshot{}, err
	}
	row, err := q.GetLatestRemoteAccountSnapshot(ctx, accountID)
	if err != nil {
		return account.RemoteSnapshot{}, postgres.NotFound(err, appaccount.ErrNotFound, "snapshot of "+accountID)
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
		return nil, postgres.NotFound(err, appaccount.ErrNotFound, "remote profile revision of "+accountID)
	}
	return digest, nil
}

// FindLatestRemoteSnapshot implements account.Repository.
func (r *Repository) FindLatestRemoteSnapshot(ctx context.Context, accountID string) (account.RemoteSnapshot, error) {
	row, err := r.db.reader().GetLatestRemoteAccountSnapshot(ctx, accountID)
	if err != nil {
		return account.RemoteSnapshot{}, postgres.NotFound(err, appaccount.ErrNotFound, "snapshot of "+accountID)
	}
	return remoteSnapshotFrom(row), nil
}

func remoteSnapshotFrom(row accountgen.RemoteAccountSnapshot) account.RemoteSnapshot {
	return account.RemoteSnapshot{
		ID: row.ID, AccountID: row.AccountID, PreferredUsername: row.PreferredUsername,
		ProfileURL: postgres.FromNullString(row.ProfileUrl), InboxURL: row.InboxUrl, SharedInboxURL: postgres.FromNullString(row.SharedInboxUrl),
		OutboxURL: postgres.FromNullString(row.OutboxUrl), FollowersURL: postgres.FromNullString(row.FollowersUrl), FollowingURL: postgres.FromNullString(row.FollowingUrl),
		FeaturedURL: postgres.FromNullString(row.FeaturedUrl), FollowersTotal: row.FollowersTotal,
		FollowingTotal: row.FollowingTotal, StatusesTotal: row.StatusesTotal,
		DocumentDigest: row.DocumentDigest, FetchedAt: postgres.UTC(row.FetchedAt),
	}
}

// ListRemoteProfileURLs implements account.Repository.
func (r *Repository) ListRemoteProfileURLs(ctx context.Context, accountIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListRemoteProfileURLs(ctx, accountIDs)
	if err != nil {
		return nil, postgres.Wrap(err, "list remote profile urls")
	}
	for _, row := range rows {
		if u := postgres.FromNullString(row.ProfileUrl); u != "" {
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
		ProfileUrl: postgres.ToNullString(s.ProfileURL), InboxUrl: s.InboxURL, SharedInboxUrl: postgres.ToNullString(s.SharedInboxURL),
		OutboxUrl: postgres.ToNullString(s.OutboxURL), FollowersUrl: postgres.ToNullString(s.FollowersURL),
		FollowingUrl: postgres.ToNullString(s.FollowingURL), FeaturedUrl: postgres.ToNullString(s.FeaturedURL),
		FollowersTotal: s.FollowersTotal, FollowingTotal: s.FollowingTotal,
		StatusesTotal: s.StatusesTotal, DocumentDigest: s.DocumentDigest, FetchedAt: s.FetchedAt,
	})
	return postgres.Wrap(err, "insert remote account snapshot")
}

// RecordRemoteFetchFailure implements account.Repository.
func (r *Repository) RecordRemoteFetchFailure(ctx context.Context, tx uow.Tx, accountID string, at time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.RecordRemoteAccountFetchFailure(ctx, accountgen.RecordRemoteAccountFetchFailureParams{AccountID: accountID, LastFetchedAt: at})
	return n > 0, postgres.Wrap(err, "record remote fetch failure")
}

// ListStaleRemoteHeads implements account.Repository.
func (r *Repository) ListStaleRemoteHeads(ctx context.Context, before time.Time, after account.RemoteHead, limit int) ([]account.RemoteHead, error) {
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListStaleRemoteAccountHeads(ctx, accountgen.ListStaleRemoteAccountHeadsParams{
		FetchedBefore: before, AfterFetchedAt: after.LastFetchedAt, AfterAccountID: postgres.ToNullString(after.AccountID), BatchLimit: batch,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list stale remote account heads")
	}
	out := make([]account.RemoteHead, len(rows))
	for i, row := range rows {
		out[i] = remoteHeadFrom(row)
	}
	return out, nil
}

// ListKnownSharedInboxes implements account.Repository.
func (r *Repository) ListKnownSharedInboxes(ctx context.Context, after string, limit int) ([]string, error) {
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListKnownSharedInboxes(ctx, accountgen.ListKnownSharedInboxesParams{After: after, BatchLimit: batch})
	if err != nil {
		return nil, postgres.Wrap(err, "list known shared inboxes")
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			out = append(out, *row)
		}
	}
	return out, nil
}

// CountKnownSharedInboxes implements account.Repository.
func (r *Repository) CountKnownSharedInboxes(ctx context.Context) (int, error) {
	n, err := r.db.reader().CountKnownSharedInboxes(ctx)
	return int(n), postgres.Wrap(err, "count known shared inboxes")
}

// LatestHandleVerification implements account.Repository.
func (r *Repository) LatestHandleVerification(ctx context.Context, tx uow.Tx, accountID string) (account.HandleVerification, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.HandleVerification{}, err
	}
	row, err := q.GetLatestRemoteAccountHandleVerification(ctx, accountID)
	if err != nil {
		return account.HandleVerification{}, postgres.NotFound(err, appaccount.ErrNotFound, "handle check of "+accountID)
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
		Outcome: string(v.Outcome), CheckedAt: v.CheckedAt,
	})
	return postgres.Wrap(err, "insert handle verification")
}

// ListLatestHandleVerifications implements account.Repository.
func (r *Repository) ListLatestHandleVerifications(ctx context.Context, tx uow.Tx) ([]account.HandleVerification, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLatestRemoteAccountHandleVerifications(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "list handle verifications")
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
	return postgres.Wrap(q.DeleteAccountHandle(ctx, accountID), "delete account handle")
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
	return postgres.Wrap(err, "delete other account handle")
}

// KeyByFingerprint implements account.Repository.
func (r *Repository) KeyByFingerprint(ctx context.Context, tx uow.Tx, keyID string, fingerprint []byte) (account.Key, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.Key{}, err
	}
	row, err := q.GetAccountKeyByKeyIDAndFingerprint(ctx, accountgen.GetAccountKeyByKeyIDAndFingerprintParams{KeyID: keyID, Fingerprint: fingerprint})
	if err != nil {
		return account.Key{}, postgres.NotFound(err, appaccount.ErrNotFound, "key "+keyID)
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
		return account.Key{}, postgres.NotFound(err, appaccount.ErrNotFound, "key "+keyID)
	}
	return keyFrom(row), nil
}

// FindCurrentKey implements account.Repository.
func (r *Repository) FindCurrentKey(ctx context.Context, keyID string) (account.Key, error) {
	row, err := r.db.reader().GetCurrentAccountKey(ctx, keyID)
	if err != nil {
		return account.Key{}, postgres.NotFound(err, appaccount.ErrNotFound, "key "+keyID)
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
		return nil, postgres.Wrap(err, "list active account keys")
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
	n, err := q.InsertAccountKeyRevocation(ctx, accountgen.InsertAccountKeyRevocationParams{AccountKeyID: rev.AccountKeyID, RevokedAt: rev.RevokedAt})
	return n > 0, postgres.Wrap(err, "insert key revocation")
}
