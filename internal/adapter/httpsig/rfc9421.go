// SPDX-License-Identifier: AGPL-3.0-only

package httpsig

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	rfc9421 "github.com/dadrus/httpsig"
	"github.com/dunglas/httpsfv"

	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/kernel"
)

// unbounded disables the library's own time checks; federation.SignatureVerifier applies the
// window of ADR-0006 with the injected clock. 100 years keeps time arithmetic in range.
const unbounded = 100 * 365 * 24 * time.Hour

// parseRFC9421 reads the first signature of Signature-Input and checks that Signature carries
// it.
func parseRFC9421(r *federation.SignedRequest) (*federation.Signature, error) {
	inputs, err := httpsfv.UnmarshalDictionary(r.Header.Values(headerSignatureInput))
	if err != nil {
		return nil, fmt.Errorf("%w: Signature-Input: %w", federation.ErrMalformedSignature, err)
	}
	sigs, err := httpsfv.UnmarshalDictionary(r.Header.Values(headerSignature))
	if err != nil {
		return nil, fmt.Errorf("%w: Signature: %w", federation.ErrMalformedSignature, err)
	}
	names := inputs.Names()
	if len(names) == 0 {
		return nil, fmt.Errorf("%w: empty Signature-Input", federation.ErrMalformedSignature)
	}
	label := names[0]
	m, _ := inputs.Get(label)
	list, ok := m.(httpsfv.InnerList)
	if !ok {
		return nil, fmt.Errorf("%w: signature %s is not an inner list", federation.ErrMalformedSignature, label)
	}
	if s, ok := sigs.Get(label); !ok {
		return nil, fmt.Errorf("%w: no Signature for %s", federation.ErrMalformedSignature, label)
	} else if it, ok := s.(httpsfv.Item); !ok {
		return nil, fmt.Errorf("%w: Signature %s is not a byte sequence", federation.ErrMalformedSignature, label)
	} else if _, ok := it.Value.([]byte); !ok {
		return nil, fmt.Errorf("%w: Signature %s is not a byte sequence", federation.ErrMalformedSignature, label)
	}

	sig := &federation.Signature{Scheme: federation.SchemeRFC9421, Label: label}
	for _, it := range list.Items {
		c, err := componentString(it)
		if err != nil {
			return nil, err
		}
		sig.Components = append(sig.Components, c)
	}
	for _, name := range list.Params.Names() {
		v, _ := list.Params.Get(name)
		switch name {
		case "keyid", "alg":
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("%w: %s is not a string", federation.ErrMalformedSignature, name)
			}
			if name == "keyid" {
				sig.KeyID = s
			} else {
				sig.Algorithm = strings.ToLower(s)
			}
		case "created", "expires":
			n, ok := v.(int64)
			if !ok {
				return nil, fmt.Errorf("%w: %s is not an integer", federation.ErrMalformedSignature, name)
			}
			if name == "created" {
				sig.Created = time.Unix(n, 0).UTC()
			} else {
				sig.Expires = time.Unix(n, 0).UTC()
			}
		}
	}
	if cd := r.Header.Values(headerContentDigest); len(cd) > 0 {
		sig.Digests, err = parseContentDigest(cd)
		if err != nil {
			return nil, err
		}
	}
	return sig, nil
}

// componentString renders a component identifier as Signature.Components holds it: the name
// without quotes, followed by its parameters in Structured Field form.
func componentString(it httpsfv.Item) (string, error) {
	name, ok := it.Value.(string)
	if !ok {
		return "", fmt.Errorf("%w: component identifier is not a string", federation.ErrMalformedSignature)
	}
	full, err := httpsfv.Marshal(it)
	if err != nil {
		return "", fmt.Errorf("%w: %w", federation.ErrMalformedSignature, err)
	}
	bare, err := httpsfv.Marshal(httpsfv.NewItem(name))
	if err != nil {
		return "", fmt.Errorf("%w: %w", federation.ErrMalformedSignature, err)
	}
	return strings.ToLower(name) + strings.TrimPrefix(full, bare), nil
}

// parseContentDigest reads Content-Digest (RFC 9530): a dictionary of byte sequences.
func parseContentDigest(values []string) ([]federation.Digest, error) {
	d, err := httpsfv.UnmarshalDictionary(values)
	if err != nil {
		return nil, fmt.Errorf("%w: Content-Digest: %w", federation.ErrMalformedSignature, err)
	}
	var out []federation.Digest
	for _, name := range d.Names() {
		m, _ := d.Get(name)
		var value []byte
		if it, ok := m.(httpsfv.Item); ok {
			value, _ = it.Value.([]byte)
		}
		out = append(out, federation.Digest{Algorithm: strings.ToLower(name), Value: value})
	}
	return out, nil
}

