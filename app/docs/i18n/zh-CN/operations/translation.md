<!-- source: translating-posts-with-a-translation-service bf15671669b4 -->
# 使用翻译服务翻译帖子

读者请求时，服务器可以通过你配置的翻译服务翻译帖子：LibreTranslate、DeepL、Google Cloud Translation 或 Amazon Translate。服务器拥有某个服务商的密钥时，该服务商即内置可用，再由[实例设置](instance-settings.md) `translation_providers` 启用它。

<!-- source: libretranslate 0d5e448bb450 -->
## LibreTranslate

LibreTranslate 是开源的机器翻译，你可以自己运行，也可以使用 [libretranslate.com](https://libretranslate.com) 提供的托管服务。通过[配置](../reference/configuration.md)中的两项设置让服务器指向它：

```sh
MY_TRANSLATOR_LIBRETRANSLATE_URL=http://libretranslate:5000
# Only when the server asks for a key, as libretranslate.com does:
MY_TRANSLATOR_LIBRETRANSLATE_API_KEY=your-key
```

- `translator_libretranslate_url` 是 LibreTranslate 服务器的基础 URL，使用 `http:` 或 `https:`，可以带路径。
  位于私有地址上的服务器也可以使用，例如与本服务器相邻的容器或 `http://10.0.0.5:5000`。
  服务器不会跟随它返回的重定向，因此密钥不会被发送到其他任何地方。
- `translator_libretranslate_api_key` 是机密信息，只从环境变量读取（`MY_TRANSLATOR_LIBRETRANSLATE_API_KEY` 或 `MY_TRANSLATOR_LIBRETRANSLATE_API_KEY_FILE`）。服务器不需要密钥时不要设置它；自己运行的服务器通常不需要。密钥绝不会显示给客户端，也不会写入日志。

然后把 `libretranslate` 按你希望尝试的顺序加入实例设置 `translation_providers`，例如只设置 `translation_providers = libretranslate`，或在它后面加上另一个服务商的 ID 作为后备。

服务器在第一次翻译前以及之后每小时向 LibreTranslate 获取其支持的语言列表，不会发送列表中没有的语言对的帖子。
帖子没有标注语言时，由 LibreTranslate 检测语言。
在你自己运行的服务器上，请安装读者需要的语言模型。

**翻译质量因语言而异。** LibreTranslate 的模型对某些语言对效果很好，对另一些则比较粗糙，从训练数据较少的语言翻译时可能会偏离原意。启用前请先试试你的社区使用的语言，如有需要，在 `translation_providers` 中把一个商业服务商排在它后面。

**文本会去哪里。** 你自己运行的服务器会把帖子的文本留在你自己的机器上，其他任何人都不会收到。
这时你可以通过 `translation_provider_visibilities` 允许它翻译 `private` 和 `direct` 帖子，例如 `libretranslate=public|local|unlisted|private|direct`，而且[隐私政策](privacy-policy-template.md#the-services-you-choose)中不需要列出用于翻译的外部服务。托管的 LibreTranslate，无论是 libretranslate.com 还是其他人提供的，都会像其他服务商一样收到文本：请阅读其条款，保留默认的可见范围，并在隐私政策中写明它。

<!-- source: deepl 80449fb800f7 -->
## DeepL

DeepL 通过其 API 进行翻译，使用 DeepL API 套餐的密钥。服务器发送帖子的文本，其中代替提及、链接和装饰的标记以 XML 元素表示，DeepL 会把它们保留在原位；帖子的其他内容一概不发送。

1. 从你的 DeepL 账户获取认证密钥。免费套餐的密钥以 `:fx` 结尾。
2. 在[部署配置](../reference/configuration.md)中设置它，然后重启服务器：

   ```sh
   MY_TRANSLATOR_DEEPL_API_KEY=your-key
   ```

   对于免费套餐的密钥，服务器调用 `https://api-free.deepl.com`；对于其他密钥，调用 `https://api.deepl.com`。
   要经过代理，请设置 `MY_TRANSLATOR_DEEPL_URL`。密钥是机密信息：服务器绝不会把它写入日志、放进错误信息，或在设置或客户端 API 的响应中显示它，并且只把它发送到该地址，不跟随任何重定向。
3. 通过[实例设置](instance-settings.md) `translation_providers` 启用该服务商，可以单独使用，也可以按你的顺序排列：

   ```sh
   my instance set translation_providers deepl
   ```

服务器在第一次翻译时以及之后每天向 DeepL 询问它能翻译哪些语言。帖子的语言会对应到 DeepL 拥有的最接近的变体：英语对应 `en-US`，葡萄牙语对应 `pt-BR`，台湾地区的中文对应 `zh-Hant`。DeepL 不支持的语言会交给下一个服务商，否则会告知读者无法翻译。

**配额。** 每个套餐都有每月的字符配额，付费套餐还可以设置你在 DeepL 账户中设定的费用上限。
DeepL 报告达到配额或速率限制时，服务器会尝试 `translation_providers` 中的下一个服务商。
没有剩余服务商时，读者会看到该帖子暂时无法翻译；如果你允许，Web 客户端会使用浏览器的翻译功能。
服务器自身的每月限额（`translation_user_monthly_characters` 和 `translation_server_monthly_characters`）在任何调用之前生效，因此请把它们设得低于你的 DeepL 配额。DeepL 计算字符数的方式可能与它们不同。

**数据。** 启用前请阅读 [DeepL 的条款](https://www.deepl.com/en/pro-license)：

- 在付费套餐中，DeepL 只在翻译所需的时间内保留文本和译文，调查错误时最多保留 72 小时。
- 在免费套餐中，DeepL 可能会永久保留文本和译文，并且其条款禁止通过它发送个人数据。
  帖子通常包含个人数据，因此免费套餐只用于试用该服务商。绝不要在 `translation_provider_visibilities` 中允许它翻译 `private` 或 `direct` 帖子，并在你的隐私政策中说明 DeepL 会保留它收到的内容。

在付费套餐中，你可以按服务商逐个允许 `private` 和 `direct` 帖子：

```sh
my instance set translation_provider_visibilities 'deepl=public|local|unlisted|private'
```

**署名。** DeepL 的条款要求在每条译文旁显示其名称或标志。Web 客户端会在每条来自 DeepL 的译文旁显示“DeepL”以及指向 `https://www.deepl.com` 的链接，因此你无需另外添加。

<!-- source: google-cloud-translation e351f57274b0 -->
## Google Cloud Translation

[Cloud Translation](https://cloud.google.com/translate) 的服务商 ID 是 `google`。服务器使用通用翻译模型调用它的 `TranslateText` 方法，并以 HTML 形式发送文本，使提及、链接以及其他必须保持原样的部分回到原位。

1. 在为翻译付费的项目中启用 Cloud Translation API（`translate.googleapis.com`）： `gcloud services enable translate.googleapis.com --project PROJECT`。
2. 在该项目上为服务器的服务账号授予 `roles/cloudtranslate.user`： `gcloud projects add-iam-policy-binding PROJECT --member serviceAccount:SA_EMAIL --role roles/cloudtranslate.user`。
3. 把 `MY_TRANSLATOR_GOOGLE_PROJECT` 设为项目 ID。`MY_TRANSLATOR_GOOGLE_LOCATION` 默认为 `global`；也可以指定 Cloud Translation 中提供通用模型的其他位置，例如 `us-central1`。
4. 重启服务器，并把 `google` 加入实例设置 `translation_providers`。

与它使用的其他 Google Cloud 服务一样，服务器使用应用默认凭据（Application Default Credentials）进行认证：在 Cloud Run 上使用该服务的服务账号；在其他环境中使用 `GOOGLE_APPLICATION_CREDENTIALS` 指定的密钥文件。不使用 API 密钥。服务器启动时不会发出任何调用，因此缺少 API 或角色只会在读者请求翻译时才暴露出来：此时服务器会尝试下一个服务商，没有剩余服务商时，读者会看到翻译失败。
Google 报告项目达到配额或速率限制时，也会转到下一个服务商。

Google 在每月免费额度之后按发送的字符收费；请参阅 [Cloud Translation 价格](https://cloud.google.com/translate/pricing)。服务器自身的每月限额也会计算字符数，但与 Google 的计费方式并不完全相同，因此请同时在 Cloud Billing 中设置预算提醒。

Google [声明](https://cloud.google.com/translate/data-usage)它只为提供翻译而使用文本，只在内存中短暂保留，不用它训练模型，也不会与他人共享。服务器调用的是 Cloud Translation 的全球端点，在这里 Google 不会把处理限定在某一个区域内。

Google 的[署名要求](https://cloud.google.com/translate/attribution)要求把译文标示为机器翻译，并在旁边显示链接到 `https://translate.google.com` 的“powered by Google Translate”徽标。
Web 客户端会在此服务商的每条译文旁显示名称“Google Translate”以及该链接。Google 只以下载形式提供徽标图片，因此服务器不会为此服务商向客户端提供标志；请确认读者看到的内容符合 Google 对你的实例的要求。

<!-- source: amazon-translate a6cd83df7ef1 -->
## Amazon Translate

Amazon Translate 的服务商 ID 是 `aws`。与其他 AWS 服务一样，服务器使用 AWS SDK 默认凭据链中的凭据调用它的 `TranslateText` 操作：任务或实例的执行角色、环境变量 `AWS_ACCESS_KEY_ID` 和 `AWS_SECRET_ACCESS_KEY`，或共享凭据文件。服务器的配置中不需要任何密钥。

1. 为服务器运行时使用的角色或用户授予 IAM 权限 `translate:TranslateText`。对于语言未知的帖子，Amazon Translate 会使用 Amazon Comprehend 检测语言；AWS 针对 Amazon Translate 的托管策略会同时授予 `comprehend:DetectDominantLanguage`，因此也请授予该权限。
2. 把 `translator_aws_region`（`MY_TRANSLATOR_AWS_REGION`）设为 Amazon Translate 所在的区域，例如 `us-east-1`。
   检测语言还需要该区域同时提供 Amazon Comprehend。
3. 重启服务器，然后通过 `translation_providers = aws` 启用该服务商，可以单独使用，也可以按你的偏好与其他服务商排列顺序（参见[实例设置](instance-settings.md)）。

Amazon Translate 按发送的字符收费，前 12 个月有免费套餐（[价格](https://aws.amazon.com/translate/pricing/)）。服务器只发送帖子中的文字：提及、话题标签、链接和代码留在服务器上，每一项都以 HTML 标记中的简短占位符代替。请把 `translation_user_monthly_characters` 和 `translation_server_monthly_characters` 的每月限额控制在你愿意支付的范围内。

**除非账号受 AWS Organizations 的 [AI 服务退出策略](https://docs.aws.amazon.com/organizations/latest/userguide/orgs_manage_policies_ai-opt-out.html) 约束，否则 Amazon 可能会存储文本并用于改进其服务，且可能在其他区域进行。** 没有退出时，对于这些文本，Amazon 就不仅仅是你的服务提供方：请在隐私政策中说明这一点（[隐私政策模板](privacy-policy-template.md#the-services-you-choose)），并且不要在 `translation_provider_visibilities` 中为 `aws` 允许 `private` 或 `direct`。默认情况下，该服务商只会收到 `public`、`local` 和 `unlisted` 帖子。

使用构建标签 `no_translator_aws` 构建的服务器没有该服务商，并且在设置了 `translator_aws_region` 时会拒绝启动（参见[构建标签](../reference/build-tags.md)）。
