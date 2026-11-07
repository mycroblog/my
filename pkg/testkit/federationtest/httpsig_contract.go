// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

// HTTPSignatureContract verifies a federation.Signer and federation.Verifier pair, together
// with the policy built on them (federation.SignatureVerifier and federation.SignedHTTPClient,
// ADR-0006): the request test vectors of RFC 9421 Appendix B verify; hand-built draft-cavage
// signatures in the forms Mastodon and GoToSocial send verify, also when the signed
// (request-target) lacks the query string; requests that miss a required component, fall
// outside the time window, or carry a tampered body are refused; signatures of both schemes
// round-trip; a rotated key is fetched again once; and outbound requests double-knock between
// the schemes and remember the hosts that want RFC 9421.
func HTTPSignatureContract(t *testing.T, signer federation.Signer, verifier federation.Verifier) {
	t.Helper()
	t.Run("RFC9421Vectors", func(t *testing.T) { testRFC9421Vectors(t, verifier) })
	t.Run("CavageMastodon", func(t *testing.T) { testCavageMastodon(t, verifier) })
	t.Run("Rejections", func(t *testing.T) { testSignatureRejections(t, signer, verifier) })
	t.Run("RoundTrip", func(t *testing.T) { testSignatureRoundTrip(t, signer, verifier) })
	t.Run("KeyRotation", func(t *testing.T) { testKeyRotation(t, signer, verifier) })
	t.Run("DoubleKnocking", func(t *testing.T) { testDoubleKnocking(t, signer, verifier) })
	t.Run("AcceptSignature", func(t *testing.T) { testAcceptSignature(t, signer, verifier) })
}

// RFC 9421 Appendix B.1.2 (test-key-rsa-pss) and B.1.4 (test-key-ed25519) public keys.
const (
	rfc9421RSAPSSPublicKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAr4tmm3r20Wd/PbqvP1s2
+QEtvpuRaV8Yq40gjUR8y2Rjxa6dpG2GXHbPfvMs8ct+Lh1GH45x28Rw3Ry53mm+
oAXjyQ86OnDkZ5N8lYbggD4O3w6M6pAvLkhk95AndTrifbIFPNU8PPMO7OyrFAHq
gDsznjPFmTOtCEcN2Z1FpWgchwuYLPL+Wokqltd11nqqzi+bJ9cvSKADYdUAAN5W
Utzdpiy6LbTgSxP7ociU4Tn0g5I6aDZJ7A8Lzo0KSyZYoA485mqcO0GVAdVw9lq4
aOT9v6d+nb4bnNkQVklLQ3fVAvJm+xdDOp9LCNCN48V2pnDOkFV6+U9nV5oyc6XI
2wIDAQAB
-----END PUBLIC KEY-----`
	rfc9421Ed25519PublicKey = `-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEAJrQLj5P/89iXES9+vFgrIy29clF9CC/oPPsw3c5D0bs=
-----END PUBLIC KEY-----`
)

type rfc9421Vector struct {
	name      string
	key       string
	alg       string
	input     string
	signature string
}

// rfc9421RequestVectors are the request signatures of RFC 9421 Appendix B.2 that use the
// algorithms of ADR-0006 (B.2.4 signs a response with ECDSA and B.2.5 uses HMAC).
var rfc9421RequestVectors = []rfc9421Vector{
	{
		name: "B.2.1 minimal", key: rfc9421RSAPSSPublicKey, alg: federation.AlgRSAPSSSHA512,
		input:     `sig-b21=();created=1618884473;keyid="test-key-rsa-pss";nonce="b3k2pp5k7z-50gnwp.yemd"`,
		signature: `sig-b21=:d2pmTvmbncD3xQm8E9ZV2828BjQWGgiwAaw5bAkgibUopemLJcWDy/lkbbHAve4cRAtx31Iq786U7it++wgGxbtRxf8Udx7zFZsckzXaJMkA7ChG52eSkFxykJeNqsrWH5S+oxNFlD4dzVuwe8DhTSja8xxbR/Z2cOGdCbzR72rgFWhzx2VjBqJzsPLMIQKhO4DGezXehhWwE56YCE+O6c0mKZsfxVrogUvA4HELjVKWmAvtl6UnCh8jYzuVG5WSb/QEVPnP5TmcAnLH1g+s++v6d4s8m0gCw1fV5/SITLq9mhho8K3+7EPYTU8IU1bLhdxO5Nyt8C8ssinQ98Xw9Q==:`,
	},
	{
		name: "B.2.2 selective", key: rfc9421RSAPSSPublicKey, alg: federation.AlgRSAPSSSHA512,
		input:     `sig-b22=("@authority" "content-digest" "@query-param";name="Pet");created=1618884473;keyid="test-key-rsa-pss";tag="header-example"`,
		signature: `sig-b22=:LjbtqUbfmvjj5C5kr1Ugj4PmLYvx9wVjZvD9GsTT4F7GrcQEdJzgI9qHxICagShLRiLMlAJjtq6N4CDfKtjvuJyE5qH7KT8UCMkSowOB4+ECxCmT8rtAmj/0PIXxi0A0nxKyB09RNrCQibbUjsLS/2YyFYXEu4TRJQzRw1rLEuEfY17SARYhpTlaqwZVtR8NV7+4UKkjqpcAoFqWFQh62s7Cl+H2fjBSpqfZUJcsIk4N6wiKYd4je2U/lankenQ99PZfB4jY3I5rSV2DSBVkSFsURIjYErOs0tFTQosMTAoxk//0RoKUqiYY8Bh0aaUEb0rQl3/XaVe4bXTugEjHSw==:`,
	},
	{
		name: "B.2.3 full coverage", key: rfc9421RSAPSSPublicKey, alg: federation.AlgRSAPSSSHA512,
		input:     `sig-b23=("date" "@method" "@path" "@query" "@authority" "content-type" "content-digest" "content-length");created=1618884473;keyid="test-key-rsa-pss"`,
		signature: `sig-b23=:bbN8oArOxYoyylQQUU6QYwrTuaxLwjAC9fbY2F6SVWvh0yBiMIRGOnMYwZ/5MR6fb0Kh1rIRASVxFkeGt683+qRpRRU5p2voTp768ZrCUb38K0fUxN0O0iC59DzYx8DFll5GmydPxSmme9v6ULbMFkl+V5B1TP/yPViV7KsLNmvKiLJH1pFkh/aYA2HXXZzNBXmIkoQoLd7YfW91kE9o/CCoC1xMy7JA1ipwvKvfrs65ldmlu9bpG6A9BmzhuzF8Eim5f8ui9eH8LZH896+QIF61ka39VBrohr9iyMUJpvRX2Zbhl5ZJzSRxpJyoEZAFL2FUo5fTIztsDZKEgM4cUA==:`,
	},
	{
		name: "B.2.6 ed25519", key: rfc9421Ed25519PublicKey, alg: federation.AlgEd25519,
		input:     `sig-b26=("date" "@method" "@path" "@authority" "content-type" "content-length");created=1618884473;keyid="test-key-ed25519"`,
		signature: `sig-b26=:wqcAqbmYJ2ji2glfAMaRy4gruYYnx2nEFN2HN6jrnDnQCK1u02Gb04v9EDgwUPiu4A0w6vuQv5lIp5WPpBKRCw==:`,
	},
}

