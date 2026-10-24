<!-- Code generated from internal/platform/config/schema.go by "scripts/task.sh go:config-docs". DO NOT EDIT. -->

# Configuration reference

The server reads each setting from these sources, highest precedence first (ADR-0010):

1. The environment variable `MY_<KEY>` (the key in upper case).
2. The TOML file named by `MY_CONFIG` (or `my config check -config <path>`), with the keys below at the top level.
3. The default.

Secrets are read only from the environment: either the variable itself or `<NAME>_FILE`, the path of a file that holds the value
(trailing newlines are removed), for Docker and Kubernetes secrets (ADR-0019 §6). Setting a secret in the TOML file is an error.

Adapters are always selected by an explicit key; nothing is inferred from a URL scheme.
Values marked "not in v0" are decided in an ADR but not shipped in v0, and are rejected.
A binary built with a build tag that leaves an adapter out rejects the values that select it ([build tags](build-tags.md)).

`my config check` validates the configuration and lists every problem at once; the server refuses to start on any of them.
The problems are described in the language of the process locale (`LANGUAGE`, `LC_ALL`, `LC_MESSAGES`, `LANG`; English for `C` or an unsupported language),
followed by the technical detail in English.

## Configuration and instance settings

The settings on this page configure the deployment: what a process runs, which adapters it uses, where they are, and its secrets.
These values are fixed for the life of a process and change only with a restart.
The policies of the instance that administrators change while it runs (registration mode, maximum post length,
default locale, which timelines are enabled) and its localized texts (name, descriptions, terms, privacy policy)
are instance settings instead: they are stored in the database with every change kept as a revision (data-model §4.9),
and are changed with `my instance set` ([instance settings](../operations/instance-settings.md)).

The two never share a key, so neither overrides the other: no environment variable or config file key sets an
instance setting, and the database never holds a setting from this page. A test keeps the two key sets disjoint.
An instance setting that has never been set reads as its default, which is defined in code.

## Summary

