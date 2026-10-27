# External login providers

This guide is for operators and administrators.

An external login provider lets users sign in with an account they have elsewhere: an OpenID Connect provider
(Google Workspace, Microsoft Entra ID, Keycloak, and the like), GitHub, or one Mastodon server.
You set providers up with the `my login-provider` command on a host that has the server's configuration, or through the
admin API (`/api/v1/admin/login_providers`) with the `manage_login_providers` permission, which only the built-in
`admin` role has. The command works before any administrator can sign in, so you can use it for the first setup.

## Kinds

| Kind | `-issuer` | Conditions |
| --- | --- | --- |
| `oidc` | The issuer URL (`https`), exactly as the provider's ID tokens name it, such as `https://accounts.google.com` | `-claim <claim>=<value>`: only users whose ID token has the claim with one of the values given for it |
| `github` | None: the server knows github.com's endpoints | `-org <organization>`: only members of one of the Organizations (add `-scope read:org`) |
| `mastodon` | The server's web domain, such as `mastodon.example` (Mastodon 4.3 or later) | None |

For Google Workspace, limit the users to your organization with `-claim hd=<your domain>`. Do not rely on the domain of the
email address.

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

Every change is recorded in the audit log under the provider's display name, as the CLI or as the administrator who made
it through the admin API.

## Providers on an internal network

The server reaches providers through its outbound HTTP guard, which refuses loopback and private addresses. To use a
provider on your own network, such as a Keycloak next to the server, list its host or address range in
[`login_provider_allowed_destinations`](../reference/configuration.md#login_provider_allowed_destinations).
