<!-- source: monitoring-the-server-from-outside 6cd7b2b809fa -->
# 從外部監控伺服器

本頁說明如何讓運作時間監控服務監控伺服器、如何避免監控提高營運成本，以及如何區分維護模式（[維護](maintenance.md)）與服務中斷。探測路徑請參閱[執行指南中的健康檢查一節](running.md#health-checks)。

下文中關於第三方服務的資訊都附有確認日期和出處。各服務會變更方案和畫面，在依賴某項限制之前，請先查看連結的頁面。

<!-- source: which-url-to-monitor 50bdaa5c9f31 -->
## 要監控哪個 URL

| 路徑 | 用途 | 是否存取資料庫 | 維護模式時 |
| --- | --- | --- | --- |
| `GET /api/v1/instance/health` | 運作時間監控、狀態頁面、網頁用戶端 | 每個程序每 60 秒最多一次 | `503`，附 `Retry-After` 和 `{"status":"maintenance"}` |
| `GET /healthz` | 平台的存活探測 | 從不 | `200` `{"status":"maintenance"}` |
| `GET /readyz` | 負載平衡器或 Kubernetes 的就緒探測 | 每次請求（同時的請求共用一次檢查） | `200` `{"status":"maintenance"}` |

請讓外部監控指向 **`/api/v1/instance/health`**，而不是 `/readyz`：`/readyz` 每次請求都會查詢資料庫，所以間隔短的監控會讓可縮減到零的資料庫一直保持喚醒。探測是給你的平台用的。[`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) 和 [`monitoring-aws`](../../deploy/opentofu/modules/monitoring-aws/README.md) 模組會從平台本身的監控以較長的間隔檢查 `/readyz`，它們的 README 說明了這樣做的費用。

`/api/v1/instance/health` 的回應（不需驗證，`Cache-Control: no-store`）：

| 狀態 | HTTP | 內文 |
| --- | --- | --- |
| 正常 | `200` | `{"status":"ok","checked_at":"<RFC 3339>"}` |
| 無法使用（無法連線到資料庫，或尚未套用結構描述） | `503` | `{"status":"unavailable","checked_at":"<RFC 3339>"}` |
| 維護模式 | `503`，附 `Retry-After` | `{"status":"maintenance"}` |

正常時，`/healthz` 和 `/readyz` 會以 `{"status":"ok"}` 回答 `200`；資料庫尚未就緒時，`/readyz` 會以問題文件回答 `503`。內文絕不會包含原因、主機名稱或版本。原因記錄在伺服器日誌中。

`checked_at` 是回答所依據的檢查時間。伺服器每個程序每分鐘最多執行一次資料庫檢查，並重複使用結果，所以輪詢間隔短於 60 秒的監控只會看到相同的 `checked_at`，沒有任何好處。有多個程序時，每個程序各自執行檢查。

設定了 [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) 時，這三個路徑即使沒有該標頭也會回答，所以它們無法顯示 CDN 遺失了標頭。如果監控是你發現問題的第一個訊號，也請透過 CDN 監控一個頁面，例如 `https://<host>/about`。

<!-- source: keep-the-monitoring-cheap 29d1da3d6dc2 -->
## 壓低監控的成本

- **可縮減到零的資料庫**。如果監控的間隔短於資料庫停止前的等待時間，資料庫就永遠不會停止。Neon 會在沒有活動 5 分鐘後暫停運算資源；Free 方案無法關閉這項功能（2026-10-25 確認，[Neon: Scale to zero](https://neon.com/docs/introduction/scale-to-zero)）。依經驗法則，5 分鐘以上的間隔可以讓閒置的資料庫在兩次檢查之間停止，而每次檢查會喚醒它，時間為檢查本身加上閒置等待時間。參與聯邦的伺服器全天候都會收到其他伺服器的傳送，所以它的資料庫一天中大部分時間本來就是喚醒的；額外的成本只限於安靜的時段。
- **如果最在意閒置成本，就選擇更便宜的分工**。以短間隔監控 `/healthz`（它從不存取資料庫），並以長間隔（15 分鐘以上）監控會存取資料庫的 `/api/v1/instance/health`。`/healthz` 在維護模式時也會回答 `200`，所以請比對內文中的 `"status":"ok"`。
- **請求本身很少**。每 5 分鐘檢查一次，每月約 8,640 次請求；每分鐘一次約 43,200 次。這在 Cloud Run 的免費額度之內（每月 200 萬次請求、180,000 vCPU 秒和 360,000 GiB 秒；時間以 100 毫秒為單位無條件進位；2026-10-23 確認，[Cloud Run 定價](https://cloud.google.com/run/pricing)）。

<!-- source: telling-the-maintenance-mode-from-an-outage 15833e5da3fc -->
## 區分維護模式與服務中斷

在維護模式期間（[維護指南](maintenance.md)），健康檢查端點會回答 `503`，大多數監控會把它當成服務中斷。光靠狀態碼無法區分兩者，所以請依內文判斷：

- 把內文中的 `"status":"ok"` 視為正常，其他一律視為中斷。關鍵字或 JSON 值監控就是這樣運作的。
- 為了避免在計畫性維護期間收到警示，如果服務有維護時段功能，請設定監控的維護時段（請參閱下表），或在開始前手動暫停監控。
- UptimeRobot 的 Free 方案沒有維護時段：請暫停監控，或接受警示。

<!-- source: setting-up-the-monitors e91307d78a94 -->
## 設定監控

以下服務檢查的都是同一件事：當 `GET https://<your-host>/api/v1/instance/health` 的回答是 `200` 且內文包含 `"status":"ok"` 時即為正常。

| 服務 | 監控類型 | 正常的條件 | 維護時段 | 確認日期 |
| --- | --- | --- | --- | --- |
| UptimeRobot | Keyword | 內文包含 `"status":"ok"` | Free 方案沒有（Solo 以上才有） | 2026-10-25，[定價](https://uptimerobot.com/pricing.md) |
| Better Stack | HTTP status 或 Keyword | `2xx`，或內文包含 `"status":"ok"` | 未確認 | 2026-10-25 |
| Gatus | Endpoint conditions | `[STATUS] == 200` 且 `[BODY].status == ok` | 設定中的 `maintenance` | 2026-10-25，[README](https://github.com/TwiN/gatus) |
| Uptime Kuma | HTTP(s) - Keyword 或 JSON Query | 內文包含 `"status":"ok"`，或 `status` 為 `ok` | 內建 | 2026-10-23，[README](https://github.com/louislam/uptime-kuma) |
| Upptime | `.upptimerc.yml` 中的 `sites` | `200` 且內文包含 `"status":"ok"` | 無 | 2026-10-25，[Configuration](https://upptime.js.org/docs/configuration/) |

<!-- source: uptimerobot b2350308dfab -->
### UptimeRobot

建立類型為 **Keyword** 的監控：URL 為 `https://<your-host>/api/v1/instance/health`，關鍵字為 `"status":"ok"`，並設定在關鍵字 **不** 存在時發出警示。Free 方案允許 50 個監控，間隔為 5 分鐘，並包含關鍵字監控。「Custom headers & statuses」和維護時段從 Solo 方案（付費，60 秒間隔）開始提供（2026-10-25 確認，[定價](https://uptimerobot.com/pricing.md)）。由於檢查的是內文，這裡用關鍵字監控就夠了。

<!-- source: better-stack 68d7ade20dca -->
### Better Stack

有兩種方式可用（2026-10-25 確認，[Uptime monitor](https://betterstack.com/docs/uptime/uptime-monitor/)、[Keyword monitor](https://betterstack.com/docs/uptime/keyword-monitor/)）：

- **HTTP status** 監控：它把 `2xx` 的回答視為正常，所以 `200` 是正常，而 `503`（無法使用或維護）會建立事件。
- **Keyword** 監控：「Alert us when」設為「URL doesn't contain keyword」，關鍵字為 `"status":"ok"`。比對不區分大小寫。

Free 方案的檢查頻率和維護時段尚未確認：官方頁面沒有說明 Free 方案的這些項目。免費額度的數量（監控和心跳合計 10 個、一個狀態頁面）取自[定價頁面](https://betterstack.com/pricing)，2026-10-23 確認。

<!-- source: gatus 2aa06e9f2812 -->
### Gatus

在 `config.yaml` 中（預設間隔為 `60s`；2026-10-25 確認，[README](https://github.com/TwiN/gatus)）：

```yaml
endpoints:
  - name: my
    url: "https://<your-host>/api/v1/instance/health"
    interval: 5m
    conditions:
      - "[STATUS] == 200"
      - "[BODY].status == ok"
```

Gatus 有一個頂層的 `maintenance` 區段，用於計畫性作業（`start` 以 `hh:mm` 表示、`duration`、`timezone` 預設為 `UTC`、`every` 為星期幾的清單，空白表示每天）。README 只說明它用於在維護時段避免警示；請在你使用的版本上確認實際行為。

<!-- source: uptime-kuma 7dd44fb38cab -->
### Uptime Kuma

新增類型為 **HTTP(s) - Keyword** 的監控並以 `"status":"ok"` 為關鍵字，或新增 **HTTP(s) - Json Query** 監控，查詢 `status`，預期值為 `ok`。接受的狀態碼預設為 `200-299`，所以維護時的 `503` 和中斷時的 `503` 都會判定為中斷。Uptime Kuma 有用於設定監控時段的維護功能（[wiki](https://github.com/louislam/uptime-kuma/wiki/Monitor-Types) 有列出；詳細內容尚未確認）。監控類型已於 2026-10-23 在原始碼中確認（[monitor.js](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/server/model/monitor.js#L478-L720)、[EditMonitor.vue](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/src/pages/EditMonitor.vue#L3349)）。

<!-- source: upptime 90c96c946d06 -->
### Upptime

在 `.upptimerc.yml` 中（2026-10-25 確認，[Configuration](https://upptime.js.org/docs/configuration/)）：

```yaml
sites:
  - name: My server
    url: https://<your-host>/api/v1/instance/health
    __dangerous__body_down_if_text_missing: '"status":"ok"'
```

預設 `20x` 和 `30x` 都算正常；設定 `expectedStatusCodes: [200]` 可以只接受 `200`。檢查會依所有網站共用的排程從 GitHub Actions 執行（`workflowSchedule.uptime`，預設每 5 分鐘一次，這也是 GitHub Actions 允許的最短間隔）。這個選項的名稱表示不建議使用，但它是檢查內文的方法。

<!-- source: a-status-page 64712ed6d8a5 -->
## 狀態頁面

把 `status_page_url`（`MY_STATUS_PAGE_URL`，一個 `https:` URL；請參閱[設定參考](../reference/configuration.md#status_page_url)）設為你的監控服務的公開頁面。伺服器會在 `GET /api/v1/instance`、它產生的登入頁面和錯誤頁面、網頁用戶端（包括伺服器沒有回應時），以及維護頁面中顯示這個連結，讓無法連線到伺服器的使用者能找到原因。它是從設定讀取，而不是從資料庫讀取，所以資料庫停止時也能運作。變更會在重新啟動後生效。

頁面本身的選項（這台伺服器不會匯入狀態頁面的狀態，所以公告也請以全站通知發布）：

- 你所選監控服務的代管狀態頁面（UptimeRobot 的 Free 方案有一個無法自訂的狀態頁面；Better Stack 的免費額度有一個）、Gatus 和 Uptime Kuma 自己提供的頁面，或 Upptime 在 GitHub Pages 上的靜態網站。
- Atlassian Statuspage，它是一個發布資訊的地方，而不是監控服務。元件有五種狀態之一：Operational、Under maintenance、Degraded performance、Partial outage、Major outage（API 值為 `operational`、`under_maintenance`、`degraded_performance`、`partial_outage`、`major_outage`；2026-10-23 確認，[What is a component?](https://support.atlassian.com/statuspage/docs/what-is-a-component/)、[Statuspage API](https://developer.statuspage.io/)）。如何根據監控的警示自動變更元件的狀態尚未確認（這需要在實際帳號上設定各服務專屬的整合）。
