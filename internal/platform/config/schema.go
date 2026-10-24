// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"cmp"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net"
	"net/mail"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/adapter/webpush"
	"github.com/mycroblog/my/internal/app/supporters"
)

// EnvPrefix is prepended to the upper-cased key to form a setting's environment variable.
const EnvPrefix = "MY_"

// EnvConfigFile names the environment variable that points at the optional TOML file.
const EnvConfigFile = EnvPrefix + "CONFIG"

// DefaultSourceURL is the repository of the unmodified server, the default of source_url.
const DefaultSourceURL = "https://github.com/mycroblog/my"

// kekSize is the length in bytes of the AES-256 key-encryption key (ADR-0019 §6).
const kekSize = 32

// choice is one allowed value of an enumerated setting.
type choice struct {
	value string
	// inV0 reports whether v0 ships this value. Values outside v0 are documented
	// (they are decided in an ADR) but rejected by validation.
	inV0 bool
	doc  string
}

// field describes one setting. The schema is the single source of truth for
// loading, validation, and the generated reference (docs/reference/configuration.md).
type field struct {
	// key is the TOML key; the environment variable is EnvPrefix + upper(key).
	key string
	doc string
	// def is the default value; empty means no default.
	def string
	// defDoc documents a default computed from other settings (def is then empty).
	defDoc   string
	required bool
	// requiredWhen documents a condition under which the setting is required (checked in crossChecks).
	requiredWhen string
	// secret settings are read only from the environment (NAME or NAME_FILE)
	// and are never printed (ADR-0010, ADR-0019 §6).
	secret  bool
	choices []choice
	// check validates a non-empty value beyond the choices.
	check func(string) error
	// set stores a validated value in the Config.
	set func(*Config, string)
}

// env returns the environment variable that sets f.
func (f *field) env() string { return EnvPrefix + strings.ToUpper(f.key) }

// Keys returns the key of every setting, in documentation order.
func Keys() []string {
	keys := make([]string, len(schema))
	for i := range schema {
		keys[i] = schema[i].key
	}
	return keys
}

