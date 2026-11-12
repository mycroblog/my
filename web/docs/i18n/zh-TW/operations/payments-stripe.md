<!-- source: supporter-payments-with-stripe e517db9d077d -->
# 使用 Stripe 的支持者付款

設定 `payments = stripe` 後，本站使用者可以在 Stripe 的代管付款頁面（Checkout）訂閱月繳或年繳方案，而方案的角色會隨付款狀態的變化而授予與移除。管理員啟用後，使用者也可以在同一個頁面送出一次性的[小費](#tips)。
部署設定鍵說明在[設定參考](../reference/configuration.md#payments)中。關於整個計畫（方案、角色、成員清單），請參閱 [supporters.md](supporters.md)。
管理員在管理介面中啟用計畫並建立方案。這項設定只是讓伺服器能夠與 Stripe 通訊。

一台伺服器只能使用一個服務提供者。若之後要改用另一個服務提供者，請請現有的訂閱者改用新的服務提供者重新訂閱。

<!-- source: what-the-server-does-at-stripe 17a592e3f643 -->
## 伺服器在 Stripe 上做的事

[訂閱的運作方式](supporters.md#how-a-subscription-works)以圖表說明付款頁面、Webhook、對帳、方案的角色和收入如何互相配合。

- **方案與價格**：伺服器會透過 API 為每個方案建立一個 Product（ID 為 `my_plan_<plan ID>`，帶有 `metadata.my_plan_id`），並為每個週期與貨幣建立一個 Price（`lookup_key` 為 `my_<plan ID>_<month|year>_<currency>`）。
  Stripe 的 Price 無法變更金額，所以變更金額時會建立新的 Price 並封存舊的。舊 Price 上的訂閱會以舊金額續約。年繳的折扣表現在年繳 Price 的金額中；不使用優惠券。
  不要在儀表板中編輯或刪除這些 Product 和 Price。
  方案可以使用 [Stripe 最低收費金額清單](https://docs.stripe.com/currencies#minimum-and-maximum-charge-amounts)中的貨幣，而且它的月繳與年繳金額都必須達到該最低金額：USD、AED、ARS、AUD、BRL、CAD、CHF、COP、CZK、DKK、EUR、GBP、HKD、HUF、IDR、ILS、INR、JPY、KRW、MXN、MYR、NOK、NZD、PHP、PLN、RON、RUB、SEK、SGD、THB 和 ZAR。伺服器會拒絕任何其他貨幣。
  這份清單適用於以帳號結算貨幣進行的收費。經 Stripe 轉換的收費，必須在轉換後達到結算貨幣的最低金額，而伺服器不會檢查這一點，所以當方案的貨幣與你的銀行帳戶不同時，請把方案的金額設得遠高於最低金額。
- **訂閱**：伺服器會建立 `mode=subscription` 的 Checkout session，並將使用者的瀏覽器導向它的 URL（`checkout.stripe.com`）。付款頁面在一小時後過期。
  伺服器的訂閱 ID 會放入 `client_reference_id`、`metadata.my_subscription_id`，以及 Stripe 訂閱的 `metadata.my_subscription_id`。不啟用自動稅額計算（Stripe Tax）。
- **重新開始**：待處理的訂閱最多只有一個頁面可以收款。針對相同的方案與週期再次開始的使用者，會回到仍開啟的頁面，該頁面會重新以空白開啟，除非它的剩餘時間不到 15 分鐘。任何其他的開始都會得到新的頁面，而伺服器會讓先前的頁面過期，之後它會顯示 Stripe 的過期訊息。如果先前的頁面已經付款，使用者不會得到頁面：他們會前往顯示該訂閱的返回畫面。如果伺服器無法讓先前的頁面過期，這次開始會失敗，使用者可以再試一次。
  關閉的待處理訂閱（它的方案已下架、它的帳號已刪除）會以同樣的方式讓它的頁面過期。
- **第二個訂閱**：當兩個頁面在同一時刻送出時，兩筆付款仍可能相遇。先到達伺服器的訂閱就是使用者的訂閱。伺服器會立即取消另一個，並以 `duplicate` 為原因，將它所有已付款的發票全額退還（包括續約），因為它沒有提供任何期間的服務，並記錄 `a second subscription was canceled and refunded`。已關閉的待處理訂閱的付款，也以同樣方式處理，即使它是在關閉前不久才到達伺服器。使用者會收到 Stripe 的退款，並看到一個訂閱。Stripe 不會退還已退款扣款的手續費，這筆手續費會顯示在收入中。
  處於爭議中的扣款不會退款，伺服器查看時尚未結清的付款（例如轉帳扣款方式）也不會。已經有人在儀表板中取消的訂閱，會連同它的扣款維持原樣，而且絕不會成為使用者的訂閱。
  **從沒有此功能的版本升級伺服器之前**，請先在 Stripe 中尋找有兩個訂閱、且它們的 `metadata.my_subscription_id` 相同的客戶。第一次對帳會取消沒在使用的那一個，並退還它所收的全部款項。你若想手動處理其中的任何一個，請在儀表板中取消它，伺服器就不會動它。
- **客戶入口網站**：第一次使用時，伺服器會透過 API 建立一個入口網站設定（`metadata.my=1`），只啟用更新付款方式、發票歷史，以及在週期結束時取消。方案變更在這台伺服器的畫面中進行，而不是在入口網站。
  在入口網站中取消，在這台伺服器上會顯示為已排定取消，使用者可以從伺服器的畫面復原。在復原之前，方案變更會被拒絕。
- **手動設定的取消日期**：入口網站設定的是訂閱的 `cancel_at`，而不是 `cancel_at_period_end`，伺服器兩者都會讀取。你在儀表板或透過 API 設定的 `cancel_at`，只要不晚於目前週期的結束，就同樣算數。當它早於該結束時，伺服器會把那個日期顯示為訂閱的結束，使用者在那之前保有角色。較晚的 `cancel_at` 還不是取消：訂閱會維持有效，直到包含該日期的週期為止。
- **方案變更**：升級，以及從月繳改為年繳，會立即按比例計算差額開立帳單（`proration_behavior=always_invoice`），而且只有在該發票付款後才切換（`payment_behavior=pending_if_incomplete`）。降級，以及從年繳改為月繳，會透過 Subscription Schedule 在週期結束時切換。
- **方案變更失敗**：當變更的付款失敗，或信用卡要求驗證時，伺服器會作廢該發票（這會捨棄該變更），使用者會看到錯誤。訂閱會維持在它的方案並保持有效，沒有寬限期，也沒有付款失敗的通知。使用者可以在入口網站更新信用卡後再試一次，且只會被收費一次。如果付款仍在處理中，伺服器會保持不動，而方案會在它成功時切換。續約不受影響：續約失敗會如下所述進入寬限期。
- **讀回狀態**：伺服器把 Webhook 視為一個訊號，並從 Stripe API 讀回訂閱、Checkout session 和金流來決定狀態。
  此外，每隔 `payments_reconcile_interval`（預設 6 小時）執行一次的對帳會讀取事件清單、未完成的 Checkout session、balance transaction，並掃描所有訂閱，修補伺服器漏接的 Webhook。
- **收入**：伺服器會為每筆 balance transaction 記錄總額、手續費和淨額。像 Billing 的 0.7% 這類手續費，會以獨立的 `stripe_fee` 交易出現，並從收入中扣除。
  `stripe_fee` 涵蓋整個帳號的手續費，所以**請使用專供這台伺服器使用的 Stripe 帳號**。在也用於其他銷售的帳號上，這些手續費同樣會從收入中扣除。
- **小費**：請參閱[小費](#tips)。
- **日期**：收入的日期以 Stripe 帳號的時區計算（儀表板「Settings > Business > Account details」下的時區，在 API 中是 `settings.dashboard.timezone`）。當無法讀取時，伺服器會以 UTC 計算，並每小時重新讀取一次。

每個伺服器程序每秒最多送出 20 個請求（測試模式為 10 個）。[Stripe 的限制](https://docs.stripe.com/rate-limits)是以帳號為單位：正式模式每秒 100 個請求，sandbox 為 25 個，每個端點每秒 25 個。同一台伺服器的各個程序共用這些限制，使用該帳號的任何其他東西也是。
當 Stripe 回傳 `429` 時，伺服器會依 `Retry-After` 所說的時間等待（若在 30 秒以內；沒有該標頭時為 1 秒），並最多重試 2 次；超過之後，就交給任務去重試。

Stripe 也會依帳號配置讀取請求：30 天內平均每筆交易 500 個，且不論交易筆數，每月至少 10,000 個。在預設的間隔下，小型伺服器的對帳遠低於這個數字。什麼都沒發現的對帳仍然會讀取幾頁，所以最短的 15 分鐘間隔（30 天內 2,880 次執行），在交易很少的帳號上會接近 10,000。
Stripe 限制每個訂閱每分鐘最多開立 10 張新發票、每天 20 張，而立即計費的方案變更會產生一張新發票。

<!-- source: tips 5c0435583f71 -->
## 小費

小費（它們是什麼以及如何啟用，請參閱[小費](supporters.md#tips)）是伺服器為這一次付款所建立的 `mode=payment` Checkout session：

- **頁面**：一個在請求中訂價的品項，以使用者的語言命名為「Tip for <server name>」，金額與貨幣就是使用者選擇的。伺服器不會為它建立任何 Product、Price 或客戶：使用者的電子郵件地址只用來預先填入頁面。頁面只接受信用卡，讓每個完成的頁面都是已付款；它不使用 Adaptive Pricing，所以使用者以你所設定金額的貨幣付款；而它的按鈕寫的是「Pay」，不是「Donate」。頁面與訂閱的頁面一樣，在一小時後過期。
- **我們的 ID**：伺服器的小費 ID 會放入 `client_reference_id`、`metadata.my_tip_id`，以及該筆付款的 PaymentIntent 的 `metadata.my_tip_id`，你可以在儀表板中搜尋它。
- **金額**：小費可以使用方案的貨幣及其最低金額（請參閱[伺服器在 Stripe 上做的事](#what-the-server-does-at-stripe)），最高為 Stripe 最大的八位數收費（999,999.99 美元，或 99,999,999 日圓）。
- **結果**：當它的 `checkout.session.completed` 或 `checkout.session.expired` 事件到達時、當使用者從頁面返回時，以及在對帳中，伺服器會讀回 session。已完成且 `payment_status` 為 `paid` 的 session，就是已付款的小費，並附有它的 PaymentIntent 和它所收取的金額。
- **退款與爭議**：請在儀表板中退還小費。退款、爭議和勝訴的爭議，會透過訂閱所使用的事件和 balance transaction 到達，並指名小費的 PaymentIntent，伺服器據此找到小費。
- **測試**：在測試模式下，請以[在測試模式下試用](#5-try-it-in-test-mode)的測試卡支付小費的頁面；`4000 0000 0000 0259` 會對它造成爭議。

[建立 API 金鑰](#2-create-an-api-key)的受限金鑰，不需要為小費另外加上任何權限。它們尚未在 sandbox 中執行過。

<!-- source: card-data-and-pci-dss 5fd1cd5551e0 -->
## 信用卡資料與 PCI DSS

付款方式只會在 Stripe 的 Checkout 和客戶入口網站上輸入，這台伺服器的網頁用戶端既不載入 Stripe 的指令碼，也不載入 iframe。
伺服器只儲存 Stripe 的物件 ID（`cus_`、`sub_`、`price_`、`prod_`、`cs_`、`pi_`、`txn_`）、金額、期間和狀態。它不儲存卡號、末四碼、有效期限或帳單地址。Webhook 的內容和 Stripe 的回應，不會保存在日誌或資料庫中。
因此，營運者的 PCI DSS 適用範圍預期是使用代管付款頁面的商家（SAQ A）。請在 Stripe 儀表板的指引（「Settings > Compliance」）中，確認適用的範圍和自我評估問卷的類型。

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

<!-- source: 3-register-the-webhook-endpoint bf645f89a942 -->
## 3. 註冊 Webhook 端點

在「Developers > Webhooks > Add endpoint」下，註冊以下內容。伺服器不會透過 API 建立端點。

- URL：`https://<server domain>/payments/webhooks/stripe`
- 事件來源：「Your account」（不是 Connected accounts）
- 內容樣式：snapshot。伺服器只會讀取內容中的 `id`、`type`、`livemode`、`created`、`data.object.id` 和 `data.object.object`，所以端點的 API 版本沒有影響。
- 要傳送的事件（16 種；小費不需要其他類型）：

  - `checkout.session.completed`、`checkout.session.expired`
  - `customer.subscription.created`、`customer.subscription.updated`、`customer.subscription.deleted`、`customer.subscription.paused`、`customer.subscription.resumed`
  - `invoice.paid`、`invoice.payment_failed`
  - `charge.refunded`、`refund.created`、`refund.updated`
  - `charge.dispute.created`、`charge.dispute.closed`、`charge.dispute.funds_withdrawn`、`charge.dispute.funds_reinstated`

請將端點的簽署密鑰（`whsec_...`）放在環境變數 `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET`（或 `_FILE`）中。

端點會驗證簽章（`Stripe-Signature` 中的 HMAC-SHA256）和時間戳記（前後 5 分鐘內），並以 `400` 拒絕以正式金鑰收到的測試事件（反之亦然）。
它會依 ID 將每個事件記錄一次、將任務加入佇列，並回傳 `2xx`。當它無法寫入資料庫時會回傳 `503`，而 Stripe 會再次傳送該事件。內容限制為 1 MiB。
在你輪換簽署密鑰期間，Stripe 會同時以舊的與新的簽署，所以只要將新的密鑰放進設定就夠了。

服務提供者的狀態（模式、上次收到 Webhook 的時間，以及上次對帳的結果與失敗種類）會顯示在管理介面 `/admin/supporters` 的「付款服務提供者」區段，也可以透過管理 API `GET /api/v1/admin/payment_status`（權限 `manage_supporters`）取得。該區段說明了如何修正每一種對帳失敗。
`warnings` 會列出 `no_webhook`（從未記錄過任何 Webhook）和 `reconciliation_corrected`（上次對帳修補了一個訂閱）。當其中任一個出現時，請懷疑 Webhook 設定有誤。
它也會列出 `duplicate_charge`：第二個訂閱尚未被取消與退款。請參閱[營運](#7-operation)。
該區段中的「Reconcile now」（`POST /api/v1/admin/payment_reconciliation/runs`）會立即對帳，而不是等待排程的執行。它限制為每個 UTC 小時一次（同一小時內第二次呼叫會回傳 `429`），而稽核紀錄會記錄 `payment_reconcile_queued`。

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

<!-- source: 6-try-it-locally-stripe-cli 115b10f5518c -->
## 6. 在本機試用（Stripe CLI）

[Stripe CLI](https://docs.stripe.com/stripe-cli)（Apache-2.0）會將 Stripe 事件轉送到你機器上的伺服器。不需要公開的 URL。

```sh
stripe login
stripe listen --forward-to http://localhost:8080/payments/webhooks/stripe \
  --events checkout.session.completed,checkout.session.expired,customer.subscription.created,customer.subscription.updated,customer.subscription.deleted,customer.subscription.paused,customer.subscription.resumed,invoice.paid,invoice.payment_failed,charge.refunded,refund.created,refund.updated,charge.dispute.created,charge.dispute.closed,charge.dispute.funds_withdrawn,charge.dispute.funds_reinstated
```

將 `stripe listen` 印出的 `whsec_...` 放入 `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET`，然後啟動伺服器（請讓埠號與你的 `listen` 設定一致）。

使用 Docker Compose 時，請在 `deploy/docker/.env` 中設定 `MY_PAYMENTS=stripe`、`MY_PAYMENTS_STRIPE_SECRET_KEY` 和 `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET`，然後執行 `docker compose up -d`。

在另一個終端機中，你可以觸發單一事件。

```sh
stripe trigger checkout.session.completed
stripe trigger invoice.payment_failed
stripe trigger charge.refunded
stripe trigger charge.dispute.created
stripe events resend evt_...   # send an event you missed again (up to 30 days)
```

`stripe trigger` 建立的物件不帶有這台伺服器的任何訂閱或小費 ID，所以不會有任何狀態改變（伺服器會讀回它們，並忽略不是它自己的訂閱或小費的東西）。若要檢查完整的訂閱流程或小費，請從網頁用戶端訂閱或給小費，並以測試卡付款。

<!-- source: 7-operation f70cd2967b9f -->
## 7. 營運

- 類型為 `permission` 的對帳失敗，表示金鑰有誤或權限不足。伺服器的日誌行（`payment reconciliation failed`）帶有 Stripe 的訊息，其中會指出需要補上的權限。
- 當對帳經常達到請求上限時，請提高 `payments_reconcile_max_requests` 或拉長 `payments_reconcile_interval`。每次讀取都會計入 Stripe 的讀取額度（請參閱[伺服器在 Stripe 上做的事](#what-the-server-does-at-stripe)），因此在交易很少的帳號上，請優先選擇拉長間隔。
- 當對帳停止超過 30 天時，伺服器無法從 Stripe 的事件列表（只能回溯 30 天）追上進度。這時會由掃描所有訂閱來修復狀態。
- 警告中的 `duplicate_charge` 表示有使用者付了兩次款，而伺服器無法完成第二筆訂閱的取消和退款。日誌行 `a second subscription could not be canceled and refunded` 會指出兩筆訂閱和失敗的類型，並在每次對帳時再次出現，因為對帳會再試一次。若類型是 `permission`，請補上寫入退款的權限。否則請在儀表板中退還該筆收費：下一次嘗試會發現沒有需要退款的款項，並清除警告。「立即對帳」會立即再試一次。
- 其他退款和退單（chargeback）在 Stripe 的儀表板中處理。全額退款和退單會移除角色並立即取消訂閱，部分退款則不會移除角色。贏得爭議並不會自動恢復角色。

<!-- source: items-to-verify 19584d53548f -->
## 待驗證的項目

這些都尚未對 Stripe 試過（不論是 sandbox 或正式環境），而是依據 Stripe 的文件。

- 已封存 Price 上的訂閱續約，它應該會停留在該 Price 上。
- 真實續約時的 `status_transitions.finalized_at`。伺服器把它當作付款第一次失敗的時間，並從它起算寬限期。
- `stripe_fee` 交易是否能指派給某個訂閱。伺服器不會嘗試：它是將這些手續費整體從收入中扣除。
- `GET /v1/account` 的 `settings.dashboard.timezone` 能否以受限金鑰讀取。讀取失敗不會讓伺服器停止，它會以 UTC 計算，所以運作正常的伺服器看不出這一點。請檢查收入的日期是否落在帳號的時區。
- `GET /v1/invoice_payments` 是否能以 `payment[payment_intent]` 篩選，它將付款連結到它的訂閱。
- 全額與部分退款以及退單，從儀表板到伺服器上的狀態。
- 掃描所有訂閱，是否能修補伺服器漏掉狀態的訂閱。
- 對於以待處理更新未列出的付款方式（ACH、SEPA 和 Bacs 直接扣款）付款的訂閱，升級會怎麼處理。在確認這一點之前，請在儀表板中保持這些方式關閉。
- `cancel_at_period_end=true` 是否會取代較晚週期中的 `cancel_at`。
- 回到仍開啟的頁面。伺服器會將 `GET /v1/checkout/sessions/{id}` 回傳的內容（`line_items`、`success_url`、`cancel_url`、`customer`、`customer_email`）與新的請求比較，任何差異都會產生新的頁面。如果 Stripe 在儲存 `success_url` 或 `customer_email` 時改寫了它們，使用者每次都會得到新的頁面並失去已輸入的內容，但只會付款一次。
- 在付款完成的同時讓 session 過期。Stripe 的文件說明，不是開啟狀態的 session 會回傳錯誤，而伺服器接著會讀取該 session。錯誤的確切代碼，以及信用卡驗證進行中的狀態，文件中都沒有說明。
- 同一個 session 無法從兩個分頁付款兩次。一個 session 只有一個訂閱且只會完成一次，但文件並未說明兩次同一時刻送出的情況。
- 付款頁面上驗證失敗，是否會留下一個指名伺服器訂閱的 `incomplete` 訂閱。伺服器會將它連結起來並讓使用者保持待處理，而在另一個頁面上的付款會被當作第二個而退款。
- 第二個訂閱的退款，使用「Charges」和「Refunds」權限如表中所述的受限金鑰，以及隨後的金流。
- 小費從它的頁面到它的退款與爭議，使用受限金鑰。特別是：`allowed_payment_method_types=["card"]` 會讓其他所有方式都不出現在頁面上、關閉 Adaptive Pricing 所建立的 session 會以所要求的貨幣收費，以及 Stripe 對表中的每一種貨幣都接受八位數的一次性收費。
- Stripe 是否接受為營運服務而收的小費，以及對日本的獨資經營者是否接受。在啟用之前，請先詢問 Stripe。
