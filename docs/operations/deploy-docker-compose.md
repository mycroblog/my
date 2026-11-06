# Deploying with Docker Compose

How to run `my` on one machine with Docker Compose: the smallest setup, one server process on SQLite with media files
in a named volume. PostgreSQL, object storage, and TLS are files to layer on top later
([Docker Compose files](running.md#docker-compose-files)).

You need Docker and Docker Compose v2.

## Start the server

The Compose files are in [`deploy/docker`](../../deploy/docker/compose.yaml) of the source. Take them from the tag of
the release you run, so that they match its image:

```sh
git clone --depth 1 --branch v0.1.0 https://github.com/mycroblog/my.git
cd my/deploy/docker
cp example.env .env
# Set MY_VERSION in .env to the same version, such as 0.1.0
mkdir -m 700 secrets
(umask 022; openssl rand -base64 32 > secrets/kek)   # the key encryption key: back it up apart from the database
docker compose up -d
```

- Compose pulls the image `ghcr.io/mycroblog/my` of `MY_VERSION`. There is no `latest` tag, so the version has to be named:
  a release can need steps that a silent update would skip ([the image's tags](running.md#get-the-image)).
  To build the image from the checkout instead, add the override: `docker compose -f compose.yaml -f compose.build.yaml up -d --build`.
- The `schema` service runs `my schema apply` of the server image, which brings the database in a named volume to the
  schema of that release, and exits; the server (`role=all`) starts once it has succeeded. The schema is applied again
  on every `up`, so after an upgrade you start the server the same way.
- The server listens on `127.0.0.1:8080` (the port is `MY_HTTP_PORT` in `.env`). It is ready when
  `curl localhost:8080/readyz` returns `{"status":"ok"}`.
- The key encryption key (KEK) reaches the server from `secrets/kek` as the Docker secret `/run/secrets/my_kek`, never
  as an environment variable ([managing the key encryption key](key-management.md)).
- Every other setting of the [configuration reference](../reference/configuration.md) is set in `.env`, whose template
  describes them.

## Create an administrator

Create an administrator, then look the account up with WebFinger and fetch its actor. The password is read from the
first line of standard input.

```sh
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
curl 'http://localhost:8080/.well-known/webfinger?resource=acct:alice@localhost:8080'
curl -H 'Accept: application/activity+json' http://localhost:8080/ap/actors/<the UUID of the self link from WebFinger>
```

## Sign in from a browser

With the pair `MY_HOST=localhost:8080` and `MY_WEB_ORIGIN=http://localhost:8080` of `example.env`, sign-in, the
client API, and media work over plain HTTP at `http://localhost:8080` (ActivityPub ids stay
`https://localhost:8080/...`). If you change `MY_HTTP_PORT`, change the port of `MY_HOST` and `MY_WEB_ORIGIN` to match.

1. Open `http://localhost:8080/login` in a browser (or press **Sign in** in the web client's top bar); it opens the server's sign-in screen.
2. On the sign-in screen, enter the email address and password of the administrator you created above.
3. You are back in the web client, on your home timeline.

- The sign-in cookie is `Secure`. Chrome and Firefox accept it on `http://localhost`; Safari (WebKit) does not, so use
  Chrome or Firefox locally.
- Links of hashtags and mentions in post bodies use `https://`, since they are the federated URLs.

## Going public

- Set `MY_HOST` in `.env` to your real domain, remove `MY_WEB_ORIGIN` (it defaults to `https://{MY_HOST}`), and terminate
  TLS in front of the server: the layer `compose.caddy.yaml` runs Caddy for that
  ([Docker Compose files](running.md#docker-compose-files)). `MY_HOST` is stored in the database on first
  start and cannot be changed later.
- Behind a reverse proxy of your own, set `MY_TRUSTED_PROXIES` in `.env` so that rate limits such as the one on sign-in
  apply per client IP rather than to the proxy ([`trusted_proxies`](../reference/configuration.md#trusted_proxies)).
- Before opening the server to users, go through the [production checklist](production-checklist.md).
- `docker compose down` stops the stack. `down -v` also deletes the data volumes.

For every setting see the [configuration reference](../reference/configuration.md); for day-to-day operation see
[running the server](running.md).
