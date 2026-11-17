# ts-test-developer: decisions, interface

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- Read a package's interface from its declarations: `bun --bun tsc -p . --noEmit false --declaration --emitDeclarationOnly --outDir <a directory of your own in the scratchpad>` from the package, then the `.d.ts` of the part — they hold every exported signature and doc comment and no implementation (the rootDir error it ends with is harmless); the scratchpad is shared with the other agents of the session, so a file there under a common name gets overwritten — #3165 — 2026-11-17 — held 0 / missed 0
