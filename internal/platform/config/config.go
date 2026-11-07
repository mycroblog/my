// SPDX-License-Identifier: AGPL-3.0-only

// Package config defines the typed configuration schema and loads it at startup.
//
// Each setting is resolved from, in order of precedence, its environment
// variable (EnvPrefix + upper-cased key), the optional TOML file named by
// MY_CONFIG, and its default. Secrets come only from the environment, either
// directly or through NAME_FILE. Load reports every problem at once.
package config

import (
	"cmp"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"

	"github.com/mycroblog/my/internal/adapter/plugin/kind"
)

// Role selects what a process runs.
type Role string

// Roles.
const (
	RoleAll    Role = "all"
	RoleWeb    Role = "web"
	RoleWorker Role = "worker"
)

// Config is the validated configuration. Adapter fields hold the adapter name.
type Config struct {
	Host          string
	AccountDomain string
	SourceURL     string
	// StatusPageURL is the address of the instance's status page, "" for none (ADR-0088 §20).
	StatusPageURL string
	// Maintenance starts the maintenance mode (ADR-0088). MaintenanceServerName, MaintenanceMessage,
	// and MaintenanceUntil (zero for none) fill the maintenance page; MaintenanceRetryAfter is the
	// Retry-After when MaintenanceUntil is zero or past.
	Maintenance           bool
	MaintenanceServerName string
	MaintenanceMessage    string
	MaintenanceUntil      time.Time
	MaintenanceRetryAfter time.Duration
	// WebOrigin is the origin of the web client that the client API allows by CORS.
	WebOrigin string
	// MediaOrigin is the origin of the media files' URLs, for a CDN in front of /media/; empty
	// serves them from ServerOrigin(). Use MediaBaseURL.
	MediaOrigin string
	Role        Role
	Listen      string
	// TrustedProxies are the peers whose TrustedProxyHeader tells the client IP (ADR-0019 §5);
	// TrustedProxyHeader is "x-forwarded-for" or "forwarded".
	TrustedProxies     []netip.Prefix
	TrustedProxyHeader string
	// OriginSecret is the value of the header OriginSecretHeader that a request must carry unless
	// the origin check exempts it, or the value OriginSecretPrevious during a rotation; empty
	// checks nothing.
	OriginSecret         string
	OriginSecretPrevious string
	OriginSecretHeader   string
	DB                   string
	DatabaseURL          string
	// DatabaseDevURL is the dev database of `my schema apply` on PostgreSQL; empty uses a
	// temporary schema of the database of DatabaseURL.
	DatabaseDevURL string
	Queue          string

	// The queue, the URL of the endpoints, the service account and audience of the OIDC tokens,
	// and the dispatch deadline of queue = cloudtasks. QueueCloudTasksURL has no trailing slash
	// and defaults to https://{host}; an empty audience is the URL of the endpoint called.
	QueueCloudTasksQueue            string
	QueueCloudTasksURL              string
	QueueCloudTasksServiceAccount   string
	QueueCloudTasksAudience         string
	QueueCloudTasksDispatchDeadline time.Duration
	// WorkerJobTypes and WorkerJobTypesExcept are the job types this process runs or skips, at
	// most one of them set (ADR-0004).
	WorkerJobTypes       []string
	WorkerJobTypesExcept []string
	// WorkerJobConcurrency is how many jobs this process runs at once (ADR-0004); 0 when the
	// setting is not given, which runs one and checks nothing against the database's pool.
	WorkerJobConcurrency int
	// DatabaseAcquireTimeout bounds the wait for a connection of the pool of database = postgres; 0
	// waits without a limit.
	DatabaseAcquireTimeout time.Duration
	// QueueCloudTasksRoutes send the wake-up signals of some job types to their own queue
	// (ADR-0004); read them with QueueRoutes.
	QueueCloudTasksRoutes []QueueRoute

	Blob string
	// MediaImage, MediaVideo, and MediaAudio select the processor of images, of videos, and of
	// audio-only files: native or plugin.
	MediaImage, MediaVideo, MediaAudio string
	// BlobFSDir is the directory of the fs blob store.
	BlobFSDir string
	// The bucket, endpoint, region, addressing, and credentials of the s3 blob store; no
	// credentials take the AWS SDK's default chain.
	BlobS3Bucket          string
	BlobS3Endpoint        string
	BlobS3Region          string
	BlobS3PathStyle       bool
	BlobS3AccessKeyID     string
	BlobS3SecretAccessKey string
	// The bucket of the gcs blob store, on Application Default Credentials.
	BlobGCSBucket string
	// Limits of image uploads and how long an upload may stay unattached.
	MediaMaxImageBytes     int64
	MediaMaxImageSide      int
	MediaMaxImagePixels    int64
	MediaUnattachedTTL     time.Duration
	MediaReplacedRetention time.Duration
	// Limits of video and audio uploads (ADR-0005 §9).
	MediaMaxAVBytes       int64
	MediaMaxVideoDuration time.Duration
	MediaMaxAudioDuration time.Duration
	// CustomEmojiMaxBytes bounds the image of a local custom emoji as uploaded.
	CustomEmojiMaxBytes int64
	// RemoteCustomEmojiMaxBytes bounds the image of a remote custom emoji that the media proxy
	// serves.
	RemoteCustomEmojiMaxBytes int64
	KV                        string
	// ValkeyURL, ValkeyTLSCAFile, and ValkeyKeyPrefix configure the Valkey server of kv = valkey.
	ValkeyURL       string
	ValkeyTLSCAFile string
	ValkeyKeyPrefix string
	// Stream is the broker of the real-time updates between processes (ADR-0050); the other
	// Stream fields configure it.
	Stream                         string
	StreamPostgresURL              string
	StreamSNSTopicARN              string
	StreamSQSQueuePrefix           string
	StreamPubSubTopic              string
	StreamPubSubSubscriptionPrefix string
	KeyManager                     string
	KEK                            []byte
	PreviousKEKs                   [][]byte
	// KMSKey and PreviousKMSKeys name the keys of the KMS of a key_manager other than local;
	// DEKCacheTTL and DEKCacheEntries bound the cache of the DEKs it unwraps (0 disables it).
	KMSKey          string
	PreviousKMSKeys []string
	DEKCacheTTL     time.Duration
	DEKCacheEntries int
	// Mail is the Mailer adapter; MailFrom and the SMTP fields configure it.
	Mail         string
	MailFrom     string
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	// SESRegion and SESConfigurationSet configure mail = ses.
	SESRegion           string
	SESConfigurationSet string
	// SESSNSTopicARN is the SNS topic of the SES notifications; set, it opens
	// POST /mail/events/ses (ADR-0086 §5).
	SESSNSTopicARN string
	// Captcha is the CAPTCHA service, or "none"; the keys configure it (ADR-0106).
	Captcha          string
	CaptchaSiteKey   string
	CaptchaSecretKey string
	// VAPIDPrivateKey is the server's Web Push key (FR-206) in base64url; "" turns Web Push off.
	VAPIDPrivateKey string
	// Profile is "production" or "development".
	Profile string
	// HTTPAllowPrivateNetworks lets the HTTPClient reach non-public destinations (development only).
	HTTPAllowPrivateNetworks bool
	// WebhookAllowedHosts and WebhookAllowedPrefixes are the destinations that the webhooks may
	// reach although the SSRF guard refuses them (webhook_allowed_destinations, ADR-0060).
	WebhookAllowedHosts    []string
	WebhookAllowedPrefixes []netip.Prefix
	// LoginProviderAllowedHosts and LoginProviderAllowedPrefixes are the destinations that the
	// requests to external identity providers may reach although the SSRF guard refuses them
	// (login_provider_allowed_destinations, ADR-0106 §5).
	LoginProviderAllowedHosts    []string
	LoginProviderAllowedPrefixes []netip.Prefix
	// PushAllowedHosts and PushAllowedPrefixes are the destinations that the Web Push sends may
	// reach although the SSRF guard refuses them (push_allowed_destinations, development only,
	// ADR-0015 §11).
	PushAllowedHosts    []string
	PushAllowedPrefixes []netip.Prefix
	// RelayIngestHourlyLimit is how many activities each relay may bring in per hour (ADR-0006).
	RelayIngestHourlyLimit int
	// BillingSources are the providers whose usage charges are imported (billing_sources,
	// ADR-0068), and the AWS caps bound the billed requests by UTC day and month (§4).
	BillingSources                []string
	BillingAWSMaxRequestsPerDay   int
	BillingAWSMaxRequestsPerMonth int
	// The AWS adapter's Cost Explorer metric, filter (none, linked_account, tag) with its tag
	// key=value or linked account IDs, and the IAM role assumed with the Google ID token on Cloud
	// Run (ADR-0068 §6, §8).
	BillingAWSMetric         string
	BillingAWSFilter         string
	BillingAWSTag            string
	BillingAWSLinkedAccounts []string
	BillingAWSRoleARN        string
	// The Google Cloud adapter's method (bigquery), the table of the Cloud Billing export, the
	// project that runs the queries (the table's when empty), the project IDs and the key=value
	// label that filter the rows, and the maximumBytesBilled of a query (ADR-0068 §4, §7).
	BillingGCPMethod             string
	BillingGCPTable              string
	BillingGCPQueryProject       string
	BillingGCPProjects           []string
	BillingGCPLabel              string
	BillingGCPMaxBytesBilled     int64
	BillingGCPBudgetSubscription string
	// Search is the search engine of the full-text search of statuses (ADR-0149 §3): "none" or a
	// backend's name.
	Search string
	// The cluster of search = opensearch: the URL of its REST API, the alias of the index, the
	// HTTP Basic authentication, and the AWS request signing ("off" or "es") with its region.
	// Empty is the default of each: opensearch.DefaultIndex, no signing, and the SDK's region.
	SearchOpenSearchURL   string
	SearchOpenSearchIndex string
	// SearchOpenSearchIndexSettingsFile is the path of the JSON file of the settings of the
	// index, empty for the adapter's own.
	SearchOpenSearchIndexSettingsFile string
	SearchOpenSearchUsername          string
	SearchOpenSearchPassword          string
	SearchOpenSearchAWSSigning        string
	SearchOpenSearchAWSRegion         string
	// PaymentsProvider is the payment provider of the supporters (ADR-0074 §5): "none" or a
	// provider's name. PaymentsReconcileInterval and PaymentsReconcileMaxRequests bound the
	// reconciliation with it (§4).
	PaymentsProvider string
	// PaymentsStripeSecretKey and PaymentsStripeWebhookSecret are the API key and the webhook
	// endpoint's signing secret of payments = stripe (ADR-0074 §5).
	PaymentsStripeSecretKey     string
	PaymentsStripeWebhookSecret string
	// PaymentsPayjpSecretKey, PaymentsPayjpPublicKey, and PaymentsPayjpWebhookToken are the keys
	// and the webhook token of payments = payjp (ADR-0074 §15).
	PaymentsPayjpSecretKey       string
	PaymentsPayjpPublicKey       string
	PaymentsPayjpWebhookToken    string
	PaymentsReconcileInterval    time.Duration
	PaymentsReconcileMaxRequests int
	// Plugins holds the destination of each delegated kind that has any setting, by the kind's name (ADR-0146 §5); read
	// it through PluginDestination.
	Plugins map[string]kind.Destination
	// PluginBillingLocation and PluginBillingProvider are what the billing source of a plugin says with no call: the
	// IANA time zone of its billing months and the provider's name that the totals show (ADR-0146 §11).
	PluginBillingLocation string
	PluginBillingProvider string
	// PluginTranslatorName is what readers see on a translation by the plugin of the translator kind (ADR-0148 §14).
	PluginTranslatorName string
	// Voice is how the voice chat rooms connect, "off" for no voice chat (ADR-0101 §1), and the
	// Voice* limits bound the rooms; zero is no limit, except for VoiceP2PMaxParticipants.
	Voice                   string
	VoiceP2PMaxParticipants int
	VoiceMaxParticipants    int
	VoiceMaxRoomDuration    time.Duration
	VoiceMaxOpenRooms       int
	// The STUN and TURN servers of voice = p2p (ADR-0101 §1, §5); VoiceTURN is "none", "coturn",
	// or "cloudflare", and VoiceICETransportPolicy "all" or "relay".
	VoiceSTUNURLs               []string
	VoiceTURN                   string
	VoiceTURNURLs               []string
	VoiceTURNSecret             string
	VoiceTURNCloudflareKeyID    string
	VoiceTURNCloudflareAPIToken string
	VoiceTURNCredentialTTL      time.Duration
	VoiceICETransportPolicy     string
	// The LiveKit server of voice = livekit (ADR-0101 §14); VoiceLiveKitAPIURL defaults to
	// VoiceLiveKitURL over https (http for ws).
	VoiceLiveKitURL       string
	VoiceLiveKitAPIURL    string
	VoiceLiveKitAPIKey    string
	VoiceLiveKitAPISecret string
	// The Sora server of voice = sora (ADR-0101 §14): VoiceSoraMode is "self_hosted" or "cloud",
	// and VoiceJoinSecret signs the join values that Sora sends back on each connection.
	VoiceSoraMode            string
	VoiceSoraSignalingURLs   []string
	VoiceSoraAPIURL          string
	VoiceSoraAPIHeaderName   string
	VoiceSoraAPIHeaderValue  string
	VoiceSoraWebhookUsername string
	VoiceSoraWebhookPassword string
	VoiceSoraCloudProjectID  string
	VoiceSoraCloudAPIKey     string
	VoiceJoinSecret          string
	// DeadJobRetention is how long a dead job is kept after it failed (FR-803).
	DeadJobRetention time.Duration
	// JobAttemptRetention is how long the record of a job's run that did not succeed is kept
	// after the run (ADR-0004 §6).
	JobAttemptRetention time.Duration
	// DynamicClientUnredeemedTTL is how long a dynamically registered OAuth client may wait for
	// the exchange of its first authorization code, and DynamicClientIdleTTL how long one
	// without a usable token may go unused, before the clean-up deletes it (ADR-0090 §1).
	DynamicClientUnredeemedTTL time.Duration
	DynamicClientIdleTTL       time.Duration
	// MetricsExporter is the Meter adapter (ADR-0055 §2); MetricsListen is the address of the
	// scrape of prometheus, and the EMF fields configure emf.
	MetricsExporter      string
	MetricsListen        string
	MetricsEMFNamespace  string
	MetricsEMFDimensions []string
	// TracesExporter is the Tracer adapter, TracesSampleRatio the share of the root spans that
	// are recorded, and TracesFlush when the spans are sent (ADR-0055 §17).
	TracesExporter    string
	TracesSampleRatio float64
	TracesFlush       string
	// ErrorsExporter is where the error reports also go, ErrorsSentryDSN the DSN of sentry, and
	// ErrorsEnvironment the environment that every report carries (ADR-0103 §9).
	ErrorsExporter    string
	ErrorsSentryDSN   string
	ErrorsEnvironment string
}

