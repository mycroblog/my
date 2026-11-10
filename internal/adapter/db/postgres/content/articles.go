// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/contentgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// Articles, the long posts in Markdown (ADR-0160 §2): the article subtype of a status and the
// article children of its revisions, which InsertRevision writes and the reads of revisions read
// with the other children.

// InsertArticle implements content.Repository.
func (r *StatusRepository) InsertArticle(ctx context.Context, tx uow.Tx, a content.Article) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertStatusArticle(ctx, contentgen.InsertStatusArticleParams{StatusID: a.StatusID, AuthorAccountID: a.AuthorAccountID})
	return postgres.Wrap(err, "insert status article")
}

// ListLiveArticleIDs implements content.Repository.
func (r *StatusRepository) ListLiveArticleIDs(ctx context.Context, authorAccountID, before string, limit int) ([]string, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListLiveArticleIDsOfAuthor(ctx, contentgen.ListLiveArticleIDsOfAuthorParams{
		AuthorAccountID: authorAccountID, Before: postgres.ToNullString(before), Lim: lim,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list articles of "+authorAccountID)
	}
	return ids, nil
}

// insertRevisionArticle writes the article children of rev, none for a revision of a post.
func insertRevisionArticle(ctx context.Context, q *contentgen.Queries, rev content.Revision) error {
	a := rev.Article
	if a == nil {
		return nil
	}
	err := q.InsertStatusRevisionArticle(ctx, contentgen.InsertStatusRevisionArticleParams{
		RevisionID: rev.ID, Title: a.Title, Summary: a.Summary, SummaryOrigin: string(a.SummaryOrigin),
	})
	if err != nil {
		return postgres.Wrap(err, "insert status revision article")
	}
	if c := a.Cover; c != nil {
		err := q.InsertStatusRevisionArticleCover(ctx, contentgen.InsertStatusRevisionArticleCoverParams{
			RevisionID: rev.ID, MediaID: c.MediaID, Description: c.Description,
		})
		if err != nil {
			return postgres.Wrap(err, "insert status revision article cover")
		}
	}
	for _, img := range a.Images {
		pos, err := postgres.ToInt32(img.Position, "article image position")
		if err != nil {
			return err
		}
		err = q.InsertStatusRevisionArticleImage(ctx, contentgen.InsertStatusRevisionArticleImageParams{
			RevisionID: rev.ID, Position: pos, MediaID: img.MediaID, Description: img.Description,
		})
		if err != nil {
			return postgres.Wrap(err, "insert status revision article image")
		}
	}
	return nil
}

// readRevisionArticle reads the article children of the revision of rr into it.
func readRevisionArticle(ctx context.Context, q *contentgen.Queries, rr *revisionRows) error {
	id := rr.row.ID
	a, err := q.GetStatusRevisionArticle(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil
	case err != nil:
		return postgres.Wrap(err, "get status revision article")
	}
	rr.article = &a
	cover, err := q.GetStatusRevisionArticleCover(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return postgres.Wrap(err, "get status revision article cover")
	default:
		rr.cover = &cover
	}
	if rr.images, err = q.ListStatusRevisionArticleImages(ctx, id); err != nil {
		return postgres.Wrap(err, "list status revision article images")
	}
	return nil
}

// readRevisionArticles reads the article children of the revisions revs into
// byRevision.
func readRevisionArticles(ctx context.Context, q *contentgen.Queries, revs []string, byRevision map[string]*revisionRows) error {
	articles, err := q.ListStatusRevisionArticlesOf(ctx, revs)
	if err != nil {
		return postgres.Wrap(err, "list status revision articles")
	}
	if len(articles) == 0 {
		return nil
	}
	for _, a := range articles {
		byRevision[a.RevisionID].article = &a
	}
	covers, err := q.ListStatusRevisionArticleCoversOf(ctx, revs)
	if err != nil {
		return postgres.Wrap(err, "list status revision article covers")
	}
	for _, c := range covers {
		byRevision[c.RevisionID].cover = &c
	}
	images, err := q.ListStatusRevisionArticleImagesOf(ctx, revs)
	if err != nil {
		return postgres.Wrap(err, "list status revision article images")
	}
	for _, img := range images {
		byRevision[img.RevisionID].images = append(byRevision[img.RevisionID].images, img)
	}
	return nil
}

// articleRevision builds the article child of a revision from its rows, nil without them.
func (rr revisionRows) articleRevision() *content.ArticleRevision {
	if rr.article == nil {
		return nil
	}
	a := &content.ArticleRevision{
		Title: rr.article.Title, Summary: rr.article.Summary, SummaryOrigin: content.SummaryOrigin(rr.article.SummaryOrigin),
	}
	if c := rr.cover; c != nil {
		a.Cover = &content.ArticleCover{MediaID: c.MediaID, Description: c.Description}
	}
	for _, img := range rr.images {
		a.Images = append(a.Images, content.ArticleImage{Position: int(img.Position), MediaID: img.MediaID, Description: img.Description})
	}
	return a
}