// schema lists every setting in documentation order.
var schema = []field{
	{
		key:      "host",
		doc:      "Domain that serves the instance. Every ActivityPub `id` is under `https://{host}/` (ADR-0018). A port is allowed for local development. Recorded in the database on first start and must not change afterwards.",
		required: true,
		check:    checkDomain,
		set:      func(c *Config, v string) { c.Host = v },
	},
	{
		key:    "account_domain",
		defDoc: "value of `host`",
		doc:    "Domain of user handles (`@alice@{account_domain}`). Defaults to `host`. When it differs, `account_domain` must redirect `/.well-known/webfinger`, `/.well-known/host-meta`, and `/.well-known/nodeinfo` to `host` (ADR-0018). Recorded in the database on first start and must not change afterwards.",
		check:  checkDomain,
		set:    func(c *Config, v string) { c.AccountDomain = v },
	},
	{
		key:   "source_url",
		doc:   "Where users get the source code of the running server, as an `https` URL or a `mailto:` address to request it from. AGPL-3.0 section 13 (ADR-0011) requires offering the source to everyone who uses the server over the network; `GET /instance`, the information page (`/about`), the landing page, and NodeInfo (`metadata.sourceCode`, and `software.repository` for an `https` URL) publish it. Point it at your own source when you run a fork or a modified version (docs/operations/running.md).",
		def:   DefaultSourceURL,
		check: checkSourceURL,
		set:   func(c *Config, v string) { c.SourceURL = v },
	},
	{
		key:    "web_origin",
		defDoc: "`https://{host}`",
		doc:    "Origin (`scheme://host[:port]`) of the web client. The client API allows only this origin by CORS (ADR-0019 §4). Set it when the web client is served from another origin, for example a development server at `http://localhost:5173`. An origin on `host` is where the server itself serves the web client, and the client served there calls the API at it: set `http://localhost:8080` (with `host` = `localhost:8080`) to run the server over plain HTTP on localhost, where the sign-in screens, the OAuth issuer, the client API, the links to the instance's pages in its responses, media URLs, and email and invite links are all on `http://localhost:8080`. ActivityPub ids stay under `https://{host}/` in every setup.",
		check:  checkOrigin,
		set:    func(c *Config, v string) { c.WebOrigin = v },
	},
	{
		key: "role",
		doc: "What this process runs.",
		def: string(RoleAll),
		choices: []choice{
			{value: string(RoleAll), inV0: true, doc: "HTTP server and job worker in one process."},
			{value: string(RoleWeb), inV0: true, doc: "HTTP server only."},
			{value: string(RoleWorker), inV0: true, doc: "Job worker only; the HTTP server answers only the health checks (docs/operations/running.md)."},
		},
		set: func(c *Config, v string) { c.Role = Role(v) },
	},
	{
		key:   "listen",
		doc:   "TCP address the HTTP server listens on, as `host:port`.",
		def:   ":8080",
		check: checkListen,
		set:   func(c *Config, v string) { c.Listen = v },
	},
	{
		key:   "trusted_proxies",
		doc:   "Reverse proxies and load balancers in front of the server whose forwarding header (`trusted_proxy_header`) tells the client IP, as comma-separated CIDRs or addresses such as `10.0.0.0/8, 2001:db8::/32, 192.0.2.10` (ADR-0019 §5). The client IP keys the rate limits of sign-in, registration, the client API, and the inbox. When the connection's peer is in this list, the client IP is the rightmost address of the header that is not in it; otherwise the header is ignored and the client IP is the peer. Empty trusts no one: behind a proxy every request then shares the proxy's limits. List only addresses that nothing but your proxies can connect from, and only proxies that append to the header rather than pass a client's value through (docs/architecture/deployment-targets.md).",
		check: checkTrustedProxies,
		set:   func(c *Config, v string) { c.TrustedProxies = parseTrustedProxies(v) },
	},
	{
		key: "trusted_proxy_header",
		doc: "Forwarding header that the `trusted_proxies` append the client IP to. Only this header is read, so that a client cannot add the other one past a proxy that does not know it.",
		def: "x-forwarded-for",
		choices: []choice{
			{value: "x-forwarded-for", inV0: true, doc: "`X-Forwarded-For`, which nginx, Caddy, Traefik, Google Cloud load balancers and Cloud Run, and AWS Application Load Balancers append to."},
			{value: "forwarded", inV0: true, doc: "The `for` parameters of `Forwarded` (RFC 7239)."},
		},
		set: func(c *Config, v string) { c.TrustedProxyHeader = v },
	},
	{
		key: "db",
		doc: "Database adapter (ADR-0003). The adapter is never inferred from `database_url`.",
		def: "sqlite",
		choices: []choice{
			{value: "sqlite", inV0: true, doc: "Local SQLite file (`modernc.org/sqlite`). The build tag `no_sqlite` leaves it out."},
			{value: "libsql", inV0: true, doc: "libSQL / Turso."},
			{value: "postgres", inV0: true, doc: "PostgreSQL and compatible services (`pgx/v5`). The build tag `no_postgres` leaves it out."},
		},
		set: func(c *Config, v string) { c.DB = v },
	},
	{
		key:      "database_url",
		doc:      "Connection string passed to the `db` adapter, in that adapter's format.",
		required: true,
		secret:   true,
		set:      func(c *Config, v string) { c.DatabaseURL = v },
	},
	{
		key: "queue",
		doc: "Job queue adapter (ADR-0004).",
		def: "db",
		choices: []choice{
			{value: "db", inV0: true, doc: "Queue table in the database."},
			{value: "inline", inV0: true, doc: "Runs jobs synchronously in the enqueuing process; for tests and development. Requires `role = all`."},
			{value: "sqs", inV0: true, doc: "Amazon SQS."},
			{value: "cloudtasks", inV0: true, doc: "Google Cloud Tasks."},
		},
		set: func(c *Config, v string) { c.Queue = v },
	},
	{
		key: "blob",
		doc: "Blob storage adapter for media (ADR-0005).",
		def: "fs",
		choices: []choice{
			{value: "fs", inV0: true, doc: "Local file system."},
			{value: "s3", inV0: true, doc: "S3-compatible API (AWS S3, Cloudflare R2, GCS XML API, MinIO)."},
			{value: "gcs", doc: "Native Google Cloud Storage API."},
		},
		set: func(c *Config, v string) { c.Blob = v },
	},
	{
		key: "media",
		doc: "Media processor adapter (ADR-0005).",
		def: "native",
		choices: []choice{
			{value: "native", inV0: true, doc: "Pure Go image processing."},
			{value: "vips", inV0: true, doc: "libvips; needs a cgo build with the adapter's build tag."},
			{value: "none", inV0: true, doc: "No processing except EXIF removal."},
		},
		set: func(c *Config, v string) { c.Media = v },
	},
	{
		key:   "blob_fs_dir",
		doc:   "Directory of the `fs` blob store, relative to the working directory or absolute. It holds the uploads as received (private) and the processed media, which the server serves under `/media/`. Share it between the web and worker processes, and back it up with the database.",
		def:   "blobs",
		check: checkNotBlank,
		set:   func(c *Config, v string) { c.BlobFSDir = v },
	},
	{
		key:   "media_max_image_bytes",
		doc:   "Largest accepted image upload in bytes (ADR-0019 §8).",
		def:   strconv.Itoa(16 << 20),
		check: checkPositiveInt,
		set:   func(c *Config, v string) { c.MediaMaxImageBytes, _ = strconv.ParseInt(v, 10, 64) },
	},
	{
		key:   "media_max_image_side",
		doc:   "Largest accepted image width and height in pixels, read from the file's header before decoding (ADR-0019 §8).",
		def:   "8192",
		check: checkPositiveInt,
		set:   func(c *Config, v string) { c.MediaMaxImageSide, _ = strconv.Atoi(v) },
	},
	{
		key:   "media_max_image_pixels",
		doc:   "Largest accepted image area (width × height, and for an animated GIF the frames together), read from the file's header before decoding (ADR-0019 §8). The default is 33 MP (7680 × 4320).",
		def:   strconv.Itoa(7680 * 4320),
		check: checkPositiveInt,
		set:   func(c *Config, v string) { c.MediaMaxImagePixels, _ = strconv.ParseInt(v, 10, 64) },
	},
	{
		key:   "media_unattached_ttl",
		doc:   "How long an upload may stay unattached to any post or profile before the hourly cleanup deletes it (FR-803), as a Go duration such as `24h`.",
		def:   "24h",
		check: checkPositiveDuration,
		set:   func(c *Config, v string) { c.MediaUnattachedTTL, _ = time.ParseDuration(v) },
	},
	{
		key:   "media_max_av_bytes",
		doc:   "Largest accepted video or audio upload in bytes (ADR-0019 §8, ADR-0054 §6). The file is held in memory while it is checked and remuxed, so this also bounds the memory of one upload.",
		def:   strconv.Itoa(100 << 20),
		check: checkPositiveInt,
		set:   func(c *Config, v string) { c.MediaMaxAVBytes, _ = strconv.ParseInt(v, 10, 64) },
	},
	{
		key:   "media_max_video_duration",
		doc:   "Longest accepted video upload (ADR-0054 §6), as a Go duration such as `5m`.",
		def:   "5m",
		check: checkPositiveDuration,
		set:   func(c *Config, v string) { c.MediaMaxVideoDuration, _ = time.ParseDuration(v) },
	},
	{
		key:   "media_max_audio_duration",
		doc:   "Longest accepted audio upload (ADR-0054 §6), as a Go duration such as `10m`.",
		def:   "10m",
		check: checkPositiveDuration,
		set:   func(c *Config, v string) { c.MediaMaxAudioDuration, _ = time.ParseDuration(v) },
	},
	{
		key:   "custom_emoji_max_bytes",
		doc:   "Largest image in bytes that a local custom emoji may be registered with (ADR-0051 §2); the image must also be a PNG, a GIF, or a WebP of at most 1,024 pixels on a side. The default is Mastodon's limit. GoToSocial does not fetch remote emoji larger than 100 KiB by default.",
		def:   strconv.Itoa(256 << 10),
		check: checkPositiveInt,
		set:   func(c *Config, v string) { c.CustomEmojiMaxBytes, _ = strconv.ParseInt(v, 10, 64) },
	},
	{
		key:   "remote_custom_emoji_max_bytes",
		doc:   "Largest image in bytes of a remote custom emoji that the media proxy serves (ADR-0051 §2); the proxy answers 404 for a larger one, and for one that is not a PNG, a GIF, or a WebP.",
		def:   strconv.Itoa(1 << 20),
		check: checkPositiveInt,
		set:   func(c *Config, v string) { c.RemoteCustomEmojiMaxBytes, _ = strconv.ParseInt(v, 10, 64) },
	},
	{
		key:   "remote_media_cache_max_bytes",
		doc:   "Size limit in bytes of the copies of remote media that the media proxy keeps when the instance setting `remote_media_policy` is `avatars` or `all` (FR-409). The hourly cleanup deletes the expired copies, then the least recently accessed ones over this limit (FR-803).",
		def:   strconv.Itoa(1 << 30),
		check: checkPositiveInt,
		set:   func(c *Config, v string) { c.RemoteMediaCacheMaxBytes, _ = strconv.ParseInt(v, 10, 64) },
	},
	{
		key: "kv",
		doc: "Key-value store adapter for caches and rate limits (ADR-0003, ADR-0019 §5).",
		def: "db",
		choices: []choice{
			{value: "db", inV0: true, doc: "Table in the database."},
			{value: "memory", inV0: true, doc: "In-process memory; limits are per process."},
			{value: "valkey", inV0: true, doc: "Valkey or another server that speaks the Redis protocol (`valkey_url`); entries are shared by every process on the server. The build tag `no_valkey` leaves it out."},
		},
		set: func(c *Config, v string) { c.KV = v },
	},
	{
		key:          "valkey_url",
		doc:          "Valkey server of the adapters that use one (`kv = valkey`, `stream = valkey`):`valkey://[user:password@]host[:port][/db]` (`redis://` is the same), `valkeys://` or `rediss://` for TLS, or `unix:///path/to/socket`. The path selects the database number. A cluster is found by itself.",
		secret:       true,
		requiredWhen: "when `kv = valkey` or `stream = valkey`",
		check:        checkValkeyURL,
		set:          func(c *Config, v string) { c.ValkeyURL = v },
	},
	{
		key:   "valkey_tls_ca_file",
		doc:   "PEM file of the certificate authorities that the TLS certificate of `valkey_url` chains to, for a managed service with a private CA. Empty uses the system roots; it needs a `valkeys://` or `rediss://` URL.",
		check: checkNotBlank,
		set:   func(c *Config, v string) { c.ValkeyTLSCAFile = v },
	},
	{
		key:   "valkey_key_prefix",
		doc:   "Prefix of every key that this server writes to `valkey_url`, so that several instances or other applications can share the server.",
		def:   "my:",
		check: checkNotBlank,
		set:   func(c *Config, v string) { c.ValkeyKeyPrefix = v },
	},
	{
		key: "stream",
		doc: "Broker that carries the signals of the real-time updates (`GET /api/v1/streaming`) between processes (ADR-0050). It is never inferred from the other settings. With `hub`, a stream receives only the changes made in its own process, so run one process with `role = all`; a process with another role warns at start. The other brokers reach every process of the instance, and a process connects to them only while it has open streams (docs/architecture/deployment-targets.md).",
		def: "hub",
		choices: []choice{
			{value: "hub", inV0: true, doc: "In-process only; for one process with `role = all` (SQLite, or one PostgreSQL server process)."},
			{value: "postgres", inV0: true, doc: "PostgreSQL `LISTEN` / `NOTIFY` on the database of `database_url` (or `stream_postgres_url`). Requires `db = postgres`. The build tag `no_postgres` leaves it out."},
			{value: "valkey", inV0: true, doc: "Valkey Pub/Sub on `valkey_url`, with the channel `{valkey_key_prefix}stream`. The build tag `no_valkey` leaves it out."},
			{value: "sns", inV0: true, doc: "An Amazon SNS standard topic (`stream_sns_topic_arn`) that fans out to one SQS queue per listening process (`stream_sqs_queue_prefix`); credentials and settings from the AWS SDK's default chain. The build tag `no_sns` leaves it out."},
			{value: "pubsub", inV0: true, doc: "A Google Cloud Pub/Sub topic (`stream_pubsub_topic`) with one subscription per listening process (`stream_pubsub_subscription_prefix`); Application Default Credentials, or the emulator at `PUBSUB_EMULATOR_HOST`. The build tag `no_pubsub` leaves it out."},
		},
		set: func(c *Config, v string) { c.Stream = v },
	},
	{
		key:    "stream_postgres_url",
		defDoc: "value of `database_url`",
		doc:    "Connection string of the `LISTEN` connections of `stream = postgres`, in the format of `database_url`. Set it when `database_url` goes through a pooler in transaction mode (PgBouncer's transaction pooling, Neon's pooled connection string, Supabase's port 6543), where `LISTEN` does not work: it must reach the same database directly or through a pooler in session mode.",
		secret: true,
		set:    func(c *Config, v string) { c.StreamPostgresURL = v },
	},
	{
		key:          "stream_sns_topic_arn",
		doc:          "ARN of the Amazon SNS standard topic of `stream = sns`, which the deployment template creates. The queues are created in its account and region.",
		requiredWhen: "when `stream = sns`",
		check:        checkSNSTopicARN,
		set:          func(c *Config, v string) { c.StreamSNSTopicARN = v },
	},
	{
		key:          "stream_sqs_queue_prefix",
		doc:          "Start of the names of the per-process SQS queues of `stream = sns`, `{prefix}-{process id}-{n}`: at most 40 letters, digits, hyphens, and underscores. The `stream.sweep` job deletes the queues under it that ended processes left behind, so it belongs to one instance.",
		requiredWhen: "when `stream = sns`",
		check:        checkSQSQueuePrefix,
		set:          func(c *Config, v string) { c.StreamSQSQueuePrefix = v },
	},
	{
		key:          "stream_pubsub_topic",
		doc:          "Full name of the Google Cloud Pub/Sub topic of `stream = pubsub`, `projects/{project}/topics/{topic}`, which the deployment template creates. The subscriptions are created in its project.",
		requiredWhen: "when `stream = pubsub`",
		check:        checkPubSubTopic,
		set:          func(c *Config, v string) { c.StreamPubSubTopic = v },
	},
	{
		key:          "stream_pubsub_subscription_prefix",
		doc:          "Start of the IDs of the per-process subscriptions of `stream = pubsub`, `{prefix}-{process id}-{n}`: a letter, then letters, digits, and `-_.~+%`, not starting with `goog`. The `stream.sweep` job deletes the subscriptions under it that ended processes left behind, so it belongs to one instance.",
		requiredWhen: "when `stream = pubsub`",
		check:        checkPubSubSubscriptionPrefix,
		set:          func(c *Config, v string) { c.StreamPubSubSubscriptionPrefix = v },
	},
	{
		key: "key_manager",
		doc: "Key manager adapter that encrypts actor private keys (ADR-0019 §6).",
		def: "local",
		choices: []choice{
			{value: "local", inV0: true, doc: "Uses the key-encryption key from `kek`."},
			{value: "gcpkms", doc: "Google Cloud KMS."},
			{value: "awskms", doc: "AWS KMS."},
		},
		set: func(c *Config, v string) { c.KeyManager = v },
	},
	{
		key:          "kek",
		doc:          "Key-encryption key for actor private keys: 32 random bytes in standard base64 (for example `openssl rand -base64 32`). Required when `key_manager = local`. Back it up separately from the database; losing it loses every actor key (ADR-0019 §6, docs/operations/key-management.md).",
		secret:       true,
		requiredWhen: "when `key_manager = local`",
		check:        checkKEK,
		set: func(c *Config, v string) {
			c.KEK, _ = base64.StdEncoding.DecodeString(v) // validated by checkKEK
		},
	},
	{
		key:    "kek_previous",
		doc:    "Previous key-encryption keys, comma-separated, each in the format of `kek`. They only decrypt; new secrets are always encrypted with `kek`. Set the replaced key here while rotating `kek` (docs/operations/key-management.md).",
		secret: true,
		check:  checkPreviousKEKs,
		set: func(c *Config, v string) {
			for k := range strings.SplitSeq(v, ",") {
				b, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(k)) // validated by checkPreviousKEKs
				c.PreviousKEKs = append(c.PreviousKEKs, b)
			}
		},
	},
	{
		key: "mail",
		doc: "Mailer adapter for verification emails (FR-002). With `none`, no email reaches users, so an unverified address does not keep a user out and sign-ups in the `open` registration mode wait for a moderator's approval instead.",
		def: "none",
		choices: []choice{
			{value: "none", inV0: true, doc: "Logs each message, including its links, instead of sending it; for development and instances without email."},
			{value: "smtp", inV0: true, doc: "SMTP submission with mandatory STARTTLS (`smtp_host`, `smtp_port`, `smtp_username`, `smtp_password`)."},
			{value: "ses", doc: "Amazon SES."},
		},
		set: func(c *Config, v string) { c.Mail = v },
	},
	{
		key:          "mail_from",
		doc:          "Sender address of emails, as a bare address such as `noreply@social.example`.",
		requiredWhen: "when `mail` is not `none`",
		check:        checkBareAddress,
		set:          func(c *Config, v string) { c.MailFrom = v },
	},
	{
		key:          "smtp_host",
		doc:          "Host name of the SMTP server. The server must offer STARTTLS with a certificate valid for this name; the connection is refused otherwise.",
		requiredWhen: "when `mail = smtp`",
		check:        checkHostName,
		set:          func(c *Config, v string) { c.SMTPHost = v },
	},
	{
		key:   "smtp_port",
		doc:   "TCP port of the SMTP server (submission).",
		def:   "587",
		check: checkPort,
		set: func(c *Config, v string) {
			c.SMTPPort, _ = strconv.Atoi(v) // validated by checkPort
		},
	},
	{
		key: "smtp_username",
		doc: "User name for SMTP AUTH PLAIN, sent only after STARTTLS. Leave it empty for a server that needs no authentication.",
		set: func(c *Config, v string) { c.SMTPUsername = v },
	},
	{
		key:    "smtp_password",
		doc:    "Password for SMTP AUTH PLAIN.",
		secret: true,
		set:    func(c *Config, v string) { c.SMTPPassword = v },
	},
	{
		key: "captcha",
		doc: "CAPTCHA service that the sign-in and registration forms check (ADR-0042). The service receives the visitor's IP address, browser details, and cookies, and the two pages load its script; say so in the instance's privacy policy. Only `none` keeps every request on this server.",
		def: "none",
		choices: []choice{
			{value: "none", inV0: true, doc: "No CAPTCHA; nothing is sent to a third party."},
			{value: "turnstile", inV0: true, doc: "Cloudflare Turnstile."},
			{value: "hcaptcha", inV0: true, doc: "hCaptcha."},
			{value: "recaptcha", inV0: true, doc: "Google reCAPTCHA v2 with the checkbox (not v3 or Enterprise)."},
		},
		set: func(c *Config, v string) { c.Captcha = v },
	},
	{
		key:          "captcha_site_key",
		doc:          "Public site key of the `captcha` service, which the widget on the forms carries.",
		requiredWhen: "when `captcha` is not `none`",
		check:        checkNotBlank,
		set:          func(c *Config, v string) { c.CaptchaSiteKey = v },
	},
	{
		key:          "captcha_secret_key",
		doc:          "Secret key of the `captcha` service, which the server sends with each token to the service's siteverify API. The server does not try the key at start: a key that the service refuses is logged as an error on the first check, and until it is fixed registrations are refused and sign-ins skip the CAPTCHA (ADR-0042 §4).",
		secret:       true,
		requiredWhen: "when `captcha` is not `none`",
		check:        checkNotBlank,
		set:          func(c *Config, v string) { c.CaptchaSecretKey = v },
	},
	{
		key:    "vapid_private_key",
		doc:    "VAPID private key of the server's Web Push notifications (FR-206, RFC 8292): a P-256 private key, 32 bytes in base64url. Generate one with `my config vapid-key`. Without it, the server sends no Web Push and clients do not offer it. Changing it ends every subscription, since browsers subscribe with its public key (docs/operations/key-management.md).",
		secret: true,
		check: func(v string) error {
			_, err := webpush.ParsePrivateKey(v)
			return err
		},
		set: func(c *Config, v string) { c.VAPIDPrivateKey = v },
	},
	{
		key: "profile",
		doc: "Whether this is a production instance. Settings that weaken a security default for development, such as `http_allow_private_networks`, are refused in the `production` profile (ADR-0019).",
		def: "production",
		choices: []choice{
			{value: "production", inV0: true, doc: "A production instance."},
			{value: "development", inV0: true, doc: "A development or test instance."},
		},
		set: func(c *Config, v string) { c.Profile = v },
	},
	{
		key: "http_allow_private_networks",
		doc: "Lets outbound HTTP (federation, media, link previews) reach loopback, private, and other non-public addresses and host names, over plain `http` and on any port. For development and tests against local servers only: the server logs a warning at start, and the `production` profile refuses it (ADR-0019 §1).",
		def: "false",
		choices: []choice{
			{value: "false", inV0: true, doc: "Only public `https` destinations on ports 443 and 80."},
			{value: "true", inV0: true, doc: "Every destination; requires `profile = development`."},
		},
		set: func(c *Config, v string) { c.HTTPAllowPrivateNetworks = v == "true" },
	},
	{
		key:   "webhook_allowed_destinations",
		doc:   "Destinations on loopback or a private network that the instance's webhooks may reach (ADR-0060), such as an automation tool on the same host or in the same VPC: comma-separated host names, IP addresses, and CIDRs such as `n8n.internal, 10.0.0.0/8`. The SSRF guard of outbound HTTP refuses them otherwise; for these destinations alone, and for the webhooks alone, it also allows plain `http` and any port. A host name is allowed whatever it resolves to; an address range allows the URLs that name an address in it and the host names that resolve into it only when they are listed too. Only the operator can change it: the administrators of the web client cannot reach an internal network on their own. Empty allows none.",
		check: checkWebhookDestinations,
		set: func(c *Config, v string) {
			c.WebhookAllowedHosts, c.WebhookAllowedPrefixes = parseWebhookDestinations(v)
		},
	},
	{
		key:   "remote_status_retention",
		doc:   "How long a remote post is kept after its publication when nothing on this instance refers to it (a local reply, boost, favourite, bookmark, notification, or a thread with a local post); the hourly purge then deletes it (ADR-0034, FR-803). A Go duration such as `2160h` (90 days).",
		def:   "2160h",
		check: checkPositiveDuration,
		set:   func(c *Config, v string) { c.RemoteStatusRetention, _ = time.ParseDuration(v) },
	},
	{
		key:   "remote_status_max_retention",
		doc:   "The longest a remote post is kept after its publication, even when this instance refers to it; the hourly purge then deletes it, while local accounts' replies, boosts, favourites, and bookmarks stay and show it as deleted (ADR-0034). A Go duration such as `8760h` (365 days); empty keeps referred posts without limit.",
		check: checkPositiveDuration,
		set:   func(c *Config, v string) { c.RemoteStatusMaxRetention, _ = time.ParseDuration(v) },
	},
	{
		key:   "relay_status_retention",
		doc:   "How long a remote post that only a relay brought in is kept after its publication when nothing on this instance refers to it and no local account follows its author; the hourly purge then deletes it (ADR-0061). A Go duration such as `168h` (7 days).",
		def:   "168h",
		check: checkPositiveDuration,
		set:   func(c *Config, v string) { c.RelayStatusRetention, _ = time.ParseDuration(v) },
	},
	{
		key:   "relay_ingest_hourly_limit",
		doc:   "How many activities each relay may bring in per hour (ADR-0061). The ones over it are dropped before their posts are fetched, and counted in `my.relay.activities` and on the relays screen. It bounds the fetches, rows, and jobs that a relay costs.",
		def:   "1000",
		check: checkPositiveInt,
		set:   func(c *Config, v string) { c.RelayIngestHourlyLimit, _ = strconv.Atoi(v) },
	},
	{
		key:   "billing_sources",
		doc:   "Cloud providers whose usage charges are imported into the instance's costs every 24 hours (ADR-0068, FR-904), comma-separated: `aws` (AWS Cost Explorer, about $0.01 per billed request) and `gcp` (the Cloud Billing export to BigQuery, or the Pub/Sub notifications of a budget: `billing_gcp_method`). Each import records the charges of the current billing month and of the previous one (through the 15th, or until it is final) by service and charge type, as estimates until the provider closes the month, then final, then corrections; `GET /api/v1/admin/billing_imports` shows the state of the imports. A provider whose adapter is not in this build is refused at start. Credentials come from the SDK's default chain of each provider (docs/operations/billing-import.md). The build tags `no_billing_aws` and `no_billing_gcp` leave `aws` and `gcp` out. Empty imports nothing.",
		check: checkBillingSources,
		set: func(c *Config, v string) {
			for s := range strings.SplitSeq(v, ",") {
				c.BillingSources = append(c.BillingSources, strings.TrimSpace(s))
			}
		},
	},
	{
		key:   "billing_aws_max_requests_per_day",
		doc:   "How many billed AWS Cost Explorer requests (one per page, $0.01 each) the imports of `billing_sources = aws` may make per UTC day, counting the scheduled imports, their retries, and the imports that an administrator starts (ADR-0068 §4). An import with none left calls nothing and is recorded as `request_cap`; the next day's import tries again.",
		def:   strconv.Itoa(defaultBillingAWSRequestsPerDay),
		check: checkPositiveInt,
		set:   func(c *Config, v string) { c.BillingAWSMaxRequestsPerDay, _ = strconv.Atoi(v) },
	},
	{
		key:   "billing_aws_max_requests_per_month",
		doc:   "How many billed AWS Cost Explorer requests the imports may make per UTC calendar month, the bound of the cost of the imports (the default is $1.20 a month: four imports a day for thirty days). An import with none left calls nothing and is recorded as `request_cap` until the next month.",
		def:   strconv.Itoa(defaultBillingAWSRequestsPerMonth),
		check: checkPositiveInt,
		set:   func(c *Config, v string) { c.BillingAWSMaxRequestsPerMonth, _ = strconv.Atoi(v) },
	},
	{
		key: "billing_aws_metric",
		doc: "Cost Explorer metric that the imports of `billing_sources = aws` record (ADR-0068 §6).",
		def: "NetUnblendedCost",
		choices: []choice{
			{value: "NetUnblendedCost", inV0: true, doc: "The cost after the discounts (Savings Plans, Reserved Instances, private pricing), the closest to the invoice."},
			{value: "UnblendedCost", inV0: true, doc: "The cost before the discounts."},
		},
		set: func(c *Config, v string) { c.BillingAWSMetric = v },
	},
	{
		key: "billing_aws_filter",
		doc: "Which costs of the AWS account the imports of `billing_sources = aws` take (ADR-0068 §6). The most accurate is an AWS account that serves this instance alone, with `none`: the taxes and the support fees carry no tag and may fall out of a tag filter (docs/operations/billing-import.md).",
		def: "none",
		choices: []choice{
			{value: "none", inV0: true, doc: "Every cost that the credentials can see: the account's own, or, from the management account of an organization, every member account's."},
			{value: "linked_account", inV0: true, doc: "The costs of the member accounts of `billing_aws_linked_accounts` (the `LINKED_ACCOUNT` dimension)."},
			{value: "tag", inV0: true, doc: "The costs of the resources that carry the cost allocation tag `billing_aws_tag`, which must be activated in the Billing console first."},
		},
		set: func(c *Config, v string) { c.BillingAWSFilter = v },
	},
	{
		key:          "billing_aws_tag",
		doc:          "The activated cost allocation tag of `billing_aws_filter = tag`, `key=value` such as `app=my`. A tag shows in the billing data from its activation on, up to 24 hours after it is activated; older costs need a backfill in the Billing console (docs/operations/billing-import.md).",
		requiredWhen: "when `billing_aws_filter = tag`",
		check:        checkBillingAWSTag,
		set:          func(c *Config, v string) { c.BillingAWSTag = strings.TrimSpace(v) },
	},
	{
		key:          "billing_aws_linked_accounts",
		doc:          "The 12-digit IDs of the member accounts of `billing_aws_filter = linked_account`, comma-separated. Reading them from a member account needs Linked Account Access, which the management account turns on in the Cost Explorer preferences.",
		requiredWhen: "when `billing_aws_filter = linked_account`",
		check:        checkBillingAWSLinkedAccounts,
		set: func(c *Config, v string) {
			for s := range strings.SplitSeq(v, ",") {
				if s = strings.TrimSpace(s); s != "" {
					c.BillingAWSLinkedAccounts = append(c.BillingAWSLinkedAccounts, s)
				}
			}
		},
	},
	{
		key:   "billing_aws_role_arn",
		doc:   "ARN of an IAM role that the imports of `billing_sources = aws` assume with the Google ID token of the instance's service account (`AssumeRoleWithWebIdentity`, audience: this ARN), for a server on Google Cloud (Cloud Run) without AWS keys (ADR-0068 §8). The role trusts the identity provider `accounts.google.com` for the service account (docs/operations/billing-import.md). Empty uses the AWS SDK's default chain (the execution role, `AWS_ACCESS_KEY_ID`, the shared files).",
		check: checkBillingAWSRoleARN,
		set:   func(c *Config, v string) { c.BillingAWSRoleARN = v },
	},
	{
		key: "billing_gcp_method",
		doc: "How the imports of `billing_sources = gcp` read the Google Cloud usage charges (ADR-0068 §7). The billing months are the invoice months of Cloud Billing, US Pacific calendar months; a month is final from the 5th of the next month on (Pacific time).",
		def: "bigquery",
		choices: []choice{
			{value: "bigquery", inV0: true, doc: "Sum the standard usage cost export of Cloud Billing to BigQuery (`billing_gcp_table`) with one query a day, within the free tier of BigQuery (docs/operations/billing-import.md)."},
			{value: "budget", inV0: true, doc: "Read the notifications of a monthly Cloud Billing budget from a Pub/Sub pull subscription (`billing_gcp_budget_subscription`) once a day: the budget's estimated total of each month, one line named after the budget, without the services, the charge types, or the export's days (docs/operations/billing-import.md). A simple way for an operator without the BigQuery export; free within the free tier of Pub/Sub."},
		},
		set: func(c *Config, v string) { c.BillingGCPMethod = v },
	},
	{
		key:          "billing_gcp_table",
		doc:          "The table of the standard usage cost export of Cloud Billing to BigQuery, `project.dataset.table` such as `my-project.billing_export.gcp_billing_export_v1_012345_6789AB_CDEF01`. The service account of the server needs `roles/bigquery.dataViewer` on its dataset (docs/operations/billing-import.md).",
		requiredWhen: "when `billing_sources` includes `gcp` and `billing_gcp_method = bigquery`",
		check:        checkBillingGCPTable,
		set:          func(c *Config, v string) { c.BillingGCPTable = strings.TrimSpace(v) },
	},
	{
		key:   "billing_gcp_query_project",
		doc:   "The Google Cloud project that runs the BigQuery queries of `billing_gcp_method = bigquery` and is billed for their bytes; the service account of the server needs `roles/bigquery.jobUser` on it. Empty uses the project of `billing_gcp_table`.",
		check: checkBillingGCPProject,
		set:   func(c *Config, v string) { c.BillingGCPQueryProject = strings.TrimSpace(v) },
	},
	{
		key:   "billing_gcp_projects",
		doc:   "Google Cloud project IDs, comma-separated, whose costs the imports of `billing_gcp_method = bigquery` take (`project.id` of the export). Empty takes every cost of the billing account. The taxes and the other charges of the invoice that carry no project may fall out of this filter (docs/operations/billing-import.md).",
		check: checkBillingGCPProjects,
		set: func(c *Config, v string) {
			for s := range strings.SplitSeq(v, ",") {
				if s = strings.TrimSpace(s); s != "" {
					c.BillingGCPProjects = append(c.BillingGCPProjects, s)
				}
			}
		},
	},
	{
		key:   "billing_gcp_label",
		doc:   "A label `key=value`, such as `app=my`, whose resources' costs the imports of `billing_gcp_method = bigquery` take (`labels` of the export). Empty takes the costs with or without labels. The taxes and the costs of resources without the label fall out of this filter.",
		check: checkBillingGCPLabel,
		set:   func(c *Config, v string) { c.BillingGCPLabel = strings.TrimSpace(v) },
	},
	{
		key:   "billing_gcp_max_bytes_billed",
		doc:   "The `maximumBytesBilled` of each BigQuery query of `billing_gcp_method = bigquery`, in bytes: a query that would bill more fails as `bytes_cap` and bills nothing (ADR-0068 §4). The default, 1 GiB, keeps a query a day well within the free tier of 1 TiB a month, which the billing account shares with its other uses; BigQuery bills at least 10 MB a query.",
		def:   strconv.FormatInt(defaultBillingGCPMaxBytesBilled, 10),
		check: checkPositiveInt64,
		set:   func(c *Config, v string) { c.BillingGCPMaxBytesBilled, _ = strconv.ParseInt(v, 10, 64) },
	},
	{
		key:          "billing_gcp_budget_subscription",
		doc:          "The pull subscription of the Pub/Sub topic of a monthly Cloud Billing budget that `billing_gcp_method = budget` reads, `projects/{project}/subscriptions/{subscription}`. The imports acknowledge a notification once a later one of its budget and month replaces it or once its month is final and no longer imported, and keep the latest of each month imported, so the subscription needs a message retention of at least 14 days (31 recommended) and must not be read by anything else. The service account of the server needs `roles/pubsub.subscriber` on it (docs/operations/billing-import.md).",
		requiredWhen: "when `billing_sources` includes `gcp` and `billing_gcp_method = budget`",
		check:        checkBillingGCPSubscription,
		set:          func(c *Config, v string) { c.BillingGCPBudgetSubscription = strings.TrimSpace(v) },
	},
	{
		key: "payments_provider",
		doc: "The payment provider of the supporters (ADR-0072, ADR-0074, FR-910): local users subscribe to paid plans on its hosted checkout, and the roles of the plans follow the state of their payments. Only one provider is active on a server; switching to another one needs the subscribers to subscribe again. The provider calls `POST /payments/webhooks/{provider}` (signed, at most 1 MiB, recorded once per event), and a reconciliation reads its lists every `payments_reconcile_interval`. Card details never reach the server. An administrator still enables the supporters and sets up the plans on the admin screen.",
		def: "none",
		choices: []choice{
			{value: "none", inV0: true, doc: "No payments: the webhook endpoint and the reconciliation do not run."},
			{value: "stripe", inV0: true, doc: "Stripe Billing: Checkout in subscription mode and the customer portal, with `payments_stripe_secret_key` and `payments_stripe_webhook_secret` (docs/operations/payments-stripe.md). The build tag `no_payments_stripe` leaves it out."},
		},
		set: func(c *Config, v string) { c.PaymentsProvider = v },
	},
	{
		key:          "payments_stripe_secret_key",
		doc:          "The API key of the Stripe account of `payments_provider = stripe` (ADR-0074 §5): a restricted key (`rk_test_` / `rk_live_`) with the permissions of docs/operations/payments-stripe.md, or a secret key (`sk_test_` / `sk_live_`). Its prefix sets the mode: a test key charges nothing, and the webhook endpoint refuses live events with it (and test events with a live key).",
		secret:       true,
		requiredWhen: "when `payments_provider = stripe`",
		check:        checkStripeSecretKey,
		set:          func(c *Config, v string) { c.PaymentsStripeSecretKey = v },
	},
	{
		key:          "payments_stripe_webhook_secret",
		doc:          "The signing secret (`whsec_`) of the webhook endpoint `https://<domain>/payments/webhooks/stripe` that the operator registers on Stripe's dashboard, or of `stripe listen` for local tests (ADR-0074 §3). While a secret is rolled, Stripe signs with the old and the new one, so one is enough.",
		secret:       true,
		requiredWhen: "when `payments_provider = stripe`",
		check:        checkStripeWebhookSecret,
		set:          func(c *Config, v string) { c.PaymentsStripeWebhookSecret = v },
	},
	{
		key:   "payments_reconcile_interval",
		doc:   "How often the reconciliation with `payments_provider` runs (ADR-0074 §4): it reads the provider's changes since the previous run, the checkouts in progress, the money movements, and a sweep of every subscription, and corrects what the webhooks missed. A Go duration from `15m` to `168h`; raise it, or `payments_reconcile_max_requests`, when the runs often stop at the cap.",
		def:   "6h",
		check: checkReconcileInterval,
		set:   func(c *Config, v string) { c.PaymentsReconcileInterval, _ = time.ParseDuration(v) },
	},
	{
		key:   "payments_reconcile_max_requests",
		doc:   "How many requests one reconciliation sends to `payments_provider` (ADR-0074 §4), a page of a list counting as one. A run that reaches it records where it stopped and a continuation picks up the rest a minute later. The provider's adapter also keeps the requests within its rate limits.",
		def:   strconv.Itoa(supporters.DefaultReconcileMaxRequests),
		check: checkPositiveInt,
		set:   func(c *Config, v string) { c.PaymentsReconcileMaxRequests, _ = strconv.Atoi(v) },
	},
	{
		key:   "dead_job_retention",
		doc:   "How long a dead job (a job that failed for good or used up its attempts, ADR-0004) is kept after it failed, for an operator to inspect and requeue it; the hourly purge then deletes it (FR-803). A Go duration such as `720h` (30 days).",
		def:   "720h",
		check: checkPositiveDuration,
		set:   func(c *Config, v string) { c.DeadJobRetention, _ = time.ParseDuration(v) },
	},
	{
		key:   "job_attempt_retention",
		doc:   "How long the record of a job's run that did not succeed (a retry, or the run that made it a dead job; ADR-0064) is kept after the run, for the queue dashboard of the admin screen; the hourly purge then deletes it. Successful runs are not recorded. A Go duration such as `720h` (30 days).",
		def:   "720h",
		check: checkPositiveDuration,
		set:   func(c *Config, v string) { c.JobAttemptRetention, _ = time.ParseDuration(v) },
	},
	{
		key:   "dynamic_client_unredeemed_ttl",
		doc:   "How long a dynamically registered OAuth client (RFC 7591, `POST /oauth/register`; ADR-0090 §1) may wait for the exchange of its first authorization code: the hourly clean-up deletes a client that is older than this and had no code exchanged. A Go duration such as `24h`.",
		def:   "24h",
		check: checkPositiveDuration,
		set:   func(c *Config, v string) { c.DynamicClientUnredeemedTTL, _ = time.ParseDuration(v) },
	},
	{
		key:   "dynamic_client_idle_ttl",
		doc:   "How long a dynamically registered OAuth client without a usable token may go unused before the hourly clean-up deletes it (ADR-0090 §1). Until the last use of a token is recorded, the time the client's newest token was issued stands for it. A Go duration such as `2160h` (90 days).",
		def:   "2160h",
		check: checkPositiveDuration,
		set:   func(c *Config, v string) { c.DynamicClientIdleTTL, _ = time.ParseDuration(v) },
	},
	{
		key:   "oauth_client_rate_limits",
		doc:   "Caps on the client API requests of OAuth clients (ADR-0090 §7), for an administrator to hold back a client that overloads the server. A comma-separated list of `client_id=requests`: the requests that all the users of the client together may make in 5 minutes, on top of the per-account limit, such as `https://app.example/client.json=3000`. A client that is not listed is not limited. Over its cap, a request is answered `429` with `Retry-After`.",
		check: checkClientRateLimits,
		set: func(c *Config, v string) {
			c.OAuthClientRateLimits, _ = parseClientRateLimits(v)
		},
	},
	{
		key: "metrics_exporter",
		doc: "Where the service metrics go (ADR-0055 §2): the request, federation, delivery, job, and stream metrics, named after the OpenTelemetry semantic conventions or with the prefix `my.`. Labels never carry a remote domain, an account, a URL, or a client IP.",
		def: "none",
		choices: []choice{
			{value: "none", inV0: true, doc: "No metrics; measuring costs next to nothing. On Cloud Run the platform's metrics and log-based metrics stand in for them."},
			{value: "prometheus", inV0: true, doc: "The Prometheus text format at `GET /metrics` on `metrics_listen`, a listener of its own, for a Prometheus-compatible server (and KEDA) to scrape; for self-hosted instances. The build tag `no_otel` leaves it out."},
			{value: "otlp", inV0: true, doc: "OTLP over HTTP (protobuf) to an OpenTelemetry Collector, configured by the standard environment variables `OTEL_EXPORTER_OTLP_ENDPOINT` (default `http://localhost:4318`), `OTEL_EXPORTER_OTLP_HEADERS`, and `OTEL_METRIC_EXPORT_INTERVAL` (default 60 seconds). The build tag `no_otel` leaves it out."},
			{value: "emf", inV0: true, doc: "CloudWatch Embedded Metric Format lines, which CloudWatch Logs turns into metrics; for AWS Lambda. The log line that ends each request and job carries its duration metrics, and the other measurements are written on standard output, one line per label set, at the end of each request and job. Requests in progress and open streams are not written."},
		},
		set: func(c *Config, v string) { c.MetricsExporter = v },
	},
	{
		key:   "metrics_listen",
		doc:   "TCP address, as `host:port`, of the listener that serves `GET /metrics` when `metrics_exporter = prometheus`, apart from `listen`. It has no authentication: keep it on loopback or a private network, or protect it with the network or a proxy.",
		def:   "127.0.0.1:9464",
		check: checkListen,
		set:   func(c *Config, v string) { c.MetricsListen = v },
	},
	{
		key:   "metrics_emf_namespace",
		doc:   "CloudWatch namespace of the metrics of `metrics_exporter = emf`.",
		def:   "my",
		check: checkNotBlank,
		set:   func(c *Config, v string) { c.MetricsEMFNamespace = v },
	},
	{
		key:   "metrics_emf_dimensions",
		doc:   "Labels that become CloudWatch dimensions with `metrics_exporter = emf`, comma-separated, such as `my.surface, my.role`; the other labels stay fields of the log line. CloudWatch bills every combination of dimension values as a metric, so the deployment template chooses them. Empty makes every metric one series per namespace.",
		check: checkLabelKeys,
		set: func(c *Config, v string) {
			for k := range strings.SplitSeq(v, ",") {
				c.MetricsEMFDimensions = append(c.MetricsEMFDimensions, strings.TrimSpace(k))
			}
		},
	},
	{
		key: "traces_exporter",
		doc: "Where the distributed traces go (ADR-0063): the spans of the HTTP requests, the job runs (each the root of a new trace linked to the request that enqueued it), the transactions and queries, the deliveries, and the outbound requests. Span attributes never carry an account, a URL or a path, a client IP, a User-Agent, an SQL text, or an error message; the outbound requests carry the destination's host name. No outbound request carries a `traceparent`, and an incoming `traceparent` is taken only from `trusted_proxies`.",
		def: "none",
		choices: []choice{
			{value: "none", inV0: true, doc: "No spans; tracing costs next to nothing. The log lines still carry `trace_id` and `span_id` of a `traceparent` from `trusted_proxies`, which correlates them with the platform's traces (Cloud Run)."},
			{value: "otlp", inV0: true, doc: "OTLP over HTTP (protobuf, gzip) to an OpenTelemetry Collector or another receiver, configured by the standard environment variables `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` or `OTEL_EXPORTER_OTLP_ENDPOINT` (default `http://localhost:4318`) and `OTEL_EXPORTER_OTLP_HEADERS`. The server does not authenticate to Cloud Trace or X-Ray itself: send to a collector next to it, which does (docs/operations/running.md). The build tag `no_otel` leaves it out."},
		},
		set: func(c *Config, v string) { c.TracesExporter = v },
	},
	{
		key:   "traces_sample_ratio",
		doc:   "Share of the traces recorded with `traces_exporter = otlp`, from `0` to `1`, among the requests and jobs without a sampled parent: a request follows the sampling of a `traceparent` from `trusted_proxies`, and a job is recorded when the request that enqueued it was. The default records about 1,000 traces a day of an instance that serves 100,000 requests a day.",
		def:   "0.01",
		check: checkRatio,
		set:   func(c *Config, v string) { c.TracesSampleRatio, _ = strconv.ParseFloat(v, 64) },
	},
	{
		key: "traces_flush",
		doc: "When the spans of `traces_exporter = otlp` are sent.",
		def: "batch",
		choices: []choice{
			{value: "batch", inV0: true, doc: "In batches, every few seconds, in the background; for processes that keep running (self-hosted)."},
			{value: "unit", inV0: true, doc: "Also at the end of every recorded HTTP request and job, before it returns, to a collector on the same host; for the platforms that throttle or freeze an idle instance (Cloud Run with request-based billing, AWS Lambda). A request that is not recorded sends nothing."},
		},
		set: func(c *Config, v string) { c.TracesFlush = v },
	},
}

