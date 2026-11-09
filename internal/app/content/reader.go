// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// Bounds of a thread (FR-106).
const (
	// MaxThreadAncestors is the number of statuses Thread walks up from a status.
	MaxThreadAncestors = 40
	// MaxThreadDescendants is the number of replies Thread returns below a status.
	MaxThreadDescendants = 60
)

// ReaderDeps are the ports a Reader uses.
type ReaderDeps struct {
	Statuses      Repository
	Interactions  InteractionRepository
	Accounts      Accounts
	Relationships Relationships
	// Restrictions are the authors' moderation restrictions; nil means NoRestrictions.
	Restrictions RestrictionPolicy
	// Media reads the attached media; nil presents no attachments.
	Media *MediaService
	// LinkCards presents the cards of the links in statuses (ADR-0062); nil presents none.
	LinkCards *LinkCards
	// Quotes reads the quotes of statuses (ADR-0069); nil presents none.
	Quotes QuoteRepository
	// DomainMutes tells which quoted statuses come from a domain that the viewer mutes; nil
	// tells none.
	DomainMutes DomainMutes
	// Clock tells whether a poll has ended; nil means the system's clock.
	Clock kernel.Clock
}

// Reader presents statuses to a viewer: one status, its thread, its edit history, and a local
// account's bookmarks, with everything a client shows of them. Every status goes through
// content.CanView in ContextStatus; a status that the viewer may not see fails with
// ErrNotFound, the same as a missing or deleted one, and is left out of lists, so that its
// existence does not leak (docs/architecture/visibility.md).
//
// The viewer is anonymous or a local account (content.ViewerAnonymous, content.ViewerLocal).
type Reader struct {
	d   ReaderDeps
	aud audience
	// reactionEmoji are the custom emoji of the reactions that a page read beforehand
	// (Reader.page); nil outside a page.
	reactionEmoji map[string]*ReactionCustomEmoji
}

// NewReader returns a Reader on d.
func NewReader(d ReaderDeps) *Reader {
	if d.Restrictions == nil {
		d.Restrictions = NoRestrictions{}
	}
	if d.Clock == nil {
		d.Clock = wallClock{}
	}
	return &Reader{d: d, aud: audience{
		statuses: d.Statuses, interactions: d.Interactions, accounts: d.Accounts,
		relationships: d.Relationships, restrictions: d.Restrictions,
	}}
}

// StatusView is a status as a viewer sees it.
type StatusView struct {
	Status   content.Status
	Revision content.Revision
	// Author is the author's account.
	Author appaccount.Actor
	// InReplyTo is the status replied to, nil when there is none or the viewer may not see it.
	InReplyTo *content.Status
	// Attachments are the attached media that finished processing, in order.
	Attachments []MediaView
	// Mentions are the mentioned accounts that exist, in the revision's order.
	Mentions []appaccount.Actor
	Tags     []content.Tag
	Counters content.Counters
	// Reactions are the counts of the emoji reactions to the status, the most used
	// content.MaxStatusReactions emoji first (ADR-0051 §10).
	Reactions []ReactionView
	// Poll is the poll of the revision, nil for none (FR-112).
	Poll *PollView
	// Relation is the viewer's relation to the status, nil for an anonymous viewer.
	Relation *ViewerRelation
	// Card is the card of the link in the status, nil for none (ADR-0062).
	Card *LinkCardView
	// Quote is the quote of the status, nil for none or for a status presented as a quote
	// (ADR-0069).
	Quote *QuoteView
	// Quotable is whether the viewer, a local account, may quote the status
	// (content.MayQuote); false for an anonymous viewer.
	Quotable bool
	// QuotePolicy is who may quote the status, for its author only; "" for anyone else.
	QuotePolicy content.QuotePolicy
}

// Edited reports whether the status has a revision after its first.
func (v StatusView) Edited() bool { return v.Revision.Seq > 1 }

// ViewerRelation is what a local viewer did with a status.
type ViewerRelation struct {
	Favourited, Boosted, Bookmarked bool
	// Author is whether the viewer is the author, who alone can pin the status.
	Author bool
	// Pinned is whether the author pinned the status; always false for other viewers.
	Pinned bool
}

