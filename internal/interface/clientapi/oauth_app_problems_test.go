// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// scopeRuleOAuthApps is fakeOAuthApps whose refusal of a token's scope wraps the rule the scope
// breaks, as the identity module's does: ErrUnknownScope from ParseScope, or ErrScopeBeyondClient.
type scopeRuleOAuthApps struct{ *fakeOAuthApps }

func (f scopeRuleOAuthApps) IssueOAuthAppTokens(ctx context.Context, userID, id, scope string) (appidentity.IssuedTokens, error) {
	a, err := f.own(userID, id)
	if err != nil {
		return appidentity.IssuedTokens{}, err
	}
	granted, err := identity.ParseScope(scope)
	if err == nil && !granted.Within(a.Client.Scope) {
		err = identity.ErrScopeBeyondClient
	}
	if err != nil {
		return appidentity.IssuedTokens{}, errors.Join(&appidentity.OAuthError{Code: appidentity.OAuthInvalidScope}, err)
	}
	return f.fakeOAuthApps.IssueOAuthAppTokens(ctx, userID, id, scope)
}

// oauthAppRefusal is a request whose setting breaks a rule, the field the problem names, and the
// sentence it says.
type oauthAppRefusal struct {
	name, body, field, detail string
}

// oauthAppBody is the body of a public app with the settings given, the others valid.
func oauthAppBody(t *testing.T, website, scope string, uris ...string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"name": "Bot", "website": website, "redirect_uris": uris, "scope": scope, "client_type": "public"})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// oauthAppSettingRefusals are the refusals of each rule that a setting can break, with the detail
// in ja, the locale of the test account.
func oauthAppSettingRefusals(t *testing.T) []oauthAppRefusal {
	t.Helper()
	messages, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	ja := messages.Localizer("ja")
	tooMany := make([]string, identity.MaxOAuthClientRedirectURIs+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("https://app.example/cb%d", i)
	}
	const cb = "https://app.example/cb"
	// The spec caps redirect_uris at the same 10, so the request skips the check of the spec to
	// reach the server's (sendJSON).
	tooManyBody := strings.Replace(oauthAppBody(t, "", "read", tooMany...), "{", `{"x":"invalid-on-purpose",`, 1)
	return []oauthAppRefusal{
		{"localhost redirect", oauthAppBody(t, "", "read", "http://localhost:8080/cb"), "redirect_uris", ja.ServerErrorsOauthAppRedirectUriForm()},
		{"ftp redirect", oauthAppBody(t, "", "read", "ftp://x/"), "redirect_uris", ja.ServerErrorsOauthAppRedirectUriForm()},
		{"scheme not in reverse domain notation", oauthAppBody(t, "", "read", "myapp:/cb"), "redirect_uris", ja.ServerErrorsOauthAppRedirectUriForm()},
		{"https without a host", oauthAppBody(t, "", "read", "https:///x"), "redirect_uris", ja.ServerErrorsOauthAppRedirectUriForm()},
		{"redirect with a fragment", oauthAppBody(t, "", "read", "https://app.example/cb#x"), "redirect_uris", ja.ServerErrorsOauthAppRedirectUriFragment()},
		{"too many redirects", tooManyBody, "redirect_uris", ja.ServerErrorsOauthAppTooManyRedirectUris(identity.MaxOAuthClientRedirectURIs)},
		{"ftp website", oauthAppBody(t, "ftp://app.example/", "read", cb), "website", ja.ServerErrorsOauthAppWebsiteNotHttp()},
		{"unknown scope", oauthAppBody(t, "", "read everything", cb), "scope", ja.ServerErrorsOauthAppUnknownScope()},
		{"admin scope", oauthAppBody(t, "", "read admin", cb), "scope", ja.ServerErrorsOauthAppFirstPartyScope()},
		{"supporters scope", oauthAppBody(t, "", "write:supporters", cb), "scope", ja.ServerErrorsOauthAppFirstPartyScope()},
	}
}

func checkOAuthAppRefusal(t *testing.T, what string, res *http.Response, tc oauthAppRefusal) {
	t.Helper()
	p := decodeProblem(t, res)
	if res.StatusCode != http.StatusUnprocessableEntity || p.Field == nil || *p.Field != tc.field || p.Detail == nil || *p.Detail != tc.detail {
		t.Errorf("%s %s = %d field %q detail %q, want 422 on %s saying %q", what, tc.name, res.StatusCode, deref(p.Field), deref(p.Detail), tc.field, tc.detail)
	}
}

func newScopeRuleOAuthAppEnv(t *testing.T) *authEnv {
	t.Helper()
	f := scopeRuleOAuthApps{&fakeOAuthApps{}}
	return newAuthEnv(t, func(o *clientapi.Options) { o.OAuthApps = f; o.RateLimiter = nil })
}

// The registration of an app refused for a setting names the field and says the rule its value
// breaks (#3215).
func TestOAuthAppRegistrationProblemsSayTheRule(t *testing.T) {
	e := newScopeRuleOAuthAppEnv(t)
	for _, tc := range oauthAppSettingRefusals(t) {
		checkOAuthAppRefusal(t, "create with", e.sendJSON(t, http.MethodPost, "/oauth_apps", "write", tc.body), tc)
	}
}

// A change of an app refused for a setting names the field and says the rule its value breaks
// (#3215).
func TestOAuthAppChangeProblemsSayTheRule(t *testing.T) {
	e := newScopeRuleOAuthAppEnv(t)
	res := e.sendJSON(t, http.MethodPost, "/oauth_apps", "write", oauthAppBody(t, "", "read", "https://app.example/cb"))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d, want 201", res.StatusCode)
	}
	id := decodeJSON[clientapi.OAuthAppCreated](t, res).App.Id
	for _, tc := range oauthAppSettingRefusals(t) {
		var in map[string]any
		if err := json.Unmarshal([]byte(tc.body), &in); err != nil {
			t.Fatal(err)
		}
		change := map[string]any{tc.field: in[tc.field]}
		if x, ok := in["x"]; ok {
			change["x"] = x
		}
		b, err := json.Marshal(change)
		if err != nil {
			t.Fatal(err)
		}
		checkOAuthAppRefusal(t, "update with", e.sendJSON(t, http.MethodPatch, "/oauth_apps/"+id, "write", string(b)), tc)
	}
}

// Tokens refused for their scope name the field and say the rule the scope breaks (#3215).
func TestOAuthAppTokenScopeProblemsSayTheRule(t *testing.T) {
	messages, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	ja := messages.Localizer("ja")
	e := newScopeRuleOAuthAppEnv(t)
	res := e.sendJSON(t, http.MethodPost, "/oauth_apps", "write", oauthAppBody(t, "", "read", "https://app.example/cb"))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d, want 201", res.StatusCode)
	}
	id := decodeJSON[clientapi.OAuthAppCreated](t, res).App.Id
	for _, tc := range []oauthAppRefusal{
		{"unknown scope", `{"scope":"read:everything"}`, "scope", ja.ServerErrorsOauthAppUnknownScope()},
		{"scope beyond the app", `{"scope":"write"}`, "scope", ja.ServerErrorsOauthAppScopeBeyondApp()},
	} {
		checkOAuthAppRefusal(t, "issue tokens with", e.sendJSON(t, http.MethodPost, "/oauth_apps/"+id+"/tokens", "write", tc.body), tc)
	}
}
