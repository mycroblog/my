<!-- source: run-voice-chat f44427fbbe76 -->
# 執行語音聊天

語音聊天讓已登入的使用者開設聊天室、邀請其他人並交談。在你選擇音訊的傳輸方式之前，它都是關閉的。本指南說明這個選擇、每種方式所需的 STUN 和 TURN 伺服器或媒體節點，以及部署、隱私和審核方面需要知道的事。這裡提到的每個設定都說明於[設定參考](../reference/configuration.md#voice)中。

<!-- source: choose-how-the-audio-travels 2ef72c636c49 -->
## 選擇音訊的傳輸方式

`voice` 為整個實例選擇一種方式；每個聊天室都使用它。

| `voice` | 由什麼傳送音訊 | 適合 |
| --- | --- | --- |
| `off` | 什麼都沒有。語音聊天 API 回應 `404`，網頁用戶端不顯示語音聊天，其他 `voice_*` 設定在啟動時也不會被檢查。 | 預設值。 |
| `p2p` | 參與者的瀏覽器直接互相傳送音訊。伺服器只中繼建立連線所需的訊息。 | 小型聊天室與最便宜的設置：除了選用的 STUN 或 TURN 伺服器，不需要額外的伺服器。 |
| `sfu` | 伺服器自己的媒體節點：每個瀏覽器把音訊傳給持有其聊天室的節點，由該節點把發言者的音訊轉送給參與者。 | 較大的聊天室，以及對誰能發言的嚴格控制。 |

![兩種方式並排比較。使用 p2p 時，伺服器中繼訊號，瀏覽器彼此傳送音訊，或透過 TURN 伺服器傳送。使用 sfu 時，瀏覽器從伺服器取得授權，並把訊號與音訊傳送給其聊天室所在的媒體節點。下方的清單說明每一種方式。](images/voice-methods.svg)

不論哪種方式，伺服器（`my serve`）都負責保管聊天室、誰參與其中及其工作階段，並交給每位參與者一份授權：瀏覽器連線所需的內容。不同的是訊號（建立連線的訊息）與音訊走哪裡：

- **`p2p`。** 瀏覽器將它的訊號（offer、answer 和 ICE candidate）傳送給伺服器，伺服器透過聊天室的事件串流轉交給另一個瀏覽器；訊號最多會在鍵值儲存中等待 30 秒。音訊直接從瀏覽器傳到瀏覽器。授權中會列出 `voice_stun_urls` 的 STUN 伺服器，以及 `voice_turn`（`coturn` 或 `cloudflare`）的 TURN 伺服器與短期有效的憑證。瀏覽器從 STUN 得知自己的公開位址，而當兩個瀏覽器找不到直接的路徑時，TURN 伺服器會中繼音訊。
- **`sfu`。** 伺服器把每個聊天室安排在一個媒體節點（選擇性轉送單元，即 SFU）上：該節點是一個 `role = all` 或 `sfu` 的程序。授權中包含該節點的 `wss:` URL（`voice_sfu_public_url`）和一張單次使用的票證。瀏覽器透過 WebSocket 把訊號傳給節點，並把音訊傳到節點的 UDP 連接埠 `voice_sfu_udp_port`（UDP 被封鎖時使用 TCP 連接埠 `voice_sfu_tcp_port`），節點再把發言者的音訊轉送給其他參與者。對於無法直接連到節點的瀏覽器，由 `voice_turn` 的 TURN 伺服器提供中繼。

關於選擇的說明：

- **`p2p`** 不需要伺服器開放 UDP 連接埠：音訊從不經過伺服器（有使用 TURN 伺服器時，則會經過它），伺服器的負載只有聊天室訊息、心跳和串流。它適合小型實例、小型聊天室，以及只部署在 Cloud Run 上的情況。
- **`sfu`** 由伺服器強制規定誰可以發言：節點只轉送發言者的音訊。瀏覽器只會連到媒體節點，因此參與者的 IP 位址只有節點看得到，其他參與者看不到。每個節點都需要一台有公開 IP、並開放 UDP 和 TCP 連接埠的機器（[媒體節點](#media-nodes)）。節點有公開位址，因此不需要 `voice_stun_urls`，而且 `voice_ice_transport_policy = relay` 會被拒絕。
- 請以 HTTPS 提供實例：瀏覽器只允許在安全的頁面上使用麥克風。

這些上限讓聊天室的成本維持在可控範圍內。除了最後一項之外，`0` 都會移除該項上限：

| 設定 | 預設值 | 意義 |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | 聊天室最長開放多久。達到時間的聊天室會被結束。 |
| `voice_max_open_rooms` | `10` | 這個伺服器上同時最多可以開放多少個聊天室。為排定時間而開啟的聊天室，從它開啟起就開始計算，包括它等待第一位參與者的時間。 |
| `voice_max_participants` | `10` | 最大的聊天室，範圍為 2 到 500（使用 `p2p` 時為 50）。主持人可以為自己的聊天室設定更小的上限。 |

誰可以開設聊天室，由實例設定 `voice_room_starters` 決定（[實例設定](instance-settings.md)）：預設只有擁有 `start_voice_rooms` 權限的人（內建的 `admin` 角色有這項權限）；`everyone` 則讓每位未被停權或限制的已登入本地使用者都能開設聊天室。

<!-- source: peer-to-peer-rooms 2199ca9a427c -->
## 點對點聊天室

<!-- source: try-it-with-docker-compose ac3681681768 -->
### 用 Docker Compose 試用

若要使用 Docker Compose 配置（[`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml)）在你的電腦上試用語音聊天，請在 `.env` 中設定 `MY_VOICE=p2p` 並重新建立伺服器。

```sh
cd deploy/docker
docker compose up -d
```

- 在 `http://localhost:8080`（`example.env` 的 `MY_WEB_ORIGIN`）開啟實例。瀏覽器只允許 `localhost` 或 HTTPS 上的頁面使用麥克風，因此像 `http://192.168.1.10:8080` 這樣的網址無法發言。
- 同一台電腦上的瀏覽器（兩個瀏覽器設定檔，或一般視窗與私密視窗）不需要 STUN 或 TURN 伺服器就能互相連線。若要連到其他電腦上的瀏覽器，請把實例放在 HTTPS 後方，並在 `.env` 中設定 `MY_VOICE_STUN_URLS`。
- 預設只有 `admin` 角色可以開設聊天室。請以你建立的管理員身分登入，或在伺服器設定的「語音聊天」頁面中把 `voice_room_starters` 設為 `everyone`，讓所有使用者都能開設聊天室。

<!-- source: size-and-bandwidth e54157c29322 -->
### 人數與頻寬

只要兩個瀏覽器中至少有一個在發言，它們就會互相連線；兩個只在收聽的人不會連線。因此發言者會把自己的音訊分別傳一份給其他每位參與者。Opus 編碼的語音加上額外負擔大約需要 30 到 50 kbit/s，所以在 10 人的聊天室中，發言者要上傳約 9 份，也就是幾百 kbit/s；在 50 人的聊天室中則要上傳 49 份，約 2 Mbit/s，使用行動數據的手機或家用網路可能無法負荷。N 人聊天室中的連線數最多為 N(N-1)/2：10 人為 45 條，50 人為 1,225 條。這就是預設值為 10 的原因。只有在使用者有足夠的上傳頻寬時才提高 `voice_max_participants`；大型聊天室請使用 `sfu`。

<!-- source: stun-and-turn 51e427976a77 -->
### STUN 與 TURN

位於 NAT 後方的瀏覽器需要 STUN 伺服器來得知自己的公開位址，並在沒有直接路徑時（嚴格的 NAT、企業防火牆、部分行動網路）需要 TURN 伺服器來轉送音訊。

- `voice_stun_urls`：以逗號分隔的 `stun:` 或 `stuns:` URL。任何公開的 STUN 伺服器都可以使用，下方的 coturn 伺服器也會回應 STUN。
- `voice_turn`：`none`、`coturn` 或 `cloudflare`。

兩者都沒有設定時，伺服器會在啟動時發出警告：只有同一個網路上的瀏覽器能夠連線。公開的實例需要 STUN，並需要 TURN 來服務無法直接連線的使用者。

伺服器發給每位參與者的是短期有效的 TURN 憑證，絕不是共用的密碼。`voice_turn_credential_ttl`（預設 `6h`）設定它們的有效期限。網頁用戶端會在憑證到期前十分鐘要求新的憑證並重新啟動 ICE，因此通話不會因為憑證到期而中斷。

<!-- source: coturn d665e942c91b -->
#### coturn

設定 `voice_turn = coturn`、`voice_turn_coturn_urls` 和 `voice_turn_coturn_secret`。伺服器會自行計算憑證（使用者名稱是到期時間和工作階段 ID，密碼是以密鑰對它計算的 HMAC-SHA1），因此不會向 TURN 伺服器發出任何請求。請在 `turnserver.conf` 中將 coturn 設定成相符的內容：

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

- `denied-peer-ip` 很重要。沒有它的話，任何持有憑證的人都能利用 TURN 伺服器連到其內部網路上的主機（雲端中繼資料服務、資料庫、其他 VM）。請拒絕所有私有、loopback 和 link-local 範圍；只有在真的需要轉送到內部位址時，才允許它（`allowed-peer-ip`）。
- 開放 UDP 和 TCP 3478、`turns:` 用的 TCP 5349，以及 coturn 的轉送連接埠範圍（`min-port` / `max-port`，預設為 49152-65535）。
- 也請在 `voice_turn_coturn_urls` 中列出 `turns:` URL 或帶有 `?transport=tcp` 的 URL，讓位於封鎖 UDP 之網路上的使用者仍然能夠連線。
- coturn 只在第一次建立配置（allocation）時檢查憑證的到期時間。已經存在的配置在到期後仍然可以運作，新的憑證會在下一次配置時生效，而網頁用戶端會透過重新啟動 ICE 建立新的配置。

<!-- source: cloudflare-turn be5846573be8 -->
#### Cloudflare TURN

設定 `voice_turn = cloudflare`、`voice_turn_cloudflare_key_id` 和 `voice_turn_cloudflare_api_token`，值來自在 Cloudflare 儀表板中建立的 TURN 金鑰。參與者開設或加入聊天室時，伺服器會向 Cloudflare 索取該參與者的憑證（如果失敗，請求會回應 `503`，不會開設或加入任何聊天室，因此可以再試一次），並在參與者離開或被移除時撤銷憑證。

- 憑證最多有效 48 小時，因此 `voice_turn_credential_ttl` 不能超過 `48h`（否則伺服器會拒絕啟動）。
- **Cloudflare 會在憑證到期後不久中斷轉送**。網頁用戶端會在那之前更新憑證，因此請讓 `voice_turn_credential_ttl` 遠大於十分鐘。
- 價格（2026-10-26 確認）：每月前 1,000 GB 免費，之後傳送給用戶端的流量每 GB $0.05。依賴這項價格之前，請確認 Cloudflare 目前的價格。

<!-- source: hiding-ip-addresses de67ffa9e7b7 -->
### 隱藏 IP 位址

使用 `voice_ice_transport_policy = all`（預設值）時，瀏覽器在可以的情況下會直接連線，因此聊天室的參與者可能會得知彼此的 IP 位址。

使用 `relay` 時，瀏覽器只會使用 TURN 伺服器，因此參與者看到的是 TURN 伺服器的位址，而不是其他參與者的位址。音訊在瀏覽器之間仍然保持加密。`relay` 無法隱藏的是：TURN 伺服器的營運者（你或 Cloudflare）看得到每位參與者的位址，而這個伺服器看得到所有呼叫其 API 的人的位址。`relay` 需要 `voice_turn` 為 `coturn` 或 `cloudflare`（否則伺服器會拒絕啟動），而且每通通話都會經過 TURN 伺服器，使用 coturn 時會消耗你的頻寬，使用 Cloudflare 時則會增加你的傳輸費用。

<!-- source: what-p2p-cannot-enforce 4f97611bece5 -->
### P2P 無法強制執行的事

聊天室中只有部分參與者可以發言：主持人，以及主持人允許的聽眾。在 `p2p` 中，這是在兩個地方強制執行的：伺服器會拒絕轉送會讓不能發言的人送出音訊的連線 offer，其他瀏覽器也不會播放非發言者的音訊。

這能阻止一般使用者和官方網頁用戶端，但並非完整的防護。**兩個使用修改過的用戶端並互相配合的參與者，仍然可以交換音訊**：不能發言者的修改版用戶端可以送出音訊，接收者的修改版用戶端可以播放它，而伺服器無法看到加密媒體的內容。如果你必須保證誰能發言，例如有主持的公開活動，請使用 `sfu`，由媒體節點本身只轉送發言者的音訊。

<!-- source: media-nodes 17f4fc8eb80b -->
## 媒體節點

使用 `voice = sfu` 時，每個聊天室由一個媒體節點持有，該節點的程序負責轉送聊天室的音訊。一個聊天室不會跨越多個節點。

<!-- source: run-media-nodes fac01c5bdbf9 -->
### 執行媒體節點

- **`role = all`** 在與其他所有功能相同的程序中執行一個節點，位址為 `wss://<host>/voice/sfu`。一台開放了 UDP 和 TCP 連接埠的機器就足夠了。
- **`role = sfu`** 單獨執行一個節點，與 `web` 和 `worker` 程序並列。它與它們共用資料庫、`stream`（不能是 `hub`）和 `kv`（不能是 `memory`）：節點把自己和它的聊天室登錄到鍵值儲存中，並透過串流得知其聊天室的變動。它不執行任何任務，只提供健康檢查和節點的 WebSocket。可以依需要執行任意多個：`web` 程序會把每個新聊天室安排到預留名額最少、且容得下該聊天室的節點上。`web` 程序不執行節點。

每個節點需要：

| 項目 | 設定 | 說明 |
| --- | --- | --- |
| 一個公開 IP 位址 | NAT 後方使用 `voice_sfu_public_ip` | 節點把這個位址告訴瀏覽器；未設定時使用它自己的位址。雲端 VM 或容器通常有自己的私有位址：請設定它。 |
| 對網際網路開放的 UDP 連接埠 | `voice_sfu_udp_port`（7882） | 節點的所有工作階段共用它。 |
| 對網際網路開放的 TCP 連接埠 | `voice_sfu_tcp_port`（7881） | 供封鎖 UDP 的網路上的瀏覽器使用。 |
| 帶 TLS 的 `wss:` URL | `voice_sfu_public_url`（`wss://<host>/voice/sfu`） | 每個節點都有自己的 URL，例如 `wss://sfu1.example.org/voice/sfu`。前方的反向代理終止 TLS 並轉送 WebSocket，或是由節點自己提供 HTTPS：`tls` 設定在 `role = sfu` 上同樣有效，作用於 `voice_sfu_public_url` 的主機（[由伺服器提供 HTTPS](running.md#serving-https-from-the-server)）。 |
| 它能容納多少參與者 | `voice_sfu_node_capacity`（500） | 請參閱[容量](#capacity)。 |

請把 UDP 和 TCP 連接埠原樣轉送給節點：節點告訴瀏覽器的是它自己的連接埠號碼。如果節點所在的主機不同於 `host`，還需要在 `web` 程序上設定 `voice_sfu_origins`，例如 `wss://*.sfu.example.org`，這樣網頁用戶端的內容安全政策才會允許該連線。

<!-- source: docker-compose 35fa31462add -->
### Docker Compose

[`compose.sfu.yaml`](../../deploy/docker/compose.sfu.yaml) 這一層在一台機器上執行 `role = web` 的伺服器、一個 `worker` 和一個媒體節點 `sfu`，資料庫為 Caddy 後方的 PostgreSQL（[Docker Compose 檔案](running.md#docker-compose-files)）：

```sh
# In .env:
# COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml:compose.sfu.yaml
# MY_VOICE_SFU_PUBLIC_IP=203.0.113.10   the public address of this machine
docker compose up -d
```

- Caddy 把 `/voice/sfu` 轉給節點，把其他所有請求轉給伺服器，因此節點的 URL 就是預設的 `wss://<MY_HOST>/voice/sfu`，不需要 `voice_sfu_origins`。
- 除了 80 和 443 之外，還要在機器的防火牆中開放 UDP 7882 和 TCP 7881。
- 不論 `.env` 怎麼寫，這一層都會為這三個程序設定 `voice = sfu` 和 `stream = postgres`。

<!-- source: capacity 2137d8a7bcce -->
### 容量

聊天室被安排到節點上時，會在該節點上預留它的上限（主持人為該聊天室設定的上限，或 `voice_max_participants`），不論實際加入了多少人。只有當節點上所有預留之和不超過 `voice_sfu_node_capacity` 時，節點才會接收聊天室，因此節點持有的參與者永遠不會超過它的容量。當沒有任何節點容得下某個聊天室的上限時，開設該聊天室或第一次加入都會回應 `503`，使用者可以稍後再試。

請依機器能轉送的量來設定 `voice_sfu_node_capacity`（[實測成本](#measured-cost)），並讓 `voice_max_participants` 不要超過你的聊天室所需：即使聊天室裡只有 3 個人，500 人的聊天室也會預留 500 個名額。

<!-- source: measured-cost 2d6571f6e923 -->
### 實測成本

測試使用以每秒 50 個封包傳送 Opus 音訊的模擬用戶端，在 AMD Ryzen 9 7900（12 核心）、WSL2 下的 Linux 和 Go 1.27 上進行，全部透過迴路介面。CPU 以該機器的核心數計；記憶體是節點的堆積和 goroutine 堆疊，加在程序其餘部分之上。

| 項目 | CPU | 記憶體 |
| --- | --- | --- |
| 閒置的節點 | 無法測出 | 約 0.1 MiB |
| 每個開啟的聊天室 | 無法測出 | 少於約 25 KiB |
| 每位參與者 | 靜音時約 0.0002 核心 | 約 180 KiB |
| 每條轉送的串流（一位發言者被一位參與者聽到） | 每個封包 18 到 27 µs，約 0.001 核心 | 少於約 25 KiB |
| 100 人的聊天室，3 位發言者正在發聲 | 約 0.3 核心 | 約 23 MiB |

更快或更慢的機器會按比例改變這些數字；節點自己的指標（[需留意的指標](#metrics-to-watch)）會顯示它承載的負載。

轉送的成本取決於節點送出的封包：每位發言者的音訊都會送給其他每位參與者，所以有 K 位發言者的 N 人聊天室大約會送出 K × (N − 1) 條 30 到 50 kbit/s 的串流。聽眾多、發言者少的聊天室成本很低；所有人同時發言的聊天室成本最高。請用同樣的方式規劃節點的外送頻寬。

<!-- source: drain-a-node-for-a-deploy 249929addcae -->
### 為部署排空節點

收到 SIGTERM 時，節點會先排空再停止：

1. 它不再接收新的聊天室。
2. 它把自己的每個聊天室安排到另一個節點上，並通知聊天室的用戶端移動。用戶端用新的授權重新連線到新節點，音訊會中斷約 1 到 2 秒。
3. 它等待用戶端離開，最多等待關機 25 秒中的 20 秒，然後等待剩餘的請求並結束（[停止伺服器](running.md#stop-the-server)）。

請讓程序在被終止之前至少有 30 秒的寬限期（Kubernetes 和 ECS 預設為 30 秒，Docker 為 10 秒，`compose.sfu.yaml` 把它調高了），並且一次只重新啟動一個節點，讓其他節點能接收它的聊天室。沒有其他節點容得下的聊天室會留在正在排空的節點上，直到聊天室結束或 20 秒過去。**只有一個節點時**，所有聊天室都會留下：節點結束時參與者會失去音訊，並在新節點啟動後重新連線。

<!-- source: when-a-node-is-lost c577f2240d7e -->
### 節點遺失時

節點不經排空就停止（當機、機器遺失）時，它的用戶端的 WebSocket 會關閉，它在鍵值儲存中的租約會在 15 秒內到期。用戶端會要求新的授權，`web` 程序會把聊天室重新安排到另一個節點上。聊天室不會因節點遺失而失去任何內容：它的參與者、發言者和靜音狀態都在資料庫中。

<!-- source: turn-for-clients-without-udp 1c1230dc875c -->
### 為沒有 UDP 的用戶端提供 TURN

瀏覽器透過 UDP 連線到節點，UDP 被封鎖時則透過 TCP。在只放行 443 連接埠的網路（部分企業網路和公共網路）上，它需要 443 連接埠上以 TLS 運作的 TURN 伺服器：把 `voice_turn` 設為 `coturn`，並在 `voice_turn_coturn_urls` 中給出 443 連接埠的 `turns:` URL，或是像 `p2p` 一樣設為 `cloudflare`（[coturn](#coturn)、[Cloudflare TURN](#cloudflare-turn)）。`sfu` 下 `voice_ice_transport_policy = relay` 會被拒絕：參與者本來就只看得到節點的位址。

<!-- source: cloud-run 5cf3ffcbb7a6 -->
### Cloud Run

Cloud Run 只接受 HTTP 請求，因此 `role = sfu` 無法在那裡執行。請把 `web` 和 `worker` 程序執行在 Cloud Run 上，把節點執行在有公開 IP 的 VM 上，並與它們共用資料庫、`stream`（`postgres` 或 `pubsub`）和 `kv`（`db` 或 `valkey`）。請為每個節點設定各自的主機名稱和 `voice_sfu_public_url`，在節點上（`tls = acme`）或其前方提供 TLS，並把 Cloud Run 服務上的 `voice_sfu_origins` 設為這些節點的來源。[在 Google Cloud 上部署](deploy-google-cloud.md)的 OpenTofu 模組不會建立這些節點。

<!-- source: metrics-to-watch a8b32402b50d -->
### 需留意的指標

每個節點都會匯出自己的 `my.voice.sfu.*` 指標（[指標](running.md#metrics)）。

| 指標 | 它告訴你什麼 |
| --- | --- |
| `my.voice.sfu.capacity.reserved` 與 `voice_sfu_node_capacity` 對照 | 節點對新聊天室來說有多滿。當所有節點都接近容量時，新聊天室會得到 `503`：請增加節點，或在節點還有閒置 CPU 時提高容量。 |
| `my.voice.sfu.capacity.connected`、`my.voice.sfu.sessions`、`my.voice.sfu.rooms` | 誰已連線。遠低於預留量表示聊天室的上限比實際需要的大。 |
| 依 `my.outcome` 區分的 `my.voice.sfu.packets` | `forwarded` 是節點的工作量。`silent` 和 `not_allowed`（不能發言或被靜音的參與者）依設計被丟棄。`queue_full` 表示某位參與者的連線跟不上：只有少數幾個時是用戶端太慢，所有用戶端同時上升時則是節點的 CPU 或頻寬不足。 |
| `my.voice.sfu.bytes` | 轉送的音訊量：節點的外送頻寬。 |
| `my.voice.sfu.client.rtt`、`my.voice.sfu.client.packet_loss` | 到參與者途中的品質。多數用戶端都偏高，表示問題在節點的網路或位置；只有少數偏高，則在他們自己的網路。 |
| `my.voice.sfu.offer.wait` | 加入的參與者等待節點第一個 offer 的時間：CPU 有餘裕的節點上是毫秒級；達到秒級表示加入要求在忙碌的節點上排隊。 |
| `my.voice.sfu.migrations`、`my.voice.sfu.ice_restarts`、`my.voice.sfu.renegotiations` | 因排空而移動的聊天室、更換了網路的用戶端，以及發言者的變動。 |
| 依 `my.voice.sfu.reason` 區分的 `my.voice.sfu.ticket_refusals` | `other_node` 持續上升，表示用戶端連到的節點不是其授權中的那個：請檢查每個節點的 `voice_sfu_public_url` 是否指向該節點。 |

<!-- source: deployment-notes 63042f72a7c5 -->
## 部署注意事項

- **每位使用者兩條串流**：通話期間，參與者除了應用程式一直開啟的串流之外，還會持有聊天室本身的事件串流（Server-Sent Events）。請以通話中的每位使用者兩個長時間連線的請求來設定連線數上限。
- **多個程序**：和應用程式的其他部分一樣，請使用共用的串流轉接器（`stream` 設為 `postgres`、`valkey`、`sns` 或 `pubsub`）。使用 `stream = hub` 且 `role` 不是 `all` 時，伺服器會在啟動時警告語音聊天無法跨程序運作。
- **Cloud Run**：串流是一個請求，在它開啟期間會分配 CPU。當請求逾時切斷它時，用戶端會重新連線。請把逾時設定得盡可能長：請參閱[在 Google Cloud 上部署](deploy-google-cloud.md#limits)中的 `service.request_timeout_seconds`。
- **媒體節點需要 VM**：`sfu` 的媒體節點在長時間執行的程序中透過 UDP 接收音訊，Cloud Run 無法代管它（[Cloud Run](#cloud-run)）。

<!-- source: ending-rooms-and-records b4d3d196ecd7 -->
## 結束聊天室與紀錄

- 擁有 `manage_reports` 的版主可以為所有人結束開啟中的聊天室，即使語音聊天已關閉也可以（[審核](moderation.md#voice-chat-rooms)）。
- 誰參與了聊天室（參與紀錄、發言者與聽眾、舉手、邀請與移除）會在聊天室結束後保留 `voice_participants_retention_days` 天（預設 90，至少 30），或在有檢舉指向該聊天室期間一直保留。聊天室的名稱、說明、主持人以及結束方式會被保留。音訊、麥克風狀態和反應從不記錄。
- `voice_public_summary` 控制無法加入 `local` 聊天室的訪客，能否在聊天室頁面和連結預覽中看到它的名稱以及它是否開啟中（[實例設定](instance-settings.md)）。

<!-- source: notes-for-users f44150f15915 -->
## 給使用者的注意事項

- 在 iPhone 或 iPad 上，應用程式在背景時可能會停止傳送音訊，因為 iOS 可能會暫停不在前景的網頁。通話期間請讓應用程式保持在畫面上開啟。
- 瀏覽器第一次會要求使用麥克風的權限。如果你拒絕了，請在瀏覽器的網站設定中重新允許。
- 在點對點聊天室中，除非實例透過轉送伺服器傳送通話，否則其他參與者可能會得知你的 IP 位址。如果這對你很重要，請詢問營運者。
