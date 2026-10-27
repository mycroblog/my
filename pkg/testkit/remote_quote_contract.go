// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
)

// The cases of RemoteStatusContract about the quotes of remote posts (ADR-0069): the quotes of
// the major implementations are recorded, the quoted posts fetched one level, the approvals
// verified and revoked, and the quotes presented to each viewer.

// quotes returns the content service as the RemoteQuotes of the status handlers, nil when the
// harness stores no quotes.
func (e *remoteStatusEnv) quotes() appfederation.RemoteQuotes {
	if e.h.Quotes == nil {
		return nil
	}
	return e.content
}

func skipWithoutQuotes(t *testing.T, e *remoteStatusEnv) {
	t.Helper()
	if e.h.Quotes == nil {
		t.Skip("the harness stores no quotes")
	}
}

// quoteOf returns the quote of the stored post uri and its state.
func (e *remoteStatusEnv) quoteOf(t *testing.T, uri string) (content.Quote, content.QuoteState) {
	t.Helper()
	q, s, err := e.content.FindQuote(t.Context(), e.post(t, uri).Status.ID)
	if err != nil {
		t.Fatalf("FindQuote(%s) = %v", uri, err)
	}
	return q, s
}

// runQuotes runs the pending verifications of quotes, and the fetches they lead to, until none
// is left, and returns how many verifications ran. Every job must complete.
func (fe *remoteFetchEnv) runQuotes(t *testing.T) int {
	t.Helper()
	h, _, ok := fe.handlers.Lookup(appfederation.JobVerifyQuote)
	if !ok {
		t.Fatalf("no handler for %s", appfederation.JobVerifyQuote)
	}
	n := 0
	for {
		jobs := fe.jobs.take(appfederation.JobVerifyQuote)
		if len(jobs) == 0 {
			fe.run(t)
			return n
		}
		for _, j := range jobs {
			n++
			if res := h(t.Context(), queue.Job{Type: j.Type, Payload: j.Payload, IdempotencyKey: j.IdempotencyKey, Attempt: 1}); !res.IsDone() {
				t.Fatalf("%s %s: %v", j.Type, j.Payload, res.Err())
			}
		}
	}
}

func remoteRecordsQuoteFixtures(t *testing.T, e *remoteStatusEnv) {
	skipWithoutQuotes(t, e)
	reader := e.user(t, "reader")
	for _, tc := range []struct {
		implementation, version, name string
		form                          content.QuoteForm
		quoted                        string
		approval                      content.QuoteApproval
	}{
		{"mastodon", "4.7", "create-note-quote", content.QuoteFormFEP044f,
			"https://mastodon.example/ap/users/117454922805771614/statuses/117454925496168340", content.QuoteApprovalPending},
		{"misskey", "circa-2022", "note-quote", content.QuoteFormLegacy,
			"https://remote.example/objects/43479e20-c0f8-4f49-bf7f-13fab8234924", content.QuoteApprovalLegacy},
		{"akkoma", "3.20", "note-quote", content.QuoteFormLegacy, "https://remote.example/status/85717e587f95d5c0", content.QuoteApprovalLegacy},
	} {
		t.Run(tc.implementation+"/"+tc.version+"/"+tc.name, func(t *testing.T) {
			doc, actor, object := fixtureActivity(t, tc.implementation, tc.version, tc.name)
			e.follow(t, reader, e.actor(t, actor))
			e.expect(t, doc, appfederation.InboxHandled)
			q, s := e.quoteOf(t, object)
			if q.QuotedURI != tc.quoted || q.Form != tc.form || s.Approval != tc.approval {
				t.Errorf("quote = %+v in %+v, want %s quoting %s, %s", q, s, tc.form, tc.quoted, tc.approval)
			}
		})
	}
	// A post without a quote has none.
	const alice = "https://remote.example/users/alice"
	e.follower(t, alice)
	plain := alice + "/notes/plain"
	e.expect(t, remoteActivity(t, "Create", plain+"/activity", alice, remoteNote(plain, alice, remoteNow.Add(-time.Hour), nil)), appfederation.InboxHandled)
	if _, _, err := e.content.FindQuote(t.Context(), e.post(t, plain).Status.ID); err == nil {
		t.Errorf("a post without a quote has one")
	}
}

