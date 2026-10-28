// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// TestOwnModerationActions checks, through the composition root, what a local account learns of
// the actions on it (FR-703, FR-715, NFR-MOD-6): a notified action becomes a moderation_notice
// with its kind, reason, and end; the list has every action and never the private note; and a
// suspended user, whose tokens end, still sees the actions on the sign-in screens.
func TestOwnModerationActions(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	alice := e.user("alice")
	issue := func(in appmoderation.AccountActionInput) moderation.Action {
		t.Helper()
		in.TargetAccountID = alice.id
		a, err := e.app.Moderation.IssueAccountAction(t.Context(), in)
		if err != nil {
			t.Fatalf("IssueAccountAction(%s): %v", in.Kind, err)
		}
		return a
	}
	warn := issue(appmoderation.AccountActionInput{
		Kind: moderation.ActionWarn, ReasonCategory: moderation.CategoryViolation, PublicReason: "Please be kind.",
		PrivateNote: "second time this week", NotifyTarget: true,
	})
	limit := issue(appmoderation.AccountActionInput{
		Kind: moderation.ActionLimit, ReasonCategory: moderation.CategorySpam, Duration: 72 * time.Hour,
	})

	var notices clientapi.NotificationPage
	e.want(e.do(http.MethodGet, "/notifications?types=moderation_notice", alice.token, nil, &notices), http.StatusOK, "notifications")
	if len(notices.Items) != 1 {
		t.Fatalf("moderation notices = %+v, want the warning only", notices.Items)
	}
	if a := notices.Items[0].ModerationAction; a == nil || a.Id != warn.ID || a.Kind != clientapi.AdminAccountActionKindWarn ||
		a.PublicReason == nil || *a.PublicReason != "Please be kind." || a.KindLabel != "Warning" || a.ReasonCategoryLabel != "Violation of the instance's rules" {
		t.Errorf("the action of the notice = %+v", notices.Items[0].ModerationAction)
	}

	res := e.do(http.MethodGet, "/moderation/actions", alice.token, nil, nil)
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || strings.Contains(string(body), "second time") || strings.Contains(string(body), "private_note") {
		t.Fatalf("GET /moderation/actions = %d %s", res.StatusCode, body)
	}
	var page clientapi.ModerationActionPage
	e.want(e.do(http.MethodGet, "/moderation/actions", alice.token, nil, &page), http.StatusOK, "own actions")
	if len(page.Items) != 2 || page.Items[0].Id != limit.ID || page.Items[0].ExpiresAt == nil || page.Items[1].Id != warn.ID {
		t.Errorf("own actions = %+v, want the limit and the warning", page.Items)
	}
	if err := e.app.Moderation.LiftAccountAction(t.Context(), limit.ID, "", ""); err != nil {
		t.Fatal(err)
	}
	e.want(e.do(http.MethodGet, "/moderation/actions?active=true", alice.token, nil, &page), http.StatusOK, "own active actions")
	if len(page.Items) != 1 || page.Items[0].Id != warn.ID {
		t.Errorf("own active actions = %+v, want the warning", page.Items)
	}

	// An action on statuses names them.
	var post clientapi.Status
	e.want(e.do(http.MethodPost, "/statuses", alice.token, map[string]any{"text": "hello", "visibility": "public"}, &post), http.StatusOK, "post")
	sensitive := issue(appmoderation.AccountActionInput{
		Kind: moderation.ActionMarkStatusesSensitive, ReasonCategory: moderation.CategoryViolation, StatusIDs: []string{post.Id},
	})
	e.want(e.do(http.MethodGet, "/moderation/actions?limit=1", alice.token, nil, &page), http.StatusOK, "own actions on statuses")
	if len(page.Items) != 1 || page.Items[0].Id != sensitive.ID || len(page.Items[0].StatusIds) != 1 || page.Items[0].StatusIds[0] != post.Id {
		t.Errorf("the action on statuses = %+v, want it to name %s", page.Items, post.Id)
	}

	// The suspension ends alice's token; she signs in to the screens and sees why.
	issue(appmoderation.AccountActionInput{
		Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategoryLegal, PublicReason: "A court order.", NotifyTarget: true,
	})
	e.want(e.do(http.MethodGet, "/moderation/actions", alice.token, nil, nil), http.StatusUnauthorized, "own actions while suspended")
	c := &screens{t: t, h: e.h, cookies: map[string]string{}}
	form := c.form(authpages.PathSignIn, "email", "alice@example.com", "password", "correct horse battery staple")
	if w := c.do(http.MethodPost, authpages.PathSignIn, form); w.Code != http.StatusSeeOther || w.Header().Get("Location") != authpages.PathSignIn {
		t.Fatalf("sign-in while suspended = %d → %q, want the status page", w.Code, w.Header().Get("Location"))
	}
	w := c.do(http.MethodGet, authpages.PathSignIn, nil)
	for _, want := range []string{
		"<h1>Your account is restricted</h1>", "<strong>Suspended:", "Reason: Legal reasons", `<p dir="auto">A court order.</p>`,
		"Lasts until a moderator lifts it",
	} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("status page lacks %s:\n%s", want, w.Body.String())
		}
	}
	if strings.Contains(w.Body.String(), "Please be kind.") {
		t.Errorf("status page lists the warning, which keeps nobody out:\n%s", w.Body.String())
	}
	if w := c.do(http.MethodGet, authpages.PathAuthorize+"?"+url.Values{
		"response_type": {"code"}, "client_id": {"my-web"}, "redirect_uri": {webOrigin + "/login/callback"},
		"code_challenge": {strings.Repeat("c", 43)}, "code_challenge_method": {"S256"},
	}.Encode(), nil); w.Code != http.StatusForbidden || w.Header().Get("Location") != "" {
		t.Errorf("authorize while suspended = %d → %q, want 403 and no code", w.Code, w.Header().Get("Location"))
	}
}

