<!-- source: federation-relays bbd5bb98dde3 -->
# 聯邦中繼站

中繼站（relay）是一種伺服器，會把加入它的伺服器的公開貼文轉送給其他加入的伺服器。即使是追蹤關係不多的小型實例，聯邦時間軸和主題標籤時間軸也會因此充實起來，自己的公開貼文也能傳得更遠。沒有加入中繼站的實例不會因此增加任何資料列或任務。

<!-- source: join-a-relay 147b07e65534 -->
## 加入中繼站

請使用管理區的「中繼站」（`/admin/relays`）。這需要 `manage_settings` 權限，內建的 `admin` 角色具有此權限。「加入中繼站」會開啟登錄中繼站的表單。列表會顯示每個中繼站的狀態，並附有退出或重新加入的按鈕；選擇某個中繼站會開啟它的頁面，顯示它的網址、過去 24 小時帶進來的內容，以及刪除它的按鈕。CLI 是 `my admin relay`（`my admin relay -help`）：

```sh
my admin relay list                                               # every relay, its state, and what it brought in during the last 24 hours
my admin relay create [-protocol mastodon|litepub] [-send-only] <url>  # register a relay and send it the Follow
my admin relay unfollow <relay id>                                # leave a pending or joined relay
my admin relay follow <relay id>                                  # join again a relay that the instance left or that rejected it
my admin relay delete <relay id>                                  # delete a relay that the instance left or that rejected it
```

1. 登錄中繼站的 URL。有兩種協定：
   - **Mastodon**：中繼站的 **inbox URL**（例如 `https://relay.example/inbox`）。包括 pub-relay 和 Activity-Relay 在內，許多中繼站都接受這種方式。
   - **LitePub**：中繼站的 **actor URL**（例如 `https://relay.example/relay`）。這是 Pleroma 和 Akkoma 中繼站使用的協定。
   如果你沒有選擇協定（「自動偵測」），當該 URL 提供 actor 文件時，伺服器會使用 LitePub，否則使用 Mastodon。
2. 登錄之後，實例 actor 會向中繼站送出 `Follow`，狀態為「等待中繼站回應」（`pending`）。中繼站接受後，狀態會變成「已加入」（`accepted`）；拒絕時則為「被中繼站拒絕」（`rejected`）。有些中繼站由營運者逐一手動核准申請，因此可能需要一段時間。
3. 如果登錄時關閉「接收來自中繼站的貼文」，伺服器就只會送出。要變更這項設定，請退出中繼站後重新登錄。

你無法登錄受到網域處置（`suspend`）的伺服器上的中繼站。如果已加入的中繼站所在的伺服器之後被停權，狀態不會改變，但伺服器不會再與該中繼站互相傳送任何內容，管理區也會顯示警告。

<!-- source: what-the-server-sends 1db52c508f9c -->
## 伺服器送出的內容

- 只會送出本站**公開**（`public`）貼文的建立、編輯與刪除。`unlisted`、僅追蹤者、私訊或僅限本站（`local`）的貼文，以及轉發，一律不會送出。若不想讓貼文進入中繼站，請以 `unlisted` 或 `local` 發布。沒有個別使用者選擇退出的設定。
- 公開貼文的文件帶有 Linked Data Signature（`RsaSignature2017`），讓 Mastodon 和 Misskey 能接受由中繼站原封不動轉送的貼文。
- 傳送次數會增加「已加入的中繼站數量 × 公開貼文數量」（對同一個中繼站的傳送會合併為一次）。

<!-- source: what-the-server-receives ff962150d1a7 -->
## 伺服器接收的內容

- 已加入的中繼站帶來的公開貼文，會先從原始伺服器重新擷取並檢查，之後才會收錄。收錄後會出現在聯邦時間軸和主題標籤時間軸上。它不會出現在首頁時間軸上，也不會產生通知。
- 每個中繼站每小時最多收錄 `relay_max_activities_per_hour` 則貼文（預設 1,000）。超過上限的部分會直接捨棄，不會擷取。管理區中該中繼站的頁面和 `my admin relay list` 會顯示過去 24 小時收錄了多少則、因超過上限而捨棄了多少則。如果捨棄的數量很多，請提高上限或重新考慮是否使用這個中繼站。
- 如果貼文的網域或帳號受到 `suspend`（聯邦限制）或 `limit`（將其從公開場所移除的限制），就不會收錄該貼文。
- 只經由中繼站送達的貼文，會在實例設定 `relay_status_retention_days`（預設 7）之後從公開檢視中移除。本站有人回覆、轉發、喜歡或加入書籤的貼文，以及本站有人追蹤的作者的貼文，保留期限與其他遠端貼文相同（`remote_status_retention_days`）。
- 每收錄一則貼文，就要從原始伺服器擷取一次（作者不明時，還要再擷取一次 actor）。以每小時 1,000 則的上限計算，一天最多 24,000 次擷取。

這些設定請參閱[設定參考](../reference/configuration.md#relay_max_activities_per_hour)。收錄的結果也會以指標 `my.relay.activities` 輸出（請參閱[執行伺服器](running.md)）。

<!-- source: leave-join-again-and-delete ea12da94f940 -->
## 退出、重新加入與刪除

- 「退出」會送出針對 `Follow` 的 `Undo`，並把狀態設為「已退出」（`left`）。之後不會再與該中繼站互相傳送任何內容。已經收錄的貼文會在保留期限結束時移除。
- 「重新加入」會向拒絕過伺服器或伺服器已退出的中繼站重新送出 `Follow`。
- 「刪除」會移除已退出或被拒絕的中繼站的登錄。刪除之後，已經送到該中繼站的貼文的編輯與刪除，仍然會傳送給它。

登錄、加入、退出和刪除，以及中繼站的接受或拒絕，都會以目標類型「中繼站」記錄在稽核日誌中（接受與拒絕會記錄為遠端操作）。