func remoteVerifiesQuotes(t *testing.T, e *remoteStatusEnv) {
	skipWithoutQuotes(t, e)
	fe := newRemoteFetchEnv(t, e)
	const (
		bob   = "https://mastodon.example/ap/users/117454922805771612"
		carol = "https://mastodon.example/ap/users/117454922805771614"
		alice = "https://remote.example/users/alice"
		dave  = "https://other.example/users/dave"
	)
	e.follower(t, bob)
	e.follower(t, alice)
	e.actor(t, carol)
	e.actor(t, dave)
	published := remoteNow.Add(-2 * time.Hour)

	// Mastodon: the quoted post is fetched and the QuoteAuthorization verifies.
	quoted := carol + "/statuses/117454925496168340"
	authorization := carol + "/quote_authorizations/117454925496168360"
	fe.serve(t, quoted, remoteNote(quoted, carol, published, nil))
	fe.serveFixture(t, authorization, "mastodon", "4.7", "quote-authorization")
	doc, _, quote := fixtureActivity(t, "mastodon", "4.7", "create-note-quote")
	e.expect(t, doc, appfederation.InboxHandled)
	if n := fe.runQuotes(t); n != 1 {
		t.Errorf("verifications = %d, want 1", n)
	}
	if !e.stored(t, quoted) {
		t.Errorf("the quoted post %s was not fetched", quoted)
	}
	if _, s := e.quoteOf(t, quote); s.Approval != content.QuoteApprovalAccepted || s.AuthorizationURI != authorization {
		t.Errorf("state of the Mastodon quote = %+v, want accepted by %s", s, authorization)
	}

	// A QuoteAuthorization of another quote, one on another host, and one that is not served
	// leave the quote pending.
	other := carol + "/statuses/2"
	fe.serve(t, other, remoteNote(other, carol, published, nil))
	for i, auth := range []string{authorization, "https://other.example/quote_authorizations/1", carol + "/quote_authorizations/missing"} {
		id := alice + "/notes/unauthorized-" + string(rune('a'+i))
		e.expect(t, remoteActivity(t, "Create", id+"/activity", alice, remoteNote(id, alice, published, map[string]any{
			"quote": other, "quoteAuthorization": auth,
		})), appfederation.InboxHandled)
		fe.runQuotes(t)
		if _, s := e.quoteOf(t, id); s.Approval != content.QuoteApprovalPending {
			t.Errorf("state of a quote with the authorization %s = %+v, want pending", auth, s)
		}
	}

	// A quote of the author's own post needs no QuoteAuthorization.
	own, self := alice+"/notes/own", alice+"/notes/self-quote"
	fe.serve(t, own, remoteNote(own, alice, published, nil))
	e.expect(t, remoteActivity(t, "Create", self+"/activity", alice, remoteNote(self, alice, published, map[string]any{"quote": own})), appfederation.InboxHandled)
	fe.runQuotes(t)
	if _, s := e.quoteOf(t, self); s.Approval != content.QuoteApprovalAccepted || s.AuthorizationURI != "" {
		t.Errorf("state of a quote of the author's own post = %+v, want accepted without an authorization", s)
	}

	// A legacy quote: the quoted post is fetched, but not the post that the quoted post quotes
	// in turn (one level).
	middle, deep := dave+"/notes/middle", dave+"/notes/deep"
	fe.serve(t, middle, remoteNote(middle, dave, published, map[string]any{"_misskey_quote": deep}))
	fe.serve(t, deep, remoteNote(deep, dave, published, nil))
	legacy := alice + "/notes/legacy"
	e.expect(t, remoteActivity(t, "Create", legacy+"/activity", alice, remoteNote(legacy, alice, published, map[string]any{
		"_misskey_quote": middle, "quoteUrl": middle,
	})), appfederation.InboxHandled)
	fe.runQuotes(t)
	if !e.stored(t, middle) {
		t.Errorf("the quoted post %s was not fetched", middle)
	}
	if e.stored(t, deep) || fe.requests(deep) != 0 {
		t.Errorf("the post %s that the quoted post quotes was fetched", deep)
	}
	if q, s := e.quoteOf(t, middle); q.QuotedURI != deep || s.Approval != content.QuoteApprovalLegacy {
		t.Errorf("quote of the fetched post = %+v in %+v, want legacy, quoting %s", q, s, deep)
	}
	if _, s := e.quoteOf(t, legacy); s.Approval != content.QuoteApprovalLegacy {
		t.Errorf("state of the legacy quote = %+v, want legacy", s)
	}
}

