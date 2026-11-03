# ADR-0074: 決済の PaymentProvider ポート（Stripe）

- Status: Accepted
- Date: 2026-10-22
- Deciders: @rosylilly
- Issue: #913

## Context（背景・課題）

M4.2（Supporters、Epic #925）は、管理者が有効にしたときだけ、ローカルの利用者が Web から月払い・年払いのプランに加入し、支払いの状態に合わせてロールが付け外しされることを求める（FR-910・FR-911）。
収入は事業者の手数料を引いた額で記録する（FR-917）。カード情報はこのサーバーを通らない（FR-910）。

決定者の回答（#925、原文の抜粋）:

> 9\. 推奨: 1つのサーバーで同時に有効にできる事業者は1つだけにします。切り替えるときは、既存の加入者に再加入してもらうことを docs に書きます。 → これで OK

spike #909 の確認事項への決定者の回答（#913、原文）:

> 1. Paddle を選択肢に残すか : 規約で認められないなら無理ですね。落とす
> 2. 返金・チャージバックのとき: 推奨通りで OK
> 3. Paddle を除外したので考慮外
> 4. Stripe の Billing の手数料 : 収入から差し引く
> 5. プランを変えたときの日割り: 推奨案で OK

決定者の回答（PR #933、原文）:

> 1. Stripe の鍵の置き場所: 設定(環境変数)におく。
> 2. プランの変更: 推奨案で OK
> 3. 照合の周期: 既定を6時間とし、設定で変えられる。6時間固定だと加入者が10,000人を越えている場合などに問題が起こる
>
> ADR Accepted

issue #913 の回答の「推奨」は次のとおり。1 = 事業者は Stripe だけ。決済はポートを通し、事業者に依存しない形に保つ。
2 = 全額の返金とチャージバックはロールを外し、事業者のサブスクリプションもすぐ解約する。一部の返金ではロールを外さない。dispute に勝ってもロールは自動で戻さない。
5 = 上げる変更はすぐ日割りで請求し、下げる変更と年払いから月払いへの変更は期間の終わりに切り替える。

事業者の事実は spike #909 の[調査報告](https://github.com/mycroblog/my/issues/909#issuecomment-6326974258)（2026-10-22 に Stripe の公式の文書・料金のページ・`stripe-go` のソースで確認。アカウントの作成と課金のある操作はしていない）による。要点は次のとおり。

- Checkout（`mode=subscription`）はホスト型の決済画面へのリダイレクトだけで済み、こちらの Web クライアントに Stripe のスクリプトを読み込まない。Checkout は PCI DSS の SAQ A の範囲に入る。
  `client_reference_id` と `subscription_data.metadata` にこちらの ID を入れられる。顧客ポータル（Customer Portal）は API で設定とセッションを作る。
- Price は金額を変えられず、変えるときは新しい Price を作って古い Price を `active=false` にする。`lookup_key` で引け、`transfer_lookup_key` で新しい Price に移せる。
- webhook は `Stripe-Signature: t=<unix>,v1=<hex>[,v1=…]` の HMAC-SHA256 で、順序を保証せず、重複がありうる。再送はライブで最大 3 日。`GET /v1/events` は 30 日しか遡れない。
  Stripe は「`invoice.paid` を受けたらサブスクリプションを取り直して状態を確かめる」ことを勧めている。
- `stripe-go`（v87、MIT、依存は MIT の `shopspring/decimal`）は `CGO_ENABLED=0` で build でき、`go-licenses` を通る。
  `webhook.ConstructEvent` はイベントの API の版の系列が SDK と違うとエラーにするので、SDK の版を上げると既存のエンドポイントのイベントを受け付けなくなる。
- お金の動きは balance transaction（`amount`・`fee`・`net`・`currency`・`reporting_category`）で、Billing の 0.7%・Stripe Tax・Radar の手数料は別の `stripe_fee` の取引として載る。返金で処理の手数料は返らず、dispute の手数料（¥1,500）は勝っても返らない。
- テストには、サンドボックス・テスト用のカード・test clock・Stripe CLI（`stripe listen --forward-to`・`stripe trigger`、Apache-2.0）がある。

同時に書かれる ADR との分担:

- ADR-0072（#912、加入とプランとロール）は、モジュール `supporters`、こちらの加入の状態（申し込み中・有効・解約の予約・支払いの遅れ・猶予の切れ・失効・取り消し）、7 日の猶予、ロールの付け外し、プランの変更と価格の改定の規則を決める。
  この ADR はポートと、事業者の状態をこちらに運ぶ経路（webhook・照合）だけを決め、状態の導き方は決めない。
- [ADR-0073](0073-income-records-and-publication.md)（#914、収入）は、`finance` の `RecordIncome(ctx, []IncomeMovement)` と、その契約（種類・元の支払い・期間・総額・手数料・net・事業者のレポートの日付）を決める。この ADR の `MoneyMovement`（§1）はそれを満たす。

