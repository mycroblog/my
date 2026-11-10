// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/moderationgen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

var _ appmoderation.DomainListRepository = (*Repository)(nil)

// domainListQueries returns the queries inside tx, or outside any transaction when tx is nil.
func (r *Repository) domainListQueries(tx uow.Tx) (*moderationgen.Queries, error) {
	if tx == nil {
		return r.db.reader(), nil
	}
	return r.db.queries(tx)
}

// InsertDomainListSubscription implements appmoderation.DomainListRepository.
func (r *Repository) InsertDomainListSubscription(ctx context.Context, tx uow.Tx, s moderation.DomainListSubscription) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertDomainListSubscription(ctx, moderationgen.InsertDomainListSubscriptionParams{
		ID: s.ID, Url: s.URL, Format: string(s.Format), CreatedByUserID: sqlite.ToNullString(s.CreatedByUserID), CreatedAt: sqlite.ToMillis(s.CreatedAt),
	}), "insert domain list subscription")
}

// InsertDomainListRevision implements appmoderation.DomainListRepository.
func (r *Repository) InsertDomainListRevision(ctx context.Context, tx uow.Tx, rev moderation.DomainListRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	st := rev.Settings
	return sqlite.Wrap(q.InsertDomainListSubscriptionRevision(ctx, moderationgen.InsertDomainListSubscriptionRevisionParams{
		ID: rev.ID, SubscriptionID: rev.SubscriptionID, Title: st.Title, Mode: string(st.Mode), SuspendAs: string(st.SuspendAs),
		SilenceAs: string(st.SilenceAs), ApplyRejectMedia: sqlite.BoolToInt(st.ApplyRejectMedia), ApplyRejectReports: sqlite.BoolToInt(st.ApplyRejectReports),
		PublicCommentFromList: sqlite.BoolToInt(st.PublicCommentFromList), RevisedByUserID: sqlite.ToNullString(rev.RevisedByUserID),
		RevisedAt: sqlite.ToMillis(rev.RevisedAt),
	}), "insert domain list subscription revision")
}

// InsertDomainListDeletion implements appmoderation.DomainListRepository.
func (r *Repository) InsertDomainListDeletion(ctx context.Context, tx uow.Tx, d moderation.DomainListDeletion) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertDomainListSubscriptionDeletion(ctx, moderationgen.InsertDomainListSubscriptionDeletionParams{
		SubscriptionID: d.SubscriptionID, LiftActions: sqlite.BoolToInt(d.LiftActions), DeletedByUserID: sqlite.ToNullString(d.DeletedByUserID),
		DeletedAt: sqlite.ToMillis(d.DeletedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert domain list subscription deletion")
	}
	if n == 0 {
		return fmt.Errorf("deletion of domain list subscription %s: %w", d.SubscriptionID, appmoderation.ErrAlreadyRecorded)
	}
	return nil
}

// PutDomainListHead implements appmoderation.DomainListRepository.
func (r *Repository) PutDomainListHead(ctx context.Context, tx uow.Tx, h moderation.DomainListHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertDomainListSubscriptionHead(ctx, moderationgen.UpsertDomainListSubscriptionHeadParams{
		SubscriptionID: h.SubscriptionID, Etag: sqlite.ToNullString(h.ETag), LastModified: sqlite.ToNullString(h.LastModified),
		Digest: sqlite.ToNullString(h.Digest), ConsecutiveFailures: int64(h.ConsecutiveFailures), LastFetchID: sqlite.ToNullString(h.LastFetchID),
	}), "put domain list subscription head")
}

// LockDomainListSubscription implements appmoderation.DomainListRepository: SQLite runs one
// writing transaction at a time.
func (r *Repository) LockDomainListSubscription(context.Context, uow.Tx, string) error {
	return nil
}

