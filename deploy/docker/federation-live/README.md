# Live interoperability test

This server, Mastodon, GoToSocial, and Misskey federating with each other over HTTPS, with an Activity-Relay relay
between this server and Mastodon, for the live test of
[federation.md §12](../../../docs/architecture/federation.md#12-テスト) ([ADR-0006](../../../docs/adr/0006-federation-protocol-scope.md),
[ADR-0015](../../../docs/adr/0015-testing-strategy.md)). CI runs it nightly (job `federation-live` in
`.github/workflows/nightly.yml`); it is not part of `scripts/task.sh check`.

| File | What it is |
| --- | --- |
| `compose.yaml` | The stack: the TLS proxy, PostgreSQL (this server, Mastodon, and Misskey), Redis, this server built from the checkout, Mastodon (web and Sidekiq), GoToSocial, Misskey with its configuration file, and Activity-Relay (server and worker) |
| `Caddyfile` | The proxy: one certificate of the private CA for `my.test`, `mastodon.test`, `gts.test`, `misskey.test`, and `relay.test`, forwarded by host name |
| `postgres-init.sh` | The roles and databases of this server, prepared with `schema/postgres.bootstrap.sql`, and of Misskey |
| `*_test.go` | The scenarios (`go test -tags federationlive`), black-box clients of this server's client API, the Mastodon client API (Mastodon and GoToSocial), and the Misskey API (`misskey_test.go`) |
| `../../../scripts/federation-live.sh` | Generates the CA and the secrets, starts the stack, creates the users, runs the scenarios, collects the logs, and removes the stack |

## How it works

- **TLS**: the script creates a private CA and one certificate for the five hosts. Every server trusts only that CA
  (`SSL_CERT_FILE`; Misskey's `NODE_EXTRA_CA_CERTS`).
- **DNS**: the proxy holds the network aliases `my.test`, `mastodon.test`, `gts.test`, `misskey.test`, and
  `relay.test`, so each server reaches the others, and itself, through the proxy, as on the internet. The servers may reach the private
  addresses of the compose network only (`MY_HTTP_ALLOW_PRIVATE_NETWORKS` in the `development` profile, Mastodon's
  `ALLOWED_PRIVATE_ADDRESSES`, GoToSocial's `http-client.allow-ips`, Misskey's `allowedPrivateNetworks`).
- **Users**: created with each server's admin CLI, and on Misskey with its API (`admin/accounts/create`: the first
  account, `mkadmin`, is the administrator and creates the others). The scenarios sign in to this server and GoToSocial
  through their OAuth screens; the script issues the Mastodon tokens with a Rails runner, and Misskey returns each
  user's API token when it creates the user.

  | User | Role |
  | --- | --- |
  | `alice@my.test` | The local user of every scenario, the moderator of `TestSuspension`, and the administrator who closes the server in `TestClosure` |
  | `frank@my.test` | `TestSuspension`: alice suspends him and lifts it |
  | `bob@mastodon.test`, `carol@gts.test`, `grace@misskey.test` | The peer users that alice follows and that follow her |
  | `dave@mastodon.test`, `erin@gts.test`, `heidi@misskey.test` | Never follow alice: the mentioned non-followers of `TestVisibilityAudiences` |
  | `eve@mastodon.test` | `TestSignatureProfile`: the proxy refuses her draft-cavage POSTs in both directions |
  | `mod@mastodon.test`, `mkadmin@misskey.test` | The administrators whose tokens read the moderators' reports in `TestReportForwarding` |
  | `ivan@my.test`, `judy@mastodon.test` | `TestRelay`: nobody follows them, so their posts reach the other server only through the relay |
  | `otto@my.test` | `TestFederationMode`: no peer knows him before the scenario, so his `Follow` is the first a peer learns of him |
  | `leavemastodon@my.test`, `leavegotosocial@my.test`, `leavemisskey@my.test` | `TestMove`: each moves to `arrive` of its peer, `leavemisskey` to `arrive@my.test` (Compatibility notes) |
  | `arrive@my.test` | `TestMove`: `leave` of every peer and `leavemisskey@my.test` move to her |
  | `arrive@`, `leave@` `mastodon.test`, `gts.test`, `misskey.test` | `TestMove`: `leave` moves to `arrive@my.test`, and `leave<peer>@my.test` moves to `arrive`. Mastodon moves once every 30 days and takes one `Move` of an account every 7 days, so every move of a run has accounts of its own |

