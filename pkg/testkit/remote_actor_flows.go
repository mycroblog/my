// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/federation"
)

// RemoteActorHarness is what RemoteActorContract and RemoteActorFlows need from a database
// adapter: an empty database with the product schema, its UnitOfWork, and the account and
// federation repositories on it.
type RemoteActorHarness struct {
	UnitOfWork uow.UnitOfWork
	Accounts   appaccount.Repository
	Federation appfederation.Repository
	Instances  appfederation.InstanceRepository
}

// RemoteActorFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type RemoteActorFactory func(t *testing.T, opts uow.Options) RemoteActorHarness

// RemoteActorFlows verifies the remote actor use cases of the account and federation modules on a
// database adapter (federation.md §6), the flows of ADR-0153 §1, which internal/flowtest/federation
// runs once on SQLite; RemoteActorContract checks the repositories' methods on every adapter:
// storing the actor documents of the main
// implementations (snapshots, sanitized profile revisions, keys, handles, the server seen), key
// rotation under the same key ID, handles that WebFinger does not confirm, documents whose id has
// another origin, the fetch rules (signed GET, Content-Type, failures remembered, 410 Gone), the
// periodic refresh of followed stale actors, NodeInfo observations, the public key resolver, that
// the handle projection equals a rebuild, the resolution of handles and URLs (FR-305), the Update
// and Delete that remote actors send of themselves (federation.md §3.4), the shared inboxes
// known to the instance, and documents with NUL in every string (stored as U+FFFD, federation.md
// §4). The network is an in-memory HTTPClient.
func RemoteActorFlows(t *testing.T, newHarness RemoteActorFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *remoteEnv)
	}{
		{"StoresActorsOfTheMainImplementations", remoteStoresFixtureActors},
		{"KeepsOneSnapshotPerDocument", remoteKeepsOneSnapshotPerDocument},
		{"RevokesARotatedKey", remoteRevokesRotatedKey},
		{"KeepsNoUnconfirmedHandle", remoteKeepsNoUnconfirmedHandle},
		{"VerifiesHandlesOnAnotherDomain", remoteVerifiesHandleOnAnotherDomain},
		{"RejectsIDsOfAnotherOrigin", remoteRejectsForeignIDs},
		{"AppliesTheFetchRules", remoteAppliesFetchRules},
		{"MarksGoneActorsDeleted", remoteMarksGoneActorsDeleted},
		{"RechecksGoneActors", remoteRechecksGoneActors},
		{"DeleteSettlesAGoneMark", remoteDeleteSettlesAGoneMark},
		{"RefreshesFollowedStaleActors", remoteRefreshesStaleActors},
		{"ObservesNodeInfo", remoteObservesNodeInfo},
		{"ResolvesPublicKeys", remoteResolvesPublicKeys},
		{"RebuildsHandles", remoteRebuildsHandles},
		{"ReceivesUpdatesOfActors", remoteReceivesActorUpdates},
		{"ReceivesDeletesOfActors", remoteReceivesActorDeletes},
		{"ListsKnownSharedInboxes", remoteListsKnownSharedInboxes},
		{"ResolvesHandlesAndURLs", remoteResolvesHandlesAndURLs},
		{"StoresStringsWithNUL", remoteStoresStringsWithNUL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newRemoteEnv(t, newHarness)) })
	}
}

type remoteEnv struct {
	h         RemoteActorHarness
	clock     *Clock
	kv        *KeyValueStore
	net       *remoteNet
	jobs      *remoteJobs
	handlers  *queue.JobHandlerRegistry
	scheduler *queue.Scheduler
	accounts  *appaccount.Service
	actors    *appfederation.RemoteActors
	followed  map[string]bool
	instance  appaccount.Actor
}

func newRemoteEnv(t *testing.T, newHarness RemoteActorFactory) *remoteEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appaccount.RegisterProjectors(projectors, h.Accounts)
	clock := NewClock(time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC))
	ids := NewIDGenerator(clock)
	e := &remoteEnv{
		h: h, clock: clock, kv: NewKeyValueStore(clock), net: &remoteNet{routes: map[string]remoteResponse{}},
		jobs: &remoteJobs{seen: map[string]bool{}}, handlers: &queue.JobHandlerRegistry{}, followed: map[string]bool{},
	}
	e.scheduler = queue.NewScheduler(e.jobs, clock, nil)
	images := map[string]string{}
	e.accounts = appaccount.NewService(appaccount.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Accounts, Clock: clock, IDs: ids,
		Sealer: kernel.NewSealer(NewKeyManager("remote")), Host: contractHost, GenerateKey: cachedKey,
		Sanitizer: remoteSanitizer{},
		RecordRemoteImage: func(_ context.Context, _ uow.Tx, accountID, u string, _ account.ProfileProperty) (string, error) {
			k := accountID + " " + u
			if images[k] == "" {
				images[k] = ids.NewID()
			}
			return images[k], nil
		},
	})
	var err error
	if _, err = e.accounts.EnsureInstanceActor(t.Context()); err != nil {
		t.Fatalf("EnsureInstanceActor: %v", err)
	}
	if e.instance, err = e.accounts.InstanceActor(t.Context()); err != nil {
		t.Fatalf("InstanceActor: %v", err)
	}
	client := appfederation.NewSignedHTTPClient(e.net, remoteSigner{}, e.kv, clock)
	fetcher := appfederation.NewFetcher(appfederation.FetcherDeps{
		Client: client, KV: e.kv, Clock: clock, Domains: appfederation.NoDomainRestrictions{}, Instances: h.Federation,
		Key: appfederation.InstanceKey(e.accounts), Host: contractHost,
	})
	e.actors = appfederation.NewRemoteActors(appfederation.RemoteActorsDeps{
		UnitOfWork: h.UnitOfWork, Accounts: e.accounts, Instances: h.Instances, Fetcher: fetcher, HTTP: e.net,
		Jobs: e.jobs, Outbox: queue.AfterCommit(projectors, e.jobs, nil), Followers: remoteFollowers(e.followed),
		Clock: clock, IDs: ids, Batch: 2,
	})
	e.actors.RegisterJobs(e.handlers, e.scheduler)
	return e
}

// remoteSanitizer stands in for the HTML sanitizer: it drops script elements and records
// version 7, so that the contract sees that remote HTML went through it.
type remoteSanitizer struct{}

var (
	remoteScripts = regexp.MustCompile(`(?s)<script.*?</script>`)
	remoteTags    = regexp.MustCompile(`<[^>]*>`)
)

func (remoteSanitizer) SanitizeHTML(s string) string { return remoteScripts.ReplaceAllString(s, "") }
func (remoteSanitizer) PlainText(s string) string    { return remoteTags.ReplaceAllString(s, "") }
func (remoteSanitizer) PolicyVersion() int           { return 7 }

// remoteSigner stands in for the HTTP signature adapter: it names the key in a Signature header.
type remoteSigner struct{}

func (remoteSigner) Sign(req *kernel.HTTPRequest, key appfederation.SigningKey, _ appfederation.SignOptions) error {
	req.Header.Set("Signature", `keyId="`+key.KeyID+`"`)
	return nil
}

func (remoteSigner) ParseAcceptSignature(string) ([]appfederation.RequestedSignature, error) {
	return nil, appfederation.ErrMalformedSignature
}

type remoteFollowers map[string]bool

func (f remoteFollowers) ActiveFollowerIDs(_ context.Context, accountID, _ string, _ int) ([]string, string, error) {
	if f[accountID] {
		return []string{"local-follower"}, "", nil
	}
	return nil, "", nil
}

// remoteResponse is what the in-memory network answers for one URL. finalURL is the URL after
// redirects ("" for the requested one); err is returned instead of a response.
type remoteResponse struct {
	status      int
	contentType string
	body        string
	finalURL    string
	err         error
}

// remoteNet is a kernel.HTTPClient that answers from routes by exact URL and keeps every request.
type remoteNet struct {
	mu       sync.Mutex
	routes   map[string]remoteResponse
	requests []kernel.HTTPRequest
}

func (n *remoteNet) set(u string, r remoteResponse) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.routes[u] = r
}

func (n *remoteNet) count(u string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	c := 0
	for _, r := range n.requests {
		if r.URL == u {
			c++
		}
	}
	return c
}

