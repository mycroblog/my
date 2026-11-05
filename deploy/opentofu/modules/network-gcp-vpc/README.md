# network-gcp-vpc

A VPC on Google Cloud that the other modules take as `network = module.network`. It follows [the network contract](../../README.md#the-network-contract): it is no backend, it returns no settings, and it goes in no `backends`.

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that Google Cloud accepts it. [Unverified](#unverified) lists what stays open.

**You need it only for something with private addresses.** Cloud Run with Neon, or with `db-gcp-cloud-sql` on its public address, needs no network: leave it out. A VPC you have already passes as an object of the same shape, with no module.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "network" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/network-gcp-vpc?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
}

module "db" {
  # ...
  network = module.network
}

module "kv" {
  # ...
  network = module.network
}

module "app" {
  # ...
  network = module.network
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id`, `region` | required | Where it goes. Use the region of the app |
| `name` | `my` | The network, and the start of the names of the rest |
| `subnet_cidr` | `10.20.0.0/24` | The subnet of the service and the jobs: an RFC 1918 range of `/26` or larger |
| `service_connect_cidr` | `10.20.1.0/24` | The subnet of Private Service Connect's endpoints: an RFC 1918 range |
| `private_services_cidr` | `10.21.0.0/20` | The range of private services access: an RFC 1918 range of `/24` or larger |
| `nat` | `false` | Cloud NAT for the subnet of the service and the jobs |

The outputs are the fields of [the shape](../../README.md#the-network-on-google-cloud), and nothing else.

The project needs the APIs of Compute Engine, Service Networking, and Network Connectivity, and whoever applies needs what these resources take, such as `roles/compute.networkAdmin`. Enable the APIs of what you put on the network as well.

## What it creates

| Resource | For |
| --- | --- |
| The VPC, with no automatic subnets | Everything below |
| A subnet `<name>-run`, with Private Google Access | Direct VPC egress of the service and the jobs |
| A subnet `<name>-psc` | The addresses of Private Service Connect's endpoints, and nothing else |
| A service connection policy of the class `gcp-memorystore` | Memorystore, which connects only through one. It sets no limit |
| A reserved range, and a connection to `servicenetworking.googleapis.com` | Private services access, which gives Cloud SQL its private address |
| A Cloud Router and Cloud NAT, with `nat` | A way out to the internet for traffic that goes into the VPC |

**The policy is here and not in `kv-gcp-memorystore`.** Google Cloud allows one policy for each network, region, and service class, so it belongs to the network, whatever the number of instances. Google's guide says to leave the limit out when the number of instances is not known, and here it is not.

**Two subnets.** Google's guide for service connection policies recommends that their subnets hold nothing else, since other resources could use up the addresses of the endpoints, and Cloud Run's instances come and go by the hundred addresses. The guide for Direct VPC egress does not say whether a subnet can serve both. Separate subnets need no answer to that.

**Private services access for Cloud SQL, not Private Service Connect.** Google now recommends Private Service Connect for new deployments, for its address management and its reach across networks. It needs an endpoint for each VPC, and the Auth Proxy then needs DNS names. This deployment has one VPC and connects by the address, and the reserved range needs no record of its own, so the older path is the plainer fit. Moving an instance between the two later is something Google's pages do not describe.

**Every range is an RFC 1918 range.** An app with egress for private ranges sends only those through the VPC, so anything else would go to the internet and find nothing. The plan refuses another range.

## Unverified

- Everything against a real project: that these resources are accepted together, in this order.
- That Cloud Run reaches an address in `<name>-psc`, and one of the reserved range, from `<name>-run`. Google's guide speaks of clients "in this network" and of connecting to "the same VPC network", and neither page shows this path.
- That the destroy order works. The connection is abandoned on destroy, since Google warns that deleting it cuts off what uses it, and Cloud SQL's hold on the range after an instance goes was not read anywhere: a destroy may leave the peering behind for a time.
- What private services access, the policy, and the subnets cost: their pricing was not read. Traffic and any Private Service Connect charge come on top of what the modules on the network bill.
- [Cloud NAT's price](https://cloud.google.com/nat/pricing), which returned no content when read on 2026-11-05. As a guide, check it yourself before you turn on `nat`.
