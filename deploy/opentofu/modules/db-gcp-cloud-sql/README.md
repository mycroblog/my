# db-gcp-cloud-sql

PostgreSQL on Cloud SQL, reached from Cloud Run through the socket of its connector, or on a private address when you give it a network. It creates the instance, the database, the
user, and the secret that holds the connection URL, and follows [the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** The tests mock the provider, and a test applies it against a local stub of the APIs, so
they show what OpenTofu and the provider do and nothing of what Cloud SQL does. Unverified:

- that an instance with these settings is created: `POSTGRES_18` with `db-f1-micro` on the Enterprise edition, which Google's
  documents neither confirm nor rule out;
- that Cloud Run connects through the socket with `ssl_mode = ENCRYPTED_ONLY` and this CA mode, and that `my schema apply` and the
  server then work. The URL's form was run against a real PostgreSQL over a Unix socket, not through the connector;
- that the user this module creates may create a schema, which `my schema apply` needs. Google's documents say it gets the role
  `cloudsqlsuperuser` and do not mention schemas;
- that a backup restores;
- with a network: that Cloud Run reaches the private address through Direct VPC egress, that `sslmode=require` is accepted with `ssl_mode = ENCRYPTED_ONLY` over it, and that a change of the instance from public to private works in place.

## Source

Pin it to the release you deploy, like every module of the deployment. Enable `sqladmin.googleapis.com` first: Cloud Run's
connector needs it.

```hcl
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/db-gcp-cloud-sql?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id`, `region` | required | Where it lives. Use the region of the service |
| `name` | `my` | Name of the instance. Cloud SQL does not give a name back for a week after deleting it |
| `database`, `username` | `my` | Names in the URL: lower-case letters, digits, and underscores |
| `url_secret_id` | `my-database-url` | Secret this module creates |
| `database_version` | `POSTGRES_18` | PostgreSQL version |
| `tier` | `db-f1-micro` | Machine type. Google's SLA does not cover shared-core types |
| `disk_size_gb` | `10` | Disk at creation. It grows by itself and never shrinks |
| `backups_enabled` | `true` | A daily backup |
| `point_in_time_recovery_enabled` | `true` | Restores to any moment since the oldest backup |
| `retained_backups`, `transaction_log_retention_days` | `7`, `7` | How much to keep |
| `deletion_protection` | `true` | Refuses to delete the instance, in OpenTofu and in Google Cloud. `prevent_destroy` refuses it in any case |
| `password_version` | `1` | Raise it for a new password: read below |
| `url_parameters` | `{}` | More parameters of the URL |
| `network` | `null` | A network from [`network-gcp-vpc`](../network-gcp-vpc/README.md), or an object of [its shape](../../README.md#the-network-on-google-cloud). With one, the instance has a private address alone. Read [The connection](#the-connection) |

| Output | Value |
| --- | --- |
| `env` | `MY_DATABASE` |
| `secret_env` | `MY_DATABASE_URL`, by secret ID |
| `direct_secret_env` | Empty: the only connection is direct |
| `permissions` | `roles/cloudsql.client` on the project, since Cloud SQL has no grant on one instance. Empty with a network |
| `cloud_sql_instances` | The instance, which the app mounts at `/cloudsql` on the service and every job. Empty with a network |
| `private_network` | The network with one, which the app must have too. Otherwise null |
| `connection_name` | For the Cloud SQL Auth Proxy from your own machine, which with a network must run where the private address can be reached |
| `url_without_password` | The URL in the secret, with `PASSWORD` in place of the password |

## The connection

**With no network**, Cloud Run mounts a socket of its connector, so the URL has no host name:

```text
postgres://my:PASSWORD@/my?host=/cloudsql/PROJECT:REGION:INSTANCE
```

The instance has a public address and allows no network to reach it, so only the connector does, with the identity's Cloud SQL
Client role. This needs no VPC.

**With a network**, the instance has a private address alone, from private services access, and the app connects straight to it:

```text
postgres://my:PASSWORD@10.21.0.3:5432/my?sslmode=require
```

The app must take the same network, which the plan checks, and it reaches the address through Direct VPC egress. Nothing goes through the connector, so the app gets neither the `/cloudsql` mount nor the Client role: Google's guide for Cloud Run describes the private path over TCP and the socket under public addresses, and it does not say that the socket works with a private address alone. `sslmode=require` encrypts and does not check the certificate, since the server image does not hold the instance's CA, so what keeps others from answering at that address is the network. Whatever is on the network can try to connect, and the password decides. A `pg_dump` must run from inside the network as well.

`url_without_password` and the URL hold the address, which is known only after the apply. If it ever changes, raise `password_version`, which writes the URL again.

Google limits a Cloud Run instance to 100 connections to Cloud SQL.

**This is a direct connection**, to the database's own port, as `database_url` must be for `my schema apply`, which holds a
session-level lock. There is no pooled connection. Cloud SQL's managed pooling needs the Enterprise Plus edition, which this
module does not create; `max_instances` of the app and `pool_max_conns` bound the connections instead. A pool on `db-f1-micro` must stay small: the instance has 0.6 GB of memory.

To set `pool_max_conns`, name it in `url_parameters`. The jobs read the same URL, and only the service opens a pool with it.

## The password

The first apply generates it, through an ephemeral resource, and hands the same value to the user and to the URL secret, through
write-only attributes. **It reaches neither the plan nor the state**, and `scripts/tofu-db-state-test.sh` shows that against a
stub: the user gets the password that is in the URL, and neither the state nor the plan holds it. Nothing has to be done by hand.

`password_version` moves both at once. Raise it to:

- **change the password**. The running instances keep the URL they started with, and fail to connect once their connections
  close, so deploy again right afterwards;
- **recover from an apply that stopped between the two**. A value is lost once the apply ends, so a second apply that finishes the
  other resource would generate another: the plan shows no difference and the URL does not work. Raise the number and apply again;
- **write the URL again**, after changing `url_parameters`. Nothing else rewrites it, since the state has nothing to compare.

Each raise leaves the previous version of the secret in place, holding a password that no longer works. Destroy it with
`gcloud secrets versions destroy` if you like.

## Coming from db-external

**In practice this is not a one-block swap.** The block changes and the app's plan stays consistent, but the new instance is empty,
and everything your instance knows is in the other database. Moving means, in this order: [the maintenance
mode](../../../../docs/operations/maintenance.md), `pg_dump` from the old database, `pg_restore` to this one through the Cloud SQL
Auth Proxy, from inside the network when it has one, then swapping the block and applying, then leaving the maintenance mode. Run `my db upgrade -check` before you trust
it. Nothing here does this for you, and no one has tried it.

The same goes the other way, and for a move between any two databases.

## Removing it

`prevent_destroy` stops any plan that would delete the instance or the database, and `deletion_protection` stops the API. To really
delete it, take your own dump first, turn off `deletion_protection` and apply, remove `prevent_destroy` from a copy of the module,
and destroy. The automated backups go with the instance.

## What it costs

A fixed cost, whatever the traffic: Cloud SQL never scales to zero, so this is the module's whole trade-off against Neon.

| Item | Price (Iowa, `us-central1`) | Per month |
| --- | --- | --- |
| `db-f1-micro` | $0.0105 per hour | $7.67 |
| 10 GiB of SSD | $0.000232877 per GiB-hour | $1.70 |
| Backups | $0.000109589 per GiB-hour, on what they use | $0.08 per GiB |

That comes to **about $9.40 a month plus the backups**, for 730 hours, from [Google's pricing page](https://cloud.google.com/sql/pricing)
on 2026-11-05. Other regions may cost more. The same page lists "IPv4 addresses while idle" at $0.01 per hour, or $7.30 a month. That
appears to mean an instance that is stopped, which this one never is, but the page does not say: check your first bill.
