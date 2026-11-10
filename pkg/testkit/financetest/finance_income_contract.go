// SPDX-License-Identifier: AGPL-3.0-only

package financetest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/pkg/testkit"
)

// The income records of FinanceRepositoryContract (ADR-0067): each money movement is recorded
// once however often it comes, a refund waits for its payment, a movement that breaks the rules
// is left out, the totals place the yearly payments and their refunds by ADR-0067 §15 whatever
// window they are read in, the balance subtracts the costs, and the listing pages through the
// movements newest first with the subscribers' accounts.

// FakeSupporterDirectory is an appfinance.SupporterDirectory with fixed plans, accounts of
// subscriptions and tips, provider rules, and whether the tips are enabled.
type FakeSupporterDirectory struct {
	Plans    []appfinance.PlanCount
	Accounts map[string]appfinance.Subscriber
	// Tippers are the accounts of the users of the tips, by tip ID.
	Tippers map[string]appfinance.Subscriber
	Rules   []appfinance.ProviderRule
	Tips    bool
}

var _ appfinance.SupporterDirectory = (*FakeSupporterDirectory)(nil)

// PlanCounts implements appfinance.SupporterDirectory.
func (d *FakeSupporterDirectory) PlanCounts(context.Context) ([]appfinance.PlanCount, error) {
	return slices.Clone(d.Plans), nil
}

// SubscriptionAccounts implements appfinance.SupporterDirectory.
func (d *FakeSupporterDirectory) SubscriptionAccounts(_ context.Context, ids []string) (map[string]appfinance.Subscriber, error) {
	out := map[string]appfinance.Subscriber{}
	for _, id := range ids {
		if acct, ok := d.Accounts[id]; ok {
			out[id] = acct
		}
	}
	return out, nil
}

// TipAccounts implements appfinance.SupporterDirectory.
func (d *FakeSupporterDirectory) TipAccounts(_ context.Context, ids []string) (map[string]appfinance.Subscriber, error) {
	out := map[string]appfinance.Subscriber{}
	for _, id := range ids {
		if acct, ok := d.Tippers[id]; ok {
			out[id] = acct
		}
	}
	return out, nil
}

// TipsEnabled implements appfinance.SupporterDirectory.
func (d *FakeSupporterDirectory) TipsEnabled(context.Context) (bool, error) { return d.Tips, nil }

// ProviderRules implements appfinance.SupporterDirectory.
func (d *FakeSupporterDirectory) ProviderRules() []appfinance.ProviderRule {
	return slices.Clone(d.Rules)
}

func incomeAt(t *testing.T, day string) time.Time {
	t.Helper()
	at, err := time.Parse(time.DateOnly, day)
	if err != nil {
		t.Fatal(err)
	}
	return at.Add(3 * time.Hour)
}

// incomeMovement is a JPY movement of Stripe booked at 03:00 UTC of day.
func incomeMovement(t *testing.T, id string, kind finance.IncomeKind, day, gross, fee, net string) appfinance.IncomeMovement {
	t.Helper()
	return appfinance.IncomeMovement{
		Provider: "stripe", ProviderID: id, Kind: string(kind), OccurredAt: incomeAt(t, day), OccurredOn: day,
		Gross: gross, Fee: fee, Net: net, Currency: "JPY",
	}
}

