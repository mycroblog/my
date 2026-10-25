# Production checklist

Go through this list before you open the server to users and to other servers, and again after a change to the deployment.
Some settings cannot be changed once the server has federated, and some mistakes, such as losing the key-encryption key,
cannot be undone at all.

Each item says why it matters, how to verify it, and where the details are.
Start with the automated part:

```sh
my config check            # the configuration, and the settings that do not suit production
my config check -instance  # also the instance settings in the database
```

## What `my config check` checks

In the `production` profile (the default of [`profile`](../reference/configuration.md#profile)), `my config check`
first validates every setting and then checks that the valid settings suit a production instance.
The problems are listed in two groups, under a summary in the language of the process locale, each with the technical
detail in English:

- **Errors** make `my config check` exit 1. They are settings that break a production instance or weaken its security:
  - `host` or `account_domain` that other servers cannot reach: with a port, `localhost`, a name without a dot, or a name under
    a top-level domain reserved for local use or tests (`.localhost`, `.local`, `.test`, `.example`, `.invalid`, `.internal`, `.home.arpa`).
  - `web_origin` over `http`.
  - An SQLite `database_url` that keeps the database in memory.
  - A `kek` that is not random, such as all zeros or a repeated byte.
  - A `captcha` site key or secret key that is one of the test keys the CAPTCHA services publish.
- **Warnings** leave the exit code at 0. They are settings that are usual in development and may be deliberate in production:
  - `mail = none`.
  - An SQLite database or the `fs` blob store in a temporary directory (`/tmp`, `/var/tmp`, `/dev/shm`, or `$TMPDIR`).
  - `queue = inline`.
  - No `vapid_private_key`.
  - A test-mode key of the payment provider.
  - With `-instance`: the instance texts `name`, `short_description`, `terms`, and `privacy_policy`, the instance settings
    `contact_account`, `contact_email`, and `security_contact` when they are not set, and a server without rules.
    When the database cannot be reached, that is one more warning and the rest of the check still runs.

`my serve` logs the same findings as warnings when it starts, but starts anyway.
The `development` profile skips these checks; use it for local development and tests, never for an instance that others use.

Run `my config check` in the same environment as the server (the same environment variables, secret files, and config file),
for example as a step of the deployment before the new version takes traffic.

## Settings you cannot change later

### The server's domain and the handle domain

- **Why**: `host` is part of every ActivityPub id of the server, and other servers store those ids.
  `account_domain` is the domain of the handles (`@alice@example.org`). Both are recorded in the database on the first start
  and must not change afterwards: a change after federating breaks every follow and every reference to the server's posts
  and accounts.
- **Decide**: whether the handles use the server's domain or a separate one (for example handles on `example.org`
  and the server on `social.example.org`). A separate handle domain must redirect its WebFinger, host-meta, and NodeInfo
  to the server.
- **Verify**: `my config check` shows `host` and `account_domain` and reports a domain that other servers cannot reach.
  With a separate handle domain, `curl -sI "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"`
  answers `301` to `https://<host>/.well-known/webfinger?...`.
- **Details**: [running the server](running.md) (the federation endpoints and serving the handles from another domain),
  [`host`](../reference/configuration.md#host) and [`account_domain`](../reference/configuration.md#account_domain).

### Reachable over `https`

- **Why**: other servers fetch the ids at `https://<host>/` on port 443, and browsers send the sign-in cookies only over `https`.
- **Verify**: `curl -sI https://<host>/healthz` answers `200` from outside your network with a certificate that is valid for `host`,
  and `my config check` reports no error about `web_origin`. `http://<host>/` redirects to `https`, which the reverse proxy,
  load balancer, or CDN in front of the server does.
- **Details**: [running the server](running.md), [`web_origin`](../reference/configuration.md#web_origin).

## Secrets

### The key-encryption key (KEK)

- **Why**: the KEK encrypts the private keys of every actor. Without it the server cannot sign anything, and the keys cannot be
  recovered from the database: every account would have to be recreated and other servers would no longer trust it.
- **Verify**:
  - It was generated randomly (`openssl rand -base64 32`); `my config check` reports a key that is not random.
  - It is stored somewhere other than the database and its backups, for example in a password manager or the cloud's
    secret store, and at least one other person who runs the server can reach it.
  - It reaches the server through `MY_KEK` or `MY_KEK_FILE` only (the config file refuses secrets).
- **Details**: [key management](key-management.md).

### The VAPID key of Web Push

- **Why**: without it the server sends no Web Push notifications. Changing it later ends every subscription, so generate it once
  and keep it like the KEK.
- **Verify**: `my config check` warns when `vapid_private_key` is not set. Generate one with `my config vapid-key`.
- **Details**: [key management](key-management.md) (the VAPID key of Web Push).

### Other secrets stay out of the repository and the image

- **Why**: a secret in a repository, a container image, or a config file is copied to every place those go.
- **Verify**: `database_url`, `smtp_password`, `captcha_secret_key`, the payment provider's keys, `valkey_url`, and every other
  setting marked secret in the configuration reference come from environment variables or `<NAME>_FILE` files that the platform's
  secret store fills (Docker or Kubernetes secrets, Secret Manager, Parameter Store). The config file cannot hold them:
  `my config check` refuses a secret there. Search the repository and the image for the values to make sure no copy is left.
  `my config check` prints `(redacted)` for every secret that is set.
- **Details**: [configuration reference](../reference/configuration.md) (the Secret column), [CAPTCHA](captcha.md),
  [Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md).

## Instance settings

The instance settings live in the database and can be changed while the server runs, in the server's administration screens
or with `my instance set`. `my config check -instance` warns about the texts, contacts, and rules that are not set.

- **Name, short description, and description**: shown on the landing page, the information page (`/about`), link previews,
  and NodeInfo. Verify on `/about`.
- **Contacts**: `contact_account` (the account that users contact about the server) and `contact_email` (the address that users
  and other servers write to; emails from the server also send replies to it). Verify on `/about`.
- **Vulnerability contact**: `security_contact`, an `https:` or `mailto:` URI. Set, the server answers
  `/.well-known/security.txt`. Verify with `curl -s https://<host>/.well-known/security.txt`.
- **Terms of service and privacy policy**: `terms` and `privacy_policy`. The privacy policy must name the services that receive
  personal data, such as the CAPTCHA service and the mail provider. Verify the links on `/about` and on the sign-up screen.
- **Server rules**: what users agree to when they sign up, and what moderators enforce. Add them with `my instance rule add`
  or in the administration screens, and verify them on `/about`.
- **Registration mode**: `registration_mode` is `approval` until you change it; choose `open`, `approval`, `invite`, or `closed`
  for how people join. With `mail = none`, `open` sign-ups wait for a moderator's approval too.
- **Search engines and AI**: `search_indexing`, `ai_usage`, and `indexable_default` refuse both until you allow them.
  Verify `/robots.txt` and `/.well-known/tdmrep.json`.
- **Status page**: set [`status_page_url`](../reference/configuration.md#status_page_url) (a setting, not an instance setting,
  so the link shows during maintenance) to a status page on a service apart from the server.

**Details**: [instance settings](instance-settings.md), [registration](registration.md),
[search engines and AI crawlers](crawlers.md), [CAPTCHA](captcha.md) (what to write in the privacy policy),
[monitoring](monitoring.md) (the status page).

## Email

- **Why**: email verifies addresses and sends password resets and notifications. With `mail = none` no email reaches users.
- **Verify**:
  - `mail` is `smtp` or `ses`, and `mail_from` is an address on a domain you control; `my config check` warns about `mail = none`.
  - The domain of `mail_from` has SPF, DKIM, and DMARC records, and the provider signs with DKIM. Without them, mail lands in spam.
  - Sign up with a test account and receive the verification email; request a password reset and receive that too.
    Check the headers of the received message for `spf=pass`, `dkim=pass`, and `dmarc=pass`.
  - With SES, the account is out of the sandbox, and the bounce and complaint notifications reach the server.
- **Details**: [email](email.md), [registration](registration.md) (email verification).

## Administrators

- **Why**: an administrator's account can change every setting, read private data, and suspend anyone.
- **Verify**:
  - Every administrator turned on two-factor authentication (Settings, Two-factor authentication in the web client)
    and keeps the recovery codes.
  - Only the people who need it hold the `admin` role; moderators hold `moderator` or a role with only the permissions they use.
    List them with `my admin role` and review them on the administration screens.
- **Details**: [administrators and roles](admin-cli.md), [moderation](moderation.md) (permissions and login recovery).

## Operations

- **Backups and recovery**: the database, the blob store (`blob_fs_dir` or the bucket), and the KEK are backed up on a schedule,
  to a place apart from the server, and a restore to a fresh server has been rehearsed: the restored server starts and the accounts
  can sign in and post. A backup that was never restored is not known to work. The database and the media must not be in a
  temporary directory; `my config check` warns when they are.
  See [key management](key-management.md) (backing up the KEK) and [database](database.md).
- **Monitoring and alerts**: an external monitor checks `/healthz` and alerts someone who can act, and the job queue's dead jobs
  are watched. See [monitoring](monitoring.md) and [running the server](running.md) (health checks, metrics, and the job queue).
- **Source code (AGPL-3.0)**: the server must offer its source code to everyone who uses it. Leave
  [`source_url`](../reference/configuration.md#source_url) at its default for an unmodified release; point it at your own
  source when you run a fork or a modified version. Verify the link on `/about`.
  See [running the server](running.md) (publishing the source code).
- **Maintenance**: know how to put the server into the maintenance mode before you need it. See [maintenance](maintenance.md).

## Federation

- **Why**: other servers find accounts through WebFinger and NodeInfo and then fetch the actor. If any of these fails from outside,
  the server is invisible to the fediverse.
- **Verify** from a machine outside your network:
  - `curl -s "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"` returns the account,
    following the redirect to `host` when the domains differ (`curl -L`).
  - `curl -s https://<host>/.well-known/nodeinfo` links `/nodeinfo/2.1`, which names the software and its source.
  - `curl -s -H 'Accept: application/activity+json' https://<host>/@<user>` redirects to the actor's id,
    and the actor at that id is JSON with a `publicKey`.
  - From an account on Mastodon or another server, search for `@<user>@<account_domain>`, follow it, and see a new post arrive.
  - A CDN in front of the server does not cache `/ap/*` and `/@*` unless it honors `Vary: Accept`.
- **Details**: [running the server](running.md) (the federation endpoints), [CDN](cdn.md), [relays](relays.md).
