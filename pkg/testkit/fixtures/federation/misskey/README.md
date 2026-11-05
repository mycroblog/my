# Misskey fixtures

See [pkg/testkit/README.md](../../../README.md#federation-fixtures) for the layout and how to add a fixture.

- `circa-2019/` and `circa-2022/` are documents of Misskey servers from the test fixtures of Akkoma v3.20.1,
  © Pleroma Authors and Akkoma Authors, AGPL-3.0. The sending server's host → `misskey.example`, other servers → `remote.example`
  (`example.com` included), and the mentioned user → `bob`.
- `2025.4/` was captured from a running Misskey 2025.4 server; it is the document of
  [peers/misskey/actor.json](https://github.com/mycroblog/my/blob/main/internal/interface/activitypub/testdata/peers/misskey/actor.json)
  (hosts, names, and texts already replaced there) with `publicKeyPem` replaced.
- `2025.4/follow.json`, `accept-follow.json`, and `undo-follow.json` are what Misskey 2025.4.0 sends, rendered from its source
  (© misskey-dev, AGPL-3.0): `UserFollowingService` wraps `ApRendererService.renderFollow` / `renderAccept` / `renderUndo` in
  `addContext`, which adds `CONTEXT` of `misc/contexts.ts` and a random-UUID `id`. The ids of users and follows are example values;
  `remote.example` is the server Misskey talks to. The `Undo` names the follow by `/follows/{follower}/{followee}`, not by
  the id of the `Follow` it sent.
- `2026.10/` was captured from Misskey 2026.10.0 by the live interoperability test
  ([test/federation-live](../../../../../test/federation-live/README.md)) on 2026-10-17: what Misskey
  delivered to this server, as this server stored it. Replaced: `misskey.test` → `misskey.example`, `my.test` →
  `remote.example`, and `publicKeyPem`. The users (`alice`, `grace`) and the texts are the test's own.
- `2026.10/outbox.json` and `2026.10/outbox-page.json` are what Misskey 2026.10.0 serves for grace's outbox, rendered from its
  source (© misskey-dev, AGPL-3.0): `outbox` of `ActivityPubServerService` builds them with `renderOrderedCollection` and
  `renderOrderedCollectionPage` of `ApRendererService` and wraps them in `addContext`; `packActivity` renders the items. The items
  are the captured `2026.10/create-note-unlisted.json`, `announce.json`, `create-note-reply.json`, and `create-note.json` without
  their `@context` (the outbox lists only `public` and `home` notes), newest first.
- `2026.10/featured.json` is what Misskey 2026.10.0 serves for grace's featured collection, rendered from its source: `featured` of
  `ActivityPubServerService` renders the pinned `public` and `home` notes with `renderNote` into `renderOrderedCollection` and wraps
  it in `addContext` (the `@context` of `2026.10/outbox.json`). The items are the objects of the captured `2026.10/create-note.json`
  and `create-note-unlisted.json`, the newest pin first (the public note was pinned last).

| File | Source | License |
| --- | --- | --- |
| `2025.4/actor.json` | capture of a Misskey 2025.4 actor | capture |
| `2025.4/follow.json` | `requestFollow` of [UserFollowingService.ts](https://github.com/misskey-dev/misskey/blob/2025.4.0/packages/backend/src/core/UserFollowingService.ts) and `renderFollow` of [ApRendererService.ts](https://github.com/misskey-dev/misskey/blob/2025.4.0/packages/backend/src/core/activitypub/ApRendererService.ts) | AGPL-3.0 |
| `2025.4/accept-follow.json` | `acceptFollowRequest` of [UserFollowingService.ts](https://github.com/misskey-dev/misskey/blob/2025.4.0/packages/backend/src/core/UserFollowingService.ts) and `renderAccept` of [ApRendererService.ts](https://github.com/misskey-dev/misskey/blob/2025.4.0/packages/backend/src/core/activitypub/ApRendererService.ts) | AGPL-3.0 |
| `2025.4/undo-follow.json` | `unfollow` of [UserFollowingService.ts](https://github.com/misskey-dev/misskey/blob/2025.4.0/packages/backend/src/core/UserFollowingService.ts) and `renderUndo` of [ApRendererService.ts](https://github.com/misskey-dev/misskey/blob/2025.4.0/packages/backend/src/core/activitypub/ApRendererService.ts) | AGPL-3.0 |
| `circa-2019/question.json` | [test/fixtures/tesla_mock/misskey_poll_no_end_date.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/tesla_mock/misskey_poll_no_end_date.json) | AGPL-3.0 |
| `circa-2022/create-note.json` | [test/fixtures/tesla_mock/mk.absturztau.be-93e7nm8wqg-activity.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/tesla_mock/mk.absturztau.be-93e7nm8wqg-activity.json) | AGPL-3.0 |
| `circa-2022/like-reaction.json` | [test/fixtures/custom-emoji-reaction.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/custom-emoji-reaction.json) | AGPL-3.0 |
| `circa-2022/note-quote.json` | [test/fixtures/misskey/quote.json](https://akkoma.dev/AkkomaGang/akkoma/src/commit/98dd958886a8b0ef65d5bd827aba3789f1dc5116/test/fixtures/misskey/quote.json) | AGPL-3.0 |

| File | Source | License |
| --- | --- | --- |
| `2026.10/follow.json` | capture of Misskey 2026.10.0, 2026-10-17: grace's `Follow` of alice | capture |
| `2026.10/accept-follow.json` | capture of Misskey 2026.10.0, 2026-10-17: grace's `Accept` of alice's `Follow`, which embeds it | capture |
| `2026.10/reject-follow.json` | capture of Misskey 2026.10.0, 2026-10-17: the `Reject` that answers alice's `Undo Follow`; the embedded `Follow` has the id `/follows/{follower}/{followee}` | capture |
| `2026.10/undo-follow.json` | capture of Misskey 2026.10.0, 2026-10-17: the `Undo` of grace's `Follow` | capture |
| `2026.10/update-person.json` | capture of Misskey 2026.10.0, 2026-10-17: the `Update` of grace's actor when she unlocks her account | capture |
| `2026.10/create-note.json` | capture of Misskey 2026.10.0, 2026-10-17: a public note | capture |
| `2026.10/create-note-reply.json` | capture of Misskey 2026.10.0, 2026-10-17: a public reply to a post of alice, mentioning her | capture |
| `2026.10/create-note-unlisted.json` | capture of Misskey 2026.10.0, 2026-10-17: a `home` note | capture |
| `2026.10/create-note-private.json` | capture of Misskey 2026.10.0, 2026-10-17: a `followers` note | capture |
| `2026.10/create-note-direct.json` | capture of Misskey 2026.10.0, 2026-10-17: a `specified` note for alice | capture |
| `2026.10/announce.json` | capture of Misskey 2026.10.0, 2026-10-17: a renote of a post of alice | capture |
| `2026.10/undo-announce.json` | capture of Misskey 2026.10.0, 2026-10-17: the `Undo` of that renote | capture |
| `2026.10/like-reaction.json` | capture of Misskey 2026.10.0, 2026-10-17: a 👍 reaction to a post of alice (`Like` with `content` and `_misskey_reaction`) | capture |
| `2026.10/undo-like-reaction.json` | capture of Misskey 2026.10.0, 2026-10-17: the `Undo` of that reaction | capture |
| `2026.10/delete-note.json` | capture of Misskey 2026.10.0, 2026-10-17: the `Delete` of a note | capture |
| `2026.10/outbox.json` | `outbox` of [server/ActivityPubServerService.ts](https://github.com/misskey-dev/misskey/blob/2026.10.0/packages/backend/src/server/ActivityPubServerService.ts) and `renderOrderedCollection` of [core/activitypub/ApRendererService.ts](https://github.com/misskey-dev/misskey/blob/2026.10.0/packages/backend/src/core/activitypub/ApRendererService.ts) | AGPL-3.0 |
| `2026.10/featured.json` | `featured` of [server/ActivityPubServerService.ts](https://github.com/misskey-dev/misskey/blob/2026.10.0/packages/backend/src/server/ActivityPubServerService.ts) and `renderOrderedCollection` of [core/activitypub/ApRendererService.ts](https://github.com/misskey-dev/misskey/blob/2026.10.0/packages/backend/src/core/activitypub/ApRendererService.ts): the captured notes above | AGPL-3.0 |
| `2026.10/outbox-page.json` | the same, with `renderOrderedCollectionPage` and `packActivity`: the captured notes and renote above | AGPL-3.0 |
| `2026.10/create-note-emoji.json` | `2026.10/create-note.json` with a custom emoji `:blobcat:` in the text and the `tag` that `renderEmoji` of [ApRendererService.ts](https://github.com/misskey-dev/misskey/blob/2026.10.0/packages/backend/src/core/activitypub/ApRendererService.ts) renders for it (`renderNote`); the note id ends in `e` | AGPL-3.0 |
| `2025.4/actor-emoji.json` | `2025.4/actor.json` with `:blobcat:` in `name` and `summary` and the `tag` of `renderPerson` with `renderEmoji` of [ApRendererService.ts](https://github.com/misskey-dev/misskey/blob/2025.4.0/packages/backend/src/core/activitypub/ApRendererService.ts) | AGPL-3.0 |
