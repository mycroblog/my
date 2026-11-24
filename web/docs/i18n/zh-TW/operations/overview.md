<!-- source: overview-of-a-mycroblog-server ce703d24935f -->
# mycroblog 伺服器概覽

這是執行伺服器之前首先要閱讀的頁面。mycroblog 是一個透過 ActivityPub 與聯邦宇宙其他部分互相聯邦的微網誌；`my` 是它的伺服器軟體，單一個執行檔就包含了伺服器、任務 worker、網頁用戶端，以及維護它們的指令。本頁說明伺服器由哪些部分組成、哪些部分可以透過設定選擇、小型伺服器如何成長為大型伺服器，以及接下來該讀什麼。

<!-- source: what-a-server-is-made-of 51fb6c4e4a24 -->
## 伺服器由什麼組成

![mycroblog 伺服器的架構：用戶端透過 HTTPS 連到 my 程序；程序使用由設定選定的各個部分，以及你啟用的伺服器外部服務。下面各節會說明每個部分。](images/architecture.svg)

從圖的上方往下看：

- **誰來連線。** 使用者在瀏覽器中使用網頁用戶端（它可以安裝成應用程式），應用程式和機器人使用用戶端 API，其他聯邦宇宙的伺服器則透過 ActivityPub 傳送和取得貼文。它們都透過 HTTPS 連到伺服器。
- **伺服器前方。** 預設情況下 `my` 說的是純 HTTP，所以要在它前面放一個反向代理或負載平衡器來終結 HTTPS。
  在單一機器上，`my` 也可以自己提供 HTTPS（[由伺服器提供 HTTPS](running.md#serving-https-from-the-server)）。
  媒體檔案前方的 CDN 是選用的。
- **`my` 程序。** 每個程序都執行 `my serve`，由它的 [`role`](../reference/configuration.md#role) 決定執行什麼：`web` 執行 HTTP 伺服器，`worker` 執行工作處理程式，負責傳送貼文、從其他伺服器取得內容、寄送電子郵件、處理媒體並執行定期工作，預設的 `all` 則在同一個程序中兩者都執行。
- **你選擇的部分。** 圖中間的每個部分都有一個設定來選擇它的轉接器，列在[下一節](#parts-you-choose-by-configuration)。
- **伺服器外部的服務。** CAPTCHA、語音聊天、付款、外掛程式和雲端成本的匯入，在你設定之前都保持關閉，設定之後伺服器才會呼叫你指定的服務。

<!-- source: parts-you-choose-by-configuration 34d13fdad524 -->
## 你透過設定選擇的部分

每個部分都是[設定參考](../reference/configuration.md)中的一項設定，可以在 TOML 檔案中設定，也可以用環境變數設定（`MY_` 加上大寫的鍵，例如 `MY_DATABASE`）。每個清單的第一個選項是預設值。
有保存資料的部分，請在第一次啟動前選好：設定改變時，資料不會自己搬移。

| 部分 | 設定 | 選項 | 接著閱讀 |
| --- | --- | --- | --- |
| 資料庫 | [`database`](../reference/configuration.md#database) | `sqlite`（一個檔案）、`postgres`（PostgreSQL 與相容的服務） | [資料庫](database.md)、[備份](backup.md) |
| 工作佇列 | [`queue`](../reference/configuration.md#queue) | `db`（資料庫中的一張資料表）、`inline`（在同一個程序中立即執行每個工作，用於測試和開發）、`cloudtasks`（Google Cloud Tasks 為每個工作喚醒伺服器；需要 PostgreSQL） | [工作佇列](running.md#the-job-queue-and-failed-dead-jobs) |
| 鍵值儲存，用於快取、速率限制，以及對重新送出的寫入保留一天的回應 | [`kv`](../reference/configuration.md#kv) | `db`（資料庫中的一張資料表）、`memory`（各程序各用各的）、`valkey`（Valkey 或其他 Redis 協定的伺服器，由所有程序共用） | [`kv`](../reference/configuration.md#kv) |
| 媒體儲存 | [`blob`](../reference/configuration.md#blob) | `fs`（一個目錄）、`s3`（Amazon S3、Cloudflare R2、MinIO、Garage 與其他 S3 相容的儲存）、`gcs`（Google Cloud Storage） | [Docker Compose 檔案](running.md#docker-compose-files)、[備份媒體](backup.md#media-in-a-bucket) |
| 媒體處理 | [`media_image`](../reference/configuration.md#media_image)、[`media_video`](../reference/configuration.md#media_video)、[`media_audio`](../reference/configuration.md#media_audio)，每種檔案各一項 | `native`（在伺服器內）、`plugin`（由你的外掛程式轉換該種檔案） | [轉換媒體](plugins.md#converting-media) |
| 程序之間的即時串流 | [`stream`](../reference/configuration.md#stream) | `hub`（在單一程序內）、`postgres`（`LISTEN` / `NOTIFY`）、`valkey`、`sns`（Amazon SNS 與 SQS）、`pubsub`（Google Cloud Pub/Sub） | [`stream`](../reference/configuration.md#stream) |
| 金鑰管理員，用來加密帳號的私密金鑰 | [`key_manager`](../reference/configuration.md#key_manager) | `local`（由你保管的金鑰加密金鑰）、`gcpkms`（Google Cloud KMS）、`awskms`（AWS KMS）、`plugin` | [管理金鑰](key-management.md) |
| 電子郵件 | [`mail`](../reference/configuration.md#mail) | `none`、`smtp`、`ses`（Amazon SES）、`plugin` | [寄送電子郵件](email.md) |
| CAPTCHA | [`captcha`](../reference/configuration.md#captcha) | `none`、`turnstile`（Cloudflare Turnstile）、`hcaptcha`、`recaptcha`（Google reCAPTCHA v2） | [CAPTCHA](captcha.md) |
| 語音聊天 | [`voice`](../reference/configuration.md#voice) | `off`、`p2p`（瀏覽器對瀏覽器，TURN 由 `voice_turn` 決定：`none`、`coturn` 或 `cloudflare`）、`livekit`、`sora` | [語音聊天](voice.md) |
| 支持者付款 | [`payments`](../reference/configuration.md#payments) | `none`、`stripe`、`payjp`（PAY.JP） | [支持者](supporters.md)、[Stripe](payments-stripe.md)、[PAY.JP](payments-payjp.md) |
| 外掛程式 | `plugin_<kind>_address` | 預設都沒有；每種類型一個位址：`translator`、`search`、`billing`、`mailer`、`media_image`、`media_video`、`media_audio`、`key_manager`，以及決策掛鉤 `inbound.status`、`local.status`、`inbound.follow`、`signup.check`、`media.check`、`inbound.actor`、`local.profile` | [外掛程式](plugins.md)、[決策掛鉤](decision-hooks.md) |
| 貼文翻譯 | `translator_<provider>_*`、[`plugin_translator_address`](../reference/configuration.md#plugin_translator_address) | 無（只有瀏覽器自己的翻譯）；`libretranslate`、`deepl`、`google`（Google Cloud Translation）、`aws`（Amazon Translate），或是放在你所選服務前面的你自己的外掛程式 | [翻譯貼文](translation.md)、[翻譯外掛程式](plugins.md#translating-posts) |
| 貼文的全文搜尋 | [`search`](../reference/configuration.md#search) | `none`；或 `elasticsearch`、`opensearch`、`meilisearch`（[搜尋引擎](search-engines.md)）；或 `plugin`，放在另一個引擎前面的你自己的外掛程式 | [全文搜尋](search.md)、[搜尋外掛程式](plugins.md#searching-posts) |
| 媒體用的 CDN | [`media_origin`](../reference/configuration.md#media_origin) | 未設定（伺服器從自己的來源提供媒體），或你的 CDN 的來源 | [透過 CDN 提供媒體](cdn.md) |
| 雲端成本 | [`billing_sources`](../reference/configuration.md#billing_sources) | 無，或以下任何幾項：`aws`（AWS Cost Explorer）、`gcp`（Google Cloud 的帳單匯出或預算）、`cloudflare`（Cloudflare 的用量費用）、`plugin` | [匯入雲端成本](billing-import.md) |
| 指標 | [`metrics_exporter`](../reference/configuration.md#metrics_exporter) | `none`、`prometheus`、`otlp`（OpenTelemetry）、`emf`（CloudWatch） | [指標](running.md#metrics)、[警示](alerts.md) |
| 追蹤 | [`traces_exporter`](../reference/configuration.md#traces_exporter) | `none`、`otlp` | [追蹤](running.md#traces) |
| 錯誤回報 | [`errors_exporter`](../reference/configuration.md#errors_exporter) | `none`（只有日誌行和錯誤群組）、`sentry`（Sentry、GlitchTip、Datadog）、`otlp` | [錯誤回報](error-reporting.md) |

發行的執行檔和容器映像檔包含表中的每一個轉接器。

`my config check` 會在你啟動伺服器之前，一次回報設定中的所有問題（[正式環境檢查清單](production-checklist.md)）。

<!-- source: from-a-small-server-to-a-large-one 2f46b66f047c -->
## 從小型伺服器到大型伺服器

![從小型伺服器到大型伺服器的三個步驟：先是一個使用 SQLite 與檔案系統的程序，接著是 PostgreSQL 與位於 CDN 後方的物件儲存，再來是分開的 web 與 worker 程序加上串流代理。下面的清單說明每個步驟。](images/growth.svg)

1. **從小處開始。** 一個 `role = all` 的程序，使用 SQLite，媒體放在檔案系統上，其餘使用預設值：佇列和鍵值儲存在資料庫中，即時串流在程序內。`deploy/docker` 中的 Docker Compose 檔案會在一台機器上啟動這個配置（[用 Docker Compose 部署](deploy-docker-compose.md)）。
   它適合個人或小型伺服器。`stream = hub` 只能到達它所在的那個程序，所以這個配置只能維持單一程序。
2. **把資料搬出去。** 資料庫用 PostgreSQL，媒體用 S3 相容的儲存貯體或 Google Cloud Storage，並在媒體前面放一個 CDN。伺服器仍是一個程序，但不再依賴單一磁碟，下一步的各個程序也就能共用資料。如果可以，請在第一次啟動前決定這件事：之後要把資料從 SQLite 和檔案系統搬出來是另一項工作（[該備份什麼](backup.md#what-to-back-up)列出了必須搬移的內容）。
3. **向外擴充。** 為處理請求執行 `role = web` 的程序，為處理工作執行 `role = worker` 的程序，各需要多少就執行多少，並依工作類型拆分 worker（[依工作類型拆分 worker](running.md#splitting-workers-by-job-type)）。
   有多個程序時，即時串流需要一個能到達每個程序的代理（`postgres`、`valkey`、`sns` 或 `pubsub`），鍵值儲存也應該共用（`db` 或 `valkey`），因為 `memory` 會讓各程序的速率限制和對重新送出的寫入的回應各自獨立。工作佇列仍留在 PostgreSQL，或改用 Google Cloud Tasks（[Cloud Tasks](running.md#cloud-tasks)）。

[Cloud Run 部署](deploy-google-cloud.md)是介於第二步和第三步之間的配置範例：Cloud Run 上的 `role = all`、Neon 上的 PostgreSQL、Cloud Storage 中的媒體、用 Cloud Tasks 處理工作，以及用 Pub/Sub 作為串流，沒有人使用伺服器時不會有執行個體在運作。

<!-- source: where-to-go-next 2f1d25c01f8a -->
## 接下來該讀什麼

請依照以下順序閱讀營運者指南：

1. **開始使用**：[執行伺服器](running.md)或[部署到 Cloud Run](deploy-google-cloud.md)，然後在開放給使用者之前，逐項確認[正式環境檢查清單](production-checklist.md)。
2. **設定實例**：它的[設定](instance-settings.md)、[註冊](registration.md)、[外部登入提供者](login-providers.md)、[電子郵件](email.md)、[CAPTCHA](captcha.md)、[CDN](cdn.md)、[連結預覽](link-previews.md)，以及[搜尋引擎與 AI 爬蟲](crawlers.md)。
3. **開啟功能**：[審核](moderation.md)、[中繼站](relays.md)、[webhook](webhooks.md)、[外掛](plugins.md)與[決策掛鉤](decision-hooks.md)、[語音聊天](voice.md)、[全文搜尋](search.md)、[支持者](supporters.md)及其付款，以及[匯入雲端費用](billing-import.md)。
4. **日常營運**：[監控](monitoring.md)、[警示](alerts.md)、[錯誤回報](error-reporting.md)、[維護](maintenance.md)、[備份](backup.md)、[資料庫](database.md)、[金鑰加密金鑰](key-management.md)，以及[管理員與角色](admin-cli.md)。
5. **法務與結束營運**：[隱私權政策](privacy-policy-template.md)與[處理活動紀錄](processing-record-template.md)範本，以及[結束服務](service-closure.md)。

[設定參考](../reference/configuration.md)列出了所有設定，[任務參考](../reference/jobs.md)則列出了 worker 執行的所有任務。
