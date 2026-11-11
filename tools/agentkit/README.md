# agentkit

Context and workflow tools for coding agents working in mycroblog/my. Agents get the parts of the documents,
code, and issues a task needs, and run the workflow steps of [docs/agents/workflow.md](../../docs/agents/workflow.md),
at a fraction of the tokens that grep, cat, and polling cost. Decisions: [docs/adr/](docs/adr/README.md).

Run it through `scripts/agentkit.sh <command>`, which builds `bin/agentkit` when it is missing or stale.
`scripts/agentkit.sh help` lists every command and flag.

## Reading

| Instead of | Run |
| --- | --- |
| Reading the issue, then every ADR, requirement, and document section it names | `ctx <issue> [--layer server\|web]`: one pack within a token budget (default 30k), also saved to `.tmp/agentkit/ctx/<n>.md` for subagents |
| `grep -rn` over the repository | `find <words> [--kind adr,section,go,ts,op,table,query,...] [--under <path>]`: ranked locations with sizes |
| `cat` of a large document | `outline <path>`, then `show '<path>#<anchor>'`, `show 'data-model§4.4'`, `show 'ADR-0070#decision'`, `show FR-915`, `show <path>:<from>-<to>` |
| Following a request through the layers by hand | `trace op:<operationId>` or `trace <GoName>`: handler → app → adapter → query → table |
| Grepping for the users of a name | `refs <ref>`: what points at it (documents, code, queries, client calls) and what it points at |
| Listing `docs/adr/` and opening every ADR | `adr list [--status Proposed]`: number, status (the first word of its `- Status:` line), and title |

`show` prints the outline instead of the text when a node is bigger than `--max` tokens (default 12k).
References: node ids (`ADR-0070`, `FR-915`, `op:<id>`, `table:<name>`, `query:<Name>`, `schema:<Name>`,
`sym:<dir>.<Recv>.<Name>`, `ts:<path>:<Name>`), short names (`SubmitRequest`, `custom_emojis`), sections by number or
anchor of any unique path suffix (`data-model§4.4`, `web-client#5-アクセシビリティ`), and line ranges.
A document split into one file per section (`docs/architecture/web-client/`, `docs/architecture/data-model/`) is
looked up by its directory: `web-client§14` finds the section in whichever file holds it, and `show web-client`
prints the directory's `README.md`.

## Workflow

| Step | Run |
| --- | --- |
| Take an issue (§1) | `issue claim <n> --plan "<one line>"`: refuses when the issue is not `agent:ready`, needs a decision, is assigned to someone else, depends on a non-Accepted ADR, waits for an open issue, or has a dependency whose state cannot be read |
| Verify | `check`: the quick checks of the changed files, one line each and the failing lines — `gofmt`, `go vet` of the changed packages, `bun fmt`, `tsc` and Oxlint, `i18n:lint`, `docs:lint` and `docs:rules`, shellcheck, and that committed generated files are current. `check --full`: `scripts/task.sh check:changed` and the workers check, which the merge queue runs anyway |
| Open the PR | `pr open [--issue n]... [--summary-file f] [--decisions text] [--confirm] [--footer text] [--base <branch>]`: prints the documents of `docs impact` the change does not touch; copies the issues' acceptance criteria with their checked state. It is the gate: before it pushes (`--force-with-lease`) it refuses, with the command that fixes it, outside a worktree, on a merge commit or a `fixup!` / `squash!` commit in `origin/main..HEAD`, on a conflict with the base, when `check` fails, and, for a change under `app/web/src` or `app/ui/src` other than tests and stories, unless a screenshot of `scripts/task.sh web:shot` is newer than the change and was opened with the Read tool since (the PostToolUse hook records the reads in `.tmp/agentkit/seen.json`); it lists those under `## Screens`. Without `--issue` it closes the issue of the branch name and the `(#N)` of the commit subjects |
| Finish the PR | `pr land <pr>` **in the background, in the main agent**: turns on auto-merge into the merge queue, prints one line per event until the merge, then removes the branch and its worktree, checks that the closed issues are closed, closes each parent issue whose sub-issues are all closed, and ends with three lines (`merged:`, `cleaned up:`, `remains:`). Exit 1 when CI failed (with the failing lines), 3 on a conflict, 4 when cancelled, 5 when the merge queue dropped it (with the failing lines of its merge group), 124 on the timeout; each prints the fix |
| Clean up | `tidy [--dry-run]`: removes the local branches whose pull request is merged or closed, with their worktrees; keeps and names those with uncommitted changes, commits the pull request does not hold, a lock, or the caller inside. The SessionStart hook prints one line when merged branches are left |
| Wait for CI only | `pr wait <pr>` **in the background**: one line when CI passes; the failing lines of the failed jobs otherwise; exit 3 on a conflict; exit 4 when checks were cancelled and no newer run replaced them. Only the newest run of each workflow counts (the checks of an older run on the same head, such as the failed `check` of a run cancelled by a newer one, are ignored while the newer run runs), and of it the latest attempt of each check; and CI passes only when the required `check` gate of the CI workflow has reported and succeeded |
| Resolve a conflict | `git rebase origin/main`, `regen` (runs the generators of the changed sources, including `scripts/tofu-workers-check.sh --write` for `internal/platform/job_types.go`), `check`, `git push --force-with-lease` |
| Watch many PRs (§9) | `pr watch [<pr>...]` with the Monitor tool: one line per opened, merged, closed, conflicting, ci-failed, ci-cancelled, ci-green, dequeued |
| Stop (§8) | `issue block <n> --reason-file f` |