// crossChecks rejects missing conditional settings and contradictory combinations.
// vals holds every setting that passed its own validation; present holds every
// setting that was given a non-empty value, valid or not.
func crossChecks(vals map[string]string, present map[string]bool) []Issue {
	var issues []Issue
	if vals["key_manager"] == "local" {
		if !present["kek"] {
			issues = append(issues, Issue{Key: "kek", Missing: true, Message: "required when key_manager is local"})
		}
	}
	if prev, ok := vals["kek_previous"]; ok {
		seen := map[string]bool{}
		if kek, ok := vals["kek"]; ok {
			seen[kek] = true
		}
		for k := range strings.SplitSeq(prev, ",") {
			k = strings.TrimSpace(k)
			if seen[k] {
				issues = append(issues, Issue{Key: "kek_previous", Message: "must not repeat a key or contain the current kek"})
				break
			}
			seen[k] = true
		}
	}
	if vals["kv"] == "valkey" && !present["valkey_url"] {
		issues = append(issues, Issue{Key: "valkey_url", Missing: true, Message: "required when kv is valkey"})
	} else if vals["stream"] == "valkey" && !present["valkey_url"] {
		issues = append(issues, Issue{Key: "valkey_url", Missing: true, Message: "required when stream is valkey"})
	}
	if db, ok := vals["db"]; ok && vals["stream"] == "postgres" && db != "postgres" {
		issues = append(issues, Issue{Key: "stream", Message: "postgres requires db = postgres, not " + db})
	}
	if vals["stream"] == "sns" {
		for _, k := range []string{"stream_sns_topic_arn", "stream_sqs_queue_prefix"} {
			if !present[k] {
				issues = append(issues, Issue{Key: k, Missing: true, Message: "required when stream is sns"})
			}
		}
	}
	if vals["stream"] == "pubsub" {
		for _, k := range []string{"stream_pubsub_topic", "stream_pubsub_subscription_prefix"} {
			if !present[k] {
				issues = append(issues, Issue{Key: k, Missing: true, Message: "required when stream is pubsub"})
			}
		}
	}
	if vals["valkey_tls_ca_file"] != "" && vals["valkey_url"] != "" && !strings.HasPrefix(vals["valkey_url"], "valkeys://") && !strings.HasPrefix(vals["valkey_url"], "rediss://") {
		issues = append(issues, Issue{Key: "valkey_tls_ca_file", Message: "needs a valkeys:// or rediss:// valkey_url"})
	}
	if mail, ok := vals["mail"]; ok && mail != "none" && !present["mail_from"] {
		issues = append(issues, Issue{Key: "mail_from", Missing: true, Message: "required when mail is not none"})
	}
	if vals["mail"] == "smtp" && !present["smtp_host"] {
		issues = append(issues, Issue{Key: "smtp_host", Missing: true, Message: "required when mail is smtp"})
	}
	if captcha, ok := vals["captcha"]; ok && captcha != "none" {
		for _, k := range []string{"captcha_site_key", "captcha_secret_key"} {
			if !present[k] {
				issues = append(issues, Issue{Key: k, Missing: true, Message: "required when captcha is not none"})
			}
		}
	}
	if vals["payments_provider"] == "stripe" {
		for _, k := range []string{"payments_stripe_secret_key", "payments_stripe_webhook_secret"} {
			if !present[k] {
				issues = append(issues, Issue{Key: k, Missing: true, Message: "required when payments_provider is stripe"})
			}
		}
	}
	if vals["billing_aws_filter"] == "tag" && !present["billing_aws_tag"] {
		issues = append(issues, Issue{Key: "billing_aws_tag", Missing: true, Message: "required when billing_aws_filter is tag"})
	}
	if vals["billing_aws_filter"] == "linked_account" && !present["billing_aws_linked_accounts"] {
		issues = append(issues, Issue{Key: "billing_aws_linked_accounts", Missing: true, Message: "required when billing_aws_filter is linked_account"})
	}
	if slices.Contains(splitList(vals["billing_sources"]), "gcp") && cmp.Or(vals["billing_gcp_method"], "bigquery") == "bigquery" && !present["billing_gcp_table"] {
		issues = append(issues, Issue{Key: "billing_gcp_table", Missing: true, Message: "required when billing_sources includes gcp and billing_gcp_method is bigquery"})
	}
	if slices.Contains(splitList(vals["billing_sources"]), "gcp") && vals["billing_gcp_method"] == "budget" && !present["billing_gcp_budget_subscription"] {
		issues = append(issues, Issue{Key: "billing_gcp_budget_subscription", Missing: true, Message: "required when billing_sources includes gcp and billing_gcp_method is budget"})
	}
	if vals["http_allow_private_networks"] == "true" && vals["profile"] != "development" {
		issues = append(issues, Issue{Key: "http_allow_private_networks", Message: "true is refused in the production profile; set profile = development"})
	}
	if vals["metrics_exporter"] == "prometheus" && vals["metrics_listen"] != "" && vals["metrics_listen"] == vals["listen"] {
		issues = append(issues, Issue{Key: "metrics_listen", Message: "must differ from listen: the metrics are never served on the public listener"})
	}
	if vals["queue"] == "inline" && vals["role"] != "" && vals["role"] != string(RoleAll) {
		issues = append(issues, Issue{Key: "queue", Message: fmt.Sprintf("inline runs jobs in the enqueuing process and requires role = all, not %s", vals["role"])})
	}
	return issues
}

