# ADR-0074: 決済: PaymentProvider・Stripe・PAY.JP

- Status: Accepted
- Date: 2026-10-22
- Deciders: @rosylilly
- Issue: #913, #909, #925, #1301, #2612

## Decision（決定）

### 1. `PaymentProvider` ポート

ポートは `internal/app/supporters`（ADR-0072）に置く。型は標準ライブラリだけで、SDK の型を出さない（ADR-0068 の `BillingSource` と同じ方針）。
ポートは「事業者の側の事実」だけを返し、こちらの加入の状態・猶予・ロールは `supporters` が導く。1 つのサーバーで有効な adapter は 1 つである。

```go
type PaymentProvider interface {
    // Name is the provider's identifier: "stripe", "payjp".
    Name() string
    // Location is the time zone of the provider's reports; MoneyMovement.OccurredOn uses it (ADR-0067 §13).
    Location() *time.Location
    // Mode reports whether the credentials are for test (no real charges) or live payments.
    Mode() Mode // test, live
    // MinimumAmount is the smallest recurring charge the provider accepts in currency, in minor units.
    MinimumAmount(currency string) (int64, bool)

    // EnsurePlan creates or reuses the product of a plan and one recurring price per entry of
    // spec.Prices, archives the plan's prices that spec no longer lists, and returns the refs.
    // Idempotent. Archived prices keep renewing the subscriptions already on them.
    EnsurePlan(ctx context.Context, spec PlanSpec) (PlanRefs, error)

    // StartCheckout creates a hosted checkout and returns the URL to redirect the browser to.
    StartCheckout(ctx context.Context, req CheckoutRequest) (Checkout, error)
    // GetCheckout reports whether a checkout completed and, if so, what it created.
    GetCheckout(ctx context.Context, ref CheckoutRef) (CheckoutResult, error)
    // ResumeCheckout reads a checkout and, while it is open and was made for what req asks, returns
    // its page; nil for a provider that keeps none.
    ResumeCheckout(ctx context.Context, ref CheckoutRef, req CheckoutRequest) (CheckoutResult, *Checkout, error)
    // ExpireCheckout makes an open checkout unpayable and reports what it is now: expired, or
    // completed when the customer got there first. A provider that cannot withdraw a page reports it open.
    ExpireCheckout(ctx context.Context, ref CheckoutRef) (CheckoutResult, error)
    // PortalURL returns a short-lived URL of the provider's customer portal.
    PortalURL(ctx context.Context, customer CustomerRef, returnURL string) (string, error)

    // Commands our own UI and jobs issue; each is safe to repeat.
    ChangePrice(ctx context.Context, sub SubscriptionRef, to PriceRef, when ChangeTiming, key string) error
    DropUnpaidChange(ctx context.Context, sub SubscriptionRef, key string) error
    CancelScheduledChange(ctx context.Context, sub SubscriptionRef, key string) error
    CancelAtPeriodEnd(ctx context.Context, sub SubscriptionRef, key string) error
    ResumeCancellation(ctx context.Context, sub SubscriptionRef, key string) error
    CancelNow(ctx context.Context, sub SubscriptionRef, key string) error
    // RefundSubscription refunds what is left of every settled charge of a subscription (ADR-0072 §6).
    RefundSubscription(ctx context.Context, sub SubscriptionRef, key string) error

    // ParseWebhook verifies a delivery and returns what it points at. It does no I/O.
    ParseWebhook(header http.Header, body []byte, now time.Time) (Notice, error)
    // Resolve turns a notice into the objects to re-read (subscriptions, a checkout, money).
    Resolve(ctx context.Context, n Notice) (Change, error)

    // GetSubscription re-reads the provider's current state of one subscription.
    GetSubscription(ctx context.Context, sub SubscriptionRef) (SubscriptionSnapshot, error)
    // ListSubscriptions pages through every subscription of the account, 100 at a time.
    ListSubscriptions(ctx context.Context, cursor string) (SubscriptionPage, error)
    // ListChanges pages through the notices of changes since a time, oldest first; it fails with
    // ErrChangesWindow when since is older than the provider keeps them.
    ListChanges(ctx context.Context, since time.Time, cursor string) (ChangePage, error)
    // ListMoney pages through money movements booked at or after since, oldest first.
    ListMoney(ctx context.Context, since time.Time, cursor string) (MoneyPage, error)
    // GetMoney re-reads one money movement (a refund's original payment, for example).
    GetMoney(ctx context.Context, id string) (MoneyMovement, error)
}

type PlanSpec struct {
    PlanID      string      // our plan id; the adapter stores it in the product's metadata
    Name        string      // shown on the hosted checkout and the receipts
    Description string
    Prices      []PriceSpec // the prices to keep active
}

type PriceSpec struct {
    Interval    Interval // month, year
    AmountMinor int64    // ADR-0067 §6: minor units of Currency
    Currency    string   // ISO 4217
}

type CheckoutRequest struct {
    LocalID    string      // our subscription id (ADR-0072); stored on the checkout and the subscription
    Price      PriceRef
    Customer   CustomerRef // zero: the provider creates a customer
    Email      string      // prefills the checkout when Customer is zero
    Locale     string      // BCP 47 of the user's UI language
    SuccessURL string      // our origin; the provider appends its checkout id
    CancelURL  string
    Key        string      // idempotency key
}

type Checkout struct {
    Ref         CheckoutRef
    RedirectURL string    // https on a host the adapter knows (§7)
    ExpiresAt   time.Time
}

type CheckoutResult struct {
    Status       CheckoutStatus // open, completed, expired
    LocalID      string
    Customer     CustomerRef
    Subscription SubscriptionRef
}

type SubscriptionSnapshot struct {
    Ref                 SubscriptionRef
    Customer            CustomerRef
    LocalID             string             // from the subscription's metadata; "" when not ours
    Price               PriceRef
    Status              SubscriptionStatus // incomplete, incomplete_expired, active, past_due, unpaid, paused, canceled
    UnpaidChange        string             // what a failed change at once left at the provider; "" without one
    PeriodStart         time.Time
    PeriodEnd           time.Time
    CancelAtPeriodEnd   bool
    ScheduledPrice      PriceRef  // zero unless a change waits for PeriodEnd
    LatestPaymentFailed time.Time // zero unless the latest invoice's payment failed and is unpaid
}

type MoneyMovement struct {
    ID           string    // provider id, unique per provider (Stripe: balance transaction id; PAY.JP: §13)
    Kind         MoneyKind // payment, refund, chargeback, chargeback_reversal, fee
    ParentID     string    // refund, chargeback, reversal: ID of the payment; "" otherwise
    ParentGross  string    // the payment's gross, for telling a full refund from a partial one
    Subscription SubscriptionRef // zero for fees not tied to one subscription
    OccurredAt   time.Time
    OccurredOn   Date      // OccurredAt in Location
    Covers       DateRange // payment: the billing period it pays for; zero otherwise
    Gross, Fee, Net string // decimal in major units; Net = Gross - Fee; negative for money out
    Currency     string
    Final        bool      // the provider will not change the amounts any more
}
```

- `Notice` は `EventID`・`Type`（事業者の種類の名前）・`Kind`（`subscription`・`checkout`・`money`・`ignored`）・`ObjectID` を持つ。`Change` は取り直す `[]SubscriptionRef`、`CheckoutRef`、`[]MoneyMovement` を持つ。
  `SubscriptionPage` は `[]SubscriptionSnapshot` と次のカーソル（空は最後）、`ChangePage` は `[]Notice` と次のカーソルと読み終えた時刻を持つ。一覧の 1 ページは 1 要求で、照合が要求を数える（§4）。
- `ChangeTiming` は `now_prorated`（差額を日割りですぐ請求し、支払いを確かめてから切り替える）と `period_end`（期間の終わりに切り替える）。どちらを使うかは ADR-0072 が決める（上げる変更はすぐ日割りで請求し、下げる変更と年払いから月払いへの変更は期間の終わりに切り替える）。adapter は事業者の機能に写す（§2・§10）。
  `now_prorated` の `ChangePrice` は、支払いが断られると `ErrPaymentDeclined`（種類は `invalid_request`）、決済がまだ確定しないと `ErrPaymentProcessing` を返し、どちらもプランを変えない。`ErrPaymentDeclined` のときは、期間の終わりを待つ変更を元のまま残し、回収できるものを事業者に残さない。
  同じ `key` でも、支払いの失敗の後の呼び出しは新しい試みで、失敗を再生しない。請求は 1 回だけである。
  `SubscriptionSnapshot.UnpaidChange` は、失敗した変更の片付けが途中で止まったときに事業者に残るものの名前で、プランと状態を変えない。`DropUnpaidChange` がそれを除く（なければ、または決済が確定していなければ何もしない）。
  `CancelScheduledChange` は、期間の終わりを待つ変更を取り下げる。`ScheduledPrice` が空なら何もしない。
