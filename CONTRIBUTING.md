# Contributing

Human contributors and coding agents work by the same rules. [AGENTS.md](AGENTS.md) lists them in full, and the
decisions behind them are the Architecture Decision Records indexed in [docs/adr/README.md](docs/adr/README.md).

Do not report security vulnerabilities in issues or pull requests; follow [SECURITY.md](SECURITY.md) instead.

## Setup

Tool versions are pinned in `.tool-versions` (use [asdf](https://asdf-vm.com/) or [mise](https://mise.jdx.dev/)).

```sh
asdf install               # or: mise install
scripts/task.sh generate   # generate the uncommitted files your editor's type checker reads
scripts/task.sh check
```

Generated files that only TypeScript reads (`app/web/src/routeTree.gen.ts`, `tools/i18n/src/runtime/messages.gen.ts`,
`app/ui/src/themeTokens.gen.ts`, and the pseudo-locales of `client`) are not committed. The tasks that read them
(type checks, lint, builds, tests, development servers) generate them first, so they work right after a clone.
Editors do not go through the tasks, so run `scripts/task.sh generate` once after cloning; it skips the generators
whose inputs have not changed.

Tasks run with [go-task](https://taskfile.dev/) and live in `Taskfile.yml` and `tasks/<area>.yml`. `scripts/task.sh`
builds the version pinned in `tools/go.mod` into `bin/task` and runs it; never use a `task` from your PATH.
`scripts/task.sh --list` lists the tasks, and [docs/reference/commands.md](docs/reference/commands.md) describes them.

`scripts/task.sh check` runs the check of every context in parallel: docs, Go, i18n, the email layout (`app/mail`),
the API, `app/ui`, `app/web`, Docker, agentkit, and `pkg/mykit` with `pkg/mybot` (`docs:check`, `go:check`,
`i18n:check`, `mail:check`, `api:check`, `ui:check`, `web:check`, `docker:check`, `agentkit:check`, `mykit:check`).
`CHECKS="go web"` runs some of them only. `TASK_CONCURRENCY` limits the parallelism; locally `scripts/task.sh` defaults it to half the CPUs.
`scripts/task.sh check:changed` runs only the contexts you changed and those that depend on them (compared with
`BASE=origin/main`); CI decides the same way (`scripts/changed-contexts.sh`) which jobs to skip, and the required
check `check` collects the results of the jobs ([CI](#ci)).

The checks need no network once the Go modules and tools and the npm packages of `tools/i18n` and `app/` have
been downloaded the first time. They cover Markdown lint and the documentation rules, the ADR index, Go
(`go vet`, golangci-lint, tests, the `CGO_ENABLED=0` build, dependency licenses, the sqlc output), the i18n catalogs,
the client API spec and the code generated from it, the Go image of the `Dockerfile` against `.tool-versions`, and
the web client (type checks, build, dependency licenses).

- The Go tools (go-licenses, sqlc, oapi-codegen) are pinned by the `tool` directives of `tools/go.mod`.
- Dependabot Alerts on GitHub report known vulnerabilities in dependencies, and Dependabot opens the version update
  pull requests (`.github/dependabot.yml`).
- The Docker Compose smoke test (building the image, applying the schema, creating an administrator, checking
  WebFinger, the actor, and NodeInfo) needs Docker, so `scripts/task.sh check` leaves it out; run it with
  `scripts/task.sh docker:smoke`. CI runs it on pull requests that change the Docker setup, `cmd/`, or
  `internal/platform/`, and nightly; it builds the linux/arm64 image on pull requests that change the Docker setup and
  nightly.
- The live interoperability tests against Mastodon, GoToSocial, and Misskey are not part of `scripts/task.sh check`
  either: run them with `scripts/task.sh docker:federation-live` (CI runs them nightly; see
  [deploy/docker/federation-live/README.md](deploy/docker/federation-live/README.md)).
- The license is AGPL-3.0-only, except for `api/spec/`, `api/openapi.yaml`, `pkg/mykit/`, and `pkg/mybot/`, which are
  MIT. Their files start with `SPDX-License-Identifier: MIT`, every other new source file with `AGPL-3.0-only`, and
  `scripts/task.sh api:license-scope` (part of `api:check`) checks them. The MIT parts depend only on permissive
  licenses such as MIT, Apache-2.0, BSD, and ISC, never on GPL-family ones.
- Every other dependency must have a license compatible with AGPL-3.0 (`GO_ALLOWED_LICENSES` in `Taskfile.yml`; the npm
  packages of `tools/i18n` and `app/` are checked against the same list). Explain in the pull request
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

## Workflow

1. Pick a GitHub issue. `agent:ready` marks the issues with clear acceptance criteria that are ready to start.
2. Create a branch `<type>/<issue>-<slug>`. Never push to `main`.
3. Run the checks of the contexts you touched (`scripts/task.sh go:check`, ...) or `scripts/task.sh check:changed`,
   then commit (Conventional Commits, in English). Run the full `scripts/task.sh check` when you change shared
   configuration. Format Go with `gofmt` and TypeScript and JavaScript with `scripts/task.sh web:fmt` before you
   commit ([Formatting](#formatting)).
4. Open a pull request from the template, with its Documentation section filled (see [Documentation](#documentation)).
5. When its CI is green, a maintainer or the orchestrator (the role that runs agents in parallel and merges their pull
   requests) adds it to the merge queue with `gh pr merge <PR> -R mycroblog/my --auto --merge`. `main` only receives
   merges from the merge queue.
6. If the queue drops the pull request because of a conflict or a failing `check`, merge `main` into the branch,
   regenerate the committed generated files (those Go reads, the output of `scripts/task.sh generate`) instead of
   resolving their conflicts by hand, run `scripts/task.sh web:fmt` if a TypeScript or JavaScript file conflicted,
   run `scripts/task.sh check:changed`, and push; the person from step 5 queues it
   again. Merge `main` only when the pull request conflicts or the queue drops it: the queue checks it on top of the
   latest `main` anyway.

## Formatting

Go is formatted with `gofmt` (`scripts/task.sh go:fmt-check` checks it). The TypeScript and JavaScript of `app/`,
`pkg/mykit/ts`, `pkg/mybot/ts`, and `tools/i18n` are formatted with `bun fmt`, the formatter built into
the pinned Bun: run `scripts/task.sh web:fmt` before you commit them, and again after resolving a conflict in one of
them. `scripts/task.sh web:fmt-check` fails when a file needs formatting; `check` and CI run it. Its options are in
the `[fmt]` section of the root `bunfig.toml`, and `.prettierignore` lists the files it leaves alone (generated files
and the Oxlint violation fixtures). To format from an editor, have it run `bun fmt --stdin-filepath <file>`.

Commits that only reformat are listed in `.git-blame-ignore-revs`. GitHub's blame view skips them; for `git blame`,
run once:

```sh
git config blame.ignoreRevsFile .git-blame-ignore-revs
```

## CI

CI (`.github/workflows/`) splits what runs where as follows. Each job of `ci.yml` sets up its tools and then runs its
checks as parallel steps. Each step is named after its check and calls one task (`scripts/task.sh go:vet`), so the
Actions page shows the log and duration of every check, and you can run a failed check locally with the same task.
`scripts/task.sh docs:ci-coverage` makes sure that every check of the local `scripts/task.sh check` runs in some CI
step.

| When | Workflow | What runs |
| --- | --- | --- |
| Pull request | `ci.yml` | `preflight`: the checks that the pull request's own files and lockfiles decide — markdownlint, shellcheck, gofmt, `bun fmt` (which the merge queue's `repo` job runs again), the Go and npm dependency licenses, the versions of the Dockerfile against `.tool-versions` and of the Playwright image, the design tokens, and the CSS `100vh` check. A change to the ui context also runs the `@my/ui` story tests (`ui-test.yml`); a change to Go or `tools/agentkit` runs golangci-lint on the changed lines (`lint`); a change to the Docker setup (`Dockerfile`, `.dockerignore`, `deploy/`) runs the Docker Compose smoke test (`docker-smoke`). The required check `check` collects the results. A pull request labelled `ci:full` also runs the merge queue's jobs (adding or removing the label runs them again) |
| Merge queue | `ci.yml` | The full check on the merge group (the pull request on top of `main` and the pull requests queued before it). By the contexts changed (`scripts/changed-contexts.sh`): for go, schema, api, or i18n, `server-static` (tidy, vet, build, the generated files Go reads, the schema of both dialects) and `server-test` (the Go tests, PostgreSQL on pgmem); for ui, web, api, or i18n, `client-static` (tsc, Oxlint, knip, the Vite build, the i18n catalogs) and `client-test` (Vitest in two shards); always `repo` (agentkit, the context map, the documentation rules, the ADR numbers, `bun fmt`, and that CI runs every check of `check`). golangci-lint and the build tag combinations do not run here (the slow suite runs them). At most six jobs; a skipped job counts as passed. The queue merges when `check` passes |
| Push to `main` | `main.yml` | No checks: it only builds the caches that pull requests and the merge queue restore. The Go cache holds what the restoring jobs read (modules, package and tool builds, test binaries, vet and golangci-lint results), built one after another with the same settings (`CGO_ENABLED=0`) and rebuilt from scratch under a key of the Go version, `go.sum`, `tools/go.sum`, and the date; the Bun cache is keyed by the lockfile. Each is built only when no cache of its key exists yet |
| Release gate, nightly, by hand | `slow-suite.yml` | The slow suite: the full web E2E matrix (eight shards), the live interoperability tests (`federation-live.yml`), the tests on a real PostgreSQL, the `stream.Bus` contract against broker containers, the smoke test and the linux/arm64 image build, golangci-lint on everything and the build tag combinations, and CodeQL code scanning (`codeql.yml`; results go to the Security tab). Never on pull requests or the merge queue, and not part of `check` |
| Release (by hand) | `release.yml` | Runs the slow suite on the given commit and, only when it passes, creates the tag and a draft GitHub Release |
| Nightly (03:00 JST) | `nightly.yml` | Runs the slow suite on the head of `main`, unless `main` has not moved since the last successful nightly |

- Pull request CI does not compile, type-check, or test. Run `scripts/task.sh check:changed` locally before you push.
  To investigate a merge queue failure on the pull request, or to check an outside contribution early, add the label
  `ci:full` to the pull request.
- Pull requests do not run the web E2E automatically. For a pull request that touches the web client, run
  `scripts/task.sh web:e2e` locally, or run `gh workflow run web-e2e.yml -R mycroblog/my --ref <branch>` against its
  branch (add `-f matrix=full` for the full matrix; the Actions tab can start it too).
- `slow-suite.yml`, `nightly.yml`, `web-e2e.yml`, `federation-live.yml`, `postgres-contract.yml`,
  `stream-contract.yml`, `mail-integration.yml`, and `codeql.yml` can also be run by hand (`workflow_dispatch`).

## Schema changes

The desired-state schema files of each dialect are the source of truth, applied with [Atlas](https://atlasgo.io/)
Community Edition. Nobody writes migration SQL.

- The desired state is one file per module, `schema/<dialect>/<NN>-<module>.sql` ([schema/README.md](schema/README.md)).
  Edit the file of the module that owns the table, in both `schema/sqlite/` and `schema/postgres/`, in the same pull
  request. Tables follow the [logical data model](docs/architecture/data-model/README.md), whose
  [§1.2](docs/architecture/data-model/01-conventions.md#12-論理型) maps the logical types to each dialect's types.
- What Atlas Community Edition cannot handle (extensions, sequences, views, triggers) stays out of the desired state.
  PostgreSQL extensions go into `schema/postgres.bootstrap.sql`, an idempotent bootstrap script.
- `scripts/install-atlas.sh` installs Atlas into `bin/atlas` with a pinned version and checksum
  (`scripts/task.sh db:atlas`; the `db:*` tasks run it for you). Nothing uses the features that need `atlas login`.

```sh
scripts/task.sh db:apply                      # SQLite: apply the desired state to my.db (the dev database is in memory)
scripts/task.sh db:diff                       # only print the SQL that would be applied
scripts/task.sh db:reset                      # delete everything in the database, then apply (data is lost)
scripts/task.sh db:check                      # db:reset, then check that no difference is left (as CI does)
scripts/task.sh db:check-all                  # db:check on SQLite and on the PostgreSQL container of db:up

scripts/task.sh db:up DB_DIALECT=postgres     # start a PostgreSQL container (database `my`, dev databases `my_dev` and `my_lint`)
scripts/task.sh db:apply DB_DIALECT=postgres  # likewise db:diff / db:reset / db:check
scripts/task.sh db:down                       # delete the container and its data
```

- `DB_URL` (the target database) and `DB_DEV_URL` (the empty database Atlas normalizes differences with) change the
  connections. A PostgreSQL URL is a schema-scoped URL with `search_path=public`.
- To apply to a PostgreSQL outside the container, also say how to run the bootstrap script with `PG_BOOTSTRAP`
  (for example `PG_BOOTSTRAP='psql -v ON_ERROR_STOP=1 -q "postgres://..."'`), and run the bootstrap script on the dev
  database first.
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
- The conventions and the steps to add an endpoint are in [docs/architecture/client-api.md](docs/architecture/client-api.md).

```sh
scripts/task.sh api:generate  # bundle api/spec into api/openapi.yaml and regenerate from it
scripts/task.sh api:lint      # lint api/spec with Redocly CLI (including the usable subset of 3.1; config in api/redocly.yaml)
scripts/task.sh api:check     # api:spec-check (the root lists every file of api/spec), api:lint, and api:generate-check, which fails when the bundle or the generated code is stale (part of check)
```

## Web client

The web client (`app/web`) is built with Vite, and `my serve` serves the build and the HTML shell embedded in the
binary. Its conventions are in [docs/architecture/web-client/README.md](docs/architecture/web-client/README.md).

- `scripts/task.sh web:build` builds into `app/web/dist` and copies it to `internal/interface/web/dist`, which Go
  embeds with `//go:embed all:dist`. Only a placeholder `.gitkeep` is committed there, so the Go build and tests need
  no Bun (such a binary serves the shell without scripts).
- `scripts/task.sh go:bin` runs `web:build` first and builds the single binary `bin/my` with the web client inside;
  the Docker image embeds the same build. Pass [build tags](docs/reference/build-tags.md) in `GO_TAGS` to leave
  unused adapters out of the binary.

During development, run the Vite dev server and pass its origin to `my serve`, then open `my serve` in the browser.
Go serves the shell, the runtime configuration, the API, and ActivityPub as in production, and only the scripts come
from Vite, with HMR.

```sh
scripts/task.sh db:apply                       # apply the schema to the SQLite my.db
scripts/task.sh web:dev                        # the Vite dev server (always http://localhost:5173; generates first)
MY_HOST=localhost:8080 MY_DATABASE_URL=my.db MY_KEK="$(openssl rand -base64 32)" \
  go run ./cmd/my serve -dev-vite-origin http://localhost:5173   # in another terminal; open http://localhost:8080
```

- With `-dev-vite-origin`, the shell is never cached (`no-store`) and the CSP is a development one that allows that
  origin's scripts and WebSocket and the `<style>` elements Vite injects. To see the app under the production CSP
  (`script-src 'self'` without a nonce), use a binary built by `scripts/task.sh go:bin`.
- The runtime configuration's `apiOrigin` is `http://{host}` only in development and when `MY_WEB_ORIGIN` is the
  `http` origin of `host` (such as `http://localhost:8080`).

## SQL and database tests

- Queries are written per dialect in `queries/<dialect>/*.sql`, and [sqlc](https://sqlc.dev/) generates the Go code.
  The configuration is `sqlc.yaml` and the sqlc version is pinned in `tools/go.mod`. The generated code is committed,
  and `scripts/task.sh check` fails when it is stale.

```sh
scripts/task.sh go:sqlc        # regenerate from sqlc.yaml
scripts/task.sh go:sqlc-check  # fail when the generated code is stale (part of check)
```

- Database adapters pass the contract tests of `pkg/testkit` (`testkit.UnitOfWorkContract`, for example) on both
  SQLite and PostgreSQL. Test databases get the desired-state files of `schema/<dialect>/` run directly, in name order
  (the same SQL Atlas applies to an empty database).
- The PostgreSQL tests get their server from `pkg/testkit/pgtest`. By default it starts pgmem (PostgreSQL in memory)
  inside the test process, so no Docker is needed: one server per test binary, and each test copies a template
  database with the schema applied.
- To run against a real PostgreSQL, use one of these (CI runs `scripts/task.sh go:test-postgres` nightly):
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

## Design decisions

A change to the architecture needs an ADR: `scripts/task.sh docs:adr-new TITLE="..." SLUG=...`. Only maintainers
mark an ADR `Accepted`.

## Documentation

- Documents for users, operators, and contributors are written in English only, with no Japanese copy: `README.md`,
  `CONTRIBUTING.md`, `docs/README.md`, `docs/operations/`, `docs/reference/`, `docs/agents/`, `schema/README.md`, the
  comments of `deploy/`, and the issue and pull request templates.
- Design documents (`docs/adr/`, `docs/requirements/`, `docs/architecture/`) are written in Japanese during v0.
- Documents for users and operators (`README.md`, `docs/operations/`, `docs/reference/` except `commands.md`, the
  development tasks for contributors) explain the behaviour themselves. They do not link to or cite ADRs, `docs/requirements/`, `docs/architecture/`, or GitHub issues and pull
  requests; link only the other user and operator documents a reader needs.
- Every document describes the present state. Write why something changed in the commit message and the pull request.
- `scripts/task.sh docs:rules` (part of `docs:check`) checks these rules and that relative links and their `#anchors`
  resolve; [docs/lint.yaml](docs/lint.yaml) lists the files each rule covers.
- A change keeps the user and operator documents that describe it true. The `userdocs` of
  [tools/agentkit/context-map.yaml](tools/agentkit/context-map.yaml) say which documents describe which paths, and
  `scripts/agentkit.sh docs impact` lists those of your change. Update each one the change makes wrong; the
  **Documentation** section of the pull request lists every one as `changed` or as `no change — <reason>`, and the
  pull request's check fails otherwise. Every command of `my`, metric, webhook event type, build tag, and task appears
  in its document (`scripts/task.sh agentkit:inventory`).

## Language

- Code, code comments, commit messages, and pull request titles: English.
- Issue and pull request descriptions: English or Japanese.

## Translations and messages

User-facing strings go into `locales/<locale>/<namespace>/<area>.json` (FormatJS JSON format), and `en` and `ja` are
updated together.
A key goes into the file of its area: the area is the key's first segment, or its first two segments for `admin`.
`settings.profile.title` goes into `locales/en/client/settings.json`, and `admin.report.title` into
`locales/en/client/admin/report.json`. A key of a new area goes into a new file of that name (nothing that bundles the
files needs to change); `scripts/task.sh i18n:check` reports keys written in the wrong file.
After editing, run `scripts/task.sh i18n:gen` to regenerate the pseudo-locales (`en-XA` / `ar-XB`) and the typed
accessors, and commit what Go reads (the pseudo-locales of `server`, `internal/i18n/messages_gen.go`, and
`internal/i18n/testdata/golden.json`). The pseudo-locales of `client` and the TypeScript accessors are not committed.
To see the pages the server renders in a pseudo-locale, start it with `my serve -pseudo-locales` and set the browser's
language to `en-XA` or `ar-XB` (they cannot be chosen without the flag).
