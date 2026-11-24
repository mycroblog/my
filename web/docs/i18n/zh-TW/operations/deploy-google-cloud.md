<!-- source: deploying-on-cloud-run 7728d9a37c56 -->
# 部署到 Cloud Run

本頁說明如何使用 OpenTofu 模組 [`mycroblog-gcp`](../../deploy/opentofu/README.md)，在 Google Cloud 上執行 `my`：使用 Cloud Run、Cloud Tasks、Cloud Scheduler、Cloud Storage 和 Pub/Sub，搭配 Neon 上的 PostgreSQL 資料庫，並置於全域外部 Application Load Balancer 之後。沒有人使用時，服務不會執行任何執行個體，只有每分鐘喚醒伺服器一次的 tick 例外。伺服器是要花錢的：在儲存空間、流量和開啟中的串流之外，預計每月約 $37，可以透過用心調整降低多少費用，請參閱[費用](#costs)。

這個模組需要 OpenTofu 1.11 以上版本。不支援 Terraform。

<!-- source: what-it-creates cae1e55a1722 -->
## 建立的資源

`mycroblog-gcp` 的一個區塊就會建立以下所有內容。每一列是模組的一個區段，其預設值就是本指南所部署的；其他種類列在[模組的 README](../../deploy/opentofu/README.md#sections)。

| 部分 | 區段 | 資源 |
| --- | --- | --- |
| 伺服器 | `service`、`jobs` | 一個 `role = all` 的 Cloud Run 服務、一個遷移任務、需要時才存在的主控台任務、一個服務帳號，以及公開存取權 |
| 流量 | `ingress` | 一個帶有憑證的全域外部 Application Load Balancer。網域對應或 Cloudflare 是其他種類，而 Cloudflare 上的媒體主機可以與任何一種並用 |
| 任務 | `queue` | 一個 Cloud Tasks 佇列、一個 Cloud Scheduler 任務，以及呼叫伺服器的服務帳號 |
| 媒體 | `media` | 一個私人 bucket |
| 即時更新 | `stream` | 一個主題和一個自訂角色。伺服器會自行建立和移除自己的訂閱 |
| 資料庫 | `database` | 資料庫 URL 的機密：資料庫由你手動建立 |
| 電子郵件 | `mail` | SMTP 密碼的機密：任何 SMTP 服務皆可 |
| 機密 | | 產生的 VAPID 金鑰，以及只有你能填入、目前為空的機密 |
| KEK | `kek` | 以檔案掛載的產生機密。Cloud KMS 是另一種 |
| 指標、追蹤、錯誤 | `observability` | Cloud Monitoring、Cloud Trace 和 Error Reporting 的 API，它們會讀取 Cloud Run 送給它們的內容 |

機密的值永遠不會進入 OpenTofu 的 plan 或 state：來自其他地方的值由你自己加入，KEK 和 VAPID 金鑰則由模組透過唯寫（write-only）屬性產生。

<!-- source: before-you-start 10846ce6f3b1 -->
## 開始之前

- 執行 apply 的地方要有 `tofu` 1.11 以上版本、`gcloud` 和 `jq`：變更版本的 apply 會從那裡透過 `gcloud` 執行遷移。
- 一個啟用計費、只供這個實例使用的 Google Cloud 專案，以及一個在該專案上擁有 Owner 角色的帳號。
- 一個用於 [`host`](../reference/configuration.md#host) 的網域。之後無法變更：請參閱[之後無法變更的設定](production-checklist.md#settings-you-cannot-change-later)。
- 來自[發行版本](running.md#get-the-image)的 `my` 版本。沒有 `latest` 標籤。

<!-- source: first-deployment 996fcc6eb35e -->
## 首次部署

<!-- source: 1-enable-the-apis c7f2ef4a9ee9 -->
### 1. 啟用 API

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com \
  monitoring.googleapis.com
```

`compute.googleapis.com` 是負載平衡器所需要的。

<!-- source: 2-create-the-database-on-neon ca5f51e8979e -->
### 2. 在 Neon 上建立資料庫

請預期使用 Launch 方案：使用預設的 tick 時，運算資源永遠不會停止。Free 方案也可以使用，但有[費用](#costs)中所述的取捨。

在 Neon 主控台中，於靠近你 Cloud Run 區域的區域建立專案，並複製它的連線字串。模組不會建立它，因此它的密碼不會進入 OpenTofu 的 state。一個專案有兩種連線字串，部署會在各自適合的地方使用：

- **直接**連線字串（主機名稱不含 `-pooler`）是必要的。遷移任務和主控台一律使用它：`my schema apply` 會持有工作階段層級的鎖，而交易模式的連線集區不會保持這個鎖。
- **集區**連線字串（主機名稱含 `-pooler`）是選用的，供服務使用。Neon 的連線集區能處理伺服器所需的一切：請參閱[透過連線集區](database.md#through-a-connection-pooler)。設定 `database = { pooled = true }`，模組就會為它另外建立一個機密。

沒有集區連線字串時，服務也會使用直接連線字串，受資料庫的連線上限限制，而這由 `service.max_instances` 決定上限。若要針對 Cloud Run 執行個體一次接受的 80 個請求，把連線池從每個執行個體預設的 4 個調高，請在服務使用的連線字串加上 `pool_max_conns`，例如 `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8`。請參閱[等待資料庫連線](running.md#waiting-for-a-database-connection)。

<!-- source: 3-copy-the-example-and-set-the-variables 1b99f37cb3a7 -->
### 3. 複製範例並設定變數

從你要部署的發行版本中，把範例連同 provider 的鎖定檔複製到一個空目錄，從那裡開始：

```sh
VERSION=v0.1.0
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r my-source/deploy/opentofu/examples/gcp-cloud-run-neon/. my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

`main.tf` 是 `google` provider 加上一個區塊，`module "mycroblog"`。**把 `ref` 設為該發行版本。** 這個區塊以 `?ref=` 指定發行版本，穩定版的副本已經設定正確。伺服器映像檔是同一個發行版本，因此只要 `ref` 就能決定執行的內容。若是預先發行版本，請自行設定兩處：`sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`，並在區塊中設定 `image = { version = "0.1.0-rc.4" }`，版本不帶 `v`，因為預先發行版本的模組不會預設使用它自己的映像檔。模組只存在於包含它的第一個發行版本之後，因此較早的標籤沒有可複製的範例。請保留這個目錄：它保存了你的 state 和設定。

填寫 `terraform.tfvars`。`mail_from`、`mail_smtp_host` 和 `mail_smtp_username` 用來設定[電子郵件](email.md)；連接埠是 587，因為 Google Cloud 會封鎖對外的連接埠 25。若要變更其他任何東西，請把該區段加入 `main.tf` 的區塊，例如 `ingress = { kind = "domain-mapping" }`：每個區段及它拒絕的內容都在[模組的 README](../../deploy/opentofu/README.md#sections)。

`settings` 以鍵接受[設定參考文件](../reference/configuration.md)中的任何其他設定，`secrets` 則接受機密設定的鍵，每個鍵都會得到一個待填入的空機密。區段或模組已經設定的鍵會使 plan 失敗，模組在 Cloud Run 上不接受的鍵（例如 `listen`）也一樣；錯誤訊息會說明原因。如果帳號代碼要使用其他網域，請現在就決定 [`account_domain`](../reference/configuration.md#account_domain)，寫成 `settings = { account_domain = "example.org" }`。

**State** 預設保存在這個目錄中。它不含任何機密的值，但它是建立了哪些資源的唯一記錄：如果有多人執行部署，請用 `backend "gcs"` 區塊把它保存在 bucket 中。

<!-- source: 4-create-the-secrets-and-add-their-values 51cddae33ec9 -->
### 4. 建立機密並加入它們的值

以下指令會建立機密，並產生 KEK 和 VAPID 金鑰：

```sh
tofu init
tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager
```

`tofu init` 也會擷取該發行版本的腳本：`migrate.sh` 由 apply 執行，`console.sh` 則在後續步驟用到：

```sh
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

**現在就複製 KEK**，並把副本與資料庫分開保存，例如存在密碼管理器中。沒有它，就無法還原[備份](backup.md)，帳號的金鑰也會遺失。`tofu output kek` 有對應的指令。VAPID 金鑰也請比照辦理，遺失它會使所有 Web Push 訂閱失效：

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

接著加入來自其他地方的值。`tofu output secrets_to_fill` 會列出模組建立的每個空機密、該填入什麼，以及加入它的指令。無法讀取機密的執行個體不會啟動，因此請在服務之前加入：

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=- # the direct string
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

KEK 以掛載檔案的形式交給伺服器，由 `MY_KEK_FILE` 指定。其他機密則是在執行個體啟動時解析的環境變數，因此新版本會在下一個執行個體生效：為了保險起見，請部署新的 revision。

**OpenTofu 只產生 KEK 一次，之後不會再產生**。機密及其版本設有 `prevent_destroy`，而會寫入新值的版本編號固定在模組中：新的隨機 KEK 會讓舊 KEK 加密的所有內容都無法讀取。若要輪替，請依照[輪替 KEK](key-management.md#rotate-the-kek)，以 `gcloud secrets versions add` 加入新的值。OpenTofu 不會追蹤你在那裡加入的內容。

若要從一開始就把 KEK 保存在 Cloud KMS 中，請在這一步之前設定 `kek = { kind = "kms" }`，把 `kek_secret_manager` 換成 `module.mycroblog.module.kek_kms` 作為目標，並啟用 `cloudkms.googleapis.com`。

<!-- source: 5-apply 2076866c0f3b -->
### 5. 套用

```sh
tofu apply
```

這會依服務的啟動探測所需要的順序，建立其餘所有資源：探測會等待 `/health/ready`，而它在結構建立之前會回應 `503`。遷移任務最先建立，連同它所依賴的資源，其中包括負載平衡器的位址，因為伺服器的設定會指名它。接著 apply 會把 `my upgrade` 當作該任務的一次執行，從你的機器透過 `gcloud` 執行並等待它完成：依序是 `my config check`、`my db upgrade -check`、`my schema apply` 和 `my db upgrade`，並在第一個失敗的指令處停止。它的輸出會出現在 apply 的輸出中。第一個使用資料庫的指令會把 `host` 和 `account_domain` 記錄在資料庫中。到那時 apply 才會建立各個服務。

如果遷移失敗，apply 會在建立服務之前停止。請修正原因（常見的是沒有值的機密），再執行一次 `tofu apply`：它會再次執行遷移。

<!-- source: 6-point-the-domain-at-the-load-balancer c81cc51be2af -->
### 6. 把網域指向負載平衡器

`tofu output dns_records` 會列出一筆 `A` 記錄，把 `host` 指向負載平衡器的位址。請在你的 DNS 服務商加入它。如果該服務商有自己的代理功能，請為這筆記錄關閉它。

憑證由 Google 管理，只有在 DNS 指向該位址後才會生效，這可能需要一段時間。在那之前，`https://{host}` 會因 TLS 錯誤而失敗，因此聯邦和瀏覽器都還無法連到伺服器。其他部分不需要等待它：Cloud Tasks 和 Cloud Scheduler 會呼叫服務預設的 `run.app` URL，`https://<service>-<project number>.<region>.run.app`。**不要停用預設 URL**，否則任務會停止。該 URL 會拒絕來自網際網路的請求，而 Cloud Run 只有在這兩者來自同一個專案、且使用該 URL（而非自訂網域）時，才會把它們視為內部請求。

你還可以選擇哪些方式，請參閱[流量如何到達服務](#how-traffic-reaches-the-service)。

<!-- source: 7-create-the-first-administrator 19137d6d01f0 -->
### 7. 建立第一位管理員

Cloud Run 任務沒有終端機也沒有標準輸入，因此 `my admin create` 不會讀取密碼，而是輸出一個設定密碼的連結。開啟[主控台](deploy-opentofu.md#the-console)，然後執行：

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

請立即開啟連結並設定密碼。它只能使用一次，有效期限為一小時。**在那之前它會留在 Cloud Logging 中**，任何能讀取專案日誌的人都可以使用它。如果它過期了，請透過主控台執行 `my admin account password-reset-link @alice`。接著請開啟兩步驟驗證。

<!-- source: 8-go-through-the-checklist e7fe33d10e80 -->
### 8. 逐項檢查清單

逐項確認[正式環境檢查清單](production-checklist.md)，從[電子郵件](production-checklist.md#email)開始：在 `mail_from` 的網域上設定 SPF、DKIM 和 DMARC。

<!-- source: the-console 52df6f8c43ac -->
## 主控台

主控台會以服務的映像檔、設定和機密執行一個 `my` 指令。它只在 `console_enabled` 開啟時存在，因此用完後請關閉：

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` 會等待指令完成，輸出其結果和結束代碼，並以該代碼結束。它需要 `roles/logging.viewer`，因為任務的輸出會寫入 Cloud Logging。它使用服務正在執行的映像檔：在停在遷移的升級之後，那會是前一個發行版本，資料庫仍然相容。

指令沒有標準輸入也沒有終端機，因此需要從它們讀取密碼或檔案的指令無法以這種方式執行。指令輸出的內容會留在日誌中，因此請不要執行你不願寫進日誌的指令。

<!-- source: upgrading fe1bed77d3af -->
## 升級

請先閱讀[升級](database.md#upgrading)和版本說明，並且一次只升一個次要版本。升級就是套用新的發行版本：plan 會顯示每個服務和任務都換上新的映像檔，而 apply 會在其中任何一個提供服務之前先執行遷移。

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

映像檔就是 `ref` 所指定的發行版本，因此變更 `ref` 就是整個升級。套用之前請先閱讀 plan：新版本的模組也可能變更資源。apply 會執行以下步驟，並在第一個失敗的步驟停下：

1. 為遷移任務和主控台任務指定新的映像檔，連同 plan 中服務不依賴的其餘部分。
2. 從你執行 apply 的地方，把 `my upgrade` 當作遷移任務的一次執行並等待它完成：`my config check`、`my db upgrade -check`、`my schema apply` 和 `my db upgrade`。它的輸出會出現在 apply 的輸出中。
3. 部署每個 worker 的新 revision，再部署服務的。只有當它的啟動探測在 `/health/ready` 通過時，revision 才會接收流量。

**執行 apply 的地方**需要 `gcloud` 和 `jq`，以及可以執行該任務並讀取它的日誌的憑證（`roles/run.developer` 和 `roles/logging.viewer`，Owner 已包含它們）。執行 apply 的 CI 任務也一樣。

**當某個步驟失敗時**，服務會停留在它們的 revision，版本說明或遷移的輸出會說明原因。請修正原因並以相同的版本再執行一次 `tofu apply`：它會再次執行遷移，已完成的步驟會發現沒有剩下的工作。被中斷或斷線的 apply 會讓該次執行繼續留在 Cloud Run 上；下一次 apply 會先等它結束，再開始自己的。若結構已經改變而要回復，請還原備份：請參閱[回復](database.md#rolling-back)。以較早的版本執行 apply 會停在遷移，因為該發行版本會拒絕較新的結構，也不會變更任何服務。

若要試用候選版本，或要在較新的模組下保留較早發行版本的映像檔，請自行設定版本；它是映像檔標籤，不帶發行版本的 `v`：

```hcl
  image = { version = "0.2.0-rc.1" }
```

<!-- source: checking-the-sql-first 9ff10080c36d -->
### 先檢查 SQL

plan 不會顯示遷移的 SQL。若要在 apply 之前閱讀它，請只讓遷移任務換上新的映像檔，並在其中執行不會變更任何內容的 `my upgrade -dry-run`：

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region=asia-northeast1 --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

<!-- source: destructive-changes 5b3fdea90a55 -->
### 破壞性變更

當版本說明要求使用 `-allow-destructive` 時，請在區塊中指定該版本：

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

`my upgrade` 只會在套用該版本的那一次 apply 取得 `-allow-destructive`，因此留著這個值，也不會讓之後的發行版本刪除資料。

<!-- source: pinning-the-image-by-digest b31962ecc908 -->
### 以 digest 固定映像檔

[正式環境檢查清單](production-checklist.md#operations)建議使用版本說明中的 digest：

```hcl
  image = { digest = "sha256:..." }
```

digest 只指向一個映像檔，無論旁邊的標籤寫什麼，所以請與 `ref` 在同一次編輯中一起變更：沿用前一個發行版本的 digest，會讓新版本繼續使用那個發行版本的映像檔。

<!-- source: the-maintenance-mode 518e9aa176c0 -->
### 維護模式

[維護模式](maintenance.md)需要三次 apply，因為服務會在升級之前和之後各變更一次。把它加入區塊的設定並套用，進行備份，調高 `ref` 並套用，然後移除它再套用：

```hcl
  settings = { maintenance = "true" }
```

沒有它時，舊的 revision 會在遷移期間繼續提供服務，這[不保證能正常運作](database.md#the-server-during-an-upgrade)。以 `gcloud` 手動變更的設定不會持久：下一次 apply 會還原區塊所寫的內容。

<!-- source: monitoring-and-alerts 930234cd0dde -->
## 監控與警示

模組會加入 `/health/live` 的可用性檢查、以記錄為基礎的指標，以及以[運作手冊](alerts.md)的名稱命名的警示政策，它們會通知 `monitoring.notification_emails`（範例中的 `alert_emails`）的地址或 `notification_channels` 的通道。這個輸入沒有預設值：請指定誰要收到通知，或設定 `monitoring = null` 完全不使用它。它從未被套用過，所以請讓一項檢查失敗來試用一次。檢查的每次執行都會喚醒一個執行個體，這會增加[tick 的取捨](#costs)：`uptime_period` 預設為 15 分鐘，就是執行的頻率。

<!-- source: changing-a-backend c20598caf230 -->
## 更換後端

變更該區段的 `kind`。每種 kind 都會在伺服器上設定它所需要的東西，而 plan 會告訴你有什麼變更。更換資料庫也代表要搬移它的資料，因為新的資料庫是空的：請為舊的資料庫做[備份](backup.md)，並在沒有任何寫入時把它還原到新的資料庫。例如，以 Cloud SQL 取代 Neon：

```hcl
  database = { kind = "cloud-sql" }
```

若要使用 Google Cloud 以外的儲存空間，請把 `media.kind` 設為 `s3`（適用於任何 S3 相容服務，並搭配它的 `endpoint`），或設為 `r2`（搭配 `cloudflare.account_id`）。兩者都會從 `secrets_to_fill` 所列的機密讀取存取金鑰。

<!-- source: when-you-need-a-network 0a4359c002ed -->
## 需要網路時

本指南不需要網路：Neon、Cloud Tasks、Cloud Storage 和 Pub/Sub 都是以服務自己的身分透過網際網路連線，因此 Cloud Run 不需連接任何網路，也沒有需要付費或照管的東西。

若要在部署中加入只有私人位址的 Memorystore，或要讓 Cloud SQL 使用私人位址取代公開位址，就需要網路。`network = { kind = "vpc" }` 會建立一個，而 `network = { kind = "existing", ... }` 會連接到你既有的 VPC，你要以 `network`、`subnetwork`、`private_services_range` 和 `memorystore_connection_policy` 來描述它：

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

只有私人範圍會經過該網路，因此服務仍像之前一樣連到網際網路，不需要 Cloud NAT。`nat = true` 則改為讓所有流量都經過 Cloud NAT，以取得固定的來源位址。請先啟用 Service Networking 和 Network Connectivity API，以及 Compute Engine API。網路會增加執行個體的冷啟動時間，而 Memorystore 在伺服器閒置時也會計費。

<!-- source: splitting-jobs-into-workers 831929244c6d -->
## 把任務分到 worker

當某一種任務拖慢了其他任務時，請給它專屬的服務。請依[依任務類型分開 worker](running.md#splitting-workers-by-job-type)的說明，根據指標決定要分出哪些任務。同一個值 `workers` 會交給服務、佇列和監控，每個佇列、URL、路由和設定都由它的鍵產生，因此這些都不需要你自己寫：

```hcl
  workers = {
    delivery = {
      job_types     = ["federation.fanout", "federation.deliver"]
      max_instances = 20
      concurrency   = 16
    }
  }
```

plan 會拒絕把必須放在一起的類型分開。它也需要一個能觸及每個處理程序的訊息中介（broker），而 `stream.kind = pubsub` 就是。

該頁面中兩種常見的配置如下。若要避免傳送拖慢其他任務，請使用上面的值。若要讓電子郵件和通知保持快速：

```hcl
  workers = {
    notify = {
      job_types = [
        "identity.verification_email", "identity.password_reset_email", "identity.password_notice_email",
        "identity.closure_email", "identity.deactivation_email",
        "notification.email", "notification.moderator_email", "notification.supporter_email",
        "notification.push", "notification.post_subscribers",
        "instance.notify_announcement", "voice.notify_started",
      ]
      max_instances = 10
    }
  }
```

依該頁面的說明，請排除你尚未設定的設定所對應的類型：伺服器未註冊的類型會讓啟動停止，而 plan 無法看出是哪些。使用 `workers` 時，預設服務即使閒置也不會協助處理這些類型，因為它們的任務會送到專屬的佇列。

每個 worker 接受 `job_types`，以及 `min_instances`、`max_instances`、`cpu`、`memory_mib`、`concurrency`、`max_concurrent_dispatches` 和 `max_dispatches_per_second`，沒有指定的部分各沿用服務或佇列的值。worker 的服務不接收來自外部的流量。升級會在遷移之後、預設服務之前更新每個 worker，不需要你另外告訴它：只有失敗的步驟會讓它停止。每個 worker 都可能帶來服務的費用，它的執行個體的連線也會計入資料庫的上限。

<!-- source: how-traffic-reaches-the-service 156fc6f76743 -->
## 流量如何到達服務

這由 `ingress.kind` 決定。預設是負載平衡器，這是為正式環境準備的。網域對應則用於以低成本試用部署：設定 `ingress = { kind = "domain-mapping" }`。

| | `load-balancer`（預設） | `domain-mapping` |
| --- | --- | --- |
| 月費 | 轉送規則約 $18，另加每 GB 的費用。請查看[價格頁面](https://cloud.google.com/vpc/network-pricing)：這個數字無法在那裡得到確認 | 無 |
| 狀態 | Google Cloud 建議的方式 | 預覽版功能，由於延遲問題，Google Cloud 不建議用於正式環境 |
| 區域 | 任何區域 | `asia-northeast1`、`us-central1` 及少數其他區域 |
| DNS | 一筆 `A` 記錄，來自 `tofu output dns_records` | 同一個輸出所列的記錄 |
| 憑證 | 由 Google 管理，DNS 指向位址後生效 | 由 Google 管理，最多需要 24 小時 |
| 網域所有權 | DNS 之外不需要其他事 | 你以及其他每個執行 apply 的帳號，都必須在 apply 之前成為[已驗證的擁有者](https://cloud.google.com/run/docs/mapping-custom-domains) |
| `run.app` URL | 拒絕來自網際網路的請求，因此沒有人能繞過負載平衡器 | 任何人都能連線：網域對應無法加入標頭，因此來源保持開放 |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` 和負載平衡器的位址 | `169.254.0.0/16` |

Cloud Tasks 和 Cloud Scheduler 不經過其中任何一個：它們直接呼叫預設的 `run.app` URL。

<!-- source: cloudflare e55e367a0064 -->
### Cloudflare

`ingress = { kind = "cloudflare" }` 會將 Cloudflare 放在 `host` 前面，並對所有東西繞過快取，而 `ingress.media_host` 則在任何種類旁加上一個有快取的媒體主機。兩者都需要 `cloudflare = { zone_id = "..." }`，權杖放在 `CLOUDFLARE_API_TOKEN` 中。它們只在模擬的供應商上執行過。

- **你的方案決定來源。** Cloud Run 依 `Host` 路由，而 Cloudflare 只在 Enterprise 方案提供 Host 標頭覆寫。在任何其他方案上，來源必須自己接受你的主機，因此模組會為每個主機建立一個網域對應，並有上表所述的取捨，Cloudflare 再連到它。它的憑證能否在代理之後簽發與更新，尚未驗證，所以請從 `cloudflare.proxied = false` 開始。在 Enterprise 方案上，`cloudflare.host_header_override = true` 會改為連到 `run.app` URL，不需要網域對應。
- **標頭會關閉來源。** 模組會加入請求標頭轉換規則，而伺服器會以 `403` 拒絕沒有該標頭的請求，但健康檢查和任務自己的 `POST`（它們會檢查自己的權杖）除外。所以沒有人能透過 `run.app` URL 繞過 Cloudflare，呼叫者自己的 Cloudflare Worker 也無法選擇速率限制所看到的用戶端位址。
- **這個值會落在狀態中。** 模組會產生它並寫入機密 `my-origin-secret`，但 Cloudflare 供應商沒有規則標頭值的唯寫形式，所以請保持狀態私有。輪換如何運作請見 [CDN 指南](cdn.md#rotate-the-value)。
- **在記錄沒有被代理時，沒有東西會加上標頭**，所以伺服器會拒絕除了健康檢查之外的所有東西：為了網域對應的憑證而設定 `proxied = false` 時，請在憑證生效後再開啟代理。
- **請關閉 Bot Fight Mode**（在儀表板中）。它涵蓋整個 zone，沒有任何規則能跳過它，而且對 `/ap/`、`/.well-known/` 或 `/api/` 的挑戰會破壞聯邦，因為其他伺服器不是瀏覽器。任務不會經過 Cloudflare。模組不會加入任何挑戰，並會關閉 Browser Integrity Check，但不管理 Bot Fight Mode。基於同樣的理由，請不要開啟「I'm Under Attack」。
- **Email Address Obfuscation 可以維持開啟。** 它會將 HTML 中的電子郵件地址改寫成 `[email protected]`，並加入一段腳本來還原，而伺服器的 Content-Security-Policy 會拒絕這段腳本。伺服器自己的頁面，例如說明確認郵件寄去哪裡的頁面，會標記它們的內容，讓它不去動這些地址。
- **如果你想讓 `http://` 被重新導向，請在儀表板中開啟 Always Use HTTPS。** 它涵蓋整個 zone，所以模組將它留給你處理。
- **逾時**：當來源 125 秒沒有送出任何東西時，Cloudflare 會回應 `524`。任務不受影響，因為它們直接呼叫 `run.app` URL。串流每 30 秒送出一次心跳，低於 900 秒的閒置上限。在一般被代理的主機上，它是否會緩衝串流，尚未驗證。
- **上傳**：Free 和 Pro 為 100 MB，高於 Cloud Run 的 32 MiB，而後者仍然是上限。
- **影片**：Cloudflare 的條款限制在 Free、Pro 和 Business 方案上提供影片，以及不成比例的大量大型檔案，它可能會將內容重新導向或採取其他行動。請在你將影片快取在媒體主機上之前，先決定這一點。

**被刪除的檔案仍會留在快取中。** 媒體主機會遵循伺服器的 `Cache-Control`，所以被刪除的本地檔案最長可能會持續可見一年，遠端檔案則為 `remote_media_cache_ttl_seconds`。伺服器不會清除它。當某些東西必須消失時，請手動清除，在儀表板的 **Caching > Configuration > Custom Purge** 下進行，或是：

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

代理 URL 的查詢字串中帶有它的 `sig`，這是被快取內容的一部分，所以請提供完整的 URL。

<!-- source: changing-the-kek-backend b1e8cbb9ccfb -->
## 更換 KEK 後端

變更 `kek.kind` 只是 `main.tf` 中的一行，但在有資料的實例上並非毫無代價。資料庫保存了以目前金鑰包裝的機密，因此失去它的實例將無法再讀取這些機密，而所有帳號的金鑰都在其中。

請依照[在本機 KEK 與 KMS 之間搬移](key-management.md#move-between-the-local-kek-and-a-kms)。從機密改為 Cloud KMS 時，在機密搬移期間，請把目前的 KEK 保留為 `kek_previous`：加入 `secrets = ["kek_previous"]`，並以目前的 KEK 填入機密 `my-kek-previous`。透過[主控台](deploy-opentofu.md#the-console)執行 `my kek rewrap`，等 `my kek status` 不再計算到 `local:` 金鑰時再移除它。只要你還保留 rewrap 之前的資料庫備份，就請保留該 KEK。

在沒有資料的新實例上，不需要搬移任何東西。

<!-- source: limits 3ff113a57279 -->
## 限制

- **上傳**：Cloud Run 會拒絕超過 32 MiB 的 HTTP/1 請求，而伺服器使用的是 HTTP/1.1。因此模組把 `MY_MEDIA_MAX_AV_BYTES` 設為 30 MiB（預設為 100 MiB），讓較大的上傳得到伺服器自己的回應。
- **串流**：請求逾時（預設 15 分鐘）會結束開啟中的串流，用戶端會重新連線。
- **用戶端位址**：速率限制仰賴 `MY_TRUSTED_PROXIES`，而模組會根據入口設定它。我們讀過的文件都沒有說明 Cloud Run 對直接請求會在 `X-Forwarded-For` 加上什麼，或容器看到的對端位址是什麼，容器合約對兩者也都沒有說明，因此 `169.254.0.0/16` 是一項假設。外部負載平衡器有文件記載的形式為 `[<supplied>,]<client-ip>,<lb-ip>`。請檢查最初幾筆請求日誌中的位址：如果用戶端 IP 是 `169.254` 位址或 Google 的位址，就表示該設定不適用於你的專案。
- **公開存取**：服務會把 `roles/run.invoker` 授予 `allUsers`，而限制網域共用的組織政策會拒絕這麼做。這樣的專案需要設定例外。

<!-- source: costs b8be3997d723 -->
## 費用

伺服器是要花錢的。這些數字是讓你了解預期費用以及在哪裡可以透過用心調整來降低費用的參考，而不是部署必須遵守的上限。它們是單一區域的大約定價，取自 2026-10 和 2026-11。請在 [Cloud Run](https://cloud.google.com/run/pricing) 和 [Neon](https://neon.com/pricing) 的價格頁面上確認。每個計費帳戶每月在 180,000 vCPU 秒和 360,000 GiB 秒以內的運算是免費的。

| 部分 | 閒置時 | 增加費用的因素 |
| --- | --- | --- |
| 負載平衡器 | 每月約 $18，未在價格頁面上得到確認 | 每 GB 的費用；請參閱[流量如何到達服務](#how-traffic-reaches-the-service) |
| Neon（Launch 方案） | 使用預設 tick 時每月約 $19 | 儲存空間每 GB-月 $0.35。改用 `database.kind = cloud-sql` 則是較小的機器，固定費用每月約 $9.40 |
| Cloud Run | $0：最小執行個體數為 0，CPU 只在處理請求期間計費 | 每個開啟中的串流都會讓一個執行個體持續計費；見下文 |
| Cloud Tasks | $0 | 100 萬次操作免費，之後每百萬次 $0.40。一個任務約需 2 次 |
| Cloud Scheduler | $0：每個計費帳戶 3 個任務免費 | 超過後每個任務 $0.10 |
| Cloud Storage | 每 GiB 每月約 $0.02 | 只有在 `us-east1`、`us-west1` 和 `us-central1` 有 5 GiB 免費。媒體透過 Cloud Run 傳出 |
| Pub/Sub | $0：10 GiB 免費 | 只在串流開啟期間產生 |
| Secret Manager | 幾美分 | 少數幾個版本免費，之後每個版本幾美分 |

**Tick 會讓 Neon 保持清醒**。每次 tick 都會讀寫資料庫，而 Neon 只有在 5 分鐘沒有活動後才會停止運算資源，因此使用預設的 `* * * * *` 時，運算資源永遠不會停止。這相當於每月 730 小時，以 0.25 CU 計算為 182.5 CU 小時，也就是上面的 $19。tick 維持每分鐘一次，是為了讓定期任務準時執行，並讓遺失的喚醒訊號能很快恢復。

**開啟中的串流會讓執行個體持續計費**。在以請求為基礎的計費方式下，執行個體在處理任何請求時都會計費，而即時更新就是一個很長的請求。一個整月都保持串流開啟的用戶端，就會讓一個執行個體全天候計費：以 1 vCPU 和 512 MiB 計算，扣除免費額度後每月約 $60。更多的串流會共用該執行個體，上限為 `concurrency`。如果串流大部分時間都保持開啟，請與以執行個體為基礎的計費方式，或小型 VM 搭配 [Docker Compose](deploy-docker-compose.md) 比較。

<!-- source: where-care-reduces-it 28bfc2e5ffcc -->
### 用心調整可以降低費用的地方

- **使用網域對應**取代負載平衡器，可以省下 $18，但有[上述的取捨](#how-traffic-reaches-the-service)。它適合用來試用部署。
- **Neon 的 Free 方案**可以省下 $19，小型實例可以在上面執行，但有以下取捨：
  - 它每月允許 100 CU 小時，預設的 tick 大約 17 天就會用完。使用較長的排程（例如 `queue = { tick_schedule = "*/10 * * * *" }`）就能保持在額度內：在其他任何東西喚醒它之前，最多一半的時間處於清醒狀態，約 91 CU 小時。這樣定期任務最多會延遲 10 分鐘執行，遺失的喚醒訊號也會較晚恢復。
  - 聯邦伺服器全天候都會收到傳送，這也會喚醒資料庫，因此不論排程為何，繁忙的實例都會超出它的額度。
  - 它每個專案只能保存 1 GB，可還原的歷史紀錄為 6 小時，而且它的運算資源一定會在閒置 5 分鐘後停止，因此在那之後的第一個請求必須等待它啟動。