// MediaView is an attached media file that can be shown: an upload that finished processing, or
// a remote file (Media.Remote) served through the media proxy.
type MediaView struct {
	Media Media
	// Description is the alt text of the attachment, "" for none.
	Description string
	// Image is where clients load the file: its variants, or for a remote file its media
	// proxy URLs.
	Image Image
}

// RevisionParts are what a revision refers to, resolved: the attached media that finished
// processing, in order; the mentioned accounts that exist, in the revision's order; the
// hashtags, in the revision's order; the custom emoji that can be shown, by shortcode; and the
// poll with its counts as an anonymous viewer sees them, nil when the revision has none.
type RevisionParts struct {
	Attachments []MediaView
	Mentions    []appaccount.Actor
	Tags        []content.Tag
	Emojis      []EmojiTag
	Poll        *PollView
}

// RevisionView is one revision of a status, for its edit history.
type RevisionView struct {
	Revision    content.Revision
	Attachments []MediaView
}

// Thread is the statuses around one that a viewer may see.
type Thread struct {
	// Ancestors are the statuses replied to, oldest first, up to the first that is missing or
	// that the viewer may not see.
	Ancestors []StatusView
	// Descendants are the replies and their replies, depth first in posting order, without
	// the replies that the viewer may not see and their replies.
	Descendants []StatusView
}

// BookmarkPage is a page of bookmarked statuses.
type BookmarkPage struct {
	Statuses []StatusView
	// Next is the cursor of the next page, "" on the last page.
	Next string
}

// View returns a status as viewer sees it.
func (r *Reader) View(ctx context.Context, viewer Viewer, statusID string) (StatusView, error) {
	st, author, err := r.aud.visibleStatus(ctx, viewer, statusID)
	if err != nil {
		return StatusView{}, err
	}
	return r.present(ctx, viewer, st, author)
}

// Visible returns a status that viewer may see, without what a client shows of it; a status
// that the viewer may not see fails with ErrNotFound, the same as a missing or deleted one.
// Other modules check through it that a viewer may refer to a status, such as in a report.
func (r *Reader) Visible(ctx context.Context, viewer Viewer, statusID string) (content.Status, error) {
	st, _, err := r.aud.visibleStatus(ctx, viewer, statusID)
	return st, err
}

// ReadableStatus is a status that a viewer may see, for a module that works on its text.
type ReadableStatus struct {
	Status content.Status
	// Revision is its current revision.
	Revision content.Revision
	// EffectiveVisibility is the visibility that the server judges it by:
	// content.EffectiveVisibility of its visibility under its author's restrictions.
	EffectiveVisibility Visibility
}

// Readable returns what a module needs to work on the text of a status that viewer may see; a
// status that the viewer may not see fails with ErrNotFound, the same as a missing or deleted one.
func (r *Reader) Readable(ctx context.Context, viewer Viewer, statusID string) (ReadableStatus, error) {
	st, _, err := r.aud.visibleStatus(ctx, viewer, statusID)
	if err != nil {
		return ReadableStatus{}, err
	}
	rev, err := r.d.Statuses.FindCurrentRevision(ctx, st.ID)
	if err != nil {
		return ReadableStatus{}, err // a deleted status has no current revision
	}
	restrictions, err := r.d.Restrictions.Restrictions(ctx, st.AuthorAccountID)
	if err != nil {
		return ReadableStatus{}, err
	}
	return ReadableStatus{Status: st, Revision: rev, EffectiveVisibility: content.EffectiveVisibility(st.Visibility, restrictions.Author)}, nil
}

// HoldLive fails with ErrNotFound when the status is deleted, and otherwise keeps its deletion
// from passing until tx ends: the deletion's projector takes the same lock before it removes the
// head, so that what another module stores under tx is there for the deletion's projector to
// remove, or fails here.
func (r *Reader) HoldLive(ctx context.Context, tx uow.Tx, statusID string) error {
	if err := r.d.Statuses.LockStatusKeys(ctx, tx, statusLockKeys(content.Status{ID: statusID}, true)); err != nil {
		return err
	}
	_, err := r.d.Statuses.Head(ctx, tx, statusID)
	return err
}

// Live fails with ErrNotFound when statusID is missing or deleted, whoever asks. It tells only
// that the status exists, so the caller shows nothing of it on the strength of this check.
func (r *Reader) Live(ctx context.Context, statusID string) error {
	_, err := r.aud.live(ctx, statusID)
	return err
}

