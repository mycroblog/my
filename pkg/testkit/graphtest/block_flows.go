// SPDX-License-Identifier: AGPL-3.0-only

package graphtest

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/pkg/testkit"
)

// BlockFlows verifies the block and mute use cases of the graph module between accounts that the
// account module keeps, the flows of ADR-0153 §1, which internal/flowtest/graph runs once on
// SQLite; BlockRepositoryContract checks the graph repository's blocks and mutes on every
// adapter. Blocking ends the follows in both directions with the reason blocked and refuses
// follows while it is in force, unblocking, blocking again as a new block that keeps the earlier
// one on record, paging blocks, rejecting invalid blocks, recording each retraction once,
// concurrent follows and blocks of one pair, the emitted domain events, the blocks of remote
// accounts received from their servers and a local account's block of a remote one, muting with
// and without expiry (an expired mute no longer applies on read), paging mutes, rejecting invalid
// mutes, deleting expired mutes with an Unmuted each, muting domains (once each, in the order of
// their domains, with their events and replay), and the property that follow_edges,
// follow_counters, and block_edges equal a rebuild from the follows, the blocks, and their events.
func BlockFlows(t *testing.T, newHarness FollowFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *followEnv)
	}{
		{"BlockingEndsFollowsInBothDirections", blockEndsFollows},
		{"RefusesFollowsWhileBlocked", blockRefusesFollows},
		{"BlocksAgainAsANewBlock", blockAgain},
		{"PagesBlocks", blockPages},
		{"RejectsInvalidBlocks", blockRejectsInvalid},
		{"RecordsEachRetractionOnce", blockRecordsOnce},
		{"AgreesOnConcurrentFollowsAndBlocks", blockConcurrent},
		{"EmitsDomainEvents", blockEmitsEvents},
		{"ReceivesBlocksOfRemoteAccounts", blockReceivesRemote},
		{"MutesUntilUnmutedOrExpired", muteExpires},
		{"PagesMutes", mutePages},
		{"RejectsInvalidMutes", muteRejectsInvalid},
		{"DeletesExpiredMutes", muteDeletesExpired},
		{"MutesDomains", muteDomains},
		{"IncrementalProjectionEqualsRebuild", blockProjectionEqualsRebuild},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newFollowEnv(t, newHarness)
			for _, typ := range []string{
				appgraph.EventBlocked, appgraph.EventUnblocked, appgraph.EventMuted, appgraph.EventUnmuted,
				appgraph.EventDomainMuted, appgraph.EventDomainUnmuted,
			} {
				e.captured.Register(typ, e.events.Record)
			}
			tc.run(t, e)
		})
	}
}

func (e *followEnv) block(t *testing.T, blocker, blocked string) appgraph.Relationship {
	t.Helper()
	e.Clock.Advance(time.Second)
	rel, err := e.graph.Block(t.Context(), blocker, blocked)
	if err != nil {
		t.Fatalf("Block(%s, %s) = %v", blocker, blocked, err)
	}
	return rel
}

func (e *followEnv) unblock(t *testing.T, blocker, blocked string) appgraph.Relationship {
	t.Helper()
	e.Clock.Advance(time.Second)
	rel, err := e.graph.Unblock(t.Context(), blocker, blocked)
	if err != nil {
		t.Fatalf("Unblock(%s, %s) = %v", blocker, blocked, err)
	}
	return rel
}

// blockHistories returns the blocks from blocker to blocked with their retractions, oldest first.
func (e *followEnv) blockHistories(t *testing.T, blocker, blocked string) []graph.BlockHistory {
	t.Helper()
	var out []graph.BlockHistory
	err := e.H.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		out = nil
		all, err := e.repo.ListBlockHistories(t.Context(), tx)
		for _, h := range all {
			if h.Block.BlockerAccountID == blocker && h.Block.BlockedAccountID == blocked {
				out = append(out, h)
			}
		}
		return err
	})
	if err != nil {
		t.Fatalf("ListBlockHistories = %v", err)
	}
	return out
}

