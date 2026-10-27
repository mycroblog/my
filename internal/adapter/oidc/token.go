// SPDX-License-Identifier: AGPL-3.0-only

package oidc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
)

// header is the JOSE header of an ID token (RFC 7515 §4.1).
type header struct {
	Alg  string          `json:"alg"`
	Kid  string          `json:"kid"`
	Crit json.RawMessage `json:"crit"`
}

// claims are the claims of an ID token that the client checks (OpenID Connect Core 1.0 §2).
type claims struct {
	Iss           string          `json:"iss"`
	Sub           string          `json:"sub"`
	Aud           json.RawMessage `json:"aud"`
	Azp           string          `json:"azp"`
	Exp           *json.Number    `json:"exp"`
	Iat           *json.Number    `json:"iat"`
	AuthTime      *json.Number    `json:"auth_time"`
	Nonce         string          `json:"nonce"`
	Email         string          `json:"email"`
	EmailVerified json.RawMessage `json:"email_verified"`
}

// VerifyIDToken implements identity.OIDCClient.
func (c *Client) VerifyIDToken(ctx context.Context, p appidentity.OIDCProvider, raw string, check appidentity.IDTokenCheck) (appidentity.IDToken, error) {
	if check.ClientID == "" || check.Nonce == "" {
		return appidentity.IDToken{}, fmt.Errorf("%w: the check needs a client ID and a nonce", appidentity.ErrInvalidIDToken)
	}
	now := c.opts.Clock.Now()
	h, input, sig, payload, err := split(raw)
	if err != nil {
		return appidentity.IDToken{}, fmt.Errorf("%w: %w", appidentity.ErrInvalidIDToken, err)
	}
	// alg comes from the provider's own list, which never has none or an HMAC (ADR-0106 §1).
	if !slices.Contains(p.SigningAlgs, h.Alg) || !slices.Contains(appidentity.OIDCSigningAlgs, h.Alg) {
		return appidentity.IDToken{}, fmt.Errorf("%w: alg %q", appidentity.ErrInvalidIDToken, h.Alg)
	}
	if err := c.verifySignature(ctx, p.JWKSURI, h, input, sig, now); err != nil {
		return appidentity.IDToken{}, err
	}
	t, err := checkClaims(payload, p.Issuer, check, now)
	if err != nil {
		return appidentity.IDToken{}, fmt.Errorf("%w: %w", appidentity.ErrInvalidIDToken, err)
	}
	return t, nil
}

// split parses a compact JWS (RFC 7515 §7.1) into its header, signing input, signature, and
// payload.
func split(raw string) (h header, input, sig, payload []byte, err error) {
	if len(raw) > maxTokenBytes {
		return h, nil, nil, nil, errors.New("the token is too large")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return h, nil, nil, nil, errors.New("the token is not a compact JWS")
	}
	hb, err := decodeBase64(parts[0])
	if err != nil {
		return h, nil, nil, nil, fmt.Errorf("header: %w", err)
	}
	if err := json.Unmarshal(hb, &h); err != nil {
		return h, nil, nil, nil, fmt.Errorf("header: %w", err)
	}
	// No extension is understood, so a critical one fails the token (RFC 7515 §4.1.11).
	if h.Crit != nil {
		return h, nil, nil, nil, errors.New("the header has crit")
	}
	if payload, err = decodeBase64(parts[1]); err != nil {
		return h, nil, nil, nil, fmt.Errorf("payload: %w", err)
	}
	if sig, err = decodeBase64(parts[2]); err != nil {
		return h, nil, nil, nil, fmt.Errorf("signature: %w", err)
	}
	return h, []byte(parts[0] + "." + parts[1]), sig, payload, nil
}

// verifySignature checks the signature with the keys of the JWKS at uri that match the header.
// When none matches, it fetches the JWKS again once, if the last fetch is at least
// identity.OIDCJWKSRefetchInterval old (ADR-0106 §5).
func (c *Client) verifySignature(ctx context.Context, uri string, h header, input, sig []byte, now time.Time) error {
	keys, fetchedAt, err := c.keys(ctx, uri, now, false)
	if err != nil {
		return err
	}
	candidates := matching(keys, h)
	if len(candidates) == 0 && now.Sub(fetchedAt) >= appidentity.OIDCJWKSRefetchInterval {
		if keys, _, err = c.keys(ctx, uri, now, true); err != nil {
			return err
		}
		candidates = matching(keys, h)
	}
	if len(candidates) == 0 {
		return fmt.Errorf("%w: no key of the JWKS matches kid %q and alg %q", appidentity.ErrInvalidIDToken, h.Kid, h.Alg)
	}
	for _, k := range candidates {
		if k.verify(input, sig) {
			return nil
		}
	}
	return fmt.Errorf("%w: the signature does not verify", appidentity.ErrInvalidIDToken)
}

