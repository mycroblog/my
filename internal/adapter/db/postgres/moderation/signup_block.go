// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/moderationgen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

var _ appmoderation.SignupBlockRepository = (*Repository)(nil)

// InsertSignupBlock implements moderation.SignupBlockRepository.
func (r *Repository) InsertSignupBlock(ctx context.Context, tx uow.Tx, b moderation.SignupBlock) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertSignupBlock(ctx, moderationgen.InsertSignupBlockParams{
		ID: b.ID, Kind: string(b.Kind), Value: b.Value, Severity: string(b.Severity), Comment: postgres.ToNullString(b.Comment),
		ModerationActionID: postgres.ToNullString(b.ModerationActionID), CreatedByUserID: postgres.ToNullString(b.CreatedByUserID),
		ExpiresAt: b.ExpiresAt, CreatedAt: b.CreatedAt,
	}), "insert signup block")
}

func signupBlockFrom(row moderationgen.SignupBlock) moderation.SignupBlock {
	return moderation.SignupBlock{
		ID: row.ID, Kind: moderation.SignupBlockKind(row.Kind), Value: row.Value, Severity: moderation.SignupBlockSeverity(row.Severity),
		Comment: postgres.FromNullString(row.Comment), ModerationActionID: postgres.FromNullString(row.ModerationActionID),
		CreatedByUserID: postgres.FromNullString(row.CreatedByUserID), ExpiresAt: postgres.UTCPtr(row.ExpiresAt), CreatedAt: postgres.UTC(row.CreatedAt),
	}
}

// SignupBlock implements moderation.SignupBlockRepository.
func (r *Repository) SignupBlock(ctx context.Context, tx uow.Tx, id string) (moderation.SignupBlock, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return moderation.SignupBlock{}, err
	}
	row, err := q.GetSignupBlock(ctx, id)
	if err != nil {
		return moderation.SignupBlock{}, postgres.NotFound(err, appmoderation.ErrNotFound, "signup block "+id)
	}
	return signupBlockFrom(row), nil
}

// InsertSignupBlockLift implements moderation.SignupBlockRepository.
func (r *Repository) InsertSignupBlockLift(ctx context.Context, tx uow.Tx, l moderation.SignupBlockLift) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertSignupBlockLift(ctx, moderationgen.InsertSignupBlockLiftParams{
		BlockID: l.BlockID, LiftedByUserID: postgres.ToNullString(l.LiftedByUserID), LiftedAt: l.LiftedAt,
	})
	if err != nil {
		return postgres.Wrap(err, "insert signup block lift")
	}
	if n == 0 {
		return fmt.Errorf("lift of signup block %s: %w", l.BlockID, appmoderation.ErrAlreadyRecorded)
	}
	return nil
}

func signupBlockRecordFrom(row moderationgen.ListSignupBlocksRow) moderation.SignupBlockRecord {
	rec := moderation.SignupBlockRecord{Block: signupBlockFrom(moderationgen.SignupBlock{
		ID: row.ID, Kind: row.Kind, Value: row.Value, Severity: row.Severity, Comment: row.Comment,
		ModerationActionID: row.ModerationActionID, CreatedByUserID: row.CreatedByUserID, ExpiresAt: row.ExpiresAt,
		CreatedAt: row.CreatedAt,
	})}
	if row.LiftBlockID != nil && row.LiftedAt != nil {
		rec.Lift = &moderation.SignupBlockLift{BlockID: row.ID, LiftedByUserID: postgres.FromNullString(row.LiftedByUserID), LiftedAt: postgres.UTC(*row.LiftedAt)}
	}
	return rec
}

// FindSignupBlockRecord implements moderation.SignupBlockRepository.
func (r *Repository) FindSignupBlockRecord(ctx context.Context, id string) (moderation.SignupBlockRecord, error) {
	row, err := r.db.reader().GetSignupBlockRecord(ctx, id)
	if err != nil {
		return moderation.SignupBlockRecord{}, postgres.NotFound(err, appmoderation.ErrNotFound, "signup block "+id)
	}
	return signupBlockRecordFrom(moderationgen.ListSignupBlocksRow(row)), nil
}

// ListSignupBlocks implements moderation.SignupBlockRepository.
func (r *Repository) ListSignupBlocks(ctx context.Context, q appmoderation.SignupBlockQuery, limit int) ([]moderation.SignupBlockRecord, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListSignupBlocks(ctx, moderationgen.ListSignupBlocksParams{
		Kind: string(q.Kind), InEffectAt: q.InEffectAt, Before: postgres.ToNullString(q.Before), Lim: lim,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list signup blocks")
	}
	out := make([]moderation.SignupBlockRecord, len(rows))
	for i, row := range rows {
		out[i] = signupBlockRecordFrom(row)
	}
	return out, nil
}

// SignupBlocksInEffect implements moderation.SignupBlockRepository.
func (r *Repository) SignupBlocksInEffect(ctx context.Context, m appmoderation.SignupBlockMatch, now time.Time) ([]moderation.SignupBlock, error) {
	domains := m.Domains
	if domains == nil {
		domains = []string{}
	}
	rows, err := r.db.reader().ListSignupBlocksInEffect(ctx, moderationgen.ListSignupBlocksInEffectParams{
		Now: now, Domains: domains, EmailHash: m.EmailHash,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list signup blocks in effect")
	}
	out := make([]moderation.SignupBlock, len(rows))
	for i, row := range rows {
		out[i] = signupBlockFrom(row)
	}
	return out, nil
}

// SignupBlocksInEffectOf implements moderation.SignupBlockRepository.
func (r *Repository) SignupBlocksInEffectOf(ctx context.Context, tx uow.Tx, kind moderation.SignupBlockKind, values []string, now time.Time) ([]moderation.SignupBlock, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	if values == nil {
		values = []string{}
	}
	rows, err := q.ListSignupBlocksInEffectOfValues(ctx, moderationgen.ListSignupBlocksInEffectOfValuesParams{
		Kind: string(kind), Vals: values, Now: now,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list signup blocks in effect of values")
	}
	out := make([]moderation.SignupBlock, len(rows))
	for i, row := range rows {
		out[i] = signupBlockFrom(row)
	}
	return out, nil
}
