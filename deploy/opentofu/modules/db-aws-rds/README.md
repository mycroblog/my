# db-aws-rds

PostgreSQL on Amazon RDS, in your VPC. It creates the instance, its subnet group and security groups, and the secret that holds the
connection URL, and follows [the backend contract](../../README.md#the-backend-contract).

**This module has never been applied, and no deployment can run on AWS yet**: read [AWS: unverified](../../README.md#aws-unverified).
The tests mock the provider, and a plan with no credentials shows what the plan holds. Unverified:

- that an instance with these settings is created, in particular that version `18` is available on `db.t4g.micro` in your region;
- what the state holds beyond what the provider's schema says, since there is no stub of AWS and no apply here. The same mechanism
  was applied against a stub of Google Cloud, which shows that OpenTofu hands one value to both resources;
- that a function or a task reaches the instance, and that `my schema apply` runs on it;
- that `password_wo` changes the password at once rather than in the maintenance window.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/db-aws-rds?ref=v0.1.0&depth=1"

  vpc_id     = var.vpc_id
  subnet_ids = var.private_subnet_ids
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `vpc_id`, `subnet_ids` | required | Where it lives: private subnets in at least two Availability Zones |
| `name` | `my` | Start of the names |
| `database`, `username` | `my` | Names in the URL: lower-case letters, digits, and underscores |
| `url_secret_name` | `my-database-url` | Secret this module creates |
| `engine_version` | `18` | PostgreSQL version |
| `instance_class` | `db.t4g.micro` | Instance class |
| `allocated_storage_gb`, `max_allocated_storage_gb` | `20`, `100` | Storage at creation, and how far it grows by itself |
| `backup_retention_days` | `7` | Automated backups and point-in-time recovery, up to 35. `0` turns them off |
| `multi_az` | `false` | A standby in another Availability Zone |
| `deletion_protection` | `true` | Refuses to delete the instance in AWS. `prevent_destroy` refuses it in OpenTofu in any case |
| `password_version` | `1` | Raise it for a new password: read below |
| `url_parameters` | `{}` | More parameters of the URL, beside `sslmode` |

| Output | Value |
| --- | --- |
| `env` | `MY_DATABASE` |
| `secret_env` | `MY_DATABASE_URL`, by the secret's ARN |
| `direct_secret_env` | Empty: the only connection is direct |
| `permissions` | Empty: the app module grants access to every secret it is given |
| `security_group_ids` | The group that the app joins to reach the instance |
| `url_without_password` | The URL in the secret, with `PASSWORD` in place of the password |

## RDS, not Aurora Serverless v2

An instance has one price and answers at once. Aurora Serverless v2 can pause, which would suit a quiet server, but the server's tick
touches the database each minute by default, so it would rarely pause, and then each resume costs a request many seconds. Nothing
stops a module for it later: its provider resource has the same write-only password, `master_password_wo`, and it would return the
same outputs.

## The connection

```text
postgres://my:PASSWORD@ENDPOINT:5432/my?sslmode=require
```

**This is a direct connection**, to the instance's own endpoint, as `database_url` must be for `my schema apply`, which holds a
session-level lock. `sslmode=require` encrypts and does not check the certificate, since the server image does not hold the CA
bundle of RDS.

There is no pooled connection, and RDS Proxy is deliberately not an option here. It is a recurring cost on top of the instance, its
secret must be a JSON document of a user and a password rather than this URL, and AWS's documents say that session-level advisory
locks, `SET`, and `LISTEN` pin a client to one connection, which removes what the proxy is for. Lambda is where this hurts: each
instance of a function holds its own connections, so bound them with `web_reserved_concurrency`, `worker_max_concurrency`, and a
small pool. A proxy of your own can still go in through `pooled_url_secret_id` of `db-external`.

To set `pool_max_conns`, name it in `url_parameters`. The one-off work reads the same URL, and only the service opens a pool with it.

## Reaching it from the app

The instance has no public address. Its security group accepts only the group in `security_group_ids`, and the app joins that:

- **Lambda**: give `app-aws-lambda` `vpc_subnet_ids`, and a group that allows the way out in `vpc_security_group_ids`. It takes the
  rest from `backends`. **An attached function loses the internet**, which it needs for federation, Secrets Manager, and the queue,
  unless the subnets are private and route through a NAT gateway: a public subnet does not help. So on Lambda, this module brings a
  NAT gateway with it, which you provide, and which costs more than the database.
- **ECS**: a task joins the group the same way and needs the way out too, but it can have a public address in a public subnet instead
  of a NAT gateway.

The creating role also needs `ec2:DescribeSecurityGroups`, `ec2:DescribeSubnets`, `ec2:DescribeVpcs`, and `ec2:GetSecurityGroupsForVpc`
for Lambda's checks. The function's role gets what Lambda needs to create its interfaces, which the function's own code can then
call too; AWS's documents show a deny statement for that, which this configuration cannot express without a cycle.

## The password

The first apply generates it through an ephemeral resource, and hands the same value to the instance and to the URL secret through
write-only attributes, so it reaches neither the plan nor the state. Nothing has to be done by hand. RDS's own
`manage_master_user_password` is not used: it keeps a JSON document in Secrets Manager, and the server reads a URL.

`password_version` moves both at once. Raise it to:

- **change the password**. What runs keeps the URL it started with, so restart it right afterwards;
- **recover from an apply that stopped between the two**. A value is lost once the apply ends, so a second apply that finishes the
  other resource would generate another: the plan shows no difference and the URL does not work. Raise the number and apply again;
- **write the URL again**, after changing `url_parameters`, or if the endpoint changes.

## Coming from db-external

**In practice this is not a one-block swap.** The block changes, and on Lambda the VPC settings of the app with it, but the new
instance is empty, and everything your instance knows is in the other database. Moving means, in this order: the maintenance mode,
`pg_dump` from the old database, `pg_restore` to this one from inside the VPC, then swapping the block and applying, then leaving the
maintenance mode. Nothing here does this for you, and no one has tried it.

## Removing it

`prevent_destroy` stops any plan that would delete the instance, and `deletion_protection` stops the API. Deleting it leaves a final
snapshot and keeps the automated backups, which go on costing until you delete them.

## What it costs

A fixed cost, whatever the traffic. On-demand, for 730 hours, from AWS's price list of 2026-10-29:

| Item | N. Virginia | Tokyo | Per month |
| --- | --- | --- | --- |
| `db.t4g.micro`, Single-AZ | $0.016 per hour | $0.025 per hour | $11.68, $18.25 |
| 20 GB of gp3 | $0.115 per GB-month | $0.138 per GB-month | $2.30, $2.76 |

That comes to **about $14 a month in N. Virginia and $21 in Tokyo**. CPU above the T4g baseline costs $0.075 per vCPU-hour, in Unlimited
mode. The price of backup storage could not be read: AWS's pricing pages load it in a way that was not available. Check your first bill.

On Lambda, add the NAT gateway, $0.045 per hour in Ohio or $32.85 a month plus $0.045 per GB, and its public address at $0.005 per hour,
or $3.65 a month. Only Ohio's price of the gateway could be read, so your region's may differ.
