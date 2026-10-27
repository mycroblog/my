// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// MediaRepository implements content.MediaRepository on SQLite.
type MediaRepository struct {
	db *DB
}

var _ appcontent.MediaRepository = (*MediaRepository)(nil)

// MediaRepository returns the content module's media repository on d.
func (d *DB) MediaRepository() *MediaRepository { return &MediaRepository{db: d} }

// Zero dimensions, durations, and blurhashes are unknown and NULL in the database.
func toNullInt(n int) sql.NullInt64 { return sql.NullInt64{Int64: int64(n), Valid: n != 0} }

// InsertMediaFile implements content.MediaRepository.
func (r *MediaRepository) InsertMediaFile(ctx context.Context, tx uow.Tx, m content.MediaFile) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertMediaFile(ctx, sqlitegen.InsertMediaFileParams{
		ID: m.ID, OwnerAccountID: toNullString(m.OwnerAccountID), Origin: string(m.Origin), CreatedAt: toMillis(m.CreatedAt),
	}), "insert media file")
}

// InsertUploadedMedia implements content.MediaRepository.
func (r *MediaRepository) InsertUploadedMedia(ctx context.Context, tx uow.Tx, u content.UploadedMedia) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertUploadedMedia(ctx, sqlitegen.InsertUploadedMediaParams{
		MediaID: u.MediaID, BlobKey: u.BlobKey, ContentType: u.ContentType, FileSize: u.FileSize,
	}), "insert uploaded media")
}

// InsertProcessingResult implements content.MediaRepository.
func (r *MediaRepository) InsertProcessingResult(ctx context.Context, tx uow.Tx, res content.MediaProcessingResult) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertMediaProcessingResult(ctx, sqlitegen.InsertMediaProcessingResultParams{
		ID: res.ID, MediaID: res.MediaID, Purpose: string(res.Purpose), Outcome: string(res.Outcome), MediaType: string(res.MediaType),
		Width: toNullInt(res.Width), Height: toNullInt(res.Height), DurationMs: toNullInt(res.DurationMS),
		Blurhash: toNullString(res.Blurhash), ProcessedAt: toMillis(res.ProcessedAt),
	})
	if err != nil {
		return wrap(err, "insert media processing result")
	}
	for _, v := range res.Variants {
		err := q.InsertMediaVariant(ctx, sqlitegen.InsertMediaVariantParams{
			ResultID: res.ID, Size: string(v.Size), Motion: string(v.Motion), BlobKey: v.BlobKey, ContentType: v.ContentType, FileSize: v.FileSize,
			Width: int64(v.Width), Height: int64(v.Height),
		})
		if err != nil {
			return wrap(err, "insert media variant")
		}
	}
	return nil
}

// HasProcessingResult implements content.MediaRepository.
func (r *MediaRepository) HasProcessingResult(ctx context.Context, tx uow.Tx, mediaID string, purpose content.MediaPurpose) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	ok, err := q.HasCurrentMediaProcessingResult(ctx, sqlitegen.HasCurrentMediaProcessingResultParams{Purpose: string(purpose), MediaID: mediaID})
	return ok, wrap(err, "look up media processing result")
}

// UpsertFocus implements content.MediaRepository.
func (r *MediaRepository) UpsertFocus(ctx context.Context, tx uow.Tx, f content.MediaFocus) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertMediaFocus(ctx, sqlitegen.UpsertMediaFocusParams{MediaID: f.MediaID, FocusX: f.X, FocusY: f.Y}), "upsert media focus")
}

// DeleteUnattachedMedia implements content.MediaRepository.
func (r *MediaRepository) DeleteUnattachedMedia(ctx context.Context, tx uow.Tx, mediaID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteUnattachedMediaFile(ctx, mediaID)
	if err != nil || n == 0 {
		return false, wrap(err, "delete media file")
	}
	for _, del := range []func(context.Context, string) error{
		q.DeleteUploadedMedia, q.DeleteMediaVariantsOf, q.DeleteMediaProcessingResults, q.DeleteMediaFocus, q.DeleteMediaDescription,
	} {
		if err := del(ctx, mediaID); err != nil {
			return false, wrap(err, "delete media file rows")
		}
	}
	return true, nil
}

// FindMediaFile implements content.MediaRepository.
func (r *MediaRepository) FindMediaFile(ctx context.Context, id string) (content.MediaFile, error) {
	row, err := r.db.reader().GetMediaFile(ctx, id)
	if err != nil {
		return content.MediaFile{}, notFound(err, appcontent.ErrNotFound, "media file "+id)
	}
	return content.MediaFile{
		ID: row.ID, OwnerAccountID: row.OwnerAccountID.String, Origin: content.MediaOrigin(row.Origin), CreatedAt: fromMillis(row.CreatedAt),
	}, nil
}

// FindUploadedMedia implements content.MediaRepository.
func (r *MediaRepository) FindUploadedMedia(ctx context.Context, mediaID string) (content.UploadedMedia, error) {
	row, err := r.db.reader().GetUploadedMedia(ctx, mediaID)
	if err != nil {
		return content.UploadedMedia{}, notFound(err, appcontent.ErrNotFound, "uploaded media "+mediaID)
	}
	return content.UploadedMedia{MediaID: row.MediaID, BlobKey: row.BlobKey, ContentType: row.ContentType, FileSize: row.FileSize}, nil
}

// FindLatestProcessingResult implements content.MediaRepository.
func (r *MediaRepository) FindLatestProcessingResult(ctx context.Context, mediaID string) (content.MediaProcessingResult, error) {
	results, err := r.ListProcessingResults(ctx, mediaID)
	if err != nil {
		return content.MediaProcessingResult{}, err
	}
	if len(results) == 0 {
		return content.MediaProcessingResult{}, notFound(sql.ErrNoRows, appcontent.ErrNotFound, "media processing result of "+mediaID)
	}
	return results[len(results)-1], nil
}

