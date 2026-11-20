# go-developer: decisions, runtime

What the server costs while it runs: its memory, its pools, what it works out at startup. One line each:
`- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- What can be worked out once at build time (an embedded asset's ETag) is worked out at build time and shipped in the binary, never by reading the embedded files at startup; the web build stays embedded — reading the build for its ETags held about 110 MB of idle memory — the decider's choice of keeping the embed with build-time ETags over serving from disk (#3152, #3156, #3158), session 28c4aedc 2026-11-17 — 2026-11-17 — held 0 / missed 0
- A connection pool's size defaults from the processor count (GOMAXPROCS), and its size and the idle connections it keeps can be set by configuration — a fixed size fits only the machine it was measured on, and a pool that drops its idle connections reconnects after every quiet spell — the decider's corrections on the SQLite pool (#3177, #3181), session 28c4aedc 2026-11-17 — 2026-11-17 — held 0 / missed 0
