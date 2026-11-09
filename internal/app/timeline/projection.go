// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// errEntryExists reports a second entry with one ID. Entry IDs are status and boost IDs, which
// their modules insert once, so it means that the projectors are registered twice.
var errEntryExists = errors.New("timeline: the feed entry already exists")

// RegisterProjectors registers the timeline module's synchronous projectors (data-model §4.6,
// §9). They read only the events and the feed_* tables:
//   - feed_entries holds a post entry for each status that is not deleted, with the language of
//     its current revision, and a boost entry for each live boost of such a status;
//   - feed_direct_recipients holds the current mentions of each direct post;
//   - feed_tag_entries holds the current tags of each post, and tag_usage counts those of the
//     posts that CountsTagUsage (ADR-0092 §1);
//   - feed_follow_edges holds each active follow with its show_boosts preference, and the end of
//     a follow takes its followee out of every list of the follower (list_members);
//   - feed_exclusions holds both directions of each block in force and each mute;
//   - feed_domain_mutes holds each domain mute;
//   - feed_author_restrictions holds profile_view for each deleted account, local or remote,
//     whose entries disappear at once while the purge removes them later.
//
// Register them after the content projectors, so that a command that those reject (such as a
// concurrent boost of the same pair) never reaches them. RebuildProjections applies the same
// projectors to the events that the source modules replay, so updating incrementally and
// rebuilding agree.
func RegisterProjectors(r *uow.Projectors, repo Repository, moderated ModeratedEffects) {
	r.Register(appcontent.EventStatusCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appcontent.StatusCreated)
		s, rev := ev.Status, ev.Revision
		domain, err := domainOf(s.IsLocal, s.URI)
		if err != nil {
			return err
		}
		entry := Entry{
			ID: s.ID, Kind: timeline.KindPost, StatusID: s.ID,
			AuthorAccountID: s.AuthorAccountID, AuthorDomain: domain,
			OriginalAuthorAccountID: s.AuthorAccountID, OriginalAuthorDomain: domain,
			Visibility: s.Visibility, IsLocal: s.IsLocal, Language: language(rev.Language), RevisionSeq: rev.Seq,
			IsReply: s.InReplyToURI != "",
		}
		if err := insertEntry(ctx, tx, repo, entry); err != nil {
			return err
		}
		return projectRevision(ctx, tx, repo, entry, rev.MentionedAccountIDs, rev.TagIDs, rev.RevisedAt)
	})
	r.Register(appcontent.EventStatusRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		rev := e.(appcontent.StatusRevised).Revision
		entry, err := repo.Entry(ctx, tx, rev.StatusID)
		if errors.Is(err, ErrNotFound) {
			return nil // the status is deleted
		}
		if err != nil || rev.Seq <= entry.RevisionSeq {
			return err
		}
		if err := repo.UpdateRevision(ctx, tx, rev.StatusID, language(rev.Language), rev.Seq); err != nil {
			return err
		}
		return projectRevision(ctx, tx, repo, entry, rev.MentionedAccountIDs, rev.TagIDs, rev.RevisedAt)
	})
	r.Register(appcontent.EventStatusDeleted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.DeleteStatusEntries(ctx, tx, e.(appcontent.StatusDeleted).Status.ID)
	})
	r.Register(appcontent.EventStatusesDeleted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ds := e.(appcontent.StatusesDeleted).Deletions
		ids := make([]string, len(ds))
		for i, d := range ds {
			ids[i] = d.Status.ID
		}
		return repo.DeleteStatusEntriesOf(ctx, tx, ids)
	})
	r.Register(appcontent.EventBoosted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appcontent.Boosted)
		b := ev.Boost
		post, err := repo.Entry(ctx, tx, b.StatusID)
		if errors.Is(err, ErrNotFound) {
			return nil // the status is deleted
		}
		if err != nil {
			return err
		}
		domain, err := domainOf(b.IsLocal, b.URI)
		if err != nil {
			return err
		}
		return insertEntry(ctx, tx, repo, Entry{
			ID: b.ID, Kind: timeline.KindBoost, StatusID: b.StatusID,
			AuthorAccountID: b.BoosterAccountID, AuthorDomain: domain,
			OriginalAuthorAccountID: post.OriginalAuthorAccountID, OriginalAuthorDomain: post.OriginalAuthorDomain,
			Visibility: b.Visibility, IsLocal: b.IsLocal, Language: post.Language, RevisionSeq: post.RevisionSeq,
		})
	})
	r.Register(appcontent.EventBoostRetracted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.DeleteEntry(ctx, tx, e.(appcontent.BoostRetracted).Boost.ID)
	})

	r.Register(appgraph.EventFollowed, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appgraph.Followed)
		return repo.UpsertFollowEdge(ctx, tx, FollowEdge{
			FollowerAccountID: ev.Follow.FollowerAccountID, FolloweeAccountID: ev.Follow.FolloweeAccountID, ShowBoosts: ev.Preferences.ShowBoosts,
		})
	})
	r.Register(appgraph.EventFollowsCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		var es []FollowEdge
		for _, f := range e.(appgraph.FollowsCreated).Followed {
			es = append(es, FollowEdge{FollowerAccountID: f.Follow.FollowerAccountID, FolloweeAccountID: f.Follow.FolloweeAccountID, ShowBoosts: f.Preferences.ShowBoosts})
		}
		return repo.UpsertFollowEdges(ctx, tx, es)
	})
	r.Register(appgraph.EventFollowAccepted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appgraph.FollowAccepted)
		return repo.UpsertFollowEdge(ctx, tx, FollowEdge{
			FollowerAccountID: ev.Follow.FollowerAccountID, FolloweeAccountID: ev.Follow.FolloweeAccountID, ShowBoosts: ev.Preferences.ShowBoosts,
		})
	})
	r.Register(appgraph.EventFollowTerminated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appgraph.FollowTerminated)
		if !ev.WasActive() {
			return nil
		}
		// The followee leaves every list of the follower with the follow, whatever ended it, and
		// a new follow does not bring it back (ADR-0013 §11).
		if err := repo.DeleteListMembersOfFollow(ctx, tx, ev.Follow.FollowerAccountID, ev.Follow.FolloweeAccountID); err != nil {
			return err
		}
		return repo.DeleteFollowEdge(ctx, tx, ev.Follow.FollowerAccountID, ev.Follow.FolloweeAccountID)
	})
	r.Register(appgraph.EventFollowsTerminated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		var followers, followees []string
		for _, t := range e.(appgraph.FollowsTerminated).Terminations {
			if t.WasActive() {
				followers, followees = append(followers, t.Follow.FollowerAccountID), append(followees, t.Follow.FolloweeAccountID)
			}
		}
		if err := repo.DeleteListMembersOfFollows(ctx, tx, followers, followees); err != nil {
			return err
		}
		return repo.DeleteFollowEdges(ctx, tx, followers, followees)
	})
	r.Register(appgraph.EventFollowPreferencesChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appgraph.FollowPreferencesChanged)
		if !ev.Active() {
			return nil
		}
		return repo.UpsertFollowEdge(ctx, tx, FollowEdge{
			FollowerAccountID: ev.Follow.FollowerAccountID, FolloweeAccountID: ev.Follow.FolloweeAccountID, ShowBoosts: ev.Preferences.ShowBoosts,
		})
	})
	r.Register(appgraph.EventBlocked, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		b := e.(appgraph.Blocked).Block
		for _, x := range blockExclusions(b.BlockerAccountID, b.BlockedAccountID) {
			if err := repo.UpsertExclusion(ctx, tx, x); err != nil {
				return err
			}
		}
		return nil
	})
	r.Register(appgraph.EventUnblocked, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		b := e.(appgraph.Unblocked).Block
		for _, x := range blockExclusions(b.BlockerAccountID, b.BlockedAccountID) {
			if err := repo.DeleteExclusion(ctx, tx, x.ViewerAccountID, x.TargetAccountID, x.Reason); err != nil {
				return err
			}
		}
		return nil
	})
	r.Register(appgraph.EventMuted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		m := e.(appgraph.Muted).Mute
		return repo.UpsertExclusion(ctx, tx, Exclusion{
			ViewerAccountID: m.AccountID, TargetAccountID: m.TargetAccountID, Reason: timeline.ExcludedMute, ExpiresAt: m.ExpiresAt,
		})
	})
	r.Register(appgraph.EventUnmuted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appgraph.Unmuted)
		return repo.DeleteExclusion(ctx, tx, ev.AccountID, ev.TargetAccountID, timeline.ExcludedMute)
	})
	r.Register(appgraph.EventDomainMuted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		m := e.(appgraph.DomainMuted).Mute
		return repo.InsertDomainMute(ctx, tx, DomainMute{ViewerAccountID: m.AccountID, Domain: m.Domain})
	})
	r.Register(appgraph.EventDomainUnmuted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appgraph.DomainUnmuted)
		return repo.DeleteDomainMute(ctx, tx, ev.AccountID, ev.Domain)
	})

	registerLifecycleProjector(r, repo, moderated)
}

