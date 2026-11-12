# FEPs

Base: `https://codeberg.org/fediverse/fep/src/branch/main/fep/<id>/fep-<id>.md`

- FEP-8b32 (Object Integrity Proofs, `eddsa-jcs-2022`): "ignore a proof you cannot verify, rely on other authentication"; a proof takes precedence over the HTTP signature -> my verifies a proof only when the signer is the actor, ignores unusable ones, rejects a wrong one with 401; sends a proof on what carries no LD signature [ADR-0006 §30; ft: SignsIntegrityProofs; domain/federation proof tests use the FEP's vectors]
- FEP-8b32 verification removes `proof` and the LD `signature`; a proof with `@context` must be a prefix of the document's (vc-di-eddsa §3.3.2) [internal/app/federation/proof.go]
- FEP-521a (`Multikey` in `assertionMethod`, `publicKeyMultibase` z6Mk…) -> my publishes its Ed25519 key at `{actor}#ed25519-key` and reads remote ones [ft: HTTPSignatureContract; FEP vectors]
- FEP-2c59 (`webfinger` on the actor): a claim is used only after WebFinger confirms it; an actor embedded in another document is not trusted (Security Considerations) -> my verifies the claim, falls back to `preferredUsername@host`, and defines the `webfinger` term inline in the actor context so JSON-LD readers need not fetch `https://purl.archive.org/socialweb/webfinger` [ft: VerifiesTheHandleTheActorNames]
- FEP-3b86 (Activity Intents): one `/intent` page for `Follow` and `Create` (§3.6); a page that only opens must not make the server fetch (§6.1); no `on-success`/`on-cancel` and no relay page (§6.2) -> my resolves only after the user presses the button [internal/interface/activitypub WebFinger tests]
- FEP-044f (`quote`, `quoteAuthorization`, `QuoteRequest`, `QuoteAuthorization`) -> my reads `quote` first, then `_misskey_quote`, `quoteUrl`, `quoteURL`, `quoteUri`, then an FEP-e232 Link [ADR-0069 §2; TestNormalizeDialects]
- FEP-e232 (object links): a `Link` tag with `rel` `_misskey_quote` and the AS2 JSON-LD `mediaType` is read as a quote [TestNormalizeDialects]
- FEP-7888 (`context`): only an `https` IRI (string, Link href, or embedded id), never fetched; `ostatus:conversation` is the fallback; my own `/ap/contexts/{uuid}` declared by others is ignored [ADR-0154 §2]
- FEP-c0e0 (emoji reactions): `EmojiReact` and a `Like` with `content` -> my reads both and sends `Like` with `content` and `_misskey_reaction` [ADR-0051 §8]
- FEP-7628 (`Move`): push mode only (`object` is the actor); pull mode is not accepted, as in Mastodon and Akkoma [ADR-0089 §4; ft: DropsUnverifiedMoves]
- FEP-fe34 (origin-based security): a fetched document is used only when its `id` has the origin of the final redirect; another `id` is fetched once to confirm [ft: RejectsIDsOfAnotherOrigin]
