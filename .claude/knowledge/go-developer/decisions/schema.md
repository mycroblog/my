# go-developer: decisions, schema

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- Change the schema only by editing the module's desired-state file of each dialect, schema/<NN>-<module>/<dialect>.sql; never write a migration — the desired state is the source of truth, and sqlc reads the same file — ADR-0023 §1 — 2026-10-16 — held 0 / missed 0
- A change that loses or rewrites data is two steps, expand and then contract in a later minor version; db:lint refuses it in one — an upgrade keeps the data, and an old server runs briefly on the new schema — ADR-0023 §5, §6 — 2026-10-16 — held 0 / missed 0
- Nothing Atlas Community Edition cannot handle in the schema (extensions, sequences, views, triggers); IDs are UUIDv7 made by the app — only the free edition applies it — ADR-0023 §2 — 2026-10-16 — held 0 / missed 0
