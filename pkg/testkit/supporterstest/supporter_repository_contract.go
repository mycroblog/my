// SPDX-License-Identifier: AGPL-3.0-only

package supporterstest

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/app/uow"
	domain "github.com/mycroblog/my/internal/domain/supporters"
	"github.com/mycroblog/my/pkg/testkit"
)

// SupporterRepositoryContract verifies the supporters repository of a database adapter by its
// methods alone, the port contract of ADR-0153 §1: the program changes, newest last; the plans
// with their revisions, retirements, and current prices, one plan per role and one plan that is
// not retired per rank; the prices and customers, recorded once per provider's ID; the
// subscriptions with their transitions, grace periods, scheduled changes, links, checkouts, and
// deadlines, one live subscription per user, read in and out of a transaction, and the snapshots
// applied in the order they were read; the duplicates and the reversals, recorded once; the lists
// and counts of the subscriptions; the supporter list by rank and newest first, in keyset pages;
// the erasure of a user; and the projections derived again. The use cases on top of it are
// SupporterFlows, which internal/flowtest/supporters runs once on SQLite.
func SupporterRepositoryContract(t *testing.T, newHarness SupporterFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *supporterRepo)
	}{
		{"RecordsProgramChanges", supporterRepoProgram},
		{"KeepsPlansByRoleAndRank", supporterRepoPlans},
		{"RecordsPricesAndCustomersOnce", supporterRepoPrices},
		{"KeepsOneLiveSubscriptionPerUser", supporterRepoSubscriptions},
		{"RecordsLinksAndCheckouts", supporterRepoCheckouts},
		{"AppliesSnapshotsInTheOrderTheyWereRead", supporterRepoReads},
		{"RecordsDuplicatesAndReversalsOnce", supporterRepoDuplicates},
		{"ListsAndCountsSubscriptions", supporterRepoLists},
		{"ListsTheSupporters", supporterRepoListings},
		{"RebuildsTheProjections", supporterRepoRebuild},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := testkit.NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			tc.run(t, &supporterRepo{h: h, s: h.Supporters, clock: clock, ids: testkit.NewIDGenerator(clock)})
		})
	}
}

const supporterRepoProvider = "fake"

type supporterRepo struct {
	h     SupporterHarness
	s     supporters.Repository
	clock *testkit.Clock
	ids   *testkit.IDGenerator
	rank  int
}

func (r *supporterRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

func (r *supporterRepo) txErr(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) error {
	t.Helper()
	return r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) })
}

// plan inserts a plan of a new role with its first revision at the next rank and a monthly
// price.
func (r *supporterRepo) plan(t *testing.T, name string) supporters.PlanRecord {
	t.Helper()
	r.rank++
	p := domain.Plan{ID: r.ids.NewID(), RoleID: r.ids.NewID(), Currency: "JPY", CreatedByUserID: testkit.NamedID("admin"), CreatedAt: r.clock.Now()}
	rev := domain.PlanRevision{ID: r.ids.NewID(), PlanID: p.ID, RevisedAt: r.clock.Now(), PlanTerms: domain.PlanTerms{
		Name: name, Description: "thanks", MonthlyAmountMinor: 500, AnnualDiscountPercent: 10, EmojiLimit: 5, Rank: r.rank,
		Visibility: domain.VisibilityPublic,
	}}
	price := supporters.Price{
		ID: r.ids.NewID(), PlanID: p.ID, Interval: domain.IntervalMonth, AmountMinor: 500, Currency: "JPY",
		Provider: supporterRepoProvider, Ref: supporters.PriceRef("price_" + p.ID), CreatedAt: r.clock.Now(),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.s.InsertPlan(ctx, tx, p),
			r.s.InsertPlanRevision(ctx, tx, rev),
			r.s.InsertPrice(ctx, tx, price),
			r.s.SetPriceHead(ctx, tx, p.ID, domain.IntervalMonth, price.ID),
		)
	})
	r.clock.Advance(time.Second)
	return supporters.PlanRecord{Plan: p, Revision: rev, Prices: map[domain.Interval]supporters.Price{domain.IntervalMonth: price}}
}