決めること: ポートの形と契約テスト、adapter と SDK、webhook の受け口、照合のジョブ、カード情報の扱い、設定と秘密、テストモードとローカルでの試し方、build tag、外向きの HTTP の分類、CSP。
実装の #919（ポート・webhook・照合）・#920（`stripe` adapter）と、決定者のテストモードでの確認 #924 がこの ADR を待っている。

## Decision Drivers（判断基準）

1. 支払いの状態とロールが、webhook の重複・順序の入れ替わり・取りこぼしがあっても最後には事業者の状態と一致すること（FR-911、ADR-0004 の at-least-once）
2. カード情報と支払い方法の詳細がこのサーバーを一切通らず、運営者の PCI の範囲が最小であること（FR-910、NFR-SEC）
3. 偽の webhook でロールを得られないこと: 署名・時刻・テストとライブの別を確かめる（ADR-0019）
4. ポートが事業者に依存せず、別の事業者の adapter を後から同じ契約テストで足せること（決定者の回答 1）
5. 運用の費用がほぼ $0 で、使わないときは何も動かず、adapter を build tag で外せること（NFR-COST、ADR-0010）
6. 依存が AGPL と両立し、`CGO_ENABLED=0` で build でき、SDK の版の追従が webhook を壊さないこと（ADR-0011）
7. 照合の費用が加入者の数に比例して膨らまず、1 万人を越えても事業者のレート制限の内で回ること（決定者の回答 3）
8. 実装が小さいこと

## Considered Options（選択肢）

論点ごとに比べる。各論点の 1 つめを採用する。

- **状態の正**: T1 webhook は合図として受け、ジョブが事業者の API からサブスクリプションを取り直して状態を導く / T2 イベントの本文から状態を順に適用する
- **事業者の数**: N1 Stripe の adapter だけを作り、ポートは事業者に依存しない形にする / N2 2 つめの事業者の adapter も作る（Paddle は利用規約が支援・寄付・コミュニティへのアクセスを禁じるので外れ、ほかの候補は調べていない）
- **API の呼び出し**: R1 `stripe-go` を adapter の中で使う / R2 `net/http` で REST を直接呼ぶ
- **webhook の検証**: W1 標準ライブラリで検証し、イベントは必要な項目だけを読む / W2 `stripe-go` の `webhook.ConstructEvent`
- **プランの変更**: P1 こちらの画面と API から `ChangePrice` で変え、顧客ポータルではプランを変えさせない / P2 顧客ポータルの `subscription_update` に任せる
- **照合**: C1 一覧の API でまとめて読み（変わったもの・お金の動き・全件の見回し）、カーソルと 1 回の要求の上限で複数の回に分ける / C2 終わっていない加入を毎回 1 件ずつ取り直す / C3 `GET /v1/events` を遡るだけ
- **設定と秘密**: S1 デプロイの設定（TOML と `MY_*`、秘密は環境変数か `_FILE`）/ S2 管理画面で設定し、秘密を `Sealer` で暗号化して DB に持つ
- **外向きの HTTP**: H1 運営者が設定した事業者の API として SDK の HTTP クライアントを使う / H2 SSRF ガード付きの `kernel.HTTPClient` を通す

## Decision（決定）

**T1・N1・R1・W1・P1・C1・S1・H1 を採用する。** 鍵の置き場所（S1）・プランの変更（P1）・照合の周期を設定で変えられること（C1）は決定者の回答（PR #933）による。

### 1. `PaymentProvider` ポート

ポートは `internal/app/supporters`（ADR-0072）に置く。型は標準ライブラリだけで、SDK の型を出さない（ADR-0068 の `BillingSource` と同じ方針）。
ポートは「事業者の側の事実」だけを返し、こちらの加入の状態・猶予・ロールは `supporters` が導く。1 つのサーバーで有効な adapter は 1 つ（決定者の回答 9）。