func checkNotBlank(v string) error {
	if strings.TrimSpace(v) == "" {
		return errors.New("must not be blank")
	}
	return nil
}

func checkRatio(v string) error {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || f < 0 || f > 1 {
		return errors.New("must be a number from 0 to 1, for example 0.01")
	}
	return nil
}

// billingSourceNames are the values of billing_sources (ADR-0068).
var billingSourceNames = []string{"aws", "gcp"}

// The defaults of the AWS request caps (ADR-0068 §4); finance.DefaultAWSRequestsPerDay and
// _PerMonth are the same.
const (
	defaultBillingAWSRequestsPerDay   = 4
	defaultBillingAWSRequestsPerMonth = 120
)

// checkBillingSources accepts comma-separated names of billingSourceNames, each once.
func checkBillingSources(v string) error {
	seen := map[string]bool{}
	for s := range strings.SplitSeq(v, ",") {
		s = strings.TrimSpace(s)
		if !slices.Contains(billingSourceNames, s) {
			return fmt.Errorf("%q is not a billing source; use aws or gcp, comma-separated", s)
		}
		if tag, ok := excludedValues["billing_sources"][s]; ok {
			return excludedError(s, tag)
		}
		if seen[s] {
			return fmt.Errorf("%q is listed twice", s)
		}
		seen[s] = true
	}
	return nil
}

