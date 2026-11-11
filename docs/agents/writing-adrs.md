# Writing ADRs

Policy: [ADR-0000](../adr/0000-record-architecture-decisions.md) §1. Template: [template.md](../adr/template.md).

An ADR holds three things: the decision, its reasons (with the alternatives not taken), and the conditions under
which to revisit it. File, function, task, and setting names, lists of what exists, and procedures belong to the code;
`scripts/task.sh docs:rules` rejects them in an ADR.

- **One ADR per topic.** Edit the ADR that owns the topic (`scripts/agentkit.sh adr list`); create a file only for a
  topic none owns: `scripts/task.sh docs:adr-new TITLE="<Japanese title>" SLUG=<kebab-case-english>`.
- **Decision** stands alone, in numbered sections others cite as `ADR-NNNN §k`. Keep the numbers: add sections at the
  end and rewrite a section in place. A Proposed ADR phrases it as a recommendation.
- **`経緯`** says why the decision has its present form, and **`採らなかった案`** gives each alternative its strongest
  case and why it lost, a line or two each. Neither is a changelog.
- State conclusions, not the decider's words. Mark a third-party fact `要検証` until verified, then give its source and date.
- Only the decider sets `Accepted` or `Rejected` or changes an Accepted conclusion; record one the decider has stated
  by rewriting the section, and propose any other in the pull request.
- Merging an ADR into another retires its number in `docs/adr/aliases.yaml`. Never reuse a number.
- To ask for a decision, open or update the ADR's issue (type ADR) with the options and your recommendation in five
  lines, and link the pull request.
