# ts-test-developer: decisions, queries

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- Find an element by its role and accessible name first; never by nth(), first(), a CSS selector or part of its text, and E2E screen helpers live in @my/e2e only (its check arrives with #2876) — a test that finds what a user perceives breaks only when the user's view changes — ADR-0170 §7 — 2026-11-14 — held 0 / missed 0
