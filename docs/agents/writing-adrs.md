# Writing ADRs (playbook)

Policy: [ADR-0000](../adr/0000-record-architecture-decisions.md). Template: [template.md](../adr/template.md).

## Creating

```sh
scripts/task.sh docs:adr-new TITLE="RDB の抽象化と対応方言" SLUG=database-abstraction
```

The script picks the next number, one past the largest ADR number on the working tree and in the files the open pull requests
add (it warns and uses the working tree only when `gh` is unavailable), and fills in the date and `Status: Proposed`.
If another pull request takes the same number first, `docs:adr-numbers` fails in the merge queue: renumber yours.
`scripts/agentkit.sh adr list [--status Proposed]` lists the existing ADRs.

## Quality bar

- **Context** cites requirement IDs and states why the decision is needed now.
- **Decision Drivers** are ranked; the decision must be explainable from them.
- At least two **Considered Options**, described fairly — including the strongest case for the one not chosen.
- **Decision** for a Proposed ADR is phrased as a recommendation (`推奨案`, "recommended option") and states what would overturn it.
- **Consequences** list the bad parts too, and follow-up work that should become issues.
- Facts about third-party services (prices, limits, feature support) are marked **`要検証`** (to be verified) unless verified against official docs,
  with the date and source of verification when they are.
- Keep it short. Long research goes into a spike report and is linked.

## Revising

- Proposed ADRs: edit freely in a PR, but write only the current proposal. Never add "previously proposed" notes or changelog sections; summarize what changed in the PR description ([ADR-0021](../adr/0021-documents-describe-present-state.md)).
- When an Open Question is resolved, write the answer into Decision / Consequences in the present tense and delete the item. Do not leave `[x]` items.
- Accepted ADRs: do not change the decision. Write a new ADR that supersedes it and set the old one's status to
  `Superseded by ADR-XXXX` in the same PR (the status change is still approved by the human decider via PR review).

## Asking for a decision

Open (or update) the `type:adr` issue for the ADR, summarize the options and recommendation in 5 lines or less,
add `status:needs-decision`, and link the PR.

Link the ADR from the issue with a `blob/main/...` URL. If the ADR is not merged yet, link the PR instead of a file on the PR branch:
branches are deleted on merge, so branch URLs break.
