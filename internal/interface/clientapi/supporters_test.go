// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"slices"
	"sort"
	"testing"
	"time"

	appsupporters "github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/supporters"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

const (
	smallPlanID  = "01890a5d-ac96-774b-bcce-b302099a8201"
	largePlanID  = "01890a5d-ac96-774b-bcce-b302099a8202"
	ownSubID     = "01890a5d-ac96-774b-bcce-b302099a8203"
	smallPriceID = "01890a5d-ac96-774b-bcce-b302099a8204"
)

// fakeOwnSupporters is the users' subscriptions of one user, alice, and the supporter list.
type fakeOwnSupporters struct {
	disabled   bool
	ineligible appsupporters.Eligibility
	// changeFailure is what ChangePlan fails with for a plan that it would change to.
	changeFailure error
	sub           *appsupporters.SubscriptionView
	checkoutErr   error
	checkouts     []appsupporters.CheckoutInput
	returned      int
	listed        map[string]bool
	list          appsupporters.SupporterList
	filters       []appsupporters.ListingFilter
}

func (f *fakeOwnSupporters) live(userID string) (appsupporters.SubscriptionView, error) {
	if f.disabled {
		return appsupporters.SubscriptionView{}, appsupporters.ErrDisabled
	}
	if f.sub == nil || f.sub.UserID != userID {
		return appsupporters.SubscriptionView{}, appsupporters.ErrNoLiveSubscription
	}
	return *f.sub, nil
}

func (f *fakeOwnSupporters) OwnSubscription(_ context.Context, userID string) (appsupporters.SubscriptionView, error) {
	return f.live(userID)
}

func (f *fakeOwnSupporters) OwnSubscriptionsByID(_ context.Context, userID string, ids []string) ([]appsupporters.SubscriptionView, error) {
	if f.sub == nil || f.sub.UserID != userID || !slices.Contains(ids, f.sub.ID) {
		return nil, nil
	}
	return []appsupporters.SubscriptionView{*f.sub}, nil
}

func (f *fakeOwnSupporters) Eligibility(context.Context, string) (appsupporters.Eligibility, error) {
	return f.ineligible, nil
}

func (f *fakeOwnSupporters) StartCheckout(_ context.Context, in appsupporters.CheckoutInput) (appsupporters.StartedCheckout, error) {
	f.checkouts = append(f.checkouts, in)
	if f.checkoutErr != nil {
		return appsupporters.StartedCheckout{}, f.checkoutErr
	}
	return appsupporters.StartedCheckout{SubscriptionID: ownSubID, RedirectURL: "https://checkout.example/c/1"}, nil
}

func (f *fakeOwnSupporters) Returned(_ context.Context, userID string) (appsupporters.SubscriptionView, error) {
	f.returned++
	return f.live(userID)
}

func (f *fakeOwnSupporters) ChangePlan(_ context.Context, userID, planID string, interval supporters.Interval) (appsupporters.SubscriptionView, error) {
	v, err := f.live(userID)
	if err != nil {
		return v, err
	}
	if v.Current.State != supporters.StateActive {
		return v, appsupporters.ErrSubscriptionState
	}
	if planID == v.Current.PlanID && interval == v.Price.Interval {
		return v, appsupporters.ErrSameChoice
	}
	if planID != largePlanID {
		return v, appsupporters.ErrPlanUnavailable
	}
	return v, f.changeFailure
}

func (f *fakeOwnSupporters) Cancel(_ context.Context, userID string) (appsupporters.SubscriptionView, error) {
	v, err := f.live(userID)
	if err != nil {
		return v, err
	}
	f.sub.Current.State = supporters.StateCanceling
	return *f.sub, nil
}

func (f *fakeOwnSupporters) Resume(_ context.Context, userID string) (appsupporters.SubscriptionView, error) {
	v, err := f.live(userID)
	if err == nil && v.Current.State != supporters.StateCanceling {
		return v, appsupporters.ErrSubscriptionState
	}
	return v, err
}

func (f *fakeOwnSupporters) CancelScheduledChange(_ context.Context, userID string) (appsupporters.SubscriptionView, error) {
	v, err := f.live(userID)
	if err == nil && v.Current.Scheduled == nil {
		return v, appsupporters.ErrSubscriptionState
	}
	return v, err
}

