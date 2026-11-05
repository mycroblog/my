# ingress-gcp-domain-mapping

Traffic reaches the service directly, on a Cloud Run domain mapping of `host`. It has no monthly fee, which suits trying a deployment
cheaply, but it is a Preview feature that Google Cloud does not recommend for production, because of latency. It follows
[the ingress contract](../../README.md#the-ingress-contract).

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "ingress" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/ingress-gcp-domain-mapping?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Meaning |
| --- | --- |
| `project_id`, `region` | Where the service runs. Domain mappings exist in some regions only |
| `host` | Domain to map |
| `service_name` | Name of the service, `module.app.service_name` |

| Output | Value |
| --- | --- |
| `env` | Empty |
| `trusted_proxies` | Empty: the app module adds `169.254.0.0/16`, Cloud Run's front end |
| `secret_env`, `permissions` | Empty |
| `service_ingress` | `all`, which the app must set: the traffic comes from the internet, so the `run.app` URL stays open too |
| `dns_records` | The records to add at your DNS provider |

Whoever applies must be a verified owner of the domain in Search Console, and the certificate can take up to 24 hours.

## The origin stays open

A domain mapping cannot add a header, and its traffic comes from the internet, so `run.app` stays open to anyone. A load balancer closes it on the platform, and Cloudflare closes it with a header that the server checks. Going from this module to either changes its block and the settings the plan asks for.
