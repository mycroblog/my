<!-- source: supporter-payments-with-payjp 1f782a11b125 -->
# 以 PAY.JP 處理支持者付款

使用 `payments = payjp` 時，本站使用者可以在這台伺服器的付款頁面上訂閱月繳或年繳方案，方案的角色會隨著付款狀態的變化授予和移除。部署用的設定鍵說明在[設定參考](../reference/configuration.md#payments)中。整個計畫（方案、角色、成員名單）請參閱 [supporters.md](supporters.md)。管理員會在管理介面中啟用計畫並建立方案。這項設定只是讓伺服器能與 PAY.JP 溝通。

一台伺服器只能使用一個付款服務提供者。要從 Stripe 切換到 PAY.JP（或反過來），請請現有的訂閱者用新的服務提供者重新訂閱。

<!-- source: what-the-server-does-at-payjp a8abbf012e10 -->
## 伺服器在 PAY.JP 上做的事

PAY.JP 有 API v1 和 v2。v2 還沒有定期扣款 API，所以伺服器使用 **API v1** 的定期扣款（Plan 和 Subscription）。v1 既沒有代管的付款頁面，也沒有客戶入口網站，所以由這台伺服器提供頁面來取代它們。

[訂閱的運作方式](supporters.md#how-a-subscription-works)以圖表說明付款頁面、Webhook、對帳、方案的角色和收入如何互相配合。

- **方案**：伺服器會為每個價格建立一個 Plan，其 ID 為 `my_<plan ID>_<month|year>_<amount>`（`metadata.my_plan_id`）。PAY.JP 的 Plan 無法變更金額，所以變更金額時會建立新的 Plan，舊的 Plan 不會刪除，而是加上 `metadata.my_archived=1`。舊 Plan 上的訂閱會以舊金額續約。不要在 PAY.JP 的管理介面中編輯或刪除這些 Plan。唯一的幣別是日圓，金額為 50 到 9,999,999 日圓。
- **訂閱**：伺服器會把使用者的瀏覽器導向它自己的付款頁面（`/payments/pages/payjp/checkout`）。這個頁面會顯示 payjp.js（PAY.JP 的 iframe）的信用卡欄位，並在建立權杖時執行 3-D Secure 驗證（iframe 類型）。伺服器只接受已完成 3-D Secure 的權杖（`three_d_secure_status` 為 `verified` 或 `attempted`），把信用卡以預設卡的身分加到客戶上（第一次時以 `my_<the server's subscription ID>` 為 ID），並建立一個在 `metadata.my_subscription_id` 中帶有伺服器訂閱 ID 的訂閱。第一筆扣款會在建立訂閱時發生。頁面 URL 的效期很短，並帶有伺服器簽署的狀態（自建立起 1 到 2 小時內有效）。使用者離開頁面後重新開始時，無論是同一個方案還是其他方案，都會取得他現在所要方案的新 URL。較早的 URL 在過期之前仍然有效，最先完成付款的頁面會建立唯一的訂閱，其他頁面則會導向這個訂閱。如果較早的頁面已經付款，使用者會被導向返回畫面，不會取得新的 URL。已簽署的 URL 無法撤回，所以即使頁面對應的待處理訂閱已關閉（方案已停用、帳號已刪除），該頁面仍然可以付款。在這種頁面上的付款不會買到任何東西：伺服器會刪除該訂閱並全額退還其扣款（PAY.JP 在銷售後 180 天內接受退款）。API v1 不接受冪等金鑰，所以當頁面的某次送出正在建立訂閱時，伺服器會在資料庫中把該結帳標記為處理中：第二次送出（連點兩下、另一個分頁，或位於同一個負載平衡器後方的另一個伺服器程序）會等待最多 30 秒，然後導向第一次送出所建立的訂閱。如果持有標記的程序已停止，標記會在 5 分鐘後失效。
- **信用卡與付款記錄頁面**（取代 Stripe 的客戶入口網站）：訂閱者可以從他的訂閱畫面開啟 `/payments/pages/payjp/portal`，變更信用卡並查看最近的付款（URL 在一小時後過期）。表單一開始會帶入 PAY.JP 客戶的電子郵件地址，也就是第一次付款時輸入的地址，而不是之後帳號變更的地址。變更信用卡會恢復因付款失敗而停止的訂閱，並立即扣款。取消和變更方案要在這台伺服器的訂閱畫面上進行。
- **付款失敗**：PAY.JP 會暫停（`paused`）續約扣款失敗的訂閱，而且 **不會重試**。伺服器會把暫停視為延遲付款，並從暫停時起保留角色 7 天的寬限期。如果訂閱者在這段期間內變更信用卡，訂閱就會恢復；如果沒有，角色會在 7 天後移除（在 PAY.JP 端，訂閱會維持暫停且不會扣款）。
- **方案變更**：升級時，會把目前週期剩餘部分按比例計算的差額以一次性扣款收取（不足 50 日圓不收取），替換方案，並在週期結束時收取新方案的全額（週期的起點不變）。如果差額的扣款被拒絕，方案不會變更。降級，以及從年繳改為月繳，會透過 `next_cycle_plan` 在週期結束時切換。
- **讀回狀態**：伺服器把 Webhook 視為一個訊號，並從 PAY.JP API 讀回訂閱和金流來決定狀態。此外，每隔 `payments_reconcile_interval` 執行一次的對帳會讀取事件清單、金流，並掃描所有訂閱，修補伺服器漏接的 Webhook。
- **收入**：伺服器會為每筆付款記錄總額、手續費（根據扣款的 `fee_rate` 計算，未滿一日圓的部分捨去）和淨額。退款和退單會把退還的手續費記錄為負的手續費。當一個結算期間結束並發出結算單時，結算單的手續費與伺服器計算結果之間的差額會記錄為一筆手續費變動（其 ID 為結算單的 ID），讓該期間的手續費總額與結算單一致。結算單涵蓋整個帳號的手續費，所以 **請使用專供這台伺服器使用的 PAY.JP 帳號**。
- **日期**：收入的日期以日本時間計算。

伺服器每秒最多傳送 20 個請求（測試模式下為 2 個）。這在 PAY.JP 的速率限制之內（使用祕密金鑰的請求，正式模式每秒 30 個，測試模式每秒 2 個）。遇到 `429` 時，伺服器會等待 1 秒，每次加倍，最多重試 3 次。在測試模式下，單次對帳會很慢（以 1,000 個請求的上限計約 8 分鐘）。後續任務會接手，所以不需要採取任何動作。

<!-- source: card-data-and-pci-dss e7247e5fea0f -->
## 信用卡資料與 PCI DSS

卡號、有效期限和安全碼只會在 payjp.js 顯示的 PAY.JP iframe 中輸入。這台伺服器的頁面和指令碼絕不會接觸到它們。伺服器只會收到 payjp.js 建立的權杖 ID，以及 3-D Secure 需要的持卡人姓名和電子郵件地址。伺服器只會儲存 PAY.JP 的物件 ID（`cus_`、`sub_`、`ch_`、Plan ID、事件 ID）、金額、期間和狀態。它不會儲存卡號、末四碼或有效期限（信用卡頁面只會從 PAY.JP 讀取末四碼、有效期限和客戶的電子郵件地址並加以顯示）。Webhook 的內文和 PAY.JP 的回應既不會保存在日誌中，也不會保存在資料庫中。

付款頁面是從這台伺服器的來源提供的，所以在建置時已考量 PCI DSS v4 對付款頁面指令碼的管理（要求 6.4.3）和竄改偵測（要求 11.6.1）。

- 頁面只會載入 `https://js.pay.jp/v2/pay.js`，以及一個嵌入在頁面中、帶有 nonce 的小型指令碼。它既不包含網頁用戶端的套件，也不包含任何分析或廣告指令碼。
- 頁面一律帶有自己的 Content-Security-Policy（`script-src` 只允許 nonce 和 `https://js.pay.jp`，`frame-src` 只允許 `https://js.pay.jp` 和 `https://api.pay.jp`，`form-action 'self'`，`frame-ancestors 'none'`）、`Referrer-Policy: no-referrer`、`Cache-Control: no-store` 和 `X-Content-Type-Options: nosniff`。沒有任何設定可以變更 CSP。
- 範本和指令碼嵌入在伺服器的二進位檔中，執行期間無法變更。表單的送出只接受來自同一來源的請求。

因此，營運者的適用範圍預期是將付款輸入交由 PAY.JP 的 iframe 處理的商家（SAQ A）。請依照 PAY.JP 的指引和各信用卡品牌的要求，確認適用的範圍和自我評估問卷的類型。確認時，保存以下各點的記錄會有幫助：付款頁面的 URL（`/payments/pages/payjp/checkout` 和 `/portal`）、頁面載入的指令碼清單（上述兩個）、頁面的 CSP，以及連到伺服器的流量使用 TLS（反向代理的設定）。

<!-- source: 1-prepare-the-payjp-account b01b53a61e40 -->
## 1. 準備 PAY.JP 帳號

1. 建立 PAY.JP 帳號，並從測試模式開始。要在正式模式下扣款，需要提出正式環境申請並通過審查。
2. 在 PAY.JP 儀表板中確認「API settings > 3-D Secure」。伺服器每次建立權杖時都會要求 3-D Secure，所以你也可以選擇對所有權杖建立都要求 3-D Secure。
3. 商業交易資訊揭露的頁面在這台伺服器的管理介面中設定（請參閱 [supporters.md](supporters.md)）。

<!-- source: 2-keys-and-the-webhook-token 7bee0ef9256a -->
## 2. 金鑰與 Webhook 權杖

在 PAY.JP 儀表板的「API」下確認以下兩把金鑰。金鑰的前綴決定模式，測試金鑰不會實際扣款。兩把金鑰必須是同一種模式。

| 金鑰 | 範例 | 用途 |
| --- | --- | --- |
| 祕密金鑰 | `sk_test_...` / `sk_live_...` | 伺服器用它呼叫 API。它也是簽署頁面 URL 之金鑰的來源 |
| 公開金鑰 | `pk_test_...` / `pk_live_...` | 付款頁面和信用卡頁面上的 payjp.js |

請以環境變數 `MY_PAYMENTS_PAYJP_SECRET_KEY` 傳遞祕密金鑰（或以 `MY_PAYMENTS_PAYJP_SECRET_KEY_FILE` 傳遞檔案路徑）。它無法寫在設定檔中。如果祕密金鑰外洩，請在儀表板中重新發行，並以新的金鑰重新部署。重新部署之前已開啟的付款頁面和信用卡頁面的 URL 會失效。

<!-- source: 3-register-the-webhook 1afaf50fdaef -->
## 3. 註冊 Webhook

在 PAY.JP 儀表板的「Settings > Webhook」下新增一個目的地。測試模式和正式模式要分別註冊。

- URL：`https://<server domain>/payments/webhooks/payjp`
- 模式：與金鑰相同

PAY.JP 的 Webhook 以帳號的 **來源權杖**（`whook_...`，放在標頭 `X-Payjp-Webhook-Token` 中）識別傳送者。請以環境變數 `MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN`（或 `_FILE`）傳遞儀表板上顯示的權杖。端點會以固定時間比對權杖，並以 `400` 拒絕以正式金鑰收到的測試事件（反之亦然）。由於沒有簽章或時間戳記，任何知道權杖的人都能傳送通知，但伺服器絕不會根據通知的內容寫入狀態；它只會從 PAY.JP 讀回狀態。

伺服器會使用以下事件（其他類型會被接受，但不會記錄）。

- `subscription.created`、`subscription.updated`、`subscription.deleted`、`subscription.paused`、`subscription.resumed`、`subscription.canceled`、`subscription.renewed`
- `charge.succeeded`、`charge.failed`、`charge.refunded`
- `dispute.created`
- `statement.created`

PAY.JP 只會每 3 分鐘重送一次通知，最多 3 次，超過之後的通知在你從儀表板重送之前都不會送達。對帳會修補漏接的部分，所以 **建議把 `payments_reconcile_interval` 設為 1 小時**。服務提供者的狀態（模式、最後一次 Webhook 送達的時間，以及最後一次對帳的結果）會顯示在管理介面 `/admin/supporters` 的「付款服務提供者」區段中。

<!-- source: 4-configure-the-server b0d24c7a2da0 -->
## 4. 設定伺服器

```sh
MY_PAYMENTS=payjp
MY_PAYMENTS_PAYJP_SECRET_KEY=sk_test_...
MY_PAYMENTS_PAYJP_PUBLIC_KEY=pk_test_...
MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN=whook_...
# The recommended reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=1h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` 會驗證設定。以建置標籤 `no_payments_payjp` 建置的二進位檔會拒絕 `payments = payjp`（請參閱 [build-tags.md](../reference/build-tags.md)）。

<!-- source: 5-try-it-in-test-mode f6ebbada0e0e -->
## 5. 在測試模式下試用

請從測試金鑰開始。測試模式和正式模式的 Plan、客戶、訂閱、Webhook 和權杖都是分開的。要上線時，請在正式模式下註冊 Webhook，把金鑰和權杖換成正式用的，然後重新部署。各方案的 Plan 會在正式帳號中重新建立。

測試卡（[PAY.JP 文件](https://pay.jp/docs/testcard)）：

| 卡號 | 行為 |
| --- | --- |
| `4242 4242 4242 4242` | 成功 |
| `4000 0000 0008 0319` | 可以新增信用卡，但扣款會被拒絕（訂閱失敗，以及變更信用卡後恢復失敗） |
| `4000 0000 0000 4012` | 可以新增信用卡，但扣款會因過期而失敗 |
| `4000 0000 0000 0002` | 新增信用卡（建立權杖）會被拒絕 |

請使用未來的有效期限；安全碼和姓名可以任意填寫。在測試模式下，PAY.JP 的 3-D Secure 測試驗證畫面可以讓你選擇結果。

要快速試用續約，請用 API 把訂閱的 `trial_end` 設在幾分鐘之後。續約扣款會在那個時間的幾分鐘之後執行，而不是準時執行。在第一次到期前再次更新 `trial_end`，等待時間會重新計算。要試用付款失敗和寬限期，請先訂閱，在信用卡頁面上把信用卡改成會被拒絕的那張（`4000 0000 0008 0319`），然後讓訂閱續約。

<!-- source: 6-try-it-locally 219997c1a489 -->
## 6. 在本機試用

PAY.JP 的 Webhook 會送到公開的 URL，所以要在你自己電腦上的伺服器接收它們，請透過通道（cloudflared、ngrok 等）公開連接埠，並把該 URL 註冊為測試模式的目的地。payjp-cli 的 `payjp listen --forward-to http://localhost:8080/payments/webhooks/payjp` 預期也能轉送事件，但它是否能轉送 v1 事件尚未驗證。

請在這台伺服器的來源上開啟付款頁面和信用卡頁面（它們在 `http://localhost:8080` 上也能運作，但 payjp.js 和 3-D Secure 的行為會因瀏覽器的設定而異）。

<!-- source: 7-operation fbdd81461f80 -->
## 7. 營運

- 種類為 `permission` 的對帳失敗表示祕密金鑰錯誤。
- 「付款服務提供者」區段的警告中出現 `duplicate_charge`，表示伺服器無法完成已關閉訂閱的付款退款。日誌行 `a second subscription could not be canceled and refunded` 會指出該訂閱，每次對帳都會再試一次，即使訂閱已刪除也一樣。如果情況持續，請在儀表板中退還該筆扣款：只要 PAY.JP 仍然列出已刪除訂閱的扣款（請參閱[待驗證的項目](#items-to-verify)），下一次嘗試就會發現它已退款並清除警告。超過 180 天的扣款無法退款：伺服器會記錄下來並繼續進行。
- 其他退款請在 PAY.JP 儀表板中處理。全額退款和退單會立即移除角色並刪除訂閱，部分退款則不會移除角色。
- **退單的撤銷** 不會自動記錄，因為 PAY.JP 既不會為此發出事件，也沒有對應的 API。發生時，請在成本頁面（`finance`）的收入畫面上記錄撤銷的金額。角色不會自動恢復。
- **PAY.JP 付費方案的費用和匯款手續費** 不會從收入中扣除。請在 `finance` 中把它們記錄為成本。
- 如果對帳經常達到請求上限，請提高 `payments_reconcile_max_requests` 或拉長 `payments_reconcile_interval`。

<!-- source: plan-changes 29dec3be3d95 -->
## 方案變更

升級時會先收取按比例計算的差額，並且只有在扣款成功後才會變更方案。扣款被拒絕時，方案維持不變，使用者會看到一則錯誤訊息，說明如何更新信用卡，不會有寬限期，也不會收到付款失敗的通知，而用有效的信用卡重試只會扣款一次。如果扣款後方案變更失敗，重試不會再次扣款。差額低於 50 日圓的最低金額時，以及改為同一方案的年繳價格（沒有正的差額）時，當下不會扣款，並立即變更；新價格會在週期結束時收取，屆時的失敗即為續約失敗。

<!-- source: items-to-verify 3c2cbf6ed2b0 -->
## 待驗證的項目

以下行為無法從 PAY.JP 的文件或測試模式中確定。

- 退單所退還的手續費如何計算，以及期間的結算單是否與伺服器的手續費一致（測試模式顯示手續費會捨去，而退款的手續費是根據其金額計算）。
- 實際的退單會發生什麼事：PAY.JP 沒有記載在測試模式下引發退單的方法，也沒有記載 `dispute.created` 的內文。從儀表板測試傳送這個事件時，會得到 `200` 的回應，而不會有任何改變。
- 事件清單可以回溯到多久以前。
- 刪除 Plan 是否會影響現有的訂閱（伺服器從不刪除 Plan），以及 Plan ID 的最大長度。
- 載入付款頁面時的 CSP 報告是否有影響：`pay.js` 會套用違反 `style-src 'self'` 的行內樣式，但在 Chromium 中信用卡欄位和 3-D Secure 都能運作。其他瀏覽器尚未確認。
- payjp-cli 是否能轉送 v1 事件。
- 對客戶已儲存的信用卡扣款時，除非請求中要求，否則不會要求 3-D Secure。被拒絕的扣款會以 `paid: false` 列在 `GET /v1/charges` 中，伺服器會忽略未付款的扣款。
- 帶有 `subscription` 的 `GET /v1/charges` 仍會列出已刪除訂閱的扣款。伺服器會在刪除之前退款，所以只有在退款永久失敗後的重試才依賴這一點。API 參考文件記載了這個篩選條件，但沒有提到已刪除的訂閱。如果清單是空的，伺服器會回報失敗而不是退款，而且即使手動退還扣款，`duplicate_charge` 仍會保留。
