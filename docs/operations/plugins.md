# Plugins

A plugin is a separate program or service that you run, to which the server hands one narrow job over the network.
The server only connects to an address: it does not start, stop, or watch the plugin. Run it as a sidecar, under systemd,
or on another machine.

Nothing is delegated by default. A server without a plugin address opens no connection and starts no process.
The build tag [`no_plugins`](../reference/build-tags.md) takes the plugin client out of the binary.

## Kinds

Each kind has its own destination, under the settings `plugin_<kind>_*`. A dot in the name of a kind becomes `_` in the
key, so `inbound.status` has `plugin_inbound_status_address`.

| Kind | What it carries | Plain TCP (`http://`) | Default protocol | Default deadline |
| --- | --- | --- | --- | --- |
| `translator` | The text of a post that a reader asked to translate | **Refused** | `json` | 10 s |
| `billing` | The costs of the instance | Allowed | `json` | 2 min |
| `mailer` | Addresses and secrets such as confirmation links | **Refused** | `json` | 30 s |
| `media` | Uploads, with their metadata, which may hold a location | Allowed | `proto` | 4 min |
| `key_manager` | Data-encryption keys in plaintext | **Refused** | `json` | 5 s |
| `inbound.status` | The text of incoming posts | **Refused** | `json` | 3 s |
| `local.status` | The text of local posts | **Refused** | `json` | 1 s |
| `inbound.follow` | The two parties of a follow | Allowed | `json` | 3 s |
| `signup.check` | Email addresses and IP addresses | **Refused** | `json` | 2 s |
| `media.check` | A small processed version of the media, never the original | Allowed | `json` | 10 s |

The settings that make the server use a destination belong to each feature: the last five kinds of the table are the
[decision hooks](decision-hooks.md). This page covers what they share.

The server sends at most a kind's fixed limit per message and reads at most another. A call over the limit fails, and
the job that made it retries.

## The destination

The settings exist only in the configuration file and the environment
([configuration reference](../reference/configuration.md)). No instance setting, admin page, or API can show or change
them, so a stolen administrator account cannot redirect posts or keys. A change takes effect on restart.

`plugin_<kind>_address` has one of three forms. The server checks the form at start without resolving the name, and
resolves the name for each connection.

- `unix:///run/my/mailer.sock`: a Unix socket.
- `https://mailer.internal:8443`: TLS over TCP. The server always verifies the certificate, against
  `plugin_<kind>_tls_ca_file` or the system's roots, and no setting turns that off.
- `http://billing.internal:5000`: plain TCP.

### Authentication

Choose one:

- **A Unix socket:** the permissions of the socket's file protect it.
- **mTLS:** `plugin_<kind>_tls_cert_file` and `plugin_<kind>_tls_key_file`, with an `https://` address.
- **A bearer token:** `plugin_<kind>_token` (or `MY_PLUGIN_<KIND>_TOKEN_FILE`), sent as `Authorization: Bearer`, over
  `https://` or a Unix socket only. A token with an `http://` address is a configuration error.

An `https://` address with neither a token nor a client certificate starts with a warning, since the plugin cannot tell
the server from anyone else who reaches it.

### Plain TCP

`mailer`, `key_manager`, `translator`, `inbound.status`, `local.status`, and `signup.check` refuse `http://`: the server does not start. That holds on
loopback too (`http://127.0.0.1`), since a loopback address does not keep other processes on the host from reading the
traffic. Put such a sidecar on a Unix socket in a shared volume.

The other kinds accept `http://`, and the server logs a warning at start that names the kind and the address. Nothing
encrypts or authenticates that traffic, and no bearer token can be used over it. Keeping that network closed is your
responsibility: what the table above lists crosses it in the clear.

## Protocols

`plugin_<kind>_protocol` says what the server sends, and it must be what the plugin accepts.

