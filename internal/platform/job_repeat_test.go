// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/platform"
)

// TestInboxAnswersOnce processes received activities again, as a job does after a crash between
// its commit and its completion: the answers that this server sends go out once, whichever
// handler sends them.
func TestInboxAnswersOnce(t *testing.T) {
	t.Parallel()
	network := &relayNet{docs: map[string][]byte{}}
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline"}, platform.AppOptions{HTTPClient: network}, "alice")
	ctx := t.Context()
	carol := newRemoteActor(t, network, "https://remote.example/users/carol", "Person", "https://remote.example/users/carol/inbox")
	dave := newRemoteActor(t, network, "https://remote.example/users/dave", "Person", "https://remote.example/users/dave/inbox")
	erin := newRemoteActor(t, network, "https://remote.example/users/erin", "Person", "https://remote.example/users/erin/inbox")
	alice := "https://example.test/ap/actors/" + c.ids["alice"]
	instance, err := c.app.Accounts.EnsureInstanceActor(ctx)
	if err != nil {
		t.Fatal(err)
	}

	sent := func(actor remoteActor) []string {
		t.Helper()
		var types []string
		for _, p := range network.taken()[actor.id+"/inbox"] {
			types = append(types, p["type"].(string))
		}
		return types
	}
	// deliver hands the activity to the inbox, wants the answer, and processes it twice more.
	deliver := func(name string, signer remoteActor, activity map[string]any, want string) {
		t.Helper()
		if code := inboxCode(t, c, signer, activity); code != http.StatusAccepted {
			t.Fatalf("%s = %d", name, code)
		}
		if got := sent(signer); len(got) != 1 || got[0] != want {
			t.Fatalf("sent after %s = %v, want a %s", name, got, want)
		}
		for range 2 {
			if _, err := c.app.InboxProcessor.Process(ctx, federation.IRI(activity["id"].(string)), federation.TrustSigned); err != nil {
				t.Fatalf("Process %s again: %v", name, err)
			}
		}
		if got := sent(signer); len(got) != 0 {
			t.Errorf("sent after %s ran again = %v, want nothing", name, got)
		}
	}

	// A Follow of the instance actor by someone who is not a relay.
	deliver("Follow of the instance actor", carol, map[string]any{
		"id": carol.id + "/activities/instance", "type": "Follow", "actor": carol.id, "object": instance.URI,
	}, "Reject")

	// A QuoteRequest of a post whose quote policy approves it.
	st := c.post("alice", map[string]any{"text": "quote me", "visibility": "public"})
	network.taken() // the post's own deliveries
	deliver("QuoteRequest", dave, map[string]any{
		"id": dave.id + "/activities/quote", "type": "QuoteRequest", "actor": dave.id,
		"object": "https://example.test/ap/objects/" + st.Id, "instrument": dave.id + "/statuses/1",
	}, "Accept")

	// A Follow that the federation mode refuses.
	body := map[string]any{"values": map[string]any{"federation_mode": "inbound"}}
	if res := c.do("alice", http.MethodPatch, "/admin/settings", body, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /admin/settings = %d", res.StatusCode)
	}
	deliver("refused Follow", erin, map[string]any{
		"id": erin.id + "/activities/follow", "type": "Follow", "actor": erin.id, "object": alice,
	}, "Reject")
}

// TestMigrationRunsAgain runs the migration of a moved account again: a follower that moved stays
// moved and is told once, and a move that was cancelled moves nobody more.
func TestMigrationRunsAgain(t *testing.T) {
	t.Parallel()
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline"}, platform.AppOptions{}, "alice", "bob", "carol", "dan")
	ctx := t.Context()
	alice, bob, carol, dan := c.ids["alice"], c.ids["bob"], c.ids["carol"], c.ids["dan"]
	if _, err := c.app.Graph.Follow(ctx, carol, alice); err != nil {
		t.Fatal(err)
	}
	move, err := c.app.Accounts.RecordLocalMove(ctx, appaccount.LocalMoveInput{AccountID: alice, TargetAccountID: bob})
	if err != nil {
		t.Fatal(err)
	}
	following := func(follower, followee string) graph.State {
		t.Helper()
		r, err := c.app.Graph.Relationship(ctx, follower, followee)
		if err != nil {
			t.Fatal(err)
		}
		return r.Following
	}
	if err := c.app.Graph.MigrateFollows(ctx, move.ID, alice, bob); err != nil {
		t.Fatal(err)
	}
	if following(carol, bob) != graph.StateActive || following(carol, alice) != graph.StateNone {
		t.Fatalf("after the migration carol follows bob = %q, alice = %q", following(carol, bob), following(carol, alice))
	}
	if err := c.app.Graph.MigrateFollows(ctx, move.ID, alice, bob); err != nil {
		t.Fatal(err)
	}
	if following(carol, bob) != graph.StateActive || following(carol, alice) != graph.StateNone {
		t.Errorf("after running it again carol follows bob = %q, alice = %q", following(carol, bob), following(carol, alice))
	}

	// Once the move is cancelled, a follower of the account stays with it.
	if err := c.app.Accounts.CancelMove(ctx, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := c.app.Graph.Follow(ctx, dan, alice); err != nil {
		t.Fatal(err)
	}
	if err := c.app.Graph.MigrateFollows(ctx, move.ID, alice, bob); err != nil {
		t.Fatal(err)
	}
	if following(dan, alice) != graph.StateActive || following(dan, bob) != graph.StateNone {
		t.Errorf("after running a cancelled move dan follows alice = %q, bob = %q", following(dan, alice), following(dan, bob))
	}
}
