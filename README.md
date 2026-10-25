# my

An ActivityPub-compatible microblog server written in Go. It aims to run at as close to zero cost as possible on
Google Cloud, AWS, or your own machine (Docker, microk8s), and is open source.

> **Status: in development, before the first release (v0).** Data may not survive an upgrade before `v0.1.0`: the
> database may have to be recreated. From `v0.1.0` on, every upgrade between releases keeps your data.

## Goals

- Run from about $0–$5 a month: the server scales to zero and needs no always-on middleware
- Switch the database, job queue, storage, and runtime by configuration
- Federate with Mastodon, Misskey, GoToSocial, and other ActivityPub servers
- Speak many languages from day one
- A web client (PWA) and mobile apps

## Quick start (Docker Compose)

With Docker and Docker Compose v2, start the smallest setup, backed by SQLite
([`deploy/docker/compose.yaml`](deploy/docker/compose.yaml)):

```sh
cd deploy/docker
cp .env.example .env
# Set MY_KEK in .env to the output of `openssl rand -base64 32` (and back it up apart from the database)
docker compose up -d --build
```

- The `schema` service applies the schema of `schema/sqlite/` to the database in a named volume with Atlas Community
  Edition and exits; the server (`role=all`) starts once it has succeeded. The schema is applied again on every `up`,
  so after an upgrade you start the server the same way.
- The server listens on `127.0.0.1:8080` (the port is `MY_HTTP_PORT` in `.env`). It is ready when
  `curl localhost:8080/readyz` returns `{"status":"ok"}`.
- The key encryption key (KEK) reaches the server as the Docker secret `/run/secrets/my_kek`, never as an environment
  variable ([managing the key encryption key](docs/operations/key-management.md)).

Create an administrator, then look the account up with WebFinger and fetch its actor. The password is read from the
first line of standard input.

```sh
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
curl 'http://localhost:8080/.well-known/webfinger?resource=acct:alice@localhost:8080'
curl -H 'Accept: application/activity+json' http://localhost:8080/ap/actors/<the UUID of the self link from WebFinger>
```

### Sign in from a browser

With the pair `MY_HOST=localhost:8080` and `MY_WEB_ORIGIN=http://localhost:8080` of `.env.example`, sign-in, the
client API, and media work over plain HTTP at `http://localhost:8080` (ActivityPub ids stay
`https://localhost:8080/...`). If you change `MY_HTTP_PORT`, change the port of `MY_HOST` and `MY_WEB_ORIGIN` to match.

1. Open `http://localhost:8080/login` in a browser and press **Continue to sign in**.
2. On the server's sign-in screen, enter the email address and password of the administrator you created above.
3. You are back in the web client, on your home timeline.

- The sign-in cookie is `Secure`. Chrome and Firefox accept it on `http://localhost`; Safari (WebKit) does not, so use
  Chrome or Firefox locally.
- Links of hashtags and mentions in post bodies use `https://`, since they are the federated URLs.

### Going public

- Set `MY_HOST` in `.env` to your real domain, remove `MY_WEB_ORIGIN` (it defaults to `https://{MY_HOST}`), and put
  a reverse proxy that terminates TLS (Caddy, for example) in front of the server. `MY_HOST` is stored in the database
  on first start and cannot be changed later.
- Behind a reverse proxy, set `MY_TRUSTED_PROXIES` in `.env` so that rate limits such as the one on sign-in apply per
  client IP rather than to the proxy ([`trusted_proxies`](docs/reference/configuration.md#trusted_proxies)).
- `docker compose down` stops the stack. `down -v` also deletes the data volumes.

For every setting see the [configuration reference](docs/reference/configuration.md); for day-to-day operation see
[running the server](docs/operations/running.md).

## Documentation

- [Documentation index](docs/README.md): operation guides and references
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md): report vulnerabilities privately, never in a public issue

## License

[GNU Affero General Public License v3.0](LICENSE), with one exception: the following are under the MIT License, so
that anyone can use the client API specification and the client libraries generated from it without the AGPL's
conditions.

- `api/spec/` and its bundle `api/openapi.yaml`
- `pkg/mykit/` (the API client library)
- `pkg/mybot/` (the bot library)

Everything else (the server, the web client, and the code they generate from `api/spec/`) is AGPL-3.0-only.
