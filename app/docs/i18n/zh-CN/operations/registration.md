<!-- source: user-registration-registration-modes-invites-approval-email-confirmation 26f54561fd7a -->
# 用户注册（注册模式、邀请、审批、邮箱确认）

用户如何注册并确认邮箱地址，以及注册模式、邀请码、注册申请的审批、密码重置和注册屏蔽。

<!-- source: from-sign-up-to-the-app fff50db25fa9 -->
## 从注册到使用应用

![新用户的路径：注册表单或登录提供方、可能拒绝注册的检查、各注册模式给出的状态，以及进入应用之前的审批和邮箱确认。下面的列表说明了同样的内容。](images/sign-up-flow.svg)

从图的上方往下：

- **开始。** 访客在注册页面（`/auth/sign_up`）上用用户名、邮箱地址和密码创建账号；[注册仅限登录提供方](#registration-limited-to-login-providers)时，该页面不提供这种方式。或者，访客用某个[登录提供方](login-providers.md)登录，而其在该提供方的账号尚未关联到任何用户；如果该提供方允许注册（`-allow-signup`），访客会看到同样的表单，只是没有密码栏。
- **依次进行的检查。** 注册模式（`closed` 拒绝所有注册）、每个网络的限制（每 30 分钟 5 次）、CAPTCHA（当 `captcha` 不是 `none` 时）、[注册屏蔽](#sign-up-blocks)（`reject` 拒绝，`require_approval` 转为等待审批）、邀请码（必须可用，`invite` 模式下必填）、对没有邀请的注册进行审查的 [`signup.check`](decision-hooks.md#sign-ups) 插件，以及用户名和邮箱地址未被占用。未通过检查的注册会被拒绝，表单上会显示错误，并且不会写入任何内容。
- **注册后的状态。** 新用户会立即登录。在 `open` 模式下，注册直接获批，`mail = none` 时则等待审批；在 `approval` 模式下，注册等待审批，有可用的邀请时则直接获批；在 `invite` 模式下，注册直接获批。无论哪种模式，`require_approval` 屏蔽或 `signup.check` 的暂缓都会让注册处于等待审批状态。
- **账号可用之前。** 等待审批的注册需要审核员批准（`my registration approve` 或管理页面）。在 `mail = smtp` 时，邮箱地址必须通过邮件中的链接确认；登录提供方已验证的地址会立即确认，在 `mail = none` 时则不需要确认。被拒绝的注册不会获批，该账号无法使用应用。
- **应用。** 可用的账号会进入欢迎步骤（设置昵称和头像），然后进入首页时间线。

以下各节说明每个步骤。

<!-- source: the-sign-up-page 111fbe5ecf64 -->
## 注册页面

用户在 `https://<host>/auth/sign_up` 上用用户名、邮箱地址和密码注册，除非[注册仅限登录提供方](#registration-limited-to-login-providers)。注册后，该浏览器会立即登录。该页面无需 JavaScript 即可使用，并且和登录页面一样，受 CSRF 令牌和拒绝跨源请求的保护。如果你选择用 CAPTCHA 防范机器人，注册和登录页面会显示其组件，并需要 JavaScript（[CAPTCHA](captcha.md)）。

注册模式是实例设置 `registration_mode`。可以用 CLI 选择，也可以在 Web 客户端中“服务器设置”的“注册”页面（`/admin/settings/registration`）上选择（[实例设置](instance-settings.md)）。更改从下一次注册起生效。在服务器宣布关闭期间以及关闭之后，无论设置如何，注册都视为 `closed`（[管理 CLI](admin-cli.md#announcing-the-end-of-the-service)）。在[单人服务器](instance-settings.md#one-person-server)上也是如此：那里同样不提供邀请、注册审查和注册屏蔽，并且没有任何方式可以创建账号，包括 `my admin create`。

```sh
my instance set registration_mode invite
```

| 模式 | 页面 | 注册后的状态 |
| --- | --- | --- |
| `open` | 任何人都可以注册；邀请码可选 | 直接获批；但在 `mail = none` 时会等待审核员批准（见下文）。邀请不改变任何东西，只记录是谁邀请了该用户 |
| `approval`（默认） | 还会询问“你为什么想加入？”（可选，最多 1,000 个字符）；邀请码可选 | 等待审批；有可用的邀请时直接获批 |
| `invite` | 必须填写邀请码 | 直接获批 |
| `closed` | 只显示不接受注册 | 无 |

- 除 `closed` 以外的所有模式下，页面上都有邀请码栏，会从邀请链接（`?invite=<code>`）中自动填入。无效的邀请码（未知、已撤销、已过期、已用完，或者来自不能再邀请的成员）在所有模式下都会被拒绝而不是被忽略，以免输错的邀请码无人察觉。
- 需要审批的注册屏蔽即使有邀请，也会让注册处于等待审批状态（[注册屏蔽](#sign-up-blocks)）。
- 如果为决策钩子 [`signup.check`](decision-hooks.md#sign-ups) 设置了插件，没有邀请的注册在通过 CAPTCHA 和屏蔽检查之后，还会由该插件审查，插件可以将其转为等待审批或拒绝。

- 来自同一网络（IP 地址；IPv6 为 `/64`）的注册限制为每 30 分钟 5 次。超出后，服务器返回带有 `Retry-After` 的 `429`。
- 如果用户名或邮箱地址已被占用，页面会在该栏显示错误，并且不会写入任何内容（邀请也不会被消耗）。
- 如果你设置了[外部登录提供方](login-providers.md)（OpenID Connect、GitHub、Discord 或 Mastodon），并允许新用户通过它注册，登录和注册页面会为每个已启用的提供方显示一个按钮，按钮上的名称与你输入的提供方显示名称完全一致（GitHub 提供方显示“使用 GitHub 登录”，Discord 提供方显示“使用 Discord 登录”，以访客的语言显示）。在该提供方的账号尚未关联到本服务器用户的人，会看到同样的注册表单，只是没有密码栏，新用户只能通过该提供方登录。注册模式、邀请码、加入理由、CAPTCHA、注册屏蔽、`signup.check` 和网络限制都与密码表单一样适用；在 `closed` 模式下，或者提供方不允许注册时，登录页面会显示没有关联的账号。提供方标记为已验证的地址（`email_verified`；GitHub 为已验证的主要地址；Discord 为 Discord 标记为已验证的地址），如果用户保留该地址，会立即确认；其他地址，以及通过 Mastodon 服务器注册的用户的所有地址（Mastodon 不提供地址），按下文通过邮件确认。与现有用户相同的邮箱地址绝不会关联到该用户：表单会显示该地址已被占用。启用了双重验证的用户通过提供方登录后，仍需输入验证码。
  GitHub 提供方会向 GitHub 请求 `user:email` 作用域；如果你列出了允许的 Organization，还会请求 `read:org`，之后服务器在每次登录时都会检查用户是否属于其中之一，不属于则拒绝登录。请把 `https://<host>/auth/external/<provider ID>/callback` 注册为你的 GitHub OAuth 应用的回调 URL。
  Discord 提供方会向 Discord 请求 `identify` 和 `email` 作用域；如果你列出了允许的服务器，还会请求 `guilds`，之后服务器在每次登录时都会检查用户是否属于其中之一。请在你的 Discord 应用中注册同样的回调 URL。
- 已登录的用户可以在设置的“关联的账号”中关联某个提供方的账号，需要先用密码、通行密钥或另一个已关联的账号确认，并且只限允许关联的提供方。浏览器必须以同一用户登录了登录页面；否则服务器会要求先登录。已被其他用户关联的提供方账号会被拒绝，每个用户在每个提供方最多关联一个账号。用户可以取消关联某个账号，除非它是最后一种登录方式（没有密码、没有通行密钥，也没有其他已关联的账号）。
- 没有密码也没有通行密钥的用户，在进行原本需要它们确认的操作（设置密码、添加通行密钥、删除或迁移账号、关联另一个账号）时，通过在已关联的提供方重新登录来确认。
  OpenID Connect 提供方会被要求重新验证用户（`max_age=0` 和 `prompt=login`），之前的验证会被拒绝。该确认只能使用一次，只对该操作有效，期限为 10 分钟；用非关联的账号登录会计为一次错误尝试，与输错密码共用同一个限制。
- 每次注册和登录时，服务器都会记录客户端 IP 地址（登录时还会记录 User-Agent）。这些记录会在 180 天后自动删除（实例设置 `session_origins_retention_days` 和 `registration_origins_retention_days`，至少 90 天）。处于保留冻结状态的账号的记录会被保留（[记录保留与冻结](moderation.md#record-retention-and-holds)）。请在你的隐私政策中写明这一保留期限（[隐私政策模板](privacy-policy-template.md)）。

<!-- source: when-a-user-can-use-the-app 317d6d8b7688 -->
## 用户何时可以使用应用

等待审批、被拒绝或尚未确认邮箱地址的用户，仍然可以在登录页面登录。自己停用了账号的用户在账号删除日期之前也可以登录：他们会在那里看到该日期，并可以重新启用或删除账号（[审核](moderation.md#accounts-that-their-owners-deactivated)）。登录后，他们会看到自己在等待什么（审核员批准或邮箱地址确认），或者看到注册未获批准，并且无法继续操作。只有获批的用户才能使用应用（客户端 API 的 OAuth 授权和令牌签发）；在要求邮箱确认的配置下，还必须已确认邮箱地址。

能够立即使用应用的新用户，会从注册页面直接进入应用，来到要求设置昵称和头像的欢迎步骤。之后首页时间线会显示入门清单和推荐关注的人（[推荐](instance-settings.md#suggestions-of-accounts)）。在确认邮箱地址的页面上选择“继续”，也会进入同样的欢迎步骤。

用 `my admin create` 创建的管理员视为已获批并已确认。

<!-- source: email-confirmation 6c1b7759c2cf -->
## 邮箱确认

设置 `mail`（[配置参考](../reference/configuration.md#mail)）决定邮件的发送方式。
SMTP 示例、DNS 设置以及如何解读投递失败，见[发送电子邮件](email.md)。

- `mail = smtp`：服务器在注册时发送确认邮件。链接有效期为 24 小时，只能使用一次。打开链接会显示一个带有确认按钮的页面，用户按下按钮后地址才会被确认（这样，预先抓取邮件中链接的扫描程序就不会消耗令牌）。登录后，用户每小时最多可以要求重发 3 次邮件。与 SMTP 服务器的连接始终用 STARTTLS 加密，服务器会在验证身份和发送之前校验证书。它从不向不提供 STARTTLS 的服务器发送邮件。
- `mail = none`（默认）：服务器不发送任何邮件，而是把 `mail not sent (mail = none)` 连同消息类别写入服务器日志。只有在 `profile = development` 时，该行才会同时包含地址和正文（包括确认链接）。由于邮件永远到不了用户，服务器会改为要求审核员批准，而不是邮箱确认。因此 `open` 模式下的注册也会从等待审批开始，未确认的邮箱地址不会妨碍使用应用。

除了这条开发用的日志行之外，确认令牌不会被记录在任何地方（访问日志不记录查询字符串）。数据库只存储令牌的 SHA-256。

<!-- source: registration-limited-to-login-providers 0d05572b3521 -->
## 注册仅限登录提供方

当实例设置 `login_providers_only` 开启且至少启用了一个[登录提供方](login-providers.md#limit-sign-in-to-login-providers)时，注册页面没有密码表单，只提供允许注册的提供方；链接中的邀请码会带到提供方的注册表单中。无论表单中填写了什么，用密码注册都会被拒绝。如果没有允许注册的提供方，注册视为 `closed`。如果没有已启用的提供方，该设置不生效。

<!-- source: password-reset-and-change bec2baef5ab7 -->
## 密码重置与更改

当实例设置 `login_providers_only` 生效时，重置链接以及设置或更改密码都会被拒绝（[该限制的作用](login-providers.md#limit-sign-in-to-login-providers)）。

- `mail = smtp`：用户在登录页面的“忘记密码？”中输入邮箱地址后，服务器会向以该地址为主要地址的用户发送设置新密码的链接。无论账号是否存在，页面的回答都相同。链接有效期为 1 小时，只能使用一次；如果在此期间密码通过其他途径更改，链接就会失效。请求限制为每个客户端 IP 地址每 30 分钟 5 次，邮件限制为每个地址每小时 3 封（超出限制的请求不会发送任何内容，但得到同样的回答）。启用 CAPTCHA 时，该表单也会经过验证。
- `mail = none`：页面会说明无法发送链接，并让用户联系管理员。管理员在管理页面中该账号的“管理”页面上，或者用 `my admin account password-reset-link @alice` 签发链接，再交给用户（[审核](moderation.md)）。
- 已登录的用户可以在设置的“密码”中输入当前密码来更改密码（每个用户每小时最多输错 5 次）。
- 重置或更改密码会撤销该用户的其他会话和应用令牌（在设置中更改时，只保留进行更改的那个应用）。通行密钥和双重验证保持不变。等待输入双重验证码的登录，以及尚未完成的应用授权，也会失效：用户需要重新开始。在 `mail = smtp` 时，服务器还会给用户发邮件，告知密码已更改。

<!-- source: invite-codes 5f98f9eb7d6f -->
## 邀请码

邀请在 `open`、`approval` 和 `invite` 模式下有效。员工邀请通过 CLI、审核页面和下文的管理 API 创建；成员也可以创建自己的邀请（[成员邀请](#member-invites)）。在 `closed` 模式下，无法创建邀请，所有邀请也都无效。

```sh
my invite create                          # single use, expires in 7 days
my invite create -max-uses 10 -expires-in 72h
my invite create -max-uses 0 -expires-in 0  # no limit on uses or time
my invite list                            # newest first; states: usable / revoked / expired / used up
my invite revoke <code>
```

- `create` 会输出邀请码（16 个字符）以及已填入邀请码的注册页面链接（`https://<host>/auth/sign_up?invite=<code>`）。
- 匹配邀请码时忽略大小写、空格和连字符。
- 即使多人同时注册，邀请码的使用次数也绝不会超出限制（`invite_redemptions` 上的唯一约束）。
- 已撤销、已过期和已用完的邀请码都返回同样的错误（邀请不可用）。
- 除了 CLI，你还可以在 Web 客户端的审核页面（“邀请”，`/moderation/invites`）中，以及通过客户端 API
  （`GET` / `POST /api/v1/admin/invites` 和 `POST /api/v1/admin/invites/{id}/revoke`）创建、列出和撤销邀请。两者都需要角色权限 `manage_invites`
  （内置的 `moderator` 没有该权限，因此要委派这项工作，请创建拥有该权限的角色并授予它）。列表上的“创建邀请”会打开表单（`/moderation/invites/new`），表单会显示新邀请的邀请码和注册链接。在列表中可以复制可用邀请的链接并撤销它。
- 列表显示所有邀请（员工的和成员的），以及是谁创建的（`created_by`）和以何种方式创建的（`creator_kind`：`staff` 或 `member`）。拥有 `manage_invites` 的人可以撤销其中任何一个。
- 审核页面中该账号的“审查”页面（`GET /api/v1/admin/accounts/{id}`，权限 `manage_accounts`）会显示该账号注册时使用的邀请及其创建者，也就是邀请该用户的人。
- 无论是在页面、API 还是 CLI 中，创建和撤销邀请都会记录在审计日志中（`invite_created`、`invite_revoked`）。目标名称是邀请码的前 4 个字符（`ABCD…`）；完整的邀请码不会复制到审计日志中。撤销已撤销的邀请不会改变任何东西，因此不会被记录。

<!-- source: member-invites db3bff74ed69 -->
## 成员邀请

除员工外，成员也可以创建自己的邀请。谁可以创建由实例设置 `invite_creators` 决定（[实例设置](instance-settings.md)）：

- `permitted`（默认）：只有角色拥有 `create_invites` 权限的用户。默认只有内置的 `admin` 角色拥有该权限。你可以把它加到手动授予的角色中，或者加到[支持者方案](supporters.md)的角色中，让支持者可以邀请他人。
- `everyone`：所有已登录的本站用户。

无论哪种方式，账号被封禁或受限、或者注册未获批准的用户都无法创建邀请；注册模式为 `closed` 时，任何人都无法创建。拥有 `manage_invites` 的人仍可以继续创建员工邀请，下文的限制都不适用于员工邀请。

成员可以在 Web 客户端的设置（“你的邀请”，`/settings/invites`）中，或者通过客户端 API（`GET` / `POST /api/v1/invites` 和 `POST /api/v1/invites/{id}/revoke`），创建、列出、复制和撤销自己的邀请。设置中只会为当前可以创建邀请的用户列出该页面；不能再创建邀请的用户仍可以通过地址打开它，复制和撤销自己创建的邀请。列表会显示每个邀请是可用、已撤销、已过期、已用完，还是因为创建者不能再邀请而不可用。

以下限制同样是实例设置，只适用于成员邀请。管理员可以在服务器设置的“注册”页面（`/admin/settings/registration`）上的“成员邀请”中更改它们以及 `invite_creators`：

| 设置 | 含义 | 默认值 | 范围 |
| --- | --- | --- | --- |
| `member_invite_max_active` | 一名成员同时可以持有多少个可用的邀请（未撤销、未过期、未用完） | `5` | 0-100（0：成员无法创建邀请） |
| `member_invite_max_uses` | 一个邀请最多可供多少人注册 | `10` | 1-1,000 |
| `member_invite_max_lifetime_seconds` | 一个邀请最长有效多久，以秒为单位 | `2592000`（30 天） | 3,600（1 小时）到 31,536,000（365 天） |

- 成员在限制范围内选择使用次数和有效期；不选择时，邀请允许 1 次注册，有效期 7 天（限制更低时则更短）。无法创建没有使用次数或时间限制的邀请。
- 降低限制不会改变现有的邀请；它从下一个创建的邀请起生效。
- 每次使用成员邀请注册时，都会检查其创建者是否仍能创建邀请。如果创建者失去了 `create_invites`（例如其方案到期）、`invite_creators` 改回了 `permitted`，或者账号被封禁、受限或删除，该邀请就会失效，注册页面会显示它无效。邀请本身不会被更改，因此在有效期内，当创建者重新可以邀请时，它会再次生效。
- 成员的创建和撤销会以 `invite_created` 和 `invite_revoked` 记录在审计日志中，执行者为该成员。

在 `approval` 模式下，可用的邀请会让新用户直接获批：邀请人代替了审核员的审查。因此在 `invite_creators = everyone` 时，每个用户都可以不经审查放人进来，这会削弱审批模式的作用。如果你依赖审查，请保持 `permitted`，并只把 `create_invites` 授予你信任的人，或者把 `member_invite_max_active` 和 `member_invite_max_uses` 设得较低。

<!-- source: approving-and-rejecting e68676f67aab -->
## 批准与拒绝

```sh
my registration list                      # waiting for approval (longest wait first); shows the reason and whether the email is confirmed
my registration list -state rejected
my registration approve alice
my registration reject -note "spam" bob
```

- 参数是本站账号的用户名（开头的 `@` 可以省略）。
- 只能对等待审批的注册作出决定。对于已作出决定的注册，或者不经注册页面创建的用户（`my admin create`），命令不会改变任何东西，并以代码 1 退出。
- 退出代码 2 表示参数错误。

同样的决定也可以在管理页面（客户端 API）中作出。所有操作都需要 `manage_accounts` 权限。

| 操作 | 作用 |
| --- | --- |
| `GET /api/v1/admin/registrations` | 等待审批的注册（等待最久的在前）。返回注册的 ID、加入理由、邮箱地址及其是否已确认，以及本站账号 |
| `POST /api/v1/admin/registrations/{id}/approve` | 批准。正文 `{"note": "..."}` 可选。回答中以 `decision_note` 显示备注；对已作出决定的注册，回答中不含邮箱地址 |
| `POST /api/v1/admin/registrations/{id}/reject` | 拒绝。正文与 `approve` 相同 |

- 这与 CLI 是同一个用例，会把决定记录在 `registration_decisions` 和审计日志中（`registration_approved`、`registration_rejected`）。CLI 作出的决定，执行者为 `cli`；API 作出的决定，执行者为作出决定的审核员。
- 对已作出决定的注册再次作出决定不会改变任何东西，并返回当前状态（如果两个人同时作出决定，只会记录一个）。不经注册页面创建的用户返回 404。

<!-- source: sign-up-blocks b70eb18cd52f -->
## 注册屏蔽

屏蔽规则按邮箱域名、邮箱地址或 IP 范围，拒绝来自注册页面的注册，或者将其转为需要审批。

```sh
my admin signup-block create -kind email_domain spam.example          # also applies to subdomains; a U-label works too
my admin signup-block create -kind ip_range -severity require_approval -duration 720h 192.0.2.0/24
my admin signup-block create -kind email_hash spammer@example.com     # only a hash of the address is kept
my admin signup-block list -active
my admin signup-block lift <block id>
```

- 匹配 `-severity reject`（默认）屏蔽规则的注册会被拒绝；匹配 `require_approval` 的注册无论注册模式如何，都会从等待审批开始。多条规则匹配时，适用更严格的那条。注册页面不会显示拒绝的原因。
- `email_hash` 与转为小写、并去掉本地部分中从第一个 `+` 起的部分后的地址进行匹配。IP 范围与注册 IP（与 `registration_origins` 相同的值）进行匹配，对 IP 未知的注册从不适用。
- 已过期和已解除的屏蔽规则不起作用。添加和解除都会记录在审计日志中（`signup_block_added`、`signup_block_lifted`）。
- 用 `-block-email`（API 中为 `block_email: true`）封禁本站账号时，服务器还会为该用户的邮箱地址创建一条 `email_hash` 屏蔽规则。

对于有数千个条目的列表（例如一次性邮箱服务的域名），可以用 `import` 从文件批量添加（`-` 表示标准输入）。所有值都使用相同的种类、严重程度、备注和期限。

```sh
my admin signup-block import -kind email_domain -comment "disposable email" disposable.txt
```

- 列表每行一个值（与 `add` 的 `<value>` 形式相同），或者是第一列为值的 CSV 行。空行和以 `#` 开头的行会被跳过。一次导入最多 10,000 个条目。
- 命令会输出添加了多少条，并对每个未添加的行输出行号、值和原因。原因有 `invalid`（不是该种类的有效值）、`duplicate`（与同一列表中较早的行规范形式相同）或 `already_blocked`（已有相同种类和值、且同等或更严格的屏蔽规则生效）。
- 每条添加的屏蔽规则都会和 `add` 一样记录在审计日志中（`signup_block_added`），并且可以逐条解除。
- 导入以每几百条为一个事务写入，因此中途失败时，之前添加的内容会保留。再次导入同一个列表只会添加剩余的部分。
- 本软件不附带任何列表，也不会定期从 URL 获取列表。公开列表（例如 GitHub 上的 disposable-email-domains）需要你自己获取和更新，并再次导入更新后的列表。

同样的操作也可以在管理页面（客户端 API）中进行。所有操作都需要 `manage_accounts` 权限。在 Web 客户端中，打开“审核”→“注册屏蔽”（`/moderation/signup-blocks`）。列表显示每条屏蔽规则的值、种类、状态和结束时间，可以只看生效中的或查看全部；“添加屏蔽规则”会打开表单（`/moderation/signup-blocks/new`），“从列表添加”会导入列表并显示跳过的行（`/moderation/signup-blocks/import`），选择某个值会打开该屏蔽规则的页面，显示其效果和备注，在那里选择“解除”即可结束它。

| 操作 | 作用 |
| --- | --- |
| `GET /api/v1/admin/signup_blocks` | 列出屏蔽规则（最新的在前）。可以用 `kind` 和 `active` 筛选 |
| `POST /api/v1/admin/signup_blocks` | 添加屏蔽规则。正文包含 `kind`、`value`、`severity`、`comment` 和 `expires_in`（秒，最长十年） |
| `POST /api/v1/admin/signup_blocks/import` | 从列表添加屏蔽规则。正文包含 `kind`、`text`（列表）、`severity`、`comment` 和 `expires_in`。响应包含 `added`（添加的数量）和 `skipped`（未添加的行：`line`、`value`、`reason`） |
| `GET /api/v1/admin/signup_blocks/{id}` | 返回一条屏蔽规则 |
| `POST /api/v1/admin/signup_blocks/{id}/lift` | 解除屏蔽规则。已解除的屏蔽规则保持原样 |
