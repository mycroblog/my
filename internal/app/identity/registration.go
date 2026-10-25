// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
)

// RegistrationRepository is the persistence of registration, invites, email verification, and
// one-time tokens (data-model §4.1). It is part of Repository.
type RegistrationRepository interface {
	InsertRegistrationApplication(ctx context.Context, tx uow.Tx, a identity.RegistrationApplication) error
	InsertRegistrationDecision(ctx context.Context, tx uow.Tx, d identity.RegistrationDecision) error
	// UpsertRegistrationState sets the current registration state of s.UserID.
	UpsertRegistrationState(ctx context.Context, tx uow.Tx, s identity.UserRegistrationState) error
	// TransitionPendingRegistrationState sets the registration state of s.UserID only while it
	// is pending, and reports whether it did.
	TransitionPendingRegistrationState(ctx context.Context, tx uow.Tx, s identity.UserRegistrationState) (bool, error)
	FindRegistrationApplication(ctx context.Context, userID string) (identity.RegistrationApplication, error)
	FindRegistrationState(ctx context.Context, userID string) (identity.UserRegistrationState, error)
	// ListRegistrations returns the users in a registration state with their applications,
	// longest in the state first, after the position after (all of them for the zero
	// position), at most limit of them.
	ListRegistrations(ctx context.Context, state identity.RegistrationState, after RegistrationPosition, limit int) ([]RegistrationEntry, error)
	// CountRegistrations counts the users in a registration state.
	CountRegistrations(ctx context.Context, state identity.RegistrationState) (int64, error)

	// InsertInvite fails with an error wrapping ErrInviteCodeTaken when another invite has the code.
	InsertInvite(ctx context.Context, tx uow.Tx, i identity.Invite) error
	FindInviteByCode(ctx context.Context, code string) (identity.Invite, error)
	FindInviteByID(ctx context.Context, id string) (identity.Invite, error)
	// InviteUsage returns how many times an invite was redeemed and whether it is revoked, as
	// the transaction sees it.
	InviteUsage(ctx context.Context, tx uow.Tx, inviteID string) (uses int, revoked bool, err error)
	// InsertInviteRedemption records a use. When another transaction recorded the same Seq
	// first, it fails with an error for which errors.Is(err, uow.ErrRetryable) holds, so that
	// the use case counts again.
	InsertInviteRedemption(ctx context.Context, tx uow.Tx, r identity.InviteRedemption) error
	// InsertInviteRevocation does nothing when the invite already has a revocation.
	InsertInviteRevocation(ctx context.Context, tx uow.Tx, r identity.InviteRevocation) error
	// ListInvites returns invites with their usage, newest first, at most limit of them; a
	// before other than "" keeps the invites whose ID sorts before it.
	ListInvites(ctx context.Context, before string, limit int) ([]InviteStatus, error)

	InsertUserToken(ctx context.Context, tx uow.Tx, t identity.UserToken) error
	FindUserTokenByHash(ctx context.Context, hash []byte) (identity.UserToken, error)
	// UserTokenRedeemed reports whether a token was used.
	UserTokenRedeemed(ctx context.Context, tokenID string) (bool, error)
	// InsertUserTokenRedemption fails with an error wrapping ErrInvalidToken when the token
	// was already redeemed.
	InsertUserTokenRedemption(ctx context.Context, tx uow.Tx, r identity.UserTokenRedemption) error
	// InsertEmailVerification does nothing when the address is already verified.
	InsertEmailVerification(ctx context.Context, tx uow.Tx, v identity.EmailVerification) error
	// FindPrimaryEmail returns a user's primary email address and whether it is verified.
	FindPrimaryEmail(ctx context.Context, userID string) (identity.EmailAddress, bool, error)
}

// RegistrationPosition is the place of a registration in ListRegistrations: the time it
// entered its state and its user. The zero position is before every registration.
type RegistrationPosition struct {
	Since  time.Time
	UserID string
}