// incomeScenario is the example of ADR-0067 §15 (a yearly payment of 12,000 yen on 2026-04-10
// refunded in full on 2026-06-01), a monthly payment with a partial refund, and a fee of no
// subscription.
func incomeScenario(t *testing.T) []appfinance.IncomeMovement {
	t.Helper()
	yearly := incomeMovement(t, "txn_yearly", finance.IncomePayment, "2026-04-10", "12000", "432", "11568")
	yearly.SubscriptionID, yearly.CoversStart, yearly.CoversEnd = testkit.NamedID("sub_yearly"), "2026-04-10", "2027-04-09"
	refund := incomeMovement(t, "txn_refund", finance.IncomeRefund, "2026-06-01", "-12000", "0", "-12000")
	refund.SubscriptionID, refund.ParentID = testkit.NamedID("sub_yearly"), "txn_yearly"
	monthly := incomeMovement(t, "txn_monthly", finance.IncomePayment, "2026-05-03", "500", "18", "482")
	monthly.SubscriptionID, monthly.CoversStart, monthly.CoversEnd = testkit.NamedID("sub_monthly"), "2026-05-03", "2026-06-02"
	partial := incomeMovement(t, "txn_partial", finance.IncomeRefund, "2026-05-20", "-200", "0", "-200")
	partial.SubscriptionID, partial.ParentID = testkit.NamedID("sub_monthly"), "txn_monthly"
	fee := incomeMovement(t, "txn_fee", finance.IncomeFee, "2026-05-31", "0", "84", "-84")
	return []appfinance.IncomeMovement{yearly, refund, monthly, partial, fee}
}

func (e *financeEnv) incomeNet(t *testing.T, from, to string, basis finance.Basis) map[string]int64 {
	t.Helper()
	got, err := e.svc.IncomeTotals(t.Context(), appfinance.IncomeQuery{TotalsQuery: appfinance.TotalsQuery{
		Granularity: finance.GranularityMonth, From: financeDate(t, from), To: financeDate(t, to), Basis: basis,
	}})
	if err != nil {
		t.Fatalf("IncomeTotals(%s..%s %s) = %v", from, to, basis, err)
	}
	out := map[string]int64{}
	for _, p := range got {
		for _, m := range p.Net {
			if m.Currency == "JPY" {
				out[p.Period.Label] = m.Minor
			}
		}
	}
	return out
}

