// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Signing in and signing up with an external login provider that an administrator set up
// (FR-010, ADR-0106 §3, §4), with the Authorization Code flow, state, PKCE (S256), and, for
// OpenID Connect, a nonce.
//
// BeginExternalLogin seals the state, the nonce, the PKCE code_verifier, and the return target
// for a short-lived cookie and returns the provider's authorization URL. FinishExternalLogin
// takes the provider's answer with that cookie: it checks the state, which works once, exchanges
// the code for tokens with a form POST through kernel.HTTPClient (kernel.HTTPPurposeLoginProvider),
// verifies the ID token, and checks the provider's conditions on every sign-in. It keeps none of
// the provider's tokens. A linked user signs in as with a password: the client's rate limit,
// the refusal of an inactive account, a new session with its origin, and the code step of a
// user with TOTP. An account that no user links goes on to sign up when the provider's
// allow_signup and the registration mode allow it (SignUpExternal); a user whose email address
// matches the provider's is never linked by it.

var (
	// ErrLoginProviderUnavailable reports a login provider that is unknown, deleted, disabled,
	// of a kind that cannot sign in yet, or that does not let new users sign up.
	ErrLoginProviderUnavailable = errors.New("identity: the login provider is not available")
	// ErrExternalLoginExpired reports a sign-in whose cookie or state is missing, malformed,
	// expired, another sign-in's, or used, and a sign-up whose ticket is: it starts again.
	ErrExternalLoginExpired = errors.New("identity: the external sign-in expired")
	// ErrExternalLoginFailed reports a sign-in that the provider did not complete: the user
	// cancelled it, the provider answered an error, or its tokens did not verify.
	ErrExternalLoginFailed = errors.New("identity: the external sign-in failed")
	// ErrExternalLoginDenied reports an account at the provider that does not meet the
	// provider's conditions (ADR-0106 §4).
	ErrExternalLoginDenied = errors.New("identity: the login provider's conditions refuse the account")
	// ErrExternalAccountNotLinked reports an account at the provider that no user links, when
	// nobody may sign up with it: the provider does not allow it, or registration is closed.
	ErrExternalAccountNotLinked = errors.New("identity: no user is linked to the external account")
)

// External sign-in defaults.
const (
	// ExternalLoginTTL is how long the user has at the provider, from BeginExternalLogin to
	// FinishExternalLogin.
	ExternalLoginTTL = 10 * time.Minute
	// ExternalSignUpTTL is how long the ticket of a sign-up stays valid.
	ExternalSignUpTTL = 30 * time.Minute
	// ExternalLoginNamespace is the KeyValueStore namespace of the used states.
	ExternalLoginNamespace = "external_login"
	// ExternalLoginPathPrefix is the start of the paths of the external sign-in:
	// <prefix><provider ID>/start and <prefix><provider ID>/callback. The authpages package
	// serves them.
	ExternalLoginPathPrefix = "/auth/external/"
)

// oidcScopes are the scopes that a sign-in with an OpenID Connect provider asks for besides
// the provider's extra scopes.
var oidcScopes = []string{"openid", "email", "profile"}

// loginProviderSecretColumn binds a sealed client secret to its column, with the secret's ID
// (login_provider_secrets.sealed_secret, ADR-0019 §6).
const loginProviderSecretColumn = "login_provider_secrets.sealed_secret"

// AADs that bind the sealed state of a sign-in and the ticket of a sign-up to their purpose.
var (
	externalLoginAAD  = kernel.AAD("external_login")
	externalSignUpAAD = kernel.AAD("external_sign_up")
)

// ExternalLoginsEnabled reports whether the service has what signing in with external login
// providers needs.
func (s *Service) ExternalLoginsEnabled() bool { return s.needExternalLoginDeps() == nil }

func (s *Service) needExternalLoginDeps() error {
	if s.d.OIDC == nil || s.d.HTTP == nil || s.d.Sealer == nil || s.d.KV == nil || s.d.RateLimiter == nil {
		return errors.New("identity: external sign-in needs OIDC, HTTP, a Sealer, a KV store, and a RateLimiter")
	}
	return nil
}

// externalLoginKind reports whether users sign in with providers of kind k.
func externalLoginKind(k identity.LoginProviderKind) bool {
	return k == identity.LoginProviderOIDC || k == identity.LoginProviderMastodon
}

