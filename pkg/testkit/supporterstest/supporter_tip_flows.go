// SPDX-License-Identifier: AGPL-3.0-only

package supporterstest

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/pkg/testkit"
)

// The flows of the tips (ADR-0162, FR-918) on the fake provider's one-time payments.

// tipWorld is the world's side of the one-time payments: the fake adapter.
type tipWorld interface {
	supporters.OneTimePayments
	PayOneTime(ref supporters.CheckoutRef) (supporters.OneTimeResult, string, error)
}

// incomeRecords is an IncomeRecorder that keeps each movement once by its ID, with the source of
// its payment for a refund, a chargeback, or a reversal, as the income records do.
type incomeRecords struct {
	mu   sync.Mutex
	rows map[string]appfinance.IncomeMovement
	// order is the IDs in the order they were recorded.
	order []string
}

func (r *incomeRecords) RecordIncome(_ context.Context, ms []appfinance.IncomeMovement) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rows == nil {
		r.rows = map[string]appfinance.IncomeMovement{}
	}
	for _, m := range ms {
		if _, ok := r.rows[m.ProviderID]; ok {
			continue
		}
		if m.ParentID != "" {
			parent, ok := r.rows[m.ParentID]
			if !ok {
				return appfinance.ErrIncomeParentMissing
			}
			m.Source = parent.Source
		}
		r.rows[m.ProviderID] = m
		r.order = append(r.order, m.ProviderID)
	}
	return nil
}

func (r *incomeRecords) KnownIncome(_ context.Context, _ string, ids []string) (map[string]bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]bool{}
	for _, id := range ids {
		if _, ok := r.rows[id]; ok {
			out[id] = true
		}
	}
	return out, nil
}

func (r *incomeRecords) get(id string) (appfinance.IncomeMovement, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.rows[id]
	return m, ok
}

func (r *incomeRecords) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.order)
}

// tipEnv is a supporterEnv whose service has the income records, the rate limits, and the tips
// enabled with 300, 500, and 1000 yen.
type tipEnv struct {
	*supporterEnv
	world  tipWorld
	income *incomeRecords
}

func newTipEnv(t *testing.T, e *supporterEnv) *tipEnv {
	t.Helper()
	world, ok := e.world.(tipWorld)
	if !ok {
		t.Skip("the payment world takes no one-time payments")
	}
	te := &tipEnv{supporterEnv: e, world: world, income: &incomeRecords{}}
	e.deps.Income = te.income
	e.deps.RateLimiter = kernel.NewRateLimiter(testkit.NewKeyValueStore(e.Clock), e.Clock)
	e.svc = supporters.NewService(e.deps)
	e.handlers = &queue.JobHandlerRegistry{}
	e.svc.RegisterJobs(e.handlers, nil)
	e.enable(t)
	if _, err := e.svc.SetTips(t.Context(), supporters.TipSettingsInput{Enabled: true, Currency: "JPY", Amounts: []int64{1000, 300, 500}}, ""); err != nil {
		t.Fatalf("SetTips = %v", err)
	}
	return te
}

// tip starts a tip of amount yen and returns it with its checkout.
func (e *tipEnv) tip(t *testing.T, userID string, amount int64) (supporters.StartedTip, supporters.CheckoutRef) {
	t.Helper()
	st, err := e.svc.StartTip(t.Context(), supporters.TipInput{
		UserID: userID, AmountMinor: amount, Currency: "JPY", Description: "Tip for my.example", ReturnURL: "https://" + testkit.ContractHost + "/settings/supporter/tip",
	})
	if err != nil {
		t.Fatalf("StartTip(%d) = %v", amount, err)
	}
	v := e.ownTip(t, userID, st.TipID)
	if v.Checkout == nil {
		return st, ""
	}
	return st, v.Checkout.Ref
}

