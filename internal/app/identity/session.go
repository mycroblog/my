// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// ErrInvalidCredentials reports a failed sign-in. It is the same error whether the email
// address is unknown or the password is wrong, so that callers cannot reveal which.
var ErrInvalidCredentials = errors.New("identity: invalid email address or password")

// ErrInvalidSession reports a session token that is unknown, revoked, or expired.
var ErrInvalidSession = errors.New("identity: invalid session")

// Session defaults (ADR-0019 §4 leaves the lifetime open; it matches the refresh token's).
const (
	DefaultSessionTTL = 30 * 24 * time.Hour
	// SessionActivityInterval is how stale session_activity may get before Authenticate
	// updates it, so that reads do not turn into a write per request (data-model §4.1).
	SessionActivityInterval = time.Hour
	// SessionTokenPrefix marks session tokens for secret scanners (ADR-0019 §4).
	SessionTokenPrefix = "my_ses_"
)

// Login rate limits (ADR-0019 §5). Only failed attempts count.
var (
	LoginIPLimit      = kernel.Limit{Name: "login.ip", Max: 5, Window: time.Minute}
	LoginAccountLimit = kernel.Limit{Name: "login.account", Max: 10, Window: time.Hour}
)

// NewToken returns a new random token with prefix (256 bits, base64url) and its SHA-256
// digest, which is all that is stored (ADR-0019 §4).
func NewToken(prefix string) (token string, hash []byte) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	token = prefix + base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token)
}

// HashToken returns the SHA-256 digest of a token, the key it is looked up by.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// SignInInput is a sign-in attempt from a browser.
type SignInInput struct {
	Email    string
	Password string
	// IP is the client address ("" when unknown); it keys a rate limit and, with UserAgent,
	// is recorded as the session's origin. An unknown or unparsable address is not recorded
	// and does not stop the sign-in.
	IP        string
	UserAgent string
	// CurrentToken is the session token the browser already holds, if any. Signing in revokes
	// it, so that the browser holds exactly the new session.
	CurrentToken string
	// HumanCheckToken is the token that the CAPTCHA widget put into the form; it is checked
	// only when Deps.HumanVerifier is set.
	HumanCheckToken string
}

// SignedIn is the result of SignIn.
type SignedIn struct {
	// Token is the new session token for the cookie; it is not stored anywhere.
	Token   string
	Session identity.Session
}

