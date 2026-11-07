# Plugins

A plugin is a separate program or service that you run, to which the server hands one narrow job over the network.
The server only connects to an address: it does not start, stop, or watch the plugin. Run it as a sidecar, under systemd,
or on another machine.

Nothing is delegated by default. A server without a plugin address opens no connection and starts no process.
The build tag [`no_plugins`](../reference/build-tags.md) takes the plugin client out of the binary.

![Where plugins sit: the my process hands five ports to a plugin and asks five decision hooks, all through one plugin client that connects to the plugins you run; the bottom of the diagram shows what each does when a call fails. The list below describes each part.](images/plugins.svg)

From the top of the diagram down:

- **The my process**, with any `role`, makes two kinds of calls to a plugin.
  - **Ports it hands over**: a plugin does a job in place of a built-in part, and each is on only when a setting names
    `plugin`: `translator` (the instance setting `translation_providers = plugin`), `billing` (`billing_sources` includes
    `plugin`), `mailer` (`mail = plugin`), `media_image`, `media_video`, and `media_audio` (`media_image = plugin` and so
    on), and `key_manager` (`key_manager = plugin`).
  - **[Decision hooks](decision-hooks.md)**: a plugin answers whether to accept something, and is asked unless the hook's
    mode `hook_<hook>_mode` is `off`: `inbound.status` for posts from other servers, `inbound.follow` for follows from
    other servers, `signup.check` for sign-ups, `media.check` for uploads, `local.status` for posts of local accounts,
    and `inbound.actor` for the profiles of accounts on other servers.