func (n *remoteNet) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.requests = append(n.requests, *req)
	r, ok := n.routes[req.URL]
	if !ok {
		r = remoteResponse{status: http.StatusNotFound}
	}
	if r.err != nil {
		return nil, r.err
	}
	final := req.URL
	if r.finalURL != "" {
		final = r.finalURL
	}
	h := http.Header{}
	if r.contentType != "" {
		h.Set("Content-Type", r.contentType)
	}
	return &kernel.HTTPResponse{StatusCode: r.status, Header: h, URL: final, Body: io.NopCloser(strings.NewReader(r.body))}, nil
}

// remoteJobs is a queue.JobQueue that keeps the jobs given to it, skipping a pending job's
// idempotency key.
type remoteJobs struct {
	mu      sync.Mutex
	pending []queue.NewJob
	seen    map[string]bool
}

func (q *remoteJobs) Enqueue(_ context.Context, jobs ...queue.NewJob) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range jobs {
		if j.IdempotencyKey != "" && q.seen[j.IdempotencyKey] {
			continue
		}
		q.seen[j.IdempotencyKey] = true
		q.pending = append(q.pending, j)
	}
	return nil
}

// take removes and returns the pending jobs of a type.
func (q *remoteJobs) take(jobType string) []queue.NewJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out, rest []queue.NewJob
	for _, j := range q.pending {
		if j.Type == jobType {
			out = append(out, j)
			delete(q.seen, j.IdempotencyKey)
		} else {
			rest = append(rest, j)
		}
	}
	q.pending = rest
	return out
}

// run runs the pending jobs of a type until none is left and returns how many ran.
func (e *remoteEnv) run(t *testing.T, jobType string) int {
	t.Helper()
	h, _, ok := e.handlers.Lookup(jobType)
	if !ok {
		t.Fatalf("no handler for %s", jobType)
	}
	n := 0
	for {
		jobs := e.jobs.take(jobType)
		if len(jobs) == 0 {
			return n
		}
		for _, j := range jobs {
			n++
			if res := h(t.Context(), queue.Job{Type: j.Type, Payload: j.Payload, IdempotencyKey: j.IdempotencyKey, Attempt: 1}); !res.IsDone() {
				t.Fatalf("%s %s: %v", jobType, j.Payload, res.Err())
			}
		}
	}
}

const activityJSON = "application/activity+json"

// serveActor serves an actor document at its id and the WebFinger answer that confirms
// acct:{username}@{host}.
func (e *remoteEnv) serveActor(doc []byte) (id, username, host string) {
	var a struct {
		ID                string `json:"id"`
		PreferredUsername string `json:"preferredUsername"`
	}
	if err := json.Unmarshal(doc, &a); err != nil {
		panic(err)
	}
	u, _ := url.Parse(a.ID)
	e.net.set(a.ID, remoteResponse{status: http.StatusOK, contentType: activityJSON + "; charset=utf-8", body: string(doc)})
	e.serveWebFinger(u.Host, a.PreferredUsername, u.Host, a.ID)
	return a.ID, a.PreferredUsername, u.Host
}

// serveWebFinger answers WebFinger of acct:{username}@{host} on host with subject
// acct:{username}@{subjectDomain} and a self link to self.
func (e *remoteEnv) serveWebFinger(host, username, subjectDomain, self string) {
	jrd, _ := json.Marshal(map[string]any{
		"subject": "acct:" + username + "@" + subjectDomain,
		"links": []map[string]string{
			{"rel": "http://webfinger.net/rel/profile-page", "type": "text/html", "href": "https://" + host + "/@" + username},
			{"rel": "self", "type": activityJSON, "href": self},
		},
	})
	e.net.set(webFingerURL(host, "acct:"+username+"@"+host), remoteResponse{status: http.StatusOK, contentType: "application/jrd+json", body: string(jrd)})
}

func webFingerURL(host, resource string) string {
	return "https://" + host + "/.well-known/webfinger?resource=" + url.QueryEscape(resource)
}

// fixtureActor returns an actor fixture, with the http:// URIs of the GoToSocial test data
// changed to https:// (GoToSocial serves https; fetches are https only).
func fixtureActor(t *testing.T, path string) []byte {
	t.Helper()
	fixtures, err := FederationFixtures()
	if err != nil {
		t.Fatalf("FederationFixtures: %v", err)
	}
	for _, f := range fixtures {
		if f.Path() == path {
			return bytes.ReplaceAll(f.JSON, []byte("http://gotosocial.example"), []byte("https://gotosocial.example"))
		}
	}
	t.Fatalf("no fixture %s", path)
	return nil
}

func (e *remoteEnv) resolve(t *testing.T, uri string) appaccount.Actor {
	t.Helper()
	a, err := e.actors.Resolve(t.Context(), uri)
	if err != nil {
		t.Fatalf("Resolve(%s): %v", uri, err)
	}
	return a
}

func (e *remoteEnv) refresh(t *testing.T, uri string) appaccount.Actor {
	t.Helper()
	a, err := e.actors.Refresh(t.Context(), uri)
	if err != nil {
		t.Fatalf("Refresh(%s): %v", uri, err)
	}
	return a
}

func (e *remoteEnv) tx(t *testing.T, fn func(tx uow.Tx) error) {
	t.Helper()
	if err := e.h.UnitOfWork.Tx(t.Context(), fn); err != nil {
		t.Fatalf("Tx: %v", err)
	}
}

func remoteStoresFixtureActors(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	cases := []struct {
		path, keyID, display, note, inbox, shared, profile string
		fields                                             []account.ProfileField
	}{
		{
			path: "mastodon/4.8/actor", keyID: "https://mastodon.example/users/alice#main-key", display: "Alice", note: "<p>Hello</p>",
			inbox: "https://mastodon.example/users/alice/inbox", shared: "https://mastodon.example/inbox", profile: "https://mastodon.example/@alice",
			fields: []account.ProfileField{{Position: 0, Name: "Website", ValueHTML: `<a href="https://example.org" rel="me">example.org</a>`}},
		},
		{
			path: "misskey/2025.4/actor", keyID: "https://misskey.example/users/9xyzabc123#main-key", display: "Alice", note: "<p>Hello</p>",
			inbox: "https://misskey.example/users/9xyzabc123/inbox", shared: "https://misskey.example/inbox", profile: "https://misskey.example/@alice",
		},
		{
			path: "gotosocial/0.22/actor", keyID: "https://gotosocial.example/users/the_mighty_zork/main-key", display: "original zork (he/they)",
			note: "<p>hey yo this is my profile!</p>", inbox: "https://gotosocial.example/users/the_mighty_zork/inbox",
			profile: "https://gotosocial.example/@the_mighty_zork",
		},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			id, username, host := e.serveActor(fixtureActor(t, c.path))
			a := e.resolve(t, id)
			if a.Account.URI != id || a.Account.Domain != host || a.Account.ActorType != account.ActorPerson || a.Local != nil {
				t.Errorf("account = %+v, want a remote person %s on %s", a.Account, id, host)
			}
			if a.Handle == nil || a.Handle.Username != username || a.Handle.Domain != host {
				t.Errorf("handle = %+v, want %s@%s", a.Handle, username, host)
			}
			if got, err := e.accounts.FindByHandle(ctx, username, host); err != nil || got.Account.ID != a.Account.ID {
				t.Errorf("FindByHandle(%s@%s) = %+v, %v", username, host, got.Account, err)
			}
			p := a.Profile
			if p == nil || p.Seq != 1 || p.DisplayName != c.display || p.NoteHTML != c.note || p.AvatarMediaID == "" || p.HeaderMediaID == "" || p.Source != nil {
				t.Errorf("profile = %+v, want revision 1 %q %q with images", p, c.display, c.note)
			}
			if c.fields != nil && (p == nil || !reflect.DeepEqual(p.Fields, c.fields)) {
				t.Errorf("fields = %+v, want %+v", p.Fields, c.fields)
			}
			if len(a.Keys) != 1 || a.Keys[0].KeyID != c.keyID || a.Keys[0].Algorithm != account.KeyRSASHA256 {
				t.Errorf("keys = %+v, want the RSA key %s", a.Keys, c.keyID)
			}
			head, err := e.accounts.RemoteHead(ctx, a.Account.ID)
			if err != nil || head.InboxURL != c.inbox || head.SharedInboxURL != c.shared || !head.LastFetchedAt.Equal(e.clock.Now()) {
				t.Errorf("head = %+v, %v; want inboxes %s %s", head, err, c.inbox, c.shared)
			}
			e.tx(t, func(tx uow.Tx) error {
				snap, err := e.h.Accounts.LatestRemoteSnapshot(ctx, tx, a.Account.ID)
				if err != nil || snap.ID != head.SnapshotID || snap.PreferredUsername != username || snap.ProfileURL != c.profile || len(snap.DocumentDigest) != 32 {
					t.Errorf("snapshot = %+v, %v", snap, err)
				}
				if again, err := e.h.Instances.InsertRemoteInstance(ctx, tx, host, e.clock.Now()); err != nil || again {
					t.Errorf("InsertRemoteInstance(%s) again = %v, %v; want the server recorded", host, again, err)
				}
				return nil
			})
			if a.Lifecycle.State != account.StateActive {
				t.Errorf("lifecycle = %+v, want active", a.Lifecycle)
			}
		})
	}
	// Every fetch is signed with the instance actor's RSA key and accepts ActivityStreams only.
	wantKey := account.KeyIRI(e.instance.Account.URI, account.KeyRSASHA256)
	for _, r := range e.net.requests {
		if strings.Contains(r.URL, "/.well-known/") {
			if r.Header.Get("Signature") != "" {
				t.Errorf("WebFinger %s is signed", r.URL)
			}
			continue
		}
		if r.Header.Get("Signature") != `keyId="`+wantKey+`"` || r.Header.Get("Accept") != appfederation.ActivityStreamsAccept || r.Purpose != kernel.HTTPPurposeJSON {
			t.Errorf("fetch %s: Signature %q Accept %q, want signed by %s", r.URL, r.Header.Get("Signature"), r.Header.Get("Accept"), wantKey)
		}
	}
	if got := len(e.jobs.take(appfederation.JobObserveInstance)); got != 3 {
		t.Errorf("%d NodeInfo observations enqueued, want one per server", got)
	}
}

