# ingress-aws-cloudfront

CloudFront serves `host` in front of the Application Load Balancer of [`ingress-aws-alb`](../ingress-aws-alb/README.md), for `app-aws-ecs`, with `/media/` cached at its edges and nothing else cached.

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it. What stays unverified is [listed below](#unverified).

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "ingress" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/ingress-aws-alb?ref=v0.1.0&depth=1"

  host              = var.host
  network           = module.network
  certificate_arn   = var.certificate_arn
  behind_cloudfront = true
  origin_secret     = var.origin_secret
}

module "cdn" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/ingress-aws-cloudfront?ref=v0.1.0&depth=1"

  host            = var.host
  origin          = module.ingress.cloudfront_origin
  origin_secret   = module.ingress.origin_secret
  certificate_arn = var.cloudfront_certificate_arn
  route53_zone_id = var.route53_zone_id
}
```

**It is no backend of the app.** `backends` of `app-aws-ecs` names `module.ingress`, the load balancer, as it does without CloudFront: that module returns what the server trusts and what the service attaches to. [`examples/aws-ecs-neon`](../../examples/aws-ecs-neon/main.tf) has both blocks, behind `cloudfront = true`.

| Input | Default | Meaning |
| --- | --- | --- |
| `host` | required | Domain of the server |
| `origin` | required | `module.ingress.cloudfront_origin`: the load balancer's domain name, the header's name, and its idle timeout |
| `origin_secret` | required | `module.ingress.origin_secret`, the header's value, **which lands in the state** |
| `certificate_arn` | required | A validated certificate for `host`, in `us-east-1` |
| `name` | `my` | Start of the names of the media's cache and origin request policies, which are unique in the account |
| `route53_zone_id` | null | Public hosted zone of `host`. Set, the module creates the records: recommended |
| `host_is_apex` | `false` | Whether `host` is the apex of its zone, for what `dns_records` asks while `route53_zone_id` is null |
| `origin_read_timeout` | `60` | Seconds it waits for bytes from the load balancer, above the 30 seconds between a stream's heartbeats |
| `origin_keepalive_timeout` | `5` | Seconds it keeps an idle connection to the load balancer, below the load balancer's idle timeout |
| `price_class` | `PriceClass_All` | Which edge locations serve the host |

| Output | Value |
| --- | --- |
| `dns_records` | What to create elsewhere: read [DNS](#dns) |
| `dns_note` | What `dns_records` cannot say, or null |
| `domain_name`, `hosted_zone_id` | Of the distribution, for records made elsewhere |

It takes the certificate as an input, unlike Google's ingress modules, which obtain their own. Create it first: CloudFront takes one only from `us-east-1`, which
would need a second AWS provider in every configuration, and validating it within the apply would hold the apply until you add records that
only the apply prints. The load balancer needs its own certificate for `host` in its region, since CloudFront checks the load balancer's against `host`; in `us-east-1` one certificate serves both.

Whoever runs a plan needs `cloudfront:ListCachePolicies`, `cloudfront:GetCachePolicy`, `cloudfront:ListOriginRequestPolicies`, and
`cloudfront:GetOriginRequestPolicy`, because the module looks up AWS's managed policies by name, and `ec2:DescribeManagedPrefixLists` and `ec2:GetManagedPrefixListEntries` for `ingress-aws-alb`. A read-only role for plans needs them too.

## What closes the load balancer

With `behind_cloudfront`, the load balancer is a public origin, so `ingress-aws-alb` closes it in two steps:

1. **Its security group admits CloudFront alone**, on port 443, through AWS's managed prefix list `com.amazonaws.global.cloudfront.origin-facing`. Nothing else on the internet can connect.
2. **Its listener forwards only a request that carries the header.** CloudFront sends `origin_secret_header` with `origin_secret` on every request, replacing what a viewer sent under that name, and the listener answers anything else with `403`. This step is needed because every distribution, of any account, connects from the same addresses.

So nobody reaches the server around this distribution, and nobody can choose the address in `X-Forwarded-For` that the server reads. The server itself checks no header (`MY_ORIGIN_SECRET` stays unset): the security group of the tasks admits the load balancer alone, as without CloudFront. The load balancer's health checks go to the tasks straight, and are not affected.

**The value is in the state**, twice: in the distribution and in the listener rule. The provider has no write-only form of either, so nothing can keep it out. The variables are sensitive, which keeps the value out of what `tofu` prints, not out of the state. Keep the state private. Anyone who can read the distribution's or the listener's configuration can read the value too.

The value is 32 to 128 letters, digits, hyphens, and underscores (`openssl rand -hex 32`), since a listener matches up to 128 characters and reads `*` and `?` as wildcards.

### Rotate the value

The listener accepts two values while the value changes, so that no request is refused while CloudFront's edges take the new one:

1. Set `origin_secret` of `ingress-aws-alb` to the new value and `origin_secret_previous` to the old one, and apply. The listener accepts both, and CloudFront starts sending the new one.
2. Wait until the distribution's status is `Deployed`.
3. Remove `origin_secret_previous` and apply.

## Client addresses

Two hops append to `X-Forwarded-For`, and the server must trust both:

1. CloudFront appends the viewer's address to the header it received, or creates the header.
2. The load balancer appends the address that connected to it, one of CloudFront's (`xff_header_processing_mode = "append"`).

So the server receives `<whatever the viewer sent>, <viewer>, <CloudFront>` from a load balancer node. `trusted_proxies` of `ingress-aws-alb` is the ranges of the load balancer's subnets and the entries of the prefix list, and the app sets `MY_TRUSTED_PROXIES` from it and `MY_TRUSTED_PROXY_HEADER` to `x-forwarded-for`. The server takes the rightmost address that is not a proxy, which is the viewer's: what the viewer wrote stays to the left of it and is never read.

The ranges come from AWS at plan time, and change only when you apply. Apply now and then: an edge on an address added since then would be taken for the client, which puts its viewers under one rate limit.

## What it caches

| Path | Cache | What reaches the load balancer |
| --- | --- | --- |
| `/media/*`, which holds `/media/proxy/` | The server's `Cache-Control`: a year for local media, `remote_media_cache_ttl_seconds` for the proxy. Nothing without a `Cache-Control` is kept. The query is in the key, since a proxy URL carries its signature there | `GET` and `HEAD`, with `Host` and the query, and no cookie |
| Everything else: pages, the client API, OAuth, ActivityPub | None (`Managed-CachingDisabled`) | Every method, with every header, cookie, and query of the viewer (`Managed-AllViewer`), `Host` among them |

- `Host` reaches the load balancer, since CloudFront checks the load balancer's certificate against it, and the server sees the host the viewer asked for.
- Responses pass with every `Set-Cookie` header the server sends, since nothing is cached.
- Nothing is compressed at the edge, which would hold back the events of a stream.
- No origin access control: the load balancer is no AWS origin that signs, and a peer of the federation does not send the SHA-256 of a `POST` body.

## Streams and timeouts

| Setting | Value | Why |
| --- | --- | --- |
| A stream's heartbeat | Every 30 seconds | The server's own |
| `origin_read_timeout` of this module | 60 seconds | Above the heartbeat. The shorter of this and the load balancer's idle timeout ends a response that stays silent |
| `idle_timeout` of `ingress-aws-alb` | 60 seconds | Above the heartbeat, and below the server's 120 for an idle connection |
| `origin_keepalive_timeout` of this module | 5 seconds | Below the load balancer's idle timeout, which the plan checks, so CloudFront never sends on a connection the load balancer is closing |

A stream stays open until a task leaves or CloudFront closes it, and then the client reconnects.

## DNS

DNS allows no `CNAME` at a zone apex, and an instance often sits on one. `host` points at the distribution, so `ingress-aws-alb` behind CloudFront creates no record and refuses `route53_zone_id`: give it to this module.

| `route53_zone_id` | `host_is_apex` | Records | `dns_records` |
| --- | --- | --- | --- |
| Set | Ignored | The module creates alias `A` and `AAAA` records, at an apex or on a subdomain | Empty |
| Null | `false` | Yours to create | A `CNAME` to the distribution |
| Null | `true` | Yours to create | An `ALIAS` to the distribution, with `dns_note` |

**Use Route 53 if you can.** It needs a public hosted zone, and whoever applies needs to change its records. An existing record of the same name and type fails the apply rather than being replaced, so a record of the load balancer from before CloudFront must go first.

At an apex elsewhere you need what your provider calls an `ALIAS` or `ANAME` record, or a flattened `CNAME`. **Not every DNS provider has one**, and without one this module cannot serve an apex
there. `host_is_apex` is an input because the labels of a name cannot tell: `social.example.org` and `example.co.uk` both have three. The module cannot check it, so a wrong `false` asks for a `CNAME` that your provider refuses.

## Limits

| Limit | Value | What it means |
| --- | --- | --- |
| Origin response timeout | 120 seconds without a quota increase | Streams send a heartbeat every 30 seconds |
| Rules of the load balancer's security group | The prefix list counts as about 55 of the default 60 | Add nothing else to that group |

## Unverified

- Everything against a real account.
- The names `Managed-CachingDisabled` and `Managed-AllViewer`. AWS's guide lists these policies without the prefix, and the provider's documentation
  says that the API adds it. A wrong name fails the plan.
- The shape of `X-Forwarded-For` through CloudFront and the load balancer. Check the client addresses in the first request logs: an address of CloudFront means that the trust list is stale or wrong.
- Whether a stream of real-time updates passes through CloudFront and the load balancer as it is sent.
- That a listener rule matches the header's value exactly, given the case-insensitive match that AWS documents. The value is long enough that this costs nothing.
