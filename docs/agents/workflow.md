# Agent workflow playbook

The step-by-step procedure an agent follows for one issue. Policy background: [ADR-0014](../adr/0014-task-management-and-agent-workflow.md).

Work runs in one of two modes:

- **Single issue**: one agent takes one issue through §1–§8.
- **Parallel batch**: an orchestrator runs several issues at once, each in its own agent and worktree.
  Each agent still follows §1–§8, with the changes of [§9](#9-parallel-batches).

## 1. Pick

```sh
gh issue list -R mycroblog/my --label agent:ready --search "no:assignee" --json number,title,milestone,labels
```

- Prefer the earliest milestone, then `priority:p0` > `p1` > `p2`.
- Read the full issue including comments: `gh issue view <N> --comments`.
- Check that every ADR the issue depends on is `Accepted` in [docs/adr/README.md](../adr/README.md). If not, comment and stop.
- Claim it: `gh issue edit <N> --add-assignee @me` and comment a one-line plan.
  `scripts/agentkit.sh issue claim <N> --plan "<one line>"` checks the conditions above (and that nothing the issue waits
  for is still open), then claims it; it refuses with the reason when a condition fails.

## 2. Understand

- Start from `scripts/agentkit.sh ctx <N>`: the issue, its requirements, the Decision of its ADRs, the sections it names,
  the rules of the areas it touches (`tools/agentkit/context-map.yaml`), and the code to start from, within a token budget.
  Read more with `show`, `find`, `refs`, and `trace` ([tools/agentkit/README.md](../../tools/agentkit/README.md)), by section, not whole files.
- Read the requirement IDs (`FR-xxx`, `NFR-xxx`) and ADRs referenced by the issue.
- If the acceptance criteria are ambiguous, ask in an issue comment, add `agent:blocked`, and stop — do not guess on scope.

## 3. Branch

```sh
git switch main && git pull --ff-only
git switch -c <type>/<N>-<short-slug>     # e.g. task/42-sqlite-adapter, spike/17-sqlc-dialects
```

## 4. Implement

- Keep the change scoped to the issue, but apply the boy scout rule ([ADR-0024](../adr/0024-boy-scout-rule.md)):
  fix broken things you find in a separate commit and list them in the PR description; file an issue for anything you cannot fix safely.
- Update docs that the change makes stale (ADR open questions, architecture overview, i18n catalogs).
- Spikes ([ADR-0022](../adr/0022-spike-outputs-outside-repository.md)):
  - Push throwaway code to a `spike/<N>-<slug>` branch only. Never open a PR to merge it.
  - Post the findings report as a comment on the issue, linking the code by commit permalink.
  - Put the conclusions into ADRs / docs in the present tense, from a separate `docs/<N>-<slug>` branch and PR, linking the report comment as evidence.

## 5. Verify

```sh
scripts/agentkit.sh check      # scripts/task.sh check:changed, printing the contexts, their results, and only the failing lines
scripts/task.sh check:changed  # the contexts changed since origin/main and their dependents, in parallel
scripts/task.sh go:check       # or one context: docs:check, go:check, i18n:check, api:check, ui:check, web:check, docker:check, agentkit:check, mykit:check
scripts/task.sh check          # every context, when the change touches shared configuration
```

Must pass. The pull request's `check` runs only the preflight (formatting, licenses, image versions, tokens); the
compile, type, lint, and test checks of the changed contexts run in the merge queue, so the local run is what keeps a
broken PR out of the queue. To run the full check on the PR itself (to look into a failure in the queue, or to check
an outside contributor's PR early), add the label `ci:full`.
If a check is wrong or flaky, fix it in a separate commit and explain in the PR.

Pull requests do not run the web E2E. In single-issue work, when the change touches the web client, run `scripts/task.sh web:e2e`
locally, or run the workflow on the PR's branch and wait for it (in a parallel batch, see [§9](#9-parallel-batches) instead):

```sh
gh workflow run web-e2e.yml -R mycroblog/my --ref <branch>   # add -f matrix=full for the full width × locale matrix
gh run list -R mycroblog/my --workflow web-e2e.yml --branch <branch> --limit 1
```

### What CI runs where

The layout of [ADR-0079](../adr/0079-ci-single-gate-merge-queue.md) (`.github/workflows/`). Each job of `ci.yml` sets up its tools,
generates what its checks read, and runs the checks as parallel steps ([ADR-0083](../adr/0083-ci-steps-not-tasks.md)). Each step is named
after the one task it runs (`scripts/task.sh go:vet`), so the failed step names the check, and running that task locally reproduces it.
`docs:ci-coverage` fails when a check of `scripts/task.sh check` runs in no step:

| When | Workflow | What runs |
| --- | --- | --- |
| Pull request | `ci.yml` | `preflight`, the checks a PR's own files or lockfiles decide, so stacking PRs cannot break them: markdownlint, shellcheck, gofmt, the Go and npm licenses, the Dockerfile and `.tool-versions` versions, the Playwright image version, the design tokens, and the CSS `100vh` check; the `@my/ui` story tests (`ui-test.yml`) when the ui context changes; `lint`, golangci-lint on the lines the PR changes (`only-new-issues`), when Go or `tools/agentkit` changes; `docker-smoke`, the Docker Compose smoke test, when the docker context changes (the images are what it checks: the Go tests already assemble and start the server); and `db-lint`, `db:lint` of both dialects against the PR's base, when the schema changes: it fails on a destructive schema change unless the PR has the `db:destructive` label ([schema/README.md](../../schema/README.md#changing-the-schema)). The required `check` collects the results. A PR labelled `ci:full` also runs the merge queue's jobs; adding or removing the label reruns the checks |
| Merge queue | `ci.yml` | The full check on the merge group (the PR on top of `main` and of the PRs queued ahead of it), one job per kind of check, each only when the group changes its contexts (`scripts/changed-contexts.sh`): `server-static` (tidy, vet, the build, the generated code Go reads, the schema of both dialects, and `db:lint` against the group's base, which checks only that the schema applies on top of it, not the label) and `server-test` (the Go tests, PostgreSQL on pgmem) for go, schema, api, and i18n; `client-static` (tsc, Oxlint, knip, the Vite build, the i18n catalogs) and `client-test` (Vitest in 2 shards) for ui, web, api, and i18n; `repo` (agentkit's checks but its lint, its context map, the ADR numbers, `docs:ci-coverage`) always. At most 6 jobs; a skipped job counts as passed. golangci-lint and the build tag sets do not run here: stacking rarely adds a lint issue and never breaks the build, and the tag sets break only when adapters or tags change; the slow suite runs both. The queue merges the group when `check` passes |
| Push to `main` | `main.yml` | No checks: only the caches that pull requests and the queue restore ([ADR-0079](../adr/0079-ci-single-gate-merge-queue.md)). The Go cache holds exactly what the jobs that restore it read, built one after another with their settings (`CGO_ENABLED=0`, which setup-go sets everywhere): the modules, the builds of the packages and the Go tools, the test binaries, go vet's results, agentkit's builds, and golangci-lint's analysis (run without failing on issues). It is built from empty directories, keyed on the Go version, `go.sum`, `tools/go.sum`, and the date; the Bun caches are keyed on the lockfiles. Each is built only when no cache of its key exists yet |
| Release gate, nightly, by hand | `slow-suite.yml` | The slow suite of [ADR-0079](../adr/0079-ci-single-gate-merge-queue.md): the full web E2E matrix on 8 shards (`web-e2e.yml`), the live interoperability test (`federation-live.yml`), the PostgreSQL tests on a real container (`postgres-contract.yml`), the `stream.Bus` contracts on broker containers (`stream-contract.yml`), the `mail = smtp` integration tests on Mailpit (`mail-integration.yml`), the smoke test and the arm64 builds (`docker:smoke`, `docker:build-arm64`), the upgrade test (`docker:upgrade-test`: a database of each dialect made and filled by the previous minor version's latest patch, upgraded with the commit's images; skipped when no previous minor version exists; [releasing.md](../operations/releasing.md#the-upgrade-test)), golangci-lint over everything with the build tag sets (`go:lint`, `agentkit:lint`, `mykit:lint`, `mykit:mybot-lint`, `go:build-tags`), and CodeQL code scanning (`codeql.yml`; the results go to the Security tab). Its last job, `result`, fails when any of them failed. Never on pull requests or merge groups, and not part of `check` |
| Release (by hand) | `release.yml` | The slow suite on the dispatched commit, whose upgrade test starts from the previous minor version of the released version; only when it passes, the tag and a draft GitHub Release ([releasing.md](../operations/releasing.md)) |
| Nightly (03:00 JST) | `nightly.yml` | The slow suite on the head of `main`, whose upgrade test starts from the latest release, skipped when `main` has not moved since the last successful Nightly run |

`slow-suite.yml`, `nightly.yml`, `web-e2e.yml`, `federation-live.yml`, `postgres-contract.yml`, `stream-contract.yml`,
`mail-integration.yml`, and `codeql.yml` also run by hand on any branch (`gh workflow run <file> --ref <branch>`). The web E2E subset (`web-e2e.yml` with
`matrix: pr`, the default) runs only by hand.

## 6. Commit & PR

- Conventional Commits in English; reference the issue (`feat(db): add sqlite adapter (#42)`).
- Push the branch and open a PR with the template:

```sh
git push -u origin HEAD
gh pr create --fill --body-file - <<'EOF'
...template contents...
EOF
```

- Comment on the issue with the PR link. Do not merge or queue it yourself; the orchestrator or the decider queues it (§7).
  `scripts/agentkit.sh pr open [--summary-file f] [--decisions text] [--confirm]` pushes, fills the template from the issue
  and the commits, and comments the link on the issue.
- Wait for the PR's CI and fix failures until it is green: run `scripts/agentkit.sh pr wait <PR>` in the background. It
  prints one line when CI passes, or the failing lines of the failed jobs, and exits 3 when the PR conflicts.

## 7. Merge through the merge queue

`main` takes pull requests only through GitHub's merge queue. CI runs the full check on each merge group (the PR on top
of `main` and of the PRs queued ahead of it) and the queue merges the group when its required `check` passes.

- Once the PR's CI is green, the orchestrator or the decider queues it: `gh pr merge <PR> -R mycroblog/my --auto --merge`.
- Merge `main` into the branch only when the PR conflicts or the queue drops it, never just to stay current: the queue
  already checks the PR on top of the latest `main`.
- When the queue drops the PR (a conflict with `main`, or a failed `check` on the merge group), merge `main` into the
  branch, regenerate the generated files instead of resolving their conflicts by hand, run the checks, and push;
  the orchestrator or the decider queues it again. Only the generated files Go reads are committed (the client API
  bundle and Go code, sqlc output, the Go side of i18n, the theme token list, the configuration reference); the ones
  only the TypeScript clients read are not, so they never conflict ([ADR-0081](../adr/0081-generated-files-policy.md)):

```sh
git fetch origin && git merge origin/main
scripts/agentkit.sh regen              # runs the generators whose sources changed: api:generate, go:sqlc, i18n:gen,
                                       # web:tokens-gen, go:config-docs
scripts/agentkit.sh check
```

## 8. When blocked

- Comment on the issue: what you tried, what decision is needed, the options you see, and your recommendation.
- `gh issue edit <N> --add-label agent:blocked` (`scripts/agentkit.sh issue block <N> --reason-file f` does both)
- If the decision is architectural, draft a Proposed ADR in the same branch and link it.

## 9. Parallel batches

When many changes land at once, running the web E2E on every pull request costs time and runners without making the
result more accurate: the PRs keep changing the same areas, so a per-PR run checks a state that `main` never has, and
a spec that passes alone can still break elsewhere. Waiting for the web E2E on `main` after each group of PRs stalls
the agents for the same reason. Parallel work therefore runs as a rolling queue: each PR is checked by the `check`
gate only, the web E2E runs nightly on `main` without holding up the queue, and its failures are fixed together
once the milestone's implementation issues are done. Single-issue work keeps the per-PR web E2E of §5.

### Planning (orchestrator)

- Keep about 4–5 agents running, capped by the load of the machine. Whenever an agent finishes and a slot frees,
  start the next issue; there is no barrier between groups of issues.
- For each candidate issue, list the shared files it is likely to touch:
  - the sources of committed generated code: `api/spec/` (its root `api/spec/openapi.yaml` and the files of the paths and components),
    the area files of `locales/<locale>/<namespace>/` (a key goes in the file of its area: its first segment, or its first two under
    `admin`, such as `client/settings.json` and `client/admin/report.json`; a new area is a new file), the module files of
    `schema/<dialect>/` and the sqlc queries, the config schema;
  - the tables many changes edit: the FR table in
    [01-functional.md](../requirements/01-functional.md), the files under `docs/architecture/`. In
    `docs/architecture/web-client/` and `docs/architecture/data-model/`, each section is a file of its own: two issues
    that edit different sections do not share a file.
- Never run two issues that touch the same shared files at the same time: when the next issue in line shares files
  with a running one, start a different issue for the free slot, and start the held issue after the running one has
  opened its PR, on that PR's branch (a stacked PR: `--base <that branch>`, "Stacked on #N" at the top of the body;
  GitHub retargets it to `main` when the base merges, and the agent then merges `origin/main`).
- Split implementation work by layer: one issue for the server (domain, application, adapters, schema, client API)
  and one for the web client (screens, `client*` messages), the web issue stacked on the server PR. A smaller issue
  reads and checks less.
- Pick the agent's model by the kind of work: the default model for spikes, new ADRs, and implementation; a lighter
  model (`model: "sonnet"`) for mechanical work, such as folding the decider's answers into an ADR, editing issue
  bodies, docs-only changes, and resolving a conflict by regenerating files.
- Hand each agent the issue's context pack (`scripts/agentkit.sh ctx <N> --layer server|web --quiet` writes
  `.tmp/agentkit/ctx/<N>.md`) so that it does not explore again.
- Ask every agent for a final report of three items only: the PR number, the CI status, and the decisions needed.
  Everything else belongs in the PR description.

### Agents in the queue

- The PR gate is `scripts/task.sh check:changed` locally and the PR's `check` CI, which runs only the preflight: the
  local run is the one that catches compile, type, and test failures before the merge queue. Do not run the web E2E per PR (`scripts/task.sh web:e2e`
  or `web-e2e.yml`), not even the specs the change adds: write and update the E2E specs the change needs, but do not
  run the suite.
- Before waiting on CI, check that the PR is mergeable (`gh pr view <PR> -R mycroblog/my --json mergeable`).
  CI does not start on a conflicting PR; merge `origin/main` into the branch first, as in §7.
- Resolve conflicts in the committed generated files by regenerating them (§7), never by hand.
- Once the PR is open and its CI is green, report the PR number, the CI status, and the decisions needed (nothing else) to the orchestrator and finish. Never queue or merge the PR.

### Merging (orchestrator)

- Enable auto-merge as soon as a PR is opened (`gh pr merge <PR> -R mycroblog/my --auto --merge`); the merge queue
  takes it when its `check` is green.
- Watch every open PR with one watcher for three events: merged, conflicting, and CI failure: `scripts/agentkit.sh pr watch`
  under the Monitor tool prints one line per event (also opened, ci-green, and dequeued).
- A conflict drops the PR from the queue and turns off its auto-merge. Have the agent that owns the PR merge `main`
  into the branch, then enable auto-merge again.
- Never remove a worktree that an agent still works in.
- Work that follows a merged PR goes to a new agent, on a new branch.

### Failures on `main` (orchestrator)

- Failures of the `check` gate, the Go tests, or the merge queue are fixed at once. The merge queue keeps them off
  `main` by design; when a build or a test on `main` still breaks, fix it before starting more issues.
- The slow suite (the full web E2E, the real PostgreSQL and broker contracts, the live federation test, the images,
  and CodeQL) runs nightly on `main` ([ADR-0079](../adr/0079-ci-single-gate-merge-queue.md)) and does not pause the
  queue. Collect its failures from the Nightly runs as they appear
  (`gh run list -R mycroblog/my --workflow nightly.yml --status failure`); a night on which `main` did not move has no run.
- Once the milestone's implementation issues are done, run the slow suite on `main`
  (`gh workflow run slow-suite.yml -R mycroblog/my --ref main`), group the collected failures by cause, file one
  issue per cause, and fix them together.
- A release needs the slow suite green: `release.yml` tags nothing while any of it fails
  ([releasing.md](../operations/releasing.md)). Fix every failure before cutting a release.

### Recording decisions

When the decider answers a question in chat, quote the answer verbatim on the PR or issue it concerns, so that the
decision stays with the work it governs.
