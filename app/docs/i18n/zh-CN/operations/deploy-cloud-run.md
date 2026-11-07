<!-- source: deploying-on-cloud-run 9d2acbfd443c -->
# 在 Cloud Run 上部署

本文介绍如何用 [`deploy/opentofu`](../../deploy/opentofu/README.md) 中的 OpenTofu 模块在 Google Cloud 上运行 `my`：使用 Cloud Run、Cloud Tasks、Cloud Scheduler、Cloud Storage 和 Pub/Sub，数据库使用 Neon 上的 PostgreSQL，前面是全球外部应用负载均衡器。没有人使用时，服务不运行任何实例，只有一个每分钟唤醒服务器的 tick。运行服务器需要花钱：在存储、流量和打开的流之外，预计每月约 37 美元；用心配置可以减少哪些费用，请阅读[费用](#costs)。

这些模块需要 OpenTofu 1.11 或更高版本。不支持 Terraform。

<!-- source: what-it-creates c95687df197f -->
## 会创建什么

| 部分 | 模块 | 资源 |
| --- | --- | --- |
| 服务器 | `app-gcp-cloud-run` | 一个 `role = all` 的 Cloud Run 服务、一个迁移作业、需要时的控制台作业、一个服务账号，以及公开访问权限 |
| 流量 | `ingress-gcp-load-balancer`，或 `ingress-gcp-domain-mapping` 或 `ingress-cloudflare-app`，以及可选的 `ingress-cloudflare-media` | 带证书的全球外部应用负载均衡器，或域名映射，或 Cloudflare，以及一个媒体主机 |
| 作业 | `queue-gcp-cloud-tasks` | 一个 Cloud Tasks 队列、一个 Cloud Scheduler 作业，以及一个调用服务器的服务账号 |
| 媒体 | `blob-gcp-gcs` | 一个私有存储桶 |
| 实时更新 | `stream-gcp-pubsub` | 一个主题和一个自定义角色。服务器自行创建和删除自己的订阅 |
| 数据库 | `db-external` | 无：数据库由你手动创建 |
| 邮件 | `mail-smtp` | 无：任何 SMTP 服务均可 |
| 机密 | `secrets-gcp-secret-manager` | 数据库 URL 和 SMTP 密码的机密（由你填写），以及一个自动生成的 VAPID 密钥 |
| KEK | `kek-gcp-secret-manager` 或 `kek-gcp-kms` | 一个以文件形式挂载的自动生成的机密，或一个 Cloud KMS 密钥 |

机密值永远不会进入 OpenTofu 的计划或状态：来自其他地方的值由你自己添加，KEK 和 VAPID 密钥则由部署通过只写属性生成。

<!-- source: before-you-start 4c624a6b7316 -->
## 开始之前

- `tofu` 1.11 或更高版本、`gcloud` 和 `jq`。
- 一个已启用结算、专供本实例使用的 Google Cloud 项目，以及该项目中拥有 Owner 角色的账号。
- 用于 [`host`](../reference/configuration.md#host) 的域名。它以后无法更改：请阅读[以后无法更改的设置](production-checklist.md#settings-you-cannot-change-later)。
- 来自[发布版本](running.md#get-the-image)的某个 `my` 版本。没有 `latest` 标签。

<!-- source: first-deployment 996fcc6eb35e -->
## 首次部署

<!-- source: 1-enable-the-apis 2d9c831fb77d -->
### 1. 启用 API

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com
```

`compute.googleapis.com` 用于负载均衡器。

<!-- source: 2-create-the-database-on-neon 139308cfc452 -->
### 2. 在 Neon 上创建数据库

请按 Launch 套餐来预期：使用默认的 tick 时，计算资源永远不会停止。Free 套餐也能用，但要接受[费用](#costs)中所述的取舍。

有两种方式，示例使用第一种：

- **手动创建，使用 `db-external`**。在 Neon 控制台中，在靠近你的 Cloud Run 区域的地区创建项目，并复制其连接字符串。密码不会进入 OpenTofu 状态。
- **使用 [`db-neon`](../../deploy/opentofu/modules/db-neon/README.md)**，它会为你创建项目并把 URL 写入机密**。此时数据库密码会进入状态**，而且该 provider 已无人维护：它的 README 说明了这意味着什么，`main.tf` 中 `db` 块上的注释展示了如何替换。

一个项目有两个连接字符串，部署会把它们各用在合适的地方：

- **直连**字符串（主机名不带 `-pooler`）是必需的。迁移作业和控制台始终使用它：`my schema apply` 会持有会话级锁，而事务模式的连接池无法保持这种锁。
- **池化**字符串（主机名带 `-pooler`）是可选的，供服务使用。Neon 的连接池能满足服务器的需求：见[通过连接池](database.md#through-a-connection-pooler)。把它放在单独的机密中，交给 `db` 模块的 `pooled_url_secret_id`，并加入 `secrets` 模块的 `referenced`。

没有池化字符串时，服务也使用直连字符串，连接数受数据库连接上限约束，而应用模块的 `max_instances` 决定了上限。要把连接池从默认的每实例 4 个调高，以应对一个 Cloud Run 实例同时处理的 80 个请求，请在服务使用的字符串中加上 `pool_max_conns`，例如 `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8`。见[等待数据库连接](running.md#waiting-for-a-database-connection)。

<!-- source: 3-copy-the-example-and-set-the-variables 3abe6b730543 -->
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

**将 `ref` 设为该发布版本**。`main.tf` 中的每个模块块都用 `?ref=` 指定一个发布版本，稳定版的副本中已经设置正确。对于预发布版本，请自己设置：`sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`。模块只存在于包含它们的第一个发布版本及之后，因此更早的标签中没有可复制的示例。让 `ref` 和 `image` 保持同一个发布版本，并保留这个目录：它保存着你的状态和设置。

填写 `terraform.tfvars`。`mail_from`、`mail_smtp_host`、`mail_smtp_port`（默认 587；Google Cloud 屏蔽出站 25 端口）和 `mail_smtp_username` 用于配置[邮件](email.md)。`env` 和 `secret_env` 接受[配置参考](../reference/configuration.md)中的任何其他设置，分别按值和按机密 ID 传入。同一项设置出现在两处会导致计划失败。

如果不发送邮件，请从 `main.tf` 中删除 `mail` 模块及其机密，`mail` 会保持为 `none`。请先阅读[这对注册意味着什么](production-checklist.md#email)。

如果账号名使用另一个域名，请现在就决定 [`account_domain`](../reference/configuration.md#account_domain)，作为 `env` 中的 `MY_ACCOUNT_DOMAIN` 设置。

**状态**默认保存在这个目录中。它不包含任何机密值，但它是已创建资源的唯一记录：如果不止一个人操作，请用 `backend "gcs"` 块把它保存在存储桶中。

<!-- source: 4-create-the-secrets-and-add-their-values f94fd484e28f -->
### 4. 创建机密并添加值

以下命令会创建机密，并生成 KEK 和 VAPID 密钥：

```sh
tofu init
tofu apply -target=module.secrets -target=module.kek
```

`tofu init` 还会获取该发布版本的 `upgrade.sh` 和 `console.sh`，后续步骤会用到它们：

```sh
SCRIPTS=.terraform/modules/app/deploy/opentofu
```

**现在就复制 KEK**，并把副本与数据库分开保存，例如放在密码管理器中。没有它就无法恢复[备份](backup.md)，账号的密钥也会丢失。VAPID 密钥也照此处理，丢失它会使所有 Web Push 订阅失效：

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

然后添加来自其他地方的值。无法读取机密的实例不会启动，因此请在部署应用之前添加：

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=- # the direct string
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

KEK 以挂载文件的形式交给服务器，由 `MY_KEK_FILE` 指定。其他机密是实例启动时解析的环境变量，因此新版本在下一个实例上生效：为确保生效，请部署一个新修订版本。

**OpenTofu 只生成一次 KEK，之后不再生成**。该机密及其版本设有 `prevent_destroy`，会写入新值的版本号在模块中是固定的：不要更改它，因为新的随机 KEK 会让旧 KEK 加密的所有内容都无法读取。要轮换，请按照[轮换 KEK](key-management.md#rotate-the-kek)操作，用 `gcloud secrets versions add` 添加新值。OpenTofu 不会跟踪你在那里添加的内容。

如果要从一开始就把 KEK 放在 Cloud KMS 中，请按照 `main.tf` 中 `kek` 块的注释替换它，并启用 `cloudkms.googleapis.com`。如果要使用你自己的 KEK 或 VAPID 密钥，请在这一步之前，在 `kek` 模块上设置 `create = false`，或从 `generate_p256_keys` 中删除该密钥。在其他地方创建的机密只需要它的 ID：把它传给应用模块的 `secret_env`，或传给使用它的模块。

<!-- source: 5-create-what-the-migration-needs eab1bb33867c -->
### 5. 创建迁移所需的资源

服务的启动探测会等待 `/readyz`，而在架构存在之前它都返回 `503`，所以必须先创建架构。创建迁移作业及其依赖的资源，其中包括负载均衡器的地址，因为服务器的设置会用到它：

```sh
tofu apply -target='module.app.google_cloud_run_v2_job.one_off["migrate"]'
```

<!-- source: 6-apply-the-schema 2946ca18d263 -->
### 6. 应用架构

```sh
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.1.0 --migrate-only
```

它会依次运行 `my config check`、`my db upgrade -check`、`my schema apply` 和 `my db upgrade`，每个命令作为该作业的一次执行，并在第一个失败处停止。第一个使用数据库的命令会把 `host` 和 `account_domain` 记录到数据库中。

<!-- source: 7-create-the-rest 368abac4aa08 -->
### 7. 创建其余资源

```sh
tofu apply
```

<!-- source: 8-point-the-domain-at-the-load-balancer 7e54edc163b2 -->
### 8. 将域名指向负载均衡器

`tofu output dns_records` 会列出一条 `A` 记录，把 `host` 指向负载均衡器的地址。请在你的 DNS 服务商处添加它。如果该服务商有自己的代理，请对这条记录关闭代理。

证书由 Google 管理，只有在 DNS 指向该地址后才会生效，这可能需要一段时间。在此之前，`https://{host}` 会因 TLS 错误而失败，因此联邦和浏览器暂时无法访问服务器。其他部分无需等待：Cloud Tasks 和 Cloud Scheduler 调用的是服务的默认 `run.app` URL，即 `https://<service>-<project number>.<region>.run.app`。**不要禁用默认 URL**，否则作业会停止。该 URL 拒绝来自互联网的请求，而 Cloud Run 只有在同一项目内、且通过该 URL（而非自定义域名）调用时，才会把这两者视为内部流量。

还有哪些选择，见[流量如何到达服务](#how-traffic-reaches-the-service)。

<!-- source: 9-create-the-first-administrator c5b03212fb36 -->
### 9. 创建第一个管理员

Cloud Run 作业没有终端，也没有标准输入，因此 `my admin create` 不读取密码，而是打印一个设置密码的链接。打开[控制台](#the-console)，然后运行：

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

请立即打开链接并设置密码。它只能使用一次，有效期一小时。**在此之前它会一直留在 Cloud Logging 中**，任何能读取该项目日志的人都可以使用它。如果链接过期，请通过控制台运行 `my admin account password-reset-link @alice`。然后开启双重验证。

<!-- source: 10-go-through-the-checklist 90add86e4288 -->
### 10. 对照检查清单

对照[生产环境检查清单](production-checklist.md)逐项检查，先从[邮件](production-checklist.md#email)开始：在 `mail_from` 的域名上配置 SPF、DKIM 和 DMARC。

<!-- source: the-console 5aebf704c6d7 -->
## 控制台

控制台使用服务的镜像、设置和机密运行一条 `my` 命令。它只在 `console_enabled` 开启时存在，因此用完后请关闭：

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` 会等待命令结束，打印其输出和退出码，并以该退出码退出。它需要 `roles/logging.viewer`，因为作业的输出会进入 Cloud Logging。无论 `image` 怎么写，它都使用服务正在运行的镜像。

该命令没有标准输入和终端，因此需要从中读取密码或文件的命令无法这样运行。它打印的内容会留在日志中，所以不要运行你不愿让输出写进日志的命令。

<!-- source: upgrading 1a1e204c5173 -->
## 升级

请先阅读[升级](database.md#upgrading)和发布说明，并且每次只升级一个次版本。

```sh
# 1. Back up the database while nothing writes. Use pg_dump or a Neon branch (see Backup and restore).
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --update-env-vars=MY_MAINTENANCE=true
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Read the SQL it would run. This changes nothing but the image of the migration job.
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --plan

# 3. Upgrade. These scripts are of the release your modules name, which is enough: they only run gcloud and my.
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --maintenance
```

脚本会执行以下步骤，并在第一个失败的步骤处停止：

1. 使用 `--maintenance` 时，将服务切换到[维护模式](maintenance.md)。
2. 给迁移作业换上新镜像。
3. 依次运行 `my config check`、`my db upgrade -check`、`my schema apply` 和 `my db upgrade`，每个都等待前一个完成。
4. 部署新修订版本，并退出维护模式。只有当它的启动探测在 `/readyz` 上通过后，才会接收流量。

不使用 `--maintenance` 时，旧修订版本会在第 2 步和第 3 步期间继续提供服务，而这[不保证能正常工作](database.md#the-server-during-an-upgrade)。发布说明要求时，请加上 `--allow-destructive`。用 `--image ghcr.io/mycroblog/my:0.2.0@sha256:...` 固定摘要，这是[生产环境检查清单](production-checklist.md#operations)所推荐的做法。

然后将 `terraform.tfvars` 中的 `image` 设为相同的值，将 `main.tf` 中的 `ref` 设为相同的发布版本，再运行 `tofu init` 和 `tofu plan`。应用之前请阅读该计划：新发布版本的模块可能会更改资源。`tofu apply` 从不更改已存在的服务或作业的镜像，因此忘记修改 `image` 不会造成损害，但新作业或重新创建的服务会以这个值启动。

**某个步骤失败时**，服务会停留在原来的修订版本上，如果你要求了维护模式，则仍处于维护模式。修复原因后再次运行脚本即可：这些步骤可以安全地重复执行。在 `my schema apply` 尚未运行的情况下，如果要不升级而退出维护模式：

```sh
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --remove-env-vars=MY_MAINTENANCE
```

架构已经变更后如果要回退，请恢复备份：见[回滚](database.md#rolling-back)。

<!-- source: monitoring-and-alerts 67bc34d23fdf -->
## 监控与告警

模块 [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) 会添加 `/readyz` 的可用性检查、使用[运行手册](alerts.md)中名称的告警策略，以及邮件通知。它从未被实际应用过，因此请通过让某项检查失败来试一次。请先启用 `monitoring.googleapis.com`，并阅读其 README 中关于检查费用的说明：每次运行都会唤醒一个实例，并通过 `/readyz` 唤醒数据库，这会加重 [tick 的取舍](#costs)。

<!-- source: changing-a-backend ef4660f60191 -->
## 更换后端

替换 `main.tf` 中该模块的块即可。应用模块接受该块返回的任何内容，因此其他部分都不需要改变。更换数据库还意味着要迁移数据，因为新数据库是空的：[`db-gcp-cloud-sql`](../../deploy/opentofu/modules/db-gcp-cloud-sql/README.md#coming-from-db-external) 中有具体步骤。例如，使用你自己的数据库：

```hcl
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/db-external?ref=v0.1.0&depth=1"

  url_secret_id = "another-database-url"
}
```

如果要使用 Google Cloud 之外的存储，请把 `blob` 块替换为适用于任何 S3 兼容服务的 [`blob-s3`](../../deploy/opentofu/modules/blob-s3/README.md)，或 [`blob-cloudflare-r2`](../../deploy/opentofu/modules/blob-cloudflare-r2/README.md)。

<!-- source: when-you-need-a-network fb392704a1e7 -->
## 需要网络时

本指南不需要网络：Neon、Cloud Tasks、Cloud Storage 和 Pub/Sub 都通过互联网、以服务自身的身份访问，因此 Cloud Run 不连接任何网络，也就没有需要付费或维护的东西。

如果要在部署中使用 Memorystore（它只有私有地址），或者让 Cloud SQL 使用私有地址代替公有地址，就需要网络。请添加 [`network-gcp-vpc`](../../deploy/opentofu/modules/network-gcp-vpc/README.md)，或把你已有的 VPC 描述为[相同结构](../../deploy/opentofu/README.md#the-network-on-google-cloud)的对象，并把同一个网络传给应用以及每个接受它的模块：

```hcl
module "network" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/network-gcp-vpc?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
}

module "kv" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kv-gcp-memorystore?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
  network    = module.network
}
```

同时给应用设置 `network = module.network`，并把 `module.kv` 加入它的 `backends`。只有私有地址段的流量经过该网络，因此服务仍像以前一样访问互联网，不需要 Cloud NAT。使用私有地址的后端，如果应用位于另一个网络或不在任何网络中，计划会失败。请先启用 Service Networking 和 Network Connectivity API，以及 Compute Engine API。网络会增加实例的冷启动时间：请阅读[私有网络增加了什么](../../deploy/opentofu/modules/kv-gcp-memorystore/README.md#what-the-private-network-adds)。

<!-- source: splitting-jobs-into-workers ed661aede0a3 -->
## 将作业拆分到 worker

当某一类作业拖慢了其他作业时，就给它一个单独的服务。按照[按作业类型拆分 worker](running.md#splitting-workers-by-job-type) 中的说明，根据指标决定拆分什么。同一个值同时交给队列和应用，所有队列、URL、路由和设置都由它的键生成，因此这些你都不用写：

```hcl
locals {
  workers = {
    delivery = {
      job_types     = ["federation.fanout", "federation.deliver"]
      max_instances = 20
      concurrency   = 16
    }
  }
}
```

在 `queue` 块和 `app` 块上都设置 `workers = local.workers`。计划会拒绝拆开必须放在一起的类型，也会拒绝队列和应用设置了不同的值。它还需要一个能到达每个进程的消息代理，`stream-gcp-pubsub` 就是这样的代理。如果你使用 `monitoring-gcp`，也要把同一个值交给它。

那一页中两种常见的布局如下。要让投递不拖慢其他作业，请使用上面的值。要让邮件和通知保持及时：

```hcl
locals {
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
}
```

如那一页所述，请省略你未设置的设置所对应的类型：服务器未注册的类型会导致启动失败，而计划无法看出是哪些类型。使用 `workers` 时，即使默认服务空闲，也不会帮忙处理这些类型，因为它们的任务会进入各自的队列。

每个字段，以及 worker 从应用继承了什么，都在[约定](../../deploy/opentofu/README.md#workers)中。worker 的服务不接收来自外部的流量。`upgrade.sh` 会在迁移之后、默认服务之前更新每个 worker，而且不需要你告诉它任何东西：只有失败的步骤会让它停下。每个 worker 都可能带来与一个服务相当的费用，其实例的连接也计入数据库的连接上限。

<!-- source: how-traffic-reaches-the-service d1e6d0ed195b -->
## 流量如何到达服务

这由 `main.tf` 的 `ingress` 块决定。示例使用面向生产环境的负载均衡器。旁边的注释中有域名映射，适合低成本地试用部署：要换成它，请修改该块以及 `app` 块的 `service_ingress`，两者不一致时计划会提示。

| | `ingress-gcp-load-balancer`（示例） | `ingress-gcp-domain-mapping` |
| --- | --- | --- |
| 月费 | 转发规则约 18 美元，另按 GB 收费。请查看[价格页面](https://cloud.google.com/vpc/network-pricing)：这个数字未能在那里得到确认 | 无 |
| 状态 | Google Cloud 推荐的方式 | 预览版功能，由于延迟问题，Google Cloud 不推荐用于生产环境 |
| 区域 | 任意 | `asia-northeast1`、`us-central1` 等少数几个 |
| DNS | 一条 `A` 记录，来自 `tofu output dns_records` | 同一输出列出的记录 |
| 证书 | 由 Google 管理，DNS 指向该地址后生效 | 由 Google 管理，最长需要 24 小时 |
| 域名所有权 | 除 DNS 外无其他要求 | 你以及其他执行 apply 的每个账号，都必须在 apply 之前成为[已验证的所有者](https://cloud.google.com/run/docs/mapping-custom-domains) |
| `run.app` URL | 拒绝来自互联网的请求，因此没有人能绕过负载均衡器 | 任何人都能访问：域名映射无法添加请求头，因此源站保持开放 |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` 和负载均衡器的地址 | `169.254.0.0/16` |
| `app` 块的 `service_ingress` | `internal-and-load-balancer`。与 `ingress` 块不一致时计划失败 | `all` |

Cloud Tasks 和 Cloud Scheduler 不经过这两者：它们直接调用默认的 `run.app` URL。

如果要使用服务自己的 URL，或在前面放置其他东西（例如 CDN），请删除 `ingress` 块及其在 `backends` 中的那一行，并把 `service_ingress` 保持为你需要的值：它始终由你明确选择，而 `all` 会让 `run.app` URL 对任何人开放。在 `env` 中将 `MY_TRUSTED_PROXIES` 设为连接到服务器的那些地址。无论你在前面放什么，它都必须仍然在 `https://{host}` 上响应。

<!-- source: cloudflare 95b7a9baaff1 -->
### Cloudflare

`ingress-cloudflare-app` 把 Cloudflare 放在 `host` 前面，并对所有内容绕过缓存；`ingress-cloudflare-media` 则在任何 ingress 旁边添加一个带缓存的媒体主机。请先阅读每个模块的 README（[app](../../deploy/opentofu/modules/ingress-cloudflare-app/README.md)、[media](../../deploy/opentofu/modules/ingress-cloudflare-media/README.md)）。令牌放在 `CLOUDFLARE_API_TOKEN` 中。这些模块只在模拟的 provider 上运行过。

- **你的套餐决定源站**。Cloud Run 按 `Host` 路由，而 Cloudflare 只在 Enterprise 套餐中提供 Host 请求头覆盖功能。在其他任何套餐上，`*.run.app` 源站会导致计划失败，因此源站本身必须接受你的主机名：保留该主机的一个 `ingress-gcp-domain-mapping`（不放在 `backends` 中），并设置 `origin_host = "ghs.googlehosted.com"`。每个主机都需要各自的映射。它的证书能否在代理后面签发和续期尚未验证，因此请从 `proxied = false` 开始。
- **请求头封闭源站**。`ingress-cloudflare-app` 会添加一条请求头转换规则，服务器对不带该请求头的请求返回 `403`，健康检查和作业自己的 `POST`（它们会校验各自的令牌）除外。因此，没有人能通过 `run.app` URL 绕过 Cloudflare，调用方自己的 Cloudflare Worker 也无法决定速率限制看到的客户端地址。
- **该值会进入状态**。Cloudflare provider 没有规则请求头值的只写形式。用 `openssl rand -hex 32` 生成一个值，用 `gcloud secrets versions add` 将其加入机密，用 `TF_VAR_origin_secret` 传入同一个值，并确保状态不公开。[CDN 指南](cdn.md#rotate-the-value)介绍了如何轮换它。
- **记录未开启代理时，没有任何东西会添加该请求头**，因此服务器会拒绝除健康检查之外的所有请求：如果为了域名映射的证书而设置了 `proxied = false`，请在那之后再设置机密。
- 在控制台中**关闭 Bot Fight Mode**。它覆盖整个区域，任何规则都无法跳过它，而对 `/ap/`、`/.well-known/` 或 `/api/` 的质询会破坏联邦，因为其他服务器不是浏览器。作业不经过 Cloudflare。这些模块不添加任何质询，并关闭 Browser Integrity Check，但不管理 Bot Fight Mode。出于同样的原因，不要开启“I'm Under Attack”。
- 如果你希望重定向 `http://`，请在控制台中**开启 Always Use HTTPS**。它覆盖整个区域，所以模块把它留给你决定。
- **超时**：源站 125 秒内没有发送任何内容时，Cloudflare 会返回 `524`。作业不受影响，因为它们直接调用 `run.app` URL。流每 30 秒发送一次心跳，低于 900 秒的空闲上限。它在普通的已代理主机上是否会缓冲流尚未验证。
- **上传**：Free 和 Pro 套餐为 100 MB，高于 Cloud Run 的 32 MiB，因此上限仍是后者。
- **视频**：Cloudflare 的条款限制在 Free、Pro 和 Business 套餐上提供视频以及不成比例的大量大文件，它可能会重定向这些内容或采取其他措施。在媒体主机上缓存视频之前，请先做好决定。

**已删除的文件仍会留在缓存中**。媒体主机遵循服务器的 `Cache-Control`，因此已删除的本地文件可能在最长一年内仍然可见，远程文件则为 `remote_media_cache_ttl_seconds`。服务器不会清除它。当某些内容必须消失时，请在控制台的 **Caching > Configuration > Custom Purge** 下手动清除，或者：

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

代理 URL 的 `sig` 在查询字符串中，而查询字符串是缓存内容的一部分，因此请提供完整的 URL。

<!-- source: changing-the-kek-backend c5e918ea5c81 -->
## 更换 KEK 后端

替换 `kek` 块只需改 `main.tf` 中的一个块，但在已有数据的实例上，这并不是没有代价的。数据库中保存着用当前密钥包装的机密，丢失该密钥的实例将无法再读取它们，而每个账号的密钥都在其中。

请按照[在本地 KEK 与 KMS 之间迁移](key-management.md#move-between-the-local-kek-and-a-kms)操作。从机密迁移到 Cloud KMS 时，在机密迁移期间，通过应用模块以 `secret_files = { MY_KEK_PREVIOUS_FILE = "my-kek" }` 传入当前 KEK 的机密，将其保留为 `kek_previous`；通过[控制台](#the-console)运行 `my kek rewrap`，等 `my kek status` 不再统计到 `local:` 密钥时再将其删除。只要你还保留着 rewrap 之前的数据库备份，就要一直保留该 KEK。

在没有数据的新实例上，不需要迁移任何东西。

<!-- source: limits 5bd276dd3a89 -->
## 限制

- **上传**：Cloud Run 拒绝超过 32 MiB 的 HTTP/1 请求，而服务器使用 HTTP/1.1。因此示例将 `MY_MEDIA_MAX_AV_BYTES` 设为 30 MiB（默认为 100 MiB），让更大的上传得到服务器自己的响应。
- **流**：请求超时（默认 15 分钟）会结束打开的流，客户端会重新连接。
- **客户端地址**：速率限制依赖于 `MY_TRUSTED_PROXIES`，应用模块根据 `ingress` 模块的返回值设置它。我们读过的文档都没有说明 Cloud Run 对直接请求会在 `X-Forwarded-For` 中添加什么，也没有说明容器看到的对端地址是什么，容器约定对两者都只字未提，因此 `169.254.0.0/16` 只是一个假设。外部负载均衡器文档中记载的形式是 `[<supplied>,]<client-ip>,<lb-ip>`。请在最初的请求日志中检查地址：如果客户端 IP 是 `169.254` 地址或 Google 的地址，就说明该设置对你的项目而言是错误的。
- **公开访问**：服务向 `allUsers` 授予 `roles/run.invoker`，而针对网域限制共享的组织政策会拒绝这一点。这样的项目需要设置例外。

<!-- source: costs 1ab8d07f6847 -->
## 费用

运行服务器需要花钱。这些数字是一份指南，说明大致会花多少、在哪里用心配置可以节省，而不是部署必须遵守的上限。它们是某一区域的大致标价，取自 2026-10 和 2026-11。请在 [Cloud Run](https://cloud.google.com/run/pricing) 和 [Neon](https://neon.com/pricing) 的价格页面上核实。每个结算账号每月有 180,000 vCPU 秒和 360,000 GiB 秒的免费计算额度。

| 部分 | 空闲时 | 增加费用的因素 |
| --- | --- | --- |
| 负载均衡器 | 每月约 18 美元，未在价格页面上得到确认 | 按 GB 收费；见[流量如何到达服务](#how-traffic-reaches-the-service) |
| Neon（Launch 套餐） | 使用默认 tick 时每月约 19 美元 | 存储每 GB 每月 0.35 美元。`db-gcp-cloud-sql` 则有每月约 9.40 美元的固定费用，机器更小：它的 README 中有明细 |
| Cloud Run | 0 美元：最小实例数为 0，CPU 只在请求期间计费 | 每个打开的流都会让一个实例持续计费；见下文 |
| Cloud Tasks | 0 美元 | 100 万次操作免费，之后每百万次 0.40 美元。一个作业约需 2 次 |
| Cloud Scheduler | 0 美元：每个结算账号 3 个作业免费 | 超出部分每个作业 0.10 美元 |
| Cloud Storage | 每 GiB 每月约 0.02 美元 | 只有 `us-east1`、`us-west1` 和 `us-central1` 有 5 GiB 免费额度。媒体经由 Cloud Run 发出 |
| Pub/Sub | 0 美元：10 GiB 免费 | 仅在流打开期间产生 |
| Secret Manager | 几美分 | 少量版本免费，之后每个版本几美分 |

**tick 会让 Neon 保持唤醒**。每次 tick 都会读写数据库，而 Neon 只有在 5 分钟没有活动后才会停止计算资源，因此使用默认的 `* * * * *` 时，计算资源永远不会停止。这相当于每月 730 小时，按 0.25 CU 计为 182.5 CU 小时，也就是上面的 19 美元。tick 保持每分钟一次，是为了让定期作业按时运行，并让丢失的唤醒信号能尽快恢复。

**打开的流会让实例持续计费**。在按请求计费的模式下，实例在处理任何请求期间都会计费，而实时更新是一个长请求。一个整月保持流打开的客户端，就会让一个实例全天候计费：扣除免费额度后，1 vCPU、512 MiB 的实例每月约 60 美元。更多的流会共用该实例，最多到 `concurrency`。如果流大部分时间都保持打开，请与按实例计费的模式，或与小型虚拟机加 [Docker Compose](deploy-docker-compose.md) 的方案进行比较。

<!-- source: where-care-reduces-it 5711f378aadb -->
### 用心配置可以节省的地方

- 用**域名映射**代替负载均衡器，可以省去 18 美元，但要接受[上文所述的取舍](#how-traffic-reaches-the-service)。它适合试用部署。
- **Neon 的 Free 套餐**可以省去 19 美元，小型实例可以在上面运行，但有以下取舍：
  - 它每月允许 100 CU 小时，默认 tick 大约 17 天就会用完。使用更长的计划可以保持在额度之内，例如 `tick_schedule = "*/10 * * * *"`：在其他因素唤醒它之前，最多一半时间处于唤醒状态，约 91 CU 小时。此时定期作业最多会延迟 10 分钟运行，丢失的唤醒信号也会更晚恢复。
  - 联邦服务器全天候接收投递，这也会唤醒数据库，因此无论计划如何，繁忙的实例都会超出它的额度。
  - 每个项目容量为 1 GB，可恢复的历史为 6 小时，而且计算资源在空闲 5 分钟后总会停止，因此之后的第一个请求需要等待它启动。
