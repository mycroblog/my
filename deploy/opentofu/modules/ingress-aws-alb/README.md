# ingress-aws-alb

Traffic reaches the app through an Application Load Balancer that serves TLS at `host`, for `app-aws-ecs`. It follows [the ingress contract](../../README.md#the-ingress-contract).

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it. What stays unverified is [listed below](#unverified).

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "ingress" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/ingress-aws-alb?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `host` | required | Domain of the server |
| `network` | required | A network from [`network-aws-vpc`](../network-aws-vpc/README.md), or an object of [its shape](../../README.md#the-network-on-aws). The load balancer takes its public subnets, in at least two Availability Zones, each a `/27` or larger with eight free addresses |
| `certificate_arn` | required | A validated certificate for `host`, in the load balancer's region |
| `name` | `my` | Name of the load balancer and its target group |
| `ssl_policy` | `ELBSecurityPolicy-TLS13-1-2-Res-PQ-2025-09` | The policy AWS recommends. The API's own default is an older one, so the module always names it |
| `ip_address_type` | `ipv4` | Or `dualstack`, which needs IPv6 on the subnets |
| `idle_timeout` | `60` | Above the 30 seconds between a stream's heartbeats, and below the 120 for which the server keeps an idle connection |
| `deregistration_delay` | `30` | How long a leaving task keeps its connections. An open stream lasts to the end of it, and clients reconnect |
| `health_check_path` | `/readyz` | What the load balancer asks. Read [Health checks](#health-checks) |
| `route53_zone_id` | null | Public hosted zone of `host`. Set, the module creates the records: recommended |
| `host_is_apex` | `false` | Whether `host` is the apex of its zone, for what `dns_records` asks while `route53_zone_id` is null |

| Output | Value |
| --- | --- |
| `env` | `MY_TRUSTED_PROXY_HEADER` |
| `secret_env`, `permissions` | Empty |
| `trusted_proxies` | The ranges of the public subnets |
| `service_ingress` | `load-balancer`, which only an app that attaches to one accepts |
| `load_balancer` | The target group and the security group that the app attaches to |
| `dns_records` | What to create elsewhere, as [`ingress-aws-cloudfront`](../ingress-aws-cloudfront/README.md#dns) does |
| `dns_note` | What `dns_records` cannot say, or null |
| `domain_name`, `hosted_zone_id` | Of the load balancer, for records made elsewhere |

It reads nothing of the app, so `backends` can name the whole module. The certificate is an input: create it first, since validating it within the apply would hold the apply until you add records that only the apply prints.

A task in an Availability Zone that the public subnets do not cover gets no traffic, so give the network its subnets in the same zones, as `network-aws-vpc` does. The tasks must be in the VPC's primary range, which is what the load balancer's security group allows out.

## The origin needs no secret

**This module sets no origin secret, and no value of its own reaches the state.** The header check exists for an origin that anyone can connect to, where only the server can tell who came through the CDN. Here the platform closes the origin:

- The security group of the tasks admits port 8080 from the load balancer's security group alone, whether a task has a public address or not. Nobody on the internet can connect to a task, so no one can reach the server without passing the load balancer.
- A load balancer could not send a secret anyway. [What AWS documents](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/header-modification.html) for requests is renaming the headers it generates for mutual TLS and TLS; it lists no way to add a header of your own.

So `origin_secret` stays empty, and nothing lands in the state, unlike `ingress-aws-cloudfront` and `ingress-cloudflare-app`. To put a CDN in front of the load balancer is another shape, in which the load balancer itself would be the open origin. This module does not do that, and two ingresses that both name `trusted_proxies` fail the plan.

## Client addresses

The load balancer's nodes connect from addresses in its subnets, so `trusted_proxies` is their ranges and nothing else can connect from there to a task on that port. The module sets `xff_header_processing_mode` to `append`, the default, which adds the client's address to whatever `X-Forwarded-For` arrived, and leaves the client port out. The server takes the rightmost address that is not a proxy, so a forged header does not help a client.

The ranges cover the whole of those subnets, so a task in the same subnet is trusted too. That is harmless with these modules, but anything else you run there and open to the tasks could write the header.

## Health checks

A load balancer has one check, while Cloud Run has a separate startup probe and liveness probe. The module uses `/readyz`, because that is what holds a bad image back: a new task takes no traffic, and an old one does not leave, until it answers `200`.

The cost is that `/readyz` also fails while the database is down. The check allows 2.5 minutes of failures (5 at 30 seconds), and then ECS replaces the task, and goes on doing so until the database is back. Nothing is lost, and jobs are retried, but the tasks restart. If that matters more to you than the check on a new image, set `health_check_path` to `/healthz`, which answers while the process runs: a new image that cannot reach the database would then take traffic.

## Limits

| Limit | Value | What it means |
| --- | --- | --- |
| Request body | No limit in the load balancer | The server's own `media_max_*` settings decide |
| Streams | Heartbeat every 30 seconds, idle timeout 60 | A stream stays open until the task leaves, then the client reconnects |

What it costs is in [the app's README](../app-aws-ecs/README.md#what-it-costs).

## Unverified

- Everything against a real account.
- The shape of `X-Forwarded-For` as the server sees it. Check the client addresses in the first request logs: a private address from the load balancer's subnets means that the setting is wrong.
- Whether a stream of real-time updates passes through as it is sent, and whether `drop_invalid_header_fields` removes anything a peer needs for its signature. No header of a signature that the server reads has a name it should drop, by what AWS documents.
- That the policy name is accepted in your region: it comes from AWS's page of policies, and a wrong name fails the apply, not the plan.
