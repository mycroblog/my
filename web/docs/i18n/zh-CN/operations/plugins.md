<!-- source: plugins 4a60f7abbec9 -->
# 插件

插件是由你运行的独立程序或服务，服务器通过网络把某一项范围很窄的工作交给它。服务器只负责连接一个地址：它不会启动、停止或监视插件。你可以把插件作为 sidecar 运行、在 systemd 下运行，或者在另一台机器上运行。

默认情况下不委托任何工作。没有设置插件地址的服务器不会建立任何连接，也不会启动任何进程。构建标签 [`no_plugins`](../reference/build-tags.md) 会把插件客户端从二进制文件中去掉。

![插件所处的位置：my 进程把六个端口交给插件，并询问五个决策钩子，所有调用都经过同一个插件客户端，由它连接你运行的插件；图的底部显示每种调用失败时会发生什么。下面的列表逐一说明各个部分。](images/plugins.svg)

从图的上方往下：

- **my 进程**（任何 `role`）会对插件进行两类调用。
  - **交出的端口**：插件代替某个内置部分完成一项工作，每一项只在某个设置指定 `plugin` 时才启用：`translator`（实例设置 `translation_providers = plugin`）、`billing`（`billing_sources` 包含 `plugin`）、`mailer`（`mail = plugin`）、`media_image`、`media_video` 和 `media_audio`（`media_image = plugin` 等）、`key_manager`（`key_manager = plugin`），以及 `search`（`search = plugin`）。
  - **[决策钩子](decision-hooks.md)**：插件回答是否接受某样东西；除非该钩子的模式 `hook_<hook>_mode` 为 `off`，否则都会询问插件：`inbound.status` 用于来自其他服务器的帖子，`inbound.follow` 用于来自其他服务器的关注，`signup.check` 用于注册，`media.check` 用于上传，`local.status` 用于本站账号的帖子，`inbound.actor` 用于其他服务器上账号的个人资料，`local.profile` 用于本站账号的个人资料更改。
