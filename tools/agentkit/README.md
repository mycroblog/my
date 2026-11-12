# agentkit

Context and workflow tools for coding agents working in mycroblog/my. Agents get the parts of the documents,
code, and issues a task needs, and run the workflow steps of [/work-on-issue](../../.claude/skills/work-on-issue/SKILL.md),
at a fraction of the tokens that grep, cat, and polling cost. Decisions: [docs/adr/](docs/adr/README.md).

Run it through `scripts/agentkit.sh <command>`, which builds `bin/agentkit` when it is missing or stale.
`scripts/agentkit.sh help` lists every command and flag.

## Reading

| Instead of | Run |
| --- | --- |
| Reading the issue, then every ADR, requirement, and document section it names | `ctx <issue> [--layer server\|web]`: one pack within a token budget (default 30k), also saved to `.tmp/agentkit/ctx/<n>.md` for subagents |
| `grep` / `rg` / `git grep` over the repository | `grep <pattern> [path\|glob ...] [-i] [-w] [-F] [-l] [-o] [-C N] [--kind go,ts,md,sql,yaml,json]`: a regular expression (or `-F` string) over the tracked files, without generated files, lock files, snapshots, and tests (`--tests`, `--only-tests`, `--gen`); at most 40 matches (`-n`) and 6,000 characters (`--max`), grouped by file, each with its declaration or section and line range, and a last line that counts what was left out |
| `grep "func X" -A 40`, `sed -n 'A,Bp'`, `head`, `tail` | `show <Name>` (also `Recv.Name`, `pkg.Name`, `<path>:<Name>`, `'func (s *Service) Create'`): the whole declaration; `show <path>:<from>-<to>`: the lines. The path may be relative to the working directory or any ending that names one file |
| Looking for where a topic lives | `find <words> [--kind adr,section,go,ts,op,table,query,...] [--under <path>]`: ranked locations with sizes |
| `cat` of a large document | `outline <path>`, then `show '<path>#<anchor>'`, `show 'federation.md§1'`, `show 'ADR-0070#decision'`, `show FR-915` |
| Following a request through the layers by hand | `trace op:<operationId>` or `trace <GoName>`: handler → app → adapter → query → table |
| Grepping for the users of a name | `refs <ref>`: what points at it (documents, code, queries, client calls) and what it points at |
| Listing `docs/adr/` and opening every ADR | `adr list [--status Proposed]`: number, status (the first word of its `- Status:` line), and title |

`show` prints the outline instead of the text when a node is bigger than `--max` tokens (default 12k), cuts a line
range there, and lists the candidates of a name that several declarations share (one declaration outside the tests
is shown, the test helpers of its name counted).
References: node ids (`ADR-0070`, `FR-915`, `op:<id>`, `table:<name>`, `query:<Name>`, `schema:<Name>`,
`sym:<dir>.<Recv>.<Name>`, `ts:<path>:<Name>`), short names (`SubmitRequest`, `custom_emojis`), sections by number or
anchor of any unique path suffix (`federation.md§1`, `federation#1-モデレーションとの連携`), and line ranges.
A document split into one file per section, a directory with a `README.md`, is looked up by its directory:
`<dir>§14` finds the section in whichever file holds it, and `show <dir>` prints the directory's `README.md`.

## Workflow

