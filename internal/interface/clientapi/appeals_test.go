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

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeAppeals keeps the appeals in memory with the rules of the moderation module: one appeal
// per action, none of the ban of a deleted account (actionID(4)) or after the deadline, and a
// decision once.
type fakeAppeals struct {
	own      *fakeOwnModeration
	now      time.Time
	appeals  []moderation.AppealRecord // oldest first
	deadline map[string]time.Time      // action ID -> deadline
	// actions are the moderators' views of the actions, by ID.
	actions map[string]appmoderation.AccountActionView
}

// actionID4 is the ban of alice's account as if it were deleted.
var actionID4 = actionID(4)

func appealID(i int) string { return fmt.Sprintf("01890a5d-ac96-774b-bcce-b302099c%04d", i) }

func (f *fakeAppeals) find(id string) (int, bool) {
	i := slices.IndexFunc(f.appeals, func(r moderation.AppealRecord) bool { return r.Appeal.ID == id })
	return i, i >= 0
}

// view returns an action as its account sees it, with its appeal.
func (f *fakeAppeals) view(accountID, actionID string) (appmoderation.TargetActionView, error) {
	v, err := f.own.OwnAction(context.Background(), accountID, actionID)
	if err != nil {
		return v, err
	}
	if d, ok := f.deadline[actionID]; ok {
		v.AppealDeadline, v.Appealable = &d, !f.now.After(d)
	}
	for _, r := range f.appeals {
		if r.Appeal.ActionID == actionID {
			a := &appmoderation.OwnAppeal{ID: r.Appeal.ID, Text: r.Appeal.Text, State: r.State(), SubmittedAt: r.Appeal.SubmittedAt}
			if r.Decision != nil {
				a.DecidedAt, a.DecisionReason = &r.Decision.DecidedAt, r.Decision.PublicReason
			}
			v.Appeal, v.Appealable = a, false
		}
	}
	return v, nil
}

// OwnActions and OwnAction serve the actions of fakeOwnModeration with their appeals.
func (f *fakeAppeals) OwnActions(ctx context.Context, accountID string, flt appmoderation.OwnActionFilter, limit int) ([]appmoderation.TargetActionView, error) {
	vs, err := f.own.OwnActions(ctx, accountID, flt, limit)
	for i := range vs {
		if vs[i], err = f.view(accountID, vs[i].ID); err != nil {
			return nil, err
		}
	}
	return vs, err
}

func (f *fakeAppeals) OwnAction(_ context.Context, accountID, id string) (appmoderation.TargetActionView, error) {
	return f.view(accountID, id)
}

func (f *fakeAppeals) FileAppeal(_ context.Context, accountID, actionID, text string) (appmoderation.TargetActionView, error) {
	v, err := f.view(accountID, actionID)
	switch {
	case err != nil:
		return v, err
	case v.Kind == moderation.ActionBan && actionID == actionID4:
		return v, appmoderation.ErrNotAppealable
	case v.Appeal != nil:
		return v, appmoderation.ErrAlreadyAppealed
	case !v.Appealable:
		return v, appmoderation.ErrAppealWindowClosed
	}
	a := moderation.Appeal{ID: appealID(len(f.appeals) + 1), ActionID: actionID, AccountID: accountID, Text: text, SubmittedAt: f.now}
	if err := a.Validate(); err != nil {
		return v, err
	}
	f.appeals = append(f.appeals, moderation.AppealRecord{Appeal: a})
	return f.view(accountID, actionID)
}

func (f *fakeAppeals) OwnAppealAction(_ context.Context, accountID, id string) (appmoderation.TargetActionView, error) {
	i, ok := f.find(id)
	if !ok || f.appeals[i].Appeal.AccountID != accountID {
		return appmoderation.TargetActionView{}, appmoderation.ErrNotFound
	}
	return f.view(accountID, f.appeals[i].Appeal.ActionID)
}