// ServerOrigin returns the origin at which browsers reach the server itself: its sign-in screens,
// client API, media and email links. It is web_origin when web_origin is on host, so that
// http://localhost:8080 runs everything over plain HTTP on localhost, and https://{host} otherwise.
// ActivityPub ids stay under https://{host}/ whatever it returns (ADR-0018).
func (c *Config) ServerOrigin() string {
	if u, err := url.Parse(c.WebOrigin); err == nil && u.Scheme == "http" && strings.EqualFold(u.Host, c.Host) {
		return "http://" + c.Host
	}
	return "https://" + c.Host
}

// MediaBaseURL returns the origin of the URLs of the local media and the media proxy: media_origin,
// or ServerOrigin() when it is unset. ActivityPub ids are not media URLs and never use it (ADR-0018).
func (c *Config) MediaBaseURL() string {
	if c.MediaOrigin != "" {
		return c.MediaOrigin
	}
	return c.ServerOrigin()
}

// LogValue logs the configuration with its secrets redacted (ADR-0019 §6).
func (c *Config) LogValue() slog.Value {
	redact := func(set bool) string {
		if set {
			return "(redacted)"
		}
		return ""
	}
	return slog.GroupValue(
		slog.String("host", c.Host),
		slog.String("account_domain", c.AccountDomain),
		slog.String("source_url", c.SourceURL),
		slog.String("status_page_url", c.StatusPageURL),
		slog.Bool("maintenance", c.Maintenance),
		slog.String("maintenance_server_name", c.MaintenanceServerName),
		slog.Int("maintenance_message_length", utf8.RuneCountInString(c.MaintenanceMessage)),
		slog.Time("maintenance_until", c.MaintenanceUntil),
		slog.Duration("maintenance_retry_after", c.MaintenanceRetryAfter),
		slog.String("web_origin", c.WebOrigin),
		slog.String("origin_secret", redact(c.OriginSecret != "")),
		slog.String("origin_secret_previous", redact(c.OriginSecretPrevious != "")),
		slog.String("role", string(c.Role)),
		slog.String("listen", c.Listen),
		slog.Any("trusted_proxies", c.TrustedProxies),
		slog.String("trusted_proxy_header", c.TrustedProxyHeader),
		slog.String("database", c.DB),
		slog.String("database_url", redact(c.DatabaseURL != "")),
		slog.String("database_dev_url", redact(c.DatabaseDevURL != "")),
		slog.String("queue", c.Queue),
		slog.String("worker_job_types", strings.Join(c.WorkerJobTypes, ",")),
		slog.String("worker_job_types_except", strings.Join(c.WorkerJobTypesExcept, ",")),
		slog.Int("worker_job_concurrency", max(c.WorkerJobConcurrency, 1)),
		slog.String("queue_cloudtasks_queue", c.QueueCloudTasksQueue),
		slog.String("queue_cloudtasks_url", c.QueueCloudTasksURL),
		slog.String("queue_cloudtasks_service_account", c.QueueCloudTasksServiceAccount),
		slog.String("queue_cloudtasks_audience", c.QueueCloudTasksAudience),
		slog.Duration("queue_cloudtasks_dispatch_deadline", c.QueueCloudTasksDispatchDeadline),
		slog.Duration("database_acquire_timeout", c.DatabaseAcquireTimeout),
		slog.String("blob", c.Blob),
		slog.String("media_image", c.MediaImage),
		slog.String("media_video", c.MediaVideo),
		slog.String("media_audio", c.MediaAudio),
		slog.String("blob_fs_dir", c.BlobFSDir),
		slog.String("blob_s3_bucket", c.BlobS3Bucket),
		slog.String("blob_s3_endpoint", c.BlobS3Endpoint),
		slog.String("blob_s3_region", c.BlobS3Region),
		slog.Bool("blob_s3_path_style", c.BlobS3PathStyle),
		slog.String("blob_s3_access_key_id", c.BlobS3AccessKeyID),
		slog.String("blob_s3_secret_access_key", redact(c.BlobS3SecretAccessKey != "")),
		slog.String("blob_gcs_bucket", c.BlobGCSBucket),
		slog.String("media_origin", c.MediaOrigin),
		slog.Int64("media_max_image_bytes", c.MediaMaxImageBytes),
		slog.Int("media_max_image_side", c.MediaMaxImageSide),
		slog.Int64("media_max_image_pixels", c.MediaMaxImagePixels),
		slog.Duration("media_unattached_retention", c.MediaUnattachedTTL),
		slog.Duration("media_replaced_retention", c.MediaReplacedRetention),
		slog.Int64("media_max_av_bytes", c.MediaMaxAVBytes),
		slog.Duration("media_max_video_duration", c.MediaMaxVideoDuration),
		slog.Duration("media_max_audio_duration", c.MediaMaxAudioDuration),
		slog.Int64("custom_emoji_max_bytes", c.CustomEmojiMaxBytes),
		slog.Int64("remote_custom_emoji_max_bytes", c.RemoteCustomEmojiMaxBytes),
		slog.String("kv", c.KV),
		slog.String("valkey_url", redact(c.ValkeyURL != "")),
		slog.String("valkey_tls_ca_file", c.ValkeyTLSCAFile),
		slog.String("valkey_key_prefix", c.ValkeyKeyPrefix),
		slog.String("stream", c.Stream),
		slog.String("stream_postgres_url", redact(c.StreamPostgresURL != "")),
		slog.String("stream_sns_topic_arn", c.StreamSNSTopicARN),
		slog.String("stream_sns_queue_prefix", c.StreamSQSQueuePrefix),
		slog.String("stream_pubsub_topic", c.StreamPubSubTopic),
		slog.String("stream_pubsub_subscription_prefix", c.StreamPubSubSubscriptionPrefix),
		slog.String("key_manager", c.KeyManager),
		slog.String("kek", redact(len(c.KEK) > 0)),
		slog.Int("kek_previous", len(c.PreviousKEKs)),
		slog.String("kms_key", c.KMSKey),
		slog.Int("kms_key_previous", len(c.PreviousKMSKeys)),
		slog.Duration("kms_dek_cache_ttl", c.DEKCacheTTL),
		slog.Int("kms_dek_cache_entries", c.DEKCacheEntries),
		slog.String("mail", c.Mail),
		slog.String("mail_from", c.MailFrom),
		slog.String("mail_smtp_host", c.SMTPHost),
		slog.Int("mail_smtp_port", c.SMTPPort),
		slog.String("mail_smtp_username", c.SMTPUsername),
		slog.String("mail_smtp_password", redact(c.SMTPPassword != "")),
		slog.String("mail_ses_region", c.SESRegion),
		slog.String("mail_ses_configuration_set", c.SESConfigurationSet),
		slog.String("mail_ses_sns_topic_arn", c.SESSNSTopicARN),
		slog.String("captcha", c.Captcha),
		slog.String("captcha_site_key", c.CaptchaSiteKey),
		slog.String("captcha_secret_key", redact(c.CaptchaSecretKey != "")),
		slog.String("vapid_private_key", redact(c.VAPIDPrivateKey != "")),
		slog.String("profile", c.Profile),
		slog.Bool("http_allow_private_networks", c.HTTPAllowPrivateNetworks),
		slog.Any("webhook_allowed_destinations", append(slices.Clone(c.WebhookAllowedHosts), prefixStrings(c.WebhookAllowedPrefixes)...)),
		slog.Any("login_provider_allowed_destinations", append(slices.Clone(c.LoginProviderAllowedHosts), prefixStrings(c.LoginProviderAllowedPrefixes)...)),
		slog.Any("push_allowed_destinations", append(slices.Clone(c.PushAllowedHosts), prefixStrings(c.PushAllowedPrefixes)...)),
		slog.Int("relay_max_activities_per_hour", c.RelayIngestHourlyLimit),
		slog.Any("billing_sources", c.BillingSources),
		slog.Int("billing_aws_max_requests_per_day", c.BillingAWSMaxRequestsPerDay),
		slog.Int("billing_aws_max_requests_per_month", c.BillingAWSMaxRequestsPerMonth),
		slog.String("billing_aws_metric", c.BillingAWSMetric),
		slog.String("billing_aws_filter", c.BillingAWSFilter),
		slog.String("billing_aws_tag", c.BillingAWSTag),
		slog.Any("billing_aws_linked_accounts", c.BillingAWSLinkedAccounts),
		slog.String("billing_aws_role_arn", c.BillingAWSRoleARN),
		slog.String("billing_gcp_method", c.BillingGCPMethod),
		slog.String("billing_gcp_table", c.BillingGCPTable),
		slog.String("billing_gcp_query_project", c.BillingGCPQueryProject),
		slog.Any("billing_gcp_projects", c.BillingGCPProjects),
		slog.String("billing_gcp_label", c.BillingGCPLabel),
		slog.Int64("billing_gcp_max_bytes_billed", c.BillingGCPMaxBytesBilled),
		slog.String("billing_gcp_budget_subscription", c.BillingGCPBudgetSubscription),
		slog.String("plugin_billing_location", c.PluginBillingLocation),
		slog.String("plugin_billing_provider", c.PluginBillingProvider),
		slog.String("plugin_translator_name", c.PluginTranslatorName),
		slog.String("payments", c.PaymentsProvider),
		slog.String("payments_stripe_secret_key", redact(c.PaymentsStripeSecretKey != "")),
		slog.String("payments_stripe_webhook_secret", redact(c.PaymentsStripeWebhookSecret != "")),
		slog.String("payments_payjp_secret_key", redact(c.PaymentsPayjpSecretKey != "")),
		slog.String("payments_payjp_public_key", c.PaymentsPayjpPublicKey),
		slog.String("payments_payjp_webhook_token", redact(c.PaymentsPayjpWebhookToken != "")),
		slog.Duration("payments_reconcile_interval", c.PaymentsReconcileInterval),
		slog.Int("payments_reconcile_max_requests", c.PaymentsReconcileMaxRequests),
		slog.String("search", c.Search),
		slog.String("search_opensearch_url", c.SearchOpenSearchURL),
		slog.String("search_opensearch_index", c.SearchOpenSearchIndex),
		slog.String("search_opensearch_index_settings_file", c.SearchOpenSearchIndexSettingsFile),
		slog.String("search_opensearch_username", c.SearchOpenSearchUsername),
		slog.String("search_opensearch_password", redact(c.SearchOpenSearchPassword != "")),
		slog.String("search_opensearch_aws_signing", c.SearchOpenSearchAWSSigning),
		slog.String("search_opensearch_aws_region", c.SearchOpenSearchAWSRegion),
		slog.String("voice", c.Voice),
		slog.Int("voice_p2p_max_participants", c.VoiceP2PMaxParticipants),
		slog.Int("voice_sfu_max_participants", c.VoiceMaxParticipants),
		slog.Duration("voice_max_room_duration", c.VoiceMaxRoomDuration),
		slog.Int("voice_max_open_rooms", c.VoiceMaxOpenRooms),
		slog.Any("voice_stun_urls", c.VoiceSTUNURLs),
		slog.String("voice_turn", c.VoiceTURN),
		slog.Any("voice_turn_coturn_urls", c.VoiceTURNURLs),
		slog.String("voice_turn_coturn_secret", redact(c.VoiceTURNSecret != "")),
		slog.String("voice_turn_cloudflare_key_id", c.VoiceTURNCloudflareKeyID),
		slog.String("voice_turn_cloudflare_api_token", redact(c.VoiceTURNCloudflareAPIToken != "")),
		slog.Duration("voice_turn_credential_ttl", c.VoiceTURNCredentialTTL),
		slog.String("voice_ice_transport_policy", c.VoiceICETransportPolicy),
		slog.String("voice_livekit_url", c.VoiceLiveKitURL),
		slog.String("voice_livekit_api_url", c.VoiceLiveKitAPIURL),
		slog.String("voice_livekit_api_key", c.VoiceLiveKitAPIKey),
		slog.String("voice_livekit_api_secret", redact(c.VoiceLiveKitAPISecret != "")),
		slog.String("voice_sora_mode", c.VoiceSoraMode),
		slog.Any("voice_sora_signaling_urls", c.VoiceSoraSignalingURLs),
		slog.String("voice_sora_api_url", c.VoiceSoraAPIURL),
		slog.String("voice_sora_api_header_name", c.VoiceSoraAPIHeaderName),
		slog.String("voice_sora_api_header_value", redact(c.VoiceSoraAPIHeaderValue != "")),
		slog.String("voice_sora_webhook_username", c.VoiceSoraWebhookUsername),
		slog.String("voice_sora_webhook_password", redact(c.VoiceSoraWebhookPassword != "")),
		slog.String("voice_sora_cloud_project_id", c.VoiceSoraCloudProjectID),
		slog.String("voice_sora_cloud_api_key", redact(c.VoiceSoraCloudAPIKey != "")),
		slog.String("voice_sora_join_secret", redact(c.VoiceJoinSecret != "")),
		slog.Duration("dead_job_retention", c.DeadJobRetention),
		slog.Duration("job_attempt_retention", c.JobAttemptRetention),
		slog.Duration("dynamic_client_unredeemed_retention", c.DynamicClientUnredeemedTTL),
		slog.Duration("dynamic_client_idle_retention", c.DynamicClientIdleTTL),
		slog.String("metrics_exporter", c.MetricsExporter),
		slog.String("metrics_listen", c.MetricsListen),
		slog.String("metrics_emf_namespace", c.MetricsEMFNamespace),
		slog.Any("metrics_emf_dimensions", c.MetricsEMFDimensions),
		slog.String("traces_exporter", c.TracesExporter),
		slog.Float64("traces_sample_ratio", c.TracesSampleRatio),
		slog.String("traces_flush", c.TracesFlush),
		slog.String("errors_exporter", c.ErrorsExporter),
		slog.String("errors_sentry_dsn", redact(c.ErrorsSentryDSN != "")),
		slog.String("errors_environment", c.ErrorsEnvironment),
	)
}