func checkBlocks(t *testing.T, e *followEnv, account, target string, blocking, blockedBy bool) {
	t.Helper()
	rel, err := e.graph.Relationship(t.Context(), account, target)
	if err != nil {
		t.Fatalf("Relationship = %v", err)
	}
	if rel.Blocking != blocking || rel.BlockedBy != blockedBy {
		t.Errorf("Relationship(%s, %s) = blocking %v, blocked by %v; want %v, %v",
			account, target, rel.Blocking, rel.BlockedBy, blocking, blockedBy)
	}
}

func blockEndsFollows(t *testing.T, e *followEnv) {
	alice, bob, carol := e.user(t, "alice", true), e.user(t, "bob", false), e.user(t, "carol", false)
	e.follow(t, alice, bob) // active
	e.follow(t, bob, alice) // pending: alice is locked
	e.follow(t, carol, bob)
	e.follow(t, alice, carol)
	checkCounters(t, e, bob, 2, 0)

	rel := e.block(t, bob, alice)
	if !rel.Blocking || rel.BlockedBy || rel.Following != graph.StateNone || rel.FollowedBy != graph.StateNone {
		t.Errorf("Block = %+v, want blocking without follows", rel)
	}
	checkBlocks(t, e, alice, bob, false, true)
	checkRelationship(t, e, alice, bob, graph.StateNone, graph.StateNone)
	checkCounters(t, e, alice, 0, 1)
	checkCounters(t, e, bob, 1, 0)
	for _, pair := range [][2]string{{alice, bob}, {bob, alice}} {
		hs := e.histories(t, pair[0], pair[1])
		if len(hs) != 1 || hs[0].Termination == nil || hs[0].Termination.Reason != graph.TerminatedBlocked {
			t.Errorf("follow %s -> %s = %+v, want one terminated as blocked", pair[0], pair[1], hs)
		}
	}
	if reqs, _ := e.graph.FollowRequests(t.Context(), alice, appgraph.Page{}); len(reqs) != 0 {
		t.Errorf("FollowRequests(alice) = %+v after the block, want none", reqs)
	}
	// Follows with other accounts stay.
	checkRelationship(t, e, carol, bob, graph.StateActive, graph.StateNone)
	checkRelationship(t, e, alice, carol, graph.StateActive, graph.StateNone)

	bs := e.blockHistories(t, bob, alice)
	if len(bs) != 1 || bs[0].Retraction != nil {
		t.Fatalf("blocks = %+v, want one in force", bs)
	}
	b := bs[0].Block
	if b.URI != graph.BlockURI(testkit.ContractHost, b.ID) || b.URI != "https://"+testkit.ContractHost+"/ap/activities/"+b.ID {
		t.Errorf("block URI = %q, want /ap/activities/{id}", b.URI)
	}
	edge, err := e.repo.FindBlockEdge(t.Context(), bob, alice)
	if err != nil || edge != graph.EdgeOf(b) {
		t.Errorf("block edge = %+v, %v; want %+v", edge, err, graph.EdgeOf(b))
	}

	// Blocking again changes nothing.
	e.block(t, bob, alice)
	if bs := e.blockHistories(t, bob, alice); len(bs) != 1 {
		t.Errorf("blocks after blocking again = %d, want 1", len(bs))
	}

	// Unblocking does not bring the follows back.
	rel = e.unblock(t, bob, alice)
	if rel.Blocking || rel.Following != graph.StateNone || rel.FollowedBy != graph.StateNone {
		t.Errorf("Unblock = %+v, want no relationship", rel)
	}
	checkBlocks(t, e, alice, bob, false, false)
	if _, err := e.repo.FindBlockEdge(t.Context(), bob, alice); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("block edge after Unblock = %v, want ErrNotFound", err)
	}
	if bs := e.blockHistories(t, bob, alice); len(bs) != 1 || bs[0].Retraction == nil {
		t.Errorf("blocks after Unblock = %+v, want one retracted", bs)
	}
	e.unblock(t, bob, alice) // no-op
	if bs := e.blockHistories(t, bob, alice); len(bs) != 1 {
		t.Errorf("blocks after unblocking again = %d, want 1", len(bs))
	}
}

