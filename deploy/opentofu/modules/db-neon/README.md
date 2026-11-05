# db-neon

PostgreSQL on Neon, with the project created by OpenTofu instead of by hand in Neon's console. It follows
[the backend contract](../../README.md#the-backend-contract), with the exception below.

**The provider this module uses is unmaintained.** `kislerdm/neon` v0.18.0 is on the OpenTofu registry, but its repository was archived
on 2026-09-11. Its README says that Neon will maintain it as `neondatabase/neon`, which is not on that registry yet, and Neon's own
documents still point to `kislerdm/neon` and say that it is not officially supported. So this module **may stop working when Neon's
API changes**, and no one is there to fix it. When the new source exists, changing it is one line, in `required_providers` of
[`main.tf`](main.tf).

**The database password lands in the state.** This is the exception to the rule that no secret value reaches the state, and the
trade-off of this module against [`db-external`](../db-external/README.md), which keeps it out. Read [What is in the
state](#what-is-in-the-state).

**This module has never been applied.** The tests mock the provider, and nothing here has called Neon's API. Unverified:

- that a project with these settings is created, on any plan;
- that the plan's refresh and the write-only hand-over work against the real provider. The hand-over was applied against a stub
  of Secret Manager with a value that was unknown at plan time, from another provider;
- that `my schema apply` and the server work on the project. The server's suites ran through PgBouncer 1.25 in transaction mode, in a
  container, with `max_prepared_statements` as Neon documents it, not through Neon's own.

## Source

Pin it to the release you deploy, like every module of the deployment. The key comes from the environment, `NEON_API_KEY`, and never
from an input: an input would put it in the state too.

A secrets module of your platform creates the secrets and writes the URLs into them:

```hcl
module "secrets" {
  source     = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/secrets-gcp-secret-manager?ref=v0.1.0&depth=1"
  project_id = var.project_id
  referenced = { database_url = "my-database-url", database_url_pooled = "my-database-url-pooled" }
  write_keys = ["database_url", "database_url_pooled"]
  write_values = {
    database_url        = module.db.connection_uri
    database_url_pooled = module.db.connection_uri_pooler
  }
}

module "db" {
  source               = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/db-neon?ref=v0.1.0&depth=1"
  org_id               = var.neon_org_id
  region_id            = "aws-ap-southeast-1"
  url_secret_id        = module.secrets.ids["database_url"]
  pooled_url_secret_id = module.secrets.ids["database_url_pooled"]
}
```

On AWS, use `secrets-aws-secrets-manager`, whose `ids` are ARNs. This is why the module takes no secret store of its own: one block
that could write to either would need both providers configured, on every deployment.

| Input | Default | Meaning |
| --- | --- | --- |
| `org_id` | required | Neon organization. Neon warns that leaving it out can create the project in the wrong one, and later plans may try destructive changes |
| `region_id` | required | Neon region. It cannot change |
| `url_secret_id` | required | Secret for `connection_uri`: its ID in Secret Manager, its ARN in AWS Secrets Manager |
| `pooled_url_secret_id` | null | Secret for `connection_uri_pooler`. Set, the service uses it |
| `name` | `my` | Name of the project |
| `pg_version` | `18` | PostgreSQL version. It cannot change without a new project |
| `history_retention_seconds` | `21600` | How long Neon keeps the history to restore from. This is the Free plan's limit; the provider's own default of a day is above it. Raise it on a paid plan |
| `suspend_timeout_seconds` | null | Idle seconds before the compute stops. Paid plans only: null sends nothing, and the Free plan then stops it after 5 minutes |

| Output | Value |
| --- | --- |
| `env` | `MY_DATABASE` |
| `secret_env` | `MY_DATABASE_URL`: the pooled secret when there is one, with the direct one as `MY_STREAM_POSTGRES_URL`. Otherwise the direct one |
| `direct_secret_env` | With a pooled secret, `MY_DATABASE_URL` as the direct one, for the migration job and the console |
| `permissions` | Empty: the app module grants access to every secret it is given |
| `connection_uri`, `connection_uri_pooler` | Sensitive. The URLs, for `write_values` |
| `project_id` | The project |

## What is in the state

The provider has no write-only attribute and no ephemeral resource, and Neon generates the role's password itself. So the state holds,
in plain text, in `neon_project`:

- `database_password`, the password of the project's owner role;
- `connection_uri`, the direct URL, with that password;
- `connection_uri_pooler`, the pooled URL, with that password.

Anyone who can read the state can do anything to the database. These are sensitive, so they stay out of what `tofu` prints, which
is all that means: **keep the state private, and use [OpenTofu's state encryption](https://opentofu.org/docs/language/state/encryption/)**
if your backend is shared. What does stay out of the state is the copy in your secret store: this module's outputs go through an
ephemeral input and a write-only attribute, so the secrets module holds nothing of them.

There was no better path:

- **`store_password = "no"` does not help.** The provider asks Neon for the password when it creates the project and on every refresh,
  so with nothing stored at Neon it would fail rather than leave it out. This comes from reading the provider's source.
- **A role of its own does not help.** `neon_role.password` is generated by Neon and computed, so it lands in the state too, and the
  owner's is there anyway. So does the data source `neon_branch_role_password`.
- **Resetting the password in the console after the apply does not help.** The next refresh reads the new one into the state.

If this is not acceptable, create the project by hand and use `db-external`.

## The connection

`connection_uri` goes to the endpoint's own host, and `connection_uri_pooler` to the same host with `-pooler`, which is PgBouncer in
transaction mode at Neon. `my schema apply` holds a session-level lock, so **the migration job and the console always use the direct
one**, and the service uses the pooled one when `pooled_url_secret_id` is set, which is what to do unless you have a reason not to:
[Through a connection pooler](../../../../docs/operations/database.md#through-a-connection-pooler) has what the server needs of it.

The secrets do not follow a change at Neon by themselves. If the password or the host changes there, raise `write_version` of the
secrets module, which writes both again, and deploy.

To set `pool_max_conns`, add it to the pooled URL where you hand it over:
`database_url_pooled = "${module.db.connection_uri_pooler}&pool_max_conns=8"`. Never add it to the direct one: `my schema apply` passes
parameters it does not know to PostgreSQL, which refuses them.

## Coming from db-external

**In practice this is not a one-block swap.** The `db` block changes and `secrets` gains its write lines, but the new project is empty,
and everything your instance knows is in the other database. Moving means, in this order: [the maintenance
mode](../../../../docs/operations/maintenance.md), `pg_dump` from the old database, `pg_restore` to the new project's direct URL, then
swapping the blocks and applying, then leaving the maintenance mode. If you want the database you already have to stay, leave it on
`db-external`, or `tofu import` the project: the provider's import exists, and no one has tried it here.

## Removing it

`prevent_destroy` stops any plan that would delete the project. Neon's project holds the database and its history together, so deleting it
leaves nothing to restore from. Take your own dump first.

## What it costs

Nothing on Neon's Free plan: $0 a month, with 100 CU-hours and 1 GB per project, a 6-hour history, and a compute that stops after 5 minutes
idle ([Neon's pricing page](https://neon.com/pricing), 2026-11-05). The paid plans have no monthly minimum and bill by use. **The server's
tick decides which of these you are on**: read [the guide's costs](../../../../docs/operations/deploy-cloud-run.md#costs).
