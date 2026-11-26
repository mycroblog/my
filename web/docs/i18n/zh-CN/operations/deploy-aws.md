<!-- source: deploying-on-aws d7e8643cd93d -->
# 在 AWS 上部署

本文介绍如何用 OpenTofu 模块 [`mycroblog-aws`](../../deploy/opentofu/README.md#aws) 在 AWS 上运行 `my`：Fargate 上的 ECS，前面是应用负载均衡器（Application Load Balancer），后台任务放在数据库中，另外还有 S3、SNS，以及通过 Amazon SES 发送的电子邮件。没有任何东西会缩容到零：在数据库之外，空闲时预计每月约 42 美元，请阅读[费用](#costs)。

**它从未在 AWS 上应用过。** 检查是在模拟的 provider 上、不使用任何凭据来做计划的，这只能说明配置前后一致，并不能说明 AWS 会接受它，也不能说明服务器能启动。请把第一次部署当作一次测试。

每个 OpenTofu 部署共有的内容，包括模块的输入、它的机密、控制台以及升级，都在[使用 OpenTofu 部署](deploy-opentofu.md)中。本文补充 AWS 所需要的内容。

<!-- source: what-it-creates 1ca642b05ab0 -->
## 会创建什么

`mycroblog-aws` 的一个块会创建下面所有内容。每一行是模块的一个部分（section），其默认值就是推荐的配置；[模块的参考](../../deploy/opentofu/README.md#sections-of-mycroblog-aws)列出了每个字段。

| 部分 | 区段 | 默认值 | 其他种类 |
| --- | --- | --- | --- |
| 服务器 | `service`、`jobs` | Fargate 上的一个 ECS 集群和服务，`role = all`；迁移所用的 task 定义，需要时还有控制台的 task 定义；以及日志组 `/ecs/<name>` | |
| 网络 | `network` | 跨两个或更多可用区的 VPC，每个可用区中有一个公有子网和一个私有子网 | 你自己的 VPC：[网络](#networks) |
| 流量 | `ingress` | 一个始终存在的应用负载均衡器，使用你的证书 | 在它前面放置 CloudFront 或 Cloudflare：[流量如何到达服务](#how-traffic-reaches-the-service) |
| 后台任务 | | 无：task 一直运行，从数据库获取后台任务 | |
| 数据库 | `database` | 一个存放你自己运行的 PostgreSQL（例如 Neon）的 URL 的机密 | Amazon RDS：[选择数据库](#2-choose-the-database) |
| 媒体 | `media` | 一个私有 S3 存储桶 | Cloudflare R2，或任何兼容 S3 的存储 |
| 缓存和速率限制 | `kv` | 无：由数据库保存 | ElastiCache Serverless for Valkey，或你自己运行的 Valkey |
| 实时更新 | `stream` | 一个 SNS 主题。服务器自行创建和删除自己的 SQS 队列 | 数据库，或 `kv` 的 Valkey |
| 邮件 | `mail` | `mail.from` 所在域名的 SES 身份：[通过 Amazon SES 发送邮件](#email-through-amazon-ses) | 任何 SMTP 服务 |
| 机密 | | Secrets Manager：一个自动生成的 VAPID 密钥，以及只有你能填写的、内容为空的机密 | |
| KEK | `kek` | Secrets Manager 中一个自动生成的机密 | AWS KMS 中的密钥，它永远不会离开 KMS |
| 指标、追踪、错误 | `observability` | CloudWatch 中的指标和错误报告，追踪通过每个 task 中的 collector 发送到 X-Ray：[追踪到 X-Ray](#traces-to-x-ray) | 你自己的导出器，通过 `settings` 配置 |
| 告警 | `monitoring` | 一个 CloudWatch Synthetics canary 和多个告警：[监控和告警](#monitoring-and-alerts) | 设为 `null` 则没有 |

`translation = {}` 会添加 Amazon Translate，`search = { domain_name = "..." }` 会添加你创建的 Amazon OpenSearch Service 域，`billing = {}` 会添加从 Cost Explorer 导入使用费用的功能：参见[翻译帖子](translation.md#amazon-translate)、[搜索引擎](search-engines.md)和[导入云使用费用](billing-import.md)。

<!-- source: before-you-start 9d5614a650e1 -->
## 开始之前

- [首次部署](deploy-opentofu.md#first-deployment)所需要的一切，其中平台的 CLI 是 `aws`。
- 一个专供本实例使用的 AWS 账号，以及可以创建上述所有内容的凭据，例如 `AdministratorAccess`。
- 一个区域。SES 从该区域发送邮件，模块从 `aws` provider 获取区域。
- 建议：在 Route 53 中为 `host` 准备一个公有托管区，这样模块就会在其中创建 `host` 的记录，在区域顶点（zone apex）也一样。

<!-- source: first-deployment 26b562a3db03 -->
## 首次部署

请在这里完成前三个步骤，然后按照[首次部署](deploy-opentofu.md#first-deployment)操作，并参考第四步的说明。

<!-- source: 1-request-the-certificates 3b26d03ba06c -->
### 1. 申请证书

负载均衡器需要 AWS Certificate Manager 中该区域内为 `host` 签发且已验证的证书。模块以 ARN 的形式接收它，因为在 apply 中验证证书，会让 apply 一直等到你添加只有 apply 才会输出的记录。

```sh
aws acm request-certificate --region ap-northeast-1 --domain-name social.example.org --validation-method DNS
aws acm describe-certificate --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN" \
  --query 'Certificate.DomainValidationOptions[].ResourceRecord'
# Add that CNAME record at your DNS provider, then:
aws acm wait certificate-validated --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN"
```

使用 `ingress.kind = cloudfront` 时，请在 `us-east-1` 再申请一个，CloudFront 只接受该区域的证书；在 `us-east-1` 中，一个证书就能同时用于两者。使用 `ingress.media_host` 时，证书也必须覆盖该主机。

<!-- source: 2-choose-the-database d9cc7b8b2210 -->
### 2. 选择数据库

由 `database.kind` 决定。无论哪种，task 都会大约每秒向数据库轮询一次后台任务，因此数据库永远不会休息。

**`external`，默认也是推荐的选择**，从你填写的机密中读取你自己运行的 PostgreSQL（例如 [Neon](https://neon.com)）的 URL。模块从不创建该数据库，因此它的密码不会进入 OpenTofu 状态。在 Neon 上，请在靠近你所在区域的地区创建项目，并复制它的连接字符串：

- **直连**字符串（主机名不带 `-pooler`）是必需的，放在机密 `<name>-database-url` 中。迁移和控制台始终使用它：`my schema apply` 会持有会话级锁，而事务模式的连接池无法保持这种锁。
- **池化**字符串（主机名带 `-pooler`）是可选的，供服务使用：设置 `database = { kind = "external", pooled = true }`，模块就会创建机密 `<name>-database-url-pooled`。参见[通过连接池](database.md#through-a-connection-pooler)。

请按 Neon 的 Launch 套餐来预期：由于计算资源永远不会停止，Free 套餐的计算时长会在当月用完。

**`rds`** 会在私有子网中创建 Amazon RDS for PostgreSQL，密码由模块生成并自行写入 `<name>-database-url`，因此没有什么需要填写的：

```hcl
  database = { kind = "rds" }
```

默认是 `db.t4g.micro` 和 20 GB（`instance_class`、`allocated_storage_gb`），保留 7 天的自动备份，并启用删除保护。它没有公开地址，只有 task 能访问它：`pg_dump` 必须从 VPC 内部运行，升级之前则用快照代替，`aws rds create-db-snapshot --db-instance-identifier my --db-snapshot-identifier my-before-upgrade`。它没有池化 URL。

<!-- source: 3-write-the-block db167b7d9566 -->
### 3. 编写块

`examples/aws-ecs-neon` 是一种起步方式：`aws` provider 加上一个使用默认值的块，其变量列在 `terraform.tfvars.example` 中。无论从什么开始，块都需要这些：

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

- `network.egress` 没有默认值，因为两种出网方式都按小时计费：请阅读[网络](#networks)。
- 如果你有 `host` 所在的托管区，`ingress.route53_zone_id` 指明它。
- `media.bucket_name` 必须在整个 AWS 中唯一。
- `monitoring` 指明告警通知谁，或者设为 `null`。

请加上你所选数据库的部分，以及你更改的其他部分。

<!-- source: 4-deploy 891c5c7cef83 -->
### 4. 部署

请按照[首次部署](deploy-opentofu.md#first-deployment)操作。在 AWS 上，本文的命令使用默认的 `name`，即 `my`：

- **机密**：它们在 Secrets Manager 中，名称为 `<name>-...`。`tofu output kek` 提供复制 KEK 的命令，`aws secretsmanager get-secret-value --secret-id my-vapid-key --query SecretString --output text` 复制 VAPID 密钥。KEK 以 `MY_KEK` 的形式交给服务器，因为 Fargate 不挂载文件。task 在启动时读取它的机密：更改某个机密之后，请运行 `aws ecs update-service --cluster my --service my --force-new-deployment`，每个 worker 也一样，即 `my-<key>`。
- **Apply**：迁移的 task 定义最先创建，连同它所依赖的内容，只有迁移成功之后才会创建服务。负载均衡器的检查请求 `/readyz`，在 schema 存在之前它会返回 `503`。
- **DNS**：`tofu output dns_records` 会列出 `host` 的记录（除非由 Route 53 或 Cloudflare 创建），以及 SES 的三条 DKIM 记录。在没有 Route 53 的区域顶点，`host` 的记录需要你的 DNS 服务商所说的 `ALIAS` 或 `ANAME`，如 `tofu output dns_note` 所述。
- **邮件**：在 AWS 授予生产访问权限之前，SES 只会向你验证过的地址发送邮件，因此其他人暂时还无法注册：请阅读[通过 Amazon SES 发送邮件](#email-through-amazon-ses)。
- **告警**：`monitoring.notification_emails` 中的每个地址都会收到来自 SNS 的订阅邮件，必须确认。
- **第一个管理员**：链接保留在 CloudWatch Logs 的 `/ecs/<name>` 中，任何能读取该账号日志的人都可以使用它。

<!-- source: the-console-and-upgrades-on-aws 0e4bc0bdeff2 -->
## AWS 上的控制台和升级

[控制台](deploy-opentofu.md#the-console)和[升级](deploy-opentofu.md#upgrading)会在你执行 apply 的地方通过 `aws` 运行一次性的 Fargate task，并从 CloudWatch Logs 读取它们的输出。被中断的 apply 会让 task 继续运行，下一次 apply 会等它结束。在升级中，每个 task 定义都会得到带有新镜像的新修订版本，在某个服务采用它之前，不会有任何东西运行它；之后每个服务会等它的新 task 启动，默认服务还要等新 task 通过负载均衡器的检查，旧 task 才会退出，而部署断路器会把 task 无法启动的修订版本退回到之前的版本。

在你执行 apply 的地方，或者执行 apply 的 CI 作业所使用的凭据，除了 apply 本身所需的权限之外，还需要：

- 迁移：`ecs:RunTask`、`ecs:DescribeTasks`、`ecs:ListTasks`、`logs:FilterLogEvents`，以及 task 两个角色上的 `iam:PassRole`；
- 控制台，另外还需要：`ecs:DescribeServices`、`ecs:DescribeTaskDefinition`、`ecs:ListTagsForResource` 和 `ecs:RegisterTaskDefinition`，因为控制台会先获取正在运行的服务的镜像。

要[先检查 SQL](deploy-opentofu.md#checking-the-sql-first)，请在块仍然指向当前发布版本时先打开控制台，然后提高 `ref`，单独 apply 迁移的 task 定义，并用它运行 `my upgrade -dry-run`：

```sh
tofu apply -var console_enabled=true
# Name the new release in main.tf, then:
tofu init
tofu apply -var console_enabled=true -target='module.mycroblog.module.app.aws_ecs_task_definition.task["migrate"]'
tofu output -json console | jq -c '.task_definition = "my-migrate" | .log_prefix = "migrate" | del(.service)' |
  "$SCRIPTS/console.sh" upgrade -dry-run
```

<!-- source: monitoring-and-alerts 9b712c82cae2 -->
## 监控和告警

`monitoring` 会添加 `/readyz` 的 CloudWatch Synthetics canary、CloudWatch 中的指标，以及名称与[运维手册](alerts.md#aws-ecs)一致的告警，它们会通知 `notification_emails` 中的地址或 `notification_topic_arns` 中的 SNS 主题。该输入没有默认值：请指明通知谁，或者设置 `monitoring = null` 把这一切都省略。canary 的每次运行都会计费，`uptime_period_minutes`（默认为 15）是它的运行频率：请阅读[费用](#costs)。

<!-- source: choosing-the-backends b83753b1f860 -->
## 选择后端

每个部分都默认使用推荐的种类；要用其他种类，请[更改某个部分的 `kind`](deploy-opentofu.md#changing-a-backend)。数据库在[上面](#2-choose-the-database)。

| 部分 | 推荐 | 其他种类及适用场景 |
| --- | --- | --- |
| `media` | `s3`，区域内的一个存储桶，`versioning` 关闭 | `cloudflare.account_id` 中的 `r2`，或 `endpoint` 处的 `s3-compatible`，都需要放在你填写的机密中的访问密钥。要共用存储桶，`existing_bucket = true` 会使用你自己的存储桶，`prefix`（`tenant-a/`）则作为每个对象名称的开头；使用 `s3` 时，task 的角色此时只能访问其下的对象 |
| `kv` | `db`，不需要额外费用 | `elasticache`，空闲时也计费，并位于私有子网中；或 `external`，即存放在你填写的机密中的 Valkey URL |
| `stream` | `sns`，能到达每个 task | `postgres`，使用数据库的直连；或 `valkey`，即 `kv` 的 Valkey |
| `mail` | `ses`，不需要密码 | `smtp`，带有 `smtp.host`、`smtp.port`（587）和 `smtp.username`，其密码放在你填写的机密中 |
| `kek` | `secrets-manager`，你只需复制一次 | `kms`，一个永远不会离开 AWS KMS 的密钥。在已有数据的实例上，[更改它](deploy-opentofu.md#the-kek)相当于一次迁移 |
| `observability` | CloudWatch 和 X-Ray | `traces = "none"`，或者对发送到其他地方（例如 Sentry）的任何信号使用 `"settings"` |

`architecture = "arm64"` 会让 task 运行在 Graviton 上，费用更低；发布的镜像两种架构都有。

<!-- source: email-through-amazon-ses 0d8bc9f93296 -->
## 通过 Amazon SES 发送邮件

默认的 `mail.kind = "ses"` 使用部署所在区域的 SES API，从 `mail.from` 所在的域名发送。模块会为该域名创建启用 Easy DKIM 的身份，并允许 task 对它执行 `ses:SendEmail`，因此不需要密码；`mail.configuration_set` 会创建一个配置集并应用于每封邮件，或者用 `existing_configuration_set = true` 指定一个各实例共用的配置集；这时每个实例只处理它自己所发邮件的通知。在 SES 向你的用户发送邮件之前，请添加 `tofu output dns_records` 列出的记录并退出沙盒：[通过 Amazon SES 发送电子邮件](email-ses.md)介绍了步骤，以及服务器如何得知退信和投诉。

<!-- source: traces-to-x-ray e5ad1c35fdd5 -->
## 追踪到 X-Ray

`observability.traces = "x-ray"` 是默认值，因为这个 collector 适合推荐的配置，且没有额外费用。它是 AWS Distro for OpenTelemetry collector，在每个 task 中与服务器并排运行，只监听 task 的回环地址，并用 task 的角色（`xray:PutTraceSegments`）写入 X-Ray。它会占用每个 task 的 32 个 CPU 单位和 128 MiB，而 AWS 只按 task 的大小计费，所以默认的服务 task（256 个 CPU 单位和 1,024 MiB）费用不变，留给服务器 224 个 CPU 单位和 896 MiB。默认的 `traces_sample_ratio` 为 1%，每天 10 万个请求时每月约记录 3 万条追踪，低于 X-Ray 每月免费记录的 10 万条追踪。

真正会产生费用的是每次启动 task 时拉取的 collector 镜像，以及服务器的内存。如果你不看追踪，或者 task 太小、挤不出 128 MiB，请设置 `traces = "none"`：对于 `service`、`jobs` 或某个 worker，如果某个大小会让服务器剩下不到 128 个 CPU 单位或 512 MiB，计划会拒绝它。collector 不是必需的：如果它停止了，服务器会在没有追踪的情况下继续运行。X-Ray 是否接收它的 span 尚未验证。

<!-- source: networks 36dc33a6cbef -->
## 网络

ECS 始终需要 VPC。推荐的 `network.kind = "vpc"` 会在 `cidr_block`（`10.0.0.0/16`）中跨 `availability_zones`（至少两个，负载均衡器和 RDS 需要）创建一个 VPC：每个可用区中有一个公有子网和一个私有子网，以及一个互联网网关。它只支持 IPv4，因此服务器无法访问仅支持 IPv6 的对端。`network.kind = "existing"` 则改用你自己的 VPC：`vpc_id`、`vpc_cidr`、`public_subnet_ids`、`public_subnet_cidrs` 和 `private_subnet_ids`。

task 需要一条通往互联网的出路，用于拉取镜像、访问数据库、联邦和 AWS API，由 `network.egress` 选择。无论哪种方式，入站流量都只通过负载均衡器到达 task：

| | `public-ip`（推荐） | `nat` |
| --- | --- | --- |
| task 所在的子网 | 公有子网，每个 task 有一个公网 IPv4 地址 | 私有子网 |
| 每小时 | 每个 task 的地址 0.005 美元 | 一个 NAT 网关，约 0.045 美元，加上它的地址 |
| 每 GB | 地址不收费 | 经过网关约 0.045 美元，包括媒体和联邦的流量 |
| 使用 `existing` 时 | 没有额外要求 | 你的私有子网必须已经能访问互联网 |

除非你的某项政策禁止使用公有子网，否则请使用 `public-ip`：它便宜得多，也不会更开放。`vpc` 会在第一个可用区中创建一个 NAT 网关：该可用区出故障时，私有子网在它恢复之前无法访问互联网。无论哪种方式，RDS 和 ElastiCache 都位于私有子网中，不需要出路。

<!-- source: how-traffic-reaches-the-service d009efcf3ee2 -->
## 流量如何到达服务

负载均衡器始终存在，使用 `ingress.certificate_arn` 的证书，task 只允许它访问。`ingress.kind` 决定在它前面放什么：

| | `load-balancer`（推荐） | `cloudfront` | `cloudflare` |
| --- | --- | --- | --- |
| `host` 指向 | 负载均衡器 | 一个 CloudFront 分配 | Cloudflare，它再连接到负载均衡器 |
| 缓存 | 无 | `/media/`，依据服务器的 `Cache-Control` | `host` 上没有；`media_host` 会添加一个带缓存的主机 |
| 记录 | Route 53 用 `route53_zone_id` 创建，或由 `dns_records` 列出 | Route 53 用 `route53_zone_id` 创建，或由 `dns_records` 列出 | 由 Cloudflare 创建 |
| 需要 | | `cloudfront_certificate_arn`，位于 `us-east-1`。`cloudfront_price_class` 选择边缘节点 | `cloudflare = { zone_id = "..." }`，以及 `CLOUDFLARE_API_TOKEN` 中的令牌 |
| 封闭源站的方式 | task 的安全组 | 负载均衡器的安全组只允许 CloudFront 访问，并且它的监听器会拒绝没有 CloudFront 所发请求头的请求 | 由 Cloudflare 的规则添加、并由服务器检查的请求头 |

在 CloudFront 或 Cloudflare 之后，该请求头的值会进入 OpenTofu 状态，因此状态必须保持私密。使用 CloudFront 之前的负载均衡器记录必须先删除：已有的同名记录会让 apply 失败。CloudFront 的地址在你做计划时来自 AWS，因此请不时地 apply 一次，否则使用新地址的边缘节点会共用其查看者的速率限制。

`ingress.media_host` 会在 `load-balancer` 或 `cloudflare` 之外，借助 `cloudflare.zone_id` 在 Cloudflare 上添加一个带缓存的媒体主机。已删除的文件会一直留在缓存中，直到你清除它：请阅读[通过 CDN 提供媒体](cdn.md)。在任何东西前面使用 Cloudflare 时，请在它的控制面板中关闭 Bot Fight Mode，因为 `/ap/`、`/.well-known/` 或 `/api/` 上的质询会破坏联邦，而模块不管理它。Cloudflare 这几种只在模拟的 provider 上运行过。

<!-- source: limits 42ed50837851 -->
## 限制

- **健康检查**：负载均衡器唯一的检查请求 `/readyz`，数据库宕机时它也会失败：失败 2.5 分钟后，ECS 会替换 task，并一直这样做，直到数据库恢复。没有容器检查，因此没有负载均衡器的 worker 遇到错误的镜像时，只会在它的日志中失败。
- **上传**：负载均衡器不限制请求体，因此由服务器的 `media_max_*` 设置决定。
- **流**：流每 30 秒发送一次心跳，短于负载均衡器 60 秒的空闲超时。它会一直保持打开，直到它的 task 退出，然后客户端会重新连接。
- **客户端地址**：速率限制依赖服务器所信任的内容，这由模块根据 ingress 设置。请在第一批请求日志中检查客户端地址：如果是负载均衡器所在子网的地址，或 CloudFront 的地址，就说明设置有误。
- **磁盘**随 task 一起消失，因此计划会拒绝 SQLite 和文件系统上的媒体。
- 使用客户管理的 KMS 密钥的**机密**还需要 `kms:Decrypt`，模块不会授予它。

<!-- source: costs 19e0e43bc201 -->
## 费用

ECS 上没有任何东西会缩容到零：task、负载均衡器和它的公网地址整月都计费。这些数字来自 AWS 东京区域 2026 年 10 月和 11 月的价格表（除非某一行另有说明），尚未与账单核对过。

| 部分 | 每月 | 选择 |
| --- | --- | --- |
| 一个 0.25 vCPU、1 GB 的 task | x86 为 13.26 美元，arm64 为 10.61 美元 | `service`、`architecture`。每个 worker 会增加它自己的 task |
| 负载均衡器 | 17.74 美元，外加整月使用的每个容量单位 0 至 5.84 美元 | 始终存在 |
| 负载均衡器的公网地址 | 7.30 美元，两个可用区 | 每个可用区一个 |
| 出路 | 使用 `public-ip` 时每个 task 3.65 美元；使用 `nat` 时约 36.50 美元，另加每 GB 0.045 美元（按俄亥俄的费率） | `network.egress` |
| 数据库 | Neon 的 Launch 套餐在 0.25 CU 时约 19 美元，因为计算资源永远不会停止；使用 `rds`、`db.t4g.micro` 和 20 GB 时约 21 美元 | `database` |
| 缓存和速率限制 | 使用 `db` 时 0 美元；使用 `elasticache` 时在弗吉尼亚北部约 6.13 美元，AWS 的另一份文档则给出约 61 美元 | `kv` |
| canary | 每 15 分钟一次时约 3.34 美元；每分钟一次时约 51.72 美元 | `monitoring.uptime_period_minutes` |
| 追踪 | 每月 10 万条追踪以内 0 美元 | `observability.traces` |

**空闲时，使用 `public-ip` 约 42 美元，使用 `nat` 约 75 美元**，另加容量单位，且不含数据库、存储、流量和 canary。没有查阅、因而不在任何一行中的有：CloudFront、Secrets Manager、CloudWatch Logs、S3、SNS 和 SQS、流出 AWS 的数据，以及任何免费额度。一次性 task 按它们运行的分钟数收取其规格对应的费用，升级会在几分钟内多运行一个服务 task。