func (f *fakeAppeals) Appeals(_ context.Context, flt appmoderation.AppealFilter, limit int) ([]appmoderation.AppealView, error) {
	if flt.State != "" && !flt.State.Valid() {
		return nil, appmoderation.ErrInvalidQuery
	}
	var out []appmoderation.AppealView
	for _, r := range slices.Backward(f.appeals) {
		if (flt.State == "" || r.State() == flt.State) && (flt.Before == "" || r.Appeal.ID < flt.Before) && len(out) < limit {
			out = append(out, appmoderation.AppealView{AppealRecord: r, Action: f.actions[r.Appeal.ActionID]})
		}
	}
	return out, nil
}

func (f *fakeAppeals) Appeal(_ context.Context, id string) (appmoderation.AppealView, error) {
	i, ok := f.find(id)
	if !ok {
		return appmoderation.AppealView{}, appmoderation.ErrNotFound
	}
	r := f.appeals[i]
	return appmoderation.AppealView{AppealRecord: r, Action: f.actions[r.Appeal.ActionID]}, nil
}

func (f *fakeAppeals) DecideAppeal(ctx context.Context, id string, decision moderation.AppealDecisionKind, reason, byUserID string) (appmoderation.AppealView, error) {
	d := moderation.AppealDecision{AppealID: id, Decision: decision, PublicReason: reason, DecidedByUserID: byUserID, DecidedAt: f.now}
	if err := d.Validate(); err != nil {
		return appmoderation.AppealView{}, err
	}
	i, ok := f.find(id)
	switch {
	case !ok:
		return appmoderation.AppealView{}, appmoderation.ErrNotFound
	case f.appeals[i].Decision != nil && f.appeals[i].Decision.Decision != decision:
		return appmoderation.AppealView{}, appmoderation.ErrAppealDecided
	case f.appeals[i].Decision == nil:
		f.appeals[i].Decision = &d
	}
	return f.Appeal(ctx, id)
}

func (f *fakeAppeals) CountPendingAppeals(context.Context) (int64, error) {
	var n int64
	for _, r := range f.appeals {
		if r.State() == moderation.AppealPending {
			n++
		}
	}
	return n, nil
}

// newAppealEnv serves alice's actions (aliceActions) with appeals: the warning's deadline has
// passed, the limit's has not, and the suspension's has not.
func newAppealEnv(t *testing.T, change ...func(*clientapi.Options)) (*authEnv, *fakeAppeals) {
	t.Helper()
	own := &fakeOwnModeration{on: map[string][]appmoderation.TargetActionView{aliceAccountID: append(aliceActions(),
		appmoderation.TargetActionView{ID: actionID(4), Kind: moderation.ActionBan, ReasonCategory: moderation.CategoryLegal, StatusIDs: []string{},
			IssuedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), State: moderation.ActionActive})}}
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	f := &fakeAppeals{own: own, now: now, deadline: map[string]time.Time{
		actionID(1): now.Add(-time.Hour), actionID(2): now.Add(time.Hour), actionID(3): now.Add(time.Hour),
	}, actions: map[string]appmoderation.AccountActionView{}}
	for _, a := range own.on[aliceAccountID] {
		f.actions[a.ID] = appmoderation.AccountActionView{ActionRecord: moderation.ActionRecord{Action: moderation.Action{
			ID: a.ID, TargetAccountID: aliceAccountID, Kind: a.Kind, ReasonCategory: a.ReasonCategory, Issuer: moderation.IssuerModerator,
			PrivateNote: "the moderators' note", IssuedAt: a.IssuedAt,
		}}, State: a.State}
	}
	e, _ := newModerationEnvWith(t, func(o *clientapi.Options) {
		o.OwnModeration, o.OwnAppeals, o.AppealModeration = f, f, f
		for _, c := range change {
			c(o)
		}
	})
	e.auth.tokens["write-accounts"] = bearer("write:accounts", aliceAccountID)
	return e, f
}

