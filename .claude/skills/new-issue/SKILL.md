---
name: new-issue
description: Create one or several issues of mycroblog/my - a task, bug, spike, ADR, or epic - under the right epic or milestone, in order, with the body shape of its type. Use when asked to file, open, create, cut, or split work into issues.
---

# Create issues

Arguments: what the issues are about. `scripts/agentkit.sh issue new` refuses what is wrong and says the fix; this
skill holds only what it cannot say.

1. Find where they belong: `gh issue list --search 'type:Epic is:open' --json number,title,milestone`. A task of an
   epic gets `--parent <epic>`; one outside every epic gets `--milestone "<title>"`. Read the epic's body first: it may
   say where its pull requests go.
2. Pick the type: `task` builds something, `bug` fixes one, `spike` finds something out, `adr` waits for the
   decider, `epic` groups tasks toward a state.
3. Write each body in English to a file in the scratchpad, in the shape of its type:
   - task: `### Goal` (what and why, citing the ADR section that decides it), `### Scope` when the goal does not say
     it, `### Done when` with checkboxes that someone else can verify.
   - bug: what happens, what should, how to reproduce it, `### Done when`.
   - spike: `### Spike` (the question and why it matters), `### Done when` (what the report must show).
   - adr: `### Decision needed`, `### Inputs` or `### Questions to settle`, the options and your recommendation.
   - epic: `### Goal`, `### Done when`. Never a list of its issues: they are its sub-issues.
   Links to repository files use `blob/main/...`. Name what to build, not how to name its files.
4. Create them in order, earliest first, so that each later one can take `--blocked-by <n>` of the one it waits
   for. Add `--area` and `--priority` when they are known; an unknown area is refused with the list of areas.
5. Report each issue's number, title, and what blocks it, in one line each.
