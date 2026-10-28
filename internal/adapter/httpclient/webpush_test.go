// SPDX-License-Identifier: AGPL-3.0-only

package httpclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// localhostTLSServer starts an https server on loopback whose certificate names localhost and
// 127.0.0.1, and returns it with a pool that trusts the certificate.
func localhostTLSServer(t *testing.T, h http.Handler) (*httptest.Server, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:        true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	srv := httptest.NewUnstartedServer(h)
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv, pool
}

// TestWebPushDestinations checks push_allowed_destinations (ADR-0118 §3): the listed
// destinations open for the Web Push sends alone, over https, without following a redirect;
// federation fetch and delivery, WebFinger, link previews, the media proxy, webhooks, and
// sign-in providers keep the guard.
func TestWebPushDestinations(t *testing.T) {
	var otherHits atomic.Int32
	other, _ := localhostTLSServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { otherHits.Add(1) }))
	srv, pool := localhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/moved":
			http.Redirect(w, r, "/end", http.StatusFound)
		case "/away":
			http.Redirect(w, r, other.URL+"/end", http.StatusTemporaryRedirect)
		default:
			w.WriteHeader(http.StatusCreated)
		}
	}))
	addr := netip.MustParseAddrPort(srv.Listener.Addr().String())
	port := ":" + strconv.Itoa(int(addr.Port()))
	resolve := func(_ context.Context, host string) ([]netip.Addr, error) {
		if host == "localhost" || host == "push.example.com" {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}
		return nil, errors.New("no such host")
	}
	newClient := func(d Destinations) *Client {
		return New(Options{LookupNetIP: resolve, RootCAs: pool, WebPush: d})
	}
	byName := newClient(Destinations{Hosts: []string{"localhost"}})
	byRange := newClient(Destinations{Prefixes: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
	none := newClient(Destinations{})

	// Every purpose but HTTPPurposeWebPush keeps the guard against the listed destinations.
	guarded := []struct {
		name    string
		purpose kernel.HTTPPurpose
	}{
		{"federation fetch, WebFinger, and delivery", kernel.HTTPPurposeJSON},
		{"link preview", kernel.HTTPPurposeHTML},
		{"media proxy image", kernel.HTTPPurposeImage},
		{"media proxy video", kernel.HTTPPurposeVideo},
		{"human check", kernel.HTTPPurposeHumanCheck},
		{"webhook", kernel.HTTPPurposeWebhook},
		{"client metadata", kernel.HTTPPurposeClientMetadata},
		{"login provider", kernel.HTTPPurposeLoginProvider},
	}
	type tcase struct {
		name    string
		c       *Client
		url     string
		purpose kernel.HTTPPurpose
		ok      bool
	}
	cases := []tcase{
		{"listed name", byName, "https://localhost" + port + "/push/1", kernel.HTTPPurposeWebPush, true},
		{"listed range", byRange, "https://127.0.0.1" + port + "/push/1", kernel.HTTPPurposeWebPush, true},
		{"listed name over http", byName, "http://localhost" + port + "/push/1", kernel.HTTPPurposeWebPush, false},
		{"listed range over http", byRange, "http://127.0.0.1" + port + "/push/1", kernel.HTTPPurposeWebPush, false},
		{"address of a listed name, not listed", byName, "https://127.0.0.1" + port + "/push/1", kernel.HTTPPurposeWebPush, false},
		{"name resolving into the listed range", byRange, "https://push.example.com" + port + "/push/1", kernel.HTTPPurposeWebPush, false},
		{"unlisted internal name", byName, "https://push.internal/push/1", kernel.HTTPPurposeWebPush, false},
		{"no list", none, "https://localhost" + port + "/push/1", kernel.HTTPPurposeWebPush, false},
		{"no list, loopback address", none, "https://127.0.0.1" + port + "/push/1", kernel.HTTPPurposeWebPush, false},
	}
	for _, g := range guarded {
		cases = append(cases,
			tcase{"listed name, " + g.name, byName, "https://localhost" + port + "/", g.purpose, false},
			tcase{"listed range, " + g.name, byRange, "https://127.0.0.1" + port + "/", g.purpose, false},
		)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := tc.c.Do(t.Context(), &kernel.HTTPRequest{Method: http.MethodPost, URL: tc.url, Body: []byte("x"), Purpose: tc.purpose})
			if tc.ok {
				if err != nil {
					t.Fatalf("Do = %v", err)
				}
				_, _ = resp.ReadAll()
				if resp.StatusCode != http.StatusCreated {
					t.Errorf("status = %d, want 201", resp.StatusCode)
				}
				return
			}
			if err == nil {
				_, _ = resp.ReadAll()
				t.Fatalf("Do = status %d, want ErrForbiddenDestination", resp.StatusCode)
			}
			if !errors.Is(err, kernel.ErrForbiddenDestination) {
				t.Errorf("Do = %v, want ErrForbiddenDestination", err)
			}
		})
	}

	// A listed destination's redirect is never followed, within it or out of it: the 3xx is the
	// answer.
	for _, path := range []string{"/moved", "/away"} {
		resp, err := byName.Do(t.Context(), &kernel.HTTPRequest{Method: http.MethodPost, URL: "https://localhost" + port + path, Purpose: kernel.HTTPPurposeWebPush})
		if err != nil {
			t.Fatalf("Do(%s) = %v", path, err)
		}
		_, _ = resp.ReadAll()
		if resp.StatusCode/100 != 3 {
			t.Errorf("Do(%s) status = %d, want the redirect itself", path, resp.StatusCode)
		}
	}
	if n := otherHits.Load(); n != 0 {
		t.Errorf("the redirect target was reached %d times, want 0", n)
	}
}
