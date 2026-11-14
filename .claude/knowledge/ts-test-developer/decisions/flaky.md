# ts-test-developer: decisions, flaky

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.
[lint] marks a line that `restrict/e2e-flaky` enforces in @my/e2e; its message names the same evidence.

- [lint] Assert on a screen at rest, with `reducedMotion: "reduce"` in playwright.config.ts, never after a wait for an animation to settle — every such wait flaked again (b5471c720, 6e27655b1, cce3243e7, 4de405ec6, b511410df) — #2876 — 2026-11-14 — held 0 / missed 0
- [lint] Read the page only through retrying assertions (`toHaveText`, `toHaveCount`, `expect.poll`), never `expect(await …)`, `textContent()` or `allTextContents()` — a one-shot read sees the page before it settles (0ab550727, 07d058a31, cce3243e7) — #2876 — 2026-11-14 — held 0 / missed 0
- [lint] Name an element exactly (`exact: true`, or a RegExp anchored `^…$`) inside a named region — a substring finds another element once a longer name shows (cce3243e7, b6632bd23, #1403) — #2876 — 2026-11-14 — held 0 / missed 0
- [lint] Never pass `timeout:` to one wait; wait for the state that comes first — a longer wait hides the state the test skipped and flakes again under load (66d4f3db6, #309, #405) — #2876 — 2026-11-14 — held 0 / missed 0
- [lint] Never sleep or read the wall clock; wait for an observable event, and move the page's time with `page.clock` (the consent screen's countdown: `clock.install()`, then `runFor`) — time differs per run and per load (66d4f3db6, 45883c030, #1588) — #2876 — 2026-11-14 — held 0 / missed 0
- [lint] Choose a React Aria radio by clicking its label, never `.check()` or `force: true` — its input is hidden and its state comes from the press (6bd4f2c17, 5946eba33) — #2876 — 2026-11-14 — held 0 / missed 0
- When a flake is a race in the product, report it for the product to fix, never bend the test around it — the user meets the same race (e13d43b6d, d3ff972e9, 852121337) — #2876 — 2026-11-14 — held 0 / missed 0
- Never leave or reload the page before a write is answered; a screen part's write returns once the server answered (screens/write.ts) — the reload aborts the request and the server drops the write (the lost favourite, ERR_ABORTED on reload) — #2876 — 2026-11-14 — held 0 / missed 0
- A test that writes acts on `fresh.*` subjects or as a fresh person and asserts what it changed, never global counts; seeded data stays read-only; run with `--repeat-each=2` as CI does — a second run or another spec finds the first one's writes, and shared limits count too, so each test's browser has its own client IP — #2876 — 2026-11-14 — held 0 / missed 0
- Set the options of every spec in a fixture of specs/test.ts or in playwright.config.ts, never with `test.use` in a shared module — the module runs once per worker, so its `test.use` reaches only the first file the worker loads (the smoke spec's "invalid URL") — #2876 — 2026-11-14 — held 0 / missed 0
