// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

var _ appcontent.LinkCardRepository = (*StatusRepository)(nil)

// UpsertStatusLinkCard implements content.LinkCardRepository.
func (r *StatusRepository) UpsertStatusLinkCard(ctx context.Context, tx uow.Tx, l content.StatusLinkCard) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertStatusLinkCard(ctx, postgresgen.UpsertStatusLinkCardParams{StatusID: l.StatusID, Url: l.URL}), "upsert status link card")
}

// DeleteStatusLinkCard implements content.LinkCardRepository.
func (r *StatusRepository) DeleteStatusLinkCard(ctx context.Context, tx uow.Tx, statusID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteStatusLinkCard(ctx, statusID), "delete status link card")
}

// StatusLinkCardURL implements content.LinkCardRepository.
func (r *StatusRepository) StatusLinkCardURL(ctx context.Context, statusID string) (string, error) {
	u, err := r.db.reader().GetStatusLinkCardURL(ctx, statusID)
	if err != nil {
		return "", notFound(err, appcontent.ErrNotFound, "status link card "+statusID)
	}
	return u, nil
}

// FindLinkCard implements content.LinkCardRepository.
func (r *StatusRepository) FindLinkCard(ctx context.Context, url string) (content.LinkCard, error) {
	row, err := r.db.reader().GetLinkCard(ctx, url)
	if err != nil {
		return content.LinkCard{}, notFound(err, appcontent.ErrNotFound, "link card "+url)
	}
	return linkCard(row), nil
}

// FindLinkCardTx implements content.LinkCardRepository.
func (r *StatusRepository) FindLinkCardTx(ctx context.Context, tx uow.Tx, url string) (content.LinkCard, error) {
	d, err := r.db.in(tx)
	if err != nil {
		return content.LinkCard{}, err
	}
	return d.StatusRepository().FindLinkCard(ctx, url)
}

// UpsertLinkCard implements content.LinkCardRepository.
func (r *StatusRepository) UpsertLinkCard(ctx context.Context, tx uow.Tx, c content.LinkCard) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertLinkCard(ctx, postgresgen.UpsertLinkCardParams{
		Url: c.URL, CardUrl: c.CardURL, Title: c.Title, Description: c.Description, SiteName: c.SiteName,
		AuthorName: c.AuthorName, PublishedAt: utcPtr(c.PublishedAt), ImageMediaID: toNullString(c.ImageMediaID),
		CreatorAccountID: toNullString(c.CreatorAccountID), Language: toNullString(c.Language), FetchedAt: c.FetchedAt,
	}), "upsert link card")
}

// ListUnreferencedLinkCards implements content.LinkCardRepository.
func (r *StatusRepository) ListUnreferencedLinkCards(ctx context.Context, fetchedBefore time.Time, limit int) ([]content.LinkCard, error) {
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListUnreferencedLinkCards(ctx, postgresgen.ListUnreferencedLinkCardsParams{
		FetchedBefore: fetchedBefore, BatchLimit: n,
	})
	if err != nil {
		return nil, wrap(err, "list unreferenced link cards")
	}
	out := make([]content.LinkCard, 0, len(rows))
	for _, row := range rows {
		out = append(out, linkCard(row))
	}
	return out, nil
}

// DeleteUnreferencedLinkCard implements content.LinkCardRepository.
func (r *StatusRepository) DeleteUnreferencedLinkCard(ctx context.Context, tx uow.Tx, url string, fetchedAt time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteUnreferencedLinkCard(ctx, postgresgen.DeleteUnreferencedLinkCardParams{Url: url, FetchedAt: fetchedAt})
	return n > 0, wrap(err, "delete link card")
}

// InsertLinkCardMedia implements content.LinkCardRepository.
func (r *StatusRepository) InsertLinkCardMedia(ctx context.Context, tx uow.Tx, m content.LinkCardMedia, createdAt time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	w, err := toInt32(m.Width, "link card media width")
	if err != nil {
		return false, err
	}
	h, err := toInt32(m.Height, "link card media height")
	if err != nil {
		return false, err
	}
	n, err := q.InsertLinkCardMedia(ctx, postgresgen.InsertLinkCardMediaParams{
		MediaID: m.MediaID, RemoteUrl: m.RemoteURL, ContentType: m.ContentType, Width: w, Height: h,
		Blurhash: m.Blurhash, Description: m.Description,
	})
	if err != nil || n == 0 {
		return false, wrap(err, "insert link card media")
	}
	err = q.InsertMediaFile(ctx, postgresgen.InsertMediaFileParams{
		ID: m.MediaID, OwnerAccountID: nil, Origin: string(content.MediaOriginRemote), CreatedAt: createdAt,
	})
	return err == nil, wrap(err, "insert link card media file")
}

// LinkCardMediaByURL implements content.LinkCardRepository.
func (r *StatusRepository) LinkCardMediaByURL(ctx context.Context, tx uow.Tx, remoteURL string) (content.LinkCardMedia, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.LinkCardMedia{}, err
	}
	row, err := q.GetLinkCardMediaByURL(ctx, remoteURL)
	if err != nil {
		return content.LinkCardMedia{}, notFound(err, appcontent.ErrNotFound, "link card media "+remoteURL)
	}
	return linkCardMedia(row), nil
}

// FindLinkCardMedia implements content.LinkCardRepository.
func (r *StatusRepository) FindLinkCardMedia(ctx context.Context, mediaID string) (content.LinkCardMedia, error) {
	row, err := r.db.reader().GetLinkCardMedia(ctx, mediaID)
	if err != nil {
		return content.LinkCardMedia{}, notFound(err, appcontent.ErrNotFound, "link card media "+mediaID)
	}
	return linkCardMedia(row), nil
}

// ListUnreferencedLinkCardMedia implements content.LinkCardRepository.
func (r *StatusRepository) ListUnreferencedLinkCardMedia(ctx context.Context, limit int) ([]string, error) {
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListUnreferencedLinkCardMedia(ctx, n)
	return ids, wrap(err, "list unreferenced link card media")
}

// DeleteUnreferencedLinkCardMedia implements content.LinkCardRepository.
func (r *StatusRepository) DeleteUnreferencedLinkCardMedia(ctx context.Context, tx uow.Tx, mediaID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteUnreferencedLinkCardMedia(ctx, mediaID)
	if err != nil || n == 0 {
		return false, wrap(err, "delete link card media")
	}
	return true, wrap(q.DeleteLinkCardMediaFile(ctx, mediaID), "delete link card media file")
}

func linkCard(row postgresgen.LinkCard) content.LinkCard {
	return content.LinkCard{
		URL: row.Url, CardURL: row.CardUrl, Title: row.Title, Description: row.Description, SiteName: row.SiteName,
		AuthorName: row.AuthorName, PublishedAt: utcPtr(row.PublishedAt), ImageMediaID: fromNullString(row.ImageMediaID),
		CreatorAccountID: fromNullString(row.CreatorAccountID), Language: fromNullString(row.Language), FetchedAt: utc(row.FetchedAt),
	}
}

func linkCardMedia(row postgresgen.LinkCardMedium) content.LinkCardMedia {
	return content.LinkCardMedia{
		MediaID: row.MediaID, RemoteURL: row.RemoteUrl, ContentType: row.ContentType, Width: int(row.Width), Height: int(row.Height),
		Blurhash: row.Blurhash, Description: row.Description,
	}
}
