// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeClosures keeps the service closure with the semantics of appinstance.Service: the domain
// validates the schedule and checks the transitions.
type fakeClosures struct {
	now     time.Time
	current instance.Closure
	calls   []string
}

func (f *fakeClosures) Closure(context.Context) (instance.Closure, error) { return f.current, nil }

func (f *fakeClosures) AnnounceClosure(_ context.Context, in appinstance.AnnounceClosureInput) (instance.Closure, error) {
	schedule, err := instance.NewClosureSchedule(in.ClosesAt, in.ExportUntil, in.Message)
	if err != nil {
		return instance.Closure{}, err
	}
	if err := f.current.Check(instance.ClosureEventAnnounced, f.now); err != nil {
		return instance.Closure{}, err
	}
	f.calls = append(f.calls, "announce by "+in.ByUserID)
	f.current = instance.Closure{State: instance.ClosureAnnounced, Schedule: schedule, AnnouncedAt: f.now, ChangedAt: f.now}
	return f.current, nil
}

func (f *fakeClosures) WithdrawClosure(_ context.Context, by string) (instance.Closure, error) {
	if err := f.current.Check(instance.ClosureEventWithdrawn, f.now); err != nil {
		return instance.Closure{}, err
	}
	f.calls = append(f.calls, "withdraw by "+by)
	f.current = instance.Closure{}
	return f.current, nil
}

func (f *fakeClosures) CheckClosure(context.Context) (appinstance.ClosureCheck, error) {
	return appinstance.ClosureCheck{LocalAccounts: 3, DeliveryInboxes: 4, EstimatedDeletes: 12, LiveSupporterSubscriptions: 1, PendingMigrations: 2, RetentionHolds: 1}, nil
}

func (f *fakeClosures) CloseService(_ context.Context, by string) (instance.Closure, error) {
	if err := f.current.Check(instance.ClosureEventClosed, f.now); err != nil {
		return instance.Closure{}, err
	}
	f.calls = append(f.calls, "close by "+by)
	f.current.State, f.current.ChangedAt = instance.ClosureClosed, f.now
	return f.current, nil
}

func newClosureEnv(t *testing.T) (*authEnv, *fakeClosures) {
	t.Helper()
	f := &fakeClosures{now: adminNow}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.Closures = f
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	e.auth.tokens["all"] = bearer("read write follow push admin", aliceAccountID)
	return e, f
}

