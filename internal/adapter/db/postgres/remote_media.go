// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
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
	width, err := toNullInt32(m.Width, "width")
	if err != nil {
		return false, err
	}
	height, err := toNullInt32(m.Height, "height")
	if err != nil {
		return false, err
	}
	var fx, fy *float64
	if m.Focus != nil {
		x, y := m.Focus.X, m.Focus.Y
		fx, fy = &x, &y
	}
	n, err := q.InsertRemoteMedia(ctx, postgresgen.InsertRemoteMediaParams{
		MediaID: m.MediaID, OwnerAccountID: m.OwnerAccountID, RemoteUrl: m.RemoteURL,
		DeclaredContentType: toNullString(m.DeclaredContentType), DeclaredMediaType: string(m.DeclaredMediaType), Purpose: string(m.Purpose),
		Width: width, Height: height, Blurhash: toNullString(m.Blurhash),
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
	row, err := q.GetRemoteMediaByURL(ctx, postgresgen.GetRemoteMediaByURLParams{OwnerAccountID: ownerAccountID, RemoteUrl: remoteURL})
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
	rows, err := r.db.reader().ListRemoteMedia(ctx, ids)
	if err != nil {
		return nil, wrap(err, "list remote media")
	}
	out := make([]content.RemoteMedia, 0, len(rows))
	for _, row := range rows {
		out = append(out, remoteMedia(row))
	}
	return out, nil
}

func remoteMedia(row postgresgen.RemoteMedium) content.RemoteMedia {
	m := content.RemoteMedia{
		MediaID: row.MediaID, OwnerAccountID: row.OwnerAccountID, RemoteURL: row.RemoteUrl,
		DeclaredContentType: fromNullString(row.DeclaredContentType), DeclaredMediaType: content.MediaType(row.DeclaredMediaType), Purpose: content.MediaPurpose(row.Purpose),
		Width: fromNullInt32(row.Width), Height: fromNullInt32(row.Height), Blurhash: fromNullString(row.Blurhash),
		Description: fromNullString(row.Description),
	}
	if row.FocusX != nil && row.FocusY != nil {
		m.Focus = &content.MediaFocus{MediaID: row.MediaID, X: *row.FocusX, Y: *row.FocusY}
	}
	return m
}
