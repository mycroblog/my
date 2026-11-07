<!-- source: account-and-domain-actions-and-handling-reports 59d39b2e3f37 -->
# 账号和域名操作，以及举报处理

本文说明运营者和审核员如何对账号和域名采取操作、裁决申诉、处理举报以及查看审计日志。
除账号管理（仅限管理 API）之外，这里的一切都可以通过管理 API（`/api/v1/admin/...`，scope 为 `admin:read` / `admin:write`）和 CLI（`my admin account action` / `my admin domain` / `my admin report` / `my admin appeal`）完成。
管理 API 需要角色权限：账号管理、操作和申诉需要 `manage_accounts`，域名操作需要 `manage_domains`，举报需要 `manage_reports`。
CLI 使用与服务器相同的配置（`-config` 或环境变量）直接连接数据库，不记录是哪个用户执行的（`issued_by_user_id` 等字段为 NULL，[审计日志](#audit-log)中的执行者为 `cli`）。

<!-- source: from-a-report-to-an-action-and-an-appeal dabd0eabc7c7 -->
## 从举报到操作和申诉

![举报的流程：举报进来，审核员审查，之后可能采取操作并通知用户，用户可以申诉；右栏显示每一步向其他服务器发送的内容。下面的列表说明了每一步。](images/moderation-lifecycle.svg)

按图中从上到下的顺序：

1. **收到举报。** 本站用户提交举报，其他服务器发来 `Flag`（如果其域名设置了 `reject_reports` 则丢弃，见[域名操作](#domain-actions)），
   或者[决策钩子](decision-hooks.md)回答 `flag`（[外部审查提交的举报](#reports-that-an-external-review-filed)）。本站用户举报其他服务器上的账号时，
   可以选择转发举报，这会立即向该服务器发送 `Flag`。
2. **审查。** 举报处于 `open` 状态，审核员会收到 `admin_report` 通知。处理后举报变为 `resolved`，结果为 `actioned` 或 `dismissed`，
   本站举报者会收到 `report_resolved` 通知。重新打开会让它回到 `open`。转发关于其他服务器上账号的举报会向该服务器发送 `Flag`
   （[举报](#reports)）。
3. **操作。** 审核员从举报发起[账号操作](#account-actions)，举报随之以 `actioned` 处理完毕；也可以不经举报直接发起操作。
   操作有类型、理由和结束时间，在被解除或到期时结束。设置 `notify_target` 时，用户会收到 `moderation_notice` 通知
   （[用户看到的内容](#what-the-user-sees)）。其他服务器会收到：`delete_statuses` 时帖子的 `Delete`；本站账号的 `suspend` 或 `ban`
   开始或结束时 actor 的 `Update`；远程账号的 `suspend` 切断关注时这些关注的 `Reject` / `Undo`。
4. **申诉。** 本站用户对操作提出申诉。申诉处于 `pending` 状态，审核员会收到 `admin_appeal` 通知。裁决为 `upheld` 或 `overturned`，
   用户会收到 `appeal_decided` 通知。推翻会解除操作并取消 `ban` 的删除，因此推翻本站账号的 `suspend` 或 `ban` 会发送 actor 的 `Update`
   （[申诉](#appeals)）。

分配、备注、通知和申诉都留在本服务器上；只有右栏中的活动会离开它。

<!-- source: moderation-and-server-administration-in-the-web-client 3beb1c9760a0 -->
## Web 客户端中的审核与服务器管理

Web 客户端把工作人员界面分成两组。
只有角色至少拥有某一组中一项权限的人，导航中才会显示“审核”和“管理”。没有权限的人在每个界面上只会看到原因，服务器也会拒绝他们的请求。

| 组 | 权限 | 界面 |
| --- | --- | --- |
| 审核（`/moderation`） | `manage_reports`、`manage_accounts`、`manage_domains`、`manage_invites`、`view_audit` | 举报（`/moderation/reports`）、申诉（`/moderation/appeals`）、账号（`/moderation/accounts`）、注册（`/moderation/registrations`）、注册屏蔽（`/moderation/signup-blocks`）、邀请（`/moderation/invites`）、域名操作（`/moderation/domain-actions`）、远程服务器（`/moderation/instances`）、审计日志（`/moderation/audit-log`） |
| 服务器管理（`/admin`） | `manage_settings`、`manage_rules`、`manage_roles`、`manage_jobs`、`manage_login_recovery`、`manage_custom_emojis`、`manage_webhooks`、`manage_announcements`、`manage_finance`、`manage_oauth_apps`、`manage_service_closure`、`manage_errors`、`manage_login_providers` | 服务器设置（`/admin/settings`）、规则（`/admin/rules`）、条款和政策（`/admin/documents`）、任务队列和失败的任务（`/admin/jobs`）、自定义表情（`/admin/custom-emojis`）、公告（`/admin/announcements`）、Webhook（`/admin/webhooks`）、费用和账单导入（`/admin/finance`，`manage_finance`）。`manage_login_recovery`（登录恢复：移除双重验证和签发密码重置链接）和 `manage_roles`（授予和撤销账号的角色）显示为账号“管理”页面（`/moderation/accounts/{id}/manage`，还需要 `manage_accounts`）中的区块 |

- “审核”上的徽章是待处理举报、等待批准的注册和等待审核的申诉的总数（[审核员的通知和待处理数量](#notifications-for-moderators-and-the-pending-counts)）。“管理”不显示数量。
- 每一组都以概览页打开；各个界面在旁边的菜单中。审核概览为你的角色可计数的每个队列（待处理举报、等待批准的注册、等待裁决的申诉）各显示一个卡片，每个卡片链接到对应队列；还有一个卡片显示最早的待处理举报已等待多久（链接到按最早排序的举报列表）；并从整个队列中列出应优先处理的待处理举报：违法内容优先，其次是等待最久的，同时显示每条已等待多久以及由谁处理。
  举报列表可以按状态、类别和负责人（你自己或无人）筛选，并按最新或最早排序；筛选条件保存在页面地址中。
  管理概览显示服务器的健康状况：失败的任务、任务队列中最早的任务已等待多久，以及未处理的错误；每个卡片链接到对应界面，需要关注时会加以标记。
- 内置的 `moderator` 只有 `manage_reports`、`manage_accounts` 和 `view_audit`，看不到“管理”。它也看不到域名操作和远程服务器界面（`manage_domains`），
  所以要委派这些工作，请创建拥有该权限的角色并授予它（[授予和撤销角色](admin-cli.md#granting-and-revoking-roles)）。
- 无论属于哪一组，管理 API 的路径都是 `/api/v1/admin/...`，scope 也都是同一对 `admin:read` / `admin:write`。在 API 参考中，各组以标签区分（`admin-moderation`、`admin-server`）。
- 还有第三组权限“用户功能”（`view_finance`、`create_custom_emojis`、`start_voice_rooms`、`create_invites`、`keep_posts_longer`），它不属于工作人员界面。只拥有这些权限的人既看不到“审核”也看不到“管理”，
  拥有 `view_finance` 的人会在导航中“设置”之后看到“财务”（`/finance`）。
  它的 API 位于用户命名空间（`GET /api/v1/finance/cost_totals` 和 `GET /api/v1/finance/income_totals`，scope 为 `read:finance`），不需要 `admin:*` scope。显示的内容由[实例设置](instance-settings.md)中的 `finance_viewer_*` 决定。

<!-- source: account-actions a61dac7c5921 -->
## 账号操作

操作在记录它的同一事务中应用到限制投影（`account_restrictions`），从下一个请求起生效。

| 类型 | 对本站账号的效果 |
| --- | --- |
| `warn` | 不做限制（只通知用户） |
| `force_sensitive` | 将所有附件显示为敏感内容 |
| `limit` | 将帖子（包括过去的帖子）视为仅关注者可见，并使其不出现在公共时间线上 |
| `restrict_posting` | 禁止发帖、回复、转发和编辑 |
| `restrict_interaction` | 禁止关注、喜欢、转发和回复 |
| `disable_login` | 禁止使用应用和 API，并撤销有效的会话和令牌（用户仍可在登录页面查看该操作） |
| `suspend` | 施加以上所有限制，并对除用户本人以外的所有人隐藏个人资料和帖子 |
| `ban` | 施加与 `suspend` 相同的限制，并在 30 天后删除账号。无法解除（只有在这 30 天内提出的申诉被推翻时才会取消） |
| `delete_statuses` | 删除指定的帖子。帖子连用户本人也看不到，服务器会向其他服务器发送与用户自己删除时相同的 `Delete` |
| `mark_statuses_sensitive` | 将指定帖子的附件显示为敏感内容 |

对远程账号只能发起 `limit`、`suspend` 和 `force_sensitive`（其他类型在管理 API 中返回 `422`，在 CLI 中失败）。
效果仅限于本服务器。唯一发送给对方服务器的是被 `suspend` 切断的关注的 `Reject` / `Undo`。

| 类型 | 对远程账号的效果 |
| --- | --- |
| `force_sensitive` | 将所有附件显示为敏感内容 |
| `limit` | 将帖子（包括过去的帖子）视为仅关注者可见，使其不出现在公共时间线上，并且不为未关注该账号的用户创建通知 |
| `suspend` | 丢弃从该账号收到的一切，隐藏其个人资料和帖子。双向切断该账号与本站用户之间的关注（包括关注请求），并向对方服务器发送该账号关注的 `Reject` 和本站用户关注的 `Undo`。解除后恢复显示，服务器重新接收该账号的内容（封禁期间被丢弃的内容和被切断的关注不会恢复） |

- `notify_target`（`-no-notify`）对远程账号没有意义（通知不会送达）。
- 因 `suspend` 而失去关注的本站用户会收到一条通知（`relationships_severed`），告知失去了多少关注者和关注（[被切断的关注](#severed-follows)）。
- 与对本站账号的操作一样，发起和解除都会记录在[审计日志](#audit-log)中。

```sh
my admin account action issue -kind suspend -category spam -public-reason "spam" -duration 720h @alice
my admin account action issue -kind limit -report <report id> @bob@remote.example
my admin account action issue -kind delete_statuses -status <status id> -status <status id> @alice
my admin account action issue -kind ban -category legal -public-reason "illegal content" @alice
my admin account action list -active @alice
my admin account action lift -reason "appealed" <action id>
```

管理 API 中：`POST /api/v1/admin/accounts/{id}/actions`（请求体包含 `kind`、`reason_category`、`public_reason`、`private_note`、`report_id`、`notify_target`、以秒为单位的 `expires_in` 和 `status_ids`）、
`GET /api/v1/admin/accounts/{id}/actions`（用 `active` 筛选）、`GET /api/v1/admin/account_actions/{id}`，以及 `POST .../{id}/lift`（请求体 `{"reason": "..."}` 可选）。

- 在 CLI 中用 `@username`（本站）、`@username@domain`（远程）或账号 ID 指定账号，在管理 API 中用账号 ID 指定。
- 多个操作可以叠加（限制取并集）。解除只会移除该操作的限制，有结束时间的操作会在该时间自动结束（列表中显示为 `expired`）。结束时间最多为十年后（`expires_in` 最大 315360000）；更长的会以 `422` 拒绝。
- 解除操作后，被 `disable_login`、`suspend` 和 `ban` 撤销的会话和令牌不会恢复。用户需要重新登录。
- 本站账号的 `suspend` 或 `ban` 会向关注者所在的服务器和已知服务器发送个人资料被清空的 Actor `Update`，以表明封禁（Mastodon 会显示为“suspended”）。
  解除和到期会发送包含原始个人资料的 `Update`。到期由每 5 分钟运行一次的任务发送，所以最多可能晚 5 分钟。
- `delete_statuses` 和 `mark_statuses_sensitive` 通过 `-status`（`status_ids`）接受目标账号 1 到 100 条未删除的帖子。它们不能设置结束时间。
  解除 `delete_statuses` 不会恢复已删除的帖子。解除 `mark_statuses_sensitive` 会恢复原来的显示。
- `ban` 不能设置结束时间，也无法解除。只有用户在删除前 30 天内提出的申诉被推翻时才会取消（[申诉](#appeals)）。拿不准时，请使用带结束时间的 `suspend`。
  操作 30 天后，每小时运行一次的任务会删除账号（账号和帖子从视图中消失，服务器向其他服务器发送 `Delete`）。
- `report_id`（`-report`）只能指定关于目标账号的举报。待处理的举报会随操作一起以 `actioned` 处理完毕（处理结果也会记入审计日志）。已处理的举报不会改变。
- `public_reason` 是向用户显示的理由，`private_note` 是仅供审核员查看的备注。`notify_target`（默认为 true；在 CLI 中用 `-no-notify` 设为 false）决定是否通知用户。

<!-- source: what-the-user-sees 0ca06c59f8e1 -->
### 用户看到的内容

- `notify_target` 为 true 的操作会以通知（`moderation_notice`）的形式送达用户。通知包含类型、理由类别、`public_reason`、结束时间和状态。类型和类别还会以用户界面语言的显示名称返回。
- 用户可以用第一方的 `GET /api/v1/moderation/actions`（用 `active` 筛选）按从新到旧的顺序查看针对自己的操作。`notify_target` 为 false 的操作以及已解除或已到期的操作也会列出。
- `private_note`、举报、执行操作的人以及解除的理由永远不会向用户显示。
- 用户可以对每个操作申诉一次（[申诉](#appeals)）。操作列表和通知中的操作包含能否申诉（`appealable`）、截止日期（`appeal_deadline`）以及申诉和其结果（`appeal`）。
- 处于 `disable_login`、`suspend` 或 `ban` 下的用户无法使用应用或 API，但仍可以在登录页面（`/auth/sign_in`）登录。他们在那里可以看到生效中的操作的类型、理由和结束时间
  （对于 `ban`，还有距离账号被删除的天数），并可以在该页面申诉（[申诉](#appeals)）。授权端点（`/oauth/authorize`）返回 `403`，不签发令牌。
  处于 `ban` 下的用户在账号被删除前都可以登录，删除也会撤销该会话。已安排删除自己账号的用户以及已被停用的账号，即使在登录页面也无法登录。

<!-- source: accounts-that-their-owners-deactivated ed010f49b40a -->
### 由所有者停用的账号

用户可以在账号设置中停用自己的账号，需要再次输入密码或双重验证码，或用通行密钥确认（三者都没有的用户需要在已关联的
登录提供方重新登录）。这不是审核操作，审核员也不能停用任何人的账号：

- 除审核员外，所有人都看不到个人资料和帖子；其他服务器会收到标记为 `suspended` 的 actor `Update`，因此可能把该账号显示为已封禁。用户的所有会话和令牌都会结束。
  如果服务器会发送邮件，会有一封邮件说明删除日期；在此之前不会再发送其他邮件。
- 任何东西都不会送达该账号：提及和私信会被丢弃，不会产生通知、邮件或 Web Push，新的关注者会被拒绝。喜欢和转发会计数。已有的关注保持不变。
- 用户登录后会看到一个服务器页面，页面显示删除日期，并允许重新激活或删除账号。重新激活会恢复一切。下一次停用需要等待 24 小时。
- 停用 30 天后，每小时运行一次的任务会删除账号。过了这个日期的重新激活会被拒绝，即使任务尚未运行。用户名不会被释放。

审核员在账号列表中会看到 `deactivated` 状态（`state=deactivated`），并且仍可以查看关于该账号的举报并对其采取操作。审核操作不会改变删除日期，
但 `ban` 例外，它会用自己的日期替换：此后用户无法重新激活。账号停用期间，封禁不会向其他服务器发送任何内容，用户重新激活后封禁仍然有效。
以下情况下用户不能停用账号：审核使其无法登录时、已安排删除时、已迁移后、注册正在等待审核时、付费订阅仍在进行时（包括设为当期结束时终止的订阅），
或者它是唯一能够登录的管理员时。已停用的管理员不算作能够登录的管理员。如果某个已停用管理员的日期到了，而没有其他管理员能够登录，
每小时运行的任务不会删除该账号，并记录一条警告。那位管理员也已无法登录，所以请用 `my admin create` 创建另一个管理员；之后的下一次运行就会删除该账号。

<!-- source: appeals 8a6f1b4c288d -->
## 申诉

本站用户可以对针对自己的操作提出申诉。
审核员审查申诉，维持（`uphold`）或推翻（`overturn`）原操作。

```sh
my admin appeal list -state pending
my admin appeal uphold -reason "The posts were spam." <appeal id>
my admin appeal overturn -reason "Sorry, our mistake." <appeal id>
```

用户通过 `POST /api/v1/moderation/actions/{id}/appeal`（scope 为 `write:accounts`，请求体为 `{"text": "..."}`）申诉。
处于 `disable_login`、`suspend` 或 `ban` 下的用户无法获取访问令牌，需通过登录页面上每个操作对应的表单申诉（`POST /auth/appeal`）。
当[登录仅限登录提供方](login-providers.md#limit-sign-in-to-login-providers)时，这类用户只能通过已关联的提供方到达该表单。
对于可以申诉的操作，页面显示表单和截止日期；对于已申诉的操作，显示申诉内容和结果；对于已过截止日期或无法申诉的操作，显示原因。申诉被驳回时会显示理由。
管理 API 中：`GET /api/v1/admin/appeals`（用 `state` 筛选）、`GET .../{id}`，以及 `POST .../{id}/uphold` 和 `POST .../{id}/overturn`（请求体 `{"public_reason": "..."}` 可选）。它们需要 `manage_accounts` 权限。

- 每个操作只能申诉一次，文本为 1 到 2,000 个字符。截止日期为操作后实例设置 `appeal_window_days` 指定的天数（默认 180 天，范围 20 到 365；见[实例设置](instance-settings.md)）。
  对已过截止日期的操作申诉以及第二次申诉会返回 `422`（`/api/v1/problems/appeal-window-closed` 和 `/api/v1/problems/already-appealed`）。
- `ban` 可以在账号被删除之前申诉（操作后 30 天，如果 `appeal_window_days` 的截止日期更早则以其为准）。对已删除账号的 `ban` 申诉会返回 `422`（`/api/v1/problems/not-appealable`）。已解除或已到期的操作在截止日期内仍可申诉。
- 用户申诉时，拥有 `manage_accounts` 的人会收到通知（`admin_appeal`）。审查完成后，用户会收到通知（`appeal_decided`），并能看到结果和 `public_reason`。审查者不会显示。
- 推翻会在同一事务中解除操作（效果与 `lift` 相同，解除理由为空）。推翻 `delete_statuses` 不会恢复已删除的帖子。审查前已被解除的操作保持原样。
- 推翻 `ban` 还会在同一事务中取消已安排的账号删除（审计日志记录 `account_deletion_canceled`），结束封禁，并向其他服务器发送包含原始个人资料的 `Update`。被撤销的会话和令牌不会恢复，所以用户需要重新登录。
  如果账号在审查前已被删除，则无法推翻（`422`，`/api/v1/problems/not-appealable`；仍可以维持）。
- 审查只进行一次。以相同结果再次审查不会改变任何东西，以相反结果审查会返回 `409`（`/api/v1/problems/appeal-decided`）。

<!-- source: account-management 99149899a198 -->
## 账号管理

搜索账号、查看其详情以及显示 IP 地址和邮箱地址都通过管理 API 完成，需要 `manage_accounts` 权限。

在 Web 客户端中，一个账号有三个页面，页面标题下方有它们之间的链接：

- **审核**（`/moderation/accounts/{id}`）：账号的状态和限制、新操作的表单、针对它的最新操作及其理由，以及它最新的 20 条举报。更早的举报只在举报列表（`/moderation/reports`）中，该列表无法按账号筛选。
- **操作**（`/moderation/accounts/{id}/actions`）：针对该账号的所有操作，分页显示，生效中的操作带有“解除”。每个操作都会打开自己的页面（`/moderation/accounts/{id}/actions/{action id}`），显示其全部详情。
- **管理**（`/moderation/accounts/{id}/manage`，仅限本站账号）：账号的角色（`manage_roles`）、邮件发送抑制、移除双重验证和签发密码重置链接（`manage_login_recovery`），以及邮箱和 IP 地址。每个区块只对拥有相应权限的人显示。

工作人员只能对级别更低的人采取行动：发起或解除操作、裁决申诉、解除邮件发送抑制、显示私密数据、签发密码重置链接以及移除双重验证，都要求你的角色中最高的 `position` 高于目标每个角色的级别。
对级别更高或相同的人（包括你自己的账号）执行这些操作会返回 `403`，问题类型为 `/api/v1/problems/target-outranks-actor`，所以审核员不能封禁管理员，管理员之间也不能互相封禁。`my admin` 命令没有这一限制。
使唯一能够登录的管理员无法登录的操作（`disable_login`、`suspend`、`ban`）无论由谁发起都会被拒绝，该管理员也不能删除自己的账号（`409`，`/api/v1/problems/last-administrator`）。角色见 [admin-cli.md](admin-cli.md)。

- `GET /api/v1/admin/accounts` 按从新到旧的顺序返回本站和远程账号（不包括实例 Actor）。
  可以按用户名前缀（`username`）、域名（`domain`）、本站或远程（`origin`）、状态（`state`）筛选；对本站账号，还可以按邮箱地址（`email`）、邮箱域名（`email_domain`）以及注册或登录时使用的 IP 地址（`ip`）筛选。
  `email`、`email_domain` 和 `ip` 只能指定其中一个。列表不包含 IP 地址和邮箱地址。
- 状态取以下列表中第一个适用的：`deleted`、`suspended`（`suspend`、`ban`）、`disabled`（`disable_login`）、`rejected` 和 `pending`（注册审核）、`limited`（`limit`）、`active`。
  状态和限制（`restrictions`）只来自针对该账号本身的操作，不包括域名操作。
- `GET /api/v1/admin/accounts/{id}` 返回状态和限制；对本站账号，还返回注册申请和审核、邮箱确认、角色，以及按从新到旧排列的最多 20 条操作和举报。
- `POST /api/v1/admin/accounts/{id}/reveal` 返回本站账号的邮箱地址、注册和登录时的 IP 地址与 User-Agent（20 条，从新到旧），以及使用过相同 IP 地址的其他账号（每个 IP 地址最多 20 个）。
  每次调用都会在审计日志中记录 `revealed_private_data`，只有成功写入记录时才会返回数据。IP 地址会在保留期（默认 180 天）后删除，所以无法显示更早的登录来源。
- `GET /api/v1/admin/accounts/{id}` 中的 `two_factor_enabled` 表示本站账号的用户是否使用双重验证（TOTP）。
  同时丢失了身份验证器应用和恢复码的用户，在你用 `POST /api/v1/admin/accounts/{id}/two_factor/disable` 移除双重验证后，就可以只用密码登录。
  只有拥有 `manage_login_recovery` 权限的用户才能执行此操作，默认只有内置的管理员角色拥有该权限（它属于服务器管理；Web 客户端只向这些人显示该区块）。
  移除会在审计日志中记录 `two_factor_disabled`。移除之前，请通过邮件等其他途径确认对方是账号的所有者。
  如果没有其他管理员的管理员丢失了它，运营者可以用 CLI 命令 `my admin account disable-two-factor @alice` 移除（审计日志中的执行者为 CLI）。
- 在会发送邮件的服务器上，忘记密码的用户可以在登录页面通过“忘记密码？”收到一封邮件，其中包含设置新密码的链接。
  对于不发送邮件的服务器（`mail = none`），或者邮件无法送达的用户，由管理员签发链接并交给对方。
  在管理界面中，通过账号“管理”页面（`/moderation/accounts/{id}/manage`，`POST /api/v1/admin/accounts/{id}/password_reset_link`）上的“重置密码”签发。
  只有拥有与移除双重验证相同权限 `manage_login_recovery` 的用户才能签发，默认只有内置的管理员角色拥有该权限（持有链接的人就能进入账号，所以审核员的 `manage_accounts` 不够）。
  签发的链接只在当场显示一次，可以复制。服务器只保存链接的哈希值，所以之后无法再查看；需要时请重新签发。
  当没有可用的管理员或管理界面不可用时，运营者可以用 CLI 命令 `my admin account password-reset-link @alice` 签发（审计日志中的执行者为 CLI）。
  当[登录仅限登录提供方](login-providers.md#limit-sign-in-to-login-providers)时，两种方式仍会签发链接，并提示在该限制结束前链接无效。
  链接有效期为 1 小时且只能使用一次，如果之后通过其他途径更改了密码，链接也会失效。两种签发方式都会在审计日志中记录 `password_reset_link_issued`。
  任何持有链接的人都可以设置密码，所以请通过邮件等其他途径确认对方身份，并只把链接交给本人。
  设置新密码会撤销该用户的所有会话和应用令牌。通行密钥和双重验证保持不变。
- `GET /api/v1/admin/accounts/{id}` 中的 `email_suppression` 表示已停止向本站账号的主邮箱地址发送邮件。
  `hard_bounce`（收件地址不存在）会停止除确认邮件外的所有邮件，`complaint`（被举报为垃圾邮件）只停止通知和公告邮件。密码重置邮件也会停止，所以需要时请签发上述链接并交给对方。
  当用户请求确认邮件并再次确认地址后，硬退信造成的抑制会解除。
  当你确认可以正常发送（例如用户修复了邮箱）后，用 `POST /api/v1/admin/accounts/{id}/email_suppression/lift`（权限为 `manage_accounts`）解除。这会在审计日志中记录 `email_suppression_lifted`。
- 一次搜索为了填满一页最多读取 400 个账号。匹配的账号较少时，即使页面很短甚至为空，也可能带有 `next_cursor`，所以请继续读取。

<!-- source: domain-actions 13dd3d4d3a0f -->
## 域名操作

| 类型 | 效果 |
| --- | --- |
| `suspend` | 停止所有投递、获取和接收（接收在验证签名之前就返回 `403`）。已有的账号和帖子会从时间线和个人资料中隐藏。双向切断本站用户与该域名账号之间的关注（不向对方发送任何内容） |
| `limit` | 使帖子不出现在公共时间线和搜索中，并且不为未关注该账号的用户创建通知 |
| `reject_media` | 不获取媒体 |
| `reject_reports` | 丢弃来自该域名的举报（`Flag`） |
| `force_sensitive` | 将媒体显示为敏感内容 |

```sh
my admin domain issue -kind suspend -public-comment "spam" spam.example
my admin domain issue -kind limit -duration 720h noisy.example
my admin domain list -active
my admin domain lift <action id>
```

管理 API 中：`POST /api/v1/admin/domain_actions`（请求体包含 `domain`、`kind`、`public_comment`、`private_note`、`obfuscate` 和以秒为单位的 `expires_in`）、
`GET /api/v1/admin/domain_actions`（用 `domain` 和 `active` 筛选）、`GET .../{id}`，以及 `POST .../{id}/lift`。

在 Web 客户端中，打开“审核”→“域名操作”（`/moderation/domain-actions`）。列表显示每个操作的域名、类型、状态和结束时间，可以只看生效中的或查看全部；
“新操作”会打开表单（`/moderation/domain-actions/new`），选择某个域名会打开该操作的页面，其中显示备注和日期，可以用“解除”结束它。
远程服务器页面上的“域名操作”链接只列出该域名的操作，在那里点击“新操作”会打开已填好域名的表单。

- 域名会规范化为 IDNA A-label 后记录（小写，无末尾的点）。`例え.jp` 和 `xn--r8jz45g.jp` 是同一个域名。
- 操作也适用于子域名（对 `spam.example` 的操作同样适用于 `a.spam.example`）。
- 操作在记录它的同一事务中应用到限制投影（`domain_restrictions`）和时间线限制，从下一次投递、获取或接收起生效。
- 有结束时间（`-duration` / `expires_in`，最长十年）的操作会在该时间自动结束。到期不算“解除”，所以列表中显示为 `expired`。
- 解除只会移除该操作的限制。同一域名上的其他操作保持不变。解除已解除的操作不会改变任何东西。
- 列表按从新到旧排列，`-active`（`active=true`）只显示生效中的操作。

<!-- source: importing-and-exporting-mastodon-csv ba9ed39bd32e -->
### 导入和导出 Mastodon CSV

你可以使用 Mastodon 域名屏蔽的 CSV 格式（`#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate`）与 Mastodon 互相传递域名操作列表。
你可以导入 Mastodon 导出的 CSV，Mastodon 也可以导入本服务器导出的 CSV。

```sh
my admin domain export > domain_blocks.csv
my admin domain import domain_blocks.csv                       # only shows what it would do
my admin domain import -apply -only spam.example,bad.example domain_blocks.csv
```

管理 API 中：`GET /api/v1/admin/domain_actions/export`（`text/csv`）、`POST .../import/preview`（请求体包含 `csv`），以及 `POST .../import`（请求体包含 `blocks`，即选中的行，以及 `private_note`）。
本服务器自身域名的行会被跳过（`own_domain`），对自身域名的域名操作会被拒绝。
在 Web 客户端中，打开“审核”→“域名操作”并选择“Mastodon 的 CSV”（`/moderation/domain-actions/import`）：“导出 CSV”会保存文件，导入时会先预览粘贴或选择的文件，再导入你保留的行。

| CSV 行 | 本服务器上的操作 |
| --- | --- |
| `#severity` 为 `suspend`（或为空） | `suspend` |
| `#severity` 为 `silence` | `limit` |
| `#severity` 为 `noop` | 无 |
| `#reject_media` / `#reject_reports` 为 true | 分别追加 `reject_media` / `reject_reports` |

- 导入分两步。先在预览中确认每一行会发起哪些操作，选择要保留的行，然后导入（在加上 `-apply` 之前，CLI 不会做任何更改；`-only` 按域名选择行）。
- 如果某行的域名或其上级域名已有同类型的生效中操作，该行不会发起该类型（如果所有类型都已存在，则为“已应用”）。同一域名的第二行、无效的行以及不会发起任何操作的行（只有 `noop`）也会被跳过。再次导入同一个 CSV 不会增加操作。
- 没有 `#domain` 表头的 CSV 会被当作每行一个域名的列表读取，每个域名都会被施加 `suspend`（与 Mastodon 相同）。一次导入最多 5,000 行。
- 导入的操作持续到“被解除为止”，并带有该行的公开备注和模糊化设置，以及你填写的私密备注。它们与逐个发起的操作一样出现在审计日志中（通过 API 时执行者为操作的人，通过 CLI 时为 `cli`）。
- 导出时，每个域名对应一行，包含生效中的操作。有 `suspend` 的域名导出为 `suspend`，有 `limit` 的导出为 `silence`，两者都没有的导出为 `noop`。只有 `force_sensitive`（Mastodon 的域名屏蔽没有此项）的域名不会导出。私密备注和结束时间不会导出。

<!-- source: severed-follows 51be435d8635 -->
### 被切断的关注

域名的 `suspend` 和远程账号的 `suspend` 会双向切断本站用户与被封禁账号之间的关注。

- 对于域名的 `suspend`，记录操作后运行的任务（`moderation.sever_domain`）会逐个本站用户地切断关注。在任务完成之前，这些关注仍会显示在列表中。子域名上的账号也会被切断。
- 对于远程账号的 `suspend`，关注在记录操作的同一事务中被切断。对已封禁的账号再次 `suspend` 不会切断任何东西。
- 每个失去已批准关注的用户都会收到一条通知（`relationships_severed`），告知失去了多少关注者和关注。只失去关注请求的用户不会收到通知。
- 解除操作不会恢复被切断的关注。用户需要重新关注。
- 再次处理同一个操作（例如任务重新运行时）不会再增加关系切断记录或通知。

<!-- source: publishing-the-domains-you-act-on b647d2c8c84c -->
### 公开你采取操作的域名

你可以通过 `GET /api/v1/instance/domain_actions` 公开生效中的 `limit` 和 `suspend` 操作。
实例设置 `domain_actions_visibility`（`off` / `signed_in` / `everyone`，默认 `off`）决定谁能看到列表，
`domain_actions_rationale_visibility`（取值相同，默认 `off`）决定谁能看到公开备注（[实例设置](instance-settings.md)）。

```sh
my instance set domain_actions_visibility signed_in
my instance set domain_actions_rationale_visibility signed_in
```

- 设为 `off` 时 API 返回 `404`；设为 `signed_in` 时，对不带访问令牌的请求返回 `401`。
- 每个操作一条记录，按从新到旧排列。每条记录包含域名、域名的 SHA-256（十六进制）和类型，对有权查看的人还包含公开备注。私密备注永远不会返回。
- 对于带 `-obfuscate`（`obfuscate`）的操作，域名中间一半的字符（点除外）会被替换为 `*`（`bad.example` 变为 `bad.*****le`）。
  任何人都可以比对自己已知域名的 SHA-256，检查它是否就是被模糊化的那个域名。
- `reject_media`、`reject_reports` 和 `force_sensitive` 操作，以及已解除或已到期的操作都不会公开。
- Web 客户端的实例信息页面（`https://<host>/about`）按相同的设置显示此列表。设为 `off` 时不显示该部分；设为 `users` 时，未登录的人会看到一条说明，告知只有已登录用户才能查看。

<!-- source: refetching-remote-posts 883740b40c9a -->
## 重新获取远程帖子

你可以从对方服务器重新获取已存储的远程帖子。
只有当你通过管理 API、CLI 或 Web 客户端请求时才会重新获取，从不定期执行。

```sh
my admin refetch status <post ID or URI>
my admin refetch account @bob@remote.example
my admin refetch domain remote.example
```

管理 API 中：`POST /api/v1/admin/statuses/{id}/refetch`、`POST /api/v1/admin/accounts/{id}/refetch_statuses` 和 `POST /api/v1/admin/instances/{domain}/refetch_statuses`。它们需要 `manage_domains` 权限。
在 Web 客户端中，使用账号“审核”页面（`/moderation/accounts/{id}`，远程账号）和远程服务器界面（`/moderation/instances/{domain}`）上的“重新获取帖子”。

- 帖子按收到 `Update` 的方式处理。如果有更新的版本，服务器会添加一个版本；会更新投票的票数；没有变化则什么也不做。对方服务器返回 410 Gone 或 `Tombstone` 的帖子会被删除。无法获取的帖子（包括 404）保持原样。
- 对于账号，服务器会重新获取其最新的最多 1,000 条未删除的已存储帖子，每次 100 条，间隔一分钟。
- 对于域名，服务器从该域名最新的账号开始重新获取，每个账号最多 20 条最新帖子，总共最多 1,000 条，每次 5 个账号，间隔一分钟。
- 不会从带有 `suspend` 的域名获取，也不会从因投递失败而处于 `unavailable` 的域名获取。同一 URI 的获取失败会被记住 1 小时，在此期间的重新获取不会去获取它。
- 你的请求会在审计日志中记录为 `status_refetch_queued`、`account_statuses_refetch_queued` 或 `domain_statuses_refetch_queued`。

<!-- source: reports e8a3f112b323 -->
## 举报

从其他服务器收到的举报（`Flag`）以待处理（`open`）状态进入举报列表。

```sh
my admin report list -state open
my admin report show <report id>
my admin report assign <report id> <username>
my admin report unassign <report id>
my admin report resolve -outcome dismissed <report id>
my admin report note <report id> "Same person as an earlier report"
my admin report reopen <report id>
my admin report forward -comment "Spam account, see the linked posts" <report id>
```

管理 API 中：`GET /api/v1/admin/reports`（用 `state`、`category` 和 `assignee` 筛选，负责人可以是 `me`、`none` 或本站账号的 ID；`order=oldest` 会把等待最久的举报排在最前，而不是最新的）、`GET .../{id}`、`POST .../{id}/assign_to_self`、`POST .../{id}/unassign`、
`POST .../{id}/resolve`（请求体 `{"outcome": "actioned" | "dismissed"}`）、`POST .../{id}/notes`（请求体 `{"body": "..."}`）、`POST .../{id}/reopen` 和 `POST .../{id}/forward`（请求体 `{"comment": "..."}`，可选）。

- 分配（`assign`）只能分配给角色拥有 `manage_reports` 的本站用户。在管理 API 中只能分配给你自己。
- 处理（`resolve`）的结果为 `actioned`（已对目标采取操作）或 `dismissed`（无需采取操作）。再次处理已处理的举报不会改变结果。
- 当你从举报发起操作（操作的 `report_id` / `-report`）时，举报会在同一次操作中以 `actioned` 处理完毕。
- 备注（`note`）是只有审核员能看到的内部备注，最多 2,000 个字符。它既不会显示给目标，也不会显示给举报者。备注无法编辑或删除。
- 重新打开（`reopen`）会把已处理的举报放回 `open`（分配保持不变）。待处理的举报不能重新打开（管理 API 中返回 `409`）。
- 转发（`forward`，举报详情中的按钮 **转发到对方服务器**）会把关于其他服务器上账号的举报发送给该服务器，形式是由你的实例 actor 发出的 `Flag`，其中包含该账号以及仍然存在的被举报帖子。它携带的唯一文本，是你在转发时写给对方审核员的内容（`-comment`、管理 API 中的 `comment`，或举报页面上的 **给对方审核员的文本**；最多 1,000 个字符，默认没有）。举报者的说明和[外部审查](#reports-that-an-external-review-filed)的备注永远不会通过这种方式离开你的服务器，举报者的任何信息也不会被发送。举报者在提交时选择转发的举报已经立即发出，并带有举报者的说明。转发适用于在本服务器上提交的、尚未转发的举报，无论是由人还是由[外部审查](#reports-that-an-external-review-filed)提交，也无论处于待处理还是已处理状态；从其他服务器收到的举报和关于语音聊天房间的举报不能转发（管理 API 中返回 `409` `report-not-forwardable`，举报详情中不显示按钮）。一条举报只发送一次：再次转发（无论文本如何），或者转发举报者在提交时已转发过的举报，都不会改变任何东西。管理 API 中的 `forwarded` 表示是否已发送，`forwardable` 表示现在能否发送。
- 举报详情（`show`、`GET .../{id}`）按从旧到新的顺序返回备注，以及关于目标的其他举报数量和针对目标的操作数量（包括已解除和已到期的）。列表不返回备注。
- 分配、处理、重新打开、备注和转发都以事件形式保存（`report_assignments`、`report_resolutions`、`report_reopenings`、`report_notes`、`report_forwards`），`report_states` 保存当前状态。
- 审核员可以在举报页面上翻译被举报的帖子和举报者的说明，也可以使用 `POST .../{id}/statuses/{status_id}/translation` 和 `POST .../{id}/comment/translation`（请求体 `{"target_language": "en"}`）。
  只有当[实例设置](instance-settings.md) `translation_reports_to_server` 开启时，文本才会发送给翻译服务商，而且只会发送给获准处理该帖子可见范围的服务商：审核员的访问权限不会放宽任何限制，所以私信帖子只会发送给获准处理 `direct` 的服务商，说明则按 `private` 计算。
  说明的翻译不会保存，已删除帖子的翻译也不会保存，字符数只计入服务器的每月限额。当 `translation_browser` 开启时，浏览器内置的翻译可用于举报，不受 `translation_reports_to_server` 的影响。
- 举报者能看到的目标内容，与他们平时能看到的该账号内容相同：当该账号被封禁或屏蔽了举报者时，举报仍会列出，但不显示该账号。审核员总能看到完整的目标。
- 本站举报者会在第一次处理时收到通知（`report_resolved`）。重新打开后再次处理不会增加通知。
  举报者可以用 `GET /api/v1/reports` 列出自己的举报，只能看到每条是待处理还是已处理。结果、采取的操作、给目标的理由以及负责人都不会显示。

<!-- source: reports-that-an-external-review-filed 8563536b46e4 -->
### 外部审查提交的举报

回答 `flag` 的[决策钩子](decision-hooks.md)会以你的实例 actor 作为举报者提交举报。举报会注明这一点：
管理 API 和 `report.created` webhook 中的 `hook_origin`、举报详情中的一行说明，以及 `my admin report show` 中的 `automated`。
它的说明是插件的备注，类别来自理由。提交时不会转发，处理时也不通知任何人。当目标在其他服务器上时，你可以自己转发
（`forward`）：插件的备注留在你的服务器上，只有你写的文本会发出。取消该决策不会改变举报。

<!-- source: voice-chat-rooms 7fd142752b07 -->
### 语音聊天房间

- 本站用户可以举报自己能看到的、正在进行的语音聊天房间（`POST /api/v1/reports`，带 `voice_room_id`，并将 `account_id` 设为房间的主持人）。
  举报的目标是当时的主持人，并指向当时的房间设置（名称和描述）。它不指向任何帖子，也永远不会转发到其他服务器。
- 举报详情（`GET /api/v1/admin/reports/{id}`）会显示房间（`voice_room`）：其设置的每次更改、谁参与过以及何时离开、谁被移出，以及房间如何结束。
  参与者的记录在房间结束后保留 90 天（`voice_participants_retention_days`），并且只要有举报指向该房间就会一直保留（[记录保留](#record-retention-and-holds)）。
  没有录音，也没有文字聊天，所以这些记录是唯一的证据。
- 拥有 `manage_reports` 的人可以用 `POST /api/v1/admin/voice_rooms/{id}/end`（请求体 `{"reason": "..."}`，可选，最多 500 个字符）为所有参与者结束正在进行的房间。
  理由只供审核员查看；参与者只能看到房间已结束。语音聊天关闭时也能使用，审计日志会记录 `voice_room_ended`。

<!-- source: notifications-for-moderators-and-the-pending-counts 7b83fa4097b2 -->
## 审核员的通知和待处理数量

新的举报、等待批准的注册和申诉会以应用内通知的形式送达拥有相应权限的人。

| 通知 | 何时送达 | 谁会收到 |
| --- | --- | --- |
| `admin_report` | 当举报的目标没有其他待处理举报时（关于同一账号的第二条及之后的举报不会通知） | 角色拥有 `manage_reports` 的人，举报者和举报目标除外 |
| `admin_sign_up` | 当注册以等待批准状态开始时（审批模式，或在无法发送邮件的环境中为 `open`） | 角色拥有 `manage_accounts` 的人 |
| `admin_appeal` | 当有人对操作提出申诉时 | 角色拥有 `manage_accounts` 的人，申诉者本人除外 |
| `admin_custom_emoji_request` | 当有人请求创建自定义表情或替换其图片时 | 角色拥有 `manage_custom_emojis` 的人，请求者除外 |
| `admin_error_group` | 当服务器或 Web 客户端报告了新类型的错误，或者已解决的错误组在另一个服务器版本上再次出现时 | 角色拥有 `manage_errors` 的人 |

- `admin_error_group` 对整个服务器每小时最多送达 10 次。该小时内其余的会合并为一条通知，
  说明还有多少个错误组。已静音的错误组从不通知。Web 客户端为未登录访客报告的错误，
  只有在某个已登录用户的浏览器也报告了它之后才会通知，其再次出现不会通知任何人。
  错误组通过 `GET /api/v1/admin/error_groups` 读取，并通过
  `POST /api/v1/admin/error_groups/{id}/resolve`（`/mute`、`/unmute`）解决、静音或取消静音，这些都需要 `manage_errors`。
- 谁会收到通知，取决于创建通知时其拥有的权限。之后失去权限的人，在其列表和未读数中将不再看到该类通知。如果权限恢复，又会重新看到。
- 上述每类通知也会通过邮件发送，默认开启。每个人可以在设置的 **通知** 页面（Web 客户端中为 `/settings/notifications`）
  分别开关每类通知的应用内通知和邮件，该页面只列出其角色会收到的类型。在应用内关闭的类型不会为其创建，所以既不会推送也不会发送邮件；
  `GET /api/v1/admin/counts` 的数量不会改变，因为它统计的是举报、注册和申诉本身。
  除了待处理举报的数量，还会返回最早的待处理举报的提交时间（`oldest_open_report_filed_at`）。
  服务器在记录通知的同一事务中将邮件任务加入队列，任务在提交后立即发送邮件，
  不会等待通知被阅读。如果在发送时此人已失去权限或已关闭该类邮件，
  或者其主邮箱地址未经确认或邮件已被停止发送，则什么也不会发送。
  邮件只包含通知的类型和指向通知界面的链接，不包含举报或账号的任何内容。邮件中的退订链接只关闭该类邮件。
  当 `mail = none` 时，不发送任何邮件，`GET /api/v1/instance` 中的 `sends_email` 为 `false`。

`GET /api/v1/admin/counts` 返回待处理举报数（`open_reports`，需要 `manage_reports`）、等待批准的注册数（`pending_registrations`）和等待审核的申诉数（`pending_appeals`）（两者都需要 `manage_accounts`）。
你没有权限的项目不会返回。

<!-- source: audit-log 86955f4182c0 -->
## 审计日志

审核员和管理员的操作会在与操作相同的事务中，以每次一行的形式记录到审计日志（`audit_log_entries`）中。
失败的操作和没有改变任何东西的操作不会记录：解除已解除的操作、处理已处理的举报、分配给相同的负责人、停用已停用的规则、移动到相同的位置、授予用户已有的角色，以及撤销用户没有的角色。

| 目标类型 | 操作 |
| --- | --- |
| `account` | `account_action_issued` 和 `account_action_lifted`（发起和解除针对账号的操作）、`account_deletion_canceled`（因针对 `ban` 的申诉被推翻而取消删除）、`revealed_private_data`（显示了 IP 地址和邮箱地址）、`two_factor_disabled`（移除了双重验证）、`password_reset_link_issued`（签发了密码重置链接）、`email_suppression_lifted`（解除了邮件发送抑制）、`account_moved`（受限的本站账号迁移到了本站账号；限制不会带到目标账号，由你决定是否在那里施加）、`account_statuses_refetch_queued`（重新获取远程账号的帖子） |
| `status` | `status_refetch_queued`（重新获取远程帖子；目标名称为帖子 URI） |
| `domain` | `domain_action_issued` 和 `domain_action_lifted`（发起和解除域名操作）、`domain_statuses_refetch_queued`（重新获取该域名账号的帖子）、`instance_marked_available`（清除某个服务器的失败记录） |
| `report` | `report_assigned`、`report_unassigned`、`report_resolved`、`report_reopened`、`report_note_added`、`report_forwarded`（分配、取消分配、处理和重新打开举报，添加备注，以及转发举报） |
| `user` | `registration_approved`、`registration_rejected`（注册审核） |
| `setting` | `setting_changed`（实例设置）、`text_changed`（实例文本） |
| `rule` | `rule_added`、`rule_revised`、`rule_moved`、`rule_retired`（实例规则） |
| `role` | `role_granted`、`role_revoked`（为用户授予和撤销角色）、`role_created`、`role_updated`、`role_deleted`（创建、更改和删除角色） |
| `appeal` | `appeal_upheld`、`appeal_overturned`（申诉审查；推翻时还会为该操作记录 `account_action_lifted`，对 `ban` 还会记录 `account_deletion_canceled`） |
| `signup_block` | `signup_block_added`、`signup_block_lifted`（添加和解除注册屏蔽） |
| `hook_decision` | `hook_decision_applied`（服务器根据外部审查的结论采取了行动并留下了状态；执行者为系统）、`hook_decision_overridden`（审核员取消了该决策，包括通过推翻其申诉）、`hook_appeal_upheld`（审核员维持了对被拒帖子的申诉，决策保持不变） |
| `custom_emoji` | `custom_emoji_created`、`custom_emoji_image_replaced`、`custom_emoji_copied`、`custom_emoji_updated`、`custom_emoji_enabled`、`custom_emoji_disabled`、`custom_emoji_deleted`（注册自定义表情、替换其图片、从其他服务器复制表情，以及更改、启用、停用和删除表情） |
| `webhook` | `webhook_created`、`webhook_updated`、`webhook_deleted`、`webhook_enabled`、`webhook_disabled`、`webhook_secret_rotated`（创建、更改、删除、启用和停用 webhook，以及重新生成其密钥） |
| `announcement` | `announcement_created`、`announcement_updated`、`announcement_published`、`announcement_withdrawn`（创建、更改、发布和撤回公告） |
| `invite` | `invite_created`、`invite_revoked`（创建和撤销邀请；目标名称为邀请码的前 4 个字符） |
| `billing_source` | `billing_import_queued`（手动开始了账单导入；目标名称为服务商名称） |
| `cost_item` | `cost_item_created`、`cost_item_relabeled`、`cost_item_price_revised`、`cost_item_end_changed`、`cost_item_deleted`（登记费用、更改其名称等、修改其金额、更改其结束日期，以及删除费用；目标名称为费用项名称） |
| `supporter_program` | `supporter_program_enabled`、`supporter_program_disabled`（启用和停用收费；目标名称为支付服务商名称） |
| `supporter_plan` | `supporter_plan_created`、`supporter_plan_revised`、`supporter_plan_retired`（创建、更改和停用支持者方案；目标名称为方案名称） |
| `supporter_subscription` | `supporter_subscription_ended`（管理员立即结束了某个订阅；目标名称为方案名称和用户的用户名） |
| `custom_emoji_request` | `custom_emoji_request_submitted`、`custom_emoji_request_withdrawn`、`custom_emoji_request_approved`、`custom_emoji_request_rejected`（提交、撤回、批准和拒绝自定义表情请求；目标名称为 `:shortcode:`） |
| `payment_provider` | `payment_reconcile_queued`（手动开始了与支付服务商的对账；目标名称为服务商名称） |
| `oauth_client` | `oauth_client_disabled`、`oauth_client_enabled`（停用 OAuth 客户端和撤销停用；停用会撤销其所有令牌；目标名称为客户端名称和 `client_id`） |
| `oauth_client_host` | `oauth_client_host_denied`、`oauth_client_host_allowed`（拒绝 Client ID Metadata Document 主机和撤销拒绝；拒绝会撤销该主机上客户端的令牌；目标名称为主机） |
| `instance` | `service_closure_announced`、`service_closure_withdrawn`、`service_closure_closed`、`service_closure_delivery_completed`（公告关闭服务或更改日期、撤回公告、关闭，以及完成关闭的投递；目标名称为 `service_closure`） |
| `voice_room` | `voice_room_ended`（审核员结束了正在进行的语音聊天房间；目标名称为房间 ID，因为名称会在写下它的账号被抹除时消失；举报详情会显示名称） |
| `link_card_domain` | `link_card_domain_fetch_stopped`、`link_card_domain_images_stopped`、`link_card_domain_rule_removed`（为某个域名设置链接卡片爬虫的规则，以及移除规则；见[链接卡片域名](instance-settings.md#link-card-domains)） |
| `login_provider` | `login_provider_created`、`login_provider_updated`、`login_provider_deleted`、`login_provider_enabled`、`login_provider_disabled`、`login_provider_secret_rotated`（添加、更改、删除、启用和停用外部登录提供方，以及替换其客户端密钥；目标名称为提供方的显示名称） |

列表会显示目标是否仍然存在。在保留期后消失的账号、举报、申诉和注册屏蔽，以及已删除的角色、自定义表情和 webhook，只显示记录时的名称，没有指向界面的链接。

```sh
my admin audit list
my admin audit list -actor alice -action domain_action_issued
my admin audit list -target-kind domain -target-id spam.example
```

管理 API 中：`GET /api/v1/admin/audit_log`（用 `actor_account_id`、`action`、`target_kind` 和 `target_id` 筛选）。它需要 `view_audit` 权限。

- 每一行包含执行者（`user` 时为用户，CLI 时为 `cli`）、操作、目标类型和 ID、目标的显示名称、原始记录的 ID 以及时间。
  目标 ID 是账号、举报、申诉、用户、规则、角色或注册屏蔽的 ID；对于域名操作为域名；对于设置和文本为键名。
- 目标的显示名称（`@alice`、`@bob@example.com`、域名、设置键名、规则的文本；对于文本为其键名，对于角色为其键名和用户的用户名，对于注册屏蔽为其类型和值）是记录时复制的，所以目标消失后仍然可读。
  API 的每一行还会返回目标是否仍然存在（`target_exists`），对于已消失的账号、举报、申诉和注册屏蔽为 `false`。对于已消失的目标，Web 审计日志只显示显示名称，不带链接。
- 理由和备注的文本不会复制。请通过原始记录（操作、分配、处理、重新打开、备注、审查，或设置或规则的版本）的 ID 查找。
- 列表按从新到旧排列，使用键集分页（CLI 中为 `-before <entry id>`）。
- 没有用于更新或删除审计日志的 API 或 CLI。旧的行会在[保留期](#record-retention-and-holds)后删除。

<!-- source: record-retention-and-holds bd806b3bbda7 -->
## 记录保留与冻结

每类记录的保留期过后，每小时运行的任务（`moderation.purge_records`、`identity.purge_origins` 和 `voice.purge_records`）会删除审核记录。
你可以在实例设置中更改保留期，但不能低于下限（[实例设置](instance-settings.md)）。更改后的保留期从下一次运行起生效。已解除的保留冻结及其理由，会在解除后经过审核操作的保留期时删除。

| 记录 | 设置 | 默认值 | 下限 | 起算点 |
| --- | --- | --- | --- | --- |
| 登录 IP 地址和 User-Agent | `session_origins_retention_days` | 180 天 | 90 天 | 登录时 |
| 注册 IP 地址 | `registration_origins_retention_days` | 180 天 | 90 天 | 注册时 |
| 举报（说明、目标帖子、备注、分配、处理、重新打开） | `reports_retention_days` | 365 天 | 180 天 | 最后一次处理时 |
| 针对账号的操作（理由、备注、目标帖子、解除）和申诉 | `moderation_actions_retention_days` | 365 天 | 180 天 | 操作结束时 |
| 审计日志 | `audit_log_retention_days` | 730 天 | 365 天 | 记录时 |
| 注册屏蔽 | 无（无法更改） | 30 天 | 不适用 | 解除和到期中较早者 |
| 谁参与过语音聊天房间（参与记录、发言者和听众、举手、邀请、移出） | `voice_participants_retention_days` | 90 天 | 30 天 | 房间结束时 |
| [决策钩子](decision-hooks.md#the-decisions)的决策 | `hook_decisions_retention_days` | 90 天 | 30 天 | 决策时 |

- 待处理的举报永远不会删除。被操作引用的举报（操作的 `report_id`）会一直保留到该操作被删除。
- 操作的结束是解除和到期中较早者。对于被申诉的操作，如果审查晚于此时间，则以审查时间为结束，申诉等待审查期间操作会被保留。
  `warn` 和 `delete_statuses` 没有持续效果，所以以发起时间为结束。没有结束的操作（`ban`、没有结束时间的 `suspend` 等）在被解除之前不会删除。
- 被举报或操作指向的帖子，即使作者已删除（从视图中消失），在该举报或操作被删除之前，清理任务也不会物理删除这些帖子及其附带的媒体。远程帖子也会超出保留期保留（包括实例设置 `remote_status_max_retention_days`）。
- 既没有解除也没有结束时间的注册屏蔽，在生效期间会一直保留。
- 被举报指向的语音聊天房间的参与者记录会保留到该举报被删除。房间本身（名称、描述、主持人以及结束方式）永远不会删除。
- 一次任务运行对每类记录只删除有上限的行数（操作、举报和注册屏蔽每次 100 行，共 20 次；审计日志每次 1,000 行，共 20 次）。剩余的由下一次运行删除。

<!-- source: holds f3bf57c2c258 -->
### 冻结

当法院命令禁止抹除数据（日本《信息流通平台对策法》第 16 条），或正在处理信息披露请求时，你可以按账号停止抹除。

```sh
my admin retention hold -reason "court order 2026-123" @alice
my admin retention list -account @alice
my admin retention release <hold id>
```

- 对于处于冻结中的账号，其操作和申诉、关于它的举报和它提交的举报、这些内容的审计日志、登录和注册 IP 地址、它的帖子，以及它参与过或在其中采取过行动的语音聊天房间的记录，都不会在保留期后删除。
  由该账号的封禁创建的注册屏蔽，在解除或到期 30 天后也会继续保留。
- 一个账号可以有多个冻结。全部解除后，已超过保留期的记录会在下一次运行时删除。解除已解除的冻结不会改变任何东西。
- 冻结及其解除只能通过 CLI 进行，记录中的执行者为 CLI。
- 已删除账号（`ban` 到期的账号、用户自己删除的账号，或其服务器删除的远程账号）的数据，会在删除 7 天后由每小时运行的任务 `account.erase_deleted` 抹除。远程账号的帖子也会被抹除，包括本站用户加入书签或回复过的帖子。
  仅因获取时返回 `410 Gone`、而没有收到它自己签名的 `Delete` 就被标记为已删除的远程账号，会先再获取一次。第二次返回 `410` 时将其抹除。如果其服务器再次提供它，它会连同帖子一起恢复，不会抹除任何东西。其他任何响应（`404` 或无响应）会等待之后的运行，删除 30 天后无论如何都会抹除该账号。
  处于冻结中的账号不会被抹除。被举报或操作指向的帖子会保留到该记录被删除。账号的 URI、用户名和操作记录会保留。
  该账号为其语音聊天房间设置的名称和描述会被清空；房间本身保留。

<!-- source: exporting-for-disclosure-requests 89f1074dd950 -->
### 为信息披露请求导出

对于信息披露请求（GDPR 第 15 条、日本《个人信息保护法》第 33 条），请将账号的所有审核记录导出为 JSON 来回应。

```sh
my admin account export-moderation @alice > alice-moderation.json
```

- 导出内容包括针对该账号的操作（包括私密备注、解除、申诉和审查）、关于该账号的举报（说明、目标帖子、引用的规则、审核员的备注、分配、处理、重新打开）、该账号提交的举报（只有每条是待处理还是已处理），以及这些内容的审计日志。
- 举报者的身份（举报的账号，以及收到的举报的 URI 和来源服务器）不会导出。在交出导出文件之前，请检查举报的说明中没有能识别举报者的内容。
- 审核员以用户 ID 导出。请先设置冻结，以免在处理请求期间记录被删除。
