# Build tags

The server binary `cmd/my` holds every adapter by default, and no test fake. A build tag leaves an adapter, and the client library,
driver, or cloud SDK that only it needs, out of the binary, to make it smaller and to shrink what an attacker can reach.
The adapters that need nothing beyond the standard library and the packages the rest of the server already uses
(`queue = inline`, `kv = memory`, `blob = fs`, `mail = smtp`, `metrics_exporter = emf`, and the like) have no tag.

| Tag | Leaves out | Settings it rejects |
| --- | --- | --- |
| `no_sqlite` | The SQLite database (`modernc.org/sqlite`), the queue and key-value store kept in it, and `my schema apply` for SQLite (Atlas's SQLite driver) | `db = sqlite` |
| `no_postgres` | The PostgreSQL database (`pgx/v5`), the queue and key-value store kept in it, the `LISTEN` / `NOTIFY` broker, and `my schema apply` for PostgreSQL (Atlas's PostgreSQL driver) | `db = postgres`, `stream = postgres` |
| `no_valkey` | The Valkey client (`valkey-go`): the key-value store and the broker on `valkey_url` | `kv = valkey`, `stream = valkey` |
| `no_sns` | The Amazon SNS / SQS broker (AWS SDK); with `no_sqs`, the AWS SDK for Amazon SQS too | `stream = sns` |
| `no_ses` | The Amazon SES mailer (AWS SDK) | `mail = ses` |
| `no_s3` | The S3-compatible blob store (AWS SDK for Amazon S3) | `blob = s3` |
| `no_gcs` | The Google Cloud Storage blob store (Cloud Storage client library) | `blob = gcs` |
| `no_pubsub` | The Google Cloud Pub/Sub broker; with `no_billing_gcp`, the Pub/Sub client library too | `stream = pubsub` |
| `no_gcpkms` | The Google Cloud KMS key manager (Cloud KMS client library) | `key_manager = gcpkms`, and Cloud KMS keys in `kms_key` / `kms_key_previous` |
| `no_awskms` | The AWS KMS key manager (AWS SDK for AWS KMS) | `key_manager = awskms`, and AWS KMS keys in `kms_key` / `kms_key_previous` |
| `no_cloudtasks` | The Google Cloud Tasks queue (Cloud Tasks client library) and its `/_internal/jobs` and `/_internal/tick` endpoints | `queue = cloudtasks` |
| `no_sqs` | The Amazon SQS queue; with `no_sns`, the AWS SDK for Amazon SQS too | `queue = sqs` |
| `no_billing_aws` | The import of the AWS usage charges (AWS Cost Explorer SDK) | `billing_sources = aws` |
| `no_billing_gcp` | The import of the Google Cloud usage charges (BigQuery REST client; with `no_pubsub`, the Pub/Sub client library of `billing_gcp_method = budget`) | `billing_sources = gcp` |
| `no_payments_stripe` | The Stripe adapter of the supporters' payments (`stripe-go`) | `payments_provider = stripe` |
| `no_payments_payjp` | The PAY.JP adapter of the supporters' payments and its checkout and card pages (no third-party library) | `payments_provider = payjp` |
| `no_voice_livekit` | The LiveKit adapter of the voice chat and the endpoint of its events (no third-party library) | `voice = livekit` |
| `no_voice_sora` | The Sora adapter of the voice chat and the endpoints of its auth, event, and session webhooks (no third-party library) | `voice = sora` |
| `no_otel` | The OpenTelemetry SDK and its Prometheus and OTLP exporters, of the metrics, the traces, and the error reports | `metrics_exporter = prometheus`, `metrics_exporter = otlp`, `traces_exporter = otlp`, `errors_exporter = otlp` |

Leave out the database that the instance does not use, but keep at least one of `no_sqlite` and `no_postgres` off:
a binary without a database adapter cannot start.

## Test builds

One tag works the other way round: it adds fakes for the end-to-end tests and the local demonstration, and no
released binary or image has it. Never run a test build for real users.

| Tag | Adds | Settings it allows |
| --- | --- | --- |
| `e2e_fakes` | The fake payment provider: its state lives in the server's memory, and its own checkout page under `/payments/pages/fake/` offers to pay, to make the payment fail, or to cancel. Paying records the subscription and its payment as paid without any money moving. The command `my dev push-service`, a fake Web Push service ([below](#the-fake-push-service)). | `payments_provider = fake`, with `profile = development` only |

```sh
scripts/task.sh go:bin GO_TAGS=e2e_fakes
MY_PROFILE=development MY_PAYMENTS_PROVIDER=fake bin/my serve
```

A fake that records payments as paid would give anyone a supporter's role without paying, so four checks keep the
fakes out of production; each one stops them on its own when another one is bypassed:

- **At start.** A binary without the tag rejects `payments_provider = fake` like a left-out adapter:

  ```text
  payments_provider (MY_PAYMENTS_PROVIDER): "fake" is only in test builds: the build tag e2e_fakes adds its adapter, and released binaries never have it (docs/reference/build-tags.md)
  ```

  A binary without the tag has no `my dev` command (`my: unknown command "dev"`). A test build refuses
  `payments_provider = fake` and `my dev push-service` unless `profile = development`, and both log a warning at start
  that it is a test build.
- **The build check.** `scripts/task.sh go:build-tags` fails when `cmd/my` without the tag (with no tags, or with any
  representative set of `no_*` tags) depends on a fake. Only files with the constraint `//go:build e2e_fakes` import
  them outside the tests: `internal/platform/adapters_payments_fake.go` the fake payment provider, and
  `cmd/my/dev_push_service.go` the fake push service.
- **The container image.** The `Dockerfile` stops when `GO_TAGS` has `e2e_fakes`, before it compiles anything, and
  checks the binary it built as the release check does.
- **The release.** `scripts/check-release-build.sh BINARY...` fails when the Go build info of a binary
  (`go version -m`, its `build -tags=` line) has `e2e_fakes`, or cannot be read. Every job that publishes a binary or an
  image runs it on what it publishes.

### The fake push service

A browser's Web Push subscription points at its vendor's push service, which the browsers of the end-to-end tests
cannot reach. `my dev push-service` stands in for it: it serves HTTPS on this machine, issues subscriptions, and receives
the server's push messages the way a push service does. It verifies each message's VAPID authorization, decrypts its
`aes128gcm` body with the keys it issued, and keeps what it received in memory for the tests to read.

```sh
scripts/mailpit-cert.sh   # the development CA and .tmp/mailpit/push-service.pem
MY_PROFILE=development bin/my dev push-service   # https://localhost:8930
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-listen` | `127.0.0.1:8930` | The address to listen on |
| `-origin` | `https://localhost:<port of -listen>` | The origin of the endpoints, which the VAPID token's `aud` must name |
| `-cert`, `-key` | `.tmp/mailpit/push-service.pem`, `.tmp/mailpit/push-service.key` | The TLS certificate and its key, for `localhost` and `127.0.0.1`, signed by the development CA |
| `-config` | `$MY_CONFIG` | The configuration file; it reads only `profile`, which must be `development` |

`my serve` sends to it only when it trusts the development CA (`SSL_CERT_FILE=.tmp/mailpit/ca.pem`, as for the
development Mailpit in [Email](../operations/email.md#development)) and its SSRF guard lets the Web Push sends reach
`localhost` ([`push_allowed_destinations`](configuration.md#push_allowed_destinations) set to `localhost`, with
`profile = development`). Its HTTP API, in JSON:

| Request | Answer |
| --- | --- |
| `POST /subscriptions` with `{"applicationServerKey": "<base64url>"}` | `201` and a new subscription in the shape of the browser's `PushSubscription.toJSON()`: `endpoint` and `keys` (`p256dh`, `auth`) |
| `POST /push/<id>`, a push message to the endpoint | `201` with `Location` when it is accepted; `401` without a `vapid` `Authorization`; `403` when the token's signature, its `k` (other than the subscription's `applicationServerKey`), its `aud` (other than the origin), its `exp` (past, or more than 24 hours ahead), or its `sub` (missing) fails; `400` without `TTL`, with a `Content-Encoding` other than `aes128gcm`, a bad `Urgency` or `Topic`, or a body that does not decrypt; `413` over 4096 bytes; `404` for an unknown subscription; `410` once it has expired |
| `POST /subscriptions/<id>/expire` | `204`; later push messages to it answer `410` |
| `GET /subscriptions/<id>/messages` | `200` and `{"messages": [...]}`: every push message to it, oldest first, with `status`, `error` (why it was refused), `plaintext`, `ttl`, `urgency`, `topic`, `subject`, and `received_at` |

## Building

Pass the tags, separated by commas, to the Go build, to `scripts/task.sh go:bin`, or to the container build:

```sh
CGO_ENABLED=0 go build -tags no_postgres,no_valkey,no_sns,no_ses,no_pubsub,no_gcpkms,no_awskms,no_cloudtasks,no_sqs,no_otel,no_billing_aws,no_billing_gcp,no_payments_stripe ./cmd/my
scripts/task.sh go:bin GO_TAGS=no_sqlite,no_valkey,no_pubsub
scripts/task.sh docker:build GO_TAGS=no_sqlite,no_valkey,no_sns  # docker build --build-arg GO_TAGS=...
```

The binary validates the configuration at start ([configuration reference](configuration.md)), and `my config check` and
`my serve` reject a setting whose adapter the build left out, naming the tag:

```text
db (MY_DB): "postgres" is not in this build: the build tag no_postgres left its adapter out (docs/reference/build-tags.md)
```

## Checks

`scripts/task.sh go:build-tags` (part of `scripts/task.sh go:check` and, in CI, of the slow suite that gates releases)
runs, for each of these representative sets of tags, `CGO_ENABLED=0 go build` and `go vet` on every package,
and fails when `cmd/my` still depends on a package that a tag of the set leaves out:

| Set | Fits |
| --- | --- |
| `no_postgres,no_valkey,no_sns,no_ses,no_s3,no_gcs,no_pubsub,no_gcpkms,no_awskms,no_cloudtasks,no_sqs,no_otel,no_billing_aws,no_billing_gcp,no_payments_stripe` | A self-hosted instance on SQLite |
| `no_sqlite,no_valkey,no_gcs,no_pubsub,no_gcpkms,no_cloudtasks,no_otel,no_billing_gcp,no_payments_payjp,no_voice_sora` | AWS: PostgreSQL, the SNS broker, SES, the EMF metrics, the AWS KMS key manager, the SQS queue, and the Stripe payments, without the Sora voice chat |
| `no_sqlite,no_valkey,no_sns,no_ses,no_awskms,no_sqs,no_billing_aws,no_voice_livekit` | Google Cloud: PostgreSQL, the Pub/Sub broker, the Cloud KMS key manager, and the Cloud Tasks queue, without the LiveKit voice chat |
| `e2e_fakes` | The test build; it also runs the tests that only this build has (named `*E2EFakes*`) |

It also fails when `cmd/my` without `e2e_fakes`, the build with no tags among them, depends on a fake, or when the set
with `e2e_fakes` does not link the fakes.
`scripts/go-build-tags.sh SET...` checks other sets. The tests (`scripts/task.sh go:test`) run without tags, on every adapter.

## Adding a tag

An adapter that brings a dependency of its own gets the tag `no_<name>`:

- The platform wires it in `internal/platform/adapters_<name>.go` with the constraint `//go:build !no_<name>`, and
  `internal/platform/adapters_no_<name>.go` (`//go:build no_<name>`) puts in its place a function of the same signature
  that returns `config.ExcludedError`. Nothing else in `cmd/` and `internal/platform` imports the adapter.
- `internal/platform/config/adapters_no_<name>.go` calls `exclude` for the settings that select it, so that `Load`
  rejects them; a check of a setting that only this adapter reads goes in `adapters_<name>.go` likewise.
- An adapter package shared by several dialects (such as `internal/adapter/kv/db`) puts the code of each dialect,
  its constructor, and its tests in files with the dialect's constraint.
- Add the tag to the table above, to the choice documentation in `internal/platform/config/schema.go`
  (`scripts/task.sh go:config-docs`), and to `excluded_packages` in `scripts/go-build-tags.sh`, and use it in one of the representative sets.