func (e *tipEnv) ownTip(t *testing.T, userID, id string) supporters.TipView {
	t.Helper()
	v, err := e.svc.OwnTip(t.Context(), userID, id)
	if err != nil {
		t.Fatalf("OwnTip(%s) = %v", id, err)
	}
	return v
}

// pay pays the checkout at the provider, re-reads it as the return does, and returns the money
// movement of the payment, which the provider booked.
func (e *tipEnv) pay(t *testing.T, userID, tipID string, ref supporters.CheckoutRef) supporters.MoneyMovement {
	t.Helper()
	if _, _, err := e.world.PayOneTime(ref); err != nil {
		t.Fatal(err)
	}
	e.ownTip(t, userID, tipID)
	e.run(t)
	if v := e.ownTip(t, userID, tipID); v.State != supporters.TipStatePaid {
		t.Fatalf("tip after its payment: %s, want paid", v.State)
	}
	return e.lastMoney(t)
}

// lastMoney returns the newest money movement at the provider.
func (e *tipEnv) lastMoney(t *testing.T) supporters.MoneyMovement {
	t.Helper()
	var all []supporters.MoneyMovement
	cursor := ""
	for {
		page, err := e.supporterEnv.world.ListMoney(t.Context(), time.Time{}, cursor)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, page.Movements...)
		cursor = page.Next
		if !page.More {
			break
		}
	}
	if len(all) == 0 {
		t.Fatal("no money at the provider")
	}
	return all[len(all)-1]
}

// child returns a refund, a chargeback, or a reversal of payment p.
func child(id string, kind supporters.MoneyKind, p supporters.MoneyMovement, gross string, at time.Time) supporters.MoneyMovement {
	return supporters.MoneyMovement{
		ID: id, Kind: kind, ParentID: p.ID, ParentGross: p.Gross, Payment: p.Payment, OccurredAt: at,
		Gross: gross, Fee: "0", Net: gross, Currency: p.Currency, Final: true,
	}
}

