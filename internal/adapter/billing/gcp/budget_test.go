// SPDX-License-Identifier: AGPL-3.0-only

package gcp_test

import (
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	pubsubv2 "cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/v2/pstest"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	billinggcp "github.com/mycroblog/my/internal/adapter/billing/gcp"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/pkg/testkit"
)

const (
	budgetTopic        = "projects/my-billing/topics/billing-budget"
	budgetSubscription = "projects/my-billing/subscriptions/my-billing-budget"
	billingAccount     = "012345-6789AB-CDEF01"
)

// published is the Pacific time at which the fake publishes the notifications: on 2026-10-20, the
// last day with data of the open month of the scenario. pstest drops a message 10 minutes after
// it was published, so every message of a test is published within seconds of it.
var published = time.Date(2026, time.October, 20, 12, 0, 0, 0, pacific)

// budgetClock is the clock of pstest: the time of the next publish. It moves only when a test
// sets it, so that the ack deadlines of the pulled messages do not pass during a test.
type budgetClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *budgetClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *budgetClock) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

// failReactor answers every call of a method with err.
type failReactor struct{ err error }

func (r failReactor) React(any) (bool, any, error) { return true, nil, r.err }

// budgetFake is a pstest server with the budget's topic and subscription.
type budgetFake struct {
	t     *testing.T
	srv   *pstest.Server
	clock *budgetClock
	opts  []option.ClientOption
}