func remoteKeepsOneSnapshotPerDocument(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	doc := fixtureActor(t, "mastodon/4.8/actor")
	id, _, _ := e.serveActor(doc)
	first := e.resolve(t, id)
	e.clock.Advance(time.Hour)
	again := e.refresh(t, id)
	head, _ := e.accounts.RemoteHead(ctx, again.Account.ID)
	if again.Account.ID != first.Account.ID || again.Profile.ID != first.Profile.ID || !head.LastFetchedAt.Equal(e.clock.Now()) {
		t.Errorf("same document: profile %s → %s, fetched %v; want the same revision and a new fetch time", first.Profile.ID, again.Profile.ID, head.LastFetchedAt)
	}
	firstSnapshot := head.SnapshotID

	e.clock.Advance(time.Hour)
	changed := bytes.Replace(doc, []byte(`"name": "Alice"`), []byte(`"name": "Alice <script>x</script>Liddell"`), 1)
	changed = bytes.Replace(changed, []byte(`"summary": "<p>Hello</p>"`), []byte(`"summary": "<p>Hi</p><script>alert(1)</script>"`), 1)
	e.serveActor(changed)
	revised := e.refresh(t, id)
	head, _ = e.accounts.RemoteHead(ctx, revised.Account.ID)
	if head.SnapshotID == firstSnapshot || revised.Profile.Seq != 2 || revised.Profile.DisplayName != "Alice xLiddell" || revised.Profile.NoteHTML != "<p>Hi</p>" {
		t.Errorf("changed document: snapshot %s, profile %+v; want a new snapshot and a sanitized revision 2", head.SnapshotID, revised.Profile)
	}
}

// rotatedKeyPEM returns the PEM of an RSA key other than ExamplePublicKeyPEM.
func rotatedKeyPEM(t *testing.T) string {
	t.Helper()
	k, err := cachedKey(account.KeyRSASHA256)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(k.(*rsa.PrivateKey).Public())
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func remoteRevokesRotatedKey(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	doc := fixtureActor(t, "mastodon/4.8/actor")
	id, _, _ := e.serveActor(doc)
	keyID := "https://mastodon.example/users/alice#main-key"
	before := e.resolve(t, id)
	old := before.Keys[0]

	e.clock.Advance(time.Minute)
	rotated := bytes.Replace(doc, []byte(jsonString(ExamplePublicKeyPEM)), []byte(jsonString(rotatedKeyPEM(t))), 1)
	e.serveActor(rotated)
	key, err := e.actors.ResolvePublicKey(ctx, keyID, true)
	if err != nil {
		t.Fatalf("ResolvePublicKey(refresh): %v", err)
	}
	want, _ := cachedKey(account.KeyRSASHA256)
	if pub, ok := key.Key.(*rsa.PublicKey); !ok || !pub.Equal(want.Public()) || key.Owner != id || key.KeyID != keyID {
		t.Errorf("ResolvePublicKey = %+v, want the rotated key of %s", key, id)
	}
	cur, err := e.accounts.CurrentKey(ctx, keyID)
	if err != nil || cur.ID == old.ID || cur.KeyID != keyID {
		t.Errorf("CurrentKey = %+v, %v; want a new key under %s", cur, err, keyID)
	}
	e.tx(t, func(tx uow.Tx) error {
		active, err := e.h.Accounts.ListActiveKeys(ctx, tx, before.Account.ID)
		if err != nil || len(active) != 1 || active[0].ID != cur.ID {
			t.Errorf("active keys = %+v, %v; want only the rotated key", active, err)
		}
		if revoked, err := e.h.Accounts.InsertKeyRevocation(ctx, tx, account.KeyRevocation{AccountKeyID: old.ID, RevokedAt: e.clock.Now()}); err != nil || revoked {
			t.Errorf("revoke the old key again = %v, %v; want it revoked already", revoked, err)
		}
		return nil
	})
	// Fetching the same document again keeps the rotated key.
	e.refresh(t, id)
	if again, err := e.accounts.CurrentKey(ctx, keyID); err != nil || again.ID != cur.ID {
		t.Errorf("CurrentKey after a refetch = %+v, %v; want %s", again, err, cur.ID)
	}

	// A key the document no longer declares is revoked.
	withdrawn := bytes.Replace(rotated, []byte(`"id": "https://mastodon.example/users/alice#main-key"`), []byte(`"id": "https://mastodon.example/users/alice#second-key"`), 1)
	e.serveActor(withdrawn)
	e.refresh(t, id)
	if _, err := e.accounts.CurrentKey(ctx, keyID); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("CurrentKey(withdrawn) = %v, want ErrNotFound", err)
	}
	if k, err := e.accounts.CurrentKey(ctx, "https://mastodon.example/users/alice#second-key"); err != nil || k.AccountID != before.Account.ID {
		t.Errorf("CurrentKey(second-key) = %+v, %v", k, err)
	}
}

func remoteKeepsNoUnconfirmedHandle(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	doc := fixtureActor(t, "mastodon/4.8/actor")
	id, username, host := e.serveActor(doc)
	resource := webFingerURL(host, "acct:"+username+"@"+host)

	// WebFinger names another actor: no handle.
	e.serveWebFinger(host, username, host, "https://mastodon.example/users/mallory")
	a := e.resolve(t, id)
	if a.Handle != nil {
		t.Errorf("handle = %+v, want none when WebFinger names another actor", a.Handle)
	}
	if _, err := e.accounts.FindByHandle(ctx, username, host); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindByHandle = %v, want ErrNotFound", err)
	}

	// Confirmed: the handle appears.
	e.serveWebFinger(host, username, host, id)
	if a = e.refresh(t, id); a.Handle == nil || a.Handle.Username != username {
		t.Errorf("handle = %+v, want %s once WebFinger confirms it", a.Handle, username)
	}

	// WebFinger does not answer: the handle stays.
	e.net.set(resource, remoteResponse{status: http.StatusServiceUnavailable})
	if a = e.refresh(t, id); a.Handle == nil {
		t.Error("handle removed when WebFinger did not answer")
	}

	// WebFinger answers 404: the handle goes.
	e.net.set(resource, remoteResponse{status: http.StatusNotFound})
	if a = e.refresh(t, id); a.Handle != nil {
		t.Errorf("handle = %+v, want none after WebFinger answered 404", a.Handle)
	}
	e.tx(t, func(tx uow.Tx) error {
		v, err := e.h.Accounts.LatestHandleVerification(ctx, tx, a.Account.ID)
		if err != nil || v.Outcome != account.HandleMismatch || v.Username != username || v.Domain != host {
			t.Errorf("latest check = %+v, %v; want a mismatch of %s@%s", v, err, username, host)
		}
		return nil
	})

	// Another actor that WebFinger confirms under the same name takes the handle.
	e.serveWebFinger(host, username, host, id)
	e.refresh(t, id)
	other := bytes.ReplaceAll(doc, []byte("https://mastodon.example/users/alice"), []byte("https://mastodon.example/users/alice2"))
	otherID, _, _ := e.serveActor(other)
	b := e.resolve(t, otherID)
	if b.Handle == nil || b.Handle.Username != username {
		t.Errorf("new holder handle = %+v", b.Handle)
	}
	if got, err := e.accounts.FindByHandle(ctx, username, host); err != nil || got.Account.ID != b.Account.ID {
		t.Errorf("FindByHandle = %s, %v; want the account that verified it last", got.Account.ID, err)
	}
}

