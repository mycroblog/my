// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/moderationgen"
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
	return postgres.Wrap(q.InsertDomainListSubscription(ctx, moderationgen.InsertDomainListSubscriptionParams{
		ID: s.ID, Url: s.URL, Format: string(s.Format), CreatedByUserID: postgres.ToNullString(s.CreatedByUserID), CreatedAt: s.CreatedAt,
	}), "insert domain list subscription")
}

// InsertDomainListRevision implements appmoderation.DomainListRepository.
func (r *Repository) InsertDomainListRevision(ctx context.Context, tx uow.Tx, rev moderation.DomainListRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	st := rev.Settings
	return postgres.Wrap(q.InsertDomainListSubscriptionRevision(ctx, moderationgen.InsertDomainListSubscriptionRevisionParams{
		ID: rev.ID, SubscriptionID: rev.SubscriptionID, Title: st.Title, Mode: string(st.Mode), SuspendAs: string(st.SuspendAs),
		SilenceAs: string(st.SilenceAs), ApplyRejectMedia: st.ApplyRejectMedia, ApplyRejectReports: st.ApplyRejectReports,
		PublicCommentFromList: st.PublicCommentFromList, RevisedByUserID: postgres.ToNullString(rev.RevisedByUserID), RevisedAt: rev.RevisedAt,
	}), "insert domain list subscription revision")
}

// InsertDomainListDeletion implements appmoderation.DomainListRepository.
func (r *Repository) InsertDomainListDeletion(ctx context.Context, tx uow.Tx, d moderation.DomainListDeletion) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertDomainListSubscriptionDeletion(ctx, moderationgen.InsertDomainListSubscriptionDeletionParams{
		SubscriptionID: d.SubscriptionID, LiftActions: d.LiftActions, DeletedByUserID: postgres.ToNullString(d.DeletedByUserID), DeletedAt: d.DeletedAt,
	})
	if err != nil {
		return postgres.Wrap(err, "insert domain list subscription deletion")
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
	failures, err := postgres.ToInt32(h.ConsecutiveFailures, "consecutive failures")
	if err != nil {
		return err
	}
	return postgres.Wrap(q.UpsertDomainListSubscriptionHead(ctx, moderationgen.UpsertDomainListSubscriptionHeadParams{
		SubscriptionID: h.SubscriptionID, Etag: postgres.ToNullString(h.ETag), LastModified: postgres.ToNullString(h.LastModified),
		Digest: postgres.ToNullString(h.Digest), ConsecutiveFailures: failures, LastFetchID: postgres.ToNullString(h.LastFetchID),
	}), "put domain list subscription head")
}

// LockDomainListSubscription implements appmoderation.DomainListRepository with a
// transaction-scoped advisory lock on the subscription.
func (r *Repository) LockDomainListSubscription(ctx context.Context, tx uow.Tx, id string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.LockModerationTarget(ctx, "moderation:domain_list:"+id), "lock the domain list subscription")
}

