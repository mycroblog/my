# stream-valkey

The broker of the real-time updates on Valkey Pub/Sub, for a deployment that has a Valkey already. It creates nothing, and follows
[the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** It has no provider and its tests only check the settings it returns. The server's adapter has
its own tests against Valkey. Unverified: Pub/Sub through any managed service, and everything that the README of the `kv-*` module
beside it lists.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "stream" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/stream-valkey?ref=v0.1.0&depth=1"

  url_secret_id = null # module.kv names it
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `url_secret_id` | required, may be null | Secret that holds the URL. **Null beside a `kv-*` module**, since one setting has one owner and two names fail the plan |
| `tls_ca_secret_id` | `null` | Secret that holds the PEM of a private CA, with `url_secret_id` |

| Output | Value |
| --- | --- |
| `env` | `MY_STREAM` |
| `secret_env` | `MY_VALKEY_URL`, unless it is null |
| `secret_files` | `MY_VALKEY_TLS_CA_FILE` when a CA is given |
| `permissions` | Empty: the app module grants access to every secret it is given |

The server has one Valkey setting for both uses, so `kv` and `stream` share a server, and a process that uses both shares one client.
The channel is `{valkey_key_prefix}stream`. To set the prefix with no `kv-*` module, pass `MY_VALKEY_KEY_PREFIX` through `env` of the app.

A process connects only while it has open streams, so this adds no traffic while nobody is connected, but the Valkey itself keeps
billing: `stream-valkey` makes sense when a Valkey is there for `kv` already. Otherwise `stream-gcp-pubsub`, `stream-aws-sns`, or
[`stream-postgres`](../stream-postgres/README.md) has no fixed cost. What reaching the server takes is in
[`kv-external`](../kv-external/README.md#what-you-must-provide-a-way-to-reach-it).
