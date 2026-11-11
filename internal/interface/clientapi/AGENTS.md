# Client API over HTTP

- Changing the body or the type of an error response: [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457), search for: `application/problem+json`, Members of a Problem Details Object, `type`, Extension Members
- Changing the bearer token challenge: [RFC 6750](https://www.rfc-editor.org/rfc/rfc6750), search for: `WWW-Authenticate`, `invalid_token`, `insufficient_scope`
- Pointing a client at the authorization server from a 401: [RFC 9728](https://www.rfc-editor.org/rfc/rfc9728), search for: `resource_metadata`, `WWW-Authenticate`
- Changing the rate limit headers (an Internet-Draft, whose field names changed between versions): [draft-ietf-httpapi-ratelimit-headers](https://datatracker.ietf.org/doc/draft-ietf-httpapi-ratelimit-headers/), search for: `RateLimit-Policy`, `RateLimit`, quota, `Retry-After`
- Changing the `Idempotency-Key` handling (an Internet-Draft): [draft-ietf-httpapi-idempotency-key-header](https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/), search for: `Idempotency-Key`, fingerprint, 409 Conflict
- Announcing the deprecation of an operation: [RFC 9745](https://www.rfc-editor.org/rfc/rfc9745), search for: `Deprecation`, `rel="deprecation"`, Sunset
- Announcing the removal of an operation: [RFC 8594](https://www.rfc-editor.org/rfc/rfc8594), search for: `Sunset`, `rel="sunset"`
- Changing the event stream of the streaming endpoints: [HTML Standard, Server-sent events](https://html.spec.whatwg.org/multipage/server-sent-events.html), search for: `text/event-stream`, `Last-Event-ID`, `retry`, Interpreting an event stream
- Changing the calendar file of a gathering: [RFC 5545](https://www.rfc-editor.org/rfc/rfc5545), search for: `VCALENDAR`, `VEVENT`, `DTSTAMP`, Content Lines, TEXT
