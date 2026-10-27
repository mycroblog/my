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
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeAccountModeration keeps actions on accounts, oldest first, with the semantics of
// appmoderation.Service: a local account takes every kind, a remote one limit, suspend, and
// force_sensitive; an action on statuses names statuses of its account; a ban is not lifted.
type fakeAccountModeration struct {
	now      time.Time
	n        int
	accounts fakeAccounts
	reports  map[string]string // report ID → target account ID
	statuses map[string]string // status ID → author account ID
	actions  []moderation.ActionRecord
	issued   []appmoderation.AccountActionInput
}

func (f *fakeAccountModeration) view(r moderation.ActionRecord) appmoderation.AccountActionView {
	return appmoderation.AccountActionView{ActionRecord: r, State: r.State(f.now)}
}

func (f *fakeAccountModeration) AccountActions(_ context.Context, flt appmoderation.AccountActionFilter, limit int) ([]appmoderation.AccountActionView, error) {
	var out []appmoderation.AccountActionView
	for _, r := range slices.Backward(f.actions) {
		v := f.view(r)
		if (flt.AccountID == "" || r.Action.TargetAccountID == flt.AccountID) && (!flt.Active || v.State == moderation.ActionActive) &&
			(flt.Before == "" || r.Action.ID < flt.Before) && len(out) < limit {
			out = append(out, v)
		}
	}
	return out, nil
}

func (f *fakeAccountModeration) AccountAction(_ context.Context, id string) (appmoderation.AccountActionView, error) {
	for _, r := range f.actions {
		if r.Action.ID == id {
			return f.view(r), nil
		}
	}
	return appmoderation.AccountActionView{}, appmoderation.ErrNotFound
}

func (f *fakeAccountModeration) IssueAccountAction(_ context.Context, in appmoderation.AccountActionInput) (moderation.Action, error) {
	target, ok := f.accounts[in.TargetAccountID]
	if !ok {
		return moderation.Action{}, appmoderation.ErrAccountNotFound
	}
	kinds := []moderation.ActionKind{moderation.ActionLimit, moderation.ActionSuspend, moderation.ActionForceSensitive}
	if target.Account.IsLocal() {
		kinds = append(kinds, moderation.ActionWarn, moderation.ActionRestrictPosting, moderation.ActionRestrictInteraction,
			moderation.ActionDisableLogin, moderation.ActionBan, moderation.ActionDeleteStatuses, moderation.ActionMarkStatusesSensitive)
	}
	if !slices.Contains(kinds, in.Kind) {
		return moderation.Action{}, appmoderation.ErrUnsupportedAction
	}
	if in.ReportID != "" && f.reports[in.ReportID] != in.TargetAccountID {
		return moderation.Action{}, appmoderation.ErrInvalidReportReference
	}
	for _, id := range in.StatusIDs {
		if f.statuses[id] != in.TargetAccountID {
			return moderation.Action{}, appmoderation.ErrInvalidStatusReference
		}
	}
	f.issued = append(f.issued, in)
	f.n++
	a := moderation.Action{
		ID: fmt.Sprintf("01890a5d-ac96-774b-bcce-b30209901%03d", f.n), TargetAccountID: in.TargetAccountID, Kind: in.Kind,
		ReasonCategory: in.ReasonCategory, PublicReason: in.PublicReason, PrivateNote: in.PrivateNote, ReportID: in.ReportID,
		Issuer: moderation.IssuerModerator, IssuedByUserID: in.IssuedByUserID, NotifyTarget: in.NotifyTarget, IssuedAt: f.now,
	}
	if len(in.StatusIDs) > 0 {
		a.StatusIDs = slices.Sorted(slices.Values(in.StatusIDs))
	}
	if in.Duration > 0 {
		t := f.now.Add(in.Duration)
		a.ExpiresAt = &t
	}
	if err := a.Validate(); err != nil {
		return moderation.Action{}, err
	}
	f.actions = append(f.actions, moderation.ActionRecord{Action: a})
	f.now = f.now.Add(time.Second)
	return a, nil
}

func (f *fakeAccountModeration) LiftAccountAction(_ context.Context, id, by, reason string) error {
	l := moderation.Lift{ActionID: id, LiftedByUserID: by, Reason: reason, LiftedAt: f.now}
	if err := l.Validate(); err != nil {
		return err
	}
	for i, r := range f.actions {
		if r.Action.ID == id {
			if !r.Action.Kind.Liftable() {
				return appmoderation.ErrNotLiftable
			}
			if r.Lift == nil {
				f.actions[i].Lift = &l
			}
			return nil
		}
	}
	return appmoderation.ErrNotFound
}

