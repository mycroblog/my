---
name: work-on-issue
description: Take an issue, several issues, an epic, or a milestone of mycroblog/my to main - claim, build on one branch, open the pull request, land it, clean up, and close what it completes. Given an epic or a milestone, plan its pull requests first and act on its own once the plan is approved. Use when asked to work on, pick up, implement, or finish issues, an epic, or a milestone.
---

# Work on a goal

Arguments: issue numbers, an epic, or a milestone. Without one, take the next issue nothing blocks of the earliest milestone.

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

1. Claim each issue: `scripts/agentkit.sh issue claim <n> --plan "<one line>"`. It refuses what is blocked.
2. One worktree and one branch per pull request, one commit per issue.
3. Build: yourself for one small issue; otherwise up to four subagents at once, `builder` for implementation,
   `routine` for mechanical work, `ui-designer` before any change to a screen. Hand each
   `scripts/agentkit.sh ctx <n> --quiet`'s file. Rebase their commits onto the branch, one commit per issue.
4. `scripts/agentkit.sh pr open`, then `scripts/agentkit.sh pr land <pr>` in the background.
5. When `pr land` reports a failure or a conflict, fix it at once (a `routine` agent for a rebase).
6. Finish when the goal is merged and closed. Report what is merged, what is not and why, and what needs a decision.