// Thread returns the ancestors and descendants of a status that viewer may see (FR-106).
func (r *Reader) Thread(ctx context.Context, viewer Viewer, statusID string) (Thread, error) {
	st, _, err := r.aud.visibleStatus(ctx, viewer, statusID)
	if err != nil {
		return Thread{}, err
	}
	var out Thread
	seen := map[string]bool{st.ID: true}
	for cur := st; len(out.Ancestors) < MaxThreadAncestors && cur.InReplyToURI != ""; {
		parent, view, ok, err := r.viewByURI(ctx, viewer, cur.InReplyToURI)
		if err != nil {
			return Thread{}, err
		}
		if !ok || seen[parent.ID] {
			break
		}
		seen[parent.ID] = true
		out.Ancestors = append([]StatusView{view}, out.Ancestors...)
		cur = parent
	}
	if err := r.descend(ctx, viewer, st, seen, &out.Descendants); err != nil {
		return Thread{}, err
	}
	return out, nil
}

// descend appends the visible replies to st, each followed by its own replies.
func (r *Reader) descend(ctx context.Context, viewer Viewer, st content.Status, seen map[string]bool, out *[]StatusView) error {
	replies, err := r.d.Statuses.ListReplies(ctx, st.URI)
	if err != nil {
		return err
	}
	for _, reply := range replies {
		if len(*out) >= MaxThreadDescendants {
			return nil
		}
		if seen[reply.ID] {
			continue
		}
		seen[reply.ID] = true
		ok, author, err := r.aud.visible(ctx, viewer, reply)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		view, err := r.present(ctx, viewer, reply, author)
		if errors.Is(err, ErrNotFound) {
			continue // deleted meanwhile
		}
		if err != nil {
			return err
		}
		*out = append(*out, view)
		if err := r.descend(ctx, viewer, reply, seen, out); err != nil {
			return err
		}
	}
	return nil
}

// viewByURI returns the live status with uri as viewer sees it; ok is false when it is
// missing, deleted, or hidden from the viewer.
func (r *Reader) viewByURI(ctx context.Context, viewer Viewer, uri string) (content.Status, StatusView, bool, error) {
	st, err := r.d.Statuses.FindStatusByURI(ctx, uri)
	if errors.Is(err, ErrNotFound) {
		return content.Status{}, StatusView{}, false, nil
	}
	if err != nil {
		return content.Status{}, StatusView{}, false, err
	}
	view, err := r.View(ctx, viewer, st.ID)
	if errors.Is(err, ErrNotFound) {
		return content.Status{}, StatusView{}, false, nil
	}
	if err != nil {
		return content.Status{}, StatusView{}, false, err
	}
	return st, view, true, nil
}

// History returns the revisions of a status that viewer may see, oldest first (FR-109). What
// moderation forces sensitive shows so in every revision with media, as in the current one.
func (r *Reader) History(ctx context.Context, viewer Viewer, statusID string) ([]RevisionView, error) {
	st, _, err := r.aud.visibleStatus(ctx, viewer, statusID)
	if err != nil {
		return nil, err
	}
	forced, err := r.forcedSensitive(ctx, st)
	if err != nil {
		return nil, err
	}
	revs, err := r.d.Statuses.ListRevisions(ctx, statusID)
	if err != nil {
		return nil, err
	}
	out := make([]RevisionView, 0, len(revs))
	for _, head := range revs {
		rev, err := r.d.Statuses.FindRevision(ctx, head.ID)
		if err != nil {
			return nil, err
		}
		media, err := r.attachments(ctx, rev.Attachments)
		if err != nil {
			return nil, err
		}
		if forced && len(media) > 0 {
			rev.Sensitive = true
		}
		out = append(out, RevisionView{Revision: rev, Attachments: media})
	}
	return out, nil
}

// Source returns the current revision of a local account's own status, whose Source holds
// what the author typed. Another account's status, or one hidden from viewer, fails with
// ErrNotFound, like a missing one.
func (r *Reader) Source(ctx context.Context, viewer Viewer, statusID string) (content.Revision, error) {
	view, err := r.View(ctx, viewer, statusID)
	if err != nil {
		return content.Revision{}, err
	}
	if view.Status.AuthorAccountID != viewer.AccountID || view.Revision.Source == nil {
		return content.Revision{}, fmt.Errorf("source of %s: %w", statusID, ErrNotFound)
	}
	return view.Revision, nil
}