func newBudgetFake(t *testing.T, subscribe bool, reactors ...pstest.ServerReactorOption) *budgetFake {
	t.Helper()
	srv := pstest.NewServer(reactors...)
	t.Cleanup(func() { _ = srv.Close() })
	clock := &budgetClock{t: published}
	srv.SetTimeNowFunc(clock.now)
	f := &budgetFake{t: t, srv: srv, clock: clock, opts: []option.ClientOption{
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		// One connection per client instead of the default pool of up to four keeps the
		// tests' loopback ports few.
		option.WithGRPCConnectionPool(1),
	}}
	c, err := pubsubv2.NewClient(t.Context(), "my-billing", f.opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.TopicAdminClient.CreateTopic(t.Context(), &pubsubpb.Topic{Name: budgetTopic}); err != nil {
		t.Fatal(err)
	}
	if subscribe {
		if _, err := c.SubscriptionAdminClient.CreateSubscription(t.Context(), &pubsubpb.Subscription{
			Name: budgetSubscription, Topic: budgetTopic, AckDeadlineSeconds: 60,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// note is a budget notification of the fake.
type note struct {
	budget   string
	name     string
	month    string
	amount   string
	currency string
}

// publish publishes n at the time at, as the budget of Cloud Billing does, and returns its ID.
func (f *budgetFake) publish(n note, at time.Time) string {
	f.t.Helper()
	m, err := finance.ParseYearMonth(n.month)
	if err != nil {
		f.t.Fatal(err)
	}
	start := time.Date(m.Year, m.Month, 1, 0, 0, 0, 0, pacific).UTC().Format(time.RFC3339)
	data := `{"budgetDisplayName":` + quote(n.name) + `,"alertThresholdExceeded":0.5,"costAmount":` + n.amount +
		`,"costIntervalStart":"` + start + `","budgetAmount":100.0,"budgetAmountType":"SPECIFIED_AMOUNT","currencyCode":"` + n.currency + `"}`
	return f.publishRaw([]byte(data), map[string]string{
		"billingAccountId": billingAccount, "budgetId": n.budget, "schemaVersion": "1.0",
	}, at)
}

func (f *budgetFake) publishRaw(data []byte, attrs map[string]string, at time.Time) string {
	f.t.Helper()
	f.clock.set(at)
	return f.srv.Publish(budgetTopic, data, attrs)
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// acks returns the acknowledgements of each message by ID.
func (f *budgetFake) acks() map[string]int {
	out := map[string]int{}
	for _, m := range f.srv.Messages() {
		out[m.ID] = m.Acks
	}
	return out
}

func (f *budgetFake) source(t *testing.T, o billinggcp.BudgetOptions) *billinggcp.BudgetSource {
	t.Helper()
	if o.Subscription == "" {
		o.Subscription = budgetSubscription
	}
	if o.Now == nil {
		o.Now = func() time.Time { return now }
	}
	if o.PullWait == 0 {
		o.PullWait = 2 * time.Second
	}
	src, err := billinggcp.NewBudget(t.Context(), o, f.opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = src.Close() })
	return src
}

// scenarioTotal sums the rows of a month of a scenario exactly.
func scenarioTotal(t *testing.T, rows []testkit.FakeBillingRow) (string, string) {
	t.Helper()
	sum, scale, currency := new(big.Rat), 0, ""
	for _, r := range rows {
		a, err := finance.ParseDecimal(r.Amount)
		if err != nil {
			t.Fatal(err)
		}
		sum.Add(sum, a)
		if _, frac, ok := strings.Cut(r.Amount, "."); ok {
			scale = max(scale, len(frac))
		}
		currency = r.Currency
	}
	return sum.FloatString(scale), currency
}

func pstestFailure(t *testing.T, kind appfinance.ImportErrorKind) []pstest.ServerReactorOption {
	t.Helper()
	fail := func(st *status.Status) []pstest.ServerReactorOption {
		return []pstest.ServerReactorOption{{FuncName: "Pull", Reactor: failReactor{st.Err()}}}
	}
	switch kind {
	case appfinance.ImportErrorPermission:
		return fail(status.New(codes.PermissionDenied, "User not authorized to perform this action."))
	case appfinance.ImportErrorNotEnabled:
		st, err := status.New(codes.PermissionDenied, "Cloud Pub/Sub API has not been used in project 123 before or it is disabled.").
			WithDetails(&errdetails.ErrorInfo{Reason: "SERVICE_DISABLED", Domain: "googleapis.com"})
		if err != nil {
			t.Fatal(err)
		}
		return fail(st)
	case appfinance.ImportErrorThrottled:
		return fail(status.New(codes.ResourceExhausted, "quota exceeded"))
	case appfinance.ImportErrorUnavailable:
		return fail(status.New(codes.Unavailable, "the service is unavailable"))
	}
	return nil
}

func TestBudgetBillingSourceContract(t *testing.T) {
	testkit.BillingTotalsSourceContract(t, func(t *testing.T, s testkit.BillingScenario) appfinance.BillingSource {
		if s.Failure == appfinance.ImportErrorBytesCap {
			t.Skip("Pub/Sub bills no bytes scanned")
		}
		f := newBudgetFake(t, s.Failure != appfinance.ImportErrorNotFound, pstestFailure(t, s.Failure)...)
		at, n := published, 0
		next := func() time.Time { at = at.Add(time.Second); n++; return at }
		for _, m := range s.Months {
			total, currency := scenarioTotal(t, m.Rows)
			// An earlier notification of the month, which the latest replaces, and the latest
			// one twice, as Pub/Sub may deliver it.
			f.publish(note{budget: "budget-1", name: "my", month: m.Month.String(), amount: "0.01", currency: currency}, next())
			latest := note{budget: "budget-1", name: "my", month: m.Month.String(), amount: total, currency: currency}
			f.publish(latest, next())
			f.publish(latest, next())
		}
		if s.Failure == appfinance.ImportErrorInvalidResponse {
			f.publish(note{budget: "budget-1", name: "my", month: "2026-10", amount: "1e3", currency: "USD"}, next())
			n++
		}
		// The fetch takes s.Pages pulls: the messages over s.Pages-1 pulls, then an empty one.
		o := billinggcp.BudgetOptions{}
		if s.Pages > 1 {
			o.MaxMessages = int32((n + s.Pages - 2) / (s.Pages - 1)) //nolint:gosec // a few messages
		}
		return f.source(t, o)
	})
}

// The same notifications fetched again, delivered twice, or arriving out of order give the same
// one line of each month; the replaced ones are acknowledged and the latest is kept for the next
// fetch.
func TestBudgetRefetchIsIdempotent(t *testing.T) {
	f := newBudgetFake(t, true)
	early := f.publish(note{budget: "b1", name: "my", month: "2026-10", amount: "1.00", currency: "JPY"}, published.Add(time.Second))
	latest := f.publish(note{budget: "b1", name: "my", month: "2026-10", amount: "12.5", currency: "JPY"}, published.Add(3*time.Second))
	again := f.publish(note{budget: "b1", name: "my", month: "2026-10", amount: "12.5", currency: "JPY"}, published.Add(2*time.Second))
	f.clock.set(published.Add(4 * time.Second))
	src := f.source(t, billinggcp.BudgetOptions{})
	for i := range 3 {
		snap, err := src.Fetch(t.Context(), appfinance.FetchRequest{Months: []finance.YearMonth{month(t, "2026-10")}, MaxRequests: 10})
		if err != nil {
			t.Fatalf("Fetch %d = %v", i, err)
		}
		if len(snap.Lines) != 1 {
			t.Fatalf("Fetch %d = %+v, want one line", i, snap.Lines)
		}
		l := snap.Lines[0]
		if l.Amount != "12.5" || l.Currency != "JPY" || l.Service != "my" || l.ChargeType != finance.ChargeUsage ||
			l.Category != finance.CategoryOther || l.Final || l.Covered.End.String() != "2026-10-20" {
			t.Errorf("Fetch %d = %+v, want the latest 12.5 JPY through 2026-10-20", i, l)
		}
		if snap.Requests != 2 {
			t.Errorf("Fetch %d made %d pulls, want 2 (one with the messages, one empty)", i, snap.Requests)
		}
	}
	acks := f.acks()
	if acks[early] != 1 || acks[again] != 1 || acks[latest] != 0 {
		t.Errorf("acknowledgements %v: want the replaced %s and %s acknowledged once, the latest %s kept", acks, early, again, latest)
	}
}

// The notifications of a month older than the ones asked for, which the import has recorded as
// final, are acknowledged; the budgets of a month are a line each.
func TestBudgetAcknowledgesTheMonthsNoLongerAskedFor(t *testing.T) {
	f := newBudgetFake(t, true)
	sep := f.publish(note{budget: "b1", name: "my", month: "2026-09", amount: "30", currency: "USD"}, published.Add(time.Second))
	f.publish(note{budget: "b1", name: "my", month: "2026-10", amount: "10", currency: "USD"}, published.Add(2*time.Second))
	f.publish(note{budget: "b2", name: "media", month: "2026-10", amount: "2.25", currency: "USD"}, published.Add(3*time.Second))
	src := f.source(t, billinggcp.BudgetOptions{})
	snap, err := src.Fetch(t.Context(), appfinance.FetchRequest{Months: []finance.YearMonth{month(t, "2026-09"), month(t, "2026-10")}, MaxRequests: 10})
	if err != nil || len(snap.Lines) != 3 {
		t.Fatalf("Fetch of both months = %+v, %v; want three lines", snap.Lines, err)
	}
	if !snap.Lines[0].Final || snap.Lines[0].Month.String() != "2026-09" || snap.Lines[0].Covered != month(t, "2026-09").Range() {
		t.Errorf("September = %+v, want final over the month", snap.Lines[0])
	}
	if got := f.acks()[sep]; got != 0 {
		t.Errorf("September acknowledged %d times while it is asked for", got)
	}
	snap, err = src.Fetch(t.Context(), appfinance.FetchRequest{Months: []finance.YearMonth{month(t, "2026-10")}, MaxRequests: 10})
	if err != nil || len(snap.Lines) != 2 || snap.Lines[0].Service != "media" || snap.Lines[1].Service != "my" {
		t.Fatalf("Fetch of October = %+v, %v; want the lines of media and my", snap.Lines, err)
	}
	if got := f.acks()[sep]; got != 1 {
		t.Errorf("September acknowledged %d times once October alone is asked for, want once", got)
	}
}

// A message that is not a notification of a monthly budget fails the fetch as invalid_response
// and is acknowledged, so that the next fetch succeeds.
func TestBudgetMalformedNotifications(t *testing.T) {
	for name, publish := range map[string]func(f *budgetFake){
		"not JSON":  func(f *budgetFake) { f.publishRaw([]byte("{"), map[string]string{"budgetId": "b1"}, published) },
		"no budget": func(f *budgetFake) { f.publishRaw([]byte(`{}`), nil, published) },
		"exponent": func(f *budgetFake) {
			f.publish(note{budget: "b1", name: "my", month: "2026-10", amount: "1e3", currency: "USD"}, published)
		},
		"currency": func(f *budgetFake) {
			f.publish(note{budget: "b1", name: "my", month: "2026-10", amount: "1", currency: "dollar"}, published)
		},
		"not monthly": func(f *budgetFake) {
			start := time.Date(2026, time.October, 15, 0, 0, 0, 0, pacific).UTC().Format(time.RFC3339)
			f.publishRaw([]byte(`{"budgetDisplayName":"my","costAmount":1,"costIntervalStart":"`+start+`","currencyCode":"USD"}`),
				map[string]string{"budgetId": "b1"}, published)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newBudgetFake(t, true)
			good := f.publish(note{budget: "b0", name: "ok", month: "2026-10", amount: "1", currency: "USD"}, published)
			publish(f)
			src := f.source(t, billinggcp.BudgetOptions{})
			req := appfinance.FetchRequest{Months: []finance.YearMonth{month(t, "2026-10")}, MaxRequests: 10}
			snap, err := src.Fetch(t.Context(), req)
			if !errors.Is(err, appfinance.ErrInvalidBillingResponse) || len(snap.Lines) != 0 {
				t.Fatalf("Fetch = %+v, %v; want invalid_response", snap.Lines, err)
			}
			snap, err = src.Fetch(t.Context(), req)
			if err != nil || len(snap.Lines) != 1 || snap.Lines[0].Service != "ok" {
				t.Errorf("the next Fetch = %+v, %v; want the line of the valid notification", snap.Lines, err)
			}
			if got := f.acks()[good]; got != 0 {
				t.Errorf("the valid notification was acknowledged %d times", got)
			}
		})
	}
}

// Two budgets of one name make two lines of one key: the fetch fails.
func TestBudgetNamesCollide(t *testing.T) {
	f := newBudgetFake(t, true)
	f.publish(note{budget: "b1", name: "my", month: "2026-10", amount: "1", currency: "USD"}, published)
	f.publish(note{budget: "b2", name: "my", month: "2026-10", amount: "2", currency: "USD"}, published)
	_, err := f.source(t, billinggcp.BudgetOptions{}).Fetch(t.Context(),
		appfinance.FetchRequest{Months: []finance.YearMonth{month(t, "2026-10")}, MaxRequests: 10})
	if !errors.Is(err, appfinance.ErrInvalidBillingResponse) {
		t.Errorf("Fetch = %v, want invalid_response", err)
	}
}

func TestCheckSubscription(t *testing.T) {
	for v, ok := range map[string]bool{
		"projects/my-billing/subscriptions/my-billing-budget":  true,
		"projects/example.com:my-billing/subscriptions/budget": true,
		"my-billing-budget":                             false,
		"projects/my-billing/topics/budget":             false,
		"projects/My_Billing/subscriptions/budget":      false,
		"projects/my-billing/subscriptions/goog-budget": false,
		"projects/my-billing/subscriptions/1budget":     false,
		"projects/my-billing/subscriptions/ab":          false,
	} {
		if err := billinggcp.CheckSubscription(v); (err == nil) != ok {
			t.Errorf("CheckSubscription(%q) = %v, want ok %v", v, err, ok)
		}
	}
}

func TestNewBudgetRefusesAMalformedSubscription(t *testing.T) {
	if _, err := billinggcp.NewBudget(t.Context(), billinggcp.BudgetOptions{Subscription: "budget"}); err == nil {
		t.Error("NewBudget of a malformed subscription succeeded")
	}
}
