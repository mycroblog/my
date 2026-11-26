<!-- source: deploying-on-google-cloud 03b87618467f -->
# 在 Google Cloud 上部署

本文介绍如何用 OpenTofu 模块 `mycroblog-gcp` 在 Google Cloud 上运行 `my`：一个 Cloud Run 服务，以及每个角色的后端，后者由其区段的 `kind` 选择。每个区段都默认使用模块的推荐配置，在没有人使用服务器时不运行任何实例，只有一个每分钟唤醒服务器的 tick。运行服务器需要花钱：在存储和流量之外，使用 Neon 上数据库的推荐配置每月约 37 美元，[费用](#costs)给出了每种选择的数字。

每个 OpenTofu 部署共有的内容，包括块及其区段、机密、控制台以及升级，都在[使用 OpenTofu 部署](deploy-opentofu.md)中。本文补充 Google Cloud 所需要的内容。

<!-- source: what-it-creates 7b13c8bbc968 -->
## 会创建什么

`mycroblog-gcp` 的一个块会创建下面的内容。每一行的第一种是推荐的种类，也是默认值；[模块的参考](../../deploy/opentofu/README.md#sections)列出了每个字段。

| 角色 | 区段 | 推荐 | 其他种类 |
| --- | --- | --- | --- |
| 服务器 | `service`、`jobs` | 一个 `role = all` 的 Cloud Run 服务、一个迁移作业、需要时的控制台作业、一个服务账号，以及公开访问权限 | |
| 流量 | `ingress` | `load-balancer`：带有 Google 管理证书的全球外部应用负载均衡器 | `domain-mapping`、`cloudflare`，以及可与其中任何一种并用的 Cloudflare 上带缓存的媒体主机 |
| 数据库 | `database` | `external`：一个存放你自己运行的 PostgreSQL（例如 Neon）的 URL 的机密 | `cloud-sql`：一个实例及其数据库、用户和密码 |
| 后台任务 | `queue` | `cloud-tasks`：一个 Cloud Tasks 队列、tick 所用的 Cloud Scheduler 作业，以及一个调用服务器的服务账号 | `db`：后台任务放在数据库中 |
| 媒体 | `media` | `gcs`：一个私有 Cloud Storage 存储桶 | `r2`、`s3` |
| 缓存和速率限制 | `kv` | `db`：放在数据库中，不需要创建任何东西 | `memorystore`、`external`（Valkey） |
| 实时更新 | `stream` | `pubsub`：一个主题，以及让服务器管理自己订阅的自定义角色 | `postgres`、`valkey` |
| 邮件 | `mail` | 任何 SMTP 服务，以及一个存放其密码的机密 | |
| KEK | `kek` | `secret-manager`：一个自动生成、以文件形式挂载的机密 | `kms`：Cloud KMS 中的密钥 |
| 网络 | `network` | `none`：没有要附加的东西 | `vpc`、`existing` |
| 指标、追踪、错误 | `observability` | Cloud Monitoring、Cloud Trace 和 Error Reporting，它们读取 Cloud Run 发送给它们的数据。模块会启用它们的 API | `settings`，例如 Sentry |
| 告警 | `monitoring` | 一个可用性检查、基于日志的指标和告警策略：[监控和告警](#monitoring-and-alerts) | `null`，即没有 |

模块还会生成 VAPID 密钥，并为只有你能填写的机密创建空机密。

`examples/gcp-cloud-run-neon` 是一种起步方式：provider 加上一个使用每种推荐种类的块，它适用于任何外部数据库，不论是不是 Neon。请把你要更改的区段加到它的块中。

<!-- source: before-you-start 9869afda9a07 -->
## 开始之前

- [首次部署](deploy-opentofu.md#first-deployment)所需要的一切，其中平台的 CLI 是 `gcloud`，并为 provider 运行 `gcloud auth application-default login`。
- 一个已启用结算、专供本实例使用的 Google Cloud 项目，以及该项目中拥有 Owner 角色的账号。`stream = pubsub` 的自定义角色是放在项目上的，所以它会触及该项目中任何其他东西的订阅。

<!-- source: first-deployment 894b6af2cee9 -->
## 首次部署

请在这里完成前两个步骤，然后按照[首次部署](deploy-opentofu.md#first-deployment)操作，并参考第三步的说明。

<!-- source: 1-enable-the-apis fdd2dc400c20 -->
### 1. 启用 API

每个部署都需要这些：

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com
```

再添加每个需要 API 的选择所对应的 API：

| 选择 | API |
| --- | --- |
| `ingress = load-balancer` | `compute.googleapis.com` |
| `database = cloud-sql` | `sqladmin.googleapis.com` |
| `kek = kms` | `cloudkms.googleapis.com` |
| `network = vpc` 或 `existing` | `compute.googleapis.com`、`servicenetworking.googleapis.com`、`networkconnectivity.googleapis.com` |
| `kv = memorystore` | `memorystore.googleapis.com`、`serviceconsumermanagement.googleapis.com`，以及网络所需的那些 |

`translation` 会启用它自己的 API，`observability` 也一样。[导入云使用费用](billing-import.md)列出了 `billing` 所需要的 API。

<!-- source: 2-prepare-the-database b54e3b01002b -->
### 2. 准备数据库

- **`external`**：创建数据库，并保留它的直连 URL 和（如果有的话）池化 URL，供 `secrets_to_fill` 使用：[数据库](#the-database)说明了各自放在哪里。模块从不创建它，因此它的密码不会进入 OpenTofu 状态。
- **`cloud-sql`**：无需做任何事。模块会创建实例，并把它的 URL（带有生成的密码）写入机密。

<!-- source: 3-deploy 4470affd40e4 -->
### 3. 部署

请按照[首次部署](deploy-opentofu.md#first-deployment)操作。在 Google Cloud 上：

- **变量**：示例中的 `mail_from`、`mail_smtp_host` 和 `mail_smtp_username` 用于配置[邮件](email.md)。端口是 587，因为 Google Cloud 屏蔽出站 25 端口。
- **机密**：KEK 是 `my-kek`，VAPID 密钥是 `my-vapid-key`，每个都以 `name` 开头，用 `gcloud secrets versions access latest --secret=<id> --project="$PROJECT"` 复制它们。KEK 以挂载文件的形式交给服务器，由 `MY_KEK_FILE` 指定，其他机密则作为环境变量。
- **Apply**：迁移作业最先创建，连同它所依赖的内容，其中包括 ingress，因为服务器的设置会指明它。服务的启动探针会等待 `/health/ready`，在 schema 存在之前它会返回 `503`。
- **DNS**：请添加 `tofu output dns_records` 中的记录，每种 kind 的做法见[流量如何到达服务](#how-traffic-reaches-the-service)。在证书生效之前，`https://{host}` 会因 TLS 错误而失败，因此联邦和浏览器暂时无法访问服务器。其余的都不必等它：Cloud Tasks 和 Cloud Scheduler 调用服务的默认 URL `https://<name>-<project number>.<region>.run.app`。**请不要禁用默认 URL**，否则后台任务会停止。Cloud Run 只把来自同一项目、使用该 URL 的这两个调用方视为内部调用，因此负载均衡器才能对互联网关闭它。
- **第一个管理员**：链接保留在 Cloud Logging 中，任何能读取项目日志的人都可以使用它。

<!-- source: the-console-and-upgrades a40a1df56865 -->
## 控制台和升级

[控制台](deploy-opentofu.md#the-console)和[升级](deploy-opentofu.md#upgrading)会在你执行 apply 的地方通过 `gcloud` 运行 Cloud Run 作业 `<name>-console` 和 `<name>-migrate` 的执行。它们需要能够执行这些作业并读取其日志的凭据，即 `roles/run.developer` 和 `roles/logging.viewer`，Owner 已包含它们。被中断的 apply 会让执行在 Cloud Run 上继续进行，下一次 apply 会等它结束。

要[先检查 SQL](deploy-opentofu.md#checking-the-sql-first)：

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region="$REGION" --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

<!-- source: the-database 0303fdd046ef -->
## 数据库

模块推荐 `external`，它不需要网络，在 Google Cloud 上也不产生任何费用：空闲时会停止的数据库（例如 Neon）能让安静的服务器保持低成本。`cloud-sql` 把一切都放在一个项目中，费用固定。在两者之间迁移数据属于[更换后端](deploy-opentofu.md#changing-a-backend)。

<!-- source: an-external-postgresql d6604bfb9e2f -->
### 外部 PostgreSQL

默认的 `database = { kind = "external" }` 接受 Cloud Run 能够访问的任何 PostgreSQL：Neon、其他托管的 PostgreSQL，或者你自己运行的 PostgreSQL，可以通过互联网，也可以通过[网络](#networks)使用私有地址。模块会为**直连** URL 创建机密 `my-database-url`，迁移作业和控制台始终使用它：`my schema apply` 会持有会话级锁，而事务模式的连接池无法保持这种锁。

`database = { kind = "external", pooled = true }` 会为**池化** URL 添加机密 `my-database-url-pooled`，服务随后会使用它。连接池适合服务器，如[通过连接池](database.md#through-a-connection-pooler)所述。没有连接池时，服务也使用直连 URL，因此 `service.max_instances` 乘以每个实例的连接池大小，必须低于数据库的连接上限。连接池默认每个实例 4 个连接，而一个实例同时接受 80 个请求：请在服务所用 URL 中用 `pool_max_conns` 调高它，例如 `...?sslmode=require&pool_max_conns=8`（[等待数据库连接](running.md#waiting-for-a-database-connection)）。

**在 Neon 上**，请在靠近你的 Cloud Run 区域的地区创建项目。它的直连字符串使用普通主机名，池化字符串使用带 `-pooler` 的主机名，那是 Neon 的连接池，适合服务器：请设置 `pooled = true`。请按 Launch 套餐来预期，因为默认的 tick 会让计算资源保持唤醒；Free 套餐也能用，但要接受[费用中的取舍](#where-care-reduces-it)。

<!-- source: cloud-sql dc9461df8cc2 -->
### Cloud SQL

`database = { kind = "cloud-sql" }` 会创建一个 PostgreSQL 18 实例（默认为 `db-f1-micro`），带有每日备份和保留 7 天的时间点恢复，以及它的数据库和用户，还有一个既不进入计划也不进入状态的生成密码。`tier`、`disk_size_gb`、`retained_backups` 和 `deletion_protection` 可以更改它。它从未被应用过：请把第一次部署当作它的测试。

- **没有网络时**，Cloud Run 通过 Cloud SQL 连接器的套接字访问它，实例的公网地址不允许任何其他访问。
- **有网络时**，它只有一个私有地址，`pg_dump` 必须从网络内部运行。
- **它没有连接池**：`pooled` 会被拒绝，连接数由 `service.max_instances` 和 `pool_max_conns` 限定。在只有 0.6 GB 内存的 `db-f1-micro` 上，请保持连接池较小。
- **它永远不会缩容到零**，所以无论流量如何都会计费，而且 Google 的 SLA 不涵盖共享核心的规格。

<!-- source: the-other-backends 91ef8e2dea11 -->
## 其他后端

每个后端都默认使用它的推荐配置，[更改 `kind`](deploy-opentofu.md#changing-a-backend)即可换成另一种。

- **后台任务**：`cloud-tasks` 把每个任务推送给可以缩容到零的服务。默认每分钟一次的 `queue.tick_schedule` 会为定期任务唤醒服务器和数据库。`db` 从一个始终运行的进程轮询数据库，因此需要 `service.min_instances` 为 1 或更大，以及 `service.cpu_always_allocated = true`。
- **媒体**：`gcs` 会在该区域创建一个私有存储桶 `media.bucket_name`。`r2` 会在 `cloudflare.account_id` 中创建一个 R2 存储桶，`s3` 则使用 `endpoint` 处任何兼容 S3 的存储；两者都从 `secrets_to_fill` 列出的机密中读取访问密钥。要与其他实例共用存储桶，`existing_bucket = true` 会使用你自己的存储桶，`prefix`（`tenant-a/`）则作为每个对象名称的开头；使用 `gcs` 时，服务账号此时只能访问其下的对象，这需要统一的存储桶级访问权限。
- **缓存和速率限制**：`db` 把它们保存在数据库中。`memorystore` 会创建 Memorystore for Valkey，它需要网络，并且没有密码：网络上的任何东西都能访问它。`external` 从机密中读取你自己运行的 Valkey 的 URL。
- **实时更新**：`pubsub` 能到达每个实例。`postgres` 通过数据库的直连使用 `LISTEN` 和 `NOTIFY`，`valkey` 则使用 `kv` 的 Valkey。
- **KEK**：`secret-manager` 只生成一次，供你复制。`kms` 把它保存在 Cloud KMS 中，它永远不会离开 KMS，所以即使数据库转储和配置同时泄露，也不会暴露任何机密。在已有数据的实例上更改它属于[一次迁移](deploy-opentofu.md#the-kek)。

<!-- source: networks 9d28311e11a6 -->
## 网络

推荐的配置不需要网络：外部数据库、Cloud Tasks、Cloud Storage 和 Pub/Sub 都通过互联网、用服务自己的身份访问，所以 Cloud Run 不附加到任何网络。

Memorystore 只有私有地址，要为 Cloud SQL 提供私有地址，或者要访问你自己位于私有网络上的数据库，就需要网络。`network = { kind = "vpc" }` 会创建 VPC、它的子网和专用服务访问，`network = { kind = "existing", ... }` 则附加到你已有的 VPC，你用 `network`、`subnetwork`、`private_services_range` 和 `memorystore_connection_policy` 来描述它：

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

服务和作业通过 Direct VPC 出站附加，只有私有范围会经过该网络，因此互联网保持自己的路径，不需要 Cloud NAT。使用 `vpc` 时，`nat = true` 会改为让所有流量经过 Cloud NAT，以获得固定的源地址。网络可能会让实例的启动延迟一分钟或更久，而缩容到零的服务器经常会遇到这种情况。模块创建的 VPC 从未被应用过。

<!-- source: monitoring-and-alerts eb28e653de36 -->
## 监控和告警

`monitoring` 会添加对 `/health/live` 的可用性检查、基于日志的指标，以及名称与[运行手册](alerts.md)一致的告警策略，它们会通知 `notification_emails`（示例中为 `alert_emails`）中的地址或 `notification_channels` 中的渠道。该区段没有默认值：请指明通知谁，或设置 `monitoring = null` 来完全不要它。它从未被应用过，所以请通过让一次检查失败来试用一次。每次运行检查都会唤醒一个实例：`uptime_period`（默认 15 分钟）决定了多久一次。

<!-- source: traces b1f84a2f93b9 -->
### 追踪

Cloud Trace 会显示 Cloud Run 为每个请求记录的追踪，服务器的日志行带有它的 `trace_id`，因为模块信任 ingress（`MY_TRUSTED_PROXIES`）。模块设置了 `logs_exporter = gcp`，因此每一行还会填充 Cloud Logging 链接到 Cloud Trace 的追踪字段（`logging.googleapis.com/trace`），每个访问日志行会显示为一个请求。`observability.logs = "settings"` 会把 `logs_exporter` 交给 `settings`。尚未验证：Cloud Logging 是否会凭它写入的裸追踪 ID 链接一行，以及 Cloud Run 是否总会替换公开请求的 `traceparent`。服务器自己的 span 需要在它旁边放一个 OpenTelemetry Collector，而模块不运行它。

<!-- source: how-traffic-reaches-the-service 094ddaf201c8 -->
## 流量如何到达服务

这由 `ingress.kind` 决定。模块为生产环境推荐负载均衡器。域名映射适合低成本地试用部署。

| | `load-balancer` | `domain-mapping` |
| --- | --- | --- |
| 月费 | 转发规则约 18 美元，另按 GB 收费。请查看[价格页面](https://cloud.google.com/vpc/network-pricing)：这个数字未能在那里得到确认 | 无 |
| 状态 | Google Cloud 推荐的方式 | 预览版功能，由于延迟问题，Google Cloud 不推荐用于生产环境 |
| 区域 | 任意 | `asia-northeast1`、`us-central1` 等少数几个 |
| DNS | 一条把 `host` 指向它的地址的 `A` 记录。请对它关闭你的 DNS 服务商的任何代理 | `dns_records` 列出的记录 |
| 证书 | 由 Google 管理，DNS 指向该地址后生效，这可能需要一段时间 | 由 Google 管理，最长需要 24 小时 |
| 域名所有权 | 除 DNS 外无其他要求 | 执行 apply 的人必须先成为该域名的[已验证所有者](https://cloud.google.com/run/docs/mapping-custom-domains) |
| `run.app` URL | 拒绝来自互联网的请求，因此没有人能绕过负载均衡器 | 任何人都能访问：域名映射无法添加请求头，因此源站保持开放 |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` 和负载均衡器的地址 | `169.254.0.0/16` |

Cloud Tasks 和 Cloud Scheduler 不经过这两者：它们直接调用默认的 `run.app` URL。

<!-- source: cloudflare 35dc1e54d2bb -->
### Cloudflare

`ingress = { kind = "cloudflare" }` 会把 Cloudflare 放在 `host` 前面，并让所有内容绕过缓存；`ingress.media_host` 则在任何 kind 旁边添加一个带缓存的媒体主机。两者都需要 `cloudflare = { zone_id = "..." }`，provider 从 `CLOUDFLARE_API_TOKEN` 读取令牌，模块会创建这些记录，所以 `dns_records` 是空的。它们只在模拟的 provider 上运行过。

- **你的套餐决定源站。** Cloud Run 按 `Host` 路由，而 Cloudflare 只在 Enterprise 套餐中覆盖 Host 请求头。在任何其他套餐上，模块会为每个主机创建一个域名映射，带有上表所述的取舍，由 Cloudflare 连接到它。它的证书能否在代理之后签发和续期尚未验证，因此请从 `cloudflare.proxied = false` 开始。在 Enterprise 套餐上，`cloudflare.host_header_override = true` 会改为连接到 `run.app` URL，不需要域名映射。
- **用请求头封闭源站。** 一条请求头转换规则会添加由模块生成的值，服务器会以 `403` 拒绝没有该值的请求，但健康检查和作业自己的 `POST`（它们会检查自己的令牌）除外。这样就没有人能通过 `run.app` URL 绕过 Cloudflare，也没有人能选择速率限制所看到的客户端地址。
- **只要记录没有被代理，就没有任何东西会添加该请求头**，所以服务器会拒绝除健康检查之外的一切：在为域名映射的证书使用 `proxied = false` 时，请在证书生效之后再开启代理。
- **该值会进入状态**，如[机密](deploy-opentofu.md#secrets)所述，也会进入机密 `my-origin-secret`。如何轮换它见 [CDN 指南](cdn.md#rotate-the-value)。
- **请关闭 Bot Fight Mode**，在仪表板中操作，并且不要开启“I'm Under Attack”：对 `/ap/`、`/.well-known/` 或 `/api/` 的质询会破坏联邦，因为其他服务器不是浏览器。两者都覆盖整个区域，没有任何规则可以豁免。模块不添加任何质询，并会关闭 Browser Integrity Check。
- **Email Address Obfuscation 可以保持开启。** 服务器自己的页面会标记它必须保留其中地址的内容。
- 如果你希望 `http://` 被重定向，请在仪表板中**开启 Always Use HTTPS**。它覆盖整个区域，所以模块把它留给你处理。
- **超时**：当源站 125 秒内不发送任何内容时，Cloudflare 会返回 `524`。作业不受影响，因为它们不经过 Cloudflare。流每 30 秒发送一次心跳，低于 900 秒的空闲上限。在被代理的主机上，Cloudflare 是否会缓冲流尚未验证。
- **视频**：Cloudflare 的条款限制在 Free、Pro 和 Business 套餐上提供视频以及不成比例的大文件，它可能会对这些内容采取措施。在把视频缓存到媒体主机之前，请先决定这一点。

**被删除的文件仍会保留在缓存中。** 媒体主机遵循服务器的 `Cache-Control`，因此被删除的本地文件最长可能继续可见一年，远程文件则是 `remote_media_cache_ttl_seconds`。服务器不会清除它。当某些内容必须消失时，请手动清除，在仪表板的 **Caching > Configuration > Custom Purge** 下，或者：

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

代理 URL 的查询字符串中带有它的 `sig`，它是被缓存内容的一部分，所以请给出完整的 URL。

<!-- source: limits 3d4721415f18 -->
## 限制

- **上传**：Cloud Run 拒绝超过 32 MiB 的 HTTP/1 请求，而服务器使用 HTTP/1.1，因此模块将 `MY_MEDIA_MAX_AV_BYTES` 设为 30 MiB（默认为 100 MiB），让更大的上传得到服务器自己的响应。Cloudflare 自己的上限（Free 和 Pro 套餐为 100 MB）高于它。
- **流**：`service.request_timeout_seconds`（默认 15 分钟）会结束打开的流，客户端会重新连接。
- **数据库连接**：Google 限制一个 Cloud Run 实例最多使用 100 个到 Cloud SQL 的连接。
- **客户端地址**：速率限制依赖于 `MY_TRUSTED_PROXIES`，模块根据 ingress 设置它。我们读过的文档都没有说明 Cloud Run 对直接请求会在 `X-Forwarded-For` 中添加什么，也没有说明容器看到的对端地址是什么，因此 `169.254.0.0/16` 只是一个假设。负载均衡器文档中记载的形式是 `[<supplied>,]<client-ip>,<lb-ip>`。请在最初的请求日志中检查地址：如果客户端地址是 `169.254` 地址或 Google 的地址，就说明该设置对你的项目而言是错误的。
- **公开访问**：服务向 `allUsers` 授予 `roles/run.invoker`，而针对网域限制共享的组织政策会拒绝这一点。这样的项目需要设置例外。
- **仍被占用的名称**：Cloud SQL 和 Cloud Tasks 会在大约一周内拒绝已删除实例或队列的名称，已删除的自定义角色也会在一段时间内保留它的 ID，因此在 `destroy` 之后重新创建的部署需要另一个 `name`。

<!-- source: costs ea658d24d79d -->
## 费用

这些数字是一份指南，说明大致会花多少、在哪里用心配置可以节省，而不是部署必须遵守的上限。它们是某一区域的大致标价，取自 2026-10 和 2026-11：请在 [Cloud Run](https://cloud.google.com/run/pricing)、[Cloud SQL](https://cloud.google.com/sql/pricing) 和 [Neon](https://neon.com/pricing) 的价格页面上核实。每个结算账号每月有 180,000 vCPU 秒和 360,000 GiB 秒的 Cloud Run 免费计算额度。

每个部署都要支付的费用，空闲时约为 0 美元：

| 部分 | 空闲时 | 增加费用的因素 |
| --- | --- | --- |
| Cloud Run | 0 美元：没有实例运行，CPU 只在请求期间计费 | 每个打开的流都会让一个实例持续计费；见下文 |
| Cloud Tasks | 0 美元 | 100 万次操作免费，之后每百万次 0.40 美元。一个任务约需 2 次操作（加入队列和投递） |
| Cloud Scheduler | 0 美元：tick 是一个作业，每个结算账号 3 个作业免费 | 超出部分每个作业 0.10 美元 |
| Pub/Sub | 0 美元：10 GiB 免费 | 仅在流打开期间产生 |
| Secret Manager | 几美分 | 少量版本免费，之后每个版本几美分 |
| Cloud Storage | 每 GiB 每月约 0.02 美元 | 只有 `us-east1`、`us-west1` 和 `us-central1` 有 5 GiB 免费额度。媒体经由 Cloud Run 发出 |
| Monitoring | 0 美元，在免费额度之内 | |

每种选择增加的费用：

| 区段 | 选择 | 每月 |
| --- | --- | --- |
| `ingress` | `load-balancer` | 约 18 美元，未在价格页面上得到确认，另按 GB 收费 |
| | `domain-mapping` | 0 美元 |
| | `cloudflare` | 你的 Cloudflare 套餐，以及源站：使用域名映射时为 0 美元 |
| `database` | Neon Launch 套餐上的 `external` | 使用默认 tick 时约 19 美元，另加每 GB 每月 0.35 美元 |
| | Neon Free 套餐上的 `external` | 0 美元，在它的限制之内：[见下文](#where-care-reduces-it) |
| | 其他地方的 `external` | 该数据库的费用 |
| | `cloud-sql` | `us-central1` 中 `db-f1-micro` 和 10 GiB 约 9.40 美元，另加备份，无论流量如何 |
| `queue` | `db` | 一个全天候计费、CPU 始终分配的实例 |
| `kv` | `memorystore` | 最小的节点约 23 美元，无论流量如何 |
| `kek` | `kms` | 几美分，用于密钥及其操作 |
| `network` | 带 `nat = true` 的 `vpc` | Cloud NAT，按小时和按 GB 计费 |

在 Neon Launch 套餐上的推荐配置，即负载均衡器加 Neon，每月约 37 美元；使用 Cloud SQL 则约 27 美元。

**tick 会让 Neon 保持唤醒**。每次 tick 都会读写数据库，而 Neon 只有在 5 分钟没有活动后才会停止计算资源，因此使用默认的 `* * * * *` 时，计算资源永远不会停止。这相当于每月 730 小时，按 0.25 CU 计为 182.5 CU 小时，也就是上面的 19 美元。tick 保持每分钟一次，是为了让定期任务按时运行，并让丢失的唤醒信号能尽快恢复。无论 tick 如何，Cloud SQL 的计费都相同。

**打开的流会让实例持续计费**。在按请求计费的模式下，实例在处理任何请求期间都会计费，而实时更新是一个长请求。一个整月保持流打开的客户端，就会让一个实例全天候计费：扣除免费额度后，1 vCPU、512 MiB 的实例每月约 60 美元。更多的流会共用该实例，最多到 `service.concurrency`。如果流大部分时间都保持打开，请与按实例计费的模式（`service.cpu_always_allocated`），或与小型虚拟机加 [Docker Compose](deploy-docker-compose.md) 的方案进行比较。

<!-- source: where-care-reduces-it 427be9d46d32 -->
### 用心配置可以节省的地方

- 用**域名映射**代替负载均衡器，可以省去 18 美元，但要接受[它的取舍](#how-traffic-reaches-the-service)。它适合试用部署。
- **Neon 的 Free 套餐**可以省去 19 美元，小型实例可以在上面运行，但有以下取舍：
  - 它每月允许 100 CU 小时，默认 tick 大约 17 天就会用完。使用更长的计划可以保持在额度之内，例如 `queue = { tick_schedule = "*/10 * * * *" }`：在其他因素唤醒它之前，最多一半时间处于唤醒状态，约 91 CU 小时。此时定期任务最多会延迟 10 分钟运行，丢失的唤醒信号也会更晚恢复。
  - 联邦服务器全天候接收投递，这也会唤醒数据库，因此无论计划如何，繁忙的实例都会超出它的额度。
  - 每个项目容量为 1 GB，可恢复的历史为 6 小时，而且计算资源在空闲 5 分钟后总会停止，因此之后的第一个请求需要等待它启动。
