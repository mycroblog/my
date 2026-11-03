// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	appsupporters "github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakePayments is a live provider that never delivered a webhook and whose last reconciliation
// failed after correcting a subscription; it records the requested reconciliations, and a second
// one is too soon.
type fakePayments struct{ requested []string }

func (f *fakePayments) Status(context.Context) (appsupporters.PaymentStatus, error) {
	return appsupporters.PaymentStatus{
		Provider: "stripe", Mode: appsupporters.ModeLive, Location: time.UTC,
		LastRun: &appsupporters.ReconcileRun{
			Outcome: appsupporters.ReconcileFailed, ErrorKind: appsupporters.ErrorPermission, Requests: 3,
			CheckedSubscriptions: 2, CorrectedSubscriptions: 1, OccurredAt: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC),
		},
	}, nil
}

func (f *fakePayments) RequestReconcile(_ context.Context, byUserID string) error {
	if len(f.requested) > 0 {
		return appsupporters.ErrReconcileTooSoon
	}
	f.requested = append(f.requested, byUserID)
	return nil
}

func TestPaymentStatus(t *testing.T) {
	payments := &fakePayments{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{
			userOf(aliceAccountID): {identity.PermManageSupporters},
			userOf(bobAccountID):   {identity.PermManageFinance},
		}
		o.Payments = payments
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)

	res := e.admin(t, http.MethodGet, "/admin/payment_status", "admin-read", nil)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("GET /admin/payment_status = %d, Cache-Control %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	st := decodeJSON[clientapi.PaymentStatus](t, res)
	if st.Provider != "stripe" || st.Mode != clientapi.PaymentStatusModeLive || st.TimeZone != "UTC" || st.LastWebhookAt != nil {
		t.Errorf("status = %+v", st)
	}
	if r := st.LastReconcile; r == nil || r.Outcome != clientapi.PaymentReconcileRunOutcomeFailed || r.ErrorKind != "permission" ||
		r.Requests != 3 || r.CheckedSubscriptions != 2 || r.CorrectedSubscriptions != 1 {
		t.Errorf("last reconcile = %+v", r)
	}
	if want := []clientapi.PaymentStatusWarnings{
		clientapi.PaymentStatusWarningsNoWebhook, clientapi.PaymentStatusWarningsReconcileCorrected,
	}; !slices.Equal(st.Warnings, want) {
		t.Errorf("warnings = %v, want %v", st.Warnings, want)
	}

	for _, tc := range []struct {
		method, path, token string
		status              int
	}{
		{http.MethodPost, "/admin/payment_reconciles", "admin", http.StatusNoContent},
		{http.MethodPost, "/admin/payment_reconciles", "admin", http.StatusTooManyRequests},
		{http.MethodPost, "/admin/payment_reconciles", "admin-read", http.StatusForbidden},
		{http.MethodPost, "/admin/payment_reconciles", "bob-admin", http.StatusForbidden},
		{http.MethodGet, "/admin/payment_status", "bob-admin", http.StatusForbidden},
	} {
		res := e.admin(t, tc.method, tc.path, tc.token, nil)
		if res.StatusCode != tc.status {
			t.Errorf("%s %s with %s = %d, want %d", tc.method, tc.path, tc.token, res.StatusCode, tc.status)
		}
		if got := res.Header.Get("Retry-After"); tc.status == http.StatusTooManyRequests && (got == "" || got == "0") {
			t.Errorf("%s %s = 429 with Retry-After %q", tc.method, tc.path, got)
		}
	}
	if want := userOf(aliceAccountID); len(payments.requested) != 1 || payments.requested[0] != want {
		t.Errorf("requested = %v, want %s", payments.requested, want)
	}
}

// Without a payment provider, the operations are a 404.
func TestPaymentStatusWithoutProvider(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): {identity.PermManageSupporters}}
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		path := "/admin/payment_status"
		if method == http.MethodPost {
			path = "/admin/payment_reconciles"
		}
		if res := e.admin(t, method, path, "admin", nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", method, path, res.StatusCode)
		}
	}
}
