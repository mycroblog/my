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
| `origin_secret` | required | The header's value, **which lands in the state**. Pass it with `TF_VAR_origin_secret` |
| `origin_secret_id` | required | The ARN of the same value in Secrets Manager |
| `origin_secret_header` | `x-my-origin-secret` | The header's name |
| `route53_zone_id` | null | Public hosted zone of `host`. Set, the module creates the records: recommended |
| `host_is_apex` | `false` | Whether `host` is the apex of its zone, for what `dns_records` asks while `route53_zone_id` is null |
| `origin_read_timeout` | `120` | Seconds it waits for bytes from the origin |
| `price_class` | `PriceClass_All` | Which edge locations serve the host |

| Output | Value |
| --- | --- |
| `env` | `MY_TRUSTED_PROXY_HEADER`, `MY_ORIGIN_SECRET_HEADER` |
| `secret_env` | `MY_ORIGIN_SECRET`, by reference |
| `trusted_proxies` | The ranges of `CLOUDFRONT_ORIGIN_FACING` |
| `service_ingress` | `all`: anyone can connect to the Function URL, and the server refuses what lacks the header |
| `dns_records` | What to create elsewhere: read [DNS](#dns) |
| `dns_note` | What `dns_records` cannot say, or null |
| `domain_name`, `hosted_zone_id` | Of the distribution, for records made elsewhere |
| `permissions` | Empty |

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

## What protects the origin: a header

CloudFront sends `origin_secret_header` on every request to the origin, replacing what a viewer sent, and the server refuses a request without it with a `403`. So **a direct caller of the Function URL is refused**, whichever address it writes in `X-Forwarded-For`. Only the
health checks and `POST` to `/_internal/...` are exempt, and the web function has no event path. The refusal is the server's: Lambda still runs the function for such a request, and bills it.

An origin access control would close the URL in AWS itself, but then whoever sends a `POST` must send the SHA-256 of its body in `x-amz-content-sha256`, and Lambda takes no unsigned payload. A peer delivering an activity does not, so federation would stop.

**The value is in the state.** The provider's `custom_header` has no write-only form, so nothing can keep it out, and generating it through an ephemeral resource would not help. The variable is sensitive, which keeps it out of what `tofu` prints, not out of the state. Keep the state private. Anyone who can
read the distribution's configuration can read the value too.

**This depends on what the server cannot yet do on Lambda**: it reads `MY_ORIGIN_SECRET` through `SECRET_ARN_MY_ORIGIN_SECRET`, like every secret there, which `app-aws-lambda` says is missing. Nothing has been run, so the `403` is what the server documents rather than what has been seen.

To rotate the value, follow the steps under "Rotate the value" in [the CDN guide](../../../../docs/operations/cdn.md).

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
