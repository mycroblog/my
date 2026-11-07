# Build tags

The server binary `cmd/my` holds every adapter by default, and no test fake. A build tag leaves an adapter, and the client library,
driver, or cloud SDK that only it needs, out of the binary, to make it smaller and to shrink what an attacker can reach.
The adapters that need nothing beyond the standard library and the packages the rest of the server already uses
(`queue = inline`, `kv = memory`, `blob = fs`, `mail = smtp`, `metrics_exporter = emf`, and the like) have no tag.

| Tag | Leaves out | Settings it rejects |
| --- | --- | --- |
| `no_sqlite` | The SQLite database (`modernc.org/sqlite`), the queue and key-value store kept in it, and `my schema apply` for SQLite (Atlas's SQLite driver) | `database = sqlite` |
| `no_postgres` | The PostgreSQL database (`pgx/v5`), the queue and key-value store kept in it, the `LISTEN` / `NOTIFY` broker, and `my schema apply` for PostgreSQL (Atlas's PostgreSQL driver) | `database = postgres`, `stream = postgres` |
| `no_valkey` | The Valkey client (`valkey-go`): the key-value store and the broker on `valkey_url` | `kv = valkey`, `stream = valkey` |
| `no_sns` | The Amazon SNS / SQS broker (AWS SDK) | `stream = sns` |
| `no_ses` | The Amazon SES mailer (AWS SDK) | `mail = ses` |
| `no_s3` | The S3-compatible blob store (AWS SDK for Amazon S3) | `blob = s3` |
| `no_gcs` | The Google Cloud Storage blob store (Cloud Storage client library) | `blob = gcs` |
| `no_pubsub` | The Google Cloud Pub/Sub broker; with `no_billing_gcp`, the Pub/Sub client library too | `stream = pubsub` |
| `no_gcpkms` | The Google Cloud KMS key manager (Cloud KMS client library) | `key_manager = gcpkms`, and Cloud KMS keys in `kms_key` / `kms_key_previous` |
| `no_awskms` | The AWS KMS key manager (AWS SDK for AWS KMS) | `key_manager = awskms`, and AWS KMS keys in `kms_key` / `kms_key_previous` |
| `no_cloudtasks` | The Google Cloud Tasks queue (Cloud Tasks client library) and its `/_internal/jobs` and `/_internal/tick` endpoints | `queue = cloudtasks` |
| `no_billing_aws` | The import of the AWS usage charges (AWS Cost Explorer SDK) | `billing_sources = aws` |
| `no_billing_gcp` | The import of the Google Cloud usage charges (BigQuery REST client; with `no_pubsub`, the Pub/Sub client library of `billing_gcp_method = budget`) | `billing_sources = gcp` |
| `no_payments_stripe` | The Stripe adapter of the supporters' payments (`stripe-go`) | `payments = stripe` |
| `no_payments_payjp` | The PAY.JP adapter of the supporters' payments and its checkout and card pages (no third-party library) | `payments = payjp` |
| `no_voice_livekit` | The LiveKit adapter of the voice chat and the endpoint of its events (no third-party library) | `voice = livekit` |
| `no_voice_sora` | The Sora adapter of the voice chat and the endpoints of its auth, event, and session webhooks (no third-party library) | `voice = sora` |
| `no_plugins` | The plugin client (`connect/v2`), the adapters of the plugins, and `my plugin test` | Every `plugin_<kind>_address`, `billing_sources = plugin`, `mail = plugin`, `key_manager = plugin`, `media_image = plugin`, `media_video = plugin`, `media_audio = plugin` |
| `no_translator_aws` | The Amazon Translate provider of the translation of posts (AWS SDK for Amazon Translate) | `translator_aws_region` |
| `no_otel` | The OpenTelemetry SDK and its Prometheus and OTLP exporters, of the metrics, the traces, and the error reports | `metrics_exporter = prometheus`, `metrics_exporter = otlp`, `traces_exporter = otlp`, `errors_exporter = otlp` |

Leave out the database that the instance does not use, but keep at least one of `no_sqlite` and `no_postgres` off:
a binary without a database adapter cannot start.

## Test builds

One tag works the other way round: it adds fakes for the end-to-end tests and the local demonstration, and no
released binary or image has it. Never run a test build for real users.

| Tag | Adds | Settings it allows |
| --- | --- | --- |
| `e2e_fakes` | The fake payment provider: its state lives in the server's memory, and its own checkout page under `/payments/pages/fake/` offers to pay, to make the payment fail, or to cancel. Paying records the subscription and its payment as paid without any money moving. The fake search index, whose documents live in the server's memory. The command `my dev push-service`, a fake Web Push service ([contributing guide](../../CONTRIBUTING.md#the-fake-push-service)). | `payments = fake` and `search = fake`, with `profile = development` only |

A fake that records payments as paid would give anyone a supporter's role without paying, so a released binary refuses them.
A binary without the tag rejects `payments = fake` and `search = fake` like a left-out adapter:

```text
payments (MY_PAYMENTS): "fake" is only in test builds: the build tag e2e_fakes adds its adapter, and released binaries never have it (docs/reference/build-tags.md)
```

A binary without the tag has no `my dev` command (`my: unknown command "dev"`). A test build refuses
`payments = fake`, `search = fake`, and `my dev push-service` unless `profile = development`, and both log a warning at start
that it is a test build.

The tag's other safeguards, and how the project's own tests use it, are in the
[contributing guide](../../CONTRIBUTING.md#build-tags).

## Building

Pass the tags, separated by commas, to the Go build or to the container build. A plain `go build` embeds no web client
([Web client](../operations/running.md#web-client)); the container build includes it.

```sh
CGO_ENABLED=0 go build -tags no_postgres,no_valkey,no_sns,no_ses,no_pubsub,no_gcpkms,no_awskms,no_cloudtasks,no_otel,no_billing_aws,no_billing_gcp,no_payments_stripe,no_translator_aws ./cmd/my
docker build --build-arg GO_TAGS=no_sqlite,no_valkey,no_sns -t my .
```

The binary validates the configuration at start ([configuration reference](configuration.md)), and `my config check` and
`my serve` reject a setting whose adapter the build left out, naming the tag and linking this page on the
documentation site for the version of the binary (`/next/` for a build that is not a release):

```text
database (MY_DATABASE): "postgres" is not in this build: the build tag no_postgres left its adapter out (https://mycroblog.dev/v0.1/reference/build-tags/)
```

## Sets of tags that are checked

Before each release, each of these sets is built and vetted, and the check fails when `cmd/my` still depends on a package that a tag of the set leaves out:

| Set | Fits |
| --- | --- |
| `no_postgres,no_valkey,no_sns,no_ses,no_s3,no_gcs,no_pubsub,no_gcpkms,no_awskms,no_cloudtasks,no_otel,no_billing_aws,no_billing_gcp,no_payments_stripe,no_plugins,no_translator_aws` | A self-hosted instance on SQLite |
| `no_sqlite,no_valkey,no_gcs,no_pubsub,no_gcpkms,no_cloudtasks,no_otel,no_billing_gcp,no_payments_payjp,no_voice_sora` | AWS: PostgreSQL, the SNS broker, SES, the EMF metrics, the AWS KMS key manager, and the Stripe payments, without the Sora voice chat |
| `no_sqlite,no_valkey,no_sns,no_ses,no_awskms,no_billing_aws,no_voice_livekit,no_translator_aws` | Google Cloud: PostgreSQL, the Pub/Sub broker, the Cloud KMS key manager, and the Cloud Tasks queue, without the LiveKit voice chat or Amazon Translate |
