// SPDX-License-Identifier: AGPL-3.0-only

package notificationtest

import (
	"context"
	"errors"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/pkg/testkit"
)

// SeveranceContract verifies the repository methods that the follows cut by suspensions (FR-717,
// ADR-0058) rest on, by the methods alone, the port contract of ADR-0153 §1: the moderation
// repository records a severance once per action and account and finds it, of its account only,
// with what its action suspended, and erases an account's severances; the graph repository lists
// the edges into and out of an account in order and locks a pair in a transaction. The use cases
// on top of them are SeveranceFlows, which internal/flowtest/notification runs once on SQLite.
func SeveranceContract(t *testing.T, newHarness SeveranceFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *severanceRepo)
	}{
		{"RecordsSeverancesOnce", severanceRepoRecords},
		{"ErasesTheSeverancesOfAnAccount", severanceRepoErases},
		{"ListsTheEdgesOfAnAccount", severanceRepoEdges},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := testkit.NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			tc.run(t, &severanceRepo{h: h, clock: clock, ids: testkit.NewIDGenerator(clock)})
		})
	}
}

type severanceRepo struct {
	h     SeveranceHarness
	clock *testkit.Clock
	ids   *testkit.IDGenerator
}

func (r *severanceRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// suspend records a suspension of a remote account and returns the action.
func (r *severanceRepo) suspend(t *testing.T, target string) moderation.Action {
	t.Helper()
	a := moderation.Action{
		ID: r.ids.NewID(), TargetAccountID: target, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam,
		Issuer: moderation.IssuerModerator, IssuedAt: r.clock.Now(),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Moderation.InsertAction(ctx, tx, a) })
	return a
}

// suspendDomain records a suspension of a domain and returns the action.
func (r *severanceRepo) suspendDomain(t *testing.T, domain string) moderation.DomainAction {
	t.Helper()
	a := moderation.DomainAction{ID: r.ids.NewID(), Domain: domain, Kind: moderation.DomainSuspend, IssuedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Moderation.InsertDomainAction(ctx, tx, a) })
	return a
}

// sever inserts s and reports whether it was inserted.
func (r *severanceRepo) sever(t *testing.T, s moderation.Severance) bool {
	t.Helper()
	var ok bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		var err error
		ok, err = r.h.Moderation.InsertSeverance(ctx, tx, s)
		return err
	})
	return ok
}

func severanceRepoRecords(t *testing.T, r *severanceRepo) {
	ctx := t.Context()
	alice, bob, remote := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	action := r.suspend(t, remote)
	domain := r.suspendDomain(t, "suspended.example")
	r.clock.Advance(time.Second)

	ofAction := moderation.Severance{ID: r.ids.NewID(), ActionID: action.ID, AccountID: alice, FollowersLost: 1, FollowingLost: 2, OccurredAt: r.clock.Now()}
	ofDomain := moderation.Severance{ID: r.ids.NewID(), DomainActionID: domain.ID, AccountID: alice, FollowingLost: 3, OccurredAt: r.clock.Now()}
	if !r.sever(t, ofAction) || !r.sever(t, ofDomain) {
		t.Fatal("InsertSeverance inserted nothing")
	}
	// The same action and account insert nothing more; another account of the action does.
	if r.sever(t, moderation.Severance{ID: r.ids.NewID(), ActionID: action.ID, AccountID: alice, FollowersLost: 5, OccurredAt: r.clock.Now()}) {
		t.Error("InsertSeverance of an action's account again inserted it")
	}
	if r.sever(t, moderation.Severance{ID: r.ids.NewID(), DomainActionID: domain.ID, AccountID: alice, FollowersLost: 5, OccurredAt: r.clock.Now()}) {
		t.Error("InsertSeverance of a domain action's account again inserted it")
	}
	if !r.sever(t, moderation.Severance{ID: r.ids.NewID(), ActionID: action.ID, AccountID: bob, FollowersLost: 1, OccurredAt: r.clock.Now()}) {
		t.Error("InsertSeverance of another account of the action inserted nothing")
	}

	got, err := r.h.Moderation.FindSeverance(ctx, alice, ofAction.ID)
	if err != nil || got.ID != ofAction.ID || got.ActionID != action.ID || got.DomainActionID != "" || got.AccountID != alice ||
		got.FollowersLost != 1 || got.FollowingLost != 2 || !got.OccurredAt.Equal(ofAction.OccurredAt) ||
		got.TargetAccountID != remote || got.Domain != "" {
		t.Errorf("FindSeverance(of the account action) = %+v, %v", got, err)
	}
	got, err = r.h.Moderation.FindSeverance(ctx, alice, ofDomain.ID)
	if err != nil || got.DomainActionID != domain.ID || got.ActionID != "" || got.FollowingLost != 3 ||
		got.Domain != "suspended.example" || got.TargetAccountID != "" {
		t.Errorf("FindSeverance(of the domain action) = %+v, %v", got, err)
	}
	if _, err := r.h.Moderation.FindSeverance(ctx, bob, ofAction.ID); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindSeverance(another account's) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Moderation.FindSeverance(ctx, alice, r.ids.NewID()); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindSeverance(unknown) = %v, want ErrNotFound", err)
	}
}

