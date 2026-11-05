# ingress-cloudflare-app

Traffic reaches the app through Cloudflare: a proxied DNS record of `host`, with the cache bypassed for everything, because
the API, ActivityPub, and HTML must never be cached. It takes the origin as an input, so it can front Cloud Run or anything
else. It follows [the ingress contract](../../README.md#the-ingress-contract). **Nothing here has run against Cloudflare:
the tests mock the provider.**

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "ingress" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/ingress-cloudflare-app?ref=v0.1.0&depth=1"
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
| `origin_secret` | required | The header's value, **which lands in the state**. Pass it with `TF_VAR_origin_secret` |
| `origin_secret_id` | required | The same value in the platform's secret store: a secret's ID, or an ARN on AWS |
| `origin_secret_header` | `x-my-origin-secret` | The header's name |
| `origin_secret_extra_hosts` | empty | Other host names of the zone that reach the server, such as a separate `account_domain` |
| `extra_rules` | `{}` | The `rules` output of `ingress-cloudflare-media` in the same zone |

| Output | Value |
| --- | --- |
| `env` | `MY_TRUSTED_PROXY_HEADER`, `MY_ORIGIN_SECRET_HEADER` |
| `secret_env` | `MY_ORIGIN_SECRET`, by reference |
| `trusted_proxies` | Cloudflare's ranges. The app module adds the platform's own front end |
| `permissions`, `dns_records` | Empty: it creates the record itself |
| `service_ingress` | `all`, which the app must set: anyone can connect, and the server refuses what lacks the header |

## Plans: Cloud Run needs a matching `Host`

Cloud Run routes by `Host`, and Cloudflare's Origin Rules offer the Host header override on the **Enterprise plan only**, not
on Free, Pro, or Business. A plan with a `*.run.app` origin and no `rewrite_host_header` fails, with that explanation.

A Lambda Function URL (`*.on.aws`) routes by `Host` the same way, and a plan with one and no `rewrite_host_header` fails too. AWS
has nothing like a domain mapping for a Function URL, so on Lambda this module needs the Enterprise plan, or CloudFront behind it.

On the other plans, use an origin that accepts `host` itself:

- `ingress-gcp-domain-mapping` for `host` with `origin_host = "ghs.googlehosted.com"`. Leave it out of `backends`: this module
  says who connects. Whether its certificate is issued and renewed behind the proxy is unverified, so start with
  `proxied = false`.
- `ingress-gcp-load-balancer`, the same way, at its address, which adds its fee. Its address is then in no trust list,
  and whether that changes the client address is unverified.

## What protects the origin: a header

A request header transform rule sets `origin_secret_header` to the value on every request to the server, replacing whatever a client sent, and the server refuses a request without it with a `403`. Only
the health checks and `POST` to `/_internal/...`, which checks its own token, are exempt. So someone who calls the `run.app` URL directly, even from a Cloudflare Worker of their own, is refused. **That closes the forgery of the client
address** from inside Cloudflare's ranges, and going around Cloudflare's protections. The platform still accepts the connection: this is a check in the server, not a closed port.

**The value is in the state.** The provider's `cloudflare_ruleset` has no write-only form of a header's value, so nothing can keep it out, and generating it through an ephemeral resource would not help. The variable is sensitive, which keeps it out of what `tofu` prints, not out of the state.
Keep the state private. Anyone who can read the zone's rules in the dashboard can read the value too.

Setting it up:

1. `openssl rand -hex 32`, and put the value in the platform's secret store, the secret of `origin_secret_id`.
2. Pass the same value with `TF_VAR_origin_secret`, not in a file you commit, and apply. The rule and the server's settings then change in the same apply, so requests may be refused until both have taken effect.
3. Check: `https://<host>/` answers `200`, and `https://<origin_host>/` answers `403`.

To rotate it, follow the steps under "Rotate the value" in [the CDN guide](../../../../docs/operations/cdn.md): `MY_ORIGIN_SECRET_PREVIOUS` goes in `secret_env` of the app while the two differ.

**A rule covers only the hosts it names**, deliberately, so that the rest of the zone never sees the value. A host name that reaches the server and is not covered is locked out: add it to `origin_secret_extra_hosts`. A media host goes through `extra_rules`, and the two share this one rule and one secret.

With `proxied = false` nothing adds the header, so the server refuses everything but its health checks. Get the certificate of a domain mapping before you set this up.

`X-Forwarded-For` is the header the server reads for the client's address, because it reads only that one or `Forwarded`. `CF-Connecting-IP` would be no better, since a direct caller could write it too.

The ranges come from Cloudflare's API at plan time and change only when you apply.

## Limits

| Limit | Value | What it means |
| --- | --- | --- |
| Proxy read timeout | 125 seconds (HTTP 524) | Not for jobs: Cloud Tasks and Cloud Scheduler call the service's `run.app` URL directly, never through Cloudflare |
| Proxy idle timeout | 900 seconds | Streams send a heartbeat every 30 seconds |
| Upload | 100 MB on Free and Pro, 200 MB on Business | Above Cloud Run's 32 MiB, so that one decides |
| Transform rules | 10 on Free, 25 on Pro, 50 on Business, 300 on Enterprise, for all kinds together | This module uses one |
| Zone rulesets | One per phase | A zone with its own cache, configuration, origin, or custom firewall rules conflicts with this module |

## Not a challenge in sight, unless you add one

Peers are not browsers, so `/ap/`, `/.well-known/`, and `/api/` must not meet a challenge. Jobs do not come through here. This
module adds none and turns off Browser Integrity Check for `host`. **Bot Fight Mode is zone-wide and cannot be skipped by any
rule**: turn it off in the dashboard. This module does not manage it, because it would change every host of the zone.

## Unverified

- Everything against a real zone: no credentials exist for that.
- That the header reaches the origin as set, and that a request without it answers `403` on a real deployment.
- That `ssl` and `bic` in a configuration rule work on the Free plan: the documentation does not say which plans have them.
- Whether Cloudflare buffers `text/event-stream` on an ordinary proxied host: the documentation read covers Tunnel only.
- The shape of `X-Forwarded-For` that reaches the server through Cloudflare and Cloud Run. Check the client addresses in the
  first request logs.
