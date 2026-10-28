// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
)

// Deps are the ports and collaborators a Service uses.
type Deps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Clock      kernel.Clock
	IDs        kernel.IDGenerator
	Passwords  *PasswordHasher
	// Accounts creates the local account a new user logs in as.
	Accounts *account.Service
	// Logins keeps the accounts that moderation forbids to log in from signing in and from
	// using their tokens; nil forbids nobody.
	Logins LoginPolicy
	// DefaultLocale returns the instance's default locale, which a new user gets unless the
	// registration names one. It is called once per registration, before the transaction, so a
	// change of the instance setting applies to the next registration. Nil or an empty result
	// means i18n.SourceLocale.
	DefaultLocale func(ctx context.Context) (string, error)

	// RateLimiter limits failed sign-ins (LoginIPLimit, LoginAccountLimit) and failed codes of
	// two-factor authentication (SecondFactorLimit); SignIn needs it.
	RateLimiter *kernel.RateLimiter
	// Sealer seals the secrets of TOTP enrollments (ADR-0019 §6), and KV keeps the sign-in
	// challenges and used TOTP steps; two-factor authentication needs both.
	Sealer *kernel.Sealer
	KV     kernel.KeyValueStore
	// Passkeys runs the WebAuthn ceremonies of passkeys (FR-004), which also need the Sealer,
	// the KV store, and the RateLimiter; nil turns passkeys off.
	Passkeys PasskeyCeremonies
	// HumanVerifier checks the CAPTCHA token of every sign-in and sign-up (ADR-0042 §3); nil,
	// as with the captcha setting none, checks nothing.
	HumanVerifier kernel.HumanVerifier
	// Logger records the sign-ins let through while the CAPTCHA service cannot check their
	// token; nil means slog.Default().
	Logger *slog.Logger
	// SessionTTL is the lifetime of a session; zero means DefaultSessionTTL.
	SessionTTL time.Duration
	// WebClient is the first-party web client of the authorization server; a zero value
	// registers none.
	WebClient WebClient

	// DynamicClientRegistration returns whether OAuth clients may register themselves at
	// POST /oauth/register (the dynamic_client_registration setting); nil refuses every
	// registration. It is called once per registration.
	DynamicClientRegistration func(ctx context.Context) (bool, error)
	// DynamicClientUnredeemedTTL and DynamicClientIdleTTL are when the clean-up deletes an unused
	// dynamically registered client; zero means DefaultDynamicClientUnredeemedTTL and
	// DefaultDynamicClientIdleTTL.
	DynamicClientUnredeemedTTL time.Duration
	DynamicClientIdleTTL       time.Duration

	// HTTP fetches the Client ID Metadata Documents of OAuth clients whose client_id is an https
	// URL (ADR-0090 §1); nil turns such clients off.
	HTTP kernel.HTTPClient
	// OIDC discovers the OpenID Connect providers that administrators set up and verifies their
	// ID tokens; with HTTP, which exchanges the codes for tokens, it turns on signing in and
	// signing up with them (external_login.go, ADR-0106 §3). Nil turns them off.
	OIDC OIDCClient
	// LoginProviderDomainRestricted reports whether a host, the server of a mastodon login
	// provider, or one of its parent domains has a domain action in effect (FR-702), which
	// refuses signing in, signing up, and linking with that provider (ADR-0106 §1). Nil
	// restricts no server.
	LoginProviderDomainRestricted func(ctx context.Context, host string) (bool, error)
	// MetadataDocumentClients returns whether the instance accepts those clients (the
	// oauth_metadata_document_clients setting); it is read per request, so a change applies to
	// the next one. Nil means it does.
	MetadataDocumentClients func(ctx context.Context) (bool, error)

	// RegistrationMode returns the instance's registration mode (the registration_mode
	// setting). SignUp calls it once per sign-up, so a change applies to the next one. Nil
	// means identity.RegistrationClosed.
	RegistrationMode func(ctx context.Context) (identity.RegistrationMode, error)
	// Closure stops the approval of pending registrations from the announcement of the service
	// closure on (ADR-0088 §2); nil never stops it.
	Closure ServiceClosure
	// ClosureView returns the state of the service closure that its emails read
	// (RegisterClosureEmails); nil sends none of them.
	ClosureView func(ctx context.Context) (identity.ServiceClosureView, error)
	// IndexableDefault returns whether new accounts are indexable (the indexable_default
	// setting, FR-412, ADR-0057); nil makes them not indexable.
	IndexableDefault func(ctx context.Context) (bool, error)
	// UserAppRegistration reports whether users may register OAuth clients (the
	// user_app_registration setting, ADR-0090 §1); RegisterOAuthApp calls it once per
	// registration. Nil lets them.
	UserAppRegistration func(ctx context.Context) (bool, error)
	// SignupBlocks screens the sign-ups from the registration page against the sign-up block
	// list (FR-719); nil screens nothing.
	SignupBlocks SignupScreen
	// MemberInvites returns the instance settings of the invites that members create
	// (invite_creators and the caps, ADR-0108 §1, §2). It is read per operation, so a change
	// applies to the next one. Nil means the defaults: only the holders of create_invites,
	// under the default caps.
	MemberInvites func(ctx context.Context) (MemberInviteSettings, error)
	// InviterRestricted reports whether moderation suspends or limits a local account, which
	// keeps its user from creating invites and its invites from working (ADR-0108 §1, §3); nil
	// restricts nobody.
	InviterRestricted func(ctx context.Context, accountID string) (bool, error)
	// Mailer sends verification emails, password reset emails, and the notices of password
	// changes from the jobs that RegisterMailJobs registers; nil sends none and issues no
	// verification tokens.
	Mailer kernel.Mailer
	// Outbox enqueues the jobs that send those emails in the transactions of the use cases
	// (ADR-0086 §1); a Mailer needs it.
	Outbox queue.Outbox
	// RequireEmailVerification keeps users whose primary address is unverified from using
	// the app (Access.Allowed). It is false when the mailer delivers no email (mail = none);
	// then a moderator's approval stands in for verification and open mode waits for one
	// (FR-002).
	RequireEmailVerification bool
	// EmailDelivery reports that the Mailer delivers email (mail is not none). Without it, a
	// user cannot ask for a password reset link by email, and only an administrator issues one
	// (IssuePasswordResetLink); no password change is notified by email.
	EmailDelivery bool
	// Unsubscriptions turns off the emails of the category of an unsubscribe link (ADR-0086 §4,
	// ADR-0109 §4); nil issues no unsubscribe links and accepts none.
	Unsubscriptions EmailUnsubscriptions
	// Messages words the emails; nil means the embedded catalogs (i18n.Load).
	Messages *i18n.Bundle
	// BaseURL is the origin that links in emails point to, such as https://social.example.
	BaseURL string
	// WebOrigin is the origin of the web client, whose pages some emails link to; "" means
	// BaseURL.
	WebOrigin string
	// MailSite returns the instance as its emails name it: its display name, its URL, and its
	// contact address (ADR-0086 §2, §3). Nil, or an empty name or URL, names the instance by the
	// host of BaseURL and links to BaseURL.
	MailSite func(ctx context.Context) kernel.MailSite
}

