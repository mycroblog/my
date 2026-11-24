# go-developer: decisions, naming

The names that leave the code and are stored or set by operators: job types, settings, defaults. One line each:
`- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- A job type is `<module>.<verb>_<object>` (`gathering.open_room`, `gathering.queue_due`), never a noun such as `gathering.tick`; a queued job stores its type, so a released one cannot be renamed without draining or migrating its jobs — the decider's K1 of the settings re-audit (#3381), the second correction after the earlier rename of nouns; a check would catch it — 2026-11-23 — held 0 / missed 0
- A setting that several adapters share takes the same name and the same default on each: `search_<engine>_index_settings_file`, `<prefix>_tls_ca_file`, the index `my-statuses` on every search engine — an operator who changes engines reads one vocabulary — the decider's K3 and K4 of the settings re-audit (#3381) — 2026-11-23 — held 0 / missed 0
- A telemetry attribute takes the OpenTelemetry semantic convention whose meaning fits, not the one whose name sounds right: the instance's `host` is `service.namespace`, never `service.instance.id`, which is unique per process and given by the platform — a shared ID across an instance's processes, or a platform ID erasing the host, would mislead the backend — the decider's choice on #3456 — 2026-11-24 — held 0 / missed 1
