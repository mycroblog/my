// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
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
	failures, err := toInt32(h.ConsecutiveFetchFailures, "consecutive fetch failures")
	if err != nil {
		return err
	}
	err = q.UpsertRemoteAccountHead(ctx, postgresgen.UpsertRemoteAccountHeadParams{
		AccountID: h.AccountID, SnapshotID: h.SnapshotID, InboxUrl: h.InboxURL,
		SharedInboxUrl: toNullString(h.SharedInboxURL), LastFetchedAt: h.LastFetchedAt,
		ConsecutiveFetchFailures: failures,
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

func keyFrom(row postgresgen.AccountKey) account.Key {
	return account.Key{
		ID: row.ID, AccountID: row.AccountID, KeyID: row.KeyID, Algorithm: account.KeyAlgorithm(row.Algorithm),
		PublicKeyPEM: row.PublicKeyPem, Fingerprint: row.Fingerprint, CreatedAt: utc(row.CreatedAt),
	}
}

func remoteHeadFrom(row postgresgen.RemoteAccountHead) account.RemoteHead {
	return account.RemoteHead{
		AccountID: row.AccountID, SnapshotID: row.SnapshotID, InboxURL: row.InboxUrl,
		SharedInboxURL: fromNullString(row.SharedInboxUrl), LastFetchedAt: utc(row.LastFetchedAt),
		ConsecutiveFetchFailures: int(row.ConsecutiveFetchFailures),
	}
}

func handleVerificationFrom(row postgresgen.RemoteAccountHandleVerification) account.HandleVerification {
	return account.HandleVerification{
		ID: row.ID, AccountID: row.AccountID, Username: row.Username, Domain: row.Domain,
		Outcome: account.HandleOutcome(row.Outcome), CheckedAt: utc(row.CheckedAt),
	}
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
	n, err := q.InsertRemoteAccount(ctx, postgresgen.InsertRemoteAccountParams{AccountID: ra.AccountID, FirstSeenAt: ra.FirstSeenAt})
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

func remoteSnapshotFrom(row postgresgen.RemoteAccountSnapshot) account.RemoteSnapshot {
	return account.RemoteSnapshot{
		ID: row.ID, AccountID: row.AccountID, PreferredUsername: row.PreferredUsername,
		ProfileURL: fromNullString(row.ProfileUrl), InboxURL: row.InboxUrl, SharedInboxURL: fromNullString(row.SharedInboxUrl),
		OutboxURL: fromNullString(row.OutboxUrl), FollowersURL: fromNullString(row.FollowersUrl), FollowingURL: fromNullString(row.FollowingUrl),
		FeaturedURL: fromNullString(row.FeaturedUrl), FollowersTotal: row.FollowersTotal,
		FollowingTotal: row.FollowingTotal, StatusesTotal: row.StatusesTotal,
		DocumentDigest: row.DocumentDigest, FetchedAt: utc(row.FetchedAt),
	}
}

// ListRemoteProfileURLs implements account.Repository.
func (r *AccountRepository) ListRemoteProfileURLs(ctx context.Context, accountIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListRemoteProfileURLs(ctx, accountIDs)
	if err != nil {
		return nil, wrap(err, "list remote profile urls")
	}
	for _, row := range rows {
		if u := fromNullString(row.ProfileUrl); u != "" {
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
	err = q.InsertRemoteAccountSnapshot(ctx, postgresgen.InsertRemoteAccountSnapshotParams{
		ID: s.ID, AccountID: s.AccountID, PreferredUsername: s.PreferredUsername,
		ProfileUrl: toNullString(s.ProfileURL), InboxUrl: s.InboxURL, SharedInboxUrl: toNullString(s.SharedInboxURL),
		OutboxUrl: toNullString(s.OutboxURL), FollowersUrl: toNullString(s.FollowersURL),
		FollowingUrl: toNullString(s.FollowingURL), FeaturedUrl: toNullString(s.FeaturedURL),
		FollowersTotal: s.FollowersTotal, FollowingTotal: s.FollowingTotal,
		StatusesTotal: s.StatusesTotal, DocumentDigest: s.DocumentDigest, FetchedAt: s.FetchedAt,
	})
	return wrap(err, "insert remote account snapshot")
}

// RecordRemoteFetchFailure implements account.Repository.
func (r *AccountRepository) RecordRemoteFetchFailure(ctx context.Context, tx uow.Tx, accountID string, at time.Time) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.RecordRemoteAccountFetchFailure(ctx, postgresgen.RecordRemoteAccountFetchFailureParams{AccountID: accountID, LastFetchedAt: at})
	return n > 0, wrap(err, "record remote fetch failure")
}

// ListStaleRemoteHeads implements account.Repository.
func (r *AccountRepository) ListStaleRemoteHeads(ctx context.Context, before time.Time, after account.RemoteHead, limit int) ([]account.RemoteHead, error) {
	batch, err := toInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListStaleRemoteAccountHeads(ctx, postgresgen.ListStaleRemoteAccountHeadsParams{
		FetchedBefore: before, AfterFetchedAt: after.LastFetchedAt, AfterAccountID: toNullString(after.AccountID), BatchLimit: batch,
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
	batch, err := toInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListKnownSharedInboxes(ctx, postgresgen.ListKnownSharedInboxesParams{After: after, BatchLimit: batch})
	if err != nil {
		return nil, wrap(err, "list known shared inboxes")
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
	err = q.InsertRemoteAccountHandleVerification(ctx, postgresgen.InsertRemoteAccountHandleVerificationParams{
		ID: v.ID, AccountID: v.AccountID, Username: v.Username, Domain: v.Domain,
		Outcome: string(v.Outcome), CheckedAt: v.CheckedAt,
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
	err = q.DeleteOtherAccountHandle(ctx, postgresgen.DeleteOtherAccountHandleParams{
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
	row, err := q.GetAccountKeyByKeyIDAndFingerprint(ctx, postgresgen.GetAccountKeyByKeyIDAndFingerprintParams{KeyID: keyID, Fingerprint: fingerprint})
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
	n, err := q.InsertAccountKeyRevocation(ctx, postgresgen.InsertAccountKeyRevocationParams{AccountKeyID: rev.AccountKeyID, RevokedAt: rev.RevokedAt})
	return n > 0, wrap(err, "insert key revocation")
}