// subscribe inserts a subscription of the user to the plan in state, with a period end and a
// deadline at it for a live one.
func (r *supporterRepo) subscribe(t *testing.T, userID string, plan supporters.PlanRecord, state domain.State) supporters.SubscriptionRecord {
	t.Helper()
	price := plan.Prices[domain.IntervalMonth]
	s := supporters.Subscription{ID: r.ids.NewID(), UserID: userID, PlanID: plan.ID, PriceID: price.ID, Provider: supporterRepoProvider, CreatedAt: r.clock.Now()}
	first := supporters.TransitionRecord{
		ID: r.ids.NewID(), SubscriptionID: s.ID, Reason: domain.ReasonStarted, OccurredAt: r.clock.Now(),
		Transition: domain.Transition{State: state, PlanID: plan.ID, PriceID: price.ID},
	}
	var d *supporters.Deadline
	switch {
	case state == domain.StatePending:
		d = &supporters.Deadline{SubscriptionID: s.ID, Kind: domain.DeadlineCheckout, DueAt: s.CreatedAt.Add(24 * time.Hour)}
	case state.HasPeriod():
		first.PeriodEndsAt = r.clock.Now().Add(30 * 24 * time.Hour)
		if state.Live() {
			d = &supporters.Deadline{SubscriptionID: s.ID, Kind: domain.DeadlinePeriodEnd, DueAt: first.PeriodEndsAt}
		}
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.InsertSubscription(ctx, tx, s, first, d) })
	r.clock.Advance(time.Second)
	return supporters.SubscriptionRecord{Subscription: s, Current: first}
}

// transition records a transition of a subscription with its deadline.
func (r *supporterRepo) transition(t *testing.T, sub supporters.SubscriptionRecord, tr domain.Transition, reason domain.Reason, d *supporters.Deadline) supporters.TransitionRecord {
	t.Helper()
	rec := supporters.TransitionRecord{ID: r.ids.NewID(), SubscriptionID: sub.ID, Transition: tr, Reason: reason, OccurredAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.InsertTransition(ctx, tx, rec, sub.UserID, d) })
	r.clock.Advance(time.Second)
	return rec
}

// find reads a subscription outside and inside a transaction, locked, and checks that they agree.
func (r *supporterRepo) find(t *testing.T, id string) supporters.SubscriptionRecord {
	t.Helper()
	got, err := r.s.FindSubscription(t.Context(), nil, id)
	if err != nil {
		t.Fatalf("FindSubscription = %v", err)
	}
	var locked supporters.SubscriptionRecord
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		locked, err = r.s.LockSubscription(ctx, tx, id)
		return err
	})
	if !reflect.DeepEqual(got, locked) {
		t.Errorf("LockSubscription = %+v, FindSubscription = %+v; want the same", locked, got)
	}
	return got
}

func (r *supporterRepo) liveID(t *testing.T, userID string) (string, error) {
	t.Helper()
	return r.s.LiveSubscriptionID(t.Context(), nil, userID)
}

// sameDuplicate reports whether two duplicates are equal, their times at the same instant.
func sameDuplicate(a, b supporters.Duplicate) bool {
	if !a.DetectedAt.Equal(b.DetectedAt) {
		return false
	}
	a.DetectedAt = b.DetectedAt
	return a == b
}

func supporterRepoProgram(t *testing.T, r *supporterRepo) {
	ctx := t.Context()
	if _, err := r.s.LastProgramChange(ctx, nil); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("LastProgramChange with none = %v, want ErrNotFound", err)
	}
	on := supporters.ProgramChange{ID: r.ids.NewID(), Enabled: true, Provider: supporterRepoProvider, ChangedByUserID: testkit.NamedID("admin"), ChangedAt: r.clock.Now()}
	r.clock.Advance(time.Second)
	off := supporters.ProgramChange{ID: r.ids.NewID(), ChangedAt: r.clock.Now()}
	for _, c := range []supporters.ProgramChange{on, off} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.InsertProgramChange(ctx, tx, c) })
		got, err := r.s.LastProgramChange(ctx, nil)
		if err != nil || got != c {
			t.Errorf("LastProgramChange = %+v, %v; want %+v", got, err, c)
		}
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			got, err = r.s.LastProgramChange(ctx, tx)
			return err
		})
		if got != c {
			t.Errorf("LastProgramChange in a transaction = %+v, want %+v", got, c)
		}
	}
}

