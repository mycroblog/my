// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeModeration keeps domain actions and reports, oldest first, with the semantics of
// appmoderation.Service.
type fakeModeration struct {
	now     time.Time
	n       int
	actions []moderation.DomainActionRecord
	reports []moderation.ReportRecord
	issued  []appmoderation.DomainActionInput
	notes   []moderation.ReportNote
	// reviewed are the statuses that ReportStatuses presents, by report.
	reviewed map[string][]appcontent.ReviewedStatus
}

func (f *fakeModeration) id() string {
	f.n++
	return fmt.Sprintf("01890a5d-ac96-774b-bcce-b30209900%03d", f.n)
}

func (f *fakeModeration) view(r moderation.DomainActionRecord) appmoderation.DomainActionView {
	return appmoderation.DomainActionView{DomainActionRecord: r, State: r.State(f.now)}
}

func (f *fakeModeration) DomainActions(_ context.Context, flt appmoderation.DomainActionFilter, limit int) ([]appmoderation.DomainActionView, error) {
	var out []appmoderation.DomainActionView
	for _, r := range slices.Backward(f.actions) {
		v := f.view(r)
		if (flt.Domain == "" || r.Action.Domain == flt.Domain) && (!flt.Active || v.State == moderation.ActionActive) &&
			(flt.Before == "" || r.Action.ID < flt.Before) && len(out) < limit {
			out = append(out, v)
		}
	}
	return out, nil
}

func (f *fakeModeration) PublicDomainActions(_ context.Context, before string, limit int) ([]moderation.PublicDomainAction, error) {
	out := []moderation.PublicDomainAction{}
	for _, r := range slices.Backward(f.actions) {
		if slices.Contains(moderation.PublicDomainActionKinds(), r.Action.Kind) && r.State(f.now) == moderation.ActionActive &&
			(before == "" || r.Action.ID < before) && len(out) < limit {
			out = append(out, r.Action.Public())
		}
	}
	return out, nil
}

func (f *fakeModeration) DomainAction(_ context.Context, id string) (appmoderation.DomainActionView, error) {
	for _, r := range f.actions {
		if r.Action.ID == id {
			return f.view(r), nil
		}
	}
	return appmoderation.DomainActionView{}, appmoderation.ErrNotFound
}

func (f *fakeModeration) IssueDomainAction(_ context.Context, in appmoderation.DomainActionInput) (moderation.DomainAction, error) {
	f.issued = append(f.issued, in)
	a := moderation.DomainAction{
		ID: f.id(), Domain: in.Domain, Kind: in.Kind, PublicComment: in.PublicComment, PrivateNote: in.PrivateNote,
		Obfuscate: in.Obfuscate, IssuedByUserID: in.IssuedByUserID, IssuedAt: f.now,
	}
	if in.Duration > 0 {
		t := f.now.Add(in.Duration)
		a.ExpiresAt = &t
	}
	if err := a.Validate(); err != nil {
		return moderation.DomainAction{}, err
	}
	f.actions = append(f.actions, moderation.DomainActionRecord{Action: a})
	return a, nil
}

func (f *fakeModeration) LiftDomainAction(_ context.Context, id, by string) error {
	for i, r := range f.actions {
		if r.Action.ID == id {
			if r.Lift == nil {
				f.actions[i].Lift = &moderation.DomainLift{ActionID: id, LiftedByUserID: by, LiftedAt: f.now}
			}
			return nil
		}
	}
	return appmoderation.ErrNotFound
}

