# ingress-gcp-load-balancer

Traffic reaches the service through a global external Application Load Balancer, with a serverless network endpoint group,
a Google-managed certificate for `host`, and a redirect from HTTP to HTTPS. It has a monthly fee. It follows
[the ingress contract](../../README.md#the-ingress-contract).

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "ingress" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/ingress-gcp-load-balancer?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id`, `region` | required | Where the service runs |
| `host` | required | Domain of the certificate |
| `service_name` | required | Name of the service, `module.app.service_name` |
| `name` | `my-lb` | Start of the names of the resources |

| Output | Value |
| --- | --- |
| `env` | Empty |
| `trusted_proxies` | The load balancer's address. The app module adds `169.254.0.0/16` |
| `secret_env`, `permissions` | Empty |
| `service_ingress` | `INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER` |
| `ip_address` | The address to point DNS at |
| `dns_records` | The same address as an `A` record |

- **The service takes traffic from the load balancer only.** Otherwise anyone could go around it through the `run.app` URL.
  Cloud Tasks and Cloud Scheduler call `https://{host}`, so they arrive through the load balancer too.
- **Client addresses.** The load balancer writes `<what the client sent>,<client>,<its own address>` to `X-Forwarded-For`, so
  the server must trust its address to find the client. Enable `compute.googleapis.com` first.
- **The certificate** becomes active only after DNS points at the address, which can take a while.
- Nothing here has `prevent_destroy`: everything can be made again, and a new address only needs a new DNS record.
