---
name: adr
description: Draft a new Architecture Decision Record or revise an existing one in docs/adr, following this repo's ADR policy. Use when asked to write, update, or propose a design decision.
---

# ADR skill

1. Find the ADR that owns the topic: `scripts/agentkit.sh adr list`, `scripts/agentkit.sh find <words> --kind adr`.
   If one owns it, edit it. Only for a topic none owns: `scripts/task.sh docs:adr-new TITLE="<Japanese title>" SLUG=<kebab-case-english>`.
2. Write three things and nothing else: the decision, its reasons (with the alternatives not taken), and the
   conditions under which to revisit it. No file, function, task, or setting names, no lists of what exists, no
   procedures: the code holds those.
3. Never set the status to Accepted or Rejected, and change the conclusion of an Accepted ADR only as the decider stated it.
4. Mark unverified third-party facts as 要検証, with the source and the date for the ones you verified.
5. `scripts/task.sh docs:check`. If the ADR needs the decider, open or update a `type:adr` issue with
   `status:needs-decision` and a five-line summary.