func domainListRecordFrom(row moderationgen.GetDomainListSubscriptionRow) moderation.DomainListSubscriptionRecord {
	rec := moderation.DomainListSubscriptionRecord{
		Subscription: moderation.DomainListSubscription{
			ID: row.ID, URL: row.Url, Format: moderation.DomainListFormat(row.Format),
			CreatedByUserID: row.CreatedByUserID.String, CreatedAt: sqlite.FromMillis(row.CreatedAt),
		},
		Revision: moderation.DomainListRevision{
			ID: row.RevisionID, SubscriptionID: row.ID, RevisedByUserID: row.RevisedByUserID.String, RevisedAt: sqlite.FromMillis(row.RevisedAt),
			Settings: moderation.DomainListSettings{
				Title: row.Title, Mode: moderation.DomainListMode(row.Mode), SuspendAs: moderation.DomainListSeverityMapping(row.SuspendAs),
				SilenceAs: moderation.DomainListSeverityMapping(row.SilenceAs), ApplyRejectMedia: row.ApplyRejectMedia != 0,
				ApplyRejectReports: row.ApplyRejectReports != 0, PublicCommentFromList: row.PublicCommentFromList != 0,
			},
		},
		Head: moderation.DomainListHead{
			SubscriptionID: row.ID, ETag: row.Etag.String, LastModified: row.LastModified.String,
			Digest: row.Digest.String, ConsecutiveFailures: int(row.ConsecutiveFailures), LastFetchID: row.LastFetchID.String,
		},
	}
	if row.LastFetchID.Valid && row.LastOutcome.Valid && row.LastFetchedAt.Valid {
		rec.LastFetch = &moderation.DomainListFetch{
			ID: row.LastFetchID.String, SubscriptionID: row.ID, Outcome: moderation.DomainListFetchOutcome(row.LastOutcome.String),
			HTTPStatus: int(row.LastHttpStatus.Int64), ErrorCode: moderation.DomainListFetchError(row.LastErrorCode.String),
			EntryCount: int(row.LastEntryCount.Int64), FetchedAt: sqlite.FromMillis(row.LastFetchedAt.Int64),
		}
	}
	if row.DeletionSubscriptionID.Valid && row.DeletedAt.Valid {
		rec.Deletion = &moderation.DomainListDeletion{
			SubscriptionID: row.ID, LiftActions: row.LiftActions.Int64 != 0,
			DeletedByUserID: row.DeletedByUserID.String, DeletedAt: sqlite.FromMillis(row.DeletedAt.Int64),
		}
	}
	return rec
}

// DomainListSubscription implements appmoderation.DomainListRepository.
func (r *Repository) DomainListSubscription(ctx context.Context, tx uow.Tx, id string) (moderation.DomainListSubscriptionRecord, error) {
	q, err := r.domainListQueries(tx)
	if err != nil {
		return moderation.DomainListSubscriptionRecord{}, err
	}
	row, err := q.GetDomainListSubscription(ctx, id)
	if err != nil {
		return moderation.DomainListSubscriptionRecord{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "domain list subscription "+id)
	}
	return domainListRecordFrom(row), nil
}

// ListDomainListSubscriptions implements appmoderation.DomainListRepository.
func (r *Repository) ListDomainListSubscriptions(ctx context.Context, before string, limit int) ([]moderation.DomainListSubscriptionRecord, error) {
	rows, err := r.db.reader().ListDomainListSubscriptions(ctx, moderationgen.ListDomainListSubscriptionsParams{Before: before, Lim: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list domain list subscriptions")
	}
	out := make([]moderation.DomainListSubscriptionRecord, len(rows))
	for i, row := range rows {
		out[i] = domainListRecordFrom(moderationgen.GetDomainListSubscriptionRow(row))
	}
	return out, nil
}

// InsertDomainListFetch implements appmoderation.DomainListRepository.
func (r *Repository) InsertDomainListFetch(ctx context.Context, tx uow.Tx, f moderation.DomainListFetch) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertDomainListFetch(ctx, moderationgen.InsertDomainListFetchParams{
		ID: f.ID, SubscriptionID: f.SubscriptionID, Outcome: string(f.Outcome),
		HttpStatus: sql.NullInt64{Int64: int64(f.HTTPStatus), Valid: f.HTTPStatus != 0},
		ErrorCode:  sqlite.ToNullString(string(f.ErrorCode)), EntryCount: int64(f.EntryCount), FetchedAt: sqlite.ToMillis(f.FetchedAt),
	}), "insert domain list fetch")
}

func domainListFetchFrom(row moderationgen.DomainListFetch) moderation.DomainListFetch {
	return moderation.DomainListFetch{
		ID: row.ID, SubscriptionID: row.SubscriptionID, Outcome: moderation.DomainListFetchOutcome(row.Outcome), HTTPStatus: int(row.HttpStatus.Int64),
		ErrorCode: moderation.DomainListFetchError(row.ErrorCode.String), EntryCount: int(row.EntryCount), FetchedAt: sqlite.FromMillis(row.FetchedAt),
	}
}

