<!-- source: deploying-on-google-cloud 03b87618467f -->
# 在 Google Cloud 上部署

本頁說明如何使用 OpenTofu 模組 `mycroblog-gcp` 在 Google Cloud 上執行 `my`：一個 Cloud Run 服務，以及每個角色各自的後端，你可以用該區段的 `kind` 來挑選。每個區段的預設值都是模組的建議，沒有人使用伺服器時不會執行任何執行個體，只有每分鐘喚醒它一次的 tick 例外。伺服器是要花錢的：建議的配置搭配 Neon 上的資料庫，在儲存空間和流量之前每月約 $37，[費用](#costs)列出了每種選擇的金額。

每個 OpenTofu 部署都共通的內容，包括區塊與它的區段、機密、主控台和升級，都在[以 OpenTofu 部署](deploy-opentofu.md)。本頁補充 Google Cloud 所需要的部分。

<!-- source: what-it-creates 7b13c8bbc968 -->
## 建立的資源

`mycroblog-gcp` 的一個區塊會建立以下內容。每一列的第一種 kind 是建議的種類，也是預設值；每個欄位都列在[模組的參考](../../deploy/opentofu/README.md#sections)。

| 角色 | 區段 | 建議 | 其他種類 |
| --- | --- | --- | --- |
| 伺服器 | `service`、`jobs` | 一個 `role = all` 的 Cloud Run 服務、一個遷移任務、需要時才存在的主控台任務、一個服務帳號，以及公開存取權 | |
| 流量 | `ingress` | `load-balancer`：一個帶有 Google 管理憑證的全域外部 Application Load Balancer | `domain-mapping`、`cloudflare`，以及可以與任何一種並用的 Cloudflare 上有快取的媒體主機 |
| 資料庫 | `database` | `external`：你自己執行的 PostgreSQL（例如 Neon）的 URL 機密 | `cloud-sql`：一個執行個體、它的資料庫、使用者和密碼 |
| 任務 | `queue` | `cloud-tasks`：一個 Cloud Tasks 佇列、tick 的 Cloud Scheduler 任務，以及呼叫伺服器的服務帳號 | `db`：任務保存在資料庫中 |
| 媒體 | `media` | `gcs`：一個私人的 Cloud Storage bucket | `r2`、`s3` |
| 快取與速率限制 | `kv` | `db`：放在資料庫中，不需要建立任何東西 | `memorystore`、`external`（Valkey） |
| 即時更新 | `stream` | `pubsub`：一個主題，以及一個讓伺服器管理自己訂閱的自訂角色 | `postgres`、`valkey` |
| 電子郵件 | `mail` | 任何 SMTP 服務，搭配它的密碼機密 | |
| KEK | `kek` | `secret-manager`：以檔案掛載的產生機密 | `kms`：Cloud KMS 中的一把金鑰 |
| 網路 | `network` | `none`：沒有要連接的東西 | `vpc`、`existing` |
| 指標、追蹤、錯誤 | `observability` | Cloud Monitoring、Cloud Trace 和 Error Reporting，它們會讀取 Cloud Run 送給它們的內容。模組會啟用它們的 API | `settings`，例如 Sentry |
| 警示 | `monitoring` | 一個可用性檢查、以日誌為基礎的指標，以及警示政策：[監控與警示](#monitoring-and-alerts) | `null`，不建立 |

模組也會產生 VAPID 金鑰，並建立只有你能填入、目前為空的機密。

`examples/gcp-cloud-run-neon` 是一種開始的方式：provider 加上一個包含所有建議種類的區塊，不論是不是 Neon，都適用於任何外部資料庫。請把你要變更的區段加入它的區塊。

<!-- source: before-you-start 9869afda9a07 -->
## 開始之前

- [首次部署](deploy-opentofu.md#first-deployment)所需要的東西，以 `gcloud` 作為平台的 CLI，並為 provider 執行 `gcloud auth application-default login`。
- 一個啟用計費、只供這個實例使用的 Google Cloud 專案，以及一個在該專案上擁有 Owner 角色的帳號。`stream = pubsub` 的自訂角色會放在專案上，因此它能涵蓋那裡其他任何東西的訂閱。

<!-- source: first-deployment 894b6af2cee9 -->
## 首次部署

請在這裡做前兩個步驟，再依照[首次部署](deploy-opentofu.md#first-deployment)進行，並參考第三步的說明。

<!-- source: 1-enable-the-apis fdd2dc400c20 -->
### 1. 啟用 API

每個部署都需要這些：

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com
```

再加入每個需要 API 的選擇所對應的 API：

| 選擇 | API |
| --- | --- |
| `ingress = load-balancer` | `compute.googleapis.com` |
| `database = cloud-sql` | `sqladmin.googleapis.com` |
| `kek = kms` | `cloudkms.googleapis.com` |
| `network = vpc` 或 `existing` | `compute.googleapis.com`、`servicenetworking.googleapis.com`、`networkconnectivity.googleapis.com` |
| `kv = memorystore` | `memorystore.googleapis.com`、`serviceconsumermanagement.googleapis.com`，以及網路所需的 API |

`translation` 會啟用它自己的 API，`observability` 也是。`billing` 需要什麼，請見[匯入雲端使用費用](billing-import.md)。

<!-- source: 2-prepare-the-database b54e3b01002b -->
### 2. 準備資料庫

- **`external`**：建立資料庫，並保留它的直接連線 URL，以及（如果有的話）它的集區 URL，供 `secrets_to_fill` 使用：[資料庫](#the-database)說明哪一個放在哪裡。模組絕不會建立它，因此它的密碼不會進入 OpenTofu 的 state。
- **`cloud-sql`**：不需要做任何事。模組會建立執行個體，並把它的 URL 連同產生的密碼寫入機密。

<!-- source: 3-deploy 4470affd40e4 -->
### 3. 部署

請依照[首次部署](deploy-opentofu.md#first-deployment)進行。在 Google Cloud 上：

- **變數**：範例中的 `mail_from`、`mail_smtp_host` 和 `mail_smtp_username` 用來設定[電子郵件](email.md)。連接埠是 587，因為 Google Cloud 會封鎖對外的連接埠 25。
- **機密**：KEK 是 `my-kek`，VAPID 金鑰是 `my-vapid-key`，各自都從 `name` 開始，並可用 `gcloud secrets versions access latest --secret=<id> --project="$PROJECT"` 複製它們。KEK 以掛載檔案的形式交給伺服器，由 `MY_KEK_FILE` 指定，其他機密則是環境變數。
- **套用**：遷移任務最先建立，連同它所依賴的資源，其中包括入口，因為伺服器的設定會指名它。服務的啟動探測會等待 `/health/ready`，而它在結構建立之前會回應 `503`。
- **DNS**：加入 `tofu output dns_records` 的記錄，做法如[流量如何到達服務](#how-traffic-reaches-the-service)中每種 kind 所述。在憑證生效之前，`https://{host}` 會因 TLS 錯誤而失敗，因此聯邦和瀏覽器都還無法連到伺服器。其他部分不需要等待它：Cloud Tasks 和 Cloud Scheduler 會呼叫服務預設的 URL，`https://<name>-<project number>.<region>.run.app`。**不要停用預設 URL**，否則任務會停止。Cloud Run 只有在這兩者來自同一個專案、且使用該 URL 時，才會把它們視為內部請求，這就是負載平衡器能對網際網路關閉它的原因。
- **第一位管理員**：連結會留在 Cloud Logging 中，任何能讀取專案日誌的人都可以使用它。

<!-- source: the-console-and-upgrades a40a1df56865 -->
## 主控台與升級

[主控台](deploy-opentofu.md#the-console)和[升級](deploy-opentofu.md#upgrading)會從你執行 apply 的地方，透過 `gcloud` 執行 Cloud Run 任務 `<name>-console` 和 `<name>-migrate` 的一次執行。它們需要可以執行這些任務並讀取其日誌的憑證，也就是 `roles/run.developer` 和 `roles/logging.viewer`，Owner 已包含它們。被中斷的 apply 會讓該次執行繼續留在 Cloud Run 上，下一次 apply 會等它結束。

若要[先檢查 SQL](deploy-opentofu.md#checking-the-sql-first)：

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region="$REGION" --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

<!-- source: the-database 0303fdd046ef -->
## 資料庫

模組建議使用 `external`，它不需要網路，在 Google Cloud 上也不會計費：像 Neon 這種閒置時會停止的資料庫，能讓閒置的伺服器維持低廉。`cloud-sql` 把所有東西都放在同一個專案中，費用固定。在兩者之間搬移資料是[更換後端](deploy-opentofu.md#changing-a-backend)。

<!-- source: an-external-postgresql d6604bfb9e2f -->
### 外部 PostgreSQL

預設的 `database = { kind = "external" }` 可以使用 Cloud Run 連得到的任何 PostgreSQL：Neon、其他託管的 PostgreSQL，或你自己執行的，可以透過網際網路，也可以透過[網路](#networks)使用私人位址。模組會為**直接** URL 建立機密 `my-database-url`，遷移任務和主控台一律使用它：`my schema apply` 會持有工作階段層級的鎖，而交易模式的連線集區不會保持這個鎖。

`database = { kind = "external", pooled = true }` 會加入機密 `my-database-url-pooled`，用於**集區** URL，服務接著會使用它。連線集區適合伺服器，如[透過連線集區](database.md#through-a-connection-pooler)所述。沒有連線集區時，服務也會使用直接 URL，因此 `service.max_instances` 乘以每個執行個體的連線池，必須維持在資料庫的連線上限以下。連線池預設是每個執行個體 4 個連線，而一個執行個體一次接受 80 個請求：請在服務使用的 URL 中以 `pool_max_conns` 調高，例如 `...?sslmode=require&pool_max_conns=8`（[等待資料庫連線](running.md#waiting-for-a-database-connection)）。

**在 Neon 上**，請在靠近你 Cloud Run 區域的區域建立專案。它的直接連線字串使用單純的主機名稱，集區連線字串則使用帶有 `-pooler` 的主機名稱，那是 Neon 的連線集區，適合伺服器：請設定 `pooled = true`。請預期使用 Launch 方案，因為預設的 tick 會讓運算資源保持清醒；Free 方案也可以使用，但有[費用中所述的取捨](#where-care-reduces-it)。

<!-- source: cloud-sql dc9461df8cc2 -->
### Cloud SQL

`database = { kind = "cloud-sql" }` 會建立一個 PostgreSQL 18 執行個體，預設為 `db-f1-micro`，具有每日備份和保留 7 天的時間點復原、它的資料庫和使用者，以及一組既不會進入 plan 也不會進入 state 的產生密碼。`tier`、`disk_size_gb`、`retained_backups` 和 `deletion_protection` 可以變更它。它從未被套用過：請把第一次部署當成它的測試。

- **沒有網路時**，Cloud Run 會透過 Cloud SQL 連接器的 socket 連到它，而執行個體的公開位址不接受其他任何連線。
- **有網路時**，它只有私人位址，`pg_dump` 必須從網路內部執行。
- **它沒有連線集區**：`pooled` 會被拒絕，連線數由 `service.max_instances` 和 `pool_max_conns` 限制。在記憶體只有 0.6 GB 的 `db-f1-micro` 上，請讓連線池保持在較小的值。
- **它永遠不會縮減至零**，所以不論流量多少都會計費，而且 Google 的 SLA 不涵蓋共用核心的方案。

<!-- source: the-other-backends 91ef8e2dea11 -->
## 其他後端

每個後端都以它的建議作為預設值，而[變更 `kind`](deploy-opentofu.md#changing-a-backend) 就能換成另一個。

- **任務**：`cloud-tasks` 會把每個任務推送到可以縮減至零的服務。`queue.tick_schedule`（預設為每分鐘一次）會為定期任務喚醒伺服器和資料庫。`db` 會從一個持續運作的程序輪詢資料庫，因此需要 `service.min_instances` 為 1 以上，以及 `service.cpu_always_allocated = true`。
- **媒體**：`gcs` 會在該區域建立一個私人 bucket，`media.bucket_name`。`r2` 會在 `cloudflare.account_id` 中建立一個 R2 bucket，`s3` 則使用位於它的 `endpoint` 的任何 S3 相容儲存空間；兩者都會從 `secrets_to_fill` 所列的機密讀取存取金鑰。若要與其他實例共用 bucket，`existing_bucket = true` 會使用你的 bucket，而 `prefix`（`tenant-a/`）會作為每個物件名稱的開頭；搭配 `gcs` 時，服務帳號就只能存取它底下的物件，這需要統一的 bucket 層級存取權。
- **快取與速率限制**：`db` 把它們保存在資料庫中。`memorystore` 會建立 Memorystore for Valkey，它需要網路且沒有密碼：網路上的任何東西都能連到它。`external` 會從機密讀取你所執行的 Valkey 的 URL。
- **即時更新**：`pubsub` 能連到每個執行個體。`postgres` 會透過資料庫的直接連線使用 `LISTEN` 和 `NOTIFY`，`valkey` 則使用 `kv` 的 Valkey。
- **KEK**：`secret-manager` 只產生一次，讓你複製。`kms` 把它保存在 Cloud KMS 中，它絕不會離開那裡，因此即使資料庫傾印和設定同時外洩，也不會暴露任何機密。在有資料的實例上變更它是[一次搬移](deploy-opentofu.md#the-kek)。

<!-- source: networks 9d28311e11a6 -->
## 網路

建議的配置不需要網路：外部資料庫、Cloud Tasks、Cloud Storage 和 Pub/Sub 都是以服務自己的身分透過網際網路連線，因此 Cloud Run 不需連接任何東西。

若要使用只有私人位址的 Memorystore、讓 Cloud SQL 有私人位址，或是連到你位於網路上的資料庫，就需要網路。`network = { kind = "vpc" }` 會建立一個 VPC、它的子網路和私人服務存取，而 `network = { kind = "existing", ... }` 會連接到你既有的 VPC，你要以 `network`、`subnetwork`、`private_services_range` 和 `memorystore_connection_policy` 來描述它：

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

服務和任務透過 Direct VPC egress 連接，只有私人範圍會經過該網路，因此網際網路仍走它自己的路徑，不需要 Cloud NAT。搭配 `vpc` 時，`nat = true` 則改為讓所有流量都經過 Cloud NAT，以取得固定的來源位址。網路可能會讓執行個體的啟動延遲一分鐘以上，而會縮減至零的伺服器常常會遇到這種情況。模組所建立的 VPC 從未被套用過。

<!-- source: how-traffic-reaches-the-service 094ddaf201c8 -->
## 流量如何到達服務

這由 `ingress.kind` 決定。模組為正式環境建議使用負載平衡器。網域對應則適合以低成本試用部署。

| | `load-balancer` | `domain-mapping` |
| --- | --- | --- |
| 月費 | 轉送規則約 $18，另加每 GB 的費用。請查看[價格頁面](https://cloud.google.com/vpc/network-pricing)：這個數字無法在那裡得到確認 | 無 |
| 狀態 | Google Cloud 建議的方式 | 預覽版功能，由於延遲問題，Google Cloud 不建議用於正式環境 |
| 區域 | 任何區域 | `asia-northeast1`、`us-central1` 及少數其他區域 |
| DNS | 一筆把 `host` 指向它的位址的 `A` 記錄。請為它關閉你的 DNS 服務商的任何代理 | `dns_records` 所列的記錄 |
| 憑證 | 由 Google 管理，DNS 指向位址後生效，這可能需要一段時間 | 由 Google 管理，最多需要 24 小時 |
| 網域所有權 | DNS 之外不需要其他事 | 執行 apply 的人必須先成為該網域的[已驗證擁有者](https://cloud.google.com/run/docs/mapping-custom-domains) |
| `run.app` URL | 拒絕來自網際網路的請求，因此沒有人能繞過負載平衡器 | 任何人都能連線：網域對應無法加入標頭，因此來源保持開放 |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` 和負載平衡器的位址 | `169.254.0.0/16` |

Cloud Tasks 和 Cloud Scheduler 不經過其中任何一個：它們直接呼叫預設的 `run.app` URL。

<!-- source: cloudflare 35dc1e54d2bb -->
### Cloudflare

`ingress = { kind = "cloudflare" }` 會將 Cloudflare 放在 `host` 前面，並對所有東西繞過快取，而 `ingress.media_host` 則在任何種類旁加上一個有快取的媒體主機。兩者都需要 `cloudflare = { zone_id = "..." }`，provider 會從 `CLOUDFLARE_API_TOKEN` 讀取權杖，而模組會建立記錄，所以 `dns_records` 是空的。它們只在模擬的 provider 上執行過。

- **你的方案決定來源。** Cloud Run 依 `Host` 路由，而 Cloudflare 只在 Enterprise 方案才會覆寫 Host 標頭。在任何其他方案上，模組會為每個主機建立一個網域對應，並有上表所述的取捨，Cloudflare 再連到它。它的憑證能否在代理之後簽發與更新，尚未驗證，所以請從 `cloudflare.proxied = false` 開始。在 Enterprise 方案上，`cloudflare.host_header_override = true` 會改為連到 `run.app` URL，不需要網域對應。
- **標頭會關閉來源。** 請求標頭轉換規則會加入一個由模組產生的值，而伺服器會以 `403` 拒絕沒有該值的請求，但健康檢查和任務自己的 `POST`（它們會檢查自己的權杖）除外。所以沒有人能透過 `run.app` URL 繞過 Cloudflare，也沒有人能選擇速率限制所看到的用戶端位址。
- **在記錄沒有被代理時，沒有東西會加上標頭**，所以伺服器會拒絕除了健康檢查之外的所有東西：為了網域對應的憑證而設定 `proxied = false` 時，請在憑證生效後再開啟代理。
- **這個值會落在 state 中**，如[機密](deploy-opentofu.md#secrets)所述，也會寫入機密 `my-origin-secret`。輪換的做法請見 [CDN 指南](cdn.md#rotate-the-value)。
- **請關閉 Bot Fight Mode**（在儀表板中），也不要開啟「I'm Under Attack」：對 `/ap/`、`/.well-known/` 或 `/api/` 的挑戰會破壞聯邦，因為其他伺服器不是瀏覽器。兩者都涵蓋整個 zone，沒有任何規則能排除它們。模組不會加入任何挑戰，並會關閉 Browser Integrity Check。
- **Email Address Obfuscation 可以維持開啟。** 伺服器自己的頁面會標記它必須保持原樣的地址所在的內容。
- **如果你想讓 `http://` 被重新導向，請在儀表板中開啟 Always Use HTTPS。** 它涵蓋整個 zone，所以模組將它留給你處理。
- **逾時**：當來源 125 秒沒有送出任何東西時，Cloudflare 會回應 `524`。任務不受影響，因為它們不經過 Cloudflare。串流每 30 秒送出一次心跳，低於 900 秒的閒置上限。在被代理的主機上，Cloudflare 是否會緩衝串流，尚未驗證。
- **影片**：Cloudflare 的條款限制在 Free、Pro 和 Business 方案上提供影片，以及不成比例的大量大型檔案，它可能會對內容採取行動。請在你將影片快取在媒體主機上之前，先決定這一點。

**被刪除的檔案仍會留在快取中。** 媒體主機會遵循伺服器的 `Cache-Control`，所以被刪除的本地檔案最長可能會持續可見一年，遠端檔案則為 `remote_media_cache_ttl_seconds`。伺服器不會清除它。當某些東西必須消失時，請手動清除，在儀表板的 **Caching > Configuration > Custom Purge** 下進行，或是：

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

代理 URL 的查詢字串中帶有它的 `sig`，這是被快取內容的一部分，所以請提供完整的 URL。

<!-- source: monitoring-and-alerts eb28e653de36 -->
## 監控與警示

`monitoring` 會加入 `/health/live` 的可用性檢查、以日誌為基礎的指標，以及以[運作手冊](alerts.md)的名稱命名的警示政策，它們會通知 `notification_emails`（範例中的 `alert_emails`）的地址或 `notification_channels` 的通道。這個區段沒有預設值：請指定誰要收到通知，或設定 `monitoring = null` 完全不使用它。它從未被套用過，所以請讓一項檢查失敗來試用一次。檢查的每次執行都會喚醒一個執行個體：`uptime_period` 預設為 15 分鐘，就是執行的頻率。

<!-- source: traces da2e58dc6a3a -->
### 追蹤

Cloud Trace 會顯示 Cloud Run 為每個請求記錄的追蹤，而伺服器的日誌行會帶有它的 `trace_id`，因為模組信任入口（`MY_TRUSTED_PROXIES`）。這些日誌行不會填入 Cloud Logging 用來連結的追蹤欄位（`logging.googleapis.com/trace`），而 Cloud Run 是否一定會取代公開請求的 `traceparent`，尚未驗證。伺服器自己的 span 需要旁邊有一個 OpenTelemetry Collector，而模組不會執行它。

<!-- source: limits 3d4721415f18 -->
## 限制

- **上傳**：Cloud Run 會拒絕超過 32 MiB 的 HTTP/1 請求，而伺服器使用的是 HTTP/1.1。因此模組把 `MY_MEDIA_MAX_AV_BYTES` 設為 30 MiB（預設為 100 MiB），讓較大的上傳得到伺服器自己的回應。Cloudflare 自己的上限（Free 和 Pro 為 100 MB）高於它。
- **串流**：`service.request_timeout_seconds`（預設 15 分鐘）會結束開啟中的串流，用戶端會重新連線。
- **資料庫連線**：Google 限制一個 Cloud Run 執行個體最多對 Cloud SQL 建立 100 個連線。
- **用戶端位址**：速率限制仰賴 `MY_TRUSTED_PROXIES`，而模組會根據入口設定它。我們讀過的文件都沒有說明 Cloud Run 對直接請求會在 `X-Forwarded-For` 加上什麼，或容器看到的對端位址是什麼，因此 `169.254.0.0/16` 是一項假設。負載平衡器有文件記載的形式為 `[<supplied>,]<client-ip>,<lb-ip>`。請檢查最初幾筆請求日誌中的位址：如果用戶端位址是 `169.254` 位址或 Google 的位址，就表示該設定不適用於你的專案。
- **公開存取**：服務會把 `roles/run.invoker` 授予 `allUsers`，而限制網域共用的組織政策會拒絕這麼做。這樣的專案需要設定例外。
- **名稱會保持被佔用**：Cloud SQL 和 Cloud Tasks 會在大約一週內拒絕已刪除的執行個體或佇列的名稱，已刪除的自訂角色也會暫時保留它的 ID，所以在 `destroy` 之後重新建立的部署需要另一個 `name`。

<!-- source: costs ea658d24d79d -->
## 費用

這些數字是讓你了解預期費用以及在哪裡可以透過用心調整來降低費用的參考，而不是部署必須遵守的上限。它們是單一區域的大約定價，取自 2026-10 和 2026-11：請在 [Cloud Run](https://cloud.google.com/run/pricing)、[Cloud SQL](https://cloud.google.com/sql/pricing) 和 [Neon](https://neon.com/pricing) 的價格頁面上確認。每個計費帳戶每月在 180,000 vCPU 秒和 360,000 GiB 秒以內的 Cloud Run 運算是免費的。

每個部署都要付的費用，閒置時約 $0：

| 部分 | 閒置時 | 增加費用的因素 |
| --- | --- | --- |
| Cloud Run | $0：沒有執行個體在執行，CPU 只在請求期間計費 | 每個開啟中的串流都會讓一個執行個體持續計費；見下文 |
| Cloud Tasks | $0 | 100 萬次操作免費，之後每百萬次 $0.40。一個任務約需 2 次操作（加入佇列和投遞） |
| Cloud Scheduler | $0：tick 是一個任務，每個計費帳戶 3 個免費 | 超過後每個任務 $0.10 |
| Pub/Sub | $0：10 GiB 免費 | 只在串流開啟期間產生 |
| Secret Manager | 幾美分 | 少數幾個版本免費，之後每個版本幾美分 |
| Cloud Storage | 每 GiB 每月約 $0.02 | 只有在 `us-east1`、`us-west1` 和 `us-central1` 有 5 GiB 免費。媒體透過 Cloud Run 傳出 |
| Monitoring | 免費額度內 $0 | |

每個選擇會增加的費用：

| 區段 | 選擇 | 每月 |
| --- | --- | --- |
| `ingress` | `load-balancer` | 約 $18，未在價格頁面上得到確認，另加每 GB 的費用 |
| | `domain-mapping` | $0 |
| | `cloudflare` | 你的 Cloudflare 方案，以及來源：搭配網域對應為 $0 |
| `database` | Neon Launch 方案上的 `external` | 使用預設 tick 時約 $19，另加每 GB-月 $0.35 |
| | Neon Free 方案上的 `external` | $0，在它的限制內：[見下文](#where-care-reduces-it) |
| | 其他地方的 `external` | 該資料庫的費用 |
| | `cloud-sql` | `db-f1-micro` 和 `us-central1` 的 10 GiB 約 $9.40，另加備份，不論流量多少 |
| `queue` | `db` | 一個全天候計費的執行個體，CPU 一直處於配置狀態 |
| `kv` | `memorystore` | 最小的節點約 $23，不論流量多少 |
| `kek` | `kms` | 幾美分，用於金鑰和它的操作 |
| `network` | 搭配 `nat = true` 的 `vpc` | Cloud NAT，依小時和 GB 計費 |

建議的配置（負載平衡器和 Neon Launch 方案上的 Neon）每月約 $37；搭配 Cloud SQL 則約 $27。

**Tick 會讓 Neon 保持清醒**。每次 tick 都會讀寫資料庫，而 Neon 只有在 5 分鐘沒有活動後才會停止運算資源，因此使用預設的 `* * * * *` 時，運算資源永遠不會停止。這相當於每月 730 小時，以 0.25 CU 計算為 182.5 CU 小時，也就是上面的 $19。tick 維持每分鐘一次，是為了讓定期任務準時執行，並讓遺失的喚醒訊號能很快恢復。不論 tick 為何，Cloud SQL 的計費都相同。

**開啟中的串流會讓執行個體持續計費**。在以請求為基礎的計費方式下，執行個體在處理任何請求時都會計費，而即時更新就是一個很長的請求。一個整月都保持串流開啟的用戶端，就會讓一個執行個體全天候計費：以 1 vCPU 和 512 MiB 計算，扣除免費額度後每月約 $60。更多的串流會共用該執行個體，上限為 `service.concurrency`。如果串流大部分時間都保持開啟，請與以執行個體為基礎的計費方式（`service.cpu_always_allocated`），或小型 VM 搭配 [Docker Compose](deploy-docker-compose.md) 比較。

<!-- source: where-care-reduces-it 427be9d46d32 -->
### 用心調整可以降低費用的地方

- **使用網域對應**取代負載平衡器，可以省下 $18，但有[它的取捨](#how-traffic-reaches-the-service)。它適合用來試用部署。
- **Neon 的 Free 方案**可以省下 $19，小型實例可以在上面執行，但有以下取捨：
  - 它每月允許 100 CU 小時，預設的 tick 大約 17 天就會用完。使用較長的排程（例如 `queue = { tick_schedule = "*/10 * * * *" }`）就能保持在額度內：在其他任何東西喚醒它之前，最多一半的時間處於清醒狀態，約 91 CU 小時。這樣定期任務最多會延遲 10 分鐘執行，遺失的喚醒訊號也會較晚恢復。
  - 聯邦伺服器全天候都會收到傳送，這也會喚醒資料庫，因此不論排程為何，繁忙的實例都會超出它的額度。
  - 它每個專案只能保存 1 GB，可還原的歷史紀錄為 6 小時，而且它的運算資源一定會在閒置 5 分鐘後停止，因此在那之後的第一個請求必須等待它啟動。
