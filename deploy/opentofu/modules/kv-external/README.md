# kv-external

The key-value store on a Valkey, or another server that speaks the Redis protocol, outside this configuration. It creates nothing,
and follows [the backend contract](../../README.md#the-backend-contract). `kv = valkey` is an option, not the default: `kv = db`
needs no module.

**This module has never been applied.** It has no provider and its tests only check the settings it returns. Whether your service
accepts what the server sends is what stays unverified.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "kv" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kv-external?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `url_secret_id` | required | Secret that holds the URL, which the operator created by hand: its ID in Secret Manager, its ARN in AWS Secrets Manager |
| `tls_ca_secret_id` | `null` | Secret that holds the PEM of a private CA. Unset, the server uses the system roots |
| `key_prefix` | `null` | Prefix of every key the server writes. Unset, the server's default, `my:` |

| Output | Value |
| --- | --- |
| `env` | `MY_KV`, and `MY_VALKEY_KEY_PREFIX` when it is set |
| `secret_env` | `MY_VALKEY_URL` |
| `secret_files` | `MY_VALKEY_TLS_CA_FILE` when a CA is given |
| `permissions` | Empty: the app module grants access to every secret it is given |

The URL is `valkey://[user:password@]host[:port][/db]`, or `valkeys://` for TLS. Use TLS whenever the path to the server leaves a
network you control. The server refuses `skip_verify`.

A CA reaches the server as a mounted file, so `tls_ca_secret_id` needs a platform that mounts secrets. On one that does not, such
as Lambda, the file's setting does not exist: use a service whose certificate chains to the system roots.

## What you must provide: a way to reach it

This module knows nothing about the network, and the server stops at start when it cannot reach the server named here. Either:

- **A service with a public TLS endpoint and a password.** Nothing to attach, and no fixed cost from the network. This is the only
  way to `kv = valkey` that adds no VPC, at whatever the service charges.
- **A server on private addresses.** Give the app a `network` that routes to it, and see
  [what that adds](../kv-gcp-memorystore/README.md#what-the-private-network-adds), and allow the port in its firewall or security group.

[`stream-valkey`](../stream-valkey/README.md) can use the same URL: it sits beside this module and names no URL of its own.
