// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"

	"golang.org/x/sync/singleflight"

	"github.com/mycroblog/my/internal/adapter/captcha/siteverify"
	"github.com/mycroblog/my/internal/adapter/httpclient"
	"github.com/mycroblog/my/internal/adapter/keymanager/local"
	mailnone "github.com/mycroblog/my/internal/adapter/mail/none"
	mailsmtp "github.com/mycroblog/my/internal/adapter/mail/smtp"
	"github.com/mycroblog/my/internal/adapter/system"
	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/dbupgrade"
	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/app/metrics"
	"github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/projection"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/rewrap"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/app/webhook"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/platform/config"
)

// WebClientCallbackPath is the path on the web origin where the web client receives the
// authorization response; its URL is the redirect URI of the first-party web client.
const WebClientCallbackPath = "/login/callback"

// App is the assembled application: the adapters chosen by the configuration and the
// application services built on them. NewApp builds it with plain constructor calls; there is
// no DI container.
type App struct {
	Config   *config.Config
	Logger   *slog.Logger
	Messages *i18n.Bundle
	// Locales resolves the locale of every HTTP response (ADR-0009 §2) on Messages and the
	// instance's default_locale setting; every HTTP surface localizes through it.
	Locales *i18n.Resolver

	Clock      kernel.Clock
	IDs        kernel.IDGenerator
	KeyManager kernel.KeyManager
	Sealer     *kernel.Sealer

	UnitOfWork uow.UnitOfWork
	Projectors *uow.Projectors

	Jobs queue.JobQueue
	// Outbox enqueues jobs inside a transaction; nil for the inline queue, which has no
	// transactional enqueue.
	Outbox   queue.Outbox
	DeadJobs queue.DeadJobs
	// JobAttempts records the runs of jobs that did not succeed, in the database whatever the
	// queue (ADR-0064 §1); JobInspector lists the pending jobs, nil for a queue that cannot.
	JobAttempts  queue.AttemptLog
	JobInspector queue.JobInspector
	Handlers     *queue.JobHandlerRegistry
	// Runner runs the jobs of the queue for the worker role.
	Runner *queue.PullRunner
	// runJobs runs the jobs of the worker role until stop is closed: Runner.RunUntil, or for
	// the inline queue its RunUntil, which runs Runner without racing the runs of Enqueue.
	runJobs func(ctx context.Context, stop <-chan struct{}) error

	// KV is the key-value store that cfg.KV selects; RateLimiter keeps its counters there.
	KV          kernel.KeyValueStore
	RateLimiter *kernel.RateLimiter
	// Mailer is the mail adapter that cfg.Mail selects.
	Mailer kernel.Mailer
	// HTTPClient sends every outbound HTTP request through the SSRF guard (ADR-0019 §1).
	HTTPClient kernel.HTTPClient
	// Meter is the metrics adapter that cfg.MetricsExporter selects (ADR-0055).
	Meter kernel.Meter
	// Tracer is the tracing adapter that cfg.TracesExporter selects (ADR-0063).
	Tracer kernel.Tracer
	// ErrorReporter receives the error reports of every module and hands them to its sinks:
	// those the modules add and the exporter that cfg.ErrorsExporter selects (ADR-0103).
	ErrorReporter *ErrorReporter
	// MetricsSnapshot reports the instance-wide gauges (ADR-0055 §4); nil when Meter records
	// nothing.
	MetricsSnapshot *metrics.Service
	// HumanVerifier checks the CAPTCHA token of the sign-in and registration forms, and
	// HumanCheckWidget is what those pages render for it; both are nil when captcha is none
	// (ADR-0042).
	HumanVerifier    kernel.HumanVerifier
	HumanCheckWidget *kernel.HumanCheckWidget

	Accounts *account.Service
	// SearchTerms keeps the account_search_terms projection, and AccountSearch searches it
	// (ADR-0092 §1).
	SearchTerms   *account.SearchTermProjector
	AccountSearch *account.PrefixSearch
	// AccountSuggestions suggests accounts to follow (FR-308).
	AccountSuggestions *account.Suggestions
	Identity           *identity.Service
	// AccountAdmin is the moderators' account management (FR-714).
	AccountAdmin *identity.AccountAdmin
	// Exports builds the archives of the users' data (FR-008).
	Exports *identity.Exports
	// Imports writes the CSV exports and runs the CSV imports of follows, blocks, mutes, and
	// bookmarks (FR-008).
	Imports *identity.Imports
	// Instance holds the instance settings and texts, which live in the database; Config holds
	// the deployment configuration. They never share a key (docs/reference/configuration.md).
	Instance *instance.Service
	Graph    *graph.Service
	Content  *content.Service
	// Interactions runs boosts, favourites, bookmarks, and pins (FR-107, FR-113).
	Interactions *content.Interactions
	// QuoteHandlers send and receive the approvals of quotes (ADR-0069).
	QuoteHandlers *federation.QuoteHandlers
	// Statuses presents statuses, their threads and history, and bookmarks to a viewer.
	Statuses *content.Reader
	// AutoDelete holds the users' automatic deletion of their old posts and runs it (FR-118).
	AutoDelete *content.AutoDelete
	// Timeline reads the home, local, hashtag, and profile timelines (FR-201 to FR-204).
	Timeline *timeline.Service
	// TagSearch is the prefix search of hashtags (ADR-0092 §1).
	TagSearch *timeline.TagSearch
	// Moderation records the actions on domains and accounts and the reports received from
	// other servers; ModerationPolicy reads the restrictions they impose for federation and the
	// media proxy (docs/architecture/federation.md §11), and AccountPolicy those that the
	// identity, graph, and content modules and the client API apply (data-model §5.3).
	Moderation       *moderation.Service
	ModerationPolicy *moderation.FederationPolicy
	AccountPolicy    *moderation.AccountPolicy
	// LocalReports files the reports of local users (FR-701).
	LocalReports *moderation.LocalReports

	// Notifications lists a local account's notifications, which projectors create from the
	// graph and content events, with its read marker and dismissals (FR-205).
	Notifications *notification.Service
	// voiceNotifier creates the notifications of the voice chat rooms (ADR-0100 §8).
	voiceNotifier *notification.VoiceNotifier
	// Push registers the Web Push subscriptions and sends the notifications to them (FR-206).
	Push *notification.PushService
	// Streams is the broker of the streaming endpoint that cfg.Stream selects: a stream.Hub,
	// which reaches only the streams of this process, or a stream.Relay on a Bus across
	// processes (ADR-0050).
	Streams Streams

	// Blobs stores media files (ADR-0005); MediaProcessor checks and re-encodes them.
	Blobs          blob.Store
	MediaProcessor media.Processor
	Media          *content.MediaService
	// CustomEmojis manages the local custom emoji (ADR-0051).
	CustomEmojis *content.CustomEmojiService
	// MediaProxy relays remote media files under /media/proxy/ and signs their URLs (ADR-0005).
	MediaProxy *content.MediaProxy
	// LinkCards fetches and presents the cards of the links in statuses (ADR-0062).
	LinkCards *content.LinkCards
	// Previews reads profiles and statuses as an anonymous visitor sees them, for the link
	// previews of the web client's profile and post pages (ADR-0028).
	Previews *content.Previews
	// Webhooks sends the events of the other modules to the instance's webhooks (ADR-0060).
	Webhooks *webhook.Service
	// Finance records the instance's costs and totals them by week, month, and year (ADR-0067).
	Finance *finance.Service
	// Payments carries the payment provider's facts to the supporters (ADR-0074); nil with
	// payments_provider = none.
	Payments *supporters.Payments
	// Supporters runs the supporter program, its plans, and their subscriptions (ADR-0072).
	Supporters *supporters.Service
	// Voice is how the voice chat runs: its method, off for none, and the limits of its rooms
	// (ADR-0101 §1).
	Voice Voice
	// VoiceRooms runs the voice chat rooms (ADR-0100) and their jobs; its use cases answer
	// ErrDisabled while Voice is off.
	VoiceRooms *appvoice.Service
	// Diagnostics groups the error reports in the database and changes the state of the groups
	// (ADR-0103 §4 (2), §7).
	Diagnostics *appdiagnostics.Service
	// Scheduler enqueues the periodic jobs for the worker role.
	Scheduler *queue.Scheduler
	// Projections rebuilds and verifies the projections (`my projection`, data-model §9).
	Projections *projection.Service
	// DataUpgrades runs and checks the data upgrade steps (`my db upgrade`, ADR-0087 §4); the
	// readiness check requires every step that it registers.
	DataUpgrades *dbupgrade.Service
	// Rewrap reports and re-wraps the stored sealed values under the current KEK (`my kek`,
	// ADR-0019 §6).
	Rewrap *rewrap.Service
	// FanOut turns the outbound activities that the federation projectors record into
	// deliveries (docs/architecture/federation.md §7).
	FanOut *federation.FanOut
	// RemoteActors fetches remote actors, checks their handles, keeps their keys, and resolves
	// the keys of inbound signatures (docs/architecture/federation.md §6).
	RemoteActors *federation.RemoteActors
	// Deliverer sends the deliveries; InstanceHealth records the remote servers that answer
	// outside deliveries (docs/architecture/federation.md §7.3).
	Deliverer      *federation.Deliverer
	InstanceHealth *federation.Health
	// ClosureDeletes records and fans out the Delete of every local actor once the service is
	// closed, and tells when their deliveries end (ADR-0088 §3).
	ClosureDeletes *federation.ClosureDeletes
	// Inbox verifies and records the activities POSTed to the inboxes
	// (docs/architecture/federation.md §3.1).
	Inbox *federation.Inbox
	// InboxProcessor processes the recorded activities and dispatches them to the handler of
	// their type (docs/architecture/federation.md §3.2).
	InboxProcessor *federation.InboxProcessor
	// RemoteStatuses ingests remote posts from the inbox and fetches remote posts and threads;
	// Resolver resolves handles and the URLs of accounts and posts
	// (docs/architecture/federation.md §6.2, §6.3).
	RemoteStatuses *federation.StatusHandlers
	Resolver       *federation.Resolver
	// Refetches queues the refetch of remote posts that operators request (FR-407,
	// docs/architecture/federation.md §6.4).
	Refetches *federation.Refetches
	// Relays joins and leaves the relays and admits what they bring in (ADR-0061).
	Relays *federation.Relays
	// FederationSever ends the follows with remote accounts for `my admin federation sever`
	// (ADR-0076).
	FederationSever *federation.Severance

	source queue.Source
	// ready reports whether the database is reachable and its schema applied.
	ready  func(ctx context.Context) error
	closer func() error
	// dbSize measures the database for the metrics snapshot.
	dbSize metrics.DatabaseSizer
	// closeBlobs releases the blob store.
	closeBlobs func() error
	// billingClosers release the clients of the billing sources that hold one (ADR-0068).
	billingClosers []io.Closer
	// valkey is the connection to the Valkey server that the adapters configured with
	// valkey_url share, nil until one of them connects it.
	valkey valkeyServer
	// newPostgresBus returns the stream.Bus of stream = postgres on the database of db =
	// postgres, listening on listenURL; nil with another db.
	newPostgresBus func(listenURL string) (stream.Bus, error)
	// closeStreams closes Streams within its context, once.
	closeStreams func(ctx context.Context) error
	routes       []route
	// http records the HTTP server metrics; nil when Meter records nothing.
	http *httpMetrics
	// readinessFailures counts the failed readiness checks (ADR-0056 §3).
	readinessFailures kernel.Counter
	// readyFlight runs one readiness check for the concurrent requests (readyCheck).
	readyFlight singleflight.Group
	// metricsHandler serves the Prometheus scrape on metrics_listen; nil unless
	// metrics_exporter = prometheus. closeMeter flushes and stops the exporter.
	metricsHandler http.Handler
	closeMeter     func(ctx context.Context) error
	// closeTracer flushes and stops the exporter of traces_exporter = otlp.
	closeTracer func(ctx context.Context) error
	// page serves the web client's HTML at the GET paths that no route serves.
	page http.Handler
	// webStylesheet is the URL of the server-rendered pages' stylesheet from the web build, empty without one.
	webStylesheet string
	// webPasskeyScript is the URL of the sign-in page's passkey script from the web build, empty without one.
	webPasskeyScript string
}

