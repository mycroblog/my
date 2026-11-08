<!-- source: external-login-providers 48a582ab87ec -->
# 外部登入服務

外部登入服務讓使用者可以用他們在其他地方的帳號登入：OpenID Connect 服務（Google Workspace、Microsoft Entra ID、Keycloak 等）、GitHub、Discord，或一個 Mastodon 伺服器。你可以在擁有伺服器設定的主機上使用 `my login-provider` 指令設定登入服務，或透過管理 API（`/api/v1/admin/login_providers`）以 `manage_login_providers` 權限設定，而只有內建的 `admin` 角色擁有這個權限。這個指令在任何管理員能登入之前就能使用，所以可以用來進行初次設定。在 Web 用戶端中，擁有該權限的管理員可以使用管理選單中的「登入服務」。清單會顯示每個服務的類型、狀態，以及是否允許註冊；「新增服務」會開啟建立服務的表單。選擇某個服務會開啟它的頁面，頁面上顯示它的回呼 URL（附有複製按鈕）與已連結的使用者人數，你也可以在這裡變更它的設定、設定新的用戶端密鑰，以及啟用、停用和刪除它。

<!-- source: what-external-sign-in-does 25fa37e22a50 -->
## 外部登入會做什麼

- 登入服務只能證明某人在該服務上的身分。帳號絕不會依電子郵件地址連結到使用者：使用者要在設定的「已連結的帳號」中連結帳號，或用它註冊成為新使用者。與現有使用者相符的電子郵件地址絕不會連結到該使用者。
- 已連結的使用者透過登入服務登入後，仍然要輸入兩步驟驗證碼（TOTP），而帳號不是啟用狀態的使用者無法登入。
- 只有當你為該服務允許註冊（`-allow-signup`），而且[註冊模式](registration.md)允許註冊時，新使用者才能透過登入服務註冊；邀請碼、CAPTCHA、註冊封鎖與網路限制都和密碼表單一樣適用。用這種方式註冊的人沒有密碼，只能透過該服務登入。[註冊流程圖](registration.md#from-sign-up-to-the-app)並列呈現了這兩種註冊方式。

<!-- source: kinds c9123566b5c5 -->
## 類型

| 類型 | `-issuer` | 條件 |
| --- | --- | --- |
| `oidc` | 簽發者 URL（`https`），與該服務的 ID 權杖中的寫法完全相同，例如 `https://accounts.google.com` | `-claim <claim>=<value>`：只允許 ID 權杖中該 claim 的值為所指定值之一的使用者 |
| `github` | 無：伺服器已知 github.com 的端點 | `-org <organization>`：只允許其中一個 Organization 的成員（需加上 `-scope read:org`） |
| `discord` | 無：伺服器已知 discord.com 的端點 | `-guild <server ID>`：只允許其中一個 Discord 伺服器（guild）的成員；未指定時，所有 Discord 使用者都能進入 |
| `mastodon` | 該伺服器的 Web 網域，例如 `mastodon.example`（Mastodon 4.3 或更新版本） | 無 |

若使用 Google Workspace，請以 `-claim hd=<your domain>` 把使用者限制在你的組織內。不要依賴電子郵件地址的網域。

<!-- source: examples 0d19ab463197 -->
## 範例

<!-- source: google b99294b8539d -->
### Google

在 Google Cloud 主控台中建立類型為「網頁應用程式」（Web application）的 OAuth 用戶端。若要只限你的組織使用，請把 OAuth 同意畫面的使用者類型設為「內部」（Internal；專案隸屬於 Google Workspace 組織時才能選擇），並且也加上 `hd` 條件：使用者類型限制的是 Google 允許誰進入，而這個條件則讓本伺服器檢查每個 ID 權杖的 `hd` claim。

```sh
printf '%s\n' "$CLIENT_SECRET" | my login-provider create -kind oidc -name Google \
  -issuer https://accounts.google.com -client-id 123.apps.googleusercontent.com -claim hd=example.com
```

<!-- source: keycloak-and-other-openid-connect-providers 0840dc38a4e0 -->
### Keycloak 與其他 OpenID Connect 服務

簽發者是 realm 的 URL，例如 `https://sso.example.com/realms/main`，與該服務的 ID 權杖中的寫法完全相同。請建立使用標準流程（standard flow）的機密用戶端，並設定 `openid`、`email` 與 `profile` 範圍。如果服務位於私人網路上，請把它列在 `login_provider_allowed_destinations` 中（[見下文](#providers-on-an-internal-network)）。使用 `-claim` 可以要求某個 claim（例如群組 claim）具有指定值之一。

<!-- source: discord 00e9659d1eae -->
### Discord

在 [Discord Developer Portal](https://discord.com/developers/applications) 建立應用程式，開啟「OAuth2」，然後把用戶端 ID 與用戶端密鑰提供給伺服器。伺服器會要求 `identify` 與 `email` 範圍；當你用 `-guild` 列出伺服器時，還會要求 `guilds`。使用者以數字形式的 Discord ID 識別；使用者名稱與顯示名稱只用來在註冊表單上建議使用者名稱。只有當 Discord 表示電子郵件地址已驗證時，該地址才視為已驗證；否則使用者要和其他地址一樣透過郵件確認。

使用 `-guild` 時，伺服器會在每次登入、註冊與連結時檢查使用者是否屬於列出的其中一個伺服器，否則就拒絕該使用者。未使用時，所有 Discord 使用者都能登入。guild ID 是最多 20 位數的數字：在 Discord 中開啟「開發者模式」（使用者設定的「進階」），然後在伺服器圖示上按右鍵並選擇「複製伺服器 ID」。

```sh
printf '%s\n' "$DISCORD_SECRET" | my login-provider create -kind discord -name Discord \
  -client-id 123456789012345678 -guild 987654321098765432
```

`add` 印出回呼 URL 之後，請把它加到應用程式的「OAuth2」、「Redirects」中。

<!-- source: github a7181ea6edd6 -->
### GitHub

建立一個 OAuth App（Settings、Developer settings、OAuth Apps），並把用戶端 ID 與密鑰提供給伺服器。伺服器會要求 `user:email` 範圍；當你用 `-org` 列出 Organization 時，還會要求 `read:org`。使用 `-org` 時，伺服器會在每次登入時檢查使用者是否屬於其中一個 Organization，因此其他 Organization 的成員會被拒絕。

<!-- source: set-up-a-provider 8e0bb73251d2 -->
## 設定登入服務

服務需要的重新導向 URI 就是該服務的回呼 URL `https://<host>/auth/external/<id>/callback`，其中包含伺服器為該服務指定的 ID。因此：

1. 先在服務端註冊 OAuth 用戶端（在 GitHub 上是「OAuth App」），重新導向 URI 暫時填任意值，並記下它提供的用戶端 ID 與用戶端密鑰。
2. 用這些值執行 `my login-provider create`。它會印出回呼 URL。
3. 把服務端用戶端的重新導向 URI 改為該回呼 URL。

```sh
# The client secret is read from standard input, or prompted for in a terminal.
printf '%s\n' "$CLIENT_SECRET" | my login-provider create -kind oidc -name "Example SSO" \
  -issuer https://sso.example.com -client-id my-client-id -claim hd=example.com
printf '%s\n' "$GITHUB_SECRET" | my login-provider create -kind github -name GitHub \
  -client-id Iv1.0123456789abcdef -org example-org -scope read:org
```

<!-- source: mastodon 6f167bff1f67 -->
### Mastodon

`mastodon` 服務是你指定的單一 Mastodon 伺服器；其他伺服器的使用者無法透過它登入。該伺服器必須是 Mastodon 4.3 或更新版本：`add` 會讀取它的授權伺服器中繼資料（`/.well-known/oauth-authorization-server`），並拒絕沒有這項資料的伺服器。未指定 `-client-id` 時，`add` 會自行在該伺服器上以回呼 URL 與 `profile` 範圍註冊用戶端，並儲存取得的用戶端 ID 與密鑰，所以你不需要在該伺服器上做任何事：

```sh
my login-provider create -kind mastodon -name mastodon.example -issuer mastodon.example
```

如果該伺服器不允許他人註冊用戶端，`add` 會失敗。這時請改在該伺服器的開發設定中，以 `profile` 範圍與回呼 URL 建立應用程式，並像其他類型一樣提供它的用戶端 ID 與密鑰（回呼 URL 包含服務的 ID，所以請依照上面的三個步驟進行）。

使用者只以 `profile` 範圍登入，這個範圍只會讀取他們的帳號，不會讀取其他內容。Mastodon 不提供電子郵件地址，因此用它註冊的人要透過郵件確認地址。註冊表單會建議該帳號的使用者名稱。當該伺服器的網域或其上層網域有網域處置（[網域處置](moderation.md#domain-actions)）時，它的按鈕會消失，也沒有人能透過它登入、註冊或連結帳號。服務對應的伺服器無法變更；若要使用另一個伺服器，請設定新的服務。

用戶端密鑰會以金鑰加密金鑰（[金鑰管理](key-management.md)）加密儲存，之後無論是指令還是管理 API 都不會再顯示它。

新的服務建立後即為啟用狀態。預設情況下，使用者可以把現有帳號連結到它（`-allow-link`），但沒有人能透過它註冊：加上 `-allow-signup` 可以讓新使用者透過它註冊，但仍受註冊模式限制（[註冊](registration.md)）。

<!-- source: change-rotate-disable-and-delete 55a0d23ac0d0 -->
## 變更、輪替、停用與刪除

```sh
my login-provider list                                 # every provider with its callback URL and linked users
my login-provider set -name "Corp SSO" <id>            # changes only the settings it names
my login-provider set -claim '' <id>                   # '' removes every -claim (or -scope, -org)
my login-provider rotate-secret <id>                   # reads the new client secret like add
my login-provider disable <id>
my login-provider enable <id>
my login-provider delete <id>
```

- `set` 只會變更它指定的設定。`-scope`、`-claim` 與 `-org` 會取代整個清單。服務的類型無法變更；請改為設定新的服務。
- `rotate-secret` 會以你輸入的用戶端密鑰取代原有的密鑰。請先在服務端輪替，再到這裡輪替。
- `disable` 會保留服務以及連結到它的帳號，但在你執行 `enable` 之前，沒有人能透過它登入、註冊或連結，登入頁面也不會顯示它。`disable` 與 `delete` 會印出有多少已連結的使用者無法再透過該服務登入，以及在設定開啟期間，[僅限登入服務](#limit-sign-in-to-login-providers)現在會產生什麼效果。
- `delete` 會永久移除服務。

<!-- source: users-who-sign-in-only-through-a-provider 11dc6f466cb7 -->
### 只透過登入服務登入的使用者

停用或刪除服務不會影響使用者。擁有密碼、通行金鑰或其他已連結帳號的使用者，可以用那些方式登入，除非[登入僅限登入服務](#limit-sign-in-to-login-providers)且仍有其他服務處於啟用狀態。三者都沒有的使用者將完全無法登入：`disable` 與 `delete` 會印出有多少已連結的使用者失去該服務，Web 用戶端也會在確認畫面中顯示這個數字。`enable` 會讓已停用服務的使用者恢復登入；已刪除的服務則無法復原。

若要協助這類使用者恢復帳號，請在管理畫面中該帳號的管理頁面，或用 `my admin account password-reset-link @alice` 核發密碼重設連結，並交給該使用者（[註冊](registration.md#password-reset-and-change)）。使用者之後就能用新密碼登入。

每次變更都會以服務的顯示名稱記錄在稽核記錄中，執行者記為 CLI，或是透過管理 API 進行變更的管理員。

<!-- source: limit-sign-in-to-login-providers bb5ebf61b647 -->
## 登入僅限登入服務

實例設定 `login_providers_only`（預設關閉）會在至少有一個服務處於啟用狀態時，讓登入服務成為登入與註冊的唯一方式。請在 Web 用戶端「伺服器設定」的「註冊」頁面開啟它，或使用 CLI：

```sh
my instance set login_providers_only true
```

Web 用戶端的確認畫面與指令的輸出會說明有多少使用中的本站使用者沒有連結到任何已啟用的服務。Web 用戶端也會指出其中是否包含你自己。它只會警告而不會拒絕：由你決定。

限制生效期間：

- 密碼與通行金鑰都無法讓任何人登入，登入頁面只會顯示登入服務。以密碼或通行金鑰開始、正在等待兩步驟驗證碼的登入也會被拒絕。
- 沒有人能用密碼註冊。註冊頁面會照常在[註冊模式](registration.md)下顯示允許註冊（`-allow-signup`）的服務。若沒有這樣的服務，註冊會顯示為 `closed`，對用戶端與 NodeInfo 也是如此。
- 沒有人能申請或使用密碼重設連結。連結在到期前仍然有效，如果限制及時解除就能使用。
- 使用者無法設定或變更密碼，也無法新增通行金鑰，因為兩者都無法用來登入。但仍可以移除它們。
- `my admin account password-reset-link` 與管理畫面仍會核發連結，並說明在限制解除前連結無法使用。`my admin create` 仍會建立管理員，並警告該密碼無法用來登入。
- 使用者無法解除自己在已啟用服務上的最後一個連結帳號。

仍然可以使用的部分：

- 工作階段、應用程式存取權杖與更新權杖都仍然有效。限制不會結束其中任何一項。
- 在工作階段中，密碼或通行金鑰仍可用來確認操作。現有使用者就是這樣轉移到登入服務的：在登入狀態下開啟設定中的「已連結的帳號」，以密碼或通行金鑰確認，然後在登入服務上登入。
- 透過登入服務登入、註冊與連結，以及透過服務登入後的兩步驟驗證碼。

無論送出的是哪個電子郵件地址、密碼或權杖，每次拒絕的訊息都相同，因此不會透露任何帳號的資訊。

**沒有連結的使用者在工作階段結束後就無法登入，直到你採取行動為止**。帳號絕不會依電子郵件地址連結，所以透過登入服務登入也找不到他們。請他們趁登入期間連結帳號。失去工作階段的使用者需要你暫時解除限制，否則就只能將他們拒於門外。這也包括處於 `disable_login`、`suspend` 或 `ban` 的使用者，他們沒有連結就沒有登入頁面可以提出[申訴](moderation.md)。連結時瀏覽器也必須在登入頁面上處於登入狀態：應用程式仍在登入狀態、但瀏覽器工作階段已結束的使用者，在限制生效期間無法連結，只能等你處理。**在開啟限制之前，請先向使用者公告這項變更**，讓他們趁工作階段還有效時連結帳號。

<!-- source: end-the-limit 77f5c71d013b -->
### 解除限制

限制會在每次請求時依照設定與服務的狀態生效，不需要重新啟動。

- 被鎖在外面的管理員可以在擁有伺服器設定的主機上執行：

  ```sh
  my instance set login_providers_only false
  ```

- 沒有任何服務處於啟用狀態時，限制不會生效：停用或刪除最後一個服務，就會重新開放密碼與通行金鑰登入，不論設定為何。既沒有密碼也沒有通行金鑰的使用者，仍然需要[重設連結](registration.md#password-reset-and-change)。
- 設定開啟且沒有任何服務啟用時，設定或啟用一個服務會立即讓限制生效；新的服務一設定完成就會啟用。Web 用戶端會先發出警告，說明有多少使用中的使用者沒有連結到該服務，以及其中是否包含你。只有在會讓限制開始生效的操作上才會警告。`my login-provider create`、`enable`、`disable` 與 `delete` 會在設定開啟期間印出限制現在會產生什麼效果。
- 當登入服務故障導致所有人都無法進入時，也請使用同一個指令。

<!-- source: re-authentication 8ec1742f8811 -->
## 重新驗證

要求密碼或通行金鑰的操作（設定密碼、新增通行金鑰、刪除或移轉帳號、連結另一個帳號），會要求兩者都沒有的使用者在已連結的服務上重新登入（[註冊](registration.md)）。伺服器會要求 OpenID Connect 服務重新驗證使用者（`max_age=0` 與 `prompt=login`），並拒絕較早的驗證；對 Mastodon 伺服器則以 `force_login=true` 提出要求。GitHub 與 Discord 沒有辦法要求重新登入，所以仍在那裡登入的使用者不需輸入任何內容就會被確認。伺服器仍會檢查該帳號是否為已連結的帳號，但這對「本人確實在電腦前」的證明較弱。若這點很重要，請優先使用 OpenID Connect 服務，或讓使用者保留密碼或通行金鑰。

<!-- source: providers-on-an-internal-network 7f19e187a375 -->
## 內部網路上的登入服務

伺服器透過其對外 HTTP 防護連線到登入服務，而這個防護會拒絕迴路位址與私人位址。若要使用你自己網路上的服務（例如與伺服器並列的 Keycloak），請在 [`login_provider_allowed_destinations`](../reference/configuration.md#login_provider_allowed_destinations) 中列出它的主機或位址範圍。
