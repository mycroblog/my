// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
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
	ints, err := autoDeleteInts(p)
	if err != nil {
		return err
	}
	err = q.UpsertStatusAutoDeletePolicy(ctx, postgresgen.UpsertStatusAutoDeletePolicyParams{
		AccountID: p.AccountID, Enabled: p.Enabled, MinAgeDays: ints[0],
		KeepPinned: p.KeepPinned, KeepBookmarked: p.KeepBookmarked, KeepFavourited: p.KeepFavourited,
		KeepMedia: p.KeepMedia, KeepPolls: p.KeepPolls,
		KeepMinBoosts: ints[1], KeepMinFavourites: ints[2], UpdatedAt: p.UpdatedAt,
	})
	if err != nil {
		return wrap(err, "save auto delete policy")
	}
	if err := q.DeleteStatusAutoDeleteKeptVisibilities(ctx, p.AccountID); err != nil {
		return wrap(err, "replace auto delete kept visibilities")
	}
	for _, v := range p.KeepVisibilities {
		err := q.InsertStatusAutoDeleteKeptVisibility(ctx, postgresgen.InsertStatusAutoDeleteKeptVisibilityParams{
			AccountID: p.AccountID, Visibility: string(v),
		})
		if err != nil {
			return wrap(err, "insert auto delete kept visibility")
		}
	}
	return nil
}

// autoDeleteInts returns the age and the thresholds of p as the integer columns.
func autoDeleteInts(p content.AutoDeletePolicy) ([3]int32, error) {
	var out [3]int32
	for i, n := range []int{p.MinAgeDays, p.KeepMinBoosts, p.KeepMinFavourites} {
		v, err := toInt32(n, "auto delete policy value")
		if err != nil {
			return out, err
		}
		out[i] = v
	}
	return out, nil
}

// ListEnabledAutoDeletePolicies implements content.AutoDeleteRepository.
func (r *StatusRepository) ListEnabledAutoDeletePolicies(ctx context.Context, after string, limit int) ([]content.AutoDeletePolicy, error) {
	n, err := toInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	q := r.db.reader()
	rows, err := q.ListEnabledStatusAutoDeletePolicies(ctx, postgresgen.ListEnabledStatusAutoDeletePoliciesParams{
		After: toNullString(after), BatchLimit: n,
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
	ints, err := autoDeleteInts(p)
	if err != nil {
		return nil, err
	}
	n, err := toInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	kept := make([]string, 0, len(p.KeepVisibilities))
	for _, v := range p.KeepVisibilities {
		kept = append(kept, string(v))
	}
	rows, err := q.ListStatusesToAutoDelete(ctx, postgresgen.ListStatusesToAutoDeleteParams{
		AccountID: p.AccountID, IDBefore: kernel.UUIDv7Floor(before.Add(time.Millisecond)), PublishedBefore: before,
		KeptVisibilities: kept, KeepPinned: p.KeepPinned, KeepBookmarked: p.KeepBookmarked,
		KeepFavourited: p.KeepFavourited, KeepMedia: p.KeepMedia, KeepPolls: p.KeepPolls,
		KeepMinBoosts: ints[1], KeepMinFavourites: ints[2], BatchLimit: n,
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

func autoDeletePolicyFrom(ctx context.Context, q *postgresgen.Queries, row postgresgen.StatusAutoDeletePolicy) (content.AutoDeletePolicy, error) {
	vs, err := q.ListStatusAutoDeleteKeptVisibilities(ctx, row.AccountID)
	if err != nil {
		return content.AutoDeletePolicy{}, wrap(err, "list auto delete kept visibilities")
	}
	p := content.AutoDeletePolicy{
		AccountID: row.AccountID, Enabled: row.Enabled, MinAgeDays: int(row.MinAgeDays),
		KeepPinned: row.KeepPinned, KeepBookmarked: row.KeepBookmarked, KeepFavourited: row.KeepFavourited,
		KeepMedia: row.KeepMedia, KeepPolls: row.KeepPolls,
		KeepMinBoosts: int(row.KeepMinBoosts), KeepMinFavourites: int(row.KeepMinFavourites), UpdatedAt: row.UpdatedAt.UTC(),
		KeepVisibilities: make([]content.Visibility, 0, len(vs)),
	}
	for _, v := range vs {
		p.KeepVisibilities = append(p.KeepVisibilities, content.Visibility(v))
	}
	// The rows come in name order; the policy keeps them in the order of the visibilities.
	return p.Normalize()
}