// Position returns the place of e in ListRegistrations.
func (e RegistrationEntry) Position() RegistrationPosition {
	return RegistrationPosition{Since: e.State.Since, UserID: e.Application.UserID}
}

// RegistrationEntry is a user's registration as moderators review it.
type RegistrationEntry struct {
	Application identity.RegistrationApplication
	State       identity.UserRegistrationState
	// Email is the user's primary email address and EmailVerified whether it is confirmed.
	Email         string
	EmailVerified bool
	// AccountID and Username are the ID and username of the user's local account, "" when
	// it is gone.
	AccountID string
	Username  string
}

// Errors of the registration use cases.
var (
	// ErrRegistrationClosed reports a sign-up while the registration mode is closed.
	ErrRegistrationClosed = errors.New("identity: registration is closed")
	// ErrSignUpRefused reports a sign-up that a sign-up block with severity reject matches
	// (FR-719). The page does not tell the applicant which block matched, or why.
	ErrSignUpRefused = errors.New("identity: the sign-up is refused")
	// ErrInviteRequired reports a sign-up without an invite code in invite mode.
	ErrInviteRequired = errors.New("identity: an invite is required")
	// ErrInvalidInvite reports an invite code that is unknown, expired, revoked, or used up.
	// It is one error for all four, so that the page does not tell them apart.
	ErrInvalidInvite = errors.New("identity: invalid invite")
	// ErrInviteCodeTaken reports an invite code that another invite already has.
	ErrInviteCodeTaken = errors.New("identity: invite code is taken")
	// ErrInvalidToken reports a one-time token that is unknown, expired, already used, or
	// for another purpose.
	ErrInvalidToken = errors.New("identity: invalid or used token")
	// ErrNotPending reports a decision on a registration that is not waiting for one.
	ErrNotPending = errors.New("identity: registration is not pending")
	// ErrAlreadyVerified reports a request for a verification email for an address that is
	// already verified.
	ErrAlreadyVerified = errors.New("identity: email address is already verified")
)

// Registration limits (ADR-0019 §5, ADR-0019 §4).
var (
	// SignUpIPLimit bounds sign-up attempts per client address; every attempt that passes
	// validation counts.
	SignUpIPLimit = kernel.Limit{Name: "signup.ip", Max: 5, Window: 30 * time.Minute}
	// VerificationEmailLimit bounds the verification emails a user can ask for again.
	VerificationEmailLimit = kernel.Limit{Name: "verify_email.user", Max: 3, Window: time.Hour}
)

// Token lifetimes and prefixes (ADR-0019 §4).
const (
	EmailVerificationTTL = 24 * time.Hour
	// EmailVerificationTokenPrefix marks email verification tokens for secret scanners.
	EmailVerificationTokenPrefix = "my_evt_"
)

// PathVerifyEmail is the path of the page that a verification email links to, with the token
// in the query parameter "token". The authpages package serves it.
const PathVerifyEmail = "/auth/verify_email"

// SignUpInput is a sign-up from the registration page.
type SignUpInput struct {
	Username string
	Email    string
	Password string
	// Locale is the new user's UI locale, a BCP 47 tag; empty means Deps.DefaultLocale.
	Locale string
	// InviteCode is the invite the applicant entered; it is required in invite mode and
	// ignored in the other modes.
	InviteCode string
	// Reason is why the applicant wants to join; it is kept in approval mode and ignored in
	// the other modes.
	Reason string
	// IP is the client address ("" when unknown); it keys SignUpIPLimit and is recorded as the
	// registration's origin and, with UserAgent, the session's. An unknown or unparsable
	// address is not recorded and does not stop the sign-up.
	IP        string
	UserAgent string
	// CurrentToken is the session token the browser already holds, if any; the new user's
	// session replaces it.
	CurrentToken string
	// HumanCheckToken is the token that the CAPTCHA widget put into the form; it is checked
	// only when Deps.HumanVerifier is set.
	HumanCheckToken string
}

