---
name: adr
description: Draft a new Architecture Decision Record or revise an existing one in docs/adr, following this repo's ADR policy. Use when asked to write, update, or propose a design decision.
---

# ADR skill

1. Read `docs/agents/writing-adrs.md` and `docs/adr/README.md`, and list the existing decisions with `scripts/agentkit.sh adr list` to avoid overlap.
2. Read the requirements referenced by the topic in `docs/requirements/` and any related ADRs.
3. If an ADR owns the topic, edit it. Only for a topic none owns, run `scripts/task.sh docs:adr-new TITLE="<Japanese title>" SLUG=<kebab-case-english>`.
4. Fill every section of the template: the decision, its progression, the rejected alternatives. No quotations of the decider's answers. Mark unverified third-party facts as 要検証; verify key ones with web search when possible and cite the source.
5. Never set the status to Accepted or Rejected, and never change the conclusion of an Accepted ADR unless the decider has stated the change.
6. Run `scripts/task.sh check`.
7. If the ADR needs a decision from the owner, open or update a `type:adr` issue with `status:needs-decision` and a 5-line summary.
