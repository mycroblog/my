<!-- source: managing-the-key-encryption-key-kek 571fb6e3c6b4 -->
# 管理密钥加密密钥（KEK）

服务器用一把密钥加密密钥（KEK）加密它保存在数据库中的机密，而这把 KEK 本身从不存入数据库。本页介绍如何生成、保管、备份和轮换 KEK，如何改为把它放在 KMS 中，以及 KEK 丢失或泄露时该怎么办。所有设置都列在[配置参考](../reference/configuration.md)中。

<!-- source: how-it-works 37074c6237d0 -->
## 工作原理

![密钥层级：密钥加密密钥保存在 key_manager 指定的位置，从不进入数据库；它包裹为每个机密生成的数据加密密钥，再由数据加密密钥加密机密；数据库在五个加密列中保存密钥 ID、被包裹的数据加密密钥和加密后的机密。下面的列表逐层说明。](images/key-hierarchy.svg)

- 存入数据库的机密（例如 actor 的私钥）使用为每个机密生成的数据加密密钥（DEK），以 AES-256-GCM 加密。
  DEK 由**密钥加密密钥（KEK）**包裹后，和密文存放在一起（信封加密）。
- KEK 不存入数据库，因此仅凭泄露的数据库转储无法得到这些机密。
- 每段密文都带有包裹其 DEK 的 KEK 的**密钥 ID**。在 `key_manager = local` 时，密钥 ID 是 `local:` 加上由 KEK 推导出的 16 位十六进制数，因此同一把 KEK 总是得到同一个 ID。你无需分配 ID。
- 密文绑定到它所在的行 ID 和列名。把它复制到其他行或列后将无法解密。
- 加密列（名称与 `my kek status` 的输出一致）有：`local_account_private_keys.private_key_enc`（actor 的私钥）、
  `totp_enrollments.secret_enc`（双重验证的密钥）、`login_provider_secrets.sealed_secret`（登录提供方的客户端密钥）、
  `webhook_secrets.sealed_secret`（Webhook 签名密钥）和 `instance_secrets.sealed_secret`（媒体代理签名密钥）。
