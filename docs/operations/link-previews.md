# Link previews (OGP / X Card / fediverse:creator)

The previews that other services show of this server's pages, what the server returns for them, and the steps to check them by hand. The cards this server shows for the links in its own posts
are in [Cards of links in posts](#cards-of-links-in-posts) below; how to stop fetching them from a domain is in [instance settings](instance-settings.md#link-card-domains).

## What the server returns

`my serve` returns the HTML of the landing page (`https://<host>/`), the instance information page (`https://<host>/about`),
the pages of the published documents (`/about/terms`, `/about/privacy`, `/about/commerce`), the page about the link card crawler (`/about/crawler`), profiles
(`https://<host>/@alice`), and posts (`https://<host>/@alice/status/<post ID>`) by embedding meta tags into the shell of the
web client.
A request for HTML is always treated as signed out, so a preview shows only what a signed-out visitor can see.

| Page | Response |
| --- | --- |
| The landing page (`/`) and the instance information page (`/about`) | 200. The shell with meta tags that introduce the instance (see "The landing page" below) |
| The page about the link card crawler (`/about/crawler`) | 200. The shell with meta tags that introduce the instance, titled "Link card crawler \| instance name" |
| The pages of published documents (`/about/terms`, `/about/privacy`, `/about/commerce`) | 200. The shell with meta tags that introduce the instance, titled "document name \| instance name". The page of a document that is not published gets the generic shell |
| A `public` post, the profile of an active local account | 200. The shell with meta tags |
| An `unlisted` post | 200. The shell with meta tags, plus `<meta name="robots" content="noindex, noarchive">` and `X-Robots-Tag: noindex, noarchive` |
| A `local`, `private`, `direct`, deleted, or nonexistent post, and the profile and posts of a suspended, deactivated, deleted, or limited account | 404. The generic shell and `X-Robots-Tag: noindex, noarchive`. The body and headers are the same whatever the reason |
| `/@<old username>/status/<post ID>` | 301, to the URL with the current username |
| Every other page of the web client (hashtags, explore, the public costs and supporters pages, sign-in, and so on) | 200. The generic shell, with the meta tags of the instance (see "The generic shell" below) |

Every page also carries the statements to search engines and AI crawlers (meta tags such as `noindex` and `noai`, and
`X-Robots-Tag`, `Content-Usage`, and `tdm-reservation`), according to the instance settings and the owner's settings
([search engines and AI crawlers](crawlers.md)). The `noindex` in the table above is always added, whatever the settings are.
`noindex` does not stop link previews.

A request with an ActivityPub `Accept` does not get the shell; the ActivityPub side answers it (a profile answers with the
actor's `id`, and a post a signed-out visitor can see answers with a 302 to its `id`). Every response carries `Vary: Accept`.

### Meta tags

| Tag | Content |
| --- | --- |
| `og:title`, `<title>` | "Display name (@username@account domain)". The username if the display name is empty |
| `og:type` | `profile` for a profile, `article` for a post |
| `og:url`, `<link rel="canonical">` | The canonical URL (the URL with the current username) |
| `og:description` | The plain text of the post body (the bio for a profile) without tags, cut to 200 graphemes. For a post with a content warning, only the content warning text |
| `og:image`, `og:image:alt` / `width` / `height` | The image chosen in the order below |
| `og:site_name` | The instance name (the text `name`, the same in every language; `host` when there is none; [name and domain](instance-settings.md#name-and-domain)) |
| `og:locale`, `<html lang dir>` | The language of the post. When it is unset (`und`), the author's default posting language, and failing that the instance's `default_locale`. It does not change with `Accept-Language`. `og:locale` has the form `language_TERRITORY`: a language without a region gets its usual one (`ja` → `ja_JP`, `en` → `en_US`, `pt` → `pt_BR`), and `<html lang>` keeps the language tag as it is |
| `article:published_time`, `article:author` | Posts only. The time of the post (UTC) and the author's profile URL |
| `profile:username` | Profiles only. `username@account domain` |
| `twitter:card` | `summary_large_image` when the image is wider than about 2:1 (width / height >= 1.7) and at least 300x157, otherwise `summary` |
| `fediverse:creator` | `@username@account domain`. Mastodon shows it as the "author" |
| `<link rel="alternate" type="application/activity+json">` | The AP `id` (the actor or the post's object) |

The meta tags of the landing page introduce the instance itself.

- `og:type` is `website`, `og:title`, `<title>`, and `og:site_name` are the instance name, and `og:url` and
  `<link rel="canonical">` are `https://<host>/`.
- `og:description` is the instance text `short_description` cut to 200 graphemes. It is left out when there is none.
- The language (`og:locale`, `<html lang dir>`) is the instance's `default_locale`. The name and description are the same value
  in every language.
- `og:image` is the instance image: number 3 below, or number 4, 5, or 6 if there is none.
- It does not belong to an account, so `fediverse:creator` and the ActivityPub `alternate` are not output.
- The instance information page (`/about`) has the same meta tags. `og:title` and `<title>` are "About {instance name}"
  (`server:preview.about_title`), and `og:url` and `<link rel="canonical">` are `https://<host>/about`.
- The pages of published documents (`/about/terms`, `/about/privacy`, `/about/commerce`) have the same meta tags too.
  `og:title` and `<title>` are "{document name} | {instance name}" (`server:preview.document_title`), and `og:url` and
  `<link rel="canonical">` are the address of that page. The page of a document that is not published gets the generic shell.
- The page about the link card crawler (`/about/crawler`) has the same meta tags, with the same form of title and address.

The generic shell, which every other page and the 404 of a hidden post return, carries the same tags as the landing page
without an address of its own: `og:type` `website`, the instance name as `og:site_name` and `og:title`, `short_description`
as `og:description`, `og:locale`, the instance image, and `twitter:card`. It has no `og:url` and no
`<link rel="canonical">`, so one copy serves every path, and the 404 of a hidden post stays identical to any other page.
A shared link to a hashtag or the public costs page shows the instance's card.

`og:image` is chosen in this order.

1. The first image attached to the post (its alt text goes into `og:image:alt`). It is not used for a post marked sensitive or
   with a content warning.
2. The profile's header image, or else the avatar (`og:image:alt` is "Header image of {name}" or "Avatar of {name}").
3. The instance preview image (the instance setting `preview_image`, under "Link previews" on Settings → General of the
   administration; `og:image:alt` is the text `preview_image_alt`, or "Image of {instance name}" if there is none).
4. The instance thumbnail (the instance setting `thumbnail`, the image of the Settings → General page; `og:image:alt` is the
   text `thumbnail_alt`, or "Image of {instance name}" if there is none).
5. The link preview version of the instance's own logo ([logo and icon](instance-settings.md#logo-and-icon)), a 1200x630 PNG
   at `https://<host>/brand/<revision>/preview-1200x630.png` (`og:image:alt` is "Image of {instance name}").
6. The bundled default image (`https://<host>/preview-image.png`, a 1200x630 PNG of the mycroblog wordmark on white, without
   the instance name; `og:image:alt` is "The mycroblog logo").

Numbers 3 to 6 are the instance image. So a page that shows a preview always has `og:image` and `og:image:alt`.
The images of 1 to 4 are the `large` variant of the processed media, and the width and height are the values from processing.
If `preview_image` is unset, still processing, failed processing, or is not an image, the thumbnail is used, then the logo,
then the bundled image. Instance settings are read on every request, so `my instance set preview_image ...` takes effect
without a restart (for the procedure, see [instance settings](instance-settings.md)).

The bundled image is part of the binary (about 20 KB). It is served with `Cache-Control: no-cache` and an ETag.

### Caching

- The response carries `Cache-Control: public, max-age=60, stale-while-revalidate=300`. A deletion, a change of visibility, or
  publishing or unpublishing a document stays in shared caches for at most that long.
- The ETag is built from the versions of the template, the assets, and the instance settings, the language, the version
  (revision) of the post, the effective visibility, the version of the profile, and the chosen image (for the landing page, the
  information page, the document pages, and the page about the crawler: the address of the page, its title, the instance description, and the chosen image;
  for the generic shell: the instance name, its description, and the instance image).
  `If-None-Match` gets `304`.
- If you put a CDN in front, include `Accept` in the cache key (so that AP JSON and HTML are not mixed up).

## Manual check

Check each service on a real device after an upgrade or a change to the preview settings. You need an instance that is reachable from outside over HTTPS (the
preview bots cannot reach `localhost`).

### Preparation

1. With an account for testing, prepare these posts.
   - A `public` post with a landscape image (for example 1200x630) and alt text
   - A `public` post without an image (from an account that has a header image)
   - A `public` post whose body is longer than 200 characters and contains emoji (such as 👨‍👩‍👧‍👦)
   - An `unlisted` post
   - A `private` post
   - A post from an account whose display name contains `"><script>alert(1)</script>`
2. Check the HTML of each URL.

   ```sh
   curl -sS -D - -H 'Accept: text/html' https://<host>/@alice/status/<post ID>
   ```

   - The status is 200 and the meta tags in the table above are there. The `unlisted` post has `noindex`.
   - A `private` post and a nonexistent ID give the same 404 body with `X-Robots-Tag: noindex, noarchive` (and `noai,
     noimageai` too when the settings refuse AI).

     ```sh
     diff <(curl -sS https://<host>/@alice/status/<private post ID>) <(curl -sS https://<host>/@alice/status/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f)
     ```

   - The `<script>` in the display name is escaped as `&lt;script&gt;`.

### X

1. Paste the URL of the post with an image into the compose screen (or a DM).
2. Check that the large image card (`summary_large_image`) shows the title, the domain, and the image.
3. For the URL of the post without an image, check that the card shows the header image. For an account with neither a header
   image nor an avatar, check that the large card shows the default image (`preview_image`, or the bundled image when it is
   unset).
4. For an account with only a square avatar, check that the card is the small image card (`summary`).

X caches cards. To check a change, paste the URL with a query (such as `?v=2`) so that it counts as a different URL.

### Mastodon

1. From an account on another instance, make a post that contains the URL of the post with an image.
2. Check that a preview card appears under the post with the title, the description, and the image.
3. Check that the test account (`fediverse:creator`) is shown as the "author" under the card and that you can go to its
   profile.

Mastodon caches the card on each server that fetched it. To make it again, post with a new URL (with a query).

### Discord

1. Paste the URL of the post with an image into a channel.
2. Check that the embed shows the site name (the instance name), the title, the description, and a large image.
3. For a post longer than 200 characters, check that the description ends with `…` and that no emoji is cut in the middle.

### Slack

1. Paste the URL of the post with an image into a channel.
2. Check that the unfurled preview shows the site name, the title, the description, and the image.
3. Check that a preview also appears for the URL of an `unlisted` post (`noindex` is for search engines and does not stop
   previews).

Slack caches previews too. To check a change, add a query to the URL.

### LINE

1. Send the URL of the post with an image in a chat.
2. Check that a preview with the title, the description, and the image appears.

LINE caches previews too. To check a change, add a query to the URL.

### What must not be visible

On every service, check that the URL of a `private` post and the URL of a nonexistent post both give the same generic preview
(only the instance name, or nothing) and that no body, display name, or image appears.

## Cards of links in posts

To show a card for a link in a post, this server's job fetches the linked page and its image, and it obeys what the linked site says about that.
Its requests are `my-linkcard/<version> (+https://<host>/)` for the page, the image, and `robots.txt`. Every other request of the server, the media proxy's included, stays `my/<version> (+https://<host>/)`.
The requests send no cookies and no `Referer`, and nothing says which account or post linked the page.

### The page for site operators

`https://<host>/about/crawler` tells the operators of linked sites what the crawler requests, what it sends, how often, and how to opt out, and that they can ask you to stop fetching from their domain. Anyone can read it without an account. The "Search engines and AI" section of the instance information page (`/about`) links to it; the `User-Agent` names only `https://<host>/`.

It says what this server does, from `link_cards` and `link_cards_ignore_robots_txt`, and it shows the contact of the instance (`contact_account`, `contact_email`). Set a contact, so that a site has somewhere to ask: without one, the page says that this server publishes none.
With `link_cards_ignore_robots_txt` on, the page says that a site cannot refuse this server in `robots.txt`.

### robots.txt

The name in `robots.txt` is `my-linkcard`, in any case. Without a group of that name, the `*` group applies, as RFC 9309 says. `Allow`, `Disallow`, `*`, and `$` are read, with the longest match winning and `Allow` winning a tie; `Crawl-delay` and other records are not read.
`robots.txt` is read up to 512 KiB in 10 seconds, and what was read of a larger file is used. The server keeps only the rules for `my-linkcard`, per origin (scheme, host, and port).

| Answer for `robots.txt` | What the server does | Kept for |
| --- | --- | --- |
| 200 | Obeys the file | 24 hours |
| 4xx other than 429 | No file: fetches | 24 hours |
| 429, 5xx, a network failure, or a redirect that the server refuses | Fetches nothing from the origin | 1 hour |
| More than 5 redirects | No file: fetches | 24 hours |

- The URL of the page is checked before it is requested, whatever the visibility of the post. So is each URL it redirects to, against the `robots.txt` of that URL's origin, and a redirect to a refused URL ends there: the server does not request it and makes no card.
- A URL that `robots.txt` disallows is not fetched again for 24 hours, like any other failure of a page. When the file only could not be read, the server tries the page again an hour later, up to the limit of the job's attempts.
- An image is checked the same way against its own origin. A disallowed image leaves a card without an image. When the file only could not be read, an image recorded earlier stays, and a new image is left out until the next refetch.
- A refetch, which a post can cause 14 days after the last fetch, removes the card when `robots.txt` disallows its URL. When the file only could not be read, the card stays.
- The server does not go through its cards to recheck them, so a site that changes its rules loses a card at that card's next refetch.

`link_cards_ignore_robots_txt` ([instance settings](instance-settings.md#settings-one-key-one-value)) turns all of this off: the server neither fetches nor uses `robots.txt`. It still names itself `my-linkcard`, and linked sites can then not refuse the fetches with `robots.txt`: only the directives of their pages and a request to you for a [rule on their domain](instance-settings.md#link-card-domains) remain, as [the page for site operators](#the-page-for-site-operators) tells them.

### Directives of a page

The server also reads the directives of the page it fetched: `<meta name="robots">` and `<meta name="my-linkcard">` in the `<head>`, and `X-Robots-Tag` with no bot's name or with `my-linkcard:`. Directives for another bot do not apply. When there are several, the stricter wins.
They apply whatever `link_cards_ignore_robots_txt` says.

| Directive | Card |
| --- | --- |
| `nosnippet`, `max-snippet:0` | No description |
| `noimageindex`, `max-image-preview:none` | No image, which is not fetched |
| `noindex`, `nofollow`, `noarchive`, `none`, and the others | No change |

The title, the site name, and the author stay under every directive; a site that does not want them refuses the fetch in `robots.txt`.
The directives are known only after the page is fetched, so they do not stop the request for the page. A refetch applies the directives the page has then.

### Limits per host

The crawler is gentle with each host, whatever `link_cards_ignore_robots_txt` says. A host is the name in the URL, without the port, so a linked site and the host of its images have separate limits.

- It makes one fetch at a time from a host, and the next starts at least 1 second after the last one ended. A fetch is one request with its redirects and, when it must be read, the `robots.txt` it needs. A page and its image are two fetches, also on one host.
- A `429` or a `503` stops the fetches from the host that answered, also when it answered a redirect, an image, or `robots.txt`. They stop for its `Retry-After`, in seconds or as a date, for at least 1 second and at most 1 hour. A missing or unreadable `Retry-After` means 1 minute.
- A job that has to wait for its turn comes back later. No worker waits, and it uses none of the job's attempts, so even a burst of links to one site ends with every card and no failed job. The jobs of one host wake about one per second.
- A URL whose host stopped the crawler keeps its card, is not remembered as failed, and is fetched when the pause ends. After 5 such answers for a URL within 24 hours, the page counts as failed like any other that is not a `200`, and an image is left out of the card.
- A card that waits for its image keeps what it read from the page for up to 2 hours, so that the page is not requested again.
- [Rules on a domain](instance-settings.md#link-card-domains) and domain actions come first: nothing waits for a host that nothing is fetched from.

The limits live in the key-value store and all expire: a worker that stops in the middle of a fetch frees its host within 2 minutes.
These numbers are not settings. To see the limits at work:

- `my.link_card.host_pauses` counts the pauses by the status that the host answered, and each pause logs `link card host paused` at `INFO` with `host`, `status`, and `pause_seconds`.
- `my.job.duration` with `my.job.type="content.fetch_link_card"` and `my.outcome="postponed"` counts the runs that came back later. A large number means many links to few sites or a long pause, not a failure.