func financeIncomeRecords(t *testing.T, e *financeEnv) {
	ctx := t.Context()
	ms := incomeScenario(t)
	// A refund before its payment waits for it and records nothing.
	if err := e.svc.RecordIncome(ctx, ms[1:2]); !errors.Is(err, appfinance.ErrIncomeParentMissing) {
		t.Fatalf("RecordIncome(refund alone) = %v, want ErrIncomeParentMissing", err)
	}
	invalid := incomeMovement(t, "txn_invalid", finance.IncomePayment, "2026-05-04", "500", "18", "500")
	for range 2 {
		if err := e.svc.RecordIncome(ctx, append(slices.Clone(ms), invalid)); err != nil {
			t.Fatalf("RecordIncome = %v", err)
		}
	}
	known, err := e.svc.KnownIncome(ctx, "stripe", []string{"txn_yearly", "txn_refund", "txn_fee", "txn_invalid", "txn_other"})
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]bool{"txn_yearly": true, "txn_refund": true, "txn_fee": true}; !mapsEqual(known, want) {
		t.Errorf("KnownIncome = %v, want %v", known, want)
	}
	all, err := e.svc.ListIncome(ctx, appfinance.IncomeFilter{From: finance.MinDate, To: finance.MaxDate}, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(ms) {
		t.Fatalf("ListIncome = %d movements, want %d, each once and without the invalid one", len(all), len(ms))
	}

	amortized := e.incomeNet(t, "2026-01-01", "2027-12-31", finance.BasisAmortized)
	// April and May carry floor(11568·52/365) = 1648 of the yearly payment; May also the monthly
	// payment, its partial refund, and the fee, all on their dates.
	if n := amortized["2026-04"] + amortized["2026-05"]; n != 1648+482-200-84 {
		t.Errorf("April and May = %d, want %d", n, 1648+482-200-84)
	}
	var sum int64
	for _, n := range amortized {
		sum += n
	}
	if want := int64(-432 + 482 - 200 - 84); sum != want {
		t.Errorf("sum of the months = %d, want %d (the fee that the refund keeps, and the rest)", sum, want)
	}
	// Read in a window that starts after the payment and the refund, the later months stay.
	window := e.incomeNet(t, "2026-08-01", "2026-12-31", finance.BasisAmortized)
	for label, n := range window {
		if amortized[label] != n {
			t.Errorf("%s read from August = %d, want %d", label, n, amortized[label])
		}
	}
	if len(window) != 5 {
		t.Errorf("months from August = %v, want five", window)
	}
	cash := e.incomeNet(t, "2026-01-01", "2027-12-31", finance.BasisCash)
	if want := map[string]int64{"2026-04": 11568, "2026-05": 482 - 200 - 84, "2026-06": -12000}; !mapsEqual(cash, want) {
		t.Errorf("cash = %v, want %v", cash, want)
	}

	// The balance subtracts the costs of the same periods under the same basis.
	e.create(t, "Server", finance.Recurrence{Schedule: finance.ScheduleMonths, Interval: 1}, "2026-05-01", financeJPY(1000))
	got, err := e.svc.IncomeTotals(ctx, appfinance.IncomeQuery{Balance: true, TotalsQuery: appfinance.TotalsQuery{
		Granularity: finance.GranularityMonth, From: financeDate(t, "2026-04-01"), To: financeDate(t, "2026-06-30"), Basis: finance.BasisCash,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[1].Balance[0] != financeJPY(482-200-84-1000) || got[0].Balance[0] != financeJPY(11568) {
		t.Errorf("balance = %+v, want the net less 1000 a month from May", got)
	}
	for _, p := range got {
		for i, g := range p.Gross {
			if g.Minor-p.Fee[i].Minor != p.Net[i].Minor {
				t.Errorf("%s: gross %v - fee %v != net %v", p.Period.Label, g, p.Fee[i], p.Net[i])
			}
		}
	}
	// The public totals stop at today.
	future, err := e.svc.IncomeTotals(ctx, appfinance.IncomeQuery{TotalsQuery: appfinance.TotalsQuery{
		Granularity: finance.GranularityYear, From: financeDate(t, "2027-01-01"), To: financeDate(t, "2027-12-31"),
		Basis: finance.BasisAmortized, UntilToday: true,
	}})
	if err != nil || len(future) != 0 {
		t.Errorf("IncomeTotals(2027, until today) = %v, %v, want no period", future, err)
	}
}

func financeIncomeList(t *testing.T, e *financeEnv) {
	ctx := t.Context()
	if err := e.svc.RecordIncome(ctx, incomeScenario(t)); err != nil {
		t.Fatal(err)
	}
	e.dir.Accounts = map[string]appfinance.Subscriber{testkit.NamedID("sub_yearly"): {AccountID: testkit.NamedID("acct_1"), Username: "alice"}}
	var ids []string
	before := ""
	for {
		page, err := e.svc.ListIncome(ctx, appfinance.IncomeFilter{From: finance.MinDate, To: finance.MaxDate}, before, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range page {
			ids = append(ids, m.ProviderID)
			switch m.ProviderID {
			case "txn_refund":
				if m.ParentProviderID != "txn_yearly" || m.Subscriber != (appfinance.Subscriber{AccountID: testkit.NamedID("acct_1"), Username: "alice"}) || m.Net != financeJPY(-12000) || m.Covers != nil {
					t.Errorf("refund = %+v, want its payment, account, and net", m)
				}
			case "txn_yearly":
				if m.Covers == nil || m.Covers.String() != "2026-04-10/2027-04-09" || m.Gross != financeJPY(12000) || m.Fee != financeJPY(432) {
					t.Errorf("payment = %+v, want its period, gross, and fee", m)
				}
			case "txn_fee":
				if m.SubscriptionID != "" || m.Subscriber != (appfinance.Subscriber{}) {
					t.Errorf("fee = %+v, want no subscription", m)
				}
			case "txn_monthly":
				// The subscriber's account of sub_monthly is gone: the row keeps its subscription.
				if m.SubscriptionID != testkit.NamedID("sub_monthly") || m.Subscriber != (appfinance.Subscriber{}) || m.Net != financeJPY(482) {
					t.Errorf("payment of a gone account = %+v, want its subscription and no subscriber", m)
				}
			}
		}
		if len(page) < 2 {
			break
		}
		before = page[len(page)-1].ID
	}
	if want := []string{"txn_refund", "txn_fee", "txn_partial", "txn_monthly", "txn_yearly"}; !slices.Equal(ids, want) {
		t.Errorf("pages = %v, want %v (newest first)", ids, want)
	}
	refunds, err := e.svc.ListIncome(ctx, appfinance.IncomeFilter{From: financeDate(t, "2026-05-01"), To: financeDate(t, "2026-05-31"), Kind: finance.IncomeRefund}, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(refunds) != 1 || refunds[0].ProviderID != "txn_partial" {
		t.Errorf("refunds of May = %+v, want txn_partial", refunds)
	}
	// The kind and the dates hold across the pages of the keyset.
	payments := appfinance.IncomeFilter{From: financeDate(t, "2026-04-01"), To: financeDate(t, "2026-05-31"), Kind: finance.IncomePayment}
	ids, before = nil, ""
	for {
		page, err := e.svc.ListIncome(ctx, payments, before, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		ids = append(ids, page[0].ProviderID)
		before = page[0].ID
	}
	if want := []string{"txn_monthly", "txn_yearly"}; !slices.Equal(ids, want) {
		t.Errorf("payments of April and May by pages of one = %v, want %v", ids, want)
	}
	may, err := e.svc.ListIncome(ctx, appfinance.IncomeFilter{From: financeDate(t, "2026-05-04"), To: financeDate(t, "2026-05-31")}, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(may) != 2 || may[0].ProviderID != "txn_fee" || may[1].ProviderID != "txn_partial" {
		t.Errorf("movements of 2026-05-04..31 = %+v, want txn_fee and txn_partial", may)
	}
}

// financeIncomeSources records a tip, its partial refund, which has no source of its own, a
// subscription's payment, and a fee: the refund takes the source of its payment, the tip's rows
// show the account of its user, and the totals break the net down by source.
func financeIncomeSources(t *testing.T, e *financeEnv) {
	ctx := t.Context()
	tip := incomeMovement(t, "txn_tip", finance.IncomePayment, "2026-05-03", "1000", "36", "964")
	tip.Source, tip.TipID = string(finance.SourceOneTime), testkit.NamedID("tip_1")
	refund := incomeMovement(t, "txn_tip_refund", finance.IncomeRefund, "2026-05-10", "-300", "0", "-300")
	refund.ParentID = "txn_tip"
	sub := incomeMovement(t, "txn_sub", finance.IncomePayment, "2026-05-04", "500", "18", "482")
	sub.Source, sub.SubscriptionID = string(finance.SourceSubscription), testkit.NamedID("sub_1")
	sub.CoversStart, sub.CoversEnd = "2026-05-04", "2026-06-03"
	fee := incomeMovement(t, "txn_fee", finance.IncomeFee, "2026-05-31", "0", "84", "-84")
	if err := e.svc.RecordIncome(ctx, []appfinance.IncomeMovement{tip, refund, sub, fee}); err != nil {
		t.Fatal(err)
	}
	e.dir.Tippers = map[string]appfinance.Subscriber{testkit.NamedID("tip_1"): {AccountID: testkit.NamedID("acct_tip"), Username: "bob"}}
	all, err := e.svc.ListIncome(ctx, appfinance.IncomeFilter{From: finance.MinDate, To: finance.MaxDate}, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]finance.IncomeSource{}
	for _, m := range all {
		sources[m.ProviderID] = m.Source
		if m.ProviderID == "txn_tip" && (m.TipID != testkit.NamedID("tip_1") || m.Subscriber.Username != "bob") {
			t.Errorf("tip = %+v, want its tip and the account of its user", m)
		}
	}
	want := map[string]finance.IncomeSource{
		"txn_tip": finance.SourceOneTime, "txn_tip_refund": finance.SourceOneTime, "txn_sub": finance.SourceSubscription, "txn_fee": finance.SourceUnknown,
	}
	if !mapsEqual(sources, want) {
		t.Errorf("sources = %v, want %v", sources, want)
	}
	got, err := e.svc.IncomeTotals(ctx, appfinance.IncomeQuery{TotalsQuery: appfinance.TotalsQuery{
		Granularity: finance.GranularityMonth, From: financeDate(t, "2026-05-01"), To: financeDate(t, "2026-05-31"), Basis: finance.BasisCash,
	}})
	if err != nil || len(got) != 1 {
		t.Fatalf("IncomeTotals(May) = %v, %v", got, err)
	}
	bySource := map[finance.IncomeSource]int64{}
	for _, s := range got[0].NetBySource {
		for _, m := range s.Net {
			bySource[s.Source] += m.Minor
		}
	}
	if w := map[finance.IncomeSource]int64{finance.SourceOneTime: 964 - 300, finance.SourceSubscription: 482, finance.SourceUnknown: -84}; !mapsEqual(bySource, w) {
		t.Errorf("net by source = %v, want %v", bySource, w)
	}
	e.dir.Tips = true
	if on, err := e.svc.TipsEnabled(ctx); err != nil || !on {
		t.Errorf("TipsEnabled = %v, %v, want true", on, err)
	}
}

// financeIncomeSourcesUpgrade records movements as the releases before the column source did
// (all of them unknown) and runs the data upgrade step: the movements of a subscription become
// subscription, a refund without a subscription takes its payment's source, a fee stays unknown,
// a tip stays one_time, each statement rewrites at most its limit, and a second run rewrites
// nothing.
func financeIncomeSourcesUpgrade(t *testing.T, e *financeEnv) {
	ctx := t.Context()
	pay := incomeMovement(t, "txn_old", finance.IncomePayment, "2026-05-03", "500", "18", "482")
	pay.SubscriptionID = testkit.NamedID("sub_old")
	refund := incomeMovement(t, "txn_old_refund", finance.IncomeRefund, "2026-05-10", "-100", "0", "-100")
	refund.ParentID, refund.SubscriptionID = "txn_old", testkit.NamedID("sub_old")
	orphan := incomeMovement(t, "txn_old_orphan", finance.IncomeRefund, "2026-05-11", "-100", "0", "-100")
	orphan.ParentID = "txn_old"
	fee := incomeMovement(t, "txn_old_fee", finance.IncomeFee, "2026-05-31", "0", "84", "-84")
	tip := incomeMovement(t, "txn_new_tip", finance.IncomePayment, "2026-05-12", "300", "10", "290")
	tip.Source = string(finance.SourceOneTime)
	if err := e.svc.RecordIncome(ctx, []appfinance.IncomeMovement{pay, refund, orphan, fee, tip}); err != nil {
		t.Fatal(err)
	}
	var total int64
	for range 20 {
		var n int64
		if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			n, err = e.h.Finance.UpgradeIncomeSources(ctx, tx, 1)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if n > 1 {
			t.Fatalf("UpgradeIncomeSources(limit 1) rewrote %d rows", n)
		}
		if n == 0 {
			break
		}
		total += n
	}
	if total != 3 {
		t.Errorf("rows rewritten = %d, want 3 (the payment, its refund, and the refund without a subscription)", total)
	}
	if err := appfinance.UpgradeIncomeSources(ctx, e.h.UnitOfWork, e.h.Finance); err != nil {
		t.Fatalf("UpgradeIncomeSources again = %v", err)
	}
	all, err := e.svc.ListIncome(ctx, appfinance.IncomeFilter{From: finance.MinDate, To: finance.MaxDate}, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]finance.IncomeSource{}
	for _, m := range all {
		got[m.ProviderID] = m.Source
	}
	want := map[string]finance.IncomeSource{
		"txn_old": finance.SourceSubscription, "txn_old_refund": finance.SourceSubscription, "txn_old_orphan": finance.SourceSubscription,
		"txn_old_fee": finance.SourceUnknown, "txn_new_tip": finance.SourceOneTime,
	}
	if !mapsEqual(got, want) {
		t.Errorf("sources after the upgrade = %v, want %v", got, want)
	}
}

func mapsEqual[K comparable, V comparable](a, b map[K]V) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}
