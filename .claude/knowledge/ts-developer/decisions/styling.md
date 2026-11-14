# ts-developer: decisions, styling

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- Tailwind classes only in @my/design-system; the layers above compose its components and variants and pass no className, and a missing variant is added to the design system — changing the design system reaches every screen — ADR-0025 §4 — 2026-10-16 — held 0 / missed 0
- Every block that holds text people wrote reads in its own direction (`dir="auto"`), lists included, never only the line items — a list takes its indent from its own direction and its markers from its items', so a list left in the page's direction around items in the other puts the markers on the side with no indent, outside the text (an English list in an RTL page, an Arabic one in an LTR page) — the decider's choice to guarantee direction by the component's design instead of a test per case — 2026-11-14 — held 0 / missed 0
