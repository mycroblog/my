# kv-gcp-memorystore

The key-value store on Memorystore for Valkey, on a network you give it. It follows
[the backend contract](../../README.md#the-backend-contract). It bills while the server is idle: see [Cost](#cost). `kv = db` is the default and needs no
module. Use this when you want caches and rate limits off the database.

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that
Google Cloud accepts it. Unverified:

- that this combination is accepted: `SHARED_CORE_NANO`, `CLUSTER_DISABLED`, no replica, and TLS on;
- that the connection type the URL reads is `CONNECTION_TYPE_PRIMARY` with the cluster mode off. The networking guide speaks only of a
  discovery endpoint, so the module falls back to that one;
- that the server's client accepts the instance's certificate when it connects by IP address;
- that Cloud Run reaches the instance from a subnet other than the policy's, which [`network-gcp-vpc`](../network-gcp-vpc/README.md#what-it-creates) has it do. The guide speaks of clients "in this network", and the two subnets avoid the question of whether one could serve both;
- how long a cold start takes: see [below](#what-the-private-network-adds);
- what happens to the CA secret when the CA changes. A later apply writes a new version, and nothing here has seen it happen.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "kv" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kv-gcp-memorystore?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
  network    = module.network
}

module "app" {
  # ...
  network = module.network
  backends = [
    # ...
    module.kv,
  ]
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id`, `region` | required | Where everything goes. Use the app's region |
| `network` | required | A network from [`network-gcp-vpc`](../network-gcp-vpc/README.md), or an object of [its shape](../../README.md#the-network-on-google-cloud) with `memorystore_connection_policy` true. The app must take the same one |
| `name` | `my-valkey` | The instance, and `<name>-ca` |
| `node_type`, `replica_count` | `SHARED_CORE_NANO`, `0` | Size of the instance. Each replica is billed as a node |
| `deletion_protection` | `true` | Refuses to delete the instance |

| Output | Value |
| --- | --- |
| `env` | `MY_KV`, `MY_VALKEY_URL` |
| `secret_env` | Empty |
| `secret_files` | `MY_VALKEY_TLS_CA_FILE`, the instance's CA |
| `permissions` | Empty |
| `private_network` | The network, through `backends`, which `app-gcp-cloud-run` must have too, or the plan fails |

The project needs the APIs of Memorystore, Network Connectivity, Service Consumer Management, Compute Engine, and Secret Manager.

The instance has one shard, because the adapters keep each data set in one hash slot, so more shards would hold nothing more of it.

## No password: the network is what keeps others out

Authorization is off (`AUTH_DISABLED`), and the URL holds no secret, so it goes in `env`. **Anything that can reach the address on
this network can read and write the store.** That is the deliberate answer for this module, and it holds on these conditions:

- **The network holds nothing you do not trust with the store.** With `network-gcp-vpc` that is the Cloud Run service and its jobs,
  unless someone attaches something else, which takes permissions on the project that already reach much more. A VPC you share with
  other workloads gives each of them the store: use a VPC of its own, or `kv-external` with a password.
- **A remote server cannot reach it through the server**: outbound HTTP refuses private addresses, as
  [The network contract](../../README.md#what-stays-closed) says.
- **What the store holds is caches, rate limits, and real-time updates.** The database's data and the secrets are elsewhere.

What this gives up against IAM authorization is a second lock for whoever is already on the network, and a record of who connected.
Memorystore's IAM authorization would take a short-lived token in `AUTH` on each new connection, an hour by default, and the server
takes one static URL. Nothing here calls for it. Token authorization exists only in the `google-beta` provider, which these modules do
not use. The transport is encrypted (`SERVER_AUTHENTICATION`). The instance's CA is a public certificate that the provider returns,
so it is written to a secret with a plain `secret_data` and reaches the state, which is harmless: it is only in the secret store
because that is what the app mounts files from.

## What the private network adds

[The network](../network-gcp-vpc/README.md) holds the service connection policy of the class `gcp-memorystore`, which Memorystore
requires before an instance exists. The app adds Direct VPC egress through its own `network`, on the service and the jobs.

- **Money**: Direct VPC egress has no connector to keep running. Cloud Run's guide says only that "network costs scale to zero just
  like the service itself" and lists no price. Whether Private Service Connect adds a charge could not be read.
- **Cold starts**: Cloud Run's guide warns of "connection establishment delays of a minute or more on instance startup", and the
  server stops at start when it cannot reach the store. So a new instance may fail and restart, and a deployment that scales to zero
  starts instances often. Nobody has measured this: try it before relying on it.
- **Addresses**: each instance of the service takes four, and each job task one for its run plus seven minutes, in the subnet of the
  service and the jobs. A `/24` is far more than the default `max_instances` needs.

## Cost

About $0.0318 per hour for the smallest node, so **about $23 a month, whether or not anyone uses the server**. That figure comes from
this project's design documents, checked on 2026-10-19; Google's pricing page returned no content when read on 2026-11-05, so check
[it](https://cloud.google.com/memorystore/valkey/pricing) yourself. Cloud Run scales to zero and this does not.
