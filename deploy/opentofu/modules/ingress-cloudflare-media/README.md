# ingress-cloudflare-media

A media host through Cloudflare: it returns `MY_MEDIA_ORIGIN`, caches `/media/` and `/media/proxy/`, and refuses every other
path, because the server answers whatever `Host` it gets. It sets no trust, so it sits beside any other `ingress-*` module.
It follows [Put a CDN in front of media delivery](../../../../docs/operations/cdn.md). **Nothing here has run against
Cloudflare: the tests mock the provider.**

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "ingress" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/ingress-cloudflare-media?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `zone_id` | required | Zone of the media host |
| `host` | required | The media host |
| `origin_host` | required | Host name or IPv4 address Cloudflare connects to |
| `rewrite_host_header` | `false` | Sends `origin_host` as `Host`. **Enterprise plan only** |
| `proxied` | `true` | Proxies the record |
| `manage_rulesets` | `true` | Creates the zone's rulesets. A zone has one per phase, so set it to `false` beside `ingress-cloudflare-app` in the same zone, and pass the `rules` output to its `extra_rules` |
| `tiered_cache` | `true` | Smart Tiered Cache, on the zone. It strengthens hits and changes no lifetime |

| Output | Value |
| --- | --- |
| `env` | `MY_MEDIA_ORIGIN` |
| `secret_env`, `roles`, `dns_records` | Empty: it creates the record itself |
| `rules` | The rules of this host, by phase |

Pass only `env` to `backends`, as `{ env = module.media.env }`.

## Caching

- **Eligible for cache**, including video and audio, for every path under `/media/`.
- **The query string is in the cache key** by Cloudflare's default, and `sig` with it. There is no custom key, which is for
  Enterprise on this setting.
- **The origin's `Cache-Control` decides how long**: `respect_origin` for the edge and the browser. Local media carries
  `public, max-age=31536000, immutable`, and `/media/proxy/` follows `remote_media_cache_ttl_seconds`, which a TTL here would
  break. So there is no long edge TTL.
- **A deleted file can stay cached for up to a year.** Purge it by hand: [the guide](../../../../docs/operations/deploy-cloud-run.md#cloudflare)
  has the call.

## Limits

- **Cloudflare's terms restrict video on Free, Pro, and Business.** Its documentation says a site "serving videos or a
  disproportionate amount of large files" without a paid service may have content redirected or face other action. Decide
  this before you serve video from this host.
- Cacheable files go up to 512 MB on Free, Pro, and Business.
- Cloud Run routes by `Host`, and the Host header override is Enterprise only, so on other plans the origin must accept the
  media host: a second `ingress-gcp-domain-mapping` for it, left out of `backends`, with
  `origin_host = "ghs.googlehosted.com"`. A load balancer's certificate covers `host` alone, which strict TLS refuses.
- Anyone can still reach `/media/` at the origin. Nothing is secret there: media URLs are not access-controlled in any case.

## Unverified

Everything against a real zone: no credentials exist for that. That Cloudflare caches `Cache-Control: immutable` for the full
`max-age`, since an edge may evict earlier; the behaviour of a domain mapping's certificate behind the proxy; and that
`ssl` in a configuration rule works on the Free plan.