- **Signatures**: to exercise RFC 9421, which Mastodon and this server only use after a 401, the proxy answers 401
  to a draft-cavage POST signed by eve (to this server) or addressed to eve's inbox (to Mastodon). The script
  recreates the proxy with eve's actor URI once Mastodon has numbered her (`FEDERATION_LIVE_SIGNER_ACTOR`,
  `FEDERATION_LIVE_SIGNER_INBOX`).
- **Mastodon** runs in secure mode (`AUTHORIZED_FETCH`), so every fetch this server makes from it must be signed.
- **The relay** is [Activity-Relay](https://github.com/yukimochi/Activity-Relay) (AGPL-3.0), configured by environment
  variables (no `config.yml`) with an actor key that the script generates, accepting every subscription. The script
  makes Mastodon join it at its inbox (`Relay.create!(...).enable!` in a Rails runner); `TestRelay` makes this server
  join it through the admin API (ADR-0061). The relay forwards public posts as they are, so Mastodon takes this
  server's posts only with their linked data signature.
- **Misskey** federates with no one until its administrator sets `federation` to `all` (`admin/update-meta`), and
  its API limits how often a user may call an endpoint (`ap/show` 30 times an hour); the script sets both, the
  latter with a rate limit factor of 0 for every user (`admin/roles/update-default-policies`).
- **The Misskey API** is driven in the terms of the Mastodon client API (`misskeyAPI` in `misskey_test.go`):
  `home` / `followers` / `specified` are `unlisted` / `private` / `direct` (a `specified` note is for the users
  that its text mentions), a renote is a boost, and a reaction (👍) is a favourite.
- The scenarios reach the servers through the proxy's published port and read what this server recorded
  (`received_activities`) from the published PostgreSQL port. `TestFederationMode` also reads the proxy's access log
  with `docker compose logs`, to log which requests a peer made to this server (`TestClosure` reads it too, for the
  deliveries that a peer made after the closure), and `TestMove` sets the aliases and moves of Mastodon users with
  `docker compose exec mastodon-web bin/rails runner`.

## Running it

```sh
scripts/task.sh docker:federation-live                         # every scenario
scripts/task.sh docker:federation-live FEDERATION_LIVE_FLAGS='-run TestFollowFromPeer'
FEDERATION_LIVE_KEEP=1 scripts/task.sh docker:federation-live  # keep the stack running afterwards
```

It needs Docker with Compose, Go, curl, jq, and openssl. The proxy publishes port 18443 and PostgreSQL port 15432 on
127.0.0.1 (`FEDERATION_LIVE_HTTPS_PORT`, `FEDERATION_LIVE_DB_PORT`); the compose network is `172.29.58.0/24`
(`FEDERATION_LIVE_SUBNET`). The output goes to `.tmp/federation-live/out` (`FEDERATION_LIVE_OUT`):

| Path | Content |
| --- | --- |
| `logs/<service>.log`, `logs/ps.txt` | The log of every container (the proxy's access log has every request's headers) and their final state |
| `captured/<host>/<nnn>-<type>.json` | Every activity this server received, as it stored it, by the sender's host |
| `scenarios.log` | The `go test -v` output |

With `FEDERATION_LIVE_KEEP=1`, rerun the scenarios against the running stack with the variables the script wrote,
and remove the stack when done:

```sh
set -a; . .tmp/federation-live/scenarios.env; set +a
go test -tags federationlive -count=1 -timeout 45m -v ./deploy/docker/federation-live
docker compose -p my-federation-live down -v
```

In the browser, the servers are at `https://my.test:18443` and so on once the host names point at 127.0.0.1 and the
CA (`.tmp/federation-live/certs/ca.pem`) is trusted.

## Scenarios

Each scenario runs against every peer, in this order: the later ones start from the follows that the earlier ones
leave (a scenario that runs alone makes the follows it needs).

| Scenario | Checks |
| --- | --- |
| `TestFederationMode` | The federation direction ([ADR-0076](../../../docs/adr/0076-federation-direction.md)), with otto, in the modes `inbound`, `outbound`, `off`, and `both` again. `inbound`: otto's follow of the peer user is accepted although his actor is the minimal document, the peer's posts reach him, and the peer user's follow of him is rejected and stays pending on neither side; the scenario logs how the peer shows otto and the GETs (WebFinger, actor) it made to this server, from the proxy's access log. `outbound`: the peer user's follow is accepted and otto's post reaches the peer; the peer's post, reply, and favourite reach the inbox and show nowhere. `off`: otto's actor, WebFinger, and NodeInfo answer 404; the peer, made to fetch otto again (Mastodon and Misskey get the 404; GoToSocial answers from what it stored and fetches nothing), keeps him and the follow. `both`: otto's next post reaches the peer. The mode is the whole server's: the scenario runs first (its file sorts first) and sets the mode back to `both` |
| `TestMove` | The move of an account ([ADR-0089](../../../docs/adr/0089-account-migration-move.md), FR-414) in both directions. Out: the peer user follows `leave<peer>@my.test`, `arrive` of the peer names it among its aliases, it moves there (`POST /accounts/move`), and the peer user follows `arrive` instead (on Misskey, the target is `arrive@my.test`: Compatibility notes). In: alice follows `leave` of the peer, `arrive@my.test` names it among her aliases (`POST /accounts/aliases`), it moves to `arrive@my.test`, and alice follows `arrive@my.test` instead and has an `account_moved` notification with the outcome `followed`. The aliases and moves on the peer: Mastodon's settings run in a Rails runner (`AccountAlias`, `AccountMigration` and `MoveService`; it has no client API for them), GoToSocial's `POST /api/v1/accounts/alias` and `/move`, and Misskey's `i/update` (`alsoKnownAs`) and `i/move`. Akkoma, which is not in the stack, has the regression test of its `Move` with fixtures instead (`AccountMoveContract`, `pkg/testkit/fixtures/federation/akkoma/3.20/move.json`) |
| `TestPeerResolvesLocal` | The peer finds `alice@my.test` through WebFinger and fetches her actor |
| `TestLocalResolvesPeer` | This server finds the peer user through WebFinger and fetches the actor with a signed GET |
| `TestFollowFromPeer` | The peer's signed `Follow` reaches this server's inbox and is verified and recorded; then it is accepted |
| `TestFollowFromLocal` | alice follows the peer user and the peer accepts |
| `TestPostFromPeer` / `TestPostFromLocal` | A public post reaches the follower's home timeline on the other side |
| `TestUnfollow` | An unfollow in either direction (`Undo Follow`) ends the follow on the other side |
| `TestFollowApproval` | A follow of a locked account waits until its user approves it, in either direction |
| `TestReply` | A reply in either direction is among the descendants of the status it replies to; the reply to alice notifies her |
| `TestBoost` / `TestFavourite` | A boost / favourite in either direction and its undo change the count on the author's side. Misskey's favourite (a 👍 reaction) arrives on this server as a reaction: the 👍 count changes, the favourite count stays 0, and alice gets a `reaction` notification ([ADR-0052](../../../docs/adr/0052-emoji-reactions.md)) |
| `TestEdit` / `TestDelete` | An edited post shows the new text / a deleted post is gone on the other side. Misskey, which does not edit, gets the `Update` of alice's edit with a 2xx and keeps the text before it |
| `TestVisibilityAudiences` | Each visibility reaches exactly its audience in both directions; a `local` post that mentions a non-follower reaches the follower but the mentioned user can neither see it nor is notified ([ADR-0017](../../../docs/adr/0017-visibility-local.md), [ADR-0035](../../../docs/adr/0035-local-visibility-mention-tags.md)) |
| `TestReportForwarding` | A report of the peer user that alice forwards reaches the moderators' reports of Mastodon and Misskey (a `Flag` of this server's instance actor with the comment); one that she does not forward does not. GoToSocial is skipped: the scenarios have no administrator of it |
| `TestSignatureProfile` | The rows of [ADR-0032](../../../docs/adr/0032-http-signature-profile.md) that need a real server: draft-cavage from every peer, RFC 9421 from Mastodon after a 401, this server's RFC 9421 resend after a 401 (double-knocking), and `Accept-Signature` on an unsigned POST. It runs after the other scenarios of `scenarios_test.go`: afterwards this server signs for `mastodon.test` with RFC 9421 first |
| `TestSuspension` | Mastodon only: Mastodon shows a local user that a moderator suspends as suspended (the actor's `Update` with `suspended: true`, through Mastodon's shared inbox), and shows him again once the suspension is lifted ([ADR-0036](../../../docs/adr/0036-moderation-federation-propagation.md)) |
| `TestClosure` | The closure of the service ([ADR-0088](../../../docs/adr/0088-service-shutdown-modes.md), FR-809). alice and the peer user follow each other and the peer stores a post of alice; alice announces a closure whose date has passed and closes the server through the admin API, and the peer user posts at once, so that the peer delivers to an inbox that answers 410. While the Deletes go out, the inbox answers 410 to an unsigned POST, WebFinger 410, and alice's actor the suspended document with its key. Each peer takes the `Delete` (Actor): alice and her post are gone there; its user posts again, and the peer sends this server nothing more (no retry of a 410, no delivery to the deleted followers); every POST it made after the closure got 410, and Misskey marks the server `goneSuspended` once one of them did. Then `delivery_completed` is recorded and the actor answers 410 with a `Tombstone`. The closure cannot be undone and ends the federation of the whole server, so the scenario runs last (its file, `zz_closure_test.go`, sorts last) and a stack kept with `FEDERATION_LIVE_KEEP=1` after it runs serves no other scenario. Akkoma, which is not in the stack, is not checked |

