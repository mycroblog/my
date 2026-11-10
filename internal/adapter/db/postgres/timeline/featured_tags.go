// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"math"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/timelinegen"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
)

var _ apptimeline.FeaturedTagRepository = (*Repository)(nil)

// FeaturedTagsTx implements timeline.FeaturedTagRepository.
func (r *Repository) FeaturedTagsTx(ctx context.Context, tx uow.Tx, accountID string) ([]apptimeline.FeaturedTagRow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListFeaturedTags(ctx, accountID)
	return featuredTagsFrom(rows, err, "list featured tags")
}

// ReplaceFeaturedTags implements timeline.FeaturedTagRepository.
func (r *Repository) ReplaceFeaturedTags(ctx context.Context, tx uow.Tx, accountID string, rows []apptimeline.FeaturedTagRow) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteFeaturedTagsOf(ctx, accountID); err != nil {
		return postgres.Wrap(err, "delete featured tags")
	}
	for _, row := range rows {
		if err := q.InsertFeaturedTag(ctx, timelinegen.InsertFeaturedTagParams{
			AccountID: accountID, TagID: row.TagID, Ordinal: int32(min(row.Ordinal, math.MaxInt32)), //nolint:gosec // G115: bounded by min
			FeaturedAt: row.FeaturedAt,
		}); err != nil {
			return postgres.Wrap(err, "insert featured tag")
		}
	}
	return nil
}

// FeaturedTagsOf implements timeline.FeaturedTagRepository.
func (r *Repository) FeaturedTagsOf(ctx context.Context, accountID string) ([]apptimeline.FeaturedTagRow, error) {
	rows, err := r.db.reader().ListFeaturedTags(ctx, accountID)
	return featuredTagsFrom(rows, err, "list featured tags")
}

// FeaturedTagUsageOf implements timeline.FeaturedTagRepository.
func (r *Repository) FeaturedTagUsageOf(ctx context.Context, accountID string) (map[string]apptimeline.FeaturedTagUsage, error) {
	rows, err := r.db.reader().ListFeaturedTagUsage(ctx, accountID)
	if err != nil {
		return nil, postgres.Wrap(err, "list featured tag usage")
	}
	out := make(map[string]apptimeline.FeaturedTagUsage, len(rows))
	for _, row := range rows {
		out[row.TagID] = apptimeline.FeaturedTagUsage{TagID: row.TagID, StatusesCount: row.StatusesCount, LastStatusAt: row.LastStatusAt}
	}
	return out, nil
}

// AllFeaturedTags implements timeline.FeaturedTagRepository.
func (r *Repository) AllFeaturedTags(ctx context.Context) ([]apptimeline.FeaturedTagRow, error) {
	rows, err := r.db.reader().ListAllFeaturedTags(ctx)
	return featuredTagsFrom(rows, err, "list all featured tags")
}

func featuredTagsFrom(rows []timelinegen.FeaturedTag, err error, what string) ([]apptimeline.FeaturedTagRow, error) {
	if err != nil {
		return nil, postgres.Wrap(err, what)
	}
	out := make([]apptimeline.FeaturedTagRow, len(rows))
	for i, row := range rows {
		out[i] = apptimeline.FeaturedTagRow{
			AccountID: row.AccountID, TagID: row.TagID, Ordinal: int(row.Ordinal), FeaturedAt: row.FeaturedAt,
		}
	}
	return out, nil
}
