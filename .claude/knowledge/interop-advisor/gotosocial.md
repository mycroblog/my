# GoToSocial

Base: <https://codeberg.org/superseriousbusiness/gotosocial/src/tag/v0.22.1/> (read 2026-11-10; 0.22.3 checked).
Fixtures: `gotosocial/0.22`. Live tests run 0.22 and, for approvals only, 0.21.

## Signatures, actors

- Signs and verifies draft-cavage only; keyId is `{actor}/main-key` -> my sends draft-cavage first and looks keys up by the exact keyId [ADR-0006 §4; ft: HTTPSignatureContract]
- Looks up the `acct:` WebFinger after fetching an actor [ADR-0006 §28]
- Answers from its stored actor and does not refetch, so the `off` 404 was not observed [ADR-0006 Open Questions]
- After `inbound` widens to `both` it keeps the minimal actor (no `followers`), and my posts do not reach its users' home though delivery gets 202 (cause unknown) [ADR-0006 Open Questions] (unconfirmed)
- Does not process a `Delete` of an actor when fetching its key answers 410 -> during the closure delivery my keeps serving the minimal actor with its keys [live: TestClosure]

## Interaction policy and requests

- `interactionPolicy` with `canReply`, `canAnnounce`, `canQuote`, each `automaticApproval`/`manualApproval` (`always`/`approvalRequired` before 0.20) -> my reads both forms and sends the new one [TestNormalizeDialects; ADR-0155 §2]
- `canQuote` is author-only: a local user cannot quote a GoToSocial post [live: TestInteractionApproval]
- Replies and boosts to a post that needs approval go as `ReplyRequest`/`AnnounceRequest`; answers and withdrawals (`Delete` of the reply, `Undo` of the boost) round-trip both ways; a rejected reply is deleted and a rejected boost undone on its side [live: TestInteractionApproval, 0.22.1 and 0.21.3, 2026-11-10]
- Reads the `Accept` of its `AnnounceRequest` and any `Reject` only through the interaction's IRI -> my answer's `object` is [the embedded request, the interaction IRI] [processing/interactionrequests; fixture gotosocial/0.22/accept-reply-request.json]
- Cannot approve a remote boost request: `acceptAnnounce` (processing/interactionrequests/accept.go) checks a Reply instead of the Announce, returns 404, sends no Accept; since 0.17, unreported upstream as of 2026-11-10 -> a local boost of a GoToSocial post that needs approval stays pending
- Drops posts from accounts it does not follow that do not mention its users -> a local reply request reaches it only when the reply mentions the author
- Withdrawn requests stay in `GET /api/v1/interaction_requests` (only account deletion removes them); that list with `status_id` returns 500

## Articles my sends

- Ingests an Article on the Note path with the full `content`; CW is `summary`, else `name`, so every article is folded; the sanitizer (internal/text/sanitize.go) keeps h1-h6, hr, pre/code, blockquote, lists, drops img and table elements (text kept); url is the post URL; image not read; `source` not read
- Refetches on `Update`, so body edits arrive (internal/federation/federatingdb/update.go)
- From 0.22.3 drops an object unless its id, every url, attributedTo, and the fetched URL share a host -> my puts `url` on the same host as `id` [ADR-0018]
- No body size limit in its code; reverse proxy limits unchecked (unconfirmed)
