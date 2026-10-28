---
name: work-on-issue
description: Take a GitHub issue from mycroblog/my through the full agent workflow — claim, branch, implement, verify with scripts/task.sh check, and open a PR. Use when asked to work on, pick up, or implement an issue.
---

# Work-on-issue skill

Arguments: an issue number (optional). Without one, pick the next issue as described in the playbook.

Follow `docs/agents/workflow.md` exactly. In particular:

- Run the steps through agentkit: `scripts/agentkit.sh issue claim <n>`, then `scripts/agentkit.sh ctx <n>` as the
  starting context (read more with `show` / `find` / `refs` / `trace`, by section), `scripts/agentkit.sh check`,
  `scripts/agentkit.sh pr open`, and `scripts/agentkit.sh pr wait <pr>` in the background.

- Only work on issues labeled `agent:ready`; stop and report if the issue is `status:needs-decision` or depends on a Proposed ADR.
- Never push to `main` and never merge.
- `scripts/task.sh check` must pass before committing. When the change touches TypeScript or JavaScript, run
  `scripts/task.sh web:fmt` first.
- Keep the user and operator documents true: update each document of `scripts/agentkit.sh docs impact` that the change
  makes wrong, and give `pr open --doc-reason '<path>=<reason>'` for each that stays true (AGENTS.md "Documentation").
- If blocked, comment on the issue, add `agent:blocked`, and stop.

Finish by reporting the PR URL and any assumptions you made.
