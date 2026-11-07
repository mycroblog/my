// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"crypto/rsa"
	"crypto/x509"
	"embed"
	"encoding/json"
	"encoding/pem"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
)

// The cross-implementation vector of CanonicalizerContract: testdata/jsonld/create-note.json, a
// Create of a public post as the fan-out builds it, and what jsonld.js 8.3.3 (the JSON-LD
// processor of Misskey) makes of it with testdata/jsonld/sign.mjs: the canonical N-Quads of the
// signature options and of the document, and the RsaSignature2017 signature of
// testdata/jsonld/key.pem.
//
//go:embed testdata/jsonld/create-note.json testdata/jsonld/create-note.jsonld-js.json testdata/jsonld/key.pem
var jsonldVector embed.FS

const (
	ldCreator = "https://my.example/ap/actors/01a148a1-365b-79f7-929a-781395cc457f#main-key"
	ldCreated = "2026-10-17T06:53:04Z"
)

// ldVector is the output of sign.mjs.
type ldVector struct {
	OptionsNQuads  string `json:"options_nquads"`
	DocumentNQuads string `json:"document_nquads"`
	SignatureValue string `json:"signatureValue"`
}

// CanonicalizerFactory returns the canonicalizer under test.
type CanonicalizerFactory func(t *testing.T) appfederation.Canonicalizer

// CanonicalizerContract verifies an appfederation.Canonicalizer against jsonld.js, without the
// network: the URDNA2015 N-Quads of a Create of a public post (the Mastodon extension context,
// a language map, a list, numbers, and the three kinds of tag) and of the signature options
// equal those of jsonld.js; SignLinkedData on it makes the RsaSignature2017 signature that
// jsonld.js makes, which VerifyLinkedData accepts, which a change of the content breaks, and
// which signing again reproduces byte for byte; a context that is not bundled is refused, never
// fetched; and concurrent calls get the same N-Quads.
func CanonicalizerContract(t *testing.T, newCanonicalizer CanonicalizerFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, c appfederation.Canonicalizer)
	}{
		{"MatchesJSONLDJS", ldMatchesJSONLDJS},
		{"SignsAsJSONLDJS", ldSignsAsJSONLDJS},
		{"RefusesUnbundledContexts", ldRefusesUnbundledContexts},
		{"CanonicalizesConcurrently", ldCanonicalizesConcurrently},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, newCanonicalizer(t))
		})
	}
}

func ldRead(t *testing.T, name string) []byte {
	t.Helper()
	b, err := jsonldVector.ReadFile("testdata/jsonld/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ldLoad returns the document, the vector, and the key of sign.mjs.
func ldLoad(t *testing.T) ([]byte, ldVector, *rsa.PrivateKey) {
	t.Helper()
	var v ldVector
	if err := json.Unmarshal(ldRead(t, "create-note.jsonld-js.json"), &v); err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(ldRead(t, "key.pem"))
	if block == nil {
		t.Fatal("key.pem holds no PEM block")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	key, ok := k.(*rsa.PrivateKey)
	if !ok {
		t.Fatalf("key.pem holds a %T, want an RSA key", k)
	}
	return ldRead(t, "create-note.json"), v, key
}

func ldDocument(t *testing.T, doc []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(doc, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func ldMatchesJSONLDJS(t *testing.T, c appfederation.Canonicalizer) {
	doc, v, _ := ldLoad(t)
	got, err := c.Canonicalize(ldDocument(t, doc))
	if err != nil {
		t.Fatalf("Canonicalize(document) = %v", err)
	}
	if got != v.DocumentNQuads {
		t.Errorf("Canonicalize(document) =\n%s\nwant\n%s", got, v.DocumentNQuads)
	}
	got, err = c.Canonicalize(map[string]any{"@context": "https://w3id.org/identity/v1", "creator": ldCreator, "created": ldCreated})
	if err != nil {
		t.Fatalf("Canonicalize(options) = %v", err)
	}
	if got != v.OptionsNQuads {
		t.Errorf("Canonicalize(options) =\n%s\nwant\n%s", got, v.OptionsNQuads)
	}
}

func ldSignsAsJSONLDJS(t *testing.T, c appfederation.Canonicalizer) {
	doc, v, key := ldLoad(t)
	at, err := time.Parse(time.RFC3339, ldCreated)
	if err != nil {
		t.Fatal(err)
	}
	signingKey := appfederation.SigningKey{KeyID: ldCreator, PrivateKey: key}
	signed, err := appfederation.SignLinkedData(doc, c, signingKey, at)
	if err != nil {
		t.Fatalf("SignLinkedData = %v", err)
	}
	var out struct {
		Signature map[string]string `json:"signature"`
	}
	if err := json.Unmarshal(signed, &out); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"type": "RsaSignature2017", "creator": ldCreator, "created": ldCreated, "signatureValue": v.SignatureValue}
	for k, w := range want {
		if out.Signature[k] != w {
			t.Errorf("signature.%s = %q, want %q", k, out.Signature[k], w)
		}
	}
	if err := appfederation.VerifyLinkedData(signed, c, &key.PublicKey); err != nil {
		t.Errorf("VerifyLinkedData = %v", err)
	}
	tampered := strings.Replace(string(signed), "こんにちは", "さようなら", 2)
	if tampered == string(signed) {
		t.Fatal("the document has no text to change")
	}
	if err := appfederation.VerifyLinkedData([]byte(tampered), c, &key.PublicKey); !errors.Is(err, appfederation.ErrLinkedDataSignature) {
		t.Errorf("VerifyLinkedData(tampered) = %v, want ErrLinkedDataSignature", err)
	}
	again, err := appfederation.SignLinkedData(signed, c, signingKey, at)
	if err != nil || string(again) != string(signed) {
		t.Errorf("SignLinkedData(signed) = %s, %v; want the same bytes", again, err)
	}
}

func ldRefusesUnbundledContexts(t *testing.T, c appfederation.Canonicalizer) {
	_, err := c.Canonicalize(map[string]any{
		"@context": []any{"https://www.w3.org/ns/activitystreams", "https://evil.example/context.jsonld"},
		"id":       "https://my.example/a", "type": "Note",
	})
	if err == nil {
		t.Error("Canonicalize with an unbundled context succeeded")
	}
}

func ldCanonicalizesConcurrently(t *testing.T, c appfederation.Canonicalizer) {
	doc, v, _ := ldLoad(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			var m map[string]any
			if err := json.Unmarshal(doc, &m); err != nil {
				t.Error(err)
				return
			}
			got, err := c.Canonicalize(m)
			if err != nil || got != v.DocumentNQuads {
				t.Errorf("Canonicalize = %v, or N-Quads that differ", err)
			}
		})
	}
	wg.Wait()
}
