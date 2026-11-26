// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// RegisterProjectors registers the synchronous projectors of the content module's statuses.
// Every status projection covers only statuses that are not deleted:
//   - status_heads points at the revision with the highest Seq;
//   - status_mention_index holds the mentions of that revision;
//   - status_reply_links links a reply to its parent once both are known, unless the reply is
//     held (a reply request of its is not accepted, ADR-0155 §6) until StatusReleased;
//   - status_counters counts the replies, not held, whose visibility CountsAsReply (the boosts and
//     favourites are counted by RegisterInteractionProjectors);
//   - account_post_counters counts each account's statuses and their latest PublishedAt, but the
//     posts of the boards of events, which no remote server learns of (ADR-0161 §17);
//   - local_post_counters counts each local account's statuses that CountAsLocalPost, but the
//     board posts (its boosts are counted by RegisterInteractionProjectors).
//
// The posts of the boards of events project alike from BoardStatusPosted, BoardStatusRevised, and
// BoardStatusesDeleted. The rebuild queries of the Repository state the same rules in SQL; the
// property test in contenttest.StatusRepositoryContract checks that both agree.
func RegisterProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventStatusCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(StatusCreated)
		if err := repo.LockStatusKeys(ctx, tx, statusLockKeys(ev.Status, false)); err != nil {
			return err
		}
		return projectCreated(ctx, tx, repo, ev.Status, ev.Revision, ev.HeldReply, true)
	})
	r.Register(EventStatusReleased, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(StatusReleased)
		if err := repo.LockStatusKeys(ctx, tx, statusLockKeys(ev.Status, false)); err != nil {
			return err
		}
		return linkReply(ctx, tx, repo, ev.Status)
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
	r.Register(EventStatusesDeleted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ds := e.(StatusesDeleted).Deletions
		var keys []string
		for _, d := range ds {
			for _, k := range statusLockKeys(d.Status, true) {
				if !slices.Contains(keys, k) {
					keys = append(keys, k)
				}
			}
		}
		slices.Sort(keys)
		if err := repo.LockStatusKeys(ctx, tx, keys); err != nil {
			return err
		}
		return projectDeletedMany(ctx, tx, repo, ds)
	})
	registerBoardProjectors(r, repo)
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