// Service runs the identity module's use cases.
type Service struct {
	d Deps

	// dummy is the hash that SignIn verifies against for unknown users, made on first use.
	dummyMu sync.Mutex
	dummy   string

	// webClientReady is set once EnsureWebClient has succeeded.
	webClientReady atomic.Bool
}

// NewService returns a Service on d.
func NewService(d Deps) *Service {
	return &Service{d: d}
}

// RegisterInput describes a new local user.
type RegisterInput struct {
	Username    string
	Password    string
	DisplayName string
	// Locale is the new user's UI locale, a BCP 47 tag; empty means Deps.DefaultLocale.
	Locale string
	// passwordless creates the user without a password: a user who signs in with an external
	// login provider only (SignUpExternal).
	passwordless bool
}

// Registration is the result of RegisterLocalUser.
type Registration struct {
	User identity.User
	// AccountID is the ID of the user's local account.
	AccountID string
}

// RegisterLocalUser creates a user with a password, default preferences, and the local account it
// logs in as, in one transaction. It fails with account.ErrUsernameTaken when the username is
// taken, and with errors wrapping identity.ErrInvalidPassword, identity.ErrInvalidLanguageTag, or
// account.ErrInvalidUsername for invalid input.
//
// Hashing the password and generating the account's keys happen before the transaction.
func (s *Service) RegisterLocalUser(ctx context.Context, in RegisterInput) (Registration, error) {
	p, err := s.prepareUser(ctx, in)
	if err != nil {
		return Registration{}, err
	}
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.writeUser(ctx, tx, p) }); err != nil {
		return Registration{}, err
	}
	return p.registration(), nil
}

