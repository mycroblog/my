# Building apps for this server

A guide for developers of third-party apps: clients, bots, integrations, and command-line tools that use the client API
(`/api/v1`) of an instance on behalf of its users. The decisions behind it are in
[ADR-0090](../adr/0090-third-party-oauth-apps.md); the operations of the API are in its OpenAPI document
(see [Stability of the API](#stability-of-the-api)).

## Mastodon client libraries do not work as they are

The client API is this project's own ([ADR-0007](../adr/0007-client-api.md)). It is **not** the Mastodon client API:

- There is no `POST /api/v1/apps`, and no other Mastodon endpoint. A client library for Mastodon, or for a server that copies its API,
  cannot register itself, authorize, or call this server.
- An app gets a `client_id` in one of three ways ([below](#registering-an-app)). Dynamic client registration is
  [RFC 7591](https://www.rfc-editor.org/rfc/rfc7591) at `POST /oauth/register`.
- Federation with Mastodon, Misskey and Pleroma servers is unaffected: this concerns only the API that apps call.

Any OAuth 2.0 client library that supports the authorization code grant with PKCE can authorize against the server.

## Discovery

An instance publishes two documents, anonymous and cacheable for 5 minutes, so that a generic client can configure itself from the instance's origin:

| URL | Document |
| --- | --- |
| `/.well-known/oauth-authorization-server` | Authorization server metadata ([RFC 8414](https://www.rfc-editor.org/rfc/rfc8414)): the endpoints, `scopes_supported` (the scopes a third party may hold), `code_challenge_methods_supported: ["S256"]`, `grant_types_supported`, and the supported client authentication methods. `registration_endpoint` is present only while dynamic registration is on, and `client_id_metadata_document_supported` only while metadata document clients are on. |
| `/.well-known/oauth-protected-resource` | Protected resource metadata ([RFC 9728](https://www.rfc-editor.org/rfc/rfc9728)) of `/api/v1`. The `WWW-Authenticate` header of a `401` from the API carries its URL as `resource_metadata`. |

OpenID Connect is not offered: there are no ID tokens and no `/.well-known/openid-configuration`.

## Registering an app

There are three ways to get a `client_id`. An administrator can turn each of them off, so an instance may offer fewer
(the metadata shows which are on). Prefer them in this order.

### 1. Client ID Metadata Document (preferred)

Use this for apps you distribute: a mobile app, a CLI, a web client. There is nothing to register. The `client_id` is an `https` URL
that serves a JSON document describing the app, and the server fetches it when the app starts an authorization.
It works on every instance that has the feature on, with no per-instance setup.
This is an IETF working-group draft
([draft-ietf-oauth-client-id-metadata-document](https://datatracker.ietf.org/doc/draft-ietf-oauth-client-id-metadata-document/)), not an RFC.

```json
{
  "client_id": "https://app.example/oauth/client.json",
  "client_name": "Example",
  "client_uri": "https://app.example/",
  "logo_uri": "https://app.example/logo.png",
  "redirect_uris": ["https://app.example/oauth/callback", "com.example.app:/oauth/callback", "http://127.0.0.1/callback"],
  "scope": "read write:statuses"
}
```

- `client_id` must equal the URL of the document exactly. The URL is `https`, has a path, and has no fragment, userinfo, or `.` / `..` segment; at most 2048 bytes.
- `redirect_uris` is required: 1 to 20 URIs, each of a form allowed by [Redirect URIs](#redirect-uris-and-native-apps).
- `client_name` (at most 200 characters; the host of the `client_id` when absent), `client_uri` and `logo_uri` (used only when `https`) are shown on the consent screen.
- `scope` is the most the app may ask for. Unknown scopes and [first-party-only scopes](#scopes) are dropped. When absent, it is every scope a third party may hold.
- `grant_types`, `response_types` and `token_endpoint_auth_method`, when present, must allow the authorization code grant and `none`.
  The document must not contain `client_secret`: these clients are public and use PKCE.
- Serve the document with a successful response and without redirects (the server does not follow them), at most 5 KB, with ordinary HTTP caching headers.
  The server reuses it for what the response allows (an hour when it says nothing, and at most 24 hours), does not cache failures, and fetches through an SSRF-guarded client.
  A fetch that fails makes the authorization request fail as an unknown client.
- An administrator can deny a host; a denied host's documents are no longer fetched and the tokens of its clients stop working.

### 2. Registration by a user

Use this for a bot, an integration, or a script that you run yourself. A signed-in user registers the app in **Settings → Development**
(or with `POST /api/v1/oauth_apps`, scope `write:accounts`): a name, the site URL, redirect URIs, the most scope the app may ask for, and the type.

- A **public** app has no secret. A **confidential** app gets a `client_secret`, shown **once**, when the app is registered or its secret reissued (`POST /api/v1/oauth_apps/{id}/secret`);
  reissuing invalidates the old secret at once. Only a hash is stored.
- A confidential client authenticates at the token endpoint with `client_secret_basic` or `client_secret_post`. **PKCE is required for it too.**
- The owner can edit and delete the app. A user keeps at most 25 apps, and registering is rate limited.
- **Tokens for yourself:** a registered app can issue an access token and a refresh token for its owner's own account from the settings page (`POST /api/v1/oauth_apps/{id}/tokens`),
  with no redirect and no consent screen. Choose the scopes there. They are ordinary tokens with the lifetimes below. A bot stores the rotated refresh token each time it refreshes.
- An instance can turn off registration by users (the setting `user_app_registration`); apps already registered keep working.

### 3. Dynamic client registration (RFC 7591)

Use this for a generic tool that cannot publish a metadata document. `POST /oauth/register` takes no authentication and a JSON body:

```sh
curl -X POST https://my.example/oauth/register \
  -H 'Content-Type: application/json' \
  -d '{"client_name":"Example","redirect_uris":["http://127.0.0.1/callback"],"scope":"read"}'
```

- Only **public clients**. `token_endpoint_auth_method` defaults to `none`; any other value is rejected with `invalid_client_metadata`.
- Accepted metadata: `client_name`, `client_uri`, `logo_uri`, `redirect_uris`, `scope`, `grant_types`, `response_types`. A `software_statement` is not accepted.
  `scope` is narrowed to what a third party may hold; the response states the scope that was granted.
- The answer is `201` with `client_id`, `client_id_issued_at` and the registered metadata. There is no `client_secret` and no registration access token:
  RFC 7592 (reading, updating, deleting a registration) is not supported. To change an app, register again.
- CORS allows any origin, because the endpoint takes no credentials. It answers `403` (`access_denied`) when registration is turned off.
- A client that never exchanges an authorization code within a day (by default), or has had no usable token and no use for 90 days (by default), is deleted by a periodic job.
  Register again if your `client_id` stops working (`dynamic_client_unredeemed_ttl` and `dynamic_client_idle_ttl` in the [configuration reference](configuration.md)).

## Authorization

The server supports the authorization code grant ([RFC 6749](https://www.rfc-editor.org/rfc/rfc6749) §4.1) and the refresh token grant.
There is no `client_credentials` (every operation is done as a user; authorize a bot as the user of its bot account), no password, implicit, or device grant.

1. Generate a `code_verifier` and its `S256` `code_challenge` ([RFC 7636](https://www.rfc-editor.org/rfc/rfc7636)). **PKCE with `S256` is required for every client.** `plain` is refused.
2. Send the user's browser to `GET /oauth/authorize` with `response_type=code`, `client_id`, `redirect_uri`, `code_challenge`, `code_challenge_method=S256`, `scope` (space-separated) and `state`.
   `redirect_uri` is required and must match one of the app's registered URIs exactly (a loopback URI ignores the port).
   Scopes beyond the app's, or a first-party-only scope, are refused with `invalid_scope` rather than dropped.
3. The user signs in if needed and sees the **consent screen**. It is shown on every authorization; consent is not remembered.
   It names the app, where it comes from, the site, the scopes in words, and the host of the redirect URI.
   Every third-party app is labelled as one the server has not verified: the name and logo are the app's own claim; only the host of a `client_id` URL or the registering user is verified.
   An app that redirects to a private-use scheme or loopback address gets an extra warning, because another app could claim the same address.
4. The browser is redirected (`303`) to `redirect_uri` with `code`, `state` and `iss` (the server's origin, [RFC 9207](https://www.rfc-editor.org/rfc/rfc9207): check it).
   The code is single use and valid for 60 seconds.
5. `POST /oauth/token` (form-encoded) with `grant_type=authorization_code`, `code`, `redirect_uri`, `client_id`, `code_verifier`
   (and the client secret for a confidential client) returns `access_token`, `refresh_token`, `token_type: Bearer`, `expires_in` and `scope`.
6. Call the API with `Authorization: Bearer <access_token>`. Cookies and query strings never authenticate.

### Tokens

| | Lifetime | Notes |
| --- | --- | --- |
| Authorization code | 60 seconds | Single use. |
| Access token (`my_at_...`) | 1 hour | |
| Refresh token (`my_rt_...`) | 30 days | **Rotated on every use**: each `refresh_token` grant returns a new access token and a new refresh token, and the old ones stop working. |

- Always store the newest refresh token. Presenting a refresh token that was already used (or revoked) **revokes the whole token family**, so serialize refreshes
  and do not send two at once; the user has to authorize again.
- `POST /oauth/revoke` ([RFC 7009](https://www.rfc-editor.org/rfc/rfc7009)) revokes a token and its whole family.
- A user can list and revoke authorized apps in the settings of the first-party web client. An administrator can disable an app (and deny a metadata document host):
  its tokens stop working at once and the app cannot be authorized.
- A `401` carries `WWW-Authenticate: Bearer` with `error="invalid_token"` when the token is not valid; refresh it, or authorize again when that fails.

### Redirect URIs and native apps

Redirect URIs follow [RFC 8252](https://www.rfc-editor.org/rfc/rfc8252) and are matched exactly against the registered ones. Allowed:

| Form | Example | Notes |
| --- | --- | --- |
| Claimed `https` (Universal Links / App Links) | `https://app.example/oauth/callback` | Recommended: the OS verifies that the app owns the domain, so no other app can take the redirect. |
| Private-use URI scheme | `com.example.app:/oauth/callback` | The scheme must be in reverse domain notation. Common web schemes (`http`, `javascript`, `data`, `file`, ...) are refused. The OS does not verify who owns a scheme. |
| Loopback | `http://127.0.0.1/callback`, `http://[::1]/callback` | The port is ignored when matching, so a CLI can listen on any free port. `localhost` is not accepted; use the IP literal. |

`urn:ietf:wg:oauth:2.0:oob`, fragments, and userinfo are refused.

For apps with a `client_id` URL or a dynamic registration, a private-use scheme or loopback address cannot prove who the app is,
so the consent screen warns the user about it. Prefer a claimed `https` redirect for an app that has one.

## Scopes

Scopes are the same as the first-party web client's, in two levels: broad scopes (`read`, `write`, `follow`, `push`) that grant the fine-grained ones under them
(`read:statuses`, `write:statuses`, ...). Each operation in the OpenAPI document lists the scopes it needs; an app may ask for either level.
The list the instance offers is `scopes_supported` in the [authorization server metadata](#discovery).
A request without enough scope gets `403` with `WWW-Authenticate: Bearer error="insufficient_scope"`.

- **Direct messages:** reading posts of the `direct` visibility needs `read:direct` (the broad `read` includes it). A token with only `read:statuses` never receives a `direct` post in a timeline, thread, or notification.
  Sending a direct post needs only `write:statuses`.
- **First-party-only scopes:** `admin`, `admin:read`, `admin:write` and `write:supporters` are never granted to a third-party app,
  not even through `write`. Asking for one at the authorization endpoint is an `invalid_scope` error. Administration is done from the first-party web client.
- **First-party-only operations:** account security (password, two-factor authentication, passkeys, deleting the account, and the list of authorized apps) is
  marked `x-first-party-only` in the OpenAPI document and answers a third-party token with `403` `/api/v1/problems/first-party-only`, whatever its scopes.

## Rate limits

The API is rate limited per account, across all of a user's apps, and anonymous requests per IP address.
The endpoints for registering and authorizing apps are limited as well: user app registration, dynamic registration, `/oauth/token`, and the fetch of metadata documents.
An administrator may also cap a single client across all its users (the setting `oauth_client_rate_limits`, see the [configuration reference](configuration.md)).
The numbers belong to the server and the operator, so read them from the responses instead of hard-coding them.

- A request over a limit is answered `429 Too Many Requests` with a `Retry-After` header (in seconds). Wait that long before retrying; back off if it happens again.
  The OAuth endpoints answer an OAuth error body (`temporarily_unavailable`); `/api/v1` answers a problem document.
- Responses of the API also carry `RateLimit-Policy` and `RateLimit` fields
  ([draft-ietf-httpapi-ratelimit-headers](https://datatracker.ietf.org/doc/draft-ietf-httpapi-ratelimit-headers/)) with the policy's name, quota, and what remains.
- Do not register an app or fetch a token per request: keep your `client_id`, cache your tokens, and refresh them only when they expire.

## Stability of the API

From the v0 release, `/api/v1` keeps these promises ([ADR-0090](../adr/0090-third-party-oauth-apps.md) §8):

- **Additions are not breaking**: new fields, operations, scopes, and enum values can appear at any time. Ignore what you do not know.
- **Breaking changes are announced first.** Removing or renaming something, changing a type, making a field required, or changing the meaning of a scope is allowed in v0,
  but the operation is first marked `deprecated: true` in the OpenAPI document and its responses carry a `Deprecation` header
  ([RFC 9745](https://www.rfc-editor.org/rfc/rfc9745), a date such as `@1688169599`) and a `Sunset` header ([RFC 8594](https://www.rfc-editor.org/rfc/rfc8594)).
  It stays for at least one v0.x minor release. The release notes have an "API changes" section. Log these headers in your client.
- **Security fixes** can change behaviour without notice.
- Version 1.0 makes no breaking change to `/api/v1`; one would go to `/api/v2`.

The OpenAPI document of the running server is at `GET /api/v1/openapi.yaml`, with the scopes of each operation; generate your client from it.
