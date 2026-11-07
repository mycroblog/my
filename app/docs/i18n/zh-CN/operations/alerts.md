<!-- source: alerts-and-runbook 2c201450ab32 -->
# 告警和运行手册

本页介绍服务器自身的指标应当就哪些情况向你发出告警、每个告警的紧急程度，以及告警触发时要检查什么、做什么。从外部监控服务器（可用性监控和状态页面）的内容见[从外部监控服务器](monitoring.md)；也请把它配置好，因为宕机的服务器不会发来任何指标。

<!-- source: getting-the-metrics 58d8055fe44d -->
## 获取指标

设置 `metrics_exporter` 后，服务器会导出其指标（[配置参考](../reference/configuration.md#metrics_exporter)）；[运行指南](running.md)列出了每个指标及其标签。下面的告警是为 Prometheus 编写的，使用的名称是 Prometheus 看到的名称：点号变为下划线，并加上单位和计数器后缀（`my.queue.oldest_ready_age` 变为 `my_queue_oldest_ready_age_seconds`，`my.delivery.attempts` 变为 `my_delivery_attempts_total`）。

| 目标环境 | 设置 | 告警方式 |
| --- | --- | --- |
| 自托管（Docker Compose、Kubernetes、虚拟机） | `metrics_exporter = prometheus` | Prometheus 抓取 `metrics_listen` 上的 `GET /metrics`，由 Alertmanager 路由告警。下面的规则可以直接使用 |
| 任何带有 OpenTelemetry Collector 的环境 | `metrics_exporter = otlp` | Collector 转发到你选择的后端。添加单位和类型后缀的 Prometheus 兼容后端（Prometheus 自带的 OTLP 接收器默认如此）看到的名称相同 |
| Google Cloud（Cloud Run） | `metrics_exporter = none` | 基于可用性检查、平台指标和基于日志的指标的 Cloud Monitoring 告警策略（参见 [Cloud Run](#google-cloud-cloud-run)） |
| AWS（ECS） | `metrics_exporter = emf` | 基于 CloudWatch Logs 提取的指标的 CloudWatch 告警（参见 [AWS ECS](#aws-ecs)） |

<!-- source: prometheus-and-alertmanager 8e8dcaa334e9 -->
### Prometheus 和 Alertmanager

规则位于 [`deploy/monitoring/prometheus/alerts.yml`](../../deploy/monitoring/prometheus/alerts.yml)。把该文件复制到你的 `prometheus.yml` 旁边并加载它：

```yaml
rule_files:
  - alerts.yml

scrape_configs:
  - job_name: my          # the rules expect this job name
    static_configs:
      - targets: ["my:9464"]
```

- `metrics_listen` 默认为 `127.0.0.1:9464`，只有同一主机上的 Prometheus 能访问。在 Docker Compose 或 Kubernetes 中，请设置 `MY_METRICS_LISTEN=0.0.0.0:9464`，并把该端口限制在私有网络中：这个监听器没有身份验证。
- 抓取每个进程（每个 `role = web` 和 `role = worker` 的进程都有自己的监听器）。队列、媒体、数据库大小、远程服务器、账单导入和快照这些仪表在每个进程中都携带相同的实例级数值，因此规则取它们的最大值。
- 每条规则都有一个 `severity` 标签：**`page`** 需要立即有人处理（用户已经或即将受到影响），**`ticket`** 可以等到工作时间处理。请在 Alertmanager 中把它们路由到不同的接收方。每条规则的 `runbook_url` 指向下面对应的小节。
- 这些阈值是针对小型服务器的起点。观察一周的数值，再根据你的流量调整阈值和 `for` 持续时间。`MyRemoteMediaCacheOverLimit`、`MyMediaStorageLarge` 和 `MyDatabaseStorageLarge` 中的大小必须按你自己的限制设置。
- 维护模式期间，服务器不提供指标并返回 `503`，因此 `MyTargetDown` 和 HTTP 告警会触发。请在开始之前，在 Alertmanager 中为这段时间静默告警（[维护](maintenance.md#steps)）。

更改规则后，用 Prometheus 自带的 `promtool` 检查文件并运行其单元测试（`alerts.test.yml`）：

```sh
cd deploy/monitoring/prometheus
promtool check rules alerts.yml
promtool test rules alerts.test.yml
```

<!-- source: google-cloud-cloud-run 72c348585d24 -->
### Google Cloud（Cloud Run）

使用 `metrics_exporter = none` 时，请利用 Cloud Run 和 Cloud Logging 已有的数据构建告警：

- **HTTP 错误和延迟**：服务的按响应码类别统计的 Cloud Run 请求数，以及请求延迟指标。
- **错误**：`error report` 行会进入 Error Reporting，它会对错误分组，并可以在出现新错误和重复错误时通知你（[错误报告](error-reporting.md#google-cloud-cloud-run)）。
- **任务和请求**：服务器为每个 HTTP 请求（`msg` = `http request`）和每次任务运行（`msg` = `job`）写一行日志，标签与指标相同。可以据此创建基于日志的指标，例如用 `jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"` 的计数器统计失败的任务，或用按 `jsonPayload."my.surface"="client_api"` 过滤的 `jsonPayload.duration_seconds` 分布衡量延迟。
- **队列积压**：使用 `queue = cloudtasks` 时，任务存放在数据库中，因此任务页面和任务运行的日志行会显示积压情况；Cloud Tasks 的队列深度指标统计的是等待投递的信号。
- **数据库**：Cloud SQL 的磁盘使用率和连接指标，或你的数据库服务商的用量告警。
- **费用**：带有告警阈值的 Cloud Billing 预算（[费用和预算](#cost-and-budget)）。

模块 [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) 会以下表中的名称把这些创建为策略，并列出它以不同方式覆盖或无法表达的每个告警。该模块从未实际应用过。只有流量才会产生的指标在缩容到零的服务上会出现空缺，因此针对其缺失的告警会误报；能发现闲置服务器宕机的是它的可用性检查。

<!-- source: aws-ecs 609e5bf632ef -->
### AWS ECS

使用 `metrics_exporter = emf` 时，CloudWatch Logs 会把服务器的日志行转换为命名空间 `metrics_emf_namespace` 中的指标，使用带点号的名称（`my.job.duration`、`my.queue.oldest_ready_age`）。请求和任务耗时指标来自请求和任务的日志行，维度是你在 `metrics_emf_dimensions` 中列出的标签；要对失败的任务告警，请包含 `my.outcome`，要对 5xx 告警，请包含 `http.response.status_code`。实例级仪表（队列、媒体、数据库大小、远程服务器、账单导入、快照）由周期性的快照任务写入。Go 运行时指标（`go.*`）不会以 EMF 写入；请改用 ECS 服务的 CPU 和内存指标。

- **HTTP 错误**：Application Load Balancer 的 5xx 和请求数指标。
- **队列积压**：由快照任务写入的 `my.queue.oldest_ready_age`。
- **数据库**：RDS 或 Aurora 的存储和连接指标，或你的数据库服务商的用量告警。
- **邮件**（`mail = ses`）：CloudWatch 中的 SES 信誉指标（退信率和投诉率）。
- **费用**：带有告警的 AWS Budgets 预算（[费用和预算](#cost-and-budget)）。

模块 [`monitoring-aws`](../../deploy/opentofu/modules/monitoring-aws/README.md) 会以下表中的名称把这些创建为告警，列表相同。该模块从未实际应用过。

<!-- source: the-alerts d76842ca02b8 -->
## 告警列表

严重程度：**page** = 立即处理，**ticket** = 在工作时间处理。规则列中的“–”表示目前还没有对应的指标；请按说明进行检查。

| 领域 | 告警 | 信号 | 阈值 | 严重程度 | 可能的原因 |
| --- | --- | --- | --- | --- | --- |
| 队列 | [`MyQueueBacklog`](#myqueuebacklog) | `my_queue_oldest_ready_age_seconds` | 超过 5 分钟，持续 10 分钟 | ticket | worker 不足以应对突发流量（热门帖子、中继）、任务缓慢、worker 已停止 |
| 队列 | [`MyQueueStalled`](#myqueuestalled) | `my_queue_oldest_ready_age_seconds` | 超过 30 分钟，持续 5 分钟 | page | 没有 worker 在运行（崩溃、未部署、只有 `role = web`）、没有 worker 运行某种任务类型、worker 卡在数据库上 |
| 队列 | [`MyDeadJobsIncreasing`](#mydeadjobsincreasing) | `delta(my_queue_dead[1h])` | 每小时超过 10 个，持续 15 分钟 | ticket | 任务中的缺陷、适配器配置错误（邮件、blob、账单）、远程服务拒绝请求 |
| 队列 | [`MyMetricsSnapshotStale`](#mymetricssnapshotstale) | `my_metrics_snapshot_age_seconds` | 队列部分超过 5 分钟、总量部分超过 1 小时，持续 15 分钟 | ticket | 无法访问 KV 存储、快照的数据库查询失败或超时 |
| 联邦 | [`MyDeliveryFailuresHigh`](#mydeliveryfailureshigh) | `my_outcome` 为 `retry` 或 `failed` 的 `my_delivery_attempts_total` | 超过尝试次数的 50%，持续 30 分钟 | ticket | 许多关注者所在的服务器宕机、某个大型服务器宕机、出站网络故障 |
| 联邦 | [`MyDeliveryAllFailing`](#mydeliveryallfailing) | 同上 | 超过尝试次数的 95%，持续 15 分钟 | page | 出站网络或 DNS 故障、出口流量被阻止、服务器的密钥或 `host` 错误 |
| 联邦 | [`MyRemoteServersPaused`](#myremoteserverspaused) | `my_federation_instances{my_federation_instance_state="unavailable"}` | 比一天前多 5 个 | ticket | 服务器已关停或屏蔽了本服务器、本方的网络问题持续了数天 |
| 联邦 | [`MyInboxRejectionsHigh`](#myinboxrejectionshigh) | `my_inbox_activities_total{my_outcome="rejected"}` | 超过 POST 的 25%，持续 30 分钟 | ticket | 签名验证失败（时钟偏差、密钥获取失败）、垃圾信息发送者或已停用的域名在重试、速率限制 |
| 联邦 | [`MyInboxSignatureFailures`](#myinboxsignaturefailures) | `my_inbox_activities_total{my_inbox_rejection="signature"}` | 超过 POST 的 10%，持续 30 分钟 | ticket | 服务器的时钟不准、获取发送方的密钥失败、某个服务器更换了密钥 |
| 联邦 | [`MyInboxErrors`](#myinboxerrors) | `my_inbox_activities_total{my_outcome="error"}` | 超过 POST 的 5%，持续 15 分钟 | ticket | 数据库故障、收件箱中的缺陷 |
| 错误 | [`MyErrorReportsIncreasing`](#myerrorreportsincreasing) | `my_errors_reported_total` | 15 分钟内超过 10 个且为前一天平均值的 3 倍，持续 15 分钟 | ticket | 新版本引入的缺陷、依赖以新的方式失败、配置损坏 |
| HTTP | [`MyHTTPErrorsHigh`](#myhttperrorshigh) | `http_server_request_duration_seconds_count` 中的 5xx | 超过 5%，持续 10 分钟 | page | 数据库或 KV 宕机、有问题的版本、维护模式 |
| HTTP | [`MyHTTPErrorsElevated`](#myhttperrorselevated) | 同上 | 超过 1%，持续 30 分钟 | ticket | 某个路由失败、某个适配器时不时失败 |
| HTTP | [`MyAPILatencyHigh`](#myapilatencyhigh) | 客户端 API 的 p95（不含流式传输） | 超过 300 ms，持续 30 分钟 | ticket | 数据库饱和或距离太远、慢查询、CPU 不足、冷启动 |
| 资源 | [`MyTargetDown`](#mytargetdown) | `up{job="my"}` | 为 0，持续 5 分钟 | page | 进程宕机、反复崩溃重启或处于维护模式；抓取路径有问题 |
| 资源 | [`MyNotReady`](#mynotready) | `my_readiness_failures_total` | 有任何增长，持续 5 分钟 | page | 无法访问数据库，或尚未应用 schema |
| 资源 | [`MyProcessRestarting`](#myprocessrestarting) | `resets(process_uptime_seconds[1h])` | 一小时内重启 3 次 | ticket | 因内存不足被终止（OOM）、panic、存活探针失败 |
| 资源 | [`MyMemoryNearLimit`](#mymemorynearlimit) | `go_memory_used_bytes` / `go_memory_limit_bytes` | 超过 90%，持续 15 分钟 | ticket | `GOMEMLIMIT` 对当前负载而言太低、打开的流过多、内存泄漏 |
| 资源 | [`MyDBPoolSaturated`](#mydbpoolsaturated) | `db_client_connection_count{db_client_connection_state="used"}` / `db_client_connection_max` | 超过 90%，持续 15 分钟 | ticket | 连接池对流量而言太小、慢查询占用连接 |
| 资源 | [`MyDBPoolWaiting`](#mydbpoolwaiting) | `db_client_connection_wait_time_seconds_total`、`db_client_connection_timeouts_total` | 每秒等待超过 0.5 秒，或出现任何超时，持续 10 分钟 | ticket | 同上；达到了数据库自身的连接数上限 |
| 资源 | [`MyDatabaseStorageLarge`](#mydatabasestoragelarge) | `my_db_size_bytes` | 超过你的套餐上限减去余量（示例为 8 GiB），持续 1 小时 | ticket | 帖子和缓存的远程内容增长；需要升级套餐或缩短保留期限 |
| 资源 | [`MyFileDescriptorsNearLimit`](#myfiledescriptorsnearlimit) | `process_unix_file_descriptor_count` / `my_process_file_descriptor_limit` | 超过 80%，持续 15 分钟 | ticket | 打开的流和连接过多、`ulimit -n` 过低 |
| 资源 | [`MyAdapterErrors`](#myadaptererrors) | `my_adapter_operation_duration_seconds_count{my_outcome="error"}` | 超过某个适配器操作的 5%，持续 15 分钟 | ticket | Valkey、blob 目录或流后端出现故障 |
| 资源 | [`MyRemoteMediaCacheOverLimit`](#myremotemediacacheoverlimit) | `my_media_stored_bytes{my_media_origin="remote_cache"}` | 超过 `remote_media_cache_max_bytes` 的 1.2 倍，持续 3 小时 | ticket | 每小时的清理没有运行或运行失败 |
| 资源 | [`MyMediaStorageLarge`](#mymediastoragelarge) | `my_media_stored_bytes` | 超过你规划的大小（示例为 50 GiB），持续 1 小时 | ticket | 上传量增长；需要升级套餐或提高预算 |
| 邮件 | [`MyEmailFailing`](#myemailfailing) | 邮件任务中 `my_outcome="dead"` 的 `my_job_duration_seconds_count` | 一小时内出现任何一个 | ticket | SMTP 或 SES 设置错误、账号被停用或超出配额、地址被永久拒绝 |
| 邮件 | [`MyEmailRetrying`](#myemailretrying) | 同上，`my_outcome="retry"` | 超过运行次数的 50%，持续 15 分钟 | ticket | 无法访问邮件服务器或其拒绝登录、发送速率限制 |
| 费用 | [`MyBillingImportFailing`](#mybillingimportfailing) | `my_billing_import_failing` | 为 1，持续 6 小时 | ticket | 缺少权限、未设置账单导出、达到请求上限、服务商不可用 |
| 费用 | 超出预算 | – | – | – | 流量、存储或配置错误导致费用超出计划（[见下文](#cost-and-budget)） |

<!-- source: where-to-look 9ef1d37c4f05 -->
## 在哪里查看

以下工具会在下面的大多数运行手册条目中反复出现。

- **任务页面**（管理 → 任务，`/admin/jobs`，需要 `manage_jobs`）：按状态（排队中、运行中、重试中、失败）和类型统计的数量，以及等待最久的任务；其**任务**页面（`/admin/jobs/list`）列出某一状态的任务，每个任务的页面显示其最后一次错误和未成功的尝试。只有保存在数据库中的队列（`queue = db` 和 `cloudtasks`）才会列出待处理的任务；失败的任务和尝试则对所有队列都会列出。
- **`my job` 命令**，在拥有服务器配置的主机上运行（`queue = db` 和 `cloudtasks`）：`my job list` 按从新到旧的顺序列出失败的任务；`my job retry <id>...` 把它们放回队列；`my job delete <id>...` 丢弃它们。任务可以安全地重新运行。
- **远程服务器页面**（审核 → 远程服务器，`/moderation/instances`，需要 `manage_domains`）：投递失败的服务器，以及投递已暂停的服务器，并附有原因和时间。“恢复投递”会把服务器重新标记为可用。
- **日志**：标准错误上的 JSON 行。每个 HTTP 请求以一行 `msg` 为 `http request` 的日志结束，每次任务运行以一行 `msg` 为 `job` 的日志结束（成功为 `INFO`，重试为 `WARN`，永久失败的任务为 `ERROR`，并附带 `error`、`job_id` 和 `my.job.type`）。请求内的日志行携带其 `request_id`，客户端看到的是 `X-Request-Id`。
  - Docker Compose：`docker compose logs my | jq -c 'select(.msg == "job" and ."my.outcome" == "dead")'`
  - Cloud Logging：`jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"`
  - CloudWatch Logs Insights：``filter msg = "job" and `my.outcome` = "dead"``
- **错误报告**：`error report` 行和错误页面（管理 → 错误，`/admin/errors`，需要 `manage_errors`）（[错误报告](error-reporting.md)）。
- **追踪**，当 `traces_exporter = otlp` 时：日志行携带 `trace_id`；在你的追踪后端中打开它，即可查看慢请求的时间花在了哪里。
- **维护模式**（[维护](maintenance.md)）：对每个请求都返回维护页面，不访问任何适配器。在修复数据库或 KV 存储时使用它。

<!-- source: runbook e60e811fcff0 -->
## 运行手册

<!-- source: myqueuebacklog e625ad2bb49f -->
### MyQueueBacklog

任务（投递、通知、邮件、时间线分发）等待 worker 的时间超过 5 分钟。

检查：

- 任务页面：哪些类型在排队，是否有很多在重试。某一类型的突发（发布一条拥有大量关注者的帖子后的 `federation.deliver`、中继活动）会自行消化；确认等待时间在下降。
- worker 进程（`role = all` 或 `role = worker`）正在运行，并且其日志中有 `job` 行。
- 是否同时出现 `MyDBPoolSaturated` 或 `MyAPILatencyHigh`：worker 在等待数据库。

处理：

- 运行更多 worker 进程（`role = worker`），或为现有进程分配更多 CPU。
- 如果某种任务类型很慢，在 `job` 日志行（`duration_seconds`）中找到它，并查找该类型的错误。

<!-- source: myqueuestalled c92f00f06f95 -->
### MyQueueStalled

最早就绪的任务已等待超过 30 分钟：没有任何任务在处理，或几乎没有。

检查：

- 至少有一个进程以 `role = all` 或 `role = worker` 运行，并且没有反复崩溃重启（`MyProcessRestarting`）。
- worker 日志中是否有每个任务都重复出现的错误（数据库、KV、panic）。
- `MyNotReady`：无法访问数据库的 worker 无法领取任务。
- `max by (my_job_type) (my_queue_oldest_ready_age_seconds)`：如果某一类型在等待而其他类型没有，请确认有 worker 运行该类型（[按任务类型拆分 worker](running.md#splitting-workers-by-job-type)）。

处理：

- 启动或重启 worker。已停止的 worker 持有的任务会在租约到期后回到队列；它们可以安全地重新运行。
- 如果原因是数据库，请按照 [MyNotReady](#mynotready) 处理。

<!-- source: mydeadjobsincreasing b8e928621ac7 -->
### MyDeadJobsIncreasing

每小时有超过 10 个任务永久失败：它们用完了重试次数，或者任务拒绝再次运行。

检查：

- 任务页面，按“失败”筛选：类型和最后一次错误。任务的尝试记录显示了过程中的每个错误。
- `my.outcome` = `dead` 的 `job` 日志行中的相同错误。

处理：

- 配置错误（邮件、blob、账单导入、Webhook 目标）：修正后，在任务页面上重新运行任务（“再次运行”），或使用 `my job retry <id>...`。
- 发往已消失的服务器的投递在重试约 4 天后会永久失败；这是预期行为，无需处理。
- 无人重试的失败任务会在 `dead_job_retention`（默认 30 天）后删除。用 `my job delete` 删除你不打算重试的任务。
- 看起来像缺陷的错误：附上任务类型和错误进行报告（其中不含个人数据）。

<!-- source: mymetricssnapshotstale c74fe15e6934 -->
### MyMetricsSnapshotStale

队列和存储仪表来自进程间通过 KV 存储共享的一个快照。该快照已经过时，因此队列和存储告警看到的是旧值，可能不会触发。

检查：

- 日志中有关指标快照或 KV 存储的错误。
- `kv.valkey` 或 `kv.db` 的 `MyAdapterErrors`，以及数据库的健康状况。

处理：

- 修复 KV 存储或数据库连接。快照会在下一次采集时重新计算；周期性快照任务也会每 15 分钟运行一次。

<!-- source: mydeliveryfailureshigh 709985dfbf2b -->
### MyDeliveryFailuresHigh

发往其他服务器的投递尝试有一半以上失败。一定程度的失败是正常的：服务器会消失，而本服务器会持续重试约 4 天。

检查：

- 远程服务器页面：失败是集中在少数几个服务器上（它们宕机了；无需处理），还是分散在许多服务器上（怀疑是本服务器的问题）。
- 按 `error_type` 统计的 `http_client_request_duration_seconds_count`：超时、DNS 错误，或 `forbidden_destination`（SSRF 防护拒绝了该目标）。

处理：

- 失败集中在少数几个服务器上：无需处理。在 7 个不同的日子失败后，发往该服务器的投递会被暂停，并每天探测一次该服务器以便恢复投递。
- 处处失败：请按照 [MyDeliveryAllFailing](#mydeliveryallfailing) 处理。

<!-- source: mydeliveryallfailing c81fcf407cb9 -->
### MyDeliveryAllFailing

几乎所有投递都失败。这几乎总是本服务器这边的问题。

检查：

- 从服务器的主机或容器中确认出站 HTTPS 是否正常（DNS、出口防火墙、NAT、代理）。
- `federation.deliver` 的 `job` 日志行中的错误：TLS 错误、超时，或来自许多服务器的 `401`（它们无法验证本服务器的签名）。
- 更改域名或密钥之后：服务器一旦开始联邦，`host` 就必须保持不变，KEK 也必须是封装这些密钥时使用的那个（[KEK 管理](key-management.md)）。

处理：

- 修复出站网络。失败的尝试会以退避方式重试（1 分钟、5 分钟、30 分钟、2 小时，依此类推），因此投递会自行补上。
- 如果投递已用完重试次数，请在任务页面上重新运行失败的 `federation.deliver` 任务。
- 如果远程服务器在故障期间被暂停，它们会在下一次成功投递、收到签名请求或每日探测时恢复；你也可以在远程服务器页面上点击“恢复投递”。

<!-- source: myremoteserverspaused 17c3ec51fb53 -->
### MyRemoteServersPaused

过去一天内，有 5 个或更多服务器的投递被暂停。在 7 个不同的日子投递失败后，服务器会被暂停，因此每周有少量暂停是正常的：服务器会关停。一次性出现很多则意味着许多服务器同时失败了数天。`my_federation_instance_state` = `available` 的 `my_federation_instances` 统计的是仍在投递但曾在某天失败的服务器；该值的跳升会先于暂停出现。

该指标没有按服务器区分的标签（标签从不携带远程域名）。检查：

- 远程服务器页面：“已暂停”标签页列出了被暂停的服务器，以及暂停的时间和原因。看看它们是否有共同点（同一托管服务商、同一软件），还是分散各处。
- 过去一周的 `MyDeliveryFailuresHigh` 和 `MyDeliveryAllFailing`：本方持续数天的网络问题会导致许多服务器被暂停。

处理：

- 已消失的服务器：无需处理。被暂停的服务器每天都会被探测，一旦响应就会自行恢复。
- 本方出现问题之后：修复问题，然后为你关心的服务器点击“恢复投递”，或等待每日探测。
- 对方运营者表示屏蔽了本服务器：本方无能为力。要有意停止与某个服务器的联邦，请使用域名措施（审核 → 域名措施）。

<!-- source: myinboxrejectionshigh ee5a244ccf12 -->
### MyInboxRejectionsHigh

其他服务器 POST 到收件箱的活动中，有超过四分之一被拒绝。

检查：

- 按 `my_inbox_rejection` 统计的 `my_inbox_activities_total{my_outcome="rejected"}`：`signature`（按照 [MyInboxSignatureFailures](#myinboxsignaturefailures) 处理）、`suspended`（已停用的域名在不断重试）、`federation_off`（联邦已关闭）、`rate_limited`（某个客户端 IP 或服务器超出速率限制），或 `invalid`（活动缺少有效的 `id`、`type`、`actor` 或 `object`）。
- 收件箱路由（`my.surface` = `activitypub`）的 `http request` 日志行按 `http.response.status_code`（`401`、`403`、`429`、`400`）给出了相同的划分，并带有客户端地址，可用于了解请求来自哪里。

处理：

- 来自已停用域名的拒绝，或联邦关闭时的拒绝，无需处理：这是有意为之的拒绝。
- 来自某个服务器的被限速请求：垃圾信息发送者或突发流量；如果持续，请停用该域名。
- 来自某个服务器的无效活动：该服务器的软件发送了本服务器无法读取的内容；请附上该服务器的软件及版本进行报告。

<!-- source: myinboxsignaturefailures 85aa65f3247c -->
### MyInboxSignatureFailures

其他服务器 POST 到收件箱的活动中，有超过 10% 因签名或完整性证明缺失或无法验证，或者因无法获取发送方的密钥而被拒绝。

检查：

- 服务器的时钟：超出时间窗口的签名会被拒绝。请检查主机或容器运行时的 NTP。
- 获取发送方的密钥：`MyDeliveryAllFailing`，以及按 `error_type` 统计的 `http_client_request_duration_seconds_count{my_http_purpose="json"}`（出站网络、DNS、SSRF 防护）。
- 拒绝来自许多服务器还是一个服务器（状态为 `401` 的 `http request` 行）：更换了密钥的服务器，或者以本服务器不接受的方式签名的服务器，只影响它自己。

处理：

- 修复时钟或出站网络。其他服务器会重试它们的投递，因此只要在一天左右修复，就不会丢失任何内容。
- 某个服务器失败：向该服务器的运营者报告，或附上该服务器的软件及版本向本项目报告。

<!-- source: myinboxerrors e3cf874c925e -->
### MyInboxErrors

收件箱对超过 5% 的活动返回 `500`。发送方会重试，但活动会延迟。

**检查**收件箱路由的 `http request` 和错误日志行中的错误，以及数据库的健康状况（`MyNotReady`、`MyDBPoolWaiting`）。

**处理**：修复数据库，或把错误作为缺陷报告。无需重放任何内容：发送方会重试。

<!-- source: myerrorreportsincreasing deed4980cc8c -->
### MyErrorReportsIncreasing

服务器报告错误（panic、5xx 背后的错误、失败的任务）的速率超过平时的三倍。

检查：

- 过去一小时的 `error report` 行，按 `my.error.fingerprint` 分组：最常见的错误、它们的 `my.error.where`，以及 `message` 的首行。在 Cloud Run 上，Google Cloud Error Reporting 会显示相同的分组。
- 是否在同一时间发布了新版本、更改了配置，或某个依赖开始失败（`MyNotReady`、`MyAdapterErrors`）。
- 具有相同 `request_id` 或 `job_id` 的请求或任务日志行。

处理：

- 依赖失败或配置错误：修复它；之后可以在任务页面上重新运行失败的任务。
- 有问题的版本：回滚到上一个镜像。
- 缺陷：附上该行的首行和堆栈进行报告。两者都已清除个人数据。

<!-- source: myhttperrorshigh dd55e9ba0dfd -->
### MyHTTPErrorsHigh

所有请求中有超过 5% 返回 5xx。用户会看到错误。

检查：

- 是否处于维护模式（预期会返回 `503`；下次请静默该告警）。
- `MyNotReady`（数据库）、`MyAdapterErrors`（KV、blob、流），以及最近是否发布了新版本。
- 按 `http.route` 统计的 `http.response.status_code` ≥ 500 的 `http request` 行，以及具有相同 `request_id` 的错误行。

处理：

- 修复出故障的依赖。如果需要处理数据库，请切换到维护模式，让用户看到明确的页面而不是错误。
- 有问题的版本：回滚到上一个镜像。
- 如果持续时间较长，请告知用户（[向用户通报故障](#telling-users-about-an-incident)）。

<!-- source: myhttperrorselevated 34d53d12ea4d -->
### MyHTTPErrorsElevated

在半小时内，有超过 1% 的请求返回 5xx。通常是某一个路由或某一个适配器的问题。

**检查**按 `http.route` 统计的返回 5xx 的 `http request` 行，以及这些请求的错误行。**处理**：修复依赖，或把错误作为缺陷报告。

<!-- source: myapilatencyhigh 81b66aee9099 -->
### MyAPILatencyHigh

客户端 API 的第 95 百分位超过 300 ms（这是缓存预热后包含 20 条帖子的主页时间线页面的目标值）。不含流式传输端点。

检查：

- 按 `http_route` 统计的 `http_server_request_duration_seconds`：是某一个路由慢，还是所有路由都慢。
- 数据库：`MyDBPoolWaiting`、数据库的 CPU 和慢查询日志、服务器与数据库之间的网络距离。
- 进程：CPU 限流、`MyMemoryNearLimit`（内存压力下的 GC）、`go_schedule_duration_seconds`（等待 CPU 的 goroutine）。
- 如果已启用，查看慢请求的追踪。

处理：

- 为数据库或服务器分配更多资源，或让它们离得更近。
- 在流量很少的服务器上，缩容到零后的冷启动会拉高百分位；这是预期行为。

<!-- source: mytargetdown e0308f141452 -->
### MyTargetDown

Prometheus 无法抓取某个进程。

检查：

- 进程是否在运行：`docker compose ps`、`kubectl get pods`，或平台的控制台。它的最后几行日志会说明停止的原因。
- 是否处于维护模式（不提供指标；属于预期）。
- 是否设置了 `metrics_exporter = prometheus`，以及 Prometheus 能否访问 `metrics_listen`。

处理：

- 重新启动进程。如果无法启动，日志会说明原因：配置错误（`my config check` 会显示）、无法访问数据库、未应用 schema。

<!-- source: mynotready 1092418fbba2 -->
### MyNotReady

就绪检查失败：无法访问数据库，或其表与此版本不匹配。负载均衡器会停止向该进程发送流量，外部健康检查端点会报告 `unavailable`。

检查：

- 就绪检查的日志行：连接错误（网络、凭据、数据库宕机或连接数耗尽），或缺失的表。
- 数据库服务商的状态页面和控制台：数据库是否在运行、存储是否已满、是否在维护中？
- 升级之后：是否在新版本启动之前应用了 schema（[初始化数据库](database.md)）？

处理：

- 数据库宕机或无法访问：恢复它，或进行故障转移。在你处理期间，维护模式会向用户显示明确的页面。
- 数据库存储已满：升级套餐或释放空间（[MyDatabaseStorageLarge](#mydatabasestoragelarge)）。
- 未应用 schema：运行同一版本的 `my schema apply`，之后进程会自行变为就绪。
- 数据丢失或损坏：从备份恢复，并保留封装备份中密钥时使用的 KEK（[KEK 管理](key-management.md)）。

<!-- source: myprocessrestarting d96b041c2ff0 -->
### MyProcessRestarting

某个进程在一小时内重启了 3 次。

检查：

- 平台给出的重启原因：`OOMKilled`（内存）、存活探针失败，或退出码。
- 每次重启前的最后几行日志，查看是否有 panic 或致命的配置错误。

处理：

- 内存不足：把 `GOMEMLIMIT` 设为容器内存上限的 90% 左右，或为容器分配更多内存（[MyMemoryNearLimit](#mymemorynearlimit)）。
- panic：附上其前后的日志行进行报告。
- 日志显示数据库属于另一个 `host` 或 `account_domain`：在配置中设置已记录的值，或让 `database_url` 指向正确的数据库。这些值无法更改（[之后无法更改的设置](production-checklist.md#settings-you-cannot-change-later)）。

<!-- source: mymemorynearlimit 438190b311c9 -->
### MyMemoryNearLimit

Go 运行时使用了超过 90% 的 `GOMEMLIMIT`。接近上限时，垃圾回收器的运行频率会大幅增加，消耗 CPU 并增加延迟；超过上限后，平台可能会终止进程。此告警需要设置 `GOMEMLIMIT`；未设置时，请改为关注容器的内存指标。

**检查** `http_server_active_requests` 和打开的流（`my_stream_connections`），以及在负载平稳时内存是否持续增长（内存泄漏）。

**处理**：提高容器的内存，并相应提高 `GOMEMLIMIT`，或运行更多进程。如果内存稳定增长且与负载无关，请报告。

<!-- source: mydbpoolsaturated a737e35a86f5 -->
### MyDBPoolSaturated

某个进程的数据库连接有超过 90% 正在使用。请求很快就会开始等待连接（`MyDBPoolWaiting`）。

**检查**是流量增长了，还是查询变慢了（数据库的慢查询日志、`my_adapter_operation_duration_seconds`）。

处理：

- 在 `database_url` 中提高连接池大小（PostgreSQL 为 `pool_max_conns`），并让所有进程的总数保持在数据库的连接数上限以下。
- 如果许多进程共享数据库，请在数据库前面放置连接池代理。

<!-- source: mydbpoolwaiting eacb1d75cd54 -->
### MyDBPoolWaiting

工作在等待空闲的数据库连接，或在 [`database_acquire_timeout`](../reference/configuration.md#database_acquire_timeout) 之后放弃等待（超时仅适用于 PostgreSQL）：请求返回 `503`，任务稍后再次运行。请按照 [MyDBPoolSaturated](#mydbpoolsaturated) 处理；同时检查数据库本身是否已达到其连接数上限。

<!-- source: mydatabasestoragelarge edf767115918 -->
### MyDatabaseStorageLarge

数据库占用的空间超出了你的规划。`my_db_size_bytes` 是 PostgreSQL 数据库的大小（`pg_database_size`，即服务商的存储上限所统计的大小），或 SQLite 文件的大小（不含其预写日志）。这是增长，而不是故障。请在规则中把阈值设为比你的数据库套餐存储上限低一定余量的值；对于 SQLite，则是比存放 SQLite 文件的卷的上限低一定余量的值，该卷还必须为预写日志和备份留出空间。

处理：

- 升级套餐或扩大卷。
- 远程帖子会在实例设置 `remote_status_retention_days` 之后被删除；调低该值可以减缓增长。
- 删除的行会释放空间，两种数据库都会重复利用这些空间，但大小很少会下降：PostgreSQL 会保留其表的空间，SQLite 会把空闲页保留在文件中，直到执行 `VACUUM`（在服务器停止时用 `sqlite3` 运行）。

<!-- source: myfiledescriptorsnearlimit 8a3112c11968 -->
### MyFileDescriptorsNearLimit

进程打开的文件描述符超过其上限的 80%。超过上限后，新连接会失败。

**处理**：提高上限（`ulimit -n`、容器运行时的 `nofile`），或运行更多进程。查看 `my_stream_connections` 和 `http_client_open_connections`，了解是什么占用了它们。

<!-- source: myadaptererrors fcf1d91a0f01 -->
### MyAdapterErrors

某个适配器的操作有超过 5% 失败：`kv.valkey`（Valkey）、`kv.db`（KV 表）、`blob.fs`（媒体目录）、`queue.db`（队列表），或某个 `stream.<backend>`（进程间的通知）。

**检查**该适配器的错误行，以及其背后的服务：Valkey 是否在运行且可以访问，媒体卷是否已挂载、可写且未满。

**处理**：修复该服务。失败的任务操作会被重试；失败的流通知只会延迟实时更新。

<!-- source: myremotemediacacheoverlimit dea5d39a7027 -->
### MyRemoteMediaCacheOverLimit

远程媒体的副本持续超过 `remote_media_cache_max_bytes`。每小时的清理通常会删除过期的和最近最少使用的副本。请在规则中把阈值设为你的 `remote_media_cache_max_bytes` 值。

**检查**任务页面和日志中清理任务的失败，以及 blob 存储的 `MyAdapterErrors`。

**处理**：修复失败的原因，或调低缓存大小，或调整实例的远程媒体策略。

<!-- source: mymediastoragelarge ff5e5906636c -->
### MyMediaStorageLarge

媒体文件占用的空间超出了你的规划。这是增长，而不是故障。请在规则中把阈值设为你的存储套餐或预算所允许的值。

**处理**：升级套餐或提高预算，或调低上传限制（`media_max_image_bytes`、`media_max_av_bytes`）。

<!-- source: myemailfailing f7a0c64c5c60 -->
### MyEmailFailing

过去一小时内有邮件任务永久失败：验证、密码重置、密码通知、通知或关停邮件。正在等待验证邮件或重置邮件的用户无法完成操作。

**检查**任务页面（按邮件任务类型筛选，它们以 `_email` 结尾）以及最后一次错误。[发送邮件](email.md#look-at-send-failures)列出了这些错误及其含义。

处理：

- SMTP 主机、端口或凭据错误，或者 SES 账号或权限有问题：修正后重新运行失败的任务。超过链接有效期的验证邮件和重置邮件在重试时会不发送而直接完成，因此请让这些用户重新申请。
- 被永久拒绝的地址会被抑制；无需处理。

<!-- source: myemailretrying 40d3ed1a76f5 -->
### MyEmailRetrying

大多数邮件发送失败并等待重试：无法访问邮件服务器、邮件服务器拒绝登录，或限制了发送速率。事务性邮件会重试约 15 分钟，通知会重试约 4 小时，之后便永久失败。请在此之前按照 [MyEmailFailing](#myemailfailing) 处理。

<!-- source: cost-and-budget 5a63bdd2754d -->
### 费用和预算

服务器不会就费用发出告警。请在服务商处设置带有告警的预算，因为服务商最先看到费用：

- AWS：AWS Budgets 费用预算，例如在月度金额的 80% 和 100% 时告警，并按本服务器的成本分配标签筛选。
- Google Cloud：项目的 Cloud Billing 预算，告警阈值设为 50%、90% 和 100%。同一预算的通知可以用于费用导入（[导入云使用费用](billing-import.md)）。

导入的费用和本月至今的估算值显示在“管理 → 费用 → 账单导入”中。失败的导入会记录在那里，并由下一次每日导入重试；[`MyBillingImportFailing`](#mybillingimportfailing) 会在当月数据过时之前通知你。

<!-- source: mybillingimportfailing da997f400ac4 -->
### MyBillingImportFailing

某个账单来源的使用费用最近一次导入失败，并且 6 小时内没有后续导入成功。导入每天运行一次；限流和不可用会在数小时内重试，其他失败则等待第二天的导入，因此在此之前费用页面显示的是较旧的数据。

检查：

- “管理 → 费用 → 账单导入”：该来源的最后一次导入及其错误。
- 按 `error_type` 统计的 `my_billing_imports_total{my_outcome="failed"}`，或 `msg` 为 `billing import failed` 的 `WARN` 日志行，其中带有 `source` 和 `error_kind`。[导入云使用费用](billing-import.md)列出了各种类型及其修复方法。

处理：

- 修复原因（权限、账单导出、请求上限），然后在“管理 → 费用 → 账单导入”中点击“立即导入”，而不必等到第二天。

如果没有 Prometheus（Cloud Run 上使用 `metrics_exporter = none`），请改为针对 `billing import failed` 行创建基于日志的告警。

<!-- source: telling-users-about-an-incident e8cb0ca13fe8 -->
## 向用户通报故障

1. **确认**告警，让其他值班人员知道已经有人在处理。
2. 一旦用户受到影响，就**在状态页面上发布**（`status_page_url`，参见[从外部监控服务器](monitoring.md#a-status-page)）：什么出了问题、从什么时候开始，以及你下一次更新的时间。即使本服务器无法访问，状态页面仍然可以访问，Web 客户端也会链接到它。
3. 如果用户仍能访问服务器（性能下降而非宕机），请**发布公告**：在“管理 → 公告”中撰写文字并发布。公告会显示在所有人的主页时间线上方。如果服务器必须停止，请改用带有 `maintenance_message` 的维护模式。
4. 即使没有任何变化，也要在你承诺的时间**更新**两者。
5. **结束**：在状态页面上把故障标记为已解决，撤回公告（或发布一条简短的后续说明），并趁记忆犹新记下发生了什么以及你做了哪些更改。