type route struct {
	pattern string
	handler http.Handler
	// surface is the my.surface label of the HTTP metrics (ADR-0055 §3).
	surface string
}

// AppOptions configure NewApp.
type AppOptions struct {
	// Logger is the process logger; the default is slog.Default().
	Logger *slog.Logger
	// Version is the server version that NodeInfo reports; the default is "dev".
	Version string
	// WebBuild is the build of the web client; the default is the one embedded in the binary.
	WebBuild fs.FS
	// DevViteOrigin, when set, makes the web client's shell load the client from a Vite dev
	// server at this origin (`my serve -dev-vite-origin`). Development only.
	DevViteOrigin string
	// PseudoLocales makes the pseudo-locales en-XA and ar-XB selectable on every surface
	// (`my serve -pseudo-locales`), for the E2E of the server-rendered pages. Development and tests only.
	PseudoLocales bool
	// HTTPClient sends every outbound request; nil builds the SSRF-guarded client from the
	// configuration. Tests only, to reach other test servers by their public hosts.
	HTTPClient kernel.HTTPClient
	// Mailer, when set, sends the emails instead of the adapter that cfg.Mail selects, which
	// still decides what the server sends. Tests only, to send with mail/smtp to a test server
	// whose certificate the system roots do not hold (pkg/testkit/mailpittest).
	Mailer kernel.Mailer
	// StreamBus, when set, carries the signals of the streams between processes instead of the
	// broker that cfg.Stream selects. Tests only, to share an in-memory broker between Apps.
	StreamBus stream.Bus
	// MetricsWriter receives the EMF lines of metrics_exporter = emf; the default is standard
	// output, which the logs (on standard error) do not share.
	MetricsWriter io.Writer
	// Meter, when set, records the metrics instead of the adapter that cfg.MetricsExporter
	// selects. Tests only, to inspect the measurements with testkit.Meter.
	Meter kernel.Meter
	// Tracer, when set, records the spans instead of the adapter that cfg.TracesExporter
	// selects. Tests only, to inspect the spans with testkit.Tracer.
	Tracer kernel.Tracer
	// BillingSources, by name, stand in for the adapters of the names of cfg.BillingSources.
	// Tests only, to import from testkit.FakeBillingSource.
	BillingSources map[string]finance.BillingSource
	// PaymentProvider stands in for the adapter of payments_provider, and SupporterSync, when
	// set, for the subscriptions of the supporters module that apply its facts. Tests only, with
	// the fake adapter of internal/adapter/payment/fake.
	PaymentProvider supporters.PaymentProvider
	SupporterSync   supporters.Syncer
	// ClientAPIAccountLimit, when its Max is set, replaces the client API's per-account rate
	// limit (clientapi.AccountLimit). Tests only, to reach the limit in a few requests and a
	// short window (pkg/mykit/integration).
	ClientAPIAccountLimit kernel.Limit
	// DataUpgrades, when set, replaces the data upgrade steps and prerequisites of this release.
	// Tests only, to run test steps through `my db upgrade` and the readiness check.
	DataUpgrades *DataUpgrades
	// Clock, when set, tells the time instead of the system clock. Tests only, to give the App a
	// clock that never goes back, as the wall clock does when the host steps it.
	Clock kernel.Clock
}

