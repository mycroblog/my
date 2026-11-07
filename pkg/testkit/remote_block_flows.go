// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/graph"
)

// remoteUser stores a remote actor and returns its account ID.
func (e *followEnv) remoteUser(t *testing.T, username string) string {
	t.Helper()
	uri := "https://remote.example/users/" + username
	remote := appaccount.NewService(appaccount.Deps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Accounts, Clock: e.clock, IDs: e.ids,
		Sealer: kernel.NewSealer(NewKeyManager("remote-block")), Host: contractHost, Sanitizer: remoteSanitizer{},
	})
	a, err := remote.SaveRemoteActor(t.Context(), appaccount.RemoteActorInput{
		URI: uri, Domain: "remote.example", Type: "Person", PreferredUsername: username, InboxURL: uri + "/inbox",
		Document: []byte(`{"id":"` + uri + `"}`),
		Handle:   appaccount.HandleCheck{Outcome: account.HandleVerified, Username: username, Domain: "remote.example"},
	})
	if err != nil {
		t.Fatalf("SaveRemoteActor(%s) = %v", username, err)
	}
	return a.Account.ID
}

// activeFollow records an active follow from follower to followee as the federation module does
// for remote accounts: the follow, its acceptance, and the Followed event its projectors use.
func (e *followEnv) activeFollow(t *testing.T, follower, followee string) {
	t.Helper()
	ctx := t.Context()
	e.clock.Advance(time.Second)
	f := graph.Follow{
		ID: e.ids.NewID(), FollowerAccountID: follower, FolloweeAccountID: followee, RequestedAt: e.clock.Now(),
	}
	f.URI = "https://remote.example/follows/" + f.ID
	prefs := graph.DefaultPreferences(f.ID)
	acc := graph.Acceptance{FollowID: f.ID, AcceptedBy: graph.AcceptedByRemote, AcceptedAt: f.RequestedAt}
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := e.repo.InsertFollow(ctx, tx, f); err != nil {
			return err
		}
		if err := e.repo.UpsertPreferences(ctx, tx, prefs); err != nil {
			return err
		}
		if err := e.repo.InsertAcceptance(ctx, tx, acc); err != nil {
			return err
		}
		return tx.Emit(ctx, appgraph.Followed{Follow: f, Acceptance: acc, Preferences: prefs})
	})
	if err != nil {
		t.Fatalf("record follow %s -> %s: %v", follower, followee, err)
	}
}

func (e *followEnv) receiveBlock(t *testing.T, in appgraph.RemoteBlockInput) {
	t.Helper()
	e.clock.Advance(time.Second)
	if err := e.graph.ReceiveBlock(t.Context(), in); err != nil {
		t.Fatalf("ReceiveBlock(%+v) = %v", in, err)
	}
}

