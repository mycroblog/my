// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/base32"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/authpages"
)

func (e *externalEnv) limitToProviders(on bool) {
	e.t.Helper()
	value := "false"
	if on {
		value = "true"
	}
	if _, err := e.s.app.Instance.SetSetting(e.t.Context(), appinstance.SetSettingInput{Key: instance.SettingLoginProvidersOnly, Value: value}); err != nil {
		e.t.Fatal(err)
	}
}

const refusedOnThePage = "This server signs in only through the login providers."

// TestLoginProvidersOnlyPasswordSignIn drives the sign-in page on the real server: the limit
// follows the setting and the providers from one request to the next, with no restart.
func TestLoginProvidersOnlyPasswordSignIn(t *testing.T) {
	t.Parallel()
	e := newExternalEnv(t)
	alice := e.user("alice", "alice@example.com")
	signIn := func(c *authClient, form url.Values) (*http.Response, string) {
		return c.do(http.MethodPost, authpages.PathSignIn, form, nil)
	}
	works := func(name string) {
		t.Helper()
		c := e.client()
		if resp, body := signIn(c, c.form("email", "alice@example.com", "password", externalTestPassword)); resp.StatusCode != http.StatusSeeOther || e.sessionUser(c) != alice {
			t.Errorf("%s: sign-in = %d\n%s", name, resp.StatusCode, body)
		}
	}
	refused := func(name string) {
		t.Helper()
		c := e.client()
		_, page := c.do(http.MethodGet, authpages.PathSignIn, nil, nil)
		if strings.Contains(page, `name="password"`) || !strings.Contains(page, refusedOnThePage) {
			t.Errorf("%s: the page still offers the password form:\n%s", name, page)
		}
		// The form of a page opened before the limit: its token is the browser's cookie.
		token := e.client()
		token.do(http.MethodGet, authpages.PathSignIn, nil, nil)
		form := url.Values{"csrf_token": {token.cookies[authpages.CookieCSRF]}, "email": {"alice@example.com"}, "password": {externalTestPassword}}
		first := ""
		for i, fields := range []url.Values{
			form,
			withFields(form, "email", "alice@example.com", "password", "wrong"),
			withFields(form, "email", "nobody@example.com", "password", externalTestPassword),
			withFields(form, "email", "", "password", ""),
		} {
			resp, body := signIn(token, fields)
			if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, refusedOnThePage) || e.sessionUser(token) != "" {
				t.Errorf("%s: POST %d = %d\n%s", name, i, resp.StatusCode, body)
			}
			if i == 0 {
				first = body
			} else if body != first {
				t.Errorf("%s: answer %d differs from the first:\n%s\n---\n%s", name, i, body, first)
			}
		}
	}

	works("no provider, setting off")
	e.limitToProviders(true)
	works("no provider, setting on")
	id := e.provider("IdP", false)
	refused("an enabled provider")
	e.limitToProviders(false)
	works("setting off")
	e.limitToProviders(true)
	refused("setting back on")
	e.disable(id)
	works("the provider disabled")
}

func withFields(form url.Values, fields ...string) url.Values {
	out := url.Values{}
	for k, v := range form {
		out[k] = v
	}
	for i := 0; i+1 < len(fields); i += 2 {
		out.Set(fields[i], fields[i+1])
	}
	return out
}

