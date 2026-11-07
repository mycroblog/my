// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// AccountRestrictionsInEffectTx implements the reads in a transaction.
func (r *Repository) AccountRestrictionsInEffectTx(ctx context.Context, tx uow.Tx, accountID string, now time.Time) ([]moderation.Restriction, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).AccountRestrictionsInEffect(ctx, accountID, now)
}

// DomainRestrictionsInEffectTx implements the reads in a transaction.
func (r *Repository) DomainRestrictionsInEffectTx(ctx context.Context, tx uow.Tx, domains []string, now time.Time) ([]moderation.Restriction, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).DomainRestrictionsInEffect(ctx, domains, now)
}

// FindReportTx implements the reads in a transaction.
func (r *Repository) FindReportTx(ctx context.Context, tx uow.Tx, id string) (moderation.Report, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return moderation.Report{}, err
	}
	return (&Repository{db: db}).FindReport(ctx, id)
}

// FindReportStatusTx implements the reads in a transaction.
func (r *Repository) FindReportStatusTx(ctx context.Context, tx uow.Tx, reportID string) (moderation.ReportStatus, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return moderation.ReportStatus{}, err
	}
	return (&Repository{db: db}).FindReportStatus(ctx, reportID)
}

// FindActionRecordTx implements the reads in a transaction.
func (r *Repository) FindActionRecordTx(ctx context.Context, tx uow.Tx, id string) (moderation.ActionRecord, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return moderation.ActionRecord{}, err
	}
	return (&Repository{db: db}).FindActionRecord(ctx, id)
}

// FindDomainActionRecordTx implements the reads in a transaction.
func (r *Repository) FindDomainActionRecordTx(ctx context.Context, tx uow.Tx, id string) (moderation.DomainActionRecord, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return moderation.DomainActionRecord{}, err
	}
	return (&Repository{db: db}).FindDomainActionRecord(ctx, id)
}

// FindAppealTx implements the reads in a transaction.
func (r *Repository) FindAppealTx(ctx context.Context, tx uow.Tx, id string) (moderation.AppealRecord, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return moderation.AppealRecord{}, err
	}
	return (&Repository{db: db}).FindAppeal(ctx, id)
}

// HeldAccountIDsTx implements the reads in a transaction.
func (r *Repository) HeldAccountIDsTx(ctx context.Context, tx uow.Tx) ([]string, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).HeldAccountIDs(ctx)
}

// ReportedVoiceRoomsTx implements the reads in a transaction.
func (r *Repository) ReportedVoiceRoomsTx(ctx context.Context, tx uow.Tx, roomIDs []string) (map[string]bool, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).ReportedVoiceRooms(ctx, roomIDs)
}
