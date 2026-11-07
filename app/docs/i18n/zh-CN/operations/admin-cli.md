<!-- source: administrators-and-roles 1baeba6d67b4 -->
# 管理员和角色

本页介绍如何创建第一个管理员、在命令行中管理角色、公告并执行服务关停、重新处理已上传的图片、重建搜索索引，以及导出维护页面。

<!-- source: prerequisites b7ae62128809 -->
## 前提条件

- 已按照[配置参考](../reference/configuration.md)的说明设置好配置（`host`、`database`、`database_url`、`kek` 等）。`my config check` 可以验证配置。
- 已将 schema 应用到数据库（`my schema apply`；参见 [database.md](database.md)）。
- `host` 和 `account_domain` 已经最终确定。第一个使用数据库的命令会记录它们，此后只要配置与记录不同，所有命令都会拒绝运行（参见[之后无法更改的设置](production-checklist.md#settings-you-cannot-change-later)）。

<!-- source: creating-an-administrator e5fec2e3d287 -->
## 创建管理员

```sh
my admin create -username alice -email alice@example.com
```

- 在终端中运行时，命令会要求输入两次密码，且不回显。
- 当标准输入不是终端时，标准输入的第一行就是密码（例如 `my admin create ... < password.txt`）。密码不能作为命令行参数或环境变量传入，因为它们会出现在进程列表和 shell 历史中。
- 如果一次性命令既没有终端也没有标准输入，例如 [Cloud Run 任务](deploy-cloud-run.md#9-create-the-first-administrator)，请加上 `-password-link`。这时命令不读取密码：它会设置一个无人知晓的随机密码，并在 `created admin` 行之后打印一个链接，该链接可在一小时内设置一次密码，与 `my admin account password-reset-link` 相同（审计日志将其记录为 `password_reset_link_issued`）。任何能读到命令输出的人都可以使用该链接，而在这类平台上，输出会一直保留在平台日志中，直到链接被使用或过期。请立即打开链接。如果链接过期，用 `my admin account password-reset-link @alice` 再签发一个。如果签发链接失败，管理员已经创建：命令会说明这一点并以退出码 1 退出，再运行同一命令即可获得新链接。
- 密码长度为 8 到 128 个字符。密码永远不会出现在输出、日志或错误信息中。
- 用户、密码、主邮箱地址、本地账号（及其密钥）和 `admin` 角色的授予（没有授予者，表示由 CLI 授予）在同一个事务中创建。如果内置角色不存在，会先创建它们。
- 成功时，命令打印 `created admin @alice@<account_domain> (user <id>, account <id>)` 并以退出码 0 退出。
- 在[单人服务器](instance-settings.md#one-person-server)上，命令不会创建账号，并以退出码 1 退出。当[登录仅限于登录提供方](login-providers.md#limit-sign-in-to-login-providers)时，命令还会警告：在该限制解除之前，这个密码无法用于登录。

<!-- source: running-it-again 197692f7f103 -->
## 再次运行

- 如果用户名或邮箱地址（均不区分大小写）已被占用，命令不做任何更改，打印被占用的是哪一项，并以退出码 1 退出。它从不更改已有用户的密码或角色。
- 失败的运行不会写入任何内容，因此你可以修正原因后再次运行同一命令。
- 退出码 2 表示用法错误。

<!-- source: granting-and-revoking-roles b87e1adb131e -->
## 授予和撤销角色

角色持有权限。内置角色有 `admin`（`position` 1000，拥有所有权限）和 `moderator`（`position` 100，拥有 `manage_reports`、`manage_accounts` 和 `view_audit`）。`my serve` 每次启动时（无论进程的 `role` 是什么）都会检查内置角色，`my admin role` 每次运行时也会检查。它们会创建缺失的部分，并赋予 `admin` 当前运行版本的所有权限。升级新增权限时无需手动操作。`moderator` 会保留管理员设置的权限和位置。

```sh
my admin role list                 # each role's position, permissions (grouped as moderation, server administration, and member features), and holders
my admin role list -user alice     # the roles of alice
my admin role grant alice moderator
my admin role revoke alice moderator
```

- 权限分属以下三组之一。
  - 审核：`manage_reports`、`manage_accounts`、`manage_domains`、`manage_invites`、`view_audit`。
  - 服务器管理：`manage_settings`、`manage_rules`、`manage_roles`、`manage_jobs`、`manage_login_recovery`、`manage_custom_emojis`、`manage_webhooks`、`manage_announcements`、`manage_finance`、`manage_supporters`、`manage_oauth_apps`、`manage_service_closure`、`manage_errors`、`manage_login_providers`。
  - 成员功能：`view_finance`、`create_custom_emojis`、`start_voice_rooms`、`create_invites`、`keep_posts_longer`。

  `list` 为每组打印一列（`MODERATION`、`SERVER`、`MEMBER`）。Web 客户端将前两组显示为“审核”和“管理”，成员功能不计入其中任何一组（参见 [moderation.md](moderation.md)）。
- 更改在该用户的下一次请求时生效。面向审核员的通知会发送给创建通知时持有相应权限的人。
- 授予和撤销会以 `role_granted` 和 `role_revoked`（目标类型 `role`）写入审计日志。CLI 操作的执行者是 `cli`（`my admin audit list -target-kind role`）。
- 授予用户已有的角色，或撤销其没有的角色，不会更改或记录任何内容。
- `admin` 的最后一个持有者不能失去该角色（退出码 1）。请先把 `admin` 授予另一个用户。
- 管理 API 是 `GET /api/v1/admin/roles`、`GET /api/v1/admin/accounts/{id}/roles` 以及 `PUT` / `DELETE /api/v1/admin/accounts/{id}/roles/{role}`，需要 `manage_roles` 权限。通过 API，你不能授予或撤销高于你自己角色中最高 `position` 的角色（403）。撤销最后一个 `admin` 会返回 409。
- 工作人员只能对级别更低的人执行操作。通过 API 执行审核操作及其解除、处理申诉、解除邮件抑制、查看账号的私密数据、签发密码重置链接以及移除双重验证时，你角色中的最高 `position` 必须高于目标的每一个角色；对级别更高或相同的人（包括你自己的账号）执行会返回 403。审核员不能对管理员执行操作，管理员之间也不能互相执行。这些命令（`my admin account ...`）没有这种限制，因此在管理员之间意见不一致时请使用它们。
- 唯一的管理员不能被锁定或移除：对其执行 `disable_login`、`suspend` 和 `ban` 会被拒绝，无论是通过 API（409）还是这些命令（退出码 1），而且其不能删除自己的账号（409）。另一个 `admin` 持有者只有在能够登录时才算数：删除账号之后，或处于停用、封禁或 `disable_login` 状态时都不算。

<!-- source: announcing-the-end-of-the-service e1c40ec145f0 -->
## 公告服务关停

要关闭服务器，请先公告关停。[service-closure.md](service-closure.md) 介绍了从公告到丢弃数据的完整流程。公告期间服务器照常运行。`GET /api/v1/instance` 中的 `closure` 和 NodeInfo 中的 `metadata.serviceClosure` 会显示状态和日期。

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for using this server."
my admin closure announce -closes-at 2027-02-28 -export-until 2027-04-30   # change the dates (a bare date means 00:00 UTC)
my admin closure status                                                   # the state and the schedule in effect
my admin closure withdraw                                                 # withdraw the announcement
```

- `-closes-at` 没有下限。`-export-until`（导出数据的截止日期）必须不早于 `-closes-at`，默认为关停日期后 30 天。
- 当关停日期距今不足 30 天时，命令会向标准错误输出一条警告（公告仍会被记录）。
- 更改日期会记录为一次新的公告，撤回后再发布的公告也是新的公告。服务器关闭之后，既不能公告也不能撤回（退出码 1）。
- 公告、更改日期和撤回会以 `service_closure_announced` 和 `service_closure_withdrawn`（目标类型 `instance`；参见 `my admin audit list -target-kind instance`）写入审计日志。
- 管理 API 是 `GET /api/v1/admin/closure`、`PUT` / `DELETE /api/v1/admin/closure/announcement`、`GET /api/v1/admin/closure/check` 和 `POST /api/v1/admin/closure/close`。它需要 `manage_service_closure` 权限（内置角色中只有 `admin` 拥有）。
- 公告期间，服务器不接受新用户，也不接受新的付款。
  - 实际生效的注册模式为 `closed`（`registration_mode` 的值不变，撤回公告后会重新生效）。注册和使用邀请注册都不可用，待处理的申请也无法批准（`409`）。
  - 公告会拒绝所有待处理的申请（记录为由 `service_closure` 决定），并在当前周期结束时取消所有进行中的支持者订阅。订阅者会收到通知（`supporter_service_closure`）和邮件。新的订阅返回 `409`。撤回公告不会撤销这些拒绝或取消。
  - 每次公告、更改日期和撤回都会向所有本地用户发送 `service_closure` 通知。这些都以任务运行，因此不要停止 worker。
  - 每个已确认邮箱地址的用户都会在以下时间收到邮件：公告、更改日期、撤回、关停日期前 7 天和 1 天、关停时，以及导出截止日期前 7 天。用户无法退订（参见 [email.md](email.md)）。
  - 关停日期过后，持有 `manage_service_closure` 的人会收到 `admin_service_closure_due` 通知。服务器不会自行关闭。

关停日期过后，请关闭服务器。关闭无法撤销。

```sh
my admin closure check                                      # review what the closure ends
my admin closure close                                      # show the review, then ask for y and the domain name
my admin closure close -confirm-domain social.example       # for automation: pass the domain name instead of answering
```

- `check` 和 `close` 会显示本地账号数、目标收件箱数（已知的共享收件箱）、要发送的 `Delete` 活动的估计数量（两者之积）、进行中的支持者订阅数、未完成的迁移（`Move`）数，以及处于保留冻结（`retention_holds`）的账号数。如果仍有迁移或存在保留冻结，命令会在标准错误中发出警告。请用 `my admin account export-moderation` 导出处于冻结状态的账号记录，并在丢弃数据之前妥善保存。
- 没有公告时以及关停日期之前，`close` 会不经询问直接拒绝（退出码 1）。除 `y` 以外的任何回答，或与服务器域名（配置中的 `host`，不区分大小写）不一致的输入，都不会更改任何内容，并以退出码 1 退出。
- 关闭会以 `service_closure_closed` 写入审计日志。在同一事务中，它会结束所有进行中的支持者订阅（原因为 `service_closed`），并将立即在支付服务商处取消订阅的任务加入队列（`CancelNow`）。
- 关闭之后，客户端 API 只放行以下请求：读取你自己的账号和设置、导出（`/exports`、`/csv_exports`）、会话（`/sessions`、`/authorized_apps`）、`GET /instance`，以及关停和任务队列的管理。其他请求一律以 `403`（`/api/v1/problems/service-closed`）拒绝。流会停止，Web Push 不再发送任何内容。登录和重置密码仍然可用。
- 关闭之后，worker 会把每个本地账号的 actor 的 `Delete` 投递给联邦对端，每次 100 个（已迁移的账号最后处理）。即使 `federation_mode` 为 `off` 或 `inbound` 也会发送，但从不发送给已停用的域名。`my admin closure status`（以及 `GET /api/v1/admin/closure` 中的 `progress`）会显示已记录 `Delete` 的账号数、尚待展开的投递数，以及各状态（`pending`、`succeeded`、`permanent_failure`、`skipped`、`dead`）的投递数。投递期间请保持 worker 运行。所有投递都完成后，每 10 分钟运行一次的检查会记录 `delivery_completed`（审计日志中为 `service_closure_delivery_completed`），之后本地 actor 会返回 `410`。服务器不会自行重新运行 `dead` 的投递；请用 `my job retry` 重新运行。

导出截止日期过后，生成服务器停止后由你的域名提供的静态关停页面。该命令会读取数据库，因此请在丢弃数据库之前运行。

```sh
my admin closure export-html -out ./closure              # every locale (the instance's default locale first)
my admin closure export-html -out ./closure -lang ja,en  # choose the locales and the order in index.html
```

- 输出：每个语言区域一个 `<dir>/<locale>.html`，以及依次包含所有语言区域的 `<dir>/index.html`。这些页面使用与 `my maintenance export` 相同的模板，显示服务器名称、关停日期、运营者在公告中的消息以及 `status_page_url`。输出中还包含前端的配置示例：`nginx.conf`、`Caddyfile` 和 `cloudfront-function.js`。它们对 `/ap/*`、`/inbox`、WebFinger、host-meta、NodeInfo 和 `/@*` 返回 `410`，其他请求则提供关停页面。
- 除非服务器已关闭，否则命令会拒绝运行（退出码 1）。在 `delivery_completed` 之前，它会打印警告，但仍会生成页面。[service-closure.md](service-closure.md) 介绍了如何把前端切换到静态页面。

<!-- source: processing-uploaded-images-again 65a9e3da0bd8 -->
## 重新处理已上传的图片

每张上传的图片保存三种尺寸，动图还会保存其动态版本。`my` 的新版本可能会更改这些尺寸或格式，而已有图片在你重新处理之前会保留原有文件：

```sh
my admin media reprocess -before 2027-01-31T09:00:00Z
```

- `-before` 接受更改发生的时间（例如你升级的时间），格式为 RFC 3339。只处理最后一次处理早于该时间的图片，尚未到来的时间会被拒绝。
- 工作由 worker 完成，每分钟 100 张图片，每张图片一个单独的任务（[任务参考](../reference/jobs.md)中的 `media.reprocess_walk` 和 `media.reprocess`）。请保持 worker 运行；管理任务页面（`/admin/jobs`）会显示队列。
- 用相同的时间再次运行是安全的。已处理的图片不会被改动，第一次运行没有处理到的图片会被接着处理。
- 原图不会保留，因此每张图片都从其大尺寸版本重新生成。大尺寸版本已丢失的细节无法恢复，而且每次用更晚的时间运行都会再重新编码一次，损失一点画质。每次更改只运行一次。
- 视频和音频文件不会重新编码，保持不变。当前限制不再接受的图片会保留现有版本，worker 会记录一条警告。

图片的新版本有新的 URL。旧文件会保留 `media_replaced_retention` 的时长（默认 30 天，[配置参考](../reference/configuration.md#media_replaced_retention)），以便已打开的页面和缓存继续可用，之后每小时运行的任务 `media.purge_replaced` 会删除它们。其他服务器不会收到新 URL 的通知：仍持有旧 URL 且没有自己副本的服务器在这段时间之后将不再显示该图片。在旧文件删除之前，请预留额外的存储空间。

<!-- source: rebuilding-the-search-index 9061821656ac -->
## 重建搜索索引

`my search` 在配置了搜索引擎的服务器上管理帖子全文搜索的索引。当 `search = none` 时，每个命令都会说明这一点，不做任何操作，并以退出码 0 退出。

```sh
my search rebuild              # empty the index, then have the worker add every post that qualifies
my search rebuild --keep       # add them without emptying the index
my search rebuild --rate 50    # at most 50 posts a second (20 by default, 1000 at most)
my search clear                # empty the index and end any rebuild
my search status               # the backend and the progress and failures of the latest rebuild
```

再次运行 `my search rebuild` 可以恢复已停止的重建。它会从最后处理的帖子继续，不会再次清空索引。在一次重建未完成期间，另一种模式会以退出码 1 被拒绝。[帖子全文搜索](search.md)说明了何时使用哪种模式。

<!-- source: exporting-the-maintenance-page 1756fcf1e02e -->
## 导出维护页面

`my maintenance export` 将维护模式的页面写成静态 HTML，供负载均衡器、CDN 或反向代理在 `my serve` 本身无法运行时（例如正在替换镜像、某个修订版无法启动）提供。它使用与维护模式相同的模板，因此两者显示的文字和外观相同。

```sh
my maintenance export -out ./maintenance -lang en,ja            # reads the database for at most 5 seconds
my maintenance export -out ./maintenance -lang en,ja -offline   # the configuration and the default theme only
```

- 输出：`-lang` 中每个语言区域一个 `<dir>/<locale>.html`，供按 `Accept-Language` 选择页面的前端使用；以及依次包含所有语言区域的 `<dir>/index.html`，供无法这样选择的前端使用。这些页面使用内联 CSS，不含脚本和外部资源；其 Content-Security-Policy 写在 `<meta>` 元素中。
- 文字：来自设置 `maintenance_server_name`、`maintenance_message`、`maintenance_until` 和 `status_page_url`（[配置参考](../reference/configuration.md)）。
- 不加 `-offline` 时，它还会读取实例的名称（当 `maintenance_server_name` 未设置时）、默认语言区域（当未指定 `-lang` 时）以及主题的令牌覆盖值。主题的自定义 CSS 永远不会包含在内。当无法读取数据库时，它会打印警告，根据配置和默认主题生成页面，并以退出码 0 退出。
- 每当这些设置或主题发生变化时，以及每次维护之前，都请重新运行它，使前端中的页面保持最新。
