// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
)

// metadataOnly serves the authorization server metadata of a Mastodon server, or doc.
type metadataOnly struct{ doc string }

func (m metadataOnly) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	if req.URL != "https://masto.example/.well-known/oauth-authorization-server" || req.Purpose != kernel.HTTPPurposeLoginProvider {
		return nil, errors.New("unexpected request " + req.URL)
	}
	return &kernel.HTTPResponse{StatusCode: http.StatusOK, URL: req.URL, Body: io.NopCloser(bytes.NewReader([]byte(m.doc)))}, nil
}

const mastodonMetadata = `{"issuer":"https://masto.example/","authorization_endpoint":"https://masto.example/oauth/authorize",
"token_endpoint":"https://masto.example/oauth/token","scopes_supported":["read","profile"]}`

func TestMastodonAuthorizationURLForceLogin(t *testing.T) {
	s := NewService(Deps{HTTP: metadataOnly{mastodonMetadata}, BaseURL: "https://my.example"})
	p := identity.LoginProvider{ID: "p1", Settings: identity.LoginProviderSettings{
		Kind: identity.LoginProviderMastodon, Issuer: "https://masto.example", ClientID: "c",
	}}
	for _, force := range []bool{false, true} {
		raw, err := s.mastodonAuthorizationURL(t.Context(), p, "st", "verifier", force)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(raw)
		q := u.Query()
		if q.Has("force_login") != force || force && q.Get("force_login") != "true" || q.Get("scope") != MastodonScope ||
			q.Get("redirect_uri") != "https://my.example/auth/external/p1/callback" {
			t.Errorf("force %v: %s", force, raw)
		}
	}
}

func TestMastodonDiscoverRefuses(t *testing.T) {
	for name, doc := range map[string]string{
		"another issuer": `{"issuer":"https://other.example/","authorization_endpoint":"https://masto.example/oauth/authorize",
"token_endpoint":"https://masto.example/oauth/token","scopes_supported":["profile"]}`,
		"endpoint elsewhere": `{"issuer":"https://masto.example/","authorization_endpoint":"https://masto.example/oauth/authorize",
"token_endpoint":"https://evil.example/oauth/token","scopes_supported":["profile"]}`,
		"no profile scope": `{"issuer":"https://masto.example/","authorization_endpoint":"https://masto.example/oauth/authorize",
"token_endpoint":"https://masto.example/oauth/token","scopes_supported":["read"]}`,
		"not JSON": `<html>`,
	} {
		s := NewService(Deps{HTTP: metadataOnly{doc}})
		if _, err := s.mastodonDiscover(t.Context(), "https://masto.example"); !errors.Is(err, ErrMastodonServer) {
			t.Errorf("%s: %v, want ErrMastodonServer", name, err)
		}
	}
}
