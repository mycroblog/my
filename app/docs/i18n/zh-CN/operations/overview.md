<!-- source: overview-of-a-mycroblog-server ce703d24935f -->
# mycroblog 服务器概述

这是运行服务器之前首先要读的页面。mycroblog 是一个通过 ActivityPub 与联邦宇宙中其他服务器联邦的微博客；`my` 是它的服务器软件，是一个单一的二进制文件，包含服务器、任务 worker、Web 客户端，以及维护它们的命令。本页介绍服务器由哪些部分组成、哪些部分通过配置选择、小型服务器如何成长为大型服务器，以及接下来该读什么。

<!-- source: what-a-server-is-made-of 1210fe6ac0c4 -->
## 服务器的组成

![mycroblog 服务器的架构：客户端通过 HTTPS 连接到 my 进程；进程使用通过配置选择的组件，以及你启用的服务器外部服务。下面各节分别介绍每个部分。](images/architecture.svg)

按图中从上到下的顺序：

- **谁会连接。** 用户在浏览器中使用 Web 客户端（也可以作为应用安装），应用和机器人使用客户端 API，联邦宇宙中的其他服务器通过 ActivityPub 投递和获取帖子。它们都通过 HTTPS 连接到服务器。
- **服务器前端。** `my` 使用明文 HTTP，因此由放在它前面的反向代理或负载均衡器终止 HTTPS。在媒体文件前面放置 CDN 是可选的。
- **`my` 进程。** 每个进程运行 `my serve`，它的 [`role`](../reference/configuration.md#role) 决定它运行什么：`web` 是 HTTP 服务器；`worker` 是任务 worker，负责投递帖子、从其他服务器获取内容、发送邮件、处理媒体以及运行周期性任务；默认值 `all` 则在一个进程中同时运行两者。
- **你选择的组件。** 图中间的每个组件都有一个选择其适配器的设置，列在[下一节](#parts-you-choose-by-configuration)中。
- **服务器外部的服务。** CAPTCHA、语音聊天、支付、插件和云费用导入在你设置之前都保持关闭，设置后服务器会调用你指定的服务。

<!-- source: parts-you-choose-by-configuration f7dbfce7a91c -->
## 通过配置选择的组件

每个组件都是[配置参考](../reference/configuration.md)中的一个设置，可以在 TOML 文件中设置，也可以作为环境变量设置（`MY_` 加上大写的键名，例如 `MY_DATABASE`）。每个列表中的第一个选项是默认值。对于保存数据的组件，请在首次启动之前做出选择：更改设置时，数据不会自动迁移。

| 组件 | 设置 | 选项 | 延伸阅读 |
| --- | --- | --- | --- |
| 数据库 | [`database`](../reference/configuration.md#database) | `sqlite`（一个文件）、`postgres`（PostgreSQL 及兼容服务） | [数据库](database.md)、[备份](backup.md) |
| 任务队列 | [`queue`](../reference/configuration.md#queue) | `db`（数据库中的一张表）、`inline`（在同一进程中立即运行每个任务，用于测试和开发）、`cloudtasks`（由 Google Cloud Tasks 为每个任务唤醒服务器；需要 PostgreSQL） | [任务队列](running.md#the-job-queue-and-failed-dead-jobs) |
| 用于缓存和速率限制的键值存储 | [`kv`](../reference/configuration.md#kv) | `db`（数据库中的一张表）、`memory`（每个进程各自独立）、`valkey`（Valkey 或其他使用 Redis 协议的服务器，由所有进程共享） | [`kv`](../reference/configuration.md#kv) |
| 媒体存储 | [`blob`](../reference/configuration.md#blob) | `fs`（一个目录）、`s3`（Amazon S3、Cloudflare R2、MinIO、Garage 及其他 S3 兼容存储）、`gcs`（Google Cloud Storage） | [Docker Compose 文件](running.md#docker-compose-files)、[备份媒体](backup.md#media-in-a-bucket) |
| 媒体处理 | [`media_image`](../reference/configuration.md#media_image)、[`media_video`](../reference/configuration.md#media_video)、[`media_audio`](../reference/configuration.md#media_audio)，每种文件各一个 | `native`（在服务器内）、`plugin`（由你的插件转换该类文件） | [转换媒体](plugins.md#converting-media) |
| 进程间的实时流 | [`stream`](../reference/configuration.md#stream) | `hub`（单个进程内）、`postgres`（`LISTEN` / `NOTIFY`）、`valkey`、`sns`（Amazon SNS 和 SQS）、`pubsub`（Google Cloud Pub/Sub） | [`stream`](../reference/configuration.md#stream) |
| 密钥管理器，用于加密账号的私钥 | [`key_manager`](../reference/configuration.md#key_manager) | `local`（由你保管的密钥加密密钥）、`gcpkms`（Google Cloud KMS）、`awskms`（AWS KMS）、`plugin` | [管理密钥](key-management.md) |
| 邮件 | [`mail`](../reference/configuration.md#mail) | `none`、`smtp`、`ses`（Amazon SES）、`plugin` | [发送邮件](email.md) |
| CAPTCHA | [`captcha`](../reference/configuration.md#captcha) | `none`、`turnstile`（Cloudflare Turnstile）、`hcaptcha`、`recaptcha`（Google reCAPTCHA v2） | [CAPTCHA](captcha.md) |
| 语音聊天 | [`voice`](../reference/configuration.md#voice) | `off`、`p2p`（浏览器到浏览器，TURN 由 `voice_turn` 指定：`none`、`coturn` 或 `cloudflare`）、`livekit`、`sora` | [语音聊天](voice.md) |
| 支持者付款 | [`payments`](../reference/configuration.md#payments) | `none`、`stripe`、`payjp`（PAY.JP） | [支持者](supporters.md)、[Stripe](payments-stripe.md)、[PAY.JP](payments-payjp.md) |
| 插件 | `plugin_<kind>_address` | 默认没有；每种类型一个地址：`translator`、`search`、`billing`、`mailer`、`media_image`、`media_video`、`media_audio`、`key_manager`，以及决策钩子 `inbound.status`、`local.status`、`inbound.follow`、`signup.check`、`media.check`、`inbound.actor`、`local.profile` | [插件](plugins.md)、[决策钩子](decision-hooks.md) |
| 帖子翻译 | `translator_<provider>_*`、[`plugin_translator_address`](../reference/configuration.md#plugin_translator_address) | 无（仅使用浏览器自带的翻译）；`libretranslate`、`deepl`、`google`（Google Cloud Translation）、`aws`（Amazon Translate），或放在你所选服务前面的你自己的插件 | [翻译帖子](translation.md)、[翻译插件](plugins.md#translating-posts) |
| 帖子全文搜索 | [`search`](../reference/configuration.md#search) | `none`；或 `elasticsearch`、`opensearch`、`meilisearch`（[搜索引擎](search-engines.md)）；或 `plugin`，即放在其他引擎前面的你自己的插件 | [全文搜索](search.md)、[搜索插件](plugins.md#searching-posts) |
| 媒体 CDN | [`media_origin`](../reference/configuration.md#media_origin) | 不设置（服务器从自己的源站提供媒体），或你的 CDN 的源站 | [通过 CDN 提供媒体](cdn.md) |
| 云费用 | [`billing_sources`](../reference/configuration.md#billing_sources) | 无，或 `aws`（AWS Cost Explorer）、`gcp`（Google Cloud 的账单导出或预算）、`plugin` 中的任意几项 | [导入云费用](billing-import.md) |
| 指标 | [`metrics_exporter`](../reference/configuration.md#metrics_exporter) | `none`、`prometheus`、`otlp`（OpenTelemetry）、`emf`（CloudWatch） | [指标](running.md#metrics)、[告警](alerts.md) |
| 追踪 | [`traces_exporter`](../reference/configuration.md#traces_exporter) | `none`、`otlp` | [追踪](running.md#traces) |
| 错误报告 | [`errors_exporter`](../reference/configuration.md#errors_exporter) | `none`（仅日志行和错误分组）、`sentry`（Sentry、GlitchTip、Datadog）、`otlp` | [错误报告](error-reporting.md) |

发布的二进制文件和容器镜像包含上表中的所有适配器。你自己构建的二进制文件可以通过[构建标签](../reference/build-tags.md)去掉不使用的适配器，这样在启动时会拒绝相应的设置。

`my config check` 会在你启动服务器之前一次性报告配置中的所有问题（[上线检查清单](production-checklist.md)）。

<!-- source: from-a-small-server-to-a-large-one 42bbd3f2a76e -->
## 从小型服务器到大型服务器

![从小型服务器到大型服务器的三个阶段：先是使用 SQLite 和文件系统的单个进程，然后是 PostgreSQL 和位于 CDN 后面的对象存储，最后是拆分开的 web 和 worker 进程以及流代理。下面的列表分别介绍每个阶段。](images/growth.svg)

1. **从小规模开始。** 一个 `role = all` 的进程，使用 SQLite，媒体存放在文件系统上，其余使用默认值：队列和键值存储在数据库中，实时流在进程内部。`deploy/docker` 中的 Docker Compose 文件可以在一台机器上启动这套配置（[使用 Docker Compose 部署](deploy-docker-compose.md)）。它适合个人或小型服务器。`stream = hub` 只能到达它所在的进程，因此这套配置只能保持一个进程。
2. **把数据移出去。** 数据库使用 PostgreSQL，媒体使用 S3 兼容存储桶或 Google Cloud Storage，并在媒体前面放置 CDN。服务器仍是一个进程，但不再依赖单块磁盘，下一阶段的多个进程也可以共享这些数据。如果可以，请在首次启动前做出决定：之后再把数据从 SQLite 和文件系统迁出是一项单独的工作（[需要备份的内容](backup.md#what-to-back-up)列出了需要迁移的内容）。
3. **横向扩展。** 用 `role = web` 的进程处理请求，用 `role = worker` 的进程处理任务，按各自需要运行任意数量，并按任务类型拆分 worker（[拆分 worker](running.md#splitting-workers-by-job-type)）。有多个进程时，实时流需要一个能到达每个进程的代理（`postgres`、`valkey`、`sns` 或 `pubsub`），键值存储也应当共享（`db` 或 `valkey`），因为 `memory` 会让每个进程的速率限制彼此独立。任务队列继续留在 PostgreSQL 中，或迁移到 Google Cloud Tasks（[Cloud Tasks](running.md#cloud-tasks)）。

[Cloud Run 部署](deploy-cloud-run.md)是介于第二和第三阶段之间的一个配置示例：在 Cloud Run 上运行 `role = all`，PostgreSQL 使用 Neon，媒体存放在 Cloud Storage，任务使用 Cloud Tasks，流使用 Pub/Sub，没有人使用服务器时不运行任何实例。

<!-- source: where-to-go-next 2f1d25c01f8a -->
## 接下来读什么

按以下顺序阅读运营指南：

1. **开始：** [运行服务器](running.md)或[在 Cloud Run 上部署](deploy-cloud-run.md)，然后在向用户开放之前完成[上线检查清单](production-checklist.md)。
2. **配置实例：** 它的[设置](instance-settings.md)、[注册](registration.md)、[外部登录提供方](login-providers.md)、[邮件](email.md)、[CAPTCHA](captcha.md)、[CDN](cdn.md)、[链接预览](link-previews.md)，以及[搜索引擎和 AI 爬虫](crawlers.md)。
3. **启用功能：** [审核](moderation.md)、[中继](relays.md)、[Webhook](webhooks.md)、[插件](plugins.md)和[决策钩子](decision-hooks.md)、[语音聊天](voice.md)、[全文搜索](search.md)、[支持者](supporters.md)及其付款，以及[导入云费用](billing-import.md)。
4. **日常运行：** [监控](monitoring.md)、[告警](alerts.md)、[错误报告](error-reporting.md)、[维护](maintenance.md)、[备份](backup.md)、[数据库](database.md)、[密钥加密密钥](key-management.md)，以及[管理员和角色](admin-cli.md)。
5. **法律与关停：** [隐私政策](privacy-policy-template.md)和[处理活动记录](processing-record-template.md)模板，以及[关停服务](service-closure.md)。

[配置参考](../reference/configuration.md)列出了所有设置，[任务参考](../reference/jobs.md)列出了 worker 运行的所有任务。
