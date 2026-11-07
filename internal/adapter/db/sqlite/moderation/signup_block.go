// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/moderationgen"
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
	return sqlite.Wrap(q.InsertSignupBlock(ctx, moderationgen.InsertSignupBlockParams{
		ID: b.ID, Kind: string(b.Kind), Value: b.Value, Severity: string(b.Severity), Comment: sqlite.ToNullString(b.Comment),
		ModerationActionID: sqlite.ToNullString(b.ModerationActionID), CreatedByUserID: sqlite.ToNullString(b.CreatedByUserID),
		ExpiresAt: sqlite.ToNullMillis(b.ExpiresAt), CreatedAt: sqlite.ToMillis(b.CreatedAt),
	}), "insert signup block")
}

func signupBlockFrom(row moderationgen.SignupBlock) moderation.SignupBlock {
	return moderation.SignupBlock{
		ID: row.ID, Kind: moderation.SignupBlockKind(row.Kind), Value: row.Value, Severity: moderation.SignupBlockSeverity(row.Severity),
		Comment: row.Comment.String, ModerationActionID: row.ModerationActionID.String, CreatedByUserID: row.CreatedByUserID.String,
		ExpiresAt: sqlite.FromNullMillis(row.ExpiresAt), CreatedAt: sqlite.FromMillis(row.CreatedAt),
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
		return moderation.SignupBlock{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "signup block "+id)
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
		BlockID: l.BlockID, LiftedByUserID: sqlite.ToNullString(l.LiftedByUserID), LiftedAt: sqlite.ToMillis(l.LiftedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert signup block lift")
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
	if row.LiftBlockID.Valid {
		rec.Lift = &moderation.SignupBlockLift{BlockID: row.ID, LiftedByUserID: row.LiftedByUserID.String, LiftedAt: sqlite.FromMillis(row.LiftedAt.Int64)}
	}
	return rec
}

// FindSignupBlockRecord implements moderation.SignupBlockRepository.
func (r *Repository) FindSignupBlockRecord(ctx context.Context, id string) (moderation.SignupBlockRecord, error) {
	row, err := r.db.reader().GetSignupBlockRecord(ctx, id)
	if err != nil {
		return moderation.SignupBlockRecord{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "signup block "+id)
	}
	return signupBlockRecordFrom(moderationgen.ListSignupBlocksRow(row)), nil
}

// ListSignupBlocks implements moderation.SignupBlockRepository.
func (r *Repository) ListSignupBlocks(ctx context.Context, q appmoderation.SignupBlockQuery, limit int) ([]moderation.SignupBlockRecord, error) {
	var at int64
	if q.InEffectAt != nil {
		at = sqlite.ToMillis(*q.InEffectAt)
	}
	rows, err := r.db.reader().ListSignupBlocks(ctx, moderationgen.ListSignupBlocksParams{
		Kind: string(q.Kind), InEffectAt: at, Before: q.Before, Lim: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list signup blocks")
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
	list, err := sqlite.JSONList(domains)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListSignupBlocksInEffect(ctx, moderationgen.ListSignupBlocksInEffectParams{
		Now: sqlite.ToMillis(now), Domains: list, EmailHash: m.EmailHash,
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list signup blocks in effect")
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
	list, err := sqlite.JSONList(values)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListSignupBlocksInEffectOfValues(ctx, moderationgen.ListSignupBlocksInEffectOfValuesParams{
		Kind: string(kind), Vals: list, Now: sqlite.ToMillis(now),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list signup blocks in effect of values")
	}
	out := make([]moderation.SignupBlock, len(rows))
	for i, row := range rows {
		out[i] = signupBlockFrom(row)
	}
	return out, nil
}
