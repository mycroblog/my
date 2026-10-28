# External login providers

This guide is for operators and administrators.

An external login provider lets users sign in with an account they have elsewhere: an OpenID Connect provider
(Google Workspace, Microsoft Entra ID, Keycloak, and the like), GitHub, Discord, or one Mastodon server.
You set providers up with the `my login-provider` command on a host that has the server's configuration, or through the
admin API (`/api/v1/admin/login_providers`) with the `manage_login_providers` permission, which only the built-in
`admin` role has. The command works before any administrator can sign in, so you can use it for the first setup.
In the web client, administrators with that permission use "Login providers" in the administration menu: "Set up a
login provider" creates one (and shows its callback URL), and the list below it shows each provider's state, whether
sign-up is allowed, and how many users are linked, with buttons to edit, enable, disable, and delete it.

## What external sign-in does

- A provider only proves who someone is at that provider. An account is never linked to a user by email address: a user
  links an account under "Linked accounts" in settings, or signs up with it as a new user. An email address that matches
  an existing user never links that user.
- A linked user still enters the two-factor code (TOTP) after signing in with a provider, and a user whose account is
  not active cannot sign in.
- New users can sign up with a provider only when you allow it for that provider (`-allow-signup`) and the
  [registration mode](registration.md) lets them register; the invite code, CAPTCHA, sign-up blocks, and network limit
  apply as on the password form. Someone who signs up this way has no password and signs in with the provider only.

## Kinds

| Kind | `-issuer` | Conditions |
| --- | --- | --- |
| `oidc` | The issuer URL (`https`), exactly as the provider's ID tokens name it, such as `https://accounts.google.com` | `-claim <claim>=<value>`: only users whose ID token has the claim with one of the values given for it |
| `github` | None: the server knows github.com's endpoints | `-org <organization>`: only members of one of the Organizations (add `-scope read:org`) |
| `discord` | None: the server knows discord.com's endpoints | `-guild <server ID>`: only members of one of the Discord servers (guilds); none lets every Discord user in |
| `mastodon` | The server's web domain, such as `mastodon.example` (Mastodon 4.3 or later) | None |

For Google Workspace, limit the users to your organization with `-claim hd=<your domain>`. Do not rely on the domain of the
email address.

## Examples

### Google

In the Google Cloud console, create an OAuth client of type "Web application". To keep it to your organization, set the
user type of the OAuth consent screen to "Internal" (available when the project belongs to a Google Workspace
organization), and also add the `hd` condition: the user type restricts who Google lets in, and the condition makes this
server check the `hd` claim of every ID token.

```sh
printf '%s\n' "$CLIENT_SECRET" | my login-provider add -kind oidc -name Google \
  -issuer https://accounts.google.com -client-id 123.apps.googleusercontent.com -claim hd=example.com
```

### Keycloak and other OpenID Connect providers

