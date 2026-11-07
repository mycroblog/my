// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
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
	return sqlite.Wrap(q.UpsertStatusLinkCard(ctx, contentgen.UpsertStatusLinkCardParams{StatusID: l.StatusID, Url: l.URL}), "upsert status link card")
}

// DeleteStatusLinkCard implements content.LinkCardRepository.
func (r *StatusRepository) DeleteStatusLinkCard(ctx context.Context, tx uow.Tx, statusID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteStatusLinkCard(ctx, statusID), "delete status link card")
}

// StatusLinkCardURL implements content.LinkCardRepository.
func (r *StatusRepository) StatusLinkCardURL(ctx context.Context, statusID string) (string, error) {
	u, err := r.db.reader().GetStatusLinkCardURL(ctx, statusID)
	if err != nil {
		return "", sqlite.NotFound(err, appcontent.ErrNotFound, "status link card "+statusID)
	}
	return u, nil
}

// FindLinkCard implements content.LinkCardRepository.
func (r *StatusRepository) FindLinkCard(ctx context.Context, url string) (content.LinkCard, error) {
	row, err := r.db.reader().GetLinkCard(ctx, url)
	if err != nil {
		return content.LinkCard{}, sqlite.NotFound(err, appcontent.ErrNotFound, "link card "+url)
	}
	return linkCard(row), nil
}

// FindLinkCardTx implements content.LinkCardRepository.
func (r *StatusRepository) FindLinkCardTx(ctx context.Context, tx uow.Tx, url string) (content.LinkCard, error) {
	d, err := r.db.in(tx)
	if err != nil {
		return content.LinkCard{}, err
	}
	return (&StatusRepository{db: d}).FindLinkCard(ctx, url)
}

// UpsertLinkCard implements content.LinkCardRepository.
func (r *StatusRepository) UpsertLinkCard(ctx context.Context, tx uow.Tx, c content.LinkCard) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertLinkCard(ctx, contentgen.UpsertLinkCardParams{
		Url: c.URL, CardUrl: c.CardURL, Title: c.Title, Description: c.Description, SiteName: c.SiteName,
		AuthorName: c.AuthorName, PublishedAt: sqlite.ToNullMillis(c.PublishedAt), ImageMediaID: sqlite.ToNullString(c.ImageMediaID),
		CreatorAccountID: sqlite.ToNullString(c.CreatorAccountID), Language: sqlite.ToNullString(c.Language), FetchedAt: sqlite.ToMillis(c.FetchedAt),
	}), "upsert link card")
}

// ListUnreferencedLinkCards implements content.LinkCardRepository.
func (r *StatusRepository) ListUnreferencedLinkCards(ctx context.Context, fetchedBefore time.Time, limit int) ([]content.LinkCard, error) {
	rows, err := r.db.reader().ListUnreferencedLinkCards(ctx, contentgen.ListUnreferencedLinkCardsParams{
		FetchedBefore: sqlite.ToMillis(fetchedBefore), BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list unreferenced link cards")
	}
	out := make([]content.LinkCard, 0, len(rows))
	for _, row := range rows {
		out = append(out, linkCard(row))
	}
	return out, nil
}

// DeleteLinkCard implements content.LinkCardRepository.
func (r *StatusRepository) DeleteLinkCard(ctx context.Context, tx uow.Tx, url string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteLinkCard(ctx, url), "delete link card")
}

// DeleteUnreferencedLinkCard implements content.LinkCardRepository.
func (r *StatusRepository) DeleteUnreferencedLinkCard(ctx context.Context, tx uow.Tx, url string, fetchedAt time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteUnreferencedLinkCard(ctx, contentgen.DeleteUnreferencedLinkCardParams{Url: url, FetchedAt: sqlite.ToMillis(fetchedAt)})
	return n > 0, sqlite.Wrap(err, "delete link card")
}

// InsertLinkCardMedia implements content.LinkCardRepository.
func (r *StatusRepository) InsertLinkCardMedia(ctx context.Context, tx uow.Tx, m content.LinkCardMedia, createdAt time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertLinkCardMedia(ctx, contentgen.InsertLinkCardMediaParams{
		MediaID: m.MediaID, RemoteUrl: m.RemoteURL, ContentType: m.ContentType, Width: int64(m.Width), Height: int64(m.Height),
		Blurhash: m.Blurhash, Description: m.Description,
	})
	if err != nil || n == 0 {
		return false, sqlite.Wrap(err, "insert link card media")
	}
	err = q.InsertMediaFile(ctx, contentgen.InsertMediaFileParams{
		ID: m.MediaID, OwnerAccountID: sqlite.ToNullString(""), Origin: string(content.MediaOriginRemote), CreatedAt: sqlite.ToMillis(createdAt),
	})
	return err == nil, sqlite.Wrap(err, "insert link card media file")
}

// LinkCardMediaByURL implements content.LinkCardRepository.
func (r *StatusRepository) LinkCardMediaByURL(ctx context.Context, tx uow.Tx, remoteURL string) (content.LinkCardMedia, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.LinkCardMedia{}, err
	}
	row, err := q.GetLinkCardMediaByURL(ctx, remoteURL)
	if err != nil {
		return content.LinkCardMedia{}, sqlite.NotFound(err, appcontent.ErrNotFound, "link card media "+remoteURL)
	}
	return linkCardMedia(row), nil
}

// FindLinkCardMedia implements content.LinkCardRepository.
func (r *StatusRepository) FindLinkCardMedia(ctx context.Context, mediaID string) (content.LinkCardMedia, error) {
	row, err := r.db.reader().GetLinkCardMedia(ctx, mediaID)
	if err != nil {
		return content.LinkCardMedia{}, sqlite.NotFound(err, appcontent.ErrNotFound, "link card media "+mediaID)
	}
	return linkCardMedia(row), nil
}

// ListUnreferencedLinkCardMedia implements content.LinkCardRepository.
func (r *StatusRepository) ListUnreferencedLinkCardMedia(ctx context.Context, limit int) ([]string, error) {
	ids, err := r.db.reader().ListUnreferencedLinkCardMedia(ctx, int64(limit))
	return ids, sqlite.Wrap(err, "list unreferenced link card media")
}

// DeleteUnreferencedLinkCardMedia implements content.LinkCardRepository.
func (r *StatusRepository) DeleteUnreferencedLinkCardMedia(ctx context.Context, tx uow.Tx, mediaID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteUnreferencedLinkCardMedia(ctx, mediaID)
	if err != nil || n == 0 {
		return false, sqlite.Wrap(err, "delete link card media")
	}
	return true, sqlite.Wrap(q.DeleteLinkCardMediaFile(ctx, mediaID), "delete link card media file")
}

func linkCard(row contentgen.LinkCard) content.LinkCard {
	return content.LinkCard{
		URL: row.Url, CardURL: row.CardUrl, Title: row.Title, Description: row.Description, SiteName: row.SiteName,
		AuthorName: row.AuthorName, PublishedAt: sqlite.FromNullMillis(row.PublishedAt), ImageMediaID: row.ImageMediaID.String,
		CreatorAccountID: row.CreatorAccountID.String, Language: row.Language.String, FetchedAt: sqlite.FromMillis(row.FetchedAt),
	}
}

func linkCardMedia(row contentgen.LinkCardMedium) content.LinkCardMedia {
	return content.LinkCardMedia{
		MediaID: row.MediaID, RemoteURL: row.RemoteUrl, ContentType: row.ContentType, Width: int(row.Width), Height: int(row.Height),
		Blurhash: row.Blurhash, Description: row.Description,
	}
}