```go
type PaymentProvider interface {
    // Name is the provider's identifier: "stripe".
    Name() string
    // Location is the time zone of the provider's reports; MoneyMovement.OccurredOn uses it (ADR-0073 §3).
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
    // PortalURL returns a short-lived URL of the provider's customer portal.
    PortalURL(ctx context.Context, customer CustomerRef, returnURL string) (string, error)

    // Commands our own UI and jobs issue; each is safe to repeat.
    ChangePrice(ctx context.Context, sub SubscriptionRef, to PriceRef, when ChangeTiming, key string) error
    CancelScheduledChange(ctx context.Context, sub SubscriptionRef, key string) error
    CancelAtPeriodEnd(ctx context.Context, sub SubscriptionRef, key string) error
    ResumeCancellation(ctx context.Context, sub SubscriptionRef, key string) error
    CancelNow(ctx context.Context, sub SubscriptionRef, key string) error

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
    PeriodStart         time.Time
    PeriodEnd           time.Time
    CancelAtPeriodEnd   bool
    ScheduledPrice      PriceRef  // zero unless a change waits for PeriodEnd
    LatestPaymentFailed time.Time // zero unless the latest invoice's payment failed and is unpaid
}

type MoneyMovement struct {
    ID           string    // provider id, unique per provider (Stripe: balance transaction id)
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
- `ChangeTiming` は `now_prorated`（すぐ切り替え、差額を日割りですぐ請求する）と `period_end`（期間の終わりに切り替える）。どちらを使うかは ADR-0072 が決め（決定者の回答 5）、adapter は事業者の機能に写す（§6）。
  `CancelScheduledChange` は、期間の終わりを待つ変更を取り下げる。`ScheduledPrice` が空なら何もしない。
- 状態の名前は Stripe の語を事業者に依存しない意味で使う。`trialing` は試用期間を作らないので `active` に写し、知らない状態は `Status` を `unpaid`（ロールを付けない側）にして警告のログを出す。
- `MoneyMovement` は ADR-0073 §1 の `IncomeMovement` に、`supporters` が `Subscription` をこちらの加入の ID に写して渡す。`ParentGross` は `supporters` が全額の返金と一部の返金を見分けるために使う（ADR-0072）。
  `ListMoney`・`GetMoney`・`Resolve` が返すのは、事業者の側で額が確定した動き（`Final = true`）だけで、まだ確定しない動きは返さず、次の照合で取り直す（ADR-0073 §1）。`Final` は ADR-0072 の判断が確かめるために残す。
- 失敗は `supporters` が定義する種類の値で返す: `permission`（鍵の誤り・権限の不足）、`not_found`、`invalid_request`（`StartCheckout` の通貨の不一致など）、`throttled`（429）、`unavailable`（5xx・ネットワーク）、`invalid_signature`・`stale_signature`・`mode_mismatch`（`ParseWebhook`）。
- **契約テスト**: `pkg/testkit.PaymentProviderContract(t, factory)` を置く。`factory` は、契約が渡す事業者の側の筋書き（プランと価格、申し込み、完了と期限切れ、更新、支払いの失敗と成功、解約の予約と取り消し、即時の解約、期間の終わりの変更、全額と一部の返金、チャージバックとその取り消し、独立した手数料、ページの数、権限の不足、レート制限）を受け、それを返す fake の上で動く `PaymentProvider` と、そのテスト用の webhook の署名器を作る。
  契約は次を確かめる。
  - `EnsurePlan` を繰り返しても価格が増えず、金額を変えると新しい価格と古い価格のアーカイブになり、古い価格のサブスクリプションが残ること。
  - `StartCheckout` の URL が §7 の許可したホストの `https` で、`GetCheckout` が `LocalID` を返すこと。
  - 各コマンドを 2 回呼んでも結果が同じこと。
  - `ParseWebhook` が、正しい署名・改ざんした本文・許容の外の時刻・複数の署名・テストとライブの食い違い・知らない種類を見分け、I/O をしないこと。
  - スナップショットの状態の写し。
  - `MoneyMovement` の符号、`Net = Gross − Fee`、`ParentID` と `ParentGross`、`Covers`、`OccurredOn` の日付の基準（ADR-0073 §1 の契約テストの項目）。
  - ページ送りの漏れと重複がないこと、失敗の種類の分類、`ctx` の取り消し。

  `fake` adapter（`internal/adapter/payment/fake`、#919）と `stripe` adapter（Stripe の REST を `httptest` で返す、#920）がこれを通す。実アカウントは使わない。

### 2. `stripe` adapter（`internal/adapter/payment/stripe`）

- `github.com/stripe/stripe-go/v87`（MIT）をこの adapter の中だけで使う（R1）。SDK の型は adapter の外に出さず、組み立ては `internal/platform`（`cmd/` から）が行う。API の版は SDK が固定する版を使う。
- **プラン**: Product の `metadata.my_plan_id` で引き、なければ作る。Price は `lookup_key` を `my_<plan_id>_<interval>_<currency>` にし、金額が違えば新しい Price を `transfer_lookup_key=true` で作って古い Price を `active=false` にする（Stripe の Price は金額を変えられない）。
  年の Price の金額は ADR-0072 が割引率から決めて渡す（クーポンを使わない）。アーカイブした Price のサブスクリプションは、その Price のまま更新され続ける（ADR-0072 の価格の改定。**要検証**、#924 で確かめる）。
- **最低額**: `MinimumAmount` は Stripe の最低の請求額の表（JPY 50、USD 0.50 など。表の全体は **要検証**）を adapter に持つ。
- **Checkout**: `mode=subscription`、`ui_mode` はホスト型、`line_items` に Price を 1 つ、`client_reference_id` と `metadata.my_subscription_id` と `subscription_data.metadata.my_subscription_id`（正確な項目名は **要検証**）に `LocalID`、`customer` か `customer_email`、`locale`、`success_url`（`?checkout={CHECKOUT_SESSION_ID}` を付ける）、`cancel_url`、`expires_at` は 1 時間後。
  税（Stripe Tax、`automatic_tax`）は使わない。運営者が使うかは Stripe のダッシュボードの設定の範囲で、この ADR は扱わない。
- **顧客ポータル**: 起動時に設定（`/v1/billing_portal/configurations`）を `metadata.my=1` で引き、なければ作る。
  有効にする機能は、支払い方法の更新、請求書の履歴、期間の終わりでの解約（`subscription_cancel.mode=at_period_end`）だけで、`subscription_update` は無効にする（P1）。`PortalURL` はこの設定と `return_url` でセッションを作る。
- **プランの変更**（決定者の回答 5）:
  - `now_prorated`: サブスクリプションの品目の Price を差し替え、`proration_behavior=always_invoice` で差額をすぐ請求する。月から年への変更は Stripe が請求日を変更の日に移し、その場で年額を請求する。
  - `period_end`: Subscription Schedule を作り（既存のサブスクリプションから）、今の期間の終わりから新しい Price の段階を足す。`CancelScheduledChange` は Schedule を解放（release）する。
    Schedule の作り方と、`subscription_update` を使わずに期間の終わりへ回せることは **要検証**（#920 で fake の REST の形を Stripe の文書に合わせる）。
- **解約**: `CancelAtPeriodEnd` と `ResumeCancellation` は `cancel_at_period_end` を立てる・下ろす。`CancelNow` は `DELETE /v1/subscriptions/{id}`（`prorate=false`、`invoice_now=false`）で、もう終わっていれば何もしない。
  全額の返金とチャージバックのときに `supporters` が呼ぶ（決定者の回答 2、ADR-0072）。
- **冪等**: 作成と変更の POST には `Idempotency-Key` に呼び出し側の `key`（ジョブの冪等キーと操作の名前）を付ける。Stripe はキーを 24 時間で消すことがあるので、各コマンドは呼ぶ前に今の状態を読み、もう望む状態なら呼ばない（24 時間を越えたジョブの再試行でも二重にならない）。
- **お金の動き**: `GET /v1/balance_transactions`（`created[gte]`、`starting_after`）を読み、`reporting_category` で種類に写す: `charge` → `payment`、`refund`・`refund_failure` → `refund`、`dispute` で負 → `chargeback`、`dispute_reversal` で正 → `chargeback_reversal`、`fee`（`type=stripe_fee`）→ `fee`。ほかは返さない。
  サブスクリプションと期間は `source` の charge → PaymentIntent → `invoice_payments` → invoice → `parent.subscription_details.subscription` と invoice の品目の期間から引く（API の版 basil 以降の形）。
  金額は Stripe の最小単位の整数を ISO 4217 の小数の桁で 10 進の文字列にする（Stripe が独自の桁を使う通貨は **要検証**）。balance transaction は作られた後に額が変わらないので `Final = true`。
- **レポートのタイムゾーン**: `Location()` は、起動時と照合のたびに Account の `settings.dashboard.timezone`（Stripe のアカウントのタイムゾーン）を読んだ値で、読めなければ UTC を返す（ADR-0073 §3。読めるかは **要検証**）。UTC にしたときは管理画面の事業者の状態に示す。

### 3. webhook の受け口（T1・W1）

- 経路は `POST /payments/webhooks/{provider}`（例 `/payments/webhooks/stripe`）。新しい `internal/interface/payments` に置き、OpenAPI の client API には載せない（事業者が呼ぶ経路で、利用者は呼ばない）。課金が無効か、`{provider}` が設定の事業者でなければ `404`。
- 受け口がすること（1 つの要求の中で、外部の I/O をしない）:
  1. 本文を 1 MiB まで読む（超えたら `413`）。
  2. `ParseWebhook` で署名を検証する。`stripe` adapter は標準ライブラリ（`crypto/hmac`・`crypto/sha256`・`crypto/subtle`）で `"<t>.<生の本文>"` の HMAC を計算し、ヘッダーのどれかの `v1` と定数時間で比べ、`t` が今から ±5 分の中にあることを確かめる（W1）。
     秘密を切り替える間は Stripe が新旧の秘密で署名を並べるので、設定の秘密は 1 つでよい。
  3. 本文は `encoding/json` で `id`・`type`・`livemode`・`data.object.id`・`data.object.object` だけを読む。API の版で形が変わる項目は読まないので、SDK の版を上げても、エンドポイントの API の版が SDK と違っても受け付ける。
     `livemode` が鍵のモード（§5）と違えば `mode_mismatch` で拒む（テストのイベントで本番のロールが付かないように）。
  4. 1 つの短いトランザクションで、受信の記録（§4 の `payment_webhook_receipts`、`(provider, event_id)` で一意）を足し、同じトランザクションで outbox（ADR-0004）にジョブ `supporters.apply_payment_event` を積む。冪等キーは `payment_event:<provider>:<event_id>`、`orderingKey` は対象のオブジェクトの ID。
     受信の記録がもうあれば何もしない（重複の受信）。
  5. `2xx` を返す。署名の誤り・古い時刻・モードの食い違いは `400`、DB の失敗は `503`（Stripe が再送する）。`ignored` の種類も記録せずに `2xx` を返す。
- 運営者が Stripe に登録するイベント: `checkout.session.completed`・`checkout.session.expired`、`customer.subscription.created`・`.updated`・`.deleted`・`.paused`・`.resumed`、`invoice.paid`・`invoice.payment_failed`、`charge.refunded`・`refund.created`・`refund.updated`、`charge.dispute.created`・`.closed`・`.funds_withdrawn`・`.funds_reinstated`。
  docs（#920）に一覧と、Stripe CLI・ダッシュボードでの登録の手順を書く。サーバーはエンドポイントを API で作らない（秘密 `whsec_` を運営者が設定に置くため）。
- **ジョブ `supporters.apply_payment_event`**（冪等、ADR-0004）:
  1. `Resolve` で、通知が指すサブスクリプション・申し込み・お金の動きを事業者から取り直す（イベントの本文の状態は使わない、T1）。
  2. 申し込みなら `GetCheckout` で完了を確かめ、顧客とサブスクリプションの ID を加入に結び付ける。
  3. サブスクリプションごとに `GetSubscription` を呼び、ADR-0072 の `supporters.sync`（スナップショットからこちらの状態を導き、ロールを付け外す）に渡す。
  4. お金の動きは、返金・チャージバックの元の支払いがまだ記録になければ `GetMoney` で先に取り、`finance.RecordIncome`（ADR-0073）と `supporters`（取り消しの判断、ADR-0072）に渡す。
  - 事業者の API はトランザクションの外で呼び、結果を短いトランザクションで書く（ADR-0003）。`throttled`・`unavailable` は ADR-0004 の指数バックオフで再試行し、`permission` は記録して成功で終える（照合と管理画面の状態に出る）。
  - 同じサブスクリプションのジョブは `orderingKey` で直列にし、取り直してすぐ適用するので、後に適用したものが新しい状態になる。重複・順序の入れ替わり・取りこぼしは、どれも「取り直して適用する」の 1 つの扱いになる。

### 4. 照合のジョブ（C1）とデータモデル

- `Scheduler`（ADR-0004）の定期ジョブ `supporters.reconcile_payments` を、課金が有効なときだけ載せる。周期は設定 `payments_reconcile_interval`（既定 6 時間、15 分〜7 日。決定者の回答 3）で、冪等キーは既存の `schedule:<name>:<slot>`。
  既定の 6 時間なら、webhook を落としても 6 時間で追いつき、7 日の猶予の判定には十分に短い。
- 1 回の照合が事業者に送る要求は、設定 `payments_reconcile_max_requests`（既定 1,000）までにする。adapter は要求を 1 秒に 20 件まで（テストのモードでは 10 件まで）に抑え、`429` には `Retry-After` に従って待つ。
  Stripe の読み取りのレート制限（ライブで 1 秒に 100 件ほど、テストで 25 件ほど）の内に収めるための値で、制限の正確な値は **要検証**。既定の上限と速さなら、1 回の照合は 1 分ほどで終わる。
- 照合は次の 4 段を順に行い、上限に達した段でカーソルを記録して止まる。続きは、続きのジョブ（冪等キー `continue:<run_id>`、1 分後）が同じ上限で引き継ぐ。どの段も一覧の API でまとめて読み、1 件ずつの取り直しは変わったものに限る（C1）。
  1. **変わったもの**: `ListChanges` で、前回の成功の照合が読み終えた時刻の 5 分前からのイベントを 100 件ずつ読む（Stripe は `GET /v1/events`、§3 の種類だけ）。
     イベントが指すサブスクリプションを重ねずに集め、それぞれ `GetSubscription` で取り直して `supporters.sync` に渡す。webhook を落としたものはここで直る。
  2. **未完了の申し込み**: 期限の切れていない申し込みごとに `GetCheckout`（期限は 1 時間なので、件数は少ない）。
  3. **お金の動き**: `ListMoney` を前回のカーソルから 100 件ずつ読む（重なりは `(provider, provider_id)` の一意で吸収する、ADR-0073 §2）。`stripe_fee` もここで入る（決定者の回答 4）。balance transaction の一覧には遡る期間の制限がない。
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
| `payment_webhook_receipts` | イベント | `id`, `provider`（`enum(stripe)`）, `event_id`（`string(255)`）, `event_type`（`string(100)`）, `object_id`（`string(255)`）, `received_at`。一意 `(provider, event_id)`。本文は持たない（§5） |
| `payment_reconcile_runs` | イベント | `id`, `provider`, `run_key`, `outcome`（`enum(succeeded, partial, failed)`）, `error_kind`（§1 の種類と `none`・`request_cap`）, `changes_until`（`timestamp`）, `changes_cursor`・`money_cursor`・`sweep_cursor`（`string(255)`、空は先頭）, `requests`・`checked_subscriptions`・`corrected_subscriptions`（`int`）, `occurred_at`。一意 `(provider, run_key)` |

- 次の照合は、最新の `succeeded` か `partial`（上限で止まった）の行のカーソルから始める。カーソルは試行ごとの行に残し、上書きする現在値の列を持たない（規約 4）。

- `payment_webhook_receipts` は 45 日を過ぎた行を定期ジョブで消す（Stripe の手での再送は CLI で 30 日まで遡れる）。消した後に同じイベントが来ても、ジョブが取り直して適用するだけで状態は変わらない。
- 管理画面 `/admin/supporters`（ADR-0072）に事業者の状態を出す: 事業者とモード（テスト・ライブ）、最後に webhook を受けた時刻、最後の照合の結果と失敗の種類（`client*` のメッセージで直し方を案内する）。
  webhook を一度も受けていないか、照合が直した加入があれば、webhook の設定の誤りとして示す。API は `GET /admin/payment_status` と `POST /admin/payment_reconciles`。手動の照合は監査ログ（ADR-0037）に残す。

### 5. カード情報と秘密

- **カード情報はこのサーバーを通らない**: 支払いの入力は Stripe のホスト型の Checkout と顧客ポータルだけで行い、こちらの Web クライアントは Stripe のスクリプトも iframe も読み込まない。
  サーバーが持つのは Stripe のオブジェクトの ID（`cus_`・`sub_`・`price_`・`prod_`・`cs_`・`txn_`）と、金額・期間・状態だけで、カードの番号・下 4 桁・有効期限・請求先の住所を保存しない。
  webhook の本文と事業者の応答はログにも DB にも残さない（受信の記録は ID と種類だけ）。運営者の PCI の範囲は SAQ A 相当になる（運営者が確かめる事項として docs に書く）。
- **設定**（S1、ADR-0010。決定者の回答 1）: デプロイの設定（鍵と秘密は環境変数）に置き、`make config-docs` の表に載せる。インスタンス設定（DB）には置かない。課金を有効にすることとプランは、管理者がインスタンス設定で行う（ADR-0072）。

| キー | 秘密 | 内容 |
| --- | --- | --- |
| `payments_provider` | | `none`（既定）か `stripe`。1 つだけ |
| `payments_stripe_secret_key` | ✓ | API の鍵。権限を絞った制限付きの鍵（`rk_`）を勧め、必要な権限を docs に書く（**要検証**）。`sk_` も受ける |
| `payments_stripe_webhook_secret` | ✓ | webhook のエンドポイントの署名の秘密（`whsec_`） |
| `payments_reconcile_interval` | | 照合の周期（既定 `6h`、`15m`〜`168h`、§4） |
| `payments_reconcile_max_requests` | | 1 回の照合が事業者に送る要求の上限（既定 1,000、§4） |

- 秘密は環境変数か `<NAME>_FILE` から読み、`slog.LogValuer` で伏せる（ADR-0019 §6）。
- **モード**: 鍵の接頭辞（`sk_test_`・`rk_test_` と `sk_live_`・`rk_live_`）で決め、`Mode()` が返す。`my config check` と管理画面に示し、テストのモードでは加入の画面に「テストのモード（課金されません）」を出す。webhook はモードの違うイベントを拒む（§3）。
- 事業者を切り替える（ほかの事業者が足されたとき）ときは、既存の加入者に再加入してもらう（決定者の回答 9）。docs に書く。

### 6. テストのしかた

1. **契約テスト**（§1）: fake と `stripe` の両方。`stripe` は `httptest` のサーバーで Stripe の REST の JSON を返し、SDK の `BackendConfig` の URL をそこに向ける（テストだけ）。fixture は Stripe の文書の例の形に合わせる。
2. **`supporters` のテスト**（#919）: fake の上で、同じ webhook を何度も受ける・順序を入れ替える・webhook を落として照合だけで追いつく・署名の誤りを拒む、の筋書きで最後の状態とロールを確かめる。
3. **ローカルでの試し方**（docs、#920）: Stripe のサンドボックスの鍵を設定し、`stripe listen --forward-to http://localhost:<port>/payments/webhooks/stripe` が出す `whsec_` を `payments_stripe_webhook_secret` に置く。
   `stripe trigger` で単発のイベントを、test clock で更新・支払いの失敗・猶予を進め、テスト用のカード（`4242…` 成功、`4000000000000341` 後で失敗、`4000000000000259` dispute）で決済する。
