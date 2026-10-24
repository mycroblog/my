// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The OAuth 2.0 authorization server (ADR-0016, ADR-0019 §4, ADR-0030): the authorization code
// grant with PKCE S256 only (RFC 7636), redirect URIs matched exactly, one-time codes, refresh
// tokens rotated on every use with the whole family revoked when a used one comes back
// (RFC 9700 §4.14.2), and revocation (RFC 7009).

// Token prefixes mark the server's secrets for secret scanners (ADR-0019 §4).
const (
	AccessTokenPrefix       = "my_at_"
	RefreshTokenPrefix      = "my_rt_"
	AuthorizationCodePrefix = "my_ac_"
)

// Token lifetimes (ADR-0019 §4).
const (
	AuthorizationCodeTTL = 60 * time.Second
	AccessTokenTTL       = time.Hour
	RefreshTokenTTL      = 30 * 24 * time.Hour
)

// DefaultWebClientID is the client_id of the first-party web client.
const DefaultWebClientID = "my-web"

// WebClient describes the first-party web client, which the server registers at startup.
// Its refresh token travels only in the __Host-my_rt cookie (ADR-0030), so the token endpoint
// refuses its refresh_token grant.
type WebClient struct {
	// ClientID is the client's public identifier; "" registers no web client.
	ClientID string
	Name     string
	// RedirectURIs are the URIs the client may redirect to, matched exactly (a loopback URI ignores the port).
	RedirectURIs []string
}

// OAuth error codes (RFC 6749 §4.1.2.1, §5.2; RFC 7009 §2.2.1).
const (
	OAuthInvalidRequest          = "invalid_request"
	OAuthInvalidClient           = "invalid_client"
	OAuthInvalidGrant            = "invalid_grant"
	OAuthUnauthorizedClient      = "unauthorized_client"
	OAuthUnsupportedGrantType    = "unsupported_grant_type"
	OAuthUnsupportedResponseType = "unsupported_response_type"
	OAuthInvalidScope            = "invalid_scope"
	OAuthAccessDenied            = "access_denied"
)

// OAuthError is an error the authorization server reports to the client by its code
// (RFC 6749 §5.2). Description is a short English hint for the client's developer, never
// shown to users and never revealing whether a token exists.
type OAuthError struct {
	Code        string
	Description string
}

func (e *OAuthError) Error() string {
	if e.Description == "" {
		return "oauth: " + e.Code
	}
	return "oauth: " + e.Code + ": " + e.Description
}

func oauthError(code, description string) *OAuthError {
	return &OAuthError{Code: code, Description: description}
}

// errInvalidGrant is the one answer for an unknown, expired, revoked, or mismatched code or
// refresh token, so that the answer does not tell which.
var errInvalidGrant = oauthError(OAuthInvalidGrant, "the grant is invalid, expired, or revoked")

// ErrUnknownClient and ErrInvalidRedirectURI reject an authorization request that cannot be
// answered by a redirect, because the redirect URI cannot be trusted (RFC 6749 §4.1.2.1). The
// user sees an error page instead.
var (
	ErrUnknownClient      = errors.New("identity: unknown OAuth client")
	ErrInvalidRedirectURI = errors.New("identity: redirect_uri is not registered for the client")
)

// ErrInvalidAccessToken reports an access token that is unknown, revoked, or expired.
var ErrInvalidAccessToken = errors.New("identity: invalid access token")

