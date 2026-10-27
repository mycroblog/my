<!-- Code generated from internal/platform/config/schema.go by "scripts/task.sh go:config-docs". DO NOT EDIT. -->

# Configuration reference

The server reads each setting from these sources, highest precedence first:

1. The environment variable `MY_<KEY>` (the key in upper case).
2. The TOML file named by `MY_CONFIG` (or `my config check -config <path>`), with the keys below at the top level.
3. The default.

Secrets are read only from the environment: either the variable itself or `<NAME>_FILE`, the path of a file that holds the value
(trailing newlines are removed), for Docker and Kubernetes secrets. Setting a secret in the TOML file is an error.

Adapters are always selected by an explicit key; nothing is inferred from a URL scheme.
Values marked "not in v0" are planned but not shipped in v0, and are rejected.
A binary built with a build tag that leaves an adapter out rejects the values that select it ([build tags](build-tags.md)).

`my config check` validates the configuration and lists every problem at once; the server refuses to start on any of them.
The problems are described in the language of the process locale (`LANGUAGE`, `LC_ALL`, `LC_MESSAGES`, `LANG`; English for `C` or an unsupported language),
followed by the technical detail in English.

A setting that a release renamed or removed is one of those problems, in the config file and in the environment alike:
the error names the new key, or tells why the setting was removed ([renamed and removed settings](#renamed-and-removed-settings)).
An unknown key in the config file is an error too. An environment variable that starts with `MY_` but sets nothing
is ignored with a warning, in the log of `my serve` and in the output of `my config check`, because the platform may add such variables:
Kubernetes adds `MY_SERVICE_HOST`, `MY_PORT` and the like for a Service named `my`, and those are ignored without a warning.

In the `production` profile, `my config check` then checks that the valid settings suit a production instance:
errors, such as a `host` that other servers cannot reach or a `kek` that is not random, make it exit 1, and warnings, such as `mail = none`, do not.
`my serve` logs both at start without refusing to start. The list of checks is in the [production checklist](../operations/production-checklist.md).

## Configuration and instance settings

The settings on this page configure the deployment: what a process runs, which adapters it uses, where they are, and its secrets.
These values are fixed for the life of a process and change only with a restart.
The policies of the instance that administrators change while it runs (registration mode, maximum post length,
default locale, which timelines are enabled) and its localized texts (name, descriptions, terms, privacy policy)
are instance settings instead: they are stored in the database with every change kept as a revision,
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
| [`status_page_url`](#status_page_url) | `MY_STATUS_PAGE_URL` | none | no | no |
| [`maintenance`](#maintenance) | `MY_MAINTENANCE` | `false` | no | no |
| [`maintenance_server_name`](#maintenance_server_name) | `MY_MAINTENANCE_SERVER_NAME` | value of `host` | no | no |
| [`maintenance_message`](#maintenance_message) | `MY_MAINTENANCE_MESSAGE` | none | no | no |
| [`maintenance_until`](#maintenance_until) | `MY_MAINTENANCE_UNTIL` | none | no | no |
| [`maintenance_retry_after`](#maintenance_retry_after) | `MY_MAINTENANCE_RETRY_AFTER` | `600` | no | no |
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
| [`media_origin`](#media_origin) | `MY_MEDIA_ORIGIN` | the server's own origin (`https://{host}`) | no | no |
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
| [`ses_region`](#ses_region) | `MY_SES_REGION` | none | no | no |
| [`ses_configuration_set`](#ses_configuration_set) | `MY_SES_CONFIGURATION_SET` | none | no | no |
| [`ses_sns_topic_arn`](#ses_sns_topic_arn) | `MY_SES_SNS_TOPIC_ARN` | none | no | no |
| [`captcha`](#captcha) | `MY_CAPTCHA` | `none` | no | no |
| [`captcha_site_key`](#captcha_site_key) | `MY_CAPTCHA_SITE_KEY` | none | when `captcha` is not `none` | no |
| [`captcha_secret_key`](#captcha_secret_key) | `MY_CAPTCHA_SECRET_KEY` | none | when `captcha` is not `none` | yes |
| [`vapid_private_key`](#vapid_private_key) | `MY_VAPID_PRIVATE_KEY` | none | no | yes |
| [`profile`](#profile) | `MY_PROFILE` | `production` | no | no |
| [`http_allow_private_networks`](#http_allow_private_networks) | `MY_HTTP_ALLOW_PRIVATE_NETWORKS` | `false` | no | no |
| [`webhook_allowed_destinations`](#webhook_allowed_destinations) | `MY_WEBHOOK_ALLOWED_DESTINATIONS` | none | no | no |
| [`login_provider_allowed_destinations`](#login_provider_allowed_destinations) | `MY_LOGIN_PROVIDER_ALLOWED_DESTINATIONS` | none | no | no |
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
| [`payments_payjp_secret_key`](#payments_payjp_secret_key) | `MY_PAYMENTS_PAYJP_SECRET_KEY` | none | when `payments_provider = payjp` | yes |
| [`payments_payjp_public_key`](#payments_payjp_public_key) | `MY_PAYMENTS_PAYJP_PUBLIC_KEY` | none | when `payments_provider = payjp` | no |
| [`payments_payjp_webhook_token`](#payments_payjp_webhook_token) | `MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN` | none | when `payments_provider = payjp` | yes |
| [`payments_reconcile_interval`](#payments_reconcile_interval) | `MY_PAYMENTS_RECONCILE_INTERVAL` | `6h` | no | no |
| [`payments_reconcile_max_requests`](#payments_reconcile_max_requests) | `MY_PAYMENTS_RECONCILE_MAX_REQUESTS` | `1000` | no | no |
| [`voice`](#voice) | `MY_VOICE` | `off` | no | no |
| [`voice_p2p_max_participants`](#voice_p2p_max_participants) | `MY_VOICE_P2P_MAX_PARTICIPANTS` | `10` | no | no |
| [`voice_max_participants`](#voice_max_participants) | `MY_VOICE_MAX_PARTICIPANTS` | `0` | no | no |
| [`voice_max_room_duration`](#voice_max_room_duration) | `MY_VOICE_MAX_ROOM_DURATION` | `0` | no | no |
| [`voice_max_open_rooms`](#voice_max_open_rooms) | `MY_VOICE_MAX_OPEN_ROOMS` | `0` | no | no |
| [`voice_stun_urls`](#voice_stun_urls) | `MY_VOICE_STUN_URLS` | none | no | no |
| [`voice_turn`](#voice_turn) | `MY_VOICE_TURN` | `none` | no | no |
| [`voice_turn_urls`](#voice_turn_urls) | `MY_VOICE_TURN_URLS` | none | when `voice = p2p` and `voice_turn = coturn` | no |
| [`voice_turn_secret`](#voice_turn_secret) | `MY_VOICE_TURN_SECRET` | none | when `voice = p2p` and `voice_turn = coturn` | yes |
| [`voice_turn_cloudflare_key_id`](#voice_turn_cloudflare_key_id) | `MY_VOICE_TURN_CLOUDFLARE_KEY_ID` | none | when `voice = p2p` and `voice_turn = cloudflare` | no |
| [`voice_turn_cloudflare_api_token`](#voice_turn_cloudflare_api_token) | `MY_VOICE_TURN_CLOUDFLARE_API_TOKEN` | none | when `voice = p2p` and `voice_turn = cloudflare` | yes |
| [`voice_turn_credential_ttl`](#voice_turn_credential_ttl) | `MY_VOICE_TURN_CREDENTIAL_TTL` | `6h` | no | no |
| [`voice_ice_transport_policy`](#voice_ice_transport_policy) | `MY_VOICE_ICE_TRANSPORT_POLICY` | `all` | no | no |
| [`voice_livekit_url`](#voice_livekit_url) | `MY_VOICE_LIVEKIT_URL` | none | when `voice = livekit` | no |
| [`voice_livekit_api_url`](#voice_livekit_api_url) | `MY_VOICE_LIVEKIT_API_URL` | `voice_livekit_url` with `https:` (`http:` for `ws:`) | no | no |
| [`voice_livekit_api_key`](#voice_livekit_api_key) | `MY_VOICE_LIVEKIT_API_KEY` | none | when `voice = livekit` | no |
| [`voice_livekit_api_secret`](#voice_livekit_api_secret) | `MY_VOICE_LIVEKIT_API_SECRET` | none | when `voice = livekit` | yes |
| [`voice_sora_mode`](#voice_sora_mode) | `MY_VOICE_SORA_MODE` | none | when `voice = sora` | no |
| [`voice_sora_signaling_urls`](#voice_sora_signaling_urls) | `MY_VOICE_SORA_SIGNALING_URLS` | none | when `voice = sora` | no |
| [`voice_sora_api_url`](#voice_sora_api_url) | `MY_VOICE_SORA_API_URL` | none | when `voice = sora` and `voice_sora_mode = self_hosted` | no |
| [`voice_sora_api_header_name`](#voice_sora_api_header_name) | `MY_VOICE_SORA_API_HEADER_NAME` | none | no | no |
| [`voice_sora_api_header_value`](#voice_sora_api_header_value) | `MY_VOICE_SORA_API_HEADER_VALUE` | none | when `voice = sora` and `voice_sora_api_header_name` is set | yes |
| [`voice_sora_webhook_username`](#voice_sora_webhook_username) | `MY_VOICE_SORA_WEBHOOK_USERNAME` | none | when `voice = sora` and `voice_sora_mode = self_hosted` | no |
| [`voice_sora_webhook_password`](#voice_sora_webhook_password) | `MY_VOICE_SORA_WEBHOOK_PASSWORD` | none | when `voice = sora` and `voice_sora_mode = self_hosted` | yes |
| [`voice_sora_cloud_project_id`](#voice_sora_cloud_project_id) | `MY_VOICE_SORA_CLOUD_PROJECT_ID` | none | when `voice = sora` and `voice_sora_mode = cloud` | no |
| [`voice_sora_cloud_api_key`](#voice_sora_cloud_api_key) | `MY_VOICE_SORA_CLOUD_API_KEY` | none | when `voice = sora` and `voice_sora_mode = cloud` | yes |
| [`voice_join_secret`](#voice_join_secret) | `MY_VOICE_JOIN_SECRET` | none | when `voice = sora` | yes |
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
| [`errors_exporter`](#errors_exporter) | `MY_ERRORS_EXPORTER` | `none` | no | no |
| [`errors_sentry_dsn`](#errors_sentry_dsn) | `MY_ERRORS_SENTRY_DSN` | none | when `errors_exporter` is `sentry` | yes |
| [`errors_environment`](#errors_environment) | `MY_ERRORS_ENVIRONMENT` | `production` | no | no |

## Settings

### `host`

Domain that serves the instance. Every ActivityPub `id` is under `https://{host}/`. A port is allowed for local development. Recorded in the database on first start and must not change afterwards.

- Environment variable: `MY_HOST`
- Default: none
- Required: yes

### `account_domain`

Domain of user handles (`@alice@{account_domain}`). Defaults to `host`. When it differs, `account_domain` must redirect `/.well-known/webfinger`, `/.well-known/host-meta`, and `/.well-known/nodeinfo` to `host`. Recorded in the database on first start and must not change afterwards.

- Environment variable: `MY_ACCOUNT_DOMAIN`
- Default: value of `host`
- Required: no

### `source_url`

Where users get the source code of the running server, as an `https` URL or a `mailto:` address to request it from. AGPL-3.0 section 13 requires offering the source to everyone who uses the server over the network; `GET /instance`, the information page (`/about`), the landing page, and NodeInfo (`metadata.sourceCode`, and `software.repository` for an `https` URL) publish it. Point it at your own source when you run a fork or a modified version (docs/operations/running.md).

- Environment variable: `MY_SOURCE_URL`
- Default: `https://github.com/mycroblog/my`
- Required: no

### `status_page_url`

Address of the instance's status page, an absolute `https` URL on a service apart from the server. Empty means none. `GET /instance`, the sign-in screen, and the server-rendered error pages link to it, and so does the maintenance page, which is why it is a setting rather than an instance setting in the database: the link must show while the database is out of reach.

- Environment variable: `MY_STATUS_PAGE_URL`
- Default: none
- Required: no

### `maintenance`

Starts `my serve` in the maintenance mode, also set by `my serve -maintenance`. The process then opens no adapter and reads only the settings marked for it (the listen address, the host and origins, the proxies, `status_page_url`, and the `maintenance_*` settings), so that it starts while the database, the KV, or the queue is out of reach. Whatever the `role`, it serves HTTP only and answers every request with a `503` that says the server is under maintenance; `/healthz` and `/readyz` answer `200` with `{"status":"maintenance"}`. Switch it by redeploying (docs/operations/maintenance.md).

- Environment variable: `MY_MAINTENANCE`
- Default: `false`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `false` | yes | Normal operation. |
| `true` | yes | Maintenance mode. |

### `maintenance_server_name`

Name of the server on the maintenance page and the exported maintenance pages (`my maintenance export`). The instance's own name is in the database, which the maintenance mode does not read.

- Environment variable: `MY_MAINTENANCE_SERVER_NAME`
- Default: value of `host`
- Required: no

### `maintenance_message`

Notice of the maintenance as plain text, at most 2,000 characters, shown on the maintenance page and in the `message` of the maintenance problem of the client API. The same text is shown in every locale.

- Environment variable: `MY_MAINTENANCE_MESSAGE`
- Default: none
- Required: no

### `maintenance_until`

When the maintenance is expected to end, as an RFC 3339 time such as `2026-10-24T03:00:00Z`. The maintenance page shows it in UTC, the maintenance problem has it as `until`, and while it is in the future `Retry-After` counts the seconds to it (at most 86,400).

- Environment variable: `MY_MAINTENANCE_UNTIL`
- Default: none
- Required: no

### `maintenance_retry_after`

Seconds of the `Retry-After` of the maintenance responses when `maintenance_until` is unset or past, from 1 to 86,400.

- Environment variable: `MY_MAINTENANCE_RETRY_AFTER`
- Default: `600`
- Required: no

### `web_origin`

Origin (`scheme://host[:port]`) of the web client. The client API allows only this origin by CORS. Set it when the web client is served from another origin, for example a development server at `http://localhost:5173`. An origin on `host` is where the server itself serves the web client, and the client served there calls the API at it: set `http://localhost:8080` (with `host` = `localhost:8080`) to run the server over plain HTTP on localhost, where the sign-in screens, the OAuth issuer, the client API, the links to the instance's pages in its responses, media URLs, and email and invite links are all on `http://localhost:8080`. ActivityPub ids stay under `https://{host}/` in every setup.

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

Reverse proxies and load balancers in front of the server whose forwarding header (`trusted_proxy_header`) tells the client IP, as comma-separated CIDRs or addresses such as `10.0.0.0/8, 2001:db8::/32, 192.0.2.10`. The client IP keys the rate limits of sign-in, registration, the client API, and the inbox. When the connection's peer is in this list, the client IP is the rightmost address of the header that is not in it; otherwise the header is ignored and the client IP is the peer. Empty trusts no one: behind a proxy every request then shares the proxy's limits. List only addresses that nothing but your proxies can connect from, and only proxies that append to the header rather than pass a client's value through.

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

Database adapter. The adapter is never inferred from `database_url`.

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

Job queue adapter.

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

Blob storage adapter for media.

- Environment variable: `MY_BLOB`
- Default: `fs`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `fs` | yes | Local file system. |
| `s3` | yes | S3-compatible API (AWS S3, Cloudflare R2, GCS XML API, MinIO). |
| `gcs` | not in v0 | Native Google Cloud Storage API. |

### `media`

Media processor adapter.

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

### `media_origin`

Origin (`scheme://host[:port]`, no path) at which the media files are served, for a CDN in front of the server's `/media/`. It is the origin of the URLs of the local media files (`/media/...`) and of the remote media proxy (`/media/proxy/...`) in the client API, ActivityPub and link previews, and the web client's Content-Security-Policy allows images and media from it. `https` is required; `http` is accepted only for a loopback host (`localhost`, `127.0.0.1`, `[::1]`). Unset, the media is served from the server's own origin. The URLs are built when they are read, so a change applies to the media that already exists. ActivityPub ids never use it (docs/operations/cdn.md).

- Environment variable: `MY_MEDIA_ORIGIN`
- Default: the server's own origin (`https://{host}`)
- Required: no

### `media_max_image_bytes`

Largest accepted image upload in bytes.

- Environment variable: `MY_MEDIA_MAX_IMAGE_BYTES`
- Default: `16777216`
- Required: no

### `media_max_image_side`

Largest accepted image width and height in pixels, read from the file's header before decoding.

- Environment variable: `MY_MEDIA_MAX_IMAGE_SIDE`
- Default: `8192`
- Required: no

### `media_max_image_pixels`

Largest accepted image area (width × height, and for an animated GIF the frames together), read from the file's header before decoding. The default is 33 MP (7680 × 4320).

- Environment variable: `MY_MEDIA_MAX_IMAGE_PIXELS`
- Default: `33177600`
- Required: no

### `media_unattached_ttl`

How long an upload may stay unattached to any post or profile before the hourly cleanup deletes it, as a Go duration such as `24h`.

- Environment variable: `MY_MEDIA_UNATTACHED_TTL`
- Default: `24h`
- Required: no

### `media_max_av_bytes`

Largest accepted video or audio upload in bytes. The file is held in memory while it is checked and remuxed, so this also bounds the memory of one upload.

- Environment variable: `MY_MEDIA_MAX_AV_BYTES`
- Default: `104857600`
- Required: no

### `media_max_video_duration`

Longest accepted video upload, as a Go duration such as `5m`.

- Environment variable: `MY_MEDIA_MAX_VIDEO_DURATION`
- Default: `5m`
- Required: no

### `media_max_audio_duration`

Longest accepted audio upload, as a Go duration such as `10m`.

- Environment variable: `MY_MEDIA_MAX_AUDIO_DURATION`
- Default: `10m`
- Required: no

### `custom_emoji_max_bytes`

Largest image in bytes that a local custom emoji may be registered with; the image must also be a PNG, a GIF, or a WebP of at most 1,024 pixels on a side. The default is Mastodon's limit. GoToSocial does not fetch remote emoji larger than 100 KiB by default.

- Environment variable: `MY_CUSTOM_EMOJI_MAX_BYTES`
- Default: `262144`
- Required: no

### `remote_custom_emoji_max_bytes`

Largest image in bytes of a remote custom emoji that the media proxy serves; the proxy answers 404 for a larger one, and for one that is not a PNG, a GIF, or a WebP.

- Environment variable: `MY_REMOTE_CUSTOM_EMOJI_MAX_BYTES`
- Default: `1048576`
- Required: no

### `remote_media_cache_max_bytes`

Size limit in bytes of the copies of remote media that the media proxy keeps when the instance setting `remote_media_policy` is `avatars` or `all`. The hourly cleanup deletes the expired copies, then the least recently accessed ones over this limit.

- Environment variable: `MY_REMOTE_MEDIA_CACHE_MAX_BYTES`
- Default: `1073741824`
- Required: no

### `kv`

Key-value store adapter for caches and rate limits.

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

Broker that carries the signals of the real-time updates (`GET /api/v1/streaming`) between processes. It is never inferred from the other settings. With `hub`, a stream receives only the changes made in its own process, so run one process with `role = all`; a process with another role warns at start. The other brokers reach every process of the instance, and a process connects to them only while it has open streams.

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

Key manager adapter that encrypts actor private keys.

- Environment variable: `MY_KEY_MANAGER`
- Default: `local`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `local` | yes | Uses the key-encryption key from `kek`. |
| `gcpkms` | not in v0 | Google Cloud KMS. |
| `awskms` | not in v0 | AWS KMS. |

### `kek`

Key-encryption key for actor private keys: 32 random bytes in standard base64 (for example `openssl rand -base64 32`). Required when `key_manager = local`. Back it up separately from the database; losing it loses every actor key (docs/operations/key-management.md).

- Environment variable: `MY_KEK` or `MY_KEK_FILE`
- Default: none
- Required: when `key_manager = local`

### `kek_previous`

Previous key-encryption keys, comma-separated, each in the format of `kek`. They only decrypt; new secrets are always encrypted with `kek`. Set the replaced key here while rotating `kek` (docs/operations/key-management.md).

- Environment variable: `MY_KEK_PREVIOUS` or `MY_KEK_PREVIOUS_FILE`
- Default: none
- Required: no

### `mail`

Mailer adapter for verification emails. With `none`, no email reaches users, so an unverified address does not keep a user out and sign-ups in the `open` registration mode wait for a moderator's approval instead.

- Environment variable: `MY_MAIL`
- Default: `none`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `none` | yes | Logs each message, including its links, instead of sending it; for development and instances without email. |
| `smtp` | yes | SMTP submission with mandatory STARTTLS (`smtp_host`, `smtp_port`, `smtp_username`, `smtp_password`). |
| `ses` | yes | The Amazon SES v2 API (`ses_region`, `ses_configuration_set`); credentials from the AWS SDK's default chain, such as the execution role, so no SMTP password. The build tag `no_ses` leaves it out. |

### `mail_from`

Sender address of emails, as a bare address such as `noreply@social.example`. Emails name the instance as the sender (`From: "<instance name>" <mail_from>`), take their `Message-ID` domain from it, and send replies to the instance setting `contact_email` when it is set.

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

### `ses_region`

AWS region of the SES endpoint of `mail = ses`, such as `eu-west-1`: the region where the identity of `mail_from` is verified. Empty takes the region of the AWS SDK's default chain (`AWS_REGION`).

- Environment variable: `MY_SES_REGION`
- Default: none
- Required: no

### `ses_configuration_set`

Name of the SES configuration set that `mail = ses` applies to every message, for its event destinations such as the SNS topic of the bounces and complaints (docs/operations/email.md). Empty sends without one.

- Environment variable: `MY_SES_CONFIGURATION_SET`
- Default: none
- Required: no

### `ses_sns_topic_arn`

ARN of the SNS topic that SES publishes the bounces and complaints to, `arn:aws:sns:<region>:<account>:<topic>`, with the topic attribute `SignatureVersion` set to `2` (docs/operations/email.md). Setting it opens `POST /mail/events/ses` for the topic's HTTPS subscription, which records the permanent bounces and the complaints as suppressions; empty answers `404` there.

- Environment variable: `MY_SES_SNS_TOPIC_ARN`
- Default: none
- Required: no

### `captcha`

CAPTCHA service that the sign-in and registration forms check. The service receives the visitor's IP address, browser details, and cookies, and the two pages load its script; say so in the instance's privacy policy. Only `none` keeps every request on this server.

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

Secret key of the `captcha` service, which the server sends with each token to the service's siteverify API. The server does not try the key at start: a key that the service refuses is logged as an error on the first check, and until it is fixed registrations are refused and sign-ins skip the CAPTCHA.

- Environment variable: `MY_CAPTCHA_SECRET_KEY` or `MY_CAPTCHA_SECRET_KEY_FILE`
- Default: none
- Required: when `captcha` is not `none`

### `vapid_private_key`

VAPID private key of the server's Web Push notifications (RFC 8292): a P-256 private key, 32 bytes in base64url. Generate one with `my config vapid-key`. Without it, the server sends no Web Push and clients do not offer it. Changing it ends every subscription, since browsers subscribe with its public key (docs/operations/key-management.md).

- Environment variable: `MY_VAPID_PRIVATE_KEY` or `MY_VAPID_PRIVATE_KEY_FILE`
- Default: none
- Required: no

### `profile`

Whether this is a production instance. Settings that weaken a security default for development, such as `http_allow_private_networks`, are refused in the `production` profile.

- Environment variable: `MY_PROFILE`
- Default: `production`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `production` | yes | A production instance. |
| `development` | yes | A development or test instance. |

### `http_allow_private_networks`

Lets outbound HTTP (federation, media, link previews) reach loopback, private, and other non-public addresses and host names, over plain `http` and on any port. For development and tests against local servers only: the server logs a warning at start, and the `production` profile refuses it.

- Environment variable: `MY_HTTP_ALLOW_PRIVATE_NETWORKS`
- Default: `false`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `false` | yes | Only public `https` destinations on ports 443 and 80. |
| `true` | yes | Every destination; requires `profile = development`. |

### `webhook_allowed_destinations`

Destinations on loopback or a private network that the instance's webhooks may reach, such as an automation tool on the same host or in the same VPC: comma-separated host names, IP addresses, and CIDRs such as `n8n.internal, 10.0.0.0/8`. The SSRF guard of outbound HTTP refuses them otherwise; for these destinations alone, and for the webhooks alone, it also allows plain `http` and any port. A host name is allowed whatever it resolves to; an address range allows the URLs that name an address in it and the host names that resolve into it only when they are listed too. Only the operator can change it: the administrators of the web client cannot reach an internal network on their own. Empty allows none.

- Environment variable: `MY_WEBHOOK_ALLOWED_DESTINATIONS`
- Default: none
- Required: no

### `login_provider_allowed_destinations`

Destinations on loopback or a private network that the external sign-in providers may be on, such as an OpenID Connect provider in the same VPC: comma-separated host names, IP addresses, and CIDRs such as `keycloak.internal, 10.0.0.0/8`. The SSRF guard of outbound HTTP refuses them otherwise; for these destinations alone, and for the requests to sign-in providers alone, it also allows plain `http` and any port. A host name is allowed whatever it resolves to; an address range allows the URLs that name an address in it and the host names that resolve into it only when they are listed too. Only the operator can change it: the administrators of the web client cannot reach an internal network on their own. Empty allows none.

- Environment variable: `MY_LOGIN_PROVIDER_ALLOWED_DESTINATIONS`
- Default: none
- Required: no

### `remote_status_retention`

How long a remote post is kept after its publication when nothing on this instance refers to it (a local reply, boost, favourite, bookmark, notification, or a thread with a local post); the hourly purge then deletes it. A Go duration such as `2160h` (90 days).

- Environment variable: `MY_REMOTE_STATUS_RETENTION`
- Default: `2160h`
- Required: no

### `remote_status_max_retention`

The longest a remote post is kept after its publication, even when this instance refers to it; the hourly purge then deletes it, while local accounts' replies, boosts, favourites, and bookmarks stay and show it as deleted. A Go duration such as `8760h` (365 days); empty keeps referred posts without limit.

- Environment variable: `MY_REMOTE_STATUS_MAX_RETENTION`
- Default: none
- Required: no

### `relay_status_retention`

How long a remote post that only a relay brought in is kept after its publication when nothing on this instance refers to it and no local account follows its author; the hourly purge then deletes it. A Go duration such as `168h` (7 days).

- Environment variable: `MY_RELAY_STATUS_RETENTION`
- Default: `168h`
- Required: no

### `relay_ingest_hourly_limit`

How many activities each relay may bring in per hour. The ones over it are dropped before their posts are fetched, and counted in `my.relay.activities` and on the relays screen. It bounds the fetches, rows, and jobs that a relay costs.

- Environment variable: `MY_RELAY_INGEST_HOURLY_LIMIT`
- Default: `1000`
- Required: no

### `billing_sources`

Cloud providers whose usage charges are imported into the instance's costs every 24 hours, comma-separated: `aws` (AWS Cost Explorer, about $0.01 per billed request) and `gcp` (the Cloud Billing export to BigQuery, or the Pub/Sub notifications of a budget: `billing_gcp_method`). Each import records the charges of the current billing month and of the previous one (through the 15th, or until it is final) by service and charge type, as estimates until the provider closes the month, then final, then corrections; `GET /api/v1/admin/billing_imports` shows the state of the imports. A provider whose adapter is not in this build is refused at start. Credentials come from the SDK's default chain of each provider (docs/operations/billing-import.md). The build tags `no_billing_aws` and `no_billing_gcp` leave `aws` and `gcp` out. Empty imports nothing.

- Environment variable: `MY_BILLING_SOURCES`
- Default: none
- Required: no

### `billing_aws_max_requests_per_day`

How many billed AWS Cost Explorer requests (one per page, $0.01 each) the imports of `billing_sources = aws` may make per UTC day, counting the scheduled imports, their retries, and the imports that an administrator starts. An import with none left calls nothing and is recorded as `request_cap`; the next day's import tries again.

- Environment variable: `MY_BILLING_AWS_MAX_REQUESTS_PER_DAY`
- Default: `4`
- Required: no

### `billing_aws_max_requests_per_month`

How many billed AWS Cost Explorer requests the imports may make per UTC calendar month, the bound of the cost of the imports (the default is $1.20 a month: four imports a day for thirty days). An import with none left calls nothing and is recorded as `request_cap` until the next month.

- Environment variable: `MY_BILLING_AWS_MAX_REQUESTS_PER_MONTH`
- Default: `120`
- Required: no

### `billing_aws_metric`

Cost Explorer metric that the imports of `billing_sources = aws` record.

- Environment variable: `MY_BILLING_AWS_METRIC`
- Default: `NetUnblendedCost`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `NetUnblendedCost` | yes | The cost after the discounts (Savings Plans, Reserved Instances, private pricing), the closest to the invoice. |
| `UnblendedCost` | yes | The cost before the discounts. |

### `billing_aws_filter`

Which costs of the AWS account the imports of `billing_sources = aws` take. The most accurate is an AWS account that serves this instance alone, with `none`: the taxes and the support fees carry no tag and may fall out of a tag filter (docs/operations/billing-import.md).

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

ARN of an IAM role that the imports of `billing_sources = aws` assume with the Google ID token of the instance's service account (`AssumeRoleWithWebIdentity`, audience: this ARN), for a server on Google Cloud (Cloud Run) without AWS keys. The role trusts the identity provider `accounts.google.com` for the service account (docs/operations/billing-import.md). Empty uses the AWS SDK's default chain (the execution role, `AWS_ACCESS_KEY_ID`, the shared files).

- Environment variable: `MY_BILLING_AWS_ROLE_ARN`
- Default: none
- Required: no

### `billing_gcp_method`

How the imports of `billing_sources = gcp` read the Google Cloud usage charges. The billing months are the invoice months of Cloud Billing, US Pacific calendar months; a month is final from the 5th of the next month on (Pacific time).

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

The `maximumBytesBilled` of each BigQuery query of `billing_gcp_method = bigquery`, in bytes: a query that would bill more fails as `bytes_cap` and bills nothing. The default, 1 GiB, keeps a query a day well within the free tier of 1 TiB a month, which the billing account shares with its other uses; BigQuery bills at least 10 MB a query.

- Environment variable: `MY_BILLING_GCP_MAX_BYTES_BILLED`
- Default: `1073741824`
- Required: no

### `billing_gcp_budget_subscription`

The pull subscription of the Pub/Sub topic of a monthly Cloud Billing budget that `billing_gcp_method = budget` reads, `projects/{project}/subscriptions/{subscription}`. The imports acknowledge a notification once a later one of its budget and month replaces it or once its month is final and no longer imported, and keep the latest of each month imported, so the subscription needs a message retention of at least 14 days (31 recommended) and must not be read by anything else. The service account of the server needs `roles/pubsub.subscriber` on it (docs/operations/billing-import.md).

- Environment variable: `MY_BILLING_GCP_BUDGET_SUBSCRIPTION`
- Default: none
- Required: when `billing_sources` includes `gcp` and `billing_gcp_method = budget`

### `payments_provider`

The payment provider of the supporters: local users subscribe to paid plans on its hosted checkout (PAY.JP: the server's own checkout page), and the roles of the plans follow the state of their payments. Only one provider is active on a server; switching to another one needs the subscribers to subscribe again. The provider calls `POST /payments/webhooks/{provider}` (signed or carrying the account's token, at most 1 MiB, recorded once per event), and a reconciliation reads its lists every `payments_reconcile_interval`. Card details never reach the server. An administrator still enables the supporters and sets up the plans on the admin screen.

- Environment variable: `MY_PAYMENTS_PROVIDER`
- Default: `none`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `none` | yes | No payments: the webhook endpoint and the reconciliation do not run. |
| `stripe` | yes | Stripe Billing: Checkout in subscription mode and the customer portal, with `payments_stripe_secret_key` and `payments_stripe_webhook_secret` (docs/operations/payments-stripe.md). The build tag `no_payments_stripe` leaves it out. |
| `payjp` | yes | PAY.JP on its API v1: plans and subscriptions of PAY.JP, with the server's own checkout and card pages that load payjp.js, with `payments_payjp_secret_key`, `payments_payjp_public_key`, and `payments_payjp_webhook_token` (docs/operations/payments-payjp.md). JPY only. The build tag `no_payments_payjp` leaves it out. |

### `payments_stripe_secret_key`

The API key of the Stripe account of `payments_provider = stripe`: a restricted key (`rk_test_` / `rk_live_`) with the permissions of docs/operations/payments-stripe.md, or a secret key (`sk_test_` / `sk_live_`). Its prefix sets the mode: a test key charges nothing, and the webhook endpoint refuses live events with it (and test events with a live key).

- Environment variable: `MY_PAYMENTS_STRIPE_SECRET_KEY` or `MY_PAYMENTS_STRIPE_SECRET_KEY_FILE`
- Default: none
- Required: when `payments_provider = stripe`

### `payments_stripe_webhook_secret`

The signing secret (`whsec_`) of the webhook endpoint `https://<domain>/payments/webhooks/stripe` that the operator registers on Stripe's dashboard, or of `stripe listen` for local tests. While a secret is rolled, Stripe signs with the old and the new one, so one is enough.

- Environment variable: `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` or `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET_FILE`
- Default: none
- Required: when `payments_provider = stripe`

### `payments_payjp_secret_key`

The secret key (`sk_test_` / `sk_live_`) of the PAY.JP account of `payments_provider = payjp`. Its prefix sets the mode: a test key charges nothing, and the webhook endpoint refuses live events with it (and test events with a live key). The server also signs the URLs of its checkout and card pages with a key derived from it, so a new secret key ends the pages open at the time.

- Environment variable: `MY_PAYMENTS_PAYJP_SECRET_KEY` or `MY_PAYMENTS_PAYJP_SECRET_KEY_FILE`
- Default: none
- Required: when `payments_provider = payjp`

### `payments_payjp_public_key`

The public key (`pk_test_` / `pk_live_`) of the PAY.JP account of `payments_provider = payjp`, of the mode of `payments_payjp_secret_key`, which the server's checkout and card pages give payjp.js.

- Environment variable: `MY_PAYMENTS_PAYJP_PUBLIC_KEY`
- Default: none
- Required: when `payments_provider = payjp`

### `payments_payjp_webhook_token`

The webhook token (`whook_`) of the PAY.JP account in the mode of `payments_payjp_secret_key`, which PAY.JP sends in `X-Payjp-Webhook-Token` to `https://<domain>/payments/webhooks/payjp`. Test and live mode have different tokens.

- Environment variable: `MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN` or `MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN_FILE`
- Default: none
- Required: when `payments_provider = payjp`

### `payments_reconcile_interval`

How often the reconciliation with `payments_provider` runs: it reads the provider's changes since the previous run, the checkouts in progress, the money movements, and a sweep of every subscription, and corrects what the webhooks missed. A Go duration from `15m` to `168h`; raise it, or `payments_reconcile_max_requests`, when the runs often stop at the cap.

- Environment variable: `MY_PAYMENTS_RECONCILE_INTERVAL`
- Default: `6h`
- Required: no

### `payments_reconcile_max_requests`

How many requests one reconciliation sends to `payments_provider`, a page of a list counting as one. A run that reaches it records where it stopped and a continuation picks up the rest a minute later. The provider's adapter also keeps the requests within its rate limits.

- Environment variable: `MY_PAYMENTS_RECONCILE_MAX_REQUESTS`
- Default: `1000`
- Required: no

### `voice`

How the voice chat rooms connect their participants. With `off`, the voice chat API answers 404, the web client shows no voice chat, and the other `voice_*` settings are not checked at start.

- Environment variable: `MY_VOICE`
- Default: `off`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `off` | yes | No voice chat. |
| `p2p` | yes | The browsers connect to each other, with the STUN servers of `voice_stun_urls`; the server only relays what they send to set up the connections. The server gives the browsers the TURN servers of `voice_turn` (coturn or Cloudflare) with short-lived credentials. A room holds at most `voice_p2p_max_participants` people. Without STUN and TURN, which the server warns about at start, only browsers on the same network connect. |
| `livekit` | yes | A LiveKit server (self-hosted or LiveKit Cloud) carries the audio, with `voice_livekit_url`, `voice_livekit_api_key`, and `voice_livekit_api_secret`. Its own STUN and TURN servers serve the browsers. LiveKit sends its events to `https://<domain>/voice/webhooks/livekit`. The build tag `no_voice_livekit` leaves it out. |
| `sora` | yes | A Sora server (self-hosted or Sora Cloud) carries the audio, with `voice_sora_mode`, `voice_sora_signaling_urls`, `voice_join_secret`, and the settings of the mode. Its own STUN and TURN servers serve the browsers. Sora calls `https://<domain>/voice/webhooks/sora/auth` (its auth webhook) before every connection and sends its event and session webhooks to `https://<domain>/voice/webhooks/sora/events`. The Sora transport is tested only against a stand-in that follows Sora's documents, not against a real Sora server. The build tag `no_voice_sora` leaves it out. |

### `voice_p2p_max_participants`

How many people a room of `voice = p2p` holds, from 2 to 50; one more is refused. Every speaker sends their audio to every other participant, so a larger room needs more upload bandwidth of each speaker.

- Environment variable: `MY_VOICE_P2P_MAX_PARTICIPANTS`
- Default: `10`
- Required: no

### `voice_max_participants`

How many people a room of `voice = livekit` or `voice = sora` holds; one more is refused. `0` sets no limit.

- Environment variable: `MY_VOICE_MAX_PARTICIPANTS`
- Default: `0`
- Required: no

### `voice_max_room_duration`

The longest a room stays open, a Go duration such as `4h`; a room that reaches it is ended. `0` sets no limit.

- Environment variable: `MY_VOICE_MAX_ROOM_DURATION`
- Default: `0`
- Required: no

### `voice_max_open_rooms`

How many rooms may be open on the server at the same time; opening one more is refused. `0` sets no limit.

- Environment variable: `MY_VOICE_MAX_OPEN_ROOMS`
- Default: `0`
- Required: no

### `voice_stun_urls`

The STUN servers with which the browsers of `voice = p2p` find their public addresses, comma-separated `stun:` or `stuns:` URLs such as `stun:stun.example.org:3478`. Empty uses none. `voice = livekit` and `voice = sora` do not use it and warn at start when it is set.

- Environment variable: `MY_VOICE_STUN_URLS`
- Default: none
- Required: no

### `voice_turn`

The TURN server that relays the audio of `voice = p2p` between browsers that cannot reach each other directly, with short-lived credentials for each participant. The server checks the setting at start and gives the browsers the TURN servers of coturn or Cloudflare. `voice = livekit` and `voice = sora` do not use it and warn at start when it is not `none`.

- Environment variable: `MY_VOICE_TURN`
- Default: `none`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `none` | yes | No TURN server: browsers behind strict NATs or firewalls may not connect. |
| `coturn` | yes | A coturn server at `voice_turn_urls`, with its `static-auth-secret` in `voice_turn_secret`. |
| `cloudflare` | yes | Cloudflare's TURN service, with `voice_turn_cloudflare_key_id` and `voice_turn_cloudflare_api_token`; the server asks it for the credentials of each participant. |

### `voice_turn_urls`

The URLs of the coturn server of `voice_turn = coturn`, comma-separated `turn:` or `turns:` URLs such as `turn:turn.example.org:3478?transport=udp, turns:turn.example.org:5349`.

- Environment variable: `MY_VOICE_TURN_URLS`
- Default: none
- Required: when `voice = p2p` and `voice_turn = coturn`

### `voice_turn_secret`

The `static-auth-secret` of the coturn server of `voice_turn = coturn`, which signs the short-lived credentials that the server gives the participants.

- Environment variable: `MY_VOICE_TURN_SECRET` or `MY_VOICE_TURN_SECRET_FILE`
- Default: none
- Required: when `voice = p2p` and `voice_turn = coturn`

### `voice_turn_cloudflare_key_id`

The ID of the TURN key of `voice_turn = cloudflare`, from Cloudflare's dashboard.

- Environment variable: `MY_VOICE_TURN_CLOUDFLARE_KEY_ID`
- Default: none
- Required: when `voice = p2p` and `voice_turn = cloudflare`

### `voice_turn_cloudflare_api_token`

The API token of the TURN key of `voice_turn = cloudflare`, with which the server asks Cloudflare for the credentials of each participant.

- Environment variable: `MY_VOICE_TURN_CLOUDFLARE_API_TOKEN` or `MY_VOICE_TURN_CLOUDFLARE_API_TOKEN_FILE`
- Default: none
- Required: when `voice = p2p` and `voice_turn = cloudflare`

### `voice_turn_credential_ttl`

How long the TURN credentials of a participant stay valid, a Go duration; the web client asks for new ones before they expire. With `voice_turn = cloudflare` it may be at most `48h`.

- Environment variable: `MY_VOICE_TURN_CREDENTIAL_TTL`
- Default: `6h`
- Required: no

### `voice_ice_transport_policy`

Which connections the browsers of `voice = p2p` make. `voice = livekit` and `voice = sora` do not use it and warn at start when it is `relay`.

- Environment variable: `MY_VOICE_ICE_TRANSPORT_POLICY`
- Default: `all`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `all` | yes | Direct connections where possible and the TURN server otherwise; the participants of a room may learn each other's IP addresses. |
| `relay` | yes | Only through the TURN server, which hides the participants' IP addresses from each other; needs `voice_turn`. |

### `voice_livekit_url`

The `wss:` URL of the LiveKit server of `voice = livekit` that the browsers connect to, such as `wss://livekit.example.org`; `ws:` is accepted for localhost only.

- Environment variable: `MY_VOICE_LIVEKIT_URL`
- Default: none
- Required: when `voice = livekit`

### `voice_livekit_api_url`

The `https:` URL of the API of the LiveKit server of `voice = livekit` that the server calls, when it differs from `voice_livekit_url`; `http:` is accepted for localhost only.

- Environment variable: `MY_VOICE_LIVEKIT_API_URL`
- Default: `voice_livekit_url` with `https:` (`http:` for `ws:`)
- Required: no

### `voice_livekit_api_key`

The API key of the LiveKit server of `voice = livekit`.

- Environment variable: `MY_VOICE_LIVEKIT_API_KEY`
- Default: none
- Required: when `voice = livekit`

### `voice_livekit_api_secret`

The API secret of `voice_livekit_api_key`, which signs the access tokens of the participants and verifies the webhooks of the LiveKit server.

- Environment variable: `MY_VOICE_LIVEKIT_API_SECRET` or `MY_VOICE_LIVEKIT_API_SECRET_FILE`
- Default: none
- Required: when `voice = livekit`

### `voice_sora_mode`

Where the Sora server of `voice = sora` runs.

- Environment variable: `MY_VOICE_SORA_MODE`
- Default: none
- Required: when `voice = sora`

| Value | In v0 | Description |
| --- | --- | --- |
| `self_hosted` | yes | A Sora server of the operator: its API behind a reverse proxy with authentication at `voice_sora_api_url`, and its webhooks with Basic authentication (`voice_sora_webhook_username`, `voice_sora_webhook_password`). |
| `cloud` | yes | Sora Cloud, with `voice_sora_cloud_project_id` and `voice_sora_cloud_api_key`. |

### `voice_sora_signaling_urls`

The signaling URLs of the Sora server of `voice = sora` that the browsers connect to, comma-separated `wss:` URLs; `ws:` is accepted for localhost only.

- Environment variable: `MY_VOICE_SORA_SIGNALING_URLS`
- Default: none
- Required: when `voice = sora`

### `voice_sora_api_url`

The `https:` URL of the reverse proxy in front of the API of the self-hosted Sora server, which the server calls; the API of Sora itself has no authentication and must not be reachable from outside. `http:` is accepted for localhost only.

- Environment variable: `MY_VOICE_SORA_API_URL`
- Default: none
- Required: when `voice = sora` and `voice_sora_mode = self_hosted`

### `voice_sora_api_header_name`

The name of the header with which the reverse proxy of `voice_sora_api_url` authenticates the server, such as `Authorization`; set it together with `voice_sora_api_header_value`. Empty sends none.

- Environment variable: `MY_VOICE_SORA_API_HEADER_NAME`
- Default: none
- Required: no

### `voice_sora_api_header_value`

The value of the header `voice_sora_api_header_name`, such as `Basic dXNlcjpwYXNz`.

- Environment variable: `MY_VOICE_SORA_API_HEADER_VALUE` or `MY_VOICE_SORA_API_HEADER_VALUE_FILE`
- Default: none
- Required: when `voice = sora` and `voice_sora_api_header_name` is set

### `voice_sora_webhook_username`

The Basic authentication user name that the self-hosted Sora server sends with its webhooks.

- Environment variable: `MY_VOICE_SORA_WEBHOOK_USERNAME`
- Default: none
- Required: when `voice = sora` and `voice_sora_mode = self_hosted`

### `voice_sora_webhook_password`

The Basic authentication password of `voice_sora_webhook_username`.

- Environment variable: `MY_VOICE_SORA_WEBHOOK_PASSWORD` or `MY_VOICE_SORA_WEBHOOK_PASSWORD_FILE`
- Default: none
- Required: when `voice = sora` and `voice_sora_mode = self_hosted`

### `voice_sora_cloud_project_id`

The project ID of Sora Cloud of `voice_sora_mode = cloud`.

- Environment variable: `MY_VOICE_SORA_CLOUD_PROJECT_ID`
- Default: none
- Required: when `voice = sora` and `voice_sora_mode = cloud`

### `voice_sora_cloud_api_key`

The API key of the Sora Cloud project, which signs the access tokens of the participants, authenticates the calls of its API, and verifies the signatures of its webhooks.

- Environment variable: `MY_VOICE_SORA_CLOUD_API_KEY` or `MY_VOICE_SORA_CLOUD_API_KEY_FILE`
- Default: none
- Required: when `voice = sora` and `voice_sora_mode = cloud`

### `voice_join_secret`

The secret that signs the values with which the participants of `voice = sora` join a room, which the Sora server sends back for the server to authorize each connection. A random string of at least 32 bytes, such as the output of `openssl rand -base64 32`.

- Environment variable: `MY_VOICE_JOIN_SECRET` or `MY_VOICE_JOIN_SECRET_FILE`
- Default: none
- Required: when `voice = sora`

### `dead_job_retention`

How long a dead job (a job that failed for good or used up its attempts) is kept after it failed, for an operator to inspect and requeue it; the hourly purge then deletes it. A Go duration such as `720h` (30 days).

- Environment variable: `MY_DEAD_JOB_RETENTION`
- Default: `720h`
- Required: no

### `job_attempt_retention`

How long the record of a job's run that did not succeed (a retry, or the run that made it a dead job) is kept after the run, for the queue dashboard of the admin screen; the hourly purge then deletes it. Successful runs are not recorded. A Go duration such as `720h` (30 days).

- Environment variable: `MY_JOB_ATTEMPT_RETENTION`
- Default: `720h`
- Required: no

### `dynamic_client_unredeemed_ttl`

How long a dynamically registered OAuth client (RFC 7591, `POST /oauth/register`) may wait for the exchange of its first authorization code: the hourly clean-up deletes a client that is older than this and had no code exchanged. A Go duration such as `24h`.

- Environment variable: `MY_DYNAMIC_CLIENT_UNREDEEMED_TTL`
- Default: `24h`
- Required: no

### `dynamic_client_idle_ttl`

How long a dynamically registered OAuth client without a usable token may go unused before the hourly clean-up deletes it. Until the last use of a token is recorded, the time the client's newest token was issued stands for it. A Go duration such as `2160h` (90 days).

- Environment variable: `MY_DYNAMIC_CLIENT_IDLE_TTL`
- Default: `2160h`
- Required: no

### `oauth_client_rate_limits`

Caps on the client API requests of OAuth clients, for an administrator to hold back a client that overloads the server. A comma-separated list of `client_id=requests`: the requests that all the users of the client together may make in 5 minutes, on top of the per-account limit, such as `https://app.example/client.json=3000`. A client that is not listed is not limited. Over its cap, a request is answered `429` with `Retry-After`.

- Environment variable: `MY_OAUTH_CLIENT_RATE_LIMITS`
- Default: none
- Required: no

### `metrics_exporter`

Where the service metrics go: the request, federation, delivery, job, and stream metrics, named after the OpenTelemetry semantic conventions or with the prefix `my.`. Labels never carry a remote domain, an account, a URL, or a client IP.

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

Where the distributed traces go: the spans of the HTTP requests, the job runs (each the root of a new trace linked to the request that enqueued it), the transactions and queries, the deliveries, and the outbound requests. Span attributes never carry an account, a URL or a path, a client IP, a User-Agent, an SQL text, or an error message; the outbound requests carry the destination's host name. No outbound request carries a `traceparent`, and an incoming `traceparent` is taken only from `trusted_proxies`.

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

### `errors_exporter`

Where the error reports also go: the panics, the errors behind a 5xx response or a job that used up its retries, the startup failures, and the uncaught errors of the web client, each with its stack and a fingerprint that groups its repeats. The log line of each report and the error groups in the database are kept whatever this setting.

- Environment variable: `MY_ERRORS_EXPORTER`
- Default: `none`
- Required: no

| Value | In v0 | Description |
| --- | --- | --- |
| `none` | yes | Only the log line and the error groups in the database. |
| `sentry` | yes | Also to a Sentry-compatible service at `errors_sentry_dsn`: Sentry (hosted or self-hosted), GlitchTip, or Datadog Error Tracking. |
| `otlp` | yes | Also as OpenTelemetry log records of exceptions (event name `exception`) over OTLP/HTTP (protobuf, gzip), to the endpoint of the standard environment variables `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` or `OTEL_EXPORTER_OTLP_ENDPOINT` (default `http://localhost:4318`) with `OTEL_EXPORTER_OTLP_HEADERS`. Whether the receiving backend groups them as errors depends on the backend; for Sentry, GlitchTip, and Datadog use `sentry`. The build tag `no_otel` leaves it out. |

### `errors_sentry_dsn`

DSN of the project that receives the error reports of `errors_exporter = sentry`, as the service shows it, such as `https://<key>@o0.ingest.sentry.io/<project>`.

- Environment variable: `MY_ERRORS_SENTRY_DSN` or `MY_ERRORS_SENTRY_DSN_FILE`
- Default: none
- Required: when `errors_exporter` is `sentry`

### `errors_environment`

Name of the environment that every error report carries, such as `staging`, to tell the reports of several deployments apart: up to 64 letters, digits, dots, hyphens, and underscores.

- Environment variable: `MY_ERRORS_ENVIRONMENT`
- Default: `production`
- Required: no

## Renamed and removed settings

Using one of these keys, in the config file or as its environment variable (or `<NAME>_FILE`), stops the server with an error.

None.