// NewApp connects the adapters that cfg selects and builds the application services on them.
// It does not apply the database schema (Atlas does, ADR-0023) and succeeds without it; the
// readiness check reports a missing schema. Close the App to release its connections.
//
// Each module is set up in setup_<module>.go. The modules register their projectors in the
// order of the calls below, which is the order the projectors run in, so the order is that of
// their dependencies and of their projectors; a comment says why where it is not obvious.
func NewApp(ctx context.Context, cfg *config.Config, opts AppOptions) (_ *App, err error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	var i18nOpts []i18n.Option
	if opts.PseudoLocales {
		i18nOpts = append(i18nOpts, i18n.WithPseudoLocales())
	}
	messages, err := i18n.Load(i18nOpts...)
	if err != nil {
		return nil, err
	}
	if opts.Clock == nil {
		opts.Clock = system.Clock{}
	}
	a := &App{
		Config:     cfg,
		Logger:     opts.Logger,
		Messages:   messages,
		Clock:      opts.Clock,
		IDs:        system.IDGenerator{},
		Projectors: &uow.Projectors{},
		Handlers:   &queue.JobHandlerRegistry{},
	}

	if opts.Version == "" {
		opts.Version = "dev"
	}
	a.Voice = newVoice(cfg, a.Logger)
	if err := a.setUpTracer(ctx, cfg, opts); err != nil {
		return nil, err
	}
	if err := a.setUpMeter(ctx, cfg, opts); err != nil {
		_ = a.Close()
		return nil, err
	}
	if err := a.setUpErrorReporter(ctx, cfg, opts); err != nil {
		_ = a.Close()
		return nil, err
	}
	if a.KeyManager, err = newKeyManager(cfg); err != nil {
		_ = a.Close()
		return nil, err
	}
	a.Sealer = kernel.NewSealer(a.KeyManager)
	repos, err := a.openDatabase(ctx, cfg)
	if err != nil {
		_ = a.Close() // releases the telemetry set up above
		return nil, err
	}
	// From here on, a failure releases what the App holds.
	defer func() {
		if err != nil {
			_ = a.Close()
		}
	}()
	if err := a.setUpQueue(cfg); err != nil {
		return nil, err
	}
	if err := a.setUpKV(ctx, cfg); err != nil {
		return nil, err
	}
	a.setUpDeadJobPurger(cfg)
	if a.Mailer = opts.Mailer; a.Mailer == nil {
		if a.Mailer, err = newMailer(ctx, cfg, a.Logger); err != nil {
			return nil, err
		}
	}
	a.HTTPClient = opts.HTTPClient
	if a.HTTPClient == nil {
		a.HTTPClient = newHTTPClient(cfg, opts.Version, a.Logger, a.Meter, a.Tracer)
	}
	// The identity module's sign-in and registration check the CAPTCHA through it (ADR-0042 §3).
	if a.HumanVerifier, a.HumanCheckWidget, err = newHumanVerifier(cfg, a.HTTPClient, a.Logger); err != nil {
		return nil, err
	}
	// The user step is the locale of the client API request's authenticated viewer.
	a.Locales = &i18n.Resolver{Bundle: a.Messages, UserLocale: a.viewerLocale, InstanceDefault: a.instanceDefaultLocale}

	a.setUpAccount(cfg, repos.accounts)
	a.setUpInstance(repos.instances)
	// Moderation needs only the account module; the modules below read its restrictions.
	a.setUpModeration(repos.mods)
	if err := a.setUpIdentity(cfg, repos.identities); err != nil {
		return nil, err
	}
	a.setUpGraph(cfg, repos.follows)
	a.setUpContent(cfg, repos.statuses, repos.quotes, repos.interactions, repos.medias)
	a.setUpTimeline(repos.feeds, repos.statuses)
	// The error groups' projector of their states runs before the notifications and the webhooks
	// that read the states of the groups in the same transactions.
	a.setUpDiagnostics(repos.diagnostics)
	if err := a.setUpNotification(cfg, repos.notifs, repos.pushes); err != nil {
		return nil, err
	}
	a.setUpRetention(cfg, repos.retention, repos.statuses)
	a.setUpProjections(repos.projections, repos.projectionRepositories())
	if err := a.setUpDataUpgrades(repos.dataUpgrades, opts.DataUpgrades); err != nil {
		return nil, err
	}
	a.setUpRewrap(repos.sealedValues)
	if err := a.setUpStreams(ctx, cfg, opts.StreamBus); err != nil {
		return nil, err
	}
	a.registerStreamCommitHooks()
	a.registerBusinessMetrics(a.Projectors)
	fedOutbox := a.setUpFederation(cfg, repos.feds)
	a.setUpPolls(cfg, repos.statuses, repos.feds, fedOutbox)
	if err := a.setUpDelivery(repos.feds); err != nil {
		return nil, err
	}
	if err := a.setUpMedia(cfg, repos.medias, repos.linkCards, repos.emojis); err != nil {
		return nil, err
	}
	a.setUpLinkCards(cfg, repos.linkCards, repos.statuses, fedOutbox)
	a.setUpExports(repos.identities)
	a.setUpImports(repos.identities)
	paymentProvider, err := a.setUpSupporters(ctx, cfg, repos.supporters, opts)
	if err != nil {
		return nil, err
	}
	if err := a.setUpErasure(repos.identities, repos.accounts, repos.statuses, repos.follows, repos.feeds, repos.notifs, repos.medias, repos.voice); err != nil {
		return nil, err
	}
	a.setUpRemoteActors(cfg, repos.feds)
	a.setUpRelays(cfg, repos.feds)
	a.setUpInbox(cfg, repos.feds)
	a.setUpFederationSever()
	a.setUpStatusReader(repos.statuses, repos.interactions, repos.quotes)
	a.setUpLocalReports(repos.mods)
	a.setUpFanOut(cfg, repos.feds, fedOutbox)
	a.setUpWebhooks(repos.hooks)
	if err := a.setUpVoice(cfg, repos.voice); err != nil {
		return nil, err
	}
	if err := a.setUpFinance(ctx, cfg, repos.finance, opts); err != nil {
		return nil, err
	}
	if err := a.setUpPayments(cfg, repos.payments, paymentProvider, a.supporterSync(opts)); err != nil {
		return nil, err
	}
	// The web client comes after the link previews, which it serves.
	a.setUpPreviews(repos.statuses)
	if err := a.setUpWeb(opts); err != nil {
		return nil, err
	}
	a.setUpActivityPub(cfg, opts.Version)
	a.setUpAuthPages(cfg)
	if err := a.setUpMailEvents(cfg); err != nil {
		return nil, err
	}
	if err := a.setUpOAuth(cfg); err != nil {
		return nil, err
	}
	a.setUpMediaHTTP()
	a.setUpClientAPI(cfg, opts.Version, opts.ClientAPIAccountLimit)
	a.registerMetricsSnapshot()
	return a, nil
}