| Step | Run |
| --- | --- |
| Create an issue | `issue new --type task\|bug\|spike\|adr\|epic --title t --body-file f --parent <epic>` (`--milestone <title>` for an epic or an issue outside every epic; `--blocked-by <n>` for order): the only way, since the hook denies `gh issue create`. It links the sub-issue, gives it the epic's milestone, and refuses an epic whose description lists issues; the type, `--priority` and `--area` are GitHub's issue type and issue fields, not labels |
| Take an issue (§1) | `issue claim <n> --plan "<one line>"`: refuses an epic or an ADR issue, and an issue that has no milestone, is assigned to someone else, depends on a non-Accepted ADR, or waits for an open issue (a decision, an investigation, or work) |
| Verify | `check`: the quick checks of the changed files, one line each and the failing lines — `gofmt`, `go vet` of the changed packages, `bun fmt`, `tsc` and Oxlint, `i18n:lint`, `docs:lint` and `docs:rules`, shellcheck, `tofu fmt` of the changed OpenTofu files, the checks of the OpenTofu modules that compare files when the tofu context changed (`tofu:env-check`, `tofu:backend-fields`, `tofu:coverage`, `tofu:sources`), and that committed generated files are current. A step whose tool is not installed is skipped with a line. `check --full`: `scripts/task.sh check:changed` and the workers check, which the merge queue runs anyway |
| Open the PR | `pr open [--issue n]... [--summary-file f] [--decisions text] [--confirm] [--footer text] [--base <branch>]`: prints the documents of `docs impact` the change does not touch; copies the issues' acceptance criteria with their checked state. It is the gate: before it pushes (`--force-with-lease`) it refuses, with the command that fixes it, outside a worktree, on a merge commit or a `fixup!` / `squash!` commit in `origin/main..HEAD`, on a conflict with the base, and when `check` fails. On a branch whose pull request is open (an epic's draft) it rewrites that pull request's description instead of opening another. Without `--issue` it closes the issue of the branch name and the `(#N)` of the commit subjects |
| Finish the PR | `pr land <pr>` **in the background, in the main agent**: turns on auto-merge into the merge queue, prints one line per event until the merge, then removes the branch and its worktree, checks that the closed issues are closed, closes each parent issue whose sub-issues are all closed and each milestone left with no open issue, and ends with three lines (`merged:`, `cleaned up:`, `remains:`). Exit 1 when CI failed (with the failing lines), 3 on a conflict, 4 when cancelled, 5 when the merge queue dropped it (with the failing lines of its merge group), 124 on the timeout; each prints the fix |
| Clean up | `tidy [--dry-run]`: removes the local branches whose pull request is merged or closed, with their worktrees, after stopping the dev server of each (only a process inside the worktree); keeps and names those with uncommitted changes, commits the pull request does not hold, a lock, or the caller inside. The SessionStart hook prints one line when merged branches are left |
| Wait for CI only | `pr wait <pr>` **in the background**: one line when CI passes; the failing lines of the failed jobs otherwise; exit 3 on a conflict; exit 4 when checks were cancelled and no newer run replaced them; exit 6 when the head moved. Only the newest run of each workflow counts (the checks of an older run on the same head, such as the failed `check` of a run cancelled by a newer one, are ignored while the newer run runs), and of it the latest attempt of each check; and CI passes only when the required `check` gate of the CI workflow has reported and succeeded |
| Find the slow job of CI | `pr jobs <pr> [--run <id>] [-n 20]`: for the newest run of each workflow on the pull request's head (or the given run), one line with the run's wall time and its number of skipped jobs, then one line per job that ran — duration, result, name — slowest first, with the three slowest steps under a job longer than 3 minutes. It reads once and does not wait |
| Resolve a conflict | `git rebase origin/main`, `regen` (runs the generators of the changed sources, including `scripts/tofu-workers-check.sh --write` for `internal/platform/job_types.go`), `check`, `git push --force-with-lease` |
| Watch many PRs (§9) | `pr watch [<pr>...]` with the Monitor tool: one line per opened, merged, closed, conflicting, ci-failed, ci-cancelled, ci-green, dequeued |
| Stop (§8) | `issue block <n> --type adr\|spike --title t --reason-file f`, or `--on <m>` |

## On-screen feedback

The decider writes notes on the catalog page ([ADR-0168](../../docs/adr/0168-on-screen-feedback.md)) and sends them as one
batch, which the dev server writes to `.tmp/feedback/<batch>.json`.

| Step | Run |
| --- | --- |
| Start | `scripts/task.sh web:catalog` in the background, then `feedback url`, and give the decider the URL it prints. A worktree runs one: while its server answers, `web:catalog` prints that URL and starts none. With no dev server answering, it names the command that starts one and fails |
| Receive | `feedback wait` **in the background**: returns with the oldest batch no wait has delivered, so a send notifies the session; run it again after each batch |
| Read | `feedback show [<batch>\|<note>]`: each note with its source (`file:line`), layer (the package), page and state, and an outline of its accessibility tree. A page without a router or an E2E scenario records the page instead; open it with Playwright at the note's viewport |
| Answer | `feedback reply <note> [--kind fixed\|question] <text>`: the overlay shows the answer next to the note at once |
| Offer options | wrap the variants in `<Options id variants>` of `@my/devtools/options`, then `feedback reply <note> --kind options --options <id> <text>`: the decider switches between them in the overlay, and the pick arrives as a note, with the decider's reason when they gave one. Keep the picked variant and remove the `<Options>`, which a production build refuses |

## The context map

A `context-map.yaml` describes its directory as a whole, down to the next one below: the document sections that
govern it (`docs`), the user and operator documents that describe it (`userdocs`), its check contexts (`checks`), and
whether parallel work treats it as shared files (`shared`). `docs`, `userdocs`, and `checks` add to those of the maps
above, so a file holds only what differs. The [root file](../../context-map.yaml) alone lists patterns, for the root's
files and dot-directories. `map check` fails when a path is under no map, a map repeats one above it, a reference does
not resolve, or a file of `userdocs.cover` is no map's `userdocs` ([ADR-0003](docs/adr/0003-context-map.md)).
`map show <path>` prints what describes a path.

## Documentation that changes with the code

`docs impact [--base origin/main]` lists the `userdocs` of the entries of the paths changed since the merge base,
passing over the paths of `userdocs.skip` (tests, fixtures) and the map's `generated` (tracked files the index passes over), and says whether
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
- Package pointers: an `AGENTS.md` below the root is a title and at most 12 list items, each
  `- <task>: [<documentation>](https://...), search for: <terms>` with one link that has no `#fragment`.

