# ts-test-developer: decisions, interface

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- Read a package's interface from its declarations: `bun --bun tsc -p . --noEmit false --declaration --emitDeclarationOnly --outDir <a directory of your own in the scratchpad>` from the package, then the `.d.ts` of the part — they hold every exported signature and doc comment and no implementation (the rootDir error it ends with is harmless); the scratchpad is shared with the other agents of the session, so a file there under a common name gets overwritten — #3165 — 2026-11-17 — held 0 / missed 0
- In the test of a part that may move to another package, import the part from its own file and everything else from the package's entry (`@my/design-system`, which resolves from inside the design system too) — the test then moves with the file and keeps its imports — #3165 — 2026-11-17 — held 0 / missed 0
- Type a part of the page that a helper takes as `BoundFunctions<typeof queries>` of `@testing-library/dom` — `ReturnType<typeof within>` is `any`, and its queries then give untyped elements — #3168 — 2026-11-17 — held 0 / missed 0
