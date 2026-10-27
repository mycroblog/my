// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// RegisterProjectors registers the synchronous projectors of the content module's statuses
// (data-model §9). Every status projection covers only statuses that are not deleted:
//   - status_heads points at the revision with the highest Seq;
//   - status_mention_index holds the mentions of that revision;
//   - status_reply_links links a reply to its parent once both are known;
//   - status_counters counts the replies whose visibility CountsAsReply (the boosts and
//     favourites are counted by RegisterInteractionProjectors);
//   - account_post_counters counts each account's statuses and their latest PublishedAt;
//   - local_post_counters counts each local account's statuses that CountAsLocalPost (its
//     boosts are counted by RegisterInteractionProjectors).
//
// The rebuild queries of the Repository state the same rules in SQL; the property test in
// testkit.StatusRepositoryContract checks that both agree.
func RegisterProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventStatusCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(StatusCreated)
		if err := repo.LockStatusKeys(ctx, tx, statusLockKeys(ev.Status, false)); err != nil {
			return err
		}
		return projectCreated(ctx, tx, repo, ev.Status, ev.Revision)
	})
	r.Register(EventStatusRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		rev := e.(StatusRevised).Revision
		if err := repo.LockStatusKeys(ctx, tx, statusLockKeys(content.Status{ID: rev.StatusID}, true)); err != nil {
			return err
		}
		deleted, err := repo.IsDeleted(ctx, tx, rev.StatusID)
		if err != nil || deleted {
			return err
		}
		return projectRevision(ctx, tx, repo, rev)
	})
	r.Register(EventStatusDeleted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(StatusDeleted)
		if err := repo.LockStatusKeys(ctx, tx, statusLockKeys(ev.Status, true)); err != nil {
			return err
		}
		return projectDeleted(ctx, tx, repo, ev.Status)
	})
	// poll_tallies holds the counts of the newest observation of each remote poll.
	r.Register(EventPollTallyObserved, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		o := e.(PollTallyObserved).Observation
		return repo.ReplacePollTallies(ctx, tx, o.PollID, o.Tallies())
	})
}

