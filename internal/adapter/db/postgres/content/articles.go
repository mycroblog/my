// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/contentgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// Articles, the long posts in Markdown (ADR-0160 §2): the article subtype of a status and the
// article children of its revisions, which InsertRevision writes and the reads of revisions read
// with the other parts (revision_parts.go).

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