// SignedUp is the result of SignUp.
type SignedUp struct {
	Registration
	State identity.RegistrationState
	// Session signs the browser in as the new user.
	Session SignedIn
}

// SignUp registers a user from the registration page according to the registration mode
// (FR-001): open mode approves at once, approval mode leaves the registration pending for a
// moderator, invite mode needs a valid invite and approves at once, and closed mode refuses.
// When email verification is not required (no mailer delivers email, FR-002), open mode
// waits for a moderator as approval mode does. The sign-up block list (FR-719) comes on top:
// a block in effect that matches the email address or the client address refuses the sign-up
// (severity reject) or leaves it pending for a moderator in every mode (require_approval).
//
// In one transaction it writes the user, password, primary email address, preferences, local
// account, application, the system's decision, the registration state, the invite's
// redemption, the job that sends the verification email (JobVerificationEmail), the
// registration's origin, and a session with its origin, and emits RegistrationPending when the
// registration awaits a moderator. The result does not tell whether the email went out: a user
// whose email does not arrive asks for it again (ResendVerificationEmail). The invite's redemption takes the next sequence number, so concurrent sign-ups never
// redeem an invite more than its maximum uses.
//
// It fails with ErrRegistrationClosed, ErrSignUpRefused, ErrInviteRequired, ErrInvalidInvite, ErrEmailTaken,
// account.ErrUsernameTaken, a *kernel.RateLimitedError (SignUpIPLimit), and errors wrapping
// identity.ErrInvalidEmail, identity.ErrInvalidPassword, identity.ErrInvalidReason, or
// account.ErrInvalidUsername for invalid input. A failure writes nothing.
//
// With a HumanVerifier, the CAPTCHA token is checked after SignUpIPLimit, which counts the
// attempt, and before anything else, in every mode and with a valid invite too (ADR-0042 §3).
// A token that fails the check fails the sign-up with an error wrapping
// kernel.ErrHumanCheckFailed, and a service that cannot check it with an error wrapping
// kernel.ErrHumanCheckUnavailable (fail closed, ADR-0042 §4).
func (s *Service) SignUp(ctx context.Context, in SignUpInput) (SignedUp, error) {
	mode, err := s.registrationMode(ctx)
	if err != nil {
		return SignedUp{}, err
	}
	if mode == identity.RegistrationClosed {
		return SignedUp{}, ErrRegistrationClosed
	}
	if err := identity.ValidateEmail(in.Email); err != nil {
		return SignedUp{}, err
	}
	if err := identity.ValidatePassword(in.Password); err != nil {
		return SignedUp{}, err
	}
	if err := account.ValidateUsername(in.Username); err != nil {
		return SignedUp{}, err
	}
	reason := ""
	if mode == identity.RegistrationApproval {
		reason = strings.TrimSpace(in.Reason)
		if err := identity.ValidateRegistrationReason(reason); err != nil {
			return SignedUp{}, err
		}
	}
	code := identity.NormalizeInviteCode(in.InviteCode)
	if mode == identity.RegistrationInvite && code == "" {
		return SignedUp{}, ErrInviteRequired
	}
	if s.d.RateLimiter == nil {
		return SignedUp{}, errors.New("identity: SignUp needs a RateLimiter")
	}
	if key := kernel.ClientKey(in.IP); key != "" {
		d, err := s.d.RateLimiter.Take(ctx, SignUpIPLimit, key)
		if err != nil {
			return SignedUp{}, err
		}
		if !d.Allowed {
			return SignedUp{}, &kernel.RateLimitedError{RetryAfter: d.RetryAfter}
		}
	}
	if err := s.checkHuman(ctx, in.HumanCheckToken, kernel.HumanCheckActionSignUp); err != nil {
		return SignedUp{}, err
	}
	severity, err := s.screenSignup(ctx, in.Email, in.IP)
	if err != nil {
		return SignedUp{}, err
	}
	if severity == appmoderation.SignupReject {
		return SignedUp{}, ErrSignUpRefused
	}
	var invite *identity.Invite
	if mode == identity.RegistrationInvite {
		inv, err := s.d.Repository.FindInviteByCode(ctx, code)
		if errors.Is(err, ErrNotFound) {
			return SignedUp{}, ErrInvalidInvite
		}
		if err != nil {
			return SignedUp{}, err
		}
		if inv.Expired(s.d.Clock.Now()) {
			return SignedUp{}, ErrInvalidInvite
		}
		invite = &inv
	}

	p, err := s.prepareUser(ctx, RegisterInput{Username: in.Username, Password: in.Password, Locale: in.Locale})
	if err != nil {
		return SignedUp{}, err
	}
	now := p.user.CreatedAt
	email := identity.EmailAddress{
		ID: s.d.IDs.NewID(), UserID: p.user.ID, Address: in.Email, AddressNormalized: identity.NormalizeEmail(in.Email), AddedAt: now,
	}
	app := identity.RegistrationApplication{ID: s.d.IDs.NewID(), UserID: p.user.ID, Reason: reason, SubmittedAt: now}
	if invite != nil {
		app.InviteID = invite.ID
	}
	state := identity.UserRegistrationState{UserID: p.user.ID, State: identity.RegistrationPending, Since: now}
	var decision *identity.RegistrationDecision
	approve := mode == identity.RegistrationInvite || (mode == identity.RegistrationOpen && s.d.RequireEmailVerification)
	if approve && severity != appmoderation.SignupRequireApproval {
		decision = &identity.RegistrationDecision{
			ID: s.d.IDs.NewID(), ApplicationID: app.ID, Decision: identity.DecisionApproved,
			DecidedBy: identity.DecidedBySystem, DecidedAt: now,
		}
		state.State = identity.RegistrationApproved
	}
	verification := s.verificationRequest(p.user.ID, email.ID, now)
	session, err := s.prepareSession(ctx, p.user.ID, in.IP, in.UserAgent, in.CurrentToken, now)
	if err != nil {
		return SignedUp{}, err
	}

	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if invite != nil {
			if err := s.redeemInvite(ctx, tx, *invite, p.user.ID, now); err != nil {
				return err
			}
		}
		if err := s.writeUser(ctx, tx, p); err != nil {
			return err
		}
		repo := s.d.Repository
		if err := repo.InsertEmailAddress(ctx, tx, email); err != nil {
			return err
		}
		if err := repo.InsertPrimaryEmail(ctx, tx, email); err != nil {
			return err
		}
		if err := repo.InsertRegistrationApplication(ctx, tx, app); err != nil {
			return err
		}
		if decision != nil {
			if err := repo.InsertRegistrationDecision(ctx, tx, *decision); err != nil {
				return err
			}
		}
		if err := repo.UpsertRegistrationState(ctx, tx, state); err != nil {
			return err
		}
		if state.State == identity.RegistrationPending {
			if err := tx.Emit(ctx, RegistrationPending{
				UserID: p.user.ID, AccountID: p.acct.Account.ID, Username: p.acct.Local.Username, SubmittedAt: now,
			}); err != nil {
				return err
			}
		} else if err := tx.Emit(ctx, RegistrationApproved{
			UserID: p.user.ID, AccountID: p.acct.Account.ID, Username: p.acct.Local.Username, ApprovedAt: now,
		}); err != nil {
			return err
		}
		if verification != nil {
			if err := s.enqueueVerification(ctx, tx, *verification); err != nil {
				return err
			}
		}
		if ip := normalizeIP(in.IP); ip != "" {
			if err := repo.InsertRegistrationOrigin(ctx, tx, identity.RegistrationOrigin{UserID: p.user.ID, IP: ip, RecordedAt: now}); err != nil {
				return err
			}
		}
		return s.writeSession(ctx, tx, session)
	})
	if err != nil {
		return SignedUp{}, err
	}
	return SignedUp{Registration: p.registration(), State: state.State, Session: session.signedIn()}, nil
}