func supporterTipSettings(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	if _, ok := e.world.(tipWorld); !ok {
		t.Skip("the payment world takes no one-time payments")
	}
	in := supporters.TipSettingsInput{Enabled: true, Currency: "JPY", Amounts: []int64{500}}
	if _, err := e.svc.SetTips(ctx, in, ""); !errors.Is(err, supporters.ErrTipsUnavailable) {
		t.Errorf("SetTips before the program = %v, want ErrTipsUnavailable", err)
	}
	e.enable(t)
	var te *supporters.TipAmountsError
	for _, bad := range []struct {
		in   supporters.TipSettingsInput
		rule supporters.TipAmountsRule
	}{
		{supporters.TipSettingsInput{Enabled: true, Currency: "JPY"}, supporters.TipAmountsCount},
		{supporters.TipSettingsInput{Enabled: true, Currency: "JPY", Amounts: []int64{100, 200, 300, 400, 500, 600, 700}}, supporters.TipAmountsCount},
		{supporters.TipSettingsInput{Enabled: true, Currency: "JPY", Amounts: []int64{500, 500}}, supporters.TipAmountsCount},
		{supporters.TipSettingsInput{Enabled: true, Currency: "JPY", Amounts: []int64{99}}, supporters.TipAmountsRange},
		{supporters.TipSettingsInput{Enabled: true, Currency: "JPY", Amounts: []int64{100_001}}, supporters.TipAmountsRange},
		{supporters.TipSettingsInput{Enabled: true, Currency: "USD", Amounts: []int64{99}}, supporters.TipAmountsRange},
		{supporters.TipSettingsInput{Enabled: true, Currency: "EUR", Amounts: []int64{500}}, supporters.TipAmountsCurrency},
	} {
		if _, err := e.svc.SetTips(ctx, bad.in, ""); !errors.As(err, &te) || te.Rule != bad.rule {
			t.Errorf("SetTips(%+v) = %v, want the rule %s", bad.in, err, bad.rule)
		}
	}
	// No plan is needed.
	in.Amounts = []int64{1000, 300, 500}
	got, err := e.svc.SetTips(ctx, in, "")
	if err != nil || !got.Enabled || !got.Accepting || !slices.Equal(got.Amounts, []int64{300, 500, 1000}) || !got.OneTimePayments {
		t.Fatalf("SetTips = %+v, %v, want them enabled with the amounts in order", got, err)
	}
	if _, err := e.svc.SetTips(ctx, in, ""); err != nil {
		t.Fatal(err)
	}
	in.Amounts = []int64{500, 2000}
	if _, err := e.svc.SetTips(ctx, in, ""); err != nil {
		t.Fatal(err)
	}
	cur, amounts, err := e.svc.TipAmounts(ctx)
	if err != nil || cur != "JPY" || !slices.Equal(amounts, []int64{500, 2000}) {
		t.Errorf("TipAmounts = %s %v, %v", cur, amounts, err)
	}
	if on, err := e.svc.TipsEnabled(ctx); err != nil || !on {
		t.Errorf("TipsEnabled = %v, %v", on, err)
	}
	if _, err := e.svc.SetTips(ctx, supporters.TipSettingsInput{}, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.TipAmounts(ctx); !errors.Is(err, supporters.ErrTipsDisabled) {
		t.Errorf("TipAmounts after disabling = %v, want ErrTipsDisabled", err)
	}
	var tips []moderation.AuditAction
	for _, a := range e.actions() {
		if strings.HasPrefix(string(a), "supporter_tips_") {
			tips = append(tips, a)
		}
	}
	want := []moderation.AuditAction{moderation.AuditSupporterTipsEnabled, moderation.AuditSupporterTipsRevised, moderation.AuditSupporterTipsDisabled}
	if !slices.Equal(tips, want) {
		t.Errorf("audit = %v, want %v (the same amounts again record nothing)", tips, want)
	}
}

func supporterTipPayment(t *testing.T, se *supporterEnv) {
	ctx := t.Context()
	e := newTipEnv(t, se)
	alice := e.user("alice")
	if _, err := e.svc.StartTip(ctx, supporters.TipInput{UserID: alice, AmountMinor: 700, Currency: "JPY"}); !errors.Is(err, supporters.ErrTipAmount) {
		t.Errorf("StartTip(700) = %v, want ErrTipAmount", err)
	}
	unverified := e.user("carol")
	e.users.put(supporters.User{ID: unverified, AccountID: testkit.NamedID("acct-carol"), Handle: "@carol", Email: "carol@example.com"})
	var ie *supporters.IneligibleError
	if _, err := e.svc.StartTip(ctx, supporters.TipInput{UserID: unverified, AmountMinor: 500, Currency: "JPY"}); !errors.As(err, &ie) {
		t.Errorf("StartTip of an unverified user = %v, want ineligible", err)
	}
	st, ref := e.tip(t, alice, 500)
	if ref == "" || !strings.HasPrefix(st.RedirectURL, "https://") {
		t.Fatalf("StartTip = %+v, want a checkout", st)
	}
	if req, ok := e.world.(interface {
		OneTimeRequest(supporters.CheckoutRef) (supporters.OneTimeRequest, bool)
	}); ok {
		r, _ := req.OneTimeRequest(ref)
		if r.LocalID != st.TipID || r.AmountMinor != 500 || r.Email != "alice@example.com" || !strings.Contains(r.SuccessURL, "tip="+st.TipID) {
			t.Errorf("the checkout's request = %+v, want the tip, its amount, and the user's address", r)
		}
	}
	if _, err := e.svc.OwnTip(ctx, e.user("dave"), st.TipID); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("OwnTip of another user = %v, want ErrNotFound", err)
	}
	pay := e.pay(t, alice, st.TipID, ref)
	// The webhook and the reconciliation both hand the payment over: it is booked once.
	for range 2 {
		if err := e.svc.RecordMoney(ctx, paymentProvider, []supporters.MoneyMovement{pay}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.SyncTip(ctx, paymentProvider, supporters.OneTimeResult{Ref: ref, Status: supporters.OneTimePaid, LocalID: st.TipID, Payment: pay.Payment, AmountMinor: 500, Currency: "JPY"}); err != nil {
			t.Fatal(err)
		}
	}
	if m, ok := e.income.get(pay.ID); !ok || m.Source != string(finance.SourceOneTime) || m.TipID != st.TipID || m.SubscriptionID != "" || m.CoversStart != "" {
		t.Errorf("income of the tip = %+v, %v, want one_time with its tip and no period", m, ok)
	}
	if n := e.income.count(); n != 1 {
		t.Errorf("income rows = %d, want 1", n)
	}
	// A partial refund is income alone; a refund in full is an outcome.
	now := e.Clock.Now()
	partial := child("txn_tip_partial", supporters.MoneyRefund, pay, "-100", now)
	full := child("txn_tip_full", supporters.MoneyRefund, pay, "-400", now)
	full.Gross, full.Net = "-500", "-500"
	if err := e.svc.RecordMoney(ctx, paymentProvider, []supporters.MoneyMovement{partial}); err != nil {
		t.Fatal(err)
	}
	if v := e.ownTip(t, alice, st.TipID); len(v.Outcomes) != 0 {
		t.Errorf("outcomes after a partial refund = %v, want none", v.Outcomes)
	}
	if err := e.svc.RecordMoney(ctx, paymentProvider, []supporters.MoneyMovement{full}); err != nil {
		t.Fatal(err)
	}
	if v := e.ownTip(t, alice, st.TipID); !v.Has(supporters.TipRefunded) || v.State != supporters.TipStatePaid {
		t.Errorf("tip after a full refund = %+v, want paid and refunded", v)
	}
	if m, _ := e.income.get(full.ID); m.Source != string(finance.SourceOneTime) {
		t.Errorf("source of the refund = %s, want one_time", m.Source)
	}
	// Another tip is charged back and the dispute is won.
	bob := e.user("bob")
	st2, ref2 := e.tip(t, bob, 1000)
	pay2 := e.pay(t, bob, st2.TipID, ref2)
	later := e.Clock.Advance(time.Hour)
	for _, m := range []supporters.MoneyMovement{pay2, child("txn_tip_cb", supporters.MoneyChargeback, pay2, "-1000", later), child("txn_tip_cbr", supporters.MoneyChargebackReversal, pay2, "1000", later)} {
		if err := e.svc.RecordMoney(ctx, paymentProvider, []supporters.MoneyMovement{m}); err != nil {
			t.Fatal(err)
		}
	}
	if v := e.ownTip(t, bob, st2.TipID); !v.Has(supporters.TipChargedBack) || !v.Has(supporters.TipDisputeWon) {
		t.Errorf("outcomes = %v, want charged_back and dispute_won", v.Outcomes)
	}
	if users, err := e.svc.TipUsers(ctx, []string{st.TipID, st2.TipID, testkit.NamedID("no-tip")}); err != nil || users[st.TipID] != alice || users[st2.TipID] != bob || len(users) != 2 {
		t.Errorf("TipUsers = %v, %v", users, err)
	}
}

// A payment that the provider booked before the notice of its checkout came still finds its tip.
func supporterTipMoneyFirst(t *testing.T, se *supporterEnv) {
	ctx := t.Context()
	e := newTipEnv(t, se)
	alice := e.user("alice")
	st, ref := e.tip(t, alice, 300)
	if _, _, err := e.world.PayOneTime(ref); err != nil {
		t.Fatal(err)
	}
	pay := e.lastMoney(t)
	if err := e.svc.RecordMoney(ctx, paymentProvider, []supporters.MoneyMovement{pay}); err != nil {
		t.Fatal(err)
	}
	if m, _ := e.income.get(pay.ID); m.Source != string(finance.SourceOneTime) || m.TipID != st.TipID {
		t.Errorf("income = %+v, want the tip's", m)
	}
	// A payment of no tip of ours is of an unknown source.
	other := pay
	other.ID, other.Payment = "txn_elsewhere", "pi_elsewhere"
	if err := e.svc.RecordMoney(ctx, paymentProvider, []supporters.MoneyMovement{other}); err != nil {
		t.Fatal(err)
	}
	if m, _ := e.income.get(other.ID); m.Source != string(finance.SourceUnknown) || m.TipID != "" {
		t.Errorf("income of a payment outside my = %+v, want unknown", m)
	}
}

// Starting again expires the open tip, and a start after the payment goes to its return.
func supporterTipAgain(t *testing.T, se *supporterEnv) {
	ctx := t.Context()
	e := newTipEnv(t, se)
	alice := e.user("alice")
	first, ref1 := e.tip(t, alice, 300)
	second, ref2 := e.tip(t, alice, 500)
	if res, err := e.world.GetOneTimeCheckout(ctx, ref1); err != nil || res.Status != supporters.OneTimeExpired {
		t.Errorf("the first checkout after a second start = %+v, %v, want expired", res, err)
	}
	if v := e.ownTip(t, alice, first.TipID); v.State != supporters.TipStateExpired {
		t.Errorf("first tip = %s, want expired", v.State)
	}
	// The second is paid in another tab; starting again sends the browser to its return.
	if _, _, err := e.world.PayOneTime(ref2); err != nil {
		t.Fatal(err)
	}
	again, err := e.svc.StartTip(ctx, supporters.TipInput{UserID: alice, AmountMinor: 1000, Currency: "JPY", ReturnURL: "https://" + testkit.ContractHost + "/tip"})
	if err != nil || again.TipID != second.TipID || !strings.Contains(again.RedirectURL, "checkout=success") {
		t.Fatalf("StartTip after the payment = %+v, %v, want the return of %s", again, err, second.TipID)
	}
	if v := e.ownTip(t, alice, second.TipID); v.State != supporters.TipStatePaid {
		t.Errorf("second tip = %s, want paid", v.State)
	}
}

// An open tip expires with its checkout, and the account's closing expires its checkout.
func supporterTipExpiry(t *testing.T, se *supporterEnv) {
	ctx := t.Context()
	e := newTipEnv(t, se)
	alice := e.user("alice")
	st, ref := e.tip(t, alice, 300)
	e.run(t)
	e.Clock.Advance(supporters.TipCheckoutLifetime + time.Minute)
	if err := e.svc.QueueDeadlines(ctx); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	if v := e.ownTip(t, alice, st.TipID); v.State != supporters.TipStateExpired {
		t.Errorf("tip after its checkout's end = %s, want expired", v.State)
	}
	if res, _ := e.world.GetOneTimeCheckout(ctx, ref); res.Status != supporters.OneTimeExpired {
		t.Errorf("checkout = %s, want expired", res.Status)
	}
	// A tip whose checkout the job expires, as the deletion of the account queues it.
	st2, ref2 := e.tip(t, alice, 500)
	if err := e.jobs.Enqueue(ctx, supporters.SyncTipJob(st2.TipID, "account_closed:test", true)); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	if res, _ := e.world.GetOneTimeCheckout(ctx, ref2); res.Status != supporters.OneTimeExpired {
		t.Errorf("checkout after the account closed = %s, want expired", res.Status)
	}
	if v := e.ownTip(t, alice, st2.TipID); v.State != supporters.TipStateExpired {
		t.Errorf("tip after the account closed = %s, want expired", v.State)
	}
}

// The starts of a user are limited by the hour.
func supporterTipLimits(t *testing.T, se *supporterEnv) {
	ctx := t.Context()
	e := newTipEnv(t, se)
	alice := e.user("alice")
	for range supporters.TipUserHourLimit.Max {
		e.tip(t, alice, 300)
	}
	var limited *kernel.RateLimitedError
	if _, err := e.svc.StartTip(ctx, supporters.TipInput{UserID: alice, AmountMinor: 300, Currency: "JPY"}); !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Errorf("StartTip beyond the limit = %v, want a rate limit", err)
	}
	if _, ref := e.tip(t, e.user("bob"), 300); ref == "" {
		t.Error("another user's tip was refused")
	}
}

