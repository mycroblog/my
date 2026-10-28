// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/oidc"
	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/queue"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpIdentity registers the identity module's projectors and builds its service, the
// moderators' account management, and the purge of the sign-in and registration origins. It
// comes after the moderation module, whose restrictions sign-in and registration read.
func (a *App) setUpIdentity(cfg *config.Config, repo identity.Repository) error {
	passkeys, err := newPasskeys(cfg)
	if err != nil {
		return err
	}
	identity.RegisterModerationProjectors(a.Projectors, repo, a.Accounts, a.Clock)
	identity.RegisterLifecycleProjectors(a.Projectors, repo, a.Accounts, a.Clock)
	outbox := a.Outbox
	if outbox == nil {
		// The inline queue has no transactional enqueue; its jobs are enqueued after the commit.
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	a.Identity = identity.NewService(identity.Deps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock, IDs: a.IDs,
		Passwords: identity.NewPasswordHasher(identity.DefaultArgon2Params, 0), Accounts: a.Accounts,
		Logins: a.AccountPolicy, RateLimiter: a.RateLimiter,
		// TOTP secrets are sealed like actor keys (ADR-0019 §6); sign-in challenges live in the KV store.
		Sealer: a.Sealer, KV: a.KV,
		// Passkeys sign in instead of the password (FR-004); their ceremonies' state is sealed too.
		Passkeys: passkeys,
		// nil for captcha = none; sign-in and sign-up check the CAPTCHA token otherwise (ADR-0042).
		HumanVerifier: a.HumanVerifier, Logger: a.Logger,
		// The default locale and the registration mode are instance settings in the database,
		// never configuration (docs/reference/configuration.md); they are read per registration so
		// changes apply at once.
		DefaultLocale: a.Instance.DefaultLocale,
		WebClient: identity.WebClient{
			ClientID: identity.DefaultWebClientID, Name: cfg.Host, RedirectURIs: []string{cfg.WebOrigin + WebClientCallbackPath},
		},
		// The effective mode: closed from the announcement of the service closure on (ADR-0088 §2),
		// which also stops the approval of the pending registrations.
		RegistrationMode: func(ctx context.Context) (domainidentity.RegistrationMode, error) {
			mode, err := a.Instance.RegistrationMode(ctx)
			return domainidentity.RegistrationMode(mode), err
		},
		Closure: a.Instance,
		// The service closure emails read the state of the closure per job (ADR-0088 §2).
		ClosureView: a.closureView,
		// A client_id that is an https URL is a Client ID Metadata Document, fetched through the
		// SSRF guard while the instance setting accepts them (ADR-0090 §1).
		HTTP: a.HTTPClient,
		// Users sign in and sign up with the OpenID Connect providers that administrators set up;
		// Discovery, the JWKS, and the token exchange go through the same guard (ADR-0106 §5).
		OIDC: oidc.New(oidc.Options{HTTP: a.HTTPClient, Clock: a.Clock}),
		// Nobody signs in with a Mastodon server whose domain has a domain action (ADR-0106 §1).
		LoginProviderDomainRestricted: a.loginProviderDomainRestricted,
		MetadataDocumentClients: func(ctx context.Context) (bool, error) {
			s, err := a.Instance.Settings(ctx)
			return s.OAuthMetadataDocumentClients(), err
		},
		// New accounts take the instance's indexable default (FR-412, ADR-0057).
		IndexableDefault: func(ctx context.Context) (bool, error) {
			s, err := a.Instance.Settings(ctx)
			return s.IndexableDefault(), err
		},
		// OAuth clients register themselves at POST /oauth/register unless the administrator
		// turned it off; the clean-up deletes the ones that are not used (ADR-0090 §1).
		DynamicClientRegistration: func(ctx context.Context) (bool, error) {
			s, err := a.Instance.Settings(ctx)
			return s.DynamicClientRegistration(), err
		},
		DynamicClientUnredeemedTTL: cfg.DynamicClientUnredeemedTTL,
		DynamicClientIdleTTL:       cfg.DynamicClientIdleTTL,
		// Users register OAuth apps while the instance allows it (ADR-0090 §1).
		UserAppRegistration: func(ctx context.Context) (bool, error) {
			s, err := a.Instance.Settings(ctx)
			return s.UserAppRegistration(), err
		},
		// The use cases enqueue their emails, and only the jobs' handlers send them (ADR-0086 §1).
		Mailer: a.Mailer, Outbox: outbox,
		// The registration page screens sign-ups against the sign-up block list (FR-719).
		SignupBlocks: a.Moderation,
		// Members create invites as the instance settings let them (ADR-0108 §1, §2), read per
		// operation, while moderation neither suspends nor limits them (§1, §3).
		MemberInvites:     a.memberInviteSettings,
		InviterRestricted: a.inviterRestricted,
		// mail = none delivers nothing, so a moderator's approval stands in for verification (FR-002).
		RequireEmailVerification: cfg.Mail != "none",
		// Users ask for password reset links by email only when the mailer delivers it (FR-002).
		EmailDelivery: sendsEmail(cfg),
		Messages:      a.Messages,
		BaseURL:       cfg.ServerOrigin(),
		WebOrigin:     cfg.WebOrigin,
		MailSite:      a.mailSite,
	})
	// The dynamically registered clients that nobody uses are deleted (ADR-0090 §1).
	a.Identity.RegisterClientJobs(a.Handlers, a.Scheduler)
	a.Identity.RegisterMailJobs(a.Handlers)
	// The start of the notice period of the service closure rejects the pending registrations.
	a.Identity.RegisterClosureProjectors(a.Projectors)
	a.Identity.RegisterClosureJobs(a.Handlers)
	// Every user with a verified address hears of the service closure by email (ADR-0088 §2).
	a.Identity.RegisterClosureEmails(a.Projectors, a.Handlers, a.Scheduler)
	a.AccountAdmin = identity.NewAccountAdmin(identity.AccountAdminDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Accounts: a.Accounts, Restrictions: a.AccountPolicy,
		Clock: a.Clock, IDs: a.IDs,
	})
	// The sign-in and registration origins are erased after their retention (ADR-0038).
	identity.NewOriginPurger(identity.OriginPurgeDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock, Policy: originRetention{a},
	}).RegisterJobs(a.Handlers, a.Scheduler)
	return nil
}

// loginProviderDomainRestricted reports whether the server of a mastodon login provider, or a
// parent domain, has a domain action in effect, from the moderation module's restrictions.
func (a *App) loginProviderDomainRestricted(ctx context.Context, host string) (bool, error) {
	canonical, err := federation.CanonicalHost(host)
	if err != nil {
		return false, err
	}
	rs, err := a.ModerationPolicy.DomainRestrictions(ctx, canonical)
	return len(rs) > 0, err
}
