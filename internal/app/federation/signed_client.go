// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"crypto/ed25519"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// The send profile of ADR-0006.
const (
	// RFC9421TTL is the lifetime of an outbound RFC 9421 signature (expires = created + 5 min).
	RFC9421TTL = 5 * time.Minute
	// SchemeMemoryTTL is how long a host is remembered to want RFC 9421 first.
	SchemeMemoryTTL = 7 * 24 * time.Hour
	// SchemeMemoryNamespace is the KeyValueStore namespace of that memory, keyed by host.
	SchemeMemoryNamespace = "federation.signature_scheme"
)

// Covered components of outbound signatures (ADR-0006).
var (
	cavageGET      = []string{"(request-target)", "host", "date"}
	cavagePOST     = []string{"(request-target)", "host", "date", "digest", "content-type"}
	rfc9421GET     = []string{"@method", "@target-uri"}
	rfc9421POST    = []string{"@method", "@target-uri", "content-digest"}
	errNoAlgorithm = errors.New("unsupported signing key type")
)

// SignedHTTPClient sends signed requests (federation.md §2, ADR-0006). It signs with
// draft-cavage-12 first; when the server answers 400 or 401, it sends the request once more
// signed with RFC 9421 (following the server's Accept-Signature when a 401 carries one). A host
// where that retry succeeded is remembered in the KeyValueStore for SchemeMemoryTTL and gets
// RFC 9421 first, falling back to draft-cavage once in the same way.
type SignedHTTPClient struct {
	http   kernel.HTTPClient
	signer Signer
	kv     kernel.KeyValueStore
	clock  kernel.Clock
}

// NewSignedHTTPClient returns a SignedHTTPClient.
func NewSignedHTTPClient(c kernel.HTTPClient, s Signer, kv kernel.KeyValueStore, clock kernel.Clock) *SignedHTTPClient {
	return &SignedHTTPClient{http: c, signer: s, kv: kv, clock: clock}
}

// SignedResponse is the final response of a signed exchange.
type SignedResponse struct {
	*kernel.HTTPResponse
	// Scheme is the signature scheme of the request that got this response.
	Scheme SignatureScheme
}

// Do signs req with key and sends it. req is not modified; a POST must carry its Content-Type.
// The caller reads and closes the response body. An error is returned only when no response
// arrives (as kernel.HTTPClient.Do) or the request cannot be signed.
func (c *SignedHTTPClient) Do(ctx context.Context, req *kernel.HTTPRequest, key SigningKey) (*SignedResponse, error) {
	u, err := url.Parse(req.URL)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%w: %q is not an absolute URL", kernel.ErrInvalidHTTPRequest, req.URL)
	}
	host, err := CanonicalHost(u.Hostname())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", kernel.ErrInvalidHTTPRequest, err)
	}
	if p := u.Port(); p != "" {
		host += ":" + p
	}

	remembered := false
	if v, err := c.kv.Get(ctx, SchemeMemoryNamespace, host); err == nil {
		remembered = string(v) == string(SchemeRFC9421)
	} else if !errors.Is(err, kernel.ErrKeyNotFound) {
		return nil, fmt.Errorf("read signature scheme memory: %w", err)
	}
	first := c.defaultOptions(req, SchemeCavage)
	if remembered {
		first = c.defaultOptions(req, SchemeRFC9421)
	}
	resp, err := c.send(ctx, req, key, first)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnauthorized {
		return &SignedResponse{HTTPResponse: resp, Scheme: first.Scheme}, nil
	}

	retry := c.defaultOptions(req, SchemeCavage)
	if first.Scheme == SchemeCavage {
		retry = c.defaultOptions(req, SchemeRFC9421)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		if requested, ok := c.requested(resp.Header.Get("Accept-Signature")); ok {
			retry = c.defaultOptions(req, SchemeRFC9421)
			applyRequested(&retry, requested)
		}
	}
	discard(resp)

	resp, err = c.send(ctx, req, key, retry)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		switch {
		case retry.Scheme == SchemeRFC9421 && !remembered:
			err = c.kv.Set(ctx, SchemeMemoryNamespace, host, []byte(SchemeRFC9421), SchemeMemoryTTL)
		case retry.Scheme == SchemeCavage && remembered:
			err = c.kv.Delete(ctx, SchemeMemoryNamespace, host)
		}
		if err != nil {
			discard(resp)
			return nil, fmt.Errorf("write signature scheme memory: %w", err)
		}
	}
	return &SignedResponse{HTTPResponse: resp, Scheme: retry.Scheme}, nil
}

// send signs a copy of req with opts and sends it.
func (c *SignedHTTPClient) send(ctx context.Context, req *kernel.HTTPRequest, key SigningKey, opts SignOptions) (*kernel.HTTPResponse, error) {
	alg, err := signingAlgorithm(key)
	if err != nil {
		return nil, err
	}
	opts.Algorithm = alg
	r := *req
	r.Header = req.Header.Clone()
	if r.Header == nil {
		r.Header = http.Header{}
	}
	r.Header.Set("Date", c.clock.Now().UTC().Format(http.TimeFormat))
	if err := c.signer.Sign(&r, key, opts); err != nil {
		return nil, fmt.Errorf("sign request: %w", err)
	}
	return c.http.Do(ctx, &r)
}

func (c *SignedHTTPClient) defaultOptions(req *kernel.HTTPRequest, scheme SignatureScheme) SignOptions {
	post := req.Method == http.MethodPost
	switch {
	case scheme == SchemeCavage && post:
		return SignOptions{Scheme: scheme, Components: cavagePOST}
	case scheme == SchemeCavage:
		return SignOptions{Scheme: scheme, Components: cavageGET}
	case post:
		return SignOptions{Scheme: scheme, Components: rfc9421POST, TTL: RFC9421TTL}
	default:
		return SignOptions{Scheme: scheme, Components: rfc9421GET, TTL: RFC9421TTL}
	}
}

// requested parses an Accept-Signature value and returns its first entry.
func (c *SignedHTTPClient) requested(value string) (RequestedSignature, bool) {
	if value == "" {
		return RequestedSignature{}, false
	}
	reqs, err := c.signer.ParseAcceptSignature(value)
	if err != nil || len(reqs) == 0 {
		return RequestedSignature{}, false
	}
	return reqs[0], true
}

// applyRequested makes opts cover what the server asked for: its components (in addition to
// the profile's), label, tag, and nonce. The key, the algorithm, created, and expires stay as
// the profile has them.
func applyRequested(opts *SignOptions, r RequestedSignature) {
	components := append([]string(nil), opts.Components...)
	for _, c := range r.Components {
		found := false
		for _, x := range components {
			if x == c {
				found = true
				break
			}
		}
		if !found {
			components = append(components, c)
		}
	}
	opts.Components = components
	opts.Label = r.Label
	opts.Tag = r.Tag
	opts.Nonce = r.Nonce
}

func signingAlgorithm(key SigningKey) (string, error) {
	switch key.PrivateKey.(type) {
	case *rsa.PrivateKey:
		return AlgRSAPKCS1SHA256, nil
	case ed25519.PrivateKey:
		return AlgEd25519, nil
	}
	return "", fmt.Errorf("%w: %T", errNoAlgorithm, key.PrivateKey)
}

// discard drains a little of the body, so that the connection can be reused, and closes it.
func discard(resp *kernel.HTTPResponse) {
	_, _ = io.CopyN(io.Discard, resp.Body, 64<<10)
	_ = resp.Body.Close()
}