// redeemInvite records the next use of an invite, failing with ErrInvalidInvite when it is
// revoked or used up. A concurrent redemption of the same sequence number makes the
// transaction retry and count again.
func (s *Service) redeemInvite(ctx context.Context, tx uow.Tx, inv identity.Invite, userID string, now time.Time) error {
	uses, revoked, err := s.d.Repository.InviteUsage(ctx, tx, inv.ID)
	if err != nil {
		return err
	}
	if revoked || inv.Exhausted(uses) {
		return ErrInvalidInvite
	}
	return s.d.Repository.InsertInviteRedemption(ctx, tx, identity.InviteRedemption{
		InviteID: inv.ID, Seq: uses + 1, UserID: userID, RedeemedAt: now,
	})
}

func (s *Service) registrationMode(ctx context.Context) (identity.RegistrationMode, error) {
	if s.d.RegistrationMode == nil {
		return identity.RegistrationClosed, nil
	}
	mode, err := s.d.RegistrationMode(ctx)
	if err != nil {
		return "", fmt.Errorf("instance registration mode: %w", err)
	}
	switch mode {
	case identity.RegistrationOpen, identity.RegistrationApproval, identity.RegistrationInvite:
		return mode, nil
	}
	return identity.RegistrationClosed, nil
}

// RegistrationMode returns the instance's registration mode; an unknown mode reads as closed.
func (s *Service) RegistrationMode(ctx context.Context) (identity.RegistrationMode, error) {
	return s.registrationMode(ctx)
}

