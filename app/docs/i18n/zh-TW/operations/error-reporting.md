<!-- source: error-reporting 1ccf0c1c98a4 -->
# 錯誤回報

本頁說明伺服器如何告訴你它自身的錯誤與故障、在各平台上到哪裡閱讀回報，以及如何把回報送到錯誤追蹤服務。指標告訴你「有地方出了問題」；錯誤回報則告訴你「什麼失敗了」，並附上堆疊追蹤。[設定參考文件](../reference/configuration.md)列出了所有設定。

不論你如何設定，每一份回報都會經過以下三件事：

1. 一行附帶堆疊追蹤的 `error report` 會寫到標準錯誤輸出（[日誌行](#the-log-line)）。
2. 回報會在資料庫中被計入一個錯誤群組。管理畫面會顯示這些群組，擁有 `manage_errors` 權限的人會收到新錯誤和再次發生的錯誤的通知（[錯誤群組與通知](#error-groups-and-notifications)）。
3. 如果你設定了 `errors_exporter`，回報也會送到外部服務（[把回報送到服務](#sending-reports-to-a-service)）。

除非你設定 `errors_exporter`，否則不會有任何資料離開伺服器。

<!-- source: what-is-reported 8e5ac9d14c36 -->
## 會回報哪些錯誤

| 會回報 | 不會回報（由指標和其他日誌行涵蓋） |
| --- | --- |
| HTTP 處理常式、任務或背景作業中的 panic | 4xx 回應（錯誤的輸入、授權、速率限制）以及維護模式 |
| 用戶端 API、管理 API、登入頁面、ActivityPub、OAuth 或媒體的 5xx 回應背後的錯誤 | 用戶端已離開的請求 |
| 最終失敗（`dead`）的任務 | 任務的重試。用完重試次數時才會回報 |
| 啟動失敗：設定無效，或無法建立資料庫或轉接器 | 失敗的就緒檢查 |
| 網頁用戶端未處理、由瀏覽器送到伺服器的錯誤 | 由其他伺服器造成的失敗：傳送的重試與放棄、被拒絕的簽章、失敗的遠端擷取 |
| | 瀏覽器自身的雜訊：網路中斷、瀏覽器擴充功能造成的錯誤，以及因新部署而被移除的 chunk |

外部服務（郵件、儲存空間、付款、語音）的故障，只有在導致任務最終失敗或請求回應 5xx 時才會回報。暫時性的故障則改為顯示在 `my.adapter.*` 指標中（[執行伺服器](running.md#metrics)）。

背景作業中的 panic 在回報之後會讓處理程序停止，因此伺服器永遠不會在損壞的狀態下繼續執行。你的平台會重新啟動它。

<!-- source: the-log-line 6b570d0bfd09 -->
## 日誌行

這一行的 `msg` = `error report`，`level` 為 `ERROR`（當故障導致處理程序停止時，`severity` 為 `CRITICAL`）。它的欄位，以及清除敏感資訊對它們的影響，請參閱[執行伺服器](running.md#error-reports)。簡單來說：

- 同一個錯誤每次發生時，`my.error.fingerprint` 都相同。請依它搜尋或分組。
- `my.error.where` 說明錯誤發生的位置，例如 `client_api GET /api/v1/statuses/{id}`。
- `request_id`、`job_id`、`trace_id` 和 `span_id` 可以找出同一個請求或任務的其他日誌行。
- `message` 位於這一行的最後：錯誤類型與訊息、一個換行，以及 Go 的堆疊追蹤。

其他日誌行都不帶堆疊追蹤。

<!-- source: google-cloud-cloud-run 925c99683713 -->
### Google Cloud（Cloud Run）

Cloud Error Reporting 不需任何設定就能讀取這一行，並在主控台的 Error Reporting 下列出錯誤。它是免費的；你只需支付 Cloud Logging 的費用。

- 它依錯誤類型和堆疊追蹤最上方的幾個框架來分組。它無法採用伺服器的指紋，因此 Error Reporting 中的群組可能與管理畫面中的群組不同。
- 它會保留群組 30 天，並在出現新錯誤以及已解決的錯誤再次發生時通知你，每個專案每小時最多 20 則通知。請在 Google Cloud 主控台設定它的通知管道。
- 若要找出某份回報前後發生了什麼事，請在 Cloud Logging 搜尋 `jsonPayload."request_id"="<id>"`，或以 `trace_id` 做同樣的搜尋。

<!-- source: aws 6ca6594afd7e -->
### AWS

CloudWatch Logs 會把這一行當作一般的 JSON 讀取。

- 為日誌群組開啟日誌異常偵測。它會標示從未見過的日誌行模式，並可透過 SNS 發出警報。它只讀取一行的前 1,500 個字元，這就是 `message` 放在最後一個欄位的原因。它不會保存「已解決」的狀態。
- 若要計算回報數量，請建立指標篩選器，例如 `{ $.msg = "error report" }`。不要使用取自 `my.error.fingerprint` 的維度：維度的每個值都會被當作一個獨立的指標計費。`my.error.source` 和 `my.error.kind` 則是安全的。
- 用 Logs Insights 找出前後脈絡：``filter `request_id` = "<id>"``。

<!-- source: self-hosted 90de7a419474 -->
### 自行架設

從容器日誌讀取這一行：`docker compose logs my | jq -c 'select(.msg == "error report")'`。使用 `metrics_exporter = prometheus` 時，警報 `MyErrorReportsIncreasing`（[警報與處理手冊](alerts.md#myerrorreportsincreasing)）會在回報急遽增加時發出警告；它背後的計數器是 `my.errors.reported`。若需要不依賴外部服務的通知，請使用管理畫面和下方的 Webhook。若要使用追蹤服務，請使用 [`errors_exporter`](#sending-reports-to-a-service)。

<!-- source: error-groups-and-notifications 8f740a328309 -->
## 錯誤群組與通知

伺服器會在資料庫中依指紋計算回報數量。它不需要任何設定或額外的服務，寫入量也很少：某個指紋的第一份回報會立即寫入，之後的回報則在記憶體中累加，每分鐘寫入一次。強制終止處理程序可能會遺失最後一分鐘的計數；日誌行則不會因此遺失。超過 90 天的計數每天都會被刪除。如果故障的正是資料庫，回報就只會留在日誌行中。

<!-- source: the-administration-screen 09092782a7c1 -->
### 管理畫面

角色擁有 `manage_errors`（內建的 `admin` 角色擁有它）的人，會在管理選單中看到「錯誤」（`/admin/errors`）。

- 列表會顯示群組，預設只顯示未解決的群組。可以依狀態（未解決、已解決、已靜音、所有狀態）和來源（伺服器、網頁用戶端）篩選。每一列顯示錯誤類型、發生位置、訊息的前幾行、最近 24 小時和 7 天的次數，以及首次和最後一次發生的時間與版本。
- 群組的頁面會顯示最近一次發生的情況（時間、版本、處理程序角色、請求 ID、追蹤 ID、類型、訊息和堆疊追蹤）、最近 90 天每小時的發生次數，以及其狀態變更的歷史。請求 ID 和追蹤 ID 旁有「複製」按鈕：把它們貼到你的日誌搜尋中。
- 「標為已解決」用於你已修正的錯誤。如果它在另一個版本的伺服器上再次發生，群組會重新開啟並再次通知。在已標記為解決的版本上發生的相同錯誤只會被計數，因此舊版本的最後幾個請求不會讓它重新開啟。
- 「靜音」會繼續計數，但不傳送通知或 Webhook 事件。「取消靜音」會重新開啟群組。

同樣的操作也可透過 `GET /api/v1/admin/error_groups` 和 `POST /api/v1/admin/error_groups/{id}/resolve`（`/mute`、`/unmute`）執行。

<!-- source: notifications-and-webhooks 99836c7f02ec -->
### 通知與 Webhook

- 擁有 `manage_errors` 的人，會在出現新的錯誤群組以及錯誤再次發生時收到 `admin_error_group` 通知。整個伺服器每小時最多送達 10 則；該小時其餘的通知會合併成一則，說明還有更多錯誤被回報。他們也可以透過電子郵件接收（[審核](moderation.md#notifications-for-moderators-and-the-pending-counts)）。
- Webhook 事件 `error_group.created` 和 `error_group.regressed` 帶有相同的群組，因此你可以把它們送到 Slack、Discord 或自動化工具（[伺服器 Webhook](webhooks.md)、[Webhook 參考文件](../reference/webhooks.md)）。Webhook 需要 `manage_webhooks`，事件則需要 `manage_errors`。
- 已靜音的群組永遠不會發出通知。

<!-- source: errors-from-the-web-client e5b2f45b62be -->
### 來自網頁用戶端的錯誤

網頁用戶端會把畫面未處理的錯誤送到你自己伺服器上的 `POST /api/v1/client_errors`，因此不會有第三方看到它們，也不需要變更任何內容安全設定。伺服器對所有請求都回應 `204`，並捨棄它不接受的內容：超過 16 KiB 的內文、不含你自己網頁用戶端檔案的堆疊追蹤，以及同一個 IP 位址每分鐘超過 10 份的回報。同一個錯誤在每次頁面瀏覽中只會送出一次，每次頁面瀏覽最多 20 份。沒有登入工作階段而送出的回報會被計數並記錄，但本身不會通知任何人，因此匿名的程式碼無法塞爆你的收件匣。當已登入者的瀏覽器也回報同一個錯誤時，群組才會發出通知。堆疊追蹤是壓縮後建置版本的內容，不會被還原成原始碼，因此請先閱讀路由和訊息。版本（release）是提供該頁面的伺服器版本。離線時開啟的頁面，或等待 3 秒仍無回應後開啟的頁面，來自瀏覽器保存的副本，它標示的是瀏覽器保存網頁用戶端時的版本：也就是具有相同網頁用戶端的較舊版本。

<!-- source: sending-reports-to-a-service eb5f208ca8fc -->
## 把回報送到服務

設定 `errors_exporter`，就能把每份回報也送到錯誤追蹤服務。日誌行和資料庫中的群組仍然保持運作。每份回報只會送出一次，永遠不會重試，也不會有任何東西等待該服務。

| 值 | 傳送到 |
| --- | --- |
| `none`（預設） | 不傳送 |
| `sentry` | 位於 `errors_sentry_dsn` 的任何 Sentry 相容服務 |
| `otlp` | OpenTelemetry collector 或後端，以日誌記錄的形式 |

`errors_environment`（預設為 `production`）會在每份回報上標示部署環境的名稱，例如 `staging`。

兩種匯出器傳送的都是與日誌行相同、已清除敏感資訊的欄位，附帶指紋以及 trace ID 和 span ID，不含任何關於主機的資訊：沒有主機名稱、IP 位址、使用者、請求內文、標頭或 Cookie。

伺服器會限制傳送量，以免服務被大量湧入的回報淹沒：

- 同一個指紋在每個處理程序中每分鐘最多送出一次，所有回報合計每分鐘最多 60 份。其餘的留在日誌和群組中。
- 最多 1,000 份回報可以等待傳送。佇列已滿時，新的回報會被捨棄。伺服器停止時，最多會等待 2 秒讓佇列清空。
- 當服務回應 `429` 或要求放慢速度（`Retry-After`、`X-Sentry-Rate-Limits`）時，伺服器會在該段時間內停止傳送。
- 伺服器永遠不會跟隨服務的重新導向，以免 DSN 金鑰和回報被送到其他地方：重新導向會被計為 `failed`。請使用服務所顯示的 DSN，它本身就指向服務的接收端點。

請留意計數器 `my.errors.exported`：`my.outcome` 為 `success`、`dropped`（超過限制、佇列已滿，或服務要求放慢速度）或 `failed`（無法連線到服務，或服務拒絕了回報）（[執行伺服器](running.md#metrics)）。

<!-- source: sentry-glitchtip-and-datadog 12c935ec3c05 -->
### Sentry、GlitchTip 與 Datadog

```sh
MY_ERRORS_EXPORTER=sentry
MY_ERRORS_SENTRY_DSN_FILE=/run/secrets/sentry_dsn   # or MY_ERRORS_SENTRY_DSN; the DSN is a secret
```

伺服器本身就使用 Sentry 協定，因此不涉及任何 Sentry SDK，也不會讀取任何 `SENTRY_*` 環境變數。它會設定指紋，讓服務依指紋分組，並設定標籤 `service`（`my`）、`my.role`、`my.error.where` 和 `error.fingerprint`。版本（release）是伺服器的版本，環境（environment）是 `errors_environment`。

- **Sentry**（託管或自行架設）：從專案的用戶端金鑰複製 DSN。
- **GlitchTip**：複製專案的 DSN。它是開放原始碼軟體，可以在小型伺服器上執行。請確認你使用的版本針對新問題與再次發生的問題提供了哪些警報規則。
- **Datadog Error Tracking**：使用你 Datadog 站點的 Sentry DSN，`https://<DD_API_KEY>@sentry-intake.<DD_SITE>/1`。Datadog 需要 `service` 標籤，而伺服器一定會設定它。Datadog 會把這些事件存為日誌，並在 Error Tracking 中分組。如果你改用代理程式或轉送器把日誌行送到 Datadog（Error Tracking for Logs），堆疊追蹤會在 `message` 中；請用日誌管線把它對應到 `error.stack`，否則 Datadog 不會把它當作堆疊追蹤。`sentry` 匯出器則不需要這樣的對應。

<!-- source: otlp-backends 778540dabdc8 -->
### OTLP 後端

```sh
MY_ERRORS_EXPORTER=otlp
OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318   # or OTEL_EXPORTER_OTLP_LOGS_ENDPOINT
OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer%20...
```

每份回報都是一筆名為 `exception` 的 OpenTelemetry 日誌記錄，嚴重程度為 `ERROR`（導致處理程序停止時為 `FATAL`），並帶有 `exception.type`、`exception.message`、`exception.stacktrace`、`my.error.fingerprint`、`my.error.where`、`my.error.source`、`my.error.kind`，以及 trace ID 和 span ID。它透過 OTLP/HTTP（protobuf、gzip）傳送，使用與指標和追蹤相同的資源。

後端是否會把這些記錄轉為分組的錯誤，取決於後端：有些後端只根據追蹤資料建立錯誤檢視，而 Datadog 需要 `error.stack`。對 Sentry、GlitchTip 和 Datadog，請使用 `sentry`。當你希望這些記錄與其他日誌放在一起時，請使用 `otlp`。建置標籤 `no_otel` 會排除 `otlp`（[建置標籤](../reference/build-tags.md)）。

<!-- source: what-scrubbing-does-and-does-not-guarantee ccf1b4716bdd -->
## 清除敏感資訊能保證與不能保證的事

清除敏感資訊只在建立回報時執行一次，因此日誌行、資料庫和每個匯出器收到的都只有清除後的文字。

- 在訊息中，URL、電子郵件地址、`@user@host` 帳號代碼、IP 位址、UUID 與 ULID，以及 10 位數以上的數字，會分別被替換為 `<url>`、`<email>`、`<acct>`、`<ip>`、`<id>` 和 `<n>`，訊息也會被截斷為 1 KiB。
- 在堆疊追蹤中，函式引數的值會被替換為 `(...)`，堆疊追蹤會被截斷為 16 KiB。檔案位置會保留，但以原始碼樹為基準的相對路徑表示，因此不會出現建置機器上的路徑。
- 任何回報都不含請求內文、標頭、查詢字串、Cookie、帳號 ID、使用者名稱、IP 位址或 User-Agent。

清除敏感資訊是以模式比對進行的，因此無法保證訊息中不含個人資料。錯誤訊息所引用的名稱、自由文字，或少於 10 位數的數字可能會被保留下來。一般 `http request` 和 `job` 日誌行的 `error` 欄位，不會以這種方式清除。請把日誌和錯誤追蹤服務視為可能保存使用者內容片段的地方，並給予它們與其他日誌相同的存取控制和保存期限。一旦你設定了 `errors_exporter`，該服務的所在地、保存期限和存取規則就會套用到這些回報上。
