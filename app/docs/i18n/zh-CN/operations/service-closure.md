<!-- source: closing-the-service 30c438995ba1 -->
# 关闭服务

本文按顺序说明停止运营服务器时要做的事：先让用户把数据带走，再删除仍留在其他服务器上的账号，然后停止服务器，切换到静态的关闭页面。各条命令的详细说明见[管理员与角色](admin-cli.md#announcing-the-end-of-the-service)。

<!-- source: the-sequence 4557c35b6851 -->
## 整体流程

![自上而下展示关闭服务的各个阶段：左侧是运营者要做的事，右侧是服务器的状态，两者之间是关闭日期和导出截止日期。下表说明了每个阶段。](images/service-closure.svg)

下表按图中从上到下的顺序排列；“服务器”一列以 `my admin closure status` 显示的状态开头。关闭日期（`-closes-at`）结束预告期，导出截止日期（`-export-until`，未设置时为关闭日期后 30 天）结束第 5 阶段。

| 阶段 | 服务器 | 运营者要做的事 |
| --- | --- | --- |
| 1. 公告 | `announced`。照常运行。停止注册、邀请和新的支持者订阅 | `my admin closure announce`，在状态页面上发布通知 |
| 2. 预告期 | `announced`。照常运行。用户可以导出数据并迁移到其他服务器 | 给年付支持者退款，检查迁移和保留义务冻结 |
| 3. 关闭（关闭日期之后） | `closed`。用户只能登录和导出。服务器为每个账号投递 `Delete` | `my admin closure check` 和 `close` |
| 4. 投递完成 | 记录 `delivery_completed`；本站 Actor 也返回 `410` | 用 `my admin closure status` 确认。不要停止 worker |
| 5. 导出截止日期之前 | `delivery_completed`。保持关闭状态，继续提供导出 | 导出保留义务冻结所涉及的记录 |
| 6. 静态页面 | 已停止。前端提供静态页面，对联邦请求返回 `410` | `my admin closure export-html`，切换前端，停止 `my serve` |
| 7. 销毁 | 无。继续提供静态页面和 `410` 约一年 | 销毁数据库、blob 和备份。保留域名约一年 |

任何阶段都可以与维护模式（[维护](maintenance.md)）结合使用。维护期间，导出和 `Delete` 活动的投递会停止。

<!-- source: 1-announce b34cb9474667 -->
## 1. 公告

确定关闭日期（`-closes-at`）和带走数据的截止日期（`-export-until`，省略时为关闭日期后 30 天），然后发布公告。

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for being here."
my admin closure status
```

- 关闭日期没有下限，但从公告到关闭日期不足 30 天时会显示警告。请给用户留出导出和迁移的时间。
- 要更改日期，再次运行 `announce`；要取消，运行 `my admin closure withdraw`。两者都会向每位用户发送 `service_closure` 通知。
- 一旦发布公告，实际生效的注册模式变为 `closed`，待处理的申请会被拒绝，所有持续中的支持者订阅都会在当期结束时取消（订阅者会收到通知和邮件）。撤回公告不会撤销这些拒绝和取消。
- 通知由任务创建，所以不要停止 worker。
- 在状态页面（[监控与状态页面](monitoring.md)）上发布关闭日期、导出截止日期，以及导出和迁移的方法。状态页面比服务器存在得更久，所以也能为关闭后来访的人提供指引。

<!-- source: 2-notice-period b18535a9c0f7 -->
## 2. 预告期

- 用户可以在设置界面导出数据（数据导出和 CSV 文件），并迁移到其他服务器（`Move`）。每个界面上的横幅和一条通知会显示关闭日期、导出截止日期以及前往该界面的方法。
- **年付支持者**：公告会在当期结束时取消订阅，但对于当期在关闭日期之后才结束的订阅，无法使用的那段时间已经付过费了。服务器不会退款，所以请在支付服务商的界面中逐一给订阅者退款（Stripe 在控制台的付款界面，PAY.JP 在管理控制台的付款界面）（[支持者付款（Stripe）](payments-stripe.md)、[支持者付款（PAY.JP）](payments-payjp.md)）。
- 关闭日期过后，拥有 `manage_service_closure` 的人会收到 `admin_service_closure_due` 通知。服务器不会自行关闭。

<!-- source: 3-close c851780cf430 -->
## 3. 关闭

关闭日期过后，先运行检查，再关闭。

```sh
my admin closure check
```

- **未完成的迁移**会产生警告。请等它们完成后再关闭，因为关闭后任何人都无法发起 `Move`。
- 任何账号上存在**保留义务冻结**（`retention_holds`）都会产生警告。冻结背后的义务不会因为关闭服务器而结束，所以请导出冻结所涉及的记录并保存在服务器之外（用 `my admin retention list` 列出冻结）。

  ```sh
  my admin account export-moderation @alice > alice-moderation.json
  ```

检查完成后执行关闭。此操作无法撤销。输入 `y` 和域名进行确认（自动化时使用 `-confirm-domain`）。

```sh
my admin closure close
```

关闭后，用户只能登录和导出数据。剩余的支持者订阅会立即取消，并且会向其他服务器投递每个本站账号的 `Delete`（Actor）（即使 `federation_mode` 为 `off` 或 `inbound` 也是如此）。收件箱以及对帖子和集合的 GET 请求返回 `410 Gone`。

<!-- source: 4-confirming-that-delivery-is-complete b545acd9a55c -->
## 4. 确认投递完成

**在状态变为 `delivery_completed` 之前，不要停止 `my serve` 和 worker（`role` 为 `all` 或 `worker` 的进程；使用推送式队列时为 Cloud Tasks 队列）。**
worker 负责投递 `Delete` 活动，在此期间服务器会继续返回本站 Actor 的文档和密钥，以便接收方验证 `Delete` 的签名。如果过早停止，这些账号就会留在其他服务器上。

```sh
my admin closure status
```

- `accounts with a Delete` 是已记录 `Delete` 的账号数，`fan-outs pending` 是剩余的扇出工作，`deliveries <state>` 是各状态的投递数。
- 投递按常规策略重试（约 4 天内最多 9 次）。全部结束后，每 10 分钟运行一次的检查会记录 `delivery_completed`（`state: delivery_completed`）。此后本站 Actor 和实例 Actor 也会返回 `410`。
- 变为 `dead` 的投递不会自动重试。要重试，请使用 `my job retry`。

<!-- source: 5-until-the-export-deadline 6464f67f67d1 -->
## 5. 导出截止日期之前

即使投递已经完成，服务器也会以关闭状态继续运行到 `export_until`，并继续为用户提供导出。服务器从不自行删除数据，所以过了截止日期也会一直运行，直到你停止它。如果你还没有导出保留义务冻结所涉及的记录（§3），现在就导出。

<!-- source: 6-switching-to-the-static-page 7735b2587738 -->
## 6. 切换到静态页面

导出截止日期过后，导出关闭页面。**此操作会读取数据库，所以请在停止 `my serve` 和销毁数据库之前运行。**

```sh
my admin closure export-html -out ./closure             # all locales; index.html starts from the instance's default locale
my admin closure export-html -out ./closure -lang ja,en # set the locales and the order in index.html
```

- 输出的内容：
  - `<dir>/<locale>.html`，以及按顺序列出所有语言的 `<dir>/index.html`。它们使用与维护页面（`my maintenance export`）相同的模板和 CSS，显示服务器名称（`maintenance_server_name`，没有时为实例名称，再没有时为主机名）、关闭日期、运营者在公告中写的消息，以及指向状态页面（`status_page_url`）的链接。CSS 仅为内联，没有脚本或外部资源，CSP 写在 `<meta>` 标签中。主题颜色的覆盖设置会被写入页面；自定义 CSS 不包含在内。
  - 前端配置示例 `nginx.conf`、`Caddyfile` 和 `cloudfront-function.js`。它们对联邦和发现路径（`/ap/*`、`/inbox`、`/.well-known/webfinger`、`/.well-known/host-meta`、`/.well-known/host-meta.json`、`/.well-known/nodeinfo`、`/nodeinfo/*`）以及个人资料（`/@*`）返回不带正文的 `410`，对其他所有路径返回 `Accept-Language` 中第一个语言的页面（没有时返回 `index.html`）。每种语言会得到应用会为它显示的那个语言的页面：`zh`、`zh-SG` 和 `zh-Hans` 得到 `zh-CN.html`，而 `zh-TW` 不会。
- 如果服务器尚未关闭，该命令会拒绝执行（退出码 1）。在 `delivery_completed` 之前，它会警告投递仍在进行（但仍会输出文件）。

切换前端。只把页面（`*.html`）放到公开位置，并按你的环境调整示例中的主机名、目录和证书路径。

| 前端 | 设置方法 |
| --- | --- |
| nginx | 把页面放在 `/srv/closure`，并在 `http` 块中 include `nginx.conf`。页面也以 `410` 返回 |
| Caddy | 把页面放在 `/srv/closure`，并使用 `Caddyfile`。Caddy 会自动获取证书，页面也以 `410` 返回 |
| CloudFront | 把页面直接放在 S3 存储桶下，将该存储桶设为默认缓存行为的源，并把 `cloudfront-function.js` 关联为 viewer-request 类型的 CloudFront Function（`cloudfront-js-2.0`）。在缓存策略的缓存键中加入 `Accept-Language`。页面以 `200` 返回（viewer-request 函数无法更改源响应的状态码） |

切换后，确认 `curl -i https://social.example/.well-known/webfinger?resource=acct:alice@social.example` 返回 `410`，并且 `curl -i https://social.example/` 返回关闭页面。然后停止 `my serve` 和 worker。对于可用性监控，要么停止健康检查监控，要么把它们指向关闭页面。

<!-- source: 7-discarding-the-db-blobs-and-backups c7c55aac56e0 -->
## 7. 销毁数据库、blob 和备份

停止后，销毁数据库、blob（媒体和导出文件）、KV 存储和队列，以及它们的备份和快照。如果使用了[搜索引擎](search.md#closing-the-service)，也要销毁它的索引和快照：其中保存着帖子的文本。在销毁任何东西之前，请再次确认你已经导出并保存了保留义务冻结所涉及的记录。销毁无法撤销。

**保留域名，并继续提供静态页面和 `410` 约一年。** 其他服务器看到 `410` 后会停止获取和投递，并清除缓存。联邦 URI 使用 UUID，所以再次创建出相同 `id` 的 Actor 的风险很小，但如果别人取得了这个域名，账号的用户名（`acct:alice@social.example`）和个人资料 URL 就归对方所有，可能被用来冒充你的用户。请持续续订域名和证书，即使一年过后，在放弃域名之前也请再三考虑。