// rfc9421VectorRequest is the request of RFC 9421 §2.5 that the Appendix B.2 vectors sign.
func rfc9421VectorRequest(v rfc9421Vector) *federation.SignedRequest {
	u, _ := url.Parse("http://example.com/foo?param=Value&Pet=dog")
	return &federation.SignedRequest{
		Method: http.MethodPost,
		URL:    u,
		Header: http.Header{
			"Host":            {"example.com"},
			"Date":            {"Tue, 20 Apr 2021 02:07:55 GMT"},
			"Content-Type":    {"application/json"},
			"Content-Digest":  {"sha-512=:WZDPaVn/7XgHaAy8pmojAkGWoRx2UFChF41A2svX+TaPm+AbwAgBWnrIiYllu7BNNyealdVLvRwEmTHWXvJwew==:"},
			"Content-Length":  {"18"},
			"Signature-Input": {v.input},
			"Signature":       {v.signature},
		},
		Body: []byte(`{"hello": "world"}`),
	}
}

func testRFC9421Vectors(t *testing.T, verifier federation.Verifier) {
	vectorTime := time.Unix(1618884473, 0).UTC()
	for _, v := range rfc9421RequestVectors {
		t.Run(v.name, func(t *testing.T) {
			pub := mustPublicKeyPEM(t, v.key)
			r := rfc9421VectorRequest(v)
			sig, err := verifier.Parse(r)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if sig.Scheme != federation.SchemeRFC9421 || !sig.Created.Equal(vectorTime) || sig.KeyID == "" {
				t.Fatalf("Parse = %+v, want an RFC 9421 signature created at %v with a key ID", sig, vectorTime)
			}
			if err := verifier.Verify(r, sig, federation.VerifyOptions{PublicKey: pub, Algorithm: v.alg}); err != nil {
				t.Fatalf("Verify: %v", err)
			}
			// Without a declared algorithm the policy tries every one the key type allows.
			keys := &staticKeys{keys: map[string]federation.PublicKey{sig.KeyID: {KeyID: sig.KeyID, Owner: "https://example.com/actor", Key: pub}}}
			sv := federation.NewSignatureVerifier(verifier, keys, testkit.NewKeyValueStore(testkit.NewClock(vectorTime)), testkit.NewClock(vectorTime))
			if _, err := verifyPolicy(sv, r, sig); err != nil {
				t.Fatalf("SignatureVerifier.Verify: %v", err)
			}

			tampered := rfc9421VectorRequest(v)
			tampered.Header.Set("Date", "Tue, 20 Apr 2021 02:07:56 GMT")
			tampered.Header.Set("Content-Type", "text/plain")
			tampered.URL.Path = "/bar"
			tampered.URL.RawQuery = "param=Value&Pet=cat"
			tampered.Header.Set("Content-Length", "19")
			if strings.Contains(v.input, "=()") {
				return // B.2.1 covers no component
			}
			sig, err = verifier.Parse(tampered)
			if err != nil {
				t.Fatalf("Parse(tampered): %v", err)
			}
			err = verifier.Verify(tampered, sig, federation.VerifyOptions{PublicKey: pub, Algorithm: v.alg})
			if !errors.Is(err, federation.ErrSignatureMismatch) {
				t.Fatalf("Verify(tampered) = %v, want ErrSignatureMismatch", err)
			}
		})
	}
	t.Run("ContentDigestMismatch", func(t *testing.T) {
		v := rfc9421RequestVectors[1] // B.2.2 covers content-digest
		r := rfc9421VectorRequest(v)
		r.Body = []byte(`{"hello": "World"}`)
		sig, err := verifier.Parse(r)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		err = verifier.Verify(r, sig, federation.VerifyOptions{PublicKey: mustPublicKeyPEM(t, v.key), Algorithm: v.alg})
		if !errors.Is(err, federation.ErrSignatureMismatch) {
			t.Fatalf("Verify = %v, want ErrSignatureMismatch for a body that does not match Content-Digest", err)
		}
	})
	t.Run("WrongKey", func(t *testing.T) {
		v := rfc9421RequestVectors[0]
		r := rfc9421VectorRequest(v)
		sig, err := verifier.Parse(r)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		err = verifier.Verify(r, sig, federation.VerifyOptions{PublicKey: testRSAKey(t).Public(), Algorithm: v.alg})
		if !errors.Is(err, federation.ErrSignatureMismatch) {
			t.Fatalf("Verify = %v, want ErrSignatureMismatch with another key", err)
		}
	})
}

// cavageSigned is a draft-cavage signature built by hand the way Mastodon and GoToSocial
// build theirs.
type cavageSigned struct {
	method, path, query string
	keyID, algorithm    string
	headers             []string
	// signedPath overrides the path and query in the signed (request-target).
	signedPath string
	date       time.Time
	body       []byte
	key        *rsa.PrivateKey
}

