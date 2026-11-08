<!-- source: administrators-and-roles 1baeba6d67b4 -->
# 管理員與角色

本頁說明如何建立第一位管理員、從命令列管理角色、公告並執行服務終止、重新處理上傳的圖片、重建搜尋索引，以及匯出維護頁面。

<!-- source: prerequisites b7ae62128809 -->
## 事前準備

- 設定（`host`、`database`、`database_url`、`kek` 等）已依照[設定參考](../reference/configuration.md)的說明設好。`my config check` 會驗證設定。
- 結構描述（schema）已套用到資料庫（`my schema apply`；請參閱 [database.md](database.md)）。
- `host` 和 `account_domain` 已經確定。第一個使用資料庫的指令會記錄這兩個值，之後只要設定不同，每個指令都會拒絕執行（請參閱[之後無法變更的設定](production-checklist.md#settings-you-cannot-change-later)）。

<!-- source: creating-an-administrator e5fec2e3d287 -->
## 建立管理員

```sh
my admin create -username alice -email alice@example.com
```

- 從終端機執行時，指令會要求輸入兩次密碼，輸入內容不會顯示在畫面上。
- 標準輸入不是終端機時，標準輸入的第一行就是密碼（例如 `my admin create ... < password.txt`）。密碼不能以命令列引數或環境變數提供，因為這些會留在行程列表和 shell 歷史記錄中。
- 在一次性指令既沒有終端機也沒有標準輸入的環境（例如 [Cloud Run job](deploy-cloud-run.md#9-create-the-first-administrator)）中，請加上 `-password-link`。此時指令不會讀取密碼：它會設定一組沒有人知道的隨機密碼，並在 `created admin` 這一行之後印出一個連結。這個連結和 `my admin account password-reset-link` 一樣，可以在一小時內設定一次密碼（稽核記錄會記為 `password_reset_link_issued`）。能讀到指令輸出的人都可以使用這個連結，而在這類平台上，輸出會一直留在平台的日誌中，直到連結被使用或過期為止。請立刻開啟連結。如果連結過期，請用 `my admin account password-reset-link @alice` 再發一個。如果發行連結失敗，管理員已經建立好了：指令會說明這一點並以結束代碼 1 結束，再執行同一個指令就會得到新的連結。
- 密碼長度為 8 到 128 個字元。密碼絕不會出現在輸出、日誌或錯誤訊息中。
- 一個交易會建立使用者、密碼、主要電子郵件地址、本站帳號（含其金鑰），以及 `admin` 角色的授予（沒有授予者，代表由 CLI 授予）。如果內建角色不存在，會先建立它們。
- 成功時，指令會印出 `created admin @alice@<account_domain> (user <id>, account <id>)`，並以結束代碼 0 結束。
- 在[單人伺服器](instance-settings.md#one-person-server)上，指令不會建立帳號，並以結束代碼 1 結束。當[登入限定使用登入提供者](login-providers.md#limit-sign-in-to-login-providers)時，指令也會警告：在限制解除之前，這組密碼無法用來登入。

<!-- source: running-it-again 197692f7f103 -->
## 再次執行

- 當使用者名稱或電子郵件地址（兩者都不分大小寫）已被使用時，指令不會做任何變更，會印出哪一個已被使用，並以結束代碼 1 結束。它絕不會變更現有使用者的密碼或角色。
- 失敗的執行不會寫入任何東西，所以你可以排除原因後再執行同一個指令。
- 結束代碼 2 表示用法錯誤。

<!-- source: granting-and-revoking-roles b87e1adb131e -->
## 授予與撤銷角色

角色擁有權限。內建角色為 `admin`（`position` 1000，擁有所有權限）和 `moderator`（`position` 100，擁有 `manage_reports`、`manage_accounts` 和 `view_audit`）。`my serve` 每次啟動時（任何 `role` 的行程）都會檢查內建角色，`my admin role` 每次執行時也會檢查。兩者都會建立缺少的角色，並將執行中版本的所有權限賦予 `admin`。升級新增權限時，不需要任何手動步驟。`moderator` 會保留管理員設定的權限和位階。

```sh
my admin role list                 # each role's position, permissions (grouped as moderation, server administration, and member features), and holders
my admin role list -user alice     # the roles of alice
my admin role grant alice moderator
my admin role revoke alice moderator
```

- 權限分屬三個群組之一。
  - 審核：`manage_reports`、`manage_accounts`、`manage_domains`、`manage_invites`、`view_audit`。
  - 伺服器管理：`manage_settings`、`manage_rules`、`manage_roles`、`manage_jobs`、`manage_login_recovery`、`manage_custom_emojis`、`manage_webhooks`、`manage_announcements`、`manage_finance`、`manage_supporters`、`manage_oauth_apps`、`manage_service_closure`、`manage_errors`、`manage_login_providers`。
  - 會員功能：`view_finance`、`create_custom_emojis`、`start_voice_rooms`、`create_invites`、`keep_posts_longer`。

  `list` 會為每個群組印出一欄（`MODERATION`、`SERVER`、`MEMBER`）。網頁用戶端將前兩個群組顯示為「審核」和「管理」，會員功能則不計入任何一個（請參閱 [moderation.md](moderation.md)）。
- 變更會在該使用者的下一次請求時生效。給版主的通知，會送給通知建立當下擁有該權限的人。
- 授予和撤銷會以 `role_granted` 和 `role_revoked`（目標種類 `role`）寫入稽核記錄。CLI 操作的執行者是 `cli`（`my admin audit list -target-kind role`）。
- 授予使用者已有的角色，或撤銷使用者沒有的角色，不會變更也不會記錄任何東西。
- `admin` 的最後一位持有者無法失去這個角色（結束代碼 1）。請先將 `admin` 授予另一位使用者。
- 管理 API 為 `GET /api/v1/admin/roles`、`GET /api/v1/admin/accounts/{id}/roles` 以及 `PUT` / `DELETE /api/v1/admin/accounts/{id}/roles/{role}`，需要 `manage_roles` 權限。透過 API，你無法授予或撤銷高於你自己角色中最高 `position` 的角色（403）。撤銷最後一個 `admin` 會回傳 409。
- 工作人員只能對位階較低的人採取行動。透過 API 執行審核處置及其解除、裁決申訴、解除電子郵件的寄送抑制、揭露帳號的私人資料、發行密碼重設連結，以及移除兩步驟驗證時，你角色中的最高 `position` 必須高於對象所有角色的位階；對位階較高或相同的對象（包括你自己的帳號）會回傳 403。版主無法對管理員採取行動，管理員之間也無法互相採取行動。這些指令（`my admin account ...`）沒有這項限制，所以當管理員之間意見不一時，請使用這些指令。
- 唯一的管理員無法被鎖在外面或被移除：對其執行 `disable_login`、`suspend` 和 `ban` 時，透過 API（409）和這些指令（結束代碼 1）都會被拒絕，而且他們也無法刪除自己的帳號（409）。其他 `admin` 持有者只有在能夠登入時才算數：刪除帳號後，或在停權、封禁或 `disable_login` 期間都不算。

<!-- source: announcing-the-end-of-the-service e1c40ec145f0 -->
## 公告服務終止

要關閉伺服器，請先公告關閉。[service-closure.md](service-closure.md) 說明從公告到捨棄資料的完整程序。公告期間伺服器照常運作。`GET /api/v1/instance` 中的 `closure` 和 NodeInfo 中的 `metadata.serviceClosure` 會顯示狀態和日期。

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for using this server."
my admin closure announce -closes-at 2027-02-28 -export-until 2027-04-30   # change the dates (a bare date means 00:00 UTC)
my admin closure status                                                   # the state and the schedule in effect
my admin closure withdraw                                                 # withdraw the announcement
```

- `-closes-at` 沒有下限。`-export-until`（取出資料的期限）必須在 `-closes-at` 當天或之後，預設為關閉日的 30 天後。
- 關閉日距今不到 30 天時，指令會在標準錯誤輸出印出警告（公告仍會被記錄）。
- 變更日期會被記錄為一則新的公告，撤回後再發的公告也是新的公告。伺服器關閉後，就既不能公告也不能撤回（結束代碼 1）。
- 公告、變更日期和撤回會以 `service_closure_announced` 和 `service_closure_withdrawn`（目標種類 `instance`；請參閱 `my admin audit list -target-kind instance`）寫入稽核記錄。
- 管理 API 為 `GET /api/v1/admin/closure`、`PUT` / `DELETE /api/v1/admin/closure/announcement`、`GET /api/v1/admin/closure/check` 和 `POST /api/v1/admin/closure/close`。需要 `manage_service_closure` 權限（內建角色中只有 `admin` 擁有）。
- 公告期間，伺服器不接受新使用者，也不接受新的付款。
  - 實際生效的註冊模式為 `closed`（`registration_mode` 的值不會改變，撤回公告後會恢復生效）。一般註冊和使用邀請的註冊都無法使用，待審核的申請也無法核准（`409`）。
  - 公告時會拒絕所有待審核的申請（記錄為由 `service_closure` 裁決），並將所有進行中的支持者訂閱設為在其期間結束時取消。訂閱者會收到通知（`supporter_service_closure`）和電子郵件。新的訂閱會回傳 `409`。撤回公告不會復原這些拒絕和取消。
  - 每一次公告、變更日期和撤回，都會將 `service_closure` 通知送給所有本站使用者。這些都以任務執行，所以請不要停止 worker。
  - 所有已確認電子郵件地址的使用者，會在公告、變更日期、撤回、關閉日前 7 天和 1 天、關閉時，以及匯出期限前 7 天收到電子郵件。使用者無法拒收（請參閱 [email.md](email.md)）。
  - 關閉日過後，擁有 `manage_service_closure` 的人會收到 `admin_service_closure_due` 通知。伺服器不會自行關閉。

關閉日過後，請關閉伺服器。關閉無法復原。

```sh
my admin closure check                                      # review what the closure ends
my admin closure close                                      # show the review, then ask for y and the domain name
my admin closure close -confirm-domain social.example       # for automation: pass the domain name instead of answering
```

- `check` 和 `close` 會顯示本站帳號數、目的地收件匣數（已知的共用收件匣）、預計要送出的 `Delete` 活動數（兩者的乘積）、進行中的支持者訂閱數、尚未完成的搬遷（`Move`）數，以及處於保留措施下的帳號數（`retention_holds`）。還有搬遷未完成或存在保留措施時，指令會在標準錯誤輸出發出警告。請用 `my admin account export-moderation` 匯出處於保留措施下帳號的紀錄，並在捨棄資料前妥善保存。
- 沒有公告時以及關閉日之前，`close` 會不經詢問直接拒絕（結束代碼 1）。回答 `y` 以外的任何內容，或輸入與伺服器網域（設定中的 `host`，不分大小寫）不同的值時，不會做任何變更，並以結束代碼 1 結束。
- 關閉會以 `service_closure_closed` 寫入稽核記錄。在同一個交易中，它會結束所有進行中的支持者訂閱（原因為 `service_closed`），並將立即在付款服務商取消訂閱的任務（`CancelNow`）排入佇列。
- 關閉後，用戶端 API 只放行以下請求：讀取你自己的帳號和設定、匯出（`/exports`、`/csv_exports`）、工作階段（`/sessions`、`/authorized_apps`）、`GET /instance`，以及關閉和任務佇列的管理。其他一切都會以 `403`（`/api/v1/problems/service-closed`）拒絕。串流會停止，Web Push 也不再送出任何東西。登入和重設密碼仍可使用。
- 關閉後，worker 會將每個本站帳號 actor 的 `Delete` 遞送給聯邦中的對方伺服器，每次 100 個（已搬遷的帳號排在最後）。即使 `federation_mode` 為 `off` 或 `inbound` 也會送出，但絕不會送到已停權的網域。`my admin closure status`（以及 `GET /api/v1/admin/closure` 中的 `progress`）會顯示已記錄 `Delete` 的帳號數、尚待展開的遞送數，以及各狀態（`pending`、`succeeded`、`permanent_failure`、`skipped`、`dead`）的遞送數。遞送期間請讓 worker 持續執行。所有遞送完成後，每 10 分鐘執行一次的檢查會記錄 `delivery_completed`（稽核記錄中為 `service_closure_delivery_completed`），之後本站 actor 會回應 `410`。伺服器不會自行重新執行 `dead` 的遞送；請用 `my job retry` 重新執行。

匯出期限過後，請寫出伺服器停止後由你的網域提供的靜態關閉頁面。這個指令會讀取資料庫，所以請在捨棄資料庫之前執行。

```sh
my admin closure export-html -out ./closure              # every locale (the instance's default locale first)
my admin closure export-html -out ./closure -lang ja,en  # choose the locales and the order in index.html
```

- 輸出：每個語系一個 `<dir>/<locale>.html`，以及依序包含所有語系的 `<dir>/index.html`。這些頁面使用與 `my maintenance export` 相同的範本，顯示伺服器名稱、關閉日、公告中營運者的訊息，以及 `status_page_url`。輸出中也包含前端的設定範例：`nginx.conf`、`Caddyfile` 和 `cloudfront-function.js`。它們對 `/ap/*`、`/inbox`、WebFinger、host-meta、NodeInfo 和 `/@*` 回應 `410`，其他一切則提供關閉頁面。
- 除非伺服器已關閉，否則指令會拒絕執行（結束代碼 1）。在 `delivery_completed` 之前，它會印出警告但仍會寫出頁面。[service-closure.md](service-closure.md) 說明如何將前端切換到靜態頁面。

<!-- source: processing-uploaded-images-again 65a9e3da0bd8 -->
## 重新處理上傳的圖片

每張上傳的圖片會保留三種尺寸，動畫圖片還會保留其動態版本。`my` 的新版本可能會變更這些尺寸或格式，而現有圖片會一直保留原本的檔案，直到你重新處理為止：

```sh
my admin media reprocess -before 2027-01-31T09:00:00Z
```

- `-before` 以 RFC 3339 格式指定變更的時間，例如你升級的時間。只有最後處理時間早於該時間的圖片會被處理，尚未到來的時間會被拒絕。
- 任務由 worker 執行，每分鐘 100 張圖片，每張各一個任務（[任務參考](../reference/jobs.md)中的 `media.reprocess_walk` 和 `media.reprocess`）。請讓 worker 保持執行；管理頁面的任務頁（`/admin/jobs`）會顯示佇列。
- 以相同時間再次執行是安全的。已處理的圖片不會被動到，第一次執行沒處理到的圖片則會被接手處理。
- 原始檔不會保留，所以每張圖片都是從其大尺寸版本重新製作。大尺寸版本已失去的細節不會回來，而且每次以更晚的時間執行，都會再重新編碼一次而損失一些畫質。每次變更請只執行一次。
- 影片和音訊檔案不會重新編碼，也不會被動到。目前限制已不再接受的圖片會保留現有版本，worker 會記錄警告。

圖片的新版本會有新的 URL。舊檔案會保留 `media_replaced_retention` 的時間（預設 30 天，[設定參考](../reference/configuration.md#media_replaced_retention)），讓開著的頁面和快取能繼續運作，之後每小時執行的任務 `media.purge_replaced` 會刪除它們。其他伺服器不會被告知新的 URL：仍持有舊 URL 且沒有自己副本的伺服器，在那之後就無法再顯示該圖片。在舊檔案刪除之前，請預留額外的儲存空間。

<!-- source: rebuilding-the-search-index 9061821656ac -->
## 重建搜尋索引

在有搜尋引擎的伺服器上，`my search` 負責處理貼文全文搜尋的索引。在 `search = none` 時，每個指令都會說明這一點、不做任何事，並以結束代碼 0 結束。

```sh
my search rebuild              # empty the index, then have the worker add every post that qualifies
my search rebuild --keep       # add them without emptying the index
my search rebuild --rate 50    # at most 50 posts a second (20 by default, 1000 at most)
my search clear                # empty the index and end any rebuild
my search status               # the backend and the progress and failures of the latest rebuild
```

再次執行 `my search rebuild` 可以繼續已停止的重建。它會從最後處理的貼文接著進行，不會再次清空索引。有一個重建尚未完成時，另一種模式會被拒絕，並以結束代碼 1 結束。[貼文的全文搜尋](search.md)說明了各自的使用時機。

<!-- source: exporting-the-maintenance-page 1756fcf1e02e -->
## 匯出維護頁面

`my maintenance export` 會將維護模式的頁面寫成靜態 HTML，讓負載平衡器、CDN 或反向代理在 `my serve` 本身無法執行時（正在替換映像檔、某個修訂版本無法啟動）提供。它使用與維護模式相同的範本，所以兩者顯示的文字和外觀相同。

```sh
my maintenance export -out ./maintenance -lang en,ja            # reads the database for at most 5 seconds
my maintenance export -out ./maintenance -lang en,ja -offline   # the configuration and the default theme only
```

- 輸出：`-lang` 的每個語系一個 `<dir>/<locale>.html`，供依 `Accept-Language` 選擇頁面的前端使用；以及依序包含所有語系的 `<dir>/index.html`，供無法這樣選擇的前端使用。這些頁面使用內嵌 CSS，沒有指令碼，也沒有外部資源；其 Content-Security-Policy 寫在 `<meta>` 元素中。
- 文字：設定 `maintenance_server_name`、`maintenance_message`、`maintenance_until` 和 `status_page_url`（[設定參考](../reference/configuration.md)）。
- 未加上 `-offline` 時，它還會讀取實例的名稱（`maintenance_server_name` 未設定時）、預設語系（未指定 `-lang` 時），以及主題的 token 覆寫值。主題的自訂 CSS 絕不會包含在內。無法讀取資料庫時，它會印出警告，以設定和預設主題寫出頁面，並以 0 結束。
- 每當這些設定或主題變更時，以及每次維護之前，請再次執行，讓前端中的頁面保持最新。
