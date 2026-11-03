// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/httpsig"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// inboxCode POSTs activity to the shared inbox signed by signer and returns the status.
func inboxCode(t *testing.T, c *statusesClient, signer remoteActor, activity map[string]any) int {
	t.Helper()
	activity["@context"] = "https://www.w3.org/ns/activitystreams"
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
	return rec.Code
}

// apGet GETs path of the federation endpoints as a remote server would, unsigned, and returns the
// status and the body.
func apGet(t *testing.T, c *statusesClient, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "https://example.test"+path, nil)
	req.Header.Set("Accept", "application/activity+json")
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	var doc map[string]any
	body, _ := io.ReadAll(rec.Body)
	_ = json.Unmarshal(body, &doc)
	return rec.Code, doc
}

// TestFederationMode switches the federation mode (FR-413, ADR-0076) through the admin API and
// checks each entry point in the four modes with the inline queue: the inbox (synchronous and
// asynchronous), the fan-out and the delivery, the GETs of the actors, the statuses, and the
// collections, WebFinger, host-meta, and NodeInfo. Narrowing the mode and widening it again keeps
// the follows, and only what is posted after it is delivered; `my admin federation sever` ends
// the follows in the direction given and sends the Reject only where the mode sends it.
func TestFederationMode(t *testing.T) {
	t.Parallel()
	network := &relayNet{docs: map[string][]byte{}}
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline"}, platform.AppOptions{HTTPClient: network}, "alice")
	ctx := t.Context()
	carol := newRemoteActor(t, network, "https://remote.example/users/carol", "Person", "https://remote.example/users/carol/inbox")
	dave := newRemoteActor(t, network, "https://remote.example/users/dave", "Person", "https://remote.example/users/dave/inbox")
	aliceID := c.ids["alice"]
	alice := "https://example.test/ap/actors/" + aliceID
	instance, err := c.app.Accounts.EnsureInstanceActor(ctx)
	if err != nil {
		t.Fatal(err)
	}

	setMode := func(mode string) {
		t.Helper()
		body := map[string]any{"values": map[string]any{"federation_mode": mode}}
		if res := c.do("alice", http.MethodPatch, "/admin/settings", body, nil); res.StatusCode != http.StatusOK {
			t.Fatalf("PATCH /admin/settings federation_mode=%s = %d", mode, res.StatusCode)
		}
	}
	activities := 0
	activity := func(typ string, actor remoteActor, object any) map[string]any {
		activities++
		return map[string]any{"id": actor.id + "/activities/" + strconv.Itoa(activities), "type": typ, "actor": actor.id, "object": object}
	}
	mention := func(n int) (map[string]any, string) {
		id := carol.id + "/statuses/" + strconv.Itoa(n)
		note := map[string]any{
			"id": id, "type": "Note", "attributedTo": carol.id, "published": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
			"to": []string{"https://www.w3.org/ns/activitystreams#Public"}, "cc": []string{alice},
			"content": "<p>hi @alice</p>", "tag": []any{map[string]any{"type": "Mention", "href": alice, "name": "@alice@example.test"}},
		}
		network.serve(t, id, note)
		return note, id
	}
	sent := func(inbox string) []string {
		t.Helper()
		var types []string
		for _, p := range network.taken()[inbox] {
			types = append(types, p["type"].(string))
		}
		return types
	}
	relationship := func(remote remoteActor) (following, followedBy graph.State) {
		t.Helper()
		a, err := c.app.Accounts.FindByURI(ctx, remote.id)
		if err != nil {
			t.Fatalf("FindByURI(%s): %v", remote.id, err)
		}
		r, err := c.app.Graph.Relationship(ctx, aliceID, a.Account.ID)
		if err != nil {
			t.Fatal(err)
		}
		return r.Following, r.FollowedBy
	}

	// both: carol follows alice and gets the Accept; alice's posts reach carol.
	if code := inboxCode(t, c, carol, activity("Follow", carol, alice)); code != http.StatusAccepted {
		t.Fatalf("Follow in both = %d", code)
	}
	if got := sent(carol.id + "/inbox"); len(got) != 1 || got[0] != "Accept" {
		t.Fatalf("sent to carol after her Follow = %v, want an Accept", got)
	}
	c.post("alice", map[string]any{"text": "both ways", "visibility": "public"})
	if got := sent(carol.id + "/inbox"); len(got) != 1 || got[0] != "Create" {
		t.Fatalf("sent to carol in both = %v, want a Create", got)
	}

	// A relay that the instance joined: inbound sends it nothing and takes in what it relays,
	// outbound sends it the public posts and drops what it relays, and off does neither.
	const relayInbox = "https://relay.example/inbox"
	relay := newRemoteActor(t, network, "https://relay.example/actor", "Application", relayInbox)
	if res := c.do("alice", http.MethodPost, "/admin/relays", map[string]any{"url": relayInbox, "protocol": "mastodon"}, nil); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /admin/relays = %d", res.StatusCode)
	}
	joins := network.taken()[relayInbox]
	if len(joins) != 1 || joins[0]["type"] != "Follow" {
		t.Fatalf("sent to the relay = %v, want its Follow", joins)
	}
	if code := inboxCode(t, c, relay, map[string]any{
		"id": "https://relay.example/activities/accept", "type": "Accept", "actor": relay.id,
		"object": map[string]any{"id": joins[0]["id"], "type": "Follow", "actor": joins[0]["actor"], "object": "https://www.w3.org/ns/activitystreams#Public"},
	}); code != http.StatusAccepted {
		t.Fatalf("Accept of the relay = %d", code)
	}
	relayed := func(n int) string {
		t.Helper()
		id := carol.id + "/relayed/" + strconv.Itoa(n)
		network.serve(t, id, map[string]any{
			"@context": "https://www.w3.org/ns/activitystreams", "id": id, "type": "Note", "attributedTo": carol.id,
			"content": "<p>relayed</p>", "published": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
			"to": []string{"https://www.w3.org/ns/activitystreams#Public"},
		})
		if code := inboxCode(t, c, relay, map[string]any{
			"id": id + "/activity", "type": "Create", "actor": carol.id, "to": []string{"https://www.w3.org/ns/activitystreams#Public"},
			"object": map[string]any{"id": id, "type": "Note", "attributedTo": carol.id, "content": "forged"},
		}); code != http.StatusAccepted {
			t.Fatalf("relayed Create = %d", code)
		}
		_, err := c.app.Content.FindStatusByURI(ctx, id)
		return map[bool]string{true: "taken", false: "dropped"}[err == nil]
	}
	for i, tc := range []struct {
		mode, taken string
		sent        int
	}{{"inbound", "taken", 0}, {"outbound", "dropped", 1}, {"both", "taken", 1}} {
		setMode(tc.mode)
		c.post("alice", map[string]any{"text": "to the relay in " + tc.mode, "visibility": "public"})
		if got := sent(relayInbox); len(got) != tc.sent {
			t.Errorf("%s: sent to the relay = %v, want %d", tc.mode, got, tc.sent)
		}
		if got := relayed(i); got != tc.taken {
			t.Errorf("%s: the relayed post is %s, want %s", tc.mode, got, tc.taken)
		}
	}

	// GETs in every mode: the status code of each endpoint, and NodeInfo's federationMode.
	public := c.post("alice", map[string]any{"text": "a public post", "visibility": "public"})
	network.taken()
	gets := []struct {
		name, path string
		// off inbound outbound both
		want [4]int
	}{
		{"actor", "/ap/actors/" + aliceID, [4]int{404, 200, 200, 200}},
		{"instance actor", "/ap/actors/" + instance.ID, [4]int{404, 200, 200, 200}},
		{"status", "/ap/objects/" + public.Id, [4]int{404, 404, 200, 200}},
		{"activity", "/ap/objects/" + public.Id + "/activity", [4]int{404, 404, 200, 200}},
		{"outbox", "/ap/actors/" + aliceID + "/outbox", [4]int{404, 404, 200, 200}},
		{"followers", "/ap/actors/" + aliceID + "/followers", [4]int{404, 404, 200, 200}},
		{"featured", "/ap/actors/" + aliceID + "/featured", [4]int{404, 404, 200, 200}},
		{"webfinger", "/.well-known/webfinger?resource=acct:alice@example.test", [4]int{404, 200, 200, 200}},
		{"host-meta", "/.well-known/host-meta", [4]int{404, 200, 200, 200}},
		{"nodeinfo index", "/.well-known/nodeinfo", [4]int{404, 200, 200, 200}},
		{"nodeinfo", "/nodeinfo/2.1", [4]int{404, 200, 200, 200}},
	}
	for i, mode := range []string{"off", "inbound", "outbound", "both"} {
		setMode(mode)
		for _, g := range gets {
			code, doc := apGet(t, c, g.path)
			if code != g.want[i] {
				t.Errorf("%s: GET %s = %d, want %d", mode, g.name, code, g.want[i])
			}
			if g.name == "nodeinfo" && code == http.StatusOK {
				if meta, _ := doc["metadata"].(map[string]any); meta["federationMode"] != mode {
					t.Errorf("%s: NodeInfo metadata.federationMode = %v", mode, meta["federationMode"])
				}
			}
			if g.name == "actor" && code == http.StatusOK {
				// inbound serves the minimal document that verifying signatures needs.
				minimal := doc["outbox"] == nil && doc["name"] == nil
				if doc["publicKey"] == nil || doc["inbox"] == nil || minimal != (mode == "inbound") {
					t.Errorf("%s: actor document = %v, minimal = %v", mode, doc, minimal)
				}
			}
		}
	}

	// inbound: nothing of alice goes out; remote posts that mention her come in; a new Follow
	// gets a Reject and no request is left pending.
	setMode("inbound")
	c.post("alice", map[string]any{"text": "inbound only", "visibility": "public"})
	if got := sent(carol.id + "/inbox"); len(got) != 0 {
		t.Errorf("sent to carol in inbound = %v, want nothing", got)
	}
	note, noteID := mention(1)
	if code := inboxCode(t, c, carol, activity("Create", carol, note)); code != http.StatusAccepted {
		t.Fatalf("Create in inbound = %d", code)
	}
	if _, err := c.app.Content.FindStatusByURI(ctx, noteID); err != nil {
		t.Errorf("the mention is not taken in in inbound: %v", err)
	}
	if code := inboxCode(t, c, dave, activity("Follow", dave, alice)); code != http.StatusAccepted {
		t.Fatalf("Follow in inbound = %d", code)
	}
	if got := sent(dave.id + "/inbox"); len(got) != 1 || got[0] != "Reject" {
		t.Errorf("sent to dave after his Follow in inbound = %v, want a Reject", got)
	}
	if _, followedBy := relationship(dave); followedBy != graph.StateNone {
		t.Errorf("dave's follow of alice in inbound = %s, want none", followedBy)
	}

	// outbound: alice's posts go out; remote posts do not come in.
	setMode("outbound")
	c.post("alice", map[string]any{"text": "outbound only", "visibility": "public"})
	if got := sent(carol.id + "/inbox"); len(got) != 1 || got[0] != "Create" {
		t.Errorf("sent to carol in outbound = %v, want a Create", got)
	}
	note, noteID = mention(2)
	if code := inboxCode(t, c, carol, activity("Create", carol, note)); code != http.StatusAccepted {
		t.Fatalf("Create in outbound = %d", code)
	}
	if _, err := c.app.Content.FindStatusByURI(ctx, noteID); err == nil {
		t.Errorf("the mention is taken in in outbound, want it dropped")
	}

	// off: the inbox refuses before the signature, and nothing goes out.
	setMode("off")
	note, _ = mention(3)
	if code := inboxCode(t, c, carol, activity("Create", carol, note)); code != http.StatusForbidden {
		t.Errorf("Create in off = %d, want 403", code)
	}
	c.post("alice", map[string]any{"text": "off", "visibility": "public"})
	if got := sent(carol.id + "/inbox"); len(got) != 0 {
		t.Errorf("sent to carol in off = %v, want nothing", got)
	}

	// both again: carol still follows alice, and only what is posted now reaches her.
	setMode("both")
	if _, followedBy := relationship(carol); followedBy != graph.StateActive {
		t.Fatalf("carol's follow of alice after the modes = %s, want active", followedBy)
	}
	c.post("alice", map[string]any{"text": "both again", "visibility": "public"})
	if got := sent(carol.id + "/inbox"); len(got) != 1 || got[0] != "Create" {
		t.Errorf("sent to carol in both again = %v, want one Create", got)
	}

	// alice follows dave, then the operator severs in off (nothing is sent) and in outbound (the
	// Reject of carol's follow is sent).
	var dv clientapi.Account
	if res := c.do("alice", http.MethodGet, "/accounts/lookup?acct=dave@remote.example", nil, &dv); res.StatusCode != http.StatusOK {
		a, err := c.app.Accounts.FindByURI(ctx, dave.id)
		if err != nil {
			t.Fatal(err)
		}
		dv.Id = a.Account.ID
	}
	if res := c.do("alice", http.MethodPost, "/accounts/"+dv.Id+"/follow", map[string]any{}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("POST follow dave = %d", res.StatusCode)
	}
	if got := sent(dave.id + "/inbox"); len(got) != 1 || got[0] != "Follow" {
		t.Fatalf("sent to dave after alice's follow = %v, want a Follow", got)
	}
	setMode("off")
	if severed, err := c.app.FederationSever.Sever(ctx, appfederation.SeverOut); err != nil || severed.Following != 0 {
		// dave never accepted: the pending follow ends without counting.
		t.Fatalf("Sever(out) in off = %+v, %v", severed, err)
	}
	if following, _ := relationship(dave); following != graph.StateNone {
		t.Errorf("alice's follow of dave after Sever(out) = %s, want none", following)
	}
	if _, followedBy := relationship(carol); followedBy != graph.StateActive {
		t.Errorf("carol's follow of alice after Sever(out) = %s, want active", followedBy)
	}
	setMode("outbound")
	if severed, err := c.app.FederationSever.Sever(ctx, appfederation.SeverIn); err != nil || severed.Followers != 1 {
		t.Fatalf("Sever(in) in outbound = %+v, %v; want one follower", severed, err)
	}
	if _, followedBy := relationship(carol); followedBy != graph.StateNone {
		t.Errorf("carol's follow of alice after Sever(in) = %s, want none", followedBy)
	}
	if got := sent(carol.id + "/inbox"); len(got) != 1 || got[0] != "Reject" {
		t.Errorf("sent to carol after Sever(in) in outbound = %v, want a Reject", got)
	}
	if got := sent(dave.id + "/inbox"); len(got) != 0 {
		t.Errorf("sent to dave after Sever(out) in off = %v, want nothing", got)
	}
	if _, err := c.app.FederationSever.Sever(ctx, "sideways"); err == nil {
		t.Error("Sever(sideways) succeeded")
	}
}