func blockRefusesFollows(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", false)
	e.block(t, alice, bob)
	for _, pair := range [][2]string{{alice, bob}, {bob, alice}} {
		if _, err := e.graph.Follow(ctx, pair[0], pair[1]); !errors.Is(err, appgraph.ErrBlocked) {
			t.Errorf("Follow(%s, %s) while blocked = %v, want ErrBlocked", pair[0], pair[1], err)
		}
	}
	if edges, _ := e.repo.ListEdges(ctx); len(edges) != 0 {
		t.Errorf("edges = %+v, want none", edges)
	}
	// A block in each direction: lifting one keeps the other in force.
	e.block(t, bob, alice)
	e.unblock(t, alice, bob)
	if _, err := e.graph.Follow(ctx, alice, bob); !errors.Is(err, appgraph.ErrBlocked) {
		t.Errorf("Follow while bob blocks alice = %v, want ErrBlocked", err)
	}
	e.unblock(t, bob, alice)
	if rel := e.follow(t, alice, bob); rel.Following != graph.StateActive {
		t.Errorf("Follow after unblocking = %+v, want active", rel)
	}
}

func blockAgain(t *testing.T, e *followEnv) {
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", false)
	e.block(t, alice, bob)
	e.unblock(t, alice, bob)
	e.block(t, alice, bob)
	bs := e.blockHistories(t, alice, bob)
	if len(bs) != 2 || bs[0].Retraction == nil || bs[1].Retraction != nil {
		t.Fatalf("blocks = %+v, want a retracted and a new block", bs)
	}
	if bs[0].Block.URI == bs[1].Block.URI || bs[0].Block.ID == bs[1].Block.ID {
		t.Errorf("blocks = %+v, want distinct IDs and URIs", bs)
	}
	edge, err := e.repo.FindBlockEdge(t.Context(), alice, bob)
	if err != nil || edge.BlockID != bs[1].Block.ID {
		t.Errorf("block edge = %+v, %v; want the newest block", edge, err)
	}
}

func blockPages(t *testing.T, e *followEnv) {
	ctx := t.Context()
	troll := e.user(t, "troll", false)
	var blocked []string
	for i := range 5 {
		id := e.user(t, fmt.Sprintf("spam%d", i), false)
		blocked = append([]string{id}, blocked...) // newest block first
		e.block(t, troll, id)
	}
	var got []string
	before := ""
	for range 3 {
		page, err := e.graph.Blocks(ctx, troll, appgraph.Page{Before: before, Limit: 2})
		if err != nil {
			t.Fatalf("Blocks = %v", err)
		}
		if len(page) > 2 {
			t.Fatalf("Blocks page = %d edges, want at most 2", len(page))
		}
		for _, edge := range page {
			got = append(got, edge.BlockedAccountID)
			before = edge.BlockID
		}
	}
	if !reflect.DeepEqual(got, blocked) {
		t.Errorf("Blocks pages = %v, want %v", got, blocked)
	}
	if page, err := e.graph.Blocks(ctx, blocked[0], appgraph.Page{}); err != nil || len(page) != 0 {
		t.Errorf("Blocks(blocked account) = %+v, %v; want none", page, err)
	}
}

func blockRejectsInvalid(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", false)
	if _, err := e.graph.Block(ctx, alice, alice); !errors.Is(err, graph.ErrSelfBlock) {
		t.Errorf("Block(self) = %v, want ErrSelfBlock", err)
	}
	missing := "00000000-0000-7000-8000-000000000000"
	if _, err := e.graph.Block(ctx, alice, missing); !errors.Is(err, appgraph.ErrAccountUnavailable) {
		t.Errorf("Block(missing) = %v, want ErrAccountUnavailable", err)
	}
	if _, err := e.graph.Block(ctx, missing, alice); !errors.Is(err, appgraph.ErrAccountUnavailable) {
		t.Errorf("Block(from missing) = %v, want ErrAccountUnavailable", err)
	}
	_, err := e.Accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: bob, Kind: account.LifecycleDeactivated, InitiatedBy: account.InitiatedBySelf,
	})
	if err != nil {
		t.Fatalf("RecordLifecycleEvent = %v", err)
	}
	if _, err := e.graph.Block(ctx, bob, alice); !errors.Is(err, appgraph.ErrAccountUnavailable) {
		t.Errorf("Block(by deactivated) = %v, want ErrAccountUnavailable", err)
	}
	// A deactivated account can still be blocked.
	if rel := e.block(t, alice, bob); !rel.Blocking {
		t.Errorf("Block(deactivated) = %+v, want blocking", rel)
	}
}