- **插件客户端**负责所有调用。每一类调用有一个目标地址 `plugin_<kind>_address` 和一个截止时间 `plugin_<kind>_timeout`，同时最多运行 8 个调用（每种媒体类别为 2 个）。它通过 Unix 套接字、`https://`，或者（仅限[部分类别](#plain-tcp)）`http://` 进行连接。
- **你的插件**是由你运行的独立程序，可以作为 sidecar、在 systemd 下或在另一台机器上运行。服务器只负责连接它们。
- **当调用失败或超时**（[详情](#when-a-call-fails)）：
  - `billing`、`mailer` 和各媒体类别：发起调用的任务稍后会再次运行。
  - `translator`：服务器会询问下一个服务商；如果已经没有服务商，读者会看到翻译失败。
  - `key_manager` 采取失败即关闭的方式：所有需要密钥的操作都会失败，没有插件时服务器不会启动。
  - `search`：写入的任务稍后会再次运行，而搜索会失败，所以用户会看到搜索不可用。
  - 决策钩子按 `hook_<hook>_on_failure` 的设置执行：`pass`、`retry`，或者（对于注册）`require_approval`。

<!-- source: kinds 73da92ef7563 -->
## 类别

每个类别都有自己的目标地址，使用 `plugin_<kind>_*` 设置。类别名称中的点在键中变为 `_`，所以 `inbound.status` 对应 `plugin_inbound_status_address`。

| 类别 | 传输的内容 | 普通 TCP（`http://`） | 默认协议 | 默认截止时间 |
| --- | --- | --- | --- | --- |
| `translator` | 读者要求翻译的帖子文本 | **拒绝** | `json` | 10 秒 |
| `search` | `public` 和 `local` 帖子的文本、作者、可见范围和语言，以及已登录用户的查询 | **拒绝** | `json` | 10 秒 |
| `billing` | 实例的费用 | 允许 | `json` | 2 分钟 |
| `mailer` | 地址，以及确认链接等机密 | **拒绝** | `json` | 30 秒 |
| `media_image` | 上传的图片及其元数据，元数据中可能含有位置 | 允许 | `proto` | 4 分钟 |
| `media_video` | 上传的视频及其元数据，元数据中可能含有位置 | 允许 | `proto` | 4 分钟 |
| `media_audio` | 上传的音频文件及其元数据 | 允许 | `proto` | 4 分钟 |
| `key_manager` | 明文的数据加密密钥 | **拒绝** | `json` | 5 秒 |
| `inbound.status` | 收到的帖子文本 | **拒绝** | `json` | 3 秒 |
| `local.status` | 本站帖子的文本 | **拒绝** | `json` | 1 秒 |
| `local.profile` | 本站账号的公开个人资料，不含图片 | 允许 | `json` | 1 秒 |
| `inbound.follow` | 关注的双方 | 允许 | `json` | 3 秒 |
| `inbound.actor` | 其他服务器上账号的公开个人资料 | 允许 | `json` | 3 秒 |
| `signup.check` | 电子邮件地址和 IP 地址 | **拒绝** | `json` | 2 秒 |
| `media.check` | 媒体经过处理的小版本，绝不是原件 | 允许 | `json` | 10 秒 |

让服务器使用某个目标地址的设置属于各自的功能：表中最后五个类别是[决策钩子](decision-hooks.md)。本页介绍它们的共同之处。

每条消息中，服务器最多发送该类别固定上限的数据量，最多读取另一个上限的数据量。超过上限的调用会失败，发起调用的任务会重试。

<!-- source: the-destination f397f6e80550 -->
## 目标地址

这些设置只存在于配置文件和环境变量中（[配置参考](../reference/configuration.md)）。没有任何实例设置、管理页面或 API 能显示或更改它们，所以被盗的管理员账号无法重定向帖子或密钥。更改在重启后生效。

`plugin_<kind>_address` 有三种形式。服务器在启动时检查形式，但不解析名称；每次连接时才解析名称。

- `unix:///run/my/mailer.sock`：Unix 套接字。
- `https://mailer.internal:8443`：基于 TCP 的 TLS。服务器总是依据 `plugin_<kind>_tls_ca_file` 或系统的根证书验证证书，没有任何设置可以关闭验证。
- `http://billing.internal:5000`：普通 TCP。

<!-- source: authentication 2fe4df9369ac -->
### 身份验证

选择其中一种：

- **Unix 套接字**：由套接字文件的权限保护。
- **mTLS**：`plugin_<kind>_tls_cert_file` 和 `plugin_<kind>_tls_key_file`，配合 `https://` 地址使用。
- **Bearer 令牌**：`plugin_<kind>_token`（或 `MY_PLUGIN_<KIND>_TOKEN_FILE`），以 `Authorization: Bearer` 发送，只能通过 `https://` 或 Unix 套接字使用。令牌配合 `http://` 地址属于配置错误。

既没有令牌也没有客户端证书的 `https://` 地址在启动时会给出警告，因为插件无法把服务器与其他能访问它的人区分开来。

服务器在启动时读取 `plugin_<kind>_tls_ca_file`、`plugin_<kind>_tls_cert_file` 和 `plugin_<kind>_tls_key_file` 指定的文件，文件缺失或无法读取时会停止。此后，每当打开新连接时，它最多每 5 秒检查一次这些文件是否有变化，所以由 Vault、cert-manager 或 SPIFFE 等证书管理器原地续期的短期证书或 CA 无需重启即可使用。已经打开的连接会一直使用建立时的证书，直到关闭。续期只写了一半时（例如新证书已经写入，但对应的私钥还没有到位），服务器会继续使用上次读取的文件，并在下一次连接时重试。令牌只在启动时读取。

<!-- source: plain-tcp cbb88078173e -->
### 普通 TCP

`mailer`、`key_manager`、`translator`、`search`、`inbound.status`、`local.status` 和 `signup.check` 拒绝 `http://`：服务器不会启动。回环地址（`http://127.0.0.1`）也是如此，因为回环地址无法阻止主机上的其他进程读取流量。请把这类 sidecar 放在共享卷中的 Unix 套接字上。

其他类别接受 `http://`，服务器在启动时会记录一条写明类别和地址的警告。这些流量既没有加密也没有身份验证，也不能使用 Bearer 令牌。保持该网络封闭是你的责任：上表列出的内容会以明文穿过它。

<!-- source: protocols eb5a03718f1b -->
## 协议

`plugin_<kind>_protocol` 指定服务器发送什么，它必须与插件接受的协议一致。

| 值 | 服务器发送 | 适用于 |
| --- | --- | --- |
| `json` | Connect 的一元协议，使用 `application/json`，在普通连接上走 HTTP/1.1 | 基于 Connect 库构建的插件，或者仅用 HTTP 服务器和 JSON 库构建的插件 |
| `proto` | Connect 的协议，使用 `application/proto` | 基于 Connect 库构建的插件 |
| `grpc` | 基于 HTTP/2 的 gRPC | 只支持 gRPC 的插件 |

插件收到错误的协议时会返回 `415`。媒体类别是双向流式传输，需要 HTTP/2，所以它们使用 `proto` 或 `grpc`，使用 `json` 属于配置错误。

协议的定义是代码仓库 `api/plugin/` 下的 `.proto` 文件。每一类端口有一个包，它们之间不共享类型；所有决策钩子共享一个包 `my.plugin.hook.v1`。[插件协议](../plugins/overview.md)中有每个包的参考文档，由这些文件生成。

<!-- source: when-a-call-fails 1caa41ab429e -->
## 当调用失败时

调用会在同一类别中同时运行的调用之间排队等候，等待时间计入它的截止时间，也计入指标 [`my.plugin.slot_wait`](running.md)。失败的调用按类别和 Connect 代码计入指标 [`my.plugin.failures`](running.md)。服务器会丢弃插件放在错误中的消息，所以插件失败的原因要看插件自己的日志。服务器本身不会重试调用：由发起调用的任务重试，所以插件必须能应对同一个调用到达两次。

服务器启动时插件没有运行，并不会让任何东西停止，`key_manager` 除外：没有它服务器不会启动，而[没有它时会停止什么](key-management.md#when-the-plugin-is-unreachable)是设置它时首先要阅读的内容。

调用的截止时间是 `plugin_<kind>_timeout`，插件以 `Connect-Timeout-Ms`（gRPC 上为 `grpc-timeout`）的形式收到它。插件可以忽略这个请求头：服务器到时仍会停止等待。对于请求可能需要等待的类别（`key_manager`、`translator`、`search`、`signup.check`、`media.check`、`local.status`、`inbound.actor` 和 `local.profile`），截止时间必须小于 30 秒；对于其他类别，必须小于任务可运行的时长 5 分钟。

<!-- source: test-a-plugin 5d8195bb27f5 -->
## 测试插件

`my plugin test` 针对正在运行的插件执行某个类别的契约，并报告每个用例的结果。它不读取任何配置，所以只需要 `my` 二进制文件。

```sh
my plugin test --kind probe unix:///run/my/plugin.sock
my plugin test --kind probe --protocol grpc --tls-ca-file ca.pem --token-file token https://plugin.internal:8443
```

```text
PASS reachable
PASS echo
FAIL echo_empty: the outcome is OUTCOME_UNSPECIFIED, not OUTCOME_OK
2 passed, 1 failed, 0 skipped
```

有用例失败时，退出码为 1。各个标志的含义与对应设置相同：`--protocol`、`--token-file`、`--tls-ca-file`、`--tls-cert-file`、`--tls-key-file` 和 `--timeout`。它采用与服务器相同的规则，所以 `--kind mailer` 会拒绝 `http://` 地址。被测试的插件可能是真实在用的插件，所以除调用本身之外还会产生其他影响的用例，只有在某个标志要求时才会运行。如果当前版本还没有某个类别的契约，会报告 `reachable` 和一个 `SKIP`。

`--kind billing` 只检查在不允许任何计费请求时，插件是否拒绝产生任何花费。`--billed` 还会真正获取本月和上个月的数据，并像导入时那样检查回答；这可能会在插件背后的服务商处产生费用。[导入云使用费用](billing-import.md#a-plugin-billing_sources--plugin)中有该类别的契约。

除非 `--mail-to` 指定了收件人，否则 `--kind mailer` 不会调用 RPC。指定后，该命令会从 `--mail-from`（默认与收件地址相同）向该地址发送一封真实的邮件，并检查插件是否拒绝没有收件人的请求。[发送电子邮件](email.md#a-plugin-mail--plugin)中有该类别的契约。

`--kind translator` 会发送几句固定的英文句子，把它们翻译成日文（对于只支持部分语言的插件，可以用 `--translate-from` 和 `--translate-to` 选择另一对语言）。插件背后按字符计费的服务也会对这些内容计费，大约 200 个字符。它检查回答的格式，以及插件是否拒绝语言 `qaa`（ISO 639 留作私用的代码）。它无法评判译文的用词，所以请自己阅读插件的译文。[翻译帖子](#translating-posts)中有该类别的契约。

`--kind search` 会进行搜索，并且只写入和删除它自己的文档：几篇使用任何帖子都没有的 ID、作者不是任何账号、含有一个为本次运行生成的词的文档，结束时会删除它们。它检查不含任何词的查询找不到任何结果，使用某个引擎的查询语言或日文的查询不会失败，插件拒绝不是它发出的游标，以及文档按从新到旧的顺序逐页找到、能被替换，并能按帖子和按作者删除。`--clear-index` 还会运行删除某个可见范围的全部文档和删除整个索引的用例：只能对测试用的索引使用它。[搜索帖子](#searching-posts)中有该类别的契约。

`--kind media_image`、`--kind media_video` 和 `--kind media_audio` 只发送它们自己生成的小测试文件：一张照片、一个表情符号和一个动画 GIF，一个带视频轨和音轨的 MP4，或者一个 MP3。每个文件都在元数据中藏有位置或标题，如果它出现在输出中，用例就会失败。它们像服务器那样检查每一个输出。[转换媒体](#converting-media)中有这些类别的契约。

`--kind key_manager` 会运行服务器每次启动时运行的检查，并检查插件是否拒绝被改动的值和它不持有的 KEK。它只发送自己临时生成、用后即弃的密钥，所以对真实的密钥存储使用也是安全的，但密钥存储可能会对这些调用计费。它检查回答的格式是否正确，而不检查插件如何保管它的 KEK。[管理密钥](key-management.md#a-plugin)中有该类别的契约。

`probe` 是最小的类别，服务器不会委托它做任何事。先实现它，在编写插件的其余部分之前检查传输、协议和身份验证。

<!-- source: translating-posts c9d31b2b9157 -->
## 翻译帖子

`translator` 类别的插件让服务器在读者要求时，通过你选择的、[内置服务商](translation.md)（LibreTranslate、DeepL、Google Cloud Translation 和 Amazon Translate）以外的服务翻译帖子。没有服务器端服务商时，唯一的翻译是读者浏览器内置的翻译，而它只存在于桌面版 Chrome、Edge 和 Opera 中：在你启用一个服务商之前，使用手机、Firefox 或 Safari 的读者不会看到翻译按钮。

```sh
MY_PLUGIN_TRANSLATOR_ADDRESS=unix:///run/my/translator.sock
MY_PLUGIN_TRANSLATOR_NAME="Our translator"    # what readers see next to a translation
```

1. 设置 `plugin_translator_address` 和 `plugin_translator_name`，然后重启。名称是必需的，因为读者是根据译文的来源来决定是否信任它的。地址绝不会离开配置。
2. [更新你的隐私政策](privacy-policy-template.md#the-services-you-choose)，把插件背后的服务列为翻译服务：插件转发的就是它收到的内容。
3. 用[实例设置](instance-settings.md) `translation_providers = plugin` 启用它。更改立即生效。

仅仅设置了地址并不会启用任何功能：在该设置指定 `plugin` 之前，服务器不会发送任何内容，读者也看不到按钮。

该插件和其他服务商一样，只是一个服务商。[`translation_provider_visibilities`](instance-settings.md) 决定它可以接收什么，每月限额统计发送给它的内容，它生成的译文也像其他译文一样保存在服务器的缓存中。开启 `translation_reports_to_server` 时，它还会按照相同的可见范围，接收审核员要求翻译的举报内容。在 `translation_providers` 的顺序中，它可以排在其他服务商之前或之后。

<!-- source: what-the-plugin-receives 975c7e56a522 -->
### 插件接收的内容

当读者要求翻译时，插件接收帖子的文本：正文、内容警告、投票选项和图片描述，以及文本的语言和要翻译成的语言。它不会收到帖子的地址、作者、读者、提及、话题标签、链接、自定义表情符号或代码：这些内容由编号标记代替。

默认情况下，这包括 `public`、`local` 和 `unlisted` 帖子。**只有在你允许时，`private` 和 `direct` 帖子才会发送到插件**，方法是设置 `translation_provider_visibilities = plugin=public|local|unlisted|private|direct`。这样，插件及其背后的任何服务都会读到作者只想给少数人看的文本，所以只有在你能控制文本去向时才允许它们，并在隐私政策中写明。服务器无法撤回已经发送的文本。

服务器对此类别拒绝 `http://`，回环地址也一样，因为传输的是帖子的文本。请使用 Unix 套接字或 `https://`。

<!-- source: an-llm-behind-a-plugin 611b6fc77bb1 -->
### 插件背后的 LLM

服务器有意不内置也不提供任何 LLM 服务商。任何人都可以发帖，所以要翻译的文本可以成为向 LLM 下达指令的途径（提示注入），而读者会把译文当作帖子的内容。净化保护的是页面的 HTML：它无法阻止译文说出帖子没有说的话。

服务器看不到插件调用了什么。**如果你的插件使用 LLM，这种风险及其后果由你自己负责。**

<!-- source: the-contract ef518f876dd2 -->
### 契约

RPC 是 `my.plugin.translator.v1.TranslatorService` 的 `Translate`：

```text
POST /my.plugin.translator.v1.TranslatorService/Translate
Content-Type: application/json

{"fragments":["Good morning ⟦1⟧, this is ⟦2⟧important⟦/2⟧.","⟦3⟧"],"sourceLanguage":"en","targetLanguage":"ja"}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","translations":["⟦1⟧さん、おはようございます。これは⟦2⟧重要⟦/2⟧です。","⟦3⟧"],"detectedLanguage":"en"}
```

- `sourceLanguage` 是 BCP 47 标签；服务器不知道语言时为 `und`。这时插件检测语言并在 `detectedLanguage` 中报告，或者回答 `OUTCOME_UNSUPPORTED_LANGUAGE`。插件没有可补充的信息时，`detectedLanguage` 可以省略；不是语言标签的值会被服务器忽略。
- `translations` 中每个片段对应一项，顺序与片段相同，且都不能为空。片段可以只由标记组成，这时它的译文就是这些标记。
- `⟦1⟧` 代表一个项目，或者开始一对由 `⟦/1⟧` 结束的标记。片段中的每个标记都必须在译文中原样出现且只出现一次。项目可以按语序需要移动位置，但一对标记必须先开始再结束，并且成对的标记可以嵌套。
- 译文是纯文本。服务器会转义其中的任何标记语言，并在保存或显示之前净化结果。

| `outcome` | 含义 | 服务器的处理 |
| --- | --- | --- |
| `OUTCOME_OK` | 响应中包含译文 | 检查、保存译文，并回应读者 |
| `OUTCOME_TEMPORARY` | 服务失败或没有响应 | 尝试下一个服务商 |
| `OUTCOME_PROVIDER_LIMIT` | 服务报告触及了它自己的速率限制或配额 | 尝试下一个服务商 |
| `OUTCOME_UNSUPPORTED_LANGUAGE` | 服务不支持源语言或目标语言，或者无法检测语言 | 尝试下一个服务商；如果都不支持，告知读者 |

违反这些规则的回答按 `OUTCOME_TEMPORARY` 处理，其中任何内容都不会保存，缺失或未知的 `outcome` 以及失败的调用也一样。当没有服务商作出回答时，读者会看到翻译失败，帖子保持原样；但如果开启了 `translation_browser`，且读者的浏览器能够翻译该帖子，Web 客户端就会在浏览器中翻译它。每次调用的字符都会计入每月限额，失败的调用也一样，因为服务可能已经收到了这些字符。

服务器在 `plugin_translator_timeout` 之后停止等待，无论该设置是多少，最多等待 20 秒。同一个请求的所有服务商总共有 45 秒。服务器不会重试失败的调用，同样的片段可能会因为另一位读者而再次发送。

<!-- source: searching-posts bd9dfba5a3e7 -->
## 搜索帖子

`search` 类别的插件把[帖子全文搜索](search.md)的索引保存在服务器没有内置支持的引擎中，例如 Groonga 或 PGroonga、用 Sudachi 构建的索引、Typesense、Manticore，或者托管的搜索服务。对服务器来说，它只是又一个引擎：相同的帖子进入索引，相同的任务保持索引最新，而且仍由服务器决定每个人可以看到什么。

```sh
MY_SEARCH=plugin
MY_PLUGIN_SEARCH_ADDRESS=unix:///run/my/search.sock
```

1. 运行插件，并用 [`my plugin test --kind search`](#test-a-plugin) 检查它。
2. 设置 `search = plugin` 和 `plugin_search_address`，然后像对待任何引擎一样按照[启用搜索](search.md#turning-it-on)操作：索引从此开始填充，`my search rebuild` 会加入已有的帖子。
3. 如果插件把帖子发送给其他公司的服务，请在你的[隐私文件](search.md#your-privacy-documents)中写明该服务。

服务器在启动时不会建立连接，所以插件没有运行时，除了搜索之外什么都不会停止。要切换到插件或从插件切换出去，请按照[关闭或更换引擎](search.md#switching-off-or-changing-the-engine)操作。

<!-- source: what-the-plugin-receives-1 d2b09f214f3a -->
### 插件接收的内容

- **索引中的帖子**。对于索引中的每篇帖子，插件接收它的文本（正文、内容警告和图片描述，均为纯文本）、ID、作者的账号 ID、可见范围、语言和发布时间。按照 [`search_visibilities`](instance-settings.md) 的规定，这些只是允许被搜索的作者的 `public` 和 `local` 帖子。`local` 帖子只面向本服务器的账号：插件及其背后的任何服务也会读到它们。`unlisted`、`private` 和 `direct` 帖子绝不会发送到插件。
- **查询**。已登录的用户搜索帖子时，插件接收他们输入的内容，以及页面的游标和结果数量。它不会收到是谁在搜索。
- **删除**：删除一篇帖子、删除某个账号的所有帖子、删除某个可见范围的所有帖子，以及删除整个索引。

插件只返回帖子 ID。服务器会从自己的数据库中重新读取每篇帖子，并只显示用户当前可以看到的内容，所以插件无法把帖子显示给无权查看的人，而落后的索引只会找到更少的结果。服务器也从不向任何人显示插件的错误。

服务器对此类别拒绝 `http://`，回环地址也一样，因为它传输的是帖子的文本和人们搜索的内容。请使用 Unix 套接字或 `https://`。

<!-- source: the-contract-1 d500365750c4 -->
### 契约

服务是 `my.plugin.search.v1.SearchService`，每种操作对应一个 RPC：

| RPC | 作用 |
| --- | --- |
| `Replace` | 添加帖子的文档，或替换具有相同 `statusId` 的文档 |
| `Remove` | 删除帖子的文档 |
| `RemoveByAuthor` | 删除某个账号的所有文档 |
| `RemoveByVisibility` | 删除某个可见范围（`VISIBILITY_PUBLIC` 或 `VISIBILITY_LOCAL`）的所有文档 |
| `Clear` | 删除所有文档；之后索引仍可再接收文档 |
| `Search` | 返回查询找到的文档 ID 中的一页，按从新到旧排序 |

```text
POST /my.plugin.search.v1.SearchService/Replace
Content-Type: application/json

{"document":{"statusId":"019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07","authorId":"0199c2d4-1e5f-7a3b-8c9d-0e1f2a3b4c5d","visibility":"VISIBILITY_PUBLIC","language":"ja","publishedAt":"2026-11-07T05:12:30Z","text":"週末に図書館で全文検索の本を読んだ"}}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK"}
```

```text
POST /my.plugin.search.v1.SearchService/Search
Content-Type: application/json

{"text":"全文検索","limit":1}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","statusIds":["019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07"],"nextCursor":"019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07"}
```

- **查询是纯文本**。插件把它拆分成词，找出包含所有词的文档。它绝不把文本当作所用引擎的查询语言来解读：其中的运算符、通配符、引号或字段名都只是普通文本。不含任何词的查询找不到任何结果。插件如何在日文以及其他不用空格分词的语言中找出词语，由它自己决定，这也正是使用插件的理由。
- **每页按从新到旧排序**。`statusIds` 按降序排列，每个 ID 最多出现一次，最多 `limit` 个，并且每个都是服务器发送时的帖子 ID（36 个小写字符）。这些 ID 是 UUIDv7，所以按字符串排序就是按时间排序。
- **游标归插件所有**。最后一页的 `nextCursor` 为空，其他页则是最多 256 字节、不含空格的可打印 ASCII，例如该页的最后一个 ID。服务器会把它原样发回以获取下一页，即使期间有文档增减，下一页也会从该 ID 之后继续。不是最后一页的页面可以包含少于 `limit` 个 ID。不是插件发出的游标会得到 `OUTCOME_INVALID_CURSOR`。
- **写入在下一次搜索中可见**，并且每个调用都可能到达两次：删除不存在的内容也算成功。对于只在刷新后才显示写入的引擎，插件要等待刷新完成。

| `outcome` | 含义 | 服务器的处理 |
| --- | --- | --- |
| `OUTCOME_OK` | 调用完成了要求的操作 | 对一页中的 ID 应用查看策略 |
| `OUTCOME_FAILED` | 引擎失败或没有响应 | 写入由其任务重试；搜索失败，用户可以再试一次 |
| `OUTCOME_INVALID_CURSOR` | 仅限 `Search`：游标不是插件发出的 | 告知客户端它的游标有误 |

缺失或未知的 `outcome`、失败的调用，以及违反这些规则的页面，都按 `OUTCOME_FAILED` 处理，这样的页面中任何内容都不会显示。一次调用最长持续 `plugin_search_timeout`（默认 10 秒），无论该设置是多少，最多 10 秒。

<!-- source: converting-media 36df3dbdb21b -->
## 转换媒体

媒体插件承担处理上传文件中最繁重的部分，因此可以在另一台机器上运行。媒体插件有三个类别，你可以分别选择：

- `media_image` 把图片重新编码为各个变体。
- `media_video` 对视频重新封装：带视频轨的文件，无论是否有声音。
- `media_audio` 对纯音频文件重新封装。

其余部分仍由服务器自己完成。它检测每个上传文件的格式，区分视频和音频文件，在上传文件到达时检查它是否超出限制，并为远程媒体生成预览。设置仍为 `native` 的类别在服务器中处理。

```sh
# Only videos go to a plugin, on a machine of its own; images and audio stay in the server.
MY_MEDIA_VIDEO=plugin
MY_PLUGIN_MEDIA_VIDEO_ADDRESS=https://media.internal:9000
```

为每个要交出的类别设置 `media_<kind> = plugin` 和 `plugin_media_<kind>_address`，然后重启。一个插件可以承担多个类别：为每个类别设置相同的地址即可。每个类别同时最多运行两个调用，每个调用最长持续 `plugin_media_<kind>_timeout`（默认 4 分钟），所以较慢的视频不会拖住图片。

- **删除元数据是插件的工作**。上传文件以发送时的原样到达插件，带有 EXIF 数据和标签，其中可能含有照片的拍摄地点。服务器会检查每个输出的格式和大小，但无法判断元数据是否已全部清除。`my plugin test --kind media_image` 会检查测试文件中的位置没有出现在输出中；其余部分请在插件中检查。
- **普通 TCP 以未加密的方式发送上传文件**。媒体类别接受 `http://`，启动时会给出警告。这时上传文件及其元数据会以明文穿过网络，两端都没有身份验证：该网络上的任何人都能读取它们，或者冒充插件作出回答。除非网络只供你自己使用，否则请使用 Unix 套接字或 `https://`。
- **服务器不信任输出**。它根据内容检测每个输出的格式，绝不依据插件声明的格式，并以该格式的内容类型和文件扩展名提供它。它在与上传文件相同的限制下解码每个输出，并检查其大小是否符合。如果输出大于声明的大小或超出这些限制、格式不是插件声明的格式，或者格式不是该变体接受的格式，上传就会作为服务器无法处理的文件而失败。
- **失败只是延迟**。插件没有运行或失败时，任务会稍后重试，与内置处理器相同。需要立即获得自定义表情符号图片的操作（例如复制远程表情符号或接受相关请求）在插件失败期间会失败。

<!-- source: the-contract-2 812c987c276c -->
### 契约

服务是 `my.plugin.media.v1.MediaService`，RPC `Process` 用于图片，`Remux` 用于视频或音频文件。两者都是双向流式传输，所以插件需要 Connect 或 gRPC 库以及 HTTP/2：`proto` 或 `grpc`，通过带 ALPN `h2` 的 TLS，或者在 Unix 套接字或普通 TCP 上使用明文 HTTP/2（h2c）。

调用是半双工的：

1. 服务器发送一个 `head`：限制，对于图片还包括用途、三种尺寸的边框和焦点。文件随后以最大 1 MiB 的 `chunk` 消息发送。然后服务器关闭自己这一端。
2. 插件回复一个 `head`，其 `outcome` 就是结果。结果为 `OUTCOME_OK` 时，`Process` 的 head 还包含按 EXIF 方向调整后的图片尺寸，以及小尺寸静态输出的 blurhash。
3. 对于每个输出，插件发送一个 `part`，写明输出的名称并声明其格式、尺寸和长度，随后是最大 1 MiB 的 `chunk` 消息。

`Process` 返回小、中、大三种静态输出，对于多于一帧的图片，还返回三种动态输出。小尺寸和中尺寸的输出是 WebP；大尺寸静态输出是 JPEG 或 PNG（自定义表情符号总是 PNG），大尺寸动态输出是 GIF。`Remux` 返回去除了元数据、保留原容器格式的文件，以及其中内嵌的封面图片（如果有）。服务器从 `Process` 最多读取图片限制的两倍，从 `Remux` 读取文件时最多读取视频限制，读取封面时最多读取图片限制。

| `outcome` | 含义 | 服务器的处理 |
| --- | --- | --- |
| `OUTCOME_OK` | 随后是输出 | 检查并保存输出 |
| `OUTCOME_TEMPORARY` | 插件失败 | 稍后重试任务 |
| `OUTCOME_UNSUPPORTED_FORMAT`、`OUTCOME_UNSUPPORTED_CODEC`、`OUTCOME_TOO_LARGE`、`OUTCOME_DIMENSIONS`、`OUTCOME_CORRUPT`、`OUTCOME_TOO_LONG` | 文件不可接受 | 把上传标记为失败 |

缺失或未知的 `outcome` 以及失败的调用按 `OUTCOME_TEMPORARY` 处理。

<!-- source: a-plugin-with-no-library be054eb80e74 -->
## 不使用库的插件

使用 `json` 时，插件只需要一个 HTTP 服务器和一个 JSON 库。一次 RPC 就是一个发往 `/<package>.<Service>/<Method>`、带有 `Content-Type: application/json` 的 `POST`，插件以 `200` 和一个 JSON 对象作答：

```text
POST /my.plugin.probe.v1.ProbeService/Echo
Content-Type: application/json
Connect-Protocol-Version: 1

{"payload":"AP8=","number":"9007199254740993"}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","payload":"AP8=","number":"9007199254740993"}
```

JSON 遵循 proto3 的标准映射：

- `int64` 是十进制字符串，`bytes` 是 base64，枚举是它的名称。
- 处于零值的字段可以省略，两个方向都是如此。插件必须把缺失的字段读作零值。
- 服务器忽略它不认识的字段。
- 结果是响应的 `outcome`。缺失或未知的 `outcome` 是失败，绝不是成功。
- 失败的插件返回 HTTP 错误，或者返回 Connect 的 `{"code":"unavailable"}` 及相应的状态码。任务会重试。

`Connect-Timeout-Ms` 和 `Connect-Protocol-Version` 可以忽略。