func supporterRepoPlans(t *testing.T, r *supporterRepo) {
	ctx := t.Context()
	basic, gold := r.plan(t, "basic"), r.plan(t, "gold")
	got, err := r.s.FindPlan(ctx, nil, gold.ID)
	if err != nil || !reflect.DeepEqual(got, gold) {
		t.Errorf("FindPlan = %+v, %v; want %+v", got, err, gold)
	}
	if _, err := r.s.FindPlan(ctx, nil, r.ids.NewID()); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("FindPlan(unknown) = %v, want ErrNotFound", err)
	}

	// One plan per role.
	err = r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		return r.s.InsertPlan(ctx, tx, domain.Plan{ID: r.ids.NewID(), RoleID: basic.RoleID, Currency: "JPY", CreatedAt: r.clock.Now()})
	})
	if !errors.Is(err, supporters.ErrPlanRoleTaken) {
		t.Errorf("InsertPlan of a role taken = %v, want ErrPlanRoleTaken", err)
	}
	// One plan that is not retired per rank; a new revision becomes the current terms.
	revise := func(plan supporters.PlanRecord, rank int) (domain.PlanRevision, error) {
		rev := domain.PlanRevision{ID: r.ids.NewID(), PlanID: plan.ID, PlanTerms: plan.Revision.PlanTerms, RevisedByUserID: testkit.NamedID("admin"), RevisedAt: r.clock.Now()}
		rev.Rank, rev.Name = rank, plan.Revision.Name+" again"
		r.clock.Advance(time.Second)
		return rev, r.txErr(t, func(ctx context.Context, tx uow.Tx) error { return r.s.InsertPlanRevision(ctx, tx, rev) })
	}
	if _, err := revise(gold, basic.Revision.Rank); !errors.Is(err, supporters.ErrRankTaken) {
		t.Errorf("InsertPlanRevision at a rank taken = %v, want ErrRankTaken", err)
	}
	rev, err := revise(gold, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := r.s.FindPlan(ctx, nil, gold.ID); err != nil || got.Revision != rev {
		t.Errorf("FindPlan after a revision = %+v, %v; want the revision %+v", got.Revision, err, rev)
	}

	retiredAt := r.clock.Now()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.s.InsertPlanRetirement(ctx, tx, basic.ID, testkit.NamedID("admin"), retiredAt)
	})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.s.FindPlan(ctx, tx, basic.ID)
		if err != nil || !got.Retired || !got.RetiredAt.Equal(retiredAt) {
			t.Errorf("FindPlan of a retired plan = %+v, %v; want it retired at %v", got, err, retiredAt)
		}
		return err
	})
	// The rank of a retired plan is free.
	if _, err := revise(gold, basic.Revision.Rank); err != nil {
		t.Errorf("InsertPlanRevision at the rank of a retired plan = %v", err)
	}
	third := r.plan(t, "third")
	list, err := r.s.ListPlans(ctx, nil)
	ids := func(ps []supporters.PlanRecord) []string {
		var out []string
		for _, p := range ps {
			out = append(out, p.ID)
		}
		return out
	}
	// basic (retired) and gold both have rank 1, gold's ID is the greater.
	if want := []string{basic.ID, gold.ID, third.ID}; err != nil || !slices.Equal(ids(list), want) {
		t.Errorf("ListPlans = %v, %v; want %v, by rank and retired ones too", ids(list), err, want)
	}
}

func supporterRepoPrices(t *testing.T, r *supporterRepo) {
	ctx := t.Context()
	plan := r.plan(t, "basic")
	month := plan.Prices[domain.IntervalMonth]
	if got, err := r.s.FindPrice(ctx, nil, month.ID); err != nil || got != month {
		t.Errorf("FindPrice = %+v, %v; want %+v", got, err, month)
	}
	year := supporters.Price{
		ID: r.ids.NewID(), PlanID: plan.ID, Interval: domain.IntervalYear, AmountMinor: 5400, Currency: "JPY",
		Provider: supporterRepoProvider, Ref: "price_year", CreatedAt: r.clock.Now(),
	}
	again := year
	again.ID, again.AmountMinor = r.ids.NewID(), 1
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.s.InsertPrice(ctx, tx, year), r.s.InsertPrice(ctx, tx, again), r.s.SetPriceHead(ctx, tx, plan.ID, domain.IntervalYear, year.ID))
	})
	if got, err := r.s.FindPriceByRef(ctx, nil, supporterRepoProvider, "price_year"); err != nil || got != year {
		t.Errorf("FindPriceByRef = %+v, %v; want the first price of the ID %+v", got, err, year)
	}
	if _, err := r.s.FindPrice(ctx, nil, again.ID); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("FindPrice(a second price of an ID) = %v, want ErrNotFound", err)
	}
	if _, err := r.s.FindPriceByRef(ctx, nil, "other", "price_year"); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("FindPriceByRef(another provider) = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.s.InsertPriceArchival(ctx, tx, month.ID, r.clock.Now()), r.s.InsertPriceArchival(ctx, tx, month.ID, r.clock.Now()))
	})
	got, err := r.s.FindPlan(ctx, nil, plan.ID)
	month.Archived = true
	if err != nil || !reflect.DeepEqual(got.Prices, map[domain.Interval]supporters.Price{domain.IntervalMonth: month, domain.IntervalYear: year}) {
		t.Errorf("FindPlan prices = %+v, %v; want the archived monthly and the yearly price", got.Prices, err)
	}

	user := r.ids.NewID()
	c := supporters.Customer{ID: r.ids.NewID(), UserID: user, Provider: supporterRepoProvider, Ref: "cus_1", CreatedAt: r.clock.Now()}
	second := c
	second.ID, second.Ref = r.ids.NewID(), "cus_2"
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.s.InsertCustomer(ctx, tx, c), r.s.InsertCustomer(ctx, tx, second))
	})
	if got, err := r.s.FindCustomer(ctx, nil, supporterRepoProvider, user); err != nil || got != c {
		t.Errorf("FindCustomer = %+v, %v; want the first customer of the user %+v", got, err, c)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.EraseUser(ctx, tx, user) })
	if _, err := r.s.FindCustomer(ctx, nil, supporterRepoProvider, user); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("FindCustomer after the erasure = %v, want ErrNotFound", err)
	}
}

