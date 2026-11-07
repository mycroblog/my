<!-- source: external-login-providers 48a582ab87ec -->
# 外部登录提供方

外部登录提供方让用户可以用他们在别处拥有的账号登录：OpenID Connect 提供方
（Google Workspace、Microsoft Entra ID、Keycloak 等）、GitHub、Discord，或某一台 Mastodon 服务器。
你可以在拥有服务器配置的主机上用 `my login-provider` 命令设置提供方，也可以通过
管理 API（`/api/v1/admin/login_providers`）设置，这需要 `manage_login_providers` 权限，只有内置的
`admin` 角色拥有该权限。该命令在任何管理员能够登录之前就可以使用，因此你可以用它完成首次设置。
在 Web 客户端中，拥有该权限的管理员使用管理菜单中的“登录提供方”。列表
显示每个提供方的类型、状态以及是否允许注册；“添加提供方”会打开用于创建提供方的表单。
选择某个提供方会打开它的页面，页面上显示其回调 URL 以及复制该 URL 的按钮、关联的用户
数量，你也可以在这里修改其设置、为其设置新的 client secret、启用、停用和删除它。

<!-- source: what-external-sign-in-does 25fa37e22a50 -->
## 外部登录的作用

- 提供方只证明某人在该提供方处是谁。账号绝不会按电子邮件地址关联到用户：用户
  在设置的“关联的账号”中关联账号，或者用它作为新用户注册。与现有用户匹配的电子邮件地址
  绝不会关联到该用户。
- 已关联的用户在通过提供方登录后，仍需输入双重验证码（TOTP），账号
  未处于活跃状态的用户无法登录。