// ExternalLoginProvider is a provider that the sign-in page offers.
type ExternalLoginProvider struct {
	ID   string
	Kind identity.LoginProviderKind
	// DisplayName is the administrator's name for the provider, shown as written.
	DisplayName string
	// AllowSignup tells that new users sign up with it.
	AllowSignup bool
}

// ExternalLoginProviders returns the enabled providers that users sign in with, oldest first;
// none without the dependencies of external sign-in.
func (s *Service) ExternalLoginProviders(ctx context.Context) ([]ExternalLoginProvider, error) {
	if !s.ExternalLoginsEnabled() {
		return nil, nil
	}
	ps, err := s.d.Repository.ListLoginProviders(ctx)
	if err != nil {
		return nil, err
	}
	var out []ExternalLoginProvider
	for _, p := range slices.Backward(ps) {
		if p.State == identity.LoginProviderEnabled && externalLoginKind(p.Settings.Kind) {
			// A Mastodon server under a domain action has no button.
			switch err := s.checkLoginProviderServer(ctx, p); {
			case errors.Is(err, ErrLoginProviderUnavailable):
				continue
			case err != nil:
				return nil, err
			}
			out = append(out, ExternalLoginProvider{
				ID: p.ID, Kind: p.Settings.Kind, DisplayName: p.Settings.DisplayName, AllowSignup: p.Settings.AllowSignup,
			})
		}
	}
	return out, nil
}

// enabledLoginProvider returns a provider that users sign in with, or ErrLoginProviderUnavailable.
func (s *Service) enabledLoginProvider(ctx context.Context, id string) (identity.LoginProvider, error) {
	if err := s.needExternalLoginDeps(); err != nil {
		return identity.LoginProvider{}, err
	}
	p, err := s.d.Repository.FindLoginProvider(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return identity.LoginProvider{}, ErrLoginProviderUnavailable
	}
	if err != nil {
		return identity.LoginProvider{}, err
	}
	if p.State != identity.LoginProviderEnabled || !externalLoginKind(p.Settings.Kind) {
		return identity.LoginProvider{}, ErrLoginProviderUnavailable
	}
	if err := s.checkLoginProviderServer(ctx, p); err != nil {
		return identity.LoginProvider{}, err
	}
	return p, nil
}

// externalCallbackURL is the redirect URI of a provider, which its administrator registers at the
// provider (ADR-0106 §2).
func (s *Service) externalCallbackURL(providerID string) string {
	return strings.TrimSuffix(s.d.BaseURL, "/") + ExternalLoginPathPrefix + url.PathEscape(providerID) + "/callback"
}

// externalLoginToken is what the cookie of a sign-in seals.
type externalLoginToken struct {
	IssuedAt   int64  `json:"iat"` // Unix milliseconds
	ProviderID string `json:"provider"`
	State      string `json:"state"`
	Nonce      string `json:"nonce"`
	Verifier   string `json:"verifier"`
	ReturnTo   string `json:"return_to,omitempty"`
}

// externalSignUpToken is what the ticket of a sign-up seals.
type externalSignUpToken struct {
	IssuedAt   int64  `json:"iat"` // Unix milliseconds
	ProviderID string `json:"provider"`
	Subject    string `json:"sub"`
	Email      string `json:"email,omitempty"`
	// EmailVerified tells that the provider verified Email.
	EmailVerified bool   `json:"email_verified,omitempty"`
	Username      string `json:"username,omitempty"`
}

// ExternalLoginStart is the start of a sign-in with a provider.
type ExternalLoginStart struct {
	// AuthorizationURL is where the browser goes: the provider's authorization endpoint with the
	// request.
	AuthorizationURL string
	// Cookie is the sealed state of the sign-in, which FinishExternalLogin takes back; the
	// browser keeps it until ExpiresAt.
	Cookie    string
	ExpiresAt time.Time
}

