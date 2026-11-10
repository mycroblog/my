// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"database/sql"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// MediaRepository implements content.MediaRepository on SQLite.
type MediaRepository struct {
	db *moduleDB
}

var _ appcontent.MediaRepository = (*MediaRepository)(nil)

// NewMediaRepository returns the content module's media repository on d.
func NewMediaRepository(d *sqlite.DB) *MediaRepository { return &MediaRepository{db: &moduleDB{d}} }

// Zero dimensions, durations, and blurhashes are unknown and NULL in the database.
func toNullInt(n int) sql.NullInt64 { return sql.NullInt64{Int64: int64(n), Valid: n != 0} }

// InsertMediaFile implements content.MediaRepository.
func (r *MediaRepository) InsertMediaFile(ctx context.Context, tx uow.Tx, m content.MediaFile) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertMediaFile(ctx, contentgen.InsertMediaFileParams{
		ID: m.ID, OwnerAccountID: sqlite.ToNullString(m.OwnerAccountID), Origin: string(m.Origin), CreatedAt: sqlite.ToMillis(m.CreatedAt),
	}), "insert media file")
}

// InsertUploadedMedia implements content.MediaRepository.
func (r *MediaRepository) InsertUploadedMedia(ctx context.Context, tx uow.Tx, u content.UploadedMedia) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertUploadedMedia(ctx, contentgen.InsertUploadedMediaParams{
		MediaID: u.MediaID, BlobKey: u.BlobKey, ContentType: u.ContentType, FileSize: u.FileSize,
	}), "insert uploaded media")
}

