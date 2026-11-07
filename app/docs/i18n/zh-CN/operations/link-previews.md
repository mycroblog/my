<!-- source: link-previews-ogp--x-card--fediversecreator 85d4b284637f -->
# 链接预览（OGP / X Card / fediverse:creator）

本页说明其他服务为本服务器的页面显示的预览、服务器为此返回的内容，以及手动检查的步骤。本服务器为自己帖子中的链接显示的卡片，见下文的[帖子中链接的卡片](#cards-of-links-in-posts)；如何停止从某个域名获取卡片，见[实例设置](instance-settings.md#link-card-domains)。

<!-- source: what-the-server-returns bb9326f9729a -->
## 服务器返回的内容

`my serve` 会把 meta 标签嵌入 Web 客户端的外壳，返回以下页面的 HTML：首页（`https://<host>/`）、实例信息页面（`https://<host>/about`）、已发布文档的页面（`/about/terms`、`/about/privacy`、`/about/commerce`）、链接卡片爬虫的说明页面（`/about/crawler`）、个人资料（`https://<host>/@alice`）和帖子（`https://<host>/@alice/status/<post ID>`）。对 HTML 的请求一律视为未登录，因此预览只显示未登录访客能看到的内容。

| 页面 | 响应 |
| --- | --- |
| 首页（`/`）和实例信息页面（`/about`） | 200。带有介绍实例的 meta 标签的外壳（见下文“首页”） |
| 链接卡片爬虫的说明页面（`/about/crawler`） | 200。带有介绍实例的 meta 标签的外壳，标题为“链接卡片爬虫 \| 实例名称” |
| 已发布文档的页面（`/about/terms`、`/about/privacy`、`/about/commerce`） | 200。带有介绍实例的 meta 标签的外壳，标题为“文档名称 \| 实例名称”。未发布的文档的页面得到通用外壳 |
| `public` 帖子、活跃的本站账号的个人资料 | 200。带有 meta 标签的外壳 |
| `unlisted` 帖子 | 200。带有 meta 标签的外壳，另加 `<meta name="robots" content="noindex, noarchive">` 和 `X-Robots-Tag: noindex, noarchive` |
| `local`、`private`、`direct`、已删除或不存在的帖子，以及被暂停、已停用、已删除或受限账号的个人资料和帖子 | 404。通用外壳和 `X-Robots-Tag: noindex, noarchive`。无论原因如何，正文和响应头都相同 |
| `/@<old username>/status/<post ID>` | 301，重定向到使用当前用户名的 URL |

每个页面还会根据实例设置和所有者的设置，带上对搜索引擎和 AI 爬虫的声明（`noindex`、`noai` 等 meta 标签，以及 `X-Robots-Tag`、`Content-Usage` 和 `tdm-reservation`）（[搜索引擎和 AI 爬虫](crawlers.md)）。上表中的 `noindex` 无论设置如何都会添加。`noindex` 不会阻止链接预览。

带有 ActivityPub `Accept` 的请求不会得到外壳，而是由 ActivityPub 一侧响应（个人资料以 actor 的 `id` 响应，未登录访客能看到的帖子以 302 重定向到它的 `id`）。每个响应都带有 `Vary: Accept`。

<!-- source: meta-tags 00a4a8115b9e -->
### meta 标签

| 标签 | 内容 |
| --- | --- |
| `og:title`、`<title>` | “昵称（@用户名@账号域名）”。昵称为空时使用用户名 |
| `og:type` | 个人资料为 `profile`，帖子为 `article` |
| `og:url`、`<link rel="canonical">` | 规范 URL（使用当前用户名的 URL） |
| `og:description` | 去掉标签的帖子正文纯文本（个人资料为简介），截断为 200 个字形。带有内容警告的帖子只使用内容警告文字 |
| `og:image`、`og:image:alt` / `width` / `height` | 按下面的顺序选出的图片 |
| `og:site_name` | 实例名称（文本 `name`，所有语言相同；没有时使用 `host`；[名称和域名](instance-settings.md#name-and-domain)） |
| `og:locale`、`<html lang dir>` | 帖子的语言。未设置（`und`）时使用作者的默认发帖语言，再没有则使用实例的 `default_locale`。它不随 `Accept-Language` 改变 |
| `article:published_time`、`article:author` | 仅帖子。帖子的时间（UTC）和作者的个人资料 URL |
| `profile:username` | 仅个人资料。`username@account domain` |
| `twitter:card` | 图片宽于约 2:1（宽 / 高 >= 1.7）且至少为 300x157 时为 `summary_large_image`，否则为 `summary` |
| `fediverse:creator` | `@username@account domain`。Mastodon 把它显示为“作者” |
| `<link rel="alternate" type="application/activity+json">` | AP `id`（actor 或帖子的对象） |

首页的 meta 标签介绍的是实例本身。

- `og:type` 为 `website`，`og:title`、`<title>` 和 `og:site_name` 为实例名称，`og:url` 和 `<link rel="canonical">` 为 `https://<host>/`。
- `og:description` 是截断为 200 个字形的实例文本 `short_description`。没有时省略。
- 语言（`og:locale`、`<html lang dir>`）为实例的 `default_locale`。名称和描述在所有语言中都是同一个值。
- `og:image` 为下面的第 3 项（实例预览图片），没有时为第 4 或第 5 项。
- 首页不属于任何账号，因此不输出 `fediverse:creator` 和 ActivityPub 的 `alternate`。
- 实例信息页面（`/about`）具有相同的 meta 标签。`og:title` 和 `<title>` 为“关于 {instance name}”（`server:preview.about_title`），`og:url` 和 `<link rel="canonical">` 为 `https://<host>/about`。
- 已发布文档的页面（`/about/terms`、`/about/privacy`、`/about/commerce`）也具有相同的 meta 标签。`og:title` 和 `<title>` 为“{document name} | {instance name}”（`server:preview.document_title`），`og:url` 和 `<link rel="canonical">` 为该页面的地址。未发布的文档的页面得到通用外壳。
- 链接卡片爬虫的说明页面（`/about/crawler`）也具有相同的 meta 标签，标题和地址的形式相同。

`og:image` 按以下顺序选择。

1. 帖子附带的第一张图片（其替代文本放入 `og:image:alt`）。被标记为敏感或带有内容警告的帖子不使用它。
2. 个人资料的封面图片，没有时使用头像（`og:image:alt` 为“{name} 的封面图片”或“{name} 的头像”）。
3. 实例预览图片（实例设置 `preview_image`；`og:image:alt` 为文本 `preview_image_alt`，没有时为“{instance name} 的图片”）。
4. 实例自身徽标的链接预览版本（[徽标和图标](instance-settings.md#logo-and-icon)），即位于 `https://<host>/brand/<revision>/preview-1200x630.png` 的 1200x630 PNG（`og:image:alt` 为“{instance name} 的图片”）。
5. 内置的默认图片（`https://<host>/preview-image.png`，1200x630 PNG。它是一张不含实例名称的通用图片，`og:image:alt` 描述这张图片）。

因此，显示预览的页面总是带有 `og:image` 和 `og:image:alt`。第 1 至 3 项的图片是处理后媒体的 `full` 变体，宽度和高度是处理得出的值。如果 `preview_image` 仍在处理中、处理失败或不是图片，则使用第 4 项（没有徽标时为第 5 项）。每次请求都会读取实例设置，因此 `my instance set preview_image ...` 无需重启即可生效（步骤见[实例设置](instance-settings.md)）。

默认图片在启动后的第一次请求时用标准库绘制，并保存在进程中（约 10 KB）。它以 `Cache-Control: no-cache` 和 ETag 提供。

<!-- source: caching 5bc906452b10 -->
### 缓存

- 响应带有 `Cache-Control: public, max-age=60, stale-while-revalidate=300`。删除、可见范围的更改，或文档的发布与取消发布，最多会在共享缓存中保留这么长时间。
- ETag 由模板、资源和实例设置的版本、语言、帖子的版本（revision）、实际生效的可见范围、个人资料的版本以及选中的图片构成（对首页、信息页面、文档页面和爬虫说明页面而言：页面的地址、标题、实例描述和选中的图片）。`If-None-Match` 会得到 `304`。
- 如果在前面放置 CDN，请把 `Accept` 包含在缓存键中（以免混淆 AP JSON 和 HTML）。

<!-- source: manual-check bf2c7a86a5de -->
## 手动检查

升级之后或更改预览设置之后，请在真实设备上检查每个服务。你需要一个可以从外部通过 HTTPS 访问的实例（预览机器人无法访问 `localhost`）。

<!-- source: preparation 9fd604dc1529 -->
### 准备

1. 用一个测试账号准备以下帖子。
   - 一条带有横向图片（例如 1200x630）和替代文本的 `public` 帖子
   - 一条不带图片的 `public` 帖子（来自设置了封面图片的账号）
   - 一条正文超过 200 个字符并包含表情符号（例如 👨‍👩‍👧‍👦）的 `public` 帖子
   - 一条 `unlisted` 帖子
   - 一条 `private` 帖子
   - 一条来自昵称包含 `"><script>alert(1)</script>` 的账号的帖子
2. 检查每个 URL 的 HTML。

   ```sh
   curl -sS -D - -H 'Accept: text/html' https://<host>/@alice/status/<post ID>
   ```

   - 状态为 200，且有上表中的 meta 标签。`unlisted` 帖子带有 `noindex`。
   - `private` 帖子和不存在的 ID 返回相同的 404 正文，并带有 `X-Robots-Tag: noindex, noarchive`（设置拒绝 AI 时还带有 `noai,
     noimageai`）。

     ```sh
     diff <(curl -sS https://<host>/@alice/status/<private post ID>) <(curl -sS https://<host>/@alice/status/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f)
     ```

   - 昵称中的 `<script>` 被转义为 `&lt;script&gt;`。

<!-- source: x 73a2dffdfdbb -->
### X

1. 把带图片的帖子的 URL 粘贴到发帖界面（或私信）中。
2. 检查大图卡片（`summary_large_image`）显示了标题、域名和图片。
3. 对于不带图片的帖子的 URL，检查卡片显示了封面图片。对于既没有封面图片也没有头像的账号，检查大卡片显示了默认图片（`preview_image`，未设置时为内置图片）。
4. 对于只有正方形头像的账号，检查卡片是小图卡片（`summary`）。

X 会缓存卡片。要检查更改，请粘贴带查询参数的 URL（例如 `?v=2`），让它被视为另一个 URL。

<!-- source: mastodon 02868aebd430 -->
### Mastodon

1. 用另一个实例上的账号发一条包含带图片帖子 URL 的帖子。
2. 检查帖子下方出现了带有标题、描述和图片的预览卡片。
3. 检查卡片下方把测试账号（`fediverse:creator`）显示为“作者”，并且可以进入它的个人资料。

Mastodon 会在每个获取过该卡片的服务器上缓存它。要重新生成，请使用新的 URL（带查询参数）发帖。

<!-- source: discord 4df04642bb64 -->
### Discord

1. 把带图片的帖子的 URL 粘贴到频道中。
2. 检查嵌入内容显示了网站名称（实例名称）、标题、描述和大图。
3. 对于超过 200 个字符的帖子，检查描述以 `…` 结尾，且没有表情符号被从中间截断。

<!-- source: slack 9f1e524968f3 -->
### Slack

1. 把带图片的帖子的 URL 粘贴到频道中。
2. 检查展开的预览显示了网站名称、标题、描述和图片。
3. 检查 `unlisted` 帖子的 URL 也会出现预览（`noindex` 针对的是搜索引擎，不会阻止预览）。

Slack 也会缓存预览。要检查更改，请在 URL 后加上查询参数。

<!-- source: line ef9e1e682603 -->
### LINE

1. 在聊天中发送带图片的帖子的 URL。
2. 检查出现了带有标题、描述和图片的预览。

LINE 也会缓存预览。要检查更改，请在 URL 后加上查询参数。

<!-- source: what-must-not-be-visible 187905fe778d -->
### 不得显示的内容

在每个服务上，检查 `private` 帖子的 URL 和不存在的帖子的 URL 都给出相同的通用预览（只有实例名称，或什么都没有），并且不显示任何正文、昵称或图片。

<!-- source: cards-of-links-in-posts 6394a6bf5641 -->
## 帖子中链接的卡片

为了给帖子中的链接显示卡片，本服务器的任务会获取被链接的页面及其图片，并遵守被链接网站对此的要求。它对页面、图片和 `robots.txt` 的请求使用 `my-linkcard/<version> (+https://<host>/)`。服务器的其他所有请求，包括媒体代理的请求，仍使用 `my/<version> (+https://<host>/)`。这些请求不发送 Cookie，也不发送 `Referer`，不会透露是哪个账号或哪条帖子链接了该页面。

<!-- source: the-page-for-site-operators 08a157cd9eac -->
### 面向网站运营者的页面

`https://<host>/about/crawler` 告诉被链接网站的运营者：爬虫请求什么、发送什么、频率如何、如何退出，以及他们可以请你停止从他们的域名获取内容。任何人无需账号即可阅读。实例信息页面（`/about`）的“搜索引擎和 AI”部分链接到该页面；`User-Agent` 中只写明 `https://<host>/`。

页面根据 `link_cards` 和 `link_cards_ignore_robots_txt` 说明本服务器的行为，并显示实例的联系方式（`contact_account`、`contact_email`）。请设置联系方式，让网站有地方可以提出请求：没有联系方式时，页面会说明本服务器没有公开任何联系方式。开启 `link_cards_ignore_robots_txt` 时，页面会说明网站无法通过 `robots.txt` 拒绝本服务器。

<!-- source: robotstxt e8d2deaef93d -->
### robots.txt

`robots.txt` 中使用的名称是 `my-linkcard`，不区分大小写。没有该名称的组时，按 RFC 9309 的规定适用 `*` 组。服务器读取 `Allow`、`Disallow`、`*` 和 `$`，以最长匹配为准，长度相同时 `Allow` 优先；不读取 `Crawl-delay` 和其他记录。`robots.txt` 最多在 10 秒内读取 512 KiB，文件更大时使用已读取的部分。服务器只按来源（scheme、主机和端口）保存适用于 `my-linkcard` 的规则。

| `robots.txt` 的响应 | 服务器的行为 | 保存时长 |
| --- | --- | --- |
| 200 | 遵守该文件 | 24 小时 |
| 429 以外的 4xx | 视为没有文件：获取 | 24 小时 |
| 429、5xx、网络故障，或服务器拒绝的重定向 | 不从该来源获取任何内容 | 1 小时 |
| 超过 5 次重定向 | 视为没有文件：获取 | 24 小时 |

- 无论帖子的可见范围如何，页面的 URL 都会在请求之前检查。它重定向到的每个 URL 也会按该 URL 来源的 `robots.txt` 检查；重定向到被拒绝的 URL 时就此终止：服务器不会请求它，也不会生成卡片。
- 被 `robots.txt` 禁止的 URL 与页面的其他失败一样，24 小时内不会再获取。如果只是无法读取该文件，服务器会在一小时后再次尝试该页面，直到达到任务的尝试次数上限。
- 图片也按其自身来源以同样方式检查。被禁止的图片会使卡片不带图片。如果只是无法读取该文件，之前记录的图片保留，新图片则在下一次重新获取之前不加入。
- 重新获取（帖子可能在上次获取 14 天后触发）时，如果 `robots.txt` 禁止该 URL，卡片会被移除。如果只是无法读取该文件，卡片保留。
- 服务器不会逐一重新检查已有的卡片，因此更改了规则的网站，会在各张卡片下一次重新获取时失去该卡片。

`link_cards_ignore_robots_txt`（[实例设置](instance-settings.md#settings-one-key-one-value)）会关闭上述全部行为：服务器既不获取也不使用 `robots.txt`。它仍然自称 `my-linkcard`，但被链接的网站将无法通过 `robots.txt` 拒绝这些获取：如[面向网站运营者的页面](#the-page-for-site-operators)所告知的，只剩下其页面中的指令，以及请你为其[域名设置规则](instance-settings.md#link-card-domains)的请求。

<!-- source: directives-of-a-page a854ef2ae8ea -->
### 页面的指令

服务器还会读取所获取页面的指令：`<head>` 中的 `<meta name="robots">` 和 `<meta name="my-linkcard">`，以及不带机器人名称或带有 `my-linkcard:` 的 `X-Robots-Tag`。针对其他机器人的指令不适用。有多条指令时，以更严格的为准。无论 `link_cards_ignore_robots_txt` 如何设置，这些指令都适用。

| 指令 | 卡片 |
| --- | --- |
| `nosnippet`、`max-snippet:0` | 不带描述 |
| `noimageindex`、`max-image-preview:none` | 不带图片，也不获取图片 |
| `noindex`、`nofollow`、`noarchive`、`none` 及其他 | 无变化 |

无论使用哪种指令，标题、网站名称和作者都会保留；不希望显示这些内容的网站，应在 `robots.txt` 中拒绝获取。指令要在获取页面之后才能知道，因此它们不会阻止对页面的请求。重新获取时适用页面当时的指令。

<!-- source: limits-per-host 41838af70d25 -->
### 每个主机的限制

无论 `link_cards_ignore_robots_txt` 如何设置，爬虫对每个主机都很克制。主机是 URL 中的名称，不含端口，因此被链接的网站和其图片所在的主机各有各的限制。

- 对同一主机一次只进行一次获取，下一次至少在上一次结束 1 秒后开始。一次获取是一个请求及其重定向，以及在需要读取时它所需的 `robots.txt`。页面和它的图片是两次获取，即使在同一主机上也是如此。
- `429` 或 `503` 会停止从作出该响应的主机获取，即使它响应的是重定向、图片或 `robots.txt` 也是如此。停止时长取其 `Retry-After`（秒数或日期），最短 1 秒，最长 1 小时。缺少或无法读取 `Retry-After` 时按 1 分钟计。
- 需要等待轮次的任务会稍后再回来。没有 worker 在等待，也不消耗任务的尝试次数，因此即使大量链接同时指向一个网站，最终也会生成所有卡片，没有失败的任务。同一主机的任务大约每秒唤醒一个。
- 主机让爬虫暂停的 URL 会保留其卡片，不会被记为失败，并在暂停结束时获取。如果一个 URL 在 24 小时内收到 5 次这样的响应，该页面与其他非 `200` 的页面一样算作失败，图片则不加入卡片。
- 等待图片的卡片会把从页面读取的内容保留最多 2 小时，以免再次请求该页面。
- [域名规则](instance-settings.md#link-card-domains)和域名处置优先：不从其获取任何内容的主机，不会有任何等待。

这些限制保存在键值存储中，并且都会过期：在获取过程中停止的 worker 会在 2 分钟内释放其主机。这些数值不是设置项。要查看限制的运作情况：

- `my.link_card.host_pauses` 按主机响应的状态统计暂停次数，每次暂停都会在 `INFO` 级别记录 `link card host paused`，带有 `host`、`status` 和 `pause_seconds`。
- 带有 `my.job.type="content.fetch_link_card"` 和 `my.outcome="postponed"` 的 `my.job.duration` 统计稍后再回来的运行次数。数值很大说明大量链接指向少数网站或暂停时间很长，并不是失败。
