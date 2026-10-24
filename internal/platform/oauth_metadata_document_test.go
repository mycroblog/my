// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/httpclient"
	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
)

// metadataSite serves Client ID Metadata Documents over TLS by path, and counts the fetches.
type metadataSite struct {
	srv  *httptest.Server
	mu   sync.Mutex
	docs map[string]func(w http.ResponseWriter, r *http.Request)
	hits map[string]int
}

func newMetadataSite(t *testing.T) *metadataSite {
	t.Helper()
	m := &metadataSite{docs: map[string]func(http.ResponseWriter, *http.Request){}, hits: map[string]int{}}
	m.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.hits[r.URL.Path]++
		serve := m.docs[r.URL.Path]
		m.mu.Unlock()
		if serve == nil {
			http.NotFound(w, r)
			return
		}
		serve(w, r)
	}))
	t.Cleanup(m.srv.Close)
	return m
}

// url returns the client_id URL of path.
func (m *metadataSite) url(path string) string { return m.srv.URL + path }

// json serves body at path as a JSON document; "{{id}}" in body is the document's URL.
func (m *metadataSite) json(path, body string, header ...string) string {
	id := m.url(path)
	body = strings.ReplaceAll(body, "{{id}}", id)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.docs[path] = func(w http.ResponseWriter, _ *http.Request) {
		for i := 0; i+1 < len(header); i += 2 {
			w.Header().Set(header[i], header[i+1])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
	return id
}

func (m *metadataSite) handle(path string, serve func(http.ResponseWriter, *http.Request)) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.docs[path] = serve
	return m.url(path)
}

func (m *metadataSite) count(path string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hits[path]
}

// client returns an SSRF-guarded HTTPClient that trusts the site's certificate and, as
// http_allow_private_networks does, reaches loopback addresses.
func (m *metadataSite) client() *httpclient.Client {
	pool := x509.NewCertPool()
	pool.AddCert(m.srv.Certificate())
	return httpclient.New(httpclient.Options{RootCAs: pool, AllowPrivateNetworks: true, AnyPort: true})
}

const metadataRedirect = "https://app.example/callback"

// TestOAuthMetadataDocumentClients runs the authorization code flow of clients whose client_id
// is the URL of a Client ID Metadata Document served by an httptest server (ADR-0090 §1): the
// document is fetched through the SSRF-guarded HTTPClient, cached by its Cache-Control, and the
// code exchanged for tokens; redirects, documents over 5 KB, a client_id other than the URL, a
// confidential client, a denied host, and the instance setting off are refused.
func TestOAuthMetadataDocumentClients(t *testing.T) {
	t.Parallel()
	site := newMetadataSite(t)
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), HTTPClient: site.client()})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ctx := t.Context()
	alice, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "alice", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("v", 43)
	authorize := func(clientID string) (identity.Authorization, error) {
		return app.Identity.ValidateAuthorization(ctx, identity.AuthorizationRequest{
			ResponseType: "code", ClientID: clientID, RedirectURI: metadataRedirect, Scope: "read",
			CodeChallenge: domain.S256Challenge(verifier), CodeChallengeMethod: domain.CodeChallengeMethodS256,
		})
	}
	wantRefused := func(clientID, what string) {
		t.Helper()
		if _, err := authorize(clientID); !errors.Is(err, identity.ErrUnknownClient) {
			t.Errorf("ValidateAuthorization(%s) = %v, want ErrUnknownClient", what, err)
		}
	}

	app1 := site.json("/app.json", `{"client_id":"{{id}}","client_name":"Pocket","client_uri":"https://app.example/",
"redirect_uris":["`+metadataRedirect+`"],"token_endpoint_auth_method":"none","grant_types":["authorization_code","refresh_token"]}`,
		"Cache-Control", "max-age=600")
	a, err := authorize(app1)
	if err != nil {
		t.Fatalf("ValidateAuthorization(metadata document) = %v", err)
	}
	if a.Client.Name != "Pocket" || a.Client.Website != "https://app.example/" || a.Client.Registration != domain.ClientRegisteredByMetadataDocument || a.Client.FirstParty() {
		t.Errorf("client = %+v, want Pocket by its metadata document", a.Client)
	}
	code, err := app.Identity.IssueAuthorizationCode(ctx, a, alice.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := app.Identity.ExchangeAuthorizationCode(ctx, identity.CodeExchange{
		ClientCredentials: identity.ClientCredentials{ClientID: app1}, Code: code, RedirectURI: metadataRedirect, CodeVerifier: verifier,
	})
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode = %v", err)
	}
	if tokens.Token.Scope != "read" || tokens.Client.ClientID != app1 {
		t.Errorf("tokens = %+v, want read for the metadata document client", tokens.Token)
	}
	if _, err := app.Identity.AuthenticateAccessToken(ctx, tokens.AccessToken); err != nil {
		t.Errorf("AuthenticateAccessToken = %v", err)
	}
	// The cached document answers the next authorization within its max-age.
	if _, err := authorize(app1); err != nil {
		t.Errorf("ValidateAuthorization(cached) = %v", err)
	}
	if n := site.count("/app.json"); n != 1 {
		t.Errorf("fetches = %d, want 1 (the second authorization uses the cache)", n)
	}

	// Refused documents, none of them stored.
	big := `{"client_id":"{{id}}","redirect_uris":["` + metadataRedirect + `"],"client_name":"` + strings.Repeat("x", 5<<10) + `"}`
	moved := site.handle("/moved.json", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app.json", http.StatusFound)
	})
	for what, id := range map[string]string{
		"a redirect":           moved,
		"a document over 5 KB": site.json("/big.json", big),
		"another client_id":    site.json("/other.json", `{"client_id":"`+app1+`","redirect_uris":["`+metadataRedirect+`"]}`),
		"a client secret":      site.json("/secret.json", `{"client_id":"{{id}}","client_secret":"s3cret","redirect_uris":["`+metadataRedirect+`"]}`),
		"client_secret_basic":  site.json("/basic.json", `{"client_id":"{{id}}","token_endpoint_auth_method":"client_secret_basic","redirect_uris":["`+metadataRedirect+`"]}`),
		"a missing document":   site.url("/missing.json"),
		"an invalid client_id": site.srv.URL + "/",
		"a malformed document": site.json("/broken.json", `{"client_id":`),
	} {
		wantRefused(id, what)
	}
	if n := site.count("/app.json"); n != 1 {
		t.Errorf("fetches of the redirect target = %d, want none beyond the first", n-1)
	}

	// An error is not cached: the next authorization fetches again and succeeds once the
	// document is fixed.
	flaky := site.json("/flaky.json", `{"client_id":"{{id}}","redirect_uris":["`+metadataRedirect+`"],"token_endpoint_auth_method":"private_key_jwt"}`)
	wantRefused(flaky, "a private_key_jwt client")
	// The failed fetch counted against the URL's limit (ADR-0090 §7): wait it out.
	resetFetchLimit(t, app, flaky)
	site.json("/flaky.json", `{"client_id":"{{id}}","redirect_uris":["`+metadataRedirect+`"]}`)
	if _, err := authorize(flaky); err != nil {
		t.Errorf("ValidateAuthorization(fixed document) = %v", err)
	}

	// A document that does not list the requested redirect URI is stored, but the request is
	// refused for its redirect URI.
	elsewhere := site.json("/elsewhere.json", `{"client_id":"{{id}}","redirect_uris":["https://elsewhere.example/cb"]}`)
	if _, err := authorize(elsewhere); !errors.Is(err, identity.ErrInvalidRedirectURI) {
		t.Errorf("ValidateAuthorization(unlisted redirect URI) = %v, want ErrInvalidRedirectURI", err)
	}

	// A no-store document is fetched for every authorization.
	noStore := site.json("/nostore.json", `{"client_id":"{{id}}","redirect_uris":["`+metadataRedirect+`"]}`, "Cache-Control", "no-store")
	for range 2 {
		resetFetchLimit(t, app, noStore)
		if _, err := authorize(noStore); err != nil {
			t.Errorf("ValidateAuthorization(no-store) = %v", err)
		}
	}
	if n := site.count("/nostore.json"); n != 2 {
		t.Errorf("fetches of a no-store document = %d, want 2", n)
	}

	// A denied host is refused before any fetch, and so are its clients' token requests.
	if err := app.Identity.DenyOAuthClientHost(ctx, "127.0.0.1", ""); err != nil {
		t.Fatal(err)
	}
	fresh := site.json("/fresh.json", `{"client_id":"{{id}}","redirect_uris":["`+metadataRedirect+`"]}`)
	wantRefused(fresh, "a denied host")
	wantRefused(app1, "a cached client of a denied host")
	if n := site.count("/fresh.json"); n != 0 {
		t.Errorf("fetches from a denied host = %d, want 0", n)
	}
	if _, err := app.Identity.RefreshAccessToken(ctx, identity.RefreshRequest{
		ClientCredentials: identity.ClientCredentials{ClientID: app1}, RefreshToken: tokens.RefreshToken,
	}); !isOAuthError(err, identity.OAuthInvalidClient) {
		t.Errorf("RefreshAccessToken(denied host) = %v, want invalid_client", err)
	}
	if err := app.Identity.AllowOAuthClientHost(ctx, "127.0.0.1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := authorize(fresh); err != nil {
		t.Errorf("ValidateAuthorization(allowed host) = %v", err)
	}

	// The instance setting turns the clients off.
	if _, err := app.Instance.SetSettings(ctx, []appinstance.SetSettingInput{
		{Key: instance.SettingOAuthMetadataDocumentClients, Value: "false"},
	}); err != nil {
		t.Fatal(err)
	}
	wantRefused(app1, "the setting off")
}

