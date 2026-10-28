# Build tags

The server binary `cmd/my` holds every adapter by default, and no test fake. A build tag leaves an adapter, and the client library,
driver, or cloud SDK that only it needs, out of the binary, to make it smaller and to shrink what an attacker can reach.
The adapters that need nothing beyond the standard library and the packages the rest of the server already uses
(`queue = inline`, `kv = memory`, `blob = fs`, `mail = smtp`, `metrics_exporter = emf`, and the like) have no tag.

| Tag | Leaves out | Settings it rejects |
| --- | --- | --- |
| `no_sqlite` | The SQLite database (`modernc.org/sqlite`) and the queue and key-value store kept in it | `db = sqlite` |
| `no_postgres` | The PostgreSQL database (`pgx/v5`), the queue and key-value store kept in it, and the `LISTEN` / `NOTIFY` broker | `db = postgres`, `stream = postgres` |
| `no_valkey` | The Valkey client (`valkey-go`): the key-value store and the broker on `valkey_url` | `kv = valkey`, `stream = valkey` |
| `no_sns` | The Amazon SNS / SQS broker (AWS SDK) | `stream = sns` |
| `no_ses` | The Amazon SES mailer (AWS SDK) | `mail = ses` |
| `no_pubsub` | The Google Cloud Pub/Sub broker; with `no_billing_gcp`, the Pub/Sub client library too | `stream = pubsub` |
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
| `e2e_fakes` | The fake payment provider: its state lives in the server's memory, and its own checkout page under `/payments/pages/fake/` offers to pay, to make the payment fail, or to cancel. Paying records the subscription and its payment as paid without any money moving. | `payments_provider = fake`, with `profile = development` only |

```sh
scripts/task.sh go:bin GO_TAGS=e2e_fakes
MY_PROFILE=development MY_PAYMENTS_PROVIDER=fake bin/my serve
```

A fake that records payments as paid would give anyone a supporter's role without paying, so four checks keep it out
of production; each one stops it on its own when another one is bypassed:

- **At start.** A binary without the tag rejects `payments_provider = fake` like a left-out adapter:

  ```text
  payments_provider (MY_PAYMENTS_PROVIDER): "fake" is only in test builds: the build tag e2e_fakes adds its adapter, and released binaries never have it (docs/reference/build-tags.md)
  ```

  A test build refuses `payments_provider = fake` unless `profile = development`, and logs a warning at start that it
  is a test build.
- **The build check.** `scripts/task.sh go:build-tags` fails when `cmd/my` without the tag (with no tags, or with any
  representative set of `no_*` tags) depends on the fake. `internal/platform/adapters_payments_fake.go`, which has the
  constraint `//go:build e2e_fakes`, is the only file outside the tests that imports it.
- **The container image.** The `Dockerfile` stops when `GO_TAGS` has `e2e_fakes`, before it compiles anything, and
  checks the binary it built as the release check does.
- **The release.** `scripts/check-release-build.sh BINARY...` fails when the Go build info of a binary
  (`go version -m`, its `build -tags=` line) has `e2e_fakes`, or cannot be read. Every job that publishes a binary or an
  image runs it on what it publishes.

## Building

Pass the tags, separated by commas, to the Go build, to `scripts/task.sh go:bin`, or to the container build:

```sh
CGO_ENABLED=0 go build -tags no_postgres,no_valkey,no_sns,no_ses,no_pubsub,no_otel,no_billing_aws,no_billing_gcp,no_payments_stripe ./cmd/my
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
| `no_postgres,no_valkey,no_sns,no_ses,no_pubsub,no_otel,no_billing_aws,no_billing_gcp,no_payments_stripe` | A self-hosted instance on SQLite |
| `no_sqlite,no_valkey,no_pubsub,no_otel,no_billing_gcp,no_payments_payjp,no_voice_sora` | AWS: PostgreSQL, the SNS broker, SES, the EMF metrics, and the Stripe payments, without the Sora voice chat |
| `no_sqlite,no_valkey,no_sns,no_ses,no_billing_aws,no_voice_livekit` | Google Cloud: PostgreSQL and the Pub/Sub broker, without the LiveKit voice chat |
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
