// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/pkg/testkit"
)

// The cases of RemoteInteractionFlows about the approvals of replies and boosts (ADR-0155 §6–§8):
// the ReplyRequests and AnnounceRequests of local posts, answered at once or by their author; the
// replies and boosts that come without a request; and the requests of local replies and boosts of
// remote posts, whose answers release them.

// requestDoc returns a remote ReplyRequest or AnnounceRequest of target by actor, with
// instrument.
func requestDoc(t *testing.T, typ, id, actor, target string, instrument any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"@context": []any{"https://www.w3.org/ns/activitystreams", "https://gotosocial.org/ns"},
		"id":       id, "type": typ, "actor": actor, "object": target, "instrument": instrument,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// manualPost makes author post a public status whose reply and boost rules admit the author's
// followers at once and everyone else once the author approves.
func (e *remoteInteractionEnv) manualPost(t *testing.T, author string) content.Status {
	t.Helper()
	p, err := e.content.Create(t.Context(), appcontent.CreateInput{
		AuthorAccountID: author, TextInput: appcontent.TextInput{Text: "ask first"}, Visibility: content.VisibilityPublic,
		InteractionPolicy: appcontent.InteractionPolicyChange{
			content.InteractionReply:    {Automatic: content.AudienceFollowers, Manual: content.AudienceEveryone},
			content.InteractionAnnounce: {Automatic: content.AudienceFollowers, Manual: content.AudienceEveryone},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return p.Status
}

// replies returns the IDs of the replies of thread that viewer sees.
func (e *remoteInteractionEnv) replies(t *testing.T, viewer, statusID string) []string {
	t.Helper()
	v := appcontent.Viewer{Kind: appcontent.ViewerAnonymous}
	if viewer != "" {
		v = appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: viewer}
	}
	th, err := e.reader.Thread(t.Context(), v, statusID)
	if err != nil {
		t.Fatalf("Thread(%s) = %v", statusID, err)
	}
	var out []string
	for _, d := range th.Descendants {
		out = append(out, d.Status.ID)
	}
	return out
}

func remoteAnswersReplyAndAnnounceRequests(t *testing.T, e *remoteInteractionEnv) {
	skipWithoutQuoteHandlers(t, e)
	ctx := t.Context()
	author := e.user(t, "author")
	const alice, bob = "https://remote.example/users/alice", "https://other.example/users/bob"
	aliceID, bobID := e.actor(t, alice), e.actor(t, bob)
	e.follow(t, aliceID, author) // alice follows the author: admitted at once, with an approval
	post := e.manualPost(t, author)

	// A follower's ReplyRequest is approved at once: the Accept names this server's
	// ReplyAuthorization, which a GET serves.
	e.expect(t, requestDoc(t, "ReplyRequest", alice+"/requests/1", alice, post.URI, alice+"/notes/r1"), appfederation.InboxHandled)
	sent := e.quoteActivities(t, federation.OutboundQuote, post.ID)
	if len(sent) != 1 || sent[0].Type != federation.ActivityAccept || !slices.Equal(sent[0].Inboxes, []string{alice + "/inbox"}) {
		t.Fatalf("answers = %+v, want an Accept to alice", sent)
	}
	result := string(sent[0].Doc.Result)
	if !strings.HasPrefix(result, "https://"+testkit.ContractHost+"/ap/reply_authorizations/") || sent[0].Doc.Object().ID != alice+"/requests/1" {
		t.Errorf("Accept = %s, want the ReplyRequest with this server's ReplyAuthorization", sent[0].Raw)
	}
	id := result[strings.LastIndexByte(result, '/')+1:]
	if ok, err := e.interactions.VerifyLocalAuthorization(ctx, content.InteractionReply, id, alice+"/notes/r1", aliceID, post.ID); err != nil || !ok {
		t.Errorf("VerifyLocalAuthorization = %t, %v; want true", ok, err)
	}
	// The same request again is answered once (the inbox activity is processed once).
	e.expect(t, requestDoc(t, "ReplyRequest", alice+"/requests/1", alice, post.URI, alice+"/notes/r1"), appfederation.InboxHandled)
	// The reply that follows, naming the approval, is in the thread at once.
	e.expect(t, remoteActivity(t, "Create", alice+"/notes/r1/activity", alice, remoteNote(alice+"/notes/r1", alice, remoteNow.Add(-time.Minute),
		map[string]any{"inReplyTo": post.URI, "replyAuthorization": result})), appfederation.InboxHandled)
	r1 := e.post(t, alice+"/notes/r1").Status
	if got := e.replies(t, "", post.ID); !slices.Equal(got, []string{r1.ID}) {
		t.Errorf("replies after the approved reply = %v, want %s", got, r1.ID)
	}

	// Anyone else's request waits for the author, who is told.
	e.expect(t, requestDoc(t, "ReplyRequest", bob+"/requests/1", bob, post.URI, bob+"/notes/r2"), appfederation.InboxHandled)
	e.expect(t, requestDoc(t, "AnnounceRequest", bob+"/requests/2", bob, post.URI, map[string]any{
		"id": bob + "/announces/1", "type": "Announce", "actor": bob, "object": post.URI, "to": []string{string(federation.PublicIRI)},
	}), appfederation.InboxHandled)
	if sent := e.quoteActivities(t, federation.OutboundQuote, post.ID); len(sent) != 1 {
		t.Errorf("answers while the author decides = %+v, want the first Accept only", sent)
	}
	if n := e.notified(t, author, bobID, notification.TypeReplyRequest); n != 1 {
		t.Errorf("reply_request notifications = %d, want 1", n)
	}
	if n := e.notified(t, author, bobID, notification.TypeBoostRequest); n != 1 {
		t.Errorf("boost_request notifications = %d, want 1", n)
	}
	page, err := e.interactions.PendingInteractionRequests(ctx, author, nil, "", 10)
	if err != nil || len(page.Requests) != 2 {
		t.Fatalf("PendingInteractionRequests = %+v, %v; want bob's two", page, err)
	}
	reply, boost := page.Requests[1], page.Requests[0]
	if reply.Kind != content.InteractionReply || boost.Kind != content.InteractionAnnounce {
		t.Fatalf("pending = %+v, want the reply and the boost, newest first", page.Requests)
	}
	if other := e.user(t, "other"); true {
		if _, err := e.interactions.AnswerInteractionRequest(ctx, other, reply.ID, true); !errors.Is(err, appcontent.ErrNotAuthor) {
			t.Errorf("answer by another account = %v, want ErrNotAuthor", err)
		}
	}
	// The author approves the reply and refuses the boost: an Accept and a Reject go to bob.
	if st, err := e.interactions.AnswerInteractionRequest(ctx, author, reply.ID, true); err != nil || st.State != content.InteractionRequestAccepted {
		t.Fatalf("approve = %+v, %v", st, err)
	}
	if _, err := e.interactions.AnswerInteractionRequest(ctx, author, reply.ID, false); !errors.Is(err, appcontent.ErrRequestAnswered) {
		t.Errorf("a second answer = %v, want ErrRequestAnswered", err)
	}
	if _, err := e.interactions.AnswerInteractionRequest(ctx, author, boost.ID, false); err != nil {
		t.Fatal(err)
	}
	sent = e.quoteActivities(t, federation.OutboundQuote, post.ID)
	if len(sent) != 3 || sent[1].Type != federation.ActivityAccept || sent[2].Type != federation.ActivityReject ||
		!slices.Equal(sent[1].Inboxes, []string{bob + "/inbox"}) || sent[1].Doc.Object().ID != bob+"/requests/1" {
		t.Fatalf("answers = %+v, want an Accept of the ReplyRequest and a Reject of the AnnounceRequest to bob", sent)
	}
	// The boost that comes all the same is not recorded; the approved reply is in the thread.
	e.expect(t, remoteAnnounce(t, bob+"/announces/1", bob, post.URI), appfederation.InboxDropped)
	if boosts, _ := e.counters(t, post.ID); boosts != 0 {
		t.Errorf("boosts = %d, want 0", boosts)
	}
	e.expect(t, remoteActivity(t, "Create", bob+"/notes/r2/activity", bob, remoteNote(bob+"/notes/r2", bob, remoteNow.Add(-time.Minute),
		map[string]any{"inReplyTo": post.URI, "replyAuthorization": string(sent[1].Doc.Result)})), appfederation.InboxHandled)
	if got := e.replies(t, "", post.ID); len(got) != 2 {
		t.Errorf("replies after the approved reply = %v, want both", got)
	}
}

func remoteHoldsRepliesWithoutRequests(t *testing.T, e *remoteInteractionEnv) {
	skipWithoutQuoteHandlers(t, e)
	ctx := t.Context()
	author := e.user(t, "author")
	const carol = "https://remote.example/users/carol"
	carolID := e.actor(t, carol)
	e.follow(t, author, carolID) // so that carol's posts are taken in
	post := e.manualPost(t, author)

	// A reply without a request (as Mastodon sends it) that a manual rule admits is held: out of
	// the thread and the counts, telling the author of the request and no one of a mention.
	note := remoteNote(carol+"/notes/1", carol, remoteNow.Add(-time.Minute), map[string]any{"inReplyTo": post.URI})
	e.expect(t, remoteActivity(t, "Create", carol+"/notes/1/activity", carol, note), appfederation.InboxHandled)
	held := e.post(t, carol+"/notes/1").Status
	if got := e.replies(t, "", post.ID); len(got) != 0 {
		t.Errorf("replies with a held reply = %v, want none", got)
	}
	if n := e.notified(t, author, carolID, notification.TypeReplyRequest); n != 1 {
		t.Errorf("reply_request notifications = %d, want 1", n)
	}
	// Delivered again, it changes nothing.
	e.expect(t, remoteActivity(t, "Create", carol+"/notes/1/activity", carol, note), appfederation.InboxHandled)
	page, err := e.interactions.PendingInteractionRequests(ctx, author, []content.InteractionKind{content.InteractionReply}, "", 10)
	if err != nil || len(page.Requests) != 1 || page.Requests[0].InteractingURI != held.URI || page.StatusIDs[held.URI] != held.ID {
		t.Fatalf("PendingInteractionRequests = %+v, %v; want carol's reply", page, err)
	}
	// Approved, it joins the thread, and the Accept names the reply and its parent.
	if _, err := e.interactions.AnswerInteractionRequest(ctx, author, page.Requests[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if got := e.replies(t, "", post.ID); !slices.Equal(got, []string{held.ID}) {
		t.Errorf("replies after the approval = %v, want %s", got, held.ID)
	}
	sent := e.quoteActivities(t, federation.OutboundQuote, post.ID)
	if len(sent) != 1 || sent[0].Type != federation.ActivityAccept || sent[0].Doc.Object().ID != federation.IRI(held.URI) ||
		sent[0].Doc.Target != federation.IRI(post.URI) || !strings.Contains(string(sent[0].Doc.Result), "/ap/reply_authorizations/") {
		t.Errorf("answers = %+v, want an Accept of the reply to its parent with a ReplyAuthorization", sent)
	}

	// A reply that no rule admits is refused with a Reject and stays held.
	closed, err := e.content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: author, TextInput: appcontent.TextInput{Text: "closed"}, Visibility: content.VisibilityPublic,
		InteractionPolicy: appcontent.InteractionPolicyChange{content.InteractionReply: {Automatic: content.AudienceNobody}},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.expect(t, remoteActivity(t, "Create", carol+"/notes/2/activity", carol, remoteNote(carol+"/notes/2", carol, remoteNow.Add(-time.Minute),
		map[string]any{"inReplyTo": closed.Status.URI})), appfederation.InboxHandled)
	if sent := e.quoteActivities(t, federation.OutboundQuote, closed.Status.ID); len(sent) != 1 || sent[0].Type != federation.ActivityReject {
		t.Errorf("answers = %+v, want a Reject", sent)
	}
	if got := e.replies(t, "", closed.Status.ID); len(got) != 0 {
		t.Errorf("replies of the closed post = %v, want none", got)
	}

	// A pending reply that wider rules admit at once is approved when the author widens them;
	// the refused one stays refused.
	waiting := e.manualPost(t, author)
	e.expect(t, remoteActivity(t, "Create", carol+"/notes/3/activity", carol, remoteNote(carol+"/notes/3", carol, remoteNow.Add(-time.Minute),
		map[string]any{"inReplyTo": waiting.URI})), appfederation.InboxHandled)
	third := e.post(t, carol+"/notes/3").Status
	if got := e.replies(t, "", waiting.ID); len(got) != 0 {
		t.Fatalf("replies before the change = %v, want none", got)
	}
	open := appcontent.InteractionPolicyChange{content.InteractionReply: {Automatic: content.AudienceEveryone}}
	for _, st := range []content.Status{waiting, closed.Status} {
		if _, err := e.content.SetInteractionPolicy(ctx, author, st.ID, open); err != nil {
			t.Fatal(err)
		}
		if err := e.interactions.ReconsiderRequests(ctx, st.ID); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.replies(t, "", waiting.ID); !slices.Equal(got, []string{third.ID}) {
		t.Errorf("replies after widening the rules = %v, want %s", got, third.ID)
	}
	if sent := e.quoteActivities(t, federation.OutboundQuote, waiting.ID); len(sent) != 1 || sent[0].Type != federation.ActivityAccept {
		t.Errorf("answers after widening the rules = %+v, want an Accept", sent)
	}
	if got := e.replies(t, "", closed.Status.ID); len(got) != 0 {
		t.Errorf("replies of the refused reply's post after widening = %v, want none", got)
	}
}

func remoteRequestsLocalRepliesAndBoosts(t *testing.T, e *remoteInteractionEnv) {
	skipWithoutQuoteHandlers(t, e)
	ctx := t.Context()
	const alice = "https://remote.example/users/alice"
	fan, other := e.user(t, "fan"), e.user(t, "other")
	aliceID := e.actor(t, alice)
	e.follow(t, fan, aliceID)
	e.follow(t, other, fan)
	manual := map[string]any{"interactionPolicy": map[string]any{
		"canReply":    map[string]any{"automaticApproval": []string{alice}, "manualApproval": []string{string(federation.PublicIRI)}},
		"canAnnounce": map[string]any{"automaticApproval": []string{alice}, "manualApproval": []string{string(federation.PublicIRI)}},
	}}
	e.expect(t, remoteActivity(t, "Create", alice+"/notes/1/activity", alice, remoteNote(alice+"/notes/1", alice, remoteNow.Add(-time.Hour), manual)), appfederation.InboxHandled)
	post := e.post(t, alice+"/notes/1").Status

	// The local reply is held: its author alone sees it, and a ReplyRequest that embeds its Note
	// goes to alice instead of its Create.
	p, err := e.content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: fan, TextInput: appcontent.TextInput{Text: "may I?"}, Visibility: content.VisibilityPublic, InReplyToID: post.ID,
	})
	if err != nil {
		t.Fatalf("reply = %v", err)
	}
	reply := p.Status
	sent := e.quoteActivities(t, federation.OutboundStatus, reply.ID)
	if len(sent) != 1 || sent[0].Type != federation.ActivityReplyRequest || !slices.Equal(sent[0].Inboxes, []string{alice + "/inbox"}) ||
		sent[0].Doc.Object().ID != federation.IRI(post.URI) || sent[0].Doc.Instrument.ID != federation.IRI(reply.URI) {
		t.Fatalf("activities of the reply = %+v, want a ReplyRequest to alice that embeds it", sent)
	}
	if _, err := e.reader.View(ctx, appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: other}, reply.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("View by another account = %v, want ErrNotFound", err)
	}
	view, err := e.reader.View(ctx, appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: fan}, reply.ID)
	if err != nil || view.Relation == nil || view.Relation.Approval != content.InteractionRequestPending {
		t.Errorf("View by its author = %+v, %v; want it pending", view.Relation, err)
	}
	if got := e.replies(t, fan, post.ID); !slices.Equal(got, []string{reply.ID}) {
		t.Errorf("thread of the reply's author = %v, want the held reply", got)
	}
	if got := e.replies(t, other, post.ID); len(got) != 0 {
		t.Errorf("thread of another account = %v, want none", got)
	}

	// The local boost is a request too: no boost until alice approves it.
	if _, err := e.interactions.Boost(ctx, fan, post.ID); !errors.Is(err, appcontent.ErrInteractionAwaitsApproval) {
		t.Fatalf("boost = %v, want ErrInteractionAwaitsApproval", err)
	}
	if _, err := e.interactions.Boost(ctx, fan, post.ID); !errors.Is(err, appcontent.ErrInteractionAwaitsApproval) {
		t.Fatalf("boost again = %v, want ErrInteractionAwaitsApproval", err)
	}
	own, err := e.interactions.ListOwnInteractions(ctx, fan, []string{post.URI, reply.URI})
	if err != nil || len(own) != 2 {
		t.Fatalf("ListOwnInteractions = %+v, %v; want the reply and one boost", own, err)
	}
	var announceRequest string
	for _, o := range own {
		if o.Kind == content.InteractionAnnounce {
			announceRequest = o.RequestID
		}
	}
	requests := e.quoteActivities(t, federation.OutboundQuote, announceRequest)
	if len(requests) != 1 || requests[0].Type != federation.ActivityAnnounceRequest || requests[0].Doc.Instrument.ID == "" {
		t.Fatalf("AnnounceRequests = %+v, want one that embeds the Announce", requests)
	}
	announceURI := string(requests[0].Doc.Instrument.ID)

	// alice approves both: the authorizations verify, and the Create and the Announce go out,
	// naming them.
	replyAuth, announceAuth := alice+"/authorizations/reply", alice+"/authorizations/announce"
	e.fetcher.docs[replyAuth] = mustJSON(t, map[string]any{"id": replyAuth, "type": "ReplyAuthorization", "attributedTo": alice, "interactingObject": reply.URI, "interactionTarget": post.URI})
	e.fetcher.docs[announceAuth] = mustJSON(t, map[string]any{"id": announceAuth, "type": "AnnounceAuthorization", "attributedTo": alice, "interactingObject": announceURI, "interactionTarget": post.URI})
	accept := func(id, request, result string) []byte {
		return mustJSON(t, map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "id": id, "type": "Accept", "actor": alice,
			"object": request, "result": result})
	}
	e.expect(t, accept(alice+"/accepts/1", string(sent[0].Doc.ID), replyAuth), appfederation.InboxHandled)
	e.expect(t, accept(alice+"/accepts/2", string(requests[0].Doc.ID), announceAuth), appfederation.InboxHandled)
	if n := e.notified(t, fan, aliceID, notification.TypeInteractionAccepted); n != 2 {
		t.Errorf("interaction_accepted notifications = %d, want 2", n)
	}
	note := e.noteOf(t, reply)
	if note == nil || note["replyAuthorization"] != replyAuth {
		t.Errorf("Note of the released reply = %v, want its replyAuthorization", note)
	}
	if got := e.replies(t, other, post.ID); !slices.Equal(got, []string{reply.ID}) {
		t.Errorf("thread of another account after the approval = %v, want the reply", got)
	}
	b, ok := e.liveBoost(t, fan, post.ID)
	if !ok || b.URI != announceURI {
		t.Fatalf("boost after the approval = %+v, %t; want the requested Announce", b, ok)
	}
	announces := e.quoteActivities(t, federation.OutboundBoost, b.ID)
	if len(announces) != 1 || announces[0].Doc.Authorization != federation.IRI(announceAuth) {
		t.Errorf("Announce = %+v, want it to name the AnnounceAuthorization", announces)
	}
	// An answer again, or one of another server, changes nothing.
	e.expect(t, accept(alice+"/accepts/3", string(sent[0].Doc.ID), replyAuth), appfederation.InboxHandled)
	const mallory = "https://evil.example/users/mallory"
	e.actor(t, mallory)
	e.expect(t, mustJSON(t, map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "id": mallory + "/rejects/1", "type": "Reject",
		"actor": mallory, "object": string(sent[0].Doc.ID)}), appfederation.InboxDropped)
	if n := len(e.quoteActivities(t, federation.OutboundStatus, reply.ID)); n != 2 {
		t.Errorf("activities of the reply = %d, want its ReplyRequest and one Create", n)
	}
}