// mailContext is what an email to a user names: the localizer of the user's locale, the
// instance's host, and its name for readers of that locale, with the composer that adds the
// common signature and envelope (ADR-0086 §2).
type mailContext struct {
	l          *i18n.Localizer
	host, name string
	composer   kernel.MailComposer
}

// mailContext returns the mailContext of an email to a user whose locale is locale.
func (s *Service) mailContext(ctx context.Context, locale string) (mailContext, error) {
	bundle := s.d.Messages
	if bundle == nil {
		var err error
		if bundle, err = i18n.Load(); err != nil {
			return mailContext{}, err
		}
	}
	users := &i18n.Resolver{Bundle: bundle, UserLocale: func(context.Context) string { return locale }}
	m := mailContext{l: users.Localizer(ctx, ""), host: s.d.BaseURL}
	if u, err := url.Parse(s.d.BaseURL); err == nil && u.Host != "" {
		m.host = u.Host
	}
	var site kernel.MailSite
	if s.d.MailSite != nil {
		site = s.d.MailSite(ctx)
	}
	if site.Name == "" {
		site.Name = m.host
	}
	if site.URL == "" {
		site.URL = s.d.BaseURL
	}
	m.name = site.Name
	m.composer = kernel.MailComposer{Site: site, Words: m.l}
	return m, nil
}

// link returns the URL of path on this server, with token in the query parameter "token" when
// it is not "".
func (s *Service) link(path, token string) string {
	link := strings.TrimSuffix(s.d.BaseURL, "/") + path
	if token != "" {
		link += "?token=" + url.QueryEscape(token)
	}
	return link
}

// VerifyEmail redeems an email verification token and marks its address verified; when the
// address is suppressed by a failure from before the token, it lifts the suppression
// (reverified). It fails
// with ErrInvalidToken when the token is unknown, expired, already used, or for another
// purpose; a token works once.
func (s *Service) VerifyEmail(ctx context.Context, token string) (userID string, err error) {
	if !strings.HasPrefix(token, EmailVerificationTokenPrefix) {
		return "", ErrInvalidToken
	}
	t, err := s.d.Repository.FindUserTokenByHash(ctx, HashToken(token))
	if errors.Is(err, ErrNotFound) {
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", err
	}
	now := s.d.Clock.Now()
	if t.Purpose != identity.PurposeEmailVerification || t.EmailAddressID == "" || t.Expired(now) {
		return "", ErrInvalidToken
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertUserTokenRedemption(ctx, tx, identity.UserTokenRedemption{TokenID: t.ID, RedeemedAt: now}); err != nil {
			return err
		}
		return s.d.Repository.InsertEmailVerification(ctx, tx, identity.EmailVerification{EmailAddressID: t.EmailAddressID, VerifiedAt: now})
	})
	if err != nil {
		return "", err
	}
	// The address received the email of the token, so it receives email again (ADR-0086 §5).
	if err := s.liftAfterVerification(ctx, t.EmailAddressID, t.IssuedAt, now); err != nil {
		return "", err
	}
	return t.UserID, nil
}