func (c cavageSigned) request(t *testing.T) *federation.SignedRequest {
	t.Helper()
	u := &url.URL{Scheme: "https", Host: "social.example", Path: c.path, RawQuery: c.query}
	h := http.Header{}
	h.Set("Host", "social.example")
	h.Set("Date", c.date.UTC().Format(http.TimeFormat))
	if c.body != nil {
		sum := sha256.Sum256(c.body)
		h.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]))
		h.Set("Content-Type", "application/activity+json")
	}
	target := u.RequestURI()
	if c.signedPath != "" {
		target = c.signedPath
	}
	lines := make([]string, 0, len(c.headers))
	for _, name := range c.headers {
		if name == "(request-target)" {
			lines = append(lines, "(request-target): "+strings.ToLower(c.method)+" "+target)
			continue
		}
		lines = append(lines, name+": "+h.Get(name))
	}
	digest := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	s, err := rsa.SignPKCS1v15(nil, c.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	h.Set("Signature", `keyId="`+c.keyID+`",algorithm="`+c.algorithm+`",headers="`+strings.Join(c.headers, " ")+`",signature="`+base64.StdEncoding.EncodeToString(s)+`"`)
	return &federation.SignedRequest{Method: c.method, URL: u, Header: h, Body: c.body}
}

func testCavageMastodon(t *testing.T, verifier federation.Verifier) {
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	key := testRSAKey(t)
	body := []byte(`{"type":"Follow","actor":"https://remote.example/users/bob"}`)
	mastodon := cavageSigned{
		method: http.MethodPost, path: "/ap/inbox",
		keyID: "https://remote.example/users/bob#main-key", algorithm: "rsa-sha256",
		headers: []string{"(request-target)", "host", "date", "digest", "content-type"},
		date:    now.Add(-30 * time.Second), body: body, key: key,
	}
	gotosocial := mastodon
	gotosocial.keyID, gotosocial.algorithm = "https://remote.example/users/bob/main-key", "hs2019"
	gotosocial.headers = []string{"(request-target)", "host", "date", "digest"}
	get := cavageSigned{
		method: http.MethodGet, path: "/ap/objects/0192", query: "page=true",
		keyID: "https://remote.example/actor#main-key", algorithm: "rsa-sha256",
		headers: []string{"(request-target)", "host", "date"}, date: now, key: key,
	}
	withoutQuery := get
	withoutQuery.signedPath = "/ap/objects/0192"

	for name, c := range map[string]cavageSigned{
		"Mastodon POST":               mastodon,
		"GoToSocial POST":             gotosocial,
		"GET with query":              get,
		"GET signed without query":    withoutQuery,
		"Mastodon POST 11 hours old":  withDate(mastodon, now.Add(-11*time.Hour)),
		"Mastodon POST 59 min future": withDate(mastodon, now.Add(59*time.Minute)),
	} {
		t.Run(name, func(t *testing.T) {
			r := c.request(t)
			sv := newPolicy(verifier, now, c.keyID, key.Public())
			sig := mustAccept(t, sv, r)
			if sig.Scheme != federation.SchemeCavage {
				t.Errorf("Scheme = %s, want cavage", sig.Scheme)
			}
			if sig.Owner != "https://remote.example/users/bob" {
				t.Errorf("Owner = %q", sig.Owner)
			}
		})
	}
	t.Run("ParsedFields", func(t *testing.T) {
		sig, err := verifier.Parse(mastodon.request(t))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		want := []string{"(request-target)", "host", "date", "digest", "content-type"}
		if sig.KeyID != mastodon.keyID || sig.Algorithm != "rsa-sha256" || strings.Join(sig.Components, " ") != strings.Join(want, " ") {
			t.Errorf("Parse = %+v", sig)
		}
		if len(sig.Digests) != 1 || sig.Digests[0].Algorithm != "sha-256" {
			t.Errorf("Digests = %+v, want one sha-256", sig.Digests)
		}
	})
}

func withDate(c cavageSigned, d time.Time) cavageSigned {
	c.date = d
	return c
}