func remoteRevokesAndDetachesQuotes(t *testing.T, e *remoteStatusEnv) {
	skipWithoutQuotes(t, e)
	fe := newRemoteFetchEnv(t, e)
	const (
		bob   = "https://mastodon.example/ap/users/117454922805771612"
		carol = "https://mastodon.example/ap/users/117454922805771614"
	)
	e.follower(t, bob)
	e.actor(t, carol)
	quoted := carol + "/statuses/117454925496168340"
	authorization := carol + "/quote_authorizations/117454925496168360"
	fe.serve(t, quoted, remoteNote(quoted, carol, remoteNow.Add(-2*time.Hour), nil))
	fe.serveFixture(t, authorization, "mastodon", "4.7", "quote-authorization")
	doc, _, quote := fixtureActivity(t, "mastodon", "4.7", "create-note-quote")
	e.expect(t, doc, appfederation.InboxHandled)
	fe.runQuotes(t)
	if _, s := e.quoteOf(t, quote); s.Approval != content.QuoteApprovalAccepted {
		t.Fatalf("state = %+v, want accepted", s)
	}

	// Only the quoted author's server revokes the authorization.
	e.actor(t, "https://other.example/users/mallory")
	e.expect(t, remoteActivity(t, "Delete", "https://other.example/users/mallory#delete", "https://other.example/users/mallory", authorization),
		appfederation.InboxDropped)
	if _, s := e.quoteOf(t, quote); s.Approval != content.QuoteApprovalAccepted {
		t.Errorf("state after a Delete from another server = %+v, want accepted", s)
	}
	// Nor does another actor of the quoted author's server.
	const dave = "https://mastodon.example/ap/users/117454922805771616"
	e.actor(t, dave)
	e.expect(t, remoteActivity(t, "Delete", dave+"#delete", dave, authorization), appfederation.InboxTombstoned)
	if _, s := e.quoteOf(t, quote); s.Approval != content.QuoteApprovalAccepted {
		t.Errorf("state after a Delete by another actor of the same server = %+v, want accepted", s)
	}
	e.expect(t, remoteActivity(t, "Delete", authorization+"#delete", carol, authorization), appfederation.InboxHandled)
	if _, s := e.quoteOf(t, quote); s.Approval != content.QuoteApprovalRevoked {
		t.Errorf("state after the Delete of the QuoteAuthorization = %+v, want revoked", s)
	}
	// Delivered again, the Delete changes nothing; the URI of a QuoteAuthorization that approves
	// nothing is an unknown object, as before.
	e.expect(t, remoteActivity(t, "Delete", authorization+"#delete-again", carol, authorization), appfederation.InboxTombstoned)

	// An edit without the quote takes it away for good.
	note := remoteNote(quote, bob, remoteNow.Add(-2*time.Hour), map[string]any{"updated": remoteNow.Add(-time.Hour).Format(time.RFC3339)})
	e.expect(t, remoteActivity(t, "Update", quote+"#update", bob, note), appfederation.InboxHandled)
	if _, s := e.quoteOf(t, quote); s.Approval != content.QuoteApprovalDetached {
		t.Errorf("state after an edit without the quote = %+v, want detached", s)
	}

	// The projection is rebuilt to the same states.
	before, err := e.h.Quotes.ListQuoteStates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := appcontent.RebuildQuoteStates(t.Context(), e.h.UnitOfWork, e.h.Quotes); err != nil {
		t.Fatal(err)
	}
	after, err := e.h.Quotes.ListQuoteStates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || len(after) != len(before) || after[0] != before[0] {
		t.Errorf("rebuilt states = %+v, want %+v", after, before)
	}
}