## The context map

Each `context-map.yaml` maps the paths of its directory (patterns relative to it) to the document sections that govern
them (`docs`), the user and operator documents that describe them (`userdocs`), their check contexts, and whether
parallel work treats them as shared files; the [root file](../../context-map.yaml) also holds `ignore`, `strict`, and
`userdocs.cover` / `skip`. A new module, adapter, or feature adds a file in its own directory.
`scripts/task.sh agentkit:check` (`map check`) fails when a file is not covered, when a directory under a `strict`
pattern holds no `context-map.yaml`, when a `docs` or `userdocs` reference does not resolve, or when a file of
`userdocs.cover` is the `userdocs` of no entry ([ADR-0003](docs/adr/0003-context-map.md)). `ctx` uses the map to pick
the documents of the areas an issue touches; `map show <path>` prints the entries of a path.

## Documentation that changes with the code

`docs impact [--base origin/main]` lists the `userdocs` of the entries of the paths changed since the merge base,
passing over the paths of `userdocs.skip` (tests, fixtures) and the map's `ignore` (generated files), and says whether
the change touches each. `check` and `pr open` print the ones it does not touch; nothing fails on them.

## Documentation rules

`docs lint` (`scripts/task.sh docs:rules`, part of `docs:check`) checks the documentation rules; each finding says the fix.
[docs/lint.yaml](../../docs/lint.yaml) names the files each rule covers and, under `pending`, the files that do not
follow a rule yet: a pending entry fails once its file follows the rule, so the lists only shrink.

- Links: every relative link and `#anchor` of a Markdown file resolves.
- Language: the English-only files have no Japanese outside code (`docs-lint: allow-japanese` on the line or the one
  before allows it).
- Operator documents: no link to or citation of design records, issues, or pull requests; none of the
  `maintainer-terms`; no opening that only names the audience; the index links each of them.
- ADR citations: each `ADR-NNNN` and `§k` resolves, and is not a number that `docs/adr/aliases.yaml` retired into
  another ADR (`show`, `find`, `refs`, `ctx`, and `issue claim` follow that file).
- ADR shape: an ADR has a Decision section, no quotations, at most 6,000 characters (4,000 in Decision), no table of
  more than 8 rows, and no code span that names a path of three segments or more, a source or YAML file, a namespaced
  task, a `pkg.Exported` of a package of the repository, or an `Err...` value. `docs adr-shape` prints the measures of
  each ADR.

