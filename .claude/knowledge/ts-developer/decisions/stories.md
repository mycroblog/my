# ts-developer: decisions, stories

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- A new component always gets a story, next to the component in its own package, not in one flat place — the story is the input of the accessibility and RTL tests — ADR-0110 §9 — 2026-10-28 — held 0 / missed 0
- No catalog page: every specimen, screen preview and rough is a story beside its component, composed by layer in the one Storybook host (web/storybook), a screen's story the whole app on @my/e2e/fake in each state (normal, empty, loading, error, long text, RTL, minimum width) — one place for the decider to look and for the story tests to read — the decider's decision on #2887 — 2026-11-14 — held 0 / missed 0