// ResendVerificationEmail enqueues a new verification email for a user's primary address
// (JobVerificationEmail). Earlier links keep working until they expire. It fails with ErrAlreadyVerified when the
// address is verified and no hard bounce suppresses it, with ErrNotFound when the user has no primary address, and with a
// *kernel.RateLimitedError after VerificationEmailLimit.
func (s *Service) ResendVerificationEmail(ctx context.Context, userID string) error {
	if s.d.Mailer == nil {
		return errors.New("identity: no mailer to send a verification email")
	}
	email, verified, err := s.d.Repository.FindPrimaryEmail(ctx, userID)
	if err != nil {
		return err
	}
	if verified {
		if again, err := s.verifiesAgain(ctx, email.ID); err != nil {
			return err
		} else if !again {
			return ErrAlreadyVerified
		}
	}
	if s.d.RateLimiter == nil {
		return errors.New("identity: ResendVerificationEmail needs a RateLimiter")
	}
	d, err := s.d.RateLimiter.Take(ctx, VerificationEmailLimit, userID)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return &kernel.RateLimitedError{RetryAfter: d.RetryAfter}
	}
	v := s.verificationRequest(userID, email.ID, s.d.Clock.Now())
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.enqueueVerification(ctx, tx, *v) })
}

// Access is what a user may do, from their registration state, email verification, and
// whether moderation forbids their local account to log in.
//
// Only an allowed user may use the app: the OAuth authorization endpoint calls UserAccess and
// refuses users for whom Allowed is false, so that a pending, rejected, unverified, or
// login-restricted user gets no access token, and AuthenticateBearer checks it again on every
// client API request. Such users can still sign in to the server-rendered screens, which tell
// them what they are waiting for or which moderation actions keep them out.
type Access struct {
	Registration identity.RegistrationState
	// Email is the user's primary email address ("" when none).
	Email string
	// EmailVerified is true when the primary address is confirmed, or when the user has none.
	EmailVerified bool
	// EmailVerificationRequired is true when an unverified address keeps the user out.
	EmailVerificationRequired bool
	// AccountID is the local account the user logs in as ("" when the user has none).
	AccountID string
	// AccountActive is true when that account is active: not deactivated, scheduled for
	// deletion, or deleted.
	AccountActive bool
	// DeletionDueAt is when the account is deleted while its deletion is scheduled; nil
	// otherwise.
	DeletionDueAt *time.Time
	// LoginRestricted is true when moderation forbids the account to log in and use the app
	// (disable_login, suspend, ban).
	LoginRestricted bool
}

// Allowed reports whether the user may use the app: approved, verified when that is required,
// and not restricted from logging in.
func (a Access) Allowed() bool {
	return a.Registration == identity.RegistrationApproved && (a.EmailVerified || !a.EmailVerificationRequired) && !a.LoginRestricted
}

// AwaitingVerification reports whether the user must still confirm their email address.
func (a Access) AwaitingVerification() bool { return a.EmailVerificationRequired && !a.EmailVerified }

