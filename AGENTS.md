# AGENTS.md

Instructions for coding agents (Claude Code, Codex, Copilot, Gemini, etc.) working in this repository.
Human contributors should read [CONTRIBUTING.md](CONTRIBUTING.md); the rules are the same.

## Project in one paragraph

An ActivityPub-compatible microblog server written in Go, designed to run at near-zero cost on Google Cloud, AWS,
or a self-hosted VM (Docker / microk8s), with swappable RDB / queue / storage adapters, its own client API (no Mastodon client API compatibility),
a PWA web client (no mobile app in v0), and i18n from day one. Distributed as OSS.

## Current phase: M5 — v0 Release

- Goal: publish v0.x and start running the maintainer's instance — moderation essentials, i18n (en / ja and more),
  documentation, the release (tags, the container image on `ghcr.io/mycroblog/my`, release notes), and the API client
  library `pkg/mykit` and the bot library `pkg/mybot` (epic [#30](https://github.com/mycroblog/my/issues/30),
  [vision](docs/requirements/00-vision.md)). The mobile app is outside v0. M4.3 — Voice Chat (epic [#1345](https://github.com/mycroblog/my/issues/1345))
  is a precondition of v0.1.0. Deployment templates for GCP / AWS / microk8s belong to M6.
- The server is Go ([ADR-0001](docs/adr/0001-server-language-and-runtime.md)); clients are TypeScript under `app/` with Bun.
  Cloudflare Workers is **not** a target.
- **Web client work must follow [docs/architecture/web-client/](docs/architecture/web-client/README.md).** Do not write React from memory or
  from typical online examples; when unsure, check react.dev, the WAI-ARIA APG and the official docs of the library in use.
- The web-client stack and libraries are fixed by Accepted ADRs (from ADR-0025; see [docs/adr/README.md](docs/adr/README.md));
  do not pick libraries outside them without a new ADR.
- The Go package layout follows [overview §6](docs/architecture/overview.md#6-リポジトリ構成); `cmd/my` is the single binary.
  Build only what an issue asks for.
- Spike reports go in the spike issue as a comment and spike code stays on an unmerged `spike/<issue>-<slug>` branch;
  only the resulting ADR / doc updates are merged ([ADR-0022](docs/adr/0022-spike-outputs-outside-repository.md)).

## Source of truth

| What | Where |
| --- | --- |
| Vision, scope, milestones | [docs/requirements/00-vision.md](docs/requirements/00-vision.md) |
| Functional requirements (`FR-xxx`) | [docs/requirements/01-functional.md](docs/requirements/01-functional.md) |
| Non-functional requirements (`NFR-xxx`) | [docs/requirements/02-non-functional.md](docs/requirements/02-non-functional.md) |
| Architecture overview, ports, flows | [docs/architecture/overview.md](docs/architecture/overview.md) |
| Hosting targets & cost notes | [docs/architecture/deployment-targets.md](docs/architecture/deployment-targets.md) |
| Federation design (inbox, delivery, fetch, sanitizing) | [docs/architecture/federation.md](docs/architecture/federation.md) |
| Decisions | [docs/adr/README.md](docs/adr/README.md) — **Accepted ADRs are binding; Proposed ADRs are not yet decisions** |
| Tasks | GitHub Issues / Milestones of `mycroblog/my` |
| Agent playbooks | [docs/agents/](docs/agents/) |
| Commands, agent tools | [docs/reference/commands.md](docs/reference/commands.md), [tools/agentkit/README.md](tools/agentkit/README.md) |

## How to pick and do work

Follow [docs/agents/workflow.md](docs/agents/workflow.md). Summary:

1. Only take issues labeled **`agent:ready`** with no assignee. Never take `status:needs-decision`.
2. Branch `<type>/<issue>-<slug>`; never push to `main`.
3. Before every commit, run the checks of the contexts you touched (`scripts/task.sh go:check`, `scripts/task.sh web:check`, ...) or
   `scripts/task.sh check:changed`; the PR's CI runs only a preflight, and the compile, type, lint, and test checks run in the
   merge queue, so this local run keeps a broken PR out of the queue. Run the full `scripts/task.sh check`
   when the change touches shared configuration.
4. Open a PR using the template, with `Closes #N` and the acceptance criteria checked, and wait for its CI to pass.
   Pull requests run the `check` gate as a preflight of under 2 minutes ([ADR-0079](docs/adr/0079-ci-single-gate-merge-queue.md);
   the label `ci:full` runs the merge queue's full check on the PR too) but not the web E2E: in single-issue work, for a
   change to the web client, run `scripts/task.sh web:e2e` or `gh workflow run web-e2e.yml --ref <branch>`. In a parallel batch run by an orchestrator,
   follow [workflow.md §9](docs/agents/workflow.md#9-parallel-batches) instead (no per-PR web E2E; failures of the nightly web E2E are fixed together at the end of the milestone).
   Do not merge or queue the PR yourself: the orchestrator or the decider queues it with `gh pr merge --auto`, and the merge queue
   runs the full `check` of the changed contexts on top of the latest `main` before merging. Merge `main` into the branch only when the PR conflicts or the
   queue drops it. Pushes to `main` check nothing and only build the caches that PRs and the queue restore. The slower
   suites and CodeQL (`slow-suite.yml`) run as the release gate (`release.yml`), nightly, and by hand, never on PRs
   ([ADR-0079](docs/adr/0079-ci-single-gate-merge-queue.md)); the table of what runs where is in the workflow.
5. If you need a decision that no Accepted ADR covers: small → state the assumption in the PR; large → draft a *Proposed* ADR
   (`scripts/task.sh docs:adr-new TITLE=... SLUG=...`), comment on the issue, add `agent:blocked`, and stop.

## Hard rules

- **Boy scout rule** ([ADR-0024](docs/adr/0024-boy-scout-rule.md)): never ignore something broken because it is outside your issue.
  Fix small things in a separate commit and list them in the PR; file an issue for anything larger and link it.
- **Never change an ADR's status to `Accepted`/`Rejected` on your own judgment.** Only the human decider decides. You may record a decision
  the decider has explicitly stated (quote it in the PR description). You may create and edit *Proposed* ADRs.
- Do not rewrite the body of an Accepted ADR; supersede it with a new ADR instead (typo fixes are fine).
- **Every file in the repo describes the present, never its history** ([ADR-0021](docs/adr/0021-documents-describe-present-state.md)).
  No changelog sections, no "previously / in the first draft / old proposal" wording, no `[x]` resolved items in Open Questions —
  fold resolved answers into the body. Explain *why it changed* in the commit message and PR description instead.
- Create ADRs with `scripts/task.sh docs:adr-new`, which picks a number no open PR uses; list them with `scripts/agentkit.sh adr list`.
- Breaking changes ([ADR-0087](docs/adr/0087-schema-changes-after-launch.md)): before `v0.1.0` the DB may be recreated and nothing keeps compat shims.
  From `v0.1.0`, every schema change must upgrade the previous `main` without losing data (drop in a later minor release, rewrite data with `my db upgrade`);
  config keys and internal interfaces may still change in v0.x with release notes, and the client API follows [ADR-0090](docs/adr/0090-third-party-oauth-apps.md) §8.
  ActivityPub wire formats and federated URIs must always stay spec-compliant.
- Do not add Mastodon *client* API compatibility code ([ADR-0007](docs/adr/0007-client-api.md)). Federation with Mastodon servers is still required.
- Language: design docs in Japanese during v0, every other document in English ([Documentation](#documentation));
  code, identifiers, code comments, commit messages, and PR titles in English.
- Commits follow Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`, `refactor:`, `test:`; `!` for breaking).
- License is AGPL-3.0-only ([ADR-0011](docs/adr/0011-license.md)). New source files start with `SPDX-License-Identifier: AGPL-3.0-only`;
  only add dependencies with AGPL-compatible licenses.
  The exception is MIT ([ADR-0093](docs/adr/0093-api-client-library-mykit.md) §2): `api/spec/`, `api/openapi.yaml`, `pkg/mykit/` and `pkg/mybot/`
  start with `SPDX-License-Identifier: MIT` (`# ...` in YAML) and depend only on permissive licenses (MIT, Apache-2.0, BSD, ISC and the like), never GPL-family ones.
  The generated code of the server and the web client stays `AGPL-3.0-only`. `scripts/task.sh api:license-scope` checks the identifiers.
- Links from issues and comments to repository files use `blob/main/...` or a commit permalink, never a feature branch
  (branches are deleted on merge). Before merge, link the PR instead.
- Do not commit secrets, `.env` files, or credentials. Do not run destructive commands against remote resources.

## Documentation

- **English only, no Japanese copy**: documents for users, operators, and contributors — `README.md`, `CONTRIBUTING.md`,
  `docs/README.md`, `docs/operations/`, `docs/reference/`, `docs/agents/`, `schema/README.md`, the comments of `deploy/`,
  and the issue and PR templates. Write English prose, not a translation; UI terms match `locales/en`.
  Japanese is fine inside code (an example of a `ja` string).
- **Japanese during v0**: design documents — `docs/adr/`, `docs/requirements/`, `docs/architecture/`.
- **User and operator documents explain the behaviour themselves** (`README.md`, `docs/operations/`, `docs/reference/`
  except the contributor document `commands.md`, and user-facing text such as CLI help): no links to or citations of ADRs, `docs/requirements/`, `docs/architecture/`, or
  GitHub issues and PRs (`ADR-0001`, `FR-101`, `#123`). Link only the other user and operator documents a reader needs.
  Contributor documents may cite the ADR a rule comes from.
- `scripts/task.sh docs:rules` (`agentkit docs lint`, part of `docs:check`) enforces these rules, that relative links and
  their `#anchors` resolve, and that `docs/README.md` links every file of `docs/operations/` and `docs/reference/`.
  [docs/lint.yaml](docs/lint.yaml) names the files each rule covers and the files not yet converted; that list only
  shrinks — when you add or rewrite a section of a listed file, convert the whole file and remove it from the list.
- **User and operator documents stay true to the code.** The `userdocs` of each entry of
  [tools/agentkit/context-map.yaml](tools/agentkit/context-map.yaml) name the documents that describe its paths; every
  file of `docs/operations/` and `docs/reference/` is the `userdocs` of some entry (`agentkit map check`).
  A change to those paths updates each document it makes wrong, in the same PR. `agentkit docs impact` (also printed by
  `agentkit check`) lists the documents of the change; the PR's **Documentation** section lists each one as changed, or as
  `- <path>: no change — <reason>` with a reason of three words or more. `agentkit pr open --doc-reason '<path>=<reason>'`
  writes the section, and the PR's preflight (`docs:pr-impact`) fails while a document is neither.
- **Every user-visible name is documented**: each command of `my`, metric, webhook event type, build tag, and listed task
  appears in its document (`agentkit docs inventory`, part of `agentkit:check`), or under `inventory.exclude` of
  `docs/lint.yaml` with the reason. `docs/reference/configuration.md` is generated (`scripts/task.sh go:config-docs`).

## Design rules

These come from Accepted ADRs and are binding.

- Dependency direction ([ADR-0002](docs/adr/0002-modular-monolith-hexagonal.md)) is enforced by depguard rules in `.golangci.yml` (part of `scripts/task.sh check`):
  `internal/domain/<module>` imports only the standard library and its own module's domain;
  `internal/app/<module>` adds its own module's domain and other modules' `internal/app` packages (the only cross-module path)
  and the leaf library `internal/i18n`;
  `internal/interface/*` and `internal/adapter/*` never import each other or `internal/platform`;
  `internal/platform` and `cmd/` assemble everything, and nothing imports `cmd/`.
  Never weaken a rule to make an import pass; restructure the code instead.
- Core (domain + application) must not import cloud SDKs or drivers; those live in adapters and `cmd/` entrypoints.
- Keep the server buildable with `CGO_ENABLED=0`; a cgo dependency needs an ADR.
- Every external resource goes through a port; every adapter must pass that port's contract test suite.
- Data modeling follows [ADR-0020](docs/adr/0020-data-modeling-principles.md): classify every table as resource / event / projection / current-value;
  never encode state as nullable timestamp columns; record state changes as events and derive current state in projections owned by one module;
  no JSON for queried or constrained data. Read [docs/architecture/data-model/](docs/architecture/data-model/README.md) before touching persistence.
- Schema changes: edit only the desired-state files `schema/<dialect>/<NN>-<module>.sql` (the owning module's file, all dialects together;
  [schema/README.md](schema/README.md)) and apply them with Atlas
  Community Edition; never hand-write migration SQL or rely on Atlas features that need `atlas login` ([ADR-0023](docs/adr/0023-declarative-schema-management.md)).
- Keep DB transactions short, retryable, and free of external I/O (HTTP, queue, blob); enqueue jobs via the outbox. Make writes safe to repeat with unique constraints, upserts, idempotency keys ([ADR-0003](docs/adr/0003-database-abstraction.md)).
- Job handlers must be idempotent; delivery is at-least-once ([ADR-0004](docs/adr/0004-job-queue-abstraction.md)).
- Outbound HTTP to destinations that remote servers or users can influence (federation fetch and delivery, WebFinger,
  media proxy, link previews, Web Push, ADR-0019 §1) goes through the SSRF-guarded `kernel.HTTPClient` port;
  `http.DefaultClient` is banned by lint. Cloud SDK clients that call the provider APIs the operator configured
  (AWS, Google Cloud) live in adapters and use the SDK's own HTTP client.
- Visibility checks go through the single domain policy function; never re-implement them in API or federation code.
  Levels are `public` / `local` / `unlisted` / `private` / `direct`; `local` is never federated beyond the author's followers ([ADR-0017](docs/adr/0017-visibility-local.md)).
- **i18n**: never hard-code user-facing strings or date/number formats. Add ICU messages to `locales/en` *and* `locales/ja`,
  run `scripts/task.sh i18n:gen`, and format them through the generated typed accessors (`internal/i18n` in Go);
  use CSS logical properties; render user content with `dir="auto"` ([ADR-0009](docs/adr/0009-internationalization.md)).
  Namespaces other than `client*` are formatted by Go too and must avoid rich-text tags, number skeletons, and `date` / `time` arguments.

## Context and commands

Use agentkit (`scripts/agentkit.sh <command>`, [tools/agentkit/README.md](tools/agentkit/README.md)) before reading files:

- Start an issue with `scripts/agentkit.sh ctx <n>`: the issue, its requirements, the Decision of its ADRs, the sections it
  names, the rules of the areas it touches, and the code to start from, in one pack within a token budget.
  Hand subagents the pack's file (`.tmp/agentkit/ctx/<n>.md`) instead of letting them explore again.
- Locate before you read: `find <words>`, `outline <path>`, `show <ref>` (`ADR-0070#decision`, `data-model§4.4`, `FR-915`,
  `op:<id>`, a Go name, `path:10-40`), `refs <name>`, `trace op:<id>`. Read documents by section, never whole:
  `docs/architecture/web-client/` and `docs/architecture/data-model/` hold one file per section, listed in their `README.md`;
  `show web-client§14` and `show data-model§4.4` find the file by the section number.
- Run the workflow steps through it: `issue claim <n>`, `check`, `pr open`, `pr wait <pr>` in the background (instead of
  watching CI), `regen` after merging `main`, `issue block <n>`; orchestrators watch every PR with `pr watch`.
- When you add a directory, module, adapter, or feature, add or extend its entry in `tools/agentkit/context-map.yaml` in
  the same change; `scripts/task.sh agentkit:check` (every PR) fails otherwise.

Tasks run with go-task through `scripts/task.sh <task>` ([ADR-0080](docs/adr/0080-task-runner-go-task.md)), never a `task` on PATH;
the task names are `<area>:<task>` from `tasks/<area>.yml`. Every task and the pinned toolchain:
[docs/reference/commands.md](docs/reference/commands.md). The common ones (each prefixed with `scripts/task.sh`):
`check:changed`, `check`, `go:check`, `web:check`, `generate`, `api:generate`, `go:sqlc`, `i18n:gen`,
`docs:adr-new TITLE=... SLUG=...`, `db:check`, `web:e2e`.
