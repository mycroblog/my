# Contributing

Human contributors and coding agents work by the same rules. [AGENTS.md](AGENTS.md) lists them in full, and the
decisions behind them are the Architecture Decision Records indexed in [docs/adr/README.md](docs/adr/README.md).

Everyone who takes part in the project follows the
[code of conduct](https://github.com/mycroblog/.github/blob/main/CODE_OF_CONDUCT.md) of the mycroblog organization.
Report a violation to [ops@mycroblog.dev](mailto:ops@mycroblog.dev), in English or Japanese.

Do not report security vulnerabilities in issues or pull requests; follow [SECURITY.md](SECURITY.md) instead.

## Setup

Tool versions are pinned in `.tool-versions` (use [asdf](https://asdf-vm.com/) or [mise](https://mise.jdx.dev/)).

```sh
asdf install               # or: mise install
scripts/task.sh web:install  # the npm packages of the Bun workspace at the root (bun.lock)
scripts/task.sh check
```

Tasks run with [go-task](https://taskfile.dev/) and live in `Taskfile.yml` and `tasks/<area>.yml`. `scripts/task.sh`
builds the version pinned in `tools/go.mod` into `bin/task` and runs it; never use a `task` from your PATH.
`scripts/task.sh --list` lists and describes the tasks.

`scripts/task.sh check` runs the check of every context in parallel: docs, Go, i18n, the API, the Bun workspace,
Docker, agentkit, `pkg/mykit` with `pkg/mybot`, and the documentation site (`docs:check`, `go:check`, `i18n:check`,
`api:check`, `web:check`, `docker:check`, `agentkit:check`, `mykit:check`, `site:check`).
`CHECKS="go web"` runs some of them only. `TASK_CONCURRENCY` limits the parallelism; locally `scripts/task.sh` defaults it to half the CPUs.
`scripts/task.sh check:changed` (optional locally) runs only the contexts you changed and those that depend on them (compared with
`BASE=origin/main`); CI decides the same way (`scripts/changed-contexts.sh`) which jobs to skip, and the required
check `check` collects the results of the jobs ([CI](#ci)).

The checks need no network once the Go modules and tools and the npm packages of the Bun workspace have been
downloaded the first time. They cover Markdown lint and the documentation rules, the ADR index, Go
(`go vet`, golangci-lint, tests, the `CGO_ENABLED=0` build, dependency licenses, the sqlc output), the i18n catalogs,
the client API spec and the code generated from it, the Go image of the `Dockerfile` against `.tool-versions`, the
formatting and dependency licenses of the Bun workspace, and the documentation site (build, type checks, lint).

- The Go tools (go-licenses, sqlc, oapi-codegen, protoc-gen-go, protoc-gen-connect-go) are pinned by the `tool` directives
  of `tools/go.mod`, and buf by that of `tools/buf/go.mod`, since its requirements would raise the product's protobuf.
- Dependabot Alerts on GitHub report known vulnerabilities in dependencies, and Dependabot opens the version update
  pull requests (`.github/dependabot.yml`). `go.work` at the root exists for its Go updates alone: tasks run with
  `GOWORK=off`, and a bare `go` command in `pkg/mykit`, `pkg/mybot`, or `tools` needs it too.
- The Docker Compose smoke test (building the image, applying the schema, creating an administrator, checking
  WebFinger, the actor, and NodeInfo) needs Docker, so `scripts/task.sh check` leaves it out; run it with
  `scripts/task.sh docker:smoke`. CI runs it on pull requests that change the Docker setup, `cmd/`, or
  `internal/platform/`, and nightly; it builds the linux/arm64 image on pull requests that change the Docker setup and
  nightly.
- The live interoperability tests against Mastodon, GoToSocial, Misskey, and Akkoma are not part of `scripts/task.sh check`
  either: run them with `scripts/task.sh docker:federation-live` (CI runs them nightly; see
  [test/federation-live/README.md](test/federation-live/README.md)).
- The license is AGPL-3.0-only, except for `api/spec/`, `api/openapi.yaml`, `pkg/mykit/`, and `pkg/mybot/`, which are
  MIT. Their files start with `SPDX-License-Identifier: MIT`, every other new source file with `AGPL-3.0-only`, and
  `scripts/task.sh api:license-scope` (part of `api:check`) checks them. The MIT parts depend only on permissive
  licenses such as MIT, Apache-2.0, BSD, and ISC, never on GPL-family ones.
- Every other dependency must have a license compatible with AGPL-3.0 (`GO_ALLOWED_LICENSES` in `Taskfile.yml`; the npm
  packages of the Bun workspace are checked against the same list). Explain in the pull request
  why you need to add one.

### Tests that listen on ports

The Go tests start fake servers (pstest, `httptest`, and others) on `127.0.0.1:0` and let the OS pick the port; the
connections to pgmem go through a Unix socket rather than TCP (`pkg/testkit/pgtest`). When you write tests, keep the
number of connections to fake servers down: reuse clients, and pass `option.WithGRPCConnectionPool(1)` to the Google
Cloud gRPC clients (they open up to four connections by default).

A test failing with `listen tcp 127.0.0.1:0: bind: address already in use` means the connections of tests run in
parallel on the same machine (TIME_WAIT keeps them for 60 seconds) used up the ephemeral port range
(`/proc/sys/net/ipv4/ip_local_port_range`). With WSL2's mirrored networking the range is only 4,096 ports
(44620–48715), shared with Windows, and several `check` runs at once use it up. Lower `TASK_CONCURRENCY` or run fewer
`check`s at the same time. Do not widen the range under mirrored networking: when the OS picks a port outside the
range WSL gave Linux, outgoing connections (`git`, `gh`, ...) time out.

## Local development

`docker compose up` in the checkout runs the server from your working tree and everything it talks to, with Docker
alone (the root [`compose.yaml`](compose.yaml), the project `my-dev`, which never collides with a deployment; what
deploys an instance is in `deploy/docker/`, see [Docker Compose files](docs/operations/running.md#docker-compose-files)):

```sh
scripts/task.sh docker:dev-cert   # once: the HTTPS certificate of my.localhost (needs mkcert, see below)
docker compose up -d --wait       # returns once https://my.localhost works; the first start builds the development
                                  # image and downloads the Go modules, npm packages, and translation models (minutes)
docker compose logs -f my         # follow the server's log
```

Open `https://my.localhost`. Browsers resolve `my.localhost` and every other name under `.localhost` to this machine,
so no hosts file entry is needed. The server runs with `profile = development` and uses every service of the file:

| Service | What the server uses it for | On this machine |
| --- | --- | --- |
| `caddy` | The way in: the server over HTTPS, and `http://my.localhost` redirected to HTTPS; it starts once the server is healthy | `:443` (`MY_DEV_HTTPS_PORT`), `:80` (`MY_DEV_HTTP_PORT`) |
| `my` | `my serve` built from the working tree | `https://my.localhost`, through Caddy only |
| `postgres` | The database (`database = postgres`) | `localhost:5432`, user and password `my` (`PG_PORT`) |
| `mailpit` | Every email (`mail = smtp`); see [Emails](#emails) | `http://127.0.0.1:8025` (`MAILPIT_UI_PORT`), SMTP on `:1025` |
| `garage` | The media (`blob = s3`), in the bucket `my-media` | S3 on `127.0.0.1:3900` (`GARAGE_S3_PORT`) |
| `meilisearch` | The full-text search of posts (`search = meilisearch`) | `http://127.0.0.1:7700` (`MEILISEARCH_PORT`) |
| `libretranslate` | The translation provider `libretranslate`, English and Japanese | `http://127.0.0.1:5000` (`LIBRETRANSLATE_PORT`) |
| `otel-collector`, `jaeger` | The traces of every request and job (`traces_exporter = otlp`) | Jaeger on `http://127.0.0.1:16686` (`JAEGER_UI_PORT`) |
| `prometheus` | Scrapes the metrics (`metrics_exporter = prometheus`) and evaluates the [alerting rules](docs/operations/alerts.md) | `http://127.0.0.1:9090` (`PROMETHEUS_PORT`) |
| `grafana` | The [dashboard](docs/operations/alerts.md#grafana-dashboard), without signing in | `http://127.0.0.1:3000` (`GRAFANA_PORT`) |

- `scripts/task.sh docker:dev-cert` (`scripts/dev-cert.sh`) makes one certificate for `my.localhost` and
  `*.my.localhost` in `.tmp/dev-certs` with [mkcert](https://github.com/FiloSottile/mkcert), whose `mkcert -install`
  makes the browsers on this machine trust its local CA. Under WSL it runs `mkcert.exe`, the Windows build
  (`winget install FiloSottile.mkcert`), so that the browsers on Windows trust it; on Linux and macOS it runs `mkcert`
  (`brew install mkcert`, or your distribution's package). Without the certificate, `docker compose up` stops and
  says so. It is valid for about two years; the task makes a new one when it expires within 30 days.
- Ports 80 and 443 of `127.0.0.1` must be free. When one is taken, move it with `MY_DEV_HTTP_PORT` or
  `MY_DEV_HTTPS_PORT` and set `MY_DEV_HOST` to the host with that port (`MY_DEV_HOST=my.localhost:8443`).
- The server's host is recorded in its database, so a database made with another host (an earlier
  `MY_DEV_HOST`, say) needs `docker compose down -v` before the server starts again.
- The database starts empty, with the schema applied. Create an administrator with
  `docker compose exec my my admin create --email you@example.com --username you --password-link`, which prints a
  link to set the password, or register on the sign-up page with the confirmation email in Mailpit.
- Translation needs the provider enabled: turn LibreTranslate on in the translation settings of the administration, or
  run `docker compose exec my my instance set translation_providers libretranslate`. Its first start downloads the
  models, and translations fail until it answers.
- The Go code is not reloaded: `docker compose restart my` builds it again and restarts the server.
- `GO_TAGS` builds the server with [build tags](docs/reference/build-tags.md), such as `GO_TAGS=e2e_fakes` for the
  test build with the fake push service, payment provider, and search index.
- Put the variables of the table and `GO_TAGS` in a `.env` file next to `compose.yaml`; it is never committed. A port
  that another program uses must move there.
- `my` runs as the owner of the checkout in the stage `dev` of the `Dockerfile` (`scripts/dev-compose.sh`), so
  the files it writes in it are yours. The Go caches and every `node_modules` are volumes of their own, apart from
  those of your machine.
- `docker compose down` stops everything and keeps the data in the volumes; `docker compose down -v` removes it too.
- The secrets in `compose.yaml` (the KEK, the keys of Garage and Meilisearch) are fixed values for development only.

The tests do not use these services: they start what they need themselves, and `scripts/task.sh db:up` and
`scripts/task.sh docker:mailpit` start the services `postgres` and `mailpit` alone.

`scripts/task.sh docs:site-dev` serves the documentation site (`web/docs`) on `http://localhost:4321`, and
`scripts/task.sh mail:preview` writes every email to `.tmp/mail/preview/` ([Emails](#emails)).

## Workflow

1. Pick a GitHub issue that no open issue blocks; an issue of type ADR waits for the maintainer's decision.
2. Create a branch `<type>/<issue>-<slug>`. Never push to `main`.
3. Commit (Conventional Commits, in English), one commit per issue. Before a push, run the fast checks of what you
   changed (`scripts/agentkit.sh check`); the merge queue's full check is the gate, so the full check is for
   reproducing a CI failure.
4. Open a pull request from the template (`scripts/agentkit.sh pr open`). Its commits contain no merge commit: bring
   the branch up to date with `git rebase origin/main`.
5. Add it to the merge queue with `gh pr merge <PR> -R mycroblog/my --auto --merge` (`scripts/agentkit.sh pr land`
   also follows it to the merge and cleans up). `main` only receives merges from the merge queue.
6. If the queue drops the pull request because of a conflict or a failing `check`, fix it while the rest of the queue
   proceeds: rebase onto `main`, regenerate the committed generated files (`scripts/agentkit.sh regen`) instead of
   resolving their conflicts by hand, push with `--force-with-lease`, and queue it again.

## Formatting

Go is formatted with `gofmt` (`scripts/task.sh go:fmt-check` checks it). The TypeScript and JavaScript of `web/`,
`pkg/mykit/ts`, `pkg/mybot/ts`, and `tools/i18n` are formatted with `bun fmt`, the formatter built into
the pinned Bun: run `scripts/task.sh web:fmt` before you commit them, and again after resolving a conflict in one of
them. `scripts/task.sh web:fmt-check` fails when a file needs formatting; `check` and CI run it. Its options and its
files are in the `[fmt]` section of the root `bunfig.toml`, so a bare `bun fmt` formats the same files. To format from
an editor, have it run `bun fmt --stdin-filepath <file>`.

Commits that only reformat are listed in `.git-blame-ignore-revs`. GitHub's blame view skips them; for `git blame`,
run once:

```sh
git config blame.ignoreRevsFile .git-blame-ignore-revs
```

## CI

A pull request runs only what its own files decide (formatting, licenses, lint of the changed modules); the merge
queue runs the compile, type, and test checks once, on the merged state, for the contexts the group changes, and is
the only gate ([ADR-0079](docs/adr/0079-ci-single-gate-merge-queue.md)). The slow suite runs nightly, by hand, and
before a release. The workflows in `.github/workflows/` say which job runs what: each step is named after the task it
runs, so a failed step is reproduced with `scripts/task.sh <step name>`, and `scripts/agentkit.sh pr jobs <pr>`
prints where the time went. To run the merge queue's jobs on a pull request (an outside contribution, a failure to
investigate), add the label `allow:full-ci`.

## Schema changes

The desired-state schema files of each dialect are the source of truth. `my schema apply` applies them with the
[Atlas](https://atlasgo.io/) Go library (Community Edition), and the Atlas CLI checks changes to them. Nobody writes migration SQL.

- The desired state is one directory per module, `schema/<NN>-<module>/`, with a file per dialect, `sqlite.sql` and
  `postgres.sql` ([schema/README.md](schema/README.md)). Edit both files of the module that owns the table in the same
  pull request. Tables follow the kinds of [ADR-0020](docs/adr/0020-data-modeling-principles.md),
  [schema/README.md](schema/README.md#conventions) maps the logical types to each dialect's types, and each module's
  data model is the package comment of its Go packages.
- What Atlas Community Edition cannot handle (extensions, sequences, views, triggers) stays out of the desired state.
  PostgreSQL extensions go into `schema/ddl/postgres.bootstrap.sql`, an idempotent bootstrap script.
- `scripts/install-atlas.sh` installs the Atlas CLI into `bin/atlas` with a pinned version and checksum, the version of the
  library in `go.mod` (`scripts/task.sh db:atlas`; the `db:*` tasks run it for you). Nothing uses the features that need `atlas login`.

```sh
scripts/task.sh db:apply                      # SQLite: apply the desired state to my.db with my schema apply
scripts/task.sh db:diff                       # only print the SQL that would be applied (my schema apply -dry-run)
scripts/task.sh db:reset                      # delete everything in the database, then apply (data is lost)
scripts/task.sh db:check                      # db:reset, then check with Atlas that no difference is left (as CI does)
scripts/task.sh db:check-all                  # db:check on SQLite and on the PostgreSQL container of db:up

scripts/task.sh db:up DB_DIALECT=postgres     # start a PostgreSQL container (database `my`, Atlas's dev databases `my_dev` and `my_lint`)
scripts/task.sh db:apply DB_DIALECT=postgres  # likewise db:diff / db:reset / db:check
scripts/task.sh db:down                       # delete the container and its data
```

- `DB_URL` (the target database, an Atlas URL; `my schema apply` takes it as `database_url`, without `sqlite://`) and
  `DB_DEV_URL` (the empty database Atlas normalizes differences with in `db:check`) change the connections.
  A PostgreSQL URL is a schema-scoped URL with `search_path=public`.
- `my schema apply` runs the bootstrap script itself, so a PostgreSQL outside the container needs nothing more for
  `db:apply`; `db:check` and `db:lint` also need the bootstrap script on Atlas's dev databases.
- If a change fails to apply as a difference, recreate a development database with `scripts/task.sh db:reset`. From
  `v0.1.0` on, a pull request's schema change must upgrade the previous `main` without losing data:
  `scripts/task.sh db:lint DB_DIALECT=sqlite|postgres` finds destructive changes, and
  [schema/README.md](schema/README.md#changing-the-schema) describes the classes of change and the approval label.
- CI checks, for both dialects, that the schema applies to an empty database and leaves no difference afterwards
  (`scripts/task.sh db:check-all`), and, on a pull request that changes the schema, that the change is not destructive
  against its base (`db:lint`).

## Client API

- `api/spec/` (OpenAPI 3.1, one file per path and per `components` entry) is the source of truth.
  `scripts/task.sh api:generate` bundles it with Redocly CLI into `api/openapi.yaml`, which the generators read; never
  edit `api/openapi.yaml` by hand.
- oapi-codegen (pinned in `tools/go.mod`) generates the Go strict server into
  `internal/interface/clientapi/api.gen.go`, which is committed.
- The conventions are in [ADR-0016](docs/adr/0016-client-api-style.md); `api/redocly.yaml` and the tests of `internal/interface/clientapi` enforce them.

```sh
scripts/task.sh api:generate  # bundle api/spec into api/openapi.yaml and regenerate from it
scripts/task.sh api:lint      # lint api/spec with Redocly CLI (including the usable subset of 3.1; config in api/redocly.yaml)
scripts/task.sh api:check     # api:spec-check (the root lists every file of api/spec), api:lint, and api:generate-check, which fails when the bundle or the generated code is stale (part of check)
```

## Web client

`my serve` serves the HTML shell embedded in the binary for `/` and the client's routes, with the link previews, the
icons, and the Web App Manifest. The shell loads no client script, and the server-rendered pages (sign-in and the
others) render without the client's stylesheet.

- `scripts/task.sh web:build` places the brand assets of `web/brand` in `internal/interface/web/dist`, which Go
  embeds with `//go:embed all:dist`. Only a placeholder `.gitkeep` is committed there, so the Go build and tests need
  no Bun. `scripts/web-icons.sh` renders the PNG icons and `internal/interface/web/preview-image.png` from its SVGs.
- `scripts/task.sh go:bin` runs `web:build` first and builds the single binary `bin/my`; the Docker image embeds the
  same files. Pass [build tags](docs/reference/build-tags.md) in `GO_TAGS` to leave unused adapters out of the binary.
- `my serve -dev-vite-origin <origin>` loads the client's scripts from the web client's Vite dev server at that origin.
  The shell is then never cached (`no-store`), and the CSP is a development one that allows that origin's scripts and
  WebSocket and the `<style>` elements Vite injects.
- The runtime configuration's `apiOrigin` is the origin of `MY_WEB_ORIGIN` when that is on `host` (such as
  `https://my.localhost`, or `http://localhost:8080`), and `https://{host}` otherwise; with `-dev-vite-origin` and
  no `MY_WEB_ORIGIN` on `host`, it is `http://{host}`.

## SQL and database tests

- Queries are written per dialect and module in `queries/<dialect>/<module>/*.sql`, and [sqlc](https://sqlc.dev/)
  generates the Go code of each module into `internal/adapter/db/<dialect>/<module>gen` (ADR-0153 §3). A query goes in
  the module of the repository that runs it, and can read the tables of that module and of the modules it depends on.
  The configuration is `sqlc.yaml` and the sqlc version is pinned in `tools/go.mod`. The generated code is committed,
  and `scripts/task.sh check` fails when it is stale.

```sh
scripts/task.sh go:sqlc        # regenerate from sqlc.yaml
scripts/task.sh go:sqlc-check  # fail when the generated code is stale (part of check)
```

- Tests come in three layers ([ADR-0153](docs/adr/0153-test-boundaries.md)):
  - A **port contract** checks the methods of one repository on every adapter, with no service of another module
    (`webhooktest.WebhookRepositoryContract`).
  - A **flow** drives the use cases of the modules it crosses. It runs once, on SQLite, in
    `internal/flowtest/<module>`, the most dependent module it goes through (`webhooktest.WebhookFlows` in
    `internal/flowtest/webhook`, with `flowtest.SQLite`). depguard keeps these packages off `internal/interface`,
    `internal/platform`, and the postgres adapter.
  - The **composition root** (`internal/platform`, `cmd/my`) checks the wiring, and holds the tests that drive the
    assembled app through its HTTP API. They depend on everything and run on every change, so keep them fast: lower
    the costs a test does not check (password hashes, key generation, the schema).

  When a contract of `pkg/testkit` builds the services of other modules, move that part to a flow and keep the
  repository's methods in a port contract that every adapter runs.
- Database adapters pass the contract tests of `pkg/testkit` on both SQLite and PostgreSQL. Each module's contracts
  and flows are in `pkg/testkit/<module>test` (`uowtest.UnitOfWorkContract`, `webhooktest.WebhookFlows`); `pkg/testkit`
  keeps what every module shares (`Clock`, `IDGenerator`, the fakes, the fixtures) and imports none of them. Test
  databases get the desired-state files run directly, in name order (the same SQL Atlas applies to an empty
  database), but only those of the modules the test binary links: each `schema/<NN>-<module>` is a package that
  imports the packages of the modules it depends on, and a module's database adapter packages import it, so a change
  to one module's files reruns only the tests that use the module (ADR-0153 §3). A test that reads the tables of a
  module it does not import adds a blank import of that module's `schema` package.
- A database adapter is a base per dialect and a package per module. `internal/adapter/db/<dialect>` holds the
  connections, the transactions and their retries, and the helpers of the queries; `internal/adapter/db/<dialect>/<module>`
  holds the repositories of one module of `internal/app`, which run that module's `<module>gen` queries over the base
  (ADR-0153 §3). A module's package imports the base, its own `<module>gen`, and the packages of the modules its module
  depends on in `internal/app`; the base imports none of them (depguard `db-base`, `db-<module>`). Move a helper that two
  modules need into the base. A contract test goes in the package of the module whose `pkg/testkit/<module>test` holds
  the contract.
- The SQLite tests get their database from `pkg/testkit/sqlitetest`: `sqlitetest.NewFile(t)` copies a file that the
  test binary made once with the schema of its linked modules applied, and `sqlitetest.ApplySchema` runs a schema in one transaction. Do not
  execute `schema.SQLite` outside a transaction: SQLite then commits and syncs the file after each of its hundreds of
  statements, which takes seconds per database and far longer on a loaded machine.
- The PostgreSQL tests get their server from `pkg/testkit/pgtest`. By default it starts pgmem (PostgreSQL in memory)
  inside the test process, so no Docker is needed: one server per test binary, and each test copies a template
  database with the schema applied.
  A test of what only a real server shows beyond pgmem (Atlas planning against the server, for example) calls
  `pgtest.RequireRealServer(t)`: it skips on pgmem and runs in `scripts/task.sh go:test-postgres`.
- To run against a real PostgreSQL, use one of these (CI runs `scripts/task.sh go:test-postgres`, the tests whose result can differ from pgmem, nightly):
  - `scripts/task.sh go:test-postgres`: starts a testcontainers-go container (`TEST_POSTGRES_IMAGE`, default
    `postgres:18-alpine`) with `TEST_POSTGRES=container`. Without Docker it fails, since it sets
    `TEST_POSTGRES_REQUIRED=1`.
  - `TEST_POSTGRES_URL=postgres://...` (a user that can create databases): uses an existing server.
- The Valkey tests (`kv/valkey` and others) get their server from `pkg/testkit/valkeytest`: with Docker it starts a
  `valkey/valkey` container (change it with `TEST_VALKEY_IMAGE`) through testcontainers-go, and without Docker the
  tests are skipped. `TEST_VALKEY_URL=valkey://...` uses an existing server. CI's `go:test` runs with
  `TEST_VALKEY_REQUIRED=1`, so it fails without Docker.
- The tests of the Amazon SNS / SQS adapter (`stream/sns`) run against the in-memory fake of `pkg/testkit/awstest`. To
  run them against the AWS emulator [Floci](https://github.com/floci-io/floci) too, use one of these (CI runs
  `scripts/task.sh go:test-stream` nightly):
  - `scripts/task.sh go:test-stream`: starts a testcontainers-go container (`TEST_AWS_IMAGE`, default
    `floci/floci:2.1.0`) with `TEST_AWS=container`. Without Docker it fails, since it sets `TEST_AWS_REQUIRED=1`.
  - `TEST_AWS_ENDPOINT=http://localhost:4566`: uses a running emulator.
- The tests of the Google Cloud Pub/Sub adapter (`stream/pubsub`) run against pstest, a fake server inside the
  process, by default, so no Docker is needed. `scripts/task.sh go:test-stream` (`TEST_STREAM=container`) starts a
  Pub/Sub emulator container (change it with `TEST_PUBSUB_IMAGE`) and runs the same contract; without Docker it fails,
  since it sets `TEST_PUBSUB_REQUIRED=1`. CI runs it nightly.
- The integration tests of `mail = smtp` (`internal/adapter/mail/smtp` and the tests of `internal/platform` with
  `Mailpit` in their names) talk to a Mailpit container that `pkg/testkit/mailpittest` starts for each test (change it
  with `TEST_MAILPIT_IMAGE`), and run only with `TEST_MAIL=container` (`scripts/task.sh go:test-mail`). CI runs them
  nightly.

## Emails

Every email the server sends is a draft from a pure function next to its job handler (`internal/app/<module>/mail_drafts.go`),
which `kernel.MailComposer` completes with the common frame: the committed HTML layout
(`internal/app/kernel/mail_frame.gen.html`) and the text version with the signature. The catalog in
`internal/platform/mailpreview/catalog.go` renders each of them from sample inputs:

- `scripts/task.sh mail:preview` writes every email in `en`, `ja`, `en-XA`, and `ar-XB` to `.tmp/mail/preview/` and
  prints the path of its `index.html`; open it in a browser. `scripts/task.sh mail:preview-watch` renders them again
  on every change of the layout, the messages, or the Go code.
- A new email needs a draft function, a job type named `Job...Email`, and its entries in the catalog: a test fails
  until the catalog has them. `scripts/task.sh mail:golden` then writes its golden files
  (`internal/platform/mailpreview/testdata/golden`), so that a change to any email shows in the pull request's diff.

With `mail = none` (the default) and `profile = development`, nothing is sent and the email goes to the log, so you can take the
confirmation link from the log.

To see the email that would really be sent (headers, the HTML version, links), send it with `mail = smtp` to the development SMTP
server [Mailpit](https://mailpit.axllent.org/) (MIT). Mailpit keeps the email it receives without delivering it to anyone and
shows it in a web UI (`http://127.0.0.1:8025`) and a REST API.

- `mail = smtp` never skips STARTTLS or certificate verification, so Mailpit requires STARTTLS with a certificate of a development
  CA (for `mailpit`, `localhost`, and `127.0.0.1`, in `.tmp/mailpit`) that `scripts/mailpit-cert.sh` creates, and `my serve` trusts
  that CA through `SSL_CERT_FILE`.
  `SSL_CERT_FILE` replaces the system root certificates, so that `my serve` cannot make HTTPS requests to outside servers
  (federation and the like). Use it only for development checks.
- The image (`axllent/mailpit`) is pinned by tag and digest, the same as in the integration tests
  (the service `mailpit` of the root [`compose.yaml`](compose.yaml)).
- The server of [Local development](#local-development) trusts the CA alongside the system roots, so it still reaches
  servers outside.

[Local development](#local-development) sends every email of the server to Mailpit, which
`scripts/task.sh docker:mailpit` starts alone (stop it with `docker compose rm -sf mailpit`).

Ask for a password reset (`/auth/forgot_password`) or register in `open` mode, and the email arrives in the
Mailpit UI. Open the link of the email you opened in the UI in your browser.

The integration tests check the headers, encoding, and body of the email the adapter sent and the combinations of STARTTLS
and authentication, and run registration confirmation and password reset through the links of the email Mailpit received.
Pull requests and the merge queue check the same adapter against a stand-in SMTP server inside the tests.

## Build tags

[The reference](docs/reference/build-tags.md) lists the tags. The test build adds the fakes for tests:

```sh
scripts/task.sh go:bin GO_TAGS=e2e_fakes
MY_PROFILE=development MY_PAYMENTS=fake bin/my serve
```

Four checks keep the fakes out of production; each one stops them on its own when another one is bypassed. The first is at start, and [the reference](docs/reference/build-tags.md#test-builds) describes it. The others:

- **The build check.** `scripts/task.sh go:build-tags` fails when `cmd/my` without the tag (with no tags, or with any
  representative set of `no_*` tags) depends on a fake. Only files with the constraint `//go:build e2e_fakes` import
  them outside the tests: `internal/platform/adapters_payments_fake.go` the fake payment provider,
  `internal/platform/adapters_search_fake.go` the fake search index, and `cmd/my/dev_push_service.go` the fake push service.
- **The container image.** The `Dockerfile` stops when `GO_TAGS` has `e2e_fakes`, before it compiles anything, and
  checks the binary it built as the release check does.
- **The release.** `scripts/check-release-build.sh BINARY...` fails when the Go build info of a binary
  (`go version -m`, its `build -tags=` line) has `e2e_fakes`, or cannot be read. Every job that publishes a binary or an
  image runs it on what it publishes.

### The fake push service

A browser's Web Push subscription points at its vendor's push service, which a test cannot reach. `my dev push-service` stands in for it: it serves HTTPS on this machine, issues subscriptions, and receives
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
development Mailpit in [Emails](#emails)) and its SSRF guard lets the Web Push sends reach
`localhost` ([`push_allowed_destinations`](docs/reference/configuration.md#push_allowed_destinations) set to `localhost`, with
`profile = development`). Its HTTP API, in JSON:

| Request | Answer |
| --- | --- |
| `POST /subscriptions` with `{"applicationServerKey": "<base64url>"}` | `201` and a new subscription in the shape of the browser's `PushSubscription.toJSON()`: `endpoint` and `keys` (`p256dh`, `auth`) |
| `POST /push/<id>`, a push message to the endpoint | `201` with `Location` when it is accepted; `401` without a `vapid` `Authorization`; `403` when the token's signature, its `k` (other than the subscription's `applicationServerKey`), its `aud` (other than the origin), its `exp` (past, or more than 24 hours ahead), or its `sub` (missing) fails; `400` without `TTL`, with a `Content-Encoding` other than `aes128gcm`, a bad `Urgency` or `Topic`, or a body that does not decrypt; `413` over 4096 bytes; `404` for an unknown subscription; `410` once it has expired |
| `POST /subscriptions/<id>/expire` | `204`; later push messages to it answer `410` |
| `GET /subscriptions/<id>/messages` | `200` and `{"messages": [...]}`: every push message to it, oldest first, with `status`, `error` (why it was refused), `plaintext`, `ttl`, `urgency`, `topic`, `subject`, and `received_at` |

### The check of the sets

`scripts/task.sh go:build-tags` (part of `scripts/task.sh go:check` and, in CI, of the slow suite that gates releases)
runs, for each representative set of tags, `CGO_ENABLED=0 go build` and `go vet` on every package,
and fails when `cmd/my` still depends on a package that a tag of the set leaves out.

The sets are in [the reference](docs/reference/build-tags.md#sets-of-tags-that-are-checked), together with `e2e_fakes`, which also runs the tests that only this build has (named `*E2EFakes*`).

It also fails when `cmd/my` without `e2e_fakes`, the build with no tags among them, depends on a fake, or when the set
with `e2e_fakes` does not link the fakes.
`scripts/go-build-tags.sh SET...` checks other sets. The tests (`scripts/task.sh go:test`) run without tags, on every adapter.

### Adding a tag

An adapter that brings a dependency of its own gets the tag `no_<name>`:

- The platform wires it in `internal/platform/adapters_<name>.go` with the constraint `//go:build !no_<name>`, and
  `internal/platform/adapters_no_<name>.go` (`//go:build no_<name>`) puts in its place a function of the same signature
  that returns `config.ExcludedError`. Nothing else in `cmd/` and `internal/platform` imports the adapter.
- `internal/platform/config/adapters_no_<name>.go` calls `exclude` for the settings that select it, so that `Load`
  rejects them; a check of a setting that only this adapter reads goes in `adapters_<name>.go` likewise.
- An adapter package shared by several dialects (such as `internal/adapter/kv/db`) puts the code of each dialect,
  its constructor, and its tests in files with the dialect's constraint.
- Add the tag to the tables of [the reference](docs/reference/build-tags.md), to the choice documentation in `internal/platform/config/schema.go`
  (`scripts/task.sh go:config-docs`), and to `excluded_packages` in `scripts/go-build-tags.sh`, and use it in one of the representative sets.

## Settings

To decide whether a new value is configuration or an instance setting, ask whether an operator changes it without a restart. A
value that is fixed per deployment and changes the structure of the process is configuration; a value that changes while the
instance is running is an instance setting. `TestConfigAndInstanceSettingsShareNoKey` fails when the two sets of keys overlap.

## The AI crawler list

The AI crawlers that `/robots.txt` names are generated from `robots.json` of
[ai-robots-txt/ai.robots.txt](https://github.com/ai-robots-txt/ai.robots.txt) (MIT) and bundled in
`internal/interface/web/ai_crawlers.gen.go`. About once a month, open an update pull request as follows.

1. In `tasks/go.yml`, change `AI_ROBOTS_COMMIT` to the new commit and `AI_ROBOTS_SHA256` to the SHA-256 of that commit's `robots.json`.
2. Run `scripts/task.sh go:ai-crawlers`.
3. Review the diff. The classification (training or AI use) is derived from the `function` text in `robots.json`, so if a newly added crawler is classified wrongly,
   fix it with `overrides` in `scripts/ai-crawlers/main.go`. Link-preview fetchers and general-purpose search engines are left out of the list with `excluded`.

## Design decisions

A change to the architecture needs an ADR: `scripts/task.sh docs:adr-new TITLE="..." SLUG=...`. Only maintainers
mark an ADR `Accepted`.

## Documentation

- Documents for users, operators, and contributors are written in English only, with no Japanese copy: `README.md`,
  `CONTRIBUTING.md`, `docs/README.md`, `docs/operations/`, `docs/reference/`, `.claude/`, `schema/README.md`, the
  comments of `deploy/`, and the issue and pull request templates.
- Design documents (`docs/adr/`, `docs/requirements/`, `docs/architecture/`) are written in Japanese during v0.
- The documentation site's Japanese pages are machine translations that an agent writes into `web/docs/i18n/`, following
  the [`/translate-docs` skill](.claude/skills/translate-docs/SKILL.md). The English document is authoritative: change it,
  not only its translation. A pull request that changes English documents does not need to translate them too.
- Documents for users and operators (`README.md`, `docs/operations/`, `docs/reference/`) explain the behaviour themselves. They do not link to or cite ADRs, `docs/requirements/`, `docs/architecture/`, or GitHub issues and pull
  requests; link only the other user and operator documents a reader needs.
- A diagram in a document follows the [`/draw-diagram` skill](.claude/skills/draw-diagram/SKILL.md): hand-written SVG that switches
  between light and dark itself, with text alternatives.
- Every document describes the present state. Write why something changed in the commit message and the pull request.
- `scripts/task.sh docs:rules` (part of `docs:check`) checks these rules and that relative links and their `#anchors`
  resolve; [docs/lint.yaml](docs/lint.yaml) lists the files each rule covers.
- A change keeps the user and operator documents that describe it true: `scripts/agentkit.sh docs impact` lists those
  of your change. Every command of `my`, metric, webhook event type, build tag, and task appears in its document
  (`scripts/task.sh agentkit:inventory`).
- The design documents and the agent instructions do not grow without the decider's `allow:docs-growth` label
  (`scripts/task.sh docs:growth`): an ADR holds a decision, its reasons, and when to revisit it, not what the code says.

## Language

- Code, code comments, commit messages, and pull request titles: English.
- Issue and pull request descriptions: English or Japanese.

## Translations and messages

User-facing strings go into `locales/<locale>/<namespace>/<area>.json` (FormatJS JSON format), and `en` and `ja` are
updated together: a key that `ja` lacks fails `scripts/task.sh i18n:check`.
Change the catalogs with `scripts/i18n.sh` (`scripts/i18n.sh help` lists its commands). It writes a key to the file of
its area (the key's first segment, or its first two for `admin`: `feed.tag_title` is in
`locales/en/server/feed.json`), refuses a text that
breaks a rule of the check and says which, and regenerates the pseudo-locales (`en-XA` / `ar-XB`) and the typed accessors:

```sh
scripts/i18n.sh add server:feed.empty_title --en "No posts yet" --ja "投稿はまだありません" --desc "Title of an empty feed."
scripts/i18n.sh set server:feed.empty_title --en "No posts" --ja "投稿はありません"  # drops the machine translations of the old text
scripts/i18n.sh mv server:feed.empty_title feed.none_title  # in every locale, and the references in the source
scripts/i18n.sh rm server:feed.none_title                   # refused while the source refers to it
scripts/i18n.sh show feed.empty                             # also: find "<text>", unused
scripts/i18n.sh check --changed                               # the checks of the keys changed since origin/main
```

Commit what Go reads (the pseudo-locales of `server`, `internal/i18n/messages_gen.go`, and
`internal/i18n/testdata/golden.json`).

Every accessor takes the arguments of a message by name, so that two arguments of the same type cannot be swapped:

- In Go, an accessor of a message with one argument takes it as its only parameter (`l.ServerErrorsUnusableImage(name)`);
  one with two or more takes a generated struct named after it, with one field per argument
  (`l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: max, Name: name})`). The struct cannot be written without
  the field names, and `scripts/task.sh go:lint` reports a field left out.

To see the pages the server renders in a pseudo-locale, start it with `my serve -pseudo-locales` and set the browser's
language to `en-XA` or `ar-XB` (they cannot be chosen without the flag).