func TestAppealModerationAction(t *testing.T) {
	e, f := newAppealEnv(t)
	limit := "/moderation/actions/" + actionID(2) + "/appeal"

	// Before the appeal, the action says until when it can be appealed.
	res := e.send(t, http.MethodGet, "/moderation/actions", "read-accounts", "")
	page := decodeJSON[clientapi.ModerationActionPage](t, res)
	byID := map[string]clientapi.ModerationAction{}
	for _, a := range page.Items {
		byID[a.Id] = a
	}
	if a := byID[actionID(2)]; !a.Appealable || a.AppealDeadline == nil || a.Appeal != nil {
		t.Errorf("limit before the appeal = %+v, want appealable", a)
	}
	if a := byID[actionID(4)]; a.Appealable {
		t.Errorf("the ban of a deleted account = %+v, want not appealable", a)
	}

	res = e.sendJSON(t, http.MethodPost, limit, "write-accounts", `{"text":"I did not spam."}`)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("POST %s = %d (Cache-Control %q), want 200 no-store", limit, res.StatusCode, res.Header.Get("Cache-Control"))
	}
	a := decodeJSON[clientapi.ModerationAction](t, res)
	if a.Id != actionID(2) || a.Appealable || a.Appeal == nil || a.Appeal.State != clientapi.ModerationAppealStatePending ||
		a.Appeal.Text != "I did not spam." || a.Appeal.DecidedAt != nil || a.Appeal.DecisionReason != nil {
		t.Errorf("appealed action = %+v, want a pending appeal", a)
	}

	for _, c := range []struct {
		name, path, token, body string
		status                  int
		typ, field              string
	}{
		{"a second appeal", limit, "write", `{"text":"again"}`, http.StatusUnprocessableEntity, clientapi.ProblemTypeAlreadyAppealed, ""},
		{"after the deadline", "/moderation/actions/" + actionID(1) + "/appeal", "write", `{"text":"late"}`, http.StatusUnprocessableEntity, clientapi.ProblemTypeAppealWindowClosed, ""},
		{"the ban of a deleted account", "/moderation/actions/" + actionID(4) + "/appeal", "write", `{"text":"please"}`, http.StatusUnprocessableEntity, clientapi.ProblemTypeNotAppealable, ""},
		{"an empty text", "/moderation/actions/" + actionID(3) + "/appeal", "write", `{"text":" ","x":"invalid-on-purpose"}`, http.StatusUnprocessableEntity, clientapi.ProblemTypeBlank, "text"},
		{"a long text", "/moderation/actions/" + actionID(3) + "/appeal", "write", `{"x":"invalid-on-purpose","text":"` + strings.Repeat("a", moderation.MaxAppealText+1) + `"}`, http.StatusUnprocessableEntity, clientapi.ProblemTypeBlank, "text"},
		{"another account's action", "/moderation/actions/" + actionID(3) + "/appeal", "bob-write", `{"text":"mine"}`, http.StatusNotFound, clientapi.ProblemTypeBlank, ""},
		{"an unknown action", "/moderation/actions/" + actionID(8) + "/appeal", "write", `{"text":"mine"}`, http.StatusNotFound, clientapi.ProblemTypeBlank, ""},
	} {
		e.auth.tokens["bob-write"] = bearer("write", bobAccountID)
		res := e.sendJSON(t, http.MethodPost, c.path, c.token, c.body)
		p := decodeProblem(t, res)
		if res.StatusCode != c.status || p.Type != c.typ || (c.field != "" && (p.Field == nil || *p.Field != c.field)) {
			t.Errorf("%s = %d %+v, want %d %s naming %q", c.name, res.StatusCode, p, c.status, c.typ, c.field)
		}
	}
	if len(f.appeals) != 1 {
		t.Errorf("appeals = %+v, want only the first", f.appeals)
	}
	if res := e.sendJSON(t, http.MethodPost, limit, "read-accounts", `{"text":"x"}`); res.StatusCode != http.StatusForbidden {
		t.Errorf("without write:accounts = %d, want 403", res.StatusCode)
	}
	res = e.sendJSON(t, http.MethodPost, "/moderation/actions/"+actionID(3)+"/appeal", "write", `{"text":"x"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusOK {
		t.Errorf("appeal of the suspension with write = %d %+v, want 200", res.StatusCode, p)
	}
}

func TestAdminAppeals(t *testing.T) {
	e, f := newAppealEnv(t)
	for _, id := range []string{actionID(2), actionID(3)} {
		if _, err := f.FileAppeal(t.Context(), aliceAccountID, id, "I did not."); err != nil {
			t.Fatal(err)
		}
	}

	res := e.admin(t, http.MethodGet, "/admin/appeals?state=pending&limit=1", "admin-read", nil)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("GET /admin/appeals = %d, want 200 no-store", res.StatusCode)
	}
	page := decodeJSON[clientapi.AdminAppealPage](t, res)
	if len(page.Items) != 1 || page.NextCursor == nil || page.Items[0].Id != appealID(2) || page.Items[0].Action.Id != actionID(3) ||
		page.Items[0].Action.PrivateNote == nil || page.Items[0].State != clientapi.ModerationAppealStatePending {
		t.Fatalf("first page = %+v", page)
	}
	res = e.admin(t, http.MethodGet, "/admin/appeals?state=pending&limit=1&cursor="+*page.NextCursor, "admin-read", nil)
	if rest := decodeJSON[clientapi.AdminAppealPage](t, res); len(rest.Items) != 1 || rest.Items[0].Id != appealID(1) || rest.NextCursor != nil {
		t.Errorf("second page = %+v", rest)
	}

	res = e.sendJSON(t, http.MethodPost, "/admin/appeals/"+appealID(1)+"/overturn", "admin", `{"public_reason":"Sorry."}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("overturn = %d, want 200", res.StatusCode)
	}
	if a := decodeJSON[clientapi.AdminAppeal](t, res); a.State != clientapi.ModerationAppealStateOverturned || a.DecisionReason == nil ||
		*a.DecisionReason != "Sorry." || a.DecidedAt == nil || a.DecidedByAccountId == nil || *a.DecidedByAccountId != aliceAccountID {
		t.Errorf("overturned = %+v", a)
	}
	if res := e.admin(t, http.MethodPost, "/admin/appeals/"+appealID(1)+"/overturn", "admin", nil); res.StatusCode != http.StatusOK {
		t.Errorf("overturning again = %d, want 200", res.StatusCode)
	}
	res = e.admin(t, http.MethodPost, "/admin/appeals/"+appealID(1)+"/uphold", "admin", map[string]string{"Accept-Language": "ja"})
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeAppealDecided || res.Header.Get("Content-Language") != "ja" {
		t.Errorf("upholding an overturned appeal = %d %+v, want 409 appeal-decided in Japanese", res.StatusCode, p)
	}
	res = e.sendJSON(t, http.MethodPost, "/admin/appeals/"+appealID(2)+"/uphold", "admin", `{"x":"invalid-on-purpose","public_reason":"`+strings.Repeat("a", moderation.MaxAppealText+1)+`"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Field == nil || *p.Field != "public_reason" {
		t.Errorf("a long reason = %d %+v, want 422 naming public_reason", res.StatusCode, p)
	}
	res = e.admin(t, http.MethodGet, "/admin/appeals/"+appealID(2), "admin-read", nil)
	if a := decodeJSON[clientapi.AdminAppeal](t, res); res.StatusCode != http.StatusOK || a.State != clientapi.ModerationAppealStatePending || a.AccountId != aliceAccountID {
		t.Errorf("GET appeal = %d %+v", res.StatusCode, a)
	}
	for _, op := range []struct{ method, path string }{
		{http.MethodGet, "/admin/appeals/" + appealID(9)},
		{http.MethodPost, "/admin/appeals/" + appealID(9) + "/uphold"},
	} {
		if res := e.admin(t, op.method, op.path, "admin", nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", op.method, op.path, res.StatusCode)
		}
	}
	if res := e.admin(t, http.MethodGet, "/admin/appeals?state=closed", "admin", nil); res.StatusCode != http.StatusBadRequest {
		t.Errorf("an unknown state = %d, want 400", res.StatusCode)
	}

	// Each operation needs manage_accounts.
	for _, op := range []struct{ method, path string }{
		{http.MethodGet, "/admin/appeals"},
		{http.MethodGet, "/admin/appeals/" + appealID(2)},
		{http.MethodPost, "/admin/appeals/" + appealID(2) + "/uphold"},
		{http.MethodPost, "/admin/appeals/" + appealID(2) + "/overturn"},
	} {
		res := e.admin(t, op.method, op.path, "bob-admin", map[string]string{"Accept-Language": "en"})
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
			p.Detail == nil || !strings.Contains(*p.Detail, "manage_accounts") {
			t.Errorf("%s %s by bob = %d %+v, want 403 naming manage_accounts", op.method, op.path, res.StatusCode, p)
		}
	}
	if f.appeals[1].Decision != nil {
		t.Error("a refused decision decided the appeal")
	}

	// The pending appeals are counted for manage_accounts.
	res = e.admin(t, http.MethodGet, "/admin/counts", "admin-read", nil)
	if c := decodeJSON[clientapi.AdminCounts](t, res); c.PendingAppeals == nil || *c.PendingAppeals != 1 {
		t.Errorf("counts = %+v, want 1 pending appeal", c)
	}
	e.auth.tokens["bob-read"] = bearer("admin:read", bobAccountID)
	if c := decodeJSON[clientapi.AdminCounts](t, e.admin(t, http.MethodGet, "/admin/counts", "bob-read", nil)); c.PendingAppeals != nil {
		t.Errorf("counts without manage_accounts = %+v, want no pending appeals", c)
	}
}

// TestListAppealNotifications checks that an appeal_decided presents the action with the appeal
// and its decision, and an admin_appeal names its appeal.
func TestListAppealNotifications(t *testing.T) {
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	notes := &fakeNotifications{dismissed: map[string]bool{}, markers: map[string]string{}, ns: []notification.Notification{
		{ID: notificationID(1), RecipientAccountID: aliceAccountID, Type: notification.TypeAdminAppeal, AppealID: appealID(1), CreatedAt: at},
		{ID: notificationID(2), RecipientAccountID: aliceAccountID, Type: notification.TypeAppealDecided, AppealID: appealID(1), CreatedAt: at},
		{ID: notificationID(3), RecipientAccountID: aliceAccountID, Type: notification.TypeAppealDecided, AppealID: appealID(7), CreatedAt: at},
	}}
	e, f := newAppealEnv(t, func(o *clientapi.Options) { o.Notifications = notes })
	if _, err := f.FileAppeal(t.Context(), aliceAccountID, actionID(2), "I did not."); err != nil {
		t.Fatal(err)
	}
	if _, err := f.DecideAppeal(t.Context(), appealID(1), moderation.AppealDecisionUpheld, "It was spam.", userOf(bobAccountID)); err != nil {
		t.Fatal(err)
	}
	res := e.send(t, http.MethodGet, "/notifications", "read", "")
	page := decode[clientapi.NotificationPage](t, res)
	if len(page.Items) != 3 {
		t.Fatalf("items = %+v, want 3", page.Items)
	}
	gone, decided, filed := page.Items[0], page.Items[1], page.Items[2]
	if gone.ModerationAction != nil {
		t.Errorf("appeal_decided of an unknown appeal = %+v, want no action", gone)
	}
	if ma := decided.ModerationAction; decided.Type != clientapi.NotificationTypeAppealDecided || ma == nil || ma.Id != actionID(2) || ma.Appeal == nil ||
		ma.Appeal.State != clientapi.ModerationAppealStateUpheld || ma.Appeal.DecisionReason == nil || *ma.Appeal.DecisionReason != "It was spam." {
		t.Errorf("appeal_decided = %+v, want the limit with the upheld appeal", decided)
	}
	if filed.Type != clientapi.NotificationTypeAdminAppeal || filed.AppealId == nil || *filed.AppealId != appealID(1) || filed.ModerationAction != nil {
		t.Errorf("admin_appeal = %+v, want appeal %s", filed, appealID(1))
	}
}