- 只有当你为某个提供方允许注册（`-allow-signup`），且
  [注册模式](registration.md)允许注册时，新用户才能通过该提供方注册；邀请码、CAPTCHA、注册屏蔽和网络限制
  与密码表单一样适用。以这种方式注册的人没有密码，只能通过该提供方登录。
  [注册路径示意图](registration.md#from-sign-up-to-the-app)并排展示了两种注册方式。

<!-- source: kinds c9123566b5c5 -->
## 类型

| 类型 | `-issuer` | 条件 |
| --- | --- | --- |
| `oidc` | issuer URL（`https`），与提供方的 ID token 中写的完全一致，例如 `https://accounts.google.com` | `-claim <claim>=<value>`：只允许 ID token 中该 claim 的值为所给值之一的用户 |
| `github` | 无：服务器知道 github.com 的端点 | `-org <organization>`：只允许其中某个 Organization 的成员（需添加 `-scope read:org`） |
| `discord` | 无：服务器知道 discord.com 的端点 | `-guild <server ID>`：只允许其中某个 Discord 服务器（guild）的成员；不指定时允许所有 Discord 用户 |
| `mastodon` | 该服务器的 Web 域名，例如 `mastodon.example`（Mastodon 4.3 或更高版本） | 无 |

对于 Google Workspace，请用 `-claim hd=<your domain>` 把用户限制在你的组织内。不要依赖电子邮件地址的
域名。

<!-- source: examples 0d19ab463197 -->
## 示例

<!-- source: google b99294b8539d -->
### Google

在 Google Cloud 控制台中，创建类型为“Web application”（Web 应用）的 OAuth 客户端。要将其限制在你的组织内，请把
OAuth 同意屏幕的用户类型设为“Internal”（内部，当项目属于某个 Google Workspace
组织时可用），并同时添加 `hd` 条件：用户类型限制 Google 允许哪些人进入，而该条件让本
服务器检查每个 ID token 的 `hd` claim。

```sh
printf '%s\n' "$CLIENT_SECRET" | my login-provider create -kind oidc -name Google \
  -issuer https://accounts.google.com -client-id 123.apps.googleusercontent.com -claim hd=example.com
```

<!-- source: keycloak-and-other-openid-connect-providers 0840dc38a4e0 -->
### Keycloak 和其他 OpenID Connect 提供方

issuer 是 realm URL，例如 `https://sso.example.com/realms/main`，与提供方的 ID token 中写的完全一致。
请创建一个使用标准流程的 confidential 客户端，并使用 `openid`、`email` 和 `profile` scope。如果提供方
位于私有网络中，请把它列在 `login_provider_allowed_destinations` 中（[见下文](#providers-on-an-internal-network)）。
使用 `-claim` 可以要求某个 claim（例如组 claim）的值为所给值之一。

<!-- source: discord 00e9659d1eae -->
### Discord

在 [Discord Developer Portal](https://discord.com/developers/applications) 中创建一个应用，打开“OAuth2”，然后
把 client ID 和 client secret 提供给服务器。服务器会请求 `identify` 和 `email` scope，当你用 `-guild` 列出服务器时还会请求
`guilds`。用户以其数字 Discord ID 识别；用户名和昵称
只用于在注册表单上建议用户名。只有当 Discord 表示电子邮件地址已验证时，该地址才视为已验证；
否则用户需要像其他地址一样通过邮件确认。

使用 `-guild` 时，服务器会在每次登录、注册和关联时检查用户是否属于所列服务器之一，
不属于则拒绝该用户。不使用时，所有 Discord 用户都可以登录。guild ID 是最多 20 位的数字：在
Discord 中打开“Developer Mode”（开发者模式，位于“用户设置”的“高级”中），然后右键单击服务器图标，选择“Copy Server
ID”（复制服务器 ID）。

```sh
printf '%s\n' "$DISCORD_SECRET" | my login-provider create -kind discord -name Discord \
  -client-id 123456789012345678 -guild 987654321098765432
```

在 `add` 打印出回调 URL 之后，把它添加到应用的“OAuth2”中的“Redirects”下。

<!-- source: github a7181ea6edd6 -->
### GitHub

创建一个 OAuth App（Settings、Developer settings、OAuth Apps），并把 client ID 和 secret 提供给服务器。服务器会请求
`user:email` scope，当你用 `-org` 列出 Organization 时还会请求 `read:org`。使用 `-org` 时，服务器会
在每次登录时检查用户是否属于所列 Organization 之一，因此其他 Organization 的成员会被拒绝。

<!-- source: set-up-a-provider 8e0bb73251d2 -->
## 设置提供方

提供方所需的 redirect URI 是该提供方的回调 URL `https://<host>/auth/external/<id>/callback`，
其中包含服务器分配给该提供方的 ID。因此：

1. 在提供方处注册一个 OAuth 客户端（在 GitHub 上是“OAuth App”），redirect URI 暂时随意填写，并记下
   它给出的 client ID 和 client secret。
2. 使用它们运行 `my login-provider create`。它会打印出回调 URL。
3. 把提供方处该客户端的 redirect URI 改为这个回调 URL。

```sh
# The client secret is read from standard input, or prompted for in a terminal.
printf '%s\n' "$CLIENT_SECRET" | my login-provider create -kind oidc -name "Example SSO" \
  -issuer https://sso.example.com -client-id my-client-id -claim hd=example.com
printf '%s\n' "$GITHUB_SECRET" | my login-provider create -kind github -name GitHub \
  -client-id Iv1.0123456789abcdef -org example-org -scope read:org
```

<!-- source: mastodon 6f167bff1f67 -->
### Mastodon

`mastodon` 提供方是你指定的某一台 Mastodon 服务器；其他服务器的用户无法通过它登录。该服务器
必须是 Mastodon 4.3 或更高版本：`add` 会读取它的授权服务器元数据
（`/.well-known/oauth-authorization-server`），并拒绝没有该元数据的服务器。不指定 `-client-id` 时，`add` 会自行在该服务器上注册客户端，
使用回调 URL 和 `profile` scope，并保存得到的 client ID 和 secret，因此
在该服务器上无需做任何操作：

```sh
my login-provider create -kind mastodon -name mastodon.example -issuer mastodon.example
```

如果该服务器不允许他人注册客户端，`add` 会失败。此时请在该服务器的开发设置中创建一个带有 `profile` scope 和
回调 URL 的应用，并像其他类型一样提供它的 client ID 和 secret
（回调 URL 中包含提供方的 ID，因此请按照上面的三个步骤操作）。

用户只使用 `profile` scope 登录，它只读取用户的账号，不读取其他任何内容。Mastodon 不提供电子邮件
地址，因此通过它注册的人需要通过邮件确认地址。注册表单会建议该账号的用户名。
当该服务器的域名或其上级域名有域名操作（[域名操作](moderation.md#domain-actions)）时，它的按钮会消失，
没有人能通过它登录、注册或关联账号。提供方的服务器不能更改；如需使用另一台服务器，请设置一个新的提供方。

client secret 使用密钥加密密钥（[密钥管理](key-management.md)）加密存储，之后无论是命令还是管理 API
都不会再显示它。

新提供方创建后即处于启用状态。默认情况下，用户可以把现有账号关联到它（`-allow-link`），但没有人能通过它
注册：添加 `-allow-signup` 可以让新用户通过它注册，但仍受注册模式的限制
（[注册](registration.md)）。

<!-- source: change-rotate-disable-and-delete 55a0d23ac0d0 -->
## 修改、轮换、停用和删除

```sh
my login-provider list                                 # every provider with its callback URL and linked users
my login-provider set -name "Corp SSO" <id>            # changes only the settings it names
my login-provider set -claim '' <id>                   # '' removes every -claim (or -scope, -org)
my login-provider rotate-secret <id>                   # reads the new client secret like add
my login-provider disable <id>
my login-provider enable <id>
my login-provider delete <id>
```

- `set` 只修改它指定的设置。`-scope`、`-claim` 和 `-org` 会替换整个列表。提供方的类型
  不能更改；请改为设置一个新的提供方。
- `rotate-secret` 把 client secret 替换为你输入的值。请先在提供方处轮换，然后再在这里轮换。
- `disable` 会保留提供方以及关联到它的账号，但在你运行 `enable` 之前，没有人能通过它登录、注册或关联，
  登录页面也不会显示它。`disable` 和 `delete` 会打印有多少关联用户将无法再
  通过该提供方登录，以及在[仅限登录提供方](#limit-sign-in-to-login-providers)的设置开启时，该限制现在会如何作用。
- `delete` 会永久删除该提供方。

<!-- source: users-who-sign-in-only-through-a-provider 11dc6f466cb7 -->
### 只通过提供方登录的用户

停用或删除提供方不会影响用户。拥有密码或通行密钥、或拥有其他关联
账号的用户，可以通过这些方式登录，除非[登录被限制为仅限登录提供方](#limit-sign-in-to-login-providers)且另一个
提供方仍处于启用状态。这些都没有的用户将完全无法登录：`disable` 和 `delete` 会打印有多少
关联用户会失去该提供方，Web 客户端也会在确认时显示这个数字。`enable` 可以恢复已停用提供方的
用户；已删除的提供方无法恢复。

要帮助这样的用户恢复，请在管理界面中该账号的“管理”页面上，或使用
`my admin account password-reset-link @alice` 签发密码重置链接，并把它交给该用户（[注册](registration.md#password-reset-and-change)）。
之后用户使用新密码登录。

每次更改都会以提供方的显示名称记录在审计日志中，操作者记录为 CLI，或通过管理 API 进行更改的
管理员。

<!-- source: limit-sign-in-to-login-providers bb5ebf61b647 -->
## 仅限通过登录提供方登录

实例设置 `login_providers_only`（默认关闭）会在至少有一个提供方处于启用状态时，让提供方成为登录和注册的唯一方式。
在 Web 客户端“服务器设置”的“注册”页面中开启它，或使用 CLI：

```sh
my instance set login_providers_only true
```

Web 客户端中的确认信息和该命令的输出会说明有多少活跃的本站用户没有关联到任何
启用的提供方。如果其中包括你自己，Web 客户端也会提示。它只发出警告而不会拒绝：由你决定。

限制生效期间：

- 密码和通行密钥无法让任何人登录，登录页面只显示提供方。已经用
  密码或通行密钥开始、正在等待双重验证码的登录也会被拒绝。
- 没有人能用密码注册。注册页面会显示允许注册（`-allow-signup`）的提供方，并照常受
  [注册模式](registration.md)约束。如果没有这样的提供方，注册状态显示为 `closed`，对客户端
  和在 NodeInfo 中也是如此。
- 没有人能申请或使用密码重置链接。链接在过期之前保持有效，如果限制及时结束，仍可使用。
- 用户无法设置或修改密码，也无法添加通行密钥，因为两者都无法用于登录。用户仍然可以删除它们。
- `my admin account password-reset-link` 和管理界面仍会签发链接，并说明在限制结束之前
  它无法使用。`my admin create` 仍会创建管理员，并警告该密码无法用于登录。
- 用户无法解除其在已启用提供方处的最后一个关联账号。

仍然可用的内容：

- 会话、应用访问令牌和刷新令牌保持有效。该限制不会使其中任何一个失效。
- 在会话中，密码或通行密钥仍可用于确认操作。现有用户就是这样迁移到提供方的：在
  登录状态下，打开设置中的“关联的账号”，用密码或通行密钥确认，然后在提供方处登录。
- 在提供方处的登录、注册和关联，以及通过提供方登录后的双重验证码。

无论发送的是什么电子邮件地址、密码或令牌，每次拒绝的内容都相同，因此不会透露任何账号的信息。

**没有关联账号的用户在会话结束后将无法登录，直到你采取措施。** 账号绝不会按电子邮件
地址关联，因此提供方的登录也找不到他们。请让他们在登录期间关联一个账号。失去会话的
用户需要你暂时结束该限制，否则只能让他们无法进入。这也包括处于 `disable_login`、
`suspend` 或 `ban` 状态的用户，没有关联账号时，他们没有可以发起[申诉](moderation.md)的登录页面。
关联还需要浏览器在登录页面上处于登录状态：应用仍处于登录状态、但浏览器
会话已经结束的用户，在限制生效期间无法关联，只能等待你处理。**在开启限制之前，请先向用户
公告这一变更，**以便他们在会话仍然有效时关联账号。

<!-- source: end-the-limit 77f5c71d013b -->
### 结束限制

该限制在每次请求时都会依据设置和提供方的状态，无需重启。

- 被锁在外面的管理员可以在拥有服务器配置的主机上运行以下命令：

  ```sh
  my instance set login_providers_only false
  ```

- 当没有任何提供方处于启用状态时，该限制不生效：停用或删除最后一个提供方，会重新开放密码和通行密钥
  登录，无论设置如何。既没有密码也没有通行密钥的用户仍然需要
  [重置链接](registration.md#password-reset-and-change)。
- 在设置开启且没有启用任何提供方时，设置或启用一个提供方会立即使限制生效；新提供方
  在设置完成后即被启用。Web 客户端会先发出警告，说明有多少活跃用户没有关联到该提供方，
  以及其中是否包括你。它只对会使限制生效的操作发出警告。`my login-provider create`、`enable`、
  `disable` 和 `delete` 会打印在设置开启时该限制现在会如何作用。
- 当提供方发生故障导致所有人都无法进入时，使用同样的命令。

<!-- source: re-authentication 8ec1742f8811 -->
## 重新认证

需要密码或通行密钥的操作（设置密码、添加通行密钥、删除或迁移账号、
关联其他账号）会要求两者都没有的用户在已关联的提供方处重新登录（[注册](registration.md)）。
服务器会要求 OpenID Connect 提供方重新认证用户（`max_age=0` 和 `prompt=login`），并
拒绝较早的认证；对 Mastodon 服务器则使用 `force_login=true` 请求。GitHub 和 Discord 无法
要求重新登录，因此仍在那里保持登录状态的用户无需输入任何内容即可通过确认。服务器仍会检查
该账号是否为已关联的账号，但这对“本人正在操作”的证明较弱。如果这一点很重要，请优先使用 OpenID
Connect 提供方，或让用户保留密码或通行密钥。

<!-- source: providers-on-an-internal-network 7f19e187a375 -->
## 内部网络上的提供方

服务器通过其出站 HTTP 防护访问提供方，该防护会拒绝回环地址和私有地址。要使用
你自己网络中的提供方（例如服务器旁边的 Keycloak），请在
[`login_provider_allowed_destinations`](../reference/configuration.md#login_provider_allowed_destinations) 中列出其主机或地址范围。
