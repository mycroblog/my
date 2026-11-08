<!-- source: privacy-policy-template 9966bbc6844c -->
# 隱私權政策範本

一份可依你的伺服器調整的隱私權政策：說明這個伺服器保存什麼、為什麼保存、保存多久，以及誰會收到這些資料。

**這是一份範本，不是法律意見**。它描述的是軟體在預設設定下的行為。它無法得知你如何營運你的實例，也不會判斷哪些法律適用於你。政策必須說明哪些內容，取決於你所在的司法管轄區以及你的使用者所在的司法管轄區（例如歐盟的 GDPR、英國 GDPR，或日本的個人資訊保護法）。發布之前，請針對你的司法管轄區請人確認，並把下方的法律依據和跨境傳輸聲明視為需要釐清的問題，而不是答案。

搭配使用的處理活動紀錄請參閱[處理活動紀錄範本](processing-record-template.md)。

<!-- source: how-to-use-it 9cb843cc1ba2 -->
## 使用方式

1. 把[範本](#the-template)複製到一個檔案 `privacy-policy.md` 中。
2. 替換每一個 `{{...}}`。大括號中的值（例如 `{{180 days}}`）是軟體的預設值：請對照你的實例確認它（[期間與對應的設定](#the-periods-and-their-settings)），然後移除大括號。
3. 以 `{{IF ...}}` 開頭的列或句子只適用於該配置（[你選擇的服務](#the-services-you-choose)）。不適用時請刪除它；適用時請移除該標記。
4. 確認沒有任何遺漏：`grep -n '{{' privacy-policy.md` 不會輸出任何一行。
5. 以 `my instance set privacy_policy - < privacy-policy.md` 發布，或在管理介面「條款與政策」中的「隱私權政策」頁面（`/admin/documents/privacy-policy`）發布。讀者可以在 `/about/privacy` 找到它（[實例設定](instance-settings.md#terms-and-privacy-policy)）。

撰寫了隱私權政策之後，註冊畫面會加上一個必須勾選的同意核取方塊。這個核取方塊表示對方讀過了這份文件。在你的法律要求取得同意的情況下，請不要把它當作對資料處理的同意。

這段文字在所有語言中都是同一個值。如果你的讀者使用多種語言，請把你需要的語言都寫進這一份文字中。

當你變更下方的設定或服務時，以及在每次版本說明提到個人資料的升級之後，請重新檢視這份政策。

<!-- source: the-periods-and-their-settings 111d5a4b6af8 -->
## 期間與對應的設定

`my instance list` 會顯示每一項實例設定，以及它是否為預設值。期間的變更會在其每小時任務的下一次執行時生效。超出範圍的值會被拒絕。

| 紀錄 | 預設值 | 實例設定 | 範圍（天） | 起算點 |
| --- | --- | --- | --- | --- |
| 登入時的 IP 位址和 User-Agent | 180 天 | `session_origins_retention_days` | 90-3650 | 登入時 |
| 註冊時的 IP 位址 | 180 天 | `registration_origins_retention_days` | 90-3650 | 註冊時 |
| 檢舉 | 365 天 | `reports_retention_days` | 180-3650 | 最後一次處理完成時 |
| 對帳號的處置及申訴 | 365 天 | `moderation_actions_retention_days` | 180-3650 | 處置結束時 |
| 稽核日誌 | 730 天 | `audit_log_retention_days` | 365-3650 | 記錄時 |
| 誰參與了語音聊天房間 | 90 天 | `voice_participants_retention_days` | 30-3650 | 房間結束時 |
| 外部審查的決定（有使用時） | 90 天 | `hook_decisions_retention_days` | 30-3650 | 決定時 |
| 本站沒有任何內容參照的遠端貼文 | 90 天 | `remote_status_retention_days` | 1-3650 | 貼文發布時 |
| 任何遠端貼文 | 無限制（`0`） | `remote_status_max_retention_days` | 0-3650 | 貼文發布時 |
| 只經由中繼站送達的遠端貼文 | 7 天 | `relay_status_retention_days` | 1-3650 | 貼文發布時 |
| 遠端媒體的副本 | 1 天；`remote_media_policy` 為 `none` 時不保存 | `remote_media_cache_ttl_seconds`、`remote_media_policy` | 60-31536000 秒 | 擷取時 |
| 貼文的翻譯 | 30 天 | `translation_retention_days` | 1-3650 | 翻譯時。閱讀翻譯不會延長期間，貼文刪除時翻譯也會一併刪除 |

以下是部署的設定（[設定參考](../reference/configuration.md)），而不是實例設定：

| 紀錄 | 預設值 | 設定 |
| --- | --- | --- |
| 沒有附加到任何內容的上傳檔案 | 24 小時 | `media_unattached_retention` |
| 被新版本取代之後的舊版圖片 | 30 天 | `media_replaced_retention` |
| 最終失敗的任務 | 30 天 | `dead_job_retention` |
| 未成功的任務執行紀錄 | 30 天 | `job_attempt_retention` |

以下期間由軟體固定，沒有任何設定可以變更：

| 紀錄 | 期間 |
| --- | --- |
| 抹除已刪除的帳號 | 刪除後 7 天；遠端帳號若只有其伺服器回傳的 `410 Gone` 表示已刪除，而該伺服器之後不再回應，最長為 30 天 |
| 刪除已停用的帳號 | 停用後 30 天 |
| 註冊封鎖 | 解除或到期後 30 天；兩者皆無時，在生效期間一直保留 |
| 工作階段、權杖和授權碼 | 到期後 30 天 |
| 資料匯出的封存檔，以及資料匯入 | 7 天 |
| 從其他伺服器收到的活動紀錄 | 30 天，文件本身為 7 天 |
| 傳送到其他伺服器的活動紀錄 | 30 天；貼文的傳送紀錄保留到該貼文或其作者刪除後 30 天 |
| 遠端帳號代稱的檢查 | 90 天，每個帳號保留最新的一筆 |
| webhook 訊息與傳送 | 14 天 |
| 付款 webhook 的收件紀錄 | 45 天 |
| 每小時的錯誤計數與錯誤通知 | 90 天 |

[保留凍結](moderation.md#holds)會暫停對單一帳號的抹除，因此對於受到凍結的帳號，上述每個期間都可能更長。如果你會使用保留凍結，請像範本一樣說明這一點。

<!-- source: records-with-no-automatic-end 90a13d1f648e -->
## 沒有自動結束的紀錄

伺服器只有在帳號被抹除時才會移除這些紀錄，或者從不移除。法律可能會要求你說明保留它們的理由，或以其他方式移除它們，因此請決定你的政策要如何描述每一項。

- 有效帳號的資料列：電子郵件地址、過去的個人檔案版本、過去的貼文版本、通知、追蹤關係，以及被拒絕之使用者的註冊申請。沒有人刪除的帳號會一直保留。
- 抹除之後仍然保留的內容：帳號的 URI 和使用者名稱（以免兩者被重複使用）、帳號的生命週期事件、貼文已被刪除的紀錄、審核紀錄（直到其各自的期間結束）、邀請與角色授予、帳號搬家紀錄、已移除名稱與說明的語音聊天房間。
- 支持者紀錄（訂閱、狀態變化、付款、退款、退單），為了會計以及與付款服務提供者對帳而保存。伺服器不會刪除付款服務提供者那邊的客戶：如果你的義務允許，請在提供者的儀表板中刪除。
- 沒有結束時間的處置（`ban`，或沒有結束時間的 `suspend`），直到解除為止。尚未處理完的檢舉。
- 遠端帳號，直到其伺服器刪除它們為止。
- 每個錯誤群組的最新報告，會被該群組的下一份報告取代。它是依模式清理的，因此可能會殘留部分使用者文字。
- 備份、你的日誌，以及你的平台和代理的日誌。伺服器不會管理這些：[備份](backup.md#how-often-and-where)說明了要保留的世代，而還原備份會讓已抹除的資料回來。

<!-- source: the-services-you-choose 81461216c966 -->
## 你選擇的服務

每個會收到個人資料的服務，只有在你開啟它時才會出現在政策中。請依照這份列表填寫第 6 節的表格，並閱讀該服務本身的政策和合約：資料送達之後該服務如何處理它，不是這份文件能告訴你的。

| 標記 | 設定 | 服務會收到的內容 |
| --- | --- | --- |
| 主機代管 | `database`、`blob`、`queue`、`key_manager` | 資料庫和 blob 儲存中的所有內容，以提供者儲存它的任何形式。雲端佇列中的任務內容包含 ID 和類別，不包含地址、訊息內文或機密資訊。 |
| `{{IF cdn}}` | `media_origin` | 讀者的 IP 位址和對媒體的請求（[CDN](cdn.md)）。 |
| 代理 | 你的反向代理或負載平衡器，伺服器需要它來處理 TLS | 每個請求及其 IP 位址，依你設定的記錄方式保存。 |
| `{{IF mail}}` | `mail` 為 `smtp` 或 `ses` | 收件人地址、主旨和內文。通知電子郵件包含與通知相同的文字（[電子郵件](email.md)）。 |
| `{{IF captcha}}` | `captcha` 不是 `none` | 每個開啟登入、註冊或密碼重設申請頁面的人，其 IP 位址、瀏覽器與裝置資訊、Cookie，以及與小工具的互動，直接從他們的瀏覽器送出。伺服器只會把權杖送給該服務，不會加上 IP 位址（[CAPTCHA](captcha.md#what-to-write-in-the-privacy-policy)）。 |
| `{{IF login providers}}` | [登入提供者](login-providers.md) | 提供者會看到登入行為。伺服器會收到提供者對該使用者的識別碼並保存它，並在註冊時使用提供者給的電子郵件地址和使用者名稱。 |
| `{{IF stripe}}` | `payments = stripe` | 預先填入 Checkout 的電子郵件地址、方案，以及本站的訂閱 ID。Stripe 會在自己的頁面上收集付款資訊（[Stripe](payments-stripe.md#card-data-and-pci-dss)）。 |
| `{{IF payjp}}` | `payments = payjp` | 電子郵件地址和方案。瀏覽器會在伺服器的付款頁面上載入 payjp.js，卡片資料會輸入到 PAY.JP 的 iframe 中，姓名和電子郵件地址則會為了 3-D Secure 送到 PAY.JP（[PAY.JP](payments-payjp.md#card-data-and-pci-dss)）。 |
| `{{IF error reports}}` | `errors_exporter` | 已清理、不含帳號、IP 位址、內文或標頭的報告。清理是依模式比對的，因此可能會殘留部分使用者文字（[錯誤回報](error-reporting.md#what-scrubbing-does-and-does-not-guarantee)）。 |
| `{{IF telemetry}}` | `metrics_exporter`、`traces_exporter` | 不含帳號或用戶端 IP 位址的指標和 span。對外請求的 span 會包含遠端伺服器的主機名稱（[執行](running.md)）。 |
| `{{IF voice media server}}` | `voice` 為 `livekit` 或 `sora` | 參與者的 IP 位址、他們發言時的音訊、工作階段 ID 和房間 ID。不會送出帳號的 ID（[語音聊天](voice.md)）。 |
| `{{IF turn}}` | `voice_stun_urls`、`voice_turn` | STUN 和 TURN 伺服器會看到參與者的 IP 位址，TURN 伺服器還會轉送他們加密過的音訊。 |
| `{{IF webhooks}}` | [伺服器 webhook](webhooks.md) | 你的接收端會收到事件內容。部分事件包含申請者的電子郵件地址或檢舉的留言。 |
| `{{IF follow review}}` | 已設定 `plugin_inbound_follow_address` 且 `hook_inbound_follow_mode` 不是 `off` | 來自其他伺服器的每個追蹤所涉及的兩個帳號的位址，在追蹤送達時由伺服器送出（[決策掛鉤](decision-hooks.md)）。記錄的決定會保存該活動的位址以及是哪個帳號送出的。 |
| `{{IF sign-up review}}` | 已設定 `plugin_signup_check_address` 且 `hook_signup_check_mode` 不是 `off` | 每個沒有邀請的申請者的電子郵件地址和 IP 位址，在他們註冊時由伺服器送出（[決策掛鉤](decision-hooks.md#sign-ups)）。記錄的決定兩者都不保存。除非外掛由你自己執行，否則請在第 6 節中寫出該服務的名稱。 |
| `{{IF post review}}` | 已設定 `plugin_local_status_address` 且 `hook_local_status_mode` 不是 `off` | 建立未滿 `hook_local_status_account_age_days`（預設 7 天；`0` 表示所有帳號）之帳號的每則 `public`、`local` 和 `unlisted` 貼文的文字，包括輸入時與轉譯後的版本，以及其內容警告、語言、是否標記為敏感內容、投票選項、圖片說明、主題標籤、提及了多少個帳號、它將得到的位址、它所回覆和引用之貼文的位址、作者的位址和使用者名稱，以及帳號的建立時間，在發文時以及每次編輯時由伺服器送出（[決策掛鉤](decision-hooks.md#local-posts)）。`private` 和 `direct` 貼文從不送出。記錄的決定不保存任何文字。對被拒絕的貼文提出申訴的作者會送出它的文字，伺服器會連同決定一起保存。除非外掛由你自己執行，否則請在第 6 節中寫出該服務的名稱。 |
| `{{IF profile review}}` | 已設定 `plugin_inbound_actor_address` 且 `hook_inbound_actor_mode` 不是 `off` | 其他伺服器上帳號的公開個人檔案，每次變更時送出：顯示名稱、簡介、個人檔案欄位、大頭貼和頁首圖片的位址（不含圖片）、帳號的位址、伺服器和使用者名稱，以及該帳號在本站是否為新帳號（[決策掛鉤](decision-hooks.md#profiles-from-other-servers)）。不會送出你伺服器上帳號的任何內容。記錄的決定會保存帳號的位址和個人檔案的摘要值。 |
| `{{IF profile review}}` | 已設定 `plugin_local_profile_address` 且 `hook_local_profile_mode` 不是 `off` | 顯示名稱、輸入時與轉譯後的簡介、個人檔案欄位、大頭貼或頁首是否變更、帳號的位址和使用者名稱，以及帳號的建立時間，在建立未滿 `hook_local_profile_account_age_days`（預設 7 天；`0` 表示所有帳號）的帳號每次變更它們時由伺服器送出（[決策掛鉤](decision-hooks.md#local-profiles)）。不會送出任何圖片。記錄的決定不保存其中任何內容。除非外掛由你自己執行，否則請在第 6 節中寫出該服務的名稱。 |
| `{{IF external review of posts}}` | 已設定 `plugin_inbound_status_address` 且 `hook_inbound_status_mode` 不是 `off` | 其他伺服器送來、與本站帳號相關的貼文及其修訂版本，包括僅追蹤者（`private`）的貼文：每種語言的文字、內容警告、是否標記為敏感內容、發布時間、位址和 URL、所回覆貼文的位址、投票選項、主題標籤、附件的位址和說明（不含媒體）、作者及其伺服器、可見範圍、提到了多少個帳號以及其中有多少個在本站，以及本站是否有人追蹤該作者（[決策掛鉤](decision-hooks.md#what-is-sent-and-what-it-costs)）。私訊貼文只有在本站有人追蹤作者時才會送出，或在設定 `hook_inbound_status_unfollowed_direct` 時不論作者為何都會送出。 |
| `{{IF media review}}` | 已設定 `plugin_media_check_address` 且 `hook_media_check_mode` 不是 `off` | 每個上傳檔案經處理後最小的圖片，不含中繼資料，從不送出原始檔案，並附上檔案的用途及其種類、大小和長度（[決策掛鉤](decision-hooks.md#what-mediacheck-reviews)）。記錄的決定會保存檔案的 ID 和上傳者。被拒絕的上傳檔案完全不會保存：如果你必須保存或通報它，就必須由你的外掛來做。除非外掛由你自己執行，否則請在第 6 節中寫出該服務的名稱。 |
| `{{IF translation}}` | `translation_providers` 不是空的 | 讀者要求翻譯時的貼文文字：內文、內容警告、投票選項和圖片說明。不會送出貼文的位址、作者、讀者、提及和連結。預設情況下，提供者會收到 `public`、`local` 和 `unlisted` 貼文。它和主機代管一樣，是作為你的服務提供者運作的，因此 `local` 貼文雖然不會聯邦，仍然會送到它那裡：請保留第 5 節中關於 `local` 的句子。為了自身用途而保存文字的提供者，不屬於這個意義上的服務提供者，政策必須說明這一點。 |
| `{{IF translation}}` 搭配 DeepL 的免費方案 | `translation_providers` 包含 `deepl`，且金鑰以 `:fx` 結尾 | 同上。DeepL 可能會永久保存文字和翻譯，政策必須說明這一點（[翻譯](translation.md#deepl)）。 |
| `{{IF translation}}` 搭配 Amazon Translate | `translation_providers` 包含 `aws` | 同上。除非你的帳號受到 AWS Organizations 的 AI 服務退出政策涵蓋，否則 Amazon 可能會儲存文字並用來改善其服務，而且可能存放在其他區域，政策必須說明這一點（[翻譯](translation.md#amazon-translate)）。 |
| `{{IF translation to private and direct}}` | `translation_provider_visibilities` 允許某個提供者處理 `private` 或 `direct` | 同上，針對這些貼文。只有在你允許時，才依提供者使用這個標記。它會在第 2 節和第 5 節中加上幾行，並擴大你對這些貼文的承諾範圍，因此請在變更設定之前公告。 |
| `{{IF translation of reports}}` | `translation_reports_to_server` | 版主要求翻譯時，檢舉的留言及其指出的貼文。每個提供者仍然只會收到它被允許的可見範圍，而留言視為 `private`：沒有任何提供者被允許處理 `private` 時，留言不會送到任何提供者。留言的翻譯不會被儲存。 |
| `{{IF browser translation}}` | `translation_browser` | 沒有任何內容會送到這個伺服器或提供者。瀏覽器會在讀者的裝置上翻譯，而瀏覽器的開發者如何處理則取決於該瀏覽器。 |

這些服務都是你的選擇，因此這份範本不會說明每個服務設立於何處，或受到什麼合約規範。請自行確認這兩點。

有兩項內容即使沒有你的設定，也會送到第三方：

- **Web Push**。使用者開啟它時，他們的瀏覽器會選擇推播服務，並把它的位址交給伺服器。訊息內文是針對該瀏覽器加密的，因此推播服務應該無法讀取它。推播服務看得到訂閱位址、每則訊息的大小和時間，以及你伺服器的聯絡資訊（VAPID subject），並會在瀏覽器離線時保存訊息最多 48 小時。
- **聯邦**。請參閱範本的第 5 節。`federation_mode` 可以關閉聯邦或限制其方向，而中繼站則由你自行加入（[中繼站](relays.md)）。

<!-- source: the-template 8b6bd2fc983a -->
## 範本

```markdown
Last updated: {{DATE}}

This policy describes how {{OPERATOR NAME}} ("we") handles personal data on {{INSTANCE NAME}} ({{https://INSTANCE DOMAIN}}).

# 1. Who is responsible

- Controller: {{OPERATOR NAME, POSTAL ADDRESS}}
- Contact for privacy questions: {{EMAIL OR FORM}}
- {{IF you have a representative in the EU or the UK: name and contact}}
- {{IF you have a data protection officer: name and contact}}

# 2. Whom this covers

This policy covers people with an account here, people who read pages here without an account, and people on other servers whose posts and profiles reach this server.

{{IF translation}} If you are on another server, the text of your posts that reach this server can go to the translation services in section 6 when a reader here asks for a translation. Your server cannot tell you when this happens.

{{IF external review of posts}} If you are on another server, your posts that relate to an account here, followers-only posts included, go with their text to the external review in section 6 before this server accepts them. It can stop a post or mark it, or file a report for our moderators; people decide on any action against an account.

# 3. What we keep, why, and for how long

"Until your account is erased" means: deleting your account hides it at once and erases it 7 days later. A deactivated account stays hidden, can be reactivated for 30 days, and is then deleted the same way.

| Data | Why | How long |
| --- | --- | --- |
| Email addresses | Signing in, confirming the address, resetting the password, and the email you choose to get | Until your account is erased |
| Password hash, two-factor secret (encrypted) and recovery codes (hashed), passkeys, the identifier of an account you linked at a login provider | Signing you in | Until your account is erased |
| Language, time zone, default post settings, notification settings, and other preferences | Running the service the way you asked | Until your account is erased |
| Registration application: the reason you gave, the invite you used, and the decision | Deciding who joins | Until your account is erased |
| IP address at registration | Preventing abuse | {{180 days}} from registration |
| IP address and browser name (User-Agent) at each sign-in | Security and preventing abuse | {{180 days}} from the sign-in |
| Sessions and app access (stored as hashes) | Keeping you signed in | 30 days after they expire. A session lasts 30 days |
| IP address, and for password-reset requests the email address, in rate limit counters | Protecting the service | Removed automatically, within twice the window of the limit |
| Profile: display name, bio, extra fields, avatar, header, aliases, and earlier versions of them | Showing your profile, which is public | Until your account is erased |
| Posts, their earlier versions, and attached media | Publishing what you write | Until you delete them, your automatic deletion policy deletes them, or your account is erased. {{IF you set a maximum for keeping posts}} At most {{N days}} after you publish them, unless your account may keep them longer. Uploads attached to nothing go after {{24 hours}} |
| Favourites, reactions, boosts, bookmarks, pins, poll votes, follows, blocks, mutes, lists | Running the features you use | Until you undo them, or your account is erased |
| Notifications and what you have read | Telling you what happened | Until your account is erased or the post they are about is removed |
| Web Push subscription: your browser's push address and keys | Sending you push notifications | Until you turn it off, the push service ends it, or your account is erased |
| Records that an email bounced or was reported as spam | Not sending mail that is unwanted or cannot arrive | Until your account is erased |
| Data export archives and data imports | Giving you your data, and bringing it in | 7 days |
| Reports: the comment, the posts and rules named, and moderators' notes | Moderation | {{365 days}} from the last resolution, or as long as an action refers to them. Open reports stay |
| Actions against accounts, and appeals | Moderation, and handling repeat abuse | {{365 days}} from the end of the action. An action with no end stays until it is lifted |
| Posts that a report or an action points to | Evidence for moderation | Hidden when deleted, and removed with that record |
| Audit log of staff actions, which can name accounts | Accountability of staff | {{730 days}} |
| Sign-up blocks: an email domain, a hashed email address, or an IP range | Preventing repeat abuse | While in effect, and 30 days after the lift or expiry |
| {{IF voice}} Voice chat rooms: name, description, hosts, and how they ended | Showing and moderating rooms | Kept. The name and description you wrote are emptied when your account is erased |
| {{IF voice}} Who took part in a room: participation, speakers, raised hands, invitations, removals | Moderating rooms | {{90 days}} from the end of the room, or as long as a report about the room. We record neither audio nor text chat |
| {{IF payments}} Supporter plans: the provider's IDs for you and your subscription, amounts, periods, and states. Not your card details | Running your subscription, and accounting | {{PERIOD AND REASON, e.g. as long as tax law requires}}. The link to the provider's customer goes when your account is erased |
| {{IF payments}} Your choice to appear in the list of supporters | Showing the list | Until the subscription ends or your account is erased |
| Copies of activities sent to and received from other servers | Delivering, retrying, and rejecting duplicates | 30 days. The first delivery of a post stays until 30 days after the post or its author is deleted |
| Posts and profiles of people on other servers | Showing them here | Posts: {{90 days}} when nothing here refers to them, {{7 days}} when only a relay brought them, and {{no limit}} otherwise. Profiles: until their server deletes the account. Then erased after 7 days |
| Copies of remote media | Showing it | {{Not kept, or the period you set}} |
| Server logs: request paths, which can contain a username or a post ID. Our server software writes no IP address in them | Running and debugging the service | {{PERIOD}} |
| {{IF proxy logs}} Logs of our proxy or load balancer: IP addresses and requests | Running and protecting the service | {{PERIOD}} |
| Error reports, with personal details removed by pattern | Fixing faults | The latest report of each kind of error stays until a newer one replaces it. Hourly counts: 90 days |
| {{IF webhooks}} Messages to our webhook receivers, which can carry an applicant's email address or a report's comment | Connecting our own tools | 14 days here. {{PERIOD AT THE RECEIVER}} |
| {{IF post review}} The text of a refused post that you send with an appeal, and the decision | Letting staff review the appeal | {{90 days}} from the refusal, and longer while you can still appeal or staff have not decided. Removed when your account is erased |
| {{IF translation}} Translations of posts that readers asked for, which every reader who may see the post can reuse | Showing a translation again without sending the post again | {{30 days}} from the translation, and no longer than the post |
| {{IF translation}} How many characters your requests sent for translation in a month | Keeping translation within its limits | Until the end of the following month, or until your account is erased |
| Backups of all the above | Restoring the service after a failure | {{PERIOD}}. Data you erased stays in a backup until the backup expires |

We can stop erasure for one account when a court order or a legal request requires it. The records of that account then stay until the hold is released.

After erasure we keep the account's address and username so that nobody can reuse them, the record that its posts were deleted, its moderation records until their period ends, invites and roles it took part in, moves it made, and {{IF payments}} the supporter records above.

# 4. Why we are allowed to

{{CHOOSE WITH YOUR ADVISER. A common starting point under GDPR Article 6(1), not a conclusion:}}

- Running your account and the features you use: {{performance of a contract, point (b)}}.
- Security, preventing abuse, moderation, the audit log, logs, and backups: {{legitimate interests, point (f): keeping the service safe and working}}.
- Storing and showing what people on other servers publish: {{legitimate interests, point (f)}}.
- Records the law makes us keep: {{legal obligation, point (c)}}.
- {{IF you rely on consent for anything: what, and how to withdraw it}}.

You need an email address {{OR a login provider}} to create an account here, and without one we cannot provide an account.

# 5. What other people and other servers receive

**This server is part of a federated network.** Other servers run by other people, in other countries, receive your data as part of how the service works, and their own policies apply to it. We cannot make them delete it.

| What | Who receives it |
| --- | --- |
| Your profile, including your avatar, header, and aliases | Anyone, on the web and through other servers |
| `public` and `unlisted` posts | Anyone with the link. The servers of your followers and of the people you mention. {{IF relays}} `public` posts also go to relays we joined, and through them to other servers |
| `local` posts | People signed in to this server, and your followers on other servers. {{IF translation}} When a reader asks for a translation, the text also goes to the translation services in section 6, which work for us. They do not receive the post's address or your handle |
| `private` posts | Your followers and the people you mention, on any server |
| `direct` posts | The people you mention, on any server. They are not end-to-end encrypted: we can technically read them |
| {{IF translation to private and direct}} {{`private` AND/OR `direct`, AS YOU ALLOWED}} posts | {{TRANSLATION SERVICES YOU ALLOWED THEM}}, when a reader who may see the post asks for a translation. They do not receive the post's address, your handle, the reader, or who is mentioned |
| Follows, blocks, boosts, favourites, reactions, poll votes, quotes | The other person's server. Boosts also go to your followers' servers |
| That your account moved, was deactivated, suspended, or deleted | Your followers' servers and, except for a move, other servers we know |
| A report you chose to forward | The reported account's server receives your comment and the posts named. It does not receive your name, IP address, or email address, so do not put those in the comment |
| {{IF translation of reports}} A report's comment and the posts it names | {{TRANSLATION SERVICES YOU ALLOWED `private` POSTS}} receive the comment, and the services in section 6 receive the posts as for a reader, when a moderator asks for a translation. They do not receive who reported, the report, or the posts' addresses |

When you edit or delete a post or delete your account, we tell those servers. Most honour it, but we cannot guarantee it, and copies, including ones made by others, can remain.

Other people here see what you publish. Staff with the right permission can reveal your email address and your registration and sign-in IP addresses, and each reveal is recorded in the audit log. Staff can also see email addresses of pending registrations.

Search engines and AI services: {{DESCRIBE YOUR search_indexing, indexable_default, AND ai_usage SETTINGS. By default we ask them not to index and not to use the content}}. These are requests, and not every crawler follows them. You can change whether your own profile appears in search in your profile settings.

{{IF the full-text search runs on another company's service}} Full-text search: the text of posts in search ({{public / local}} posts of people who chose to appear in search) goes to {{SERVICE}} to make them searchable, and is removed there when the post is deleted or you stop appearing in search. {{SAY HOW LONG ITS BACKUPS KEEP IT}}. {{IF you keep logs that hold search terms}} What you search for can appear in our request logs, which we keep for {{PERIOD}}.

Apps you authorize can read and do what the permissions you granted allow. Their own policies apply. You can revoke them in your settings.

{{IF the list of supporters is public}} If you choose to appear in the list of supporters, it shows your avatar, display name, handle, and plan.

{{IF browser translation}} A reader can also translate a post with the translation built into their browser. That sends nothing to us or to a translation service, and the translation is not stored.

# 6. Service providers

We use these services to run the instance. {{SAY WHETHER EACH ACTS ON YOUR BEHALF AND WHAT CONTRACT APPLIES.}}

| Service | Purpose | Data | Location | Their policy |
| --- | --- | --- | --- | --- |
| {{HOSTING PROVIDER}} | Server, database, and file storage | Everything in section 3 | {{COUNTRY}} | {{LINK}} |
| {{IF cdn}} {{CDN PROVIDER}} | Delivering media | IP address and requests for media | {{COUNTRY}} | {{LINK}} |
| {{IF mail}} {{MAIL PROVIDER}} | Sending email | Your address, and the subject and text of each email | {{COUNTRY}} | {{LINK}} |
| {{IF captcha}} {{CAPTCHA SERVICE}} | Stopping automated sign-ins, sign-ups, and password-reset requests | IP address, browser and device information, cookies, and interactions with the widget. Your browser sends these directly, from the sign-in, sign-up, and password-reset request pages only | {{COUNTRY}} | {{LINK}} |
| {{IF login providers}} {{PROVIDER}} | Signing in with an account there | That you sign in. We receive their identifier for you, and at sign-up your email address and username | {{COUNTRY}} | {{LINK}} |
| {{IF follow review}} {{FOLLOW REVIEW SERVICE}} | Screening follows from other servers for abuse | The addresses of your account and of the account on the other server, when someone there follows you | {{COUNTRY}} | {{LINK}} |
| {{IF sign-up review}} {{SIGN-UP REVIEW SERVICE}} | Screening sign-ups for abuse | The email address and IP address you sign up with, unless you sign up with an invite | {{COUNTRY}} | {{LINK}} |
| {{IF post review}} {{POST REVIEW SERVICE}} | Screening new posts and their edits for abuse | Your `public`, `local`, and `unlisted` posts, each time you post or edit one, while your account is younger than {{7 days}}: their text, content warning, language, whether they are marked sensitive, poll options, image descriptions, hashtags, and how many accounts they mention, their address and the addresses of the posts they reply to or quote, your address and username, and when your account was created. Not your media | {{COUNTRY}} | {{LINK}} |
| {{IF profile review}} {{PROFILE REVIEW SERVICE}} | Screening the profiles of accounts on other servers for abuse | Nothing about your account on this server. The public profile of an account on another server when this server takes in a change of it: its display name, bio, profile fields, the addresses of its avatar and header image, its address and username | {{COUNTRY}} | {{LINK}} |
| {{IF profile review}} {{PROFILE REVIEW SERVICE}} | Screening changes of profiles for abuse | Your display name, bio, and profile fields, and whether you changed your avatar or header, each time you change them while your account is younger than {{7 days}}, with your address and username and when your account was created. Not your images | {{COUNTRY}} | {{LINK}} |
| {{IF media review}} {{MEDIA REVIEW SERVICE}} | Screening uploads for abuse before anyone can see them | A small processed copy of each image you upload, or of the poster or cover of a video or audio file, without its metadata, and what the file is for. Not the original, its description, or your account | {{COUNTRY}} | {{LINK}} |
| {{IF translation}} {{TRANSLATION SERVICE}} | Translating a post when a reader asks | The text of the post: its body, content warning, poll options, and image descriptions. Not its address, its author, the reader, mentions, or links. Text sent to the service cannot be recalled, and deleting the post does not remove it there: {{HOW LONG THE SERVICE KEEPS IT}} | {{COUNTRY}} | {{LINK}} |
| {{IF external review of posts}} {{REVIEW SERVICE}} | Reviewing posts from other servers before they are accepted | The post as its server sent it, followers-only posts included: its text and language, content warning, whether it is marked sensitive, when it was published, its address, the address of the post it replies to, poll options, hashtags, its author, its visibility, attachments' addresses and descriptions, how many accounts it names and how many of them are here, and whether anyone here follows its author. Direct posts {{only when someone here follows their author, or from any author if you turned on `hook_inbound_status_unfollowed_direct`}}. No post of this server's accounts | {{COUNTRY}} | {{LINK}} |
| {{IF stripe}} Stripe | Taking payments for supporter plans | Your email address and plan. You enter payment details on Stripe's pages | {{COUNTRY}} | <https://stripe.com/privacy> |
| {{IF payjp}} PAY.JP | Taking payments for supporter plans | Your email address, cardholder name, and plan. You enter card details in PAY.JP's frame, whose script runs on our payment pages | {{COUNTRY}} | {{LINK}} |
| {{IF error reports}} {{ERROR TRACKER}} | Fixing faults | Error reports with personal details removed by pattern | {{COUNTRY}} | {{LINK}} |
| {{IF telemetry}} {{METRICS OR TRACING SERVICE}} | Watching the health of the service | Measurements, and the names of other servers we contact | {{COUNTRY}} | {{LINK}} |
| {{IF voice media server}} {{VOICE SERVICE}} | Carrying voice chat | Your IP address and audio while you speak in a room, and a session ID. Not your account | {{COUNTRY}} | {{LINK}} |
| {{IF turn}} {{STUN OR TURN SERVICE}} | Connecting voice chat | Your IP address, and encrypted audio when relayed | {{COUNTRY}} | {{LINK}} |

{{IF peer-to-peer voice}} In voice chat rooms, your browser connects directly to other participants, so they can see your IP address.

If you turn on push notifications, your browser's maker or its push service delivers them. The content is encrypted for your browser, but the service sees your browser's push address and when messages are sent.

# 7. Transfers abroad

Delivery to other servers (section 5) sends your data to servers abroad, wherever those servers are. We do not choose or know their countries in advance, and we cannot limit delivery by country.

{{YOUR STATEMENT ON THIS AND ON THE SERVICES OF SECTION 6: which transfer rules you consider apply, and the safeguards you rely on, if any. Settle this with your adviser: whether federated delivery is a restricted transfer is not settled, and this template takes no position.}}

# 8. Cookies and storage in your browser

We use no advertising or analytics cookies. Our pages use these strictly to work:

| Name | Purpose | Lasts |
| --- | --- | --- |
| `__Host-my_session` | Keeps you signed in to the sign-in pages | Up to 30 days |
| `__Host-my_rt` | Keeps the app signed in | Up to 30 days |
| `__Host-my_csrf` | Protects forms against forged requests | Until you close your browser |
| `__Host-my_2fa` | Carries a sign-in to the two-factor step | 5 minutes |
| `__Host-my_external` | Carries a sign-in through a login provider | At most 30 minutes |

The app also stores data in your browser only: display settings, recently used emoji and hashtags, the draft of a post (for the tab), and the newest posts of your home timeline for offline reading, without followers-only and direct posts. The drafts and the timeline are removed when you sign out.

{{IF captcha}} The CAPTCHA service sets its own cookies on the sign-in, sign-up, and password-reset request pages.

{{IF payjp}} PAY.JP's script runs on our payment pages and may set its own cookies. {{CHECK AGAINST PAY.JP'S POLICY.}}

# 9. How we protect it

Passwords are stored as Argon2id hashes. Two-factor secrets and the keys that sign your activities are encrypted, and the encryption key is kept outside the database. Session and access tokens are stored as hashes. Traffic uses HTTPS. {{YOUR MEASURES: encryption at rest and of backups, who has access, and how you handle a breach.}}

# 10. Your rights

{{ADJUST TO THE RIGHTS OF YOUR JURISDICTION. Under the GDPR these are access, rectification, erasure, restriction, objection, and portability, and the right to withdraw consent.}}

- **See and take your data:** your settings offer a data export (your profile, your posts, and your moderation records) and CSV files of follows, blocks, mutes, muted domains, and bookmarks. For everything else, including your email and IP addresses, write to us.
- **Correct it:** edit your profile and account settings, or write to us.
- **Delete it:** delete your account in your settings, or deactivate it first. This is what section 3 and section 5 describe.
- **Move:** you can move your followers to an account on another server.
- **Limit it:** choose your notifications in your settings. Every notification email has an unsubscribe link. You can also object to or ask us to restrict a use of your data by writing to us.
- **Reports about you:** we do not reveal who reported you.

Write to {{CONTACT}}. We answer within {{one month, or the period your law sets}}. You can complain to a data protection authority: {{YOUR AUTHORITY, OR "the authority where you live"}}.

We keep some records even when you ask us to delete them, where we need them for moderation or the law requires them (section 3).

# 11. Children

{{YOUR MINIMUM AGE AND HOW YOU APPLY IT. The server does not ask for a birth date or check age.}}

# 12. Automated decisions

We make no decisions about you by automated means that have legal or similarly significant effects, {{CONFIRM FOR YOUR USE. These are automatic: a sign-up that matches a block, or that an external review answers about, is refused or sent for review, an upload that an external review rejects is refused, {{IF post review}} a post that an external review refuses, which you can appeal to our staff, {{IF profile review}} a change of your profile that an external review refuses, which you can change and save again, the CAPTCHA check, and rate limits. Your staff decide the rest.}}

# 13. Changes

We change this policy when what we do changes, and show the new date at the top. {{HOW YOU TELL PEOPLE, for example an announcement.}} If we close the service, we tell you in advance, with a deadline for taking your data. {{HOW YOU TELL PEOPLE, for example by email and an announcement.}}
```