// newAccountModerationEnv is newModerationEnv with the account actions: alice, a moderator,
// acts on bob, and reportOne is a report of bob.
func newAccountModerationEnv(t *testing.T) (*authEnv, *fakeAccountModeration) {
	t.Helper()
	f := &fakeAccountModeration{
		now: adminNow, reports: map[string]string{reportOne: bobAccountID, reportTwo: aliceAccountID},
		statuses: map[string]string{bobStatusOne: bobAccountID, bobStatusTwo: bobAccountID, aliceStatus: aliceAccountID},
	}
	e, _ := newModerationEnvWith(t, func(o *clientapi.Options) {
		f.accounts = o.Accounts.(fakeAccounts)
		f.accounts[bobAccountID] = appaccount.Actor{
			Account:   account.Account{ID: bobAccountID, URI: "https://social.example/ap/actors/" + bobAccountID, CreatedAt: adminNow},
			Local:     &account.LocalAccount{AccountID: bobAccountID, UserID: userOf(bobAccountID), Username: "bob"},
			Lifecycle: account.InitialLifecycleState(bobAccountID),
		}
		f.accounts[instanceActorID] = appaccount.Actor{
			Account: account.Account{ID: instanceActorID, Domain: account.LocalDomain, ActorType: account.ActorApplication, CreatedAt: adminNow},
		}
		o.AccountModeration = f
	})
	return e, f
}

const (
	instanceActorID = "01890a5d-ac96-774b-bcce-b302099a5009"
	bobStatusOne    = "01890a5d-ac96-774b-bcce-b302099a6001"
	bobStatusTwo    = "01890a5d-ac96-774b-bcce-b302099a6002"
	aliceStatus     = "01890a5d-ac96-774b-bcce-b302099a6003"
)