// OAuthRepository is the persistence of the authorization server (data-model §4.1). It is part
// of Repository. Every clientID argument and field is the ID of an oauth_clients row, except
// the public client_id that FindOAuthClient looks up.
type OAuthRepository interface {
	// EnsureOAuthClient inserts c unless a client with the same public client_id exists, and
	// returns the stored client.
	EnsureOAuthClient(ctx context.Context, tx uow.Tx, c identity.OAuthClient) (identity.OAuthClient, error)
	// ReplaceOAuthClientRedirectURIs makes uris the client's redirect URIs.
	ReplaceOAuthClientRedirectURIs(ctx context.Context, tx uow.Tx, clientID string, uris []string) error
	InsertAuthorizationCode(ctx context.Context, tx uow.Tx, c identity.AuthorizationCode) error
	// InsertAuthorizationCodeRedemption records the exchange of a code and reports whether it
	// did; false means the code was exchanged before.
	InsertAuthorizationCodeRedemption(ctx context.Context, tx uow.Tx, r identity.AuthorizationCodeRedemption) (bool, error)
	InsertAccessToken(ctx context.Context, tx uow.Tx, t identity.AccessToken) error
	// InsertTokenRevocation records that a token stopped working and reports whether it did;
	// false means the token had a revocation already, which keeps its first reason.
	InsertTokenRevocation(ctx context.Context, tx uow.Tx, r identity.TokenRevocation) (bool, error)
	// RevokeTokenFamily revokes every token of a family that has no revocation yet.
	RevokeTokenFamily(ctx context.Context, tx uow.Tx, familyID string, reason identity.TokenRevocationReason, at time.Time) error
	// RevokeTokensOfUser revokes every token of a user whose access or refresh token has not
	// expired at at and that has no revocation yet.
	RevokeTokensOfUser(ctx context.Context, tx uow.Tx, userID string, reason identity.TokenRevocationReason, at time.Time) error
	// RevokeOtherTokensOfUser is RevokeTokensOfUser for the tokens outside the family
	// keepFamilyID, which keeps working.
	RevokeOtherTokensOfUser(ctx context.Context, tx uow.Tx, userID, keepFamilyID string, reason identity.TokenRevocationReason, at time.Time) error
	// RevokeTokensOfClient revokes every token of a client whose access or refresh token has
	// not expired at at and that has no revocation yet.
	RevokeTokensOfClient(ctx context.Context, tx uow.Tx, clientID string, reason identity.TokenRevocationReason, at time.Time) error
	// UpsertTokenFamilyLastUsed records that a token family authenticated a request at at.
	UpsertTokenFamilyLastUsed(ctx context.Context, tx uow.Tx, familyID string, at time.Time) error
	// ListAuthorizedAppTokens returns the tokens of a user that have no revocation and an
	// access or refresh token that has not expired at now, with their clients.
	ListAuthorizedAppTokens(ctx context.Context, userID string, now time.Time) ([]identity.AuthorizedAppToken, error)
	// RevokeTokensOfUserAndClient revokes every token of the client that the user holds and that
	// has no revocation yet, and returns how many it revoked.
	RevokeTokensOfUserAndClient(ctx context.Context, tx uow.Tx, userID, clientID string, reason identity.TokenRevocationReason, at time.Time) (int64, error)
	// InsertOAuthClientDisablement records d and reports whether it did; false means the client
	// was disabled already.
	InsertOAuthClientDisablement(ctx context.Context, tx uow.Tx, d identity.OAuthClientDisablement) (bool, error)
	// LiftOAuthClientDisablements lifts every disablement of a client that has no lift yet and
	// reports whether there was one. byUserID is "" for the CLI.
	LiftOAuthClientDisablements(ctx context.Context, tx uow.Tx, clientID, byUserID string, at time.Time) (bool, error)
	// InsertOAuthClientDeletion records d and reports whether it did; false means the client
	// was deleted before.
	InsertOAuthClientDeletion(ctx context.Context, tx uow.Tx, d identity.OAuthClientDeletion) (bool, error)
	// ListUnusedDynamicOAuthClients returns the IDs of up to limit dynamically registered
	// clients without a deletion that are unused (ADR-0090 §1), oldest first: no authorization
	// code of them was exchanged and they were registered before unredeemedBefore, or none of
	// their tokens was issued at or after idleBefore or is still usable at now.
	ListUnusedDynamicOAuthClients(ctx context.Context, unredeemedBefore, idleBefore, now time.Time, limit int) ([]string, error)
	// TokenFamilyActive reports whether a token of the family has no revocation and an access
	// or refresh token that has not expired at now.
	TokenFamilyActive(ctx context.Context, familyID string, now time.Time) (bool, error)
	// RevokeSessionOfTokenFamily revokes the browser session that authorized the code a family
	// started from, unless the session has a revocation already or the code had no session.
	RevokeSessionOfTokenFamily(ctx context.Context, tx uow.Tx, familyID string, reason identity.RevocationReason, at time.Time) error

	// UpsertOAuthClientMetadataDocument stores the last fetch of a client's metadata document.
	UpsertOAuthClientMetadataDocument(ctx context.Context, tx uow.Tx, d identity.ClientMetadataDocument) error
	// FindOAuthClientMetadataDocument returns the last fetch of the metadata document of the
	// client with the ID; ErrNotFound when there is none.
	FindOAuthClientMetadataDocument(ctx context.Context, clientID string) (identity.ClientMetadataDocument, error)
	// InsertOAuthClientHostDenial records d and reports whether it did; false means the host was
	// denied already.
	InsertOAuthClientHostDenial(ctx context.Context, tx uow.Tx, d identity.OAuthClientHostDenial) (bool, error)
	// LiftOAuthClientHostDenials lifts every denial of a host that has no lift yet and reports
	// whether there was one. byUserID is "" for the CLI.
	LiftOAuthClientHostDenials(ctx context.Context, tx uow.Tx, host, byUserID string, at time.Time) (bool, error)
	// OAuthClientHostDenied reports whether a host has a denial without a lift.
	OAuthClientHostDenied(ctx context.Context, host string) (bool, error)

	// ListOAuthClients returns the clients that are not deleted, newest first, of the
	// registration route ("" for every route), whose ID is below before ("" from the newest).
	ListOAuthClients(ctx context.Context, registration identity.OAuthClientRegistration, before string, limit int) ([]identity.OAuthClientListing, error)
	// ListMetadataDocumentClientsOfHost returns the ID and client_id of the clients registered by
	// their metadata document whose client_id URL may be on the host; the caller keeps the exact
	// matches.
	ListMetadataDocumentClientsOfHost(ctx context.Context, tx uow.Tx, host string) ([]identity.OAuthClient, error)
	// ListOAuthClientHostDenials returns the denials of hosts without a lift, newest first,
	// whose ID is below before ("" from the newest).
	ListOAuthClientHostDenials(ctx context.Context, before string, limit int) ([]identity.OAuthClientHostDenial, error)

	// FindOAuthClient returns the client with the public client_id and its redirect URIs,
	// sorted.
	FindOAuthClient(ctx context.Context, clientID string) (identity.OAuthClient, []string, error)
	// FindOAuthClientByID returns the client with the ID.
	FindOAuthClientByID(ctx context.Context, clientID string) (identity.OAuthClient, error)
	// FindOAuthClientState returns what the disablements and the deletion of a client say about
	// it; ErrNotFound when there is no such client.
	FindOAuthClientState(ctx context.Context, clientID string) (identity.OAuthClientState, error)
	FindAuthorizationCode(ctx context.Context, codeHash []byte) (identity.AuthorizationCode, error)
	FindAuthorizationCodeRedemption(ctx context.Context, codeHash []byte) (identity.AuthorizationCodeRedemption, error)
	// FindAccessTokenByAccessHash and FindAccessTokenByRefreshHash return the token whose
	// access or refresh token hashes to hash, with whether it has a revocation and the state of
	// its client. Expiry is the caller's to check.
	FindAccessTokenByAccessHash(ctx context.Context, hash []byte) (identity.AccessToken, identity.TokenState, error)
	FindAccessTokenByRefreshHash(ctx context.Context, hash []byte) (identity.AccessToken, identity.TokenState, error)
}