func remoteVerifiesHandleOnAnotherDomain(t *testing.T, e *remoteEnv) {
	doc := fixtureActor(t, "mastodon/4.8/actor")
	id, username, host := e.serveActor(doc)
	// The server's WebFinger names the handle on the account domain example.org.
	e.serveWebFinger(host, username, "example.org", id)
	a := e.resolve(t, id)
	if a.Handle != nil {
		t.Errorf("handle = %+v before example.org confirms it", a.Handle)
	}
	jrd, _ := json.Marshal(map[string]any{
		"subject": "acct:" + username + "@example.org",
		"links":   []map[string]string{{"rel": "self", "type": activityJSON, "href": id}},
	})
	e.net.set(webFingerURL("example.org", "acct:"+username+"@example.org"), remoteResponse{status: http.StatusOK, contentType: "application/jrd+json", body: string(jrd)})
	if a = e.refresh(t, id); a.Handle == nil || a.Handle.Username != username || a.Handle.Domain != "example.org" {
		t.Errorf("handle = %+v, want %s@example.org", a.Handle, username)
	}
}

func remoteRejectsForeignIDs(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	doc := fixtureActor(t, "mastodon/4.8/actor")
	id, _, _ := e.serveActor(doc)

	// A server that serves a document of another origin.
	e.net.set("https://evil.example/users/alice", remoteResponse{status: http.StatusOK, contentType: activityJSON, body: string(doc)})
	if _, err := e.actors.Resolve(ctx, "https://evil.example/users/alice"); !errors.Is(err, appfederation.ErrFetchNotFound) {
		t.Errorf("Resolve(evil) = %v, want ErrFetchNotFound", err)
	}
	// A redirect to another origin than the id's.
	e.net.set("https://mastodon.example/@alice", remoteResponse{status: http.StatusOK, contentType: activityJSON, body: string(doc), finalURL: "https://evil.example/alice"})
	if _, err := e.actors.Resolve(ctx, "https://mastodon.example/@alice"); !errors.Is(err, appfederation.ErrFetchNotFound) {
		t.Errorf("Resolve(redirected) = %v, want ErrFetchNotFound", err)
	}
	if _, err := e.accounts.FindByURI(ctx, id); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindByURI(%s) = %v; want nothing stored", id, err)
	}
	// Another URI of the same origin: the id is fetched once more and kept.
	e.net.set("https://mastodon.example/users/alice/alias", remoteResponse{status: http.StatusOK, contentType: activityJSON, body: string(doc)})
	if a := e.resolve(t, "https://mastodon.example/users/alice/alias"); a.Account.URI != id {
		t.Errorf("Resolve(alias) = %s, want %s", a.Account.URI, id)
	}
	if got := e.net.count(id); got != 1 {
		t.Errorf("%d fetches of the id, want 1", got)
	}
	// The id of the refetched document must be itself.
	e.net.set("https://mastodon.example/users/bob", remoteResponse{status: http.StatusOK, contentType: activityJSON, body: string(bytes.ReplaceAll(doc, []byte(id), []byte("https://mastodon.example/users/bob2")))})
	e.net.set("https://mastodon.example/users/bob2", remoteResponse{status: http.StatusOK, contentType: activityJSON, body: string(bytes.ReplaceAll(doc, []byte(id), []byte("https://mastodon.example/users/bob3")))})
	if _, err := e.actors.Resolve(ctx, "https://mastodon.example/users/bob"); !errors.Is(err, appfederation.ErrFetchNotFound) {
		t.Errorf("Resolve(bob) = %v, want ErrFetchNotFound", err)
	}
}

func remoteAppliesFetchRules(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	doc := fixtureActor(t, "mastodon/4.8/actor")
	cases := []struct {
		name string
		resp remoteResponse
		want error
	}{
		{"HTML", remoteResponse{status: http.StatusOK, contentType: "text/html", body: string(doc)}, appfederation.ErrFetchNotFound},
		{"JSON-LD without the profile", remoteResponse{status: http.StatusOK, contentType: "application/ld+json", body: string(doc)}, appfederation.ErrFetchNotFound},
		{"404", remoteResponse{status: http.StatusNotFound}, appfederation.ErrFetchNotFound},
		{"410", remoteResponse{status: http.StatusGone}, appfederation.ErrFetchGone},
		{"503", remoteResponse{status: http.StatusServiceUnavailable}, appfederation.ErrFetchFailed},
		{"network", remoteResponse{err: errors.New("connection reset")}, appfederation.ErrFetchFailed},
		{"refused", remoteResponse{err: kernel.ErrForbiddenDestination}, appfederation.ErrFetchNotFound},
	}
	for i, c := range cases {
		u := fmt.Sprintf("https://rules.example/users/%d", i)
		e.net.set(u, c.resp)
		if _, err := e.actors.Resolve(ctx, u); !errors.Is(err, c.want) {
			t.Errorf("%s: Resolve = %v, want %v", c.name, err, c.want)
		}
		// The failure is remembered: the next fetch within FetchFailureTTL sends nothing.
		if _, err := e.actors.Resolve(ctx, u); !errors.Is(err, c.want) || e.net.count(u) != 1 {
			t.Errorf("%s: second Resolve = %v after %d requests, want the remembered failure", c.name, err, e.net.count(u))
		}
	}
	e.clock.Advance(appfederation.FetchFailureTTL)
	e.net.set("https://rules.example/users/0", remoteResponse{status: http.StatusOK, contentType: `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`,
		body: string(bytes.ReplaceAll(doc, []byte("https://mastodon.example/users/alice"), []byte("https://rules.example/users/0")))})
	if _, err := e.actors.Resolve(ctx, "https://rules.example/users/0"); err != nil {
		t.Errorf("Resolve after FetchFailureTTL with the JSON-LD profile = %v", err)
	}
	if _, err := e.actors.Resolve(ctx, "http://rules.example/users/0"); !errors.Is(err, appfederation.ErrFetchNotFound) {
		t.Errorf("Resolve(http) = %v, want ErrFetchNotFound", err)
	}
	if _, err := e.actors.Resolve(ctx, "https://"+contractHost+"/ap/actors/x"); !errors.Is(err, appfederation.ErrFetchRefused) {
		t.Errorf("Resolve(local) = %v, want ErrFetchRefused", err)
	}
}

func remoteMarksGoneActorsDeleted(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	id, _, _ := e.serveActor(fixtureActor(t, "misskey/2025.4/actor"))
	a := e.resolve(t, id)

	e.net.set(id, remoteResponse{status: http.StatusServiceUnavailable})
	if _, err := e.actors.Refresh(ctx, id); !errors.Is(err, appfederation.ErrFetchFailed) {
		t.Fatalf("Refresh(503) = %v", err)
	}
	head, _ := e.accounts.RemoteHead(ctx, a.Account.ID)
	if head.ConsecutiveFetchFailures != 1 {
		t.Errorf("failures = %d, want 1", head.ConsecutiveFetchFailures)
	}

	e.clock.Advance(appfederation.FetchFailureTTL)
	e.net.set(id, remoteResponse{status: http.StatusGone})
	for range 2 {
		if _, err := e.actors.Refresh(ctx, id); !errors.Is(err, appfederation.ErrFetchGone) {
			t.Fatalf("Refresh(410) = %v", err)
		}
	}
	got, err := e.accounts.FindByID(ctx, a.Account.ID)
	if err != nil || got.Lifecycle.State != account.StateDeleted {
		t.Errorf("lifecycle = %+v, %v; want deleted", got.Lifecycle, err)
	}
	e.tx(t, func(tx uow.Tx) error {
		// The second 410 records nothing more.
		st, err := e.h.Accounts.LifecycleState(ctx, tx, a.Account.ID)
		if err != nil || st.State != account.StateDeleted {
			t.Errorf("state = %+v, %v", st, err)
		}
		return nil
	})
}

