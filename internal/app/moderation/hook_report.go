// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// HookReportInput is the report that a flag decision of a hook files (ADR-0147 §7).
type HookReportInput struct {
	// ReporterAccountID is the instance actor: a report must have a reporter, and no person is one.
	ReporterAccountID string
	// TargetAccountID is the author of the subject.
	TargetAccountID string
	// StatusIDs are the target's statuses that the decision is about.
	StatusIDs []string
	// DecisionID is the decision, one report for each. Hook and ReasonCode are copied from it, and
	// ReasonCode gives the category.
	DecisionID string
	Hook       string
	ReasonCode string
	// Comment is the plugin's note to the moderators, "" for none.
	Comment string
}

// FileHookReport records inside tx the report of a flag decision, open, in the moderators' queue,
// and emits ReportFiled. It reports whether it filed one: a decision that has a report keeps it, so a
// job that runs again files nothing. The report has no URI and is not forwarded. The caller's
// transaction is the one that stores what the report points at, which is why neither
// FileRemoteReport nor LocalReports.File fits: the first needs a URI and its own transaction, and the
// second checks what a person may see.
func (s *Service) FileHookReport(ctx context.Context, tx uow.Tx, in HookReportInput) (bool, error) {
	now := kernel.Normalize(s.d.Clock.Now())
	r := moderation.Report{
		ID: s.d.IDs.NewID(), ReporterAccountID: in.ReporterAccountID, TargetAccountID: in.TargetAccountID,
		Category: moderation.CategoryOfHookReason(in.ReasonCode), Comment: moderation.TruncateComment(in.Comment), FiledAt: now,
		StatusIDs: uniqueSorted(in.StatusIDs), RuleIDs: []string{},
		HookOrigin: &moderation.ReportHookOrigin{DecisionID: in.DecisionID, Hook: in.Hook, ReasonCode: in.ReasonCode},
	}
	if err := r.Validate(); err != nil {
		return false, err
	}
	repo := s.d.Repository
	ok, err := repo.InsertReport(ctx, tx, r)
	if err != nil || !ok {
		return false, err
	}
	if err := repo.UpsertReportStatus(ctx, tx, moderation.ReportStatus{ReportID: r.ID, State: moderation.ReportOpen, Since: now}); err != nil {
		return false, err
	}
	return true, emitReportFiled(ctx, tx, repo, r)
}
