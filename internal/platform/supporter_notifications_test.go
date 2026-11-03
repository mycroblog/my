// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
	domain "github.com/mycroblog/my/internal/domain/supporters"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestSupporterNotifications tells a subscriber of their subscription (ADR-0072 §16), on both
// dialects: a failed payment once when it enters past_due, however often the provider's state
// is applied again, the retirement of the plan once, and the end once when the provider ends
// the subscription; each with the subscription as it is now.
func TestSupporterNotifications(t *testing.T) {
	t.Parallel()
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			t.Parallel()
			testSupporterNotifications(t, dialect)
		})
	}
}

func testSupporterNotifications(t *testing.T, dialect string) {
	e, prov := newSupportersEnvOn(t, dialect)
	ctx := t.Context()
	admin := e.user("alice")
	e.want(e.do(http.MethodPost, "/admin/roles", admin.token, map[string]any{"key": "supporter", "position": 10, "permissions": []string{}}, nil),
		http.StatusCreated, "POST /admin/roles")
	var plan clientapi.AdminSupporterPlan
	e.want(e.do(http.MethodPost, "/admin/supporter_plans", admin.token, map[string]any{
		"role": "supporter", "name": "Gold", "monthly_price": map[string]string{"amount": "500", "currency": "JPY"},
		"annual_discount_percent": 0, "emoji_limit": 0, "rank": 1, "visibility": "public",
	}, &plan), http.StatusCreated, "POST /admin/supporter_plans")
	e.want(e.do(http.MethodPut, "/admin/supporter_program", admin.token, map[string]any{"enabled": true}, nil), http.StatusOK, "enable")
	carol := e.user("carol")
	subID := subscribeOnFake(t, e.app, prov, carol.id, plan.Id, "sub_n")
	p, err := e.app.Supporters.Plan(ctx, plan.Id)
	if err != nil {
		t.Fatal(err)
	}

	supporterNotices := func() map[clientapi.NotificationType][]clientapi.Notification {
		t.Helper()
		var page clientapi.NotificationPage
		e.want(e.do(http.MethodGet, "/notifications", carol.token, nil, &page), http.StatusOK, "GET /notifications")
		out := map[clientapi.NotificationType][]clientapi.Notification{}
		for _, n := range page.Items {
			out[n.Type] = append(out[n.Type], n)
		}
		return out
	}
	if got := supporterNotices(); len(got) != 0 {
		t.Fatalf("notifications of an active subscription: %+v", got)
	}

	// A failed payment: past_due, applied three times, the last with another period end.
	failed, periodEnd := time.Now().Add(-time.Hour), time.Now().Add(30*24*time.Hour)
	snap := supporters.SubscriptionSnapshot{
		Ref: "sub_n", Customer: "cus_sub_n", LocalID: subID, Price: p.Prices[domain.IntervalMonth].Ref, Status: supporters.StatusPastDue,
		PeriodStart: time.Now(), PeriodEnd: periodEnd, LatestPaymentFailed: failed,
	}
	for i := range 3 {
		if i == 2 {
			snap.PeriodEnd = periodEnd.Add(time.Hour)
		}
		prov.PutSubscription(snap)
		if r := e.app.Supporters.Sync(ctx, subID); !r.IsDone() {
			t.Fatalf("Sync = %v", r.Err())
		}
	}
	got := supporterNotices()
	pf := got[clientapi.NotificationTypeSupporterPaymentFailed]
	if len(pf) != 1 || len(got) != 1 {
		t.Fatalf("after the failed payment: %+v", got)
	}
	if s := pf[0].SupporterSubscription; s == nil || s.Id != subID || s.State != clientapi.SupporterSubscriptionStatePastDue ||
		s.PlanName != "Gold" || s.GraceEndsAt == nil || s.EndReason != nil || pf[0].Account != nil {
		t.Errorf("supporter_payment_failed: %+v", pf[0])
	}

	// The plan is retired: the subscription is canceled at the end of its period.
	if err := e.app.Supporters.RetirePlan(ctx, plan.Id, ""); err != nil {
		t.Fatalf("RetirePlan = %v", err)
	}
	got = supporterNotices()
	if pr := got[clientapi.NotificationTypeSupporterPlanRetired]; len(pr) != 1 || pr[0].SupporterSubscription == nil ||
		pr[0].SupporterSubscription.Id != subID {
		t.Fatalf("after the retirement: %+v", got)
	}

	// The provider ends the subscription: expired, once.
	snap.Status, snap.LatestPaymentFailed = supporters.StatusCanceled, time.Time{}
	for range 2 {
		prov.PutSubscription(snap)
		if r := e.app.Supporters.Sync(ctx, subID); !r.IsDone() {
			t.Fatalf("Sync = %v", r.Err())
		}
	}
	got = supporterNotices()
	ended := got[clientapi.NotificationTypeSupporterSubscriptionEnded]
	if len(ended) != 1 || len(got[clientapi.NotificationTypeSupporterPaymentFailed]) != 1 || len(got[clientapi.NotificationTypeSupporterPlanRetired]) != 1 {
		t.Fatalf("after the end: %+v", got)
	}
	if s := ended[0].SupporterSubscription; s == nil || s.State != clientapi.SupporterSubscriptionStateExpired || s.EndReason == nil {
		t.Errorf("supporter_subscription_ended: %+v", ended[0].SupporterSubscription)
	}
	// The administrator, who does not subscribe, is told of nothing.
	var adminPage clientapi.NotificationPage
	e.want(e.do(http.MethodGet, "/notifications", admin.token, nil, &adminPage), http.StatusOK, "GET /notifications")
	for _, n := range adminPage.Items {
		if n.SupporterSubscription != nil {
			t.Errorf("the administrator's notification %+v", n)
		}
	}
}