// remoteRechecksGoneActors runs the fetch before the erasure on an actor that a 410 marked.
func remoteRechecksGoneActors(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	doc := fixtureActor(t, "misskey/2025.4/actor")
	id, _, _ := e.serveActor(doc)
	a := e.resolve(t, id)
	served := remoteResponse{status: http.StatusOK, contentType: activityJSON + "; charset=utf-8", body: string(doc)}
	recheck := func(answer remoteResponse, want appaccount.DeletionRecheck) {
		t.Helper()
		e.clock.Advance(appfederation.FetchFailureTTL)
		e.net.set(id, answer)
		if got, err := e.actors.RecheckDeletion(ctx, a.Account.ID); err != nil || got != want {
			t.Errorf("RecheckDeletion(%d) = %v, %v; want %v", answer.status, got, err, want)
		}
	}
	state := func() account.LifecycleStateName {
		t.Helper()
		got, err := e.accounts.FindByID(ctx, a.Account.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.Lifecycle.State
	}

	e.clock.Advance(appfederation.FetchFailureTTL)
	e.net.set(id, remoteResponse{status: http.StatusGone})
	if _, err := e.actors.Refresh(ctx, id); !errors.Is(err, appfederation.ErrFetchGone) {
		t.Fatalf("Refresh(410) = %v", err)
	}
	recheck(remoteResponse{status: http.StatusServiceUnavailable}, appaccount.RecheckInconclusive)
	recheck(remoteResponse{status: http.StatusNotFound}, appaccount.RecheckInconclusive)
	recheck(remoteResponse{status: http.StatusGone}, appaccount.RecheckConfirmed)
	if got := state(); got != account.StateDeleted {
		t.Fatalf("before the document came back: %s, want deleted", got)
	}
	recheck(served, appaccount.RecheckRestored)
	if got := state(); got != account.StateActive {
		t.Errorf("after the document came back: %s, want active", got)
	}
	// A restored actor is an ordinary one: it stores its documents, and its own Delete is final.
	if _, err := e.actors.Refresh(ctx, id); err != nil {
		t.Errorf("Refresh of the restored actor = %v", err)
	}
	e.tx(t, func(tx uow.Tx) error { return e.accounts.MarkRemoteDeleted(ctx, tx, a.Account.ID) })
	recheck(served, appaccount.RecheckConfirmed)
	if got := state(); got != account.StateDeleted {
		t.Errorf("after its Delete: %s, want deleted", got)
	}
}

// remoteDeleteSettlesAGoneMark runs a signed Delete of an actor that a 410 marked: the erasure no
// longer asks its server, and a redelivered Delete records nothing more.
func remoteDeleteSettlesAGoneMark(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	doc := fixtureActor(t, "misskey/2025.4/actor")
	id, _, _ := e.serveActor(doc)
	a := e.resolve(t, id)
	e.clock.Advance(appfederation.FetchFailureTTL)
	e.net.set(id, remoteResponse{status: http.StatusGone})
	if _, err := e.actors.Refresh(ctx, id); !errors.Is(err, appfederation.ErrFetchGone) {
		t.Fatalf("Refresh(410) = %v", err)
	}
	unconfirmed := func() bool {
		t.Helper()
		got, err := e.accounts.RemoteDeletionUnconfirmed(ctx, a.Account.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if !unconfirmed() {
		t.Error("after the 410: confirmed, want unconfirmed")
	}
	e.tx(t, func(tx uow.Tx) error { return e.accounts.MarkRemoteGone(ctx, tx, a.Account.ID) })
	if !unconfirmed() {
		t.Error("after a second 410: confirmed, want unconfirmed")
	}
	for range 2 {
		e.tx(t, func(tx uow.Tx) error { return e.accounts.MarkRemoteDeleted(ctx, tx, a.Account.ID) })
		if unconfirmed() {
			t.Error("after the Delete: unconfirmed, want confirmed")
		}
	}
	e.tx(t, func(tx uow.Tx) error {
		if err := e.accounts.MarkRemoteGone(ctx, tx, a.Account.ID); err != nil {
			t.Errorf("MarkRemoteGone after the Delete = %v", err)
		}
		if err := e.accounts.RestoreRemoteActor(ctx, tx, a.Account.ID); !errors.Is(err, appaccount.ErrActorDeleted) {
			t.Errorf("RestoreRemoteActor after the Delete = %v, want ErrActorDeleted", err)
		}
		return nil
	})
	if unconfirmed() {
		t.Error("after a 410 that follows the Delete: unconfirmed, want confirmed")
	}
	e.clock.Advance(appfederation.FetchFailureTTL)
	before := e.net.count(id)
	if got, err := e.actors.RecheckDeletion(ctx, a.Account.ID); err != nil || got != appaccount.RecheckConfirmed {
		t.Errorf("RecheckDeletion = %v, %v; want confirmed", got, err)
	}
	if n := e.net.count(id) - before; n != 0 {
		t.Errorf("fetched the actor %d times after its Delete, want 0", n)
	}
}

func remoteRefreshesStaleActors(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	var ids []string
	for _, p := range []string{"mastodon/4.8/actor", "misskey/2025.4/actor", "gotosocial/0.22/actor"} {
		id, _, _ := e.serveActor(fixtureActor(t, p))
		a := e.resolve(t, id)
		ids = append(ids, a.Account.ID)
		e.clock.Advance(time.Minute)
	}
	e.followed[ids[0]] = true
	e.followed[ids[2]] = true

	// Fresh actors are not refreshed.
	e.scheduler.Tick(ctx)
	e.run(t, appfederation.JobRefreshActors)
	if n := len(e.jobs.take(appfederation.JobRefreshActor)); n != 0 {
		t.Errorf("%d refreshes of fresh actors", n)
	}

	e.clock.Advance(account.RemoteActorTTL)
	e.scheduler.Tick(ctx)
	e.run(t, appfederation.JobRefreshActors)
	if n := e.run(t, appfederation.JobRefreshActor); n != 2 {
		t.Errorf("%d refreshes, want the 2 followed actors", n)
	}
	for i, id := range ids {
		head, _ := e.accounts.RemoteHead(ctx, id)
		if fresh := !head.Stale(e.clock.Now()); fresh != e.followed[id] {
			t.Errorf("actor %d: fresh = %v, want %v", i, fresh, e.followed[id])
		}
	}

	// A stale actor used for a signature is refreshed asynchronously.
	a, _ := e.accounts.FindByID(ctx, ids[1])
	if _, err := e.actors.ResolvePublicKey(ctx, a.Keys[0].KeyID, false); err != nil {
		t.Fatalf("ResolvePublicKey: %v", err)
	}
	if n := e.run(t, appfederation.JobRefreshActor); n != 1 {
		t.Errorf("%d refreshes after a stale key was used, want 1", n)
	}
	if head, _ := e.accounts.RemoteHead(ctx, ids[1]); head.Stale(e.clock.Now()) {
		t.Error("the actor whose key was used is still stale")
	}
}

func remoteObservesNodeInfo(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	links := `{"links":[{"rel":"http://nodeinfo.diaspora.software/ns/schema/2.0","href":"https://mastodon.example/nodeinfo/2.0"},` +
		`{"rel":"http://nodeinfo.diaspora.software/ns/schema/2.1","href":"https://mastodon.example/nodeinfo/2.1"}]}`
	e.net.set("https://mastodon.example/.well-known/nodeinfo", remoteResponse{status: http.StatusOK, contentType: "application/json", body: links})
	e.net.set("https://mastodon.example/nodeinfo/2.1", remoteResponse{status: http.StatusOK, contentType: "application/json",
		body: `{"version":"2.1","software":{"name":"Mastodon","version":"4.8.0"}}`})
	id, _, _ := e.serveActor(fixtureActor(t, "mastodon/4.8/actor"))
	e.resolve(t, id)
	if n := e.run(t, appfederation.JobObserveInstance); n != 1 {
		t.Fatalf("%d observations ran, want 1", n)
	}
	var first string
	e.tx(t, func(tx uow.Tx) error {
		o, err := e.h.Instances.LatestNodeInfoObservation(ctx, tx, "mastodon.example")
		if err != nil || o.SoftwareName != "mastodon" || o.SoftwareVersion != "4.8.0" {
			t.Errorf("observation = %+v, %v", o, err)
		}
		first = o.ID
		return nil
	})
	// The same software again records nothing; a new version is recorded.
	if err := e.actors.ObserveInstance(ctx, "mastodon.example"); err != nil {
		t.Fatalf("ObserveInstance: %v", err)
	}
	e.net.set("https://mastodon.example/nodeinfo/2.1", remoteResponse{status: http.StatusOK, contentType: "application/json",
		body: `{"version":"2.1","software":{"name":"mastodon","version":"4.9.0"}}`})
	if err := e.actors.ObserveInstance(ctx, "mastodon.example"); err != nil {
		t.Fatalf("ObserveInstance: %v", err)
	}
	e.tx(t, func(tx uow.Tx) error {
		o, err := e.h.Instances.LatestNodeInfoObservation(ctx, tx, "mastodon.example")
		if err != nil || o.ID == first || o.SoftwareVersion != "4.9.0" {
			t.Errorf("observation = %+v, %v; want a new one for 4.9.0", o, err)
		}
		if _, err := e.h.Instances.LatestNodeInfoObservation(ctx, tx, "unseen.example"); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("LatestNodeInfoObservation(unseen) = %v, want ErrNotFound", err)
		}
		return nil
	})
	// NodeInfo on another host is not believed.
	e.net.set("https://misskey.example/.well-known/nodeinfo", remoteResponse{status: http.StatusOK, contentType: "application/json",
		body: `{"links":[{"rel":"http://nodeinfo.diaspora.software/ns/schema/2.1","href":"https://elsewhere.example/nodeinfo"}]}`})
	if err := e.actors.ObserveInstance(ctx, "misskey.example"); err == nil {
		t.Error("ObserveInstance accepted NodeInfo on another host")
	}
}

func remoteResolvesPublicKeys(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	// A key ID on its own path (GoToSocial): the key document leads to its owner.
	doc := fixtureActor(t, "gotosocial/0.22/actor")
	actorID := "https://gotosocial.example/users/the_mighty_zork"
	keyID := actorID + "/main-key"
	e.serveActor(doc)
	keyDoc, _ := json.Marshal(map[string]any{
		"@context": "https://w3id.org/security/v1", "id": keyID, "type": "Key", "owner": actorID, "publicKeyPem": ExamplePublicKeyPEM,
	})
	e.net.set(keyID, remoteResponse{status: http.StatusOK, contentType: activityJSON, body: string(keyDoc)})
	key, err := e.actors.ResolvePublicKey(ctx, keyID, false)
	if err != nil || key.Owner != actorID || key.KeyID != keyID {
		t.Fatalf("ResolvePublicKey(%s) = %+v, %v", keyID, key, err)
	}
	if _, ok := key.Key.(*rsa.PublicKey); !ok {
		t.Errorf("key is %T, want RSA", key.Key)
	}
	// Stored now: no fetch.
	before := len(e.net.requests)
	if again, err := e.actors.ResolvePublicKey(ctx, keyID, false); err != nil || again.Owner != actorID || len(e.net.requests) != before {
		t.Errorf("stored ResolvePublicKey = %+v, %v after %d requests", again, err, len(e.net.requests)-before)
	}
	// A fragment key ID (Mastodon) of an unknown actor.
	e.serveActor(fixtureActor(t, "mastodon/4.8/actor"))
	if k, err := e.actors.ResolvePublicKey(ctx, "https://mastodon.example/users/alice#main-key", false); err != nil || k.Owner != "https://mastodon.example/users/alice" {
		t.Errorf("ResolvePublicKey(#main-key) = %+v, %v", k, err)
	}
	// A key the owner does not declare.
	if _, err := e.actors.ResolvePublicKey(ctx, "https://mastodon.example/users/alice#other-key", false); !errors.Is(err, appfederation.ErrPublicKeyNotFound) {
		t.Errorf("ResolvePublicKey(undeclared) = %v, want ErrPublicKeyNotFound", err)
	}
	// Unknown and unreachable.
	e.net.set("https://down.example/users/x", remoteResponse{err: errors.New("timeout")})
	if _, err := e.actors.ResolvePublicKey(ctx, "https://down.example/users/x#main-key", false); !errors.Is(err, appfederation.ErrPublicKeyNotFound) {
		t.Errorf("ResolvePublicKey(down) = %v, want ErrPublicKeyNotFound", err)
	}
	// A server marked unavailable answers 503 to the inbox.
	e.tx(t, func(tx uow.Tx) error {
		return e.h.Federation.UpsertInstanceHealth(ctx, tx, unavailableHealth("broken.example", e.clock.Now()))
	})
	if _, err := e.actors.ResolvePublicKey(ctx, "https://broken.example/users/x#main-key", false); !errors.Is(err, appfederation.ErrPublicKeyUnavailable) {
		t.Errorf("ResolvePublicKey(unavailable) = %v, want ErrPublicKeyUnavailable", err)
	}
}

func unavailableHealth(domain string, since time.Time) federation.InstanceHealth {
	return federation.InstanceHealth{Domain: domain, State: federation.InstanceUnavailable, Since: since, FailureDays: 7, LastFailureDay: since.Format("20060102")}
}

func remoteRebuildsHandles(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	for _, p := range []string{"mastodon/4.8/actor", "misskey/2025.4/actor"} {
		id, _, _ := e.serveActor(fixtureActor(t, p))
		e.resolve(t, id)
	}
	id, username, host := e.serveActor(fixtureActor(t, "gotosocial/0.22/actor"))
	e.serveWebFinger(host, username, host, "https://gotosocial.example/users/someone_else")
	e.resolve(t, id)
	want, err := e.h.Accounts.ListHandles(ctx)
	if err != nil {
		t.Fatalf("ListHandles: %v", err)
	}
	if len(want) != 2 {
		t.Errorf("%d handles, want the 2 verified ones", len(want))
	}
	if err := appaccount.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Accounts); err != nil {
		t.Fatalf("RebuildProjections: %v", err)
	}
	if got, err := e.h.Accounts.ListHandles(ctx); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("rebuilt handles = %+v, %v; want %+v", got, err, want)
	}
}

