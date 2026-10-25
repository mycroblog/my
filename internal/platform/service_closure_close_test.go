// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"crypto/ecdh"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/supporters"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/moderation"
	domain "github.com/mycroblog/my/internal/domain/supporters"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestCloseService closes the server through the admin API against a real database (FR-809,
// ADR-0088 §1, §2): the checks count what the closure ends; the closure is refused without an
// announcement, before the closing date, and without the domain; it cancels the live supporter
// subscription at once, at the provider too; and from then on the client API refuses posting,
// following, and changing the settings with 403 service-closed, while the users still read their
// own account and request and download their exports.
func TestCloseService(t *testing.T) {
	t.Parallel()
	e, prov := newSupportersEnv(t)
	ctx := t.Context()
	admin := e.user("alice")
	e.want(e.do(http.MethodPost, "/admin/roles", admin.token, map[string]any{"key": "supporter", "position": 10, "permissions": []string{}}, nil),
		http.StatusOK, "POST /admin/roles")
	var plan clientapi.AdminSupporterPlan
	e.want(e.do(http.MethodPost, "/admin/supporter_plans", admin.token, map[string]any{
		"role": "supporter", "name": "Supporter", "monthly_price": map[string]string{"amount": "500", "currency": "JPY"},
		"annual_discount_percent": 0, "emoji_limit": 0, "rank": 1, "visibility": "public",
	}, &plan), http.StatusOK, "POST /admin/supporter_plans")
	e.want(e.do(http.MethodPut, "/admin/supporter_program", admin.token, map[string]any{"enabled": true}, nil), http.StatusOK, "enable")
	carol := e.user("carol")
	subID := subscribeOnFake(t, e.app, prov, carol.id, plan.Id, "sub_c")
	if _, err := e.app.Moderation.PlaceRetentionHold(ctx, carol.id, "court order", ""); err != nil {
		t.Fatal(err)
	}

	var check clientapi.AdminServiceClosureCheck
	e.want(e.do(http.MethodGet, "/admin/closure/check", admin.token, nil, &check), http.StatusOK, "GET /admin/closure/check")
	if check.LocalAccounts != 2 || check.EstimatedDeletes != check.LocalAccounts*check.DeliveryInboxes ||
		check.LiveSupporterSubscriptions != 1 || check.PendingMigrations != 0 || check.RetentionHolds != 1 {
		t.Errorf("check = %+v", check)
	}

	var inst clientapi.Instance
	e.want(e.do(http.MethodGet, "/instance", "", nil, &inst), http.StatusOK, "GET /instance")
	confirm := map[string]any{"confirm_domain": inst.Domain}
	if typ := e.want(e.do(http.MethodPost, "/admin/closure/close", admin.token, confirm, nil), http.StatusConflict, "close without an announcement"); typ != clientapi.ProblemTypeServiceClosureState {
		t.Errorf("close without an announcement: %s", typ)
	}
	e.want(e.do(http.MethodPut, "/admin/closure/announcement", admin.token, map[string]any{"closes_at": time.Now().Add(time.Hour)}, nil), http.StatusOK, "announce")
	if typ := e.want(e.do(http.MethodPost, "/admin/closure/close", admin.token, confirm, nil), http.StatusConflict, "close before the date"); typ != clientapi.ProblemTypeServiceClosureState {
		t.Errorf("close before the date: %s", typ)
	}
	e.want(e.do(http.MethodPut, "/admin/closure/announcement", admin.token, map[string]any{"closes_at": time.Now().Add(-time.Minute)}, nil), http.StatusOK, "announce")
	e.want(e.do(http.MethodPost, "/admin/closure/close", admin.token, map[string]any{"confirm_domain": "other.example"}, nil), http.StatusBadRequest, "close with another domain")

	// Before the closure, carol posts as usual.
	e.want(e.do(http.MethodPost, "/statuses", carol.token, map[string]any{"text": "bye", "visibility": "public"}, nil), http.StatusOK, "POST /statuses before the closure")

	var closed clientapi.AdminServiceClosure
	e.want(e.do(http.MethodPost, "/admin/closure/close", admin.token, confirm, &closed), http.StatusOK, "close")
	if closed.State != clientapi.ServiceClosureStateClosed {
		t.Errorf("close = %+v", closed)
	}
	v, err := e.app.Supporters.Subscription(ctx, subID)
	if err != nil || v.Current.State != domain.StateExpired || v.Current.Reason != domain.ReasonServiceClosed {
		t.Errorf("the subscription after the closure: %+v, %v", v.Current, err)
	}
	if s, _ := prov.Subscription("sub_c"); s.Status != supporters.StatusCanceled {
		t.Errorf("the provider's subscription after the closure: %+v", s)
	}

	for _, op := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/statuses", map[string]any{"text": "still here?", "visibility": "public"}},
		{http.MethodPost, "/accounts/" + admin.id + "/follow", nil},
		{http.MethodPatch, "/preferences", map[string]any{"locale": "ja"}},
		{http.MethodGet, "/timelines/home", nil},
		{http.MethodGet, "/streaming", nil},
	} {
		if typ := e.want(e.do(op.method, op.path, carol.token, op.body, nil), http.StatusForbidden, op.method+" "+op.path+" while closed"); typ != clientapi.ProblemTypeServiceClosed {
			t.Errorf("%s %s while closed: %s, want service-closed", op.method, op.path, typ)
		}
	}
	e.want(e.do(http.MethodGet, "/accounts/verify_credentials", carol.token, nil, nil), http.StatusOK, "verify_credentials while closed")
	e.want(e.do(http.MethodGet, "/csv_exports/following", carol.token, nil, nil), http.StatusOK, "GET /csv_exports/following while closed")
	var export clientapi.DataExport
	e.want(e.do(http.MethodPost, "/exports", carol.token, nil, &export), http.StatusAccepted, "POST /exports while closed")
	res := e.do(http.MethodGet, "/exports/"+export.Id+"/archive", carol.token, nil, nil)
	if body, _ := io.ReadAll(res.Body); res.StatusCode != http.StatusOK || len(body) == 0 {
		t.Errorf("download of the export while closed = %d (%d bytes)", res.StatusCode, len(body))
	}
	e.want(e.do(http.MethodGet, "/instance", "", nil, &inst), http.StatusOK, "GET /instance while closed")
	if inst.Closure == nil || inst.Closure.State != clientapi.ServiceClosureStateClosed {
		t.Errorf("Instance.closure while closed = %+v", inst.Closure)
	}

	entries, err := e.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetInstance}, 10)
	if err != nil || len(entries) == 0 || entries[0].Action != moderation.AuditServiceClosureClosed || entries[0].ActorKind != moderation.AuditActorUser {
		t.Errorf("audit log of the closure = %+v, %v", entries, err)
	}
}

