<!-- source: importing-cloud-usage-costs db52fcd17ff9 -->
# 导入云服务使用费用

服务器每 24 小时导入一次设置 [`billing_sources`](../reference/configuration.md#billing_sources) 中所列服务商的使用费用（对于服务器没有适配器的服务商，则通过[插件](#a-plugin-billing_sources--plugin)导入），并把它们加入实例的费用。默认值为空，即不导入任何内容。
它导入当前账单月和上一个账单月的费用（上一个月导入到当月 15 日为止，或直到上一个月的费用确定为止），每项服务和每种费用类型（使用费、抵扣额、退款、税费、支持费、手续费、其他）各占一行。
在服务商结算某个月之前，数值记录为估算值；结算之后记录为确定值；确定之后数值如果发生变化，则记录为更正。过去的记录从不被改写。
导入的状态（最近一次成功、最近一次失败的类型以及本月的请求数）显示在管理界面的费用页面上，也可以通过 `GET /api/v1/admin/billing_imports` 获取。
指标按来源、结果和失败类型统计每次尝试（`my.billing.imports`），并报告最近一次尝试失败的来源（`my.billing.import.failing`）；
[告警与处置手册](alerts.md#mybillingimportfailing)中有针对它们的告警。

本服务器的配置中不保存任何凭据。请为运行环境提供各服务商 SDK 的默认凭据链所读取的内容（AWS 为执行角色、环境变量或共享凭据文件；Google Cloud 为应用默认凭据）。

<!-- source: with-the-opentofu-modules 51ec4f78f058 -->
## 使用 OpenTofu 模块

模块 [`billing-aws`](../../deploy/opentofu/modules/billing-aws/README.md) 和 [`billing-gcp`](../../deploy/opentofu/modules/billing-gcp/README.md) 会设置 `billing_sources`，并完成本指南中服务商 API 允许自动完成的部分。`billing-gcp` 只能与 Google Cloud 的应用模块一起使用，因此在 AWS 上，这些模块只导入 AWS 的费用。`billing-aws` 可以与 AWS 的应用模块以及 Cloud Run 的应用模块一起使用，在后者中两个模块可以同时放在 `backends` 里。两个模块都尚未在真实账号上应用过。

| 本指南中的步骤 | 使用模块时 |
| --- | --- |
| IAM 策略，以及“通过 Web 身份从 Cloud Run 调用”中的角色 | `billing-aws` |
| 启用 Cost Explorer、Linked Account Access、成本分配标签 | 在 AWS 控制台中手动完成 |
| 导出所用的数据集，以及“IAM 角色”中的角色 | `billing-gcp` |
| 启用导出（“Standard usage cost”） | 在 Google Cloud 控制台中手动完成，需要结算账号上的角色 |
| 预算的主题和拉取订阅，以及订阅者角色 | `billing-gcp`，设置 `method = "budget"` |
| 创建预算并将其连接到主题 | 在 Google Cloud 控制台中手动完成，需要结算账号上的角色 |
| 启用 BigQuery 或 Pub/Sub API | 手动完成，方法同[在 Cloud Run 上部署](deploy-cloud-run.md#1-enable-the-apis) |
| 插件 | 这些模块不负责。`backends` 中没有账单模块时，在应用模块的 `env` 中设置 `MY_BILLING_SOURCES = plugin` 以及[插件](#a-plugin-billing_sources--plugin)的各项设置；与账单模块同时使用时，应用模块会拒绝 `env` 中的 `MY_BILLING_SOURCES`，因此请自行添加一个返回 `billing_sources = ["plugin"]` 的后端（[后端的字段](../../deploy/opentofu/README.md)） |

<!-- source: aws-billing_sources--aws 9e5fa0132ca3 -->
## AWS（`billing_sources = aws`）

服务器每个账单月（UTC 的自然月）调用一次 AWS Cost Explorer 的 `GetCostAndUsage`，按服务（`SERVICE`）和记录类型（`RECORD_TYPE`）分组。端点始终是 `us-east-1`。
金额以美元计。在 Cost Explorer 把某个月标记为估算（`Estimated`）期间，服务器把截至前一天的金额记录为估算值；不再标记之后，记录为确定值。
使用构建标签 `no_billing_aws` 构建的二进制文件会拒绝 `aws`（见 [build-tags.md](../reference/build-tags.md)）。

<!-- source: cost-and-limits a555f91984c3 -->
### 费用与限额

Cost Explorer API 对每个分页请求收取 0.01 美元。小型账号每天只需一个单页请求，每月约 0.31 美元。
为了防止意外费用，服务器按 UTC 日（`billing_aws_max_requests_per_day`，默认 4）和 UTC 自然月（`billing_aws_max_requests_per_month`，默认 120，即 1.20 美元）统计请求数，达到限额后不再调用并停止。
定时导入、任务重试以及管理界面中的“立即导入”都会计入。达到限额的导入记录为 `request_cap` 类型的失败，并在第二天（每日限额）或下个月的第一天（每月限额）的导入中重试。
导入在调用之前会预留限额剩余的请求数，因此同时运行的多个导入加起来也不会超过限额。
同一服务商的导入一次只运行一个：在另一个导入尚未结束时开始的导入会在一分钟后重试，已经成功导入的任务再次运行时不会进行任何调用。
在完成之前就停止了的导入（进程退出）会计入它预留的请求数，下一次导入最多会等待它 15 分钟。

<!-- source: enable-cost-explorer 2f6cd1add48d -->
### 启用 Cost Explorer

1. 在 Billing and Cost Management 控制台中打开 Cost Explorer 并启用它。一旦启用就无法停用。数据准备好最多需要 24 小时。
2. 未启用 Cost Explorer 的账号会返回 `DataUnavailableException`，导入会以 `not_enabled` 失败。

<!-- source: iam-permissions 9f1c23f3cb5c -->
### IAM 权限

唯一需要的权限是 `ce:GetCostAndUsage`（资源为 `*`）。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": "ce:GetCostAndUsage", "Resource": "*" }
  ]
}
```

在各运行环境中传递凭据的方法：

| 运行环境 | 凭据 |
| --- | --- |
| AWS ECS | 将此策略附加到任务角色。无需任何配置 |
| Google Cloud（Cloud Run） | 创建下文“通过 Web 身份从 Cloud Run 调用”中的 IAM 角色，并将其 ARN 设置到 `billing_aws_role_arn`。无需 AWS 密钥 |
| 你自己的服务器 | 通过 `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`（或共享凭据文件和 `AWS_PROFILE`）传入拥有此策略的 IAM 用户的访问密钥。请从密钥存储中传入 |

权限不足或凭据无效时，会以 `permission` 失败。

<!-- source: calling-from-cloud-run-with-web-identity 7d109914680a -->
### 通过 Web 身份从 Cloud Run 调用

设置了 `billing_aws_role_arn` 时，服务器会从元数据服务器获取 Cloud Run 服务账号的 Google ID 令牌（受众为该角色的 ARN），并用 `AssumeRoleWithWebIdentity` 将其换成该角色的临时凭据。

1. 在 Google Cloud 中，记下 Cloud Run 服务的服务账号的唯一 ID（数字形式的 `uniqueId`）：`gcloud iam service-accounts describe <email> --format='value(uniqueId)'`。
2. 在 AWS IAM 中，创建一个带有上述权限策略的角色，并让它的信任策略信任 `accounts.google.com`。

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

   `accounts.google.com:aud` 对应令牌的 `azp`（服务账号的 ID），`accounts.google.com:oaud` 对应令牌的 `aud`（服务器请求的受众，即该角色的 ARN）。
3. 设置 `MY_BILLING_AWS_ROLE_ARN=arn:aws:iam::<account ID>:role/<role name>`。

此配置尚未在真实账号上验证。

<!-- source: calling-from-a-linked-account-of-organizations d12e10b51af3 -->
### 从 Organizations 的关联账号调用

AWS Organizations 的管理账号可以查看组织中每个成员账号的费用。要使用成员（关联）账号的凭据进行调用，
请在管理账号的 Cost Explorer 偏好设置中打开“Linked Account Access”。它关闭时，无论 IAM 权限如何，调用都会被拒绝，导入会以 `not_enabled` 或 `permission` 失败。

<!-- source: choosing-which-costs-to-import-filtering 60cfa862b6a2 -->
### 选择要导入的费用（筛选）

用 `billing_aws_filter` 选择。

| `billing_aws_filter` | 导入的费用 | 其他设置 |
| --- | --- | --- |
| `none`（默认） | 凭据能看到的所有费用 | 无 |
| `linked_account` | 你指定的成员账号的费用（`LINKED_ACCOUNT`） | `billing_aws_linked_accounts`（12 位账号 ID，以逗号分隔） |
| `tag` | 带有你指定的成本分配标签的资源的费用 | `billing_aws_tag`（`key=value`，例如 `app=my`） |

最准确的配置是为本服务器专门准备一个 AWS 账号（可以是 Organizations 的成员账号），并用 `none`（使用该账号的凭据调用）或 `linked_account`（从管理账号调用）导入。
税费、支持费和部分抵扣额无法带有标签，可能会被标签筛选排除（尚未在真实账号上验证）。

<!-- source: activating-a-cost-allocation-tag-billing_aws_filter--tag 8e642123a388 -->
### 激活成本分配标签（`billing_aws_filter = tag`）

资源上的标签只有在激活为成本分配标签之后，才会出现在账单数据中。

1. 为本服务器的资源（ECS 服务、RDS、S3 存储桶、负载均衡器等）加上相同的标签（例如 `app=my`）。
2. 在 Billing and Cost Management 控制台的“Cost allocation tags”中，选择该标签键并激活它。附加标签后，最多需要 24 小时才会出现在这个列表中；激活后，最多还需要 24 小时才会出现在账单数据中。
3. 要反映激活之前那段时间的费用，请使用同一页面上的“Backfill”（API 为 `StartCostAllocationTagBackfill`）。
4. 设置 `MY_BILLING_AWS_FILTER=tag` 和 `MY_BILLING_AWS_TAG=app=my`。

<!-- source: metric 3e034dd351bd -->
### 指标

`billing_aws_metric` 可以是 `NetUnblendedCost`（默认，更接近账单），即扣除折扣（Savings Plans、预留实例、协商价格）之后的费用；也可以是 `UnblendedCost`，即扣除折扣之前的费用。
由于更新时间和舍入的原因，Cost Explorer 的数值可能与账单不同。两者不一致时，以账单为准。

<!-- source: kinds-of-failure 2a6f9042f460 -->
### 失败类型

| 类型 | AWS 响应 | 解决方法 |
| --- | --- | --- |
| `permission` | `AccessDeniedException`、凭据无效 | 检查 IAM 权限和凭据的传递方式 |
| `not_enabled` | `DataUnavailableException` | 启用 Cost Explorer。对于关联账号，启用 Linked Account Access |
| `not_found` | `BillExpirationException` | Cost Explorer 已不再保留要导入的期间的账单数据 |
| `throttled` | `LimitExceededException` | 任务会在暂停后重试（计入请求限额） |
| `unavailable` | 5xx、网络故障 | 任务会在暂停后重试（计入请求限额） |
| `invalid_response` | 无法读取的响应、`ValidationException` 等 | 检查设置（标签、账号 ID） |
| `request_cap` | （不进行调用） | 等待受限的那一天或那个月过去，或提高限额 |

除 `throttled` 和 `unavailable` 之外的失败无法通过重试解决，因此服务器会记录失败并停止，由第二天的定时导入再次尝试。

<!-- source: google-cloud-billing_sources--gcp c74934cb4d9a -->
## Google Cloud（`billing_sources = gcp`）

Google Cloud 没有返回实际费用的 API，因此服务器每天用一次 BigQuery `jobs.query` 汇总 Cloud Billing 导出到 BigQuery 的数据（标准使用费用）（`billing_gcp_method = bigquery`，默认）。
无法设置导出时，还有一种简易方法，只从预算的 Pub/Sub 通知导入一个近似的总额（见[从预算通知导入](#importing-from-budget-notifications-billing_gcp_method--budget)）。
账单月是导出数据中的 `invoice.month`，即美国太平洋时间（含夏令时）的自然月。日期按太平洋时间原样记录。
金额以结算账号的货币计，按账单月、服务（`service.description`）和费用类型（`cost_type`）以 `NUMERIC` 求和；抵扣额（`credits[]`）每项服务合并为一行抵扣。导出中用于抵消先前行的更正行也计入总和。
导出数据没有表示已确定的标记，因此从次月 5 日（太平洋时间）起运行的导入会把该月视为已确定，在此之前则记录为从当月第一天到最后一次使用当天的估算值。确定之后，导入仍会在 15 日之前再次读取该月，并把任何差额记录为更正。
使用构建标签 `no_billing_gcp` 构建的二进制文件会拒绝 `gcp`（见 [build-tags.md](../reference/build-tags.md)）。

<!-- source: cost-and-limits-1 ca56b5d7b34d -->
### 费用与限额

一次导入就是一次查询，BigQuery 按每次查询扫描的字节数计费（至少 10 MB）。小型账号每月扫描约 0.3 GB，在免费额度每月 1 TiB（与结算账号的其他用途共享）之内，因此费用为 0 美元。
为了防止意外的扫描，每次查询都带有 `maximumBytesBilled`（`billing_gcp_max_bytes_billed`，默认 1 GiB）。会超过该值的查询不计费并且失败，记录为 `bytes_cap` 类型的失败。管理界面会显示计费的字节数。

<!-- source: enable-the-export-to-bigquery 40c7a176b9f1 -->
### 启用到 BigQuery 的导出

运营者需要在控制台中设置一次（尚未发现通过 API 设置的方法）。

1. 在关联到结算账号的项目中创建一个 BigQuery 数据集（例如 `billing_export`）。选择多区域位置（`US` 或 `EU`）。当月和上个月的数据会被回填（选择单一区域时，数据从启用当天开始）。位置之后无法更改。
   不要为表设置过期时间。过期的行不会再回来。
2. 在“Billing > Billing export > BigQuery export”中启用“Standard usage cost”，并选择第 1 步中的数据集。设置需要结算账号上的“Billing Account Costs Manager”或“Billing Account Administrator”，以及项目上的 BigQuery User 角色。
3. 几小时到一天之后，会创建表 `gcp_billing_export_v1_<BILLING_ACCOUNT_ID>`（ID 中的连字符会变成 `_`）。
4. 设置 `MY_BILLING_GCP_TABLE=<project>.billing_export.gcp_billing_export_v1_<BILLING_ACCOUNT_ID>`。

找不到数据集或表时，导入会以 `not_found` 失败。

<!-- source: iam-roles f5628acd0d94 -->
### IAM 角色

为执行导入的服务器的服务账号授予以下两个角色。

| 角色 | 授予位置 | 用途 |
| --- | --- | --- |
| `roles/bigquery.jobUser`（BigQuery Job User） | 运行查询的项目（`billing_gcp_query_project`，为空时为表所在的项目） | `bigquery.jobs.create`。扫描的字节数计费到此项目 |
| `roles/bigquery.dataViewer`（BigQuery Data Viewer） | 导出所在的数据集 | `bigquery.tables.getData` |

```sh
gcloud projects add-iam-policy-binding <query project> \
  --member=serviceAccount:<service account> --role=roles/bigquery.jobUser
bq add-iam-policy-binding --member=serviceAccount:<service account> \
  --role=roles/bigquery.dataViewer <project>:billing_export
```

查询项目中未启用 BigQuery API 时，导入会以 `not_enabled` 失败；缺少角色时，以 `permission` 失败。

身份验证使用应用默认凭据（ADC）。

| 运行环境 | 凭据 |
| --- | --- |
| Google Cloud（Cloud Run） | 使用该服务的服务账号的 ADC。无需密钥，也无需配置 |
| AWS ECS | 放置一个 Workload Identity Federation（AWS 类型）的凭据配置文件，并通过 `GOOGLE_APPLICATION_CREDENTIALS` 传入其路径（尚未在真实账号上验证） |
| 你自己的服务器 | 使用 Workload Identity Federation，或通过 `GOOGLE_APPLICATION_CREDENTIALS` 传入服务账号的 JSON 密钥。请从密钥存储中传入密钥 |

<!-- source: choosing-which-costs-to-import-filtering-1 7067e430a4c7 -->
### 选择要导入的费用（筛选）

默认情况下，服务器导入结算账号的所有费用。最准确的配置是为本服务器专门准备一个结算账号，不加筛选地导入。

| 设置 | 导入的费用 |
| --- | --- |
| `billing_gcp_projects`（项目 ID，以逗号分隔） | 你指定的项目的费用（`project.id`） |
| `billing_gcp_label`（`key=value`，例如 `app=my`） | 带有你指定的标签的资源的费用（`labels`） |

两者都设置时，服务器只导入同时符合两者的费用。
账单层级的费用（例如税费）可能既不带项目也不带标签，这时会被筛选排除（尚未在真实账号上验证）。

<!-- source: kinds-of-failure-1 612d14e2ba5c -->
### 失败类型

| 类型 | BigQuery 响应 | 解决方法 |
| --- | --- | --- |
| `permission` | 401、403（`accessDenied`） | 检查服务账号的角色和凭据的传递方式 |
| `not_enabled` | 403（`accessNotConfigured` 等） | 在查询项目中启用 BigQuery API |
| `not_found` | 404 | 检查 `billing_gcp_table` 和导出设置 |
| `bytes_cap` | 400（`bytesBilledLimitExceeded`） | 扫描量超过了 `billing_gcp_max_bytes_billed`。如果表很大，请提高限额 |
| `throttled` | 429、403（`rateLimitExceeded`、`quotaExceeded`） | 任务会在暂停后重试 |
| `unavailable` | 5xx、网络故障、未完成的查询 | 任务会在暂停后重试 |
| `invalid_response` | 无法读取的响应、同一个月内混有多种货币的行、其他任何 400 | 检查导出表（是否为标准使用费用？） |

除 `throttled` 和 `unavailable` 之外的失败无法通过重试解决，因此服务器会记录失败并停止，由第二天的定时导入再次尝试。

<!-- source: importing-from-budget-notifications-billing_gcp_method--budget b90e6393e3d3 -->
### 从预算通知导入（`billing_gcp_method = budget`）

这是在无法设置 BigQuery 导出时使用的简易方法。服务器每天一次从拉取订阅中读取 Cloud Billing 预算发送到 Pub/Sub 主题的通知。
它使用拉取而不是推送，因此通知从不会唤醒服务器（例如 Cloud Run）。

这种方法**只导入基于估算值的近似总额**。

- 通知中的 `costAmount` 是预算期间的估算累计费用，没有按服务或费用类型细分。服务器每月每个预算只导入一行（服务名称为预算的显示名称，费用类型为使用费，类别为其他）。抵扣额和税费是否计入总额，取决于预算的配置。
- 与 BigQuery 一样，某个月在从次月 5 日（太平洋时间）起运行的导入中变为确定。预算通知对账单结算一无所知，因此即使是被视为确定的数值，也可能与账单金额不同。
- 估算中的月份记录为从当月第一天到最后一条通知发布当天（太平洋时间）的费用。
- 筛选由预算的范围（项目、服务、一个标签键和值）决定，不使用 `billing_gcp_projects` 和 `billing_gcp_label`。

<!-- source: prepare-the-budget-the-topic-and-the-subscription ee38acf8cb72 -->
#### 准备预算、主题和订阅

1. 创建一个主题和一个只由本服务器读取的拉取订阅。保留期限和过期设置的原因见下文“如何读取通知”。

   ```sh
   gcloud pubsub topics create billing-budget --project=<project>
   gcloud pubsub subscriptions create my-billing-budget --project=<project> \
     --topic=billing-budget --ack-deadline=60 \
     --message-retention-duration=31d --expiration-period=never
   ```

2. 在控制台的“Billing > Budgets & alerts”中创建一个预算。把它的期间设为**每月**（非每月预算的通知会以 `invalid_response` 失败）。
   选择本服务器的项目作为范围，并在“Manage notifications > Connect a Pub/Sub topic to this budget”中选择第 1 步中的主题。
   创建预算需要结算账号上的“Billing Account Costs Manager”或“Billing Account Administrator”，以及主题所在项目上的 `pubsub.topics.setIamPolicy` 和 `pubsub.topics.list`。
   第一条通知需要几个小时才会到达，之后每天会到达几条。
3. 为执行导入的服务器的服务账号授予该订阅上的 `roles/pubsub.subscriber`（Pub/Sub Subscriber）。它用于拉取消息并确认消息（`pubsub.subscriptions.consume`）。

   ```sh
   gcloud pubsub subscriptions add-iam-policy-binding my-billing-budget --project=<project> \
     --member=serviceAccount:<service account> --role=roles/pubsub.subscriber
   ```

4. 设置 `MY_BILLING_GCP_METHOD=budget` 和 `MY_BILLING_GCP_BUDGET_SUBSCRIPTION=projects/<project>/subscriptions/my-billing-budget`。

身份验证与 BigQuery 相同，使用应用默认凭据（见上文 [IAM 角色](#iam-roles)中的表格）。
构建标签 `no_billing_gcp` 也会移除这种方法。Pub/Sub 客户端库与 `stream = pubsub` 共用，因此只有同时设置 `no_pubsub` 时，它才会从二进制文件中移除（见 [build-tags.md](../reference/build-tags.md)）。

<!-- source: how-notifications-are-read 646a6dd9b2b6 -->
#### 如何读取通知

- 一次导入会反复执行 Pull，直到不再有新消息到达，并为每个预算和月份取发布时间最新的那条通知。这样可以吸收 Pub/Sub 的重复投递、数值相同的重复通知以及顺序错乱，因此同一条通知收到多次，在总额中也只计一次。
  一次导入计入的请求数就是 Pull 的次数，通常为两次：一次返回消息，一次返回空结果。最后进行的确认（`Acknowledge`、`ModifyAckDeadline`）不计入。
- 服务器只确认（ack）已被同一预算和月份的更新通知取代的通知，以及已不再导入的月份的通知（上一个月在被记录为确定之后）。正在导入的月份的最新通知不会被确认，而是退回订阅，因此下一次导入会再次读取同一条通知，在没有新通知的日子里数值也不会改变。
- 因此，请把订阅的消息保留期限设为**至少 14 天（建议 31 天）**。某个月预算的最后一条通知在次月 2 日前后到达，而上一个月会被反复读取到 15 日。通知已过期的月份会被记录为费用 0。
- 默认情况下，订阅在 31 天未使用后过期。请把过期设置设为“never”，使它在导入停止的期间也能保留下来。
- 不要向该主题发送其他消息。无法作为预算通知读取的消息会使那次导入以 `invalid_response` 失败，并会被确认，以免下一次导入卡在它上面。
- 费用：每天几十条消息在 Pub/Sub 的免费额度（每月 10 GiB）之内，因此费用为 0 美元。服务器不设任何限额。

<!-- source: kinds-of-failure-budget-notifications c96440fefa1a -->
#### 失败类型（预算通知）

| 类型 | Pub/Sub 响应 | 解决方法 |
| --- | --- | --- |
| `permission` | `PERMISSION_DENIED`、`UNAUTHENTICATED` | 检查订阅上的 `roles/pubsub.subscriber` 和凭据的传递方式 |
| `not_enabled` | `PERMISSION_DENIED`（`SERVICE_DISABLED`） | 在订阅所在项目中启用 Pub/Sub API |
| `not_found` | `NOT_FOUND` | 检查 `billing_gcp_budget_subscription`，以及订阅是否已过期 |
| `throttled` | `RESOURCE_EXHAUSTED` | 任务会在暂停后重试 |
| `unavailable` | `UNAVAILABLE`、`INTERNAL` 等，网络故障 | 任务会在暂停后重试 |
| `invalid_response` | 无法作为预算通知读取的消息、非每月的预算、显示名称相同的两个预算 | 把预算设为每月，为各预算设置不同的显示名称，并且不向该主题发送其他消息 |

<!-- source: a-plugin-billing_sources--plugin 7589c3333153 -->
## 插件（`billing_sources = plugin`）

对于服务器没有适配器的服务商（例如 Azure、Hetzner 或 Sakura），请运行一个 `billing` 类型的[插件](plugins.md)，由它读取服务商的数据。服务器只有一个 `plugin` 来源。涉及多个服务商的插件需要自行汇总，并通过服务名称加以区分。

```toml
billing_sources = "plugin"
plugin_billing_address = "unix:///run/my/billing.sock"
plugin_billing_location = "Europe/Berlin"
plugin_billing_provider = "Hetzner"
```

使用这个来源时，这三项设置都是必需的；服务器启动时不会调用插件，即使插件没有运行也能启动。

- `plugin_billing_location` 是划分插件账单月的时区。
- `plugin_billing_provider` 是在合计、费用页面和审计日志中显示的服务商名称。

服务器向插件请求当前月份；与其他来源一样，还会请求上一个月，直到 15 日或直到其确定为止。其他来源的限额不适用：`max_requests` 为 `2147483647`，表示没有限额，由插件决定可以花费多少。插件会报告它被计费的请求数和字节数，服务器会记录并显示它们。

这种类型允许使用明文 TCP（启动时会给出警告），因为它只传输费用。使用 bearer 令牌需要 `https://` 或 Unix 套接字：见[插件的目标地址](plugins.md#the-destination)。

<!-- source: the-call 6a2bcce82ab2 -->
### 调用

使用默认的 `plugin_billing_protocol = json` 时，插件只需要一个 HTTP 服务器和一个 JSON 库（[不使用库的插件](plugins.md#a-plugin-with-no-library)）。`Fetch` 最长持续 2 分钟，或者持续 `plugin_billing_timeout`（小于 5 分钟）。

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

- 一行对应一个月份、一项服务和一种费用类型，其中的抵消行、抵扣额和退款已经合计在内。`service` 最多 200 个字符，且不含控制字符。
- `coveredStart` 和 `coveredEnd` 位于该月份之内：`final` 为 true 时为整个月，在此之前为截至有数据的最后一天。
- `amount` 是以主货币单位表示的未经舍入的十进制数，可以为负数。`currency` 是 ISO 4217 代码。
- `chargeType` 是 `CHARGE_TYPE_` 加上 `USAGE`、`CREDIT`、`REFUND`、`TAX`、`SUPPORT`、`FEE`、`ADJUSTMENT`、`OTHER` 之一。
- `category` 是 `CATEGORY_` 加上 `COMPUTE`、`DATABASE`、`STORAGE`、`NETWORK`、`DOMAIN`、`EMAIL`、`SOFTWARE`、`OTHER` 之一。
- `requests` 最多为 `maxRequests`。`bytesBilled` 是十进制字符串，用于按字节计费的服务商。
- `OUTCOME_OK` 没有返回的键在其月份中没有费用，因此服务器会为它记录一条值为 0 的修订。无法读取某个服务商的插件必须报告失败，而不是省略它的行。
- 调用可能会重复：失败导入的重试就是再次进行同一个调用。

`maxRequests` 为 0 表示禁止任何计费请求。服务器在要发送 0 时不会进行调用，但 `my plugin test --kind billing` 会调用，并按照 proto3 映射的允许省略该字段，期望得到不含任何行、请求数为零的 `OUTCOME_REQUEST_CAP`。

<!-- source: kinds-of-failure-2 205a6c0ecdd9 -->
### 失败类型

插件在 `outcome` 中报告失败。与其他来源一样，服务器会保留失败时的 `requests`，忽略其中的行，并保持先前的导入不变。

| 结果 | 类型 | 是否重试 |
| --- | --- | --- |
| `OUTCOME_PERMISSION` | `permission` | 否 |
| `OUTCOME_NOT_ENABLED` | `not_enabled` | 否 |
| `OUTCOME_NOT_FOUND` | `not_found` | 否 |
| `OUTCOME_THROTTLED` | `throttled` | 是 |
| `OUTCOME_REQUEST_CAP` | `request_cap` | 否 |
| `OUTCOME_BYTES_CAP` | `bytes_cap` | 否 |
| `OUTCOME_INVALID_RESPONSE` | `invalid_response` | 否 |
| `OUTCOME_UNAVAILABLE` | `unavailable` | 是 |
| 缺失或未知的结果，或者违反上述规则的行或计数 | `invalid_response` | 否 |
| HTTP 或 Connect 错误、超过时限、超过 4 MiB 的响应、没有响应 | `unavailable` | 是 |

不重试的失败会等待下一次每日导入。失败的原因记录在插件的日志中，因为服务器会丢弃错误的消息；它也会计入 [`my.plugin.failures`](running.md)。

<!-- source: test-it 10a2c62a53d8 -->
### 测试

```sh
my plugin test --kind billing unix:///run/my/billing.sock
my plugin test --kind billing --billed unix:///run/my/billing.sock
```

如果插件遵守约定，第一条命令不会产生任何计费请求。第二条命令还会进行一次真实的获取，这可能会在插件的服务商处产生费用。见[测试插件](plugins.md#test-a-plugin)。