// BeginExternalLogin starts a sign-in with the provider providerID that returns to returnTo, a
// path on this site that the caller checked, or "". It fails with ErrLoginProviderUnavailable
// for a provider that users cannot sign in with, and with an error wrapping
// ErrExternalLoginFailed when the provider's Discovery document cannot be used.
func (s *Service) BeginExternalLogin(ctx context.Context, providerID, returnTo string) (ExternalLoginStart, error) {
	p, err := s.enabledLoginProvider(ctx, providerID)
	if err != nil {
		return ExternalLoginStart{}, err
	}
	if p.Settings.Kind == identity.LoginProviderMastodon {
		return s.beginMastodonLogin(ctx, p, returnTo)
	}
	op, err := s.d.OIDC.Discover(ctx, p.Settings.Issuer)
	if err != nil {
		return ExternalLoginStart{}, fmt.Errorf("%w: provider %s: %w", ErrExternalLoginFailed, p.ID, err)
	}
	now := s.d.Clock.Now()
	t := externalLoginToken{
		IssuedAt: now.UnixMilli(), ProviderID: p.ID, ReturnTo: returnTo,
		State: randomToken(), Nonce: randomToken(), Verifier: randomToken(),
	}
	cookie, err := s.sealJSON(ctx, t, externalLoginAAD)
	if err != nil {
		return ExternalLoginStart{}, err
	}
	u, err := url.Parse(op.AuthorizationEndpoint)
	if err != nil {
		return ExternalLoginStart{}, fmt.Errorf("%w: provider %s: authorization endpoint: %w", ErrExternalLoginFailed, p.ID, err)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", p.Settings.ClientID)
	q.Set("redirect_uri", s.externalCallbackURL(p.ID))
	q.Set("scope", strings.Join(append(slices.Clone(oidcScopes), p.Settings.Scopes...), " "))
	q.Set("state", t.State)
	q.Set("nonce", t.Nonce)
	// PKCE goes out even to a provider that does not name S256, which ignores it (ADR-0106 §1).
	challenge := sha256.Sum256([]byte(t.Verifier))
	q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	q.Set("code_challenge_method", "S256")
	// A condition on one hd (a Google Workspace domain) also goes out as the hint of the
	// account chooser; the ID token's claim decides (ADR-0106 §4).
	for _, c := range p.Settings.Claims {
		if c.Claim == "hd" && len(c.Values) == 1 {
			q.Set("hd", c.Values[0])
		}
	}
	u.RawQuery = q.Encode()
	return ExternalLoginStart{AuthorizationURL: u.String(), Cookie: cookie, ExpiresAt: now.Add(ExternalLoginTTL)}, nil
}

// ExternalLoginInput is the provider's answer to a sign-in, at its callback.
type ExternalLoginInput struct {
	// ProviderID is the provider of the callback's path.
	ProviderID string
	// Cookie is BeginExternalLogin's.
	Cookie string
	// State, Code, and Error are the query parameters of the answer.
	State string
	Code  string
	Error string
	// IP, UserAgent, and CurrentToken are as in SignInInput.
	IP           string
	UserAgent    string
	CurrentToken string
}

// ExternalLogin is the outcome of a sign-in with a provider: one of SignedIn,
// SecondFactorChallenge, and SignUp.
type ExternalLogin struct {
	// ReturnTo is the return target of BeginExternalLogin.
	ReturnTo string
	// SignedIn is the session of the linked user.
	SignedIn *SignedIn
	// SecondFactorChallenge is the challenge of a linked user with TOTP, which CompleteSignIn
	// takes with the code.
	SecondFactorChallenge string
	// SignUp asks the user to sign up: no user links the account at the provider.
	SignUp *ExternalSignUpView
}

// FinishExternalLogin completes a sign-in with a provider.
//
// It fails with a *kernel.RateLimitedError when too many sign-ins failed from the client
// (LoginIPLimit); with ErrExternalLoginExpired when the cookie or the state is missing, does not
// match, expired, or was used; with ErrLoginProviderUnavailable for a provider that users cannot
// sign in with; with an error wrapping ErrExternalLoginFailed when the user cancelled at the
// provider, the provider answered an error, or its ID token does not verify; with
// ErrExternalLoginDenied when the account does not meet the provider's conditions; with
// ErrInvalidCredentials when the linked user's account is not active; and with
// ErrExternalAccountNotLinked when no user links the account and nobody may sign up with it.
// A state that does not match, an ID token that does not verify, and an inactive account count
// against the client's limit.
func (s *Service) FinishExternalLogin(ctx context.Context, in ExternalLoginInput) (ExternalLogin, error) {
	if err := s.needExternalLoginDeps(); err != nil {
		return ExternalLogin{}, err
	}
	ipKey := kernel.ClientKey(in.IP)
	if err := s.checkLoginLimits(ctx, ipKey, ""); err != nil {
		return ExternalLogin{}, err
	}
	// fail counts a failure against the client's limit.
	fail := func(err error) error {
		if rerr := s.recordLoginFailure(ctx, ipKey, ""); rerr != nil {
			return rerr
		}
		return err
	}
	var t externalLoginToken
	if err := s.openJSON(ctx, in.Cookie, externalLoginAAD, ExternalLoginTTL, &t); err != nil {
		if errors.Is(err, ErrExternalLoginExpired) {
			return ExternalLogin{}, fail(err)
		}
		return ExternalLogin{}, err
	}
	if t.ProviderID != in.ProviderID || in.State == "" || subtle.ConstantTimeCompare([]byte(t.State), []byte(in.State)) != 1 {
		return ExternalLogin{}, fail(ErrExternalLoginExpired)
	}
	// The state works once; the mark outlives the cookie.
	digest := sha256.Sum256([]byte(t.State))
	n, err := s.d.KV.IncrBy(ctx, ExternalLoginNamespace, "used:"+hex.EncodeToString(digest[:]), 1, ExternalLoginTTL+2*time.Minute)
	if err != nil {
		return ExternalLogin{}, fmt.Errorf("identity: record a used external sign-in state: %w", err)
	}
	if n != 1 {
		return ExternalLogin{}, fail(ErrExternalLoginExpired)
	}
	out := ExternalLogin{ReturnTo: t.ReturnTo}
	p, err := s.enabledLoginProvider(ctx, t.ProviderID)
	if err != nil {
		return out, err
	}
	if in.Error != "" || in.Code == "" {
		return out, fmt.Errorf("%w: provider %s answered %q", ErrExternalLoginFailed, p.ID, in.Error)
	}
	var tok IDToken
	if p.Settings.Kind == identity.LoginProviderMastodon {
		tok, err = s.mastodonAccount(ctx, p, in.Code, t)
	} else {
		tok, err = s.oidcIDToken(ctx, p, in.Code, t)
	}
	if errors.Is(err, ErrInvalidIDToken) {
		return out, fail(fmt.Errorf("%w: %w", ErrExternalLoginFailed, err))
	}
	if err != nil {
		return out, err
	}
	if !meetsClaims(p.Settings.Claims, tok.Claims) {
		return out, ErrExternalLoginDenied
	}
	link, err := s.d.Repository.FindLinkedExternalIdentity(ctx, p.ID, tok.Subject)
	switch {
	case errors.Is(err, ErrNotFound):
		view, err := s.offerExternalSignUp(ctx, p, tok)
		if err != nil {
			return out, err
		}
		out.SignUp = &view
		return out, nil
	case err != nil:
		return out, err
	}
	if err := s.checkActive(ctx, link.UserID); err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return out, fail(err)
		}
		return out, err
	}
	switch _, err := s.d.Repository.FindActiveTOTPEnrollment(ctx, link.UserID); {
	case err == nil:
		// The second factor follows the account's settings, whatever the provider asked for
		// (ADR-0106 §3).
		challenge, err := s.newChallenge(ctx, link.UserID)
		if err != nil {
			return out, err
		}
		out.SecondFactorChallenge = challenge
		return out, nil
	case !errors.Is(err, ErrNotFound):
		return out, err
	}
	session, err := s.prepareSession(ctx, link.UserID, in.IP, in.UserAgent, in.CurrentToken, s.d.Clock.Now())
	if err != nil {
		return out, err
	}
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.writeSession(ctx, tx, session) }); err != nil {
		return out, err
	}
	signedIn := session.signedIn()
	out.SignedIn = &signedIn
	return out, nil
}

