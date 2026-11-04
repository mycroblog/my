// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"crypto"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// SignatureScheme names an HTTP signature format (ADR-0006). The values are the ones
// stored in received_activities.signature_scheme and delivery_attempts.signature_scheme.
type SignatureScheme string

// Signature schemes.
const (
	// SchemeCavage is draft-cavage-http-signatures-12 (the Signature header).
	SchemeCavage SignatureScheme = "cavage"
	// SchemeRFC9421 is RFC 9421 HTTP Message Signatures (Signature-Input and Signature).
	SchemeRFC9421 SignatureScheme = "rfc9421"
)

// Signature algorithms, named as in the RFC 9421 registry. A draft-cavage signature made with
// rsa-sha256 is verified as AlgRSAPKCS1SHA256.
const (
	AlgRSAPKCS1SHA256 = "rsa-v1_5-sha256"
	AlgRSAPSSSHA512   = "rsa-pss-sha512"
	AlgEd25519        = "ed25519"
)

// SignedRequest is an inbound request as its signature covers it.
type SignedRequest struct {
	Method string
	// URL is the absolute target URI: the configured public origin with the request's path and
	// query (ADR-0006: X-Forwarded-* headers are not trusted).
	URL *url.URL
	// Header holds the request headers, including Host.
	Header http.Header
	// Body is the request body as read (at most the inbox limit).
	Body []byte
}

// NewSignedRequest returns the SignedRequest for r received on the public origin (the
// configured scheme and host). The Host header is set to the origin's host, so that a
// signature made for another host never verifies here.
func NewSignedRequest(origin *url.URL, r *http.Request, body []byte) *SignedRequest {
	h := r.Header.Clone()
	if h == nil {
		h = http.Header{}
	}
	h.Set("Host", origin.Host)
	return &SignedRequest{
		Method: r.Method,
		URL: &url.URL{
			Scheme:   origin.Scheme,
			Host:     origin.Host,
			Path:     r.URL.Path,
			RawPath:  r.URL.RawPath,
			RawQuery: r.URL.RawQuery,
		},
		Header: h,
		Body:   body,
	}
}

// Signature is a parsed request signature. A Verifier returns it from Parse; the
// SignatureVerifier applies the acceptance policy of ADR-0006 to it.
type Signature struct {
	Scheme SignatureScheme
	// Label is the RFC 9421 signature label; empty for draft-cavage.
	Label string
	// KeyID is the keyId (draft-cavage) or keyid (RFC 9421) parameter.
	KeyID string
	// Algorithm is the declared algorithm, lowercased; empty when the signature declares none.
	Algorithm string
	// Components are the covered components in signing order. For draft-cavage they are the
	// lowercased entries of the headers parameter ("(request-target)", "host", ...). For RFC 9421
	// they are the component identifiers in Structured Field form without the quotes around the
	// name ("@method", "content-digest", `@query-param;name="Pet"`).
	Components []string
	// Created and Expires are the signature's creation and expiry times; zero when absent.
	Created time.Time
	Expires time.Time
	// Digests are the body digests the request declares: the Digest header (RFC 3230) for
	// draft-cavage and the Content-Digest header (RFC 9530) for RFC 9421.
	Digests []Digest
}

// Covers reports whether the signature covers the component c.
func (s *Signature) Covers(c string) bool {
	for _, x := range s.Components {
		if x == c {
			return true
		}
	}
	return false
}

// KeyHost returns the canonical host (CanonicalHost) of the key ID, or "" when the key ID is
// not an absolute URL with a valid host. The inbox uses it for domain checks before resolving
// the key (federation.md §3.1).
func (s *Signature) KeyHost() string {
	u, err := url.Parse(s.KeyID)
	if err != nil || !u.IsAbs() {
		return ""
	}
	h, err := CanonicalHost(u.Hostname())
	if err != nil {
		return ""
	}
	return h
}

// Digest is one declared body digest.
type Digest struct {
	// Algorithm is the lowercased algorithm name, such as "sha-256".
	Algorithm string
	Value     []byte
}