// TestAppealFromTheSignInScreens checks, through the composition root, that a suspended user,
// who has no access token, appeals the suspension from the status page of the sign-in screens,
// that the other moderators get admin_appeal, and that a second appeal is refused with the
// reason (FR-707, ADR-0038).
func TestAppealFromTheSignInScreens(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob")
	ctx := t.Context()
	suspend, err := c.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: c.ids["bob"], Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam, PublicReason: "Spam links.",
	})
	if err != nil {
		t.Fatal(err)
	}
	sc := &screens{t: t, h: c.h, cookies: map[string]string{}}
	form := sc.form(authpages.PathSignIn, "email", "bob@example.com", "password", "correct horse battery staple")
	if w := sc.do(http.MethodPost, authpages.PathSignIn, form); w.Code != http.StatusSeeOther || w.Header().Get("Location") != authpages.PathSignIn {
		t.Fatalf("sign-in while suspended = %d → %q, want the status page", w.Code, w.Header().Get("Location"))
	}
	if body := sc.do(http.MethodGet, authpages.PathSignIn, nil).Body.String(); !strings.Contains(body, `name="action_id" value="`+suspend.ID+`"`) {
		t.Fatalf("status page has no appeal form for %s:\n%s", suspend.ID, body)
	}

	w := sc.do(http.MethodPost, authpages.PathAppeal, sc.form(authpages.PathSignIn, "action_id", suspend.ID, "text", "They were not spam."))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Your appeal was sent.") {
		t.Fatalf("appeal = %d\n%s", w.Code, w.Body.String())
	}
	ns, err := c.app.Notifications.List(ctx, c.ids["alice"], appnotification.Query{Types: []notification.Type{notification.TypeAdminAppeal}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 1 || ns[0].AppealID == "" {
		t.Fatalf("alice's admin_appeal notifications = %+v, want one", ns)
	}
	appeal, err := c.app.Moderation.OwnAppealAction(ctx, c.ids["bob"], ns[0].AppealID)
	if err != nil || appeal.ID != suspend.ID || appeal.Appeal == nil || appeal.Appeal.Text != "They were not spam." {
		t.Fatalf("the appealed action = %+v, %v; want the suspension with bob's text", appeal, err)
	}

	w = sc.do(http.MethodPost, authpages.PathAppeal, sc.form(authpages.PathSignIn, "action_id", suspend.ID, "text", "Again."))
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "You have already appealed this action.") {
		t.Fatalf("second appeal = %d\n%s", w.Code, w.Body.String())
	}
}

