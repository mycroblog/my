// SPDX-License-Identifier: AGPL-3.0-only

package timelinetest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/timeline"
	"github.com/mycroblog/my/pkg/testkit"
)

// create inserts s with its first revision rev and puts it in a conversation, as the content
// module does (ADR-0154 §2): conversationID, else the conversation of its stored parent, else a
// new one of this server. It emits StatusCreated with the conversation.
func (e *timelineEnv) create(t *testing.T, s content.Status, rev content.Revision, conversationID string) error {
	t.Helper()
	newID := e.IDs.NewID()
	return e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		ctx, repo, conv := t.Context(), e.h.Statuses, conversationID
		if err := repo.InsertStatus(ctx, tx, s); err != nil {
			return err
		}
		if err := repo.InsertRevision(ctx, tx, rev); err != nil {
			return err
		}
		if conv == "" && s.InReplyToURI != "" {
			p, err := repo.FindLiveStatusByURI(ctx, tx, s.InReplyToURI)
			switch {
			case errors.Is(err, appcontent.ErrNotFound):
			case err != nil:
				return err
			default:
				of, err := repo.StatusConversationsTx(ctx, tx, []string{p.ID})
				if err != nil {
					return err
				}
				conv = of[p.ID].ID
			}
		}
		if conv == "" {
			c, err := repo.EnsureConversation(ctx, tx, content.Conversation{ID: newID, URI: content.ConversationURI(testkit.ContractHost, newID), IsLocal: true})
			if err != nil {
				return err
			}
			conv = c.ID
		}
		if err := repo.InsertStatusConversation(ctx, tx, s.ID, conv); err != nil {
			return err
		}
		return tx.Emit(ctx, appcontent.StatusCreated{Status: s, Revision: rev, ConversationID: conv})
	})
}

// conversationOf returns the conversation that a status belongs to now.
func (e *timelineEnv) conversationOf(t *testing.T, s content.Status) string {
	t.Helper()
	of, err := e.h.Statuses.StatusConversations(t.Context(), []string{s.ID})
	if err != nil {
		t.Fatalf("StatusConversations(%s): %v", s.ID, err)
	}
	return of[s.ID].ID
}

// mergeOf merges the conversation of reply into that of parent, as the content module does when
// a reply links them, emitting ConversationMerged; nothing happens when they are one.
func (e *timelineEnv) mergeOf(t *testing.T, parent, reply content.Status) {
	t.Helper()
	into, from := e.conversationOf(t, parent), e.conversationOf(t, reply)
	if into == "" || from == "" || into == from {
		return
	}
	m := content.ConversationMerge{MergedConversationID: from, IntoConversationID: into, StatusID: reply.ID, MergedAt: e.Clock.Now()}
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := e.h.Statuses.MergeConversation(t.Context(), tx, m); err != nil {
			return err
		}
		return tx.Emit(t.Context(), appcontent.ConversationMerged{Merge: m})
	})
	if err != nil {
		t.Fatalf("merge conversation %s into %s: %v", from, into, err)
	}
}

// dm posts a direct post of author to the accounts to, in the conversation conv ("" for a new
// one).
func (e *timelineEnv) dm(t *testing.T, author, conv string, to ...string) content.Status {
	t.Helper()
	return e.post(t, author, content.VisibilityDirect, postOpts{mentions: to, conversation: conv})
}

// conversationResolver resolves the conversations of the content repository through their
// merges, as content.Service.ResolveConversation does.
type conversationResolver struct{ repo appcontent.Repository }

func (r conversationResolver) ResolveConversation(ctx context.Context, id string) (appcontent.ResolvedConversation, error) {
	named, err := r.repo.FindConversation(ctx, id)
	if err != nil {
		return appcontent.ResolvedConversation{}, err
	}
	out := appcontent.ResolvedConversation{Named: named, Final: named}
	for range content.MaxConversationMergeHops {
		m, err := r.repo.FindConversationMerge(ctx, out.Final.ID)
		if errors.Is(err, appcontent.ErrNotFound) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		if out.Final, err = r.repo.FindConversation(ctx, m.IntoConversationID); err != nil {
			return out, err
		}
	}
	return out, content.ErrConversationMergeChain
}

// conv is a conversation of a list as the flows compare it.
type conv struct {
	ID       string
	Last     string
	Accounts []string
	Unread   bool
}

