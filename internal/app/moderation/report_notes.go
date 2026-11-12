// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The moderators' handling of a report beyond its assignment and resolution (FR-710): internal
// notes, reopening a resolved report, and the history of its target that the report's detail
// shows. Notes are for the moderators only: no use case shows them to the target or the
// reporter.

// ErrReportNotResolved reports the reopening of a report that is open.
var ErrReportNotResolved = errors.New("moderation: the report is not resolved")

// ReportNoteRepository is the persistence port of the notes on reports, the reopenings, and the
// counts of a report's detail, part of Repository.
type ReportNoteRepository interface {
	InsertReportReopening(ctx context.Context, tx uow.Tx, r moderation.ReportReopening) error
	InsertReportNote(ctx context.Context, tx uow.Tx, n moderation.ReportNote) error
	// ListReportNotes returns the notes on a report, oldest first.
	ListReportNotes(ctx context.Context, reportID string) ([]moderation.ReportNote, error)
	// CountOtherReportsOf counts the reports of a target account other than reportID.
	CountOtherReportsOf(ctx context.Context, targetAccountID, reportID string) (int64, error)
	// CountActionsOf counts the actions on an account, lifted and expired ones included.
	CountActionsOf(ctx context.Context, accountID string) (int64, error)
}

// ReportDetail returns a report with its state, its notes, and the history of its target; it
// fails with ErrNotFound for an unknown ID.
func (s *Service) ReportDetail(ctx context.Context, id string) (moderation.ReportDetail, error) {
	rec, err := s.ReportRecord(ctx, id)
	if err != nil {
		return moderation.ReportDetail{}, err
	}
	repo := s.d.Repository
	notes, err := repo.ListReportNotes(ctx, id)
	if err != nil {
		return moderation.ReportDetail{}, err
	}
	reports, err := repo.CountOtherReportsOf(ctx, rec.Report.TargetAccountID, id)
	if err != nil {
		return moderation.ReportDetail{}, err
	}
	actions, err := repo.CountActionsOf(ctx, rec.Report.TargetAccountID)
	if err != nil {
		return moderation.ReportDetail{}, err
	}
	return moderation.ReportDetail{
		ReportRecord: rec, Notes: notes, History: moderation.TargetHistory{Reports: reports, Actions: actions},
	}, nil
}

// AddReportNote records a moderator's note on a report and its audit log entry in one
// transaction. An invalid body fails with an error wrapping moderation.ErrInvalidReportNote,
// an unknown report with ErrNotFound. byUserID is the moderator, "" for the CLI.
func (s *Service) AddReportNote(ctx context.Context, reportID, body, byUserID string) (moderation.ReportNote, error) {
	now := kernel.Normalize(s.d.Clock.Now())
	n := moderation.ReportNote{ID: s.d.IDs.NewID(), ReportID: reportID, AuthorUserID: byUserID, Body: body, WrittenAt: now}
	if err := n.Validate(); err != nil {
		return moderation.ReportNote{}, err
	}
	label, err := s.reportLabel(ctx, reportID)
	if err != nil {
		return moderation.ReportNote{}, err
	}
	auditID := s.d.IDs.NewID()
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertReportNote(ctx, tx, n); err != nil {
			return err
		}
		return s.emitAudit(ctx, tx, moderation.AuditEntry{
			ID: auditID, Action: moderation.AuditReportNoteAdded, TargetKind: moderation.AuditTargetReport,
			TargetID: reportID, TargetLabel: label, SourceEventID: n.ID, OccurredAt: now,
		}, byUserID)
	})
	if err != nil {
		return moderation.ReportNote{}, err
	}
	return n, nil
}

// ReopenReport reopens a resolved report: it records the reopening and, in the same
// transaction, the report's open state (keeping its assignee) and the audit log entry. An open
// report fails with ErrReportNotResolved, an unknown one with ErrNotFound. Resolving the report
// again does not notify its reporter again. byUserID is the moderator, "" for
// the CLI.
func (s *Service) ReopenReport(ctx context.Context, reportID, byUserID string) (moderation.ReportRecord, error) {
	now := kernel.Normalize(s.d.Clock.Now())
	report, err := s.d.Repository.FindReport(ctx, reportID)
	if err != nil {
		return moderation.ReportRecord{}, err
	}
	label, err := s.accountLabel(ctx, report.TargetAccountID)
	if err != nil {
		return moderation.ReportRecord{}, err
	}
	repo := s.d.Repository
	reopeningID, auditID := s.d.IDs.NewID(), s.d.IDs.NewID()
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		st, err := repo.ReportStatusForUpdate(ctx, tx, reportID)
		if err != nil {
			return err
		}
		if st.State != moderation.ReportResolved {
			return fmt.Errorf("%w: %s", ErrReportNotResolved, reportID)
		}
		reopening := moderation.ReportReopening{ID: reopeningID, ReportID: reportID, ReopenedByUserID: byUserID, ReopenedAt: now}
		if err := repo.InsertReportReopening(ctx, tx, reopening); err != nil {
			return err
		}
		st.State, st.Outcome, st.Since = moderation.ReportOpen, "", now
		if err := repo.UpsertReportStatus(ctx, tx, st); err != nil {
			return err
		}
		if err := tx.Emit(ctx, ReportReopened{Report: report, Reopening: reopening}); err != nil {
			return err
		}
		return s.emitAudit(ctx, tx, moderation.AuditEntry{
			ID: auditID, Action: moderation.AuditReportReopened, TargetKind: moderation.AuditTargetReport,
			TargetID: reportID, TargetLabel: label, SourceEventID: reopeningID, OccurredAt: now,
		}, byUserID)
	})
	if err != nil {
		return moderation.ReportRecord{}, err
	}
	return s.ReportRecord(ctx, reportID)
}

// resolveByAction resolves in tx the report that the action a follows from as actioned, with
// the audit log entry of the resolution, unless the report is resolved already (data-model
// §4.3). label is the handle of the target, which is also the report's target.
func (s *Service) resolveByAction(ctx context.Context, tx uow.Tx, report moderation.Report, a moderation.Action, label, resolutionID, auditID string) error {
	resolved, err := s.resolveReport(ctx, tx, report, moderation.ReportResolution{
		ID: resolutionID, ReportID: report.ID, Outcome: moderation.OutcomeActioned,
		ResolvedByUserID: a.IssuedByUserID, ResolvedAt: a.IssuedAt,
	})
	if err != nil || !resolved {
		return err
	}
	return s.emitAudit(ctx, tx, moderation.AuditEntry{
		ID: auditID, Action: moderation.AuditReportResolved, TargetKind: moderation.AuditTargetReport,
		TargetID: report.ID, TargetLabel: label, SourceEventID: resolutionID, OccurredAt: a.IssuedAt,
	}, a.IssuedByUserID)
}