func newMailer(ctx context.Context, cfg *config.Config, logger *slog.Logger) (kernel.Mailer, error) {
	switch cfg.Mail {
	case "", "none":
		return mailnone.New(logger), nil
	case "smtp":
		return mailsmtp.New(mailsmtp.Options{
			Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword,
			From: cfg.MailFrom, HelloName: hostOnly(cfg.Host),
		})
	case "ses":
		return newSESMailer(ctx, cfg)
	default:
		return nil, fmt.Errorf("mail %q is not implemented yet", cfg.Mail)
	}
}

// newHTTPClient returns the SSRF-guarded HTTPClient. http_allow_private_networks, which the
// configuration accepts only in the development profile, turns the guard off with a warning;
// push_allowed_destinations, likewise development only, opens it for the Web Push sends to the
// listed destinations with a warning.
func newHTTPClient(cfg *config.Config, version string, logger *slog.Logger, meter kernel.Meter, tracer kernel.Tracer) kernel.HTTPClient {
	opts := httpclient.Options{
		UserAgent: fmt.Sprintf("my/%s (+https://%s/)", version, cfg.Host), Meter: meter, Tracer: tracer,
		// The webhooks alone may reach the internal destinations that the operator lists (ADR-0060).
		Webhook: httpclient.Destinations{Hosts: cfg.WebhookAllowedHosts, Prefixes: cfg.WebhookAllowedPrefixes},
		// So may the requests to the external identity providers (ADR-0106 §5).
		LoginProvider: httpclient.Destinations{Hosts: cfg.LoginProviderAllowedHosts, Prefixes: cfg.LoginProviderAllowedPrefixes},
		// And, in the development profile alone, the Web Push sends to a fake push service (ADR-0118 §3).
		WebPush: httpclient.Destinations{Hosts: cfg.PushAllowedHosts, Prefixes: cfg.PushAllowedPrefixes},
	}
	if len(cfg.PushAllowedHosts) > 0 || len(cfg.PushAllowedPrefixes) > 0 {
		logger.Warn("push_allowed_destinations is set: Web Push sends can reach the listed loopback and private destinations; never use it in production")
	}
	if cfg.HTTPAllowPrivateNetworks {
		logger.Warn("http_allow_private_networks is on: outbound HTTP can reach loopback and private networks; never use it in production")
		opts.AllowPrivateNetworks, opts.AllowHTTP, opts.AnyPort = true, true, true
	}
	return httpclient.New(opts)
}

