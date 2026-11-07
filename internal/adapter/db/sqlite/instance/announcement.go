// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/instancegen"
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
	return sqlite.Wrap(q.InsertAnnouncement(ctx, instancegen.InsertAnnouncementParams{ID: id, CreatedAt: sqlite.ToMillis(createdAt)}), "insert announcement")
}

// InsertAnnouncementRevision implements appinstance.AnnouncementRepository.
func (r *Repository) InsertAnnouncementRevision(ctx context.Context, tx uow.Tx, rev instance.AnnouncementRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAnnouncementRevision(ctx, instancegen.InsertAnnouncementRevisionParams{
		ID: rev.ID, AnnouncementID: rev.AnnouncementID, Title: rev.Title, Body: rev.Body,
		StartsAt: sqlite.ToNullTime(rev.StartsAt), EndsAt: sqlite.ToNullTime(rev.EndsAt),
		RevisedByUserID: sqlite.ToNullString(rev.RevisedByUserID), RevisedAt: sqlite.ToMillis(rev.RevisedAt),
	})
	return sqlite.Wrap(err, "insert announcement revision")
}

// InsertAnnouncementStateChange implements appinstance.AnnouncementRepository.
func (r *Repository) InsertAnnouncementStateChange(ctx context.Context, tx uow.Tx, c instance.AnnouncementStateChange) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAnnouncementStateChange(ctx, instancegen.InsertAnnouncementStateChangeParams{
		ID: c.ID, AnnouncementID: c.AnnouncementID, State: string(c.State),
		ChangedByUserID: sqlite.ToNullString(c.ChangedByUserID), ChangedAt: sqlite.ToMillis(c.ChangedAt),
	})
	return sqlite.Wrap(err, "insert announcement state change")
}

// AnnouncementHead implements appinstance.AnnouncementRepository.
func (r *Repository) AnnouncementHead(ctx context.Context, tx uow.Tx, id string) (instance.AnnouncementHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.AnnouncementHead{}, err
	}
	row, err := q.GetAnnouncementHead(ctx, id)
	if err != nil {
		return instance.AnnouncementHead{}, sqlite.NotFound(err, appinstance.ErrNotFound, "announcement head "+id)
	}
	return instance.AnnouncementHead{
		AnnouncementID: row.AnnouncementID, RevisionID: row.RevisionID, State: instance.AnnouncementState(row.State),
		PublishedAt: sqlite.FromNullTime(row.PublishedAt),
	}, nil
}

// UpsertAnnouncementHead implements appinstance.AnnouncementRepository.
func (r *Repository) UpsertAnnouncementHead(ctx context.Context, tx uow.Tx, h instance.AnnouncementHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertAnnouncementHead(ctx, instancegen.UpsertAnnouncementHeadParams{
		AnnouncementID: h.AnnouncementID, RevisionID: h.RevisionID, State: string(h.State), PublishedAt: sqlite.ToNullTime(h.PublishedAt),
	})
	return sqlite.Wrap(err, "upsert announcement head")
}

// DeleteAllAnnouncementHeads implements appinstance.AnnouncementRepository.
func (r *Repository) DeleteAllAnnouncementHeads(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAllAnnouncementHeads(ctx), "delete announcement heads")
}

// ListAnnouncementRevisionIDs implements appinstance.AnnouncementRepository.
func (r *Repository) ListAnnouncementRevisionIDs(ctx context.Context, tx uow.Tx) ([]instance.AnnouncementHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAnnouncementRevisionIDs(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list announcement revisions")
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
		return nil, sqlite.Wrap(err, "list announcement state changes")
	}
	out := make([]instance.AnnouncementStateChange, len(rows))
	for i, row := range rows {
		out[i] = instance.AnnouncementStateChange{
			ID: row.ID, AnnouncementID: row.AnnouncementID, State: instance.AnnouncementState(row.State),
			ChangedByUserID: row.ChangedByUserID.String, ChangedAt: sqlite.FromMillis(row.ChangedAt),
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
		return instance.Announcement{}, sqlite.NotFound(err, appinstance.ErrNotFound, "announcement "+id)
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
			return nil, sqlite.Wrap(err, "list published announcements")
		}
		for _, row := range published {
			rows = append(rows, instancegen.ListAnnouncementsRow(row))
		}
	} else {
		var err error
		if rows, err = q.ListAnnouncements(ctx); err != nil {
			return nil, sqlite.Wrap(err, "list announcements")
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
	rows, err := r.db.reader().ListAnnouncementPage(ctx, instancegen.ListAnnouncementPageParams{
		BeforeID: q.Before, Filter: string(q.Filter), Now: sqlite.ToMillis(q.Now), RowLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list a page of announcements")
	}
	out := make([]instance.Announcement, len(rows))
	for i, row := range rows {
		out[i] = announcementOf(instancegen.ListAnnouncementsRow(row))
	}
	return out, nil
}

func announcementOf(row instancegen.ListAnnouncementsRow) instance.Announcement {
	return instance.Announcement{
		ID: row.ID, CreatedAt: sqlite.FromMillis(row.CreatedAt),
		State: instance.AnnouncementState(row.State), PublishedAt: sqlite.FromNullTime(row.PublishedAt),
		Revision: instance.AnnouncementRevision{
			ID: row.RevisionID, AnnouncementID: row.ID, Title: row.Title, Body: row.Body,
			StartsAt: sqlite.FromNullTime(row.StartsAt), EndsAt: sqlite.FromNullTime(row.EndsAt),
			RevisedByUserID: row.RevisedByUserID.String, RevisedAt: sqlite.FromMillis(row.RevisedAt),
		},
	}
}

// InsertAnnouncementRead implements appinstance.AnnouncementRepository.
func (r *Repository) InsertAnnouncementRead(ctx context.Context, tx uow.Tx, userID, announcementID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAnnouncementRead(ctx, instancegen.InsertAnnouncementReadParams{UserID: userID, AnnouncementID: announcementID, ReadAt: sqlite.ToMillis(at)})
	return sqlite.Wrap(err, "insert announcement read")
}

// DeleteAnnouncementReadsOfUser implements appinstance.AnnouncementRepository.
func (r *Repository) DeleteAnnouncementReadsOfUser(ctx context.Context, tx uow.Tx, userID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	_, err = q.DeleteAnnouncementReadsOfUser(ctx, userID)
	return sqlite.Wrap(err, "delete announcement reads")
}

// ListReadAnnouncementIDs implements appinstance.AnnouncementRepository.
func (r *Repository) ListReadAnnouncementIDs(ctx context.Context, userID string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	out, err := r.db.reader().ListReadAnnouncementIDs(ctx, instancegen.ListReadAnnouncementIDsParams{UserID: userID, AnnouncementIds: ids})
	return out, sqlite.Wrap(err, "list read announcements")
}