// Origin describes where a resolved value came from.
type Origin string

// Origins.
const (
	OriginUnset   Origin = "unset"
	OriginDefault Origin = "default"
	OriginFile    Origin = "file"
	OriginEnv     Origin = "env"
)

// Setting is one resolved setting, for display by `my config check`.
type Setting struct {
	Key    string
	Env    string
	Value  string // "" when unset; redacted for secrets
	Secret bool
	Origin Origin
}

// Issue is one validation problem.
type Issue struct {
	// Key is the setting the problem is about, or "" for a problem with the config file itself.
	Key string
	// Missing reports that the setting Key is required but not set; otherwise an issue
	// with a Key is about a value that is set but invalid.
	Missing bool
	// Message is the technical detail in English; it may be empty for a Missing issue.
	Message string
}

// Env returns the environment variable of the setting Key, or "" when there is none.
func (i Issue) Env() string {
	for k := range schema {
		if schema[k].key == i.Key {
			return schema[k].env()
		}
	}
	return ""
}

func (i Issue) String() string {
	if i.Key == "" {
		return i.Message
	}
	if i.Missing && i.Message == "" {
		i.Message = "required"
	}
	if env := i.Env(); env != "" {
		return fmt.Sprintf("%s (%s): %s", i.Key, env, i.Message)
	}
	return fmt.Sprintf("%s: %s", i.Key, i.Message)
}

