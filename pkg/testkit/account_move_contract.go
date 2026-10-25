// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/domain/notification"
)

// AccountMoveContract verifies the Move of a remote account (ADR-0089, FR-414, federation.md
// §3.4) on a database adapter, through the real account, graph, notification, and federation use
// cases and projectors: a Move whose target names its actor among its aliases is recorded once,
// in account_moves and account_move_states, however often it arrives; a Move of someone else, to
// a target without the alias, to a target that moved itself, or of a suspended account is
// dropped, with its reason in my.move.activities; and the migration job moves the local
// followers to the target (a Follow to the target and an Undo Follow to the source), keeps the
// follow of a follower that blocks the target, copies the blocks and mutes of the source to the
// target, notifies each follower once with account_moved, and changes nothing when it runs
// again. The target is fetched from a fake origin; nothing goes over the network.
func AccountMoveContract(t *testing.T, newHarness RemoteFollowFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *moveEnv)
	}{
		{"RecordsAVerifiedMoveOnce", movesRecordOnce},
		{"DropsUnverifiedMoves", movesDropUnverified},
		{"MigratesTheLocalRelationships", movesMigrate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newMoveEnv(t, newHarness)) })
	}
}

const (
	moveSource = "https://old.example/users/alice"
	moveTarget = "https://new.example/users/alice"
)

type moveEnv struct {
	*remoteFollowEnv
	moves     *appgraph.Service
	targets   *moveTargets
	suspended map[string]bool
	meter     *Meter
}

func newMoveEnv(t *testing.T, newHarness RemoteFollowFactory) *moveEnv {
	t.Helper()
	e := &moveEnv{remoteFollowEnv: newRemoteFollowEnv(t, newHarness), suspended: map[string]bool{}, meter: NewMeter()}
	outbox := queue.AfterCommit(e.projectors, e.jobs, nil)
	e.moves = appgraph.NewService(appgraph.Deps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Graph, Accounts: e.accounts, Clock: e.clock, IDs: e.ids, Host: contractHost,
		Outbox: outbox, Jobs: e.jobs,
	})
	e.moves.RegisterMoveProjector(e.projectors)
	e.moves.RegisterJobs(e.handlers, &queue.Scheduler{})
	e.targets = &moveTargets{e: e, docs: map[federation.IRI]*federation.Actor{}}
	appfederation.RegisterMoveHandler(e.processor, appfederation.MoveHandlerDeps{
		Targets: e.targets, Accounts: e.accounts, Suspensions: moveSuspensions(e.suspended),
		Host: contractHost, Meter: e.meter,
	})
	return e
}

// moveTargets serves the actors that Moves name as their targets, as their origin would.
type moveTargets struct {
	e    *moveEnv
	t    *testing.T
	docs map[federation.IRI]*federation.Actor
}

func (m *moveTargets) RefreshActor(ctx context.Context, uri string) (appaccount.Actor, *federation.Actor, error) {
	doc, ok := m.docs[federation.IRI(uri)]
	if !ok {
		return appaccount.Actor{}, nil, fmt.Errorf("%w: %s", appfederation.ErrFetchNotFound, uri)
	}
	m.e.remote(m.t, uri)
	a, err := m.e.accounts.FindByURI(ctx, uri)
	return a, doc, err
}

// serve makes the origin of uri serve an actor with the aliases aka that moved to movedTo ("" for
// none).
func (e *moveEnv) serve(t *testing.T, uri string, movedTo federation.IRI, aka ...federation.IRI) {
	e.targets.t = t
	e.targets.docs[federation.IRI(uri)] = &federation.Actor{
		ID: federation.IRI(uri), Type: federation.ActorPerson, Inbox: federation.IRI(uri + "/inbox"), AlsoKnownAs: aka, MovedTo: movedTo,
	}
}

type moveSuspensions map[string]bool

func (s moveSuspensions) AccountFederationSuspended(_ context.Context, accountID string) (bool, error) {
	return s[accountID], nil
}

// moveDoc returns a Move by actor of object to target, with the id suffix n.
func moveDoc(actor, object, target string, n int) []byte {
	return fmt.Appendf(nil, `{"@context":"https://www.w3.org/ns/activitystreams","id":"%s#moves/%d","type":"Move",`+
		`"actor":"%s","object":"%s","target":"%s","to":"%s/followers"}`, actor, n, actor, object, target, actor)
}

// moveOutcomes returns the number of Moves counted with each outcome.
func (e *moveEnv) moveOutcomes(outcome string) float64 {
	return e.meter.Sum(appfederation.MoveActivities.Name, map[string]string{appfederation.LabelOutcome: outcome})
}