// verifyRFC9421 checks the signature labeled sig.Label, and nothing else in the message.
func verifyRFC9421(r *federation.SignedRequest, sig *federation.Signature, opts federation.VerifyOptions) error {
	h, err := onlyLabel(r.Header, sig.Label)
	if err != nil {
		return fmt.Errorf("%w: %w", federation.ErrSignatureMismatch, err)
	}
	v, err := rfc9421.NewVerifier(
		rfc9421.Key{KeyID: sig.KeyID, Key: opts.PublicKey, Algorithm: rfc9421.SignatureAlgorithm(opts.Algorithm)},
		rfc9421.WithValidateAllSignatures(),
		rfc9421.WithCreatedTimestampRequired(false),
		rfc9421.WithExpiredTimestampRequired(false),
		rfc9421.WithMaxAge(unbounded),
		rfc9421.WithValidityTolerance(unbounded),
	)
	if err != nil {
		return fmt.Errorf("create RFC 9421 verifier: %w", err)
	}
	if err := v.Verify(message(r.Method, r.URL, h, r.Body)); err != nil {
		return fmt.Errorf("%w: %w", federation.ErrSignatureMismatch, err)
	}
	return nil
}

// onlyLabel returns a copy of h whose Signature-Input and Signature hold only label.
func onlyLabel(h http.Header, label string) (http.Header, error) {
	out := h.Clone()
	for _, name := range []string{headerSignatureInput, headerSignature} {
		d, err := httpsfv.UnmarshalDictionary(h.Values(name))
		if err != nil {
			return nil, err
		}
		m, ok := d.Get(label)
		if !ok {
			return nil, fmt.Errorf("no %s for %s", name, label)
		}
		one := httpsfv.NewDictionary()
		one.Add(label, m)
		v, err := httpsfv.Marshal(one)
		if err != nil {
			return nil, err
		}
		out.Set(name, v)
	}
	return out, nil
}

func message(method string, u *url.URL, h http.Header, body []byte) *rfc9421.Message {
	return &rfc9421.Message{
		Context:   context.Background(),
		Method:    method,
		Authority: u.Host,
		URL:       u,
		Header:    h,
		Body: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		},
		IsRequest: true,
	}
}

type fixedNonce string

func (n fixedNonce) GetNonce(context.Context) (string, error) { return string(n), nil }

// signRFC9421 signs req with RFC 9421. created comes from the system clock (the library's).
func signRFC9421(req *kernel.HTTPRequest, key federation.SigningKey, opts federation.SignOptions) error {
	u, err := url.Parse(req.URL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%w: %q is not an absolute URL", kernel.ErrInvalidHTTPRequest, req.URL)
	}
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	label := opts.Label
	if label == "" {
		label = "sig1"
	}
	signerOpts := []rfc9421.SignerOption{
		rfc9421.WithComponents(opts.Components...),
		rfc9421.WithTTL(opts.TTL),
		rfc9421.WithLabel(label),
		rfc9421.WithTag(opts.Tag),
		rfc9421.WithContentDigestAlgorithm(rfc9421.Sha256),
	}
	if opts.Nonce != "" {
		signerOpts = append(signerOpts, rfc9421.WithNonce(fixedNonce(opts.Nonce)))
	}
	s, err := rfc9421.NewSigner(
		rfc9421.Key{KeyID: key.KeyID, Key: key.PrivateKey, Algorithm: rfc9421.SignatureAlgorithm(opts.Algorithm)},
		signerOpts...,
	)
	if err != nil {
		return fmt.Errorf("create RFC 9421 signer: %w", err)
	}
	h, err := s.Sign(message(method, u, req.Header.Clone(), req.Body))
	if err != nil {
		return fmt.Errorf("sign with RFC 9421: %w", err)
	}
	for _, name := range []string{headerSignatureInput, headerSignature, headerContentDigest} {
		if v := h.Values(name); len(v) > 0 {
			req.Header[name] = v
		}
	}
	return nil
}

// parseAcceptSignature reads an Accept-Signature dictionary (RFC 9421 §5.1).
func parseAcceptSignature(value string) ([]federation.RequestedSignature, error) {
	d, err := httpsfv.UnmarshalDictionary([]string{value})
	if err != nil {
		return nil, fmt.Errorf("%w: Accept-Signature: %w", federation.ErrMalformedSignature, err)
	}
	var out []federation.RequestedSignature
	for _, label := range d.Names() {
		m, _ := d.Get(label)
		list, ok := m.(httpsfv.InnerList)
		if !ok {
			return nil, fmt.Errorf("%w: Accept-Signature %s is not an inner list", federation.ErrMalformedSignature, label)
		}
		rs := federation.RequestedSignature{Label: label}
		for _, it := range list.Items {
			c, err := componentString(it)
			if err != nil {
				return nil, err
			}
			rs.Components = append(rs.Components, c)
		}
		for _, name := range list.Params.Names() {
			v, _ := list.Params.Get(name)
			s, _ := v.(string)
			switch name {
			case "keyid":
				rs.KeyID = s
			case "alg":
				rs.Algorithm = s
			case "tag":
				rs.Tag = s
			case "nonce":
				rs.Nonce = s
			case "created":
				rs.Created = true
			case "expires":
				rs.Expires = true
			}
		}
		out = append(out, rs)
	}
	return out, nil
}
