# ts-test-developer: decisions, queries

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- Find an element by its role and accessible name first; never by nth(), first(), a CSS selector or part of its text, and E2E screen helpers live in @my/e2e only (its check arrives with #2876) — a test that finds what a user perceives breaks only when the user's view changes — ADR-0170 §7 — 2026-11-14 — held 0 / missed 0
- When the copy changes the words under a test, match the new words in each locale; never fall back to the parts' order or a looser pattern — the check is what the reader reads (#3091, the lead's correction) — #3091 — 2026-11-16 — held 0 / missed 0