// ValidationError lists every problem found while loading.
type ValidationError struct {
	Issues []Issue
}

func (e *ValidationError) Error() string {
	lines := make([]string, len(e.Issues))
	for i, is := range e.Issues {
		lines[i] = is.String()
	}
	return "invalid configuration:\n  " + strings.Join(lines, "\n  ")
}

// Source supplies the raw inputs. Zero fields fall back to the process environment and file system.
type Source struct {
	// LookupEnv reads an environment variable; defaults to os.LookupEnv.
	LookupEnv func(string) (string, bool)
	// EnvNames lists the names of the environment variables, for finding the retired and
	// unknown ones; defaults to the names of os.Environ when LookupEnv is nil too, and to none
	// otherwise.
	EnvNames func() []string
	// ReadFile reads a file; defaults to os.ReadFile.
	ReadFile func(string) ([]byte, error)
	// File is the TOML file path; when empty, MY_CONFIG is used, and no file is read if that is unset too.
	File string
}

// Load resolves, validates, and returns the configuration together with the
// resolved settings. On failure the error is a *ValidationError listing every
// issue, and the settings are still returned for display.
func Load(src Source) (*Config, []Setting, error) { return load(src, nil) }

// LoadMaintenance is Load for the maintenance mode (ADR-0088 §9): it reads and validates only the
// settings marked for it (the listen address, the host and origins, the proxies, status_page_url,
// and the maintenance_* settings), so that a configuration whose adapter settings are wrong or
// unreachable still starts. The settings it does not read are absent from the returned list.
func LoadMaintenance(src Source) (*Config, []Setting, error) { return load(src, maintenanceKeys) }