// oidcIDToken exchanges code for the tokens of an OpenID Connect provider and returns its
// verified ID token. It fails with an error wrapping ErrInvalidIDToken for a token that does not
// verify or has no usable subject, and with one wrapping ErrExternalLoginFailed when the
// provider cannot be reached or refuses the code.
func (s *Service) oidcIDToken(ctx context.Context, p identity.LoginProvider, code string, t externalLoginToken) (IDToken, error) {
	op, err := s.d.OIDC.Discover(ctx, p.Settings.Issuer)
	if err != nil {
		return IDToken{}, fmt.Errorf("%w: provider %s: %w", ErrExternalLoginFailed, p.ID, err)
	}
	secret, err := s.loginProviderClientSecret(ctx, p.ID)
	if err != nil {
		return IDToken{}, err
	}
	raw, err := s.exchangeCode(ctx, op.TokenEndpoint, p.Settings.ClientID, secret, code, s.externalCallbackURL(p.ID), t.Verifier)
	if err != nil {
		return IDToken{}, fmt.Errorf("%w: provider %s: %w", ErrExternalLoginFailed, p.ID, err)
	}
	tok, err := s.d.OIDC.VerifyIDToken(ctx, op, raw, IDTokenCheck{ClientID: p.Settings.ClientID, Nonce: t.Nonce})
	if err != nil {
		if errors.Is(err, ErrInvalidIDToken) {
			return IDToken{}, err
		}
		return IDToken{}, fmt.Errorf("%w: provider %s: %w", ErrExternalLoginFailed, p.ID, err)
	}
	if err := identity.ValidateExternalSubject(tok.Subject); err != nil {
		return IDToken{}, fmt.Errorf("%w: %w", ErrInvalidIDToken, err)
	}
	return tok, nil
}