// receivedActivity normalizes an inbound activity document as the inbox job does.
func (e *remoteEnv) receivedActivity(t *testing.T, raw []byte) *federation.Activity {
	t.Helper()
	doc, err := appfederation.Normalize(raw, e.clock.Now())
	if err != nil || doc.Activity == nil {
		t.Fatalf("Normalize(%s) = %+v, %v; want an activity", raw, doc, err)
	}
	return doc.Activity
}

// renamedUpdate returns the GoToSocial Update of the actor of "gotosocial/0.22/actor" with the
// display name changed to name.
func renamedUpdate(t *testing.T, name string) []byte {
	t.Helper()
	return bytes.ReplaceAll(fixtureActor(t, "gotosocial/0.22/update-person"), []byte(`"name": "original zork (he/they)"`), []byte(`"name": "`+name+`"`))
}

func remoteReceivesActorUpdates(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	id, _, _ := e.serveActor(fixtureActor(t, "gotosocial/0.22/actor"))
	first := e.resolve(t, id)
	if first.Profile == nil || first.Profile.Seq != 1 {
		t.Fatalf("profile = %+v, want revision 1", first.Profile)
	}

	// An Update with the actor's own document embedded makes a new profile revision from it,
	// without fetching the actor.
	update := renamedUpdate(t, "renamed zork")
	fetches := e.net.count(id)
	act := e.receivedActivity(t, update)
	if !appfederation.IsOwnActorActivity(act) {
		t.Fatalf("IsOwnActorActivity(%+v) = false", act)
	}
	for range 2 { // handling it again changes nothing
		if err := e.actors.ReceiveActorUpdate(ctx, act, update); err != nil {
			t.Fatalf("ReceiveActorUpdate: %v", err)
		}
		got, err := e.accounts.FindByID(ctx, first.Account.ID)
		if err != nil || got.Profile == nil || got.Profile.Seq != 2 || got.Profile.DisplayName != "renamed zork" {
			t.Errorf("profile = %+v, %v; want revision 2 renamed", got.Profile, err)
		}
		if len(got.Keys) != 1 || got.Keys[0].KeyID != "https://gotosocial.example/users/the_mighty_zork/main-key" {
			t.Errorf("keys = %+v, want the embedded key", got.Keys)
		}
	}
	if n := e.net.count(id); n != fetches {
		t.Errorf("%d fetches of the actor, want none for an embedded document", n-fetches)
	}

	// An Update that names the actor only makes it be fetched again.
	e.clock.Advance(time.Minute)
	byIRI := []byte(`{"@context":"https://www.w3.org/ns/activitystreams","id":"` + id + `#updates/2","type":"Update","actor":"` + id + `","object":"` + id + `"}`)
	if err := e.actors.ReceiveActorUpdate(ctx, e.receivedActivity(t, byIRI), byIRI); err != nil {
		t.Fatalf("ReceiveActorUpdate(IRI): %v", err)
	}
	if n := e.net.count(id); n != fetches+1 {
		t.Errorf("%d fetches of the actor, want 1 for an Update without its document", n-fetches)
	}
	got, _ := e.accounts.FindByID(ctx, first.Account.ID)
	if got.Profile == nil || got.Profile.Seq != 3 || got.Profile.DisplayName != "original zork (he/they)" {
		t.Errorf("profile after the fetch = %+v, want revision 3 from the served document", got.Profile)
	}

	// Activities about other objects are not the actor handlers'.
	note := []byte(`{"@context":"https://www.w3.org/ns/activitystreams","id":"` + id + `/statuses/1#update","type":"Update","actor":"` + id + `",` +
		`"object":{"id":"` + id + `/statuses/1","type":"Note","attributedTo":"` + id + `","content":"<p>hi</p>","to":["https://www.w3.org/ns/activitystreams#Public"]}}`)
	act = e.receivedActivity(t, note)
	if appfederation.IsOwnActorActivity(act) {
		t.Errorf("IsOwnActorActivity(Update of a Note) = true")
	}
	if err := e.actors.ReceiveActorUpdate(ctx, act, note); !errors.Is(err, appfederation.ErrNotOwnActor) {
		t.Errorf("ReceiveActorUpdate(Note) = %v, want ErrNotOwnActor", err)
	}
}

