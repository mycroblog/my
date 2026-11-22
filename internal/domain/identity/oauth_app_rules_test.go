// SPDX-License-Identifier: AGPL-3.0-only

package identity_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mycroblog/my/internal/domain/identity"
)

// The refusal of a setting is the error of the rule it breaks, which wraps the error of the
// setting, so that callers tell the rule and the setting apart (#3215).
func TestOAuthClientSettingErrorsNameTheRule(t *testing.T) {
	uris := []string{"https://app.example/cb"}
	tooMany := make([]string, identity.MaxOAuthClientRedirectURIs+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("https://app.example/cb%d", i)
	}
	for _, tc := range []struct {
		name, website, scope string
		uris                 []string
		rule, setting        []error
	}{
		{"localhost redirect", "", "read", []string{"http://localhost:8080/cb"}, []error{identity.ErrRedirectURIForm, identity.ErrInvalidRedirectURI}, []error{identity.ErrInvalidOAuthClientRedirectURIs}},
		{"ftp redirect", "", "read", []string{"ftp://x/"}, []error{identity.ErrRedirectURIForm, identity.ErrInvalidRedirectURI}, []error{identity.ErrInvalidOAuthClientRedirectURIs}},
		{"scheme not in reverse domain notation", "", "read", []string{"myapp:/cb"}, []error{identity.ErrRedirectURIForm, identity.ErrInvalidRedirectURI}, []error{identity.ErrInvalidOAuthClientRedirectURIs}},
		{"https without a host", "", "read", []string{"https:///x"}, []error{identity.ErrRedirectURIForm, identity.ErrInvalidRedirectURI}, []error{identity.ErrInvalidOAuthClientRedirectURIs}},
		{"redirect with a fragment", "", "read", []string{"https://app.example/cb#x"}, []error{identity.ErrRedirectURIFragment, identity.ErrInvalidRedirectURI}, []error{identity.ErrInvalidOAuthClientRedirectURIs}},
		{"too many redirects", "", "read", tooMany, []error{identity.ErrTooManyOAuthClientRedirectURIs}, []error{identity.ErrInvalidOAuthClientRedirectURIs}},
		{"ftp website", "ftp://app.example/", "read", uris, []error{identity.ErrOAuthClientWebsiteNotHTTP}, []error{identity.ErrInvalidOAuthClientWebsite}},
		{"unknown scope", "", "read everything", uris, []error{identity.ErrUnknownScope, identity.ErrInvalidScope}, []error{identity.ErrInvalidOAuthClientScope}},
		{"admin scope", "", "read admin", uris, []error{identity.ErrFirstPartyOnlyScope}, []error{identity.ErrInvalidOAuthClientScope}},
		{"supporters scope", "", "write:supporters", uris, []error{identity.ErrFirstPartyOnlyScope}, []error{identity.ErrInvalidOAuthClientScope}},
	} {
		_, err := identity.ValidateOAuthClientSettings("Bot", tc.website, tc.uris, tc.scope)
		for _, want := range append(tc.rule, tc.setting...) {
			if !errors.Is(err, want) {
				t.Errorf("%s: ValidateOAuthClientSettings = %v, want it to wrap %v", tc.name, err, want)
			}
		}
	}
}

// A redirect URI refused on its own says which rule it breaks (#3215).
func TestRedirectURIErrorsNameTheRule(t *testing.T) {
	for _, tc := range []struct {
		uri  string
		want error
	}{
		{"http://localhost:8080/cb", identity.ErrRedirectURIForm},
		{"ftp://x/", identity.ErrRedirectURIForm},
		{"myapp:/cb", identity.ErrRedirectURIForm},
		{"https:///x", identity.ErrRedirectURIForm},
		{"https://app.example/cb#x", identity.ErrRedirectURIFragment},
	} {
		err := identity.ValidateRedirectURI(tc.uri)
		if !errors.Is(err, tc.want) || !errors.Is(err, identity.ErrInvalidRedirectURI) {
			t.Errorf("ValidateRedirectURI(%q) = %v, want %v wrapping ErrInvalidRedirectURI", tc.uri, err, tc.want)
		}
	}
}

// A scope the server does not know is ErrUnknownScope, an ErrInvalidScope (#3215).
func TestParseScopeUnknown(t *testing.T) {
	for _, s := range []string{"read everything", "read:everything"} {
		if _, err := identity.ParseScope(s); !errors.Is(err, identity.ErrUnknownScope) || !errors.Is(err, identity.ErrInvalidScope) {
			t.Errorf("ParseScope(%q) = %v, want ErrUnknownScope wrapping ErrInvalidScope", s, err)
		}
	}
}