// statusLockKeys returns the keys of Repository.LockStatusKeys, sorted, that a projector of s
// takes before it reads the projections:
//   - the URI of s and the URI it replies to, so that a status and its parent or replies created
//     or deleted at the same time each see the other once it commits (the second one waits for the
//     first one's commit, and READ COMMITTED reads what it committed);
//   - with byID, the ID of s, so that a revision and the deletion of a status do not both read
//     the status as live: a revision projected after the deletion sees it.
//
// Every transaction takes its keys in this order, so that two of them do not wait for each other.
func statusLockKeys(s content.Status, byID bool) []string {
	var keys []string
	if byID {
		keys = append(keys, "id:"+s.ID)
	}
	for _, uri := range []string{s.URI, s.InReplyToURI} {
		if uri != "" {
			keys = append(keys, "uri:"+uri)
		}
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

func projectRevision(ctx context.Context, tx uow.Tx, repo Repository, rev content.Revision) error {
	head, err := repo.Head(ctx, tx, rev.StatusID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if !rev.Supersedes(head) {
		return nil
	}
	if err := repo.UpsertHead(ctx, tx, content.HeadOf(rev)); err != nil {
		return err
	}
	return repo.ReplaceMentions(ctx, tx, rev.StatusID, rev.MentionedAccountIDs)
}

func projectCreated(ctx context.Context, tx uow.Tx, repo Repository, s content.Status, rev content.Revision) error {
	if err := projectRevision(ctx, tx, repo, rev); err != nil {
		return err
	}
	// Replies that arrived before this status now have a known parent.
	replies, err := repo.ListLiveRepliesTo(ctx, tx, s.URI)
	if err != nil {
		return err
	}
	counters := content.Counters{StatusID: s.ID}
	for _, reply := range replies {
		if reply.ID == s.ID {
			continue // a status that replies to itself is linked below, as a reply
		}
		if err := repo.UpsertReplyLink(ctx, tx, linkTo(reply, s)); err != nil {
			return err
		}
		if reply.Visibility.CountsAsReply() {
			counters.RepliesCount++
		}
	}
	if err := repo.InsertCounters(ctx, tx, counters); err != nil {
		return err
	}
	if s.InReplyToURI != "" {
		parent, err := repo.FindLiveStatusByURI(ctx, tx, s.InReplyToURI)
		switch {
		case errors.Is(err, ErrNotFound):
			// The parent is unknown or deleted; a parent that arrives later links this reply.
		case err != nil:
			return err
		default:
			if err := repo.UpsertReplyLink(ctx, tx, linkTo(s, parent)); err != nil {
				return err
			}
			if s.Visibility.CountsAsReply() {
				if err := repo.AddReplies(ctx, tx, parent.ID, 1); err != nil {
					return err
				}
			}
		}
	}
	if err := repo.AddPost(ctx, tx, s.AuthorAccountID, s.PublishedAt); err != nil {
		return err
	}
	if s.CountsAsLocalPost() {
		return repo.AddLocalPosts(ctx, tx, s.AuthorAccountID, 1)
	}
	return nil
}

func projectDeleted(ctx context.Context, tx uow.Tx, repo Repository, s content.Status) error {
	link, err := repo.ReplyLink(ctx, tx, s.ID)
	switch {
	case errors.Is(err, ErrNotFound):
	case err != nil:
		return err
	case link.ParentStatusID != s.ID && s.Visibility.CountsAsReply():
		if err := repo.AddReplies(ctx, tx, link.ParentStatusID, -1); err != nil {
			return err
		}
	}
	if err := repo.DeleteStatusProjections(ctx, tx, s.ID); err != nil {
		return err
	}
	if err := repo.RemovePost(ctx, tx, s.AuthorAccountID); err != nil {
		return err
	}
	if s.CountsAsLocalPost() {
		return repo.AddLocalPosts(ctx, tx, s.AuthorAccountID, -1)
	}
	return nil
}

func linkTo(reply, parent content.Status) content.ReplyLink {
	return content.ReplyLink{StatusID: reply.ID, ParentStatusID: parent.ID, ParentAuthorAccountID: parent.AuthorAccountID}
}

// RebuildProjections empties the projections of the content module's statuses and derives
// them again from their sources (data-model §9), in one transaction per projection.
func RebuildProjections(ctx context.Context, u uow.UnitOfWork, repo Repository) error {
	steps := []struct {
		name string
		fn   func(context.Context, uow.Tx) error
	}{
		{"status_heads", repo.RebuildHeads},
		{"status_mention_index", repo.RebuildMentions},
		{"status_reply_links", repo.RebuildReplyLinks},
		{"status_counters", repo.RebuildCounters},
		{"account_post_counters", repo.RebuildPostCounters},
		{"local_post_counters", repo.RebuildLocalPostCounters},
		{"poll_tallies", repo.RebuildPollTallies},
	}
	for _, s := range steps {
		if err := u.Tx(ctx, func(tx uow.Tx) error { return s.fn(ctx, tx) }); err != nil {
			return fmt.Errorf("rebuild %s: %w", s.name, err)
		}
	}
	return nil
}

// SensitivityMarks calls mark inside tx for each status that a moderation action in force shows
// as sensitive, with the action: what the moderation module recorded through
// MarkSensitiveByModeration and has not ended through ClearSensitivityOf. The content module
// does not read the moderation tables, so the moderation module's public API replays them
// (appmoderation.ReplaySensitivityMarks).
type SensitivityMarks func(ctx context.Context, tx uow.Tx, mark func(statusID, actionID string) error) error

// RebuildSensitivityOverrides empties status_sensitivity_overrides and records again, in one
// transaction, the statuses that marks names (data-model §9).
func RebuildSensitivityOverrides(ctx context.Context, u uow.UnitOfWork, repo Repository, marks SensitivityMarks) error {
	err := u.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.DeleteAllSensitivityOverrides(ctx, tx); err != nil {
			return err
		}
		return marks(ctx, tx, func(statusID, actionID string) error {
			return repo.InsertSensitivityOverride(ctx, tx, statusID, actionID)
		})
	})
	if err != nil {
		return fmt.Errorf("rebuild status_sensitivity_overrides: %w", err)
	}
	return nil
}