// TestOAuthMetadataDocumentInternalAddress checks that the SSRF guard of the configured
// HTTPClient refuses a client_id URL on an internal address (ADR-0019 §1).
func TestOAuthMetadataDocumentInternalAddress(t *testing.T) {
	t.Parallel()
	site := newMetadataSite(t)
	id := site.json("/app.json", `{"client_id":"{{id}}","redirect_uris":["`+metadataRedirect+`"]}`)
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	_, err = app.Identity.ValidateAuthorization(t.Context(), identity.AuthorizationRequest{
		ResponseType: "code", ClientID: id, RedirectURI: metadataRedirect,
		CodeChallenge: domain.S256Challenge(strings.Repeat("v", 43)), CodeChallengeMethod: domain.CodeChallengeMethodS256,
	})
	if !errors.Is(err, identity.ErrUnknownClient) || !strings.Contains(err.Error(), "forbidden destination") {
		t.Errorf("ValidateAuthorization(loopback client_id) = %v, want ErrUnknownClient for a forbidden destination", err)
	}
	if n := site.count("/app.json"); n != 0 {
		t.Errorf("fetches = %d, want 0", n)
	}
}

func isOAuthError(err error, code string) bool {
	var e *identity.OAuthError
	return errors.As(err, &e) && e.Code == code
}

