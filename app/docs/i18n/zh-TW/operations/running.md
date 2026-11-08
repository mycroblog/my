<!-- source: running-the-server-start-stop-and-health-checks 5164430b3203 -->
# 執行伺服器：啟動、停止與健康檢查

本頁說明如何啟動和停止伺服器、取得其映像檔，以及在執行期間監看它：指標、追蹤、任務佇列和健康檢查。

<!-- source: start-the-server 0d5f8898a2f5 -->
## 啟動伺服器

```sh
my config check   # report every configuration problem at once
my config check -instance   # also warn about instance settings that are not set (opens the database)
my serve          # start according to the configured role (-config <file> reads a TOML file)
my version        # print the version of the binary
```

`my serve` 會驗證設定，建立資料庫、任務佇列和金鑰管理的轉接器，並在 `listen` 上接受 HTTP。它在那裡使用不含 TLS 的純 HTTP/1.1：請在前端放置反向代理或負載平衡器，由它終止 HTTPS（並為用戶端提供 HTTP/2 或 HTTP/3），再以 HTTP/1.1 轉送。即時更新（`GET /api/v1/streaming`、本站與聯邦時間軸的串流，以及語音聊天室的串流）是長時間持續的回應，每 30 秒（語音聊天室為 15 秒）傳送一次心跳註解，並在閒置時保持開啟。代理不得緩衝這些回應（nginx：伺服器會傳送 `X-Accel-Buffering: no`），而且它的閒置或讀取逾時必須比心跳間隔長。它不會套用結構描述：請在啟動前以 `my schema apply` 套用（請參閱[資料庫](database.md)；升級時會在[升級](database.md#upgrading)的流程中執行）。在 Docker Compose 配置中（`deploy/docker/compose.yaml`；請參閱[用 Docker Compose 部署](deploy-docker-compose.md)），一次性的 `schema` 服務會在伺服器啟動前執行 `my schema apply`。如果尚未套用結構描述，程序仍會啟動，但就緒檢查會回傳 503，在結構描述就緒之前，它不會提供其他任何服務。

第一個使用資料庫的 `my serve` 或 `my` 指令會把 `host` 和 `account_domain` 記錄到資料庫中。當設定與記錄的值不同時，`my serve` 會以兩組值記錄 `start failed`，並以代碼 1 結束，其他指令也會顯示相同的錯誤。例外是不做檢查的 `my schema apply` 和 `my db backup`，以及不讀取資料庫的維護模式。`my config check -instance` 和 `my db upgrade -check` 會檢查，但不會記錄任何東西。啟動時無法讀取資料庫的程序，在能夠讀取之前都不會就緒；在那之前，它不會執行任何任務，並對下列三個健康檢查（`/healthz`、`/readyz`、`/api/v1/instance/health`）以外的所有請求以 `503` 附 `Retry-After` 回應。在 `my schema apply` 之前啟動的伺服器也是如此。如果它之後發現值不同，就會以相同的錯誤停止。

在 `production` 設定檔中，`my config check` 還會檢查有效的設定是否適合正式環境的實例：錯誤（例如其他伺服器無法連線的 `host`，或不是隨機的 KEK）會讓它以 1 結束，而警告（例如 `mail = none`）則不會。`my serve` 啟動時會把兩者都記錄為警告，但仍會啟動，所以請把 `my config check` 當作部署的一個步驟來執行。[正式環境檢查清單](production-checklist.md)列出了這些檢查，以及上線前需要手動確認的事項。

| `role` | HTTP | 任務工作程序 |
| --- | --- | --- |
| `all`（預設） | 所有路由 | 執行 |
| `web` | 所有路由 | 不執行 |
| `worker` | 只有健康檢查（供平台探測使用） | 執行 |

日誌會以 JSON Lines 格式輸出到標準錯誤。每一行都有 `level`（`DEBUG`、`INFO`、`WARN`、`ERROR`）和 `severity`，後者是以 Cloud Logging 讀取的形式表示的同一個等級（`DEBUG`、`INFO`、`WARNING`、`ERROR`，以及用於讓程序停止之錯誤的 `CRITICAL`）。處理請求時寫入的日誌會帶有 `request_id`，同一個 ID 也會在 `X-Request-Id` 回應標頭中傳回。如果請求的 `X-Request-Id` 最多 128 個字元，且只由英文字母、數字和 `-_.:` 組成，就會被視為負載平衡器指定的 ID 而原樣使用。追蹤內（請求和任務）的日誌也會帶有 `trace_id` 和 `span_id`（請參閱[追蹤](#traces)）。

每個 HTTP 請求和每次任務執行在結束時都會寫入一行。這一行的名稱和值與下方[指標](#metrics)的標籤相同，所以在 `metrics_exporter = none` 的 Cloud Run 上，你可以從這些行建立 Cloud Logging 的記錄型指標（計數器和分布）。

| 行（`msg`） | 欄位 |
| --- | --- |
| `http request`（`INFO`；健康檢查使用 `DEBUG`） | `http.request.method`、`http.route`、`my.surface` 和 `http.response.status_code`（數字）的值與 `http.server.request.duration` 的標籤相同。另外還有 `duration_seconds`（秒數）、`url.path`（不含查詢字串的路徑）、`http.response.body.size` 和 `request_id`。對於沒有到達路由的請求（健康檢查、在路由之前就被拒絕的請求），沒有 `http.route` 和 `my.surface`，指標也不會計入。用戶端在伺服器回應前就離開的請求，狀態會是 `499` 而不是 5xx，除非處理常式發生 panic；用戶端沒有收到任何東西，所以不算伺服器錯誤 |
| `job`（成功和 `postponed` 時為 `INFO`，`retry` 時為 `WARN`，`dead` 時為 `ERROR`） | `my.job.type` 和 `my.outcome` 的值與 `my.job.duration` 的標籤相同。另外還有 `duration_seconds`（`my.job.duration`）、`wait_seconds`（`my.job.wait`）、`job_id`、`attempt`，以及失敗時的 `error` |
| `error report`（`ERROR`；讓程序停止的失敗為 `CRITICAL`） | 每個回報的錯誤一行（[錯誤回報](#error-reports)） |

<!-- source: error-reports 3d780744461c -->
### 錯誤回報

[錯誤回報](error-reporting.md)指南說明在各平台上去哪裡閱讀這些行、「錯誤」畫面、通知，以及把回報傳送到錯誤追蹤服務。

伺服器會把需要營運者注意的錯誤，各以一行 `error report` 回報：

- HTTP 處理常式、任務或背景作業中的 panic；
- 用戶端 API、管理 API、登入頁面、ActivityPub、OAuth 或媒體回應 5xx 背後的錯誤；
- 永久失敗（`dead`）的任務：重試次數已用完，或拒絕再次執行。因為對方伺服器持續失敗而放棄的傳送不會回報；
- 啟動失敗：設定無效，或無法建立資料庫或轉接器。

4xx 回應、任務重試、維護模式、失敗的就緒檢查、用戶端已離開的請求，以及由其他伺服器造成的失敗都不會回報；指標和其他日誌行涵蓋了這些情況。

當請求的用戶端離開時，伺服器針對該請求的其他錯誤行會以 `INFO` 而不是 `ERROR` 寫入，因為它們大多只是表示請求已被取消。

這一行採用 Google Cloud Error Reporting 讀取的格式：在 Cloud Run 上，回報會在不做任何設定的情況下出現在 Error Reporting 中，並依錯誤分組。在其他平台上，它是一般的 JSON 行：

| 欄位 | 值 |
| --- | --- |
| `@type`、`serviceContext` | Error Reporting 的事件類型，以及 `{"service": "my", "version": "<version>"}` |
| `my.error.fingerprint` | 32 位十六進位數字，同一個錯誤每次發生時都相同：根據來源、位置、錯誤類型和堆疊中伺服器的函式建立，不含行號 |
| `my.error.source`、`my.error.kind` | `server`（或 `web`），以及 `panic` 或 `error` |
| `my.error.where` | 發生的位置：請求為 `<surface> <method> <route>`（例如 `client_api GET /api/v1/statuses/{id}`），任務為任務類型，背景作業為作業名稱，啟動失敗為 `startup` |
| `my.role`、`deployment.environment.name` | 程序的 `role` 和 `errors_environment` |
| `request_id`、`job_id`、`attempt`、`trace_id`、`span_id` | 用來找出同一個錯誤的請求行或任務行的 ID（如果有的話） |
| `message` | 位於行的最後：`<type>: <message>`、換行，以及 Go 的堆疊 |

回報離開程序之前，訊息中的 URL、電子郵件地址、`@user@host` 帳號代稱、IP 位址、UUID 和 ULID，以及 10 位數以上的數字，會分別被替換為 `<url>`、`<email>`、`<acct>`、`<ip>`、`<id>` 和 `<n>`，並截斷為 1 KiB。堆疊會保留檔案位置，但不保留函式行的引數值（顯示為 `(...)`），並截斷為 16 KiB。任何回報都不會包含請求內文、標頭、查詢、cookie、帳號、使用者名稱、IP 位址或 User-Agent。其他任何行都不帶堆疊，所以 Error Reporting 只會在這些行中找到堆疊。

欄位名稱是包含 `.` 的單一鍵，而不是巢狀結構。在 Cloud Logging 的篩選器和擷取中請加上引號（例如 `jsonPayload."http.route"`，分布值則用 `EXTRACT(jsonPayload.duration_seconds)`）。作為標籤使用的欄位只會取有限集合中的值；`url.path`、`request_id` 和 `job_id` 絕不會是標籤。

使用 `metrics_exporter = emf` 時，這兩行本身就是 EMF 行。`http request` 行帶有 `http.server.request.duration` 的值，`job` 行帶有 `my.job.duration` 和 `my.job.wait` 的值（鍵以指標命名），另外還有 `_aws` 指示詞、`service.name`、`service.version` 和 `my.role`。這三個指標不會寫在其他任何行上。你在 `metrics_emf_dimensions` 中選擇的欄位會依 EMF 的要求以字串寫入（例如維度 `http.response.status_code` 為 `"200"`）。日誌輸出到標準錯誤，其他 EMF 行則輸出到標準輸出。ECS 的 `awslogs` 日誌驅動程式會把兩者都送到 CloudWatch Logs，所以會從兩者擷取指標。

<!-- source: docker-compose-files df45a902f45b -->
## Docker Compose 檔案

`deploy/docker/` 存放部署實例所需的檔案。`compose.yaml` 可以單獨執行，其他檔案則疊加在它之上。

| 檔案 | 新增的內容 | 使用時機 |
| --- | --- | --- |
| `compose.yaml` | 伺服器及其一次性的資料庫服務，使用 SQLite，媒體檔案放在磁碟區 `data` 中 | 一律使用 |
| `compose.postgres.yaml` | 容器中的 PostgreSQL，放在磁碟區 `postgres` 中 | 你想在這台機器上使用 PostgreSQL。若要使用其他地方的 PostgreSQL 伺服器，請不要加上它，並在 `.env` 中設定 `MY_DATABASE` 和 `MY_DATABASE_URL` |
| `compose.s3.yaml` | 把媒體放在相容 S3 的儲存貯體中；搭配 `COMPOSE_PROFILES=garage` 時，還會在磁碟區 `garage` 中加上單節點的 [Garage](https://garagehq.deuxfleurs.fr/) 儲存 | 媒體超出一顆硬碟的容量，或你想把媒體放在儲存貯體中。不使用該設定檔時，儲存空間是 `MY_BLOB_S3_*` 設定所指的外部儲存 |
| `compose.caddy.yaml` | Caddy，在連接埠 80 和 443 上提供 TLS 憑證 | `MY_HOST` 是公開網域，而這台機器上沒有其他元件終止 TLS |
| `compose.build.yaml` | 從簽出的原始碼建置映像檔 | 你執行自己的建置 |

請在第一次啟動前選好資料庫和媒體儲存：檔案變更時，資料不會自動搬移（[備份](backup.md)說明了哪些資料需要搬移）。

**疊加檔案** 時，請依上述順序使用 `-f` 選項，或在 `.env` 中指定一次，讓之後所有的 `docker compose` 指令（`exec`、`logs`、`pull`，以及其他指南中的指令）都使用相同的檔案：

```sh
cd deploy/docker
cp example.env .env
# In .env: COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml
```

只有在 `-f` 和 `COMPOSE_FILE` 都沒有指定檔案時，才會讀取目錄中名為 `compose.override.yaml` 的檔案。使用它們時，也請把它列進去。

**設定**。把 `example.env` 複製為 `.env`。Compose 會讀取 `.env` 來填入其檔案中的變數，並把整個檔案交給伺服器的容器，所以[設定參考](../reference/configuration.md)中的每一項設定都可以在這裡設定。範本列出了所有設定，每項都附有一句說明。

- 金鑰加密金鑰不放在 `.env` 中，而是放在 `secrets/kek` 檔案中，容器會以 Docker secret `/run/secrets/my_kek` 讀取它，所以它絕不會變成環境變數。`.env` 中若有 `MY_KEK` 這一行，伺服器會停止，因為它拒絕同時設定 `MY_KEK` 和 `MY_KEK_FILE`（[管理金鑰](key-management.md)）。
- 伺服器以使用者 `65532` 執行，而 Compose 會以檔案原有的權限掛載它，所以這個檔案必須讓其他使用者可以讀取。請把它放在只有你能進入的目錄中，就像 `example.env` 中的指令那樣。
- 其他機密會以環境變數的形式傳給容器。若要改以檔案傳遞，請在你自己的 `compose.override.yaml` 中以 secret 掛載它，並設定其 `<NAME>_FILE`，就像 `compose.yaml` 對金鑰所做的那樣。
- `.env` 包含機密：請執行 `chmod 600 .env`，並與備份一起保留一份副本。
- `MY_LISTEN` 維持 `:8080`，因為這些檔案會連線到那個連接埠。

**使用 Caddy 的 TLS**。把 `MY_HOST` 設為 DNS 指向這台機器的網域，讓連接埠 80 和 443 能從網際網路連線，並從 `.env` 中移除 `MY_WEB_ORIGIN`。Caddy 會在第一個請求時自行取得並更新憑證。伺服器會信任來自 Docker 預設位址集區的 `X-Forwarded-For`；如果你的位址集區不同，請設定 `MY_TRUSTED_PROXIES`。

**內附的 Garage**。搭配 `COMPOSE_PROFILES=garage` 的 `compose.s3.yaml` 需要 `example.env` 開頭所列的值。一次性的服務 `garage-init` 會在每次 `up` 時準備儲存空間。它只在 Compose 網路內部運作，因為媒體由伺服器自己提供。一個節點就是一顆硬碟：請備份磁碟區 `garage` 或儲存貯體（[備份](backup.md#media-in-a-bucket)）。

<!-- source: get-the-image 238d00ea9826 -->
## 取得映像檔

伺服器映像檔是 `ghcr.io/mycroblog/my`，公開提供，支援 `linux/amd64` 和 `linux/arm64`，並包含所有轉接器。

| 標籤 | 指向 | 用途 |
| --- | --- | --- |
| `0.1.0` | 該版本，永久不變 | 指定版本 |
| `0.1` | `v0.1` 的最新修補版本 | Docker Compose，它會在每次 `up` 時套用結構描述 |
| `0.1.0-rc.1` | 該預發布版本 | 在正式發布前試用某個版本 |

沒有 `latest` 和 `0`：兩者都會跨越次要版本，而每次升級到新的次要版本都必須[經過前一個版本](database.md#skipping-releases)。請在每個指令中指定版本；只執行 `docker pull ghcr.io/mycroblog/my` 會以 `manifest unknown` 失敗。修補版本也可能需要 `my schema apply` 和 `my db upgrade`，所以在 Docker Compose 以外的環境，請針對每個版本依照[升級](database.md#upgrading)的說明進行。在正式環境中，請固定使用版本發布說明中提供的摘要值：

```text
ghcr.io/mycroblog/my:0.1.0@sha256:...
```

執行的位置：

- **Docker Compose**：`deploy/docker/compose.yaml` 會執行 `.env` 中 `MY_VERSION` 所指的映像檔（[Docker Compose 檔案](#docker-compose-files)）。若要改為從簽出的原始碼建置，請加上覆寫檔 `compose.build.yaml`（`docker compose -f compose.yaml -f compose.build.yaml up -d --build`），並把 `MY_VERSION` 設為你自己取的名稱，因為它是你本機映像檔的名稱，也是 `my version` 會顯示的內容。
- **Kubernetes**：直接從 `ghcr.io` 拉取。
- **Cloud Run**：直接使用 `ghcr.io` 的映像檔。部署時會把標籤解析為摘要值。
- **AWS（ECS、EKS）**：把映像檔複製到 Amazon ECR，或使用 ECR 提取快取（需要 ghcr.io 的憑證）。
- **較小的映像檔**：在簽出的原始碼中執行 `docker build --build-arg VERSION=<name> --build-arg GO_TAGS=... -t my:<name> .`，可以建置包含較少轉接器的映像檔（[建置標籤](../reference/build-tags.md)）。這是你自己的建置，所以不在下列驗證的範圍內。

<!-- source: verify-an-image b696c257662d -->
### 驗證映像檔

每個發布的映像檔都附有透過 Sigstore 簽署的來源證明（provenance），它把摘要值與建置它的工作流程和提交綁定在一起，另外還附有 SBOM。使用 [GitHub CLI](https://cli.github.com/)，並在指令要求時登入 `ghcr.io`：

```sh
gh attestation verify oci://ghcr.io/mycroblog/my@sha256:... -R mycroblog/my \
  --signer-workflow mycroblog/my/.github/workflows/release.yml
docker buildx imagetools inspect ghcr.io/mycroblog/my@sha256:... --format '{{ json .SBOM }}'
```

除非映像檔是由這個儲存庫的發布工作流程建置，否則第一個指令會失敗。它會顯示提交，也就是該版本標籤的提交；映像檔的標籤 `org.opencontainers.image.revision` 也會指出同一個提交。

<!-- source: publish-the-source-code-agpl-30 2f3b73ce9dfc -->
## 公開原始碼（AGPL-3.0）

`my` 以 AGPL-3.0-only 授權。AGPL-3.0 第 13 條要求，任何執行伺服器並讓使用者透過網路與其互動的人，都必須向這些使用者提供取得正在執行之程式的對應原始碼的方法。這適用於所有與伺服器互動的人，而不只是擁有帳號的人。

請以設定 `source_url`（環境變數 `MY_SOURCE_URL`；請參閱[設定參考](../reference/configuration.md#source_url)）指出原始碼公開的位置。

| 值 | 意義 |
| --- | --- |
| 未設定 | 預設值 `https://github.com/mycroblog/my`。在你執行未經修改的 `my` 時使用：這時發布版本的 `GET /instance` 會指出該版本的原始碼 `https://github.com/mycroblog/my/tree/v0.1.0`，NodeInfo 則會指出儲存庫 |
| `https:` URL | 可以閱讀原始碼的地方（分支專案的儲存庫、特定版本的標籤或封存檔等） |
| `mailto:` URI | 用來索取原始碼的電子郵件地址（例如 `mailto:source@example.org`；可以加上 `?subject=...`） |

其他任何值（`http:`、`git@...`、相對路徑、含有使用者資訊或 `#` 的 URL、沒有地址的 `mailto:` 等）都會讓 `my config check` 失敗，伺服器也不會啟動。

這個值會出現在以下位置：

- 用戶端 API 的 `GET /instance` 中的 `source_url`
- 網頁用戶端實例資訊頁面（`/about`）的「原始碼」區段，以及入口頁面（`/`）上的連結
- NodeInfo（`/nodeinfo/2.1`）中的 `metadata.sourceCode`。`https:` URL 也會以 `software.repository` 公開（NodeInfo 2.1 的結構描述在 `software.repository` 中只允許 `http(s)` URL，所以 `mailto:` 值只會出現在 `metadata.sourceCode` 中）

<!-- source: when-you-fork-or-modify-my be83256fe6a3 -->
### 當你分支或修改 `my` 時

如果你執行的是修改過的 `my`（套用修補、建置分支專案或替換相依套件），預設的 URL 就不再指向正在執行之程式的原始碼。請把 `source_url` 改為可以找到正在執行的版本（包含你的修改）之原始碼的位置。指向與執行版本相符的提交、標籤或封存檔，而不是儲存庫的最新狀態，能讓使用者確實取得相同的原始碼。如果你執行的是未經修改的官方發布版本，請保留預設值。

<!-- source: when-you-use-mailto 11155a8b71ed -->
### 當你使用 `mailto:` 時

- 請準備好在有人索取時，迅速提供對應的原始碼（包括建置所需的指令碼）。只寫上地址而沒有準備好交付，並不符合義務。
- 使用在你停止營運伺服器或負責人更換後仍然有效的地址。共用的營運地址比個人信箱更好。
- 確認垃圾郵件過濾不會擋掉索取請求。
- 使用你願意公開的地址：任何人都能在資訊頁面、入口頁面和 NodeInfo 中看到它。

<!-- source: metrics 2dea3a7f8f83 -->
## 指標

設定 `metrics_exporter`（預設 `none`）會匯出服務指標。相關設定請參閱[設定參考](../reference/configuration.md)。

| `metrics_exporter` | 輸出路徑 | 適用於 |
| --- | --- | --- |
| `none` | 不匯出任何內容。量測幾乎不產生成本 | Cloud Run（改用平台的指標和記錄型指標） |
| `prometheus` | `metrics_listen`（預設 `127.0.0.1:9464`）上的 `GET /metrics`。這是與公開的 `listen` 分開的監聽器，不需驗證，在每種 `role` 下都會執行 | 自行架設（由相容 Prometheus 的伺服器或 KEDA 擷取） |
| `otlp` | 透過 OTLP/HTTP 傳送到 OpenTelemetry Collector。目的地取自 `OTEL_EXPORTER_OTLP_ENDPOINT` 等標準 OTel 環境變數 | 可以執行收集器的環境 |
| `emf` | CloudWatch Embedded Metric Format 的 JSON 行。請求和任務的指標附在上述結束時的日誌行上；其他指標會在請求或任務結束時寫到標準輸出，每組標籤一行 | AWS ECS |

指標列在下方。在 Prometheus 中，名稱中的 `.` 會變成 `_`，並加上單位後綴（`_seconds`，計數器則為 `_total`）。

| 指標 | 種類 | 標籤 |
| --- | --- | --- |
| `http.server.request.duration` | 直方圖，秒 | `http.request.method`、`http.route`（路由模式的路徑；沒有相符時為 `other`）、`http.response.status_code`、`my.surface`（`client_api` / `admin` / `activitypub` / `oauth` / `media` / `web` / `payments` / `voice` / `queue`） |
| `http.server.active_requests` | UpDownCounter | `my.surface` |
| `my.inbox.activities` | 計數器 | `activity.type`（已知的 Activity 類型，或 `other`）、`my.outcome`（`accepted` / `duplicate` / `rejected` / `ignored` / `error`）、`my.inbox.rejection`（僅在 `rejected` 時：`signature` 表示簽章或完整性證明缺少、無效、無法驗證，或無法取得其金鑰；`suspended` 表示網域已停權；`federation_off`；`rate_limited`；`invalid` 表示活動沒有有效的 `id`、`type`、`actor` 或 `object`） |
| `my.relay.activities` | 計數器 | `my.outcome`（`admitted` / `over_limit` / `restricted` / `not_receiving`：由中繼站傳來的活動是否被接收，以及被捨棄的原因；請參閱[聯邦中繼站](relays.md)） |
| `my.move.activities` | 計數器 | `my.outcome`（`recorded` / `duplicate` / `not_own_actor` / `same_account` / `target_unavailable` / `alias_missing` / `target_moved` / `suspended` / `too_soon`：收到的 `Move` 是否已記錄，以及被捨棄的原因） |
| `my.delivery.attempts` | 計數器 | `my.outcome`（`success` / `retry` / `gone` / `failed`；`gone` 包括 410，以及因伺服器無法使用或已停權而略過的傳送） |
| `my.delivery.duration` | 直方圖，秒 | `my.outcome` |
| `my.plugin.failures` | 計數器 | `my.plugin.kind`（[外掛](plugins.md)的種類）、`my.plugin.code`（失敗的 Connect 代碼） |
| `my.plugin.slot_wait` | 直方圖，秒 | `my.plugin.kind`（呼叫在同時執行的同種類呼叫中等待輪到自己的時間） |
| `my.hook.decisions` | 計數器 | `hook`、`mode`、`outcome`（[決策掛鉤](decision-hooks.md)所記錄的決策） |
| `my.hook.failures` | 計數器 | `hook`、`outcome`（`timeout`、`error`、`invalid` 或 `breaker_open`） |
| `my.hook.duration` | 直方圖 | `hook`（每次呼叫外掛的秒數） |
| `my.job.duration` | 直方圖，秒 | `my.job.type`（已註冊的任務類型，或 `other`）、`my.outcome`（`success` / `retry` / `dead` / `postponed`：`postponed` 是沒有做任何工作、也不消耗任務嘗試次數的執行，例如等待其主機的連結卡片擷取） |
| `my.job.wait` | 直方圖，秒 | `my.job.type` |
| `my.link_card.host_pauses` | 計數器 | `http.response.status_code`（`429` / `503`）。每次暫停從某個主機擷取連結卡片時計一次；請參閱[連結預覽](link-previews.md#limits-per-host) |
| `my.translation.requests` | 計數器 | `my.outcome`（`author` / `cache` / `provider` / `no_provider` / `limit` / `unsupported` / `failed`）。每次請求翻譯檢視者可以看到的貼文，或版主要求翻譯檢舉的貼文或留言時計一次：由作者自己的文字、保留的翻譯或服務提供者回答，或因沒有服務提供者可以接收該貼文、已達每月上限、沒有服務提供者支援該語言，或沒有服務提供者傳回可用的翻譯而拒絕。 |
| `my.translation.characters` | 計數器 | `my.provider`（翻譯服務提供者的 ID）。傳送給服務提供者的貼文和檢舉留言的字元數，不含標記，包括失敗的呼叫。這就是[每月上限](instance-settings.md#admin-screens-and-the-client-api)所計算的數量，可能與服務提供者的計費不同 |
| `my.timeline.delivery.lag` | 直方圖，秒 | 無（從本站貼文建立到寫入首頁時間軸並通知串流的時間） |
| `my.stream.connections` | UpDownCounter | `my.stream.kind`（`user`；`timeline` 表示本站與聯邦時間軸的串流；`voice` 表示語音聊天室的串流） |
| `my.accounts.registered` | 計數器 | 無 |
| `my.statuses.created` | 計數器 | `my.visibility` |
| `my.billing.imports` | 計數器 | `my.billing.source`（`aws` / `gcp` / `plugin`）、`my.outcome`（`success` / `failed`）、`error.type`（僅在 `failed` 時：失敗的種類，如[匯入雲端使用費用](billing-import.md)所述）。每次嘗試都會計入，包括重試和等待下一次每日匯入的失敗 |
| `my.voice.turn.credentials` | 計數器 | `my.voice.turn`（`coturn` / `cloudflare`）、`my.outcome`（`issued` / `reused` / `failed`）。語音聊天參與者在加入聊天室或更新連線時要求的每個 TURN 憑證計一次：`reused` 表示再次傳回先前為同一個連線發出的 Cloudflare 憑證，`failed` 表示無法連線到 Cloudflare 或遭到拒絕，參與者因此收到錯誤而無法連線 |
| `my.errors.exported` | 計數器 | `my.outcome`（`success` / `dropped` / `failed`：傳給 `errors_exporter` 服務的每份錯誤回報計一次。`dropped` 表示因為已有 1,000 份回報在等待，或服務要求伺服器放慢速度而沒有傳送，伺服器會依服務要求的時間等待後才再次傳送；`failed` 表示無法連線到服務或服務拒絕了回報。回報只會傳送一次，絕不重試，而關機開始 2 秒後仍在等待的回報會遺失） |
| `my.queue.ready` | 量測計 | `my.job.type`（資料庫佇列中可執行的任務數） |
| `my.queue.oldest_ready_age` | 量測計，秒 | `my.job.type`（資料庫佇列中各類型最舊的可執行任務已等待的時間；沒有可執行任務的類型沒有值） |
| `my.queue.dead` | 量測計 | 無 |
| `my.instance.users`、`my.instance.statuses` | 量測計 | 無（本站的使用者數和貼文數，與 NodeInfo 相同） |
| `my.media.stored_bytes` | 量測計，位元組 | `my.media.origin`（`local` / `remote_cache`） |
| `my.db.size` | 量測計，位元組 | 無（資料庫的大小：PostgreSQL 為 `pg_database_size`；SQLite 為不含預寫日誌的資料庫檔案） |
| `my.billing.import.failing` | 量測計 | `my.billing.source`（該來源最新一次匯入失敗時為 1，否則為 0） |
| `my.federation.instances` | 量測計 | `my.federation.instance.state`（`available`：仍會傳送到、但自上次回應以來至少有一天失敗的伺服器；`unavailable`：傳送已暫停的伺服器） |
| `my.metrics.snapshot.age` | 量測計，秒 | `my.metrics.snapshot.part`（`queue` / `totals`） |
| `my.errors.reported` | 計數器 | `my.error.source`（`server` / `web`）、`my.error.kind`（`panic` / `error`）。每份[錯誤回報](#error-reports)計一次 |

健康檢查（`/healthz`、`/readyz`）不會被量測。任何標籤都不包含遠端網域、帳號、URL 或用戶端 IP。`my.queue.*`、`my.instance.*`、`my.media.stored_bytes`、`my.db.size`、`my.federation.instances` 和 `my.billing.import.failing` 是整個實例的值：每個程序都會從保存在鍵值儲存中的同一份快照回報相同的值。

<!-- source: system-metrics 8b5d40a4eb98 -->
### 系統指標

關於程序、Go 執行階段以及與資源之連線的指標，會透過同一個 `metrics_exporter` 匯出。容器的 CPU、記憶體、執行個體數和輸入請求交給平台的指標；這些指標涵蓋的是平台看不到的程序內部飽和狀態。不要用它們來觸發擴展；請把它們用於警示和估算執行個體數上限。

| 指標 | 種類 | 標籤 |
| --- | --- | --- |
| `go.memory.used`、`go.memory.limit`、`go.memory.gc.goal` | 量測計，位元組 | 只有 `go.memory.used` 有 `go.memory.type`（`stack` / `other`）。`go.memory.limit` 只在設定了 `GOMEMLIMIT` 時出現 |
| `go.memory.allocated`、`go.memory.allocations` | 計數器 | 無 |
| `go.goroutine.count`、`go.processor.limit`、`go.config.gogc` | 量測計 | 無 |
| `go.schedule.duration`、`go.memory.gc.pause.duration` | 直方圖，秒 | 無（從變成可執行到開始執行的等待時間，以及 GC 暫停） |
| `go.memory.gc.cycles` | 計數器 | 無 |
| `process.cpu.time` | Observable Counter，秒（自啟動以來的總計） | `cpu.mode`（`user` / `system`） |
| `process.memory.usage` | 量測計，位元組 | 無（常駐記憶體，RSS） |
| `process.unix.file_descriptor.count`、`my.process.file_descriptor.limit` | 量測計 | 無（開啟的檔案描述元及其上限） |
| `process.uptime` | 量測計，秒 | 無 |
| `db.client.connection.count` | 量測計 | `db.client.connection.pool.name`（`postgres` / `sqlite`）、`db.client.connection.state`（`idle` / `used`） |
| `db.client.connection.max` | 量測計 | `db.client.connection.pool.name` |
| `db.client.connection.wait_time`、`my.db.client.connection.waits`、`db.client.connection.timeouts` | Observable Counter（自啟動以來的總計；秒或次數） | `db.client.connection.pool.name`。`timeouts` 僅限 PostgreSQL |
| `my.adapter.operation.duration` | 直方圖，秒 | `my.adapter`（`queue.db` / `kv.db` / `kv.memory` / `kv.valkey` / `stream.<stream>` / `blob.fs`）、`my.operation`、`my.outcome`（`success` / `error`） |
| `my.adapter.connections` | UpDownCounter | `my.adapter`（`valkey` 連線，以及 `stream.<stream>` 訂閱者） |
| `my.adapter.reconnects` | 計數器 | `my.adapter` |
| `http.client.request.duration` | 直方圖，秒 | `my.http.purpose`（`json` / `html` / `image` / `video` / `human_check` / `web_push` / `webhook` / `client_metadata` / `login_provider` / `robots`）、`http.request.method`、`my.http.status_class`（`2xx` 等）、`error.type`（`forbidden_destination` 表示被 SSRF 防護拒絕） |
| `http.client.open_connections` | UpDownCounter | 無 |
| `my.readiness.failures` | 計數器 | 無 |
| `my.http.origin_refused` | 計數器 | 無。每個因為沒有經過 CDN 而被拒絕的請求計一次（[`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)）。對來源位址的探測會讓它緩慢增加；如果在流量停止的同時它突然跳升，就表示 CDN 不再傳送該標頭。維護模式不會量測任何東西，所以只有它的存取日誌會顯示這些請求 |

- Go 執行階段指標（`go.*`）只由 `prometheus` 和 `otlp` 匯出。使用 `emf` 時，定期的快照任務會寫入程序和連線池的量測計與 Observable Counter。
- 連線池等待和 CPU 時間是 Observable Counter：連線池和核心會保存自啟動以來的總計，並在收集時讀取。在 Prometheus 中它們是 `_total` 計數器（例如 `process_cpu_time_seconds_total`），在 OTLP 中是單調遞增的總和，你可以用 PromQL 的 `rate()` 查看其增長。`emf` 沒有總計型的計數器，所以它會寫入自上次寫入以來的增加量（用 CloudWatch 的 Sum 統計值加總）。
- 佇列的 `claim` 只在取得任務或失敗時才會量測。等待中的工作程序的輪詢不會記錄任何東西。
- `db.client.connection.pending_requests` 不會匯出：pgx 和 `database/sql` 的統計資料不會回報有多少請求在等待，所以改以等待的次數和時間代替。
- 請針對 `go.memory.used` 的總和與 `go.memory.limit` 的比率，以及 `db.client.connection.wait_time` 的增長（表示已達連線池上限）設定警示。

<!-- source: memory-limit-gomemlimit 8ea44722eefd -->
### 記憶體上限（`GOMEMLIMIT`）

`GOMEMLIMIT` 是由營運者設定的 Go 執行階段環境變數；應用程式不會自行決定。沒有設定時，Go 以其預設值（無上限）執行，堆積在接近容器記憶體上限時也不會積極回收。在有容器記憶體上限的環境中（Cloud Run、ECS、Kubernetes），請把它設為該上限的約 90%（例如 512 MiB 的容器設為 `GOMEMLIMIT=460MiB`）。不要設定 `GOMAXPROCS`：Go 的預設值會依循容器的 CPU 上限。

<!-- source: traces e274793f5885 -->
## 追蹤

設定 `traces_exporter`（預設 `none`）會匯出分散式追蹤。相關設定請參閱[設定參考](../reference/configuration.md)。

| Span | 名稱 | 屬性 |
| --- | --- | --- |
| HTTP 請求（伺服器） | `<method> <http.route>`（沒有到達路由的請求為 `<method>`） | `http.request.method`、`http.route`、`http.response.status_code`、`my.surface`。5xx 視為失敗 |
| 任務執行（消費者） | 任務類型（`my.job.type`） | `my.job.type`、`my.job.attempt`、`my.outcome`。每次重試都是獨立的 span |
| 交易 | `my.db.transaction` | `db.system.name`。每次重試都是獨立的 span |
| 查詢（用戶端） | sqlc 的查詢名稱（例如 `InsertJob`；否則為 SQL 的第一個字，例如 `BEGIN`） | `db.system.name`、`db.operation.name` |
| 傳送（`my.delivery`） | `my.delivery` | `activity.type`、`my.outcome` |
| 對外 HTTP（用戶端） | `<method>` | `http.request.method`、`server.address`（只有目的地的主機名稱）、`http.response.status_code`。4xx 或 5xx 視為失敗 |

失敗的 span 帶有 `error.type`（例如 `timeout` 這類有限的名稱，或狀態碼）。屬性絕不包含帳號、貼文或任務的 ID、URL、路徑或查詢、用戶端 IP、User-Agent 的值、SQL 文字或錯誤字串。`server.address` 會把遠端伺服器的主機名稱留在接收端，所以由你決定接收端要保存它們多久，以及誰可以檢視。健康檢查和被 `origin_secret` 拒絕的請求不會建立 span。

- 任務執行不是把它放入佇列的 span（通常是 HTTP 請求）的子項。它是 **新追蹤的根**，並帶有指向放入佇列之 span 的連結。在檢視器中，請從請求的追蹤沿著連結查看。
- 只有在連線的對象位於 `trusted_proxies` 中，而且（使用 [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) 時）請求帶有該密鑰時，傳入的 `traceparent` 才會成為父項。否則它會被捨棄，並開始新的追蹤。`tracestate` 一律會被捨棄。對外的 HTTP（聯邦、WebFinger、媒體、Web Push 等）不帶 `traceparent`。
- 取樣：如果請求來自受信任代理的父項已被記錄，或任務的連結目標已被記錄，該請求或任務就會被記錄；否則以 `traces_sample_ratio`（預設 0.01）的比率記錄。降低比率絕不會讓延續已記錄請求的任務缺少它們的追蹤。

| 目標 | 設定 | 輸出路徑 |
| --- | --- | --- |
| 預設、小型自行架設 | `traces_exporter = none` | 不匯出任何 span。請在日誌中追蹤 `request_id` 和 `job_id`。如果受信任的代理提供了 `traceparent`，其 `trace_id` 和 `span_id` 會加到日誌中 |
| 自行架設（Jaeger、Tempo、OpenTelemetry Collector 等） | `traces_exporter = otlp`、`traces_flush = batch` | 透過 OTLP/HTTP 傳送到位於 `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`（或 `OTEL_EXPORTER_OTLP_ENDPOINT`，預設 `http://localhost:4318`）的接收端。驗證標頭放在 `OTEL_EXPORTER_OTLP_HEADERS` 中。若要確實保留錯誤和緩慢的追蹤，請使用 Collector 的尾端取樣 |
| Google Cloud（Cloud Run） | `none`，或以 `otlp` 搭配 `unit` 啟用追蹤 | 請參閱下文 |
| AWS（ECS） | `none`，或以 `otlp` 搭配 `batch` 啟用追蹤 | 請參閱下文 |

<!-- source: cloud-run 7f1a21bbfbef -->
### Cloud Run

即使使用 `none`，Cloud Run 自動建立的請求追蹤與日誌中的 `trace_id` 也會共用同一個 ID（當你以 `trusted_proxies` 信任來自 Cloud Run 前端的連線時）。Cloud Run 是否一律覆寫公開請求的 `traceparent` 尚未驗證。日誌並未採用 Cloud Logging 所辨識的追蹤欄位格式（`logging.googleapis.com/trace`）。

若要把應用程式的 span 傳送到 Cloud Trace，請在同一個服務中 **以 sidecar 執行 OpenTelemetry Collector**。伺服器不持有任何 Google 憑證；由 sidecar 以 Application Default Credentials 傳送到 Cloud Trace 的 OTLP 端點。

- 伺服器（輸入容器）：`MY_TRACES_EXPORTER=otlp`、`MY_TRACES_FLUSH=unit`、`OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`
- Sidecar：一個 OpenTelemetry Collector，以 `otlp` 接收器透過 HTTP 在 `localhost:4318` 上接收，並以 `googlecloud` 匯出器或 `otlphttp` 匯出器（`https://telemetry.googleapis.com`，搭配 `googleclientauth` 擴充功能）傳送。請授予服務帳號寫入 Cloud Trace 的權限。
- 在每個被記錄的請求或任務結束時，`unit` 會在返回之前把目前為止的 span 清出到 sidecar。為了讓 sidecar 能在沒有分配 CPU 時傳送它們，啟用追蹤的配置請使用以執行個體計費的方式。
- 費用是 sidecar 的資源以及 span 的數量（Cloud Trace 定價）。請以 `traces_sample_ratio` 選擇比率。

<!-- source: aws-ecs 52fa09412f20 -->
### AWS ECS

若要傳送到 X-Ray，請在任務中以 sidecar 容器執行 **AWS Distro for OpenTelemetry（ADOT）Collector**。伺服器沒有 SigV4 簽章功能；由 Collector 簽章並傳送到 X-Ray 的 OTLP 端點（`https://xray.<region>.amazonaws.com/v1/traces`）。

- 伺服器：`MY_TRACES_EXPORTER=otlp`、`OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`。在 Collector 的設定中，以 `otlp` 接收器在 `localhost:4318` 上接收，並以使用 `sigv4auth` 擴充功能的 `otlphttp` 匯出器傳送到 X-Ray。
- 授予任務角色寫入 X-Ray 的權限。
- 費用是 sidecar 在任務資源中所佔的份額，以及 X-Ray 記錄的追蹤數量（每月前 100,000 筆免費）。

<!-- source: stop-the-server 37d4c0cc3d97 -->
## 停止伺服器

收到 SIGTERM 或 SIGINT 時，伺服器會正常關機。

1. 停止接受新的連線和取得任務。
2. 最多等待 25 秒，讓處理中的請求和執行中的任務完成。
3. 25 秒後，取消剩餘請求和任務的 context 並關閉連線。接著再給被取消的任務最多 3 秒記錄其結果，然後以代碼 1 結束（總計最多 28 秒，在 Kubernetes 和 ECS 預設的 30 秒寬限期之內）。被取消的任務會重試。在這 3 秒內沒有停止，或在中途被終止的任務會被放棄，並在其租約到期時由工作程序重新取得，所以可能會執行兩次。

伺服器的背景作業發生 panic 時，也會啟動相同的關機流程，讓伺服器絕不會在損壞的狀態下繼續執行。`serve failed` 日誌行會指出該作業，程序會以代碼 1 結束，由平台或程序管理工具重新啟動它。

若要對資料庫、鍵值儲存或佇列進行作業，請改以維護模式（`MY_MAINTENANCE=true`）重新啟動，而不是停止。請參閱[維護](maintenance.md)。

<!-- source: the-job-queue-and-failed-dead-jobs 55023b12a1aa -->
## 任務佇列與失敗（dead）的任務

[任務](../reference/jobs.md)列出了每種任務類型：它做什麼、由什麼啟動、如何重試，以及重新執行時會發生什麼事。

管理介面的任務頁面（`/admin/jobs`，權限 `manage_jobs`）有三個頁面：

- **佇列狀態**（`/admin/jobs`）：各狀態、各類型與狀態的數量，各類型讓任務等待的時間，以及最舊的等待中任務已等待的時間。
- **任務**（`/admin/jobs/list`）：某個狀態（待處理、等待中、執行中、等待重試、失敗）的任務表格，分頁顯示，包含類型、狀態、排定或失敗的時間，以及嘗試次數。失敗的任務有 **重新執行**。
- **單一任務**（`/admin/jobs/<id>`，從表格中的類型進入）：任務的所有資訊、最後一次錯誤、鍵和酬載、未成功嘗試的記錄（每次嘗試的結果、錯誤、工作程序，以及開始和結束時間），以及失敗任務的 **重新執行**。

管理 API 為 `GET /api/v1/admin/queue`、`GET /api/v1/admin/jobs?state=&type=`、`GET /api/v1/admin/jobs/{id}`、`GET /api/v1/admin/jobs/{id}/attempts` 和 `POST /api/v1/admin/jobs/{id}/retry`。已完成的任務不再位於佇列中，所以找不到它的頁面。

- 只有伺服器持有其任務的佇列，才能列出等待中、執行中和等待重試的任務：`queue = db`、`queue = cloudtasks`（任務也在資料庫中）以及 `queue = inline`。失敗的任務和嘗試記錄在每種佇列下都存放在應用程式資料庫中，所以一律可以列出。
- 嘗試記錄只保留未成功的嘗試（重試以及最終失敗的那次嘗試）。成功的執行請透過結構化的 `job` 日誌行（帶有 `job_id`）追蹤。嘗試結束後經過 `job_attempt_retention`（預設 30 天；請參閱[該設定](../reference/configuration.md#job_attempt_retention)），記錄就會由每小時的清理移除。記錄不包含酬載或處理常式的日誌，錯誤訊息會截斷為前 2,000 位元組。任務處理常式不會把電子郵件地址、IP 位址、權杖和貼文內文放進錯誤訊息中。只檢視電子郵件任務類型的方式，請參閱[寄送電子郵件](email.md)。

處理常式判定為永久失敗的任務，以及用完重試次數的任務，會在資料庫佇列（`queue = db` 和 `queue = cloudtasks`）中成為 dead 任務。`my job` 可以列出、重試和刪除它們。任務頁面的「失敗」狀態和管理 API 也可以列出並重試它們。

```sh
my job list                  # newest failures first: ID, type, run count, time of failure, last error (-limit sets the count, default 50)
my job retry <id> [<id>...]  # reset the attempt count and put the job back in the queue to run immediately
my job delete <id> [<id>...] # remove without running
my job types                 # print every job type of this release
```

- 如果已經有相同冪等金鑰的任務在等待，那個任務就會代替被重試的任務。任務處理常式是冪等的，所以把同一個任務放回佇列多少次都是安全的。
- 未知的 ID 會被顯示出來，指令以代碼 1 結束；其他 ID 仍會被處理。結束代碼 2 表示用法錯誤。
- 沒有人重試的 dead 任務，會在失敗後經過 `dead_job_retention`（預設 30 天；請參閱[該設定](../reference/configuration.md#dead_job_retention)）時由每小時的清理移除。
- `queue = inline` 的 dead 任務存放在伺服器程序的記憶體中，所以 `my job` 無法處理它們（結束代碼 1）。

<!-- source: splitting-workers-by-job-type 2ec73f050551 -->
### 依任務類型分開工作程序

預設情況下，每個工作程序都會執行所有任務類型，依任務到期的順序，每個程序一次執行一個任務（請參閱[同時執行多個任務](#running-several-jobs-at-once)）。這樣一來，大量的傳送會延誤電子郵件和通知，而你新增的任何工作程序也會把時間分給所有類型。使用 `queue = db` 時，你可以讓某些類型擁有自己的工作程序：

- [`worker_job_types`](../reference/configuration.md#worker_job_types)：這個程序只執行這些類型。
- [`worker_job_types_except`](../reference/configuration.md#worker_job_types_except)：這個程序執行除了這些以外的所有類型。

兩者都是以逗號分隔的[任務類型](../reference/jobs.md)清單（`my job types` 會列出它們；`my config check` 會拒絕不符合任何類型的名稱），以名稱（`federation.deliver`）或前綴（`federation.*`）表示。請依程序設定，例如使用 `MY_WORKER_JOB_TYPES`。同一個程序不能同時設定兩者。不符合任何任務類型的名稱或前綴會中止啟動，這也能抓出被某個版本改名的類型，或只有在你尚未設定的某項設定下才存在的類型。它們不適用於 `role = web`，也不適用於 `queue = cloudtasks` 或 `inline`，在這些情況下啟動也會失敗：那裡是由訊號指定要執行的任務，所以程序無法選擇自己的任務。

啟動時，設定了其中一項的程序會記錄它略過的類型。它看不到其他程序的設定，所以 **請確認每種類型都有某個程序執行**，否則那些任務會永遠等待。定期清理和電子郵件也是任務類型。每個程序仍會啟動定期任務；由哪個程序執行它們取決於這些設定。

請依據指標而不是類型名稱來決定要分開什麼。`my.job.wait`（Prometheus：`my_job_wait_seconds`）會依類型顯示任務在開始前等待了多久：

```promql
histogram_quantile(0.95, sum by (my_job_type, le) (rate(my_job_wait_seconds_bucket[15m])))
```

`sum by (my_job_type) (rate(my_job_duration_seconds_sum[1h]))` 會顯示每種類型使用的工作程序時間，這也大致代表它需要多少個程序。佔用大部分時間並讓其他類型等待的類型，就是應該移出去的那一個。

兩種常見的配置，程序數量由你決定：

| 配置 | 程序 |
| --- | --- |
| 避免傳送延誤其他任務 | 傳送用 `worker_job_types = federation.fanout,federation.deliver`，其他所有任務用 `worker_job_types_except = federation.fanout,federation.deliver`。大量傳送時，為第一組增加程序。 |
| 避免搜尋後端延誤其他任務 | 搜尋任務用 `worker_job_types = search.*`，其他所有任務用 `worker_job_types_except = search.*`（[避免搜尋延誤其他任務](search.md#keeping-search-from-delaying-other-jobs)）。 |
| 讓電子郵件和通知保持快速 | 這些類型用 `worker_job_types = identity.verification_email,identity.password_reset_email,identity.password_notice_email,identity.closure_email,identity.deactivation_email,notification.email,notification.moderator_email,notification.supporter_email,notification.push,notification.post_subscribers,instance.notify_announcement,voice.notify_started`，再加上一個沒有設定的工作程序處理所有任務，它在空閒時也會協助處理這些類型。請略去你尚未設定之設定項目的類型，否則啟動會中止：`notification.push` 需要 `vapid_private_key`，而 `notification.email`、`notification.moderator_email` 和 `notification.supporter_email` 需要 `mail`。請參閱[依賴設定的任務](../reference/jobs.md#jobs-that-depend-on-settings)。 |

像 `identity.*` 這樣的前綴代表擁有這些類型的程式碼模組，而不是一種工作。`identity.*` 也會包含資料匯出、匯入和到期任務，卻漏掉 `instance.notify_announcement` 和 `voice.notify_started`，所以針對某一種工作的配置會逐一指名每個類型。

請把以下類型放在一起，交給設定相同的工作程序。同一個排序鍵的任務無論類型為何都會依序執行，所以如果沒有程序執行較早的任務，較晚的任務也會等待；如果較早任務所屬的群組積壓，較晚的任務就會等它：

| 類型 | 維持順序的對象 |
| --- | --- |
| `federation.fanout`、`federation.deliver` | 喜歡及其撤銷的傳送 |
| `voice.apply_media_event`、`voice.apply` | 同一個語音聊天室的事件 |
| `supporters.sync`、`supporters.command` | 同一個訂閱的變更 |

升級時：

- 設定了 `worker_job_types` 的程序不會執行新版本新增的類型。請查看版本發布說明和日誌中被略過的類型。設定了 `worker_job_types_except` 的程序會執行它們，所以它是處理其他所有任務較安全的地方。
- 被改名的類型會中止啟動，所以請先閱讀版本發布說明。

要推出分開的配置，請先啟動新的工作程序，再把設定加到舊的工作程序上。要還原，請先從一個群組移除設定，再停止另一個群組。任務會留在佇列中，所以不會遺失任何東西，暫時沒有程序執行的類型只會等待。

**偵測沒有任何程序執行的類型**。這種類型的任務會等待，所以該類型的 `my.queue.oldest_ready_age` 會增長，而 `MyQueueBacklog` 和 `MyQueueStalled` 警示會針對等待最久的類型觸發。若要查看是哪一個，或依類型設定警示：

```promql
max by (my_job_type) (my_queue_oldest_ready_age_seconds) > 1800
```

管理介面的任務頁面會依類型顯示相同的等待時間。設定了其中一項的工作程序也不會處理完全沒有處理常式之類型的任務（它在指標中計入 `other`），所以如果沒有不帶設定的工作程序，這種任務會一直等待，而不會變成 dead 任務。

<!-- source: running-several-jobs-at-once 91e5181261b2 -->
### 同時執行多個任務

[`worker_job_concurrency`](../reference/configuration.md#worker_job_concurrency) 設定一個程序同時執行多少個任務，範圍為 1 到 128。它適用於 `queue = db`；使用 Cloud Tasks 時由平台決定。預設是一次一個任務，設為 1 的行為也相同。

對於大部分時間都在等待其他伺服器的任務（例如 `federation.deliver`），請調高這個值。這類任務每個只使用很少的記憶體和資料庫連線，所以一個程序就能做許多程序的工作：在一項本機測試中，任務只等待 200 毫秒的回應，一個有 32 個槽位的程序與 32 個各有一個槽位的程序表現相當，而只使用了一小部分的記憶體。那些任務沒有做其他任何事，所以請在你自己的實例上量測。請搭配 `worker_job_types` 使用，並讓每種任務有一個設定了自己數值的程序。

請記住以下限制：

- **資料庫連線**：任務在被取得、執行自己的工作以及記錄結果時都需要連線。使用 `database = postgres` 時，這個設定必須低於連線池的上限，否則伺服器不會啟動。上限是 `database_url` 中的 `pool_max_conns`，預設為 4 和 CPU 數之中較大者，所以在你調高之前，一個 vCPU 允許 3。請確認你的資料庫或連線池管理工具能接受所有程序的連線。
- **SQLite**：它一次只寫入一個交易。超過 32 時伺服器會記錄警告，在一項本機測試中，SQLite 在 128 時比 32 時還慢。
- **耗用大量 CPU 的任務**：槽位共用一個程序的 CPU 和記憶體。`media.process` 預設會在 CPU 數一半的上限下等待輪到自己，而等待時間會計入任務 5 分鐘的租約，所以充滿 `media.process` 的程序可能會超過租約而重新開始。請讓這類任務使用槽位較少的程序。
- **一次一台伺服器**：一個程序對任何一個主機最多建立 8 個連線，所以無論有多少槽位，一個程序對單一伺服器的傳送速度都不會超過 8 個連線所允許的速度。在大部分傳送都送往少數大型伺服器的情況下，增加程序有幫助，增加槽位則沒有。
- **卡住的伺服器會佔住槽位**：等待緩慢伺服器的槽位無法供其他任務使用，所以請把傳送與電子郵件和通知分開。
- **部署會消耗嘗試次數**：伺服器停止時，超過 `ShutdownTimeout`（預設 25 秒）仍在執行的任務會被取消，每個被取消的任務會消耗一次嘗試。執行緩慢任務的槽位越多，每次部署消耗的嘗試次數就越多。結果沒有及時記錄的被取消任務，會等到租約到期才再次執行。
- **當機會帶走所有槽位**：如果程序記憶體不足，它所有的任務都會等到租約到期。

要決定是否調高，請依 `my.job.type` 查看 `my.job.wait`：如果某類型的任務等待很久，而 `my.job.duration` 大多是在等待其他伺服器，且 CPU 和記憶體維持在低水準，增加槽位應該會有幫助。如果調高後等待時間仍然很長，請先查看上述的資料庫連線和主機限制，再考慮繼續調高。

沒有到期的任務時，一個槽位會輪詢，其他槽位則等待，所以閒置的程序對資料庫造成的負擔與一次執行一個任務的程序相同。

<!-- source: waiting-for-a-database-connection f481d577ea58 -->
### 等待資料庫連線

使用 `database = postgres` 時，每個陳述式和交易都會先等待程序連線池中的空閒連線。[`database_acquire_timeout`](../reference/configuration.md#database_acquire_timeout)（預設 `60s`）只限制這段等待時間，不限制其他任何事：已經取得連線的長時間查詢或交易會繼續執行，無論需要多久。`0` 表示無限期等待。SQLite 沒有需要等待的連線數上限，所以在 SQLite 上設定它會在啟動時失敗。

等待逾時時：

- 請求會以 `503 Service Unavailable` 附 `Retry-After: 5` 回應。
- 任務會在 30 秒後再次執行，且不消耗嘗試次數，所以忙碌的資料庫絕不會讓任務變成 dead 任務。
- `db.client.connection.timeouts` 指標會計入一次，並觸發 [`MyDBPoolWaiting`](alerts.md#mydbpoolwaiting) 警示。

預設值是伺服器 60 秒的寫入逾時：等待更久的請求反正也無法送出回應，所以預設值不會讓任何原本還能成功的請求失敗。請依照你的實例的運作方式選擇值：

- **調低它** 可以快速卸載負載：用戶端會在 `Retry-After` 之後重試，而不是讓請求一直保持開啟。請讓它比你最慢的一般交易更長，否則短暫的流量高峰就會開始回應 `503`。
- **先調整連線池的大小**。一個程序需要的連線數大約等於它同時執行的工作數。`database_url` 中的 `pool_max_conns` 預設為 4 和 CPU 數之中較大者，而所有程序合計必須低於資料庫的連線上限。
  - 使用 [`worker_job_concurrency`](#running-several-jobs-at-once) 時，槽位也會等待連線，而且這個設定必須低於連線池的上限。
  - 在 Cloud Run 上，服務預設每個執行個體接受 80 個同時請求。連線池為 4 時，一波緩慢的請求會在這裡排隊，所以請調高 `pool_max_conns`、降低服務的並行數，或兩者都做。
- **頻繁出現 `503` 或 `db.client.connection.timeouts`** 表示連線池太小或查詢變慢了；更長的逾時只會掩蓋問題。請依照 [`MyDBPoolSaturated`](alerts.md#mydbpoolsaturated) 處理。

<!-- source: cloud-tasks ef3f7d7cc896 -->
### Cloud Tasks

[在 Cloud Run 上部署](deploy-cloud-run.md)會以 OpenTofu 模組設定以下所有項目。

使用 `queue = cloudtasks` 時，任務留在資料庫中（`database = postgres`），Google Cloud Tasks 只負責為每個任務喚醒伺服器：

- 對於伺服器儲存、重試或放回的每個任務，伺服器會在交易提交後於 `queue_cloudtasks_queue` 中建立一個 Cloud Tasks 工作。這個工作會在任務到期時，以任務的 ID 為內文，帶著 `queue_cloudtasks_service_account` 的 OIDC ID 權杖呼叫 `POST {queue_cloudtasks_url}/_internal/jobs`。到期時間在 30 天以後的任務，會在 30 天時發出訊號，並在該工作送達時再次發出訊號。
- 收到工作的程序會執行任務，並在記錄其結果後回應 `204`，即使任務已經執行過或尚未到期也一樣。失敗的任務會由伺服器在下一次嘗試時間重試，而不是由 Cloud Tasks 重試。Cloud Tasks 只在回應為 `5xx`（無法連線到資料庫）或 `401`（伺服器拒絕的權杖）時才會重試工作，所以它的 `retryConfig` 不需要調整。
- 沒有程序會輪詢資料表，也沒有程序會自行執行定期任務。請建立一個 Cloud Scheduler 工作，每分鐘（`* * * * *`）以同一個服務帳號的 OIDC 權杖（設定了 `queue_cloudtasks_audience` 時，並帶有該對象）呼叫 `POST {queue_cloudtasks_url}/_internal/tick`。每次 tick 都會把已到週期的定期任務放入佇列，整個實例只放一次（週期會在 KV 中被佔用，所以 `kv` 必須是 `db` 或 `valkey`），並再次為已到期超過 2 分鐘仍未執行的任務發出訊號，以補救遺失的訊號。
- `/_internal/jobs` 和 `/_internal/tick` 由 `role = all` 或 `worker` 的程序提供，只接受由 Google 為 `queue_cloudtasks_service_account` 簽署、電子郵件已驗證，且對象為 `queue_cloudtasks_audience`（預設為被呼叫的 URL）的權杖。其他任何請求都是 `401`。
- 一次投遞執行一個任務，最多持續 5 分鐘的租約。請讓 `queue_cloudtasks_dispatch_deadline`（預設 10 分鐘）和 Cloud Run 服務的請求逾時比這更長。
- 伺服器自己的服務帳號需要佇列上的 `roles/cloudtasks.enqueuer`，以及 `queue_cloudtasks_service_account` 上的 `roles/iam.serviceAccountUser`。它以 Application Default Credentials 進行驗證。

<!-- source: routing-job-types-to-their-own-queues 292b3f6071a8 -->
### 把任務類型路由到各自的佇列

使用 `queue = cloudtasks` 時，工作程序無法選擇它執行的任務：`worker_job_types` 只適用於 `queue = db`，因為由平台決定誰接收訊號。若要讓某些任務類型擁有自己的容量，請把它們的訊號送到它們自己的佇列，並在平台上設定該佇列的容量。只要你沒有設定路由，就不會有任何改變。

[`queue_cloudtasks_routes`](../reference/configuration.md#queue_cloudtasks_routes)：`federation.*=projects/p/locations/l/queues/my-federation@https://federation-worker.example.com,notification.email=projects/p/locations/l/queues/my-mail`。沒有 `@` 和 URL 的路由會呼叫 `queue_cloudtasks_url`。

左邊是[任務參考](../reference/jobs.md)中的任務類型或前綴。名稱優先於前綴，較長的前綴優先於較短的前綴。不符合任何路由的類型（包括之後的版本新增的類型）會使用預設佇列（`queue_cloudtasks_queue`），所以它們會繼續執行。`my config check` 會檢查路由的格式；不符合任何任務類型的左邊會讓 `my serve` 在啟動時中止。

**讓每個程序都使用相同的路由，包括 `role = web`**。訊號由儲存任務的程序發出：使用者大部分的操作（例如貼文的第一個任務）由 web 程序發出，而任務所儲存的任務、重試、排序鍵的下一個任務，以及遺失訊號的清查，則由工作程序發出。在每種角色有各自設定的環境中（例如 Cloud Run 服務），請把相同的值傳給所有角色：

```sh
routes='federation.*=projects/p/locations/l/queues/my-federation@https://federation-worker.example.com'
gcloud run services update my-web --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
gcloud run services update my-worker --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
gcloud run services update my-federation-worker --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
```

路由不同的程序不會產生錯誤，也不會遺失任務：它的訊號會送到另一個佇列，任務會在那裡、在你設定的容量之外執行。`my.queue.oldest_ready_age` 不會顯示這種情況，因為那些任務沒有在等待。為了比較各程序，每個程序都會在啟動時記錄一行，內含其 `routes` 及其 `routes_hash`：這個雜湊值在每個地方都必須相同。若要查看訊號送往何處，請查看平台本身每個佇列的指標。

**權限**。每個程序的憑證都需要能送到路由中的每個佇列：`roles/cloudtasks.enqueuer`。傳送失敗會被記錄下來，而不會改送到其他佇列，以免你所限制的類型溢出到其他佇列。這些任務會延後開始，等清查在 2 分鐘後再次發出它們的訊號。

**容量**。任何接收端都能執行任何任務，所以由你決定要把它們分開到什麼程度：

| 平台 | 單一接收端 | 分開的接收端 |
| --- | --- | --- |
| Google Cloud | 一個服務。每個佇列有自己的 `maxConcurrentDispatches` 和 `maxDispatchesPerSecond`；服務的執行個體上限是共用的 | 每個佇列一個服務，由路由中的 URL 指定，各有自己的執行個體上限和並行數。每個服務都把 `queue_cloudtasks_url` 設為自己的 URL |

佇列需要與預設佇列相同的設定：佇列上的角色。只有預設佇列需要 tick。

**佇列增加的負擔**。每個佇列都是多一個需要建立、監看和付費的東西，這有多重要取決於你的實例規模。請只把你在 `my.job.wait` 中看到在等待的類型分開，而不是全部分開。

**清查**。清查每分鐘都會尋找已到期超過 2 分鐘仍未執行的任務，每個路由最多 500 個，預設佇列也最多 500 個，並再次為它們發出訊號。因此，跟不上的佇列每分鐘最多會收到 500 個重複的訊號。它們不會讓任務執行兩次，但它們是你需要付費的訊息，而且在積壓清除後就會停止。

**排序鍵** 仍然有效：由資料庫決定什麼可以執行。同一個鍵的任務可能位於不同的佇列，但停滯的佇列也會延誤其鍵中較晚的任務。

**新增、移動和移除佇列**。

1. 建立佇列及其接收端，並讓每個程序都有權限傳送到它。
2. 在每個程序上變更路由。在路由不一致的期間，訊號會送到任一個佇列，任務會在兩個地方執行。
3. 要移除佇列，請先從每個地方的路由中拿掉它，並等它清空後再刪除。被刪除之佇列中的訊號會遺失，但任務不會：一旦任務到期滿 2 分鐘，清查就會再次為它發出訊號。Cloud Tasks 的工作可以為最多 30 天後才到期的任務保存訊號，所以到期時間晚於等待時間的任務，只會在時候到了時由清查發出訊號。

<!-- source: health-checks 06a8834ade0d -->
## 健康檢查

| 路徑 | 種類 | 回傳 200 的條件 |
| --- | --- | --- |
| `GET /healthz` | 存活 | 程序能回應 HTTP。它不會檢查資料庫等相依項目（所以緩慢的資料庫不會讓程序重新啟動） |
| `GET /readyz` | 就緒 | 能連線到資料庫，且已套用結構描述 |
| `GET /api/v1/instance/health` | 運作時間監控、狀態頁面和用戶端（由 `web` 角色提供） | 與就緒相同。結果會在程序中快取 60 秒 |

- `/healthz` 或 `/readyz` 回傳 200 時，內文為 `{"status":"ok"}`。
- `/readyz` 每次請求都會檢查資料庫，同時的請求共用一次檢查。
- `/api/v1/instance/health` 正常時回傳 200 和 `{"status":"ok","checked_at":"<RFC 3339>"}`，失敗時回傳 503 和 `{"status":"unavailable","checked_at":...}`（`Cache-Control: no-store`）。無論被監控或請求的頻率多高，每個程序每分鐘最多存取資料庫一次，而且它既不檢查存取權杖，也不檢查速率限制。請讓運作時間監控指向這個端點，而不是探測用的端點（`/readyz`）。各服務的設定方式、監控間隔與費用，以及維護的處理方式，請參閱[運作時間監控指南](monitoring.md)。
- 這三者在存取日誌中都以 Debug 等級記錄，也不會建立追蹤 span。
- 未達就緒時，回應為 503，內文為 RFC 9457 的 `application/problem+json`。其 `title` 會依 `Accept-Language` 翻譯，不包含任何內部細節。原因（連線錯誤，或缺少的資料表名稱）會記錄在日誌中。
- 一次就緒檢查會在 3 秒後中斷。
- 就緒還要求資料庫中保存著設定的 `host` 和 `account_domain`，或程序能夠記錄它們。
- **已套用結構描述** 表示資料庫的結構描述檔案（`schema/<NN>-<module>/<dialect>.sql`）以 `CREATE TABLE` 宣告的每個資料表都存在（SQLite 透過 `sqlite_schema` 檢查，PostgreSQL 透過連線的 `current_schema()` 檢查）。`my schema apply` 會套用整個結構描述，所以缺少資料表表示尚未套用結構描述，或結構描述比二進位檔舊。欄位和索引的差異不會檢查；`my schema apply -dry-run` 會顯示將會變更的內容。

<!-- source: requests-that-bypass-the-cdn 2cca4127af07 -->
## 繞過 CDN 的請求

當伺服器位於 CDN 後方，而它自己的位址仍然公開時，請設定 `origin_secret` 和 `origin_secret_header`，讓它拒絕沒有經過 CDN 的請求。健康檢查仍然可以連線。請參閱[拒絕沒有經過 CDN 的請求](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)。

<!-- source: security-headers 8c5e82848149 -->
## 安全性標頭

每個回應都帶有 `Strict-Transport-Security`、`X-Content-Type-Options`、`Referrer-Policy`、`Permissions-Policy`、`Cross-Origin-Opener-Policy`、`Cross-Origin-Resource-Policy: same-site` 和 `X-Frame-Options`，而非文件的回應還帶有 `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'; base-uri 'none'`。對於需要其他值的回應（例如 ActivityPub JSON 和公開媒體），處理常式會覆寫這些標頭。`Permissions-Policy` 會拒絕相機、地理位置、付款和 USB。它也會拒絕麥克風，但語音聊天開啟時（`voice` 不是 `off`）除外：這時它只允許伺服器自身來源的頁面使用麥克風（`microphone=(self)`），讓瀏覽器能在通話中發言。網頁用戶端的 HTML 會取得不使用 nonce 的文件 CSP（`script-src 'self'` 等）。它要求 Trusted Types（`require-trusted-types-for 'script'`），並把可以建立的政策限制為用戶端的兩個（`trusted-types my-server-sanitized-html my-service-worker`）。

<!-- source: web-client f35b79dd2cc3 -->
## 網頁用戶端

`role` 為 `web` 或 `all` 的程序會提供嵌入在二進位檔中的網頁用戶端。

| 路徑 | 內容 | `Cache-Control` |
| --- | --- | --- |
| 沒有其他路由符合的任何 `GET`（`/`、`/@alice`、`/@alice/status/{id}`、`/settings` 等） | SPA 的 HTML 外殼。實例設定透過 `<script type="application/json" id="config">` 傳遞 | `public, max-age=60, stale-while-revalidate=300`、ETag |
| `GET /assets/*` | 名稱帶有雜湊值的建置檔案 | `public, max-age=31536000, immutable`、ETag |
| `GET /manifest.webmanifest` | 根據實例設定建立的網頁應用程式資訊清單 | `public, max-age=60, stale-while-revalidate=300`、ETag |
| `GET /sw.js`、`/icon.svg` 以及建置目錄下的其他檔案 | 原樣提供 | `no-cache`、ETag |
| `GET /licenses/*` | 建置所包含之內容的授權（npm 套件的聲明，以及內附的非 npm 資料所用的 `LICENSE-Unicode.txt` 和 `OFL.txt`）。無論副檔名為何，都以 `text/plain; charset=utf-8` 提供 | `no-cache`、ETag |
| `GET /robots.txt`、`/.well-known/tdmrep.json`、`/.well-known/security.txt` | 根據實例設定建立（請參閱[搜尋引擎與 AI 爬蟲](crawlers.md)） | `public, max-age=3600`、ETag |

- `/api`、`/oauth`、`/auth`、`/.well-known`、`/nodeinfo`、`/ap`、`/media`、`/uploads`、`/theme`（伺服器佈景主題的樣式表）和 `/licenses` 底下的路徑，以及 `/robots.txt`，絕不會回傳外殼；如果沒有其他路由符合，它們會回傳 404 問題文件。
- 外殼會在 ActivityPub 內容協商之後提供。以 ActivityPub 的 `Accept` 請求外殼路徑會回傳 404（貼文頁面會重新導向，如下所述）。外殼一律帶有 `Vary: Accept`。在前端放置 CDN 時，請把 `Accept` 的種類（ActivityPub 或 HTML）納入快取鍵，或把外殼、`/ap/*` 和 `/@*` 排除在快取之外。只從 CDN 提供 `/assets/*`。
- 沒有包含網頁用戶端而建置的二進位檔（單純執行 `go build`，而沒有先建置用戶端）會在啟動時記錄警告，並提供不含指令碼的外殼。

<!-- source: federation-endpoints 7aaee8c9da42 -->
## 聯邦端點

`role` 為 `web` 或 `all` 的程序會回應以下端點，其他聯邦宇宙伺服器會用它們來尋找和顯示你的帳號。

| 路徑 | 內容 |
| --- | --- |
| `GET /.well-known/webfinger` | WebFinger（RFC 7033）。`resource` 是 `acct:alice@{account_domain}`（`acct:alice@{host}` 也可以）、Actor 的 `id`，或 `/@alice` URL。`subject` 一律是 `account_domain` 端的帳號代稱 |
| `GET /.well-known/host-meta`、`/.well-known/host-meta.json` | WebFinger 的 URI 範本（XRD / JRD） |
| `GET /.well-known/nodeinfo`、`GET /nodeinfo/2.1` | NodeInfo 2.1。`metadata.sourceCode` 是設定 `source_url`，若為 `https:` URL，也會設定 `software.repository`（請參閱[公開原始碼](#publish-the-source-code-agpl-30)） |
| `GET /ap/actors/{uuid}` | Actor（`publicKey` 是 RSA 的 `#main-key`）。偏好 HTML 的請求會收到指向 `/@{username}` 的 303 |
| `GET /ap/actors/{uuid}/{outbox,followers,following,featured,featured-tags}` | 集合 |
| `POST /ap/actors/{uuid}/inbox`、`POST /ap/inbox` | 每個本站 actor 的收件匣和共用收件匣。請求必須帶有 HTTP 簽章 |
| `GET /@{username}` | ActivityPub 的 `Accept` 會收到指向 Actor `id` 的 302；否則為網頁用戶端的 HTML（請參閱下文） |
| `GET /@{username}/status/{uuid}` | 對於 ActivityPub 的 `Accept`，如果未登入的訪客可以看到該貼文，回傳指向其 `id`（`/ap/objects/{uuid}`）的 302，否則回傳 404。其他情況為網頁用戶端的 HTML |

- 實例 Actor 的帳號代稱是 `{account_domain}@{account_domain}`（與 Mastodon 的形式相同）。
- 不存在或無法顯示的帳號（已停權、已停用、已刪除或遠端帳號）一律收到相同的 404（`Cache-Control: no-store`）。
- 匿名回應帶有 `Cache-Control: public, max-age=60`。依 `Accept` 而不同的回應帶有 `Vary: Accept`。對於無法處理 `Vary: Accept` 的 CDN，請把 `/ap/*` 和 `/@*` 排除在快取之外。

<!-- source: separate-handle-and-delivery-domains b1c23d994d0e -->
### 分開帳號代稱網域與傳送網域

當 `account_domain` 與 `host` 不同時，對 `account_domain` 端的 `/.well-known/webfinger`、`/.well-known/host-meta` 和 `/.well-known/nodeinfo` 的請求會以 301 重新導向到 `host`（帶有 `Access-Control-Allow-Origin: *`）。如果你把 `account_domain` 的請求路由到這台伺服器，伺服器會自己回傳這個重新導向。如果改由其他伺服器或反向代理回應，請保留路徑和查詢字串，重新導向到 `https://{host}`。