// WebClientID returns the client_id of the first-party web client; "" when there is none.
func (s *Service) WebClientID() string { return s.d.WebClient.ClientID }

// EnsureWebClient registers the first-party web client with every scope, or makes the
// configured redirect URIs those of the registered one. It is safe to run on every start.
func (s *Service) EnsureWebClient(ctx context.Context) error {
	w := s.d.WebClient
	if w.ClientID == "" {
		return nil
	}
	c := identity.OAuthClient{
		ID: s.d.IDs.NewID(), ClientID: w.ClientID, Name: w.Name, Scope: identity.AllScopes(),
		Registration: identity.ClientRegisteredFirstParty, CreatedAt: s.d.Clock.Now(),
	}
	uris := slices.Clone(w.RedirectURIs)
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		stored, err := s.d.Repository.EnsureOAuthClient(ctx, tx, c)
		if err != nil {
			return err
		}
		return s.d.Repository.ReplaceOAuthClientRedirectURIs(ctx, tx, stored.ID, uris)
	})
	if err != nil {
		return fmt.Errorf("identity: register the web client: %w", err)
	}
	s.webClientReady.Store(true)
	return nil
}

// client returns the client with the public client_id and its redirect URIs. A disabled or
// deleted client, and one whose owner cannot use the app, is ErrNotFound, as an unknown one: it
// may not be authorized, get tokens, or revoke them (ADR-0090 §1), and so is a metadata document
// client while the instance does not accept them or its host is denied; its name, website, and
// scope are those of its stored document. It registers the web client first when the start-up registration
// has not succeeded yet (for example because the schema was applied after the server started).
func (s *Service) client(ctx context.Context, clientID string) (identity.OAuthClient, []string, error) {
	if clientID == "" {
		return identity.OAuthClient{}, nil, ErrNotFound
	}
	if clientID == s.d.WebClient.ClientID && !s.webClientReady.Load() {
		if err := s.EnsureWebClient(ctx); err != nil {
			return identity.OAuthClient{}, nil, err
		}
	}
	c, uris, err := s.d.Repository.FindOAuthClient(ctx, clientID)
	if err != nil {
		return identity.OAuthClient{}, nil, err
	}
	state, err := s.d.Repository.FindOAuthClientState(ctx, c.ID)
	if err != nil {
		return identity.OAuthClient{}, nil, err
	}
	if !state.Usable() {
		return identity.OAuthClient{}, nil, ErrNotFound
	}
	switch c.Registration {
	case identity.ClientRegisteredByUser:
		switch ok, err := s.ownerCanUseApps(ctx, c.OwnerUserID); {
		case err != nil:
			return identity.OAuthClient{}, nil, err
		case !ok:
			return identity.OAuthClient{}, nil, ErrNotFound
		}
	case identity.ClientRegisteredByMetadataDocument:
		if c, err = s.metadataDocumentClient(ctx, c); err != nil {
			return identity.OAuthClient{}, nil, err
		}
	}
	return c, uris, nil
}

