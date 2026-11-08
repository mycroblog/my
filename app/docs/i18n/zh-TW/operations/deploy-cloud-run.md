<!-- source: deploying-on-cloud-run 9d2acbfd443c -->
# 部署到 Cloud Run

本頁說明如何使用 [`deploy/opentofu`](../../deploy/opentofu/README.md) 中的 OpenTofu 模組，在 Google Cloud 上執行 `my`：使用 Cloud Run、Cloud Tasks、Cloud Scheduler、Cloud Storage 和 Pub/Sub，搭配 Neon 上的 PostgreSQL 資料庫，並置於全域外部 Application Load Balancer 之後。沒有人使用時，服務不會執行任何執行個體，只有每分鐘喚醒伺服器一次的 tick 例外。伺服器是要花錢的：在儲存空間、流量和開啟中的串流之外，預計每月約 $37，可以透過用心調整降低多少費用，請參閱[費用](#costs)。

這些模組需要 OpenTofu 1.11 以上版本。不支援 Terraform。

<!-- source: what-it-creates c95687df197f -->
## 建立的資源

| 部分 | 模組 | 資源 |
| --- | --- | --- |
| 伺服器 | `app-gcp-cloud-run` | 一個 `role = all` 的 Cloud Run 服務、一個遷移任務、需要時才存在的主控台任務、一個服務帳號，以及公開存取權 |
| 流量 | `ingress-gcp-load-balancer`，或 `ingress-gcp-domain-mapping` 或 `ingress-cloudflare-app`，另可選用 `ingress-cloudflare-media` | 一個帶有憑證的全域外部 Application Load Balancer、網域對應或 Cloudflare，以及媒體主機 |
| 任務 | `queue-gcp-cloud-tasks` | 一個 Cloud Tasks 佇列、一個 Cloud Scheduler 任務，以及呼叫伺服器的服務帳號 |
| 媒體 | `blob-gcp-gcs` | 一個私人 bucket |
| 即時更新 | `stream-gcp-pubsub` | 一個主題和一個自訂角色。伺服器會自行建立和移除自己的訂閱 |
| 資料庫 | `db-external` | 無：資料庫由你手動建立 |
| 電子郵件 | `mail-smtp` | 無：任何 SMTP 服務皆可 |
| 機密 | `secrets-gcp-secret-manager` | 由你填入的資料庫 URL 和 SMTP 密碼的機密，以及產生的 VAPID 金鑰 |
| KEK | `kek-gcp-secret-manager` 或 `kek-gcp-kms` | 以檔案掛載的產生機密，或 Cloud KMS 金鑰 |

機密的值永遠不會進入 OpenTofu 的 plan 或 state：來自其他地方的值由你自己加入，KEK 和 VAPID 金鑰則由部署透過唯寫（write-only）屬性產生。

<!-- source: before-you-start 4c624a6b7316 -->
## 開始之前

- `tofu` 1.11 以上版本、`gcloud` 和 `jq`。
- 一個啟用計費、只供這個實例使用的 Google Cloud 專案，以及一個在該專案上擁有 Owner 角色的帳號。
- 一個用於 [`host`](../reference/configuration.md#host) 的網域。之後無法變更：請參閱[之後無法變更的設定](production-checklist.md#settings-you-cannot-change-later)。
- 來自[發行版本](running.md#get-the-image)的 `my` 版本。沒有 `latest` 標籤。

<!-- source: first-deployment 996fcc6eb35e -->
## 首次部署

<!-- source: 1-enable-the-apis 2d9c831fb77d -->
### 1. 啟用 API

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com
```

`compute.googleapis.com` 是負載平衡器所需要的。

<!-- source: 2-create-the-database-on-neon 139308cfc452 -->
### 2. 在 Neon 上建立資料庫

請預期使用 Launch 方案：使用預設的 tick 時，運算資源永遠不會停止。Free 方案也可以使用，但有[費用](#costs)中所述的取捨。

有兩種方式，範例使用第一種：

- **手動，使用 `db-external`**。在 Neon 主控台中，於靠近你 Cloud Run 區域的區域建立專案，並複製它的連線字串。密碼不會進入 OpenTofu 的 state。
- **使用 [`db-neon`](../../deploy/opentofu/modules/db-neon/README.md)**，它會為你建立專案，並把 URL 寫入機密**。這樣資料庫密碼就會進入 state**，而且該 provider 已無人維護：它的 README 說明了這代表什麼，`main.tf` 中 `db` 區塊的註解示範了如何替換。

一個專案有兩種連線字串，部署會在各自適合的地方使用：

- **直接**連線字串（主機名稱不含 `-pooler`）是必要的。遷移任務和主控台一律使用它：`my schema apply` 會持有工作階段層級的鎖，而交易模式的連線集區不會保持這個鎖。
- **集區**連線字串（主機名稱含 `-pooler`）是選用的，供服務使用。Neon 的連線集區能處理伺服器所需的一切：請參閱[透過連線集區](database.md#through-a-connection-pooler)。請把它放在一個獨立的機密中，交給 `db` 模組的 `pooled_url_secret_id`，並加入 `secrets` 模組的 `referenced`。

沒有集區連線字串時，服務也會使用直接連線字串，受資料庫的連線上限限制，而這由 app 模組的 `max_instances` 決定上限。若要針對 Cloud Run 執行個體一次接受的 80 個請求，把連線池從每個執行個體預設的 4 個調高，請在服務使用的連線字串加上 `pool_max_conns`，例如 `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8`。請參閱[等待資料庫連線](running.md#waiting-for-a-database-connection)。

<!-- source: 3-copy-the-example-and-set-the-variables 3abe6b730543 -->
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

**把 `ref` 設為該發行版本**。`main.tf` 中的每個模組區塊都以 `?ref=` 指定發行版本，穩定版的副本已經設定正確。若是預先發行版本，請自行設定：`sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`。模組只存在於包含它們的第一個發行版本之後，因此較早的標籤沒有可複製的範例。請讓 `ref` 和 `image` 保持相同的發行版本，並保留這個目錄：它保存了你的 state 和設定。

填寫 `terraform.tfvars`。`mail_from`、`mail_smtp_host`、`mail_smtp_port`（預設為 587；Google Cloud 會封鎖對外的連接埠 25）和 `mail_smtp_username` 用來設定[電子郵件](email.md)。`env` 和 `secret_env` 分別以值和機密 ID 接受[設定參考文件](../reference/configuration.md)中的任何其他設定。在兩個地方指定同一個設定會使 plan 失敗。

若不寄送電子郵件，請從 `main.tf` 移除 `mail` 模組及其機密，`mail` 就會維持 `none`。請先閱讀[這對註冊有什麼影響](production-checklist.md#email)。

如果帳號代碼要使用其他網域，請現在就決定 [`account_domain`](../reference/configuration.md#account_domain)，在 `env` 中以 `MY_ACCOUNT_DOMAIN` 設定。

**State** 預設保存在這個目錄中。它不含任何機密的值，但它是建立了哪些資源的唯一記錄：如果有多人執行部署，請用 `backend "gcs"` 區塊把它保存在 bucket 中。

<!-- source: 4-create-the-secrets-and-add-their-values f94fd484e28f -->
### 4. 建立機密並加入它們的值

以下指令會建立機密，並產生 KEK 和 VAPID 金鑰：

```sh
tofu init
tofu apply -target=module.secrets -target=module.kek
```

`tofu init` 也會擷取該發行版本的 `upgrade.sh` 和 `console.sh`，後續步驟會用到它們：

```sh
SCRIPTS=.terraform/modules/app/deploy/opentofu
```

**現在就複製 KEK**，並把副本與資料庫分開保存，例如存在密碼管理器中。沒有它，就無法還原[備份](backup.md)，帳號的金鑰也會遺失。VAPID 金鑰也請比照辦理，遺失它會使所有 Web Push 訂閱失效：

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

接著加入來自其他地方的值。無法讀取機密的執行個體不會啟動，因此請在 app 之前加入：

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=- # the direct string
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

KEK 以掛載檔案的形式交給伺服器，由 `MY_KEK_FILE` 指定。其他機密則是在執行個體啟動時解析的環境變數，因此新版本會在下一個執行個體生效：為了保險起見，請部署新的 revision。

**OpenTofu 只產生 KEK 一次，之後不會再產生**。機密及其版本設有 `prevent_destroy`，而會寫入新值的版本編號固定在模組中：請不要更改它，因為新的隨機 KEK 會讓舊 KEK 加密的所有內容都無法讀取。若要輪替，請依照[輪替 KEK](key-management.md#rotate-the-kek)，以 `gcloud secrets versions add` 加入新的值。OpenTofu 不會追蹤你在那裡加入的內容。

若要從一開始就把 KEK 保存在 Cloud KMS 中，請依照 `main.tf` 中 `kek` 區塊的註解替換它，並啟用 `cloudkms.googleapis.com`。若要使用你自己的 KEK 或 VAPID 金鑰，請在這一步之前，於 `kek` 模組設定 `create = false`，或從 `generate_p256_keys` 移除該金鑰。在其他地方建立的機密只需要它的 ID：把它交給 app 模組的 `secret_env`，或交給使用它的模組。

<!-- source: 5-create-what-the-migration-needs eab1bb33867c -->
### 5. 建立遷移所需的資源

服務的啟動探測會等待 `/readyz`，而它在結構描述（schema）存在之前都會回應 `503`，因此必須先建立結構描述。請建立遷移任務及其相依的資源，其中包括負載平衡器的位址，因為伺服器的設定會用到它：

```sh
tofu apply -target='module.app.google_cloud_run_v2_job.one_off["migrate"]'
```

<!-- source: 6-apply-the-schema 2946ca18d263 -->
### 6. 套用結構描述

```sh
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.1.0 --migrate-only
```

它會以任務執行的方式，依序執行 `my config check`、`my db upgrade -check`、`my schema apply` 和 `my db upgrade`，並在第一個失敗的指令處停止。第一個使用資料庫的指令會把 `host` 和 `account_domain` 記錄在資料庫中。

<!-- source: 7-create-the-rest 368abac4aa08 -->
### 7. 建立其餘資源

```sh
tofu apply
```

<!-- source: 8-point-the-domain-at-the-load-balancer 7e54edc163b2 -->
### 8. 把網域指向負載平衡器

`tofu output dns_records` 會列出一筆 `A` 記錄，把 `host` 指向負載平衡器的位址。請在你的 DNS 服務商加入它。如果該服務商有自己的代理功能，請為這筆記錄關閉它。

憑證由 Google 管理，只有在 DNS 指向該位址後才會生效，這可能需要一段時間。在那之前，`https://{host}` 會因 TLS 錯誤而失敗，因此聯邦和瀏覽器都還無法連到伺服器。其他部分不需要等待它：Cloud Tasks 和 Cloud Scheduler 會呼叫服務預設的 `run.app` URL，`https://<service>-<project number>.<region>.run.app`**。不要停用預設 URL**，否則任務會停止。該 URL 會拒絕來自網際網路的請求，而 Cloud Run 只有在這兩者來自同一個專案、且使用該 URL（而非自訂網域）時，才會把它們視為內部請求。

你還可以選擇哪些方式，請參閱[流量如何到達服務](#how-traffic-reaches-the-service)。

<!-- source: 9-create-the-first-administrator c5b03212fb36 -->
### 9. 建立第一位管理員

Cloud Run 任務沒有終端機也沒有標準輸入，因此 `my admin create` 不會讀取密碼，而是輸出一個設定密碼的連結。開啟[主控台](#the-console)，然後執行：

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

請立即開啟連結並設定密碼。它只能使用一次，有效期限為一小時**。在那之前它會留在 Cloud Logging 中**，任何能讀取專案日誌的人都可以使用它。如果它過期了，請透過主控台執行 `my admin account password-reset-link @alice`。接著請開啟兩步驟驗證。

<!-- source: 10-go-through-the-checklist 90add86e4288 -->
### 10. 逐項檢查清單

逐項確認[正式環境檢查清單](production-checklist.md)，從[電子郵件](production-checklist.md#email)開始：在 `mail_from` 的網域上設定 SPF、DKIM 和 DMARC。

<!-- source: the-console 5aebf704c6d7 -->
## 主控台

主控台會以服務的映像檔、設定和機密執行一個 `my` 指令。它只在 `console_enabled` 開啟時存在，因此用完後請關閉：

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` 會等待指令完成，輸出其結果和結束代碼，並以該代碼結束。它需要 `roles/logging.viewer`，因為任務的輸出會寫入 Cloud Logging。不論 `image` 為何，它都使用服務正在執行的映像檔。

指令沒有標準輸入也沒有終端機，因此需要從它們讀取密碼或檔案的指令無法以這種方式執行。指令輸出的內容會留在日誌中，因此請不要執行你不願寫進日誌的指令。

<!-- source: upgrading 1a1e204c5173 -->
## 升級

請先閱讀[升級](database.md#upgrading)和發行說明，並且一次只升級一個次要版本。

```sh
# 1. Back up the database while nothing writes. Use pg_dump or a Neon branch (see Backup and restore).
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --update-env-vars=MY_MAINTENANCE=true
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Read the SQL it would run. This changes nothing but the image of the migration job.
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --plan

# 3. Upgrade. These scripts are of the release your modules name, which is enough: they only run gcloud and my.
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --maintenance
```

這個指令碼會執行以下步驟，並在第一個失敗的步驟處停止：

1. 使用 `--maintenance` 時，把服務切換到[維護模式](maintenance.md)。
2. 把新的映像檔交給遷移任務。
3. 執行 `my config check`、`my db upgrade -check`、`my schema apply` 和 `my db upgrade`，每一個都會等待前一個完成。
4. 部署不含維護模式的新 revision。只有在它的啟動探測通過 `/readyz` 時，才會接收流量。

不使用 `--maintenance` 時，舊的 revision 會在步驟 2 和 3 期間繼續提供服務，而這[不保證能正常運作](database.md#the-server-during-an-upgrade)。當發行說明要求時，請加上 `--allow-destructive`。請以 `--image ghcr.io/mycroblog/my:0.2.0@sha256:...` 固定 digest，這是[正式環境檢查清單](production-checklist.md#operations)所建議的。

接著把 `terraform.tfvars` 中的 `image` 設為相同的值，`main.tf` 中的 `ref` 設為相同的發行版本，然後執行 `tofu init` 和 `tofu plan`。套用之前請先閱讀該 plan：新發行版本的模組可能會變更資源。`tofu apply` 永遠不會變更既有服務或任務的映像檔，因此忘了更新 `image` 並不會造成傷害，但新任務或重新建立的服務會從這個值啟動。

**某個步驟失敗時**，服務會停留在它原本的 revision 上，若你有要求，則維持在維護模式。排除原因後再執行一次指令碼：這些步驟可以安全地重複執行。在 `my schema apply` 尚未執行時，若要不升級就離開維護模式：

```sh
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --remove-env-vars=MY_MAINTENANCE
```

若要在結構描述變更之後回復，請還原備份：請參閱[回復](database.md#rolling-back)。

<!-- source: monitoring-and-alerts 67bc34d23fdf -->
## 監控與警報

模組 [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) 會加入 `/readyz` 的運作時間檢查、以[處理手冊](alerts.md)中的名稱命名的警報政策，以及電子郵件通知。它從未被實際套用過，因此請讓檢查失敗一次來試試看。請先啟用 `monitoring.googleapis.com`，並閱讀它 README 中關於檢查費用的說明：每次執行都會喚醒一個執行個體，並透過 `/readyz` 喚醒資料庫，這會加重 [tick 的取捨](#costs)。

<!-- source: changing-a-backend ef4660f60191 -->
## 更換後端

替換 `main.tf` 中該模組的區塊即可。app 模組會接收區塊回傳的任何內容，因此其他部分都不需要變更。更換資料庫也代表要搬移它的資料，因為新的資料庫是空的：步驟請參閱 [`db-gcp-cloud-sql`](../../deploy/opentofu/modules/db-gcp-cloud-sql/README.md#coming-from-db-external)。例如，使用你自己的資料庫：

```hcl
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/db-external?ref=v0.1.0&depth=1"

  url_secret_id = "another-database-url"
}
```

若要使用 Google Cloud 以外的儲存空間，請把 `blob` 區塊替換為 [`blob-s3`](../../deploy/opentofu/modules/blob-s3/README.md)（適用於任何 S3 相容服務）或 [`blob-cloudflare-r2`](../../deploy/opentofu/modules/blob-cloudflare-r2/README.md)。

<!-- source: when-you-need-a-network fb392704a1e7 -->
## 需要網路時

本指南不需要網路：Neon、Cloud Tasks、Cloud Storage 和 Pub/Sub 都是以服務自己的身分透過網際網路連線，因此 Cloud Run 不需連接任何網路，也沒有需要付費或照管的東西。

若要在部署中加入只有私人位址的 Memorystore，或要讓 Cloud SQL 使用私人位址取代公開位址，就需要網路。請加入 [`network-gcp-vpc`](../../deploy/opentofu/modules/network-gcp-vpc/README.md)，或把你既有的 VPC 描述為[相同形狀](../../deploy/opentofu/README.md#the-network-on-google-cloud)的物件，再把同一個網路交給 app 和每個接受它的模組：

```hcl
module "network" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/network-gcp-vpc?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
}

module "kv" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kv-gcp-memorystore?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
  network    = module.network
}
```

也請為 app 設定 `network = module.network`，並把 `module.kv` 加入它的 `backends`。只有私人範圍會經過該網路，因此服務仍像之前一樣連到網際網路，不需要 Cloud NAT。使用私人位址的後端搭配在其他網路上（或沒有網路）的 app，會使 plan 失敗。請先啟用 Service Networking 和 Network Connectivity API，以及 Compute Engine API。網路會增加執行個體的冷啟動時間：請參閱[私人網路增加了什麼](../../deploy/opentofu/modules/kv-gcp-memorystore/README.md#what-the-private-network-adds)。

<!-- source: splitting-jobs-into-workers ed661aede0a3 -->
## 把任務分到 worker

當某一種任務拖慢了其他任務時，請給它專屬的服務。請依[依任務類型分開 worker](running.md#splitting-workers-by-job-type)的說明，根據指標決定要分出哪些任務。同一個值會交給佇列和 app，每個佇列、URL、路由和設定都由它的鍵產生，因此這些都不需要你自己寫：

```hcl
locals {
  workers = {
    delivery = {
      job_types     = ["federation.fanout", "federation.deliver"]
      max_instances = 20
      concurrency   = 16
    }
  }
}
```

在 `queue` 區塊和 `app` 區塊都設定 `workers = local.workers`。plan 會拒絕把必須放在一起的類型分開，也會拒絕佇列和 app 被給予不同的值。它也需要一個能觸及每個處理程序的訊息中介（broker），而 `stream-gcp-pubsub` 就是。如果你使用 `monitoring-gcp`，也請把相同的值交給它。

該頁面中兩種常見的配置如下。若要避免傳送拖慢其他任務，請使用上面的值。若要讓電子郵件和通知保持快速：

```hcl
locals {
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
}
```

依該頁面的說明，請排除你尚未設定的設定所對應的類型：伺服器未註冊的類型會讓啟動停止，而 plan 無法看出是哪些。使用 `workers` 時，預設服務即使閒置也不會協助處理這些類型，因為它們的任務會送到專屬的佇列。

所有欄位，以及 worker 從 app 繼承了什麼，請參閱[合約](../../deploy/opentofu/README.md#workers)。worker 的服務不接收來自外部的流量。`upgrade.sh` 會在遷移之後、預設服務之前更新每個 worker，不需要你另外告訴它：只有失敗的步驟會讓它停止。每個 worker 都可能帶來服務的費用，它的執行個體的連線也會計入資料庫的上限。

<!-- source: how-traffic-reaches-the-service d1e6d0ed195b -->
## 流量如何到達服務

這由 `main.tf` 的 `ingress` 區塊決定。範例使用負載平衡器，這是為正式環境準備的。網域對應寫在它旁邊的註解中，用於以低成本試用部署：若要換用它，請變更該區塊和 `app` 區塊的 `service_ingress`，兩者不一致時 plan 會告訴你。

| | `ingress-gcp-load-balancer`（範例） | `ingress-gcp-domain-mapping` |
| --- | --- | --- |
| 月費 | 轉送規則約 $18，另加每 GB 的費用。請查看[價格頁面](https://cloud.google.com/vpc/network-pricing)：這個數字無法在那裡得到確認 | 無 |
| 狀態 | Google Cloud 建議的方式 | 預覽版功能，由於延遲問題，Google Cloud 不建議用於正式環境 |
| 區域 | 任何區域 | `asia-northeast1`、`us-central1` 及少數其他區域 |
| DNS | 一筆 `A` 記錄，來自 `tofu output dns_records` | 同一個輸出所列的記錄 |
| 憑證 | 由 Google 管理，DNS 指向位址後生效 | 由 Google 管理，最多需要 24 小時 |
| 網域所有權 | DNS 之外不需要其他事 | 你以及其他每個執行 apply 的帳號，都必須在 apply 之前成為[已驗證的擁有者](https://cloud.google.com/run/docs/mapping-custom-domains) |
| `run.app` URL | 拒絕來自網際網路的請求，因此沒有人能繞過負載平衡器 | 任何人都能連線：網域對應無法加入標頭，因此來源保持開放 |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` 和負載平衡器的位址 | `169.254.0.0/16` |
| `app` 區塊的 `service_ingress` | `internal-and-load-balancer`。與 `ingress` 區塊不一致時，plan 會失敗 | `all` |

Cloud Tasks 和 Cloud Scheduler 不經過其中任何一個：它們直接呼叫預設的 `run.app` URL。

若要使用服務自己的 URL，或在前方放置其他東西（例如 CDN），請移除 `ingress` 區塊及其在 `backends` 中的那一行，並把 `service_ingress` 保持為你需要的值：這一律是你明確的選擇，而 `all` 會讓 `run.app` URL 對任何人開放。在 `env` 中把 `MY_TRUSTED_PROXIES` 設為連線到伺服器的對象。不論你在前方放了什麼，都必須能在 `https://{host}` 回應。

<!-- source: cloudflare 95b7a9baaff1 -->
### Cloudflare

`ingress-cloudflare-app` 會把 Cloudflare 放在 `host` 前方，並對所有內容略過快取；`ingress-cloudflare-media` 則會在任何 ingress 旁加入一個有快取的媒體主機。請先閱讀各模組的 README（[app](../../deploy/opentofu/modules/ingress-cloudflare-app/README.md)、[media](../../deploy/opentofu/modules/ingress-cloudflare-media/README.md)）。權杖放在 `CLOUDFLARE_API_TOKEN` 中。這些模組只在模擬的 provider 上執行過。

- **你的方案決定來源**。Cloud Run 依 `Host` 進行路由，而 Cloudflare 只在 Enterprise 方案提供 Host 標頭覆寫。在其他任何方案上，`*.run.app` 來源會使 plan 失敗，因此來源本身必須接受你的主機：請保留該主機的 `ingress-gcp-domain-mapping`（不放入 `backends`），並設定 `origin_host = "ghs.googlehosted.com"`。每個主機都需要自己的對應。它的憑證在代理之後能否發行和更新尚未驗證，因此請從 `proxied = false` 開始。
- **標頭會封閉來源**。`ingress-cloudflare-app` 會加入一條請求標頭轉換規則，伺服器會以 `403` 拒絕沒有該標頭的請求，但健康檢查和任務自己的 `POST` 除外，它們會檢查各自的權杖。因此沒有人能透過 `run.app` URL 繞過 Cloudflare，呼叫者自己的 Cloudflare Worker 也無法選擇速率限制所看到的用戶端位址。
- **這個值會進入 state**。Cloudflare provider 沒有規則標頭值的唯寫形式。請用 `openssl rand -hex 32` 產生一個值，以 `gcloud secrets versions add` 加入機密，用 `TF_VAR_origin_secret` 傳入相同的值，並把 state 保持私密。如何輪替它，請參閱 [CDN 指南](cdn.md#rotate-the-value)。
- **記錄未經代理時，沒有任何東西會加上標頭**，因此伺服器會拒絕健康檢查以外的所有請求：為了網域對應的憑證而設定 `proxied = false` 時，請在那之後再設定機密。
- **在儀表板中關閉 Bot Fight Mode**。它涵蓋整個區域（zone），沒有任何規則能略過它，而對 `/ap/`、`/.well-known/` 或 `/api/` 的驗證挑戰會破壞聯邦，因為其他伺服器不是瀏覽器。任務不會經過 Cloudflare。這些模組不會加入任何驗證挑戰，並會關閉 Browser Integrity Check，但不會管理 Bot Fight Mode。基於相同的理由，請不要開啟「I'm Under Attack」。
- **如果你希望 `http://` 被重新導向，請在儀表板中開啟 Always Use HTTPS**。它涵蓋整個區域，因此模組把它留給你決定。
- **逾時**：當來源 125 秒沒有傳送任何東西時，Cloudflare 會回應 `524`。任務不受影響，因為它們直接呼叫 `run.app` URL。串流每 30 秒傳送一次心跳，低於 900 秒的閒置上限。在一般經過代理的主機上，它是否會緩衝串流尚未驗證。
- **上傳**：Free 和 Pro 方案為 100 MB，高於 Cloud Run 的 32 MiB，因此上限仍是後者。
- **影片**：Cloudflare 的條款限制在 Free、Pro 和 Business 方案上提供影片及不成比例的大量大型檔案，它可能會重新導向這些內容或採取其他措施。在媒體主機上快取影片之前，請先決定這件事。

**已刪除的檔案會留在快取中**。媒體主機遵循伺服器的 `Cache-Control`，因此已刪除的本站檔案最多可能在一年內仍然可見，遠端檔案則為 `remote_media_cache_ttl_seconds`。伺服器不會清除快取。當某些內容必須消失時，請在儀表板的 **Caching > Configuration > Custom Purge** 下手動清除，或執行：

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

代理 URL 的查詢字串中帶有它的 `sig`，而查詢字串也是快取內容的一部分，因此請提供完整的 URL。

<!-- source: changing-the-kek-backend c5e918ea5c81 -->
## 更換 KEK 後端

替換 `kek` 區塊只是 `main.tf` 中的一個區塊，但在有資料的實例上並非毫無代價。資料庫保存了以目前金鑰包裝的機密，因此失去它的實例將無法再讀取這些機密，而所有帳號的金鑰都在其中。

請依照[在本機 KEK 與 KMS 之間搬移](key-management.md#move-between-the-local-kek-and-a-kms)。從機密改為 Cloud KMS 時，在機密搬移期間，請把目前的 KEK 保留為 `kek_previous`，方法是透過 app 模組以 `secret_files = { MY_KEK_PREVIOUS_FILE = "my-kek" }` 傳入它的機密；透過[主控台](#the-console)執行 `my kek rewrap`，等 `my kek status` 不再計算到 `local:` 金鑰時再移除它。只要你還保留 rewrap 之前的資料庫備份，就請保留該 KEK。

在沒有資料的新實例上，不需要搬移任何東西。

<!-- source: limits 5bd276dd3a89 -->
## 限制

- **上傳**：Cloud Run 會拒絕超過 32 MiB 的 HTTP/1 請求，而伺服器使用的是 HTTP/1.1。因此範例把 `MY_MEDIA_MAX_AV_BYTES` 設為 30 MiB（預設為 100 MiB），讓較大的上傳得到伺服器自己的回應。
- **串流**：請求逾時（預設 15 分鐘）會結束開啟中的串流，用戶端會重新連線。
- **用戶端位址**：速率限制仰賴 `MY_TRUSTED_PROXIES`，而 app 模組會根據 `ingress` 模組回傳的內容設定它。我們讀過的文件都沒有說明 Cloud Run 對直接請求會在 `X-Forwarded-For` 加上什麼，或容器看到的對端位址是什麼，容器合約對兩者也都沒有說明，因此 `169.254.0.0/16` 是一項假設。外部負載平衡器有文件記載的形式為 `[<supplied>,]<client-ip>,<lb-ip>`。請檢查最初幾筆請求日誌中的位址：如果用戶端 IP 是 `169.254` 位址或 Google 的位址，就表示該設定不適用於你的專案。
- **公開存取**：服務會把 `roles/run.invoker` 授予 `allUsers`，而限制網域共用的組織政策會拒絕這麼做。這樣的專案需要設定例外。

<!-- source: costs 1ab8d07f6847 -->
## 費用

伺服器是要花錢的。這些數字是讓你了解預期費用以及在哪裡可以透過用心調整來降低費用的參考，而不是部署必須遵守的上限。它們是單一區域的大約定價，取自 2026-10 和 2026-11。請在 [Cloud Run](https://cloud.google.com/run/pricing) 和 [Neon](https://neon.com/pricing) 的價格頁面上確認。每個計費帳戶每月在 180,000 vCPU 秒和 360,000 GiB 秒以內的運算是免費的。

| 部分 | 閒置時 | 增加費用的因素 |
| --- | --- | --- |
| 負載平衡器 | 每月約 $18，未在價格頁面上得到確認 | 每 GB 的費用；請參閱[流量如何到達服務](#how-traffic-reaches-the-service) |
| Neon（Launch 方案） | 使用預設 tick 時每月約 $19 | 儲存空間每 GB-月 $0.35。`db-gcp-cloud-sql` 則是較小的機器，固定費用每月約 $9.40：明細請參閱它的 README |
| Cloud Run | $0：最小執行個體數為 0，CPU 只在處理請求期間計費 | 每個開啟中的串流都會讓一個執行個體持續計費；見下文 |
| Cloud Tasks | $0 | 100 萬次操作免費，之後每百萬次 $0.40。一個任務約需 2 次 |
| Cloud Scheduler | $0：每個計費帳戶 3 個任務免費 | 超過後每個任務 $0.10 |
| Cloud Storage | 每 GiB 每月約 $0.02 | 只有在 `us-east1`、`us-west1` 和 `us-central1` 有 5 GiB 免費。媒體透過 Cloud Run 傳出 |
| Pub/Sub | $0：10 GiB 免費 | 只在串流開啟期間產生 |
| Secret Manager | 幾美分 | 少數幾個版本免費，之後每個版本幾美分 |

**Tick 會讓 Neon 保持清醒**。每次 tick 都會讀寫資料庫，而 Neon 只有在 5 分鐘沒有活動後才會停止運算資源，因此使用預設的 `* * * * *` 時，運算資源永遠不會停止。這相當於每月 730 小時，以 0.25 CU 計算為 182.5 CU 小時，也就是上面的 $19。tick 維持每分鐘一次，是為了讓定期任務準時執行，並讓遺失的喚醒訊號能很快恢復。

**開啟中的串流會讓執行個體持續計費**。在以請求為基礎的計費方式下，執行個體在處理任何請求時都會計費，而即時更新就是一個很長的請求。一個整月都保持串流開啟的用戶端，就會讓一個執行個體全天候計費：以 1 vCPU 和 512 MiB 計算，扣除免費額度後每月約 $60。更多的串流會共用該執行個體，上限為 `concurrency`。如果串流大部分時間都保持開啟，請與以執行個體為基礎的計費方式，或小型 VM 搭配 [Docker Compose](deploy-docker-compose.md) 比較。

<!-- source: where-care-reduces-it 5711f378aadb -->
### 用心調整可以降低費用的地方

- **使用網域對應**取代負載平衡器，可以省下 $18，但有[上述的取捨](#how-traffic-reaches-the-service)。它適合用來試用部署。
- **Neon 的 Free 方案**可以省下 $19，小型實例可以在上面執行，但有以下取捨：
  - 它每月允許 100 CU 小時，預設的 tick 大約 17 天就會用完。使用較長的排程（例如 `tick_schedule = "*/10 * * * *"`）就能保持在額度內：在其他任何東西喚醒它之前，最多一半的時間處於清醒狀態，約 91 CU 小時。這樣定期任務最多會延遲 10 分鐘執行，遺失的喚醒訊號也會較晚恢復。
  - 聯邦伺服器全天候都會收到傳送，這也會喚醒資料庫，因此不論排程為何，繁忙的實例都會超出它的額度。
  - 它每個專案只能保存 1 GB，可還原的歷史紀錄為 6 小時，而且它的運算資源一定會在閒置 5 分鐘後停止，因此在那之後的第一個請求必須等待它啟動。