// moveRecords returns every recorded move.
func (e *moveEnv) moveRecords(t *testing.T) []account.MoveRecord {
	t.Helper()
	var out []account.MoveRecord
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		var err error
		out, err = e.h.Accounts.ListMoveRecords(t.Context(), tx)
		return err
	})
	if err != nil {
		t.Fatalf("ListMoveRecords: %v", err)
	}
	return out
}

// follow makes the local account follow the remote one, accepted by its server.
func (e *moveEnv) follow(t *testing.T, local, remote string) graph.Follow {
	t.Helper()
	if _, err := e.graph.Follow(t.Context(), local, remote); err != nil {
		t.Fatalf("Follow(%s, %s): %v", local, remote, err)
	}
	f := e.liveFollow(t, local, remote)
	if err := e.graph.AcceptRemote(t.Context(), appgraph.RemoteResponse{FollowURI: f.URI, FolloweeID: remote, FollowerID: local}); err != nil {
		t.Fatalf("AcceptRemote: %v", err)
	}
	return f
}

// migrate runs the pending migration jobs, and the pages they enqueue, leaving the other jobs.
func (e *moveEnv) migrate(t *testing.T) int {
	t.Helper()
	handle, _, ok := e.handlers.Lookup(appgraph.JobMigrateFollows)
	if !ok {
		t.Fatal("no handler for " + appgraph.JobMigrateFollows)
	}
	ran := 0
	for {
		e.jobs.mu.Lock()
		var mine, rest []queue.NewJob
		for _, j := range e.jobs.pending {
			if j.Type == appgraph.JobMigrateFollows {
				mine = append(mine, j)
			} else {
				rest = append(rest, j)
			}
		}
		e.jobs.pending = rest
		e.jobs.mu.Unlock()
		if len(mine) == 0 {
			return ran
		}
		for _, nj := range mine {
			j, err := nj.Build(e.ids.NewID(), e.clock.Now())
			if err != nil {
				t.Fatal(err)
			}
			if r := handle(t.Context(), j); !r.IsDone() {
				t.Fatalf("%s %s = %+v (%v), want Done", j.Type, j.Payload, r, r.Err())
			}
			ran++
		}
	}
}

func movesRecordOnce(t *testing.T, e *moveEnv) {
	ctx := t.Context()
	source := e.remote(t, moveSource)
	e.serve(t, moveTarget, "", moveSource)
	if got := e.receive(t, moveDoc(moveSource, moveSource, moveTarget, 1)); got != appfederation.InboxHandled {
		t.Fatalf("Move = %q, want handled", got)
	}
	target, err := e.accounts.FindByURI(ctx, moveTarget)
	if err != nil {
		t.Fatalf("the target was not stored: %v", err)
	}
	st, err := e.accounts.MoveState(ctx, source)
	if err != nil || st.TargetAccountID != target.Account.ID {
		t.Fatalf("MoveState = %+v, %v, want the target %s", st, err, target.Account.ID)
	}
	// The same Move again, and another Move to the same target, change nothing.
	e.receive(t, moveDoc(moveSource, moveSource, moveTarget, 1))
	e.receive(t, moveDoc(moveSource, moveSource, moveTarget, 2))
	recs := e.moveRecords(t)
	if len(recs) != 1 || recs[0].Move.AccountID != source || recs[0].Move.TargetAccountID != target.Account.ID ||
		recs[0].Move.InitiatedBy != account.MoveInitiatedByRemote || !strings.HasPrefix(recs[0].Move.ActivityURI, moveSource+"#moves/") {
		t.Errorf("moves = %+v, want one remote move of %s to %s", recs, source, target.Account.ID)
	}
	if again, err := e.accounts.MoveState(ctx, source); err != nil || again != st {
		t.Errorf("MoveState after the Move again = %+v, %v, want %+v", again, err, st)
	}
	if got := e.moveOutcomes(appfederation.MoveOutcomeRecorded); got != 1 {
		t.Errorf("recorded Moves = %v, want 1", got)
	}
	if got := e.moveOutcomes(appfederation.MoveOutcomeDuplicate); got != 2 {
		t.Errorf("duplicate Moves = %v, want 2", got)
	}
	// A Move to yet another target within the interval is dropped.
	const third = "https://third.example/users/alice"
	e.serve(t, third, "", moveSource)
	if got := e.receive(t, moveDoc(moveSource, moveSource, third, 3)); got != appfederation.InboxDropped {
		t.Errorf("Move to another target too soon = %q, want dropped", got)
	}
	if got := e.moveOutcomes(appfederation.MoveOutcomeTooSoon); got != 1 {
		t.Errorf("too_soon Moves = %v, want 1", got)
	}
	// After the interval, it moves on.
	e.clock.Advance(account.RemoteMoveInterval)
	if got := e.receive(t, moveDoc(moveSource, moveSource, third, 4)); got != appfederation.InboxHandled {
		t.Errorf("Move to another target after the interval = %q, want handled", got)
	}
	if st, err := e.accounts.MoveState(ctx, source); err != nil || st.TargetAccountID == target.Account.ID {
		t.Errorf("MoveState after the second move = %+v, %v, want the third account", st, err)
	}
	// account_move_states is rebuilt from the moves as it was projected.
	incremental := snapshotAccountProjections(t, e.h.Accounts)
	if len(incremental.moves) != 1 {
		t.Fatalf("move states = %+v, want one", incremental.moves)
	}
	if err := appaccount.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Accounts); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	if rebuilt := snapshotAccountProjections(t, e.h.Accounts); !reflect.DeepEqual(rebuilt.moves, incremental.moves) {
		t.Errorf("rebuilt move states = %+v, want %+v", rebuilt.moves, incremental.moves)
	}
	if err := e.meter.Err(); err != nil {
		t.Error(err)
	}
}