func blockRecordsOnce(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", false)
	e.block(t, alice, bob)
	b := e.blockHistories(t, alice, bob)[0].Block
	for i := range 2 {
		err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return e.repo.InsertBlockRetraction(ctx, tx, graph.BlockRetraction{BlockID: b.ID, RetractedAt: e.Clock.Now()})
		})
		if dup := i == 1; dup != errors.Is(err, appgraph.ErrAlreadyRecorded) {
			t.Errorf("insert retraction #%d = %v; want ErrAlreadyRecorded only for the second", i+1, err)
		}
	}
	if err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.repo.InsertBlock(ctx, tx, b) }); err == nil {
		t.Error("InsertBlock with a taken ID and URI succeeded")
	}
}

// blockConcurrent races follows and blocks of one pair: whatever the order, the block ends up in
// force and no follow survives it.
func blockConcurrent(t *testing.T, e *followEnv) {
	ctx := t.Context()
	for round := range 3 {
		a, b := e.user(t, fmt.Sprintf("a%d", round), false), e.user(t, fmt.Sprintf("b%d", round), false)
		var wg sync.WaitGroup
		for i := range 6 {
			wg.Go(func() {
				var err error
				switch i % 3 {
				case 0:
					_, err = e.graph.Block(ctx, a, b)
				case 1:
					_, err = e.graph.Follow(ctx, a, b)
				default:
					_, err = e.graph.Follow(ctx, b, a)
				}
				if err != nil && !errors.Is(err, appgraph.ErrBlocked) {
					t.Errorf("command = %v", err)
				}
			})
		}
		wg.Wait()
		if bs := e.blockHistories(t, a, b); len(bs) != 1 {
			t.Errorf("round %d: blocks = %+v, want one", round, bs)
		}
		checkBlocks(t, e, a, b, true, false)
		checkRelationship(t, e, a, b, graph.StateNone, graph.StateNone)
		checkCounters(t, e, a, 0, 0)
		checkCounters(t, e, b, 0, 0)
	}
}

func blockEmitsEvents(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", true)
	e.follow(t, alice, bob) // pending
	e.follow(t, bob, alice) // active
	e.events.Take()

	e.block(t, alice, bob)
	got := e.events.Take()
	if len(got) != 3 {
		t.Fatalf("events of Block = %+v, want two FollowTerminated and Blocked", got)
	}
	states := map[string]graph.State{}
	for _, ev := range got[:2] {
		ft, ok := ev.(appgraph.FollowTerminated)
		if !ok || ft.Termination.Reason != graph.TerminatedBlocked {
			t.Fatalf("event = %+v, want FollowTerminated as blocked", ev)
		}
		states[ft.Follow.FollowerAccountID] = ft.PreviousState
	}
	if states[alice] != graph.StatePending || states[bob] != graph.StateActive {
		t.Errorf("terminated follows = %v, want alice's pending and bob's active", states)
	}
	blocked, ok := got[2].(appgraph.Blocked)
	if !ok || blocked.Block.BlockerAccountID != alice || blocked.Block.BlockedAccountID != bob || blocked.Block.URI == "" {
		t.Errorf("last event = %+v, want Blocked by alice", got[2])
	}

	e.unblock(t, alice, bob)
	got = e.events.Take()
	if len(got) != 1 {
		t.Fatalf("events of Unblock = %+v, want one Unblocked", got)
	}
	if u, ok := got[0].(appgraph.Unblocked); !ok || u.Block != blocked.Block || u.Retraction.BlockID != blocked.Block.ID {
		t.Errorf("Unblocked = %+v, want the block's retraction", got[0])
	}

	// Commands that change nothing emit nothing, and a block without follows emits only Blocked.
	e.unblock(t, alice, bob)
	if got := e.events.Take(); len(got) != 0 {
		t.Errorf("events of a no-op Unblock = %+v, want none", got)
	}
	e.block(t, alice, bob)
	if got := e.events.Take(); len(got) != 1 || got[0].EventType() != appgraph.EventBlocked {
		t.Errorf("events of Block without follows = %+v, want one Blocked", got)
	}
	e.block(t, alice, bob)
	if got := e.events.Take(); len(got) != 0 {
		t.Errorf("events of a no-op Block = %+v, want none", got)
	}

	// Muting, also again with other settings, emits Muted with the mute as it now is; unmuting
	// emits Unmuted, and unmuting without a mute emits nothing.
	for _, hide := range []bool{false, true} {
		if _, err := e.graph.Mute(ctx, alice, bob, appgraph.MuteInput{HideNotifications: hide}); err != nil {
			t.Fatal(err)
		}
		got := e.events.Take()
		if len(got) != 1 {
			t.Fatalf("events of Mute = %+v, want one Muted", got)
		}
		if m, ok := got[0].(appgraph.Muted); !ok || m.Mute.AccountID != alice || m.Mute.TargetAccountID != bob || m.Mute.HideNotifications != hide {
			t.Errorf("Muted = %+v, want alice's mute of bob with hide_notifications %v", got[0], hide)
		}
	}
	for range 2 {
		if _, err := e.graph.Unmute(ctx, alice, bob); err != nil {
			t.Fatal(err)
		}
	}
	want := []uow.Event{appgraph.Unmuted{AccountID: alice, TargetAccountID: bob}}
	if got := e.events.Take(); !reflect.DeepEqual(got, want) {
		t.Errorf("events of two Unmutes = %+v, want %+v", got, want)
	}
}

