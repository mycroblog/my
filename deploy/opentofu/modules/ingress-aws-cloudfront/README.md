# ingress-aws-cloudfront

Traffic reaches the app through CloudFront at `host`, with no cache. A Function URL takes no custom domain, so a deployment on Lambda needs something in
front of it to answer at `https://{host}`. It follows [the ingress contract](../../README.md#the-ingress-contract).

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it. It needs `app-aws-lambda`, and what that module lacks.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "ingress" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/ingress-aws-cloudfront?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `host` | required | Domain of the server |
| `origin_host` | required | `module.app.origin_host` |
| `certificate_arn` | required | A validated certificate for `host`, in `us-east-1` |
| `route53_zone_id` | null | Public hosted zone of `host`. Set, the module creates the records: recommended |
| `host_is_apex` | `false` | Whether `host` is the apex of its zone, for what `dns_records` asks while `route53_zone_id` is null |
| `origin_read_timeout` | `120` | Seconds it waits for bytes from the origin |
| `price_class` | `PriceClass_All` | Which edge locations serve the host |

| Output | Value |
| --- | --- |
| `env` | `MY_TRUSTED_PROXY_HEADER` |
| `trusted_proxies` | The ranges of `CLOUDFRONT_ORIGIN_FACING` |
| `service_ingress` | Null |
| `dns_records` | What to create elsewhere: read [DNS](#dns) |
| `dns_note` | What `dns_records` cannot say, or null |
| `domain_name`, `hosted_zone_id` | Of the distribution, for records made elsewhere |
| `secret_env`, `roles` | Empty |

It takes the certificate as an input, unlike Google's ingress modules, which obtain their own. Create it first: CloudFront takes one only from `us-east-1`, which
would need a second AWS provider in every configuration, and validating it within the apply would hold the apply until you add records that
only the apply prints.

Whoever runs a plan needs `cloudfront:ListCachePolicies`, `cloudfront:GetCachePolicy`, `cloudfront:ListOriginRequestPolicies`, and
`cloudfront:GetOriginRequestPolicy`, because the module looks up AWS's managed policies by name. A read-only role for plans needs them too.

## DNS

DNS allows no `CNAME` at a zone apex, and an instance often sits on one.

| `route53_zone_id` | `host_is_apex` | Records | `dns_records` |
| --- | --- | --- | --- |
| Set | Ignored | The module creates alias `A` and `AAAA` records, at an apex or on a subdomain | Empty |
| Null | `false` | Yours to create | A `CNAME` to the distribution |
| Null | `true` | Yours to create | An `ALIAS` to the distribution, with `dns_note` |

**Use Route 53 if you can.** It needs a public hosted zone, and whoever applies needs to change its records. An existing record of the same name and type fails the apply rather than being replaced.

At an apex elsewhere you need what your provider calls an `ALIAS` or `ANAME` record, or a flattened `CNAME`. **Not every DNS provider has one**, and without one this module cannot serve an apex
there. `host_is_apex` is an input because the labels of a name cannot tell: `social.example.org` and `example.co.uk` both have three. The module cannot check it, so a wrong `false` asks for a `CNAME` that your provider refuses.

## What protects the origin: nothing

**The Function URL stays open.** CloudFront can sign its requests to a Function URL with an origin access control, but then whoever sends a `POST` must send the
SHA-256 of its body in `x-amz-content-sha256`, and Lambda takes no unsigned payload. A peer delivering an activity does not, so federation would stop.

So anyone can go around CloudFront through the Function URL. The server trusts CloudFront's ranges and the adapter's loopback address, so **how far a direct caller
can forge its address depends on what a Function URL does to `X-Forwarded-For`, which is unverified**. A shared header that the server checks would close this; the
server has no such check.

## Limits

| Limit | Value | What it means |
| --- | --- | --- |
| Origin response timeout | 120 seconds without a quota increase | Streams send a heartbeat every 30 seconds. Jobs do not come through here, unlike Cloud Tasks on Google Cloud |
| Request body | 64 GB | Above Lambda's 6 MB, so that one decides |
| Trust list | 1,736 bytes in November 2026 | Part of the 4 KB of each function's environment, of which the example's other settings take about 1,100. `CLOUDFRONT` in full was 4,318 bytes, which does not fit. The plan fails if the total no longer does |

The ranges come from AWS at plan time and change only when you apply. Whether a request's Host matters: the server verifies signatures against `host`, so losing the
viewer's Host header is fine, but `account_domain` other than `host` compares the header and will not work through this module.

## Unverified

- Everything against a real account.
- The names `Managed-CachingDisabled` and `Managed-AllViewerExceptHostHeader`. AWS's guide lists these policies without the prefix, and the provider's documentation
  says that the API adds it. A wrong name fails the plan.
- The shape of `X-Forwarded-For` through CloudFront, a Function URL, and the adapter. Check the client addresses in the first request logs.
- Whether a stream of real-time updates passes through CloudFront as it is sent, with `RESPONSE_STREAM` behind it.