func movesDropUnverified(t *testing.T, e *moveEnv) {
	source := e.remote(t, moveSource)
	const (
		noAlias = "https://new.example/users/noalias"
		moved   = "https://new.example/users/moved"
		blocked = "https://new.example/users/blocked"
	)
	e.serve(t, moveTarget, "", moveSource)
	e.serve(t, noAlias, "", "https://other.example/users/bob")
	e.serve(t, moved, "https://elsewhere.example/users/alice", moveSource)
	e.serve(t, blocked, "", moveSource)
	e.suspended[e.remote(t, blocked)] = true
	for _, tc := range []struct {
		name    string
		doc     []byte
		outcome string
	}{
		{"a Move of someone else", moveDoc(moveSource, "https://other.example/users/bob", moveTarget, 1), appfederation.MoveOutcomeNotOwnActor},
		{"a Move to itself", moveDoc(moveSource, moveSource, moveSource, 2), appfederation.MoveOutcomeSameAccount},
		{"a target without the alias", moveDoc(moveSource, moveSource, noAlias, 3), appfederation.MoveOutcomeAliasMissing},
		{"a target that moved", moveDoc(moveSource, moveSource, moved, 4), appfederation.MoveOutcomeTargetMoved},
		{"a suspended target", moveDoc(moveSource, moveSource, blocked, 5), appfederation.MoveOutcomeSuspended},
		{"an unknown target", moveDoc(moveSource, moveSource, "https://gone.example/users/alice", 6), appfederation.MoveOutcomeTargetUnavailable},
	} {
		if got := e.receive(t, tc.doc); got != appfederation.InboxDropped {
			t.Errorf("%s = %q, want dropped", tc.name, got)
		}
		if got := e.moveOutcomes(tc.outcome); got != 1 {
			t.Errorf("%s: %s Moves = %v, want 1", tc.name, tc.outcome, got)
		}
	}
	e.suspended[source] = true
	if got := e.receive(t, moveDoc(moveSource, moveSource, moveTarget, 7)); got != appfederation.InboxDropped {
		t.Errorf("a Move of a suspended account = %q, want dropped", got)
	}
	if recs := e.moveRecords(t); len(recs) != 0 {
		t.Errorf("moves = %+v, want none", recs)
	}
	if _, err := e.accounts.MoveState(t.Context(), source); err == nil {
		t.Error("the source has a move state")
	}
	if err := e.meter.Err(); err != nil {
		t.Error(err)
	}
}