func TestAccountActionOperationsNeedAPermission(t *testing.T) {
	e, f := newAccountModerationEnv(t)
	for _, op := range []struct{ method, path, body string }{
		{http.MethodGet, "/admin/accounts/" + bobAccountID + "/actions", ""},
		{http.MethodPost, "/admin/accounts/" + bobAccountID + "/actions", `{"kind":"suspend"}`},
		{http.MethodGet, "/admin/account_actions/" + reportOne, ""},
		{http.MethodPost, "/admin/account_actions/" + reportOne + "/lift", ""},
	} {
		res := e.send(t, op.method, op.path, "bob-admin", op.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
			p.Detail == nil || !strings.Contains(*p.Detail, "manage_accounts") {
			t.Errorf("%s %s by bob = %d %+v, want 403 for manage_accounts", op.method, op.path, res.StatusCode, p)
		}
	}
	if len(f.issued) != 0 {
		t.Error("a refused operation issued an action")
	}
	res := e.send(t, http.MethodPost, "/admin/accounts/"+bobAccountID+"/actions", "admin-read", `{"kind":"suspend"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
		t.Errorf("create with admin:read = %d %+v, want 403 insufficient-scope", res.StatusCode, p)
	}
}

func TestCreateListAndLiftAccountActions(t *testing.T) {
	e, f := newAccountModerationEnv(t)
	path := "/admin/accounts/" + bobAccountID + "/actions"
	res := e.sendJSON(t, http.MethodPost, path, "admin",
		`{"kind":"suspend","reason_category":"spam","public_reason":"spam","private_note":"see #12","report_id":"`+reportOne+`","expires_in":3600}`)
	got := decodeJSON[clientapi.AdminAccountAction](t, res)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("create = %d (%s)", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	if got.AccountId != bobAccountID || got.Kind != clientapi.AdminAccountActionKindSuspend || got.State != clientapi.AdminAccountActionStateActive ||
		got.ReasonCategory != clientapi.AdminAccountActionCategorySpam || got.Issuer != clientapi.AdminAccountActionIssuerModerator ||
		!got.NotifyTarget || got.PublicReason == nil || *got.PublicReason != "spam" || got.PrivateNote == nil ||
		got.ReportId == nil || *got.ReportId != reportOne || got.IssuedByAccountId == nil || *got.IssuedByAccountId != aliceAccountID ||
		got.ExpiresAt == nil || !got.ExpiresAt.Equal(adminNow.Add(time.Hour)) || got.LiftedAt != nil {
		t.Errorf("created = %+v", got)
	}
	if in := f.issued[0]; in.IssuedByUserID != userOf(aliceAccountID) || in.Duration != time.Hour {
		t.Errorf("issued = %+v, want by alice for an hour", in)
	}
	// The defaults: the category other, a notice, no end.
	res = e.sendJSON(t, http.MethodPost, path, "admin", `{"kind":"warn"}`)
	if w := decodeJSON[clientapi.AdminAccountAction](t, res); w.ReasonCategory != clientapi.AdminAccountActionCategoryOther || !w.NotifyTarget ||
		w.ExpiresAt != nil || w.PublicReason != nil || w.ReportId != nil {
		t.Errorf("warn = %+v", w)
	}
	res = e.sendJSON(t, http.MethodPost, path, "admin", `{"kind":"disable_login","notify_target":false}`)
	login := decodeJSON[clientapi.AdminAccountAction](t, res)
	if login.NotifyTarget {
		t.Errorf("disable_login without a notice = %+v", login)
	}
	// A remote account takes the kinds that apply to it.
	res = e.sendJSON(t, http.MethodPost, "/admin/accounts/"+reporterAccountID+"/actions", "admin", `{"kind":"limit"}`)
	if r := decodeJSON[clientapi.AdminAccountAction](t, res); res.StatusCode != http.StatusOK || r.AccountId != reporterAccountID {
		t.Errorf("remote limit = %d %+v", res.StatusCode, r)
	}

	res = e.sendJSON(t, http.MethodPost, "/admin/account_actions/"+login.Id+"/lift", "admin", `{"reason":"appealed"}`)
	lifted := decodeJSON[clientapi.AdminAccountAction](t, res)
	if res.StatusCode != http.StatusOK || lifted.State != clientapi.AdminAccountActionStateLifted || lifted.LiftedAt == nil ||
		lifted.LiftReason == nil || *lifted.LiftReason != "appealed" {
		t.Errorf("lift = %d %+v", res.StatusCode, lifted)
	}
	res = e.sendJSON(t, http.MethodPost, "/admin/account_actions/"+got.Id+"/lift", "admin", "")
	if l := decodeJSON[clientapi.AdminAccountAction](t, res); res.StatusCode != http.StatusOK || l.LiftReason != nil || l.State != clientapi.AdminAccountActionStateLifted {
		t.Errorf("lift without a body = %d %+v", res.StatusCode, l)
	}
	res = e.sendJSON(t, http.MethodGet, "/admin/account_actions/"+login.Id, "admin-read", "")
	if again := decodeJSON[clientapi.AdminAccountAction](t, res); again.State != clientapi.AdminAccountActionStateLifted {
		t.Errorf("get = %+v", again)
	}

	ids := func(p clientapi.AdminAccountActionPage) []string {
		out := []string{}
		for _, a := range p.Items {
			out = append(out, a.Id)
		}
		return out
	}
	warnID := f.actions[1].Action.ID
	res = e.sendJSON(t, http.MethodGet, path+"?limit=2", "admin-read", "")
	page := decodeJSON[clientapi.AdminAccountActionPage](t, res)
	if res.StatusCode != http.StatusOK || !slices.Equal(ids(page), []string{login.Id, warnID}) || page.NextCursor == nil {
		t.Fatalf("first page = %d %+v", res.StatusCode, page)
	}
	res = e.sendJSON(t, http.MethodGet, path+"?limit=2&cursor="+*page.NextCursor, "admin-read", "")
	if rest := decodeJSON[clientapi.AdminAccountActionPage](t, res); !slices.Equal(ids(rest), []string{got.Id}) || rest.NextCursor != nil {
		t.Errorf("second page = %+v", rest)
	}
	res = e.sendJSON(t, http.MethodGet, path+"?active=true", "admin-read", "")
	if active := decodeJSON[clientapi.AdminAccountActionPage](t, res); !slices.Equal(ids(active), []string{warnID}) {
		t.Errorf("active = %v", ids(active))
	}
	for _, query := range []string{"limit=0", "cursor=%25", "active=maybe"} {
		res := e.send(t, http.MethodGet, path+"?"+query, "admin-read", "")
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("?%s = %d, want 400", query, res.StatusCode)
		}
	}
	for _, op := range []struct{ method, path string }{
		{http.MethodGet, "/admin/accounts/" + reportThree + "/actions"},
		{http.MethodPost, "/admin/accounts/" + reportThree + "/actions"},
		{http.MethodGet, "/admin/account_actions/" + reportOne},
		{http.MethodPost, "/admin/account_actions/" + reportOne + "/lift"},
	} {
		body := ""
		if op.method == http.MethodPost {
			body = `{"kind":"warn"}`
		}
		res := e.sendJSON(t, op.method, op.path, "admin", body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusNotFound || p.Type != clientapi.ProblemTypeBlank {
			t.Errorf("%s %s = %d %+v, want 404", op.method, op.path, res.StatusCode, p)
		}
	}
}

func TestAccountActionsOnStatusesAndBans(t *testing.T) {
	e, f := newAccountModerationEnv(t)
	path := "/admin/accounts/" + bobAccountID + "/actions"
	res := e.sendJSON(t, http.MethodPost, path, "admin", `{"kind":"delete_statuses","status_ids":["`+bobStatusTwo+`","`+bobStatusOne+`"]}`)
	del := decodeJSON[clientapi.AdminAccountAction](t, res)
	if res.StatusCode != http.StatusOK || del.Kind != clientapi.AdminAccountActionKindDeleteStatuses || del.StatusIds == nil ||
		!slices.Equal(*del.StatusIds, []string{bobStatusOne, bobStatusTwo}) {
		t.Errorf("delete_statuses = %d %+v", res.StatusCode, del)
	}
	res = e.sendJSON(t, http.MethodPost, path, "admin", `{"kind":"warn"}`)
	if w := decodeJSON[clientapi.AdminAccountAction](t, res); w.StatusIds != nil {
		t.Errorf("warn = %+v, want no status_ids", w)
	}
	res = e.sendJSON(t, http.MethodPost, path, "admin", `{"kind":"ban","public_reason":"spam"}`)
	ban := decodeJSON[clientapi.AdminAccountAction](t, res)
	if res.StatusCode != http.StatusOK || ban.Kind != clientapi.AdminAccountActionKindBan || ban.ExpiresAt != nil {
		t.Fatalf("ban = %d %+v", res.StatusCode, ban)
	}
	res = e.sendJSON(t, http.MethodPost, "/admin/account_actions/"+ban.Id+"/lift", "admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, "id") {
		t.Errorf("lift a ban = %d %+v, want 422 naming id", res.StatusCode, p)
	}
	if f.actions[2].Lift != nil {
		t.Error("the ban was lifted")
	}
}

func TestCreateAccountActionRejectsInvalidInput(t *testing.T) {
	e, f := newAccountModerationEnv(t)
	bob := "/admin/accounts/" + bobAccountID + "/actions"
	for _, tc := range []struct{ path, body, want string }{
		{bob, `{"kind":"silence","x":"invalid-on-purpose"}`, "kind"},
		{"/admin/accounts/" + reporterAccountID + "/actions", `{"kind":"ban"}`, "kind"},
		{bob, `{"kind":"ban","expires_in":3600}`, "expires_in"},
		{bob, `{"kind":"delete_statuses"}`, "status_ids"},
		{bob, `{"kind":"delete_statuses","status_ids":["` + bobStatusOne + `"],"expires_in":3600}`, "expires_in"},
		{bob, `{"kind":"delete_statuses","status_ids":["` + aliceStatus + `"]}`, "status_ids"},
		{bob, `{"kind":"mark_statuses_sensitive","status_ids":["` + reportThree + `"]}`, "status_ids"},
		{bob, `{"kind":"warn","status_ids":["` + bobStatusOne + `"]}`, "status_ids"},
		{bob, `{"kind":"delete_statuses","x":"invalid-on-purpose","status_ids":["` + bobStatusOne + `","` + bobStatusOne + `"]}`, "status_ids"},
		{"/admin/accounts/" + reporterAccountID + "/actions", `{"kind":"disable_login"}`, "kind"},
		{"/admin/accounts/" + instanceActorID + "/actions", `{"kind":"suspend"}`, "id"},
		{bob, `{"kind":"warn","reason_category":"rude","x":"invalid-on-purpose"}`, "reason_category"},
		{bob, `{"kind":"warn","report_id":"` + reportTwo + `"}`, "report_id"},
		{bob, `{"kind":"warn","report_id":"` + reportThree + `"}`, "report_id"},
		{bob, `{"kind":"warn","expires_in":0,"x":"invalid-on-purpose"}`, "expires_in"},
		{bob, `{"kind":"suspend","expires_in":315360001,"x":"invalid-on-purpose"}`, "expires_in"},
		{bob, `{"kind":"suspend","expires_in":10000000000000,"x":"invalid-on-purpose"}`, "expires_in"},
		{bob, `{"kind":"warn","x":"invalid-on-purpose","private_note":"` + strings.Repeat("a", moderation.MaxActionText+1) + `"}`, "private_note"},
		{bob, `{"kind":"warn","public_reason":"spam\u0000"}`, "public_reason"},
	} {
		res := e.sendJSON(t, http.MethodPost, tc.path, "admin", tc.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, tc.want) {
			t.Errorf("%s %.80s = %d %+v, want 422 naming %s", tc.path, tc.body, res.StatusCode, p, tc.want)
		}
	}
	if len(f.actions) != 0 {
		t.Errorf("actions = %+v, want none", f.actions)
	}
	a, _ := f.IssueAccountAction(t.Context(), appmoderation.AccountActionInput{TargetAccountID: bobAccountID, Kind: moderation.ActionWarn, ReasonCategory: moderation.CategoryOther})
	body := `{"x":"invalid-on-purpose","reason":"` + strings.Repeat("a", moderation.MaxActionText+1) + `"}`
	res := e.sendJSON(t, http.MethodPost, "/admin/account_actions/"+a.ID+"/lift", "admin", body)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, "reason") {
		t.Errorf("lift with a long reason = %d %+v, want 422 naming reason", res.StatusCode, p)
	}
	if f.actions[0].Lift != nil {
		t.Error("a refused lift lifted the action")
	}
}