// Bookmarks returns a page of the bookmarked statuses of the local account viewer, newest
// bookmark first (FR-113): up to limit bookmarks older than the cursor before ("" for the
// newest). Statuses that viewer may not see are left out, so a page may be shorter than limit
// and still have a next page; the last page has none.
func (r *Reader) Bookmarks(ctx context.Context, viewer Viewer, before string, limit int) (BookmarkPage, error) {
	marks, err := r.d.Interactions.ListBookmarks(ctx, viewer.AccountID, before, limit+1)
	if err != nil {
		return BookmarkPage{}, err
	}
	var out BookmarkPage
	if len(marks) > limit {
		marks = marks[:limit]
		out.Next = marks[limit-1].ID
	}
	for _, b := range marks {
		view, err := r.View(ctx, viewer, b.StatusID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return BookmarkPage{}, err
		}
		out.Statuses = append(out.Statuses, view)
	}
	return out, nil
}

// present returns st, which viewer may see, with everything a client shows of it.
func (r *Reader) present(ctx context.Context, viewer Viewer, st content.Status, author appaccount.Actor) (StatusView, error) {
	return r.presentWith(ctx, viewer, st, author, true)
}

// presentWith is present; withQuote presents the quote of st, which a quoted status does not.
func (r *Reader) presentWith(ctx context.Context, viewer Viewer, st content.Status, author appaccount.Actor, withQuote bool) (StatusView, error) {
	rev, err := r.d.Statuses.FindCurrentRevision(ctx, st.ID)
	if err != nil {
		return StatusView{}, err // a deleted status has no current revision
	}
	out := StatusView{Status: st, Revision: rev, Author: author, Counters: content.Counters{StatusID: st.ID}}
	if st.InReplyToURI != "" {
		parent, err := r.d.Statuses.FindStatusByURI(ctx, st.InReplyToURI)
		switch {
		case errors.Is(err, ErrNotFound):
		case err != nil:
			return StatusView{}, err
		default:
			_, _, err := r.aud.visibleStatus(ctx, viewer, parent.ID)
			switch {
			case errors.Is(err, ErrNotFound):
			case err != nil:
				return StatusView{}, err
			default:
				out.InReplyTo = &parent
			}
		}
	}
	// A view presents no Emoji tags (a client reads the custom emoji of a page together), so it
	// reads the parts without them.
	parts, err := r.viewParts(ctx, rev)
	if err != nil {
		return StatusView{}, err
	}
	out.Attachments, out.Mentions, out.Tags = parts.Attachments, parts.Mentions, parts.Tags
	if r.d.LinkCards != nil {
		if out.Card, err = r.d.LinkCards.View(ctx, st, rev); err != nil {
			return StatusView{}, err
		}
	}
	// The image of a card is shown as sensitive as the attachments are (ADR-0062 §7).
	if (len(out.Attachments) > 0 || out.Card != nil && out.Card.Image != nil) && !out.Revision.Sensitive {
		if out.Revision.Sensitive, err = r.forcedSensitive(ctx, st); err != nil {
			return StatusView{}, err
		}
	}
	switch c, err := r.d.Statuses.FindCounters(ctx, st.ID); {
	case err == nil:
		out.Counters = c
	case !errors.Is(err, ErrNotFound):
		return StatusView{}, err
	}
	var reactor string
	if viewer.Kind == ViewerLocal && viewer.AccountID != "" {
		reactor = viewer.AccountID
		if out.Relation, err = r.relation(ctx, viewer.AccountID, st); err != nil {
			return StatusView{}, err
		}
	}
	if out.Reactions, err = r.reactions(ctx, st, reactor); err != nil {
		return StatusView{}, err
	}
	if out.Poll, err = r.poll(ctx, rev, reactor); err != nil {
		return StatusView{}, err
	}
	if withQuote {
		if out.Quote, err = r.quote(ctx, viewer, st); err != nil {
			return StatusView{}, err
		}
	}
	if out.Quotable, out.QuotePolicy, err = r.quotable(ctx, viewer, st); err != nil {
		return StatusView{}, err
	}
	return out, nil
}