func supporterRepoSubscriptions(t *testing.T, r *supporterRepo) {
	ctx := t.Context()
	plan := r.plan(t, "basic")
	alice := r.ids.NewID()
	sub := r.subscribe(t, alice, plan, domain.StateActive)
	if got := r.find(t, sub.ID); !reflect.DeepEqual(got, sub) {
		t.Errorf("FindSubscription = %+v, want %+v", got, sub)
	}
	if id, err := r.liveID(t, alice); err != nil || id != sub.ID {
		t.Errorf("LiveSubscriptionID = %q, %v; want %s", id, err, sub.ID)
	}
	err := r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		price := plan.Prices[domain.IntervalMonth]
		s := supporters.Subscription{ID: r.ids.NewID(), UserID: alice, PlanID: plan.ID, PriceID: price.ID, Provider: supporterRepoProvider, CreatedAt: r.clock.Now()}
		return r.s.InsertSubscription(ctx, tx, s, supporters.TransitionRecord{
			ID: r.ids.NewID(), SubscriptionID: s.ID, Reason: domain.ReasonStarted, OccurredAt: r.clock.Now(),
			Transition: domain.Transition{State: domain.StatePending, PlanID: plan.ID, PriceID: price.ID},
		}, nil)
	})
	if !errors.Is(err, supporters.ErrLiveSubscription) {
		t.Errorf("InsertSubscription of a second live subscription = %v, want ErrLiveSubscription", err)
	}

	// A past-due subscription keeps its grace, a canceling one its scheduled change.
	failed := r.clock.Now()
	pastDue := domain.Transition{State: domain.StatePastDue, PlanID: plan.ID, PriceID: sub.PriceID, PeriodEndsAt: sub.Current.PeriodEndsAt,
		Grace: &domain.Grace{FailedAt: failed, EndsAt: failed.Add(7 * 24 * time.Hour)}}
	tr := r.transition(t, sub, pastDue, domain.ReasonSynced, &supporters.Deadline{SubscriptionID: sub.ID, Kind: domain.DeadlineGrace, DueAt: pastDue.Grace.EndsAt})
	if got := r.find(t, sub.ID); !reflect.DeepEqual(got.Current, tr) {
		t.Errorf("Current after a past due = %+v, want %+v", got.Current, tr)
	}
	scheduled := domain.Transition{State: domain.StateCanceling, PlanID: plan.ID, PriceID: sub.PriceID, PeriodEndsAt: sub.Current.PeriodEndsAt,
		Scheduled: &domain.ScheduledChange{PriceID: r.ids.NewID(), EffectiveAt: sub.Current.PeriodEndsAt}}
	tr = r.transition(t, sub, scheduled, domain.ReasonSynced, nil)
	if got := r.find(t, sub.ID); !reflect.DeepEqual(got.Current, tr) {
		t.Errorf("Current after a scheduled change = %+v, want %+v", got.Current, tr)
	}
	if due, err := r.s.DueDeadlines(ctx, r.clock.Now().Add(365*24*time.Hour), 10); err != nil || len(due) != 0 {
		t.Errorf("DueDeadlines after a transition without one = %+v, %v; want none", due, err)
	}

	// A final transition ends the live subscription: the user may subscribe again.
	r.transition(t, sub, domain.Transition{State: domain.StateExpired, PlanID: plan.ID, PriceID: sub.PriceID, PeriodEndsAt: sub.Current.PeriodEndsAt}, domain.ReasonSynced, nil)
	if _, err := r.liveID(t, alice); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("LiveSubscriptionID after the end = %v, want ErrNotFound", err)
	}
	next := r.subscribe(t, alice, plan, domain.StatePending)
	if id, err := r.liveID(t, alice); err != nil || id != next.ID {
		t.Errorf("LiveSubscriptionID of the next subscription = %q, %v; want %s", id, err, next.ID)
	}
	// A subscription inserted in a final state is not live.
	bob := r.ids.NewID()
	r.subscribe(t, bob, plan, domain.StateAbandoned)
	if _, err := r.liveID(t, bob); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("LiveSubscriptionID of an abandoned subscription = %v, want ErrNotFound", err)
	}
	if _, err := r.s.FindSubscription(ctx, nil, r.ids.NewID()); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("FindSubscription(unknown) = %v, want ErrNotFound", err)
	}
	err = r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.s.LockSubscription(ctx, tx, r.ids.NewID())
		return err
	})
	if !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("LockSubscription(unknown) = %v, want ErrNotFound", err)
	}
}