## Compatibility notes

What the scenarios show about the pinned peers, and how this server and the scenarios deal with it:

| Peer | Behavior | Handling |
| --- | --- | --- |
| Mastodon 4.7 | Actors created by 4.7 have numeric URIs (`/ap/users/{id}`, not `/users/{name}`) and sign with the key `{actor}#rsa-{hash}`, not `{actor}#main-key` | Nothing to do: keys are resolved by the `keyId` value itself ([ADR-0032](../../../docs/adr/0032-http-signature-profile.md)); the proxy learns eve's URI from the script |
| Mastodon 4.7 | Signs deliveries with draft-cavage (`host date content-type digest (request-target)`) and resends them with RFC 9421 (`("@method" "@target-uri" "content-digest");created;keyid`, no `alg` and no `expires`) after a 400 / 401; verifies RFC 9421 by default | Both verify (ADR-0032); `TestSignatureProfile` |
| Mastodon 4.7 | A mention of a user who is neither addressed nor a follower is filtered (notification requests), not dropped, and lets the user see a `private` post | This server leaves such mentions out of the `tag` of a `local` post ([ADR-0035](../../../docs/adr/0035-local-visibility-mention-tags.md)); the scenario reads the filtered notifications too (`include_filtered`) |
| GoToSocial 0.22 | Signs with draft-cavage only (`hs2019`); has no shared inbox, so every delivery goes to the user's inbox | Nothing to do |
| GoToSocial 0.22 | Accounts that `admin account create` makes are locked | The scenarios unlock carol and erin, and `TestFollowApproval` locks them itself |
| GoToSocial 0.22 | The first page of the home timeline comes from a cache that does not always take a status that arrives after the cache was filled; the status is in the database | The scenarios read GoToSocial's home timeline with a `max_id`, which reads the database |
| GoToSocial 0.22 | Logs `error fetching account featured collection … empty url host` for this server's instance actor, which has no `featured` | Harmless: the actor is still stored and the signature verified |
| Misskey 2026.10 | Signs with draft-cavage only (`rsa-sha256`, `{actor}#main-key`); delivers to this server's shared inbox | Nothing to do |
| Misskey 2026.10 | A new server federates with no one (`federation: none`) and limits the API calls of each user | The script opens federation and lifts the limits (How it works) |
| Misskey 2026.10 | A favourite is a reaction: a `Like` with the emoji in `content` and `_misskey_reaction`; a `Like` without one shows as Misskey's default reaction | This server records it as a reaction with the emoji, not as a favourite ([ADR-0052](../../../docs/adr/0052-emoji-reactions.md), [federation.md §3](../../../docs/architecture/federation.md#3-受信inbox)); a favourite from this server (a `Like` without an emoji) shows on Misskey as its default reaction |
| Misskey 2026.10 | Cannot edit a note, and its inbox skips an `Update` of a `Note` (`skip: Unknown type: Note`, answered 202) | `TestEdit` checks that the `Update` is delivered and that the text stays |
| Misskey 2026.10 | `renoteCount` never goes down: deleting a renote, also on an `Undo Announce`, leaves it as it was | The scenarios count the renotes that exist (`notes/renotes`) |
| Misskey 2026.10 | Answers an `Undo Follow` with a `Reject` of the follow (`UserFollowingService.unfollow`), whose `Follow` has the id `/follows/{follower}/{followee}` | This server drops it (`inbox activity dropped`, the follow has already ended) |
| Misskey 2026.10 | A `followers` note is visible to every user its `tag` mentions, addressed or not (`NoteEntityService.isVisibleForMe` reads the `mentions` that `ApNoteService` takes from `tag`), and each of them gets a `mention` notification (`NoteCreateService.createMentionedEvents`) | This server leaves such mentions out of the `tag` of a `local` post ([ADR-0035](../../../docs/adr/0035-local-visibility-mention-tags.md)); `TestVisibilityAudiences` checks that heidi neither sees it nor is notified |
| Misskey 2026.10 | The API answers a missing note with a 400 and the code `NO_SUCH_NOTE` | The scenarios read it as a 404 |
| Mastodon 4.7, GoToSocial 0.22 | Accept a `Follow` from the minimal actor document of the `inbound` mode (no name, icon, or collections), fetch the actor, and then look the handle up through WebFinger (`acct:otto@my.test`); they also try the collections of the instance actor, which answer 404 in that mode | Nothing to do: WebFinger answers in `inbound` ([ADR-0076](../../../docs/adr/0076-federation-direction.md)); `TestFederationMode` logs the requests |
| Misskey 2026.10 | Accepts a `Follow` from the minimal actor document and shows the follower without a WebFinger lookup | Nothing to do |
| GoToSocial 0.22 | Keeps the minimal actor document of `inbound` (no `followers`) after the mode widens, and leaves otto's posts out of carol's home timeline, from its database too, though it stores them and lists them on his profile | `TestFederationMode` reads the author's statuses instead of the home timeline |
| Misskey 2026.10 | Fails the `Move` of a remote account to one of its own accounts: `ApPersonService.processRemoteMove` looks the local target up by `uri`, which a local user does not have (`EntityNotFoundError`, logged as `Processing Move Failed`), so its followers stay with the source, whoever sends the `Move`. A `Move` to an account of another server works | `TestMove` moves `leavemisskey@my.test` to `arrive@my.test`, an account of another server to Misskey |
| Mastodon 4.7, GoToSocial 0.22 | Take this server's `Move` to one of their own accounts, follow the target with the follower, and end the follow of the source | Nothing to do |
| Mastodon 4.7, GoToSocial 0.22, Misskey 2026.10 | Take the closure's `Delete` (Actor) from the suspended actor document and delete the account and its posts; do not retry a delivery that the closed inbox answered with 410. Misskey also marks the closed server `goneSuspended` (`isSuspended`, `isNotResponding`) | Nothing to do: `TestClosure` ([ADR-0088](../../../docs/adr/0088-service-shutdown-modes.md)) |
| Every peer | None learns that alice locked her account while she has no follower there (the actor `Update` goes to followers only) | Expected: the follow request still waits for her approval on this server |