func (c conv) String() string {
	return fmt.Sprintf("{%s last %s accounts %v unread %v}", c.ID, c.Last, c.Accounts, c.Unread)
}

func convOf(c apptimeline.Conversation) conv {
	accounts := slices.Clone(c.AccountIDs)
	slices.Sort(accounts)
	return conv{ID: c.ID, Last: c.LastEntry.ID, Accounts: accounts, Unread: c.Unread}
}

func newConv(id, last string, unread bool, accounts ...string) conv {
	slices.Sort(accounts)
	return conv{ID: id, Last: last, Accounts: accounts, Unread: unread}
}

// conversations checks the whole list of viewer.
func (e *timelineEnv) conversations(t *testing.T, viewer string, want ...conv) {
	t.Helper()
	p, err := e.svc.Conversations(t.Context(), viewerOf(viewer), apptimeline.Query{Limit: apptimeline.MaxLimit})
	if err != nil {
		t.Fatalf("Conversations(%s): %v", viewer, err)
	}
	got := make([]conv, len(p.Conversations))
	for i, c := range p.Conversations {
		got[i] = convOf(c)
	}
	if !slices.EqualFunc(got, want, func(a, b conv) bool { return a.String() == b.String() }) {
		t.Errorf("Conversations(%s) = %v, want %v", viewer, got, want)
	}
	if p.Next != "" {
		t.Errorf("Conversations(%s).Next = %q, want none", viewer, p.Next)
	}
}

func (e *timelineEnv) conversation(t *testing.T, viewer, id string) conv {
	t.Helper()
	c, err := e.svc.Conversation(t.Context(), viewerOf(viewer), id)
	if err != nil {
		t.Fatalf("Conversation(%s, %s): %v", viewer, id, err)
	}
	return convOf(c)
}