`scripts/task.sh docs:growth` (`scripts/docs-growth.sh`, a step of the pull request's preflight) fails when a group of
the prose agents maintain is larger than at the merge base, unless the pull request has the `allow:docs-growth` label.

`docs inventory` (`scripts/task.sh agentkit:inventory`, part of `agentkit:check`) checks the other direction: every
name that users and operators see and the code defines is mentioned in the document they look it up in.

| Inventory | Defined by | Mentioned in |
| --- | --- | --- |
| `cli` | the commands of `cmd/my`: the words its `run*` functions switch on (`switch sub`, `args[0] == "x"`) and the lists of `dispatch("admin relay", args, []string{...})` | a command line (`my admin relay add`, `my job list\|retry`) in `README.md`, `docs/operations/`, or `docs/reference/`, or a code span that starts with the last word in a document with a command line of the parent |
| `metrics` | the `Name` of each `kernel.Instrument` literal in `internal/` and `cmd/` | `docs/operations/running.md`, as a whole name |
| `webhook-events` | the constants of type `EventType` in `internal/domain/webhook` | a code span of `docs/reference/webhooks.md` |
| `build-tags` | the `no_*` tags of the `//go:build` lines of the server module | a code span of `docs/reference/build-tags.md` |
| `job-types` | the first argument of each `Register` and `RegisterWithPolicy` call in `internal/` and `cmd/`, other than the events of the projectors; a constant is resolved through its package, and an argument that cannot be resolved fails the check | a code span of `docs/reference/jobs.md` |
| `job-schedules` | the `Name` of each `queue.Schedule` literal; of a concatenation, its first part | a code span under `## Periodic jobs` of `docs/reference/jobs.md` |

`inventory.exclude.<inventory>` of `docs/lint.yaml` leaves a name out, with the reason in a comment; an exclusion
fails once the name is mentioned or no longer defined. `docs/reference/configuration.md` needs no inventory: it is
generated from the config schema, and `go:test` fails when it is stale.

The first sentences of `docs/reference/jobs.md` state how many job types and periodic jobs there are; the inventory also
compares those numbers with the names it collected, and fails when the sentence is gone.

## Claude Code hooks

`.claude/settings.json` runs `scripts/agentkit-hook.sh` ([ADR-0004](docs/adr/0004-claude-code-hooks.md)): at session
start it builds and indexes, and it cuts Bash output over 20,000 characters to its head and tail, saving the whole to
`.tmp/agentkit/out/`. Before a Bash or Read call it denies the following, for every agent, and the reason names the
command to run instead, filled in from the denied one:

| Denied | Use instead |
| --- | --- |
| `grep` / `rg` / `ag` / `git grep` over files of the repository | `grep`, or `show <Name>` for a declaration |
| `sed -n 'A,Bp'`, `head`, `tail`, `awk 'NR>=A'` on a file of the repository, `cat <file> \|` | the Read tool with offset and limit, `show <path>:A-B`, `outline` |
| `sed -i`, `perl -pi`, a `python3` / `node` script that writes a file of the repository | the Edit tool; `scripts/i18n.sh add\|mv\|rm` for `locales/` |
| `git merge` of `main`, `git pull` without `--rebase` or `--ff-only` | `git fetch origin && git rebase origin/main`, `regen`, `git push --force-with-lease` |
| `gh pr checks`, `gh run watch`, `gh run view` without `--log-failed`, `gh run list` without `--workflow`, `gh api` of check runs and workflow runs, `statusCheckRollup` | `pr wait <pr>` in the background; `pr jobs <pr>` for the jobs of a run and their durations |
| a loop that sleeps, `pgrep`, a `sleep` over a second, also inside `bash -c` | end the turn: `run_in_background`, a subagent's result, and a message each notify |
| reading a whole Markdown file of 40KB or more (code: 80KB) | `outline`, then `show` |

It splits a compound command at `;`, `&&`, `||`, `|` and newlines outside quotes and here-documents, and follows `cd`
and the variables the command sets. It leaves alone: grep as a filter on a pipe, files outside the working tree, files
git ignores (`.tmp/`, `node_modules/`, logs), a path the shell computes, `grep -q`, a search of a revision, a merge with
`--ff-only` or of another branch, and anything it cannot parse. `AGENTKIT_BIG_FILE` and `AGENTKIT_MAX_OUTPUT` change the limits.

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
- `internal/kb` answers `find` / `show` / `outline` / `refs` / `trace`; `internal/grep` searches the working tree;
  `internal/pack` builds `ctx`; `internal/ops` runs the workflow steps; `internal/hook` implements the hooks; `internal/stats` measures; `internal/docslint`
  checks the documentation rules.

## Developing

`scripts/task.sh agentkit:check` runs `go mod tidy -diff`, `go vet`, golangci-lint (`.golangci.yml` here), the tests, the
dependency check (only module versions the server's `go.sum` has, so `scripts/task.sh go:licenses` covers their licenses), and
`map check`. Bump `extract.Version` when an extractor's output changes, so that indexes are rebuilt.

`cmd/releasenotes` is not part of agentkit: it is the reader behind `scripts/release-notes.sh`, in this module for its YAML parser,
and is tested with that script by `scripts/task.sh shell:test`. `cmd/releaseprose` is that script's step that asks Claude for the
prose around those facts and checks it; its tests here use a fake model, and the script's tests a fake `claude`.