// UserAccess returns what a user may do. A user without a registration state (created with
// the CLI) is approved.
func (s *Service) UserAccess(ctx context.Context, userID string) (Access, error) {
	out := Access{Registration: identity.RegistrationApproved, EmailVerificationRequired: s.d.RequireEmailVerification}
	st, err := s.d.Repository.FindRegistrationState(ctx, userID)
	switch {
	case err == nil:
		out.Registration = st.State
	case !errors.Is(err, ErrNotFound):
		return Access{}, err
	}
	email, verified, err := s.d.Repository.FindPrimaryEmail(ctx, userID)
	switch {
	case err == nil:
		out.Email, out.EmailVerified = email.Address, verified
	case errors.Is(err, ErrNotFound):
		out.EmailVerified = true
	default:
		return Access{}, err
	}
	if s.d.Accounts == nil {
		return out, nil
	}
	life, err := s.d.Accounts.UserLifecycle(ctx, userID)
	switch {
	case errors.Is(err, account.ErrNotFound):
		return out, nil
	case err != nil:
		return Access{}, err
	}
	out.AccountID, out.AccountActive = life.AccountID, life.Active()
	if life.DeletionScheduled() {
		out.DeletionDueAt = life.DeletionDueAt
	}
	if out.LoginRestricted, err = s.loginRestricted(ctx, life.AccountID); err != nil {
		return Access{}, err
	}
	return out, nil
}

// MaxListLimit bounds the limit of ListRegistrations and ListInvites.
const MaxListLimit = 1000

// ListRegistrations returns the registrations in a state, longest in the state first, at most
// limit (1 to MaxListLimit) of them, with the usernames of their local accounts.
func (s *Service) ListRegistrations(ctx context.Context, state identity.RegistrationState, limit int) ([]RegistrationEntry, error) {
	return s.ListRegistrationsAfter(ctx, state, RegistrationPosition{}, limit)
}

// ListRegistrationsAfter is ListRegistrations from after a position, for the pages that follow
// the first.
func (s *Service) ListRegistrationsAfter(ctx context.Context, state identity.RegistrationState, after RegistrationPosition, limit int) ([]RegistrationEntry, error) {
	if limit < 1 || limit > MaxListLimit {
		return nil, fmt.Errorf("identity: limit must be 1 to %d", MaxListLimit)
	}
	entries, err := s.d.Repository.ListRegistrations(ctx, state, after, limit)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].AccountID, entries[i].Username, err = s.localAccountOf(ctx, entries[i].Application.UserID); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

// Registration returns the registration of a user, in whatever state. It fails with
// ErrNotFound when the user did not sign up on the registration page.
func (s *Service) Registration(ctx context.Context, userID string) (RegistrationEntry, error) {
	app, err := s.d.Repository.FindRegistrationApplication(ctx, userID)
	if err != nil {
		return RegistrationEntry{}, err
	}
	st, err := s.d.Repository.FindRegistrationState(ctx, userID)
	if err != nil {
		return RegistrationEntry{}, err
	}
	e := RegistrationEntry{Application: app, State: st}
	email, verified, err := s.d.Repository.FindPrimaryEmail(ctx, userID)
	switch {
	case err == nil:
		e.Email, e.EmailVerified = email.Address, verified
	case !errors.Is(err, ErrNotFound):
		return RegistrationEntry{}, err
	}
	if e.AccountID, e.Username, err = s.localAccountOf(ctx, userID); err != nil {
		return RegistrationEntry{}, err
	}
	return e, nil
}

// localAccountOf returns the ID and username of a user's local account, or "" for both when
// it is gone.
func (s *Service) localAccountOf(ctx context.Context, userID string) (id, username string, err error) {
	la, err := s.d.Accounts.LocalAccountOfUser(ctx, userID)
	switch {
	case err == nil:
		return la.AccountID, la.Username, nil
	case errors.Is(err, account.ErrNotFound):
		return "", "", nil
	}
	return "", "", err
}

// DecideInput is a moderator's decision on a pending registration.
type DecideInput struct {
	UserID   string
	Decision identity.Decision
	// DecidedByUserID is the moderator, or "" for the CLI.
	DecidedByUserID string
	Note            string
}