4. **テストモードでの通しの確認**（#924、決定者）: 加入・ロールの付与・プランの変更（上げる・下げる・月と年）・支払いの失敗と猶予・解約・全額と一部の返金・dispute・収入の記録。

### 7. 外向きの HTTP（H1）と CSP

- Stripe の API は、運営者が鍵を設定した事業者の固定のエンドポイント（`api.stripe.com`）なので、AWS・Google Cloud の SDK のクライアントと同じく SDK の HTTP クライアントを使い、`kernel.HTTPClient` を通さない（AGENTS.md の Design rules）。
  宛先は SDK が決め、設定で変えられない（テストの `httptest` だけが変える）。`internal/platform` がタイムアウト（全体 30 秒）と計測の付いた `*http.Client` を `stripe.BackendConfig.HTTPClient` に渡す。
- サーバーは Checkout と顧客ポータルの URL を取りに行かない。ブラウザがそこへ移るだけである。adapter は返す前に、その URL が `https` で、ホストが `checkout.stripe.com` か `billing.stripe.com` であることを確かめる（応答の改ざんで別のサイトへ送らないため）。
- **CSP は変えない**: Web クライアントは client API で URL を受け、`location.assign` でトップレベルを移す。トップレベルの移動は CSP（`form-action`・`connect-src`・`frame-src`）の対象外で、Stripe のスクリプトも iframe も読まないので、ADR-0019 §9 の既定のポリシーと `Permissions-Policy: payment=()` のままで動く。
  戻り先の `success_url` と `cancel_url` はこちらのオリジンの画面（ADR-0072）。モバイルアプリには加入の導線を出さない（FR-911）。