- 状態の名前は Stripe の語を事業者に依存しない意味で使う。`trialing` は試用期間を作らないので `active` に写し、知らない状態は `Status` を `unpaid`（ロールを付けない側）にして警告のログを出す。
- `MoneyMovement` は ADR-0067 §11 の `IncomeMovement` に、`supporters` が `Subscription` をこちらの加入の ID に写して渡す。`ParentGross` は `supporters` が全額の返金と一部の返金を見分けるために使う（ADR-0072）。
  `ListMoney`・`GetMoney`・`Resolve` が返すのは、事業者の側で額が確定した動き（`Final = true`）だけで、まだ確定しない動きは返さず、次の照合で取り直す（ADR-0067 §11）。`Final` は ADR-0072 の判断が確かめるために残す。
- 失敗は `supporters` が定義する種類の値で返す: `permission`（鍵の誤り・権限の不足）、`not_found`、`invalid_request`（`StartCheckout` の通貨の不一致など）、`throttled`（429）、`unavailable`（5xx・ネットワーク）、`invalid_signature`・`stale_signature`・`mode_mismatch`（`ParseWebhook`）。
- **契約テスト**: `pkg/testkit.PaymentProviderContract(t, factory)` を置く。`factory` は、契約が渡す事業者の側の筋書き（プランと価格、申し込み、完了と期限切れ、更新、支払いの失敗と成功、解約の予約と取り消し、即時の解約、期間の終わりの変更、全額と一部の返金、チャージバックとその取り消し、独立した手数料、ページの数、権限の不足、レート制限）を受け、それを返す fake の上で動く `PaymentProvider` と、そのテスト用の webhook の署名器を作る。
  契約は次を確かめる。
  - `EnsurePlan` を繰り返しても価格が増えず、金額を変えると新しい価格と古い価格のアーカイブになり、古い価格のサブスクリプションが残ること。
  - `StartCheckout` の URL が §7 の許可したホストの `https` で、`GetCheckout` が `LocalID` を返すこと。
  - 各コマンドを 2 回呼んでも結果が同じこと。
  - `ParseWebhook` が、正しい署名・改ざんした本文・許容の外の時刻・複数の署名・テストとライブの食い違い・知らない種類を見分け、I/O をしないこと。
  - スナップショットの状態の写し。
  - `MoneyMovement` の符号、`Net = Gross − Fee`、`ParentID` と `ParentGross`、`Covers`、`OccurredOn` の日付の基準（ADR-0067 §11 の契約テストの項目）。
  - ページ送りの漏れと重複がないこと、失敗の種類の分類、`ctx` の取り消し。

  `fake` adapter（`internal/adapter/payment/fake`）と `stripe` adapter（Stripe の REST を `httptest` で返す）がこれを通す。`payjp` adapter は §9 の違いを除いて同じ契約を通す。実アカウントは使わない。

### 2. `stripe` adapter（`internal/adapter/payment/stripe`）

- `github.com/stripe/stripe-go/v87`（MIT）をこの adapter の中だけで使う。SDK の型は adapter の外に出さず、組み立ては `internal/platform`（`cmd/` から）が行う。API の版は SDK が固定する版を使う。
- **プラン**: Product の `metadata.my_plan_id` で引き、なければ作る。Price は `lookup_key` を `my_<plan_id>_<interval>_<currency>` にし、金額が違えば新しい Price を `transfer_lookup_key=true` で作って古い Price を `active=false` にする（Stripe の Price は金額を変えられない）。
  年の Price の金額は ADR-0072 が割引率から決めて渡す（クーポンを使わない）。アーカイブした Price のサブスクリプションは、その Price のまま更新され続ける（ADR-0072 の価格の改定。**要検証**）。
- **最低額**: `MinimumAmount` は Stripe の最低の請求額の表（JPY 50、USD 0.50 など）を adapter に持つ。
- **Checkout**: `mode=subscription`、`ui_mode` はホスト型、`line_items` に Price を 1 つ、`client_reference_id` と `metadata.my_subscription_id` と `subscription_data.metadata.my_subscription_id` に `LocalID`、`customer` か `customer_email`、`locale`、`success_url`（`?checkout={CHECKOUT_SESSION_ID}` を付ける）、`cancel_url`、`expires_at` は 1 時間後。
  税（Stripe Tax、`automatic_tax`）は使わない。運営者が使うかは Stripe のダッシュボードの設定の範囲で、この ADR は扱わない。
- **顧客ポータル**: 起動時に設定（`/v1/billing_portal/configurations`）を `metadata.my=1` で引き、なければ作る。
  有効にする機能は、支払い方法の更新、請求書の履歴、期間の終わりでの解約（`subscription_cancel.mode=at_period_end`）だけで、`subscription_update` は無効にする。`PortalURL` はこの設定と `return_url` でセッションを作る。
- **プランの変更**:
  - `now_prorated`: サブスクリプションの品目の Price を差し替え、`proration_behavior=always_invoice` で差額をすぐ請求し、`payment_behavior=pending_if_incomplete` で**支払いが済んだときだけ**新しい Price を適用する。月から年への変更は Stripe が請求日を変更の日に移し、その場で年額を請求する。
    - **結論**: 支払いが済めば切り替わる。済まなければ、その更新の請求書を void して保留の更新を捨て、`ErrPaymentDeclined` を返す。決済が `processing` のときだけ void せず `ErrPaymentProcessing` を返す。
    - **文書にあること**（Stripe の pending updates と API リファレンス、2026-11-04 に確認）: 失敗すると返す Subscription に `pending_update` があり、変更は適用されない。支払いが成功すると適用される。保留は最長 23 時間で、期限で請求書が void され更新が捨てられる。取り消すには `latest_invoice` を void する。新しい更新は前の保留を置き換え、その請求書を void する。`items`・`metadata`・`proration_behavior` は対応の属性である。
      `pending_if_incomplete` に対応する支払い方法はカードなどで、ACH・SEPA・Bacs は含まれない。Stripe は 500 の応答も冪等キーに保存して再生し、キーは 24 時間以上たつと消えることがある。
    - **文書にないので仮定すること**（§2 の要検証。adapter はどちらでも壊れないように書く）: 断られたときの HTTP の状態（200 と `pending_update` でも 402 でも扱う）。認証が要るときも `pending_update` で返ること。失敗の間の Subscription の状態（`active` のはず。`pending_update` があれば `past_due` でも `active` に写し、支払いの失敗の時刻も報告しない。その間の請求書は更新の請求書で、更新の請求はないため）。void が Smart Retries を止めること（void の後は開いた請求書がないので再試行されないはず）。`processing` の請求書の void の扱い。
    - **冪等**: 更新の `Idempotency-Key` は、呼び出し側の `key` に、最新の請求書の ID と試みの分を足す。失敗の後の試みは新しい請求書の後なので失敗を再生せず、保存された 500 は次の分に消える。同時の 2 つの送信は同じキーを共有する。どの試みも先に読むので、成功の後は更新を送らず二重に請求しない。
    - **途中の失敗**: void に失敗すると、次の取り直し（webhook か照合）が `UnpaidChange` を見て、ジョブ `supporters.command` の `drop_unpaid_change` が void する。そのため restricted key に Invoices の書き込みが要る。
    - **待っている変更**: Schedule の段階の移行は保留の更新を捨てるので、更新の前に解放する。支払いが断られたら同じ変更を Schedule に作り直す。
  - `period_end`: Subscription Schedule を作り（既存のサブスクリプションから）、今の期間の終わりから新しい Price の段階を足す。`CancelScheduledChange` は Schedule を解放（release）する。
- **画面を続ける・失効させる**（ADR-0072 §6）: `ResumeCheckout` は `GET /v1/checkout/sessions/{id}`（`expand[]=line_items`）で読む。`url` はセッションが開いている間だけある（文書の記載）。`line_items` の価格・`success_url`・`cancel_url`・顧客かメールが今の依頼と同じときだけ、その `url` を返す。Stripe が値を書き換えて保存するなら一致せず、新しい画面になるだけで、支払いは増えない（**要検証**）。
  `ExpireCheckout` は `POST /v1/checkout/sessions/{id}/expire`。開いていないセッションには誤りを返す（文書の記載）ので、400 ならセッションを読み直し、完了か失効ならそれを答え、まだ開いているなら誤りのままにする。誤りのコードは文書にないので見ない。
