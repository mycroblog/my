# Mastodon fixtures

See [pkg/testkit/README.md](../../../README.md#federation-fixtures) for the layout and how to add a fixture.

- `circa-2018/` are Mastodon 2.2-era documents (February to May 2018, with the OStatus context and Linked Data signatures)
  from the test fixtures of Akkoma v3.20.1, © Pleroma Authors and Akkoma Authors, AGPL-3.0.
  Replaced in all of them: `localtesting.pleroma.lol` → `remote.example`, the users `lain` → `bob` and `gargron` → `alice`,
  the media host → `files.mastodon.example.org`, `publicKeyPem`, and `signatureValue`.
- `circa-2023/` is a January 2023 document of mastodon.social from the same Akkoma fixtures;
  `mastodon.social` → `mastodon.example` and the user → `alice`.
- `4.8/` was captured from a running Mastodon 4.8 server; it is the document of
  [peers/mastodon/actor.json](https://github.com/mycroblog/my/blob/main/internal/interface/activitypub/testdata/peers/mastodon/actor.json)
  (hosts, names, and texts already replaced there) with `publicKeyPem` replaced.
- `4.7/` was captured from Mastodon v4.7.3 by the live interoperability test
  ([test/federation-live](../../../../../test/federation-live/README.md)) on 2026-10-17: what Mastodon delivered to
  this server, as this server stored it. Replaced: `mastodon.test` → `mastodon.example`, `my.test` → `remote.example`,
  `publicKeyPem`, and the Linked Data `signatureValue`. The users (`alice`, `bob`) and the texts are the test's own.
- `4.7/outbox.json` and `4.7/outbox-page.json` are what Mastodon v4.7.3 serves for bob's outbox, rendered from its source
  (© Mastodon gGmbH and contributors, AGPL-3.0): `outbox_presenter` of `ActivityPub::OutboxesController` and the attributes of
  `ActivityPub::CollectionSerializer`, with `ActivityPub::OutboxSerializer` rendering the items. The items are the captured
  `4.7/create-note-unlisted.json`, `announce.json`, `create-note-reply.json`, and `create-note.json` without their `@context`
  (what an anonymous or instance-actor request sees: no followers-only or direct posts), newest first; the page's `@context` is
  that of `create-note.json`. With fewer items than `LIMIT` (20), the page has no `next`.
- `4.7/featured.json` is what Mastodon v4.7.3 serves for bob's featured collection, rendered from its source: `collection_presenter`
  of `ActivityPub::CollectionsController` and `ActivityPub::CollectionSerializer`, with `ActivityPub::NoteSerializer` rendering the
  pinned posts inline. The items are the objects of the captured `4.7/create-note.json` and `create-note-unlisted.json`, the newest
  pin first (the public post was pinned last); the `@context` is that of `4.7/outbox-page.json`.
- `4.7/featured-tags.json` is what Mastodon v4.7.3 serves for bob's featuredTags collection, rendered from its source: `set_items`
  (`tags`) and `collection_presenter` of `ActivityPub::CollectionsController` (an unordered `Collection`) and
  `ActivityPub::CollectionSerializer`, with `ActivityPub::HashtagSerializer` rendering each `FeaturedTag` (`href` is the profile's
  page of the tag, `name` the tag with `#`); the tags `cats` and `Photography` are the fixture's own.

| File | Source | License |
| --- | --- | --- |
| `4.7/follow.json` | capture of Mastodon 4.7.3, 2026-10-17: the `Follow` of bob | capture |
| `4.7/accept-follow.json` | capture of Mastodon 4.7.3, 2026-10-17: the `Accept` of alice's `Follow` | capture |
| `4.7/undo-follow.json` | capture of Mastodon 4.7.3, 2026-10-17: the `Undo` of bob's `Follow` | capture |
| `4.7/update-person.json` | capture of Mastodon 4.7.3, 2026-10-17: the `Update` of bob's actor (locked) | capture |
| `4.7/create-note.json` | capture of Mastodon 4.7.3, 2026-10-17: a public post | capture |
| `4.7/create-note-reply.json` | capture of Mastodon 4.7.3, 2026-10-17: a public reply to a post of alice, mentioning her | capture |
| `4.7/create-note-unlisted.json` | capture of Mastodon 4.7.3, 2026-10-17: an unlisted post | capture |
| `4.7/create-note-private.json` | capture of Mastodon 4.7.3, 2026-10-17: a followers-only post | capture |
| `4.7/create-note-direct.json` | capture of Mastodon 4.7.3, 2026-10-17: a direct post to alice | capture |
| `4.7/announce.json` | capture of Mastodon 4.7.3, 2026-10-17: a boost of a post of alice | capture |
| `4.7/undo-announce.json` | capture of Mastodon 4.7.3, 2026-10-17: the `Undo` of that boost | capture |
| `4.7/like.json` | capture of Mastodon 4.7.3, 2026-10-17: a favourite of a post of alice | capture |
| `4.7/undo-like.json` | capture of Mastodon 4.7.3, 2026-10-17: the `Undo` of that favourite | capture |
| `4.7/update-note.json` | capture of Mastodon 4.7.3, 2026-10-17: an edit of a public post | capture |
| `4.7/delete-note.json` | capture of Mastodon 4.7.3, 2026-10-17: the deletion of a public post | capture |
| `4.7/outbox.json` | `outbox_presenter` of [app/controllers/activitypub/outboxes_controller.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/controllers/activitypub/outboxes_controller.rb) and [app/serializers/activitypub/collection_serializer.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/serializers/activitypub/collection_serializer.rb) | AGPL-3.0 |
| `4.7/outbox-page.json` | the same, with the page's items by [app/serializers/activitypub/outbox_serializer.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/serializers/activitypub/outbox_serializer.rb): the captured posts and boost above | AGPL-3.0 |
| `4.7/featured.json` | `collection_presenter` and `set_items` of [app/controllers/activitypub/collections_controller.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/controllers/activitypub/collections_controller.rb) and [app/serializers/activitypub/collection_serializer.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/serializers/activitypub/collection_serializer.rb): the captured posts above | AGPL-3.0 |
| `4.7/featured-tags.json` | `set_items` and `collection_presenter` of [app/controllers/activitypub/collections_controller.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/controllers/activitypub/collections_controller.rb), [app/serializers/activitypub/collection_serializer.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/serializers/activitypub/collection_serializer.rb), and [app/serializers/activitypub/hashtag_serializer.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/serializers/activitypub/hashtag_serializer.rb) | AGPL-3.0 |
| `4.8/actor.json` | capture of a Mastodon 4.8 actor | capture |
| `circa-2018/follow.json` | [test/fixtures/mastodon-follow-activity.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/mastodon-follow-activity.json) | AGPL-3.0 |
| `circa-2018/accept-follow.json` | [test/fixtures/mastodon-accept-activity.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/mastodon-accept-activity.json) | AGPL-3.0 |
| `circa-2018/undo-follow.json` | [test/fixtures/mastodon-unfollow-activity.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/mastodon-unfollow-activity.json) | AGPL-3.0 |
| `circa-2018/create-note.json` | [test/fixtures/mastodon-post-activity.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/mastodon-post-activity.json) | AGPL-3.0 |
| `circa-2018/update-person.json` | [test/fixtures/mastodon-update.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/mastodon-update.json) | AGPL-3.0 |
| `circa-2018/delete-note.json` | [test/fixtures/mastodon-delete.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/mastodon-delete.json) | AGPL-3.0 |
| `circa-2018/delete-person.json` | [test/fixtures/mastodon-delete-user.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/mastodon-delete-user.json) | AGPL-3.0 |
| `circa-2018/announce.json` | [test/fixtures/mastodon-announce.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/mastodon-announce.json) | AGPL-3.0 |
| `circa-2018/like.json` | [test/fixtures/mastodon-like.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/mastodon-like.json) | AGPL-3.0 |
| `circa-2018/undo-like.json` | [test/fixtures/mastodon-undo-like.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/mastodon-undo-like.json) | AGPL-3.0 |
| `circa-2018/block.json` | [test/fixtures/mastodon-block-activity.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/mastodon-block-activity.json) | AGPL-3.0 |
| `circa-2018/undo-block.json` | [test/fixtures/mastodon-unblock-activity.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/mastodon-unblock-activity.json) | AGPL-3.0 |
| `circa-2023/note-language.json` | [test/fixtures/mastodon/note_with_language.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/mastodon/note_with_language.json) | AGPL-3.0 |
| `4.7/create-note-emoji.json` | `4.7/create-note.json` with a custom emoji `:blobcat:` in the text and the `tag` that [emoji_serializer.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/serializers/activitypub/emoji_serializer.rb) and [image_serializer.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/serializers/activitypub/image_serializer.rb) render for it (the `virtual_tags` of [note_serializer.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/serializers/activitypub/note_serializer.rb)), and `Emoji` in the context (`context_extensions :emoji`); the status id ends in `2` | AGPL-3.0 |
| `4.7/create-note-quote.json` | `4.7/create-note.json` as a quote of a post of carol (account `…614`), rendered from the source of v4.7.3: the `quote`, `_misskey_quote`, `quoteUri`, and `quoteAuthorization` of [note_serializer.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/serializers/activitypub/note_serializer.rb) and the `quote-inline` paragraph that `add_quote_fallback` of [text_formatter.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/lib/text_formatter.rb) puts before the text; the status id ends in `3`, and the `QuoteAuthorization` is at the `quote_authorizations` route of the numeric actor IRI (`approval_uri_for` of [tag_manager.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/lib/activitypub/tag_manager.rb)) | AGPL-3.0 |
| `4.7/quote-authorization.json` | the `QuoteAuthorization` of that quote, rendered by [quote_authorization_serializer.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/serializers/activitypub/quote_authorization_serializer.rb) with `context_extensions :quote_authorizations` | AGPL-3.0 |
| `4.8/actor-emoji.json` | `4.8/actor.json` with `:blobcat:` in `name` and `summary` and the same `Emoji` in `tag` (the `virtual_tags` of [actor_serializer.rb](https://github.com/mastodon/mastodon/blob/v4.7.3/app/serializers/activitypub/actor_serializer.rb)) | AGPL-3.0 |

No public source under a compatible license has a Mastodon `Flag` as JSON; one is added when it is captured.
