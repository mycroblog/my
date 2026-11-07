// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

// The cases of RemoteStatusFlows that fetch remote posts (federation.md §6.1, §6.3): the real
// Fetcher, RemoteActors, StatusHandlers, and Resolver over an in-memory network.

// remoteFetchEnv is a remoteStatusEnv whose inbox processor and status handlers fetch through
// the real Fetcher from net, and whose jobs wait in jobs until run.
type remoteFetchEnv struct {
	*remoteStatusEnv
	net      *remoteNet
	jobs     *remoteJobs
	handlers *queue.JobHandlerRegistry
	statuses *appfederation.StatusHandlers
	resolver *appfederation.Resolver
	// remoteActors fetch and refresh remote actors, which enqueues the fetches of their
	// featured collections.
	remoteActors *appfederation.RemoteActors
	// suspended are the accounts with the federation restriction.
	suspended remoteSuspensions
}

func newRemoteFetchEnv(t *testing.T, e *remoteStatusEnv) *remoteFetchEnv {
	t.Helper()
	if e.h.Federation == nil || e.h.Instances == nil {
		t.Skip("the harness has no federation repositories")
	}
	fe := &remoteFetchEnv{
		remoteStatusEnv: e, net: &remoteNet{routes: map[string]remoteResponse{}},
		jobs: &remoteJobs{seen: map[string]bool{}}, handlers: &queue.JobHandlerRegistry{}, suspended: remoteSuspensions{},
	}
	kv := testkit.NewKeyValueStore(e.Clock)
	fetcher := appfederation.NewFetcher(appfederation.FetcherDeps{
		Client: appfederation.NewSignedHTTPClient(fe.net, remoteSigner{}, kv, e.Clock), KV: kv, Clock: e.Clock,
		Domains: appfederation.NoDomainRestrictions{}, Instances: e.h.Federation, Host: testkit.ContractHost,
		Key: func(context.Context) (appfederation.SigningKey, error) {
			k, err := identitytest.CachedKey(account.KeyRSASHA256)
			return appfederation.SigningKey{KeyID: "https://" + testkit.ContractHost + "/ap/actor#main-key", PrivateKey: k}, err
		},
	})
	outbox := queue.AfterCommit(e.projectors, fe.jobs, nil)
	actors := appfederation.NewRemoteActors(appfederation.RemoteActorsDeps{
		UnitOfWork: e.h.UnitOfWork, Accounts: e.remote, Instances: e.h.Instances, Fetcher: fetcher, HTTP: fe.net,
		Jobs: fe.jobs, Outbox: outbox, Followers: e.graph, Clock: e.Clock, IDs: e.IDs,
	})
	e.processor = appfederation.NewInboxProcessor(appfederation.InboxProcessorDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Inbox, Actors: actors, Fetcher: actors, Clock: e.Clock,
	})
	fe.remoteActors = actors
	var pins appfederation.RemotePins
	if e.h.Pins != nil {
		pins = appcontent.NewInteractions(appcontent.InteractionDeps{
			UnitOfWork: e.h.UnitOfWork, Statuses: e.h.Statuses, Interactions: e.h.Pins, Accounts: e.Accounts,
			Relationships: e.graph, Clock: e.Clock, IDs: e.IDs, Host: testkit.ContractHost,
		})
	}
	fe.statuses = appfederation.NewStatusHandlers(appfederation.StatusHandlersDeps{
		UnitOfWork: e.h.UnitOfWork, Statuses: e.content, Accounts: e.remote, Actors: actors, Fetcher: actors,
		Followers: e.graph, Tombstones: e.h.Inbox, Outbox: outbox, Jobs: fe.jobs, KV: kv, Clock: e.Clock, Host: testkit.ContractHost,
		Profiles: e.remote, Relationships: e.graph, Restrictions: fe.suspended, Pins: pins, Quotes: e.quotes(),
	})
	fe.statuses.Register(e.processor)
	fe.statuses.RegisterJobs(fe.handlers)
	fe.resolver = appfederation.NewResolver(actors, fe.statuses)
	return fe
}

