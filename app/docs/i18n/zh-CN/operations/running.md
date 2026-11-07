<!-- source: running-the-server-start-stop-and-health-checks 5164430b3203 -->
# 运行服务器：启动、停止与健康检查

本文介绍如何启动和停止服务器、获取镜像，以及在运行期间监控它：指标、追踪、任务队列和健康检查。

<!-- source: start-the-server 0d5f8898a2f5 -->
## 启动服务器

```sh
my config check   # report every configuration problem at once
my config check -instance   # also warn about instance settings that are not set (opens the database)
my serve          # start according to the configured role (-config <file> reads a TOML file)
my version        # print the version of the binary
```

`my serve` 会校验配置，构建数据库、任务队列和密钥管理的适配器，并在 `listen` 上接受 HTTP 请求。它在那里使用不带 TLS 的普通 HTTP/1.1：请在前面放置一个反向代理或负载均衡器，由它终结 HTTPS（以及面向客户端的 HTTP/2 或 HTTP/3），再通过 HTTP/1.1 转发。实时更新（`GET /api/v1/streaming`、本站时间线和联邦时间线的流，以及语音聊天房间的流）是长时间保持的响应，每 30 秒（语音房间为 15 秒）发送一次心跳注释，空闲时也保持打开。代理不得缓冲这些响应（nginx：服务器会发送 `X-Accel-Buffering: no`），并且它的空闲超时或读取超时必须长于心跳间隔。它不会应用 schema：请在启动前用 `my schema apply` 应用（参见[数据库](database.md)；升级时会在[升级](database.md#upgrading)的步骤中运行它）。在 Docker Compose 部署（`deploy/docker/compose.yaml`；参见[使用 Docker Compose 部署](deploy-docker-compose.md)）中，一个一次性的 `schema` 服务会在服务器启动前运行 `my schema apply`。如果 schema 尚未应用，进程仍会启动，就绪检查返回 503，并且在 schema 就位之前不提供其他任何服务。

第一个使用数据库的 `my serve` 或 `my` 命令会把 `host` 和 `account_domain` 记录到数据库中。当配置与记录的值不同时，`my serve` 会记录 `start failed` 日志并附上两组值，然后以退出码 1 退出，命令也会输出同样的错误。例外是不做检查的 `my schema apply` 和 `my db backup`，以及不读取数据库的维护模式。`my config check -instance` 和 `my db upgrade -check` 会检查，但不记录任何内容。启动时无法读取数据库的进程，在能够读取之前不会就绪；在此之前它不运行任何任务，并对除下面三个健康检查（`/healthz`、`/readyz`、`/api/v1/instance/health`）以外的所有请求返回带 `Retry-After` 的 `503`。在 `my schema apply` 之前启动的服务器也是如此。如果之后它发现值不同，就会以同样的错误停止。

在 `production` 配置档中，`my config check` 还会检查有效的设置是否适合生产实例：错误（例如其他服务器无法访问的 `host`，或不是随机生成的 KEK）会让它以 1 退出，而警告（例如 `mail = none`）不会。`my serve` 启动时会把两者都记录为警告，但仍然会启动，所以请把 `my config check` 作为部署的一个步骤来运行。[生产环境检查清单](production-checklist.md)列出了这些检查，以及上线前需要手动确认的事项。

| `role` | HTTP | 任务 worker |
| --- | --- | --- |
| `all`（默认） | 所有路由 | 运行 |
| `web` | 所有路由 | 不运行 |
| `worker` | 仅健康检查（供平台探针使用） | 运行 |

日志以 JSON Lines 格式写入标准错误。每一行都有 `level`（`DEBUG`、`INFO`、`WARN`、`ERROR`）和 `severity`，后者是以 Cloud Logging 读取的形式表示的同一级别（`DEBUG`、`INFO`、`WARNING`、`ERROR`，以及用于导致进程停止的错误的 `CRITICAL`）。处理请求期间写入的日志带有 `request_id`，同一 ID 也会在 `X-Request-Id` 响应头中返回。如果请求的 `X-Request-Id` 不超过 128 个字符，且只由字母、数字和 `-_.:` 组成，就会作为负载均衡器分配的 ID 原样使用。追踪中的日志（请求和任务）还带有 `trace_id` 和 `span_id`（参见[追踪](#traces)）。

每个 HTTP 请求和每次任务执行在结束时都会写入一行。这一行使用与下面[指标](#metrics)的标签相同的名称和值，因此在 `metrics_exporter = none` 的 Cloud Run 上，你可以基于这些行构建 Cloud Logging 的基于日志的指标（计数器和分布）。

| 行（`msg`） | 字段 |
| --- | --- |
| `http request`（`INFO`；健康检查使用 `DEBUG`） | `http.request.method`、`http.route`、`my.surface` 和 `http.response.status_code`（数字）与 `http.server.request.duration` 的标签值相同。另有 `duration_seconds`（秒数）、`url.path`（不含查询字符串的路径）、`http.response.body.size` 和 `request_id`。未到达路由的请求（健康检查、在路由前被拒绝的请求）没有 `http.route` 和 `my.surface`，指标也不计入它们。客户端在服务器作出响应前离开的请求，其状态为 `499` 而不是 5xx，除非处理程序发生了 panic；客户端什么也没收到，因此不算作服务器错误 |
| `job`（成功和 `postponed` 时为 `INFO`，`retry` 时为 `WARN`，`dead` 时为 `ERROR`） | `my.job.type` 和 `my.outcome` 与 `my.job.duration` 的标签值相同。另有 `duration_seconds`（`my.job.duration`）、`wait_seconds`（`my.job.wait`）、`job_id`、`attempt`，失败时还有 `error` |
| `error report`（`ERROR`；导致进程停止的失败为 `CRITICAL`） | 每个上报的错误一行（[错误报告](#error-reports)） |

<!-- source: error-reports 3d780744461c -->
### 错误报告

[错误报告](error-reporting.md)指南介绍了在各个平台上到哪里查看这些行、“错误”页面、通知，以及如何把报告发送到错误追踪服务。

服务器会把需要运营者关注的错误各写成一行 `error report`：

- HTTP 处理程序、任务或后台任务中的 panic；
- 客户端 API、管理 API、登录页面、ActivityPub、OAuth 或媒体返回 5xx 响应背后的错误；
- 永久失败（`dead`）的任务：重试次数用尽，或拒绝再次运行。因对方服务器持续失败而放弃的投递不会上报；
- 启动失败：配置无效，或者数据库或适配器无法建立。

4xx 响应、任务重试、维护模式、失败的就绪检查、客户端已离开的请求，以及由其他服务器引起的失败不会上报；指标和其他日志行会覆盖它们。

当请求的客户端离开时，服务器为该请求写的其他错误行会以 `INFO` 而不是 `ERROR` 写入，因为其中大多数只是说明请求被取消了。

这一行采用 Google Cloud Error Reporting 读取的形式：在 Cloud Run 上，无需任何设置，报告就会作为错误分组显示在 Error Reporting 中。在其他平台上，它是一行普通的 JSON：

| 字段 | 值 |
| --- | --- |
| `@type`、`serviceContext` | Error Reporting 的事件类型，以及 `{"service": "my", "version": "<version>"}` |
| `my.error.fingerprint` | 32 位十六进制数字，同一错误的每次发生都相同：由来源、位置、错误类型以及栈中服务器的函数构成，不含行号 |
| `my.error.source`、`my.error.kind` | `server`（或 `web`），以及 `panic` 或 `error` |
| `my.error.where` | 发生位置：请求为 `<surface> <method> <route>`（例如 `client_api GET /api/v1/statuses/{id}`），任务为任务类型，后台任务为任务名，启动失败为 `startup` |
| `my.role`、`deployment.environment.name` | 进程的 `role` 和 `errors_environment` |
| `request_id`、`job_id`、`attempt`、`trace_id`、`span_id` | 用于查找同一错误的请求行或任务行的 ID（如果有） |
| `message` | 位于行末：`<type>: <message>`、一个换行符，以及 Go 栈 |

在报告离开进程之前，消息中的 URL、电子邮件地址、`@user@host` 形式的用户名、IP 地址、UUID 和 ULID，以及 10 位及以上的数字，会分别替换为 `<url>`、`<email>`、`<acct>`、`<ip>`、`<id>` 和 `<n>`，并截断到 1 KiB。栈保留文件位置，但不保留函数行中的参数值（显示为 `(...)`），并截断到 16 KiB。任何报告都不包含请求体、请求头、查询、Cookie、账号、用户名、IP 地址或 User-Agent。其他行都不带栈，因此 Error Reporting 只会从这些行中找到栈。

字段名是一个包含 `.` 的单一键，并非嵌套。在 Cloud Logging 的过滤和提取中请给它加引号（例如 `jsonPayload."http.route"`，以及用于分布值的 `EXTRACT(jsonPayload.duration_seconds)`）。用作标签的字段只取有限集合中的值；`url.path`、`request_id` 和 `job_id` 从不作为标签。

当 `metrics_exporter = emf` 时，这两行本身就是 EMF 行。`http request` 行携带 `http.server.request.duration` 的值，`job` 行携带 `my.job.duration` 和 `my.job.wait` 的值（键以指标命名），另外还有 `_aws` 指令、`service.name`、`service.version` 和 `my.role`。这三个指标不会写在其他任何行上。你在 `metrics_emf_dimensions` 中选择的字段按 EMF 的要求以字符串写入（例如维度 `http.response.status_code` 为 `"200"`）。日志写入标准错误，其他 EMF 行写入标准输出。ECS 的 `awslogs` 日志驱动会把两者都发送到 CloudWatch Logs，因此会从两者中提取指标。

<!-- source: docker-compose-files df45a902f45b -->
## Docker Compose 文件

`deploy/docker/` 中存放着部署实例所需的文件。`compose.yaml` 可以单独运行，其他文件叠加在它之上。

| 文件 | 添加的内容 | 适用场景 |
| --- | --- | --- |
| `compose.yaml` | 服务器及其一次性的数据库服务，使用卷 `data` 中的 SQLite 和媒体文件 | 始终需要 |
| `compose.postgres.yaml` | 容器中的 PostgreSQL，位于卷 `postgres` 中 | 你想在这台机器上运行 PostgreSQL。如果 PostgreSQL 服务器在别处，就不要使用它，而是在 `.env` 中设置 `MY_DATABASE` 和 `MY_DATABASE_URL` |
| `compose.s3.yaml` | 把媒体放在兼容 S3 的存储桶中；配合 `COMPOSE_PROFILES=garage` 时，还会在卷 `garage` 中运行一个单节点的 [Garage](https://garagehq.deuxfleurs.fr/) 存储 | 媒体超出了一块磁盘，或者你想把它们放进存储桶。不使用该 profile 时，存储是 `MY_BLOB_S3_*` 设置所指定的外部存储 |
| `compose.caddy.yaml` | Caddy，在 80 和 443 端口上提供 TLS 证书 | `MY_HOST` 是公网域名，且这台机器上没有其他程序终结 TLS |
| `compose.build.yaml` | 从检出的代码构建镜像 | 你自己构建镜像 |

请在首次启动前选好数据库和媒体存储：文件改变时，数据不会自行迁移（[备份](backup.md)说明了需要迁移哪些内容）。

**叠加文件**：按上述顺序使用 `-f` 选项，或者在 `.env` 中一次性指定，这样之后的每条 `docker compose` 命令（`exec`、`logs`、`pull`，以及其他指南中的命令）都会使用相同的文件：

```sh
cd deploy/docker
cp example.env .env
# In .env: COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml
```

目录中名为 `compose.override.yaml` 的文件，只有在 `-f` 和 `COMPOSE_FILE` 都没有指定文件时才会被读取。如果指定了，也要把它列进去。

**设置。** 把 `example.env` 复制为 `.env`。Compose 读取 `.env` 来填充其文件中的变量，并把整个文件交给服务器的容器，因此[配置参考](../reference/configuration.md)中的每个设置都可以在这里设置后生效。模板列出了所有设置，并对每一项附有一句说明。

- 密钥加密密钥（KEK）不放在 `.env` 中，而是放在文件 `secrets/kek` 中，容器以 Docker secret
  `/run/secrets/my_kek` 的形式读取它，因此它永远不会成为环境变量。`.env` 中的 `MY_KEK` 一行会让服务器停止，因为服务器拒绝同时使用 `MY_KEK` 和 `MY_KEK_FILE`（[管理密钥](key-management.md)）。
- 服务器以用户 `65532` 运行，而 Compose 会按文件原有的权限挂载它，因此该文件必须对其他用户可读。请把它放在只有你能进入的目录中，就像 `example.env` 中的命令那样。
- 其他密钥以环境变量的形式传入容器。如果想改为以文件形式传入，请在你自己的 `compose.override.yaml` 中用 secret 挂载它，并设置其 `<NAME>_FILE`，就像 `compose.yaml` 对密钥所做的那样。
- `.env` 中含有机密信息：执行 `chmod 600 .env`，并在备份中保留一份副本。
- `MY_LISTEN` 保持为 `:8080`，因为这些文件连接的是该端口。

**使用 Caddy 的 TLS。** 把 `MY_HOST` 设置为 DNS 指向这台机器的域名，并确保 80 和 443 端口可从互联网访问，然后从 `.env` 中删除 `MY_WEB_ORIGIN`。Caddy 会在第一次请求时自行获取并续期证书。服务器信任来自 Docker 默认地址池的 `X-Forwarded-For`；如果你的地址池不同，请设置 `MY_TRUSTED_PROXIES`。

**内置的 Garage。** 配合 `COMPOSE_PROFILES=garage` 使用 `compose.s3.yaml` 时，需要 `example.env` 顶部列出的那些值。一次性服务 `garage-init` 会在每次 `up` 时准备存储。它只留在 Compose 网络内部，因为媒体由服务器自己提供。一个节点就是一块磁盘：请备份卷 `garage` 或存储桶（[备份](backup.md#media-in-a-bucket)）。

<!-- source: get-the-image 238d00ea9826 -->
## 获取镜像

服务器镜像是 `ghcr.io/mycroblog/my`，公开提供，支持 `linux/amd64` 和 `linux/arm64`，包含所有适配器。

| 标签 | 指向 | 用途 |
| --- | --- | --- |
| `0.1.0` | 该版本，永久不变 | 指定某个版本 |
| `0.1` | `v0.1` 的最新补丁版本 | Docker Compose，它会在每次 `up` 时应用 schema |
| `0.1.0-rc.1` | 该预发布版本 | 在正式发布前试用 |

没有 `latest`，也没有 `0`：两者都会跨越次版本，而每次升级到新的次版本都必须[经过上一个次版本](database.md#skipping-releases)。请在每条命令中指定版本；只写 `docker pull ghcr.io/mycroblog/my` 会以 `manifest unknown` 失败。补丁版本也可能需要 `my schema apply` 和 `my db upgrade`，因此在 Docker Compose 之外，请对每个版本按照[升级](database.md#upgrading)操作。在生产环境中，请固定该版本发布说明中给出的摘要：

```text
ghcr.io/mycroblog/my:0.1.0@sha256:...
```

在哪里运行：

- **Docker Compose**：`deploy/docker/compose.yaml` 运行 `.env` 中 `MY_VERSION` 所指定的镜像（[Docker Compose 文件](#docker-compose-files)）。如果要改为从检出的代码构建，请添加覆盖文件 `compose.build.yaml`（`docker compose -f compose.yaml -f compose.build.yaml up -d --build`），并把 `MY_VERSION` 设置为你自己的名称，因为它是你本地镜像的名称，也是 `my version` 输出的内容。
- **Kubernetes**：直接从 `ghcr.io` 拉取。
- **Cloud Run**：直接使用 `ghcr.io` 的镜像。部署时会把标签解析为摘要。
- **AWS（ECS、EKS）**：把镜像复制到 Amazon ECR，或者使用 ECR 拉取缓存（pull through cache），后者需要 ghcr.io 的凭据。
- **更小的镜像**：在检出的代码中执行 `docker build --build-arg VERSION=<name> --build-arg GO_TAGS=... -t my:<name> .`，可以构建包含较少适配器的镜像（[构建标签](../reference/build-tags.md)）。这是你自己的构建，因此不在下面的校验范围内。

<!-- source: verify-an-image b696c257662d -->
### 校验镜像

每个发布的镜像都带有通过 Sigstore 签名的来源证明（provenance），它把摘要与构建它的工作流和提交关联起来，此外还带有 SBOM。使用 [GitHub CLI](https://cli.github.com/)，如果命令要求，请先登录 `ghcr.io`：

```sh
gh attestation verify oci://ghcr.io/mycroblog/my@sha256:... -R mycroblog/my \
  --signer-workflow mycroblog/my/.github/workflows/release.yml
docker buildx imagetools inspect ghcr.io/mycroblog/my@sha256:... --format '{{ json .SBOM }}'
```

除非镜像是由本仓库的发布工作流构建的，否则第一条命令会失败。它会输出提交，即该版本标签所在的提交；镜像的标签 `org.opencontainers.image.revision` 指向同一个提交。

<!-- source: publish-the-source-code-agpl-30 2f3b73ce9dfc -->
## 公开源代码（AGPL-3.0）

`my` 采用 AGPL-3.0-only 许可证。AGPL-3.0 第 13 条要求，任何运行该服务器并让用户通过网络与之交互的人，都必须向这些用户提供获取正在运行的程序的对应源代码的方式。这适用于所有与服务器交互的人，而不仅仅是拥有账号的人。

用设置 `source_url`（环境变量 `MY_SOURCE_URL`；参见[配置参考](../reference/configuration.md#source_url)）指明源代码的公开位置。

| 值 | 含义 |
| --- | --- |
| 未设置 | 默认值 `https://github.com/mycroblog/my`。在你运行未经修改的 `my` 时使用：此时正式版本的 `GET /instance` 会指向该版本的源代码 `https://github.com/mycroblog/my/tree/v0.1.0`，NodeInfo 则指向仓库 |
| `https:` URL | 可以阅读源代码的位置（分支项目的仓库、特定版本的标签或归档等） |
| `mailto:` URI | 用于索取源代码的电子邮件地址（例如 `mailto:source@example.org`；允许 `?subject=...`） |

其他任何值（`http:`、`git@...`、相对路径、带有用户信息或 `#` 的 URL、没有地址的 `mailto:` 等）都会让 `my config check` 失败，服务器也不会启动。

该值出现在以下位置：

- 客户端 API 的 `GET /instance` 中的 `source_url`
- 网页客户端实例信息页面（`/about`）中的“源代码”部分，以及首页（`/`）上的链接
- NodeInfo（`/nodeinfo/2.1`）中的 `metadata.sourceCode`。`https:` URL 还会作为 `software.repository` 公开（NodeInfo 2.1 的 schema 只允许在 `software.repository` 中使用 `http(s)` URL，因此 `mailto:` 值只出现在 `metadata.sourceCode` 中）

<!-- source: when-you-fork-or-modify-my be83256fe6a3 -->
### 当你分支或修改 `my` 时

如果你运行的是修改过的 `my`（打了补丁、构建了分支版本，或者替换了依赖），默认 URL 就不再指向正在运行的程序的源代码。请把 `source_url` 改为能找到正在运行的版本（包括你的修改）的源代码的位置。指向与正在运行的版本相符的提交、标签或归档，而不是仓库的最新状态，能让用户可靠地获得相同的源代码。如果你运行的是未经修改的官方版本，请保留默认值。

<!-- source: when-you-use-mailto 11155a8b71ed -->
### 当你使用 `mailto:` 时

- 准备好在有人索取时及时提供对应源代码（包括构建所需的脚本）。只写一个地址而没有做好提供的准备，并不能履行这一义务。
- 使用在你停止运营服务器或负责人变更后仍然有效的地址。共用的运维地址比个人邮箱更合适。
- 确保垃圾邮件过滤不会丢弃索取请求。
- 使用你愿意公开的地址：任何人都能在信息页面、首页和 NodeInfo 中看到它。

<!-- source: metrics 2dea3a7f8f83 -->
## 指标

设置 `metrics_exporter`（默认 `none`）用于导出服务指标。相关设置见[配置参考](../reference/configuration.md)。

| `metrics_exporter` | 方式 | 适用于 |
| --- | --- | --- |
| `none` | 不导出任何内容。埋点几乎没有开销 | Cloud Run（改用平台的指标和基于日志的指标） |
| `prometheus` | 在 `metrics_listen`（默认 `127.0.0.1:9464`）上提供 `GET /metrics`。它是独立于公开 `listen` 的监听器，没有认证，并且在每种 `role` 下都会运行 | 自托管（由兼容 Prometheus 的服务器或 KEDA 抓取） |
| `otlp` | 通过 OTLP/HTTP 发送到 OpenTelemetry Collector。目标地址来自 `OTEL_EXPORTER_OTLP_ENDPOINT` 等标准 OTel 环境变量 | 可以运行 Collector 的环境 |
| `emf` | CloudWatch Embedded Metric Format 的 JSON 行。请求和任务的指标附带在上面所述的结束日志行上；其他指标在请求或任务结束时写入标准输出，每个标签组合一行 | AWS ECS |

下面列出了这些指标。在 Prometheus 中，名称里的 `.` 会变成 `_`，并加上单位后缀（`_seconds`，计数器还有 `_total`）。

| 指标 | 类型 | 标签 |
| --- | --- | --- |
| `http.server.request.duration` | Histogram，秒 | `http.request.method`、`http.route`（路由模式的路径；没有匹配时为 `other`）、`http.response.status_code`、`my.surface`（`client_api` / `admin` / `activitypub` / `oauth` / `media` / `web` / `payments` / `voice` / `queue`） |
| `http.server.active_requests` | UpDownCounter | `my.surface` |
| `my.inbox.activities` | Counter | `activity.type`（已知的 Activity 类型，或 `other`）、`my.outcome`（`accepted` / `duplicate` / `rejected` / `ignored` / `error`）、`my.inbox.rejection`（仅在 `rejected` 时：`signature` 表示签名或完整性证明缺失、无效或无法验证，或者无法获取其密钥；`suspended` 表示已停用的域名；`federation_off`；`rate_limited`；`invalid` 表示活动缺少有效的 `id`、`type`、`actor` 或 `object`） |
| `my.relay.activities` | Counter | `my.outcome`（`admitted` / `over_limit` / `restricted` / `not_receiving`：中继携带的活动是否被接收，以及被丢弃的原因；参见[联邦中继](relays.md)） |
| `my.move.activities` | Counter | `my.outcome`（`recorded` / `duplicate` / `not_own_actor` / `same_account` / `target_unavailable` / `alias_missing` / `target_moved` / `suspended` / `too_soon`：收到的 `Move` 是否被记录，以及被丢弃的原因） |
| `my.delivery.attempts` | Counter | `my.outcome`（`success` / `retry` / `gone` / `failed`；`gone` 包括 410，以及因对方服务器不可用或被停用而跳过的投递） |
| `my.delivery.duration` | Histogram，秒 | `my.outcome` |
| `my.plugin.failures` | Counter | `my.plugin.kind`（一种[插件](plugins.md)）、`my.plugin.code`（失败的 Connect 代码） |
| `my.plugin.slot_wait` | Histogram，秒 | `my.plugin.kind`（某次调用在该种类同时运行的调用中等待轮到自己的时长） |
| `my.hook.decisions` | Counter | `hook`、`mode`、`outcome`（[决策钩子](decision-hooks.md)记录的决策） |
| `my.hook.failures` | Counter | `hook`、`outcome`（`timeout`、`error`、`invalid` 或 `breaker_open`） |
| `my.hook.duration` | Histogram | `hook`（每次调用插件的秒数） |
| `my.job.duration` | Histogram，秒 | `my.job.type`（已注册的任务类型，或 `other`）、`my.outcome`（`success` / `retry` / `dead` / `postponed`：`postponed` 是一次没有做任何工作、也不消耗任务尝试次数的运行，例如等待其主机的链接卡片抓取） |
| `my.job.wait` | Histogram，秒 | `my.job.type` |
| `my.link_card.host_pauses` | Counter | `http.response.status_code`（`429` / `503`）。对某个主机的链接卡片抓取每暂停一次计一次；参见[链接预览](link-previews.md#limits-per-host) |
| `my.translation.requests` | Counter | `my.outcome`（`author` / `cache` / `provider` / `no_provider` / `limit` / `unsupported` / `failed`）。每次请求翻译查看者可见的帖子，或审核员请求翻译举报中的帖子或评论，计一次：由作者本人的文字、保留的译文或提供方作答，或者因没有可接收该帖子的提供方、达到每月上限、没有提供方能翻译该语言，或没有提供方返回可用的译文而被拒绝。 |
| `my.translation.characters` | Counter | `my.provider`（翻译提供方的 ID）。发送给提供方的帖子和举报评论的字符数，不含标记，包括失败的调用。[每月上限](instance-settings.md#admin-screens-and-the-client-api)统计的就是它，它可能与提供方计费的数量不同 |
| `my.timeline.delivery.lag` | Histogram，秒 | 无（从本站帖子创建到写入主页时间线并通知到流的时间） |
| `my.stream.connections` | UpDownCounter | `my.stream.kind`（`user`，`timeline` 表示本站时间线和联邦时间线的流，`voice` 表示语音聊天房间的流） |
| `my.accounts.registered` | Counter | 无 |
| `my.statuses.created` | Counter | `my.visibility` |
| `my.billing.imports` | Counter | `my.billing.source`（`aws` / `gcp` / `plugin`）、`my.outcome`（`success` / `failed`）、`error.type`（仅在 `failed` 时：失败的种类，与[导入云用量费用](billing-import.md)中相同）。每次尝试都计入，包括重试和等待下一次每日导入的失败 |
| `my.voice.turn.credentials` | Counter | `my.voice.turn`（`coturn` / `cloudflare`）、`my.outcome`（`issued` / `reused` / `failed`）。语音聊天参与者在加入房间或续期连接时每请求一次 TURN 凭据计一次：`reused` 表示再次返回了之前为同一连接签发的 Cloudflare 凭据，`failed` 表示无法连接到 Cloudflare 或被其拒绝，参与者因此收到错误而无法连接 |
| `my.errors.exported` | Counter | `my.outcome`（`success` / `dropped` / `failed`：发往 `errors_exporter` 服务的每份错误报告计一次。`dropped` 表示因已有 1,000 份报告在等待，或者服务要求服务器放慢速度而没有发送该报告，服务器会按服务要求的时长等待后再发送；`failed` 表示无法连接到服务或服务拒绝了报告。报告只发送一次，从不重试，关闭过程开始 2 秒后仍在等待的报告会丢失） |
| `my.queue.ready` | Gauge | `my.job.type`（数据库队列中可运行任务的数量） |
| `my.queue.oldest_ready_age` | Gauge，秒 | `my.job.type`（数据库队列中每种类型最早的可运行任务已等待的时长；没有可运行任务的类型没有值） |
| `my.queue.dead` | Gauge | 无 |
| `my.instance.users`、`my.instance.statuses` | Gauge | 无（本站用户数和帖子数，与 NodeInfo 中相同） |
| `my.media.stored_bytes` | Gauge，字节 | `my.media.origin`（`local` / `remote_cache`） |
| `my.db.size` | Gauge，字节 | 无（数据库的大小：PostgreSQL 为 `pg_database_size`；SQLite 为不含预写日志的数据库文件） |
| `my.billing.import.failing` | Gauge | `my.billing.source`（该来源最近一次导入失败时为 1，否则为 0） |
| `my.federation.instances` | Gauge | `my.federation.instance.state`（`available`：投递仍会发往、但自上次响应以来至少有一天失败过的服务器；`unavailable`：投递已暂停的服务器） |
| `my.metrics.snapshot.age` | Gauge，秒 | `my.metrics.snapshot.part`（`queue` / `totals`） |
| `my.errors.reported` | Counter | `my.error.source`（`server` / `web`）、`my.error.kind`（`panic` / `error`）。每份[错误报告](#error-reports)计一次 |

健康检查（`/healthz`、`/readyz`）不会被统计。任何标签都不包含远程域名、账号、URL 或客户端 IP。`my.queue.*`、`my.instance.*`、`my.media.stored_bytes`、`my.db.size`、`my.federation.instances` 和 `my.billing.import.failing` 是整个实例的值：每个进程都从保存在键值存储中的同一份快照报告相同的值。

<!-- source: system-metrics 8b5d40a4eb98 -->
### 系统指标

关于进程、Go 运行时以及与资源的连接的指标，通过同一个 `metrics_exporter` 导出。容器的 CPU、内存、实例数和入站请求交给平台的指标；这些指标覆盖的是平台看不到的进程内部的饱和情况。不要用它们来触发扩缩容；请把它们用于告警和估算实例数上限。

| 指标 | 类型 | 标签 |
| --- | --- | --- |
| `go.memory.used`、`go.memory.limit`、`go.memory.gc.goal` | Gauge，字节 | 只有 `go.memory.used` 带有 `go.memory.type`（`stack` / `other`）。`go.memory.limit` 仅在设置了 `GOMEMLIMIT` 时出现 |
| `go.memory.allocated`、`go.memory.allocations` | Counter | 无 |
| `go.goroutine.count`、`go.processor.limit`、`go.config.gogc` | Gauge | 无 |
| `go.schedule.duration`、`go.memory.gc.pause.duration` | Histogram，秒 | 无（从变为可运行到开始运行的等待时间，以及 GC 暂停） |
| `go.memory.gc.cycles` | Counter | 无 |
| `process.cpu.time` | Observable Counter，秒（自启动以来的总计） | `cpu.mode`（`user` / `system`） |
| `process.memory.usage` | Gauge，字节 | 无（常驻内存，RSS） |
| `process.unix.file_descriptor.count`、`my.process.file_descriptor.limit` | Gauge | 无（打开的文件描述符及其上限） |
| `process.uptime` | Gauge，秒 | 无 |
| `db.client.connection.count` | Gauge | `db.client.connection.pool.name`（`postgres` / `sqlite`）、`db.client.connection.state`（`idle` / `used`） |
| `db.client.connection.max` | Gauge | `db.client.connection.pool.name` |
| `db.client.connection.wait_time`、`my.db.client.connection.waits`、`db.client.connection.timeouts` | Observable Counter（自启动以来的总计；秒或次数） | `db.client.connection.pool.name`。`timeouts` 仅适用于 PostgreSQL |
| `my.adapter.operation.duration` | Histogram，秒 | `my.adapter`（`queue.db` / `kv.db` / `kv.memory` / `kv.valkey` / `stream.<stream>` / `blob.fs`）、`my.operation`、`my.outcome`（`success` / `error`） |
| `my.adapter.connections` | UpDownCounter | `my.adapter`（`valkey` 连接，以及 `stream.<stream>` 订阅者） |
| `my.adapter.reconnects` | Counter | `my.adapter` |
| `http.client.request.duration` | Histogram，秒 | `my.http.purpose`（`json` / `html` / `image` / `video` / `human_check` / `web_push` / `webhook` / `client_metadata` / `login_provider` / `robots`）、`http.request.method`、`my.http.status_class`（`2xx` 等）、`error.type`（`forbidden_destination` 表示被 SSRF 防护拒绝） |
| `http.client.open_connections` | UpDownCounter | 无 |
| `my.readiness.failures` | Counter | 无 |
| `my.http.origin_refused` | Counter | 无。每个因未经过 CDN 而被拒绝的请求计一次（[`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)）。对源站地址的探测会让它缓慢增长；如果它在流量停止的同时猛增，说明 CDN 不再发送该请求头。维护模式不统计任何内容，因此只有它的访问日志会显示这些请求 |

- Go 运行时指标（`go.*`）只由 `prometheus` 和 `otlp` 导出。使用 `emf` 时，一个定期快照任务会写入进程和连接池的 Gauge 以及 Observable Counter。
- 连接池等待和 CPU 时间是 Observable Counter：连接池和内核会保存自启动以来的总计，并在采集时读取。在 Prometheus 中它们是 `_total` 计数器（例如 `process_cpu_time_seconds_total`），在 OTLP 中是单调递增的总和，你可以用 PromQL 的 `rate()` 查看其增长。`emf` 没有总计计数器，因此它写入的是自上次写入以来的增量（用 CloudWatch 的 Sum 统计量把它们加起来）。
- 队列的 `claim` 只在取到任务或失败时才统计。等待中的 worker 的轮询不记录任何内容。
- 不导出 `db.client.connection.pending_requests`：pgx 和 `database/sql` 的统计信息不报告有多少请求在等待，因此用等待的次数和时长来代替。
- 请针对 `go.memory.used` 的总和与 `go.memory.limit` 之比，以及 `db.client.connection.wait_time` 的增长（说明达到了连接池上限）设置告警。

<!-- source: memory-limit-gomemlimit 8ea44722eefd -->
### 内存上限（`GOMEMLIMIT`）

`GOMEMLIMIT` 是由运营者设置的 Go 运行时环境变量；应用程序不会自行选择它。如果不设置，Go 会以默认值（无上限）运行，堆在接近容器内存上限时也不会积极回收。在有容器内存上限的环境中（Cloud Run、ECS、Kubernetes），请把它设置为该上限的 90% 左右（例如 512 MiB 的容器设置 `GOMEMLIMIT=460MiB`）。不要设置 `GOMAXPROCS`：Go 的默认值会跟随容器的 CPU 上限。

<!-- source: traces e274793f5885 -->
## 追踪

设置 `traces_exporter`（默认 `none`）用于导出分布式追踪。相关设置见[配置参考](../reference/configuration.md)。

| Span | 名称 | 属性 |
| --- | --- | --- |
| HTTP 请求（服务器） | `<method> <http.route>`（未到达路由的请求为 `<method>`） | `http.request.method`、`http.route`、`http.response.status_code`、`my.surface`。5xx 视为失败 |
| 任务执行（消费者） | 任务类型（`my.job.type`） | `my.job.type`、`my.job.attempt`、`my.outcome`。每次重试都是单独的 span |
| 事务 | `my.db.transaction` | `db.system.name`。每次重试都是单独的 span |
| 查询（客户端） | sqlc 的查询名（例如 `InsertJob`；否则为 SQL 的第一个词，例如 `BEGIN`） | `db.system.name`、`db.operation.name` |
| 投递（`my.delivery`） | `my.delivery` | `activity.type`、`my.outcome` |
| 出站 HTTP（客户端） | `<method>` | `http.request.method`、`server.address`（仅目标主机名）、`http.response.status_code`。4xx 或 5xx 视为失败 |

失败的 span 带有 `error.type`（例如 `timeout` 这样取值有限的名称，或状态码）。属性中从不包含账号、帖子或任务的 ID、URL、路径或查询、客户端 IP、User-Agent 值、SQL 文本或错误字符串。`server.address` 会把远程服务器的主机名留在接收端，因此由你决定接收端保留它们多久、谁可以查看。健康检查和被 `origin_secret` 拒绝的请求不会创建 span。

- 任务执行不是把它加入队列的 span（通常是 HTTP 请求）的子 span。它是一条**新追踪的根**，并带有指向加入队列的 span 的链接。在查看器中，请从请求的追踪沿着链接查看。
- 只有当连接方位于 `trusted_proxies` 中，并且在使用 [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) 时请求带有该密钥，传入的 `traceparent` 才会成为父级。否则它会被丢弃，并开始一条新追踪。`tracestate` 总是被丢弃。出站 HTTP（联邦、WebFinger、媒体、Web Push 等）不携带 `traceparent`。
- 采样：如果请求的可信代理父级被记录，或者任务的链接目标被记录，该请求就会被记录；否则按比例 `traces_sample_ratio`（默认 0.01）记录。降低比例永远不会让延续某个已记录请求的任务丢失其追踪。

| 目标 | 设置 | 方式 |
| --- | --- | --- |
| 默认，小规模自托管 | `traces_exporter = none` | 不导出任何 span。请在日志中跟踪 `request_id` 和 `job_id`。如果可信代理提供了 `traceparent`，其 `trace_id` 和 `span_id` 会被添加到日志中 |
| 自托管（Jaeger、Tempo、OpenTelemetry Collector 等） | `traces_exporter = otlp`、`traces_flush = batch` | 通过 OTLP/HTTP 发送到 `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`（或 `OTEL_EXPORTER_OTLP_ENDPOINT`，默认 `http://localhost:4318`）处的接收端。认证头放在 `OTEL_EXPORTER_OTLP_HEADERS` 中。要可靠地保留错误和慢追踪，请使用 Collector 的尾部采样 |
| Google Cloud（Cloud Run） | `none`，或用 `otlp` 加 `unit` 启用追踪 | 见下文 |
| AWS（ECS） | `none`，或用 `otlp` 加 `batch` 启用追踪 | 见下文 |

<!-- source: cloud-run 7f1a21bbfbef -->
### Cloud Run

即使使用 `none`，Cloud Run 自动创建的请求追踪与日志中的 `trace_id` 也共用同一个 ID（当你用 `trusted_proxies` 信任来自 Cloud Run 前端的连接时）。Cloud Run 是否总是覆盖公开请求的 `traceparent`，目前尚未验证。日志没有采用 Cloud Logging 能识别的追踪字段（`logging.googleapis.com/trace`）的形式。

要把应用的 span 发送到 Cloud Trace，请在同一服务中**以 sidecar 方式运行 OpenTelemetry Collector**。服务器不持有任何 Google 凭据；sidecar 使用应用默认凭据（Application Default Credentials）发送到 Cloud Trace 的 OTLP 端点。

- 服务器（入站容器）：`MY_TRACES_EXPORTER=otlp`、`MY_TRACES_FLUSH=unit`、`OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`
- Sidecar：一个 OpenTelemetry Collector，用 `otlp` 接收器通过 HTTP 在 `localhost:4318` 上接收，并用 `googlecloud` 导出器或 `otlphttp` 导出器（`https://telemetry.googleapis.com`，配合 `googleclientauth` 扩展）发送。请授予服务账号写入 Cloud Trace 的权限。
- 在每个被记录的请求或任务结束时，`unit` 会在返回前把到目前为止的 span 刷新到 sidecar。为了让 sidecar 在没有分配 CPU 时也能发送它们，启用追踪的环境请使用基于实例的计费。
- 成本是 sidecar 的资源和 span 的数量（Cloud Trace 的价格）。请用 `traces_sample_ratio` 选择比例。

<!-- source: aws-ecs 52fa09412f20 -->
### AWS ECS

要发送到 X-Ray，请在任务定义中以 sidecar 容器的方式运行 **AWS Distro for OpenTelemetry（ADOT）Collector**。服务器不具备 SigV4 签名能力；由 Collector 签名并发送到 X-Ray 的 OTLP 端点（`https://xray.<region>.amazonaws.com/v1/traces`）。

- 服务器：`MY_TRACES_EXPORTER=otlp`、`OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`。在 Collector 的配置中，用 `otlp` 接收器在 `localhost:4318` 上接收，并用使用 `sigv4auth` 扩展的 `otlphttp` 导出器发送到 X-Ray。
- 请授予任务角色写入 X-Ray 的权限。
- 成本是 sidecar 在任务资源中所占的部分，以及 X-Ray 记录的追踪数量（每月前 100,000 条免费）。

<!-- source: stop-the-server 37d4c0cc3d97 -->
## 停止服务器

收到 SIGTERM 或 SIGINT 时，服务器会优雅地关闭。

1. 它停止接受新连接和领取任务。
2. 它最多等待 25 秒，让进行中的请求和正在运行的任务完成。
3. 25 秒后，它会取消剩余请求和任务的上下文并关闭连接。然后它再给被取消的任务最多 3 秒来记录结果，并以退出码 1 退出（总共最多 28 秒，在 Kubernetes 和 ECS 默认的 30 秒宽限期之内）。被取消的任务会重试。在这 3 秒内没有停止，或者中途被终止的任务会被放弃，并在其租约到期后由 worker 重新领取，因此它可能运行两次。

服务器的某个后台任务发生 panic 时，也会开始同样的关闭过程，使服务器永远不会在损坏的状态下继续运行。`serve failed` 日志行会指出是哪个任务，进程以退出码 1 退出，由平台或进程管理器重新启动它。

对数据库、键值存储或队列进行操作时，请以维护模式（`MY_MAINTENANCE=true`）重新启动，而不是停止服务器。参见[维护](maintenance.md)。

<!-- source: the-job-queue-and-failed-dead-jobs 55023b12a1aa -->
## 任务队列与失败（dead）的任务

[任务](../reference/jobs.md)列出了每种任务类型：它做什么、由什么启动、如何重试，以及再次运行会产生什么效果。

管理后台的任务页面（`/admin/jobs`，权限 `manage_jobs`）有三个页面：

- **队列状态**（`/admin/jobs`）：每种状态以及每种类型和状态的数量、每种类型让任务等待多久，以及等待最久的任务已等待的时长。
- **任务列表**（`/admin/jobs/list`）：处于某一状态（待处理、等待中、运行中、重试中、失败）的任务表格，分页显示，包括类型、状态、计划运行或失败的时间以及尝试次数。失败的任务有“再次运行”。
- **单个任务**（`/admin/jobs/<id>`，从表格中的类型进入）：任务的全部信息、最后一次错误、键和载荷、未成功尝试的历史（每次尝试的结果、错误、worker 以及开始和结束时间），失败的任务还有“再次运行”。

管理 API 为 `GET /api/v1/admin/queue`、`GET /api/v1/admin/jobs?state=&type=`、`GET /api/v1/admin/jobs/{id}`、`GET /api/v1/admin/jobs/{id}/attempts` 和 `POST /api/v1/admin/jobs/{id}/retry`。已完成的任务不再位于队列中，因此找不到它的页面。

- 只有对由服务器保存任务的队列，才能列出等待中、运行中和等待重试的任务：`queue = db`、`queue = cloudtasks`（任务也在数据库中）和 `queue = inline`。对于每种队列，失败的任务和尝试历史都保存在应用数据库中，因此总能列出。
- 尝试历史只保留未成功的尝试（重试和最终失败的那次尝试）。成功的运行请通过结构化的 `job` 日志行（带有 `job_id`）跟踪。当尝试结束后经过 `job_attempt_retention`（默认 30 天；参见该[设置](../reference/configuration.md#job_attempt_retention)）时，历史会被每小时一次的清理删除。历史中不含载荷或处理程序的日志，错误消息会截断为前 2,000 字节。任务处理程序不会把电子邮件地址、IP 地址、令牌和帖子正文写入错误消息。仅限电子邮件任务类型的视图，参见[发送电子邮件](email.md)。

处理程序判定为永久失败的任务，以及用尽重试次数的任务，会在数据库队列（`queue = db` 和 `queue = cloudtasks`）中成为 dead 任务。`my job` 可以列出、重试和删除它们。任务页面和管理 API 的“失败”状态也可以列出并重试它们。

```sh
my job list                  # newest failures first: ID, type, run count, time of failure, last error (-limit sets the count, default 50)
my job retry <id> [<id>...]  # reset the attempt count and put the job back in the queue to run immediately
my job delete <id> [<id>...] # remove without running
my job types                 # print every job type of this release
```

- 如果已有一个具有相同幂等键的任务在等待，就由该任务代替被重试的任务。任务处理程序是幂等的，因此把同一个任务放回任意次都是安全的。
- 未知的 ID 会被输出，命令以退出码 1 退出；其他 ID 仍会被处理。退出码 2 表示用法错误。
- 没有人重试的 dead 任务，会在失败后经过 `dead_job_retention`（默认 30 天；参见该[设置](../reference/configuration.md#dead_job_retention)）时被每小时一次的清理删除。
- `queue = inline` 的 dead 任务存在于服务器进程的内存中，因此 `my job` 无法处理它们（退出码 1）。

<!-- source: splitting-workers-by-job-type 2ec73f050551 -->
### 按任务类型拆分 worker

默认情况下，每个 worker 都运行所有任务类型，按任务到期的顺序，每个进程一次运行一个任务（参见[同时运行多个任务](#running-several-jobs-at-once)）。这样一来，一阵突发的投递会延迟电子邮件和通知，而你新增的任何 worker 也要把时间分给所有这些任务。使用 `queue = db` 时，你可以为某些类型分配专用的 worker：

- [`worker_job_types`](../reference/configuration.md#worker_job_types)：该进程只运行这些类型。
- [`worker_job_types_except`](../reference/configuration.md#worker_job_types_except)：该进程运行除这些以外的所有类型。

两者都是以逗号分隔的[任务类型](../reference/jobs.md)列表（`my job types` 会输出它们；`my config check` 会拒绝不匹配任何类型的名称），写作名称（`federation.deliver`）或前缀（`federation.*`）。请按进程设置它们，例如通过 `MY_WORKER_JOB_TYPES`。同一个进程不能同时设置两者。不匹配任何任务类型的名称或前缀会让启动停止，这也能发现被某个版本改名的类型，或者只有在设置了你尚未设置的某项设置时才存在的类型。它们不适用于 `role = web`，也不适用于 `queue = cloudtasks` 或 `inline`，这些情况下启动同样会失败：在那里由信号指定要运行的任务，因此进程无法选择自己的任务。

启动时，设置了其中一项的进程会在日志中记录它跳过的类型。它看不到其他进程的设置，所以**请确保每种类型都有某个进程在运行**，否则这些任务会永远等待。定期清理和电子邮件也是任务类型。每个进程仍然都会启动定期任务；由哪个进程运行它们取决于这些设置。

请根据指标而不是类型名称来决定拆分什么。`my.job.wait`（Prometheus：`my_job_wait_seconds`）按类型显示任务在开始前等待了多久：

```promql
histogram_quantile(0.95, sum by (my_job_type, le) (rate(my_job_wait_seconds_bucket[15m])))
```

`sum by (my_job_type) (rate(my_job_duration_seconds_sum[1h]))` 显示每种类型占用的 worker 时间，这也大致相当于它需要多少个进程。占用大部分时间并让其他类型等待的类型，就是应当移出去的那个。

两种常见的布局，进程数量由你决定：

| 布局 | 进程 |
| --- | --- |
| 避免投递延迟其他任务 | 投递用 `worker_job_types = federation.fanout,federation.deliver`，其他所有任务用 `worker_job_types_except = federation.fanout,federation.deliver`。突发期间给第一组增加进程。 |
| 避免搜索后端延迟其他任务 | 搜索任务用 `worker_job_types = search.*`，其他所有任务用 `worker_job_types_except = search.*`（[避免搜索延迟其他任务](search.md#keeping-search-from-delaying-other-jobs)）。 |
| 让电子邮件和通知保持迅速 | 这些任务用 `worker_job_types = identity.verification_email,identity.password_reset_email,identity.password_notice_email,identity.closure_email,identity.deactivation_email,notification.email,notification.moderator_email,notification.supporter_email,notification.push,notification.post_subscribers,instance.notify_announcement,voice.notify_started`，另设一个不带设置的 worker 运行所有任务，它在空闲时也会帮忙处理这些类型。请去掉你尚未设置的设置所对应的类型，否则启动会停止：`notification.push` 需要 `vapid_private_key`，`notification.email`、`notification.moderator_email` 和 `notification.supporter_email` 需要 `mail`。参见[依赖设置的任务](../reference/jobs.md#jobs-that-depend-on-settings)。 |

像 `identity.*` 这样的前缀表示拥有这些类型的代码模块，而不是一类工作。`identity.*` 还会包括数据导出、导入和过期任务，却不包括 `instance.notify_announcement` 和 `voice.notify_started`，因此针对某一类工作的布局要逐一写出每个类型。

请把下面这些类型放在一起，放在设置相同的 worker 上。同一排序键的任务无论类型如何都按顺序运行，所以如果没有进程运行较早的任务，较晚的任务也会等待；如果较早任务所在的组积压，较晚的任务也要等它：

| 类型 | 保持顺序的内容 |
| --- | --- |
| `federation.fanout`、`federation.deliver` | 一个喜欢及其撤销的投递 |
| `voice.apply_media_event`、`voice.apply` | 一个语音房间的事件 |
| `supporters.sync`、`supporters.command` | 一个订阅的变更 |

升级时：

- 设置了 `worker_job_types` 的进程不会运行新版本新增的类型。请查看发布说明和日志中被跳过的类型。设置了 `worker_job_types_except` 的进程会运行它们，因此它更适合用来处理“其他所有任务”。
- 被改名的类型会让启动停止，所以请先阅读发布说明。

要推行拆分，请先启动新的 worker，再给旧的 worker 加上设置。要回滚，请先从一组中移除设置，再停止另一组。任务会留在队列中，因此不会丢失任何东西，一段时间内无人运行的类型只是在等待。

**检测无人运行的类型。** 这类类型的任务会等待，因此该类型的 `my.queue.oldest_ready_age` 会增长，告警 `MyQueueBacklog` 和 `MyQueueStalled` 会针对等待最久的类型触发。要查看是哪个类型，或者按类型告警：

```promql
max by (my_job_type) (my_queue_oldest_ready_age_seconds) > 1800
```

管理后台的任务页面会按类型显示同样的等待时长。设置了其中一项的 worker 也不会处理根本没有处理程序的类型的任务（在指标中计入 `other`），因此如果没有不带设置的 worker，这样的任务会一直等待，而不会成为 dead 任务。

<!-- source: running-several-jobs-at-once 91e5181261b2 -->
### 同时运行多个任务

[`worker_job_concurrency`](../reference/configuration.md#worker_job_concurrency) 设置一个进程同时运行多少个任务，范围为 1 到 128。它适用于 `queue = db`；使用 Cloud Tasks 时由平台决定。默认一次运行一个任务，设置为 1 时效果相同。

对于把时间花在等待其他服务器上的任务（例如 `federation.deliver`），请调高它。这类任务每个只占用很少的内存和数据库连接，因此一个进程就能完成许多进程的工作：在一次本地测试中，任务只是等待 200 ms 的响应，一个拥有 32 个槽位的进程与 32 个各有一个槽位的进程效果相当，而只用了其中一小部分内存。那些任务不做别的事，因此请测量你自己的实例。请把它与 `worker_job_types` 搭配使用，为每类任务分配一个使用各自数值的进程。

请注意以下限制：

- **数据库连接**：一个任务在被领取、执行自身工作和记录结果时都需要连接。使用 `database = postgres` 时，该设置必须小于连接池的最大值，否则服务器不会启动。最大值是 `database_url` 中的 `pool_max_conns`，默认为 4 与 CPU 数量中的较大者，因此在你调高它之前，一个 vCPU 只允许 3。请确认你的数据库或连接池代理能接受所有进程的连接。
- **SQLite**：它一次只写一个事务。超过 32 时服务器会记录警告；在一次本地测试中，SQLite 在 128 时比在 32 时运行得更慢。
- **CPU 密集型任务**：槽位共享一个进程的 CPU 和内存。`media.process` 默认会在一半 CPU 数量的限制下等待轮到自己，而这段等待计入任务 5 分钟的租约，因此塞满 `media.process` 的进程可能超出租约并重新开始。请为这类任务分配槽位较少的进程。
- **每次一个服务器**：一个进程与任意单个主机最多建立 8 个连接，因此无论有多少槽位，一个进程向单个服务器投递的速度都不会超过 8 个连接所允许的速度。如果大多数投递都发往少数几个大型服务器，增加进程有用，而增加槽位没用。
- **卡住的服务器会占用槽位**：等待缓慢服务器的槽位无法用于其他任务，因此请把投递与电子邮件和通知分开。
- **部署会消耗尝试次数**：服务器停止时，在 `ShutdownTimeout`（默认 25 秒）之后仍在运行的任务会被取消，每个被取消的任务消耗一次尝试。运行慢任务的槽位越多，每次部署消耗的尝试次数就越多。没能及时记录结果的被取消任务，要等租约到期后才会再次运行。
- **崩溃会带走所有槽位**：如果进程耗尽内存，它的所有任务都要等租约到期。

要决定是否调高它，请按 `my.job.type` 查看 `my.job.wait`：如果某种类型的任务等待很久，而 `my.job.duration` 大部分是在等待其他服务器，同时 CPU 和内存都保持在低位，那么增加槽位应该会有帮助。如果调高后等待仍然很长，请先检查上面所说的数据库连接和主机限制，再考虑继续调高。

没有到期的任务时，一个槽位负责轮询，其他槽位等待，因此空闲进程给数据库带来的开销与一次运行一个任务的进程相同。

<!-- source: waiting-for-a-database-connection f481d577ea58 -->
### 等待数据库连接

使用 `database = postgres` 时，每条语句和每个事务都要先等待进程连接池中的空闲连接。[`database_acquire_timeout`](../reference/configuration.md#database_acquire_timeout)（默认 `60s`）限制的就是这段等待，而不限制其他任何东西：已经拿到连接的长查询或长事务会一直运行下去，无论耗时多久。`0` 表示无限等待。SQLite 没有需要等待的连接数上限，因此在 SQLite 上设置它会导致启动失败。

等待超时后：

- 请求会收到带 `Retry-After: 5` 的 `503 Service Unavailable` 响应。
- 任务会在 30 秒后再次运行，并且不消耗尝试次数，因此繁忙的数据库永远不会把任务变成 dead 任务。
- `db.client.connection.timeouts` 指标会计入一次，并触发 [`MyDBPoolWaiting`](alerts.md#mydbpoolwaiting) 告警。

默认值等于服务器 60 秒的写超时：等待更久的请求无论如何都无法发送响应，因此默认值不会让任何本可以成功的请求失败。请根据你的实例的运行方式选择一个值：

- **调低它**可以快速卸载负载：客户端会在 `Retry-After` 之后重试，而不是让请求一直挂着。请让它长于你最慢的正常事务，否则一阵短暂的突发就会开始返回 `503`。
- **先确定连接池的大小。** 一个进程需要的连接数大约等于它同时运行的工作数。`database_url` 中的 `pool_max_conns` 默认为 4 与 CPU 数量中的较大者，并且所有进程加起来必须低于数据库的连接上限。
  - 使用 [`worker_job_concurrency`](#running-several-jobs-at-once) 时，槽位也要等待连接，并且该设置必须小于连接池的最大值。
  - 在 Cloud Run 上，一个服务默认每个实例接受 80 个并发请求。连接池为 4 时，一阵慢请求的突发会在这里排队，因此请调高 `pool_max_conns`、调低服务的并发数，或两者都做。
- **频繁出现 `503` 或 `db.client.connection.timeouts`** 说明连接池太小或查询变慢了；更长的超时只会掩盖这一点。请按 [`MyDBPoolSaturated`](alerts.md#mydbpoolsaturated) 处理。

<!-- source: cloud-tasks ef3f7d7cc896 -->
### Cloud Tasks

[在 Cloud Run 上部署](deploy-cloud-run.md)使用 OpenTofu 模块完成下面的全部设置。

使用 `queue = cloudtasks` 时，任务保存在数据库中（`database = postgres`），Google Cloud Tasks 只负责为每个任务唤醒服务器：

- 对于它保存、重试或放回的每个任务，服务器会在事务提交后在 `queue_cloudtasks_queue` 中创建一个 task。该 task 会在任务到期时调用
  `POST {queue_cloudtasks_url}/_internal/jobs`，请求体为任务的 ID，并附带
  `queue_cloudtasks_service_account` 的 OIDC ID 令牌。到期时间超过 30 天之后的任务会在 30 天时发出信号，并在该 task 到达时再次发出信号。
- 收到 task 的进程运行该任务，并在记录结果后返回 `204`，任务已经运行过或尚未到期时也是如此。失败的任务由服务器在下一次尝试时间重试，而不是由 Cloud Tasks 重试。Cloud Tasks 只在响应为
  `5xx`（无法连接到数据库）或 `401`（服务器拒绝的令牌）时重试 task，因此无需调整它的 `retryConfig`。
- 没有进程轮询表，也没有进程自行运行定期任务。请创建一个 Cloud Scheduler 作业，每分钟（`* * * * *`）调用
  `POST {queue_cloudtasks_url}/_internal/tick`，并附带同一服务账号的 OIDC 令牌（设置了 `queue_cloudtasks_audience` 时，还要使用其受众）。每次 tick 会把到达周期的定期任务加入队列，在整个实例中只加入一次（周期在 KV 中领取，因此 `kv` 必须为 `db` 或 `valkey`），并对到期超过 2 分钟仍未运行的任务再次发出信号，从而恢复丢失的信号。
- `/_internal/jobs` 和 `/_internal/tick` 由 `role = all` 或 `worker` 的进程提供，只接受由 Google 为
  `queue_cloudtasks_service_account` 签名、电子邮件已验证，且受众为 `queue_cloudtasks_audience`（默认为被调用的 URL）的令牌。其他任何请求都会得到 `401`。
- 一次投递运行一个任务，最长不超过 5 分钟的租约。请让 `queue_cloudtasks_dispatch_deadline`（默认 10 分钟）和
  Cloud Run 服务的请求超时都长于这个时间。
- 服务器自身的服务账号需要队列上的 `roles/cloudtasks.enqueuer` 以及 `queue_cloudtasks_service_account` 上的 `roles/iam.serviceAccountUser`。它使用应用默认凭据（Application Default Credentials）进行认证。

<!-- source: routing-job-types-to-their-own-queues 292b3f6071a8 -->
### 把任务类型路由到专用队列

使用 `queue = cloudtasks` 时，worker 无法选择它运行的任务：`worker_job_types` 只适用于 `queue = db`，因为由平台决定谁接收信号。要为某些任务类型分配专用容量，请把它们的信号发送到专用队列，并在平台上设置该队列的容量。不设置路由就不会有任何变化。

[`queue_cloudtasks_routes`](../reference/configuration.md#queue_cloudtasks_routes)：`federation.*=projects/p/locations/l/queues/my-federation@https://federation-worker.example.com,notification.email=projects/p/locations/l/queues/my-mail`。不带 `@` 和 URL 的路由会调用 `queue_cloudtasks_url`。

左侧是[任务参考](../reference/jobs.md)中的任务类型或前缀。名称优先于前缀，较长的前缀优先于较短的前缀。不匹配任何路由的类型，包括以后的版本新增的类型，会使用默认队列（`queue_cloudtasks_queue`），因此它们会继续运行。`my config check` 会检查路由的格式；左侧不匹配任何任务类型时，`my serve` 会在启动时停止。

**给每个进程设置相同的路由，包括 `role = web`。** 信号由保存任务的进程发送：对于用户的大部分操作（例如帖子的第一个任务），这是 web 进程；对于由任务保存的任务、重试、同一排序键的下一个任务以及丢失信号的补发，这是 worker。如果每个角色有各自的设置（就像 Cloud Run 服务那样），请把相同的值传给所有角色：

```sh
routes='federation.*=projects/p/locations/l/queues/my-federation@https://federation-worker.example.com'
gcloud run services update my-web --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
gcloud run services update my-worker --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
gcloud run services update my-federation-worker --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
```

路由不同的进程不会报错，也不会丢失任务：它的信号会发往另一个队列，任务在那里运行，不受你设置的容量约束。`my.queue.oldest_ready_age` 不会显示这一点，因为这些任务并不等待。为了比较各个进程，每个进程在启动时都会记录一行日志，包含它的 `routes` 及其 `routes_hash`：这个哈希在所有地方都必须相同。要查看信号发往何处，请查看平台自身提供的每个队列的指标。

**权限。** 每个进程的凭据都需要能向路由中的每个队列发送：`roles/cloudtasks.enqueuer`。发送失败会被记录，并且不会改为尝试另一个队列，这样你限制的类型就不会溢出到其他队列。这些任务会延迟开始，要等 2 分钟后补发再次发送它们的信号。

**容量。** 任何接收方都能运行任何任务，因此由你选择把它们分开到什么程度：

| 平台 | 单一接收方 | 独立的接收方 |
| --- | --- | --- |
| Google Cloud | 一个服务。每个队列有各自的 `maxConcurrentDispatches` 和 `maxDispatchesPerSecond`；服务的实例数上限是共用的 | 每个队列一个服务，由路由中的 URL 指定，各有自己的实例数上限和并发数。每个服务都把 `queue_cloudtasks_url` 设置为自己的 URL |

每个队列都需要与默认队列相同的设置：队列上的角色。只有默认队列需要 tick。

**增加队列的代价。** 每个队列都是又一个需要创建、监控和付费的东西，其影响有多大取决于你的实例规模。请只拆分你在 `my.job.wait` 中看到在等待的类型，而不是全部拆分。

**补发。** 补发每分钟查找一次到期超过 2 分钟仍未运行的任务，每条路由最多 500 个，默认队列最多 500 个，并再次为它们发出信号。因此，跟不上的队列每分钟最多会收到 500 个重复信号。它们不会让任务运行两次，但这些都是需要付费的消息，积压清除后它们就会停止。

**排序键**依然有效：由数据库决定什么可以运行。同一键的任务可能位于不同的队列，但停滞的队列也会延迟其键下较晚的任务。

**添加、移动和删除队列。**

1. 创建队列及其接收方，并赋予每个进程向它发送的权限。
2. 在每个进程上修改路由。在它们不一致期间，信号会发往任意一个队列，任务会在两处运行。
3. 要删除队列，请先在所有地方把它从路由中移除，等它清空后再删除。已删除队列中的信号会丢失，但任务不会：任务到期 2 分钟后，补发会再次为它发出信号。Cloud Tasks 的 task 可以为最多 30 天之后到期的任务保存信号，因此到期时间晚于等待时间的任务，会在到期时直接由补发发出信号。

<!-- source: health-checks 924644f30f58 -->
## 健康检查

| 路径 | 类型 | 何时返回 200 |
| --- | --- | --- |
| `GET /healthz` | 存活检查（liveness） | 进程能够响应 HTTP。它不检查数据库等依赖（因此缓慢的数据库不会导致进程重启） |
| `GET /readyz` | 就绪检查（readiness） | 数据库可以访问，且 schema 已应用 |
| `GET /api/v1/instance/health` | 可用性监控、状态页面和客户端（由 `web` 角色提供） | 与就绪检查相同。结果在进程中缓存 60 秒 |

- `/healthz` 或 `/readyz` 返回 200 时，响应体为 `{"status":"ok"}`。
- `/readyz` 在每次请求时都检查数据库，并发的请求共用一次检查。
- `/api/v1/instance/health` 在健康时返回 200 和 `{"status":"ok","checked_at":"<RFC 3339>"}`，失败时返回 503 和 `{"status":"unavailable","checked_at":...}`（`Cache-Control: no-store`）。无论被监控或请求得多频繁，每个进程每分钟最多访问一次数据库，并且它既不查看访问令牌，也不受速率限制约束。请把可用性监控指向这个端点，而不是探针（`/readyz`）。关于各服务的设置、监控间隔和成本，以及如何处理维护，参见[可用性监控指南](monitoring.md)。
- 这三个端点在访问日志中都以 Debug 级别记录，并且不创建追踪 span。
- 就绪条件不满足时，响应为 503，响应体为 RFC 9457 的 `application/problem+json`。其 `title` 会根据 `Accept-Language` 进行翻译，且不包含任何内部细节。原因（连接错误，或缺失的表名）会写入日志。
- 单次就绪检查在 3 秒后中止。
- 就绪还要求数据库中保存着已配置的 `host` 和 `account_domain`，或者进程能够记录它们。
- **schema 已应用**是指 `schema/<dialect>/` 中的文件用 `CREATE TABLE` 声明的每张表都存在（SQLite 通过 `sqlite_schema` 检查，PostgreSQL 通过连接的 `current_schema()` 检查）。`my schema apply` 会应用整个 schema，因此缺少某张表意味着 schema 尚未应用，或者比二进制文件旧。列和索引的差异不会被检查；`my schema apply -dry-run` 会输出将要发生的变更。

<!-- source: requests-that-bypass-the-cdn 2cca4127af07 -->
## 绕过 CDN 的请求

当服务器位于 CDN 之后、而它自己的地址仍然公开时，请设置 `origin_secret` 和 `origin_secret_header`，让它拒绝未经过 CDN 的请求。健康检查仍然可以访问。参见[拒绝未经过 CDN 的请求](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)。

<!-- source: security-headers 8c5e82848149 -->
## 安全响应头

每个响应都带有 `Strict-Transport-Security`、`X-Content-Type-Options`、`Referrer-Policy`、`Permissions-Policy`、`Cross-Origin-Opener-Policy`、`Cross-Origin-Resource-Policy: same-site` 和 `X-Frame-Options`，对于不是文档的响应，还带有 `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'; base-uri 'none'`。对于需要其他值的响应（例如 ActivityPub JSON 和公开媒体），处理程序会覆盖这些值。`Permissions-Policy` 禁止摄像头、地理位置、支付和 USB。它也禁止麦克风，但语音聊天开启时（`voice` 不是 `off`）除外：此时它只允许服务器自身源的页面使用麦克风（`microphone=(self)`），以便浏览器能在通话中说话。网页客户端的 HTML 会得到一个不使用 nonce 的文档 CSP（`script-src 'self'` 等）。它要求使用 Trusted Types（`require-trusted-types-for 'script'`），并把可以创建的策略限制为客户端的两个（`trusted-types my-server-sanitized-html my-service-worker`）。

<!-- source: web-client f35b79dd2cc3 -->
## 网页客户端

`role` 为 `web` 或 `all` 的进程会提供嵌入在二进制文件中的网页客户端。

| 路径 | 内容 | `Cache-Control` |
| --- | --- | --- |
| 不匹配其他任何路由的 `GET`（`/`、`/@alice`、`/@alice/status/{id}`、`/settings` 等） | SPA 的 HTML 外壳。实例设置通过 `<script type="application/json" id="config">` 传入 | `public, max-age=60, stale-while-revalidate=300`、ETag |
| `GET /assets/*` | 带哈希名称的构建文件 | `public, max-age=31536000, immutable`、ETag |
| `GET /manifest.webmanifest` | 根据实例设置生成的 Web 应用清单 | `public, max-age=60, stale-while-revalidate=300`、ETag |
| `GET /sw.js`、`/icon.svg` 以及构建目录下的其他文件 | 原样提供 | `no-cache`、ETag |
| `GET /licenses/*` | 构建中打包内容的许可证（npm 包的声明，以及打包的非 npm 数据所用的 `LICENSE-Unicode.txt` 和 `OFL.txt`）。无论扩展名如何，都以 `text/plain; charset=utf-8` 提供 | `no-cache`、ETag |
| `GET /robots.txt`、`/.well-known/tdmrep.json`、`/.well-known/security.txt` | 根据实例设置生成（参见[搜索引擎与 AI 爬虫](crawlers.md)） | `public, max-age=3600`、ETag |

- `/api`、`/oauth`、`/auth`、`/.well-known`、`/nodeinfo`、`/ap`、`/media`、`/uploads`、`/theme`（服务器主题的样式表）和 `/licenses` 下的路径，以及 `/robots.txt`，永远不会返回外壳；如果没有其他路由匹配，它们会返回 404 problem。
- 外壳在 ActivityPub 内容协商之后提供。用 ActivityPub 的 `Accept` 请求外壳路径会返回 404（帖子页面会重定向，见下文）。外壳总是带有 `Vary: Accept`。如果在前面放置 CDN，请把 `Accept` 的种类（ActivityPub 或 HTML）加入缓存键，或者把外壳、`/ap/*` 和 `/@*` 排除在缓存之外。只让 CDN 提供 `/assets/*`。
- 不含网页客户端构建的二进制文件（直接 `go build`，没有先构建客户端）会在启动时记录警告，并提供一个不含脚本的外壳。

<!-- source: federation-endpoints 7aaee8c9da42 -->
## 联邦端点

`role` 为 `web` 或 `all` 的进程会响应这些端点，其他联邦宇宙服务器用它们来查找和显示你的账号。

| 路径 | 内容 |
| --- | --- |
| `GET /.well-known/webfinger` | WebFinger（RFC 7033）。`resource` 为 `acct:alice@{account_domain}`（`acct:alice@{host}` 也可以）、Actor 的 `id` 或 `/@alice` URL。`subject` 始终是 `account_domain` 一侧的用户名 |
| `GET /.well-known/host-meta`、`/.well-known/host-meta.json` | WebFinger 的 URI 模板（XRD / JRD） |
| `GET /.well-known/nodeinfo`、`GET /nodeinfo/2.1` | NodeInfo 2.1。`metadata.sourceCode` 为设置 `source_url`，对于 `https:` URL 还会设置 `software.repository`（参见[公开源代码](#publish-the-source-code-agpl-30)） |
| `GET /ap/actors/{uuid}` | Actor（`publicKey` 为 RSA 的 `#main-key`）。偏好 HTML 的请求会得到指向 `/@{username}` 的 303 |
| `GET /ap/actors/{uuid}/{outbox,followers,following,featured,featured-tags}` | 集合 |
| `POST /ap/actors/{uuid}/inbox`、`POST /ap/inbox` | 每个本站 actor 的收件箱和共享收件箱。请求必须带有 HTTP 签名 |
| `GET /@{username}` | 对于 ActivityPub 的 `Accept`，返回指向 Actor 的 `id` 的 302；否则返回网页客户端的 HTML（见下文） |
| `GET /@{username}/status/{uuid}` | 对于 ActivityPub 的 `Accept`，如果未登录的访客可以看到该帖子，则返回指向帖子 `id`（`/ap/objects/{uuid}`）的 302，否则返回 404。其他情况返回网页客户端的 HTML |

- 实例 Actor 的用户名为 `{account_domain}@{account_domain}`（与 Mastodon 的形式相同）。
- 不存在或无法显示（已停用、已注销、已删除或远程）的账号，总是得到相同的 404（`Cache-Control: no-store`）。
- 匿名响应带有 `Cache-Control: public, max-age=60`。随 `Accept` 而变化的响应带有 `Vary: Accept`。对于无法处理 `Vary: Accept` 的 CDN，请把 `/ap/*` 和 `/@*` 排除在缓存之外。

<!-- source: separate-handle-and-delivery-domains b1c23d994d0e -->
### 分离用户名域名与投递域名

当 `account_domain` 与 `host` 不同时，对 `account_domain` 一侧的 `/.well-known/webfinger`、`/.well-known/host-meta` 和 `/.well-known/nodeinfo` 的请求会以 301 重定向到 `host`（带有 `Access-Control-Allow-Origin: *`）。如果你把发往 `account_domain` 的请求路由到这台服务器，服务器会自己返回该重定向。如果由其他服务器或反向代理来响应，请重定向到 `https://{host}`，并保留路径和查询字符串。
