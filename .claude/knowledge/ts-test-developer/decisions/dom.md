# ts-test-developer: decisions, dom

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- Name a component test `*.dom.test.tsx` (happy-dom), and `*.jsdom.test.tsx` only when happy-dom cannot decide it; run one with `bun test --preload ../devtools/src/happy-dom.ts <file>` (or `jsdom.ts`) from its package — a bare `bun test <file>` has no `document`, and a `*.test.tsx` must never get one — ADR-0171 §4 — 2026-11-17 — held 0 / missed 0
- Test a React Aria menu's keys on jsdom — on happy-dom the menu takes no focus when it opens, so the arrows, End and Enter reach no item; only Escape works — #3164 — 2026-11-17 — held 0 / missed 0
- While a menu or a popover is open, find what is in it, never its button by role — the page outside it is `aria-hidden`, so `getByRole("button", { expanded: true })` finds nothing — #3164 — 2026-11-17 — held 0 / missed 0
- Read the focus a part gives back when it closes inside `waitFor` on jsdom — it returns a frame after the close; on happy-dom at once — #3164 — 2026-11-17 — held 0 / missed 0
- Put a focusable element of the page beside a dialog whose focus trap is under test — with nothing outside, Tab stays inside by itself — #3164 — 2026-11-17 — held 0 / missed 0