// DecideRegistration approves or rejects a pending registration. It fails with
// identity.ErrInvalidNote for a note that identity.ValidateDecisionNote rejects, with
// ErrNotFound when the user has no application, and with ErrNotPending when the registration is
// already decided, also by a concurrent decision: a decided registration never changes again.
// The decision is audited in its transaction (ADR-0037).
func (s *Service) DecideRegistration(ctx context.Context, in DecideInput) (identity.RegistrationDecision, error) {
	if in.Decision != identity.DecisionApproved && in.Decision != identity.DecisionRejected {
		return identity.RegistrationDecision{}, fmt.Errorf("identity: unknown decision %q", in.Decision)
	}
	if err := identity.ValidateDecisionNote(in.Note); err != nil {
		return identity.RegistrationDecision{}, err
	}
	app, err := s.d.Repository.FindRegistrationApplication(ctx, in.UserID)
	if err != nil {
		return identity.RegistrationDecision{}, err
	}
	st, err := s.d.Repository.FindRegistrationState(ctx, in.UserID)
	if err != nil {
		return identity.RegistrationDecision{}, err
	}
	if st.State != identity.RegistrationPending {
		return identity.RegistrationDecision{}, ErrNotPending
	}
	now := s.d.Clock.Now()
	d := identity.RegistrationDecision{
		ID: s.d.IDs.NewID(), ApplicationID: app.ID, Decision: in.Decision, DecidedBy: identity.DecidedByModerator,
		DecidedByUserID: in.DecidedByUserID, Note: in.Note, DecidedAt: now,
	}
	audit, err := s.registrationAudit(ctx, in, d)
	if err != nil {
		return identity.RegistrationDecision{}, err
	}
	var approved *RegistrationApproved
	if in.Decision == identity.DecisionApproved {
		la, err := s.d.Accounts.LocalAccountOfUser(ctx, in.UserID)
		if err != nil {
			return identity.RegistrationDecision{}, err
		}
		approved = &RegistrationApproved{UserID: in.UserID, AccountID: la.AccountID, Username: la.Username, ApprovedAt: now}
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		moved, err := s.d.Repository.TransitionPendingRegistrationState(ctx, tx, identity.UserRegistrationState{
			UserID: in.UserID, State: in.Decision.State(), Since: now,
		})
		if err != nil {
			return err
		}
		if !moved {
			return ErrNotPending
		}
		if err := s.d.Repository.InsertRegistrationDecision(ctx, tx, d); err != nil {
			return err
		}
		if approved != nil {
			if err := tx.Emit(ctx, *approved); err != nil {
				return err
			}
		}
		return tx.Emit(ctx, audit)
	})
	if err != nil {
		return identity.RegistrationDecision{}, err
	}
	return d, nil
}

// registrationAudit returns the audit log entry of a decision on a registration. Its target is
// the user, labeled with the username of the user's local account.
func (s *Service) registrationAudit(ctx context.Context, in DecideInput, d identity.RegistrationDecision) (appmoderation.AuditRecorded, error) {
	label := in.UserID
	la, err := s.d.Accounts.LocalAccountOfUser(ctx, in.UserID)
	switch {
	case err == nil:
		label = "@" + la.Username
	case !errors.Is(err, account.ErrNotFound):
		return appmoderation.AuditRecorded{}, err
	}
	action := appmoderation.AuditRegistrationApproved
	if d.Decision == identity.DecisionRejected {
		action = appmoderation.AuditRegistrationRejected
	}
	actor, user := appmoderation.AuditActor(in.DecidedByUserID)
	return appmoderation.AuditRecorded{Entry: appmoderation.AuditEntry{
		ID: s.d.IDs.NewID(), ActorKind: actor, ActorUserID: user, Action: action, TargetKind: appmoderation.AuditTargetUser,
		TargetID: in.UserID, TargetLabel: label, SourceEventID: d.ID, OccurredAt: d.DecidedAt,
	}}, nil
}
