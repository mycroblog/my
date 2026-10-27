// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The server-wide announcements (FR-726): the instance.AnnouncementRepository part of
// InstanceRepository. The zero time of an open side of a window is NULL.

func toNullTime(t time.Time) sql.NullInt64 {
	if t.IsZero() {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: toMillis(t), Valid: true}
}

func fromNullTime(n sql.NullInt64) time.Time {
	if !n.Valid {
		return time.Time{}
	}
	return fromMillis(n.Int64)
}

// InsertAnnouncement implements appinstance.AnnouncementRepository.
func (r *InstanceRepository) InsertAnnouncement(ctx context.Context, tx uow.Tx, id string, createdAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertAnnouncement(ctx, sqlitegen.InsertAnnouncementParams{ID: id, CreatedAt: toMillis(createdAt)}), "insert announcement")
}

// InsertAnnouncementRevision implements appinstance.AnnouncementRepository.
func (r *InstanceRepository) InsertAnnouncementRevision(ctx context.Context, tx uow.Tx, rev instance.AnnouncementRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAnnouncementRevision(ctx, sqlitegen.InsertAnnouncementRevisionParams{
		ID: rev.ID, AnnouncementID: rev.AnnouncementID, Body: rev.Body,
		StartsAt: toNullTime(rev.StartsAt), EndsAt: toNullTime(rev.EndsAt),
		RevisedByUserID: toNullString(rev.RevisedByUserID), RevisedAt: toMillis(rev.RevisedAt),
	})
	return wrap(err, "insert announcement revision")
}

// InsertAnnouncementStateChange implements appinstance.AnnouncementRepository.
func (r *InstanceRepository) InsertAnnouncementStateChange(ctx context.Context, tx uow.Tx, c instance.AnnouncementStateChange) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAnnouncementStateChange(ctx, sqlitegen.InsertAnnouncementStateChangeParams{
		ID: c.ID, AnnouncementID: c.AnnouncementID, State: string(c.State),
		ChangedByUserID: toNullString(c.ChangedByUserID), ChangedAt: toMillis(c.ChangedAt),
	})
	return wrap(err, "insert announcement state change")
}

// AnnouncementHead implements appinstance.AnnouncementRepository.
func (r *InstanceRepository) AnnouncementHead(ctx context.Context, tx uow.Tx, id string) (instance.AnnouncementHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.AnnouncementHead{}, err
	}
	row, err := q.GetAnnouncementHead(ctx, id)
	if err != nil {
		return instance.AnnouncementHead{}, notFound(err, appinstance.ErrNotFound, "announcement head "+id)
	}
	return instance.AnnouncementHead{
		AnnouncementID: row.AnnouncementID, RevisionID: row.RevisionID, State: instance.AnnouncementState(row.State),
		PublishedAt: fromNullTime(row.PublishedAt),
	}, nil
}

// UpsertAnnouncementHead implements appinstance.AnnouncementRepository.
func (r *InstanceRepository) UpsertAnnouncementHead(ctx context.Context, tx uow.Tx, h instance.AnnouncementHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertAnnouncementHead(ctx, sqlitegen.UpsertAnnouncementHeadParams{
		AnnouncementID: h.AnnouncementID, RevisionID: h.RevisionID, State: string(h.State), PublishedAt: toNullTime(h.PublishedAt),
	})
	return wrap(err, "upsert announcement head")
}

// DeleteAllAnnouncementHeads implements appinstance.AnnouncementRepository.
func (r *InstanceRepository) DeleteAllAnnouncementHeads(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAllAnnouncementHeads(ctx), "delete announcement heads")
}

// ListAnnouncementRevisionIDs implements appinstance.AnnouncementRepository.
func (r *InstanceRepository) ListAnnouncementRevisionIDs(ctx context.Context, tx uow.Tx) ([]instance.AnnouncementHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAnnouncementRevisionIDs(ctx)
	if err != nil {
		return nil, wrap(err, "list announcement revisions")
	}
	out := make([]instance.AnnouncementHead, len(rows))
	for i, row := range rows {
		out[i] = instance.AnnouncementHead{AnnouncementID: row.AnnouncementID, RevisionID: row.ID}
	}
	return out, nil
}

