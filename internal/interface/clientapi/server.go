// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/i18n"
)

// BasePath is the path prefix of every client API endpoint, the server URL of api/openapi.yaml.
const BasePath = "/api/v1"

// Options configure a Server.
type Options struct {
	// Host is the domain that serves the instance.
	Host string
	// AccountDomain is the domain of local handles (@alice@AccountDomain) when it differs
	// from Host; "" means Host.
	AccountDomain string
	// Origin is the origin at which browsers reach the server's own pages, for the links that
	// responses carry to them (Account.url of a local account, Status.url of a local status,
	// StatusTag.url); "" means https://Host. The ActivityPub ids stay under https://Host.
	Origin string
	// WebOrigin is the origin of the web client, which the payment provider's checkout and
	// portal return to (supporters.go); "" means Origin.
	WebOrigin string
	// SoftwareVersion is the server version.
	SoftwareVersion string
	// SourceURL is where users get the server's source code, an https URL or a mailto: address (ADR-0011).
	SourceURL string
	// Instance reads the instance settings and texts; nil reads every setting as its default
	// and no text.
	Instance InstanceInfo
	// Documents reads the rendered documents for GET /instance/documents/{kind}; nil answers
	// 404 to it.
	Documents InstanceDocuments
	// InstanceThumbnail returns the image that represents the instance (the thumbnail setting)
	// for a reader of locale, for Instance.thumbnail; nil, or a nil image, leaves it out.
	InstanceThumbnail func(ctx context.Context) (*InstanceImage, error)
	// ResourceMetadataURL is the URL of the protected resource metadata (RFC 9728), which the
	// WWW-Authenticate challenge of a 401 response names as resource_metadata; "" leaves it out.
	ResourceMetadataURL string
	// SendsEmail tells whether the instance sends email (the setting mail is not none), for
	// Instance.sends_email.
	SendsEmail bool

	// Authenticator authenticates bearer access tokens; nil accepts none.
	Authenticator Authenticator
	// Accounts reads accounts.
	Accounts Accounts
	// RemoteProfiles reads the profile pages of remote accounts for Account.url; nil links
	// remote accounts to their uri.
	RemoteProfiles RemoteProfiles
	// AccountSearch is the prefix search of accounts (searchAccounts); nil finds none.
	AccountSearch AccountSearch
	// TagSearch is the prefix search of hashtags (searchTags); nil finds none.
	TagSearch TagSearch
	// Resolver fetches unknown remote accounts for lookupAccount's resolve; nil resolves none
	// (lookups find only stored accounts).
	Resolver AccountResolver
	// Profiles changes the viewer's profile.
	Profiles Profiles
	// Posting reads what the viewer's restrictions let them post, for CredentialAccount; nil
	// presents every account as unrestricted.
	Posting PostingPolicy
	// Preferences reads and changes the viewer's preferences.
	Preferences UserPreferences
	// Badges reads the role badges that users show on their profiles and changes the viewer's
	// choice of them (ADR-0071); nil shows none and answers updateBadgeChoice 404.
	Badges RoleBadges
	// AutoDelete reads and replaces the viewer's automatic deletion of their old posts (FR-118);
	// nil answers its operations 404.
	AutoDelete AutoDeletePolicies
	// TwoFactor runs the viewer's two-factor authentication; nil answers its operations 404.
	TwoFactor TwoFactorSettings
	// Passkeys runs the viewer's passkeys; nil answers their operations 404.
	Passkeys PasskeySettings
	// AuthorizedApps runs the viewer's authorized OAuth apps; nil answers their operations 404.
	AuthorizedApps AuthorizedApps
	// OAuthApps runs the OAuth apps that the viewer registers; nil answers their operations 404.
	OAuthApps OAuthAppSettings
	// Media runs the media use cases and presents profile images; nil serves no media
	// operation (they fail with 500) and presents no images.
	Media MediaFiles
	// Graph reads and changes follows, blocks, and mutes.
	Graph Graph
	// Notifications runs the use cases of the viewer's notifications.
	Notifications Notifications
	// Push runs the Web Push subscription use cases and gives Instance.push_server_key; nil
	// sends no Web Push.
	Push WebPush
	// Statuses posts, edits, and deletes statuses; StatusReader presents them; Interactions
	// boosts, favourites, bookmarks, and pins them (the statuses operations).
	Statuses     Statuses
	StatusReader StatusReader
	Interactions Interactions
	// Threads starts fetching the remote parts of a thread that a viewer opens (getStatusContext);
	// nil fetches none.
	Threads ThreadFetcher
	// Outboxes starts fetching the recent posts of a remote account whose profile a logged-in
	// viewer opens (the first page of listAccountStatuses); nil fetches none.
	Outboxes OutboxFetcher
	// Timelines reads the timelines, and TimelineReader the statuses they list.
	Timelines      Timelines
	TimelineReader TimelineReader
	// TimelineLanguages returns the languages that a user's timelines show, none for every
	// language (FR-208); nil shows every language.
	TimelineLanguages func(ctx context.Context, userID string) ([]string, error)
	// Streams opens the subscriptions of the streaming endpoint; nil answers it with 503.
	// HomeEntries tells the streams whether a home timeline lists a new entry; nil streams no
	// home timeline entries. StreamHeartbeat overrides DefaultStreamHeartbeat when positive.
	Streams         stream.Subscriber
	HomeEntries     HomeEntries
	StreamHeartbeat time.Duration
	// Permissions reads the permissions of the viewer's roles, which the admin operations need
	// (admin.go); nil grants none. InstanceHealth reads and clears the health of remote servers,
	// and DeadJobs lists and requeues the dead jobs, for the admin operations; Queue reads the
	// rest of the queue dashboard (admin_jobs.go, ADR-0064).
	Permissions    Permissions
	InstanceHealth InstanceHealth
	DeadJobs       queue.DeadJobs
	Queue          QueueDashboard
	// Moderation runs the moderators' use cases on domain actions and reports for the admin
	// operations (admin_moderation.go); UserAccounts presents the moderator of a report by
	// account, and nil leaves it out.
	Moderation   Moderation
	UserAccounts UserAccounts
	// AccountModeration runs the moderators' use cases on the actions on accounts for the admin
	// operations (admin_account_actions.go).
	AccountModeration AccountModeration
	// AccountAdmin runs the account management of the admin operations, and AccountReports
	// reads the reports of an account for its details (admin_accounts.go).
	AccountAdmin   AccountAdmin
	AccountReports AccountReports
	// PasswordResetLinks issues the password reset links that administrators hand to users
	// (admin_accounts.go); nil answers 404 to the operation.
	PasswordResetLinks PasswordResetLinks
	// OwnModeration reads the actions on the viewer's own account, for GET /moderation/actions
	// and the moderation notices (moderation.go); nil leaves the action out of the notices.
	OwnModeration OwnModeration
	// OwnAppeals files the viewer's appeals of the actions on their account and reads the action
	// of an appeal_decided notice (appeals.go); nil refuses appeals with 404 and leaves the action
	// out of those notices. AppealModeration runs the moderators' use cases on the appeals
	// (admin_appeals.go) and counts the pending ones for GET /admin/counts.
	OwnAppeals       OwnAppeals
	AppealModeration AppealModeration
	// OwnAccount deletes the viewer's own account (own_account.go); nil refuses it with 404.
	OwnAccount OwnAccount
	// Exports runs the viewer's data exports (own_account.go); nil refuses them with 404.
	Exports Exports
	// Imports writes the viewer's CSV exports and runs the CSV imports (data_import.go); nil
	// refuses them with 404.
	Imports Imports
	// ProfileRestrictions hides the accounts whose profiles moderation hides (profile_view)
	// from everyone but themselves; nil hides none. Moderators see them through the admin
	// operations.
	ProfileRestrictions ProfileRestrictions
	// AuditLog lists the audit log for the admin operations (admin_audit.go).
	AuditLog AuditLog
	// Announcements runs the server-wide announcements (announcements.go): the users' list and
	// reads, the admin operations, and the announcements of the streams; nil lists none, answers
	// 404 to the others, and streams none.
	Announcements Announcements
	// Rules runs the use cases of the instance rules: the admin operations (admin_rules.go)
	// and the rules of GET /instance, which lists none when it is nil.
	Rules InstanceRules
	// InstanceTexts reads and writes the instance texts for the admin operations
	// (admin_instance_texts.go).
	InstanceTexts InstanceTexts
	// TermsPath and PrivacyPolicyPath are the paths, on Origin, of the web client's pages of the
	// terms of service and the privacy policy, for Instance.terms_url and Instance.privacy_policy_url;
	// empty leaves the link out.
	TermsPath, PrivacyPolicyPath string
	// CommerceDisclosurePath is the path, on Origin, of the web client's page of the commerce
	// disclosures, for Instance.commerce_disclosure_url; empty leaves the link out.
	CommerceDisclosurePath string
	// CustomEmojis runs the custom emoji of the picker and the admin operations
	// (admin_custom_emojis.go); nil answers an empty picker and 404 to the admin operations.
	CustomEmojis CustomEmojis
	// CustomEmojiRequests runs the requests of the holders of create_custom_emojis and their
	// own emoji (ADR-0070); nil answers those operations 404.
	CustomEmojiRequests CustomEmojiRequests
	// Webhooks runs the admin operations on the instance's webhooks (admin_webhooks.go); nil
	// answers 404 to them.
	Webhooks Webhooks
	// WebhookReads are the reads in a transaction behind the data of the webhook messages
	// (webhook_data.go); without them the presenter methods fail.
	WebhookReads WebhookReads
	// Relays runs the admin operations on the relays (admin_relays.go); nil answers 404 to
	// them.
	Relays Relays
	// Refetches queues the refetch of remote posts (admin_refetch.go); nil answers 404 to its
	// operations.
	Refetches Refetches
	// BillingImports reads and starts the billing imports (admin_billing_imports.go); nil
	// answers 404 to their operations.
	BillingImports BillingImports
	// Costs records the instance's costs and totals them (admin_finance.go); nil answers 404 to
	// their operations.
	Costs Costs
	// Income totals the instance's income and lists its money movements (income.go); nil
	// answers 404 to their operations.
	Income Income
	// Supporters runs the supporter program, its plans, and their subscriptions
	// (admin_supporters.go); nil answers 404 to their operations and a disabled program.
	Supporters Supporters
	// Payments reads the state of the payment provider and reconciles with it
	// (admin_payments.go); nil, without a provider, answers 404 to their operations.
	Payments Payments
	// OwnSupporters runs the users' own subscriptions and the supporter list (supporters.go);
	// nil answers 404 to their operations.
	OwnSupporters OwnSupporters
	// Themes reads and replaces the instance theme for the admin operations and Instance.theme
	// (admin_theme.go); nil answers no theme and 404 to the admin operations.
	Themes Themes
	// ThemeStylesheetPath returns the path, on Origin, of a theme revision's stylesheet
	// (web.ThemeStylesheetPath); nil leaves the stylesheet URLs and Instance.theme out.
	ThemeStylesheetPath func(revisionID string) string
	// Roles lists, grants, and revokes the roles of users for the admin operations
	// (admin_roles.go).
	Roles Roles
	// Reports files the reports of local users (reports.go).
	Reports Reports
	// FiledReports reads the reports that the viewer filed (listOwnReports) and that their
	// report_resolved notifications tell of; nil leaves the report out of the notifications.
	FiledReports FiledReports
	// Severances reads the follows that the viewer's relationships_severed notifications tell
	// of; nil leaves them out of the notifications.
	Severances Severances
	// ReportCounts and RegistrationCounts count what awaits the moderators (admin_counts.go);
	// nil leaves the count out.
	ReportCounts       ReportCounts
	RegistrationCounts RegistrationCounts
	// Registrations runs the registration review (admin_registrations.go).
	Registrations Registrations
	// SignupBlocks runs the sign-up block list (admin_signup_blocks.go).
	SignupBlocks SignupBlocks
	// Settings reads and changes the instance settings (admin_settings.go).
	Settings InstanceSettings
	// Invites runs the invites (admin_invites.go); SignUpPath is the path, on Origin, of the
	// registration page that AdminInvite.url opens with the code filled in.
	Invites    Invites
	SignUpPath string
	// RateLimiter applies AnonymousLimit and AccountLimit; nil applies no limit.
	RateLimiter *kernel.RateLimiter
	// AnonymousLimit and AccountLimit override the package's limits of the same names when
	// their Max is set.
	AnonymousLimit kernel.Limit
	AccountLimit   kernel.Limit

	// Locales resolves the locale of problem documents (ADR-0009 §2). Its UserLocale step
	// finds the authenticated user with ViewerFrom.
	Locales *i18n.Resolver
	// RequestID returns the ID of the request in ctx, echoed in problem documents; nil omits it.
	RequestID func(ctx context.Context) string
	// Logger logs unexpected errors; nil means slog.Default().
	Logger *slog.Logger
	// Meter records my.stream.connections (ADR-0055 §3); nil records nothing. The HTTP
	// metrics of every operation are recorded around the Router.
	Meter kernel.Meter
}

