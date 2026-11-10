// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"slices"
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

// loginBatch is a LoginPolicy that reads many accounts at once, as *moderation.AccountPolicy does.
type loginBatch interface {
	LoginRestrictedAmong(ctx context.Context, accountIDs []string) (map[string]bool, error)
}

// ValidBearers reports, by token, whether AuthenticateBearer would accept each of accessTokens
// now, with a number of queries that does not grow with them: the open streams check their
// tokens with it on every heartbeat. It does not record their use; the caller records it with
// RecordBearerUse as often as it needs.
func (s *Service) ValidBearers(ctx context.Context, accessTokens []string) (map[string]bool, error) {
	out := make(map[string]bool, len(accessTokens))
	hashes := make([][]byte, 0, len(accessTokens))
	for _, t := range accessTokens {
		if t != "" {
			hashes = append(hashes, HashToken(t))
		}
	}
	if len(hashes) == 0 {
		return out, nil
	}
	tokens, err := s.d.Repository.FindAccessTokensByAccessHashes(ctx, hashes)
	if err != nil {
		return nil, err
	}
	now := s.d.Clock.Now()
	var users, clients []string
	usable := map[string]identity.AccessToken{}
	for _, t := range accessTokens {
		tw, ok := tokens[string(HashToken(t))]
		if t == "" || !ok || !tw.State.Usable() || tw.Token.AccessExpired(now) {
			continue
		}
		usable[t] = tw.Token
		if !slices.Contains(users, tw.Token.UserID) {
			users = append(users, tw.Token.UserID)
		}
		if !slices.Contains(clients, tw.Token.ClientID) {
			clients = append(clients, tw.Token.ClientID)
		}
	}
	allowed, err := s.usersAllowed(ctx, users)
	if err != nil {
		return nil, err
	}
	existing, err := s.d.Repository.ExistingOAuthClients(ctx, clients)
	if err != nil {
		return nil, err
	}
	for t, tok := range usable {
		out[t] = allowed[tok.UserID] && existing[tok.ClientID]
	}
	return out, nil
}

// RecordBearerUse records the use of the token family of accessToken, as AuthenticateBearer
// does.
func (s *Service) RecordBearerUse(ctx context.Context, accessToken string) {
	tok, _, err := s.d.Repository.FindAccessTokenByAccessHash(ctx, HashToken(accessToken))
	if err == nil {
		s.recordTokenUse(ctx, tok.FamilyID)
	}
}

// usersAllowed reports, by user, whether each of userIDs has the access AuthenticateBearer
// requires (UserAccess: Allowed, with an active account), with one query per part.
func (s *Service) usersAllowed(ctx context.Context, userIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(userIDs))
	if len(userIDs) == 0 || s.d.Accounts == nil {
		return out, nil
	}
	states, err := s.d.Repository.FindRegistrationStates(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	verified, err := s.d.Repository.PrimaryEmailsVerified(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	lives, err := s.d.Accounts.UserLifecycles(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	var accounts []string
	for _, l := range lives {
		accounts = append(accounts, l.AccountID)
	}
	restricted := map[string]bool{}
	if b, ok := s.d.Logins.(loginBatch); ok {
		if restricted, err = b.LoginRestrictedAmong(ctx, accounts); err != nil {
			return nil, err
		}
	} else {
		for _, id := range accounts {
			if restricted[id], err = s.loginRestricted(ctx, id); err != nil {
				return nil, err
			}
		}
	}
	for _, u := range userIDs {
		life, ok := lives[u]
		if !ok {
			continue // no local account: AuthenticateBearer refuses it
		}
		a := Access{Registration: identity.RegistrationApproved, EmailVerificationRequired: s.d.RequireEmailVerification, EmailVerified: true}
		if st, ok := states[u]; ok {
			a.Registration = st.State
		}
		if v, ok := verified[u]; ok {
			a.EmailVerified = v
		}
		a.AccountID, a.AccountActive, a.Deactivated = life.AccountID, life.Active(), life.Deactivated()
		a.LoginRestricted = restricted[life.AccountID]
		out[u] = a.Allowed() && a.AccountID != "" && a.AccountActive
	}
	return out, nil
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
