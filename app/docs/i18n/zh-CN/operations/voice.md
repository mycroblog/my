<!-- source: run-voice-chat 54808e78eb0d -->
# 运行语音聊天

语音聊天让已登录的用户开设房间、邀请他人并交谈。在你选择音频的传输方式之前，它处于关闭状态。本指南介绍如何做出这个选择、每种方式所需的 STUN 和 TURN 服务器或媒体服务器，以及部署、隐私和审核方面需要了解的事项。这里提到的每项设置都在[配置参考](../reference/configuration.md#voice)中有说明。

<!-- source: choose-how-the-audio-travels 9d60bd0655e9 -->
## 选择音频的传输方式

`voice` 为整个实例选择一种方式，所有房间都使用它。

| `voice` | 由什么传输音频 | 适用场景 |
| --- | --- | --- |
| `off` | 无。语音聊天 API 返回 `404`，Web 客户端不显示语音聊天，启动时也不检查其他 `voice_*` 设置。 | 默认值。 |
| `p2p` | 参与者的浏览器直接互相发送音频。服务器只中转建立连接所需的消息。 | 小型房间和成本最低的配置：除了可选的 STUN 或 TURN 服务器外，无需额外的服务器。 |
| `livekit` | LiveKit 服务器，自行托管或使用 LiveKit Cloud。 | 较大的房间，以及需要严格控制谁能发言的场合。 |
| `sora` | Sora 服务器，自行托管或使用 Sora Cloud。 | 同上，适合已经在运行 Sora 的情况。 |

![三种方式的对比。使用 p2p 时，服务器中转信令，浏览器之间直接或经由 TURN 服务器互相发送音频。使用 livekit 和 sora 时，浏览器把信令和音频发送到媒体服务器，服务器通过 API 和 webhook 与该媒体服务器通信。下面的列表逐一说明各种方式。](images/voice-methods.svg)

在所有方式中，服务器（`my serve`）都负责管理房间、参与者及其会话，并向每位参与者发放一份授权：浏览器连接所需的信息。它从不传输音频。不同之处在于信令（建立连接的消息）和音频去往哪里：

- **`p2p`**。浏览器把信令（offer、answer 和 ICE 候选）发送到服务器，服务器通过房间的事件流转交给另一个浏览器；信令最多在键值存储中等待 30 秒。音频直接在浏览器之间传输。授权中列出 `voice_stun_urls` 的 STUN 服务器，以及 `voice_turn`
  （`coturn` 或 `cloudflare`）的 TURN 服务器和短期凭据。浏览器通过 STUN 获知自己的公网地址，当两个浏览器找不到直接路径时，由 TURN 服务器中转音频。
- **`livekit`**。授权中包含 `voice_livekit_url` 和有效期 60 秒的访问令牌。浏览器把信令和音频发送到 LiveKit 服务器，LiveKit 自带 STUN 和 TURN，并把音频转发给其他参与者（选择性转发单元，即 SFU）。服务器通过 LiveKit 的 API 创建 LiveKit 房间并更改谁能发言，LiveKit 则通过 webhook 向服务器报告加入和离开。
- **`sora`**。授权中包含 `voice_sora_signaling_urls` 和签名的加入值。浏览器把信令和音频发送到 Sora 服务器，Sora 自带 STUN 和 TURN。Sora 在每次连接之前询问服务器的认证 webhook，服务器通过 Sora 的 API 控制房间，Sora 则通过 webhook 报告连接情况。

关于选择的说明：

- 使用 `p2p` 时，音频从不经过本服务器（使用 TURN 服务器时会经过 TURN 服务器）。服务器的负载仅限于房间消息、心跳和事件流。
- 小型实例最适合使用 `p2p`，或 LiveKit Cloud 的免费 Build 套餐（见 [LiveKit Cloud](#livekit-cloud)）。
- 使用 `livekit` 或 `sora` 时，下面的 STUN、TURN 设置和 ICE 策略都不会使用（如果设置了，服务器会在启动时发出警告），因为媒体服务器自带这些服务器。此时参与者的 IP 地址只有媒体服务器可见，其他参与者看不到。
- 构建标签 `no_voice_livekit` 和 `no_voice_sora` 会把这两种媒体服务器方式排除在二进制文件之外（[构建标签](../reference/build-tags.md)）。
- 请通过 HTTPS 提供实例：浏览器只允许安全页面访问麦克风。

以下限制可以控制付费媒体服务器的成本。除最后一项外，设为 `0` 即取消该限制：

| 设置 | 默认值 | 含义 |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | 房间保持开放的最长时间。达到该时间的房间会被结束。 |
| `voice_max_open_rooms` | `10` | 本服务器上同时开放的房间数上限。 |
| `voice_sfu_max_participants` | `10` | `livekit` 和 `sora` 的房间人数。 |
| `voice_p2p_max_participants` | `10` | `p2p` 的房间人数，范围为 2 到 50。 |

谁能开设房间由实例设置 `voice_room_starters` 决定（[实例设置](instance-settings.md)）：默认只有持有权限 `start_voice_rooms` 的人可以（内置的 `admin` 角色拥有该权限）；设为 `everyone` 则允许所有未被停用或限制的已登录本站用户开设房间。

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

<!-- source: size-and-bandwidth 9872bf829e26 -->
### 规模与带宽

只要两个浏览器中至少有一方在发言，它们就会互相连接；两个只听不说的人之间不会连接。因此，发言者要向其他每位参与者分别发送一份音频。用 Opus 编码的语音加上开销约为 30 到 50 kbit/s，所以在 10 人房间中，发言者约上传 9 份，共几百 kbit/s；在 50 人房间中则上传 49 份，约 2 Mbit/s，使用移动数据的手机或家庭网络可能难以承受。N 人房间中的连接数最多为 N(N-1)/2：10 人为 45 个，50 人为 1,225 个。这就是默认值为 10 的原因。只有在用户具备足够上传带宽时才提高 `voice_p2p_max_participants`；大型房间请使用 `livekit` 或 `sora`。

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

<!-- source: what-p2p-cannot-enforce 31e06affcd4f -->
### P2P 无法强制执行的事项

房间中只有部分参与者可以发言：主持人和主持人允许的听众。在 `p2p` 中，这一点在两处执行：服务器拒绝中转会让无权发言者发送音频的连接 offer，其他浏览器也不会播放非发言者的音频。

这能阻止普通用户和官方 Web 客户端，但并不是完整的防护。**两个使用修改版客户端的参与者相互配合，仍然可以交换音频**：无权发言者的修改版客户端可以发送音频，接收方的修改版客户端可以播放它，而服务器无法看到加密媒体的内容。如果你必须保证谁能发言（例如有审核的公开活动），请使用 `livekit` 或 `sora`，由媒体服务器本身丢弃无权限者的音频。

<!-- source: deployment-notes b0f03da306b7 -->
## 部署说明

- **每位用户两个流**：通话期间，参与者除了应用始终打开的流之外，还会保持房间自己的事件流（Server-Sent Events）。请按通话中每位用户两个长连接请求来设置连接数限制。
- **多个进程**：和应用的其他部分一样，使用共享的流适配器（`stream` 设为 `postgres`、`valkey`、`sns` 或 `pubsub`）。使用 `stream = hub` 且 `role` 不是 `all` 时，服务器会在启动时警告语音聊天无法跨进程工作。
- **Cloud Run**：一个流就是一个请求，打开期间会分配 CPU。请求超时切断它后，客户端会重新连接。请把超时时间设得尽可能长。
- **媒体服务器需要虚拟机**：LiveKit 和 Sora 在长时间运行的进程中通过 UDP 接收音频。Cloud Run 只接受 HTTP 请求，因此无法托管它们。请在虚拟机上运行它们（以及 coturn），或使用 LiveKit Cloud、Sora Cloud 或 Cloudflare TURN。

<!-- source: livekit b16c1d192419 -->
## LiveKit

设置 `voice = livekit`，以及：

| 设置 | 值 |
| --- | --- |
| `voice_livekit_url` | 浏览器连接的 `wss:` URL，例如 `wss://livekit.example.org`。 |
| `voice_livekit_api_url` | 本服务器调用的 `https:` URL，在与上面不同时设置。默认是同一主机加 `https:`。 |
| `voice_livekit_api_key`、`voice_livekit_api_secret` | LiveKit 服务器的一对密钥。secret 用于签名参与者的访问令牌和验证 webhook。 |

服务器在 LiveKit 上创建每个房间，以房间 ID 命名，并给每位参与者一个有效期 60 秒的访问令牌。听众的令牌不能发布；发言者的令牌只能发布麦克风。

<!-- source: self-hosted-livekit 2b118136f4dc -->
### 自行托管 LiveKit

在 `livekit.yaml` 中：

```yaml
keys:
  <api key>: <api secret>
room:
  # Rooms are created only by this server.
  auto_create: false
  # Audio only.
  enabled_codecs:
    - mime: audio/opus
    - mime: audio/red
webhook:
  api_key: <api key>
  urls:
    - https://<your domain>/voice/webhooks/livekit
```

- LiveKit 服务器必须能通过 HTTPS 访问 webhook URL。服务器会用 API secret 验证每个 webhook 的签名。
- webhook 只起加速作用。服务器每分钟都会把每个开放房间的参与者与 LiveKit 报告的情况进行比较，并移除不应在场的人，因此实例冷启动期间丢失的 webhook 也会得到弥补。
- 要在你的机器上试用，`livekit-server --dev` 会使用固定的开发密钥对启动。
- **被移出的参与者可以在短时间内重新加入**。自行托管的 LiveKit 无法吊销令牌：它会不断向已连接的客户端发送新令牌，每个最长有效约 10 分钟。因此，被移出的人最多可以在约 10 分钟内用最新的令牌重新加入，直到服务器察觉（通过加入 webhook 或每分钟的检查）并将其切断，在此期间其声音可能会被听到几秒到一分钟。如果必须立即移出，请使用 LiveKit Cloud：它会立即吊销令牌。

<!-- source: livekit-cloud 5bd9f833be11 -->
### LiveKit Cloud

使用项目的 WebSocket URL、API key 和 secret，并在项目设置中将 `https://<your domain>/voice/webhooks/livekit` 添加为 webhook 端点。免费的 Build 套餐每月提供 5,000 分钟和 50 GB，以及 100 个并发连接（2026-10-26 核实）；当前套餐请查看 LiveKit 的价格页面。

<!-- source: sora 5e9209dd3baa -->
## Sora

**Sora 适配器只针对按照 Sora 文档模拟的 Sora 进行过测试，没有针对真实的 Sora 服务器测试过**。在依赖它之前，请在预发布实例上用真实的 Sora 试用。

设置 `voice = sora`、`voice_sora_signaling_urls`（浏览器连接的 `wss:` URL，以逗号分隔）、`voice_sora_join_secret`（至少 32 字节，例如用 `openssl rand -base64 32` 生成；用于签名参与者加入时使用的值）和 `voice_sora_mode`（`self_hosted` 或 `cloud`）。实例必须通过 HTTPS 提供，因为 Sora 会回调 `https://<your domain>`。

<!-- source: webhooks 11b2c486a476 -->
### Webhook

Sora 会调用本服务器的两个端点：

| 端点 | 用途 |
| --- | --- |
| `/voice/webhooks/sora/auth` | 认证 webhook。Sora 在每次连接前调用它。服务器只允许在房间中、未被移出、以发言者身份连接时有发言权限、且未超出人数限制的参与者连接，并告诉 Sora 只接受音频。 |
| `/voice/webhooks/sora/events` | 事件和会话 webhook（连接的创建与销毁、会话结束）。 |

**认证 webhook 必须及时响应**。自行托管时 Sora 等待 5 秒，Sora Cloud 上等待 10 秒。服务器根据自己的数据响应，不调用其他任何东西，但实例冷启动（Cloud Run 从零开始扩容）可能会超出这个时间，此时连接会被拒绝。使用语音聊天期间，请保持一个实例处于运行状态。

自行托管时，请在 `sora.conf` 中设置 `default_duplicate_client_id = evict`。成为发言者的听众会以相同身份重新连接（Sora 无法更改连接的角色），`evict` 会让 Sora 在新连接到达时断开旧连接。使用默认的 `allow` 时，服务器在得知新连接后会自己切断旧连接，因此两者会短暂重叠。

<!-- source: self-hosted-sora dd3244368b73 -->
### 自行托管 Sora

| 设置 | 值 |
| --- | --- |
| `voice_sora_mode` | `self_hosted` |
| `voice_sora_api_url` | Sora HTTP API 前面的反向代理的 `https:` URL。 |
| `voice_sora_api_header_name`、`voice_sora_api_header_value` | 代理用来认证本服务器的请求头，例如 `Authorization` 和 `Basic ...`。 |
| `voice_sora_webhook_username`、`voice_sora_webhook_password` | webhook 的 Basic 凭据。必填。 |

**Sora 自身的 HTTP API 没有认证**，本应只在回环地址上监听。切勿将其暴露。请在它前面放一个要求凭据的反向代理，并通过这两项请求头设置把该凭据交给本服务器。在 Sora 中用相同的 Basic 凭据配置 webhook URL。服务器只接受 `https:` 的 API URL（只有 localhost 可以用 `http:`），没有 webhook 凭据时不会启动。不支持 mTLS，因为 Cloud Run 和 ECS 前面的负载均衡器会在服务器之前终止 TLS。

<!-- source: sora-cloud ac4935c7f7a7 -->
### Sora Cloud

设置 `voice_sora_mode = cloud`、`voice_sora_cloud_project_id` 和 `voice_sora_cloud_api_key`。服务器用该密钥签名访问令牌、调用 API 并验证 webhook 签名；不需要 Basic 凭据。

<!-- source: behavior-to-expect 03fd5e1655fa -->
### 预期行为

- 被允许发言的听众会自动以发言者身份重新连接，因此在新连接建立期间会有短暂的中断。
- 被取消发言权的发言者会立即被转发过滤器静音，然后以听众身份重新连接。

<!-- source: licensing 534e304817e3 -->
### 许可

Sora 是 Shiguredo的商业产品。截至 2026-10-26 的核实：Sora Cloud 的基本费用为每月 5,000 日元，没有免费期；自行托管需要购买套餐许可，按并发连接数和期限预先付费。产品页面称不向日本以外的公司、组织或个人提供，因此在围绕它做规划之前，请先确认你能否购买。自行托管的 LiveKit 是开源的。

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
