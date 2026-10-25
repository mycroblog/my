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

func newClosureEnv(t *testing.T) (*authEnv, *fakeClosures) {
	t.Helper()
	f := &fakeClosures{now: adminNow}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.Closures = f
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	return e, f
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
