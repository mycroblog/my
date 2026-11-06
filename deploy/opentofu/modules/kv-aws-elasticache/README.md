# kv-aws-elasticache

The key-value store on ElastiCache Serverless for Valkey, on a network you give it. It follows
[the backend contract](../../README.md#the-backend-contract). It bills while the server is idle: see [Cost](#cost). `kv = db` is the default and needs no
module. Use this when you want caches and rate limits off the database.

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS
accepts it. Unverified:

- that a cache with no user group takes a connection with no password. The pages read do not say so, and the module relies on it;
- that the server's client connects at all. A serverless cache refuses the `CLIENT` subcommands that AWS lists as unavailable, and the
  list of supported commands does not name `CLIENT SETINFO`, which clients often send. The commands the adapters use (`EVALSHA`,
  `HSET`, `PEXPIRE`, sorted sets, `PUBLISH`, `SUBSCRIBE`) are listed as supported, and the server turns client-side caching off;
- that the certificate chains to the roots of the server's image, so that no CA file is needed;
- the minimum billed storage: see [Cost](#cost).

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "kv" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kv-aws-elasticache?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `network` | required | A network from [`network-aws-vpc`](../network-aws-vpc/README.md), or an object of [its shape](../../README.md#the-network-on-aws). The cache takes its addresses in the private subnets |
| `name` | `my-valkey` | The cache, and the start of the security groups' names |

| Output | Value |
| --- | --- |
| `env` | `MY_KV`, `MY_VALKEY_URL` |
| `secret_env` | Empty |
| `permissions` | Empty |
| `security_group_ids` | The group the app joins, through `backends` |
| `private_network` | The VPC, through `backends`, which the app must have too |

## What you must provide: the network

- **A network**, which the module takes as an input and does not create.
- **The app on that network.** `backends` hands it `security_group_ids`, and the cache takes port 6379 from that group alone.

On Fargate this adds only the groups, since a task is already in a VPC, and Valkey needs no route to the internet. What the task
needs for everything else is the same with or without Valkey, and it is the larger cost: a public IPv4 address on each task, $0.005
per hour (about $3.65 a month), or a NAT gateway, $0.045 per hour (about $33 a month in US East, Ohio) and $0.045 per GB.

## No password: the network is what keeps others out

The URL holds no secret, so it goes in `env`, and there is nothing to generate or keep out of the state. **The security group decides
who connects**, and it admits only the client group that the app joins. That is the deliberate answer for this module, and it holds on
these conditions:

- **Nothing you do not trust with the store joins the client group.** Joining takes permissions on the account that already reach much
  more. Being on the same VPC is not enough here, unlike on Memorystore.
- **A remote server cannot reach it through the server**: outbound HTTP refuses private addresses, as
  [The network contract](../../README.md#what-stays-closed) says.
- **What the store holds is caches, rate limits, and real-time updates.** The database's data and the secrets are elsewhere.

What this gives up against IAM authentication is a second lock for whoever is already in that group, and a record of who connected.
IAM tokens last 15 minutes and an IAM connection drops after 12 hours, and the server takes one static URL. Nothing here calls for it.

A password would take an `aws_elasticache_user`, whose `passwords_wo` can keep it out of the state, and a URL secret; the module does
not do this, because the security group already decides who connects. For a password, make the cache elsewhere and use
[`kv-external`](../kv-external/README.md).

## Cost

From [ElastiCache's pricing page](https://aws.amazon.com/elasticache/pricing/), read on 2026-11-05, in US East (N. Virginia): $0.084
per GB-hour of data stored and $0.0023 per million ECPUs, with a minimum of 100 MB per cache. That is **about $6.13 a month, whether
or not anyone uses the server**. It is the cheapest Valkey here.

**AWS's documents disagree.** The guide's
[How ElastiCache works](https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/WhatIs.corecomponents.html) says each serverless cache
is metered for at least 1 GB, which would be about $61 a month. Check your first bill or the pricing calculator before relying on either.
