// SPDX-License-Identifier: AGPL-3.0-only

// Package httpsig implements the federation.Signer and federation.Verifier ports: RFC 9421
// HTTP Message Signatures with github.com/dadrus/httpsig and draft-cavage-http-signatures-12
// with code.superseriousbusiness.org/httpsig (ADR-0006). Both libraries stay inside this
// package. It builds signature bases and checks signatures only; the acceptance policy
// (required components, time window, digests, keys) is federation.SignatureVerifier's
// (docs/architecture/federation.md §1.1, ADR-0006).
package httpsig

import (
	"fmt"
	"strings"

	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/kernel"
)

// Adapter signs and verifies HTTP signatures. The zero value is ready to use and safe for
// concurrent use.
type Adapter struct{}

var (
	_ federation.Signer   = Adapter{}
	_ federation.Verifier = Adapter{}
)

// New returns an Adapter.
func New() Adapter { return Adapter{} }

// Parse implements federation.Verifier.
func (Adapter) Parse(r *federation.SignedRequest) (*federation.Signature, error) {
	if len(r.Header.Values(headerSignatureInput)) > 0 {
		return parseRFC9421(r)
	}
	if v := r.Header.Get(headerSignature); v != "" {
		return parseCavage(r, v)
	}
	return nil, federation.ErrNoSignature
}

// Verify implements federation.Verifier.
func (Adapter) Verify(r *federation.SignedRequest, sig *federation.Signature, opts federation.VerifyOptions) error {
	switch sig.Scheme {
	case federation.SchemeCavage:
		return verifyCavage(r, sig, opts)
	case federation.SchemeRFC9421:
		return verifyRFC9421(r, sig, opts)
	}
	return fmt.Errorf("%w: unknown scheme %q", federation.ErrSignatureMismatch, sig.Scheme)
}

// Sign implements federation.Signer.
func (Adapter) Sign(req *kernel.HTTPRequest, key federation.SigningKey, opts federation.SignOptions) error {
	if req.Header == nil {
		return fmt.Errorf("%w: request without headers", kernel.ErrInvalidHTTPRequest)
	}
	for _, h := range []string{headerSignature, headerSignatureInput, headerDigest, headerContentDigest} {
		req.Header.Del(h)
	}
	switch opts.Scheme {
	case federation.SchemeCavage:
		return signCavage(req, key, opts)
	case federation.SchemeRFC9421:
		return signRFC9421(req, key, opts)
	}
	return fmt.Errorf("unknown signature scheme %q", opts.Scheme)
}

// ParseAcceptSignature implements federation.Signer.
func (Adapter) ParseAcceptSignature(value string) ([]federation.RequestedSignature, error) {
	return parseAcceptSignature(value)
}

const (
	headerSignature      = "Signature"
	headerSignatureInput = "Signature-Input"
	headerDigest         = "Digest"
	headerContentDigest  = "Content-Digest"
)

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
