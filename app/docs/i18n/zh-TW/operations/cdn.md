<!-- source: put-a-cdn-in-front-of-media-delivery 20ee3d2cefe9 -->
# 在媒體傳送前方加上 CDN

放在伺服器前方的 CDN（Cloudflare、Google Cloud CDN、CloudFront 等）可以負責傳送媒體（`/media/` 與 `/media/proxy/`）。由 CDN 快取回應的請求，完全不會用到伺服器的 CPU 或傳輸量。本指南說明以伺服器作為 CDN 來源（origin pull）的架構。它適用於所有 blob 儲存：使用 `blob = s3` 或 `blob = gcs` 時，伺服器會從私有 bucket 讀取檔案，並與 `blob = fs` 一樣在 `/media/` 下提供，因此 bucket 永遠不會成為 CDN 的來源。當整個伺服器都位於 CDN 後方時，還可以[拒絕沒有經過 CDN 的請求](#refuse-requests-that-did-not-come-through-the-cdn)。

<!-- source: configure-it e915ce01da94 -->
## 設定方式

將 `media_origin`（`MY_MEDIA_ORIGIN`）設為 CDN 的主機名稱（例如 `media.example.org`）。

```sh
MY_MEDIA_ORIGIN=https://media.example.org
```

- 這個值只能是 origin（`scheme://host[:port]`），不能有路徑、查詢字串或結尾斜線。必須使用 `https`，只有 `localhost` 這類 loopback 主機才允許 `http`。值無效時，啟動會失敗。
- 未設定時，媒體會從伺服器自己的 origin（`https://{host}`）提供。
- 以下 URL 使用這個 origin：本地媒體為 `{media_origin}/media/...`，遠端媒體的代理為 `{media_origin}/media/proxy/...`。用戶端 API、ActivityPub（`attachment`、`icon`、`image`，以及自訂表情符號的 `icon`）和連結預覽（OGP）輸出的所有 URL 都涵蓋在內。
- 網頁用戶端的 Content-Security-Policy 會把這個 origin 加入 `img-src` 和 `media-src`。
- 聯邦 URI（例如 ActivityPub 物件的 `id`）不論 `media_origin` 為何，都維持在 `https://{host}/` 之下。改變的只有提供檔案的位置，而不是識別碼。
- URL 是每次讀取時產生的，不會儲存在資料庫中，因此變更 `media_origin` 也會改變既有媒體的 URL。只要舊 origin 上的 `/media/` 持續提供服務，遠端伺服器儲存的舊 URL 就仍然有效。

<!-- source: point-the-cdn-at-the-server ffb279f38f08 -->
## 讓 CDN 指向伺服器

- 將 origin（後端）設為伺服器的公開主機名稱（`https://{host}`）。CDN 只需要從伺服器取得 `/media/` 底下的路徑。
- 伺服器不會依主機名稱限制 `/media/` 和 `/media/proxy/`。CDN 送來任何 `Host` 都會提供。
- `/media/` 不需要驗證，Authorized Fetch 也不適用。伺服器會直接回應 CDN 送來的未簽章請求。
- 遠端媒體（`/media/proxy/{id}/{variant}?sig=...`）的 `sig` 是 URL 的一部分。**請把查詢字串納入快取鍵**。否則簽章不同的 URL 會共用同一筆快取。
- 讓 origin 的 `Cache-Control` 決定快取多久（不要在 CDN 端覆寫）。

| 路徑 | `Cache-Control` |
| --- | --- |
| `/media/{...}`（本地媒體） | `public, max-age=31536000, immutable`（一年） |
| `/media/proxy/{...}`（遠端媒體） | `public, max-age=` 加上實例設定 `remote_media_cache_ttl_seconds` 的秒數 |

兩者回應時都帶有 `Cross-Origin-Resource-Policy: cross-origin`，因此其他 origin 的頁面也能讀取。

在 Cloud Run 上，`ingress-cloudflare-media` 會用 OpenTofu 完成這些設定：請參閱[在 Cloud Run 上部署](deploy-cloud-run.md#cloudflare)。

<!-- source: who-can-open-media-urls 27306b0bc9be -->
## 誰能開啟媒體 URL

媒體 URL 沒有存取控制。任何擁有檔案 URL 的人都能開啟它，不需要登入，也不論附加它的貼文是什麼可見範圍。不論有沒有 CDN 都是如此，包括僅追蹤者貼文和私訊的附件，以及遠端媒體的代理 URL。

- 這些 URL 很難猜到：本地檔案的 URL 包含帶有隨機部分的識別碼，代理 URL 則帶有簽章。知道一個 URL 並不會洩漏其他 URL。
- 任何看過貼文的人都可以把它的媒體 URL 轉給別人，而這些 URL 對別人也同樣有效。
- 回應標示為 `public`，因此 CDN 以及伺服器和讀者之間的任何共用快取都可能保留副本（保留多久請見上表）。正因如此，CDN 才能不詢問伺服器就提供媒體。

請告訴你的使用者：非公開貼文會限制誰能看到貼文，但無法阻止擁有附件檔案連結的人開啟該檔案。

<!-- source: deletion-and-the-cache baac405df923 -->
## 刪除與快取

刪除媒體後，CDN 的快取仍會保留它 `max-age` 的時間。已刪除的檔案在本地媒體最長可能仍可看到一年，代理則最長為 `remote_media_cache_ttl_seconds`。伺服器不會清除 CDN 的快取。如果某些內容必須在刪除或審核處置後立刻消失，請在 CDN 的主控台或 API 手動清除。

<!-- source: service-worker f6efdae181a5 -->
## Service Worker

網頁用戶端的 Service Worker 不會快取來自其他 origin 的媒體，而是交給瀏覽器的網路處理，因此只有 CDN 的快取會生效。

<!-- source: refuse-requests-that-did-not-come-through-the-cdn 5df4225c62b3 -->
## 拒絕沒有經過 CDN 的請求

CDN 只能保護伺服器免受經過它的請求影響。當 origin 也能透過自己的位址存取時（例如在 Cloud Run 上接受網際網路流量的 `*.run.app`），任何人都可以繞過 WAF 和快取直接呼叫它。如果 CDN 的位址列在 `trusted_proxies` 中，那麼在 CDN 本身的 serverless 平台上執行的呼叫者（例如他們自己的 Cloudflare Worker）也會從受信任的位址連線，並能自行決定速率限制所看到的用戶端 IP。

以下三個設定可以封住這個漏洞。CDN 會在轉送的每個請求加上一個祕密標頭，伺服器在處理請求的任何其他步驟之前，會對沒有這個標頭的請求回應 `403`。

![啟用 origin 檢查時請求的路徑：瀏覽器、應用程式和其他伺服器經過 CDN，由 CDN 加上祕密標頭；平台和其他任何人則不帶標頭直接連到伺服器；伺服器的 origin 檢查會放行帶有標頭的請求和豁免的請求，其餘則以 403 拒絕。下方清單說明每條路徑。](images/cdn-request-flow.svg)

- 瀏覽器和應用程式（頁面、用戶端 API、OAuth 和媒體 URL）以及其他伺服器（ActivityPub 的投遞與擷取）會連到位於 `host` 和 `media_origin` 主機的 CDN。CDN 會自行回應已快取的 `/media/` 檔案，並在轉送給伺服器的每個請求上把 `origin_secret_header` 設為 `origin_secret`。
- 平台（健康檢查探測、運作時間監控、Cloud Tasks 和 Cloud Scheduler）以及其他任何人（例如你自己的腳本或 webhook）會不帶標頭，直接連到伺服器自己的位址。
- origin 檢查會在伺服器中最先執行。它會放行只帶一次標頭且值為 `origin_secret` 或 `origin_secret_previous` 的請求；沒有標頭時，只放行健康檢查的 `GET` 和 `HEAD`，以及送往 `/_internal/` 的 `POST`。它會從每個請求中移除這個標頭，其餘請求則以 `403` 拒絕。
- 通過檢查的請求會到達網頁用戶端、用戶端 API、OAuth、ActivityPub、健康檢查或 `/_internal/`，或是 `/media/` 和 `/media/proxy/`，後兩者會讀取媒體儲存空間（`blob = fs`、`s3` 或 `gcs`），或轉送其他伺服器的檔案。被拒絕的請求會收到帶有 `Cache-Control: no-store` 的 `403`，並計入 `my.http.origin_refused`。

當伺服器有公開位址，而平台無法將它限制為只允許 CDN 連線時，請使用這個功能。如果防火牆或平台已經限制了誰能連線，例如伺服器只在你的負載平衡器後方的私有網路上監聽，就不需要它。如果負載平衡器本身會檢查 CDN 的標頭，且只接受 CDN 的位址，也不需要它，例如位於 CloudFront 後方的 AWS Application Load Balancer 就能透過 listener rule 和 managed prefix list `com.amazonaws.global.cloudfront.origin-facing` 做到。這時請在 `trusted_proxies` 中同時列出負載平衡器和 CDN 的位址，因為兩者都會附加到 `X-Forwarded-For`。

當流量是由平台本身的功能轉送，且不會加上你能控制的標頭時（例如 Cloud Run 的網域對應），這個功能就幫不上忙。只有能夠加上請求標頭的 CDN 或負載平衡器才可以。

| 設定 | 內容 |
| --- | --- |
| [`origin_secret`](../reference/configuration.md#origin_secret)（`MY_ORIGIN_SECRET`） | 值，至少 32 位元組的可見 ASCII 字元。屬於機密，從環境變數或 `MY_ORIGIN_SECRET_FILE` 讀取 |
| [`origin_secret_header`](../reference/configuration.md#origin_secret_header)（`MY_ORIGIN_SECRET_HEADER`） | 標頭名稱，由你自訂。使用 `origin_secret` 時必須設定。CDN 拒絕設定的名稱，或伺服器、代理會讀取的名稱（例如以 `cf-` 或 `x-cf-` 開頭者）都無效 |
| [`origin_secret_previous`](../reference/configuration.md#origin_secret_previous)（`MY_ORIGIN_SECRET_PREVIOUS`） | 第二個可接受的值，僅在[輪替期間](#rotate-the-value)使用 |

這些都未設定時，不會進行任何檢查，伺服器的行為就如同這一節不存在。

<!-- source: what-passes-without-the-header 38e1ceda6e34 -->
### 不帶標頭也能通過的請求

| 請求 | 原因 |
| --- | --- |
| `/healthz`、`/readyz`、`/api/v1/instance/health` 的 `GET` 或 `HEAD` | 平台的探測和運作時間監控會直接呼叫它們 |
| 送往 `/_internal/` 路徑的 `POST` | Cloud Tasks 和 Cloud Scheduler 會透過伺服器自己的 URL 呼叫它們。它們帶有自己的簽章權杖 |

其他所有請求都需要這個標頭，維護模式下也一樣：網頁用戶端及其資源、`/media/`、用戶端 API、OAuth 和 ActivityPub，包括其他伺服器的投遞。因此，任何不經過 CDN、直接以伺服器自己的位址呼叫它的東西都會被拒絕：你自己的 webhook、語音媒體伺服器、腳本。請讓它們指向 CDN 的主機名稱。

指標連接埠（`metrics_listen`）是不公開的獨立 listener，不會受到檢查。

不帶祕密的請求即使通過了，也無法影響後續的處理：不論 `X-Forwarded-For` 寫了什麼，它的用戶端 IP 都是連線的對端，而它的 `traceparent` 和 `X-Request-Id` 會被忽略。

<!-- source: set-it-up ee0a17ad2d33 -->
### 設定步驟

CDN 轉送給伺服器的每個主機名稱都需要這個標頭：`host`、`media_origin` 的主機，以及 CDN 有提供的獨立 `account_domain`。由同一個 CDN 代理的媒體主機也要設定，因為 `/media/` 和其他請求一樣會被拒絕。

1. 產生值，並選擇一個沒有其他用途的名稱：

   ```sh
   openssl rand -hex 32
   ```

2. **先在 CDN 設定標頭**。伺服器會忽略它不認識的標頭，因此這個步驟還不會改變任何事。
   - **Cloudflare**：請求標頭轉換規則（Rules → Transform Rules → Modify Request Header），所有方案都可使用。比對該 zone 的所有傳入請求，並以你的名稱和值選擇 **Set static**。它會取代用戶端送來的值。只針對部分主機名稱的規則，會讓其他主機名稱被擋在外面。
   - **Amazon CloudFront**：在每個 distribution 的 origin 上設定 origin custom header。它同樣會取代瀏覽者送來的值。
   - **Google Cloud 負載平衡器**：在後端服務上設定自訂請求標頭。
   - 其他任何 CDN 或代理：必須在送往伺服器的每個請求上設定（而不是附加）這個標頭。
3. 設定 `MY_ORIGIN_SECRET` 和 `MY_ORIGIN_SECRET_HEADER` 後部署。`my config check` 會驗證它們並重述檢查的內容，但它看不到 CDN。
4. 從外部以 origin 本身的位址確認：

   ```sh
   curl -s -o /dev/null -w '%{http_code}\n' https://<host>/                 # 200: through the CDN
   curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/       # 403: directly
   curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/healthz # 200: a health check
   ```

也請在 [`trusted_proxies`](../reference/configuration.md#trusted_proxies) 中列出 CDN 的位址，否則伺服器只會看到一個用戶端，所有請求都會共用 CDN 的速率限制；`my config check` 會對此發出警告。

被拒絕的請求不論標頭錯在哪裡，都會和其他被禁止的請求一樣收到相同的 `403` problem document，並帶有 `Cache-Control: no-store`。回應中不會提到這個標頭。

<!-- source: when-everyone-is-locked-out 47c8f706e5e7 -->
### 所有人都被擋在外面時

設定錯誤時，所有請求都會被拒絕，但健康檢查仍然是 `200`，因此平台看到的是健康的伺服器，監控 `/healthz` 的運作時間監控也察覺不到。徵兆如下：

- 經過 CDN 的每個頁面都是 `403`。
- 指標 `my.http.origin_refused` 上升，而 `http.server.request.duration` 的請求停止。
- 日誌中出現狀態為 `403` 且沒有 `http.route` 的 `http request` 行。日誌和追蹤永遠不會記錄這個標頭、它的名稱或值。被拒絕的請求不會建立追蹤 span。

要復原，請在不啟用檢查的情況下部署：取消設定 `MY_ORIGIN_SECRET` 和 `MY_ORIGIN_SECRET_HEADER`。其他伺服器會重試它們的投遞，但在這段期間使用者無法連到伺服器。接著比對 CDN 上的名稱和值與設定是否一致，確認規則涵蓋了每個主機名稱，然後再重新設定。

<!-- source: rotate-the-value b88f695fde31 -->
### 輪替值

CDN 和伺服器無法在同一瞬間變更，而伺服器的每個副本也會依各自的步調重新啟動，因此如果只接受一個值，變更期間只要兩者不一致，流量就會被拒絕。所以伺服器在輪替期間會接受兩個值：

1. 將 `MY_ORIGIN_SECRET` 設為新值，`MY_ORIGIN_SECRET_PREVIOUS` 設為舊值，然後部署。兩個值都會通過。
2. 在 CDN 變更值，並等待變更傳遞到它的所有據點。
3. 取消設定 `MY_ORIGIN_SECRET_PREVIOUS` 後部署。設定期間 `my config check` 會發出警告。

標頭名稱無法用這種方式輪替。要變更名稱，請在暫時關閉伺服器檢查的情況下重複[設定步驟](#set-it-up)，或在部署新名稱之前，先在 CDN 上同時設定新名稱和舊名稱。
