# db-external

A PostgreSQL outside this configuration: Neon, or any other. It creates nothing, and follows
[the backend contract](../../README.md#the-backend-contract).

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/db-external?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `url_secret_id` | required | Secret that holds the direct connection URL, which the operator created by hand: its ID in Secret Manager, its ARN in AWS Secrets Manager |
| `pooled_url_secret_id` | null | Secret that holds the URL the service uses instead, such as Neon's pooled one |

| Output | Value |
| --- | --- |
| `env` | `MY_DATABASE` |
| `secret_env` | `MY_DATABASE_URL`: the pooled secret when there is one, with the direct one as `MY_STREAM_POSTGRES_URL`. Otherwise the direct one |
| `direct_secret_env` | With a pooled secret, `MY_DATABASE_URL` as the direct one, for the migration job and the console |
| `permissions` | Empty: the app module grants access to every secret it is given |

`url_secret_id` must not go through a pooler in transaction mode. `my schema apply` holds a session-level lock, and through such a
pooler two runs at once hang, so use Neon's direct connection string (the host without `-pooler`). With `stream = pubsub` the server needs no `LISTEN` connection.

`pooled_url_secret_id` may go through one, for the service alone, as long as the pooler handles protocol-level prepared statements:
[Through a connection pooler](../../../../docs/operations/database.md#through-a-connection-pooler) has what that takes. Neon's pooled
string does. It also takes `pool_max_conns`, which the direct one must not hold: `my schema apply` passes parameters it does not know to
PostgreSQL, which refuses them.

In practice, changing this block for another database is not a one-block swap: a new database is empty, so it is a data migration,
which [`db-gcp-cloud-sql`](../db-gcp-cloud-sql/README.md#coming-from-db-external) describes.

If the role of the URL cannot create a schema, pass `MY_DATABASE_DEV_URL` through the app module's `secret_env`.