// loginProviderClientSecret returns the current client secret of a provider, unsealed.
func (s *Service) loginProviderClientSecret(ctx context.Context, providerID string) (string, error) {
	sec, err := s.d.Repository.FindLoginProviderSecret(ctx, providerID)
	if err != nil {
		return "", fmt.Errorf("identity: the client secret of login provider %s: %w", providerID, err)
	}
	plain, err := s.d.Sealer.Open(ctx, sec.Sealed, kernel.AAD(sec.ID, loginProviderSecretColumn))
	if err != nil {
		return "", fmt.Errorf("identity: open the client secret of login provider %s: %w", providerID, err)
	}
	return string(plain), nil
}

// exchangeCode redeems an authorization code at a token endpoint (RFC 6749 §4.1.3, RFC 7636
// §4.5) with the client's credentials in HTTP Basic authentication (client_secret_basic, the
// default of OpenID Connect Core 1.0 §9), and returns the ID token. The other tokens of the
// answer are dropped: the server keeps none of them (ADR-0106 §3).
func (s *Service) exchangeCode(ctx context.Context, endpoint, clientID, secret, code, redirectURI, verifier string) (string, error) {
	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI}, "code_verifier": {verifier},
	}
	// RFC 6749 §2.3.1: the credentials are form-encoded before Basic encodes them.
	basic := base64.StdEncoding.EncodeToString([]byte(url.QueryEscape(clientID) + ":" + url.QueryEscape(secret)))
	resp, err := s.d.HTTP.Do(ctx, &kernel.HTTPRequest{
		Method: http.MethodPost, URL: endpoint, Purpose: kernel.HTTPPurposeLoginProvider, NoRedirects: true,
		Header: http.Header{
			"Content-Type":  {"application/x-www-form-urlencoded"},
			"Accept":        {"application/json"},
			"Authorization": {"Basic " + basic},
		},
		Body: []byte(form.Encode()),
	})
	if err != nil {
		return "", err
	}
	body, err := resp.ReadAll()
	if err != nil {
		return "", err
	}
	var answer struct {
		IDToken string `json:"id_token"`
		Error   string `json:"error"`
	}
	_ = json.Unmarshal(body, &answer) // an answer that is not JSON has neither
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint answered %d %q", resp.StatusCode, answer.Error)
	}
	if answer.IDToken == "" {
		return "", errors.New("token endpoint answered no id_token")
	}
	return answer.IDToken, nil
}

// meetsClaims reports whether the claims of an ID token meet the conditions of a provider: each
// claim has one of its allowed values (ADR-0106 §4). A string claim is compared as it is, an
// array of strings by its members, and any other value by its JSON text (true, 42).
func meetsClaims(conds []identity.ClaimCondition, claims map[string]json.RawMessage) bool {
	for _, c := range conds {
		raw, ok := claims[c.Claim]
		if !ok || !slices.ContainsFunc(claimValues(raw), func(v string) bool { return slices.Contains(c.Values, v) }) {
			return false
		}
	}
	return true
}

func claimValues(raw json.RawMessage) []string {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{one}
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return many
	}
	return []string{strings.TrimSpace(string(raw))}
}