// TestWebPushStopsWhenClosed checks that a closed server sends no Web Push (ADR-0088 §2).
func TestWebPushStopsWhenClosed(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline",
		"MY_VAPID_PRIVATE_KEY": "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw",
	})
	service := &pushService{status: http.StatusCreated}
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), HTTPClient: service})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h, spec, ctx := app.Handler(), apitest.Load(t), t.Context()
	alice, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "bob", Password: "correct horse battery staple", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	ua, err := ecdh.P256().GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/push/subscription", strings.NewReader(`{"endpoint":"https://push.example.net/send/1","keys":{"p256dh":"`+
		base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes())+`","auth":"`+base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef"))+`"},"alerts":["mention"]}`))
	req.Header.Set("Authorization", "Bearer "+issueAccessToken(t, app, alice.User.ID, "push"))
	req.Header.Set("Content-Type", "application/json")
	if res := spec.Serve(t, h, req); res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /push/subscription = %d", res.StatusCode)
	}
	mention := func() {
		t.Helper()
		if _, err := app.Content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: bob.AccountID, TextInput: appcontent.TextInput{Text: "@alice hello"}, Visibility: domcontent.VisibilityPublic,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mention()
	if n := len(service.sent()); n != 1 {
		t.Fatalf("%d messages before the closure, want 1", n)
	}
	if _, err := app.Instance.AnnounceClosure(ctx, appinstance.AnnounceClosureInput{ClosesAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Instance.CloseService(ctx, ""); err != nil {
		t.Fatal(err)
	}
	mention()
	if n := len(service.sent()); n != 1 {
		t.Errorf("%d messages after the closure, want still 1", n)
	}
}
