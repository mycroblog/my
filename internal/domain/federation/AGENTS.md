# ActivityPub and Activity Streams 2.0 documents and their extensions

- Adding or reading a type or a property of an activity, an actor, or an object: [Activity Vocabulary](https://www.w3.org/TR/activitystreams-vocabulary/), search for: `attributedTo`, `inReplyTo`, `Tombstone`, `OrderedCollectionPage`, `Question`
- Changing how a document is serialized or parsed (`@context`, language maps, dates, IRIs): [Activity Streams 2.0](https://www.w3.org/TR/activitystreams-core/), search for: Natural Language Values, `contentMap`, Date and Times, Compact URIs
- Changing the addressing, the side effects of an activity, or what an actor document must hold: [ActivityPub](https://www.w3.org/TR/activitypub/), search for: Public Addressing, `sharedInbox`, Server to Server Interactions, Actor objects
- Changing a property Mastodon defines or reads (`toot:`, `featured`, `blurhash`, `focalPoint`, `votersCount`): [Mastodon documentation, ActivityPub](https://docs.joinmastodon.org/spec/activitypub/), search for: `featured`, `blurhash`, `focalPoint`, `votersCount`, `discoverable`, `indexable`
- Changing a Misskey property (`_misskey_quote`, `_misskey_reaction`): [Misskey Extensions to ActivityPub](https://misskey-hub.net/ns/), search for: `_misskey_quote`, `_misskey_reaction`
- Changing quote posts and their approval: [FEP-044f](https://fediverse.codeberg.page/fep/fep/044f/), search for: `QuoteRequest`, `QuoteAuthorization`, `quoteAuthorization`, `interactingObject`, `interactionTarget`
- Changing the interaction policy of a status (`canReply`, `canAnnounce`, `canQuote`): [GoToSocial documentation, Interaction Controls](https://docs.gotosocial.org/en/latest/federation/interaction_controls/), search for: `interactionPolicy`, `automaticApproval`, `manualApproval`, `canAnnounce`
- Changing the integrity proof of an activity: [FEP-8b32](https://fediverse.codeberg.page/fep/fep/8b32/), search for: `DataIntegrityProof`, `eddsa-jcs-2022`, `proofValue`, `verificationMethod`
- Changing the canonical JSON that a proof signs: [RFC 8785](https://www.rfc-editor.org/rfc/rfc8785), search for: Sorting of Object Properties, Serialization of Numbers
- Changing the Ed25519 key of an actor document: [FEP-521a](https://fediverse.codeberg.page/fep/fep/521a/), search for: `Multikey`, `assertionMethod`, `publicKeyMultibase`
- Changing the `webfinger` property or the `context` of a conversation: [FEP-2c59](https://fediverse.codeberg.page/fep/fep/2c59/), search for: `webfinger`, `acct:`
- Changing the links to objects inside a post (`Link` tags): [FEP-e232](https://fediverse.codeberg.page/fep/fep/e232/), search for: `Link`, `mediaType`, `application/ld+json`
