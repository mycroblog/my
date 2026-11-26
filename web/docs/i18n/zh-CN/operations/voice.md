<!-- source: run-voice-chat f44427fbbe76 -->
# 运行语音聊天

语音聊天让已登录的用户开设房间、邀请他人并交谈。在你选择音频的传输方式之前，它处于关闭状态。本指南介绍如何做出这个选择、每种方式所需的 STUN 和 TURN 服务器或媒体节点，以及部署、隐私和审核方面需要了解的事项。这里提到的每项设置都在[配置参考](../reference/configuration.md#voice)中有说明。

<!-- source: choose-how-the-audio-travels 2ef72c636c49 -->
## 选择音频的传输方式

`voice` 为整个实例选择一种方式；每个房间都使用它。

| `voice` | 由什么传输音频 | 适合 |
| --- | --- | --- |
| `off` | 无。语音聊天 API 返回 `404`，Web 客户端不显示语音聊天，启动时也不会检查其他 `voice_*` 设置。 | 默认值。 |
| `p2p` | 参与者的浏览器把音频直接发给彼此。服务器只中继用于建立连接的消息。 | 小房间和成本最低的部署：除了可选的 STUN 或 TURN 服务器之外，不需要额外的服务器。 |
| `sfu` | 服务器自己的媒体节点：每个浏览器把音频发送给持有其房间的节点，由该节点把发言者的音频转发给参与者。 | 较大的房间，以及对谁能发言的严格控制。 |

![两种方式并排比较。使用 p2p 时，服务器中继信令，浏览器之间直接发送音频，或者经由 TURN 服务器。使用 sfu 时，浏览器从服务器获得授权，并把信令和音频发送给其房间所在的媒体节点。下面的列表介绍了每种方式。](images/voice-methods.svg)

无论哪种方式，服务器（`my serve`）都会保存房间、谁参与其中以及它们的会话，并给每位参与者一个授权：浏览器连接所需的内容。不同之处在于信令（用于建立连接的消息）和音频走向何处：

- **`p2p`。** 浏览器把它的信令（offer、answer 和 ICE 候选）发送给服务器，服务器通过房间的事件流把它们传给另一个浏览器；信令在键值存储中最多等待 30 秒。音频直接从浏览器传到浏览器。授权中列出了 `voice_stun_urls` 的 STUN 服务器，以及 `voice_turn`（`coturn` 或 `cloudflare`）的 TURN 服务器和短期凭据。浏览器通过 STUN 得知自己的公网地址，当两个浏览器找不到直连路径时，TURN 服务器会中继音频。
- **`sfu`。** 服务器把每个房间安排在一个媒体节点（选择性转发单元，即 SFU）上：该节点是一个 `role = all` 或 `sfu` 的进程。授权中包含该节点的 `wss:` URL（`voice_sfu_public_url`）和一张一次性票据。浏览器通过 WebSocket 把信令发送给节点，并把音频发送到节点的 UDP 端口 `voice_sfu_udp_port`（UDP 被阻断时使用 TCP 端口 `voice_sfu_tcp_port`），节点再把发言者的音频转发给其他参与者。对于无法直接连到节点的浏览器，由 `voice_turn` 的 TURN 服务器提供中继。

关于选择的说明：

- **`p2p`** 不需要服务器开放 UDP 端口：音频从不经过服务器（如果使用了 TURN 服务器，则经过它），服务器的负载只有房间消息、心跳和流。它适合小型实例、小房间，以及只部署在 Cloud Run 上的情况。
- **`sfu`** 由服务器强制规定谁可以发言：节点只转发发言者的音频。浏览器只会连到媒体节点，因此参与者的 IP 地址只有节点能看到，其他参与者看不到。每个节点都需要一台带公网 IP、并开放 UDP 和 TCP 端口的机器（[媒体节点](#media-nodes)）。节点有公网地址，因此不需要 `voice_stun_urls`，并且 `voice_ice_transport_policy = relay` 会被拒绝。
- 请通过 HTTPS 提供实例服务：浏览器只允许在安全页面上访问麦克风。

这些限制会让房间的成本保持有界。除最后一项外，都可以设为 `0` 来取消限制：

| 设置 | 默认值 | 含义 |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | 房间保持开启的最长时间。达到该时间的房间会被结束。 |
| `voice_max_open_rooms` | `10` | 本服务器上同时最多可以开启多少个房间。为预定时间开启的房间，从它开启起计算，即使它在等待第一位参与者。 |
| `voice_max_participants` | `10` | 最大的房间，从 2 到 500（使用 `p2p` 时为 50）。主持人可以为自己的房间设置更小的上限。 |

谁可以开启房间由实例设置 `voice_room_starters` 决定（[实例设置](instance-settings.md)）：默认只有拥有权限 `start_voice_rooms` 的人（内置的 `admin` 角色拥有它）；`everyone` 则允许每位未被封禁或限制的已登录本站用户开启房间。

<!-- source: peer-to-peer-rooms 2199ca9a427c -->
## 点对点房间

<!-- source: try-it-with-docker-compose ac3681681768 -->
### 用 Docker Compose 试用

要在你的机器上用 Docker Compose 配置（[`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml)）试用语音聊天，请在 `.env` 中设置 `MY_VOICE=p2p` 并重新创建服务器。

```sh
cd deploy/docker
docker compose up -d
```

- 在 `http://localhost:8080`（`example.env` 的 `MY_WEB_ORIGIN`）打开实例。浏览器只允许 `localhost` 或 HTTPS 上的页面使用麦克风，因此 `http://192.168.1.10:8080` 这样的地址无法发言。
- 同一台机器上的浏览器（两个浏览器配置文件，或一个普通窗口和一个隐私窗口）无需 STUN 或 TURN
  服务器即可互相连接。要连接其他机器上的浏览器，请把实例放在 HTTPS 后面，并在 `.env` 中设置 `MY_VOICE_STUN_URLS`。
- 默认只有 `admin` 角色可以开设房间。请以你创建的管理员身份登录，或在服务器设置的“语音聊天”页面将 `voice_room_starters` 设为 `everyone`，让所有用户都能开设房间。

<!-- source: size-and-bandwidth e54157c29322 -->
### 规模与带宽

只要两个浏览器中至少有一方在发言，它们就会互相连接；两个只听不说的人之间不会连接。因此，发言者要向其他每位参与者分别发送一份音频。用 Opus 编码的语音加上开销约为 30 到 50 kbit/s，所以在 10 人房间中，发言者约上传 9 份，共几百 kbit/s；在 50 人房间中则上传 49 份，约 2 Mbit/s，使用移动数据的手机或家庭网络可能难以承受。N 人房间中的连接数最多为 N(N-1)/2：10 人为 45 个，50 人为 1,225 个。这就是默认值为 10 的原因。只有在用户具备足够上传带宽时才提高 `voice_max_participants`；大型房间请使用 `sfu`。

<!-- source: stun-and-turn 51e427976a77 -->
### STUN 与 TURN

位于 NAT 后面的浏览器需要 STUN 服务器来获知自己的公网地址，在没有直接路径时（严格的 NAT、企业防火墙、部分移动网络）还需要 TURN 服务器来中转音频。

- `voice_stun_urls`：以逗号分隔的 `stun:` 或 `stuns:` URL。任何公共 STUN 服务器都可以，下文的 coturn 服务器也能响应 STUN。
- `voice_turn`：`none`、`coturn` 或 `cloudflare`。

两者都未设置时，服务器会在启动时发出警告：只有同一网络中的浏览器能够连接。公开实例需要 STUN，还需要为无法直接连接的用户提供 TURN。

服务器向每位参与者发放短期 TURN 凭据，从不使用共享密码。`voice_turn_credential_ttl`（默认 `6h`）设置凭据的有效期。Web 客户端会在凭据到期前十分钟申请新凭据并重启 ICE，因此通话不会因凭据到期而中断。

<!-- source: coturn d665e942c91b -->
#### coturn

设置 `voice_turn = coturn`、`voice_turn_coturn_urls` 和 `voice_turn_coturn_secret`。服务器自己计算凭据（用户名是到期时间和会话 ID，密码是用密钥对用户名计算的 HMAC-SHA1），因此不会向 TURN 服务器发出请求。在 `turnserver.conf` 中对 coturn 做相应配置：

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

- `denied-peer-ip` 很重要。没有它，任何持有凭据的人都能利用 TURN 服务器访问其内部网络上的主机（云元数据服务、数据库、其他虚拟机）。请拒绝所有私有、回环和链路本地地址段；只有确实需要中转到某个内部地址时，才允许它（`allowed-peer-ip`）。
- 开放 UDP 和 TCP 3478、用于 `turns:` 的 TCP 5349，以及 coturn 的中继端口范围（`min-port` / `max-port`，默认为 49152-65535）。
- 在 `voice_turn_coturn_urls` 中同时列出一个 `turns:` URL 或带 `?transport=tcp` 的 URL，这样处于屏蔽 UDP 的网络中的用户也能连接。
- coturn 只在首次创建分配时检查凭据是否到期。已存在的分配在凭据到期后仍可继续使用，新凭据在下一次分配时生效，而 Web 客户端会通过重启 ICE 来创建新的分配。

<!-- source: cloudflare-turn be5846573be8 -->
#### Cloudflare TURN

设置 `voice_turn = cloudflare`、`voice_turn_cloudflare_key_id` 和 `voice_turn_cloudflare_api_token`，取值来自在 Cloudflare 控制台中创建的 TURN 密钥。服务器会在参与者开设或加入房间时向 Cloudflare 申请其凭据（如果失败，请求返回 `503`，不会开设或加入任何房间，因此可以重试），并在参与者离开或被移出时吊销凭据。

- 凭据最长有效 48 小时，因此 `voice_turn_credential_ttl` 不能超过 `48h`（否则服务器拒绝启动）。
- **凭据到期后不久，Cloudflare 就会断开中继**。Web 客户端会在此之前续期，因此请让 `voice_turn_credential_ttl` 远大于十分钟。
- 价格（2026-10-26 核实）：每月前 1,000 GB 免费，之后发送给客户端的流量为每 GB 0.05 美元。依赖它之前，请查看 Cloudflare 当前的价格。

<!-- source: hiding-ip-addresses de67ffa9e7b7 -->
### 隐藏 IP 地址

使用 `voice_ice_transport_policy = all`（默认值）时，浏览器能直接连接就会直接连接，因此房间的参与者可能会得知彼此的 IP 地址。

使用 `relay` 时，浏览器只使用 TURN 服务器，因此参与者看到的是 TURN 服务器的地址，而不是其他参与者的地址。音频在浏览器之间仍保持加密。`relay` 隐藏不了的是：运营 TURN 服务器的一方（你或 Cloudflare）能看到每位参与者的地址，本服务器也能看到调用其 API 的每个人的地址。`relay` 要求 `voice_turn` 为 `coturn` 或 `cloudflare`（否则服务器拒绝启动），并且会让每次通话都经过 TURN 服务器，使用 coturn 时消耗的是你的带宽，使用 Cloudflare 时则计入你的流量账单。

<!-- source: what-p2p-cannot-enforce 4f97611bece5 -->
### P2P 无法强制执行的事项

房间中只有部分参与者可以发言：主持人和主持人允许的听众。在 `p2p` 中，这一点在两处执行：服务器拒绝中转会让无权发言者发送音频的连接 offer，其他浏览器也不会播放非发言者的音频。

这能阻止普通用户和官方 Web 客户端，但并不是完整的防护。**两个使用修改版客户端的参与者相互配合，仍然可以交换音频**：无权发言者的修改版客户端可以发送音频，接收方的修改版客户端可以播放它，而服务器无法看到加密媒体的内容。如果你必须保证谁能发言（例如有审核的公开活动），请使用 `sfu`，由媒体节点本身只转发发言者的音频。

<!-- source: media-nodes 17f4fc8eb80b -->
## 媒体节点

使用 `voice = sfu` 时，每个房间由一个媒体节点持有，该节点的进程负责转发房间的音频。一个房间不会跨越多个节点。

<!-- source: run-media-nodes fac01c5bdbf9 -->
### 运行媒体节点

- **`role = all`** 在与其他所有功能相同的进程中运行一个节点，地址为 `wss://<host>/voice/sfu`。一台开放了 UDP 和 TCP 端口的机器就足够了。
- **`role = sfu`** 单独运行一个节点，与 `web` 和 `worker` 进程并列。它与它们共用数据库、`stream`（不能是 `hub`）和 `kv`（不能是 `memory`）：节点把自己和它的房间注册到键值存储中，并通过流得知其房间的变化。它不运行任何作业，只提供健康检查和节点的 WebSocket。可以按需运行任意多个：`web` 进程会把每个新房间安排到预留名额最少、且还容得下该房间的节点上。`web` 进程不运行节点。

每个节点需要：

| 内容 | 设置 | 说明 |
| --- | --- | --- |
| 一个公网 IP 地址 | NAT 之后使用 `voice_sfu_public_ip` | 节点把这个地址告诉浏览器；未设置时使用它自己的地址。云虚拟机或容器通常有自己的私有地址：请设置它。 |
| 向互联网开放的 UDP 端口 | `voice_sfu_udp_port`（7882） | 节点的所有会话共用它。 |
| 向互联网开放的 TCP 端口 | `voice_sfu_tcp_port`（7881） | 供阻断 UDP 的网络中的浏览器使用。 |
| 带 TLS 的 `wss:` URL | `voice_sfu_public_url`（`wss://<host>/voice/sfu`） | 每个节点都有自己的 URL，例如 `wss://sfu1.example.org/voice/sfu`。前面的反向代理终止 TLS 并转发 WebSocket，或者由节点自己提供 HTTPS：`tls` 设置在 `role = sfu` 上同样有效，作用于 `voice_sfu_public_url` 的主机（[由服务器提供 HTTPS](running.md#serving-https-from-the-server)）。 |
| 它能容纳多少参与者 | `voice_sfu_node_capacity`（500） | 见[容量](#capacity)。 |

请把 UDP 和 TCP 端口原样转发给节点：节点告诉浏览器的是它自己的端口号。如果节点所在的主机不同于 `host`，还需要在 `web` 进程上设置 `voice_sfu_origins`，例如 `wss://*.sfu.example.org`，这样 Web 客户端的内容安全策略才会允许该连接。

<!-- source: docker-compose 35fa31462add -->
### Docker Compose

[`compose.sfu.yaml`](../../deploy/docker/compose.sfu.yaml) 这一层在一台机器上运行 `role = web` 的服务器、一个 `worker` 和一个媒体节点 `sfu`，数据库为 Caddy 之后的 PostgreSQL（[Docker Compose 文件](running.md#docker-compose-files)）：

```sh
# In .env:
# COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml:compose.sfu.yaml
# MY_VOICE_SFU_PUBLIC_IP=203.0.113.10   the public address of this machine
docker compose up -d
```

- Caddy 把 `/voice/sfu` 转给节点，把其他所有请求转给服务器，因此节点的 URL 就是默认的 `wss://<MY_HOST>/voice/sfu`，不需要 `voice_sfu_origins`。
- 除 80 和 443 外，还要在机器的防火墙中开放 UDP 7882 和 TCP 7881。
- 无论 `.env` 怎么写，这一层都会为这三个进程设置 `voice = sfu` 和 `stream = postgres`。

<!-- source: capacity 2137d8a7bcce -->
### 容量

房间被安排到节点上时，会在该节点上预留它的上限（主持人为该房间设置的上限，或 `voice_max_participants`），不论实际加入了多少人。只有当节点上所有预留之和不超过 `voice_sfu_node_capacity` 时，节点才会接收房间，因此节点持有的参与者永远不会超过它的容量。当没有任何节点容得下某个房间的上限时，开设该房间或第一次加入都会返回 `503`，用户可以稍后重试。

请根据机器能转发的量来设置 `voice_sfu_node_capacity`（[实测成本](#measured-cost)），并让 `voice_max_participants` 不要超过你的房间所需：即使房间里只有 3 个人，500 人的房间也会预留 500 个名额。

<!-- source: measured-cost 2d6571f6e923 -->
### 实测成本

测试使用以每秒 50 个数据包发送 Opus 音频的模拟客户端，在 AMD Ryzen 9 7900（12 核）、WSL2 下的 Linux 和 Go 1.27 上进行，全部通过回环接口。CPU 以该机器的核数计；内存是节点的堆和 goroutine 栈，加在进程其余部分之上。

| 内容 | CPU | 内存 |
| --- | --- | --- |
| 空闲的节点 | 无法测出 | 约 0.1 MiB |
| 每个开启的房间 | 无法测出 | 小于约 25 KiB |
| 每位参与者 | 静音时约 0.0002 核 | 约 180 KiB |
| 每路转发的流（一位发言者被一位参与者听到） | 每个数据包 18 到 27 µs，约 0.001 核 | 小于约 25 KiB |
| 100 人的房间，3 位发言者正在发声 | 约 0.3 核 | 约 23 MiB |

更快或更慢的机器会按比例改变这些数字；节点自己的指标（[需关注的指标](#metrics-to-watch)）会显示它承载的负载。

转发的成本取决于节点发出的数据包：每位发言者的音频都会发给其他每位参与者，所以有 K 位发言者的 N 人房间大约发送 K × (N − 1) 路 30 到 50 kbit/s 的流。听众多、发言者少的房间成本很低；所有人同时发言的房间成本最高。请用同样的方式规划节点的出站带宽。

<!-- source: drain-a-node-for-a-deploy 249929addcae -->
### 为部署排空节点

收到 SIGTERM 时，节点会先排空再停止：

1. 它不再接收新房间。
2. 它把自己的每个房间安排到另一个节点上，并通知房间的客户端迁移。客户端用新的授权重新连接到新节点，音频会中断约 1 到 2 秒。
3. 它等待客户端离开，最多等待关闭过程 25 秒中的 20 秒，然后等待剩余的请求并退出（[停止服务器](running.md#stop-the-server)）。

请让进程在被杀死之前至少有 30 秒的宽限期（Kubernetes 和 ECS 默认为 30 秒，Docker 为 10 秒，`compose.sfu.yaml` 把它调高了），并且每次只重启一个节点，让其他节点能接收它的房间。没有其他节点容得下的房间会留在正在排空的节点上，直到房间结束或 20 秒过去。**只有一个节点时**，所有房间都会留下：节点退出时参与者会失去音频，并在新节点启动后重新连接。

<!-- source: when-a-node-is-lost c577f2240d7e -->
### 节点丢失时

节点不经排空就停止（崩溃、机器丢失）时，它的客户端的 WebSocket 会关闭，它在键值存储中的租约会在 15 秒内过期。客户端会请求新的授权，`web` 进程会把房间重新安排到另一个节点上。房间不会因节点丢失而丢失任何内容：它的参与者、发言者和静音状态都在数据库中。

<!-- source: turn-for-clients-without-udp 1c1230dc875c -->
### 为没有 UDP 的客户端提供 TURN

浏览器通过 UDP 连接节点，UDP 被阻断时则通过 TCP。在只放行 443 端口的网络（部分企业网络和公共网络）中，它需要 443 端口上基于 TLS 的 TURN 服务器：把 `voice_turn` 设为 `coturn`，并在 `voice_turn_coturn_urls` 中给出 443 端口的 `turns:` URL，或者像 `p2p` 一样设为 `cloudflare`（[coturn](#coturn)、[Cloudflare TURN](#cloudflare-turn)）。`sfu` 下 `voice_ice_transport_policy = relay` 会被拒绝：参与者本来就只能看到节点的地址。

<!-- source: cloud-run 5cf3ffcbb7a6 -->
### Cloud Run

Cloud Run 只接受 HTTP 请求，因此 `role = sfu` 无法在那里运行。请把 `web` 和 `worker` 进程运行在 Cloud Run 上，把节点运行在带公网 IP 的虚拟机上，并与它们共用数据库、`stream`（`postgres` 或 `pubsub`）和 `kv`（`db` 或 `valkey`）。请为每个节点设置各自的主机名和 `voice_sfu_public_url`，在节点上（`tls = acme`）或其前面提供 TLS，并把 Cloud Run 服务上的 `voice_sfu_origins` 设为这些节点的源。[在 Google Cloud 上部署](deploy-google-cloud.md)的 OpenTofu 模块不会创建这些节点。

<!-- source: metrics-to-watch a8b32402b50d -->
### 需关注的指标

每个节点都会导出自己的 `my.voice.sfu.*` 指标（[指标](running.md#metrics)）。

| 指标 | 它告诉你什么 |
| --- | --- |
| `my.voice.sfu.capacity.reserved` 与 `voice_sfu_node_capacity` 对比 | 节点对新房间来说有多满。当所有节点都接近容量时，新房间会得到 `503`：请增加节点，或者在节点还有空闲 CPU 时提高容量。 |
| `my.voice.sfu.capacity.connected`、`my.voice.sfu.sessions`、`my.voice.sfu.rooms` | 谁已连接。远低于预留量说明房间的上限比实际需要的大。 |
| 按 `my.outcome` 区分的 `my.voice.sfu.packets` | `forwarded` 是节点的工作量。`silent` 和 `not_allowed`（无权发言或被静音的参与者）按设计被丢弃。`queue_full` 表示某位参与者的连接跟不上：只有少数几个时是客户端太慢，所有客户端同时上升时则是节点的 CPU 或带宽不足。 |
| `my.voice.sfu.bytes` | 转发的音频量：节点的出站带宽。 |
| `my.voice.sfu.client.rtt`、`my.voice.sfu.client.packet_loss` | 到参与者途中的质量。大多数客户端都偏高，说明问题在节点的网络或位置；只有少数偏高，则在他们自己的网络。 |
| `my.voice.sfu.offer.wait` | 加入的参与者等待节点第一个 offer 的时间：CPU 有余量的节点上是毫秒级；达到秒级说明加入请求在繁忙的节点上排队。 |
| `my.voice.sfu.migrations`、`my.voice.sfu.ice_restarts`、`my.voice.sfu.renegotiations` | 因排空而迁移的房间、更换了网络的客户端，以及发言者的变化。 |
| 按 `my.voice.sfu.reason` 区分的 `my.voice.sfu.ticket_refusals` | `other_node` 持续上升，说明客户端连到的节点不是其授权中的那个：请检查每个节点的 `voice_sfu_public_url` 是否指向该节点。 |

<!-- source: deployment-notes 63042f72a7c5 -->
## 部署说明

- **每位用户两个流**：通话期间，参与者除了应用始终打开的流之外，还会保持房间自己的事件流（Server-Sent Events）。请按通话中每位用户两个长连接请求来设置连接数限制。
- **多个进程**：和应用的其他部分一样，使用共享的流适配器（`stream` 设为 `postgres`、`valkey`、`sns` 或 `pubsub`）。使用 `stream = hub` 且 `role` 不是 `all` 时，服务器会在启动时警告语音聊天无法跨进程工作。
- **Cloud Run**：一个流就是一个请求，打开期间会分配 CPU。请求超时切断它后，客户端会重新连接。请把超时时间设得尽可能长：见[在 Google Cloud 上部署](deploy-google-cloud.md#limits)中的 `service.request_timeout_seconds`。
- **媒体节点需要虚拟机**：`sfu` 的媒体节点在长时间运行的进程中通过 UDP 接收音频，Cloud Run 无法托管它（[Cloud Run](#cloud-run)）。

<!-- source: ending-rooms-and-records b4d3d196ecd7 -->
## 结束房间与记录

- 持有 `manage_reports` 的审核员可以为所有人结束一个开放的房间，即使语音聊天已关闭也可以（[审核](moderation.md#voice-chat-rooms)）。
- 房间的参与情况（参与记录、发言者和听众、举手、邀请和移出）会在房间结束后保留 `voice_participants_retention_days` 天（默认 90，至少 30），只要有举报指向该房间，就会一直保留。房间的名称、说明、主持人以及结束方式会被保留。音频、麦克风状态和回应从不记录。
- `voice_public_summary` 控制无法加入 `local` 房间的访客能否在其页面和链接预览中看到房间名称以及房间是否开放（[实例设置](instance-settings.md)）。

<!-- source: notes-for-users f44150f15915 -->
## 给用户的说明

- 在 iPhone 或 iPad 上，应用处于后台时可能会停止发送音频，因为 iOS 可能会挂起不在前台的网页。通话期间请让应用保持在屏幕上打开。
- 浏览器第一次会请求使用麦克风的权限。如果你拒绝了，请在浏览器的网站设置中重新允许。
- 在点对点房间中，除非实例通过中继转发通话，否则其他参与者可能会得知你的 IP 地址。如果你在意这一点，请询问运营者。