| Value | The server sends | For |
| --- | --- | --- |
| `json` | Connect's unary protocol with `application/json`, over HTTP/1.1 on plain connections | A plugin built on a Connect library, or on an HTTP server and a JSON library alone |
| `proto` | Connect's protocol with `application/proto` | A plugin built on a Connect library |
| `grpc` | gRPC over HTTP/2 | A plugin that speaks only gRPC |

A plugin answers the wrong protocol with `415`. `media` streams both ways, which needs HTTP/2, so it takes `proto` or
`grpc` and `json` is a configuration error.

The protocol's definitions are the `.proto` files under `api/plugin/` of the repository. There is one package for each
kind, and they share no types.

## When a call fails

A call waits for its turn among the calls of its kind that run at once, and the wait counts in its deadline and in the
metric [`my.plugin.slot_wait`](running.md). A call that fails counts in the metric [`my.plugin.failures`](running.md) by kind and Connect code. The server drops the
message that a plugin puts in an error, so a plugin's failures have their reasons in the plugin's own log. The server
does not retry a call itself: the job that made it does, so a plugin must cope with the same call twice.

A call has the deadline of `plugin_<kind>_timeout`, below 5 minutes, and a plugin receives it as `Connect-Timeout-Ms`
(`grpc-timeout` on gRPC). A plugin may ignore that header: the server stops waiting anyway.

## Test a plugin

`my plugin test` runs the contract of a kind against a running plugin and reports each case. It reads no configuration,
so all it needs is the `my` binary.

```sh
my plugin test --kind probe unix:///run/my/plugin.sock
my plugin test --kind probe --protocol grpc --tls-ca-file ca.pem --token-file token https://plugin.internal:8443
```

```text
PASS reachable
PASS echo
FAIL echo_empty: the outcome is OUTCOME_UNSPECIFIED, not OUTCOME_OK
2 passed, 1 failed, 0 skipped
```

The exit code is 1 when a case fails. The flags mean what the settings mean: `--protocol`, `--token-file`, `--tls-ca-file`,
`--tls-cert-file`, `--tls-key-file`, and `--timeout`. It applies the same rules as the server, so `--kind mailer` refuses an
`http://` address. The plugin under test can be a real one, so a case that does more than the call itself runs only when
a flag asks for it. A kind whose contract the version does not have yet reports `reachable` and a `SKIP`.

`--kind billing` checks only that the plugin refuses to spend anything when no billed request is allowed. `--billed` also
makes a real fetch of the current and the previous month, and checks the answer as an import does; it may cost money at
the providers behind the plugin. [Importing cloud usage costs](billing-import.md#a-plugin-billing_sources--plugin) has the
contract of the kind.

`--kind mailer` makes no call of the RPC unless `--mail-to` names a recipient. With it, the command sends one real message there,
from `--mail-from` (default: the same address), and checks that the plugin refuses a request with no recipient.
[Sending email](email.md#a-plugin-mail--plugin) has the contract of the kind.

`probe` is the smallest kind, and the server delegates nothing to it. Serve it first, to check the transport, the
protocol, and the authentication before you write the rest of the plugin.

## A plugin with no library

With `json`, a plugin needs an HTTP server and a JSON library alone. An RPC is a `POST` to
`/<package>.<Service>/<Method>` with `Content-Type: application/json`, and the plugin answers `200` with a JSON object:

```text
POST /my.plugin.probe.v1.ProbeService/Echo
Content-Type: application/json
Connect-Protocol-Version: 1

{"payload":"AP8=","number":"9007199254740993"}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","payload":"AP8=","number":"9007199254740993"}
```

The JSON follows proto3's standard mapping:

- An `int64` is a decimal string, `bytes` are base64, and an enum is its name.
- A field at its zero value may be missing, in both directions. A plugin must read a missing field as zero.
- The server ignores fields it does not know.
- A result is the `outcome` of the response. A missing or unknown `outcome` is a failure, never a success.
- A plugin that fails answers an HTTP error, or Connect's `{"code":"unavailable"}` with the matching status. The job retries.

`Connect-Timeout-Ms` and `Connect-Protocol-Version` may be ignored.