// LoadSchemaApply is Load for `my schema apply`: it reads and validates only database, database_url,
// and database_dev_url, so that the one-off job that applies the schema needs nothing but the
// database's settings. The settings it does not read are absent from the returned list.
func LoadSchemaApply(src Source) (*Config, []Setting, error) { return load(src, schemaApplyKeys) }

// MaintenanceRequested reads only the maintenance setting, from the environment or the TOML
// file, for my serve to choose between the normal start and the maintenance mode before loading
// the rest. An invalid value or an unreadable file is an error.
func MaintenanceRequested(src Source) (bool, error) {
	v, err := single(src, "maintenance")
	return v == "true", err
}

// ProfileRequested reads only the profile setting, from the environment or the TOML file, for the
// commands of the test builds (build tag e2e_fakes) that refuse every profile but development
// without needing the rest of the server's configuration. An invalid value or an unreadable file
// is an error.
func ProfileRequested(src Source) (string, error) { return single(src, "profile") }

// single resolves and validates the one setting key, ignoring the others.
func single(src Source, key string) (string, error) {
	src, file, issues := open(src)
	f := &schema[slices.IndexFunc(schema, func(f field) bool { return f.key == key })]
	v, _, srcIssues := resolve(f, src, file)
	issues = append(issues, srcIssues...)
	if len(issues) == 0 {
		if err := validate(f, v); err != nil {
			msg := err.Error()
			if f.secret {
				msg = redactSecret(msg, v)
			}
			issues = append(issues, Issue{Key: f.key, Message: msg})
		}
	}
	if len(issues) > 0 {
		return "", &ValidationError{Issues: issues}
	}
	return v, nil
}

