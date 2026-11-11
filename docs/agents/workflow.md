# Agent workflow

How a goal goes from issues to `main` ([ADR-0000](../adr/0000-record-architecture-decisions.md) §5,
[ADR-0079](../adr/0079-ci-single-gate-merge-queue.md)). The commands refuse what must not pass and say the fix, so
this page only says what to do in which order and where judgment is needed.

## The goal and its branch

The unit of delivery is a goal: an epic, a phase of one, or a milestone. One issue is the small case.

- Take only issues that nothing blocks, created with `scripts/agentkit.sh issue new`: `issue claim <n>`.
- One branch per goal, in a worktree, named `<type>/<issue>-<slug>` after its epic or first issue. One commit per issue.
- Issues that change the same shared files (`scripts/agentkit.sh map show <path>` says `shared`) go into the same
  branch. A goal too large for one pull request is cut where its shared files allow, server before client.
- A spike's report goes into its issue as a comment and its code stays on an unmerged `spike/<issue>-<slug>` branch.

## Building

- Start from `scripts/agentkit.sh ctx <n>` and hand its file to subagents instead of letting them explore again.
- Bring the branch up to date with `git fetch origin && git rebase origin/main`; after a conflict in generated files,
  `scripts/agentkit.sh regen`. Fold a correction into the commit it corrects (`git commit --fixup`, then
  `git rebase --autosquash origin/main`) and push with `--force-with-lease`.
- A UI change starts with the design, not the code: what the screen is for, what is on it now, and what the change
  removes or merges before it adds. A new screen or a large change gets three options first.

## Checking

`scripts/agentkit.sh check` runs what is fast for the files you changed. The merge queue runs everything else,
once, on the merged state; do not run the full check or the web E2E locally except to reproduce a failure.

## Opening and landing

- `scripts/agentkit.sh pr open` opens the pull request of the branch.
- `scripts/agentkit.sh pr land <pr>`, in the background, queues it and follows it to the merge, removes the worktree
  and the branch, and closes the epic and reports the milestone when the pull request completes them.
- When it reports a failure or a conflict, fix it at once: a pull request never waits for the decider to notice.
- The work is finished when the goal is merged and closed. The final report has three parts: what is merged, what is
  not and why, and what needs a decision.

## Orchestrating

- Run up to four subagents at once, each on one issue or one layer of it, in its own worktree, ending at its commit.
  Subagents do not open pull requests and do not wait for CI: the orchestrator rebases their commits onto the goal
  branch, opens the pull request, and lands it.
- Use the agent definitions of `.claude/agents/`, which fix the model, the worktree, and the report of each kind of work.
- A red Nightly stops the line: fix it before starting more issues (`gh issue list --label ci:nightly-failure`).
- When the decider answers a question, state the conclusion in the ADR that owns the topic, or on the issue.

## When a decision is needed

Decide what is cheap to undo and state it in the pull request. Stop only for a decision that no Accepted ADR covers
and that is expensive to undo: a public contract, a schema that loses data, a paid service, a new setting users see.
Then `scripts/agentkit.sh issue block <n> --type adr` with the options and your recommendation (the decider reads
`gh issue list --search 'type:ADR is:open'`), draft a Proposed ADR if it is architectural, and go on with the rest.
