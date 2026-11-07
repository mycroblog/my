<!-- source: privacy-policy-template 9966bbc6844c -->
# 隐私政策模板

一份供你根据自己的服务器修改的隐私政策：它说明这台服务器保存什么、为什么保存、保存多久，以及谁会收到这些数据。

**这是一份模板，不是法律建议。** 它描述的是软件在默认设置下的行为。它无法知道你如何运行自己的实例，也不判断哪些法律适用于你。一份政策必须写明什么，取决于你所在的司法辖区以及你的用户所在的司法辖区（例如欧盟的 GDPR、英国的 UK GDPR，或日本的《个人信息保护法》）。发布前请针对你的司法辖区进行审查，并把下文中的法律依据和跨境传输声明当作需要确定的问题，而不是现成的答案。

与之配套的处理活动记录见[处理活动记录模板](processing-record-template.md)。

<!-- source: how-to-use-it 9cb843cc1ba2 -->
## 如何使用

1. 把[模板](#the-template)复制到文件 `privacy-policy.md` 中。
2. 替换每一处 `{{...}}`。花括号中的值（例如 `{{180 days}}`）是软件的默认值：请对照你的实例确认它（[期限及其设置](#the-periods-and-their-settings)），然后去掉花括号。
3. 以 `{{IF ...}}` 开头的行或句子只适用于相应的配置（[你选择的服务](#the-services-you-choose)）。不适用时删除它；适用时去掉这个标记。
4. 确认没有遗漏：`grep -n '{{' privacy-policy.md` 不输出任何行。
5. 使用 `my instance set privacy_policy - < privacy-policy.md` 发布，或在管理页面中“条款和政策”的“隐私政策”页面（`/admin/documents/privacy-policy`）发布。读者可以在 `/about/privacy` 找到它（[实例设置](instance-settings.md#terms-and-privacy-policy)）。

写好隐私政策后，注册界面会增加一个必须勾选的同意复选框。这个复选框表明有人读过该文档。在法律要求征得同意的地方，不要把它当作对数据处理的同意。

这段文本对所有语言只有一个值。如果你的读者使用多种语言，请把需要的各种语言都写进这一段文本中。

当你更改下文中的某项设置或某项服务时，以及每次升级的发布说明提到个人数据时，请重新审查这份政策。

<!-- source: the-periods-and-their-settings 111d5a4b6af8 -->
## 期限及其设置

`my instance list` 会显示每项实例设置，以及它是否为默认值。对期限的更改会在其每小时任务的下一次运行时生效。超出范围的值会被拒绝。

| 记录 | 默认值 | 实例设置 | 范围（天） | 起算点 |
| --- | --- | --- | --- | --- |
| 登录时的 IP 地址和 User-Agent | 180 天 | `session_origins_retention_days` | 90-3650 | 登录 |
| 注册时的 IP 地址 | 180 天 | `registration_origins_retention_days` | 90-3650 | 注册 |
| 举报 | 365 天 | `reports_retention_days` | 180-3650 | 最后一次处理完毕 |
| 针对账号的处置和申诉 | 365 天 | `moderation_actions_retention_days` | 180-3650 | 处置结束 |
| 审计日志 | 730 天 | `audit_log_retention_days` | 365-3650 | 记录 |
| 谁参加了语音聊天房间 | 90 天 | `voice_participants_retention_days` | 30-3650 | 房间结束 |
| 外部审核的决定（使用外部审核时） | 90 天 | `hook_decisions_retention_days` | 30-3650 | 作出决定 |
| 本服务器上没有任何内容引用的远程帖子 | 90 天 | `remote_status_retention_days` | 1-3650 | 帖子发布 |
| 所有远程帖子 | 无限制（`0`） | `remote_status_max_retention_days` | 0-3650 | 帖子发布 |
| 仅由中继带来的远程帖子 | 7 天 | `relay_status_retention_days` | 1-3650 | 帖子发布 |
| 远程媒体的副本 | 1 天；`remote_media_policy` 为 `none` 时不保存 | `remote_media_cache_ttl_seconds`、`remote_media_policy` | 60-31536000 秒 | 获取 |
| 帖子的译文 | 30 天 | `translation_retention_days` | 1-3650 | 翻译。阅读译文不会延长期限，帖子删除时译文也会删除 |

以下是部署的设置（[配置参考](../reference/configuration.md)），而不是实例设置：

| 记录 | 默认值 | 设置 |
| --- | --- | --- |
| 未附加到任何内容的上传文件 | 24 小时 | `media_unattached_retention` |
| 被新版本替换后的旧版本图片 | 30 天 | `media_replaced_retention` |
| 永久失败的任务 | 30 天 | `dead_job_retention` |
| 未成功的任务运行记录 | 30 天 | `job_attempt_retention` |

以下期限由软件固定，任何设置都无法更改：

| 记录 | 期限 |
| --- | --- |
| 清除已删除的账号 | 删除后 7 天；对于远程账号，如果只是其服务器返回了 `410 Gone`，且该服务器之后不再响应，则最长 30 天 |
| 删除已停用的账号 | 停用后 30 天 |
| 注册封锁 | 解除或到期后 30 天；两者都没有时，在生效期间一直保留 |
| 会话、令牌和授权码 | 过期后 30 天 |
| 数据导出的归档和数据导入 | 7 天 |
| 从其他服务器收到的活动的记录 | 30 天，文档本身 7 天 |
| 投递到其他服务器的活动的记录 | 30 天；帖子的投递记录保留到帖子或其作者被删除后 30 天 |
| 远程用户名的检查 | 90 天，保留每个账号最新的一条 |
| Webhook 消息和投递 | 14 天 |
| 支付 webhook 的接收记录 | 45 天 |
| 每小时的错误计数和错误通知 | 90 天 |

[保留冻结](moderation.md#holds)会停止对某个账号的清除，因此对于处于冻结中的账号，上述每个期限都可能更长。如果你使用冻结，请像模板中那样写明这一点。

<!-- source: records-with-no-automatic-end 90a13d1f648e -->
## 没有自动结束期限的记录

服务器只在账号被清除时才删除以下记录，或者从不删除。法律可能要求你说明保留它们的理由，或通过其他方式删除它们，因此请决定你的政策对每一项怎么写。

- 活跃账号的数据行：电子邮件地址、个人资料的旧版本、帖子的旧版本、通知、关注关系，以及被拒绝用户的注册申请。没有人删除的账号会一直保留。
- 清除后仍保留的内容：账号的 URI 和用户名（使两者都不能被重复使用）、账号的生命周期事件、帖子已被删除的记录、在各自期限结束前的审核记录、邀请和角色授予、账号迁移，以及去掉名称和说明的语音聊天房间。
- 支持者记录（订阅、状态变化、付款、退款、拒付），为会计和与支付服务商对账而保留。服务器不会删除支付服务商处的客户：如果你的义务允许，请在服务商的控制台中删除。
- 没有结束时间的处置（`ban`，或没有结束时间的 `suspend`），保留到解除为止。未处理的举报。
- 远程账号，保留到其服务器删除它们为止。
- 每个错误组的最新报告，会被该组的下一份报告替换。报告按模式清洗，因此可能残留用户文本的片段。
- 备份、你的日志，以及你的平台和代理的日志。服务器不管理这些：[备份](backup.md#how-often-and-where)说明了要保留的代数，而恢复备份会把已清除的数据带回来。

<!-- source: the-services-you-choose 81461216c966 -->
## 你选择的服务

每个接收个人数据的服务，只有在你启用了它时才出现在政策中。请根据下表填写第 6 节的表格，并阅读该服务自己的政策和合同：数据到达后该服务如何处理，不是本文档能告诉你的。

| 标记 | 设置 | 服务收到的内容 |
| --- | --- | --- |
| 托管 | `database`、`blob`、`queue`、`key_manager` | 数据库和 blob 存储保存的所有内容，形式取决于服务商的存储方式。云队列中的任务负载包含 ID 和类别，不含地址、消息正文或机密信息。 |
| `{{IF cdn}}` | `media_origin` | 读者的 IP 地址和对媒体的请求（[CDN](cdn.md)）。 |
| 代理 | 你的反向代理或负载均衡器，服务器需要它来提供 TLS | 每个请求及其 IP 地址，记录范围取决于你配置的日志。 |
| `{{IF mail}}` | `mail` 为 `smtp` 或 `ses` | 收件人地址、主题和正文。通知邮件包含与通知相同的文字（[电子邮件](email.md)）。 |
| `{{IF captcha}}` | `captcha` 不为 `none` | 每个打开登录、注册或密码重置请求页面的人的 IP 地址、浏览器和设备信息、Cookie 以及与小组件的交互，直接从其浏览器发送。服务器只向该服务发送令牌，不附加 IP 地址（[CAPTCHA](captcha.md#what-to-write-in-the-privacy-policy)）。 |
| `{{IF login providers}}` | [登录服务商](login-providers.md) | 服务商能看到登录。服务器收到并保存服务商对此人的标识符，并在注册时使用服务商提供的电子邮件地址和用户名。 |
| `{{IF stripe}}` | `payments = stripe` | 预先填入 Checkout 的电子邮件地址、方案和本站的订阅 ID。Stripe 在自己的页面上收集付款信息（[Stripe](payments-stripe.md#card-data-and-pci-dss)）。 |
| `{{IF payjp}}` | `payments = payjp` | 电子邮件地址和方案。浏览器在服务器的付款页面上加载 payjp.js，银行卡信息输入到 PAY.JP 的 iframe 中，姓名和电子邮件地址为 3-D Secure 发送给 PAY.JP（[PAY.JP](payments-payjp.md#card-data-and-pci-dss)）。 |
| `{{IF error reports}}` | `errors_exporter` | 经过清洗、不含账号、IP 地址、正文或请求头的报告。清洗按模式匹配，因此可能残留用户文本的片段（[错误报告](error-reporting.md#what-scrubbing-does-and-does-not-guarantee)）。 |
| `{{IF telemetry}}` | `metrics_exporter`、`traces_exporter` | 不含账号或客户端 IP 地址的指标和 span。出站请求的 span 会写明远程服务器的主机名（[运行](running.md)）。 |
| `{{IF voice media server}}` | `voice` 为 `livekit` 或 `sora` | 参与者的 IP 地址、发言时的音频、会话 ID 和房间 ID。不会发送账号的 ID（[语音聊天](voice.md)）。 |
| `{{IF turn}}` | `voice_stun_urls`、`voice_turn` | STUN 和 TURN 服务器能看到参与者的 IP 地址，TURN 服务器还会中继他们经过加密的音频。 |
| `{{IF webhooks}}` | [服务器 webhook](webhooks.md) | 你的接收端会收到事件正文。其中一些包含申请人的电子邮件地址或举报的说明。 |
| `{{IF follow review}}` | 设置了 `plugin_inbound_follow_address`，且 `hook_inbound_follow_mode` 不为 `off` | 来自其他服务器的每个关注所涉及的两个账号的地址，在关注到达时由服务器发送（[决策钩子](decision-hooks.md)）。记录的决定会保留该活动的地址以及发送它的账号。 |
| `{{IF sign-up review}}` | 设置了 `plugin_signup_check_address`，且 `hook_signup_check_mode` 不为 `off` | 每个未使用邀请的申请人的电子邮件地址和 IP 地址，在其注册时由服务器发送（[决策钩子](decision-hooks.md#sign-ups)）。记录的决定两者都不保存。除非你自己运行该插件，否则请在第 6 节中写明该服务。 |
| `{{IF post review}}` | 设置了 `plugin_local_status_address`，且 `hook_local_status_mode` 不为 `off` | 注册时间短于 `hook_local_status_account_age_days`（默认 7 天；`0` 表示所有账号）的账号所发的每条 `public`、`local` 和 `unlisted` 帖子的文本（输入的原文和渲染后的文本），以及其内容警告、语言、是否标记为敏感、投票选项、图片描述、话题标签、提及了多少个账号、它将获得的地址、它回复和引用的帖子的地址、作者的地址和用户名，以及账号的创建时间，在发帖时以及每次编辑时由服务器发送（[决策钩子](decision-hooks.md#local-posts)）。`private` 和 `direct` 帖子绝不会发送。记录的决定不保存任何文本。对被拒绝的帖子提出申诉的作者会提交其文本，服务器会将其与决定一起保存。除非你自己运行该插件，否则请在第 6 节中写明该服务。 |
| `{{IF profile review}}` | 设置了 `plugin_inbound_actor_address`，且 `hook_inbound_actor_mode` 不为 `off` | 其他服务器上账号的公开个人资料，在每次变更时发送：昵称、简介、个人资料字段、头像和横幅图片的地址（不含图片）、账号的地址、服务器和用户名，以及该账号在本服务器上是否为新账号（[决策钩子](decision-hooks.md#profiles-from-other-servers)）。不会发送你服务器上账号的任何内容。记录的决定会保留账号的地址和个人资料的摘要。 |
| `{{IF profile review}}` | 设置了 `plugin_local_profile_address`，且 `hook_local_profile_mode` 不为 `off` | 昵称、简介（输入的原文和渲染后的文本）、个人资料字段、头像或横幅是否更改、账号的地址和用户名，以及账号的创建时间，在注册时间短于 `hook_local_profile_account_age_days`（默认 7 天；`0` 表示所有账号）的账号每次更改这些内容时由服务器发送（[决策钩子](decision-hooks.md#local-profiles)）。不会发送任何图片。记录的决定不保存其中任何内容。除非你自己运行该插件，否则请在第 6 节中写明该服务。 |
| `{{IF external review of posts}}` | 设置了 `plugin_inbound_status_address`，且 `hook_inbound_status_mode` 不为 `off` | 其他服务器发送的、与本服务器上某个账号相关的帖子及其修订版本，包括仅关注者（`private`）帖子：各语言的文本、内容警告、是否标记为敏感、发布时间、地址和 URL、所回复帖子的地址、投票选项、话题标签、附件的地址和描述（不含媒体）、作者及作者的服务器、可见范围、提及了多少个账号以及其中多少个在本服务器上，以及本服务器上是否有人关注该作者（[决策钩子](decision-hooks.md#what-is-sent-and-what-it-costs)）。私信帖子只有在本服务器上有人关注其作者时才会发送；启用 `hook_inbound_status_unfollowed_direct` 时，任何作者的私信帖子都会发送。 |
| `{{IF media review}}` | 设置了 `plugin_media_check_address`，且 `hook_media_check_mode` 不为 `off` | 每个上传文件经过处理后最小的图片，不含元数据，绝不是原件，以及文件的用途、种类、大小和时长（[决策钩子](decision-hooks.md#what-mediacheck-reviews)）。记录的决定会保留文件的 ID 及其上传者。被拒绝的上传完全不会保留：如果你必须保全或举报某个上传，需要由你的插件来完成。除非你自己运行该插件，否则请在第 6 节中写明该服务。 |
| `{{IF translation}}` | `translation_providers` 不为空 | 读者请求翻译时帖子的文本：正文、内容警告、投票选项和图片描述。不会发送帖子的地址、作者、读者、提及和链接。默认情况下，服务商会收到 `public`、`local` 和 `unlisted` 帖子。它像托管服务一样作为你的服务提供方工作，因此 `local` 帖子虽然不会联邦，也会到达它那里：请保留第 5 节中关于 `local` 的句子。为自身用途保留文本的服务商不属于这里所说的服务提供方，政策中必须写明这一点。 |
| 使用 DeepL 免费套餐的 `{{IF translation}}` | `translation_providers` 包含 `deepl`，且密钥以 `:fx` 结尾 | 同上。DeepL 可能会永久保留文本和译文，政策中必须写明这一点（[翻译](translation.md#deepl)）。 |
| 使用 Amazon Translate 的 `{{IF translation}}` | `translation_providers` 包含 `aws` | 同上。除非你的账号受 AWS Organizations 的 AI 服务退出策略约束，否则 Amazon 可能会存储文本并用于改进其服务，且可能在其他区域进行，政策中必须写明这一点（[翻译](translation.md#amazon-translate)）。 |
| `{{IF translation to private and direct}}` | `translation_provider_visibilities` 允许某个服务商翻译 `private` 或 `direct` | 同上，针对这些帖子。只有在你按服务商允许了它时才使用此标记。它会在第 2 节和第 5 节中增加内容，并扩大你对这些帖子的承诺范围，因此请在更改设置前公告。 |
| `{{IF translation of reports}}` | `translation_reports_to_server` | 审核员请求翻译时，举报的说明及其所指的帖子。每个服务商仍然只会收到它被允许的可见范围，举报的说明按 `private` 计算：如果没有服务商被允许翻译 `private`，说明不会发送给任何服务商。说明的译文不会保存。 |
| `{{IF browser translation}}` | `translation_browser` | 不会有任何内容到达本服务器或服务商。浏览器在读者的设备上进行翻译，浏览器厂商如何处理取决于该浏览器。 |

这些服务都由你选择，因此本模板不说明每个服务设立在哪里，也不说明适用什么合同。请自行确认这两点。

有两类内容无需你进行任何设置就会到达第三方：

- **Web Push。** 用户启用它时，其浏览器会选择推送服务，并把推送服务的地址告诉服务器。消息正文是为该浏览器加密的，因此推送服务应该无法读取。但推送服务能看到订阅地址、每条消息的大小和时间，以及你的服务器的联系方式（VAPID subject），并且在浏览器离线期间最多保留消息 48 小时。
- **联邦。** 参见模板的第 5 节。`federation_mode` 可以关闭联邦或限制其方向，中继则由你自己添加（[中继](relays.md)）。

<!-- source: the-template 8b6bd2fc983a -->
## 模板

```markdown
Last updated: {{DATE}}

This policy describes how {{OPERATOR NAME}} ("we") handles personal data on {{INSTANCE NAME}} ({{https://INSTANCE DOMAIN}}).

# 1. Who is responsible

- Controller: {{OPERATOR NAME, POSTAL ADDRESS}}
- Contact for privacy questions: {{EMAIL OR FORM}}
- {{IF you have a representative in the EU or the UK: name and contact}}
- {{IF you have a data protection officer: name and contact}}

# 2. Whom this covers

This policy covers people with an account here, people who read pages here without an account, and people on other servers whose posts and profiles reach this server.

{{IF translation}} If you are on another server, the text of your posts that reach this server can go to the translation services in section 6 when a reader here asks for a translation. Your server cannot tell you when this happens.

{{IF external review of posts}} If you are on another server, your posts that relate to an account here, followers-only posts included, go with their text to the external review in section 6 before this server accepts them. It can stop a post or mark it, or file a report for our moderators; people decide on any action against an account.

# 3. What we keep, why, and for how long

"Until your account is erased" means: deleting your account hides it at once and erases it 7 days later. A deactivated account stays hidden, can be reactivated for 30 days, and is then deleted the same way.

| Data | Why | How long |
| --- | --- | --- |
| Email addresses | Signing in, confirming the address, resetting the password, and the email you choose to get | Until your account is erased |
| Password hash, two-factor secret (encrypted) and recovery codes (hashed), passkeys, the identifier of an account you linked at a login provider | Signing you in | Until your account is erased |
| Language, time zone, default post settings, notification settings, and other preferences | Running the service the way you asked | Until your account is erased |
| Registration application: the reason you gave, the invite you used, and the decision | Deciding who joins | Until your account is erased |
| IP address at registration | Preventing abuse | {{180 days}} from registration |
| IP address and browser name (User-Agent) at each sign-in | Security and preventing abuse | {{180 days}} from the sign-in |
| Sessions and app access (stored as hashes) | Keeping you signed in | 30 days after they expire. A session lasts 30 days |
| IP address, and for password-reset requests the email address, in rate limit counters | Protecting the service | Removed automatically, within twice the window of the limit |
| Profile: display name, bio, extra fields, avatar, header, aliases, and earlier versions of them | Showing your profile, which is public | Until your account is erased |
| Posts, their earlier versions, and attached media | Publishing what you write | Until you delete them, your automatic deletion policy deletes them, or your account is erased. {{IF you set a maximum for keeping posts}} At most {{N days}} after you publish them, unless your account may keep them longer. Uploads attached to nothing go after {{24 hours}} |
| Favourites, reactions, boosts, bookmarks, pins, poll votes, follows, blocks, mutes, lists | Running the features you use | Until you undo them, or your account is erased |
| Notifications and what you have read | Telling you what happened | Until your account is erased or the post they are about is removed |
| Web Push subscription: your browser's push address and keys | Sending you push notifications | Until you turn it off, the push service ends it, or your account is erased |
| Records that an email bounced or was reported as spam | Not sending mail that is unwanted or cannot arrive | Until your account is erased |
| Data export archives and data imports | Giving you your data, and bringing it in | 7 days |
| Reports: the comment, the posts and rules named, and moderators' notes | Moderation | {{365 days}} from the last resolution, or as long as an action refers to them. Open reports stay |
| Actions against accounts, and appeals | Moderation, and handling repeat abuse | {{365 days}} from the end of the action. An action with no end stays until it is lifted |
| Posts that a report or an action points to | Evidence for moderation | Hidden when deleted, and removed with that record |
| Audit log of staff actions, which can name accounts | Accountability of staff | {{730 days}} |
| Sign-up blocks: an email domain, a hashed email address, or an IP range | Preventing repeat abuse | While in effect, and 30 days after the lift or expiry |
| {{IF voice}} Voice chat rooms: name, description, hosts, and how they ended | Showing and moderating rooms | Kept. The name and description you wrote are emptied when your account is erased |
| {{IF voice}} Who took part in a room: participation, speakers, raised hands, invitations, removals | Moderating rooms | {{90 days}} from the end of the room, or as long as a report about the room. We record neither audio nor text chat |
| {{IF payments}} Supporter plans: the provider's IDs for you and your subscription, amounts, periods, and states. Not your card details | Running your subscription, and accounting | {{PERIOD AND REASON, e.g. as long as tax law requires}}. The link to the provider's customer goes when your account is erased |
| {{IF payments}} Your choice to appear in the list of supporters | Showing the list | Until the subscription ends or your account is erased |
| Copies of activities sent to and received from other servers | Delivering, retrying, and rejecting duplicates | 30 days. The first delivery of a post stays until 30 days after the post or its author is deleted |
| Posts and profiles of people on other servers | Showing them here | Posts: {{90 days}} when nothing here refers to them, {{7 days}} when only a relay brought them, and {{no limit}} otherwise. Profiles: until their server deletes the account. Then erased after 7 days |
| Copies of remote media | Showing it | {{Not kept, or the period you set}} |
| Server logs: request paths, which can contain a username or a post ID. Our server software writes no IP address in them | Running and debugging the service | {{PERIOD}} |
| {{IF proxy logs}} Logs of our proxy or load balancer: IP addresses and requests | Running and protecting the service | {{PERIOD}} |
| Error reports, with personal details removed by pattern | Fixing faults | The latest report of each kind of error stays until a newer one replaces it. Hourly counts: 90 days |
| {{IF webhooks}} Messages to our webhook receivers, which can carry an applicant's email address or a report's comment | Connecting our own tools | 14 days here. {{PERIOD AT THE RECEIVER}} |
| {{IF post review}} The text of a refused post that you send with an appeal, and the decision | Letting staff review the appeal | {{90 days}} from the refusal, and longer while you can still appeal or staff have not decided. Removed when your account is erased |
| {{IF translation}} Translations of posts that readers asked for, which every reader who may see the post can reuse | Showing a translation again without sending the post again | {{30 days}} from the translation, and no longer than the post |
| {{IF translation}} How many characters your requests sent for translation in a month | Keeping translation within its limits | Until the end of the following month, or until your account is erased |
| Backups of all the above | Restoring the service after a failure | {{PERIOD}}. Data you erased stays in a backup until the backup expires |

We can stop erasure for one account when a court order or a legal request requires it. The records of that account then stay until the hold is released.

After erasure we keep the account's address and username so that nobody can reuse them, the record that its posts were deleted, its moderation records until their period ends, invites and roles it took part in, moves it made, and {{IF payments}} the supporter records above.

# 4. Why we are allowed to

{{CHOOSE WITH YOUR ADVISER. A common starting point under GDPR Article 6(1), not a conclusion:}}

- Running your account and the features you use: {{performance of a contract, point (b)}}.
- Security, preventing abuse, moderation, the audit log, logs, and backups: {{legitimate interests, point (f): keeping the service safe and working}}.
- Storing and showing what people on other servers publish: {{legitimate interests, point (f)}}.
- Records the law makes us keep: {{legal obligation, point (c)}}.
- {{IF you rely on consent for anything: what, and how to withdraw it}}.

You need an email address {{OR a login provider}} to create an account here, and without one we cannot provide an account.

# 5. What other people and other servers receive

**This server is part of a federated network.** Other servers run by other people, in other countries, receive your data as part of how the service works, and their own policies apply to it. We cannot make them delete it.

| What | Who receives it |
| --- | --- |
| Your profile, including your avatar, header, and aliases | Anyone, on the web and through other servers |
| `public` and `unlisted` posts | Anyone with the link. The servers of your followers and of the people you mention. {{IF relays}} `public` posts also go to relays we joined, and through them to other servers |
| `local` posts | People signed in to this server, and your followers on other servers. {{IF translation}} When a reader asks for a translation, the text also goes to the translation services in section 6, which work for us. They do not receive the post's address or your handle |
| `private` posts | Your followers and the people you mention, on any server |
| `direct` posts | The people you mention, on any server. They are not end-to-end encrypted: we can technically read them |
| {{IF translation to private and direct}} {{`private` AND/OR `direct`, AS YOU ALLOWED}} posts | {{TRANSLATION SERVICES YOU ALLOWED THEM}}, when a reader who may see the post asks for a translation. They do not receive the post's address, your handle, the reader, or who is mentioned |
| Follows, blocks, boosts, favourites, reactions, poll votes, quotes | The other person's server. Boosts also go to your followers' servers |
| That your account moved, was deactivated, suspended, or deleted | Your followers' servers and, except for a move, other servers we know |
| A report you chose to forward | The reported account's server receives your comment and the posts named. It does not receive your name, IP address, or email address, so do not put those in the comment |
| {{IF translation of reports}} A report's comment and the posts it names | {{TRANSLATION SERVICES YOU ALLOWED `private` POSTS}} receive the comment, and the services in section 6 receive the posts as for a reader, when a moderator asks for a translation. They do not receive who reported, the report, or the posts' addresses |

When you edit or delete a post or delete your account, we tell those servers. Most honour it, but we cannot guarantee it, and copies, including ones made by others, can remain.

Other people here see what you publish. Staff with the right permission can reveal your email address and your registration and sign-in IP addresses, and each reveal is recorded in the audit log. Staff can also see email addresses of pending registrations.

Search engines and AI services: {{DESCRIBE YOUR search_indexing, indexable_default, AND ai_usage SETTINGS. By default we ask them not to index and not to use the content}}. These are requests, and not every crawler follows them. You can change whether your own profile appears in search in your profile settings.

{{IF the full-text search runs on another company's service}} Full-text search: the text of posts in search ({{public / local}} posts of people who chose to appear in search) goes to {{SERVICE}} to make them searchable, and is removed there when the post is deleted or you stop appearing in search. {{SAY HOW LONG ITS BACKUPS KEEP IT}}. {{IF you keep logs that hold search terms}} What you search for can appear in our request logs, which we keep for {{PERIOD}}.

Apps you authorize can read and do what the permissions you granted allow. Their own policies apply. You can revoke them in your settings.

{{IF the list of supporters is public}} If you choose to appear in the list of supporters, it shows your avatar, display name, handle, and plan.

{{IF browser translation}} A reader can also translate a post with the translation built into their browser. That sends nothing to us or to a translation service, and the translation is not stored.

# 6. Service providers

We use these services to run the instance. {{SAY WHETHER EACH ACTS ON YOUR BEHALF AND WHAT CONTRACT APPLIES.}}

| Service | Purpose | Data | Location | Their policy |
| --- | --- | --- | --- | --- |
| {{HOSTING PROVIDER}} | Server, database, and file storage | Everything in section 3 | {{COUNTRY}} | {{LINK}} |
| {{IF cdn}} {{CDN PROVIDER}} | Delivering media | IP address and requests for media | {{COUNTRY}} | {{LINK}} |
| {{IF mail}} {{MAIL PROVIDER}} | Sending email | Your address, and the subject and text of each email | {{COUNTRY}} | {{LINK}} |
| {{IF captcha}} {{CAPTCHA SERVICE}} | Stopping automated sign-ins, sign-ups, and password-reset requests | IP address, browser and device information, cookies, and interactions with the widget. Your browser sends these directly, from the sign-in, sign-up, and password-reset request pages only | {{COUNTRY}} | {{LINK}} |
| {{IF login providers}} {{PROVIDER}} | Signing in with an account there | That you sign in. We receive their identifier for you, and at sign-up your email address and username | {{COUNTRY}} | {{LINK}} |
| {{IF follow review}} {{FOLLOW REVIEW SERVICE}} | Screening follows from other servers for abuse | The addresses of your account and of the account on the other server, when someone there follows you | {{COUNTRY}} | {{LINK}} |
| {{IF sign-up review}} {{SIGN-UP REVIEW SERVICE}} | Screening sign-ups for abuse | The email address and IP address you sign up with, unless you sign up with an invite | {{COUNTRY}} | {{LINK}} |
| {{IF post review}} {{POST REVIEW SERVICE}} | Screening new posts and their edits for abuse | Your `public`, `local`, and `unlisted` posts, each time you post or edit one, while your account is younger than {{7 days}}: their text, content warning, language, whether they are marked sensitive, poll options, image descriptions, hashtags, and how many accounts they mention, their address and the addresses of the posts they reply to or quote, your address and username, and when your account was created. Not your media | {{COUNTRY}} | {{LINK}} |
| {{IF profile review}} {{PROFILE REVIEW SERVICE}} | Screening the profiles of accounts on other servers for abuse | Nothing about your account on this server. The public profile of an account on another server when this server takes in a change of it: its display name, bio, profile fields, the addresses of its avatar and header image, its address and username | {{COUNTRY}} | {{LINK}} |
| {{IF profile review}} {{PROFILE REVIEW SERVICE}} | Screening changes of profiles for abuse | Your display name, bio, and profile fields, and whether you changed your avatar or header, each time you change them while your account is younger than {{7 days}}, with your address and username and when your account was created. Not your images | {{COUNTRY}} | {{LINK}} |
| {{IF media review}} {{MEDIA REVIEW SERVICE}} | Screening uploads for abuse before anyone can see them | A small processed copy of each image you upload, or of the poster or cover of a video or audio file, without its metadata, and what the file is for. Not the original, its description, or your account | {{COUNTRY}} | {{LINK}} |
| {{IF translation}} {{TRANSLATION SERVICE}} | Translating a post when a reader asks | The text of the post: its body, content warning, poll options, and image descriptions. Not its address, its author, the reader, mentions, or links. Text sent to the service cannot be recalled, and deleting the post does not remove it there: {{HOW LONG THE SERVICE KEEPS IT}} | {{COUNTRY}} | {{LINK}} |
| {{IF external review of posts}} {{REVIEW SERVICE}} | Reviewing posts from other servers before they are accepted | The post as its server sent it, followers-only posts included: its text and language, content warning, whether it is marked sensitive, when it was published, its address, the address of the post it replies to, poll options, hashtags, its author, its visibility, attachments' addresses and descriptions, how many accounts it names and how many of them are here, and whether anyone here follows its author. Direct posts {{only when someone here follows their author, or from any author if you turned on `hook_inbound_status_unfollowed_direct`}}. No post of this server's accounts | {{COUNTRY}} | {{LINK}} |
| {{IF stripe}} Stripe | Taking payments for supporter plans | Your email address and plan. You enter payment details on Stripe's pages | {{COUNTRY}} | <https://stripe.com/privacy> |
| {{IF payjp}} PAY.JP | Taking payments for supporter plans | Your email address, cardholder name, and plan. You enter card details in PAY.JP's frame, whose script runs on our payment pages | {{COUNTRY}} | {{LINK}} |
| {{IF error reports}} {{ERROR TRACKER}} | Fixing faults | Error reports with personal details removed by pattern | {{COUNTRY}} | {{LINK}} |
| {{IF telemetry}} {{METRICS OR TRACING SERVICE}} | Watching the health of the service | Measurements, and the names of other servers we contact | {{COUNTRY}} | {{LINK}} |
| {{IF voice media server}} {{VOICE SERVICE}} | Carrying voice chat | Your IP address and audio while you speak in a room, and a session ID. Not your account | {{COUNTRY}} | {{LINK}} |
| {{IF turn}} {{STUN OR TURN SERVICE}} | Connecting voice chat | Your IP address, and encrypted audio when relayed | {{COUNTRY}} | {{LINK}} |

{{IF peer-to-peer voice}} In voice chat rooms, your browser connects directly to other participants, so they can see your IP address.

If you turn on push notifications, your browser's maker or its push service delivers them. The content is encrypted for your browser, but the service sees your browser's push address and when messages are sent.

# 7. Transfers abroad

Delivery to other servers (section 5) sends your data to servers abroad, wherever those servers are. We do not choose or know their countries in advance, and we cannot limit delivery by country.

{{YOUR STATEMENT ON THIS AND ON THE SERVICES OF SECTION 6: which transfer rules you consider apply, and the safeguards you rely on, if any. Settle this with your adviser: whether federated delivery is a restricted transfer is not settled, and this template takes no position.}}

# 8. Cookies and storage in your browser

We use no advertising or analytics cookies. Our pages use these strictly to work:

| Name | Purpose | Lasts |
| --- | --- | --- |
| `__Host-my_session` | Keeps you signed in to the sign-in pages | Up to 30 days |
| `__Host-my_rt` | Keeps the app signed in | Up to 30 days |
| `__Host-my_csrf` | Protects forms against forged requests | Until you close your browser |
| `__Host-my_2fa` | Carries a sign-in to the two-factor step | 5 minutes |
| `__Host-my_external` | Carries a sign-in through a login provider | At most 30 minutes |

The app also stores data in your browser only: display settings, recently used emoji and hashtags, the draft of a post (for the tab), and the newest posts of your home timeline for offline reading, without followers-only and direct posts. The drafts and the timeline are removed when you sign out.

{{IF captcha}} The CAPTCHA service sets its own cookies on the sign-in, sign-up, and password-reset request pages.

{{IF payjp}} PAY.JP's script runs on our payment pages and may set its own cookies. {{CHECK AGAINST PAY.JP'S POLICY.}}

# 9. How we protect it

Passwords are stored as Argon2id hashes. Two-factor secrets and the keys that sign your activities are encrypted, and the encryption key is kept outside the database. Session and access tokens are stored as hashes. Traffic uses HTTPS. {{YOUR MEASURES: encryption at rest and of backups, who has access, and how you handle a breach.}}

# 10. Your rights

{{ADJUST TO THE RIGHTS OF YOUR JURISDICTION. Under the GDPR these are access, rectification, erasure, restriction, objection, and portability, and the right to withdraw consent.}}

- **See and take your data:** your settings offer a data export (your profile, your posts, and your moderation records) and CSV files of follows, blocks, mutes, muted domains, and bookmarks. For everything else, including your email and IP addresses, write to us.
- **Correct it:** edit your profile and account settings, or write to us.
- **Delete it:** delete your account in your settings, or deactivate it first. This is what section 3 and section 5 describe.
- **Move:** you can move your followers to an account on another server.
- **Limit it:** choose your notifications in your settings. Every notification email has an unsubscribe link. You can also object to or ask us to restrict a use of your data by writing to us.
- **Reports about you:** we do not reveal who reported you.

Write to {{CONTACT}}. We answer within {{one month, or the period your law sets}}. You can complain to a data protection authority: {{YOUR AUTHORITY, OR "the authority where you live"}}.

We keep some records even when you ask us to delete them, where we need them for moderation or the law requires them (section 3).

# 11. Children

{{YOUR MINIMUM AGE AND HOW YOU APPLY IT. The server does not ask for a birth date or check age.}}

# 12. Automated decisions

We make no decisions about you by automated means that have legal or similarly significant effects, {{CONFIRM FOR YOUR USE. These are automatic: a sign-up that matches a block, or that an external review answers about, is refused or sent for review, an upload that an external review rejects is refused, {{IF post review}} a post that an external review refuses, which you can appeal to our staff, {{IF profile review}} a change of your profile that an external review refuses, which you can change and save again, the CAPTCHA check, and rate limits. Your staff decide the rest.}}

# 13. Changes

We change this policy when what we do changes, and show the new date at the top. {{HOW YOU TELL PEOPLE, for example an announcement.}} If we close the service, we tell you in advance, with a deadline for taking your data. {{HOW YOU TELL PEOPLE, for example by email and an announcement.}}
```
