// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
)

// The income records of FinanceRepositoryContract (ADR-0067): each money movement is recorded
// once however often it comes, a refund waits for its payment, a movement that breaks the rules
// is left out, the totals place the yearly payments and their refunds by ADR-0067 §15 whatever
// window they are read in, the balance subtracts the costs, and the listing pages through the
// movements newest first with the subscribers' accounts.

// FakeSupporterDirectory is an appfinance.SupporterDirectory with fixed plans, accounts of
// subscriptions, and provider rules.
type FakeSupporterDirectory struct {
	Plans    []appfinance.PlanCount
	Accounts map[string]appfinance.Subscriber
	Rules    []appfinance.ProviderRule
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
	yearly.SubscriptionID, yearly.CoversStart, yearly.CoversEnd = NamedID("sub_yearly"), "2026-04-10", "2027-04-09"
	refund := incomeMovement(t, "txn_refund", finance.IncomeRefund, "2026-06-01", "-12000", "0", "-12000")
	refund.SubscriptionID, refund.ParentID = NamedID("sub_yearly"), "txn_yearly"
	monthly := incomeMovement(t, "txn_monthly", finance.IncomePayment, "2026-05-03", "500", "18", "482")
	monthly.SubscriptionID, monthly.CoversStart, monthly.CoversEnd = NamedID("sub_monthly"), "2026-05-03", "2026-06-02"
	partial := incomeMovement(t, "txn_partial", finance.IncomeRefund, "2026-05-20", "-200", "0", "-200")
	partial.SubscriptionID, partial.ParentID = NamedID("sub_monthly"), "txn_monthly"
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
	e.dir.Accounts = map[string]appfinance.Subscriber{NamedID("sub_yearly"): {AccountID: NamedID("acct_1"), Username: "alice"}}
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
				if m.ParentProviderID != "txn_yearly" || m.Subscriber != (appfinance.Subscriber{AccountID: NamedID("acct_1"), Username: "alice"}) || m.Net != financeJPY(-12000) || m.Covers != nil {
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
				if m.SubscriptionID != NamedID("sub_monthly") || m.Subscriber != (appfinance.Subscriber{}) || m.Net != financeJPY(482) {
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