// ExternalSignUpView is a sign-up with an account at a provider, for the registration form.
type ExternalSignUpView struct {
	// Ticket is the sealed sign-up, which SignUpExternal takes back.
	Ticket       string
	ProviderID   string
	ProviderName string
	// Email is the provider's address of the account, "" when it gave none; EmailVerified tells
	// that it verified the address.
	Email         string
	EmailVerified bool
	// Username is a suggestion from the provider's preferred_username, "" for none.
	Username string
}

// offerExternalSignUp returns the sign-up of an account that no user links, or
// ErrExternalAccountNotLinked when nobody may sign up with it.
func (s *Service) offerExternalSignUp(ctx context.Context, p identity.LoginProvider, tok IDToken) (ExternalSignUpView, error) {
	if !p.Settings.AllowSignup {
		return ExternalSignUpView{}, ErrExternalAccountNotLinked
	}
	switch mode, err := s.registrationMode(ctx); {
	case err != nil:
		return ExternalSignUpView{}, err
	case mode == identity.RegistrationClosed:
		return ExternalSignUpView{}, ErrExternalAccountNotLinked
	}
	t := externalSignUpToken{
		IssuedAt: s.d.Clock.Now().UnixMilli(), ProviderID: p.ID, Subject: tok.Subject,
		Username: suggestedUsername(tok.Claims["preferred_username"]),
	}
	if identity.ValidateEmail(tok.Email) == nil {
		t.Email, t.EmailVerified = tok.Email, tok.EmailVerified
	}
	ticket, err := s.sealJSON(ctx, t, externalSignUpAAD)
	if err != nil {
		return ExternalSignUpView{}, err
	}
	return externalSignUpView(ticket, p, t), nil
}

func externalSignUpView(ticket string, p identity.LoginProvider, t externalSignUpToken) ExternalSignUpView {
	return ExternalSignUpView{
		Ticket: ticket, ProviderID: p.ID, ProviderName: p.Settings.DisplayName,
		Email: t.Email, EmailVerified: t.EmailVerified, Username: t.Username,
	}
}

