# db-external

A PostgreSQL outside this configuration: Neon, or any other. It creates nothing, and follows
[the backend contract](../../README.md#the-backend-contract).

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/db-external?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Meaning |
| --- | --- |
| `url_secret_id` | Secret in Secret Manager that holds the connection URL, which the operator created by hand |

| Output | Value |
| --- | --- |
| `env` | `MY_DATABASE` |
| `secret_env` | `MY_DATABASE_URL` |
| `roles` | Empty: the app module grants access to every secret it is given |

The URL must not go through a pooler in transaction mode. `my schema apply` holds a session-level lock, so use Neon's
direct connection string (the host without `-pooler`). With `stream = pubsub` the server needs no `LISTEN` connection.

If the role of the URL cannot create a schema, pass `MY_DATABASE_DEV_URL` through the app module's `secret_env`.