func (f *fakeOwnSupporters) PortalURL(_ context.Context, userID, returnURL string) (string, error) {
	if _, err := f.live(userID); err != nil {
		return "", err
	}
	return "https://portal.example/p?return=" + returnURL, nil
}

func (f *fakeOwnSupporters) member(userID string) error {
	v, err := f.live(userID)
	if err != nil && !errors.Is(err, appsupporters.ErrNoLiveSubscription) {
		return err
	}
	if err != nil || !v.Current.State.Member() {
		return appsupporters.ErrNotMember
	}
	return nil
}

func (f *fakeOwnSupporters) Listed(_ context.Context, userID string) (bool, error) {
	if err := f.member(userID); err != nil {
		return false, err
	}
	return f.listed[userID], nil
}

func (f *fakeOwnSupporters) SetListed(_ context.Context, userID string, listed bool) (bool, error) {
	if err := f.member(userID); err != nil {
		return false, err
	}
	f.listed[userID] = listed
	return listed, nil
}

func (f *fakeOwnSupporters) SupporterList(_ context.Context, filter appsupporters.ListingFilter) (appsupporters.SupporterList, error) {
	f.filters = append(f.filters, filter)
	if f.disabled {
		return appsupporters.SupporterList{}, appsupporters.ErrDisabled
	}
	return f.list, nil
}

func plan(id, name string, rank int) appsupporters.PlanRecord {
	return appsupporters.PlanRecord{
		Plan:     supporters.Plan{ID: id, Currency: "JPY"},
		Revision: supporters.PlanRevision{PlanID: id, PlanTerms: supporters.PlanTerms{Name: name, Rank: rank}},
	}
}

func activeSubscription() *appsupporters.SubscriptionView {
	ends := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	return &appsupporters.SubscriptionView{
		SubscriptionRecord: appsupporters.SubscriptionRecord{
			Subscription: appsupporters.Subscription{ID: ownSubID, UserID: userOf(aliceAccountID), PlanID: smallPlanID, CreatedAt: ends.AddDate(0, -1, 0)},
			Current: appsupporters.TransitionRecord{Transition: supporters.Transition{
				State: supporters.StateActive, PlanID: smallPlanID, PriceID: smallPriceID, PeriodEndsAt: ends,
			}},
		},
		Plan:  plan(smallPlanID, "Small", 1),
		Price: appsupporters.Price{ID: smallPriceID, PlanID: smallPlanID, Interval: supporters.IntervalMonth, AmountMinor: 500, Currency: "JPY"},
	}
}

func newOwnSupportersEnv(t *testing.T, f *fakeOwnSupporters, visibility instance.Audience) *authEnv {
	t.Helper()
	settings := settingsInstance{&fakeSettings{stored: []instance.CurrentSetting{
		{Key: instance.SettingSupporterListVisibility, Value: string(visibility)},
	}}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.OwnSupporters, o.Instance, o.Settings = f, settings, settings.fakeSettings
		o.WebOrigin = "https://app.social.example"
	})
	e.auth.tokens["supporters"] = bearer("read:supporters write:supporters", aliceAccountID)
	e.auth.tokens["accounts"] = bearer("read:accounts write:accounts", aliceAccountID)
	return e
}

func problemType(t *testing.T, res *http.Response) string {
	t.Helper()
	return decode[clientapi.Problem](t, res).Type
}

