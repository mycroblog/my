// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/graph"
)

// The moves of local accounts (ADR-0089 送る側), part of AccountMoveContract: a move whose target
// names the account among its aliases records the move, makes the actor carry movedTo, queues the
// Update of the actor and the Move to the remote followers' shared inbox, and moves the local
// followers; a target without the alias, the account itself, a target that moved, and a move
// within 30 days of the last one, cancelled or not, are refused; the cancellation queues the
// Update without movedTo; a moved account answers a Follow with a Reject; and a move to a local
// account moves the local followers to it.

// moveAliases are the aliases of local accounts, by account ID.
type moveAliases map[string][]string

func (a moveAliases) HasAlias(_ context.Context, accountID, aliasURI string) (bool, error) {
	return slices.Contains(a[accountID], aliasURI), nil
}

// localMoves returns the local moves of the environment, with the local aliases aliases.
func (e *moveEnv) localMoves(aliases moveAliases) *appfederation.LocalMoves {
	return appfederation.NewLocalMoves(appfederation.LocalMovesDeps{
		Accounts: e.accounts, Targets: e.targets, Aliases: aliases, Suspensions: moveSuspensions(e.suspended),
	})
}

// remoteShared stores the remote actor uri with its inbox at uri/inbox and the shared inbox
// shared, and returns its account ID.
func (e *moveEnv) remoteShared(t *testing.T, uri, shared string) string {
	t.Helper()
	ctx := t.Context()
	var a account.Account
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		a, err = e.remotes.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: uri, Domain: federation.IRI(uri).Host(), Type: "Person", PreferredUsername: uri[strings.LastIndex(uri, "/")+1:],
			InboxURL: uri + "/inbox", SharedInboxURL: shared, Document: []byte(`{"id":"` + uri + `"}`),
		})
		return err
	})
	if err != nil {
		t.Fatalf("UpsertRemoteActor(%s): %v", uri, err)
	}
	return a.ID
}

// followedBy makes the remote accounts follow the local account, accepted at once.
func (e *moveEnv) followedBy(t *testing.T, local string, remotes ...string) {
	t.Helper()
	for i, r := range remotes {
		in := appgraph.ReceiveFollowInput{URI: fmt.Sprintf("https://remote.example/follows/%s/%d", local, i), FollowerID: r, FolloweeID: local}
		if err := e.graph.ReceiveFollow(t.Context(), in); err != nil {
			t.Fatalf("ReceiveFollow: %v", err)
		}
	}
}

