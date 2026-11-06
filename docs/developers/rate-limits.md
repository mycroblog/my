# Rate limits

How a client should behave when the server limits it. The numbers belong to the server and the operator, so this guide says what to read, not what to expect.

The API is rate limited per account, across all of a user's apps, and anonymous requests per IP address.
The endpoints for registering and authorizing apps are limited as well: user app registration, dynamic registration, `/oauth/token`, and the fetch of metadata documents.
An administrator may also cap a single client across all its users (the instance setting `oauth_client_rate_limits`, see [instance settings](../operations/instance-settings.md)).
The numbers belong to the server and the operator, so read them from the responses instead of hard-coding them.

- A request answered `503 Service Unavailable` with a `Retry-After` header found the server busy. Wait that long before retrying.
- A request over a limit is answered `429 Too Many Requests` with a `Retry-After` header (in seconds). Wait that long before retrying; back off if it happens again.
  The OAuth endpoints answer an OAuth error body (`temporarily_unavailable`); `/api/v1` answers a problem document.
- Responses of the API also carry `RateLimit-Policy` and `RateLimit` fields
  ([draft-ietf-httpapi-ratelimit-headers](https://datatracker.ietf.org/doc/draft-ietf-httpapi-ratelimit-headers/)) with the policy's name, quota, and what remains.
- Do not register an app or fetch a token per request: keep your `client_id`, cache your tokens, and refresh them only when they expire.