`scripts/task.sh docs:growth` (`scripts/docs-growth.sh`, a step of the pull request's preflight) fails when a group of
the prose agents maintain is larger than at the merge base, unless the pull request has the `docs:grow` label.

`docs inventory` (`scripts/task.sh agentkit:inventory`, part of `agentkit:check`) checks the other direction: every
name that users and operators see and the code defines is mentioned in the document they look it up in.

| Inventory | Defined by | Mentioned in |
| --- | --- | --- |
| `cli` | the commands of `cmd/my`: the words its `run*` functions switch on (`switch sub`, `args[0] == "x"`) and the lists of `dispatch("admin relay", args, []string{...})` | a command line (`my admin relay add`, `my job list\|retry`) in `README.md`, `docs/operations/`, or `docs/reference/`, or a code span that starts with the last word in a document with a command line of the parent |
| `metrics` | the `Name` of each `kernel.Instrument` literal in `internal/` and `cmd/` | `docs/operations/running.md`, as a whole name |
| `webhook-events` | the constants of type `EventType` in `internal/domain/webhook` | a code span of `docs/reference/webhooks.md` |
| `build-tags` | the `no_*` tags of the `//go:build` lines of the server module | a code span of `docs/reference/build-tags.md` |
| `tasks` | the tasks with a `desc` (what `scripts/task.sh --list` lists) of `Taskfile.yml` and its includes | a code span of `docs/reference/commands.md` that starts with the name |
| `job-types` | the first argument of each `Register` and `RegisterWithPolicy` call in `internal/` and `cmd/`, other than the events of the projectors; a constant is resolved through its package, and an argument that cannot be resolved fails the check | a code span of `docs/reference/jobs.md` |
| `job-schedules` | the `Name` of each `queue.Schedule` literal; of a concatenation, its first part | a code span under `## Periodic jobs` of `docs/reference/jobs.md` |

`inventory.exclude.<inventory>` of `docs/lint.yaml` leaves a name out, with the reason in a comment; an exclusion
fails once the name is mentioned or no longer defined. `docs/reference/configuration.md` needs no inventory: it is
generated from the config schema, and `go:test` fails when it is stale.

The first sentences of `docs/reference/jobs.md` state how many job types and periodic jobs there are; the inventory also
compares those numbers with the names it collected, and fails when the sentence is gone.

## Claude Code hooks

`.claude/settings.json` runs `scripts/agentkit-hook.sh` ([ADR-0004](docs/adr/0004-claude-code-hooks.md)): at session
start it builds and indexes; it denies reading a whole Markdown file of 40KB or more (code: 80KB) and watching CI with
`gh pr checks --watch` / `gh run watch`, and polling for a background command with a loop that sleeps or with
`pgrep`, with what to use instead; it hints at `find` for unbounded
recursive greps; and it cuts Bash output over 20,000 characters to its head and tail, saving the whole to
`.tmp/agentkit/out/`. `AGENTKIT_BIG_FILE` and `AGENTKIT_MAX_OUTPUT` change the limits.

## Measuring

`stats [--since 2026-10-01] [--until ...]` reads the Claude Code transcripts of this repository and its worktrees
(`~/.claude/projects/`) and prints the API token use and the tool output by kind. Compare the periods before and after
a change to see what it saved.

## How it works

- `internal/extract` turns each file into nodes (Markdown sections, requirement rows, Go and TypeScript declarations,
  tables, sqlc queries, OpenAPI operations and components, messages) and edges (links, ID mentions, code spans, imports,
  calls by name, API calls by path, tables a query uses). Generated files are left out.
- `internal/store` keeps them in SQLite (FTS5 with the trigram tokenizer, so Japanese needs no morphological analysis)
  at `$(git rev-parse --git-common-dir)/agentkit/kb.db`, keyed by path and blob id: worktrees share it, and each command
  indexes only what changed (the first build takes seconds, later ones tens of milliseconds).
- `internal/gh` talks to GitHub through `gh` and caches issues and merged pull requests (with their changed files) in
  the same database.
- `internal/kb` answers `find` / `show` / `outline` / `refs` / `trace`; `internal/pack` builds `ctx`; `internal/ops`
  runs the workflow steps; `internal/hook` implements the hooks; `internal/stats` measures; `internal/docslint`
  checks the documentation rules.

## Developing

`scripts/task.sh agentkit:check` runs `go mod tidy -diff`, `go vet`, golangci-lint (`.golangci.yml` here), the tests, the
dependency check (only module versions the server's `go.sum` has, so `scripts/task.sh go:licenses` covers their licenses), and
`map check`. Bump `extract.Version` when an extractor's output changes, so that indexes are rebuilt.

`cmd/releasenotes` is not part of agentkit: it is the reader behind `scripts/release-notes.sh`, in this module for its YAML parser,
and is tested with that script by `scripts/task.sh shell:test`. `cmd/releaseprose` is that script's step that asks Claude for the
prose around those facts and checks it; its tests here use a fake model, and the script's tests a fake `claude`.
