<!-- Code generated from the commands of cmd/my by "scripts/task.sh go:cli-docs". DO NOT EDIT. -->

# Command reference

`my` is the single binary of the server. `my serve` runs the server; the other commands set up, operate,
and upgrade an instance from the command line, against the same database and storage as the server.
Every command prints its help with `-help`. This page lists each command of the standard build with its usage,
its flags and their defaults, and its help.

Most commands read the configuration as the server does: from the environment, and from the TOML file named by
`-config` or `MY_CONFIG` ([configuration reference](configuration.md)). A command exits with 2 when its command
line is wrong, without changing anything.

A binary built with build tags that leave adapters out has the same commands; the ones that need what a tag left out
are marked below and fail with a message that names the tag. A test build adds `my dev push-service`
([build tags](build-tags.md)).

## `my`

Subcommands: [`admin`](#my-admin), [`config`](#my-config), [`db`](#my-db), [`instance`](#my-instance), [`invite`](#my-invite), [`job`](#my-job), [`kek`](#my-kek), [`login-provider`](#my-login-provider), [`maintenance`](#my-maintenance), [`plugin`](#my-plugin), [`projection`](#my-projection), [`registration`](#my-registration), [`schema`](#my-schema), [`search`](#my-search), [`serve`](#my-serve), [`version`](#my-version).

```text
Usage: my <command>

Commands:
  admin     Create an administrator, moderate domains, handle reports,
            manage roles, and block sign-ups
            (my admin create|account|appeal|audit|closure|domain|emoji|
            federation|media|refetch|relay|report|retention|role|signup-block|
            supporters; my admin -help)
  config    Check the configuration and generate the Web Push key
            (my config check|vapid-key)
  db        Run the data upgrade steps of a release upgrade, check before
            applying its schema, or back up a SQLite database
            (my db upgrade [-check]|backup)
  instance  Read and change the instance settings and rules
            (my instance list|get|set|history|rule|theme|brand)
  invite    Manage invite codes (my invite create|list|revoke)
  job       List, retry, and delete the dead jobs (my job list|retry|delete|types)
  kek       Count the stored secrets by key-encryption key and re-wrap them
            after a rotation (my kek status|rewrap)
  login-provider
            Set up the external providers that users sign in with
            (my login-provider list|create|set|rotate-secret|enable|disable|delete)
  maintenance
            Write the maintenance page as static HTML (my maintenance export)
  plugin    Test a plugin against the contract of its kind (my plugin test)
  projection
            Rebuild and verify the projections
            (my projection list|rebuild|verify)
  registration
            Review sign-ups (my registration list|approve|reject)
  schema    Apply this release's schema to the database (my schema apply)
  search    Rebuild, clear, and check the index of the full-text search of posts
            (my search rebuild|clear|status)
  serve     Run the server (role web, worker, or all)
  version   Print the version
```

## `my admin`

Subcommands: [`account`](#my-admin-account), [`appeal`](#my-admin-appeal), [`audit`](#my-admin-audit), [`closure`](#my-admin-closure), [`create`](#my-admin-create), [`domain`](#my-admin-domain), [`emoji`](#my-admin-emoji), [`federation`](#my-admin-federation), [`media`](#my-admin-media), [`refetch`](#my-admin-refetch), [`relay`](#my-admin-relay), [`report`](#my-admin-report), [`retention`](#my-admin-retention), [`role`](#my-admin-role), [`signup-block`](#my-admin-signup-block), [`supporters`](#my-admin-supporters).

```text
Usage: my admin create -username <name> -email <address> [-password-link] [-config <file>]
       my admin account action list|issue|lift ...   (my admin account action -help)
       my admin account export-moderation <account>   (my admin account export-moderation -help)
       my admin account disable-two-factor <account>   (my admin account disable-two-factor -help)
       my admin account password-reset-link <account>   (my admin account password-reset-link -help)
       my admin appeal list|uphold|overturn ...   (my admin appeal -help)
       my admin closure announce|withdraw|status|check|close|export-html ...   (my admin closure -help)
       my admin domain list|issue|lift|export|import ...   (my admin domain -help)
       my admin emoji create|list|update|replace-image|enable|disable|copy|delete|requests ...   (my admin emoji -help)
       my admin federation sever ...   (my admin federation -help)
       my admin media reprocess ...   (my admin media -help)
       my admin refetch status|account|domain ...   (my admin refetch -help)
       my admin relay create|follow|unfollow|delete|list ...   (my admin relay -help)
       my admin report list|show|assign|unassign|resolve|note|reopen|forward ...   (my admin report -help)
       my admin audit list ...   (my admin audit -help)
       my admin retention hold|release|list ...   (my admin retention -help)
       my admin role list|grant|revoke ...   (my admin role -help)
       my admin signup-block create|import|list|lift ...   (my admin signup-block -help)
       my admin supporters status|enable|disable|plans|subscriptions|cancel ...   (my admin supporters -help)

my admin account, my admin domain, my admin report, and my admin appeal moderate
accounts and remote domains and handle reports and appeals.
my admin audit lists the audit log of the moderators' and administrators' operations.
my admin retention holds an account's records past their retention, and my admin
account export-moderation writes them all for a request of access.
my admin account disable-two-factor removes the two-factor authentication of a
user who lost their authenticator app and recovery codes.
my admin account password-reset-link prints a link that sets a new password for
a user who forgot theirs.
my admin role grants and revokes the roles of local users.
my admin closure announces the closure of the service, changes its dates,
withdraws it, checks the server before the closure, closes it, and writes the
static closure page for the domain to serve after the server stops.
my admin emoji registers and manages the custom emoji.
my admin federation severs the follows with remote accounts.
my admin media reprocess queues the processing of uploaded images again with the
current settings.
my admin refetch queues the refetch of remote posts from their servers.
my admin relay joins and leaves the relays that pass public posts between servers.
my admin signup-block blocks sign-ups by email domain, email address, or IP range.
my admin supporters enables and disables the supporter program, lists its plans
and subscriptions, and cancels a subscription.

my admin create creates an administrator: a user with the password, the email address as primary
address, a local account with the username, and the built-in admin role, in one
transaction. The database schema must already be applied with Atlas.

The password is prompted for twice without echo when standard input is a
terminal; otherwise the first line of standard input is the password. It is
never printed or logged.

If the username or the email address is already in use, the command fails with
exit code 1 and changes nothing; it never modifies an existing user. A failed
run writes nothing, so it is safe to run again.
```

### `my admin account`

Subcommands: [`action`](#my-admin-account-action), [`disable-two-factor`](#my-admin-account-disable-two-factor), [`export-moderation`](#my-admin-account-export-moderation), [`password-reset-link`](#my-admin-account-password-reset-link).

```text
Usage:
  my admin account action list [-active] [-before <id>] [-limit <n>] [-config <file>] <account>
  my admin account action issue -kind <kind> [-category <category>] [-public-reason <text>]
                                [-private-note <text>] [-report <report id>] [-no-notify]
                                [-duration <d>] [-status <status id>]... [-block-email]
                                [-config <file>] <account>
  my admin account action lift [-reason <text>] [-config <file>] <action id>

Moderation actions on accounts, in effect at once. <account> is
@username for a local account, @username@domain for a remote one, or an account
ID. Several actions on one account add up. On a local account:

  warn                  only tells the account
  force_sensitive       its media are shown as sensitive
  limit                 its posts, past ones included, are followers-only and
                        stay off the public timelines
  restrict_posting      it cannot post, reply, boost, or edit
  restrict_interaction  it cannot follow, favourite, boost, or reply
  disable_login         its sessions and tokens end and it cannot log in
  suspend               all of the above, and its profile and posts are hidden
  ban                   a suspension that is never lifted; the account is
                        deleted 30 days later
  delete_statuses       deletes the statuses given with -status, for everyone
  mark_statuses_sensitive
                        shows the media of the statuses given with -status as
                        sensitive

delete_statuses and mark_statuses_sensitive need one -status or more (up to
100), and take no -duration; nor does ban. A remote account takes limit,
suspend, and force_sensitive, with the same effects here; suspend also drops
what the account sends. An action on a remote account is not sent to its
server, and -no-notify does not apply to it. -block-email, only with a ban of a
local account, also rejects sign-ups with the email address of the account's
user (a sign-up block of kind email_hash; see my admin signup-block -help).

list shows the actions on the account newest first, lifted and expired ones
included unless -active is given. lift ends one action; the other actions on
the account stay, and so do the statuses that delete_statuses deleted. Lifting
a lifted action changes nothing; a ban is never lifted.
```

### `my admin account action`

Subcommands: [`issue`](#my-admin-account-action-issue), [`lift`](#my-admin-account-action-lift), [`list`](#my-admin-account-action-list).

Its help is the help of [`my admin account`](#my-admin-account).

### `my admin account action issue`

```text
my admin account action issue -kind <kind> [-category <category>] [-public-reason <text>]
                              [-private-note <text>] [-report <report id>] [-no-notify]
                              [-duration <d>] [-status <status id>]... [-block-email]
                              [-config <file>] <account>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-block-email` | `false` | with a ban, also block sign-ups with the email address of the account's user |
| `-category <string>` | `other` | spam, legal, violation, or other |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-duration <duration>` | `0s` | how long the action lasts, such as 720h; 0 lasts until lifted |
| `-kind <string>` |  | the kind of action (required; see below) |
| `-no-notify` | `false` | do not tell the account of the action |
| `-private-note <string>` |  | a note for the moderators |
| `-public-reason <string>` |  | the reason shown to the account |
| `-report <string>` |  | the ID of the report of the account that the action follows from; an open report is resolved as actioned |
| `-status <value>` |  | the ID of a status of the account that delete_statuses or mark_statuses_sensitive acts on; repeat it for several |

Its help is the help of [`my admin account action`](#my-admin-account-action).

### `my admin account action lift`

```text
my admin account action lift [-reason <text>] [-config <file>] <action id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-reason <string>` |  | why the action is lifted |

Its help is the help of [`my admin account action`](#my-admin-account-action).

### `my admin account action list`

```text
my admin account action list [-active] [-before <id>] [-limit <n>] [-config <file>] <account>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-active` | `false` | show only the actions in effect now |
| `-before <string>` |  | show the actions older than this action ID |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-limit <int>` | `50` | number of actions to show, newest first |

Its help is the help of [`my admin account action`](#my-admin-account-action).

### `my admin account disable-two-factor`

```text
my admin account disable-two-factor [-config <file>] <account>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

```text
Usage: my admin account disable-two-factor [-config <file>] <account>

Removes the two-factor authentication (TOTP) of a local account's user, who
then signs in with the password alone, for a user who lost both the
authenticator app and the recovery codes. Make sure the request comes from the
account's owner first. The audit log records it as two_factor_disabled.
<account> is @username or an account ID.
```

### `my admin account export-moderation`

```text
my admin account export-moderation [-config <file>] <account>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

```text
Usage: my admin account export-moderation [-config <file>] <account>

Writes every moderation record about the account to standard output as JSON,
to answer a request of access (GDPR art. 15, APPI art. 33): the actions on it
with their private notes, lifts, and appeals; the reports of it with the
moderators' notes, assignments, and resolutions; the reports it filed; and the
audit log entries on it and on its reports and appeals. The reporters of the
reports of the account, and the servers they came from, are left out.
<account> is @username for a local account, @username@domain for a remote one,
or an account ID.
```

### `my admin account password-reset-link`

```text
my admin account password-reset-link [-config <file>] <account>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

```text
Usage: my admin account password-reset-link [-config <file>] <account>

Prints a link that sets a new password for a local account's user who forgot
it, such as on a server that sends no email. The link works once, for one hour,
and stops working when the password changes in another way. Setting the new
password signs the user out everywhere; passkeys and two-factor authentication
stay. Hand the link to the account's owner only, after making sure the request
comes from them. The audit log records it as password_reset_link_issued.

While sign-in is limited to login providers (the instance setting
login_providers_only), the link does not work until the limit ends; the command
still prints it and warns about it.
<account> is @username or an account ID.
```

### `my admin appeal`

Subcommands: [`list`](#my-admin-appeal-list), [`overturn`](#my-admin-appeal-overturn), [`uphold`](#my-admin-appeal-uphold).

```text
Usage:
  my admin appeal list [-state pending|upheld|overturned] [-before <id>] [-limit <n>] [-config <file>]
  my admin appeal uphold [-reason <text>] [-config <file>] <appeal id>
  my admin appeal overturn [-reason <text>] [-config <file>] <appeal id>

The appeals that local users made of the moderation actions on their accounts.
list shows them newest first. uphold keeps the action; overturn lifts
it at once (the posts that delete_statuses deleted stay deleted); overturning a
ban, appealed within the 30 days before the account's deletion, also cancels
that deletion, and fails once the account is deleted. -reason is
shown to the user, who is notified of the decision. Deciding a decided appeal
the same way changes nothing; deciding it the other way fails.
```

### `my admin appeal list`

```text
my admin appeal list [-state pending|upheld|overturned] [-before <id>] [-limit <n>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-before <string>` |  | show the appeals older than this appeal ID |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-limit <int>` | `50` | number of appeals to show, newest first |
| `-state <string>` |  | show only the appeals in this state: pending, upheld, or overturned |

Its help is the help of [`my admin appeal`](#my-admin-appeal).

### `my admin appeal overturn`

```text
my admin appeal overturn [-reason <text>] [-config <file>] <appeal id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-reason <string>` |  | the explanation of the decision shown to the user who appealed |

Its help is the help of [`my admin appeal`](#my-admin-appeal).

### `my admin appeal uphold`

```text
my admin appeal uphold [-reason <text>] [-config <file>] <appeal id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-reason <string>` |  | the explanation of the decision shown to the user who appealed |

Its help is the help of [`my admin appeal`](#my-admin-appeal).

### `my admin audit`

Subcommands: [`list`](#my-admin-audit-list).

```text
Usage:
  my admin audit list [-actor <username>] [-action <action>] [-target-kind <kind>]
                      [-target-id <id>] [-before <id>] [-limit <n>] [-config <file>]

The audit log: the operations of the moderators and administrators,
newest first, with who performed them (a user, or cli for this command line),
the target, and the ID of the operation's own record, which holds its reasons
and notes. The target's name is the one it had at the time, so entries stay
readable after the target is gone. The audit log cannot be changed or deleted.

Actions: account_action_issued, account_action_lifted,
account_deletion_canceled, revealed_private_data, two_factor_disabled,
password_reset_link_issued, email_suppression_lifted, account_moved (target kind account); domain_action_issued, domain_action_lifted, instance_marked_available (domain);
report_assigned, report_unassigned, report_resolved, report_reopened,
report_note_added (report); registration_approved, registration_rejected
(user); setting_changed, text_changed (setting); rule_added, rule_revised,
rule_moved, rule_retired (rule); link_card_domain_fetch_stopped,
link_card_domain_images_stopped, link_card_domain_rule_removed (link_card_domain); role_granted, role_revoked, role_created,
role_updated, role_deleted (role); appeal_upheld, appeal_overturned (appeal);
signup_block_added, signup_block_lifted (signup_block);
hook_decision_applied, hook_decision_overridden, hook_appeal_upheld (hook_decision);
custom_emoji_created, custom_emoji_image_replaced, custom_emoji_copied,
custom_emoji_updated, custom_emoji_enabled, custom_emoji_disabled,
custom_emoji_deleted (custom_emoji); webhook_created, webhook_updated, webhook_deleted,
webhook_enabled, webhook_disabled, webhook_secret_rotated (webhook);
announcement_created, announcement_updated, announcement_published,
announcement_withdrawn (announcement); invite_created, invite_revoked (invite).
```

### `my admin audit list`

```text
my admin audit list [-actor <username>] [-action <action>] [-target-kind <kind>]
                    [-target-id <id>] [-before <id>] [-limit <n>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-action <string>` |  | show only this kind of operation, such as domain_action_issued |
| `-actor <string>` |  | show only the operations of the local user with this username |
| `-before <string>` |  | show the entries older than this entry ID |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-limit <int>` | `50` | number of entries to show, newest first |
| `-target-id <string>` |  | show only the operations on this target (needs -target-kind) |
| `-target-kind <string>` |  | show only the operations on this kind of target, such as domain |

Its help is the help of [`my admin audit`](#my-admin-audit).

### `my admin closure`

Subcommands: [`announce`](#my-admin-closure-announce), [`check`](#my-admin-closure-check), [`close`](#my-admin-closure-close), [`export-html`](#my-admin-closure-export-html), [`status`](#my-admin-closure-status), [`withdraw`](#my-admin-closure-withdraw).

```text
Usage:
  my admin closure announce -closes-at <time> [-export-until <time>] [-message <text>] [-config <file>]
  my admin closure withdraw [-config <file>]
  my admin closure status [-config <file>]
  my admin closure check [-config <file>]
  my admin closure close [-confirm-domain <domain>] [-config <file>]
  my admin closure export-html -out <dir> [-lang en,ja] [-config <file>]

The closure of the service.

announce announces that the server closes at -closes-at, or changes the dates of
the announcement in effect. The users may export their data until -export-until
(30 days after the closing date by default; not before it). Times are RFC 3339
(2027-01-31T15:00:00Z) or dates (2027-01-31, midnight UTC). The closing date has
no lower bound, but an announcement that gives the users less than 30 days is
warned about on standard error. withdraw withdraws the announcement: the server
runs on as before. status prints the state and the schedule in effect, and once
the server is closed, the progress of the closure's Deletes: the accounts whose
Delete is recorded, the fan-outs left, and the deliveries by state. The
deliveries need the workers running; delivery_completed is recorded when they
have all ended.

check prints what the closure would end: the local accounts and the inboxes
that their Delete goes to, the supporter subscriptions that it cancels, the
account migrations that have not ended, and the accounts under a retention hold.

close closes the server for good once the closing date has passed: it cannot be
undone. It prints the checks, then asks "y" and the domain of the server on
standard input; -confirm-domain gives the domain instead, without the questions.
From then on, the users can only sign in and export their data.

export-html writes the static closure page, for the domain to serve once the
server is stopped after the export deadline: <dir>/<locale>.html for each locale
of -lang (every locale by default, the instance's default locale first) and
<dir>/index.html with every locale one after another, in the template of the
maintenance page (my maintenance export), with the server's name, the closing
date, and the operator's message. It also writes examples of the front end's
configuration (nginx.conf, Caddyfile, cloudfront-function.js) that answer 410
Gone to /ap/*, /inbox, /.well-known/webfinger, host-meta, NodeInfo, and /@*,
and the closure page to every other path. It needs the database, and the server
closed; before delivery_completed it warns that the Deletes are still being
delivered.

Every change is in the audit log as the CLI's. The server keeps running after
the closing date until it is closed.
```

Documentation: [docs/operations/service-closure.md](../operations/service-closure.md)

### `my admin closure announce`

```text
my admin closure announce -closes-at <time> [-export-until <time>] [-message <text>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-closes-at <string>` |  | when the server closes, as RFC 3339 or a date (UTC midnight) (required) |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-export-until <string>` |  | until when the users may export their data, not before -closes-at (default: 30 days after it) |
| `-message <string>` |  | the operator's message to the users, in plain text |

Its help is the help of [`my admin closure`](#my-admin-closure).

### `my admin closure check`

```text
my admin closure check [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin closure`](#my-admin-closure).

### `my admin closure close`

```text
my admin closure close [-confirm-domain <domain>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-confirm-domain <string>` |  | the domain of the server, which confirms the closure without the questions (for automation) |

Its help is the help of [`my admin closure`](#my-admin-closure).

### `my admin closure export-html`

```text
my admin closure export-html -out <dir> [-lang en,ja] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-lang <string>` |  | comma-separated locales, in the order of index.html (default: every locale, the instance's default locale first) |
| `-out <string>` |  | directory to write the pages and the configuration examples to (required) |

Its help is the help of [`my admin closure`](#my-admin-closure).

### `my admin closure status`

```text
my admin closure status [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin closure`](#my-admin-closure).

### `my admin closure withdraw`

```text
my admin closure withdraw [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin closure`](#my-admin-closure).

### `my admin create`

```text
my admin create -username <name> -email <address> [-password-link] [-config <file>]
my admin create creates an administrator: a user with the password, the email address as primary
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-email <string>` |  | email address of the new admin (required) |
| `-password-link` | `false` | print a single-use link that sets the password, instead of reading it |
| `-username <string>` |  | username of the new admin (required) |

Its help is the help of [`my admin`](#my-admin).

### `my admin domain`

Subcommands: [`export`](#my-admin-domain-export), [`import`](#my-admin-domain-import), [`issue`](#my-admin-domain-issue), [`lift`](#my-admin-domain-lift), [`list`](#my-admin-domain-list).

```text
Usage:
  my admin domain list [-domain <domain>] [-active] [-before <id>] [-limit <n>] [-config <file>]
  my admin domain issue -kind <kind> [-public-comment <text>] [-private-note <text>]
                        [-obfuscate] [-duration <d>] [-config <file>] <domain>
  my admin domain lift [-config <file>] <action id>
  my admin domain export [-config <file>]
  my admin domain import [-apply] [-only <domains>] [-private-note <text>]
                         [-config <file>] <file | ->

Moderation actions on remote domains and their subdomains, in effect at
once for federation and the timelines:

  suspend          no deliveries to the domain, no fetches from it, and no
                   activities accepted from it; its accounts and posts are hidden
  limit            its posts stay off the public timelines and its accounts reach
                   only their followers
  reject_media     its media are not fetched
  reject_reports   the reports it sends are dropped
  force_sensitive  its media are marked sensitive

The domain may be given in U-labels (例え.jp); it is stored in IDNA A-labels.
list shows the actions newest first, lifted and expired ones included unless
-active is given. lift ends one action; the other actions on the domain stay.
Lifting a lifted action changes nothing.

export writes the actions in effect to stdout as Mastodon's domain block CSV
(#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate),
one row per domain: severity suspend for a suspend, silence for a limit, noop
otherwise. Domains with only force_sensitive, which Mastodon cannot hold, are
left out.

import reads a CSV that Mastodon exported ("-" reads stdin; a file without the
#domain header is a list of domains to suspend; at most 5000 rows) and prints
what each row would issue: suspend for suspend, limit for silence, nothing for
noop, plus reject_media and reject_reports. Rows whose actions are already in
effect on the domain or a parent domain, duplicates, and invalid rows issue
nothing. Nothing changes without -apply; -only keeps the rows of the listed
domains. Imported actions last until lifted and are audited like any other.
```

### `my admin domain export`

```text
my admin domain export [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin domain`](#my-admin-domain).

### `my admin domain import`

```text
my admin domain import [-apply] [-only <domains>] [-private-note <text>]
                       [-config <file>] <file | ->
```

| Flag | Default | Description |
| --- | --- | --- |
| `-apply` | `false` | issue the actions; without it, only show what the import would do |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-only <string>` |  | import only these domains, separated by commas |
| `-private-note <string>` |  | a note for the moderators on every action issued |

Its help is the help of [`my admin domain`](#my-admin-domain).

### `my admin domain issue`

```text
my admin domain issue -kind <kind> [-public-comment <text>] [-private-note <text>]
                      [-obfuscate] [-duration <d>] [-config <file>] <domain>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-duration <duration>` | `0s` | how long the action lasts, such as 720h; 0 lasts until lifted |
| `-kind <string>` |  | limit, suspend, reject_media, reject_reports, or force_sensitive (required) |
| `-obfuscate` | `false` | hide part of the domain where domain actions are listed publicly |
| `-private-note <string>` |  | a note for the moderators |
| `-public-comment <string>` |  | the reason that may be shown publicly |

Its help is the help of [`my admin domain`](#my-admin-domain).

### `my admin domain lift`

```text
my admin domain lift [-config <file>] <action id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin domain`](#my-admin-domain).

### `my admin domain list`

```text
my admin domain list [-domain <domain>] [-active] [-before <id>] [-limit <n>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-active` | `false` | show only the actions in effect now |
| `-before <string>` |  | show the actions older than this action ID |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-domain <string>` |  | show only the actions on this domain |
| `-limit <int>` | `50` | number of actions to show, newest first |

Its help is the help of [`my admin domain`](#my-admin-domain).

### `my admin emoji`

Subcommands: [`copy`](#my-admin-emoji-copy), [`create`](#my-admin-emoji-create), [`delete`](#my-admin-emoji-delete), [`disable`](#my-admin-emoji-disable), [`enable`](#my-admin-emoji-enable), [`list`](#my-admin-emoji-list), [`replace-image`](#my-admin-emoji-replace-image), [`requests`](#my-admin-emoji-requests), [`update`](#my-admin-emoji-update).

```text
Usage:
  my admin emoji create -shortcode <name> [-category <text>] [-alias <name>]... [-hidden]
                        [-config <file>] <image file>
  my admin emoji list [-local | -remote] [-domain <domain>] [-shortcode <text>] [-before <id>]
                      [-limit <n>] [-config <file>]
  my admin emoji update [-category <text>] [-alias <name>]... [-no-aliases]
                        [-hidden | -hidden=false] [-config <file>] <emoji id>
  my admin emoji replace-image [-config <file>] <emoji id> <image file>
  my admin emoji enable|disable [-config <file>] <emoji id>
  my admin emoji copy [-shortcode <name>] [-config <file>] <emoji id>
  my admin emoji delete [-config <file>] <emoji id>
  my admin emoji requests list|approve|reject ...   (see my admin emoji requests -h)

The custom emoji of this instance, which posts and profiles use as
:shortcode:, and those of other servers that posts and profiles brought in.

create registers a local emoji with an image: a PNG, a GIF, or a WebP of at most
custom_emoji_max_bytes (256 KiB by default) and 1,024 pixels on each side. The
image is re-encoded without metadata into variants of up to 256 pixels; an APNG
shows only its first frame. GoToSocial does not fetch emoji larger than 100 KiB
by default. -hidden keeps the emoji out of the picker.

update changes what it is given of a local emoji: -category ("" removes it), the
aliases (-alias replaces them all, -no-aliases removes them), and whether the
picker shows it (-hidden, or -hidden=false). replace-image gives a local emoji a
new image, checked as for create; the servers that have the emoji fetch it again.

disable shows the emoji nowhere (its :shortcode: stays text) until enable; both
work on remote emoji too. copy registers a new local emoji with the image of a
remote one, fetched under the limits of the media proxy (never from a
reject_media domain), with the remote shortcode or -shortcode, in the picker,
without a category or aliases. delete deletes a local emoji with its images for
good; the posts that used it show its :shortcode: as text.

Every operation is recorded in the audit log as the CLI's, as the same operation
of the admin screen and the admin API (/api/v1/admin/custom_emojis) is.

requests reviews the requests of the users whose roles grant
create_custom_emojis.
```

### `my admin emoji copy`

```text
my admin emoji copy [-shortcode <name>] [-config <file>] <emoji id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-shortcode <string>` |  | the shortcode of the local emoji (default: the remote emoji's) |

Its help is the help of [`my admin emoji`](#my-admin-emoji).

### `my admin emoji create`

```text
my admin emoji create -shortcode <name> [-category <text>] [-alias <name>]... [-hidden]
                      [-config <file>] <image file>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-alias <value>` |  | another name that the picker finds the emoji by; repeat for more |
| `-category <string>` |  | the category of the picker |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-hidden` | `false` | keep the emoji out of the picker; texts can still use it |
| `-shortcode <string>` |  | 2 to 30 letters, digits, and underscores (required) |

Its help is the help of [`my admin emoji`](#my-admin-emoji).

### `my admin emoji delete`

```text
my admin emoji delete [-config <file>] <emoji id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin emoji`](#my-admin-emoji).

### `my admin emoji disable`

```text
my admin emoji disable [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin emoji`](#my-admin-emoji).

### `my admin emoji enable`

```text
my admin emoji enable [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin emoji`](#my-admin-emoji).

### `my admin emoji list`

```text
my admin emoji list [-local | -remote] [-domain <domain>] [-shortcode <text>] [-before <id>]
                    [-limit <n>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-before <string>` |  | show the emoji older than this emoji ID |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-domain <string>` |  | show only the emoji of this remote domain |
| `-limit <int>` | `50` | number of emoji to show, newest first |
| `-local` | `false` | show only the emoji of this instance |
| `-remote` | `false` | show only the emoji of other servers |
| `-shortcode <string>` |  | show only the emoji whose shortcode contains this text |

Its help is the help of [`my admin emoji`](#my-admin-emoji).

### `my admin emoji replace-image`

```text
my admin emoji replace-image [-config <file>] <emoji id> <image file>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin emoji`](#my-admin-emoji).

### `my admin emoji requests`

Subcommands: [`approve`](#my-admin-emoji-requests-approve), [`list`](#my-admin-emoji-requests-list), [`reject`](#my-admin-emoji-requests-reject).

```text
Usage:
  my admin emoji requests list [-state <state>|all] [-before <id>] [-limit <n>] [-config <file>]
  my admin emoji requests approve [-config <file>] <request id>
  my admin emoji requests reject [-reason <text>] [-config <file>] <request id>

The requests of the users whose roles grant create_custom_emojis: a
request for a new local custom emoji (kind create) or for a new image of one the
requester made (kind image). A request is no emoji until it is approved.

list shows the pending requests, newest first, or those in -state (approved,
rejected, withdrawn, lapsed, or all); REQUESTER is the requester's user ID.
approve makes the emoji, without a category, in the picker, whose creator is the
requester, or gives the emoji the new image; the image becomes an upload of the
instance actor. It fails when the requester can no longer create custom emoji or
a local emoji has the shortcode: reject the request then. reject rejects it with
an optional -reason that the requester sees.

The approvals and rejections are recorded in the audit log as the CLI's and are
notified to the requester, as those of the admin screen and the admin API
(/api/v1/admin/custom_emoji_requests) are.
```

### `my admin emoji requests approve`

```text
my admin emoji requests approve [-config <file>] <request id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin emoji requests`](#my-admin-emoji-requests).

### `my admin emoji requests list`

```text
my admin emoji requests list [-state <state>|all] [-before <id>] [-limit <n>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-before <string>` |  | show the requests older than this request ID |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-limit <int>` | `50` | number of requests to show, newest first |
| `-state <string>` | `pending` | show the requests in this state: pending, approved, rejected, withdrawn, lapsed, or all |

Its help is the help of [`my admin emoji requests`](#my-admin-emoji-requests).

### `my admin emoji requests reject`

```text
my admin emoji requests reject [-reason <text>] [-config <file>] <request id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-reason <string>` |  | why, up to 500 characters, which the requester sees |

Its help is the help of [`my admin emoji requests`](#my-admin-emoji-requests).

### `my admin emoji update`

```text
my admin emoji update [-category <text>] [-alias <name>]... [-no-aliases]
                      [-hidden | -hidden=false] [-config <file>] <emoji id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-alias <value>` |  | another name that the picker finds the emoji by; repeat for more (replaces the aliases) |
| `-category <string>` |  | the category of the picker; "" removes it |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-hidden` | `false` | keep the emoji out of the picker; -hidden=false shows it there again |
| `-no-aliases` | `false` | remove every alias |

Its help is the help of [`my admin emoji`](#my-admin-emoji).

### `my admin federation`

Subcommands: [`sever`](#my-admin-federation-sever).

```text
Usage:
  my admin federation sever -direction in|out|both [-config <file>]

The follows between the local accounts and the remote ones.

sever ends every pending or active follow with a remote account in the direction
given: in ends the remote accounts' follows of local ones, out the local accounts'
follows of remote ones, and both all of them. They end as a domain suspension ends
them (follow_terminations with the reason moderation) and do not come back when
federation is widened again. Where the federation mode sends them, the remote
side gets a Reject of its Follow (in) or an Undo of the local account's Follow
(out); in a mode that does not, nothing is sent. Changing the federation mode
never severs anything by itself.
```

### `my admin federation sever`

```text
my admin federation sever -direction in|out|both [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-direction <string>` |  | in (remote followers of local accounts), out (local accounts' follows of remote ones), or both (required) |

Its help is the help of [`my admin federation`](#my-admin-federation).

### `my admin media`

Subcommands: [`reprocess`](#my-admin-media-reprocess).

```text
Usage:
  my admin media reprocess [-config <file>] -before <time>

Queues the processing again, with the current settings, of the uploaded images
last processed before the time (RFC 3339, such as 2027-01-31T09:00:00Z). Use the
time when the media settings changed. The worker takes the images in batches a
minute apart and processes each one in a job of its own.

Running it again with the same time is safe: an image processed since is left
alone, and the images the first run did not reach are picked up. Originals are
not kept, so each processing re-encodes the large version: it cannot bring back
detail, and every run with a later time loses a little quality. Video and audio
files are never re-encoded and are left alone.

An image's new version has new URLs. Its old files stay for
media_replaced_retention, and are deleted by the hourly cleanup.
```

### `my admin media reprocess`

```text
my admin media reprocess [-config <file>] -before <time>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-before <string>` |  | reprocess the images last processed before this time, in RFC 3339 (required) |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin media`](#my-admin-media).

### `my admin refetch`

Subcommands: [`account`](#my-admin-refetch-account), [`domain`](#my-admin-refetch-domain), [`status`](#my-admin-refetch-status).

```text
Usage:
  my admin refetch status [-config <file>] <status id or URI>
  my admin refetch account [-config <file>] <@username@domain or account id>
  my admin refetch domain [-config <file>] <domain>

Queues the refetch of stored remote posts from their servers. Each
post is fetched again with the rules of every fetch (signed by the instance
actor, never from a suspended or unavailable server) and recorded as a received
Update would be: a newer revision, the counts of its poll, or its deletion when
the server answers 410 Gone or serves a Tombstone. A post that cannot be
fetched is left as it is. Nothing refetches posts on a schedule.

status refetches one remote post. account refetches the newest 1000 posts of a
remote account that are not deleted. domain refetches the newest 20 posts of
each account of a remote server, newest accounts first, 1000 in all. The posts
of account and domain are fetched in batches a minute apart. Every request is
recorded in the audit log.
```

### `my admin refetch account`

```text
my admin refetch account [-config <file>] <@username@domain or account id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin refetch`](#my-admin-refetch).

### `my admin refetch domain`

```text
my admin refetch domain [-config <file>] <domain>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin refetch`](#my-admin-refetch).

### `my admin refetch status`

```text
my admin refetch status [-config <file>] <status id or URI>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin refetch`](#my-admin-refetch).

### `my admin relay`

Subcommands: [`create`](#my-admin-relay-create), [`delete`](#my-admin-relay-delete), [`follow`](#my-admin-relay-follow), [`list`](#my-admin-relay-list), [`unfollow`](#my-admin-relay-unfollow).

```text
Usage:
  my admin relay list [-config <file>]
  my admin relay create [-protocol mastodon|litepub] [-send-only] [-config <file>] <url>
  my admin relay follow [-config <file>] <relay id>
  my admin relay unfollow [-config <file>] <relay id>
  my admin relay delete [-config <file>] <relay id>

The relays the instance joins. A relay passes the public posts of the
servers that join it to the others.

create registers a relay and sends the instance actor's Follow; the relay is
pending until it accepts or rejects it. <url> is the relay's inbox for a
mastodon relay (pub-relay, Activity-Relay) and its actor for a litepub one
(Pleroma and Akkoma's relays); without -protocol, a URL that serves an actor is
a litepub relay and any other a mastodon one. Only public posts go to a relay,
with a linked data signature, and the public posts it relays are fetched from
their origin into the federated and hashtag timelines, up to
relay_max_activities_per_hour per hour, and kept for relay_status_retention_days.
-send-only takes in nothing from the relay.

unfollow leaves a pending or accepted relay with the Undo of its Follow, and
follow joins a relay that the instance left or that rejected it again. delete
removes the registration of a relay that the instance left or that rejected it.
list shows every relay with its state and what it brought in during the last
24 hours. Every operation, and the relay's Accept or Reject, is recorded in the
audit log.
```

### `my admin relay create`

```text
my admin relay create [-protocol mastodon|litepub] [-send-only] [-config <file>] <url>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-protocol <string>` |  | mastodon (the URL is the relay's inbox) or litepub (the URL is its actor); detected when omitted |
| `-send-only` | `false` | send public posts to the relay without taking in what it relays |

Its help is the help of [`my admin relay`](#my-admin-relay).

### `my admin relay delete`

```text
my admin relay delete [-config <file>] <relay id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin relay`](#my-admin-relay).

### `my admin relay follow`

```text
my admin relay follow [-config <file>] <relay id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin relay`](#my-admin-relay).

### `my admin relay list`

```text
my admin relay list [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin relay`](#my-admin-relay).

### `my admin relay unfollow`

```text
my admin relay unfollow [-config <file>] <relay id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin relay`](#my-admin-relay).

### `my admin report`

Subcommands: [`assign`](#my-admin-report-assign), [`forward`](#my-admin-report-forward), [`list`](#my-admin-report-list), [`note`](#my-admin-report-note), [`reopen`](#my-admin-report-reopen), [`resolve`](#my-admin-report-resolve), [`show`](#my-admin-report-show), [`unassign`](#my-admin-report-unassign).

```text
Usage:
  my admin report list [-state open|resolved] [-before <id>] [-limit <n>] [-config <file>]
  my admin report show [-config <file>] <report id>
  my admin report assign [-config <file>] <report id> <username>
  my admin report unassign [-config <file>] <report id>
  my admin report resolve -outcome actioned|dismissed [-config <file>] <report id>
  my admin report note [-config <file>] <report id> <text>
  my admin report reopen [-config <file>] <report id>
  my admin report forward [-comment <text>] [-config <file>] <report id>

The reports in the moderators' queue, including those received from
other servers. list shows them newest first; show adds the moderators' notes
and the number of the target's other reports and actions. assign makes a local
user whose roles grant manage_reports handle a report; unassign leaves it to
nobody. resolve closes an open report with its outcome; resolving a resolved
report changes nothing. note adds an internal note (up to 2,000 characters)
that only the moderators see. reopen puts a resolved report back in the queue.
forward sends a report filed on this server about a remote account to that
account's server, as your server's report, with the account and its posts.
The only text sent is -comment, written for that server's moderators (none
by default): the reporter's comment and the note of an automated review stay
here. A report forwarded already stays as it is.
```

### `my admin report assign`

```text
my admin report assign [-config <file>] <report id> <username>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin report`](#my-admin-report).

### `my admin report forward`

```text
my admin report forward [-comment <text>] [-config <file>] <report id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-comment <string>` |  | text for the moderators of the target's server, up to 1,000 characters |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin report`](#my-admin-report).

### `my admin report list`

```text
my admin report list [-state open|resolved] [-before <id>] [-limit <n>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-before <string>` |  | show the reports older than this report ID |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-limit <int>` | `50` | number of reports to show, newest first |
| `-state <string>` |  | show only the reports in this state: open or resolved |

Its help is the help of [`my admin report`](#my-admin-report).

### `my admin report note`

```text
my admin report note [-config <file>] <report id> <text>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin report`](#my-admin-report).

### `my admin report reopen`

```text
my admin report reopen [-config <file>] <report id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin report`](#my-admin-report).

### `my admin report resolve`

```text
my admin report resolve -outcome actioned|dismissed [-config <file>] <report id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-outcome <string>` |  | actioned or dismissed (required) |

Its help is the help of [`my admin report`](#my-admin-report).

### `my admin report show`

```text
my admin report show [-config <file>] <report id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin report`](#my-admin-report).

### `my admin report unassign`

```text
my admin report unassign [-config <file>] <report id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin report`](#my-admin-report).

### `my admin retention`

Subcommands: [`hold`](#my-admin-retention-hold), [`list`](#my-admin-retention-list), [`release`](#my-admin-retention-release).

```text
Usage:
  my admin retention hold -reason <text> [-config <file>] <account>
  my admin retention release [-config <file>] <hold id>
  my admin retention list [-account <account>] [-config <file>]

Holds on the erasure of an account's records after their retention:
while a hold is in effect, the purges keep the account's moderation records
(the actions on it and their appeals, the reports of it or by it, and the audit
log entries on them), its sign-in and registration IP addresses, and its posts.
Use one for a court order to preserve the records or while a request of access
or disclosure is handled. An account may have several holds; each is released
on its own, and the purges erase what is past its retention at their next run
after the last one is released. Releasing a released hold changes nothing.
list shows the holds newest first, released ones included.
<account> is @username for a local account, @username@domain for a remote one,
or an account ID.
```

### `my admin retention hold`

```text
my admin retention hold -reason <text> [-config <file>] <account>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-reason <string>` |  | why the records are preserved, such as a court order (required) |

Its help is the help of [`my admin retention`](#my-admin-retention).

### `my admin retention list`

```text
my admin retention list [-account <account>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-account <string>` |  | show only the holds on this account |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin retention`](#my-admin-retention).

### `my admin retention release`

```text
my admin retention release [-config <file>] <hold id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin retention`](#my-admin-retention).

### `my admin role`

Subcommands: [`grant`](#my-admin-role-grant), [`list`](#my-admin-role-list), [`revoke`](#my-admin-role-revoke).

```text
Usage:
  my admin role list [-user <username>] [-config <file>]
  my admin role grant [-config <file>] <username> <role>
  my admin role revoke [-config <file>] <username> <role>

The roles of local users. list shows every role, highest position
first, with the permissions it grants (moderation and server administration
apart) and the users who hold it, or with -user
the roles of one user. grant and revoke change one role of one user; the
change applies from the user's next request and is recorded in the audit log
(my admin audit list -target-kind role). Granting a role the user holds, or
revoking one they do not hold, changes nothing. The only holder of the admin
role cannot lose it.

The built-in roles are admin (position 1000, every permission) and moderator
(position 100: manage_reports, manage_accounts, view_audit).
```

### `my admin role grant`

```text
my admin role grant [-config <file>] <username> <role>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin role`](#my-admin-role).

### `my admin role list`

```text
my admin role list [-user <username>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-user <string>` |  | show only the roles of the local user with this username |

Its help is the help of [`my admin role`](#my-admin-role).

### `my admin role revoke`

```text
my admin role revoke [-config <file>] <username> <role>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin role`](#my-admin-role).

### `my admin signup-block`

Subcommands: [`create`](#my-admin-signup-block-create), [`import`](#my-admin-signup-block-import), [`lift`](#my-admin-signup-block-lift), [`list`](#my-admin-signup-block-list).

```text
Usage:
  my admin signup-block list [-kind <kind>] [-active] [-before <id>] [-limit <n>] [-config <file>]
  my admin signup-block create -kind <kind> [-severity reject|require_approval] [-comment <text>]
                            [-duration <d>] [-config <file>] <value>
  my admin signup-block import -kind <kind> [-severity reject|require_approval] [-comment <text>]
                               [-duration <d>] [-config <file>] <file | ->
  my admin signup-block lift [-config <file>] <block id>

Blocks on sign-ups from the registration page, in effect at once:

  email_domain  the domain of the email address and its subdomains; <value> is a
                domain such as spam.example (U-labels such as 例え.jp are
                stored in IDNA A-labels)
  email_hash    one email address, compared in lower case without the part of
                the local part from its first "+"; <value> is the address, which
                is stored only as its SHA-256, or that hash in hex
  ip_range      the client addresses in a CIDR prefix such as 192.0.2.0/24; a
                single address blocks only itself

A sign-up that a block matches is refused (reject, the default) or starts
pending for a moderator whatever the registration mode (require_approval); the
registration page does not say why. Bans can add the email_hash block of the
banned account (my admin account action issue -kind ban -block-email).

import adds a block for each value of a list file ("-" reads stdin), such as a
public list of disposable email domains, all with the same kind and settings:
one value per line, or a CSV row whose first field is the value; blank lines
and lines starting with # are ignored; at most 10000 values. It skips and
prints the lines that are not valid for the kind, repeat an earlier line, or
are already blocked by a block in effect at least as strong; importing the
same list again adds only what is new.

list shows the blocks newest first, lifted and expired ones included unless
-active is given. lift ends one block; lifting a lifted block changes nothing.
Every addition and lift is recorded in the audit log.
```

### `my admin signup-block create`

```text
my admin signup-block create -kind <kind> [-severity reject|require_approval] [-comment <text>]
                             [-duration <d>] [-config <file>] <value>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-comment <string>` |  | a note for the moderators |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-duration <duration>` | `0s` | how long the block lasts, such as 720h; 0 lasts until lifted |
| `-kind <string>` |  | email_domain, email_hash, or ip_range (required) |
| `-severity <string>` | `reject` | reject or require_approval |

Its help is the help of [`my admin signup-block`](#my-admin-signup-block).

### `my admin signup-block import`

```text
my admin signup-block import -kind <kind> [-severity reject|require_approval] [-comment <text>]
                             [-duration <d>] [-config <file>] <file | ->
```

| Flag | Default | Description |
| --- | --- | --- |
| `-comment <string>` |  | a note for the moderators |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-duration <duration>` | `0s` | how long the block lasts, such as 720h; 0 lasts until lifted |
| `-kind <string>` |  | email_domain, email_hash, or ip_range (required) |
| `-severity <string>` | `reject` | reject or require_approval |

Its help is the help of [`my admin signup-block`](#my-admin-signup-block).

### `my admin signup-block lift`

```text
my admin signup-block lift [-config <file>] <block id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin signup-block`](#my-admin-signup-block).

### `my admin signup-block list`

```text
my admin signup-block list [-kind <kind>] [-active] [-before <id>] [-limit <n>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-active` | `false` | show only the blocks in effect now |
| `-before <string>` |  | show the blocks older than this block ID |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-kind <string>` |  | show only the blocks of this kind: email_domain, email_hash, or ip_range |
| `-limit <int>` | `50` | number of blocks to show, newest first |

Its help is the help of [`my admin signup-block`](#my-admin-signup-block).

### `my admin supporters`

Subcommands: [`cancel`](#my-admin-supporters-cancel), [`disable`](#my-admin-supporters-disable), [`enable`](#my-admin-supporters-enable), [`plans`](#my-admin-supporters-plans), [`status`](#my-admin-supporters-status), [`subscriptions`](#my-admin-supporters-subscriptions).

```text
Usage:
  my admin supporters status [-config <file>]
  my admin supporters enable [-config <file>]
  my admin supporters disable [-config <file>]
  my admin supporters plans [-config <file>]
  my admin supporters subscriptions [-plan <id>] [-state <state>] [-before <id>] [-limit <n>] [-config <file>]
  my admin supporters cancel [-config <file>] <subscription id>

The supporter program: paid plans that local users subscribe to on the
web client, through the payment provider of payments.

status shows whether the program is enabled and what enabling it needs. enable
enables it with the configured payment provider; it needs a provider and email
(mail other than none). disable disables it; it fails while subscriptions go on:
retire the plans first (on the admin screen) and wait for them to end.

plans lists every plan, retired ones too, by rank: STATE is ready, preparing
(its prices are not made at the provider yet), or retired, and MEMBERS counts the
subscriptions that hold its role. subscriptions lists the subscriptions, newest
first, of a -plan or in a -state; USER is the subscriber's user ID.

cancel ends a subscription at once, without a refund: the plan's role goes, and
the provider's subscription is canceled at once by a job. A payment for a pending
subscription gave nothing, and the job refunds it.

Enabling, disabling, and cancelling are recorded in the audit log as the CLI's,
as those of the admin screen and the admin API (/api/v1/admin/supporter_program,
/api/v1/admin/supporter_subscriptions) are.
```

### `my admin supporters cancel`

```text
my admin supporters cancel [-config <file>] <subscription id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin supporters`](#my-admin-supporters).

### `my admin supporters disable`

```text
my admin supporters disable [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin supporters`](#my-admin-supporters).

### `my admin supporters enable`

```text
my admin supporters enable [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin supporters`](#my-admin-supporters).

### `my admin supporters plans`

```text
my admin supporters plans [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin supporters`](#my-admin-supporters).

### `my admin supporters status`

```text
my admin supporters status [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my admin supporters`](#my-admin-supporters).

### `my admin supporters subscriptions`

```text
my admin supporters subscriptions [-plan <id>] [-state <state>] [-before <id>] [-limit <n>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-before <string>` |  | show the subscriptions older than this subscription ID |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-limit <int>` | `50` | number of subscriptions to show, newest first |
| `-plan <string>` |  | show the subscriptions of this plan ID |
| `-state <string>` |  | show the subscriptions in this state: pending, active, canceling, past_due, lapsed, expired, revoked, or abandoned |

Its help is the help of [`my admin supporters`](#my-admin-supporters).

## `my config`

Subcommands: [`check`](#my-config-check), [`vapid-key`](#my-config-vapid-key).

```text
Usage: my config check [-config <file>] [-maintenance | -instance]
       my config vapid-key

check validates the configuration from the environment, the TOML file, and
defaults, listing every problem at once. Exits 1 if the configuration is invalid.
A renamed or removed setting is a problem that names its replacement. An
environment variable that starts with MY_ but sets nothing is ignored with a
warning.
In the production profile it then checks that the valid settings suit a
production instance. Errors, such as
a host that other servers cannot reach or a key-encryption key that is not
random, exit 1; warnings, such as mail = none, do not. my serve logs them at
start without refusing to start.
With -instance it also opens the database and warns about the instance
settings that are not set: the name, short description, terms, privacy policy,
contacts, and rules.
With -maintenance it checks only the settings that the maintenance mode reads,
leaving the adapters' settings unread.

vapid-key generates a key pair for Web Push and prints the private key, the
value of MY_VAPID_PRIVATE_KEY, and the public key that browsers subscribe with.
```

Documentation: [docs/operations/production-checklist.md](../operations/production-checklist.md)

### `my config check`

```text
my config check [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-instance` | `false` | also check the instance settings in the database, when it can be reached |
| `-maintenance` | `false` | check only the settings that the maintenance mode reads |

### `my config vapid-key`

```text
my config vapid-key
```

Its help is the help of [`my config`](#my-config).

## `my db`

Subcommands: [`backup`](#my-db-backup), [`upgrade`](#my-db-upgrade).

```text
Usage:
  my db upgrade [-config <file>]
  my db upgrade -check [-config <file>]
  my db backup [-config <file>] <file>

Upgrading to a new release runs, in this order: back up the database, run the
new release's "my db upgrade -check", apply the new release's schema, run the
new release's "my db upgrade", then start the new release's server. Both
commands are one-off jobs; "my serve" never runs them, and only one of them may
run against a database at a time. Each release supports upgrades from the same
minor version and from the latest patch release of the previous minor version.

my db upgrade -check runs before the schema is applied and changes nothing. It
exits with 0 when the database can take this release's schema, and with 1 when
the database has not run data upgrade steps that this release's schema relies
on: the schema would drop data that those steps carry over. It then lists the
missing steps and the release whose "my db upgrade" runs each; upgrade through
that release first. A database without any table is new and passes.

my db upgrade runs after the schema is applied. It runs, in order, each data
upgrade step of this release that the database has not recorded, rebuilds the
projections that the step names, and records the step. A second run does
nothing. It exits with 0 when every step has run, and with 1 at the first step
that fails; the steps that completed stay recorded, and running it again
resumes from the failed step. The server reports not ready (503 on /readyz)
until every step of its release is recorded.

my db backup copies a SQLite database (database = sqlite) to the new <file>, which
must not exist, in one consistent snapshot: the server may keep running and
writing meanwhile, and the copy holds every write committed before the backup
started. The copy is one file; restore it by putting it in place of the
database file, with the server stopped and the old -wal and -shm files
removed. Back up PostgreSQL with pg_dump instead.
```

### `my db backup`

A binary built with the build tag `no_sqlite` has no SQLite and refuses to back up.

```text
my db backup [-config <file>] <file>
my db backup copies a SQLite database (database = sqlite) to the new <file>, which
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my db`](#my-db).

### `my db upgrade`

```text
my db upgrade [-config <file>]
my db upgrade -check [-config <file>]
my db upgrade -check runs before the schema is applied and changes nothing. It
my db upgrade runs after the schema is applied. It runs, in order, each data
```

| Flag | Default | Description |
| --- | --- | --- |
| `-check` | `false` | check that the database ran the data upgrade steps that this release's schema requires, and change nothing |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my db`](#my-db).

## `my instance`

Subcommands: [`brand`](#my-instance-brand), [`get`](#my-instance-get), [`history`](#my-instance-history), [`list`](#my-instance-list), [`rule`](#my-instance-rule), [`set`](#my-instance-set), [`theme`](#my-instance-theme).

```text
Usage:
  my instance list [-config <file>]
  my instance get [-config <file>] <key>
  my instance set [-config <file>] <key> <value>
  my instance set -owner <username> [-config <file>] <preview_image|thumbnail> <image file>
  my instance history [-limit <n>] [-config <file>] <key>
  my instance rule <list|create|edit|move|retire> ...  (my instance rule -h)
  my instance theme <get|set> ...  (my instance theme -h)
  my instance brand <show|clear> ...  (my instance brand -h)

Reads and changes the instance settings and texts stored in the database.
Every change is kept as a revision; history shows them, newest first. The
database schema must already be applied with Atlas.

Setting keys hold one value each; a key that has never been set reads as its
default. Text keys (name, short_description, description, terms,
privacy_policy, preview_image_alt, thumbnail_alt) hold one text each, written
in any language and shown as it is in every locale. An empty value clears a
text. A value of "-" is read from standard input.

preview_image is the media ID of the image of link previews for profiles and
posts without images (empty: the built-in image), and the text
preview_image_alt is its alt text; thumbnail and thumbnail_alt are the image
that represents the instance and its alt text. With -owner, the value of a
media key is an image file instead: it is uploaded as a media file of that
local account and used once the worker has processed it.

contact_account (the username of a local account) and contact_email are how
people reach the operators.

Deployment configuration (adapters, host, secrets) is not an instance setting;
it comes from the environment and the config file (my config check).
```

Documentation: [docs/operations/instance-settings.md](../operations/instance-settings.md)

### `my instance brand`

Subcommands: [`clear`](#my-instance-brand-clear), [`show`](#my-instance-brand-show).

```text
Usage:
  my instance brand show [-config <file>]
  my instance brand clear [-config <file>] <icon|logo|logo_dark>

Shows and clears the instance's own icon, logo, and dark logo. They are set
from the server settings of the admin screen, which draws the PNG versions of
the uploaded SVG; the command line cannot set them.

show lists each image with its revision, when and by whom it was set, the
background color of the icon, and the URL of its SVG, or (built-in) when the
built-in image is in use. clear brings one image back to the built-in one and
records the change; the files of earlier revisions stay at their URLs.
```

Documentation: [docs/operations/instance-settings.md](../operations/instance-settings.md)

### `my instance brand clear`

```text
my instance brand clear [-config <file>] <icon|logo|logo_dark>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my instance brand`](#my-instance-brand).

### `my instance brand show`

```text
my instance brand show [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my instance brand`](#my-instance-brand).

### `my instance get`

```text
my instance get [-config <file>] <key>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my instance`](#my-instance).

### `my instance history`

```text
my instance history [-limit <n>] [-config <file>] <key>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-limit <int>` | `20` | number of revisions to show, newest first |

Its help is the help of [`my instance`](#my-instance).

### `my instance list`

```text
my instance list [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my instance`](#my-instance).

### `my instance rule`

Subcommands: [`create`](#my-instance-rule-create), [`edit`](#my-instance-rule-edit), [`list`](#my-instance-rule-list), [`move`](#my-instance-rule-move), [`retire`](#my-instance-rule-retire).

```text
Usage:
  my instance rule list [-all] [-config <file>]
  my instance rule create [-hint <text>] [-config <file>] <text>
  my instance rule edit [-hint <text>] [-config <file>] <rule> <text>
  my instance rule move [-config <file>] <rule> <position>
  my instance rule retire [-config <file>] <rule>

Manages the instance rules, which GET /api/v1/instance lists and reports cite.
A rule has one line of text and an optional hint, written in any language and
shown as they are in every locale.
Every change is kept as a revision, and a rule keeps its ID for life.

create appends a rule after the rules in effect. edit changes the text of a rule;
the hint stays unless -hint is given. move puts a rule at a position among the
rules in effect, from 1, and numbers them again. retire removes a rule from the
rules in effect for good; reports that cite it still refer to it. list shows
the rules in effect, and with -all the retired ones after them.
```

Documentation: [docs/operations/instance-settings.md](../operations/instance-settings.md)

### `my instance rule create`

```text
my instance rule create [-hint <text>] [-config <file>] <text>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-hint <string>` |  | explanation of the rule (edit keeps the current one unless given) |

Its help is the help of [`my instance rule`](#my-instance-rule).

### `my instance rule edit`

```text
my instance rule edit [-hint <text>] [-config <file>] <rule> <text>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-hint <string>` |  | explanation of the rule (edit keeps the current one unless given) |

Its help is the help of [`my instance rule`](#my-instance-rule).

### `my instance rule list`

```text
my instance rule list [-all] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-all` | `false` | also list the retired rules |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my instance rule`](#my-instance-rule).

### `my instance rule move`

```text
my instance rule move [-config <file>] <rule> <position>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my instance rule`](#my-instance-rule).

### `my instance rule retire`

```text
my instance rule retire [-config <file>] <rule>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my instance rule`](#my-instance-rule).

### `my instance set`

```text
my instance set [-config <file>] <key> <value>
my instance set -owner <username> [-config <file>] <preview_image|thumbnail> <image file>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-owner <string>` |  | local username that owns an image file given as a media setting's value |

Its help is the help of [`my instance`](#my-instance).

### `my instance theme`

Subcommands: [`get`](#my-instance-theme-get), [`set`](#my-instance-theme-set).

```text
Usage:
  my instance theme get [-config <file>]
  my instance theme set [-config <file>] < theme.json

Reads and replaces the theme of the web client: overrides of the design tokens
and a custom CSS, as JSON of the admin API's PUT /api/v1/admin/theme:

  {"tokens": [{"token": "color-accent", "light": "#1447e6", "dark": "#92c5fe"},
              {"token": "radius-md", "value": "0.25rem"}],
   "custom_css": ""}

set replaces the whole theme (tokens left out go back to their defaults; no
tokens and no CSS reset it), checks every value and the contrast of the color
pairs in both themes, records a revision, and prints the theme as saved. get
prints the current theme.
```

Documentation: [docs/operations/instance-settings.md](../operations/instance-settings.md)

### `my instance theme get`

```text
my instance theme get [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my instance theme`](#my-instance-theme).

### `my instance theme set`

```text
my instance theme set [-config <file>] < theme.json
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my instance theme`](#my-instance-theme).

## `my invite`

Subcommands: [`create`](#my-invite-create), [`list`](#my-invite-list), [`revoke`](#my-invite-revoke).

```text
Usage:
  my invite create [-max-uses <n>] [-expires-in <duration>] [-config <file>]
  my invite list [-limit <n>] [-config <file>]
  my invite revoke [-config <file>] <code>

Manages the staff's invite codes, which let people sign up while the
registration_mode instance setting is "open", "approval" (approved at once), or
"invite"; no invite is created or works while it is "closed". list also shows
the invites that members created; one whose creator can no longer invite does
not work. create prints the new code and a sign-up link that fills it in; by
default an invite allows 1 sign-up and expires after 7 days (-max-uses 0 and
-expires-in 0 lift the limits). Concurrent sign-ups never use an invite more
often than it allows.
revoke stops a code from working; revoking it again changes nothing.
```

Documentation: [docs/operations/registration.md](../operations/registration.md)

### `my invite create`

```text
my invite create [-max-uses <n>] [-expires-in <duration>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-expires-in <duration>` | `168h0m0s` | how long the invite works, such as 72h; 0 means forever |
| `-max-uses <int>` | `1` | how many sign-ups the invite allows; 0 means no limit |

Its help is the help of [`my invite`](#my-invite).

### `my invite list`

```text
my invite list [-limit <n>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-limit <int>` | `50` | number of invites to show, newest first |

Its help is the help of [`my invite`](#my-invite).

### `my invite revoke`

```text
my invite revoke [-config <file>] <code>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my invite`](#my-invite).

## `my job`

Subcommands: [`delete`](#my-job-delete), [`list`](#my-job-list), [`retry`](#my-job-retry), [`types`](#my-job-types).

```text
Usage:
  my job list [-config <file>] [-limit <n>]
  my job retry [-config <file>] <id>...
  my job delete [-config <file>] <id>...
  my job types

Dead jobs are the jobs of the database queue (queue = db) that failed for good or
used up their attempts. list shows them, most recently failed first,
with their type, number of runs, failure time, and last error. retry enqueues
each named dead job again with a fresh attempt count, to run now; when a job
with the same idempotency key is pending, that job stands in for it. delete
removes each named dead job without running it. Dead jobs that nobody retries
are deleted after dead_job_retention. The exit code is 1 when any ID
names no dead job; the other IDs are still handled. types prints every job type
of this release, the names that worker_job_types and the queue routes take.
```

### `my job delete`

```text
my job delete [-config <file>] <id>...
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my job`](#my-job).

### `my job list`

```text
my job list [-config <file>] [-limit <n>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-limit <int>` | `50` | number of dead jobs to show, most recently failed first |

Its help is the help of [`my job`](#my-job).

### `my job retry`

```text
my job retry [-config <file>] <id>...
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my job`](#my-job).

### `my job types`

```text
my job types
```

Its help is the help of [`my job`](#my-job).

## `my kek`

Subcommands: [`rewrap`](#my-kek-rewrap), [`status`](#my-kek-status).

```text
Usage:
  my kek status [-config <file>]
  my kek rewrap [-config <file>]

Manages the secrets that the database stores encrypted with the key-encryption
key (KEK), such as the private keys of local actors.

status counts the stored secrets by the KEK that wraps them, by its key ID,
without decrypting anything.

rewrap re-wraps every secret that a KEK of kek_previous wraps with the current
kek, keeping the encrypted secret as it is. Run it after restarting every
process with the new kek and the old one in kek_previous. It works in small
batches and can be stopped and run again at any time: it skips the secrets
that are already wrapped with the current KEK. A secret that it cannot
re-wrap is left as it is and reported, and the command exits with 1. Once it
exits with 0, no stored secret needs the keys of kek_previous.
```

### `my kek rewrap`

```text
my kek rewrap [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my kek`](#my-kek).

### `my kek status`

```text
my kek status [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my kek`](#my-kek).

## `my login-provider`

Subcommands: [`create`](#my-login-provider-create), [`delete`](#my-login-provider-delete), [`disable`](#my-login-provider-disable), [`enable`](#my-login-provider-enable), [`list`](#my-login-provider-list), [`rotate-secret`](#my-login-provider-rotate-secret), [`set`](#my-login-provider-set).

```text
Usage:
  my login-provider list [-config <file>]
  my login-provider create -kind oidc|github|mastodon|discord -name <name> [-client-id <id>]
                        [-issuer <url or domain>] [-scope <scope>]... [-claim <claim>=<value>]...
                        [-org <organization>]... [-guild <id>]... [-allow-signup] [-allow-link=false] [-config <file>]
  my login-provider set [-name <name>] [-issuer <url or domain>] [-client-id <id>] [-scope <scope>]...
                        [-claim <claim>=<value>]... [-org <organization>]... [-guild <id>]...
                        [-allow-signup=true|false]
                        [-allow-link=true|false] [-config <file>] <provider id>
  my login-provider rotate-secret [-config <file>] <provider id>
  my login-provider enable|disable|delete [-config <file>] <provider id>

External login providers, which users sign in, sign up, and link their accounts
with. The same settings are on the administration screens.

  oidc      an OpenID Connect provider; -issuer is its issuer URL (https),
            exactly as its ID tokens name it
  github    github.com; no -issuer
  discord   discord.com; no -issuer
  mastodon  one Mastodon server (4.3 or later); -issuer is its domain or
            https://<domain>. Without -client-id, create registers a client at
            the server itself; give -client-id and the secret of a client
            registered there when the server does not let others register.
            set does not move it to another server.

create sets up an enabled provider and prints its callback URL, which you register
at the provider as the redirect URI. -client-id is required but for mastodon.
create with -client-id and rotate-secret read the client secret that the provider
gave from standard input (the first line), or prompt for it in a terminal; it
is never shown again.

New users sign up with a provider only with -allow-signup, and only as the
registration mode allows. Users may link their account to it unless
-allow-link=false. -claim (oidc only) lets in only the users whose ID token has
the claim with one of the values given for it, such as -claim hd=example.com
for a Google Workspace domain. -org (github only) lets in only the members of
one of the Organizations, which needs -scope read:org. -guild (discord only)
lets in only the members of one of the Discord servers, given by their IDs
(Server Settings > Widget, or Developer Mode > Copy Server ID); the provider then
also asks for the guilds scope.

set changes only the settings it names; -scope, -claim, -org, and -guild
replace the whole list, and -scope '' (or -claim '', -org '', -guild '') removes it. The kind does not
change.

disable keeps the provider and its links but lets nobody sign in, sign up, or
link with it until enable; delete removes it for good. Both print how many
linked users can no longer sign in with it. Every change is recorded in the
audit log.
```

### `my login-provider create`

```text
my login-provider create -kind oidc|github|mastodon|discord -name <name> [-client-id <id>]
                         [-issuer <url or domain>] [-scope <scope>]... [-claim <claim>=<value>]...
                         [-org <organization>]... [-guild <id>]... [-allow-signup] [-allow-link=false] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-allow-link` | `true` | let users link their account to the provider |
| `-allow-signup` | `false` | let new users sign up with the provider |
| `-claim <value>` |  | an OIDC condition <claim>=<value> (repeatable) |
| `-client-id <string>` |  | the client ID that the provider gave |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-guild <value>` |  | a Discord server (guild) ID that users must belong to (repeatable) |
| `-issuer <string>` |  | the OIDC issuer URL, or the Mastodon server's domain |
| `-kind <string>` |  | oidc, github, mastodon, or discord (required) |
| `-name <string>` |  | the display name |
| `-org <value>` |  | a GitHub Organization that users must belong to (repeatable) |
| `-scope <value>` |  | a scope to ask for besides the ones the kind needs (repeatable) |

Its help is the help of [`my login-provider`](#my-login-provider).

### `my login-provider delete`

```text
my login-provider delete [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my login-provider`](#my-login-provider).

### `my login-provider disable`

```text
my login-provider disable [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my login-provider`](#my-login-provider).

### `my login-provider enable`

```text
my login-provider enable [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my login-provider`](#my-login-provider).

### `my login-provider list`

```text
my login-provider list [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my login-provider`](#my-login-provider).

### `my login-provider rotate-secret`

```text
my login-provider rotate-secret [-config <file>] <provider id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my login-provider`](#my-login-provider).

### `my login-provider set`

```text
my login-provider set [-name <name>] [-issuer <url or domain>] [-client-id <id>] [-scope <scope>]...
                      [-claim <claim>=<value>]... [-org <organization>]... [-guild <id>]...
                      [-allow-signup=true|false]
                      [-allow-link=true|false] [-config <file>] <provider id>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-allow-link` | `true` | let users link their account to the provider |
| `-allow-signup` | `false` | let new users sign up with the provider |
| `-claim <value>` |  | an OIDC condition <claim>=<value> (repeatable) |
| `-client-id <string>` |  | the client ID that the provider gave |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-guild <value>` |  | a Discord server (guild) ID that users must belong to (repeatable) |
| `-issuer <string>` |  | the OIDC issuer URL, or the Mastodon server's domain |
| `-name <string>` |  | the display name |
| `-org <value>` |  | a GitHub Organization that users must belong to (repeatable) |
| `-scope <value>` |  | a scope to ask for besides the ones the kind needs (repeatable) |

Its help is the help of [`my login-provider`](#my-login-provider).

## `my maintenance`

Subcommands: [`export`](#my-maintenance-export).

```text
Usage: my maintenance export -out <dir> [-lang en,ja] [-config <file>] [-offline]

export writes the maintenance page of the maintenance mode as static,
self-contained HTML for a load balancer, CDN, or reverse proxy to serve while
my serve cannot run: <dir>/<locale>.html for each locale of -lang, and
<dir>/index.html with every locale one after another, for the front ends that
cannot choose a page by Accept-Language. The pages have inline CSS, no script,
and no external resource; their Content-Security-Policy is in a <meta> element.

The words come from the maintenance_* settings and status_page_url. Unless
-offline is given, export also reads the database, waiting at most 5 seconds,
for the instance's name (when maintenance_server_name is unset), its default
locale (when -lang is not given), and the token overrides of its theme; the
custom CSS of the theme is never included. When the database cannot be read,
export warns, writes the pages from the configuration and the default theme,
and still exits 0.
```

### `my maintenance export`

```text
my maintenance export -out <dir> [-lang en,ja] [-config <file>] [-offline]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-lang <string>` |  | comma-separated locales, in the order of index.html (default: the instance's default locale, or en) |
| `-offline` | `false` | do not read the database: use the configuration and the default theme |
| `-out <string>` |  | directory to write the pages to (required) |

Its help is the help of [`my maintenance`](#my-maintenance).

## `my plugin`

A binary built with the build tag `no_plugins` has no plugins: `my plugin` and its subcommands exit with 1 and say so.

Subcommands: [`test`](#my-plugin-test).

```text
Usage: my plugin test --kind <kind> [flags] <address>

Run the contract cases of a kind against a running plugin, and report PASS, FAIL, or
SKIP per case. Exits 1 when a case fails. The plugin may be a real one: a case with a
side effect beyond the call runs only when a flag asks for it.

<address> is unix:///path/to.sock, https://host[:port], or http://host[:port].

Flags:
  --kind           probe, translator, billing, mailer, media, key_manager, inbound.status, inbound.follow, signup.check, media.check, local.status
  --protocol       json, proto, or grpc (default: the kind's; media takes proto or grpc)
  --token-file     a file that holds the bearer token (https:// and unix:// only)
  --tls-ca-file    verify the plugin against this CA (default: the system's roots)
  --tls-cert-file  the client certificate of mTLS
  --tls-key-file   the key of the client certificate
  --timeout        the deadline of one call, such as 5s (default: the kind's)
  --billed         --kind billing: also run the case that may cost money at the plugin's
                   providers
  --mail-to        --kind mailer: run the cases that send real mail, to this address
  --mail-from      --kind mailer: the sender of that mail, as mail_from (default: the
                   --mail-to address)
  --translate-from --kind translator: the source language of the cases (default: en)
  --translate-to   --kind translator: the target language of the cases (default: ja)

The flags mean what the settings plugin_<kind>_* mean, and this command reads no
configuration.
```

### `my plugin test`

```text
my plugin test --kind <kind> [flags] <address>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-billed` | `false` | --kind billing: also run the case that may cost money at the plugin's providers |
| `-kind <string>` |  | the kind of the plugin: probe, translator, billing, mailer, media, key_manager, inbound.status, inbound.follow, signup.check, media.check, local.status |
| `-mail-from <string>` |  | --kind mailer: the sender of that mail, as mail_from (default: the --mail-to address) |
| `-mail-to <string>` |  | --kind mailer: run the cases that send real mail, to this address |
| `-protocol <string>` |  | json, proto, or grpc (default: the kind's) |
| `-timeout <duration>` | `0s` | the deadline of one call (default: the kind's) |
| `-tls-ca-file <string>` |  | verify the plugin against this CA (default: the system's roots) |
| `-tls-cert-file <string>` |  | the client certificate of mTLS |
| `-tls-key-file <string>` |  | the key of the client certificate |
| `-token-file <string>` |  | a file that holds the bearer token |
| `-translate-from <string>` |  | --kind translator: the source language of the cases (default: en) |
| `-translate-to <string>` |  | --kind translator: the target language of the cases (default: ja) |

Its help is the help of [`my plugin`](#my-plugin).

## `my projection`

Subcommands: [`list`](#my-projection-list), [`rebuild`](#my-projection-rebuild), [`verify`](#my-projection-verify).

```text
Usage:
  my projection list [-config <file>]
  my projection verify [-config <file>] [<name>]
  my projection rebuild [-config <file>] [<name>]

Projections are tables derived from the events and resources of their module.
list shows each projection with its module and tables. verify derives every
projection, or the one named, from its sources inside a transaction that it
rolls back, prints the rows that differ from what the tables hold ("-" held but
not derived, "+" derived but missing), and exits with 1 when any differ; it
changes nothing. rebuild derives them the same way and writes the difference, at
most 500 rows per transaction, so that readers never see an emptied projection.
Run rebuild while the instance is quiet: a row that changes during the rebuild
keeps its new value, and verify reports any drift that is left. <name> is a
projection or one of its tables. The worker runs the same check and correction
once a day (projection.verify).
```

### `my projection list`

```text
my projection list [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my projection`](#my-projection).

### `my projection rebuild`

```text
my projection rebuild [-config <file>] [<name>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my projection`](#my-projection).

### `my projection verify`

```text
my projection verify [-config <file>] [<name>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my projection`](#my-projection).

## `my registration`

Subcommands: [`approve`](#my-registration-approve), [`list`](#my-registration-list), [`reject`](#my-registration-reject).

```text
Usage:
  my registration list [-state pending|approved|rejected] [-limit <n>] [-config <file>]
  my registration approve [-note <text>] [-config <file>] <username>
  my registration reject [-note <text>] [-config <file>] <username>

Reviews the sign-ups from the registration page.
In the "approval" registration mode, and in "open" mode while mail = none, new
users wait for a moderator: they can sign in, but cannot use the app until
their registration is approved. list shows the pending registrations by
default, longest waiting first, with the reason the applicant gave. approve and
reject decide one pending registration; a decided registration cannot be
decided again.
```

Documentation: [docs/operations/registration.md](../operations/registration.md)

### `my registration approve`

```text
my registration approve [-note <text>] [-config <file>] <username>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-note <string>` |  | a note for the record |

Its help is the help of [`my registration`](#my-registration).

### `my registration list`

```text
my registration list [-state pending|approved|rejected] [-limit <n>] [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-limit <int>` | `50` | number of registrations to show, longest in the state first |
| `-state <string>` | `pending` | registrations to show: pending, approved, or rejected |

Its help is the help of [`my registration`](#my-registration).

### `my registration reject`

```text
my registration reject [-note <text>] [-config <file>] <username>
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-note <string>` |  | a note for the record |

Its help is the help of [`my registration`](#my-registration).

## `my schema`

Subcommands: [`apply`](#my-schema-apply).

```text
Usage:
  my schema apply [-dry-run] [-allow-destructive] [-config <file>]

my schema apply brings the database of db and database_url (the server's own
settings; it reads only those and database_dev_url) to the schema of this
release, which the binary holds. It compares
the database with the schema, prints the SQL statements of the difference, and
runs them in one transaction: when one fails, none of them stays. A database
that already matches the schema is left unchanged, so running it again is safe.
Apply the schema before starting a new release's server; "my serve" never
applies it. database = sqlite and database = postgres are supported.

On PostgreSQL it builds the schema to compare with in a temporary
schema of the database, which it drops at the end, also on failure; when the
role may not create a schema there, set database_dev_url to another database
where it may. Runs against one database take turns: a second run waits for the
first and then finds nothing to apply.

A plan that drops a table or a column loses the data in it, so my schema apply
refuses it and changes nothing unless -allow-destructive is given. Rebuilding a
SQLite table to change it does not count as a drop.

Flags:
  -dry-run            print the SQL of the plan and change nothing
  -allow-destructive  apply a plan that drops a table or a column
  -config <file>      TOML config file (default: $MY_CONFIG)

It exits with 0 when the database matches the schema afterwards (or, with
-dry-run, when the plan was printed), and with 1 when it applied nothing
because of an error or a refused drop.
```

### `my schema apply`

A binary built with the build tag `no_sqlite` or `no_postgres` cannot apply the schema of the database that the tag leaves out.

```text
my schema apply [-dry-run] [-allow-destructive] [-config <file>]
my schema apply brings the database of db and database_url (the server's own
```

| Flag | Default | Description |
| --- | --- | --- |
| `-allow-destructive` | `false` | apply a plan that drops a table or a column |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-dry-run` | `false` | print the SQL of the plan and change nothing |

Its help is the help of [`my schema`](#my-schema).

## `my search`

Subcommands: [`clear`](#my-search-clear), [`rebuild`](#my-search-rebuild), [`status`](#my-search-status).

```text
Usage:
  my search rebuild [-config <file>] [--keep] [--rate <posts a second>]
  my search clear [-config <file>]
  my search status [-config <file>]

The index of the full-text search of posts is a copy of the database, and needs
a search engine: with search = none each command says so and does nothing.

rebuild empties the index and then has the worker add every post that qualifies,
in small batches, up to 20 posts a second by default. --keep adds them without
emptying the index, which keeps search working but leaves documents that should
have gone. Run it again to resume a rebuild that stopped: it goes on from where
it was, and does not empty the index again.

clear empties the index and ends any rebuild. Run it before you change search.

status shows the backend and the progress and failures of the latest rebuild.
```

### `my search clear`

```text
my search clear [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my search`](#my-search).

### `my search rebuild`

```text
my search rebuild [-config <file>] [--keep] [--rate <posts a second>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-keep` | `false` | add the posts without emptying the index first |
| `-rate <int>` | `0` | the most posts a second, up to 1000 (default 20) |

Its help is the help of [`my search`](#my-search).

### `my search status`

```text
my search status [-config <file>]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |

Its help is the help of [`my search`](#my-search).

## `my serve`

```text
my serve [-config <file>] [-maintenance] [-dev-vite-origin <origin>] [-pseudo-locales]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config <string>` |  | TOML config file (default: $MY_CONFIG) |
| `-dev-vite-origin <string>` |  | development only: load the web client from the Vite dev server at this origin |
| `-maintenance` | `false` | start in the maintenance mode (as maintenance = true), opening no adapter |
| `-pseudo-locales` | `false` | development and tests only: make the pseudo-locales en-XA and ar-XB selectable |

```text
Usage: my serve [-config <file>] [-maintenance] [-dev-vite-origin <origin>] [-pseudo-locales]

Runs the server in the role set by role (MY_ROLE): web, worker, or all.
Stops gracefully on SIGTERM or SIGINT, waiting for in-flight requests and jobs.

-maintenance (or maintenance = true, MY_MAINTENANCE=true) starts the maintenance mode instead:
whatever the role, the process opens no database, KV, queue, or blob store, runs no job, and
answers every request with a 503 that says the server is under maintenance. It reads only the
listen address, the host and origins, the proxies, the origin_secret settings, status_page_url,
and the maintenance_* settings, so the other settings may be invalid or unreachable meanwhile.

-dev-vite-origin (development only) makes the web client's HTML load the client from a
Vite dev server, such as http://localhost:5173, instead of the build embedded in the binary.

-pseudo-locales (development and tests only) makes the pseudo-locales en-XA (longer, accented
text) and ar-XB (right-to-left) selectable through Accept-Language.
The E2E of the server-rendered pages uses it.
```

## `my version`

```text
my version
```
