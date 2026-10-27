// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// InsertRemoteMedia implements content.MediaRepository.
func (r *MediaRepository) InsertRemoteMedia(ctx context.Context, tx uow.Tx, m content.RemoteMedia) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	var fx, fy sql.NullFloat64
	if m.Focus != nil {
		fx, fy = sql.NullFloat64{Float64: m.Focus.X, Valid: true}, sql.NullFloat64{Float64: m.Focus.Y, Valid: true}
	}
	n, err := q.InsertRemoteMedia(ctx, sqlitegen.InsertRemoteMediaParams{
		MediaID: m.MediaID, OwnerAccountID: m.OwnerAccountID, RemoteUrl: m.RemoteURL,
		DeclaredContentType: toNullString(m.DeclaredContentType), DeclaredMediaType: string(m.DeclaredMediaType), Purpose: string(m.Purpose),
		Width: toNullInt(m.Width), Height: toNullInt(m.Height), Blurhash: toNullString(m.Blurhash),
		FocusX: fx, FocusY: fy, Description: toNullString(m.Description),
	})
	return n > 0, wrap(err, "insert remote media")
}

// RemoteMediaByURL implements content.MediaRepository.
func (r *MediaRepository) RemoteMediaByURL(ctx context.Context, tx uow.Tx, ownerAccountID, remoteURL string) (content.RemoteMedia, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.RemoteMedia{}, err
	}
	row, err := q.GetRemoteMediaByURL(ctx, sqlitegen.GetRemoteMediaByURLParams{OwnerAccountID: ownerAccountID, RemoteUrl: remoteURL})
	if err != nil {
		return content.RemoteMedia{}, notFound(err, appcontent.ErrNotFound, "remote media "+remoteURL)
	}
	return remoteMedia(row), nil
}

// FindRemoteMedia implements content.MediaRepository.
func (r *MediaRepository) FindRemoteMedia(ctx context.Context, mediaID string) (content.RemoteMedia, error) {
	row, err := r.db.reader().GetRemoteMedia(ctx, mediaID)
	if err != nil {
		return content.RemoteMedia{}, notFound(err, appcontent.ErrNotFound, "remote media "+mediaID)
	}
	return remoteMedia(row), nil
}

// ListRemoteMedia implements content.MediaRepository.
func (r *MediaRepository) ListRemoteMedia(ctx context.Context, ids []string) ([]content.RemoteMedia, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := jsonList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListRemoteMedia(ctx, list)
	if err != nil {
		return nil, wrap(err, "list remote media")
	}
	out := make([]content.RemoteMedia, 0, len(rows))
	for _, row := range rows {
		out = append(out, remoteMedia(row))
	}
	return out, nil
}

func remoteMedia(row sqlitegen.RemoteMedium) content.RemoteMedia {
	m := content.RemoteMedia{
		MediaID: row.MediaID, OwnerAccountID: row.OwnerAccountID, RemoteURL: row.RemoteUrl,
		DeclaredContentType: row.DeclaredContentType.String, DeclaredMediaType: content.MediaType(row.DeclaredMediaType), Purpose: content.MediaPurpose(row.Purpose),
		Width: int(row.Width.Int64), Height: int(row.Height.Int64), Blurhash: row.Blurhash.String,
		Description: row.Description.String,
	}
	if row.FocusX.Valid && row.FocusY.Valid {
		m.Focus = &content.MediaFocus{MediaID: row.MediaID, X: row.FocusX.Float64, Y: row.FocusY.Float64}
	}
	return m
}
