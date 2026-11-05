# network-aws-vpc

A VPC on AWS that the other modules take as `network = module.network`. It follows [the network contract](../../README.md#the-network-contract): it is no backend, it returns no settings, and it goes in no `backends`.

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it. [Unverified](#unverified) lists what stays open.

**You need a network for ECS, RDS, ElastiCache, and the load balancer.** Lambda with Neon needs none. A VPC you have already passes as an object of the same shape, with no module.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "network" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/network-aws-vpc?ref=v0.1.0&depth=1"

  availability_zones = ["ap-northeast-1a", "ap-northeast-1c"]
}

module "app" {
  # ...
  network = module.network
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `availability_zones` | required | Zones of the subnets, by name: 2 to 8. An input, so that a plan needs no call to AWS. A load balancer and RDS each need two |
| `name` | `my` | The `Name` tag of the VPC, and the start of the others |
| `cidr_block` | `10.0.0.0/16` | Range of the VPC, from `/16` to `/22`. Each subnet takes a sixteenth of it |
| `nat` | `false` | One NAT gateway, with a route from the private subnets |

The outputs are the fields of [the shape](../../README.md#the-network-on-aws), and nothing else.

## What it creates

A VPC with DNS on, an internet gateway, a public and a private subnet in each zone, a route from the public subnets to the internet gateway, and with `nat`, an Elastic IP, one NAT gateway in the first zone, and a route from the private subnets to it. A private subnet has no way out without it.

**One gateway**: when its zone goes down, the private subnets lose the internet until it comes back, and traffic from another zone crosses zones to reach it. For more, pass a VPC of your own.

**IPv4 only.** Neither subnet has an IPv6 range, so `ip_address_type = dualstack` of the load balancer needs a VPC of your own, and a server here reaches IPv6-only peers not at all.

## Unverified

- Everything against a real account.
- That a load balancer accepts the `/26` subnets of a `/22`, which is what its documents ask for; the default gives `/20` subnets.
- As a guide, a NAT gateway is $0.045 an hour and $0.045 per GB, and its public address $0.005 an hour, about $36.50 a month before traffic. Only the page for US East (Ohio) could be read, so your region may differ.
