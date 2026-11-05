# GoToSocial fixtures

See [pkg/testkit/README.md](../../../README.md#federation-fixtures) for the layout and how to add a fixture.

`0.22/` comes from GoToSocial v0.22.2: the JSON that its own tests expect it to serialize, and the examples of its
federation documentation, © GoToSocial Authors, AGPL-3.0. Replaced in all of them: `localhost:8080` → `gotosocial.example`,
`fossbros-anonymous.io` → `remote.example`, `unknown-instance.com` and `somewhere.com` → `other.example`, and `publicKeyPem`.
The id of `update-person.json`, which the test generates, is an example value.
`follow.json` and `accept-follow.json` are what v0.22.2 sends, rendered from its source: `FollowToAS` builds the `Follow`, and
`AcceptFollow` embeds it in an `Accept` addressed to the follower, whose id has the form of `uris.GenerateURIForAccept`.
The follow is the one of `NewTestFollows` in `testrig/testmodels.go`; `remote.example` is the server GoToSocial talks to.
The files of the second table were captured from GoToSocial v0.22.1 by the live interoperability test
([test/federation-live](../../../../../test/federation-live/README.md)) on 2026-10-17: what GoToSocial delivered to this
server, as this server stored it. Replaced: `gts.test` → `gotosocial.example` and `my.test` → `remote.example`. The users
(`alice`, `carol`) and the texts are the test's own.

| File | Source | License |
| --- | --- | --- |
| `0.22/actor.json` | `TestAccountToAS` in [internal/typeutils/internaltoas_test.go](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/fb2bf1330c355f19c6f3614d7434c56f8d55aeaf/internal/typeutils/internaltoas_test.go) | AGPL-3.0 |
| `0.22/create-note.json` | `TestWrapNoteInCreate` in [internal/typeutils/wrap_test.go](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/fb2bf1330c355f19c6f3614d7434c56f8d55aeaf/internal/typeutils/wrap_test.go) | AGPL-3.0 |
| `0.22/update-person.json` | `TestWrapAccountableInUpdate` in [internal/typeutils/wrap_test.go](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/fb2bf1330c355f19c6f3614d7434c56f8d55aeaf/internal/typeutils/wrap_test.go) | AGPL-3.0 |
| `0.22/announce.json` | `TestSelfBoostFollowersOnlyToAS` in [internal/typeutils/internaltoas_test.go](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/fb2bf1330c355f19c6f3614d7434c56f8d55aeaf/internal/typeutils/internaltoas_test.go) | AGPL-3.0 |
| `0.22/flag.json` | `TestReportToAS` in [internal/typeutils/internaltoas_test.go](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/fb2bf1330c355f19c6f3614d7434c56f8d55aeaf/internal/typeutils/internaltoas_test.go) | AGPL-3.0 |
| `0.22/accept-reply-request.json` | `TestAcceptRemoteReplyRequest` in [internal/federation/federatingdb/accept_test.go](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/fb2bf1330c355f19c6f3614d7434c56f8d55aeaf/internal/federation/federatingdb/accept_test.go) | AGPL-3.0 |
| `0.22/delete-note.json` | the first `Delete` example of [docs/federation/posts.md](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/fb2bf1330c355f19c6f3614d7434c56f8d55aeaf/docs/federation/posts.md) | AGPL-3.0 |
| `0.22/follow.json` | `FollowToAS` in [internal/typeutils/internaltoas.go](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/fb2bf1330c355f19c6f3614d7434c56f8d55aeaf/internal/typeutils/internaltoas.go) | AGPL-3.0 |
| `0.22/outbox.json` | `TestGetOutbox` in [internal/api/activitypub/users/outboxget_test.go](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/fb2bf1330c355f19c6f3614d7434c56f8d55aeaf/internal/api/activitypub/users/outboxget_test.go) | AGPL-3.0 |
| `0.22/featured.json` | `TestPinnedStatusesToASSomeItems` in [internal/typeutils/internaltoas_test.go](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/fb2bf1330c355f19c6f3614d7434c56f8d55aeaf/internal/typeutils/internaltoas_test.go) (the pinned posts of admin, as IRIs) | AGPL-3.0 |
| `0.22/outbox-page.json` | `TestGetOutboxFirstPage` in [internal/api/activitypub/users/outboxget_test.go](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/fb2bf1330c355f19c6f3614d7434c56f8d55aeaf/internal/api/activitypub/users/outboxget_test.go) (the test drops `published` from the items) | AGPL-3.0 |
| `0.22/accept-follow.json` | `AcceptFollow` in [internal/processing/workers/federate.go](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/fb2bf1330c355f19c6f3614d7434c56f8d55aeaf/internal/processing/workers/federate.go) and `GenerateURIForAccept` in [internal/uris/uri.go](https://codeberg.org/superseriousbusiness/gotosocial/src/commit/fb2bf1330c355f19c6f3614d7434c56f8d55aeaf/internal/uris/uri.go) | AGPL-3.0 |

| File | Source | License |
| --- | --- | --- |
| `0.22/create-note-reply.json` | capture of GoToSocial 0.22.1, 2026-10-17: a public reply to a post of alice, mentioning her | capture |
| `0.22/create-note-unlisted.json` | capture of GoToSocial 0.22.1, 2026-10-17: an unlisted post | capture |
| `0.22/create-note-private.json` | capture of GoToSocial 0.22.1, 2026-10-17: a followers-only post | capture |
| `0.22/create-note-direct.json` | capture of GoToSocial 0.22.1, 2026-10-17: a direct post to alice | capture |
| `0.22/undo-follow.json` | capture of GoToSocial 0.22.1, 2026-10-17: the `Undo` of carol's `Follow` | capture |
| `0.22/undo-announce.json` | capture of GoToSocial 0.22.1, 2026-10-17: the `Undo` of a boost of a post of alice | capture |
| `0.22/like.json` | capture of GoToSocial 0.22.1, 2026-10-17: a favourite of a post of alice | capture |
| `0.22/undo-like.json` | capture of GoToSocial 0.22.1, 2026-10-17: the `Undo` of that favourite | capture |
| `0.22/update-note.json` | capture of GoToSocial 0.22.1, 2026-10-17: an edit of a public post | capture |