// VerifyOptions select how a Verifier checks one signature.
type VerifyOptions struct {
	// PublicKey is the signer's public key: *rsa.PublicKey or ed25519.PublicKey.
	PublicKey crypto.PublicKey
	// Algorithm is the algorithm to verify with, one of the Alg constants.
	Algorithm string
	// ExcludeQuery builds the draft-cavage (request-target) without the query string.
	ExcludeQuery bool
}

// Verifier parses and cryptographically verifies HTTP signatures (draft-cavage-12 and
// RFC 9421). It builds the signature base and checks the signature, nothing more: which
// components are required, the time window, the digests, and the keys are the
// SignatureVerifier's policy (federation.md §1.1). The adapter lives in internal/adapter/httpsig
// and passes testkit.HTTPSignatureContract.
type Verifier interface {
	// Parse reads the signature of r: RFC 9421 when a Signature-Input header is present (the
	// first signature in it), draft-cavage otherwise. It fails with an error wrapping
	// ErrNoSignature when r carries none, and ErrMalformedSignature when it cannot be parsed.
	Parse(r *SignedRequest) (*Signature, error)
	// Verify checks sig, as Parse returned it for r, with the key and algorithm of opts. For
	// RFC 9421 it also checks a covered Content-Digest against the body. It fails with an
	// error wrapping ErrSignatureMismatch when the signature does not verify.
	Verify(r *SignedRequest, sig *Signature, opts VerifyOptions) error
}

// SigningKey is a private key that signs outbound requests.
type SigningKey struct {
	// KeyID is the key's URI, {actor}#main-key (ADR-0018).
	KeyID string
	// PrivateKey is *rsa.PrivateKey or ed25519.PrivateKey.
	PrivateKey crypto.Signer
}

// SignOptions select how a Signer signs one request.
type SignOptions struct {
	Scheme SignatureScheme
	// Algorithm is one of the Alg constants and must match the key. draft-cavage signatures
	// declare it as rsa-sha256 (AlgRSAPKCS1SHA256) or hs2019 (AlgEd25519).
	Algorithm string
	// Components are the components to cover: header names and "(request-target)" for
	// draft-cavage, component identifiers for RFC 9421. A covered "digest" (draft-cavage) or
	// "content-digest" (RFC 9421) makes the Signer set that header from the body.
	Components []string
	// TTL sets the RFC 9421 expires parameter to created + TTL; 0 omits it.
	TTL time.Duration
	// Label, Tag, and Nonce are optional RFC 9421 parameters, used when a server asks for them
	// with Accept-Signature.
	Label string
	Tag   string
	Nonce string
}

// RequestedSignature is one signature a server asks for in Accept-Signature (RFC 9421 §5).
type RequestedSignature struct {
	Label      string
	Components []string
	KeyID      string
	Algorithm  string
	Tag        string
	Nonce      string
	Created    bool
	Expires    bool
}

// Signer signs outbound requests. Draft-cavage signatures cover the Date header that the
// caller sets; RFC 9421 signatures take created from the system clock. The adapter lives in
// internal/adapter/httpsig and passes testkit.HTTPSignatureContract.
type Signer interface {
	// Sign adds the signature headers, and the digest header the components ask for, to req.
	// It replaces any signature or digest headers req already has.
	Sign(req *kernel.HTTPRequest, key SigningKey, opts SignOptions) error
	// ParseAcceptSignature parses an Accept-Signature field value. It fails with an error
	// wrapping ErrMalformedSignature for a value that is not a valid Accept-Signature.
	ParseAcceptSignature(value string) ([]RequestedSignature, error)
}

var (
	// ErrNoSignature reports a request without signature headers.
	ErrNoSignature = errors.New("no HTTP signature")
	// ErrMalformedSignature reports signature headers that cannot be parsed.
	ErrMalformedSignature = errors.New("malformed HTTP signature")
	// ErrSignatureMismatch reports a signature that does not verify with the given key.
	ErrSignatureMismatch = errors.New("HTTP signature does not verify")
)