// CreateAdminInput describes a new administrator.
type CreateAdminInput struct {
	Username string
	Email    string
	Password string
}

// CreateAdmin creates a user with a password, a primary email address (verified, since the
// operator gave it), default preferences in the instance's default locale, the local account it
// logs in as, and a grant of the built-in admin role from the CLI (no granting user), in one
// transaction. It creates the built-in roles first if they are missing.
//
// It fails with account.ErrUsernameTaken or ErrEmailTaken when another user has the username or
// email address, and with errors wrapping identity.ErrInvalidPassword, identity.ErrInvalidEmail,
// or account.ErrInvalidUsername for invalid input. A failure writes nothing, so running it again
// is safe; it never changes an existing user.
func (s *Service) CreateAdmin(ctx context.Context, in CreateAdminInput) (Registration, error) {
	if err := identity.ValidateEmail(in.Email); err != nil {
		return Registration{}, err
	}
	p, err := s.prepareUser(ctx, RegisterInput{Username: in.Username, Password: in.Password})
	if err != nil {
		return Registration{}, err
	}
	email := identity.EmailAddress{
		ID:                s.d.IDs.NewID(),
		UserID:            p.user.ID,
		Address:           in.Email,
		AddressNormalized: identity.NormalizeEmail(in.Email),
		AddedAt:           p.user.CreatedAt,
	}
	roles := s.builtinRoles()
	grantID := s.d.IDs.NewID()
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		admin, err := s.ensureBuiltinRoles(ctx, tx, roles)
		if err != nil {
			return err
		}
		if err := s.writeUser(ctx, tx, p); err != nil {
			return err
		}
		if err := s.d.Repository.InsertEmailAddress(ctx, tx, email); err != nil {
			return err
		}
		if err := s.d.Repository.InsertPrimaryEmail(ctx, tx, email); err != nil {
			return err
		}
		// The operator gave the address on the command line; there is nobody to send a link to.
		verified := identity.EmailVerification{EmailAddressID: email.ID, VerifiedAt: email.AddedAt}
		if err := s.d.Repository.InsertEmailVerification(ctx, tx, verified); err != nil {
			return err
		}
		return s.d.Repository.InsertRoleGrant(ctx, tx, identity.RoleGrant{
			ID: grantID, UserID: p.user.ID, RoleID: admin.ID, GrantedAt: p.user.CreatedAt,
		})
	})
	if err != nil {
		return Registration{}, err
	}
	return p.registration(), nil
}

// preparedUser is a new user with everything slow (hashing, key generation) done, ready to be
// written by writeUser; a retried transaction writes the same values.
type preparedUser struct {
	user identity.User
	// cred is the password; its ID is "" for a user without one.
	cred  identity.PasswordCredential
	prefs identity.Preferences
	acct  *account.PreparedLocalAccount
}

func (p *preparedUser) registration() Registration {
	return Registration{User: p.user, AccountID: p.acct.Account.ID}
}

