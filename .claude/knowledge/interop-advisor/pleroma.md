# Pleroma and Akkoma

Base: <https://akkoma.dev/AkkomaGang/akkoma/src/tag/v3.20.0/> (read 2026-11-10 through the akkoma.dev API; v3.20.1
checked). Fixtures: `akkoma/3.20`, `circa-2021`. Live tests use Akkoma only for the LitePub relay.

## Relay

- The relay actor (`/relay`) has `inbox: null` and only `endpoints.sharedInbox` -> my stores the shared inbox as its inbox and resolves its signing key that way [live: TestRelayLitePub]
- A LitePub relay follows the instance actor back and relays with `Announce` from the relay actor -> my accepts a Follow of the instance actor only from a joined relay's actor, and refetches the announced object [live: TestRelayLitePub; internal/platform TestRelays]
- Some Pleroma-family relays relay only `Announce`, which my does not send, so my posts may not be relayed (unconfirmed) [ADR-0006 Consequences]
- Whether Pleroma/Akkoma refetch relayed activities or verify LD signatures (unconfirmed) [ADR-0006 Open Questions]

## Posts and polls

- Counts a poll vote sent as a `Create` of a `Note` named after the option [mastodon-content.md]
- RFC 9421 support on receive and send (unconfirmed) [ADR-0006 Open Questions]
- Accepting a `Follow` from the minimal `inbound` actor (unconfirmed) [ADR-0006 Open Questions]

## Articles my sends

- Validated like Note and Page (lib/pleroma/web/activity_pub/object_validators/article_note_page_validator.ex); shown as `<p><a href="{url}">{name}</a></p>` then the full `content` (mastodon_api/views/status_view.ex); summary becomes the CW
- The default scrubber (priv/scrubbers/default.ex) drops headings and tables (h6 even when headings are allowed), keeps `img` with http(s) -> body images show only here
- Drops (does not cut) a `Create`/`Update` whose `content` exceeds 100,000 chars (`remote_limit`, config/config.exs); inbox POST up to 16 MB (`upload_limit`) -> my `max_article_bytes` default 100,000 fits
- Stores `source` but re-renders only `text/x.misskeymarkdown`
- Applies an `Update` only when `updated` is newer, and not to `name`: title edits do not arrive (object/updater.ex)
- Embeds the title unescaped in HTML: a title with `<` or `&` may show broken
