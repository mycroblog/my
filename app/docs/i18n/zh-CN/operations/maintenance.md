<!-- source: maintenance 46ba28c229dd -->
# 维护

如何为数据库、KV 或队列的维护工作停止服务器，以及停止期间显示什么。

有两种工具：

- **维护模式**（`maintenance = true`、`MY_MAINTENANCE=true` 或 `my serve -maintenance`）。`my serve` 启动时不打开任何适配器，对每个请求都返回 `503`、`Retry-After` 以及各客户端都能理解的响应。只要 `my serve` 本身能够运行，就使用它。
- **导出的页面**（`my maintenance export`）。由同一模板生成的静态 HTML，供负载均衡器、CDN 或反向代理在 `my serve` 完全无法运行时（例如正在替换镜像，或者某个修订版本无法启动）提供。参见[导出维护页面](admin-cli.md#exporting-the-maintenance-page)。

<!-- source: what-the-maintenance-mode-answers 05dd30158a8e -->
## 维护模式的响应

进程只读取监听地址、`host`、`account_domain`、`web_origin`、代理（`trusted_proxies`、`trusted_proxy_header`）、
[来源检查](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)（`origin_secret`、`origin_secret_previous`、`origin_secret_header`）、`status_page_url` 以及 `maintenance_*` 设置（[配置参考](../reference/configuration.md)）。数据库、队列、blob 存储、KV、邮件和流的设置既不读取也不检查，因此这期间它们即使有误或无法访问也没关系。无论 `role` 是什么，它都只提供 HTTP：不运行任务、定期任务，也不运行流代理。
`my config check -maintenance` 检查的正是维护模式所读取的内容。

如果设置了 [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)，需要该头部却缺少它的请求会得到该检查的 `403`，而不是下表中的响应。

| 请求 | 状态 | 正文 |
| --- | --- | --- |
| `GET /healthz`、`GET /readyz` | `200` | `{"status":"maintenance"}` |
| `GET /api/v1/instance/health` | `503` | `{"status":"maintenance"}` |
| 客户端 API 的其余部分（`/api/*`）以及返回 JSON 的 OAuth 端点 | `503` | 类型为 `/api/v1/problems/maintenance` 的 `application/problem+json`，包含 `message`、`until`，设置时还包含 `status_page_url` |
| ActivityPub（按路径、`Accept` 或 `Content-Type` 判断）、WebFinger、host-meta、NodeInfo、支付 Webhook、Cloud Tasks 和 Cloud Scheduler 的调用（`/_internal/*`） | `503` | `{"error":"maintenance"}` |
| 其他所有请求（页面、Web 客户端及其资源、表单提交） | `503` | 维护页面；`Accept` 要求 JSON 时则为 problem 响应 |

每个响应都带有 `Retry-After` 和 `Cache-Control: no-store`。在 `maintenance_until` 尚未到来时，`Retry-After` 为距离它的秒数（最多 86,400）；否则为 `maintenance_retry_after`（Go 的 duration，默认 `10m`）。

页面和 problem 响应显示以下内容：

| 设置 | 显示为 |
| --- | --- |
| `maintenance_server_name` | 服务器名称。未设置时为主机名（实例自己的名称保存在数据库中） |
| `maintenance_message` | 纯文本通知，最多 2,000 个字符，所有语言相同 |
| `maintenance_until` | 预计工作结束的时间（RFC 3339）。页面以 UTC 显示；Web 客户端以用户的时区显示 |
| `status_page_url` | 指向状态页面的链接（[监控](monitoring.md#a-status-page)） |

`maintenance_*` 设置在正常运行期间可以保持设置：只有在 `maintenance` 为 true 时才会使用它们。页面上的文字依据 `Accept-Language`（否则为英语）。

当 API 返回维护 problem 响应时，Web 客户端会显示全屏的维护通知，保持用户的登录状态，并在 `Retry-After` 之后重试。

<!-- source: how-long a7b6a44f0269 -->
## 时长

请把维护控制在**几个小时以内**。其他服务器在收到 `503` 时会重试投递，但它们都不读取 `Retry-After`，重试用完后，这期间发送的活动对本服务器来说就丢失了。主要实现中最短的重试窗口大约为一天（根据它们的重试计划，Akkoma 约 1 天，Misskey 约 1.5 天，Mastodon 约 2–3 天，Pleroma 约 7 天）。搜索引擎会把短时间的 `503` 视为临时故障，但 Google 建议不要让 `robots.txt` 返回 `503` 超过一两天。停止超过一两天就不是维护了：请改为关闭服务（[关闭服务](service-closure.md)）。

<!-- source: steps 1427d2c6d499 -->
## 步骤

1. **发布通知。** 提前一段时间在状态页面（`status_page_url`）上和公告（“管理”→“公告”）中发布维护时间段。在部署的配置中设置 `maintenance_message` 和 `maintenance_until`；如果前端提供导出的页面，请刷新它们（`my maintenance export`）。
2. **停止推送队列**（Cloud Tasks），参见[下文](#push-queues)。否则维护期间的每次投递都会得到 `503`，并消耗该任务的一次尝试机会。
3. **切换到维护模式**：以 `MY_MAINTENANCE=true` 重新部署（各部署目标的做法见下文）。所有副本都会以维护模式重启；由于 `/readyz` 返回 `200`，负载均衡器会保留它们。
4. **进行维护工作。**
5. **切换回来**：不带 `MY_MAINTENANCE`（或设为 `false`）重新部署，确认 `/readyz` 和 `/api/v1/instance/health` 返回 `{"status":"ok"}`，然后恢复推送队列并结束公告。

![按请求种类显示服务器在维护模式之前、期间和之后的响应：之前和之后照常响应，期间返回带 Retry-After 的 503，但 /healthz 和 /readyz 保持 200。下面的列表说明了同样的内容。](images/maintenance.svg)

从左到右：

- **之前**，在你发布维护通知期间，所有请求照常响应：`/healthz`、`/readyz` 和
  `/api/v1/instance/health` 返回 `200` 和 `{"status":"ok"}`。
- **维护模式下**（`MY_MAINTENANCE=true`）：
  - `/healthz` 和 `/readyz` 返回 `200` 和 `{"status":"maintenance"}`。
  - `/api/v1/instance/health` 返回 `503` 和 `{"status":"maintenance"}`。
  - 客户端 API 的其余部分以及返回 JSON 的 OAuth 端点得到 `503` 和维护 problem 响应。
  - ActivityPub、WebFinger 和 NodeInfo 返回 `503` 和 `{"error":"maintenance"}`。
  - 页面和 Web 客户端得到 `503` 和维护页面。
  - 每个响应都带有 `Retry-After` 和 `Cache-Control: no-store`。
- **之后**，你不带 `MY_MAINTENANCE` 重新部署后，所有请求再次照常响应。

<!-- source: push-queues 0d0d1fc56a29 -->
### 推送队列

- **Cloud Tasks**：暂停队列（队列仍接受新任务，但停止投递），维护完成后恢复。一个任务有多少次尝试机会，由队列的 `retryConfig.maxAttempts` 决定。

  ```sh
  gcloud tasks queues pause my-jobs --location=asia-northeast1
  gcloud tasks queues resume my-jobs --location=asia-northeast1
  ```

  同时暂停 `/_internal/tick` 的 Cloud Scheduler 作业（`gcloud scheduler jobs pause` / `resume`），或者让它的调用在这期间以 `503` 失败。任务本身保存在数据库中：用完尝试机会的任务只会丢失它的信号，维护结束后的第一次 tick 会再次通知所有到期的任务。

<!-- source: per-target 4e060394be4b -->
## 各部署目标

下文中前端的错误页面，会用导出的页面来响应服务器的错误。只匹配服务器未运行时的错误（`502` 和 `504`，以及负载均衡器在没有健康后端时自己返回的 `503`），或者只在 `my serve` 无法运行期间启用该规则：对所有 `503` 生效的规则也会替换维护模式自己的响应，例如客户端 API 和 Web 客户端读取的 problem 响应。

<!-- source: cloud-run b728619281de -->
### Cloud Run

用该变量部署新修订版本，之后再删除它：

```sh
gcloud run services update my --region=asia-northeast1 \
  --update-env-vars=MY_MAINTENANCE=true,MY_MAINTENANCE_UNTIL=2026-10-24T03:00:00Z
gcloud run services update my --region=asia-northeast1 --remove-env-vars=MY_MAINTENANCE,MY_MAINTENANCE_UNTIL
```

如果 Cloud Run 不经负载均衡器直接公开，它前面就没有任何东西，因此维护模式是唯一的工具。

**`my serve` 无法运行时**，如果它位于全球外部应用负载均衡器之后：把导出的 `index.html` 上传到作为后端存储桶添加的 Cloud Storage 存储桶，并添加自定义错误响应策略，用它来响应后端的 `502` 和 `504`，并设置 `overrideResponseCode: 503`。只有全球外部应用负载均衡器支持自定义错误响应（区域级和传统版都不支持），并且该策略只适用于来自后端的响应，不适用于 Google Front End 自己生成的响应。这种方式无法发送 `Retry-After`：错误来源的自定义头部不会被添加。

```yaml
# Part of the URL map (gcloud compute url-maps import).
defaultCustomErrorResponsePolicy:
  errorService: https://www.googleapis.com/compute/v1/projects/<project>/global/backendBuckets/my-maintenance
  errorResponseRules:
    - matchResponseCodes: ["502", "504"]
      path: /index.html
      overrideResponseCode: 503
```

<!-- source: aws-ecs 8c6aa496b517 -->
### AWS ECS

在容器的环境中加入 `MY_MAINTENANCE=true`，为服务的任务定义注册一个修订版本，并把服务更新到该版本；维护结束后，以不带该变量的方式再做一次。使用 OpenTofu 模块时，`deploy/opentofu/upgrade.sh --platform aws-ecs --maintenance`
会在升级之前把服务切换到维护模式，并用新修订版本切换回来。

**`my serve` 无法运行时**，如果它位于 CloudFront 之后：把导出的 `index.html` 上传到作为第二个源的 S3 存储桶（错误页面不能来自出错的源），并为 `502` 和 `504`（以及只在 `my serve` 无法运行期间的 `503`）添加自定义错误响应，设置 `ResponsePagePath: /maintenance/index.html` 和 `ResponseCode: 503`。
CloudFront 可以通过带自定义头部的响应头策略添加 `Retry-After`，但该策略适用于缓存行为的所有响应，因此只在维护期间附加它，之后再解除。

应用负载均衡器的固定响应无法承载导出的页面：其正文最多 1,024 个字符，并且不设置 `Retry-After` 等头部。只用它返回简短的文字和状态：

```json
{
  "Type": "fixed-response",
  "FixedResponseConfig": {
    "StatusCode": "503",
    "ContentType": "text/plain",
    "MessageBody": "This server is under maintenance. Please try again later. / メンテナンス中です。しばらくしてからお試しください。"
  }
}
```

没有 `Retry-After` 时，其他服务器会按自己的计划重试，上文的时间窗口已经考虑到了这一点。

<!-- source: docker-compose d564c0b39ae8 -->
### Docker Compose

`deploy/docker/compose.yaml` 会把 `.env` 中的设置传给服务器，并且也会从 shell 获取 `MY_MAINTENANCE`、`MY_MAINTENANCE_MESSAGE` 和 `MY_MAINTENANCE_UNTIL`。设置它们并重新创建 `my` 服务，维护结束后取消设置 `MY_MAINTENANCE`，再重新创建一次。
`--no-deps` 可以阻止一次性的数据库服务（`upgrade-check`、`schema`、`upgrade`）运行；升级时才运行它们（[升级](database.md#upgrading)）：

```sh
MY_MAINTENANCE=true MY_MAINTENANCE_UNTIL=2026-10-24T03:00:00Z docker compose up -d --no-deps --force-recreate my
docker compose up -d --no-deps --force-recreate my
```

**`my serve` 无法运行时**，如果它位于 nginx 之后：从一个目录提供导出的页面，按 `Accept-Language` 选择其中之一，并用它以 `503` 响应 nginx 自己的 `502` 和 `504`（服务器已停止或没有响应）。由于 `proxy_intercept_errors` 保持关闭，维护模式自己的 `503` 会原样通过。

```nginx
map $http_accept_language $maintenance_page {
    default  en.html;
    ~*^ja    ja.html;
}

server {
    # ...
    location / {
        proxy_pass http://my:8080;
        error_page 502 504 =503 /maintenance/$maintenance_page;
    }
    location /maintenance/ {
        internal;
        alias /srv/maintenance/;   # the -out directory of my maintenance export
        add_header Retry-After 600 always;
        add_header Cache-Control no-store always;
    }
}
```

<!-- source: microk8s-kubernetes a831623b2454 -->
### microk8s（Kubernetes）

在 Deployment 上设置该变量（这会滚动更新 pod），之后再删除它：

```sh
microk8s kubectl set env deployment/my MY_MAINTENANCE=true MY_MAINTENANCE_UNTIL=2026-10-24T03:00:00Z
microk8s kubectl rollout status deployment/my
microk8s kubectl set env deployment/my MY_MAINTENANCE- MY_MAINTENANCE_UNTIL-
```

维护模式下，`/readyz` 上的就绪探针会继续通过，因此滚动更新会完成，Service 也会继续把流量路由到这些 pod。

**`my serve` 无法运行时**，如果它位于 NGINX Ingress Controller（microk8s 的 `ingress` 插件）之后：用一个小型 `nginx` Deployment
提供导出的页面（例如来自包含 `index.html`、`en.html` 和 `ja.html` 的 ConfigMap），并把它设为自定义默认后端，让控制器用它来响应服务器的 `502` 和 `504`（只在 `my serve` 无法运行期间才加上 `503`）：

```yaml
metadata:
  annotations:
    nginx.ingress.kubernetes.io/custom-http-errors: "502,504"
    nginx.ingress.kubernetes.io/default-backend: my-maintenance   # a Service in the same namespace
```

<!-- source: monitoring-during-the-maintenance 91e64028447e -->
## 维护期间的监控

- `/healthz` 和 `/readyz` 保持返回 `200` 和 `{"status":"maintenance"}`：负载均衡器和 Kubernetes 会保留这些实例。
- `/api/v1/instance/health` 返回 `503` 和 `{"status":"maintenance"}`（[监控](monitoring.md)）。正常运行时间监控会认为服务器已停止，除非它们匹配正文或设置了维护时间窗口；请暂停它们或设置时间窗口。