// remoteStoresStringsWithNUL stores an actor whose every string has a NUL (\u0000), fetched and
// received in an Update: each NUL is stored as U+FFFD (federation.md §4) the same way in every
// dialect, and nothing fails.
func remoteStoresStringsWithNUL(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	const id, host = "https://nul.example/users/alice", "nul.example"
	doc := func(name string) map[string]any {
		return map[string]any{
			"@context": "https://www.w3.org/ns/activitystreams", "id": id, "type": "Person",
			"preferredUsername": "ali\x00ce", "name": name, "summary": "<p>bio\x00</p>", "url": "https://nul.example/@alice\x00",
			"inbox": id + "/inbox", "outbox": id + "/outbox\x00", "followers": id + "/followers\x00", "following": id + "/following\x00",
			"featured": id + "/featured\x00", "endpoints": map[string]any{"sharedInbox": "https://nul.example/inbox\x00"},
			"icon":       map[string]any{"type": "Image", "url": "https://nul.example/avatar\x00.png"},
			"image":      map[string]any{"type": "Image", "url": "https://nul.example/header\x00.png"},
			"publicKey":  map[string]any{"id": id + "#main-key", "owner": id, "publicKeyPem": rotatedKeyPEM(t)},
			"attachment": []any{map[string]any{"type": "PropertyValue", "name": "Site\x00", "value": "<p>v\x00</p>"}},
			"tag":        []any{map[string]any{"type": "Emoji", "id": "https://nul.example/emojis/1\x00", "name": ":blob\x00:", "icon": map[string]any{"type": "Image", "url": "https://nul.example/e\x00.png"}}},
		}
	}
	raw, err := json.Marshal(doc("Alice\x00"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`\u0000`)) {
		t.Fatalf("the document %s has no \\u0000", raw)
	}
	e.net.set(id, remoteResponse{status: http.StatusOK, contentType: activityJSON, body: string(raw)})
	e.serveWebFinger(host, "ali\uFFFDce", host, id)
	a := e.resolve(t, id)
	if p := nulPath(reflect.ValueOf(a), "Actor"); p != "" {
		t.Errorf("%s has a NUL", p)
	}
	if a.Profile == nil || a.Profile.DisplayName != "Alice\uFFFD" || a.Profile.NoteHTML != "<p>bio\uFFFD</p>" ||
		!reflect.DeepEqual(a.Profile.Fields, []account.ProfileField{{Position: 0, Name: "Site\uFFFD", ValueHTML: "<p>v\uFFFD</p>"}}) {
		t.Errorf("profile = %+v, want every NUL as U+FFFD", a.Profile)
	}
	if a.Handle == nil || a.Handle.Username != "ali\uFFFDce" || len(a.Keys) != 1 {
		t.Errorf("handle = %+v, keys = %+v; want ali\uFFFDce and the key", a.Handle, a.Keys)
	}
	head, err := e.accounts.RemoteHead(ctx, a.Account.ID)
	if err != nil || head.SharedInboxURL != "https://nul.example/inbox%EF%BF%BD" {
		t.Errorf("head = %+v, %v; want the shared inbox with U+FFFD", head, err)
	}
	e.tx(t, func(tx uow.Tx) error {
		snap, err := e.h.Accounts.LatestRemoteSnapshot(ctx, tx, a.Account.ID)
		if err != nil || snap.PreferredUsername != "ali\uFFFDce" || snap.ProfileURL != "https://nul.example/@alice%EF%BF%BD" {
			t.Errorf("snapshot = %+v, %v; want every NUL as U+FFFD", snap, err)
		}
		return nil
	})

	// An Update with the document embedded stores the same way.
	e.clock.Advance(time.Minute)
	update, err := json.Marshal(map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams", "id": id + "#updates/1\x00", "type": "Update", "actor": id, "object": doc("Renamed\x00"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.actors.ReceiveActorUpdate(ctx, e.receivedActivity(t, update), update); err != nil {
		t.Fatalf("ReceiveActorUpdate: %v", err)
	}
	got, err := e.accounts.FindByID(ctx, a.Account.ID)
	if err != nil || got.Profile == nil || got.Profile.Seq != 2 || got.Profile.DisplayName != "Renamed\uFFFD" {
		t.Errorf("profile = %+v, %v; want revision 2 named Renamed\uFFFD", got.Profile, err)
	}
}

func remoteReceivesActorDeletes(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	id, _, _ := e.serveActor(fixtureActor(t, "gotosocial/0.22/actor"))
	a := e.resolve(t, id)

	del := []byte(`{"@context":"https://www.w3.org/ns/activitystreams","id":"` + id + `#delete","type":"Delete","actor":"` + id + `",` +
		`"object":"` + id + `","to":["https://www.w3.org/ns/activitystreams#Public"]}`)
	act := e.receivedActivity(t, del)
	if !appfederation.IsOwnActorActivity(act) {
		t.Fatalf("IsOwnActorActivity(%+v) = false", act)
	}
	for range 2 { // handling it again changes nothing
		if err := e.actors.ReceiveActorDelete(ctx, act); err != nil {
			t.Fatalf("ReceiveActorDelete: %v", err)
		}
	}
	got, err := e.accounts.FindByID(ctx, a.Account.ID)
	if err != nil || got.Lifecycle.State != account.StateDeleted || !got.Deleted() {
		t.Errorf("lifecycle = %+v, %v; want deleted", got.Lifecycle, err)
	}
	e.tx(t, func(tx uow.Tx) error {
		events, err := e.h.Accounts.ListLifecycleEvents(ctx, tx)
		var kinds []account.LifecycleKind
		for _, ev := range events {
			if ev.AccountID == a.Account.ID {
				kinds = append(kinds, ev.Kind)
			}
		}
		if err != nil || !reflect.DeepEqual(kinds, []account.LifecycleKind{account.LifecycleRemoteDeleted}) {
			t.Errorf("lifecycle events = %v, %v; want one remote_deleted", kinds, err)
		}
		return nil
	})

	// A later Update does not bring the actor back.
	update := renamedUpdate(t, "back again")
	if err := e.actors.ReceiveActorUpdate(ctx, e.receivedActivity(t, update), update); err != nil {
		t.Fatalf("ReceiveActorUpdate after the Delete: %v", err)
	}
	if got, _ := e.accounts.FindByID(ctx, a.Account.ID); got.Profile == nil || got.Profile.Seq != 1 || !got.Deleted() {
		t.Errorf("after the Update: profile %+v, lifecycle %+v; want revision 1, deleted", got.Profile, got.Lifecycle)
	}

	// Nor does a fetch of its document, which its server still serves.
	if got, err := e.actors.Refresh(ctx, a.Account.URI); err != nil || !got.Deleted() || got.Profile == nil || got.Profile.Seq != 1 {
		t.Errorf("Refresh after the Delete = %+v, %v; want the deleted actor as it was", got, err)
	}

	// The Delete of an actor this server never knew is a no-op.
	unknown := bytes.ReplaceAll(fixtureActor(t, "mastodon/circa-2018/delete-person"), []byte("http://mastodon.example.org"), []byte("https://mastodon.example.org"))
	if err := e.actors.ReceiveActorDelete(ctx, e.receivedActivity(t, unknown)); err != nil {
		t.Errorf("ReceiveActorDelete(unknown) = %v", err)
	}
	if _, err := e.accounts.FindByURI(ctx, "https://mastodon.example.org/users/admin"); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindByURI(unknown) = %v, want ErrNotFound", err)
	}
}

