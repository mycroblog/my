<!-- source: sending-email 941ecea58cd9 -->
# 发送邮件

服务器会发送哪些邮件、如何配置发送方式，以及如何处理发送失败和退订。
各项设置列在[配置参考](../reference/configuration.md#mail)中。
面向用户的邮箱验证和密码重置流程见[注册与邮箱验证](registration.md)。

<!-- source: what-is-sent-and-how-to-configure-it 3f387cd1afcc -->
## 发送哪些邮件，以及如何配置

服务器从不在请求处理过程中发送邮件。它会针对每种用途，在触发邮件的那个事实所在的同一事务中把一个任务放入队列，再由任务 worker 发送。发送失败不会体现在注册或重置的响应中：
用户看到的是“我们已发送验证邮件”，如果没有收到，就再请求一次。

| 类别 | 邮件 | 任务类型 | 用户能否退订？ |
| --- | --- | --- | --- |
| 事务性（`transactional`） | 邮箱地址验证 | `identity.verification_email` | 否 |
| 事务性 | 密码重置 | `identity.password_reset_email` | 否 |
| 事务性 | 密码修改或删除的通知 | `identity.password_notice_email` | 否 |
| 事务性 | 账号停用的通知，附带删除日期 | `identity.deactivation_email` | 否 |
| 事务性 | 服务关闭（公告、日期变更、取消、关闭日期前 7 天和前 1 天、关闭时，以及导出截止日期前 7 天） | `identity.closure_email` | 否 |
| 通知（`notification`） | 发给审核员的通知（默认开启，按通知种类设置） | `notification.moderator_email` | 是 |
| 通知（`notification`） | 15 分钟内一直未读的通知：提及、关注、转发以及来自其他用户的其他通知，还有来自审核员的通知（默认关闭，按种类设置） | `notification.email` | 是 |

面向支持者的通知邮件（`notification.supporter_email`）也经过同一个任务队列。

<!-- source: which-notifications-are-emailed 76c2b871762c -->
### 哪些通知会发送邮件

每个用户都可以在设置的 **通知** 页面（Web 客户端中的 `/settings/notifications`）按通知种类选择是否在应用内接收、是否通过邮件接收。只有在服务器发送邮件时，那里才会显示“邮件”一栏。在应用内关闭的种类根本不会生成通知，因此也不会发送邮件或推送。

| 种类 | 默认发送邮件 | 发送时机 |
| --- | --- | --- |
| 来自其他用户的通知（提及、新关注者、关注请求、关注请求被接受、转发、喜欢、表情回应、引用、投票结束、你关注的账号迁移、语音聊天） | 关闭 | 通知产生 15 分钟后，仅当它仍未读（未标记为已读、未关闭）且仍然可见，并且用户仍通过邮件接收该种类时发送。每条通知一封邮件 |
| 来自审核员的通知（对账号的审核操作、举报已处理、申诉已裁定、因封禁而失去的关注、自定义表情请求已裁定） | 关闭 | 同上 |
| 发给审核员的通知（新举报、待审批的注册、申诉、被停用的 webhook、自定义表情请求、受限账号迁移、关闭日期已过、新出现和再次出现的错误），发给其角色拥有相应权限的人 | 开启 | 立即发送，不等待已读 |
| 服务关闭，以及支持者方案的通知 | 始终 | 立即发送；无法关闭 |

通知邮件只发送到用户已验证的主邮箱地址，并且从不发送到邮件已被停止的地址（见下文）。来自其他用户的通知，其邮件主题与对应的 Web Push 消息文本相同，邮件中会链接到帖子、个人资料或通知页面。

服务关闭邮件会发送给所有主邮箱地址已验证的用户。错过这类邮件可能意味着丢失数据，因此它们作为事务性邮件发送，不带退订链接，也不会因投诉而停止（硬退信会停止它们）。
公告、日期变更、取消和关闭各自会在其记录所在的同一事务中把 `identity.closure_email_fanout` 放入队列。每小时运行的 `identity.queue_closure_reminders` 决定何时发送关闭日期前 7 天和前 1 天、以及导出截止日期前 7 天的提醒。如果某个提醒的时间在公告发布时已经过去，就不会发送，因为公告邮件中已写明了这些日期。
分发任务会为每个用户记录一行 `service_closure_emails`，并且只为成功记录的用户把 `identity.closure_email` 放入队列，因此无论该任务运行多少次，每个用户每封邮件只会收到一次。发送前，任务会检查这封邮件是否仍然有效（不是针对已取消的公告，也不是针对之后已变更的日期）。

设置项 `mail` 决定邮件的发送方式。

| `mail` | 行为 |
| --- | --- |
| `none`（默认） | 不发送任何邮件，并把 `mail not sent (mail = none)` 连同消息类别写入服务器日志。在 `profile = development` 下，这一行还包含地址、主题和正文（含链接）。适用于不使用邮件的部署和开发环境 |
| `smtp` | 通过 SMTP 提交发送。要求 STARTTLS，并在认证和发送之前验证证书 |
| `ses` | 通过 Amazon SES v2 API（使用原始内容的 `SendEmail`）发送。通过 AWS SDK 的默认凭证链（例如 IAM 角色）认证，不保存 SMTP 密码 |
| `plugin` | 把每封消息交给你运行的[插件](#a-plugin-mail--plugin)，适用于只提供 HTTP API 的邮件服务 |

`smtp` 的设置项是 `mail_from`、`mail_smtp_host`、`mail_smtp_port`（默认 587）、`mail_smtp_username` 和 `mail_smtp_password`。`ses` 的设置项是 `mail_from`、`mail_ses_region` 和 `mail_ses_configuration_set`。`plugin` 的设置项是 `mail_from` 和 `plugin_mailer_address`，以及[其目标地址](plugins.md#the-destination)的其余设置。
`mail_from` 是一个纯地址，例如 `noreply@social.example`。`From` 会变成 `"<server name>" <mail_from>`，`mail_from` 的域名同时也是 `Message-ID` 的域名，以及 SPF、DKIM 和 DMARC 所适用的域名（见下文）。
设置了实例设置 `contact_email` 时，`Reply-To` 就是它（[实例设置](instance-settings.md)）。
每封邮件都带有 `Auto-Submitted: auto-generated`。
`smtp` 和 `ses` 构建的消息相同。不使用 SMTPUTF8，因此地址以 ASCII 发送，国际化域名会转换为 Punycode。本地部分（`@` 之前）不是 ASCII 的地址无法发送，其任务会失败。

<!-- source: examples-by-provider 7ab129a02c43 -->
## 各服务商的示例

通过环境变量或文件（`MY_MAIL_SMTP_PASSWORD` 或 `MY_MAIL_SMTP_PASSWORD_FILE`）传递密钥。不要把它们写进配置文件。

<!-- source: amazon-ses-api 1cef4371e61c -->
### Amazon SES（API）

在 AWS 上运行时，`mail = ses` 使用 SES v2 API。凭证来自 AWS SDK 的默认凭证链（ECS 的任务角色、环境变量、共享配置文件），因此无需配置任何密钥。

1. 在 SES 中把 `mail_from` 的域名验证为身份（identity），并启用 Easy DKIM（见下文）。
2. 申请生产访问权限以脱离沙盒。在沙盒中只能发送到已验证的地址。
3. 为执行角色授予对 `mail_from` 的身份（如果使用配置集，还包括该配置集）的 `ses:SendEmail` 权限。

```sh
MY_MAIL=ses
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SES_REGION=ap-northeast-1          # the region where you verified the identity; AWS_REGION when empty
MY_MAIL_SES_CONFIGURATION_SET=my-events    # optional; a configuration set with event destinations
```

设置了 `mail_ses_configuration_set` 时，每封邮件都使用该配置集。SES 会用它自己的值覆盖邮件的 `Message-ID` 和 `Date`。
SES 从不在发送时同步拒绝收件人。它会接受发往账号抑制列表中地址的邮件，然后不投递，退信稍后以通知（SNS）的形式到达。
要记录退信和投诉，请按下文设置让 SES 通知通过 SNS 送达。

<!-- source: a-plugin-mail--plugin 65b46feaf1cf -->
### 插件（`mail = plugin`）

对于只提供 HTTP API 的邮件服务，运行一个种类为 `mailer` 的[插件](plugins.md)，由它调用该服务。

```sh
MY_MAIL=plugin
MY_MAIL_FROM=noreply@social.example
MY_PLUGIN_MAILER_ADDRESS=unix:///run/my/mailer.sock
```

使用这个值时，`plugin_mailer_address` 是必需的。消息中可能包含密钥，例如验证链接，因此该种类拒绝 `http://`，即使是回环地址也不行：请使用 Unix 套接字或 `https://`。服务器启动时不会调用插件，即使插件已停止也能启动，邮件会在其任务中等待。

使用 `plugin_mailer_protocol = json`（默认）时，插件只需要一个 HTTP 服务器和一个 JSON 库（[不使用库的插件](plugins.md#a-plugin-with-no-library)）。`Send` 最多持续 30 秒，或 `plugin_mailer_timeout` 指定的时长（小于 5 分钟）。请求最大 2 MiB，响应最大 64 KiB。

```text
POST /my.plugin.mailer.v1.MailerService/Send
Content-Type: application/json
Connect-Protocol-Version: 1

{
  "category": "CATEGORY_NOTIFICATION",
  "to": "alice@example.com",
  "envelopeFrom": "noreply@social.example",
  "fromName": "My Server",
  "replyTo": "admin@social.example",
  "subject": "You have a new follower",
  "text": "Bob followed you.\n",
  "html": "<p>Bob followed you.</p>",
  "headers": {
    "List-Unsubscribe": "<https://social.example/unsubscribe?token=abc>",
    "List-Unsubscribe-Post": "List-Unsubscribe=One-Click"
  }
}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_RECIPIENT_REJECTED","enhancedStatus":"5.1.1"}
```

服务器在调用插件之前会检查消息，因此无法通过这些字段注入邮件头。请求中包含消息的各个部分，由插件构建其服务所需的内容：

- **插件需要添加三个邮件头：** 每封消息唯一、位于 `envelopeFrom` 域名下的 `Message-ID`；`Date`；以及防止自动回复形成循环的 `Auto-Submitted: auto-generated`。如果服务会自行写入 `Message-ID` 和 `Date`，这两个就由服务负责。
- `From` 由 `fromName` 和 `envelopeFrom` 组成。`fromName`、`replyTo`、`html` 和 `headers` 可能缺失。`headers` 只包含两个 `List-Unsubscribe` 邮件头。
- 地址是 UTF-8。如果插件的服务需要 ASCII，插件要把域名转换为 Punycode，并对无法发送的本地部分回答 `OUTCOME_INVALID`。
- 同一封消息可能到达两次，因为失败的任务会重试。不要把消息写入日志。

`outcome` 表示这封消息的结果：

| `outcome` | 含义 | 服务器的处理 |
| --- | --- | --- |
| `OUTCOME_SENT` | 服务已接收这封消息 | 完成任务 |
| `OUTCOME_INVALID` | 消息按原样无法发送 | 立即使任务失败 |
| `OUTCOME_RECIPIENT_REJECTED` | 地址不存在 | 立即使任务失败，并[抑制该地址](#suppression-of-addresses-that-cannot-receive-email) |
| `OUTCOME_REJECTED` | 服务因其他原因永久拒绝 | 立即使任务失败 |
| `OUTCOME_TEMPORARY` | 服务暂时拒绝，或没有响应 | 重试任务 |
| 其他值，或没有值 | | 重试任务 |

HTTP 错误，或带有相应状态码的 Connect `{"code":"unavailable"}`，也属于临时失败。
`enhancedStatus` 是可选的：拒绝的增强状态码，例如 `5.1.1`。服务器会保留拒绝结果中的 5 类代码和 `OUTCOME_TEMPORARY` 中的 4 类代码，忽略其他代码。它不保留插件给出的任何文本，因此不要在那里放原因：原因应写在插件自己的日志里。

服务器只能通过这个响应得知退信。它没有用于接收插件服务后续退信和投诉的端点，因此服务事后报告的退信不会抑制该地址。

用 `my` 二进制文件测试插件：

```sh
my plugin test --kind mailer unix:///run/my/mailer.sock
my plugin test --kind mailer --mail-to you@example.com --mail-from noreply@social.example unix:///run/my/mailer.sock
```

第一条只检查插件是否响应，不发送任何邮件。第二条向 `--mail-to` 发送一封真实的邮件，并向插件发送一个没有收件人的请求，插件必须拒绝它。插件拿到消息后服务器就看不到它了，因此请查看实际收到的邮件：其邮件头中必须有 `Message-ID`、`Date` 和 `Auto-Submitted: auto-generated`。
参见[测试插件](plugins.md#test-a-plugin)。

<!-- source: receive-ses-notifications-sns e21b650ab5fe -->
### 接收 SES 通知（SNS）

SES 的退信和投诉会发送到一个 SNS 主题，该主题上的 HTTPS 订阅再把它们投递到服务器的 `POST /mail/events/ses`。
只有设置了 `mail_ses_sns_topic_arn` 时这个端点才会开放；否则它回答 `404`。

1. 在与 SES 身份相同的区域创建一个标准 SNS 主题。
2. 把主题属性 `SignatureVersion` 设为 `2`（SHA256withRSA）。使用默认值 `1` 的签名不被接受，服务器会回答 `400`。
3. 在主题的访问策略中，允许 `ses.amazonaws.com` 从你自己的账号执行 `SNS:Publish`（SES 控制台创建主题时会自动添加）。
4. 用以下任一方式把 SES 通知指向该主题（服务器会读取正文中的 `notificationType` 和 `eventType`）。
   - 身份通知：把这个主题用于 `mail_from` 身份的“Bounce”和“Complaint”反馈通知。
   - 配置集事件目标：把这个主题添加为 `mail_ses_configuration_set` 中配置集的目标，事件类型为 `BOUNCE` 和 `COMPLAINT`。
5. 把 `MY_MAIL_SES_SNS_TOPIC_ARN` 设为该主题的 ARN，用 `my config check` 检查，然后重启。
6. 在主题上创建一个订阅，协议为 `https`，端点为 `https://<server host>/mail/events/ses`。
   **不要启用原始消息传输**（没有签名的正文会被拒绝）。

```sh
aws sns create-topic --region ap-northeast-1 --name my-ses-feedback
aws sns set-topic-attributes --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --attribute-name SignatureVersion --attribute-value 2
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Bounce --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Complaint --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback

MY_MAIL_SES_SNS_TOPIC_ARN=arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback   # restart, then create the subscription below

aws sns subscribe --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --protocol https --notification-endpoint https://social.example/mail/events/ses
```

服务器收到订阅确认（`SubscriptionConfirmation`）时，会向其 `SubscribeURL` 发送 `GET` 以确认订阅，并记录日志 `SNS subscription of the SES notifications confirmed`。
用 `aws sns list-subscriptions-by-topic` 确认订阅不再是 `PendingConfirmation`。

在使用一条消息之前，端点会检查以下所有条件。任何一项不满足，它就回答 `400`，并且不记录任何内容。

- `TopicArn` 等于 `mail_ses_sns_topic_arn`。
- `SignatureVersion` 是 `2`，且签名有效。
- 签名的证书 URL（`SigningCertURL`）是 `https`，其主机是该主题所在区域的 `sns.<region>.amazonaws.com`。订阅确认的 `SubscribeURL` 也必须是同一主机。

证书通过防范 SSRF 的出站 HTTP 客户端获取，并在 KV 存储中缓存 24 小时。正文最多读取 1 MiB，更大的正文会得到 `413`。
无法获取证书、订阅确认失败或无法写入记录时，端点回答 `503`，交由 SNS 重试。维护模式期间它也回答 `503`。

以下是它会记录的通知。它会把收件人地址与主邮箱地址进行匹配（不区分大小写，Punycode 域名也能与其 Unicode 形式匹配）；如果没有匹配的地址，就什么也不做。

| 通知 | 记录为 |
| --- | --- |
| `bounceType` 为 `Permanent` 的退信（任何子类型，例如 `General`、`NoEmail`、`Suppressed` 或 `OnAccountSuppressionList`） | `hard_bounce` |
| 投诉（`complaintFeedbackType` 为 `not-spam` 的除外） | `complaint` |
| `Transient` 或 `Undetermined` 的退信，以及投递、发送等其他通知 | 不记录（回答 `200`） |

SES 的 `feedbackId` 是记录的唯一键，因此无论 SNS 把同一条通知发送多少次，都只有一条记录。

<!-- source: amazon-ses-smtp-interface f46a7a562966 -->
### Amazon SES（SMTP 接口）

无法提供 IAM 凭证时，例如在 AWS 之外运行，请通过 `mail = smtp` 使用 SES 的 SMTP 接口。在这种配置下，只有发送时对收件人的拒绝会被记录为抑制（[抑制无法接收邮件的地址](#suppression-of-addresses-that-cannot-receive-email)）。

1. 在 SES 中把 `mail_from` 的域名验证为身份（identity），并启用 Easy DKIM（见下文）。
2. 申请生产访问权限以脱离沙盒。在沙盒中只能发送到已验证的地址。
3. 在 SES 的“SMTP settings”中创建 SMTP 凭证（它与 IAM 访问密钥不同）。

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=email-smtp.ap-northeast-1.amazonaws.com   # the endpoint of your SES region
MY_MAIL_SMTP_PORT=587                                       # STARTTLS
MY_MAIL_SMTP_USERNAME=<user name of the SMTP credentials>
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

SES 会用它自己的值覆盖 `Message-ID` 和 `Date`。

<!-- source: a-general-smtp-submission-server 62921988a793 -->
### 通用 SMTP 提交服务器

适用于你自己的 SMTP 服务器（例如 Postfix），或邮件投递服务商的提交服务器。

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=smtp.provider.example     # a STARTTLS server whose certificate is valid for this name
MY_MAIL_SMTP_PORT=587
MY_MAIL_SMTP_USERNAME=<user name>           # empty for a server without authentication
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

如果对方服务器不提供 STARTTLS，或者其证书无法针对 `mail_smtp_host` 中的名称通过验证，服务器就不会向它发送。凭证只在 STARTTLS 之后发送。
修改设置后，用 `my config check` 检查，然后重启。

<!-- source: spf-dkim-and-dmarc d6b36a4f4d74 -->
## SPF、DKIM 和 DMARC

Gmail 和 Yahoo 要求每天发送大量邮件的发件人配置 SPF、DKIM、DMARC 和一键退订。即使发送量很小，没有这些配置的邮件也更容易被归类为垃圾邮件。
请在 `mail_from` 的域名（例如 `social.example`）的 DNS 中全部设置好，取值使用服务商提供的值。

- **SPF**：在域名上放一条 TXT 记录，授权你所使用的发送服务商（例如，如果你使用 SES 的自定义 MAIL FROM 域名，就在该域名上放服务商指定的 `include:`）。
- **DKIM**：把服务商签发的公钥放进 DNS，并在发送端启用签名。对于 SES，就是 Easy DKIM 的三条 CNAME 记录。服务器本身不签名，由服务商签名。
- **DMARC**：位于 `_dmarc.social.example` 的 TXT 记录。**先从 `p=none` 开始**，通过汇总报告（`rua=`）确认 SPF 和 DKIM 已对齐（与 From 的域名一致），再逐步收紧到 `quarantine` 和 `reject`。

```text
_dmarc.social.example.  TXT  "v=DMARC1; p=none; rua=mailto:dmarc@social.example"
```

要在 SES 上对齐 SPF，请设置自定义 MAIL FROM 域名（默认的 MAIL FROM 是 `amazonses.com`，与 From 的域名不一致）。只要 DKIM 对齐，DMARC 就能通过。

<!-- source: look-at-send-failures ab9c4f356d3b -->
## 查看发送失败

在任务页面（`/admin/jobs/list`，权限 `manage_jobs`；参见[任务队列与失败的任务](running.md#the-job-queue-and-failed-dead-jobs)）查看发送失败。
按任务类型筛选即可只看邮件任务（在管理 API 中为 `GET /api/v1/admin/jobs?type=identity.verification_email`）。
各类型见上表。

- **临时失败**（连接错误、超时、4xx 响应）会重试。事务性邮件尝试 6 次（从 30 秒开始指数退避，总共约 15 分钟）；通知邮件使用默认策略（10 次，约 4 小时）。尝试次数用完后，任务会变成失败的任务（dead job）。错误的 `mail_smtp_host`、认证失败和网络被阻断都会出现在这里。修复原因后，用 `my job retry <id>` 或在任务页面重新运行。
- **永久拒绝**（针对收件人或内容的 5xx）不会重试，立即失败。
- 使用 `mail = ses` 时，SES 的 `MessageRejected`（内容被拒绝）属于永久拒绝。`TooManyRequestsException`（发送速率限制）、`SendingPausedException`、`MailFromDomainNotVerifiedException`、`AccountSuspendedException` 以及未能到达 SES 的请求属于临时失败，修复账号或权限后重试即可成功。
  错误文本中只保留 SES 的错误代码。
- 使用 `mail = plugin` 时，由响应中的 [`outcome`](#a-plugin-mail--plugin) 决定。无法连接的插件或返回错误的插件属于临时失败，并计入 [`my.plugin.failures`](running.md)。错误文本中只保留结果和增强状态码，因此失败的原因要看插件的日志。
- 从请求起超过时限（重置为 15 分钟，验证为 1 小时）的事务性邮件会不发送而直接完成。这样可以避免旧链接迟迟才送达；这不算失败。
- 任务运行时如果已没有发送的理由（地址被抑制、期间已完成验证，或用户已失去该通知的权限或设置），它会不发送而直接完成。

任务载荷中不包含邮件地址、正文或令牌，错误文本中也不包含。只保留 ID、任务类型和 SMTP 响应码。
发送成功可以在每次运行的结构化 `job` 日志行（`my.job.type`）中跟踪。

<!-- source: suppression-of-addresses-that-cannot-receive-email 18b4750f1531 -->
## 抑制无法接收邮件的地址

当 SMTP 服务器表示收件人地址不存在（对 `RCPT TO` 返回扩展状态为 `5.1.x` 的 5xx）时，服务器会把该地址记录为硬退信，并停止向它发送。
使用 `mail = plugin` 时，`OUTCOME_RECIPIENT_REJECTED` 会记录同样的硬退信，其他结果都不会。
使用 `mail = smtp` 时，只记录发送时的这种拒绝。SMTP 服务器接收邮件之后才返回的异步退信，以及投诉反馈循环，都不会被接收。
使用 `mail = ses` 时，SES 从不同步拒绝收件人，因此发送时不会记录任何内容；永久退信和投诉根据 SES 通知记录（[接收 SES 通知（SNS）](#receive-ses-notifications-sns)）。

| 种类 | 被停止的邮件 |
| --- | --- |
| `hard_bounce` | 除验证邮件以外的所有邮件。密码重置邮件也会被停止 |
| `complaint`（被举报为垃圾邮件） | 通知邮件和公告邮件。事务性邮件仍会发送 |

- 审核员可以在管理界面中账号的“管理”页面看到抑制的种类和来源（`GET /api/v1/admin/accounts/{id}` 中的 `email_suppression`）。
- 有两种方式可以**解除**抑制。用户请求验证邮件并再次验证地址后，硬退信抑制会被解除（即使地址被抑制，验证邮件也会发送）。用户可以通过客户端 API（`POST /api/v1/accounts/email/verification`，每小时最多 3 次）或在登录状态页面请求。当你确认可以发送时，例如邮箱已修复，审核员可以用 `POST /api/v1/admin/accounts/{id}/email_suppression/lift`（权限 `manage_accounts`）解除抑制。这会在审计日志中记录 `email_suppression_lifted`（[审核](moderation.md)）。
- 对于密码重置邮件被停止的用户，管理员可以签发一个重置链接并交给对方（`my admin account password-reset-link @alice`）。
- 在设置页面上，用户可以看到邮件无法送达其主邮箱地址，以及从何时开始。

<!-- source: unsubscribing 2806dbccc409 -->
## 退订

通知邮件带有退订链接，以及邮件头 `List-Unsubscribe` 和 `List-Unsubscribe-Post:
List-Unsubscribe=One-Click`（RFC 8058）。邮件客户端的“退订”按钮会使用它们。

- 退订类别与通知设置中用户可以选择邮件的各行一一对应，例如 `mention` 或 `admin_report`（发给审核员的通知每个种类一个，来自审核员的通知为 `moderation`）。
- 链接是 `https://{host}/mail/unsubscribe?token=...`，无需登录。每封邮件签发一个令牌，数据库中只保存其哈希值，令牌在 90 天后过期。
  - `GET` 只显示确认页面，不做任何更改（这样预取链接的扫描器不会停止邮件）。页面上的按钮会发送同样的 `POST`。
  - `POST`（一键）关闭通知设置中那一行的邮件，不做其他任何事情。在令牌过期之前，再次发送会得到相同的结果。
- **事务性邮件无法退订。** 退订验证、重置和密码修改通知，会让与安全相关的通知无法送达。这些邮件也不带上述邮件头。
- 用户也可以在设置的 **通知** 页面切换同一个设置。

<!-- source: the-html-version 2ca008b1a48a -->
## HTML 版本

每封邮件都是 `multipart/alternative`，以 `text/plain` 和 `text/html`（均为 UTF-8）提供相同的内容。在 `development` 配置下使用 `mail = none` 时，只有文本版本会写入日志。

- 有一个统一的外层布局，包含段落、按钮和签名（服务器名称、URL、联系方式、发送这封邮件的原因，以及退订链接）。`<html lang dir>` 取自收件人的语言区域，`ar` 等语言会从右到左排版。
- 不加载任何外部字体或样式表。颜色使用默认的设计令牌，浅色和深色跟随 `color-scheme` 和 `prefers-color-scheme`。管理员对主题的覆盖设置不影响邮件。
- 不加载任何图片，但设置了实例自己的徽标时除外（[徽标和图标](instance-settings.md#logo-and-icon)）：此时 HTML 版本以徽标开头（`https://<host>/brand/...`，在跟随深色配色方案的邮件客户端中改为深色徽标），所有收件人使用同一个 URL，因此不会透露是谁打开了邮件。不加载外部图片的客户端会显示服务器名称（图片的替代文本）。没有徽标，或服务器 URL 不是 `https` 时，邮件中没有图片。
- 只有 `https` URL 会变成链接。当 URL 是 `http` 时，例如在 `web_origin` 为 `http://{host}` 的开发环境中，URL 会以文本形式写出，而不是按钮或退订链接，签名中的服务器 URL 也会省略。
- 布局内置于二进制文件中，因此服务器运行时不会执行任何 JavaScript。
