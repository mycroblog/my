// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/queue"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/pkg/testkit"
)

// The cases of RemoteStatusFlows that fetch the recent posts of a remote profile that a user
// opens (federation.md §6.3, ADR-0006): StatusHandlers.FetchOutbox and federation.fetch_outbox
// through the real Fetcher over an in-memory network.

// remoteSuspensions is an AccountPolicy that suspends the accounts set in it.
type remoteSuspensions map[string]bool

func (s remoteSuspensions) AccountFederationSuspended(_ context.Context, accountID string) (bool, error) {
	return s[accountID], nil
}

// outboxActor stores the remote actor uri with the outbox outbox ("" for none) and returns its
// account ID.
func (fe *remoteFetchEnv) outboxActor(t *testing.T, uri, outbox string) string {
	t.Helper()
	iri := federation.IRI(uri)
	a, err := fe.remote.SaveRemoteActor(t.Context(), appaccount.RemoteActorInput{
		URI: uri, Domain: iri.Host(), Type: "Person", PreferredUsername: uri[strings.LastIndexByte(uri, '/')+1:],
		InboxURL: uri + "/inbox", OutboxURL: outbox, FollowersURL: uri + "/followers", Document: []byte(`{"id":"` + uri + `","outbox":"` + outbox + `"}`),
	})
	if err != nil {
		t.Fatalf("SaveRemoteActor(%s): %v", uri, err)
	}
	return a.Account.ID
}

// serveFixture answers GET uri with a federation fixture, its http IRIs made https.
func (fe *remoteFetchEnv) serveFixture(t *testing.T, uri, implementation, version, name string) {
	t.Helper()
	fixtures, err := testkit.FederationFixtures()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(fixtures, func(f testkit.FederationFixture) bool {
		return f.Implementation == implementation && f.Version == version && f.Name == name
	})
	if i < 0 {
		t.Fatalf("no fixture %s/%s/%s", implementation, version, name)
	}
	body := strings.ReplaceAll(string(fixtures[i].JSON), `"http://`, `"https://`)
	fe.net.set(uri, remoteResponse{status: http.StatusOK, contentType: activityJSON, body: body})
}

// runOutboxes runs the pending outbox fetches and returns how many ran. Every one must complete.
func (fe *remoteFetchEnv) runOutboxes(t *testing.T) int {
	t.Helper()
	h, _, ok := fe.handlers.Lookup(appfederation.JobFetchOutbox)
	if !ok {
		t.Fatalf("no handler for %s", appfederation.JobFetchOutbox)
	}
	jobs := fe.jobs.take(appfederation.JobFetchOutbox)
	for _, j := range jobs {
		if res := h(t.Context(), queue.Job{Type: j.Type, Payload: j.Payload, IdempotencyKey: j.IdempotencyKey, Attempt: 1}); !res.IsDone() {
			t.Fatalf("%s %s: %v", j.Type, j.Payload, res.Err())
		}
	}
	return len(jobs)
}

// fetchOutbox opens the profile of accountID as viewer and runs the outbox fetch it starts,
// failing unless it starts one.
func (fe *remoteFetchEnv) fetchOutbox(t *testing.T, viewer, accountID string) {
	t.Helper()
	started, err := fe.statuses.FetchOutbox(t.Context(), viewer, accountID)
	if err != nil || !started {
		t.Fatalf("FetchOutbox(%s, %s) = %v, %v; want a fetch started", viewer, accountID, started, err)
	}
	if n := fe.runOutboxes(t); n != 1 {
		t.Fatalf("outbox fetches = %d, want 1", n)
	}
	if n := len(fe.jobs.take(appfederation.JobFetchObject)); n != 0 {
		t.Errorf("the outbox fetch enqueued %d fetches of posts, want none (no parents)", n)
	}
}

// profile returns the IDs of the statuses that the profile timeline of accountID lists to viewer.
func (fe *remoteFetchEnv) profile(t *testing.T, viewer, accountID string) []string {
	t.Helper()
	p, err := fe.timeline.Profile(t.Context(), appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: viewer}, accountID,
		apptimeline.ProfileFilter{}, apptimeline.Query{Limit: apptimeline.MaxLimit})
	if err != nil {
		t.Fatalf("Profile(%s): %v", accountID, err)
	}
	ids := make([]string, 0, len(p.Entries))
	for _, e := range p.Entries {
		ids = append(ids, e.StatusID)
	}
	return ids
}

