# Link previews (OGP / X Card / fediverse:creator)

This page explains link previews to operators and developers and lists the steps to check them by hand.

## What the server returns

`my serve` returns the HTML of the landing page (`https://<host>/`), the instance information page (`https://<host>/about`),
the pages of the published documents (`/about/terms`, `/about/privacy`, `/about/commerce`), profiles
(`https://<host>/@alice`), and posts (`https://<host>/@alice/status/<post ID>`) by embedding meta tags into the shell of the
web client.
A request for HTML is always treated as signed out, so a preview shows only what a signed-out visitor can see.

| Page | Response |
| --- | --- |
| The landing page (`/`) and the instance information page (`/about`) | 200. The shell with meta tags that introduce the instance (see "The landing page" below) |
| The pages of published documents (`/about/terms`, `/about/privacy`, `/about/commerce`) | 200. The shell with meta tags that introduce the instance, titled "document name \| instance name". The page of a document that is not published gets the generic shell |
| A `public` post, the profile of an active local account | 200. The shell with meta tags |
| An `unlisted` post | 200. The shell with meta tags, plus `<meta name="robots" content="noindex, noarchive">` and `X-Robots-Tag: noindex, noarchive` |
| A `local`, `private`, `direct`, deleted, or nonexistent post, and the profile and posts of a suspended, deleted, or limited account | 404. The generic shell and `X-Robots-Tag: noindex, noarchive`. The body and headers are the same whatever the reason |
| `/@<old username>/status/<post ID>` | 301, to the URL with the current username |

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
| `og:locale`, `<html lang dir>` | The language of the post. When it is unset (`und`), the author's default posting language, and failing that the instance's `default_locale`. It does not change with `Accept-Language` |
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
- `og:image` is number 3 below (the instance preview image), or number 4 if there is none.
- It does not belong to an account, so `fediverse:creator` and the ActivityPub `alternate` are not output.
- The instance information page (`/about`) has the same meta tags. `og:title` and `<title>` are "About {instance name}"
  (`server:preview.about_title`), and `og:url` and `<link rel="canonical">` are `https://<host>/about`.
- The pages of published documents (`/about/terms`, `/about/privacy`, `/about/commerce`) have the same meta tags too.
  `og:title` and `<title>` are "{document name} | {instance name}" (`server:preview.document_title`), and `og:url` and
  `<link rel="canonical">` are the address of that page. The page of a document that is not published gets the generic shell.

`og:image` is chosen in this order.

1. The first image attached to the post (its alt text goes into `og:image:alt`). It is not used for a post marked sensitive or
   with a content warning.
2. The profile's header image, or else the avatar (`og:image:alt` is "Header image of {name}" or "Avatar of {name}").
3. The instance preview image (the instance setting `preview_image`; `og:image:alt` is the text `preview_image_alt`, or "Image
   of {instance name}" if there is none).
4. The bundled default image (`https://<host>/preview-image.png`, a 1200x630 PNG. It is a generic picture without the instance
   name, and `og:image:alt` describes the picture).

So a page that shows a preview always has `og:image` and `og:image:alt`.
The images of 1 to 3 are the `full` variant of the processed media, and the width and height are the values from processing.
If `preview_image` is still processing, failed processing, or is not an image, number 4 is used.
Instance settings are read on every request, so `my instance set preview_image ...` takes effect without a restart (for the
procedure, see [instance settings](instance-settings.md)).

The default image is drawn with the standard library on the first request after startup and kept in the process (about 10 KB;
no binary is kept in the repository, and the dimensions can be written as constants in `og:image:width` / `height`).
It is served with `Cache-Control: no-cache` and an ETag.

### Caching

- The response carries `Cache-Control: public, max-age=60, stale-while-revalidate=300`. A deletion, a change of visibility, or
  publishing or unpublishing a document stays in shared caches for at most that long.
- The ETag is built from the versions of the template, the assets, and the instance settings, the language, the version
  (revision) of the post, the effective visibility, the version of the profile, and the chosen image (for the landing page, the
  information page, and the document pages: the address of the page, its title, the instance description, and the chosen image).
  `If-None-Match` gets `304`.
- If you put a CDN in front, include `Accept` in the cache key (so that AP JSON and HTML are not mixed up).

## Manual check

Check each service on a real device at every release. You need an instance that is reachable from outside over HTTPS (the
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
