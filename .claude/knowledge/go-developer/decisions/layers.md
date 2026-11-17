# go-developer: decisions, layers

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- The layers run internal/domain (pure), internal/app (use cases and ports), internal/interface and internal/adapter, then internal/platform and cmd/; depguard in .golangci.yml refuses the reverse — the domain stays free of I/O and adapters stay replaceable — ADR-0002 §1 — 2026-10-16 — held 0 / missed 0
- A module reaches another only through that module's app package, and a table has one owning module — a module's insides stay its own — ADR-0002 §1 — 2026-10-16 — held 0 / missed 0
- Wiring happens only in internal/platform and cmd/, by constructors, with no DI container — one composition root to read — ADR-0002 §1 — 2026-10-16 — held 0 / missed 0
- A port only where two implementations exist or are certainly needed — an interface with one implementation is indirection — ADR-0002 §1 — 2026-10-16 — held 0 / missed 0
- Modules talk asynchronously by a domain event, the transactional outbox and the job queue — the event commits with the change that caused it — ADR-0002 §1 — 2026-10-16 — held 0 / missed 0