// defaultBillingGCPMaxBytesBilled is the default maximumBytesBilled of a BigQuery query, 1 GiB
// (ADR-0068 §4).
const defaultBillingGCPMaxBytesBilled int64 = 1 << 30

// splitList splits a comma-separated value, dropping the blanks.
func splitList(v string) []string {
	var out []string
	for s := range strings.SplitSeq(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func checkPositiveInt64(v string) error {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("must be a whole number from 1 to %d", int64(math.MaxInt64))
	}
	return nil
}

func checkPositiveInt(v string) error {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 || n > math.MaxInt32 {
		return fmt.Errorf("must be a whole number from 1 to %d", math.MaxInt32)
	}
	return nil
}

// checkStripeSecretKey accepts the prefixes of Stripe's secret and restricted keys, which set
// the mode (ADR-0074 §5); publishable keys (pk_) are refused.
func checkStripeSecretKey(v string) error {
	for _, p := range []string{"sk_test_", "rk_test_", "sk_live_", "rk_live_"} {
		if strings.HasPrefix(v, p) && len(v) > len(p) {
			return nil
		}
	}
	return errors.New("must be a Stripe secret key (sk_test_, sk_live_) or restricted key (rk_test_, rk_live_)")
}

func checkStripeWebhookSecret(v string) error {
	if !strings.HasPrefix(v, "whsec_") || len(v) == len("whsec_") {
		return errors.New("must be a Stripe webhook signing secret (whsec_)")
	}
	return nil
}

func checkReconcileInterval(v string) error {
	d, err := time.ParseDuration(v)
	if err != nil || d < supporters.MinReconcileInterval || d > supporters.MaxReconcileInterval {
		return fmt.Errorf("must be a Go duration from %s to %s", supporters.MinReconcileInterval, supporters.MaxReconcileInterval)
	}
	return nil
}

func checkPositiveDuration(v string) error {
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return errors.New("must be a positive Go duration, for example 24h or 90m")
	}
	return nil
}

