# ts-developer: decisions, styling

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- Tailwind classes only in @my/design-system; the layers above compose its components and variants and pass no className, and a missing variant is added to the design system — changing the design system reaches every screen — ADR-0025 §4 — 2026-10-16 — held 0 / missed 0