func TestCloseService(t *testing.T) {
	e, f := newClosureEnv(t)
	check := decodeJSON[clientapi.AdminServiceClosureCheck](t, e.admin(t, http.MethodGet, "/admin/closure/check", "admin", nil))
	if check != (clientapi.AdminServiceClosureCheck{LocalAccounts: 3, DeliveryInboxes: 4, EstimatedDeletes: 12, LiveSupporterSubscriptions: 1, PendingMigrations: 2, RetentionHolds: 1}) {
		t.Errorf("check = %+v", check)
	}
	if res := e.send(t, http.MethodGet, "/admin/closure/check", "bob-admin", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("check by bob = %d, want 403", res.StatusCode)
	}

	// No announcement, then an announcement whose closing date has not come: 409.
	confirm := `{"confirm_domain":"social.example"}`
	for _, step := range []string{"none", "announced"} {
		if step == "announced" {
			f.current = instance.Closure{State: instance.ClosureAnnounced, AnnouncedAt: adminNow.Add(-time.Hour),
				Schedule: instance.ClosureSchedule{ClosesAt: adminNow.Add(time.Minute), ExportUntil: adminNow.Add(time.Hour)}}
		}
		res := e.send(t, http.MethodPost, "/admin/closure/close", "admin", confirm)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeServiceClosureState {
			t.Errorf("close (%s) = %d %+v, want 409", step, res.StatusCode, p)
		}
	}
	f.current.Schedule.ClosesAt = adminNow.Add(-time.Minute)
	for _, body := range []string{`{"confirm_domain":"other.example"}`, `{"confirm_domain":""}`} {
		res := e.send(t, http.MethodPost, "/admin/closure/close", "admin", body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusBadRequest || p.Detail == nil || !strings.Contains(*p.Detail, "confirm_domain") {
			t.Errorf("close with %s = %d %+v, want 400", body, res.StatusCode, p)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("a refused closure closed: %v", f.calls)
	}
	got := decodeJSON[clientapi.AdminServiceClosure](t, e.send(t, http.MethodPost, "/admin/closure/close", "admin", `{"confirm_domain":"Social.Example"}`))
	if got.State != clientapi.ServiceClosureStateClosed || len(f.calls) != 1 || f.calls[0] != "close by "+userOf(aliceAccountID) {
		t.Errorf("close = %+v, calls %v", got, f.calls)
	}
}

// TestClosedServerRefusesTheClientAPI checks the gate of a closed server (ADR-0088 §2): the
// users' own data, the exports, the sessions, and GET /instance pass; every other operation, the
// streaming endpoint included, is the 403 service-closed, whoever asks.
func TestClosedServerRefusesTheClientAPI(t *testing.T) {
	e, f := newClosureEnv(t)
	f.current = instance.Closure{State: instance.ClosureClosed}
	for _, op := range []struct{ method, path, body string }{
		{http.MethodPost, "/statuses", `{"status":"hi"}`},
		{http.MethodPost, "/accounts/" + bobAccountID + "/follow", ""},
		{http.MethodPatch, "/preferences", `{}`},
		{http.MethodGet, "/timelines/home", ""},
		{http.MethodGet, "/streaming", ""},
		{http.MethodPut, "/admin/closure/announcement", `{"closes_at":"2027-01-01T00:00:00Z"}`},
		{http.MethodPost, "/accounts/delete", `{}`},
	} {
		res := e.send(t, op.method, op.path, "all", op.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeServiceClosed {
			t.Errorf("%s %s while closed = %d %+v, want 403 service-closed", op.method, op.path, res.StatusCode, p)
		}
	}
	if res := e.get(t, "/timelines/public", nil); res.StatusCode != http.StatusForbidden {
		t.Errorf("anonymous GET /timelines/public while closed = %d, want 403", res.StatusCode)
	}
	if inst := decodeJSON[clientapi.Instance](t, e.get(t, "/instance", nil)); inst.Closure == nil || inst.Closure.State != clientapi.ServiceClosureStateClosed {
		t.Errorf("GET /instance while closed = %+v", inst.Closure)
	}
	if got := decodeJSON[clientapi.AdminServiceClosure](t, e.admin(t, http.MethodGet, "/admin/closure", "admin", nil)); got.State != clientapi.ServiceClosureStateClosed {
		t.Errorf("GET /admin/closure while closed = %+v", got)
	}
	// The same operations run during an announcement.
	f.current.State = instance.ClosureAnnounced
	if res := e.send(t, http.MethodGet, "/timelines/home", "all", ""); res.StatusCode != http.StatusOK {
		t.Errorf("GET /timelines/home while announced = %d, want 200", res.StatusCode)
	}
}

func TestServiceClosureOperationsNeedAPermission(t *testing.T) {
	e, f := newClosureEnv(t)
	f.current = instance.Closure{State: instance.ClosureAnnounced, AnnouncedAt: adminNow,
		Schedule: instance.ClosureSchedule{ClosesAt: adminNow.Add(time.Hour), ExportUntil: adminNow.Add(time.Hour)}}
	for _, op := range []struct{ method, path, body string }{
		{http.MethodGet, "/admin/closure", ""},
		{http.MethodPut, "/admin/closure/announcement", `{"closes_at":"2027-01-01T00:00:00Z"}`},
		{http.MethodDelete, "/admin/closure/announcement", ""},
	} {
		res := e.send(t, op.method, op.path, "bob-admin", op.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
			p.Detail == nil || !strings.Contains(*p.Detail, "manage_service_closure") {
			t.Errorf("%s %s by bob = %d %+v, want 403 for manage_service_closure", op.method, op.path, res.StatusCode, p)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("a refused operation changed the closure: %v", f.calls)
	}
}

func TestManageServiceClosure(t *testing.T) {
	e, f := newClosureEnv(t)
	got := decodeJSON[clientapi.AdminServiceClosure](t, e.admin(t, http.MethodGet, "/admin/closure", "admin", nil))
	if got.State != clientapi.ServiceClosureStateNone || got.ClosesAt != nil || got.ShortNotice {
		t.Errorf("closure before an announcement = %+v", got)
	}
	if inst := decodeJSON[clientapi.Instance](t, e.get(t, "/instance", nil)); inst.Closure != nil {
		t.Errorf("Instance.closure without a closure = %+v", inst.Closure)
	}

	// A notice of 10 days is allowed, with a warning; export_until defaults to 30 days later.
	res := e.send(t, http.MethodPut, "/admin/closure/announcement", "admin", `{"closes_at":"2026-10-27T09:00:00Z","message":"Thank you."}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("announce = %d %+v", res.StatusCode, decodeProblem(t, res))
	}
	got = decodeJSON[clientapi.AdminServiceClosure](t, res)
	closes := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	if got.State != clientapi.ServiceClosureStateAnnounced || !got.ShortNotice || got.ClosesAt == nil || !got.ClosesAt.Equal(closes) ||
		got.ExportUntil == nil || !got.ExportUntil.Equal(closes.Add(30*24*time.Hour)) || got.Message == nil || *got.Message != "Thank you." ||
		got.AnnouncedAt == nil || !got.AnnouncedAt.Equal(adminNow) {
		t.Errorf("announce = %+v", got)
	}
	inst := decodeJSON[clientapi.Instance](t, e.get(t, "/instance", nil))
	if c := inst.Closure; c == nil || c.State != clientapi.ServiceClosureStateAnnounced || !c.ClosesAt.Equal(closes) ||
		!c.ExportUntil.Equal(closes.Add(30*24*time.Hour)) || c.Message == nil || *c.Message != "Thank you." {
		t.Errorf("Instance.closure = %+v", inst.Closure)
	}

	// A change of the dates with a notice of 60 days has no warning.
	got = decodeJSON[clientapi.AdminServiceClosure](t, e.send(t, http.MethodPut, "/admin/closure/announcement", "admin",
		`{"closes_at":"2026-12-16T09:00:00Z","export_until":"2026-12-16T09:00:00Z"}`))
	if got.ShortNotice || got.ExportUntil == nil || !got.ExportUntil.Equal(time.Date(2026, 12, 16, 9, 0, 0, 0, time.UTC)) || got.Message != nil {
		t.Errorf("change of the dates = %+v", got)
	}

	for _, tc := range []struct{ body, param string }{
		{`{"closes_at":"2026-12-16T09:00:00Z","export_until":"2026-12-15T09:00:00Z"}`, "export_until"},
		{`{"closes_at":"2026-12-16T09:00:00Z","message":"` + strings.Repeat("a", instance.MaxClosureMessageLength+1) + `"}`, "message"},
	} {
		res := e.send(t, http.MethodPut, "/admin/closure/announcement", "admin", tc.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusBadRequest || p.Detail == nil || !strings.Contains(*p.Detail, tc.param) {
			t.Errorf("announce with a bad %s = %d %+v", tc.param, res.StatusCode, p)
		}
	}

	got = decodeJSON[clientapi.AdminServiceClosure](t, e.send(t, http.MethodDelete, "/admin/closure/announcement", "admin", ""))
	if got.State != clientapi.ServiceClosureStateNone {
		t.Errorf("withdraw = %+v", got)
	}
	res = e.send(t, http.MethodDelete, "/admin/closure/announcement", "admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeServiceClosureState {
		t.Errorf("withdraw without an announcement = %d %+v, want 409", res.StatusCode, p)
	}
	if want := []string{"announce by " + userOf(aliceAccountID), "announce by " + userOf(aliceAccountID), "withdraw by " + userOf(aliceAccountID)}; strings.Join(f.calls, ",") != strings.Join(want, ",") {
		t.Errorf("calls = %v, want %v", f.calls, want)
	}
}
