// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// ReportedStatuses is the part of the content module's public API that filing a local report
// reads: whether the reporter may see a status, decided by content.CanView in ContextStatus.
// *appcontent.Reader implements it.
type ReportedStatuses interface {
	// Visible fails with an error wrapping appcontent.ErrNotFound for a status that is missing,
	// deleted, or that viewer may not see.
	Visible(ctx context.Context, viewer appcontent.Viewer, statusID string) (appcontent.Status, error)
}

// CitableRules is the part of the instance module's public API that filing a local report
// reads. *appinstance.Service implements it.
type CitableRules interface {
	// RuleInEffect reports whether a rule exists and is not retired.
	RuleInEffect(ctx context.Context, id string) (bool, error)
}

// LocalReportDeps are the ports LocalReports uses.
type LocalReportDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Accounts   Accounts
	Statuses   ReportedStatuses
	Rules      CitableRules
	Clock      kernel.Clock
	IDs        kernel.IDGenerator
}

// LocalReports files the reports of local users (FR-701). It is apart from Service because it
// reads the content and instance modules, which are set up after this module.
type LocalReports struct{ d LocalReportDeps }

// NewLocalReports returns LocalReports on d.
func NewLocalReports(d LocalReportDeps) *LocalReports { return &LocalReports{d: d} }

// LocalReportInput is a report that a local user files.
type LocalReportInput struct {
	// ReporterAccountID is the local account of the reporter.
	ReporterAccountID string
	// TargetAccountID is the account reported, local or remote.
	TargetAccountID string
	// StatusIDs are statuses of the target that the reporter may see.
	StatusIDs []string
	Category  moderation.Category
	// RuleIDs are rules in effect; only a report of moderation.CategoryViolation cites rules.
	RuleIDs []string
	// Comment is plain text of at most moderation.MaxLocalReportComment characters.
	Comment string
	// Forward asks to forward the report to the server of a remote target (FR-709); it means
	// nothing for a local target.
	Forward bool
	// VoiceRoom is the voice chat room reported, nil for none (ADR-0100 §11). The voice module
	// checks that the reporter sees the room and names its host as the target and its current
	// revision; such a report points at no statuses and is never forwarded.
	VoiceRoom *moderation.ReportVoiceRoom
}

// ReportFieldError reports a field of a local report that does not hold: an unknown category,
// an account that does not exist or is the reporter's own, a status that the reporter may not
// see or that is not the target's, or rules cited by a report that is not of
// moderation.CategoryViolation or that are not in effect. It wraps
// moderation.ErrInvalidReport.
type ReportFieldError struct {
	// Field is the property of the report, such as account_id, status_ids, or rule_ids.
	Field string
}

func (e *ReportFieldError) Error() string {
	return fmt.Sprintf("%s: the %s of the report do not hold", moderation.ErrInvalidReport, e.Field)
}

// Unwrap returns moderation.ErrInvalidReport.
func (e *ReportFieldError) Unwrap() error { return moderation.ErrInvalidReport }

// File records a local user's report, open, in the moderators' queue: the report, the statuses
// it points at, the rules it cites, and its open state in one transaction, and emits
// ReportFiled. A report that does not hold fails with an error wrapping
// moderation.ErrInvalidReport: a *ReportFieldError, or the *moderation.TextError or
// *moderation.CountError of moderation.Report.ValidateLocal. The report never reaches the
// target: nothing tells them of it, and nothing that they read names the reporter.
func (s *LocalReports) File(ctx context.Context, in LocalReportInput) (moderation.Report, error) {
	now := kernel.Normalize(s.d.Clock.Now())
	r := moderation.Report{
		ID: s.d.IDs.NewID(), ReporterAccountID: in.ReporterAccountID, TargetAccountID: in.TargetAccountID,
		Category: in.Category, Comment: in.Comment, FiledAt: now,
		StatusIDs: uniqueSorted(in.StatusIDs), RuleIDs: uniqueSorted(in.RuleIDs),
	}
	if in.VoiceRoom != nil {
		room := *in.VoiceRoom
		r.VoiceRoom = &room
	}
	switch {
	case !r.Category.Valid():
		return moderation.Report{}, &ReportFieldError{Field: "category"}
	case len(r.RuleIDs) > 0 && r.Category != moderation.CategoryViolation:
		return moderation.Report{}, &ReportFieldError{Field: "rule_ids"}
	case r.VoiceRoom != nil && len(r.StatusIDs) > 0:
		return moderation.Report{}, &ReportFieldError{Field: "status_ids"}
	case r.VoiceRoom != nil && (r.VoiceRoom.RoomID == "" || r.VoiceRoom.RevisionID == ""):
		return moderation.Report{}, &ReportFieldError{Field: "voice_room_id"}
	}
	if err := r.ValidateLocal(); err != nil {
		return moderation.Report{}, err
	}
	if in.TargetAccountID == in.ReporterAccountID {
		return moderation.Report{}, &ReportFieldError{Field: "account_id"}
	}
	target, err := s.d.Accounts.FindByID(ctx, in.TargetAccountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return moderation.Report{}, &ReportFieldError{Field: "account_id"}
	case err != nil:
		return moderation.Report{}, err
	}
	r.Forwarded = in.Forward && !target.Account.IsLocal() && r.VoiceRoom == nil
	viewer := appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: in.ReporterAccountID}
	for _, id := range r.StatusIDs {
		st, err := s.d.Statuses.Visible(ctx, viewer, id)
		switch {
		case errors.Is(err, appcontent.ErrNotFound):
			return moderation.Report{}, &ReportFieldError{Field: "status_ids"}
		case err != nil:
			return moderation.Report{}, err
		case st.AuthorAccountID != r.TargetAccountID:
			return moderation.Report{}, &ReportFieldError{Field: "status_ids"}
		}
	}
	for _, id := range r.RuleIDs {
		ok, err := s.d.Rules.RuleInEffect(ctx, id)
		if err != nil {
			return moderation.Report{}, err
		}
		if !ok {
			return moderation.Report{}, &ReportFieldError{Field: "rule_ids"}
		}
	}
	repo := s.d.Repository
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := repo.InsertReport(ctx, tx, r); err != nil {
			return err
		}
		if err := repo.UpsertReportStatus(ctx, tx, moderation.ReportStatus{ReportID: r.ID, State: moderation.ReportOpen, Since: now}); err != nil {
			return err
		}
		return emitReportFiled(ctx, tx, repo, r)
	})
	if err != nil {
		return moderation.Report{}, err
	}
	return r, nil
}

// FindReport returns a report with its statuses, as the federation module reads it to forward
// the report to the target's server (FR-709). It fails with an error wrapping ErrNotFound for an
// unknown ID.
func (s *Service) FindReport(ctx context.Context, id string) (Report, error) {
	return s.d.Repository.FindReport(ctx, id)
}
