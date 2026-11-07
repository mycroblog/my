<!-- source: search-engines-and-ai-crawlers 648c61a01042 -->
# 搜索引擎和 AI 爬虫

本文说明服务器向搜索引擎和 AI 爬虫声明了什么、哪些设置会改变这些声明，以及如何确认服务器实际返回的内容。

robots.txt 和 meta 标签只是一种声明，爬虫可能遵守，也可能不遵守，它们并不能在技术上阻止访问。如果需要阻止，请使用 WAF 或速率限制。

<!-- source: settings 13dfb1b04948 -->
## 设置

| 范围 | 设置 | 修改位置 | 未设置时 |
| --- | --- | --- | --- |
| 服务器 | `search_indexing`（`allow` / `deny`） | 管理后台服务器设置中“搜索与爬虫”页面的“搜索引擎与 AI”部分、`my instance set`、`PATCH /api/v1/admin/settings`（[实例设置](instance-settings.md)） | `deny` |
| 服务器 | `ai_usage`（`allow` / `deny_training` / `deny`） | 同上 | `deny` |
| 服务器 | `indexable_default`（新账号的 `indexable`） | 同上 | `false` |
| 服务器 | `security_contact`（报告漏洞的联系方式） | 同上 | 空（不提供 `security.txt`） |
| 个人 | `indexable`（出现在搜索结果中） | 个人资料设置中的“搜索引擎和 AI”、`PATCH /api/v1/accounts/update_credentials` | 注册时的 `indexable_default` |
| 个人 | `ai_opt_out`（拒绝 AI 训练和使用） | 同上 | `false` |

- 页面的声明取服务器设置和页面所有者设置中更严格的一方。个人不能允许超出服务器所允许的范围。
- 只有在 `search_indexing = allow` 的服务器上，`indexable = true` 的账号的个人资料和 `public` 帖子才会被告知搜索引擎可以收录。希望能被搜索到的服务器需要同时修改 `search_indexing` 和 `indexable_default`。在你修改 `indexable_default` 之前注册的用户，只有在自己选择 `indexable` 后才会被包含在内。
- 修改后的设置无需重启即可在 `/robots.txt`、页面和媒体响应中生效。

<!-- source: what-the-server-returns 31584e03601b -->
## 服务器返回的内容

| 响应 | 拒绝搜索时 | 拒绝 AI 时 |
| --- | --- | --- |
| 个人资料页面和 `public` 帖子页面 | 在 `<meta name="robots">` 和 `X-Robots-Tag` 中加入 `noindex, noarchive` | `noai, noimageai`、`Content-Usage: train-ai=n`（`deny` 时为 `train-ai=n, ai-use=n`）以及 `tdm-reservation: 1`（header 和 meta） |
| `unlisted` 帖子、隐藏或不存在页面的 404 页面、远程账号的页面、应用界面（`/home`、`/settings` 等） | 无论设置如何都为 `noindex, noarchive` | 遵循服务器设置 |
| `/`、`/about`、`/about/crawler` | 遵循服务器设置 | 遵循服务器设置 |
| `/media/`（由本服务器提供时） | 遵循服务器设置。没有按所有者的声明 | 遵循服务器设置。没有按所有者的声明 |
| 客户端 API、ActivityPub 文档、`/.well-known/`、`/nodeinfo/`、登录和 OAuth 界面 | `X-Robots-Tag: noindex` | 不添加（robots.txt 已足够） |

`noindex` 不会阻止链接预览（参见[链接预览](link-previews.md)）。

<!-- source: robotstxt 636c85dabf63 -->
### `/robots.txt`

`GET /robots.txt` 根据服务器设置和内置的 AI 爬虫列表生成（`text/plain; charset=utf-8`、`Cache-Control: public, max-age=3600`、ETag）。运营者无法添加自己的规则。

- 当 `ai_usage` 为 `deny_training` 或 `deny` 时，训练用爬虫（`GPTBot`、`ClaudeBot`、`CCBot`、`Google-Extended`、`Applebot-Extended` 等）会得到 `Disallow: /`。
- 当 `ai_usage` 为 `deny_training` 或 `deny` 时，带有 `Disallow: /` 的 `User-agent: __usage_ai-training` 组会对所有路径声明拒绝 AI 训练的偏好，无论由哪个爬虫抓取内容（Internet-Draft `draft-kelley-aipref-training-preference-signal`）。它指定的是一种用途而不是某个爬虫，因此不会阻止任何抓取。`ai_usage = allow` 时不输出该组，即不声明任何偏好，因为其中的 `Allow: /` 会对所有路径声明支持训练的偏好，其中也包括设置了 `ai_opt_out` 的用户的页面。
- 当 `ai_usage` 为 `deny` 时，AI 搜索、助手和代理的爬虫（`OAI-SearchBot`、`ChatGPT-User`、`Claude-User`、`PerplexityBot` 等）也会得到 `Disallow: /`。
- 对所有爬虫（`User-agent: *`）只拒绝 `/api/`、`/oauth/`、`/auth/`、`/ap/` 和 `/media/proxy/`，设置则通过 `Content-Usage`（IETF AI Preferences 草案）和
  `Content-Signal`（Content Signals）行来声明。拒绝搜索不会生成 `Disallow: /`：阻止抓取会让爬虫读不到 `noindex`，也会阻止遵守 robots.txt 的链接预览抓取程序。
