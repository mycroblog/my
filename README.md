<div align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="app/docs/src/assets/wordmark-dark.svg">
    <img src="app/docs/src/assets/wordmark-light.svg" alt="mycroblog" width="320">
  </picture>
</div>

<h1 align="center">mycroblog</h1>

<p align="center">
  <strong>mycroblog</strong> — a microblog for the fediverse that starts small and grows with your community.
</p>

<p align="center">
  <code>my</code>, the mycroblog server, speaks ActivityPub and is written in Go. Start on one small machine or a
  scale-to-zero container with SQLite. When your community grows, move to PostgreSQL, a job queue, a stream broker and
  separate web and worker processes: same code, same data.
</p>

<p align="center">
  <a href="https://github.com/mycroblog/my/actions/workflows/ci.yml?query=event%3Amerge_group"><img src="https://github.com/mycroblog/my/actions/workflows/ci.yml/badge.svg?event=merge_group" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/mycroblog/my" alt="License: AGPL-3.0"></a>
  <a href="https://github.com/mycroblog/my/releases"><img src="https://img.shields.io/github/v/release/mycroblog/my?include_prereleases&sort=semver" alt="Latest release"></a>
  <a href="https://github.com/mycroblog/my/pkgs/container/my"><img src="https://img.shields.io/badge/ghcr.io-mycroblog%2Fmy-blue?logo=docker&logoColor=white" alt="Container image: ghcr.io/mycroblog/my"></a>
</p>

<h3 align="center">
  <a href="https://mycroblog.dev/latest/">Read the documentation at mycroblog.dev</a>
</h3>

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="app/docs/src/assets/screenshots/home-dark.webp">
    <img src="app/docs/src/assets/screenshots/home-light.webp" alt="The home timeline of mycroblog: a post quoting two photos of a lake and a sunset, with custom emoji reactions, then a post in Japanese about bread, beside the navigation, the latest notifications and the server's information.">
  </picture>
</p>

## Features

<table>
  <tr>
    <td width="50%" valign="top">
      <strong>Start small, scale out</strong><br>
      One binary and SQLite on a $5 VM, or a container that scales to zero. Grow into PostgreSQL, Cloud Tasks, Valkey,
      Pub/Sub or SNS, S3 or GCS behind a CDN, and as many web and worker processes as you need.
    </td>
    <td width="50%" valign="top">
      <strong>Federates</strong><br>
      Follows, posts, quote posts and custom emoji reactions travel to and from Mastodon, Misskey, GoToSocial and more.
      The "This server" visibility keeps a post within your server and your followers.
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <strong>Supporters and open books</strong><br>
      Paid plans through Stripe or PAY.JP that grant roles, badges and the right to add custom emoji. Record what the
      server costs, import AWS and Google Cloud bills automatically, and publish costs and income to your community.
    </td>
    <td width="50%" valign="top">
      <strong>Voice rooms</strong><br>
      Live audio rooms for the people of your server, with hosts, speakers, invitations and reactions. Peer to peer for
      small rooms, LiveKit or Sora for large ones.
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <strong>A web app for everyone</strong><br>
      An installable web app with push notifications and passkeys, translated from day one.
    </td>
    <td width="50%" valign="top">
      <strong>Moderation built in</strong><br>
      Reports with appeals, domain blocks, an audit log, and decision hooks that let your own program review posts and
      sign-ups before they are accepted.
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <strong>Extend it</strong><br>
      Plugins over a versioned protocol, server webhooks, OAuth apps, and client and bot libraries for Go and TypeScript.
      Translation of posts runs through a plugin of your choice.
    </td>
    <td width="50%" valign="top"></td>
  </tr>
</table>

## Quick start (Docker Compose)

With Docker and Docker Compose v2, start the smallest setup, backed by SQLite
([`deploy/docker/compose.yaml`](deploy/docker/compose.yaml)):

```sh
cd deploy/docker
cp example.env .env
# Set MY_VERSION in .env to a version from the releases page, such as 0.1.0
mkdir -m 700 secrets
(umask 022; openssl rand -base64 32 > secrets/kek)   # the key encryption key: back it up apart from the database
docker compose up -d
```