// registerLifecycleProjector registers the projector of the lifecycle of accounts:
// feed_author_restrictions holds profile_view for each deleted or deactivated account, and a
// reactivation puts back what moderation has in effect (ADR-0089 §15), so a suspension goes on.
func registerLifecycleProjector(r *uow.Projectors, repo Repository, moderated ModeratedEffects) {
	r.Register(appaccount.EventLifecycleEventRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appaccount.LifecycleEventRecorded)
		id := ev.Event.AccountID
		switch {
		case ev.Deletes(), ev.Deactivates():
			// A deletion's restriction has no end of its own (data-model §4.6, §8), and a
			// deactivation lasts until the reactivation.
			return repo.UpsertAuthorRestriction(ctx, tx, AuthorRestriction{AccountID: id, Restriction: timeline.RestrictProfileView})
		case ev.Reactivates(), ev.RestoresRemoteActor():
			var effects []appmoderation.Effect
			if moderated != nil {
				var err error
				if effects, err = moderated.AccountEffectsTx(ctx, tx, id); err != nil {
					return err
				}
			}
			return replaceRestrictions(effects, func(r timeline.Restriction, until *time.Time) error {
				return repo.UpsertAuthorRestriction(ctx, tx, AuthorRestriction{AccountID: id, Restriction: r, EffectiveUntil: until})
			}, func(r timeline.Restriction) error {
				return repo.DeleteAuthorRestriction(ctx, tx, id, r)
			})
		}
		return nil
	})
}