func remoteListsKnownSharedInboxes(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	ids := map[string]string{}
	for _, p := range []string{"mastodon/4.8/actor", "misskey/2025.4/actor", "gotosocial/0.22/actor"} {
		id, _, _ := e.serveActor(fixtureActor(t, p))
		ids[p] = e.resolve(t, id).Account.ID
	}
	list := func(after string, limit int, want ...string) {
		t.Helper()
		got, err := e.accounts.KnownSharedInboxes(ctx, after, limit)
		if err != nil || len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, want)) {
			t.Errorf("KnownSharedInboxes(%q, %d) = %v, %v; want %v", after, limit, got, err, want)
		}
	}
	// GoToSocial declares no shared inbox.
	list("", 10, "https://mastodon.example/inbox", "https://misskey.example/inbox")
	list("", 1, "https://mastodon.example/inbox")
	list("https://mastodon.example/inbox", 1, "https://misskey.example/inbox")
	list("https://misskey.example/inbox", 1)
	// The shared inbox of a deleted actor is not known any more.
	e.tx(t, func(tx uow.Tx) error { return e.accounts.MarkRemoteDeleted(ctx, tx, ids["misskey/2025.4/actor"]) })
	list("", 10, "https://mastodon.example/inbox")
}

// remoteResolvesHandlesAndURLs resolves an unknown handle through WebFinger and stores the actor,
// answers a known handle or URL from the database, and refuses what does not lead to an actor
// with a confirmed handle.
func remoteResolvesHandlesAndURLs(t *testing.T, e *remoteEnv) {
	ctx := t.Context()
	id, username, host := e.serveActor(fixtureActor(t, "mastodon/4.8/actor"))
	a, err := e.actors.ResolveHandle(ctx, username, strings.ToUpper(host))
	if err != nil {
		t.Fatalf("ResolveHandle(%s@%s): %v", username, host, err)
	}
	if a.Account.URI != id || a.Handle == nil || a.Handle.Username != username || a.Handle.Domain != host {
		t.Fatalf("ResolveHandle = %s with handle %+v, want %s as %s@%s", a.Account.URI, a.Handle, id, username, host)
	}
	if stored, err := e.accounts.FindByHandle(ctx, username, host); err != nil || stored.Account.ID != a.Account.ID {
		t.Fatalf("FindByHandle after resolving = %+v, %v", stored.Account, err)
	}
	// Known handles and URLs are answered without the network.
	before := len(e.net.requests)
	if again, err := e.actors.ResolveHandle(ctx, username, host); err != nil || again.Account.ID != a.Account.ID {
		t.Errorf("ResolveHandle again = %+v, %v", again.Account, err)
	}
	if again, err := e.actors.ResolveURL(ctx, id); err != nil || again.Account.ID != a.Account.ID {
		t.Errorf("ResolveURL(%s) = %+v, %v", id, again.Account, err)
	}
	if got := len(e.net.requests); got != before {
		t.Errorf("%d requests to resolve a known account, want none", got-before)
	}

	// An unknown URL is fetched and stored.
	gtsID, _, _ := e.serveActor(fixtureActor(t, "gotosocial/0.22/actor"))
	if g, err := e.actors.ResolveURL(ctx, gtsID); err != nil || g.Account.URI != gtsID || g.Handle == nil {
		t.Errorf("ResolveURL(%s) = %+v, %v", gtsID, g, err)
	}

	// An actor whose own handle WebFinger does not confirm.
	doc := fixtureActor(t, "misskey/2025.4/actor")
	var misskey struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(doc, &misskey); err != nil {
		t.Fatal(err)
	}
	e.net.set(misskey.ID, remoteResponse{status: http.StatusOK, contentType: activityJSON, body: string(doc)})
	mu, _ := url.Parse(misskey.ID)
	e.serveWebFinger(mu.Host, "alias", mu.Host, misskey.ID)

	for _, tc := range []struct{ name, username, domain string }{
		{"UnknownHandle", "nobody", host},
		{"UnconfirmedHandle", "alias", mu.Host},
		{"LocalDomain", "alice", contractHost},
		{"NotAHandle", "a/b", host},
		{"NotADomain", "bob", "bad domain"},
	} {
		if _, err := e.actors.ResolveHandle(ctx, tc.username, tc.domain); !errors.Is(err, appfederation.ErrUnresolvable) {
			t.Errorf("%s: ResolveHandle(%s@%s) = %v, want ErrUnresolvable", tc.name, tc.username, tc.domain, err)
		}
	}
	for _, uri := range []string{misskey.ID, "https://" + host + "/users/nobody", "http://" + host + "/users/x", "not a url"} {
		if _, err := e.actors.ResolveURL(ctx, uri); !errors.Is(err, appfederation.ErrUnresolvable) {
			t.Errorf("ResolveURL(%s) = %v, want ErrUnresolvable", uri, err)
		}
	}

	// The profile page of a resolved actor is its url.
	if u, err := e.accounts.RemoteProfileURL(ctx, a.Account.ID); err != nil || u != "https://mastodon.example/@alice" {
		t.Errorf("RemoteProfileURL = %q, %v; want the actor's url", u, err)
	}
	if _, err := e.accounts.RemoteProfileURL(ctx, e.instance.Account.ID); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("RemoteProfileURL of a local account = %v, want ErrNotFound", err)
	}
	// A page of accounts reads the profile pages together; local accounts have none.
	urls, err := e.accounts.RemoteProfileURLs(ctx, []string{a.Account.ID, e.instance.Account.ID, NamedID("unknown")})
	if want := map[string]string{a.Account.ID: "https://mastodon.example/@alice"}; err != nil || !maps.Equal(urls, want) {
		t.Errorf("RemoteProfileURLs = %v, %v; want %v", urls, err, want)
	}

	// WebFinger names an actor that holds another handle, names no actor, or does not answer.
	e.serveWebFinger(host, "mallory", host, id)
	e.net.set(webFingerURL(host, "acct:bob@"+host), remoteResponse{status: http.StatusOK, contentType: "application/jrd+json", body: `{"subject":"acct:bob@` + host + `"}`})
	e.net.set(webFingerURL(host, "acct:carol@"+host), remoteResponse{status: http.StatusServiceUnavailable})
	for _, name := range []string{"mallory", "bob", "carol"} {
		if _, err := e.actors.ResolveHandle(ctx, name, host); !errors.Is(err, appfederation.ErrUnresolvable) {
			t.Errorf("ResolveHandle(%s@%s) = %v, want ErrUnresolvable", name, host, err)
		}
	}

	// A handle on an account domain: the stored actor holds the handle on its server's domain
	// until it is checked again, and then holds the handle on example.org.
	e.serveWebFinger(host, username, "example.org", id)
	jrd, _ := json.Marshal(map[string]any{
		"subject": "acct:" + username + "@example.org",
		"links":   []map[string]string{{"rel": "self", "type": activityJSON, "href": id}},
	})
	e.net.set(webFingerURL("example.org", "acct:"+username+"@example.org"), remoteResponse{status: http.StatusOK, contentType: "application/jrd+json", body: string(jrd)})
	if c, err := e.actors.ResolveHandle(ctx, username, "Example.ORG"); err != nil || c.Account.ID != a.Account.ID ||
		c.Handle == nil || c.Handle.Domain != "example.org" {
		t.Errorf("ResolveHandle on the account domain = %s %+v, %v; want %s@example.org", c.Account.ID, c.Handle, err, username)
	}
}
