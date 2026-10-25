# Put a CDN in front of media delivery

A guide for operators. The setting is described in the [configuration reference](../reference/configuration.md#media_origin).

A CDN in front of the server (Cloudflare, Google Cloud CDN, CloudFront and so on) can deliver media (`/media/` and `/media/proxy/`).
Requests answered from the CDN cache use none of the server's CPU or transfer.
This guide covers the setup where the server is the CDN's origin (origin pull). It does not cover the public URL of the `blob/s3` adapter.

## Configure it

Set `media_origin` (`MY_MEDIA_ORIGIN`) to the CDN's host name (for example `media.example.org`).

```sh
MY_MEDIA_ORIGIN=https://media.example.org
```

- The value is an origin only (`scheme://host[:port]`), with no path, query or trailing slash. `https` is required, and `http` is allowed only for loopback hosts such as `localhost`. An invalid value makes startup fail.
- If it is not set, media is served from the server's own origin (`https://{host}`).
- These URLs use this origin: `{media_origin}/media/...` for local media and `{media_origin}/media/proxy/...` for the proxy of remote media.
  Every URL that the client API, ActivityPub (`attachment`, `icon`, `image`, and the `icon` of custom emoji) and link previews (OGP) emit is covered.
- The web client's Content-Security-Policy adds this origin to `img-src` and `media-src`.
- Federated URIs, such as the `id` of an ActivityPub object, stay under `https://{host}/` whatever `media_origin` is. Only the place where files are served changes, not identifiers.
- URLs are built each time they are read and are not stored in the database, so changing `media_origin` changes the URLs of existing media too.
  Old URLs that remote servers stored keep working as long as `/media/` on the old origin keeps serving.

## Point the CDN at the server

- Make the origin (backend) the server's public host name (`https://{host}`). The CDN needs to fetch only paths under `/media/` from the server.
- The server does not restrict `/media/` and `/media/proxy/` by host name. It serves whatever `Host` the CDN sends.
- `/media/` needs no authentication, and Authorized Fetch does not apply to it. The server answers unsigned requests from the CDN as they are.
- For remote media (`/media/proxy/{id}/{variant}?sig=...`), `sig` is part of the URL. **Include the query string in the cache key.** Otherwise URLs with different signatures share one entry.
- Let the origin's `Cache-Control` decide how long things are cached (do not override it at the CDN).

| Path | `Cache-Control` |
| --- | --- |
| `/media/{...}` (local media) | `public, max-age=31536000, immutable` (one year) |
| `/media/proxy/{...}` (remote media) | `public, max-age=` the number of seconds of the instance setting `remote_media_cache_ttl` |

Both are returned with `Cross-Origin-Resource-Policy: cross-origin`, so pages from other origins can read them.

## Deletion and the cache

After you delete media, the CDN's cache keeps it for `max-age`. A deleted file can stay visible for up to a year for local media, and up to `remote_media_cache_ttl` for the proxy.
The server does not purge the CDN's cache. If something must disappear right after a deletion or a moderation action, purge it by hand in the CDN's console or API.

## Service Worker

The web client's Service Worker does not cache media from another origin and leaves it to the browser's network handling, so only the CDN's cache applies.
