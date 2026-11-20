# motion: decisions, frequency

When not to animate: the more often a change comes, the less it moves. One line each:
`- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- What the reader does many times a minute (a hover, a press, typing, a toggle such as a favourite, the scroll of a timeline) answers at once with no transition: a color and shape change only, the press's 1px shift — a motion paced on every press makes the reader wait for the show (ADR-0110 P2) — the button of the design system and the tokens `press`, as merged — 2026-11-20 — held 0 / missed 0
- What the reader does several times a session in a row (a place of the navigation) moves the least of the moving kinds: a 150 ms crossfade — direction it does not have, and time it must not cost — #2893 — 2026-11-14 — held 0 / missed 0
- No motion where nothing moved for the reader: the first screen, a change of the search or the hash alone — a motion there tells of a place change that did not happen — #2893 — 2026-11-14 — held 0 / missed 0
- With reduced motion nothing moves: a change of screen is at once, a modal is there and gone at once, and a control answers by color and shape (ADR-0110 P2) — the reader asked for it — #2893, ADR-0110 P2 — 2026-11-14 — held 0 / missed 0
- Only what runs while it lasts loops: the spinner of a control in flight, and the halo of who speaks in a voice chat room; nothing loops to draw the eye — a loop says something is going on now — the tokens `motion.spin` and `voice` as merged — 2026-11-20 — held 0 / missed 0
