<!-- source: server-webhooks e73d0e0966b7 -->
# 伺服器 Webhook

Webhook 會把檢舉、註冊申請、帳號建立與個人檔案變更、審核處置、申訴、網域處置、本站的公開貼文，以及自訂表情符號的管理操作，送到 Slack 或 Discord 的轉送服務、工單系統，或 n8n 之類的自動化工具。沒有設定任何 Webhook 的伺服器，不會儲存額外的資料列，也不會執行額外的任務。接收端會收到什麼（標頭、簽章、事件、內文），請參閱 [Webhook 參考文件](../reference/webhooks.md)。

<!-- source: set-up-a-webhook c8a80bc40038 -->
## 設定 Webhook

擁有 `manage_webhooks` 權限的人，都可以在管理畫面「Webhook」（`/admin/webhooks`）設定 Webhook。內建的 `admin` 角色擁有這個權限。「新增 Webhook」會開啟表單；在列表中選擇一個 Webhook 會開啟它的頁面，你可以在那裡修改、測試、啟用、停用和刪除它，為它產生新的密鑰，以及查看它的傳送記錄。

1. 建立 Webhook，填入目的地 URL、說明，以及要訂閱的事件。除了 `manage_webhooks` 之外，每種事件還各自需要對應的權限：檢舉需要 `manage_reports`，註冊、帳號、審核處置和申訴需要 `manage_accounts`，網域處置需要 `manage_domains`，自訂表情符號需要 `manage_custom_emojis`，錯誤群組需要 `manage_errors`。
   公開貼文不需要額外的權限。你無法選擇自己缺少權限的事件。
2. 密鑰（`whsec_...`）只會在建立 Webhook 時顯示一次。請把它設定在接收端。之後無法再次顯示。
3. 在 Webhook 的頁面選擇「傳送測試」來傳送 `webhook.test`，並在傳送記錄中確認它已送達。

建立、修改、刪除、啟用、停用 Webhook，以及輪替它的密鑰，都會記錄在稽核日誌中。目標會以說明和 URL 的主機名稱顯示；URL 本身不會被記錄。

<!-- source: rotate-the-secret 928c62f87538 -->
## 輪替密鑰

「新密鑰」會建立新的密鑰並顯示一次。接下來的 24 小時內，訊息也會用舊的密鑰簽章（`webhook-signature` 會帶有兩個簽章），因此你可以在這段期間把接收端切換到新的密鑰。密鑰在存入資料庫之前，會先以 `kernel.Sealer` 加密封存；所用的金鑰是 [KEK](key-management.md)。

<!-- source: failures-and-redelivery 65aba0d2089e -->
## 失敗與重新傳送

- 如果目的地回應 `2xx` 以外的狀態、15 秒內沒有回應，或無法連線，訊息會在 5 秒、5 分鐘、30 分鐘、2 小時、5 小時、10 小時和 10 小時後重試（總共 8 次嘗試，約 27 小時）。用完所有嘗試次數的傳送會成為失敗任務（dead job），在傳送記錄中選擇「重新傳送」，會以相同的 `webhook-id` 再傳送一次。
- 如果目的地回應 `410 Gone`，Webhook 會立即停用。
- 若某個目的地從第一次失敗起 72 小時內都沒有成功，且連續失敗 20 次以上，伺服器會停用它。稽核日誌會留下一筆由 `system` 執行的 `webhook_disabled` 記錄，所有擁有 `manage_webhooks` 的人都會收到 App 內通知（如果他們選擇以電子郵件接收通知，也會收到電子郵件）。
- 管理畫面的列表會顯示連續失敗的次數，以及伺服器停用 Webhook 的原因（`gone` 或 `failing`）。修好目的地之後，請重新啟用 Webhook。停用期間的訊息不會再次傳送。
- 訊息與傳送記錄會在 14 天後刪除（固定）。內文含有檢舉的留言等個人資料。

<!-- source: destinations-on-an-internal-network 3f8b39001831 -->
## 內部網路上的目的地

傳送會經過對外 HTTP 的 SSRF 防護。預設只能透過 `https`（連接埠 443 和 80）連到公開位址。若要傳送到內部的目的地，例如同一台主機上的 n8n 或 VPC 內的聊天服務，請在設定 `webhook_allowed_destinations`（環境變數 `MY_WEBHOOK_ALLOWED_DESTINATIONS`；請參閱[設定參考文件](../reference/configuration.md#webhook_allowed_destinations)）中，以逗號分隔列出主機名稱、IP 位址和 CIDR。

```sh
MY_WEBHOOK_ALLOWED_DESTINATIONS=n8n.internal,10.0.0.0/8
```

- 只有列出的目的地、而且只在 Webhook 傳送時，才不受防護限制。聯邦、媒體和連結預覽都不受影響。不受限制的目的地也可以使用 `http` 和任意連接埠。
- 主機名稱不論解析成哪個位址都會被允許。CIDR 則是在 URL 指定該範圍內的 IP 位址時才會被允許。如果你使用的主機名稱會解析到該範圍內，請把主機名稱也列進去。
- 只有能存取伺服器的營運者可以變更這項設定。光靠管理畫面的權限，無法觸及內部網路。
- 嘗試連到被防護拒絕的目的地時，傳送記錄會顯示 `forbidden_destination`。
- 開發用設定 `http_allow_private_networks` 會在所有用途上解除防護，包括 Webhook。