- **返金**: `RefundSubscription` は、`GET /v1/invoices`（`subscription`、`status=paid`）と `GET /v1/invoice_payments` で課金を引き、`POST /v1/refunds`（`charge`、`reason=duplicate`、額は付けない）を送る。額を付けない返金は残りの全額で、返金済みの課金には `charge_already_refunded` を返す（文書の記載）ので、完了として扱い、繰り返しても二重にならない。失敗を再生しないよう、冪等キーは試みごとに変える。dispute 中の課金（`charge_disputed`・`refund_disputed_payment`）は dispute に任せて飛ばす。1 つの課金の失敗で残りを止めず、最後に誤りを返す。制限付きの鍵には Refunds の書き込みの権限が要る。
- **解約**: `CancelAtPeriodEnd` と `ResumeCancellation` は `cancel_at_period_end` を立てる・下ろす。`CancelNow` は `DELETE /v1/subscriptions/{id}`（`prorate=false`、`invoice_now=false`）で、もう終わっていれば何もしない。
  全額の返金とチャージバックのときに `supporters` が呼ぶ（ADR-0072）。一部の返金ではロールを外さず、dispute に勝ってもロールは自動で戻さない。
- **冪等**: 作成と変更の POST には `Idempotency-Key` に呼び出し側の `key`（ジョブの冪等キーと操作の名前）を付ける。Stripe はキーを 24 時間で消すことがあるので、各コマンドは呼ぶ前に今の状態を読み、もう望む状態なら呼ばない（24 時間を越えたジョブの再試行でも二重にならない）。
- **お金の動き**: `GET /v1/balance_transactions`（`created[gte]`、`starting_after`）を読み、`reporting_category` で種類に写す: `charge` → `payment`、`refund`・`refund_failure` → `refund`、`dispute` で負 → `chargeback`、`dispute_reversal` で正 → `chargeback_reversal`、`fee`（`type=stripe_fee`）→ `fee`。ほかは返さない。
  サブスクリプションと期間は `source` の charge → PaymentIntent → `invoice_payments` → invoice → `parent.subscription_details.subscription` と invoice の品目の期間から引く（API の版 basil 以降の形）。
  金額は Stripe の最小単位の整数を ISO 4217 の小数の桁で 10 進の文字列にする（Stripe が ISO 4217 と違う桁で扱う通貨は Stripe の桁に従う）。balance transaction は作られた後に額が変わらないので `Final = true`。
- **レポートのタイムゾーン**: `Location()` は、起動時と照合のたびに Account の `settings.dashboard.timezone`（Stripe のアカウントのタイムゾーン）を読んだ値で、読めなければ UTC を返す（ADR-0067 §13。読めるかは **要検証**）。UTC にしたときは管理画面の事業者の状態に示す。

### 3. webhook の受け口

- 経路は `POST /payments/webhooks/{provider}`（例 `/payments/webhooks/stripe`）。新しい `internal/interface/payments` に置き、OpenAPI の client API には載せない（事業者が呼ぶ経路で、利用者は呼ばない）。課金が無効か、`{provider}` が設定の事業者でなければ `404`。
- 受け口がすること（1 つの要求の中で、外部の I/O をしない）:
  1. 本文を 1 MiB まで読む（超えたら `413`）。
  2. `ParseWebhook` で署名を検証する。`stripe` adapter は標準ライブラリ（`crypto/hmac`・`crypto/sha256`・`crypto/subtle`）で `"<t>.<生の本文>"` の HMAC を計算し、ヘッダーのどれかの `v1` と定数時間で比べ、`t` が今から ±5 分の中にあることを確かめる。
     秘密を切り替える間は Stripe が新旧の秘密で署名を並べるので、設定の秘密は 1 つでよい。
  3. 本文は `encoding/json` で `id`・`type`・`livemode`・`data.object.id`・`data.object.object` だけを読む。API の版で形が変わる項目は読まないので、SDK の版を上げても、エンドポイントの API の版が SDK と違っても受け付ける。
     `livemode` が鍵のモード（§5）と違えば `mode_mismatch` で拒む（テストのイベントで本番のロールが付かないように）。
  4. 1 つの短いトランザクションで、受信の記録（§4 の `payment_webhook_receipts`、`(provider, event_id)` で一意）を足し、同じトランザクションで outbox（ADR-0004）にジョブ `supporters.apply_payment_event` を積む。冪等キーは `payment_event:<provider>:<event_id>`、`orderingKey` は対象のオブジェクトの ID。
     受信の記録がもうあれば何もしない（重複の受信）。
  5. `2xx` を返す。署名の誤り・古い時刻・モードの食い違いは `400`、DB の失敗は `503`（Stripe が再送する）。`ignored` の種類も記録せずに `2xx` を返す。
- 運営者が Stripe に登録するイベント: `checkout.session.completed`・`checkout.session.expired`、`customer.subscription.created`・`.updated`・`.deleted`・`.paused`・`.resumed`、`invoice.paid`・`invoice.payment_failed`、`charge.refunded`・`refund.created`・`refund.updated`、`charge.dispute.created`・`.closed`・`.funds_withdrawn`・`.funds_reinstated`。
  docs に一覧と、Stripe CLI・ダッシュボードでの登録の手順を書く。サーバーはエンドポイントを API で作らない（秘密 `whsec_` を運営者が設定に置くため）。
- **ジョブ `supporters.apply_payment_event`**（冪等、ADR-0004）:
  1. `Resolve` で、通知が指すサブスクリプション・申し込み・お金の動きを事業者から取り直す（イベントの本文の状態は使わない）。
  2. 申し込みなら `GetCheckout` で完了を確かめ、顧客とサブスクリプションの ID を加入に結び付ける。
  3. サブスクリプションごとに `GetSubscription` を呼び、ADR-0072 の `supporters.sync`（スナップショットからこちらの状態を導き、ロールを付け外す）に渡す。
  4. お金の動きは、返金・チャージバックの元の支払いがまだ記録になければ `GetMoney` で先に取り、`finance.RecordIncome`（ADR-0067）と `supporters`（取り消しの判断、ADR-0072）に渡す。
  - 事業者の API はトランザクションの外で呼び、結果を短いトランザクションで書く（ADR-0003）。`throttled`・`unavailable` は ADR-0004 の指数バックオフで再試行し、`permission` は記録して成功で終える（照合と管理画面の状態に出る）。
  - 同じサブスクリプションのジョブは `orderingKey` で直列にし、取り直してすぐ適用するので、後に適用したものが新しい状態になる。重複・順序の入れ替わり・取りこぼしは、どれも「取り直して適用する」の 1 つの扱いになる。

### 4. 照合のジョブとデータモデル

- `Scheduler`（ADR-0004）の定期ジョブ `supporters.reconcile_payments` を、課金が有効なときだけ載せる。周期は設定 `payments_reconcile_interval`（既定 6 時間、15 分〜7 日）で、冪等キーは既存の `schedule:<name>:<slot>`。
  既定の 6 時間なら、webhook を落としても 6 時間で追いつき、7 日の猶予の判定には十分に短い。
- 1 回の照合が事業者に送る要求は、設定 `payments_reconcile_max_requests`（既定 1,000）までにする。adapter は要求を 1 秒に 20 件まで（テストのモードでは 10 件まで）に抑え、`429` には `Retry-After` に従って待つ。
  Stripe の読み取りのレート制限（ライブで 1 秒に 100 件ほど、テストで 25 件ほど）の内に収めるための値である。既定の上限と速さなら、1 回の照合は 1 分ほどで終わる。
- 照合は次の 4 段を順に行い、上限に達した段でカーソルを記録して止まる。続きは、続きのジョブ（冪等キー `continue:<run_id>`、1 分後）が同じ上限で引き継ぐ。どの段も一覧の API でまとめて読み、1 件ずつの取り直しは変わったものに限る。
  1. **変わったもの**: `ListChanges` で、前回の成功の照合が読み終えた時刻の 5 分前からのイベントを 100 件ずつ読む（Stripe は `GET /v1/events`、§3 の種類だけ）。
     イベントが指すサブスクリプションを重ねずに集め、それぞれ `GetSubscription` で取り直して `supporters.sync` に渡す。webhook を落としたものはここで直る。
  2. **未完了の申し込み**: 期限の切れていない申し込みごとに `GetCheckout`（期限は 1 時間なので、件数は少ない）。
  3. **お金の動き**: `ListMoney` を前回のカーソルから 100 件ずつ読む（重なりは `(provider, provider_id)` の一意で吸収する、ADR-0067 §12）。`stripe_fee` もここで入り、収入から差し引く。balance transaction の一覧には遡る期間の制限がない。
  4. **全件の見回し**: 残りの上限で、`ListSubscriptions` で事業者の側のすべてのサブスクリプションを、前回の見回しのカーソルから 100 件ずつ読み、こちらの `LocalID` を持つものを `supporters.sync` に渡す。
     最後のページを読んだらカーソルを先頭に戻す。1〜3 で拾えない食い違い（イベントの漏れ、30 日を越えて照合が止まったとき）も、見回しの 1 周で直る。
- 要求の数の見積もり（既定の 6 時間・上限 1,000、1 ページ 100 件）:

