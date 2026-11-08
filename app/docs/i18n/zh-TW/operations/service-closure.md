<!-- source: closing-the-service 30c438995ba1 -->
# 結束服務

本頁依序說明停止營運伺服器時要做的事：讓使用者把資料帶走、刪除留在其他伺服器上的帳號，然後停止伺服器並切換到靜態的結束服務頁面。每個指令的詳細說明，請參閱[管理員與角色](admin-cli.md#announcing-the-end-of-the-service)。

<!-- source: the-sequence 4557c35b6851 -->
## 流程

![由上而下顯示結束服務的各個階段：左側是營運者要做的事，右側是伺服器的狀態，中間標示關閉日和匯出期限。下方表格說明每個階段。](images/service-closure.svg)

表格依照圖由上而下排列；「伺服器」欄以 `my admin closure status` 顯示的狀態開頭。關閉日（`-closes-at`）是通知期間的結束，匯出期限（`-export-until`，未設定時為關閉日後 30 天）則是第 5 階段的結束。

| 階段 | 伺服器 | 營運者要做的事 |
| --- | --- | --- |
| 1. 公告 | `announced`。照常運作。停止註冊、邀請和新的支持者訂閱 | `my admin closure announce`，在狀態頁面上張貼公告 |
| 2. 通知期間 | `announced`。照常運作。使用者可以匯出資料並搬家到其他伺服器 | 退款給年付支持者，確認搬家和保存保留 |
| 3. 關閉（關閉日之後） | `closed`。使用者只能登入和匯出。伺服器會為每個帳號投遞 `Delete` | `my admin closure check` 和 `close` |
| 4. 投遞完成 | 記錄 `delivery_completed`；本站的 Actor 也會回傳 `410` | 以 `my admin closure status` 確認。不要停止 worker |
| 5. 直到匯出期限 | `delivery_completed`。維持關閉並持續提供匯出 | 匯出保存保留下的紀錄 |
| 6. 靜態頁面 | 已停止。前端提供靜態頁面，聯邦請求則回傳 `410` | `my admin closure export-html`，切換前端，停止 `my serve` |
| 7. 銷毀 | 無。持續提供靜態頁面和 `410` 約一年 | 銷毀資料庫、blob 和備份。保留網域約一年 |

任何階段都可以搭配維護模式（[維護](maintenance.md)）。維護期間，匯出和 `Delete` 活動的投遞會停止。

<!-- source: 1-announce b34cb9474667 -->
## 1. 公告

決定關閉日（`-closes-at`）和帶走資料的期限（`-export-until`，省略時為關閉日後 30 天），然後公告。

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for being here."
my admin closure status
```

- 關閉日沒有下限，但從公告到關閉日不到 30 天時會出現警告。請留給使用者匯出和搬家的時間。
- 要變更日期，請再次執行 `announce`；要取消，請執行 `my admin closure withdraw`。兩者都會向每位使用者傳送 `service_closure` 通知。
- 公告後，實際的註冊模式會變成 `closed`，等待審核的申請會被拒絕，每筆持續中的支持者訂閱都會在其週期結束時取消（訂閱者會收到通知和電子郵件）。撤回公告不會恢復這些拒絕和取消。
- 通知是由任務建立的，因此不要停止 worker。
- 在狀態頁面（[監控與狀態頁面](monitoring.md)）上張貼關閉日、匯出期限，以及匯出和搬家的方式。狀態頁面的存續時間比伺服器長，因此也能引導在關閉後造訪的人。

<!-- source: 2-notice-period b18535a9c0f7 -->
## 2. 通知期間

- 使用者可以從設定畫面匯出資料（資料匯出和 CSV 檔案），並搬家到其他伺服器（`Move`）。每個畫面上的橫幅和一則通知會顯示關閉日、匯出期限，以及前往該畫面的方式。
- **年付支持者**：公告會在週期結束時取消訂閱，但對於週期在關閉日之後才結束的訂閱，無法使用的那段時間已經付過費用了。伺服器不會退款，因此請在付款服務商的畫面上為每位訂閱者退款（Stripe 為儀表板的付款畫面；PAY.JP 為管理主控台的付款畫面）（[支持者付款（Stripe）](payments-stripe.md)、[支持者付款（PAY.JP）](payments-payjp.md)）。
- 關閉日過後，具備 `manage_service_closure` 的人會收到 `admin_service_closure_due` 通知。伺服器不會自行關閉。

<!-- source: 3-close c851780cf430 -->
## 3. 關閉

關閉日過後，請在關閉之前先執行檢查。

```sh
my admin closure check
```

- **未完成的搬家**會產生警告。請等它們完成後再關閉，因為關閉後就沒有人能開始 `Move`。
- 任何帳號上的**保存保留（`retention_holds`）**都會產生警告。保存保留背後的義務不會因為你關閉伺服器而結束，因此請匯出保存保留下的紀錄，並保存在伺服器之外（用 `my admin retention list` 列出保存保留）。

  ```sh
  my admin account export-moderation @alice > alice-moderation.json
  ```

檢查完成後就關閉。這個操作無法復原。請輸入 `y` 和網域名稱確認（自動化時請使用 `-confirm-domain`）。

```sh
my admin closure close
```

關閉後，使用者只能登入和匯出資料。剩下的支持者訂閱會立即取消，並將每個本站帳號的 `Delete`（Actor）投遞到其他伺服器（即使 `federation_mode` 為 `off` 或 `inbound` 也一樣）。收件匣以及對貼文和集合的 GET 會回傳 `410 Gone`。

<!-- source: 4-confirming-that-delivery-is-complete b545acd9a55c -->
## 4. 確認投遞完成

**在狀態變成 `delivery_completed` 之前，不要停止 `my serve` 和 worker（`role` 為 `all` 或 `worker` 的程序；使用推送式佇列時為 Cloud Tasks 佇列）**。`Delete` 活動由 worker 投遞，在投遞期間，伺服器會持續回傳本站 Actor 的文件和金鑰，讓接收方能驗證 `Delete` 的簽章。如果太早停止，帳號會留在其他伺服器上。

```sh
my admin closure status
```

- `accounts with a Delete` 是已記錄 `Delete` 的帳號數，`fan-outs pending` 是剩下的分送工作，`deliveries <state>` 則是各狀態的投遞數。
- 投遞會依一般策略重試（約 4 天內最多 9 次）。全部結束後，每 10 分鐘執行一次的檢查會記錄 `delivery_completed`（`state: delivery_completed`）。從那時起，本站 Actor 和實例 Actor 也會回傳 `410`。
- 變成 `dead` 的投遞不會自動重試。要重試它，請使用 `my job retry`。

<!-- source: 5-until-the-export-deadline 6464f67f67d1 -->
## 5. 直到匯出期限

即使投遞完成，伺服器仍會以關閉狀態持續運作到 `export_until`，並持續提供使用者匯出。伺服器永遠不會自行刪除資料，因此過了期限後，它仍會持續運作直到你停止它。如果你還沒有匯出保存保留下的紀錄（§3），請現在匯出。

<!-- source: 6-switching-to-the-static-page 9795c9aea7ae -->
## 6. 切換到靜態頁面

匯出期限過後，匯出結束服務頁面。**這會讀取資料庫，因此請在停止 `my serve` 和銷毀資料庫之前執行**。

```sh
my admin closure export-html -out ./closure             # all locales; index.html starts from the instance's default locale
my admin closure export-html -out ./closure -lang ja,en # set the locales and the order in index.html
```

- 它會寫出：
  - `<dir>/<locale>.html`，以及依序列出所有語系的 `<dir>/index.html`。它們使用與維護頁面（`my maintenance export`）相同的範本和 CSS，並顯示伺服器名稱（`maintenance_server_name`，未設定時為實例名稱，再沒有則為主機名稱）、關閉日、公告中營運者的訊息，以及狀態頁面的連結（`status_page_url`）。CSS 只有內嵌，沒有腳本或外部資源，CSP 寫在 `<meta>` 標籤中。主題色彩的覆寫會被寫入；自訂 CSS 不包含在內。
  - 前端設定範例 `nginx.conf`、`Caddyfile` 和 `cloudfront-function.js`。每個範例對聯邦和探索路徑（`/ap/*`、`/inbox`、`/.well-known/webfinger`、`/.well-known/host-meta`、`/.well-known/host-meta.json`、`/.well-known/nodeinfo`、`/nodeinfo/*`）以及個人檔案（`/@*`）回傳不含內文的 `410`，對其他所有路徑則回傳 `Accept-Language` 中第一個語言的頁面（沒有時為 `index.html`）。每種語言會導向應用程式會以哪個語系顯示它的那個語系頁面：`zh`、`zh-SG` 和 `zh-Hans` 會得到 `zh-CN.html`，`zh-Hant`、`zh-HK` 和 `zh-MO` 會得到 `zh-TW.html`。
- 伺服器尚未關閉時，指令會拒絕執行（結束代碼 1）。在 `delivery_completed` 之前，它會警告投遞仍在進行（但仍會寫出檔案）。

切換前端。只將頁面（`*.html`）放在公開位置，並依你的環境調整範例中的主機名稱、目錄和憑證路徑。

| 前端 | 設定方式 |
| --- | --- |
| nginx | 將頁面放在 `/srv/closure`，並從 `http` 區塊 include `nginx.conf`。頁面也會以 `410` 提供 |
| Caddy | 將頁面放在 `/srv/closure`，並使用 `Caddyfile`。Caddy 會取得憑證，頁面也會以 `410` 提供 |
| CloudFront | 將頁面直接放在 S3 bucket 底下，讓該 bucket 成為預設快取行為的來源，並將 `cloudfront-function.js` 關聯為 viewer-request 的 CloudFront Function（`cloudfront-js-2.0`）。請在快取政策的快取鍵中加入 `Accept-Language`。頁面會以 `200` 提供（viewer-request 函式無法變更來源回應的狀態） |

切換後，請確認 `curl -i https://social.example/.well-known/webfinger?resource=acct:alice@social.example` 回傳 `410`，且 `curl -i https://social.example/` 回傳結束服務頁面。接著停止 `my serve` 和 worker。至於運作時間監控，請停止健康檢查的監控，或讓它們指向結束服務頁面。

<!-- source: 7-discarding-the-db-blobs-and-backups c7c55aac56e0 -->
## 7. 銷毀資料庫、blob 和備份

停止後，銷毀資料庫、blob（媒體和匯出檔案）、KV 儲存和佇列，以及它們的備份和快照。使用[搜尋引擎](search.md#closing-the-service)時，也請銷毀其索引和快照：它們保存了貼文的文字。在銷毀任何東西之前，請再確認一次你已經匯出並保存了保存保留下的紀錄。銷毀無法復原。

**保留網域，並持續提供靜態頁面和 `410` 約一年**。其他伺服器看到 `410` 後，會停止擷取和投遞，並清除它們的快取。聯邦 URI 使用 UUID，因此再次建立相同 `id` 的 Actor 的風險很低，但如果其他人取得了這個網域，帳號代碼（`acct:alice@social.example`）和個人檔案 URL 就會歸他們所有，並可能被用來冒充你的使用者。請持續更新網域和憑證，即使過了一年，在放棄網域之前也請再三考慮。