// withDefaults fills the zero fields of src.
func withDefaults(src Source) Source {
	if src.EnvNames == nil {
		src.EnvNames = func() []string { return nil }
		if src.LookupEnv == nil {
			src.EnvNames = osEnvNames
		}
	}
	if src.LookupEnv == nil {
		src.LookupEnv = os.LookupEnv
	}
	if src.ReadFile == nil {
		src.ReadFile = os.ReadFile
	}
	return src
}

// osEnvNames returns the names of the variables of the process environment.
func osEnvNames() []string {
	env := os.Environ()
	names := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		names = append(names, name)
	}
	return names
}

// open fills the defaults of src and reads its TOML file. A retired key in the file or the
// environment, and an unknown key in the file, is an issue.
func open(src Source) (Source, map[string]any, []Issue) {
	src = withDefaults(src)
	var issues []Issue

	file := map[string]any{}
	path := src.File
	if path == "" {
		path, _ = src.LookupEnv(EnvConfigFile)
	}
	if path != "" {
		data, err := src.ReadFile(path)
		if err != nil {
			issues = append(issues, Issue{Message: fmt.Sprintf("config file: %v", err)})
		} else if err := toml.Unmarshal(data, &file); err != nil {
			issues = append(issues, Issue{Message: fmt.Sprintf("config file %s: %v", path, err)})
		}
	}
	issues = append(issues, retiredIssues(path, file, src.EnvNames())...)
	for _, k := range unknownKeys(file) {
		issues = append(issues, Issue{Message: fmt.Sprintf("config file %s: unknown key %q", path, k)})
	}
	return src, file, issues
}

