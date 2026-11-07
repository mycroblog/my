<!-- source: put-a-cdn-in-front-of-media-delivery 20ee3d2cefe9 -->
# 在媒体分发前设置 CDN

在服务器前面设置 CDN（Cloudflare、Google Cloud CDN、CloudFront 等）后，可以由 CDN 分发媒体（`/media/` 和 `/media/proxy/`）。
由 CDN 缓存响应的请求不会占用服务器的 CPU 和流量。
本指南介绍以服务器作为 CDN 源站（回源拉取）的配置。它适用于所有 blob 存储：使用 `blob = s3` 或 `blob = gcs` 时，服务器
从私有存储桶读取文件，并像 `blob = fs` 一样在 `/media/` 下提供，因此存储桶永远不会成为 CDN 的源站。
当整个服务器都位于 CDN 之后时，还可以[拒绝未经 CDN 的请求](#refuse-requests-that-did-not-come-through-the-cdn)。

<!-- source: configure-it e915ce01da94 -->
## 配置

将 `media_origin`（`MY_MEDIA_ORIGIN`）设置为 CDN 的主机名（例如 `media.example.org`）。

```sh
MY_MEDIA_ORIGIN=https://media.example.org
```

- 该值只能是源（`scheme://host[:port]`），不带路径、查询字符串或末尾斜杠。必须使用 `https`，只有 `localhost` 等回环主机才允许使用 `http`。值无效会导致启动失败。
- 不设置时，媒体从服务器自身的源（`https://{host}`）提供。
- 以下 URL 使用这个源：本地媒体为 `{media_origin}/media/...`，远程媒体的代理为 `{media_origin}/media/proxy/...`。
  客户端 API、ActivityPub（`attachment`、`icon`、`image` 以及自定义表情的 `icon`）和链接预览（OGP）输出的所有 URL 都包括在内。
- Web 客户端的 Content-Security-Policy 会将这个源加入 `img-src` 和 `media-src`。
- 无论 `media_origin` 是什么，联邦 URI（例如 ActivityPub 对象的 `id`）都保持在 `https://{host}/` 下。改变的只是文件的提供位置，而不是标识符。
- URL 在每次读取时生成，不存储在数据库中，因此修改 `media_origin` 也会改变现有媒体的 URL。
  只要旧源上的 `/media/` 继续提供服务，远程服务器保存的旧 URL 就仍然可用。

<!-- source: point-the-cdn-at-the-server ffb279f38f08 -->
## 让 CDN 指向服务器

- 将源站（后端）设为服务器的公开主机名（`https://{host}`）。CDN 只需从服务器获取 `/media/` 下的路径。
- 服务器不按主机名限制 `/media/` 和 `/media/proxy/`，无论 CDN 发送什么 `Host` 都会响应。
- `/media/` 无需认证，Authorized Fetch 也不适用于它。服务器会照常响应来自 CDN 的未签名请求。
- 对于远程媒体（`/media/proxy/{id}/{variant}?sig=...`），`sig` 是 URL 的一部分。**请将查询字符串纳入缓存键**。否则签名不同的 URL 会共用同一个缓存条目。
- 缓存时长由源站的 `Cache-Control` 决定（不要在 CDN 上覆盖它）。

| 路径 | `Cache-Control` |
| --- | --- |
| `/media/{...}`（本地媒体） | `public, max-age=31536000, immutable`（一年） |
| `/media/proxy/{...}`（远程媒体） | `public, max-age=` 实例设置 `remote_media_cache_ttl_seconds` 的秒数 |

两者都带有 `Cross-Origin-Resource-Policy: cross-origin` 返回，因此其他源的页面也能读取。

在 Cloud Run 上，`ingress-cloudflare-media` 会用 OpenTofu 完成这些配置：见[在 Cloud Run 上部署](deploy-cloud-run.md#cloudflare)。

<!-- source: who-can-open-media-urls 27306b0bc9be -->
## 谁能打开媒体 URL

媒体 URL 没有访问控制。无论附带它的帖子可见范围如何，任何拿到文件 URL 的人都能打开它，无需登录。
无论是否使用 CDN 都是如此，仅关注者帖子和私信的附件以及远程媒体的代理 URL 也不例外。

- 这些 URL 很难猜到：本地文件的 URL 包含带随机部分的标识符，代理 URL 带有签名。知道一个 URL 并不会泄露其他 URL。
- 看过帖子的人都可以把其中的媒体 URL 转给别人，这些 URL 对别人同样有效。
- 响应被标记为 `public`，因此 CDN 以及服务器和读者之间的任何共享缓存都可能保留副本（保留多久见上表）。
  正因如此，CDN 才能在不询问服务器的情况下分发媒体。

请告诉你的用户：非公开帖子限制的是谁能看到帖子，但无法阻止拿到附件链接的人打开文件。

<!-- source: deletion-and-the-cache baac405df923 -->
## 删除与缓存

删除媒体后，CDN 的缓存会在 `max-age` 期间内继续保留它。已删除的文件可能仍然可见：本地媒体最长一年，代理最长 `remote_media_cache_ttl_seconds`。
服务器不会清除 CDN 的缓存。如果某些内容必须在删除或审核操作后立即消失，请在 CDN 的控制台或 API 中手动清除。

<!-- source: service-worker f6efdae181a5 -->
## Service Worker

Web 客户端的 Service Worker 不缓存来自其他源的媒体，而是交给浏览器的网络处理，因此只有 CDN 的缓存起作用。

<!-- source: refuse-requests-that-did-not-come-through-the-cdn 5df4225c62b3 -->
## 拒绝未经 CDN 的请求

CDN 只能保护服务器免受经过它的请求的影响。如果源站本身的地址也能访问（例如 Cloud Run 上接受互联网流量的 `*.run.app`），任何人都可以绕过 WAF 和缓存直接调用它。
如果 CDN 的地址在 `trusted_proxies` 中，那么在 CDN 自己的无服务器平台上运行的调用方（例如某人自己的 Cloudflare Worker）也会从受信任的地址连接，并能自行决定速率限制所看到的客户端 IP。

下面三项设置可以堵住这个漏洞。CDN 会为它转发的每个请求加上一个秘密请求头，服务器对不带该请求头的请求一律在做任何其他处理之前返回 `403`。

![开启源站检查时请求的路径：浏览器、应用和其他服务器经过 CDN，由 CDN 加上秘密请求头；平台和其他任何人不带该请求头直接到达服务器；服务器的源站检查放行带请求头的请求和豁免的请求，其余以 403 拒绝。下面的列表逐一说明各条路径。](images/cdn-request-flow.svg)

- 浏览器和应用（页面、客户端 API、OAuth 和媒体 URL）以及其他服务器（ActivityPub 投递和获取）
  通过 `host` 和 `media_origin` 的主机访问 CDN。CDN 自行响应已缓存的 `/media/` 文件，并在
  转发给服务器的每个请求上将 `origin_secret_header` 设为 `origin_secret`。
- 平台（健康探测、可用性监控、Cloud Tasks 和 Cloud Scheduler）以及其他任何人（例如你自己的脚本或
  webhook）不带该请求头，直接访问服务器自身的地址。
- 源站检查在服务器中最先运行。它放行带有一次该请求头、且值为 `origin_secret` 或
  `origin_secret_previous` 的请求；不带该请求头时，只放行健康检查的 `GET` 和 `HEAD` 以及发往 `/_internal/` 的 `POST`。
  它会从每个请求中移除该请求头，并以 `403` 拒绝其余请求。
- 通过检查的请求会到达 Web 客户端、客户端 API、OAuth、ActivityPub、健康检查或 `/_internal/`，
  或者到达 `/media/` 和 `/media/proxy/`，它们读取媒体存储（`blob = fs`、`s3` 或 `gcs`）或中转
  其他服务器的文件。被拒绝的请求会收到带 `Cache-Control: no-store` 的 `403`，并计入 `my.http.origin_refused`。

当服务器有一个公开地址、而平台无法将其限制为只允许 CDN 访问时，请使用此功能。如果防火墙或平台已经限制了谁能连接，就不需要它，例如服务器只在你的负载均衡器后面的私有网络上监听。
如果该负载均衡器自己检查 CDN 的请求头并只接纳 CDN 的地址，也不需要它，例如位于 CloudFront 后面的 AWS Application Load Balancer 可以用监听器规则和托管前缀列表 `com.amazonaws.global.cloudfront.origin-facing` 做到这一点。此时请在 `trusted_proxies` 中同时列出负载均衡器和 CDN 的地址，因为两者都会追加 `X-Forwarded-For`。

如果流量由平台自身的功能转发、且不会添加你能控制的请求头（例如 Cloud Run 的域名映射），此功能就帮不上忙。只有能添加请求头的 CDN 或负载均衡器才行。

| 设置 | 内容 |
| --- | --- |
| [`origin_secret`](../reference/configuration.md#origin_secret)（`MY_ORIGIN_SECRET`） | 值，至少 32 字节的可见 ASCII 字符。属于机密，从环境变量或 `MY_ORIGIN_SECRET_FILE` 读取 |
| [`origin_secret_header`](../reference/configuration.md#origin_secret_header)（`MY_ORIGIN_SECRET_HEADER`） | 请求头的名称，由你自己决定。与 `origin_secret` 一起设置时必填。CDN 拒绝设置的名称，或服务器、代理会读取的名称（例如以 `cf-` 或 `x-cf-` 开头的名称）均无效 |
| [`origin_secret_previous`](../reference/configuration.md#origin_secret_previous)（`MY_ORIGIN_SECRET_PREVIOUS`） | 第二个可接受的值，仅在[轮换期间](#rotate-the-value)使用 |

三项都不设置时，不进行任何检查，服务器的行为就像本节不存在一样。

<!-- source: what-passes-without-the-header 38e1ceda6e34 -->
### 不带请求头也能通过的请求

| 请求 | 原因 |
| --- | --- |
| `/healthz`、`/readyz`、`/api/v1/instance/health` 的 `GET` 或 `HEAD` | 平台的探测和可用性监控会直接调用它们 |
| 发往 `/_internal/` 路径的 `POST` | Cloud Tasks 和 Cloud Scheduler 通过服务器自身的 URL 调用它们。这些请求带有自己的签名令牌 |

其他所有请求都需要该请求头，维护模式下也是如此：Web 客户端及其资源、`/media/`、客户端 API、OAuth 和 ActivityPub（包括其他服务器的投递）。
因此，任何不经 CDN、而是通过服务器自身地址直接调用它的东西都会被拒绝：你自己的 webhook、语音媒体服务器、脚本。请让它们指向 CDN 的主机名。

指标端口（`metrics_listen`）是一个独立的监听器，不对外公开，也不受检查。

不带秘密值的请求即使通过检查，也无法影响后续处理：无论 `X-Forwarded-For` 怎么写，它的客户端 IP 都是连接的对端，它的 `traceparent` 和 `X-Request-Id` 也会被忽略。

<!-- source: set-it-up ee0a17ad2d33 -->
### 设置方法

CDN 转发给服务器的每个主机名都需要该请求头：`host`、`media_origin` 的主机，以及 CDN 提供服务时单独设置的 `account_domain`。由同一个 CDN 代理的媒体主机也要设置，因为 `/media/` 和其他请求一样会被拒绝。

1. 生成值，并选择一个没有被其他地方使用的名称：

   ```sh
   openssl rand -hex 32
   ```

2. **先在 CDN 上设置请求头**。服务器会忽略它不认识的请求头，因此这一步暂时不会带来任何变化。
   - **Cloudflare**：请求头转换规则（Rules → Transform Rules → Modify Request Header），所有套餐都可用。匹配该区域的所有传入请求，选择 **Set static** 并填入你的名称和值。它会替换客户端发送的值。只针对部分主机名的规则会把其他主机名锁在外面。
   - **Amazon CloudFront**：在每个分配的源上设置源自定义请求头。它同样会替换查看者发送的值。
   - **Google Cloud 负载均衡器**：在后端服务上设置自定义请求头。
   - 其他 CDN 或代理：必须在发往服务器的每个请求上设置（而不是追加）该请求头。
3. 设置 `MY_ORIGIN_SECRET` 和 `MY_ORIGIN_SECRET_HEADER` 并部署。`my config check` 会验证它们并复述将检查的内容，但它看不到 CDN。
4. 从外部用源站本身的地址检查：

   ```sh
   curl -s -o /dev/null -w '%{http_code}\n' https://<host>/                 # 200: through the CDN
   curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/       # 403: directly
   curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/healthz # 200: a health check
   ```

还要在 [`trusted_proxies`](../reference/configuration.md#trusted_proxies) 中列出 CDN 的地址，否则服务器只会看到一个客户端，所有请求共用 CDN 的速率限制；`my config check` 会对此发出警告。

无论请求头错在哪里，被拒绝的请求都会收到与其他禁止访问的请求相同的 `403` 问题文档，并带有 `Cache-Control: no-store`。响应中不会提及该请求头。

<!-- source: when-everyone-is-locked-out 47c8f706e5e7 -->
### 所有人都被锁在外面时

配置错误时，所有请求都会被拒绝，而健康检查仍返回 `200`，因此平台认为服务器运行正常，监控 `/healthz` 的可用性监控也察觉不到任何异常。迹象如下：

- 经过 CDN 的所有页面都返回 `403`。
- 指标 `my.http.origin_refused` 上升，而 `http.server.request.duration` 的请求停止。
- 日志中出现状态为 `403` 且没有 `http.route` 的 `http request` 行。日志和追踪中从不包含该请求头及其名称或值。被拒绝的请求不会创建追踪 span。

要恢复，请在关闭检查的状态下部署：取消设置 `MY_ORIGIN_SECRET` 和 `MY_ORIGIN_SECRET_HEADER`。其他服务器会重试投递，但在此期间人们无法访问服务器。
然后对照设置检查 CDN 上的名称和值，确认规则覆盖了每个主机名，再重新设置。

<!-- source: rotate-the-value b88f695fde31 -->
### 轮换值

CDN 和服务器无法在同一时刻变更，服务器的每个副本也会按各自的节奏重启，因此如果只接受一个值，变更期间只要两边不一致，流量就会被拒绝。所以服务器在轮换期间接受两个值：

1. 将 `MY_ORIGIN_SECRET` 设为新值，`MY_ORIGIN_SECRET_PREVIOUS` 设为旧值，然后部署。两个值都能通过。
2. 修改 CDN 上的值，并等待变更传播到它的所有节点。
3. 取消设置 `MY_ORIGIN_SECRET_PREVIOUS` 并部署。设置该项期间，`my config check` 会发出警告。

请求头的名称不能这样轮换。要更改名称，请在暂时关闭服务器检查的情况下重复[设置方法](#set-it-up)，或者在部署新名称之前先在 CDN 上同时设置新旧两个名称。