// checkDomain accepts a lower-case DNS name with an optional port.
func checkDomain(v string) error {
	errSyntax := errors.New("must be a domain name with an optional :port, without scheme or path")
	if strings.ContainsAny(v, "/?#@") {
		return errSyntax
	}
	name := v
	if strings.Contains(v, ":") {
		h, p, err := net.SplitHostPort(v)
		if err != nil {
			return errSyntax
		}
		if err := checkPort(p); err != nil {
			return err
		}
		name = h
	}
	if name != strings.ToLower(name) {
		return errors.New("must be lower case")
	}
	if net.ParseIP(name) != nil {
		return errors.New("must be a domain name, not an IP address")
	}
	if len(name) > 253 {
		return errors.New("must be at most 253 characters")
	}
	for label := range strings.SplitSeq(name, ".") {
		if !validLabel(label) {
			return errSyntax
		}
	}
	return nil
}

func validLabel(l string) bool {
	if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	for _, r := range l {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

// checkSourceURL accepts an absolute https URL or a mailto: URI with an address, the two ways of offering the source
// that AGPL-3.0 section 13 leaves to the operator.
func checkSourceURL(v string) error {
	const msg = "must be an absolute https URL without user info or fragment, or a mailto: URI with an email address"
	u, err := url.Parse(v)
	if err != nil || u.Fragment != "" {
		return errors.New(msg)
	}
	switch u.Scheme {
	case "https":
		if u.Host == "" || u.User != nil {
			return errors.New(msg)
		}
	case "mailto":
		// mailto:alice@example.org[?subject=...]: the address is the opaque part, without "//".
		to, err := url.PathUnescape(u.Opaque)
		if err != nil || u.Host != "" || !strings.Contains(to, "@") {
			return errors.New(msg)
		}
	default:
		return errors.New(msg)
	}
	return nil
}

func checkOrigin(v string) error {
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" || v != u.Scheme+"://"+u.Host {
		return errors.New("must be an origin: http or https, a host, and an optional port, without path or trailing slash")
	}
	if u.Host != strings.ToLower(u.Host) {
		return errors.New("must be lower case")
	}
	return nil
}

func checkListen(v string) error {
	_, p, err := net.SplitHostPort(v)
	if err != nil {
		return errors.New("must be host:port, for example :8080 or 127.0.0.1:8080")
	}
	return checkPort(p)
}

// checkLabelKeys accepts comma-separated metric label keys such as my.surface.
func checkClientRateLimits(v string) error {
	_, err := parseClientRateLimits(v)
	return err
}

// parseClientRateLimits parses `client_id=requests` pairs. The client_id of a metadata document
// is a URL that may hold "=", so each pair splits at its last "=".
func parseClientRateLimits(v string) (map[string]int64, error) {
	limits := map[string]int64{}
	for pair := range strings.SplitSeq(v, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		i := strings.LastIndex(pair, "=")
		if i <= 0 {
			return nil, errors.New("must be comma-separated client_id=requests pairs, such as app-id=3000")
		}
		n, err := strconv.ParseInt(strings.TrimSpace(pair[i+1:]), 10, 64)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("the cap of %q must be a positive integer", pair[:i])
		}
		limits[strings.TrimSpace(pair[:i])] = n
	}
	return limits, nil
}

