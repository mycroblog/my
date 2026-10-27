// SPDX-License-Identifier: AGPL-3.0-only

package testkit

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
)

// The cases of RemoteInteractionContract about the quotes that local accounts post and the
// approvals this server gives (FR-114, ADR-0069 §3): quotes of local posts approved at once, the
// QuoteRequest of a quote of a remote post and its answer, the answer to a remote QuoteRequest,
// and the revocation of an approval.

func skipWithoutQuoteHandlers(t *testing.T, e *remoteInteractionEnv) {
	t.Helper()
	if e.quotes == nil {
		t.Skip("the harness stores no quotes")
	}
}

// quotePost makes the local account author post a quote of quoted with visibility v.
func (e *remoteInteractionEnv) quotePost(t *testing.T, author, quoted string, v content.Visibility) (content.Status, error) {
	t.Helper()
	p, err := e.content.Create(t.Context(), appcontent.CreateInput{
		AuthorAccountID: author, TextInput: appcontent.TextInput{Text: "look"}, Visibility: v, QuotedStatusID: quoted,
	})
	return p.Status, err
}

// quoteSent is an activity of the approval of a quote, as it was delivered.
type quoteSent struct {
	Type    federation.ActivityType
	Doc     *federation.Activity
	Raw     []byte
	Inboxes []string
}

