<!-- source: error-reporting 1ccf0c1c98a4 -->
# 错误报告

服务器如何告诉你它自身的缺陷和故障、在各个平台上到哪里查看报告，以及如何把报告发送到错误跟踪服务。指标告诉你出了问题；错误报告则告诉你是什么失败了，并附带调用栈。[配置参考](../reference/configuration.md)列出了所有设置项。

无论你如何配置，每份报告都会经过以下三步：

1. 向标准错误输出一行 `error report`，附带调用栈（[日志行](#the-log-line)）。
2. 该报告会在数据库中计入一个错误分组。管理界面会显示这些分组，拥有 `manage_errors` 权限的人会收到新出现和再次出现的分组的通知（[错误分组与通知](#error-groups-and-notifications)）。
3. 如果你设置了 `errors_exporter`，报告还会发送到外部服务（[把报告发送到服务](#sending-reports-to-a-service)）。

除非你设置了 `errors_exporter`，否则任何内容都不会离开服务器。

<!-- source: what-is-reported 8e5ac9d14c36 -->
## 报告哪些内容

| 会报告 | 不会报告（由指标和其他日志行覆盖） |
| --- | --- |
| HTTP 处理程序、任务或后台任务中的 panic | 4xx 响应（错误的输入、授权、速率限制）和维护模式 |
| 导致客户端 API、管理 API、登录页面、ActivityPub、OAuth 或媒体返回 5xx 响应的错误 | 客户端已断开的请求 |
| 最终失败（`dead`）的任务 | 任务的重试。重试用完时才会报告 |
| 启动失败：配置无效，或数据库或适配器无法初始化 | 就绪检查失败 |
| Web 客户端未处理、由浏览器发送到服务器的错误 | 由其他服务器引起的失败：投递的重试和放弃、被拒绝的签名、失败的远程获取 |
| | 浏览器自身的噪声：网络中断、浏览器扩展引起的错误，以及因新部署而被移除的代码块（chunk） |

外部服务（邮件、存储、支付、语音）的故障，只有在导致任务最终失败或请求返回 5xx 时才会报告。临时故障则体现在 `my.adapter.*` 指标中（[运行服务器](running.md#metrics)）。

后台任务中的 panic 在报告后会停止进程，因此服务器永远不会在损坏的状态下继续运行。你的平台会重启它。

<!-- source: the-log-line 6b570d0bfd09 -->
## 日志行

这一行的 `msg` 为 `error report`，`level` 为 `ERROR`（当故障导致进程停止时，`severity` 为 `CRITICAL`）。它的字段以及清洗对这些字段的处理，见[运行服务器](running.md#error-reports)。简而言之：

- 同一错误的每次出现，`my.error.fingerprint` 都相同。可以按它搜索或分组。
- `my.error.where` 表示错误发生的位置，例如 `client_api GET /api/v1/statuses/{id}`。
- 通过 `request_id`、`job_id`、`trace_id` 和 `span_id` 可以找到同一请求或任务的其他日志行。
- `message` 位于行末：错误类型和消息、一个换行符，以及 Go 调用栈。

其他任何日志行都不带调用栈。

<!-- source: google-cloud-cloud-run 925c99683713 -->
### Google Cloud（Cloud Run）

Cloud Error Reporting 无需任何设置即可读取这一行，并在控制台的 Error Reporting 下列出错误。它是免费的；你只需为 Cloud Logging 付费。

- 它按错误类型和调用栈顶部的几帧对错误分组。它无法使用服务器的指纹，因此 Error Reporting 中的分组可能与管理界面中的分组不同。
- 它保留分组 30 天，并在出现新错误以及已解决的错误再次发生时通知你，整个项目每小时最多 20 条通知。请在 Google Cloud 控制台中设置它的通知渠道。
- 要查看报告前后发生了什么，在 Cloud Logging 中搜索 `jsonPayload."request_id"="<id>"`，或以同样方式搜索 `trace_id`。

<!-- source: aws 6ca6594afd7e -->
### AWS

CloudWatch Logs 会把这一行当作普通 JSON 读取。

- 为日志组开启日志异常检测。它会标记从未见过的日志行模式，并可以通过 SNS 发出告警。它只读取一行的前 1,500 个字符，这就是 `message` 放在最后一个字段的原因。它不保留已解决的状态。
- 要统计报告数量，可以创建一个指标筛选器，例如 `{ $.msg = "error report" }`。不要使用取自 `my.error.fingerprint` 的维度：维度的每个取值都会作为一个单独的指标计费。`my.error.source` 和 `my.error.kind` 是安全的。
- 用 Logs Insights 查找上下文：``filter `request_id` = "<id>"``。

<!-- source: self-hosted 90de7a419474 -->
### 自托管

从容器日志中读取这一行：`docker compose logs my | jq -c 'select(.msg == "error report")'`。使用 `metrics_exporter = prometheus` 时，告警 `MyErrorReportsIncreasing`（[告警与运维手册](alerts.md#myerrorreportsincreasing)）会在报告急剧增加时发出警告；其背后的计数器是 `my.errors.reported`。如果需要不依赖外部服务的通知，请使用管理界面和下文的 webhook。如果需要跟踪服务，请使用 [`errors_exporter`](#sending-reports-to-a-service)。

<!-- source: error-groups-and-notifications 8f740a328309 -->
## 错误分组与通知

服务器在数据库中按指纹统计报告。它不需要任何设置，也不需要额外的服务，写入量也很小：某个指纹的第一份报告会立即写入，后续的报告在内存中累加，每分钟写入一次。强制终止进程可能会丢失最后一分钟的计数；日志行不会因此丢失。超过 90 天的计数每天删除。如果出故障的正是数据库，报告就只保留在日志行中。

<!-- source: the-administration-screen 09092782a7c1 -->
### 管理界面

角色拥有 `manage_errors` 权限的人（内置的 `admin` 角色拥有该权限）会在管理菜单中看到“错误”（`/admin/errors`）。

- 列表显示各个分组，默认显示未处理的分组。可以按状态（未处理、已解决、已静默、所有状态）和来源（服务器、Web 客户端）筛选。每一行显示错误类型、发生位置、消息的前几行、最近 24 小时和 7 天的次数，以及首次和最近一次出现的时间和版本。
- 分组页面显示最近一次出现的详情（时间、版本、进程角色、请求 ID、跟踪 ID、类型、消息和调用栈）、最近 90 天按小时统计的出现次数，以及其状态变更的历史。请求 ID 和跟踪 ID 旁有“复制”按钮：把它们粘贴到你的日志搜索中。
- “标记为已解决”用于你已修复的错误。如果它在服务器的另一个版本上再次发生，该分组会重新打开并再次通知。在被标记为已解决的那个版本上发生的同一错误只会被计数，因此旧版本的最后一些请求不会让它重新打开。
- “静默”会继续计数，但不发送通知或 webhook 事件。“取消静默”会重新打开该分组。

同样的操作也可以通过 `GET /api/v1/admin/error_groups` 和 `POST /api/v1/admin/error_groups/{id}/resolve`（`/mute`、`/unmute`）完成。

<!-- source: notifications-and-webhooks 99836c7f02ec -->
### 通知与 webhook

- 拥有 `manage_errors` 权限的人会在出现新错误分组和错误再次出现时收到通知 `admin_error_group`。整个服务器每小时最多收到 10 条；该小时内其余的会合并成一条通知，说明又报告了更多错误。他们也可以通过邮件接收（[审核](moderation.md#notifications-for-moderators-and-the-pending-counts)）。
- webhook 事件 `error_group.created` 和 `error_group.regressed` 携带同样的分组，因此你可以把它们发送到 Slack、Discord 或自动化工具（[服务器 webhook](webhooks.md)、[webhook 参考](../reference/webhooks.md)）。webhook 需要 `manage_webhooks` 权限，这些事件需要 `manage_errors` 权限。
- 已静默的分组从不发送通知。

<!-- source: errors-from-the-web-client e5b2f45b62be -->
### 来自 Web 客户端的错误

Web 客户端会把其页面未处理的错误发送到你自己服务器上的 `POST /api/v1/client_errors`，因此任何第三方都看不到它们，内容安全相关的设置也无需更改。服务器对所有请求都回答 `204`，并丢弃它不接受的内容：超过 16 KiB 的正文、不包含你自己 Web 客户端文件的调用栈，以及同一 IP 地址每分钟超过 10 份的报告。同一错误每次页面浏览只发送一次，每次页面浏览最多发送 20 个。没有登录会话时发送的报告会被计数和记录日志，但它本身不会通知任何人，因此匿名脚本无法塞满你的收件箱。当已登录用户的浏览器也报告该错误时，分组才会发送通知。调用栈来自压缩后的构建产物，不会映射回源代码，因此请先看路由和消息。版本（release）是提供该页面的服务器的版本。离线打开的页面，或在 3 秒内没有得到响应后打开的页面，来自浏览器保存的副本，其中记录的是浏览器保存 Web 客户端时的版本：一个 Web 客户端相同的旧版本。

<!-- source: sending-reports-to-a-service eb5f208ca8fc -->
## 把报告发送到服务

设置 `errors_exporter`，即可把每份报告同时发送到错误跟踪服务。日志行和数据库中的分组仍会保留。每份报告只发送一次，从不重试，也没有任何操作会等待该服务。

| 取值 | 发送到 |
| --- | --- |
| `none`（默认） | 不发送 |
| `sentry` | 位于 `errors_sentry_dsn` 的任何兼容 Sentry 的服务 |
| `otlp` | OpenTelemetry 采集器或后端，作为日志记录发送 |

`errors_environment`（默认 `production`）用于在每份报告上标明部署环境，例如 `staging`。

两种导出器发送的都是与日志行相同的、已清洗的字段，附带指纹以及跟踪 ID 和 span ID，不包含任何关于主机的信息：没有主机名、IP 地址、用户、请求正文、请求头或 cookie。

服务器会限制发送量，以免该服务被大量报告淹没：

- 同一指纹每个进程每分钟最多发送一次，所有报告每分钟最多 60 份。其余的只保留在日志和分组中。
- 最多 1,000 份报告排队等待发送。遇到队列已满的报告会被丢弃。服务器停止时，最多等待 2 秒让队列清空。
- 当服务回答 `429` 或要求降速（`Retry-After`、`X-Sentry-Rate-Limits`）时，服务器会在相应时长内停止发送。
- 服务器从不跟随服务返回的重定向，以免 DSN 密钥和报告被发送到别处：重定向计为 `failed`。请使用服务显示的 DSN，它直接指向服务的接收端。

关注计数器 `my.errors.exported`：`my.outcome` 为 `success`、`dropped`（超出限制、队列已满，或服务要求降速）或 `failed`（无法连接服务，或服务拒绝了报告）（[运行服务器](running.md#metrics)）。

<!-- source: sentry-glitchtip-and-datadog 12c935ec3c05 -->
### Sentry、GlitchTip 和 Datadog

```sh
MY_ERRORS_EXPORTER=sentry
MY_ERRORS_SENTRY_DSN_FILE=/run/secrets/sentry_dsn   # or MY_ERRORS_SENTRY_DSN; the DSN is a secret
```

服务器自己实现了 Sentry 协议，因此不涉及 Sentry SDK，也不会读取任何 `SENTRY_*` 环境变量。它会设置指纹，让服务按指纹分组，并设置标签 `service`（`my`）、`my.role`、`my.error.where` 和 `error.fingerprint`。版本（release）是服务器的版本，环境是 `errors_environment`。

- **Sentry**（托管或自托管）：从项目的客户端密钥中复制 DSN。
- **GlitchTip**：复制项目的 DSN。它是开源的，可以在小型服务器上运行。请确认你所用的版本为新出现和再次出现的问题提供了哪些告警规则。
- **Datadog Error Tracking**：使用你的 Datadog 站点的 Sentry DSN，`https://<DD_API_KEY>@sentry-intake.<DD_SITE>/1`。Datadog 要求有 `service` 标签，服务器始终会设置它。Datadog 把这些事件存储为日志，并在 Error Tracking 中分组。如果你改为用代理或转发器把日志行发送到 Datadog（Error Tracking for Logs），调用栈位于 `message` 中；请用日志管道把它映射到 `error.stack`，否则 Datadog 不会把它识别为调用栈。`sentry` 导出器不需要这样的映射。

<!-- source: otlp-backends 778540dabdc8 -->
### OTLP 后端

```sh
MY_ERRORS_EXPORTER=otlp
OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318   # or OTEL_EXPORTER_OTLP_LOGS_ENDPOINT
OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer%20...
```

每份报告都是一条名为 `exception` 的 OpenTelemetry 日志记录，严重级别为 `ERROR`（导致进程停止时为 `FATAL`），带有 `exception.type`、`exception.message`、`exception.stacktrace`、`my.error.fingerprint`、`my.error.where`、`my.error.source`、`my.error.kind`，以及跟踪 ID 和 span ID。它通过 OTLP/HTTP（protobuf、gzip）发送，使用与指标和跟踪相同的资源。

后端是否会把这些记录变成分组的错误取决于后端：有些后端只根据跟踪数据构建错误视图，而 Datadog 需要 `error.stack`。对于 Sentry、GlitchTip 和 Datadog，请使用 `sentry`。如果你希望这些记录与其他日志放在一起，请使用 `otlp`。构建标签 `no_otel` 会去掉 `otlp`（[构建标签](../reference/build-tags.md)）。

<!-- source: what-scrubbing-does-and-does-not-guarantee ccf1b4716bdd -->
## 清洗能保证什么、不能保证什么

清洗只在创建报告时运行一次，因此日志行、数据库和每个导出器收到的都只是清洗后的文本。

- 在消息中，URL、邮件地址、`@user@host` 形式的账号标识、IP 地址、UUID 和 ULID，以及 10 位或更多位的数字，会分别替换为 `<url>`、`<email>`、`<acct>`、`<ip>`、`<id>` 和 `<n>`，消息会被截断为 1 KiB。
- 在调用栈中，函数参数的值会替换为 `(...)`，调用栈会被截断为 16 KiB。文件位置会保留，且相对于源代码树，因此不会出现构建机器上的路径。
- 任何报告都不包含请求正文、请求头、查询参数、cookie、账号 ID、用户名、IP 地址或 User-Agent。

清洗基于模式匹配，因此不能保证消息中不含个人数据。错误消息中引用的名字、自由文本或少于 10 位的数字可能会保留下来。普通的 `http request` 和 `job` 日志行中的 `error` 字段不会经过这样的清洗。请把日志和错误跟踪服务视为可能包含用户内容片段的地方，并为它们设置与其他日志相同的访问控制和保留期限。一旦设置了 `errors_exporter`，该服务的存储位置、保留期限和访问规则就会适用于这些报告。
