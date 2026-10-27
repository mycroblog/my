// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestLoginProviderKinds(t *testing.T) {
	for _, k := range LoginProviderKinds() {
		if !k.Valid() {
			t.Errorf("%q.Valid() = false", k)
		}
	}
	for _, k := range []LoginProviderKind{"", "oauth2", "OIDC", "discord"} {
		if k.Valid() {
			t.Errorf("%q.Valid() = true", k)
		}
	}
	for _, s := range []LoginProviderState{LoginProviderEnabled, LoginProviderDisabled, LoginProviderDeleted} {
		if !s.Valid() {
			t.Errorf("%q.Valid() = false", s)
		}
	}
	if LoginProviderState("paused").Valid() {
		t.Error(`"paused".Valid() = true`)
	}
}

func TestNewLoginProviderSettingsDefaults(t *testing.T) {
	s := NewLoginProviderSettings(LoginProviderGitHub)
	if s.Kind != LoginProviderGitHub || s.AllowSignup || !s.AllowLink {
		t.Errorf("NewLoginProviderSettings = %+v, want github without signup and with linking", s)
	}
}

func TestLoginProviderSettingsNormalize(t *testing.T) {
	got := LoginProviderSettings{
		Kind:        LoginProviderOIDC,
		DisplayName: "  Example SSO ",
		Issuer:      " https://id.example.com/ ",
		ClientID:    " my-client ",
		Scopes:      []string{"email", " groups ", "email", ""},
		Claims: []ClaimCondition{
			{Claim: "hd", Values: []string{"example.com", " example.org "}},
			{Claim: " groups", Values: []string{"staff"}},
			{Claim: "hd", Values: []string{"example.com"}},
		},
	}.Normalize()
	want := LoginProviderSettings{
		Kind:        LoginProviderOIDC,
		DisplayName: "Example SSO",
		Issuer:      "https://id.example.com/", // the iss of the ID tokens must equal it
		ClientID:    "my-client",
		Scopes:      []string{"email", "groups"},
		Claims: []ClaimCondition{
			{Claim: "groups", Values: []string{"staff"}},
			{Claim: "hd", Values: []string{"example.com", "example.org"}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Normalize = %+v\nwant %+v", got, want)
	}

	gh := LoginProviderSettings{Kind: LoginProviderGitHub, Organizations: []string{" MyCroBlog ", "mycroblog", "Acme"}}.Normalize()
	if want := []string{"acme", "mycroblog"}; !reflect.DeepEqual(gh.Organizations, want) {
		t.Errorf("Organizations = %v, want %v", gh.Organizations, want)
	}

	for in, want := range map[string]string{
		"Mastodon.Social":             "https://mastodon.social",
		"https://Mastodon.Social/":    "https://mastodon.social",
		"https://example.social:8443": "https://example.social:8443",
		"http://example.social":       "http://example.social",
		"example.social/@alice":       "https://example.social/@alice",
	} {
		s := LoginProviderSettings{Kind: LoginProviderMastodon, Issuer: in}.Normalize()
		if s.Issuer != want {
			t.Errorf("Normalize(mastodon %q).Issuer = %q, want %q", in, s.Issuer, want)
		}
	}
}

func validSettings(k LoginProviderKind) LoginProviderSettings {
	s := NewLoginProviderSettings(k)
	s.DisplayName = "Provider"
	s.ClientID = "client"
	switch k {
	case LoginProviderOIDC:
		s.Issuer = "https://accounts.google.com"
	case LoginProviderMastodon:
		s.Issuer = "https://mastodon.social"
	}
	return s
}

func TestLoginProviderSettingsValidate(t *testing.T) {
	for _, k := range LoginProviderKinds() {
		if err := validSettings(k).Validate(); err != nil {
			t.Errorf("Validate(%s) = %v, want nil", k, err)
		}
	}
	ok := func(k LoginProviderKind, f func(*LoginProviderSettings)) LoginProviderSettings {
		s := validSettings(k)
		f(&s)
		return s.Normalize()
	}
	for name, s := range map[string]LoginProviderSettings{
		"oidc with a path": ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Issuer = "https://login.example.com/realms/staff" }),
		"oidc with a port": ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Issuer = "https://idp.internal:8443" }),
		"oidc with claims": ok(LoginProviderOIDC, func(s *LoginProviderSettings) {
			s.Claims = []ClaimCondition{{Claim: "hd", Values: []string{"example.com"}}}
		}),
		"oidc with scopes":       ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Scopes = []string{"groups", "https://example.com/scope"} }),
		"github with orgs":       ok(LoginProviderGitHub, func(s *LoginProviderSettings) { s.Organizations = []string{"mycroblog", "a-b-c"} }),
		"github with read:org":   ok(LoginProviderGitHub, func(s *LoginProviderSettings) { s.Scopes = []string{"read:org"} }),
		"mastodon as a domain":   ok(LoginProviderMastodon, func(s *LoginProviderSettings) { s.Issuer = "Example.Social" }),
		"signup without linking": ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.AllowSignup, s.AllowLink = true, false }),
		"a long name":            ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.DisplayName = strings.Repeat("名", MaxLoginProviderDisplayName) }),
	} {
		if err := s.Validate(); err != nil {
			t.Errorf("Validate(%s) = %v, want nil", name, err)
		}
	}

	for name, tc := range map[string]struct {
		s    LoginProviderSettings
		want error
	}{
		"unknown kind":             {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Kind = "discord" }), ErrInvalidLoginProviderKind},
		"no name":                  {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.DisplayName = "  " }), ErrInvalidLoginProviderName},
		"long name":                {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.DisplayName = strings.Repeat("a", MaxLoginProviderDisplayName+1) }), ErrInvalidLoginProviderName},
		"name with a newline":      {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.DisplayName = "a\nb" }), ErrInvalidLoginProviderName},
		"no client_id":             {ok(LoginProviderGitHub, func(s *LoginProviderSettings) { s.ClientID = "" }), ErrInvalidLoginProviderClientID},
		"long client_id":           {ok(LoginProviderGitHub, func(s *LoginProviderSettings) { s.ClientID = strings.Repeat("c", MaxLoginProviderClientID+1) }), ErrInvalidLoginProviderClientID},
		"client_id with a tab":     {ok(LoginProviderGitHub, func(s *LoginProviderSettings) { s.ClientID = "a\tb" }), ErrInvalidLoginProviderClientID},
		"oidc without an issuer":   {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Issuer = "" }), ErrInvalidLoginProviderURL},
		"oidc over http":           {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Issuer = "http://id.example.com" }), ErrInvalidLoginProviderURL},
		"oidc without a host":      {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Issuer = "https:///path" }), ErrInvalidLoginProviderURL},
		"oidc relative":            {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Issuer = "id.example.com" }), ErrInvalidLoginProviderURL},
		"oidc with user info":      {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Issuer = "https://user:pass@id.example.com" }), ErrInvalidLoginProviderURL},
		"oidc with a query":        {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Issuer = "https://id.example.com/?tenant=1" }), ErrInvalidLoginProviderURL},
		"oidc with an empty query": {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Issuer = "https://id.example.com/?" }), ErrInvalidLoginProviderURL},
		"oidc with a fragment":     {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Issuer = "https://id.example.com/#x" }), ErrInvalidLoginProviderURL},
		"oidc too long": {ok(LoginProviderOIDC, func(s *LoginProviderSettings) {
			s.Issuer = "https://id.example.com/" + strings.Repeat("p", MaxLoginProviderURL)
		}), ErrInvalidLoginProviderURL},
		"github with an issuer":     {ok(LoginProviderGitHub, func(s *LoginProviderSettings) { s.Issuer = "https://github.example.com" }), ErrInvalidLoginProviderURL},
		"mastodon without a server": {ok(LoginProviderMastodon, func(s *LoginProviderSettings) { s.Issuer = "" }), ErrInvalidLoginProviderURL},
		"mastodon over http":        {ok(LoginProviderMastodon, func(s *LoginProviderSettings) { s.Issuer = "http://example.social" }), ErrInvalidLoginProviderURL},
		"mastodon with a path":      {ok(LoginProviderMastodon, func(s *LoginProviderSettings) { s.Issuer = "https://example.social/@alice" }), ErrInvalidLoginProviderURL},
		"mastodon with a query":     {ok(LoginProviderMastodon, func(s *LoginProviderSettings) { s.Issuer = "https://example.social?x" }), ErrInvalidLoginProviderURL},
		"mastodon with scopes":      {ok(LoginProviderMastodon, func(s *LoginProviderSettings) { s.Scopes = []string{"read:accounts"} }), ErrInvalidLoginProviderScopes},
		"scope with a quote":        {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Scopes = []string{`a"b`} }), ErrInvalidLoginProviderScopes},
		"scope with a backslash":    {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Scopes = []string{`a\b`} }), ErrInvalidLoginProviderScopes},
		"scope with a space":        {LoginProviderSettings{Kind: LoginProviderOIDC, DisplayName: "P", ClientID: "c", Issuer: "https://id.example.com", Scopes: []string{"a b"}}, ErrInvalidLoginProviderScopes},
		"scope not ASCII":           {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Scopes = []string{"スコープ"} }), ErrInvalidLoginProviderScopes},
		"too many scopes": {ok(LoginProviderOIDC, func(s *LoginProviderSettings) {
			for i := range MaxLoginProviderScopes + 1 {
				s.Scopes = append(s.Scopes, "s"+strings.Repeat("x", i))
			}
		}), ErrInvalidLoginProviderScopes},
		"claims on github":   {ok(LoginProviderGitHub, func(s *LoginProviderSettings) { s.Claims = []ClaimCondition{{Claim: "hd", Values: []string{"x"}}} }), ErrInvalidLoginProviderConditions},
		"claims on mastodon": {ok(LoginProviderMastodon, func(s *LoginProviderSettings) { s.Claims = []ClaimCondition{{Claim: "hd", Values: []string{"x"}}} }), ErrInvalidLoginProviderConditions},
		"orgs on oidc":       {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Organizations = []string{"acme"} }), ErrInvalidLoginProviderConditions},
		"orgs on mastodon":   {ok(LoginProviderMastodon, func(s *LoginProviderSettings) { s.Organizations = []string{"acme"} }), ErrInvalidLoginProviderConditions},
		"claim without values": {ok(LoginProviderOIDC, func(s *LoginProviderSettings) {
			s.Claims = []ClaimCondition{{Claim: "hd", Values: []string{" "}}}
		}), ErrInvalidLoginProviderConditions},
		"claim without a name": {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Claims = []ClaimCondition{{Claim: " ", Values: []string{"x"}}} }), ErrInvalidLoginProviderConditions},
		"claim with a space":   {ok(LoginProviderOIDC, func(s *LoginProviderSettings) { s.Claims = []ClaimCondition{{Claim: "a b", Values: []string{"x"}}} }), ErrInvalidLoginProviderConditions},
		"claim value with a newline": {ok(LoginProviderOIDC, func(s *LoginProviderSettings) {
			s.Claims = []ClaimCondition{{Claim: "hd", Values: []string{"a\nb"}}}
		}), ErrInvalidLoginProviderConditions},
		"long claim value": {ok(LoginProviderOIDC, func(s *LoginProviderSettings) {
			s.Claims = []ClaimCondition{{Claim: "hd", Values: []string{strings.Repeat("v", MaxLoginProviderClaimValue+1)}}}
		}), ErrInvalidLoginProviderConditions},
		"too many claims": {ok(LoginProviderOIDC, func(s *LoginProviderSettings) {
			for i := range MaxLoginProviderClaims + 1 {
				s.Claims = append(s.Claims, ClaimCondition{Claim: "c" + strings.Repeat("x", i), Values: []string{"v"}})
			}
		}), ErrInvalidLoginProviderConditions},
		"org with an underscore":   {ok(LoginProviderGitHub, func(s *LoginProviderSettings) { s.Organizations = []string{"my_org"} }), ErrInvalidLoginProviderConditions},
		"org ending in a hyphen":   {ok(LoginProviderGitHub, func(s *LoginProviderSettings) { s.Organizations = []string{"org-"} }), ErrInvalidLoginProviderConditions},
		"org with a double hyphen": {ok(LoginProviderGitHub, func(s *LoginProviderSettings) { s.Organizations = []string{"a--b"} }), ErrInvalidLoginProviderConditions},
		"org too long":             {ok(LoginProviderGitHub, func(s *LoginProviderSettings) { s.Organizations = []string{strings.Repeat("o", MaxGitHubLogin+1)} }), ErrInvalidLoginProviderConditions},
		"org as a URL":             {ok(LoginProviderGitHub, func(s *LoginProviderSettings) { s.Organizations = []string{"https://github.com/acme"} }), ErrInvalidLoginProviderConditions},
	} {
		if err := tc.s.Validate(); !errors.Is(err, tc.want) {
			t.Errorf("Validate(%s) = %v, want %v", name, err, tc.want)
		}
	}
}