func checkMute(t *testing.T, e *followEnv, account, target string, muting, notifications bool) {
	t.Helper()
	rel, err := e.graph.Relationship(t.Context(), account, target)
	if err != nil {
		t.Fatalf("Relationship = %v", err)
	}
	if rel.Muting != muting || rel.MutingNotifications != notifications {
		t.Errorf("Relationship(%s, %s) = muting %v (notifications %v); want %v (%v)",
			account, target, rel.Muting, rel.MutingNotifications, muting, notifications)
	}
}

func muteExpires(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob, carol := e.user(t, "alice", false), e.user(t, "bob", false), e.user(t, "carol", false)
	e.follow(t, alice, bob)

	rel, err := e.graph.Mute(ctx, alice, bob, appgraph.MuteInput{HideNotifications: true})
	if err != nil || !rel.Muting || !rel.MutingNotifications || rel.Following != graph.StateActive {
		t.Fatalf("Mute = %+v, %v; want muting with notifications, still following", rel, err)
	}
	checkMute(t, e, bob, alice, false, false)
	e.Clock.Advance(365 * 24 * time.Hour)
	checkMute(t, e, alice, bob, true, true)

	// Muting again replaces the settings and the expiry.
	rel, err = e.graph.Mute(ctx, alice, bob, appgraph.MuteInput{Duration: time.Hour})
	if err != nil || !rel.Muting || rel.MutingNotifications {
		t.Fatalf("Mute again = %+v, %v; want muting without notifications", rel, err)
	}
	mutes, err := e.graph.Mutes(ctx, alice, appgraph.MutePage{})
	if err != nil || len(mutes) != 1 || mutes[0].TargetAccountID != bob || mutes[0].ExpiresAt == nil ||
		!mutes[0].ExpiresAt.Equal(mutes[0].CreatedAt.Add(time.Hour)) {
		t.Fatalf("Mutes = %+v, %v; want bob expiring in an hour", mutes, err)
	}
	e.Clock.Advance(time.Hour - time.Second)
	checkMute(t, e, alice, bob, true, false)
	e.Clock.Advance(time.Second)
	checkMute(t, e, alice, bob, false, false)
	if mutes, err := e.graph.Mutes(ctx, alice, appgraph.MutePage{}); err != nil || len(mutes) != 0 {
		t.Errorf("Mutes after expiry = %+v, %v; want none", mutes, err)
	}

	// An expired mute can be renewed, and Unmute removes a mute in force.
	if _, err := e.graph.Mute(ctx, alice, bob, appgraph.MuteInput{}); err != nil {
		t.Fatalf("Mute after expiry = %v", err)
	}
	checkMute(t, e, alice, bob, true, false)
	if _, err := e.graph.Mute(ctx, alice, carol, appgraph.MuteInput{}); err != nil {
		t.Fatalf("Mute(carol) = %v", err)
	}
	rel, err = e.graph.Unmute(ctx, alice, bob)
	if err != nil || rel.Muting {
		t.Errorf("Unmute = %+v, %v; want not muting", rel, err)
	}
	if _, err := e.graph.Unmute(ctx, alice, bob); err != nil {
		t.Errorf("Unmute again = %v, want no-op", err)
	}
	checkMute(t, e, alice, carol, true, false)
	checkRelationship(t, e, alice, bob, graph.StateActive, graph.StateNone)
}

