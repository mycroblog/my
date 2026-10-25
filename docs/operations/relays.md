# Federation relays

A guide for operators.

A relay is a server that passes the public posts of the servers that join it on to the other joined servers. Even a small instance with few follows gets a filled federated timeline and hashtag timelines,
and its own public posts reach further. An instance that does not join a relay gains no rows and no jobs.

## Join a relay

Use "Relays" in the admin area (`/admin/relays`). It requires the permission `manage_settings`, which the built-in `admin` role has.
The CLI is `my admin relay` (`my admin relay -help`).

1. Register the relay's URL. There are two protocols:
   - **Mastodon**: the relay's **inbox URL** (such as `https://relay.example/inbox`). Many relays, including pub-relay and Activity-Relay, accept this.
   - **LitePub**: the relay's **actor URL** (such as `https://relay.example/relay`). This is the protocol of Pleroma and Akkoma relays.
   If you do not choose a protocol ("Detect automatically"), the server uses LitePub when the URL serves an actor document, and Mastodon otherwise.
2. After you register it, the instance actor sends the relay a `Follow`, and the state is "Awaiting the relay's answer" (`pending`).
   When the relay accepts, the state becomes "Joined" (`accepted`); when it refuses, "Rejected by the relay" (`rejected`). Some relays take a while because an operator approves each request by hand.
3. If you turn off "Receive posts from the relay" when registering, the server only sends. To change this, leave the relay and register it again.

You cannot register the relay of a server under a domain action (`suspend`). If the server of a joined relay is suspended later, the state does not change,
but nothing is sent to or received from that relay, and the admin area shows a warning.

## What the server sends

- Only the creation, edit and deletion of local **public** (`public`) posts are sent. Posts that are `unlisted`, followers-only, direct or local-only (`local`), and boosts, are never sent.
  To keep a post off relays, post it as `unlisted` or `local`. There is no per-user opt-out.
- The documents of public posts carry a Linked Data Signature (`RsaSignature2017`), so that Mastodon and Misskey can accept posts that a relay passes on unchanged.
- The number of deliveries grows by the number of joined relays times the number of public posts (deliveries to a relay are merged into one).

## What the server receives

- A public post carried by a joined relay is fetched again from its origin and checked before it is ingested. It then shows in the federated timeline and hashtag timelines. It does not show in home timelines and creates no notification.
- At most `relay_ingest_hourly_limit` posts per hour (default 1,000) are ingested per relay. Anything above the limit is dropped without being fetched.
  The admin area and `my admin relay list` show how many were ingested in the last 24 hours and how many were dropped over the limit. If many are dropped, raise the limit or reconsider the relay.
- A post is not ingested when its domain or account has a `suspend` (a federation restriction) or a `limit` (a restriction that removes it from public places).
- A post that arrived only through a relay is removed from public view after `relay_status_retention` (default 7 days). A post that a local person replied to, boosted, favorited or bookmarked,
  and the posts of authors that a local person follows, get the same retention as other remote posts (`remote_status_retention`).
- Ingesting costs one fetch from the origin per post (plus a fetch of the actor for an unknown author). At the limit of 1,000 an hour, that is at most 24,000 fetches a day.

The settings are in the [configuration reference](../reference/configuration.md#relay_ingest_hourly_limit). The result of ingesting is also exported as the metric `my.relay.activities` (see [Running the server](running.md)).

## Leave, join again and delete

- "Leave" sends an `Undo` of the `Follow` and sets the state to "Left" (`left`). Nothing is sent to or received from the relay afterwards. Posts already ingested are removed when their retention ends.
- "Join again" sends a new `Follow` to a relay that rejected the server or that the server left.
- "Delete" removes the registration of a relay that was left or rejected. Edits and deletions of posts already sent to the relay are still delivered to it after you delete it.

Registering, joining, leaving and deleting, and the relay's acceptance or refusal, are recorded in the audit log with the target type "Relay" (acceptance and refusal as remote actions).