// The open tips rebuild to what the use cases wrote.
func supporterTipRebuild(t *testing.T, se *supporterEnv) {
	ctx := t.Context()
	e := newTipEnv(t, se)
	alice, bob, carol := e.user("alice"), e.user("bob"), e.user("carol")
	e.tip(t, alice, 300)
	e.tip(t, alice, 500) // expires the first
	st, ref := e.tip(t, bob, 300)
	e.pay(t, bob, st.TipID, ref)
	e.tip(t, carol, 1000)
	e.run(t)
	snapshot := func() []any {
		due, err := e.h.Supporters.DueOpenTips(ctx, e.Clock.Now().Add(24*time.Hour), 10)
		if err != nil {
			t.Fatal(err)
		}
		refs, err := e.h.Supporters.OpenTipCheckouts(ctx, paymentProvider, e.Clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		var open []string
		for _, u := range []string{alice, bob, carol} {
			id, err := e.h.Supporters.OpenTipID(ctx, nil, u)
			if err != nil && !errors.Is(err, supporters.ErrNotFound) {
				t.Fatal(err)
			}
			open = append(open, id)
		}
		return []any{due, refs, open}
	}
	before := snapshot()
	if n := len(before[0].([]string)); n != 2 {
		t.Fatalf("open tips = %d, want 2 (alice's second and carol's)", n)
	}
	if err := supporters.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Supporters); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(); !reflect.DeepEqual(after, before) {
		t.Errorf("after the rebuild = %v, want %v", after, before)
	}
}

