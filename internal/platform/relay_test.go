// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/httpsig"
	"github.com/mycroblog/my/internal/adapter/jsonld"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	domainaccount "github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// relayNet is a kernel.HTTPClient that serves documents by URL, answers every POST with 202, and
// keeps the POSTs.
type relayNet struct {
	mu    sync.Mutex
	docs  map[string][]byte
	posts []recordedPost
}

func (n *relayNet) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	status, body, h := http.StatusNotFound, []byte(nil), http.Header{}
	switch {
	case req.Method == http.MethodPost:
		status = http.StatusAccepted
		n.posts = append(n.posts, recordedPost{url: req.URL, header: req.Header.Clone(), body: slices.Clone(req.Body)})
	case n.docs[req.URL] != nil:
		status, body = http.StatusOK, n.docs[req.URL]
		h.Set("Content-Type", "application/activity+json")
	}
	return &kernel.HTTPResponse{StatusCode: status, Header: h, URL: req.URL, Body: io.NopCloser(bytes.NewReader(body))}, nil
}

func (n *relayNet) serve(t *testing.T, id string, doc map[string]any) {
	t.Helper()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.docs[id] = b
}

// taken returns the POSTs since the last call, by inbox.
func (n *relayNet) taken() map[string][]map[string]any {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := map[string][]map[string]any{}
	for _, p := range n.posts {
		var doc map[string]any
		_ = json.Unmarshal(p.body, &doc)
		doc["_raw"] = string(p.body)
		out[p.url] = append(out[p.url], doc)
	}
	n.posts = nil
	return out
}

// remoteActor is an actor of the fake network with its key.
type remoteActor struct {
	id  string
	key *rsa.PrivateKey
}

func newRemoteActor(t *testing.T, n *relayNet, id, typ, inbox string) remoteActor {
	t.Helper()
	return newRemoteActorWith(t, n, id, typ, func(doc map[string]any) { doc["inbox"] = inbox })
}

// newRemoteActorWith serves an actor whose document edit completes, and returns it with its key.
func newRemoteActorWith(t *testing.T, n *relayNet, id, typ string, edit func(doc map[string]any)) remoteActor {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{
		"@context": []string{"https://www.w3.org/ns/activitystreams", "https://w3id.org/security/v1"},
		"id":       id, "type": typ, "preferredUsername": "relay",
		"followers": id + "/followers", "following": id + "/following",
		"publicKey": map[string]any{
			"id": id + "#main-key", "owner": id,
			"publicKeyPem": string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})),
		},
	}
	edit(doc)
	n.serve(t, id, doc)
	return remoteActor{id: id, key: key}
}

// postInbox POSTs activity to the shared inbox signed by signer, and expects 202.
func postInbox(t *testing.T, c *statusesClient, signer remoteActor, activity map[string]any) {
	t.Helper()
	if _, ok := activity["@context"]; !ok {
		activity["@context"] = "https://www.w3.org/ns/activitystreams"
	}
	body, err := json.Marshal(activity)
	if err != nil {
		t.Fatal(err)
	}
	const host = "example.test"
	req := &kernel.HTTPRequest{
		Method: http.MethodPost, URL: "https://" + host + "/ap/inbox", Body: body,
		Header: http.Header{"Date": {time.Now().UTC().Format(http.TimeFormat)}, "Host": {host}},
	}
	err = httpsig.New().Sign(req, appfederation.SigningKey{KeyID: signer.id + "#main-key", PrivateKey: signer.key}, appfederation.SignOptions{
		Scheme: appfederation.SchemeCavage, Algorithm: appfederation.AlgRSAPKCS1SHA256,
		Components: []string{"(request-target)", "host", "date", "digest"},
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	hr := httptest.NewRequest(http.MethodPost, "https://"+host+"/ap/inbox", bytes.NewReader(body))
	for k, v := range req.Header {
		if k != "Host" {
			hr.Header[k] = v
		}
	}
	hr.Header.Set("Content-Type", "application/activity+json")
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, hr)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST %v = %d %s, want 202", activity["id"], rec.Code, rec.Body)
	}
}