func movesMigrate(t *testing.T, e *moveEnv) {
	ctx := t.Context()
	source := e.remote(t, moveSource)
	follower := e.register(t, "follower").AccountID
	blocker := e.register(t, "blocker").AccountID
	sourceBlocker := e.register(t, "sourceblocker").AccountID
	muter := e.register(t, "muter").AccountID
	e.serve(t, moveTarget, "", moveSource)
	target := e.remote(t, moveTarget)

	old := e.follow(t, follower, source)
	if _, err := e.graph.UpdatePreferences(ctx, follower, source, appgraph.PreferencesInput{ShowBoosts: false, Notify: true}); err != nil {
		t.Fatal(err)
	}
	kept := e.follow(t, blocker, source)
	if _, err := e.graph.Block(ctx, blocker, target); err != nil {
		t.Fatal(err)
	}
	if _, err := e.graph.Block(ctx, sourceBlocker, source); err != nil {
		t.Fatal(err)
	}
	if _, err := e.graph.Mute(ctx, muter, source, appgraph.MuteInput{HideNotifications: true, Duration: 48 * time.Hour}); err != nil {
		t.Fatal(err)
	}
	e.sent(t, old.ID) // the Follows so far

	if got := e.receive(t, moveDoc(moveSource, moveSource, moveTarget, 1)); got != appfederation.InboxHandled {
		t.Fatalf("Move = %q, want handled", got)
	}
	if ran := e.migrate(t); ran == 0 {
		t.Fatal("the Move enqueued no migration")
	}

	// The follower follows the target, pending its server's Accept, with the preferences of
	// its follow of the source, which ended.
	r := e.relationship(t, follower, target)
	if r.Following != graph.StatePending || r.ShowBoosts || !r.Notify {
		t.Errorf("follower → target = %+v, want a pending follow with show_boosts off and notify on", r)
	}
	if r := e.relationship(t, follower, source); r.Following != graph.StateNone {
		t.Errorf("follower → source = %q, want none", r.Following)
	}
	if got := types(e.sent(t, old.ID)); !reflect.DeepEqual(got, []federation.ActivityType{federation.ActivityFollow, federation.ActivityUndo}) {
		t.Errorf("activities of the follow of the source = %v, want Follow, Undo", got)
	}
	moved := e.liveFollow(t, follower, target)
	sent := e.sent(t, moved.ID)
	if !reflect.DeepEqual(types(sent), []federation.ActivityType{federation.ActivityFollow}) || !slices.Equal(sent[0].Inboxes, []string{moveTarget + "/inbox"}) {
		t.Errorf("activities of the follow of the target = %+v, want a Follow to %s/inbox", sent, moveTarget)
	}
	// The follower that blocks the target keeps following the source.
	if r := e.relationship(t, blocker, source); !r.ActivelyFollowing() {
		t.Errorf("blocker → source = %q, want the follow kept", r.Following)
	}
	if e.liveFollow(t, blocker, source).ID != kept.ID {
		t.Error("the kept follow was replaced")
	}
	// The block and the mute of the source apply to the target too.
	if r := e.relationship(t, sourceBlocker, target); !r.Blocking {
		t.Error("the blocker of the source does not block the target")
	}
	if r := e.relationship(t, muter, target); !r.Muting || !r.MutingNotifications {
		t.Errorf("muter → target = %+v, want a mute that hides notifications", r)
	}
	mute, err := e.h.Graph.FindMute(ctx, muter, target, e.clock.Now())
	if err != nil || mute.ExpiresAt == nil {
		t.Fatalf("FindMute = %+v, %v, want a mute with an expiry", mute, err)
	}
	sourceMute, err := e.h.Graph.FindMute(ctx, muter, source, e.clock.Now())
	if err != nil || !sourceMute.ExpiresAt.Equal(*mute.ExpiresAt) {
		t.Errorf("the mute of the target expires at %v, want %v", mute.ExpiresAt, sourceMute.ExpiresAt)
	}

	notified := func() map[string][]notification.Notification {
		out := map[string][]notification.Notification{}
		for _, id := range []string{follower, blocker, sourceBlocker, muter} {
			ns, err := e.notifs.List(ctx, id, appnotification.Query{Types: []notification.Type{notification.TypeAccountMoved}})
			if err != nil {
				t.Fatal(err)
			}
			out[id] = ns
		}
		return out
	}
	first := notified()
	if ns := first[follower]; len(ns) != 1 || ns[0].SourceAccountID != source || ns[0].AccountMoveOutcome != notification.MoveRequested {
		t.Errorf("notifications of the follower = %+v, want one account_moved from the source, requested", ns)
	}
	if ns := first[blocker]; len(ns) != 1 || ns[0].AccountMoveOutcome != notification.MoveBlocking {
		t.Errorf("notifications of the follower that blocks the target = %+v, want one account_moved, blocking", ns)
	}
	if len(first[sourceBlocker])+len(first[muter]) != 0 {
		t.Errorf("accounts that only block or mute the source were notified: %+v", first)
	}

	// Running the migration again changes nothing.
	edges, err := e.h.Graph.ListEdges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := e.h.Graph.ListBlockEdges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recs := e.moveRecords(t)
	if len(recs) != 1 {
		t.Fatalf("moves = %+v, want one", recs)
	}
	if err := e.moves.MigrateFollows(ctx, recs[0].Move.ID, source, target); err != nil {
		t.Fatalf("MigrateFollows again: %v", err)
	}
	if again, err := e.h.Graph.ListEdges(ctx); err != nil || !reflect.DeepEqual(again, edges) {
		t.Errorf("edges after the migration again = %+v, %v, want %+v", again, err, edges)
	}
	if again, err := e.h.Graph.ListBlockEdges(ctx); err != nil || !reflect.DeepEqual(again, blocks) {
		t.Errorf("blocks after the migration again = %+v, %v, want %+v", again, err, blocks)
	}
	if again, err := e.h.Graph.FindMute(ctx, muter, target, e.clock.Now()); err != nil || !again.CreatedAt.Equal(mute.CreatedAt) {
		t.Errorf("the mute of the target after the migration again = %+v, %v, want it kept", again, err)
	}
	if again := notified(); !reflect.DeepEqual(again, first) {
		t.Errorf("notifications after the migration again = %+v, want %+v", again, first)
	}
}
