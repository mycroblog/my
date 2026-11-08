// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/instancegen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InsertAnnouncement implements appinstance.AnnouncementRepository.
func (r *Repository) InsertAnnouncement(ctx context.Context, tx uow.Tx, id string, createdAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertAnnouncement(ctx, instancegen.InsertAnnouncementParams{ID: id, CreatedAt: createdAt}), "insert announcement")
}

// InsertAnnouncementRevision implements appinstance.AnnouncementRepository.
func (r *Repository) InsertAnnouncementRevision(ctx context.Context, tx uow.Tx, rev instance.AnnouncementRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAnnouncementRevision(ctx, instancegen.InsertAnnouncementRevisionParams{
		ID: rev.ID, AnnouncementID: rev.AnnouncementID, Title: rev.Title, Body: rev.Body,
		StartsAt: postgres.ToNullTime(rev.StartsAt), EndsAt: postgres.ToNullTime(rev.EndsAt),
		RevisedByUserID: postgres.ToNullString(rev.RevisedByUserID), RevisedAt: rev.RevisedAt,
	})
	return postgres.Wrap(err, "insert announcement revision")
}

// InsertAnnouncementStateChange implements appinstance.AnnouncementRepository.
func (r *Repository) InsertAnnouncementStateChange(ctx context.Context, tx uow.Tx, c instance.AnnouncementStateChange) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAnnouncementStateChange(ctx, instancegen.InsertAnnouncementStateChangeParams{
		ID: c.ID, AnnouncementID: c.AnnouncementID, State: string(c.State),
		ChangedByUserID: postgres.ToNullString(c.ChangedByUserID), ChangedAt: c.ChangedAt,
	})
	return postgres.Wrap(err, "insert announcement state change")
}

// AnnouncementHead implements appinstance.AnnouncementRepository.
func (r *Repository) AnnouncementHead(ctx context.Context, tx uow.Tx, id string) (instance.AnnouncementHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.AnnouncementHead{}, err
	}
	row, err := q.GetAnnouncementHead(ctx, id)
	if err != nil {
		return instance.AnnouncementHead{}, postgres.NotFound(err, appinstance.ErrNotFound, "announcement head "+id)
	}
	return instance.AnnouncementHead{
		AnnouncementID: row.AnnouncementID, RevisionID: row.RevisionID, State: instance.AnnouncementState(row.State),
		PublishedAt: postgres.FromNullTime(row.PublishedAt),
	}, nil
}

// UpsertAnnouncementHead implements appinstance.AnnouncementRepository.
func (r *Repository) UpsertAnnouncementHead(ctx context.Context, tx uow.Tx, h instance.AnnouncementHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertAnnouncementHead(ctx, instancegen.UpsertAnnouncementHeadParams{
		AnnouncementID: h.AnnouncementID, RevisionID: h.RevisionID, State: string(h.State), PublishedAt: postgres.ToNullTime(h.PublishedAt),
	})
	return postgres.Wrap(err, "upsert announcement head")
}

// DeleteAllAnnouncementHeads implements appinstance.AnnouncementRepository.
func (r *Repository) DeleteAllAnnouncementHeads(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteAllAnnouncementHeads(ctx), "delete announcement heads")
}

// ListAnnouncementRevisionIDs implements appinstance.AnnouncementRepository.
func (r *Repository) ListAnnouncementRevisionIDs(ctx context.Context, tx uow.Tx) ([]instance.AnnouncementHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAnnouncementRevisionIDs(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "list announcement revisions")
	}
	out := make([]instance.AnnouncementHead, len(rows))
	for i, row := range rows {
		out[i] = instance.AnnouncementHead{AnnouncementID: row.AnnouncementID, RevisionID: row.ID}
	}
	return out, nil
}

// ListAnnouncementStateChanges implements appinstance.AnnouncementRepository.
func (r *Repository) ListAnnouncementStateChanges(ctx context.Context, tx uow.Tx) ([]instance.AnnouncementStateChange, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAnnouncementStateChanges(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "list announcement state changes")
	}
	out := make([]instance.AnnouncementStateChange, len(rows))
	for i, row := range rows {
		out[i] = instance.AnnouncementStateChange{
			ID: row.ID, AnnouncementID: row.AnnouncementID, State: instance.AnnouncementState(row.State),
			ChangedByUserID: postgres.FromNullString(row.ChangedByUserID), ChangedAt: postgres.UTC(row.ChangedAt),
		}
	}
	return out, nil
}

