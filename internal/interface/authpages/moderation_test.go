// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/pkg/testkit"
)

// restrictedUser returns a browser signed in as alice, whose account account moderation keeps out of
// the app with the actions of mod, on screens in English.
// action1 and action3 are the actions the appeal forms are posted for.
var (
	action1 = testkit.OrderedID(1)
	action3 = testkit.OrderedID(3)
)

func restrictedUser(t *testing.T, mod *fakeModeration) (*browser, *fakeIdentity) {
	t.Helper()
	id := newFakeIdentity()
	id.access[userAlice] = appidentity.Access{
		Registration: identity.RegistrationApproved, EmailVerified: true, AccountID: accountAlice, AccountActive: true, LoginRestricted: true,
	}
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := mux{http.NewServeMux()}
	authpages.New(authpages.Options{Identity: id, Locales: &i18n.Resolver{Bundle: b}, Moderation: mod}).Register(m)
	br := newBrowser(t, m)
	page := br.do(http.MethodGet, authpages.PathSignIn, nil, map[string]string{"Accept-Language": "en"})
	if w := br.do(http.MethodPost, authpages.PathSignIn, br.form(page, "email", "alice@example.com", "password", "correct horse"), nil); w.Code != http.StatusSeeOther {
		t.Fatalf("sign-in = %d", w.Code)
	}
	return br, id
}

var en = map[string]string{"Accept-Language": "en"}

type fakeModeration struct {
	actions map[string][]appmoderation.TargetActionView
	filters []appmoderation.OwnActionFilter

	appeals   []fakeAppeal
	appealErr error
}

type fakeAppeal struct{ accountID, actionID, text string }

func (f *fakeModeration) OwnActions(_ context.Context, accountID string, flt appmoderation.OwnActionFilter, _ int) ([]appmoderation.TargetActionView, error) {
	f.filters = append(f.filters, flt)
	return f.actions[accountID], nil
}

// FileAppeal records the appeal and marks the action appealed, unless appealErr is set.
func (f *fakeModeration) FileAppeal(_ context.Context, accountID, actionID, text string) (appmoderation.TargetActionView, error) {
	f.appeals = append(f.appeals, fakeAppeal{accountID, actionID, text})
	if f.appealErr != nil {
		return appmoderation.TargetActionView{}, f.appealErr
	}
	for i, v := range f.actions[accountID] {
		if v.ID == actionID {
			v.Appealable = false
			v.Appeal = &appmoderation.OwnAppeal{ID: testkit.NamedID("appeal-1"), Text: text, State: moderation.AppealPending, SubmittedAt: time.Now()}
			f.actions[accountID][i] = v
			return v, nil
		}
	}
	return appmoderation.TargetActionView{}, appmoderation.ErrNotFound
}

type fakeUserLocales map[string]string

func (f fakeUserLocales) UserLocale(_ context.Context, userID string) (string, error) {
	return f[userID], nil
}