### 8. build tag

- `no_payments_stripe` は `internal/adapter/payment/stripe` と `stripe-go` を外し、`payments_provider = stripe` を拒む（[build-tags.md](../reference/build-tags.md) の「Adding a tag」の手順）。`fake` は test だけで使い、本番の binary に入らない。
- 代表の組み合わせのうち、self-hosted の組に `no_payments_stripe` を足す（AWS と Google Cloud の組は含めたまま build を確かめる）。

### この決定を覆す条件

- 見回しの 1 ページに `latest_invoice` を展開できず、支払いの失敗をスナップショットに入れられないと分かったら（#920）、見回しは状態と期間だけを直し、支払いの失敗は 1 の変わったものに任せる。
- Subscription Schedule で期間の終わりの変更を組めないと分かったら（#920）、P2（顧客ポータルの `subscription_update` の `schedule_at_period_end`）に変え、ポータルの設定をプランに合わせて同期する。
- 再デプロイなしに管理画面から鍵を設定したいという要望が多ければ、S2（秘密を `Sealer` で暗号化して DB に持つ）を別の ADR で検討する。
- 2 つめの事業者が要るときは、同じ契約テストを通す adapter を足し、`payments_provider` の値と各表の `provider` の enum に足す。

## Consequences（影響）

- 良い影響
  - 状態を事業者から取り直して導くので、webhook の重複・順序・取りこぼしを 1 つの扱いで済ませ、照合のジョブも同じ経路を使う。
  - カード情報と支払い方法の詳細を持たず、Web クライアントの CSP も変えない。
  - webhook の検証が SDK の版によらないので、`stripe-go` の major の版を上げても既存のエンドポイントのイベントを受け続ける。
  - ポートが事業者の側の事実だけを返すので、ほかの事業者の adapter を同じ契約テストで足せる。課金が無効なら定期ジョブも経路も動かない。