func supporterRepoCheckouts(t *testing.T, r *supporterRepo) {
	ctx := t.Context()
	plan := r.plan(t, "basic")
	pending, other := r.subscribe(t, r.ids.NewID(), plan, domain.StatePending), r.subscribe(t, r.ids.NewID(), plan, domain.StatePending)
	active := r.subscribe(t, r.ids.NewID(), plan, domain.StateActive)
	checkout := func(sub supporters.SubscriptionRecord, ref supporters.CheckoutRef, d time.Duration) supporters.CheckoutRecord {
		c := supporters.CheckoutRecord{ID: r.ids.NewID(), SubscriptionID: sub.ID, Provider: supporterRepoProvider, Ref: ref, ExpiresAt: r.clock.Now().Add(d), CreatedAt: r.clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.InsertCheckout(ctx, tx, c) })
		r.clock.Advance(time.Second)
		return c
	}
	expiring := checkout(pending, "cs_1", time.Minute)
	latest := checkout(pending, "cs_2", time.Hour)
	again := latest
	again.ID = r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.InsertCheckout(ctx, tx, again) })
	checkout(other, "cs_3", time.Hour)
	checkout(active, "cs_4", time.Hour)

	if got := r.find(t, pending.ID); got.Checkout == nil || *got.Checkout != latest {
		t.Errorf("Checkout = %+v, want the latest %+v", got.Checkout, latest)
	}
	if got, err := r.s.LiveCheckouts(ctx, nil, pending.ID, r.clock.Now()); err != nil || !slices.Equal(got, []supporters.CheckoutRef{"cs_2", "cs_1"}) {
		t.Errorf("LiveCheckouts = %v, %v; want both, newest first", got, err)
	}
	if got, err := r.s.LiveCheckouts(ctx, nil, pending.ID, expiring.ExpiresAt); err != nil || !slices.Equal(got, []supporters.CheckoutRef{"cs_2"}) {
		t.Errorf("LiveCheckouts at the first expiry = %v, %v; want the latest", got, err)
	}
	if got, err := r.s.OpenCheckouts(ctx, supporterRepoProvider, r.clock.Now()); err != nil || !slices.Equal(got, []supporters.CheckoutRef{"cs_2", "cs_3"}) {
		t.Errorf("OpenCheckouts = %v, %v; want the latest of each pending subscription", got, err)
	}
	if id, err := r.s.FindSubscriptionIDByCheckout(ctx, nil, supporterRepoProvider, "cs_1"); err != nil || id != pending.ID {
		t.Errorf("FindSubscriptionIDByCheckout = %q, %v; want %s", id, err, pending.ID)
	}
	if _, err := r.s.FindSubscriptionIDByCheckout(ctx, nil, supporterRepoProvider, "cs_9"); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("FindSubscriptionIDByCheckout(unknown) = %v, want ErrNotFound", err)
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.s.InsertLink(ctx, tx, active.ID, supporterRepoProvider, "sub_1", r.clock.Now()),
			r.s.InsertLink(ctx, tx, active.ID, supporterRepoProvider, "sub_2", r.clock.Now()),
		)
	})
	if got := r.find(t, active.ID); got.Link != "sub_1" {
		t.Errorf("Link = %q, want the first sub_1", got.Link)
	}
	if id, err := r.s.FindSubscriptionIDByRef(ctx, nil, supporterRepoProvider, "sub_1"); err != nil || id != active.ID {
		t.Errorf("FindSubscriptionIDByRef = %q, %v; want %s", id, err, active.ID)
	}
	if _, err := r.s.FindSubscriptionIDByRef(ctx, nil, supporterRepoProvider, "sub_2"); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("FindSubscriptionIDByRef(a second link) = %v, want ErrNotFound", err)
	}
}

func supporterRepoReads(t *testing.T, r *supporterRepo) {
	sub := r.subscribe(t, r.ids.NewID(), r.plan(t, "basic"), domain.StateActive)
	at := r.clock.Now()
	for _, c := range []struct {
		name string
		at   time.Time
		want bool
	}{
		{"the first", at, true},
		{"one read later", at.Add(time.Second), true},
		{"the same again", at.Add(time.Second), true},
		{"one read earlier", at, false},
	} {
		var applied bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			applied, err = r.s.MarkSubscriptionRead(ctx, tx, sub.ID, c.at)
			return err
		})
		if applied != c.want {
			t.Errorf("MarkSubscriptionRead(%s) = %t, want %t", c.name, applied, c.want)
		}
	}
}

