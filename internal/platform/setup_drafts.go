// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/queue"
)

// setUpDrafts builds the users' drafts and scheduled posts (ADR-0156) and registers the job that
// publishes a scheduled draft at its time, which the transaction that schedules it enqueues. It
// comes after the content module, which publishes the drafts as new posts.
func (a *App) setUpDrafts(drafts content.DraftRepository, medias content.MediaRepository, outbox queue.Outbox) {
	a.Drafts = content.NewDraftService(content.DraftDeps{
		UnitOfWork: a.UnitOfWork, Drafts: drafts, Media: medias, Statuses: a.Content, Outbox: outbox,
		Limits: a.draftLimits, Clock: a.Clock, IDs: a.IDs,
	})
	a.Drafts.RegisterJobs(a.Handlers)
}

// draftLimits returns the limits of drafts and scheduled posts of the instance settings.
func (a *App) draftLimits(ctx context.Context) (content.DraftLimits, error) {
	s, err := a.Instance.Settings(ctx)
	if err != nil {
		return content.DraftLimits{}, err
	}
	return content.DraftLimits{
		MaxDrafts: s.MaxDrafts(), MaxScheduledPosts: s.MaxScheduledPosts(), MaxScheduledPerDay: s.MaxScheduledPostsPerDay(),
		MinLeadMinutes: s.ScheduledPostMinLeadMinutes(), MaxLeadDays: s.ScheduledPostMaxLeadDays(),
	}, nil
}
