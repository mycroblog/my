# kv-gcp-memorystore

The key-value store on Memorystore for Valkey, with the VPC it needs. It follows
[the backend contract](../../README.md#the-backend-contract). **It breaks the near-zero goal**: see [Cost](#cost). `kv = db` needs no
module and costs nothing more, so use this only when the database cannot take the load of caches and rate limits.

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that
Google Cloud accepts it. Unverified:

- that this combination is accepted: `SHARED_CORE_NANO`, `CLUSTER_DISABLED`, no replica, and TLS on;
- that the connection type the URL reads is `CONNECTION_TYPE_PRIMARY` with the cluster mode off. The networking guide speaks only of a
  discovery endpoint, so the module falls back to that one;
- that the server's client accepts the instance's certificate when it connects by IP address;
- that one subnet can serve both Private Service Connect and Direct VPC egress. The guide states no subnet requirement;
- how long a cold start takes: see [below](#what-the-private-network-adds);
- what happens to the CA secret when the CA changes. A later apply writes a new version, and nothing here has seen it happen.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "kv" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kv-gcp-memorystore?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
}

module "app" {
  # ...
  backends = [
    # ...
    module.kv,
  ]
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id`, `region` | required | Where everything goes. Use the app's region |
| `name` | `my-valkey` | The instance, the network, the subnet, the policy, and `<name>-ca` |
| `subnet_cidr` | `10.20.0.0/24` | Range of the subnet: `/26` or larger, which Direct VPC egress needs |
| `node_type`, `replica_count` | `SHARED_CORE_NANO`, `0` | Size of the instance. Each replica is billed as a node |
| `deletion_protection` | `true` | Refuses to delete the instance |

| Output | Value |
| --- | --- |
| `env` | `MY_KV`, `MY_VALKEY_URL` |
| `secret_env` | Empty |
| `secret_files` | `MY_VALKEY_TLS_CA_FILE`, the instance's CA |
| `permissions` | Empty |
| `vpc_access` | `{ network, subnetwork }`, which `app-gcp-cloud-run` attaches, through `backends` |

The project needs the APIs of Memorystore, Network Connectivity, Service Consumer Management, Compute Engine, and Secret Manager.

The instance has one shard, because the adapters keep each data set in one hash slot, so more shards would hold nothing more of it.

## No password: the network is what keeps others out

Authorization is off (`AUTH_DISABLED`), and the URL holds no secret, so it goes in `env`. **Anything that can reach the address in
this VPC can read and write the store.** The VPC is the module's own, so that is the Cloud Run service and its jobs unless you attach
something else.

Memorystore's IAM authorization is not an option yet. It wants a short-lived token in `AUTH` on each new connection, an hour by
default, and the server takes one static URL. Token authorization exists only in the `google-beta` provider, which these modules do
not use. The transport is encrypted (`SERVER_AUTHENTICATION`). The instance's CA is a public certificate that the provider returns,
so it is written to a secret with a plain `secret_data` and reaches the state, which is harmless: it is only in the secret store
because that is what the app mounts files from.

## What the private network adds

The module creates a VPC, a subnet, and a service connection policy of the class `gcp-memorystore`, which Memorystore requires
before an instance exists. The app adds Direct VPC egress through `vpc_access`, on the service and the jobs, for private ranges only.

- **Money**: Direct VPC egress has no connector to keep running. Cloud Run's guide says only that "network costs scale to zero just
  like the service itself" and lists no price. No Cloud NAT is needed for this, since internet traffic is not routed into the VPC
  under this setting; the page describing the setting does not state that in so many words, so it is unverified. Whether Private
  Service Connect adds a charge could not be read.
- **Cold starts**: Cloud Run's guide warns of "connection establishment delays of a minute or more on instance startup", and the
  server stops at start when it cannot reach the store. So a new instance may fail and restart, and a deployment that scales to zero
  starts instances often. Nobody has measured this: try it before relying on it.
- **Addresses**: each instance takes four, and each job task one for its run plus seven minutes. A `/24` is far more than the
  default `max_instances` needs.

## Cost

About $0.0318 per hour for the smallest node, so **about $23 a month, whether or not anyone uses the server**. That figure comes from
this project's design documents, checked on 2026-10-19; Google's pricing page returned no content when read on 2026-11-05, so check
[it](https://cloud.google.com/memorystore/valkey/pricing) yourself. Cloud Run scales to zero and this does not, so one small instance
costs more than the rest of a quiet deployment together.