// forcedSensitive reports whether moderation shows the attachments of st as sensitive
// (data-model §5.2): a mark_statuses_sensitive action marks the status, or its author is under
// force_sensitive.
func (r *Reader) forcedSensitive(ctx context.Context, st content.Status) (bool, error) {
	marked, err := r.d.Statuses.SensitivityOverridden(ctx, st.ID)
	if err != nil || marked {
		return marked, err
	}
	rs, err := r.d.Restrictions.Restrictions(ctx, st.AuthorAccountID)
	return rs.MediaSensitive, err
}

// Parts resolves what rev refers to. It does not decide who may see the revision; callers
// that present it to a viewer decide that first. ActivityPub documents of local statuses are
// built from it (federation.md §7.4).
func (r *Reader) Parts(ctx context.Context, rev content.Revision) (RevisionParts, error) {
	out, err := r.viewParts(ctx, rev)
	if err != nil {
		return RevisionParts{}, err
	}
	if out.Emojis, err = r.EmojiTags(ctx, rev.EmojiIDs); err != nil {
		return RevisionParts{}, err
	}
	if out.Poll, err = r.poll(ctx, rev, ""); err != nil {
		return RevisionParts{}, err
	}
	return out, nil
}

// viewParts resolves the attachments, mentioned accounts, and hashtags of rev, the parts of
// Parts that a StatusView presents.
func (r *Reader) viewParts(ctx context.Context, rev content.Revision) (RevisionParts, error) {
	var out RevisionParts
	var err error
	if out.Attachments, err = r.attachments(ctx, rev.Attachments); err != nil {
		return RevisionParts{}, err
	}
	for _, id := range rev.MentionedAccountIDs {
		a, err := r.d.Accounts.FindByID(ctx, id)
		if errors.Is(err, appaccount.ErrNotFound) {
			continue
		}
		if err != nil {
			return RevisionParts{}, err
		}
		out.Mentions = append(out.Mentions, a)
	}
	for _, id := range rev.TagIDs {
		tag, err := r.d.Statuses.FindTag(ctx, id)
		if err != nil {
			return RevisionParts{}, err
		}
		out.Tags = append(out.Tags, tag)
	}
	return out, nil
}

// EmojiTags returns the Emoji tags of the custom emoji with an ID in ids that can be shown
// ((*MediaService).EmojiTags); none without Media.
func (r *Reader) EmojiTags(ctx context.Context, ids []string) ([]EmojiTag, error) {
	if r.d.Media == nil {
		return nil, nil
	}
	return r.d.Media.EmojiTags(ctx, ids)
}

// relation returns what accountID did with st.
func (r *Reader) relation(ctx context.Context, accountID string, st content.Status) (*ViewerRelation, error) {
	repo := r.d.Interactions
	var rel ViewerRelation
	var err error
	if rel.Favourited, err = found(repo.FindFavourite(ctx, accountID, st.ID)); err != nil {
		return nil, err
	}
	if rel.Boosted, err = found(repo.FindBoostEdge(ctx, accountID, st.ID)); err != nil {
		return nil, err
	}
	if rel.Bookmarked, err = found(repo.FindBookmark(ctx, accountID, st.ID)); err != nil {
		return nil, err
	}
	if st.AuthorAccountID == accountID {
		rel.Author = true
		if rel.Pinned, err = found(repo.FindPin(ctx, accountID, st.ID)); err != nil {
			return nil, err
		}
	}
	return &rel, nil
}

// found turns a lookup into whether it found a row.
func found[T any](_ T, err error) (bool, error) {
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrNotFound):
		return false, nil
	default:
		return false, err
	}
}

// attachments returns the attached media that can be shown, in order: the uploads that finished
// processing, and the remote files when the media proxy is configured.
func (r *Reader) attachments(ctx context.Context, in []content.Attachment) ([]MediaView, error) {
	if r.d.Media == nil {
		return nil, nil
	}
	out := make([]MediaView, 0, len(in))
	for _, at := range in {
		m, err := r.d.Media.Media(ctx, at.MediaID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if img, ok := r.d.Media.Image(m); ok {
			out = append(out, MediaView{Media: m, Description: at.Description, Image: img})
		}
	}
	return out, nil
}