func domainListRecordFrom(row moderationgen.GetDomainListSubscriptionRow) moderation.DomainListSubscriptionRecord {
	rec := moderation.DomainListSubscriptionRecord{
		Subscription: moderation.DomainListSubscription{
			ID: row.ID, URL: row.Url, Format: moderation.DomainListFormat(row.Format),
			CreatedByUserID: postgres.FromNullString(row.CreatedByUserID), CreatedAt: postgres.UTC(row.CreatedAt),
		},
		Revision: moderation.DomainListRevision{
			ID: row.RevisionID, SubscriptionID: row.ID, RevisedByUserID: postgres.FromNullString(row.RevisedByUserID), RevisedAt: postgres.UTC(row.RevisedAt),
			Settings: moderation.DomainListSettings{
				Title: row.Title, Mode: moderation.DomainListMode(row.Mode), SuspendAs: moderation.DomainListSeverityMapping(row.SuspendAs),
				SilenceAs: moderation.DomainListSeverityMapping(row.SilenceAs), ApplyRejectMedia: row.ApplyRejectMedia,
				ApplyRejectReports: row.ApplyRejectReports, PublicCommentFromList: row.PublicCommentFromList,
			},
		},
		Head: moderation.DomainListHead{
			SubscriptionID: row.ID, ETag: postgres.FromNullString(row.Etag), LastModified: postgres.FromNullString(row.LastModified),
			Digest: postgres.FromNullString(row.Digest), ConsecutiveFailures: int(row.ConsecutiveFailures), LastFetchID: postgres.FromNullString(row.LastFetchID),
		},
	}
	if row.LastFetchID != nil && row.LastOutcome != nil && row.LastFetchedAt != nil {
		f := moderation.DomainListFetch{
			ID: *row.LastFetchID, SubscriptionID: row.ID, Outcome: moderation.DomainListFetchOutcome(*row.LastOutcome),
			ErrorCode: moderation.DomainListFetchError(postgres.FromNullString(row.LastErrorCode)), FetchedAt: postgres.UTC(*row.LastFetchedAt),
		}
		if row.LastHttpStatus != nil {
			f.HTTPStatus = int(*row.LastHttpStatus)
		}
		if row.LastEntryCount != nil {
			f.EntryCount = int(*row.LastEntryCount)
		}
		rec.LastFetch = &f
	}
	if row.DeletionSubscriptionID != nil && row.DeletedAt != nil {
		rec.Deletion = &moderation.DomainListDeletion{
			SubscriptionID: row.ID, LiftActions: row.LiftActions != nil && *row.LiftActions,
			DeletedByUserID: postgres.FromNullString(row.DeletedByUserID), DeletedAt: postgres.UTC(*row.DeletedAt),
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
		return moderation.DomainListSubscriptionRecord{}, postgres.NotFound(err, appmoderation.ErrNotFound, "domain list subscription "+id)
	}
	return domainListRecordFrom(row), nil
}

// ListDomainListSubscriptions implements appmoderation.DomainListRepository.
func (r *Repository) ListDomainListSubscriptions(ctx context.Context, before string, limit int) ([]moderation.DomainListSubscriptionRecord, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDomainListSubscriptions(ctx, moderationgen.ListDomainListSubscriptionsParams{Before: postgres.ToNullString(before), Lim: lim})
	if err != nil {
		return nil, postgres.Wrap(err, "list domain list subscriptions")
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
	count, err := postgres.ToInt32(f.EntryCount, "entry count")
	if err != nil {
		return err
	}
	var status *int32
	if f.HTTPStatus != 0 {
		s, err := postgres.ToInt32(f.HTTPStatus, "http status")
		if err != nil {
			return err
		}
		status = &s
	}
	return postgres.Wrap(q.InsertDomainListFetch(ctx, moderationgen.InsertDomainListFetchParams{
		ID: f.ID, SubscriptionID: f.SubscriptionID, Outcome: string(f.Outcome), HttpStatus: status,
		ErrorCode: postgres.ToNullString(string(f.ErrorCode)), EntryCount: count, FetchedAt: f.FetchedAt,
	}), "insert domain list fetch")
}

func domainListFetchFrom(row moderationgen.DomainListFetch) moderation.DomainListFetch {
	f := moderation.DomainListFetch{
		ID: row.ID, SubscriptionID: row.SubscriptionID, Outcome: moderation.DomainListFetchOutcome(row.Outcome),
		ErrorCode: moderation.DomainListFetchError(postgres.FromNullString(row.ErrorCode)), EntryCount: int(row.EntryCount), FetchedAt: postgres.UTC(row.FetchedAt),
	}
	if row.HttpStatus != nil {
		f.HTTPStatus = int(*row.HttpStatus)
	}
	return f
}

// ListDomainListFetches implements appmoderation.DomainListRepository.
func (r *Repository) ListDomainListFetches(ctx context.Context, subscriptionID, before string, limit int) ([]moderation.DomainListFetch, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDomainListFetches(ctx, moderationgen.ListDomainListFetchesParams{
		SubscriptionID: subscriptionID, Before: postgres.ToNullString(before), Lim: lim,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list domain list fetches")
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
		RejectMedia: row.RejectMedia, RejectReports: row.RejectReports, PublicComment: postgres.FromNullString(row.PublicComment),
		Obfuscate: row.Obfuscate, SinceFetchID: row.SinceFetchID,
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
		return nil, postgres.Wrap(err, "list domain list entries")
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
	if errors.Is(err, pgx.ErrNoRows) {
		return moderation.DomainListEntry{}, false, nil
	}
	if err != nil {
		return moderation.DomainListEntry{}, false, postgres.Wrap(err, "get domain list entry")
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
			SubscriptionID: e.SubscriptionID, Domain: e.Domain, Severity: string(e.Severity), RejectMedia: e.RejectMedia,
			RejectReports: e.RejectReports, PublicComment: postgres.ToNullString(e.PublicComment), Obfuscate: e.Obfuscate, SinceFetchID: e.SinceFetchID,
		})
		if err != nil {
			return postgres.Wrap(err, "put domain list entry")
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
		return postgres.Wrap(q.DeleteDomainListEntriesOf(ctx, subscriptionID), "delete domain list entries")
	}
	for _, d := range domains {
		if err := q.DeleteDomainListEntry(ctx, moderationgen.DeleteDomainListEntryParams{SubscriptionID: subscriptionID, Domain: d}); err != nil {
			return postgres.Wrap(err, "delete domain list entry")
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
	return postgres.Wrap(q.InsertSubscriptionDomainAction(ctx, moderationgen.InsertSubscriptionDomainActionParams{
		ActionID: a.ActionID, SubscriptionID: a.SubscriptionID, FetchID: postgres.ToNullString(a.FetchID),
	}), "insert subscription domain action")
}

// SubscriptionOfDomainAction implements appmoderation.DomainListRepository.
func (r *Repository) SubscriptionOfDomainAction(ctx context.Context, tx uow.Tx, actionID string) (moderation.SubscriptionDomainAction, bool, error) {
	q, err := r.domainListQueries(tx)
	if err != nil {
		return moderation.SubscriptionDomainAction{}, false, err
	}
	row, err := q.GetSubscriptionDomainAction(ctx, actionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return moderation.SubscriptionDomainAction{}, false, nil
	}
	if err != nil {
		return moderation.SubscriptionDomainAction{}, false, postgres.Wrap(err, "get subscription domain action")
	}
	return moderation.SubscriptionDomainAction{ActionID: row.ActionID, SubscriptionID: row.SubscriptionID, FetchID: postgres.FromNullString(row.FetchID)}, true, nil
}

// SubscriptionDomainActionsInEffect implements appmoderation.DomainListRepository.
func (r *Repository) SubscriptionDomainActionsInEffect(ctx context.Context, tx uow.Tx, subscriptionID, domain string, now time.Time) ([]moderation.DomainAction, error) {
	q, err := r.domainListQueries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListSubscriptionDomainActionsInEffect(ctx, moderationgen.ListSubscriptionDomainActionsInEffectParams{
		SubscriptionID: subscriptionID, Domain: domain, Now: now,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list subscription domain actions")
	}
	out := make([]moderation.DomainAction, len(rows))
	for i, row := range rows {
		out[i] = moderation.DomainAction{
			ID: row.ID, Domain: row.Domain, Kind: moderation.DomainActionKind(row.Kind), PublicComment: postgres.FromNullString(row.PublicComment),
			PrivateNote: postgres.FromNullString(row.PrivateNote), Obfuscate: row.Obfuscate, IssuedByUserID: postgres.FromNullString(row.IssuedByUserID),
			ExpiresAt: postgres.UTCPtr(row.ExpiresAt), IssuedAt: postgres.UTC(row.IssuedAt),
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
	return postgres.Wrap(q.InsertDomainListDraftRejection(ctx, moderationgen.InsertDomainListDraftRejectionParams{
		ID: rj.ID, SubscriptionID: rj.SubscriptionID, ChangeKind: string(rj.Change), Domain: rj.Domain, Kind: string(rj.Kind),
		EntrySinceFetchID: postgres.ToNullString(rj.EntrySinceFetchID), ActionID: postgres.ToNullString(rj.ActionID),
		RejectedByUserID: postgres.ToNullString(rj.RejectedByUserID), RejectedAt: rj.RejectedAt,
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
		return nil, postgres.Wrap(err, "list domain list draft rejections")
	}
	out := make([]moderation.DomainListRejection, len(rows))
	for i, row := range rows {
		out[i] = moderation.DomainListRejection{
			ID: row.ID, SubscriptionID: row.SubscriptionID, Change: moderation.DomainListChangeKind(row.ChangeKind), Domain: row.Domain,
			Kind: moderation.DomainActionKind(row.Kind), EntrySinceFetchID: postgres.FromNullString(row.EntrySinceFetchID),
			ActionID: postgres.FromNullString(row.ActionID), RejectedByUserID: postgres.FromNullString(row.RejectedByUserID), RejectedAt: postgres.UTC(row.RejectedAt),
		}
	}
	return out, nil
}