// Server implements the generated StrictServerInterface by calling application use cases.
type Server struct {
	o Options
	// streams counts the open streams.
	streams kernel.UpDownCounter
}

var _ StrictServerInterface = (*Server)(nil)

// New returns a Server on o.
func New(o Options) *Server {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.RequestID == nil {
		o.RequestID = func(context.Context) string { return "" }
	}
	if o.AnonymousLimit.Max == 0 {
		o.AnonymousLimit = AnonymousLimit
	}
	if o.AccountLimit.Max == 0 {
		o.AccountLimit = AccountLimit
	}
	if o.Authenticator == nil {
		o.Authenticator = noTokens{}
	}
	return &Server{o: o, streams: kernel.MeterOrNop(o.Meter).UpDownCounter(StreamConnections)}
}

// Router registers handlers for http.ServeMux patterns.
type Router interface {
	Handle(pattern string, h http.Handler)
}

// Register registers every operation of the spec on r under BasePath, the generated ones and
// the hand-written streaming endpoint (streaming.go) and the bundled OpenAPI document (openapi.go), behind authentication and rate limiting, with the security of each operation enforced (auth.go). Requests that
// match no operation are left to r, which answers them with its own 404 or 405 problem.
func (s *Server) Register(r Router) {
	strict := NewStrictHandlerWithOptions(s, []StrictMiddlewareFunc{s.rejectInvalidBodyText}, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  s.requestError,
		ResponseErrorHandlerFunc: s.responseError,
	})
	HandlerWithOptions(strict, StdHTTPServerOptions{
		BaseURL:          BasePath,
		BaseRouter:       routerMux{r: r, wrap: s.guard},
		Middlewares:      []MiddlewareFunc{s.localize},
		ErrorHandlerFunc: s.requestError,
	})
	s.registerStreaming(r)
	s.registerOpenAPI(r)
}

// routerMux adapts a Router to the ServeMux that the generated code registers on, wrapping
// every operation's handler by its pattern.
type routerMux struct {
	r    Router
	wrap func(pattern string, h http.Handler) http.Handler
}

func (m routerMux) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	m.r.Handle(pattern, m.wrap(pattern, http.HandlerFunc(h)))
}

// ServeHTTP is never called: the Router serves the registered handlers.
func (routerMux) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}