// runFanOut runs the pending fan-outs of every outbound activity.
func (e *remoteInteractionEnv) runFanOut(t *testing.T) {
	t.Helper()
	fanOut, _, ok := e.handlers.Lookup(appfederation.JobFanOut)
	if !ok {
		t.Fatal("no handler for " + appfederation.JobFanOut)
	}
	for {
		nj, ok := e.jobs.pop()
		if !ok {
			return
		}
		if nj.Type != appfederation.JobFanOut {
			continue
		}
		j, err := nj.Build(e.ids.NewID(), e.clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		if r := fanOut(t.Context(), j); !r.IsDone() {
			t.Fatalf("fan-out %s = %+v (%v), want Done", j.Payload, r, r.Err())
		}
	}
}

// quoteActivities returns the activities of kind about objectID, after running the fan-outs.
func (e *remoteInteractionEnv) quoteActivities(t *testing.T, kind federation.OutboundObjectKind, objectID string) []quoteSent {
	t.Helper()
	e.runFanOut(t)
	acts, err := e.ih.Federation.ListOutboundActivitiesOf(t.Context(), kind, objectID)
	if err != nil {
		t.Fatalf("ListOutboundActivitiesOf: %v", err)
	}
	var out []quoteSent
	for _, a := range acts {
		s := quoteSent{Type: a.Type}
		if s.Raw, err = e.ih.Federation.FindOutboundDocument(t.Context(), a.ID); err != nil {
			t.Fatalf("FindOutboundDocument(%s): %v", a.ID, err)
		}
		doc, err := appfederation.Normalize(s.Raw, e.clock.Now())
		if err != nil || doc.Activity == nil {
			t.Fatalf("Normalize(%s) = %+v, %v", s.Raw, doc, err)
		}
		s.Doc = doc.Activity
		ds, err := e.ih.Federation.ListDeliveries(t.Context(), a.ID, "", 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range ds {
			s.Inboxes = append(s.Inboxes, string(d.InboxURL))
		}
		out = append(out, s)
	}
	return out
}

// quoteState returns the state of the quote of the stored status id.
func (e *remoteInteractionEnv) quoteState(t *testing.T, id string) content.QuoteState {
	t.Helper()
	_, s, err := e.content.FindQuote(t.Context(), id)
	if err != nil {
		t.Fatalf("FindQuote(%s) = %v", id, err)
	}
	return s
}

// noteOf returns the Note of the local status st as its Create delivered it.
func (e *remoteInteractionEnv) noteOf(t *testing.T, st content.Status) map[string]any {
	t.Helper()
	for _, a := range e.quoteActivities(t, federation.OutboundStatus, st.ID) {
		if a.Type != federation.ActivityCreate {
			continue
		}
		var doc struct {
			Object map[string]any `json:"object"`
		}
		if err := json.Unmarshal(a.Raw, &doc); err != nil {
			t.Fatal(err)
		}
		return doc.Object
	}
	t.Fatalf("no Create of %s", st.ID)
	return nil
}

func remoteQuotesLocalPosts(t *testing.T, e *remoteInteractionEnv) {
	skipWithoutQuoteHandlers(t, e)
	ctx := t.Context()
	author, quoter := e.user(t, "author"), e.user(t, "quoter")
	public := e.localPost(t, author, content.VisibilityPublic)

	// A quote of a local post is approved at once with this server's QuoteAuthorization, and its
	// author is notified.
	q, err := e.quotePost(t, quoter, public.ID, content.VisibilityPublic)
	if err != nil {
		t.Fatalf("quote = %v", err)
	}
	s := e.quoteState(t, q.ID)
	if s.Approval != content.QuoteApprovalAccepted || !strings.HasPrefix(s.AuthorizationURI, "https://"+contractHost+"/ap/quote_authorizations/") {
		t.Errorf("state = %+v, want accepted by a QuoteAuthorization of this server", s)
	}
	if n := e.notified(t, author, quoter, notification.TypeQuote); n != 1 {
		t.Errorf("quote notifications = %d, want 1", n)
	}
	id := s.AuthorizationURI[strings.LastIndexByte(s.AuthorizationURI, '/')+1:]
	if ok, err := e.interactions.VerifyLocalQuoteAuthorization(ctx, id, q.URI, quoter, public.ID); err != nil || !ok {
		t.Errorf("VerifyLocalQuoteAuthorization = %v, %v; want true", ok, err)
	}
	// Its Note quotes the post with every alias, the approval, and the fallback, and says who
	// may quote it.
	note := e.noteOf(t, q)
	for _, k := range []string{"quote", "_misskey_quote", "quoteUrl", "quoteUri"} {
		if note[k] != public.URI {
			t.Errorf("%s = %v, want %s", k, note[k], public.URI)
		}
	}
	if note["quoteAuthorization"] != s.AuthorizationURI {
		t.Errorf("quoteAuthorization = %v, want %s", note["quoteAuthorization"], s.AuthorizationURI)
	}
	if c, _ := note["content"].(string); !strings.HasPrefix(c, `<p class="quote-inline">RE: <a href="`) {
		t.Errorf("content = %q, want the quote-inline fallback first", c)
	}
	policy, _ := json.Marshal(note["interactionPolicy"])
	if string(policy) != `{"canQuote":{"automaticApproval":["`+string(federation.PublicIRI)+`"]}}` {
		t.Errorf("interactionPolicy = %s, want everyone", policy)
	}

	// The author's own post, also a followers-only one in a followers-only post, needs no
	// QuoteAuthorization.
	private := e.localPost(t, author, content.VisibilityPrivate)
	own, err := e.quotePost(t, author, private.ID, content.VisibilityPrivate)
	if err != nil {
		t.Fatalf("own quote = %v", err)
	}
	if s := e.quoteState(t, own.ID); s.Approval != content.QuoteApprovalAccepted || s.AuthorizationURI != "" {
		t.Errorf("state of a quote of the author's own post = %+v, want accepted without an authorization", s)
	}
	// The policy and visibility refuse the others.
	if _, err := e.quotePost(t, author, private.ID, content.VisibilityPublic); !errors.Is(err, appcontent.ErrQuoteNotAllowed) {
		t.Errorf("public quote of an own followers-only post = %v, want ErrQuoteNotAllowed", err)
	}
	if _, err := e.content.SetQuotePolicy(ctx, author, public.ID, content.QuotePolicyNobody); err != nil {
		t.Fatal(err)
	}
	if _, err := e.quotePost(t, quoter, public.ID, content.VisibilityPublic); !errors.Is(err, appcontent.ErrQuoteNotAllowed) {
		t.Errorf("quote of a post of nobody = %v, want ErrQuoteNotAllowed", err)
	}
	if _, err := e.quotePost(t, quoter, e.ids.NewID(), content.VisibilityPublic); !errors.Is(err, appcontent.ErrQuoteTargetNotFound) {
		t.Errorf("quote of a missing post = %v, want ErrQuoteTargetNotFound", err)
	}
	// The change of the policy goes out in an Update.
	updates := 0
	for _, a := range e.quoteActivities(t, federation.OutboundStatus, public.ID) {
		if a.Type == federation.ActivityUpdate {
			updates++
		}
	}
	if updates != 1 {
		t.Errorf("Updates after the policy changed = %d, want 1", updates)
	}

	// An edit can take the quote away, for good.
	if _, err := e.content.Edit(ctx, appcontent.EditInput{AccountID: quoter, StatusID: q.ID, TextInput: appcontent.TextInput{Text: "no more"}, DetachQuote: true}); err != nil {
		t.Fatal(err)
	}
	if s := e.quoteState(t, q.ID); s.Approval != content.QuoteApprovalDetached {
		t.Errorf("state after removing the quote = %+v, want detached", s)
	}
}

func remoteSendsQuoteRequests(t *testing.T, e *remoteInteractionEnv) {
	skipWithoutQuoteHandlers(t, e)
	const alice = "https://remote.example/users/alice"
	fan := e.user(t, "fan")
	aliceID := e.actor(t, alice)
	e.follow(t, fan, aliceID)
	published := remoteNow.Add(-time.Hour)
	deliver := func(id string, fields map[string]any) content.Status {
		t.Helper()
		e.expect(t, remoteActivity(t, "Create", id+"/activity", alice, remoteNote(id, alice, published, fields)), appfederation.InboxHandled)
		return e.post(t, id).Status
	}
	everyone := map[string]any{"interactionPolicy": map[string]any{"canQuote": map[string]any{"automaticApproval": []string{string(federation.PublicIRI)}}}}
	open := deliver(alice+"/notes/open", everyone)
	legacy := deliver(alice+"/notes/legacy", nil)
	closed := deliver(alice+"/notes/closed", map[string]any{"interactionPolicy": map[string]any{"canQuote": map[string]any{"automaticApproval": []string{alice}}}})

	if _, err := e.quotePost(t, fan, closed.ID, content.VisibilityPublic); !errors.Is(err, appcontent.ErrQuoteNotAllowed) {
		t.Errorf("quote of a post its author alone may quote = %v, want ErrQuoteNotAllowed", err)
	}
	// A post without canQuote is quoted as a legacy quote, without a QuoteRequest.
	l, err := e.quotePost(t, fan, legacy.ID, content.VisibilityPublic)
	if err != nil {
		t.Fatalf("legacy quote = %v", err)
	}
	if s := e.quoteState(t, l.ID); s.Approval != content.QuoteApprovalLegacy {
		t.Errorf("state = %+v, want legacy", s)
	}
	if got := e.quoteActivities(t, federation.OutboundQuote, l.ID); len(got) != 0 {
		t.Errorf("activities of a legacy quote = %+v, want none", got)
	}

	// A post that everyone may quote gets a QuoteRequest to its author, and waits for the answer.
	q, err := e.quotePost(t, fan, open.ID, content.VisibilityPublic)
	if err != nil {
		t.Fatalf("quote = %v", err)
	}
	if s := e.quoteState(t, q.ID); s.Approval != content.QuoteApprovalPending {
		t.Errorf("state = %+v, want pending", s)
	}
	sent := e.quoteActivities(t, federation.OutboundQuote, q.ID)
	if len(sent) != 1 || sent[0].Type != federation.ActivityQuoteRequest || !slices.Equal(sent[0].Inboxes, []string{alice + "/inbox"}) {
		t.Fatalf("activities = %+v, want a QuoteRequest to alice", sent)
	}
	request := sent[0].Doc
	if request.ID != federation.IRI(q.URI+"/quote_request") || request.Object().ID != federation.IRI(open.URI) || request.Instrument.ID != federation.IRI(q.URI) {
		t.Errorf("QuoteRequest = %+v, want %s quoting %s", request, q.URI, open.URI)
	}

	// Another's Accept, and one naming a QuoteAuthorization that does not approve the quote,
	// change nothing; the author's Accept with its QuoteAuthorization approves it, which an
	// Update tells.
	bad := alice + "/quote_authorizations/bad"
	e.fetcher.docs[bad] = mustJSON(t, map[string]any{"id": bad, "type": "QuoteAuthorization", "attributedTo": alice, "interactingObject": alice + "/notes/x", "interactionTarget": open.URI})
	accept := func(id, actor, result string) []byte {
		b, err := json.Marshal(map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "id": id, "type": "Accept", "actor": actor, "object": request.ID, "result": result})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	const mallory = "https://other.example/users/mallory"
	e.actor(t, mallory)
	e.expect(t, accept(mallory+"#accept", mallory, bad), appfederation.InboxDropped)
	e.expect(t, accept(alice+"#accept-bad", alice, bad), appfederation.InboxDropped)
	if s := e.quoteState(t, q.ID); s.Approval != content.QuoteApprovalPending {
		t.Errorf("state after a wrong Accept = %+v, want pending", s)
	}
	auth := alice + "/quote_authorizations/1"
	e.fetcher.docs[auth] = mustJSON(t, map[string]any{"id": auth, "type": "QuoteAuthorization", "attributedTo": alice, "interactingObject": q.URI, "interactionTarget": open.URI})
	e.expect(t, accept(alice+"#accept", alice, auth), appfederation.InboxHandled)
	if s := e.quoteState(t, q.ID); s.Approval != content.QuoteApprovalAccepted || s.AuthorizationURI != auth {
		t.Errorf("state after the Accept = %+v, want accepted by %s", s, auth)
	}
	var update map[string]any
	for _, a := range e.quoteActivities(t, federation.OutboundStatus, q.ID) {
		if a.Type == federation.ActivityUpdate {
			var doc struct {
				Object map[string]any `json:"object"`
			}
			if err := json.Unmarshal(a.Raw, &doc); err != nil {
				t.Fatal(err)
			}
			update = doc.Object
		}
	}
	if update == nil || update["quoteAuthorization"] != auth {
		t.Errorf("Update = %v, want one naming %s", update, auth)
	}

	// A Reject refuses the quote.
	r, err := e.quotePost(t, fan, open.ID, content.VisibilityUnlisted)
	if err != nil {
		t.Fatal(err)
	}
	reject, err := json.Marshal(map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "id": alice + "#reject", "type": "Reject", "actor": alice,
		"object": map[string]any{"id": r.URI + "/quote_request", "type": "QuoteRequest", "actor": e.localIRI(fan), "object": open.URI, "instrument": r.URI}})
	if err != nil {
		t.Fatal(err)
	}
	e.expect(t, reject, appfederation.InboxHandled)
	if s := e.quoteState(t, r.ID); s.Approval != content.QuoteApprovalRejected {
		t.Errorf("state after the Reject = %+v, want rejected", s)
	}
}

func remoteAnswersQuoteRequests(t *testing.T, e *remoteInteractionEnv) {
	skipWithoutQuoteHandlers(t, e)
	ctx := t.Context()
	author := e.user(t, "author")
	const bob, carol = "https://remote.example/users/bob", "https://other.example/users/carol"
	bobID := e.actor(t, bob)
	e.follow(t, author, bobID)
	e.follow(t, e.actor(t, carol), author) // a follower of the author on another server
	public := e.localPost(t, author, content.VisibilityPublic)
	quoting := bob + "/notes/quote"
	request := func(id, object string) []byte {
		b, err := json.Marshal(map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "id": id, "type": "QuoteRequest", "actor": bob,
			"object": object, "instrument": quoting})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	// The quoting post arrives first, then the QuoteRequest: the Accept names this server's
	// QuoteAuthorization, which approves the stored quote at once.
	e.expect(t, remoteActivity(t, "Create", quoting+"/activity", bob, remoteNote(quoting, bob, remoteNow.Add(-time.Hour), map[string]any{"quote": public.URI})), appfederation.InboxHandled)
	stored := e.post(t, quoting).Status
	e.expect(t, request(bob+"/quote_requests/1", public.URI), appfederation.InboxHandled)
	sent := e.quoteActivities(t, federation.OutboundQuote, public.ID)
	if len(sent) != 1 || sent[0].Type != federation.ActivityAccept || !slices.Equal(sent[0].Inboxes, []string{bob + "/inbox"}) {
		t.Fatalf("answers = %+v, want an Accept to bob", sent)
	}
	result := sent[0].Doc.Result
	if !strings.HasPrefix(string(result), "https://"+contractHost+"/ap/quote_authorizations/") || sent[0].Doc.Object().ID != bob+"/quote_requests/1" {
		t.Errorf("Accept = %s, want the QuoteRequest with this server's QuoteAuthorization", sent[0].Raw)
	}
	if s := e.quoteState(t, stored.ID); s.Approval != content.QuoteApprovalAccepted || s.AuthorizationURI != string(result) {
		t.Errorf("state of bob's quote = %+v, want accepted by %s", s, result)
	}
	if n := e.notified(t, author, bobID, notification.TypeQuote); n != 1 {
		t.Errorf("quote notifications = %d, want 1", n)
	}
	// The same request again gets the same approval.
	e.expect(t, request(bob+"/quote_requests/2", public.URI), appfederation.InboxHandled)
	if sent := e.quoteActivities(t, federation.OutboundQuote, public.ID); len(sent) != 2 || sent[1].Doc.Result != result {
		t.Errorf("answers = %+v, want the same QuoteAuthorization twice", sent)
	}
	// The approval is bob's: it approves no quote of another account.
	approval := string(result)[strings.LastIndexByte(string(result), '/')+1:]
	const dave = "https://remote.example/users/dave"
	daveID := e.actor(t, dave)
	if ok, err := e.interactions.VerifyLocalQuoteAuthorization(ctx, approval, quoting, daveID, public.ID); err != nil || ok {
		t.Errorf("VerifyLocalQuoteAuthorization for another account = %v, %v; want false", ok, err)
	}
	if ok, err := e.interactions.VerifyLocalQuoteAuthorization(ctx, approval, quoting, bobID, public.ID); err != nil || !ok {
		t.Errorf("VerifyLocalQuoteAuthorization for bob = %v, %v; want true", ok, err)
	}
	// A QuoteRequest for a stored post of someone else is refused, and leaves its quote pending.
	bobsOther := bob + "/notes/quote-other"
	e.expect(t, remoteActivity(t, "Create", bobsOther+"/activity", bob, remoteNote(bobsOther, bob, remoteNow.Add(-time.Hour), map[string]any{"quote": public.URI})), appfederation.InboxHandled)
	other := e.post(t, bobsOther).Status
	daveRequest, err := json.Marshal(map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "id": dave + "/quote_requests/1", "type": "QuoteRequest", "actor": dave,
		"object": public.URI, "instrument": bobsOther})
	if err != nil {
		t.Fatal(err)
	}
	e.expect(t, daveRequest, appfederation.InboxHandled)
	if sent := e.quoteActivities(t, federation.OutboundQuote, public.ID); len(sent) != 3 || sent[2].Type != federation.ActivityReject {
		t.Errorf("answers = %+v, want a Reject of dave's request for bob's post", sent)
	}
	if s := e.quoteState(t, other.ID); s.Approval != content.QuoteApprovalPending {
		t.Errorf("state of bob's other quote after dave's request = %+v, want pending", s)
	}

	// A local-only post, and a post of nobody, are refused.
	local := e.localPost(t, author, content.VisibilityLocal)
	e.expect(t, request(bob+"/quote_requests/3", local.URI), appfederation.InboxHandled)
	if sent := e.quoteActivities(t, federation.OutboundQuote, local.ID); len(sent) != 1 || sent[0].Type != federation.ActivityReject {
		t.Errorf("answers about a local-only post = %+v, want a Reject", sent)
	}
	closed := e.localPost(t, author, content.VisibilityPublic)
	if _, err := e.content.SetQuotePolicy(ctx, author, closed.ID, content.QuotePolicyNobody); err != nil {
		t.Fatal(err)
	}
	e.expect(t, request(bob+"/quote_requests/4", closed.URI), appfederation.InboxHandled)
	if sent := e.quoteActivities(t, federation.OutboundQuote, closed.ID); len(sent) != 1 || sent[0].Type != federation.ActivityReject {
		t.Errorf("answers about a post of nobody = %+v, want a Reject", sent)
	}

	// The author revokes the approval: the quote shows the post no more, and bob's server and the
	// author's followers get the Delete of the QuoteAuthorization.
	if err := e.interactions.RevokeQuote(ctx, author, public.ID, stored.ID); err != nil {
		t.Fatalf("RevokeQuote = %v", err)
	}
	if s := e.quoteState(t, stored.ID); s.Approval != content.QuoteApprovalRevoked {
		t.Errorf("state after the revocation = %+v, want revoked", s)
	}
	id := string(result)[strings.LastIndexByte(string(result), '/')+1:]
	deletes := e.quoteActivities(t, federation.OutboundQuote, id)
	if len(deletes) != 1 || deletes[0].Type != federation.ActivityDelete || deletes[0].Doc.Object().ID != result ||
		!slices.Equal(deletes[0].Inboxes, []string{bob + "/inbox", carol + "/inbox"}) {
		t.Errorf("activities after the revocation = %+v, want the Delete of %s to bob and carol", deletes, result)
	}
	if err := e.interactions.RevokeQuote(ctx, author, public.ID, stored.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("RevokeQuote again = %v, want ErrNotFound", err)
	}
	if ok, err := e.interactions.VerifyLocalQuoteAuthorization(ctx, id, quoting, bobID, public.ID); err != nil || ok {
		t.Errorf("VerifyLocalQuoteAuthorization after the revocation = %v, %v; want false", ok, err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