func remotePresentsQuotes(t *testing.T, e *remoteStatusEnv) {
	skipWithoutQuotes(t, e)
	if e.h.Pins == nil {
		t.Skip("the harness has no interaction repository")
	}
	ctx := t.Context()
	const (
		alice = "https://remote.example/users/alice"
		bob   = "https://other.example/users/bob"
		carol = "https://third.example/users/carol"
	)
	e.follower(t, alice)
	bobID := e.follower(t, bob)
	e.follower(t, carol)
	published := remoteNow.Add(-time.Hour)
	deliver := func(id, author string, fields map[string]any) string {
		t.Helper()
		e.expect(t, remoteActivity(t, "Create", id+"/activity", author, remoteNote(id, author, published, fields)), appfederation.InboxHandled)
		return e.post(t, id).Status.ID
	}
	public := bob + "/notes/public"
	deliver(public, bob, nil)
	private := bob + "/notes/private"
	deliver(private, bob, map[string]any{"to": []string{bob + "/followers"}, "cc": []string{}})
	legacy := deliver(alice+"/notes/legacy", alice, map[string]any{"_misskey_quote": public})
	pending := deliver(alice+"/notes/pending", alice, map[string]any{"quote": public})
	ofPrivate := deliver(alice+"/notes/of-private", alice, map[string]any{"_misskey_quote": private})
	nested := deliver(carol+"/notes/nested", carol, map[string]any{"_misskey_quote": alice + "/notes/legacy"})
	unfetched := deliver(alice+"/notes/unfetched", alice, map[string]any{"_misskey_quote": bob + "/notes/unknown"})

	// Local posts quoted by remote legacy quotes: shown when the quoting author may see them.
	local := e.user(t, "author")
	localPublic, err := e.content.Create(ctx, appcontent.CreateInput{AuthorAccountID: local, TextInput: appcontent.TextInput{Text: "public"}, Visibility: content.VisibilityPublic})
	if err != nil {
		t.Fatal(err)
	}
	localPrivate, err := e.content.Create(ctx, appcontent.CreateInput{AuthorAccountID: local, TextInput: appcontent.TextInput{Text: "private"}, Visibility: content.VisibilityPrivate})
	if err != nil {
		t.Fatal(err)
	}
	ofLocalPublic := deliver(alice+"/notes/of-local-public", alice, map[string]any{"_misskey_quote": localPublic.Status.URI})
	ofLocalPrivate := deliver(alice+"/notes/of-local-private", alice, map[string]any{"_misskey_quote": localPrivate.Status.URI})

	reader := appcontent.NewReader(appcontent.ReaderDeps{
		Statuses: e.h.Statuses, Interactions: e.h.Pins, Accounts: e.accounts, Relationships: e.graph,
		Quotes: e.h.Quotes, DomainMutes: e.graph, Clock: e.clock,
	})
	viewer := func(name string) appcontent.Viewer {
		return appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: e.user(t, name)}
	}
	check := func(what string, v appcontent.Viewer, statusID string, want appcontent.QuoteViewState, shown bool) {
		t.Helper()
		got, err := reader.View(ctx, v, statusID)
		if err != nil {
			t.Fatalf("%s: View = %v", what, err)
		}
		switch {
		case got.Quote == nil:
			t.Errorf("%s: no quote, want %s", what, want)
		case got.Quote.State != want || (got.Quote.Status != nil) != shown:
			t.Errorf("%s: quote %s with the status %v, want %s with it %v", what, got.Quote.State, got.Quote.Status != nil, want, shown)
		case shown && got.Quote.Status.Quote != nil:
			t.Errorf("%s: the quoted status has a quote of its own; quotes are one level deep", what)
		}
	}
	plain := viewer("plain")
	check("legacy", plain, legacy, appcontent.QuoteViewLegacy, true)
	check("anonymous", appcontent.Viewer{}, legacy, appcontent.QuoteViewLegacy, true)
	check("pending", plain, pending, appcontent.QuoteViewPending, false)
	check("not fetched", plain, unfetched, appcontent.QuoteViewPending, false)
	check("a private post of someone the viewer does not follow", plain, ofPrivate, appcontent.QuoteViewUnauthorized, false)
	check("a quote of a quote", plain, nested, appcontent.QuoteViewLegacy, true)
	check("a public local post", plain, ofLocalPublic, appcontent.QuoteViewLegacy, true)
	check("a local post the quoting author may not see", plain, ofLocalPrivate, appcontent.QuoteViewRejected, false)

	muting := viewer("muting")
	if _, err := e.graph.Mute(ctx, muting.AccountID, bobID, appgraph.MuteInput{}); err != nil {
		t.Fatal(err)
	}
	check("muted author", muting, legacy, appcontent.QuoteViewMutedAccount, true)
	domainMuting := viewer("domain_muting")
	if _, err := e.graph.MuteDomain(ctx, domainMuting.AccountID, federation.IRI(bob).Host()); err != nil {
		t.Fatal(err)
	}
	check("muted domain", domainMuting, legacy, appcontent.QuoteViewBlockedDomain, true)
	blocking := viewer("blocking")
	if _, err := e.graph.Block(ctx, blocking.AccountID, bobID); err != nil {
		t.Fatal(err)
	}
	check("blocked author", blocking, legacy, appcontent.QuoteViewBlockedAccount, false)

	// A quote that its author takes away is not shown; a deleted quoted post is shown as such.
	e.expect(t, remoteActivity(t, "Delete", public+"#delete", bob, public), appfederation.InboxHandled)
	check("deleted", plain, legacy, appcontent.QuoteViewDeleted, false)
	edited := remoteNote(alice+"/notes/pending", alice, published, map[string]any{"updated": remoteNow.Format(time.RFC3339)})
	e.expect(t, remoteActivity(t, "Update", alice+"/notes/pending#update", alice, edited), appfederation.InboxHandled)
	if got, err := reader.View(ctx, plain, pending); err != nil || got.Quote != nil {
		t.Errorf("a quote taken away = %+v, %v; want none", got.Quote, err)
	}
}
