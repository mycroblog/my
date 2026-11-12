# Mastodon: signatures, delivery, fetching, collections

Base: <https://github.com/mastodon/mastodon/blob/v4.7.3/> unless a line says otherwise. Fixtures: `mastodon/4.7`, `4.8`.

## Signatures and keys

- Sends draft-cavage first, accepts RFC 9421 too; 4.4 and older do not accept RFC 9421 -> my sends draft-cavage, retries once with RFC 9421 on 400/401, remembers the host 7 days [#232 survey of 4.8.0-dev, 2026-10-17; ft: HTTPSignatureContract]
- Receives `ed25519` signatures from 4.7 -> my still signs with RSA only [ADR-0006 §5]
- keyId is `{actor}#main-key` -> my looks the key up by the exact keyId [ft: HTTPSignatureContract]
- Verifies FEP-8b32 proofs only on main (from 2026-07), not in 4.7 -> my sends proofs, but LD signatures on public posts [ADR-0006 History]
- Compacts an LD-signed document as JSON-LD before use; a document invalid as JSON-LD loses its LD signature, so a relayed copy is dropped (direct delivery still works) -> my resolves `@context` only from bundled documents [ADR-0006 Consequences; live: TestRelay]
- LD signature shape `LinkedDataSignature#sign!` (URDNA2015, options with identity/v1 context) -> my builds the same [app/lib/activitypub/linked_data_signature.rb; ft: CanonicalizerContract]

## Delivery and health

- `Stoplight`: 10 consecutive failures open the circuit of a host for 60 s -> my uses the same values [internal/domain/federation/delivery.go BreakerThreshold]
- `DeliveryFailureTracker`: failures on 7 distinct days mark a server unavailable -> same threshold [ft: MarksServersUnavailableAndBack]
- Resends a `Follow` with a new id until an `Accept` arrives -> my records it in follow_resends and sends `Accept` again [ft: InboxProcessingContract OrderAndDuplicates]
- Sends `Like` without addressing, `Flag` without to/cc with the instance actor as actor, `Block` to the personal inbox -> my sends the same shapes [live: TestReportForwarding]
- `Move` pull mode (sent by the target) is not accepted; on a Move, a follower who blocks or is blocked by the target keeps following the origin -> same [ADR-0089 §4; ft: MigratesTheLocalRelationships]

## Fetching and actors

- Unsigned GET is anonymous; only secure mode (Authorized Fetch) answers 401; its minimal actor holds @context, id, type, preferredUsername, inbox, publicKey, endpoints -> same fields, plus assertionMethod and movedTo [ADR-0006 §31; TestAuthorizedFetch]
- After fetching an actor it looks up its `acct:` WebFinger -> my answers WebFinger even in `inbound` mode [ADR-0006 §28, live 2026-10-22]
- A 404 on refetching an actor deletes neither the account nor the follows [ADR-0006 §28]
- Hidden followers/following: `OrderedCollection` with `totalItems` only, pages 403 -> same shape [TestCollectionPages]
- Collection pages: `?page=true`, next `?max_id=…&page=true` -> same shape
- Featured hashtags: changes go out as an `Add` of a `Hashtag` to `featured`; 10 per account (`FeaturedTag::LIMIT`), counted from public/unlisted posts -> my refetches `featuredTags` on any `Add`/`Remove` to `featured`, and sends an actor `Update` instead [app/services/activitypub/process_account_service.rb, main 2026-11-09; live: TestFeaturedTags]
- Resolves a remote custom emoji by GET of its `Emoji` id -> my serves `/ap/emojis/{uuid}` [ADR-0051 §3]
- Shows a local suspension (`toot:suspended` actor Update) and its lift [live: TestSuspension]
