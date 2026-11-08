<!-- source: supporter-payments-with-stripe f12ff4a0af71 -->
# 使用 Stripe 收取支持者付款

設定 `payments = stripe` 後，本站使用者可以在 Stripe 代管的付款頁面（Checkout）上訂閱月付或年付方案，方案的角色會隨著付款狀態的變化而授予或移除。部署用的設定鍵請參閱[設定參考](../reference/configuration.md#payments)。關於整個支持者計畫（方案、角色、成員列表），請參閱 [supporters.md](supporters.md)。由管理員在管理介面中啟用計畫並建立方案。這項設定只是讓伺服器能與 Stripe 通訊。

一個伺服器只能使用一個付款服務提供者。若之後要改用其他提供者，需要請現有的訂閱者透過新的提供者重新訂閱。

<!-- source: what-the-server-does-at-stripe ca3e0d4f59f3 -->
## 伺服器在 Stripe 上做的事

[訂閱如何運作](supporters.md#how-a-subscription-works)以圖表說明付款頁面、webhook、對帳、方案的角色和收入如何互相配合。

- **方案與價格**：伺服器會透過 API 為每個方案建立一個 Product（ID 為 `my_plan_<plan ID>`，帶有 `metadata.my_plan_id`），並為每種週期和幣別各建立一個 Price（`lookup_key` 為 `my_<plan ID>_<month|year>_<currency>`）。Stripe 的 Price 無法變更金額，因此變更金額時會建立新的 Price 並封存舊的 Price。使用舊 Price 的訂閱會以舊金額續訂。年付的折扣直接反映在年付 Price 的金額上；不使用優惠券。請不要在儀表板中編輯或刪除這些 Product 和 Price。方案可以使用 [Stripe 最低收費金額列表](https://docs.stripe.com/currencies#minimum-and-maximum-charge-amounts)中的幣別，而且月付和年付金額都必須達到該最低金額：USD、AED、ARS、AUD、BRL、CAD、CHF、COP、CZK、DKK、EUR、GBP、HKD、HUF、IDR、ILS、INR、JPY、KRW、MXN、MYR、NOK、NZD、PHP、PLN、RON、RUB、SEK、SGD、THB 和 ZAR。伺服器會拒絕其他幣別。這份列表適用於以帳號結算幣別收費的情況。由 Stripe 換匯的收費，換匯後的金額必須達到結算幣別的最低金額，而伺服器不會檢查這一點，因此當方案的幣別與你銀行帳戶的幣別不同時，請把方案金額訂得遠高於最低金額。
- **訂閱**：伺服器會以 `mode=subscription` 建立 Checkout 工作階段，並把使用者的瀏覽器導向它的 URL（`checkout.stripe.com`）。付款頁面會在一小時後失效。伺服器的訂閱 ID 會放進 `client_reference_id`、`metadata.my_subscription_id`，以及 Stripe 訂閱的 `metadata.my_subscription_id`。不會啟用自動稅額計算（Stripe Tax）。
- **重新開始**：一筆待處理的訂閱最多只有一個頁面可以收款。使用者以相同方案和週期重新開始時，會回到仍然開啟的頁面（頁面會重新以空白狀態開啟），除非它剩下不到 15 分鐘。其他情況的重新開始都會得到新的頁面，伺服器則會讓先前的頁面失效，該頁面隨後會顯示 Stripe 的失效訊息。如果先前的頁面已經付款，使用者就不會得到頁面：他會被帶到返回畫面，畫面上會顯示該訂閱。如果伺服器無法讓先前的頁面失效，這次開始就會失敗，使用者可以再試一次。已關閉的待處理訂閱（方案已停用、帳號已刪除）也會以同樣的方式讓它的頁面失效。
- **第二筆訂閱**：兩個頁面在同一瞬間送出時，仍然可能產生兩筆付款。最先到達伺服器的訂閱就是使用者的訂閱。伺服器會立即取消另一筆，並全額退還它所有已付款的發票（包括續訂），因為它沒有為任何期間提供服務，退款原因為 `duplicate`，並記錄 `a second subscription was canceled and refunded`。已關閉的待處理訂閱收到的付款也會以同樣的方式處理，即使付款是在關閉前一刻到達伺服器的也一樣。使用者會收到 Stripe 的退款，並只看到一筆訂閱。Stripe 不會退還已退款收費的手續費，該手續費會顯示在收入中。處於爭議中的收費不會退款，伺服器查看時尚未結清的付款（例如扣帳類付款方式）也不會。已經有人在儀表板中取消的訂閱會保持原狀，連同它的收費，而且它永遠不會成為使用者的訂閱。**從沒有這項功能的版本升級伺服器之前**，請在 Stripe 中尋找擁有兩筆 `metadata.my_subscription_id` 相同之訂閱的客戶。第一次對帳會取消沒有在使用的那一筆，並退還它收取的所有款項。若有你寧可手動處理的訂閱，請在儀表板中取消它，伺服器就不會碰它。
- **客戶入口**：第一次使用時，伺服器會透過 API 建立一個入口設定（`metadata.my=1`），只啟用更新付款方式、發票紀錄，以及在期間結束時取消。方案變更在這個伺服器的畫面上進行，而不是在入口中。在入口中取消時，這個伺服器上會顯示為預定取消，使用者可以從伺服器的畫面撤銷取消。在撤銷之前，方案變更會被拒絕。
- **手動設定的取消日期**：入口設定的是訂閱的 `cancel_at`，而不是 `cancel_at_period_end`，伺服器兩者都會讀取。你在儀表板或透過 API 設定的 `cancel_at`，只要不晚於目前期間的結束，也會被同樣看待。若早於該結束時間，伺服器會把該日期顯示為訂閱的結束日，使用者會保有角色直到那時。較晚的 `cancel_at` 還不算取消：訂閱會維持有效，直到包含該日期的期間為止。
- **方案變更**：升級以及從月付改為年付，會立即按比例收取差額（`proration_behavior=always_invoice`），並且只在該發票付款後才切換（`payment_behavior=pending_if_incomplete`）。降級以及從年付改為月付，會透過 Subscription Schedule 在期間結束時切換。
- **方案變更失敗**：當變更的付款失敗或信用卡要求驗證時，伺服器會讓該發票作廢，藉此捨棄這次變更，使用者會看到錯誤。訂閱會維持原方案並保持有效，不會進入寬限期，也不會發送付款失敗通知。使用者可以在入口中更新信用卡後再試一次，只會被收費一次。如果付款仍在處理中，伺服器會放著不動，付款成功時方案就會切換。續訂不受影響：續訂失敗會如下文所述進入寬限期。
- **讀回狀態**：伺服器把 webhook 當作訊號，並從 Stripe API 讀回訂閱、Checkout 工作階段和資金異動來決定狀態。此外，每隔 `payments_reconcile_interval`（預設 6 小時）會進行一次對帳，讀取事件列表、尚未完成的 Checkout 工作階段、餘額交易，並掃描所有訂閱，修復伺服器漏收的 webhook。
- **收入**：伺服器會記錄每筆餘額交易的總額、手續費和淨額。像 Billing 的 0.7% 這類費用會以獨立的 `stripe_fee` 交易出現，並從收入中扣除。`stripe_fee` 涵蓋整個帳號的費用，因此**請使用這個伺服器專用的 Stripe 帳號**。如果帳號也用於其他銷售，那些費用也會從收入中扣除。
- **日期**：收入的日期以 Stripe 帳號的時區計算（儀表板中「Settings > Business > Account details」底下的時區，API 中的 `settings.dashboard.timezone`）。無法讀取時，伺服器會以 UTC 計算，並每小時重新讀取一次。

每個伺服器程序每秒最多送出 20 個請求（測試模式為 10 個）。[Stripe 的限制](https://docs.stripe.com/rate-limits)是以帳號為單位：正式模式每秒 100 個請求，沙盒為 25 個，每個端點則是每秒 25 個。同一個伺服器的各程序會共用這些額度，其他使用該帳號的程式也一樣。當 Stripe 回傳 `429` 時，如果 `Retry-After` 指示的時間在 30 秒以內（沒有該標頭時為 1 秒），伺服器會等待該時間並最多重試 2 次；超過的部分則交給任務重試。

Stripe 也會依帳號分配讀取請求的額度：以 30 天計算，平均每筆交易 500 個，且不論交易數量，每月至少 10,000 個。以預設間隔而言，小型伺服器的對帳遠低於這個額度。即使沒有發現任何變化，對帳仍然會讀取幾頁資料，因此在交易很少的帳號上，最短的 15 分鐘間隔（30 天 2,880 次）會接近 10,000 的額度。Stripe 限制每筆訂閱每分鐘最多 10 張、每天最多 20 張新發票，而立即收費的方案變更會產生一張新發票。

<!-- source: card-data-and-pci-dss f45a3c7d1846 -->
## 信用卡資料與 PCI DSS

付款方式只會在 Stripe 的 Checkout 和客戶入口中輸入，這個伺服器的網頁用戶端既不載入 Stripe 的指令碼，也不使用 iframe。伺服器只會儲存 Stripe 物件 ID（`cus_`、`sub_`、`price_`、`prod_`、`cs_`、`txn_`）、金額、期間和狀態。它不會儲存卡號、末四碼、有效期限或帳單地址。webhook 的內容和 Stripe 的回應既不會保存在日誌中，也不會保存在資料庫中。因此，營運者的 PCI DSS 範圍預期會是使用代管付款頁面的商家範圍（SAQ A）。請在 Stripe 儀表板的說明（「Settings > Compliance」）中確認適用的範圍和自我評估問卷的類型。

<!-- source: 1-prepare-the-stripe-account 06eefed5779b -->
## 1. 準備 Stripe 帳號

1. 建立 Stripe 帳號，並在儀表板中輸入你的商家資訊。如果你在日本銷售，請在「Settings > Business > Public information > Notice based on the Act on Specified Commercial Transactions」底下登錄你的特定商業交易法標示頁面的 URL，Checkout 下方就會出現連結。
2. 確認「Settings > Business > Account details」底下的時區。收入的日期會以這個時區計算。
3. 在「Settings > Billing > Subscriptions and emails > Manage failed payments」底下，選擇重試方式（Smart Retries）以及重試用完時的處理方式。移除角色之前的寬限期（從第一次失敗起算 7 天）是由這個伺服器計算的，因此 Stripe 的重試設定不會影響它。重試用完時，建議選擇「Cancel the subscription」。
4. 若要寄送收據給訂閱者，請在「Settings > Customer emails」底下啟用付款成功收據。

<!-- source: 2-create-an-api-key 95e2c858acca -->
## 2. 建立 API 金鑰

建議在「Developers > API keys」底下建立**受限金鑰**（`rk_test_...` / `rk_live_...`）。也可以使用秘密金鑰（`sk_test_...` / `sk_live_...`）。金鑰的前綴決定模式，測試金鑰不會實際收費。無法使用可公開金鑰（`pk_`）。

請為受限金鑰授予以下權限。具有本表權限的受限金鑰已在沙盒中執行過訂閱、方案變更、取消、對帳和發票作廢。退款、讓頁面失效和爭議尚未以它執行過，也沒有人確認過本表是否為能運作的最小權限組合。

| 資源 | 權限 | 使用的 API |
| --- | --- | --- |
| Products | Write | `GET`、`POST /v1/products`、`POST /v1/products/{id}` |
| Prices | Write | `GET`、`POST /v1/prices`、`POST /v1/prices/{id}` |
| Checkout Sessions | Write | `POST /v1/checkout/sessions`、`GET /v1/checkout/sessions/{id}`、`POST /v1/checkout/sessions/{id}/expire` |
| Customer portal | Write | `GET`、`POST /v1/billing_portal/configurations`、`POST /v1/billing_portal/sessions` |
| Subscriptions | Write | `GET /v1/subscriptions`、`POST`、`DELETE /v1/subscriptions/{id}` |
| Subscription Schedules（屬於 Subscriptions） | Write | `POST /v1/subscription_schedules`、`POST /v1/subscription_schedules/{id}`、`POST /v1/subscription_schedules/{id}/release` |
| Invoices | Write | `GET /v1/invoices/{id}`、`GET /v1/invoice_payments`、`POST /v1/invoices/{id}/void` |
| Charges、Refunds | Write | `GET /v1/charges/{id}`、`GET /v1/refunds`、`POST /v1/refunds` |
| Disputes | Read | 透過餘額交易和收費讀取 |
| Balance transaction sources | Read | `GET /v1/balance_transactions` |
| Events | Read | `GET /v1/events` |
| Account（Connect 以外帳號的資訊） | Read | `GET /v1/account`（時區） |

沒有寫入退款的權限時，伺服器仍然會取消第二筆訂閱，而 `duplicate_charge` 會一直留在警告中（請參閱[登錄 webhook 端點](#3-register-the-webhook-endpoint)），直到能夠退款為止。在這項權限列入本表之前建立的金鑰，需要補上這項權限。

請以環境變數 `MY_PAYMENTS_STRIPE_SECRET_KEY` 傳入金鑰（或在 `MY_PAYMENTS_STRIPE_SECRET_KEY_FILE` 中指定檔案路徑）。它不能寫在設定檔中。如果金鑰外洩，請在儀表板中輪替金鑰，並以新的金鑰重新部署。

<!-- source: 3-register-the-webhook-endpoint 1cbc2cf9ace3 -->
## 3. 登錄 webhook 端點

在「Developers > Webhooks > Add endpoint」底下登錄以下內容。伺服器不會透過 API 建立端點。

- URL：`https://<server domain>/payments/webhooks/stripe`
- 事件來源：「Your account」（不是 Connected accounts）
- Payload 樣式：snapshot。伺服器只會讀取內容中的 `id`、`type`、`livemode`、`created`、`data.object.id` 和 `data.object.object`，因此端點的 API 版本不影響運作。
- 要傳送的事件（16 種）：

  - `checkout.session.completed`、`checkout.session.expired`
  - `customer.subscription.created`、`customer.subscription.updated`、`customer.subscription.deleted`、`customer.subscription.paused`、`customer.subscription.resumed`
  - `invoice.paid`、`invoice.payment_failed`
  - `charge.refunded`、`refund.created`、`refund.updated`
  - `charge.dispute.created`、`charge.dispute.closed`、`charge.dispute.funds_withdrawn`、`charge.dispute.funds_reinstated`

請以環境變數 `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET`（或 `_FILE`）傳入端點的簽署密鑰（`whsec_...`）。

端點會驗證簽章（`Stripe-Signature` 中的 HMAC-SHA256）和時間戳記（前後 5 分鐘以內），並以 `400` 拒絕用正式金鑰收到的測試事件（反之亦然）。它會依 ID 記錄每個事件一次，把任務排入佇列，然後回傳 `2xx`。無法寫入資料庫時，它會回傳 `503`，Stripe 就會再次傳送該事件。內容的上限為 1 MiB。輪替簽署密鑰期間，Stripe 會同時以舊的和新的密鑰簽署，因此只要把新的密鑰放進設定即可。

付款服務提供者的狀態（模式、最後一次收到 webhook 的時間，以及最後一次對帳的結果和失敗類型），會顯示在管理介面 `/admin/supporters` 的「金流服務商」區塊中，也可以透過管理 API `GET /api/v1/admin/payment_status`（權限 `manage_supporters`）取得。該區塊會說明如何修正各種對帳失敗。`warnings` 會列出 `no_webhook`（從未記錄過任何 webhook）和 `reconciliation_corrected`（最後一次對帳修復了某筆訂閱）。出現其中任何一項時，請懷疑 webhook 的設定有誤。它也會列出 `duplicate_charge`：第二筆訂閱尚未被取消並退款。請參閱[營運](#7-operation)。該區塊中的「立即對帳」（`POST /api/v1/admin/payment_reconciliation/runs`）會立即對帳，而不必等待排程的執行。它每個 UTC 小時只能執行一次（同一小時內的第二次呼叫會回傳 `429`），稽核日誌會記錄 `payment_reconcile_queued`。

<!-- source: 4-configure-the-server 7b3f7d01839f -->
## 4. 設定伺服器

```sh
MY_PAYMENTS=stripe
MY_PAYMENTS_STRIPE_SECRET_KEY=rk_test_...
MY_PAYMENTS_STRIPE_WEBHOOK_SECRET=whsec_...
# Optional: the reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=6h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` 會驗證設定。以建置標籤 `no_payments_stripe` 建置的執行檔會拒絕 `payments = stripe`（請參閱 [build-tags.md](../reference/build-tags.md)）。

<!-- source: 5-try-it-in-test-mode 94db104964c3 -->
## 5. 在測試模式中試用

請從測試金鑰（`rk_test_` / `sk_test_`，或沙盒金鑰）開始。測試模式不會實際收費。測試金鑰和正式金鑰各自擁有獨立的 Product、Price、客戶、訂閱和 webhook 端點。要切換到正式環境時，請在正式模式中重新登錄端點，把金鑰和簽署密鑰換成正式的，然後重新部署。方案的 Price 會在正式帳號中重新建立。

測試卡（[Stripe 文件](https://docs.stripe.com/testing)）：

| 卡號 | 行為 |
| --- | --- |
| `4242 4242 4242 4242` | 成功 |
| `4000 0000 0000 0341` | 可以綁定，但收費失敗（用來確認付款失敗和寬限期的開始） |
| `4000 0000 0000 9995` | 因餘額不足而失敗 |
| `4000 0000 0000 0259` | 付款後會產生爭議 |

有效期限請使用任何未來的日期，安全碼請使用任意 3 位數字。

若要試用續訂和付款失敗，請使用[測試時鐘](https://docs.stripe.com/billing/testing/test-clocks/api-advanced-usage)。由 Checkout 建立的客戶在建立時沒有時鐘，因此請先訂閱，再透過 API 為該客戶建立時鐘，也就是把 `customer` 傳給 `POST /v1/test_helpers/test_clocks`（API 版本 `2026-05-27.dahlia` 或更新版本）。只有在 `frozen_time` 不是過去的時間、帳號沒有設定 Automations，且客戶最多只有三筆訂閱時，Stripe 才允許這麼做。客戶之後無法脫離時鐘，刪除時鐘會一併刪除客戶及其訂閱。請在儀表板的「Billing > Test clocks」底下推進時間，以觸發續訂收費和失敗。

時鐘無法呈現所有情況：

- **寬限期的結束**：伺服器會比較真實時間與 Stripe 回報的失敗時間（`finalized_at`），而後者是模擬的、比真實時間超前的時間。訂閱會進入寬限期，但繼續推進時鐘並不會讓它結束。角色會保留到真實時間超過模擬失敗時間後 7 天，或者在帳號設定為重試用完時取消訂閱的情況下，保留到 Stripe 取消訂閱為止。
- **掃描所有訂閱**：除非請求指定了時鐘或客戶，否則 Stripe 的訂閱列表不會包含掛在時鐘上的訂閱，而伺服器的掃描兩者都沒有指定。只有 webhook 和事件列表會把它們的變化帶給伺服器。

<!-- source: 6-try-it-locally-stripe-cli 8a3e87b02ce0 -->
## 6. 在本機試用（Stripe CLI）

[Stripe CLI](https://docs.stripe.com/stripe-cli)（Apache-2.0）會把 Stripe 事件轉送到你電腦上的伺服器。不需要公開的 URL。

```sh
stripe login
stripe listen --forward-to http://localhost:8080/payments/webhooks/stripe \
  --events checkout.session.completed,checkout.session.expired,customer.subscription.created,customer.subscription.updated,customer.subscription.deleted,customer.subscription.paused,customer.subscription.resumed,invoice.paid,invoice.payment_failed,charge.refunded,refund.created,refund.updated,charge.dispute.created,charge.dispute.closed,charge.dispute.funds_withdrawn,charge.dispute.funds_reinstated
```

把 `stripe listen` 輸出的 `whsec_...` 放進 `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET`，然後啟動伺服器（連接埠請配合你的 `listen` 設定）。

使用 Docker Compose 時，請在 `deploy/docker/.env` 中設定 `MY_PAYMENTS=stripe`、`MY_PAYMENTS_STRIPE_SECRET_KEY` 和 `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET`，然後執行 `docker compose up -d`。

你可以在另一個終端機中觸發單一事件。

```sh
stripe trigger checkout.session.completed
stripe trigger invoice.payment_failed
stripe trigger charge.refunded
stripe trigger charge.dispute.created
stripe events resend evt_...   # send an event you missed again (up to 30 days)
```

`stripe trigger` 建立的物件不帶有這個伺服器的訂閱 ID，因此不會有任何訂閱改變狀態（伺服器會讀回它們，並忽略不屬於自己訂閱的內容）。若要確認完整的訂閱流程，請從網頁用戶端訂閱，並以測試卡付款。

<!-- source: 7-operation f70cd2967b9f -->
## 7. 營運

- 類型為 `permission` 的對帳失敗，表示金鑰有誤或權限不足。伺服器的日誌行（`payment reconciliation failed`）帶有 Stripe 的訊息，其中會指出需要補上的權限。
- 當對帳經常達到請求上限時，請提高 `payments_reconcile_max_requests` 或拉長 `payments_reconcile_interval`。每次讀取都會計入 Stripe 的讀取額度（請參閱[伺服器在 Stripe 上做的事](#what-the-server-does-at-stripe)），因此在交易很少的帳號上，請優先選擇拉長間隔。
- 當對帳停止超過 30 天時，伺服器無法從 Stripe 的事件列表（只能回溯 30 天）追上進度。這時會由掃描所有訂閱來修復狀態。
- 警告中的 `duplicate_charge` 表示有使用者付了兩次款，而伺服器無法完成第二筆訂閱的取消和退款。日誌行 `a second subscription could not be canceled and refunded` 會指出兩筆訂閱和失敗的類型，並在每次對帳時再次出現，因為對帳會再試一次。若類型是 `permission`，請補上寫入退款的權限。否則請在儀表板中退還該筆收費：下一次嘗試會發現沒有需要退款的款項，並清除警告。「立即對帳」會立即再試一次。
- 其他退款和退單（chargeback）在 Stripe 的儀表板中處理。全額退款和退單會移除角色並立即取消訂閱，部分退款則不會移除角色。贏得爭議並不會自動恢復角色。

<!-- source: items-to-verify 8960333fa514 -->
## 待驗證項目

以下項目尚未在 Stripe 的沙盒或正式環境中實際試過，是依據 Stripe 的文件而來。

- 使用已封存 Price 的訂閱續訂時，應該會維持使用該 Price。
- 實際續訂時的 `status_transitions.finalized_at`。伺服器把它當作付款第一次失敗的時間，並從這個時間開始計算寬限期。
- `stripe_fee` 交易是否能歸屬到某筆訂閱。伺服器不會嘗試這麼做：它會把這些費用從整體收入中扣除。
- 能否以受限金鑰讀取 `GET /v1/account` 的 `settings.dashboard.timezone`。讀取失敗不會讓伺服器停止，它會以 UTC 計算，因此正常運作的伺服器不會顯示這個問題。請確認收入的日期落在帳號的時區中。
- `GET /v1/invoice_payments` 能依 `payment[payment_intent]` 篩選，這是把付款連結到其訂閱的方式。
- 全額與部分退款以及退單，從儀表板操作到伺服器上的狀態。
- 掃描所有訂閱能修復伺服器漏掉狀態的訂閱。
- 對於以待處理更新（pending updates）未列出的付款方式（ACH、SEPA 和 Bacs 直接扣款）付款的訂閱，升級時會發生什麼事。在確認之前，請在儀表板中關閉這些付款方式。
- `cancel_at_period_end=true` 會取代設在之後期間的 `cancel_at`。
- 回到仍然開啟的頁面。伺服器會比較 `GET /v1/checkout/sessions/{id}` 回傳的內容（`line_items`、`success_url`、`cancel_url`、`customer`、`customer_email`）與新的請求，只要有任何差異就會建立新的頁面。如果 Stripe 在儲存 `success_url` 或 `customer_email` 時改寫了它們，使用者每次都會得到新的頁面並失去已輸入的內容，但只會付款一次。
- 在付款完成的同時讓工作階段失效。Stripe 的文件說，不處於開啟狀態的工作階段會回傳錯誤，伺服器接著會讀取該工作階段。錯誤的確切代碼，以及信用卡驗證進行中時的狀態，文件中都沒有說明。
- 同一個工作階段無法從兩個分頁付款兩次。一個工作階段只包含一筆訂閱且只會完成一次，但文件沒有說明在同一瞬間送出兩次的情況。
- 付款頁面上的驗證失敗，是否會留下一筆指向伺服器訂閱的 `incomplete` 訂閱。伺服器會連結它並讓使用者維持在待處理狀態，而在另一個頁面上的付款則會被當作第二筆訂閱退款。
- 以「Charges」和「Refunds」權限如本表所示的受限金鑰退還第二筆訂閱，以及隨後的資金異動。