func supporterRepoDuplicates(t *testing.T, r *supporterRepo) {
	ctx := t.Context()
	sub := r.subscribe(t, r.ids.NewID(), r.plan(t, "basic"), domain.StateActive)
	dup := func(ref supporters.SubscriptionRef) supporters.Duplicate {
		d := supporters.Duplicate{ID: r.ids.NewID(), SubscriptionID: sub.ID, Provider: supporterRepoProvider, Ref: ref, DetectedAt: r.clock.Now()}
		var ok bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			ok, err = r.s.InsertDuplicate(ctx, tx, d)
			return err
		})
		if !ok {
			t.Errorf("InsertDuplicate(%s) = false, want true", ref)
		}
		r.clock.Advance(time.Second)
		return d
	}
	first, second := dup("sub_dup_1"), dup("sub_dup_2")
	var ok bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		ok, err = r.s.InsertDuplicate(ctx, tx, supporters.Duplicate{ID: r.ids.NewID(), SubscriptionID: sub.ID, Provider: supporterRepoProvider, Ref: "sub_dup_1", DetectedAt: r.clock.Now()})
		return err
	})
	if ok {
		t.Error("InsertDuplicate of a recorded one = true, want false")
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.s.CancelDuplicate(ctx, tx, first.ID, r.clock.Now()), r.s.CancelDuplicate(ctx, tx, first.ID, r.clock.Now()))
	})
	first.Canceling = true
	if got, err := r.s.FindDuplicate(ctx, nil, supporterRepoProvider, "sub_dup_1"); err != nil || !sameDuplicate(got, first) {
		t.Errorf("FindDuplicate = %+v, %v; want %+v", got, err, first)
	}
	if got, err := r.s.UnsettledDuplicates(ctx, supporterRepoProvider); err != nil || len(got) != 2 || got[0].ID != first.ID || got[1].ID != second.ID || !got[1].DetectedAt.Equal(second.DetectedAt) {
		t.Errorf("UnsettledDuplicates = %+v, %v; want both, oldest first", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.s.SettleDuplicate(ctx, tx, first.ID, r.clock.Now()), r.s.SettleDuplicate(ctx, tx, first.ID, r.clock.Now()))
	})
	first.Settled = true
	if got, err := r.s.FindDuplicate(ctx, nil, supporterRepoProvider, "sub_dup_1"); err != nil || !sameDuplicate(got, first) {
		t.Errorf("FindDuplicate after the settlement = %+v, %v; want %+v", got, err, first)
	}
	if got, err := r.s.UnsettledDuplicates(ctx, supporterRepoProvider); err != nil || len(got) != 1 || got[0].ID != second.ID {
		t.Errorf("UnsettledDuplicates after a settlement = %+v, %v; want the second", got, err)
	}
	if _, err := r.s.FindDuplicate(ctx, nil, supporterRepoProvider, "sub_dup_9"); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("FindDuplicate(unknown) = %v, want ErrNotFound", err)
	}

	reverse := func(kind supporters.ReversalKind, providerID string) bool {
		var ok bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			ok, err = r.s.InsertReversal(ctx, tx, supporters.Reversal{
				ID: r.ids.NewID(), SubscriptionID: sub.ID, Kind: kind, Provider: supporterRepoProvider, ProviderID: providerID, OccurredAt: r.clock.Now(),
			})
			return err
		})
		return ok
	}
	revoking := func() bool {
		var ok bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			ok, err = r.s.HasRevokingReversal(ctx, tx, sub.ID)
			return err
		})
		return ok
	}
	if !reverse(supporters.ReversalPartialRefund, "re_1") || revoking() {
		t.Error("a partial refund: not recorded, or revoking")
	}
	if reverse(supporters.ReversalFullRefund, "re_1") {
		t.Error("InsertReversal of a recorded ID = true, want false")
	}
	if !reverse(supporters.ReversalChargeback, "dp_1") || !revoking() {
		t.Error("a chargeback: not recorded, or not revoking")
	}
	if got, err := r.s.KnownReversals(ctx, supporterRepoProvider, []string{"re_9", "re_1", "dp_1"}); err != nil || !slices.Equal(got, []string{"dp_1", "re_1"}) {
		t.Errorf("KnownReversals = %v, %v; want dp_1 and re_1", got, err)
	}
}