func muteDeletesExpired(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob, carol, dave := e.user(t, "alice", false), e.user(t, "bob", false), e.user(t, "carol", false), e.user(t, "dave", false)
	for target, d := range map[string]time.Duration{bob: time.Hour, carol: 0, dave: time.Hour} {
		if _, err := e.graph.Mute(ctx, alice, target, appgraph.MuteInput{Duration: d}); err != nil {
			t.Fatalf("Mute = %v", err)
		}
	}
	if n, err := e.graph.DeleteExpiredMutes(ctx); err != nil || n != 0 {
		t.Fatalf("DeleteExpiredMutes before expiry = %d, %v; want 0", n, err)
	}
	e.Clock.Advance(time.Hour)
	// dave's mute is renewed after it expired, so only bob's is deleted.
	if _, err := e.graph.Mute(ctx, alice, dave, appgraph.MuteInput{Duration: time.Hour}); err != nil {
		t.Fatalf("Mute(dave) again = %v", err)
	}
	e.events.Take()
	if n, err := e.graph.DeleteExpiredMutes(ctx); err != nil || n != 1 {
		t.Fatalf("DeleteExpiredMutes = %d, %v; want 1", n, err)
	}
	want := []uow.Event{appgraph.Unmuted{AccountID: alice, TargetAccountID: bob}}
	if got := e.events.Take(); !reflect.DeepEqual(got, want) {
		t.Errorf("events of DeleteExpiredMutes = %+v, want %+v", got, want)
	}
	var mutes []graph.Mute
	err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		mutes, err = e.repo.ListAllMutes(ctx, tx)
		return err
	})
	if err != nil {
		t.Fatalf("ListAllMutes = %v", err)
	}
	got := map[string]bool{}
	for _, m := range mutes {
		got[m.TargetAccountID] = true
	}
	if !reflect.DeepEqual(got, map[string]bool{carol: true, dave: true}) {
		t.Errorf("mutes after the cleanup = %+v, want carol's and dave's", mutes)
	}
	if n, err := e.graph.DeleteExpiredMutes(ctx); err != nil || n != 0 {
		t.Errorf("DeleteExpiredMutes again = %d, %v; want 0", n, err)
	}
	checkMute(t, e, alice, carol, true, false)
	checkMute(t, e, alice, dave, true, false)
}

