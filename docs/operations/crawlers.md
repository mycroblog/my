# Search engines and AI crawlers

An explanation and verification guide for operators.

robots.txt and meta tags are statements that crawlers may or may not honor; they are not a technical block. If you need to block, use a WAF or rate limiting.

## Settings

| Scope | Setting | Where to change it | When not set |
| --- | --- | --- | --- |
| Server | `search_indexing` (`allow` / `deny`) | The "Search engines and AI" section of the admin server settings, `my instance set`, `PATCH /api/v1/admin/settings` ([Instance settings](instance-settings.md)) | `deny` |
| Server | `ai_usage` (`allow` / `deny_training` / `deny`) | Same | `deny` |
| Server | `indexable_default` (the `indexable` of new accounts) | Same | `false` |
| Server | `security_contact` (where to report vulnerabilities) | Same | Empty (no `security.txt` is served) |
| Person | `indexable` (appear in search) | "Search engines and AI" in the profile settings, `PATCH /api/v1/accounts/update_credentials` | `indexable_default` at registration |
| Person | `ai_opt_out` (refuse AI training and use) | Same | `false` |

- A page's statement is the stricter of the server's setting and the page owner's setting. A person cannot allow more than the server does.
- Search engines are told they may index only the profiles and `public` posts of accounts with `indexable = true`, on a server with `search_indexing = allow`.
  A server that wants to be searchable changes both `search_indexing` and `indexable_default`. People who registered before you changed `indexable_default` are not included until they choose `indexable` themselves.
- A changed setting takes effect in `/robots.txt`, pages and media responses without a restart.

## What the server returns

| Response | When refusing search | When refusing AI |
| --- | --- | --- |
| Pages of profiles and `public` posts | `noindex, noarchive` in `<meta name="robots">` and `X-Robots-Tag` | `noai, noimageai`, `Content-Usage: train-ai=n` (`train-ai=n, ai-use=n` for `deny`), and `tdm-reservation: 1` (header and meta) |
| `unlisted` posts, 404 pages for hidden or nonexistent pages, pages of remote accounts, app screens (`/home`, `/settings` and so on) | `noindex, noarchive` whatever the setting | Follows the server setting |
| `/`, `/about` | Follows the server setting | Follows the server setting |
| `/media/` (when this server serves it) | Follows the server setting. No per-owner statement | Follows the server setting. No per-owner statement |
| The client API, ActivityPub documents, `/.well-known/`, `/nodeinfo/`, sign-in and OAuth screens | `X-Robots-Tag: noindex` | Not added (robots.txt is enough) |

`noindex` does not stop link previews (see [Link previews](link-previews.md)).

### `/robots.txt`

`GET /robots.txt` is built from the server settings and the bundled list of AI crawlers (`text/plain; charset=utf-8`, `Cache-Control: public, max-age=3600`, ETag).
Operators cannot add rules of their own.

- If `ai_usage` is `deny_training` or `deny`, training crawlers (`GPTBot`, `ClaudeBot`, `CCBot`, `Google-Extended`, `Applebot-Extended` and so on) get `Disallow: /`.
- If `ai_usage` is `deny`, the crawlers of AI search, assistants and agents (`OAI-SearchBot`, `ChatGPT-User`, `Claude-User`, `PerplexityBot` and so on) get `Disallow: /` too.
- All crawlers (`User-agent: *`) are refused only `/api/`, `/oauth/`, `/auth/`, `/ap/` and `/media/proxy/`, and the settings are stated in `Content-Usage` (the IETF AI Preferences draft) and
  `Content-Signal` (Content Signals) lines. Refusing search does not produce `Disallow: /`: blocking fetches would stop crawlers from reading `noindex`,
  and would also stop the link-preview fetchers that obey robots.txt.
- `/.well-known/` and `/nodeinfo` are not refused, so that server lists and statistics keep working.
- When `account_domain` is separate from the host, the host side serves it. The robots.txt of the handle's domain is the operator's own business.

### `/.well-known/`

| Path | Served when | Content |
| --- | --- | --- |
| `/.well-known/tdmrep.json` | `ai_usage` is anything but `allow` | `[{"location": "/*", "tdm-reservation": 1}]` (TDM Reservation Protocol). 404 for `allow` |
| `/.well-known/security.txt` | `security_contact` is set | `Contact`, `Expires` (30 days after the time of the response), `Preferred-Languages` (`default_locale` and `en`), `Canonical` (RFC 9116) |

### Information page

The instance information page (`/about`) shows whether search and AI are allowed, and what the statement means, from `search_indexing` and `ai_usage` in `GET /api/v1/instance`.
The explanation places the statements against Article 30-4 of the Japanese Copyright Act (such statements may not be treated as a legal refusal) and Article 4 of the EU DSM Directive (a machine-readable reservation of text and data mining).
It does not guarantee any legal effect.

## Limits

- A person's `ai_opt_out` is stated only on pages this server serves. Media has no owner in its key, and some setups let a CDN serve it directly, so media carries only the server-wide statement.
  Statements on pages served by other servers are left to those servers (no custom ActivityPub extension is sent).
- `ai_usage = deny` also states a refusal of the actions in which a user of ChatGPT or a similar service has it read a page of this server.
- The `indexable` of a remote account (Mastodon's `toot:indexable`) is received and stored. Pages of remote accounts are always `noindex` on this server.

## Serving media from object storage or a CDN

When object storage or a CDN serves media directly, `/media/` responses do not pass through this server and do not carry the headers in the table above.
The server-wide statement still applies to everything through `/robots.txt`; to add the headers too, add the following to `/media/*` in the CDN's response-header rules (an example for `ai_usage = deny`).

```text
X-Robots-Tag: noindex, noarchive, noai, noimageai
Content-Usage: train-ai=n, ai-use=n
tdm-reservation: 1
```

When you change the settings, change the CDN rules to match. `/media/proxy/` is served by this server, so it needs no rule.

## Updating the AI crawler list

The list is generated from `robots.json` of [ai-robots-txt/ai.robots.txt](https://github.com/ai-robots-txt/ai.robots.txt) (MIT) and bundled in `internal/interface/web/ai_crawlers.gen.go`. It is not fetched at run time.
About once a month, open an update pull request as follows.

1. In `tasks/go.yml`, change `AI_ROBOTS_COMMIT` to the new commit and `AI_ROBOTS_SHA256` to the SHA-256 of that commit's `robots.json`.
2. Run `scripts/task.sh go:ai-crawlers`.
3. Review the diff. The classification (training or AI use) is derived from the `function` text in `robots.json`, so if a newly added crawler is classified wrongly,
   fix it with `overrides` in `scripts/ai-crawlers/main.go`. Link-preview fetchers and general-purpose search engines are left out of the list with `excluded`.

## Verify

```sh
curl -sS https://<host>/robots.txt
curl -sS -o /dev/null -D - -H 'Accept: text/html' https://<host>/@alice | grep -iE 'x-robots-tag|content-usage|tdm-reservation'
curl -sS https://<host>/.well-known/tdmrep.json
curl -sS https://<host>/.well-known/security.txt
```

- On a server with no settings, `/robots.txt` has `User-agent: GPTBot` and `Content-Usage: search=n, train-ai=n, ai-use=n`,
  and profiles carry `X-Robots-Tag: noindex, noarchive, noai, noimageai`.
- With `search_indexing = allow` and `ai_usage = allow`, the profile of an account that chose `indexable` carries no `X-Robots-Tag`, and `/.well-known/tdmrep.json` is 404.
