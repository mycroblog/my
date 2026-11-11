# Federation endpoints: WebFinger, host-meta, NodeInfo, actors, collections, inboxes

- Changing the WebFinger response or its errors: [RFC 7033](https://www.rfc-editor.org/rfc/rfc7033), search for: The JSON Resource Descriptor (JRD), `resource`, `rel`, `subject`, `Access-Control-Allow-Origin`
- Changing host-meta: [RFC 6415](https://www.rfc-editor.org/rfc/rfc6415), search for: `host-meta`, `lrdd`, XRD
- Changing the NodeInfo document (schema 2.1) or its discovery: [NodeInfo protocol](https://nodeinfo.diaspora.software/protocol.html), search for: `/.well-known/nodeinfo`, `http://nodeinfo.diaspora.software/ns/schema/2.1`, `links`
- Changing the media types an endpoint answers to or the actor document: [ActivityPub](https://www.w3.org/TR/activitypub/), search for: `application/activity+json`, `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`, Actor objects, `outbox`
- Changing a collection or its paging: [Activity Streams 2.0](https://www.w3.org/TR/activitystreams-core/), search for: `OrderedCollection`, `OrderedCollectionPage`, `totalItems`, `partOf`
- Changing the remote follow and share links of WebFinger (activity intents): [FEP-3b86](https://fediverse.codeberg.page/fep/fep/3b86/), search for: `https://w3id.org/fep/3b86/Follow`, `https://w3id.org/fep/3b86/Create`, `template`
- Changing what Mastodon expects of an actor, a collection, or secure mode: [Mastodon documentation, ActivityPub](https://docs.joinmastodon.org/spec/activitypub/), search for: `featured`, `featuredTags`, `publicKey`, Secure mode
- Asking a signer for a scheme with `Accept-Signature`: [RFC 9421](https://www.rfc-editor.org/rfc/rfc9421), search for: `Accept-Signature`, Requesting Signatures