// load reads every setting, or, when only is not nil, only the settings it holds.
func load(src Source, only map[string]bool) (*Config, []Setting, error) {
	src, file, issues := open(src)
	cfg := &Config{}
	settings := make([]Setting, 0, len(schema))
	valid := map[string]string{}
	present := map[string]bool{}
	for i := range schema {
		f := &schema[i]
		if only != nil && !only[f.key] {
			continue
		}
		v, origin, srcIssues := resolve(f, src, file)
		issues = append(issues, srcIssues...)
		s := Setting{Key: f.key, Env: f.env(), Value: v, Secret: f.secret, Origin: origin}
		if f.secret && v != "" {
			s.Value = "(redacted)"
		}
		settings = append(settings, s)
		if len(srcIssues) > 0 {
			present[f.key] = true
			continue
		}
		if v == "" {
			if f.required {
				issues = append(issues, Issue{Key: f.key, Missing: true})
			}
			continue
		}
		present[f.key] = true
		if err := validate(f, v); err != nil {
			msg := err.Error()
			if f.secret {
				msg = redactSecret(msg, v)
			}
			issues = append(issues, Issue{Key: f.key, Message: msg})
			continue
		}
		valid[f.key] = v
		f.set(cfg, v)
	}
	if only == nil {
		issues = append(issues, crossChecks(valid, present)...)
	}
	if only == nil || only["origin_secret"] {
		issues = append(issues, originChecks(valid, present)...)
	}
	if cfg.AccountDomain == "" {
		cfg.AccountDomain = cfg.Host
	}
	if cfg.ErrorsEnvironment == "" {
		cfg.ErrorsEnvironment = cfg.Profile
	}
	if cfg.WebOrigin == "" {
		cfg.WebOrigin = "https://" + cfg.Host
	}
	if cfg.DB == "postgres" && !present["database_acquire_timeout"] {
		cfg.DatabaseAcquireTimeout = DefaultDatabaseAcquireTimeout
	}
	if cfg.Queue == "cloudtasks" {
		cfg.QueueCloudTasksURL = cmp.Or(cfg.QueueCloudTasksURL, "https://"+cfg.Host)
		cfg.QueueCloudTasksDispatchDeadline = cmp.Or(cfg.QueueCloudTasksDispatchDeadline, DefaultCloudTasksDispatchDeadline)
	}
	if cfg.VoiceLiveKitAPIURL == "" {
		cfg.VoiceLiveKitAPIURL = voiceAPIURL(cfg.VoiceLiveKitURL)
	}
	if len(issues) > 0 {
		return nil, settings, &ValidationError{Issues: issues}
	}
	return cfg, settings, nil
}