func testSignatureRejections(t *testing.T, signer federation.Signer, verifier federation.Verifier) {
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	key := testRSAKey(t)
	body := []byte(`{"type":"Like"}`)
	base := cavageSigned{
		method: http.MethodPost, path: "/ap/inbox",
		keyID: "https://remote.example/users/bob#main-key", algorithm: "rsa-sha256",
		headers: []string{"(request-target)", "host", "date", "digest", "content-type"},
		date:    now, body: body, key: key,
	}
	cases := map[string]struct {
		req    func(t *testing.T) *federation.SignedRequest
		status int
	}{
		"no signature": {func(t *testing.T) *federation.SignedRequest {
			r := base.request(t)
			r.Header.Del("Signature")
			return r
		}, http.StatusUnauthorized},
		"malformed signature": {func(t *testing.T) *federation.SignedRequest {
			r := base.request(t)
			r.Header.Set("Signature", `keyId="https://remote.example/users/bob#main-key",signature="abc`)
			return r
		}, http.StatusBadRequest},
		"cavage without digest": {func(t *testing.T) *federation.SignedRequest {
			c := base
			c.headers = []string{"(request-target)", "host", "date"}
			return c.request(t)
		}, http.StatusUnauthorized},
		"cavage without host": {func(t *testing.T) *federation.SignedRequest {
			c := base
			c.headers = []string{"(request-target)", "date", "digest"}
			return c.request(t)
		}, http.StatusUnauthorized},
		"cavage without (request-target)": {func(t *testing.T) *federation.SignedRequest {
			c := base
			c.headers = []string{"host", "date", "digest"}
			return c.request(t)
		}, http.StatusUnauthorized},
		"cavage without date": {func(t *testing.T) *federation.SignedRequest {
			c := base
			c.headers = []string{"(request-target)", "host", "digest"}
			return c.request(t)
		}, http.StatusUnauthorized},
		"cavage unknown algorithm": {func(t *testing.T) *federation.SignedRequest {
			c := base
			c.algorithm = "rsa-sha1"
			return c.request(t)
		}, http.StatusUnauthorized},
		"date 13 hours old": {func(t *testing.T) *federation.SignedRequest {
			return withDate(base, now.Add(-13*time.Hour)).request(t)
		}, http.StatusUnauthorized},
		"date 2 hours ahead": {func(t *testing.T) *federation.SignedRequest {
			return withDate(base, now.Add(2*time.Hour)).request(t)
		}, http.StatusUnauthorized},
		"tampered body": {func(t *testing.T) *federation.SignedRequest {
			r := base.request(t)
			r.Body = []byte(`{"type":"Delete"}`)
			return r
		}, http.StatusUnauthorized},
		"tampered digest header": {func(t *testing.T) *federation.SignedRequest {
			r := base.request(t)
			r.Body = []byte(`{"type":"Delete"}`)
			sum := sha256.Sum256(r.Body)
			r.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]))
			return r
		}, http.StatusUnauthorized},
		"unknown digest algorithm only": {func(t *testing.T) *federation.SignedRequest {
			r := base.request(t)
			r.Header.Set("Digest", "MD5=AAAA")
			return r
		}, http.StatusUnauthorized},
		"tampered date header": {func(t *testing.T) *federation.SignedRequest {
			r := base.request(t)
			r.Header.Set("Date", now.Add(time.Second).Format(http.TimeFormat))
			return r
		}, http.StatusUnauthorized},
		"signed for another host": {func(t *testing.T) *federation.SignedRequest {
			r := base.request(t)
			r.Header.Set("Host", "other.example")
			return r
		}, http.StatusUnauthorized},
		"rfc9421 without content-digest": {func(t *testing.T) *federation.SignedRequest {
			return signedRFC9421(t, signer, key, now, http.MethodPost, "https://social.example/ap/inbox", body, []string{"@method", "@target-uri"}, 5*time.Minute)
		}, http.StatusUnauthorized},
		"rfc9421 without @method": {func(t *testing.T) *federation.SignedRequest {
			return signedRFC9421(t, signer, key, now, http.MethodPost, "https://social.example/ap/inbox", body, []string{"@target-uri", "content-digest"}, 5*time.Minute)
		}, http.StatusUnauthorized},
		"rfc9421 authority and path without query": {func(t *testing.T) *federation.SignedRequest {
			return signedRFC9421(t, signer, key, now, http.MethodGet, "https://social.example/ap/objects/1?page=2", nil, []string{"@method", "@authority", "@path"}, 5*time.Minute)
		}, http.StatusUnauthorized},
		"rfc9421 tampered body": {func(t *testing.T) *federation.SignedRequest {
			r := signedRFC9421(t, signer, key, now, http.MethodPost, "https://social.example/ap/inbox", body, []string{"@method", "@target-uri", "content-digest"}, 5*time.Minute)
			r.Body = []byte(`{"type":"Delete"}`)
			return r
		}, http.StatusUnauthorized},
		"rfc9421 tampered target": {func(t *testing.T) *federation.SignedRequest {
			r := signedRFC9421(t, signer, key, now, http.MethodPost, "https://social.example/ap/inbox", body, []string{"@method", "@target-uri", "content-digest"}, 5*time.Minute)
			r.URL.Path = "/ap/actors/x/inbox"
			return r
		}, http.StatusUnauthorized},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sv := newPolicy(verifier, now, base.keyID, key.Public())
			err := accept(sv, tc.req(t))
			var se *federation.SignatureError
			if !errors.As(err, &se) || se.Status != tc.status {
				t.Fatalf("accept = %v, want a SignatureError with status %d", err, tc.status)
			}
			if tc.status == http.StatusUnauthorized && !strings.Contains(se.AcceptSignature, `"@method" "@target-uri"`) {
				t.Errorf("AcceptSignature = %q, want the required RFC 9421 components", se.AcceptSignature)
			}
		})
	}

	// RFC 9421 time window: created up to 1 hour ahead, expires (or created + 12 hours) up to
	// 1 hour past. The signer takes created from the system clock, so the policy's clock moves.
	r := signedRFC9421(t, signer, key, time.Now(), http.MethodPost, "https://social.example/ap/inbox", body, []string{"@method", "@target-uri", "content-digest"}, 5*time.Minute)
	for name, tc := range map[string]struct {
		offset time.Duration
		ok     bool
	}{
		"created 2 hours ahead":           {-2 * time.Hour, false},
		"created 59 minutes ahead":        {-59 * time.Minute, true},
		"expired 2 hours ago":             {5*time.Minute + 2*time.Hour, false},
		"expired 50 minutes ago":          {5*time.Minute + 50*time.Minute, true},
		"just signed":                     {0, true},
		"no expires, 13 hours after":      {13*time.Hour + time.Minute, false},
		"no expires, 12.5 hours after":    {12*time.Hour + 30*time.Minute, true},
		"no expires, created 2 hours out": {-2 * time.Hour, false},
	} {
		t.Run(name, func(t *testing.T) {
			req := r
			if strings.HasPrefix(name, "no expires") {
				req = signedRFC9421(t, signer, key, time.Now(), http.MethodPost, "https://social.example/ap/inbox", body, []string{"@method", "@target-uri", "content-digest"}, 0)
			}
			sv := newPolicy(verifier, time.Now().Add(tc.offset), base.keyID, key.Public())
			err := accept(sv, req)
			if tc.ok && err != nil {
				t.Fatalf("accept = %v, want success", err)
			}
			if !tc.ok && !errors.Is(err, federation.ErrSignatureRejected) {
				t.Fatalf("accept = %v, want rejection", err)
			}
		})
	}
}

