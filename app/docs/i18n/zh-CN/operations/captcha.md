<!-- source: captcha-bot-protection-turnstile--hcaptcha--recaptcha 2c065ac1d2fb -->
# 用 CAPTCHA 防范机器人（Turnstile / hCaptcha / reCAPTCHA）

CAPTCHA 默认关闭：设置 [`captcha`](../reference/configuration.md#captcha) 为 `none`。此时登录、注册和重置密码页面无需 JavaScript 即可使用，也不会向第三方发送任何数据。选择一个 CAPTCHA 服务后，该服务的组件会出现在三个表单上：登录（`/auth/sign_in`）、注册（`/auth/sign_up`）和申请重置密码链接（`/auth/forgot_password`，仅限会发送邮件的服务器）。服务器通过该服务的 siteverify API 验证组件生成的令牌。无法只保护其中一部分表单。

<!-- source: choosing-a-service 22ddd3cc639f -->
## 选择服务

| `captcha` | 服务 | 费用 | 用户需要做什么 |
| --- | --- | --- | --- |
| `turnstile`（推荐） | Cloudflare Turnstile | 免费，不限量 | 通常什么都不用做；仅在需要时勾选复选框 |
| `hcaptcha` | hCaptcha | 有免费额度 | 勾选复选框，需要时还要完成图片挑战 |
| `recaptcha` | Google reCAPTCHA v2（复选框） | 每月 10,000 次评估以内免费（见下文） | 勾选复选框，需要时还要完成图片或音频挑战 |

不支持 reCAPTCHA v3（仅评分）和 reCAPTCHA Enterprise。

<!-- source: creating-keys 38cd93394638 -->
## 创建密钥

每个服务都会给你一对密钥：公开的**站点密钥**，以及只有服务器持有的**私密密钥**。将设置 `host` 的主机名（不含端口）注册为域名。如果 siteverify 响应中的主机名与 `host` 不一致，服务器会拒绝该令牌。

- **Turnstile**：在 Cloudflare 控制台的“Turnstile”下添加组件并注册主机名。组件模式选择“Managed”。
- **hCaptcha**：在 hCaptcha 控制台添加站点并注册主机名。私密密钥在账号设置中。
- **reCAPTCHA**：在 Google reCAPTCHA 管理控制台中，选择带“I'm not a robot”复选框的类型“Challenge (v2)”，并注册域名。密钥属于某个 Google Cloud 项目。

各服务的测试密钥（始终验证成功的那种）适合用来确认组件能显示并生成令牌。但使用测试密钥时，siteverify 返回固定的主机名（Turnstile 为 `example.com`，hCaptcha 为 `dummy-key-pass`，等等），因此在本服务器上登录和注册都会失败，并提示“请完成人机验证，然后重试”。

<!-- source: configuring 3301db5c6286 -->
## 配置

```sh
MY_CAPTCHA=turnstile
MY_CAPTCHA_SITE_KEY=0x4AAAAAAA...
MY_CAPTCHA_SECRET_KEY_FILE=/run/secrets/my_captcha_secret_key
```

- 私密密钥只能通过环境变量 `MY_CAPTCHA_SECRET_KEY`，或指向文件的 `MY_CAPTCHA_SECRET_KEY_FILE` 传入。日志中显示为 `[REDACTED]`。
- 如果选择了 `captcha` 却缺少密钥，`my config check` 会报告问题，服务器也不会启动。
- 服务器启动时不会试用密钥。当服务拒绝私密密钥或站点密钥时，第一次验证会记录 ERROR 日志“the CAPTCHA service refused the secret key or the site key”。在你修正之前，请求会按下文“服务无法访问时”的方式处理。
- 修改在重启服务器后生效。

<!-- source: how-verification-works c5cc12dd3769 -->
## 验证的工作方式

- 顺序依次为：速率限制、CAPTCHA 验证，然后是密码检查、注册处理或发送重置邮件。超出速率限制的请求不会发送到服务。
- 持有有效邀请码的注册同样需要验证。
- 未通过验证的登录会计为该客户端 IP 地址的一次登录失败。它不计入账号的失败次数，因此任何人都无法把别人锁在外面。未通过验证的注册会计为一次注册尝试（见[注册](registration.md)中的速率限制）。申请重置链接同样计为一次申请尝试。
- 对于 Turnstile，组件会带上操作名（`sign_in` / `sign_up` / `password_reset`），服务器会检查响应中的操作是否相同。一个表单的令牌不能用在另一个表单上。
- 服务器不会把用户的 IP 地址（`remoteip`）发送给服务。令牌只能使用一次，因此服务器不会重试。超时时间为 10 秒。

<!-- source: when-the-service-is-unreachable fe6a6d5915ea -->
### 服务无法访问时

服务无法访问、超时、返回 5xx 或拒绝密钥时：

| 表单 | 处理方式 |
| --- | --- |
| 注册和申请重置链接 | 拒绝（`503`），并提示“请稍后再试”。否则机器人可能借此触发邮件和注册。 |
| 登录 | 不经 CAPTCHA 直接放行，服务器记录 WARN 日志“the CAPTCHA service could not check a sign-in; letting it through”。速率限制和密码检查仍然有效。 |

放行登录是为了避免服务故障或密钥错误把所有人（包括管理员）都锁在外面。如果 WARN 日志持续出现，请检查服务状态和密钥。

<!-- source: recaptcha-free-tier 4f413b07dbb9 -->
### reCAPTCHA 免费额度

超出每月 10,000 次评估的免费额度后，reCAPTCHA 的 siteverify **总是验证成功**（因此在这段时间内 CAPTCHA 不提供任何保护）。服务器无法察觉超额，请在 Google Cloud 控制台中查看用量。

<!-- source: pages-and-content-security-policy b0cf9741b2ce -->
## 页面与 Content-Security-Policy

- 组件的语言跟随页面的语言区域，主题跟随浏览器的配色方案（浅色或深色）。Turnstile 组件自行跟随配色方案。hCaptcha 和 reCAPTCHA 则根据浏览器发送的客户端提示 `Sec-CH-Prefers-Color-Scheme` 选择主题；不发送该提示的浏览器使用浅色主题。
- 页面会在组件旁说明由哪个服务进行验证、该服务会收到 IP 地址和浏览器信息，并链接到该服务的隐私政策（hCaptcha 和 reCAPTCHA 还会链接到其条款）。对于 reCAPTCHA，这同时也是 Google 要求展示的声明：“This site is protected by reCAPTCHA and the Google Privacy Policy and Terms of Service apply.”
- 在不支持 JavaScript 的浏览器中，`<noscript>` 消息会说明验证需要 JavaScript。无法完成 CAPTCHA 的用户既不能注册也不能登录，你只能通过其他方式帮助他们，例如用 `my admin create` 创建账号，或暂时设置 `captcha = none`。
- 启用 CAPTCHA 时，`Content-Security-Policy`（CSP）只在登录、注册和申请重置密码页面上放宽。绝不会添加 `'unsafe-inline'` 和 `'unsafe-eval'`。Web 客户端、客户端 API、授权同意页面、邮箱确认等其他页面的 CSP 不变。

| `captcha` | 添加的来源 |
| --- | --- |
| `turnstile` | `script-src`、`frame-src`：`https://challenges.cloudflare.com` |
| `hcaptcha` | `script-src`、`frame-src`、`style-src`、`connect-src`：`https://hcaptcha.com`、`https://*.hcaptcha.com` |
| `recaptcha` | `script-src`：`https://www.google.com/recaptcha/`、`https://www.gstatic.com/recaptcha/`；`frame-src`：`https://www.google.com/recaptcha/`、`https://recaptcha.google.com/recaptcha/`；`connect-src`：`https://www.google.com/recaptcha/` |

如果服务器前面的反向代理或 CDN 会覆盖 CSP，请为这三个页面添加相同的来源。

<!-- source: what-to-write-in-the-privacy-policy d601071ebd4c -->
## 隐私政策中要写什么

启用 CAPTCHA 后，任何打开登录、注册或申请重置密码页面的人，其浏览器都会直接连接到该服务，服务会收到 IP 地址、浏览器信息和 Cookie。实例的隐私政策（`my instance set privacy_policy`，见[实例设置](instance-settings.md)）由你自己撰写，服务器不会往里添加任何内容。[隐私政策模板](privacy-policy-template.md)中有一行是为该服务准备的。至少写明以下内容。

- 你使用哪个服务（提供商的名称）以及原因：防止自动化的登录、注册和重置密码请求。
- 哪些数据会到达该服务（IP 地址、浏览器和设备信息、Cookie、与组件的交互），以及只有登录、注册和申请重置密码页面会发送这些数据。
- 该服务隐私政策的链接（Turnstile：<https://www.cloudflare.com/privacypolicy/>，hCaptcha：<https://www.hcaptcha.com/privacy>，reCAPTCHA：<https://policies.google.com/privacy>）。
- 对于 reCAPTCHA，说明适用 Google 的隐私政策和条款（<https://policies.google.com/terms>）。
