# ingress-aws-cloudfront

Traffic reaches the app through CloudFront at `host`, with no cache. A Function URL takes no custom domain, so a deployment on Lambda needs something in
front of it to answer at `https://{host}`. It follows [the ingress contract](../../README.md#the-ingress-contract).

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it. It needs `app-aws-lambda`, and what that module lacks.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "ingress" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/ingress-aws-cloudfront?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `host` | required | Domain of the server |
| `origin_host` | required | `module.app.origin_host` |
| `certificate_arn` | required | A validated certificate for `host`, in `us-east-1` |
| `origin_read_timeout` | `120` | Seconds it waits for bytes from the origin |
| `price_class` | `PriceClass_All` | Which edge locations serve the host |

| Output | Value |
| --- | --- |
| `env` | `MY_TRUSTED_PROXY_HEADER` |
| `trusted_proxies` | The ranges of `CLOUDFRONT_ORIGIN_FACING` |
| `service_ingress` | Null |
| `dns_records` | A `CNAME` of `host` to the distribution |
| `secret_env`, `roles` | Empty |

It takes the certificate as an input, unlike Google's ingress modules, which obtain their own. Create it first: CloudFront takes one only from `us-east-1`, which
would need a second AWS provider in every configuration, and validating it within the apply would hold the apply until you add records that
only the apply prints.

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
- The shape of `X-Forwarded-For` through CloudFront, a Function URL, and the adapter. Check the client addresses in the first request logs.
- Whether a stream of real-time updates passes through CloudFront as it is sent, with `RESPONSE_STREAM` behind it.
