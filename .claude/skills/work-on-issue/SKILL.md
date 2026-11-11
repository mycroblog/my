---
name: work-on-issue
description: Take an issue, several issues, an epic, or a milestone of mycroblog/my to main - claim, build on one branch, open the pull request, land it, clean up, and close what it completes. Use when asked to work on, pick up, implement, or finish issues, an epic, or a milestone.
---

# Work on a goal

Arguments: issue numbers, an epic, or a milestone. Without one, take the next `agent:ready` issue of the earliest milestone.

1. Claim each issue: `scripts/agentkit.sh issue claim <n> --plan "<one line>"`. It refuses what is not ready.
2. One worktree and one branch for the whole goal. Group the issues by the shared files they touch
   (`scripts/agentkit.sh map show <path>`), server before client.
3. Build: yourself for one small issue; otherwise up to four subagents at once, `builder` for implementation,
   `routine` for mechanical work, `ui-designer` before any change to a screen. Hand each
   `scripts/agentkit.sh ctx <n> --quiet`'s file. Rebase their commits onto the goal branch, one commit per issue.
4. `scripts/agentkit.sh pr open`, then `scripts/agentkit.sh pr land <pr>` in the background.
5. When `pr land` reports a failure or a conflict, fix it at once (a `routine` agent for a rebase).
6. Finish when the goal is merged and closed. Report what is merged, what is not and why, and what needs a decision.
