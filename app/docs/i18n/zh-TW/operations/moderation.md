<!-- source: account-and-domain-actions-and-handling-reports 59d39b2e3f37 -->
# 帳號與網域處置，以及處理檢舉

說明營運者與版主如何對帳號與網域採取處置、決定申訴、處理檢舉，以及閱讀稽核記錄。除了帳號管理（僅限管理 API）之外，這裡的所有操作都可以透過管理 API（`/api/v1/admin/...`，範圍 `admin:read` / `admin:write`）與 CLI（`my admin account action` / `my admin domain` / `my admin report` / `my admin appeal`）進行。管理 API 需要角色權限：帳號管理、處置與申訴需要 `manage_accounts`，網域處置需要 `manage_domains`，檢舉則需要 `manage_reports`。CLI 會以與伺服器相同的設定（`-config` 或環境變數）直接連線到資料庫，不會記錄是哪位使用者操作的（`issued_by_user_id` 等欄位為 NULL，[稽核記錄](#audit-log)中的執行者為 `cli`）。

<!-- source: from-a-report-to-an-action-and-an-appeal dabd0eabc7c7 -->
## 從檢舉到處置與申訴

![檢舉的流程：檢舉進來、版主審查、可能接著採取處置並通知使用者，使用者則可能提出申訴；右欄顯示每個步驟會傳送給其他伺服器的內容。下方的清單說明每個步驟。](images/moderation-lifecycle.svg)

從圖的上方往下：

1. **檢舉進來**。可能是本站使用者提出檢舉、其他伺服器傳來 `Flag`（若其網域設有 `reject_reports` 則會被捨棄，請參閱[網域處置](#domain-actions)），或是[決策 hook](decision-hooks.md) 回答 `flag`（[由外部審查提出的檢舉](#reports-that-an-external-review-filed)）。檢舉其他伺服器上帳號的本站使用者可以選擇轉送檢舉，這會立即向該伺服器傳送 `Flag`。
2. **審查**。檢舉處於 `open`，版主會收到 `admin_report` 通知。解決檢舉會讓它變成 `resolved`，結果為 `actioned` 或 `dismissed`，本站的檢舉者會收到 `report_resolved` 通知。重新開啟會讓它回到 `open`。轉送關於其他伺服器上帳號的檢舉，會向該伺服器傳送 `Flag`（[檢舉](#reports)）。
3. **處置**。版主從檢舉發出[帳號處置](#account-actions)，這會把檢舉以 `actioned` 解決；也可以不經檢舉直接發出處置。處置具有類型、理由與結束時間，在被解除或到期時結束。若設定 `notify_target`，使用者會收到 `moderation_notice` 通知（[使用者看到的內容](#what-the-user-sees)）。其他伺服器會收到：`delete_statuses` 時貼文的 `Delete`、本站帳號的 `suspend` 或 `ban` 開始或結束時 actor 的 `Update`，以及遠端帳號被 `suspend` 而切斷的追蹤的 `Reject` / `Undo`。
4. **申訴**。本站使用者對處置提出申訴。申訴處於 `pending`，版主會收到 `admin_appeal` 通知。決定為 `upheld` 或 `overturned`，使用者會收到 `appeal_decided` 通知。推翻會解除處置並取消 `ban` 的刪除，因此本站帳號的 `suspend` 或 `ban` 被推翻時，會傳送 actor 的 `Update`（[申訴](#appeals)）。

指派、筆記、通知與申訴都留在這個伺服器上；只有右欄中的活動會離開伺服器。

<!-- source: moderation-and-server-administration-in-the-web-client 3beb1c9760a0 -->
## Web 用戶端中的審核與伺服器管理

Web 用戶端把工作人員的畫面分成兩組。導覽列只會對角色至少擁有該組一項權限的人顯示「審核」與「管理」。每個畫面對沒有權限的人只會顯示原因，伺服器也會拒絕他們的請求。

| 群組 | 權限 | 畫面 |
| --- | --- | --- |
| 審核（`/moderation`） | `manage_reports`、`manage_accounts`、`manage_domains`、`manage_invites`、`view_audit` | 檢舉（`/moderation/reports`）、申訴（`/moderation/appeals`）、帳號（`/moderation/accounts`）、註冊申請（`/moderation/registrations`）、註冊封鎖（`/moderation/signup-blocks`）、邀請（`/moderation/invites`）、網域處置（`/moderation/domain-actions`）、遠端伺服器（`/moderation/instances`）、稽核記錄（`/moderation/audit-log`） |
| 伺服器管理（`/admin`） | `manage_settings`、`manage_rules`、`manage_roles`、`manage_jobs`、`manage_login_recovery`、`manage_custom_emojis`、`manage_webhooks`、`manage_announcements`、`manage_finance`、`manage_oauth_apps`、`manage_service_closure`、`manage_errors`、`manage_login_providers` | 伺服器設定（`/admin/settings`）、規則（`/admin/rules`）、條款與政策（`/admin/documents`）、任務佇列與失敗的任務（`/admin/jobs`）、自訂表情符號（`/admin/custom-emojis`）、公告（`/admin/announcements`）、Webhook（`/admin/webhooks`）、支出與帳單匯入（`/admin/finance`，`manage_finance`）。`manage_login_recovery`（登入復原：移除兩步驟驗證與核發密碼重設連結）與 `manage_roles`（授予與撤銷帳號的角色）會以區塊的形式出現在帳號的管理頁面（`/moderation/accounts/{id}/manage`，也需要 `manage_accounts`） |

- 「審核」上的徽章是待處理檢舉、等待核准的註冊申請與等待審查的申訴的總數（[給版主的通知與待處理數量](#notifications-for-moderators-and-the-pending-counts)）。「管理」不顯示數量。
- 每個群組都會先開啟概覽；畫面本身位於旁邊的選單中。審核概覽中，你的角色會計算的每個佇列（待處理檢舉、等待核准的註冊申請、等待決定的申訴）各有一個方塊，分別連結到該佇列；另有一個方塊顯示最舊的待處理檢舉已等待多久（連結到依最舊排序的檢舉清單），並從整個佇列中列出應優先處理的待處理檢舉：先是違法內容，再來是等待最久的檢舉，並顯示每件已等待多久與由誰處理。檢舉清單可以依狀態、類別與負責人（你自己或無人）篩選，並依最新或最舊排序；篩選條件會保留在頁面的網址中。管理概覽會顯示伺服器的健康狀態：失敗的任務、任務佇列中最舊的任務已等待多久，以及未處理的錯誤；每個方塊都連結到對應的畫面，需要注意時會加上標示。
- 內建的 `moderator` 只有 `manage_reports`、`manage_accounts` 與 `view_audit`，看不到「管理」。它也看不到網域處置與遠端伺服器畫面（`manage_domains`），因此若要委派這些工作，請建立具有該權限的角色並授予它（[授予與撤銷角色](admin-cli.md#granting-and-revoking-roles)）。
- 不論屬於哪個群組，管理 API 的路徑都是 `/api/v1/admin/...`，範圍也都是同一組 `admin:read` / `admin:write`。在 API 參考中，群組以標籤區分（`admin-moderation`、`admin-server`）。
- 還有第三組權限「使用者功能」（`view_finance`、`create_custom_emojis`、`start_voice_rooms`、`create_invites`、`keep_posts_longer`），它位於工作人員畫面之外。只有這些權限的人既看不到「審核」也看不到「管理」；擁有 `view_finance` 的人，導覽列會在「設定」之後顯示「財務」（`/finance`）。它的 API 位於使用者命名空間（`GET /api/v1/finance/cost_totals` 與 `GET /api/v1/finance/income_totals`，範圍 `read:finance`），不需要 `admin:*` 範圍。它顯示的內容由[實例設定](instance-settings.md)中的 `finance_viewer_*` 決定。

<!-- source: account-actions a61dac7c5921 -->
## 帳號處置

處置會在記錄它的同一個交易中套用到限制投影（`account_restrictions`），並從下一個請求開始生效。

| 類型 | 對本站帳號的效果 |
| --- | --- |
| `warn` | 沒有限制（只通知使用者） |
| `force_sensitive` | 把所有附件都顯示為敏感內容 |
| `limit` | 把貼文（包括過去的貼文）視為僅追蹤者，並讓它們不出現在公開時間軸上 |
| `restrict_posting` | 禁止發文、回覆、轉發與編輯 |
| `restrict_interaction` | 禁止追蹤、按喜歡、轉發與回覆 |
| `disable_login` | 禁止使用應用程式與 API，並撤銷有效的工作階段與權杖（登入頁面仍讓使用者可以查看處置） |
| `suspend` | 套用上述所有限制，並對使用者本人以外的所有人隱藏個人檔案與貼文 |
| `ban` | 套用與 `suspend` 相同的限制，並在 30 天後刪除帳號。無法解除（只有在這 30 天內提出的申訴被推翻時才會取消） |
| `delete_statuses` | 刪除指定的貼文。連使用者本人也看不到它們，伺服器會向其他伺服器傳送與使用者自行刪除時相同的 `Delete` |
| `mark_statuses_sensitive` | 把指定貼文的附件顯示為敏感內容 |

對遠端帳號只能發出 `limit`、`suspend` 與 `force_sensitive`（其他類型在管理 API 中會回傳 `422`，在 CLI 中則會失敗）。效果只限於這個伺服器。唯一會傳送給對方伺服器的，是因 `suspend` 而切斷的追蹤的 `Reject` / `Undo`。

| 類型 | 對遠端帳號的效果 |
| --- | --- |
| `force_sensitive` | 把所有附件都顯示為敏感內容 |
| `limit` | 把貼文（包括過去的貼文）視為僅追蹤者，讓它們不出現在公開時間軸上，並且不會為沒有追蹤該帳號的使用者產生通知 |
| `suspend` | 捨棄從該帳號收到的所有內容，並隱藏它的個人檔案與貼文。切斷該帳號與本站使用者之間雙向的追蹤（包括追蹤請求），並向對方伺服器傳送該帳號的追蹤的 `Reject`，以及使用者的追蹤的 `Undo`。解除後會恢復顯示，伺服器也會再次接收來自該帳號的內容（停權期間被捨棄的內容與被切斷的追蹤不會恢復） |

- `notify_target`（`-no-notify`）對遠端帳號沒有意義（通知不會送達）。
- 因 `suspend` 而失去追蹤的本站使用者會收到一則通知（`relationships_severed`），說明失去了多少追蹤者與追蹤中的帳號（[被切斷的追蹤](#severed-follows)）。
- 與對本站帳號的處置相同，發出與解除都會記錄在[稽核記錄](#audit-log)中。

```sh
my admin account action issue -kind suspend -category spam -public-reason "spam" -duration 720h @alice
my admin account action issue -kind limit -report <report id> @bob@remote.example
my admin account action issue -kind delete_statuses -status <status id> -status <status id> @alice
my admin account action issue -kind ban -category legal -public-reason "illegal content" @alice
my admin account action list -active @alice
my admin account action lift -reason "appealed" <action id>
```

在管理 API 中：`POST /api/v1/admin/accounts/{id}/actions`（本文包含 `kind`、`reason_category`、`public_reason`、`private_note`、`report_id`、`notify_target`、以秒為單位的 `expires_in` 與 `status_ids`）、`GET /api/v1/admin/accounts/{id}/actions`（以 `active` 篩選）、`GET /api/v1/admin/account_actions/{id}`，以及 `POST .../{id}/lift`（本文 `{"reason": "..."}` 可省略）。

- 在 CLI 中以 `@username`（本站）、`@username@domain`（遠端）或帳號 ID 指定帳號；在管理 API 中則以帳號 ID 指定。
- 多個處置會疊加（限制取聯集）。解除只會移除該處置的限制；有結束時間的處置會在該時間自行結束（在清單中顯示為 `expired`）。結束時間最多為十年後（`expires_in` 最大為 315360000）；更長的時間會以 `422` 拒絕。
- 被 `disable_login`、`suspend` 與 `ban` 撤銷的工作階段與權杖，在解除處置後不會恢復。使用者要重新登入。
- 對本站帳號的 `suspend` 或 `ban` 會向追蹤者的伺服器與已知的伺服器傳送個人檔案被清空的 Actor `Update`，以表示停權（Mastodon 會顯示為「suspended」）。解除與到期時會傳送附有原本個人檔案的 `Update`。到期是由每 5 分鐘執行一次的任務傳送，因此最多可能延遲 5 分鐘。
- `delete_statuses` 與 `mark_statuses_sensitive` 透過 `-status`（`status_ids`）接受目標帳號 1 到 100 則未刪除的貼文。它們不能設定結束時間。解除 `delete_statuses` 不會恢復已刪除的貼文。解除 `mark_statuses_sensitive` 會恢復原本的顯示。
- `ban` 不能設定結束時間，也無法解除。只有在使用者於刪除前 30 天內提出的申訴被推翻時才會取消（[申訴](#appeals)）。拿不定主意時，請使用有結束時間的 `suspend`。處置的 30 天後，每小時執行一次的任務會刪除該帳號（帳號與貼文從畫面上消失，伺服器會向其他伺服器傳送 `Delete`）。
- `report_id`（`-report`）只能指定關於目標帳號的檢舉。待處理的檢舉會隨著處置一起以 `actioned` 解決（解決也會記錄在稽核記錄中）。已解決的檢舉不會變更。
- `public_reason` 是顯示給使用者的理由，`private_note` 則是只給版主看的筆記。`notify_target`（預設為 true；在 CLI 中以 `-no-notify` 設為 false）決定是否通知使用者。

<!-- source: what-the-user-sees 0ca06c59f8e1 -->
### 使用者看到的內容

- `notify_target` 為 true 的處置會以通知（`moderation_notice`）送達使用者。通知包含類型、理由類別、`public_reason`、結束時間與狀態。類型與類別也會以使用者介面語言的顯示名稱回傳。
- 使用者可以用第一方的 `GET /api/v1/moderation/actions`（以 `active` 篩選）依最新排序查看針對自己的處置。`notify_target` 為 false 的處置，以及已解除或已到期的處置也會列出。
- `private_note`、檢舉、採取處置的人以及解除的理由，絕不會顯示給使用者。
- 使用者可以對每個處置提出一次申訴（[申訴](#appeals)）。處置清單與通知中的處置會包含是否可申訴（`appealable`）、期限（`appeal_deadline`），以及申訴與其結果（`appeal`）。
- 處於 `disable_login`、`suspend` 或 `ban` 的使用者無法使用應用程式或 API，但仍可以在登入頁面（`/auth/sign_in`）登入。在那裡他們會看到生效中處置的類型、理由與結束時間（若為 `ban`，則是距離帳號被刪除還有幾天），並可以從該頁面提出申訴（[申訴](#appeals)）。授權端點（`/oauth/authorize`）會回應 `403`，不會核發權杖。處於 `ban` 的使用者在帳號被刪除前都可以登入，刪除時也會撤銷該工作階段。已排定刪除自己帳號的使用者，以及已被停用的帳號，連登入頁面也無法登入。

<!-- source: accounts-that-their-owners-deactivated ed010f49b40a -->
### 由擁有者自行停用的帳號

使用者可以在帳號設定中，再次輸入密碼或兩步驟驗證碼，或以通行金鑰確認之後（三者都沒有的使用者則要在已連結的登入服務上重新登入），停用自己的帳號。這不是審核處置，版主也無法停用任何人的帳號：

- 個人檔案與貼文會對版主以外的所有人隱藏，其他伺服器會收到標示為 `suspended` 的 actor `Update`，因此它們可能會把該帳號顯示為已停權。使用者的所有工作階段與權杖都會結束。若伺服器有寄送郵件，會寄出一封說明刪除日期的電子郵件；在那之前不會寄出其他電子郵件。
- 不會有任何東西送達該帳號：提及與私訊會被捨棄，不會產生通知、電子郵件或 Web Push，新的追蹤者會被拒絕。喜歡與轉發會被計數。現有的追蹤會保留。
- 使用者登入伺服器的頁面，頁面會顯示刪除日期，並讓他們重新啟用或刪除帳號。重新啟用會恢復一切。下一次停用要等 24 小時。
- 停用 30 天後，每小時執行的任務會刪除該帳號。即使任務尚未執行，在該日期之後的重新啟用也會被拒絕。使用者名稱不會被釋出。

版主可以在帳號清單中看到 `deactivated` 狀態（`state=deactivated`），仍然可以閱讀關於該帳號的檢舉並對它採取處置。審核處置不會改變刪除日期，但 `ban` 例外，它會以自己的日期取代：從那時起，使用者就無法重新啟用。帳號停用期間，停權不會向其他伺服器傳送任何東西，而在使用者重新啟用後仍然有效。在下列情況下，使用者無法停用帳號：審核讓帳號無法登入時、帳號已排定刪除時、帳號已搬家之後、註冊仍在等待審查時、付費訂閱仍在進行中時（包括設定為在期間結束時終止的訂閱），或是它是唯一能登入的管理員時。已停用的管理員不算是能登入的管理員。若已停用管理員的日期到了，而沒有其他管理員能登入，每小時執行的任務不會刪除該帳號，並會在日誌中記錄警告。那位管理員也無法再登入，因此請用 `my admin create` 建立另一位管理員；下一次執行時就會刪除該帳號。

<!-- source: appeals 8a6f1b4c288d -->
## 申訴

本站使用者可以對針對自己的處置提出申訴。版主審查申訴後，會維持原處置（`uphold`）或推翻它（`overturn`）。

```sh
my admin appeal list -state pending
my admin appeal uphold -reason "The posts were spam." <appeal id>
my admin appeal overturn -reason "Sorry, our mistake." <appeal id>
```

使用者以 `POST /api/v1/moderation/actions/{id}/appeal`（範圍 `write:accounts`，本文 `{"text": "..."}`）提出申訴。處於 `disable_login`、`suspend` 或 `ban` 的使用者無法取得存取權杖，要透過登入頁面上每個處置各自的表單（`POST /auth/appeal`）提出申訴。在[登入僅限登入服務](login-providers.md#limit-sign-in-to-login-providers)期間，這樣的使用者只能透過已連結的登入服務到達該表單。對於可以申訴的處置，頁面會顯示表單與期限；對於已申訴的處置，顯示內容與結果；對於已過期限或無法申訴的處置，則顯示原因。對於被駁回的申訴，會顯示理由。在管理 API 中：`GET /api/v1/admin/appeals`（以 `state` 篩選）、`GET .../{id}`，以及 `POST .../{id}/uphold` 與 `POST .../{id}/overturn`（本文 `{"public_reason": "..."}` 可省略）。它們需要 `manage_accounts` 權限。

- 每個處置只能申訴一次，內容為 1 到 2,000 個字元。期限是處置後經過實例設定 `appeal_window_days` 的天數（預設 180 天，範圍 20 到 365；請參閱[實例設定](instance-settings.md)）。已過期限的處置與第二次申訴會回傳 `422`（`/api/v1/problems/appeal-window-closed` 與 `/api/v1/problems/already-appealed`）。
- `ban` 可以申訴到帳號被刪除為止（處置後 30 天，若 `appeal_window_days` 的期限較短則以其為準）。已刪除帳號的 `ban` 會回傳 `422`（`/api/v1/problems/not-appealable`）。已解除或已到期的處置在期限內仍可申訴。
- 使用者提出申訴時，擁有 `manage_accounts` 的人會收到通知（`admin_appeal`）。審查完成時，使用者會收到通知（`appeal_decided`），並可以看到結果與 `public_reason`。審查者不會顯示。
- 推翻會在同一個交易中解除處置（效果與 `lift` 相同，解除理由為空）。推翻 `delete_statuses` 不會恢復已刪除的貼文。在審查前已解除的處置維持原狀。
- 推翻 `ban` 也會在同一個交易中取消已排定的帳號刪除（稽核記錄會記下 `account_deletion_canceled`）、結束停權，並向其他伺服器傳送原本個人檔案的 `Update`。已撤銷的工作階段與權杖不會恢復，因此使用者要重新登入。若帳號在審查前已被刪除，就無法推翻（`422`，`/api/v1/problems/not-appealable`；仍可以維持原處置）。
- 審查只會進行一次。以相同結果再次審查不會有任何變更，相反的結果則會回傳 `409`（`/api/v1/problems/appeal-decided`）。

<!-- source: account-management 99149899a198 -->
## 帳號管理

搜尋帳號、查看詳細資料，以及顯示 IP 位址與電子郵件地址，都是透過管理 API 進行，需要 `manage_accounts` 權限。

在 Web 用戶端中，一個帳號有三個頁面，頁面標題下方有它們之間的連結：

- **審查**（`/moderation/accounts/{id}`）：帳號的狀態與限制、新處置的表單、對它的最新處置及其理由，以及它最新的 20 件檢舉。較舊的檢舉只在檢舉清單（`/moderation/reports`）中，而該清單無法依帳號篩選。
- **處置**（`/moderation/accounts/{id}/actions`）：對該帳號的所有處置，分頁顯示，生效中的處置附有「解除」。每個處置都會開啟自己的頁面（`/moderation/accounts/{id}/actions/{action id}`），顯示其所有詳細資料。
- **管理**（`/moderation/accounts/{id}/manage`，僅限本站帳號）：帳號的角色（`manage_roles`）、電子郵件停止寄送狀態、移除兩步驟驗證與核發密碼重設連結（`manage_login_recovery`），以及它的電子郵件與 IP 位址。每個區塊只會對擁有其權限的人顯示。

工作人員只能對階級較低的人採取行動：發出或解除處置、決定申訴、解除電子郵件停止寄送、顯示私人資料、核發密碼重設連結與移除兩步驟驗證，都需要你的角色中最高的 `position` 高於目標的每個角色。對階級較高或相同的人（包括你自己的帳號）會回傳 `403`，問題類型為 `/api/v1/problems/target-outranks-actor`，因此版主無法對管理員停權，管理員之間也無法互相停權。`my admin` 指令沒有這項限制。讓唯一能登入的管理員無法登入的處置（`disable_login`、`suspend`、`ban`），不論由誰發出都會被拒絕，而該管理員也無法刪除自己的帳號（`409`，`/api/v1/problems/last-administrator`）。關於角色，請參閱 [admin-cli.md](admin-cli.md)。

- `GET /api/v1/admin/accounts` 會依最新排序回傳本站與遠端帳號（不包含實例 Actor）。你可以依使用者名稱前綴（`username`）、網域（`domain`）、本站或遠端（`origin`）、狀態（`state`）篩選，若是本站帳號，還可以依電子郵件地址（`email`）、電子郵件網域（`email_domain`）以及註冊或登入時使用的 IP 位址（`ip`）篩選。`email`、`email_domain` 與 `ip` 只能指定其中一個。清單不包含 IP 位址或電子郵件地址。
- 狀態是下列清單中第一個符合的項目：`deleted`、`suspended`（`suspend`、`ban`）、`disabled`（`disable_login`）、`rejected` 與 `pending`（註冊審查）、`limited`（`limit`）、`active`。狀態與限制（`restrictions`）只來自針對該帳號本身的處置，不包含網域處置。
- `GET /api/v1/admin/accounts/{id}` 會回傳狀態與限制；若是本站帳號，還會回傳註冊申請與審查、電子郵件確認、角色，以及最多 20 件依最新排序的處置與檢舉。
- `POST /api/v1/admin/accounts/{id}/reveal` 會回傳本站帳號的電子郵件地址、註冊與登入時的 IP 位址與 User-Agent（20 筆，依最新排序），以及使用相同 IP 位址的其他帳號（每個 IP 位址最多 20 個）。每次呼叫都會在稽核記錄中記下 `revealed_private_data`，只有在紀錄能寫入時才會回傳資料。IP 位址會在保留期間（預設 180 天）過後移除，因此無法顯示比這更早的登入來源。
- `GET /api/v1/admin/accounts/{id}` 中的 `two_factor_enabled` 顯示本站帳號的使用者是否使用兩步驟驗證（TOTP）。同時遺失驗證器應用程式與復原碼的使用者，在你以 `POST /api/v1/admin/accounts/{id}/two_factor/disable` 移除兩步驟驗證後，就能只用密碼登入。只有擁有 `manage_login_recovery` 權限的使用者可以進行這項操作，預設只有內建的管理員角色擁有它（它屬於伺服器管理；Web 用戶端只對他們顯示這個區塊）。移除時會在稽核記錄中記下 `two_factor_disabled`。移除前，請透過電子郵件等其他管道確認對方是帳號的擁有者。當沒有其他管理員的管理員遺失它時，營運者可以用 CLI 指令 `my admin account disable-two-factor @alice` 移除（稽核記錄的執行者為 CLI）。
- 在會寄送郵件的伺服器上，忘記密碼的使用者可以從登入頁面的「忘記密碼？」收到設定新密碼的連結郵件。對於不寄送郵件的伺服器（`mail = none`），或郵件無法送達的使用者，則由管理員核發連結並轉交。在管理畫面中，於帳號的管理頁面（`/moderation/accounts/{id}/manage`，`POST /api/v1/admin/accounts/{id}/password_reset_link`）使用「密碼重設」核發。只有擁有與移除兩步驟驗證相同權限 `manage_login_recovery` 的使用者可以核發，預設只有內建的管理員角色擁有它（持有連結的人就能進入帳號，因此版主的 `manage_accounts` 並不足夠）。核發的連結只會當場顯示一次，並可以複製。伺服器只儲存連結的雜湊值，所以之後無法再查看；需要時請再次核發。沒有可用的管理員或無法使用管理畫面時，營運者可以用 CLI 指令 `my admin account password-reset-link @alice` 核發（稽核記錄的執行者為 CLI）。在[登入僅限登入服務](login-providers.md#limit-sign-in-to-login-providers)期間，兩者仍會核發連結，並說明在限制解除前連結無法使用。連結的有效期限為 1 小時且只能使用一次，若之後密碼經由其他管道變更，連結就會失效。不論以哪種方式核發，都會在稽核記錄中記下 `password_reset_link_issued`。任何持有連結的人都能設定密碼，因此請透過電子郵件等其他管道確認對方身分，並只把連結交給本人。設定新密碼會撤銷該使用者所有的工作階段與應用程式權杖。通行金鑰與兩步驟驗證會保留。
- `GET /api/v1/admin/accounts/{id}` 中的 `email_suppression` 表示已停止寄送到本站帳號的主要地址。`hard_bounce`（收件地址不存在）會停止確認郵件以外的所有郵件，`complaint`（被回報為垃圾郵件）則只停止通知與公告郵件。密碼重設郵件也會停止，因此需要時請核發上述連結並轉交。當使用者要求確認郵件並再次確認地址時，hard bounce 的停止寄送就會解除。若你已確認寄送沒有問題（例如使用者已修正信箱），請以 `POST /api/v1/admin/accounts/{id}/email_suppression/lift`（權限 `manage_accounts`）解除。這會在稽核記錄中記下 `email_suppression_lifted`。
- 一次搜尋最多會讀取 400 個帳號來填滿一頁。符合的帳號很少時，即使是很短或空白的頁面也可能帶有 `next_cursor`，因此請繼續讀取。

<!-- source: domain-actions 13dd3d4d3a0f -->
## 網域處置

| 類型 | 效果 |
| --- | --- |
| `suspend` | 停止所有傳送、擷取與接收（接收會在驗證簽章之前就回傳 `403`）。現有的帳號與貼文會從時間軸與個人檔案中隱藏。切斷本站使用者與該網域帳號之間雙向的追蹤（不會向對方傳送任何東西） |
| `limit` | 讓貼文不出現在公開時間軸與搜尋中，並且不會為沒有追蹤該帳號的使用者產生通知 |
| `reject_media` | 不擷取媒體 |
| `reject_reports` | 捨棄來自該網域的檢舉（`Flag`） |
| `force_sensitive` | 把媒體顯示為敏感內容 |

```sh
my admin domain issue -kind suspend -public-comment "spam" spam.example
my admin domain issue -kind limit -duration 720h noisy.example
my admin domain list -active
my admin domain lift <action id>
```

在管理 API 中：`POST /api/v1/admin/domain_actions`（本文包含 `domain`、`kind`、`public_comment`、`private_note`、`obfuscate` 與以秒為單位的 `expires_in`）、`GET /api/v1/admin/domain_actions`（以 `domain` 與 `active` 篩選）、`GET .../{id}`，以及 `POST .../{id}/lift`。

在 Web 用戶端中，開啟「審核」→「網域處置」（`/moderation/domain-actions`）。清單會顯示每個處置的網域、類型、狀態與結束時間，可選擇只看生效中的或全部；「新增處置」會開啟表單（`/moderation/domain-actions/new`），選擇網域則會開啟該處置的頁面，顯示其註解與日期，可在那裡以「解除」結束它。遠端伺服器頁面上的「網域處置」連結只會列出該網域的處置，從那裡的「新增處置」開始時，表單會預先填入網域。

- 網域會正規化為 IDNA A-label 後記錄（小寫，結尾沒有點）。`例え.jp` 與 `xn--r8jz45g.jp` 是同一個網域。
- 處置也會套用到子網域（對 `spam.example` 的處置也會套用到 `a.spam.example`）。
- 處置會在記錄它的同一個交易中套用到限制投影（`domain_restrictions`）與時間軸限制，並從下一次傳送、擷取或接收開始生效。
- 有結束時間的處置（`-duration` / `expires_in`，最多十年）會在該時間自行結束。到期不算是「解除」，因此清單會顯示為 `expired`。
- 解除只會移除該處置的限制。對同一網域的其他處置會保留。解除已解除的處置不會有任何變更。
- 清單依最新排序，`-active`（`active=true`）只顯示生效中的處置。

<!-- source: importing-and-exporting-mastodon-csv ba9ed39bd32e -->
### 匯入與匯出 Mastodon CSV

你可以用 Mastodon 網域封鎖的 CSV 格式（`#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate`）與 Mastodon 互相交換網域處置清單。你可以匯入 Mastodon 匯出的 CSV，Mastodon 也可以匯入這個伺服器匯出的 CSV。

```sh
my admin domain export > domain_blocks.csv
my admin domain import domain_blocks.csv                       # only shows what it would do
my admin domain import -apply -only spam.example,bad.example domain_blocks.csv
```

在管理 API 中：`GET /api/v1/admin/domain_actions/export`（`text/csv`）、`POST .../import/preview`（本文包含 `csv`），以及 `POST .../import`（本文包含 `blocks`，即選擇的列，以及 `private_note`）。這個伺服器本身網域的列會被略過（`own_domain`），對它的網域處置也會被拒絕。在 Web 用戶端中，開啟「審核」→「網域處置」並選擇「Mastodon 的 CSV」（`/moderation/domain-actions/import`）：「匯出 CSV」會儲存檔案，匯入則會先預覽貼上或選擇的檔案，再匯入你保留的列。

| CSV 列 | 在這個伺服器上的處置 |
| --- | --- |
| `#severity` 為 `suspend`（或空白） | `suspend` |
| `#severity` 為 `silence` | `limit` |
| `#severity` 為 `noop` | 無 |
| `#reject_media` / `#reject_reports` 為 true | 分別加上 `reject_media` / `reject_reports` |

- 匯入分為兩個步驟。先在預覽中確認每一列會發出哪些處置，選擇要保留的列，然後再匯入（在加上 `-apply` 之前，CLI 不會變更任何東西，`-only` 則依網域選擇列）。
- 若某列的網域或其上層網域已有相同類型的生效中處置，該列就不會發出該類型（若每種類型都已存在，就是「已套用」）。同一網域的第二列、無效的列，以及不會發出任何處置的列（只有 `noop`）也會被略過。再次匯入同一個 CSV 不會新增處置。
- 沒有 `#domain` 標頭的 CSV 會被讀取為每行一個網域的清單，每個網域都會得到 `suspend`（與 Mastodon 相同）。一次匯入最多 5,000 列。
- 匯入的處置會持續「直到解除」，並帶有該列的公開註解與模糊化設定，以及你提供的私人筆記。它們會像逐一發出的處置一樣出現在稽核記錄中（透過 API 時執行者為操作者本人，透過 CLI 時為 `cli`）。
- 匯出會為生效中的處置每個網域寫出一列。有 `suspend` 的網域得到 `suspend`，有 `limit` 的得到 `silence`，兩者都沒有的得到 `noop`。只有 `force_sensitive`（Mastodon 的網域封鎖沒有這種類型）的網域不會被匯出。私人筆記與結束時間不會被匯出。

<!-- source: severed-follows 51be435d8635 -->
### 被切斷的追蹤

網域 `suspend` 與遠端帳號 `suspend` 會切斷本站使用者與被停權帳號之間雙向的追蹤。

- 對於網域 `suspend`，在處置記錄後執行的任務（`moderation.sever_domain`）會逐一為每位本站使用者切斷追蹤。在任務完成之前，追蹤仍會顯示在清單中。子網域上的帳號也會被切斷。
- 對於遠端帳號 `suspend`，追蹤會在記錄處置的同一個交易中切斷。對已被停權的帳號再次 `suspend` 不會切斷任何東西。
- 每位失去已核准追蹤的使用者都會收到一則通知（`relationships_severed`），說明失去了多少追蹤者與追蹤中的帳號。只失去追蹤請求的使用者不會收到通知。
- 解除處置不會恢復被切斷的追蹤。使用者要重新追蹤。
- 再次處理同一個處置（例如任務重新執行時）不會再新增被切斷關係的紀錄或通知。

<!-- source: publishing-the-domains-you-act-on b647d2c8c84c -->
### 公開你處置的網域

你可以透過 `GET /api/v1/instance/domain_actions` 公開生效中的 `limit` 與 `suspend` 處置。實例設定 `domain_actions_visibility`（`off` / `signed_in` / `everyone`，預設 `off`）決定誰能看到清單，`domain_actions_rationale_visibility`（相同的值，預設 `off`）則決定誰能看到公開註解（[實例設定](instance-settings.md)）。

```sh
my instance set domain_actions_visibility signed_in
my instance set domain_actions_rationale_visibility signed_in
```

- 設為 `off` 時，API 會回傳 `404`；設為 `signed_in` 時，對沒有存取權杖的請求會回傳 `401`。
- 每個處置一個項目，依最新排序。項目包含網域、網域的 SHA-256（十六進位）與類型，對獲准查看的人還會包含公開註解。私人筆記絕不會回傳。
- 對於設定了 `-obfuscate`（`obfuscate`）的處置，網域中（點以外的）字元中間一半會被替換為 `*`（`bad.example` 會變成 `bad.*****le`）。任何人都可以比對自己知道的網域的 SHA-256，確認它是否就是被模糊化的那一個。
- `reject_media`、`reject_reports` 與 `force_sensitive` 處置，以及已解除或已到期的處置不會被公開。
- Web 用戶端的實例資訊頁面（`https://<host>/about`）會依相同的設定顯示這份清單。設為 `off` 時不會顯示這個區塊；設為 `users` 時，未登入的人會看到只有已登入使用者才能查看的說明。

<!-- source: refetching-remote-posts 883740b40c9a -->
## 重新擷取遠端貼文

你可以從對方伺服器重新擷取已儲存的遠端貼文。只有在你透過管理 API、CLI 或 Web 用戶端提出要求時才會重新擷取，絕不會定期進行。

```sh
my admin refetch status <post ID or URI>
my admin refetch account @bob@remote.example
my admin refetch domain remote.example
```

在管理 API 中：`POST /api/v1/admin/statuses/{id}/refetch`、`POST /api/v1/admin/accounts/{id}/refetch_statuses` 與 `POST /api/v1/admin/instances/{domain}/refetch_statuses`。它們需要 `manage_domains` 權限。在 Web 用戶端中，請使用帳號審查頁面（`/moderation/accounts/{id}`，遠端帳號）與遠端伺服器畫面（`/moderation/instances/{domain}`）上的「重新擷取貼文」。

- 貼文會像收到的 `Update` 一樣處理。若有較新的版本，伺服器會新增一個版本；會更新投票的票數；若沒有任何變更則什麼也不做。對方伺服器回傳 410 Gone 或 `Tombstone` 的貼文會被刪除。無法擷取的貼文（包括 404）會維持原狀。
- 對於帳號，伺服器最多會重新擷取它已儲存且未刪除的最新 1,000 則貼文，每次 100 則，間隔一分鐘。
- 對於網域，伺服器會從該網域最新的帳號開始重新擷取，每個帳號最多擷取最新的 20 則貼文，總共最多 1,000 則，每次 5 個帳號，間隔一分鐘。
- 不會從設有 `suspend` 的網域，或因傳送失敗而處於 `unavailable` 的網域擷取。同一個 URI 的擷取失敗會被記住 1 小時，在這段時間內重新擷取時不會擷取它。
- 你的要求會以 `status_refetch_queued`、`account_statuses_refetch_queued` 或 `domain_statuses_refetch_queued` 記錄在稽核記錄中。

<!-- source: reports e8a3f112b323 -->
## 檢舉

從其他伺服器收到的檢舉（`Flag`）會以待處理（`open`）狀態進入檢舉清單。

```sh
my admin report list -state open
my admin report show <report id>
my admin report assign <report id> <username>
my admin report unassign <report id>
my admin report resolve -outcome dismissed <report id>
my admin report note <report id> "Same person as an earlier report"
my admin report reopen <report id>
my admin report forward -comment "Spam account, see the linked posts" <report id>
```

在管理 API 中：`GET /api/v1/admin/reports`（以 `state`、`category` 與 `assignee` 篩選：`me`、`none` 或本站帳號的 ID；`order=oldest` 會改為先列出等待最久的檢舉，而不是最新的）、`GET .../{id}`、`POST .../{id}/assign_to_self`、`POST .../{id}/unassign`、`POST .../{id}/resolve`（本文 `{"outcome": "actioned" | "dismissed"}`）、`POST .../{id}/notes`（本文 `{"body": "..."}`）、`POST .../{id}/reopen` 與 `POST .../{id}/forward`（本文 `{"comment": "..."}`，可省略）。

- 指派（`assign`）只能指派給角色擁有 `manage_reports` 的本站使用者。在管理 API 中只能指派給自己。
- 解決（`resolve`）的結果為 `actioned`（已對目標採取處置）或 `dismissed`（不需要處置）。再次解決已解決的檢舉不會改變結果。
- 從檢舉發出處置時（處置的 `report_id` / `-report`），檢舉會在同一個操作中以 `actioned` 解決。
- 筆記（`note`）是只有版主看得到的內部筆記，最多 2,000 個字元。目標與檢舉者都看不到它。筆記無法編輯或刪除。
- 重新開啟（`reopen`）會讓已解決的檢舉回到 `open`（指派會保留）。待處理的檢舉無法重新開啟（在管理 API 中為 `409`）。
- 轉送（`forward`，檢舉詳細資料中的按鈕**轉送到對方伺服器**）會把關於其他伺服器上帳號的檢舉，以你的實例 actor 發出的 `Flag` 傳送給該伺服器，附上該帳號與仍然存在的被檢舉貼文。它唯一攜帶的文字，是你轉送時寫給該伺服器版主的內容（`-comment`、管理 API 中的 `comment`，或檢舉頁面上的**給對方版主的文字**；最多 1,000 個字元，預設為無）。檢舉者的留言與[外部審查](#reports-that-an-external-review-filed)的筆記絕不會以這種方式離開你的伺服器，也不會傳送任何關於檢舉者的資訊。檢舉者在提出檢舉時選擇轉送的檢舉，已經附上檢舉者的留言立即送出。轉送適用於在這個伺服器上由人或[外部審查](#reports-that-an-external-review-filed)提出、待處理或已解決、尚未轉送過的檢舉；從其他伺服器收到的檢舉，或關於語音聊天房間的檢舉無法轉送（在管理 API 中為 `409` `report-not-forwardable`，檢舉詳細資料也不會顯示按鈕）。檢舉只會傳送一次：再次轉送（無論附上什麼文字），或轉送檢舉者提出時已轉送過的檢舉，都不會有任何變更。管理 API 中的 `forwarded` 表示是否已傳送，`forwardable` 則表示現在是否可以傳送。
- 檢舉詳細資料（`show`、`GET .../{id}`）會依最舊排序回傳筆記，以及關於該目標的其他檢舉數量與對該目標的處置數量（包括已解除與已到期的處置）。清單不會回傳筆記。
- 指派、解決、重新開啟、筆記與轉送都會保存為事件（`report_assignments`、`report_resolutions`、`report_reopenings`、`report_notes`、`report_forwards`），`report_states` 則保存目前的狀態。
- 版主可以在檢舉頁面上翻譯被檢舉的貼文與檢舉者的留言，或使用 `POST .../{id}/statuses/{status_id}/translation` 與 `POST .../{id}/comment/translation`（本文 `{"target_language": "en"}`）。只有在[實例設定](instance-settings.md) `translation_reports_to_server` 開啟時，文字才會送到翻譯服務，而且只會送到獲准處理該貼文可見範圍的服務：版主的存取權限不會擴大任何範圍，因此私訊貼文只會送到獲准處理 `direct` 的服務，留言則視為 `private`。留言的翻譯不會保留，已刪除貼文的翻譯也不會保留，字元數只計入伺服器的每月上限。開啟 `translation_browser` 時，瀏覽器內建的翻譯功能不論 `translation_reports_to_server` 為何，都能用於檢舉。
- 檢舉者看到的目標內容，與他們平常看該帳號時相同：當帳號被停權或封鎖了他們時，他們的檢舉仍會列出，但不顯示該帳號。版主則總是能看到完整的目標。
- 本站檢舉者會在第一次解決時收到通知（`report_resolved`）。重新開啟並再次解決不會再新增通知。檢舉者可以用 `GET /api/v1/reports` 列出自己的檢舉，只能看到每件是待處理還是已解決。結果、採取的處置、給目標的理由以及負責人都不會顯示。

<!-- source: reports-that-an-external-review-filed 8563536b46e4 -->
### 由外部審查提出的檢舉

回答 `flag` 的[決策 hook](decision-hooks.md) 會以你的實例 actor 作為檢舉者提出檢舉。檢舉會標示這一點：管理 API 與 `report.created` webhook 中的 `hook_origin`、檢舉詳細資料中的一行說明，以及 `my admin report show` 中的 `automated`。它的留言是外掛的筆記，類別則來自理由。它在提出時不會被轉送，解決時也不會通知任何人。當目標位於其他伺服器時，你可以自行轉送（`forward`）：外掛的筆記會留在你的伺服器上，只有你寫的文字會送出。取消該決策不會影響檢舉。

<!-- source: voice-chat-rooms 7fd142752b07 -->
### 語音聊天房間

- 本站使用者可以檢舉自己看得到的開放中語音聊天房間（`POST /api/v1/reports`，帶有 `voice_room_id`，並把 `account_id` 設為房間的主持人）。檢舉的目標是當時的主持人，並指向當時的房間設定（名稱與說明）。它不指向任何貼文，也絕不會被轉送到其他伺服器。
- 檢舉詳細資料（`GET /api/v1/admin/reports/{id}`）會顯示房間（`voice_room`）：其設定的每次變更、誰參與過以及何時離開、誰被移出，以及房間如何結束。參與者的紀錄會在房間結束後保留 90 天（`voice_participants_retention_days`），只要有檢舉指向該房間就會一直保留（[紀錄保留](#record-retention-and-holds)）。沒有錄音也沒有文字聊天，因此這些紀錄是唯一的證據。
- 擁有 `manage_reports` 的人可以用 `POST /api/v1/admin/voice_rooms/{id}/end`（本文 `{"reason": "..."}`，可省略，最多 500 個字元）為所有參與者結束開放中的房間。理由只給版主看；參與者只會看到房間已結束。語音聊天關閉時也能使用，稽核記錄會記下 `voice_room_ended`。

<!-- source: notifications-for-moderators-and-the-pending-counts 7b83fa4097b2 -->
## 給版主的通知與待處理數量

新的檢舉、等待核准的註冊申請與申訴，會以應用程式內通知送達擁有權限的人。

| 通知 | 送達時機 | 接收者 |
| --- | --- | --- |
| `admin_report` | 檢舉的目標沒有其他待處理檢舉時（關於同一帳號的第二件以後的檢舉不會通知） | 角色擁有 `manage_reports` 的人，檢舉者與檢舉目標除外 |
| `admin_sign_up` | 註冊以等待核准狀態開始時（核准制，或在無法寄送郵件的設定下為 `open`） | 角色擁有 `manage_accounts` 的人 |
| `admin_appeal` | 有人對處置提出申訴時 | 角色擁有 `manage_accounts` 的人，提出申訴者除外 |
| `admin_custom_emoji_request` | 有人申請建立自訂表情符號或替換其圖片時 | 角色擁有 `manage_custom_emojis` 的人，申請者除外 |
| `admin_error_group` | 伺服器或 Web 用戶端回報新種類的錯誤時，或已解決的錯誤群組在另一個伺服器版本上再次發生時 | 角色擁有 `manage_errors` 的人 |

- `admin_error_group` 整個伺服器每小時最多送達 10 次。該小時內其餘的會合併成一則通知，說明還有多少個錯誤群組。已靜音的群組絕不會通知。Web 用戶端為未登入訪客回報的錯誤，只有在已登入使用者的瀏覽器也回報時才會通知，而其再次發生不會通知任何人。錯誤群組可以透過 `GET /api/v1/admin/error_groups` 與 `POST /api/v1/admin/error_groups/{id}/resolve`（`/mute`、`/unmute`）讀取並解決、靜音或取消靜音，這些需要 `manage_errors`。
- 誰會收到通知，取決於通知建立時的權限。之後失去權限的人，在通知清單與未讀數量中不會再看到該類型。若權限恢復，就會再次看到。
- 上述每種通知也會以電子郵件寄送，預設為開啟。每個人可以在設定的**通知**頁面（Web 用戶端中為 `/settings/notifications`）分別開啟或關閉每種通知的應用程式內通知與電子郵件，該頁面只列出其角色會收到的類型。在應用程式中關閉的類型不會為他們建立，因此既不會推播也不會寄送電子郵件；`GET /api/v1/admin/counts` 的數量不會改變，因為它計算的是檢舉、註冊申請與申訴本身。除了待處理檢舉之外，數量也會回傳最舊的待處理檢舉的提出時間（`oldest_open_report_filed_at`）。伺服器會在記錄通知的交易中把郵件任務放入佇列，任務會在提交後立即寄出郵件，不會等待通知被閱讀。若在寄送時該人已失去權限或關閉了該類型的電子郵件，或其主要地址未經確認或已停止寄送，則不會寄出任何東西。郵件只包含通知的類型與通知畫面的連結，不包含檢舉或帳號的任何內容。其取消訂閱連結只會關閉該類型的電子郵件。設定 `mail = none` 時不會寄送任何郵件，`GET /api/v1/instance` 中的 `sends_email` 為 `false`。

`GET /api/v1/admin/counts` 會回傳待處理檢舉的數量（`open_reports`，需要 `manage_reports`）、等待核准的註冊申請數量（`pending_registrations`）與等待審查的申訴數量（`pending_appeals`）（兩者都需要 `manage_accounts`）。你沒有權限的項目不會回傳。

<!-- source: audit-log 86955f4182c0 -->
## 稽核記錄

版主與管理員的操作會在與操作相同的交易中，每次一列記錄在稽核記錄（`audit_log_entries`）中。失敗的操作與沒有任何變更的操作不會被記錄：解除已解除的處置、解決已解決的檢舉、指派給同一位負責人、停用已停用的規則、移動到相同位置、授予使用者已擁有的角色，以及撤銷使用者沒有的角色。

| 目標類型 | 操作 |
| --- | --- |
| `account` | `account_action_issued` 與 `account_action_lifted`（發出與解除對帳號的處置）、`account_deletion_canceled`（因對 `ban` 的申訴被推翻而取消刪除）、`revealed_private_data`（顯示 IP 位址與電子郵件地址）、`two_factor_disabled`（移除兩步驟驗證）、`password_reset_link_issued`（核發密碼重設連結）、`email_suppression_lifted`（解除郵件停止寄送）、`account_moved`（受限制的本站帳號搬到另一個本站帳號；限制不會帶到目的地，因此由你決定是否要在那裡套用）、`account_statuses_refetch_queued`（重新擷取遠端帳號的貼文） |
| `status` | `status_refetch_queued`（重新擷取遠端貼文；目標名稱為貼文 URI） |
| `domain` | `domain_action_issued` 與 `domain_action_lifted`（發出與解除網域處置）、`domain_statuses_refetch_queued`（重新擷取該網域帳號的貼文）、`instance_marked_available`（清除某個伺服器的失敗紀錄） |
| `report` | `report_assigned`、`report_unassigned`、`report_resolved`、`report_reopened`、`report_note_added`、`report_forwarded`（指派、取消指派、解決與重新開啟檢舉、新增筆記，以及轉送檢舉） |
| `user` | `registration_approved`、`registration_rejected`（註冊審查） |
| `setting` | `setting_changed`（實例設定）、`text_changed`（實例文字） |
| `rule` | `rule_added`、`rule_revised`、`rule_moved`、`rule_retired`（實例規則） |
| `role` | `role_granted`、`role_revoked`（為使用者授予與撤銷角色）、`role_created`、`role_updated`、`role_deleted`（建立、變更與刪除角色） |
| `appeal` | `appeal_upheld`、`appeal_overturned`（申訴審查；推翻時也會為該處置記錄 `account_action_lifted`，若是 `ban` 則記錄 `account_deletion_canceled`） |
| `signup_block` | `signup_block_added`、`signup_block_lifted`（新增與解除註冊封鎖） |
| `hook_decision` | `hook_decision_applied`（伺服器依外部審查的判定採取行動並留下狀態；執行者為系統）、`hook_decision_overridden`（版主取消了決策，包括推翻其申訴）、`hook_appeal_upheld`（版主維持了被拒貼文申訴的原判，決策維持不變） |
| `custom_emoji` | `custom_emoji_created`、`custom_emoji_image_replaced`、`custom_emoji_copied`、`custom_emoji_updated`、`custom_emoji_enabled`、`custom_emoji_disabled`、`custom_emoji_deleted`（註冊自訂表情符號、替換其圖片、從其他伺服器複製表情符號、變更、啟用、停用與刪除） |
| `webhook` | `webhook_created`、`webhook_updated`、`webhook_deleted`、`webhook_enabled`、`webhook_disabled`、`webhook_secret_rotated`（建立、變更、刪除、啟用與停用 webhook，以及重新產生其密鑰） |
| `announcement` | `announcement_created`、`announcement_updated`、`announcement_published`、`announcement_withdrawn`（建立、變更、發布與撤回公告） |
| `invite` | `invite_created`、`invite_revoked`（建立與撤銷邀請；目標名稱為邀請碼的前 4 個字元） |
| `billing_source` | `billing_import_queued`（手動開始帳單匯入；目標名稱為供應商名稱） |
| `cost_item` | `cost_item_created`、`cost_item_relabeled`、`cost_item_price_revised`、`cost_item_end_changed`、`cost_item_deleted`（登記支出、變更其名稱等、修訂其金額、變更其結束日期，以及刪除；目標名稱為項目名稱） |
| `supporter_program` | `supporter_program_enabled`、`supporter_program_disabled`（啟用與停用收費；目標名稱為金流服務名稱） |
| `supporter_plan` | `supporter_plan_created`、`supporter_plan_revised`、`supporter_plan_retired`（建立、變更與停用支持者方案；目標名稱為方案名稱） |
| `supporter_subscription` | `supporter_subscription_ended`（管理員立即結束訂閱；目標名稱為方案名稱與使用者帳號） |
| `custom_emoji_request` | `custom_emoji_request_submitted`、`custom_emoji_request_withdrawn`、`custom_emoji_request_approved`、`custom_emoji_request_rejected`（提出、撤回、核准與拒絕自訂表情符號申請；目標名稱為 `:shortcode:`） |
| `payment_provider` | `payment_reconcile_queued`（手動開始與金流服務對帳；目標名稱為服務名稱） |
| `oauth_client` | `oauth_client_disabled`、`oauth_client_enabled`（停用與撤銷停用 OAuth 用戶端；停用會撤銷其所有權杖；目標名稱為用戶端名稱與 `client_id`） |
| `oauth_client_host` | `oauth_client_host_denied`、`oauth_client_host_allowed`（拒絕與撤銷拒絕 Client ID Metadata Document 的主機；拒絕會撤銷該主機用戶端的權杖；目標名稱為主機） |
| `instance` | `service_closure_announced`、`service_closure_withdrawn`、`service_closure_closed`、`service_closure_delivery_completed`（公告服務關閉或變更日期、撤回公告、關閉，以及完成關閉通知的傳送；目標名稱為 `service_closure`） |
| `voice_room` | `voice_room_ended`（版主結束了開放中的語音聊天房間；由於撰寫名稱的帳號被清除時名稱也會消失，目標名稱為房間 ID；檢舉詳細資料會顯示名稱） |
| `link_card_domain` | `link_card_domain_fetch_stopped`、`link_card_domain_images_stopped`、`link_card_domain_rule_removed`（為網域設定連結卡片爬蟲的規則，以及移除規則；請參閱[連結卡片網域](instance-settings.md#link-card-domains)） |
| `login_provider` | `login_provider_created`、`login_provider_updated`、`login_provider_deleted`、`login_provider_enabled`、`login_provider_disabled`、`login_provider_secret_rotated`（新增、變更、刪除、啟用與停用外部登入服務，以及替換其用戶端密鑰；目標名稱為服務的顯示名稱） |

清單會顯示目標是否仍然存在。在保留期間後消失的帳號、檢舉、申訴與註冊封鎖，以及已刪除的角色、自訂表情符號與 webhook，只會顯示記錄時的名稱，沒有連到畫面的連結。

```sh
my admin audit list
my admin audit list -actor alice -action domain_action_issued
my admin audit list -target-kind domain -target-id spam.example
```

在管理 API 中：`GET /api/v1/admin/audit_log`（以 `actor_account_id`、`action`、`target_kind` 與 `target_id` 篩選）。它需要 `view_audit` 權限。

- 一列包含執行者（`user` 時為使用者，CLI 時為 `cli`）、操作、目標類型與 ID、目標的顯示名稱、原始紀錄的 ID，以及時間。目標 ID 是帳號、檢舉、申訴、使用者、規則、角色或註冊封鎖的 ID；網域處置為網域；設定與文字則為鍵。
- 目標的顯示名稱（`@alice`、`@bob@example.com`、網域、設定鍵、規則的文字；文字為其鍵，角色為其鍵與使用者帳號，註冊封鎖為其類型與值）是記錄時取得的副本，因此在目標消失後仍然可以閱讀。API 的每一列也會回傳目標是否仍然存在（`target_exists`），對於已消失的帳號、檢舉、申訴與註冊封鎖為 `false`。Web 的稽核記錄對已消失的目標只會顯示顯示名稱，不附連結。
- 理由與筆記的文字不會被複製。請透過原始紀錄（處置、指派、解決、重新開啟、筆記、審查，或設定或規則的版本）的 ID 查詢。
- 清單依最新排序，使用 keyset 分頁（CLI 中為 `-before <entry id>`）。
- 沒有更新或刪除稽核記錄的 API 或 CLI。舊的列會在[保留期間](#record-retention-and-holds)過後移除。

<!-- source: record-retention-and-holds bd806b3bbda7 -->
## 紀錄保留與保全

每種紀錄的保留期間過後，每小時執行的任務（`moderation.purge_records`、`identity.purge_origins` 與 `voice.purge_records`）會移除審核紀錄。你可以在實例設定中變更期間，但不能低於下限（[實例設定](instance-settings.md)）。變更後的期間從下一次執行開始生效。已解除的保留保全連同其理由，會在解除後經過審核處置的保留期間時移除。

| 紀錄 | 設定 | 預設 | 下限 | 起算點 |
| --- | --- | --- | --- | --- |
| 登入的 IP 位址與 User-Agent | `session_origins_retention_days` | 180 天 | 90 天 | 登入時 |
| 註冊的 IP 位址 | `registration_origins_retention_days` | 180 天 | 90 天 | 註冊時 |
| 檢舉（留言、目標貼文、筆記、指派、解決、重新開啟） | `reports_retention_days` | 365 天 | 180 天 | 最後一次解決 |
| 對帳號的處置（理由、筆記、目標貼文、解除）與申訴 | `moderation_actions_retention_days` | 365 天 | 180 天 | 處置結束時 |
| 稽核記錄 | `audit_log_retention_days` | 730 天 | 365 天 | 記錄時 |
| 註冊封鎖 | 無（無法變更） | 30 天 | 不適用 | 解除與到期中較早者 |
| 語音聊天房間的參與者（參與、發言者與聽眾、舉手、邀請、移出） | `voice_participants_retention_days` | 90 天 | 30 天 | 房間結束時 |
| [決策 hook](decision-hooks.md#the-decisions) 的決策 | `hook_decisions_retention_days` | 90 天 | 30 天 | 決策時 |

- 待處理的檢舉絕不會被移除。處置所引用的檢舉（處置的 `report_id`）會保留到該處置被移除為止。
- 處置的結束是解除與到期中較早者。對於已申訴的處置，若審查比那更晚，則結束時間為審查時間，而申訴等待審查期間處置會被保留。`warn` 與 `delete_statuses` 沒有持續的效果，因此結束時間就是發出的時間。沒有結束的處置（`ban`、沒有結束時間的 `suspend` 等）在解除前不會被移除。
- 檢舉或處置所指向的貼文，即使作者已刪除（從畫面上消失），在該檢舉或處置被移除之前，清除任務也不會連同附加的媒體實際刪除它們。遠端貼文也會保留超過保留期間（包括實例設定 `remote_status_max_retention_days`）。
- 既沒有解除也沒有結束時間的註冊封鎖，在生效期間會一直保留。
- 檢舉所指向的語音聊天房間的參與者紀錄，會保留到該檢舉被移除為止。房間本身（名稱、說明、主持人以及如何結束）絕不會被移除。
- 一次任務執行每種紀錄只會移除有上限的列數（處置、檢舉與註冊封鎖為每次 100 列、共 20 次；稽核記錄為每次 1,000 列、共 20 次）。其餘的由下一次執行移除。

<!-- source: holds f3bf57c2c258 -->
### 保全

當法院命令禁止刪除（日本《資訊流通平台處理法》第 16 條），或正在處理揭露請求時，你可以針對個別帳號停止刪除。

```sh
my admin retention hold -reason "court order 2026-123" @alice
my admin retention list -account @alice
my admin retention release <hold id>
```

- 對於受到保全的帳號，其處置與申訴、關於它的檢舉與它提出的檢舉、這些的稽核記錄、登入與註冊的 IP 位址、它的貼文，以及它參與或有所行動的語音聊天房間紀錄，都不會在保留期間過後移除。從該帳號的永久停權建立的註冊封鎖，在解除或到期 30 天後也會保留。
- 你可以對一個帳號設定多個保全。全部解除後，超過期間的紀錄會在下一次執行時移除。解除已解除的保全不會有任何變更。
- 保全與解除只能透過 CLI 進行，紀錄中的執行者為 CLI。
- 已刪除帳號（`ban` 到期的帳號、使用者自行刪除的帳號，或被其伺服器刪除的遠端帳號）的資料，會在刪除 7 天後由每小時執行的任務 `account.erase_deleted` 清除。遠端帳號的貼文也會一併清除，包括本站使用者加入書籤或回覆過的貼文。只因擷取時回應 `410 Gone`、而沒有收到其本身簽章的 `Delete` 而被標示為已刪除的遠端帳號，會先再擷取一次。第二次 `410` 就會清除它。若其伺服器再次提供它，它會連同貼文一起回來，什麼都不會被清除。其他任何回應（`404` 或沒有回應）會等待之後的執行，刪除 30 天後無論如何都會清除該帳號。受到保全的帳號不會被清除。檢舉或處置所指向的貼文會保留到該紀錄被移除為止。帳號的 URI、使用者名稱與處置紀錄會保留。該帳號為其語音聊天房間設定的名稱與說明會被清空；房間本身會保留。

<!-- source: exporting-for-disclosure-requests 89f1074dd950 -->
### 為揭露請求匯出

對於揭露請求（GDPR 第 15 條、日本《個人資訊保護法》第 33 條），請將帳號的所有審核紀錄匯出為 JSON 來回應。

```sh
my admin account export-moderation @alice > alice-moderation.json
```

- 匯出內容包括對該帳號的處置（包括私人筆記、解除、申訴與審查）、關於該帳號的檢舉（留言、目標貼文、引用的規則、版主的筆記、指派、解決、重新開啟）、該帳號提出的檢舉（只有每件是待處理還是已解決），以及這些的稽核記錄。
- 檢舉者的身分（提出檢舉的帳號，以及收到的檢舉的 URI 與來源伺服器）不會被匯出。交出匯出內容之前，請確認檢舉的留言中沒有任何能識別檢舉者的內容。
- 版主以使用者 ID 匯出。請先設定保全，以免在處理請求期間紀錄被移除。
