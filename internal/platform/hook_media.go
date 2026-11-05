// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/app/content"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
)

// mediaReview is the hook media.check (ADR-0147 §2), which the media processing and the posts of
// the content module share. It comes after the hooks, the accounts, and the moderation module.
func (a *App) mediaReview() *content.MediaReview {
	if a.mediaCheck == nil {
		a.mediaCheck = &content.MediaReview{Gate: a.Hooks, Reports: hookReports{a.Moderation}, InstanceActor: a.emojiImageOwner}
	}
	return a.mediaCheck
}

// hookReports files the reports of the content module's hook decisions on the moderation module,
// which the content module may not import.
type hookReports struct{ m *appmoderation.Service }

func (r hookReports) FileHookReport(ctx context.Context, tx uow.Tx, in content.HookReport) (bool, error) {
	return r.m.FileHookReport(ctx, tx, appmoderation.HookReportInput{
		ReporterAccountID: in.ReporterAccountID, TargetAccountID: in.TargetAccountID, StatusIDs: in.StatusIDs,
		DecisionID: in.DecisionID, Hook: in.Hook, ReasonCode: in.ReasonCode, Comment: in.Comment,
	})
}