// ListDomainListFetches implements appmoderation.DomainListRepository.
func (r *Repository) ListDomainListFetches(ctx context.Context, subscriptionID, before string, limit int) ([]moderation.DomainListFetch, error) {
	rows, err := r.db.reader().ListDomainListFetches(ctx, moderationgen.ListDomainListFetchesParams{
		SubscriptionID: subscriptionID, Before: before, Lim: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list domain list fetches")
	}
	out := make([]moderation.DomainListFetch, len(rows))
	for i, row := range rows {
		out[i] = domainListFetchFrom(row)
	}
	return out, nil
}

func domainListEntryFrom(row moderationgen.DomainListEntry) moderation.DomainListEntry {
	return moderation.DomainListEntry{
		SubscriptionID: row.SubscriptionID, Domain: row.Domain, Severity: moderation.DomainBlockSeverity(row.Severity),
		RejectMedia: row.RejectMedia != 0, RejectReports: row.RejectReports != 0, PublicComment: row.PublicComment.String,
		Obfuscate: row.Obfuscate != 0, SinceFetchID: row.SinceFetchID,
	}
}

// DomainListEntries implements appmoderation.DomainListRepository.
func (r *Repository) DomainListEntries(ctx context.Context, tx uow.Tx, subscriptionID string) ([]moderation.DomainListEntry, error) {
	q, err := r.domainListQueries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListDomainListEntries(ctx, subscriptionID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list domain list entries")
	}
	out := make([]moderation.DomainListEntry, len(rows))
	for i, row := range rows {
		out[i] = domainListEntryFrom(row)
	}
	return out, nil
}

// DomainListEntry implements appmoderation.DomainListRepository.
func (r *Repository) DomainListEntry(ctx context.Context, tx uow.Tx, subscriptionID, domain string) (moderation.DomainListEntry, bool, error) {
	q, err := r.domainListQueries(tx)
	if err != nil {
		return moderation.DomainListEntry{}, false, err
	}
	row, err := q.GetDomainListEntry(ctx, moderationgen.GetDomainListEntryParams{SubscriptionID: subscriptionID, Domain: domain})
	if errors.Is(err, sql.ErrNoRows) {
		return moderation.DomainListEntry{}, false, nil
	}
	if err != nil {
		return moderation.DomainListEntry{}, false, sqlite.Wrap(err, "get domain list entry")
	}
	return domainListEntryFrom(row), true, nil
}

// PutDomainListEntries implements appmoderation.DomainListRepository.
func (r *Repository) PutDomainListEntries(ctx context.Context, tx uow.Tx, entries []moderation.DomainListEntry) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, e := range entries {
		err := q.UpsertDomainListEntry(ctx, moderationgen.UpsertDomainListEntryParams{
			SubscriptionID: e.SubscriptionID, Domain: e.Domain, Severity: string(e.Severity), RejectMedia: sqlite.BoolToInt(e.RejectMedia),
			RejectReports: sqlite.BoolToInt(e.RejectReports), PublicComment: sqlite.ToNullString(e.PublicComment),
			Obfuscate: sqlite.BoolToInt(e.Obfuscate), SinceFetchID: e.SinceFetchID,
		})
		if err != nil {
			return sqlite.Wrap(err, "put domain list entry")
		}
	}
	return nil
}

// DeleteDomainListEntries implements appmoderation.DomainListRepository.
func (r *Repository) DeleteDomainListEntries(ctx context.Context, tx uow.Tx, subscriptionID string, domains []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if len(domains) == 0 {
		return sqlite.Wrap(q.DeleteDomainListEntriesOf(ctx, subscriptionID), "delete domain list entries")
	}
	for _, d := range domains {
		if err := q.DeleteDomainListEntry(ctx, moderationgen.DeleteDomainListEntryParams{SubscriptionID: subscriptionID, Domain: d}); err != nil {
			return sqlite.Wrap(err, "delete domain list entry")
		}
	}
	return nil
}

