// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/oauth"
	"github.com/mycroblog/my/internal/interface/web"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpClientAPI registers the client API on every module, and makes the data of the webhook
// messages the admin API's representation (ADR-0060). It comes after every module it serves.
func (a *App) setUpClientAPI(cfg *config.Config, version string) {
	api := clientapi.New(clientapi.Options{
		Host: cfg.Host, Origin: cfg.ServerOrigin(), ResourceMetadataURL: cfg.ServerOrigin() + oauth.PathProtectedResourceMetadata, WebOrigin: cfg.WebOrigin, SoftwareVersion: version, SourceURL: cfg.SourceURL, Instance: a.Instance,
		SendsEmail:    sendsEmail(cfg),
		Authenticator: a.Identity, Accounts: a.Accounts, Profiles: a.Accounts, Posting: a.Content, Preferences: a.Identity, Badges: a.Identity, AutoDelete: a.AutoDelete, TwoFactor: a.Identity, Passkeys: a.Identity, AuthorizedApps: a.Identity, OAuthApps: a.Identity, Media: a.Media,
		AccountDomain: cfg.AccountDomain, Graph: a.Graph, InstanceThumbnail: a.instanceThumbnail,
		Resolver: a.RemoteActors, RemoteProfiles: a.Accounts, AccountSearch: a.AccountSearch, TagSearch: a.TagSearch,
		Notifications: a.Notifications, Push: a.Push,
		Statuses: a.Content, StatusReader: a.Statuses, Interactions: a.Interactions, Threads: a.RemoteStatuses, Outboxes: a.RemoteStatuses,
		Timelines: a.Timeline, TimelineReader: a.Statuses, TimelineLanguages: a.timelineLanguages,
		Streams: a.Streams, HomeEntries: a.Timeline,
		Permissions: a.Identity, InstanceHealth: a.InstanceHealth, DeadJobs: a.DeadJobs,
		Queue:      clientapi.QueueDashboard{Adapter: cfg.Queue, Inspector: a.JobInspector, Attempts: a.JobAttempts, Clock: a.Clock},
		Moderation: a.Moderation, UserAccounts: a.Accounts, FiledReports: a.Moderation, Severances: a.Moderation,
		AccountModeration: a.Moderation, ProfileRestrictions: a.AccountPolicy, OwnModeration: a.Moderation,
		AccountAdmin: a.AccountAdmin, AccountReports: a.Moderation, PasswordResetLinks: a.Identity,
		AuditLog: a.Moderation,
		Rules:    a.Instance,
		// The server-wide announcements (FR-726).
		Announcements: a.Instance,
		Roles:         a.Identity,
		Reports:       a.LocalReports,
		ReportCounts:  a.Moderation, RegistrationCounts: a.Identity, Registrations: a.Identity,
		OwnAppeals: a.Moderation, AppealModeration: a.Moderation, SignupBlocks: a.Moderation,
		OwnAccount: a.Identity, Exports: a.Exports, Imports: a.Imports,
		// The instance texts and the web client's pages of the documents among them (#489), which
		// GET /instance/documents/{kind} feeds.
		InstanceTexts: a.Instance, Documents: a.Instance, TermsPath: web.PathAboutTerms, PrivacyPolicyPath: web.PathAboutPrivacyPolicy,
		// The instance settings, and the invites with the registration page they open (#624).
		Settings: a.Instance, Invites: a.Identity, SignUpPath: authpages.PathSignUp,
		// The instance theme (ADR-0059) and the URLs of its stylesheets, which the web handler serves.
		Themes: a.Instance, ThemeStylesheetPath: web.ThemeStylesheetPath,
		// The custom emoji of the picker and their administration (ADR-0051).
		CustomEmojis:        a.CustomEmojis,
		CustomEmojiRequests: a.CustomEmojis,
		// The instance's webhooks (ADR-0060).
		Webhooks:     a.Webhooks,
		WebhookReads: webhookReads{a},
		// The relays the instance joins (ADR-0061).
		Relays: a.Relays,
		// The refetch of remote posts from their servers (FR-407).
		Refetches: a.Refetches,
		// The billing imports (ADR-0068).
		BillingImports: a.Finance,
		// The instance's costs (ADR-0067).
		Costs: a.Finance, Income: a.Finance,
		// The supporter program (ADR-0072) and the page of its commerce disclosures (§13).
		Supporters: a.Supporters, OwnSupporters: a.Supporters, CommerceDisclosurePath: web.PathAboutCommerceDisclosure,
		// The payment provider's state and the reconciliation by hand (ADR-0074 §4).
		Payments:    a.paymentsAPI(),
		RateLimiter: a.RateLimiter, Locales: a.Locales, RequestID: RequestID, Logger: a.Logger, Meter: a.Meter,
	})
	api.Register(clientAPIRouter{a})
	// The data of the webhook messages is the admin API's representation (ADR-0060).
	a.Webhooks.SetPresenter(api)
}

// timelineLanguages returns the languages that a user's timelines show (FR-208); a user without
// preferences sees every language.
func (a *App) timelineLanguages(ctx context.Context, userID string) ([]string, error) {
	p, err := a.Identity.Preferences(ctx, userID)
	if errors.Is(err, identity.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p.TimelineLanguages, nil
}