func remoteFetchesOutboxFixtures(t *testing.T, e *remoteStatusEnv) {
	fe := newRemoteFetchEnv(t, e)
	viewer := fe.user(t, "viewer")
	for _, tc := range []struct {
		implementation, version string
		actor, page             string
		// posts are the posts that the page lists, embedded or not; served are the documents
		// served at the IRIs of those that are not embedded.
		posts, served []string
		// left are the IRIs that must not be fetched: the objects of boosts and the parents of
		// replies.
		left []string
	}{
		{
			implementation: "mastodon", version: "4.7",
			actor: "https://mastodon.example/ap/users/117454922805771612",
			page:  "https://mastodon.example/ap/users/117454922805771612/outbox?page=true",
			posts: []string{
				"https://mastodon.example/ap/users/117454922805771612/statuses/117454936765897454",
				"https://mastodon.example/ap/users/117454922805771612/statuses/117454927899945619",
				"https://mastodon.example/ap/users/117454922805771612/statuses/117454925496168351",
			},
			left: []string{
				"https://remote.example/ap/objects/01a148a2-b24c-73b5-b159-fdfd429294e4",
				"https://remote.example/ap/objects/01a148a2-753a-7b02-b9bc-71380ccdfdd0",
			},
		},
		{
			implementation: "misskey", version: "2026.10",
			actor: "https://misskey.example/users/asfo7no84zv70002",
			page:  "https://misskey.example/users/asfo7no84zv70002/outbox?page=true",
			posts: []string{
				"https://misskey.example/notes/asfoesv84zv7001s",
				"https://misskey.example/notes/asfo9e964zv7000k",
				"https://misskey.example/notes/asfo829i4zv7000d",
			},
			left: []string{
				"https://remote.example/ap/objects/01a148e1-c3b2-76cf-b3a7-7c073768309f",
				"https://remote.example/ap/objects/01a148e1-502c-7f77-b5ca-5cef75200a1a",
			},
		},
		{
			implementation: "gotosocial", version: "0.22",
			actor: "https://gotosocial.example/users/the_mighty_zork",
			page:  "https://gotosocial.example/users/the_mighty_zork/outbox?limit=40",
			posts: []string{
				"https://gotosocial.example/users/the_mighty_zork/statuses/01JDPZC707CKDN8N4QVWM4Z1NR",
				"https://gotosocial.example/users/the_mighty_zork/statuses/01HH9KYNQPA416TNJ53NSATP40",
				"https://gotosocial.example/users/the_mighty_zork/statuses/01F8MHAMCHF6Y650WCRSCP4WMY",
			},
			served: []string{
				"https://gotosocial.example/users/the_mighty_zork/statuses/01JDPZC707CKDN8N4QVWM4Z1NR",
				"https://gotosocial.example/users/the_mighty_zork/statuses/01HH9KYNQPA416TNJ53NSATP40",
				"https://gotosocial.example/users/the_mighty_zork/statuses/01F8MHAMCHF6Y650WCRSCP4WMY",
			},
		},
	} {
		t.Run(tc.implementation+"/"+tc.version, func(t *testing.T) {
			outbox := tc.actor + "/outbox"
			fe.serveFixture(t, outbox, tc.implementation, tc.version, "outbox")
			fe.serveFixture(t, tc.page, tc.implementation, tc.version, "outbox-page")
			for i, uri := range tc.served {
				fe.serve(t, uri, remoteNote(uri, tc.actor, remoteNow.Add(-time.Duration(i+1)*time.Hour), nil))
			}
			account := fe.outboxActor(t, tc.actor, outbox)
			fe.fetchOutbox(t, viewer, account)

			var want []string
			for _, uri := range tc.posts {
				p := fe.post(t, uri)
				if p.Status.AuthorAccountID != account || p.Status.IsLocal || p.Revision.Remote == nil || p.Revision.Remote.ActivityURI != "" {
					t.Errorf("%s = %+v, want a fetched post of the account", uri, p.Status)
				}
				want = append(want, p.Status.ID)
			}
			for _, uri := range tc.left {
				if fe.net.count(uri) != 0 || fe.stored(t, uri) {
					t.Errorf("%s was fetched: boosts and parents are not", uri)
				}
			}
			// The posts are on the profile, newest first.
			if got := fe.profile(t, viewer, account); !slices.Equal(got, want) {
				t.Errorf("profile = %v, want %v", got, want)
			}
		})
	}
}

