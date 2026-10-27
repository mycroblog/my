# Administrators and roles

A guide for operators: how to create the first administrator, manage roles from the command line, announce and carry out the end of the service, and export the maintenance page.

## Prerequisites

- The configuration (`host`, `db`, `database_url`, `kek`, and so on) is set as the [configuration reference](../reference/configuration.md) describes. `my config check` verifies it.
- The schema is applied to the database (apply `schema/<dialect>/` with Atlas; see [database.md](database.md)).

## Creating an administrator

```sh
my admin create -username alice -email alice@example.com
```

- From a terminal, the command asks for the password twice, without echoing it.
- When standard input is not a terminal, the first line of standard input is the password (for example `my admin create ... < password.txt`).
  The password cannot be given as a command-line argument or an environment variable, because those end up in process lists and shell history.
- A password has 8 to 128 characters. It never appears in the output, the logs, or an error message.
- One transaction creates the user, the password, the primary email address, the local account (with its keys), and the grant of the `admin` role (with no grantor, which means the CLI).
  The built-in roles are created first if they are missing.
- On success the command prints `created admin @alice@<account_domain> (user <id>, account <id>)` and exits with code 0.

## Running it again

- When the username or the email address (both case-insensitive) is already taken, the command changes nothing, prints which one is taken, and exits with code 1.
  It never changes the password or the roles of the existing user.
- A failed run writes nothing, so you can fix the cause and run the same command again.
- Exit code 2 means a usage error.

## Granting and revoking roles

Roles hold permissions. The built-in roles are `admin` (`position` 1000, every permission) and
`moderator` (`position` 100, with `manage_reports`, `manage_accounts`, and `view_audit`).
`my serve` checks the built-in roles every time it starts (in a process of any `role`), and `my admin role` checks them every time it runs.
Each creates what is missing and gives `admin` every permission of the running version.
When an upgrade adds a permission, no manual step is needed. A `moderator` keeps the permissions and the position the administrator set.

```sh
my admin role list                 # each role's position, permissions (grouped as moderation, server administration, and member features), and holders
my admin role list -user alice     # the roles of alice
my admin role grant alice moderator
my admin role revoke alice moderator
```

- Permissions belong to one of three groups.
  - Moderation: `manage_reports`, `manage_accounts`, `manage_domains`, `manage_invites`, `view_audit`.
  - Server administration: `manage_settings`, `manage_rules`, `manage_roles`, `manage_jobs`, `manage_login_recovery`, `manage_custom_emojis`, `manage_webhooks`, `manage_announcements`, `manage_finance`, `manage_supporters`, `manage_oauth_apps`, `manage_service_closure`, `manage_errors`.
  - Member features: `view_finance`, `create_custom_emojis`, `start_voice_rooms`.

  `list` prints one column per group (`MODERATION`, `SERVER`, `MEMBER`). The web client shows "Moderation" and "Administration" for the first two groups and counts the member features toward neither (see [moderation.md](moderation.md)).
- A change takes effect on the user's next request. Notifications for moderators go to the people who hold the permission when the notification is created.
- Grants and revocations are written to the audit log as `role_granted` and `role_revoked` (target kind `role`). The actor of a CLI operation is `cli` (`my admin audit list -target-kind role`).
- Granting a role the user already has, or revoking one they do not have, changes and records nothing.
- The last holder of `admin` cannot lose it (exit code 1). Grant `admin` to another user first.
- The admin API is `GET /api/v1/admin/roles`, `GET /api/v1/admin/accounts/{id}/roles`, and `PUT` / `DELETE /api/v1/admin/accounts/{id}/roles/{role}`, and it needs the permission `manage_roles`.
  Through the API, you cannot grant or revoke a role above the highest `position` among your own roles (403). Revoking the last `admin` returns 409.

## Announcing the end of the service

