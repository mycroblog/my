// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
)

// setUpTimeline registers the timeline module's projectors and builds its service. It comes
// after the content and graph projectors, which reject the commands that must not reach the
// timelines (data-model §4.6). It also builds the prefix search of hashtags over the tags of
// statuses and the tag_usage projection (ADR-0092 §1). The direct messages by conversation
// resolve merged conversations, and the followed hashtags find and store their tags, through the
// content service.
func (a *App) setUpTimeline(repo timeline.Repository, statuses appcontent.Repository) {
	timeline.RegisterProjectors(a.Projectors, repo, moderatedEffects{a})
	timeline.RegisterModerationProjectors(a.Projectors, repo, a.Accounts)
	a.Timeline = timeline.NewService(timeline.Deps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock, IDs: a.IDs,
		Conversations: a.Content, Stream: timelineStreams{a}, Tags: a.Content,
	})
	a.TagSearch = timeline.NewTagSearch(statuses, repo)
}

// timelineStreams signals the streams for the timeline service, which is built before
// setUpStreams builds the broker; it is read when a command signals.
type timelineStreams struct{ a *App }

// Publish implements stream.Publisher.
func (t timelineStreams) Publish(e stream.Event) {
	if t.a.Streams != nil {
		t.a.Streams.Publish(e)
	}
}

// moderatedEffects reads the restrictions in effect on an account for the timeline projectors. The
// moderation policy is built after them; it is read when they run.
type moderatedEffects struct{ a *App }

// AccountEffectsTx implements timeline.ModeratedEffects.
func (m moderatedEffects) AccountEffectsTx(ctx context.Context, tx uow.Tx, accountID string) ([]moderation.Effect, error) {
	return m.a.AccountPolicy.AccountEffectsTx(ctx, tx, accountID)
}
