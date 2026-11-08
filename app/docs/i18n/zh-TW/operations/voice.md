<!-- source: run-voice-chat 54808e78eb0d -->
# 執行語音聊天

語音聊天讓已登入的使用者開設聊天室、邀請其他人並交談。在你選擇音訊的傳輸方式之前，它都是關閉的。本指南說明這個選擇、每種方式所需的 STUN 和 TURN 伺服器或媒體伺服器，以及部署、隱私和審核方面需要知道的事。這裡提到的每個設定都說明於[設定參考](../reference/configuration.md#voice)中。

<!-- source: choose-how-the-audio-travels 9d60bd0655e9 -->
## 選擇音訊的傳輸方式

`voice` 為整個實例選擇一種方式；所有聊天室都使用它。

| `voice` | 由誰傳送音訊 | 適合的情況 |
| --- | --- | --- |
| `off` | 無。語音聊天 API 回應 `404`，網頁用戶端不顯示語音聊天，啟動時也不會檢查其他 `voice_*` 設定。 | 預設值。 |
| `p2p` | 參與者的瀏覽器直接把音訊傳給彼此。伺服器只轉送建立連線用的訊息。 | 小型聊天室，以及最便宜的配置：除了選用的 STUN 或 TURN 伺服器之外，不需要額外的伺服器。 |
| `livekit` | LiveKit 伺服器，自行架設或使用 LiveKit Cloud。 | 較大的聊天室，以及嚴格控制誰可以發言。 |
| `sora` | Sora 伺服器，自行架設或使用 Sora Cloud。 | 同上，適合你已經在運作 Sora 的情況。 |

![三種方式並列。使用 p2p 時，伺服器轉送訊號，瀏覽器直接或經由 TURN 伺服器把音訊傳給彼此。使用 livekit 和 sora 時，瀏覽器把訊號和音訊傳給媒體伺服器，伺服器則透過 API 和 webhook 與該媒體伺服器溝通。下方列表會說明每種方式。](images/voice-methods.svg)

不論哪種方式，伺服器（`my serve`）都負責保存聊天室、參與者及其工作階段，並發給每位參與者一份授權：也就是瀏覽器連線所需的資訊。它從不傳送音訊。不同之處在於信令（建立連線用的訊息）和音訊流向哪裡：

- **`p2p`**。瀏覽器把它的訊號（offer、answer 和 ICE candidate）傳給伺服器，伺服器再透過聊天室的事件串流轉給另一個瀏覽器；訊號最多在鍵值儲存中等待 30 秒。音訊直接從瀏覽器傳到瀏覽器。授權中列出 `voice_stun_urls` 的 STUN 伺服器和 `voice_turn`（`coturn` 或 `cloudflare`）的 TURN 伺服器，以及短期有效的憑證。瀏覽器透過 STUN 得知自己的公開位址，當兩個瀏覽器找不到直接路徑時，則由 TURN 伺服器轉送音訊。
- **`livekit`**。授權中包含 `voice_livekit_url` 和一個 60 秒有效的存取權杖。瀏覽器把訊號和音訊傳給 LiveKit 伺服器，LiveKit 自備 STUN 和 TURN，並把音訊轉送給其他參與者（選擇性轉送單元，SFU）。伺服器透過 LiveKit 的 API 建立 LiveKit 聊天室並變更誰可以發言，LiveKit 則透過 webhook 向伺服器回報加入與離開。
- **`sora`**。授權中包含 `voice_sora_signaling_urls` 和一個已簽署的加入值。瀏覽器把訊號和音訊傳給 Sora 伺服器，Sora 自備 STUN 和 TURN。Sora 在每次連線之前都會詢問伺服器的驗證 webhook，伺服器透過 Sora 的 API 控制聊天室，Sora 則透過 webhook 回報連線狀況。

選擇時的注意事項：

- 使用 `p2p` 時，音訊從不經過這個伺服器（有使用 TURN 伺服器時會經過 TURN 伺服器）。伺服器的負載只有聊天室訊息、心跳和串流。
- 小型實例最適合使用 `p2p`，或 LiveKit Cloud 的免費 Build 方案（請參閱 [LiveKit Cloud](#livekit-cloud)）。
- 使用 `livekit` 或 `sora` 時，不會使用下方的 STUN 和 TURN 設定以及 ICE 政策（如果你設定了，伺服器會在啟動時發出警告），因為媒體伺服器自備這些伺服器。這時參與者的 IP 位址只有媒體伺服器看得到，其他參與者看不到。
- 建置標籤 `no_voice_livekit` 和 `no_voice_sora` 可以把這兩種媒體伺服器方式從執行檔中排除（[建置標籤](../reference/build-tags.md)）。
- 請以 HTTPS 提供實例：瀏覽器只允許安全的頁面存取麥克風。

這些限制讓付費媒體伺服器的費用維持在一定範圍內。除了最後一項之外，設為 `0` 即可取消任一限制：

| 設定 | 預設值 | 意義 |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | 聊天室最長的開啟時間。達到時聊天室會結束。 |
| `voice_max_open_rooms` | `10` | 這個伺服器上可以同時開啟的聊天室數。 |
| `voice_sfu_max_participants` | `10` | `livekit` 和 `sora` 的聊天室人數上限。 |
| `voice_p2p_max_participants` | `10` | `p2p` 的聊天室人數上限，從 2 到 50。 |

誰可以開設聊天室由實例設定 `voice_room_starters` 決定（[實例設定](instance-settings.md)）：預設只有擁有 `start_voice_rooms` 權限的人（內建的 `admin` 角色具有此權限）；`everyone` 則讓每個沒有被停權或限制、已登入的本站使用者都能開設聊天室。

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

<!-- source: size-and-bandwidth 9872bf829e26 -->
### 人數與頻寬

只要兩個瀏覽器中至少有一個在發言，它們就會互相連線；兩個只在收聽的人不會連線。因此發言者會把自己的音訊分別傳一份給其他每位參與者。Opus 編碼的語音加上額外負擔大約需要 30 到 50 kbit/s，所以在 10 人的聊天室中，發言者要上傳約 9 份，也就是幾百 kbit/s；在 50 人的聊天室中則要上傳 49 份，約 2 Mbit/s，使用行動數據的手機或家用網路可能無法負荷。N 人聊天室中的連線數最多為 N(N-1)/2：10 人時為 45 條，50 人時為 1,225 條。這就是預設值為 10 的原因。只有在使用者有足夠的上傳頻寬時才提高 `voice_p2p_max_participants`；大型聊天室請使用 `livekit` 或 `sora`。

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

<!-- source: what-p2p-cannot-enforce 31e06affcd4f -->
### P2P 無法強制執行的事

聊天室中只有部分參與者可以發言：主持人，以及主持人允許的聽眾。在 `p2p` 中，這是在兩個地方強制執行的：伺服器會拒絕轉送會讓不能發言的人送出音訊的連線 offer，而其他瀏覽器也不會播放非發言者的音訊。

這能阻止一般使用者和官方網頁用戶端，但並非完整的防護。**兩個使用修改過的用戶端並互相配合的參與者，仍然可以交換音訊**：不能發言者的修改版用戶端可以送出音訊，接收者的修改版用戶端可以播放它，而伺服器無法看到加密媒體的內容。如果你必須保證誰能發言，例如有主持的公開活動，請使用 `livekit` 或 `sora`，由媒體伺服器本身丟棄沒有權限者的音訊。

<!-- source: deployment-notes b0f03da306b7 -->
## 部署注意事項

- **每位使用者兩條串流**：通話期間，參與者除了應用程式一直開啟的串流之外，還會持有聊天室本身的事件串流（Server-Sent Events）。請以通話中的每位使用者有兩個長時間請求來規劃連線上限。
- **多個程序**：和應用程式的其他部分一樣，請使用共用的串流轉接器（`stream` 設為 `postgres`、`valkey`、`sns` 或 `pubsub`）。使用 `stream = hub` 且 `role` 不是 `all` 時，伺服器會在啟動時警告語音聊天無法跨程序運作。
- **Cloud Run**：串流是一個請求，在它開啟期間會分配 CPU。當請求逾時切斷它時，用戶端會重新連線。請把逾時設定得盡可能長。
- **媒體伺服器需要 VM**：LiveKit 和 Sora 在長時間執行的程序中透過 UDP 接收音訊。Cloud Run 只接受 HTTP 請求，因此無法執行它們。請在 VM 上執行它們（以及 coturn），或使用 LiveKit Cloud、Sora Cloud 或 Cloudflare TURN。

<!-- source: livekit b16c1d192419 -->
## LiveKit

設定 `voice = livekit` 以及：

| 設定 | 值 |
| --- | --- |
| `voice_livekit_url` | 瀏覽器連線的 `wss:` URL，例如 `wss://livekit.example.org`。 |
| `voice_livekit_api_url` | 這個伺服器呼叫的 `https:` URL，在它與上方的 URL 不同時設定。預設為同一個主機，使用 `https:`。 |
| `voice_livekit_api_key`、`voice_livekit_api_secret` | LiveKit 伺服器的金鑰組。密鑰用於簽署參與者的存取權杖，並驗證 webhook。 |

伺服器會在 LiveKit 上建立每個聊天室，以聊天室 ID 命名，並發給每位參與者一個 60 秒有效的存取權杖。聽眾的權杖無法發布；發言者的權杖只能發布麥克風。

<!-- source: self-hosted-livekit 2b118136f4dc -->
### 自行架設 LiveKit

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

- LiveKit 伺服器必須能透過 HTTPS 連到 webhook URL。伺服器會以 API 密鑰驗證每個 webhook 的簽章。
- webhook 只是用來加快反應。伺服器每分鐘都會比較每個開啟中聊天室的參與者與 LiveKit 回報的內容，並移除不該在場的人，因此實例冷啟動期間遺失的 webhook 也會被補上。
- 若要在你的電腦上試用，`livekit-server --dev` 會以固定的開發用金鑰組啟動。
- **被移除的參與者可以在短時間內重新加入**。自行架設的 LiveKit 無法撤銷權杖：它會持續把新的權杖送給已連線的用戶端，每個權杖的有效期限最長約 10 分鐘。因此被移除的人最多可以在約 10 分鐘內用最新的權杖重新加入，直到伺服器察覺（透過加入 webhook 或每分鐘的檢查）並切斷他們為止，這段期間他們可能會被聽見幾秒到一分鐘。如果移除必須立即生效，請使用 LiveKit Cloud：它會立即撤銷權杖。

<!-- source: livekit-cloud 5bd9f833be11 -->
### LiveKit Cloud

使用專案的 WebSocket URL、API 金鑰和密鑰，並在專案設定中把 `https://<your domain>/voice/webhooks/livekit` 加為 webhook 端點。免費的 Build 方案每月提供 5,000 分鐘和 50 GB，以及 100 個同時連線（2026-10-26 確認）；目前的方案請查看 LiveKit 的價格頁面。

<!-- source: sora 5e9209dd3baa -->
## Sora

**Sora 轉接器只針對依照 Sora 文件模擬的 Sora 測試過，沒有針對真正的 Sora 伺服器測試過**。在依賴它之前，請先在預備環境的實例上搭配真正的 Sora 試用。

設定 `voice = sora`、`voice_sora_signaling_urls`（以逗號分隔、供瀏覽器連線的 `wss:` URL）、`voice_sora_join_secret`（至少 32 位元組，例如由 `openssl rand -base64 32` 產生；用來簽署參與者加入時使用的值），以及 `voice_sora_mode`（`self_hosted` 或 `cloud`）。實例必須以 HTTPS 提供，因為 Sora 會回呼 `https://<your domain>`。

<!-- source: webhooks 11b2c486a476 -->
### Webhook

Sora 會呼叫這個伺服器的兩個端點：

| 端點 | 用途 |
| --- | --- |
| `/voice/webhooks/sora/auth` | 驗證 webhook。Sora 在每次連線之前都會呼叫它。伺服器只會允許位於聊天室中、沒有被移除、以發言者身分連線時具有發言權限，且未超過人數上限的參與者，並告訴 Sora 只接受音訊。 |
| `/voice/webhooks/sora/events` | 事件與工作階段 webhook（連線的建立與銷毀、工作階段的結束）。 |

**驗證 webhook 必須及時回應**。自行架設時 Sora 會等待 5 秒，Sora Cloud 則是 10 秒。伺服器會根據自己的資料回應，不會呼叫其他任何服務，但實例的冷啟動（Cloud Run 從零開始擴充）可能會超過這個時間，這時連線就會被拒絕。在語音聊天使用期間，請保持一個執行個體處於暖機狀態。

自行架設時，請在 `sora.conf` 中設定 `default_duplicate_client_id = evict`。聽眾變成發言者時會以相同身分重新連線（Sora 無法變更連線的角色），而 `evict` 會讓 Sora 在新的連線到達時丟棄舊的連線。使用預設的 `allow` 時，伺服器會在得知新連線後自行切斷舊連線，因此兩者會短暫重疊。

<!-- source: self-hosted-sora dd3244368b73 -->
### 自行架設 Sora

| 設定 | 值 |
| --- | --- |
| `voice_sora_mode` | `self_hosted` |
| `voice_sora_api_url` | 位於 Sora HTTP API 前方之反向代理的 `https:` URL。 |
| `voice_sora_api_header_name`、`voice_sora_api_header_value` | 代理用來驗證這個伺服器的標頭，例如 `Authorization` 和 `Basic ...`。 |
| `voice_sora_webhook_username`、`voice_sora_webhook_password` | webhook 的 Basic 認證資訊。必填。 |

**Sora 本身的 HTTP API 沒有驗證機制**，設計上只應在 loopback 位址上監聽。絕對不要把它公開。請在它前面放置一個要求認證資訊的反向代理，並透過上述兩個標頭設定把該認證資訊交給這個伺服器。請在 Sora 中以相同的 Basic 認證資訊設定 webhook URL。伺服器只接受 `https:` 的 API URL（`http:` 只限 localhost），沒有 webhook 認證資訊時不會啟動。不支援 mTLS，因為 Cloud Run 和位於 ECS 前方的負載平衡器會在伺服器之前終止 TLS。

<!-- source: sora-cloud ac4935c7f7a7 -->
### Sora Cloud

設定 `voice_sora_mode = cloud`、`voice_sora_cloud_project_id` 和 `voice_sora_cloud_api_key`。伺服器會以該金鑰簽署存取權杖、呼叫 API，並驗證 webhook 的簽章；不需要 Basic 認證資訊。

<!-- source: behavior-to-expect 03fd5e1655fa -->
### 預期的行為

- 獲准發言的聽眾會自動以發言者身分重新連線，因此在新連線建立期間會有短暫的空檔。
- 被取消發言權的發言者會立即被轉送過濾器靜音，然後以聽眾身分重新連線。

<!-- source: licensing 534e304817e3 -->
### 授權

Sora 是時雨堂（Shiguredo）的商業產品。2026-10-26 確認時：Sora Cloud 的基本費用為每月 ¥5,000，沒有免費期間；自行架設需要套裝授權，依同時連線數和期間預先付費。產品頁面表示不提供給日本以外的公司、組織或個人，因此在以它為前提進行規劃之前，請確認你是否能夠購買。自行架設時，LiveKit 是開放原始碼的。

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