To close the server, announce the closure first. [service-closure.md](service-closure.md) describes the whole procedure, from the announcement to discarding the data.
The server keeps working as usual during the announcement. `closure` in `GET /api/v1/instance` and `metadata.serviceClosure` in NodeInfo show the state and the dates.

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for using this server."
my admin closure announce -closes-at 2027-02-28 -export-until 2027-04-30   # change the dates (a bare date means 00:00 UTC)
my admin closure status                                                   # the state and the schedule in effect
my admin closure withdraw                                                 # withdraw the announcement
```

- `-closes-at` has no lower bound. `-export-until` (the deadline for taking data out) must be on or after `-closes-at`, and defaults to 30 days after the closing date.
- When the closing date is less than 30 days away, the command prints a warning to standard error (the announcement is still recorded).
- Changing the dates is recorded as a new announcement, and an announcement made after a withdrawal is a new one. Once the server is closed, you can neither announce nor withdraw (exit code 1).
- Announcing, changing the dates, and withdrawing are written to the audit log as `service_closure_announced` and `service_closure_withdrawn` (target kind `instance`; see `my admin audit list -target-kind instance`).
- The admin API is `GET /api/v1/admin/closure`, `PUT` / `DELETE /api/v1/admin/closure/announcement`, `GET /api/v1/admin/closure/check`, and `POST /api/v1/admin/closure/close`. It needs the permission `manage_service_closure` (among the built-in roles, only `admin` has it).
- During the announcement, the server accepts no new users and no new payments.
  - The effective registration mode is `closed` (the value of `registration_mode` does not change, and returns to effect when the announcement is withdrawn). Neither sign-ups nor sign-ups with an invitation work, and pending applications cannot be approved (`409`).
  - Announcing rejects every pending application (the decider is recorded as `service_closure`) and cancels all running supporter subscriptions at the end of their period.
    The subscribers get a notification (`supporter_service_closure`) and an email. New subscriptions return `409`. Withdrawing the announcement does not undo the rejections or the cancellations.
  - Every announcement, change of dates, and withdrawal sends the notification `service_closure` to all local users. All of this runs as jobs, so do not stop the worker.
  - Every user with a confirmed email address gets an email for the announcement, a change of dates, a withdrawal, 7 days and 1 day before the closing date, the closure, and 7 days before the export deadline. Users cannot opt out (see [email.md](email.md)).
  - After the closing date, the people who hold `manage_service_closure` get the notification `admin_service_closure_due`. The server does not close itself.

After the closing date, close the server. Closing cannot be undone.

```sh
my admin closure check                                      # review what the closure ends
my admin closure close                                      # show the review, then ask for y and the domain name
my admin closure close -confirm-domain social.example       # for automation: pass the domain name instead of answering
```

- `check` and `close` show the number of local accounts, the number of destination inboxes (the known shared inboxes), an estimate of the `Delete` activities to send (their product), the number of running supporter subscriptions,
  the number of unfinished migrations (`Move`), and the number of accounts under a retention hold (`retention_holds`).
  When migrations remain or a hold exists, the command warns on standard error. Export the records of the accounts under a hold with `my admin account export-moderation` and keep them before discarding the data.
- `close` refuses without asking when there is no announcement and before the closing date (exit code 1). Any answer other than `y`, or an input that differs from the server's domain (`host` in the configuration, case-insensitive), changes nothing and exits with code 1.
- The closure is written to the audit log as `service_closure_closed`. In the same transaction it ends every running supporter subscription (reason `service_closed`) and enqueues the jobs that cancel the subscriptions at the payment provider immediately (`CancelNow`).
- After the closure, the client API passes only these requests: reading your own account and settings, the exports (`/exports`, `/csv_exports`), sessions (`/authorized_apps`), `GET /instance`, and the administration of the closure and the job queue.
  Everything else is refused with `403` (`/api/v1/problems/service-closed`). Streams stop and Web Push sends nothing. Signing in and resetting a password still work.
- After the closure, the worker delivers a `Delete` of the actor of every local account to the federation peers, 100 at a time (migrated accounts last). It sends them even when `federation_mode` is `off` or `inbound`, and never to a suspended domain.
  `my admin closure status` (and `progress` in `GET /api/v1/admin/closure`) shows how many accounts have a recorded `Delete`, how many deliveries remain to be expanded, and the number of deliveries in each state (`pending`, `succeeded`, `permanent_failure`, `skipped`, `dead`). Keep the worker running during delivery.
  When every delivery has finished, a check that runs every 10 minutes records `delivery_completed` (`service_closure_delivery_completed` in the audit log), and the local actors then answer `410`.
  The server does not rerun `dead` deliveries by itself; rerun them with `my job retry`.

After the export deadline has passed, write the static closing page that your domain serves once the server is stopped. The command reads the database, so run it before discarding the database.

```sh
my admin closure export-html -out ./closure              # every locale (the instance's default locale first)
my admin closure export-html -out ./closure -lang ja,en  # choose the locales and the order in index.html
```

- Output: `<dir>/<locale>.html` for each locale and `<dir>/index.html` with every locale one after another. The pages use the same template as `my maintenance export` and show the server name, the closing date, the operator's message of the announcement, and `status_page_url`.
  The output also holds configuration examples for the front end: `nginx.conf`, `Caddyfile`, and `cloudfront-function.js`. They answer `410` for `/ap/*`, `/inbox`, WebFinger, host-meta, NodeInfo, and `/@*`, and serve the closing page for everything else.
- The command refuses (exit code 1) unless the server is closed. Before `delivery_completed` it prints a warning and still writes the pages. [service-closure.md](service-closure.md) describes how to switch the front end to the static page.

## Exporting the maintenance page

`my maintenance export` writes the page of the maintenance mode as static HTML,
for a load balancer, CDN, or reverse proxy to serve while `my serve` itself cannot run (an image being replaced, a revision that does not start).
It uses the same template as the maintenance mode, so both show the same words and look.

```sh
my maintenance export -out ./maintenance -lang en,ja            # reads the database for at most 5 seconds
my maintenance export -out ./maintenance -lang en,ja -offline   # the configuration and the default theme only
```

- Output: `<dir>/<locale>.html` for each locale of `-lang`, for front ends that choose a page by `Accept-Language`,
  and `<dir>/index.html` with every locale one after another, for those that cannot.
  The pages have inline CSS, no script, and no external resource; their Content-Security-Policy is in a `<meta>` element.
- Words: the settings `maintenance_server_name`, `maintenance_message`, `maintenance_until`, and `status_page_url`
  ([configuration reference](../reference/configuration.md)).
- Without `-offline`, it also reads the instance's name (when `maintenance_server_name` is unset), its default locale (when `-lang` is not given),
  and the token overrides of its theme. The theme's custom CSS is never included. When the database cannot be read,
  it prints a warning, writes the pages from the configuration and the default theme, and exits 0.
- Run it again whenever those settings or the theme change, and before each maintenance, so that the pages in the front end stay current.
