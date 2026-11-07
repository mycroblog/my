# The plugin protocol

A plugin is a program of yours that the server calls to do a job in place of a built-in part, such as translating posts,
sending mail, converting media, or deciding whether to accept a post. This chapter describes the calls and messages of
each kind of plugin, as the protocol's definitions state them. [Plugins](../operations/plugins.md) explains how an
operator connects one: its settings, the kinds, authentication, plain TCP, and what happens when a call fails.

## The packages

The protocol is defined by the `.proto` files under [`api/plugin/`](../../api/plugin/), under the MIT license, so that a
plugin of any license may use them. There is one package for each kind of port, and the packages share no types. The
decision hooks all share one package.

| Page | Package | Kinds |
| --- | --- | --- |
| [Probe](probe.md) | `my.plugin.probe.v1` | `probe`, which every plugin may serve to check its connection |
| [Translator](translator.md) | `my.plugin.translator.v1` | `translator` |
| [Billing](billing.md) | `my.plugin.billing.v1` | `billing` |
| [Mailer](mailer.md) | `my.plugin.mailer.v1` | `mailer` |
| [Media](media.md) | `my.plugin.media.v1` | `media_image`, `media_video`, `media_audio` |
| [Key manager](key-manager.md) | `my.plugin.key_manager.v1` | `key_manager` |
| [Decision hooks](hooks.md) | `my.plugin.hook.v1` | `inbound.status`, `inbound.follow`, `signup.check`, `media.check`, `local.status`, `inbound.actor`, `local.profile` |

The pages are generated from the comments of the `.proto` files, which are the contract: what each call does, the order
of the messages of a stream, the limits, and what each field and value means.

## Calls

The server is the client and the plugin is the server. A plugin serves the Connect protocol, gRPC, or both, as the
operator's `plugin_<kind>_protocol` says:

- With `json`, an RPC is a `POST` to `/<package>.<Service>/<Method>` with a JSON body, which a plugin can serve with an
  HTTP server and a JSON library alone ([a plugin with no library](../operations/plugins.md#a-plugin-with-no-library)).
- With `proto` or `grpc`, a plugin uses a Connect or gRPC library, generated from the `.proto` files.
- The media kinds stream both ways, which needs HTTP/2, so they take `proto` or `grpc` only.

In JSON, the fields take proto3's standard mapping: an `int64` is a decimal string, `bytes` are base64, and an enum value
is its name. A field at its zero value may be missing in both directions, and the server ignores fields it does not
know. The result of a call is the `outcome` of its response: a missing or unknown `outcome` is a failure, never a
success.

## Testing a plugin

`my plugin test --kind <kind> <address>` calls a plugin as the server does and reports each case as passed, failed, or
skipped. It needs no build of yours against the server: run the `my` binary or its container image.
[Test a plugin](../operations/plugins.md#test-a-plugin) lists the cases of each kind and what they send.

## Versions

Each package has its own version, `v1`. Until the release `v0.1.0`, the packages may still change. From that release,
`v1` changes only in ways that a plugin written for an earlier release keeps working with: new fields that a plugin may
ignore, new optional calls, and looser limits. A change that a plugin must know of comes as a new package, `v2`, beside
`v1`.