The issuer is the realm URL, such as `https://sso.example.com/realms/main`, exactly as the provider's ID tokens name it.
Create a confidential client with the standard flow, and the `openid`, `email`, and `profile` scopes. If the provider
is on a private network, list it in `login_provider_allowed_destinations` ([below](#providers-on-an-internal-network)).
Use `-claim` to require a claim, such as a group claim, with one of the values given.

### Discord

Create an application in the [Discord Developer Portal](https://discord.com/developers/applications), open "OAuth2", and
give the server the client ID and the client secret. The server asks for the `identify` and `email` scopes, and also
`guilds` when you list servers with `-guild`. A user is known by their numeric Discord ID; the username and display name
only suggest the username on the sign-up form. The email address counts as verified only when Discord says it is
verified; otherwise the user confirms it by mail like any other address.

With `-guild`, the server checks on every sign-in, sign-up, and link that the user belongs to one of the listed servers
and refuses the user otherwise. Without it, every Discord user may sign in. A guild ID is a number of up to 20 digits: in
Discord, turn on "Developer Mode" (User Settings, Advanced), then right-click the server's icon and choose "Copy Server
ID".

```sh
printf '%s\n' "$DISCORD_SECRET" | my login-provider add -kind discord -name Discord \
  -client-id 123456789012345678 -guild 987654321098765432
```

Add the callback URL under "OAuth2", "Redirects" in the application, after `add` prints it.

### GitHub

Create an OAuth App (Settings, Developer settings, OAuth Apps) and give it the client ID and secret. The server asks
for the `user:email` scope, and also `read:org` when you list Organizations with `-org`. With `-org`, the server checks
on every sign-in that the user belongs to one of the Organizations, so members of other Organizations are refused.

## Set up a provider

The redirect URI that the provider needs is the provider's callback URL, `https://<host>/auth/external/<id>/callback`,
which contains the ID that the server gives the provider. So:

1. Register an OAuth client at the provider (an "OAuth App" on GitHub) with any redirect URI for now, and note the
   client ID and the client secret it gives.
2. Run `my login-provider add` with them. It prints the callback URL.
3. Change the redirect URI of the client at the provider to that callback URL.

```sh
# The client secret is read from standard input, or prompted for in a terminal.
printf '%s\n' "$CLIENT_SECRET" | my login-provider add -kind oidc -name "Example SSO" \
  -issuer https://sso.example.com -client-id my-client-id -claim hd=example.com
printf '%s\n' "$GITHUB_SECRET" | my login-provider add -kind github -name GitHub \
  -client-id Iv1.0123456789abcdef -org example-org -scope read:org
```

### Mastodon

A `mastodon` provider is one Mastodon server that you name; users of other servers cannot sign in with it. The server
must be Mastodon 4.3 or later: `add` reads its authorization server metadata
(`/.well-known/oauth-authorization-server`) and refuses a server without it. Without `-client-id`, `add` registers a client
at the server itself, with the callback URL and the `profile` scope, and stores the client ID and secret it gets, so
there is nothing to do at the server:

```sh
my login-provider add -kind mastodon -name mastodon.example -issuer mastodon.example
```

If the server does not let others register clients, `add` fails. Create an application with the `profile` scope and the
callback URL in the server's development settings instead, and give its client ID and secret as for the other kinds
(the callback URL contains the provider's ID, so follow the three steps above).

Users sign in with the `profile` scope only, which reads their account and nothing else. Mastodon gives no email
address, so someone who signs up with it confirms the address by mail. The sign-up form suggests the account's username.
While the server's domain, or a parent domain, has a domain action ([domain actions](moderation.md#domain-actions)), its button is gone and
nobody signs in, signs up, or links an account with it. The server of a provider does not change; set up a new provider
for another server.

The client secret is stored encrypted with the key encryption key ([key management](key-management.md)) and is never shown
again, by the command or by the admin API.

A new provider is enabled. By default users may link their existing account to it (`-allow-link`), but nobody may sign up
with it: add `-allow-signup` to let new users register with it, which the registration mode still limits
([registration](registration.md)).

## Change, rotate, disable, and delete

```sh
my login-provider list                                 # every provider with its callback URL and linked users
my login-provider set -name "Corp SSO" <id>            # changes only the settings it names
my login-provider set -claim '' <id>                   # '' removes every -claim (or -scope, -org)
my login-provider rotate-secret <id>                   # reads the new client secret like add
my login-provider disable <id>
my login-provider enable <id>
my login-provider delete <id>
```

- `set` changes only the settings it names. `-scope`, `-claim`, and `-org` replace the whole list. The kind of a provider
  does not change; set up a new provider instead.
- `rotate-secret` replaces the client secret with the one you enter. Rotate it at the provider first, then here.
- `disable` keeps the provider and the accounts linked to it, but nobody signs in, signs up, or links with it, and the
  sign-in page does not show it, until you run `enable`. `disable` and `delete` print how many linked users can no longer
  sign in with the provider.
- `delete` removes the provider for good.

### Users who sign in only through a provider

Disabling or deleting a provider does not touch the users. A user who has a password or a passkey, or has another linked
account, signs in that way. A user who has none of these can no longer sign in at all: `disable` and `delete` print how
many linked users lose the provider, and the web client shows the number in the confirmation. `enable` restores the
users of a disabled provider; a deleted provider cannot come back.

To recover such a user, issue a password reset link for the account on its page of the admin screens, or with
`my admin account password-reset-link @alice`, and give it to the user ([registration](registration.md#password-reset-and-change)).
The user then signs in with the new password.

Every change is recorded in the audit log under the provider's display name, as the CLI or as the administrator who made
it through the admin API.

## Re-authentication

Operations that ask for the password or a passkey (setting a password, adding a passkey, deleting or moving the account,
linking another account) ask a user who has neither to sign in again at a linked provider ([registration](registration.md)).
An OpenID Connect provider is asked to authenticate the user again (`max_age=0` and `prompt=login`) and the server
refuses an earlier authentication; a Mastodon server is asked with `force_login=true`. GitHub and Discord have no way to
ask for a fresh sign-in, so a user who is still signed in there is confirmed without entering anything. The server still checks
that the account is the linked one, but this is weaker proof that the person is at the keyboard. Prefer an OpenID
Connect provider, or have users keep a password or a passkey, when this matters.

## Providers on an internal network

The server reaches providers through its outbound HTTP guard, which refuses loopback and private addresses. To use a
provider on your own network, such as a Keycloak next to the server, list its host or address range in
[`login_provider_allowed_destinations`](../reference/configuration.md#login_provider_allowed_destinations).