// TestOwnSupporterSubscription checks the user's subscription (ADR-0072 §6, §8, §18): a 404
// while the program is disabled; why the user may not subscribe; one subscription at a time;
// the checkout and the portal return to the web client; and the problems of the changes.
func TestOwnSupporterSubscription(t *testing.T) {
	f := &fakeOwnSupporters{disabled: true, listed: map[string]bool{}}
	e := newOwnSupportersEnv(t, f, instance.AudienceOff)
	if res := e.send(t, http.MethodGet, "/supporter/subscription", "supporters", ""); res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET while disabled = %d, want 404", res.StatusCode)
	}
	f.disabled = false

	// An unverified address: no subscription, the reason, and the checkout is a 422.
	f.ineligible = appsupporters.IneligibleEmail
	res := e.send(t, http.MethodGet, "/supporter/subscription", "supporters", "")
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("GET = %d, Cache-Control %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	own := decode[clientapi.OwnSupporterSubscription](t, res)
	if own.Subscription != nil || own.IneligibleReason == nil || *own.IneligibleReason != clientapi.SupporterIneligibleReasonEmailUnverified {
		t.Errorf("GET with an unverified address = %+v", own)
	}
	f.checkoutErr = &appsupporters.IneligibleError{Reason: appsupporters.IneligibleEmail}
	body := `{"plan_id":"` + smallPlanID + `","interval":"month"}`
	res = e.sendJSON(t, http.MethodPost, "/supporter/checkout", "supporters", body)
	if res.StatusCode != http.StatusUnprocessableEntity || problemType(t, res) != clientapi.ProblemTypeSupporterIneligible {
		t.Errorf("checkout with an unverified address = %d", res.StatusCode)
	}

	// An eligible user starts a checkout that returns to the web client.
	f.ineligible, f.checkoutErr = "", nil
	res = e.sendJSON(t, http.MethodPost, "/supporter/checkout", "supporters", body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("checkout = %d", res.StatusCode)
	}
	if co := decode[clientapi.SupporterCheckout](t, res); co.RedirectUrl != "https://checkout.example/c/1" || co.SubscriptionId != ownSubID {
		t.Errorf("checkout = %+v", co)
	}
	in := f.checkouts[len(f.checkouts)-1]
	if in.UserID != userOf(aliceAccountID) || in.PlanID != smallPlanID || in.Interval != supporters.IntervalMonth ||
		in.SuccessURL != "https://app.social.example/settings/supporter?checkout=success" ||
		in.CancelURL != "https://app.social.example/settings/supporter?checkout=canceled" {
		t.Errorf("the checkout's input = %+v", in)
	}

	// A user with a subscription cannot start another one.
	f.sub, f.checkoutErr = activeSubscription(), appsupporters.ErrLiveSubscription
	res = e.sendJSON(t, http.MethodPost, "/supporter/checkout", "supporters", `{"plan_id":"`+largePlanID+`","interval":"year"}`)
	if res.StatusCode != http.StatusConflict || problemType(t, res) != clientapi.ProblemTypeSupporterSubscribed {
		t.Errorf("a second checkout = %d", res.StatusCode)
	}
	res = e.send(t, http.MethodPost, "/supporter/subscription/sync", "supporters", "")
	own = decode[clientapi.OwnSupporterSubscription](t, res)
	if f.returned != 1 || own.Subscription == nil || own.Subscription.State != clientapi.SupporterSubscriptionStateActive ||
		own.Subscription.Price != (clientapi.Money{Amount: "500", Currency: "JPY"}) || own.Subscription.PeriodEndsAt == nil || own.IneligibleReason != nil {
		t.Errorf("sync = %+v (returned %d)", own, f.returned)
	}

	for _, tc := range []struct {
		body   string
		status int
		typ    string
	}{
		{`{"plan_id":"` + smallPlanID + `","interval":"month"}`, http.StatusUnprocessableEntity, clientapi.ProblemTypeSupporterSamePlan},
		{`{"plan_id":"01890a5d-ac96-774b-bcce-b302099a8299","interval":"month"}`, http.StatusUnprocessableEntity, clientapi.ProblemTypeSupporterPlanUnavailable},
		{`{"plan_id":"` + largePlanID + `","interval":"year"}`, http.StatusOK, ""},
	} {
		res := e.sendJSON(t, http.MethodPost, "/supporter/subscription/change", "supporters", tc.body)
		if res.StatusCode != tc.status || (tc.typ != "" && problemType(t, res) != tc.typ) {
			t.Errorf("change %s = %d, want %d %s", tc.body, res.StatusCode, tc.status, tc.typ)
		}
	}
	for failure, want := range map[error]struct {
		status int
		typ    string
	}{
		appsupporters.ErrPaymentDeclined:   {http.StatusPaymentRequired, clientapi.ProblemTypeSupporterPaymentFailed},
		appsupporters.ErrPaymentProcessing: {http.StatusConflict, clientapi.ProblemTypeSupporterPaymentProcessing},
	} {
		f.changeFailure = failure
		res := e.sendJSON(t, http.MethodPost, "/supporter/subscription/change", "supporters", `{"plan_id":"`+largePlanID+`","interval":"month"}`)
		if res.StatusCode != want.status || problemType(t, res) != want.typ {
			t.Errorf("change with %v = %d, want %d %s", failure, res.StatusCode, want.status, want.typ)
		}
		f.changeFailure = nil
	}
	if res := e.send(t, http.MethodPost, "/supporter/subscription/resume", "supporters", ""); res.StatusCode != http.StatusConflict ||
		problemType(t, res) != clientapi.ProblemTypeSupporterSubscriptionState {
		t.Errorf("resume an active subscription = %d, want 409", res.StatusCode)
	}
	if res := e.send(t, http.MethodPost, "/supporter/subscription/cancel_scheduled_change", "supporters", ""); res.StatusCode != http.StatusConflict {
		t.Errorf("withdraw no scheduled change = %d, want 409", res.StatusCode)
	}
	res = e.send(t, http.MethodPost, "/supporter/subscription/cancel", "supporters", "")
	if sub := decode[clientapi.SupporterSubscription](t, res); sub.State != clientapi.SupporterSubscriptionStateCanceling {
		t.Errorf("cancel = %+v", sub)
	}
	res = e.send(t, http.MethodPost, "/supporter/portal", "supporters", "")
	if p := decode[clientapi.SupporterPortal](t, res); p.Url != "https://portal.example/p?return=https://app.social.example/settings/supporter?portal=returned" {
		t.Errorf("portal = %+v", p)
	}

	// Without a subscription, its operations are 409s, not 500s.
	f.sub = nil
	if res := e.send(t, http.MethodPost, "/supporter/subscription/cancel", "supporters", ""); res.StatusCode != http.StatusConflict {
		t.Errorf("cancel without a subscription = %d, want 409", res.StatusCode)
	}
}