- 通行密钥登录或通过外部提供方登录时的状态也以同样方式加密，但不会存储：由浏览器保管，最长 30 分钟。
- 媒体代理 URL 的签名密钥（HMAC-SHA256）是服务器首次启动时生成的随机密钥，和其他机密一样加密后存入数据库。轮换 KEK 时它不会改变，因此媒体代理 URL 在轮换前后都能正常使用；`my kek rewrap` 会把它和其他机密一起重新包裹。
- `key_manager` 可以是 `local`（默认，使用 `kek` 设置）、`gcpkms`（KEK 保存在 Google Cloud KMS 中，参见 [Google Cloud KMS](#google-cloud-kms)）、
  `awskms`（KEK 保存在 AWS KMS 中，参见 [AWS KMS](#aws-kms)）或 `plugin`（KEK 由你自己的程序保管，参见[插件](#a-plugin)）。

<!-- source: what-happens-if-you-lose-the-kek 76126b09f09c -->
## 丢失 KEK 会怎样

没有 KEK，即使恢复了数据库也无法解密 actor 的私钥。私钥一旦丢失，其他服务器就无法再验证该 actor 的签名，
**该 actor 在联邦宇宙中的身份也随之失效**（关注者关系无法迁移）。请像对待数据库一样谨慎对待 KEK。

![丢失 KEK 后会失去什么：在没有 KEK 的情况下恢复后，所有未加密的内容仍可读取，所有加密的值都无法解开，actor 失去身份。下面的段落说明了同样的内容。](images/kek-loss.svg)

未加密的内容仍可读取：帖子、账号、关注关系和数据库中的其他内容，以及存储中的媒体文件。所有加密的值都会以 `unknown key-encryption key` 错误失败：actor 的私钥、双重验证的密钥、登录提供方的客户端密钥、Webhook 签名密钥和媒体代理签名密钥。如果设置了另一把 KEK 来代替包裹这些机密的那一把，或者包裹它们的 KMS 密钥或插件密钥被停用或销毁，也会发生同样的情况。

<!-- source: generate-a-kek 44b338b7440b -->
## 生成 KEK

将 32 个随机字节以标准 base64 编码。

```sh
openssl rand -base64 32
```

没有 `openssl` 时，也可以使用 `head -c 32 /dev/urandom | base64`。不要让生成的值留在终端历史或工单中。

<!-- source: store-and-deliver-it 9300565c2a3c -->
## 保管与传递

KEK 是机密设置，只能通过环境变量 `MY_KEK` 传入，或者通过 `MY_KEK_FILE` 传入保存该值的文件路径。把它写进配置文件（TOML）会导致无法启动。

| 部署目标 | 传递方式示例 |
| --- | --- |
| 自托管（二进制 / systemd） | 放进只有所有者可读的文件（`chmod 600`），并让 `MY_KEK_FILE` 指向它 |
| Docker / Compose | 把 Docker secret 挂载到 `/run/secrets/my_kek`，并设置 `MY_KEK_FILE=/run/secrets/my_kek`。`deploy/docker` 中的文件会从 `secrets/kek` 这样做（[Docker Compose 文件](running.md#docker-compose-files)）：轮换时替换该文件，并在 `.env` 中设置 `MY_KEK_PREVIOUS` |
| Kubernetes / microk8s | 把 Secret 挂载为文件，并让 `MY_KEK_FILE` 指向它 |
| Cloud Run | 把 Secret Manager 的 secret 以文件或环境变量的形式公开 |
| AWS ECS | 从 Secrets Manager 或 SSM Parameter Store（SecureString）传入 |

- 为所有进程（例如 web 和 worker）提供同一把 KEK。
- KEK 从不出现在日志或错误消息中（只会出现密钥 ID）。
- `my config check` 会校验 KEK 的格式（32 字节的 base64），并把值显示为 `(redacted)`。在 `production` 配置档中，对于不随机的 KEK（例如全为零或重复同一个字节），它还会报错，`my serve` 也会为此记录警告（参见[生产环境检查清单](production-checklist.md)）。

<!-- source: back-it-up 29e7bdb3b114 -->
## 备份

**分别备份数据库和 KEK。**

- 把 KEK 保存在与数据库备份不同的地方（密码管理器、离线介质、另一个账号中的机密存储等）。如果两者放在一处，那里一旦泄露，数据库加密就失去了意义。
- 轮换期间，只要数据库中还有用 `kek_previous` 中的密钥包裹的密文，就要把这些密钥也纳入备份。
- 把 KEK 纳入恢复流程，并定期测试恢复（[备份与恢复](backup.md#recovery-drill)）。恢复数据库后，把 KEK（以及 `kek_previous`）设置为备份时所用的同样的值。如果 KEK 不同，读取机密的操作都会以 `unknown key-encryption key` 错误失败，错误中会写明密钥 ID。

<!-- source: rotate-the-kek eb4af3189053 -->
## 轮换 KEK

`kek_previous` 中的 KEK 只用于解密，新机密总是用 `kek` 加密。滚动部署时新旧进程会同时运行；为了不产生某些进程无法读取的密文，需要分两步切换。

![本地 KEK 的五步轮换，以及每一步之后 kek 和 kek_previous 的值：新 KEK 先放进 kek_previous，然后和旧 KEK 互换位置，my kek rewrap 迁移已存储的机密，最后旧 KEK 从配置中移除。下面的步骤说明了同样的内容。](images/kek-rotation.svg)

1. 生成新的 KEK 并备份。
2. 以 `MY_KEK=<old KEK>` 和 `MY_KEK_PREVIOUS=<new KEK>` 重启所有进程。此时所有进程都能用新 KEK 解密。
3. 以 `MY_KEK=<new KEK>` 和 `MY_KEK_PREVIOUS=<old KEK>` 重启所有进程。从此机密都用新 KEK 包裹，旧 KEK 的密文仍可读取。
4. 用新 KEK 重新包裹已存储的机密：以与第 3 步相同的配置运行一次 `my kek rewrap`（参见[重新包裹已存储的机密](#re-wrap-the-stored-secrets)）。它以 0 退出后，数据库中就没有需要旧 KEK 的机密了。
5. 在第 3 步之后至少等待 30 分钟，让用旧 KEK 签发的短期登录令牌过期，然后以 `MY_KEK=<new KEK>` 且不设置 `kek_previous` 重启所有进程。只要你还保留第 4 步之前的数据库备份，就要保留旧 KEK 的备份：恢复这些备份时还需要它。

`kek_previous` 可以接受多把以逗号分隔的密钥。同一把密钥列出两次，或者列出与 `kek` 相同的密钥，都会导致无法启动。

<!-- source: re-wrap-the-stored-secrets 67af175f1d56 -->
### 重新包裹已存储的机密

`my kek rewrap` 只替换每个机密被包裹的数据加密密钥，密文保持不变；它从不写出或打印机密。

```sh
my kek status   # count the stored secrets by the key ID of the KEK that wraps them; decrypts nothing
my kek rewrap   # re-wrap every secret that a key of kek_previous wraps with kek
```

- `my kek status` 按列和密钥 ID 每组输出一行，用 `(current)` 标出 `kek` 的密钥 ID，最后说明是否还有机密需要 `kek_previous` 中的密钥。如果某个密钥 ID 既不是 `kek`，也不在 `kek_previous` 中，说明配置中缺少一把 KEK。
- `my kek rewrap` 以 100 个为一批读取机密，每批在一个短事务中写入。它按列输出重新包裹了多少个机密，以及多少个已经用 `kek` 包裹。它可以在服务器提供服务时运行，并且随时可以停止或再次运行：已经用 `kek` 包裹的机密会被跳过，期间被服务器改写过的机密则保持服务器写入的样子。
- 无法重新包裹的机密保持不变，并连同行 ID 一起报告，命令以 1 退出。原因要么是配置中没有它的 KEK
  （消息中会写明密钥 ID：把那把 KEK 加入 `kek_previous` 后再运行一次），要么是存储的值已损坏或是从其他行复制过来的。只要 `my kek status` 仍统计到某把 KEK 下的机密，就不要把它从 `kek_previous` 中删除。

<!-- source: google-cloud-kms 1c47dd463234 -->
## Google Cloud KMS

在 `key_manager = gcpkms` 时，由一把对称的 Cloud KMS 密钥（用途为 `ENCRYPT_DECRYPT` 的 CryptoKey）包裹数据加密密钥，KEK 从不离开 Cloud KMS。即使数据库转储和配置同时泄露，机密也不会暴露：解密需要使用该密钥的权限。

- 把 `kms_key` 设置为密钥的资源名称，不带版本：
  `projects/{project}/locations/{location}/keyRings/{ring}/cryptoKeys/{key}`。超过 248 字节的名称在启动时会被拒绝。已存储机密的密钥 ID 是 `gcpkms:` 加上这个名称。
- 服务器使用应用默认凭据（Application Default Credentials）进行身份验证：在 Cloud Run 上是该服务的服务账号；在其他环境中是 `GOOGLE_APPLICATION_CREDENTIALS`。向该服务账号授予密钥（而不是项目）上的 `roles/cloudkms.cryptoKeyEncrypterDecrypter`，供所有进程（例如 web 和 worker）以及 `my kek status`、`my kek rewrap` 命令使用。没有该角色时，读取机密会失败，错误中会写明该角色；密钥版本不存在或已停用时也同样失败，这些错误在修正配置之前不会重试。配额错误或 Cloud KMS 不可用属于临时问题，需要该机密的工作会重试。
- 服务器会把 Cloud KMS 解密的数据加密密钥保留 `kms_dek_cache_ttl`（默认 `10m`），因此大多数机密读取不会调用 Cloud KMS。
- 构建标签 `no_gcpkms` 会把该适配器排除在二进制文件之外（[构建标签](../reference/build-tags.md)）。

<!-- source: rotate-the-key-in-cloud-kms ba58f052af2a -->
### 在 Cloud KMS 中轮换密钥

Cloud KMS 中的轮换（手动进行，或为密钥设置轮换周期）会添加新的主版本，并保持旧版本启用。新机密用新版本包裹，服务器用包裹每个机密的那个版本来解密它，密钥 ID 不变，因此**轮换不需要 `my kek rewrap`**。

**只要还有机密需要旧版本，就不要停用或销毁它**：用已停用或已销毁的版本包裹的机密将无法再解密。
Cloud KMS 不会告诉你某个版本包裹了哪些机密，因此要淘汰密钥材料，需要先把机密迁移到新密钥：

1. 创建新密钥（可以在同一个密钥环中），并向服务账号授予该密钥上的角色。
2. 把 `kms_key` 设置为新密钥、`kms_key_previous` 设置为旧密钥，然后重启所有进程。此时旧密钥只用于解密。
3. 以相同的配置运行 `my kek rewrap`。当 `my kek status` 不再统计到旧密钥 ID 下的机密时，删除 `kms_key_previous` 并重启。
4. 此后才停用旧密钥的版本；只要你还保留第 3 步之前的数据库备份，就要让这些版本保持可恢复：恢复这些备份时还需要它们。

<!-- source: aws-kms f97406d91b4c -->
## AWS KMS

在 `key_manager = awskms` 时，由一把对称的 AWS KMS 密钥（密钥规格 `SYMMETRIC_DEFAULT`、密钥用途 `ENCRYPT_DECRYPT`、客户托管）包裹数据加密密钥，KEK 从不离开 AWS KMS。即使数据库转储和配置同时泄露，机密也不会暴露：解密需要使用该密钥的权限。

- 把 `kms_key` 设置为密钥的 ARN（`arn:aws:kms:{region}:{account}:key/{key-id}`）、别名（`alias/{name}` 或其 ARN）或密钥 ID。服务器在启动时把名称解析为密钥的 ARN，已存储机密的密钥 ID 是 `awskms:` 加上该 ARN。因此，把别名指向另一把密钥，在下次启动之前不会产生任何影响，启动之后也只影响新机密：已存储的机密仍指向旧密钥，所以要把旧密钥设置到 `kms_key_previous` 中（参见[淘汰 AWS KMS 密钥](#retire-an-aws-kms-key)）。
  AWS 托管密钥（`alias/aws/...`）会被拒绝；只有拥有它的 AWS 服务才能使用。
- 服务器使用 AWS SDK 的默认凭据链进行身份验证：在 ECS 上是任务的角色；在 EC2 上是实例配置文件；在其他环境中是环境变量（`AWS_ACCESS_KEY_ID` 等）或共享配置文件。区域取自密钥的 ARN；使用别名或密钥 ID 时，请设置 `AWS_REGION`。
- 为所有进程（例如 web 和 worker）以及 `my kek status`、`my kek rewrap` 命令允许该密钥上的 `kms:Encrypt`、`kms:Decrypt` 和 `kms:DescribeKey`。IAM 角色的策略示例：

  ```json
  {
    "Effect": "Allow",
    "Action": ["kms:Encrypt", "kms:Decrypt", "kms:DescribeKey"],
    "Resource": "arn:aws:kms:us-east-1:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab"
  }
  ```

  即使 `kms_key` 是别名，资源也要写密钥的 ARN。密钥策略还必须允许账号的 IAM 策略授予访问权限（默认密钥策略允许）。
- 当密钥不存在、已停用或等待删除，或者不是对称加密密钥时，服务器无法启动。之后，缺少权限或密钥被停用会使机密读取失败，错误中会写明原因，这些错误在修正配置之前不会重试。限流（`ThrottlingException`）或 AWS KMS 不可用属于临时问题，需要该机密的工作会重试。
- 每次调用的加密上下文包含机密的行 ID 和列名（`{"my-aad": ...}`，以 base64url 编码）。CloudTrail 会以明文记录它；其中不含机密。
- 费用：AWS 对每把客户托管密钥按月收费（撰写本文时为 1 美元，其密钥材料的前两次轮换各另收同样的费用），超出每月免费额度的请求按次收费（参见 [AWS KMS 定价](https://aws.amazon.com/kms/pricing/)）。服务器会把 AWS KMS 解密的数据加密密钥保留 `kms_dek_cache_ttl`（默认 `10m`），因此一台有 10 个持续投递的 actor 的服务器，每个进程每月大约发出 43,000 次请求，超出免费额度的费用只有几美分。
- 构建标签 `no_awskms` 会把该适配器排除在二进制文件之外（[构建标签](../reference/build-tags.md)）。

<!-- source: retire-an-aws-kms-key b670091ad7f1 -->
### 淘汰 AWS KMS 密钥

AWS KMS 的自动密钥轮换会保留密钥的 ARN 及其之前的密钥材料，因此密钥 ID 不变，**轮换不需要 `my kek rewrap`**。

要停止使用某把密钥，或者把别名指向另一把密钥，需要先把机密迁移到新密钥。**只要还有机密需要旧密钥，就不要停用它或安排删除它**：用它包裹的机密将无法再解密。

1. 创建新密钥，并允许角色使用它。
2. 把 `kms_key` 设置为新密钥、`kms_key_previous` 设置为旧密钥（写它的 ARN，而不是别名），然后重启所有进程。此时旧密钥只用于解密。
3. 以相同的配置运行 `my kek rewrap`。当 `my kek status` 不再统计到旧密钥 ID 下的机密时，删除 `kms_key_previous` 并重启。
4. 此后才停用旧密钥；只要你还保留第 3 步之前的数据库备份，就要保留它：恢复这些备份时还需要它。

<!-- source: a-plugin 43023e60bac9 -->
## 插件

在 `key_manager = plugin` 时，由你自己的[插件](plugins.md)保管 KEK（放在 Vault、HSM 或它选择的任何地方），并为服务器包裹数据加密密钥。

**插件与 KEK 处于同一信任边界内。** 它能看到每一把明文的数据加密密钥，因此控制它或它所在主机的人，可以读取服务器的所有机密，包括 actor 的私钥。请像保护 KEK 一样保护它。这也是为什么只有配置文件和环境变量能更改它的地址，其他任何设置都不行。

```sh
MY_KEY_MANAGER=plugin
MY_PLUGIN_KEY_MANAGER_ADDRESS=unix:///run/my/key_manager.sock
```

- 地址使用 `unix://` 或 `https://`。服务器会拒绝 `http://`，即使在回环地址上也是如此，因为密钥会以明文传输。目标的其他设置见[插件](plugins.md#the-destination)。
- 在让服务器使用某个插件之前，先运行 `my plugin test --kind key_manager <address>`（[测试插件](plugins.md#test-a-plugin)）。它只包裹自己临时生成、用后即弃的密钥。
- 已存储机密的密钥 ID 是 `plugin:` 加上插件为其 KEK 给出的 ID，该 ID 是 1 到 248 字节的可见 ASCII 字符。
- **保留已存储机密仍在引用的每一把 KEK。** 插件中的轮换会在下一次包裹时体现为新的 ID。之前包裹的机密仍引用旧 ID，插件必须仍能解开它们。`my kek rewrap` 会把它们迁移到当前 ID，`my kek status` 会统计剩余的数量。
- 插件和 KMS 不能同时使用：使用插件时会拒绝 `kms_key` 和 `kms_key_previous`，在 `key_manager = gcpkms` 或 `awskms` 时也会拒绝 `plugin_key_manager_address`。
- 构建标签 `no_plugins` 会把该适配器排除在二进制文件之外（[构建标签](../reference/build-tags.md)）。

<!-- source: when-the-server-starts 5faeda92e43b -->
### 服务器启动时

每个进程，以及每个会打开数据库的 `my` 命令（例如 `my kek status`），都会在做任何其他事情之前执行以下操作。这是唯一一种失败会阻止启动的插件。

1. 向插件询问其当前 KEK 的 ID。
2. 用一把用后即弃的随机密钥进行自检：包裹后再解包必须得到原来的密钥；用不同的认证数据解包必须返回 `OUTCOME_DECRYPT`；对同一把密钥包裹两次的结果必须不同；包裹后的值中不能包含该密钥。

插件没有响应或返回 `503` 时，会在最长 30 秒内重试，以应对比服务器晚启动的 sidecar。超过之后，进程以错误退出。错误的回答以及其他任何失败（例如令牌被拒绝）会立即使其停止：自检正是为了拦下忽略认证数据或直接透传密钥的插件，而等待并不能修好这样的插件。

<!-- source: when-the-plugin-is-unreachable 71bf24801c2b -->
### 插件无法访问时

服务器以失败关闭（fail closed）的方式处理。无论出了什么问题，无论是调用被拒绝或超时，还是回答为空、未知或格式错误，需要该密钥的操作都会以错误失败。它从不以明文存储机密，从不切换到本地 KEK 或其他密钥，也从不把失败当作结果。以这种方式失败的任务会稍后重试，请求则对发起它的人返回失败。每次失败的调用都会按种类和 Connect 代码计入 [`my.plugin.failures`](running.md)，日志中不含密钥、认证数据或包裹后的值，也不含插件返回的任何内容。

包裹操作从不缓存，刚刚加密的值在缓存中也没有记录，因此以下操作会立即停止：

- 创建账号，因为要生成 actor 的密钥。
- 设置双重验证，以及注册通行密钥或用通行密钥登录，因为这些流程会把状态加密保存。
- 通过外部提供方登录，以及保存提供方的客户端密钥。
- 注册 Webhook，以及 `my kek rewrap`。

读取已存储的机密，只有在进程仍持有之前读取时得到的数据加密密钥时才能进行，最长为 `kms_dek_cache_ttl`（默认 `10m`）。超过这个时间，或者对于尚未读取过的机密，以下操作也会停止：

- 联邦投递和签名获取，因为需要 actor 的私钥。插件恢复后，投递会通过任务的重试追上进度。
- 登录时校验双重验证码。
- Webhook 投递。

重启会如上所述停止，因此**不要为了应对插件的故障而重启服务器**：正在运行的服务器会继续提供缓存所覆盖的服务，而重启后的服务器在插件响应之前什么也无法提供。不需要机密的操作（例如阅读时间线和发帖）会继续进行。媒体代理的签名密钥在启动时读取，并一直保留在内存中。更新客户端证书或 CA 文件也不需要重启：文件变化后，服务器会重新读取它们（[插件](plugins.md#authentication)）。

<!-- source: the-contract 6183eb7a7dfa -->
### 协议约定

RPC 为 `my.plugin.key_manager.v1.KeyManagerService` 中定义的那些，默认截止时间为 5 秒，每个方向的消息最大 128 KiB。

```text
POST /my.plugin.key_manager.v1.KeyManagerService/Describe
{}
{"outcome":"OUTCOME_OK","keyId":"transit/my-kek-3"}

POST /my.plugin.key_manager.v1.KeyManagerService/WrapKey
{"dek":"<base64>","aad":"<base64>"}
{"outcome":"OUTCOME_OK","keyId":"transit/my-kek-3","wrapped":"<base64>"}

POST /my.plugin.key_manager.v1.KeyManagerService/UnwrapKey
{"keyId":"transit/my-kek-3","wrapped":"<base64>","aad":"<base64>"}
{"outcome":"OUTCOME_OK","dek":"<base64>"}
```

- `keyId` 是插件自己的 ID，不带 `plugin:`。`UnwrapKey` 可能指定比当前 ID 更早的 ID。
- `wrapped` 为 1 到 65,535 字节，且不能包含密钥本身。对同一把密钥包裹两次的结果必须不同。
- `aad` 是包含行 ID 和列名的认证数据，除非它与 `WrapKey` 收到的一致，否则 `UnwrapKey` 必须失败。正是这一点阻止了把机密复制到其他行。
- **绝不要记录 `dek`、`aad` 或 `wrapped`。**

| `outcome` | 含义 |
| --- | --- |
| `OUTCOME_OK` | 调用成功 |
| `OUTCOME_UNKNOWN_KEY` | `UnwrapKey`：插件没有 `keyId` 对应的 KEK |
| `OUTCOME_DECRYPT` | `UnwrapKey`：`wrapped` 或 `aad` 无法通过认证 |

无法回答时（例如密钥存储无法访问），请返回 HTTP 或 Connect 错误。此时不要返回 `OUTCOME_DECRYPT` 或 `OUTCOME_UNKNOWN_KEY`：对服务器来说，它们表示值本身有问题。服务器自己不会重试调用，因此插件可能会从任务那里再次收到同一个调用。

<!-- source: move-between-the-local-kek-and-a-kms 81a909a14c05 -->
## 在本地 KEK 与 KMS 之间迁移

密钥 ID 以包裹它的密钥管理器的种类开头（`local:`、`gcpkms:`、`awskms:`、`plugin:`）。服务器用当前的 `key_manager` 包裹新机密，并用每个已存储机密的密钥 ID 所指的密钥管理器来解包，因此在本地 KEK 与 KMS 之间迁移，就是一次跨种类的轮换。

- 使用 KMS 时，`kek` 和 `kek_previous` 只用于解密：它们解开仍由本地 KEK 包裹的机密，所有新机密都用 `kms_key` 包裹。
  `kms_key_previous` 列出同一 KMS 中更早的密钥，它们也只用于解密。
- 在 `key_manager = local` 时，`kms_key` 和 `kms_key_previous` 如果已设置，也只用于解密：它们解开仍由 KMS 包裹的机密。
- 从本地 KEK 迁移到 KMS：
  1. 按照 [Google Cloud KMS](#google-cloud-kms) 或 [AWS KMS](#aws-kms) 中的说明创建密钥并授予权限。
  2. 以 `key_manager = gcpkms` 或 `awskms`、把 `kms_key` 设置为该密钥，并把当前 KEK 从 `kek` 移到 `kek_previous`，重启所有进程。此后机密由 KMS 包裹，由本地 KEK 包裹的机密仍能解开。
  3. 以相同的配置运行 `my kek rewrap`。
  4. 当 `my kek status` 不再统计到 `local:` 密钥 ID 下的机密时，删除 `kek_previous` 并重启。只要你还保留第 3 步之前的数据库备份，就要保留 KEK 的备份。
- 从 KMS 迁回本地 KEK 时，反过来做同样的事：设置 `key_manager = local` 和新的 `kek`，并保留 `kms_key`（此时只用于解密），直到 `my kek rewrap` 迁移完所有机密，且 `my kek status` 不再统计到 `gcpkms:` 或 `awskms:` 密钥 ID 下的机密。此时 `kms_key` 和 `kms_key_previous` 中的密钥必须都属于同一个 KMS。
- 要从一家云的 KMS 迁移到另一家的 KMS，需要先迁移到本地 KEK，再迁移到另一个 KMS；不能直接迁移。
- 插件也以同样的方式迁移，并且只能经由本地 KEK：
  - 迁移到插件时，以 `key_manager = plugin`、`plugin_key_manager_address`，并把当前 KEK 从 `kek` 移到 `kek_previous`，重启所有进程，然后运行 `my kek rewrap`。当 `my kek status` 不再统计到 `local:` 密钥 ID 下的机密时，删除 `kek_previous` 并重启。
  - 从插件迁出时，以 `key_manager = local`、新的 `kek`，并保留 `plugin_key_manager_address`（此时只用于解密），重启所有进程，然后运行 `my kek rewrap`。当 `my kek status` 不再统计到 `plugin:` 密钥 ID 下的机密时，删除该地址并重启。在此之前，插件仍必须通过[启动时的检查](#when-the-server-starts)，其中包括一次包裹。
- 服务器会把 KMS 或插件解密的数据加密密钥保留 `kms_dek_cache_ttl`（默认 `10m`，最多 `kms_dek_cache_entries` 个，默认 `1024`），因此正在使用的机密在每个周期、每个进程中大约只访问它一次。任一设置为 `0` 都会禁用缓存。

<!-- source: if-the-kek-leaks be6e788e4cea -->
## KEK 泄露时

如果只有 KEK 泄露，按上述方法轮换即可。如果 KEK 和数据库都可能已经泄露，轮换就无法保护你：数据库中 actor 的私钥已经可以被读取，因此必须重新生成 actor 密钥本身。

<!-- source: web-push-vapid-key aa32321d1e4a -->
## Web Push 的 VAPID 密钥

Web Push 用服务器的 VAPID 密钥（P-256，RFC 8292）签名。它是独立于 KEK 的机密，不存入数据库。

- 用 `my config vapid-key` 创建密钥对，并通过 `MY_VAPID_PRIVATE_KEY` 或 `MY_VAPID_PRIVATE_KEY_FILE` 把私钥（`vapid_private_key = ...` 的值）提供给所有进程。服务器由私钥推导出公钥并交给客户端，因此你不需要配置公钥。
- 如果不设置，Web Push 会被禁用，客户端也不会提供它。在 `production` 配置档中，`my config check` 和 `my serve` 启动时会对此发出警告。
- 浏览器会把订阅绑定到公钥，因此更换密钥会停止向现有订阅投递。用户在设置中重新开启推送通知后，客户端会用新密钥重新订阅。除非密钥泄露，否则不要更换；如果泄露了，请切换到新密钥，并请用户重新开启通知。
