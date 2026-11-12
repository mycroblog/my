---
name: work-on-issue
description: Take an issue, several issues, an epic, or a milestone of mycroblog/my to main - claim, build on one branch, open the pull request, land it, clean up, and close what it completes. Given an epic or a milestone, plan its pull requests first and act on its own once the plan is approved. Use when asked to work on, pick up, implement, or finish issues, an epic, or a milestone.
---

# Work on a goal

Arguments: issue numbers, an epic, or a milestone. Without one, take the next issue nothing blocks of the earliest milestone.
The commands refuse what must not pass and say the fix; this says the order and where judgment is needed.

## Plan an epic or a milestone

Given an epic or a milestone, you are the orchestrator. Before building:

1. Read the epic's body (it may say where its pull requests go) and its open sub-issues; for a milestone, every
   epic in it and the issues outside them. Order them by what blocks what, and group them into pull requests by
   the shared files they touch (`scripts/agentkit.sh map show <path>`), server before client.
2. If it all fits one pull request, go on to the next section without asking.
3. Otherwise present the plan and wait for the decider's approval: the pull requests in order, the issues of each,
   what is left out and why, and the decisions it needs. Revise it until it is approved.
4. Record the approved plan with `/comment-issue` on the epic (for a milestone, each epic's part on that epic),
   and follow it from there, after a compaction too. Ask again only for a decision that is expensive to undo and
   that no Accepted ADR covers, or when the plan stops holding; then say what changed and propose the new plan.

## Build and land

1. A red Nightly stops the line: fix it before starting more (`gh issue list --label ci:nightly-failure`).
2. Claim each issue: `scripts/agentkit.sh issue claim <n> --plan "<one line>"`. It refuses what is blocked.
3. One worktree and one branch per pull request, `<type>/<issue>-<slug>`, one commit per issue. A spike's report
   is a comment on its issue; its code stays on an unmerged `spike/<issue>-<slug>` branch.
4. Build: yourself for one small issue; otherwise up to four subagents at once, `builder` for implementation,
   `routine` for mechanical work, the roles of ADR-0169 for a UI change, which starts with the design, not the code.
   Hand each `scripts/agentkit.sh ctx <n> --quiet`'s file. Rebase their commits onto the branch.
5. Keep the branch on `origin/redesign-web` (the trunk during #2863) by rebase (`agentkit regen` after a conflict in generated files).
   Fold a correction into its commit (`git commit --fixup`, `GIT_SEQUENCE_EDITOR=: git rebase -i --autosquash origin/redesign-web`), push
   `--force-with-lease`. `main` comes in by `git merge origin/main` on `sync/main`, `gh pr create --base redesign-web`, `pr land`. `scripts/agentkit.sh check` is the local check; the merge queue runs the rest, so run the
   full check or the web E2E only to reproduce a failure.
6. `scripts/agentkit.sh pr open`, then `scripts/agentkit.sh pr land <pr>` in the background. An epic is one pull
   request: `pr open -draft` with its first commit, `pr open` again after each later one (it rewrites the open pull
   request's description), and `gh pr ready` before `pr land` once the last is in. When `pr land` reports a failure
   or a conflict, fix it at once (a `routine` agent for a rebase).
7. Finish when the goal is merged and closed. Report what is merged, what is not and why, and what needs a decision.

## When a decision is needed

Decide what is cheap to undo and say so in the pull request. Stop only for one that no Accepted ADR covers and that is
expensive to undo: a public contract, a schema that loses data, a paid service, a setting users see. Then
`scripts/agentkit.sh issue block <n> --type adr` with the options and your recommendation, draft a Proposed ADR if it
is architectural, and go on with the rest. When the decider answers, state the conclusion in the ADR that owns the
topic, or on the issue.
