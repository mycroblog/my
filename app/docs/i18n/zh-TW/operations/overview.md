<!-- source: overview-of-a-mycroblog-server ce703d24935f -->
# mycroblog 伺服器概覽

這是執行伺服器之前首先要閱讀的頁面。mycroblog 是一個透過 ActivityPub 與聯邦宇宙其他部分互相聯邦的微網誌；`my` 是它的伺服器軟體，單一個執行檔就包含了伺服器、任務 worker、網頁用戶端，以及維護它們的指令。本頁說明伺服器由哪些部分組成、哪些部分可以透過設定選擇、小型伺服器如何成長為大型伺服器，以及接下來該讀什麼。

<!-- source: what-a-server-is-made-of 1210fe6ac0c4 -->
## 伺服器由哪些部分組成

![mycroblog 伺服器的架構：用戶端透過 HTTPS 連到 my 程序；程序使用透過設定選擇的元件，以及你開啟的伺服器外部服務。下方各節會說明每個部分。](images/architecture.svg)

由圖的上方往下：

- **誰會連線**。使用者在瀏覽器中使用網頁用戶端（可以安裝成應用程式），應用程式和機器人使用用戶端 API，聯邦宇宙中的其他伺服器則透過 ActivityPub 傳送和擷取貼文。它們全都透過 HTTPS 連到伺服器。
- **伺服器前端**。`my` 使用一般的 HTTP，因此需要在它前面放置反向代理或負載平衡器來終止 HTTPS。媒體檔案前面的 CDN 則是選用的。
- **`my` 程序**。每個程序都執行 `my serve`，由它的 [`role`](../reference/configuration.md#role) 決定要執行什麼：`web` 是 HTTP 伺服器；`worker` 是任務 worker，負責傳送貼文、從其他伺服器擷取內容、寄送電子郵件、處理媒體，以及執行定期任務；預設的 `all` 則在一個程序中同時執行兩者。
- **你選擇的元件**。圖中間的每個元件都有一個用來選擇其轉接器的設定，列在[下一節](#parts-you-choose-by-configuration)。
- **伺服器外部的服務**。CAPTCHA、語音聊天、付款、外掛和雲端費用匯入，在你設定之前都保持關閉；設定之後，伺服器會呼叫你指定的服務。

<!-- source: parts-you-choose-by-configuration f7dbfce7a91c -->
## 透過設定選擇的元件

每個元件都是[設定參考](../reference/configuration.md)中的一個設定，可以寫在 TOML 檔案中，或以環境變數設定（`MY_` 加上大寫的鍵名，例如 `MY_DATABASE`）。各列表中的第一個選項是預設值。會保存資料的元件，請在第一次啟動前選好：設定改變時，資料不會自動搬移。

| 元件 | 設定 | 選項 | 延伸閱讀 |
| --- | --- | --- | --- |
| 資料庫 | [`database`](../reference/configuration.md#database) | `sqlite`（一個檔案）、`postgres`（PostgreSQL 及相容服務） | [資料庫](database.md)、[備份](backup.md) |
| 任務佇列 | [`queue`](../reference/configuration.md#queue) | `db`（資料庫中的一個資料表）、`inline`（在同一個程序中立即執行每個任務，用於測試和開發）、`cloudtasks`（由 Google Cloud Tasks 為每個任務喚醒伺服器；需要 PostgreSQL） | [任務佇列](running.md#the-job-queue-and-failed-dead-jobs) |
| 鍵值儲存，用於快取和速率限制 | [`kv`](../reference/configuration.md#kv) | `db`（資料庫中的一個資料表）、`memory`（每個程序各自獨立）、`valkey`（Valkey 或其他使用 Redis 協定的伺服器，由所有程序共用） | [`kv`](../reference/configuration.md#kv) |
| 媒體儲存 | [`blob`](../reference/configuration.md#blob) | `fs`（一個目錄）、`s3`（Amazon S3、Cloudflare R2、MinIO、Garage 及其他 S3 相容儲存）、`gcs`（Google Cloud Storage） | [Docker Compose 檔案](running.md#docker-compose-files)、[備份媒體](backup.md#media-in-a-bucket) |
| 媒體處理 | [`media_image`](../reference/configuration.md#media_image)、[`media_video`](../reference/configuration.md#media_video)、[`media_audio`](../reference/configuration.md#media_audio)，每種檔案各一個 | `native`（在伺服器內處理）、`plugin`（由你的外掛轉換該類檔案） | [轉換媒體](plugins.md#converting-media) |
| 程序之間的即時串流 | [`stream`](../reference/configuration.md#stream) | `hub`（在單一程序內）、`postgres`（`LISTEN` / `NOTIFY`）、`valkey`、`sns`（Amazon SNS 與 SQS）、`pubsub`（Google Cloud Pub/Sub） | [`stream`](../reference/configuration.md#stream) |
| 金鑰管理器，用來加密帳號的私密金鑰 | [`key_manager`](../reference/configuration.md#key_manager) | `local`（由你保管的金鑰加密金鑰）、`gcpkms`（Google Cloud KMS）、`awskms`（AWS KMS）、`plugin` | [管理金鑰](key-management.md) |
| 電子郵件 | [`mail`](../reference/configuration.md#mail) | `none`、`smtp`、`ses`（Amazon SES）、`plugin` | [寄送電子郵件](email.md) |
| CAPTCHA | [`captcha`](../reference/configuration.md#captcha) | `none`、`turnstile`（Cloudflare Turnstile）、`hcaptcha`、`recaptcha`（Google reCAPTCHA v2） | [CAPTCHA](captcha.md) |
| 語音聊天 | [`voice`](../reference/configuration.md#voice) | `off`、`p2p`（瀏覽器對瀏覽器，TURN 由 `voice_turn` 提供：`none`、`coturn` 或 `cloudflare`）、`livekit`、`sora` | [語音聊天](voice.md) |
| 支持者付款 | [`payments`](../reference/configuration.md#payments) | `none`、`stripe`、`payjp`（PAY.JP） | [支持者](supporters.md)、[Stripe](payments-stripe.md)、[PAY.JP](payments-payjp.md) |
| 外掛 | `plugin_<kind>_address` | 預設沒有；每種類型一個位址：`translator`、`search`、`billing`、`mailer`、`media_image`、`media_video`、`media_audio`、`key_manager`，以及決策掛鉤 `inbound.status`、`local.status`、`inbound.follow`、`signup.check`、`media.check`、`inbound.actor`、`local.profile` | [外掛](plugins.md)、[決策掛鉤](decision-hooks.md) |
| 貼文翻譯 | `translator_<provider>_*`、[`plugin_translator_address`](../reference/configuration.md#plugin_translator_address) | 無（只有瀏覽器本身的翻譯）；`libretranslate`、`deepl`、`google`（Google Cloud Translation）、`aws`（Amazon Translate），或是放在你所選服務前面的自製外掛 | [翻譯貼文](translation.md)、[翻譯外掛](plugins.md#translating-posts) |
| 貼文全文搜尋 | [`search`](../reference/configuration.md#search) | `none`；或 `elasticsearch`、`opensearch`、`meilisearch`（[搜尋引擎](search-engines.md)）；或 `plugin`，放在其他引擎前面的自製外掛 | [全文搜尋](search.md)、[搜尋外掛](plugins.md#searching-posts) |
| 媒體 CDN | [`media_origin`](../reference/configuration.md#media_origin) | 不設定（伺服器從自己的來源提供媒體），或你的 CDN 的來源 | [透過 CDN 提供媒體](cdn.md) |
| 雲端費用 | [`billing_sources`](../reference/configuration.md#billing_sources) | 無，或 `aws`（AWS Cost Explorer）、`gcp`（Google Cloud 的帳單匯出或預算）、`plugin` 中的任意幾項 | [匯入雲端費用](billing-import.md) |
| 指標 | [`metrics_exporter`](../reference/configuration.md#metrics_exporter) | `none`、`prometheus`、`otlp`（OpenTelemetry）、`emf`（CloudWatch） | [指標](running.md#metrics)、[警示](alerts.md) |
| 追蹤（traces） | [`traces_exporter`](../reference/configuration.md#traces_exporter) | `none`、`otlp` | [追蹤](running.md#traces) |
| 錯誤回報 | [`errors_exporter`](../reference/configuration.md#errors_exporter) | `none`（只有日誌行和錯誤群組）、`sentry`（Sentry、GlitchTip、Datadog）、`otlp` | [錯誤回報](error-reporting.md) |

發行的執行檔和容器映像檔包含表中所有的轉接器。自行建置的執行檔可以用[建置標籤](../reference/build-tags.md)排除不使用的轉接器，這時啟動時會拒絕它們的設定。

`my config check` 會在你啟動伺服器之前，一次回報設定中的所有問題（[正式環境檢查清單](production-checklist.md)）。

<!-- source: from-a-small-server-to-a-large-one 42bbd3f2a76e -->
## 從小型伺服器到大型伺服器

![從小型伺服器到大型伺服器的三個階段：先是使用 SQLite 和檔案系統的單一程序，接著是 PostgreSQL 和位於 CDN 後方的物件儲存，最後是搭配串流代理的獨立 web 與 worker 程序。下方列表會說明每個階段。](images/growth.svg)

1. **從小規模開始**。單一個 `role = all` 的程序，使用 SQLite，媒體放在檔案系統上，其他部分使用預設值：佇列和鍵值儲存放在資料庫中，即時串流在程序內部。`deploy/docker` 中的 Docker Compose 檔案會在一台機器上啟動這種配置（[使用 Docker Compose 部署](deploy-docker-compose.md)）。它適合個人或小型伺服器。`stream = hub` 只能觸及它所在的程序，因此這種配置只能維持單一程序。
2. **把資料移到外部**。資料庫使用 PostgreSQL，媒體使用 S3 相容的 bucket 或 Google Cloud Storage，並在媒體前面放置 CDN。伺服器仍然是單一程序，但不再依賴單一硬碟，下一階段的多個程序也能共用這些資料。可以的話，請在第一次啟動前決定：之後要把資料從 SQLite 和檔案系統搬出來，本身就是一項工程（[要備份什麼](backup.md#what-to-back-up)列出了需要搬移的內容）。
3. **水平擴充**。執行處理請求的 `role = web` 程序和處理任務的 `role = worker` 程序，各自依需要執行多個，並依任務類型分開 worker（[依任務類型分開 worker](running.md#splitting-workers-by-job-type)）。有多個程序時，即時串流需要一個能觸及所有程序的代理（`postgres`、`valkey`、`sns` 或 `pubsub`），鍵值儲存也應該共用（`db` 或 `valkey`），因為 `memory` 會讓每個程序的速率限制各自分開。任務佇列可以留在 PostgreSQL，或改用 Google Cloud Tasks（[Cloud Tasks](running.md#cloud-tasks)）。

[Cloud Run 部署](deploy-cloud-run.md)是介於第二和第三階段之間的配置範例：在 Cloud Run 上執行 `role = all`，PostgreSQL 使用 Neon，媒體放在 Cloud Storage，任務使用 Cloud Tasks，串流使用 Pub/Sub，沒有人使用伺服器時不會有任何執行個體在運作。

<!-- source: where-to-go-next 2f1d25c01f8a -->
## 接下來該讀什麼

請依照以下順序閱讀營運者指南：

1. **開始使用**：[執行伺服器](running.md)或[部署到 Cloud Run](deploy-cloud-run.md)，然後在開放給使用者之前，逐項確認[正式環境檢查清單](production-checklist.md)。
2. **設定實例**：它的[設定](instance-settings.md)、[註冊](registration.md)、[外部登入提供者](login-providers.md)、[電子郵件](email.md)、[CAPTCHA](captcha.md)、[CDN](cdn.md)、[連結預覽](link-previews.md)，以及[搜尋引擎與 AI 爬蟲](crawlers.md)。
3. **開啟功能**：[審核](moderation.md)、[中繼站](relays.md)、[webhook](webhooks.md)、[外掛](plugins.md)與[決策掛鉤](decision-hooks.md)、[語音聊天](voice.md)、[全文搜尋](search.md)、[支持者](supporters.md)及其付款，以及[匯入雲端費用](billing-import.md)。
4. **日常營運**：[監控](monitoring.md)、[警示](alerts.md)、[錯誤回報](error-reporting.md)、[維護](maintenance.md)、[備份](backup.md)、[資料庫](database.md)、[金鑰加密金鑰](key-management.md)，以及[管理員與角色](admin-cli.md)。
5. **法務與結束營運**：[隱私權政策](privacy-policy-template.md)與[處理活動紀錄](processing-record-template.md)範本，以及[結束服務](service-closure.md)。

[設定參考](../reference/configuration.md)列出了所有設定，[任務參考](../reference/jobs.md)則列出了 worker 執行的所有任務。