- Compose pulls the image `ghcr.io/mycroblog/my` of `MY_VERSION`. There is no `latest` tag, so the version has to be named:
  a release can need steps that a silent update would skip ([the image's tags](docs/operations/running.md#get-the-image)).
  To build the image from this checkout instead, add the override: `docker compose -f compose.yaml -f compose.build.yaml up -d --build`.
- The `schema` service runs `my schema apply` of the server image, which brings the database in a named volume to the
  schema of that release, and exits; the server (`role=all`) starts once it has succeeded. The schema is applied again
  on every `up`, so after an upgrade you start the server the same way.
- The server listens on `127.0.0.1:8080` (the port is `MY_HTTP_PORT` in `.env`). It is ready when
  `curl localhost:8080/readyz` returns `{"status":"ok"}`.
- The key encryption key (KEK) reaches the server from `secrets/kek` as the Docker secret `/run/secrets/my_kek`, never
  as an environment variable ([managing the key encryption key](docs/operations/key-management.md)).
- Every other setting of the [configuration reference](docs/reference/configuration.md) is set in `.env`, whose template
  describes them. PostgreSQL, object storage, and TLS are files to layer on top
  ([Docker Compose files](docs/operations/running.md#docker-compose-files)).

Create an administrator, then look the account up with WebFinger and fetch its actor. The password is read from the
first line of standard input.

```sh
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
curl 'http://localhost:8080/.well-known/webfinger?resource=acct:alice@localhost:8080'
curl -H 'Accept: application/activity+json' http://localhost:8080/ap/actors/<the UUID of the self link from WebFinger>
```

### Sign in from a browser

With the pair `MY_HOST=localhost:8080` and `MY_WEB_ORIGIN=http://localhost:8080` of `example.env`, sign-in, the
client API, and media work over plain HTTP at `http://localhost:8080` (ActivityPub ids stay
`https://localhost:8080/...`). If you change `MY_HTTP_PORT`, change the port of `MY_HOST` and `MY_WEB_ORIGIN` to match.

1. Open `http://localhost:8080/login` in a browser (or press **Sign in** in the web client's top bar); it opens the server's sign-in screen.
2. On the sign-in screen, enter the email address and password of the administrator you created above.
3. You are back in the web client, on your home timeline.

- The sign-in cookie is `Secure`. Chrome and Firefox accept it on `http://localhost`; Safari (WebKit) does not, so use
  Chrome or Firefox locally.
- Links of hashtags and mentions in post bodies use `https://`, since they are the federated URLs.

### Going public

- Set `MY_HOST` in `.env` to your real domain, remove `MY_WEB_ORIGIN` (it defaults to `https://{MY_HOST}`), and terminate
  TLS in front of the server: the layer `compose.caddy.yaml` runs Caddy for that
  ([Docker Compose files](docs/operations/running.md#docker-compose-files)). `MY_HOST` is stored in the database on first
  start and cannot be changed later.
- Behind a reverse proxy of your own, set `MY_TRUSTED_PROXIES` in `.env` so that rate limits such as the one on sign-in
  apply per client IP rather than to the proxy ([`trusted_proxies`](docs/reference/configuration.md#trusted_proxies)).
- `docker compose down` stops the stack. `down -v` also deletes the data volumes.

For every setting see the [configuration reference](docs/reference/configuration.md); for day-to-day operation see
[running the server](docs/operations/running.md).

## Documentation

The documentation lives at **[mycroblog.dev](https://mycroblog.dev/latest/)**, one version per release (`/latest/` for
the newest, `/v0.1/` and the like) and `/next/` for the development version.

- **Run a server**: [running the server](https://mycroblog.dev/latest/operations/running/),
  [deploying on Cloud Run](https://mycroblog.dev/latest/operations/deploy-cloud-run/), and the
  [production checklist](https://mycroblog.dev/latest/operations/production-checklist/)
- **Use mycroblog**: [signing up and signing in](https://mycroblog.dev/latest/guide/signing-in/),
  [posting](https://mycroblog.dev/latest/guide/posting/), [following people](https://mycroblog.dev/latest/guide/following/),
  and [voice chat](https://mycroblog.dev/latest/guide/voice-chat/)
- **Build on mycroblog**: [the client API](https://mycroblog.dev/latest/developers/client-api/), the
  [API reference](https://mycroblog.dev/latest/api/), and the [client libraries](https://mycroblog.dev/latest/developers/libraries/)
- **Reference**: the [configuration reference](https://mycroblog.dev/latest/reference/configuration/)

In this repository:

- [Documentation index](docs/README.md): the same guides and references, as they are on the development version
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md): report vulnerabilities privately, never in a public issue

> **Status: in development, before the first release (v0).** Data may not survive an upgrade before `v0.1.0`: the
> database may have to be recreated. From `v0.1.0` on, every upgrade between releases keeps your data.

## License

[GNU Affero General Public License v3.0](LICENSE), with one exception: the following are under the MIT License, so
that anyone can use the client API specification and the client libraries generated from it without the AGPL's
conditions.

- `api/spec/` and its bundle `api/openapi.yaml`
- `pkg/mykit/` (the API client library)
- `pkg/mybot/` (the bot library)

`api/plugin/`, the definitions of the plugin protocol, is under the MIT License too. A plugin runs as a separate process
and talks to the server only through that protocol, so the AGPL does not extend to it, and it may have any license.

Everything else (the server, the web client, and the code they generate from `api/spec/` and `api/plugin/`) is
AGPL-3.0-only.
