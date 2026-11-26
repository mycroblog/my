<!-- source: deploying-on-aws d7e8643cd93d -->
# 在 AWS 上部署

本頁說明如何使用 OpenTofu 模組 [`mycroblog-aws`](../../deploy/opentofu/README.md#aws) 在 AWS 上執行 `my`：Application Load Balancer 後方的 Fargate ECS、保存在資料庫中的任務、S3、SNS，以及透過 Amazon SES 寄送的電子郵件。沒有任何東西會縮減至零：在資料庫之前，閒置時預計每月約 $42，請閱讀[費用](#costs)。

**其中沒有任何東西曾在 AWS 上實際套用過。** 檢查是在模擬的 provider 上、沒有憑證的情況下規劃它，這只能說明設定前後一致，不能說明 AWS 會接受它，或伺服器能啟動。請把第一次部署當成測試。

每個 OpenTofu 部署都共通的內容，包括模組的輸入、機密、主控台和升級，都在[以 OpenTofu 部署](deploy-opentofu.md)。本頁補充 AWS 所需要的部分。

<!-- source: what-it-creates 1ca642b05ab0 -->
## 建立的資源

`mycroblog-aws` 的一個區塊會建立以下所有內容。每一列是模組的一個區段，它的預設值就是建議的配置；每個欄位都列在[模組的參考](../../deploy/opentofu/README.md#sections-of-mycroblog-aws)。

| 部分 | 區段 | 預設 | 其他種類 |
| --- | --- | --- | --- |
| 伺服器 | `service`、`jobs` | 一個 Fargate 上的 ECS 叢集和服務，`role = all`，遷移用的 task definition、需要時才存在的主控台 task definition，以及日誌群組 `/ecs/<name>` | |
| 網路 | `network` | 橫跨兩個以上可用區域的 VPC，每個區域各有一個公開子網路和一個私人子網路 | 你自己的 VPC：[網路](#networks) |
| 流量 | `ingress` | 一個一律存在的 Application Load Balancer，搭配你的憑證 | 它前面的 CloudFront 或 Cloudflare：[流量如何到達服務](#how-traffic-reaches-the-service) |
| 任務 | | 無：task 保持運作，並從資料庫領取任務 | |
| 資料庫 | `database` | 你自己執行的 PostgreSQL（例如 Neon）的 URL 機密 | Amazon RDS：[選擇資料庫](#2-choose-the-database) |
| 媒體 | `media` | 一個私人 S3 bucket | Cloudflare R2，或任何 S3 相容的儲存空間 |
| 快取與速率限制 | `kv` | 無：由資料庫保存 | ElastiCache Serverless for Valkey，或你自己執行的 Valkey |
| 即時更新 | `stream` | 一個 SNS 主題。伺服器會自行建立和移除自己的 SQS 佇列 | 資料庫，或 `kv` 的 Valkey |
| 電子郵件 | `mail` | `mail.from` 的網域的 SES 身分：[透過 Amazon SES 寄送電子郵件](#email-through-amazon-ses) | 任何 SMTP 服務 |
| 機密 | | Secrets Manager：產生的 VAPID 金鑰，以及只有你能填入、目前為空的機密 | |
| KEK | `kek` | Secrets Manager 中的一個產生機密 | AWS KMS 中的一把金鑰，它絕不會離開那裡 |
| 指標、追蹤、錯誤 | `observability` | CloudWatch 中的指標與錯誤回報，追蹤則透過每個 task 中的收集器送到 X-Ray：[追蹤到 X-Ray](#traces-to-x-ray) | 你自己的匯出器，透過 `settings` |
| 警示 | `monitoring` | 一個 CloudWatch Synthetics canary 和警報：[監控與警示](#monitoring-and-alerts) | 無，設為 `null` |

`translation = {}` 會加入 Amazon Translate，`search = { domain_name = "..." }` 會加入你所建立的 Amazon OpenSearch Service 網域，`billing = {}` 則會從 Cost Explorer 匯入使用費用：請參閱[翻譯貼文](translation.md#amazon-translate)、[搜尋引擎](search-engines.md)和[匯入雲端使用費用](billing-import.md)。

<!-- source: before-you-start 9d5614a650e1 -->
## 開始之前

- [首次部署](deploy-opentofu.md#first-deployment)所需要的東西，以 `aws` 作為平台的 CLI。
- 一個只供這個實例使用的 AWS 帳號，以及它的憑證，其權限必須能建立上述的所有東西，例如 `AdministratorAccess`。
- 一個區域。SES 從那裡寄信，模組從 `aws` provider 取得它。
- 建議：在 Route 53 中有 `host` 的公開託管區域，這樣模組就會建立 `host` 的記錄，在區域頂點（zone apex）也一樣。

<!-- source: first-deployment 26b562a3db03 -->
## 首次部署

請在這裡做前三個步驟，再依照[首次部署](deploy-opentofu.md#first-deployment)進行，並參考第四步的說明。

<!-- source: 1-request-the-certificates 3b26d03ba06c -->
### 1. 申請憑證

負載平衡器需要 AWS Certificate Manager 中、該區域內 `host` 的已驗證憑證。模組以 ARN 接受它，因為在 apply 之內驗證它，會讓 apply 一直等到你加入只有 apply 才會印出的記錄。

```sh
aws acm request-certificate --region ap-northeast-1 --domain-name social.example.org --validation-method DNS
aws acm describe-certificate --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN" \
  --query 'Certificate.DomainValidationOptions[].ResourceRecord'
# Add that CNAME record at your DNS provider, then:
aws acm wait certificate-validated --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN"
```

使用 `ingress.kind = cloudfront` 時，請在 `us-east-1` 再申請一個，那是 CloudFront 唯一接受憑證的區域；在 `us-east-1` 時，一個憑證就能同時用於兩者。使用 `ingress.media_host` 時，憑證也必須涵蓋該主機。

<!-- source: 2-choose-the-database d9cc7b8b2210 -->
### 2. 選擇資料庫

由 `database.kind` 挑選。不論哪一種，task 都會大約每秒輪詢一次資料庫來領取任務，所以它從不休息。

**`external`，預設也是建議的選擇**，會從你填入的機密讀取你自己執行的 PostgreSQL（例如 [Neon](https://neon.com)）的 URL。模組絕不會建立資料庫，因此它的密碼不會進入 OpenTofu 的 state。在 Neon 上，請在靠近你所在的區域建立專案，並複製它的連線字串：

- **直接**連線字串（主機名稱不含 `-pooler`）是必要的，放在機密 `<name>-database-url` 中。遷移和主控台一律使用它：`my schema apply` 會持有工作階段層級的鎖，而交易模式的連線集區不會保持這個鎖。
- **集區**連線字串（主機名稱含 `-pooler`）是選用的，供服務使用：設定 `database = { kind = "external", pooled = true }`，模組就會建立機密 `<name>-database-url-pooled`。請參閱[透過連線集區](database.md#through-a-connection-pooler)。

請預期使用 Neon 的 Launch 方案：由於運算資源永遠不會停止，Free 方案的運算時數會在一個月內用完。

**`rds`** 會在私人子網路中建立 Amazon RDS for PostgreSQL，密碼由模組產生並自行寫入 `<name>-database-url`，所以沒有東西需要填入：

```hcl
  database = { kind = "rds" }
```

預設是 `db.t4g.micro` 和 20 GB（`instance_class`、`allocated_storage_gb`），保留 7 天的自動備份，並啟用刪除保護。它沒有公開位址，所以只有 task 連得到它：`pg_dump` 必須從 VPC 內部執行，而升級之前要改用快照，`aws rds create-db-snapshot --db-instance-identifier my --db-snapshot-identifier my-before-upgrade`。它沒有集區 URL。

<!-- source: 3-write-the-block db167b7d9566 -->
### 3. 寫出區塊

`examples/aws-ecs-neon` 是一種開始的方式：`aws` provider 加上一個使用預設值的區塊，它的變數列在 `terraform.tfvars.example` 中。不論你從什麼開始，區塊都需要這些：

```hcl
provider "aws" {
  region = "ap-northeast-1"
}

module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-aws?ref=v0.1.0&depth=1"

  host       = "social.example.org"
  network    = { egress = "public-ip", availability_zones = ["ap-northeast-1a", "ap-northeast-1c"] }
  ingress    = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/00000000-0000-0000-0000-000000000000" }
  media      = { bucket_name = "social-example-org-media" }
  mail       = { from = "noreply@social.example.org" }
  monitoring = { notification_emails = ["ops@social.example.org"] }
}
```

- `network.egress` 沒有預設值，因為兩種對外的方式都是依小時計費：請閱讀[網路](#networks)。
- `ingress.route53_zone_id` 指定 `host` 的託管區域（如果你有的話）。
- `media.bucket_name` 在整個 AWS 中必須是唯一的。
- `monitoring` 指定警報要通知誰，或設為 `null`。

請加入你所選資料庫的區段，以及你要變更的任何其他區段。

<!-- source: 4-deploy 891c5c7cef83 -->
### 4. 部署

請依照[首次部署](deploy-opentofu.md#first-deployment)進行。在 AWS 上，本頁的指令使用預設的 `name`，也就是 `my`：

- **機密**：它們在 Secrets Manager 中，名稱為 `<name>-...`。`tofu output kek` 有複製 KEK 的指令，而 `aws secretsmanager get-secret-value --secret-id my-vapid-key --query SecretString --output text` 會複製 VAPID 金鑰。KEK 以 `MY_KEK` 的形式交給伺服器，因為 Fargate 不掛載檔案。task 在啟動時讀取它的機密：變更其中一個之後，請執行 `aws ecs update-service --cluster my --service my --force-new-deployment`，每個 worker（`my-<key>`）也一樣。
- **套用**：遷移的 task definition 最先建立，連同它所依賴的資源，只有在遷移成功之後才會建立服務。負載平衡器的檢查會請求 `/readyz`，它在結構建立之前會回應 `503`。
- **DNS**：`tofu output dns_records` 會列出 `host` 的記錄（除非由 Route 53 或 Cloudflare 建立）和 SES 的三筆 DKIM 記錄。在沒有 Route 53 的區域頂點，`host` 的記錄需要你的 DNS 服務商所稱的 `ALIAS` 或 `ANAME`，如 `tofu output dns_note` 所述。
- **電子郵件**：在 AWS 授予正式環境存取權之前，SES 只會寄給你已驗證的地址，因此其他人還無法註冊：請閱讀[透過 Amazon SES 寄送電子郵件](#email-through-amazon-ses)。
- **警示**：`monitoring.notification_emails` 的每個地址都會收到 SNS 的訂閱郵件，必須確認它。
- **第一位管理員**：連結會留在 CloudWatch Logs 的 `/ecs/<name>` 中，任何能讀取帳號日誌的人都可以使用它。

<!-- source: the-console-and-upgrades-on-aws 0e4bc0bdeff2 -->
## AWS 上的主控台與升級

[主控台](deploy-opentofu.md#the-console)和[升級](deploy-opentofu.md#upgrading)會從你執行 apply 的地方，透過 `aws` 執行一次性的 Fargate task，並從 CloudWatch Logs 讀取它們的輸出。被中斷的 apply 會讓 task 繼續執行，下一次 apply 會等它結束。升級時，每個 task definition 都會得到帶有新映像檔的新 revision，在某個服務採用它之前，沒有東西會執行它；接著每個服務會等它的新 task 啟動（預設服務還要通過負載平衡器的檢查），舊的才會離開，而 deployment circuit breaker 會把 task 無法啟動的 revision 退回到前一個。

執行 apply 的地方的憑證，或執行 apply 的 CI 任務的憑證，除了 apply 本身所需要的之外，還需要：

- 遷移：`ecs:RunTask`、`ecs:DescribeTasks`、`ecs:ListTasks`、`logs:FilterLogEvents`，以及 task 兩個角色上的 `iam:PassRole`；
- 主控台另外還需要：`ecs:DescribeServices`、`ecs:DescribeTaskDefinition`、`ecs:ListTagsForResource` 和 `ecs:RegisterTaskDefinition`，因為主控台會先取得正在執行的服務的映像檔。

若要[先檢查 SQL](deploy-opentofu.md#checking-the-sql-first)，請在區塊仍指向目前發行版本時開啟主控台，然後調高 `ref`，只套用遷移的 task definition，並用它執行 `my upgrade -dry-run`：

```sh
tofu apply -var console_enabled=true
# Name the new release in main.tf, then:
tofu init
tofu apply -var console_enabled=true -target='module.mycroblog.module.app.aws_ecs_task_definition.task["migrate"]'
tofu output -json console | jq -c '.task_definition = "my-migrate" | .log_prefix = "migrate" | del(.service)' |
  "$SCRIPTS/console.sh" upgrade -dry-run
```

<!-- source: monitoring-and-alerts 9b712c82cae2 -->
## 監控與警示

`monitoring` 會加入 `/readyz` 的 CloudWatch Synthetics canary、CloudWatch 中的指標，以及以[運作手冊](alerts.md#aws-ecs)的名稱命名的警報，它們會通知 `notification_emails` 的地址或 `notification_topic_arns` 的 SNS 主題。這個輸入沒有預設值：請指定誰要收到通知，或設定 `monitoring = null` 完全不使用它。canary 的每次執行都會計費，而 `uptime_period_minutes`（預設為 15）就是它執行的頻率：請閱讀[費用](#costs)。

<!-- source: choosing-the-backends b83753b1f860 -->
## 選擇後端

每個區段的預設值都是建議的 kind；若要換成另一個，請[變更區段的 `kind`](deploy-opentofu.md#changing-a-backend)。資料庫在[上面](#2-choose-the-database)。

| 區段 | 建議 | 其他種類，以及何時使用 |
| --- | --- | --- |
| `media` | `s3`，位於該區域的 bucket，`versioning` 關閉 | 位於 `cloudflare.account_id` 的 `r2`，或位於 `endpoint` 的 `s3-compatible`，各自使用你填入的機密中的存取金鑰。若要共用 bucket，`existing_bucket = true` 會使用你的 bucket，而 `prefix`（`tenant-a/`）會作為每個物件名稱的開頭；搭配 `s3` 時，task 的角色就只能存取它底下的物件 |
| `kv` | `db`，不需要額外費用 | `elasticache`，閒置時也會計費，並位於私人子網路中；或 `external`，你填入的機密中的 Valkey URL |
| `stream` | `sns`，能連到每個 task | `postgres`，透過資料庫的直接連線；或 `valkey`，也就是 `kv` 的 Valkey |
| `mail` | `ses`，沒有密碼 | `smtp`，搭配 `smtp.host`、`smtp.port`（587）和 `smtp.username`，它的密碼放在你填入的機密中 |
| `kek` | `secrets-manager`，你只需複製一次 | `kms`，一把絕不會離開 AWS KMS 的金鑰。在有資料的實例上，[變更它](deploy-opentofu.md#the-kek)是一次搬移 |
| `observability` | CloudWatch 和 X-Ray | `traces = "none"`，或 `"settings"` 用於你要送到其他地方（例如 Sentry）的任何訊號 |

`architecture = "arm64"` 會在 Graviton 上執行 task，費用較低；已發行的映像檔兩者都有。

<!-- source: email-through-amazon-ses 0d8bc9f93296 -->
## 透過 Amazon SES 寄送電子郵件

預設的 `mail.kind = "ses"` 會使用部署所在區域的 SES API，從 `mail.from` 的網域寄信。模組會為該網域建立附有 Easy DKIM 的身分，並允許 task 對它執行 `ses:SendEmail`，所以沒有密碼；`mail.configuration_set` 會建立一個 configuration set 並套用到每封郵件，或搭配 `existing_configuration_set = true` 指定一個由多個實例共用的 configuration set；這時每個實例只會處理它自己所寄郵件的通知。在 SES 寄信給你的使用者之前，請加入 `tofu output dns_records` 所列的記錄，並離開沙盒：[透過 Amazon SES 寄送電子郵件](email-ses.md)有相關步驟，以及伺服器如何得知退信和投訴。

<!-- source: traces-to-x-ray e5ad1c35fdd5 -->
## 追蹤到 X-Ray

`observability.traces = "x-ray"` 是預設值，因為這個收集器符合建議的配置，且不需要額外費用。它是 AWS Distro for OpenTelemetry 收集器，在每個 task 中與伺服器並行執行，只監聽 task 的 loopback，並以 task 的角色（`xray:PutTraceSegments`）寫入 X-Ray。它會從每個 task 取走 32 個 CPU 單位和 128 MiB，而 AWS 只依 task 的大小計費，所以預設的服務 task（256 個 CPU 單位和 1,024 MiB）費用不變，並讓伺服器使用 224 個 CPU 單位和 896 MiB。預設的 `traces_sample_ratio` 為 1%，在每天 100,000 個請求的情況下，每月約記錄 30,000 個追蹤，低於 X-Ray 每月免費記錄的 100,000 個追蹤。

它確實會花費的是每次 task 啟動時拉取的收集器映像檔，以及伺服器的記憶體。當你不讀取追蹤，或 task 太小而無法讓出 128 MiB 時，請設定 `traces = "none"`：plan 會拒絕讓伺服器在 `service`、`jobs` 或 worker 中得到少於 128 個 CPU 單位或 512 MiB 的大小。收集器不是必要的：如果它停止，伺服器會在沒有追蹤的情況下繼續運作。X-Ray 是否會接收它的 span，尚未驗證。

<!-- source: networks 36dc33a6cbef -->
## 網路

ECS 一律需要 VPC。建議的 `network.kind = "vpc"` 會在 `cidr_block`（`10.0.0.0/16`）中建立一個，橫跨 `availability_zones`（至少兩個，負載平衡器和 RDS 需要）：每個區域各有一個公開子網路和一個私人子網路，以及一個網際網路閘道。它只支援 IPv4，所以伺服器連不到只有 IPv6 的對象。`network.kind = "existing"` 則改為使用你自己的 VPC：`vpc_id`、`vpc_cidr`、`public_subnet_ids`、`public_subnet_cidrs` 和 `private_subnet_ids`。

task 需要一條通往網際網路的出口，用於映像檔、資料庫、聯邦和 AWS API，由 `network.egress` 挑選。不論哪一種，來自外部的流量都只經由負載平衡器到達 task：

| | `public-ip`（建議） | `nat` |
| --- | --- | --- |
| task 的子網路 | 公開，每個 task 有一個公開的 IPv4 位址 | 私人 |
| 每小時 | 每個 task 的位址 $0.005 | 一個 NAT 閘道約 $0.045，加上它的位址 |
| 每 GB | 位址沒有費用 | 經過閘道約 $0.045，包含媒體和聯邦 |
| 搭配 `existing` | 沒有額外費用 | 你的私人子網路必須已經能連到網際網路 |

除非你的政策禁止公開子網路，否則請使用 `public-ip`：它的費用低得多，也沒有比較開放。`vpc` 會在第一個區域建立一個 NAT 閘道：當該區域故障時，私人子網路會失去網際網路，直到它恢復。不論哪一種，RDS 和 ElastiCache 都位於私人子網路中，不需要出口。

<!-- source: how-traffic-reaches-the-service d009efcf3ee2 -->
## 流量如何到達服務

負載平衡器一律存在，使用 `ingress.certificate_arn` 的憑證，而 task 只接受它的連線。`ingress.kind` 決定它前面放什麼：

| | `load-balancer`（建議） | `cloudfront` | `cloudflare` |
| --- | --- | --- | --- |
| `host` 指向 | 負載平衡器 | 一個 CloudFront distribution | Cloudflare，它再連到負載平衡器 |
| 快取 | 無 | `/media/`，依伺服器的 `Cache-Control` | `host` 上沒有；`media_host` 會加入一個有快取的主機 |
| 記錄 | Route 53 以 `route53_zone_id` 建立，或由 `dns_records` 列出 | Route 53 搭配 `route53_zone_id`，或 `dns_records` | 由 Cloudflare 建立 |
| 需要 | | `cloudfront_certificate_arn`，位於 `us-east-1`。`cloudfront_price_class` 挑選邊緣節點 | `cloudflare = { zone_id = "..." }`，以及 `CLOUDFLARE_API_TOKEN` 中的權杖 |
| 以什麼關閉來源 | task 的安全群組 | 負載平衡器的安全群組只接受 CloudFront，而它的 listener 會拒絕沒有 CloudFront 所送標頭的請求 | Cloudflare 的規則所加入、由伺服器檢查的標頭 |

在 CloudFront 或 Cloudflare 後方時，標頭的值會落在 OpenTofu 的 state 中，而 state 必須保持私有。CloudFront 之前就有的負載平衡器記錄必須先移除：相同名稱的既有記錄會讓 apply 失敗。CloudFront 的位址是在 plan 時從 AWS 取得，所以請不時 apply 一次，否則使用新位址的邊緣節點會與它的檢視者共用速率限制。

`ingress.media_host` 會在 `load-balancer` 或 `cloudflare` 旁邊加入 Cloudflare 上有快取的媒體主機，並搭配 `cloudflare.zone_id`。被刪除的檔案會一直留在快取中，直到你清除它：請閱讀[透過 CDN 提供媒體](cdn.md)。不論 Cloudflare 位於什麼前面，請在它的儀表板中關閉 Bot Fight Mode，因為對 `/ap/`、`/.well-known/` 或 `/api/` 的挑戰會破壞聯邦，而模組不管理它。Cloudflare 的種類只在模擬的 provider 上執行過。

<!-- source: limits 42ed50837851 -->
## 限制

- **健康**：負載平衡器唯一的檢查會請求 `/readyz`，它在資料庫停止時也會失敗：連續失敗 2.5 分鐘後，ECS 會替換 task，並持續這樣做，直到資料庫恢復。沒有容器檢查，所以沒有負載平衡器的 worker，只有在它的日誌中才看得到映像檔有問題。
- **上傳**：負載平衡器不對請求內容設限，所以由伺服器的 `media_max_*` 設定決定。
- **串流**：串流每 30 秒送出一次心跳，低於負載平衡器 60 秒的閒置逾時。它會一直保持開啟，直到它的 task 離開，之後用戶端會重新連線。
- **用戶端位址**：速率限制仰賴伺服器所信任的內容，而模組會根據入口設定它。請檢查最初幾筆請求日誌中的用戶端位址：如果是負載平衡器子網路的位址或 CloudFront 的位址，就表示它是錯的。
- **磁碟**會隨 task 消失，所以 plan 會拒絕 SQLite 和檔案系統上的媒體。
- 位於客戶管理的 KMS 金鑰之下的**機密**，也需要 `kms:Decrypt`，而模組不會授予它。

<!-- source: costs 19e0e43bc201 -->
## 費用

ECS 上沒有任何東西會縮減至零：task、負載平衡器和它的公開位址整個月都會計費。這些數字取自 AWS 東京的價格表，時間為 2026-10 和 2026-11，除非某一列另有說明，而且尚未與帳單核對過。

| 部分 | 每月 | 選擇 |
| --- | --- | --- |
| 0.25 vCPU 和 1 GB 的 task | x86 為 $13.26，arm64 為 $10.61 | `service`、`architecture`。每個 worker 會加上它自己的 task |
| 負載平衡器 | $17.74，另加整月使用的每個容量單位 $0 到 $5.84 | 一律存在 |
| 負載平衡器的公開位址 | $7.30，兩個區域 | 每個區域一個 |
| 出口 | 搭配 `public-ip` 時每個 task $3.65；搭配 `nat` 時約 $36.50，另加每 GB $0.045（俄亥俄的費率） | `network.egress` |
| 資料庫 | Neon Launch 方案以 0.25 CU 計算約 $19，因為運算資源永遠不會停止；搭配 `rds`、`db.t4g.micro` 和 20 GB 約 $21 | `database` |
| 快取與速率限制 | 搭配 `db` 為 $0；搭配 `elasticache` 約 $6.13，或依 AWS 的另一份文件約 $61（維吉尼亞北部） | `kv` |
| canary | 每 15 分鐘一次約 $3.34；每分鐘一次約 $51.72 | `monitoring.uptime_period_minutes` |
| 追蹤 | 每月 100,000 個追蹤以內為 $0 | `observability.traces` |

**閒置時，搭配 `public-ip` 約 $42，搭配 `nat` 約 $75**，另加容量單位，這還沒計入資料庫、儲存空間、流量和 canary。沒有讀取、因此不在任何一列的項目有：CloudFront、Secrets Manager、CloudWatch Logs、S3、SNS 和 SQS、離開 AWS 的資料，以及任何免費額度。一次性的 task 會依它們執行的分鐘數，以它們的大小計費，而升級會有幾分鐘多執行一個服務 task。
