// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/moderationgen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

var _ appmoderation.AuditRepository = (*Repository)(nil)

// InsertAuditEntry implements moderation.AuditRepository.
func (r *Repository) InsertAuditEntry(ctx context.Context, tx uow.Tx, e moderation.AuditEntry) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertAuditLogEntry(ctx, moderationgen.InsertAuditLogEntryParams{
		ID: e.ID, ActorKind: string(e.ActorKind), ActorUserID: postgres.ToNullString(e.ActorUserID), Action: string(e.Action),
		TargetKind: string(e.TargetKind), TargetID: e.TargetID, TargetLabel: e.TargetLabel,
		SourceEventID: postgres.ToNullString(e.SourceEventID), OccurredAt: e.OccurredAt,
	}), "insert audit log entry")
}

// ListAuditEntries implements moderation.AuditRepository.
func (r *Repository) ListAuditEntries(ctx context.Context, q appmoderation.AuditQuery, limit int) ([]moderation.AuditEntry, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListAuditLogEntries(ctx, moderationgen.ListAuditLogEntriesParams{
		ActorUserID: postgres.ToNullString(q.ActorUserID), Action: string(q.Action), TargetKind: string(q.TargetKind), TargetID: q.TargetID,
		Before: postgres.ToNullString(q.Before), Lim: lim,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list audit log entries")
	}
	out := make([]moderation.AuditEntry, len(rows))
	for i, row := range rows {
		out[i] = moderation.AuditEntry{
			ID: row.ID, ActorKind: moderation.AuditActorKind(row.ActorKind), ActorUserID: postgres.FromNullString(row.ActorUserID),
			Action: moderation.AuditAction(row.Action), TargetKind: moderation.AuditTargetKind(row.TargetKind),
			TargetID: row.TargetID, TargetLabel: row.TargetLabel, SourceEventID: postgres.FromNullString(row.SourceEventID),
			OccurredAt: row.OccurredAt.UTC(),
		}
	}
	return out, nil
}

// ExistingAuditTargets implements moderation.AuditRepository.
func (r *Repository) ExistingAuditTargets(ctx context.Context, kind moderation.AuditTargetKind, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	q := r.db.reader()
	var out []string
	var err error
	switch kind {
	case moderation.AuditTargetReport:
		out, err = q.ListExistingReportIDs(ctx, ids)
	case moderation.AuditTargetAppeal:
		out, err = q.ListExistingAppealIDs(ctx, ids)
	case moderation.AuditTargetSignupBlock:
		out, err = q.ListExistingSignupBlockIDs(ctx, ids)
	default:
		return nil, fmt.Errorf("postgres: the existence of %s targets is not recorded here", kind)
	}
	if err != nil {
		return nil, postgres.Wrap(err, "list existing "+string(kind)+" targets")
	}
	return out, nil
}