// matching returns the keys for h's algorithm with h's key ID; without a key ID, every key for
// the algorithm.
func matching(keys []jwk, h header) []jwk {
	var out []jwk
	for _, k := range keys {
		if k.alg == h.Alg && (h.Kid == "" || k.kid == h.Kid) {
			out = append(out, k)
		}
	}
	return out
}

// checkClaims checks the claims of a token whose signature verified (OpenID Connect Core 1.0
// §3.1.3.7 steps 2, 3, 4, 5, 9, 10, 11, and 13).
func checkClaims(payload []byte, issuer string, check appidentity.IDTokenCheck, now time.Time) (appidentity.IDToken, error) {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(payload, &all); err != nil {
		return appidentity.IDToken{}, fmt.Errorf("payload: %w", err)
	}
	var c claims
	d := json.NewDecoder(bytes.NewReader(payload))
	d.UseNumber()
	if err := d.Decode(&c); err != nil {
		return appidentity.IDToken{}, fmt.Errorf("payload: %w", err)
	}
	if c.Iss != issuer {
		return appidentity.IDToken{}, fmt.Errorf("iss %q is not %q", c.Iss, issuer)
	}
	if c.Sub == "" {
		return appidentity.IDToken{}, errors.New("no sub")
	}
	aud, err := audience(c.Aud)
	if err != nil {
		return appidentity.IDToken{}, err
	}
	if !slices.Contains(aud, check.ClientID) {
		return appidentity.IDToken{}, fmt.Errorf("aud %q does not have the client", aud)
	}
	if (len(aud) > 1 || c.Azp != "") && c.Azp != check.ClientID {
		return appidentity.IDToken{}, fmt.Errorf("azp %q is not the client", c.Azp)
	}
	exp, err := seconds("exp", c.Exp)
	if err != nil {
		return appidentity.IDToken{}, err
	}
	if !now.Before(exp.Add(ClockSkew)) {
		return appidentity.IDToken{}, fmt.Errorf("the token expired at %s", exp)
	}
	iat, err := seconds("iat", c.Iat)
	if err != nil {
		return appidentity.IDToken{}, err
	}
	if iat.After(now.Add(ClockSkew)) {
		return appidentity.IDToken{}, fmt.Errorf("the token is issued in the future at %s", iat)
	}
	if c.Nonce != check.Nonce {
		return appidentity.IDToken{}, errors.New("the nonce does not match")
	}
	var authTime time.Time
	if c.AuthTime != nil {
		if authTime, err = seconds("auth_time", c.AuthTime); err != nil {
			return appidentity.IDToken{}, err
		}
	}
	if !check.AuthenticatedAfter.IsZero() {
		if authTime.IsZero() {
			return appidentity.IDToken{}, errors.New("no auth_time")
		}
		if authTime.Add(ClockSkew).Before(check.AuthenticatedAfter) {
			return appidentity.IDToken{}, fmt.Errorf("auth_time %s is before %s", authTime, check.AuthenticatedAfter)
		}
	}
	return appidentity.IDToken{
		Issuer: c.Iss, Subject: c.Sub, Audience: aud, IssuedAt: iat, Expiry: exp, AuthTime: authTime,
		Nonce: c.Nonce, Email: c.Email, EmailVerified: emailVerified(c.EmailVerified), Claims: all,
	}, nil
}

// audience reads aud: a string or an array of strings (RFC 7519 §4.1.3).
func audience(raw json.RawMessage) ([]string, error) {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		if one == "" {
			return nil, errors.New("empty aud")
		}
		return []string{one}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil || len(many) == 0 {
		return nil, errors.New("no aud")
	}
	return many, nil
}

// maxNumericDate is the last second of the year 9999.
const maxNumericDate = 253402300799

// seconds reads a NumericDate claim (RFC 7519 §2), which is required.
func seconds(name string, n *json.Number) (time.Time, error) {
	if n == nil {
		return time.Time{}, fmt.Errorf("no %s", name)
	}
	f, err := n.Float64()
	if err != nil || math.IsNaN(f) || f <= 0 || f > maxNumericDate {
		return time.Time{}, fmt.Errorf("bad %s", name)
	}
	sec, frac := math.Modf(f)
	return time.Unix(int64(sec), int64(frac*1e9)).UTC(), nil
}

// emailVerified reads email_verified: a boolean, or the strings "true" and "false" that some
// providers send.
func emailVerified(raw json.RawMessage) bool {
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b
	}
	var s string
	return json.Unmarshal(raw, &s) == nil && s == "true"
}