func (s *Service) prepareUser(ctx context.Context, in RegisterInput) (*preparedUser, error) {
	if !in.passwordless {
		if err := identity.ValidatePassword(in.Password); err != nil {
			return nil, err
		}
	}
	now := s.d.Clock.Now()
	user := identity.User{ID: s.d.IDs.NewID(), CreatedAt: now}
	prefs, err := s.defaultPreferences(ctx, user.ID, in.Locale, now)
	if err != nil {
		return nil, err
	}
	indexable := false
	if s.d.IndexableDefault != nil {
		if indexable, err = s.d.IndexableDefault(ctx); err != nil {
			return nil, err
		}
	}
	acct, err := s.d.Accounts.PrepareLocalAccount(ctx, account.LocalAccountInput{
		UserID:      user.ID,
		Username:    in.Username,
		DisplayName: in.DisplayName,
		Indexable:   indexable,
	})
	if err != nil {
		return nil, err
	}
	p := &preparedUser{user: user, prefs: prefs, acct: acct}
	if in.passwordless {
		return p, nil
	}
	hash, err := s.d.Passwords.Hash(ctx, in.Password)
	if err != nil {
		return nil, err
	}
	p.cred = identity.PasswordCredential{ID: s.d.IDs.NewID(), UserID: user.ID, Hash: hash, SetAt: now}
	return p, nil
}

func (s *Service) writeUser(ctx context.Context, tx uow.Tx, p *preparedUser) error {
	if err := s.d.Repository.InsertUser(ctx, tx, p.user); err != nil {
		return err
	}
	if p.cred.ID != "" {
		if err := s.d.Repository.InsertPasswordCredential(ctx, tx, p.cred); err != nil {
			return err
		}
	}
	if err := s.d.Repository.InsertPreferences(ctx, tx, p.prefs); err != nil {
		return err
	}
	return s.d.Accounts.WriteLocalAccount(ctx, tx, p.acct)
}

type roleToEnsure struct {
	role  identity.Role
	perms []identity.Permission
}

// builtinRoles returns the built-in roles with fresh IDs, for EnsureRole.
func (s *Service) builtinRoles() []roleToEnsure {
	now := s.d.Clock.Now()
	builtins := identity.BuiltinRoles()
	out := make([]roleToEnsure, len(builtins))
	for i, b := range builtins {
		out[i] = roleToEnsure{
			role:  identity.Role{ID: s.d.IDs.NewID(), Key: b.Key, Position: b.Position, CreatedAt: now},
			perms: b.Permissions,
		}
	}
	return out
}

// HasPermission reports whether one of the roles that the user userID holds grants p.
func (s *Service) HasPermission(ctx context.Context, userID string, p identity.Permission) (bool, error) {
	perms, err := s.Permissions(ctx, userID)
	if err != nil {
		return false, err
	}
	return slices.Contains(perms, p), nil
}

// Permissions returns the permissions that the roles of the user userID grant, in the order of
// identity.AllPermissions; none for a user without roles.
func (s *Service) Permissions(ctx context.Context, userID string) ([]identity.Permission, error) {
	roles, err := s.d.Repository.ListUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	granted := map[identity.Permission]bool{}
	for _, r := range roles {
		_, perms, err := s.d.Repository.FindRole(ctx, r.Key)
		if err != nil {
			return nil, err
		}
		for _, p := range perms {
			granted[p] = true
		}
	}
	out := []identity.Permission{}
	for _, p := range identity.AllPermissions() {
		if granted[p] {
			out = append(out, p)
		}
	}
	return out, nil
}

// EnsureBuiltinRoles creates the built-in roles (admin and moderator) with their permissions,
// and gives admin every permission. It is safe to run on every start: it leaves the permissions
// and the position of an existing moderator role as the administrators set them.
func (s *Service) EnsureBuiltinRoles(ctx context.Context) error {
	roles := s.builtinRoles()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := s.ensureBuiltinRoles(ctx, tx, roles)
		return err
	})
}

// ensureBuiltinRoles creates inside tx the built-in roles that are missing, gives admin every
// permission (a new version may add permissions), and returns the admin role.
func (s *Service) ensureBuiltinRoles(ctx context.Context, tx uow.Tx, roles []roleToEnsure) (identity.Role, error) {
	var admin identity.Role
	for _, r := range roles {
		stored, err := s.d.Repository.EnsureRole(ctx, tx, r.role, r.perms)
		if err != nil {
			return identity.Role{}, err
		}
		if stored.Key == identity.RoleAdmin {
			admin = stored
			if err := s.d.Repository.SetRolePermissions(ctx, tx, stored.ID, identity.AllPermissions()); err != nil {
				return identity.Role{}, err
			}
		}
	}
	return admin, nil
}