func TestValidateLoginProviderChange(t *testing.T) {
	prev := validSettings(LoginProviderOIDC)
	next := prev
	next.DisplayName = "Renamed"
	next.Issuer = "https://other.example.com"
	if err := ValidateLoginProviderChange(prev, next); err != nil {
		t.Errorf("ValidateLoginProviderChange(renamed) = %v, want nil", err)
	}
	if err := ValidateLoginProviderChange(prev, validSettings(LoginProviderGitHub)); !errors.Is(err, ErrLoginProviderKindChanged) {
		t.Errorf("ValidateLoginProviderChange(oidc to github) = %v, want ErrLoginProviderKindChanged", err)
	}
}

func TestValidateExternalSubject(t *testing.T) {
	for _, s := range []string{"1", "110169484474386276334", "AItOawl-x_y", strings.Repeat("s", MaxExternalSubject)} {
		if err := ValidateExternalSubject(s); err != nil {
			t.Errorf("ValidateExternalSubject(%q) = %v", s, err)
		}
	}
	for _, s := range []string{"", strings.Repeat("s", MaxExternalSubject+1), "a\x00b", "\xff"} {
		if err := ValidateExternalSubject(s); !errors.Is(err, ErrInvalidExternalSubject) {
			t.Errorf("ValidateExternalSubject(%q) = %v, want ErrInvalidExternalSubject", s, err)
		}
	}
}
