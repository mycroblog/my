<!-- source: user-registration-registration-modes-invites-approval-email-confirmation 26f54561fd7a -->
# 使用者註冊（註冊模式、邀請、核准、電子郵件確認）

本頁說明使用者如何註冊並確認電子郵件地址，以及註冊模式、邀請碼、申請的核准、密碼重設和註冊封鎖。

<!-- source: from-sign-up-to-the-app fff50db25fa9 -->
## 從註冊到使用 App

![新使用者的路徑：註冊表單或登入服務提供者、可能拒絕註冊的檢查、各註冊模式給予的狀態，以及進入 App 前的核准與電子郵件確認。下方的列表說明了相同的內容。](images/sign-up-flow.svg)

由圖的上方往下：

- **開始**。訪客在註冊頁面（`/auth/sign_up`）以使用者名稱、電子郵件地址和密碼建立帳號；當[註冊僅限於登入服務提供者](#registration-limited-to-login-providers)時，頁面不提供這種方式。訪客也可以用尚未連結到任何使用者的帳號，透過[登入服務提供者](login-providers.md)登入；當服務提供者允許註冊（`-allow-signup`）時，訪客會看到相同但沒有密碼欄位的表單。
- **依序進行的檢查**。註冊模式（`closed` 會拒絕所有註冊）、每個網路的限制（每 30 分鐘 5 次）、CAPTCHA（當 `captcha` 不是 `none` 時）、[註冊封鎖](#sign-up-blocks)（`reject` 會拒絕，`require_approval` 會保留待核准）、邀請碼（必須可以使用，且 `invite` 模式必須有邀請碼）、對沒有邀請的註冊執行 [`signup.check`](decision-hooks.md#sign-ups) 外掛，以及使用者名稱和電子郵件地址是否已被使用。未通過檢查的註冊會被拒絕，表單上會顯示錯誤，且不會寫入任何資料。
- **註冊後立即的狀態**。新使用者會立即登入。在 `open` 模式下，註冊會被核准，或在 `mail = none` 時等待核准；在 `approval` 模式下會等待核准，或在有可用邀請時被核准；在 `invite` 模式下會被核准。`require_approval` 封鎖或 `signup.check` 的保留，在任何模式下都會讓註冊進入等待狀態。
- **直到帳號準備就緒**。等待中的註冊需要版主核准（`my registration approve` 或管理畫面）。在 `mail = smtp` 時，電子郵件地址必須透過郵件中的連結確認；登入服務提供者已驗證的地址會立即確認，而在 `mail = none` 時不需要確認。被拒絕的註冊不會被核准，該帳號也無法使用 App。
- **App**。準備就緒的帳號會進入歡迎步驟（顯示名稱和大頭貼），接著進入首頁時間軸。

以下各節說明每個步驟。

<!-- source: the-sign-up-page 111fbe5ecf64 -->
## 註冊頁面

使用者在 `https://<host>/auth/sign_up` 以使用者名稱、電子郵件地址和密碼註冊，除非[註冊僅限於登入服務提供者](#registration-limited-to-login-providers)。註冊後，該瀏覽器會立即登入。這個頁面不需要 JavaScript 也能運作，並與登入頁面一樣，以 CSRF 權杖及拒絕跨來源請求加以保護。當你選擇以 CAPTCHA 防範機器人時，註冊和登入頁面會顯示它的小工具，且需要 JavaScript（[CAPTCHA](captcha.md)）。

註冊模式是實例設定 `registration_mode`。可以用 CLI 選擇，或在網頁用戶端「伺服器設定」的「註冊」頁面（`/admin/settings/registration`）選擇（[實例設定](instance-settings.md)）。變更會從下一次註冊開始生效。在伺服器公告結束服務期間及結束服務之後，不論設定為何，註冊都會被視為 `closed`（[管理 CLI](admin-cli.md#announcing-the-end-of-the-service)）。在[一人伺服器](instance-settings.md#one-person-server)上也是如此，該伺服器也不提供邀請、註冊審核和註冊封鎖，且沒有任何方式（包括 `my admin create`）可以建立帳號。

```sh
my instance set registration_mode invite
```

| 模式 | 頁面 | 註冊後立即的狀態 |
| --- | --- | --- |
| `open` | 任何人都可以註冊；邀請碼可填可不填 | 已核准；但在 `mail = none` 時，會等待版主核准（見下文）。邀請不會改變任何事，只會記錄是誰邀請了該使用者 |
| `approval`（預設） | 另外詢問「你為什麼想加入？」（選填，最多 1,000 個字元）；邀請碼可填可不填 | 等待核准；有可用的邀請時會立即核准 |
| `invite` | 必須填寫邀請碼 | 已核准 |
| `closed` | 只說明不接受註冊 | 無 |

- 在 `closed` 以外的所有模式下，頁面都有邀請碼欄位，會從邀請連結（`?invite=<code>`）自動填入。無法使用的邀請碼（不存在、已撤銷、已過期、已用完，或屬於已無法再邀請的成員）在所有模式下都會被拒絕，而不是被忽略，以免打錯的邀請碼沒被注意到。
- 要求核准的註冊封鎖，即使有邀請，也會讓註冊保持等待核准（[註冊封鎖](#sign-up-blocks)）。
- 為決策掛鉤 [`signup.check`](decision-hooks.md#sign-ups) 設定外掛時，沒有邀請的註冊在通過 CAPTCHA 和封鎖之後，還會由該外掛審查，外掛可以將它保留待核准或拒絕。

- 來自同一個網路（IP 位址；IPv6 為 `/64`）的註冊，每 30 分鐘限制 5 次。超過時伺服器會回傳帶有 `Retry-After` 的 `429`。
- 如果使用者名稱或電子郵件地址已被使用，頁面會在該欄位顯示錯誤，且不寫入任何資料（邀請也不會被消耗）。
- 當你設定了[外部登入服務提供者](login-providers.md)（OpenID Connect、GitHub、Discord 或 Mastodon），並允許新使用者用它註冊時，登入和註冊頁面會為每個啟用的服務提供者顯示一個按鈕，標籤是你輸入的服務提供者顯示名稱（GitHub 服務提供者會以訪客的語言顯示「使用 GitHub 登入」，Discord 則顯示「使用 Discord 登入」）。在服務提供者的帳號尚未連結到本伺服器使用者的人，會看到相同但沒有密碼欄位的註冊表單，新使用者只能透過該服務提供者登入。註冊模式、邀請碼、加入原因、CAPTCHA、註冊封鎖、`signup.check` 和網路限制都與密碼表單一樣適用；在 `closed` 模式下，或服務提供者不允許註冊時，登入頁面會說明沒有連結的帳號。服務提供者標示為已驗證的地址（`email_verified`；GitHub 為已驗證的主要地址；Discord 為 Discord 標示為已驗證的地址），只要使用者保留它，就會立即確認；其他任何地址，以及透過 Mastodon 伺服器（它不提供地址）註冊的使用者的所有地址，都會依下文以郵件確認。與現有使用者相符的電子郵件地址永遠不會連結到該使用者：表單會說明該地址已被使用。啟用兩步驟驗證的使用者，透過服務提供者登入後仍需輸入代碼。GitHub 服務提供者會向 GitHub 要求 `user:email` 範圍，當你列出允許的 Organization 時，還會要求 `read:org`；伺服器接著會在每次登入時檢查使用者是否屬於其中之一，否則拒絕登入。請把 `https://<host>/auth/external/<provider ID>/callback` 登記為你 GitHub OAuth App 的回呼 URL。Discord 服務提供者會向 Discord 要求 `identify` 和 `email` 範圍，當你列出允許的伺服器時，還會要求 `guilds`；伺服器接著會在每次登入時檢查使用者是否屬於其中之一。請在你的 Discord 應用程式中登記相同的回呼 URL。
- 已登入的使用者可以在設定的「連結的帳號」中連結服務提供者的帳號，連結前要以密碼、通行金鑰或另一個已連結的帳號確認身分，且只限允許連結的服務提供者。瀏覽器必須以同一個使用者登入登入頁面；否則伺服器會要求先登入。其他使用者已連結的服務提供者帳號會被拒絕，每位使用者在每個服務提供者最多只能連結一個帳號。除非是最後一種登入方式（沒有密碼、沒有通行金鑰，也沒有其他已連結的帳號），否則使用者可以取消連結帳號。
- 沒有密碼也沒有通行金鑰的使用者，在進行原本需要它們的操作（設定密碼、新增通行金鑰、刪除或搬移帳號、連結其他帳號）時，要在已連結的服務提供者重新登入以確認身分。OpenID Connect 服務提供者會被要求重新驗證使用者（`max_age=0` 和 `prompt=login`），先前的驗證會被拒絕。這項確認只能在 10 分鐘內、針對該項操作使用一次；以非已連結的帳號登入會被視為一次錯誤嘗試，與密碼錯誤計入相同的限制。
- 每次註冊和登入時，伺服器都會記錄用戶端 IP 位址（登入時還會記錄 User-Agent）。這些記錄會在 180 天後自動移除（實例設定 `session_origins_retention_days` 和 `registration_origins_retention_days`，至少 90 天）。處於保存保留狀態的帳號，其記錄會被保留（[記錄保存與保留](moderation.md#record-retention-and-holds)）。請在你的隱私權政策中說明這個保存期限（[隱私權政策範本](privacy-policy-template.md)）。

<!-- source: when-a-user-can-use-the-app 317d6d8b7688 -->
## 使用者何時可以使用 App

等待核准、被拒絕或尚未確認電子郵件地址的使用者，仍可以在登入頁面登入。自行停用帳號的使用者，在帳號刪除日期之前也可以登入：他們會在那裡看到該日期，並可以重新啟用或刪除帳號（[審核](moderation.md#accounts-that-their-owners-deactivated)）。登入後，他們會看到自己正在等待的事項（版主核准或電子郵件地址確認），或註冊未獲核准，且無法繼續操作。只有已核准的使用者可以使用 App（用戶端 API 的 OAuth 授權和權杖發行），而在要求電子郵件確認的設定下，只有已確認電子郵件地址的使用者可以使用。

可以立即使用 App 的新使用者，會從註冊頁面直接進入 App 的歡迎步驟，設定顯示名稱和大頭貼。接著首頁時間軸會顯示入門步驟的檢查清單和推薦追蹤的人（[推薦](instance-settings.md#suggestions-of-accounts)）。在確認電子郵件地址的頁面上按「繼續」，也會進入相同的歡迎步驟。

以 `my admin create` 建立的管理員會被視為已核准且已確認。

<!-- source: email-confirmation 6c1b7759c2cf -->
## 電子郵件確認

設定 `mail`（[設定參考文件](../reference/configuration.md#mail)）決定寄送郵件的方式。SMTP 範例、DNS 設定以及如何解讀寄送失敗，請參閱[寄送電子郵件](email.md)。

- `mail = smtp`：伺服器會在註冊時寄送確認郵件。連結的有效期限為 24 小時，且只能使用一次。開啟連結後會顯示一個有確認按鈕的頁面，使用者按下按鈕後地址才會被確認（以免預先擷取郵件中連結的掃描程式消耗掉權杖）。登入後，使用者每小時最多可以要求重新寄送郵件 3 次。與 SMTP 伺服器的連線一律以 STARTTLS 加密，伺服器會在驗證身分和寄送之前驗證憑證。它永遠不會寄送到不提供 STARTTLS 的伺服器。
- `mail = none`（預設）：伺服器不寄送任何郵件，而是把 `mail not sent (mail = none)` 連同訊息類別寫入伺服器日誌。只有在 `profile = development` 時，這一行才會同時包含地址和內文（包括確認連結）。由於郵件永遠不會送達使用者，伺服器改為要求版主核准，而不是電子郵件確認。因此，`open` 模式下的註冊也會從等待核准開始，未確認的電子郵件地址也不會妨礙 App 的使用。

確認權杖除了這行開發用日誌之外，不會記錄在任何地方（存取日誌不記錄查詢字串）。資料庫只儲存權杖的 SHA-256。

<!-- source: registration-limited-to-login-providers 0d05572b3521 -->
## 註冊僅限於登入服務提供者

當實例設定 `login_providers_only` 開啟，且至少啟用了一個[登入服務提供者](login-providers.md#limit-sign-in-to-login-providers)時，註冊頁面不會有密碼表單，只提供允許註冊的服務提供者；連結中的邀請碼會被帶到服務提供者的註冊表單。不論表單內容為何，以密碼註冊都會被拒絕。沒有任何允許註冊的服務提供者時，註冊會被視為 `closed`。沒有任何啟用的服務提供者時，這項設定不會生效。

<!-- source: password-reset-and-change bec2baef5ab7 -->
## 密碼重設與變更

當實例設定 `login_providers_only` 生效時，重設連結以及設定或變更密碼都會被拒絕（[這項限制的作用](login-providers.md#limit-sign-in-to-login-providers)）。

- `mail = smtp`：當使用者在登入頁面的「忘記密碼了嗎？」輸入電子郵件地址時，伺服器會把設定新密碼的連結寄給以該地址為主要地址的使用者。不論帳號是否存在，頁面的回應都相同。連結的有效期限為 1 小時且只能使用一次，若在這段期間密碼以其他方式變更，連結也會失效。每個用戶端 IP 位址每 30 分鐘最多請求 5 次，每個地址每小時最多寄送 3 封郵件（超過限制的請求不會寄出任何東西，並得到相同的回應）。啟用 CAPTCHA 時，這個表單也會經過驗證。
- `mail = none`：頁面會說明無法寄送連結，並請使用者洽詢管理員。管理員可以在管理畫面中該帳號的「管理」頁面，或以 `my admin account password-reset-link @alice` 發行連結，再交給使用者（[審核](moderation.md)）。
- 已登入的使用者可以在設定的「密碼」中輸入目前的密碼來變更密碼（每位使用者每小時最多輸入錯誤 5 次）。
- 重設或變更會撤銷使用者的其他工作階段和 App 權杖（從設定變更時，只保留進行變更的那個 App）。通行金鑰和兩步驟驗證會保留。正在等待兩步驟驗證代碼的登入，以及尚未完成的 App 授權也會失效：使用者需要重新開始。在 `mail = smtp` 時，伺服器也會寄送電子郵件通知使用者密碼已變更。

<!-- source: invite-codes 5f98f9eb7d6f -->
## 邀請碼

邀請在 `open`、`approval` 和 `invite` 模式下可以使用。管理團隊的邀請以 CLI、審核畫面和下方的管理 API 建立；成員也可以建立自己的邀請（[成員邀請](#member-invites)）。在 `closed` 模式下，無法建立任何邀請，所有邀請也都無法使用。

```sh
my invite create                          # single use, expires in 7 days
my invite create -max-uses 10 -expires-in 72h
my invite create -max-uses 0 -expires-in 0  # no limit on uses or time
my invite list                            # newest first; states: usable / revoked / expired / used up
my invite revoke <code>
```

- `create` 會輸出邀請碼（16 個字元），以及已填入邀請碼的註冊頁面連結（`https://<host>/auth/sign_up?invite=<code>`）。
- 比對邀請碼時會忽略大小寫、空格和連字號。
- 即使多人同時註冊，邀請碼也永遠不會超過使用上限（`invite_redemptions` 上的唯一性限制）。
- 已撤銷、已過期或已用完的邀請碼都會顯示相同的錯誤（無法使用的邀請）。
- 除了 CLI 之外，你也可以在網頁用戶端的審核畫面（「邀請」，`/moderation/invites`）以及透過用戶端 API（`GET` / `POST /api/v1/admin/invites` 和 `POST /api/v1/admin/invites/{id}/revoke`）建立、列出和撤銷邀請。兩者都需要角色權限 `manage_invites`（內建的 `moderator` 沒有這項權限，若要委派，請建立擁有該權限的角色並授予）。列表上的「建立邀請」會開啟表單（`/moderation/invites/new`），並顯示新邀請的邀請碼和註冊連結。列表可以讓你複製可用邀請的連結並撤銷它。
- 列表會顯示所有邀請，包括管理團隊和成員的邀請，以及是誰建立的（`created_by`）和建立方式（`creator_kind`：`staff` 或 `member`）。擁有 `manage_invites` 的人可以撤銷其中任何一個。
- 審核畫面中帳號的「審查」頁面（`GET /api/v1/admin/accounts/{id}`，權限 `manage_accounts`）會顯示該帳號註冊時使用的邀請及其建立者，也就是邀請該使用者的人。
- 不論從畫面、API 或 CLI，建立和撤銷邀請都會記錄在稽核日誌中（`invite_created`、`invite_revoked`）。目標名稱是邀請碼的前 4 個字元（`ABCD…`）；完整的邀請碼不會複製到稽核日誌。撤銷已撤銷的邀請不會改變任何事，因此不會被記錄。

<!-- source: member-invites db3bff74ed69 -->
## 成員邀請

除了管理團隊之外，成員也可以建立自己的邀請。誰可以建立由實例設定 `invite_creators` 決定（[實例設定](instance-settings.md)）：

- `permitted`（預設）：只有角色擁有 `create_invites` 權限的使用者。預設只有內建的 `admin` 角色擁有它。你可以把它加到手動授予的角色，或[支持者方案](supporters.md)的角色，讓支持者可以邀請別人。
- `everyone`：所有已登入的本站使用者。

不論哪一種，帳號被停權或受限、或註冊未獲核准的使用者都無法建立邀請，而在註冊模式為 `closed` 時，任何人都無法建立。擁有 `manage_invites` 的人仍可繼續建立管理團隊的邀請，下方的限制都不適用於這些邀請。

成員可以在網頁用戶端的設定（「你的邀請」，`/settings/invites`）或透過用戶端 API（`GET` / `POST /api/v1/invites` 和 `POST /api/v1/invites/{id}/revoke`）建立、列出、複製和撤銷自己的邀請。設定中只會為目前可以建立邀請的使用者列出這個頁面；已無法建立邀請的使用者，仍可透過網址開啟它，以複製和撤銷自己建立的邀請。列表會說明每個邀請是可用、已撤銷、已過期、已用完，還是因建立者已無法再邀請而無法使用。

以下限制同樣是實例設定，只適用於成員邀請。管理員可以在伺服器設定的「註冊」頁面（`/admin/settings/registration`）的「成員邀請」中變更這些限制和 `invite_creators`：

| 設定 | 意義 | 預設值 | 範圍 |
| --- | --- | --- | --- |
| `member_invite_max_active` | 每位成員同時可持有的可用邀請（未撤銷、未過期且未用完）數量 | `5` | 0-100（0：成員無法建立邀請） |
| `member_invite_max_uses` | 一個邀請最多可供多少人註冊 | `10` | 1-1,000 |
| `member_invite_max_lifetime_seconds` | 一個邀請的最長有效時間（秒） | `2592000`（30 天） | 3,600（1 小時）至 31,536,000（365 天） |

- 成員可以在限制範圍內選擇使用次數和有效時間；沒有選擇時，邀請允許 1 次註冊，有效期 7 天（若限制較低則更短）。無法建立沒有使用次數或時間限制的邀請。
- 調低限制不會改變既有的邀請；它會套用到下一個建立的邀請。
- 每次以成員的邀請註冊時，都會檢查其建立者是否仍可建立邀請。當建立者失去 `create_invites`（例如他們的方案已結束）、`invite_creators` 改回 `permitted`，或帳號被停權、受限或刪除時，該邀請就無法使用，註冊頁面會說明它無效。邀請本身不會被變更，因此當建立者再次可以邀請時，它會在有效期限內重新生效。
- 成員的建立和撤銷會以 `invite_created` 和 `invite_revoked` 記錄在稽核日誌中，執行者為該成員。

在 `approval` 模式下，可用的邀請會立即核准新使用者：邀請他們的人代替了版主的審查。因此，在 `invite_creators = everyone` 時，每位使用者都可以不經審查就讓人加入，這會削弱核准模式的效果。如果你仰賴審查，請保持 `permitted`，只把 `create_invites` 給你信任的人，或把 `member_invite_max_active` 和 `member_invite_max_uses` 設低。

<!-- source: approving-and-rejecting e68676f67aab -->
## 核准與拒絕

```sh
my registration list                      # waiting for approval (longest wait first); shows the reason and whether the email is confirmed
my registration list -state rejected
my registration approve alice
my registration reject -note "spam" bob
```

- 引數是本站帳號的使用者名稱（開頭的 `@` 可以省略）。
- 只有等待核准的註冊可以做出決定。對已決定的註冊，或未經註冊頁面建立的使用者（`my admin create`），指令不會改變任何事，並以代碼 1 結束。
- 結束代碼 2 表示引數錯誤。

同樣的決定也可以在管理畫面（用戶端 API）中進行。全部都需要 `manage_accounts` 權限。

| 操作 | 作用 |
| --- | --- |
| `GET /api/v1/admin/registrations` | 等待核准的註冊（等待最久的排在前面）。回傳註冊的 ID、加入原因、電子郵件地址及是否已確認，以及本站帳號 |
| `POST /api/v1/admin/registrations/{id}/approve` | 核准。內文 `{"note": "..."}` 為選填。回應會以 `decision_note` 顯示備註，對已決定的註冊的回應不含電子郵件地址 |
| `POST /api/v1/admin/registrations/{id}/reject` | 拒絕。內文與 `approve` 相同 |

- 這與 CLI 是相同的使用案例，會把決定記錄在 `registration_decisions` 和稽核日誌中（`registration_approved`、`registration_rejected`）。CLI 決定的執行者是 `cli`；API 決定的執行者是做出決定的版主。
- 對已決定的註冊再次做決定不會改變任何事，並回傳目前的狀態（如果兩人同時做決定，只會記錄一個決定）。未經註冊頁面建立的使用者會回傳 404。

<!-- source: sign-up-blocks b70eb18cd52f -->
## 註冊封鎖

封鎖會依電子郵件網域、電子郵件地址或 IP 範圍，拒絕來自註冊頁面的註冊，或將它們改為需要核准。

```sh
my admin signup-block create -kind email_domain spam.example          # also applies to subdomains; a U-label works too
my admin signup-block create -kind ip_range -severity require_approval -duration 720h 192.0.2.0/24
my admin signup-block create -kind email_hash spammer@example.com     # only a hash of the address is kept
my admin signup-block list -active
my admin signup-block lift <block id>
```

- 符合 `-severity reject`（預設）封鎖的註冊會被拒絕，符合 `require_approval` 的註冊，不論註冊模式為何，都會從等待核准開始。符合多個封鎖時，套用較嚴格的那一個。註冊頁面不會顯示拒絕的原因。
- `email_hash` 會與轉為小寫、並去除本地部分中第一個 `+` 之後內容的地址比對。IP 範圍會與註冊 IP（與 `registration_origins` 相同的值）比對，永遠不會套用到 IP 不明的註冊。
- 已過期和已解除的封鎖沒有作用。新增和解除會記錄在稽核日誌中（`signup_block_added`、`signup_block_lifted`）。
- 當你以 `-block-email`（API 中為 `block_email: true`）停權本站帳號時，伺服器也會為該使用者的電子郵件地址建立 `email_hash` 封鎖。

對於有數千筆項目的列表，例如拋棄式電子郵件服務的網域，請用 `import` 從檔案（`-` 為標準輸入）大量新增。每個值都會套用相同的種類、嚴重程度、備註和期間。

```sh
my admin signup-block import -kind email_domain -comment "disposable email" disposable.txt
```

- 列表每行一個值（與 `add` 的 `<value>` 形式相同），或第一欄為值的 CSV 列。空白行和以 `#` 開頭的行會被略過。一次匯入最多 10,000 筆。
- 指令會輸出新增了多少筆，並針對每一列未新增的項目，輸出行號、值和原因。原因為 `invalid`（不是該種類的有效值）、`duplicate`（與同一列表中先前某列的正規形式相同）或 `already_blocked`（已有相同種類和值、且同等或更嚴格的封鎖生效中）。
- 每個新增的封鎖都會與 `add` 一樣記錄在稽核日誌中（`signup_block_added`），也可以逐一解除。
- 匯入以每次數百筆的交易寫入，因此若中途失敗，先前新增的部分會保留。再次匯入相同的列表只會新增剩下的部分。
- 軟體不附帶任何列表，也不會定期從 URL 擷取列表。公開的列表（例如 GitHub 上的 disposable-email-domains）需要由你自己擷取和更新，再重新匯入更新後的列表。

同樣的操作也可以在管理畫面（用戶端 API）中進行。全部都需要 `manage_accounts` 權限。在網頁用戶端中，開啟「審核」→「註冊封鎖」（`/moderation/signup-blocks`）。列表會顯示每個封鎖的值、種類、狀態和結束時間，可切換為生效中或全部；「新增封鎖」會開啟表單（`/moderation/signup-blocks/new`），「從列表新增」會匯入列表並顯示被略過的行（`/moderation/signup-blocks/import`），選擇某個值會開啟該封鎖的頁面，顯示其效果和備註，在那裡按「解除」即可結束封鎖。

| 操作 | 作用 |
| --- | --- |
| `GET /api/v1/admin/signup_blocks` | 列出封鎖（最新的在前）。可用 `kind` 和 `active` 篩選 |
| `POST /api/v1/admin/signup_blocks` | 新增封鎖。內文包含 `kind`、`value`、`severity`、`comment` 和 `expires_in`（秒，最多十年） |
| `POST /api/v1/admin/signup_blocks/import` | 從列表新增封鎖。內文包含 `kind`、`text`（列表）、`severity`、`comment` 和 `expires_in`。回應包含 `added`（新增的數量）和 `skipped`（未新增的列：`line`、`value`、`reason`） |
| `GET /api/v1/admin/signup_blocks/{id}` | 回傳一個封鎖 |
| `POST /api/v1/admin/signup_blocks/{id}/lift` | 解除封鎖。已解除的封鎖維持原狀 |
