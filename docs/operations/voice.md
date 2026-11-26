# Run voice chat

Voice chat lets signed-in users open a room, invite others and talk. It is off until you choose how the audio travels.
This guide covers that choice, the STUN and TURN servers or the media nodes each choice needs, and what to know about deployment, privacy and moderation.
Every setting named here is described in the [configuration reference](../reference/configuration.md#voice).

## Choose how the audio travels

`voice` picks one method for the whole instance; every room uses it.

| `voice` | What carries the audio | Fits |
| --- | --- | --- |
| `off` | Nothing. The voice chat API answers `404`, the web client shows no voice chat, and the other `voice_*` settings are not checked at start. | The default. |
| `p2p` | The participants' browsers send audio straight to each other. The server relays only the messages that set up the connections. | Small rooms and the cheapest setup: no extra server except an optional STUN or TURN server. |
| `sfu` | The server's own media nodes: each browser sends its audio to the node that holds its room, which forwards the speakers' audio to the participants. | Larger rooms, and strict control over who can speak. |

![The two methods side by side. With p2p, the server relays the signals and the browsers send audio to each other, or through a TURN server. With sfu, the browsers get their grants from the server and send signals and audio to the media node of their room. The list below describes each method.](images/voice-methods.svg)

In every method the server (`my serve`) holds the rooms, who takes part in them and their sessions, and hands each
participant a grant: what the browser needs to connect. What differs is where the signaling (the messages that set up a
connection) and the audio go:

- **`p2p`.** A browser sends its signals (offers, answers and ICE candidates) to the server, which passes them to the
  other browser over the room's event stream; a signal waits in the key-value store for at most 30 seconds. The audio
  goes straight from browser to browser. The grant lists the STUN servers of `voice_stun_urls` and the TURN server of
  `voice_turn` (`coturn` or `cloudflare`) with short-lived credentials. A browser learns its public address from STUN,
  and the TURN server relays the audio when the two browsers find no direct path.
- **`sfu`.** The server places each room on a media node (a selective forwarding unit, or SFU): a process with
  `role = all` or `sfu`. The grant holds the `wss:` URL of that node (`voice_sfu_public_url`) and a single-use ticket.
  The browser sends its signals over a WebSocket to the node and its audio to the node's UDP port `voice_sfu_udp_port`
  (TCP `voice_sfu_tcp_port` when UDP is blocked), and the node forwards the audio of the speakers to the other
  participants. The TURN server of `voice_turn` relays for the browsers that cannot reach the node directly.

Notes on the choice:

- **`p2p`** needs no UDP port on the server: the audio never passes through it (it passes through the TURN server when one is used), and the server's load is only the room messages, heartbeats and streams. It fits a small instance, small rooms, and a deployment on Cloud Run alone.
- **`sfu`** enforces on the server who may speak: the node forwards only the audio of the speakers. The browsers reach only the media node, so participants' IP addresses are visible to the node, not to the other participants. It needs a machine with a public IP and open UDP and TCP ports for each node ([Media nodes](#media-nodes)). `voice_stun_urls` is not needed, since a node has a public address, and `voice_ice_transport_policy = relay` is refused.
- Serve the instance over HTTPS: browsers allow microphone access only on secure pages.

The limits keep the cost of the rooms bounded. `0` removes any of them except the last:

| Setting | Default | Meaning |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | The longest a room stays open. A room that reaches it is ended. |
| `voice_max_open_rooms` | `10` | How many rooms may be open at once on this server. A room opened for a scheduled time counts from its opening, while it waits for its first participant. |
| `voice_max_participants` | `10` | The largest room, from 2 to 500 (50 with `p2p`). A host may set a smaller maximum for their room. |

Who may open rooms is the instance setting `voice_room_starters` ([instance settings](instance-settings.md)): by default only holders of the permission `start_voice_rooms` (the built-in `admin` role has it); `everyone` lets every signed-in local user who is not suspended or limited open rooms.

## Peer-to-peer rooms

### Try it with Docker Compose

To try the voice chat on your machine with the Docker Compose setup ([`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml)),
set `MY_VOICE=p2p` in `.env` and recreate the server.

```sh
cd deploy/docker
docker compose up -d
```

- Open the instance at `http://localhost:8080` (the `MY_WEB_ORIGIN` of `example.env`). Browsers let a page use the microphone only on
  `localhost` or over HTTPS, so an address such as `http://192.168.1.10:8080` cannot speak.
- Browsers on the same machine (two browser profiles, or a normal and a private window) connect to each other without a STUN or TURN
  server. To reach browsers on other machines, put the instance behind HTTPS and set `MY_VOICE_STUN_URLS` in `.env`.
- Only the `admin` role may open rooms by default. Sign in as the administrator you created, or set `voice_room_starters` to `everyone`
  on the "Voice chat" page of the server settings to let every user open rooms.

### Size and bandwidth

Two browsers connect to each other when at least one of them is speaking; two people who only listen do not connect.
A speaker therefore sends a separate copy of their audio to every other participant. Speech in Opus takes roughly 30 to 50 kbit/s with overhead, so a speaker in a room of 10 uploads about 9 copies, a few hundred kbit/s,
and a speaker in a room of 50 uploads 49 copies, around 2 Mbit/s, which a phone on mobile data or a home connection may not sustain. The number of connections in a room of N people is at most N(N-1)/2: 45 for 10 and 1,225 for 50.
That is why the default is 10. Raise `voice_max_participants` only when your users have the upload bandwidth; use `sfu` for large rooms.

### STUN and TURN

A browser behind NAT needs a STUN server to learn its public address, and a TURN server to relay the audio when no direct path exists (strict NATs, corporate firewalls, some mobile networks).

- `voice_stun_urls`: comma-separated `stun:` or `stuns:` URLs. Any public STUN server works, and the coturn server below answers STUN too.
- `voice_turn`: `none`, `coturn` or `cloudflare`.

With neither, the server warns at start: only browsers on the same network connect. A public instance wants STUN, and TURN for the users who cannot connect directly.

The server hands each participant short-lived TURN credentials, never a shared password. `voice_turn_credential_ttl` (default `6h`) sets how long they last. The web client asks for new credentials ten minutes before they expire and restarts ICE, so a call does not stop when they run out.

#### coturn

Set `voice_turn = coturn`, `voice_turn_coturn_urls` and `voice_turn_coturn_secret`. The server computes the credentials itself (the username is the expiry time and the session ID, the password is an HMAC-SHA1 of it with the secret), so it makes no request to the TURN server.
Configure coturn to match, in `turnserver.conf`:

```conf
# Time-limited credentials signed with a shared secret.
use-auth-secret
static-auth-secret=<the same value as voice_turn_coturn_secret>
realm=turn.example.org

listening-port=3478
tls-listening-port=5349
# If the host is behind NAT (most cloud VMs), tell coturn its public address.
external-ip=203.0.113.10

# Do not let the relay reach your internal networks.
no-multicast-peers
denied-peer-ip=0.0.0.0-0.255.255.255
denied-peer-ip=10.0.0.0-10.255.255.255
denied-peer-ip=100.64.0.0-100.127.255.255
denied-peer-ip=127.0.0.0-127.255.255.255
denied-peer-ip=169.254.0.0-169.254.255.255
denied-peer-ip=172.16.0.0-172.31.255.255
denied-peer-ip=192.168.0.0-192.168.255.255
```

- `denied-peer-ip` matters. Without it, anyone with a credential can use the TURN server to reach hosts on its internal network (cloud metadata services, databases, other VMs). Deny every private, loopback and link-local range; allow an internal address (`allowed-peer-ip`) only if you really need to relay to it.
- Open UDP and TCP 3478, TCP 5349 for `turns:`, and coturn's relay port range (`min-port` / `max-port`, by default 49152-65535).
- List a `turns:` URL or one with `?transport=tcp` in `voice_turn_coturn_urls` as well, so users on networks that block UDP can still connect.
- coturn checks a credential's expiry only when an allocation is first created. An allocation that already exists keeps working past it, and the new credentials take effect for the next allocation, which the web client creates by restarting ICE.

#### Cloudflare TURN

Set `voice_turn = cloudflare`, `voice_turn_cloudflare_key_id` and `voice_turn_cloudflare_api_token`, from a TURN key created in Cloudflare's dashboard. The server asks Cloudflare for each participant's credentials when they open or join a room (if that fails, the request answers `503` and opens or joins nothing, so it can be tried again) and revokes them when the participant leaves or is removed.

- Credentials last at most 48 hours, so `voice_turn_credential_ttl` may not exceed `48h` (the server refuses to start otherwise).
- **Cloudflare disconnects a relay shortly after its credentials expire.** The web client renews them before that, so keep `voice_turn_credential_ttl` well above ten minutes.
- Pricing, as checked on 2026-10-26: the first 1,000 GB per month are free, then $0.05 per GB sent to clients. Check Cloudflare's current pricing before relying on it.

### Hiding IP addresses

With `voice_ice_transport_policy = all` (the default) browsers connect directly when they can, so the participants of a room may learn each other's IP addresses.

With `relay`, browsers use only the TURN server, so a participant sees the TURN server's address and not the other participants'. The audio stays encrypted between the browsers. What `relay` does not hide: whoever runs the TURN server (you, or Cloudflare) sees every participant's address, and this server sees the address of everyone who calls its API.
`relay` needs `voice_turn` to be `coturn` or `cloudflare` (the server refuses to start otherwise), and it sends every call through the TURN server, which is your bandwidth with coturn and your transfer bill with Cloudflare.

### What P2P cannot enforce

Only some participants may speak in a room: the hosts and the listeners a host allows. In `p2p` this is enforced in two places: the server refuses to relay a connection offer that would send audio from someone who may not speak, and the other browsers do not play audio from a participant who is not a speaker.

That stops ordinary users and the official web client, but it is not a complete defense. **Two participants with modified clients who work together can still exchange audio**: a modified client of someone who may not speak can send it, and a modified client of a receiver can play it, and the server cannot see inside the encrypted media.
If you must guarantee who speaks, for example for moderated public events, use `sfu`, where the media node itself forwards only the audio of the speakers.

## Media nodes

With `voice = sfu`, every room is held by one media node, whose process forwards the room's audio. A room never spans
nodes.

### Run media nodes

- **`role = all`** runs a node in the same process as everything else, at `wss://<host>/voice/sfu`. One machine with
  the UDP and TCP ports open is enough.
- **`role = sfu`** runs a node of its own, beside the `web` and `worker` processes. It shares their database, their
  `stream` (not `hub`) and their `kv` (not `memory`): the nodes register themselves and their rooms in the key-value
  store, and learn of changes to their rooms through the stream. It runs no jobs and serves only the health checks and
  the node's WebSocket. Run as many as you need: a `web` process places each new room on the node with the fewest
  places reserved that has room for it. A `web` process runs no node.

What each node needs:

| What | Setting | Notes |
| --- | --- | --- |
| A public IP address | `voice_sfu_public_ip` behind NAT | The node gives the browsers this address, or its own address when it is not set. A cloud VM or a container usually has a private address of its own: set it. |
| The UDP port, open to the internet | `voice_sfu_udp_port` (7882) | All sessions of the node share it. |
| The TCP port, open to the internet | `voice_sfu_tcp_port` (7881) | For browsers on networks that block UDP. |
| A `wss:` URL with TLS | `voice_sfu_public_url` (`wss://<host>/voice/sfu`) | Each node has its own, such as `wss://sfu1.example.org/voice/sfu`. A reverse proxy in front terminates TLS and forwards the WebSocket, or the node serves HTTPS itself: the `tls` settings work on `role = sfu`, for the host of `voice_sfu_public_url` ([Serving HTTPS from the server](running.md#serving-https-from-the-server)). |
| How many participants it holds | `voice_sfu_node_capacity` (500) | See [Capacity](#capacity). |

Forward the UDP and TCP ports to the node unchanged: the node gives the browsers its own port numbers. A node on
another host than `host` also needs `voice_sfu_origins` on the `web` processes, such as `wss://*.sfu.example.org`, so
that the web client's Content Security Policy allows the connection.

### Docker Compose

The layer [`compose.sfu.yaml`](../../deploy/docker/compose.sfu.yaml) runs the server as `role = web`, a `worker`, and
a media node `sfu` on one machine, on PostgreSQL behind Caddy ([Docker Compose files](running.md#docker-compose-files)):

```sh
# In .env:
# COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml:compose.sfu.yaml
# MY_VOICE_SFU_PUBLIC_IP=203.0.113.10   the public address of this machine
docker compose up -d
```

- Caddy sends `/voice/sfu` to the node and every other request to the server, so the node's URL is the default
  `wss://<MY_HOST>/voice/sfu` and `voice_sfu_origins` is not needed.
- Open UDP 7882 and TCP 7881 in the machine's firewall, besides 80 and 443.
- The layer sets `voice = sfu` and `stream = postgres` for the three processes, whatever `.env` says.

### Capacity

A room reserves its maximum (the host's maximum for the room, or `voice_max_participants`) on its node when it is
placed, however many have joined. A node takes a room only while the sum of its reservations stays within
`voice_sfu_node_capacity`, so a node never holds more participants than its capacity. When no node has room for a
room's maximum, opening the room or its first join answers `503`, and the user can try again later.

Set `voice_sfu_node_capacity` from what the machine can forward ([Measured cost](#measured-cost)), and keep
`voice_max_participants` no larger than your rooms need: a room of 500 reserves 500 places even with 3 people in it.

### Measured cost

Measured with simulated clients sending Opus audio at 50 packets per second, on an AMD Ryzen 9 7900 (12 cores) with
Linux under WSL2 and Go 1.27, everything on loopback. CPU is in cores of that machine; memory is the node's heap and
goroutine stacks on top of the rest of the process.

| What | CPU | Memory |
| --- | --- | --- |
| An idle node | none measurable | about 0.1 MiB |
| Each open room | none measurable | about 24 KiB |
| Each participant | about 0.0002 cores while silent | about 200 KiB |
| Each forwarded stream (a speaker heard by one participant) | 20 to 27 µs per packet, about 0.001 cores | about 24 KiB |
| A room of 100 with 3 speakers making sound | about 0.3 cores | about 25 MiB |

A faster or slower machine scales these figures; a node's own metrics ([Metrics to watch](#metrics-to-watch)) show
what it carries.

The cost of forwarding follows the packets the node sends: the audio of every speaker goes to every other
participant, so a room of N participants with K speakers sends about K × (N − 1) streams of 30 to 50 kbit/s. A room of
many listeners and few speakers is cheap; a room where everyone speaks at once costs the most. Plan the node's
outbound bandwidth the same way.

### Drain a node for a deploy

On SIGTERM a node drains before it stops:

1. It stops taking new rooms.
2. It places each of its rooms on another node and tells the room's clients to move. They reconnect to the new node
   with a new grant, and the audio stops for about 1 to 2 seconds.
3. It waits until its clients have left, for at most 20 of the 25 seconds of the shutdown, then waits for the
   remaining requests and exits ([Stop the server](running.md#stop-the-server)).

Give the process a grace period of at least 30 seconds before it is killed (Kubernetes and ECS default to 30, Docker
to 10, which `compose.sfu.yaml` raises), and restart the nodes one at a time, so that the others can take their rooms.
A room that no other node has room for stays on the draining node until it ends or the 20 seconds pass. **With a
single node**, every room stays: its participants lose the audio when the node exits, and reconnect once the new node
has started.

### When a node is lost

When a node stops without draining (a crash, a lost machine), its clients' WebSockets close and its leases in the
key-value store expire within 15 seconds. The clients ask for a new grant, and the `web` process places the room again
on another node. A room loses nothing with its node: its participants, speakers and mutes are in the database.

### TURN for clients without UDP

A browser reaches a node over UDP, or over TCP when UDP is blocked. On networks that let only port 443 through (some
corporate and public networks) it needs a TURN server over TLS on 443: set `voice_turn` to `coturn`, with a `turns:`
URL on port 443 in `voice_turn_coturn_urls`, or to `cloudflare`, as for `p2p` ([coturn](#coturn),
[Cloudflare TURN](#cloudflare-turn)). `voice_ice_transport_policy = relay` is refused with `sfu`: the participants
already see only the node's address.

### Cloud Run

Cloud Run accepts only HTTP requests, so `role = sfu` cannot run there. Run the `web` and `worker` processes on Cloud
Run and the nodes on VMs with a public IP, sharing the database, the `stream` (`postgres` or `pubsub`) and the `kv`
(`db` or `valkey`) with them. Give each node its own host name and `voice_sfu_public_url`, serve TLS on the node
(`tls = acme`) or in front of it, and set `voice_sfu_origins` on the Cloud Run service to the nodes' origins. The
OpenTofu modules of [Deploying on Google Cloud](deploy-google-cloud.md) do not create the nodes.

### Metrics to watch

Every node exports its own `my.voice.sfu.*` metrics ([Metrics](running.md#metrics)).

| Metric | What it tells you |
| --- | --- |
| `my.voice.sfu.capacity.reserved` against `voice_sfu_node_capacity` | How full a node is for new rooms. When every node is near it, new rooms get `503`: add a node, or raise the capacity if the node has CPU to spare. |
| `my.voice.sfu.capacity.connected`, `my.voice.sfu.sessions`, `my.voice.sfu.rooms` | Who is connected. Far below the reservations means the rooms' maximums are larger than they need. |
| `my.voice.sfu.packets` by `my.outcome` | `forwarded` is the node's work. `silent` and `not_allowed` (a participant who may not speak or is muted) are dropped by design. `queue_full` means a participant's connection could not keep up: a slow client when it is a few, a node short of CPU or bandwidth when it rises for every client at once. |
| `my.voice.sfu.bytes` | The audio forwarded: the node's outbound bandwidth. |
| `my.voice.sfu.client.rtt`, `my.voice.sfu.client.packet_loss` | The quality on the way to the participants. High for most clients points at the node's network or location; for a few, at their own networks. |
| `my.voice.sfu.migrations`, `my.voice.sfu.ice_restarts`, `my.voice.sfu.renegotiations` | Rooms moved by a drain, clients that changed networks, and changes of speakers. |
| `my.voice.sfu.ticket_refusals` by `my.voice.sfu.reason` | An `other_node` that keeps rising means clients reach another node than the one of their grant: check that each node's `voice_sfu_public_url` leads to that node. |

## Deployment notes

- **Two streams per user**: during a call a participant holds the room's own event stream (Server-Sent Events) in addition to the stream the app always opens. Size connection limits for two long-lived requests per user in a call.
- **Several processes**: use a shared stream adapter (`stream` set to `postgres`, `valkey`, `sns` or `pubsub`), as for the rest of the app. With `stream = hub` and a `role` other than `all`, the server warns at start that voice chat will not work across processes.
- **Cloud Run**: a stream is a request, and CPU is allocated while it is open. When the request timeout cuts it, the client reconnects. Set the timeout as long as you can: `service.request_timeout_seconds` of [Deploying on Google Cloud](deploy-google-cloud.md#limits).
- **Media nodes need a VM**: a media node of `sfu` takes audio over UDP in a long-running process, which Cloud Run cannot host ([Cloud Run](#cloud-run)).

## Ending rooms and records

- A moderator who holds `manage_reports` can end an open room for everyone, even while voice chat is off ([moderation](moderation.md#voice-chat-rooms)).
- Who took part in a room (participations, speakers and listeners, raised hands, invitations and removals) is kept for `voice_participants_retention_days` days after the room ends (default 90, at least 30), or as long as a report points at the room. The room's name, description, hosts and how it ended are kept. The audio, the microphone state and reactions are never recorded.
- `voice_public_summary` controls whether visitors who cannot join a `local` room see its name and whether it is open, on its page and in its link preview ([instance settings](instance-settings.md)).

## Notes for users

- On an iPhone or iPad, sending audio may stop while the app is in the background, because iOS can suspend a web page that is not in front. Keep the app open on screen during a call.
- The browser asks for permission to use the microphone the first time. If you denied it, allow it again in the browser's site settings.
- In a peer-to-peer room, other participants may be able to find out your IP address unless the instance routes calls through a relay. Ask the operator if this matters to you.