func conversationsList(t *testing.T, e *timelineEnv) {
	alice, bob, carol := e.account(true), e.account(true), e.account(true)
	remote := e.account(false)
	toBob := e.dm(t, alice, "", bob)
	c1 := e.conversationOf(t, toBob)
	fromRemote := e.dm(t, remote, "", carol)
	c2 := e.conversationOf(t, fromRemote)
	e.post(t, alice, content.VisibilityPublic, postOpts{mentions: []string{bob}})
	e.post(t, alice, content.VisibilityPrivate, postOpts{mentions: []string{carol}})

	// Neither follows the other: the list has the direct posts sent and received.
	e.conversations(t, bob, newConv(c1, toBob.ID, true, alice))
	e.conversations(t, alice, newConv(c1, toBob.ID, false, bob))
	e.conversations(t, carol, newConv(c2, fromRemote.ID, true, remote))
	e.conversations(t, e.account(true))

	// A token without read:direct sees none of them.
	denied := viewerOf(bob)
	denied.DirectDenied = true
	if p, err := e.svc.Conversations(t.Context(), denied, apptimeline.Query{}); err != nil || len(p.Conversations) != 0 {
		t.Errorf("Conversations without read:direct = %v, %v, want none", p.Conversations, err)
	}
	if _, err := e.svc.Conversations(t.Context(), anonymous, apptimeline.Query{}); !errors.Is(err, apptimeline.ErrLoginRequired) {
		t.Errorf("Conversations(anonymous) = %v, want ErrLoginRequired", err)
	}
	if _, err := e.svc.Conversation(t.Context(), viewerOf(carol), c1); !errors.Is(err, apptimeline.ErrNotFound) {
		t.Errorf("Conversation of another's conversation = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.Conversation(t.Context(), viewerOf(bob), e.IDs.NewID()); !errors.Is(err, apptimeline.ErrNotFound) {
		t.Errorf("Conversation of an unknown ID = %v, want ErrNotFound", err)
	}
}

func conversationsPaging(t *testing.T, e *timelineEnv) {
	alice := e.account(true)
	var posts []content.Status
	var ids []string
	for range 3 {
		s := e.dm(t, e.account(true), "", alice)
		posts, ids = append(posts, s), append(ids, e.conversationOf(t, s))
	}
	page := func(before string, want ...string) string {
		t.Helper()
		p, err := e.svc.Conversations(t.Context(), viewerOf(alice), apptimeline.Query{Before: before, Limit: 2})
		if err != nil {
			t.Fatalf("Conversations: %v", err)
		}
		var got []string
		for _, c := range p.Conversations {
			got = append(got, c.ID)
		}
		if !slices.Equal(got, want) {
			t.Errorf("Conversations(before %q) = %v, want %v", before, got, want)
		}
		return p.Next
	}
	next := page("", ids[2], ids[1])
	if next != posts[1].ID {
		t.Errorf("Next = %q, want the last entry of the page %q", next, posts[1].ID)
	}
	if next := page(next, ids[0]); next != "" {
		t.Errorf("Next of the last page = %q, want none", next)
	}
	// A newer post brings its conversation to the top.
	e.dm(t, alice, ids[0], posts[0].AuthorAccountID)
	page("", ids[0], ids[2])
}

func conversationsVisibility(t *testing.T, e *timelineEnv) {
	alice, bob, carol := e.account(true), e.account(true), e.account(true)
	s1 := e.dm(t, bob, "", alice)
	c1 := e.conversationOf(t, s1)
	s2 := e.dm(t, alice, c1, bob)
	s3 := e.dm(t, bob, c1, alice)
	s4 := e.dm(t, carol, "", alice)
	c2 := e.conversationOf(t, s4)
	e.conversations(t, alice, newConv(c2, s4.ID, true, carol), newConv(c1, s3.ID, true, bob))

	// A muted author's posts are skipped for the previous visible one, the viewer's own.
	if _, err := e.graph.Mute(t.Context(), alice, bob, appgraph.MuteInput{}); err != nil {
		t.Fatal(err)
	}
	e.conversations(t, alice, newConv(c2, s4.ID, true, carol), newConv(c1, s2.ID, false, bob))
	if _, err := e.graph.Unmute(t.Context(), alice, bob); err != nil {
		t.Fatal(err)
	}
	// A conversation with no visible post left is left out.
	if _, err := e.graph.Block(t.Context(), alice, carol); err != nil {
		t.Fatal(err)
	}
	e.conversations(t, alice, newConv(c1, s3.ID, true, bob))
	if _, err := e.svc.Conversation(t.Context(), viewerOf(alice), c2); !errors.Is(err, apptimeline.ErrNotFound) {
		t.Errorf("Conversation with no visible post = %v, want ErrNotFound", err)
	}
	// The restrictions of an author apply as on the home timeline.
	e.restrict(t, bob, timeline.RestrictProfileView)
	e.conversations(t, alice, newConv(c1, s2.ID, false, bob))
}

func conversationsRead(t *testing.T, e *timelineEnv) {
	alice, bob, carol := e.account(true), e.account(true), e.account(true)
	s1 := e.dm(t, bob, "", alice)
	c1 := e.conversationOf(t, s1)
	got, err := e.svc.ReadConversation(t.Context(), viewerOf(alice), c1)
	if err != nil {
		t.Fatalf("ReadConversation: %v", err)
	}
	if want := newConv(c1, s1.ID, false, bob); convOf(got).String() != want.String() {
		t.Errorf("ReadConversation = %v, want %v", convOf(got), want)
	}
	e.conversations(t, alice, newConv(c1, s1.ID, false, bob))
	// A newer post by another account is unread; the viewer's own is not.
	s2 := e.dm(t, bob, c1, alice)
	e.conversations(t, alice, newConv(c1, s2.ID, true, bob))
	s3 := e.dm(t, alice, c1, bob)
	e.conversations(t, alice, newConv(c1, s3.ID, true, bob))
	if _, err := e.svc.ReadConversation(t.Context(), viewerOf(alice), c1); err != nil {
		t.Fatalf("ReadConversation: %v", err)
	}
	e.conversations(t, alice, newConv(c1, s3.ID, false, bob))
	// Reading another's conversation, or an unknown one, is not found and changes nothing.
	for _, id := range []string{c1, e.IDs.NewID()} {
		if _, err := e.svc.ReadConversation(t.Context(), viewerOf(carol), id); !errors.Is(err, apptimeline.ErrNotFound) {
			t.Errorf("ReadConversation(carol, %s) = %v, want ErrNotFound", id, err)
		}
	}
	e.conversations(t, bob, newConv(c1, s3.ID, true, alice))
}

func conversationsHide(t *testing.T, e *timelineEnv) {
	alice, bob := e.account(true), e.account(true)
	s1 := e.dm(t, bob, "", alice)
	c1 := e.conversationOf(t, s1)
	for range 2 {
		if err := e.svc.HideConversation(t.Context(), viewerOf(alice), c1); err != nil {
			t.Fatalf("HideConversation: %v", err)
		}
	}
	e.conversations(t, alice)
	e.conversations(t, bob, newConv(c1, s1.ID, false, alice))
	if got, want := e.conversation(t, alice, c1), newConv(c1, s1.ID, true, bob); got.String() != want.String() {
		t.Errorf("Conversation of a hidden one = %v, want %v", got, want)
	}
	if _, ok, err := e.svc.StreamConversation(t.Context(), viewerOf(alice), c1); err != nil || ok {
		t.Errorf("StreamConversation of a hidden one = %v, %v, want none", ok, err)
	}
	// A newer post brings it back.
	s2 := e.dm(t, bob, c1, alice)
	e.conversations(t, alice, newConv(c1, s2.ID, true, bob))
	if c, ok, err := e.svc.StreamConversation(t.Context(), viewerOf(alice), c1); err != nil || !ok || c.LastEntry.ID != s2.ID {
		t.Errorf("StreamConversation = %v, %v, %v, want the new post", c.LastEntry.ID, ok, err)
	}
	if err := e.svc.HideConversation(t.Context(), viewerOf(e.account(true)), c1); !errors.Is(err, apptimeline.ErrNotFound) {
		t.Errorf("HideConversation of another's = %v, want ErrNotFound", err)
	}
}

func conversationsMerge(t *testing.T, e *timelineEnv) {
	alice, bob, carol := e.account(true), e.account(true), e.account(true)
	s1 := e.dm(t, bob, "", alice)
	c1 := e.conversationOf(t, s1)
	s2 := e.dm(t, carol, "", alice)
	c2 := e.conversationOf(t, s2)
	if _, err := e.svc.ReadConversation(t.Context(), viewerOf(alice), c2); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.HideConversation(t.Context(), viewerOf(alice), c1); err != nil {
		t.Fatal(err)
	}
	e.conversations(t, alice, newConv(c2, s2.ID, false, carol))

	// The merge keeps the newer boundary of each: read up to s2, hidden only up to s1.
	e.mergeOf(t, s1, s2)
	e.conversations(t, alice, newConv(c1, s2.ID, false, bob, carol))
	// Each account sees the parties of its own direct posts only.
	e.conversations(t, bob, newConv(c1, s1.ID, false, alice))
	e.conversations(t, carol, newConv(c1, s2.ID, false, alice))
	// The merged conversation's ID answers with the conversation it was merged into.
	if got := e.conversation(t, alice, c2); got.ID != c1 {
		t.Errorf("Conversation(%s) = %v, want %s", c2, got, c1)
	}
	if got, err := e.svc.ReadConversation(t.Context(), viewerOf(bob), c2); err != nil || got.ID != c1 {
		t.Errorf("ReadConversation(%s) = %v, %v, want %s", c2, got.ID, err, c1)
	}
	s3 := e.dm(t, bob, c1, alice, carol)
	e.conversations(t, carol, newConv(c1, s3.ID, true, alice, bob))
}

func conversationsEditsAndDeletes(t *testing.T, e *timelineEnv) {
	alice, bob, carol := e.account(true), e.account(true), e.account(true)
	s1 := e.dm(t, alice, "", bob, carol)
	c1 := e.conversationOf(t, s1)
	e.conversations(t, carol, newConv(c1, s1.ID, true, alice, bob))
	// An edit that drops a recipient takes the conversation out of its list.
	e.reviseWith(t, s1, postOpts{mentions: []string{bob}, language: "en"})
	e.conversations(t, carol)
	e.conversations(t, bob, newConv(c1, s1.ID, true, alice))
	// Deleting the last post falls back to the previous one; deleting all of them empties it.
	e.Clock.Advance(time.Millisecond)
	s2 := e.dm(t, alice, c1, bob)
	e.conversations(t, bob, newConv(c1, s2.ID, true, alice))
	e.MustRemove(t, s2)
	e.conversations(t, bob, newConv(c1, s1.ID, true, alice))
	e.MustRemove(t, s1)
	e.conversations(t, bob)
	e.conversations(t, alice)
}
