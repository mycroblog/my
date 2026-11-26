<!-- source: run-voice-chat 54808e78eb0d -->
# 執行語音聊天

語音聊天讓已登入的使用者開設聊天室、邀請其他人並交談。在你選擇音訊的傳輸方式之前，它都是關閉的。本指南說明這個選擇、每種方式所需的 STUN 和 TURN 伺服器或媒體伺服器，以及部署、隱私和審核方面需要知道的事。這裡提到的每個設定都說明於[設定參考](../reference/configuration.md#voice)中。

<!-- source: choose-how-the-audio-travels 1fe795fb2216 -->
## 選擇音訊的傳輸方式

`voice` 為整個實例選擇一種方式；每個聊天室都使用它。

| `voice` | 由什麼傳送音訊 | 適合 |
| --- | --- | --- |
| `off` | 什麼都沒有。語音聊天 API 回應 `404`，網頁用戶端不顯示語音聊天，其他 `voice_*` 設定在啟動時也不會被檢查。 | 預設值。 |
| `p2p` | 參與者的瀏覽器直接互相傳送音訊。伺服器只中繼建立連線所需的訊息。 | 小型聊天室與最便宜的設置：除了選用的 STUN 或 TURN 伺服器，不需要額外的伺服器。 |

![三種方式並排比較。使用 p2p 時，伺服器中繼訊號，瀏覽器彼此傳送音訊，或透過 TURN 伺服器傳送。使用 livekit 和 sora 時，瀏覽器將訊號與音訊傳送給媒體伺服器，而伺服器透過其 API 與 webhook 與該媒體伺服器溝通。下方的清單說明每一種方式。](images/voice-methods.svg)

不論哪種方式，伺服器（`my serve`）都負責保管聊天室、誰參與其中及其工作階段，並交給每位參與者一份授權：瀏覽器連線所需的內容。它從不傳送音訊。不同的是訊號（建立連線的訊息）與音訊走哪裡：

- **`p2p`。** 瀏覽器將它的訊號（offer、answer 和 ICE candidate）傳送給伺服器，伺服器透過聊天室的事件串流轉交給另一個瀏覽器；訊號最多會在鍵值儲存中等待 30 秒。音訊直接從瀏覽器傳到瀏覽器。授權中會列出 `voice_stun_urls` 的 STUN 伺服器，以及 `voice_turn`（`coturn` 或 `cloudflare`）的 TURN 伺服器與短期有效的憑證。瀏覽器從 STUN 得知自己的公開位址，而當兩個瀏覽器找不到直接的路徑時，TURN 伺服器會中繼音訊。

關於選擇的說明：

- 使用 `p2p` 時，音訊從不經過這個伺服器（有使用 TURN 伺服器時，則會經過它）。伺服器的負載只有聊天室訊息、心跳和串流。
- 小型實例最適合使用 `p2p`，或使用 LiveKit Cloud 免費的 Build 方案（請參閱 LiveKit Cloud）。
- 使用 `livekit` 或 `sora` 時，STUN 和 TURN 設定與下方的 ICE 政策都不會用到（如果你設定了，伺服器會在啟動時警告），因為媒體伺服器自備伺服器。這時參與者的 IP 位址只有媒體伺服器看得到，其他參與者看不到。
- 請以 HTTPS 提供實例：瀏覽器只允許在安全的頁面上使用麥克風。

這些上限讓付費媒體伺服器的成本維持在可控範圍內。除了最後一項之外，`0` 都會移除該項上限：

| 設定 | 預設值 | 意義 |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | 聊天室最長開放多久。達到時間的聊天室會被結束。 |
| `voice_max_open_rooms` | `10` | 這個伺服器上同時最多可以開放多少個聊天室。為排定時間而開啟的聊天室，從它開啟起就開始計算，包括它等待第一位參與者的時間。 |
| `voice_sfu_max_participants` | `10` | `livekit` 和 `sora` 的聊天室大小。 |
| `voice_p2p_max_participants` | `10` | `p2p` 的聊天室大小，範圍為 2 到 50。 |

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

<!-- source: deployment-notes 9daf6e4d8d98 -->
## 部署注意事項

- **每位使用者兩條串流**：通話期間，參與者除了應用程式一直開啟的串流之外，還會持有聊天室本身的事件串流（Server-Sent Events）。請以通話中的每位使用者有兩個長時間請求來規劃連線上限。
- **多個程序**：和應用程式的其他部分一樣，請使用共用的串流轉接器（`stream` 設為 `postgres`、`valkey`、`sns` 或 `pubsub`）。使用 `stream = hub` 且 `role` 不是 `all` 時，伺服器會在啟動時警告語音聊天無法跨程序運作。
- **Cloud Run**：串流是一個請求，在它開啟期間會分配 CPU。當請求逾時切斷它時，用戶端會重新連線。請把逾時設定得盡可能長：請參閱[在 Google Cloud 上部署](deploy-google-cloud.md#limits)的 `service.request_timeout_seconds`。
- **媒體伺服器需要 VM**：LiveKit 和 Sora 在長時間執行的程序中透過 UDP 接收音訊。Cloud Run 只接受 HTTP 請求，因此無法執行它們。請在 VM 上執行它們（以及 coturn），或使用 LiveKit Cloud、Sora Cloud 或 Cloudflare TURN。

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
