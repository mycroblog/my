# Misskey

Base: <https://github.com/misskey-dev/misskey/blob/2026.10.0/packages/backend/src/> (read 2026-11-10; 2026.10.1
checked for differences). Fixtures: `misskey/2025.4`, `2026.10`, `circa-2019`, `circa-2022`.

## Follows and reactions

- The `Undo` of a `Follow` has an id other than the `Follow` it undoes -> my matches the embedded `object.actor`/`object.object` pair and keeps a follow tombstone per pair [ft: DropsAFollowAfterAnEarlyUndo]
- One reaction per user per note: a `Like` that overtakes the `Undo` of the previous one is refused and nothing remains -> my serializes Like/Undo Like per account and post with an ordering key [ft: SendsReactionsInTheLikeSlot]
- Reacts with a `Like` carrying `_misskey_reaction` (Unicode or `:shortcode:` with an `Emoji` tag) -> my reads it as a reaction, and sends `content` and `_misskey_reaction` with the same string [ADR-0051 §8; live: TestReaction]
- Sends `Add`/`Remove` to `featured` when pins change -> my refetches the collection [ADR-0006 §17]

## Posts

- `_misskey_content` (MFM source) and `_misskey_summary` -> my ignores them and reads `content`/`summary`
- Quotes with `_misskey_quote` (also `quoteUrl`) -> my reads them as legacy quotes and sends `_misskey_quote`, `quoteUrl`, `quoteUri` with `quote` [TestNormalizeDialects]
- `isCat` and other unknown properties -> ignored
- Does not refetch WebFinger after fetching an actor [ADR-0006 §28, live 2026-10-22]
- Accepts a `Follow` from the minimal `inbound` actor document [ADR-0006 §28]

## Articles my sends, and limits

- inbox POST body at most 64 KiB (`server/ActivityPubServerService.ts` bodyLimit), else 413 -> my sends the body once (no `source`, no `contentMap` for one language); an article over about 50-60 KB of HTML still fails there, accepted [decisions.md]
- An Article becomes a plain note (`core/activitypub/type.ts` validPost); a non-empty summary is always a CW; name is stored but not shown; HTML goes to MFM (`core/MfmService.ts`: headings/paragraphs as paragraphs, lists without marks, tables as text, `hr` and `img` dropped); url stored only; image not read
- Note text cut at 8,192 chars (`const.ts` DB_MAX_NOTE_TEXT_LENGTH); `cw` varchar(512), `name` varchar(256) (`models/Note.ts`) -> my summary (500) and title (200) fit
- Reads `source` only as `text/x.misskeymarkdown`; `text/markdown` is ignored
- Drops a note `Update` except a `Question`'s counts: edits do not arrive (`ApInboxService.ts`)

## Relays, migration

- Drops relayed activities whose signer is not the actor unless they carry an LD signature -> my LD-signs public Create/Update/Delete; json-gold vs jsonld.js equivalence is tested [ADR-0006 History; ft: CanonicalizerContract] (Misskey verification of my LD signature unconfirmed)
- On a `Move` it refetches the actor and migrates by its `movedTo` -> my serves `movedTo` from the transaction that records the move, before sending `Move` [live: TestMove]
