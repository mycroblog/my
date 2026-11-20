# motion: decisions, states

The states every control has, and how each change of state shows. One line each:
`- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- Every control has rest, hover, pressed, focus by the keyboard, and disabled; one that runs an action also has busy, and a toggle on and off — a state without a look is one the reader cannot tell — the button of the design system and the press tokens, as merged — 2026-11-20 — held 0 / missed 0
- Hover mixes 10% of the shade into the fill and a press 20%, and a press moves the control down 1px, at once — the press shows under the finger as the finger lands — the tokens `press`, as merged — 2026-11-20 — held 0 / missed 0
- Focus by the keyboard is a ring: 2px of the surface, 2px of the accent text, and a 3px halo; disabled shows at 40% opacity with no hover or press — the ring is seen on any surface, and a disabled control must not seem to answer — the tokens `focusRing` and `press`, as merged — 2026-11-20 — held 0 / missed 0
- Busy keeps the control's width and its label as its name, shows a spinner that turns once in 800 ms (still with reduced motion), and ignores presses; the result shows where the control is — nothing beside it moves, and the reader looks where they pressed (ADR-0110 P2) — the button's `busy` and the token `motion.spin`, as merged — 2026-11-20 — held 0 / missed 0