- 悪い影響・トレードオフ
  - webhook ごと・照合ごとに Stripe の API を呼ぶ。全件の見回しは 1 ページ 100 件の一覧なので、1 万人で 1 回 100 要求ほどだが、10 万人を越えると見回しの 1 周が数回の照合にまたがり、取りこぼしの直りが遅れる。
  - `stripe-go` は半年ほどで major の版が上がり、API の版（basil 以降の項目の場所など）の追従が adapter に要る。
  - プランの変更をこちらの画面で持つので、Subscription Schedule の扱いを adapter が持つ（P2 よりコードが多い）。
  - 鍵と webhook の秘密をデプロイの設定に置くので、Stripe を有効にするには再デプロイが要る。webhook のエンドポイントの登録は運営者の手作業になる。
  - webhook の受け口が匿名で叩ける経路を 1 つ増やす。署名の検証の前に読む本文を 1 MiB に限り、ADR-0019 §5 のレート制限の対象にする。
- 後続で必要になる作業（Epic #925）
  - #919（ポート・webhook・照合）: §1 のポートと型と失敗の種類、`pkg/testkit.PaymentProviderContract`、`internal/adapter/payment/fake`、`internal/interface/payments` の `POST /payments/webhooks/{provider}`（本文の上限・レート制限・受信の記録と outbox）、ジョブ `supporters.apply_payment_event` と `supporters.reconcile_payments`（4 段・カーソル・要求の上限・続きのジョブ・手動）、`payments_reconcile_interval`・`payments_reconcile_max_requests` の設定、受信の記録の掃除、§4 の 2 つの表と `schema/<dialect>.sql`・sqlc、`payments_provider` の設定、`GET /admin/payment_status`・`POST /admin/payment_reconciles` と管理画面の状態の表示、overview §3 のポートの表と data-model の所有の表
  - #920（`stripe` adapter）: §2 の adapter（プランと価格・Checkout・顧客ポータルの設定・プランの変更と Subscription Schedule・解約・balance transaction の写し・`ListSubscriptions`（`latest_invoice` の展開）と `ListChanges`（events）・要求の速さの制御・最低額の表・レポートのタイムゾーン）、§3 の標準ライブラリの検証器、§7 の URL のホストの確かめ、`payments_stripe_secret_key`・`payments_stripe_webhook_secret` の設定と `make config-docs`、`no_payments_stripe` と build-tags.md、`stripe-go` の go.mod と `make go-licenses`、docs（ダッシュボードの設定、制限付きの鍵の権限、登録するイベント、Stripe CLI でのローカルの試し方、テストモード、PCI の範囲）
  - #924（決定者）: §6 の 4 の筋書きをテストモードで確かめ、要検証の項目（Subscription Schedule、アーカイブした Price での更新、`subscription_data.metadata`、Account のタイムゾーン、制限付きの鍵の権限、最低額）の結果を issue に書く