// SignIn checks an email address and password and starts a new session with a new token, so
// that a token planted before the sign-in is never promoted (session fixation, ADR-0019 §4).
//
// It fails with ErrInvalidCredentials for an unknown address, a wrong password, or a user whose
// local account is not active (data-model §5.1), after the same amount of password hashing, and with a *kernel.RateLimitedError when too
// many attempts failed from the client's IP address or for the email address (ADR-0019 §5).
//
// With a HumanVerifier, the CAPTCHA token is checked after the rate limits and before the
// password (ADR-0042 §3). A token that fails the check fails the sign-in with an error wrapping
// kernel.ErrHumanCheckFailed and counts as a failure of the client's IP address only, so that
// nobody can lock another person's account with bad tokens. When the service cannot check the
// token (kernel.ErrHumanCheckUnavailable), the sign-in goes on without the check (fail open,
// ADR-0042 §4), so that an outage of the service or a wrong key does not lock everyone out.
//
// While registration and sign-in are limited to the login providers, it fails with
// ErrLoginProvidersOnly before it looks at anything else.
//
// For a user with two-factor authentication, a right password starts no session: SignIn fails
// with a *SecondFactorRequiredError (ErrSecondFactorRequired) whose challenge CompleteSignIn
// takes with the code. The CAPTCHA belongs to this first step only.
func (s *Service) SignIn(ctx context.Context, in SignInInput) (SignedIn, error) {
	if err := s.refuseWhileLoginProvidersOnly(ctx); err != nil {
		return SignedIn{}, err
	}
	if s.d.RateLimiter == nil {
		return SignedIn{}, errors.New("identity: SignIn needs a RateLimiter")
	}
	ipKey := kernel.ClientKey(in.IP)
	// No stored address is longer, so the cut only bounds the rate-limit key.
	accountKey := truncate(identity.NormalizeEmail(in.Email), identity.MaxEmailLength)
	if err := s.checkLoginLimits(ctx, ipKey, accountKey); err != nil {
		return SignedIn{}, err
	}
	switch err := s.checkHuman(ctx, in.HumanCheckToken, kernel.HumanCheckActionSignIn); {
	case errors.Is(err, kernel.ErrHumanCheckFailed):
		if ipKey != "" {
			if rerr := s.d.RateLimiter.Record(ctx, LoginIPLimit, ipKey); rerr != nil {
				return SignedIn{}, rerr
			}
		}
		return SignedIn{}, err
	case err != nil:
		s.logger().WarnContext(ctx, "the CAPTCHA service could not check a sign-in; letting it through", slog.Any("error", err))
	}
	cred, err := s.verifyPassword(ctx, accountKey, in.Password)
	if err == nil {
		err = s.checkActive(ctx, cred.cred.UserID)
	}
	if errors.Is(err, ErrInvalidCredentials) {
		if rerr := s.recordLoginFailure(ctx, ipKey, accountKey); rerr != nil {
			return SignedIn{}, rerr
		}
		return SignedIn{}, err
	}
	if err != nil {
		return SignedIn{}, err
	}

	now := s.d.Clock.Now()
	var rehashed *identity.PasswordCredential
	if cred.needsRehash {
		h, err := s.d.Passwords.Hash(ctx, in.Password)
		if err != nil {
			return SignedIn{}, err
		}
		rehashed = &identity.PasswordCredential{ID: s.d.IDs.NewID(), UserID: cred.cred.UserID, Hash: h, SetAt: now}
	}
	writeRehash := func(tx uow.Tx) error {
		if rehashed == nil {
			return nil
		}
		return s.d.Repository.InsertPasswordCredential(ctx, tx, *rehashed)
	}
	switch _, err := s.d.Repository.FindActiveTOTPEnrollment(ctx, cred.cred.UserID); {
	case err == nil:
		// The password was right; the session waits for the second factor (CompleteSignIn).
		challenge, err := s.newChallenge(ctx, cred.cred.UserID, challengeLocalCredential)
		if err != nil {
			return SignedIn{}, err
		}
		if rehashed != nil {
			if err := s.d.UnitOfWork.Tx(ctx, writeRehash); err != nil {
				return SignedIn{}, err
			}
		}
		return SignedIn{}, &SecondFactorRequiredError{Challenge: challenge}
	case !errors.Is(err, ErrNotFound):
		return SignedIn{}, err
	}
	session, err := s.prepareSession(ctx, cred.cred.UserID, in.IP, in.UserAgent, in.CurrentToken, now)
	if err != nil {
		return SignedIn{}, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := writeRehash(tx); err != nil {
			return err
		}
		return s.writeSession(ctx, tx, session)
	})
	if err != nil {
		return SignedIn{}, err
	}
	return session.signedIn(), nil
}

// newSession is a session prepared before its transaction: its token and row, and the
// browser's earlier session that it replaces.
type newSession struct {
	token          string
	session        identity.Session
	origin         identity.SessionOrigin
	previous       string // ID of the session to revoke as replaced, or ""
	previousUserID string // the user of that session
}

func (n newSession) signedIn() SignedIn { return SignedIn{Token: n.token, Session: n.session} }

// prepareSession returns a new session of userID with a new token and its origin (ip and
// userAgent). currentToken is the session token the browser holds, if any; the new session
// replaces it.
func (s *Service) prepareSession(ctx context.Context, userID, ip, userAgent, currentToken string, now time.Time) (newSession, error) {
	token, hash := NewToken(SessionTokenPrefix)
	n := newSession{token: token, session: identity.Session{
		ID:        s.d.IDs.NewID(),
		UserID:    userID,
		TokenHash: hash,
		CreatedAt: now,
		ExpiresAt: now.Add(s.sessionTTL()),
	}}
	n.origin = identity.SessionOrigin{
		SessionID: n.session.ID,
		UserID:    userID,
		IP:        normalizeIP(ip),
		UserAgent: truncate(userAgent, identity.MaxUserAgentLength),
		CreatedAt: now,
	}
	if currentToken != "" {
		switch old, _, err := s.d.Repository.FindUnrevokedSession(ctx, HashToken(currentToken)); {
		case err == nil:
			n.previous, n.previousUserID = old.ID, old.UserID
		case !errors.Is(err, ErrNotFound):
			return newSession{}, err
		}
	}
	return n, nil
}

