# Put a CDN in front of media delivery

A guide for operators. The setting is described in the [configuration reference](../reference/configuration.md#media_origin).

A CDN in front of the server (Cloudflare, Google Cloud CDN, CloudFront and so on) can deliver media (`/media/` and `/media/proxy/`).
Requests answered from the CDN cache use none of the server's CPU or transfer.
This guide covers the setup where the server is the CDN's origin (origin pull). It applies to both blob stores: with `blob = s3` the server
reads the files from the private bucket and serves them under `/media/` as with `blob = fs`, so the bucket is never the CDN's origin.

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

## Who can open media URLs

Media URLs are not access-controlled. Anyone who has the URL of a file can open it, without signing in, whatever the visibility of the post that attaches it.
This holds with or without a CDN, and includes the attachments of followers-only posts and direct messages and the proxy URLs of remote media.

- The URLs are hard to guess: a local file's URL contains identifiers with a random part, and a proxy URL carries a signature. Knowing one URL does not reveal others.
- Anyone who saw a post can pass its media URLs on, and the URLs keep working for others.
- Responses are marked `public`, so the CDN and any shared cache between the server and the reader may keep a copy (see the table above for how long).
  This is what lets a CDN serve media without asking the server.

Tell your users that a non-public post limits who sees the post, but does not stop someone who has the link of an attached file from opening the file.

## Deletion and the cache

After you delete media, the CDN's cache keeps it for `max-age`. A deleted file can stay visible for up to a year for local media, and up to `remote_media_cache_ttl` for the proxy.
The server does not purge the CDN's cache. If something must disappear right after a deletion or a moderation action, purge it by hand in the CDN's console or API.

## Service Worker

The web client's Service Worker does not cache media from another origin and leaves it to the browser's network handling, so only the CDN's cache applies.