func supporterRepoLists(t *testing.T, r *supporterRepo) {
	ctx := t.Context()
	basic, gold := r.plan(t, "basic"), r.plan(t, "gold")
	a := r.subscribe(t, r.ids.NewID(), basic, domain.StateActive)
	b := r.subscribe(t, r.ids.NewID(), gold, domain.StateActive)
	c := r.subscribe(t, r.ids.NewID(), basic, domain.StatePending)
	d := r.subscribe(t, r.ids.NewID(), basic, domain.StateAbandoned)
	ids := func(recs []supporters.SubscriptionRecord) []string {
		var out []string
		for _, rec := range recs {
			out = append(out, rec.ID)
		}
		return out
	}
	for _, f := range []struct {
		name string
		f    supporters.SubscriptionFilter
		want []string
	}{
		{"every subscription", supporters.SubscriptionFilter{Limit: 10}, []string{d.ID, c.ID, b.ID, a.ID}},
		{"of a plan", supporters.SubscriptionFilter{PlanID: basic.ID, Limit: 10}, []string{d.ID, c.ID, a.ID}},
		{"active", supporters.SubscriptionFilter{State: domain.StateActive, Limit: 10}, []string{b.ID, a.ID}},
		{"a page", supporters.SubscriptionFilter{Limit: 2}, []string{d.ID, c.ID}},
		{"the next page", supporters.SubscriptionFilter{Before: c.ID, Limit: 2}, []string{b.ID, a.ID}},
	} {
		got, err := r.s.ListSubscriptions(ctx, f.f)
		if err != nil || !slices.Equal(ids(got), f.want) {
			t.Errorf("ListSubscriptions(%s) = %v, %v; want %v", f.name, ids(got), err, f.want)
		}
	}
	if got, err := r.s.ListSubscriptions(ctx, supporters.SubscriptionFilter{State: domain.StateActive, PlanID: gold.ID, Limit: 1}); err != nil ||
		len(got) != 1 || !reflect.DeepEqual(got[0], b) {
		t.Errorf("ListSubscriptions(gold) = %+v, %v; want %+v", got, err, b)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		live, err := r.s.LiveSubscriptionIDs(ctx, tx)
		if want := testkit.SortedIDs(a.ID, b.ID, c.ID); err != nil || !slices.Equal(live, want) {
			t.Errorf("LiveSubscriptionIDs = %v, %v; want %v", live, err, want)
		}
		ofBasic, err := r.s.LiveSubscriptionIDsOfPlan(ctx, tx, basic.ID)
		if want := testkit.SortedIDs(a.ID, c.ID); err != nil || !slices.Equal(ofBasic, want) {
			t.Errorf("LiveSubscriptionIDsOfPlan = %v, %v; want %v", ofBasic, err, want)
		}
		n, err := r.s.CountLiveSubscriptions(ctx, tx)
		if err != nil || n != 3 {
			t.Errorf("CountLiveSubscriptions = %d, %v; want 3", n, err)
		}
		return err
	})
	if n, err := r.s.CountLiveSubscriptions(ctx, nil); err != nil || n != 3 {
		t.Errorf("CountLiveSubscriptions outside a transaction = %d, %v; want 3", n, err)
	}
	if got, err := r.s.CountMembers(ctx); err != nil || !reflect.DeepEqual(got, map[string]int64{basic.ID: 1, gold.ID: 1}) {
		t.Errorf("CountMembers = %v, %v; want one member of each plan", got, err)
	}
	got, err := r.s.SubscriptionUsers(ctx, []string{b.ID, r.ids.NewID(), a.ID})
	slices.SortFunc(got, func(x, y supporters.SubscriptionUser) int { return strings.Compare(x.SubscriptionID, y.SubscriptionID) })
	want := []supporters.SubscriptionUser{{SubscriptionID: a.ID, UserID: a.UserID, PlanID: basic.ID}, {SubscriptionID: b.ID, UserID: b.UserID, PlanID: gold.ID}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("SubscriptionUsers = %+v, %v; want %+v", got, err, want)
	}
}