func checkLabelKeys(v string) error {
	for k := range strings.SplitSeq(v, ",") {
		k = strings.TrimSpace(k)
		if k == "" || strings.Trim(k, "abcdefghijklmnopqrstuvwxyz0123456789._") != "" {
			return errors.New("must be comma-separated label keys of lower-case letters, digits, dots, and underscores, such as my.surface")
		}
	}
	return nil
}

func checkPort(p string) error {
	n, err := strconv.Atoi(p)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("port %q must be a number from 1 to 65535", p)
	}
	return nil
}

func checkKEK(v string) error {
	b, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return errors.New("must be standard base64")
	}
	if len(b) != kekSize {
		return fmt.Errorf("must decode to %d bytes, got %d", kekSize, len(b))
	}
	return nil
}

// checkBareAddress accepts an email address without a display name or angle brackets.
func checkBareAddress(v string) error {
	a, err := mail.ParseAddress(v)
	if err != nil || a.Name != "" || a.Address != v {
		return errors.New("must be a bare email address such as noreply@example.com")
	}
	return nil
}

// checkHostName accepts a lower-case DNS name or an IP address, without a port.
func checkHostName(v string) error {
	if net.ParseIP(v) != nil {
		return nil
	}
	if v != strings.ToLower(v) {
		return errors.New("must be lower case")
	}
	if len(v) > 253 {
		return errors.New("must be at most 253 characters")
	}
	for label := range strings.SplitSeq(v, ".") {
		if !validLabel(label) {
			return errors.New("must be a host name or an IP address, without scheme or port")
		}
	}
	return nil
}