// blockReceivesRemote checks the blocks of a remote account received from its server and a local
// account's block of a remote one.
func blockReceivesRemote(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, carol := e.user(t, "alice", false), e.user(t, "carol", false)
	bob := e.remoteUser(t, "bob")
	e.activeFollow(t, alice, bob)
	e.activeFollow(t, bob, alice)
	e.activeFollow(t, bob, carol)
	checkCounters(t, e, alice, 1, 1)
	e.events.take()

	uri := "https://remote.example/users/bob#blocks/1"
	e.receiveBlock(t, appgraph.RemoteBlockInput{URI: uri, BlockerAccountID: bob, BlockedAccountID: alice})
	checkBlocks(t, e, alice, bob, false, true)
	checkRelationship(t, e, alice, bob, graph.StateNone, graph.StateNone)
	checkRelationship(t, e, carol, bob, graph.StateNone, graph.StateActive)
	checkCounters(t, e, alice, 0, 0)
	for _, pair := range [][2]string{{alice, bob}, {bob, alice}} {
		hs := e.histories(t, pair[0], pair[1])
		if len(hs) != 1 || hs[0].Termination == nil || hs[0].Termination.Reason != graph.TerminatedBlocked {
			t.Errorf("follow %s -> %s = %+v, want one terminated as blocked", pair[0], pair[1], hs)
		}
	}
	var blocked int
	for _, ev := range e.events.take() {
		if b, ok := ev.(appgraph.Blocked); ok {
			blocked++
			if b.Block.URI != uri || b.Block.BlockerAccountID != bob {
				t.Errorf("Blocked = %+v, want bob's block %s", b, uri)
			}
		}
	}
	if blocked != 1 {
		t.Errorf("%d Blocked events, want 1", blocked)
	}

	// The same Block again, and another Block while one is in force, change nothing.
	e.receiveBlock(t, appgraph.RemoteBlockInput{URI: uri, BlockerAccountID: bob, BlockedAccountID: alice})
	e.receiveBlock(t, appgraph.RemoteBlockInput{URI: uri + "-again", BlockerAccountID: bob, BlockedAccountID: alice})
	if hs := e.blockHistories(t, bob, alice); len(hs) != 1 {
		t.Errorf("blocks of alice by bob = %+v, want 1", hs)
	}

	// Only the blocker undoes its block; undoing it again changes nothing.
	if err := e.graph.RetractRemoteBlock(ctx, appgraph.RemoteUnblockInput{BlockURI: uri, BlockerAccountID: carol}); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("RetractRemoteBlock by another account = %v, want ErrNotFound", err)
	}
	if err := e.graph.RetractRemoteBlock(ctx, appgraph.RemoteUnblockInput{BlockURI: uri + "-unknown", BlockerAccountID: bob}); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("RetractRemoteBlock of an unknown block = %v, want ErrNotFound", err)
	}
	for range 2 {
		if err := e.graph.RetractRemoteBlock(ctx, appgraph.RemoteUnblockInput{BlockURI: uri, BlockerAccountID: bob}); err != nil {
			t.Fatalf("RetractRemoteBlock = %v", err)
		}
	}
	checkBlocks(t, e, alice, bob, false, false)
	if hs := e.blockHistories(t, bob, alice); len(hs) != 1 || hs[0].Retraction == nil {
		t.Errorf("blocks of alice by bob = %+v, want 1 retracted", hs)
	}
	// A retracted Block received again stays retracted.
	e.receiveBlock(t, appgraph.RemoteBlockInput{URI: uri, BlockerAccountID: bob, BlockedAccountID: alice})
	checkBlocks(t, e, alice, bob, false, false)

	// A Block without an id is undone by its accounts.
	e.receiveBlock(t, appgraph.RemoteBlockInput{BlockerAccountID: bob, BlockedAccountID: alice})
	checkBlocks(t, e, alice, bob, false, true)
	if err := e.graph.RetractRemoteBlock(ctx, appgraph.RemoteUnblockInput{BlockerAccountID: bob, BlockedAccountID: alice}); err != nil {
		t.Fatalf("RetractRemoteBlock without a URI = %v", err)
	}
	checkBlocks(t, e, alice, bob, false, false)

	// Only a remote account blocks a local one through a received Block.
	if err := e.graph.ReceiveBlock(ctx, appgraph.RemoteBlockInput{URI: uri + "-x", BlockerAccountID: carol, BlockedAccountID: alice}); !errors.Is(err, appgraph.ErrLocalAccount) {
		t.Errorf("ReceiveBlock by a local account = %v, want ErrLocalAccount", err)
	}
	other := e.remoteUser(t, "dave")
	if err := e.graph.ReceiveBlock(ctx, appgraph.RemoteBlockInput{URI: uri + "-y", BlockerAccountID: bob, BlockedAccountID: other}); !errors.Is(err, appgraph.ErrRemoteAccount) {
		t.Errorf("ReceiveBlock of a remote account = %v, want ErrRemoteAccount", err)
	}
	if err := e.graph.ReceiveBlock(ctx, appgraph.RemoteBlockInput{URI: uri + "-z", BlockerAccountID: e.ids.NewID(), BlockedAccountID: alice}); !errors.Is(err, appgraph.ErrAccountUnavailable) {
		t.Errorf("ReceiveBlock by a missing account = %v, want ErrAccountUnavailable", err)
	}

	// A local account blocks a remote one; the block ends their follows.
	if rel := e.block(t, carol, bob); !rel.Blocking || rel.FollowedBy != graph.StateNone {
		t.Errorf("Block(carol, bob) = %+v, want blocking without follows", rel)
	}
	hs := e.blockHistories(t, carol, bob)
	if len(hs) != 1 {
		t.Fatalf("blocks of bob by carol = %+v, want 1", hs)
	}
	got, err := e.graph.FindBlock(ctx, hs[0].Block.ID)
	if err != nil || got != hs[0].Block || got.URI != graph.BlockURI(contractHost, got.ID) {
		t.Errorf("FindBlock = %+v, %v; want %+v with its local URI", got, err, hs[0].Block)
	}
	if _, err := e.graph.FindBlock(ctx, e.ids.NewID()); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindBlock(missing) = %v, want ErrNotFound", err)
	}
}
