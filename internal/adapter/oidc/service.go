// SPDX-License-Identifier: AGPL-3.0-only

package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	appidentity "github.com/mycroblog/my/internal/app/identity"
)

// ServiceTokenCheck is what VerifyServiceToken expects of an ID token that a platform service
// sends on behalf of a service account (the OIDC token of a Cloud Tasks or Cloud Scheduler HTTP
// target), besides its signature and lifetime. Such a token has no nonce: it answers no
// authentication request of this server.
type ServiceTokenCheck struct {
	// JWKSURI is the JWKS of the token's issuer.
	JWKSURI string
	// SigningAlgs are the algorithms of appidentity.OIDCSigningAlgs that the issuer signs with.
	SigningAlgs []string
	// Issuers are the accepted values of iss.
	Issuers []string
	// Audience must be an audience of the token, and its authorized party when it has several.
	Audience string
}

// VerifyServiceToken verifies the signature of the compact JWS raw with a key of the JWKS at
// check.JWKSURI and checks iss, sub, aud, azp, exp, and iat (OpenID Connect Core 1.0 §3.1.3.7
// steps 2, 3, 4, 5, 9, and 10), and returns the claims. Like VerifyIDToken, it fetches the JWKS
// again for a key ID it does not list at most once every appidentity.OIDCJWKSRefetchInterval. It
// fails with an error wrapping appidentity.ErrInvalidIDToken when the token does not verify, and
// with another error when the JWKS cannot be fetched or read.
func (c *Client) VerifyServiceToken(ctx context.Context, raw string, check ServiceTokenCheck) (appidentity.IDToken, error) {
	if check.Audience == "" || len(check.Issuers) == 0 || check.JWKSURI == "" {
		return appidentity.IDToken{}, fmt.Errorf("%w: the check needs a JWKS, an issuer, and an audience", appidentity.ErrInvalidIDToken)
	}
	now := c.opts.Clock.Now()
	h, input, sig, payload, err := split(raw)
	if err != nil {
		return appidentity.IDToken{}, fmt.Errorf("%w: %w", appidentity.ErrInvalidIDToken, err)
	}
	// alg comes from the issuer's own list, which never has none or an HMAC.
	if !slices.Contains(check.SigningAlgs, h.Alg) || !slices.Contains(appidentity.OIDCSigningAlgs, h.Alg) {
		return appidentity.IDToken{}, fmt.Errorf("%w: alg %q", appidentity.ErrInvalidIDToken, h.Alg)
	}
	if err := c.verifySignature(ctx, check.JWKSURI, h, input, sig, now); err != nil {
		return appidentity.IDToken{}, err
	}
	var iss struct {
		Iss string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &iss); err != nil {
		return appidentity.IDToken{}, fmt.Errorf("%w: payload: %w", appidentity.ErrInvalidIDToken, err)
	}
	if !slices.Contains(check.Issuers, iss.Iss) {
		return appidentity.IDToken{}, fmt.Errorf("%w: iss %q is not one of %q", appidentity.ErrInvalidIDToken, iss.Iss, check.Issuers)
	}
	t, err := checkClaims(payload, iss.Iss, appidentity.IDTokenCheck{ClientID: check.Audience}, now, false)
	if err != nil {
		return appidentity.IDToken{}, fmt.Errorf("%w: %w", appidentity.ErrInvalidIDToken, err)
	}
	return t, nil
}