// movedTo returns the movedTo of the actor of the local account id.
func (e *moveEnv) movedTo(t *testing.T, id string) federation.IRI {
	t.Helper()
	a, err := e.accounts.FindByID(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	got, err := appfederation.LocalActorMovedTo(t.Context(), e.accounts, a)
	if err != nil {
		t.Fatalf("LocalActorMovedTo: %v", err)
	}
	return got
}

// actorUpdates returns the Updates of the actor of the local account id sent so far, oldest
// first, with the movedTo of each.
func (e *moveEnv) actorUpdates(t *testing.T, id string) ([]remoteSent, []any) {
	t.Helper()
	var ups []remoteSent
	var moved []any
	for _, s := range e.sentOf(t, federation.OutboundAccount, id) {
		if s.Type != federation.ActivityUpdate {
			continue
		}
		ups = append(ups, s)
		obj, _ := s.Document["object"].(map[string]any)
		moved = append(moved, obj["movedTo"])
	}
	return ups, moved
}

func movesSendLocal(t *testing.T, e *moveEnv) {
	ctx := t.Context()
	reg := e.register(t, "alice")
	alice := reg.AccountID
	aliceURI := e.actorURI(alice)
	const shared = "https://remote.example/inbox"
	bob := e.remoteShared(t, "https://remote.example/users/bob", shared)
	carol := e.remoteShared(t, "https://remote.example/users/carol", shared)
	e.followedBy(t, alice, bob, carol)
	dave := e.register(t, "dave").AccountID
	if _, err := e.graph.Follow(ctx, dave, alice); err != nil {
		t.Fatal(err)
	}
	e.serve(t, moveTarget, "", federation.IRI(aliceURI))
	target := e.remote(t, moveTarget)
	const noAlias = "https://new.example/users/other"
	e.serve(t, noAlias, "", "https://other.example/users/alice")
	moves := e.localMoves(nil)

	if _, err := moves.Move(ctx, appfederation.LocalMoveRequest{AccountID: alice, UserID: reg.User.ID, TargetAccountID: e.remote(t, noAlias)}); !errors.Is(err, appfederation.ErrMoveAliasMissing) {
		t.Errorf("Move to a target without the alias = %v, want ErrMoveAliasMissing", err)
	}
	if got := e.movedTo(t, alice); got != "" {
		t.Errorf("movedTo after a refused move = %q, want none", got)
	}
	m, err := moves.Move(ctx, appfederation.LocalMoveRequest{AccountID: alice, UserID: reg.User.ID, TargetAccountID: target})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if m.InitiatedBy != account.MoveInitiatedBySelf || m.ActivityURI != "" {
		t.Errorf("move = %+v, want one initiated by self", m)
	}
	if st, err := e.accounts.MoveState(ctx, alice); err != nil || st.TargetAccountID != target {
		t.Fatalf("MoveState = %+v, %v, want the target", st, err)
	}
	if got := e.movedTo(t, alice); got != moveTarget {
		t.Errorf("movedTo = %q, want %s", got, moveTarget)
	}
	// The local follower follows the target.
	if ran := e.migrate(t); ran == 0 {
		t.Fatal("the move enqueued no migration")
	}
	if r := e.relationship(t, dave, target); r.Following != graph.StatePending {
		t.Errorf("dave → target = %q, want a pending follow", r.Following)
	}
	if r := e.relationship(t, dave, alice); r.Following != graph.StateNone {
		t.Errorf("dave → alice = %q, want none", r.Following)
	}
	// The Update of the actor carries movedTo, and the Move follows it, both to the shared inbox
	// of the remote followers.
	ups, moved := e.actorUpdates(t, alice)
	if len(ups) != 1 || moved[0] != moveTarget || !slices.Equal(ups[0].Inboxes, []string{shared}) {
		t.Errorf("Updates of alice = %+v, want one with movedTo %s to %s", ups, moveTarget, shared)
	}
	sent := e.sentOf(t, federation.OutboundMove, m.ID)
	if len(sent) != 1 || sent[0].Type != federation.ActivityMove || !slices.Equal(sent[0].Inboxes, []string{shared}) {
		t.Fatalf("Moves = %+v, want one Move to %s", sent, shared)
	}
	doc := sent[0].Document
	id, _ := doc["id"].(string)
	if doc["actor"] != aliceURI || doc["object"] != aliceURI || doc["target"] != moveTarget ||
		!reflect.DeepEqual(doc["to"], []any{aliceURI + "/followers"}) || !strings.HasPrefix(id, "https://"+contractHost+"/ap/activities/") {
		t.Errorf("Move = %v, want alice moving herself to %s, to her followers, with an /ap/activities id", doc, moveTarget)
	}
}

func movesRefuseAndCancelLocal(t *testing.T, e *moveEnv) {
	ctx := t.Context()
	reg := e.register(t, "alice")
	alice := reg.AccountID
	aliceURI := federation.IRI(e.actorURI(alice))
	bob := e.remote(t, "https://remote.example/users/bob")
	e.followedBy(t, alice, bob)
	const (
		second = "https://second.example/users/alice"
		moved  = "https://moved.example/users/alice"
	)
	e.serve(t, moveTarget, "", aliceURI)
	e.serve(t, second, "", aliceURI)
	e.serve(t, moved, "https://elsewhere.example/users/alice", aliceURI)
	target, secondID := e.remote(t, moveTarget), e.remote(t, second)
	moves := e.localMoves(nil)
	move := func(targetID string) error {
		_, err := moves.Move(ctx, appfederation.LocalMoveRequest{AccountID: alice, UserID: reg.User.ID, TargetAccountID: targetID})
		return err
	}

	if err := move(alice); !errors.Is(err, appfederation.ErrMoveTargetUnavailable) {
		t.Errorf("Move to itself = %v, want ErrMoveTargetUnavailable", err)
	}
	if err := move(e.remote(t, moved)); !errors.Is(err, appfederation.ErrMoveTargetMoved) {
		t.Errorf("Move to a target that moved = %v, want ErrMoveTargetMoved", err)
	}
	if err := appaccount.ErrNotMoved; !errors.Is(moves.Cancel(ctx, alice), err) {
		t.Errorf("Cancel before a move, want ErrNotMoved")
	}
	if err := move(target); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if err := move(secondID); !errors.Is(err, appaccount.ErrMoveTooSoon) {
		t.Errorf("Move again at once = %v, want ErrMoveTooSoon", err)
	}

	// A Follow of the moved account gets a Reject, and no follow.
	carol := e.remote(t, "https://remote.example/users/carol")
	follow := fmt.Appendf(nil, `{"@context":"https://www.w3.org/ns/activitystreams","id":"https://remote.example/follows/carol","type":"Follow",`+
		`"actor":"https://remote.example/users/carol","object":"%s"}`, aliceURI)
	if got := e.receive(t, follow); got != appfederation.InboxHandled {
		t.Errorf("Follow of the moved account = %q, want handled", got)
	}
	if r := e.relationship(t, carol, alice); r.Following != graph.StateNone {
		t.Errorf("carol → alice = %q, want none", r.Following)
	}
	if sent := e.sentOf(t, federation.OutboundRefusal, carol); len(sent) != 1 || sent[0].Type != federation.ActivityReject {
		t.Errorf("answers to carol = %+v, want one Reject", sent)
	}

	// The cancellation sends the Update without movedTo, and the wait still counts from the
	// cancelled move.
	if err := moves.Cancel(ctx, alice); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if _, err := e.accounts.MoveState(ctx, alice); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("MoveState after the cancellation = %v, want ErrNotFound", err)
	}
	if got := e.movedTo(t, alice); got != "" {
		t.Errorf("movedTo after the cancellation = %q, want none", got)
	}
	if _, moved := e.actorUpdates(t, alice); !reflect.DeepEqual(moved, []any{moveTarget, nil}) {
		t.Errorf("movedTo of the Updates of alice = %v, want %s then none", moved, moveTarget)
	}
	if err := moves.Cancel(ctx, alice); !errors.Is(err, appaccount.ErrNotMoved) {
		t.Errorf("Cancel again = %v, want ErrNotMoved", err)
	}
	if err := move(secondID); !errors.Is(err, appaccount.ErrMoveTooSoon) {
		t.Errorf("Move after the cancellation = %v, want ErrMoveTooSoon", err)
	}
	e.clock.Advance(account.LocalMoveInterval)
	if err := move(target); !errors.Is(err, appaccount.ErrMovedBefore) {
		t.Errorf("Move to the first target again = %v, want ErrMovedBefore", err)
	}
	if err := move(secondID); err != nil {
		t.Errorf("Move after the interval = %v", err)
	}
	if st, err := e.accounts.MoveState(ctx, alice); err != nil || st.TargetAccountID != secondID {
		t.Errorf("MoveState after the second move = %+v, %v, want the second target", st, err)
	}
}

