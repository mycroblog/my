// SPDX-License-Identifier: AGPL-3.0-only

package httpsig

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	cavage "code.superseriousbusiness.org/httpsig"

	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/kernel"
)

// parseCavage parses a draft-cavage Signature header. The library parses it too, but does not
// expose the covered headers and the declared algorithm, which the policy needs.
func parseCavage(r *federation.SignedRequest, value string) (*federation.Signature, error) {
	params, err := parseCavageParams(value)
	if err != nil {
		return nil, err
	}
	sig := &federation.Signature{
		Scheme:    federation.SchemeCavage,
		KeyID:     params["keyid"],
		Algorithm: strings.ToLower(params["algorithm"]),
	}
	if sig.KeyID == "" || params["signature"] == "" {
		return nil, fmt.Errorf("%w: keyId and signature are required", federation.ErrMalformedSignature)
	}
	if h, ok := params["headers"]; ok {
		sig.Components = strings.Fields(strings.ToLower(h))
	} else {
		sig.Components = []string{"date"} // the library's default, as in draft-cavage before -11
	}
	for name, dst := range map[string]*time.Time{"created": &sig.Created, "expires": &sig.Expires} {
		if v, ok := params[name]; ok {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%w: %s: %w", federation.ErrMalformedSignature, name, err)
			}
			*dst = time.Unix(n, 0).UTC()
		}
	}
	sig.Digests = parseDigestHeader(r.Header.Values(headerDigest))
	return sig, nil
}

// parseCavageParams splits the Signature header into its parameters: key="value" pairs (or
// bare values for created and expires) separated by commas. Keys are lowercased.
func parseCavageParams(s string) (map[string]string, error) {
	params := map[string]string{}
	for i := 0; i < len(s); {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == ',') {
			i++
		}
		if i == len(s) {
			break
		}
		eq := strings.IndexByte(s[i:], '=')
		if eq <= 0 {
			return nil, fmt.Errorf("%w: parameter without a value", federation.ErrMalformedSignature)
		}
		key := strings.ToLower(strings.TrimSpace(s[i : i+eq]))
		i += eq + 1
		var val string
		if i < len(s) && s[i] == '"' {
			end := strings.IndexByte(s[i+1:], '"')
			if end < 0 {
				return nil, fmt.Errorf("%w: unterminated quoted value", federation.ErrMalformedSignature)
			}
			val = s[i+1 : i+1+end]
			i += end + 2
		} else {
			end := strings.IndexByte(s[i:], ',')
			if end < 0 {
				end = len(s) - i
			}
			val = strings.TrimSpace(s[i : i+end])
			i += end
		}
		if _, dup := params[key]; dup {
			return nil, fmt.Errorf("%w: duplicate parameter %q", federation.ErrMalformedSignature, key)
		}
		params[key] = val
	}
	return params, nil
}

// parseDigestHeader reads Digest (RFC 3230) values: comma-separated algorithm=base64 pairs.
// A value that does not decode is kept with a nil digest, so that it fails to match.
func parseDigestHeader(values []string) []federation.Digest {
	var out []federation.Digest
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			alg, b64, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok {
				continue
			}
			d, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
			if err != nil {
				d = nil
			}
			out = append(out, federation.Digest{Algorithm: strings.ToLower(alg), Value: d})
		}
	}
	return out
}

func cavageAlgorithm(alg string) (cavage.Algorithm, error) {
	switch alg {
	case federation.AlgRSAPKCS1SHA256:
		return cavage.RSA_SHA256, nil
	case federation.AlgEd25519:
		return cavage.ED25519, nil
	}
	return "", fmt.Errorf("algorithm %q is not available for draft-cavage", alg)
}

// cavageRequest returns the *http.Request the library reads. Its URL.Path holds the escaped
// path, because the library writes URL.Path into (request-target) as is and the sender signed
// the path as it appears on the wire.
func cavageRequest(method string, u *url.URL, h http.Header) *http.Request {
	return &http.Request{
		Method: method,
		URL:    &url.URL{Path: u.EscapedPath(), RawQuery: u.RawQuery},
		Host:   u.Host,
		Header: h,
	}
}

// verifyCavage checks a draft-cavage signature. The library also refuses, by the system
// clock, a covered (created) more than 10 seconds ahead and an expires more than 10 seconds
// past; the policy's own window is checked by federation.SignatureVerifier.
func verifyCavage(r *federation.SignedRequest, _ *federation.Signature, opts federation.VerifyOptions) error {
	alg, err := cavageAlgorithm(opts.Algorithm)
	if err != nil {
		return fmt.Errorf("%w: %w", federation.ErrSignatureMismatch, err)
	}
	h := r.Header.Clone()
	h.Del("Authorization")
	if h.Get("Host") == "" {
		h.Set("Host", r.URL.Host)
	}
	v, err := cavage.NewVerifier(cavageRequest(r.Method, r.URL, h))
	if err != nil {
		return fmt.Errorf("%w: %w", federation.ErrSignatureMismatch, err)
	}
	if err := v.VerifyWithOptions(opts.PublicKey, alg, cavage.SignatureOption{ExcludeQueryStringFromPathPseudoHeader: opts.ExcludeQuery}); err != nil {
		return fmt.Errorf("%w: %w", federation.ErrSignatureMismatch, err)
	}
	return nil
}

// signCavage signs req with draft-cavage. The library always declares algorithm="hs2019";
// an RSA signature is declared as rsa-sha256 instead (ADR-0006). The algorithm parameter is
// not covered by the signature, so rewriting it keeps the signature valid.
func signCavage(req *kernel.HTTPRequest, key federation.SigningKey, opts federation.SignOptions) error {
	alg, err := cavageAlgorithm(opts.Algorithm)
	if err != nil {
		return err
	}
	u, err := url.Parse(req.URL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%w: %q is not an absolute URL", kernel.ErrInvalidHTTPRequest, req.URL)
	}
	h := req.Header.Clone()
	h.Set("Host", u.Host)
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	signer, got, err := cavage.NewSigner([]cavage.Algorithm{alg}, cavage.DigestSha256, opts.Components, cavage.Signature, 0)
	if err != nil {
		return fmt.Errorf("create draft-cavage signer: %w", err)
	}
	if got != alg {
		return fmt.Errorf("draft-cavage signer chose %s instead of %s", got, alg)
	}
	var body []byte
	if contains(opts.Components, "digest") {
		body = req.Body
		if body == nil {
			body = []byte{}
		}
	}
	hr := cavageRequest(method, u, h)
	if err := signer.SignRequest(key.PrivateKey, key.KeyID, hr, body); err != nil {
		return fmt.Errorf("sign with draft-cavage: %w", err)
	}
	value := hr.Header.Get(headerSignature)
	if opts.Algorithm == federation.AlgRSAPKCS1SHA256 {
		value = strings.Replace(value, `algorithm="hs2019"`, `algorithm="rsa-sha256"`, 1)
	}
	req.Header.Set(headerSignature, value)
	if d := hr.Header.Get(headerDigest); d != "" {
		req.Header.Set(headerDigest, d)
	}
	return nil
}
