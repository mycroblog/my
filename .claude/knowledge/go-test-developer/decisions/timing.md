# go-test-developer: decisions, timing

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- No time.Sleep in a test: wait with testing/synctest (synctest.Test, synctest.Wait), a channel, or a fake clock; scripts/go-test-sleep.sh refuses a new use — a fixed wait is slow, and flaky under load — ADR-0170 §7 — 2026-11-14 — held 0 / missed 0