// TestLoginProvidersOnlyKeepsWhatIsRunning checks that the limit ends no session and no token,
// and that sign-in and sign-up at a provider, and the code after one, go on.
func TestLoginProvidersOnlyKeepsWhatIsRunning(t *testing.T) {
	t.Parallel()
	e := newExternalEnvWith(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	alice := e.user("alice", "alice@example.com")
	session := e.signedIn("alice@example.com")
	tokens := issueTokens(t, e.s.app, alice, "")
	id := e.provider("IdP", true)
	e.link(id, "subject-alice", alice)
	e.limitToProviders(true)

	if e.sessionUser(session) != alice {
		t.Error("the browser's session ended")
	}
	if resp, body := e.client().do(http.MethodGet, "/api/v1/accounts/verify_credentials", nil, map[string]string{"Authorization": "Bearer " + tokens.AccessToken}); resp.StatusCode != http.StatusOK {
		t.Errorf("the access token = %d\n%s", resp.StatusCode, body)
	}
	if refreshed, err := e.s.app.Identity.RefreshWebClient(t.Context(), tokens.RefreshToken); err != nil || refreshed.AccessToken == "" {
		t.Errorf("refresh = %+v, %v", refreshed, err)
	}

	c := e.client()
	if resp, body := e.signInWith(c, id, map[string]any{"sub": "subject-alice"}); resp.StatusCode != http.StatusSeeOther || e.sessionUser(c) != alice {
		t.Errorf("sign-in at the provider = %d\n%s", resp.StatusCode, body)
	}

	// A new user signs up with the provider; the form asks for no password.
	e.setMode(instance.RegistrationOpen)
	c = e.client()
	resp2, body := e.signInWith(c, id, map[string]any{"sub": "subject-dana", "email": "dana@example.com", "email_verified": true, "preferred_username": "dana"})
	if resp2.StatusCode != http.StatusOK || !strings.Contains(body, `action="/auth/external/sign_up"`) || strings.Contains(body, `name="password"`) {
		t.Fatalf("sign-up form = %d\n%s", resp2.StatusCode, body)
	}
	m := csrfToken.FindStringSubmatch(body)
	ticket := ticketField.FindStringSubmatch(body)
	if m == nil || ticket == nil {
		t.Fatalf("no form fields in\n%s", body)
	}
	resp2, body = c.do(http.MethodPost, authpages.PathExternalSignUp, url.Values{
		"csrf_token": {m[1]}, "ticket": {ticket[1]}, "username": {"dana"}, "email": {"dana@example.com"},
	}, nil)
	if resp2.StatusCode != http.StatusSeeOther || e.sessionUser(c) == "" {
		t.Errorf("sign-up at the provider = %d\n%s", resp2.StatusCode, body)
	}

	// The password form of the registration page is refused, whatever it holds.
	c = e.client()
	_, page := c.do(http.MethodGet, authpages.PathSignUp, nil, nil)
	if strings.Contains(page, `name="password"`) || !strings.Contains(page, "/auth/external/"+id+"/start") {
		t.Errorf("registration page:\n%s", page)
	}
	resp2, body = c.do(http.MethodPost, authpages.PathSignUp, url.Values{
		"csrf_token": {c.cookies[authpages.CookieCSRF]}, "username": {"erin"}, "email": {"erin@example.com"}, "password": {externalTestPassword},
	}, nil)
	if resp2.StatusCode != http.StatusForbidden || e.sessionUser(c) != "" {
		t.Errorf("password sign-up = %d\n%s", resp2.StatusCode, body)
	}
}

// TestLoginProvidersOnlyLinkFromASession is the way out of the limit: a user with only a password,
// signed in before it, confirms with the password and links a provider, and then signs in there.
func TestLoginProvidersOnlyLinkFromASession(t *testing.T) {
	t.Parallel()
	e := newExternalEnv(t)
	ctx := t.Context()
	svc := e.s.app.Identity
	alice := e.user("alice", "alice@example.com")
	c := e.signedIn("alice@example.com")
	id := e.provider("IdP", false)
	e.limitToProviders(true)

	if limited, err := svc.LoginProvidersOnly(ctx); err != nil || !limited {
		t.Fatalf("LoginProvidersOnly = %v, %v", limited, err)
	}
	has, err := svc.HasPassword(ctx, alice)
	if err != nil || !has {
		t.Fatalf("HasPassword = %v, %v", has, err)
	}
	links, err := svc.LinkedIdentities(ctx, alice)
	if err != nil || len(links.Links) != 0 || len(links.Providers) != 1 {
		t.Fatalf("LinkedIdentities = %+v, %v", links, err)
	}
	start, err := svc.BeginExternalLink(ctx, alice, id, identity.Confirmation{Password: externalTestPassword}, "/settings/linked-accounts")
	if err != nil {
		t.Fatalf("BeginExternalLink with the password = %v", err)
	}
	if _, out := e.runIntent(c, start, id, map[string]any{"sub": "subject-alice"}); out.Get("linked") != id {
		t.Fatalf("link outcome = %v", out)
	}
	c2 := e.client()
	if resp, body := e.signInWith(c2, id, map[string]any{"sub": "subject-alice"}); resp.StatusCode != http.StatusSeeOther || e.sessionUser(c2) != alice {
		t.Errorf("sign-in with the new link = %d\n%s", resp.StatusCode, body)
	}
	// That link is now the user's only way in, so it stays; the password does not stand in.
	list, err := svc.LinkedIdentities(ctx, alice)
	if err != nil || len(list.Links) != 1 {
		t.Fatalf("LinkedIdentities = %+v, %v", list, err)
	}
	if err := svc.UnlinkExternalIdentity(ctx, alice, list.Links[0].LinkID); err == nil {
		t.Error("the last link to an enabled provider was unlinked under the limit")
	}
	// A session that the limit found keeps the settings' other confirmations too.
	if err := svc.ConfirmMove(ctx, alice, identity.Confirmation{Password: externalTestPassword}); err != nil {
		t.Errorf("ConfirmMove = %v", err)
	}
}

// TestLoginProvidersOnlySecondFactor refuses the code of a password sign-in that began before the
// limit, and takes the code after a provider's sign-in.
func TestLoginProvidersOnlySecondFactor(t *testing.T) {
	t.Parallel()
	e := newExternalEnv(t)
	ctx := t.Context()
	alice := e.user("alice", "alice@example.com")
	id := e.provider("IdP", false)
	e.link(id, "subject-alice", alice)
	setup, err := e.s.app.Identity.BeginTOTPSetup(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(setup.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.app.Identity.EnableTOTP(ctx, alice, setup.Token, domain.TOTPCode(secret, domain.TOTPStep(time.Now()))); err != nil {
		t.Fatal(err)
	}
	// The code of the next step: the one EnableTOTP used is spent.
	code := func(step int) string {
		return domain.TOTPCode(secret, domain.TOTPStep(time.Now().Add(time.Duration(step)*30*time.Second)))
	}

	c := e.client()
	if resp, body := c.do(http.MethodPost, authpages.PathSignIn, c.form("email", "alice@example.com", "password", externalTestPassword), nil); resp.Header.Get("Location") != authpages.PathSignInTwoFactor {
		t.Fatalf("sign-in = %d\n%s", resp.StatusCode, body)
	}
	form := c.formAt(authpages.PathSignInTwoFactor, "code", code(1))
	e.limitToProviders(true)
	resp, body := c.do(http.MethodPost, authpages.PathSignInTwoFactor, form, nil)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, refusedOnThePage) || e.sessionUser(c) != "" {
		t.Errorf("code of a password sign-in = %d\n%s", resp.StatusCode, body)
	}
	if _, ok := c.cookies[authpages.CookieTwoFactor]; ok {
		t.Error("the refusal kept the challenge")
	}

	c = e.client()
	if resp, body := e.signInWith(c, id, map[string]any{"sub": "subject-alice"}); resp.Header.Get("Location") != authpages.PathSignInTwoFactor {
		t.Fatalf("sign-in at the provider = %d %q\n%s", resp.StatusCode, resp.Header.Get("Location"), body)
	}
	form = c.formAt(authpages.PathSignInTwoFactor, "code", code(1))
	if resp, body := c.do(http.MethodPost, authpages.PathSignInTwoFactor, form, nil); resp.StatusCode != http.StatusSeeOther || e.sessionUser(c) != alice {
		t.Errorf("code after the provider = %d\n%s", resp.StatusCode, body)
	}
}

// TestLoginProvidersOnlyInstanceAndImpact reads what clients learn: GET /instance for everyone and
// the count for those with the settings permission.
func TestLoginProvidersOnlyInstanceAndImpact(t *testing.T) {
	t.Parallel()
	e := newExternalEnvWith(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	e.setMode(instance.RegistrationOpen)
	member, err := e.s.app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{Username: "alice", Password: externalTestPassword})
	if err != nil {
		t.Fatal(err)
	}
	e.user("bob", "bob@example.com")
	admin, err := e.s.app.Identity.CreateAdmin(t.Context(), identity.CreateAdminInput{Username: "root", Email: "root@example.com", Password: externalTestPassword})
	if err != nil {
		t.Fatal(err)
	}
	get := func(path, token string, out any) int {
		t.Helper()
		header := map[string]string{}
		if token != "" {
			header["Authorization"] = "Bearer " + token
		}
		resp, body := e.client().do(http.MethodGet, path, nil, header)
		if out != nil && resp.StatusCode == http.StatusOK {
			if err := json.Unmarshal([]byte(body), out); err != nil {
				t.Fatalf("%s: %v\n%s", path, err, body)
			}
		}
		return resp.StatusCode
	}
	type inst struct {
		LoginProvidersOnly bool `json:"login_providers_only"`
		Registrations      struct {
			Mode string `json:"mode"`
		} `json:"registrations"`
	}
	check := func(name string, wantLimited bool, wantMode string) {
		t.Helper()
		var got inst
		if code := get("/api/v1/instance", "", &got); code != http.StatusOK || got.LoginProvidersOnly != wantLimited || got.Registrations.Mode != wantMode {
			t.Errorf("%s: GET /instance = %d %+v; want %v, %s", name, code, got, wantLimited, wantMode)
		}
	}
	check("no setting", false, "open")
	e.limitToProviders(true)
	check("no provider", false, "open")
	id := e.provider("IdP", false)
	check("a provider without sign-ups", true, "closed")
	e.provider("Other", true)
	check("a provider with sign-ups", true, "open")

	type impact struct {
		SettingOn            bool `json:"setting_on"`
		EnabledProviders     int  `json:"enabled_providers"`
		UsersWithoutProvider int  `json:"users_without_provider"`
		IncludesViewer       bool `json:"includes_viewer"`
	}
	const path = "/api/v1/admin/settings/login_providers_only/impact"
	if code := get(path, "", nil); code != http.StatusUnauthorized {
		t.Errorf("impact without a token = %d", code)
	}
	userTokens := issueTokens(t, e.s.app, member.User.ID, "")
	if code := get(path, userTokens.AccessToken, nil); code != http.StatusForbidden {
		t.Errorf("impact as a user = %d", code)
	}
	adminTokens := issueTokens(t, e.s.app, admin.User.ID, "")
	var got impact
	if code := get(path, adminTokens.AccessToken, &got); code != http.StatusOK || !got.SettingOn || got.EnabledProviders != 2 || got.UsersWithoutProvider != 3 || !got.IncludesViewer {
		t.Errorf("impact = %d %+v", code, got)
	}
	e.link(id, "subject-root", admin.User.ID)
	if code := get(path, adminTokens.AccessToken, &got); code != http.StatusOK || got.UsersWithoutProvider != 2 || got.IncludesViewer {
		t.Errorf("impact with the administrator linked = %d %+v", code, got)
	}
}

// jsonRequest sends a client API request with a bearer token and returns its status and body.
func (e *externalEnv) jsonRequest(method, path, token, body string) (int, string) {
	e.t.Helper()
	req, err := http.NewRequestWithContext(e.t.Context(), method, e.s.url+path, strings.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}

// TestLoginProvidersOnlyClientAPI goes through the client API with a session's token: no new
// credential, the status still reads, the link confirmed with the password, and the reset link that
// says it does not work.
func TestLoginProvidersOnlyClientAPI(t *testing.T) {
	t.Parallel()
	e := newExternalEnvWith(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	admin, err := e.s.app.Identity.CreateAdmin(t.Context(), identity.CreateAdminInput{Username: "root", Email: "root@example.com", Password: externalTestPassword})
	if err != nil {
		t.Fatal(err)
	}
	browser := e.signedIn("root@example.com")
	tokens := issueTokens(t, e.s.app, admin.User.ID, "")
	id := e.provider("IdP", false)
	const refused = "/api/v1/problems/login-providers-only"
	change := `{"current_password":"` + externalTestPassword + `","new_password":"another good password"}`
	register := `{"name":"phone","password":"` + externalTestPassword + `"}`

	e.limitToProviders(true)
	if code, body := e.jsonRequest(http.MethodPost, "/api/v1/accounts/password", tokens.AccessToken, change); code != http.StatusForbidden || !strings.Contains(body, refused) {
		t.Errorf("change the password = %d %s", code, body)
	}
	if code, body := e.jsonRequest(http.MethodPost, "/api/v1/passkeys/registration", tokens.AccessToken, register); code != http.StatusForbidden || !strings.Contains(body, refused) {
		t.Errorf("add a passkey = %d %s", code, body)
	}
	if code, body := e.jsonRequest(http.MethodGet, "/api/v1/accounts/password", tokens.AccessToken, ""); code != http.StatusOK || !strings.Contains(body, `"has_password":true`) {
		t.Errorf("password status = %d %s", code, body)
	}
	account, err := e.s.app.Accounts.FindByHandle(t.Context(), "root", "")
	if err != nil {
		t.Fatal(err)
	}
	if code, body := e.jsonRequest(http.MethodPost, "/api/v1/admin/accounts/"+account.Account.ID+"/password_reset_link", tokens.AccessToken, ""); code != http.StatusCreated || !strings.Contains(body, `"login_providers_only":true`) {
		t.Errorf("issue a reset link = %d %s", code, body)
	}
	code, body := e.jsonRequest(http.MethodPost, "/api/v1/linked_identities", tokens.AccessToken, `{"provider_id":"`+id+`","password":"`+externalTestPassword+`","return_to":"/settings/linked-accounts"}`)
	if code != http.StatusOK {
		t.Fatalf("begin a link with the password = %d %s", code, body)
	}
	var start identity.ExternalIntent
	var answer struct {
		StartURL string `json:"start_url"`
	}
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatal(err)
	}
	start.StartURL = answer.StartURL
	if _, out := e.runIntent(browser, start, id, map[string]any{"sub": "subject-root"}); out.Get("linked") != id {
		t.Errorf("link outcome = %v", out)
	}

	e.limitToProviders(false)
	if code, body := e.jsonRequest(http.MethodPost, "/api/v1/accounts/password", tokens.AccessToken, change); code != http.StatusNoContent {
		t.Errorf("change the password without the limit = %d %s", code, body)
	}
}