// ListProcessingResults implements content.MediaRepository.
func (r *MediaRepository) ListProcessingResults(ctx context.Context, mediaID string) ([]content.MediaProcessingResult, error) {
	rows, err := r.db.reader().ListMediaProcessingResults(ctx, mediaID)
	if err != nil {
		return nil, wrap(err, "list media processing results")
	}
	variants, err := r.db.reader().ListMediaVariantsOf(ctx, mediaID)
	if err != nil {
		return nil, wrap(err, "list media variants")
	}
	results := make([]content.MediaProcessingResult, 0, len(rows))
	for _, row := range rows {
		res := content.MediaProcessingResult{
			ID: row.ID, MediaID: row.MediaID, Purpose: content.MediaPurpose(row.Purpose), Outcome: content.MediaOutcome(row.Outcome), MediaType: content.MediaType(row.MediaType),
			Width: int(row.Width.Int64), Height: int(row.Height.Int64), DurationMS: int(row.DurationMs.Int64),
			Blurhash: row.Blurhash.String, ProcessedAt: fromMillis(row.ProcessedAt),
		}
		for _, v := range variants {
			if v.ResultID == row.ID {
				res.Variants = append(res.Variants, content.MediaVariant{
					ResultID: v.ResultID, Size: content.MediaSize(v.Size), Motion: content.MediaMotion(v.Motion), BlobKey: v.BlobKey, ContentType: v.ContentType,
					FileSize: v.FileSize, Width: int(v.Width), Height: int(v.Height),
				})
			}
		}
		results = append(results, res)
	}
	return results, nil
}

// ListReadyVariants implements content.MediaRepository.
func (r *MediaRepository) ListReadyVariants(ctx context.Context, ids []string) ([]appcontent.FileVariant, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := jsonList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListReadyMediaVariants(ctx, list)
	if err != nil {
		return nil, wrap(err, "list ready media variants")
	}
	out := make([]appcontent.FileVariant, 0, len(rows))
	for _, row := range rows {
		out = append(out, appcontent.FileVariant{
			File: content.MediaFile{
				ID: row.ID, OwnerAccountID: row.OwnerAccountID.String, Origin: content.MediaOrigin(row.Origin), CreatedAt: fromMillis(row.CreatedAt),
			},
			Variant: content.MediaVariant{
				ResultID: row.ResultID, Size: content.MediaSize(row.Size), Motion: content.MediaMotion(row.Motion), BlobKey: row.BlobKey, ContentType: row.ContentType,
				FileSize: row.FileSize, Width: int(row.Width), Height: int(row.Height),
			},
		})
	}
	return out, nil
}

// FindFocus implements content.MediaRepository.
func (r *MediaRepository) FindFocus(ctx context.Context, mediaID string) (content.MediaFocus, error) {
	row, err := r.db.reader().GetMediaFocus(ctx, mediaID)
	if err != nil {
		return content.MediaFocus{}, notFound(err, appcontent.ErrNotFound, "media focus of "+mediaID)
	}
	return content.MediaFocus{MediaID: row.MediaID, X: row.FocusX, Y: row.FocusY}, nil
}

// ListUnattachedUploads implements content.MediaRepository.
func (r *MediaRepository) ListUnattachedUploads(ctx context.Context, afterID string, createdBefore time.Time, limit int) ([]string, error) {
	ids, err := r.db.reader().ListUnattachedUploads(ctx, sqlitegen.ListUnattachedUploadsParams{
		AfterID: afterID, BeforeID: kernel.UUIDv7Floor(createdBefore), RowLimit: int64(limit),
	})
	return ids, wrap(err, "list unattached uploads")
}

// ListOwnerUploads implements content.MediaRepository.
func (r *MediaRepository) ListOwnerUploads(ctx context.Context, ownerAccountID, afterID string, limit int) ([]appcontent.OwnedUpload, error) {
	rows, err := r.db.reader().ListOwnerUploads(ctx, sqlitegen.ListOwnerUploadsParams{
		OwnerAccountID: toNullString(ownerAccountID), AfterID: afterID, RowLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list owner uploads")
	}
	out := make([]appcontent.OwnedUpload, len(rows))
	for i, row := range rows {
		out[i] = appcontent.OwnedUpload{MediaID: row.ID, Attached: row.Attached}
	}
	return out, nil
}

// UpsertDescription implements content.MediaRepository.
func (r *MediaRepository) UpsertDescription(ctx context.Context, tx uow.Tx, d content.MediaDescription) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertMediaDescription(ctx, sqlitegen.UpsertMediaDescriptionParams{MediaID: d.MediaID, Description: d.Text}), "upsert media description")
}

// DeleteDescription implements content.MediaRepository.
func (r *MediaRepository) DeleteDescription(ctx context.Context, tx uow.Tx, mediaID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteMediaDescription(ctx, mediaID), "delete media description")
}

// IsAttached implements content.MediaRepository.
func (r *MediaRepository) IsAttached(ctx context.Context, tx uow.Tx, mediaID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	ok, err := q.IsMediaAttached(ctx, mediaID)
	return ok, wrap(err, "look up media attachments")
}

// FindDescription implements content.MediaRepository.
func (r *MediaRepository) FindDescription(ctx context.Context, mediaID string) (content.MediaDescription, error) {
	row, err := r.db.reader().GetMediaDescription(ctx, mediaID)
	if err != nil {
		return content.MediaDescription{}, notFound(err, appcontent.ErrNotFound, "media description of "+mediaID)
	}
	return content.MediaDescription{MediaID: row.MediaID, Text: row.Description}, nil
}