// newHumanVerifier returns the CAPTCHA verifier and widget that cfg.Captcha selects, and
// nil for none, which sends nothing to a third party (ADR-0042).
func newHumanVerifier(cfg *config.Config, client kernel.HTTPClient, logger *slog.Logger) (kernel.HumanVerifier, *kernel.HumanCheckWidget, error) {
	if cfg.Captcha == "" || cfg.Captcha == "none" {
		return nil, nil, nil
	}
	v, err := siteverify.New(siteverify.Options{
		Service: kernel.HumanCheckService(cfg.Captcha), SiteKey: cfg.CaptchaSiteKey, SecretKey: siteverify.Secret(cfg.CaptchaSecretKey),
		Host: cfg.Host, HTTP: client, Logger: logger,
	})
	if err != nil {
		return nil, nil, err
	}
	w := v.Widget()
	return v, &w, nil
}

// hostOnly returns host without a port.
func hostOnly(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func newKeyManager(cfg *config.Config) (kernel.KeyManager, error) {
	switch cfg.KeyManager {
	case "local":
		return local.New(cfg.KEK, cfg.PreviousKEKs...)
	default:
		return nil, fmt.Errorf("key_manager %q is not implemented yet", cfg.KeyManager)
	}
}

// Ready reports whether the App can serve requests: the database is reachable, every table
// of its schema file exists, and it recorded every data upgrade step of this release
// (`my db upgrade`), and the Valkey server, when the App uses one, answers.
func (a *App) Ready(ctx context.Context) error {
	if a.ready == nil {
		return errors.New("no database")
	}
	if err := a.ready(ctx); err != nil {
		return err
	}
	if a.DataUpgrades != nil {
		if err := a.DataUpgrades.Ready(ctx); err != nil {
			return err
		}
	}
	if a.valkey != nil {
		return a.valkey.Ping(ctx)
	}
	return nil
}

// Handle registers h for pattern (an http.ServeMux pattern) on the HTTP server of the web role,
// as a route of the web surface. Call it before Serve.
func (a *App) Handle(pattern string, h http.Handler) { a.handle(pattern, h, surfaceWeb) }

func (a *App) handle(pattern string, h http.Handler, surface string) {
	a.routes = append(a.routes, route{pattern: pattern, handler: h, surface: surface})
}

// Close releases the database connections.
func (a *App) Close() error {
	if a.closeStreams != nil {
		ctx, cancel := context.WithTimeout(context.Background(), streamsCloseTimeout)
		a.shutDownStreams(ctx)
		cancel()
	}
	if a.closeBlobs != nil {
		_ = a.closeBlobs()
	}
	for _, c := range a.billingClosers {
		_ = c.Close()
	}
	a.billingClosers = nil
	if a.valkey != nil {
		a.valkey.Close()
	}
	// The sinks of the error reports may write to the database and measure what they send.
	a.closeErrorReporter()
	if a.closeMeter != nil {
		ctx, cancel := context.WithTimeout(context.Background(), meterCloseTimeout)
		_ = a.closeMeter(ctx) // the last measurements are lost when the collector is down
		cancel()
		a.closeMeter = nil
	}
	if a.closeTracer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), meterCloseTimeout)
		_ = a.closeTracer(ctx) // the last spans are lost when the collector is down
		cancel()
		a.closeTracer = nil
	}
	if a.closer == nil {
		return nil
	}
	return a.closer()
}