func remoteFetchesOutboxesWithinBounds(t *testing.T, e *remoteStatusEnv) {
	fe := newRemoteFetchEnv(t, e)
	const bob, carol = "https://other.example/users/bob", "https://third.example/users/carol"
	outbox, page := bob+"/outbox", bob+"/outbox?page=1"
	note := func(n int) string { return fmt.Sprintf("%s/notes/%d", bob, n) }
	create := func(object any, published time.Time, to ...string) map[string]any {
		id := fmt.Sprint(object)
		if m, ok := object.(map[string]any); ok {
			id = fmt.Sprint(m["id"])
		}
		return map[string]any{
			"id": id + "/activity", "type": "Create", "actor": bob, "published": published.Format(time.RFC3339),
			"to": to, "cc": []string{bob + "/followers"}, "object": object,
		}
	}
	public := string(federation.PublicIRI)
	newest := remoteNow.Add(-time.Minute)
	items := []any{
		// A boost, a followers-only post, a post of another origin, and an embedded post of
		// someone else are not taken.
		map[string]any{"id": bob + "/boosts/1", "type": "Announce", "actor": bob, "published": newest.Format(time.RFC3339), "to": []string{public}, "object": carol + "/notes/1"},
		create(bob+"/notes/private", newest, bob+"/followers"),
		create(carol+"/notes/2", newest, public),
		create(remoteNote(bob+"/notes/carols", carol, newest, nil), newest, public),
	}
	const posts = appfederation.OutboxMaxPosts + 5
	for n := range posts {
		published := remoteNow.Add(-time.Duration(n+1) * time.Hour)
		fields := map[string]any{}
		if n == 0 {
			fields["inReplyTo"] = carol + "/notes/parent"
		}
		fe.serve(t, note(n), remoteNote(note(n), bob, published, fields))
		items = append(items, create(note(n), published, public))
	}
	// The page lists the oldest first: the published times order it.
	slices.Reverse(items[4:])
	fe.serve(t, outbox, map[string]any{"id": outbox, "type": "OrderedCollection", "totalItems": posts, "first": page})
	fe.serve(t, page, map[string]any{"id": page, "type": "OrderedCollectionPage", "partOf": outbox, "orderedItems": items, "next": bob + "/outbox?page=2"})
	fe.serve(t, bob+"/outbox?page=2", map[string]any{"id": bob + "/outbox?page=2", "type": "OrderedCollectionPage", "orderedItems": []any{}})
	fe.serve(t, bob+"/notes/private", remoteNote(bob+"/notes/private", bob, newest, map[string]any{"to": []string{bob + "/followers"}}))
	account := fe.outboxActor(t, bob, outbox)
	fe.actor(t, carol)
	// One of the newest posts is stored already.
	if _, err := fe.resolver.Resolve(t.Context(), note(1)); err != nil {
		t.Fatalf("Resolve(%s): %v", note(1), err)
	}
	before := fe.requests("https://")
	fe.fetchOutbox(t, fe.user(t, "viewer"), account)

	if n := fe.requests("https://") - before; n > appfederation.OutboxMaxDocuments {
		t.Errorf("the outbox fetch made %d requests, more than %d", n, appfederation.OutboxMaxDocuments)
	}
	if fe.net.count(note(1)) != 1 {
		t.Errorf("the stored post was fetched again")
	}
	for n := range posts {
		if want := n < appfederation.OutboxMaxPosts; fe.stored(t, note(n)) != want {
			t.Errorf("post %d stored = %v, want %v: the newest %d", n, !want, want, appfederation.OutboxMaxPosts)
		}
	}
	for _, uri := range []string{carol + "/notes/1", carol + "/notes/2", carol + "/notes/parent", bob + "/notes/private", bob + "/outbox?page=2"} {
		if fe.net.count(uri) != 0 {
			t.Errorf("%s was fetched", uri)
		}
	}
	for _, uri := range []string{bob + "/notes/private", bob + "/notes/carols", carol + "/notes/2"} {
		if fe.stored(t, uri) {
			t.Errorf("%s was stored", uri)
		}
	}

	// A failing outbox ends the job.
	const dave = "https://fourth.example/users/dave"
	fe.net.set(dave+"/outbox", remoteResponse{status: http.StatusServiceUnavailable})
	fe.fetchOutbox(t, fe.user(t, "other"), fe.outboxActor(t, dave, dave+"/outbox"))
}

