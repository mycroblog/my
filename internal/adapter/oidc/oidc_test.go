// SPDX-License-Identifier: AGPL-3.0-only

package oidc_test

import (
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/httpclient"
	"github.com/mycroblog/my/internal/adapter/oidc"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestContract(t *testing.T) {
	testkit.OIDCClientContract(t, func(_ *testing.T, h kernel.HTTPClient, clock kernel.Clock) appidentity.OIDCClient {
		return oidc.New(oidc.Options{HTTP: h, Clock: clock})
	})
}

// TestPrivateNetworkIssuer checks that the SSRF-guarded client refuses an issuer on a private
// network unless login_provider_allowed_destinations lists it (ADR-0106 §5).
func TestPrivateNetworkIssuer(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issuer":"` + srv.URL + `","authorization_endpoint":"` + srv.URL + `/authorize",` +
			`"token_endpoint":"` + srv.URL + `/token","jwks_uri":"` + srv.URL + `/jwks",` +
			`"id_token_signing_alg_values_supported":["RS256"]}`))
	}))
	t.Cleanup(srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	clock := testkit.NewClock(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))

	guarded := oidc.New(oidc.Options{HTTP: httpclient.New(httpclient.Options{RootCAs: pool}), Clock: clock})
	if _, err := guarded.Discover(t.Context(), srv.URL); !errors.Is(err, kernel.ErrForbiddenDestination) {
		t.Errorf("Discover(unlisted private issuer) = %v, want ErrForbiddenDestination", err)
	}
	webhookOnly := oidc.New(oidc.Options{HTTP: httpclient.New(httpclient.Options{
		RootCAs: pool, Webhook: httpclient.Destinations{Prefixes: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}},
	}), Clock: clock})
	if _, err := webhookOnly.Discover(t.Context(), srv.URL); !errors.Is(err, kernel.ErrForbiddenDestination) {
		t.Errorf("Discover(issuer listed for webhooks only) = %v, want ErrForbiddenDestination", err)
	}
	allowed := oidc.New(oidc.Options{HTTP: httpclient.New(httpclient.Options{
		RootCAs: pool, LoginProvider: httpclient.Destinations{Prefixes: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}},
	}), Clock: clock})
	p, err := allowed.Discover(t.Context(), srv.URL)
	if err != nil {
		t.Fatalf("Discover(listed private issuer) = %v", err)
	}
	if p.Issuer != srv.URL || p.JWKSURI != srv.URL+"/jwks" {
		t.Errorf("Discover = %+v", p)
	}
}
