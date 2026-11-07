<!-- source: instance-settings 4eb467f0dbca -->
# 实例设置

这些设置决定实例如何运营：注册方式、帖子长度、默认语言区域、谁能查看时间线，以及实例的名称、简介、条款、规则、主题和自定义表情。

<!-- source: configuration-and-instance-settings a0b0908cef1e -->
## 配置与实例设置

| | 配置 | 实例设置 |
| --- | --- | --- |
| 内容 | 部署如何构建：角色、适配器、端点、主机名、密钥 | 实例如何运营：注册方式、帖子长度上限、默认语言区域、谁能查看时间线，以及实例的名称、简介和条款 |
| 存放位置 | 环境变量 `MY_*` 和配置文件（[配置参考](../reference/configuration.md)） | 数据库。每次更改都会保存为一个修订版本 |
| 更改 | 进程重启后生效 | 通过 `my instance set`、Web 客户端的管理界面或客户端 API 更改后立即生效（[管理界面与客户端 API](#admin-screens-and-the-client-api)） |
| 未设置时 | 采用 schema 中的默认值（如果该键是必填项，服务器不会启动） | 采用代码中定义的默认值 |

- **没有任何键同时属于两者。** 两者互不覆盖。你不能通过环境变量或配置文件提供实例设置，也不能把配置值存入数据库。
- 每个部署固定不变、并会改变进程结构的值属于配置；在实例运行期间会变化的值属于实例设置。

<!-- source: prerequisites 12391490eea0 -->
## 前提条件

- 已按照[配置参考](../reference/configuration.md)提供配置（`my config check`）。
- 已将 schema 应用到数据库（`my schema apply`；参见[数据库](database.md)）。

<!-- source: keys ae25d36239c2 -->
## 键

<!-- source: settings-one-key-one-value 4f827bf97480 -->
### 设置（一个键对应一个值）

| 键 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `registration_mode` | `open` / `approval` / `invite` / `closed` | `approval` | 注册方式（[注册](registration.md)）。在已公告停止服务期间和服务关闭之后，以及在[个人服务器](#one-person-server)上，实际生效的值为 `closed`。存储的值不会改变，取消公告或关闭 `single_user` 后会恢复。`GET /instance` 和 NodeInfo 显示实际生效的值 |
| `max_status_length` | 整数 1-100000 | `500` | 帖子的最大长度，单位为字符 |
| `default_locale` | BCP 47 语言标签 | `en` | 默认语言区域。它是新用户界面的初始语言区域，也是没有任何语言目录符合用户偏好时的后备语言区域 |
| `local_timeline_visibility` | `off` / `signed_in` / `everyone` | `everyone` | 谁能查看本站时间线（`GET /api/v1/timelines/local`）：无人、已登录用户或所有人。`off` 返回 404，`signed_in` 对未登录的访客返回 401；这两种情况下导航中都会隐藏它。除非值为 `everyone`，否则首页不会显示示例帖子。在[个人服务器](#one-person-server)上它按 `off` 处理 |
| `federated_timeline_visibility` | `off` / `signed_in` / `everyone` | `signed_in` | 谁能查看联邦时间线（`GET /api/v1/timelines/federated`）。各个值的含义与本站时间线相同，在个人服务器上同样按 `off` 处理 |
| `preview_image` | 媒体 ID（留空 = 未设置） | 空 | 没有图片的帖子和个人资料所用的链接预览图（`og:image`）。未设置时使用内置的默认图片（[链接预览](link-previews.md)） |
| `contact_account` | 本站账号的用户名（留空 = 未设置） | 空 | 用于联系运营者的账号。参见[联系方式与缩略图](#contact-and-thumbnail) |
| `contact_email` | 电子邮件地址（留空 = 未设置） | 空 | 用于联系运营者的电子邮件地址。它也是服务器所发邮件的 `Reply-To` 和签名 |
| `thumbnail` | 媒体 ID（留空 = 未设置） | 空 | 代表实例的图片（缩略图） |
| `remote_media_policy` | `none` / `avatars` / `all` | `none` | 是否保留远程媒体的副本。`none`：媒体代理转发每个请求。`avatars`：头像、横幅和自定义表情会在 blob 存储中缓存 `remote_media_cache_ttl_seconds`。`all`：所有远程媒体都会缓存。缓存的总大小受配置 `remote_media_cache_max_bytes` 限制 |
| `remote_media_cache_ttl_seconds` | 整数 60-31536000（秒） | `86400` | 已获取的远程媒体最多可以保留多久。它也是媒体代理响应的 `Cache-Control: max-age`，因此 CDN 和浏览器会缓存这么长时间 |
| `remote_media_cache_max_bytes` | 从 1 起的整数（字节） | `1073741824` | 媒体代理所保留的远程媒体副本的大小上限。每小时的清理会删除过期的副本，然后删除超出此上限的、最久未被访问的副本 |
| `authorized_fetch` | `true` / `false` | `false` | Authorized Fetch（安全模式）。为 `true` 时，对于没有可验证 HTTP 签名的 ActivityPub GET，对象和集合返回 `401`，用户的 actor 返回一个只带有密钥的最简文档。实例 actor 总是返回完整的文档，因此任何服务器都能获取密钥来验证签名 |
| `federation_mode` | `off` / `inbound` / `outbound` / `both` | `both` | 联邦的方向。`off`：不联邦。`inbound`：你可以关注并阅读远程账号，但本站的帖子、个人资料和互动不会发送出去，来自外部的关注会被拒绝（`Reject`）。`outbound`：你接受来自外部的关注并投递本站的帖子，但不接收远程的帖子和互动。`both`：双向收发。更改立即生效，并且需要权限 `manage_settings` |
| `dynamic_client_registration` | `true` / `false` | `true` | 动态客户端注册（RFC 7591）。为 `true` 时，`POST /oauth/register` 无需认证即可接受公开客户端的注册。为 `false` 时，该端点以 `403`（`access_denied`）拒绝所有请求；已注册的客户端继续可用。一个定期任务会按照配置 `dynamic_client_unredeemed_retention` 和 `dynamic_client_idle_retention` 删除未被使用的动态注册客户端。更改立即生效，并且需要 `manage_settings` |
| `domain_actions_visibility` | `off` / `signed_in` / `everyone` | `off` | 谁能查看已采取措施的域名列表（`GET /api/v1/instance/domain_actions`）：无人、已登录用户或所有人（[审核](moderation.md#publishing-the-domains-you-act-on)） |
| `domain_actions_rationale_visibility` | `off` / `signed_in` / `everyone` | `off` | 谁能查看该列表中的公开说明。看不到列表的人永远看不到它们 |
| `appeal_window_days` | 整数 20-365（天） | `180` | 采取措施后，当事人可以在多少天内申诉（[审核](moderation.md#appeals)）。更改它也会改变已采取措施的申诉期限 |
| `session_origins_retention_days` | 整数 90-3650（天） | `180` | 登录后保留该次登录的 IP 地址和 User-Agent 的天数（[审核](moderation.md#record-retention-and-holds)） |
| `registration_origins_retention_days` | 整数 90-3650（天） | `180` | 注册后保留注册 IP 地址的天数 |
| `reports_retention_days` | 整数 180-3650（天） | `365` | 举报在最后一次处理后保留的天数。被某项措施引用的举报与该措施保留同样长的时间 |
| `moderation_actions_retention_days` | 整数 180-3650（天） | `365` | 账号措施和申诉在措施结束后（解除、到期或申诉审查，以最晚者为准）保留的天数 |
| `audit_log_retention_days` | 整数 365-3650（天） | `730` | 审计日志的每一行在记录后保留的天数 |
| `remote_status_retention_days` | 整数 1-3650（天） | `90` | 当本实例上没有任何内容引用某条远程帖子时（本站的回复、转发、喜欢、书签、通知，或包含本站帖子的讨论串），该帖子在发布后保留的天数 |
| `remote_status_max_retention_days` | 整数 0-3650（天） | `0` | 任何远程帖子在发布后最多保留的天数，即使本实例引用了它。本站的回复、转发、喜欢和书签会保留，并将它显示为已删除。`0` 表示不设上限 |
| `relay_status_retention_days` | 整数 1-3650（天） | `7` | 仅由中继带来的远程帖子在本实例上没有任何内容引用、且没有本站账号关注其作者时保留的天数 |
| `oauth_client_rate_limits` | 以逗号分隔的 `client_id=requests` 对 | 空 | 对 OAuth 客户端的客户端 API 请求设置上限，用于压制使服务器过载的客户端：即该客户端的所有用户在 5 分钟内合计可发出的请求数。更改最多需要一分钟才会生效 |
| `search_indexing` | `allow` / `deny` | `deny` | 是否允许搜索引擎索引本实例。即使为 `allow`，也只会索引 `indexable` 账号的个人资料和 `public` 帖子（[搜索引擎与 AI 爬虫](crawlers.md)） |
| `ai_usage` | `allow` / `deny_training` / `deny` | `deny` | 对 AI 服务的声明。`deny_training` 拒绝训练；`deny` 拒绝训练以及 AI 的使用（AI 搜索、助手、智能体） |
| `indexable_default` | `true` / `false` | `false` | 新账号 `indexable` 的默认值。更改它不会改变现有账号 |
| `auto_delete_default_enabled` | `true` / `false` | `false` | 新账号是否一开始就启用自动删除旧帖子。它与其他 `auto_delete_default_*` 键一起构成新账号获得的策略（[自动删除旧帖子](#automatic-deletion-of-old-posts)） |
| `auto_delete_default_min_age_days` | 整数 1-3650（天） | `30` | 新账号的策略在帖子发布多少天后删除它 |
| `auto_delete_default_keep_pinned`、`auto_delete_default_keep_bookmarked`、`auto_delete_default_keep_favourited` | `true` / `false` | `true` | 新账号的策略是否保留作者置顶、加入书签或喜欢的帖子 |
| `auto_delete_default_keep_media`、`auto_delete_default_keep_polls` | `true` / `false` | `false` | 新账号的策略是否保留带有媒体或投票的帖子 |
| `auto_delete_default_keep_public`、`auto_delete_default_keep_local`、`auto_delete_default_keep_unlisted`、`auto_delete_default_keep_private` | `true` / `false` | `false` | 新账号的策略是否保留该可见范围的帖子 |
| `auto_delete_default_keep_direct` | `true` / `false` | `true` | 新账号的策略是否保留私信帖子 |
| `auto_delete_default_keep_min_boosts`、`auto_delete_default_keep_min_favourites` | 整数 0-1000000 | `0` | 新账号的策略会保留被转发或喜欢至少这么多次的帖子。`0` 表示不设阈值 |
| `auto_delete_max_age_days` | 整数 0-3650（天） | `0` | 任何本站帖子在发布后最多保留的天数（[上限](#a-maximum-for-keeping-posts)）。`0` 表示不设上限 |
| `auto_delete_max_allow_pinned` | `true` / `false` | `true` | 在上限之下是否仍可保留置顶的帖子 |
| `auto_delete_max_kept_pins` | 整数 1-5 | `5` | 在上限之下有多少个置顶可以保留帖子，最近置顶的优先。`5` 是一个账号置顶数的上限，会保留所有置顶的帖子 |
| `auto_delete_max_allow_bookmarked`、`auto_delete_max_allow_favourited`、`auto_delete_max_allow_media`、`auto_delete_max_allow_polls` | `true` / `false` | `false` | 在上限之下是否仍可保留作者加入书签或喜欢的帖子，或带有媒体或投票的帖子 |
| `auto_delete_max_allow_visibilities` | `true` / `false` | `false` | 在上限之下是否仍可按可见范围保留帖子 |
| `auto_delete_max_allow_thresholds` | `true` / `false` | `false` | 在上限之下是否仍可按转发数和喜欢数保留帖子 |
| `auto_delete_grace_days` | 整数 0-365（天） | `30` | 新受到上限约束的账号在收到通知后，还能按自己的设置保留多少天。更改它不会移动已通知账号的日期 |
| `search_visibilities` | `public` / `local` / `public,local` | `public` | 配有搜索后端的服务器的全文搜索涵盖哪些帖子。`unlisted`、`private` 和 `direct` 帖子永远不会被涵盖，而且只涵盖启用了 `indexable` 的账号。去掉某个可见范围会把它的帖子从索引中移除。添加某个可见范围后，只能找到它的新帖子，直到你[重建索引](search.md#changing-search_visibilities)。在 `search = none` 时它不起作用 |
| `security_contact` | `https:` 或 `mailto:` URI（留空 = 未设置） | 空 | 报告本服务器安全问题的地方：服务器的配置和运营方式，以及它的账号和数据。设置后，服务器会提供 `/.well-known/security.txt`（RFC 9116）。这是你的联系方式，而不是软件的：如果收到软件本身漏洞的报告，请按照其[安全策略](../../SECURITY.md)所述转交给项目 |
| `require_media_description` | `true` / `false` | `false` | 本站帖子所附的每个媒体文件（图片、视频或音频）是否都需要描述。为 `true` 时，若帖子或编辑中有文件的描述缺失或只有空白，会以 422 拒绝。它在 `GET /api/v1/instance` 中以 `limits.require_media_description` 公开，Web 客户端会将描述字段设为必填。它与用户个人的设置 `require_media_description` 是分开的，后者只在发送前检查 |
| `link_cards` | `true` / `false` | `true` | 是否为帖子中的链接显示卡片。为 `true` 时，服务器会获取链接的页面来生成卡片。为 `false` 时，服务器停止获取，并隐藏已经生成的卡片。如果只想对部分域名停止，参见[链接卡片域名](#link-card-domains) |
| `link_cards_ignore_robots_txt` | `true` / `false` | `false` | 获取链接卡片时是否跳过 `robots.txt`。为 `false` 时，服务器遵守被链接网站的 `robots.txt`，因此禁止 `my-linkcard` 或 `*` 的网站不会得到卡片（[链接预览](link-previews.md#cards-of-links-in-posts)）。`/about/crawler` 会告诉被链接的网站它是什么。为 `true` 时，服务器既不获取也不使用 `robots.txt`，被链接的网站也就无法再用它拒绝这些获取；服务器仍以 `my-linkcard` 自称。只有在缺失的卡片过多时才开启它。开启期间生成的卡片，会在关闭后的下一次重新获取时检查。无论哪种情况，页面的指令和链接卡片域名都会生效 |
| `oauth_metadata_document_clients` | `true` / `false` | `true` | 是否接受 Client ID Metadata Document 客户端。为 `true` 时，对于 `client_id` 是 `https` URL 的授权请求，服务器会获取该 URL 上的元数据并将其视为客户端（无需事先注册）。获取不跟随重定向，最多读取 5 KB，并按照响应的缓存指令最多复用 24 小时。为 `false` 时，服务器停止获取，并拒绝这些客户端的授权请求、令牌交换和刷新 |
| `finance_visibility` | `private` / `totals` / `breakdown` | `private` | 公开多少运营成本（在 `/admin/finance/costs` 记录，在 `/admin/finance/publication` 设置）。`private`：不公开任何内容（`GET /api/v1/instance/cost_totals` 为 404）。`totals`：只公开每个周期和每种货币的合计。`breakdown`：公开合计以及按服务商和类别的明细，在信息页面（`/about` 和 `/about/costs`）和 API 中向所有人显示。`breakdown` 会暴露你在哪个服务商（例如哪家云）上运行。无论哪个级别，项目名称、备注和单笔付款都不公开。公开截至并包括今天（UTC）所在周期的数据 |
| `finance_viewer_item_names` | `true` / `false` | `false` | 拥有权限 `view_finance` 的用户能否在财务界面（`/finance`、`GET /api/v1/finance/cost_totals`）上看到成本项目名称和导入成本的服务名称。无论此设置如何，合计以及按服务商和类别的明细总是显示 |
| `finance_viewer_item_memos` | `true` / `false` | `false` | 拥有 `view_finance` 的用户能否在财务界面上看到成本项目的备注。只要名称或备注的设置之一为 `true`，每个周期就会显示项目行（服务商、类别、该周期的金额，以及已启用的名称和备注）。拥有 `manage_finance` 的人无论如何都能看到全部内容 |
| `finance_viewer_income` | `totals` / `plans` | `totals` | 拥有 `view_finance` 的用户只能看到收入合计（`totals`），还是还能看到每个方案的订阅人数（`plans`）。每个方案的金额和付款人永远不会显示。如果 `income_visibility` 为 `plans`，无论此值如何都会显示公开的人数。收入可以在 Web 客户端的财务界面（`/finance`）和 `GET /api/v1/finance/income_totals` 中查看 |
| `income_visibility` | `private` / `totals` / `plans` | `private` | 公开多少支持者收入（扣除支付服务商手续费后的金额）。它与 `finance_visibility` 分开选择。`private`：不公开任何内容（`GET /api/v1/instance/income_totals` 为 404）。`totals`：只公开每个周期和每种货币的收入合计。`plans`：还向所有人显示每个方案当前的订阅人数。对于订阅人数很少的方案，人数可能让人猜出谁在订阅。只有当此值和 `finance_visibility` 都不是 `private` 时才显示结余（收入减去成本）；否则结余会暴露你隐藏的那一方。无论哪个级别，单笔付款、订阅者、每个方案的金额、总额和手续费都不公开。公开截至并包括今天（UTC）所在周期的数据 |
| `supporter_list_visibility` | `off` / `signed_in` / `everyone` | `off` | 谁能查看支持者列表（`GET /api/v1/instance/supporters` 和 `/about/supporters`）。`off`：无人（API 返回 404，`/about` 中没有链接）。`signed_in`：只有已登录本服务器的人（否则 API 返回 401）。`everyone`：所有人。列表只显示选择了公开列出的有效订阅者的账号和方案；金额、按月或按年计费、日期和人数永远不会显示。无论 `search_indexing` 如何，列表页面和 API 响应总是 `noindex, noarchive` |
| `user_app_registration` | `true` / `false` | `true` | 已登录用户能否通过客户端 API（`POST /api/v1/oauth_apps`）注册自己的 OAuth 应用（公开或机密）。为 `false` 时，注册返回 403。列出、更改和删除已注册的应用、重新签发密钥、为自己签发令牌以及其他用户的授权都继续可用 |
| `voice_room_starters` | `everyone` / `permitted` | `permitted` | 在配置 `voice` 不为 `off` 时，谁能开启语音聊天房间（[配置](../reference/configuration.md#voice)）。`everyone`：每个未被封禁或限制的已登录本站用户。`permitted`：只有拥有权限 `start_voice_rooms` 的人；默认只有内置的 `admin` 角色拥有该权限，支持者方案的角色也可以拥有 |
| `voice_public_summary` | `true` / `false` | `false` | 对于可见范围为 `local` 的语音聊天房间，其页面是否向未登录本服务器的访客和远程用户显示房间的名称、简介、主持人以及是否仍在进行，并在链接预览中显示。参与者永远不会显示。为 `false` 时，这些访客只能看到该房间面向本服务器的本站用户 |
| `public_counts` | `true` / `false` | `true` | 首页（`/`，未登录时）和 `GET /api/v1/instance/overview` 是否显示服务器有多少成员、其用户在最近 7 天中每天发布了多少公开帖子，以及服务器何时启动。只统计本服务器用户的公开帖子；数字每隔几分钟刷新一次。为 `false` 时，API 会省略它们 |
| `featured_accounts` | 最多 20 个本站账号的用户名，以空格或逗号分隔（留空 = 无） | 空 | 关注推荐中最先显示的账号，按此顺序排列，排在本服务器最近发帖的人之前。如果账号的所有者关闭了“推荐我的账号，并允许通过部分名称找到它”，该账号不会被推荐。参见[账号推荐](#suggestions-of-accounts) |
| `login_providers_only` | `true` / `false` | `false` | 在至少启用了一个登录服务时，是否将注册和登录限制为只能通过登录服务：密码和通行密钥不再能登录，没有关联服务商的用户在会话结束后无法再次登录。没有启用任何服务商时它不生效。`my instance set` 会输出有多少活跃用户没有关联到已启用的服务商。参见[将登录限制为登录服务](login-providers.md#limit-sign-in-to-login-providers)。 |
| `invite_creators` | `everyone` / `permitted` | `permitted` | 除了拥有 `manage_invites` 的工作人员之外，谁能创建自己的邀请（[成员邀请](registration.md#member-invites)）。`everyone`：每个未被封禁或限制的已登录本站用户。`permitted`：只有拥有权限 `create_invites` 的人；默认只有内置的 `admin` 角色拥有该权限，支持者方案的角色也可以拥有。在 `approval` 模式下，可用的邀请会立即批准新用户，因此 `everyone` 会让每个用户都能跳过审核 |
| `member_invite_max_active` | 整数 0-100 | `5` | 一个成员同时最多可以持有多少个可用的邀请（未撤销、未过期、未用完）；`0` 表示成员不能创建邀请。工作人员的邀请不计入 |
| `member_invite_max_uses` | 整数 1-1000 | `10` | 成员创建的一个邀请最多可以让多少人注册 |
| `member_invite_max_lifetime_seconds` | 整数 3600-31536000（秒） | `2592000`（30 天） | 成员创建的一个邀请最长有效多久。降低它不会缩短现有的邀请 |
| `translation_providers` | 以逗号分隔的服务商 ID | 空 | 已启用的服务器端翻译服务商，排在前面的先尝试。为空时不会向任何服务商发送内容。本服务器没有对应服务商的 ID 不起作用。内置的[翻译服务商](translation.md)的 ID 为 `libretranslate`、`deepl`、`google` 和 `aws`，[翻译插件](plugins.md#translating-posts)的 ID 为 `plugin` |
| `translation_provider_visibilities` | 以逗号分隔的 `provider=visibility\|visibility` 对 | 空 | 每个服务商可以接收哪些内容。未列出的服务商接收 `public`、`local` 和 `unlisted` 帖子；`provider=` 后面不写任何内容则不接收任何帖子。`private` 和 `direct` 只发送给明确列出它们的服务商，因此允许一个服务商并不意味着允许下一个 |
| `translation_browser` | 布尔值 | `false` | 读者能否使用浏览器内置的翻译功能，它不会向本服务器或服务商发送任何内容。如果同时启用了服务器端服务商，则服务商优先，浏览器翻译服务器拒绝的内容：没有服务商接受的帖子或语言、达到每月上限，或服务商出错 |
| `translation_user_monthly_characters` | 整数 0-1000000000000 | `50000` | 一个用户的请求在一个 UTC 月内最多可以向翻译服务商发送多少字符。为 `0` 时不发送任何内容。没有表示无限制的值：请设置一个较大的数字 |
| `translation_server_monthly_characters` | 整数 0-1000000000000 | `500000` | 整个服务器在一个 UTC 月内最多可以向翻译服务商发送多少字符。为 `0` 时不发送任何内容。没有表示无限制的值：请设置一个较大的数字 |
| `translation_retention_days` | 整数 1-3650（天） | `30` | 翻译在生成后保留的天数。阅读不会延长这一期限，帖子删除时它也随之删除 |
| `translation_reports_to_server` | 布尔值 | `false` | 审核员能否将举报的文本（被举报的帖子和举报人的说明）发送给服务器端服务商。无论由谁发起，每个服务商仍然只接收 `translation_provider_visibilities` 允许它接收的可见范围：被举报的私信帖子只发送给允许 `direct` 的服务商，说明按 `private` 计。说明的翻译和已删除帖子的翻译永远不会保留，字符数只计入 `translation_server_monthly_characters`。为 `false` 时，举报的任何内容都不会到达服务商 |
| `voice_participants_retention_days` | 整数 30-3650（天） | `90` | 语音聊天房间结束后，保留参与记录的天数：参与情况、发言者和听众、举手、邀请和移出。房间本身（名称、简介、主持人以及结束方式）会保留。被举报房间的记录与举报保留同样长的时间，保全措施与其他记录一样适用 |
| `hook_inbound_status_mode`、`hook_inbound_follow_mode`、`hook_signup_check_mode`、`hook_media_check_mode`、`hook_local_status_mode`、`hook_inbound_actor_mode`、`hook_local_profile_mode` | `off`、`observe`、`enforce` | `observe` | 每个[决策钩子](decision-hooks.md#modes)在有插件时作用到什么程度。`off` 只能经由 `observe` 变为 `enforce`。 |
| `hook_inbound_status_on_failure`、`hook_inbound_follow_on_failure`、`hook_media_check_on_failure`、`hook_inbound_actor_on_failure` | `pass`、`retry` | `pass` | 钩子在强制执行时，其插件出错后如何处理（[详情](decision-hooks.md#when-the-plugin-fails)）。 |
| `hook_inbound_status_unfollowed_direct` | 布尔值 | `false` | `inbound.status` 是否也发送没有本站账号关注的作者的私信帖子（[详情](decision-hooks.md#what-inboundstatus-reviews)）。钩子使用期间，信息页面（`/about`）会说明这一点。 |
| `hook_signup_check_on_failure` | `pass`、`require_approval` | `pass` | `signup.check` 在强制执行时，其插件出错后如何处理。 |
| `hook_signup_check_reject` | 布尔值 | `false` | 在强制执行时，`signup.check` 的 `reject` 是否[拒绝注册](decision-hooks.md#sign-ups)。关闭时，该回答只会被记录，与 `observe` 相同。 |
| `hook_local_status_account_age_days` | 整数 0-3650（天） | `7` | `local.status` [审查帖子及其编辑](decision-hooks.md#local-posts)的对象是注册时间短于此天数的账号；`0` 表示审查所有账号。钩子使用期间，信息页面（`/about`）会说明这一点。 |
| `hook_local_profile_account_age_days` | 整数 0-3650（天） | `7` | `local.profile` [审查个人资料更改](decision-hooks.md#local-profiles)的对象是注册时间短于此天数的账号；`0` 表示审查所有账号。钩子使用期间，信息页面（`/about`）会说明这一点。 |
| `hook_decisions_retention_days` | 整数 30-3650（天） | `90` | [钩子的决策](decision-hooks.md#the-decisions)保留的天数。 |
| `single_user` | 布尔值 | `false` | 服务器是否为[个人服务器](#one-person-server)。只有在服务器恰好有一个本站账号时才能开启，随时都可以关闭 |

- 值在存储前会被规范化（`TRUE` 变为 `true`，`0750` 变为 `750`，`ja_jp` 变为 `ja-JP`，媒体 ID 转为小写）。无效的值会被拒绝，不会写入任何内容。
- `preview_image` 和 `thumbnail` 只接受已存在的图片媒体（包括仍在处理中的）。在处理完成之前，以及在处理失败的图片的位置上，`preview_image` 会退回到默认图片。媒体一旦在任何修订版本中被设置过，未关联上传的清理就不会删除它。
- `contact_account` 只接受未被删除的本站用户的用户名（以小写存储）。`contact_email` 只接受 `alice@example.com` 形式的地址。
- `featured_accounts` 只接受未被删除的本站用户的用户名；开头的 `@` 会被去掉，名称转为小写，重复的名称只保留一次，并以单个空格分隔存储。

<!-- source: suggestions-of-accounts b7d362466e33 -->
### 账号推荐

已登录用户会看到关注推荐（注册后空白的首页时间线上，以及通过 `GET /api/v1/accounts/suggestions`）。推荐先按顺序列出 `featured_accounts` 中的账号，然后是本服务器在最近 30 天内发布过公开或本站帖子的账号，最后发帖的排在最前。用户自己的账号、用户已关注、已屏蔽、已隐藏或已将其屏蔽的账号，以及被封禁或停用的账号都会被排除。

新账号会被推荐，并且可以在账号搜索中通过部分名称找到。在个人资料设置中关闭“推荐我的账号，并允许通过部分名称找到它”的用户会同时退出这两者：此后搜索只能通过完整的用户名找到该账号，或者由关注它的人找到。

<!-- source: automatic-deletion-of-old-posts f0cf962c9de0 -->
### 自动删除旧帖子

用户可以让自己的帖子在发布若干天后被自动删除（设置中的“删除旧帖子”）。每个用户的策略都是自己的：是否启用、多少天后删除，以及保留哪些帖子（置顶、加入书签和喜欢的帖子，带有媒体或投票的帖子，所选可见范围的帖子，以及转发或喜欢达到一定次数的帖子）。以这种方式删除帖子与作者自己删除相同：帖子送达过的服务器会收到 `Delete`。只有本站账号的帖子会以这种方式删除，转发永远不会。

`auto_delete_default_*` 键是每个新的本站账号初始拥有的策略，无论账号以何种方式创建（注册表单、外部登录服务、邀请或 `my admin`）。账号在创建时获得该策略，此后由账号自己拥有：之后更改这些键既不会改变已有的账号，也不会改变获得了较早值的账号。设置界面将这些值显示为用户自己的值。这些键未设置时，新账号初始为关闭自动删除、30 天后删除、保留置顶、加入书签和喜欢的帖子以及私信帖子。默认值不会向实例外的人显示。设置了上限时，新账号获得的是按上限截断后的默认值（如下所述），因此一开始就在上限之内。

<!-- source: a-maximum-for-keeping-posts b1d83e01feda -->
#### 帖子保留的上限

当 `auto_delete_max_age_days` 大于 `0` 时，任何本站帖子在发布后保留的时间都不会超过这个天数。对于每个没有权限 `keep_posts_longer` 的本站账号：

- 自动删除总是开启：用户不能关闭它，也不能选择超过上限的天数（更少则可以）。设置界面和 `PUT /api/v1/preferences/auto_delete` 都会拒绝这两种操作。
- 只有 `auto_delete_max_allow_*` 键允许的例外才能保留帖子。默认只有置顶的帖子可以保留，而且是全部置顶；当 `auto_delete_max_kept_pins` 小于 5 时，只有最近的这么多个置顶能保留其帖子。
- 关闭了自动删除或从未设置过的用户同样受上限约束。

开启上限、降低上限或更改例外都不会改写任何人的设置。相反，任务 `content.auto_delete_statuses` 每小时会找出那些自己的设置会保留上限所不允许保留的帖子的账号（因为上限被开启或降低，或者账号失去了 `keep_posts_longer`）。它会通过默认开启的通知和电子邮件告知每个这样的账号一次，并说明上限从何时开始适用：通知后的 `auto_delete_grace_days` 天。在此之前，账号自己的设置继续有效；此后其帖子按上限处理。重新回到上限之内的账号（用户更改了设置、获得了 `keep_posts_longer`，或者上限被取消或提高）不再受约束，之后的更改会再次告知它，并给予新的宽限期。

开启上限后，一小时内会告知所有受其约束的账号。删除会保持节奏：每个账号每小时最多删除 100 条帖子，每条都以 `Delete` 发送到帖子送达过的服务器，因此旧帖子很多的账号需要几小时甚至几天才能清空。用户在此期间更改设置，会停止剩余的删除。

`keep_posts_longer` 是一项成员权限：将它授予某个角色，即可让该角色的持有者按自己的意愿长期保留帖子并关闭自动删除。内置的 `admin` 角色拥有该权限。[支持者方案](supporters.md)可以附带它，因此更长期地保留帖子可以成为方案的权益；订阅结束后，宽限期过后上限会再次适用。

上限是公开的：`GET /api/v1/instance` 在 `auto_delete_limit` 中返回它，关于页面也会显示它，因此人们在注册前就能知道。更改这些键需要 `manage_settings`，并且与其他设置一样记录在审计日志中。在 Web 客户端中，它们位于“服务器设置”的“帖子删除”页面，在保存开启或收紧的上限之前会先进行确认。

<!-- source: texts-one-key-one-value c639fc7dfe23 -->
### 文本（一个键对应一个值）

| 键 | 限制 | 说明 |
| --- | --- | --- |
| `name` | 100 个字符，单行（不允许只有空白） | 实例名称（显示名称）。参见[名称与域名](#name-and-domain) |
| `short_description` | 500 个字符 | 简短介绍 |
| `description` | 20000 个字符 | 简介 |
| `terms` | 100000 个字符 | 服务条款（Markdown） |
| `privacy_policy` | 100000 个字符 | 隐私政策（Markdown） |
| `commerce_disclosure` | 100000 个字符 | 付费方案的销售者信息披露，例如日本法律要求的标示（Markdown；[支持者](supporters.md)） |
| `preview_image_alt` | 1500 个字符 | `preview_image` 图片的替代文本（`og:image:alt`）。未设置时使用“{实例名称}的图片” |
| `thumbnail_alt` | 1500 个字符 | `thumbnail` 图片的替代文本 |

- 每个文本只有一个值。管理员用自己喜欢的任意语言写一次，所有语言区域的读者看到的都是同一个值；其他语言的读者依靠机器翻译等方式阅读。管理字段的标签和说明与其他界面文本一样会翻译成查看者的语言；只有管理员所写的值不会翻译。
- 服务器不知道文本的语言，因此 Web 客户端和服务器渲染的界面在输出文本时使用 `dir="auto"` 和 `lang=""`（HTML 中表示未知语言的值），而不是继承界面的语言。
- 将文本设为空会删除它。
- 除了 CLI，还可以在 Web 客户端的管理界面上更改文本（名称、简短介绍和简介在“服务器设置”的“常规”页面 `/admin/settings`；服务条款在“条款和政策”的第一页 `/admin/documents`，隐私政策在 `/admin/documents/privacy-policy`，销售者信息披露等在 `/admin/documents/commerce-disclosure`），也可以通过客户端 API 更改（`GET /api/v1/admin/instance_texts`、`PUT /api/v1/admin/instance_texts/{key}`）。两者都需要角色权限 `manage_settings`，更改会记录在审计日志中（`text_changed`）。

<!-- source: terms-and-privacy-policy 82e7bc325cf1 -->
### 服务条款与隐私政策

- 用 Markdown 编写 `terms`、`privacy_policy` 和 `commerce_disclosure`（CommonMark，加上 GitHub Flavored Markdown 的表格、删除线和自动链接）。段落内的换行会保留为换行，`#` 标题从页面标题的下一级（`h2`）开始。
- 服务器将它们转换为 HTML 并进行清理（匿名的 `GET /api/v1/instance/documents/{kind}`），Web 客户端的页面 `/about/terms`、`/about/privacy` 和 `/about/commerce` 会显示它们。原始 HTML、图片以及 `http`、`https` 和 `mailto` 以外的链接会被移除。这些页面无需登录（它们是 Web 客户端的页面，因此需要 JavaScript）。`/about/commerce` 是要提供给支付服务商的 URL。
- 你编写的文件会在注册界面、服务器渲染的每个界面的页脚以及 Web 客户端（第三栏、菜单、设置和登录界面）中提供链接。没有编写的文件不会提供链接，其 API 返回 404。
- 只要编写了服务条款或隐私政策，注册界面就会在创建按钮之前列出它们，并附带一个必选的复选框“我已阅读并同意上述文件”。服务器会拒绝没有勾选它的注册，包括通过登录服务的注册。两份文件都没有时，界面不要求同意。

<!-- source: name-and-domain 1e14aa3e6c58 -->
### 名称与域名

实例的域名和名称是分开的。

- **域名**是配置 `host`（以及 `account_domain`，即用户名中的域名）。实例运行期间不能更改。即使设置了名称，用户名（`@alice@example.com`）、URL、ActivityPub ID 以及 `GET /api/v1/instance` 中的 `domain` 仍然是域名。
- **名称**是文本 `name`。凡是向人们称呼实例的地方都会出现它：Web 客户端的侧边栏和“关于本服务器”，浏览器标签页标题，服务器渲染的登录、注册、电子邮件确认、账号状态和授权同意界面，电子邮件（电子邮件地址确认和审核员通知），链接预览的 `og:site_name`，Web App Manifest 的 `name` / `short_name`，NodeInfo 的 `metadata.nodeName`，以及 `GET /api/v1/instance` 中的 `name`。
- 名称在所有语言区域中都相同。没有名称时，会改为显示域名（`host`）。
- 更改从下一个请求开始生效，无需重启。在管理界面保存后，已打开的 Web 客户端的标签页标题和侧边栏会随之更新。

<!-- source: contact-and-thumbnail 71513ab82d51 -->
### 联系方式与缩略图

运营者的联系方式（`contact_account`、`contact_email`）和代表实例的图片（`thumbnail` 以及文本 `thumbnail_alt`）会出现在以下位置。未设置的项目不会出现。

- Web 客户端的“关于本服务器”（`/about`）。对于未登录的人，缩略图和联系账号也会出现在首个界面（`/`）上。
- `GET /api/v1/instance` 中的 `contact`（`account` 和 `email`）和 `thumbnail`（`url`、`width`、`height`、`alt`）。
- NodeInfo 中的 `metadata`：遵循 Misskey 惯例的 `maintainer`（`name` 是联系账号的昵称，没有昵称时为其用户名；`email` 是 `contact_email`），遵循 Pleroma 和 Akkoma 惯例的 `staffAccounts`（联系账号的 actor ID），`thumbnail`（`url` 和 `alt`），以及来自简短介绍 `short_description` 的 `nodeDescription`。

联系账号被封禁或删除时，不会显示该账号，只显示电子邮件地址。可以在 Web 客户端管理界面的“常规”页面（“服务器设置”，`/admin/settings`）更改它们，也可以使用 CLI 或客户端 API。

<!-- source: one-person-server 6ba524c58a4f -->
### 个人服务器

只属于运营者一个人、作为运营者在联邦宇宙中的个人家园的服务器，就是个人服务器：开启 `single_user` 即可。在 Web 客户端中，管理员可以在**服务器设置**的**注册**页面通过**设为个人服务器**开启或关闭它；当服务器有多个本站账号时，该处会说明服务器拒绝的原因。

```sh
my instance set single_user true
my instance set single_user false   # back to a community server
```

- 只有在恰好有一个本站账号计入时才能开启。每个未被删除的本站账号都计入，包括等待审核、已停用、计划删除、被封禁或被限制的账号，因为它们每一个都可能作为第二个人回来。注册被拒绝的账号不计入。如果是其他数量，`my instance set` 会失败并给出数量，管理 API 返回 `422`，类型为 `/api/v1/problems/single-user-requires-one-account`。服务器的实例 actor 不是账号，永远不计入。
- 随时都可以关闭。开启或关闭都不会删除任何内容，也不会改变其他任何设置的存储值：关闭后，它所限制的内容会恢复原样生效。与所有设置一样，这一更改会记录在审计日志中。

开启期间：

- **注册关闭。** 无论存储的值是什么，`registration_mode` 都按 `closed` 处理，因此注册页面、通过登录服务注册以及 `my admin create` 都不会创建账号。通过登录服务登录仍然可用。
- **只有首页时间线。** `local_timeline_visibility` 和 `federated_timeline_visibility` 按 `off` 处理。话题标签时间线和列表时间线保留。
- **社区所需的功能被隐藏。** 本站时间线和联邦时间线及其流、账号推荐、用户和管理员的邀请、注册审核以及注册屏蔽都返回 `404`，类型为 `/api/v1/problems/single-user-mode`。
- **不能发布新的本站帖子。** 在只有一个人的服务器上，可见范围为“本站”的帖子除了关注者之外谁也看不到，因此新帖子不能设为本站：客户端 API 返回 `422`，并将其从账号可发布的可见范围中去掉。默认可见范围“本站”按“仅关注者”处理且无法保存，面向本服务器用户的语音聊天房间会以仅关注者可见的帖子发布公告。已经是本站可见范围的帖子保持不变。
- **服务器表明自己的身份。** `GET /api/v1/instance` 中的 `single_user` 为 `true`，并以实际生效的注册方式和时间线以及作为 `single_user_account` 的运营者账号一同返回；NodeInfo 返回 `openRegistrations: false` 和 `metadata.singleUser: true`。
- **首页是运营者的个人资料。** 未登录的人打开服务器地址时，会看到其唯一账号的个人资料，以及登录方式和指向服务器所发布的关于页面、服务条款和隐私政策的链接。Web 客户端会在导航、菜单和管理界面中隐藏服务器不提供的内容：本站时间线和联邦时间线、注册、推荐账号、邀请、注册审核、注册屏蔽、可见范围“本站”，以及注册、邀请、公开时间线、首页和推荐账号的设置。

联邦、审核、全文搜索、支持者计划、文件和管理界面与在任何服务器上一样正常工作。

<!-- source: usage 68ad8384438f -->
## 用法

```sh
my instance list                                      # all settings (and whether each is the default) and texts
my instance get registration_mode
my instance set registration_mode open
my instance set name "Example Social"                 # the same name in every locale
my instance set description - < description.txt       # the value "-" reads standard input
my instance get name
my instance set name ""                               # delete the name
my instance history registration_mode                 # revision history (newest first, -limit sets the count)
my instance history description
my instance set -owner admin preview_image ./preview.png   # upload the image as media of admin and use it as the preview image
my instance set preview_image 0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f   # give the ID of media that is already uploaded
my instance set preview_image_alt "A blue bird mascot"
my instance set preview_image ""                      # back to the bundled default image
my instance set contact_account admin                 # the contact account
my instance set contact_email admin@example.com
my instance set -owner admin thumbnail ./thumbnail.png  # the thumbnail
my instance set thumbnail_alt "A town by the sea"
```

- 标志写在键和值之前。
- 使用 `-owner <username>` 时，`preview_image` 或 `thumbnail` 的值会被当作图片文件的路径读取，并作为该本站账号的媒体上传。图片在使用前由 worker 处理。推荐使用 1200x630 的横向图片。
- 一次更改会在同一个事务中记录一个修订版本（`instance_*_revisions`）并更新当前值（`instance_settings` / `instance_texts`）。通过 CLI 进行的更改不记录操作者（历史中的 `BY` 为 `cli`）。
- 再次设置相同的值仍然会添加一个修订版本。
- 退出码：0 表示成功，1 表示值被拒绝或数据库错误（不会有任何更改），2 表示参数错误。

<!-- source: changing-the-direction-of-federation 9b9b63689c84 -->
### 更改联邦的方向

- 收窄方向时，关注关系、已导入的帖子和投递记录都会保留，也不会发送 `Undo` 或 `Reject`。流动只是停止，恢复为 `both`（或其他模式）后会按原样继续。
- 收窄期间发布的帖子和互动，在你再次放宽后不会追溯投递。
- 在远程一方，模式为 `inbound` 期间，本站用户的关注者收不到任何内容；模式为 `outbound` 或 `off` 期间，本站用户的关注不会更新。
- 模式为 `off` 期间进行的删除（帖子或账号）不会发送出去。在 `inbound` 和 `outbound` 模式下，对已发送内容的撤回（`Delete` 等）会发送给原来的接收者以及当时的关注者。
- 更改会记录在 `instance_setting_revisions` 和审计日志中（`target_kind = setting`）。

如果还要切断关系，请使用 CLI `my admin federation sever`。

```sh
my admin federation sever -direction in     # end follows of local users by remote accounts
my admin federation sever -direction out    # end follows of remote accounts by local users
my admin federation sever -direction both
```

- 它会将本站账号与远程账号之间待处理和有效的关注作为审核终止来结束（`follow_terminations`，`reason = moderation`）。更改模式永远不会自动调用它。
- 只有当前模式发送该方向时，才会向远程一方发送 `Reject`（`in`）或 `Undo`（`out`）。在不发送该方向的模式下（例如 `off`），关注只在本站一侧结束，不会通知远程一方。
- 可以安全地重复运行。它会在标准输出中打印所结束的关注数。

<!-- source: admin-screens-and-the-client-api 6a747f50af97 -->
## 管理界面与客户端 API

CLI 能更改的每项设置，也都可以在 Web 客户端的管理界面上或通过客户端 API 更改。两者都需要角色权限 `manage_settings`（只有内置的 `admin` 角色拥有）。与 CLI 一样，更改会保存为修订版本，并按键记录在审计日志中（`setting_changed`，对象是该设置，操作者是该用户）。可以在审核界面（“审计日志”，`/moderation/audit-log`）查看审计日志。

- **Web 客户端**（“服务器设置”，`/admin/settings`）：该界面每个主题一个页面，链接列在标题下方。每个页面有一个表单和一个保存按钮，保存时只发送你在该页面上更改的内容；其他页面上的设置保持不变。

  | 页面 | 分区与设置 |
  | --- | --- |
  | 常规（`/admin/settings`） | 名称、介绍（`name`、`short_description`、`description`）；联系方式与缩略图（`contact_account`、`contact_email`、`thumbnail` 以及文本 `thumbnail_alt`） |
  | 图标与徽标（`/admin/settings/branding`） | 图标、徽标以及用于深色背景的徽标，各自用自己的按钮保存（[徽标与图标](#logo-and-icon)） |
  | 注册（`/admin/settings/registration`） | 注册（`registration_mode`、`login_providers_only`）；成员邀请（`invite_creators`、`member_invite_max_active`、`member_invite_max_uses`、`member_invite_max_lifetime_seconds`）；应用（`user_app_registration`、`oauth_metadata_document_clients`、`dynamic_client_registration`） |
  | 内容（`/admin/settings/content`） | 语言（`default_locale`，从应用支持的语言中选择）；时间线（`local_timeline_visibility`、`federated_timeline_visibility`）；帖子（`require_media_description`）；落地页（`public_counts`）；推荐账号（`featured_accounts`） |
  | 帖子删除（`/admin/settings/auto-delete`） | 新账号的默认设置（`auto_delete_default_*` 设置）；上限（`auto_delete_max_age_days`、`auto_delete_max_allow_*` 设置、`auto_delete_max_kept_pins`、`auto_delete_grace_days`）。保存开启或收紧（天数更少、例外或置顶更少）的上限之前会先进行确认，因为它会通知新受其约束的账号（[帖子保留的上限](#a-maximum-for-keeping-posts)） |
  | 联邦（`/admin/settings/federation`） | 联邦（`federation_mode`、`authorized_fetch`、`remote_media_policy`、`remote_media_cache_ttl_seconds`（从 1 小时、1 天、7 天、30 天或 365 天中选择）、`link_cards`、`link_cards_ignore_robots_txt`）；已限制和已封禁的服务器（`domain_actions_visibility`、`domain_actions_rationale_visibility`）；外部审查（`hook_*` 设置，参见[决策钩子](decision-hooks.md)） |
  | 搜索与爬虫（`/admin/settings/discovery`） | 搜索引擎与 AI（`search_indexing`、`indexable_default`、`ai_usage`、`security_contact`）；帖子搜索（`search_visibilities`，仅在服务器具备全文搜索时） |
  | 语音聊天（`/admin/settings/voice`） | `voice_room_starters`、`voice_public_summary`、`voice_participants_retention_days`；只有服务器提供语音聊天时才会列出该页面 |

  在“常规”页面上，选择图片时就会上传，保存时它才成为缩略图。谁能查看运营成本、收入和支持者列表，在“财务”和“支持者”界面上设置。

  `max_status_length`、`preview_image`、`appeal_window_days`、各保留期限（`*_retention_days`）、`remote_media_cache_max_bytes` 和 `oauth_client_rate_limits` 不会出现在界面上；请使用 CLI 或客户端 API 更改它们。翻译的设置有自己的界面，见下文。
- **Web 客户端**（“翻译”，`/admin/translation`）：本服务器拥有的服务商、它们的顺序以及每个服务商可以接收的内容，浏览器翻译，每月上限，以及翻译保留多久，并显示本月已发送的字符数和缓存命中率。每个服务器进程最多每分钟一次将其缓存命中数计入命中率，因此最新的请求可能需要一分钟才会显示，而停止的进程可能会留下最后一分钟未计入。在启用服务器端服务商之前，请更新你的隐私政策（[模板](privacy-policy-template.md#the-services-you-choose)）：发送给服务商的文本无法收回。服务商的 API 密钥和[翻译插件](plugins.md#translating-posts)的地址保存在[部署配置](../reference/configuration.md)中；任何设置和客户端 API 的任何响应都不包含它们。
- **翻译上限。** 计数的是发送的字符数，以码位计：代替提及、链接等的标记不计入，服务器已保存的翻译、已经是读者语言的帖子以及浏览器翻译也不计入。每次调用服务商都会计数，包括失败的调用，因为服务商可能已经收到了文本，所以这个数字可能与服务商计费的数字不同。超过上限时，请求会以 `429` 被拒绝，不会调用任何服务商；此时如果你允许、且浏览器能翻译该帖子，Web 客户端会使用读者浏览器的翻译。月份按 UTC 计算。
- **客户端 API**：`GET /api/v1/admin/settings` 返回每项设置的当前值、默认值、是否已设置，以及类型和可接受的值（选项或范围）。`PATCH /api/v1/admin/settings` 用 `{"values": {"<key>": "<value>", ...}}` 一并更改多项设置。所有值都是字符串，与 CLI 一样经过规范化后存储。只要有一个键未知或有一个值无效，就会返回指明该键的 422，不会有任何更改。更改的键会在同一个事务中记录为修订版本并写入审计日志。
- 邀请在 `open`、`approval` 和 `invite` 模式下有效，可以在审核界面上创建（“邀请”，`/moderation/invites`，权限 `manage_invites`；参见[邀请码](registration.md#invite-codes)），成员也可以在 `invite_creators` 允许的范围内创建（[成员邀请](registration.md#member-invites)）。

<!-- source: rules b1076bb81e0c -->
## 规则

实例的规则出现在 `GET /api/v1/instance` 的 `rules` 中，举报按 ID 引用它们。

- 一条规则有一句单行的句子（最多 300 个字符）和一段可选的补充说明（最多 1,000 个字符）。与文本一样，管理员用自己喜欢的任意语言编写，所有语言区域看到的都是同一个值（`dir="auto"` 和 `lang=""`）。有效的规则最多 100 条。
- 每次更改（修改句子、调整顺序）都会保存为修订版本（`instance_rule_revisions`），规则的 ID 不会改变。
- 停用的规则会从有效规则列表中消失。它无法恢复，但举报仍然可以引用它。

```sh
my instance rule create -hint "No harassment" "Be kind"   # add at the end of the active rules
my instance rule edit <rule> "Always be kind"          # change the sentence; the supplement changes only with -hint
my instance rule move <rule> 1                         # move to position 1 and renumber the active rules 1, 2, ...
my instance rule retire <rule>                         # retire it
my instance rule list                                  # active rules (-all includes retired ones)
```

- 管理 API 是 `/api/v1/admin/rules`（列出、添加、获取、修改、移动、停用），需要角色权限 `manage_rules`。
- 在 Web 客户端的管理界面（“规则”，`/admin/rules`，`manage_rules`）上，列表按顺序显示生效中的规则，用“上移”和“下移”调整顺序；列表上方的筛选器可以改为显示已停用的规则。“添加规则”会打开一个填写句子和补充说明的页面。选择一条规则的句子即可打开它的页面，在那里编辑它或停用它（经过确认对话框之后）。每次更改都会记录在审计日志中（`rule_added`、`rule_revised`、`rule_moved`、`rule_retired`）。
- 退出码与设置相同（0 表示成功，1 表示值被拒绝或数据库错误，2 表示参数错误）。

<!-- source: link-card-domains c8f674387fa8 -->
## 链接卡片域名

帖子的链接卡片（`link_cards`）来自一个爬虫：服务器获取被链接的页面及其图片。一条规则会在某个域名及其子域名上停止这个爬虫，而不影响该域名的其他任何方面。规则有两种模式之一。规则是运营者的选择，它在被链接网站自己拒绝的内容之外额外生效（[链接预览](link-previews.md#cards-of-links-in-posts)）。

| 模式 | 获取什么 | 已有的卡片 |
| --- | --- | --- |
| 停止获取（`no_fetch`） | 不从该域名获取任何内容，页面和图片都不获取，重定向到那里时也一样 | 隐藏 |
| 仅停止图片（`no_images`） | 获取页面，但不从该域名获取图片 | 显示，但不带图片 |

- 规则只影响链接卡片。该域名仍然联邦，其帖子的媒体照常获取：如果要切断某个服务器，请使用[域名措施](moderation.md)，它也会停止该服务器的链接卡片。规则不会与域名措施一起列出，也不会与它们一起导出。
- 规则涵盖该域名及其子域名，图片会同时按页面的主机和图片的主机进行检查。涵盖某个主机的每条规则都会生效，因此子域名上的规则不会放宽其父域名上的规则。域名的写法和规范化方式与域名措施相同：不带协议和端口的主机名，国际化域名可以使用任意写法。
- 更改立即生效，也作用于服务器已有的卡片；在停止图片期间，这些卡片的代理图片也会停止提供。
- 移除规则后，服务器已有的卡片会恢复原样。被跳过的链接会在下次有帖子需要时获得卡片；不会批量重新获取。在停止图片期间重新获取的卡片，直到下一次重新获取之前都不带图片。
- 在 Web 客户端的管理界面（“链接卡片域名”，`/admin/link-card-domains`，`manage_settings`）上，“添加规则”可以设置一条带备注的规则。列表每页显示 40 条规则，可以按域名搜索；选择一个域名会打开其规则的页面，在那里更改模式和备注，或移除规则（经过确认对话框之后）。
- 管理 API 是 `/api/v1/admin/link_card_domains`（列出、设置；对域名发送 `DELETE` 即可移除），需要角色权限 `manage_settings`。一个服务器最多保存 1,000 条规则，每条的备注最多 5,000 个字符。
- 每次更改都会记录在审计日志中（`link_card_domain_fetch_stopped`、`link_card_domain_images_stopped`、`link_card_domain_rule_removed`）。

<!-- source: theme 380636a0e583 -->
## 主题

服务器全局主题是一组对 Web 客户端设计令牌的覆盖，外加一段可选的 CSS（自定义 CSS）。它适用于每位查看者的 Web 客户端以及服务器渲染的界面（登录、注册、条款等），浅色和深色都适用。

- 你可以覆盖 `app/ui/src/theme.css` 中除基础颜色（`--palette-*`）以外的所有令牌。令牌列表、默认值和范围见 [app/ui/README.md](../../app/ui/README.md#server-themes)。颜色分别为浅色和深色指定 `#rrggbb`（`color-overlay` 也可以是 `#rrggbbaa`）；其他令牌接受符合其类型格式（长度、数字、时长、缓动、字体名称列表、阴影）且在其范围内的值。覆盖 `font-sans` 或 `font-mono` 时，请将 `"my Noto Color Emoji"` 保留在最前面；否则 Web 客户端的表情会使用操作系统的字体绘制。
- 保存时，服务器会检查值的格式和范围、每一对颜色令牌的对比度（未覆盖的令牌使用默认值；浅色和深色都需满足 WCAG 2.2 AA），以及自定义 CSS 的长度（64 KiB）。只要有一项检查失败，就不会保存任何内容。
- 服务器不解读自定义 CSS。它把自定义 CSS 放在同一个样式表中令牌规则之后，位于任何 `@layer` 之外。它与 Web 客户端在相同的 CSP 下运行，因此无法加载来自其他源的图片、字体和 `@import`。**服务器不检查自定义 CSS 的效果（对比度、横向滚动、点击目标大小、焦点指示、从右到左的语言）；这是管理员的责任。** 类名和页面结构会随版本变化，因此只有令牌是可以稳定依赖的。
- 主题以修订版本保存（`instance_theme_revisions`），当前修订版本的样式表在 `/theme/{revision ID}.css` 提供（`immutable`，长期缓存）。HTML 外壳使用共享缓存（60 秒，外加 300 秒的重新验证），因此保存后需要几分钟才能让所有人都拿到。已经打开的页面在重新加载之前会保持旧的外观。
- 外壳中的 `<meta name="theme-color">`（浅色和深色）以及 Web App Manifest 的 `theme_color` / `background_color`（浅色）取主题的 `color-surface` 的值。
- 保存一个没有令牌也没有自定义 CSS 的修订版本，会将主题重置为默认值，样式表也不再加载。
- 可以在 Web 客户端的管理界面（“主题”，`/admin/theme`）上、通过客户端 API（`GET` / `PUT
  /api/v1/admin/theme`）以及通过 CLI 更改它。管理界面和客户端 API 需要角色权限 `manage_settings`，更改会记录在审计日志中（`setting_changed`，对象为 `theme`）。

```sh
my instance theme get > theme.json                    # print the current theme as the same JSON as the admin API
my instance theme set < theme.json                    # replace the whole theme (if a check fails, it prints why and changes nothing)
echo '{"tokens":[],"custom_css":""}' | my instance theme set   # back to the default
```

```json
{
  "tokens": [
    { "token": "color-accent", "light": "#1447e6", "dark": "#92c5fe" },
    { "token": "radius-md", "value": "0.25rem" }
  ],
  "custom_css": ".brand { letter-spacing: 0.02em; }"
}
```

<!-- source: logo-and-icon a2d23def322b -->
## 徽标与图标

除了缩略图和预览图之外，实例还可以拥有自己的图标、徽标和深色徽标，每个都以一个 SVG 上传。在设置之前使用内置的 mycroblog 图片，清除后也会恢复为内置图片。

| 图片 | 形状 | 出现的位置 |
| --- | --- | --- |
| `icon` | 正方形（误差在 1% 以内） | 网站图标、iOS 的主屏幕图标（`apple-touch-icon`）、已安装应用的图标（Web App Manifest）、首页上的服务器卡片和 Web 客户端侧边面板中服务器名称旁边，以及客户端以正方形图片显示服务器的任何地方 |
| `logo` | 宽度是高度的 1 到 8 倍 | Web 客户端的页眉以及登录、注册和同意页面的顶部，代替内置的文字标志和服务器名称（徽标应包含名称，名称即其替代文本）；每封电子邮件的 HTML 版本；以及未设置 `preview_image` 时的链接预览图 |
| `logo_dark` | 与 `logo` 相同 | 深色背景；没有它时，那里也使用 `logo` |

- **服务器会清理 SVG。** 服务器将它作为 XML 读取，并只用允许的元素（形状、分组、`defs`、`symbol`、`use`、渐变、`clipPath`、`mask`、`pattern`、`style`、`title`、`desc`）和属性（几何、变换、呈现属性、`id`、`class`、`style`）写出一个新文档。脚本、`foreignObject`、嵌入的图片、链接、动画、滤镜（阴影和模糊）、事件处理器、对文件外任何内容的引用以及 `data:` URL 都会被移除，响应会列出被移除的内容。只存储清理后的 SVG。
- 以下情况的 SVG 会被拒绝（不会保存任何内容）：不是格式良好的 UTF-8 XML、带有 `DOCTYPE` 或实体声明、没有 `viewBox`、超过 256 KiB（清理后超过 128 KiB）、元素超过 5,000 个、嵌套深度超过 32 层、`use` 元素超过 200 个、形状不符合其种类、包含 `text`（请先在编辑器中将文字转换为轮廓），或者带有可能加载内容的 CSS（`@import`、`@font-face`、`url(#id)` 以外的 `url()`、转义）。允许在 `<style>` 中使用 `@media (prefers-color-scheme: dark)`，因此图标可以在深色模式下变化。
- 有些地方不接受 SVG（iOS 主屏幕、较旧的 Safari 网站图标、链接预览、电子邮件）。为此，上传 SVG 的客户端还会根据清理后的 SVG 绘制 PNG 版本并一同发送：`icon` 为 32、48、180、192 和 512 px 以及一个 512 px 的可遮罩图标；`logo` 为一个 96 px 高的电子邮件徽标和一张 1200 × 630 的链接预览图；`logo_dark` 为一个 96 px 高的电子邮件徽标。服务器会检查每个文件是否为尺寸正确的 PNG（最大 1 MiB；180 px 图标、可遮罩图标和链接预览图必须不透明），并去除元数据后重新写入。只要有一个文件不正确，就不会保存任何内容。图标的不透明版本绘制在一个背景色 `#rrggbb` 上，默认为主题浅色的 `color-surface`。服务器不检查 PNG 版本显示的画面是否与 SVG 相同。
- 这些文件存储在 blob 存储的 `instance/brand/<revision>/` 下，并在 `https://<host>/brand/<revision>/<version>.svg|.png` 提供，带有 `Cache-Control: public, max-age=31536000, immutable`，因为一个修订版本的内容永远不会改变。SVG 提供时带有 `Content-Security-Policy: default-src 'none'; style-src
  'unsafe-inline'; sandbox` 和 `X-Content-Type-Options: nosniff`，因此即使单独打开，它也不会运行或加载任何内容。较早修订版本的文件会保留，因此已发送的电子邮件仍然保有其徽标。
- 设置了图标后，HTML 外壳会链接其 PNG 网站图标、SVG 以及 180 px 的 `apple-touch-icon`，Web App Manifest 按顺序列出其 512 px、192 px、可遮罩和 SVG 图标（其他联邦宇宙服务器会将第一个作为本服务器的图标），`/apple-touch-icon.png` 返回 180 px 版本。外壳的 ETag 会随图片变化，外壳的共享缓存与主题一样会持续几分钟。在 Android 上，已安装的应用会在 Chrome 下次检查 manifest 时获取新图标；在桌面上，应用会保持旧图标，直到重新安装。
- `GET /api/v1/instance` 返回 `icon`、`logo` 和 `logo_dark`，包含 SVG 的 URL、每个 PNG 版本的 URL 和尺寸以及修订版本；使用内置图片时会省略相应项。
- 在 Web 客户端的**服务器设置** > **图标与徽标**（`/admin/settings/branding`，角色权限 `manage_settings`）中设置它们。为图标、徽标或深色背景的徽标选择一个 SVG 文件：服务器检查它，浏览器绘制 PNG 版本，表单会预览它们（浏览器标签页、主屏幕、应用图标、浅色和深色背景上的电子邮件，以及链接预览），并列出服务器移除的内容。如果服务器拒绝该文件，原因和修正方法会显示在按钮下方。图标的不透明版本的背景色也在那里设置。**保存**会发送 SVG 及每个 PNG 版本；**使用内置图标**、**使用内置徽标**和**移除深色背景徽标**会清除它们。Web 客户端会保留已显示过的图片以供离线使用。
- 客户端 API 可以做同样的事，需要角色权限 `manage_settings`：`POST /api/v1/admin/instance/brand/sanitize` 检查一个 SVG，并返回清理后的 SVG 以及需要绘制的 PNG 版本（不保存任何内容），`PUT /api/v1/admin/instance/brand/{icon|logo|logo_dark}` 保存 SVG 及每个 PNG 版本（`multipart/form-data`，每个版本一个部分），对同一路径发送 `DELETE` 会清除其中一个，`GET /api/v1/admin/instance/brand` 返回当前的图片以及图标的背景色。每次更改都会保存为修订版本（`instance_brand_revisions`）并记录在审计日志中（`setting_changed`，对象为 `brand_icon`、`brand_logo` 或 `brand_logo_dark`）。
- CLI 不能设置它们，因为 PNG 版本由客户端绘制，但它可以显示它们，并清除损坏的图片：

```sh
my instance brand show          # each image with its revision, when and by whom it was set, and the URL of its SVG
my instance brand clear icon    # back to the built-in icon (also logo, logo_dark)
```

<!-- source: custom-emoji 5e0ddb3251cf -->
## 自定义表情

当有人在帖子的正文或内容警告中，或在昵称、简介或个人资料字段中写下 `:shortcode:` 时，本站的自定义表情会显示为图片。它们也会通过你发送的 Note 和 actor 的 `Emoji` 标签传到 Mastodon、Misskey、GoToSocial 和 Akkoma。

- 短代码由 2-30 个字母、数字或 `_` 组成。它区分大小写，表情注册后不能更改。别名（最多 10 个）和分类（最多 64 个字符）用于在表情选择器中搜索和分组。
- 图片为 PNG、GIF 或 WebP，最大 `custom_emoji_max_bytes`（默认 256 KiB），每边最多 1,024 px。服务器会生成适配 64、128 和 256 px 框的派生图片，不保留原图。APNG 会变成其第一帧的静态图片。GoToSocial 默认不获取大于 100 KiB 的表情，因此推荐使用小图片。
- 已停用的表情不会出现在任何地方（帖子、个人资料、联邦、选择器），`:shortcode:` 会重新显示为文本。设置为在选择器中隐藏的表情只是不出现在选择器中，仍然可以通过手写使用。来自其他服务器的表情只能停用，或复制为本服务器的表情。
- 复制会在与媒体代理相同的限制下获取其他服务器表情的图片（不会从带有 `reject_media` 的域名获取图片），执行与上面相同的检查，并将其注册为新的本站表情。短代码为原表情的短代码或你指定的短代码。图片会缩小到 256 px 的框内，无法获取或未通过检查的图片不会留下任何内容。
- 删除无法撤销。过去帖子中的表情也会变回文本，再次注册相同的短代码会得到一个不同的表情。联邦服务器已复制的图片无法删除。
- 管理界面（“自定义表情”，`/admin/custom-emojis`）、客户端 API（`/api/v1/admin/custom_emojis`）和 CLI 可以做同样的事。在界面上，“注册自定义表情”会打开注册表单（`/admin/custom-emojis/new`），在列表中选择一个表情会打开它的页面（`/admin/custom-emojis/{id}`），在那里可以更改其设置、替换其图片、启用或停用、删除它，或复制其他服务器的表情；列表本身可以一键启用和停用表情。管理界面和客户端 API 需要角色权限 `manage_custom_emojis`（只有内置的 `admin` 拥有）。注册、复制、替换图片、更改设置和别名、启用和停用以及删除，无论以何种方式进行，都会记录在审计日志中（对象为 `custom_emoji`）（来自 CLI 的操作记录为 CLI 操作）。

```sh
my admin emoji create -shortcode blobcat -category Cats -alias neko blobcat.png   # register one
my admin emoji list -local                                                        # list local emoji, newest first
my admin emoji update -category Animals -alias cat -alias neko <emoji id>         # change the category and aliases (aliases are replaced; -no-aliases removes them)
my admin emoji update -hidden <emoji id>                                          # hide it from the picker (-hidden=false restores it)
my admin emoji replace-image <emoji id> blobcat2.png                              # replace the image
my admin emoji disable <emoji id>                                                 # disable it (remote emoji too)
my admin emoji enable <emoji id>                                                  # enable it again
my admin emoji list -remote -domain mastodon.example                              # list emoji of another server
my admin emoji copy [-shortcode <name>] <emoji id>                                # copy a remote emoji to local
my admin emoji delete <emoji id>                                                  # delete it
my admin emoji requests list [-state pending|approved|rejected|withdrawn|lapsed|all] # list creation requests (pending by default)
my admin emoji requests approve <request id>                                      # approve a request
my admin emoji requests reject [-reason <text>] <request id>                      # reject a request (the requester sees the reason)
```

- 拥有角色权限 `create_custom_emojis`（一项用户功能）的用户可以在设置的“自定义表情”（`/settings/custom-emojis`）中申请创建表情，更改自己所创建表情的别名、分类和在选择器中的可见性，也可以删除它们。替换图片也是一种申请。申请在拥有 `manage_custom_emojis` 的人批准之前不会出现在任何地方（选择器、帖子、回应、联邦）。可以在管理界面“自定义表情”的“请求”页面（`/admin/custom-emojis/requests`）、通过客户端 API（`/api/v1/admin/custom_emoji_requests`）或使用上面的 CLI 批准或拒绝它。拥有 `manage_custom_emojis` 的人会收到申请的通知（`admin_custom_emoji_request`），申请人会收到批准或拒绝的通知。申请、撤回、批准和拒绝都会记录在审计日志中（对象为 `custom_emoji_request`）。
- 待处理的创建申请会预留其短代码。在待处理的申请被拒绝或撤回之前，不能用相同的短代码申请或注册。已批准表情的图片会作为实例 actor 的上传进行复制，因此即使申请人的账号被删除，它也会保留。
- 创建者只有在持有 `create_custom_emojis` 期间才能更改自己的表情。角色被移除后，表情仍会保留。无论创建者是谁，管理员都可以更改和删除每个表情。
- 当角色来自支持者方案时，用户每月可以创建的表情数量受方案的每月表情上限限制。它统计用户在当前日历月（UTC）内提交的、已批准或待处理的创建申请；被拒绝和已撤回的申请不计入，删除表情也不会腾出名额。申请计入其提交的月份，即使它在下个月才被批准。计数在每月第一天的 00:00 UTC 重置，设置页面会以用户自己的时区告知重置时间。替换表情的图片以及管理员注册的表情不计入。管理员手动分配的角色，以及禁用了支持者功能的服务器，没有上限。