func mutePages(t *testing.T, e *followEnv) {
	ctx := t.Context()
	owner := e.user(t, "owner", false)
	var want []string
	for i := range 5 {
		id := e.user(t, fmt.Sprintf("loud%d", i), false)
		e.Clock.Advance(time.Second)
		if _, err := e.graph.Mute(ctx, owner, id, appgraph.MuteInput{}); err != nil {
			t.Fatalf("Mute = %v", err)
		}
		want = append([]string{id}, want...) // newest first
	}
	// Two mutes created at the same instant are ordered by target account ID.
	e.Clock.Advance(time.Second)
	x, y := e.user(t, "same0", false), e.user(t, "same1", false)
	for _, id := range []string{x, y} {
		if _, err := e.graph.Mute(ctx, owner, id, appgraph.MuteInput{}); err != nil {
			t.Fatalf("Mute = %v", err)
		}
	}
	if y < x {
		x, y = y, x
	}
	want = append([]string{y, x}, want...)
	// An expired mute is left out.
	expired := e.user(t, "brief", false)
	if _, err := e.graph.Mute(ctx, owner, expired, appgraph.MuteInput{Duration: time.Second}); err != nil {
		t.Fatalf("Mute = %v", err)
	}
	e.Clock.Advance(time.Second)

	var got []string
	var before appgraph.MuteCursor
	for range 4 {
		page, err := e.graph.Mutes(ctx, owner, appgraph.MutePage{Before: before, Limit: 2})
		if err != nil {
			t.Fatalf("Mutes = %v", err)
		}
		if len(page) > 2 {
			t.Fatalf("Mutes page = %d mutes, want at most 2", len(page))
		}
		for _, m := range page {
			got = append(got, m.TargetAccountID)
			before = appgraph.MuteCursorOf(m)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Mutes pages = %v, want %v", got, want)
	}
}

func muteRejectsInvalid(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", false)
	if _, err := e.graph.Mute(ctx, alice, alice, appgraph.MuteInput{}); !errors.Is(err, graph.ErrSelfMute) {
		t.Errorf("Mute(self) = %v, want ErrSelfMute", err)
	}
	for _, d := range []time.Duration{-time.Hour, time.Millisecond} {
		if _, err := e.graph.Mute(ctx, alice, bob, appgraph.MuteInput{Duration: d}); !errors.Is(err, graph.ErrMuteExpiry) {
			t.Errorf("Mute(duration %v) = %v, want ErrMuteExpiry", d, err)
		}
	}
	missing := "00000000-0000-7000-8000-000000000000"
	if _, err := e.graph.Mute(ctx, alice, missing, appgraph.MuteInput{}); !errors.Is(err, appgraph.ErrAccountUnavailable) {
		t.Errorf("Mute(missing) = %v, want ErrAccountUnavailable", err)
	}
	if _, err := e.graph.Mute(ctx, missing, alice, appgraph.MuteInput{}); !errors.Is(err, appgraph.ErrAccountUnavailable) {
		t.Errorf("Mute(from missing) = %v, want ErrAccountUnavailable", err)
	}
	if mutes, _ := e.graph.Mutes(ctx, alice, appgraph.MutePage{}); len(mutes) != 0 {
		t.Errorf("Mutes = %+v, want none", mutes)
	}
}

func muteDomains(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", false)
	domains := []string{"c.example", "a.example", "b.example"}
	for _, d := range domains {
		m, err := e.graph.MuteDomain(ctx, alice, d)
		if err != nil {
			t.Fatalf("MuteDomain(%s) = %v", d, err)
		}
		if m.AccountID != alice || m.Domain != d || !m.CreatedAt.Equal(e.Clock.Now()) {
			t.Errorf("MuteDomain(%s) = %+v", d, m)
		}
		want := []uow.Event{appgraph.DomainMuted{Mute: m}}
		if got := e.events.Take(); !reflect.DeepEqual(got, want) {
			t.Errorf("events of MuteDomain = %+v, want %+v", got, want)
		}
	}
	// Muting a domain again changes nothing and emits nothing.
	if _, err := e.graph.MuteDomain(ctx, alice, "a.example"); err != nil {
		t.Fatal(err)
	}
	if got := e.events.Take(); len(got) != 0 {
		t.Errorf("events of a repeated MuteDomain = %+v, want none", got)
	}
	if _, err := e.graph.MuteDomain(ctx, bob, "a.example"); err != nil {
		t.Fatal(err)
	}
	e.events.Take()

	var got []string
	after := ""
	for range 3 {
		page, err := e.graph.DomainMutes(ctx, alice, appgraph.DomainMutePage{After: after, Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range page {
			got = append(got, m.Domain)
			after = m.Domain
		}
	}
	if want := []string{"a.example", "b.example", "c.example"}; !reflect.DeepEqual(got, want) {
		t.Errorf("DomainMutes pages = %v, want %v", got, want)
	}

	for _, d := range []string{"", "Upper.example", "a..example", "a.example/path", "a.example:443", testkit.ContractHost} {
		if _, err := e.graph.MuteDomain(ctx, alice, d); !errors.Is(err, graph.ErrInvalidDomain) {
			t.Errorf("MuteDomain(%q) = %v, want ErrInvalidDomain", d, err)
		}
	}
	missing := "00000000-0000-7000-8000-000000000000"
	if _, err := e.graph.MuteDomain(ctx, missing, "a.example"); !errors.Is(err, appgraph.ErrAccountUnavailable) {
		t.Errorf("MuteDomain(from missing) = %v, want ErrAccountUnavailable", err)
	}

	for range 2 {
		if err := e.graph.UnmuteDomain(ctx, alice, "b.example"); err != nil {
			t.Fatal(err)
		}
	}
	want := []uow.Event{appgraph.DomainUnmuted{AccountID: alice, Domain: "b.example"}}
	if got := e.events.Take(); !reflect.DeepEqual(got, want) {
		t.Errorf("events of two UnmuteDomains = %+v, want %+v", got, want)
	}

	// Replaying emits a DomainMuted for each domain mute, ordered by account and domain.
	var replayed []string
	err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return appgraph.ReplayEvents(ctx, tx, e.repo, func(ev uow.Event) error {
			if m, ok := ev.(appgraph.DomainMuted); ok {
				replayed = append(replayed, m.Mute.AccountID+" "+m.Mute.Domain)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	wantReplay := []string{alice + " a.example", alice + " c.example", bob + " a.example"}
	if bob < alice {
		wantReplay = []string{bob + " a.example", alice + " a.example", alice + " c.example"}
	}
	if !reflect.DeepEqual(replayed, wantReplay) {
		t.Errorf("replayed domain mutes = %v, want %v", replayed, wantReplay)
	}
}

func blockProjectionEqualsRebuild(t *testing.T, e *followEnv) {
	ctx := t.Context()
	rng := rand.New(rand.NewPCG(96, 20261016)) //nolint:gosec // deterministic test input, not a secret
	var ids []string
	for i := range 6 {
		ids = append(ids, e.user(t, fmt.Sprintf("u%d", i), i%3 == 0))
	}
	pick := func() (string, string) {
		a := ids[rng.IntN(len(ids))]
		for {
			if b := ids[rng.IntN(len(ids))]; b != a {
				return a, b
			}
		}
	}
	for round := range 3 {
		for range 80 {
			e.Clock.Advance(time.Duration(rng.IntN(5000)+1) * time.Millisecond)
			a, b := pick()
			var err error
			switch rng.IntN(7) {
			case 0, 1, 2:
				_, err = e.graph.Follow(ctx, a, b)
			case 3:
				_, err = e.graph.AcceptFollowRequest(ctx, b, a)
			case 4:
				_, err = e.graph.Unfollow(ctx, a, b)
			case 5:
				_, err = e.graph.Block(ctx, a, b)
			default:
				_, err = e.graph.Unblock(ctx, a, b)
			}
			if err != nil && !errors.Is(err, appgraph.ErrNotFound) && !errors.Is(err, appgraph.ErrBlocked) {
				t.Fatalf("command = %v", err)
			}
		}
		incremental := snapshotBlockProjections(t, e.repo)
		if len(incremental.blocks) == 0 || len(incremental.follows.edges) == 0 {
			t.Fatalf("round %d: projections are empty: %+v", round, incremental)
		}
		for _, be := range incremental.blocks {
			for _, fe := range incremental.follows.edges {
				if (fe.FollowerAccountID == be.BlockerAccountID && fe.FolloweeAccountID == be.BlockedAccountID) ||
					(fe.FollowerAccountID == be.BlockedAccountID && fe.FolloweeAccountID == be.BlockerAccountID) {
					t.Fatalf("round %d: follow %+v survives block %+v", round, fe, be)
				}
			}
		}
		if err := appgraph.RebuildProjections(ctx, e.H.UnitOfWork, e.repo); err != nil {
			t.Fatalf("RebuildProjections = %v", err)
		}
		rebuilt := snapshotBlockProjections(t, e.repo)
		if !reflect.DeepEqual(incremental, rebuilt) {
			t.Fatalf("round %d: incremental projections differ from a rebuild:\nincremental %+v\nrebuilt     %+v", round, incremental, rebuilt)
		}
	}
}

type blockProjections struct {
	follows FollowProjections
	blocks  []graph.BlockEdge
}

func snapshotBlockProjections(t *testing.T, repo appgraph.Repository) blockProjections {
	t.Helper()
	blocks, err := repo.ListBlockEdges(t.Context())
	if err != nil {
		t.Fatalf("ListBlockEdges = %v", err)
	}
	return blockProjections{follows: SnapshotFollowProjections(t, repo), blocks: blocks}
}
