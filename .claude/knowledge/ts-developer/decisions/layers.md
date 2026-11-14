# ts-developer: decisions, layers

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- Imports run design-tokens, design-system, app-components, app-features, app, and platform and rtc, app-data, app-features, never back; the lint refuses the reverse — the direction is checked, not named — ADR-0025 §3 — 2026-10-16 — held 0 / missed 0
- A feature never imports another feature; features meet through app-data, or app wires them — a feature stays removable — ADR-0025 §3 — 2026-10-16 — held 0 / missed 0
- app-components only displays: its data comes in props, it fetches nothing — a component works with any source — ADR-0025 §4 — 2026-10-16 — held 0 / missed 0
- A feature's logic lives in hooks without UI, and server data only in app-data, never copied into useState or a Context — one cache to keep fresh — ADR-0025 §4 — 2026-10-16 — held 0 / missed 0
- Only app decides by the viewport; components and features use container queries — a component fits any pane — ADR-0025 §4 — 2026-10-16 — held 0 / missed 0
