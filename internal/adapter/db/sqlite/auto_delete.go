// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

var _ appcontent.AutoDeleteRepository = (*StatusRepository)(nil)

// FindAutoDeletePolicy implements content.AutoDeleteRepository.
func (r *StatusRepository) FindAutoDeletePolicy(ctx context.Context, accountID string) (content.AutoDeletePolicy, error) {
	q := r.db.reader()
	row, err := q.GetStatusAutoDeletePolicy(ctx, accountID)
	if err != nil {
		return content.AutoDeletePolicy{}, notFound(err, appcontent.ErrNotFound, "auto delete policy of "+accountID)
	}
	return autoDeletePolicyFrom(ctx, q, row)
}

// SaveAutoDeletePolicy implements content.AutoDeleteRepository.
func (r *StatusRepository) SaveAutoDeletePolicy(ctx context.Context, tx uow.Tx, p content.AutoDeletePolicy) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertStatusAutoDeletePolicy(ctx, sqlitegen.UpsertStatusAutoDeletePolicyParams{
		AccountID: p.AccountID, Enabled: boolToInt(p.Enabled), MinAgeDays: int64(p.MinAgeDays),
		KeepPinned: boolToInt(p.KeepPinned), KeepBookmarked: boolToInt(p.KeepBookmarked),
		KeepFavourited: boolToInt(p.KeepFavourited), KeepMedia: boolToInt(p.KeepMedia), KeepPolls: boolToInt(p.KeepPolls),
		KeepMinBoosts: int64(p.KeepMinBoosts), KeepMinFavourites: int64(p.KeepMinFavourites), UpdatedAt: toMillis(p.UpdatedAt),
	})
	if err != nil {
		return wrap(err, "save auto delete policy")
	}
	if err := q.DeleteStatusAutoDeleteKeptVisibilities(ctx, p.AccountID); err != nil {
		return wrap(err, "replace auto delete kept visibilities")
	}
	for _, v := range p.KeepVisibilities {
		err := q.InsertStatusAutoDeleteKeptVisibility(ctx, sqlitegen.InsertStatusAutoDeleteKeptVisibilityParams{
			AccountID: p.AccountID, Visibility: string(v),
		})
		if err != nil {
			return wrap(err, "insert auto delete kept visibility")
		}
	}
	return nil
}

// ListEnabledAutoDeletePolicies implements content.AutoDeleteRepository.
func (r *StatusRepository) ListEnabledAutoDeletePolicies(ctx context.Context, after string, limit int) ([]content.AutoDeletePolicy, error) {
	q := r.db.reader()
	rows, err := q.ListEnabledStatusAutoDeletePolicies(ctx, sqlitegen.ListEnabledStatusAutoDeletePoliciesParams{
		After: after, BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list enabled auto delete policies")
	}
	out := make([]content.AutoDeletePolicy, 0, len(rows))
	for _, row := range rows {
		p, err := autoDeletePolicyFrom(ctx, q, row)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// ListStatusesToAutoDelete implements content.AutoDeleteRepository. The ID of a local status is
// stamped with its publication time, so the scan of the author's statuses stops at the UUIDv7
// floor just past before.
func (r *StatusRepository) ListStatusesToAutoDelete(ctx context.Context, tx uow.Tx, p content.AutoDeletePolicy, before time.Time, limit int) ([]content.Status, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	kept := make([]string, 0, len(p.KeepVisibilities))
	for _, v := range p.KeepVisibilities {
		kept = append(kept, string(v))
	}
	keptJSON, err := jsonList(kept)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListStatusesToAutoDelete(ctx, sqlitegen.ListStatusesToAutoDeleteParams{
		AccountID: p.AccountID, IDBefore: kernel.UUIDv7Floor(before.Add(time.Millisecond)), PublishedBefore: toMillis(before),
		KeptVisibilities: keptJSON, KeepPinned: boolToInt(p.KeepPinned), KeepBookmarked: boolToInt(p.KeepBookmarked),
		KeepFavourited: boolToInt(p.KeepFavourited), KeepMedia: boolToInt(p.KeepMedia), KeepPolls: boolToInt(p.KeepPolls),
		KeepMinBoosts: int64(p.KeepMinBoosts), KeepMinFavourites: int64(p.KeepMinFavourites), BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list statuses to auto delete")
	}
	out := make([]content.Status, 0, len(rows))
	for _, row := range rows {
		out = append(out, statusFrom(row))
	}
	return out, nil
}

func autoDeletePolicyFrom(ctx context.Context, q *sqlitegen.Queries, row sqlitegen.StatusAutoDeletePolicy) (content.AutoDeletePolicy, error) {
	vs, err := q.ListStatusAutoDeleteKeptVisibilities(ctx, row.AccountID)
	if err != nil {
		return content.AutoDeletePolicy{}, wrap(err, "list auto delete kept visibilities")
	}
	p := content.AutoDeletePolicy{
		AccountID: row.AccountID, Enabled: row.Enabled == 1, MinAgeDays: int(row.MinAgeDays),
		KeepPinned: row.KeepPinned == 1, KeepBookmarked: row.KeepBookmarked == 1, KeepFavourited: row.KeepFavourited == 1,
		KeepMedia: row.KeepMedia == 1, KeepPolls: row.KeepPolls == 1,
		KeepMinBoosts: int(row.KeepMinBoosts), KeepMinFavourites: int(row.KeepMinFavourites), UpdatedAt: fromMillis(row.UpdatedAt),
		KeepVisibilities: make([]content.Visibility, 0, len(vs)),
	}
	for _, v := range vs {
		p.KeepVisibilities = append(p.KeepVisibilities, content.Visibility(v))
	}
	// The rows come in name order; the policy keeps them in the order of the visibilities.
	return p.Normalize()
}
