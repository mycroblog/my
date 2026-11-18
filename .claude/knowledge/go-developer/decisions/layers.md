# go-developer: decisions, layers

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- The layers run internal/domain (pure), internal/app (use cases and ports), internal/interface and internal/adapter, then internal/platform and cmd/; depguard in .golangci.yml refuses the reverse — the domain stays free of I/O and adapters stay replaceable — ADR-0002 §1 — 2026-10-16 — held 0 / missed 0
- A module reaches another only through that module's app package, and a table has one owning module — a module's insides stay its own — ADR-0002 §1 — 2026-10-16 — held 0 / missed 0
- Wiring happens only in internal/platform and cmd/, by constructors, with no DI container — one composition root to read — ADR-0002 §1 — 2026-10-16 — held 0 / missed 0
- A port only where two implementations exist or are certainly needed — an interface with one implementation is indirection — ADR-0002 §1 — 2026-10-16 — held 0 / missed 0
- An outside service that a flow of the E2E demo needs (mail, an identity provider, a push service) gets a fake of the test build, never a container: internal/adapter/<family>/fake, linked from one file with `//go:build e2e_fakes`, refused outside profile = development, listed in scripts/go-fakes.sh and depguard — no Docker in the E2E job, and one shape for every fake — the decider's choice, ADR-0015 §6, §9 — 2026-11-18 — held 0 / missed 0
- Modules talk asynchronously by a domain event, the transactional outbox and the job queue — the event commits with the change that caused it — ADR-0002 §1 — 2026-10-16 — held 0 / missed 0