func checkPreviousKEKs(v string) error {
	for i, k := range strings.Split(v, ",") {
		if err := checkKEK(strings.TrimSpace(k)); err != nil {
			return fmt.Errorf("key #%d %w", i+1, err)
		}
	}
	return nil
}

// parseProxy parses one entry of trusted_proxies: a CIDR, or an address as its single-address
// prefix. IPv4-mapped IPv6 entries are unmapped, like the addresses they are compared with.
func parseProxy(v string) (netip.Prefix, error) {
	if strings.Contains(v, "/") {
		p, err := netip.ParsePrefix(v)
		if err != nil {
			return netip.Prefix{}, err
		}
		if a := p.Addr(); a.Is4In6() {
			bits := p.Bits() - 96
			if bits < 0 {
				return netip.Prefix{}, errors.New("an IPv4-mapped prefix must be at least /96")
			}
			p = netip.PrefixFrom(a.Unmap(), bits)
		}
		return p.Masked(), nil
	}
	a, err := netip.ParseAddr(v)
	if err != nil {
		return netip.Prefix{}, err
	}
	if a.Zone() != "" {
		return netip.Prefix{}, errors.New("must not have a zone")
	}
	a = a.Unmap()
	return netip.PrefixFrom(a, a.BitLen()), nil
}

func checkTrustedProxies(v string) error {
	for i, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if _, err := parseProxy(p); err != nil {
			return fmt.Errorf("entry #%d %q must be a CIDR such as 10.0.0.0/8 or an IP address", i+1, p)
		}
	}
	return nil
}

func parseTrustedProxies(v string) []netip.Prefix {
	var ps []netip.Prefix
	for p := range strings.SplitSeq(v, ",") {
		pfx, _ := parseProxy(strings.TrimSpace(p)) // validated by checkTrustedProxies
		ps = append(ps, pfx)
	}
	return ps
}

// checkWebhookDestinations accepts comma-separated host names, IP addresses, and CIDRs.
func checkWebhookDestinations(v string) error {
	for i, d := range strings.Split(v, ",") {
		d = strings.TrimSpace(d)
		if _, err := parseProxy(d); err == nil {
			continue
		}
		if d == "" || checkHostName(d) != nil {
			return fmt.Errorf("entry #%d %q must be a lower-case host name, an IP address, or a CIDR such as 10.0.0.0/8", i+1, d)
		}
	}
	return nil
}

// parseWebhookDestinations splits a value that checkWebhookDestinations accepted into host names
// and address ranges.
func parseWebhookDestinations(v string) (hosts []string, prefixes []netip.Prefix) {
	for d := range strings.SplitSeq(v, ",") {
		d = strings.TrimSpace(d)
		if p, err := parseProxy(d); err == nil {
			prefixes = append(prefixes, p)
			continue
		}
		hosts = append(hosts, strings.TrimSuffix(d, "."))
	}
	return hosts, prefixes
}
