// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/big"
	"net/http"
	"sync"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
)

// FakeOIDCIssuer is the issuer identifier of the provider that OIDCClientContract fakes.
const FakeOIDCIssuer = "https://idp.example"

// FakeOIDCProvider is an OpenID Connect provider served as a kernel.HTTPClient: its Discovery
// document at FakeOIDCIssuer, its JWKS, and ID tokens signed with its keys. It answers only
// requests of kernel.HTTPPurposeLoginProvider and counts the fetches of each URL. It is safe
// for concurrent use.
type FakeOIDCProvider struct {
	mu        sync.Mutex
	discovery map[string]any
	keys      map[string]FakeOIDCKey
	cache     string
	fetches   map[string]int
	refuse    bool
}

// FakeOIDCKey is a signing key of a FakeOIDCProvider.
type FakeOIDCKey struct {
	Alg    string
	Signer crypto.Signer
}

// NewFakeOIDCProvider returns a provider with an RS256, an ES256, and an EdDSA key (key IDs
// "rsa", "ec", and "ed"), whose responses may be reused for an hour.
func NewFakeOIDCProvider(t *testing.T) *FakeOIDCProvider {
	t.Helper()
	p := &FakeOIDCProvider{
		discovery: map[string]any{
			"issuer":                                FakeOIDCIssuer,
			"authorization_endpoint":                FakeOIDCIssuer + "/authorize",
			"token_endpoint":                        FakeOIDCIssuer + "/token",
			"userinfo_endpoint":                     FakeOIDCIssuer + "/userinfo",
			"jwks_uri":                              FakeOIDCIssuer + "/jwks",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256", "ES256", "EdDSA", "HS256", "none"},
			"code_challenge_methods_supported":      []string{"S256"},
		},
		keys:    map[string]FakeOIDCKey{},
		cache:   "max-age=3600",
		fetches: map[string]int{},
	}
	p.AddKey("rsa", "RS256", NewFakeOIDCSigner(t, "RS256"))
	p.AddKey("ec", "ES256", NewFakeOIDCSigner(t, "ES256"))
	p.AddKey("ed", "EdDSA", NewFakeOIDCSigner(t, "EdDSA"))
	return p
}

// NewFakeOIDCSigner returns a new key for alg (RS256, ES256, or EdDSA).
func NewFakeOIDCSigner(t *testing.T, alg string) crypto.Signer {
	t.Helper()
	var (
		s   crypto.Signer
		err error
	)
	switch alg {
	case "RS256":
		s, err = rsa.GenerateKey(rand.Reader, 2048)
	case "ES256":
		s, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "EdDSA":
		_, s, err = ed25519.GenerateKey(rand.Reader)
	default:
		t.Fatalf("NewFakeOIDCSigner: unknown alg %q", alg)
	}
	if err != nil {
		t.Fatalf("NewFakeOIDCSigner(%s): %v", alg, err)
	}
	return s
}

// AddKey publishes a key in the JWKS.
func (p *FakeOIDCProvider) AddKey(kid, alg string, s crypto.Signer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys[kid] = FakeOIDCKey{Alg: alg, Signer: s}
}

// SetDiscovery replaces one member of the Discovery document.
func (p *FakeOIDCProvider) SetDiscovery(name string, v any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.discovery[name] = v
}

// SetCacheControl sets the Cache-Control of the responses; empty sends none.
func (p *FakeOIDCProvider) SetCacheControl(v string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = v
}

// Refuse makes every request fail as a destination the HTTP client refuses.
func (p *FakeOIDCProvider) Refuse() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refuse = true
}

// Fetches returns how many times url was fetched.
func (p *FakeOIDCProvider) Fetches(url string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fetches[url]
}