// The erasure of an account deletes its tips; their income stays without a user.
func supporterTipErasure(t *testing.T, se *supporterEnv) {
	ctx := t.Context()
	e := newTipEnv(t, se)
	alice := e.user("alice")
	st, ref := e.tip(t, alice, 300)
	pay := e.pay(t, alice, st.TipID, ref)
	if err := e.svc.RecordMoney(ctx, paymentProvider, []supporters.MoneyMovement{pay}); err != nil {
		t.Fatal(err)
	}
	open, _ := e.tip(t, alice, 500)
	if _, err := e.svc.EraseAccountData(ctx, account.ErasureTarget{UserID: alice}, "", 0); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{st.TipID, open.TipID} {
		if _, err := e.svc.OwnTip(ctx, alice, id); !errors.Is(err, supporters.ErrNotFound) {
			t.Errorf("OwnTip(%s) after the erasure = %v, want ErrNotFound", id, err)
		}
	}
	if users, err := e.svc.TipUsers(ctx, []string{st.TipID}); err != nil || len(users) != 0 {
		t.Errorf("TipUsers after the erasure = %v, %v, want none", users, err)
	}
	if m, ok := e.income.get(pay.ID); !ok || m.Source != string(finance.SourceOneTime) {
		t.Errorf("income after the erasure = %+v, want it kept", m)
	}
	// A refund of the erased tip is still of the tips.
	refund := child("txn_tip_after", supporters.MoneyRefund, pay, "-300", e.Clock.Now())
	if err := e.svc.RecordMoney(ctx, paymentProvider, []supporters.MoneyMovement{refund}); err != nil {
		t.Fatal(err)
	}
	if m, _ := e.income.get(refund.ID); m.Source != string(finance.SourceOneTime) {
		t.Errorf("source of a refund after the erasure = %s, want one_time", m.Source)
	}
}