// InsertSubscriptionDomainAction implements appmoderation.DomainListRepository.
func (r *Repository) InsertSubscriptionDomainAction(ctx context.Context, tx uow.Tx, a moderation.SubscriptionDomainAction) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertSubscriptionDomainAction(ctx, moderationgen.InsertSubscriptionDomainActionParams{
		ActionID: a.ActionID, SubscriptionID: a.SubscriptionID, FetchID: sqlite.ToNullString(a.FetchID),
	}), "insert subscription domain action")
}

// SubscriptionOfDomainAction implements appmoderation.DomainListRepository.
func (r *Repository) SubscriptionOfDomainAction(ctx context.Context, tx uow.Tx, actionID string) (moderation.SubscriptionDomainAction, bool, error) {
	q, err := r.domainListQueries(tx)
	if err != nil {
		return moderation.SubscriptionDomainAction{}, false, err
	}
	row, err := q.GetSubscriptionDomainAction(ctx, actionID)
	if errors.Is(err, sql.ErrNoRows) {
		return moderation.SubscriptionDomainAction{}, false, nil
	}
	if err != nil {
		return moderation.SubscriptionDomainAction{}, false, sqlite.Wrap(err, "get subscription domain action")
	}
	return moderation.SubscriptionDomainAction{ActionID: row.ActionID, SubscriptionID: row.SubscriptionID, FetchID: row.FetchID.String}, true, nil
}

// SubscriptionDomainActionsInEffect implements appmoderation.DomainListRepository.
func (r *Repository) SubscriptionDomainActionsInEffect(ctx context.Context, tx uow.Tx, subscriptionID, domain string, now time.Time) ([]moderation.DomainAction, error) {
	q, err := r.domainListQueries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListSubscriptionDomainActionsInEffect(ctx, moderationgen.ListSubscriptionDomainActionsInEffectParams{
		SubscriptionID: subscriptionID, Domain: domain, Now: sqlite.ToMillis(now),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list subscription domain actions")
	}
	out := make([]moderation.DomainAction, len(rows))
	for i, row := range rows {
		out[i] = moderation.DomainAction{
			ID: row.ID, Domain: row.Domain, Kind: moderation.DomainActionKind(row.Kind), PublicComment: row.PublicComment.String,
			PrivateNote: row.PrivateNote.String, Obfuscate: row.Obfuscate != 0, IssuedByUserID: row.IssuedByUserID.String,
			ExpiresAt: sqlite.FromNullMillis(row.ExpiresAt), IssuedAt: sqlite.FromMillis(row.IssuedAt),
		}
	}
	return out, nil
}

// InsertDomainListRejection implements appmoderation.DomainListRepository.
func (r *Repository) InsertDomainListRejection(ctx context.Context, tx uow.Tx, rj moderation.DomainListRejection) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertDomainListDraftRejection(ctx, moderationgen.InsertDomainListDraftRejectionParams{
		ID: rj.ID, SubscriptionID: rj.SubscriptionID, ChangeKind: string(rj.Change), Domain: rj.Domain, Kind: string(rj.Kind),
		EntrySinceFetchID: sqlite.ToNullString(rj.EntrySinceFetchID), ActionID: sqlite.ToNullString(rj.ActionID),
		RejectedByUserID: sqlite.ToNullString(rj.RejectedByUserID), RejectedAt: sqlite.ToMillis(rj.RejectedAt),
	}), "insert domain list draft rejection")
}

// DomainListRejections implements appmoderation.DomainListRepository.
func (r *Repository) DomainListRejections(ctx context.Context, tx uow.Tx, subscriptionID string) ([]moderation.DomainListRejection, error) {
	q, err := r.domainListQueries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListDomainListDraftRejections(ctx, subscriptionID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list domain list draft rejections")
	}
	out := make([]moderation.DomainListRejection, len(rows))
	for i, row := range rows {
		out[i] = moderation.DomainListRejection{
			ID: row.ID, SubscriptionID: row.SubscriptionID, Change: moderation.DomainListChangeKind(row.ChangeKind), Domain: row.Domain,
			Kind: moderation.DomainActionKind(row.Kind), EntrySinceFetchID: row.EntrySinceFetchID.String,
			ActionID: row.ActionID.String, RejectedByUserID: row.RejectedByUserID.String, RejectedAt: sqlite.FromMillis(row.RejectedAt),
		}
	}
	return out, nil
}