// Do implements kernel.HTTPClient.
func (p *FakeOIDCProvider) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if req.Purpose != kernel.HTTPPurposeLoginProvider {
		return nil, fmt.Errorf("%w: purpose %d", kernel.ErrInvalidHTTPRequest, req.Purpose)
	}
	if p.refuse {
		return nil, fmt.Errorf("%w: %s", kernel.ErrForbiddenDestination, req.URL)
	}
	p.fetches[req.URL]++
	var body any
	switch req.URL {
	case FakeOIDCIssuer + "/.well-known/openid-configuration":
		body = maps.Clone(p.discovery)
	case FakeOIDCIssuer + "/jwks":
		var keys []map[string]string
		for kid, k := range p.keys {
			keys = append(keys, fakeJWK(kid, k))
		}
		body = map[string]any{"keys": keys}
	default:
		return &kernel.HTTPResponse{StatusCode: http.StatusNotFound, URL: req.URL, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	h := http.Header{"Content-Type": {"application/json"}}
	if p.cache != "" {
		h.Set("Cache-Control", p.cache)
	}
	return &kernel.HTTPResponse{StatusCode: http.StatusOK, URL: req.URL, Header: h, Body: io.NopCloser(bytes.NewReader(b))}, nil
}

func fakeJWK(kid string, k FakeOIDCKey) map[string]string {
	b64 := base64.RawURLEncoding.EncodeToString
	m := map[string]string{"kid": kid, "alg": k.Alg, "use": "sig"}
	switch pub := k.Signer.Public().(type) {
	case *rsa.PublicKey:
		m["kty"], m["n"], m["e"] = "RSA", b64(pub.N.Bytes()), b64(big.NewInt(int64(pub.E)).Bytes())
	case *ecdsa.PublicKey:
		raw, _ := pub.Bytes() // 0x04 || X || Y
		m["kty"], m["crv"], m["x"], m["y"] = "EC", "P-256", b64(raw[1:33]), b64(raw[33:])
	case ed25519.PublicKey:
		m["kty"], m["crv"], m["x"] = "OKP", "Ed25519", b64(pub)
	}
	return m
}

// SignFakeIDToken returns a compact JWS of claims signed with s for alg (RS256, ES256, EdDSA, or
// none with a nil s), with kid in its header unless empty.
func SignFakeIDToken(t *testing.T, alg, kid string, s crypto.Signer, claims map[string]any) string {
	t.Helper()
	input := fakeJWSInput(t, alg, kid, claims)
	var sig []byte
	h := sha256.Sum256([]byte(input))
	var err error
	switch alg {
	case "none":
	case "RS256":
		sig, err = rsa.SignPKCS1v15(rand.Reader, s.(*rsa.PrivateKey), crypto.SHA256, h[:])
	case "ES256":
		var r, ss *big.Int
		r, ss, err = ecdsa.Sign(rand.Reader, s.(*ecdsa.PrivateKey), h[:])
		if err == nil {
			sig = append(r.FillBytes(make([]byte, 32)), ss.FillBytes(make([]byte, 32))...)
		}
	case "EdDSA":
		sig = ed25519.Sign(s.(ed25519.PrivateKey), []byte(input))
	default:
		t.Fatalf("SignFakeIDToken: unknown alg %q", alg)
	}
	if err != nil {
		t.Fatalf("SignFakeIDToken(%s): %v", alg, err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// signFakeHMACToken returns a compact JWS of claims with HS256 and secret as the key.
func signFakeHMACToken(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	input := fakeJWSInput(t, "HS256", "", claims)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(input))
	return input + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func fakeJWSInput(t *testing.T, alg, kid string, claims map[string]any) string {
	t.Helper()
	h := map[string]string{"alg": alg, "typ": "JWT"}
	if kid != "" {
		h["kid"] = kid
	}
	hb, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)
}

// OIDCClientContract checks an identity.OIDCClient against a FakeOIDCProvider (ADR-0106 §1, §5).
// newClient returns the client under test sending its requests through http and telling the
// time with clock.
func OIDCClientContract(t *testing.T, newClient func(t *testing.T, http kernel.HTTPClient, clock kernel.Clock) appidentity.OIDCClient) {
	const (
		clientID = "my-client"
		nonce    = "n-0S6_WzA2Mj"
	)
	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	setup := func(t *testing.T) (*FakeOIDCProvider, *Clock, appidentity.OIDCClient) {
		t.Helper()
		idp := NewFakeOIDCProvider(t)
		clock := NewClock(start)
		return idp, clock, newClient(t, idp, clock)
	}
	claimsAt := func(now time.Time) map[string]any {
		return map[string]any{
			"iss": FakeOIDCIssuer, "sub": "248289761001", "aud": clientID, "nonce": nonce,
			"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "auth_time": now.Add(-time.Second).Unix(),
			"email": "alice@example.org", "email_verified": true, "hd": "example.org",
		}
	}
	with := func(c map[string]any, kv ...any) map[string]any {
		c = maps.Clone(c)
		for i := 0; i < len(kv); i += 2 {
			if kv[i+1] == nil {
				delete(c, kv[i].(string))
				continue
			}
			c[kv[i].(string)] = kv[i+1]
		}
		return c
	}
	check := appidentity.IDTokenCheck{ClientID: clientID, Nonce: nonce}
	discover := func(t *testing.T, c appidentity.OIDCClient) appidentity.OIDCProvider {
		t.Helper()
		p, err := c.Discover(t.Context(), FakeOIDCIssuer)
		if err != nil {
			t.Fatalf("Discover = %v", err)
		}
		return p
	}

	t.Run("Discover", func(t *testing.T) {
		idp, clock, c := setup(t)
		p := discover(t, c)
		want := appidentity.OIDCProvider{
			Issuer: FakeOIDCIssuer, AuthorizationEndpoint: FakeOIDCIssuer + "/authorize", TokenEndpoint: FakeOIDCIssuer + "/token",
			UserinfoEndpoint: FakeOIDCIssuer + "/userinfo", JWKSURI: FakeOIDCIssuer + "/jwks",
			SigningAlgs: []string{"RS256", "ES256", "EdDSA"}, CodeChallengeMethods: []string{"S256"},
		}
		if fmt.Sprint(p) != fmt.Sprint(want) {
			t.Errorf("Discover = %+v, want %+v", p, want)
		}
		const url = FakeOIDCIssuer + "/.well-known/openid-configuration"
		discover(t, c)
		if n := idp.Fetches(url); n != 1 {
			t.Errorf("fetches within max-age = %d, want 1", n)
		}
		clock.Advance(time.Hour)
		discover(t, c)
		if n := idp.Fetches(url); n != 2 {
			t.Errorf("fetches after max-age = %d, want 2", n)
		}
		idp.SetCacheControl("no-store")
		clock.Advance(time.Hour)
		discover(t, c)
		discover(t, c)
		if n := idp.Fetches(url); n != 4 {
			t.Errorf("fetches with no-store = %d, want 4", n)
		}
	})

	t.Run("Discover refuses", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			name2 string
			v     any
		}{
			{"another issuer", "issuer", "https://evil.example"},
			{"no token endpoint", "token_endpoint", ""},
			{"a JWKS that is not a URL", "jwks_uri", "jwks"},
			{"only HMAC and none", "id_token_signing_alg_values_supported", []string{"HS256", "none"}},
		} {
			idp, _, c := setup(t)
			idp.SetDiscovery(tc.name2, tc.v)
			if _, err := c.Discover(t.Context(), FakeOIDCIssuer); !errors.Is(err, appidentity.ErrOIDCDiscovery) {
				t.Errorf("%s: Discover = %v, want ErrOIDCDiscovery", tc.name, err)
			}
		}
		_, _, c := setup(t)
		if _, err := c.Discover(t.Context(), "https://idp.example/other"); !errors.Is(err, appidentity.ErrOIDCDiscovery) {
			t.Errorf("Discover(not found) = %v, want ErrOIDCDiscovery", err)
		}
		idp, _, c := setup(t)
		idp.Refuse()
		if _, err := c.Discover(t.Context(), FakeOIDCIssuer); !errors.Is(err, kernel.ErrForbiddenDestination) {
			t.Errorf("Discover(refused destination) = %v, want ErrForbiddenDestination", err)
		}
	})

	t.Run("valid tokens", func(t *testing.T) {
		idp, clock, c := setup(t)
		p := discover(t, c)
		for _, kid := range []string{"rsa", "ec", "ed"} {
			k := idp.keys[kid]
			raw := SignFakeIDToken(t, k.Alg, kid, k.Signer, claimsAt(clock.Now()))
			tok, err := c.VerifyIDToken(t.Context(), p, raw, check)
			if err != nil {
				t.Errorf("%s: VerifyIDToken = %v", k.Alg, err)
				continue
			}
			if tok.Subject != "248289761001" || tok.Issuer != FakeOIDCIssuer || tok.Nonce != nonce ||
				tok.Email != "alice@example.org" || !tok.EmailVerified || string(tok.Claims["hd"]) != `"example.org"` ||
				!tok.Expiry.Equal(clock.Now().Add(5*time.Minute)) || tok.AuthTime.IsZero() {
				t.Errorf("%s: VerifyIDToken = %+v", k.Alg, tok)
			}
		}
		// Without a key ID, every key for the algorithm is tried.
		k := idp.keys["ed"]
		if _, err := c.VerifyIDToken(t.Context(), p, SignFakeIDToken(t, "EdDSA", "", k.Signer, claimsAt(clock.Now())), check); err != nil {
			t.Errorf("no kid: VerifyIDToken = %v", err)
		}
		// Several audiences with the client as the authorized party.
		multi := with(claimsAt(clock.Now()), "aud", []string{clientID, "other"}, "azp", clientID)
		if _, err := c.VerifyIDToken(t.Context(), p, SignFakeIDToken(t, "RS256", "rsa", idp.keys["rsa"].Signer, multi), check); err != nil {
			t.Errorf("several audiences: VerifyIDToken = %v", err)
		}
		// A re-authentication after the request.
		reauth := check
		reauth.AuthenticatedAfter = clock.Now().Add(-time.Minute)
		if _, err := c.VerifyIDToken(t.Context(), p, SignFakeIDToken(t, "RS256", "rsa", idp.keys["rsa"].Signer, claimsAt(clock.Now())), reauth); err != nil {
			t.Errorf("auth_time: VerifyIDToken = %v", err)
		}
	})

	t.Run("invalid tokens", func(t *testing.T) {
		idp, clock, c := setup(t)
		p := discover(t, c)
		now := clock.Now()
		rsaKey := idp.keys["rsa"].Signer
		good := claimsAt(now)
		sign := func(claims map[string]any) string { return SignFakeIDToken(t, "RS256", "rsa", rsaKey, claims) }
		stranger := NewFakeOIDCSigner(t, "RS256")
		tampered := []byte(sign(good))
		tampered[len(tampered)-5] ^= 1
		reauth := check
		reauth.AuthenticatedAfter = now.Add(10 * time.Minute)
		for _, tc := range []struct {
			name  string
			raw   string
			check appidentity.IDTokenCheck
		}{
			{"wrong iss", sign(with(good, "iss", "https://evil.example")), check},
			{"wrong aud", sign(with(good, "aud", "another-client")), check},
			{"several audiences without azp", sign(with(good, "aud", []string{clientID, "other"})), check},
			{"azp of another client", sign(with(good, "azp", "another-client")), check},
			{"wrong nonce", sign(with(good, "nonce", "replayed")), check},
			{"no nonce", sign(with(good, "nonce", nil)), check},
			{"no sub", sign(with(good, "sub", nil)), check},
			{"expired", sign(with(good, "exp", now.Add(-2*time.Minute).Unix())), check},
			{"no exp", sign(with(good, "exp", nil)), check},
			{"no iat", sign(with(good, "iat", nil)), check},
			{"issued in the future", sign(with(good, "iat", now.Add(10*time.Minute).Unix())), check},
			{"auth_time before the re-authentication", sign(good), reauth},
			{"no auth_time for a re-authentication", sign(with(good, "auth_time", nil)), reauth},
			{"alg none", SignFakeIDToken(t, "none", "", nil, good), check},
			{"HMAC with the client secret", signFakeHMACToken(t, "client-secret", good), check},
			{"unknown key", SignFakeIDToken(t, "RS256", "stranger", stranger, good), check},
			{"known kid, other key", SignFakeIDToken(t, "RS256", "rsa", stranger, good), check},
			{"alg of another key", SignFakeIDToken(t, "ES256", "rsa", idp.keys["ec"].Signer, good), check},
			{"tampered signature", string(tampered), check},
			{"not a JWS", "not.a.token.at.all", check},
		} {
			if _, err := c.VerifyIDToken(t.Context(), p, tc.raw, tc.check); !errors.Is(err, appidentity.ErrInvalidIDToken) {
				t.Errorf("%s: VerifyIDToken = %v, want ErrInvalidIDToken", tc.name, err)
			}
		}
		// An algorithm the provider does not list.
		rsaOnly := p
		rsaOnly.SigningAlgs = []string{"RS256"}
		ed := idp.keys["ed"]
		if _, err := c.VerifyIDToken(t.Context(), rsaOnly, SignFakeIDToken(t, "EdDSA", "ed", ed.Signer, good), check); !errors.Is(err, appidentity.ErrInvalidIDToken) {
			t.Errorf("unlisted alg: VerifyIDToken = %v, want ErrInvalidIDToken", err)
		}
	})

	t.Run("unknown key ID refetches the JWKS once, spaced", func(t *testing.T) {
		idp, clock, c := setup(t)
		p := discover(t, c)
		const jwks = FakeOIDCIssuer + "/jwks"
		rsaKey := idp.keys["rsa"].Signer
		if _, err := c.VerifyIDToken(t.Context(), p, SignFakeIDToken(t, "RS256", "rsa", rsaKey, claimsAt(clock.Now())), check); err != nil {
			t.Fatalf("VerifyIDToken = %v", err)
		}
		if n := idp.Fetches(jwks); n != 1 {
			t.Fatalf("JWKS fetches = %d, want 1", n)
		}
		rotated := NewFakeOIDCSigner(t, "RS256")
		idp.AddKey("rotated", "RS256", rotated)
		// Right after a fetch, an unknown key ID does not fetch again.
		if _, err := c.VerifyIDToken(t.Context(), p, SignFakeIDToken(t, "RS256", "rotated", rotated, claimsAt(clock.Now())), check); !errors.Is(err, appidentity.ErrInvalidIDToken) {
			t.Errorf("VerifyIDToken(rotated, soon) = %v, want ErrInvalidIDToken", err)
		}
		if n := idp.Fetches(jwks); n != 1 {
			t.Errorf("JWKS fetches right after a fetch = %d, want 1", n)
		}
		clock.Advance(appidentity.OIDCJWKSRefetchInterval)
		if _, err := c.VerifyIDToken(t.Context(), p, SignFakeIDToken(t, "RS256", "rotated", rotated, claimsAt(clock.Now())), check); err != nil {
			t.Errorf("VerifyIDToken(rotated, later) = %v", err)
		}
		if n := idp.Fetches(jwks); n != 2 {
			t.Errorf("JWKS fetches for a rotated key = %d, want 2", n)
		}
		// A key that is still unknown after the refetch fails, with one fetch at most.
		clock.Advance(appidentity.OIDCJWKSRefetchInterval)
		stranger := NewFakeOIDCSigner(t, "RS256")
		if _, err := c.VerifyIDToken(t.Context(), p, SignFakeIDToken(t, "RS256", "stranger", stranger, claimsAt(clock.Now())), check); !errors.Is(err, appidentity.ErrInvalidIDToken) {
			t.Errorf("VerifyIDToken(stranger) = %v, want ErrInvalidIDToken", err)
		}
		if n := idp.Fetches(jwks); n != 3 {
			t.Errorf("JWKS fetches for an unknown key = %d, want 3", n)
		}
		// The known keys come from the cache.
		if _, err := c.VerifyIDToken(t.Context(), p, SignFakeIDToken(t, "RS256", "rsa", rsaKey, claimsAt(clock.Now())), check); err != nil {
			t.Errorf("VerifyIDToken = %v", err)
		}
		if n := idp.Fetches(jwks); n != 3 {
			t.Errorf("JWKS fetches for a cached key = %d, want 3", n)
		}
	})
}