// TestRelays joins a Mastodon-style relay and a LitePub one through the admin API with the inline
// queue (ADR-0061): the instance actor's Follow, the relay's Accept, public posts to the relay
// with their linked data signature (and nothing else), the posts the relay forwards fetched from
// their origin into the federated timeline within the hourly limit, the LitePub relay's
// Announce, the Undo that leaves, the deletion, and the audit log.
func TestRelays(t *testing.T) {
	t.Parallel()
	network := &relayNet{docs: map[string][]byte{}}
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline", "MY_RELAY_INGEST_HOURLY_LIMIT": "2"},
		platform.AppOptions{HTTPClient: network}, "alice")
	ctx := t.Context()
	relay := newRemoteActor(t, network, "https://relay.example/actor", "Application", "https://relay.example/inbox")
	carol := newRemoteActor(t, network, "https://remote.example/users/carol", "Person", "https://remote.example/users/carol/inbox")

	// Register and join the relay at its inbox.
	var r clientapi.AdminRelay
	if res := c.do("alice", http.MethodPost, "/admin/relays", map[string]any{"url": "https://relay.example/inbox"}, &r); res.StatusCode != http.StatusOK {
		t.Fatalf("POST /admin/relays = %d", res.StatusCode)
	}
	if r.Protocol != clientapi.RelayProtocolMastodon || r.State != clientapi.RelayStatePending || !r.Receive || r.InboxUrl != "https://relay.example/inbox" {
		t.Fatalf("relay = %+v, want a pending mastodon relay that receives", r)
	}
	posts := network.taken()["https://relay.example/inbox"]
	if len(posts) != 1 || posts[0]["type"] != "Follow" || posts[0]["object"] != "https://www.w3.org/ns/activitystreams#Public" {
		t.Fatalf("POSTs to the relay = %v, want its Follow of Public", posts)
	}
	follow := posts[0]["id"].(string)
	instance := posts[0]["actor"].(string)
	if res := c.do("alice", http.MethodPost, "/admin/relays", map[string]any{"url": "https://relay.example/inbox"}, nil); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("registering the relay again = %d, want 422", res.StatusCode)
	}

	// The relay accepts; an Accept of something else is not about it.
	postInbox(t, c, relay, map[string]any{
		"id": "https://relay.example/activities/accept-1", "type": "Accept", "actor": relay.id,
		"object": map[string]any{"id": follow, "type": "Follow", "actor": instance, "object": "https://www.w3.org/ns/activitystreams#Public"},
	})
	var list clientapi.AdminRelayList
	if res := c.do("alice", http.MethodGet, "/admin/relays", nil, &list); res.StatusCode != http.StatusOK || len(list.Items) != 1 {
		t.Fatalf("GET /admin/relays = %d %+v", res.StatusCode, list)
	}
	if got := list.Items[0]; got.State != clientapi.RelayStateAccepted || got.ActorUri == nil || *got.ActorUri != relay.id {
		t.Fatalf("relay after the Accept = %+v, want accepted by its actor", got)
	}

	// A public post goes to the relay with the author's linked data signature; an unlisted one
	// does not.
	public := c.post("alice", map[string]any{"text": "hello relays #relay", "visibility": "public"})
	unlisted := c.post("alice", map[string]any{"text": "not for relays", "visibility": "unlisted"})
	posts = network.taken()["https://relay.example/inbox"]
	if len(posts) != 1 || posts[0]["type"] != "Create" {
		t.Fatalf("POSTs to the relay = %v, want the Create of the public post alone", posts)
	}
	if obj, _ := posts[0]["object"].(map[string]any); obj["id"] == nil || strings.Contains(posts[0]["_raw"].(string), unlisted.Id) {
		t.Fatalf("Create = %v, want the public post", posts[0])
	}
	sig, _ := posts[0]["signature"].(map[string]any)
	if sig["type"] != "RsaSignature2017" || posts[0]["proof"] != nil {
		t.Fatalf("Create signature = %v, proof = %v; want an RsaSignature2017 and no proof", sig, posts[0]["proof"])
	}
	alice, err := c.app.Accounts.FindByID(ctx, c.ids["alice"])
	if err != nil {
		t.Fatal(err)
	}
	pub := alicePublicKey(t, alice.Keys)
	canon, err := jsonld.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := appfederation.VerifyLinkedData([]byte(posts[0]["_raw"].(string)), canon, pub); err != nil {
		t.Errorf("VerifyLinkedData(Create) = %v", err)
	}
	_ = public

	// The relay forwards public posts of carol, signed by itself: they are fetched from their
	// origin and come into the federated timeline, two per hour; an unlisted one does not.
	forward := func(n int, to string) string {
		id := carol.id + "/statuses/" + strconv.Itoa(n)
		network.serve(t, id, map[string]any{
			"@context": "https://www.w3.org/ns/activitystreams",
			"id":       id, "type": "Note", "attributedTo": carol.id, "content": "<p>relayed " + id + "</p>",
			"published": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), "to": []string{to}, "cc": []string{carol.id + "/followers"},
		})
		postInbox(t, c, relay, map[string]any{
			"id": id + "/activity", "type": "Create", "actor": carol.id, "to": []string{to},
			"object": map[string]any{"id": id, "type": "Note", "attributedTo": carol.id, "content": "forged"},
		})
		return id
	}
	first := forward(1, "https://www.w3.org/ns/activitystreams#Public")
	unlistedNote := forward(2, carol.id+"/followers")
	overLimit := forward(3, "https://www.w3.org/ns/activitystreams#Public")
	st, err := c.app.Content.FindStatusByURI(ctx, first)
	if err != nil {
		t.Fatalf("the relayed post is not stored: %v", err)
	}
	if p, err := c.app.Content.Find(ctx, st.ID); err != nil || !strings.Contains(p.Revision.Bodies[0].ContentHTML, "relayed") {
		t.Errorf("relayed post = %+v, %v; want the document of its origin", p, err)
	}
	for _, id := range []string{unlistedNote, overLimit} {
		if _, err := c.app.Content.FindStatusByURI(ctx, id); err == nil {
			t.Errorf("%s is stored, want it dropped", id)
		}
	}
	var timeline clientapi.StatusPage
	if res := c.do("alice", http.MethodGet, "/timelines/public", nil, &timeline); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /timelines/public = %d", res.StatusCode)
	}
	if !slices.ContainsFunc(timeline.Items, func(s clientapi.Status) bool { return s.Id == st.ID }) {
		t.Errorf("the federated timeline does not show the relayed post")
	}
	if res := c.do("alice", http.MethodGet, "/admin/relays", nil, &list); res.StatusCode != http.StatusOK ||
		list.Items[0].TakenLastDay != 2 || list.Items[0].DroppedLastDay != 1 {
		t.Errorf("relay counts = %+v, want 2 taken and 1 dropped", list.Items)
	}

	// A LitePub relay: its actor is fetched for its inbox, and its Announce brings the post in
	// without a boost. Its actor has a shared inbox and no inbox of its own, as Akkoma's relay
	// actor has, and its signatures verify with its key all the same.
	lite := newRemoteActorWith(t, network, "https://litepub.example/relay", "Application", func(doc map[string]any) {
		doc["inbox"] = nil
		doc["endpoints"] = map[string]any{"sharedInbox": "https://litepub.example/inbox"}
	})
	var lr clientapi.AdminRelay
	if res := c.do("alice", http.MethodPost, "/admin/relays", map[string]any{"url": lite.id}, &lr); res.StatusCode != http.StatusOK {
		t.Fatalf("POST /admin/relays (litepub) = %d", res.StatusCode)
	}
	if lr.Protocol != clientapi.RelayProtocolLitepub || lr.InboxUrl != "https://litepub.example/inbox" {
		t.Fatalf("litepub relay = %+v", lr)
	}
	posts = network.taken()["https://litepub.example/inbox"]
	if len(posts) != 1 || posts[0]["object"] != lite.id {
		t.Fatalf("POSTs to the LitePub relay = %v, want its Follow of the relay actor", posts)
	}
	postInbox(t, c, lite, map[string]any{"id": lite.id + "/accept", "type": "Accept", "actor": lite.id, "object": posts[0]["id"]})
	// The relay follows the instance actor back, and gets an Accept.
	postInbox(t, c, lite, map[string]any{"id": lite.id + "/follow", "type": "Follow", "actor": lite.id, "object": instance})
	if got := network.taken()["https://litepub.example/inbox"]; len(got) != 1 || got[0]["type"] != "Accept" {
		t.Errorf("answer to the LitePub relay's Follow = %v, want an Accept", got)
	}
	announced := carol.id + "/statuses/9"
	network.serve(t, announced, map[string]any{
		"id": announced, "type": "Note", "attributedTo": carol.id, "content": "<p>announced</p>",
		"published": time.Now().UTC().Format(time.RFC3339), "to": []string{"https://www.w3.org/ns/activitystreams#Public"},
	})
	postInbox(t, c, lite, map[string]any{
		"id": lite.id + "/announce/1", "type": "Announce", "actor": lite.id, "object": announced,
		"to": []string{lite.id + "/followers"},
	})
	ast, err := c.app.Content.FindStatusByURI(ctx, announced)
	if err != nil {
		t.Fatalf("the announced post is not stored: %v", err)
	}
	var shown clientapi.Status
	if res := c.do("alice", http.MethodGet, "/statuses/"+ast.ID, nil, &shown); res.StatusCode != http.StatusOK || shown.BoostsCount != 0 {
		t.Errorf("GET the announced post = %d %+v, want it without a boost", res.StatusCode, shown.BoostsCount)
	}

	// Leaving the Mastodon relay sends the Undo; what it forwards afterwards is dropped.
	if res := c.do("alice", http.MethodPost, "/admin/relays/"+r.Id+"/unfollow", nil, &r); res.StatusCode != http.StatusOK || r.State != clientapi.RelayStateLeft {
		t.Fatalf("unfollow = %d %+v", res.StatusCode, r)
	}
	posts = network.taken()["https://relay.example/inbox"]
	if len(posts) != 1 || posts[0]["type"] != "Undo" {
		t.Fatalf("POSTs to the relay = %v, want the Undo", posts)
	}
	if inner, _ := posts[0]["object"].(map[string]any); inner["id"] != follow {
		t.Errorf("Undo object = %v, want the Follow %s", posts[0]["object"], follow)
	}
	late := forward(4, "https://www.w3.org/ns/activitystreams#Public")
	if _, err := c.app.Content.FindStatusByURI(ctx, late); err == nil {
		t.Errorf("a post forwarded after leaving is stored")
	}
	if res := c.do("alice", http.MethodPost, "/admin/relays/"+r.Id+"/unfollow", nil, nil); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("leaving a left relay = %d, want 422", res.StatusCode)
	}
	// A pending or accepted relay cannot be deleted; a left one can.
	if res := c.do("alice", http.MethodDelete, "/admin/relays/"+lr.Id, nil, nil); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("deleting an accepted relay = %d, want 422", res.StatusCode)
	}
	if res := c.do("alice", http.MethodDelete, "/admin/relays/"+r.Id, nil, nil); res.StatusCode != http.StatusNoContent {
		t.Errorf("deleting a left relay = %d, want 204", res.StatusCode)
	}

	// Every operation is in the audit log, the Accepts as the remote relay's.
	entries, err := c.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetRelay}, 50)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[moderation.AuditAction]int{}
	for _, e := range entries {
		counts[e.Action]++
		if e.Action == moderation.AuditRelayAccepted && e.ActorKind != moderation.AuditActorRemote {
			t.Errorf("relay_accepted by %s, want remote", e.ActorKind)
		}
	}
	want := map[moderation.AuditAction]int{
		moderation.AuditRelayAdded: 2, moderation.AuditRelayFollowed: 2, moderation.AuditRelayAccepted: 2,
		moderation.AuditRelayUnfollowed: 1, moderation.AuditRelayRemoved: 1,
	}
	for a, n := range want {
		if counts[a] != n {
			t.Errorf("audit entries %s = %d, want %d (all: %v)", a, counts[a], n, counts)
		}
	}
}

// alicePublicKey returns the RSA public key among a local actor's keys.
func alicePublicKey(t *testing.T, keys []domainaccount.Key) *rsa.PublicKey {
	t.Helper()
	for _, k := range keys {
		block, _ := pem.Decode([]byte(k.PublicKeyPEM))
		if block == nil {
			continue
		}
		if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
			if r, ok := pub.(*rsa.PublicKey); ok {
				return r
			}
		}
	}
	t.Fatal("no RSA key")
	return nil
}