// ErrFirstPartyClient refuses to disable or delete one of the server's own clients, which the
// server needs to work.
var ErrFirstPartyClient = errors.New("identity: a first-party OAuth client cannot be disabled or deleted")

// clientByID returns the client with the ID for disabling, enabling, or deleting it: ErrNotFound
// when there is none, ErrFirstPartyClient for one of the server's own clients.
func (s *Service) clientByID(ctx context.Context, clientID string) (identity.OAuthClient, error) {
	c, err := s.d.Repository.FindOAuthClientByID(ctx, clientID)
	if err != nil {
		return identity.OAuthClient{}, err
	}
	if c.FirstParty() {
		return identity.OAuthClient{}, ErrFirstPartyClient
	}
	return c, nil
}

// DisableOAuthClient records that the client with the ID (not its public client_id) is
// disabled and revokes every token family of it with client_disabled (ADR-0090 §1): until the
// disablement is lifted, the client cannot be authorized, exchange or refresh tokens, or use
// its access tokens. byUserID is the administrator, "" for the CLI. Disabling a disabled
// client records nothing new but revokes again, so the call is safe to repeat.
func (s *Service) DisableOAuthClient(ctx context.Context, clientID, byUserID string) error {
	c, err := s.clientByID(ctx, clientID)
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	d := identity.OAuthClientDisablement{ID: s.d.IDs.NewID(), ClientID: c.ID, DisabledByUserID: byUserID, DisabledAt: now}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		inserted, err := s.d.Repository.InsertOAuthClientDisablement(ctx, tx, d)
		if err != nil {
			return err
		}
		if err := s.d.Repository.RevokeTokensOfClient(ctx, tx, c.ID, identity.TokenRevokedClientDisabled, now); err != nil {
			return err
		}
		if !inserted {
			return nil
		}
		return s.emitOAuthClientAudit(ctx, tx, appmoderation.AuditOAuthClientDisabled, c, d.ID, byUserID, now)
	})
}

// EnableOAuthClient lifts the disablements of the client with the ID; byUserID is the
// administrator, "" for the CLI. The tokens that the disablement revoked stay revoked: the
// users authorize the client again. Enabling a client that is not disabled does nothing.
func (s *Service) EnableOAuthClient(ctx context.Context, clientID, byUserID string) error {
	c, err := s.clientByID(ctx, clientID)
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		lifted, err := s.d.Repository.LiftOAuthClientDisablements(ctx, tx, c.ID, byUserID, now)
		if err != nil || !lifted {
			return err
		}
		return s.emitOAuthClientAudit(ctx, tx, appmoderation.AuditOAuthClientEnabled, c, "", byUserID, now)
	})
}

// emitOAuthClientAudit emits the audit log entry of an administrator's operation on a client,
// labeled with its name and client_id (ADR-0037, ADR-0090 §1); source is the ID of the event
// row, "" when the operation has no row of its own.
func (s *Service) emitOAuthClientAudit(ctx context.Context, tx uow.Tx, action appmoderation.AuditAction, c identity.OAuthClient, source, byUserID string, at time.Time) error {
	entry := appmoderation.AuditEntry{
		ID: s.d.IDs.NewID(), Action: action, TargetKind: appmoderation.AuditTargetOAuthClient, TargetID: c.ID,
		TargetLabel: c.Name + " (" + c.ClientID + ")", SourceEventID: source, OccurredAt: at,
	}
	entry.ActorKind, entry.ActorUserID = appmoderation.AuditActor(byUserID)
	return tx.Emit(ctx, appmoderation.AuditRecorded{Entry: entry})
}

// ListOAuthClients returns the clients that are not deleted, newest first, at most limit (1 to
// MaxListLimit) of them, of the registration route ("" for every route) when given. A before
// other than "" is the ID of the last client of the previous page (ADR-0090 §1).
func (s *Service) ListOAuthClients(ctx context.Context, registration identity.OAuthClientRegistration, before string, limit int) ([]identity.OAuthClientListing, error) {
	if limit < 1 || limit > MaxListLimit {
		return nil, fmt.Errorf("identity: limit must be 1 to %d", MaxListLimit)
	}
	return s.d.Repository.ListOAuthClients(ctx, registration, before, limit)
}

// FindOAuthClientListing returns the client with the ID as the administrators list it;
// ErrNotFound when there is none or it is deleted.
func (s *Service) FindOAuthClientListing(ctx context.Context, clientID string) (identity.OAuthClientListing, error) {
	c, err := s.d.Repository.FindOAuthClientByID(ctx, clientID)
	if err != nil {
		return identity.OAuthClientListing{}, err
	}
	st, err := s.d.Repository.FindOAuthClientState(ctx, c.ID)
	if err != nil {
		return identity.OAuthClientListing{}, err
	}
	if st.Deleted {
		return identity.OAuthClientListing{}, ErrNotFound
	}
	return identity.OAuthClientListing{OAuthClient: c, Disabled: st.Disabled}, nil
}