// writeSession writes a prepared session and its origin, revoking the one it replaces.
func (s *Service) writeSession(ctx context.Context, tx uow.Tx, n newSession) error {
	now := n.session.CreatedAt
	if n.previous != "" {
		// The families of the replaced session end with it: this browser gets a new one, and the
		// old one would stay usable and show as another browser (ADR-0136 §5).
		if err := s.d.Repository.LockTokenGrants(ctx, tx, n.previousUserID); err != nil {
			return err
		}
		rev := identity.SessionRevocation{SessionID: n.previous, Reason: identity.RevokedReplaced, RevokedAt: now}
		if err := s.d.Repository.InsertSessionRevocation(ctx, tx, rev); err != nil {
			return err
		}
		if err := s.d.Repository.RevokeFirstPartyTokensOfSession(ctx, tx, n.previous, identity.TokenRevokedSessionReplaced, now); err != nil {
			return err
		}
	}
	if err := s.d.Repository.InsertSession(ctx, tx, n.session); err != nil {
		return err
	}
	if !n.origin.Empty() {
		if err := s.d.Repository.InsertSessionOrigin(ctx, tx, n.origin); err != nil {
			return err
		}
	}
	return s.d.Repository.UpsertSessionActivity(ctx, tx, identity.SessionActivity{SessionID: n.session.ID, LastSeenAt: now})
}

func (s *Service) sessionTTL() time.Duration {
	if s.d.SessionTTL > 0 {
		return s.d.SessionTTL
	}
	return DefaultSessionTTL
}

func (s *Service) checkLoginLimits(ctx context.Context, ipKey, accountKey string) error {
	var wait time.Duration
	for _, c := range []struct {
		lim kernel.Limit
		key string
	}{{LoginIPLimit, ipKey}, {LoginAccountLimit, accountKey}} {
		if c.key == "" {
			continue
		}
		d, err := s.d.RateLimiter.Check(ctx, c.lim, c.key)
		if err != nil {
			return err
		}
		if !d.Allowed {
			wait = max(wait, d.RetryAfter)
		}
	}
	if wait > 0 {
		return &kernel.RateLimitedError{RetryAfter: wait}
	}
	return nil
}

// checkHuman checks the CAPTCHA token of action with the HumanVerifier, and nothing without
// one. It returns nil, an error wrapping kernel.ErrHumanCheckFailed, or an error wrapping
// kernel.ErrHumanCheckUnavailable: any other error of the verifier counts as the latter.
func (s *Service) checkHuman(ctx context.Context, token, action string) error {
	if s.d.HumanVerifier == nil {
		return nil
	}
	err := s.d.HumanVerifier.Verify(ctx, token, action)
	if err == nil || errors.Is(err, kernel.ErrHumanCheckFailed) || errors.Is(err, kernel.ErrHumanCheckUnavailable) {
		return err
	}
	return fmt.Errorf("%w: %w", kernel.ErrHumanCheckUnavailable, err)
}

func (s *Service) logger() *slog.Logger {
	if s.d.Logger != nil {
		return s.d.Logger
	}
	return slog.Default()
}

func (s *Service) recordLoginFailure(ctx context.Context, ipKey, accountKey string) error {
	if ipKey != "" {
		if err := s.d.RateLimiter.Record(ctx, LoginIPLimit, ipKey); err != nil {
			return err
		}
	}
	if accountKey != "" {
		return s.d.RateLimiter.Record(ctx, LoginAccountLimit, accountKey)
	}
	return nil
}

type verifiedPassword struct {
	cred        identity.PasswordCredential
	needsRehash bool
}

