// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

var _ appmoderation.SignupBlockRepository = (*ModerationRepository)(nil)

// InsertSignupBlock implements moderation.SignupBlockRepository.
func (r *ModerationRepository) InsertSignupBlock(ctx context.Context, tx uow.Tx, b moderation.SignupBlock) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertSignupBlock(ctx, sqlitegen.InsertSignupBlockParams{
		ID: b.ID, Kind: string(b.Kind), Value: b.Value, Severity: string(b.Severity), Comment: toNullString(b.Comment),
		ModerationActionID: toNullString(b.ModerationActionID), CreatedByUserID: toNullString(b.CreatedByUserID),
		ExpiresAt: toNullMillis(b.ExpiresAt), CreatedAt: toMillis(b.CreatedAt),
	}), "insert signup block")
}

func signupBlockFrom(row sqlitegen.SignupBlock) moderation.SignupBlock {
	return moderation.SignupBlock{
		ID: row.ID, Kind: moderation.SignupBlockKind(row.Kind), Value: row.Value, Severity: moderation.SignupBlockSeverity(row.Severity),
		Comment: row.Comment.String, ModerationActionID: row.ModerationActionID.String, CreatedByUserID: row.CreatedByUserID.String,
		ExpiresAt: fromNullMillis(row.ExpiresAt), CreatedAt: fromMillis(row.CreatedAt),
	}
}

// SignupBlock implements moderation.SignupBlockRepository.
func (r *ModerationRepository) SignupBlock(ctx context.Context, tx uow.Tx, id string) (moderation.SignupBlock, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return moderation.SignupBlock{}, err
	}
	row, err := q.GetSignupBlock(ctx, id)
	if err != nil {
		return moderation.SignupBlock{}, notFound(err, appmoderation.ErrNotFound, "signup block "+id)
	}
	return signupBlockFrom(row), nil
}

// InsertSignupBlockLift implements moderation.SignupBlockRepository.
func (r *ModerationRepository) InsertSignupBlockLift(ctx context.Context, tx uow.Tx, l moderation.SignupBlockLift) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertSignupBlockLift(ctx, sqlitegen.InsertSignupBlockLiftParams{
		BlockID: l.BlockID, LiftedByUserID: toNullString(l.LiftedByUserID), LiftedAt: toMillis(l.LiftedAt),
	})
	if err != nil {
		return wrap(err, "insert signup block lift")
	}
	if n == 0 {
		return fmt.Errorf("lift of signup block %s: %w", l.BlockID, appmoderation.ErrAlreadyRecorded)
	}
	return nil
}

func signupBlockRecordFrom(row sqlitegen.ListSignupBlocksRow) moderation.SignupBlockRecord {
	rec := moderation.SignupBlockRecord{Block: signupBlockFrom(sqlitegen.SignupBlock{
		ID: row.ID, Kind: row.Kind, Value: row.Value, Severity: row.Severity, Comment: row.Comment,
		ModerationActionID: row.ModerationActionID, CreatedByUserID: row.CreatedByUserID, ExpiresAt: row.ExpiresAt,
		CreatedAt: row.CreatedAt,
	})}
	if row.LiftBlockID.Valid {
		rec.Lift = &moderation.SignupBlockLift{BlockID: row.ID, LiftedByUserID: row.LiftedByUserID.String, LiftedAt: fromMillis(row.LiftedAt.Int64)}
	}
	return rec
}

// FindSignupBlockRecord implements moderation.SignupBlockRepository.
func (r *ModerationRepository) FindSignupBlockRecord(ctx context.Context, id string) (moderation.SignupBlockRecord, error) {
	row, err := r.db.reader().GetSignupBlockRecord(ctx, id)
	if err != nil {
		return moderation.SignupBlockRecord{}, notFound(err, appmoderation.ErrNotFound, "signup block "+id)
	}
	return signupBlockRecordFrom(sqlitegen.ListSignupBlocksRow(row)), nil
}

// ListSignupBlocks implements moderation.SignupBlockRepository.
func (r *ModerationRepository) ListSignupBlocks(ctx context.Context, q appmoderation.SignupBlockQuery, limit int) ([]moderation.SignupBlockRecord, error) {
	var at int64
	if q.InEffectAt != nil {
		at = toMillis(*q.InEffectAt)
	}
	rows, err := r.db.reader().ListSignupBlocks(ctx, sqlitegen.ListSignupBlocksParams{
		Kind: string(q.Kind), InEffectAt: at, Before: q.Before, Lim: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list signup blocks")
	}
	out := make([]moderation.SignupBlockRecord, len(rows))
	for i, row := range rows {
		out[i] = signupBlockRecordFrom(row)
	}
	return out, nil
}

// SignupBlocksInEffect implements moderation.SignupBlockRepository.
func (r *ModerationRepository) SignupBlocksInEffect(ctx context.Context, m appmoderation.SignupBlockMatch, now time.Time) ([]moderation.SignupBlock, error) {
	domains := m.Domains
	if domains == nil {
		domains = []string{}
	}
	list, err := jsonList(domains)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListSignupBlocksInEffect(ctx, sqlitegen.ListSignupBlocksInEffectParams{
		Now: toMillis(now), Domains: list, EmailHash: m.EmailHash,
	})
	if err != nil {
		return nil, wrap(err, "list signup blocks in effect")
	}
	out := make([]moderation.SignupBlock, len(rows))
	for i, row := range rows {
		out[i] = signupBlockFrom(row)
	}
	return out, nil
}

// SignupBlocksInEffectOf implements moderation.SignupBlockRepository.
func (r *ModerationRepository) SignupBlocksInEffectOf(ctx context.Context, tx uow.Tx, kind moderation.SignupBlockKind, values []string, now time.Time) ([]moderation.SignupBlock, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	if values == nil {
		values = []string{}
	}
	list, err := jsonList(values)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListSignupBlocksInEffectOfValues(ctx, sqlitegen.ListSignupBlocksInEffectOfValuesParams{
		Kind: string(kind), Vals: list, Now: toMillis(now),
	})
	if err != nil {
		return nil, wrap(err, "list signup blocks in effect of values")
	}
	out := make([]moderation.SignupBlock, len(rows))
	for i, row := range rows {
		out[i] = signupBlockFrom(row)
	}
	return out, nil
}