// Announcement implements appinstance.AnnouncementRepository.
func (r *Repository) Announcement(ctx context.Context, tx uow.Tx, id string) (instance.Announcement, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.Announcement{}, err
	}
	return findAnnouncement(ctx, q, id)
}

// FindAnnouncement implements appinstance.AnnouncementRepository.
func (r *Repository) FindAnnouncement(ctx context.Context, id string) (instance.Announcement, error) {
	return findAnnouncement(ctx, r.db.reader(), id)
}

func findAnnouncement(ctx context.Context, q *instancegen.Queries, id string) (instance.Announcement, error) {
	row, err := q.GetAnnouncement(ctx, id)
	if err != nil {
		return instance.Announcement{}, postgres.NotFound(err, appinstance.ErrNotFound, "announcement "+id)
	}
	return announcementOf(instancegen.ListAnnouncementsRow(row)), nil
}

// ListAnnouncements implements appinstance.AnnouncementRepository.
func (r *Repository) ListAnnouncements(ctx context.Context, publishedOnly bool) ([]instance.Announcement, error) {
	q := r.db.reader()
	var rows []instancegen.ListAnnouncementsRow
	if publishedOnly {
		published, err := q.ListPublishedAnnouncements(ctx)
		if err != nil {
			return nil, postgres.Wrap(err, "list published announcements")
		}
		for _, row := range published {
			rows = append(rows, instancegen.ListAnnouncementsRow(row))
		}
	} else {
		var err error
		if rows, err = q.ListAnnouncements(ctx); err != nil {
			return nil, postgres.Wrap(err, "list announcements")
		}
	}
	out := make([]instance.Announcement, len(rows))
	for i, row := range rows {
		out[i] = announcementOf(row)
	}
	return out, nil
}

// ListAnnouncementPage implements appinstance.AnnouncementRepository.
func (r *Repository) ListAnnouncementPage(ctx context.Context, q appinstance.AnnouncementQuery, limit int) ([]instance.Announcement, error) {
	n, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListAnnouncementPage(ctx, instancegen.ListAnnouncementPageParams{
		BeforeID: postgres.ToNullString(q.Before), Filter: string(q.Filter), Now: q.Now, RowLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list a page of announcements")
	}
	out := make([]instance.Announcement, len(rows))
	for i, row := range rows {
		out[i] = announcementOf(instancegen.ListAnnouncementsRow(row))
	}
	return out, nil
}

func announcementOf(row instancegen.ListAnnouncementsRow) instance.Announcement {
	return instance.Announcement{
		ID: row.ID, CreatedAt: postgres.UTC(row.CreatedAt),
		State: instance.AnnouncementState(row.State), PublishedAt: postgres.FromNullTime(row.PublishedAt),
		Revision: instance.AnnouncementRevision{
			ID: row.RevisionID, AnnouncementID: row.ID, Title: row.Title, Body: row.Body,
			StartsAt: postgres.FromNullTime(row.StartsAt), EndsAt: postgres.FromNullTime(row.EndsAt),
			RevisedByUserID: postgres.FromNullString(row.RevisedByUserID), RevisedAt: postgres.UTC(row.RevisedAt),
		},
	}
}

// InsertAnnouncementRead implements appinstance.AnnouncementRepository.
func (r *Repository) InsertAnnouncementRead(ctx context.Context, tx uow.Tx, userID, announcementID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAnnouncementRead(ctx, instancegen.InsertAnnouncementReadParams{UserID: userID, AnnouncementID: announcementID, ReadAt: at})
	return postgres.Wrap(err, "insert announcement read")
}

// DeleteAnnouncementReadsOfUser implements appinstance.AnnouncementRepository.
func (r *Repository) DeleteAnnouncementReadsOfUser(ctx context.Context, tx uow.Tx, userID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	_, err = q.DeleteAnnouncementReadsOfUser(ctx, userID)
	return postgres.Wrap(err, "delete announcement reads")
}

// ListReadAnnouncementIDs implements appinstance.AnnouncementRepository.
func (r *Repository) ListReadAnnouncementIDs(ctx context.Context, userID string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	out, err := r.db.reader().ListReadAnnouncementIDs(ctx, instancegen.ListReadAnnouncementIDsParams{UserID: userID, AnnouncementIds: ids})
	return out, postgres.Wrap(err, "list read announcements")
}
