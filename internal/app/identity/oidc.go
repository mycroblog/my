// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// OIDCClient is the relying party's side of an OpenID Connect provider that an administrator
// configured (ADR-0106 §1 `oidc`, §5): Discovery, the provider's JWKS, and the verification of
// an ID token (OpenID Connect Core 1.0 §3.1.3.7). Every request goes through kernel.HTTPClient
// with kernel.HTTPPurposeLoginProvider. The adapter lives in internal/adapter/oidc and passes
// testkit.OIDCClientContract.
type OIDCClient interface {
	// Discover returns the metadata of the provider whose issuer identifier is issuer, from its
	// /.well-known/openid-configuration (OpenID Connect Discovery 1.0 §4), reused as long as
	// the response's cache directives allow. It fails with an error wrapping ErrOIDCDiscovery
	// when the document cannot be fetched, names another issuer, lacks an endpoint, or offers
	// none of OIDCSigningAlgs, and with an error wrapping kernel.ErrForbiddenDestination for a
	// destination the HTTP client refuses.
	Discover(ctx context.Context, issuer string) (OIDCProvider, error)
	// VerifyIDToken verifies the signature of the compact JWS raw with a key of p's JWKS and
	// checks its claims against check, and returns them. A token whose key ID the JWKS does not
	// list makes it fetch the JWKS again, at most once and only when the last fetch is at least
	// OIDCJWKSRefetchInterval old. It fails with an error wrapping ErrInvalidIDToken when the
	// token does not verify, and with another error when the JWKS cannot be fetched or read
	// (the provider's failure, not the token's).
	VerifyIDToken(ctx context.Context, p OIDCProvider, raw string, check IDTokenCheck) (IDToken, error)
}

// OIDCSigningAlgs are the JWS algorithms an ID token may be signed with (ADR-0106 §1): never
// none, and never an HMAC, whose key would be the client secret.
var OIDCSigningAlgs = []string{"RS256", "ES256", "EdDSA"}

// OIDCJWKSRefetchInterval is the least time between two fetches of a JWKS for an ID token whose
// key ID it does not list (ADR-0106 §5).
const OIDCJWKSRefetchInterval = time.Minute

// OIDCProvider is the metadata of an OpenID Connect provider (OpenID Connect Discovery 1.0 §3).
type OIDCProvider struct {
	// Issuer is the issuer identifier, equal to the one Discover was asked for.
	Issuer                string
	AuthorizationEndpoint string
	TokenEndpoint         string
	// UserinfoEndpoint is empty when the provider names none.
	UserinfoEndpoint string
	JWKSURI          string
	// SigningAlgs are the algorithms of OIDCSigningAlgs that the provider's
	// id_token_signing_alg_values_supported lists; VerifyIDToken accepts only these.
	SigningAlgs []string
	// CodeChallengeMethods is the provider's code_challenge_methods_supported, empty when it
	// names none.
	CodeChallengeMethods []string
}

// IDTokenCheck is what VerifyIDToken expects of an ID token besides its signature, its issuer
// (OIDCProvider.Issuer), and its lifetime.
type IDTokenCheck struct {
	// ClientID must be an audience of the token, and its authorized party when it has several.
	ClientID string
	// Nonce must equal the token's nonce; it is required.
	Nonce string
	// AuthenticatedAfter, when not zero, requires the token's auth_time no earlier than it: the
	// time the server sent a request with max_age=0 for a re-authentication (ADR-0106 §3.7).
	AuthenticatedAfter time.Time
}

// IDToken holds the claims of a verified ID token.
type IDToken struct {
	Issuer   string
	Subject  string
	Audience []string
	IssuedAt time.Time
	Expiry   time.Time
	// AuthTime is zero when the token has no auth_time.
	AuthTime time.Time
	Nonce    string
	// Email is empty when the token has none; EmailVerified is its email_verified.
	Email         string
	EmailVerified bool
	// Claims holds every claim of the token as it came, for the policies that restrict users
	// by a claim (ADR-0106 §4).
	Claims map[string]json.RawMessage
}

var (
	// ErrOIDCDiscovery reports a provider whose Discovery document cannot be used.
	ErrOIDCDiscovery = errors.New("identity: OpenID Connect discovery failed")
	// ErrInvalidIDToken reports an ID token that does not verify.
	ErrInvalidIDToken = errors.New("identity: invalid ID token")
)
