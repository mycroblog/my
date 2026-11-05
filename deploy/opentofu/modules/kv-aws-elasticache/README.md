# kv-aws-elasticache

The key-value store on ElastiCache Serverless for Valkey, in a VPC you provide. It follows
[the backend contract](../../README.md#the-backend-contract). **It breaks the near-zero goal**: see [Cost](#cost). `kv = db` needs no
module and costs nothing more, so use this only when the database cannot take the load of caches and rate limits.

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
| `vpc_id`, `subnet_ids` | required | The VPC, and the subnets where the cache takes its addresses |
| `name` | `my-valkey` | The cache, and the start of the security groups' names |

| Output | Value |
| --- | --- |
| `env` | `MY_KV`, `MY_VALKEY_URL` |
| `secret_env` | Empty |
| `permissions` | Empty |
| `client_security_group_id` | The group to attach to the tasks |

## What you must provide: the network

- **A VPC and subnets**, which the module takes as inputs and does not create.
- **The tasks in that VPC, with `client_security_group_id` attached.** The cache takes port 6379 from that group alone.

On Fargate this adds only the groups, since a task is already in a VPC, and Valkey needs no route to the internet. What the task
needs for everything else is the same with or without Valkey, and it is the larger cost: a public IPv4 address on each task, $0.005
per hour (about $3.65 a month), or a NAT gateway, $0.045 per hour (about $33 a month in US East, Ohio) and $0.045 per GB.

**On Lambda this module does not work yet.** `app-aws-lambda` has no VPC input, and a function attached to a VPC reaches the internet
only through something like that NAT gateway, which federation needs. A public endpoint would avoid it, but it requires IAM
authentication, and the server takes one static URL.

## No password: the network is what keeps others out

The URL holds no secret, so it goes in `env`, and there is nothing to generate or keep out of the state. A password would take an
`aws_elasticache_user`, whose `passwords_wo` can keep it out of the state, and a URL secret; the module does not do this, because the
security group already decides who connects. For a password, make the cache elsewhere and use [`kv-external`](../kv-external/README.md).

## Cost

From [ElastiCache's pricing page](https://aws.amazon.com/elasticache/pricing/), read on 2026-11-05, in US East (N. Virginia): $0.084
per GB-hour of data stored and $0.0023 per million ECPUs, with a minimum of 100 MB per cache. That is **about $6.13 a month, whether
or not anyone uses the server**. It is the cheapest Valkey here, and it is still a bill for idle time that a deployment at zero
otherwise does not have.

**AWS's documents disagree.** The guide's
[How ElastiCache works](https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/WhatIs.corecomponents.html) says each serverless cache
is metered for at least 1 GB, which would be about $61 a month. Check your first bill or the pricing calculator before relying on either.
