---
name: adr
description: Draft a new Architecture Decision Record or revise an existing one in docs/adr, following this repo's ADR policy. Use when asked to write, update, or propose a design decision.
---

# ADR skill

Policy: ADR-0000 §1. Template: `docs/adr/template.md`.

1. Find the ADR that owns the topic: `scripts/agentkit.sh adr list`, `scripts/agentkit.sh find <words> --kind adr`.
   One ADR per topic: if one owns it, edit it. Only for a topic none owns:
   `scripts/task.sh docs:adr-new TITLE="<Japanese title>" SLUG=<kebab-case-english>`.
2. Write three things and nothing else: the decision, its reasons (with the alternatives not taken), and the
   conditions under which to revisit it. No file, function, task, or setting names, no lists of what exists, no
   procedures: the code holds those.
   - **Decision** stands alone, in numbered sections others cite as `ADR-NNNN §k`. Keep the numbers: add sections
     at the end and rewrite a section in place. A Proposed ADR phrases it as a recommendation.
   - **`経緯`** says why the decision has its present form; **`採らなかった案`** gives each alternative its
     strongest case and why it lost, a line or two each. Neither is a changelog.
   - State conclusions, not the decider's words.
3. Never set the status to Accepted or Rejected, and change the conclusion of an Accepted ADR only as the decider
   stated it, by rewriting the section. Propose any other change in the pull request.
4. Mark unverified third-party facts as `要検証`, with the source and the date for the ones you verified.
5. Merging an ADR into another retires its number in `docs/adr/aliases.yaml`. Never reuse a number.
6. `scripts/task.sh docs:check`. If the ADR needs the decider, open or update an issue of type ADR with the options
   and your recommendation in five lines, and link the pull request.