| 加入者の数 | 1 の変わったもの | 2 と 3 | 4 の見回し | 1 回の要求 | 見回しの 1 周 |
| --- | --- | --- | --- | --- | --- |
| 300 | 1〜2 ページと数件の取り直し | 数件 | 3 ページ | 10 件ほど | 毎回 |
| 10,000 | 5 ページほどと 100〜150 件の取り直し（6 時間に更新を迎えるのは 1 万 ÷ 30 日 ÷ 4 ≒ 85 件） | 数件 | 100 ページ | 260 件ほど | 毎回 |
| 100,000 | 40 ページほどと 1,000 件ほどの取り直し | 10 ページほど | 残りの上限 | 1,000 件（上限）と続きのジョブ 2〜3 回 | 続きのジョブを合わせて数分 |

- 1 回の要求の上限は事業者への負荷をならすためのもので、続きのジョブが残りを引き継ぐ。運営者は、上限に当たる回が多ければ `payments_reconcile_max_requests` を上げるか周期を延ばす。webhook が届いていれば状態はそれで直るので、照合が遅れても加入者に見える影響は取りこぼしの直りが遅れることだけである。
- `ListChanges` の起点が Stripe の events の 30 日の窓より古いとき（照合が長く止まっていたとき）、adapter は `ErrChangesWindow` を返し、照合は 1 を飛ばして 4 の見回しに任せる。
- 管理画面の「今すぐ照合する」は、同じジョブを冪等キー `manual:<provider>:<UTC の時>` で積む（1 時間に 1 回まで）。
- データモデル（ADR-0020）。`supporters` が所有する。加入・顧客の結び付き・価格の対応の表は ADR-0072 が決める。

| テーブル | 分類 | 内容 |
| --- | --- | --- |
| `payment_webhook_receipts` | イベント | `id`, `provider`（`enum(stripe, payjp)`）, `event_id`（`string(255)`）, `event_type`（`string(100)`）, `object_id`（`string(255)`）, `received_at`。一意 `(provider, event_id)`。本文は持たない（§5） |
| `payment_reconcile_runs` | イベント | `id`, `provider`, `run_key`, `outcome`（`enum(succeeded, partial, failed)`）, `error_kind`（§1 の種類と `none`・`request_cap`）, `changes_until`（`timestamp`）, `changes_cursor`・`money_cursor`・`sweep_cursor`（`string(255)`、空は先頭）, `requests`・`checked_subscriptions`・`corrected_subscriptions`（`int`）, `occurred_at`。一意 `(provider, run_key)` |

- 次の照合は、最新の `succeeded` か `partial`（上限で止まった）の行のカーソルから始める。カーソルは試行ごとの行に残し、上書きする現在値の列を持たない（ADR-0020 の規約 4）。

- `payment_webhook_receipts` は 45 日を過ぎた行を定期ジョブで消す（Stripe の手での再送は CLI で 30 日まで遡れる）。消した後に同じイベントが来ても、ジョブが取り直して適用するだけで状態は変わらない。
- 管理画面 `/admin/supporters`（ADR-0072）に事業者の状態を出す: 事業者とモード（テスト・ライブ）、最後に webhook を受けた時刻、最後の照合の結果と失敗の種類（`client*` のメッセージで直し方を案内する）。
  webhook を一度も受けていないか、照合が直した加入があれば、webhook の設定の誤りとして示す。API は `GET /admin/payment_status` と `POST /admin/payment_reconciliation/runs`。手動の照合は監査ログ（ADR-0058）に残す。

### 5. カード情報と秘密

- **カード情報はこのサーバーを通らない**: Stripe では支払いの入力をホスト型の Checkout と顧客ポータルだけで行い、こちらの Web クライアントは Stripe のスクリプトも iframe も読み込まない（PAY.JP は §11）。
  サーバーが持つのは Stripe のオブジェクトの ID（`cus_`・`sub_`・`price_`・`prod_`・`cs_`・`txn_`）と、金額・期間・状態だけで、カードの番号・下 4 桁・有効期限・請求先の住所を保存しない。
  webhook の本文と事業者の応答はログにも DB にも残さない（受信の記録は ID と種類だけ）。運営者の PCI の範囲は SAQ A 相当になる（運営者が確かめる事項として docs に書く）。
- **設定**（ADR-0010）: デプロイの設定（鍵と秘密は環境変数）に置き、`scripts/task.sh go:config-docs` の表に載せる。インスタンス設定（DB）には置かない。課金を有効にすることとプランは、管理者がインスタンス設定で行う（ADR-0072）。

| キー | 秘密 | 内容 |
| --- | --- | --- |
| `payments` | | `none`（既定）・`stripe`・`payjp`。1 つだけ |
| `payments_stripe_secret_key` | ✓ | API の鍵。権限を絞った制限付きの鍵（`rk_`）を勧め、必要な権限を docs に書く。`sk_` も受ける |
| `payments_stripe_webhook_secret` | ✓ | webhook のエンドポイントの署名の秘密（`whsec_`） |
| `payments_reconcile_interval` | | 照合の周期（既定 `6h`、`15m`〜`168h`、§4） |
| `payments_reconcile_max_requests` | | 1 回の照合が事業者に送る要求の上限（既定 1,000、§4） |

- 秘密は環境変数か `<NAME>_FILE` から読み、`slog.LogValuer` で伏せる（ADR-0019 §6）。
- **モード**: 鍵の接頭辞（`sk_test_`・`rk_test_` と `sk_live_`・`rk_live_`）で決め、`Mode()` が返す。`my config check` と管理画面に示し、テストのモードでは加入の画面に「テストのモード（課金されません）」を出す。webhook はモードの違うイベントを拒む（§3）。
- 事業者を切り替えるときは、既存の加入者に再加入してもらう。docs に書く。

### 6. テストのしかた

1. **契約テスト**（§1）: fake と `stripe` の両方。`stripe` は `httptest` のサーバーで Stripe の REST の JSON を返し、SDK の `BackendConfig` の URL をそこに向ける（テストだけ）。fixture は Stripe の文書の例の形に合わせる。
2. **`supporters` のテスト**: fake の上で、同じ webhook を何度も受ける・順序を入れ替える・webhook を落として照合だけで追いつく・署名の誤りを拒む、の筋書きで最後の状態とロールを確かめる。
3. **ローカルでの試し方**（docs）: Stripe のサンドボックスの鍵を設定し、`stripe listen --forward-to http://localhost:<port>/payments/webhooks/stripe` が出す `whsec_` を `payments_stripe_webhook_secret` に置く。
   `stripe trigger` で単発のイベントを、test clock で更新・支払いの失敗・猶予を進め、テスト用のカード（`4242…` 成功、`4000000000000341` 後で失敗、`4000000000000259` dispute）で決済する。
4. **テストモードでの通しの確認**（決定者）: 加入・ロールの付与・プランの変更（上げる・下げる・月と年）・支払いの失敗と猶予・解約・全額と一部の返金・dispute・収入の記録。

### 7. 外向きの HTTP と CSP

- Stripe の API は、運営者が鍵を設定した事業者の固定のエンドポイント（`api.stripe.com`）なので、AWS・Google Cloud の SDK のクライアントと同じく SDK の HTTP クライアントを使い、`kernel.HTTPClient` を通さない（AGENTS.md の Design rules）。
  宛先は SDK が決め、設定で変えられない（テストの `httptest` だけが変える）。`internal/platform` がタイムアウト（全体 30 秒）と計測の付いた `*http.Client` を `stripe.BackendConfig.HTTPClient` に渡す。
- サーバーは Checkout と顧客ポータルの URL を取りに行かない。ブラウザがそこへ移るだけである。adapter は返す前に、その URL が `https` で、ホストが `checkout.stripe.com` か `billing.stripe.com` であることを確かめる（応答の改ざんで別のサイトへ送らないため）。
- **CSP は変えない**: Web クライアントは client API で URL を受け、`location.assign` でトップレベルを移す。トップレベルの移動は CSP（`form-action`・`connect-src`・`frame-src`）の対象外で、Stripe のスクリプトも iframe も読まないので、ADR-0019 §9 の既定のポリシーと `Permissions-Policy: payment=()` のままで動く。
  戻り先の `success_url` と `cancel_url` はこちらのオリジンの画面（ADR-0072）。モバイルアプリには加入の導線を出さない（FR-911）。

### 8. build tag

- `no_payments_stripe` は `internal/adapter/payment/stripe` と `stripe-go` を外し、`payments = stripe` を拒む（[build-tags.md](../reference/build-tags.md) の「Adding a tag」の手順）。`fake` は test だけで使い、本番の binary に入らない。
- 代表の組み合わせのうち、self-hosted の組に `no_payments_stripe` を足す（AWS と Google Cloud の組は含めたまま build を確かめる）。

### 9. ポートと契約テストに足すもの

PAY.JP でも §1 のポートの形をそのまま使い、足すのは次の 2 つだけである。