// TestAnnounceDereference checks that the id of a boost's Announce serves the activity that its
// fan-out delivered, until the boost is retracted, and that other activity ids stay 404
// (ADR-0144 §5).
func TestAnnounceDereference(t *testing.T) {
	t.Parallel()
	network := &relayNet{docs: map[string][]byte{}}
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline"}, platform.AppOptions{HTTPClient: network}, "alice", "bob")
	ctx := t.Context()
	dave := newRemoteActor(t, network, "https://remote.example/users/dave", "Person", "https://remote.example/users/dave/inbox")
	bob := "https://example.test/ap/actors/" + c.ids["bob"]
	if code := inboxCode(t, c, dave, map[string]any{
		"id": dave.id + "/activities/follow", "type": "Follow", "actor": dave.id, "object": bob,
	}); code != http.StatusAccepted {
		t.Fatalf("Follow = %d", code)
	}
	st := c.post("alice", map[string]any{"text": "boost me", "visibility": "public"})
	network.taken()

	boost, err := c.app.Interactions.Boost(ctx, c.ids["bob"], st.Id)
	if err != nil {
		t.Fatal(err)
	}
	var delivered map[string]any
	for _, p := range network.taken()[dave.id+"/inbox"] {
		if p["type"] == "Announce" {
			delivered = p
		}
	}
	if delivered == nil {
		t.Fatal("the Announce was not delivered")
	}
	delete(delivered, "_raw")
	// The delivered document adds its integrity proof and that proof's context.
	delete(delivered, "proof")
	delete(delivered, "@context")
	path := strings.TrimPrefix(boost.URI, "https://example.test")
	code, served := apGet(t, c, path)
	if code != http.StatusOK || served["@context"] == nil {
		t.Errorf("GET the Announce = %d %v", code, served)
	}
	delete(served, "@context")
	if len(served) != len(delivered) {
		t.Errorf("GET the Announce has %d properties, the delivered one %d", len(served), len(delivered))
	}
	for k, want := range delivered {
		if got := served[k]; !reflect.DeepEqual(got, want) {
			t.Errorf("GET the Announce: %s = %v, delivered %v", k, got, want)
		}
	}

	fav, err := c.app.Interactions.Favourite(ctx, c.ids["bob"], st.Id)
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := apGet(t, c, strings.TrimPrefix(fav.URI, "https://example.test")); code != http.StatusNotFound {
		t.Errorf("GET the Like = %d, want 404", code)
	}

	if err := c.app.Interactions.Unboost(ctx, c.ids["bob"], st.Id); err != nil {
		t.Fatal(err)
	}
	if code, _ := apGet(t, c, path); code != http.StatusNotFound {
		t.Errorf("GET a retracted Announce = %d, want 404", code)
	}
}
