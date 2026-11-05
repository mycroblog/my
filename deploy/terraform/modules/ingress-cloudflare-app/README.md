# ingress-cloudflare-app

Traffic reaches the app through Cloudflare: a proxied DNS record of `host`, with the cache bypassed for everything, because
the API, ActivityPub, and HTML must never be cached. It takes the origin as an input, so it can front Cloud Run or anything
else. It follows [the ingress contract](../../README.md#the-ingress-contract). **Nothing here has run against Cloudflare:
the tests mock the provider.**

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "ingress" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/ingress-cloudflare-app?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `zone_id` | required | Zone of `host` |
| `host` | required | Domain of the server |
| `origin_host` | required | Host name or IPv4 address Cloudflare connects to |
| `rewrite_host_header` | `false` | Sends `origin_host` as `Host`. **Enterprise plan only** |
| `proxied` | `true` | Proxies the record |
| `extra_rules` | `{}` | The `rules` output of `ingress-cloudflare-media` in the same zone |

| Output | Value |
| --- | --- |
| `env` | `MY_TRUSTED_PROXY_HEADER` |
| `trusted_proxies` | Cloudflare's ranges. The app module adds the platform's own front end |
| `secret_env`, `roles`, `dns_records` | Empty: it creates the record itself |
| `service_ingress` | `INGRESS_TRAFFIC_ALL` |

## Plans: Cloud Run needs a matching `Host`

Cloud Run routes by `Host`, and Cloudflare's Origin Rules offer the Host header override on the **Enterprise plan only**, not
on Free, Pro, or Business. A plan with a `*.run.app` origin and no `rewrite_host_header` fails, with that explanation.

On the other plans, use an origin that accepts `host` itself:

- `ingress-gcp-domain-mapping` for `host` with `origin_host = "ghs.googlehosted.com"`. Leave it out of `backends`: this module
  says who connects. Whether its certificate is issued and renewed behind the proxy is unverified, so start with
  `proxied = false`.
- `ingress-gcp-load-balancer`, the same way, at its address, which adds its fee. Its address is then in no trust list,
  and whether that changes the client address is unverified.

## What protects the origin: nothing

**No lock exists.** The service stays open, because only a Google load balancer can be its sole entry. Authenticated origin
pulls need a client certificate check that a `run.app` origin does not do.

- Anyone can reach the service through its `run.app` URL, going around Cloudflare's protections. Do not rely on them for
  anything that must hold.
- **The client address can be forged from inside Cloudflare's own ranges.** The server trusts those ranges, and takes the
  rightmost address of `X-Forwarded-For` that is not trusted. Someone who calls the `run.app` URL from a Cloudflare Worker of
  their own appears from a trusted range, so an address they wrote in the header counts, which lets them dodge the rate limits
  by IP. A direct caller outside those ranges cannot do this.
- `X-Forwarded-For` is the header because the server reads only that one or `Forwarded`. `CF-Connecting-IP` would be no better:
  a direct caller can write it too.

The ranges come from Cloudflare's API at plan time and change only when you apply.

## Limits

| Limit | Value | What it means |
| --- | --- | --- |
| Proxy read timeout | 125 seconds (HTTP 524) | A job's request can take longer than this if Cloud Tasks calls through Cloudflare |
| Proxy idle timeout | 900 seconds | Streams send a heartbeat every 30 seconds |
| Upload | 100 MB on Free and Pro, 200 MB on Business | Above Cloud Run's 32 MiB, so that one decides |
| Zone rulesets | One per phase | A zone with its own cache, configuration, origin, or custom firewall rules conflicts with this module |

The read timeout matters most: a job runs for up to 5 minutes, and Cloudflare answers `524` to Cloud Tasks at about 2, which
then tries the task again. The job itself continues.

## Not a challenge in sight, unless you add one

Peers and Cloud Tasks are not browsers, so `/ap/`, `/_internal/`, `/.well-known/`, and `/api/` must not meet a challenge. This
module adds none and turns off Browser Integrity Check for `host`. **Bot Fight Mode is zone-wide and cannot be skipped by any
rule**: turn it off in the dashboard. This module does not manage it, because it would change every host of the zone.

## Unverified

- Everything against a real zone: no credentials exist for that.
- That `ssl` and `bic` in a configuration rule work on the Free plan: the documentation does not say which plans have them.
- Whether Cloudflare buffers `text/event-stream` on an ordinary proxied host: the documentation read covers Tunnel only.
- The shape of `X-Forwarded-For` that reaches the server through Cloudflare and Cloud Run. Check the client addresses in the
  first request logs.
- Tokens of Cloud Tasks and Cloud Scheduler through Cloudflare, whose addresses may be treated as bots.