// ListAnnouncementStateChanges implements appinstance.AnnouncementRepository.
func (r *InstanceRepository) ListAnnouncementStateChanges(ctx context.Context, tx uow.Tx) ([]instance.AnnouncementStateChange, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAnnouncementStateChanges(ctx)
	if err != nil {
		return nil, wrap(err, "list announcement state changes")
	}
	out := make([]instance.AnnouncementStateChange, len(rows))
	for i, row := range rows {
		out[i] = instance.AnnouncementStateChange{
			ID: row.ID, AnnouncementID: row.AnnouncementID, State: instance.AnnouncementState(row.State),
			ChangedByUserID: row.ChangedByUserID.String, ChangedAt: fromMillis(row.ChangedAt),
		}
	}
	return out, nil
}

// Announcement implements appinstance.AnnouncementRepository.
func (r *InstanceRepository) Announcement(ctx context.Context, tx uow.Tx, id string) (instance.Announcement, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.Announcement{}, err
	}
	return findAnnouncement(ctx, q, id)
}

// FindAnnouncement implements appinstance.AnnouncementRepository.
func (r *InstanceRepository) FindAnnouncement(ctx context.Context, id string) (instance.Announcement, error) {
	return findAnnouncement(ctx, r.db.reader(), id)
}

func findAnnouncement(ctx context.Context, q *sqlitegen.Queries, id string) (instance.Announcement, error) {
	row, err := q.GetAnnouncement(ctx, id)
	if err != nil {
		return instance.Announcement{}, notFound(err, appinstance.ErrNotFound, "announcement "+id)
	}
	return announcementOf(sqlitegen.ListAnnouncementsRow(row)), nil
}

// ListAnnouncements implements appinstance.AnnouncementRepository.
func (r *InstanceRepository) ListAnnouncements(ctx context.Context, publishedOnly bool) ([]instance.Announcement, error) {
	q := r.db.reader()
	var rows []sqlitegen.ListAnnouncementsRow
	if publishedOnly {
		published, err := q.ListPublishedAnnouncements(ctx)
		if err != nil {
			return nil, wrap(err, "list published announcements")
		}
		for _, row := range published {
			rows = append(rows, sqlitegen.ListAnnouncementsRow(row))
		}
	} else {
		var err error
		if rows, err = q.ListAnnouncements(ctx); err != nil {
			return nil, wrap(err, "list announcements")
		}
	}
	out := make([]instance.Announcement, len(rows))
	for i, row := range rows {
		out[i] = announcementOf(row)
	}
	return out, nil
}

func announcementOf(row sqlitegen.ListAnnouncementsRow) instance.Announcement {
	return instance.Announcement{
		ID: row.ID, CreatedAt: fromMillis(row.CreatedAt),
		State: instance.AnnouncementState(row.State), PublishedAt: fromNullTime(row.PublishedAt),
		Revision: instance.AnnouncementRevision{
			ID: row.RevisionID, AnnouncementID: row.ID, Body: row.Body,
			StartsAt: fromNullTime(row.StartsAt), EndsAt: fromNullTime(row.EndsAt),
			RevisedByUserID: row.RevisedByUserID.String, RevisedAt: fromMillis(row.RevisedAt),
		},
	}
}

// InsertAnnouncementRead implements appinstance.AnnouncementRepository.
func (r *InstanceRepository) InsertAnnouncementRead(ctx context.Context, tx uow.Tx, userID, announcementID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAnnouncementRead(ctx, sqlitegen.InsertAnnouncementReadParams{UserID: userID, AnnouncementID: announcementID, ReadAt: toMillis(at)})
	return wrap(err, "insert announcement read")
}

// DeleteAnnouncementReadsOfUser implements appinstance.AnnouncementRepository.
func (r *InstanceRepository) DeleteAnnouncementReadsOfUser(ctx context.Context, tx uow.Tx, userID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	_, err = q.DeleteAnnouncementReadsOfUser(ctx, userID)
	return wrap(err, "delete announcement reads")
}

// ListReadAnnouncementIDs implements appinstance.AnnouncementRepository.
func (r *InstanceRepository) ListReadAnnouncementIDs(ctx context.Context, userID string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	out, err := r.db.reader().ListReadAnnouncementIDs(ctx, sqlitegen.ListReadAnnouncementIDsParams{UserID: userID, AnnouncementIds: ids})
	return out, wrap(err, "list read announcements")
}
