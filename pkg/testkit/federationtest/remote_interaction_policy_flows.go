// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"errors"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/notification"
)

// The case of RemoteInteractionFlows about who may reply to and boost a post (ADR-0155 §3, §6,
// §7): the reply and boost rules of a local post refuse the local accounts outside them, keep a
// remote reply outside them out of the thread, the replies count, and the notifications while it
// stays stored, and leave a remote boost outside them unrecorded; the rules of a remote post refuse
// the local accounts outside them.

func remoteEnforcesInteractionPolicies(t *testing.T, e *remoteInteractionEnv) {
	ctx := t.Context()
	const alice, bob = "https://remote.example/users/alice", "https://other.example/users/bob"
	author, fan, stranger := e.user(t, "author"), e.user(t, "fan"), e.user(t, "stranger")
	aliceID, bobID := e.actor(t, alice), e.actor(t, bob)
	e.follow(t, fan, author)
	e.follow(t, aliceID, author)
	closed, err := e.content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: author, TextInput: appcontent.TextInput{Text: "closed"}, Visibility: content.VisibilityPublic,
		InteractionPolicy: appcontent.InteractionPolicyChange{
			content.InteractionReply:    {Automatic: content.AudienceNobody},
			content.InteractionAnnounce: {Automatic: content.AudienceFollowers},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	post := closed.Status

	// Local accounts: the author may do anything; a follower may boost but not reply; anyone else
	// may do neither, each refused with its own error.
	if _, err := e.content.Create(ctx, appcontent.CreateInput{AuthorAccountID: stranger, TextInput: appcontent.TextInput{Text: "no"}, Visibility: content.VisibilityPublic, InReplyToID: post.ID}); !errors.Is(err, appcontent.ErrReplyNotAllowed) {
		t.Errorf("reply of a stranger = %v, want ErrReplyNotAllowed", err)
	}
	if _, err := e.content.Create(ctx, appcontent.CreateInput{AuthorAccountID: fan, TextInput: appcontent.TextInput{Text: "no"}, Visibility: content.VisibilityPublic, InReplyToID: post.ID}); !errors.Is(err, appcontent.ErrReplyNotAllowed) {
		t.Errorf("reply of a follower to a post nobody replies to = %v, want ErrReplyNotAllowed", err)
	}
	if _, err := e.interactions.Boost(ctx, stranger, post.ID); !errors.Is(err, appcontent.ErrBoostNotAllowed) {
		t.Errorf("boost of a stranger = %v, want ErrBoostNotAllowed", err)
	}
	if _, err := e.interactions.Boost(ctx, fan, post.ID); err != nil {
		t.Errorf("boost of a follower = %v", err)
	}
	own, err := e.content.Create(ctx, appcontent.CreateInput{AuthorAccountID: author, TextInput: appcontent.TextInput{Text: "more"}, Visibility: content.VisibilityPublic, InReplyToID: post.ID})
	if err != nil {
		t.Fatalf("the author's reply = %v", err)
	}
	if _, err := e.interactions.Boost(ctx, author, post.ID); err != nil {
		t.Errorf("the author's boost = %v", err)
	}
	// The account that a post replies to may reply to it whatever its rules.
	open := e.localPost(t, stranger, content.VisibilityPublic)
	answer, err := e.content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: author, TextInput: appcontent.TextInput{Text: "answer"}, Visibility: content.VisibilityPublic, InReplyToID: open.ID,
		InteractionPolicy: appcontent.InteractionPolicyChange{content.InteractionReply: {Automatic: content.AudienceNobody}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.content.Create(ctx, appcontent.CreateInput{AuthorAccountID: stranger, TextInput: appcontent.TextInput{Text: "thanks"}, Visibility: content.VisibilityPublic, InReplyToID: answer.Status.ID}); err != nil {
		t.Errorf("reply of the account replied to = %v", err)
	}

	// A remote boost by a follower is recorded; one by anyone else is dropped, uncounted and
	// unnotified.
	e.expect(t, remoteAnnounce(t, alice+"/announces/1", alice, post.URI), appfederation.InboxHandled)
	e.expect(t, remoteAnnounce(t, bob+"/announces/1", bob, post.URI), appfederation.InboxDropped)
	if _, ok := e.liveBoost(t, bobID, post.ID); ok {
		t.Error("boost outside the rules recorded")
	}
	if boosts, _ := e.counters(t, post.ID); boosts != 3 {
		t.Errorf("boosts = %d, want 3: the follower's, the author's, and alice's", boosts)
	}
	if n := e.notified(t, author, bobID, notification.TypeBoost); n != 0 {
		t.Errorf("boost notifications of a boost outside the rules = %d, want 0", n)
	}

	// A remote reply outside the rules is stored and shown on its own, but kept out of the thread
	// and the replies count, and it notifies no one, not even after an Update.
	replyID := bob + "/notes/1"
	reply := func(id string, fields map[string]any) map[string]any {
		f := map[string]any{
			"inReplyTo": post.URI, "cc": []string{bob + "/followers", e.localIRI(author)},
			"tag": []any{map[string]any{"type": "Mention", "href": e.localIRI(author)}},
		}
		for k, v := range fields {
			f[k] = v
		}
		return remoteNote(id, bob, remoteNow.Add(-time.Hour), f)
	}
	e.expect(t, remoteActivity(t, "Create", replyID+"/activity", bob, reply(replyID, nil)), appfederation.InboxHandled)
	held := e.post(t, replyID).Status
	viewer := appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: author}
	if _, err := e.reader.View(ctx, viewer, held.ID); err != nil {
		t.Errorf("View of the held reply = %v, want it shown on its own", err)
	}
	e.expect(t, remoteActivity(t, "Update", replyID+"#update-1", bob, reply(replyID, map[string]any{
		"content": "<p>edited</p>", "updated": remoteNow.Format(time.RFC3339),
	})), appfederation.InboxHandled)
	checkThread := func(when string) {
		t.Helper()
		thread, err := e.reader.Thread(ctx, viewer, post.ID)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, len(thread.Descendants))
		for i, v := range thread.Descendants {
			ids[i] = v.Status.ID
		}
		if !slices.Equal(ids, []string{own.Status.ID}) {
			t.Errorf("%s: the thread holds %v, want the author's reply %s alone", when, ids, own.Status.ID)
		}
		if c, err := e.h.Statuses.FindCounters(ctx, post.ID); err != nil || c.RepliesCount != 1 {
			t.Errorf("%s: counters = %+v, %v; want the author's reply alone", when, c, err)
		}
	}
	checkThread("as received")
	if n := e.notified(t, author, bobID, notification.TypeMention); n != 0 {
		t.Errorf("mentions by the held reply = %d, want 0", n)
	}
	// The rebuild of the projections keeps it out too, and a later change of the rules does not
	// let it in.
	if _, err := e.content.SetInteractionPolicy(ctx, author, post.ID, appcontent.InteractionPolicyChange{content.InteractionReply: {Automatic: content.AudienceEveryone}}); err != nil {
		t.Fatal(err)
	}
	if err := appcontent.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Statuses); err != nil {
		t.Fatal(err)
	}
	checkThread("after the rebuild")
	// A reply admitted by the rules joins the thread and notifies the author.
	laterID := bob + "/notes/2"
	e.expect(t, remoteActivity(t, "Create", laterID+"/activity", bob, reply(laterID, nil)), appfederation.InboxHandled)
	if n := e.notified(t, author, bobID, notification.TypeMention); n != 1 {
		t.Errorf("mentions by the admitted reply = %d, want 1", n)
	}
	if c, err := e.h.Statuses.FindCounters(ctx, post.ID); err != nil || c.RepliesCount != 2 {
		t.Errorf("counters after the admitted reply = %+v, %v; want 2 replies", c, err)
	}

	// The rules of a remote post refuse the local accounts outside them: only its author may reply
	// to it here, and anyone may boost it, which it does not declare.
	e.follow(t, fan, aliceID)
	remoteID := alice + "/notes/closed"
	e.expect(t, remoteActivity(t, "Create", remoteID+"/activity", alice, remoteNote(remoteID, alice, remoteNow.Add(-time.Hour), map[string]any{
		"interactionPolicy": map[string]any{"canReply": map[string]any{"automaticApproval": []string{alice}}},
	})), appfederation.InboxHandled)
	remote := e.post(t, remoteID).Status
	if _, err := e.content.Create(ctx, appcontent.CreateInput{AuthorAccountID: fan, TextInput: appcontent.TextInput{Text: "no"}, Visibility: content.VisibilityPublic, InReplyToID: remote.ID}); !errors.Is(err, appcontent.ErrReplyNotAllowed) {
		t.Errorf("reply to a remote post its author alone replies to = %v, want ErrReplyNotAllowed", err)
	}
	if _, err := e.interactions.Boost(ctx, fan, remote.ID); err != nil {
		t.Errorf("boost of a remote post that declares no boost rules = %v", err)
	}
	// A followers-only Note of this server says that its author alone may boost it.
	private := e.localPost(t, author, content.VisibilityPrivate)
	policy := e.noteOf(t, private)["interactionPolicy"].(map[string]any)
	if got := string(mustJSON(t, policy["canAnnounce"])); got != `{"automaticApproval":["`+e.localIRI(author)+`"]}` {
		t.Errorf("canAnnounce of a followers-only Note = %s, want the author alone", got)
	}
	if got := string(mustJSON(t, policy["canReply"])); got != `{"automaticApproval":["`+string(federation.PublicIRI)+`"]}` {
		t.Errorf("canReply of a followers-only Note = %s, want everyone", got)
	}
}
