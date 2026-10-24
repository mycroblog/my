// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/domain/identity"
)

// ErrAccountUnavailable reports a valid access token whose user cannot use the app: the user
// is not Access.Allowed (registration pending or rejected, email unverified), their local
// account is not active (deactivated, scheduled for deletion, or deleted), or moderation
// forbids the account to log in (disable_login, suspend).
var ErrAccountUnavailable = errors.New("identity: account cannot use the app")

// Bearer is who a client API request acts as: an access token that is valid now, of a user
// who may use the app, with the local account the user logs in as.
type Bearer struct {
	TokenID string
	// FamilyID is the token family of the grant that issued the token: the ID of its first
	// token, which refreshing the grant keeps.
	FamilyID string
	// ClientID is the ID of the OAuthClient (not its public client_id).
	ClientID string
	// ClientPublicID is the client's public client_id.
	ClientPublicID string
	UserID         string
	// AccountID is the local account the user logs in as.
	AccountID string
	Scope     identity.Scope
	// FirstParty is true when the token's client is this server's own (ADR-0090 §4).
	FirstParty bool
	// ExpiresAt is when the access token expires.
	ExpiresAt time.Time
}

// AuthenticateBearer authenticates the bearer access token of a client API request
// (ADR-0019 §4). It fails with ErrInvalidAccessToken when the token is unknown, revoked, or
// expired, and with ErrAccountUnavailable when the token is valid but its user cannot use the
// app; the user's state is checked on every request, so that a token issued before a
// moderator's decision stops working at once.
func (s *Service) AuthenticateBearer(ctx context.Context, accessToken string) (Bearer, error) {
	tok, err := s.AuthenticateAccessToken(ctx, accessToken)
	if err != nil {
		return Bearer{}, err
	}
	access, err := s.UserAccess(ctx, tok.UserID)
	if err != nil {
		return Bearer{}, err
	}
	if !access.Allowed() || access.AccountID == "" || !access.AccountActive {
		return Bearer{}, ErrAccountUnavailable
	}
	client, err := s.d.Repository.FindOAuthClientByID(ctx, tok.ClientID)
	if err != nil {
		return Bearer{}, err
	}
	return Bearer{
		TokenID: tok.ID, FamilyID: tok.FamilyID, ClientID: tok.ClientID, ClientPublicID: client.ClientID, UserID: tok.UserID,
		AccountID: access.AccountID, Scope: tok.Scope, FirstParty: client.FirstParty(), ExpiresAt: tok.ExpiresAt,
	}, nil
}

// UserLocale returns the UI locale of a user's preferences, for localizing the responses to
// the user's requests (ADR-0009 §2). It returns "" when the user has no preferences.
func (s *Service) UserLocale(ctx context.Context, userID string) (string, error) {
	p, err := s.d.Repository.FindPreferences(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return p.Locale, nil
}