## Pros and Cons of the Options（詳細比較・任意）

### 状態の正: T1 取り直し（採用）/ T2 イベントを順に適用

- T2 の最も強い論点: webhook ごとに API を呼ばずに済み、事業者が止まっていても受け取れる。悪い点: Stripe は順序を保証せず `created` は 1 秒の分解能なので順序を決められず、イベントの形が API の版で変わり、取りこぼしは別の仕組みで直す必要がある。Stripe 自身も取り直しを勧めている。

### 事業者の数: N1 Stripe だけ（採用）/ N2 2 つめも作る

- N2 の最も強い論点: 決定者の当初の意図（どちらかにロックインされない）に沿い、ポートの抽象が 2 つの実装で確かめられる。悪い点: 候補の Paddle は利用規約が支援・寄付・コミュニティへのアクセスを禁じ、ほかの候補は調べていない。N1 でも fake と契約テストがポートを Stripe の型から切り離す。

### API の呼び出し: R1 `stripe-go`（採用）/ R2 REST を直接

- R2 の最も強い論点: 依存が増えず、API の版を自分で固定でき、SDK の major の版の追従が要らない。悪い点: 使う 10 ほどの API の型・ページ送り・エラー・冪等キー・再試行を自分で持つ。R1 の依存は MIT の 2 つで、`CGO_ENABLED=0` と `go-licenses` を通る。