// InsertProcessingResult implements content.MediaRepository.
func (r *MediaRepository) InsertProcessingResult(ctx context.Context, tx uow.Tx, res content.MediaProcessingResult) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertMediaProcessingResult(ctx, contentgen.InsertMediaProcessingResultParams{
		ID: res.ID, MediaID: res.MediaID, Purpose: string(res.Purpose), Outcome: string(res.Outcome), MediaType: string(res.MediaType),
		Width: toNullInt(res.Width), Height: toNullInt(res.Height), DurationMs: toNullInt(res.DurationMS),
		Blurhash: sqlite.ToNullString(res.Blurhash), ProcessedAt: sqlite.ToMillis(res.ProcessedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert media processing result")
	}
	for _, v := range res.Variants {
		err := q.InsertMediaVariant(ctx, contentgen.InsertMediaVariantParams{
			ResultID: res.ID, Size: string(v.Size), Motion: string(v.Motion), BlobKey: v.BlobKey, ContentType: v.ContentType, FileSize: v.FileSize,
			Width: int64(v.Width), Height: int64(v.Height),
		})
		if err != nil {
			return sqlite.Wrap(err, "insert media variant")
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
	ok, err := q.HasCurrentMediaProcessingResult(ctx, contentgen.HasCurrentMediaProcessingResultParams{Purpose: string(purpose), MediaID: mediaID})
	return ok, sqlite.Wrap(err, "look up media processing result")
}

// UpsertFocus implements content.MediaRepository.
func (r *MediaRepository) UpsertFocus(ctx context.Context, tx uow.Tx, f content.MediaFocus) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertMediaFocus(ctx, contentgen.UpsertMediaFocusParams{MediaID: f.MediaID, FocusX: f.X, FocusY: f.Y}), "upsert media focus")
}

// DeleteUnattachedMedia implements content.MediaRepository.
func (r *MediaRepository) DeleteUnattachedMedia(ctx context.Context, tx uow.Tx, mediaID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteUnattachedMediaFile(ctx, mediaID)
	if err != nil || n == 0 {
		return false, sqlite.Wrap(err, "delete media file")
	}
	for _, del := range []func(context.Context, string) error{
		q.DeleteUploadedMedia, q.DeleteMediaVariantsOf, q.DeleteMediaProcessingResults, q.DeleteMediaFocus, q.DeleteMediaDescription,
	} {
		if err := del(ctx, mediaID); err != nil {
			return false, sqlite.Wrap(err, "delete media file rows")
		}
	}
	return true, nil
}

// FindMediaFile implements content.MediaRepository.
func (r *MediaRepository) FindMediaFile(ctx context.Context, id string) (content.MediaFile, error) {
	row, err := r.db.reader().GetMediaFile(ctx, id)
	if err != nil {
		return content.MediaFile{}, sqlite.NotFound(err, appcontent.ErrNotFound, "media file "+id)
	}
	return mediaFileFrom(row), nil
}

// FindUploadedMedia implements content.MediaRepository.
func (r *MediaRepository) FindUploadedMedia(ctx context.Context, mediaID string) (content.UploadedMedia, error) {
	row, err := r.db.reader().GetUploadedMedia(ctx, mediaID)
	if err != nil {
		return content.UploadedMedia{}, sqlite.NotFound(err, appcontent.ErrNotFound, "uploaded media "+mediaID)
	}
	return uploadedMediaFrom(row), nil
}

// FindLatestProcessingResult implements content.MediaRepository.
func (r *MediaRepository) FindLatestProcessingResult(ctx context.Context, mediaID string) (content.MediaProcessingResult, error) {
	results, err := r.ListProcessingResults(ctx, mediaID)
	if err != nil {
		return content.MediaProcessingResult{}, err
	}
	if len(results) == 0 {
		return content.MediaProcessingResult{}, sqlite.NotFound(sql.ErrNoRows, appcontent.ErrNotFound, "media processing result of "+mediaID)
	}
	return results[len(results)-1], nil
}

// ListProcessingResults implements content.MediaRepository.
func (r *MediaRepository) ListProcessingResults(ctx context.Context, mediaID string) ([]content.MediaProcessingResult, error) {
	rows, err := r.db.reader().ListMediaProcessingResults(ctx, mediaID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list media processing results")
	}
	variants, err := r.db.reader().ListMediaVariantsOf(ctx, mediaID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list media variants")
	}
	return processingResultsFrom(rows, variants), nil
}

// ListReadyVariants implements content.MediaRepository.
func (r *MediaRepository) ListReadyVariants(ctx context.Context, ids []string) ([]appcontent.FileVariant, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListReadyMediaVariants(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list ready media variants")
	}
	out := make([]appcontent.FileVariant, 0, len(rows))
	for _, row := range rows {
		out = append(out, appcontent.FileVariant{
			File: content.MediaFile{
				ID: row.ID, OwnerAccountID: row.OwnerAccountID.String, Origin: content.MediaOrigin(row.Origin), CreatedAt: sqlite.FromMillis(row.CreatedAt),
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
		return content.MediaFocus{}, sqlite.NotFound(err, appcontent.ErrNotFound, "media focus of "+mediaID)
	}
	return mediaFocusFrom(row), nil
}

// ListUnattachedUploads implements content.MediaRepository.
func (r *MediaRepository) ListUnattachedUploads(ctx context.Context, afterID string, createdBefore time.Time, limit int) ([]string, error) {
	ids, err := r.db.reader().ListUnattachedUploads(ctx, contentgen.ListUnattachedUploadsParams{
		AfterID: afterID, BeforeID: kernel.UUIDv7Floor(createdBefore), RowLimit: int64(limit),
	})
	return ids, sqlite.Wrap(err, "list unattached uploads")
}

// ListOwnerUploads implements content.MediaRepository.
func (r *MediaRepository) ListOwnerUploads(ctx context.Context, ownerAccountID, afterID string, limit int) ([]appcontent.OwnedUpload, error) {
	rows, err := r.db.reader().ListOwnerUploads(ctx, contentgen.ListOwnerUploadsParams{
		OwnerAccountID: sqlite.ToNullString(ownerAccountID), AfterID: afterID, RowLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list owner uploads")
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
	return sqlite.Wrap(q.UpsertMediaDescription(ctx, contentgen.UpsertMediaDescriptionParams{MediaID: d.MediaID, Description: d.Text}), "upsert media description")
}

// DeleteDescription implements content.MediaRepository.
func (r *MediaRepository) DeleteDescription(ctx context.Context, tx uow.Tx, mediaID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteMediaDescription(ctx, mediaID), "delete media description")
}

// IsAttached implements content.MediaRepository.
func (r *MediaRepository) IsAttached(ctx context.Context, tx uow.Tx, mediaID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	ok, err := q.IsMediaAttached(ctx, mediaID)
	return ok, sqlite.Wrap(err, "look up media attachments")
}

// FindDescription implements content.MediaRepository.
func (r *MediaRepository) FindDescription(ctx context.Context, mediaID string) (content.MediaDescription, error) {
	row, err := r.db.reader().GetMediaDescription(ctx, mediaID)
	if err != nil {
		return content.MediaDescription{}, sqlite.NotFound(err, appcontent.ErrNotFound, "media description of "+mediaID)
	}
	return mediaDescriptionFrom(row), nil
}

// HasNewerProcessingResult implements content.MediaRepository.
func (r *MediaRepository) HasNewerProcessingResult(ctx context.Context, tx uow.Tx, mediaID, resultID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	ok, err := q.HasNewerMediaProcessingResult(ctx, contentgen.HasNewerMediaProcessingResultParams{MediaID: mediaID, ResultID: resultID})
	return ok, sqlite.Wrap(err, "look up newer media processing result")
}

// DeleteProcessingResult implements content.MediaRepository.
func (r *MediaRepository) DeleteProcessingResult(ctx context.Context, tx uow.Tx, resultID string) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	keys, err := q.DeleteMediaVariantsOfResult(ctx, resultID)
	if err != nil {
		return nil, sqlite.Wrap(err, "delete media variants")
	}
	return keys, sqlite.Wrap(q.DeleteMediaProcessingResult(ctx, resultID), "delete media processing result")
}

// ListReplacedProcessingResults implements content.MediaRepository.
func (r *MediaRepository) ListReplacedProcessingResults(ctx context.Context, afterID string, replacedBefore time.Time, limit int) ([]string, error) {
	ids, err := r.db.reader().ListReplacedMediaProcessingResults(ctx, contentgen.ListReplacedMediaProcessingResultsParams{
		AfterID: afterID, ReplacedBefore: sqlite.ToMillis(replacedBefore), RowLimit: int64(limit),
	})
	return ids, sqlite.Wrap(err, "list replaced media processing results")
}

// ListReprocessableUploads implements content.MediaRepository.
func (r *MediaRepository) ListReprocessableUploads(ctx context.Context, afterID string, processedBefore time.Time, limit int) ([]string, error) {
	ids, err := r.db.reader().ListReprocessableUploads(ctx, contentgen.ListReprocessableUploadsParams{
		AfterID: afterID, ProcessedBefore: sqlite.ToMillis(processedBefore), RowLimit: int64(limit),
	})
	return ids, sqlite.Wrap(err, "list reprocessable uploads")
}

func mediaFileFrom(row contentgen.MediaFile) content.MediaFile {
	return content.MediaFile{
		ID: row.ID, OwnerAccountID: row.OwnerAccountID.String, Origin: content.MediaOrigin(row.Origin), CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}
}

func uploadedMediaFrom(row contentgen.UploadedMedium) content.UploadedMedia {
	return content.UploadedMedia{MediaID: row.MediaID, BlobKey: row.BlobKey, ContentType: row.ContentType, FileSize: row.FileSize}
}

func mediaFocusFrom(row contentgen.MediaFocu) content.MediaFocus {
	return content.MediaFocus{MediaID: row.MediaID, X: row.FocusX, Y: row.FocusY}
}

func mediaDescriptionFrom(row contentgen.MediaDescription) content.MediaDescription {
	return content.MediaDescription{MediaID: row.MediaID, Text: row.Description}
}

// processingResultsFrom builds the processing results of rows with their variants, in the order of
// rows; variants of other results are skipped.
func processingResultsFrom(rows []contentgen.MediaProcessingResult, variants []contentgen.MediaVariant) []content.MediaProcessingResult {
	results := make([]content.MediaProcessingResult, 0, len(rows))
	for _, row := range rows {
		res := content.MediaProcessingResult{
			ID: row.ID, MediaID: row.MediaID, Purpose: content.MediaPurpose(row.Purpose), Outcome: content.MediaOutcome(row.Outcome), MediaType: content.MediaType(row.MediaType),
			Width: int(row.Width.Int64), Height: int(row.Height.Int64), DurationMS: int(row.DurationMs.Int64),
			Blurhash: row.Blurhash.String, ProcessedAt: sqlite.FromMillis(row.ProcessedAt),
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
	return results
}