func remoteStartsOutboxFetches(t *testing.T, e *remoteStatusEnv) {
	fe := newRemoteFetchEnv(t, e)
	ctx := t.Context()
	viewer := fe.user(t, "viewer")
	start := func(viewer, account string) bool {
		t.Helper()
		started, err := fe.statuses.FetchOutbox(ctx, viewer, account)
		if err != nil {
			t.Fatalf("FetchOutbox(%s, %s): %v", viewer, account, err)
		}
		jobs := len(fe.jobs.take(appfederation.JobFetchOutbox))
		if (jobs == 1) != started || jobs > 1 {
			t.Fatalf("FetchOutbox(%s, %s) = %v and enqueued %d jobs", viewer, account, started, jobs)
		}
		return started
	}
	actor := func(name string) string {
		uri := "https://other.example/users/" + name
		return fe.outboxActor(t, uri, uri+"/outbox")
	}

	// An active remote account with an outbox is fetched once in OutboxFetchTTL.
	bob := actor("bob")
	if !start(viewer, bob) {
		t.Fatal("the first opening did not start a fetch")
	}
	if start(viewer, bob) || start(fe.user(t, "another"), bob) {
		t.Error("an outbox was fetched again within OutboxFetchTTL")
	}
	fe.Clock.Advance(appfederation.OutboxFetchTTL)
	if !start(viewer, bob) {
		t.Error("the outbox was not fetched again after OutboxFetchTTL")
	}

	// No outbox, an outbox of another origin, a local account, an anonymous viewer, and the
	// viewer's own account start nothing.
	none := fe.outboxActor(t, "https://other.example/users/none", "")
	foreign := fe.outboxActor(t, "https://other.example/users/foreign", "https://third.example/outbox")
	for _, account := range []string{none, foreign, fe.user(t, "local"), viewer} {
		if start(viewer, account) {
			t.Errorf("FetchOutbox(%s) started a fetch", account)
		}
	}
	if start("", actor("anonymous")) {
		t.Error("an anonymous viewer started a fetch")
	}

	// Neither a suspended account, a deleted one, nor blocks in either direction are fetched.
	suspended, deleted, blocked, blocking := actor("suspended"), actor("deleted"), actor("blocked"), actor("blocking")
	fe.suspended[suspended] = true
	if err := fe.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return fe.remote.MarkRemoteDeleted(ctx, tx, deleted) }); err != nil {
		t.Fatal(err)
	}
	if _, err := fe.graph.Block(ctx, viewer, blocked); err != nil {
		t.Fatal(err)
	}
	if err := fe.graph.ReceiveBlock(ctx, appgraph.RemoteBlockInput{URI: "https://other.example/blocks/1", BlockerAccountID: blocking, BlockedAccountID: viewer}); err != nil {
		t.Fatal(err)
	}
	for _, account := range []string{suspended, deleted, blocked, blocking} {
		if start(viewer, account) {
			t.Errorf("FetchOutbox(%s) started a fetch", account)
		}
	}
	// They are left for when the restriction ends.
	delete(fe.suspended, suspended)
	if !start(viewer, suspended) {
		t.Error("the account was not fetched after its restriction ended")
	}

	// A viewer starts at most OutboxUserLimit fetches in OutboxUserWindow; the account over the
	// limit is left for another viewer.
	fe.Clock.Advance(appfederation.OutboxUserWindow)
	busy := fe.user(t, "busy")
	for i := range appfederation.OutboxUserLimit {
		if !start(busy, actor(fmt.Sprint("many", i))) {
			t.Fatalf("fetch %d was not started", i)
		}
	}
	over := actor("over")
	if start(busy, over) {
		t.Error("a fetch over OutboxUserLimit was started")
	}
	if !start(viewer, over) {
		t.Error("another viewer did not start the fetch that the busy viewer could not")
	}
	fe.Clock.Advance(appfederation.OutboxUserWindow)
	if !start(busy, actor("later")) {
		t.Error("the busy viewer started no fetch after OutboxUserWindow")
	}
}