// suggestedUsername returns the provider's preferred_username when it is a valid username here,
// in lower case, and "" otherwise.
func suggestedUsername(raw json.RawMessage) string {
	var name string
	if raw == nil || json.Unmarshal(raw, &name) != nil {
		return ""
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if account.ValidateUsername(name) != nil {
		return ""
	}
	return name
}

// openExternalSignUp opens a sign-up ticket and returns it with its provider, which must still
// let users sign up.
func (s *Service) openExternalSignUp(ctx context.Context, ticket string) (externalSignUpToken, identity.LoginProvider, error) {
	if err := s.needExternalLoginDeps(); err != nil {
		return externalSignUpToken{}, identity.LoginProvider{}, err
	}
	var t externalSignUpToken
	if err := s.openJSON(ctx, ticket, externalSignUpAAD, ExternalSignUpTTL, &t); err != nil {
		return externalSignUpToken{}, identity.LoginProvider{}, err
	}
	p, err := s.enabledLoginProvider(ctx, t.ProviderID)
	if err != nil {
		return externalSignUpToken{}, identity.LoginProvider{}, err
	}
	if !p.Settings.AllowSignup {
		return externalSignUpToken{}, identity.LoginProvider{}, ErrLoginProviderUnavailable
	}
	return t, p, nil
}

// ExternalSignUp returns the sign-up of a ticket, for the registration form to show again. It
// fails with ErrExternalLoginExpired for a ticket that is malformed or expired, and with
// ErrLoginProviderUnavailable when its provider no longer lets users sign up.
func (s *Service) ExternalSignUp(ctx context.Context, ticket string) (ExternalSignUpView, error) {
	t, p, err := s.openExternalSignUp(ctx, ticket)
	if err != nil {
		return ExternalSignUpView{}, err
	}
	return externalSignUpView(ticket, p, t), nil
}

// ExternalSignUpInput is a sign-up from the registration form of an account at a provider.
type ExternalSignUpInput struct {
	// Ticket is ExternalSignUpView's.
	Ticket string
	// Username, Email, Locale, InviteCode, Reason, IP, UserAgent, CurrentToken, and
	// HumanCheckToken are as in SignUpInput.
	Username        string
	Email           string
	Locale          string
	InviteCode      string
	Reason          string
	IP              string
	UserAgent       string
	CurrentToken    string
	HumanCheckToken string
}

// SignUpExternal registers a user without a password who signs in with the account at a
// provider of the ticket, as SignUp does in every other way: the registration mode, the
// CAPTCHA, the sign-up block list, the invite, and the session. The address is verified at once
// when it is the one that the provider verified (email_verified), and by the verification email
// otherwise. An address that another user has fails with ErrEmailTaken: a matching address
// never links that user (ADR-0106 §3.5).
//
// It fails with the errors of SignUp but those of the password, with ErrExternalLoginExpired for
// a ticket that is malformed or expired, with ErrLoginProviderUnavailable when its provider no
// longer lets users sign up, and with ErrExternalIdentityLinked when a user links the account
// already (the ticket was used).
func (s *Service) SignUpExternal(ctx context.Context, in ExternalSignUpInput) (SignedUp, error) {
	t, p, err := s.openExternalSignUp(ctx, in.Ticket)
	if err != nil {
		return SignedUp{}, err
	}
	// A used ticket fails before anything else; the link's uniqueness still decides a race.
	switch _, err := s.d.Repository.FindLinkedExternalIdentity(ctx, p.ID, t.Subject); {
	case err == nil:
		return SignedUp{}, ErrExternalIdentityLinked
	case !errors.Is(err, ErrNotFound):
		return SignedUp{}, err
	}
	ext := &externalSignUp{
		identity: identity.ExternalIdentity{ID: s.d.IDs.NewID(), ProviderID: p.ID, Subject: t.Subject},
		linkID:   s.d.IDs.NewID(),
		emailVerified: t.EmailVerified && t.Email != "" &&
			identity.NormalizeEmail(t.Email) == identity.NormalizeEmail(strings.TrimSpace(in.Email)),
	}
	return s.signUp(ctx, SignUpInput{
		Username: in.Username, Email: strings.TrimSpace(in.Email), Locale: in.Locale, InviteCode: in.InviteCode, Reason: in.Reason,
		IP: in.IP, UserAgent: in.UserAgent, CurrentToken: in.CurrentToken, HumanCheckToken: in.HumanCheckToken,
	}, ext)
}

// writeExternalSignUp links a new user to the account at the provider and, when the provider
// verified the user's address, marks it verified.
func (s *Service) writeExternalSignUp(ctx context.Context, tx uow.Tx, ext externalSignUp, userID, emailID string, now time.Time) error {
	x := ext.identity
	x.CreatedAt = now
	x, err := s.d.Repository.EnsureExternalIdentity(ctx, tx, x)
	if err != nil {
		return err
	}
	link := identity.ExternalIdentityLink{ID: ext.linkID, ExternalIdentityID: x.ID, UserID: userID, LinkedAt: now}
	if err := s.d.Repository.InsertExternalIdentityLink(ctx, tx, x, link); err != nil {
		return err
	}
	if !ext.emailVerified {
		return nil
	}
	return s.d.Repository.InsertEmailVerification(ctx, tx, identity.EmailVerification{EmailAddressID: emailID, VerifiedAt: now})
}

// randomToken returns 256 random bits in base64url.
func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	return base64.RawURLEncoding.EncodeToString(b)
}

// sealJSON seals v as JSON for the browser to keep.
func (s *Service) sealJSON(ctx context.Context, v any, aad []byte) (string, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sealed, err := s.d.Sealer.Seal(ctx, plain, aad)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// openJSON opens what sealJSON sealed into v, whose iat must be younger than ttl. It fails with
// ErrExternalLoginExpired for a value that is malformed, sealed for another purpose, or expired.
func (s *Service) openJSON(ctx context.Context, sealed string, aad []byte, ttl time.Duration, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil || sealed == "" {
		return ErrExternalLoginExpired
	}
	plain, err := s.d.Sealer.Open(ctx, b, aad)
	if errors.Is(err, kernel.ErrDecrypt) || errors.Is(err, kernel.ErrUnknownKey) {
		return ErrExternalLoginExpired
	}
	if err != nil {
		return err
	}
	var iat struct {
		IssuedAt int64 `json:"iat"`
	}
	if json.Unmarshal(plain, &iat) != nil || json.Unmarshal(plain, v) != nil {
		return ErrExternalLoginExpired
	}
	issued := time.UnixMilli(iat.IssuedAt)
	now := s.d.Clock.Now()
	if !now.Before(issued.Add(ttl)) || now.Before(issued.Add(-time.Minute)) {
		return ErrExternalLoginExpired
	}
	return nil
}