// TestStatusOfARestrictedUser checks that a user whom moderation keeps out of the app signs in
// to the screens and sees the actions in effect, in their saved locale, and gets no
// authorization code.
func TestStatusOfARestrictedUser(t *testing.T) {
	id := newFakeIdentity()
	id.access[userAlice] = appidentity.Access{
		Registration: identity.RegistrationApproved, EmailVerified: true, AccountID: accountAlice, AccountActive: true, LoginRestricted: true,
	}
	ends := time.Now().Add(72*time.Hour - time.Minute)
	mod := &fakeModeration{actions: map[string][]appmoderation.TargetActionView{accountAlice: {
		{ID: testkit.OrderedID(2), Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam, PublicReason: "spam <wave>",
			ExpiresAt: &ends, State: moderation.ActionActive},
		{ID: testkit.OrderedID(1), Kind: moderation.ActionDisableLogin, ReasonCategory: moderation.CategoryOther, State: moderation.ActionActive},
		{ID: testkit.OrderedID(0), Kind: moderation.ActionWarn, ReasonCategory: moderation.CategoryOther, PublicReason: "a warning", State: moderation.ActionActive},
	}}}
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	oauth := &fakeOAuth{}
	m := mux{http.NewServeMux()}
	authpages.New(authpages.Options{
		Identity: id, OAuth: oauth, Locales: &i18n.Resolver{Bundle: b}, Moderation: mod, UserLocales: fakeUserLocales{userAlice: "ja"},
	}).Register(m)
	br := newBrowser(t, m)

	page := br.do(http.MethodGet, authpages.PathSignIn, nil, map[string]string{"Accept-Language": "en"})
	w := br.do(http.MethodPost, authpages.PathSignIn, br.form(page, "email", "alice@example.com", "password", "correct horse", "return_to", "/home"), nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != authpages.PathSignIn {
		t.Fatalf("sign-in of a restricted user = %d → %q, want the status page", w.Code, w.Header().Get("Location"))
	}
	w = br.do(http.MethodGet, authpages.PathSignIn, nil, map[string]string{"Accept-Language": "en"})
	if w.Code != http.StatusOK || w.Header().Get("Content-Language") != "ja" {
		t.Fatalf("status page = %d in %q, want 200 in the user's locale", w.Code, w.Header().Get("Content-Language"))
	}
	body := w.Body.String()
	wantContains(t, body,
		`<h1>アカウントは処分を受けています</h1>`,
		`<strong>凍結（アプリを使えず、プロフィールと投稿が隠れる）</strong> <span class="status-badge status-badge-warning">適用中</span>`,
		`<p class="hint">理由：スパム</p>`,
		`<p dir="auto">spam &lt;wave&gt;</p>`,
		`<time datetime="`+ends.UTC().Format(time.RFC3339)+`">あと 3 日で終わります</time>`,
		`<strong>ログインの停止</strong>`,
		`<p>モデレーターが解除するまで続きます</p>`,
		`action="/auth/sign_out"`,
	)
	wantLacks(t, body, "verify_email/resend", "承認待ち", "a warning")
	if len(mod.filters) == 0 || !mod.filters[0].Active {
		t.Errorf("filters = %+v, want the actions in effect", mod.filters)
	}

	// The authorization endpoint gives no code; it shows the same page.
	w = br.do(http.MethodGet, authorizeQuery("web", webRedirect), nil, nil)
	if w.Code != http.StatusForbidden || len(oauth.issued) != 0 {
		t.Fatalf("authorize of a restricted user = %d with %d codes, want 403 and none", w.Code, len(oauth.issued))
	}
	wantContains(t, w.Body.String(), `<strong>ログインの停止</strong>`)
}

// TestAppealFromTheStatusPage checks that a user whom moderation keeps out of the app appeals an
// action from the status page: the page offers a form for each action that can be appealed,
// says why the others cannot, and shows the appeal once it is filed (FR-707).
func TestAppealFromTheStatusPage(t *testing.T) {
	now := time.Now()
	deadline, closed, ends := now.Add(48*time.Hour-time.Minute), now.Add(-time.Hour), now.Add(24*time.Hour)
	decided := now.Add(-time.Minute)
	mod := &fakeModeration{actions: map[string][]appmoderation.TargetActionView{accountAlice: {
		{ID: testkit.OrderedID(3), Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam, State: moderation.ActionActive,
			ExpiresAt: &ends, AppealDeadline: &deadline, Appealable: true},
		{ID: testkit.OrderedID(2), Kind: moderation.ActionDisableLogin, ReasonCategory: moderation.CategoryOther, State: moderation.ActionActive,
			AppealDeadline: &closed},
		{ID: testkit.OrderedID(1), Kind: moderation.ActionDisableLogin, ReasonCategory: moderation.CategoryOther, State: moderation.ActionActive,
			AppealDeadline: &deadline, Appeal: &appmoderation.OwnAppeal{
				ID: testkit.NamedID("appeal-0"), Text: "It was <not> me.", State: moderation.AppealUpheld, DecidedAt: &decided, DecisionReason: "We checked the logs.",
			}},
		{ID: testkit.OrderedID(0), Kind: moderation.ActionBan, ReasonCategory: moderation.CategoryLegal, State: moderation.ActionActive, AppealDeadline: &deadline},
	}}}
	br, _ := restrictedUser(t, mod)

	page := br.do(http.MethodGet, authpages.PathSignIn, nil, en)
	body := page.Body.String()
	wantContains(t, body,
		`<form class="form appeal-form" method="post" action="/auth/appeal">`,
		`<input type="hidden" name="action_id" value="`+action3+`">`,
		`<label for="appeal-`+action3+`">Appeal this action</label>`,
		`<p id="appeal-`+action3+`-hint" class="hint">You can appeal once, within 2 days. Up to 2,000 characters.</p>`,
		`<textarea id="appeal-`+action3+`" name="text" rows="4" required maxlength="2000" dir="auto" aria-describedby="appeal-`+action3+`-hint">`,
		`<p class="hint">The time to appeal this action has passed.</p>`,
		`<p><span class="status-badge">Action kept</span></p>`,
		`<p>A moderator reviewed your appeal and kept the action.</p>`,
		`<button class="button" type="submit">Send appeal</button>`,
		`<p dir="auto">It was &lt;not&gt; me.</p>`,
		`<p dir="auto">We checked the logs.</p>`,
		`<p class="hint">This action cannot be appealed.</p>`,
	)
	if n := strings.Count(body, `action="/auth/appeal"`); n != 1 {
		t.Errorf("the page has %d appeal forms, want 1 (for a3)", n)
	}

	w := br.do(http.MethodPost, authpages.PathAppeal, br.form(page, "action_id", action3, "text", "I did not\r\nspam."), en)
	if w.Code != http.StatusOK {
		t.Fatalf("appeal = %d\n%s", w.Code, w.Body.String())
	}
	if len(mod.appeals) != 1 || mod.appeals[0] != (fakeAppeal{accountAlice, action3, "I did not\nspam."}) {
		t.Errorf("appeals = %+v, want the account's of action 3 with LF line breaks", mod.appeals)
	}
	wantContains(t, w.Body.String(),
		`<p class="notice" role="status">Your appeal was sent. A moderator will review it.</p>`,
		`<p><span class="status-badge">Under review</span></p>`,
		`<p>Your appeal is waiting for a moderator&#39;s review.</p>`,
	)
	wantLacks(t, w.Body.String(), `action="/auth/appeal"`)
}

// TestAppealRefusals checks that the appeal form shows why an appeal was refused: a second
// appeal, a closed window, a ban that cannot be appealed, an action of someone else, and an empty
// or too long text, which goes back into its form.
func TestAppealRefusals(t *testing.T) {
	deadline := time.Now().Add(240 * time.Hour)
	newMod := func() *fakeModeration {
		return &fakeModeration{actions: map[string][]appmoderation.TargetActionView{accountAlice: {
			{ID: testkit.OrderedID(1), Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam, State: moderation.ActionActive,
				AppealDeadline: &deadline, Appealable: true},
		}}}
	}
	for _, c := range []struct {
		name   string
		err    error
		status int
		want   []string
	}{
		{"second appeal", appmoderation.ErrAlreadyAppealed, http.StatusUnprocessableEntity,
			[]string{`<p class="alert" role="alert">You have already appealed this action.</p>`}},
		{"window closed", appmoderation.ErrAppealWindowClosed, http.StatusUnprocessableEntity,
			[]string{`<p class="alert" role="alert">The time to appeal this action has passed.</p>`}},
		{"ban", appmoderation.ErrNotAppealable, http.StatusUnprocessableEntity,
			[]string{`<p class="alert" role="alert">This action cannot be appealed.</p>`}},
		{"another account's action", appmoderation.ErrNotFound, http.StatusNotFound, []string{`role="alert"`}},
		{"too long", errors.Join(moderation.ErrInvalidAppeal, &moderation.TextError{Field: "text", Max: moderation.MaxAppealText}),
			http.StatusUnprocessableEntity, []string{
				`<p class="alert" role="alert">The appeal was not sent. Check the field marked below.</p>`,
				`<p id="appeal-` + action1 + `-error" class="field-error">Write your appeal in up to 2,000 characters.</p>`,
				`aria-describedby="appeal-` + action1 + `-hint appeal-` + action1 + `-error" aria-invalid="true">some &lt;text&gt;</textarea>`,
			}},
	} {
		t.Run(c.name, func(t *testing.T) {
			mod := newMod()
			mod.appealErr = c.err
			br, _ := restrictedUser(t, mod)
			page := br.do(http.MethodGet, authpages.PathSignIn, nil, en)
			w := br.do(http.MethodPost, authpages.PathAppeal, br.form(page, "action_id", action1, "text", "some <text>"), en)
			if w.Code != c.status {
				t.Fatalf("appeal = %d, want %d\n%s", w.Code, c.status, w.Body.String())
			}
			wantContains(t, w.Body.String(), c.want...)
			wantContains(t, w.Body.String(), `<h1>Your account is restricted</h1>`)
		})
	}

	t.Run("forged form", func(t *testing.T) {
		mod := newMod()
		br, _ := restrictedUser(t, mod)
		w := br.do(http.MethodPost, authpages.PathAppeal, url.Values{"csrf_token": {strings.Repeat("A", 43)}, "action_id": {action1}, "text": {"x"}}, en)
		if w.Code != http.StatusForbidden || len(mod.appeals) != 0 {
			t.Fatalf("appeal without the anti-forgery token = %d with %d appeals, want 403 and none", w.Code, len(mod.appeals))
		}
	})

	t.Run("signed out", func(t *testing.T) {
		mod := newMod()
		br, id := restrictedUser(t, mod)
		page := br.do(http.MethodGet, authpages.PathSignIn, nil, en)
		for token := range id.live {
			delete(id.live, token)
		}
		w := br.do(http.MethodPost, authpages.PathAppeal, br.form(page, "action_id", action1, "text", "x"), en)
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != authpages.PathSignIn || len(mod.appeals) != 0 {
			t.Fatalf("appeal without a session = %d → %q with %d appeals, want the sign-in page", w.Code, w.Header().Get("Location"), len(mod.appeals))
		}
	})
}