// DeleteOAuthClient records that the client with the ID is deleted for the reason and revokes
// every token family of it with client_deleted (ADR-0090 §1); the client never works again.
// byUserID is the user who deleted it, "" for the clean-up. The call is safe to repeat: the
// first deletion keeps its reason.
func (s *Service) DeleteOAuthClient(ctx context.Context, clientID string, reason identity.OAuthClientDeletionReason, byUserID string) error {
	c, err := s.clientByID(ctx, clientID)
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	d := identity.OAuthClientDeletion{ClientID: c.ID, Reason: reason, DeletedByUserID: byUserID, DeletedAt: now}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.d.Repository.InsertOAuthClientDeletion(ctx, tx, d); err != nil {
			return err
		}
		return s.d.Repository.RevokeTokensOfClient(ctx, tx, c.ID, identity.TokenRevokedClientDeleted, now)
	})
}

// AuthorizationRequest is the query of an authorization request (RFC 6749 §4.1.1, RFC 7636 §4.3).
type AuthorizationRequest struct {
	ResponseType        string
	ClientID            string
	RedirectURI         string
	Scope               string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
}

// Authorization is a validated authorization request, waiting for the user's consent.
type Authorization struct {
	Client identity.OAuthClient
	// RedirectURI is the client's registered redirect URI that the request named, taken from
	// the registration, so that a redirect never carries the request's own value.
	RedirectURI   string
	Scope         identity.Scope
	State         string
	CodeChallenge string
	// OwnerUsername is the username of the user who registered the client, for the consent
	// page; "" when the client has no owner or the owner has no local account.
	OwnerUsername string
	// SessionID is the browser session in which the user authorizes the client, recorded with
	// the code so that the web client's sign-out can end it (RevokeToken); "" for none.
	SessionID string
}

// maxStateLength bounds the state the server echoes back.
const maxStateLength = 1024

// ValidateAuthorization checks an authorization request. A client_id that is an https URL is a
// Client ID Metadata Document, fetched first unless a fresh copy is stored
// (refreshMetadataDocument); a document that cannot be fetched or is refused makes the client
// unknown. It fails with ErrUnknownClient or
// ErrInvalidRedirectURI when the client is unknown or the redirect_uri is missing or not one
// of the client's registered URIs (compared exactly); the caller shows an error page. Other
// problems are an *OAuthError for the caller to send to the redirect URI: an unsupported
// response_type, a missing or malformed code_challenge, a code_challenge_method other than
// S256, or a scope the client may not have. An empty scope means the client's whole scope.
func (s *Service) ValidateAuthorization(ctx context.Context, req AuthorizationRequest) (Authorization, error) {
	if identity.IsMetadataDocumentClientID(req.ClientID) {
		err := s.refreshMetadataDocument(ctx, req.ClientID)
		if errors.Is(err, errMetadataDocumentClient) {
			return Authorization{}, fmt.Errorf("%w: %w", ErrUnknownClient, err)
		}
		if err != nil {
			return Authorization{}, err
		}
	}
	client, uris, err := s.client(ctx, req.ClientID)
	if errors.Is(err, ErrNotFound) {
		return Authorization{}, ErrUnknownClient
	}
	if err != nil {
		return Authorization{}, err
	}
	registered := slices.IndexFunc(uris, func(u string) bool { return identity.MatchRedirectURI(u, req.RedirectURI) })
	if req.RedirectURI == "" || registered < 0 {
		return Authorization{}, ErrInvalidRedirectURI
	}
	// The redirect goes to the registered URI, not to the request's copy of it. A loopback
	// registration matches any port, so then the request's value (which matched) is used and
	// the code exchange compares it exactly (RFC 8252 §7.3).
	a := Authorization{Client: client, RedirectURI: uris[registered], State: req.State, CodeChallenge: req.CodeChallenge}
	if uris[registered] != req.RedirectURI {
		a.RedirectURI = req.RedirectURI
	}
	switch {
	case req.ResponseType != "code":
		return a, oauthError(OAuthUnsupportedResponseType, "response_type must be code")
	case len(req.State) > maxStateLength:
		a.State = ""
		return a, oauthError(OAuthInvalidRequest, "state is too long")
	case req.CodeChallengeMethod != identity.CodeChallengeMethodS256:
		return a, oauthError(OAuthInvalidRequest, "code_challenge_method must be S256")
	case !identity.ValidCodeChallenge(req.CodeChallenge):
		return a, oauthError(OAuthInvalidRequest, "code_challenge must be a base64url S256 digest")
	}
	scope, err := identity.ParseScope(req.Scope)
	if err != nil {
		return a, oauthError(OAuthInvalidScope, "unknown scope")
	}
	if scope == "" {
		scope = client.Scope
	}
	if !scope.Within(client.Scope) {
		return a, oauthError(OAuthInvalidScope, "the client may not request this scope")
	}
	if scope.HasFirstPartyOnly() && !client.FirstParty() {
		return a, oauthError(OAuthInvalidScope, "the scope is for first-party clients only")
	}
	a.Scope = scope
	if client.OwnerUserID != "" {
		la, err := s.d.Accounts.LocalAccountOfUser(ctx, client.OwnerUserID)
		switch {
		case err == nil:
			a.OwnerUsername = la.Username
		case !errors.Is(err, account.ErrNotFound):
			return a, err
		}
	}
	return a, nil
}

