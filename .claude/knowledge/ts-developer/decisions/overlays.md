# ts-developer: decisions, overlays

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- A modal or layer that looks late or wrong is explained by reading its path from the press to the first frame, through the library's source in node_modules, before any trace or screenshot — the behavior is written in the code, and a browser here differs from the decider's (no touch screen) — the decider's correction on #3067 — 2026-11-14 — held 0 / missed 0
- A modal is the design system's `ModalLayer` (modal.tsx: React Aria's hooks, moved by Motion from one variant state, rendered in a `ModalPresence` to leave), never RAC's `ModalOverlay` — `ModalOverlay` mounts hidden and reveals through `runAfterKeyboard`, which waits up to 600 ms for a keyboard on any device with `maxTouchPoints > 0` while a text field has focus, so the motion runs unseen — the decider's choice of Motion on #3067 — 2026-11-14 — held 0 / missed 0
