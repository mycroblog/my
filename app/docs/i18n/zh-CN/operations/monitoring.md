<!-- source: monitoring-the-server-from-outside 6cd7b2b809fa -->
# 从外部监控服务器

如何让正常运行时间监控服务指向服务器、如何避免监控推高运行成本，以及如何区分维护模式（[维护](maintenance.md)）和故障。探测路径见[运行指南中的健康检查一节](running.md#health-checks)。

下文中关于第三方的事实都附有确认日期和来源。各服务会更改其方案和界面；在依赖某项限制之前，请查看链接的页面。

<!-- source: which-url-to-monitor 50bdaa5c9f31 -->
## 监控哪个 URL

| 路径 | 用途 | 是否访问数据库 | 维护模式下 |
| --- | --- | --- | --- |
| `GET /api/v1/instance/health` | 正常运行时间监控、状态页面、Web 客户端 | 每个进程每 60 秒最多一次 | `503`，带 `Retry-After` 和 `{"status":"maintenance"}` |
| `GET /healthz` | 平台的存活探测 | 从不 | `200` `{"status":"maintenance"}` |
| `GET /readyz` | 负载均衡器或 Kubernetes 的就绪探测 | 每个请求都访问（并发请求共享一次检查） | `200` `{"status":"maintenance"}` |

请让外部监控指向 **`/api/v1/instance/health`**，而不是 `/readyz`：`/readyz` 每个请求都会查询数据库，因此间隔很短的监控会让可缩容到零的数据库一直保持唤醒。探测是给你的平台用的。模块 [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) 和 [`monitoring-aws`](../../deploy/opentofu/modules/monitoring-aws/README.md) 会以较长的间隔从平台自己的监控检查 `/readyz`，其 README 说明了相应的费用。

`/api/v1/instance/health` 的响应（无需认证，`Cache-Control: no-store`）：

| 状态 | HTTP | 正文 |
| --- | --- | --- |
| 健康 | `200` | `{"status":"ok","checked_at":"<RFC 3339>"}` |
| 不可用（无法连接数据库，或尚未应用 schema） | `503` | `{"status":"unavailable","checked_at":"<RFC 3339>"}` |
| 维护模式 | `503`，带 `Retry-After` | `{"status":"maintenance"}` |

健康时，`/healthz` 和 `/readyz` 以 `200` 回答 `{"status":"ok"}`；数据库未就绪时，`/readyz` 以 `503` 回答一个 problem 文档。正文从不包含原因、主机名或版本。原因记录在服务器日志中。

`checked_at` 是该响应所依据的检查时间。服务器每个进程每分钟最多运行一次数据库检查，并复用其结果，因此轮询频率高于每 60 秒一次的监控只会看到相同的 `checked_at`，没有任何收益。有多个进程时，每个进程各自运行检查。

设置了 [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) 时，这三个路径即使没有该请求头也会回答，因此它们无法反映 CDN 丢失了该请求头。如果监控是你发现问题的第一信号，请同时监控一个经过 CDN 的页面，例如 `https://<host>/about`。

<!-- source: keep-the-monitoring-cheap 29d1da3d6dc2 -->
## 让监控保持低成本

- **可缩容到零的数据库。** 如果监控的间隔短于数据库停止前的等待时间，数据库就永远不会停止。Neon 会在 5 分钟无活动后挂起计算实例；Free 方案无法关闭这一行为（2026-10-25 确认，[Neon: Scale to zero](https://neon.com/docs/introduction/scale-to-zero)）。一般来说，5 分钟或更长的间隔能让空闲的数据库在两次检查之间停止，而每次检查会唤醒它，持续检查本身的时间加上空闲等待时间。参与联邦的服务器全天候接收来自其他服务器的投递，因此其数据库一天中大部分时间本来就处于唤醒状态；额外的成本仅限于安静的时段。
- **如果最在意空闲成本，可以选择更便宜的分工方式。** 以较短的间隔监控 `/healthz`（它从不访问数据库），以较长的间隔（15 分钟或更长）监控依赖数据库的 `/api/v1/instance/health`。`/healthz` 在维护模式下也回答 `200`，因此请匹配正文中的 `"status":"ok"`。
- **请求本身很小。** 每 5 分钟检查一次，每月约 8,640 个请求；每分钟一次约 43,200 个。这在 Cloud Run 的免费额度之内（每月 200 万个请求、180,000 vCPU 秒和 360,000 GiB 秒；时间向上取整到 100 毫秒；2026-10-23 确认，[Cloud Run pricing](https://cloud.google.com/run/pricing)）。

<!-- source: telling-the-maintenance-mode-from-an-outage 15833e5da3fc -->
## 区分维护模式和故障

在维护模式（[维护指南](maintenance.md)）期间，健康检查端点回答 `503`，大多数监控会把它当作故障。仅凭状态码无法区分两者，因此请根据正文判断：

- 把正文中有 `"status":"ok"` 视为健康，其他任何情况都视为宕机。关键字监控或 JSON 值监控可以做到这一点。
- 为了避免计划维护期间的告警，如果服务提供维护窗口，就设置监控的维护窗口（见下表），或者在开始前手动暂停监控。
- UptimeRobot 的 Free 方案没有维护窗口：请暂停监控，或者接受告警。

<!-- source: setting-up-the-monitors e91307d78a94 -->
## 设置监控

以下服务检查的都是同一件事：当 `GET https://<your-host>/api/v1/instance/health` 的响应为 `200` 且正文包含 `"status":"ok"` 时即为健康。

| 服务 | 监控类型 | 健康条件 | 维护窗口 | 确认 |
| --- | --- | --- | --- | --- |
| UptimeRobot | Keyword | 正文包含 `"status":"ok"` | Free 方案没有（Solo 及以上） | 2026-10-25，[pricing](https://uptimerobot.com/pricing.md) |
| Better Stack | HTTP status 或 Keyword | `2xx`，或正文包含 `"status":"ok"` | 未验证 | 2026-10-25 |
| Gatus | Endpoint conditions | `[STATUS] == 200` 且 `[BODY].status == ok` | 配置中的 `maintenance` | 2026-10-25，[README](https://github.com/TwiN/gatus) |
| Uptime Kuma | HTTP(s) - Keyword 或 JSON Query | 正文包含 `"status":"ok"`，或 `status` 为 `ok` | 内置 | 2026-10-23，[README](https://github.com/louislam/uptime-kuma) |
| Upptime | `.upptimerc.yml` 中的 `sites` | `200` 且正文包含 `"status":"ok"` | 无 | 2026-10-25，[Configuration](https://upptime.js.org/docs/configuration/) |

<!-- source: uptimerobot b2350308dfab -->
### UptimeRobot

创建一个类型为 **Keyword** 的监控：URL 为 `https://<your-host>/api/v1/instance/health`，关键字为 `"status":"ok"`，在关键字**不**存在时告警。Free 方案允许 50 个监控，间隔为 5 分钟，包含关键字监控。“Custom headers & statuses”和维护窗口从 Solo 方案（付费，间隔 60 秒）开始提供（2026-10-25 确认，[pricing](https://uptimerobot.com/pricing.md)）。由于检查针对的是正文，这里用关键字监控就足够了。

<!-- source: better-stack 68d7ade20dca -->
### Better Stack

两种方式都可以（2026-10-25 确认，[Uptime monitor](https://betterstack.com/docs/uptime/uptime-monitor/)、[Keyword monitor](https://betterstack.com/docs/uptime/keyword-monitor/)）：

- **HTTP status** 监控：它把 `2xx` 响应视为正常，因此 `200` 为正常，`503`（不可用或维护）会触发事件。
- **Keyword** 监控：“Alert us when”设为“URL doesn't contain keyword”，关键字为 `"status":"ok"`。匹配不区分大小写。

Free 方案的检查频率和维护窗口尚未验证：官方页面没有针对 Free 方案说明这些内容。免费额度的数量（监控和心跳合计 10 个，一个状态页面）来自[价格页面](https://betterstack.com/pricing)，2026-10-23 确认。

<!-- source: gatus 2aa06e9f2812 -->
### Gatus

在 `config.yaml` 中（默认间隔为 `60s`；2026-10-25 确认，[README](https://github.com/TwiN/gatus)）：

```yaml
endpoints:
  - name: my
    url: "https://<your-host>/api/v1/instance/health"
    interval: 5m
    conditions:
      - "[STATUS] == 200"
      - "[BODY].status == ok"
```

Gatus 有一个顶层的 `maintenance` 部分（`start` 为 `hh:mm` 格式，`duration`，`timezone` 默认为 `UTC`，`every` 为星期几的列表，为空表示每天），用于计划内的工作。README 只说明它用于在维护窗口期间避免告警；请在你使用的版本上确认其行为。

<!-- source: uptime-kuma 7dd44fb38cab -->
### Uptime Kuma

添加一个类型为 **HTTP(s) - Keyword**、关键字为 `"status":"ok"` 的监控，或者类型为 **HTTP(s) - Json Query**、查询为 `status`、期望值为 `ok` 的监控。接受的状态码默认为 `200-299`，因此维护时的 `503` 和故障时的 `503` 都会被视为宕机。Uptime Kuma 有一个用于设置监控窗口的维护功能（[wiki](https://github.com/louislam/uptime-kuma/wiki/Monitor-Types) 列出了它；其细节未经验证）。这些监控类型已于 2026-10-23 在源代码中确认（[monitor.js](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/server/model/monitor.js#L478-L720)、[EditMonitor.vue](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/src/pages/EditMonitor.vue#L3349)）。

<!-- source: upptime 90c96c946d06 -->
### Upptime

在 `.upptimerc.yml` 中（2026-10-25 确认，[Configuration](https://upptime.js.org/docs/configuration/)）：

```yaml
sites:
  - name: My server
    url: https://<your-host>/api/v1/instance/health
    __dangerous__body_down_if_text_missing: '"status":"ok"'
```

默认情况下，`20x` 和 `30x` 都算正常；设置 `expectedStatusCodes: [200]` 可以只接受 `200`。检查通过 GitHub Actions 按为所有站点统一设置的计划运行（`workflowSchedule.uptime`，默认每 5 分钟一次，这也是 GitHub Actions 允许的最短间隔）。该选项的名称表明不推荐使用它，但这是检查正文的方法。

<!-- source: a-status-page 64712ed6d8a5 -->
## 状态页面

把 `status_page_url`（`MY_STATUS_PAGE_URL`，一个 `https:` URL；参见[配置参考](../reference/configuration.md#status_page_url)）设为你的监控服务的公开页面。服务器会在 `GET /api/v1/instance`、它渲染的登录页面和错误页面、Web 客户端（包括服务器无响应时）以及维护页面中显示这个链接，这样无法访问服务器的用户也能了解原因。它从配置中读取，而不是从数据库读取，因此在数据库宕机时也能工作。修改在重启后生效。

页面本身的选择（本服务器不会导入状态页面的状态，因此也请以全服务器通知的形式发布公告）：

- 你所选监控服务的托管状态页面（UptimeRobot 的 Free 方案有一个不可自定义的状态页面；Better Stack 的免费额度有一个）、Gatus 和 Uptime Kuma 自己提供的页面，或者部署在 GitHub Pages 上的 Upptime 静态站点。
- Atlassian Statuspage，它是一个发布状态的地方，而不是监控服务。一个组件有五种状态之一：Operational、Under maintenance、Degraded performance、Partial outage、Major outage（API 取值为 `operational`、`under_maintenance`、`degraded_performance`、`partial_outage`、`major_outage`；2026-10-23 确认，[What is a component?](https://support.atlassian.com/statuspage/docs/what-is-a-component/)、[Statuspage API](https://developer.statuspage.io/)）。如何根据监控的告警自动更改组件的状态尚未验证（这需要在实际账号上设置针对各服务的集成）。
