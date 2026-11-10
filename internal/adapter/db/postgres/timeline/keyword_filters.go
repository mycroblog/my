// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/timelinegen"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

var _ apptimeline.KeywordFilterRepository = (*Repository)(nil)

// InsertKeywordFilter implements timeline.KeywordFilterRepository.
func (r *Repository) InsertKeywordFilter(ctx context.Context, tx uow.Tx, f timeline.KeywordFilter) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.InsertKeywordFilter(ctx, timelinegen.InsertKeywordFilterParams{
		ID: f.ID, AccountID: f.AccountID, CreatedAt: f.CreatedAt,
	}); err != nil {
		return postgres.Wrap(err, "insert keyword filter")
	}
	return writeKeywordFilterParts(ctx, q, f)
}

// ReplaceKeywordFilter implements timeline.KeywordFilterRepository.
func (r *Repository) ReplaceKeywordFilter(ctx context.Context, tx uow.Tx, f timeline.KeywordFilter) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteKeywordFilterContexts(ctx, f.ID); err != nil {
		return postgres.Wrap(err, "delete keyword filter contexts")
	}
	if err := q.DeleteKeywordFilterWords(ctx, f.ID); err != nil {
		return postgres.Wrap(err, "delete keyword filter words")
	}
	return writeKeywordFilterParts(ctx, q, f)
}

// writeKeywordFilterParts writes the settings of f and inserts its contexts and words.
func writeKeywordFilterParts(ctx context.Context, q *timelinegen.Queries, f timeline.KeywordFilter) error {
	if err := q.UpsertKeywordFilterSettings(ctx, timelinegen.UpsertKeywordFilterSettingsParams{
		FilterID: f.ID, Title: f.Title, Action: string(f.Action), ExpiresAt: postgres.ToNullTime(f.ExpiresAt),
	}); err != nil {
		return postgres.Wrap(err, "write keyword filter settings")
	}
	for _, c := range f.Contexts {
		if err := q.InsertKeywordFilterContext(ctx, timelinegen.InsertKeywordFilterContextParams{FilterID: f.ID, Context: string(c)}); err != nil {
			return postgres.Wrap(err, "insert keyword filter context")
		}
	}
	for i, w := range f.Words {
		if err := q.InsertKeywordFilterWord(ctx, timelinegen.InsertKeywordFilterWordParams{
			FilterID: f.ID, NormalizedWord: w.Normalized, Word: w.Word, WholeWord: w.WholeWord, Position: int32(i),
		}); err != nil {
			return postgres.Wrap(err, "insert keyword filter word")
		}
	}
	return nil
}

// DeleteKeywordFilter implements timeline.KeywordFilterRepository.
func (r *Repository) DeleteKeywordFilter(ctx context.Context, tx uow.Tx, id string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, step := range []func(context.Context, string) error{
		q.DeleteKeywordFilterWords, q.DeleteKeywordFilterContexts, q.DeleteKeywordFilterSettings, q.DeleteKeywordFilter,
	} {
		if err := step(ctx, id); err != nil {
			return postgres.Wrap(err, "delete keyword filter")
		}
	}
	return nil
}

// FindKeywordFilter implements timeline.KeywordFilterRepository.
func (r *Repository) FindKeywordFilter(ctx context.Context, tx uow.Tx, id string) (timeline.KeywordFilter, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return timeline.KeywordFilter{}, err
	}
	row, err := q.GetKeywordFilter(ctx, id)
	if err != nil {
		return timeline.KeywordFilter{}, postgres.NotFound(err, apptimeline.ErrNotFound, "keyword filter")
	}
	fs, err := keywordFiltersWithParts(ctx, q, []timelinegen.ListKeywordFiltersRow{timelinegen.ListKeywordFiltersRow(row)})
	if err != nil {
		return timeline.KeywordFilter{}, err
	}
	return fs[0], nil
}

// KeywordFiltersOf implements timeline.KeywordFilterRepository.
func (r *Repository) KeywordFiltersOf(ctx context.Context, accountID string) ([]timeline.KeywordFilter, error) {
	q := r.db.reader()
	rows, err := q.ListKeywordFilters(ctx, accountID)
	if err != nil {
		return nil, postgres.Wrap(err, "list keyword filters")
	}
	return keywordFiltersWithParts(ctx, q, rows)
}

// keywordFiltersWithParts returns the filters of rows with their contexts and words.
func keywordFiltersWithParts(ctx context.Context, q *timelinegen.Queries, rows []timelinegen.ListKeywordFiltersRow) ([]timeline.KeywordFilter, error) {
	out := make([]timeline.KeywordFilter, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	index := make(map[string]int, len(rows))
	ids := make([]string, len(rows))
	for i, row := range rows {
		out[i] = timeline.KeywordFilter{
			ID: row.ID, AccountID: row.AccountID, Title: row.Title, Action: timeline.FilterAction(row.Action),
			ExpiresAt: postgres.FromNullTime(row.ExpiresAt), CreatedAt: postgres.UTC(row.CreatedAt),
		}
		index[row.ID], ids[i] = i, row.ID
	}
	list := ids
	contexts, err := q.ListKeywordFilterContexts(ctx, list)
	if err != nil {
		return nil, postgres.Wrap(err, "list keyword filter contexts")
	}
	for _, c := range contexts {
		f := &out[index[c.FilterID]]
		f.Contexts = append(f.Contexts, timeline.FilterContext(c.Context))
	}
	words, err := q.ListKeywordFilterWords(ctx, list)
	if err != nil {
		return nil, postgres.Wrap(err, "list keyword filter words")
	}
	for _, w := range words {
		f := &out[index[w.FilterID]]
		f.Words = append(f.Words, timeline.FilterWord{Word: w.Word, Normalized: w.NormalizedWord, WholeWord: w.WholeWord})
	}
	return out, nil
}

// CountKeywordFilters implements timeline.KeywordFilterRepository.
func (r *Repository) CountKeywordFilters(ctx context.Context, tx uow.Tx, accountID string) (int, int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, 0, err
	}
	row, err := q.CountKeywordFilters(ctx, accountID)
	return int(row.Filters), int(row.Words), postgres.Wrap(err, "count keyword filters")
}

// ActiveKeywordFilters implements timeline.KeywordFilterRepository.
func (r *Repository) ActiveKeywordFilters(ctx context.Context, accountIDs []string, c timeline.FilterContext, now time.Time) (map[string][]timeline.KeywordFilter, error) {
	out := map[string][]timeline.KeywordFilter{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	list := accountIDs
	rows, err := r.db.reader().ListActiveKeywordFilterWords(ctx, timelinegen.ListActiveKeywordFilterWordsParams{
		Context: string(c), AccountIds: list, Now: now,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list active keyword filter words")
	}
	for _, row := range rows {
		fs := out[row.AccountID]
		if n := len(fs); n == 0 || fs[n-1].ID != row.ID {
			fs = append(fs, timeline.KeywordFilter{
				ID: row.ID, AccountID: row.AccountID, Title: row.Title, Action: timeline.FilterAction(row.Action),
			})
		}
		f := &fs[len(fs)-1]
		f.Words = append(f.Words, timeline.FilterWord{Word: row.Word, Normalized: row.NormalizedWord, WholeWord: row.WholeWord})
		out[row.AccountID] = fs
	}
	return out, nil
}