func testSignatureRoundTrip(t *testing.T, signer federation.Signer, verifier federation.Verifier) {
	rsaKey := testRSAKey(t)
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"type":"Create"}`)
	for _, tc := range []struct {
		name   string
		scheme federation.SignatureScheme
		key    crypto.Signer
		alg    string
		method string
		url    string
		comps  []string
	}{
		{"cavage POST rsa", federation.SchemeCavage, rsaKey, federation.AlgRSAPKCS1SHA256, http.MethodPost, "https://social.example/ap/inbox", []string{"(request-target)", "host", "date", "digest", "content-type"}},
		{"cavage GET rsa with query", federation.SchemeCavage, rsaKey, federation.AlgRSAPKCS1SHA256, http.MethodGet, "https://social.example/ap/objects/1?page=true", []string{"(request-target)", "host", "date"}},
		{"cavage POST ed25519", federation.SchemeCavage, edKey, federation.AlgEd25519, http.MethodPost, "https://social.example/ap/inbox", []string{"(request-target)", "host", "date", "digest"}},
		{"cavage escaped path", federation.SchemeCavage, rsaKey, federation.AlgRSAPKCS1SHA256, http.MethodGet, "https://social.example/tags/%E6%97%A5%E6%9C%AC", []string{"(request-target)", "host", "date"}},
		{"rfc9421 POST rsa", federation.SchemeRFC9421, rsaKey, federation.AlgRSAPKCS1SHA256, http.MethodPost, "https://social.example/ap/inbox", []string{"@method", "@target-uri", "content-digest"}},
		{"rfc9421 GET rsa with query", federation.SchemeRFC9421, rsaKey, federation.AlgRSAPKCS1SHA256, http.MethodGet, "https://social.example/ap/objects/1?page=true", []string{"@method", "@target-uri"}},
		{"rfc9421 POST ed25519", federation.SchemeRFC9421, edKey, federation.AlgEd25519, http.MethodPost, "https://social.example/ap/inbox", []string{"@method", "@target-uri", "content-digest"}},
		{"rfc9421 authority path query", federation.SchemeRFC9421, rsaKey, federation.AlgRSAPKCS1SHA256, http.MethodGet, "https://social.example/ap/objects/1?page=true", []string{"@method", "@authority", "@path", "@query"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := kernel.Normalize(time.Now())
			req := &kernel.HTTPRequest{Method: tc.method, URL: tc.url, Header: http.Header{}, Purpose: kernel.HTTPPurposeJSON}
			req.Header.Set("Date", now.Format(http.TimeFormat))
			if tc.method == http.MethodPost {
				req.Body = body
				req.Header.Set("Content-Type", "application/activity+json")
			}
			keyID := "https://social.example/ap/actors/a#main-key"
			opts := federation.SignOptions{Scheme: tc.scheme, Algorithm: tc.alg, Components: tc.comps}
			if tc.scheme == federation.SchemeRFC9421 {
				opts.TTL = 5 * time.Minute
			}
			if err := signer.Sign(req, federation.SigningKey{KeyID: keyID, PrivateKey: tc.key}, opts); err != nil {
				t.Fatalf("Sign: %v", err)
			}
			if req.Header.Get("Host") != "" {
				t.Errorf("Sign left a Host header in the request")
			}
			switch tc.scheme {
			case federation.SchemeCavage:
				s := req.Header.Get("Signature")
				wantAlg := `algorithm="rsa-sha256"`
				if tc.alg == federation.AlgEd25519 {
					wantAlg = `algorithm="hs2019"`
				}
				if !strings.Contains(s, wantAlg) || !strings.Contains(s, `headers="`+strings.Join(tc.comps, " ")+`"`) || !strings.Contains(s, `keyId="`+keyID+`"`) {
					t.Errorf("Signature = %q", s)
				}
				if tc.method == http.MethodPost && !strings.HasPrefix(req.Header.Get("Digest"), "SHA-256=") {
					t.Errorf("Digest = %q, want SHA-256", req.Header.Get("Digest"))
				}
			case federation.SchemeRFC9421:
				in := req.Header.Get("Signature-Input")
				if !strings.Contains(in, `alg="`+tc.alg+`"`) || !strings.Contains(in, `keyid="`+keyID+`"`) || !strings.Contains(in, "expires=") {
					t.Errorf("Signature-Input = %q", in)
				}
				if tc.method == http.MethodPost && !strings.HasPrefix(req.Header.Get("Content-Digest"), "sha-256=:") {
					t.Errorf("Content-Digest = %q, want sha-256 only", req.Header.Get("Content-Digest"))
				}
			}
			sv := newPolicy(verifier, now, keyID, tc.key.Public())
			got := mustAccept(t, sv, inbound(t, req))
			if got.Scheme != tc.scheme {
				t.Errorf("Scheme = %s, want %s", got.Scheme, tc.scheme)
			}
			if tc.scheme == federation.SchemeRFC9421 {
				sig, err := verifier.Parse(inbound(t, req))
				if err != nil {
					t.Fatal(err)
				}
				if d := sig.Expires.Sub(sig.Created); d != 5*time.Minute {
					t.Errorf("expires - created = %v, want 5m", d)
				}
			}
		})
	}
}

func testKeyRotation(t *testing.T, signer federation.Signer, verifier federation.Verifier) {
	now := kernel.Normalize(time.Now())
	oldKey, newKey := testRSAKey(t), testRSAKey2(t)
	keyID := "https://remote.example/users/bob#main-key"
	req := &kernel.HTTPRequest{Method: http.MethodGet, URL: "https://social.example/ap/objects/1", Header: http.Header{}, Purpose: kernel.HTTPPurposeJSON}
	req.Header.Set("Date", now.Format(http.TimeFormat))
	if err := signer.Sign(req, federation.SigningKey{KeyID: keyID, PrivateKey: newKey}, federation.SignOptions{
		Scheme: federation.SchemeCavage, Algorithm: federation.AlgRSAPKCS1SHA256, Components: []string{"(request-target)", "host", "date"},
	}); err != nil {
		t.Fatal(err)
	}
	clock := testkit.NewClock(now)
	keys := &staticKeys{
		keys:  map[string]federation.PublicKey{keyID: {KeyID: keyID, Owner: "https://remote.example/users/bob", Key: oldKey.Public()}},
		fresh: map[string]federation.PublicKey{keyID: {KeyID: keyID, Owner: "https://remote.example/users/bob", Key: newKey.Public()}},
	}
	sv := federation.NewSignatureVerifier(verifier, keys, testkit.NewKeyValueStore(clock), clock)
	if err := accept(sv, inbound(t, req)); err != nil {
		t.Fatalf("accept with a rotated key = %v, want success after one refresh", err)
	}
	if keys.refreshes != 1 {
		t.Errorf("refreshes = %d, want 1", keys.refreshes)
	}

	// A refresh within KeyRefreshInterval of the last one is not made again.
	keys.fresh[keyID] = federation.PublicKey{KeyID: keyID, Owner: "https://remote.example/users/bob", Key: oldKey.Public()}
	keys.keys[keyID] = keys.fresh[keyID]
	clock.Advance(time.Minute)
	if err := accept(sv, inbound(t, req)); !errors.Is(err, federation.ErrSignatureRejected) {
		t.Fatalf("accept = %v, want rejection", err)
	}
	if keys.refreshes != 1 {
		t.Errorf("refreshes = %d, want still 1 within %v", keys.refreshes, federation.KeyRefreshInterval)
	}
	clock.Advance(federation.KeyRefreshInterval)
	_ = accept(sv, inbound(t, req))
	if keys.refreshes != 2 {
		t.Errorf("refreshes = %d, want 2 after %v", keys.refreshes, federation.KeyRefreshInterval)
	}

	t.Run("UnavailableKey", func(t *testing.T) {
		keys := &staticKeys{err: federation.ErrPublicKeyUnavailable}
		sv := federation.NewSignatureVerifier(verifier, keys, testkit.NewKeyValueStore(clock), clock)
		var se *federation.SignatureError
		if err := accept(sv, inbound(t, req)); !errors.As(err, &se) || se.Status != http.StatusServiceUnavailable {
			t.Fatalf("accept = %v, want status 503", err)
		}
	})
	t.Run("UnknownKey", func(t *testing.T) {
		keys := &staticKeys{}
		sv := federation.NewSignatureVerifier(verifier, keys, testkit.NewKeyValueStore(clock), clock)
		var se *federation.SignatureError
		if err := accept(sv, inbound(t, req)); !errors.As(err, &se) || se.Status != http.StatusUnauthorized {
			t.Fatalf("accept = %v, want status 401", err)
		}
	})
	t.Run("ShortRSAKey", func(t *testing.T) {
		short, err := rsa.GenerateKey(rand.Reader, 1024) //nolint:gosec // the contract checks that keys this short are refused
		if err != nil {
			t.Fatal(err)
		}
		r := &kernel.HTTPRequest{Method: http.MethodGet, URL: "https://social.example/ap/objects/1", Header: http.Header{}, Purpose: kernel.HTTPPurposeJSON}
		r.Header.Set("Date", now.Format(http.TimeFormat))
		if err := signer.Sign(r, federation.SigningKey{KeyID: keyID, PrivateKey: short}, federation.SignOptions{
			Scheme: federation.SchemeCavage, Algorithm: federation.AlgRSAPKCS1SHA256, Components: []string{"(request-target)", "host", "date"},
		}); err != nil {
			t.Fatal(err)
		}
		sv := newPolicy(verifier, now, keyID, short.Public())
		if err := accept(sv, inbound(t, r)); !errors.Is(err, federation.ErrSignatureRejected) {
			t.Fatalf("accept = %v, want rejection of a 1024-bit key", err)
		}
	})
}

// signatureServer is an in-memory kernel.HTTPClient whose handler sees every request.
type signatureServer struct {
	mu       sync.Mutex
	handler  func(r *federation.SignedRequest) (int, http.Header)
	t        *testing.T
	requests []*federation.SignedRequest
}

func (s *signatureServer) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	r := inbound(s.t, req)
	s.mu.Lock()
	s.requests = append(s.requests, r)
	s.mu.Unlock()
	status, h := s.handler(r)
	if h == nil {
		h = http.Header{}
	}
	return &kernel.HTTPResponse{StatusCode: status, Header: h, URL: req.URL, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func (s *signatureServer) schemes() []federation.SignatureScheme {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]federation.SignatureScheme, 0, len(s.requests))
	for _, r := range s.requests {
		if r.Header.Get("Signature-Input") != "" {
			out = append(out, federation.SchemeRFC9421)
		} else {
			out = append(out, federation.SchemeCavage)
		}
	}
	s.requests = nil
	return out
}

func testDoubleKnocking(t *testing.T, signer federation.Signer, verifier federation.Verifier) {
	key := testRSAKey(t)
	keyID := "https://social.example/ap/actors/a#main-key"
	signing := federation.SigningKey{KeyID: keyID, PrivateKey: key}
	post := func(host string) *kernel.HTTPRequest {
		h := http.Header{}
		h.Set("Content-Type", "application/activity+json")
		return &kernel.HTTPRequest{Method: http.MethodPost, URL: "https://" + host + "/inbox", Header: h, Body: []byte(`{"type":"Create"}`), Purpose: kernel.HTTPPurposeJSON}
	}
	// accepts answers 202 to valid signatures of the given schemes and 401 to the others.
	accepts := func(clock *testkit.Clock, schemes ...federation.SignatureScheme) func(r *federation.SignedRequest) (int, http.Header) {
		return func(r *federation.SignedRequest) (int, http.Header) {
			sv := newPolicy(verifier, clock.Now(), keyID, key.Public())
			sig, err := sv.Parse(r)
			if err != nil {
				return http.StatusUnauthorized, nil
			}
			ok := false
			for _, s := range schemes {
				ok = ok || s == sig.Scheme
			}
			if !ok {
				return http.StatusUnauthorized, nil
			}
			if err := sv.Check(r, sig); err != nil {
				return http.StatusUnauthorized, nil
			}
			if _, err := verifyPolicy(sv, r, sig); err != nil {
				return http.StatusUnauthorized, nil
			}
			return http.StatusAccepted, nil
		}
	}
	setup := func(t *testing.T) (*testkit.Clock, *testkit.KeyValueStore, *signatureServer, *federation.SignedHTTPClient) {
		clock := testkit.NewClock(time.Now())
		kv := testkit.NewKeyValueStore(clock)
		srv := &signatureServer{t: t}
		return clock, kv, srv, federation.NewSignedHTTPClient(srv, signer, kv, clock)
	}
	do := func(t *testing.T, c *federation.SignedHTTPClient, req *kernel.HTTPRequest) *federation.SignedResponse {
		t.Helper()
		resp, err := c.Do(context.Background(), req, signing)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		_ = resp.Body.Close()
		return resp
	}
	eq := func(t *testing.T, got []federation.SignatureScheme, want ...federation.SignatureScheme) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("requests = %v, want %v", got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("requests = %v, want %v", got, want)
			}
		}
	}

	t.Run("CavageServer", func(t *testing.T) {
		clock, kv, srv, c := setup(t)
		srv.handler = accepts(clock, federation.SchemeCavage)
		resp := do(t, c, post("cavage.example"))
		if resp.StatusCode != http.StatusAccepted || resp.Scheme != federation.SchemeCavage {
			t.Fatalf("response = %d %s, want 202 cavage", resp.StatusCode, resp.Scheme)
		}
		eq(t, srv.schemes(), federation.SchemeCavage)
		if _, err := kv.Get(context.Background(), federation.SchemeMemoryNamespace, "cavage.example"); !errors.Is(err, kernel.ErrKeyNotFound) {
			t.Errorf("memory = %v, want none for a draft-cavage host", err)
		}
	})
	t.Run("RFC9421Server", func(t *testing.T) {
		clock, kv, srv, c := setup(t)
		srv.handler = accepts(clock, federation.SchemeRFC9421)
		resp := do(t, c, post("rfc9421.example"))
		if resp.StatusCode != http.StatusAccepted || resp.Scheme != federation.SchemeRFC9421 {
			t.Fatalf("response = %d %s, want 202 rfc9421", resp.StatusCode, resp.Scheme)
		}
		eq(t, srv.schemes(), federation.SchemeCavage, federation.SchemeRFC9421)
		if v, err := kv.Get(context.Background(), federation.SchemeMemoryNamespace, "rfc9421.example"); err != nil || string(v) != "rfc9421" {
			t.Fatalf("memory = %q, %v; want rfc9421", v, err)
		}
		// The remembered host gets RFC 9421 first.
		do(t, c, post("rfc9421.example"))
		eq(t, srv.schemes(), federation.SchemeRFC9421)
		// GET follows the same rule.
		get := &kernel.HTTPRequest{Method: http.MethodGet, URL: "https://rfc9421.example/users/x", Header: http.Header{}, Purpose: kernel.HTTPPurposeJSON}
		do(t, c, get)
		eq(t, srv.schemes(), federation.SchemeRFC9421)
		// Other hosts are not affected.
		srv.handler = accepts(clock, federation.SchemeCavage, federation.SchemeRFC9421)
		do(t, c, post("other.example"))
		eq(t, srv.schemes(), federation.SchemeCavage)
		// The memory lasts SchemeMemoryTTL.
		clock.Advance(federation.SchemeMemoryTTL + time.Minute)
		srv.handler = func(*federation.SignedRequest) (int, http.Header) { return http.StatusAccepted, nil }
		do(t, c, post("rfc9421.example"))
		eq(t, srv.schemes(), federation.SchemeCavage)
	})
	t.Run("RememberedHostFallsBack", func(t *testing.T) {
		clock, kv, srv, c := setup(t)
		if err := kv.Set(context.Background(), federation.SchemeMemoryNamespace, "back.example", []byte("rfc9421"), federation.SchemeMemoryTTL); err != nil {
			t.Fatal(err)
		}
		srv.handler = accepts(clock, federation.SchemeCavage)
		resp := do(t, c, post("back.example"))
		if resp.StatusCode != http.StatusAccepted || resp.Scheme != federation.SchemeCavage {
			t.Fatalf("response = %d %s, want 202 cavage", resp.StatusCode, resp.Scheme)
		}
		eq(t, srv.schemes(), federation.SchemeRFC9421, federation.SchemeCavage)
		if _, err := kv.Get(context.Background(), federation.SchemeMemoryNamespace, "back.example"); !errors.Is(err, kernel.ErrKeyNotFound) {
			t.Errorf("memory = %v, want forgotten after draft-cavage succeeded", err)
		}
	})
	t.Run("OnlyOneRetry", func(t *testing.T) {
		_, kv, srv, c := setup(t)
		srv.handler = func(*federation.SignedRequest) (int, http.Header) { return http.StatusBadRequest, nil }
		resp := do(t, c, post("broken.example"))
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		eq(t, srv.schemes(), federation.SchemeCavage, federation.SchemeRFC9421)
		if _, err := kv.Get(context.Background(), federation.SchemeMemoryNamespace, "broken.example"); !errors.Is(err, kernel.ErrKeyNotFound) {
			t.Errorf("memory = %v, want none after a failed retry", err)
		}
	})
	t.Run("NoRetryOnOtherStatus", func(t *testing.T) {
		_, _, srv, c := setup(t)
		for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError, http.StatusAccepted} {
			srv.handler = func(*federation.SignedRequest) (int, http.Header) { return status, nil }
			resp := do(t, c, post("status.example"))
			if resp.StatusCode != status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, status)
			}
			eq(t, srv.schemes(), federation.SchemeCavage)
		}
	})
}

func testAcceptSignature(t *testing.T, signer federation.Signer, verifier federation.Verifier) {
	key := testRSAKey(t)
	keyID := "https://social.example/ap/actors/a#main-key"
	got, err := signer.ParseAcceptSignature(`sig1=("@method" "@target-uri" "content-digest" "content-type");created;expires;tag="app";nonce="n0nce"`)
	if err != nil {
		t.Fatalf("ParseAcceptSignature: %v", err)
	}
	if len(got) != 1 || got[0].Label != "sig1" || strings.Join(got[0].Components, " ") != "@method @target-uri content-digest content-type" ||
		!got[0].Created || !got[0].Expires || got[0].Tag != "app" || got[0].Nonce != "n0nce" {
		t.Fatalf("ParseAcceptSignature = %+v", got)
	}
	if _, err := signer.ParseAcceptSignature(`sig1=(`); !errors.Is(err, federation.ErrMalformedSignature) {
		t.Errorf("ParseAcceptSignature(malformed) = %v, want ErrMalformedSignature", err)
	}

	// A 401 with Accept-Signature gets an RFC 9421 retry that covers what it asks for.
	clock := testkit.NewClock(time.Now())
	srv := &signatureServer{t: t}
	srv.handler = func(r *federation.SignedRequest) (int, http.Header) {
		sv := newPolicy(verifier, clock.Now(), keyID, key.Public())
		sig, err := sv.Parse(r)
		if err == nil && sig.Scheme == federation.SchemeRFC9421 && sig.Covers("content-type") && sig.Label == "want" {
			if err := sv.Check(r, sig); err == nil {
				if _, err := verifyPolicy(sv, r, sig); err == nil {
					return http.StatusAccepted, nil
				}
			}
		}
		h := http.Header{}
		h.Set("Accept-Signature", `want=("@method" "@target-uri" "content-digest" "content-type");created`)
		return http.StatusUnauthorized, h
	}
	c := federation.NewSignedHTTPClient(srv, signer, testkit.NewKeyValueStore(clock), clock)
	h := http.Header{}
	h.Set("Content-Type", "application/activity+json")
	resp, err := c.Do(context.Background(), &kernel.HTTPRequest{Method: http.MethodPost, URL: "https://ask.example/inbox", Header: h, Body: []byte(`{}`), Purpose: kernel.HTTPPurposeJSON}, federation.SigningKey{KeyID: keyID, PrivateKey: key})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 after following Accept-Signature", resp.StatusCode)
	}
}

// accept runs the whole receive policy on r.
func accept(sv *federation.SignatureVerifier, r *federation.SignedRequest) error {
	_, err := acceptSig(sv, r)
	return err
}

func acceptSig(sv *federation.SignatureVerifier, r *federation.SignedRequest) (*federation.VerifiedSignature, error) {
	sig, err := sv.Parse(r)
	if err != nil {
		return nil, err
	}
	if err := sv.Check(r, sig); err != nil {
		return nil, err
	}
	return verifyPolicy(sv, r, sig)
}

// verifyPolicy runs sv.Verify and returns its refusal as the error.
func verifyPolicy(sv *federation.SignatureVerifier, r *federation.SignedRequest, sig *federation.Signature) (*federation.VerifiedSignature, error) {
	v, rejected, err := sv.Verify(context.Background(), r, sig)
	if rejected != nil {
		return nil, rejected
	}
	return v, err
}

func mustAccept(t *testing.T, sv *federation.SignatureVerifier, r *federation.SignedRequest) *federation.VerifiedSignature {
	t.Helper()
	v, err := acceptSig(sv, r)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	return v
}

func newPolicy(verifier federation.Verifier, now time.Time, keyID string, pub crypto.PublicKey) *federation.SignatureVerifier {
	clock := testkit.NewClock(now)
	keys := &staticKeys{keys: map[string]federation.PublicKey{keyID: {KeyID: keyID, Owner: "https://remote.example/users/bob", Key: pub}}}
	return federation.NewSignatureVerifier(verifier, keys, testkit.NewKeyValueStore(clock), clock)
}

// inbound turns a signed outbound request into the SignedRequest its receiver sees.
func inbound(t *testing.T, req *kernel.HTTPRequest) *federation.SignedRequest {
	t.Helper()
	u, err := url.Parse(req.URL)
	if err != nil {
		t.Fatal(err)
	}
	h := req.Header.Clone()
	h.Set("Host", u.Host)
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	return &federation.SignedRequest{Method: method, URL: u, Header: h, Body: bytes.Clone(req.Body)}
}

func signedRFC9421(t *testing.T, signer federation.Signer, key crypto.Signer, now time.Time, method, target string, body []byte, comps []string, ttl time.Duration) *federation.SignedRequest {
	t.Helper()
	req := &kernel.HTTPRequest{Method: method, URL: target, Header: http.Header{}, Body: body, Purpose: kernel.HTTPPurposeJSON}
	req.Header.Set("Date", now.UTC().Format(http.TimeFormat))
	if body != nil {
		req.Header.Set("Content-Type", "application/activity+json")
	}
	if err := signer.Sign(req, federation.SigningKey{KeyID: "https://remote.example/users/bob#main-key", PrivateKey: key}, federation.SignOptions{
		Scheme: federation.SchemeRFC9421, Algorithm: federation.AlgRSAPKCS1SHA256, Components: comps, TTL: ttl,
	}); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	return inbound(t, req)
}

// staticKeys is a federation.PublicKeyResolver over fixed keys. A refresh returns the key in
// fresh when there is one.
type staticKeys struct {
	mu        sync.Mutex
	keys      map[string]federation.PublicKey
	fresh     map[string]federation.PublicKey
	err       error
	refreshes int
}

func (k *staticKeys) ResolvePublicKey(_ context.Context, keyID string, refresh bool) (federation.PublicKey, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.err != nil {
		return federation.PublicKey{}, k.err
	}
	if refresh {
		k.refreshes++
		if f, ok := k.fresh[keyID]; ok {
			k.keys[keyID] = f
		}
	}
	if key, ok := k.keys[keyID]; ok {
		return key, nil
	}
	return federation.PublicKey{}, federation.ErrPublicKeyNotFound
}

var (
	testRSAKeys    [2]*rsa.PrivateKey
	testRSAKeyOnce [2]sync.Once
)

func testRSAKeyN(t *testing.T, n int) *rsa.PrivateKey {
	t.Helper()
	testRSAKeyOnce[n].Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		testRSAKeys[n] = k
	})
	return testRSAKeys[n]
}

func testRSAKey(t *testing.T) *rsa.PrivateKey  { return testRSAKeyN(t, 0) }
func testRSAKey2(t *testing.T) *rsa.PrivateKey { return testRSAKeyN(t, 1) }

func mustPublicKeyPEM(t *testing.T, s string) crypto.PublicKey {
	t.Helper()
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		t.Fatal("no PEM block")
	}
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return k
}
