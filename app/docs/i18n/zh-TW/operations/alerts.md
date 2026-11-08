<!-- source: alerts-and-runbook 2c201450ab32 -->
# 警報與處理手冊

本頁說明伺服器自身的指標應該在哪些情況向你發出警報、每個警報的緊急程度，以及警報觸發時要檢查和處理什麼。從外部監看伺服器（運作時間監控和狀態頁面）請參閱[從外部監控伺服器](monitoring.md)；也請一併設定，因為停擺的伺服器不會送出任何指標。

<!-- source: getting-the-metrics 58d8055fe44d -->
## 取得指標

設定 `metrics_exporter` 後，伺服器就會匯出它的指標（[設定參考文件](../reference/configuration.md#metrics_exporter)）；[執行指南](running.md)列出了每個指標及其標籤。以下警報是為 Prometheus 撰寫的，名稱也是 Prometheus 看到的名稱：點會變成底線，並加上單位和計數器的後綴（`my.queue.oldest_ready_age` 是 `my_queue_oldest_ready_age_seconds`，`my.delivery.attempts` 是 `my_delivery_attempts_total`）。

| 目標 | 設定 | 警報 |
| --- | --- | --- |
| 自行架設（Docker Compose、Kubernetes、VM） | `metrics_exporter = prometheus` | Prometheus 從 `metrics_listen` 擷取 `GET /metrics`，Alertmanager 負責分派警報。下方的規則可以直接使用 |
| 任何搭配 OpenTelemetry Collector 的目標 | `metrics_exporter = otlp` | Collector 轉送到你選擇的後端。會加上單位和類型後綴的 Prometheus 相容後端（如 Prometheus 自己的 OTLP 接收器預設的行為），會看到相同的名稱 |
| Google Cloud（Cloud Run） | `metrics_exporter = none` | 以運作時間檢查、平台指標和以日誌為基礎的指標建立 Cloud Monitoring 警報政策（請參閱 [Cloud Run](#google-cloud-cloud-run)） |
| AWS（ECS） | `metrics_exporter = emf` | 針對 CloudWatch Logs 擷取出的指標設定 CloudWatch 警示（請參閱 [AWS ECS](#aws-ecs)） |

<!-- source: prometheus-and-alertmanager 8e8dcaa334e9 -->
### Prometheus 與 Alertmanager

規則位於 [`deploy/monitoring/prometheus/alerts.yml`](../../deploy/monitoring/prometheus/alerts.yml)。把這個檔案複製到你的 `prometheus.yml` 旁邊並載入它：

```yaml
rule_files:
  - alerts.yml

scrape_configs:
  - job_name: my          # the rules expect this job name
    static_configs:
      - targets: ["my:9464"]
```

- `metrics_listen` 預設為 `127.0.0.1:9464`，只有同一台主機上的 Prometheus 能連到。在 Docker Compose 或 Kubernetes 中，請設定 `MY_METRICS_LISTEN=0.0.0.0:9464`，並把該連接埠保留在私人網路上：這個監聽端沒有驗證機制。
- 請擷取每一個處理程序（每個 `role = web` 和 `role = worker` 處理程序都有自己的監聽端）。佇列、媒體、資料庫大小、遠端伺服器、帳單匯入和快照的 gauge，在每個處理程序中都帶有相同的全實例值，因此規則會取它們的最大值。
- 每條規則都有 `severity` 標籤：**`page`** 需要立即有人處理（使用者已受到或即將受到影響），**`ticket`** 可以等到上班時間。請在 Alertmanager 中把它們分派給不同的接收者。每條規則的 `runbook_url` 都指向下方對應的章節。
- 這些門檻是小型伺服器的起始值。請觀察一週的數值，再依你的流量調整門檻和 `for` 的持續時間。`MyRemoteMediaCacheOverLimit`、`MyMediaStorageLarge` 和 `MyDatabaseStorageLarge` 帶有你必須依自己的上限設定的大小。
- 在維護模式期間，伺服器不提供指標並回應 `503`，因此 `MyTargetDown` 和 HTTP 警報會觸發。開始之前，請先在 Alertmanager 中為這段時間設定靜音（[維護](maintenance.md#steps)）。

變更規則之後，請用 Prometheus 隨附的 `promtool` 檢查檔案並執行它的單元測試（`alerts.test.yml`）：

```sh
cd deploy/monitoring/prometheus
promtool check rules alerts.yml
promtool test rules alerts.test.yml
```

<!-- source: google-cloud-cloud-run 72c348585d24 -->
### Google Cloud（Cloud Run）

使用 `metrics_exporter = none` 時，請以 Cloud Run 和 Cloud Logging 既有的資料建立警報：

- **HTTP 錯誤與延遲**：服務依回應代碼類別區分的 Cloud Run 請求數，以及請求延遲指標。
- **錯誤**：`error report` 行會進入 Error Reporting，它會將錯誤分組，並可在出現新錯誤和錯誤再次發生時通知你（[錯誤回報](error-reporting.md#google-cloud-cloud-run)）。
- **任務與請求**：伺服器會為每個 HTTP 請求（`msg` = `http request`）和每次任務執行（`msg` = `job`）寫入一行日誌，帶有與指標相同的標籤。請以它們建立以日誌為基礎的指標，例如以 `jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"` 計算失敗任務的計數器，或以 `jsonPayload."my.surface"="client_api"` 篩選 `jsonPayload.duration_seconds` 的分布來衡量延遲。
- **佇列積壓**：使用 `queue = cloudtasks` 時，任務保存在資料庫中，因此任務畫面和任務執行的日誌行會顯示積壓情況；Cloud Tasks 的佇列深度指標計算的是等待傳送的訊號。
- **資料庫**：Cloud SQL 的磁碟使用率和連線指標，或你的資料庫服務商的用量警報。
- **費用**：設有警報門檻的 Cloud Billing 預算（[費用與預算](#cost-and-budget)）。

模組 [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) 會以下表中的名稱把這些建立為政策，並列出它以不同方式涵蓋或無法表達的每個警報。它從未被實際套用過。只有流量才會產生的指標，在縮減到零的服務上會有空缺，因此針對它不存在而設的警報會誤報；能察覺閒置伺服器已停擺的，是它的運作時間檢查。

<!-- source: aws-ecs 609e5bf632ef -->
### AWS ECS

使用 `metrics_exporter = emf` 時，CloudWatch Logs 會把伺服器的日誌行轉換為命名空間 `metrics_emf_namespace` 中的指標，使用以點分隔的名稱（`my.job.duration`、`my.queue.oldest_ready_age`）。請求和任務的持續時間指標來自請求和任務的日誌行，維度則是你在 `metrics_emf_dimensions` 中列出的標籤；若要針對失敗的任務發出警示，請包含 `my.outcome`，若要針對 5xx 發出警示，請包含 `http.response.status_code`。全實例的 gauge（佇列、媒體、資料庫大小、遠端伺服器、帳單匯入、快照）由定期快照任務寫入。Go 執行階段指標（`go.*`）不會以 EMF 寫入；請改用 ECS 服務的 CPU 和記憶體指標。

- **HTTP 錯誤**：Application Load Balancer 的 5xx 和請求數指標。
- **佇列積壓**：由快照任務寫入的 `my.queue.oldest_ready_age`。
- **資料庫**：RDS 或 Aurora 的儲存空間和連線指標，或你的資料庫服務商的用量警報。
- **電子郵件**（`mail = ses`）：CloudWatch 中的 SES 信譽指標（退信率和投訴率）。
- **費用**：設有警報的 AWS Budgets 預算（[費用與預算](#cost-and-budget)）。

模組 [`monitoring-aws`](../../deploy/opentofu/modules/monitoring-aws/README.md) 會以下表中的名稱把這些建立為警示，並附有相同的清單。它從未被實際套用過。

<!-- source: the-alerts d76842ca02b8 -->
## 警報一覽

嚴重程度：**page** = 立即處理，**ticket** = 上班時間處理。規則欄中的「–」表示目前還沒有對應的指標；請依說明檢查。

| 領域 | 警報 | 訊號 | 門檻 | 嚴重程度 | 可能的原因 |
| --- | --- | --- | --- | --- | --- |
| 佇列 | [`MyQueueBacklog`](#myqueuebacklog) | `my_queue_oldest_ready_age_seconds` | 超過 5 分鐘並持續 10 分鐘 | ticket | worker 不足以應付突發流量（熱門貼文、轉送伺服器）、任務緩慢、worker 停止 |
| 佇列 | [`MyQueueStalled`](#myqueuestalled) | `my_queue_oldest_ready_age_seconds` | 超過 30 分鐘並持續 5 分鐘 | page | 沒有 worker 在執行（當機、未部署、只有 `role = web`）、沒有 worker 執行某一種任務類型、worker 卡在資料庫上 |
| 佇列 | [`MyDeadJobsIncreasing`](#mydeadjobsincreasing) | `delta(my_queue_dead[1h])` | 每小時超過 10 個並持續 15 分鐘 | ticket | 任務的錯誤、轉接器設定錯誤（郵件、blob、帳單）、拒絕請求的遠端服務 |
| 佇列 | [`MyMetricsSnapshotStale`](#mymetricssnapshotstale) | `my_metrics_snapshot_age_seconds` | 佇列部分超過 5 分鐘、總計部分超過 1 小時，並持續 15 分鐘 | ticket | 無法連到 KV 儲存、快照的資料庫查詢失敗或逾時 |
| 聯邦 | [`MyDeliveryFailuresHigh`](#mydeliveryfailureshigh) | `my_outcome` 為 `retry` 或 `failed` 的 `my_delivery_attempts_total` | 超過嘗試次數的 50% 並持續 30 分鐘 | ticket | 許多追蹤者所在的伺服器停擺、某個大型伺服器停擺、對外網路問題 |
| 聯邦 | [`MyDeliveryAllFailing`](#mydeliveryallfailing) | 同上 | 超過嘗試次數的 95% 並持續 15 分鐘 | page | 對外網路或 DNS 故障、對外流量被封鎖、伺服器的金鑰或 `host` 錯誤 |
| 聯邦 | [`MyRemoteServersPaused`](#myremoteserverspaused) | `my_federation_instances{my_federation_instance_state="unavailable"}` | 比一天前多 5 個 | ticket | 伺服器關閉或封鎖了本伺服器、本端持續數天的網路問題 |
| 聯邦 | [`MyInboxRejectionsHigh`](#myinboxrejectionshigh) | `my_inbox_activities_total{my_outcome="rejected"}` | 超過 POST 的 25% 並持續 30 分鐘 | ticket | 簽章驗證失敗（時鐘偏差、金鑰擷取失敗）、垃圾訊息發送者或被停權的網域不斷重試、速率限制 |
| 聯邦 | [`MyInboxSignatureFailures`](#myinboxsignaturefailures) | `my_inbox_activities_total{my_inbox_rejection="signature"}` | 超過 POST 的 10% 並持續 30 分鐘 | ticket | 伺服器的時鐘不準、擷取發送者的金鑰失敗、某個伺服器更換了金鑰 |
| 聯邦 | [`MyInboxErrors`](#myinboxerrors) | `my_inbox_activities_total{my_outcome="error"}` | 超過 POST 的 5% 並持續 15 分鐘 | ticket | 資料庫故障、收件匣的錯誤 |
| 錯誤 | [`MyErrorReportsIncreasing`](#myerrorreportsincreasing) | `my_errors_reported_total` | 15 分鐘內超過 10 個且為前一天平均的 3 倍，並持續 15 分鐘 | ticket | 發行版本帶來的新錯誤、相依服務以新的方式失敗、設定損壞 |
| HTTP | [`MyHTTPErrorsHigh`](#myhttperrorshigh) | `http_server_request_duration_seconds_count` 的 5xx | 超過 5% 並持續 10 分鐘 | page | 資料庫或 KV 停擺、有問題的發行版本、維護模式 |
| HTTP | [`MyHTTPErrorsElevated`](#myhttperrorselevated) | 同上 | 超過 1% 並持續 30 分鐘 | ticket | 某一個路由失敗、某個轉接器偶爾失敗 |
| HTTP | [`MyAPILatencyHigh`](#myapilatencyhigh) | 用戶端 API 的 p95（不含串流） | 超過 300 ms 並持續 30 分鐘 | ticket | 資料庫飽和或距離太遠、緩慢的查詢、CPU 不足、冷啟動 |
| 資源 | [`MyTargetDown`](#mytargetdown) | `up{job="my"}` | 0 並持續 5 分鐘 | page | 處理程序停擺、反覆當機重啟或處於維護模式；擷取路徑損壞 |
| 資源 | [`MyNotReady`](#mynotready) | `my_readiness_failures_total` | 任何增加並持續 5 分鐘 | page | 無法連到資料庫，或結構描述未套用 |
| 資源 | [`MyProcessRestarting`](#myprocessrestarting) | `resets(process_uptime_seconds[1h])` | 一小時內重新啟動 3 次 | ticket | 因記憶體不足被終止（OOM）、panic、存活探測失敗 |
| 資源 | [`MyMemoryNearLimit`](#mymemorynearlimit) | `go_memory_used_bytes` / `go_memory_limit_bytes` | 超過 90% 並持續 15 分鐘 | ticket | `GOMEMLIMIT` 對負載而言太低、許多開啟中的串流、記憶體洩漏 |
| 資源 | [`MyDBPoolSaturated`](#mydbpoolsaturated) | `db_client_connection_count{db_client_connection_state="used"}` / `db_client_connection_max` | 超過 90% 並持續 15 分鐘 | ticket | 連線池對流量而言太小、緩慢的查詢佔用連線 |
| 資源 | [`MyDBPoolWaiting`](#mydbpoolwaiting) | `db_client_connection_wait_time_seconds_total`、`db_client_connection_timeouts_total` | 每秒等待超過 0.5 秒，或有任何逾時，並持續 10 分鐘 | ticket | 同上；已達資料庫本身的連線上限 |
| 資源 | [`MyDatabaseStorageLarge`](#mydatabasestoragelarge) | `my_db_size_bytes` | 超過你方案的上限減去餘裕（範例為 8 GiB）並持續 1 小時 | ticket | 貼文和快取的遠端內容增加；是時候升級方案或縮短保存期限了 |
| 資源 | [`MyFileDescriptorsNearLimit`](#myfiledescriptorsnearlimit) | `process_unix_file_descriptor_count` / `my_process_file_descriptor_limit` | 超過 80% 並持續 15 分鐘 | ticket | 許多開啟中的串流和連線、`ulimit -n` 太低 |
| 資源 | [`MyAdapterErrors`](#myadaptererrors) | `my_adapter_operation_duration_seconds_count{my_outcome="error"}` | 超過單一轉接器操作的 5% 並持續 15 分鐘 | ticket | Valkey、blob 目錄或串流後端故障 |
| 資源 | [`MyRemoteMediaCacheOverLimit`](#myremotemediacacheoverlimit) | `my_media_stored_bytes{my_media_origin="remote_cache"}` | 超過 `remote_media_cache_max_bytes` 的 1.2 倍並持續 3 小時 | ticket | 每小時的清理沒有執行或失敗 |
| 資源 | [`MyMediaStorageLarge`](#mymediastoragelarge) | `my_media_stored_bytes` | 超過你規劃的大小（範例為 50 GiB）並持續 1 小時 | ticket | 上傳增加；是時候升級方案或提高預算了 |
| 電子郵件 | [`MyEmailFailing`](#myemailfailing) | 電子郵件任務中 `my_outcome="dead"` 的 `my_job_duration_seconds_count` | 一小時內有任何一個 | ticket | SMTP 或 SES 設定錯誤、帳號被停權或超出配額、地址被永久拒絕 |
| 電子郵件 | [`MyEmailRetrying`](#myemailretrying) | 同上，`my_outcome="retry"` | 超過執行次數的 50% 並持續 15 分鐘 | ticket | 無法連到郵件伺服器或登入被拒、寄送速率限制 |
| 費用 | [`MyBillingImportFailing`](#mybillingimportfailing) | `my_billing_import_failing` | 1 並持續 6 小時 | ticket | 缺少權限、未設定帳單匯出、達到請求上限、服務商無法使用 |
| 費用 | 超出預算 | – | – | – | 流量、儲存空間或設定錯誤導致費用超出規劃（[見下文](#cost-and-budget)） |

<!-- source: where-to-look 9ef1d37c4f05 -->
## 該查看哪裡

以下工具會在下方大多數的處理手冊項目中反覆出現。

- **任務畫面**（「管理」→「任務」，`/admin/jobs`，需要 `manage_jobs`）：依狀態（等待中、執行中、重試中、失敗）和類型區分的數量，以及等待最久的任務；它的**任務**頁面（`/admin/jobs/list`）會列出某一種狀態的任務，每個任務的頁面都有它最後的錯誤和未成功的嘗試。只有保存在資料庫中的佇列（`queue = db` 和 `cloudtasks`）才會列出待處理的任務；失敗的任務和嘗試則會在所有佇列中列出。
- **`my job` 指令**：在擁有伺服器設定的主機上執行（`queue = db` 和 `cloudtasks`）。`my job list` 會列出失敗的任務，最新的在前；`my job retry <id>...` 會把它們放回佇列；`my job delete <id>...` 會捨棄它們。任務可以安全地重新執行。
- **遠端伺服器畫面**（「審核」→「遠端伺服器」，`/moderation/instances`，需要 `manage_domains`）：傳送失敗的伺服器，以及傳送被暫停的伺服器，附有原因和時間。「恢復傳送」會把伺服器重新標示為可用。
- **日誌**：寫到標準錯誤輸出的 JSON 行。每個 HTTP 請求都以一行 `msg` 為 `http request` 的日誌結束，每次任務執行都以一行 `msg` 為 `job` 的日誌結束（成功為 `INFO`，重試為 `WARN`，最終失敗的任務為 `ERROR`，並帶有 `error`、`job_id` 和 `my.job.type`）。請求內的日誌行帶有它的 `request_id`，用戶端會以 `X-Request-Id` 看到它。
  - Docker Compose：`docker compose logs my | jq -c 'select(.msg == "job" and ."my.outcome" == "dead")'`
  - Cloud Logging：`jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"`
  - CloudWatch Logs Insights：``filter msg = "job" and `my.outcome` = "dead"``
- **錯誤回報**：`error report` 行和錯誤畫面（「管理」→「錯誤」，`/admin/errors`，需要 `manage_errors`）（[錯誤回報](error-reporting.md)）。
- **追蹤**，在 `traces_exporter = otlp` 時：日誌行帶有 `trace_id`；在你的追蹤後端開啟它，就能看到緩慢的請求把時間花在哪裡。
- **維護模式**（[維護](maintenance.md)）：對每個請求都回應維護頁面，且不使用任何轉接器。在修復資料庫或 KV 儲存時使用它。

<!-- source: runbook e60e811fcff0 -->
## 處理手冊

<!-- source: myqueuebacklog e625ad2bb49f -->
### MyQueueBacklog

任務（傳送、通知、電子郵件、時間軸分發）等待 worker 超過 5 分鐘。

檢查：

- 任務畫面：哪些類型在排隊，以及是否有許多任務在重試。某一種類型的突發流量（擁有許多追蹤者的貼文之後的 `federation.deliver`、轉送伺服器的活動）會自行消化；請確認等待時間有在下降。
- worker 處理程序（`role = all` 或 `role = worker`）是否在執行，且它們的日誌有顯示 `job` 行。
- 是否同時發生 `MyDBPoolSaturated` 或 `MyAPILatencyHigh`：worker 在等待資料庫。

處理：

- 執行更多 worker 處理程序（`role = worker`），或給既有的 worker 更多 CPU。
- 如果某一種任務類型很慢，請在 `job` 日誌行（`duration_seconds`）中找出它，並查看該類型的錯誤。

<!-- source: myqueuestalled c92f00f06f95 -->
### MyQueueStalled

最舊的就緒任務已等待超過 30 分鐘：沒有任何東西在處理，或幾乎沒有。

檢查：

- 至少有一個以 `role = all` 或 `role = worker` 執行的處理程序，且它沒有反覆當機重啟（`MyProcessRestarting`）。
- worker 的日誌中是否有每個任務都重複出現的錯誤（資料庫、KV、panic）。
- `MyNotReady`：無法連到資料庫的 worker 無法接收任務。
- `max by (my_job_type) (my_queue_oldest_ready_age_seconds)`：如果某一種類型在等待而其他類型沒有，請確認有 worker 在執行它（[依任務類型分開 worker](running.md#splitting-workers-by-job-type)）。

處理：

- 啟動或重新啟動 worker。停止的 worker 所持有的任務，會在租約到期後回到佇列；它們可以安全地重新執行。
- 如果原因是資料庫，請依照 [MyNotReady](#mynotready) 處理。

<!-- source: mydeadjobsincreasing b8e928621ac7 -->
### MyDeadJobsIncreasing

每小時有超過 10 個任務最終失敗：它們用完了重試次數，或任務拒絕再次執行。

檢查：

- 篩選為「失敗」的任務畫面：類型和最後的錯誤。任務的嘗試記錄會顯示過程中的每個錯誤。
- `my.outcome` = `dead` 的 `job` 日誌行中的相同錯誤。

處理：

- 設定錯誤（郵件、blob、帳單匯入、Webhook 目的地）：修正後，從任務畫面（「再次執行」）或以 `my job retry <id>...` 重新執行這些任務。
- 對已經消失的伺服器的傳送，會在約 4 天的重試後最終失敗；這是預期中的情況，不需要處理。
- 沒有人重試的失敗任務，會在 `dead_job_retention`（預設 30 天）後被移除。你不打算重試的任務，請以 `my job delete` 刪除。
- 看起來像程式錯誤的錯誤：請附上任務類型和錯誤回報它（其中不含個人資料）。

<!-- source: mymetricssnapshotstale c74fe15e6934 -->
### MyMetricsSnapshotStale

佇列和儲存空間的 gauge 來自處理程序之間透過 KV 儲存共用的一份快照。這份快照已經過時，因此佇列和儲存空間的警報看到的是舊的值，可能不會發出警報。

檢查：

- 日誌中與指標快照或 KV 儲存相關的錯誤。
- `kv.valkey` 或 `kv.db` 的 `MyAdapterErrors`，以及資料庫的健康狀態。

處理：

- 修復 KV 儲存或資料庫連線。快照會在下次收集時重新計算；定期快照任務也會每 15 分鐘執行一次。

<!-- source: mydeliveryfailureshigh 709985dfbf2b -->
### MyDeliveryFailuresHigh

對其他伺服器的傳送嘗試有超過一半失敗。一定程度的失敗是正常的：伺服器會消失，而本伺服器會持續重試約 4 天。

檢查：

- 遠端伺服器畫面：失敗是集中在少數伺服器（它們停擺了；不需要處理），還是分散在許多伺服器（懷疑是本伺服器的問題）。
- 依 `error_type` 區分的 `http_client_request_duration_seconds_count`：逾時、DNS 錯誤，或 `forbidden_destination`（SSRF 防護拒絕了目的地）。

處理：

- 只在少數伺服器上失敗：不需要處理。在 7 個不同的日子失敗後，對該伺服器的傳送會被暫停，並每天探測該伺服器以便恢復傳送。
- 到處都失敗：請依照 [MyDeliveryAllFailing](#mydeliveryallfailing) 處理。

<!-- source: mydeliveryallfailing c81fcf407cb9 -->
### MyDeliveryAllFailing

幾乎所有傳送都失敗。這幾乎總是本伺服器這一端的問題。

檢查：

- 從伺服器的主機或容器確認對外 HTTPS 是否正常（DNS、對外防火牆、NAT、代理）。
- `federation.deliver` 的 `job` 日誌行中的錯誤：TLS 錯誤、逾時，或來自許多伺服器的 `401`（它們無法驗證本伺服器的簽章）。
- 在變更網域或金鑰之後：伺服器一旦開始聯邦，`host` 就必須保持不變，KEK 也必須是封存金鑰時所用的那一個（[KEK 管理](key-management.md)）。

處理：

- 修復對外網路。失敗的嘗試會以退避方式重試（1 分鐘、5 分鐘、30 分鐘、2 小時，依此類推），因此傳送會自行追上進度。
- 如果傳送用完了重試次數，請從任務畫面重新執行失敗的 `federation.deliver` 任務。
- 如果遠端伺服器在故障期間被暫停，它們會在下一次成功的傳送、簽章請求或每日探測時恢復；你也可以在遠端伺服器畫面按下「恢復傳送」。

<!-- source: myremoteserverspaused 17c3ec51fb53 -->
### MyRemoteServersPaused

過去一天內，對 5 個以上伺服器的傳送被暫停。對某個伺服器的傳送在 7 個不同的日子失敗後，該伺服器就會被暫停，因此每週有幾次暫停是正常的：伺服器會關閉。同時大量暫停，代表許多伺服器在同一時間失敗了好幾天。`my_federation_instance_state` = `available` 的 `my_federation_instances` 會計算仍在傳送、但曾在某天失敗的伺服器；那裡的數字跳升會出現在暫停之前。

這個指標沒有區分個別伺服器的標籤（標籤永遠不會帶有遠端網域）。檢查：

- 遠端伺服器畫面：「已暫停」分頁會列出被暫停的伺服器，以及暫停的時間和原因。看看它們是否有共通點（同一個託管服務商、同一種軟體），還是分散各處。
- 過去一週的 `MyDeliveryFailuresHigh` 和 `MyDeliveryAllFailing`：本端持續數天的網路問題會讓許多伺服器被暫停。

處理：

- 已經消失的伺服器：不需要處理。被暫停的伺服器每天都會被探測，有回應時會自行恢復。
- 本端發生問題之後：修復它，然後為你在意的伺服器按下「恢復傳送」，或等待每日探測。
- 營運者表示封鎖了本伺服器的伺服器：本端做什麼都沒有幫助。若要刻意停止與某個伺服器聯邦，請使用網域處置（「審核」→「網域處置」）。

<!-- source: myinboxrejectionshigh ee5a244ccf12 -->
### MyInboxRejectionsHigh

其他伺服器 POST 到收件匣的活動中，有超過四分之一被拒絕。

檢查：

- 依 `my_inbox_rejection` 區分的 `my_inbox_activities_total{my_outcome="rejected"}`：`signature`（請依照 [MyInboxSignatureFailures](#myinboxsignaturefailures) 處理）、`suspended`（被停權的網域不斷重試）、`federation_off`（聯邦已關閉）、`rate_limited`（某個用戶端 IP 或伺服器超過速率限制），或 `invalid`（沒有有效 `id`、`type`、`actor` 或 `object` 的活動）。
- 收件匣路由的 `http request` 日誌行（`my.surface` = `activitypub`）依 `http.response.status_code`（`401`、`403`、`429`、`400`）帶有相同的分類以及用戶端的位址，需要知道請求來自哪裡時可以使用。

處理：

- 來自被停權網域的拒絕，或聯邦關閉時的拒絕，不需要處理：它們是依預期被拒絕的。
- 來自某個伺服器的請求受到速率限制：垃圾訊息發送者或突發流量；如果持續發生，請將該網域停權。
- 來自某個伺服器的無效活動：該伺服器的軟體傳送了本伺服器無法讀取的內容；請附上該伺服器的軟體和版本回報它。

<!-- source: myinboxsignaturefailures 85aa65f3247c -->
### MyInboxSignatureFailures

其他伺服器 POST 到收件匣的活動中，有超過 10% 因簽章或完整性證明缺少或無法驗證，或因無法擷取發送者的金鑰而被拒絕。

檢查：

- 伺服器的時鐘：超出時間範圍的簽章會被拒絕。請檢查主機或容器執行環境的 NTP。
- 擷取發送者的金鑰：`MyDeliveryAllFailing`，以及依 `error_type` 區分的 `http_client_request_duration_seconds_count{my_http_purpose="json"}`（對外網路、DNS、SSRF 防護）。
- 拒絕是來自許多伺服器還是單一伺服器（狀態為 `401` 的 `http request` 行）：某個更換了金鑰、或以本伺服器不接受的方式簽章的伺服器，只會影響該伺服器本身。

處理：

- 修正時鐘或對外網路。其他伺服器會重試它們的傳送，因此只要在一天左右內修好，就不會遺失任何東西。
- 單一伺服器失敗：請回報給該伺服器的營運者，或附上該伺服器的軟體和版本回報給本專案。

<!-- source: myinboxerrors e3cf874c925e -->
### MyInboxErrors

收件匣對超過 5% 的活動回應 `500`。發送者會重試，但活動會延遲。

**檢查**收件匣路由的 `http request` 和錯誤日誌行中的錯誤，以及資料庫的健康狀態（`MyNotReady`、`MyDBPoolWaiting`）。

**處理**：修復資料庫，或把錯誤當作程式錯誤回報。不需要重新處理任何東西：發送者會重試。

<!-- source: myerrorreportsincreasing deed4980cc8c -->
### MyErrorReportsIncreasing

伺服器回報錯誤（panic、5xx 背後的錯誤、失敗的任務）的頻率超過平常的三倍。

檢查：

- 過去一小時的 `error report` 行，依 `my.error.fingerprint` 分組：最常出現的錯誤、它們的 `my.error.where`，以及 `message` 的標頭行。在 Cloud Run 上，Google Cloud Error Reporting 會顯示相同的群組。
- 是否有發行版本、設定變更或失敗的相依服務（`MyNotReady`、`MyAdapterErrors`）在同一時間開始。
- 帶有相同 `request_id` 或 `job_id` 的請求或任務日誌行。

處理：

- 相依服務故障或設定錯誤：修正它；之後可以從任務畫面重新執行失敗的任務。
- 有問題的發行版本：回復到先前的映像檔。
- 程式錯誤：請附上該行的標頭和堆疊追蹤回報它。兩者都已清除個人資料。

<!-- source: myhttperrorshigh dd55e9ba0dfd -->
### MyHTTPErrorsHigh

所有請求中有超過 5% 回應 5xx。使用者會看到錯誤。

檢查：

- 你是否處於維護模式（預期會回應 `503`；下次請為警報設定靜音）。
- `MyNotReady`（資料庫）、`MyAdapterErrors`（KV、blob、串流），以及最近的發行版本。
- 依 `http.route` 區分、`http.response.status_code` ≥ 500 的 `http request` 行，以及帶有相同 `request_id` 的錯誤行。

處理：

- 修復故障的相依服務。如果資料庫需要維修，請切換到維護模式，讓使用者看到清楚的頁面而不是錯誤。
- 有問題的發行版本：回復到先前的映像檔。
- 如果持續發生，請告知使用者（[向使用者說明事故](#telling-users-about-an-incident)）。

<!-- source: myhttperrorselevated 34d53d12ea4d -->
### MyHTTPErrorsElevated

在半小時內有超過 1% 的請求回應 5xx。通常是某一個路由或某一個轉接器的問題。

**檢查**依 `http.route` 區分的 5xx `http request` 行，以及這些請求的錯誤行。**處理**：修復相依服務，或把錯誤當作程式錯誤回報。

<!-- source: myapilatencyhigh 81b66aee9099 -->
### MyAPILatencyHigh

用戶端 API 的第 95 百分位數超過 300 ms（這是快取已熱的情況下，20 則貼文的首頁時間軸頁面的目標）。串流端點不計入。

檢查：

- 依 `http_route` 區分的 `http_server_request_duration_seconds`：是某一個路由慢，還是全部都慢。
- 資料庫：`MyDBPoolWaiting`、資料庫的 CPU 和慢查詢日誌、伺服器與資料庫之間的網路距離。
- 處理程序：CPU 節流、`MyMemoryNearLimit`（記憶體壓力下的 GC）、`go_schedule_duration_seconds`（等待 CPU 的 goroutine）。
- 若已啟用，查看緩慢請求的追蹤。

處理：

- 給資料庫或伺服器更多資源，或讓它們的位置更接近。
- 在流量少的伺服器上，縮減到零之後的冷啟動會拉高百分位數；這是預期中的情況。

<!-- source: mytargetdown e0308f141452 -->
### MyTargetDown

Prometheus 無法擷取某個處理程序。

檢查：

- 處理程序是否在執行：`docker compose ps`、`kubectl get pods` 或平台的主控台。它最後的日誌行會說明它停止的原因。
- 它是否處於維護模式（不提供指標；這是預期中的情況）。
- 是否設定了 `metrics_exporter = prometheus`，且 Prometheus 能連到 `metrics_listen`。

處理：

- 重新啟動處理程序。如果它無法啟動，日誌會說明原因：設定錯誤（`my config check` 會顯示）、無法連到資料庫、結構描述未套用。

<!-- source: mynotready 1092418fbba2 -->
### MyNotReady

就緒檢查失敗：無法連到資料庫，或它的資料表與這個版本不相符。負載平衡器會停止把流量送到該處理程序，外部健康檢查端點會回報 `unavailable`。

檢查：

- 就緒檢查的日誌行：連線錯誤（網路、認證資訊、資料庫停擺或連線用盡），或缺少的資料表。
- 資料庫服務商的狀態頁面和主控台：它是否正常運作、儲存空間是否用盡、是否正在維護？
- 升級之後：是否在新版本啟動前套用了結構描述（[初始化資料庫](database.md)）？

處理：

- 資料庫停擺或無法連線：恢復它，或進行容錯移轉。在處理期間，維護模式會向使用者顯示清楚的頁面。
- 資料庫儲存空間用盡：升級方案或釋放空間（[MyDatabaseStorageLarge](#mydatabasestoragelarge)）。
- 結構描述未套用：執行相同版本的 `my schema apply`，之後處理程序會自行進入就緒狀態。
- 資料遺失或損毀：從備份還原，並保留封存該備份中金鑰時所用的 KEK（[KEK 管理](key-management.md)）。

<!-- source: myprocessrestarting d96b041c2ff0 -->
### MyProcessRestarting

某個處理程序在一小時內重新啟動了 3 次。

檢查：

- 平台說明的重新啟動原因：`OOMKilled`（記憶體）、存活探測失敗，或結束代碼。
- 每次重新啟動前的最後幾行日誌，查看是否有 panic 或致命的設定錯誤。

處理：

- 記憶體不足：把 `GOMEMLIMIT` 設為容器記憶體上限的約 90%，或給容器更多記憶體（[MyMemoryNearLimit](#mymemorynearlimit)）。
- panic：請附上前後的日誌行回報它。
- 日誌顯示資料庫屬於其他的 `host` 或 `account_domain`：請在設定中使用資料庫記錄的值，或把 `database_url` 指向正確的資料庫。這些值無法變更（[之後無法變更的設定](production-checklist.md#settings-you-cannot-change-later)）。

<!-- source: mymemorynearlimit 438190b311c9 -->
### MyMemoryNearLimit

Go 執行階段使用了超過 `GOMEMLIMIT` 的 90%。接近上限時，垃圾回收器會更頻繁地執行，耗費 CPU 並增加延遲，超過上限時平台可能會終止處理程序。這個警報需要 `GOMEMLIMIT`；沒有它時，請改為觀察容器的記憶體指標。

**檢查** `http_server_active_requests` 和開啟中的串流（`my_stream_connections`），以及在負載持平時記憶體是否持續增加（記憶體洩漏）。

**處理**：提高容器的記憶體並一併提高 `GOMEMLIMIT`，或執行更多處理程序。不隨負載變化而穩定增加的情況，請回報。

<!-- source: mydbpoolsaturated a737e35a86f5 -->
### MyDBPoolSaturated

某個處理程序的資料庫連線有超過 90% 正在使用中。請求很快就會開始等待連線（`MyDBPoolWaiting`）。

**檢查**流量是否增加，或查詢是否變慢（資料庫的慢查詢日誌、`my_adapter_operation_duration_seconds`）。

處理：

- 提高 `database_url` 中的連線池大小（PostgreSQL 為 `pool_max_conns`），並讓所有處理程序的總數保持在資料庫的連線上限之下。
- 如果許多處理程序共用資料庫，請在資料庫前方放置連線集區。

<!-- source: mydbpoolwaiting eacb1d75cd54 -->
### MyDBPoolWaiting

任務正在等待空閒的資料庫連線，或在 [`database_acquire_timeout`](../reference/configuration.md#database_acquire_timeout) 後放棄等待（逾時僅限 PostgreSQL）：請求會回應 `503`，任務則會稍後重新執行。請依照 [MyDBPoolSaturated](#mydbpoolsaturated) 處理；也請確認資料庫本身沒有達到連線上限。

<!-- source: mydatabasestoragelarge edf767115918 -->
### MyDatabaseStorageLarge

資料庫佔用的空間超過你的規劃。`my_db_size_bytes` 是 PostgreSQL 資料庫的大小（`pg_database_size`，也就是服務商的儲存空間上限所計算的值），或 SQLite 檔案的大小（不含預寫日誌）。這是成長，不是故障。請把規則中的門檻設為比資料庫方案的儲存空間上限，或保存 SQLite 檔案的磁碟區上限低一些的值，磁碟區還必須為預寫日誌和備份保留空間。

處理：

- 升級方案或擴大磁碟區。
- 遠端貼文會在實例設定 `remote_status_retention_days` 之後被移除；調低它可以減緩成長。
- 刪除的資料列會釋放兩種資料庫都會重複使用的空間，但大小很少會縮小：PostgreSQL 會保留其資料表的空間，SQLite 則會把空閒頁面保留在檔案中，直到執行 `VACUUM`（在伺服器停止時以 `sqlite3` 執行）。

<!-- source: myfiledescriptorsnearlimit 8a3112c11968 -->
### MyFileDescriptorsNearLimit

處理程序開啟的檔案描述元超過其上限的 80%。超過上限時，新的連線會失敗。

**處理**：提高上限（`ulimit -n`、容器執行環境的 `nofile`），或執行更多處理程序。請檢查 `my_stream_connections` 和 `http_client_open_connections`，看看是什麼佔用了它們。

<!-- source: myadaptererrors fcf1d91a0f01 -->
### MyAdapterErrors

某一個轉接器的操作有超過 5% 失敗：`kv.valkey`（Valkey）、`kv.db`（KV 資料表）、`blob.fs`（媒體目錄）、`queue.db`（佇列資料表），或某個 `stream.<backend>`（處理程序之間的通知）。

**檢查**該轉接器的錯誤行，以及它背後的服務：Valkey 是否正常運作且可連線，媒體磁碟區是否已掛載、可寫入且未滿。

**處理**：修復該服務。失敗的任務操作會被重試；失敗的串流通知只會延遲即時更新。

<!-- source: myremotemediacacheoverlimit dea5d39a7027 -->
### MyRemoteMediaCacheOverLimit

遠端媒體的副本持續超過 `remote_media_cache_max_bytes`。每小時的清理通常會移除已過期和最久未使用的副本。請把規則中的門檻設為你的 `remote_media_cache_max_bytes` 值。

**檢查**任務畫面和日誌中清理任務的失敗，以及 blob 儲存的 `MyAdapterErrors`。

**處理**：排除失敗的原因，或調低快取大小或實例的遠端媒體政策。

<!-- source: mymediastoragelarge ff5e5906636c -->
### MyMediaStorageLarge

媒體檔案佔用的空間超過你的規劃。這是成長，不是故障。請把規則中的門檻設為你的儲存方案或預算所允許的值。

**處理**：升級方案或提高預算，或調低上傳上限（`media_max_image_bytes`、`media_max_av_bytes`）。

<!-- source: myemailfailing f7a0c64c5c60 -->
### MyEmailFailing

過去一小時內有電子郵件任務最終失敗：驗證、密碼重設、密碼通知、通知，或結束服務的電子郵件。正在等待驗證或重設郵件的使用者將無法完成操作。

**檢查**篩選為電子郵件任務類型（以 `_email` 結尾）的任務畫面，以及最後的錯誤。[寄送電子郵件](email.md#look-at-send-failures)列出了這些錯誤及其意義。

處理：

- SMTP 主機、連接埠或認證資訊錯誤，或 SES 帳號或權限的問題：修正後，重新執行失敗的任務。超過連結有效期限的驗證和重設郵件，在重試時會不寄送就完成，因此請這些使用者重新申請。
- 被永久拒絕的地址會被停止寄送；不需要處理。

<!-- source: myemailretrying 40d3ed1a76f5 -->
### MyEmailRetrying

大多數電子郵件寄送都失敗並等待重試：無法連到郵件伺服器、登入被拒，或寄送速率受到限制。在最終失敗之前，交易型郵件的重試會持續約 15 分鐘，通知則約 4 小時。請在它們最終失敗之前，依照 [MyEmailFailing](#myemailfailing) 處理。

<!-- source: cost-and-budget 5a63bdd2754d -->
### 費用與預算

伺服器不會針對費用發出警報。請在最先看到費用的服務商設定帶有警報的預算：

- AWS：AWS Budgets 的費用預算，例如在每月金額的 80% 和 100% 時發出警報，並以這台伺服器的成本分配標籤篩選。
- Google Cloud：專案的 Cloud Billing 預算，警報門檻設為 50%、90% 和 100%。同一個預算的通知也可以提供給費用匯入使用（[匯入雲端使用費用](billing-import.md)）。

匯入的費用和本月至今的估計值會顯示在「管理」→「費用」→「帳單匯入」中。失敗的匯入會記錄在那裡，並由下一次的每日匯入重試；[`MyBillingImportFailing`](#mybillingimportfailing) 會在當月數字過時之前通知你。

<!-- source: mybillingimportfailing da997f400ac4 -->
### MyBillingImportFailing

某個帳單來源的使用費用最新一次匯入失敗，且之後的 6 小時內都沒有成功的匯入。匯入每天執行一次；節流和無法使用的情況會在數小時內重試，其他失敗則要等到隔天的匯入，因此在那之前，費用畫面會顯示較舊的數字。

檢查：

- 「管理」→「費用」→「帳單匯入」：該來源最後一次的匯入及其錯誤。
- 依 `error_type` 區分的 `my_billing_imports_total{my_outcome="failed"}`，或 `msg` 為 `billing import failed` 的 `WARN` 日誌行，它帶有 `source` 和 `error_kind`。[匯入雲端使用費用](billing-import.md)列出了這些種類以及各自的修正方式。

處理：

- 排除原因（權限、帳單匯出、請求上限），然後在「管理」→「費用」→「帳單匯入」中按下「立即匯入」，而不是等到隔天。

沒有 Prometheus 時（使用 `metrics_exporter = none` 的 Cloud Run），請改為針對 `billing import failed` 行建立以日誌為基礎的警報。

<!-- source: telling-users-about-an-incident e8cb0ca13fe8 -->
## 向使用者說明事故

1. **確認**警報，讓其他值班人員知道已經有人在處理。
2. 使用者一受到影響，就**在狀態頁面上發布**（`status_page_url`，請參閱[從外部監控伺服器](monitoring.md#a-status-page)）：什麼故障了、從何時開始，以及你下次何時會再更新。即使這台伺服器無法連線，狀態頁面仍可連線，網頁用戶端也會連結到它。
3. 如果使用者仍能連到伺服器（效能降低，而非停擺），請**發布公告**：在「管理」→「公告」中撰寫文字並發布。公告會顯示在每個人的首頁時間軸上方。如果伺服器必須停止，請改用維護模式並搭配 `maintenance_message`。
4. 即使沒有任何變化，也請在你承諾的時間**更新**兩者。
5. **結束**：在狀態頁面上把事故標示為已解決，撤下公告（或發布簡短的後續說明），並趁記憶猶新時寫下發生了什麼事以及你做了哪些變更。