func supporterRepoListings(t *testing.T, r *supporterRepo) {
	ctx := t.Context()
	basic, gold, retired := r.plan(t, "basic"), r.plan(t, "gold"), r.plan(t, "retired")
	listed := func(plan supporters.PlanRecord, state domain.State) supporters.Listing {
		user := r.ids.NewID()
		r.subscribe(t, user, plan, state)
		at := r.clock.Now()
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return errors.Join(r.s.InsertListing(ctx, tx, user, at), r.s.InsertListing(ctx, tx, user, at.Add(time.Hour)))
		})
		r.clock.Advance(time.Second)
		return supporters.Listing{UserID: user, PlanID: plan.ID, Rank: plan.Revision.Rank, ListedAt: at}
	}
	b1, g1, b2 := listed(basic, domain.StateActive), listed(gold, domain.StateCanceling), listed(basic, domain.StatePastDue)
	listed(basic, domain.StatePending)
	listed(retired, domain.StateActive)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.s.InsertPlanRetirement(ctx, tx, retired.ID, "", r.clock.Now())
	})
	unlisted := r.ids.NewID()
	r.subscribe(t, unlisted, basic, domain.StateActive)

	if ok, err := r.s.HasListing(ctx, nil, b1.UserID); err != nil || !ok {
		t.Errorf("HasListing = %t, %v; want true", ok, err)
	}
	if ok, err := r.s.HasListing(ctx, nil, unlisted); err != nil || ok {
		t.Errorf("HasListing(unlisted) = %t, %v; want false", ok, err)
	}
	for _, c := range []struct {
		name string
		f    supporters.ListingFilter
		want []supporters.Listing
	}{
		{"every member", supporters.ListingFilter{Limit: 10}, []supporters.Listing{b2, b1, g1}},
		{"of a plan", supporters.ListingFilter{PlanID: gold.ID, Limit: 10}, []supporters.Listing{g1}},
		{"a page", supporters.ListingFilter{Limit: 1}, []supporters.Listing{b2}},
		{"the next page", supporters.ListingFilter{After: &b2, Limit: 1}, []supporters.Listing{b1}},
		{"the last page", supporters.ListingFilter{After: &b1, Limit: 10}, []supporters.Listing{g1}},
	} {
		got, err := r.s.ListListings(ctx, c.f)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("ListListings(%s) = %+v, %v; want %+v", c.name, got, err, c.want)
		}
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.DeleteListing(ctx, tx, b2.UserID) })
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.EraseUser(ctx, tx, g1.UserID) })
	if got, err := r.s.ListListings(ctx, supporters.ListingFilter{Limit: 10}); err != nil || !reflect.DeepEqual(got, []supporters.Listing{b1}) {
		t.Errorf("ListListings after a delete and an erasure = %+v, %v; want %+v", got, err, b1)
	}
}

func supporterRepoRebuild(t *testing.T, r *supporterRepo) {
	ctx := t.Context()
	basic, retired := r.plan(t, "basic"), r.plan(t, "retired")
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.s.InsertPlanRetirement(ctx, tx, retired.ID, "", r.clock.Now())
	})
	active := r.subscribe(t, r.ids.NewID(), basic, domain.StateActive)
	pending := r.subscribe(t, r.ids.NewID(), basic, domain.StatePending)
	pastDue := r.subscribe(t, r.ids.NewID(), basic, domain.StateActive)
	failed := r.clock.Now()
	r.transition(t, pastDue, domain.Transition{State: domain.StatePastDue, PlanID: basic.ID, PriceID: pastDue.PriceID, PeriodEndsAt: pastDue.Current.PeriodEndsAt,
		Grace: &domain.Grace{FailedAt: failed, EndsAt: failed.Add(7 * 24 * time.Hour)}}, domain.ReasonSynced,
		&supporters.Deadline{SubscriptionID: pastDue.ID, Kind: domain.DeadlineGrace, DueAt: failed.Add(7 * 24 * time.Hour)})
	ended := r.subscribe(t, r.ids.NewID(), basic, domain.StateActive)
	r.transition(t, ended, domain.Transition{State: domain.StateExpired, PlanID: basic.ID, PriceID: ended.PriceID, PeriodEndsAt: ended.Current.PeriodEndsAt}, domain.ReasonSynced, nil)

	type snapshot struct {
		plans     []supporters.PlanRecord
		subs      []supporters.SubscriptionRecord
		live      []string
		deadlines []supporters.Deadline
	}
	snap := func() snapshot {
		t.Helper()
		var s snapshot
		var err error
		s.plans, err = r.s.ListPlans(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, sub := range []supporters.SubscriptionRecord{active, pending, pastDue, ended} {
			s.subs = append(s.subs, r.find(t, sub.ID))
		}
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			s.live, err = r.s.LiveSubscriptionIDs(ctx, tx)
			return err
		})
		if s.deadlines, err = r.s.DueDeadlines(ctx, r.clock.Now().Add(365*24*time.Hour), 10); err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := snap()
	if len(before.deadlines) != 3 || before.deadlines[0].SubscriptionID != pending.ID || before.deadlines[0].Kind != domain.DeadlineCheckout ||
		before.deadlines[1].SubscriptionID != pastDue.ID || before.deadlines[2].SubscriptionID != active.ID {
		t.Errorf("DueDeadlines = %+v; want the checkout, the grace, and the period end, the earliest first", before.deadlines)
	}
	if due, err := r.s.DueDeadlines(ctx, r.clock.Now().Add(365*24*time.Hour), 1); err != nil || len(due) != 1 || due[0] != before.deadlines[0] {
		t.Errorf("DueDeadlines(limit 1) = %+v, %v; want the earliest", due, err)
	}
	if due, err := r.s.DueDeadlines(ctx, r.clock.Now(), 10); err != nil || len(due) != 0 {
		t.Errorf("DueDeadlines(now) = %+v, %v; want none", due, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.RebuildProjections(ctx, tx) })
	if after := snap(); !reflect.DeepEqual(after, before) {
		t.Errorf("after RebuildProjections = %+v,\nwant %+v", after, before)
	}
}
