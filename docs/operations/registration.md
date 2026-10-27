# User registration (registration modes, invites, approval, email confirmation)

This guide is for operators.

## The sign-up page

Users register at `https://<host>/auth/sign_up` with a username, an email address and a password.
After registering, that browser is signed in right away.
The page works without JavaScript and, like the sign-in page, is protected by a CSRF token and by rejecting cross-origin requests.
When you choose CAPTCHA for bot protection, the sign-up and sign-in pages show its widget and need JavaScript ([CAPTCHA](captcha.md)).

The registration mode is the instance setting `registration_mode`. Choose it with the CLI or in the web client under "Server settings" (`/admin/settings`) ([instance settings](instance-settings.md)). A change applies from the next registration.
While the server announces its closure and after it closes, registration is treated as `closed` whatever the setting says ([admin CLI](admin-cli.md#announcing-the-end-of-the-service)).

```sh
my instance set registration_mode invite
```

| Mode | Page | State right after registering |
| --- | --- | --- |
| `open` | Anyone can register | Approved, except with `mail = none`, where it waits for a moderator's approval (see below) |
| `approval` (default) | Also asks "Why do you want to join?" (optional, up to 1,000 characters) | Waiting for approval |
| `invite` | An invite code is required | Approved |
| `closed` | Only says that registration is not accepted | None |

- Registrations from the same network (IP address; `/64` for IPv6) are limited to 5 per 30 minutes. Beyond that the server returns `429` with `Retry-After`.
- If the username or the email address is taken, the page shows an error on that field and writes nothing (an invite is not consumed either).
- When you set up external login providers (OpenID Connect or GitHub) and let new users sign up with one, the sign-in and sign-up pages show a button for each enabled provider, labelled with the provider's display name exactly as you entered it (a GitHub provider shows "Sign in with GitHub" in the visitor's language).
  Someone whose account at the provider is not linked to a user of this server gets the same sign-up form without the password field, and the new user signs in with the provider only.
  The registration mode, the invite code, the reason, CAPTCHA, the sign-up blocks and the network limit apply as on the password form; in `closed` mode, or when the provider does not allow sign-ups, the sign-in page says that no account is linked.
  An address that the provider marks as verified (`email_verified`; for GitHub, the primary address when it is verified) is confirmed at once if the user keeps it; any other address is confirmed by mail as below.
  An email address that matches an existing user never links that user: the form says the address is taken.
  A user with two-factor authentication still enters the code after signing in with a provider.
  A GitHub provider asks GitHub for the `user:email` scope, and also `read:org` when you list allowed Organizations; the server then checks on every sign-in that the user belongs to one of them and refuses the sign-in otherwise. Register `https://<host>/auth/external/<provider ID>/callback` as the callback URL of your GitHub OAuth app.
- A signed-in user links an account at a provider under "Linked accounts" in settings, after confirming with the password, a passkey, or another linked account, and only for providers that allow links.
  The browser must be signed in to the sign-in pages as the same user; otherwise the server asks to sign in first.
  An account at a provider that another user links is refused, and a user links at most one account per provider.
  A user can unlink an account unless it is the last way to sign in (no password, no passkey, and no other linked account).
- A user without a password and without passkeys confirms the operations that otherwise ask for them (setting a password, adding a passkey, deleting or moving the account, linking another account) by signing in again at a linked provider.
  An OpenID Connect provider is asked to authenticate the user again (`max_age=0` and `prompt=login`), and an earlier authentication is refused.
  The confirmation works once, for that operation only, within 10 minutes; signing in with an account that is not the linked one counts as a wrong attempt, against the same limit as a wrong password.
- On every registration and sign-in, the server records the client IP address (and, for sign-in, the User-Agent). The records are removed automatically after 180 days
  (the instance settings `retention_session_origins` and `retention_registration_origins`, at least 90 days). Records of an account under a retention hold are kept ([record retention and holds](moderation.md#record-retention-and-holds)).
  State this retention period in your privacy policy.

## When a user can use the app

Users who are waiting for approval, were rejected, or have not confirmed their email address can still sign in on the sign-in page.
After signing in, they see what they are waiting for (a moderator's approval, or confirmation of the email address), or that their registration was not approved, and they cannot go further.
Only approved users can use the app (the OAuth authorization and token issuance of the client API), and on setups that require email confirmation, only those who have confirmed their email address.

An administrator created with `my admin create` counts as approved and confirmed.

## Email confirmation

The setting `mail` ([configuration reference](../reference/configuration.md#mail)) selects how mail is sent.
SMTP examples, DNS setup and how to read delivery failures are in [sending email](email.md).

- `mail = smtp`: the server sends a confirmation mail on registration. The link is valid for 24 hours and works once.
  Opening the link shows a page with a confirm button, and the address is confirmed when the user presses it (so that scanners that prefetch links in mail do not consume the token).
  After signing in, the user can have the mail sent again up to 3 times an hour.
  The connection to the SMTP server is always encrypted with STARTTLS, and the server verifies the certificate before authenticating and sending. It never sends to a server that does not offer STARTTLS.
- `mail = none` (default): the server sends no mail and writes the body (including the confirmation link) to the server log as `mail not sent (mail = none)`.
  Because mail never reaches the user, the server asks for a moderator's approval instead of email confirmation.
  So a registration in `open` mode also starts as waiting for approval, and an unconfirmed email address does not block use of the app.

The confirmation token is recorded nowhere except this log line (access logs do not record query strings). The database stores only the SHA-256 of the token.

## Password reset and change

- `mail = smtp`: when a user enters an email address under "Forgot your password?" on the sign-in page, the server sends a link for setting a new password to the user whose primary address it is.
  The page's answer is the same whether or not an account exists. The link is valid for 1 hour and works once, and it stops working if the password changes by another route in the meantime.
  Requests are limited to 5 per 30 minutes per client IP address, and mail to 3 per hour per address (a request over a limit sends nothing and gets the same answer). When CAPTCHA is enabled, this form is verified too.
- `mail = none`: the page says that it cannot send a link and tells the user to ask an administrator. The administrator issues a link on the account page of the admin screens, or with `my admin account password-reset-link @alice`, and gives it to the user ([moderation](moderation.md)).
- A signed-in user can change the password under "Password" in settings by entering the current password (wrong entries are limited to 5 per hour per user).
- A reset or a change revokes the user's other sessions and app tokens (a change from settings keeps only the app that made the change). Passkeys and two-factor authentication stay.
  A sign-in that waits for its two-factor code, and an app authorization not yet completed, stop working too: the user starts them again.
  With `mail = smtp`, the server also emails the user to say the password changed.

## Invite codes

```sh
my invite create                          # single use, expires in 7 days
my invite create -max-uses 10 -expires-in 72h
my invite create -max-uses 0 -expires-in 0  # no limit on uses or time
my invite list                            # newest first; states: usable / revoked / expired / used up
my invite revoke <code>
```

- `create` prints the code (16 characters) and the link to the sign-up page with the code filled in (`https://<host>/auth/sign_up?invite=<code>`).
- Codes are matched ignoring case, spaces and hyphens.
- Even when several people register at once, a code is never used beyond its limit (a unique constraint on `invite_redemptions`).
- A revoked, expired or used-up code all give the same error (an unusable invite).
- Besides the CLI, you can create, list and revoke invites in the web client's Moderation screens ("Invites", `/moderation/invites`) and through the client API
  (`GET` / `POST /api/v1/admin/invites` and `POST /api/v1/admin/invites/{id}/revoke`). Both need the role permission `manage_invites`
  (the built-in `moderator` does not have it, so to delegate this, create a role with that permission and grant it).
  The screen shows the code and the sign-up link of a new invite, and lets you copy the link of a usable invite. When the registration mode is not `invite`, the screen says that nobody can register with an invite.
- Creating and revoking an invite is recorded in the audit log (`invite_created`, `invite_revoked`) from the screen, the API and the CLI alike. The target name is the first 4 characters of the code (`ABCD…`); the full code is not copied to the audit log.
  Revoking an already revoked invite changes nothing, so it is not recorded.

## Approving and rejecting

```sh
my registration list                      # waiting for approval (longest wait first); shows the reason and whether the email is confirmed
my registration list -state rejected
my registration approve alice
my registration reject -note "spam" bob
```

- The argument is the username of a local account (the leading `@` may be omitted).
- Only registrations that are waiting for approval can be decided. For a decided registration, or a user created without the sign-up page (`my admin create`), the command changes nothing and exits with code 1.
- Exit code 2 means a wrong argument.

The same decisions can be made from the admin screens (the client API). All of them need the `manage_accounts` permission.

| Operation | What it does |
| --- | --- |
| `GET /api/v1/admin/registrations` | Registrations waiting for approval (longest wait first). Returns the reason for joining, the email address and whether it is confirmed, and the local account |
| `POST /api/v1/admin/registrations/{user_id}/approve` | Approves. The body `{"note": "..."}` is optional |
| `POST /api/v1/admin/registrations/{user_id}/reject` | Rejects. The body is the same as for `approve` |

- This is the same use case as the CLI, and it records the decision in `registration_decisions` and the audit log (`registration_approved`, `registration_rejected`). The actor of a CLI decision is `cli`; the actor of an API decision is the moderator who decided.
- Deciding an already decided registration changes nothing and returns the current state (if two people decide at once, only one decision is recorded). A user created without the sign-up page gives 404.

## Sign-up blocks

Blocks reject registrations from the sign-up page, or switch them to approval, by email domain, email address or IP range.

```sh
my admin signup-block add -kind email_domain spam.example          # also applies to subdomains; a U-label works too
my admin signup-block add -kind ip_range -severity require_approval -duration 720h 192.0.2.0/24
my admin signup-block add -kind email_hash spammer@example.com     # only a hash of the address is kept
my admin signup-block list -active
my admin signup-block lift <block id>
```

- A registration that matches a block with `-severity reject` (the default) is refused, and one that matches `require_approval` starts as waiting for approval whatever the registration mode is. When several match, the stronger one applies. The sign-up page does not show the reason for a rejection.
- `email_hash` is matched against the address in lower case, without the part of the local part from the first `+`. An IP range is matched against the registration IP (the same value as `registration_origins`) and never applies to a registration whose IP is unknown.
- Expired and lifted blocks have no effect. Adding and lifting are recorded in the audit log (`signup_block_added`, `signup_block_lifted`).
- When you ban a local account with `-block-email` (`block_email: true` in the API), the server also creates an `email_hash` block for that user's email address.

For a list of thousands of entries, such as the domains of disposable email services, add them in bulk with `import` from a file (`-` for standard input). Every value gets the same kind, severity, comment and duration.

```sh
my admin signup-block import -kind email_domain -comment "disposable email" disposable.txt
```

- The list has one value per line (the same form as `<value>` of `add`), or CSV rows whose first column is the value. Blank lines and lines that start with `#` are skipped. One import takes up to 10,000 entries.
- The command prints how many were added and, for each row it did not add, the line number, the value and the reason. The reason is `invalid` (not a valid value of that kind), `duplicate` (the same normal form as an earlier row of the same list) or `already_blocked` (a block of the same kind and value that is equally or more severe is in effect).
- Each added block is recorded in the audit log (`signup_block_added`) as with `add`, and can be lifted one by one.
- The import writes in transactions of a few hundred, so if it fails midway, what was added before stays. Importing the same list again adds only the rest.
- The software ships no list and does not fetch one from a URL periodically. You fetch and update public lists (such as disposable-email-domains on GitHub) yourself and import the updated list again.

The same operations are available from the admin screens (the client API). All of them need the `manage_accounts` permission.

| Operation | What it does |
| --- | --- |
| `GET /api/v1/admin/signup_blocks` | Lists blocks (newest first). Filter with `kind` and `active` |
| `POST /api/v1/admin/signup_blocks` | Adds a block. The body has `kind`, `value`, `severity`, `comment` and `expires_in` (seconds, at most ten years) |
| `POST /api/v1/admin/signup_blocks/import` | Adds blocks from a list. The body has `kind`, `text` (the list), `severity`, `comment` and `expires_in`. The response has `added`, the number added, and `skipped`, the rows not added (`line`, `value`, `reason`) |
| `GET /api/v1/admin/signup_blocks/{id}` | Returns one block |
| `POST /api/v1/admin/signup_blocks/{id}/lift` | Lifts a block. A block already lifted stays as it is |
