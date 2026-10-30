# Instance settings

This page is for operators. It covers the settings that define how your instance is run: registration mode, post length,
default locale, who can see the timelines, the instance name, description, terms, rules, theme, and custom emoji.

## Configuration and instance settings

| | Configuration | Instance settings |
| --- | --- | --- |
| What | How the deployment is built: roles, adapters, endpoints, host names, secrets | How the instance is run: registration mode, post length limit, default locale, who can see the timelines, the instance name, description, and terms |
| Where | Environment variables `MY_*` and the configuration file ([configuration reference](../reference/configuration.md)) | The database. Every change is kept as a revision |
| Changing | Takes effect when the process restarts | Takes effect immediately with `my instance set`, the admin screens of the web client, or the client API ([admin screens and the client API](#admin-screens-and-the-client-api)) |
| When unset | The default in the schema (the server does not start if the key is required) | The default defined in the code |

- **No key is in both.** Neither overrides the other. You cannot give an instance setting through an environment variable or
  the configuration file, and you cannot store a configuration value in the database. A test
  (`TestConfigAndInstanceSettingsShareNoKey`) guarantees that the two sets of keys do not overlap.
- To decide where a new value belongs, ask whether an operator changes it without a restart. A value that is fixed per
  deployment and changes the structure of the process is configuration; a value that changes while the instance is running is
  an instance setting.

## Prerequisites

- The configuration is given as described in the [configuration reference](../reference/configuration.md) (`my config check`).
- The schema is applied to the database (`my schema apply`; see [Database](database.md)).

## Keys

### Settings (one key, one value)

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `registration_mode` | `open` / `approval` / `invite` / `closed` | `approval` | The registration mode ([registration](registration.md)). While a service shutdown is announced and after it is closed, the effective value is `closed`. The stored value does not change, and cancelling the announcement restores it. `GET /instance` and NodeInfo show the effective value |
| `max_status_length` | integer 1-100000 | `500` | The maximum length of a post, in characters |
| `default_locale` | BCP 47 language tag | `en` | The default locale. It is the initial UI locale of new users, and the fallback when no catalog matches a user's preference |
| `local_timeline_access` | `off` / `logged_in` / `everyone` | `everyone` | Who can see the local timeline (`GET /api/v1/timelines/local`): nobody, signed-in users, or everyone. `off` answers 404 and `logged_in` answers visitors who are not signed in with 401; in both cases the navigation hides it. Unless the value is `everyone`, the landing page shows no sample posts |
| `federated_timeline_access` | `off` / `logged_in` / `everyone` | `everyone` | Who can see the federated timeline (`GET /api/v1/timelines/public`). The values mean the same as for the local timeline |
| `preview_image` | media ID (empty = unset) | empty | The link preview image (`og:image`) for posts and profiles without an image. When unset, the bundled default image is used ([link previews](link-previews.md)) |
| `contact_account` | username of a local account (empty = unset) | empty | The account to contact the operators. See [contact and thumbnail](#contact-and-thumbnail) |
| `contact_email` | email address (empty = unset) | empty | The email address to contact the operators. It is also the `Reply-To` and the signature of the emails the server sends |
| `thumbnail` | media ID (empty = unset) | empty | An image that represents the instance (thumbnail) |
| `remote_media_policy` | `none` / `avatars` / `all` | `none` | Whether to keep copies of remote media. `none`: the media proxy relays every request. `avatars`: avatars, headers, and custom emoji are cached in blob storage for `remote_media_cache_ttl`. `all`: all remote media is. The total size of the cache is limited by the configuration `remote_media_cache_max_bytes` |
| `remote_media_cache_ttl` | integer 60-31536000 (seconds) | `86400` | How long fetched remote media may be kept. It is also the `Cache-Control: max-age` of the media proxy responses, so CDNs and browsers cache for this long |
| `authorized_fetch` | `true` / `false` | `false` | Authorized Fetch (secure mode). When `true`, an ActivityPub GET without a verifiable HTTP signature is answered with `401` for objects and collections, and with a minimal document that carries the key for a user's actor. The instance actor always returns its whole document, so any server can fetch the key to verify a signature |
| `federation_mode` | `off` / `inbound` / `outbound` / `both` | `both` | The direction of federation. `off`: no federation. `inbound`: you follow and read remote accounts, but local posts, profiles, and reactions are not sent out, and follows from outside are refused (`Reject`). `outbound`: you accept follows from outside and deliver local posts, but remote posts and reactions are not taken in. `both`: send and receive. A change takes effect at once and needs the permission `manage_settings` |
| `dynamic_client_registration` | `true` / `false` | `true` | Dynamic client registration (RFC 7591). While `true`, `POST /oauth/register` accepts the registration of public clients without authentication. When `false`, the endpoint refuses every request with `403` (`access_denied`); clients that are already registered keep working. A periodic job deletes dynamically registered clients that are not used, following the configuration `dynamic_client_unredeemed_ttl` and `dynamic_client_idle_ttl`. A change takes effect at once and needs `manage_settings` |
| `domain_actions_visibility` | `none` / `users` / `all` | `none` | Who can see the list of domains with actions taken (`GET /api/v1/instance/domain_actions`): nobody, signed-in users, or everyone ([moderation](moderation.md#publishing-the-domains-you-act-on)) |
| `domain_actions_rationale_visibility` | `none` / `users` / `all` | `none` | Who can see the public comments of that list. Someone who cannot see the list never sees them |
| `appeal_window` | integer 20-365 (days) | `180` | How many days after an action the person concerned can appeal ([moderation](moderation.md#appeals)). Changing it also changes the deadline of actions already taken |
| `retention_session_origins` | integer 90-3650 (days) | `180` | How many days after a login to keep the IP address and User-Agent of the login ([moderation](moderation.md#record-retention-and-holds)) |
| `retention_registration_origins` | integer 90-3650 (days) | `180` | How many days after registration to keep the IP address of the registration |
| `retention_reports` | integer 180-3650 (days) | `365` | How many days after the last resolution to keep a report. A report that an action refers to is kept as long as the action |
| `retention_moderation_actions` | integer 180-3650 (days) | `365` | How many days after the end of an action (the lift, the expiry, or the review of an appeal, whichever is later) to keep account actions and appeals |
| `retention_audit_log` | integer 365-3650 (days) | `730` | How many days after being recorded to keep an audit log row |
| `search_indexing` | `allow` / `deny` | `deny` | Whether search engines may index the instance. Even with `allow`, only the profiles of `indexable` accounts and `public` posts are indexed ([search engines and AI crawlers](crawlers.md)) |
| `ai_usage` | `allow` / `deny_training` / `deny` | `deny` | The statement to AI services. `deny_training` refuses training; `deny` refuses training and use by AI (AI search, assistants, agents) |
| `indexable_default` | `true` / `false` | `false` | The default of `indexable` for new accounts. Changing it does not change existing accounts |
| `security_contact` | `https:` or `mailto:` URI (empty = unset) | empty | Where to report security problems of this server: how it is configured and run, and its accounts and data. When set, the server serves `/.well-known/security.txt` (RFC 9116). This is your contact, not the software's: forward a report of a vulnerability in the software itself to the project as its [security policy](../../SECURITY.md) describes |
| `require_media_description` | `true` / `false` | `false` | Whether every image attached to a local post needs alt text. When `true`, a post or an edit with an image whose alt text is missing or only whitespace is rejected with 422. It is published as `limits.require_media_description` in `GET /api/v1/instance`, and the web client makes the description field required. It is separate from the per-user setting `require_media_description`, which only checks before sending |
| `link_cards_enabled` | `true` / `false` | `true` | Whether to show cards for links in posts. When `true`, the server fetches the linked page to build a card. When `false`, it stops fetching and also hides cards already built |
| `oauth_metadata_document_clients` | `true` / `false` | `true` | Whether to accept Client ID Metadata Document clients. When `true`, for an authorization request whose `client_id` is an `https` URL, the server fetches the metadata at that URL and treats it as the client (no prior registration). The fetch follows no redirects, reads at most 5 KB, and reuses the response for at most 24 hours following its cache directives. When `false`, it stops fetching and refuses authorization requests, token exchanges, and refreshes of these clients |
| `finance_visibility` | `private` / `totals` / `breakdown` | `private` | How much of the running costs (recorded on `/admin/finance`) to publish. `private`: nothing is published (`GET /api/v1/instance/cost_totals` is 404). `totals`: only the total per period and currency. `breakdown`: the totals and a breakdown by provider and category, shown to everyone on the information pages (`/about` and `/about/costs`) and in the API. `breakdown` reveals which provider (for example, which cloud) you run on. At every level, item names, memos, and individual payments stay private. Periods up to and including the one containing today (UTC) are published |
| `finance_viewer_item_names` | `true` / `false` | `false` | Whether users with the permission `view_finance` see cost item names and the service names of imported costs on the finance screen (`/finance`, `GET /api/v1/finance/cost_totals`). Totals and the breakdown by provider and category are always shown, whatever this setting is |
| `finance_viewer_item_memos` | `true` / `false` | `false` | Whether users with `view_finance` see the memos of cost items on the finance screen. If either the name or the memo setting is `true`, each period shows item rows (provider, category, and the amount of that period, plus the name and memo where enabled). People with `manage_finance` see everything regardless |
| `finance_viewer_income` | `totals` / `plans` | `totals` | Whether users with `view_finance` see only the income totals (`totals`) or also the number of subscribers per plan (`plans`). The amount per plan and who paid are never shown. If `income_visibility` is `plans`, the published counts are shown whatever this value is. Income is read on the finance screen of the web client (`/finance`) and in `GET /api/v1/finance/income_totals` |
| `income_visibility` | `private` / `totals` / `plans` | `private` | How much of the supporter income (the amount after the payment provider's fees) to publish. It is chosen separately from `finance_visibility`. `private`: nothing is published (`GET /api/v1/instance/income_totals` is 404). `totals`: only the income total per period and currency. `plans`: also the current number of subscribers per plan, shown to everyone. For a plan with few subscribers, the count can let people guess who subscribes. The balance (income minus costs) is shown only when neither this value nor `finance_visibility` is `private`; otherwise the balance would reveal the side you hid. At every level, individual payments, subscribers, the amount per plan, and gross amounts and fees stay private. Periods up to and including the one containing today (UTC) are published |
| `supporter_list_visibility` | `off` / `public` / `signed_in` | `off` | Who can see the list of supporters (`GET /api/v1/instance/supporters` and `/about/supporters`). `off`: nobody (the API answers 404 and `/about` has no link). `public`: everyone. `signed_in`: only people signed in to this server (the API answers 401 otherwise). The list shows only the accounts and plans of active subscribers who chose to be listed; amounts, monthly or yearly billing, dates, and counts are never shown. The list page and the API response are always `noindex, noarchive`, whatever `search_indexing` is |
| `user_app_registration` | `true` / `false` | `true` | Whether signed-in users can register their own OAuth apps (public or confidential) with the client API (`POST /api/v1/oauth_apps`). When `false`, registering answers 403. Listing, changing, and deleting registered apps, reissuing secrets, issuing a token to yourself, and authorization by other users keep working |
| `voice_room_starters` | `everyone` / `permitted` | `permitted` | Who can open voice chat rooms, while the configuration `voice` is not `off` ([configuration](../reference/configuration.md#voice)). `everyone`: every signed-in local user who is not suspended or limited. `permitted`: only the holders of the permission `start_voice_rooms`, which only the built-in `admin` role has by default and which a role of a supporter plan may have |
| `voice_public_summary` | `true` / `false` | `false` | Whether the page of a voice chat room whose visibility is `local` shows its name, description, host, and whether it is still open to visitors who are not signed in to this server and to remote users, and in its link preview. The participants are never shown. When `false`, those visitors see only that the room is for the local users of this server |
| `public_counts` | `true` / `false` | `true` | Whether the landing page (`/`, signed out) and `GET /api/v1/instance/overview` show how many members the server has, how many public posts its users made on each of the last 7 days, and when the server started. Only public posts of this server's users count; the numbers are refreshed every few minutes. When `false`, the API leaves them out |
| `public_operators` | `true` / `false` | `true` | Whether the landing page and `GET /api/v1/instance/overview` list the people who run the server: the accounts of this server that show the badge of a staff role (the built-in `admin` or `moderator` role, or a role with a moderation or server permission) on their profile, with their badges. A staff member who does not show their badge is never listed. When `false`, the API leaves the list out |
| `featured_accounts` | usernames of up to 20 local accounts, separated by spaces or commas (empty = none) | empty | The accounts that the suggestions of people to follow show first, in this order, before the people of this server who posted recently. An account whose owner turned off "Suggest my account and find it by part of its name" is not suggested. See [suggestions](#suggestions-of-accounts) |
| `invite_creators` | `everyone` / `permitted` | `permitted` | Who can create invites of their own besides the staff with `manage_invites` ([member invites](registration.md#member-invites)). `everyone`: every signed-in local user who is not suspended or limited. `permitted`: only the holders of the permission `create_invites`, which only the built-in `admin` role has by default and which a role of a supporter plan may have. In `approval` mode, a usable invite approves the new user at once, so `everyone` lets every user skip the review |
| `member_invite_max_active` | integer 0-100 | `5` | How many usable invites (not revoked, expired, or used up) one member may hold at once; `0` lets members create none. Staff invites do not count |
| `member_invite_max_uses` | integer 1-1000 | `10` | How many people may sign up with one invite that a member creates |
| `member_invite_max_expires_in` | integer 3600-31536000 (seconds) | `2592000` (30 days) | How long one invite that a member creates may work at most. Lowering it does not shorten existing invites |
| `retention_voice_participants` | integer 30-3650 (days) | `90` | How many days after a voice chat room ended to keep the records of who took part in it: the participations, the speakers and listeners, the raised hands, the invitations, and the removals. The room itself (its name, description, hosts, and how it ended) is kept. The records of a reported room are kept as long as the report, and holds apply as to the other records |

- Values are normalized before they are stored (`TRUE` becomes `true`, `0750` becomes `750`, `ja_jp` becomes `ja-JP`, media IDs are lowercased).
  An invalid value is rejected and nothing is written.
- `preview_image` and `thumbnail` accept only the media of an image that exists (including one still being processed).
  Until processing finishes, and in place of an image whose processing failed, `preview_image` falls back to the default image.
  Once a media has been set in any revision, the cleanup of unattached uploads does not delete it.
- `contact_account` accepts only the username of a local user who is not deleted (stored in lowercase). `contact_email`
  accepts only an address of the form `alice@example.com`.
- `featured_accounts` accepts only the usernames of local users who are not deleted; a leading `@` is dropped, the names
  are lowercased, a repeated name is kept once, and they are stored separated by single spaces.

### Suggestions of accounts

Signed-in users see suggestions of people to follow (on the empty home timeline after sign-up, and with
`GET /api/v1/accounts/suggestions`). The suggestions are the accounts of `featured_accounts` in their order, then the
accounts of this server that published a public or local post in the last 30 days, the one that posted last first. The
user's own account, the accounts the user follows, blocks, mutes, or is blocked by, and accounts that are suspended or
deactivated are left out.

New accounts are suggested and can be found by part of their name in the account search. A user who turns off "Suggest my
account and find it by part of its name" in the profile settings leaves both: the search then finds the account only by its
whole username, or for the people who follow it.

### Texts (one key, one value)

| Key | Limit | Description |
| --- | --- | --- |
| `name` | 100 characters, one line (whitespace alone is not allowed) | The instance name (display name). See [name and domain](#name-and-domain) |
| `short_description` | 500 characters | A short description |
| `description` | 20000 characters | The description |
| `terms` | 100000 characters | The terms of service (Markdown) |
| `privacy_policy` | 100000 characters | The privacy policy (Markdown) |
| `commerce_disclosure` | 100000 characters | The seller's disclosure for paid plans, such as the notice that Japanese law requires (Markdown; [supporters](supporters.md)) |
| `preview_image_alt` | 1500 characters | The alt text of the `preview_image` image (`og:image:alt`). When unset, "Image of {instance name}" is used |
| `thumbnail_alt` | 1500 characters | The alt text of the `thumbnail` image |

- Each text has one value. An administrator writes it once in whatever language they like, and readers of every locale see the
  same value; readers of other languages rely on machine translation or the like. The labels and descriptions of the admin
  fields are translated into the viewer's language like any other screen text; only the value an administrator writes is not.
- The server does not know the language of a text, so the web client and the screens rendered by the server output texts with
  `dir="auto"` and `lang=""` (the HTML value for an unknown language) instead of inheriting the language of the screen.
- Setting an empty text deletes it.
- Besides the CLI, texts can be changed on the admin screens of the web client (name, short description, and description in
  "Server settings", `/admin/settings`; terms, privacy policy, the commerce disclosure, and the like in "Terms and policies",
  `/admin/documents`) and with the client API (`GET /api/v1/admin/instance_texts`, `PUT /api/v1/admin/instance_texts/{key}`).
  Both need the role permission `manage_settings`, and changes are recorded in the audit log (`text_changed`).

### Terms and privacy policy

- Write `terms`, `privacy_policy`, and `commerce_disclosure` in Markdown (CommonMark plus the GitHub Flavored Markdown tables,
  strikethrough, and autolinks). A line break inside a paragraph stays a line break, and `#` headings start one level below the
  page title (`h2`).
- The server converts them to HTML and sanitizes it (anonymous `GET /api/v1/instance/documents/{kind}`), and the web client
  pages `/about/terms`, `/about/privacy`, and `/about/commerce` show them. Raw HTML, images, and links other than `http`,
  `https`, and `mailto` are removed. The pages need no sign-in (they are web client pages, so they need JavaScript).
  `/about/commerce` is the URL to give to the payment provider.
- A document you write is linked from the registration screen, the footer of every screen the server renders, and the web client (the third column, the menu, the settings, and the sign-in screen).
  A document that is not written is not linked, and its API answers 404.
- While the terms of service or the privacy policy is written, the registration screen lists them before the create button
  with a required checkbox, "I have read and agree to the documents above". The server refuses a registration without it,
  also one through a login provider. Without either document, the screen asks for no agreement.

### Name and domain

An instance has a domain and a name, separately.

- The **domain** is the configuration `host` (and `account_domain`, the domain of handles). It cannot be changed while the
  instance is running. Handles (`@alice@example.com`), URLs, ActivityPub IDs, and `domain` in `GET /api/v1/instance` stay the
  domain even when there is a name.
- The **name** is the text `name`. It appears wherever the instance is named to people: the sidebar and "About this server" of the
  web client, the browser tab title, the sign-in, registration, email confirmation, account status, and authorization consent
  screens rendered by the server, emails (email address confirmation and moderator notifications), `og:site_name` of link
  previews, `name` / `short_name` of the Web App Manifest, `metadata.nodeName` of NodeInfo, and `name` in `GET /api/v1/instance`.
- The name is the same in every locale. When there is no name, the domain (`host`) is shown instead.
- A change takes effect from the next request, without a restart. The tab title and sidebar of an open web client update when
  you save on the admin screen.

### Contact and thumbnail

The contact for the operators (`contact_account`, `contact_email`) and the image that represents the instance (`thumbnail`
and the text `thumbnail_alt`) appear in the places below. Whatever is unset does not appear.

- "About this server" of the web client (`/about`). The thumbnail and the contact account also appear on the first screen (`/`)
  for people who are not signed in.
- `contact` (`account` and `email`) and `thumbnail` (`url`, `width`, `height`, `alt`) in `GET /api/v1/instance`.
- `metadata` in NodeInfo: `maintainer` by the Misskey convention (`name` is the display name of the contact account, or its
  username when it has none; `email` is `contact_email`), `staffAccounts` by the Pleroma and Akkoma convention (the actor ID of
  the contact account), `thumbnail` (`url` and `alt`), and `nodeDescription` from the short description `short_description`.

When the contact account is suspended or deleted, the account is not shown and only the email address is.
Change them on the admin screen of the web client ("Server settings", `/admin/settings`), or with the CLI or the client API.

## Usage

```sh
my instance list                                      # all settings (and whether each is the default) and texts
my instance get registration_mode
my instance set registration_mode open
my instance set name "Example Social"                 # the same name in every locale
my instance set description - < description.txt       # the value "-" reads standard input
my instance get name
my instance set name ""                               # delete the name
my instance history registration_mode                 # revision history (newest first, -limit sets the count)
my instance history description
my instance set -owner admin preview_image ./preview.png   # upload the image as media of admin and use it as the preview image
my instance set preview_image 0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f   # give the ID of media that is already uploaded
my instance set preview_image_alt "A blue bird mascot"
my instance set preview_image ""                      # back to the bundled default image
my instance set contact_account admin                 # the contact account
my instance set contact_email admin@example.com
my instance set -owner admin thumbnail ./thumbnail.png  # the thumbnail
my instance set thumbnail_alt "A town by the sea"
```

- Write flags before the key and value.
- With `-owner <username>`, the value of `preview_image` or `thumbnail` is read as the path of an image file and uploaded as
  media of that local account. The worker processes the image before it is used. A landscape image of 1200x630 is recommended.
- A change records a revision (`instance_*_revisions`) and updates the current value (`instance_settings` / `instance_texts`)
  in one transaction. A change from the CLI is recorded without an actor (`BY` is `cli` in the history).
- Setting the same value again still adds a revision.
- Exit codes: 0 for success, 1 for a rejected value or a database error (nothing changes), 2 for wrong arguments.

### Changing the direction of federation

- Narrowing it keeps follow relationships, imported posts, and delivery records, and sends no `Undo` or `Reject`. The flow just
  stops, and returning to `both` (or another mode) resumes it as it was.
- Posts and reactions made while narrowed are not delivered retroactively after you widen it again.
- On the remote side, local users' followers receive nothing while the mode is `inbound`, and local users' follows are not
  updated while it is `outbound` or `off`.
- Deletions (of posts or accounts) made while the mode is `off` are not sent out. In `inbound` and `outbound`, the retraction
  of what was already sent out (`Delete` and the like) goes to the original recipients and the followers at that time.
- Changes are recorded in `instance_setting_revisions` and the audit log (`target_kind = setting`).

To cut the relationships too, use the CLI `my admin federation sever`.

```sh
my admin federation sever -direction in     # end follows of local users by remote accounts
my admin federation sever -direction out    # end follows of remote accounts by local users
my admin federation sever -direction both
```

- It ends the pending and active follows between local and remote accounts as terminations by moderation
  (`follow_terminations`, `reason = moderation`). A change of mode never calls it automatically.
- `Reject` (`in`) or `Undo` (`out`) is sent to the remote side only when the current mode sends that direction. In a mode that
  does not (such as `off`), the follow ends on the local side only and the remote side is not told.
- It is safe to run repeatedly. It prints the number of follows it ended to standard output.

## Admin screens and the client API

Every setting the CLI can change can also be changed on the admin screens of the web client and with the client API. Both need
the role permission `manage_settings` (only the built-in `admin` role has it). As with the CLI, a change is kept as a revision
and recorded per key in the audit log (`setting_changed`, the target is the setting and the actor is that user). Look at the
audit log on the moderation screen ("Audit log", `/moderation/audit-log`).

- **Web client** ("Server settings", `/admin/settings`): you can change the name and description, and the settings below.
  The settings are saved together in one form, and only what you changed is sent.

  | Section | Settings |
  | --- | --- |
  | Registration | `registration_mode` |
  | Language | `default_locale` (chosen from the languages the app has) |
  | Timelines | `local_timeline_access`, `federated_timeline_access` |
  | Posts | `require_media_description` |
  | Federation | `authorized_fetch`, `remote_media_policy`, `remote_media_cache_ttl` (choose from 1 hour, 1 day, 7 days, 30 days, or 365 days), `link_cards_enabled` |
  | Limited and suspended servers | `domain_actions_visibility`, `domain_actions_rationale_visibility` |
  | Running costs | `finance_visibility` (the `breakdown` option says that the provider and category become known), `finance_viewer_item_names`, `finance_viewer_item_memos`, `finance_viewer_income` (how much people who can view finance see) |
  | Income | `income_visibility` (the `plans` option says that, for a plan with few subscribers, people may guess who subscribes) |
  | Supporter list | `supporter_list_visibility` |
  | Landing page | `public_counts`, `public_operators` |
  | Suggested accounts | `featured_accounts` |
  | Search engines and AI | `search_indexing`, `indexable_default`, `ai_usage`, `security_contact` |
  | Apps | `user_app_registration`, `oauth_metadata_document_clients`, `dynamic_client_registration` |

  Contact and thumbnail (`contact_account`, `contact_email`, `thumbnail`, and the text `thumbnail_alt`) are changed in the
  "Contact and thumbnail" section below the name and description. An image is uploaded when you choose it and becomes the
  thumbnail when you save.

  `max_status_length`, `preview_image`, `appeal_window`, and the retention periods (`retention_*`) do not appear on the screen;
  change them with the CLI or the client API.
- **Client API**: `GET /api/v1/admin/settings` returns every setting with its current value, its default, whether it is set,
  and its type and accepted values (choices or range). `PATCH /api/v1/admin/settings` changes settings together with
  `{"values": {"<key>": "<value>", ...}}`. All values are strings, normalized and stored as with the CLI. If a single key is
  unknown or a value is invalid, the answer is a 422 that names the key and nothing changes. The changed keys are recorded as
  revisions and in the audit log in the same transaction.
- Invitations, which work in the `open`, `approval`, and `invite` modes, are created on the moderation screen ("Invites",
  `/moderation/invites`, permission `manage_invites`; see [invite codes](registration.md#invite-codes)), and by members
  as `invite_creators` allows ([member invites](registration.md#member-invites)).

## Rules

The rules of the instance appear in `rules` of `GET /api/v1/instance`, and reports cite them by ID.

- A rule has a one-line sentence (up to 300 characters) and an optional supplement (up to 1,000 characters). As with texts, the
  administrator writes them in whatever language they like and every locale sees the same value (`dir="auto"` and `lang=""`).
  There can be at most 100 active rules.
- Every change (a revised sentence, a reordering) is kept as a revision (`instance_rule_revisions`), and the ID of a rule does
  not change.
- A retired rule disappears from the list of active rules. It cannot be brought back, but reports can still refer to it.

```sh
my instance rule add -hint "No harassment" "Be kind"   # add at the end of the active rules
my instance rule edit <rule> "Always be kind"          # change the sentence; the supplement changes only with -hint
my instance rule move <rule> 1                         # move to position 1 and renumber the active rules 1, 2, ...
my instance rule retire <rule>                         # retire it
my instance rule list                                  # active rules (-all includes retired ones)
```

- The admin API is `/api/v1/admin/rules` (list, add, get, revise, move, retire) and needs the role permission `manage_rules`.
- On the admin screen of the web client ("Rules", `/admin/rules`, `manage_rules`) you can add a rule with a sentence and a
  supplement, edit it, reorder it with "Move up" and "Move down", and retire it (after a confirmation dialog). Every change
  is recorded in the audit log (`rule_added`, `rule_revised`, `rule_moved`, `rule_retired`).
- The exit codes are the same as for settings (0 for success, 1 for a rejected value or a database error, 2 for wrong arguments).

## Theme

The server-wide theme is a set of overrides of the web client's design tokens plus one optional piece of CSS (custom CSS). It
applies to the web client of every viewer and to the screens the server renders (sign-in, registration, terms, and so on), in
both light and dark.

- You can override every token in `app/ui/src/theme.css` except the primitive colors (`--palette-*`). The list, defaults, and
  ranges are in [app/ui/README.md](../../app/ui/README.md#server-themes). A color is `#rrggbb` for light and for dark
  (`color-overlay` may also be `#rrggbbaa`); the other tokens take a value of the shape of their kind (length, number,
  duration, easing, font name list, shadow) within its range. When you override `font-sans` or `font-mono`, keep
  `"my Noto Color Emoji"` first; otherwise the emoji of the web client are drawn with the OS font.
- When you save, the server checks the shape and range of the values, the contrast of every pair of color tokens (tokens you do
  not override use their defaults; WCAG 2.2 AA in both light and dark), and the length of the custom CSS (64 KiB). If any check
  fails, nothing is saved.
- The server does not read the custom CSS. It puts it in the same stylesheet after the token rules, outside any `@layer`. It runs
  under the same CSP as the web client, so images, fonts, and `@import` from another origin cannot load.
  **The server does not check the effects of custom CSS (contrast, horizontal scrolling, tap target size, focus indication,
  right-to-left languages); that is the administrator's responsibility.** Class names and the page structure change between
  versions, so only the tokens are stable to rely on.
- A theme is kept as revisions (`instance_theme_revisions`), and the stylesheet of the current revision is served at
  `/theme/{revision ID}.css` (`immutable`, cached for a long time). The HTML shell has a shared cache (60 seconds plus 300
  seconds of revalidation), so it takes a few minutes after you save for everyone to get it. A page that is already open keeps
  the old look until it is reloaded.
- `<meta name="theme-color">` (light and dark) in the shell and `theme_color` / `background_color` (light) of the Web App
  Manifest take the value of the theme's `color-surface`.
- Saving a revision with no tokens and no custom CSS resets the theme to the default, and the stylesheet is no longer loaded.
- You can change it on the admin screen of the web client ("Theme", `/admin/theme`), with the client API (`GET` / `PUT
  /api/v1/admin/theme`), and with the CLI. The admin screen and the client API need the role permission `manage_settings`, and
  changes are recorded in the audit log (`setting_changed`, target `theme`).

```sh
my instance theme get > theme.json                    # print the current theme as the same JSON as the admin API
my instance theme set < theme.json                    # replace the whole theme (if a check fails, it prints why and changes nothing)
echo '{"tokens":[],"custom_css":""}' | my instance theme set   # back to the default
```

```json
{
  "tokens": [
    { "token": "color-accent", "light": "#1447e6", "dark": "#92c5fe" },
    { "token": "radius-md", "value": "0.25rem" }
  ],
  "custom_css": ".brand { letter-spacing: 0.02em; }"
}
```

## Logo and icon

Besides the thumbnail and the preview image, the instance can have its own icon, logo, and dark logo, each uploaded as one SVG.
Until one is set, the built-in images of my are used, and clearing one brings the built-in image back.

| Image | Shape | Where it appears |
| --- | --- | --- |
| `icon` | Square (within 1%) | The favicon, the home screen icon of iOS (`apple-touch-icon`), the icons of the installed app (Web App Manifest), the server's card on the landing page and beside its name in the web client's side panel, and wherever a client shows the server as a square picture |
| `logo` | 1 to 8 times as wide as it is high | The header of the web client and the top of the sign-in, registration, and consent pages instead of the built-in wordmark and the server name (the logo should contain the name, which is its alt text), the HTML version of every email, and the link preview image when `preview_image` is not set |
| `logo_dark` | As `logo` | Dark backgrounds; without it, `logo` is used there too |

- **The server sanitizes the SVG.** It reads it as XML and writes a new document from the allowed elements (shapes, groups,
  `defs`, `symbol`, `use`, gradients, `clipPath`, `mask`, `pattern`, `style`, `title`, `desc`) and attributes (geometry,
  transforms, presentation attributes, `id`, `class`, `style`) only. Scripts, `foreignObject`, embedded images, links,
  animations, filters (shadows and blurs), event handlers, references to anything outside the file, and `data:` URLs are removed,
  and the answer lists what was removed. Only the sanitized SVG is stored.
- An SVG is refused (nothing is saved) when it is not well-formed XML in UTF-8, has a `DOCTYPE` or entity declarations, has no
  `viewBox`, is over 256 KiB (128 KiB after sanitizing), has over 5,000 elements, nests over 32 deep, has over 200 `use`
  elements, has the wrong shape for its kind, contains `text` (convert text to outlines in your editor first), or has CSS that
  could load something (`@import`, `@font-face`, `url()` other than `url(#id)`, escapes). `@media (prefers-color-scheme: dark)`
  in a `<style>` is allowed, so an icon can change in dark mode.
- Some places take no SVG (the iOS home screen, older Safari favicons, link previews, email). For them the client that uploads the
  SVG also draws PNG versions from the sanitized SVG and sends them with it: 32, 48, 180, 192, and 512 px and a 512 px maskable
  icon for `icon`; a 96 px high email logo and a 1200 × 630 link preview image for `logo`; a 96 px high email logo for
  `logo_dark`. The server checks that each is a PNG of the right size (at most 1 MiB; the 180 px icon, the maskable icon, and the
  link preview must be opaque) and writes it again without metadata. If any file is wrong, nothing is saved. The icon's opaque
  versions are drawn on a background color, `#rrggbb`, which defaults to the theme's light `color-surface`. The server does
  not check that the PNG versions show the same picture as the SVG.
- The files are stored in the blob store under `instance/brand/<revision>/` and served at
  `https://<host>/brand/<revision>/<version>.svg|.png` with `Cache-Control: public, max-age=31536000, immutable`, since the
  content of a revision never changes. An SVG is served with `Content-Security-Policy: default-src 'none'; style-src
  'unsafe-inline'; sandbox` and `X-Content-Type-Options: nosniff`, so even opened on its own it runs nothing and loads nothing.
  The files of earlier revisions stay, so emails already sent keep their logo.
- With an icon set, the HTML shell links its PNG favicons, its SVG, and its 180 px `apple-touch-icon`, the Web App Manifest lists
  its 512 px, 192 px, maskable, and SVG icons in that order (other fediverse servers take the first one as this server's icon),
  and `/apple-touch-icon.png` returns the 180 px version. The shell's ETag changes with the images, and the shell's shared cache
  lasts a few minutes as with the theme. An installed app picks up a new icon on Android when Chrome next checks the manifest;
  on desktops the app keeps its old icon until it is installed again.
- `GET /api/v1/instance` returns `icon`, `logo`, and `logo_dark` with the URL of the SVG, the URL and size of each PNG version,
  and the revision; each is left out while the built-in image is in use.
- Set them in the web client under **Server settings** > **Icon and logo** (role permission `manage_settings`). Choose an SVG
  file for the icon, the logo, or the logo for dark backgrounds: the server checks it, the browser draws the PNG versions, and the
  form previews them (a browser tab, the home screen, the app icon, an email on a light and a dark background, and the link
  preview) with the list of what the server removed. If the server refuses the file, the reason shows under the button with how
  to fix it. For the icon, set the background color of its opaque versions there too. **Save** sends the SVG with every PNG
  version; **Use the built-in icon**, **Use the built-in logo**, and **Remove the logo for dark backgrounds** clear them. The
  web client keeps the images it has shown for offline use.
- The client API does the same, with the role permission `manage_settings`:
  `POST /api/v1/admin/instance/brand/sanitize` checks an SVG and returns the sanitized SVG with the PNG versions to draw (it
  saves nothing), `PUT /api/v1/admin/instance/brand/{icon|logo|logo_dark}` saves the SVG with every PNG version
  (`multipart/form-data`, one part per version), `DELETE` on the same path clears one, and `GET /api/v1/admin/instance/brand` returns the current
  images with the icon's background color. Every change is kept as a revision (`instance_brand_revisions`) and recorded in the
  audit log (`setting_changed`, target `brand_icon`, `brand_logo`, or `brand_logo_dark`).
- The CLI cannot set them, since the PNG versions are drawn by the client, but it can show them and clear a broken one:

```sh
my instance brand show          # each image with its revision, when and by whom it was set, and the URL of its SVG
my instance brand clear icon    # back to the built-in icon (also logo, logo_dark)
```

## Custom emoji

Local custom emoji are shown as images when someone writes `:shortcode:` in the body or content warning of a post, or in the
display name, bio, or profile fields. They also reach Mastodon, Misskey, GoToSocial, and Akkoma through the `Emoji` tag of the
Notes and actors you send.

- A shortcode is 2-30 letters, digits, or `_`. It is case-sensitive and cannot be changed after the emoji is registered.
  Aliases (up to 10) and a category (up to 64 characters) are used for searching and grouping in the emoji picker.
- The image is PNG, GIF, or WebP, at most `custom_emoji_max_bytes` (default 256 KiB) and at most 1,024 px on each side. The
  server makes derivatives that fit boxes of 64, 128, and 256 px and does not keep the original. An APNG becomes a still image
  of its first frame. GoToSocial by default does not fetch emoji larger than 100 KiB, so small images are recommended.
- A disabled emoji appears nowhere (posts, profiles, federation, the picker), and `:shortcode:` is shown as text again. An emoji
  set to be hidden from the picker is only missing from the picker and can still be used by writing it. An emoji that came from
  another server can only be disabled or copied as an emoji of this server.
- Copying fetches the image of another server's emoji under the same limits as the media proxy (images from domains with
  `reject_media` are not fetched), runs the same checks as above, and registers it as a new local emoji. The shortcode is the
  original's or the one you give. The image shrinks to the 256 px box, and an image that cannot be fetched or fails the checks
  leaves nothing behind.
- Deletion cannot be undone. Emoji in past posts turn back into text too, and registering the same shortcode again makes a
  different emoji. Images that federated servers copied cannot be deleted.
- The admin screen ("Custom emoji", `/admin/custom-emojis`), the client API (`/api/v1/admin/custom_emojis`), and the CLI do the
  same things. The admin screen and the client API need the role permission `manage_custom_emojis` (only the built-in `admin`
  has it). Registering, copying, replacing the image, changing settings and aliases, enabling and disabling, and deleting are
  recorded in the audit log (target `custom_emoji`) whichever way they are done (an operation from the CLI is recorded as a CLI
  operation).

```sh
my admin emoji create -shortcode blobcat -category Cats -alias neko blobcat.png   # register one
my admin emoji list -local                                                        # list local emoji, newest first
my admin emoji update -category Animals -alias cat -alias neko <emoji id>         # change the category and aliases (aliases are replaced; -no-aliases removes them)
my admin emoji update -hidden <emoji id>                                          # hide it from the picker (-hidden=false restores it)
my admin emoji replace-image <emoji id> blobcat2.png                              # replace the image
my admin emoji disable <emoji id>                                                 # disable it (remote emoji too)
my admin emoji enable <emoji id>                                                  # enable it again
my admin emoji list -remote -domain mastodon.example                              # list emoji of another server
my admin emoji copy [-shortcode <name>] <emoji id>                                # copy a remote emoji to local
my admin emoji delete <emoji id>                                                  # delete it
my admin emoji requests list [-state pending|approved|rejected|withdrawn|lapsed|all] # list creation requests (pending by default)
my admin emoji requests approve <request id>                                      # approve a request
my admin emoji requests reject [-reason <text>] <request id>                      # reject a request (the requester sees the reason)
```

- A user with the role permission `create_custom_emojis` (a user feature) can request the creation of an emoji from "Custom
  emoji" in the settings (`/settings/custom-emojis`), change the aliases, category, and picker visibility of emoji they
  created, and delete them. Replacing an image is also a request. A request appears nowhere (picker, posts, reactions,
  federation) until someone with `manage_custom_emojis` approves it. Approve or reject it on the "Requests" tab of "Custom
  emoji" on the admin screen, with the client API (`/api/v1/admin/custom_emoji_requests`), or with the CLI above. People with
  `manage_custom_emojis` are notified of a request (`admin_custom_emoji_request`), and the requester is notified of the
  approval or rejection. Requesting, withdrawing, approving, and rejecting are recorded in the audit log (target
  `custom_emoji_request`).
- A pending creation request reserves its shortcode. A request or a registration with the same shortcode is not possible until
  the pending one is rejected or withdrawn. The image of an approved emoji is copied as an upload of the instance actor, so it
  stays even if the requester's account is deleted.
- A creator can change their emoji only while they hold `create_custom_emojis`. The emoji stay if the role is removed.
  Administrators can change and delete every emoji regardless of the creator.
- When the role comes from a supporter plan, the number of emoji a user can create each month is limited by the plan's
  monthly emoji limit. It counts the creation requests the user submitted in the current calendar month (UTC) that are
  approved or pending; rejected and withdrawn requests do not count, and deleting an emoji does not free a place. A request
  counts in the month it was submitted, even if it is approved in the next one. The count resets at 00:00 UTC on the first
  day of each month, and the settings page shows the user when, in their own time zone. Replacing the image of an emoji, and
  emoji that administrators register, do not count. A role an administrator assigned by hand, and a server where supporters
  are disabled, have no limit.
