# Account and domain actions, and handling reports

This guide is for operators and moderators.

Everything here except account management (admin API only) can be done through the admin API (`/api/v1/admin/...`, scopes `admin:read` / `admin:write`) and the CLI (`my admin account action` / `my admin domain` / `my admin report` / `my admin appeal`).
The admin API requires role permissions: account management, actions and appeals need `manage_accounts`, domain actions need `manage_domains`, and reports need `manage_reports`.
The CLI connects to the DB directly with the same configuration as the server (`-config` or environment variables) and does not record which user operated it (`issued_by_user_id` and similar fields are NULL, and the actor in the [audit log](#audit-log) is `cli`).

## Moderation and server administration in the web client

The web client splits the staff screens in two.
The navigation shows "Moderation" and "Administration" only to people whose role has at least one permission of that group. Every screen shows only a reason to people without the permission, and the server rejects their requests too.

| Group | Permissions | Screens |
| --- | --- | --- |
| Moderation (`/moderation`) | `manage_reports`, `manage_accounts`, `manage_domains`, `manage_invites`, `view_audit` | Reports (`/moderation/reports`), Appeals (`/moderation/appeals`), Accounts (`/moderation/accounts`), Registrations (`/moderation/registrations`), Sign-up blocks (`/moderation/signup-blocks`), Invites (`/moderation/invites`), Domain actions (`/moderation/domain-actions`), Remote servers (`/moderation/instances`), Audit log (`/moderation/audit-log`) |
| Server administration (`/admin`) | `manage_settings`, `manage_rules`, `manage_roles`, `manage_jobs`, `manage_login_recovery`, `manage_custom_emojis`, `manage_webhooks`, `manage_announcements`, `manage_finance`, `manage_oauth_apps`, `manage_service_closure`, `manage_errors`, `manage_login_providers` | Server settings (`/admin/settings`), Rules (`/admin/rules`), Terms and policies (`/admin/documents`), the job queue and failed jobs (`/admin/jobs`), Custom emoji (`/admin/custom-emojis`), Announcements (`/admin/announcements`), Webhooks (`/admin/webhooks`), costs and billing import (`/admin/finance`, `manage_finance`). `manage_login_recovery` (login recovery: removing two-factor authentication and issuing a password-reset link) appears in a section of the account page (`/moderation/accounts/{id}`, which also needs `manage_accounts`) |

- The badge on "Moderation" is the total of open reports, registrations waiting for approval and appeals waiting for review ([notifications for moderators and the pending counts](#notifications-for-moderators-and-the-pending-counts)). "Administration" shows no count.
- The built-in `moderator` has only `manage_reports`, `manage_accounts` and `view_audit`, and does not see "Administration". It also does not see the Domain actions and Remote servers screens (`manage_domains`),
  so to delegate those, create a role with that permission and grant it ([granting and revoking roles](admin-cli.md#granting-and-revoking-roles)).
- The admin API paths stay `/api/v1/admin/...` whatever the group, and the scopes are the same pair `admin:read` / `admin:write`. The groups are separated by tags in the API reference (`admin-moderation`, `admin-server`).
- There is a third group of permissions, "user features" (`view_finance`, `create_custom_emojis`, `start_voice_rooms`, `create_invites`), which lives outside the staff screens. Someone who has only these permissions sees neither "Moderation" nor "Administration",
  and for someone with `view_finance` the navigation shows "Finances" (`/finance`) after Settings.
  Its API is in the user namespace (`GET /api/v1/finance/cost_totals` and `GET /api/v1/finance/income_totals`, scope `read:finance`) and needs no `admin:*` scope. What it shows is set by `finance_viewer_*` in the [instance settings](instance-settings.md).

## Account actions

An action is applied to the restriction projection (`account_restrictions`) in the transaction that records it, and takes effect from the next request.

| Kind | Effect on a local account |
| --- | --- |
| `warn` | No restriction (only informs the user) |
| `force_sensitive` | Shows all attachments as sensitive |
| `limit` | Treats posts (including past ones) as followers-only and keeps them off the public timelines |
| `restrict_posting` | Forbids posting, replying, boosting and editing |
| `restrict_interaction` | Forbids following, favoriting, boosting and replying |
| `disable_login` | Forbids use of the app and the API, and revokes active sessions and tokens (the sign-in page still lets the user see the action) |
| `suspend` | Applies all the restrictions above, and hides the profile and posts from everyone but the user |
| `ban` | Applies the same restrictions as `suspend` and deletes the account after 30 days. It cannot be lifted (it is canceled only when an appeal made during those 30 days is overturned) |
| `delete_statuses` | Deletes the given posts. They become invisible even to the user, and the server sends other servers the same `Delete` as when the user deletes them |
| `mark_statuses_sensitive` | Shows the attachments of the given posts as sensitive |

For a remote account you can issue only `limit`, `suspend` and `force_sensitive` (any other kind gives `422` in the admin API and fails in the CLI).
The effect is local to this server. The only things sent to the other server are the `Reject` / `Undo` of follows cut by `suspend`.

| Kind | Effect on a remote account |
| --- | --- |
| `force_sensitive` | Shows all attachments as sensitive |
| `limit` | Treats posts (including past ones) as followers-only, keeps them off the public timelines, and creates no notifications for users who do not follow the account |
| `suspend` | Drops everything received from the account and hides its profile and posts. Cuts follows between the account and local users in both directions (follow requests too), and sends the other server a `Reject` of the account's follows and an `Undo` of the users' follows. Lifting it brings back the display and the server receives from the account again (what was dropped and the follows that were cut during the suspension do not come back) |

- `notify_target` (`-no-notify`) has no meaning for a remote account (no notification reaches it).
- A local user who lost follows to a `suspend` gets one notification (`relationships_severed`) that tells how many followers and follows were lost ([severed follows](#severed-follows)).
- Issuing and lifting are recorded in the [audit log](#audit-log), as with actions on local accounts.

```sh
my admin account action issue -kind suspend -category spam -public-reason "spam" -duration 720h @alice
my admin account action issue -kind limit -report <report id> @bob@remote.example
my admin account action issue -kind delete_statuses -status <status id> -status <status id> @alice
my admin account action issue -kind ban -category legal -public-reason "illegal content" @alice
my admin account action list -active @alice
my admin account action lift -reason "appealed" <action id>
```

In the admin API: `POST /api/v1/admin/accounts/{id}/actions` (the body has `kind`, `reason_category`, `public_reason`, `private_note`, `report_id`, `notify_target`, `expires_in` in seconds and `status_ids`),
`GET /api/v1/admin/accounts/{id}/actions` (filter with `active`), `GET /api/v1/admin/account_actions/{id}`, and `POST .../{id}/lift` (the body `{"reason": "..."}` is optional).

- Name an account with `@username` (local), `@username@domain` (remote) or the account ID in the CLI, and with the account ID in the admin API.
- Several actions stack (the union of their restrictions). Lifting removes only that action's restrictions, and an action with an end time ends by itself at that time (shown as `expired` in lists). An end time is at most ten years away (`expires_in` up to 315360000); a longer one is refused with `422`.
- Sessions and tokens that `disable_login`, `suspend` and `ban` revoked do not come back when you lift the action. The user signs in again.
- A `suspend` or `ban` of a local account sends an Actor `Update` with an emptied profile to the followers' servers and to known servers to show the suspension (Mastodon shows it as "suspended").
  Lifting and expiry send an `Update` with the original profile. Expiry is sent by a job that runs every 5 minutes, so it can be up to 5 minutes late.
- `delete_statuses` and `mark_statuses_sensitive` take 1 to 100 undeleted posts of the target account through `-status` (`status_ids`). They cannot have an end time.
  Lifting `delete_statuses` does not bring back the deleted posts. Lifting `mark_statuses_sensitive` restores the original display.
- A `ban` cannot have an end time and cannot be lifted. It is canceled only when an appeal the user made during the 30 days before deletion is overturned ([appeals](#appeals)). When in doubt, use a `suspend` with an end time.
  Thirty days after the action, a job that runs every hour deletes the account (the account and posts disappear from view, and the server sends a `Delete` to other servers).
- `report_id` (`-report`) can name only a report about the target account. An open report is resolved as `actioned` together with the action (the resolution is also in the audit log). A resolved report does not change.
- `public_reason` is the reason shown to the user and `private_note` is a note for moderators only. `notify_target` (true by default; false with `-no-notify` in the CLI) decides whether the user is informed.

### What the user sees

- An action with `notify_target` true reaches the user as a notification (`moderation_notice`). It carries the kind, the reason category, `public_reason`, the end time and the state. The kind and the category also come back as display names in the user's UI language.
- The user can check the actions against them with `GET /api/v1/moderation/actions` (scope `read:accounts`, filter with `active`), newest first. Actions with `notify_target` false and lifted or expired actions are listed too.
- `private_note`, the report, who took the action and the reason for lifting are never shown to the user.
- A user can appeal each action once ([appeals](#appeals)). The action list and the action in a notification include whether it can be appealed (`appealable`), the deadline (`appeal_deadline`), and the appeal and its outcome (`appeal`).
- A user under `disable_login`, `suspend` or `ban` cannot use the app or the API but can still sign in on the sign-in page (`/auth/sign_in`). There they see the kind, the reason and the end time of the action in effect
  (for `ban`, the number of days until the account is deleted), and they can appeal from that page ([appeals](#appeals)). The authorization endpoint (`/oauth/authorize`) answers with `403` and issues no token.
  A user under `ban` can sign in until the account is deleted, and the deletion revokes that session too. A user whose own deletion is scheduled, and an account that has been disabled, cannot sign in even on the sign-in page.

### Appeals

A local user can appeal an action against them.
A moderator reviews the appeal and either upholds it (`uphold`) or overturns it (`overturn`).

```sh
my admin appeal list -state pending
my admin appeal uphold -reason "The posts were spam." <appeal id>
my admin appeal overturn -reason "Sorry, our mistake." <appeal id>
```

The user appeals with `POST /api/v1/moderation/actions/{id}/appeal` (scope `write:accounts`, body `{"text": "..."}`).
A user under `disable_login`, `suspend` or `ban` cannot get an access token and appeals through the per-action form on the sign-in page (`POST /auth/appeal`).
The page shows the form and the deadline for an action that can be appealed, the content and outcome for an action already appealed, and the reason for an action whose deadline has passed or that cannot be appealed. For a rejected appeal it shows the reason.
In the admin API: `GET /api/v1/admin/appeals` (filter with `state`), `GET .../{id}`, and `POST .../{id}/uphold` and `POST .../{id}/overturn` (the body `{"public_reason": "..."}` is optional). They need the `manage_accounts` permission.

- An action can be appealed once, and the text is 1 to 2,000 characters. The deadline is the number of days in the instance setting `appeal_window` after the action (default 180 days, 20 to 365; see [instance settings](instance-settings.md)).
  An action past the deadline and a second appeal give `422` (`/api/v1/problems/appeal-window-closed` and `/api/v1/problems/already-appealed`).
- A `ban` can be appealed until the account is deleted (30 days after the action, or until the `appeal_window` deadline if that is shorter). A `ban` of a deleted account gives `422` (`/api/v1/problems/not-appealable`). A lifted or expired action can still be appealed within the deadline.
- When a user appeals, people with `manage_accounts` get a notification (`admin_appeal`). When it is reviewed, the user gets a notification (`appeal_decided`) and can see the outcome and `public_reason`. The reviewer is not shown.
- Overturning lifts the action in the same transaction (the same effect as `lift`, with an empty lift reason). Overturning `delete_statuses` does not bring back the deleted posts. An action already lifted before the review stays as it is.
- Overturning a `ban` also cancels the scheduled account deletion in the same transaction (the audit log records `account_deletion_cancelled`), ends the suspension, and sends the original profile `Update` to other servers. Revoked sessions and tokens do not come back, so the user signs in again.
  If the account was deleted before the review, it cannot be overturned (`422`, `/api/v1/problems/not-appealable`; upholding is still possible).
- A review happens once. Reviewing again with the same outcome changes nothing, and the opposite outcome gives `409` (`/api/v1/problems/appeal-decided`).

## Account management

Searching accounts, viewing their details and revealing IP addresses and email addresses are done through the admin API and need the `manage_accounts` permission.

- `GET /api/v1/admin/accounts` returns local and remote accounts, newest first (the instance Actor is excluded).
  You can filter by username prefix (`username`), domain (`domain`), local or remote (`origin`), state (`state`), and, for local accounts, email address (`email`), email domain (`email_domain`) and the IP address used to register or sign in (`ip`).
  You can give only one of `email`, `email_domain` and `ip`. The list does not include IP addresses or email addresses.
- The state is the first that applies from this list: `deleted`, `suspended` (`suspend`, `ban`), `disabled` (`disable_login`), `rejected` and `pending` (registration review), `limited` (`limit`), `active`.
  The state and the restrictions (`restrictions`) come only from actions against the account itself and do not include domain actions.
- `GET /api/v1/admin/accounts/{id}` returns the state and restrictions and, for a local account, the registration application and review, the email confirmation, the roles, and up to 20 actions and reports, newest first.
- `POST /api/v1/admin/accounts/{id}/reveal` returns a local account's email address, the IP addresses and User-Agents of registration and sign-ins (20 entries, newest first), and other accounts that used the same IP address (up to 20 per IP address).
  Every call records `revealed_private_data` in the audit log, and the data is returned only if the record could be written. IP addresses are removed after the retention period (180 days by default), so sign-in origins older than that cannot be shown.
- `two_factor_enabled` in `GET /api/v1/admin/accounts/{id}` shows whether the user of a local account uses two-factor authentication (TOTP).
  A user who lost both the authenticator app and the recovery codes can sign in with the password alone once you remove two-factor authentication with `POST /api/v1/admin/accounts/{id}/two_factor/disable`.
  Only users with the permission `manage_login_recovery` can do this, and by default only the built-in administrator role has it (moderators can see the state but cannot remove it; it belongs to server administration).
  Removal records `two_factor_disabled` in the audit log. Verify that the person is the account's owner by another route, such as email, before you remove it.
  When an administrator with no other administrator lost it, the operator removes it with the CLI command `my admin account disable-two-factor @alice` (the audit log actor is the CLI).
- On a server that sends mail, a user who forgot the password gets an emailed link for setting a new one from "Forgot your password?" on the sign-in page.
  For a server that does not send mail (`mail = none`), or a user the mail does not reach, an administrator issues the link and hands it over.
  In the admin screens, issue it with "Password reset" on the account page (`POST /api/v1/admin/accounts/{id}/password_reset_link`).
  Only users with the same permission as removing two-factor authentication, `manage_login_recovery`, can issue it, and by default only the built-in administrator role has it (whoever holds the link can get into the account, so a moderator's `manage_accounts` is not enough).
  The issued link is shown once on the spot and can be copied. The server stores only a hash of the link, so you cannot see it later; issue it again if you need to.
  When no administrator is available or the admin screens are not, the operator issues it with the CLI command `my admin account password-reset-link @alice` (the audit log actor is the CLI).
  The link works for 1 hour and once, and stops working if the password changes by another route afterwards. Either way of issuing records `password_reset_link_issued` in the audit log.
  Anyone who holds the link can set the password, so verify the person by another route such as email and give the link only to them.
  Setting the new password revokes all of the user's sessions and app tokens. Passkeys and two-factor authentication stay.
- `email_suppression` in `GET /api/v1/admin/accounts/{id}` shows that sending to the primary address of a local account is stopped.
  A `hard_bounce` (the destination does not exist) stops all mail except the confirmation mail, and a `complaint` (reported as spam) stops only notification and announcement mail. Password-reset mail stops too, so issue the link above and hand it over if needed.
  When the user asks for a confirmation mail and confirms the address again, the hard-bounce suppression is lifted.
  When you have verified that sending is fine, for example because the user fixed the mailbox, lift it with `POST /api/v1/admin/accounts/{id}/email_suppression/lift` (permission `manage_accounts`). It records `email_suppression_lifted` in the audit log.
- A search reads up to 400 accounts to fill one page. When few accounts match, even a short or empty page can carry `next_cursor`, so keep reading.

## Domain actions

| Kind | Effect |
| --- | --- |
| `suspend` | Stops all delivery, fetching and receiving (receiving gets `403` before the signature is verified). Existing accounts and posts are hidden from timelines and profiles. Cuts follows between local users and the domain's accounts in both directions (nothing is sent to the other side) |
| `limit` | Keeps posts off the public timelines and search, and creates no notifications for users who do not follow the account |
| `reject_media` | Does not fetch media |
| `reject_reports` | Drops reports (`Flag`) from the domain |
| `force_sensitive` | Shows media as sensitive |

```sh
my admin domain issue -kind suspend -public-comment "spam" spam.example
my admin domain issue -kind limit -duration 720h noisy.example
my admin domain list -active
my admin domain lift <action id>
```

In the admin API: `POST /api/v1/admin/domain_actions` (the body has `domain`, `kind`, `public_comment`, `private_note`, `obfuscate` and `expires_in` in seconds),
`GET /api/v1/admin/domain_actions` (filter with `domain` and `active`), `GET .../{id}`, and `POST .../{id}/lift`.

- Domains are recorded normalized to the IDNA A-label (lower case, no trailing dot). `例え.jp` and `xn--r8jz45g.jp` are the same domain.
- An action also applies to subdomains (an action on `spam.example` applies to `a.spam.example` too).
- An action is applied to the restriction projection (`domain_restrictions`) and to the timeline restrictions in the transaction that records it, and takes effect from the next delivery, fetch or receipt.
- An action with an end time (`-duration` / `expires_in`, at most ten years) ends by itself at that time. Expiry is not a "lift", so lists show it as `expired`.
- Lifting removes only that action's restrictions. Other actions on the same domain stay. Lifting an already lifted action changes nothing.
- Lists are newest first, and `-active` (`active=true`) shows only the actions in effect.

### Importing and exporting Mastodon CSV

You can pass the list of domain actions to and from Mastodon in the CSV format of Mastodon's domain blocks (`#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate`).
You can import a CSV that Mastodon exported, and Mastodon can import a CSV that this server exported.

```sh
my admin domain export > domain_blocks.csv
my admin domain import domain_blocks.csv                       # only shows what it would do
my admin domain import -apply -only spam.example,bad.example domain_blocks.csv
```

In the admin API: `GET /api/v1/admin/domain_actions/export` (`text/csv`), `POST .../import/preview` (the body has `csv`), and `POST .../import` (the body has `blocks`, the chosen rows, and `private_note`).
In the web client, use "Import a CSV" and "Export CSV" on the Domain actions screen (`/moderation/domain-actions`).

| CSV row | Action on this server |
| --- | --- |
| `#severity` is `suspend` (or empty) | `suspend` |
| `#severity` is `silence` | `limit` |
| `#severity` is `noop` | None |
| `#reject_media` / `#reject_reports` is true | Adds `reject_media` / `reject_reports` respectively |

- An import has two steps. First check in the preview which actions each row would issue, choose the rows to keep, and then import (the CLI changes nothing until you add `-apply`, and `-only` chooses rows by domain).
- A row whose domain or a parent domain already has an action of the same kind in effect does not issue that kind (if every kind is already there, it is "already applied"). A second row for the same domain, an invalid row and a row that would issue nothing (`noop` only) are skipped too. Importing the same CSV again does not add actions.
- A CSV without a `#domain` header is read as a list with one domain per line, and every domain gets `suspend` (the same as Mastodon). One import takes up to 5,000 rows.
- Imported actions last "until lifted" and carry the row's public comment and obfuscation and the private note you gave. They appear in the audit log like actions issued one by one (the actor is the person for the API and `cli` for the CLI).
- An export writes one row per domain for the actions in effect. A domain with a `suspend` gets `suspend`, one with a `limit` gets `silence`, and one with neither gets `noop`. A domain with only `force_sensitive`, which Mastodon's domain blocks do not have, is not exported. Private notes and end times are not exported.

### Severed follows

A domain `suspend` and a remote account `suspend` cut follows between local users and the suspended accounts in both directions.

- For a domain `suspend`, a job (`moderation.sever_domain`) that runs after the action is recorded cuts them one local user at a time. Until the job finishes, the follows still show in lists. Accounts on subdomains are cut too.
- For a remote account `suspend`, the follows are cut in the transaction that records the action. A second `suspend` of an account already suspended cuts nothing.
- Each user who lost approved follows gets one notification (`relationships_severed`) that tells how many followers and follows were lost. A user who lost only follow requests gets none.
- Lifting the action does not bring back the cut follows. Users follow again.
- Processing the same action again (for example when a job re-runs) adds no more severed-relationship records or notifications.

### Publishing the domains you act on

You can publish the `limit` and `suspend` actions in effect through `GET /api/v1/instance/domain_actions`.
The instance setting `domain_actions_visibility` (`none` / `users` / `all`, default `none`) decides who sees the list, and
`domain_actions_rationale_visibility` (the same values, default `none`) decides who sees the public comments ([instance settings](instance-settings.md)).

```sh
my instance set domain_actions_visibility users
my instance set domain_actions_rationale_visibility users
```

- With `none` the API returns `404`, and with `users` it returns `401` to a request without an access token.
- There is one entry per action, newest first. An entry has the domain, the SHA-256 of the domain (hex) and the kind, and the public comment for people allowed to see it. Private notes are never returned.
- For an action with `-obfuscate` (`obfuscate`), the middle half of the domain's characters (other than dots) is replaced with `*` (`bad.example` becomes `bad.*****le`).
  Anyone can compare the SHA-256 of a domain they know to check whether it is an obfuscated one.
- `reject_media`, `reject_reports` and `force_sensitive` actions, and lifted or expired actions, are not published.
- The web client's instance information page (`https://<host>/about`) shows this list under the same settings. With `none` the section is not shown, and with `users` people who are not signed in see a note that only signed-in users can see it.

## Refetching remote posts

You can refetch stored remote posts from the other server.
A refetch happens only when you ask for it through the admin API, the CLI or the web client, never periodically.

```sh
my admin refetch status <post ID or URI>
my admin refetch account @bob@remote.example
my admin refetch domain remote.example
```

In the admin API: `POST /api/v1/admin/statuses/{id}/refetch`, `POST /api/v1/admin/accounts/{id}/refetch_statuses` and `POST /api/v1/admin/instances/{domain}/refetch_statuses`. They need the `manage_domains` permission.
In the web client, use "Refetch posts" on the account screen (`/moderation/accounts/{id}`, remote accounts) and the remote server screen (`/moderation/instances/{domain}`).

- A post is handled like a received `Update`. If there is a newer version, the server adds a version; it updates poll vote counts; and it does nothing if nothing changed. A post for which the other server returns 410 Gone or a `Tombstone` is deleted. A post that cannot be fetched (404 included) is left as it is.
- For an account, the server refetches up to 1,000 of its newest undeleted stored posts, 100 at a time, one minute apart.
- For a domain, the server refetches from the newest accounts of the domain, up to the 20 newest posts per account and up to 1,000 posts in all, 5 accounts at a time, one minute apart.
- It does not fetch from a domain with a `suspend`, or from a domain that is `unavailable` because deliveries failed. A failed fetch of the same URI is remembered for 1 hour, and a refetch in that time does not fetch it.
- Your request is recorded in the audit log as `status_refetch_queued`, `account_statuses_refetch_queued` or `domain_statuses_refetch_queued`.

## Reports

Reports (`Flag`) received from other servers enter the report list as open (`open`).

```sh
my admin report list -state open
my admin report show <report id>
my admin report assign <report id> <username>
my admin report unassign <report id>
my admin report resolve -outcome dismissed <report id>
my admin report note <report id> "Same person as an earlier report"
my admin report reopen <report id>
```

In the admin API: `GET /api/v1/admin/reports` (filter with `state`), `GET .../{id}`, `POST .../{id}/assign_to_self`, `POST .../{id}/unassign`,
`POST .../{id}/resolve` (body `{"outcome": "actioned" | "dismissed"}`), `POST .../{id}/notes` (body `{"body": "..."}`) and `POST .../{id}/reopen`.

- An assignment (`assign`) can go only to a local user whose role has `manage_reports`. In the admin API you assign yourself.
- The outcome of a resolution (`resolve`) is `actioned` (an action was taken against the target) or `dismissed` (no action was needed). Resolving a resolved report again does not change the outcome.
- When you issue an action from a report (`report_id` / `-report` of the action), the report is resolved as `actioned` in the same operation.
- A note (`note`) is an internal note that only moderators see, up to 2,000 characters. It is shown neither to the target nor to the reporter. It cannot be edited or deleted.
- Reopening (`reopen`) puts a resolved report back to `open` (the assignment stays). An open report cannot be reopened (`409` in the admin API).
- The report details (`show`, `GET .../{id}`) return the notes, oldest first, with the number of other reports about the target and the number of actions against the target (lifted and expired ones included). The list does not return notes.
- Assignments, resolutions, reopenings and notes are kept as events (`report_assignments`, `report_resolutions`, `report_reopenings`, `report_notes`), and `report_states` holds the current state.
- A local reporter gets a notification (`report_resolved`) at the first resolution. Reopening and resolving again adds no notification.
  A reporter can list their own reports with `GET /api/v1/reports` and sees only whether each is open or resolved. The outcome, the action taken, the reason given to the target and the assignee are not shown.

### Voice chat rooms

- A local user can report an open voice chat room that they can see (`POST /api/v1/reports` with `voice_room_id`, and `account_id` set to the room's host).
  The report targets the host at that moment and points at the room's settings (name and description) as they were then. It points at no posts and is never forwarded to another server.
- The report details (`GET /api/v1/admin/reports/{id}`) show the room (`voice_room`): every change of its settings, who took part and when they left, who was removed from it, and how it ended.
  The records of who took part are kept for 90 days after the room ends (`retention_voice_participants`) and as long as a report points at the room ([record retention](#record-retention-and-holds)).
  There are no recordings and no text chat, so these records are the only evidence.
- A holder of `manage_reports` can end an open room for every participant with `POST /api/v1/admin/voice_rooms/{id}/end` (body `{"reason": "..."}`, optional, up to 500 characters).
  The reason is only for the moderators; the participants see only that the room ended. It works while the voice chat is off too, and the audit log records `voice_room_ended`.

## Notifications for moderators and the pending counts

New reports, registrations waiting for approval and appeals reach people with the permission as in-app notifications.

| Notification | When it arrives | Who gets it |
| --- | --- | --- |
| `admin_report` | When the report's target has no other open report (a second or later report about the same account does not notify) | People whose role has `manage_reports`, except the reporter and the report's target |
| `admin_sign_up` | When a registration starts as waiting for approval (approval mode, or `open` on a setup that cannot send mail) | People whose role has `manage_accounts` |
| `admin_appeal` | When an appeal is made against an action | People whose role has `manage_accounts`, except the person who appealed |
| `admin_custom_emoji_request` | When creating a custom emoji or replacing its image is requested | People whose role has `manage_custom_emojis`, except the requester |
| `admin_error_group` | When the server or the web client reports an error of a new kind, or an error group that was resolved happens again on another version of the server | People whose role has `manage_errors` |

- `admin_error_group` arrives at most 10 times an hour for the whole server. The rest of the hour fold into one notification
  that says how many more error groups there were. A muted group never notifies. An error that the web client reports for a
  visitor who is not signed in notifies only once a signed-in user's browser reports it too, and its recurrence notifies nobody.
  The error groups are read and resolved, muted, or unmuted through `GET /api/v1/admin/error_groups` and
  `POST /api/v1/admin/error_groups/{id}/resolve` (`/mute`, `/unmute`), which need `manage_errors`.
- Who receives a notification is decided by their permission when the notification is created. A person who later loses the permission no longer sees that kind in their list and unread count. If the permission comes back, they see it again.
- Each kind of notification above also comes by email, on by default. Each person turns each kind on or off in the app and by
  email in their notification settings (`GET` and `PATCH /api/v1/preferences/notifications`), which list only the kinds their
  role receives. A kind turned off in the app is not created for them, so it is neither pushed nor emailed; the counts of
  `GET /api/v1/admin/counts` do not change, because they count the reports, registrations, and appeals themselves.
  The server enqueues the email job in the transaction that records the notification, and the job sends the mail right after the
  commit, without waiting for the notification to be read. If the person has lost the permission or turned that kind's email
  off by the time of sending, or their primary address is not confirmed or its emails are stopped, nothing is sent.
  The mail contains only the kind of notification and a link to the notifications screen, and none of the contents of the report or the account. Its unsubscribe link turns off the email of that kind only.
  With `mail = none`, no mail is sent, and `sends_email` in `GET /api/v1/instance` is `false`.

`GET /api/v1/admin/counts` returns the number of open reports (`open_reports`, needs `manage_reports`), the number of registrations waiting for approval (`pending_registrations`) and the number of appeals waiting for review (`pending_appeals`) (both need `manage_accounts`).
Items you lack the permission for are not returned.

## Audit log

Operations by moderators and administrators are recorded one row each in the audit log (`audit_log_entries`) in the same transaction as the operation.
Failed operations and operations that changed nothing are not recorded: lifting an already lifted action, resolving a resolved report, assigning to the same assignee, retiring an already retired rule, moving to the same position, granting a role the user already has, and revoking a role the user does not have.

| Target kind | Operations |
| --- | --- |
| `account` | `account_action_issued` and `account_action_lifted` (issuing and lifting an action against an account), `account_deletion_cancelled` (the deletion canceled because an appeal against a `ban` was overturned), `revealed_private_data` (IP addresses and email addresses revealed), `two_factor_disabled` (two-factor authentication removed), `password_reset_link_issued` (a password-reset link issued), `email_suppression_lifted` (mail suppression lifted), `account_moved` (a restricted local account moved to a local account; restrictions are not carried to the destination, so you decide whether to apply them there), `account_statuses_refetch_queued` (refetch of a remote account's posts) |
| `status` | `status_refetch_queued` (refetch of a remote post; the target name is the post URI) |
| `domain` | `domain_action_issued` and `domain_action_lifted` (issuing and lifting a domain action), `domain_statuses_refetch_queued` (refetch of posts of the domain's accounts) |
| `report` | `report_assigned`, `report_unassigned`, `report_resolved`, `report_reopened`, `report_note_added` (assigning, unassigning, resolving and reopening a report, and adding a note) |
| `user` | `registration_approved`, `registration_rejected` (registration review) |
| `setting` | `setting_changed` (instance settings), `text_changed` (instance texts) |
| `rule` | `rule_added`, `rule_revised`, `rule_moved`, `rule_retired` (instance rules) |
| `role` | `role_granted`, `role_revoked` (granting and revoking a role for a user), `role_created`, `role_updated`, `role_deleted` (creating, changing and deleting a role) |
| `appeal` | `appeal_upheld`, `appeal_overturned` (appeal review; overturning also records `account_action_lifted` for the action, and `account_deletion_cancelled` for a `ban`) |
| `signup_block` | `signup_block_added`, `signup_block_lifted` (adding and lifting a sign-up block) |
| `custom_emoji` | `custom_emoji_created`, `custom_emoji_image_replaced`, `custom_emoji_copied`, `custom_emoji_updated`, `custom_emoji_enabled`, `custom_emoji_disabled`, `custom_emoji_deleted` (registering a custom emoji, replacing its image, copying an emoji from another server, changing, enabling, disabling and deleting it) |
| `webhook` | `webhook_created`, `webhook_updated`, `webhook_deleted`, `webhook_enabled`, `webhook_disabled`, `webhook_secret_rotated` (creating, changing, deleting, enabling and disabling a webhook, and regenerating its secret) |
| `announcement` | `announcement_created`, `announcement_updated`, `announcement_published`, `announcement_withdrawn` (creating, changing, publishing and withdrawing an announcement) |
| `invite` | `invite_created`, `invite_revoked` (creating and revoking an invite; the target name is the first 4 characters of the code) |
| `billing_source` | `billing_import_queued` (a billing import was started by hand; the target name is the provider's name) |
| `cost_item` | `cost_item_created`, `cost_item_relabeled`, `cost_item_price_revised`, `cost_item_end_changed`, `cost_item_canceled` (registering a cost, changing its name and the like, revising its amount, changing its end date, and canceling it; the target name is the item's name) |
| `supporter_program` | `supporter_program_enabled`, `supporter_program_disabled` (enabling and disabling billing; the target name is the payment provider's name) |
| `supporter_plan` | `supporter_plan_created`, `supporter_plan_revised`, `supporter_plan_retired` (creating, changing and retiring a supporter plan; the target name is the plan's name) |
| `supporter_subscription` | `supporter_subscription_canceled` (an administrator canceled a subscription; the target name is the plan's name and the user's handle) |
| `custom_emoji_request` | `custom_emoji_request_submitted`, `custom_emoji_request_withdrawn`, `custom_emoji_request_approved`, `custom_emoji_request_rejected` (submitting, withdrawing, approving and rejecting a custom emoji request; the target name is the `:shortcode:`) |
| `payment_provider` | `payment_reconcile_queued` (reconciliation with the payment provider was started by hand; the target name is the provider's name) |
| `oauth_client` | `oauth_client_disabled`, `oauth_client_enabled` (disabling and revoking an OAuth client; disabling revokes all its tokens; the target name is the client's name and `client_id`) |
| `oauth_client_host` | `oauth_client_host_denied`, `oauth_client_host_allowed` (denying and revoking the denial of a Client ID Metadata Document host; denying revokes the tokens of that host's clients; the target name is the host) |
| `instance` | `service_closure_announced`, `service_closure_withdrawn`, `service_closure_closed`, `service_closure_delivery_completed` (announcing the closure of the service or changing the date, withdrawing the announcement, closing, and completing the delivery of the closure; the target name is `service_closure`) |
| `voice_room` | `voice_room_ended` (a moderator ended an open voice chat room; the target name is the room ID, since the name goes when the account that wrote it is erased; the report details show the name) |
| `login_provider` | `login_provider_created`, `login_provider_updated`, `login_provider_deleted`, `login_provider_enabled`, `login_provider_disabled`, `login_provider_secret_rotated` (adding, changing, deleting, enabling and disabling an external login provider, and replacing its client secret; the target name is the provider's display name) |

The list shows whether the target still exists. Accounts, reports, appeals and sign-up blocks that disappeared after their retention period, and deleted roles, custom emoji and webhooks, show only the name at the time of recording and no link to a screen.

```sh
my admin audit list
my admin audit list -actor alice -action domain_action_issued
my admin audit list -target-kind domain -target-id spam.example
```

In the admin API: `GET /api/v1/admin/audit_log` (filter with `actor_account_id`, `action`, `target_kind` and `target_id`). It needs the `view_audit` permission.

- A row is the actor (the user for `user`, or `cli` for the CLI), the operation, the target kind and ID, the target's display name, the ID of the original record, and the time.
  The target ID is the ID of the account, report, appeal, user, rule, role or sign-up block; for a domain action it is the domain; for settings and texts it is the key.
- The target's display name (`@alice`, `@bob@example.com`, a domain, a setting key, the text of a rule; for a text its key, for a role its key and the user's handle, for a sign-up block its kind and value) is a copy taken at recording time, so it is still readable after the target is gone.
  Each API row also returns whether the target still exists (`target_exists`), which is `false` for accounts, reports, appeals and sign-up blocks that are gone. The web audit log shows only the display name, without a link, for a target that is gone.
- The text of reasons and notes is not copied. Look it up through the ID of the original record (the action, assignment, resolution, reopening, note, review, or the version of a setting or rule).
- The list is newest first with keyset paging (`-before <entry id>` in the CLI).
- There is no API or CLI to update or delete the audit log. Old rows are removed after the [retention period](#record-retention-and-holds).

## Record retention and holds

After the retention period for each kind of record, hourly jobs (`moderation.purge_records`, `identity.purge_origins`, and `voice.purge_records`) remove moderation records.
You can change the periods in the instance settings, but not below the lower limit ([instance settings](instance-settings.md)). A changed period takes effect from the next run.

| Record | Setting | Default | Lower limit | Starts from |
| --- | --- | --- | --- | --- |
| Sign-in IP address and User-Agent | `retention_session_origins` | 180 days | 90 days | The sign-in |
| Registration IP address | `retention_registration_origins` | 180 days | 90 days | The registration |
| Reports (comment, target posts, notes, assignments, resolutions, reopenings) | `retention_reports` | 365 days | 180 days | The last resolution |
| Actions against accounts (reason, note, target posts, lifting) and appeals | `retention_moderation_actions` | 365 days | 180 days | The end of the action |
| Audit log | `retention_audit_log` | 730 days | 365 days | The record |
| Sign-up blocks | None (cannot be changed) | 30 days | n/a | The earlier of lifting and expiry |
| Who took part in a voice chat room (participations, speakers and listeners, raised hands, invitations, removals) | `retention_voice_participants` | 90 days | 30 days | The end of the room |

- Open reports are never removed. A report that an action refers to (`report_id` of the action) stays until that action is removed.
- The end of an action is the earlier of lifting and expiry. For an action that was appealed, if the review comes later than that, the end is the time of the review, and the action is kept while the appeal waits for review.
  `warn` and `delete_statuses` have no lasting effect, so the end is the time they were issued. An action without an end (a `ban`, a `suspend` without an end time, and so on) is not removed until it is lifted.
- Posts that a report or an action points to are not physically deleted by the purge job, with their attached media, until that report or action is removed, even if the author deleted them (they disappear from view). Remote posts also stay beyond the retention period (including the setting `remote_status_max_retention`).
- A sign-up block that has neither a lift nor an end time stays as long as it is in effect.
- The records of who took part in a voice chat room that a report points at stay until that report is removed. The room itself (its name, description, hosts, and how it ended) is never removed.
- One job run removes only a capped number of rows per kind (100 at a time, 20 times, for actions, reports and sign-up blocks; 1,000 rows at a time, 20 times, for the audit log). The next run removes the rest.

### Holds

While a court order forbids erasure (Japan's Information Distribution Platform Act, Article 16) or a disclosure request is being handled, you can stop erasure per account.

```sh
my admin retention hold -reason "court order 2026-123" @alice
my admin retention list -account @alice
my admin retention release <hold id>
```

- For an account under a hold, its actions and appeals, the reports about it and the reports it made, the audit logs of those, the sign-in and registration IP addresses, its posts, and the records of the voice chat rooms that it took part in or acted in are not removed after the retention period.
  A sign-up block created from that account's ban is also kept past 30 days after lifting or expiry.
- You can place several holds on one account. When you release them all, records past their period are removed on the next run. Releasing an already released hold changes nothing.
- Holds and their release are done only with the CLI, and the actor in the record is the CLI.
- The data of a deleted account (one whose `ban` reached its date, or one the user deleted) is erased 7 days after deletion by the hourly job `account.erase_deleted`.
  An account under a hold is not erased. Posts that a report or action points to stay until that record is removed. The account's URI and username and the action records remain.
  The names and descriptions that the account gave its voice chat rooms are emptied; the rooms stay.

### Exporting for disclosure requests

For a disclosure request (GDPR Article 15, Japan's Act on the Protection of Personal Information Article 33), respond by exporting all of an account's moderation records as JSON.

```sh
my admin account export-moderation @alice > alice-moderation.json
```

- The export contains the actions against the account (including private notes, lifting, appeals and reviews), the reports about the account (comment, target posts, rules cited, moderators' notes, assignments, resolutions, reopenings), the reports the account made (only whether each is open or resolved), and the audit logs of those.
- The identity of reporters (the reporting account, and the URI and the origin server of a received report) is not exported. Before you hand the export over, check that a report's comment does not say anything that identifies the reporter.
- A moderator is exported by user ID. Place a hold first so that records are not removed while you handle the request.