- 不拒绝 `/.well-known/` 和 `/nodeinfo`，以便服务器列表和统计能够继续工作。
- 当 `account_domain` 与主机不同时，由主机一侧提供该文件。用户名所用域名上的 robots.txt 由运营者自行负责。

<!-- source: well-known 72d7e8cbbc33 -->
### `/.well-known/`

| 路径 | 提供条件 | 内容 |
| --- | --- | --- |
| `/.well-known/tdmrep.json` | `ai_usage` 不是 `allow` | `[{"location": "/*", "tdm-reservation": 1}]`（TDM Reservation Protocol）。`allow` 时返回 404 |
| `/.well-known/security.txt` | 设置了 `security_contact` | `Contact`、`Expires`（响应时间的 30 天后）、`Preferred-Languages`（`default_locale` 和 `en`）、`Canonical`（RFC 9116） |

<!-- source: information-page 0f89d6c3c3ce -->
### 信息页面

实例信息页面（`/about`）根据 `GET /api/v1/instance` 中的 `search_indexing` 和 `ai_usage`，显示是否允许搜索和 AI，以及该声明的含义。说明中将这些声明与日本《著作权法》第 30 条之 4（此类声明可能不被视为法律上的拒绝）和欧盟 DSM 指令第 4 条（以机器可读方式保留文本与数据挖掘的权利）相对照。它不保证任何法律效力。

<!-- source: this-servers-own-fetching 0cb9510e19e5 -->
## 本服务器自身的抓取

这些设置针对的是抓取本服务器的爬虫。本服务器也会以 `my-linkcard` 的身份抓取其他站点，用于生成帖子中链接的卡片：它遵守 `robots.txt` 和被链接页面的指令，并限制对每个主机的请求，详见[链接预览](link-previews.md#cards-of-links-in-posts)。该爬虫通过 `link_cards`、`link_cards_ignore_robots_txt` 设置，并可通过[链接卡片域名](instance-settings.md#link-card-domains)按域名设置。

这些站点的运营者可以在 `https://<host>/about/crawler` 了解它，`/about` 中有指向该页面的链接（[面向站点运营者的页面](link-previews.md#the-page-for-site-operators)）。开启 `link_cards_ignore_robots_txt` 时，他们无法通过 `robots.txt` 选择退出，该页面也会说明这一点：他们只能用自己页面的指令限制卡片，或请你为他们的域名设置规则。

<!-- source: limits 93da7e422ee8 -->
## 限制

- 个人的 `ai_opt_out` 只在本服务器提供的页面上声明。媒体的键中不包含所有者，而且有些部署会让 CDN 直接提供媒体，因此媒体只带有服务器整体的声明。其他服务器提供的页面上的声明由那些服务器负责（不发送自定义的 ActivityPub 扩展）。
- `ai_usage = deny` 还声明拒绝这样的行为：ChatGPT 或类似服务的用户让该服务读取本服务器的页面。
- 远程账号的 `indexable`（Mastodon 的 `toot:indexable`）会被接收并保存。远程账号的页面在本服务器上始终为 `noindex`。
- 在配有搜索引擎的服务器上，`indexable` 还决定个人的帖子是否出现在本服务器自己的[全文搜索](search.md)中。在那里它覆盖 `search_visibilities` 中的可见范围，其中可以包括 `local`。`search_indexing` 对该搜索没有影响，而向搜索引擎声明的内容仍如本页所述。

<!-- source: serving-media-from-object-storage-or-a-cdn 98c7ab4994ab -->
## 由对象存储或 CDN 提供媒体

当对象存储或 CDN 直接提供媒体时，`/media/` 的响应不经过本服务器，也不带有上表中的 header。服务器整体的声明仍然通过 `/robots.txt` 适用于所有内容；如果还想添加这些 header，请在 CDN 的响应 header 规则中为 `/media/*` 添加以下内容（`ai_usage = deny` 的示例）。

```text
X-Robots-Tag: noindex, noarchive, noai, noimageai
Content-Usage: train-ai=n, ai-use=n
tdm-reservation: 1
```

修改设置时，请相应地修改 CDN 规则。`/media/proxy/` 由本服务器提供，因此不需要规则。

<!-- source: the-ai-crawler-list 675a37cb0caf -->
## AI 爬虫列表

该列表由 [ai-robots-txt/ai.robots.txt](https://github.com/ai-robots-txt/ai.robots.txt)（MIT）的 `robots.json` 生成，并内置在二进制文件中。它不会在运行时获取，因此更新的列表随升级一起提供。

<!-- source: verify 1633f03abedb -->
## 确认

```sh
curl -sS https://<host>/robots.txt
curl -sS -o /dev/null -D - -H 'Accept: text/html' https://<host>/@alice | grep -iE 'x-robots-tag|content-usage|tdm-reservation'
curl -sS https://<host>/.well-known/tdmrep.json
curl -sS https://<host>/.well-known/security.txt
```

- 在未做任何设置的服务器上，`/robots.txt` 中有 `User-agent: GPTBot`、`User-agent: __usage_ai-training` 和 `Content-Usage: search=n, train-ai=n, ai-use=n`，个人资料带有 `X-Robots-Tag: noindex, noarchive, noai, noimageai`。
- 在 `search_indexing = allow` 且 `ai_usage = allow` 时，选择了 `indexable` 的账号的个人资料不带 `X-Robots-Tag`，`/.well-known/tdmrep.json` 返回 404。