// TestSupporterListingChoice checks the member's choice to be listed (ADR-0070 §13): only a
// member of a plan has one.
func TestSupporterListingChoice(t *testing.T) {
	f := &fakeOwnSupporters{listed: map[string]bool{}}
	e := newOwnSupportersEnv(t, f, instance.AudienceEveryone)
	if res := e.send(t, http.MethodGet, "/preferences/supporter_listing", "accounts", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET without a subscription = %d, want 404", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPut, "/preferences/supporter_listing", "accounts", `{"listed":true}`); res.StatusCode != http.StatusNotFound {
		t.Errorf("PUT without a subscription = %d, want 404", res.StatusCode)
	}
	f.sub = activeSubscription()
	if got := decode[clientapi.SupporterListing](t, e.send(t, http.MethodGet, "/preferences/supporter_listing", "accounts", "")); got.Listed {
		t.Error("a member is listed before choosing to be")
	}
	if got := decode[clientapi.SupporterListing](t, e.sendJSON(t, http.MethodPut, "/preferences/supporter_listing", "accounts", `{"listed":true}`)); !got.Listed ||
		!f.listed[userOf(aliceAccountID)] {
		t.Errorf("PUT = %+v, stored %v", got, f.listed)
	}
}

// TestInstanceSupporters checks the supporter list (ADR-0070 §13): supporter_list_visibility
// decides who reads it (off is a 404, signed_in a 401 for an anonymous request); it lists the
// accounts the viewer may see by plan, with the plans' names only, never an amount, an
// interval, a date, or a count; and its pages are never for search engines.
func TestInstanceSupporters(t *testing.T) {
	listedAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	last := appsupporters.Listing{UserID: bobAccountID, PlanID: largePlanID, Rank: 2, ListedAt: listedAt}
	f := &fakeOwnSupporters{listed: map[string]bool{}, list: appsupporters.SupporterList{
		Plans: []appsupporters.PlanRecord{plan(smallPlanID, "Small", 1), plan(largePlanID, "Large", 2)},
		Members: []appsupporters.ListedSupporter{
			{Listing: appsupporters.Listing{UserID: aliceAccountID, PlanID: smallPlanID, Rank: 1, ListedAt: listedAt}, AccountID: aliceAccountID},
			// An account that is gone is left out.
			{Listing: last, AccountID: bobAccountID},
		},
		Last: &last,
	}}

	off := newOwnSupportersEnv(t, f, instance.AudienceOff)
	if v := decode[clientapi.Instance](t, off.send(t, http.MethodGet, "/instance", "", "")).SupporterListVisibility; v != clientapi.InstanceAudienceOff {
		t.Errorf("Instance.supporter_list_visibility = %q, want off", v)
	}
	for _, token := range []string{"", "read"} {
		if res := off.send(t, http.MethodGet, "/instance/supporters", token, ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET with off (token %q) = %d, want 404", token, res.StatusCode)
		}
	}
	if len(f.filters) != 0 {
		t.Errorf("off read the list: %v", f.filters)
	}

	signedIn := newOwnSupportersEnv(t, f, instance.AudienceSignedIn)
	res := signedIn.send(t, http.MethodGet, "/instance/supporters", "", "")
	if res.StatusCode != http.StatusUnauthorized || problemType(t, res) != clientapi.ProblemTypeUnauthenticated {
		t.Errorf("GET with signed_in, anonymous = %d, want 401", res.StatusCode)
	}
	res = signedIn.send(t, http.MethodGet, "/instance/supporters", "read", "")
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "private, no-store" {
		t.Errorf("GET with signed_in, signed in = %d, Cache-Control %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}

	public := newOwnSupportersEnv(t, f, instance.AudienceEveryone)
	res = public.send(t, http.MethodGet, "/instance/supporters?plan_id="+smallPlanID+"&limit=2", "", "")
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "public, max-age=60" ||
		res.Header.Get("X-Robots-Tag") != "noindex, noarchive" {
		t.Fatalf("GET with public = %d, Cache-Control %q, X-Robots-Tag %q", res.StatusCode, res.Header.Get("Cache-Control"), res.Header.Get("X-Robots-Tag"))
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var page clientapi.SupporterListPage
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Account.Username != "alice" || page.Items[0].PlanId != smallPlanID || page.NextCursor == nil {
		t.Fatalf("page = %+v", page)
	}
	if !reflect.DeepEqual(page.Plans, []clientapi.SupporterListPlan{{Id: smallPlanID, Name: "Small"}, {Id: largePlanID, Name: "Large"}}) {
		t.Errorf("plans = %+v", page.Plans)
	}
	// Nothing but the plan and the account: no amount, interval, date, or count.
	var shape struct {
		Items []map[string]json.RawMessage `json:"items"`
		Plans []map[string]json.RawMessage `json:"plans"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	for what, got := range map[string][]string{"page": keys(top), "item": keys(shape.Items[0]), "plan": keys(shape.Plans[0])} {
		allowed := map[string][]string{"page": {"items", "next_cursor", "plans"}, "item": {"account", "plan_id"}, "plan": {"id", "name"}}[what]
		if !slices.Equal(got, allowed) {
			t.Errorf("the %s has %v, want only %v", what, got, allowed)
		}
	}
	if filter := f.filters[len(f.filters)-1]; filter.PlanID != smallPlanID || filter.Limit != 2 || filter.After != nil {
		t.Errorf("the filter = %+v", filter)
	}

	// The next page starts after the last member read.
	public.send(t, http.MethodGet, "/instance/supporters?cursor="+*page.NextCursor, "", "")
	if after := f.filters[len(f.filters)-1].After; after == nil || after.UserID != last.UserID || after.Rank != last.Rank || !after.ListedAt.Equal(last.ListedAt) {
		t.Errorf("the next page starts after %+v, want %+v", after, last)
	}
	if res := public.send(t, http.MethodGet, "/instance/supporters?cursor=bm90LWpzb24", "", ""); res.StatusCode != http.StatusBadRequest {
		t.Errorf("a broken cursor = %d, want 400", res.StatusCode)
	}

	// The program disabled: nothing to list.
	f.disabled = true
	if res := public.send(t, http.MethodGet, "/instance/supporters", "", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET while disabled = %d, want 404", res.StatusCode)
	}
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