// resetFetchLimit forgets the fetches counted against the URL limit of clientID, as if the
// limit's window had passed.
func resetFetchLimit(t *testing.T, app *platform.App, clientID string) {
	t.Helper()
	lim := identity.MetadataDocumentURLLimit
	index := time.Now().UnixMilli() / lim.Window.Milliseconds()
	for i := index - 1; i <= index+1; i++ {
		key := fmt.Sprintf("%s:%d:%s", lim.Name, i, clientID)
		if err := app.KV.Delete(t.Context(), kernel.RateLimitNamespace, key); err != nil {
			t.Fatal(err)
		}
	}
}

// TestOAuthMetadataDocumentFetchLimits checks the limits of the fetch of a metadata document
// (ADR-0090 §7): a URL is fetched once in 5 minutes, a host 30 times an hour, and over a limit
// the authorization request fails with a rate-limit error without fetching.
func TestOAuthMetadataDocumentFetchLimits(t *testing.T) {
	t.Parallel()
	site := newMetadataSite(t)
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), HTTPClient: site.client()})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	authorize := func(clientID string) error {
		_, err := app.Identity.ValidateAuthorization(t.Context(), identity.AuthorizationRequest{
			ResponseType: "code", ClientID: clientID, RedirectURI: metadataRedirect, Scope: "read",
			CodeChallenge: domain.S256Challenge(strings.Repeat("v", 43)), CodeChallengeMethod: domain.CodeChallengeMethodS256,
		})
		return err
	}
	wantLimited := func(err error, what string) {
		t.Helper()
		var limited *kernel.RateLimitedError
		if !errors.As(err, &limited) || limited.RetryAfter <= 0 {
			t.Errorf("%s = %v, want a *kernel.RateLimitedError with a RetryAfter", what, err)
		}
	}

	noStore := site.json("/nostore.json", `{"client_id":"{{id}}","redirect_uris":["`+metadataRedirect+`"]}`, "Cache-Control", "no-store")
	if err := authorize(noStore); err != nil {
		t.Fatalf("first authorization = %v", err)
	}
	wantLimited(authorize(noStore), "the second fetch of one URL")
	if n := site.count("/nostore.json"); n != 1 {
		t.Errorf("fetches = %d, want 1: a limited request does not fetch", n)
	}

	// 29 more URLs of the host use up its 30 fetches an hour.
	for i := range 29 {
		id := site.json(fmt.Sprintf("/doc%d.json", i), `{"client_id":"{{id}}","redirect_uris":["`+metadataRedirect+`"]}`)
		if err := authorize(id); err != nil {
			t.Fatalf("fetch %d = %v", i+2, err)
		}
	}
	over := site.json("/over.json", `{"client_id":"{{id}}","redirect_uris":["`+metadataRedirect+`"]}`)
	wantLimited(authorize(over), "the 31st fetch from one host")
	if n := site.count("/over.json"); n != 0 {
		t.Errorf("fetches over the host limit = %d, want 0", n)
	}
	// A cached document needs no fetch, so the limits do not apply to it.
	if err := authorize(site.url("/doc0.json")); err != nil {
		t.Errorf("authorization of a cached document = %v", err)
	}
}
