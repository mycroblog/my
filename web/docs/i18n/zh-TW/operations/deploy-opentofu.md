<!-- source: deploying-with-opentofu 178b85dbd9bd -->
# 以 OpenTofu 部署

使用 OpenTofu 根模組的每個部署都共通的內容：[在 Google Cloud 上部署](deploy-google-cloud.md)和[在 AWS 上部署](deploy-aws.md)會補充它們各自平台所需要的部分。兩個模組都需要 [OpenTofu](https://opentofu.org) 1.11 以上版本。不支援 Terraform。

<!-- source: the-root-modules 4e0bf528bd0e -->
## 根模組

根模組的一個區塊就會部署整個伺服器：服務、它的任務，以及每個角色的後端。

| 根模組 | 平台 | 可複製的範例 |
| --- | --- | --- |
| `mycroblog-gcp` | Google Cloud 上的 Cloud Run | `examples/gcp-cloud-run-neon` |
| `mycroblog-aws` | AWS 上的 Fargate ECS。它從未被套用過 | `examples/aws-ecs-neon` |

範例是平台的 provider 加上一個區塊 `module "mycroblog"`，它的來源是固定在某個發行版本的 Git 來源：

```hcl
module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-gcp?ref=v0.1.0&depth=1"

  host = "social.example.org"
  # the sections, below
}
```

必須固定版本：沒有固定的模組可能會在你不知情時變更 KEK 的資源。伺服器映像檔預設為 `ref` 所指定的發行版本，所以只要 `ref` 就能決定執行的內容，調高它就是升級。模組只存在於包含它的第一個發行版本之後。

<!-- source: sections 728259c0b297 -->
### 區段

每個角色都是區塊的一個區段，是一個物件，由它的 `kind` 挑選後端，例如 `database = { kind = "external" }`。每個欄位都有預設值，也就是建議的配置，所以你只需要寫你要變更的部分。無法運作的組合會在 plan 之前就失敗，並以訊息指出相關的輸入。[模組的參考](../../deploy/opentofu/README.md#sections)列出了每個模組的區段、kind 和欄位。

<!-- source: other-settings-and-secrets 112fdabc5bb0 -->
### 其他設定與機密

`settings` 以鍵接受[設定參考](../reference/configuration.md)中的任何其他設定，模組會把它傳給服務、worker 和任務。`secrets` 接受機密設定的鍵：模組會為每個鍵建立一個空機密 `<name>-<key>`（底線換成連字號），把它傳給伺服器，並列出它讓你填入。

```hcl
  settings = { translator_deepl_url = "https://api.deepl.com" }
  secrets  = ["translator_deepl_api_key"]
```

區段已經設定或模組固定的鍵會使 plan 失敗，模組不接受的鍵（例如 `listen`）也一樣：錯誤訊息會說明原因。為 null 的區段（例如 `translation`），以及設為 `settings` 的 `observability` 訊號，不會設定任何東西，所以 `settings` 可以指名它們的鍵。例如，把錯誤回報送到 Sentry：

```hcl
  observability = { errors = "settings" }
  settings      = { errors_exporter = "sentry" }
  secrets       = ["errors_sentry_dsn"]
```

在平台上手動變更的設定不會持久：下一次 apply 會還原區塊所寫的內容。

<!-- source: secrets d58c587dc144 -->
### 機密

你提供的任何機密值都不會進入 OpenTofu 的 plan 或 state。模組會盡可能透過唯寫（write-only）屬性產生：KEK、VAPID 金鑰，以及它所建立資料庫的密碼。唯一的例外是 `ingress.kind = cloudflare` 的來源機密，它由 Cloudflare 的規則保存，所以會落在 state 中，而 state 必須保持私有。

只有你擁有的機密，例如外部資料庫的 URL、SMTP 密碼、存取金鑰，或 `secrets` 的鍵，模組會建立成空的。`tofu output secrets_to_fill` 會列出每一個、該填入什麼，以及加入它的指令。機密沒有值的伺服器不會啟動。伺服器在執行個體或任務啟動時讀取它的機密，因此新值會在下一個執行個體生效。

**OpenTofu 只產生 KEK 一次，之後不會再產生。** 機密及其版本設有 `prevent_destroy`，而會寫入新值的版本固定在模組中：新的隨機 KEK 會讓舊 KEK 加密的所有內容都無法讀取。若要輪替，請依照[輪替 KEK](key-management.md#rotate-the-kek)，用平台的 CLI 加入新的值，OpenTofu 不會追蹤它。

<!-- source: first-deployment 9ae06fd14fc1 -->
## 首次部署

你執行 apply 的地方需要 `tofu` 1.11 以上版本、`jq` 和平台的 CLI（`gcloud` 或 `aws`），因為 apply 會從那裡執行遷移；還需要一個用於 [`host`](../reference/configuration.md#host) 的網域，之後無法變更（[之後無法變更的設定](production-checklist.md#settings-you-cannot-change-later)）；以及來自[發行版本](running.md#get-the-image)的 `my` 版本。沒有 `latest` 標籤。請先完成平台頁面所要求的事，例如建立資料庫。

<!-- source: 1-copy-the-example-and-set-the-variables 8f1ba80ba8d2 -->
### 1. 複製範例並設定變數

從你要部署的發行版本中，把範例連同 provider 的鎖定檔複製到一個空目錄：

```sh
VERSION=v0.1.0
EXAMPLE=gcp-cloud-run-neon # or aws-ecs-neon
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r "my-source/deploy/opentofu/examples/$EXAMPLE/." my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

**把 `main.tf` 中的 `ref` 設為該發行版本**，穩定版的副本已經設定正確。若是預先發行版本，請同時設定 `ref` 和映像檔（版本不帶 `v`），因為預先發行版本的模組不會預設使用它自己的映像檔：`sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`，並在區塊中設定 `image = { version = "0.1.0-rc.4" }`。

填寫 `terraform.tfvars`，並把你要變更的區段加入 `main.tf` 的區塊。如果帳號代碼要使用其他網域，請現在就決定 [`account_domain`](../reference/configuration.md#account_domain)，寫成 `settings = { account_domain = "example.org" }`。

請保留這個目錄：它保存了你的設定，預設也保存了 state。state 不含任何機密的值，但它是建立了哪些資源的唯一記錄：如果有多人執行 apply，請把它保存在遠端後端，例如 `gcs` 或 `s3` bucket。

<!-- source: 2-create-the-secrets-and-add-their-values 9f13e8109bc9 -->
### 2. 建立機密並加入它們的值

```sh
tofu init
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

`tofu init` 會以你的區塊的名稱，擷取該發行版本的模組與它的腳本：`migrate.sh` 由 apply 執行，`console.sh` 則執行[主控台](#the-console)。

先只套用機密，這會建立它們，並以 KEK 種類對應的模組產生 KEK 和 VAPID 金鑰：

| 根模組 | 套用 |
| --- | --- |
| `mycroblog-gcp` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager` |
| `mycroblog-aws` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secrets_manager` |

使用 `kek = { kind = "kms" }` 時，請以 `module.mycroblog.module.kek_kms` 取代 KEK 機密作為目標。

**現在就複製 KEK**，並把副本與資料庫分開保存，例如存在密碼管理器中。沒有它，就無法還原[備份](backup.md)，帳號的金鑰也會遺失。`tofu output kek` 有對應的指令。VAPID 金鑰 `<name>-vapid-key` 也請比照辦理：遺失它會使所有 Web Push 訂閱失效。

接著用各自的值執行 `tofu output secrets_to_fill` 的每個指令。請在下一步之前全部加入。

<!-- source: 3-apply b9f715c7b55d -->
### 3. 套用

```sh
tofu apply
```

這會建立遷移任務和它所依賴的資源，透過 `migrate.sh` 在其中執行 `my upgrade`，並等待它完成：依序是 `my config check`、`my db upgrade -check`、`my schema apply` 和 `my db upgrade`，並在第一個失敗的指令處停止。它的輸出會出現在 apply 的輸出中。第一個使用資料庫的指令會把 `host` 和 `account_domain` 記錄在資料庫中。到那時 apply 才會建立各個服務，而它們的健康檢查在結構建立之前會失敗。

如果遷移失敗，apply 會在建立服務之前停止。請修正原因（常見的是沒有值的機密），再執行一次 `tofu apply`：它會再次執行遷移。

<!-- source: 4-point-the-domain-at-the-server b7b1ead901bc -->
### 4. 把網域指向伺服器

`tofu output dns_records` 會列出要在你的 DNS 服務商加入的記錄；模組自己建立記錄時，它是空的。

<!-- source: 5-create-the-first-administrator bb599aec43c2 -->
### 5. 建立第一位管理員

主控台沒有終端機也沒有標準輸入，因此 `my admin create` 不會讀取密碼，而是輸出一個設定密碼的連結：

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

請立即開啟連結並設定密碼。它只能使用一次，有效期限為一小時，**在那之前它會留在平台的日誌中**，任何能讀取日誌的人都可以使用它。如果它過期了，請透過主控台執行 `my admin account password-reset-link @alice`。接著請開啟兩步驟驗證。

<!-- source: 6-go-through-the-checklist 12999c41613d -->
### 6. 逐項檢查清單

逐項確認[正式環境檢查清單](production-checklist.md)，從[電子郵件](production-checklist.md#email)開始：在 `mail.from` 的網域上設定 SPF、DKIM 和 DMARC。

<!-- source: the-console 1b3d6dfa9c3c -->
## 主控台

主控台會以服務的映像檔、設定和機密執行一個 `my` 指令。它只在 `console_enabled` 開啟時存在，因此用完後請關閉：

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` 會等待指令完成，輸出它從平台日誌讀回的結果，並以該指令的結束代碼結束。它使用服務正在執行的映像檔：在停在遷移的升級之後，那會是前一個發行版本，資料庫仍然相容。

指令沒有標準輸入也沒有終端機，因此需要從它們讀取密碼或檔案的指令無法以這種方式執行。指令輸出的內容會留在日誌中，因此請不要執行你不願寫進日誌的指令。

<!-- source: upgrading 4be8a01734ce -->
## 升級

請先閱讀[升級](database.md#upgrading)和版本說明，並且一次只升一個次要版本。升級就是套用新的發行版本：

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

套用之前請先閱讀 plan：新版本的模組也可能變更資源。apply 會執行以下步驟，並在第一個失敗的步驟停下：

1. 為遷移任務和主控台任務（在 AWS 上是 task definition）指定新的映像檔，連同 plan 中服務不依賴的其餘部分。
2. 從你執行 apply 的地方，透過 `migrate.sh` 在遷移任務中執行 `my upgrade` 並等待它完成，做法與[首次部署](#3-apply)相同。它會先等待被中斷的 apply 所留下仍在執行的任何一次執行。
3. 把每個 worker 移到新的映像檔，再移動服務。只有當它的健康檢查通過時，新的 revision 才會接收流量。

**執行 apply 的地方**需要 `jq`、平台的 CLI，以及可以執行該任務並讀取它的日誌的憑證，平台的頁面會列出它們。執行 apply 的 CI 任務也一樣。

**當某個步驟失敗時**，服務會停留在它們的 revision，版本說明或遷移的輸出會說明原因。請修正原因並以相同的版本再執行一次 `tofu apply`：它會再次執行遷移，已完成的步驟會發現沒有剩下的工作。以相同的映像檔執行 apply 不會執行任何東西。若結構已經改變而要回復，請還原備份：請參閱[回復](database.md#rolling-back)。以較早的版本執行 apply 會停在遷移，因為該發行版本會拒絕較新的結構，也不會變更任何服務。

若要試用候選版本，或要在較新的模組下保留較早發行版本的映像檔，請自行設定版本；它是映像檔標籤，不帶發行版本的 `v`：

```hcl
  image = { version = "0.2.0-rc.1" }
```

<!-- source: checking-the-sql-first 84178f101ce0 -->
### 先檢查 SQL

plan 不會顯示遷移的 SQL。若要在 apply 之前閱讀它，請只套用遷移任務並換上新的映像檔，並在其中執行不會變更任何內容的 `my upgrade -dry-run`，然後閱讀它的日誌。平台的頁面有相關的指令。

<!-- source: destructive-changes 0bea88d60224 -->
### 破壞性變更

當版本說明要求使用 `-allow-destructive` 時，請在區塊中指定該版本：

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

`my upgrade` 只會在套用該版本的那一次 apply 取得 `-allow-destructive`，因此留著這個值，也不會讓之後的發行版本刪除資料。

<!-- source: pinning-the-image-by-digest 9c3f49e5ee10 -->
### 以 digest 固定映像檔

[正式環境檢查清單](production-checklist.md#operations)建議使用版本說明中的 digest：

```hcl
  image = { digest = "sha256:..." }
```

digest 只指向一個映像檔，無論旁邊的標籤寫什麼，所以請與 `ref` 在同一次編輯中一起變更：沿用前一個發行版本的 digest，會讓新版本繼續使用那個發行版本的映像檔。

<!-- source: the-maintenance-mode 57bd5487b045 -->
### 維護模式

[維護模式](maintenance.md)需要三次 apply，因為服務會在升級之前和之後各變更一次。把它加入區塊的設定並套用，進行備份，調高 `ref` 並套用，然後移除它再套用：

```hcl
  settings = { maintenance = "true" }
```

沒有它時，舊的 revision 會在遷移期間繼續提供服務，這[不保證能正常運作](database.md#the-server-during-an-upgrade)。

<!-- source: splitting-jobs-into-workers 6336c0873c82 -->
## 把任務分到 worker

當某一種任務拖慢了其他任務時，請給它專屬的服務。請依[依任務類型分開 worker](running.md#splitting-workers-by-job-type)的說明，根據指標決定要分出哪些任務。`workers` 接受一個映射，鍵是每個 worker 的名稱，值帶有它的 `job_types`，而模組會由它產生這次拆分的每個佇列、路由和設定，因此這些都不需要你自己寫。若要避免傳送拖慢其他任務：

```hcl
  workers = {
    delivery = { job_types = ["federation.fanout", "federation.deliver"] }
  }
```

若要讓電子郵件和通知保持快速：

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
    }
  }
```

plan 會拒絕把必須放在一起的類型分開。依該頁面的說明，請排除你尚未設定的設定所對應的類型：伺服器未註冊的類型會讓啟動停止，而 plan 無法看出是哪些。預設服務會執行沒有任何 worker 指名的每一種類型，而不執行任何 worker 指名的類型。

worker 也接受它的服務的大小，沒有指定的部分各沿用預設服務的值：[模組的參考](../../deploy/opentofu/README.md#workers)列出了每個平台的欄位。worker 不接收來自外部的流量，升級會在遷移之後、預設服務之前移動它。每個 worker 都可能帶來服務的費用，它的執行個體的連線也會計入資料庫的上限。

<!-- source: changing-a-backend df886f733f8e -->
## 更換後端

變更該區段的 `kind`。每種 kind 都會在伺服器上設定它所需要的東西，而 plan 會告訴你有什麼變更。更換資料庫也代表要搬移它的資料，因為新的資料庫是空的：請為舊的資料庫做[備份](backup.md)，並在沒有任何寫入時把它還原到新的資料庫。

<!-- source: the-kek 62356f591998 -->
### KEK

變更 `kek.kind` 只是一行，但在有資料的實例上並非毫無代價。資料庫保存了以目前金鑰包裝的機密，其中包括每個帳號的金鑰，因此失去它的實例將無法再讀取這些機密。請依照[在本機 KEK 與 KMS 之間搬移](key-management.md#move-between-the-local-kek-and-a-kms)。從機密改為 KMS 時，在機密搬移期間，請把目前的 KEK 保留為 `kek_previous`：加入 `secrets = ["kek_previous"]`，並以目前的 KEK 填入機密 `<name>-kek-previous`。透過[主控台](#the-console)執行 `my kek rewrap`，等 `my kek status` 不再計算到 `local:` 金鑰時再移除它。只要你還保留 rewrap 之前的資料庫備份，就請保留該 KEK。

在沒有資料的新實例上，不需要搬移任何東西。