- **ページの受け口**: `internal/app/supporters` に任意のインターフェースを足す。adapter が実装していれば、`internal/interface/payments` が `GET`・`POST /payments/pages/{provider}/...` をその `http.Handler` に渡す。
  `{provider}` が設定の事業者でないか、実装していなければ `404`。経路は webhook と同じく OpenAPI の client API に載せず、ADR-0019 §5 のレート制限の対象にする。

  ```go
  // PageServer is a provider whose checkout and portal pages our server serves, for a provider
  // without hosted ones (§11). StartCheckout and PortalURL return URLs of these pages.
  type PageServer interface {
      // Pages handles the requests under PagesPath(provider).
      Pages() http.Handler
  }
  ```

- **契約テスト**: `PaymentUnderTest` に `TimedSignature bool` を足す。false の adapter（`payjp`）では、`ParseWebhook` の時刻の許容（`stale_signature`）と複数の署名・秘密の切り替えの項目を飛ばし、代わりに「トークンの誤り・欠け・テストとライブの食い違いを見分ける」ことを確かめる。
  決済の画面と顧客ポータルの URL の許可するホスト（`CheckoutHosts`）には、`SuccessURL`・`returnURL` のオリジンを渡す（§11）。ほかの契約の項目はそのまま通す。

### 10. `payjp` adapter（`internal/adapter/payment/payjp`）

- **API の版と呼び出し**: API v1 の定期課金 API（Plan・Subscription）を使う。`net/http` で `https://api.pay.jp/v1` を呼ぶ（Basic 認証のユーザー名に秘密鍵、本文は `application/x-www-form-urlencoded`、応答は JSON）。使うのは十数の API で、`payjp-go` の `beta` ブランチを追わない。宛先は固定で、設定で変えられない（テストの `httptest` だけが変える）。
  外向きの HTTP は §7 と同じく、`internal/platform` が渡すタイムアウトと計測の付いた `*http.Client` を使う。
- `Name()` は `payjp`。`Location()` は日本時間（`time.FixedZone`、UTC+9。日本に夏時間はない）で、PAY.JP の入金日などの日付の基準に合わせる（ADR-0067 §13）。`MinimumAmount("JPY")` は 50 で、ほかの通貨は false。
- **プラン**: Plan の ID を `my_<plan_id>_<interval>_<amount>` に決めて作る（`currency=jpy`、`metadata.my_plan_id`、`trial_days` と `billing_day` は使わない）。同じ ID があれば作らず、名前が違えば `name` を更新する。金額を変えると ID が変わるので新しい Plan になる。
  spec にない同じプランの Plan は、削除せずに `metadata.my_archived=1` を付けてアーカイブとする（削除が既存の定期課金に効くかは **要検証**、効かなくても削除しない）。`ProductRef` は Plan をまとめる名前 `my_<plan_id>` で、PAY.JP のオブジェクトはない。9,999,999 を越える額と JPY 以外は `invalid_request`。