// resolve returns a setting's raw value and origin, following the precedence env > file > default.
func resolve(f *field, src Source, file map[string]any) (string, Origin, []Issue) {
	var issues []Issue
	fileVal, inFile := file[f.key]
	if f.secret {
		if inFile {
			issues = append(issues, Issue{Key: f.key, Message: fmt.Sprintf("is a secret; set %s or %s_FILE instead of the config file", f.env(), f.env())})
		}
		v, ok := src.LookupEnv(f.env())
		pathV, pathOK := src.LookupEnv(f.env() + "_FILE")
		switch {
		case ok && pathOK:
			issues = append(issues, Issue{Key: f.key, Message: fmt.Sprintf("set only one of %s and %s_FILE", f.env(), f.env())})
		case pathOK:
			data, err := src.ReadFile(pathV)
			if err != nil {
				issues = append(issues, Issue{Key: f.key, Message: fmt.Sprintf("%s_FILE: %v", f.env(), err)})
				break
			}
			return strings.TrimRight(string(data), "\r\n"), OriginEnv, issues
		case ok:
			return v, OriginEnv, issues
		}
		return "", OriginUnset, issues
	}
	if v, ok := src.LookupEnv(f.env()); ok {
		return v, OriginEnv, nil
	}
	if inFile {
		switch v := fileVal.(type) {
		case string:
			return v, OriginFile, nil
		case int64:
			return strconv.FormatInt(v, 10), OriginFile, nil
		case bool:
			return strconv.FormatBool(v), OriginFile, nil
		}
		return "", OriginFile, []Issue{{Key: f.key, Message: fmt.Sprintf("must be a string, a whole number, or a boolean in the config file, got %T", fileVal)}}
	}
	if f.def != "" {
		return f.def, OriginDefault, nil
	}
	return "", OriginUnset, nil
}

func validate(f *field, v string) error {
	if len(f.choices) > 0 {
		if !slices.ContainsFunc(f.choices, func(c choice) bool { return c.value == v }) {
			return fmt.Errorf("unknown value %q; want one of %s", v, strings.Join(allowedValues(f), ", "))
		}
		if tag, ok := excludedValues[f.key][v]; ok {
			return excludedError(v, tag)
		}
		if tag, ok := testOnlyValues[f.key][v]; ok {
			return testOnlyError(v, tag)
		}
	}
	if f.check != nil {
		return f.check(v)
	}
	return nil
}

// redactSecret removes from msg the value of a secret setting, and the userinfo and the password
// of a URL value, which the error of a URL parser quotes verbatim.
func redactSecret(msg, v string) string {
	msg = strings.ReplaceAll(msg, v, "(redacted)")
	if u, err := url.Parse(v); err == nil && u.User != nil {
		if pw, ok := u.User.Password(); ok && pw != "" {
			msg = strings.ReplaceAll(msg, pw, "(redacted)")
		}
	}
	return msg
}

func allowedValues(f *field) []string {
	var vs []string
	for _, c := range f.choices {
		if _, testOnly := testOnlyValues[f.key][c.value]; !testOnly {
			vs = append(vs, c.value)
		}
	}
	return vs
}

func unknownKeys(file map[string]any) []string {
	var unknown []string
	for k := range file {
		if !slices.ContainsFunc(schema, func(f field) bool { return f.key == k }) &&
			!slices.ContainsFunc(retiredKeys, func(r retiredKey) bool { return r.key == k }) {
			unknown = append(unknown, k)
		}
	}
	slices.Sort(unknown)
	return unknown
}

// prefixStrings returns the text of prefixes.
func prefixStrings(prefixes []netip.Prefix) []string {
	out := make([]string, len(prefixes))
	for i, p := range prefixes {
		out[i] = p.String()
	}
	return out
}
