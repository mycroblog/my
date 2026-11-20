<!-- source: sending-email-through-amazon-ses cb9101de6cc9 -->
# 通过 Amazon SES 发送电子邮件

如何让 Amazon SES 做好发送服务器电子邮件的准备：用 DNS 记录验证发件人的域名、离开新账号起步时所处的沙盒，并让账号保持良好状态。OpenTofu 模块 [`mycroblog-aws`](../../deploy/opentofu/README.md#aws) 默认通过 SES 发送，并会创建身份和权限；这些记录以及生产访问权限的申请则是你自己要完成的步骤。设置本身见[发送电子邮件](email.md#amazon-ses-api)。

**AWS 部署从未被应用过**，所以这些步骤来自 AWS 的文档，尚未在本项目的服务器上完整走过一遍。

<!-- source: 1-add-the-records-of-the-domain b96b2365fdd4 -->
## 1. 添加域名的记录

SES 从已验证的身份发送邮件。`mycroblog-aws` 会在部署所在的区域，用 Easy DKIM 为 `mail.from` 的域名创建身份（对于 `noreply@social.example`，域名是 `social.example`）。应用之后，输出 `dns_records` 会列出需要在你的 DNS 提供商处添加的记录，其中包含 `host` 的记录：

```sh
tofu output dns_records
```

```text
abcdef...._domainkey.social.example.  CNAME  abcdef....dkim.amazonses.com
ghijkl...._domainkey.social.example.  CNAME  ghijkl....dkim.amazonses.com
mnopqr...._domainkey.social.example.  CNAME  mnopqr....dkim.amazonses.com
```

这三条 CNAME 记录发布 SES 用来签名的密钥。SES 会自行检查它们，一旦解析成功就把身份标记为已验证，通常在一小时内，最长不超过 72 小时；在此之前它拒绝发送。SES 控制台在 **Identities** 下显示身份的状态。

还要添加 SPF 和 DMARC 记录，接收方会与 DKIM 一并检查它们：[SPF、DKIM 和 DMARC](email.md#spf-dkim-and-dmarc) 说明了每条记录应填什么。由 SES 签名的 DKIM 与 `mail.from` 的域名对齐，因此只有这些记录，DMARC 就能通过；自定义 MAIL FROM 域名（它也会让 SPF 对齐）是可选的。

<!-- source: 2-leave-the-sandbox 7f27054422ae -->
## 2. 离开沙盒

每个新账号都从 SES 沙盒开始，每个区域分别如此。在沙盒中，SES 只会向你自己验证过的地址和域名发送，24 小时内最多 200 封，每秒一封。处于沙盒中的服务器可以给自己的运营者发送测试邮件，但其他任何人的注册都会失败。

要向所有人发送，请为部署所在的区域申请生产访问权限：

1. 打开该区域的 SES 控制台，在 **Account dashboard** 上选择 **Request production access**（或 **Get set up**）。
2. 邮件类型选择 **Transactional**，填写服务器的 URL，并描述它发送的内容：注册和新地址的确认邮件、密码重置，以及每位用户自己开启、并可通过一键退订关闭的通知。
3. 说明你如何处理退信和投诉：下面的通知到达之后，服务器会停止向发生退信或投诉的地址发送。

AWS 会通过电子邮件答复，通常在一天之内，授予访问权限之前可能还会要求补充信息。一旦授予，控制台会显示你的发送配额，它会随着你的使用而增长。

<!-- source: 3-watch-bounces-and-complaints b1182f32de88 -->
## 3. 留意退信和投诉

SES 会监控退信或遭到投诉的邮件所占的比例，对比例持续偏高的账号先进行审查，然后暂停。服务器通过 Amazon SNS 从 SES 的通知中得知退信或投诉，并停止向该地址发送。`mycroblog-aws` 不会创建这个主题：请按[接收 SES 通知](email.md#receive-ses-notifications-sns)所述设置，并通过模块的 `settings` 把它的 ARN 提供给服务器：

```hcl
  settings = { mail_ses_sns_topic_arn = "arn:aws:sns:ap-northeast-1:111122223333:my-ses-events" }
```

SES 控制台在 **Reputation metrics** 下显示账号的退信率和投诉率。

<!-- source: sending-from-another-region 61edc3b77ea9 -->
## 从其他区域发送

模块在部署所在的区域创建身份，服务器也从那里发送。如果要通过其他区域的 SES，或改用 SMTP 服务发送，请把 `mail = { kind = "smtp", ... }` 设为该服务的 SMTP 接口：设置见 [Amazon SES（SMTP 接口）](email.md#amazon-ses-smtp-interface)。