// IssueAuthorizationCode returns a new one-time authorization code of a for the user, valid
// for AuthorizationCodeTTL. Only its hash is stored.
func (s *Service) IssueAuthorizationCode(ctx context.Context, a Authorization, userID string) (string, error) {
	now := s.d.Clock.Now()
	code, hash := NewToken(AuthorizationCodePrefix)
	c := identity.AuthorizationCode{
		CodeHash: hash, ClientID: a.Client.ID, UserID: userID, RedirectURI: a.RedirectURI, Scope: a.Scope,
		CodeChallenge: a.CodeChallenge, SessionID: a.SessionID, IssuedAt: now, ExpiresAt: now.Add(AuthorizationCodeTTL),
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.InsertAuthorizationCode(ctx, tx, c) })
	if err != nil {
		return "", err
	}
	return code, nil
}

// ClientCredentials identify the client of a token or revocation request: client_id, plus the
// secret of a confidential client.
type ClientCredentials struct {
	ClientID     string
	ClientSecret string
}

// IssuedTokens is the result of a token request.
type IssuedTokens struct {
	AccessToken  string
	RefreshToken string
	Token        identity.AccessToken
	Client       identity.OAuthClient
}

// authenticateClient returns the client of creds. A confidential client must present its
// secret; a public client must not present one.
func (s *Service) authenticateClient(ctx context.Context, creds ClientCredentials) (identity.OAuthClient, error) {
	invalid := oauthError(OAuthInvalidClient, "client authentication failed")
	client, _, err := s.client(ctx, creds.ClientID)
	if errors.Is(err, ErrNotFound) {
		return identity.OAuthClient{}, invalid
	}
	if err != nil {
		return identity.OAuthClient{}, err
	}
	if client.Public() {
		if creds.ClientSecret != "" {
			return identity.OAuthClient{}, invalid
		}
		return client, nil
	}
	if creds.ClientSecret == "" || subtle.ConstantTimeCompare(HashToken(creds.ClientSecret), client.SecretHash) != 1 {
		return identity.OAuthClient{}, invalid
	}
	return client, nil
}

// CodeExchange is a token request of the authorization_code grant (RFC 6749 §4.1.3).
type CodeExchange struct {
	ClientCredentials
	Code         string
	RedirectURI  string
	CodeVerifier string
}

// ExchangeAuthorizationCode exchanges a code for an access token and a refresh token. The
// code must belong to the client, not have expired, come with the redirect_uri it was issued
// for, and come with the code_verifier of its S256 challenge; otherwise it fails with an
// *OAuthError invalid_grant. A code works once: presenting it again fails and revokes every
// token issued from it (RFC 6749 §4.1.2).
func (s *Service) ExchangeAuthorizationCode(ctx context.Context, in CodeExchange) (IssuedTokens, error) {
	client, err := s.authenticateClient(ctx, in.ClientCredentials)
	if err != nil {
		return IssuedTokens{}, err
	}
	if in.Code == "" || in.CodeVerifier == "" || in.RedirectURI == "" {
		return IssuedTokens{}, oauthError(OAuthInvalidRequest, "code, redirect_uri, and code_verifier are required")
	}
	codeHash := HashToken(in.Code)
	code, err := s.d.Repository.FindAuthorizationCode(ctx, codeHash)
	if errors.Is(err, ErrNotFound) {
		return IssuedTokens{}, errInvalidGrant
	}
	if err != nil {
		return IssuedTokens{}, err
	}
	if code.ClientID != client.ID {
		return IssuedTokens{}, errInvalidGrant
	}
	switch red, err := s.d.Repository.FindAuthorizationCodeRedemption(ctx, codeHash); {
	case err == nil:
		return IssuedTokens{}, s.revokeFamily(ctx, red.TokenID, identity.TokenRevokedCodeReuse)
	case !errors.Is(err, ErrNotFound):
		return IssuedTokens{}, err
	}
	now := s.d.Clock.Now()
	if code.Expired(now) || code.RedirectURI != in.RedirectURI || !identity.VerifyCodeVerifier(in.CodeVerifier, code.CodeChallenge) {
		return IssuedTokens{}, errInvalidGrant
	}
	issued := s.newTokens(client, code.UserID, code.Scope, "", now)
	red := identity.AuthorizationCodeRedemption{CodeHash: codeHash, TokenID: issued.Token.ID, RedeemedAt: now}
	var reused bool
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ok, err := s.d.Repository.InsertAuthorizationCodeRedemption(ctx, tx, red)
		if err != nil || !ok {
			reused = !ok
			return err
		}
		return s.d.Repository.InsertAccessToken(ctx, tx, issued.Token)
	})
	if err != nil {
		return IssuedTokens{}, err
	}
	if reused { // a concurrent exchange won the race
		red, err := s.d.Repository.FindAuthorizationCodeRedemption(ctx, codeHash)
		if err != nil {
			return IssuedTokens{}, err
		}
		return IssuedTokens{}, s.revokeFamily(ctx, red.TokenID, identity.TokenRevokedCodeReuse)
	}
	return issued, nil
}

