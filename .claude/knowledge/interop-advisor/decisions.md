# Interoperability decisions

One line each: `- <decision> — why — from (ADR, decider, or test) — date — held N / missed N`. A decision an ADR
records is cited, not restated. When the same correction comes twice, propose a check or a test and delete the line.

- Accept that an article over about 50-60 KB of HTML does not reach Misskey (64 KiB inbox) — sending it twice or cutting it loses more — federation design, articles (ADR-0160) — 2026-11-10 — held 0 / missed 0
- Send each article body once: no `source`, no `contentMap` for one language, language through `nameMap`/`summaryMap` — every copy counts against Misskey's 64 KiB, and no major server reads `text/markdown` — ADR-0160 §9 — 2026-11-10 — held 0 / missed 0
- Always send an article summary, made from the body when empty — it is the visible body on Mastodon and the CW elsewhere — ADR-0160 — 2026-11-10 — held 0 / missed 0
- Send article title and summary as plain text — Mastodon and Akkoma embed them unescaped, escaping would show entities elsewhere — ADR-0160 — 2026-11-10 — held 0 / missed 0
- Answer reply and boost requests with `object` = [embedded request, interaction IRI] — GoToSocial reads only the IRI, others the embedded request — ADR-0155 §6 — 2026-11-10 — held 0 / missed 0
- Keep serving keys of local actors while the closure `Delete`s go out — GoToSocial ignores a `Delete` whose key fetch gets 410 — ADR-0088 — held 0 / missed 0
- Serve `movedTo` before sending `Move` — Misskey migrates by refetching the actor — ADR-0089 — held 0 / missed 0
- Order `Like`/`Undo Like` of one account on one post — Misskey keeps one reaction and refuses a `Like` that overtakes the `Undo` — ADR-0051 §6 — held 0 / missed 0
- LD-sign public `Create`/`Update`/`Delete`, and put a proof only on documents without an LD signature — relays forward originals, and a proof added after breaks the LD signature — ADR-0006 §19, §30 — held 0 / missed 0
