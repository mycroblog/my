<!-- source: deploying-on-cloud-run 7728d9a37c56 -->
# 在 Cloud Run 上部署

本文介绍如何用 OpenTofu 模块 [`mycroblog-gcp`](../../deploy/opentofu/README.md) 在 Google Cloud 上运行 `my`：使用 Cloud Run、Cloud Tasks、Cloud Scheduler、Cloud Storage 和 Pub/Sub，数据库使用 Neon 上的 PostgreSQL，前面是全球外部应用负载均衡器。没有人使用时，服务不运行任何实例，只有一个每分钟唤醒服务器的 tick。运行服务器需要花钱：在存储、流量和打开的流之外，预计每月约 37 美元；用心配置可以减少哪些费用，请阅读[费用](#costs)。

该模块需要 OpenTofu 1.11 或更高版本。不支持 Terraform。

<!-- source: what-it-creates cae1e55a1722 -->
## 会创建什么

`mycroblog-gcp` 的一个块会创建下面所有内容。每一行是模块的一个部分（section），其默认值就是本文部署的内容；其他种类见[模块的 README](../../deploy/opentofu/README.md#sections)。

| 部分 | 区段 | 资源 |
| --- | --- | --- |
| 服务器 | `service`、`jobs` | 一个 `role = all` 的 Cloud Run 服务、一个迁移作业、需要时的控制台作业、一个服务账号，以及公开访问权限 |
| 流量 | `ingress` | 带证书的全球外部应用负载均衡器。域名映射或 Cloudflare 是另外的种类，Cloudflare 上的媒体主机可以与任何一种并用 |
| 任务 | `queue` | 一个 Cloud Tasks 队列、一个 Cloud Scheduler 作业，以及一个调用服务器的服务账号 |
| 媒体 | `media` | 一个私有存储桶 |
| 实时更新 | `stream` | 一个主题和一个自定义角色。服务器自行创建和删除自己的订阅 |
| 数据库 | `database` | 一个存放其 URL 的机密：数据库由你手动创建 |
| 邮件 | `mail` | 一个存放 SMTP 密码的机密：任何 SMTP 服务均可 |
| 机密 | | 一个自动生成的 VAPID 密钥，以及只有你能填写的、内容为空的机密 |
| KEK | `kek` | 一个以文件形式挂载的自动生成的机密。Cloud KMS 是另一种 |
| 指标、追踪、错误 | `observability` | Cloud Monitoring、Cloud Trace 和 Error Reporting 的 API，它们读取 Cloud Run 发送给它们的数据 |

机密值永远不会进入 OpenTofu 的计划或状态：来自其他地方的值由你自己添加，KEK 和 VAPID 密钥则由模块通过只写属性生成。

<!-- source: before-you-start 10846ce6f3b1 -->
## 开始之前

- `tofu` 1.11 或更高版本、`gcloud` 和 `jq`，装在你执行 apply 的地方：更改版本的 apply 会从那里通过 `gcloud` 运行迁移。
- 一个已启用结算、专供本实例使用的 Google Cloud 项目，以及该项目中拥有 Owner 角色的账号。
- 用于 [`host`](../reference/configuration.md#host) 的域名。它以后无法更改：请阅读[以后无法更改的设置](production-checklist.md#settings-you-cannot-change-later)。
- 来自[发布版本](running.md#get-the-image)的某个 `my` 版本。没有 `latest` 标签。

<!-- source: first-deployment 996fcc6eb35e -->
## 首次部署

<!-- source: 1-enable-the-apis c7f2ef4a9ee9 -->
### 1. 启用 API

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com \
  monitoring.googleapis.com
```

`compute.googleapis.com` 用于负载均衡器。

<!-- source: 2-create-the-database-on-neon ca5f51e8979e -->
### 2. 在 Neon 上创建数据库

请按 Launch 套餐来预期：使用默认的 tick 时，计算资源永远不会停止。Free 套餐也能用，但要接受[费用](#costs)中所述的取舍。

在 Neon 控制台中，在靠近你的 Cloud Run 区域的地区创建项目，并复制其连接字符串。模块从不创建它，因此其密码不会进入 OpenTofu 状态。一个项目有两个连接字符串，部署会把它们各用在合适的地方：

- **直连**字符串（主机名不带 `-pooler`）是必需的。迁移作业和控制台始终使用它：`my schema apply` 会持有会话级锁，而事务模式的连接池无法保持这种锁。
- **池化**字符串（主机名带 `-pooler`）是可选的，供服务使用。Neon 的连接池能满足服务器的需求：见[通过连接池](database.md#through-a-connection-pooler)。设置 `database = { pooled = true }`，模块就会为它创建第二个机密。

没有池化字符串时，服务也使用直连字符串，连接数受数据库连接上限约束，而 `service.max_instances` 决定了上限。要把连接池从默认的每实例 4 个调高，以应对一个 Cloud Run 实例同时处理的 80 个请求，请在服务使用的字符串中加上 `pool_max_conns`，例如 `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8`。见[等待数据库连接](running.md#waiting-for-a-database-connection)。

<!-- source: 3-copy-the-example-and-set-the-variables 1b99f37cb3a7 -->
### 3. 复制示例并设置变量

从你要部署的发布版本中，把示例连同 provider 锁定文件复制到一个空目录中：

```sh
VERSION=v0.1.0
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r my-source/deploy/opentofu/examples/gcp-cloud-run-neon/. my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

`main.tf` 由 `google` provider 和一个块 `module "mycroblog"` 组成。**将 `ref` 设为该发布版本**。该块用 `?ref=` 指定一个发布版本，稳定版的副本中已经设置正确。服务器镜像就是同一个发布版本，因此只有 `ref` 决定运行什么。对于预发布版本，请自己设置两处：`sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`，以及块中的 `image = { version = "0.1.0-rc.4" }`（版本号不带 `v`），因为预发布版本的模块不会默认使用自己的镜像。模块只存在于包含它的第一个发布版本及之后，因此更早的标签中没有可复制的示例。请保留这个目录：它保存着你的状态和设置。

填写 `terraform.tfvars`。`mail_from`、`mail_smtp_host` 和 `mail_smtp_username` 用于配置[邮件](email.md)；端口是 587，因为 Google Cloud 屏蔽出站 25 端口。要更改其他内容，请把对应的部分加到 `main.tf` 的块中，例如 `ingress = { kind = "domain-mapping" }`：每个部分及其拒绝的内容都在[模块的 README](../../deploy/opentofu/README.md#sections) 中。

`settings` 按键接受[配置参考](../reference/configuration.md)中的任何其他设置，`secrets` 接受机密设置的键，每个键都会得到一个待填写的空机密。如果某个键已被某个部分或模块设置，计划会失败；模块在 Cloud Run 上不接受的键（例如 `listen`）也会失败，错误信息会说明原因。如果账号名使用另一个域名，请现在就决定 [`account_domain`](../reference/configuration.md#account_domain)，写成 `settings = { account_domain = "example.org" }`。

**状态**默认保存在这个目录中。它不包含任何机密值，但它是已创建资源的唯一记录：如果不止一个人操作，请用 `backend "gcs"` 块把它保存在存储桶中。

<!-- source: 4-create-the-secrets-and-add-their-values 51cddae33ec9 -->
### 4. 创建机密并添加值

以下命令会创建机密，并生成 KEK 和 VAPID 密钥：

```sh
tofu init
tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager
```

`tofu init` 还会获取该发布版本的脚本：apply 会运行的 `migrate.sh`，以及后续步骤会用到的 `console.sh`：

```sh
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

**现在就复制 KEK**，并把副本与数据库分开保存，例如放在密码管理器中。没有它就无法恢复[备份](backup.md)，账号的密钥也会丢失。`tofu output kek` 给出相应的命令。VAPID 密钥也照此处理，丢失它会使所有 Web Push 订阅失效：

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

然后添加来自其他地方的值。`tofu output secrets_to_fill` 会列出模块创建的每个空机密、应填入的内容，以及添加它的命令。无法读取机密的实例不会启动，因此请在部署服务之前添加：

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=- # the direct string
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

KEK 以挂载文件的形式交给服务器，由 `MY_KEK_FILE` 指定。其他机密是实例启动时解析的环境变量，因此新版本在下一个实例上生效：为确保生效，请部署一个新修订版本。

**OpenTofu 只生成一次 KEK，之后不再生成**。该机密及其版本设有 `prevent_destroy`，会写入新值的版本号在模块中是固定的：新的随机 KEK 会让旧 KEK 加密的所有内容都无法读取。要轮换，请按照[轮换 KEK](key-management.md#rotate-the-kek)操作，用 `gcloud secrets versions add` 添加新值。OpenTofu 不会跟踪你在那里添加的内容。

如果要从一开始就把 KEK 放在 Cloud KMS 中，请在这一步之前设置 `kek = { kind = "kms" }`，把目标 `kek_secret_manager` 换成 `module.mycroblog.module.kek_kms`，并启用 `cloudkms.googleapis.com`。

<!-- source: 5-apply 2076866c0f3b -->
### 5. 应用

```sh
tofu apply
```

这会创建其余所有资源，顺序由服务的启动探测所要求：探测会等待 `/health/ready`，而在架构存在之前它都返回 `503`。迁移作业最先创建，连同它依赖的资源，其中包括负载均衡器的地址，因为服务器的设置会用到它。然后 apply 会在你的机器上通过 `gcloud` 把 `my upgrade` 作为该作业的一次执行来运行，并等待它完成：依次是 `my config check`、`my db upgrade -check`、`my schema apply` 和 `my db upgrade`，并在第一个失败处停止。它的输出会出现在 apply 的输出中。第一个使用数据库的命令会把 `host` 和 `account_domain` 记录到数据库中。之后 apply 才会创建服务。

如果迁移失败，apply 会在创建服务之前停止。请修复原因（通常是某个机密没有值），然后再次运行 `tofu apply`：它会重新运行迁移。

<!-- source: 6-point-the-domain-at-the-load-balancer c81cc51be2af -->
### 6. 将域名指向负载均衡器

`tofu output dns_records` 会列出一条 `A` 记录，把 `host` 指向负载均衡器的地址。请在你的 DNS 服务商处添加它。如果该服务商有自己的代理，请对这条记录关闭代理。

证书由 Google 管理，只有在 DNS 指向该地址后才会生效，这可能需要一段时间。在此之前，`https://{host}` 会因 TLS 错误而失败，因此联邦和浏览器暂时无法访问服务器。其他部分无需等待：Cloud Tasks 和 Cloud Scheduler 调用的是服务的默认 `run.app` URL，即 `https://<service>-<project number>.<region>.run.app`。**不要禁用默认 URL**，否则任务会停止。该 URL 拒绝来自互联网的请求，而 Cloud Run 只有在同一项目内、且通过该 URL（而非自定义域名）调用时，才会把这两者视为内部流量。

还有哪些选择，见[流量如何到达服务](#how-traffic-reaches-the-service)。

<!-- source: 7-create-the-first-administrator 19137d6d01f0 -->
### 7. 创建第一个管理员

Cloud Run 作业没有终端，也没有标准输入，因此 `my admin create` 不读取密码，而是打印一个设置密码的链接。打开[控制台](deploy-opentofu.md#the-console)，然后运行：

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

请立即打开链接并设置密码。它只能使用一次，有效期一小时。**在此之前它会一直留在 Cloud Logging 中**，任何能读取该项目日志的人都可以使用它。如果链接过期，请通过控制台运行 `my admin account password-reset-link @alice`。然后开启双重验证。

<!-- source: 8-go-through-the-checklist e7fe33d10e80 -->
### 8. 对照检查清单

对照[生产环境检查清单](production-checklist.md)逐项检查，先从[邮件](production-checklist.md#email)开始：在 `mail_from` 的域名上配置 SPF、DKIM 和 DMARC。

<!-- source: the-console 52df6f8c43ac -->
## 控制台

控制台使用服务的镜像、设置和机密运行一条 `my` 命令。它只在 `console_enabled` 开启时存在，因此用完后请关闭：

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` 会等待命令结束，打印其输出和退出码，并以该退出码退出。它需要 `roles/logging.viewer`，因为任务的输出会进入 Cloud Logging。它使用服务正在运行的镜像：在停在迁移处的升级之后，那就是上一个发布版本，数据库仍然与它匹配。

该命令没有标准输入和终端，因此需要从中读取密码或文件的命令无法这样运行。它打印的内容会留在日志中，所以不要运行你不愿让输出写进日志的命令。

<!-- source: upgrading fe1bed77d3af -->
## 升级

请先阅读[升级](database.md#upgrading)和发布说明，并且一次只升一个小版本。升级就是对新发布版本执行一次 apply：计划会显示每个服务和作业都换上它的镜像，而 apply 会在其中任何一个开始提供服务之前运行迁移。

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

镜像就是 `ref` 所指定的发布版本，因此更改 `ref` 就是整个升级。应用之前请阅读该计划：新发布版本的模块可能也会更改资源。apply 会执行以下操作，并在第一个失败的步骤处停止：

1. 给迁移作业和控制台作业换上新镜像，同时执行计划中服务不依赖的其余部分。
2. 在你执行 apply 的地方，把 `my upgrade` 作为迁移作业的一次执行来运行，并等待它完成：`my config check`、`my db upgrade -check`、`my schema apply` 和 `my db upgrade`。它的输出会出现在 apply 的输出中。
3. 先部署每个 worker 的新修订版本，再部署服务的。只有当修订版本的启动探测在 `/health/ready` 上通过后，它才会接收流量。

**执行 apply 的地方**需要 `gcloud` 和 `jq`，以及可以执行该作业并读取其日志的凭据（`roles/run.developer` 和 `roles/logging.viewer`，Owner 已包含它们）。执行 apply 的 CI 作业也是如此。

**当某个步骤失败时**，服务会保持在它当前的修订版本上，发布说明或迁移的输出会说明原因。请修复原因，并用相同的版本再次运行 `tofu apply`：它会重新运行迁移，已完成的步骤会发现无事可做。被中断或丢失连接的 apply 会让该执行继续在 Cloud Run 上运行；下一次 apply 会等它结束后再开始自己的执行。若要在架构变更之后回退，请恢复备份：请参阅[回滚](database.md#rolling-back)。用更早的版本执行 apply 会停在迁移处，因为那个发布版本拒绝更新的架构，并且不会更改任何服务。

若要试用候选发布版本，或在较新的模块下保留较早发布版本的镜像，请自己设置版本；它是镜像标签，不带发布版本的 `v`：

```hcl
  image = { version = "0.2.0-rc.1" }
```

<!-- source: checking-the-sql-first 9ff10080c36d -->
### 先检查 SQL

计划不会显示迁移的 SQL。若要在 apply 之前阅读它，请只给迁移作业换上新镜像，并在其中运行不会更改任何内容的 `my upgrade -dry-run`：

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region=asia-northeast1 --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

<!-- source: destructive-changes 5b3fdea90a55 -->
### 破坏性变更

当发布说明要求使用 `-allow-destructive` 时，请在块中写明该版本：

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

`my upgrade` 只在该版本的 apply 中才会得到 `-allow-destructive`，因此保留在原处的值不会让之后的任何发布版本删除数据。

<!-- source: pinning-the-image-by-digest b31962ecc908 -->
### 按摘要固定镜像

[生产环境检查清单](production-checklist.md#operations)推荐使用摘要，它来自发布说明：

```hcl
  image = { digest = "sha256:..." }
```

无论旁边的标签写的是什么，摘要只指向一个镜像，因此请在更改 `ref` 的同一次编辑中更改它：上一个发布版本遗留的摘要，会让新版本下仍保留那个发布版本的镜像。

<!-- source: the-maintenance-mode 518e9aa176c0 -->
### 维护模式

[维护模式](maintenance.md)需要三次 apply，因为服务在升级之前和之后都要更改。把它加到块的设置中并 apply，进行备份，提高 `ref` 并 apply，然后移除它并 apply：

```hcl
  settings = { maintenance = "true" }
```

不使用它时，旧修订版本会在迁移期间继续提供服务，这[不保证能正常工作](database.md#the-server-during-an-upgrade)。用 `gcloud` 手动更改的设置不会保留：下一次 apply 会恢复块中所写的内容。

<!-- source: monitoring-and-alerts 930234cd0dde -->
## 监控和告警

模块会添加对 `/health/live` 的可用性检查、基于日志的指标，以及名称与[运行手册](alerts.md)一致的告警策略，它们会通知 `monitoring.notification_emails`（示例中为 `alert_emails`）中的地址或 `notification_channels` 中的渠道。该输入没有默认值：请指明通知谁，或设置 `monitoring = null` 来完全不要它。它从未被应用过，所以请通过让一次检查失败来试用一次。每次运行检查都会唤醒一个实例，这会加重[tick 的取舍](#costs)：`uptime_period`（默认 15 分钟）决定了多久一次。

<!-- source: changing-a-backend c20598caf230 -->
## 更换后端

更改对应部分的 `kind` 即可。每种 kind 会在服务器上设置它所需要的内容，计划会说明哪些内容会改变。更换数据库还意味着要迁移数据，因为新数据库是空的：请对旧数据库做一次[备份](backup.md)，并在没有任何写入时把它恢复到新数据库中。例如，用 Cloud SQL 代替 Neon：

```hcl
  database = { kind = "cloud-sql" }
```

如果要使用 Google Cloud 之外的存储，请把 `media.kind` 设为 `s3`（适用于任何 S3 兼容服务，需要 `endpoint`），或设为 `r2`（需要 `cloudflare.account_id`）。两者都会从 `secrets_to_fill` 列出的机密中读取访问密钥。

<!-- source: when-you-need-a-network 0a4359c002ed -->
## 需要网络时

本指南不需要网络：Neon、Cloud Tasks、Cloud Storage 和 Pub/Sub 都通过互联网、以服务自身的身份访问，因此 Cloud Run 不连接任何网络，也就没有需要付费或维护的东西。

如果要在部署中使用 Memorystore（它只有私有地址），或者让 Cloud SQL 使用私有地址代替公有地址，就需要网络。`network = { kind = "vpc" }` 会创建一个，`network = { kind = "existing", ... }` 则连接到你已有的 VPC，你用 `network`、`subnetwork`、`private_services_range` 和 `memorystore_connection_policy` 来描述它：

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

只有私有地址段的流量经过该网络，因此服务仍像以前一样访问互联网，不需要 Cloud NAT。`nat = true` 则让所有流量改走 Cloud NAT，以获得固定的源地址。请先启用 Service Networking 和 Network Connectivity API，以及 Compute Engine API。网络会增加实例的冷启动时间，而且服务器空闲时 Memorystore 仍在计费。

<!-- source: splitting-jobs-into-workers 831929244c6d -->
## 将任务拆分到 worker

当某一类任务拖慢了其他任务时，就给它一个单独的服务。按照[按任务类型拆分 worker](running.md#splitting-workers-by-job-type) 中的说明，根据指标决定拆分什么。一个值 `workers` 会同时交给服务、队列和监控，所有队列、URL、路由和设置都由它的键生成，因此这些你都不用写：

```hcl
  workers = {
    delivery = {
      job_types     = ["federation.fanout", "federation.deliver"]
      max_instances = 20
      concurrency   = 16
    }
  }
```

计划会拒绝拆开必须放在一起的类型。它还需要一个能到达每个进程的消息代理，`stream.kind = pubsub` 就是这样的代理。

那一页中两种常见的布局如下。要让投递不拖慢其他任务，请使用上面的值。要让邮件和通知保持及时：

```hcl
  workers = {
    notify = {
      job_types = [
        "identity.verification_email", "identity.password_reset_email", "identity.password_notice_email",
        "identity.closure_email", "identity.deactivation_email",
        "notification.email", "notification.moderator_email", "notification.supporter_email",
        "notification.push", "notification.post_subscribers",
        "instance.notify_announcement", "voice.notify_started",
      ]
      max_instances = 10
    }
  }
```

如那一页所述，请省略你未设置的设置所对应的类型：服务器未注册的类型会导致启动失败，而计划无法看出是哪些类型。使用 `workers` 时，即使默认服务空闲，也不会帮忙处理这些类型，因为它们的任务会进入各自的队列。

每个 worker 接受 `job_types`，以及 `min_instances`、`max_instances`、`cpu`、`memory_mib`、`concurrency`、`max_concurrent_dispatches` 和 `max_dispatches_per_second`，未指定的项分别沿用服务或队列的值。worker 的服务不接收来自外部的流量。升级会在迁移之后、默认服务之前更新每个 worker，而且不需要你告诉它任何东西：只有失败的步骤会让它停下。每个 worker 都可能带来与一个服务相当的费用，其实例的连接也计入数据库的连接上限。

<!-- source: how-traffic-reaches-the-service 156fc6f76743 -->
## 流量如何到达服务

这由 `ingress.kind` 决定。默认是面向生产环境的负载均衡器。域名映射适合低成本地试用部署：设置 `ingress = { kind = "domain-mapping" }`。

| | `load-balancer`（默认） | `domain-mapping` |
| --- | --- | --- |
| 月费 | 转发规则约 18 美元，另按 GB 收费。请查看[价格页面](https://cloud.google.com/vpc/network-pricing)：这个数字未能在那里得到确认 | 无 |
| 状态 | Google Cloud 推荐的方式 | 预览版功能，由于延迟问题，Google Cloud 不推荐用于生产环境 |
| 区域 | 任意 | `asia-northeast1`、`us-central1` 等少数几个 |
| DNS | 一条 `A` 记录，来自 `tofu output dns_records` | 同一输出列出的记录 |
| 证书 | 由 Google 管理，DNS 指向该地址后生效 | 由 Google 管理，最长需要 24 小时 |
| 域名所有权 | 除 DNS 外无其他要求 | 你以及其他执行 apply 的每个账号，都必须在 apply 之前成为[已验证的所有者](https://cloud.google.com/run/docs/mapping-custom-domains) |
| `run.app` URL | 拒绝来自互联网的请求，因此没有人能绕过负载均衡器 | 任何人都能访问：域名映射无法添加请求头，因此源站保持开放 |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` 和负载均衡器的地址 | `169.254.0.0/16` |

Cloud Tasks 和 Cloud Scheduler 不经过这两者：它们直接调用默认的 `run.app` URL。

<!-- source: cloudflare e55e367a0064 -->
### Cloudflare

`ingress = { kind = "cloudflare" }` 会把 Cloudflare 放在 `host` 前面，并让所有内容绕过缓存；`ingress.media_host` 则在任何 kind 旁边添加一个带缓存的媒体主机。两者都需要 `cloudflare = { zone_id = "..." }`，令牌放在 `CLOUDFLARE_API_TOKEN` 中。它们只在模拟的提供程序上运行过。

- **你的套餐决定源站。** Cloud Run 按 `Host` 路由，而 Cloudflare 只在 Enterprise 套餐中提供 Host 请求头覆盖。在任何其他套餐上，源站必须自己接受你的主机，因此模块会为每个主机创建一个域名映射（取舍见上表），由 Cloudflare 连接到它。它的证书能否在代理之后签发和续期尚未验证，因此请从 `cloudflare.proxied = false` 开始。在 Enterprise 套餐上，`cloudflare.host_header_override = true` 会改为连接到 `run.app` URL，不需要域名映射。
- **用请求头封闭源站。** 模块会添加一条请求头转换规则，服务器会以 `403` 拒绝没有该请求头的请求，但健康检查和任务自己的 `POST`（它们会检查自己的令牌）除外。这样就没有人能通过 `run.app` URL 绕过 Cloudflare，调用方自己的 Cloudflare Worker 也无法选择速率限制所看到的客户端地址。
- **该值会进入状态。** 模块会生成该值并写入机密 `my-origin-secret`，但 Cloudflare 提供程序没有规则请求头值的只写形式，因此请让状态保持私有。如何轮换它见 [CDN 指南](cdn.md#rotate-the-value)。
- **只要记录没有被代理，就没有任何东西会添加该请求头**，所以服务器会拒绝除健康检查之外的一切：在为域名映射的证书使用 `proxied = false` 时，请在证书生效之后再开启代理。
- **请关闭 Bot Fight Mode**，在仪表板中操作。它覆盖整个区域，没有任何规则可以跳过它，而对 `/ap/`、`/.well-known/` 或 `/api/` 的质询会破坏联邦，因为其他服务器不是浏览器。任务不经过 Cloudflare。模块不添加任何质询，并会关闭 Browser Integrity Check，但不管理 Bot Fight Mode。出于同样的原因，不要开启“I'm Under Attack”。
- **Email Address Obfuscation 可以保持开启。** 它会把 HTML 中的电子邮件地址改写成 `[email protected]`，并添加一个用于还原它们的脚本，而服务器的 Content-Security-Policy 会拒绝该脚本。服务器自己的页面，例如说明确认邮件发到哪里的页面，会标记其内容，使其不改动这些地址。
- 如果你希望 `http://` 被重定向，请在仪表板中**开启 Always Use HTTPS**。它覆盖整个区域，所以模块把它留给你处理。
- **超时**：当源站 125 秒内不发送任何内容时，Cloudflare 会返回 `524`。任务不受影响，因为它们直接调用 `run.app` URL。流每 30 秒发送一次心跳，低于 900 秒的空闲上限。在普通的被代理主机上，它是否会缓冲流尚未验证。
- **上传**：Free 和 Pro 套餐为 100 MB，高于 Cloud Run 的 32 MiB，而后者仍是实际的限制。
- **视频**：Cloudflare 的条款限制在 Free、Pro 和 Business 套餐上提供视频以及不成比例的大文件，它可能会重定向这些内容或采取其他措施。在把视频缓存到媒体主机之前，请先决定这一点。

**被删除的文件仍会保留在缓存中。** 媒体主机遵循服务器的 `Cache-Control`，因此被删除的本地文件最长可能继续可见一年，远程文件则是 `remote_media_cache_ttl_seconds`。服务器不会清除它。当某些内容必须消失时，请手动清除，在仪表板的 **Caching > Configuration > Custom Purge** 下，或者：

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

代理 URL 的查询字符串中带有它的 `sig`，它是被缓存内容的一部分，所以请给出完整的 URL。

<!-- source: changing-the-kek-backend b1e8cbb9ccfb -->
## 更换 KEK 后端

更改 `kek.kind` 只需改 `main.tf` 中的一行，但在已有数据的实例上，这并不是没有代价的。数据库中保存着用当前密钥包装的机密，丢失该密钥的实例将无法再读取它们，而每个账号的密钥都在其中。

请按照[在本地 KEK 与 KMS 之间迁移](key-management.md#move-between-the-local-kek-and-a-kms)操作。从机密迁移到 Cloud KMS 时，在机密迁移期间，把当前 KEK 保留为 `kek_previous`：添加 `secrets = ["kek_previous"]`，并用当前 KEK 填写机密 `my-kek-previous`。通过[控制台](deploy-opentofu.md#the-console)运行 `my kek rewrap`，等 `my kek status` 不再统计到 `local:` 密钥时再将其移除。只要你还保留着 rewrap 之前的数据库备份，就要一直保留该 KEK。

在没有数据的新实例上，不需要迁移任何东西。

<!-- source: limits 3ff113a57279 -->
## 限制

- **上传**：Cloud Run 拒绝超过 32 MiB 的 HTTP/1 请求，而服务器使用 HTTP/1.1。因此模块将 `MY_MEDIA_MAX_AV_BYTES` 设为 30 MiB（默认为 100 MiB），让更大的上传得到服务器自己的响应。
- **流**：请求超时（默认 15 分钟）会结束打开的流，客户端会重新连接。
- **客户端地址**：速率限制依赖于 `MY_TRUSTED_PROXIES`，模块根据入口设置它。我们读过的文档都没有说明 Cloud Run 对直接请求会在 `X-Forwarded-For` 中添加什么，也没有说明容器看到的对端地址是什么，容器约定对两者都只字未提，因此 `169.254.0.0/16` 只是一个假设。外部负载均衡器文档中记载的形式是 `[<supplied>,]<client-ip>,<lb-ip>`。请在最初的请求日志中检查地址：如果客户端 IP 是 `169.254` 地址或 Google 的地址，就说明该设置对你的项目而言是错误的。
- **公开访问**：服务向 `allUsers` 授予 `roles/run.invoker`，而针对网域限制共享的组织政策会拒绝这一点。这样的项目需要设置例外。

<!-- source: costs b8be3997d723 -->
## 费用

运行服务器需要花钱。这些数字是一份指南，说明大致会花多少、在哪里用心配置可以节省，而不是部署必须遵守的上限。它们是某一区域的大致标价，取自 2026-10 和 2026-11。请在 [Cloud Run](https://cloud.google.com/run/pricing) 和 [Neon](https://neon.com/pricing) 的价格页面上核实。每个结算账号每月有 180,000 vCPU 秒和 360,000 GiB 秒的免费计算额度。

| 部分 | 空闲时 | 增加费用的因素 |
| --- | --- | --- |
| 负载均衡器 | 每月约 18 美元，未在价格页面上得到确认 | 按 GB 收费；见[流量如何到达服务](#how-traffic-reaches-the-service) |
| Neon（Launch 套餐） | 使用默认 tick 时每月约 19 美元 | 存储每 GB 每月 0.35 美元。改用 `database.kind = cloud-sql` 则有每月约 9.40 美元的固定费用，机器更小 |
| Cloud Run | 0 美元：最小实例数为 0，CPU 只在请求期间计费 | 每个打开的流都会让一个实例持续计费；见下文 |
| Cloud Tasks | 0 美元 | 100 万次操作免费，之后每百万次 0.40 美元。一个任务约需 2 次 |
| Cloud Scheduler | 0 美元：每个结算账号 3 个作业免费 | 超出部分每个作业 0.10 美元 |
| Cloud Storage | 每 GiB 每月约 0.02 美元 | 只有 `us-east1`、`us-west1` 和 `us-central1` 有 5 GiB 免费额度。媒体经由 Cloud Run 发出 |
| Pub/Sub | 0 美元：10 GiB 免费 | 仅在流打开期间产生 |
| Secret Manager | 几美分 | 少量版本免费，之后每个版本几美分 |

**tick 会让 Neon 保持唤醒**。每次 tick 都会读写数据库，而 Neon 只有在 5 分钟没有活动后才会停止计算资源，因此使用默认的 `* * * * *` 时，计算资源永远不会停止。这相当于每月 730 小时，按 0.25 CU 计为 182.5 CU 小时，也就是上面的 19 美元。tick 保持每分钟一次，是为了让定期任务按时运行，并让丢失的唤醒信号能尽快恢复。

**打开的流会让实例持续计费**。在按请求计费的模式下，实例在处理任何请求期间都会计费，而实时更新是一个长请求。一个整月保持流打开的客户端，就会让一个实例全天候计费：扣除免费额度后，1 vCPU、512 MiB 的实例每月约 60 美元。更多的流会共用该实例，最多到 `concurrency`。如果流大部分时间都保持打开，请与按实例计费的模式，或与小型虚拟机加 [Docker Compose](deploy-docker-compose.md) 的方案进行比较。

<!-- source: where-care-reduces-it 28bfc2e5ffcc -->
### 用心配置可以节省的地方

- 用**域名映射**代替负载均衡器，可以省去 18 美元，但要接受[上文所述的取舍](#how-traffic-reaches-the-service)。它适合试用部署。
- **Neon 的 Free 套餐**可以省去 19 美元，小型实例可以在上面运行，但有以下取舍：
  - 它每月允许 100 CU 小时，默认 tick 大约 17 天就会用完。使用更长的计划可以保持在额度之内，例如 `queue = { tick_schedule = "*/10 * * * *" }`：在其他因素唤醒它之前，最多一半时间处于唤醒状态，约 91 CU 小时。此时定期任务最多会延迟 10 分钟运行，丢失的唤醒信号也会更晚恢复。
  - 联邦服务器全天候接收投递，这也会唤醒数据库，因此无论计划如何，繁忙的实例都会超出它的额度。
  - 每个项目容量为 1 GB，可恢复的历史为 6 小时，而且计算资源在空闲 5 分钟后总会停止，因此之后的第一个请求需要等待它启动。