// RefreshRequest is a token request of the refresh_token grant (RFC 6749 §6).
type RefreshRequest struct {
	ClientCredentials
	RefreshToken string
	// Scope narrows the new token's scope; "" keeps the old token's.
	Scope string
}

// RefreshAccessToken exchanges a refresh token for a new access token and a new refresh token
// and revokes the old ones. Presenting a refresh token that was already used (or revoked)
// revokes its whole family and fails with invalid_grant (RFC 9700 §4.14.2). The web client may
// not use this grant: its refresh token is only accepted by RefreshWebClient.
func (s *Service) RefreshAccessToken(ctx context.Context, in RefreshRequest) (IssuedTokens, error) {
	client, err := s.authenticateClient(ctx, in.ClientCredentials)
	if err != nil {
		return IssuedTokens{}, err
	}
	if client.ClientID == s.d.WebClient.ClientID {
		return IssuedTokens{}, oauthError(OAuthUnauthorizedClient, "this client refreshes only through its cookie")
	}
	if in.RefreshToken == "" {
		return IssuedTokens{}, oauthError(OAuthInvalidRequest, "refresh_token is required")
	}
	return s.rotate(ctx, client, in.RefreshToken, in.Scope)
}

// RefreshWebClient is RefreshAccessToken for the refresh token of the web client's cookie
// (ADR-0030). It fails with invalid_grant for a refresh token of any other client.
func (s *Service) RefreshWebClient(ctx context.Context, refreshToken string) (IssuedTokens, error) {
	if s.d.WebClient.ClientID == "" || refreshToken == "" {
		return IssuedTokens{}, errInvalidGrant
	}
	client, _, err := s.client(ctx, s.d.WebClient.ClientID)
	if err != nil {
		return IssuedTokens{}, err
	}
	return s.rotate(ctx, client, refreshToken, "")
}

func (s *Service) rotate(ctx context.Context, client identity.OAuthClient, refreshToken, scope string) (IssuedTokens, error) {
	old, state, err := s.d.Repository.FindAccessTokenByRefreshHash(ctx, HashToken(refreshToken))
	if errors.Is(err, ErrNotFound) {
		return IssuedTokens{}, errInvalidGrant
	}
	if err != nil {
		return IssuedTokens{}, err
	}
	if old.ClientID != client.ID || !state.Client.Usable() {
		return IssuedTokens{}, errInvalidGrant
	}
	if state.Revoked {
		return IssuedTokens{}, s.revokeFamily(ctx, old.FamilyID, identity.TokenRevokedRefreshReuse)
	}
	now := s.d.Clock.Now()
	if old.RefreshExpired(now) {
		return IssuedTokens{}, errInvalidGrant
	}
	narrowed, err := identity.ParseScope(scope)
	if err != nil {
		return IssuedTokens{}, oauthError(OAuthInvalidScope, "unknown scope")
	}
	if narrowed == "" {
		narrowed = old.Scope
	}
	if !narrowed.Within(old.Scope) {
		return IssuedTokens{}, oauthError(OAuthInvalidScope, "the scope exceeds the original grant")
	}
	issued := s.newTokens(client, old.UserID, narrowed, old.FamilyID, now)
	issued.Token.RotatedFromTokenID = old.ID
	rev := identity.TokenRevocation{TokenID: old.ID, Reason: identity.TokenRevokedRotation, RevokedAt: now}
	var reused bool
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ok, err := s.d.Repository.InsertTokenRevocation(ctx, tx, rev)
		if err != nil || !ok {
			reused = !ok
			return err
		}
		return s.d.Repository.InsertAccessToken(ctx, tx, issued.Token)
	})
	if err != nil {
		return IssuedTokens{}, err
	}
	if reused { // a concurrent refresh with the same token won the race
		return IssuedTokens{}, s.revokeFamily(ctx, old.FamilyID, identity.TokenRevokedRefreshReuse)
	}
	return issued, nil
}

