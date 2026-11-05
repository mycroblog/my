# stream-postgres

The broker of the real-time updates on PostgreSQL `LISTEN` / `NOTIFY`, on the database the server already has. It creates nothing,
needs `database = postgres`, and follows [the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** It has no provider and its tests only check the settings it returns. The server's adapter has
its own tests against PostgreSQL. Unverified: all of it against Neon, and in particular what is listed as not stated below.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "stream" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/stream-postgres?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `listen_url_secret_id` | `null` | Secret that holds a direct connection string to the same database, only when no `db-*` module supplies one. Unset, the `db-*` module's direct connection is used if it returns one, and otherwise the URL of the database |

| Output | Value |
| --- | --- |
| `env` | `MY_STREAM` |
| `secret_env` | `MY_STREAM_POSTGRES_URL`, when it is set |
| `permissions` | Empty: the app module grants access to every secret it is given |

Each process with open streams holds one connection for `LISTEN`, outside its pool, so count it against the database's limit. A
process with no open stream holds none.

## The pooler

`LISTEN` belongs to a session, so it does not work through a pooler in transaction mode: PgBouncer's transaction pooling, Neon's
pooled connection string, Supabase's port 6543. Neon's [connection pooling page](https://neon.com/docs/connect/connection-pooling)
lists `LISTEN` / `NOTIFY` as not supported through it. On Neon the direct string is the same one with no `-pooler` in the host name.

`MY_STREAM_POSTGRES_URL` has one owner, and this module is the last choice for it:

1. **A `db-*` module that returns a pooled URL and a direct connection** sets `MY_STREAM_POSTGRES_URL` from the direct one. Leave
   `listen_url_secret_id` unset: this module then names only `MY_STREAM`, and nothing is named twice. Setting it as well fails the plan as a duplicate.
2. **A `db-*` module whose URL is already direct** needs nothing: the listeners use that URL. Leave it unset.
3. **A database URL through a pooler, with nothing naming a direct one**: set `listen_url_secret_id` to a direct one.

## Neon's scale to zero

As read on 2026-11-05:

- A compute scales to zero after 5 minutes of inactivity, and paid plans can turn that off; [the Free plan cannot](https://neon.com/docs/introduction/scale-to-zero).
- [Neon's guide to `LISTEN` / `NOTIFY`](https://neon.com/guides/pub-sub-listen-notify) says scaling to zero "will terminate all
  listeners", that a message sent with no listener disappears, and advises turning scale to zero off.
- **Not stated**: whether an idle listening connection counts as activity and keeps the compute awake. None of Neon's pages read says
  either way. This project's design document assumes it does not, and nothing here has measured it. If it does, an open stream keeps
  the compute billed.

What that means for the server: when its listener is lost, the process ends every stream it holds, and the clients connect again,
which opens a new listener and wakes the compute. So a compute that suspends under an open stream costs those clients a reconnection,
and signals sent in between are lost, as they are for any stream that is reconnecting. A process with no open stream does not listen,
so the listener does not by itself stop the compute from scaling to zero while nobody is connected.

On the Free plan you cannot turn scale to zero off, so expect this. Where real-time updates must not drop, turn it off on a paid plan,
which gives up what scale to zero saves, or use [`stream-gcp-pubsub`](../stream-gcp-pubsub/README.md), which has no such link to the database.
