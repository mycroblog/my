// SPDX-License-Identifier: AGPL-3.0-only

package cloudtasks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mycroblog/my/internal/adapter/oidc"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/queue"
)

// The issuer of the OIDC tokens of Google service accounts, which Cloud Tasks and Cloud
// Scheduler send. Google's own libraries accept iss with and without the scheme.
const (
	GoogleJWKSURI = "https://www.googleapis.com/oauth2/v3/certs"
	googleIssuer  = "https://accounts.google.com"
)

// AuthOptions configure an Authenticator. OIDC and ServiceAccount are required.
type AuthOptions struct {
	// OIDC verifies the tokens and keeps the issuer's JWKS.
	OIDC *oidc.Client
	// ServiceAccount is the email that a token must have, verified.
	ServiceAccount string
	// Audience is the audience that a token must have; empty requires the URL of the endpoint
	// called, the audience that Cloud Tasks and Cloud Scheduler give by default.
	Audience string
	// JWKSURI and Issuers override GoogleJWKSURI and Google's issuer (tests only).
	JWKSURI string
	Issuers []string
}

// Authenticator is the queue.PushAuthenticator of Cloud Tasks and Cloud Scheduler: it accepts
// a Google-signed OIDC ID token (RS256, Google's JWKS) whose iss is Google, whose aud is the
// configured audience, whose email is the configured service account with email_verified, and
// that has not expired.
type Authenticator struct{ o AuthOptions }

var _ queue.PushAuthenticator = (*Authenticator)(nil)

// NewAuthenticator returns an Authenticator on o. It panics when a required option is missing.
func NewAuthenticator(o AuthOptions) *Authenticator {
	if o.OIDC == nil || o.ServiceAccount == "" {
		panic("cloudtasks: AuthOptions.OIDC and ServiceAccount are required")
	}
	if o.JWKSURI == "" {
		o.JWKSURI = GoogleJWKSURI
	}
	if len(o.Issuers) == 0 {
		o.Issuers = []string{googleIssuer, strings.TrimPrefix(googleIssuer, "https://")}
	}
	return &Authenticator{o: o}
}

// Authenticate implements queue.PushAuthenticator.
func (a *Authenticator) Authenticate(ctx context.Context, token, endpoint string) error {
	aud := a.o.Audience
	if aud == "" {
		aud = endpoint
	}
	t, err := a.o.OIDC.VerifyServiceToken(ctx, token, oidc.ServiceTokenCheck{
		JWKSURI: a.o.JWKSURI, SigningAlgs: []string{"RS256"}, Issuers: a.o.Issuers, Audience: aud,
	})
	if err != nil {
		if errors.Is(err, appidentity.ErrInvalidIDToken) {
			return fmt.Errorf("%w: %w", queue.ErrPushUnauthenticated, err)
		}
		return fmt.Errorf("cloudtasks: %w", err)
	}
	if !strings.EqualFold(t.Email, a.o.ServiceAccount) || !t.EmailVerified {
		return fmt.Errorf("%w: the token is not of the service account %s with a verified email", queue.ErrPushUnauthenticated, a.o.ServiceAccount)
	}
	return nil
}