// TestBanAppealFromTheSignInScreens checks, through the composition root, that a banned user
// signs in to the status page of the sign-in screens until the ban deletes the account, appeals
// the ban there so that the other moderators get admin_appeal (FR-707, ADR-0038), and is signed
// out by the deletion and cannot sign in again.
func TestBanAppealFromTheSignInScreens(t *testing.T) {
	t.Parallel()
	// The status page counts the days left from a reading of the clock after the ban, which
	// must not be earlier than the ban.
	c := newStatusesClientWith(t, nil, platform.AppOptions{Clock: newSteadyClock()}, "alice", "bob")
	ctx := t.Context()
	ban, err := c.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: c.ids["bob"], Kind: moderation.ActionBan, ReasonCategory: moderation.CategoryLegal, PublicReason: "Illegal content.",
	})
	if err != nil {
		t.Fatal(err)
	}
	sc := &screens{t: t, h: c.h, cookies: map[string]string{}}
	signIn := func() *httptest.ResponseRecorder {
		t.Helper()
		form := sc.form(authpages.PathSignIn, "email", "bob@example.com", "password", "correct horse battery staple")
		return sc.do(http.MethodPost, authpages.PathSignIn, form)
	}
	if w := signIn(); w.Code != http.StatusSeeOther || w.Header().Get("Location") != authpages.PathSignIn {
		t.Fatalf("sign-in while banned = %d → %q, want the status page", w.Code, w.Header().Get("Location"))
	}
	if body := sc.do(http.MethodGet, authpages.PathSignIn, nil).Body.String(); !strings.Contains(body, `name="action_id" value="`+ban.ID+`"`) ||
		!strings.Contains(body, "Your account will be deleted in 30 days") {
		t.Fatalf("status page has no appeal form for %s or no deletion:\n%s", ban.ID, body)
	}
	w := sc.do(http.MethodPost, authpages.PathAppeal, sc.form(authpages.PathSignIn, "action_id", ban.ID, "text", "It was legal."))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Your appeal was sent.") {
		t.Fatalf("appeal = %d\n%s", w.Code, w.Body.String())
	}
	ns, err := c.app.Notifications.List(ctx, c.ids["alice"], appnotification.Query{Types: []notification.Type{notification.TypeAdminAppeal}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 1 || ns[0].AppealID == "" {
		t.Fatalf("alice's admin_appeal notifications = %+v, want one", ns)
	}

	// The deletion that the ban scheduled ends the session, and the account no longer signs in.
	if _, err := c.app.Accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: c.ids["bob"], Kind: account.LifecycleDeleted, InitiatedBy: account.InitiatedBySystem,
	}); err != nil {
		t.Fatal(err)
	}
	if body := sc.do(http.MethodGet, authpages.PathSignIn, nil).Body.String(); strings.Contains(body, `name="action_id"`) || !strings.Contains(body, `name="password"`) {
		t.Fatalf("sign-in screen after the deletion, want the sign-in form:\n%s", body)
	}
	if w := signIn(); w.Code == http.StatusSeeOther {
		t.Fatalf("sign-in after the deletion = %d → %q, want a refusal", w.Code, w.Header().Get("Location"))
	}
}

// screens drives the server-rendered screens on a handler, keeping their cookies.
type screens struct {
	t       *testing.T
	h       http.Handler
	cookies map[string]string
}

func (c *screens) do(method, target string, form url.Values) *httptest.ResponseRecorder {
	c.t.Helper()
	var body io.Reader = strings.NewReader("")
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	r := httptest.NewRequest(method, "https://social.example"+target, body)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("Accept-Language", "en")
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	for name, value := range c.cookies {
		r.Header.Add("Cookie", name+"="+value)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	for _, ck := range w.Result().Cookies() {
		c.cookies[ck.Name] = ck.Value
	}
	return w
}

// form fetches the screen at path and returns its form fields with the anti-forgery token.
func (c *screens) form(path string, fields ...string) url.Values {
	c.t.Helper()
	m := csrfToken.FindStringSubmatch(c.do(http.MethodGet, path, nil).Body.String())
	if m == nil {
		c.t.Fatalf("%s has no csrf_token", path)
	}
	v := url.Values{"csrf_token": {m[1]}}
	for i := 0; i+1 < len(fields); i += 2 {
		v.Set(fields[i], fields[i+1])
	}
	return v
}
