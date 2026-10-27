// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/diagnostics"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeErrorGroups keeps the summaries newest seen first and records the changes.
type fakeErrorGroups struct {
	groups []appdiagnostics.Summary
	calls  []string
}

func (f *fakeErrorGroups) Summaries(_ context.Context, q appdiagnostics.SummaryQuery) ([]appdiagnostics.Summary, error) {
	var out []appdiagnostics.Summary
	past := q.BeforeID == ""
	for _, g := range f.groups {
		if !past {
			past = g.Group.ID == q.BeforeID
			continue
		}
		if (q.State == "" || g.Head.State == q.State) && (q.Source == "" || g.Group.Source == q.Source) && len(out) < q.Limit {
			out = append(out, g)
		}
	}
	return out, nil
}

func (f *fakeErrorGroups) find(id string) (int, error) {
	for i, g := range f.groups {
		if g.Group.ID == id {
			return i, nil
		}
	}
	return 0, appdiagnostics.ErrNotFound
}

func (f *fakeErrorGroups) Summary(_ context.Context, id string) (appdiagnostics.Summary, error) {
	i, err := f.find(id)
	if err != nil {
		return appdiagnostics.Summary{}, err
	}
	return f.groups[i], nil
}

func (f *fakeErrorGroups) SummaryTx(ctx context.Context, _ uow.Tx, id string) (appdiagnostics.Summary, error) {
	return f.Summary(ctx, id)
}

func (f *fakeErrorGroups) Sample(_ context.Context, id string) (appdiagnostics.Sample, error) {
	if id != groupA {
		return appdiagnostics.Sample{}, appdiagnostics.ErrNotFound
	}
	return appdiagnostics.Sample{
		Fingerprint: "fp-a", ErrorType: "*fmt.wrapError > *pgconn.PgError", Message: "query failed: <id>",
		Stack: "goroutine 1 [running]:\n", RequestID: "req-1", Version: "1.2.0", Role: "all", OccurredAt: adminNow,
	}, nil
}

func (f *fakeErrorGroups) HourCounts(_ context.Context, id string) ([]appdiagnostics.HourCount, error) {
	if id != groupA {
		return nil, nil
	}
	return []appdiagnostics.HourCount{{Hour: adminNow.Truncate(time.Hour), Occurrences: 3, LastSeenAt: adminNow, LastVersion: "1.2.0"}}, nil
}

func (f *fakeErrorGroups) Events(_ context.Context, id string) ([]appdiagnostics.Event, error) {
	if id != groupA {
		return nil, nil
	}
	return []appdiagnostics.Event{
		{ID: "e1", GroupID: id, Kind: diagnostics.EventResolved, Actor: appdiagnostics.Actor{Kind: diagnostics.ActorCLI}, Version: "1.1.0", OccurredAt: adminNow.Add(-time.Hour)},
		{ID: "e2", GroupID: id, Kind: diagnostics.EventRegressed, Actor: appdiagnostics.Actor{Kind: diagnostics.ActorSystem}, Version: "1.2.0", OccurredAt: adminNow},
	}, nil
}

func (f *fakeErrorGroups) change(id string, by appdiagnostics.Actor, kind string, state diagnostics.State) (appdiagnostics.Head, error) {
	i, err := f.find(id)
	if err != nil {
		return appdiagnostics.Head{}, err
	}
	f.calls = append(f.calls, kind+" by "+string(by.Kind)+" "+by.UserID)
	f.groups[i].Head.State, f.groups[i].Head.Since = state, adminNow
	return f.groups[i].Head, nil
}

func (f *fakeErrorGroups) Resolve(_ context.Context, id string, by appdiagnostics.Actor) (appdiagnostics.Head, error) {
	return f.change(id, by, "resolve", diagnostics.StateResolved)
}

func (f *fakeErrorGroups) Mute(_ context.Context, id string, by appdiagnostics.Actor) (appdiagnostics.Head, error) {
	return f.change(id, by, "mute", diagnostics.StateMuted)
}