// newTokens returns a new access token and refresh token for the user; familyID "" starts a
// new family.
func (s *Service) newTokens(client identity.OAuthClient, userID string, scope identity.Scope, familyID string, now time.Time) IssuedTokens {
	at, atHash := NewToken(AccessTokenPrefix)
	rt, rtHash := NewToken(RefreshTokenPrefix)
	id := s.d.IDs.NewID()
	if familyID == "" {
		familyID = id
	}
	return IssuedTokens{
		AccessToken:  at,
		RefreshToken: rt,
		Client:       client,
		Token: identity.AccessToken{
			ID: id, FamilyID: familyID, ClientID: client.ID, UserID: userID,
			AccessTokenHash: atHash, RefreshTokenHash: rtHash, Scope: scope,
			IssuedAt: now, ExpiresAt: now.Add(AccessTokenTTL), RefreshExpiresAt: now.Add(RefreshTokenTTL),
		},
	}
}

// revokeFamily revokes a token family and returns errInvalidGrant, the answer to the request
// that revealed the reuse.
func (s *Service) revokeFamily(ctx context.Context, familyID string, reason identity.TokenRevocationReason) error {
	now := s.d.Clock.Now()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.RevokeTokenFamily(ctx, tx, familyID, reason, now)
	})
	if err != nil {
		return err
	}
	return errInvalidGrant
}

// RevokeRequest is a revocation request (RFC 7009 §2.1).
type RevokeRequest struct {
	ClientCredentials
	Token string
	// TokenTypeHint is "access_token", "refresh_token", or "" (RFC 7009 §2.1).
	TokenTypeHint string
}

// RevokeToken revokes an access token or a refresh token of the client and the whole family it
// belongs to, so that neither the access token nor the refresh token of the grant keeps
// working. An unknown token, or one of another client, is ignored (RFC 7009 §2.2).
//
// A revocation by the web client is its sign-out, so it also ends the browser session of the
// authorization screens in which the user authorized the family's code (as a logout).
// Otherwise the next sign-in on a shared device would pass without the password.
func (s *Service) RevokeToken(ctx context.Context, in RevokeRequest) error {
	client, err := s.authenticateClient(ctx, in.ClientCredentials)
	if err != nil {
		return err
	}
	if in.Token == "" {
		return oauthError(OAuthInvalidRequest, "token is required")
	}
	hash := HashToken(in.Token)
	finders := []func(context.Context, []byte) (identity.AccessToken, identity.TokenState, error){
		s.d.Repository.FindAccessTokenByAccessHash, s.d.Repository.FindAccessTokenByRefreshHash,
	}
	if in.TokenTypeHint == "refresh_token" {
		slices.Reverse(finders)
	}
	for _, find := range finders {
		tok, _, err := find(ctx, hash)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if tok.ClientID != client.ID {
			return nil
		}
		now := s.d.Clock.Now()
		web := client.ClientID == s.d.WebClient.ClientID
		return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			if err := s.d.Repository.RevokeTokenFamily(ctx, tx, tok.FamilyID, identity.TokenRevokedUser, now); err != nil {
				return err
			}
			if !web {
				return nil
			}
			return s.d.Repository.RevokeSessionOfTokenFamily(ctx, tx, tok.FamilyID, identity.RevokedLogout, now)
		})
	}
	return nil
}

// TokenFamilyActive reports whether the grant that a token family stands for can still be
// used: one of its tokens is neither revoked nor expired, counting its refresh token. Signing
// out of an app, a moderator's revocation, and a password change end it.
func (s *Service) TokenFamilyActive(ctx context.Context, familyID string) (bool, error) {
	return s.d.Repository.TokenFamilyActive(ctx, familyID, s.d.Clock.Now())
}

// AuthenticateAccessToken returns the token of a bearer access token, for the client API's
// authentication. It fails with ErrInvalidAccessToken when the token is unknown, revoked, or
// expired, or its client is disabled or deleted.
func (s *Service) AuthenticateAccessToken(ctx context.Context, accessToken string) (identity.AccessToken, error) {
	if accessToken == "" {
		return identity.AccessToken{}, ErrInvalidAccessToken
	}
	tok, state, err := s.d.Repository.FindAccessTokenByAccessHash(ctx, HashToken(accessToken))
	if errors.Is(err, ErrNotFound) {
		return identity.AccessToken{}, ErrInvalidAccessToken
	}
	if err != nil {
		return identity.AccessToken{}, err
	}
	if !state.Usable() || tok.AccessExpired(s.d.Clock.Now()) {
		return identity.AccessToken{}, ErrInvalidAccessToken
	}
	s.recordTokenUse(ctx, tok.FamilyID)
	return tok, nil
}
