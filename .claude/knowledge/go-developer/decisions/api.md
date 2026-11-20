# go-developer: decisions, api

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- Every write of the client API takes an Idempotency-Key, whose first answer the guard keeps and replays; a client retries a write that got no answer in place with it and keeps nothing to send later (securitygen refuses a write without the key or x-no-idempotency-key) — a client that queues every write it could not send has no bound on what it holds — the decider's choice, #2892, ADR-0028 §18 — 2026-11-20 — held 0 / missed 0
