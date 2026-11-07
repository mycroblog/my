<!-- source: production-checklist e5f6c0c5b10a -->
# 生产环境检查清单

在向用户和其他服务器开放服务器之前，请逐项检查这份清单；部署发生变化后也要再检查一遍。有些设置在服务器联邦之后就无法更改，而有些失误（例如丢失密钥加密密钥）根本无法挽回。

每一项都说明了它为什么重要、如何验证，以及详细信息在哪里。先从自动检查的部分开始：

```sh
my config check            # the configuration, and the settings that do not suit production
my config check -instance  # also the instance settings in the database
```

<!-- source: what-my-config-check-checks f5feea1abf22 -->
## `my config check` 检查什么

在 `production` 配置档（[`profile`](../reference/configuration.md#profile) 的默认值）下，`my config check` 先验证每一项设置，然后检查这些有效的设置是否适合生产实例。发现的问题分为两组列出，每组前面有一段使用进程区域设置语言的摘要，每个问题附有英文的技术细节：

- **错误**会让 `my config check` 以 1 退出。它们是会破坏生产实例或削弱其安全性的设置：
  - 其他服务器无法访问的 `host` 或 `account_domain`：带端口、`localhost`、不含点的名称，或者属于保留给本地使用或测试的顶级域名的名称（`.localhost`、`.local`、`.test`、`.example`、`.invalid`、`.internal`、`.home.arpa`）。
  - 使用 `http` 的 `web_origin`。
  - 把数据库放在内存中的 SQLite `database_url`。
  - 不是随机生成的 `kek`，例如全为零或由同一个字节重复组成。
  - 属于 CAPTCHA 服务公开的测试密钥的 `captcha` 站点密钥或密钥。
  - 指定了不存在的作业类型的 `worker_job_types`、`worker_job_types_except` 或队列路由（在所有配置档下都会检查；`my job types` 会列出作业类型）。
  - 使用 `-instance` 时，数据库连接池无法满足的 `worker_job_concurrency`。
- **警告**不改变退出码，仍为 0。它们是在开发中很常见、在生产中也可能是有意为之的设置：
  - `mail = none`。
  - SQLite 数据库或 `fs` blob 存储位于临时目录中（`/tmp`、`/var/tmp`、`/dev/shm` 或 `$TMPDIR`）。
  - `queue = inline`。
  - 没有 `vapid_private_key`。
  - `role` 不是 `all` 时使用 `stream = hub` 或 `kv = memory`：它们只在单个进程内有效。
  - 很可能并非本意的语音设置，例如没有 STUN 或 TURN 的 `voice = p2p`。
  - 位于 `http://` 地址的[插件](plugins.md)，或者位于 `https://` 地址但既没有令牌也没有客户端证书的插件。
  - 支付服务商的测试模式密钥。
  - 设置了 `origin_secret` 但 `trusted_proxies` 为空，以及轮换之后仍然保留着 `origin_secret_previous`。
  - 使用 `-instance` 时：未设置的实例文本 `name`、`short_description`、`terms` 和 `privacy_policy`，未设置的实例设置 `contact_account`、`contact_email` 和 `security_contact`，以及没有规则的服务器。无法连接数据库时，这本身算作一条警告，其余检查仍会继续。

`my serve` 启动时会把同样的发现记录为警告，但仍然会启动。`development` 配置档会跳过这些检查；它用于本地开发和测试，绝不要用于供他人使用的实例。

请在与服务器相同的环境中（相同的环境变量、密钥文件和配置文件）运行 `my config check`，例如作为部署的一个步骤，在新版本接收流量之前运行。

<!-- source: settings-you-cannot-change-later b2c961cb26f5 -->
## 之后无法更改的设置

<!-- source: the-servers-domain-and-the-handle-domain b01ce235bcc0 -->
### 服务器的域名和用户名的域名

- **原因**：`host` 是服务器每个 ActivityPub id 的一部分，而其他服务器会保存这些 id。`account_domain` 是用户名（`@alice@example.org`）的域名。第一个使用数据库的 `my serve` 或 `my` 命令会把两者记录到数据库中，此后只要配置与之不同，`my serve` 和这些命令就会拒绝运行：联邦之后再更改，会破坏所有关注，以及所有指向该服务器帖子和账号的引用。请在首次启动之前设置好两者。如果在联邦之前发现了错误，可以通过[重新创建数据库](database.md#recreating-erasing-every-row)来修正，这会删除所有行。
- **决定**：用户名使用服务器的域名还是另一个域名（例如用户名在 `example.org` 上，服务器在 `social.example.org` 上）。单独的用户名域名必须把它的 WebFinger、host-meta 和 NodeInfo 重定向到服务器。
- **验证**：`my config check` 会显示 `host` 和 `account_domain`，并报告其他服务器无法访问的域名。当它们与数据库中的值不同时，`my config check -instance` 也会失败，并且不记录任何内容。使用单独的用户名域名时，`curl -sI "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"` 会以 `301` 重定向到 `https://<host>/.well-known/webfinger?...`。
- **详情**：[运行服务器](running.md)（联邦端点，以及从另一个域名提供用户名），[`host`](../reference/configuration.md#host) 和 [`account_domain`](../reference/configuration.md#account_domain)。

<!-- source: reachable-over-https 1cf53c613f7b -->
### 可通过 `https` 访问

- **原因**：其他服务器通过 443 端口获取 `https://<host>/` 上的 id，而浏览器只通过 `https` 发送登录 cookie。
- **验证**：从你的网络外部访问时，`curl -sI https://<host>/healthz` 返回 `200`，且证书对 `host` 有效；`my config check` 没有报告关于 `web_origin` 的错误。`http://<host>/` 会重定向到 `https`，这由服务器前面的反向代理、负载均衡器或 CDN 完成（使用 Docker Compose 时，叠加文件 `compose.caddy.yaml` 就是这样的代理）。
- **详情**：[运行服务器](running.md)，[`web_origin`](../reference/configuration.md#web_origin)。

<!-- source: not-reachable-past-the-cdn 58310b9f29d7 -->
### 无法绕过 CDN 访问

- **原因**：当前面有 CDN，而服务器自身的地址仍然公开时（Cloud Run 上的 `*.run.app`），任何人都可以绕过 WAF 和缓存直接调用服务器，而位于 CDN 自身平台上的调用者可以自行选择速率限制所看到的客户端 IP。只有这种情况才需要这一项：如果防火墙或平台已经限制了谁能连接，就不需要；没有 CDN 时也没有可以绕过的东西。
- **验证**：已设置 `origin_secret` 和 `origin_secret_header`，且 CDN 在每个主机名上都设置该请求头。`curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/` 返回 `403`，它的 `/healthz` 返回 `200`，而 `https://<host>/` 返回 `200`。
- **详情**：[拒绝不经过 CDN 的请求](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)，其中包括所有人都被拒之门外时如何恢复。

<!-- source: secrets ec71f1f20574 -->
## 密钥与机密

<!-- source: the-key-encryption-key-kek 89df548c5bf9 -->
### 密钥加密密钥（KEK）

- **原因**：KEK 加密所有 actor 的私钥。没有它，服务器就无法签名任何内容，而且无法从数据库中恢复这些私钥：所有账号都必须重新创建，其他服务器也将不再信任它。
- **验证**：
  - 它是随机生成的（`openssl rand -base64 32`）；`my config check` 会报告不是随机生成的密钥。
  - 它保存在数据库及其备份以外的地方，例如密码管理器或云的密钥存储中，并且至少还有一位运行服务器的人能够取得它。
  - 它只通过 `MY_KEK` 或 `MY_KEK_FILE` 传给服务器（配置文件拒绝存放机密）。使用 Docker Compose 时，它是文件 `secrets/kek`，不在 `.env` 中（[Docker Compose 文件](running.md#docker-compose-files)）。
- **详情**：[密钥管理](key-management.md)。

<!-- source: the-vapid-key-of-web-push 76bc2a1dd030 -->
### Web Push 的 VAPID 密钥

- **原因**：没有它，服务器就不会发送 Web Push 通知。之后更改它会终止所有订阅，所以只生成一次，并像 KEK 一样妥善保管。
- **验证**：未设置 `vapid_private_key` 时，`my config check` 会发出警告。用 `my config vapid-key` 生成一个。
- **详情**：[密钥管理](key-management.md)（Web Push 的 VAPID 密钥）。

<!-- source: other-secrets-stay-out-of-the-repository-and-the-image 94f290afcdbb -->
### 其他机密不放进代码仓库和镜像

- **原因**：放在代码仓库、容器镜像或配置文件中的机密，会被复制到这些东西所到的每一个地方。
- **验证**：`database_url`、`mail_smtp_password`、`captcha_secret_key`、支付服务商的密钥、`valkey_url`，以及配置参考中标记为机密的其他所有设置，都来自由平台密钥存储（Docker 或 Kubernetes secret、Secret Manager、Parameter Store）填充的环境变量或 `<NAME>_FILE` 文件。配置文件不能存放它们：`my config check` 会拒绝其中的机密。使用 Docker Compose 时，它们来自只有你能读取的 `.env`（`chmod 600`）。在代码仓库和镜像中搜索这些值，确保没有留下副本。`my config check` 会对每个已设置的机密显示 `(redacted)`。
- **详情**：[配置参考](../reference/configuration.md)（Secret 列），[CAPTCHA](captcha.md)，[Stripe](payments-stripe.md)，[PAY.JP](payments-payjp.md)。

<!-- source: instance-settings baae9cc3e09e -->
## 实例设置

实例设置保存在数据库中，可以在服务器运行时通过服务器的管理页面或 `my instance set` 更改。`my config check -instance` 会对未设置的文本、联系方式和规则发出警告。

- **名称、简短描述和描述**：显示在首页、信息页面（`/about`）、链接预览和 NodeInfo 中。在 `/about` 上验证。
- **联系方式**：`contact_account`（用户就服务器事宜联系的账号）和 `contact_email`（用户和其他服务器写信的地址；服务器发出的邮件也会把回复发到这里）。在 `/about` 上验证。
- **漏洞联系方式**：`security_contact`，一个 `https:` 或 `mailto:` URI。设置后，服务器会响应 `/.well-known/security.txt`。用 `curl -s https://<host>/.well-known/security.txt` 验证。
- **服务条款和隐私政策**：`terms` 和 `privacy_policy`。隐私政策必须列出接收个人数据的服务，例如 CAPTCHA 服务和邮件服务商。验证 `/about` 和注册页面上的链接。
- **服务器规则**：用户注册时同意的内容，也是审核员执行的内容。用 `my instance rule create` 或在管理页面中添加，并在 `/about` 上验证。
- **注册模式**：在你更改之前，`registration_mode` 为 `approval`；根据人们加入的方式，从 `open`、`approval`、`invite` 或 `closed` 中选择。在 `mail = none` 时，`open` 的注册也要等待审核员批准。
- **搜索引擎和 AI**：在你允许之前，`search_indexing`、`ai_usage` 和 `indexable_default` 会拒绝两者。验证 `/robots.txt` 和 `/.well-known/tdmrep.json`。
- **状态页面**：把 [`status_page_url`](../reference/configuration.md#status_page_url)（这是一项设置而不是实例设置，所以维护期间也会显示该链接）设为位于服务器以外服务上的状态页面。

**详情**：[实例设置](instance-settings.md)，[注册](registration.md)，[搜索引擎和 AI 爬虫](crawlers.md)，[CAPTCHA](captcha.md)（隐私政策中要写什么），[隐私政策模板](privacy-policy-template.md)，[处理活动记录模板](processing-record-template.md)，[监控](monitoring.md)（状态页面）。

<!-- source: email 40c18999c46f -->
## 电子邮件

- **原因**：电子邮件用于验证地址，并发送密码重置邮件和通知。在 `mail = none` 时，没有任何邮件会送达用户。
- **验证**：
  - `mail` 为 `smtp`、`ses` 或 `plugin`，且 `mail_from` 是你所控制的域名上的地址；`my config check` 会对 `mail = none` 发出警告。
  - `mail_from` 的域名有 SPF、DKIM 和 DMARC 记录，且服务商使用 DKIM 签名。没有这些，邮件会进入垃圾邮件。
  - 用测试账号注册并收到验证邮件；请求重置密码，并且同样收到邮件。检查收到的邮件头中是否有 `spf=pass`、`dkim=pass` 和 `dmarc=pass`。
  - 使用 SES 时，账号已脱离沙盒，且退信和投诉通知能送达服务器。
- **详情**：[电子邮件](email.md)，[注册](registration.md)（电子邮件验证）。

<!-- source: administrators 2c05e6644d88 -->
## 管理员

- **原因**：管理员账号可以更改所有设置、读取私密数据，并停用任何人的账号。
- **验证**：
  - 每位管理员都已开启双重验证（Web 客户端中的“设置”、“双重验证”），并保存好恢复码。
  - 只有需要的人才拥有 `admin` 角色；审核员拥有 `moderator` 角色，或者只包含他们所用权限的角色。用 `my admin role` 列出他们，并在管理页面上进行审查。
- **详情**：[管理员和角色](admin-cli.md)，[审核](moderation.md)（权限和登录恢复）。

<!-- source: operations 56bd643c2241 -->
## 运维

- **备份和恢复**：数据库、blob 存储（`blob_fs_dir` 或存储桶）和 KEK 都按计划备份到与服务器分开的地方，并且已经演练过恢复到一台全新的服务器：恢复后的服务器能够启动，账号能够登录并发帖。从未恢复过的备份，无法知道它是否可用。数据库和媒体不能放在临时目录中；如果放在那里，`my config check` 会发出警告。请参阅[密钥管理](key-management.md)（备份 KEK）和[数据库](database.md)。
- **监控和告警**：由外部监控检查 `/healthz`，并向能够处理问题的人发出告警，同时关注作业队列中的死信作业。请参阅[监控](monitoring.md)和[运行服务器](running.md)（健康检查、指标和作业队列）。
- **镜像**：实例运行按摘要固定的已发布镜像（`ghcr.io/mycroblog/my:0.1.0@sha256:...`，见版本说明），而不是重启时会变化的镜像，并且[已验证其来源](running.md#verify-an-image)。镜像没有 `latest` 标签。升级时有意地更改该版本，并按照[升级](database.md#upgrading)的步骤，每次升级一个次版本。
- **源代码（AGPL-3.0）**：服务器必须向所有使用者提供其源代码。运行未经修改的版本时，让 [`source_url`](../reference/configuration.md#source_url) 保持默认值；运行分支或修改版本时，把它指向你自己的源代码。在 `/about` 上验证该链接。请参阅[运行服务器](running.md)（公开源代码）。
- **维护**：在需要之前就了解如何让服务器进入维护模式。请参阅[维护](maintenance.md)。

<!-- source: federation a5ab186ec04b -->
## 联邦

- **原因**：其他服务器通过 WebFinger 和 NodeInfo 查找账号，然后获取 actor。如果从外部访问时其中任何一项失败，服务器在联邦宇宙中就是不可见的。
- 从你的网络外部的机器上**验证**：
  - `curl -s "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"` 返回该账号；域名不同时，跟随重定向到 `host`（`curl -L`）。
  - `curl -s https://<host>/.well-known/nodeinfo` 链接到 `/nodeinfo/2.1`，其中写明了软件及其源代码。
  - `curl -s -H 'Accept: application/activity+json' https://<host>/@<user>` 重定向到 actor 的 id，而该 id 上的 actor 是带有 `publicKey` 的 JSON。
  - 从 Mastodon 或其他服务器上的账号搜索 `@<user>@<account_domain>`，关注它，并看到新帖子送达。
  - 服务器前面的 CDN 不缓存 `/ap/*` 和 `/@*`，除非它遵循 `Vary: Accept`。
- **详情**：[运行服务器](running.md)（联邦端点），[CDN](cdn.md)，[中继](relays.md)。
