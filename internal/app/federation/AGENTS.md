# Federation between servers: delivery, fetching, signatures, discovery

- Signing or verifying a request with HTTP Message Signatures: [RFC 9421](https://www.rfc-editor.org/rfc/rfc9421), search for: `Signature-Input`, `@target-uri`, `@method`, Creating the Signature Base, `Accept-Signature`
- Signing or verifying with the draft Mastodon and most servers still use (draft-cavage-12, not the RFC): [draft-cavage-http-signatures-12](https://datatracker.ietf.org/doc/html/draft-cavage-http-signatures-12), search for: `(request-target)`, `keyId`, `hs2019`, `(created)`
- Choosing the scheme for a remote server, or what Mastodon accepts and requires: [Mastodon documentation, Security](https://docs.joinmastodon.org/spec/security/), search for: HTTP Signatures, `(request-target)`, RFC9421, Linked Data Signatures
- Changing the digest of a body: [RFC 9530](https://www.rfc-editor.org/rfc/rfc9530), search for: `Content-Digest`, `sha-256`, `Repr-Digest`
- Reading the `Digest` header that draft-cavage peers send (obsoleted by RFC 9530): [RFC 3230](https://www.rfc-editor.org/rfc/rfc3230), search for: `Digest`, `Want-Digest`
- Resolving an `acct:` address to an actor: [RFC 7033](https://www.rfc-editor.org/rfc/rfc7033), search for: `acct`, `rel`, `application/jrd+json`, `subject`
- Reading the NodeInfo of a remote server: [NodeInfo protocol](https://nodeinfo.diaspora.software/protocol.html), search for: `/.well-known/nodeinfo`, `http://nodeinfo.diaspora.software/ns/schema/2.1`
- Changing delivery, inbox handling, or forwarding: [ActivityPub](https://www.w3.org/TR/activitypub/), search for: Delivery, `sharedInbox`, Forwarding from Inbox, Shared Inbox Delivery
- Changing the proof of an outgoing or incoming activity: [FEP-8b32](https://fediverse.codeberg.page/fep/fep/8b32/), search for: `DataIntegrityProof`, `eddsa-jcs-2022`, `proofValue`
- Changing emoji reactions: [FEP-c0e0](https://fediverse.codeberg.page/fep/fep/c0e0/), search for: `EmojiReact`, `Like`
- Changing the migration of an account (`Move`, `movedTo`): [FEP-7628](https://fediverse.codeberg.page/fep/fep/7628/), search for: `Move`, `movedTo`, `alsoKnownAs`
- Before changing how this package talks to other servers, ask the `interop-advisor` agent: [interop-advisor](https://github.com/mycroblog/my/blob/main/.claude/agents/interop-advisor.md), search for: Mastodon, Misskey, GoToSocial, Akkoma