### webhook の検証: W1 標準ライブラリ（採用）/ W2 `webhook.ConstructEvent`

- W2 の良い点: 公式の実装で、自分で書くコードがない。悪い点: イベントの API の版の系列が SDK と違うと拒むので、SDK を上げるとエンドポイントの API の版も同時に上げる必要がある（`IgnoreAPIVersionMismatch` で外せるが、そのとき型付きの本文の形が合う保証はない）。W1 は 40 行ほどで、契約テストで確かめる。

### プランの変更: P1 こちらの画面と `ChangePrice`（採用）/ P2 顧客ポータル

- P2 の最も強い論点: 期間の終わりへの変更（`schedule_at_period_end` の `decreasing_item_amount`・`shortening_interval`）と日割りを Stripe が実装し、adapter のコードが減る。悪い点: 変更先は 10 個の Product まで、プランの名前と説明は Stripe の表示で i18n（ADR-0009）の外になり、ポータルの設定をプランの作成・廃止のたびに同期する。プランの規則（ADR-0072）がポータルの設定に分かれ、ほかの事業者に移せない。

### 照合: C1 一覧とカーソルと上限（採用）/ C2 毎回 1 件ずつ取り直す / C3 events だけ

- C2 の最も強い論点: こちらの記録だけで決まり、コードが最も少ない。悪い点: 要求の数が加入者の数に比例し、1 万人で 1 回 1 万要求・1 秒 20 件で 8 分を越え、レート制限に当たる（決定者の回答 3）。
- C3 の最も強い論点: 変わったものだけを取り直すので最も安い。悪い点: 30 日しか遡れず、照合が長く止まると追いつけず、イベントの漏れを直せない。C1 はこれを 1 の段に使い、見回しで補う。
- C1 の悪い点: 4 段とカーソルと続きのジョブの分だけコードが増え、ごく大きなサーバーでは見回しの 1 周が 1 回に収まらない。

### 設定: S1 デプロイの設定（採用）/ S2 管理画面と DB

- S2 の最も強い論点: FR-910 の「権限を持つ人が事業者を設定して課金を有効にする」を再デプロイなしに管理画面だけで満たし、`Sealer`（TOTP の秘密で使っている）がもうある。悪い点: 返金や解約ができる鍵が DB に入り、DB と KEK が両方漏れると使われる。CAPTCHA の秘密（ADR-0042）や ADR-0068 の設定とも場所が違う。S1 でも、有効化とプランは管理画面で行う。

### 外向きの HTTP: H1 SDK の HTTP クライアント（採用）/ H2 `kernel.HTTPClient`

- H2 の最も強い論点: 外向きの HTTP の方針が 1 か所になり、CAPTCHA の検証（固定の公開エンドポイント）も `kernel.HTTPClient` を通している。悪い点: 宛先は SDK が固定する Stripe のホストだけで、利用者もリモートのサーバーも変えられないので、SSRF ガードが守るものがない。SDK は `*http.Client` を受けるので、`kernel.HTTPClient` を `http.RoundTripper` に包む橋と、一覧のページに合う新しい用途の上限が要る。

## Open Questions / 要検証

- Subscription Schedule で、既存のサブスクリプションの Price を期間の終わりに変え、取り下げられるか（#920・#924）。
- アーカイブした Price のサブスクリプションが、その Price のまま更新され続けるか（#924）。
- `subscription_data.metadata` の正確な項目名、Account の `settings.dashboard.timezone` を API で読めるか、制限付きの鍵（`rk_`）に要る権限の一覧（#920）。
- 通貨ごとの最低の請求額の表と、Stripe が ISO 4217 と違う小数の桁で扱う通貨（#920）。
- Stripe の読み取りのレート制限の正確な値と、`GET /v1/subscriptions` の一覧で `latest_invoice` を展開できるか（#920）。
