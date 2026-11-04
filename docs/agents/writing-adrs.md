# Writing ADRs (playbook)

Policy: [ADR-0000](../adr/0000-record-architecture-decisions.md). Template: [template.md](../adr/template.md).

## Creating

```sh
scripts/task.sh docs:adr-new TITLE="RDB の抽象化と対応方言" SLUG=database-abstraction
```

The script picks the next number, one past the largest ADR number on the working tree, among the numbers retired in
`docs/adr/aliases.yaml`, and in the files the open pull requests add (it warns and uses the working tree only when `gh` is
unavailable), and fills in the date and `Status: Proposed`.
If another pull request takes the same number first, `docs:adr-numbers` fails in the merge queue: renumber yours.
`scripts/agentkit.sh adr list [--status Proposed]` lists the existing ADRs.

Create a file only for a topic no ADR owns. A small decision within a topic goes into the ADR that owns it: a tree of many
narrow ADRs is noise to whoever has to find the conclusion.

## Quality bar

An ADR says what was decided, how the decision came to have that form, and what was rejected.

- **Decision** comes first and stands alone: the conclusion, in numbered sections that others can cite. For a Proposed ADR, it is
  phrased as a recommendation (`推奨案`, "recommended option") and states what would overturn it.
- **`経緯`** (progression): the problem, the requirement IDs, and the turns that led here, short. The detail stays in git.
- **`採らなかった案`** (rejected alternatives): each option fairly, including the strongest case for it, and why it lost, in a line or two.
- **Consequences** list the bad parts too, and follow-up work that should become issues.
- No quotations of the decider's answers: state the conclusion and the reason. `docs lint` reports quotation lines.
- Facts about third-party services (prices, limits, feature support) are marked **`要検証`** (to be verified) unless verified against official docs,
  with the date and source of verification when they are.
- Keep it short. Long research goes into a spike report and is linked.

## Revising

- Proposed ADRs: edit freely in a PR, but write only the current proposal. Never add "previously proposed" notes or changelog sections; summarize what changed in the PR description ([ADR-0021](../adr/0021-documents-describe-present-state.md)).
- When an Open Question is resolved, write the answer into Decision / Consequences in the present tense and delete the item. Do not leave `[x]` items.
- Accepted ADRs: edit the ADR in place when the decider changes a decision or when the code has to follow something new. Rewrite
  the conclusion, and move what it replaces to `経緯` or `採らなかった案` if a reader still needs it. Only the decider changes a
  conclusion, so an agent records one the decider has stated, and proposes any other in the PR or the issue.
- Keep section numbers: code and documents cite `ADR-NNNN §k`. Add sections at the end, and rewrite a section's content in place.
- Merging an ADR into another retires its number: list it in `docs/adr/aliases.yaml`, with where its sections went. Old issues and
  pull requests keep citing it, and agentkit resolves it through that file. Never reuse the number.

## Asking for a decision

Open (or update) the `type:adr` issue for the ADR, summarize the options and recommendation in 5 lines or less,
add `status:needs-decision`, and link the PR.

Link the ADR from the issue with a `blob/main/...` URL. If the ADR is not merged yet, link the PR instead of a file on the PR branch:
branches are deleted on merge, so branch URLs break.