func severanceRepoErases(t *testing.T, r *severanceRepo) {
	ctx := t.Context()
	alice, bob := r.ids.NewID(), r.ids.NewID()
	action := r.suspend(t, r.ids.NewID())
	erased := moderation.Severance{ID: r.ids.NewID(), ActionID: action.ID, AccountID: alice, FollowersLost: 1, OccurredAt: r.clock.Now()}
	kept := moderation.Severance{ID: r.ids.NewID(), ActionID: action.ID, AccountID: bob, FollowersLost: 1, OccurredAt: r.clock.Now()}
	r.sever(t, erased)
	r.sever(t, kept)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Moderation.EraseSeverancesOf(ctx, tx, alice) })
	if _, err := r.h.Moderation.FindSeverance(ctx, alice, erased.ID); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindSeverance(erased) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Moderation.FindSeverance(ctx, bob, kept.ID); err != nil {
		t.Errorf("FindSeverance(another account's) = %v", err)
	}
}

func severanceRepoEdges(t *testing.T, r *severanceRepo) {
	ids := []string{r.ids.NewID(), r.ids.NewID(), r.ids.NewID(), r.ids.NewID()}
	alice, bob, carol, dave := ids[0], ids[1], ids[2], ids[3]
	edge := func(follower, followee string, state graph.State) graph.Edge {
		return graph.Edge{FollowerAccountID: follower, FolloweeAccountID: followee, FollowID: r.ids.NewID(), State: state, Since: r.clock.Now()}
	}
	// In ID order of follower and followee: alice → carol, bob → alice, dave → alice; carol → dave
	// is not alice's.
	toCarol, fromBob, fromDave := edge(alice, carol, graph.StateActive), edge(bob, alice, graph.StatePending), edge(dave, alice, graph.StateActive)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, e := range []graph.Edge{fromDave, toCarol, edge(carol, dave, graph.StateActive), fromBob} {
			if ok, err := r.h.Graph.InsertEdge(ctx, tx, e); err != nil || !ok {
				return errors.Join(err, errors.New("InsertEdge inserted nothing"))
			}
		}
		return nil
	})

	var got []graph.Edge
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		// The locks of the pairs, in either order, are held to the end of the transaction.
		for _, other := range []string{carol, bob} {
			if err := errors.Join(r.h.Graph.LockPair(ctx, tx, alice, other), r.h.Graph.LockPair(ctx, tx, other, alice)); err != nil {
				return err
			}
		}
		var err error
		got, err = r.h.Graph.ListEdgesOf(ctx, tx, alice)
		return err
	})
	want := []graph.Edge{toCarol, fromBob, fromDave}
	if len(got) != len(want) {
		t.Fatalf("ListEdgesOf = %+v, want %+v", got, want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.FollowerAccountID != w.FollowerAccountID || g.FolloweeAccountID != w.FolloweeAccountID || g.FollowID != w.FollowID ||
			g.State != w.State || !g.Since.Equal(w.Since) {
			t.Errorf("ListEdgesOf[%d] = %+v, want %+v", i, g, w)
		}
	}
}