// run runs the pending fetch jobs, and the ones they enqueue, until none is left, and returns
// how many ran. Every one must complete.
func (fe *remoteFetchEnv) run(t *testing.T) int {
	t.Helper()
	h, _, ok := fe.handlers.Lookup(appfederation.JobFetchObject)
	if !ok {
		t.Fatalf("no handler for %s", appfederation.JobFetchObject)
	}
	n := 0
	for {
		jobs := fe.jobs.take(appfederation.JobFetchObject)
		if len(jobs) == 0 {
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

// serve answers GET uri with doc as ActivityStreams.
func (fe *remoteFetchEnv) serve(t *testing.T, uri string, doc map[string]any) {
	t.Helper()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	fe.net.set(uri, remoteResponse{status: http.StatusOK, contentType: activityJSON, body: string(b)})
}

// requests counts the requests to URLs that start with prefix.
func (fe *remoteFetchEnv) requests(prefix string) int {
	fe.net.mu.Lock()
	defer fe.net.mu.Unlock()
	n := 0
	for _, r := range fe.net.requests {
		if strings.HasPrefix(r.URL, prefix) {
			n++
		}
	}
	return n
}

// reply delivers a Create of a public reply by author to parent, from an author that a local
// account follows, and returns its URI.
func (fe *remoteFetchEnv) reply(t *testing.T, author, id, parent string) string {
	t.Helper()
	fe.expect(t, remoteActivity(t, "Create", id+"/activity", author, remoteNote(id, author, remoteNow.Add(-time.Hour), map[string]any{
		"inReplyTo": parent,
	})), appfederation.InboxHandled)
	return id
}

func remoteFetchesParents(t *testing.T, e *remoteStatusEnv) {
	fe := newRemoteFetchEnv(t, e)
	const alice, bob = "https://remote.example/users/alice", "https://other.example/users/bob"
	e.follower(t, alice)
	bobID := e.actor(t, bob)
	grandparent, parent := bob+"/notes/1", bob+"/notes/2"
	published := remoteNow.Add(-2 * time.Hour)
	fe.serve(t, grandparent, remoteNote(grandparent, bob, published, nil))
	fe.serve(t, parent, remoteNote(parent, bob, published, map[string]any{"inReplyTo": grandparent, "content": "<p>parent</p>"}))

	reply := fe.reply(t, alice, alice+"/notes/1", parent)
	if n := fe.run(t); n != 1 {
		t.Errorf("fetch jobs = %d, want 1: the parent only", n)
	}
	p := e.post(t, parent)
	if p.Status.AuthorAccountID != bobID || p.Revision.Bodies[0].ContentHTML != "<p>parent</p>" || p.Revision.Remote == nil || p.Revision.Remote.ActivityURI != "" {
		t.Errorf("fetched parent = %+v, want bob's post from a fetch (no activity)", p)
	}
	if e.stored(t, grandparent) || fe.requests(grandparent) != 0 {
		t.Errorf("the grandparent was fetched: one level only")
	}
	// The parent fetched after its reply links to it.
	if c, err := e.h.Statuses.FindCounters(t.Context(), p.Status.ID); err != nil || c.RepliesCount != 1 {
		t.Errorf("counters of the parent = %+v, %v; want the reply %s counted", c, err, reply)
	}
	// A reply to a stored post, and a reply to a local post, fetch nothing.
	fe.reply(t, alice, alice+"/notes/2", parent)
	mine, err := e.content.Create(t.Context(), appcontent.CreateInput{
		AuthorAccountID: e.user(t, "local"), TextInput: appcontent.TextInput{Text: "local"}, Visibility: content.VisibilityPublic,
	})
	if err != nil {
		t.Fatal(err)
	}
	fe.reply(t, alice, alice+"/notes/3", mine.Status.URI)
	if n := fe.run(t); n != 0 {
		t.Errorf("fetch jobs for replies to known posts = %d, want 0", n)
	}
}

func remoteRejectsForeignAttribution(t *testing.T, e *remoteStatusEnv) {
	fe := newRemoteFetchEnv(t, e)
	const alice, bob, carol = "https://remote.example/users/alice", "https://other.example/users/bob", "https://third.example/users/carol"
	e.follower(t, alice)
	e.actor(t, bob)
	e.actor(t, carol)
	published := remoteNow.Add(-2 * time.Hour)
	foreign, unattributed, notPost := bob+"/notes/foreign", bob+"/notes/unattributed", bob+"/collections/1"
	fe.serve(t, foreign, remoteNote(foreign, carol, published, nil))
	note := remoteNote(unattributed, bob, published, nil)
	delete(note, "attributedTo")
	fe.serve(t, unattributed, note)
	fe.serve(t, notPost, map[string]any{"id": notPost, "type": "OrderedCollection"})
	for i, parent := range []string{foreign, unattributed, notPost} {
		fe.reply(t, alice, fmt.Sprintf("%s/notes/r%d", alice, i), parent)
	}
	fe.run(t)
	for _, uri := range []string{foreign, unattributed, notPost} {
		if e.stored(t, uri) {
			t.Errorf("%s was stored", uri)
		}
		if _, err := fe.resolver.Resolve(t.Context(), uri); !errors.Is(err, appfederation.ErrUnresolvable) {
			t.Errorf("Resolve(%s) = %v, want ErrUnresolvable", uri, err)
		}
	}
}

func remoteFetchesThreadsWithinBounds(t *testing.T, e *remoteStatusEnv) {
	fe := newRemoteFetchEnv(t, e)
	const bob = "https://other.example/users/bob"
	e.follower(t, bob)
	published := remoteNow.Add(-3 * time.Hour)
	note := func(n int) string { return fmt.Sprintf("%s/notes/%d", bob, n) }

	// Ancestors: a chain of 30 posts, of which the last is delivered and its parent fetched on
	// receipt.
	const chain = 30
	for i := range chain - 1 {
		f := map[string]any{}
		if i > 0 {
			f["inReplyTo"] = note(i - 1)
		}
		fe.serve(t, note(i), remoteNote(note(i), bob, published, f))
	}
	// Replies: the last post lists 150 replies on pages of 50.
	last := note(chain - 1)
	const replies = 150
	pages := []string{last + "/replies?page=1", last + "/replies?page=2", last + "/replies?page=3"}
	for p, page := range pages {
		var items []string
		for i := range 50 {
			r := fmt.Sprintf("%s/notes/reply-%d", bob, p*50+i)
			items = append(items, r)
			fe.serve(t, r, remoteNote(r, bob, published, map[string]any{"inReplyTo": last}))
		}
		doc := map[string]any{"id": page, "type": "OrderedCollectionPage", "orderedItems": items}
		if p+1 < len(pages) {
			doc["next"] = pages[p+1]
		}
		fe.serve(t, page, doc)
	}
	fe.serve(t, last+"/replies", map[string]any{"id": last + "/replies", "type": "OrderedCollection", "first": pages[0]})
	lastDoc := remoteNote(last, bob, published, map[string]any{"inReplyTo": note(chain - 2), "replies": last + "/replies"})
	fe.serve(t, last, lastDoc)
	fe.expect(t, remoteActivity(t, "Create", last+"/activity", bob, lastDoc), appfederation.InboxHandled)
	fe.run(t)
	if !e.stored(t, note(chain-2)) || e.stored(t, note(chain-3)) {
		t.Fatalf("on receipt, the parent and only the parent was fetched")
	}

	before := fe.requests(bob)
	st := e.post(t, last).Status
	if err := fe.statuses.FetchThread(t.Context(), st.ID); err != nil {
		t.Fatalf("FetchThread: %v", err)
	}
	fe.run(t)
	// ThreadDepth levels of ancestors, counting the one fetched on receipt.
	for i := range chain - 1 {
		level := chain - 1 - i
		if want := level <= appfederation.ThreadDepth; e.stored(t, note(i)) != want {
			t.Errorf("ancestor %d levels up stored = %v, want %v", level, !want, want)
		}
	}
	fetched := fe.requests(bob) - before
	if fetched > appfederation.ThreadMaxItems {
		t.Errorf("the thread fetched %d documents, more than %d", fetched, appfederation.ThreadMaxItems)
	}
	stored := 0
	for i := range replies {
		if e.stored(t, fmt.Sprintf("%s/notes/reply-%d", bob, i)) {
			stored++
		}
	}
	if stored == 0 || stored >= replies {
		t.Errorf("stored replies = %d, want some, bounded by %d documents", stored, appfederation.ThreadMaxItems)
	}

	// Opened again within ThreadFetchTTL, the thread is not fetched again.
	if err := fe.statuses.FetchThread(t.Context(), st.ID); err != nil {
		t.Fatalf("FetchThread again: %v", err)
	}
	if n := fe.run(t); n != 0 || fe.requests(bob)-before != fetched {
		t.Errorf("the thread opened again ran %d jobs", n)
	}

	// Replies: ThreadDepth levels below the opened post, each listing the next in an embedded
	// collection.
	const deep = appfederation.ThreadDepth + 5
	reply := func(n int) string { return fmt.Sprintf("%s/deep/%d", bob, n) }
	for i := range deep {
		f := map[string]any{"replies": map[string]any{
			"id": reply(i) + "/replies", "type": "Collection",
			"first": map[string]any{"type": "CollectionPage", "items": []string{reply(i + 1)}},
		}}
		if i > 0 {
			f["inReplyTo"] = reply(i - 1)
		}
		fe.serve(t, reply(i), remoteNote(reply(i), bob, published, f))
	}
	root, err := fe.resolver.Resolve(t.Context(), reply(0))
	if err != nil || root.Status == nil {
		t.Fatalf("Resolve(%s) = %+v, %v", reply(0), root, err)
	}
	if err := fe.statuses.FetchThread(t.Context(), root.Status.ID); err != nil {
		t.Fatalf("FetchThread: %v", err)
	}
	fe.run(t)
	for i := 1; i < deep; i++ {
		if want := i <= appfederation.ThreadDepth; e.stored(t, reply(i)) != want {
			t.Errorf("reply %d levels down stored = %v, want %v", i, !want, want)
		}
	}
}

func remoteRemembersFailedFetches(t *testing.T, e *remoteStatusEnv) {
	fe := newRemoteFetchEnv(t, e)
	const alice, bob = "https://remote.example/users/alice", "https://other.example/users/bob"
	e.follower(t, alice)
	e.actor(t, bob)
	failing, missing := bob+"/notes/failing", bob+"/notes/missing"
	fe.net.set(failing, remoteResponse{status: http.StatusServiceUnavailable})
	// The first reply fetches each parent once.
	first := fe.reply(t, alice, alice+"/notes/1", failing)
	fe.reply(t, alice, alice+"/notes/2", missing)
	fe.run(t)
	if fe.requests(failing) != 1 || fe.requests(missing) != 1 {
		t.Fatalf("requests = %d, %d; want one each", fe.requests(failing), fe.requests(missing))
	}
	// Later replies, and opening the thread, do not fetch them again within FetchFailureTTL.
	fe.reply(t, alice, alice+"/notes/3", failing)
	fe.reply(t, alice, alice+"/notes/4", missing)
	if err := fe.statuses.FetchThread(t.Context(), e.post(t, first).Status.ID); err != nil {
		t.Fatalf("FetchThread: %v", err)
	}
	fe.run(t)
	if fe.requests(failing) != 1 || fe.requests(missing) != 1 {
		t.Errorf("requests after the failures = %d, %d; want no new ones", fe.requests(failing), fe.requests(missing))
	}
	// After FetchFailureTTL, the parent is fetched again and stored.
	e.Clock.Advance(appfederation.FetchFailureTTL)
	fe.serve(t, failing, remoteNote(failing, bob, remoteNow.Add(-2*time.Hour), nil))
	fe.reply(t, alice, alice+"/notes/5", failing)
	fe.run(t)
	if fe.requests(failing) != 2 || !e.stored(t, failing) {
		t.Errorf("after FetchFailureTTL: requests = %d, stored = %v; want the parent fetched again", fe.requests(failing), e.stored(t, failing))
	}
}

func remoteResolves(t *testing.T, e *remoteStatusEnv) {
	fe := newRemoteFetchEnv(t, e)
	ctx := t.Context()
	const host = "other.example"
	bob := "https://" + host + "/users/bob"
	fe.serve(t, bob, map[string]any{
		"id": bob, "type": "Person", "preferredUsername": "bob", "inbox": bob + "/inbox", "followers": bob + "/followers",
	})
	jrd, err := json.Marshal(map[string]any{
		"subject": "acct:bob@" + host, "links": []map[string]string{{"rel": "self", "type": activityJSON, "href": bob}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fe.net.set(webFingerURL(host, "acct:bob@"+host), remoteResponse{status: http.StatusOK, contentType: "application/jrd+json", body: string(jrd)})

	// A handle and an actor URL are accounts.
	for _, q := range []string{"@bob@" + host, "bob@" + strings.ToUpper(host), bob} {
		got, err := fe.resolver.Resolve(ctx, q)
		if err != nil || got.Account == nil || got.Status != nil || got.Account.Account.URI != bob {
			t.Errorf("Resolve(%s) = %+v, %v; want bob's account", q, got, err)
		}
	}
	// A post URL is a status, stored from its fetch, whose parent is fetched one level.
	published := remoteNow.Add(-2 * time.Hour)
	parent, post, page := bob+"/notes/1", bob+"/notes/2", "https://"+host+"/@bob/2"
	fe.serve(t, parent, remoteNote(parent, bob, published, nil))
	note := remoteNote(post, bob, published, map[string]any{"inReplyTo": parent, "content": "<p>found</p>"})
	fe.serve(t, post, note)
	fe.serve(t, page, note)
	got, err := fe.resolver.Resolve(ctx, page)
	if err != nil || got.Status == nil || got.Account != nil || got.Status.URI != post {
		t.Fatalf("Resolve(%s) = %+v, %v; want the post %s", page, got, err, post)
	}
	if p := e.post(t, post); p.Revision.Bodies[0].ContentHTML != "<p>found</p>" {
		t.Errorf("resolved post = %+v", p.Revision)
	}
	fe.run(t)
	if !e.stored(t, parent) {
		t.Errorf("the parent of the resolved post was not fetched")
	}
	// Stored ones are not fetched again.
	before := fe.requests("https://" + host)
	if again, err := fe.resolver.Resolve(ctx, post); err != nil || again.Status == nil || again.Status.ID != got.Status.ID {
		t.Errorf("Resolve(%s) again = %+v, %v", post, again, err)
	}
	// A local status is found without a fetch; an unknown local URL, a collection, and a missing
	// document are not resolvable.
	local, err := e.content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: e.user(t, "local"), TextInput: appcontent.TextInput{Text: "local"}, Visibility: content.VisibilityPublic,
	})
	if err != nil {
		t.Fatal(err)
	}
	if mine, err := fe.resolver.Resolve(ctx, local.Status.URI); err != nil || mine.Status == nil || mine.Status.ID != local.Status.ID {
		t.Errorf("Resolve(%s) = %+v, %v; want the local status", local.Status.URI, mine, err)
	}
	if fe.requests("https://"+host) != before {
		t.Errorf("resolving stored things fetched something")
	}
	collection := bob + "/collections/featured"
	fe.serve(t, collection, map[string]any{"id": collection, "type": "OrderedCollection"})
	for _, q := range []string{"https://" + testkit.ContractHost + "/ap/objects/unknown", collection, bob + "/notes/missing", "bob", "not a handle@"} {
		if res, err := fe.resolver.Resolve(ctx, q); !errors.Is(err, appfederation.ErrUnresolvable) {
			t.Errorf("Resolve(%s) = %+v, %v; want ErrUnresolvable", q, res, err)
		}
	}
	if fe.requests("https://"+testkit.ContractHost) != 0 {
		t.Errorf("Resolve fetched from this server")
	}
	// The account of the resolved post is bob's.
	if a, err := e.remote.FindByURI(ctx, bob); err != nil || got.Status.AuthorAccountID != a.Account.ID {
		t.Errorf("author of the resolved post = %s, want bob %+v (%v)", got.Status.AuthorAccountID, a.Account, err)
	}
}