// verifyPassword returns the current credential of the user with the email address when
// password matches it. When the user or their password does not exist, it verifies against a
// dummy hash instead, so that the response time does not tell (ADR-0019 §3).
func (s *Service) verifyPassword(ctx context.Context, normalizedEmail, password string) (verifiedPassword, error) {
	var cred identity.PasswordCredential
	userID, err := s.d.Repository.FindUserIDByEmail(ctx, normalizedEmail)
	if err == nil {
		cred, err = s.d.Repository.FindCurrentPasswordCredential(ctx, userID)
	}
	if errors.Is(err, ErrNotFound) {
		dummy, err := s.dummyHash(ctx)
		if err != nil {
			return verifiedPassword{}, err
		}
		if _, _, err := s.d.Passwords.Verify(ctx, password, dummy); err != nil {
			return verifiedPassword{}, err
		}
		return verifiedPassword{}, ErrInvalidCredentials
	}
	if err != nil {
		return verifiedPassword{}, err
	}
	ok, rehash, err := s.d.Passwords.Verify(ctx, password, cred.Hash)
	if err != nil {
		return verifiedPassword{}, fmt.Errorf("identity: verify the password of user %s: %w", userID, err)
	}
	if !ok {
		return verifiedPassword{}, ErrInvalidCredentials
	}
	return verifiedPassword{cred: cred, needsRehash: rehash}, nil
}

// checkActive fails with ErrInvalidCredentials unless the user's local account is active, so
// that a deleted or deletion-scheduled account cannot sign in and the response does not tell that
// the password was right. An account that moderation forbids to log in (disable_login, suspend,
// ban) signs in to the server-rendered screens only, which show the user the actions that keep
// them out (Access.LoginRestricted); it gets no access token. So does an account whose deletion
// is scheduled while a ban is in effect on it, until the deletion, so that the user can appeal the
// ban (FR-707), and an account that its owner deactivated, until the date of its deletion, so that
// the user can reactivate or delete it (Access.Deactivated, ADR-0145 §4).
func (s *Service) checkActive(ctx context.Context, userID string) error {
	st, err := s.d.Accounts.UserLifecycle(ctx, userID)
	if errors.Is(err, account.ErrNotFound) {
		return ErrInvalidCredentials
	}
	if err != nil {
		return err
	}
	if st.Active() {
		return nil
	}
	if st.Deactivated() && !st.DeactivationExpired(s.d.Clock.Now()) {
		return nil
	}
	if st.DeletionScheduled() {
		switch banned, err := s.banned(ctx, st.AccountID); {
		case err != nil:
			return err
		case banned:
			return nil
		}
	}
	return ErrInvalidCredentials
}

// dummyHash returns a hash of a random password with the current parameters, made once.
func (s *Service) dummyHash(ctx context.Context) (string, error) {
	s.dummyMu.Lock()
	defer s.dummyMu.Unlock()
	if s.dummy != "" {
		return s.dummy, nil
	}
	pw, _ := NewToken("")
	h, err := s.d.Passwords.Hash(ctx, pw)
	if err != nil {
		return "", err
	}
	s.dummy = h
	return h, nil
}

// Authenticate returns the session of a token. It fails with ErrInvalidSession when the token
// is unknown, revoked, or expired. It records the session's activity at most once per
// SessionActivityInterval.
func (s *Service) Authenticate(ctx context.Context, token string) (identity.Session, error) {
	if token == "" {
		return identity.Session{}, ErrInvalidSession
	}
	session, activity, err := s.d.Repository.FindUnrevokedSession(ctx, HashToken(token))
	if errors.Is(err, ErrNotFound) {
		return identity.Session{}, ErrInvalidSession
	}
	if err != nil {
		return identity.Session{}, err
	}
	now := s.d.Clock.Now()
	if session.Expired(now) {
		return identity.Session{}, ErrInvalidSession
	}
	if now.Sub(activity.LastSeenAt) >= SessionActivityInterval {
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return s.d.Repository.UpsertSessionActivity(ctx, tx, identity.SessionActivity{SessionID: session.ID, LastSeenAt: now})
		})
		if err != nil {
			return identity.Session{}, err
		}
	}
	return session, nil
}

// SignOut revokes the session of a token. Signing out with an unknown or already revoked token
// does nothing.
func (s *Service) SignOut(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	session, _, err := s.d.Repository.FindUnrevokedSession(ctx, HashToken(token))
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	rev := identity.SessionRevocation{SessionID: session.ID, Reason: identity.RevokedLogout, RevokedAt: s.d.Clock.Now()}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.InsertSessionRevocation(ctx, tx, rev) })
}

// truncate cuts s to at most n bytes without splitting a UTF-8 sequence.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xc0 == 0x80 {
		n--
	}
	return s[:n]
}