func insertEntry(ctx context.Context, tx uow.Tx, repo Repository, e Entry) error {
	inserted, err := repo.InsertEntry(ctx, tx, e)
	if err != nil {
		return err
	}
	if !inserted {
		return fmt.Errorf("feed entry %s: %w", e.ID, errEntryExists)
	}
	return nil
}

// projectRevision makes the mentions and tags of a post's current revision, from revisedAt, its
// direct recipients (for a direct post) and its tags.
func projectRevision(ctx context.Context, tx uow.Tx, repo Repository, post Entry, mentions, tags []string, revisedAt time.Time) error {
	if post.Visibility == appcontent.VisibilityDirect {
		if err := repo.ReplaceDirectRecipients(ctx, tx, post.ID, mentions); err != nil {
			return err
		}
	}
	return repo.ReplaceTags(ctx, tx, post, tags, revisedAt)
}

// blockExclusions are the exclusions of a block: the blocker excludes the blocked account, and
// the blocked account excludes the blocker.
func blockExclusions(blocker, blocked string) []Exclusion {
	return []Exclusion{
		{ViewerAccountID: blocker, TargetAccountID: blocked, Reason: timeline.ExcludedBlock},
		{ViewerAccountID: blocked, TargetAccountID: blocker, Reason: timeline.ExcludedBlockedBy},
	}
}

// domainOf returns the domain of the account behind a status or boost: "" for a local one, the
// lowercased host of its URI for a remote one.
func domainOf(isLocal bool, uri string) (string, error) {
	if isLocal {
		return "", nil
	}
	u, err := url.Parse(uri)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("timeline: remote object %q has no host", uri)
	}
	return strings.ToLower(u.Hostname()), nil
}

func language(lang string) string {
	if lang == "" {
		return timeline.UndeterminedLanguage
	}
	return lang
}

// Source replays events of another module inside tx through emit, for RebuildProjections.
type Source func(ctx context.Context, tx uow.Tx, emit func(uow.Event) error) error

// ContentSource replays the statuses and boosts of the content module (appcontent.ReplayEvents).
func ContentSource(statuses appcontent.Repository, interactions appcontent.InteractionRepository) Source {
	return func(ctx context.Context, tx uow.Tx, emit func(uow.Event) error) error {
		return appcontent.ReplayEvents(ctx, tx, statuses, interactions, emit)
	}
}

// AccountSource replays the deletions and the deactivations in force of accounts of the account
// module (appaccount.ReplayLifecycle).
func AccountSource(repo appaccount.Repository) Source {
	return func(ctx context.Context, tx uow.Tx, emit func(uow.Event) error) error {
		return appaccount.ReplayLifecycle(ctx, tx, repo, emit)
	}
}

// ModerationSource replays the restrictions in effect at the time of clock of the moderation
// module (appmoderation.ReplayEvents).
func ModerationSource(repo appmoderation.Repository, clock kernel.Clock) Source {
	return func(ctx context.Context, tx uow.Tx, emit func(uow.Event) error) error {
		return appmoderation.ReplayEvents(ctx, tx, repo, kernel.Normalize(clock.Now()), emit)
	}
}

// GraphSource replays the follows, blocks, and mutes of the graph module (appgraph.ReplayEvents).
func GraphSource(repo appgraph.Repository) Source {
	return func(ctx context.Context, tx uow.Tx, emit func(uow.Event) error) error {
		return appgraph.ReplayEvents(ctx, tx, repo, emit)
	}
}

// RebuildProjections empties the projections of Projection and derives them again, in one
// transaction, by applying this module's projectors to the events that sources replay
// (data-model §9). The restrictions are left alone: RebuildRestrictions derives them. The lists
// are not projections and stay, but a member whose follow the rebuilt feed_follow_edges no longer
// holds leaves them, as the end of the follow would have taken it out.
func RebuildProjections(ctx context.Context, u uow.UnitOfWork, repo Repository, sources ...Source) error {
	projectors := &uow.Projectors{}
	RegisterProjectors(projectors, repo, nil)
	err := u.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.DeleteProjection(ctx, tx); err != nil {
			return err
		}
		for _, src := range sources {
			if err := src(ctx, tx, func(e uow.Event) error { return projectors.Project(ctx, tx, e) }); err != nil {
				return err
			}
		}
		return repo.DeleteListMembersWithoutFollow(ctx, tx)
	})
	if err != nil {
		return fmt.Errorf("rebuild the feed projections: %w", err)
	}
	return nil
}