// projectCreated projects the new status s with its first revision rev. A held reply is not
// linked to its parent nor counted among its replies; a status that is not counted (a post of the
// board of an event, which remote servers learn nothing of) counts among no posts of its author.
func projectCreated(ctx context.Context, tx uow.Tx, repo Repository, s content.Status, rev content.Revision, held, counted bool) error {
	if err := projectRevision(ctx, tx, repo, rev); err != nil {
		return err
	}
	parent, replies, err := createdReplyLinks(ctx, tx, repo, s, held)
	if err != nil {
		return err
	}
	counters := content.Counters{StatusID: s.ID}
	for _, reply := range replies {
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
	if parent != nil {
		if err := addReplyLink(ctx, tx, repo, s, *parent); err != nil {
			return err
		}
	}
	if !counted {
		return nil
	}
	if err := repo.AddPost(ctx, tx, s.AuthorAccountID, s.PublishedAt); err != nil {
		return err
	}
	if s.CountsAsLocalPost() {
		return repo.AddLocalPosts(ctx, tx, s.AuthorAccountID, 1)
	}
	return nil
}

// createdReplyLinks returns the reply links that projecting the new status s makes
// (projectCreated): the stored replies that waited for s as their parent, neither deleted nor
// held and s itself left out, each of which links to s, and, unless s is held, the link of s to
// its parent (replyLinkOf). The caller holds the lock keys statusLockKeys(s, false), which keep
// the links as they are until it commits, so that the command that stores s and the projector of
// its StatusCreated find the same links: the command puts them in the event (createdEventLinks)
// and the projector writes them.
func createdReplyLinks(ctx context.Context, tx uow.Tx, repo Repository, s content.Status, held bool) (*content.ReplyLink, []content.Status, error) {
	all, err := repo.ListLiveRepliesTo(ctx, tx, s.URI)
	if err != nil {
		return nil, nil, err
	}
	// A status that replies to itself is linked as a reply, below.
	replies := slices.DeleteFunc(all, func(r content.Status) bool { return r.ID == s.ID })
	if held {
		return nil, replies, nil
	}
	parent, err := replyLinkOf(ctx, tx, repo, s)
	return parent, replies, err
}

// LinkCreated returns ev with ReplyLink and LinkedReplies filled in, as the projector of ev links
// them, for the status of ev that tx has just stored, before tx emits ev. It takes the lock keys of
// the status first. The commands of this module, which hold them already, fill the links the same
// way (createdEventLinks); a test kit that stores a status and emits StatusCreated itself calls it.
func LinkCreated(ctx context.Context, tx uow.Tx, repo Repository, ev StatusCreated) (StatusCreated, error) {
	if err := repo.LockStatusKeys(ctx, tx, statusLockKeys(ev.Status, false)); err != nil {
		return ev, err
	}
	var err error
	ev.ReplyLink, ev.LinkedReplies, err = createdEventLinks(ctx, tx, repo, ev.Status, ev.HeldReply)
	return ev, err
}

// createdEventLinks returns StatusCreated.ReplyLink and StatusCreated.LinkedReplies of the new
// status s, which createdReplyLinks decides.
func createdEventLinks(ctx context.Context, tx uow.Tx, repo Repository, s content.Status, held bool) (*content.ReplyLink, []content.ReplyLink, error) {
	parent, replies, err := createdReplyLinks(ctx, tx, repo, s, held)
	if err != nil {
		return nil, nil, err
	}
	var linked []content.ReplyLink
	for _, r := range replies {
		linked = append(linked, linkTo(r, s))
	}
	return parent, linked, nil
}

// replyLinkOf returns the link that linkReply makes for the reply s: to its parent when the
// parent is stored and live and s is not linked yet; nil otherwise, so that a parent that arrives
// later links it.
func replyLinkOf(ctx context.Context, tx uow.Tx, repo Repository, s content.Status) (*content.ReplyLink, error) {
	if s.InReplyToURI == "" {
		return nil, nil
	}
	parent, err := repo.FindLiveStatusByURI(ctx, tx, s.InReplyToURI)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, err
	}
	if _, err := repo.ReplyLink(ctx, tx, s.ID); err == nil {
		return nil, nil // linked already
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	l := linkTo(s, parent)
	return &l, nil
}

// linkReply links the reply s to its parent and counts it among the parent's replies, when
// replyLinkOf finds a link to make.
func linkReply(ctx context.Context, tx uow.Tx, repo Repository, s content.Status) error {
	l, err := replyLinkOf(ctx, tx, repo, s)
	if err != nil || l == nil {
		return err
	}
	return addReplyLink(ctx, tx, repo, s, *l)
}

// addReplyLink stores the link l of the reply s and counts s among the replies of its parent.
func addReplyLink(ctx context.Context, tx uow.Tx, repo Repository, s content.Status, l content.ReplyLink) error {
	if err := repo.UpsertReplyLink(ctx, tx, l); err != nil {
		return err
	}
	if s.Visibility.CountsAsReply() {
		return repo.AddReplies(ctx, tx, l.ParentStatusID, 1)
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
	// A board post counts among no posts (projectCreated).
	if gid, err := boardOf(ctx, repo, tx, s); err != nil || gid != "" {
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

// projectDeletedMany is projectDeleted of the statuses of ds, together.
func projectDeletedMany(ctx context.Context, tx uow.Tx, repo Repository, ds []StatusDeleted) error {
	ids := make([]string, len(ds))
	for i, d := range ds {
		ids[i] = d.Status.ID
	}
	links, err := repo.ReplyLinksOf(ctx, tx, ids)
	if err != nil {
		return err
	}
	// The board posts count among no posts (projectCreated).
	boards, err := repo.StatusBoardsAmong(ctx, tx, ids)
	if err != nil {
		return err
	}
	replies := map[string]int64{}
	posts := map[string]int64{}
	localPosts := map[string]int64{}
	for _, d := range ds {
		s := d.Status
		if link, ok := links[s.ID]; ok && link.ParentStatusID != s.ID && s.Visibility.CountsAsReply() && !slices.Contains(ids, link.ParentStatusID) {
			replies[link.ParentStatusID]--
		}
		if _, onBoard := boards[s.ID]; onBoard {
			continue
		}
		posts[s.AuthorAccountID]++
		if s.CountsAsLocalPost() {
			localPosts[s.AuthorAccountID]++
		}
	}
	if err := repo.AddManyReplies(ctx, tx, replies); err != nil {
		return err
	}
	if err := repo.DeleteStatusProjectionsOf(ctx, tx, ids); err != nil {
		return err
	}
	for _, author := range slices.Sorted(maps.Keys(posts)) {
		if err := repo.RemovePosts(ctx, tx, author, posts[author]); err != nil {
			return err
		}
	}
	return repo.SubtractLocalPostsOf(ctx, tx, localPosts)
}

func linkTo(reply, parent content.Status) content.ReplyLink {
	return content.ReplyLink{StatusID: reply.ID, ParentStatusID: parent.ID, ParentAuthorAccountID: parent.AuthorAccountID}
}

// RebuildProjections empties the projections of the content module's statuses and derives
// them again from their sources, in one transaction per projection.
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

// SensitivitySource is what shows a status as sensitive: a mark_statuses_sensitive action or a
// mark_sensitive decision of a hook, exactly one of the two.
type SensitivitySource struct {
	ActionID       string
	HookDecisionID string
}

// SensitivityMarks calls mark inside tx for each status that a source in force shows as sensitive:
// what the moderation module recorded through MarkSensitiveByModeration and has not ended through
// ClearSensitivityOf, and what the owners of the hooks recorded through MarkSensitiveByHook and no
// moderator cancelled. The content module does not read the tables of those modules, so their public
// APIs replay them (appmoderation.ReplaySensitivityMarks, apphook.ReplaySensitivityMarks).
type SensitivityMarks func(ctx context.Context, tx uow.Tx, mark func(statusID string, source SensitivitySource) error) error

// RebuildSensitivityOverrides empties status_sensitivity_overrides and records again, in one
// transaction, the statuses that marks names. A status with no row is dropped: its
// purge removed its rows here, and the records of its sources can outlast it (ADR-0147 §8).
func RebuildSensitivityOverrides(ctx context.Context, u uow.UnitOfWork, repo Repository, marks SensitivityMarks) error {
	err := u.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.DeleteAllSensitivityOverrides(ctx, tx); err != nil {
			return err
		}
		err := marks(ctx, tx, func(statusID string, source SensitivitySource) error {
			if source.HookDecisionID != "" {
				return repo.InsertSensitivityOverrideOfHookDecision(ctx, tx, statusID, source.HookDecisionID)
			}
			return repo.InsertSensitivityOverride(ctx, tx, statusID, source.ActionID)
		})
		if err != nil {
			return err
		}
		return repo.DeleteSensitivityOverridesWithoutStatus(ctx, tx)
	})
	if err != nil {
		return fmt.Errorf("rebuild status_sensitivity_overrides: %w", err)
	}
	return nil
}