| Key | Environment variable | Default | Required | Secret |
| --- | --- | --- | --- | --- |
| [`host`](#host) | `MY_HOST` | none | yes | no |
| [`account_domain`](#account_domain) | `MY_ACCOUNT_DOMAIN` | value of `host` | no | no |
| [`source_url`](#source_url) | `MY_SOURCE_URL` | `https://github.com/mycroblog/my` | no | no |
| [`web_origin`](#web_origin) | `MY_WEB_ORIGIN` | `https://{host}` | no | no |
| [`role`](#role) | `MY_ROLE` | `all` | no | no |
| [`listen`](#listen) | `MY_LISTEN` | `:8080` | no | no |
| [`trusted_proxies`](#trusted_proxies) | `MY_TRUSTED_PROXIES` | none | no | no |
| [`trusted_proxy_header`](#trusted_proxy_header) | `MY_TRUSTED_PROXY_HEADER` | `x-forwarded-for` | no | no |
| [`db`](#db) | `MY_DB` | `sqlite` | no | no |
| [`database_url`](#database_url) | `MY_DATABASE_URL` | none | yes | yes |
| [`queue`](#queue) | `MY_QUEUE` | `db` | no | no |
| [`blob`](#blob) | `MY_BLOB` | `fs` | no | no |
| [`media`](#media) | `MY_MEDIA` | `native` | no | no |
| [`blob_fs_dir`](#blob_fs_dir) | `MY_BLOB_FS_DIR` | `blobs` | no | no |
| [`media_max_image_bytes`](#media_max_image_bytes) | `MY_MEDIA_MAX_IMAGE_BYTES` | `16777216` | no | no |
| [`media_max_image_side`](#media_max_image_side) | `MY_MEDIA_MAX_IMAGE_SIDE` | `8192` | no | no |
| [`media_max_image_pixels`](#media_max_image_pixels) | `MY_MEDIA_MAX_IMAGE_PIXELS` | `33177600` | no | no |
| [`media_unattached_ttl`](#media_unattached_ttl) | `MY_MEDIA_UNATTACHED_TTL` | `24h` | no | no |
| [`media_max_av_bytes`](#media_max_av_bytes) | `MY_MEDIA_MAX_AV_BYTES` | `104857600` | no | no |
| [`media_max_video_duration`](#media_max_video_duration) | `MY_MEDIA_MAX_VIDEO_DURATION` | `5m` | no | no |
| [`media_max_audio_duration`](#media_max_audio_duration) | `MY_MEDIA_MAX_AUDIO_DURATION` | `10m` | no | no |
| [`custom_emoji_max_bytes`](#custom_emoji_max_bytes) | `MY_CUSTOM_EMOJI_MAX_BYTES` | `262144` | no | no |
| [`remote_custom_emoji_max_bytes`](#remote_custom_emoji_max_bytes) | `MY_REMOTE_CUSTOM_EMOJI_MAX_BYTES` | `1048576` | no | no |
| [`remote_media_cache_max_bytes`](#remote_media_cache_max_bytes) | `MY_REMOTE_MEDIA_CACHE_MAX_BYTES` | `1073741824` | no | no |
| [`kv`](#kv) | `MY_KV` | `db` | no | no |
| [`valkey_url`](#valkey_url) | `MY_VALKEY_URL` | none | when `kv = valkey` or `stream = valkey` | yes |
| [`valkey_tls_ca_file`](#valkey_tls_ca_file) | `MY_VALKEY_TLS_CA_FILE` | none | no | no |
| [`valkey_key_prefix`](#valkey_key_prefix) | `MY_VALKEY_KEY_PREFIX` | `my:` | no | no |
| [`stream`](#stream) | `MY_STREAM` | `hub` | no | no |
| [`stream_postgres_url`](#stream_postgres_url) | `MY_STREAM_POSTGRES_URL` | value of `database_url` | no | yes |
| [`stream_sns_topic_arn`](#stream_sns_topic_arn) | `MY_STREAM_SNS_TOPIC_ARN` | none | when `stream = sns` | no |
| [`stream_sqs_queue_prefix`](#stream_sqs_queue_prefix) | `MY_STREAM_SQS_QUEUE_PREFIX` | none | when `stream = sns` | no |
| [`stream_pubsub_topic`](#stream_pubsub_topic) | `MY_STREAM_PUBSUB_TOPIC` | none | when `stream = pubsub` | no |
| [`stream_pubsub_subscription_prefix`](#stream_pubsub_subscription_prefix) | `MY_STREAM_PUBSUB_SUBSCRIPTION_PREFIX` | none | when `stream = pubsub` | no |
| [`key_manager`](#key_manager) | `MY_KEY_MANAGER` | `local` | no | no |
| [`kek`](#kek) | `MY_KEK` | none | when `key_manager = local` | yes |
| [`kek_previous`](#kek_previous) | `MY_KEK_PREVIOUS` | none | no | yes |
| [`mail`](#mail) | `MY_MAIL` | `none` | no | no |
| [`mail_from`](#mail_from) | `MY_MAIL_FROM` | none | when `mail` is not `none` | no |
| [`smtp_host`](#smtp_host) | `MY_SMTP_HOST` | none | when `mail = smtp` | no |
| [`smtp_port`](#smtp_port) | `MY_SMTP_PORT` | `587` | no | no |
| [`smtp_username`](#smtp_username) | `MY_SMTP_USERNAME` | none | no | no |
| [`smtp_password`](#smtp_password) | `MY_SMTP_PASSWORD` | none | no | yes |
| [`captcha`](#captcha) | `MY_CAPTCHA` | `none` | no | no |
| [`captcha_site_key`](#captcha_site_key) | `MY_CAPTCHA_SITE_KEY` | none | when `captcha` is not `none` | no |
| [`captcha_secret_key`](#captcha_secret_key) | `MY_CAPTCHA_SECRET_KEY` | none | when `captcha` is not `none` | yes |
| [`vapid_private_key`](#vapid_private_key) | `MY_VAPID_PRIVATE_KEY` | none | no | yes |
| [`profile`](#profile) | `MY_PROFILE` | `production` | no | no |
| [`http_allow_private_networks`](#http_allow_private_networks) | `MY_HTTP_ALLOW_PRIVATE_NETWORKS` | `false` | no | no |
| [`webhook_allowed_destinations`](#webhook_allowed_destinations) | `MY_WEBHOOK_ALLOWED_DESTINATIONS` | none | no | no |
| [`remote_status_retention`](#remote_status_retention) | `MY_REMOTE_STATUS_RETENTION` | `2160h` | no | no |
| [`remote_status_max_retention`](#remote_status_max_retention) | `MY_REMOTE_STATUS_MAX_RETENTION` | none | no | no |
| [`relay_status_retention`](#relay_status_retention) | `MY_RELAY_STATUS_RETENTION` | `168h` | no | no |
| [`relay_ingest_hourly_limit`](#relay_ingest_hourly_limit) | `MY_RELAY_INGEST_HOURLY_LIMIT` | `1000` | no | no |
| [`billing_sources`](#billing_sources) | `MY_BILLING_SOURCES` | none | no | no |
| [`billing_aws_max_requests_per_day`](#billing_aws_max_requests_per_day) | `MY_BILLING_AWS_MAX_REQUESTS_PER_DAY` | `4` | no | no |
| [`billing_aws_max_requests_per_month`](#billing_aws_max_requests_per_month) | `MY_BILLING_AWS_MAX_REQUESTS_PER_MONTH` | `120` | no | no |
| [`billing_aws_metric`](#billing_aws_metric) | `MY_BILLING_AWS_METRIC` | `NetUnblendedCost` | no | no |
| [`billing_aws_filter`](#billing_aws_filter) | `MY_BILLING_AWS_FILTER` | `none` | no | no |
| [`billing_aws_tag`](#billing_aws_tag) | `MY_BILLING_AWS_TAG` | none | when `billing_aws_filter = tag` | no |
| [`billing_aws_linked_accounts`](#billing_aws_linked_accounts) | `MY_BILLING_AWS_LINKED_ACCOUNTS` | none | when `billing_aws_filter = linked_account` | no |
| [`billing_aws_role_arn`](#billing_aws_role_arn) | `MY_BILLING_AWS_ROLE_ARN` | none | no | no |
| [`billing_gcp_method`](#billing_gcp_method) | `MY_BILLING_GCP_METHOD` | `bigquery` | no | no |
| [`billing_gcp_table`](#billing_gcp_table) | `MY_BILLING_GCP_TABLE` | none | when `billing_sources` includes `gcp` and `billing_gcp_method = bigquery` | no |
| [`billing_gcp_query_project`](#billing_gcp_query_project) | `MY_BILLING_GCP_QUERY_PROJECT` | none | no | no |
| [`billing_gcp_projects`](#billing_gcp_projects) | `MY_BILLING_GCP_PROJECTS` | none | no | no |
| [`billing_gcp_label`](#billing_gcp_label) | `MY_BILLING_GCP_LABEL` | none | no | no |
| [`billing_gcp_max_bytes_billed`](#billing_gcp_max_bytes_billed) | `MY_BILLING_GCP_MAX_BYTES_BILLED` | `1073741824` | no | no |
| [`billing_gcp_budget_subscription`](#billing_gcp_budget_subscription) | `MY_BILLING_GCP_BUDGET_SUBSCRIPTION` | none | when `billing_sources` includes `gcp` and `billing_gcp_method = budget` | no |
| [`payments_provider`](#payments_provider) | `MY_PAYMENTS_PROVIDER` | `none` | no | no |
| [`payments_stripe_secret_key`](#payments_stripe_secret_key) | `MY_PAYMENTS_STRIPE_SECRET_KEY` | none | when `payments_provider = stripe` | yes |
| [`payments_stripe_webhook_secret`](#payments_stripe_webhook_secret) | `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` | none | when `payments_provider = stripe` | yes |
| [`payments_reconcile_interval`](#payments_reconcile_interval) | `MY_PAYMENTS_RECONCILE_INTERVAL` | `6h` | no | no |
| [`payments_reconcile_max_requests`](#payments_reconcile_max_requests) | `MY_PAYMENTS_RECONCILE_MAX_REQUESTS` | `1000` | no | no |
| [`dead_job_retention`](#dead_job_retention) | `MY_DEAD_JOB_RETENTION` | `720h` | no | no |
| [`job_attempt_retention`](#job_attempt_retention) | `MY_JOB_ATTEMPT_RETENTION` | `720h` | no | no |
| [`dynamic_client_unredeemed_ttl`](#dynamic_client_unredeemed_ttl) | `MY_DYNAMIC_CLIENT_UNREDEEMED_TTL` | `24h` | no | no |
| [`dynamic_client_idle_ttl`](#dynamic_client_idle_ttl) | `MY_DYNAMIC_CLIENT_IDLE_TTL` | `2160h` | no | no |
| [`oauth_client_rate_limits`](#oauth_client_rate_limits) | `MY_OAUTH_CLIENT_RATE_LIMITS` | none | no | no |
| [`metrics_exporter`](#metrics_exporter) | `MY_METRICS_EXPORTER` | `none` | no | no |
| [`metrics_listen`](#metrics_listen) | `MY_METRICS_LISTEN` | `127.0.0.1:9464` | no | no |
| [`metrics_emf_namespace`](#metrics_emf_namespace) | `MY_METRICS_EMF_NAMESPACE` | `my` | no | no |
| [`metrics_emf_dimensions`](#metrics_emf_dimensions) | `MY_METRICS_EMF_DIMENSIONS` | none | no | no |
| [`traces_exporter`](#traces_exporter) | `MY_TRACES_EXPORTER` | `none` | no | no |
| [`traces_sample_ratio`](#traces_sample_ratio) | `MY_TRACES_SAMPLE_RATIO` | `0.01` | no | no |
| [`traces_flush`](#traces_flush) | `MY_TRACES_FLUSH` | `batch` | no | no |

## Settings

### `host`

Domain that serves the instance. Every ActivityPub `id` is under `https://{host}/` (ADR-0018). A port is allowed for local development. Recorded in the database on first start and must not change afterwards.

- Environment variable: `MY_HOST`
- Default: none
- Required: yes

### `account_domain`

Domain of user handles (`@alice@{account_domain}`). Defaults to `host`. When it differs, `account_domain` must redirect `/.well-known/webfinger`, `/.well-known/host-meta`, and `/.well-known/nodeinfo` to `host` (ADR-0018). Recorded in the database on first start and must not change afterwards.

- Environment variable: `MY_ACCOUNT_DOMAIN`
- Default: value of `host`
- Required: no

### `source_url`

Where users get the source code of the running server, as an `https` URL or a `mailto:` address to request it from. AGPL-3.0 section 13 (ADR-0011) requires offering the source to everyone who uses the server over the network; `GET /instance`, the information page (`/about`), the landing page, and NodeInfo (`metadata.sourceCode`, and `software.repository` for an `https` URL) publish it. Point it at your own source when you run a fork or a modified version (docs/operations/running.md).

- Environment variable: `MY_SOURCE_URL`
- Default: `https://github.com/mycroblog/my`
- Required: no

### `web_origin`

Origin (`scheme://host[:port]`) of the web client. The client API allows only this origin by CORS (ADR-0019 §4). Set it when the web client is served from another origin, for example a development server at `http://localhost:5173`. An origin on `host` is where the server itself serves the web client, and the client served there calls the API at it: set `http://localhost:8080` (with `host` = `localhost:8080`) to run the server over plain HTTP on localhost, where the sign-in screens, the OAuth issuer, the client API, the links to the instance's pages in its responses, media URLs, and email and invite links are all on `http://localhost:8080`. ActivityPub ids stay under `https://{host}/` in every setup.

- Environment variable: `MY_WEB_ORIGIN`
- Default: `https://{host}`
- Required: no

### `role`

What this process runs.

- Environment variable: `MY_ROLE`
- Default: `all`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `all` | yes | HTTP server and job worker in one process. |
| `web` | yes | HTTP server only. |
| `worker` | yes | Job worker only; the HTTP server answers only the health checks (docs/operations/running.md). |

### `listen`

TCP address the HTTP server listens on, as `host:port`.

- Environment variable: `MY_LISTEN`
- Default: `:8080`
- Required: no

### `trusted_proxies`

Reverse proxies and load balancers in front of the server whose forwarding header (`trusted_proxy_header`) tells the client IP, as comma-separated CIDRs or addresses such as `10.0.0.0/8, 2001:db8::/32, 192.0.2.10` (ADR-0019 §5). The client IP keys the rate limits of sign-in, registration, the client API, and the inbox. When the connection's peer is in this list, the client IP is the rightmost address of the header that is not in it; otherwise the header is ignored and the client IP is the peer. Empty trusts no one: behind a proxy every request then shares the proxy's limits. List only addresses that nothing but your proxies can connect from, and only proxies that append to the header rather than pass a client's value through (docs/architecture/deployment-targets.md).

- Environment variable: `MY_TRUSTED_PROXIES`
- Default: none
- Required: no

### `trusted_proxy_header`

Forwarding header that the `trusted_proxies` append the client IP to. Only this header is read, so that a client cannot add the other one past a proxy that does not know it.

- Environment variable: `MY_TRUSTED_PROXY_HEADER`
- Default: `x-forwarded-for`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `x-forwarded-for` | yes | `X-Forwarded-For`, which nginx, Caddy, Traefik, Google Cloud load balancers and Cloud Run, and AWS Application Load Balancers append to. |
| `forwarded` | yes | The `for` parameters of `Forwarded` (RFC 7239). |

### `db`

Database adapter (ADR-0003). The adapter is never inferred from `database_url`.

- Environment variable: `MY_DB`
- Default: `sqlite`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `sqlite` | yes | Local SQLite file (`modernc.org/sqlite`). The build tag `no_sqlite` leaves it out. |
| `libsql` | yes | libSQL / Turso. |
| `postgres` | yes | PostgreSQL and compatible services (`pgx/v5`). The build tag `no_postgres` leaves it out. |

### `database_url`

Connection string passed to the `db` adapter, in that adapter's format.

- Environment variable: `MY_DATABASE_URL` or `MY_DATABASE_URL_FILE`
- Default: none
- Required: yes

### `queue`

Job queue adapter (ADR-0004).

- Environment variable: `MY_QUEUE`
- Default: `db`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `db` | yes | Queue table in the database. |
| `inline` | yes | Runs jobs synchronously in the enqueuing process; for tests and development. Requires `role = all`. |
| `sqs` | yes | Amazon SQS. |
| `cloudtasks` | yes | Google Cloud Tasks. |

### `blob`

Blob storage adapter for media (ADR-0005).

- Environment variable: `MY_BLOB`
- Default: `fs`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `fs` | yes | Local file system. |
| `s3` | yes | S3-compatible API (AWS S3, Cloudflare R2, GCS XML API, MinIO). |
| `gcs` | not in v0 | Native Google Cloud Storage API. |

### `media`

Media processor adapter (ADR-0005).

- Environment variable: `MY_MEDIA`
- Default: `native`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `native` | yes | Pure Go image processing. |
| `vips` | yes | libvips; needs a cgo build with the adapter's build tag. |
| `none` | yes | No processing except EXIF removal. |

### `blob_fs_dir`

Directory of the `fs` blob store, relative to the working directory or absolute. It holds the uploads as received (private) and the processed media, which the server serves under `/media/`. Share it between the web and worker processes, and back it up with the database.

- Environment variable: `MY_BLOB_FS_DIR`
- Default: `blobs`
- Required: no

### `media_max_image_bytes`

Largest accepted image upload in bytes (ADR-0019 §8).

- Environment variable: `MY_MEDIA_MAX_IMAGE_BYTES`
- Default: `16777216`
- Required: no

### `media_max_image_side`

Largest accepted image width and height in pixels, read from the file's header before decoding (ADR-0019 §8).

- Environment variable: `MY_MEDIA_MAX_IMAGE_SIDE`
- Default: `8192`
- Required: no

### `media_max_image_pixels`

Largest accepted image area (width × height, and for an animated GIF the frames together), read from the file's header before decoding (ADR-0019 §8). The default is 33 MP (7680 × 4320).

- Environment variable: `MY_MEDIA_MAX_IMAGE_PIXELS`
- Default: `33177600`
- Required: no

### `media_unattached_ttl`

How long an upload may stay unattached to any post or profile before the hourly cleanup deletes it (FR-803), as a Go duration such as `24h`.

- Environment variable: `MY_MEDIA_UNATTACHED_TTL`
- Default: `24h`
- Required: no

### `media_max_av_bytes`

Largest accepted video or audio upload in bytes (ADR-0019 §8, ADR-0054 §6). The file is held in memory while it is checked and remuxed, so this also bounds the memory of one upload.

- Environment variable: `MY_MEDIA_MAX_AV_BYTES`
- Default: `104857600`
- Required: no

### `media_max_video_duration`

Longest accepted video upload (ADR-0054 §6), as a Go duration such as `5m`.

- Environment variable: `MY_MEDIA_MAX_VIDEO_DURATION`
- Default: `5m`
- Required: no

### `media_max_audio_duration`

Longest accepted audio upload (ADR-0054 §6), as a Go duration such as `10m`.

- Environment variable: `MY_MEDIA_MAX_AUDIO_DURATION`
- Default: `10m`
- Required: no

### `custom_emoji_max_bytes`

Largest image in bytes that a local custom emoji may be registered with (ADR-0051 §2); the image must also be a PNG, a GIF, or a WebP of at most 1,024 pixels on a side. The default is Mastodon's limit. GoToSocial does not fetch remote emoji larger than 100 KiB by default.

- Environment variable: `MY_CUSTOM_EMOJI_MAX_BYTES`
- Default: `262144`
- Required: no

### `remote_custom_emoji_max_bytes`

Largest image in bytes of a remote custom emoji that the media proxy serves (ADR-0051 §2); the proxy answers 404 for a larger one, and for one that is not a PNG, a GIF, or a WebP.

- Environment variable: `MY_REMOTE_CUSTOM_EMOJI_MAX_BYTES`
- Default: `1048576`
- Required: no

### `remote_media_cache_max_bytes`

Size limit in bytes of the copies of remote media that the media proxy keeps when the instance setting `remote_media_policy` is `avatars` or `all` (FR-409). The hourly cleanup deletes the expired copies, then the least recently accessed ones over this limit (FR-803).

- Environment variable: `MY_REMOTE_MEDIA_CACHE_MAX_BYTES`
- Default: `1073741824`
- Required: no

### `kv`

Key-value store adapter for caches and rate limits (ADR-0003, ADR-0019 §5).

- Environment variable: `MY_KV`
- Default: `db`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `db` | yes | Table in the database. |
| `memory` | yes | In-process memory; limits are per process. |
| `valkey` | yes | Valkey or another server that speaks the Redis protocol (`valkey_url`); entries are shared by every process on the server. The build tag `no_valkey` leaves it out. |

### `valkey_url`

Valkey server of the adapters that use one (`kv = valkey`, `stream = valkey`):`valkey://[user:password@]host[:port][/db]` (`redis://` is the same), `valkeys://` or `rediss://` for TLS, or `unix:///path/to/socket`. The path selects the database number. A cluster is found by itself.

- Environment variable: `MY_VALKEY_URL` or `MY_VALKEY_URL_FILE`
- Default: none
- Required: when `kv = valkey` or `stream = valkey`

### `valkey_tls_ca_file`

PEM file of the certificate authorities that the TLS certificate of `valkey_url` chains to, for a managed service with a private CA. Empty uses the system roots; it needs a `valkeys://` or `rediss://` URL.

- Environment variable: `MY_VALKEY_TLS_CA_FILE`
- Default: none
- Required: no

### `valkey_key_prefix`

Prefix of every key that this server writes to `valkey_url`, so that several instances or other applications can share the server.

- Environment variable: `MY_VALKEY_KEY_PREFIX`
- Default: `my:`
- Required: no

### `stream`

Broker that carries the signals of the real-time updates (`GET /api/v1/streaming`) between processes (ADR-0050). It is never inferred from the other settings. With `hub`, a stream receives only the changes made in its own process, so run one process with `role = all`; a process with another role warns at start. The other brokers reach every process of the instance, and a process connects to them only while it has open streams (docs/architecture/deployment-targets.md).

- Environment variable: `MY_STREAM`
- Default: `hub`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `hub` | yes | In-process only; for one process with `role = all` (SQLite, or one PostgreSQL server process). |
| `postgres` | yes | PostgreSQL `LISTEN` / `NOTIFY` on the database of `database_url` (or `stream_postgres_url`). Requires `db = postgres`. The build tag `no_postgres` leaves it out. |
| `valkey` | yes | Valkey Pub/Sub on `valkey_url`, with the channel `{valkey_key_prefix}stream`. The build tag `no_valkey` leaves it out. |
| `sns` | yes | An Amazon SNS standard topic (`stream_sns_topic_arn`) that fans out to one SQS queue per listening process (`stream_sqs_queue_prefix`); credentials and settings from the AWS SDK's default chain. The build tag `no_sns` leaves it out. |
| `pubsub` | yes | A Google Cloud Pub/Sub topic (`stream_pubsub_topic`) with one subscription per listening process (`stream_pubsub_subscription_prefix`); Application Default Credentials, or the emulator at `PUBSUB_EMULATOR_HOST`. The build tag `no_pubsub` leaves it out. |

### `stream_postgres_url`

Connection string of the `LISTEN` connections of `stream = postgres`, in the format of `database_url`. Set it when `database_url` goes through a pooler in transaction mode (PgBouncer's transaction pooling, Neon's pooled connection string, Supabase's port 6543), where `LISTEN` does not work: it must reach the same database directly or through a pooler in session mode.

- Environment variable: `MY_STREAM_POSTGRES_URL` or `MY_STREAM_POSTGRES_URL_FILE`
- Default: value of `database_url`
- Required: no

### `stream_sns_topic_arn`

ARN of the Amazon SNS standard topic of `stream = sns`, which the deployment template creates. The queues are created in its account and region.

- Environment variable: `MY_STREAM_SNS_TOPIC_ARN`
- Default: none
- Required: when `stream = sns`

### `stream_sqs_queue_prefix`

Start of the names of the per-process SQS queues of `stream = sns`, `{prefix}-{process id}-{n}`: at most 40 letters, digits, hyphens, and underscores. The `stream.sweep` job deletes the queues under it that ended processes left behind, so it belongs to one instance.

- Environment variable: `MY_STREAM_SQS_QUEUE_PREFIX`
- Default: none
- Required: when `stream = sns`

### `stream_pubsub_topic`

Full name of the Google Cloud Pub/Sub topic of `stream = pubsub`, `projects/{project}/topics/{topic}`, which the deployment template creates. The subscriptions are created in its project.

- Environment variable: `MY_STREAM_PUBSUB_TOPIC`
- Default: none
- Required: when `stream = pubsub`

### `stream_pubsub_subscription_prefix`

Start of the IDs of the per-process subscriptions of `stream = pubsub`, `{prefix}-{process id}-{n}`: a letter, then letters, digits, and `-_.~+%`, not starting with `goog`. The `stream.sweep` job deletes the subscriptions under it that ended processes left behind, so it belongs to one instance.

- Environment variable: `MY_STREAM_PUBSUB_SUBSCRIPTION_PREFIX`
- Default: none
- Required: when `stream = pubsub`

### `key_manager`

Key manager adapter that encrypts actor private keys (ADR-0019 §6).

- Environment variable: `MY_KEY_MANAGER`
- Default: `local`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `local` | yes | Uses the key-encryption key from `kek`. |
| `gcpkms` | not in v0 | Google Cloud KMS. |
| `awskms` | not in v0 | AWS KMS. |

### `kek`

Key-encryption key for actor private keys: 32 random bytes in standard base64 (for example `openssl rand -base64 32`). Required when `key_manager = local`. Back it up separately from the database; losing it loses every actor key (ADR-0019 §6, docs/operations/key-management.md).

- Environment variable: `MY_KEK` or `MY_KEK_FILE`
- Default: none
- Required: when `key_manager = local`

### `kek_previous`

Previous key-encryption keys, comma-separated, each in the format of `kek`. They only decrypt; new secrets are always encrypted with `kek`. Set the replaced key here while rotating `kek` (docs/operations/key-management.md).

- Environment variable: `MY_KEK_PREVIOUS` or `MY_KEK_PREVIOUS_FILE`
- Default: none
- Required: no

### `mail`

Mailer adapter for verification emails (FR-002). With `none`, no email reaches users, so an unverified address does not keep a user out and sign-ups in the `open` registration mode wait for a moderator's approval instead.

- Environment variable: `MY_MAIL`
- Default: `none`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `none` | yes | Logs each message, including its links, instead of sending it; for development and instances without email. |
| `smtp` | yes | SMTP submission with mandatory STARTTLS (`smtp_host`, `smtp_port`, `smtp_username`, `smtp_password`). |
| `ses` | not in v0 | Amazon SES. |

### `mail_from`

Sender address of emails, as a bare address such as `noreply@social.example`.

- Environment variable: `MY_MAIL_FROM`
- Default: none
- Required: when `mail` is not `none`

### `smtp_host`

Host name of the SMTP server. The server must offer STARTTLS with a certificate valid for this name; the connection is refused otherwise.

- Environment variable: `MY_SMTP_HOST`
- Default: none
- Required: when `mail = smtp`

### `smtp_port`

TCP port of the SMTP server (submission).

- Environment variable: `MY_SMTP_PORT`
- Default: `587`
- Required: no

### `smtp_username`

User name for SMTP AUTH PLAIN, sent only after STARTTLS. Leave it empty for a server that needs no authentication.

- Environment variable: `MY_SMTP_USERNAME`
- Default: none
- Required: no

### `smtp_password`

Password for SMTP AUTH PLAIN.

- Environment variable: `MY_SMTP_PASSWORD` or `MY_SMTP_PASSWORD_FILE`
- Default: none
- Required: no

### `captcha`

CAPTCHA service that the sign-in and registration forms check (ADR-0042). The service receives the visitor's IP address, browser details, and cookies, and the two pages load its script; say so in the instance's privacy policy. Only `none` keeps every request on this server.

- Environment variable: `MY_CAPTCHA`
- Default: `none`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `none` | yes | No CAPTCHA; nothing is sent to a third party. |
| `turnstile` | yes | Cloudflare Turnstile. |
| `hcaptcha` | yes | hCaptcha. |
| `recaptcha` | yes | Google reCAPTCHA v2 with the checkbox (not v3 or Enterprise). |

### `captcha_site_key`

Public site key of the `captcha` service, which the widget on the forms carries.

- Environment variable: `MY_CAPTCHA_SITE_KEY`
- Default: none
- Required: when `captcha` is not `none`

### `captcha_secret_key`

Secret key of the `captcha` service, which the server sends with each token to the service's siteverify API. The server does not try the key at start: a key that the service refuses is logged as an error on the first check, and until it is fixed registrations are refused and sign-ins skip the CAPTCHA (ADR-0042 §4).

- Environment variable: `MY_CAPTCHA_SECRET_KEY` or `MY_CAPTCHA_SECRET_KEY_FILE`
- Default: none
- Required: when `captcha` is not `none`

### `vapid_private_key`

VAPID private key of the server's Web Push notifications (FR-206, RFC 8292): a P-256 private key, 32 bytes in base64url. Generate one with `my config vapid-key`. Without it, the server sends no Web Push and clients do not offer it. Changing it ends every subscription, since browsers subscribe with its public key (docs/operations/key-management.md).

- Environment variable: `MY_VAPID_PRIVATE_KEY` or `MY_VAPID_PRIVATE_KEY_FILE`
- Default: none
- Required: no

### `profile`

Whether this is a production instance. Settings that weaken a security default for development, such as `http_allow_private_networks`, are refused in the `production` profile (ADR-0019).

- Environment variable: `MY_PROFILE`
- Default: `production`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `production` | yes | A production instance. |
| `development` | yes | A development or test instance. |

### `http_allow_private_networks`

Lets outbound HTTP (federation, media, link previews) reach loopback, private, and other non-public addresses and host names, over plain `http` and on any port. For development and tests against local servers only: the server logs a warning at start, and the `production` profile refuses it (ADR-0019 §1).

- Environment variable: `MY_HTTP_ALLOW_PRIVATE_NETWORKS`
- Default: `false`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `false` | yes | Only public `https` destinations on ports 443 and 80. |
| `true` | yes | Every destination; requires `profile = development`. |

### `webhook_allowed_destinations`

Destinations on loopback or a private network that the instance's webhooks may reach (ADR-0060), such as an automation tool on the same host or in the same VPC: comma-separated host names, IP addresses, and CIDRs such as `n8n.internal, 10.0.0.0/8`. The SSRF guard of outbound HTTP refuses them otherwise; for these destinations alone, and for the webhooks alone, it also allows plain `http` and any port. A host name is allowed whatever it resolves to; an address range allows the URLs that name an address in it and the host names that resolve into it only when they are listed too. Only the operator can change it: the administrators of the web client cannot reach an internal network on their own. Empty allows none.

- Environment variable: `MY_WEBHOOK_ALLOWED_DESTINATIONS`
- Default: none
- Required: no

### `remote_status_retention`

How long a remote post is kept after its publication when nothing on this instance refers to it (a local reply, boost, favourite, bookmark, notification, or a thread with a local post); the hourly purge then deletes it (ADR-0034, FR-803). A Go duration such as `2160h` (90 days).

- Environment variable: `MY_REMOTE_STATUS_RETENTION`
- Default: `2160h`
- Required: no

### `remote_status_max_retention`

The longest a remote post is kept after its publication, even when this instance refers to it; the hourly purge then deletes it, while local accounts' replies, boosts, favourites, and bookmarks stay and show it as deleted (ADR-0034). A Go duration such as `8760h` (365 days); empty keeps referred posts without limit.

- Environment variable: `MY_REMOTE_STATUS_MAX_RETENTION`
- Default: none
- Required: no

### `relay_status_retention`

How long a remote post that only a relay brought in is kept after its publication when nothing on this instance refers to it and no local account follows its author; the hourly purge then deletes it (ADR-0061). A Go duration such as `168h` (7 days).

- Environment variable: `MY_RELAY_STATUS_RETENTION`
- Default: `168h`
- Required: no

### `relay_ingest_hourly_limit`

How many activities each relay may bring in per hour (ADR-0061). The ones over it are dropped before their posts are fetched, and counted in `my.relay.activities` and on the relays screen. It bounds the fetches, rows, and jobs that a relay costs.

- Environment variable: `MY_RELAY_INGEST_HOURLY_LIMIT`
- Default: `1000`
- Required: no

### `billing_sources`

Cloud providers whose usage charges are imported into the instance's costs every 24 hours (ADR-0068, FR-904), comma-separated: `aws` (AWS Cost Explorer, about $0.01 per billed request) and `gcp` (the Cloud Billing export to BigQuery, or the Pub/Sub notifications of a budget: `billing_gcp_method`). Each import records the charges of the current billing month and of the previous one (through the 15th, or until it is final) by service and charge type, as estimates until the provider closes the month, then final, then corrections; `GET /api/v1/admin/billing_imports` shows the state of the imports. A provider whose adapter is not in this build is refused at start. Credentials come from the SDK's default chain of each provider (docs/operations/billing-import.md). The build tags `no_billing_aws` and `no_billing_gcp` leave `aws` and `gcp` out. Empty imports nothing.

- Environment variable: `MY_BILLING_SOURCES`
- Default: none
- Required: no

### `billing_aws_max_requests_per_day`

How many billed AWS Cost Explorer requests (one per page, $0.01 each) the imports of `billing_sources = aws` may make per UTC day, counting the scheduled imports, their retries, and the imports that an administrator starts (ADR-0068 §4). An import with none left calls nothing and is recorded as `request_cap`; the next day's import tries again.

- Environment variable: `MY_BILLING_AWS_MAX_REQUESTS_PER_DAY`
- Default: `4`
- Required: no

### `billing_aws_max_requests_per_month`

How many billed AWS Cost Explorer requests the imports may make per UTC calendar month, the bound of the cost of the imports (the default is $1.20 a month: four imports a day for thirty days). An import with none left calls nothing and is recorded as `request_cap` until the next month.

- Environment variable: `MY_BILLING_AWS_MAX_REQUESTS_PER_MONTH`
- Default: `120`
- Required: no

### `billing_aws_metric`

Cost Explorer metric that the imports of `billing_sources = aws` record (ADR-0068 §6).

- Environment variable: `MY_BILLING_AWS_METRIC`
- Default: `NetUnblendedCost`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `NetUnblendedCost` | yes | The cost after the discounts (Savings Plans, Reserved Instances, private pricing), the closest to the invoice. |
| `UnblendedCost` | yes | The cost before the discounts. |

### `billing_aws_filter`

Which costs of the AWS account the imports of `billing_sources = aws` take (ADR-0068 §6). The most accurate is an AWS account that serves this instance alone, with `none`: the taxes and the support fees carry no tag and may fall out of a tag filter (docs/operations/billing-import.md).

- Environment variable: `MY_BILLING_AWS_FILTER`
- Default: `none`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `none` | yes | Every cost that the credentials can see: the account's own, or, from the management account of an organization, every member account's. |
| `linked_account` | yes | The costs of the member accounts of `billing_aws_linked_accounts` (the `LINKED_ACCOUNT` dimension). |
| `tag` | yes | The costs of the resources that carry the cost allocation tag `billing_aws_tag`, which must be activated in the Billing console first. |

### `billing_aws_tag`

The activated cost allocation tag of `billing_aws_filter = tag`, `key=value` such as `app=my`. A tag shows in the billing data from its activation on, up to 24 hours after it is activated; older costs need a backfill in the Billing console (docs/operations/billing-import.md).

- Environment variable: `MY_BILLING_AWS_TAG`
- Default: none
- Required: when `billing_aws_filter = tag`

### `billing_aws_linked_accounts`

The 12-digit IDs of the member accounts of `billing_aws_filter = linked_account`, comma-separated. Reading them from a member account needs Linked Account Access, which the management account turns on in the Cost Explorer preferences.

- Environment variable: `MY_BILLING_AWS_LINKED_ACCOUNTS`
- Default: none
- Required: when `billing_aws_filter = linked_account`

### `billing_aws_role_arn`

ARN of an IAM role that the imports of `billing_sources = aws` assume with the Google ID token of the instance's service account (`AssumeRoleWithWebIdentity`, audience: this ARN), for a server on Google Cloud (Cloud Run) without AWS keys (ADR-0068 §8). The role trusts the identity provider `accounts.google.com` for the service account (docs/operations/billing-import.md). Empty uses the AWS SDK's default chain (the execution role, `AWS_ACCESS_KEY_ID`, the shared files).

- Environment variable: `MY_BILLING_AWS_ROLE_ARN`
- Default: none
- Required: no

### `billing_gcp_method`

How the imports of `billing_sources = gcp` read the Google Cloud usage charges (ADR-0068 §7). The billing months are the invoice months of Cloud Billing, US Pacific calendar months; a month is final from the 5th of the next month on (Pacific time).

- Environment variable: `MY_BILLING_GCP_METHOD`
- Default: `bigquery`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `bigquery` | yes | Sum the standard usage cost export of Cloud Billing to BigQuery (`billing_gcp_table`) with one query a day, within the free tier of BigQuery (docs/operations/billing-import.md). |
| `budget` | yes | Read the notifications of a monthly Cloud Billing budget from a Pub/Sub pull subscription (`billing_gcp_budget_subscription`) once a day: the budget's estimated total of each month, one line named after the budget, without the services, the charge types, or the export's days (docs/operations/billing-import.md). A simple way for an operator without the BigQuery export; free within the free tier of Pub/Sub. |

### `billing_gcp_table`

The table of the standard usage cost export of Cloud Billing to BigQuery, `project.dataset.table` such as `my-project.billing_export.gcp_billing_export_v1_012345_6789AB_CDEF01`. The service account of the server needs `roles/bigquery.dataViewer` on its dataset (docs/operations/billing-import.md).

- Environment variable: `MY_BILLING_GCP_TABLE`
- Default: none
- Required: when `billing_sources` includes `gcp` and `billing_gcp_method = bigquery`

### `billing_gcp_query_project`

The Google Cloud project that runs the BigQuery queries of `billing_gcp_method = bigquery` and is billed for their bytes; the service account of the server needs `roles/bigquery.jobUser` on it. Empty uses the project of `billing_gcp_table`.

- Environment variable: `MY_BILLING_GCP_QUERY_PROJECT`
- Default: none
- Required: no

### `billing_gcp_projects`

Google Cloud project IDs, comma-separated, whose costs the imports of `billing_gcp_method = bigquery` take (`project.id` of the export). Empty takes every cost of the billing account. The taxes and the other charges of the invoice that carry no project may fall out of this filter (docs/operations/billing-import.md).

- Environment variable: `MY_BILLING_GCP_PROJECTS`
- Default: none
- Required: no

### `billing_gcp_label`

A label `key=value`, such as `app=my`, whose resources' costs the imports of `billing_gcp_method = bigquery` take (`labels` of the export). Empty takes the costs with or without labels. The taxes and the costs of resources without the label fall out of this filter.

- Environment variable: `MY_BILLING_GCP_LABEL`
- Default: none
- Required: no

### `billing_gcp_max_bytes_billed`

The `maximumBytesBilled` of each BigQuery query of `billing_gcp_method = bigquery`, in bytes: a query that would bill more fails as `bytes_cap` and bills nothing (ADR-0068 §4). The default, 1 GiB, keeps a query a day well within the free tier of 1 TiB a month, which the billing account shares with its other uses; BigQuery bills at least 10 MB a query.

- Environment variable: `MY_BILLING_GCP_MAX_BYTES_BILLED`
- Default: `1073741824`
- Required: no

### `billing_gcp_budget_subscription`

The pull subscription of the Pub/Sub topic of a monthly Cloud Billing budget that `billing_gcp_method = budget` reads, `projects/{project}/subscriptions/{subscription}`. The imports acknowledge a notification once a later one of its budget and month replaces it or once its month is final and no longer imported, and keep the latest of each month imported, so the subscription needs a message retention of at least 14 days (31 recommended) and must not be read by anything else. The service account of the server needs `roles/pubsub.subscriber` on it (docs/operations/billing-import.md).

- Environment variable: `MY_BILLING_GCP_BUDGET_SUBSCRIPTION`
- Default: none
- Required: when `billing_sources` includes `gcp` and `billing_gcp_method = budget`

### `payments_provider`

The payment provider of the supporters (ADR-0072, ADR-0074, FR-910): local users subscribe to paid plans on its hosted checkout, and the roles of the plans follow the state of their payments. Only one provider is active on a server; switching to another one needs the subscribers to subscribe again. The provider calls `POST /payments/webhooks/{provider}` (signed, at most 1 MiB, recorded once per event), and a reconciliation reads its lists every `payments_reconcile_interval`. Card details never reach the server. An administrator still enables the supporters and sets up the plans on the admin screen.

- Environment variable: `MY_PAYMENTS_PROVIDER`
- Default: `none`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `none` | yes | No payments: the webhook endpoint and the reconciliation do not run. |
| `stripe` | yes | Stripe Billing: Checkout in subscription mode and the customer portal, with `payments_stripe_secret_key` and `payments_stripe_webhook_secret` (docs/operations/payments-stripe.md). The build tag `no_payments_stripe` leaves it out. |

### `payments_stripe_secret_key`

The API key of the Stripe account of `payments_provider = stripe` (ADR-0074 §5): a restricted key (`rk_test_` / `rk_live_`) with the permissions of docs/operations/payments-stripe.md, or a secret key (`sk_test_` / `sk_live_`). Its prefix sets the mode: a test key charges nothing, and the webhook endpoint refuses live events with it (and test events with a live key).

- Environment variable: `MY_PAYMENTS_STRIPE_SECRET_KEY` or `MY_PAYMENTS_STRIPE_SECRET_KEY_FILE`
- Default: none
- Required: when `payments_provider = stripe`

### `payments_stripe_webhook_secret`

The signing secret (`whsec_`) of the webhook endpoint `https://<domain>/payments/webhooks/stripe` that the operator registers on Stripe's dashboard, or of `stripe listen` for local tests (ADR-0074 §3). While a secret is rolled, Stripe signs with the old and the new one, so one is enough.

- Environment variable: `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` or `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET_FILE`
- Default: none
- Required: when `payments_provider = stripe`

### `payments_reconcile_interval`

How often the reconciliation with `payments_provider` runs (ADR-0074 §4): it reads the provider's changes since the previous run, the checkouts in progress, the money movements, and a sweep of every subscription, and corrects what the webhooks missed. A Go duration from `15m` to `168h`; raise it, or `payments_reconcile_max_requests`, when the runs often stop at the cap.

- Environment variable: `MY_PAYMENTS_RECONCILE_INTERVAL`
- Default: `6h`
- Required: no

### `payments_reconcile_max_requests`

How many requests one reconciliation sends to `payments_provider` (ADR-0074 §4), a page of a list counting as one. A run that reaches it records where it stopped and a continuation picks up the rest a minute later. The provider's adapter also keeps the requests within its rate limits.

- Environment variable: `MY_PAYMENTS_RECONCILE_MAX_REQUESTS`
- Default: `1000`
- Required: no

### `dead_job_retention`

How long a dead job (a job that failed for good or used up its attempts, ADR-0004) is kept after it failed, for an operator to inspect and requeue it; the hourly purge then deletes it (FR-803). A Go duration such as `720h` (30 days).

- Environment variable: `MY_DEAD_JOB_RETENTION`
- Default: `720h`
- Required: no

### `job_attempt_retention`

How long the record of a job's run that did not succeed (a retry, or the run that made it a dead job; ADR-0064) is kept after the run, for the queue dashboard of the admin screen; the hourly purge then deletes it. Successful runs are not recorded. A Go duration such as `720h` (30 days).

- Environment variable: `MY_JOB_ATTEMPT_RETENTION`
- Default: `720h`
- Required: no

### `dynamic_client_unredeemed_ttl`

How long a dynamically registered OAuth client (RFC 7591, `POST /oauth/register`; ADR-0090 §1) may wait for the exchange of its first authorization code: the hourly clean-up deletes a client that is older than this and had no code exchanged. A Go duration such as `24h`.

- Environment variable: `MY_DYNAMIC_CLIENT_UNREDEEMED_TTL`
- Default: `24h`
- Required: no

### `dynamic_client_idle_ttl`

How long a dynamically registered OAuth client without a usable token may go unused before the hourly clean-up deletes it (ADR-0090 §1). Until the last use of a token is recorded, the time the client's newest token was issued stands for it. A Go duration such as `2160h` (90 days).

- Environment variable: `MY_DYNAMIC_CLIENT_IDLE_TTL`
- Default: `2160h`
- Required: no

### `oauth_client_rate_limits`

Caps on the client API requests of OAuth clients (ADR-0090 §7), for an administrator to hold back a client that overloads the server. A comma-separated list of `client_id=requests`: the requests that all the users of the client together may make in 5 minutes, on top of the per-account limit, such as `https://app.example/client.json=3000`. A client that is not listed is not limited. Over its cap, a request is answered `429` with `Retry-After`.

- Environment variable: `MY_OAUTH_CLIENT_RATE_LIMITS`
- Default: none
- Required: no

### `metrics_exporter`

Where the service metrics go (ADR-0055 §2): the request, federation, delivery, job, and stream metrics, named after the OpenTelemetry semantic conventions or with the prefix `my.`. Labels never carry a remote domain, an account, a URL, or a client IP.

- Environment variable: `MY_METRICS_EXPORTER`
- Default: `none`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `none` | yes | No metrics; measuring costs next to nothing. On Cloud Run the platform's metrics and log-based metrics stand in for them. |
| `prometheus` | yes | The Prometheus text format at `GET /metrics` on `metrics_listen`, a listener of its own, for a Prometheus-compatible server (and KEDA) to scrape; for self-hosted instances. The build tag `no_otel` leaves it out. |
| `otlp` | yes | OTLP over HTTP (protobuf) to an OpenTelemetry Collector, configured by the standard environment variables `OTEL_EXPORTER_OTLP_ENDPOINT` (default `http://localhost:4318`), `OTEL_EXPORTER_OTLP_HEADERS`, and `OTEL_METRIC_EXPORT_INTERVAL` (default 60 seconds). The build tag `no_otel` leaves it out. |
| `emf` | yes | CloudWatch Embedded Metric Format lines, which CloudWatch Logs turns into metrics; for AWS Lambda. The log line that ends each request and job carries its duration metrics, and the other measurements are written on standard output, one line per label set, at the end of each request and job. Requests in progress and open streams are not written. |

### `metrics_listen`

TCP address, as `host:port`, of the listener that serves `GET /metrics` when `metrics_exporter = prometheus`, apart from `listen`. It has no authentication: keep it on loopback or a private network, or protect it with the network or a proxy.

- Environment variable: `MY_METRICS_LISTEN`
- Default: `127.0.0.1:9464`
- Required: no

### `metrics_emf_namespace`

CloudWatch namespace of the metrics of `metrics_exporter = emf`.

- Environment variable: `MY_METRICS_EMF_NAMESPACE`
- Default: `my`
- Required: no

### `metrics_emf_dimensions`

Labels that become CloudWatch dimensions with `metrics_exporter = emf`, comma-separated, such as `my.surface, my.role`; the other labels stay fields of the log line. CloudWatch bills every combination of dimension values as a metric, so the deployment template chooses them. Empty makes every metric one series per namespace.

- Environment variable: `MY_METRICS_EMF_DIMENSIONS`
- Default: none
- Required: no

### `traces_exporter`

Where the distributed traces go (ADR-0063): the spans of the HTTP requests, the job runs (each the root of a new trace linked to the request that enqueued it), the transactions and queries, the deliveries, and the outbound requests. Span attributes never carry an account, a URL or a path, a client IP, a User-Agent, an SQL text, or an error message; the outbound requests carry the destination's host name. No outbound request carries a `traceparent`, and an incoming `traceparent` is taken only from `trusted_proxies`.

- Environment variable: `MY_TRACES_EXPORTER`
- Default: `none`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `none` | yes | No spans; tracing costs next to nothing. The log lines still carry `trace_id` and `span_id` of a `traceparent` from `trusted_proxies`, which correlates them with the platform's traces (Cloud Run). |
| `otlp` | yes | OTLP over HTTP (protobuf, gzip) to an OpenTelemetry Collector or another receiver, configured by the standard environment variables `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` or `OTEL_EXPORTER_OTLP_ENDPOINT` (default `http://localhost:4318`) and `OTEL_EXPORTER_OTLP_HEADERS`. The server does not authenticate to Cloud Trace or X-Ray itself: send to a collector next to it, which does (docs/operations/running.md). The build tag `no_otel` leaves it out. |

### `traces_sample_ratio`

Share of the traces recorded with `traces_exporter = otlp`, from `0` to `1`, among the requests and jobs without a sampled parent: a request follows the sampling of a `traceparent` from `trusted_proxies`, and a job is recorded when the request that enqueued it was. The default records about 1,000 traces a day of an instance that serves 100,000 requests a day.

- Environment variable: `MY_TRACES_SAMPLE_RATIO`
- Default: `0.01`
- Required: no

### `traces_flush`

When the spans of `traces_exporter = otlp` are sent.

- Environment variable: `MY_TRACES_FLUSH`
- Default: `batch`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `batch` | yes | In batches, every few seconds, in the background; for processes that keep running (self-hosted). |
| `unit` | yes | Also at the end of every recorded HTTP request and job, before it returns, to a collector on the same host; for the platforms that throttle or freeze an idle instance (Cloud Run with request-based billing, AWS Lambda). A request that is not recorded sends nothing. |