## Adding a captured activity as a fixture

The activities under `captured/` (also in the `federation-live` artifact of every CI run) are real documents of the
pinned Mastodon, GoToSocial, and Misskey versions, so they can become fixtures of the regression tests
([pkg/testkit/README.md](../../../pkg/testkit/README.md#federation-fixtures)):

1. Pick the document, for example `captured/mastodon.test/001-follow.json`.
2. Follow "Adding a fixture" in `pkg/testkit/README.md`: replace the hosts (`mastodon.test` → `mastodon.example`,
   `my.test` → `remote.example`, `gts.test` → `gotosocial.example`, `misskey.test` → `misskey.example`), the keys, and the texts, and save it as
   `pkg/testkit/fixtures/federation/<implementation>/<version>/<name>.json` with the version of the image in
   `compose.yaml` (`mastodon/4.7/follow.json`).
3. Record it in `pkg/testkit/fixtures/federation/<implementation>/README.md` as a capture from that server version, with
   the capture date, and update the golden files.

The stored document has the bytes of the request body as this server received it, key order and whitespace
included; reformat it (`jq --indent 2 .`) before saving it as a fixture.

## Updating the peers

The images are pinned by tag and digest in `compose.yaml`. To move to a new Mastodon, GoToSocial, or Misskey release, take a tag
at least three days old, put its multi-arch digest (`docker buildx imagetools inspect <image>:<tag>`) next to it, and
run `scripts/task.sh docker:federation-live` to see what changed.