func (f *fakeModeration) Reports(_ context.Context, q appmoderation.ReportQuery, limit int) ([]moderation.ReportRecord, error) {
	if q.State != "" && q.State != moderation.ReportOpen && q.State != moderation.ReportResolved {
		return nil, appmoderation.ErrInvalidQuery
	}
	reports := slices.Clone(f.reports)
	if !q.OldestFirst {
		slices.Reverse(reports)
	}
	var out []moderation.ReportRecord
	for _, r := range reports {
		past := q.After == "" || (q.OldestFirst && r.Report.ID > q.After) || (!q.OldestFirst && r.Report.ID < q.After)
		if (q.State == "" || r.Status.State == q.State) && (q.Category == "" || r.Report.Category == q.Category) &&
			(q.AssigneeUserID == "" || r.Status.AssigneeUserID == q.AssigneeUserID) && (!q.Unassigned || r.Status.AssigneeUserID == "") &&
			past && len(out) < limit {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeModeration) report(id string) (int, error) {
	i := slices.IndexFunc(f.reports, func(r moderation.ReportRecord) bool { return r.Report.ID == id })
	if i < 0 {
		return 0, appmoderation.ErrNotFound
	}
	return i, nil
}

func (f *fakeModeration) ReportRecord(_ context.Context, id string) (moderation.ReportRecord, error) {
	i, err := f.report(id)
	if err != nil {
		return moderation.ReportRecord{}, err
	}
	return f.reports[i], nil
}

func (f *fakeModeration) AssignReport(_ context.Context, id, assignee, _ string) (moderation.ReportRecord, error) {
	i, err := f.report(id)
	if err != nil {
		return moderation.ReportRecord{}, err
	}
	f.reports[i].Status.AssigneeUserID = assignee
	return f.reports[i], nil
}

func (f *fakeModeration) ResolveReport(_ context.Context, id string, outcome moderation.ReportOutcome, _ string) (moderation.ReportRecord, error) {
	if !outcome.Valid() {
		return moderation.ReportRecord{}, moderation.ErrInvalidReport
	}
	i, err := f.report(id)
	if err != nil {
		return moderation.ReportRecord{}, err
	}
	if st := &f.reports[i].Status; st.State == moderation.ReportOpen {
		st.State, st.Outcome, st.Since = moderation.ReportResolved, outcome, f.now
	}
	return f.reports[i], nil
}

// fakeUserAccounts maps the users of newAuthEnv (userOf(account ID)) to their accounts.
type fakeUserAccounts struct{}

func (fakeUserAccounts) LocalAccountOfUser(_ context.Context, userID string) (account.LocalAccount, error) {
	id, ok := accountOfUser(userID)
	if !ok {
		return account.LocalAccount{}, appaccount.ErrNotFound
	}
	return account.LocalAccount{AccountID: id, UserID: userID}, nil
}

const (
	reporterAccountID = "01890a5d-ac96-774b-bcce-b302099a5001"
	reportOne         = "01890a5d-ac96-774b-bcce-b302099a4001"
	reportTwo         = "01890a5d-ac96-774b-bcce-b302099a4002"
	reportThree       = "01890a5d-ac96-774b-bcce-b302099a4003"
)

// newModerationEnv is newAuthEnv with the tokens of newAdminEnv and the moderation operations:
// alice holds every permission and bob none, with three open reports of alice
// by a remote server's actor, the second one about a status.
func newModerationEnv(t *testing.T) (*authEnv, *fakeModeration) {
	t.Helper()
	return newModerationEnvWith(t, nil)
}

// newModerationEnvWith is newModerationEnv whose options change also sets, when not nil.
func newModerationEnvWith(t *testing.T, change func(*clientapi.Options)) (*authEnv, *fakeModeration) {
	t.Helper()
	f := &fakeModeration{now: adminNow}
	for i, id := range []string{reportOne, reportTwo, reportThree} {
		r := moderation.Report{
			ID: id, URI: "https://remote.example/flags/" + id, ReporterAccountID: reporterAccountID, TargetAccountID: aliceAccountID,
			Category: moderation.CategoryOther, Comment: "spam", FiledAt: adminNow.Add(time.Duration(i-3) * time.Hour), StatusIDs: []string{},
		}
		if id == reportTwo {
			r.StatusIDs = []string{"01890a5d-ac96-774b-bcce-b302099a3001"}
		}
		f.reports = append(f.reports, moderation.ReportRecord{Report: r, Status: moderation.ReportStatus{ReportID: id, State: moderation.ReportOpen, Since: r.FiledAt}})
	}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.Moderation = f
		o.UserAccounts = fakeUserAccounts{}
		accounts := o.Accounts.(fakeAccounts)
		accounts[reporterAccountID] = appaccount.Actor{
			Account: account.Account{ID: reporterAccountID, Domain: "remote.example", URI: "https://remote.example/actor", CreatedAt: adminNow},
			Handle:  &account.Handle{Username: "remote.example", Domain: "remote.example"},
		}
		if change != nil {
			change(o)
		}
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	return e, f
}

func TestModerationOperationsNeedAPermission(t *testing.T) {
	e, f := newModerationEnv(t)
	for _, op := range []struct{ method, path, body, permission string }{
		{http.MethodGet, "/admin/domain_actions", "", "manage_domains"},
		{http.MethodPost, "/admin/domain_actions", `{"domain":"bad.example","kind":"suspend"}`, "manage_domains"},
		{http.MethodGet, "/admin/domain_actions/" + reportOne, "", "manage_domains"},
		{http.MethodPost, "/admin/domain_actions/" + reportOne + "/lift", "", "manage_domains"},
		{http.MethodGet, "/admin/reports", "", "manage_reports"},
		{http.MethodGet, "/admin/reports/" + reportOne, "", "manage_reports"},
		{http.MethodPost, "/admin/reports/" + reportOne + "/assign_to_self", "", "manage_reports"},
		{http.MethodPost, "/admin/reports/" + reportOne + "/unassign", "", "manage_reports"},
		{http.MethodPost, "/admin/reports/" + reportOne + "/resolve", `{"outcome":"dismissed"}`, "manage_reports"},
		{http.MethodPost, "/admin/reports/" + reportOne + "/notes", `{"body":"look"}`, "manage_reports"},
		{http.MethodPost, "/admin/reports/" + reportOne + "/reopen", "", "manage_reports"},
		{http.MethodPost, "/admin/voice_rooms/" + reportOne + "/end", "", "manage_reports"},
	} {
		res := e.send(t, op.method, op.path, "bob-admin", op.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
			p.Detail == nil || !strings.Contains(*p.Detail, op.permission) {
			t.Errorf("%s %s by bob = %d %+v, want 403 for %s", op.method, op.path, res.StatusCode, p, op.permission)
		}
	}
	if len(f.issued) != 0 || f.reports[0].Status.State != moderation.ReportOpen {
		t.Error("a refused operation changed something")
	}
	res := e.send(t, http.MethodPost, "/admin/domain_actions", "admin-read", `{"domain":"bad.example","kind":"suspend"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
		t.Errorf("create with admin:read = %d %+v, want 403 insufficient-scope", res.StatusCode, p)
	}
}

func TestCreateAndLiftDomainAction(t *testing.T) {
	e, f := newModerationEnv(t)
	res := e.sendJSON(t, http.MethodPost, "/admin/domain_actions", "admin",
		`{"domain":"Bad.Example.","kind":"suspend","public_comment":"spam","private_note":"see #12","obfuscate":true,"expires_in":3600}`)
	got := decodeJSON[clientapi.AdminDomainAction](t, res)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("create = %d (%s)", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	if got.Domain != "bad.example" || got.Kind != clientapi.AdminDomainActionKindSuspend || got.State != clientapi.AdminDomainActionStateActive ||
		!got.Obfuscate || got.PublicComment == nil || *got.PublicComment != "spam" || got.PrivateNote == nil ||
		got.ExpiresAt == nil || !got.ExpiresAt.Equal(adminNow.Add(time.Hour)) || got.LiftedAt != nil {
		t.Errorf("created = %+v", got)
	}
	if in := f.issued[0]; in.IssuedByUserID != userOf(aliceAccountID) || in.Duration != time.Hour {
		t.Errorf("issued = %+v, want by alice for an hour", in)
	}
	// A U-label domain is stored as its A-labels.
	res = e.sendJSON(t, http.MethodPost, "/admin/domain_actions", "admin", `{"domain":"例え.jp","kind":"limit"}`)
	if idn := decodeJSON[clientapi.AdminDomainAction](t, res); idn.Domain != "xn--r8jz45g.jp" || idn.ExpiresAt != nil || idn.PublicComment != nil {
		t.Errorf("IDN action = %+v", idn)
	}

	res = e.sendJSON(t, http.MethodPost, "/admin/domain_actions/"+got.Id+"/lift", "admin", "")
	lifted := decodeJSON[clientapi.AdminDomainAction](t, res)
	if res.StatusCode != http.StatusOK || lifted.State != clientapi.AdminDomainActionStateLifted || lifted.LiftedAt == nil {
		t.Errorf("lift = %d %+v", res.StatusCode, lifted)
	}
	if l := f.actions[0].Lift; l == nil || l.LiftedByUserID != userOf(aliceAccountID) {
		t.Errorf("lift = %+v, want by alice", l)
	}
	res = e.sendJSON(t, http.MethodGet, "/admin/domain_actions/"+got.Id, "admin-read", "")
	if again := decodeJSON[clientapi.AdminDomainAction](t, res); again.State != clientapi.AdminDomainActionStateLifted {
		t.Errorf("get = %+v", again)
	}
	for _, path := range []string{"/admin/domain_actions/" + reportOne, "/admin/domain_actions/" + reportOne + "/lift"} {
		method := http.MethodGet
		if strings.HasSuffix(path, "/lift") {
			method = http.MethodPost
		}
		res := e.sendJSON(t, method, path, "admin", "")
		if p := decodeProblem(t, res); res.StatusCode != http.StatusNotFound || p.Type != clientapi.ProblemTypeBlank {
			t.Errorf("%s %s = %d %+v, want 404", method, path, res.StatusCode, p)
		}
	}
}

func TestCreateDomainActionRejectsInvalidInput(t *testing.T) {
	e, f := newModerationEnv(t)
	for body, want := range map[string]string{
		`{"domain":"https://bad.example/","kind":"suspend"}`:                                                                                          "domain",
		`{"domain":"bad..example","kind":"suspend"}`:                                                                                                  "domain",
		`{"domain":"bad.example","kind":"silence","x":"invalid-on-purpose"}`:                                                                          "kind",
		`{"domain":"bad.example","kind":"suspend","expires_in":0,"x":"invalid-on-purpose"}`:                                                           "expires_in",
		`{"domain":"bad.example","kind":"suspend","expires_in":315360001,"x":"invalid-on-purpose"}`:                                                   "expires_in",
		`{"domain":"bad.example","kind":"suspend","expires_in":10000000000000,"x":"invalid-on-purpose"}`:                                              "expires_in",
		`{"domain":"bad.example","kind":"suspend","x":"invalid-on-purpose","private_note":"` + strings.Repeat("a", moderation.MaxActionText+1) + `"}`: "private_note",
		`{"domain":"bad.example","kind":"suspend","public_comment":"spam\u0000"}`:                                                                     "public_comment",
		`{"domain":"bad.example","kind":"suspend","private_note":"\u0000"}`:                                                                           "private_note",
	} {
		res := e.sendJSON(t, http.MethodPost, "/admin/domain_actions", "admin", body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, want) {
			t.Errorf("%.80s = %d %+v, want 422 naming %s", body, res.StatusCode, p, want)
		}
	}
	if len(f.actions) != 0 {
		t.Errorf("actions = %+v, want none", f.actions)
	}
}

func TestListDomainActions(t *testing.T) {
	e, f := newModerationEnv(t)
	ctx := t.Context()
	first, _ := f.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "bad.example", Kind: moderation.DomainSuspend})
	expired, _ := f.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "media.example", Kind: moderation.DomainRejectMedia, Duration: time.Hour})
	last, _ := f.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "bad.example", Kind: moderation.DomainLimit})
	f.now = f.now.Add(time.Hour)
	ids := func(p clientapi.AdminDomainActionPage) []string {
		out := []string{}
		for _, a := range p.Items {
			out = append(out, a.Id)
		}
		return out
	}
	res := e.sendJSON(t, http.MethodGet, "/admin/domain_actions?limit=2", "admin-read", "")
	page := decodeJSON[clientapi.AdminDomainActionPage](t, res)
	if res.StatusCode != http.StatusOK || !slices.Equal(ids(page), []string{last.ID, expired.ID}) || page.NextCursor == nil ||
		page.Items[1].State != clientapi.AdminDomainActionStateExpired {
		t.Fatalf("first page = %d %+v", res.StatusCode, page)
	}
	res = e.sendJSON(t, http.MethodGet, "/admin/domain_actions?limit=2&cursor="+*page.NextCursor, "admin-read", "")
	if rest := decodeJSON[clientapi.AdminDomainActionPage](t, res); !slices.Equal(ids(rest), []string{first.ID}) || rest.NextCursor != nil {
		t.Errorf("second page = %+v", rest)
	}
	res = e.sendJSON(t, http.MethodGet, "/admin/domain_actions?active=true", "admin-read", "")
	if active := decodeJSON[clientapi.AdminDomainActionPage](t, res); !slices.Equal(ids(active), []string{last.ID, first.ID}) {
		t.Errorf("active = %v", ids(active))
	}
	res = e.sendJSON(t, http.MethodGet, "/admin/domain_actions?domain=MEDIA.example", "admin-read", "")
	if media := decodeJSON[clientapi.AdminDomainActionPage](t, res); !slices.Equal(ids(media), []string{expired.ID}) {
		t.Errorf("media.example = %v", ids(media))
	}
	for _, query := range []string{"domain=a%20b", "limit=0", "cursor=%25", "active=maybe"} {
		res := e.send(t, http.MethodGet, "/admin/domain_actions?"+query, "admin-read", "")
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("?%s = %d, want 400", query, res.StatusCode)
		}
	}
}

func TestListAndGetReports(t *testing.T) {
	e, f := newModerationEnv(t)
	res := e.sendJSON(t, http.MethodGet, "/admin/reports?limit=2", "admin-read", "")
	page := decodeJSON[clientapi.AdminReportPage](t, res)
	if res.StatusCode != http.StatusOK || len(page.Items) != 2 || page.Items[0].Id != reportThree || page.Items[1].Id != reportTwo || page.NextCursor == nil {
		t.Fatalf("first page = %d %+v", res.StatusCode, page)
	}
	two := page.Items[1]
	if two.State != clientapi.AdminReportStateOpen || two.Outcome != nil || two.AssigneeAccountId != nil || two.Uri == nil ||
		two.ReporterAccount == nil || two.ReporterAccount.Acct != "remote.example@remote.example" ||
		two.TargetAccount == nil || two.TargetAccount.Username != "alice" || len(two.StatusIds) != 1 ||
		two.Category != clientapi.AdminReportCategoryOther || !two.StateChangedAt.Equal(two.FiledAt) {
		t.Errorf("report = %+v", two)
	}
	res = e.sendJSON(t, http.MethodGet, "/admin/reports?limit=2&cursor="+*page.NextCursor, "admin-read", "")
	if rest := decodeJSON[clientapi.AdminReportPage](t, res); len(rest.Items) != 1 || rest.Items[0].Id != reportOne || rest.NextCursor != nil {
		t.Errorf("second page = %+v", rest)
	}

	// A report whose reporter is gone presents it by ID alone.
	f.reports[0].Report.ReporterAccountID = "01890a5d-ac96-774b-bcce-b302099a5999"
	res = e.sendJSON(t, http.MethodGet, "/admin/reports/"+reportOne, "admin-read", "")
	if one := decodeJSON[clientapi.AdminReport](t, res); res.StatusCode != http.StatusOK || one.ReporterAccount != nil || one.TargetAccount == nil {
		t.Errorf("report without its reporter = %d %+v", res.StatusCode, one)
	}
	res = e.sendJSON(t, http.MethodGet, "/admin/reports/01890a5d-ac96-774b-bcce-b302099a4999", "admin-read", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown report = %d, want 404", res.StatusCode)
	}
	res = e.send(t, http.MethodGet, "/admin/reports?state=closed", "admin-read", "")
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("?state=closed = %d, want 400", res.StatusCode)
	}
}

// TestListReportsFiltersAndOrder checks the assignee, category, and order of the report queue.
func TestListReportsFiltersAndOrder(t *testing.T) {
	e, f := newModerationEnvWith(t, func(o *clientapi.Options) {
		o.Accounts.(fakeAccounts)[bobAccountID] = appaccount.Actor{
			Account:   account.Account{ID: bobAccountID, URI: "https://social.example/ap/actors/" + bobAccountID, CreatedAt: adminNow},
			Local:     &account.LocalAccount{AccountID: bobAccountID, UserID: userOf(bobAccountID), Username: "bob"},
			Lifecycle: account.InitialLifecycleState(bobAccountID),
		}
	})
	f.reports[0].Status.AssigneeUserID = userOf(aliceAccountID)
	f.reports[1].Status.AssigneeUserID = userOf(bobAccountID)
	f.reports[1].Report.Category = moderation.CategorySpam
	ids := func(p clientapi.AdminReportPage) []string {
		out := []string{}
		for _, r := range p.Items {
			out = append(out, r.Id)
		}
		return out
	}
	for query, want := range map[string][]string{
		"assignee=me":                 {reportOne},
		"assignee=none":               {reportThree},
		"assignee=" + bobAccountID:    {reportTwo},
		"category=spam":               {reportTwo},
		"category=other&order=oldest": {reportOne, reportThree},
		"order=oldest":                {reportOne, reportTwo, reportThree},
		"order=newest":                {reportThree, reportTwo, reportOne},
	} {
		res := e.sendJSON(t, http.MethodGet, "/admin/reports?"+query, "admin-read", "")
		if page := decodeJSON[clientapi.AdminReportPage](t, res); res.StatusCode != http.StatusOK || !slices.Equal(ids(page), want) {
			t.Errorf("?%s = %d %v, want %v", query, res.StatusCode, ids(page), want)
		}
	}

	// The cursor continues the oldest-first order.
	res := e.sendJSON(t, http.MethodGet, "/admin/reports?order=oldest&limit=2", "admin-read", "")
	page := decodeJSON[clientapi.AdminReportPage](t, res)
	if !slices.Equal(ids(page), []string{reportOne, reportTwo}) || page.NextCursor == nil {
		t.Fatalf("oldest first page = %+v", page)
	}
	res = e.sendJSON(t, http.MethodGet, "/admin/reports?order=oldest&limit=2&cursor="+*page.NextCursor, "admin-read", "")
	if rest := decodeJSON[clientapi.AdminReportPage](t, res); !slices.Equal(ids(rest), []string{reportThree}) || rest.NextCursor != nil {
		t.Errorf("oldest second page = %+v", rest)
	}

	// An assignee that is not a local account, and an unknown order or category, are refused.
	for _, query := range []string{"assignee=" + reporterAccountID, "assignee=01890a5d-ac96-774b-bcce-b302099a4999", "order=random", "category=rude"} {
		res := e.send(t, http.MethodGet, "/admin/reports?"+query, "admin-read", "")
		if p := decodeProblem(t, res); res.StatusCode != http.StatusBadRequest || p.Detail == nil || !strings.Contains(*p.Detail, strings.Split(query, "=")[0]) {
			t.Errorf("?%s = %d %+v, want 400 naming the parameter", query, res.StatusCode, p)
		}
	}
}

func TestAssignAndResolveReport(t *testing.T) {
	e, f := newModerationEnv(t)
	res := e.sendJSON(t, http.MethodPost, "/admin/reports/"+reportTwo+"/assign_to_self", "admin", "")
	got := decodeJSON[clientapi.AdminReport](t, res)
	if res.StatusCode != http.StatusOK || got.AssigneeAccountId == nil || *got.AssigneeAccountId != aliceAccountID {
		t.Fatalf("assign = %d %+v", res.StatusCode, got)
	}
	res = e.sendJSON(t, http.MethodPost, "/admin/reports/"+reportTwo+"/unassign", "admin", "")
	if got := decodeJSON[clientapi.AdminReport](t, res); got.AssigneeAccountId != nil {
		t.Errorf("unassign = %+v", got)
	}
	f.now = adminNow.Add(time.Minute)
	res = e.sendJSON(t, http.MethodPost, "/admin/reports/"+reportTwo+"/resolve", "admin", `{"outcome":"actioned"}`)
	got = decodeJSON[clientapi.AdminReport](t, res)
	if res.StatusCode != http.StatusOK || got.State != clientapi.AdminReportStateResolved || got.Outcome == nil ||
		*got.Outcome != clientapi.AdminReportOutcomeActioned || !got.StateChangedAt.Equal(f.now) {
		t.Errorf("resolve = %d %+v", res.StatusCode, got)
	}
	res = e.sendJSON(t, http.MethodGet, "/admin/reports?state=open", "admin-read", "")
	if open := decodeJSON[clientapi.AdminReportPage](t, res); len(open.Items) != 2 || open.Items[0].Id != reportThree || open.Items[1].Id != reportOne {
		t.Errorf("open reports = %+v", open)
	}

	res = e.sendJSON(t, http.MethodPost, "/admin/reports/"+reportOne+"/resolve", "admin", `{"outcome":"ignored","x":"invalid-on-purpose"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, "outcome") {
		t.Errorf("resolve(ignored) = %d %+v, want 422", res.StatusCode, p)
	}
	for _, path := range []string{"/assign_to_self", "/unassign", "/resolve"} {
		res := e.sendJSON(t, http.MethodPost, "/admin/reports/01890a5d-ac96-774b-bcce-b302099a4999"+path, "admin", `{"outcome":"dismissed"}`)
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("%s of an unknown report = %d, want 404", path, res.StatusCode)
		}
	}
}