- **スナップショット**（`GetSubscription`・`ListSubscriptions`）: `Price` は `plan.id`、`ScheduledPrice` は `next_cycle_plan.id`、期間は `current_period_start`・`current_period_end`、`LocalID` は `metadata.my_subscription_id`。状態は次のとおり写す。

  | PAY.JP | ポートの状態 | ほかの項目 |
  | --- | --- | --- |
  | `active`・`trial` | `active` | 試用は作らない。`trial` は §10 の上げる変更で期間を保つときにだけ現れる |
  | `canceled` | `active` | `CancelAtPeriodEnd = true`（PAY.JP の `canceled` は期間の終わりの解約の予約） |
  | `paused` | `past_due` | `LatestPaymentFailed = paused_at`。こちらは停止の命令を出さないので、停止は支払いの失敗による（ダッシュボードで止めたときも同じに扱う） |
  | 取得が `404` | `canceled` | 期間の終わった解約と削除は取得できなくなる。こちらが記録した ID だけを取り直すので、`404` は終わったサブスクリプションである |
  | 知らない値 | `unpaid` | 警告のログ（§1） |

  PAY.JP は支払いに失敗した定期課金を再試行しない（[定期課金のガイド](https://docs.pay.jp/v1/subscription)、2026-10-25 に確認）。ADR-0072 §7 の猶予（7 日）の起点は `paused_at` で、その間に利用者が §11 のポータルでカードを直すと adapter が `/resume` で課金して `active` に戻る。直さなければ 7 日で `lapsed` になり、PAY.JP の側は停止のまま課金しない。
- **命令**（どれも呼ぶ前に定期課金を読み、もう望む状態なら呼ばない。v1 に冪等キーの記載がないので、これで二重の操作を防ぐ）:
  - `ChangePrice(now_prorated)`: 差額 = 切り捨て((新しい価格の 1 秒あたり − 今の価格の 1 秒あたり) × 今の期間の残りの秒数)。1 秒あたりは価格をその間隔の 1 期間（今の期間の始まりから 1 か月か 1 年）の秒数で割る。
    差額が 50 円以上なら、顧客に都度の課金（`POST /v1/charges`、`customer`・`amount`・`metadata` に `my_subscription_id`・`my_subscription`（PAY.JP の定期課金の ID）・`my_change`（呼び出し側の `key`）・`my_period_end`）をする。課金の前に、その顧客の直近 2 日の charge に同じ `my_change` があるかを確かめる。
    続けて定期課金の `plan` を新しいプランにし、`trial_end` を今の期間の終わりにする（即時の課金をさせず、次の課金は期間の終わりに新しいプランの全額）。差額の課金が断られたら、プランを変えずに §1 の `ErrPaymentDeclined`（種類は `invalid_request`）を返す（利用者に支払い方法を直すよう示す）。
    月払いから年払いへの同じ順位の変更は、年払いの 1 秒あたりが安いので差額がなく、年払いは今の期間の終わりから始まる。
  - `ChangePrice(period_end)`: `next_cycle_plan` を新しいプランにする。`CancelScheduledChange` は `next_cycle_plan` を空にする。
  - `CancelAtPeriodEnd`: `POST /v1/subscriptions/:id/cancel`。`ResumeCancellation`: 状態が `canceled` で期間の終わりの前なら `POST /v1/subscriptions/:id/resume`（期間の内なので課金しない）。
  - `CancelNow`: `DELETE /v1/subscriptions/:id`（`prorate=false`）。`404` は済んだものとする。
  - `ResumeCheckout`: 入力を持たず、`StartCheckout` が今の依頼の URL に署名するので、続ける画面はない（`GetCheckout` の結果だけを返し、`Checkout` は `nil`）。`ExpireCheckout`: 署名した URL は取り消せないので、開いている画面は開いたままで、`GetCheckout` の結果を返す。画面からはこちらの ID を載せた定期課金が 1 つだけできる。
  - `DropUnpaidChange`: 課金の後でだけプランを動かすので、失敗した変更は何も残さない。何もしない。
  - `RefundSubscription`: `GET /v1/charges`（`subscription`）で課金を引き、支払い済みで未返金のものに `POST /v1/charges/:id/refund` を送る。売上から 180 日を過ぎた課金は PAY.JP が返金しないので、警告のログを残して飛ばす。課金が 1 つもなければ `ErrInvalidPaymentResponse` を返す。
- **失敗の種類**（§1）: `401` → `permission`、`404` → `not_found`、`400`・`402` → `invalid_request`、`429` → `throttled`、`5xx` とネットワーク → `unavailable`。
- **要求の速さ**: ライブで 1 秒 20 件、テストで 1 秒 2 件まで（PAY.JP の秘密鍵の制限 30 件・2 件の内）。`429` は指数バックオフで待つ（`Retry-After` の記載はない）。

### 11. 決済の画面と顧客ポータルの代わり

v1 にホスト型の画面がないので、adapter が Stripe のホスト型の画面の役をする短命のページを、このサーバーのオリジンの `/payments/pages/payjp/` に出す（§9 の `PageServer`）。ページは ADR-0090 のサーバー描画の画面（`authpages`）と同じく `html/template` で作り、Web クライアント（SPA）の外に置く。

- **署名した状態**: `StartCheckout` と `PortalURL` は、ページに要る状態（こちらの加入の ID・Plan・顧客・メール・ロケール・戻り先・有効期限）を JSON にし、秘密鍵から導いた鍵（HMAC-SHA256）で署名して URL に載せる。サーバーは DB に何も持たない。
  ページの URL のオリジンは `SuccessURL`（ポータルは `returnURL`）のオリジンで、adapter はそれ以外へ送らない（§7 の URL の確かめに当たる）。URL は Stripe のセッションの URL と同じく短命の持参人の鍵で、`Referrer-Policy: no-referrer` と `Cache-Control: no-store` を付ける。
  有効期限は作成の時刻を 1 時間に切り捨てて 2 時間を足した時刻で、同じ `Key` は同じ時間帯に同じ申し込みを返す。
- **申し込み**（`StartCheckout` → ページ → `SuccessURL`）:
  1. `GET` のページは、プランの名前・金額・間隔、テストのモードの表示（§5）、メール（主アドレスを入れておく）とカードの名義の欄、payjp.js の card element（PAY.JP の iframe）を出す。ページの小さなスクリプトが payjp.js でトークンを作り、フォームで送る。
     スクリプトは `createToken` に `three_d_secure: true` と名義・メールを渡し、iframe 型の 3D セキュアを経たトークンを受ける。
  2. `POST` は、トークンを取り直して（`GET /v1/tokens/:id`）`card.three_d_secure_status` が `verified` か `attempted` であることを確かめる。顧客を用意し（`CheckoutRequest.Customer` が空なら ID `my_<こちらの加入の ID>` の顧客を作る。あればそれを使う）、トークンのカードを顧客の既定のカードとして足す。
     続けて、`metadata.my_subscription_id` にこちらの加入の ID を入れた定期課金を作る（作る前に顧客の定期課金を読み、もうあれば作らない）。作成は最初の課金を伴う。
     成功すれば `SuccessURL` に `checkout=<CheckoutRef>` を付けて `303` で送る。3D セキュアの失敗・課金の失敗は、理由をページに出し、足したカードを外して入力からやり直させる。`CancelURL` へ戻るリンクを常に出す。
  - `CheckoutRef` は `<顧客>/<こちらの加入の ID>/<有効期限>`。`GetCheckout` は顧客の定期課金を読み、`metadata.my_subscription_id` が一致するものがあれば `completed`（その顧客と定期課金）、なく有効期限を過ぎていれば `expired`、ほかは `open` を返す。
- **ポータル**（`PortalURL`）: 既定のカードのブランド・下 4 桁・有効期限（表示するだけで保存しない）、定期課金の状態（停止していれば支払いに失敗したことと、カードを更新すると再開すること）、直近の支払いの履歴（`GET /v1/charges?customer=`、日付・金額・返金）、カードの変更を出す。
  カードの変更は申し込みと同じ 1・2 の手順（定期課金は作らない）で、成功したら新しいカードを既定にして古いカードを消し、停止している定期課金を `/resume` で再開する（その場で課金する）。解約はポータルに出さない（解約はこのサーバーの画面と API で行う、ADR-0072 §8）。
- **CSP**: ページにだけ次の CSP を付け、Web クライアントの CSP（ADR-0019 §9）は変えない。正確なオリジンは **要検証**（実装で payjp.js の文書に合わせる）。

  ```text
  default-src 'none'; script-src 'nonce-<n>' https://js.pay.jp; style-src 'nonce-<n>'; frame-src https://js.pay.jp https://api.pay.jp;
  connect-src https://api.pay.jp; img-src 'self' data:; form-action 'self'; frame-ancestors 'none'; base-uri 'none'
  ```

- **偽造の防止**: `POST` は `net/http.CrossOriginProtection` を通し、署名した状態をフォームにも載せる（ADR-0019 §4）。ページはカード情報を受けず、受けるのは payjp.js が作ったトークンの ID だけである。
- **i18n**: 文言はサーバーのメッセージ（`server.payments.*`、`locales/en` と `locales/ja`）で、ロケールは状態のロケールを使う（ADR-0009）。
- **PCI**: カードの番号は PAY.JP の iframe の中だけで入力され、このサーバーと Web クライアントのスクリプトは触れない。運営者の範囲は SAQ A 相当で、運営者が確かめる事項として docs に書く（§5 と同じ）。
  決済のページがこのサーバーのオリジンに載るので、PCI DSS v4 の決済のページのスクリプトの管理（要件 6.4.3）と改ざんの検知（要件 11.6.1）に沿って、次を守る。
  - ページが読むスクリプトは、payjp.js と、テンプレートに埋めた nonce 付きの自前の小さなスクリプトだけにする。外部のスクリプトは CSP の `script-src` で `https://js.pay.jp` だけを許す。Web クライアントの bundle・解析・広告のスクリプトを載せない。
  - ページのテンプレートとスクリプトは adapter のソースに `embed` で入れ、実行時に変えられない。CSP と `X-Content-Type-Options: nosniff`・`frame-ancestors 'none'` は常に付け、ページのテストで確かめる。
  - カードの番号・有効期限・CVC を受ける項目を、フォームにもサーバーにも持たない。サーバーが受けるのはトークンの ID・名義・メールだけで、ログに残すのは PAY.JP のオブジェクトの ID だけにする。
  - 運営者の docs に、SAQ A の範囲の確かめ方（このページと PAY.JP の iframe の分担、CSP、TLS）と、ページの CSP を変える設定を持たないことを書く。

### 12. webhook

- 経路は §3 の `POST /payments/webhooks/payjp`。`ParseWebhook` は `X-Payjp-Webhook-Token` を設定のトークンと定数時間で比べ、欠けか違いは `invalid_signature`。PAY.JP の通知には時刻の署名がないので `stale_signature` を返さない。本文の `livemode` が鍵のモードと違えば `mode_mismatch`。
- 本文は `id`・`type`・`livemode`・`created`・`data.id`・`data.object`・`data.subscription`・`data.metadata.my_subscription` だけを読む。通知を受けても状態は書かず、ジョブが取り直す（§3）ので、トークンを知る者が古い通知を再び送っても状態は変わらない。
- 種類の写し: `subscription.*`（`created`・`updated`・`deleted`・`paused`・`resumed`・`canceled`・`renewed`）と `charge.failed` は `subscription`、`charge.succeeded`・`charge.refunded`・`dispute.created`・`statement.created` は `money`、ほかは `ignored`。運営者はこの種類を PAY.JP の管理画面で登録する（docs）。
- PAY.JP の再送が短い（3 分おきに最大 3 回）ので、取りこぼしは照合（§4）が拾う。docs で、PAY.JP では `payments_reconcile_interval` を 1 時間にすることを勧める。

### 13. お金の動き

- **支払い**: 確定した charge（`captured`）のうち、`subscription` を持つものと `metadata.my_subscription` を持つもの（§10 の差額）だけを返す。ID は charge の ID、`Gross` は `amount`、`Fee` は `amount × fee_rate ÷ 100` の端数を切り捨てた額（端数の扱いは **要検証**）、`OccurredAt` は `captured_at`。
  `Covers` は、定期課金の今の期間の始まりが charge の作成と 10 分以内ならその期間、ほかは作成の日から定期課金のプランの間隔の 1 期間、差額の課金は作成の日から `metadata.my_period_end` の前の日まで（日本時間の日付）。
- **返金**: `charge.refunded` のイベントごとに 1 つの動きにする。ID はイベントの ID、`ParentID` は charge、`ParentGross` は charge の `amount`、`Gross` は −（そのイベントの `amount_refunded` − 同じ charge の 1 つ前の `charge.refunded` のイベントの `amount_refunded`）。PAY.JP は返金で手数料を返す（明細の `refund_fee_offset`）ので、`Fee` は返金の額の手数料の負の額とする（返る額の計算は **要検証**）。
- **チャージバック**: `dispute.created` のイベントから、charge と額を読んで `chargeback` にする（イベントの本文の形は **要検証**）。PAY.JP にはチャージバックの取り消しを知らせるイベントと API がないので、`chargeback_reversal` は返さない。運営者が `finance` の画面で記録する（docs）。
- **手数料の補正**: `statement.created`（`type=sales`）ごとに、明細の `fee`・`refund_fee_offset`・`chargeback_fee_offset` の合計と、その区間（`term`）の charge から上の式で計算した手数料の合計を比べ、差があれば `fee` の動き（ID `<statement の ID>:fee`、サブスクリプションなし）を 1 つ返す。これで区間ごとの手数料の合計が PAY.JP の明細と一致する。
  PAY.JP の有料プランの利用料（`service_fee`）と振込の手数料（`transfer_fee`）は収入から引かず、運営者が費用（FR-901）として記録する。
- どの動きも額が後から変わらないので `Final = true`。`ListMoney` は Events API を時刻の窓（1 時間）に区切って `offset` で読み、上の種類のイベントから動きを作る（カーソルは窓と `offset`）。PAY.JP の一覧は新しい順なので窓の中の順は新しい順になるが、照合は `(provider, provider_id)` の一意で重なりを吸収する（ADR-0067 §12）。
  `GetMoney` は ID の形（`ch_`・イベント・`:fee`）で読み分ける。

### 14. 照合

- §4 の 4 段をそのまま使う。1 の変わったもの（`ListChanges`）は Events API の §12 の種類を同じ窓で読み、2 の未完了の申し込みは `GetCheckout`、3 は §13、4 の見回しは `GET /v1/subscriptions`（`limit=100`、見回しを始めた時刻を `until` に固定して `offset` を進める。カーソルは `until` と `offset`）。
- 見回しの 1 ページで停止の時刻まで分かるので、支払いの失敗のために追加の要求は要らない。期間の終わった定期課金は一覧から消えるが、こちらが続いていると思う加入は Scheduler の期限（ADR-0072 §7）が取り直し、`404` が `canceled` になる。
- Events API の保持期間が分かるまで `ErrChangesWindow` は返さない。保持期間が照合の周期より短いと分かったら、「この決定を覆す条件」に従う。
- テストのモードの 1 秒 2 件では、1 回の照合の上限（既定 1,000 要求）に 8 分ほどかかる。続きのジョブ（§4）がそのまま引き継ぐ。

### 15. 設定と秘密

§5 の表に足す（デプロイの設定、秘密は環境変数か `_FILE`、`scripts/task.sh go:config-docs` の表に載せる）。

| キー | 秘密 | 内容 |
| --- | --- | --- |
| `payments_payjp_secret_key` | ✓ | 秘密鍵（`sk_test_`・`sk_live_`） |
| `payments_payjp_public_key` | | 公開鍵（`pk_test_`・`pk_live_`）。payjp.js と 3D セキュアの開始に使う。秘密鍵とモードが違えば設定の検査で拒む |
| `payments_payjp_webhook_token` | ✓ | webhook のトークン（`whook_`）。モードごとに違う |

- モードは秘密鍵の接頭辞で決める。§11 の状態の署名の鍵は秘密鍵から導く（秘密鍵を替えると、発行済みのページの URL は使えなくなる）。

### 16. build tag とデータモデル

- `no_payments_payjp` は `internal/adapter/payment/payjp` を外し、`payments = payjp` を拒む（[build-tags.md](../reference/build-tags.md)）。依存を増やさないので、代表の組み合わせはどれも含めたまま build を確かめる。
- `schema/12-finance/<dialect>.sql`（`income_movements`）と `13-supporters.sql` の `provider` の値は `payjp` を含む。表は増えない。FR-910 の決済の事業者は Stripe か PAY.JP である。

### 17. テストのしかた

1. **契約テスト**（§9）: `payjp` adapter を、PAY.JP v1 の REST の JSON を返す `httptest` のサーバーの上で通す。fixture はリファレンスの例の形に合わせる。
2. **ページのテスト**: 申し込みとカードの変更を、`httptest` の PAY.JP の上で、署名の誤り・期限切れ・3D セキュアの失敗・課金の失敗・二度目の戻り（定期課金を二重に作らない）・CSP のヘッダーで確かめる。
3. **ローカルでの試し方**（docs）: テストの鍵を設定し、webhook は公開の URL（トンネル）か payjp-cli の `payjp listen --forward-to http://localhost:<port>/payments/webhooks/payjp`（v1 のイベントを転送できるかは **要検証**）で受ける。
   テストのカード（`4242424242424242` など）で申し込み、更新は定期課金の `trial_end` を数分後にして早める（リファレンスがテストの用途として案内している）。
4. **テストモードでの通しの確認**（決定者）: §6 の 4 の筋書き（加入・ロール・プランの変更・支払いの失敗と猶予とカードの変更での再開・解約・全額と一部の返金・チャージバック・収入の記録と手数料の補正）。

### 18. 1 回の支払い（チップ、ADR-0162）

- チップ（FR-918）の 1 回の支払いは、§1 の加入のメソッドの外で受ける。足すメソッドと型（任意のインターフェース、`MoneyMovement` の支払いの参照）は ADR-0162 §9 が決める。
- **Stripe**: `mode=payment` の Checkout Session（ホスト型）で受ける。`submit_type=donate` は使わない。
- **PAY.JP**: 1 回の支払いは API v2 で受ける。Checkout Session（`mode=payment`）、支払いの `payment_flow_id`、返金の `POST /v2/payment_refunds`、dispute の `/v2/payment_disputes`、作成の要求の `Idempotency-Key` を使う。
  加入は §10 のとおり v1 の定期課金 API のままで、同じ秘密鍵で v1 と v2 を併用する（PAY.JP は移行の期間の併用を認めている。[API v2 への移行](https://docs.pay.jp/v2/guide/v2-migration)、2026-11-09 に確認）。§11 の自前のページは 1 回の支払いに使わない。

### この決定を覆す条件

- 再デプロイなしに管理画面から鍵を設定したいという要望が多ければ、秘密を `Sealer` で暗号化して DB に持つ案を別の ADR で検討する。
- ほかの事業者が要るときは、同じ契約テストを通す adapter を足し、`payments` の値と各表の `provider` の enum に足す。
- PAY.JP が v2 の定期課金 API を出したら、v2 の adapter への移行を別の ADR で検討する（v1 と v2 は顧客の ID を共有するが、v2 で作った支払い方法は v1 で使えない）。v1 の終了が告知されたときも同じ。
- Events API の保持期間が短く照合で追いつけないと分かったら、PAY.JP の `ListMoney` の支払いは `GET /v1/charges` から読み、返金は charge の `amount_refunded` の差（イベントの ID の代わりに累計の額を ID に含める）に変える。
- 取引明細の手数料と charge ごとの計算が区間をまたいでずれる（明細の区間と `term_id` が一致しない）と分かったら、取引明細の区間ごとの額だけを記録する形に変える。

## 経緯

M4.2（Supporters、Epic #925）は、管理者が有効にしたときだけ、ローカルの利用者が Web から月払い・年払いのプランに加入し、支払いの状態に合わせてロールが付け外しされることを求める（FR-910・FR-911）。収入は事業者の手数料を引いた額で記録し（FR-917）、カード情報はこのサーバーを通らない（FR-910）。
この ADR はポートと、事業者の状態をこちらに運ぶ経路だけを決める。加入の状態・猶予・ロールとプランの規則は ADR-0072、収入の記録は [ADR-0067](0067-finance-cost-records.md) が決める。
重視したのは、webhook の重複・順序の入れ替わり・取りこぼしがあっても最後には事業者の状態と一致すること（FR-911、ADR-0004）、偽の webhook でロールを得られないこと（ADR-0019）、使わないときは何も動かず費用を増やさないこと（NFR-COST、ADR-0010）、依存が AGPL と両立すること（ADR-0011）である。

最初の事業者は Stripe だけにした。ロックインされないように決済はポートを通す。もう 1 つの候補の Paddle は利用規約が支援・寄付・コミュニティへのアクセスを禁じるので外した。
Stripe の事実は spike #909 の[調査報告](https://github.com/mycroblog/my/issues/909#issuecomment-6326974258)による（2026-10-22 に公式の文書・料金のページ・`stripe-go` のソースで確認）。形を決めたのは次の点である。Checkout はホスト型の画面へのリダイレクトだけで済み、PCI DSS の SAQ A の範囲に入る。webhook は順序を保証せず、重複がありうる。`GET /v1/events` は 30 日しか遡れない。Stripe 自身が `invoice.paid` の後の取り直しを勧めている。`webhook.ConstructEvent` はイベントの API の版の系列が SDK と違うとエラーにする。Billing の 0.7% などの手数料は別の `stripe_fee` の取引として載る。
照合の周期は 6 時間固定だと加入者が 1 万人を越えたときに問題が起こるので、既定を 6 時間として設定で変えられるようにした。鍵の置き場所は、CAPTCHA の秘密（ADR-0106）や ADR-0068 と同じデプロイの設定にした。

日本の運営者が国内の決済代行で課金できるように、2 つめの事業者として PAY.JP を足した（#1301）。PAY.JP の v2 は定期課金の API をまだ持たないので（[v2 のドキュメント](https://docs.pay.jp/v2/llms-full.txt)の「API v1 からの移行について」、2026-10-25 に確認）、v1 を使う。
v1 の事実は 2026-10-25 に [API v1 リファレンス](https://pay.jp/docs/api/)で確かめた。Stripe との次の違いが §9 から §17 の形を決めた。ホスト型の決済画面も顧客ポータルもない。webhook の検証は固定のトークンだけで、時刻も本文の署名もない。支払いに失敗した定期課金は `paused` で止まり、再試行しない。返金のオブジェクトはなく、手数料の額は区間ごとの取引明細にだけ出る。チャージバックは `dispute.created` のイベントだけで、取り消しを知る手段がない。リファレンスに冪等キーの記載がない。レート制限はテストで 1 秒 2 件と狭い。
日本の EC のクレジットカード決済には 3D セキュアの義務があるので、カードの登録はトークン作成時の 3D セキュアを通す。
PAY.JP の有料プランの利用料と振込の手数料は費用として扱い、チャージバックの取り消しは運営者が記録することにした。決済のページがこのサーバーのオリジンに載るので、セキュリティと PCI DSS を §11 に具体的に書いた。

チップ（#2612）のために、1 回の支払いを §18 に足した。PAY.JP v2 はホスト型の決済画面・冪等キー・dispute の一覧を持ち、1 回の支払いには v1 の自前のページより向くので、定期課金のない v2 を 1 回の支払いにだけ使い、加入は v1 に残した。

Stripe の上げる変更は、はじめは `pending_if_incomplete` を付けず、Stripe の既定（`allow_incomplete`）で切り替えていた。Stripe はこの既定を「支払いの成否に関わらず適用し、失敗すれば手で戻す」と文書にしている。テストモードで払えないカードの利用者が上位のプランのまま `past_due` になったので、ADR-0072 §8 の決定に合わせて §2 の形に変えた。

## 採らなかった案

- イベントの本文から状態を順に適用する: webhook ごとに API を呼ばずに済み、事業者が止まっていても受け取れる。ただ Stripe は順序を保証せず `created` は 1 秒の分解能で、形が API の版で変わり、取りこぼしは別の仕組みが要る。
- Stripe の REST を直接呼ぶ: 依存が増えず、SDK の major の版の追従が要らない。ただ使う 10 ほどの API の型・ページ送り・エラー・冪等キー・再試行を自分で持つ。`stripe-go` の依存は MIT の 2 つで、`CGO_ENABLED=0` と `go-licenses` を通る。
- `webhook.ConstructEvent` で検証する: 公式の実装で自分で書くコードがない。ただ SDK を上げるとエンドポイントの API の版も同時に上げる必要がある（`IgnoreAPIVersionMismatch` で外せるが、型付きの本文の形が合う保証はない）。
- 顧客ポータルの `subscription_update` でプランを変える: 期間の終わりへの変更（`schedule_at_period_end` の `decreasing_item_amount`・`shortening_interval`）と日割りを Stripe が実装し、コードが減る。ただ変更先は 10 個の Product まで、プランの名前と説明が i18n（ADR-0009）の外になり、プランの規則がポータルの設定に分かれ、ほかの事業者に移せない。
- `allow_incomplete` のまま失敗したら価格を戻す: 片付けが単純に見える。ただ一度は切り替わるのでロールが往復し、戻す要求も失敗しうる。
- `default_incomplete`: Stripe が支払いを試みないので、確かめる手順をこちらが全部持つことになる。
- `error_if_incomplete`: 失敗を 402 で返して更新しないと文書にあり、片付けが要らないように見える。ただ失敗した試みの請求書がどうなるかの記述がなく、回収できるものが残らないことを文書で保証できない。
- 認証のために Stripe の請求書の画面へ送る: 確かめてから変える決定そのものは満たす。ただ 23 時間のあいだ請求書が開いたままで、後からプランが変わる（ADR-0072 §8）。
- 終わっていない加入を毎回 1 件ずつ取り直す照合: こちらの記録だけで決まり、コードが最も少ない。ただ要求の数が加入者の数に比例し、1 万人で 1 回 1 万要求・1 秒 20 件で 8 分を越え、レート制限に当たる。
- `GET /v1/events` を遡るだけの照合: 変わったものだけを取り直すので最も安い。ただ 30 日しか遡れず、イベントの漏れを直せない。§4 はこれを 1 段めに使い、見回しで補う。
- 管理画面で鍵を設定し、`Sealer` で暗号化して DB に持つ: FR-910 の課金の有効化を再デプロイなしに管理画面だけで満たす。ただ返金や解約ができる鍵が DB に入り、DB と KEK が両方漏れると使われる。
- `kernel.HTTPClient` を通す: 外向きの HTTP の方針が 1 か所になる。ただ宛先は固定のホストだけで SSRF ガードが守るものがなく、`http.RoundTripper` に包む橋と新しい用途の上限が要る。
- PAY.JP v2 の Setup Flow と Payment Flow でこちらが毎期の課金を組む: 移行先で、Checkout v2 の `setup` モードでカードの登録までホスト型の画面で済み、冪等キーがある。ただ定期課金のエンジンをこちらが持つことになり、事業者の側の事実を取り直す前提（§3）が崩れる。
- PAY.JP v2 の定期課金 API を待つ: 作業も移行も要らない。ただ時期が決まっておらず（「2026年対応予定」）、日本の運営者が今は使えない。
- payjp.js を Web クライアント（SPA）に組み込む: 画面の見た目と部品が揃い、React と client の i18n で書ける。ただ Web クライアント全体の CSP に PAY.JP のオリジンを足し、事業者ごとのコードが SPA に入り、SPA のどの画面にも PAY.JP のスクリプトを載せうる範囲になる。
- PAY.JP v1 の Checkout（`checkout.pay.jp` のボタンのスクリプト）: フォームのコードが要らない。ただ結局こちらのページにスクリプトを載せ、見た目と文言を変えにくく、3D セキュアの扱いを別に組む。
- PAY.JP の `prorate=true` で上げる: PAY.JP の機能でコードが少ない。ただ旧プランの残りを返金として返すので、支払いの直後の変更では返金が支払いの全額に等しくなりえ、ADR-0072 §7 が全額の返金として加入を `revoked` にしてしまう。
- PAY.JP の手数料を取引明細の区間ごとの額だけで記録する: 明細そのままで計算の誤りがない。ただ支払いごとの手数料と net（FR-917、ADR-0067 §12）が埋まらず、区間が締まるまで手数料が記録されない。
- `payjp-go` を使う: 公式の SDK で、型とリトライがある。ただリファレンスが `beta` ブランチを指し、版の追従が読みにくい。使う API は十数で、標準ライブラリで足りる。

## Consequences（影響）

- 良い影響
  - 状態を事業者から取り直して導くので、webhook の重複・順序・取りこぼしを 1 つの扱いで済ませ、照合のジョブも同じ経路を使う。
  - カード情報と支払い方法の詳細を持たず、Web クライアントは事業者を知らず、その CSP も変えない。PAY.JP のスクリプトは adapter の短命のページにだけ載る。
  - webhook の検証が SDK の版によらないので、`stripe-go` の major の版を上げても既存のエンドポイントのイベントを受け続ける。
  - ポートが事業者の側の事実だけを返すので、別の事業者を同じ契約テストで足せる。PAY.JP で足した型は任意の `PageServer` と契約テストの 1 項目だけで、依存も増えなかった。課金が無効なら定期ジョブも経路も動かない。
- 悪い影響・トレードオフ
  - webhook ごと・照合ごとに事業者の API を呼ぶ。全件の見回しは 1 万人で 1 回 100 要求ほどだが、10 万人を越えると 1 周が数回の照合にまたがり、取りこぼしの直りが遅れる。§4 の 4 段とカーソルと続きのジョブの分だけコードが増える。
  - `stripe-go` は半年ほどで major の版が上がり、API の版（basil 以降の項目の場所など）の追従が adapter に要る。
  - プランの変更をこちらの画面で持つので、Subscription Schedule の扱いを adapter が持つ。
  - 鍵と webhook の秘密をデプロイの設定に置くので、事業者を有効にするには再デプロイが要る。webhook のエンドポイントの登録は運営者の手作業になる。
  - webhook の受け口が匿名で叩ける経路を 1 つ増やす。署名の検証の前に読む本文を 1 MiB に限り、ADR-0019 §5 のレート制限の対象にする。
  - カードの入力・3D セキュア・顧客ポータルのページを `payjp` adapter が持つので、`stripe` adapter よりコードが多く、i18n の文言とページのテストが増える。カードの入力のページがこのサーバーのオリジンに載る。
  - PAY.JP の webhook の検証は固定トークンなので、トークンが漏れると通知を偽造できる（状態は取り直すので書き換えられないが、ジョブと PAY.JP への要求を増やせる）。トークンの切り替えには PAY.JP 側の再発行と設定の変更を同時に行う。
  - PAY.JP は支払いの失敗で定期課金を止め、再試行しないので、猶予の内に利用者がカードを直さなければ戻らない。猶予の通知（ADR-0072 §16）がより大事になる。
  - PAY.JP の上げる変更は、Stripe と違って期間の始まりを動かさない。差額の課金が断られたら変更しないのは Stripe と同じである。
  - PAY.JP のチャージバックの取り消しは自動で記録されない。手数料は区間の締めまで charge ごとの計算の額で、締めで補正される。
  - PAY.JP v1 は v2 への移行が求められている API で、いずれ移行の作業が要る。

## Open Questions / 要検証

- アーカイブした Price のサブスクリプションが、その Price のまま更新され続けるか（§2）。
- Account の `settings.dashboard.timezone` を制限付きの鍵で読めるか（§2）。
- Stripe の `ResumeCheckout` の比較が、Stripe の保存する値と一致するか（§2）。
- Plan の削除が既存の定期課金に効くか（効かなくても §10 は削除しない）と、Plan の ID の長さの上限。
- `fee_rate` からの手数料の端数の扱い、返金とチャージバックで返る手数料の計算（§13）。
- `dispute.created` のイベントの本文の形（§13）。
- Events API の保持期間（§12・§14）。
- payjp.js（`https://js.pay.jp/v2/pay.js`）の iframe 型の 3D セキュアの CSP に要るオリジン（§11）。
- v1 で `Idempotency-Key` が使えるか（使えれば §10 の命令にも付ける）。
- payjp-cli が v1 のイベントを転送できるか（§17）。
