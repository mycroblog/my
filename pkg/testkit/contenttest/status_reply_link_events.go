// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

import (
	"reflect"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// createLinked inserts s with its first revision, held out of its thread when heldBy is the
// author of its parent (with the reply request that holds it, as the content module stores one),
// and emits the StatusCreated that LinkCreated fills in, which it returns.
func (e *StatusEnv) createLinked(t *testing.T, s content.Status, heldBy string) appcontent.StatusCreated {
	t.Helper()
	var ev appcontent.StatusCreated
	err := e.H.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		ctx := t.Context()
		if err := e.H.Statuses.InsertStatus(ctx, tx, s); err != nil {
			return err
		}
		rev := e.FirstRevision(s)
		if err := e.H.Statuses.InsertRevision(ctx, tx, rev); err != nil {
			return err
		}
		if heldBy != "" {
			approvals := e.H.Statuses.(appcontent.ApprovalRepository)
			if _, err := approvals.InsertInteractionRequest(ctx, tx, content.InteractionRequest{
				ID: e.IDs.NewID(), Kind: content.InteractionReply, Direction: content.DirectionInbound, TargetURI: s.InReplyToURI,
				TargetAccountID: heldBy, InteractingURI: s.URI, InteractingAccountID: s.AuthorAccountID, CreatedAt: e.Clock.Now(),
			}); err != nil {
				return err
			}
		}
		var err error
		ev, err = appcontent.LinkCreated(ctx, tx, e.H.Statuses, appcontent.StatusCreated{Status: s, Revision: rev, HeldReply: heldBy != ""})
		if err != nil {
			return err
		}
		return tx.Emit(ctx, ev)
	})
	if err != nil {
		t.Fatalf("create status %s: %v", s.URI, err)
	}
	return ev
}

// statusCreatedCarriesReplyLinks checks the links that LinkCreated puts in StatusCreated, which
// the timelines copy (ADR-0013 §31): ReplyLink is the link that storing the status made, set only
// for a reply that is not held whose parent is stored and live; LinkedReplies are the links that
// storing a parent made from the replies that waited for it, neither deleted nor held. Both are
// the links that status_reply_links holds afterwards.
func statusCreatedCarriesReplyLinks(t *testing.T, e *StatusEnv) {
	_, approvals := e.H.Statuses.(appcontent.ApprovalRepository)
	parentAuthor := e.IDs.NewID()
	parent := e.NewStatus(parentAuthor, content.VisibilityPublic)
	reply := func(to content.Status) content.Status {
		s := e.NewStatus(e.IDs.NewID(), content.VisibilityPublic)
		s.InReplyToURI = to.URI
		return s
	}
	check := func(what string, ev appcontent.StatusCreated, link *content.ReplyLink, linked ...content.ReplyLink) {
		t.Helper()
		if !reflect.DeepEqual(ev.ReplyLink, link) {
			t.Errorf("%s: ReplyLink = %+v, want %+v", what, ev.ReplyLink, link)
		}
		if len(ev.LinkedReplies) != len(linked) || len(linked) > 0 && !reflect.DeepEqual(ev.LinkedReplies, linked) {
			t.Errorf("%s: LinkedReplies = %+v, want %+v", what, ev.LinkedReplies, linked)
		}
	}
	linkOf := func(s content.Status) content.ReplyLink {
		return content.ReplyLink{StatusID: s.ID, ParentStatusID: parent.ID, ParentAuthorAccountID: parentAuthor}
	}

	// Replies that arrive before their parent are not linked yet.
	early := reply(parent)
	check("a reply before its parent", e.createLinked(t, early, ""), nil)
	gone := reply(parent)
	e.createLinked(t, gone, "")
	e.MustRemove(t, gone)
	if approvals {
		check("a held reply before its parent", e.createLinked(t, reply(parent), parentAuthor), nil)
	}
	// The parent links the waiting reply that is neither deleted nor held.
	check("the parent", e.createLinked(t, parent, ""), nil, linkOf(early))
	// A reply to a stored parent is linked at once, unless it is held.
	late := reply(parent)
	want := linkOf(late)
	check("a reply to a stored parent", e.createLinked(t, late, ""), &want)
	if approvals {
		check("a held reply to a stored parent", e.createLinked(t, reply(parent), parentAuthor), nil)
	}
	// A reply to a reply links to its own parent.
	nested := reply(late)
	check("a reply to a reply", e.createLinked(t, nested, ""),
		&content.ReplyLink{StatusID: nested.ID, ParentStatusID: late.ID, ParentAuthorAccountID: late.AuthorAccountID})
	// A reply to a deleted parent is not linked.
	e.MustRemove(t, parent)
	check("a reply to a deleted parent", e.createLinked(t, reply(parent), ""), nil)

	links, err := e.H.Statuses.ListReplyLinks(t.Context())
	if err != nil || len(links) != 1 || links[0].StatusID != nested.ID {
		t.Errorf("ListReplyLinks = %+v, %v; want only the link of the reply to the live reply", links, err)
	}
}