func (f *fakeErrorGroups) Unmute(_ context.Context, id string, by appdiagnostics.Actor) (appdiagnostics.Head, error) {
	return f.change(id, by, "unmute", diagnostics.StateOpen)
}

const (
	groupA = "01890a5d-ac96-774b-bcce-b30209900001"
	groupB = "01890a5d-ac96-774b-bcce-b30209900002"
	groupC = "01890a5d-ac96-774b-bcce-b30209900003"
)

func errorSummary(id, source string, state diagnostics.State, seen time.Time) appdiagnostics.Summary {
	return appdiagnostics.Summary{
		Group: appdiagnostics.Group{
			ID: id, Fingerprint: "fp-" + id[len(id)-1:], Source: source, Kind: "panic", ErrorType: "*pgconn.PgError",
			Message: "query failed: <id>", Where: "client_api GET /api/v1/statuses/{id}", FirstVersion: "1.0.0", FirstSeenAt: seen.Add(-48 * time.Hour),
		},
		Head:       appdiagnostics.Head{GroupID: id, State: state, Since: seen.Add(-48 * time.Hour)},
		LastSeenAt: seen, LastVersion: "1.2.0", CountDay: 3, CountWeek: 7,
	}
}

func newErrorGroupsEnv(t *testing.T) (*authEnv, *fakeErrorGroups) {
	t.Helper()
	f := &fakeErrorGroups{groups: []appdiagnostics.Summary{
		errorSummary(groupA, "server", diagnostics.StateOpen, adminNow),
		errorSummary(groupB, "web", diagnostics.StateOpen, adminNow.Add(-time.Minute)),
		errorSummary(groupC, "server", diagnostics.StateMuted, adminNow.Add(-time.Hour)),
	}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{
			userOf(aliceAccountID): identity.AllPermissions(),
			userOf(bobAccountID):   {identity.PermManageReports, identity.PermManageAccounts, identity.PermViewAudit},
		}
		o.ErrorGroups = f
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	return e, f
}

func TestListErrorGroups(t *testing.T) {
	e, _ := newErrorGroupsEnv(t)
	page := decodeJSON[clientapi.ErrorGroupPage](t, e.admin(t, http.MethodGet, "/admin/error_groups?limit=2", "admin", nil))
	if len(page.Items) != 2 || page.Items[0].Id != groupA || page.Items[1].Id != groupB || page.NextCursor == nil {
		t.Fatalf("first page = %+v", page)
	}
	g := page.Items[0]
	if g.State != clientapi.ErrorGroupStateOpen || g.Source != clientapi.ErrorGroupSourceServer || g.Kind != clientapi.ErrorGroupKindPanic ||
		g.Count24h != 3 || g.Count7d != 7 || g.Type != "*pgconn.PgError" || g.Where != "client_api GET /api/v1/statuses/{id}" ||
		g.FirstVersion != "1.0.0" || g.LastVersion != "1.2.0" || !g.LastSeenAt.Equal(adminNow) || g.ResolvedVersion != nil {
		t.Errorf("group = %+v", g)
	}
	rest := decodeJSON[clientapi.ErrorGroupPage](t, e.admin(t, http.MethodGet, "/admin/error_groups?limit=2&cursor="+url.QueryEscape(*page.NextCursor), "admin", nil))
	if len(rest.Items) != 1 || rest.Items[0].Id != groupC || rest.NextCursor != nil {
		t.Errorf("second page = %+v", rest)
	}
	muted := decodeJSON[clientapi.ErrorGroupPage](t, e.admin(t, http.MethodGet, "/admin/error_groups?state=muted", "admin", nil))
	if len(muted.Items) != 1 || muted.Items[0].Id != groupC {
		t.Errorf("muted = %+v", muted)
	}
	web := decodeJSON[clientapi.ErrorGroupPage](t, e.admin(t, http.MethodGet, "/admin/error_groups?source=web", "admin", nil))
	if len(web.Items) != 1 || web.Items[0].Id != groupB {
		t.Errorf("web = %+v", web)
	}
	for _, q := range []string{"cursor=bm9wZQ", "state=fixed", "source=cli"} {
		if res := e.send(t, http.MethodGet, "/admin/error_groups?"+q, "admin", ""); res.StatusCode != http.StatusBadRequest {
			t.Errorf("?%s = %d, want 400", q, res.StatusCode)
		}
	}
	res := e.send(t, http.MethodGet, "/admin/error_groups", "bob-admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Detail == nil || !strings.Contains(*p.Detail, "manage_errors") {
		t.Errorf("list by bob = %d %+v, want 403 for manage_errors", res.StatusCode, p)
	}
}

func TestGetErrorGroup(t *testing.T) {
	e, _ := newErrorGroupsEnv(t)
	d := decodeJSON[clientapi.ErrorGroupDetail](t, e.admin(t, http.MethodGet, "/admin/error_groups/"+groupA, "admin", nil))
	if d.Id != groupA || d.Sample == nil || d.Sample.RequestId == nil || *d.Sample.RequestId != "req-1" || d.Sample.TraceId != nil ||
		d.Sample.Stack == "" || len(d.HourlyCounts) != 1 || d.HourlyCounts[0].Count != 3 ||
		len(d.Events) != 2 || d.Events[0].Kind != clientapi.ErrorGroupEventKindResolved || d.Events[0].Actor != clientapi.ErrorGroupEventActorCli ||
		d.Events[1].Actor != clientapi.ErrorGroupEventActorSystem || d.Events[1].Version == nil || *d.Events[1].Version != "1.2.0" {
		t.Errorf("detail = %+v", d)
	}
	b := decodeJSON[clientapi.ErrorGroupDetail](t, e.admin(t, http.MethodGet, "/admin/error_groups/"+groupB, "admin", nil))
	if b.Sample != nil || b.HourlyCounts == nil || len(b.HourlyCounts) != 0 || b.Events == nil {
		t.Errorf("detail without a sample = %+v", b)
	}
	if res := e.send(t, http.MethodGet, "/admin/error_groups/01890a5d-ac96-774b-bcce-b30209900999", "admin", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown group = %d, want 404", res.StatusCode)
	}
	if res := e.send(t, http.MethodGet, "/admin/error_groups/"+groupA, "bob-admin", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("get by bob = %d, want 403", res.StatusCode)
	}
}

func TestChangeErrorGroups(t *testing.T) {
	e, f := newErrorGroupsEnv(t)
	for _, step := range []struct {
		op   string
		want clientapi.ErrorGroupState
	}{{"resolve", clientapi.ErrorGroupStateResolved}, {"mute", clientapi.ErrorGroupStateMuted}, {"unmute", clientapi.ErrorGroupStateOpen}} {
		g := decodeJSON[clientapi.ErrorGroup](t, e.admin(t, http.MethodPost, "/admin/error_groups/"+groupA+"/"+step.op, "admin", nil))
		if g.Id != groupA || g.State != step.want || !g.StateSince.Equal(adminNow) {
			t.Errorf("%s = %+v, want %s", step.op, g, step.want)
		}
		if res := e.send(t, http.MethodPost, "/admin/error_groups/01890a5d-ac96-774b-bcce-b30209900999/"+step.op, "admin", ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s of an unknown group = %d, want 404", step.op, res.StatusCode)
		}
		if res := e.send(t, http.MethodPost, "/admin/error_groups/"+groupA+"/"+step.op, "bob-admin", ""); res.StatusCode != http.StatusForbidden {
			t.Errorf("%s by bob = %d, want 403", step.op, res.StatusCode)
		}
	}
	by := "user " + userOf(aliceAccountID)
	if len(f.calls) != 3 || f.calls[0] != "resolve by "+by || f.calls[2] != "unmute by "+by {
		t.Errorf("calls = %v", f.calls)
	}
}
