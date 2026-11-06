# The event stream

The server pushes new posts, notifications, and announcements as [Server-Sent Events](https://html.spec.whatwg.org/multipage/server-sent-events.html) (`text/event-stream`), so a client need not poll.

| Endpoint | Scopes | Events |
| --- | --- | --- |
| `GET /api/v1/streaming` | `read:statuses` and `read:notifications` | The user's home and list timelines, notifications, and announcements. |
| `GET /api/v1/streaming/timelines/{timeline}` | `read:statuses` | New posts (`update` only) of the local or federated timeline. |

Open a stream with `fetch` and the `Authorization: Bearer` header, like any other call. The browser's `EventSource` cannot send the header, so it cannot be used.

## Events of `/api/v1/streaming`

| Event | `data` |
| --- | --- |
| `update` | A `Status`: a new post or boost in the home timeline, as it would appear there. |
| `list_update` | `{"list_id": "...", "status": {...}}`: the same post, once for each of the user's lists that shows it, right after its `update`. |
| `notification` | A `Notification`. |
| `announcement` | An `Announcement` that was published or changed. |
| `announcement_removed` | `{"id": "..."}`: an announcement no longer shows. |

The server sends a comment line (`: heartbeat`) every 30 seconds. Ignore events you do not know: new ones can appear ([stability](stability.md)).

## Staying connected

- **The server replays nothing.** After a stream ends, reconnect and read what you missed with ordinary requests: the first page of the home timeline, of the open list timelines (or of the local or federated timeline for those streams), and the notifications. Announcements whose window starts later arrive with no event; list them from time to time.
- The server ends a stream when the access token stops being valid, when the client does not keep up, when the server shuts down, and when it may have missed updates. A timeline stream also ends, and then answers `404`, while the instance turns that timeline off. Reconnect with a backoff, and with a refreshed token if the old one expired.
- `429` with the type `/api/v1/problems/too-many-streams`: the user already holds as many streams open as the server allows. Close one.
- `503` with `Retry-After`: the server cannot receive updates now. Retry after that many seconds.
- Other `4xx` answers (a `401` that a refresh did not cure, a `403` for a missing scope) will not change by retrying: fix the cause.
- A proxy that buffers responses breaks a stream. Where you cannot avoid one, read the notifications and the timelines with ordinary requests instead.
- On an instance that runs several server processes without a shared stream broker, a stream may miss updates. The operator can tell you which your instance uses.

The [client libraries](libraries.md#streaming) implement the reconnecting for you.
