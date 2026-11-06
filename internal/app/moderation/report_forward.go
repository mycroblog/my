// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// A moderator's forward of a report to the target's server after it was filed (FR-709, ADR-0147
// §7): a report that a decision hook filed is never forwarded then, and neither is one whose
// reporter did not ask, so a moderator decides it later. The federation module subscribes to
// ReportForwarded and sends the Flag of the instance actor, as it does for a report forwarded
// when it was filed.

// ErrReportNotForwardable reports the forward of a report that cannot be forwarded: one received
// from another server, one of a voice chat room, or one of a local account.
var ErrReportNotForwardable = errors.New("moderation: the report cannot be forwarded")

// EventReportForwarded follows a moderator's forward of a report after it was filed.
const EventReportForwarded = "moderation.report_forwarded"

// ReportForwarded carries a report that a moderator forwarded, with the forward in
// Report.Forward.
type ReportForwarded struct{ Report moderation.Report }

// EventType implements uow.Event.
func (ReportForwarded) EventType() string { return EventReportForwarded }

// ReportForwardRepository is the persistence port of the forwards of reports, part of
// Repository.
type ReportForwardRepository interface {
	// InsertReportForward records f and reports whether it did: a report has one forward at
	// most, and a second inserts nothing.
	InsertReportForward(ctx context.Context, tx uow.Tx, f moderation.ReportForward) (bool, error)
}

// ForwardReport forwards a report to the server of its remote target, open or resolved: it
// records the forward, the audit log entry, and ReportForwarded, which sends the Flag, in one
// transaction. A report forwarded already, when it was filed or later, records nothing, so a
// request sent again changes nothing. It fails with ErrNotFound for an unknown report and with
// ErrReportNotForwardable for one that cannot be forwarded. byUserID is the moderator, "" for
// the CLI.
func (s *Service) ForwardReport(ctx context.Context, reportID, byUserID string) (moderation.ReportRecord, error) {
	// A report never changes once filed, and its forward is checked again in the transaction.
	report, err := s.d.Repository.FindReport(ctx, reportID)
	if err != nil {
		return moderation.ReportRecord{}, err
	}
	if !report.SentToTarget() {
		target, err := s.d.Accounts.FindByID(ctx, report.TargetAccountID)
		switch {
		case errors.Is(err, appaccount.ErrNotFound):
			return moderation.ReportRecord{}, fmt.Errorf("%w: the target %s is gone", ErrReportNotForwardable, report.TargetAccountID)
		case err != nil:
			return moderation.ReportRecord{}, err
		case !report.Forwardable(!target.Account.IsLocal()):
			return moderation.ReportRecord{}, fmt.Errorf("%w: %s", ErrReportNotForwardable, reportID)
		}
		if err := s.forwardReport(ctx, report, actorLabel(target), byUserID); err != nil {
			return moderation.ReportRecord{}, err
		}
	}
	return s.ReportRecord(ctx, reportID)
}

func (s *Service) forwardReport(ctx context.Context, report moderation.Report, label, byUserID string) error {
	now := kernel.Normalize(s.d.Clock.Now())
	f := moderation.ReportForward{ID: s.d.IDs.NewID(), ReportID: report.ID, ForwardedByUserID: byUserID, ForwardedAt: now}
	auditID := s.d.IDs.NewID()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ok, err := s.d.Repository.InsertReportForward(ctx, tx, f)
		if err != nil || !ok {
			return err // !ok: a request at the same time forwarded it
		}
		report.Forward = &f
		if err := tx.Emit(ctx, ReportForwarded{Report: report}); err != nil {
			return err
		}
		return s.emitAudit(ctx, tx, moderation.AuditEntry{
			ID: auditID, Action: moderation.AuditReportForwarded, TargetKind: moderation.AuditTargetReport,
			TargetID: report.ID, TargetLabel: label, SourceEventID: f.ID, OccurredAt: now,
		}, byUserID)
	})
}
