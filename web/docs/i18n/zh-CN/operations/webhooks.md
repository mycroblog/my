<!-- source: server-webhooks e73d0e0966b7 -->
# 服务器 Webhook

Webhook 会把举报、注册申请、账号创建和个人资料变更、管理操作、申诉、域名操作、本站的公开帖子以及自定义表情的管理操作，发送到 Slack 或 Discord 的中继、工单系统，或者 n8n 等自动化工具。没有配置任何 Webhook 的服务器不会存储额外的行，也不会运行额外的任务。接收方会收到什么（头部、签名、事件、正文）见 [Webhook 参考](../reference/webhooks.md)。

<!-- source: set-up-a-webhook c8a80bc40038 -->
## 设置 Webhook

拥有 `manage_webhooks` 权限的人都可以在管理页面“Webhook”（`/admin/webhooks`）中设置 Webhook。内置的 `admin` 角色拥有该权限。“添加 Webhook”会打开表单；在列表中选择一个 Webhook 会打开它的页面，你可以在那里修改、测试、启用、停用和删除它，为它生成新密钥，并查看它的投递记录。

1. 创建 Webhook，填写目标 URL、说明以及要订阅的事件。除了 `manage_webhooks` 之外，每个事件还需要各自的权限：举报需要 `manage_reports`；注册、账号、管理操作和申诉需要 `manage_accounts`；域名操作需要 `manage_domains`；自定义表情需要 `manage_custom_emojis`；错误组需要 `manage_errors`。公开帖子不需要额外权限。你无法选择自己缺少相应权限的事件。
2. 密钥（`whsec_...`）只在创建 Webhook 时显示一次。请把它设置到接收方。之后无法再次显示。
3. 在 Webhook 的页面上选择“发送测试”来发送 `webhook.test`，并在投递记录中确认它已送达。

Webhook 的创建、修改、删除、启用、停用和密钥轮换都会记录在审计日志中。目标显示为说明和 URL 的主机名；URL 本身不会被记录。

<!-- source: rotate-the-secret 928c62f87538 -->
## 轮换密钥

“新密钥”会创建新密钥并显示一次。在接下来的 24 小时内，消息也会用旧密钥签名（`webhook-signature` 带有两个签名），因此你可以在这段时间内把接收方切换到新密钥。密钥在存入数据库之前会用 `kernel.Sealer` 加密；所用的密钥是
[KEK](key-management.md)。

<!-- source: failures-and-redelivery 65aba0d2089e -->
## 失败与重新投递

- 如果目标返回 `2xx` 以外的响应、15 秒内没有响应，或者无法访问，消息会在 5 秒、5 分钟、30 分钟、2 小时、5 小时、10 小时和 10 小时后重试（共 8 次尝试，约 27 小时）。用完所有重试的投递会成为死任务，在投递记录中选择“重新发送”会以相同的 `webhook-id`
  再次发送它。
- 如果目标返回 `410 Gone`，Webhook 会立即停用。
- 如果一个目标自首次失败起 72 小时内都没有成功，并且连续失败 20 次或以上，服务器会停用它。审计日志会记录一条由 `system` 执行的 `webhook_disabled`，所有拥有 `manage_webhooks` 的人都会收到应用内通知（如果他们选择通过电子邮件接收通知，还会收到邮件）。
- 管理页面的列表会显示连续失败的次数，以及服务器停用 Webhook 的原因（`gone` 或 `failing`）。修复目标后请重新启用 Webhook。停用期间的消息不会再次发送。
- 消息和投递记录在 14 天后删除（固定）。正文中包含个人数据，例如举报的附言。

<!-- source: destinations-on-an-internal-network 3f8b39001831 -->
## 内部网络上的目标

投递会经过出站 HTTP 的 SSRF 防护。默认情况下，它只能通过 `https`（端口 443
和 80）访问公网地址。要发送到内部目标（例如同一主机上的 n8n 或 VPC 内的聊天服务），请在设置 `webhook_allowed_destinations`（环境变量
`MY_WEBHOOK_ALLOWED_DESTINATIONS`；参见[配置参考](../reference/configuration.md#webhook_allowed_destinations)）中列出主机名、IP 地址和 CIDR，以逗号分隔。

```sh
MY_WEBHOOK_ALLOWED_DESTINATIONS=n8n.internal,10.0.0.0/8
```

- 只有列出的目标、并且只在 Webhook 投递时才不受防护限制。联邦、媒体和链接预览不受影响。不受限制的目标也可以使用 `http` 和任意端口。
- 主机名无论解析到什么地址都会被允许。当 URL 指定的 IP 地址落在某个 CIDR 范围内时，才允许该 CIDR。如果你使用的主机名会解析到该范围内，请把主机名也列出来。
- 只有能访问服务器的运营者才能更改该设置。仅凭管理页面的权限无法访问内部网络。
- 被防护拒绝的目标的投递尝试，会在投递记录中显示 `forbidden_destination`。
- 开发用设置 `http_allow_private_networks` 会为所有用途解除防护，包括 Webhook。
