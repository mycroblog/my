<!-- source: instance-settings 4eb467f0dbca -->
# 實例設定

這些設定決定你的實例如何營運：註冊模式、貼文長度、預設語系、誰能看到時間軸、實例名稱、說明、服務條款、規則、主題和自訂表情符號。

<!-- source: configuration-and-instance-settings a0b0908cef1e -->
## 設定與實例設定

| | 設定 | 實例設定 |
| --- | --- | --- |
| 內容 | 部署的構成方式：角色、轉接器、端點、主機名稱、密鑰 | 實例的營運方式：註冊模式、貼文長度上限、預設語系、誰能看到時間軸、實例名稱、說明和服務條款 |
| 位置 | 環境變數 `MY_*` 和設定檔（[設定參考](../reference/configuration.md)） | 資料庫。每次變更都會保留為一個修訂版本 |
| 變更 | 在行程重新啟動時生效 | 透過 `my instance set`、網頁用戶端的管理畫面或用戶端 API 立即生效（[管理畫面與用戶端 API](#admin-screens-and-the-client-api)） |
| 未設定時 | 結構描述中的預設值（如果該鍵為必要，伺服器不會啟動） | 程式碼中定義的預設值 |

- **沒有任何鍵同時存在於兩者中**。 兩者都不會覆寫對方。你無法透過環境變數或設定檔提供實例設定，也無法將設定值存放在資料庫中。
- 每個部署固定、會改變行程結構的值屬於設定；在實例執行期間會變更的值屬於實例設定。

<!-- source: prerequisites 12391490eea0 -->
## 事前準備

- 已依照[設定參考](../reference/configuration.md)的說明提供設定（`my config check`）。
- 結構描述已套用到資料庫（`my schema apply`；請參閱[資料庫](database.md)）。

<!-- source: keys ae25d36239c2 -->
## 鍵

<!-- source: settings-one-key-one-value 4f827bf97480 -->
### 設定（一個鍵，一個值）

| 鍵 | 型別 | 預設值 | 說明 |
| --- | --- | --- | --- |
| `registration_mode` | `open` / `approval` / `invite` / `closed` | `approval` | 註冊模式（[註冊](registration.md)）。在服務終止已公告期間、關閉之後，以及在[單人伺服器](#one-person-server)上，實際生效的值為 `closed`。儲存的值不會改變，撤回公告或關閉 `single_user` 就會恢復。`GET /instance` 和 NodeInfo 會顯示實際生效的值 |
| `max_status_length` | 整數 1-100000 | `500` | 貼文的最大長度（字元數） |
| `default_locale` | BCP 47 語言標籤 | `en` | 預設語系。它是新使用者的初始介面語系，也是沒有任何語言目錄符合使用者偏好時的備用語系 |
| `local_timeline_visibility` | `off` / `signed_in` / `everyone` | `everyone` | 誰能看到本站時間軸（`GET /api/v1/timelines/local`）：沒有人、已登入的使用者，或所有人。`off` 會回應 404，`signed_in` 則對未登入的訪客回應 401；兩種情況下導覽列都會隱藏它。除非值為 `everyone`，否則首頁不會顯示範例貼文。在[單人伺服器](#one-person-server)上，它會被視為 `off` |
| `federated_timeline_visibility` | `off` / `signed_in` / `everyone` | `signed_in` | 誰能看到聯邦時間軸（`GET /api/v1/timelines/federated`）。各值的意義與本站時間軸相同，在單人伺服器上也會被視為 `off` |
| `preview_image` | 媒體 ID（空白 = 未設定） | 空白 | 沒有圖片的貼文和個人檔案所使用的連結預覽圖片（`og:image`）。未設定時，會使用內建的預設圖片（[連結預覽](link-previews.md)） |
| `contact_account` | 本站帳號的使用者名稱（空白 = 未設定） | 空白 | 用來聯絡營運者的帳號。請參閱[聯絡方式與縮圖](#contact-and-thumbnail) |
| `contact_email` | 電子郵件地址（空白 = 未設定） | 空白 | 用來聯絡營運者的電子郵件地址。它也是伺服器寄出郵件的 `Reply-To` 和署名 |
| `thumbnail` | 媒體 ID（空白 = 未設定） | 空白 | 代表實例的圖片（縮圖） |
| `remote_media_policy` | `none` / `avatars` / `all` | `none` | 是否保留遠端媒體的副本。`none`：媒體代理會轉送每個請求。`avatars`：大頭貼、頁首圖片和自訂表情符號會在 blob 儲存區中快取 `remote_media_cache_ttl_seconds` 的時間。`all`：所有遠端媒體都會快取。快取的總大小受設定 `remote_media_cache_max_bytes` 限制 |
| `remote_media_cache_ttl_seconds` | 整數 60-31536000（秒） | `86400` | 擷取的遠端媒體可以保留多久。它也是媒體代理回應的 `Cache-Control: max-age`，所以 CDN 和瀏覽器會快取這麼久 |
| `remote_media_cache_max_bytes` | 1 以上的整數（位元組） | `1073741824` | 媒體代理保留的遠端媒體副本的大小上限。每小時的清理會刪除過期的副本，接著刪除超過此上限、最久未被存取的副本 |
| `authorized_fetch` | `true` / `false` | `false` | Authorized Fetch（安全模式）。為 `true` 時，沒有可驗證 HTTP 簽章的 ActivityPub GET，對物件和集合會回應 `401`，對使用者的 actor 則回應一份只帶有金鑰的最小文件。實例 actor 一律回傳完整文件，讓任何伺服器都能擷取金鑰來驗證簽章 |
| `federation_mode` | `off` / `inbound` / `outbound` / `both` | `both` | 聯邦的方向。`off`：不聯邦。`inbound`：你可以追蹤並閱讀遠端帳號，但本站的貼文、個人檔案和回應不會送出，來自外部的追蹤會被拒絕（`Reject`）。`outbound`：接受來自外部的追蹤並遞送本站貼文，但不接收遠端的貼文和回應。`both`：雙向收送。變更會立即生效，需要 `manage_settings` 權限 |
| `dynamic_client_registration` | `true` / `false` | `true` | 動態用戶端註冊（RFC 7591）。為 `true` 時，`POST /oauth/register` 不需驗證即可接受公開用戶端的註冊。為 `false` 時，該端點會以 `403`（`access_denied`）拒絕所有請求；已註冊的用戶端仍可繼續使用。定期任務會依照設定 `dynamic_client_unredeemed_retention` 和 `dynamic_client_idle_retention`，刪除未使用的動態註冊用戶端。變更會立即生效，需要 `manage_settings` |
| `domain_actions_visibility` | `off` / `signed_in` / `everyone` | `off` | 誰能看到已採取處置的網域列表（`GET /api/v1/instance/domain_actions`）：沒有人、已登入的使用者，或所有人（[審核](moderation.md#publishing-the-domains-you-act-on)） |
| `domain_actions_rationale_visibility` | `off` / `signed_in` / `everyone` | `off` | 誰能看到該列表的公開說明。看不到列表的人永遠看不到它們 |
| `appeal_window_days` | 整數 20-365（天） | `180` | 處置後幾天內，當事人可以提出申訴（[審核](moderation.md#appeals)）。變更它也會改變已採取處置的期限 |
| `session_origins_retention_days` | 整數 90-3650（天） | `180` | 登入後保留該次登入的 IP 位址和 User-Agent 的天數（[審核](moderation.md#record-retention-and-holds)） |
| `registration_origins_retention_days` | 整數 90-3650（天） | `180` | 註冊後保留註冊時 IP 位址的天數 |
| `reports_retention_days` | 整數 180-3650（天） | `365` | 最後一次處理後保留檢舉的天數。被處置引用的檢舉，會與該處置保留一樣久 |
| `moderation_actions_retention_days` | 整數 180-3650（天） | `365` | 處置結束（解除、到期或申訴審查，以較晚者為準）後保留帳號處置和申訴的天數 |
| `audit_log_retention_days` | 整數 365-3650（天） | `730` | 稽核記錄的資料列在記錄後保留的天數 |
| `remote_status_retention_days` | 整數 1-3650（天） | `90` | 當這個實例上沒有任何東西參照遠端貼文時（本站的回覆、轉發、喜歡、書籤、通知，或含有本站貼文的討論串），該貼文在發布後保留的天數 |
| `remote_status_max_retention_days` | 整數 0-3650（天） | `0` | 任何遠端貼文在發布後保留的最長天數，即使這個實例有參照它也一樣。本站的回覆、轉發、喜歡和書籤會留下，並將它顯示為已刪除。`0` 表示沒有上限 |
| `relay_status_retention_days` | 整數 1-3650（天） | `7` | 只由中繼站帶進來的遠端貼文，在這個實例上沒有任何東西參照它、也沒有本站帳號追蹤其作者時，保留的天數 |
| `oauth_client_rate_limits` | 以逗號分隔的 `client_id=requests` 配對 | 空白 | OAuth 用戶端的用戶端 API 請求上限，用來抑制讓伺服器過載的用戶端：其所有使用者合計在 5 分鐘內可發出的請求數。變更最多需要一分鐘才會套用 |
| `search_indexing` | `allow` / `deny` | `deny` | 搜尋引擎是否可以為實例建立索引。即使為 `allow`，也只有 `indexable` 帳號的個人檔案和 `public` 貼文會被索引（[搜尋引擎與 AI 爬蟲](crawlers.md)） |
| `ai_usage` | `allow` / `deny_training` / `deny` | `deny` | 對 AI 服務的聲明。`deny_training` 拒絕訓練；`deny` 拒絕訓練以及 AI 的使用（AI 搜尋、助理、代理程式） |
| `indexable_default` | `true` / `false` | `false` | 新帳號的 `indexable` 預設值。變更它不會改變現有帳號 |
| `auto_delete_default_enabled` | `true` / `false` | `false` | 新帳號是否一開始就開啟自動刪除舊貼文。它與其他 `auto_delete_default_*` 鍵一起構成新帳號所得到的規則（[自動刪除舊貼文](#automatic-deletion-of-old-posts)） |
| `auto_delete_default_min_age_days` | 整數 1-3650（天） | `30` | 新帳號的規則在貼文發布幾天後刪除它 |
| `auto_delete_default_keep_pinned`, `auto_delete_default_keep_bookmarked`, `auto_delete_default_keep_favourited` | `true` / `false` | `true` | 新帳號的規則是否保留作者釘選、加入書籤或按喜歡的貼文 |
| `auto_delete_default_keep_media`, `auto_delete_default_keep_polls` | `true` / `false` | `false` | 新帳號的規則是否保留含有媒體或投票的貼文 |
| `auto_delete_default_keep_public`, `auto_delete_default_keep_local`, `auto_delete_default_keep_unlisted`, `auto_delete_default_keep_private` | `true` / `false` | `false` | 新帳號的規則是否保留該可見範圍的貼文 |
| `auto_delete_default_keep_direct` | `true` / `false` | `true` | 新帳號的規則是否保留私訊貼文 |
| `auto_delete_default_keep_min_boosts`, `auto_delete_default_keep_min_favourites` | 整數 0-1000000 | `0` | 新帳號的規則會保留被轉發或按喜歡至少這麼多次的貼文。`0` 表示不設門檻 |
| `auto_delete_max_age_days` | 整數 0-3650（天） | `0` | 任何本站貼文在發布後最多保留的天數（[上限](#a-maximum-for-keeping-posts)）。`0` 表示沒有上限 |
| `auto_delete_max_allow_pinned` | `true` / `false` | `true` | 在上限之下，釘選的貼文是否仍可保留 |
| `auto_delete_max_kept_pins` | 整數 1-5 | `5` | 在上限之下，有幾個釘選可以保留貼文，從最近釘選的開始算。`5` 是帳號可釘選的最大數量，會保留所有釘選的貼文 |
| `auto_delete_max_allow_bookmarked`, `auto_delete_max_allow_favourited`, `auto_delete_max_allow_media`, `auto_delete_max_allow_polls` | `true` / `false` | `false` | 在上限之下，作者加入書籤或按喜歡的貼文，或含有媒體或投票的貼文，是否仍可保留 |
| `auto_delete_max_allow_visibilities` | `true` / `false` | `false` | 在上限之下，是否仍可依可見範圍保留貼文 |
| `auto_delete_max_allow_thresholds` | `true` / `false` | `false` | 在上限之下，是否仍可依轉發和喜歡的次數保留貼文 |
| `auto_delete_grace_days` | 整數 0-365（天） | `30` | 新適用上限的帳號，在收到通知後還能保留自己設定的天數。變更它不會移動已通知帳號的日期 |
| `search_visibilities` | `public` / `local` / `public,local` | `public` | 在有搜尋後端的伺服器上，全文搜尋涵蓋哪些貼文。`unlisted`、`private` 和 `direct` 貼文永遠不會被涵蓋，而且只涵蓋設定了 `indexable` 的帳號。移除某個可見範圍，會將其貼文從索引中移除。新增某個可見範圍，在你[重建索引](search.md#changing-search_visibilities)之前，只會找到該可見範圍的新貼文。在 `search = none` 時沒有作用 |
| `security_contact` | `https:` 或 `mailto:` URI（空白 = 未設定） | 空白 | 回報這台伺服器安全問題的聯絡方式：包括其設定和營運方式，以及其帳號和資料。設定後，伺服器會提供 `/.well-known/security.txt`（RFC 9116）。這是你的聯絡方式，而不是軟體的：軟體本身弱點的回報，請依照其[安全政策](../../SECURITY.md)的說明轉交給專案 |
| `require_media_description` | `true` / `false` | `false` | 附加在本站貼文上的每個媒體檔案（圖片、影片或音訊）是否都需要說明。為 `true` 時，含有說明缺漏或只有空白字元之檔案的貼文或編輯，會以 422 拒絕。它會以 `GET /api/v1/instance` 中的 `limits.require_media_description` 公開，網頁用戶端會將說明欄位設為必填。它與每位使用者的設定 `require_media_description` 是分開的，後者只在送出前檢查 |
| `link_cards` | `true` / `false` | `true` | 是否為貼文中的連結顯示卡片。為 `true` 時，伺服器會擷取被連結的頁面來建立卡片。為 `false` 時，它會停止擷取，並隱藏已建立的卡片。若只想在部分網域停止，請參閱[連結卡片網域](#link-card-domains) |
| `link_cards_ignore_robots_txt` | `true` / `false` | `false` | 連結卡片的擷取是否略過 `robots.txt`。為 `false` 時，伺服器會遵守被連結網站的 `robots.txt`，所以禁止 `my-linkcard` 或 `*` 的網站不會有卡片（[連結預覽](link-previews.md#cards-of-links-in-posts)）。`/about/crawler` 會告知被連結的網站目前是哪一種。為 `true` 時，伺服器既不擷取也不使用 `robots.txt`，被連結的網站也就無法再用它拒絕這些擷取；伺服器仍以 `my-linkcard` 自稱。只有在缺少的卡片太多時才開啟它。開啟期間建立的卡片，會在關閉後下一次重新擷取時接受檢查。頁面的指示和連結卡片網域在兩種情況下都適用 |
| `oauth_metadata_document_clients` | `true` / `false` | `true` | 是否接受 Client ID Metadata Document 用戶端。為 `true` 時，對於 `client_id` 為 `https` URL 的授權請求，伺服器會擷取該 URL 的中繼資料，並將其視為用戶端（不需事先註冊）。擷取不會跟隨重新導向，最多讀取 5 KB，並依照其快取指示最多重複使用回應 24 小時。為 `false` 時，它會停止擷取，並拒絕這些用戶端的授權請求、token 交換和更新 |
| `finance_visibility` | `private` / `totals` / `breakdown` | `private` | 要公開多少營運成本（在 `/admin/finance/costs` 記錄，在 `/admin/finance/publication` 設定）。`private`：不公開任何東西（`GET /api/v1/instance/cost_totals` 為 404）。`totals`：只公開每個期間和幣別的總額。`breakdown`：公開總額，以及依服務商和類別的明細，在資訊頁面（`/about` 和 `/about/costs`）和 API 中向所有人顯示。`breakdown` 會透露你使用哪個服務商（例如哪個雲端）。不論哪個層級，項目名稱、備註和個別付款都保持不公開。公開的期間到包含今天（UTC）的期間為止 |
| `finance_viewer_item_names` | `true` / `false` | `false` | 擁有 `view_finance` 權限的使用者，是否能在財務畫面（`/finance`、`GET /api/v1/finance/cost_totals`）上看到成本項目名稱和匯入成本的服務名稱。不論此設定為何，總額以及依服務商和類別的明細一律會顯示 |
| `finance_viewer_item_memos` | `true` / `false` | `false` | 擁有 `view_finance` 的使用者，是否能在財務畫面上看到成本項目的備註。只要名稱或備註的設定其中之一為 `true`，每個期間就會顯示項目列（服務商、類別和該期間的金額，以及已啟用的名稱和備註）。擁有 `manage_finance` 的人不論如何都能看到全部 |
| `finance_viewer_income` | `totals` / `plans` | `totals` | 擁有 `view_finance` 的使用者，只能看到收入總額（`totals`），還是也能看到每個方案的訂閱者人數（`plans`）。每個方案的金額以及誰付了款永遠不會顯示。如果 `income_visibility` 為 `plans`，不論此值為何都會顯示公開的人數。收入可在網頁用戶端的財務畫面（`/finance`）和 `GET /api/v1/finance/income_totals` 中查看 |
| `income_visibility` | `private` / `totals` / `plans` | `private` | 要公開多少支持者收入（扣除付款服務商手續費後的金額）。它與 `finance_visibility` 分開選擇。`private`：不公開任何東西（`GET /api/v1/instance/income_totals` 為 404）。`totals`：只公開每個期間和幣別的收入總額。`plans`：還會向所有人顯示目前每個方案的訂閱者人數。對於訂閱者很少的方案，人數可能讓人猜出是誰在訂閱。只有在此值和 `finance_visibility` 都不是 `private` 時才會顯示收支（收入減去成本）；否則收支會透露你隱藏的那一方。不論哪個層級，個別付款、訂閱者、每個方案的金額、總金額和手續費都保持不公開。公開的期間到包含今天（UTC）的期間為止 |
| `supporter_list_visibility` | `off` / `signed_in` / `everyone` | `off` | 誰能看到支持者列表（`GET /api/v1/instance/supporters` 和 `/about/supporters`）。`off`：沒有人（API 回應 404，`/about` 上沒有連結）。`signed_in`：只有登入這台伺服器的人（否則 API 回應 401）。`everyone`：所有人。列表只會顯示選擇公開的有效訂閱者的帳號和方案；金額、按月或按年計費、日期和人數永遠不會顯示。不論 `search_indexing` 為何，列表頁面和 API 回應一律為 `noindex, noarchive` |
| `user_app_registration` | `true` / `false` | `true` | 已登入的使用者是否能透過用戶端 API（`POST /api/v1/oauth_apps`）註冊自己的 OAuth 應用程式（公開或機密）。為 `false` 時，註冊會回應 403。列出、變更和刪除已註冊的應用程式、重新發行密鑰、為自己發行 token，以及其他使用者的授權仍可繼續使用 |
| `voice_room_starters` | `everyone` / `permitted` | `permitted` | 在設定 `voice` 不是 `off` 時，誰能開設語音聊天室（[設定](../reference/configuration.md#voice)）。`everyone`：所有已登入、未被停權或限制的本站使用者。`permitted`：只有 `start_voice_rooms` 權限的持有者；預設只有內建的 `admin` 角色擁有此權限，支持者方案的角色也可以擁有 |
| `voice_public_summary` | `true` / `false` | `false` | 可見範圍為 `local` 的語音聊天室頁面，是否向未登入這台伺服器的訪客和遠端使用者顯示其名稱、說明、主持人以及是否仍開放中，並顯示在其連結預覽中。參與者永遠不會顯示。為 `false` 時，這些訪客只會看到這是給這台伺服器本站使用者的聊天室 |
| `public_counts` | `true` / `false` | `true` | 首頁（`/`，未登入時）和 `GET /api/v1/instance/overview` 是否顯示伺服器有多少成員、使用者在過去 7 天中每天發了多少公開貼文，以及伺服器何時開始運作。只計算這台伺服器使用者的公開貼文；數字每幾分鐘更新一次。為 `false` 時，API 會省略它們 |
| `featured_accounts` | 最多 20 個本站帳號的使用者名稱，以空白或逗號分隔（空白 = 無） | 空白 | 追蹤對象推薦中優先顯示的帳號，依此順序排在這台伺服器最近發文的人之前。擁有者關閉了「推薦我的帳號，並允許用部分名稱找到它」的帳號不會被推薦。請參閱[推薦](#suggestions-of-accounts) |
| `login_providers_only` | `true` / `false` | `false` | 在至少啟用一個登入提供者時，註冊和登入是否限定使用登入提供者：密碼和通行金鑰不再能登入，沒有連結提供者的使用者在其工作階段結束後就無法再登入。沒有啟用任何提供者時不適用。`my instance set` 會印出有多少活躍使用者沒有連結到已啟用的提供者。請參閱[登入限定使用登入提供者](login-providers.md#limit-sign-in-to-login-providers)。 |
| `invite_creators` | `everyone` / `permitted` | `permitted` | 除了擁有 `manage_invites` 的工作人員之外，誰能建立自己的邀請（[成員邀請](registration.md#member-invites)）。`everyone`：所有已登入、未被停權或限制的本站使用者。`permitted`：只有 `create_invites` 權限的持有者；預設只有內建的 `admin` 角色擁有此權限，支持者方案的角色也可以擁有。在 `approval` 模式下，可用的邀請會立即核准新使用者，所以 `everyone` 會讓每位使用者都能略過審查 |
| `member_invite_max_active` | 整數 0-100 | `5` | 一位成員同時可以持有多少個可用的邀請（未撤銷、未過期、未用完）；`0` 表示成員無法建立任何邀請。工作人員的邀請不計入 |
| `member_invite_max_uses` | 整數 1-1000 | `10` | 成員建立的一個邀請可以讓多少人註冊 |
| `member_invite_max_lifetime_seconds` | 整數 3600-31536000（秒） | `2592000`（30 天） | 成員建立的一個邀請最長可以有效多久。調低它不會縮短現有的邀請 |
| `translation_providers` | 以逗號分隔的提供者 ID | 空白 | 已啟用的伺服器端翻譯提供者，排在前面的先嘗試。空白時不會將任何東西送給任何提供者。這台伺服器沒有對應提供者的 ID 不會有任何作用。內建的[翻譯提供者](translation.md)的 ID 為 `libretranslate`、`deepl`、`google` 和 `aws`，[翻譯外掛](plugins.md#translating-posts)的 ID 為 `plugin` |
| `translation_provider_visibilities` | 以逗號分隔的 `provider=visibility\|visibility` 配對 | 空白 | 每個提供者可以收到什麼。未指定的提供者會收到 `public`、`local` 和 `unlisted` 貼文；`provider=` 後面沒有內容則什麼都不會收到。`private` 和 `direct` 只會送給有明確指定它們的提供者，所以允許一個提供者並不代表允許下一個 |
| `translation_browser` | 布林值 | `false` | 讀者是否可以使用瀏覽器內建的翻譯功能，這不會將任何東西送到這台伺服器或提供者。同時啟用伺服器端提供者時，提供者優先，而瀏覽器會翻譯伺服器拒絕的內容：沒有提供者接受的貼文或語言、達到每月上限，或提供者失敗時 |
| `translation_user_monthly_characters` | 整數 0-1000000000000 | `50000` | 一位使用者的請求在一個 UTC 月內可以送給翻譯提供者的字元數。為 `0` 時不會送出任何東西。沒有無上限的值：請設定一個很大的數字 |
| `translation_server_monthly_characters` | 整數 0-1000000000000 | `500000` | 整台伺服器在一個 UTC 月內可以送給翻譯提供者的字元數。為 `0` 時不會送出任何東西。沒有無上限的值：請設定一個很大的數字 |
| `translation_retention_days` | 整數 1-3650（天） | `30` | 翻譯在產生後保留的天數。閱讀它不會延長期限，而且它會隨貼文一起刪除 |
| `translation_reports_to_server` | 布林值 | `false` | 版主是否可以將檢舉的文字（被檢舉的貼文和檢舉者的留言）送給伺服器端提供者。不論是誰提出請求，每個提供者仍只會收到 `translation_provider_visibilities` 允許它的可見範圍：被檢舉的私訊貼文只會送給允許 `direct` 的提供者，而留言視為 `private`。留言的翻譯永遠不會保留，已刪除貼文的翻譯也不會，而且字元數只計入 `translation_server_monthly_characters`。為 `false` 時，檢舉的任何內容都不會送到提供者 |
| `voice_participants_retention_days` | 整數 30-3650（天） | `90` | 語音聊天室結束後，保留參與者紀錄的天數：參與、發言者和聽眾、舉手、邀請和移除。聊天室本身（名稱、說明、主持人和結束方式）會保留。被檢舉聊天室的紀錄會與檢舉保留一樣久，保留措施也如同其他紀錄一樣適用 |
| `hook_inbound_status_mode`, `hook_inbound_follow_mode`, `hook_signup_check_mode`, `hook_media_check_mode`, `hook_local_status_mode`, `hook_inbound_actor_mode`, `hook_local_profile_mode` | `off`, `observe`, `enforce` | `observe` | 每個[決策掛鉤](decision-hooks.md#modes)在有外掛時作用到什麼程度。`off` 只能經由 `observe` 才能變成 `enforce`。 |
| `hook_inbound_status_on_failure`, `hook_inbound_follow_on_failure`, `hook_media_check_on_failure`, `hook_inbound_actor_on_failure` | `pass`, `retry` | `pass` | 掛鉤在強制執行時，其外掛失敗時要怎麼做（[詳細說明](decision-hooks.md#when-the-plugin-fails)）。 |
| `hook_inbound_status_unfollowed_direct` | 布林值 | `false` | `inbound.status` 是否也送出沒有任何本站帳號追蹤之作者的私訊貼文（[詳細說明](decision-hooks.md#what-inboundstatus-reviews)）。使用此掛鉤時，資訊頁面（`/about`）會載明這一點。 |
| `hook_signup_check_on_failure` | `pass`, `require_approval` | `pass` | `signup.check` 在強制執行時，其外掛失敗時要怎麼做。 |
| `hook_signup_check_reject` | 布林值 | `false` | `signup.check` 在強制執行時，其 `reject` 是否會[拒絕註冊](decision-hooks.md#sign-ups)。關閉時，回應只會被記錄，就像 `observe` 一樣。 |
| `hook_local_status_account_age_days` | 整數 0-3650（天） | `7` | `local.status` 會[審查貼文及其編輯](decision-hooks.md#local-posts)的帳號，其帳號年齡須小於此天數；`0` 表示審查所有帳號。使用此掛鉤時，資訊頁面（`/about`）會載明這一點。 |
| `hook_local_profile_account_age_days` | 整數 0-3650（天） | `7` | `local.profile` 會[審查個人檔案變更](decision-hooks.md#local-profiles)的帳號，其帳號年齡須小於此天數；`0` 表示審查所有帳號。使用此掛鉤時，資訊頁面（`/about`）會載明這一點。 |
| `hook_decisions_retention_days` | 整數 30-3650（天） | `90` | [掛鉤的決策](decision-hooks.md#the-decisions)保留的天數。 |
| `single_user` | 布林值 | `false` | 伺服器是否為[單人伺服器](#one-person-server)。只有在伺服器恰好只有一個本站帳號時才能開啟，隨時都可以關閉 |

- 值在儲存前會先正規化（`TRUE` 變成 `true`，`0750` 變成 `750`，`ja_jp` 變成 `ja-JP`，媒體 ID 轉為小寫）。無效的值會被拒絕，不會寫入任何東西。
- `preview_image` 和 `thumbnail` 只接受存在的圖片媒體（包括仍在處理中的）。在處理完成之前，以及取代處理失敗的圖片時，`preview_image` 會改用預設圖片。媒體一旦在任何修訂版本中被設定過，未附加上傳檔案的清理就不會刪除它。
- `contact_account` 只接受未刪除的本站使用者的使用者名稱（以小寫儲存）。`contact_email` 只接受 `alice@example.com` 形式的地址。
- `featured_accounts` 只接受未刪除的本站使用者的使用者名稱；開頭的 `@` 會被去掉，名稱會轉為小寫，重複的名稱只保留一個，並以單一空白分隔儲存。

<!-- source: suggestions-of-accounts b7d362466e33 -->
### 帳號推薦

已登入的使用者會看到追蹤對象的推薦（註冊後空白的首頁時間軸上，以及透過 `GET /api/v1/accounts/suggestions`）。推薦依序為 `featured_accounts` 中的帳號，接著是過去 30 天內發布過公開或本站貼文的這台伺服器帳號，最後發文的排在最前面。使用者自己的帳號、使用者追蹤、封鎖、靜音或被其封鎖的帳號，以及已停權或停用的帳號，都會被排除。

新帳號會被推薦，也能在帳號搜尋中用名稱的一部分找到。在個人檔案設定中關閉「推薦我的帳號，並允許用部分名稱找到它」的使用者，兩者都會退出：此時搜尋只能用完整的使用者名稱找到該帳號，或者只有追蹤它的人才找得到。

<!-- source: automatic-deletion-of-old-posts f0cf962c9de0 -->
### 自動刪除舊貼文

使用者可以讓自己的貼文在發布幾天後自動刪除（「設定」→「刪除舊貼文」）。每位使用者的規則都是自己的：是否開啟、幾天後刪除，以及要保留哪些貼文（釘選、加入書籤和按喜歡的貼文、含有媒體或投票的貼文、所選可見範圍的貼文，以及轉發或喜歡達到一定次數的貼文）。以這種方式刪除貼文，與作者自己刪除相同：貼文曾送達的伺服器會收到 `Delete`。只有本站帳號的貼文會以這種方式刪除，轉發永遠不會。

`auto_delete_default_*` 鍵是每個新的本站帳號一開始的規則，不論帳號以何種方式建立（註冊表單、外部登入提供者、邀請或 `my admin`）。帳號在建立時取得這份規則，之後就歸帳號所有：之後變更這些鍵，既不會改變現有的帳號，也不會改變取得較早值的帳號。設定畫面會將這些值顯示為使用者自己的設定。未設定這些鍵時，新帳號一開始會關閉自動刪除、設為 30 天後，並保留釘選、加入書籤和按喜歡的貼文以及私訊貼文。預設值不會向實例外部的人顯示。設定了上限時，新帳號會取得依下述方式裁切到上限的預設值，讓它一開始就在上限之內。

<!-- source: a-maximum-for-keeping-posts b1d83e01feda -->
#### 貼文保留的上限

`auto_delete_max_age_days` 大於 `0` 時，任何本站貼文在發布後都不會保留超過該天數。對於每個沒有 `keep_posts_longer` 權限的本站帳號：

- 自動刪除一律開啟：使用者無法關閉它，也無法選擇比上限更多的天數（較少則沒問題）。設定畫面和 `PUT /api/v1/preferences/auto_delete` 都會拒絕這兩者。
- 只有 `auto_delete_max_allow_*` 鍵允許的例外能保留貼文。預設只有釘選的貼文可以，而且是全部；`auto_delete_max_kept_pins` 小於 5 時，只有最近的那幾個釘選能保留其貼文。
- 關閉了自動刪除或從未設定的使用者，也同樣受上限約束。

開啟上限、調低上限或變更例外，都不會改寫任何人的設定。取而代之的是，每小時任務 `content.auto_delete_statuses` 會找出那些自己的設定所保留的貼文超出上限允許範圍的帳號（因為上限被開啟或調低，或帳號失去了 `keep_posts_longer`）。它會透過預設開啟的通知和電子郵件，通知每個帳號一次，並告知上限從何時開始適用：通知後的 `auto_delete_grace_days`。在那之前，帳號自己的設定仍然有效；從那時起，其貼文就依照上限處理。再次回到上限之內的帳號（其使用者變更了設定、取得了 `keep_posts_longer`，或上限被移除或放寬）就不再受約束，而之後的變更會再次通知它，並給予新的寬限期。

開啟上限後，一小時內就會通知所有受其約束的帳號。刪除會維持一定的速度：每個帳號每小時最多 100 則貼文，每則都會以 `Delete` 送到該貼文曾送達的伺服器，所以有很多舊貼文的帳號需要數小時或數天才會清空。在這段期間變更設定的使用者，會停止剩下的刪除。

`keep_posts_longer` 是一項會員權限：將它賦予某個角色，就能讓其持有者自行決定貼文保留多久，並關閉自動刪除。內建的 `admin` 角色擁有此權限。[支持者方案](supporters.md)可以帶有它，所以保留貼文更久可以成為方案的福利；訂閱結束時，上限會在寬限期後再次適用。

上限是公開的：`GET /api/v1/instance` 會在 `auto_delete_limit` 中回傳它，關於頁面也會顯示它，讓人們在註冊前就知道。變更這些鍵需要 `manage_settings`，而且和其他設定一樣會記錄在稽核記錄中。在網頁用戶端中，它們位於「伺服器設定」的「貼文刪除」頁面，在儲存開啟或更嚴格的上限之前會先詢問。

<!-- source: texts-one-key-one-value c639fc7dfe23 -->
### 文字（一個鍵，一個值）

| 鍵 | 限制 | 說明 |
| --- | --- | --- |
| `name` | 100 個字元，單行（不允許只有空白字元） | 實例名稱（顯示名稱）。請參閱[名稱與網域](#name-and-domain) |
| `short_description` | 500 個字元 | 簡短說明 |
| `description` | 20000 個字元 | 說明 |
| `terms` | 100000 個字元 | 服務條款（Markdown） |
| `privacy_policy` | 100000 個字元 | 隱私權政策（Markdown） |
| `commerce_disclosure` | 100000 個字元 | 付費方案的銷售者揭露事項，例如日本法律要求的標示（Markdown；[支持者](supporters.md)） |
| `preview_image_alt` | 1500 個字元 | `preview_image` 圖片的替代文字（`og:image:alt`）。未設定時，會使用「{instance name} 的圖片」 |
| `thumbnail_alt` | 1500 個字元 | `thumbnail` 圖片的替代文字 |

- 每段文字只有一個值。管理員以自己喜歡的任何語言寫一次，所有語系的讀者都會看到相同的值；其他語言的讀者則依靠機器翻譯之類的方式。管理欄位的標籤和說明會像其他畫面文字一樣翻譯成檢視者的語言；只有管理員寫的值不會翻譯。
- 伺服器不知道文字的語言，所以網頁用戶端和伺服器產生的畫面在輸出文字時，會使用 `dir="auto"` 和 `lang=""`（HTML 中表示未知語言的值），而不是繼承畫面的語言。
- 將文字設為空白會刪除它。
- 除了 CLI 之外，文字也可以在網頁用戶端的管理畫面中變更（名稱、簡短說明和說明在「伺服器設定」的「一般」頁面 `/admin/settings`；服務條款在「條款與政策」的第一頁 `/admin/documents`，隱私權政策在 `/admin/documents/privacy-policy`，商業揭露事項等在 `/admin/documents/commerce-disclosure`），或透過用戶端 API（`GET /api/v1/admin/instance_texts`、`PUT /api/v1/admin/instance_texts/{key}`）變更。兩者都需要角色權限 `manage_settings`，變更會記錄在稽核記錄中（`text_changed`）。

<!-- source: terms-and-privacy-policy 82e7bc325cf1 -->
### 服務條款與隱私權政策

- 請以 Markdown（CommonMark，加上 GitHub Flavored Markdown 的表格、刪除線和自動連結）撰寫 `terms`、`privacy_policy` 和 `commerce_disclosure`。段落中的換行會保留為換行，`#` 標題則從頁面標題的下一層（`h2`）開始。
- 伺服器會將它們轉換為 HTML 並加以清理（匿名的 `GET /api/v1/instance/documents/{kind}`），由網頁用戶端的頁面 `/about/terms`、`/about/privacy` 和 `/about/commerce` 顯示。原始 HTML、圖片，以及 `http`、`https` 和 `mailto` 以外的連結都會被移除。這些頁面不需要登入（它們是網頁用戶端的頁面，所以需要 JavaScript）。`/about/commerce` 是要提供給付款服務商的 URL。
- 你撰寫的文件會從註冊畫面、伺服器產生的每個畫面的頁尾，以及網頁用戶端（第三欄、選單、設定和登入畫面）連結過去。未撰寫的文件不會被連結，其 API 會回應 404。
- 撰寫了服務條款或隱私權政策時，註冊畫面會在建立按鈕之前列出它們，並附上必填的核取方塊「我已閱讀並同意上述文件」。沒有勾選的註冊會被伺服器拒絕，透過登入提供者的註冊也一樣。兩份文件都沒有時，畫面不會要求同意。

<!-- source: name-and-domain 1e14aa3e6c58 -->
### 名稱與網域

實例分別擁有網域和名稱。

- **網域**是設定 `host`（以及帳號名稱的網域 `account_domain`）。實例執行期間無法變更。即使設定了名稱，帳號名稱（`@alice@example.com`）、URL、ActivityPub ID 以及 `GET /api/v1/instance` 中的 `domain` 仍然是網域。
- **名稱**是文字 `name`。凡是向人們稱呼實例的地方都會出現它：網頁用戶端的側邊欄和「關於這個伺服器」、瀏覽器分頁標題、伺服器產生的登入、註冊、電子郵件確認、帳號狀態和授權同意畫面、電子郵件（電子郵件地址確認和給版主的通知）、連結預覽的 `og:site_name`、Web App Manifest 的 `name` / `short_name`、NodeInfo 的 `metadata.nodeName`，以及 `GET /api/v1/instance` 中的 `name`。
- 名稱在所有語系中都相同。沒有名稱時，會改為顯示網域（`host`）。
- 變更會從下一個請求開始生效，不需要重新啟動。在管理畫面儲存後，已開啟的網頁用戶端的分頁標題和側邊欄也會更新。

<!-- source: contact-and-thumbnail 71513ab82d51 -->
### 聯絡方式與縮圖

營運者的聯絡方式（`contact_account`、`contact_email`）和代表實例的圖片（`thumbnail` 和文字 `thumbnail_alt`）會出現在以下地方。未設定的項目不會出現。

- 網頁用戶端的「關於這個伺服器」（`/about`）。縮圖和聯絡帳號也會在未登入者看到的第一個畫面（`/`）上出現。
- `GET /api/v1/instance` 中的 `contact`（`account` 和 `email`）和 `thumbnail`（`url`、`width`、`height`、`alt`）。
- NodeInfo 中的 `metadata`：依照 Misskey 慣例的 `maintainer`（`name` 為聯絡帳號的顯示名稱，沒有時則為其使用者名稱；`email` 為 `contact_email`）、依照 Pleroma 和 Akkoma 慣例的 `staffAccounts`（聯絡帳號的 actor ID）、`thumbnail`（`url` 和 `alt`），以及取自簡短說明 `short_description` 的 `nodeDescription`。

聯絡帳號被停權或刪除時，不會顯示該帳號，只會顯示電子郵件地址。請在網頁用戶端管理畫面的「一般」頁面（「伺服器設定」，`/admin/settings`）中，或透過 CLI 或用戶端 API 變更。

<!-- source: one-person-server 6ba524c58a4f -->
### 單人伺服器

只屬於營運者一人的伺服器，也就是營運者自己在聯邦宇宙中的家，就是單人伺服器：請開啟 `single_user`。在網頁用戶端中，管理員可以在「伺服器設定」的「註冊」頁面，用「將這個伺服器設為單人伺服器」來開啟或關閉它；伺服器有一個以上的本站帳號而拒絕時，該頁面會說明原因。

```sh
my instance set single_user true
my instance set single_user false   # back to a community server
```

- 只有在恰好只有一個本站帳號被計算時才能開啟。每個未刪除的本站帳號都會被計算，包括待核准、已停用、預定刪除、已停權或已限制的帳號，因為每一個都可能以第二個人的身分回來。註冊被拒絕的帳號不計算。在其他任何數量下，`my instance set` 會附上數量而失敗，管理 API 則會以類型 `/api/v1/problems/single-user-requires-one-account` 回應 `422`。伺服器的實例 actor 不是帳號，永遠不計算。
- 隨時都可以關閉。開啟或關閉都不會刪除任何東西，也不會改變任何其他設定的儲存值：它所限制的事項在關閉後會恢復原狀。和所有設定一樣，變更會寫入稽核記錄。

開啟期間：

- **註冊關閉**。 不論 `registration_mode` 儲存的值為何，都會被視為 `closed`，所以註冊頁面、透過登入提供者的註冊以及 `my admin create` 都不會建立帳號。使用登入提供者登入仍可運作。
- **只有首頁時間軸**。 `local_timeline_visibility` 和 `federated_timeline_visibility` 會被視為 `off`。主題標籤和列表時間軸則保留。
- **社群才需要的功能會被隱藏**。 本站和聯邦時間軸及其串流、帳號推薦、使用者和管理員的邀請、註冊審查，以及註冊封鎖，都會以類型 `/api/v1/problems/single-user-mode` 回應 `404`。
- **不能發新的本站貼文**。 在只有一個人的伺服器上，「本站」的貼文除了追蹤者之外送不到任何人，所以新貼文不能設為本站：用戶端 API 會回應 `422`，並將它從帳號可用的發文選項中排除。預設可見範圍為「本站」時會被視為「僅追蹤者」且無法儲存，而給這台伺服器使用者的語音聊天室，會以給追蹤者的貼文來宣告。已經是本站的貼文仍保持本站。
- **伺服器會表明自己的性質**。 `GET /api/v1/instance` 中的 `single_user` 為 `true`，並附上實際生效的註冊模式和時間軸，以及作為 `single_user_account` 的營運者帳號；NodeInfo 則回應 `openRegistrations: false` 和 `metadata.singleUser: true`。
- **首頁就是營運者的個人檔案**。 未登入的人開啟伺服器的網址時，會看到其唯一帳號的個人檔案，以及登入方式，和伺服器公開的關於頁面、服務條款和隱私權政策的連結。網頁用戶端會從導覽列、選單和管理畫面中隱藏伺服器不提供的功能：本站和聯邦時間軸、註冊、推薦帳號、邀請、註冊審查、註冊封鎖、可見範圍「本站」，以及註冊、邀請、公開時間軸、首頁和推薦帳號的設定。

聯邦、審核、全文搜尋、支持者計畫、文件和管理畫面，都和其他伺服器一樣運作。

<!-- source: usage 68ad8384438f -->
## 使用方式

```sh
my instance list                                      # all settings (and whether each is the default) and texts
my instance get registration_mode
my instance set registration_mode open
my instance set name "Example Social"                 # the same name in every locale
my instance set description - < description.txt       # the value "-" reads standard input
my instance get name
my instance set name ""                               # delete the name
my instance history registration_mode                 # revision history (newest first, -limit sets the count)
my instance history description
my instance set -owner admin preview_image ./preview.png   # upload the image as media of admin and use it as the preview image
my instance set preview_image 0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f   # give the ID of media that is already uploaded
my instance set preview_image_alt "A blue bird mascot"
my instance set preview_image ""                      # back to the bundled default image
my instance set contact_account admin                 # the contact account
my instance set contact_email admin@example.com
my instance set -owner admin thumbnail ./thumbnail.png  # the thumbnail
my instance set thumbnail_alt "A town by the sea"
```

- 旗標請寫在鍵和值之前。
- 使用 `-owner <username>` 時，`preview_image` 或 `thumbnail` 的值會被當作圖片檔案的路徑讀取，並作為該本站帳號的媒體上傳。圖片在使用前會由 worker 處理。建議使用 1200x630 的橫向圖片。
- 變更會在一個交易中記錄修訂版本（`instance_*_revisions`）並更新目前的值（`instance_settings` / `instance_texts`）。從 CLI 進行的變更不記錄執行者（歷史記錄中的 `BY` 為 `cli`）。
- 再次設定相同的值，仍會新增一個修訂版本。
- 結束代碼：0 表示成功，1 表示值被拒絕或資料庫錯誤（不會有任何變更），2 表示引數錯誤。

<!-- source: changing-the-direction-of-federation 9b9b63689c84 -->
### 變更聯邦的方向

- 縮小方向時，會保留追蹤關係、匯入的貼文和遞送紀錄，也不會送出 `Undo` 或 `Reject`。流動只是停止，回到 `both`（或其他模式）就會恢復原狀。
- 縮小期間發布的貼文和回應，在你再次擴大後不會追溯遞送。
- 在遠端那一側，模式為 `inbound` 時本站使用者的追蹤者什麼都收不到；模式為 `outbound` 或 `off` 時，本站使用者的追蹤不會更新。
- 模式為 `off` 時進行的刪除（貼文或帳號）不會送出。在 `inbound` 和 `outbound` 下，撤回已經送出的內容（`Delete` 等）會送給原本的收件者和當時的追蹤者。
- 變更會記錄在 `instance_setting_revisions` 和稽核記錄（`target_kind = setting`）中。

若要連關係也一併切斷，請使用 CLI `my admin federation sever`。

```sh
my admin federation sever -direction in     # end follows of local users by remote accounts
my admin federation sever -direction out    # end follows of remote accounts by local users
my admin federation sever -direction both
```

- 它會將本站與遠端帳號之間待處理和有效的追蹤，以審核所做的終止來結束（`follow_terminations`，`reason = moderation`）。變更模式永遠不會自動呼叫它。
- 只有在目前的模式會送出該方向時，才會將 `Reject`（`in`）或 `Undo`（`out`）送到遠端那一側。在不送出的模式（例如 `off`）下，追蹤只會在本站這一側結束，遠端那一側不會被告知。
- 重複執行是安全的。它會將結束的追蹤數印到標準輸出。

<!-- source: admin-screens-and-the-client-api 6a747f50af97 -->
## 管理畫面與用戶端 API

CLI 能變更的每項設定，也都能在網頁用戶端的管理畫面和透過用戶端 API 變更。兩者都需要角色權限 `manage_settings`（只有內建的 `admin` 角色擁有）。和 CLI 一樣，變更會保留為修訂版本，並按鍵記錄在稽核記錄中（`setting_changed`，目標為該設定，執行者為該使用者）。請在審核畫面（「稽核日誌」，`/moderation/audit-log`）查看稽核記錄。

- **網頁用戶端**（「伺服器設定」，`/admin/settings`）：畫面依主題分為多個頁面，各自連結在其標題之下。每個頁面有一個表單和一個儲存按鈕，儲存時只會送出你在該頁面上變更的內容；其他頁面上的設定維持原狀。

  | 頁面 | 區塊與設定 |
  | --- | --- |
  | 一般（`/admin/settings`） | 名稱、說明（`name`、`short_description`、`description`）；聯絡資訊與縮圖（`contact_account`、`contact_email`、`thumbnail`，以及文字 `thumbnail_alt`） |
  | 圖示與標誌（`/admin/settings/branding`） | 圖示、標誌和深色背景用的標誌，各自以自己的按鈕儲存（[標誌與圖示](#logo-and-icon)） |
  | 註冊（`/admin/settings/registration`） | 註冊（`registration_mode`、`login_providers_only`）；成員邀請（`invite_creators`、`member_invite_max_active`、`member_invite_max_uses`、`member_invite_max_lifetime_seconds`）；應用程式（`user_app_registration`、`oauth_metadata_document_clients`、`dynamic_client_registration`） |
  | 內容（`/admin/settings/content`） | 語言（`default_locale`，從應用程式支援的語言中選擇）；時間軸（`local_timeline_visibility`、`federated_timeline_visibility`）；貼文（`require_media_description`）；首頁（`public_counts`）；推薦帳號（`featured_accounts`） |
  | 貼文刪除（`/admin/settings/auto-delete`） | 新帳號的預設值（`auto_delete_default_*` 設定）；上限（`auto_delete_max_age_days`、`auto_delete_max_allow_*` 設定、`auto_delete_max_kept_pins`、`auto_delete_grace_days`）。儲存開啟或更嚴格（更少天數、更少例外或釘選）的上限時會先詢問，因為它會通知新受其約束的帳號（[貼文保留的上限](#a-maximum-for-keeping-posts)） |
  | 聯邦（`/admin/settings/federation`） | 聯邦（`federation_mode`、`authorized_fetch`、`remote_media_policy`、`remote_media_cache_ttl_seconds`（從 1 小時、1 天、7 天、30 天或 365 天中選擇）、`link_cards`、`link_cards_ignore_robots_txt`）；受限制與停權的伺服器（`domain_actions_visibility`、`domain_actions_rationale_visibility`）；外部審核（`hook_*` 設定，請參閱[決策掛鉤](decision-hooks.md)） |
  | 搜尋與爬蟲（`/admin/settings/discovery`） | 搜尋引擎與 AI（`search_indexing`、`indexable_default`、`ai_usage`、`security_contact`）；貼文搜尋（`search_visibilities`，僅限伺服器有全文搜尋時） |
  | 語音聊天（`/admin/settings/voice`） | `voice_room_starters`、`voice_public_summary`、`voice_participants_retention_days`；只有在伺服器提供語音聊天時才會列出這個頁面 |

  在「一般」頁面上，圖片會在你選擇時上傳，並在儲存時成為縮圖。誰能看到營運成本、收入和支持者列表，是在「費用」和「支持者」畫面上設定的。

  `max_status_length`、`preview_image`、`appeal_window_days` 和保留期間（`*_retention_days`）、`remote_media_cache_max_bytes`，以及 `oauth_client_rate_limits` 不會出現在畫面上；請透過 CLI 或用戶端 API 變更。翻譯的設定有自己的畫面，見下文。
- **網頁用戶端**（「翻譯」，`/admin/translation`）：這台伺服器擁有的提供者、它們的順序以及各自可以收到什麼、瀏覽器的翻譯、每月上限，以及翻譯保留多久，並顯示本月送出的字元數和快取命中率。每個伺服器行程最多每分鐘將其快取命中次數加到命中率一次，所以最新的請求可能需要一分鐘才會顯示，而停止的行程可能會留下最後一分鐘未計入。在啟用伺服器端提供者之前，請先更新你的隱私權政策（[範本](privacy-policy-template.md#the-services-you-choose)）：送給提供者的文字無法收回。提供者的 API 金鑰和[翻譯外掛](plugins.md#translating-posts)的位址存放在[部署設定](../reference/configuration.md)中；任何設定和用戶端 API 的回應都不會包含它們。
- **翻譯上限**。 計算的是送出的字元數（以碼位計）：代替提及、連結等的標記不計入，伺服器已保留的翻譯、已經是讀者語言的貼文，以及瀏覽器的翻譯也都不計入。每次呼叫提供者都會計入，失敗的呼叫也一樣，因為提供者可能已經收到文字，所以這個數字可能與提供者的計費不同。超過上限時，請求會以 `429` 拒絕，且不會呼叫任何提供者；此時若你允許且瀏覽器能翻譯該貼文，網頁用戶端會改用讀者瀏覽器的翻譯。月份以 UTC 計算。
- **用戶端 API**：`GET /api/v1/admin/settings` 會回傳每項設定的目前值、預設值、是否已設定，以及其型別和可接受的值（選項或範圍）。`PATCH /api/v1/admin/settings` 會以 `{"values": {"<key>": "<value>", ...}}` 一次變更多項設定。所有值都是字串，會和 CLI 一樣正規化後儲存。只要有一個鍵未知或一個值無效，就會回應指出該鍵的 422，而且不會有任何變更。變更的鍵會在同一個交易中記錄為修訂版本並寫入稽核記錄。
- 在 `open`、`approval` 和 `invite` 模式下可用的邀請，是在審核畫面（「邀請」，`/moderation/invites`，權限 `manage_invites`；請參閱[邀請碼](registration.md#invite-codes)）中建立，成員也可以在 `invite_creators` 允許的範圍內建立（[成員邀請](registration.md#member-invites)）。

<!-- source: rules b1076bb81e0c -->
## 規則

實例的規則會出現在 `GET /api/v1/instance` 的 `rules` 中，檢舉會以 ID 引用它們。

- 一條規則有一句單行的文字（最多 300 個字元）和選填的補充說明（最多 1,000 個字元）。和文字一樣，管理員以自己喜歡的任何語言撰寫，所有語系都會看到相同的值（`dir="auto"` 和 `lang=""`）。有效的規則最多 100 條。
- 每次變更（修改文字、重新排序）都會保留為修訂版本（`instance_rule_revisions`），規則的 ID 不會改變。
- 廢止的規則會從有效規則列表中消失。它無法恢復，但檢舉仍可以引用它。

```sh
my instance rule create -hint "No harassment" "Be kind"   # add at the end of the active rules
my instance rule edit <rule> "Always be kind"          # change the sentence; the supplement changes only with -hint
my instance rule move <rule> 1                         # move to position 1 and renumber the active rules 1, 2, ...
my instance rule retire <rule>                         # retire it
my instance rule list                                  # active rules (-all includes retired ones)
```

- 管理 API 為 `/api/v1/admin/rules`（列出、新增、取得、修改、移動、廢止），需要角色權限 `manage_rules`。
- 在網頁用戶端的管理畫面（「規則」，`/admin/rules`，`manage_rules`）中，列表會依序顯示有效的規則，可以用「上移」和「下移」重新排序；列表上方的篩選器可以改為顯示已廢止的規則。「新增規則」會開啟一個填寫文字和補充說明的頁面。選擇規則的文字可以開啟其頁面，在那裡編輯它，或（經過確認對話框後）廢止它。每次變更都會記錄在稽核記錄中（`rule_added`、`rule_revised`、`rule_moved`、`rule_retired`）。
- 結束代碼與設定相同（0 表示成功，1 表示值被拒絕或資料庫錯誤，2 表示引數錯誤）。

<!-- source: link-card-domains c8f674387fa8 -->
## 連結卡片網域

貼文的連結卡片（`link_cards`）來自一個爬蟲：伺服器會擷取被連結的頁面及其圖片。規則會讓這個爬蟲在某個網域及其子網域上停止，而不影響該網域的其他一切。規則有兩種模式之一。規則是營運者的選擇，除了被連結網站本身拒絕的內容之外另外適用（[連結預覽](link-previews.md#cards-of-links-in-posts)）。

| 模式 | 擷取的內容 | 已有的卡片 |
| --- | --- | --- |
| 停止擷取（`no_fetch`） | 不擷取該網域的任何東西，頁面和圖片都不擷取，經由重新導向到達時也一樣 | 隱藏 |
| 只停止圖片（`no_images`） | 擷取頁面，但不擷取該網域的圖片 | 顯示，但不含圖片 |

- 規則只與連結卡片有關。該網域仍會聯邦，其貼文的媒體也照常擷取：若要切斷某台伺服器，請使用[網域處置](moderation.md)，它也會停止其連結卡片。規則不會與網域處置一起列出或匯出。
- 規則涵蓋網域及其子網域，圖片則會以頁面和圖片兩者的主機來檢查。涵蓋某個主機的所有規則都會適用，所以子網域上的規則不會放寬其上層網域的規則。網域的寫法和正規化方式與網域處置相同：不含 scheme 或連接埠的主機名稱，國際化網域可用任何拼寫方式。
- 變更會立即生效，已有的卡片也一樣，而且在停止圖片期間，這些卡片經代理的圖片也會停止提供。
- 移除規則後，伺服器已有的卡片會恢復原狀。被略過的連結，會在下次有貼文需要時取得卡片；不會大量重新擷取。在停止圖片期間重新擷取的卡片，在下一次重新擷取之前都不會有圖片。
- 在網頁用戶端的管理畫面（「連結卡片網域」，`/admin/link-card-domains`，`manage_settings`）中，「新增規則」可以設定附帶備註的規則。列表每頁顯示 40 條規則，可以依網域搜尋；選擇網域會開啟其規則的頁面，在那裡可以變更模式和備註，或（經過確認對話框後）移除它。
- 管理 API 為 `/api/v1/admin/link_card_domains`（列出、設定；以網域發出 `DELETE` 即可移除），需要角色權限 `manage_settings`。一台伺服器最多保存 1,000 條規則，每條的備註最多 5,000 個字元。
- 每次變更都會記錄在稽核記錄中（`link_card_domain_fetch_stopped`、`link_card_domain_images_stopped`、`link_card_domain_rule_removed`）。

<!-- source: theme 380636a0e583 -->
## 主題

伺服器整體的主題，是一組覆寫網頁用戶端設計 token 的值，再加上一段選填的 CSS（自訂 CSS）。它會套用到每位檢視者的網頁用戶端，以及伺服器產生的畫面（登入、註冊、服務條款等），淺色和深色都會套用。

- 除了基本色（`--palette-*`）之外，你可以覆寫 `app/ui/src/theme.css` 中的每個 token。列表、預設值和範圍請參閱 [app/ui/README.md](../../app/ui/README.md#server-themes)。顏色分別以淺色和深色的 `#rrggbb` 指定（`color-overlay` 也可以是 `#rrggbbaa`）；其他 token 則接受其種類形式（長度、數字、時間長度、緩動、字型名稱列表、陰影）且在其範圍內的值。覆寫 `font-sans` 或 `font-mono` 時，請將 `"my Noto Color Emoji"` 保持在第一個；否則網頁用戶端的表情符號會以作業系統的字型繪製。
- 儲存時，伺服器會檢查值的形式和範圍、每一組顏色 token 的對比（未覆寫的 token 使用預設值；淺色和深色都須符合 WCAG 2.2 AA），以及自訂 CSS 的長度（64 KiB）。任何一項檢查失敗，就不會儲存任何東西。
- 伺服器不會解讀自訂 CSS。它會將其放在同一份樣式表中、token 規則之後，不在任何 `@layer` 內。它在與網頁用戶端相同的 CSP 下執行，所以無法載入來自其他來源的圖片、字型和 `@import`。**伺服器不會檢查自訂 CSS 的影響（對比、水平捲動、點擊目標大小、焦點指示、由右至左的語言）；這是管理員的責任**。 類別名稱和頁面結構會隨版本改變，所以只有 token 是可以穩定依賴的。
- 主題會保留為修訂版本（`instance_theme_revisions`），目前修訂版本的樣式表會在 `/theme/{revision ID}.css` 提供（`immutable`，長時間快取）。HTML 外殼有共用快取（60 秒，加上 300 秒的重新驗證），所以儲存後需要幾分鐘，所有人才會拿到它。已開啟的頁面在重新載入之前會維持舊的外觀。
- 外殼中的 `<meta name="theme-color">`（淺色和深色），以及 Web App Manifest 的 `theme_color` / `background_color`（淺色），會採用主題的 `color-surface` 值。
- 儲存一個沒有 token 也沒有自訂 CSS 的修訂版本，會將主題重設為預設值，樣式表也不再載入。
- 你可以在網頁用戶端的管理畫面（「主題」，`/admin/theme`）、透過用戶端 API（`GET` / `PUT /api/v1/admin/theme`），以及透過 CLI 變更它。管理畫面和用戶端 API 需要角色權限 `manage_settings`，變更會記錄在稽核記錄中（`setting_changed`，目標為 `theme`）。

```sh
my instance theme get > theme.json                    # print the current theme as the same JSON as the admin API
my instance theme set < theme.json                    # replace the whole theme (if a check fails, it prints why and changes nothing)
echo '{"tokens":[],"custom_css":""}' | my instance theme set   # back to the default
```

```json
{
  "tokens": [
    { "token": "color-accent", "light": "#1447e6", "dark": "#92c5fe" },
    { "token": "radius-md", "value": "0.25rem" }
  ],
  "custom_css": ".brand { letter-spacing: 0.02em; }"
}
```

<!-- source: logo-and-icon a2d23def322b -->
## 標誌與圖示

除了縮圖和預覽圖片之外，實例還可以有自己的圖示、標誌和深色標誌，各自以一個 SVG 上傳。在設定之前，會使用內建的 mycroblog 圖片，清除後也會恢復內建圖片。

| 圖片 | 形狀 | 出現的位置 |
| --- | --- | --- |
| `icon` | 正方形（誤差 1% 以內） | 網站圖示（favicon）、iOS 的主畫面圖示（`apple-touch-icon`）、已安裝應用程式的圖示（Web App Manifest）、首頁上的伺服器卡片和網頁用戶端側邊面板中伺服器名稱旁，以及用戶端以正方形圖片顯示伺服器的任何地方 |
| `logo` | 寬度為高度的 1 到 8 倍 | 網頁用戶端的頁首，以及登入、註冊和同意頁面的頂端，取代內建的文字標誌和伺服器名稱（標誌應包含名稱，名稱即為其替代文字）、每封電子郵件的 HTML 版本，以及未設定 `preview_image` 時的連結預覽圖片 |
| `logo_dark` | 與 `logo` 相同 | 深色背景；沒有它時，那裡也使用 `logo` |

- **伺服器會清理 SVG**。 它會將 SVG 當作 XML 讀取，並只用允許的元素（形狀、群組、`defs`、`symbol`、`use`、漸層、`clipPath`、`mask`、`pattern`、`style`、`title`、`desc`）和屬性（幾何、變形、呈現屬性、`id`、`class`、`style`）寫出新的文件。指令碼、`foreignObject`、內嵌圖片、連結、動畫、濾鏡（陰影和模糊）、事件處理器、對檔案外部任何東西的參照，以及 `data:` URL 都會被移除，回應中會列出被移除的項目。只有清理後的 SVG 會被儲存。
- 以下情況 SVG 會被拒絕（不會儲存任何東西）：不是格式正確的 UTF-8 XML、含有 `DOCTYPE` 或實體宣告、沒有 `viewBox`、超過 256 KiB（清理後超過 128 KiB）、超過 5,000 個元素、巢狀深度超過 32 層、超過 200 個 `use` 元素、形狀不符合其種類、含有 `text`（請先在編輯器中將文字轉為外框），或含有可能載入東西的 CSS（`@import`、`@font-face`、`url(#id)` 以外的 `url()`、跳脫字元）。允許在 `<style>` 中使用 `@media (prefers-color-scheme: dark)`，所以圖示可以在深色模式下改變。
- 有些地方不接受 SVG（iOS 主畫面、較舊 Safari 的網站圖示、連結預覽、電子郵件）。為此，上傳 SVG 的用戶端也會從清理後的 SVG 繪製 PNG 版本並一起送出：`icon` 為 32、48、180、192 和 512 px，以及一個 512 px 的 maskable 圖示；`logo` 為高 96 px 的電子郵件標誌和 1200 × 630 的連結預覽圖片；`logo_dark` 為高 96 px 的電子郵件標誌。伺服器會檢查每一個都是尺寸正確的 PNG（最多 1 MiB；180 px 圖示、maskable 圖示和連結預覽必須不透明），並在去除中繼資料後重新寫出。只要有任何一個檔案有誤，就不會儲存任何東西。圖示的不透明版本會繪製在背景色 `#rrggbb` 上，預設為主題的淺色 `color-surface`。伺服器不會檢查 PNG 版本是否與 SVG 顯示相同的圖案。
- 檔案儲存在 blob 儲存區的 `instance/brand/<revision>/` 之下，並以 `Cache-Control: public, max-age=31536000, immutable` 在 `https://<host>/brand/<revision>/<version>.svg|.png` 提供，因為修訂版本的內容永遠不會改變。SVG 會以 `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; sandbox` 和 `X-Content-Type-Options: nosniff` 提供，所以即使單獨開啟，它也不會執行或載入任何東西。先前修訂版本的檔案會保留，讓已寄出的電子郵件仍保有其標誌。
- 設定了圖示時，HTML 外殼會連結其 PNG 網站圖示、SVG 和 180 px 的 `apple-touch-icon`，Web App Manifest 會依序列出其 512 px、192 px、maskable 和 SVG 圖示（其他聯邦宇宙伺服器會將第一個當作這台伺服器的圖示），而 `/apple-touch-icon.png` 會回傳 180 px 的版本。外殼的 ETag 會隨圖片改變，外殼的共用快取和主題一樣會持續幾分鐘。已安裝的應用程式在 Android 上，會在 Chrome 下次檢查 manifest 時取得新圖示；在桌面上，應用程式在重新安裝之前會保留舊圖示。
- `GET /api/v1/instance` 會回傳 `icon`、`logo` 和 `logo_dark`，附上 SVG 的 URL、每個 PNG 版本的 URL 和尺寸，以及修訂版本；使用內建圖片時則會省略。
- 請在網頁用戶端的「伺服器設定」>「圖示與標誌」（`/admin/settings/branding`，角色權限 `manage_settings`）中設定。為圖示、標誌或深色背景用的標誌選擇一個 SVG 檔案：伺服器會檢查它，瀏覽器會繪製 PNG 版本，表單會預覽它們（瀏覽器分頁、主畫面、應用程式圖示、淺色和深色背景上的電子郵件，以及連結預覽），並列出伺服器移除的項目。如果伺服器拒絕該檔案，按鈕下方會顯示原因及修正方式。圖示的不透明版本的背景色也在那裡設定。「儲存」會將 SVG 連同所有 PNG 版本一起送出；「使用內建圖示」、「使用內建標誌」和「移除深色背景用的標誌」會清除它們。網頁用戶端會保留已顯示過的圖片以供離線使用。
- 用戶端 API 也能做相同的事，需要角色權限 `manage_settings`：`POST /api/v1/admin/instance/brand/sanitize` 會檢查 SVG，並回傳清理後的 SVG 以及要繪製的 PNG 版本（不會儲存任何東西），`PUT /api/v1/admin/instance/brand/{icon|logo|logo_dark}` 會將 SVG 連同所有 PNG 版本一起儲存（`multipart/form-data`，每個版本一個部分），對同一路徑發出 `DELETE` 會清除其中一個，而 `GET /api/v1/admin/instance/brand` 會回傳目前的圖片以及圖示的背景色。每次變更都會保留為修訂版本（`instance_brand_revisions`），並記錄在稽核記錄中（`setting_changed`，目標為 `brand_icon`、`brand_logo` 或 `brand_logo_dark`）。
- 由於 PNG 版本是由用戶端繪製的，CLI 無法設定它們，但可以顯示它們，並清除損壞的圖片：

```sh
my instance brand show          # each image with its revision, when and by whom it was set, and the URL of its SVG
my instance brand clear icon    # back to the built-in icon (also logo, logo_dark)
```

<!-- source: custom-emoji 5e0ddb3251cf -->
## 自訂表情符號

當有人在貼文的內文或內容警告中，或在顯示名稱、自我介紹或個人檔案欄位中寫下 `:shortcode:` 時，本站的自訂表情符號會以圖片顯示。它們也會透過你送出的 Note 和 actor 的 `Emoji` 標籤，送達 Mastodon、Misskey、GoToSocial 和 Akkoma。

- 短代碼為 2 到 30 個字母、數字或 `_`。它區分大小寫，表情符號註冊後就無法變更。別名（最多 10 個）和類別（最多 64 個字元）用於在表情符號選擇器中搜尋和分組。
- 圖片為 PNG、GIF 或 WebP，最大 `custom_emoji_max_bytes`（預設 256 KiB），每邊最多 1,024 px。伺服器會製作符合 64、128 和 256 px 方框的衍生圖片，不保留原始檔。APNG 會變成其第一個影格的靜態圖片。GoToSocial 預設不會擷取大於 100 KiB 的表情符號，所以建議使用小圖片。
- 停用的表情符號不會出現在任何地方（貼文、個人檔案、聯邦、選擇器），`:shortcode:` 會再次以文字顯示。設定為在選擇器中隱藏的表情符號只是不出現在選擇器中，仍可透過直接輸入來使用。來自其他伺服器的表情符號只能停用，或複製為這台伺服器的表情符號。
- 複製會在與媒體代理相同的限制下擷取其他伺服器表情符號的圖片（不會擷取設有 `reject_media` 網域的圖片），執行與上述相同的檢查，並將其註冊為新的本站表情符號。短代碼為原本的短代碼，或你指定的短代碼。圖片會縮小到 256 px 的方框內，而無法擷取或未通過檢查的圖片不會留下任何東西。
- 刪除無法復原。過去貼文中的表情符號也會變回文字，再次註冊相同的短代碼會成為不同的表情符號。聯邦伺服器已複製的圖片無法刪除。
- 管理畫面（「自訂表情符號」，`/admin/custom-emojis`）、用戶端 API（`/api/v1/admin/custom_emojis`）和 CLI 能做相同的事。在畫面上，「註冊自訂表情符號」會開啟註冊表單（`/admin/custom-emojis/new`），在列表中選擇表情符號會開啟其頁面（`/admin/custom-emojis/{id}`），在那裡可以變更設定、替換圖片、啟用或停用、刪除，或複製其他伺服器的表情符號；列表本身也能一鍵啟用和停用表情符號。管理畫面和用戶端 API 需要角色權限 `manage_custom_emojis`（只有內建的 `admin` 擁有）。不論以何種方式進行，註冊、複製、替換圖片、變更設定和別名、啟用和停用，以及刪除，都會記錄在稽核記錄中（目標為 `custom_emoji`）（從 CLI 進行的操作會記錄為 CLI 操作）。

```sh
my admin emoji create -shortcode blobcat -category Cats -alias neko blobcat.png   # register one
my admin emoji list -local                                                        # list local emoji, newest first
my admin emoji update -category Animals -alias cat -alias neko <emoji id>         # change the category and aliases (aliases are replaced; -no-aliases removes them)
my admin emoji update -hidden <emoji id>                                          # hide it from the picker (-hidden=false restores it)
my admin emoji replace-image <emoji id> blobcat2.png                              # replace the image
my admin emoji disable <emoji id>                                                 # disable it (remote emoji too)
my admin emoji enable <emoji id>                                                  # enable it again
my admin emoji list -remote -domain mastodon.example                              # list emoji of another server
my admin emoji copy [-shortcode <name>] <emoji id>                                # copy a remote emoji to local
my admin emoji delete <emoji id>                                                  # delete it
my admin emoji requests list [-state pending|approved|rejected|withdrawn|lapsed|all] # list creation requests (pending by default)
my admin emoji requests approve <request id>                                      # approve a request
my admin emoji requests reject [-reason <text>] <request id>                      # reject a request (the requester sees the reason)
```

- 擁有角色權限 `create_custom_emojis`（一項使用者功能）的使用者，可以從設定中的「自訂表情符號」（`/settings/custom-emojis`）申請建立表情符號、變更自己建立的表情符號的別名、類別和在選擇器中的顯示，以及刪除它們。替換圖片也是一種申請。申請在擁有 `manage_custom_emojis` 的人核准之前，不會出現在任何地方（選擇器、貼文、回應、聯邦）。請在管理畫面「自訂表情符號」的「申請」頁面（`/admin/custom-emojis/requests`）、透過用戶端 API（`/api/v1/admin/custom_emoji_requests`），或透過上述 CLI 核准或拒絕。擁有 `manage_custom_emojis` 的人會收到申請的通知（`admin_custom_emoji_request`），申請者則會收到核准或拒絕的通知。申請、撤回、核准和拒絕都會記錄在稽核記錄中（目標為 `custom_emoji_request`）。
- 待處理的建立申請會保留其短代碼。在該申請被拒絕或撤回之前，無法以相同短代碼提出申請或進行註冊。已核准表情符號的圖片會被複製為實例 actor 的上傳檔案，所以即使申請者的帳號被刪除，它仍會保留。
- 建立者只有在持有 `create_custom_emojis` 期間才能變更自己的表情符號。角色被移除後，表情符號仍會保留。管理員不論建立者是誰，都能變更和刪除所有表情符號。
- 當角色來自支持者方案時，使用者每月可建立的表情符號數量受方案的每月表情符號上限限制。它計算的是使用者在目前日曆月（UTC）內提出、已核准或待處理的建立申請；被拒絕和已撤回的申請不計入，刪除表情符號也不會釋出名額。申請計入其提出的月份，即使在下個月才被核准也一樣。計數會在每月第一天的 00:00 UTC 重設，設定頁面會以使用者自己的時區顯示重設時間。替換表情符號的圖片，以及管理員註冊的表情符號，都不計入。管理員手動指派的角色，以及停用支持者功能的伺服器，都沒有上限。
