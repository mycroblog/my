<!-- source: deploying-with-opentofu 178b85dbd9bd -->
# 使用 OpenTofu 部署

使用 OpenTofu 根模块的每个部署所共有的内容：[在 Google Cloud 上部署](deploy-google-cloud.md)和[在 AWS 上部署](deploy-aws.md)补充各自平台所需要的内容。两个模块都需要 [OpenTofu](https://opentofu.org) 1.11 或更高版本。不支持 Terraform。

<!-- source: the-root-modules 4e0bf528bd0e -->
## 根模块

根模块的一个块就能部署整个服务器：服务、它的后台任务，以及每个角色的后端。

| 根模块 | 平台 | 可复制的示例 |
| --- | --- | --- |
| `mycroblog-gcp` | Google Cloud 上的 Cloud Run | `examples/gcp-cloud-run-neon` |
| `mycroblog-aws` | AWS 上 Fargate 的 ECS。它从未被应用过 | `examples/aws-ecs-neon` |

示例由平台的 provider 和一个块 `module "mycroblog"` 组成，其来源是固定在某个发布版本上的 Git 来源：

```hcl
module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-gcp?ref=v0.1.0&depth=1"

  host = "social.example.org"
  # the sections, below
}
```

必须固定版本：未固定的模块可能在你不知情时更改 KEK 的资源。服务器镜像默认使用 `ref` 所指定的发布版本，因此只有 `ref` 决定运行什么，提高它就是一次升级。模块只存在于包含它的第一个发布版本及之后。

<!-- source: sections 728259c0b297 -->
### 区段

每个角色是块的一个区段（section），它是一个对象，由 `kind` 选择后端，例如 `database = { kind = "external" }`。每个字段都有默认值，即推荐的配置，所以你只需要写你要更改的内容。无法工作的组合会在计划之前失败，并给出指明相关输入的消息。[模块的参考](../../deploy/opentofu/README.md#sections)列出了每个模块的区段、种类和字段。

<!-- source: other-settings-and-secrets 112fdabc5bb0 -->
### 其他设置和机密

`settings` 按键接受[配置参考](../reference/configuration.md)中的任何其他设置，模块会把它传给服务、worker 和作业。`secrets` 接受机密设置的键：模块会为每个键创建一个空机密 `<name>-<key>`（下划线换成连字符），把它传给服务器，并列出它供你填写。

```hcl
  settings = { translator_deepl_url = "https://api.deepl.com" }
  secrets  = ["translator_deepl_api_key"]
```

如果某个键已被某个区段设置或由模块固定，计划会失败；模块不接受的键（例如 `listen`）也会失败，错误信息会给出原因。为 null 的区段（例如 `translation`）以及设为 `settings` 的 `observability` 信号不设置任何内容，所以 `settings` 可以指定它们的键。例如，把错误报告发送到 Sentry：

```hcl
  observability = { errors = "settings" }
  settings      = { errors_exporter = "sentry" }
  secrets       = ["errors_sentry_dsn"]
```

在平台上手动更改的设置不会保留：下一次 apply 会还原块中所写的内容。

<!-- source: secrets d58c587dc144 -->
### 机密

你提供的任何机密值都不会进入 OpenTofu 的计划或状态。模块会通过只写属性生成它能生成的内容：KEK、VAPID 密钥，以及它所创建的数据库的密码。唯一的例外是 `ingress.kind = cloudflare` 的源站机密，它由 Cloudflare 的规则持有，所以会进入状态，状态必须保持私密。

只有你才有的机密，例如外部数据库的 URL、SMTP 密码、访问密钥，或 `secrets` 中的键，模块会创建为空。`tofu output secrets_to_fill` 会列出每一个，说明应填入什么，以及添加它的命令。机密没有值的服务器不会启动。服务器在实例或 task 启动时读取机密，因此新值在下一个实例或 task 上生效。

**OpenTofu 只生成一次 KEK，之后不再生成。** 该机密及其版本设有 `prevent_destroy`，会写入新值的版本号在模块中是固定的：新的随机 KEK 会让旧 KEK 加密的所有内容都无法读取。要轮换，请按照[轮换 KEK](key-management.md#rotate-the-kek)操作，用平台的 CLI 添加新值，OpenTofu 不会跟踪它。

<!-- source: first-deployment 9ae06fd14fc1 -->
## 首次部署

你需要在执行 apply 的地方装有 `tofu` 1.11 或更高版本、`jq` 和平台的 CLI（`gcloud` 或 `aws`），因为 apply 会从那里运行迁移；需要用于 [`host`](../reference/configuration.md#host) 的域名，它以后无法更改（[以后无法更改的设置](production-checklist.md#settings-you-cannot-change-later)）；还需要来自[发布版本](running.md#get-the-image)的某个 `my` 版本。没有 `latest` 标签。请先完成平台页面要求的事项，例如创建数据库。

<!-- source: 1-copy-the-example-and-set-the-variables 8f1ba80ba8d2 -->
### 1. 复制示例并设置变量

从你要部署的发布版本中，把示例连同 provider 锁定文件复制到一个空目录中：

```sh
VERSION=v0.1.0
EXAMPLE=gcp-cloud-run-neon # or aws-ecs-neon
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r "my-source/deploy/opentofu/examples/$EXAMPLE/." my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

**把 `main.tf` 中的 `ref` 设为该发布版本**，稳定版的副本中已经设置正确。对于预发布版本，请同时设置 `ref` 和镜像（版本号不带 `v`），因为预发布版本的模块不会默认使用自己的镜像：`sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`，以及块中的 `image = { version = "0.1.0-rc.4" }`。

填写 `terraform.tfvars`，并把你要更改的任何区段加到 `main.tf` 的块中。如果用户名使用另一个域名，请现在就决定 [`account_domain`](../reference/configuration.md#account_domain)，写成 `settings = { account_domain = "example.org" }`。

请保留这个目录：它保存着你的设置，默认还保存着状态。状态不包含任何机密值，但它是已创建资源的唯一记录：如果不止一个人执行 apply，请把它保存在远程后端中，例如 `gcs` 或 `s3` 存储桶。

<!-- source: 2-create-the-secrets-and-add-their-values 9f13e8109bc9 -->
### 2. 创建机密并添加值

```sh
tofu init
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

`tofu init` 会以你的块的名称获取该发布版本的模块及其脚本：apply 会运行的 `migrate.sh`，以及运行[控制台](#the-console)的 `console.sh`。

先单独 apply 机密，这会创建它们，并通过 KEK 对应种类的模块生成 KEK 和 VAPID 密钥：

| 根模块 | Apply |
| --- | --- |
| `mycroblog-gcp` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager` |
| `mycroblog-aws` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secrets_manager` |

使用 `kek = { kind = "kms" }` 时，请以 `module.mycroblog.module.kek_kms` 作为目标，取代 KEK 的机密。

**现在就复制 KEK**，并把副本与数据库分开保存，例如放在密码管理器中：没有它就无法恢复[备份](backup.md)，账号的密钥也会丢失。`tofu output kek` 给出相应的命令。VAPID 密钥 `<name>-vapid-key` 也照此复制：丢失它会使所有 Web Push 订阅失效。

然后运行 `tofu output secrets_to_fill` 中的每条命令，并填入各自的值。请在下一步之前全部添加。

<!-- source: 3-apply b9f715c7b55d -->
### 3. Apply

```sh
tofu apply
```

这会创建迁移作业及其依赖的内容，通过 `migrate.sh` 在其中运行 `my upgrade`，并等待它完成：依次是 `my config check`、`my db upgrade -check`、`my schema apply` 和 `my db upgrade`，在第一个失败的命令处停止。它的输出会出现在 apply 的输出中。第一个使用数据库的命令会把 `host` 和 `account_domain` 记录到数据库中。只有在那之后，apply 才会创建服务，而服务的健康检查在 schema 存在之前会一直失败。

如果迁移失败，apply 会在创建服务之前停止。请修复原因（通常是某个机密没有值），然后再次运行 `tofu apply`：它会再次运行迁移。

<!-- source: 4-point-the-domain-at-the-server b7b1ead901bc -->
### 4. 把域名指向服务器

`tofu output dns_records` 会列出要在你的 DNS 服务商处添加的记录；在模块自己创建记录的地方，它是空的。

<!-- source: 5-create-the-first-administrator bb599aec43c2 -->
### 5. 创建第一个管理员

控制台没有终端，也没有标准输入，所以 `my admin create` 会打印一个设置密码的链接，而不是读取密码：

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

请立即打开链接并设置密码。它只能使用一次，有效期一小时，而且**在那之前一直保留在平台的日志中**，任何能读取日志的人都可以使用它。如果链接过期，请通过控制台运行 `my admin account password-reset-link @alice`。然后启用双重验证。

<!-- source: 6-go-through-the-checklist 12999c41613d -->
### 6. 完成检查清单

请完成[生产环境检查清单](production-checklist.md)，从[邮件](production-checklist.md#email)开始：`mail.from` 所在域名的 SPF、DKIM 和 DMARC。

<!-- source: the-console 1b3d6dfa9c3c -->
## 控制台

控制台使用服务的镜像、设置和机密运行一条 `my` 命令。它只在 `console_enabled` 打开期间存在，所以完成后请关闭它：

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` 会等待该命令，打印它的输出（它从平台的日志中读回），并以该命令的退出码退出。它使用服务正在运行的镜像：在停在迁移的升级之后，这就是上一个发布版本，数据库仍与之匹配。

该命令没有标准输入，也没有终端，所以需要从中读取密码或文件的命令无法这样运行。它打印的内容会留在日志中，所以不要运行输出内容不适合写进日志的命令。

<!-- source: upgrading 4be8a01734ce -->
## 升级

请先阅读[升级](database.md#upgrading)和发布说明，并且一次只升一个次版本。升级就是对新发布版本执行一次 apply：

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

请阅读计划：新发布版本的模块也可能更改资源。apply 会依次执行以下步骤，并在第一个失败的步骤处停止：

1. 为迁移作业和控制台作业（在 AWS 上是 task 定义）换上新镜像，同时应用计划中服务不依赖的其余部分。
2. 通过 `migrate.sh`，从你执行 apply 的地方在迁移作业中运行 `my upgrade`，并等待它完成，方式与[首次部署](#3-apply)相同。它会先等待被中断的 apply 遗留下来、仍在运行的任何一次运行。
3. 先让每个 worker 换上新镜像，然后是服务。新修订版本只有在健康检查通过后才会接收流量。

**你执行 apply 的地方**需要 `jq`、平台的 CLI，以及可以运行该作业并读取其日志的凭据，平台的页面列出了这些权限。执行 apply 的 CI 作业也是如此。

**某个步骤失败时**，各服务会停留在它们当前的修订版本上，发布说明或迁移的输出会说明原因。请修复原因，并用相同的版本再次运行 `tofu apply`：它会再次运行迁移，而已经完成的步骤会发现无事可做。对同一个镜像执行 apply 不会运行任何东西。要在 schema 更改之后回退，请恢复备份：参见[回滚](database.md#rolling-back)。对更早的版本执行 apply 会停在迁移处，因为那个发布版本会拒绝更新的 schema，并且不会更改任何服务。

要试用候选发布版本，或者在更新的模块下保留更早发布版本的镜像，请自己设置版本，即发布版本去掉 `v` 的镜像标签：

```hcl
  image = { version = "0.2.0-rc.1" }
```

<!-- source: checking-the-sql-first 84178f101ce0 -->
### 先检查 SQL

计划不会显示迁移的 SQL。若要在 apply 之前阅读它，请用新镜像单独 apply 迁移作业，在其中运行不会改变任何内容的 `my upgrade -dry-run`，并阅读它的日志。平台的页面给出了具体命令。

<!-- source: destructive-changes 0bea88d60224 -->
### 破坏性变更

当发布说明要求使用 `-allow-destructive` 时，请在块中指明该版本：

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

只有在该版本的 apply 中，`my upgrade` 才会得到 `-allow-destructive`，所以遗留下来的值不会让之后的任何发布版本删除数据。

<!-- source: pinning-the-image-by-digest 9c3f49e5ee10 -->
### 按摘要固定镜像

[生产环境检查清单](production-checklist.md#operations)建议使用发布说明中给出的摘要：

```hcl
  image = { digest = "sha256:..." }
```

摘要只对应一个镜像，不管旁边的标签怎么写，所以请在修改 `ref` 的同一次编辑中更改它：遗留的上一个发布版本的摘要，会让新版本下仍使用那个发布版本的镜像。

<!-- source: the-maintenance-mode 57bd5487b045 -->
### 维护模式

[维护模式](maintenance.md)需要执行三次 apply，因为服务在升级之前和之后都会改变。先把它加入块的设置并 apply，做好备份，再提高 `ref` 并 apply，然后移除它并 apply：

```hcl
  settings = { maintenance = "true" }
```

没有它时，旧的修订版本会在迁移期间继续提供服务，而这[并不保证能正常工作](database.md#the-server-during-an-upgrade)。

<!-- source: splitting-jobs-into-workers 6336c0873c82 -->
## 把任务拆分给多个 worker

当某一类任务拖慢了其余任务时，请给它单独一个服务。要拆分什么，请根据指标来决定，如[按任务类型拆分 worker](running.md#splitting-workers-by-job-type)所述。`workers` 接受一个以名称命名每个 worker 的键的映射，其中带有它的 `job_types`，模块会据此生成这次拆分所需的每个队列、路由和设置，所以你一个都不用写。要避免投递拖慢其余任务：

```hcl
  workers = {
    delivery = { job_types = ["federation.fanout", "federation.deliver"] }
  }
```

要让邮件和通知保持快速：

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
    }
  }
```

对于必须放在一起的类型，计划会拒绝拆分。请像那一页所说的那样，省去你没有设置的那些设置所对应的类型：服务器没有注册的类型会让服务无法启动，而计划看不出哪些类型是这样的。默认服务运行所有没有被任何 worker 指名的类型，而不运行被某个 worker 指名的类型。

worker 还可以设置它所在服务的大小，没有指定的地方都沿用默认服务的大小：[模块的参考](../../deploy/opentofu/README.md#workers)列出了各平台的字段。worker 不接收来自外部的流量，升级时它会在迁移之后、默认服务之前被切换。每个 worker 都可能带来服务所需的费用，而它的各个实例的连接数都会计入数据库的连接上限。

<!-- source: changing-a-backend df886f733f8e -->
## 更换后端

更改其区段的 `kind`。每个种类都会在服务器上设置它所需要的内容，计划会说明哪些内容会改变。更换数据库还意味着迁移它的数据，因为新的数据库是空的：请为旧数据库做一次[备份](backup.md)，并在没有任何写入时把它恢复到新数据库中。

<!-- source: the-kek 62356f591998 -->
### KEK

更改 `kek.kind` 只需要一行，但在已有数据的实例上并非没有代价：数据库中保存着用当前密钥封装的机密，其中包括每个账号的密钥，所以丢失它的实例就再也无法读取这些机密。请按照[在本地 KEK 与 KMS 之间迁移](key-management.md#move-between-the-local-kek-and-a-kms)操作。从机密迁移到 KMS 时，请在机密迁移期间把当前 KEK 保留为 `kek_previous`：添加 `secrets = ["kek_previous"]`，并用当前 KEK 填写机密 `<name>-kek-previous`。通过[控制台](#the-console)运行 `my kek rewrap`，当 `my kek status` 不再统计 `local:` 密钥时再移除它。只要你还保留着重新封装之前的数据库备份，就请一直保留这个 KEK。

在没有数据的新实例上，不需要迁移任何东西。
