<!-- source: importing-cloud-usage-costs db52fcd17ff9 -->
# 匯入雲端使用費用

伺服器每 24 小時會匯入設定 [`billing_sources`](../reference/configuration.md#billing_sources) 中列出的服務供應商的使用費用，或是透過[外掛](#a-plugin-billing_sources--plugin)匯入伺服器沒有轉接器的供應商的費用，並加到實例的支出中。預設為空，不會匯入任何東西。它會匯入當前與上一個帳單月份的費用（上一個月份會匯入到當月 15 日為止，或直到它確定為止），每個服務與費用類型（使用量、抵用額、退款、稅金、支援、手續費、其他）各一列。在供應商結算該月份之前，數值會記錄為估計值；結算後記錄為確定值；確定後若數值有變動，則記錄為更正。過去的紀錄絕不會被改寫。匯入的狀態（最後一次成功、最後一次失敗的類型，以及本月的請求次數）會顯示在管理介面的支出頁面，也可以透過 `GET /api/v1/admin/billing_imports` 取得。指標會依來源、結果與失敗類型計算每次嘗試（`my.billing.imports`），並回報最新一次嘗試失敗的來源（`my.billing.import.failing`）；[警示與操作手冊](alerts.md#mybillingimportfailing)中有針對它們的警示。

這個伺服器的設定中不保存任何憑證。請在執行環境中提供各供應商 SDK 預設憑證鏈會讀取的內容（AWS 為執行角色、環境變數或共用憑證檔案；Google Cloud 為應用程式預設憑證）。

<!-- source: with-the-opentofu-modules 51ec4f78f058 -->
## 使用 OpenTofu 模組

模組 [`billing-aws`](../../deploy/opentofu/modules/billing-aws/README.md) 與 [`billing-gcp`](../../deploy/opentofu/modules/billing-gcp/README.md) 會設定 `billing_sources`，並完成本指南中供應商 API 允許的部分。`billing-gcp` 只能搭配 Google Cloud 的應用程式模組使用，因此在 AWS 上，這些模組只會匯入 AWS 的費用。`billing-aws` 可以搭配 AWS 的應用程式模組與 Cloud Run 的應用程式模組使用，在後者中兩者可以同時放在 `backends` 中。兩者都還沒有在真實帳戶上套用過。

| 本指南中的步驟 | 使用模組時 |
| --- | --- |
| IAM 政策，以及「從 Cloud Run 以 web identity 呼叫」的角色 | `billing-aws` |
| 啟用 Cost Explorer、Linked Account Access、成本分配標籤 | 在 AWS 主控台中手動進行 |
| 匯出的資料集，以及「IAM 角色」中的角色 | `billing-gcp` |
| 啟用匯出（「Standard usage cost」） | 在 Google Cloud 主控台中，以帳單帳戶上的角色手動進行 |
| 預算的主題與 pull 訂閱，以及訂閱者角色 | `billing-gcp`，搭配 `method = "budget"` |
| 建立預算並將它連接到主題 | 在 Google Cloud 主控台中，以帳單帳戶上的角色手動進行 |
| 啟用 BigQuery 或 Pub/Sub API | 手動進行，方法同[在 Cloud Run 上部署](deploy-cloud-run.md#1-enable-the-apis) |
| 外掛 | 這些模組不處理。`backends` 中沒有帳單模組時，請在應用程式模組的 `env` 中設定 `MY_BILLING_SOURCES = plugin` 與[外掛](#a-plugin-billing_sources--plugin)的設定；與帳單模組並用時，應用程式模組會拒絕 `env` 中的 `MY_BILLING_SOURCES`，因此請自行新增一個回傳 `billing_sources = ["plugin"]` 的 backend（[backend 的欄位](../../deploy/opentofu/README.md)） |

<!-- source: aws-billing_sources--aws 9e5fa0132ca3 -->
## AWS（`billing_sources = aws`）

伺服器會針對每個帳單月份（UTC 的日曆月）呼叫一次 AWS Cost Explorer 的 `GetCostAndUsage`，依服務（`SERVICE`）與紀錄類型（`RECORD_TYPE`）分組。端點一律是 `us-east-1`。金額以美元計。當 Cost Explorer 把某個月份標示為估計（`Estimated`）時，伺服器會把到前一天為止的金額記錄為估計值；不再標示後，則記錄為確定值。以建置標籤 `no_billing_aws` 建置的執行檔會拒絕 `aws`（請參閱 [build-tags.md](../reference/build-tags.md)）。

<!-- source: cost-and-limits a555f91984c3 -->
### 費用與上限

Cost Explorer API 每個分頁請求收費 0.01 美元。小型帳戶每天只需要一個單頁請求，每月約 0.31 美元。為了避免意外的費用，伺服器會計算每個 UTC 日（`billing_aws_max_requests_per_day`，預設 4）與每個 UTC 日曆月（`billing_aws_max_requests_per_month`，預設 120 = 1.20 美元）的請求次數，達到上限後就停止，不再呼叫。排程匯入、任務重試，以及管理介面中的「立即匯入」都會計入。達到上限的匯入會記錄為類型 `request_cap` 的失敗，並由隔天的匯入（每日上限時）或下個月第一天的匯入（每月上限時）再試一次。匯入在呼叫前會先預留上限所剩的請求次數，因此同時執行的多個匯入加起來也不會超過上限。同一個供應商的匯入一次只執行一個：在另一個匯入尚未完成時開始的匯入，會在一分鐘後再試；已經成功匯入的任務再次執行時不會進行任何呼叫。在完成前就停止（程序已結束）的匯入，會把它預留的請求次數計入，下一個匯入最多會等它 15 分鐘。

<!-- source: enable-cost-explorer 2f6cd1add48d -->
### 啟用 Cost Explorer

1. 在 Billing and Cost Management 主控台中開啟 Cost Explorer 並啟用它。啟用後就無法停用。資料最多需要 24 小時才會準備好。
2. 尚未啟用的帳戶會回傳 `DataUnavailableException`，匯入會以 `not_enabled` 失敗。

<!-- source: iam-permissions 9f1c23f3cb5c -->
### IAM 權限

唯一需要的權限是 `ce:GetCostAndUsage`（資源為 `*`）。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": "ce:GetCostAndUsage", "Resource": "*" }
  ]
}
```

在各執行環境中傳遞憑證的方式：

| 執行環境 | 憑證 |
| --- | --- |
| AWS ECS | 把這個政策附加到任務角色。不需要任何設定 |
| Google Cloud（Cloud Run） | 建立下方「從 Cloud Run 以 web identity 呼叫」中的 IAM 角色，並把它的 ARN 設定在 `billing_aws_role_arn` 中。不需要 AWS 金鑰 |
| 你自己的伺服器 | 以 `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`（或共用憑證檔案與 `AWS_PROFILE`）傳遞具有此政策的 IAM 使用者存取金鑰。請從機密資訊儲存區傳遞 |

缺少權限或憑證無效時會以 `permission` 失敗。

<!-- source: calling-from-cloud-run-with-web-identity 7d109914680a -->
### 從 Cloud Run 以 web identity 呼叫

設定 `billing_aws_role_arn` 時，伺服器會從中繼資料伺服器取得 Cloud Run 服務帳戶的 Google ID 權杖（audience 為該角色的 ARN），並以 `AssumeRoleWithWebIdentity` 將它交換為該角色的臨時憑證。

1. 在 Google Cloud 中，記下 Cloud Run 服務的服務帳戶的唯一 ID（數字形式的 `uniqueId`）：`gcloud iam service-accounts describe <email> --format='value(uniqueId)'`。
2. 在 AWS IAM 中，以上述權限的政策建立角色，並讓它的信任政策信任 `accounts.google.com`。

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Principal": { "Federated": "accounts.google.com" },
         "Action": "sts:AssumeRoleWithWebIdentity",
         "Condition": {
           "StringEquals": {
             "accounts.google.com:aud": "<uniqueId of the service account>",
             "accounts.google.com:oaud": "<ARN of this role>"
           }
         }
       }
     ]
   }
   ```

   `accounts.google.com:aud` 對應權杖的 `azp`（服務帳戶的 ID），`accounts.google.com:oaud` 則對應權杖的 `aud`（伺服器請求的 audience，也就是該角色的 ARN）。
3. 設定 `MY_BILLING_AWS_ROLE_ARN=arn:aws:iam::<account ID>:role/<role name>`。

這個設定尚未在真實帳戶上驗證過。

<!-- source: calling-from-a-linked-account-of-organizations d12e10b51af3 -->
### 從 Organizations 的連結帳戶呼叫

AWS Organizations 的管理帳戶可以看到組織中每個成員帳戶的費用。若要以成員（連結）帳戶的憑證呼叫，請在管理帳戶的 Cost Explorer 偏好設定中開啟「Linked Account Access」。關閉時，不論 IAM 權限為何，呼叫都會被拒絕，匯入會以 `not_enabled` 或 `permission` 失敗。

<!-- source: choosing-which-costs-to-import-filtering 60cfa862b6a2 -->
### 選擇要匯入哪些費用（篩選）

以 `billing_aws_filter` 選擇。

| `billing_aws_filter` | 匯入的費用 | 其他設定 |
| --- | --- | --- |
| `none`（預設） | 憑證能看到的所有費用 | 無 |
| `linked_account` | 你指定的成員帳戶的費用（`LINKED_ACCOUNT`） | `billing_aws_linked_accounts`（12 位數的帳戶 ID，以逗號分隔） |
| `tag` | 帶有你指定的成本分配標籤的資源的費用 | `billing_aws_tag`（`key=value`，例如 `app=my`） |

最準確的做法是為這個伺服器準備專用的 AWS 帳戶（可以是 Organizations 的成員帳戶），並以 `none`（用該帳戶的憑證呼叫）或 `linked_account`（從管理帳戶呼叫）匯入。稅金、支援費與部分抵用額無法加上標籤，可能會被標籤篩選排除（尚未在真實帳戶上驗證）。

<!-- source: activating-a-cost-allocation-tag-billing_aws_filter--tag 8e642123a388 -->
### 啟用成本分配標籤（`billing_aws_filter = tag`）

資源上的標籤要啟用為成本分配標籤之後，才會出現在帳單資料中。

1. 在這個伺服器的資源（ECS 服務、RDS、S3 儲存貯體、負載平衡器等）上加上相同的標籤（例如 `app=my`）。
2. 在 Billing and Cost Management 主控台的「Cost allocation tags」中選擇標籤鍵並啟用它。加上標籤後，最多需要 24 小時才會出現在這份清單中；啟用後，最多還需要 24 小時才會出現在帳單資料中。
3. 若要反映啟用前那段期間的費用，請使用同一個畫面上的「Backfill」（API 為 `StartCostAllocationTagBackfill`）。
4. 設定 `MY_BILLING_AWS_FILTER=tag` 與 `MY_BILLING_AWS_TAG=app=my`。

<!-- source: metric 3e034dd351bd -->
### 指標

`billing_aws_metric` 可以是套用折扣（Savings Plans、預留執行個體、議定價格）後的 `NetUnblendedCost`（預設，較接近請款單），或套用折扣前的 `UnblendedCost`。Cost Explorer 的數值與請款單可能因更新時間與四捨五入而有差異。兩者不同時，以請款單為準。

<!-- source: kinds-of-failure 2a6f9042f460 -->
### 失敗類型

| 類型 | AWS 回應 | 修正方式 |
| --- | --- | --- |
| `permission` | `AccessDeniedException`、無效的憑證 | 檢查 IAM 權限與憑證的傳遞方式 |
| `not_enabled` | `DataUnavailableException` | 啟用 Cost Explorer。若是連結帳戶，請啟用 Linked Account Access |
| `not_found` | `BillExpirationException` | Cost Explorer 已不再保有要匯入期間的帳單資料 |
| `throttled` | `LimitExceededException` | 任務會在暫停後重試（會計入請求上限） |
| `unavailable` | 5xx、網路故障 | 任務會在暫停後重試（會計入請求上限） |
| `invalid_response` | 無法讀取的回應、`ValidationException` 等 | 檢查設定（標籤、帳戶 ID） |
| `request_cap` | （不會進行呼叫） | 等待受限的日或月份過去，或提高上限 |

除了 `throttled` 與 `unavailable` 之外的失敗，重試也無法修正，因此伺服器會記錄後停止，由隔天的排程匯入再試一次。

<!-- source: google-cloud-billing_sources--gcp c74934cb4d9a -->
## Google Cloud（`billing_sources = gcp`）

Google Cloud 沒有回傳實際費用的 API，因此伺服器每天一次以 BigQuery 的 `jobs.query` 彙總匯出到 BigQuery 的 Cloud Billing 資料（標準使用費用）（`billing_gcp_method = bigquery`，預設）。無法設定匯出時，也有一種簡易方法，只從預算的 Pub/Sub 通知匯入大略的總額（請參閱[從預算通知匯入](#importing-from-budget-notifications-billing_gcp_method--budget)）。帳單月份是匯出資料的 `invoice.month`，也就是以美國太平洋時間（含夏令時間）計算的日曆月。日期依太平洋時間原樣記錄。金額以帳單帳戶的幣別計，依帳單月份、服務（`service.description`）與費用類型（`cost_type`）以 `NUMERIC` 加總，抵用額（`credits[]`）則成為每個服務一列的抵用額。匯出資料中用來抵銷先前列的更正列也會納入加總。匯出資料沒有表示確定的標記，因此從下個月 5 日（太平洋時間）起執行的匯入會把該月份視為確定，在那之前則記錄為從該月第一天到最後使用日的估計值。確定之後，匯入仍會在 15 日之前重新讀取該月份，並把任何差異記錄為更正。以建置標籤 `no_billing_gcp` 建置的執行檔會拒絕 `gcp`（請參閱 [build-tags.md](../reference/build-tags.md)）。

<!-- source: cost-and-limits-1 ca56b5d7b34d -->
### 費用與上限

一次匯入就是一個查詢，BigQuery 依每個查詢掃描的位元組數計費（最少 10 MB）。小型帳戶每月掃描約 0.3 GB，在免費方案每月最初的 1 TiB 之內（與帳單帳戶的其他用途共用），因此費用為 0 美元。為了避免意外的掃描，每個查詢都帶有 `maximumBytesBilled`（`billing_gcp_max_bytes_billed`，預設 1 GiB）。會超過此值的查詢不會計費並會失敗，並記錄為類型 `bytes_cap` 的失敗。管理介面會顯示計費的位元組數。

<!-- source: enable-the-export-to-bigquery 40c7a176b9f1 -->
### 啟用匯出到 BigQuery

營運者要在主控台中設定一次（目前沒有找到透過 API 設定的方法）。

1. 在連結到帳單帳戶的專案中，建立 BigQuery 資料集（例如 `billing_export`）。請選擇多區域位置（`US` 或 `EU`）。當月與上個月的資料會被回填（若選擇單一區域，資料會從你啟用的那天開始）。位置之後無法變更。不要為資料表設定到期時間。到期的列不會再回來。
2. 在「Billing > Billing export > BigQuery export」中啟用「Standard usage cost」，並選擇步驟 1 的資料集。進行這項設定需要帳單帳戶上的「Billing Account Costs Manager」或「Billing Account Administrator」，以及專案上的 BigQuery User 角色。
3. 經過數小時到一天後，會建立資料表 `gcp_billing_export_v1_<BILLING_ACCOUNT_ID>`（ID 中的連字號會變成 `_`）。
4. 設定 `MY_BILLING_GCP_TABLE=<project>.billing_export.gcp_billing_export_v1_<BILLING_ACCOUNT_ID>`。

找不到資料集或資料表時，匯入會以 `not_found` 失敗。

<!-- source: iam-roles f5628acd0d94 -->
### IAM 角色

為執行匯入的伺服器的服務帳戶授予這兩個角色。

| 角色 | 授予的位置 | 用途 |
| --- | --- | --- |
| `roles/bigquery.jobUser`（BigQuery Job User） | 執行查詢的專案（`billing_gcp_query_project`，為空時則為資料表所屬的專案） | `bigquery.jobs.create`。掃描的位元組數會計費到這個專案 |
| `roles/bigquery.dataViewer`（BigQuery Data Viewer） | 匯出的資料集 | `bigquery.tables.getData` |

```sh
gcloud projects add-iam-policy-binding <query project> \
  --member=serviceAccount:<service account> --role=roles/bigquery.jobUser
bq add-iam-policy-binding --member=serviceAccount:<service account> \
  --role=roles/bigquery.dataViewer <project>:billing_export
```

查詢專案中沒有啟用 BigQuery API 時，匯入會以 `not_enabled` 失敗；缺少角色時則以 `permission` 失敗。

驗證方式為應用程式預設憑證（ADC）。

| 執行環境 | 憑證 |
| --- | --- |
| Google Cloud（Cloud Run） | 以服務的服務帳戶使用 ADC。不需要金鑰，也不需要任何設定 |
| AWS ECS | 放置 Workload Identity Federation（AWS 類型）的憑證設定檔，並以 `GOOGLE_APPLICATION_CREDENTIALS` 傳遞其路徑（尚未在真實帳戶上驗證） |
| 你自己的伺服器 | Workload Identity Federation，或以 `GOOGLE_APPLICATION_CREDENTIALS` 傳遞服務帳戶的 JSON 金鑰。請從機密資訊儲存區傳遞金鑰 |

<!-- source: choosing-which-costs-to-import-filtering-1 7067e430a4c7 -->
### 選擇要匯入哪些費用（篩選）

預設情況下，伺服器會匯入帳單帳戶的所有費用。最準確的做法是為這個伺服器準備專用的帳單帳戶，不加篩選地匯入。

| 設定 | 匯入的費用 |
| --- | --- |
| `billing_gcp_projects`（專案 ID，以逗號分隔） | 你指定的專案的費用（`project.id`） |
| `billing_gcp_label`（`key=value`，例如 `app=my`） | 帶有你指定標籤的資源的費用（`labels`） |

兩者都設定時，伺服器只會匯入同時符合兩者的費用。請款單層級的費用（例如稅金）可能既沒有專案也沒有標籤，因此會被篩選排除（尚未在真實帳戶上驗證）。

<!-- source: kinds-of-failure-1 612d14e2ba5c -->
### 失敗類型

| 類型 | BigQuery 回應 | 修正方式 |
| --- | --- | --- |
| `permission` | 401、403（`accessDenied`） | 檢查服務帳戶的角色與憑證的傳遞方式 |
| `not_enabled` | 403（`accessNotConfigured` 等） | 在查詢專案中啟用 BigQuery API |
| `not_found` | 404 | 檢查 `billing_gcp_table` 與匯出的設定 |
| `bytes_cap` | 400（`bytesBilledLimitExceeded`） | 掃描量超過了 `billing_gcp_max_bytes_billed`。若資料表很大，請提高上限 |
| `throttled` | 429、403（`rateLimitExceeded`、`quotaExceeded`） | 任務會在暫停後重試 |
| `unavailable` | 5xx、網路故障、未完成的查詢 | 任務會在暫停後重試 |
| `invalid_response` | 無法讀取的回應、同一個月份內混有多種幣別的列、其他任何 400 | 檢查匯出資料表（是否為標準使用費用？） |

除了 `throttled` 與 `unavailable` 之外的失敗，重試也無法修正，因此伺服器會記錄後停止，由隔天的排程匯入再試一次。

<!-- source: importing-from-budget-notifications-billing_gcp_method--budget b90e6393e3d3 -->
### 從預算通知匯入（`billing_gcp_method = budget`）

這是無法設定 BigQuery 匯出時使用的簡易方法。伺服器每天一次從 pull 訂閱讀取 Cloud Billing 預算傳送到 Pub/Sub 主題的通知。它使用 pull 而不是 push，因此通知絕不會喚醒伺服器（例如 Cloud Run）。

這個方法**只會匯入根據估計值得出的大略總額**。

- 通知的 `costAmount` 是預算期間的估計累計費用，沒有依服務或費用類型細分。伺服器每個月份與預算只會匯入一列（服務名稱為預算的顯示名稱，費用類型為使用量，類別為其他）。抵用額與稅金是否包含在總額中，取決於預算的設定。
- 與 BigQuery 相同，月份會在下個月 5 日（太平洋時間）起執行的匯入中確定。預算通知不知道請款單何時結算，因此即使是視為確定的數值，也可能與請款金額不同。
- 估計中的月份會記錄為從該月第一天到最後一則通知發布當天（太平洋時間）的費用。
- 篩選由預算的範圍（專案、服務、一組標籤鍵與值）決定，不會使用 `billing_gcp_projects` 與 `billing_gcp_label`。

<!-- source: prepare-the-budget-the-topic-and-the-subscription ee38acf8cb72 -->
#### 準備預算、主題與訂閱

1. 建立只有這個伺服器會讀取的主題與 pull 訂閱。關於保留期間與到期時間的理由，請參閱下方的「如何讀取通知」。

   ```sh
   gcloud pubsub topics create billing-budget --project=<project>
   gcloud pubsub subscriptions create my-billing-budget --project=<project> \
     --topic=billing-budget --ack-deadline=60 \
     --message-retention-duration=31d --expiration-period=never
   ```

2. 在主控台的「Billing > Budgets & alerts」中建立預算。把它的期間設為**每月**（非每月預算的通知會以 `invalid_response` 失敗）。範圍選擇這個伺服器的專案，並在「Manage notifications > Connect a Pub/Sub topic to this budget」中選擇步驟 1 的主題。建立預算需要帳單帳戶上的「Billing Account Costs Manager」或「Billing Account Administrator」，以及主題所屬專案上的 `pubsub.topics.setIamPolicy` 與 `pubsub.topics.list`。第一則通知需要數小時才會送達，之後每天會送達數則。
3. 在訂閱上授予執行匯入的伺服器的服務帳戶 `roles/pubsub.subscriber`（Pub/Sub Subscriber）。它用來提取訊息並確認收到（`pubsub.subscriptions.consume`）。

   ```sh
   gcloud pubsub subscriptions add-iam-policy-binding my-billing-budget --project=<project> \
     --member=serviceAccount:<service account> --role=roles/pubsub.subscriber
   ```

4. 設定 `MY_BILLING_GCP_METHOD=budget` 與 `MY_BILLING_GCP_BUDGET_SUBSCRIPTION=projects/<project>/subscriptions/my-billing-budget`。

驗證方式與 BigQuery 相同，為應用程式預設憑證（請參閱上方 [IAM 角色](#iam-roles)中的表格）。建置標籤 `no_billing_gcp` 也會移除這個方法。Pub/Sub 用戶端函式庫與 `stream = pubsub` 共用，因此只有同時設定 `no_pubsub` 時，它才會從執行檔中移除（請參閱 [build-tags.md](../reference/build-tags.md)）。

<!-- source: how-notifications-are-read 646a6dd9b2b6 -->
#### 如何讀取通知

- 一次匯入會重複 Pull 直到沒有新訊息為止，並針對每個預算與月份採用發布時間最新的通知。這樣可以吸收 Pub/Sub 的重複傳遞、數值相同的重複通知以及順序錯亂，因此即使多次收到同一則通知，在總額中也只算一次。一次匯入計入的請求次數是 Pull 的次數，通常為兩次：一次回傳訊息，一次回傳空結果。最後進行的確認（`Acknowledge`、`ModifyAckDeadline`）不計入。
- 伺服器只會確認（ack）已被同一預算與月份的較新通知取代的通知，以及不再匯入的月份的通知（上一個月份在被記錄為確定之後）。已匯入月份的最新通知不會被 ack，而是退回訂閱，因此下一次匯入會再次讀取同一則通知，在沒有新通知的日子數值也不會改變。
- 因此，請把訂閱的訊息保留期間設為**至少 14 天（建議 31 天）**。某個月份預算的最後一則通知大約在下個月 2 日送達，而上一個月份會被重新讀取到 15 日為止。通知已過期的月份會被記錄為費用 0。
- 預設情況下，訂閱在 31 天未使用後就會到期。請把到期時間設為「never」，讓它在匯入停止的期間也能保留下來。
- 不要把其他訊息傳送到這個主題。無法讀取為預算通知的訊息會讓那次匯入以 `invalid_response` 失敗，並會被 ack，以免下一次匯入卡在它上面。
- 費用：每天幾十則訊息在 Pub/Sub 的免費方案（每月 10 GiB）之內，因此費用為 0 美元。伺服器不設上限。

<!-- source: kinds-of-failure-budget-notifications c96440fefa1a -->
#### 失敗類型（預算通知）

| 類型 | Pub/Sub 回應 | 修正方式 |
| --- | --- | --- |
| `permission` | `PERMISSION_DENIED`、`UNAUTHENTICATED` | 檢查訂閱上的 `roles/pubsub.subscriber` 與憑證的傳遞方式 |
| `not_enabled` | `PERMISSION_DENIED`（`SERVICE_DISABLED`） | 在訂閱所屬專案中啟用 Pub/Sub API |
| `not_found` | `NOT_FOUND` | 檢查 `billing_gcp_budget_subscription`，以及訂閱是否已到期 |
| `throttled` | `RESOURCE_EXHAUSTED` | 任務會在暫停後重試 |
| `unavailable` | `UNAVAILABLE`、`INTERNAL` 等，網路故障 | 任務會在暫停後重試 |
| `invalid_response` | 無法讀取為預算通知的訊息、非每月的預算、兩個顯示名稱相同的預算 | 把預算設為每月，為各預算取不同的顯示名稱，並且不要把其他訊息傳送到主題 |

<!-- source: a-plugin-billing_sources--plugin 7589c3333153 -->
## 外掛（`billing_sources = plugin`）

對於伺服器沒有轉接器的供應商，例如 Azure、Hetzner 或 Sakura，請執行類型為 `billing` 的[外掛](plugins.md)，讓它讀取供應商的資料。伺服器只有一個 `plugin` 來源。涵蓋多個供應商的外掛要自行彙總，並以其服務的名稱加以區分。

```toml
billing_sources = "plugin"
plugin_billing_address = "unix:///run/my/billing.sock"
plugin_billing_location = "Europe/Berlin"
plugin_billing_provider = "Hetzner"
```

使用這個來源時，這三個設定都是必要的；伺服器啟動時不會呼叫外掛，即使外掛停止運作也能啟動。

- `plugin_billing_location` 是劃分外掛帳單月份的時區。
- `plugin_billing_provider` 是供應商在總計、支出頁面與稽核記錄中顯示的名稱。

伺服器會向外掛要求當前月份，並與其他來源一樣，要求上一個月份直到 15 日或確定為止。其他來源的上限不適用：`max_requests` 為 `2147483647`，代表沒有上限，由外掛決定可以花費多少。外掛會回報它被計費的請求次數與位元組數，伺服器會記錄並顯示這些數值。

由於這個類型只傳遞費用，因此允許使用一般的 TCP，啟動時會發出警告。持有者權杖則需要 `https://` 或 Unix socket：請參閱[外掛的連線目的地](plugins.md#the-destination)。

<!-- source: the-call 6a2bcce82ab2 -->
### 呼叫

使用預設的 `plugin_billing_protocol = json` 時，外掛只需要 HTTP 伺服器與 JSON 函式庫（[不使用函式庫的外掛](plugins.md#a-plugin-with-no-library)）。`Fetch` 最長持續 2 分鐘，或 `plugin_billing_timeout` 所設的時間（低於 5 分鐘）。

```text
POST /my.plugin.billing.v1.BillingService/Fetch
Content-Type: application/json

{"months":["2026-09","2026-10"],"maxRequests":2147483647}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{
  "outcome": "OUTCOME_OK",
  "requests": 1,
  "lines": [
    {
      "month": "2026-09",
      "coveredStart": "2026-09-01",
      "coveredEnd": "2026-09-30",
      "service": "Hetzner Cloud",
      "chargeType": "CHARGE_TYPE_USAGE",
      "category": "CATEGORY_COMPUTE",
      "amount": "12.3456",
      "currency": "EUR",
      "final": true
    }
  ]
}
```

- 一列代表一個月份、服務與費用類型，其抵銷列、抵用額與退款都已加總。`service` 最多 200 個字元，且不含控制字元。
- `coveredStart` 與 `coveredEnd` 位於該月份之內：`final` 為 true 時為整個月份，在那之前則到有資料的最後一天為止。
- `amount` 是以主要貨幣單位表示、未經四捨五入的十進位數，可以是負數。`currency` 是 ISO 4217 代碼。
- `chargeType` 是 `CHARGE_TYPE_` 加上 `USAGE`、`CREDIT`、`REFUND`、`TAX`、`SUPPORT`、`FEE`、`ADJUSTMENT`、`OTHER` 其中之一。
- `category` 是 `CATEGORY_` 加上 `COMPUTE`、`DATABASE`、`STORAGE`、`NETWORK`、`DOMAIN`、`EMAIL`、`SOFTWARE`、`OTHER` 其中之一。
- `requests` 最多為 `maxRequests`。`bytesBilled` 是十進位字串，用於依位元組計費的供應商。
- `OUTCOME_OK` 沒有回傳的鍵在其月份中沒有費用，因此伺服器會為它記錄 0 的修訂。無法讀取某個供應商的外掛必須回報失敗，而不是省略它的列。
- 呼叫可能重複：失敗匯入的重試就是再次進行相同的呼叫。

`maxRequests` 為 0 時禁止任何計費請求。伺服器在會送出 0 時不會進行呼叫，但 `my plugin test --kind billing` 會呼叫，並依 proto3 對應規則的允許省略該欄位，預期得到沒有任何列與請求的 `OUTCOME_REQUEST_CAP`。

<!-- source: kinds-of-failure-2 205a6c0ecdd9 -->
### 失敗類型

外掛在 `outcome` 中回報失敗。與其他來源相同，伺服器會保留失敗的 `requests`，忽略其列，並讓先前的匯入維持原狀。

| Outcome | 類型 | 是否重試 |
| --- | --- | --- |
| `OUTCOME_PERMISSION` | `permission` | 否 |
| `OUTCOME_NOT_ENABLED` | `not_enabled` | 否 |
| `OUTCOME_NOT_FOUND` | `not_found` | 否 |
| `OUTCOME_THROTTLED` | `throttled` | 是 |
| `OUTCOME_REQUEST_CAP` | `request_cap` | 否 |
| `OUTCOME_BYTES_CAP` | `bytes_cap` | 否 |
| `OUTCOME_INVALID_RESPONSE` | `invalid_response` | 否 |
| `OUTCOME_UNAVAILABLE` | `unavailable` | 是 |
| 缺少或未知的 outcome，或違反上述規則的列或計數 | `invalid_response` | 否 |
| HTTP 或 Connect 錯誤、逾時、超過 4 MiB 的回應、沒有回應 | `unavailable` | 是 |

不重試的失敗會等待下一次每日匯入。由於伺服器會捨棄錯誤的訊息，失敗的原因請查看外掛的日誌；失敗也會計入 [`my.plugin.failures`](running.md)。

<!-- source: test-it 10a2c62a53d8 -->
### 測試

```sh
my plugin test --kind billing unix:///run/my/billing.sock
my plugin test --kind billing --billed unix:///run/my/billing.sock
```

如果外掛遵守契約，第一個指令不會發出任何計費請求。第二個指令還會實際取得資料，可能會在外掛的供應商產生費用。請參閱[測試外掛](plugins.md#test-a-plugin)。