func movesWithinTheServer(t *testing.T, e *moveEnv) {
	ctx := t.Context()
	reg := e.register(t, "alice")
	alice := reg.AccountID
	newAlice := e.register(t, "newalice").AccountID
	dave := e.register(t, "dave").AccountID
	if _, err := e.graph.Follow(ctx, dave, alice); err != nil {
		t.Fatal(err)
	}
	in := appfederation.LocalMoveRequest{AccountID: alice, UserID: reg.User.ID, TargetAccountID: newAlice}
	if _, err := e.localMoves(nil).Move(ctx, in); !errors.Is(err, appfederation.ErrMoveAliasMissing) {
		t.Errorf("Move to a local account without the alias = %v, want ErrMoveAliasMissing", err)
	}
	if _, err := e.localMoves(moveAliases{newAlice: {e.actorURI(alice)}}).Move(ctx, in); err != nil {
		t.Fatalf("Move to a local account: %v", err)
	}
	if got := e.movedTo(t, alice); got != federation.IRI(e.actorURI(newAlice)) {
		t.Errorf("movedTo = %q, want the local target", got)
	}
	if ran := e.migrate(t); ran == 0 {
		t.Fatal("the move enqueued no migration")
	}
	if r := e.relationship(t, dave, newAlice); !r.ActivelyFollowing() {
		t.Errorf("dave → newalice = %q, want an active follow", r.Following)
	}
	if r := e.relationship(t, dave, alice); r.Following != graph.StateNone {
		t.Errorf("dave → alice = %q, want none", r.Following)
	}
}