- **The plugin client** makes every call. Each kind has one destination, `plugin_<kind>_address`, and a deadline,
  `plugin_<kind>_timeout`, and runs at most 8 calls at once (2 for each media kind). It connects over a Unix socket, `https://`, or,
  for [some kinds](#plain-tcp) only, `http://`.
- **Your plugins** are separate programs that you run, as a sidecar, under systemd, or on another machine. The server only
  connects to them.
- **When a call fails or times out** ([details](#when-a-call-fails)):
  - `billing`, `mailer`, and the media kinds: the job that made the call runs again later.
  - `translator`: the server asks the next provider, and when none is left the reader sees that the translation failed.
  - `key_manager` fails closed: whatever needs a key fails, and the server does not start without the plugin.
  - A decision hook does what `hook_<hook>_on_failure` says: `pass`, `retry`, or, for sign-ups, `require_approval`.

## Kinds

Each kind has its own destination, under the settings `plugin_<kind>_*`. A dot in the name of a kind becomes `_` in the
key, so `inbound.status` has `plugin_inbound_status_address`.

| Kind | What it carries | Plain TCP (`http://`) | Default protocol | Default deadline |
| --- | --- | --- | --- | --- |
| `translator` | The text of a post that a reader asked to translate | **Refused** | `json` | 10 s |
| `billing` | The costs of the instance | Allowed | `json` | 2 min |
| `mailer` | Addresses and secrets such as confirmation links | **Refused** | `json` | 30 s |
| `media_image` | Uploaded images, with their metadata, which may hold a location | Allowed | `proto` | 4 min |
| `media_video` | Uploaded videos, with their metadata, which may hold a location | Allowed | `proto` | 4 min |
| `media_audio` | Uploaded audio files, with their metadata | Allowed | `proto` | 4 min |
| `key_manager` | Data-encryption keys in plaintext | **Refused** | `json` | 5 s |
| `inbound.status` | The text of incoming posts | **Refused** | `json` | 3 s |
| `local.status` | The text of local posts | **Refused** | `json` | 1 s |
| `inbound.follow` | The two parties of a follow | Allowed | `json` | 3 s |
| `inbound.actor` | The public profiles of accounts on other servers | Allowed | `json` | 3 s |
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

The server reads the files of `plugin_<kind>_tls_ca_file`, `plugin_<kind>_tls_cert_file`, and
`plugin_<kind>_tls_key_file` at start, where a missing or unreadable file stops it. After that it looks at them again
for a change at most every 5 seconds, when it opens a new connection, so a short-lived certificate or a CA that a
certificate manager such as Vault, cert-manager, or SPIFFE renews in place works with no restart. Connections already
open keep the certificate they started with until they close. While a renewal is half written, such as a new
certificate whose key is not in place yet, the server keeps the files it read last and tries again on the next
connection. The token is read at start only.

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

A plugin answers the wrong protocol with `415`. The media kinds stream both ways, which needs HTTP/2, so they take `proto`
or `grpc` and `json` is a configuration error.

The protocol's definitions are the `.proto` files under `api/plugin/` of the repository. There is one package for each
kind of port, and they share no types; the decision hooks all share one package, `my.plugin.hook.v1`.
[The plugin protocol](../plugins/overview.md) has the reference of every package, generated from those files.

## When a call fails

A call waits for its turn among the calls of its kind that run at once, and the wait counts in its deadline and in the
metric [`my.plugin.slot_wait`](running.md). A call that fails counts in the metric [`my.plugin.failures`](running.md) by kind and Connect code. The server drops the
message that a plugin puts in an error, so a plugin's failures have their reasons in the plugin's own log. The server
does not retry a call itself: the job that made it does, so a plugin must cope with the same call twice.

A plugin that is down when the server starts stops nothing, except `key_manager`: the server does not start without it, and
[what stops without it](key-management.md#when-the-plugin-is-unreachable) is a part of its setup to read first.

A call has the deadline of `plugin_<kind>_timeout`, and a plugin receives it as `Connect-Timeout-Ms` (`grpc-timeout` on
gRPC). A plugin may ignore that header: the server stops waiting anyway. The deadline must stay below 30 seconds for the
kinds that a request can wait for (`key_manager`, `translator`, `signup.check`, `media.check`, `local.status`, and `inbound.actor`), and
below 5 minutes, how long a job may run, for the others.

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

`--kind translator` sends a few fixed English sentences, translating them to Japanese (`--translate-from` and `--translate-to`
choose another pair for a plugin that has only some). A service behind the plugin that bills per character bills these too,
about 200 characters. It checks the form of the answers, and that the plugin refuses the language `qaa`, a code that ISO 639
leaves for private use. It cannot judge the words, so read a translation of the plugin yourself.
[Translating posts](#translating-posts) has the contract of the kind.

`--kind media_image` and `--kind media_audio` send only small test files that they make themselves: a photo, an emoji, and
an animated GIF, or an MP3. Each one hides a location or a title in its metadata, and a case fails when that comes back in
an output. They check every output as the server does. `--kind media_video` has no cases of its own yet; it checks only
that the plugin answers. [Converting media](#converting-media) has the contract of the kinds.

`--kind key_manager` runs the checks the server runs at each start, and checks that the plugin refuses a changed value and a
KEK it does not hold. It sends only keys it makes up and throws away, so it is safe against a real key store, which may bill
the calls. It checks that the answers have the right form, not how the plugin keeps its KEK.
[Managing the key](key-management.md#a-plugin) has the contract of the kind.

`probe` is the smallest kind, and the server delegates nothing to it. Serve it first, to check the transport, the
protocol, and the authentication before you write the rest of the plugin.

## Translating posts

A plugin of the kind `translator` lets the server translate a post when a reader asks, through a service you choose
other than the [built-in providers](translation.md) (LibreTranslate, DeepL, Google Cloud Translation, and Amazon
Translate). Without a server-side provider, the only translation is the one built into the reader's browser, which exists
only in desktop Chrome, Edge, and Opera: readers on a phone, in Firefox, or in Safari get no translate button until you
enable one.

```sh
MY_PLUGIN_TRANSLATOR_ADDRESS=unix:///run/my/translator.sock
MY_PLUGIN_TRANSLATOR_NAME="Our translator"    # what readers see next to a translation
```

1. Set `plugin_translator_address` and `plugin_translator_name`, and restart. The name is required, since readers trust a
   translation according to where it comes from. The address never leaves the configuration.
2. [Update your privacy policy](privacy-policy-template.md#the-services-you-choose), and name the service behind the plugin
   as a translation service: what the plugin passes on is what it receives.
3. Enable it with the [instance setting](instance-settings.md) `translation_providers = plugin`. A change takes effect at once.

Having an address enables nothing: until the setting names `plugin`, the server sends nothing and readers see no button.

The plugin is one provider like any other. [`translation_provider_visibilities`](instance-settings.md) decides what it may
receive, the monthly limits count what it is sent, and a translation it made stays in the server's cache like another.
With `translation_reports_to_server` on, it also receives what moderators ask to translate in a report, by the same visibilities.
It can come before or after another provider in the order of `translation_providers`.

### What the plugin receives

The plugin receives the text of a post when a reader asks for a translation: its body, content warning, poll options, and
image descriptions, and the language of the text and the language to translate into. It does not receive the post's address,
its author, the reader, mentions, hashtags, links, custom emoji, or code: numbered markers stand in for them.

By default that is `public`, `local`, and `unlisted` posts. **`private` and `direct` posts reach the plugin only when you allow
them**, with `translation_provider_visibilities = plugin=public|local|unlisted|private|direct`. Then the plugin, and any
service behind it, reads text that the author meant for few people, so allow them only if you control where the text goes,
and say so in your privacy policy. The server cannot recall text it has sent.

The server refuses `http://` for this kind, also on loopback, because the text is the text of posts. Use a Unix socket or
`https://`.

### An LLM behind a plugin

The server has no LLM provider and offers none, on purpose. Anyone can write a post, so the text to translate is a way to
give an LLM instructions (a prompt injection), and readers take a translation for what the post says. Sanitizing protects
the HTML of a page: it cannot stop a translation from saying something that the post does not say.

The server cannot see what a plugin calls. **If your plugin uses an LLM, this risk and its consequences are your own
responsibility.**

### The contract

The RPC is `Translate` of `my.plugin.translator.v1.TranslatorService`:

```text
POST /my.plugin.translator.v1.TranslatorService/Translate
Content-Type: application/json

{"fragments":["Good morning ⟦1⟧, this is ⟦2⟧important⟦/2⟧.","⟦3⟧"],"sourceLanguage":"en","targetLanguage":"ja"}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","translations":["⟦1⟧さん、おはようございます。これは⟦2⟧重要⟦/2⟧です。","⟦3⟧"],"detectedLanguage":"en"}
```

- `sourceLanguage` is a BCP 47 tag, or `und` when the server does not know the language. The plugin then detects it and reports
  it in `detectedLanguage`, or answers `OUTCOME_UNSUPPORTED_LANGUAGE`. `detectedLanguage` may be missing when the plugin has
  nothing to add, and the server ignores a value that is no language tag.
- `translations` has one entry per fragment, in the order of the fragments, none of them empty. A fragment can be nothing but
  markers, and its translation is then those markers.
- `⟦1⟧` stands for one item or opens a pair that `⟦/1⟧` closes. Each marker of a fragment comes back exactly once in its
  translation, unchanged. Items may move as word order requires, but a pair stays open before it closes, and pairs nest.
- A translation is plain text. The server escapes any markup in it, and sanitizes the result before it stores or shows it.

| `outcome` | Meaning | What the server does |
| --- | --- | --- |
| `OUTCOME_OK` | The translations are in the response | Checks them, stores them, and answers the reader |
| `OUTCOME_TEMPORARY` | The service failed or did not answer | Tries the next provider |
| `OUTCOME_PROVIDER_LIMIT` | The service reports its own rate limit or quota | Tries the next provider |
| `OUTCOME_UNSUPPORTED_LANGUAGE` | The service does not have the source or target language, or cannot detect it | Tries the next provider, and tells the reader if none has it |

An answer that breaks these rules counts as `OUTCOME_TEMPORARY` and nothing of it is stored, like a missing or unknown
`outcome` and a failed call. When no provider answers, the reader sees that the translation failed and the post stays as it is, unless
`translation_browser` is on and the reader's browser can translate the post: the web client then translates it there.
The characters of each call count against the monthly limits, also of a call that fails, since the service may have received them.

The server stops waiting after `plugin_translator_timeout`, and after 20 seconds whatever that setting says. All the
providers of one request get 45 seconds in all. It does not retry a
failed call, and the same fragments can come again for another reader.

## Converting media

A media plugin does the heavy part of processing uploads, so that it can run on another machine. There are three kinds,
and you choose each one on its own:

- `media_image` re-encodes images into their variants.
- `media_video` remuxes videos: files with a video track, with or without sound.
- `media_audio` remuxes audio-only files.

The server still does the rest itself. It detects the format of each upload, tells videos from audio files, checks each
upload against the limits when it arrives, and makes the previews of remote media. A kind whose setting stays `native` is
processed in the server.

```sh
# Only videos go to a plugin, on a machine of its own; images and audio stay in the server.
MY_MEDIA_VIDEO=plugin
MY_PLUGIN_MEDIA_VIDEO_ADDRESS=https://media.internal:9000
```

Set `media_<kind> = plugin` and `plugin_media_<kind>_address` for each kind you hand over, and restart. One plugin may
take more than one kind: give it the same address for each. Each kind runs two calls at once at most, and each call lasts
`plugin_media_<kind>_timeout` (4 minutes by default) at most, so a slow video does not hold up the images.

- **Removing metadata is the plugin's job.** The uploads reach the plugin as they were sent, with their EXIF data and
  tags, which may hold where a photo was taken. The server checks the format and the size of every output, but it cannot
  tell that every trace of metadata is gone. `my plugin test --kind media_image` checks that the location of its test files
  does not come back; check the rest in the plugin.
- **Plain TCP sends the uploads unencrypted.** The media kinds accept `http://`, with a warning at start. Uploads, with their
  metadata, then cross the network in the clear, and nothing authenticates either end: anyone on that network can read
  them or answer for the plugin. Use a Unix socket or `https://` unless the network is yours alone.
- **The server does not trust the output.** It detects the format of each output from its content, never from what the
  plugin declares, and serves it with the content type and file extension of that format. It decodes each output under
  the same limits as an upload, and checks that it fits its size. An output that is larger than declared or than those
  limits, of a format the plugin did not declare, or of a format the variant does not take, fails the upload as a file
  the server cannot process.
- **A failure is a delay.** When the plugin is down or fails, the job retries later, as with the built-in processor. An
  action that needs a custom emoji's image at once, such as copying a remote emoji or accepting a request for one, fails
  while the plugin does.

### The contract

The service is `my.plugin.media.v1.MediaService`, with the RPCs `Process` for an image and `Remux` for a video or audio file.
Both stream both ways, so the plugin needs a Connect or gRPC library and HTTP/2: `proto` or `grpc`, over TLS with ALPN
`h2`, or over cleartext HTTP/2 (h2c) on a Unix socket or plain TCP.

A call is half-duplex:

1. The server sends a `head`: the limits, and for an image its purpose, the boxes of its three sizes, and its focal
   point. The file follows in `chunk` messages of 1 MiB at most. The server then closes its side.
2. The plugin answers with a `head` whose `outcome` is the result. With `OUTCOME_OK`, the head of `Process` also holds the
   dimensions of the image after its EXIF orientation and the blurhash of the small still output.
3. For each output, the plugin sends a `part`, which names the output and declares its format, dimensions, and length,
   followed by `chunk` messages of 1 MiB at most.

`Process` returns the small, medium, and large still outputs, and the three moving ones of an image of more than one
frame. The small and medium outputs are WebP; the large still one is a JPEG or a PNG (always a PNG for a custom emoji),
and the large moving one a GIF. `Remux` returns the file in its own container without metadata, and the cover picture it
embeds, if any. The server reads at most twice the image limit from `Process`, and from `Remux` the video limit for the
file and the image limit for its cover.

| `outcome` | Meaning | What the server does |
| --- | --- | --- |
| `OUTCOME_OK` | The outputs follow | Checks them and stores them |
| `OUTCOME_TEMPORARY` | The plugin failed | Retries the job later |
| `OUTCOME_UNSUPPORTED_FORMAT`, `OUTCOME_UNSUPPORTED_CODEC`, `OUTCOME_TOO_LARGE`, `OUTCOME_DIMENSIONS`, `OUTCOME_CORRUPT`, `OUTCOME_TOO_LONG` | The file is not acceptable | Marks the upload as failed |

A missing or unknown `outcome` and a failed call count as `OUTCOME_TEMPORARY`.

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
