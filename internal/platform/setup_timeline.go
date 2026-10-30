// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/timeline"
)

// setUpTimeline registers the timeline module's projectors and builds its service. It comes
// after the content and graph projectors, which reject the commands that must not reach the
// timelines (data-model §4.6). It also builds the prefix search of hashtags over the tags of
// statuses and the tag_usage projection (ADR-0092 §1).
func (a *App) setUpTimeline(repo timeline.Repository, statuses appcontent.Repository) {
	timeline.RegisterProjectors(a.Projectors, repo)
	timeline.RegisterModerationProjectors(a.Projectors, repo, a.Accounts)
	a.Timeline = timeline.NewService(timeline.Deps{UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock, IDs: a.IDs})
	a.TagSearch = timeline.NewTagSearch(statuses, repo)
}
